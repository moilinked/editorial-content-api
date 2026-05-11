package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains runtime settings loaded from environment variables.
type Config struct {
	Env                     string
	Host                    string
	Port                    string
	DatabaseURL             string
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime time.Duration
	PublicBaseURL           string
	JWTSecret               string
	JWTIssuer               string
	JWTAccessTokenTTL       time.Duration
	ImageUploadMaxBytes     int64
	RevalidateURL           string
	RevalidateSecret        string
	AllowedOrigins          []string
	LoginRateLimit          int
	LoginRateWindow         time.Duration
	Storage                 StorageConfig
}

// StorageConfig contains S3-compatible storage settings for SeaweedFS.
type StorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	PublicBaseURL   string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	cfg := Config{
		Env:                     getenv("APP_ENV", "development"),
		Host:                    getenv("HTTP_HOST", "0.0.0.0"),
		Port:                    getenv("HTTP_PORT", "8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		DatabaseMaxOpenConns:    getenvInt("DATABASE_MAX_OPEN_CONNS", 25),
		DatabaseMaxIdleConns:    getenvInt("DATABASE_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime: getenvDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		PublicBaseURL:           getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		JWTIssuer:               getenv("JWT_ISSUER", "editorial-content-api"),
		JWTAccessTokenTTL:       getenvDuration("JWT_ACCESS_TOKEN_TTL", time.Hour),
		ImageUploadMaxBytes:     getenvInt64("IMAGE_UPLOAD_MAX_BYTES", 10<<20),
		RevalidateURL:           os.Getenv("NEXT_REVALIDATE_URL"),
		RevalidateSecret:        os.Getenv("NEXT_REVALIDATE_SECRET"),
		AllowedOrigins:          splitCSV(os.Getenv("ALLOWED_ORIGINS")),
		LoginRateLimit:          getenvInt("LOGIN_RATE_LIMIT", 10),
		LoginRateWindow:         getenvDuration("LOGIN_RATE_WINDOW", time.Minute),
		Storage: StorageConfig{
			Endpoint:        getenv("S3_ENDPOINT", "http://localhost:8333"),
			Region:          getenv("S3_REGION", "us-east-1"),
			Bucket:          getenv("S3_BUCKET", "blog"),
			AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			UsePathStyle:    getenvBool("S3_USE_PATH_STYLE", true),
			PublicBaseURL:   os.Getenv("S3_PUBLIC_BASE_URL"),
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.JWTAccessTokenTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_ACCESS_TOKEN_TTL must be positive")
	}
	if cfg.Storage.Bucket == "" {
		return Config{}, fmt.Errorf("S3_BUCKET is required")
	}
	if cfg.ImageUploadMaxBytes <= 0 {
		return Config{}, fmt.Errorf("IMAGE_UPLOAD_MAX_BYTES must be positive")
	}
	if cfg.LoginRateLimit < 0 {
		return Config{}, fmt.Errorf("LOGIN_RATE_LIMIT must not be negative")
	}
	if cfg.LoginRateWindow <= 0 {
		return Config{}, fmt.Errorf("LOGIN_RATE_WINDOW must be positive")
	}

	return cfg, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// HTTPAddr returns the host:port pair used by net/http.
func (c Config) HTTPAddr() string {
	return net.JoinHostPort(c.Host, c.Port)
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
