package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"editorial-content-api/internal/config"
	"editorial-content-api/internal/service"
)

// Router contains HTTP handlers for public and admin clients.
type Router struct {
	postService  *service.PostService
	authService  *service.AuthService
	imageService *service.ImageService
	loginLimiter *loginRateLimiter
	cfg          config.Config
	logger       *slog.Logger
	mux          *http.ServeMux
}

// NewRouter wires all HTTP routes and middleware.
func NewRouter(
	postService *service.PostService,
	authService *service.AuthService,
	imageService *service.ImageService,
	cfg config.Config,
	logger *slog.Logger,
) http.Handler {
	router := &Router{
		postService:  postService,
		authService:  authService,
		imageService: imageService,
		loginLimiter: newLoginRateLimiter(cfg.LoginRateLimit, cfg.LoginRateWindow),
		cfg:          cfg,
		logger:       logger,
		mux:          http.NewServeMux(),
	}

	router.routes()

	var handler http.Handler = router.mux
	handler = corsMiddleware(cfg.AllowedOrigins, handler)
	handler = loggingMiddleware(logger, handler)
	return handler
}

func (r *Router) routes() {
	r.mux.HandleFunc("GET /healthz", r.handleHealth)
	r.mux.HandleFunc("GET /posts", r.handleListPublishedPosts)
	r.mux.HandleFunc("GET /posts/{slug}", r.handleGetPublishedPost)

	r.mux.HandleFunc("POST /admin/login", r.loginLimiter.middleware(r.handleLogin))
	r.mux.HandleFunc("GET /admin/me", requireAdmin(r.authService, r.handleMe))
	r.mux.HandleFunc("POST /admin/uploads/images", requireAdmin(r.authService, r.handleUploadImage))
	r.mux.HandleFunc("GET /admin/posts", requireAdmin(r.authService, r.handleListAdminPosts))
	r.mux.HandleFunc("POST /admin/posts", requireAdmin(r.authService, r.handleSaveDraft))
	r.mux.HandleFunc("POST /admin/posts/{id}/publish", requireAdmin(r.authService, r.handlePublish))
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	if decoder.More() {
		return errors.New("invalid json: multiple objects")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, "encode json response", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

func parseIntQuery(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
