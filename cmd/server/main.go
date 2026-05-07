package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"editorial-content-api/internal/config"
	"editorial-content-api/internal/markdown"
	mysqlrepo "editorial-content-api/internal/repository/mysql"
	"editorial-content-api/internal/service"
	s3store "editorial-content-api/internal/storage/s3"
	httptransport "editorial-content-api/internal/transport/http"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	db, err := gorm.Open(mysql.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("get database handle", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	sqlDB.SetMaxOpenConns(cfg.DatabaseMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DatabaseMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DatabaseConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}

	migrateCtx, migrateCancel := context.WithTimeout(ctx, 10*time.Second)
	defer migrateCancel()
	if err := mysqlrepo.AutoMigrate(migrateCtx, db); err != nil {
		logger.Error("auto migrate database", "error", err)
		os.Exit(1)
	}

	objectStore, err := s3store.New(ctx, cfg.Storage)
	if err != nil {
		logger.Error("create object store", "error", err)
		os.Exit(1)
	}

	postRepo := mysqlrepo.NewPostRepository(db)
	userRepo := mysqlrepo.NewUserRepository(db)
	postService := service.NewPostService(postRepo, objectStore, markdown.NewRenderer(), cfg.PublicBaseURL)
	authService := service.NewAuthService(userRepo, service.AuthConfig{
		JWTSecret:      cfg.JWTSecret,
		JWTIssuer:      cfg.JWTIssuer,
		AccessTokenTTL: cfg.JWTAccessTokenTTL,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           httptransport.NewRouter(postService, authService, cfg, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen and serve", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown server", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
