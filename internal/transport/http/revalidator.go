package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"editorial-content-api/internal/domain"
)

// HTTPRevalidator notifies the Next.js blog that a post was published so it can
// invalidate ISR caches.
type HTTPRevalidator struct {
	url    string
	secret string
	client *http.Client
	logger *slog.Logger
}

// NewHTTPRevalidator constructs a revalidator. Pass an empty url to disable.
func NewHTTPRevalidator(url, secret string, logger *slog.Logger) *HTTPRevalidator {
	return &HTTPRevalidator{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

// Revalidate calls the configured Next.js revalidate endpoint. Errors are
// logged and returned so callers can react if desired; PostService treats them
// as non-fatal.
func (h *HTTPRevalidator) Revalidate(ctx context.Context, post domain.Post) error {
	if h == nil || h.url == "" {
		return nil
	}

	payload := map[string]string{
		"secret": h.secret,
		"path":   "/posts/" + post.Slug,
		"tag":    "posts",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal revalidate payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create revalidate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Warn("revalidate next failed", "post_id", post.ID, "slug", post.Slug, "error", err)
		return fmt.Errorf("call revalidate endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger.Warn("revalidate next non-2xx",
			"post_id", post.ID,
			"slug", post.Slug,
			"status", resp.Status,
		)
		return fmt.Errorf("revalidate failed with status %s", resp.Status)
	}

	return nil
}
