package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"editorial-content-api/internal/config"
	"editorial-content-api/internal/domain"
	"editorial-content-api/internal/service"
)

// Router contains HTTP handlers for public and admin clients.
type Router struct {
	postService *service.PostService
	cfg         config.Config
	logger      *slog.Logger
	mux         *http.ServeMux
}

// NewRouter wires all HTTP routes.
func NewRouter(postService *service.PostService, cfg config.Config, logger *slog.Logger) http.Handler {
	router := &Router{
		postService: postService,
		cfg:         cfg,
		logger:      logger,
		mux:         http.NewServeMux(),
	}

	router.routes()
	return loggingMiddleware(logger, router.mux)
}

func (r *Router) routes() {
	r.mux.HandleFunc("GET /healthz", r.handleHealth)
	r.mux.HandleFunc("GET /posts", r.handleListPublishedPosts)
	r.mux.HandleFunc("GET /posts/{slug}", r.handleGetPublishedPost)

	r.mux.HandleFunc("GET /admin/posts", r.requireAdmin(r.handleListAdminPosts))
	r.mux.HandleFunc("POST /admin/posts", r.requireAdmin(r.handleSaveDraft))
	r.mux.HandleFunc("POST /admin/posts/{id}/publish", r.requireAdmin(r.handlePublish))
}

func (r *Router) handleHealth(w http.ResponseWriter, request *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"env":    r.cfg.Env,
	})
}

func (r *Router) handleListPublishedPosts(w http.ResponseWriter, request *http.Request) {
	posts, err := r.postService.List(request.Context(), domain.ListPostsFilter{
		Status: domain.PostStatusPublished,
		Limit:  parseIntQuery(request, "limit", 20),
		Offset: parseIntQuery(request, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list published posts")
		return
	}

	writeJSON(w, http.StatusOK, posts)
}

func (r *Router) handleGetPublishedPost(w http.ResponseWriter, request *http.Request) {
	post, err := r.postService.GetPublicBySlug(request.Context(), request.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, post)
}

func (r *Router) handleListAdminPosts(w http.ResponseWriter, request *http.Request) {
	posts, err := r.postService.List(request.Context(), domain.ListPostsFilter{
		Status: domain.PostStatus(request.URL.Query().Get("status")),
		Limit:  parseIntQuery(request, "limit", 20),
		Offset: parseIntQuery(request, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list posts")
		return
	}

	writeJSON(w, http.StatusOK, posts)
}

func (r *Router) handleSaveDraft(w http.ResponseWriter, request *http.Request) {
	var input service.SavePostInput
	if err := decodeJSON(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	post, err := r.postService.SaveDraft(request.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, post)
}

func (r *Router) handlePublish(w http.ResponseWriter, request *http.Request) {
	post, err := r.postService.Publish(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := r.revalidateNext(request.Context(), post); err != nil {
		r.logger.Warn("revalidate next failed", "post_id", post.ID, "slug", post.Slug, "error", err)
	}

	writeJSON(w, http.StatusOK, post)
}

func (r *Router) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("X-Admin-API-Key")
		if token == "" {
			token = strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		}
		if token == "" || token != r.cfg.AdminAPIKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next(w, request)
	}
}

func (r *Router) revalidateNext(ctx context.Context, post domain.Post) error {
	if r.cfg.RevalidateURL == "" {
		return nil
	}

	payload := map[string]string{
		"secret": r.cfg.RevalidateSecret,
		"path":   "/posts/" + post.Slug,
		"tag":    "posts",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal revalidate payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.RevalidateURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create revalidate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call revalidate endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("revalidate failed with status %s", resp.Status)
	}

	return nil
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 2<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	if decoder.Decode(&struct{}{}) == nil {
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

func parseIntQuery(request *http.Request, key string, fallback int) int {
	value := request.URL.Query().Get(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(w, request)
		logger.Info("http request", "method", request.Method, "path", request.URL.Path)
	})
}
