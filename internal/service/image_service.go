package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"editorial-content-api/internal/storage"
)

var (
	// ErrImageTooLarge is returned when an upload exceeds the configured byte limit.
	ErrImageTooLarge = errors.New("image is too large")
	// ErrInvalidImage is returned when an upload is not a supported image type.
	ErrInvalidImage = errors.New("invalid image")
)

// ImageUploadConfig contains image upload constraints.
type ImageUploadConfig struct {
	MaxBytes int64
}

// ImageUploadResult is returned after a successful image upload.
type ImageUploadResult struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
}

// ImageService validates image uploads and stores the original file.
type ImageService struct {
	objectStore storage.ObjectStore
	maxBytes    int64
}

// NewImageService creates an ImageService.
func NewImageService(objectStore storage.ObjectStore, cfg ImageUploadConfig) *ImageService {
	return &ImageService{
		objectStore: objectStore,
		maxBytes:    cfg.MaxBytes,
	}
}

// Upload stores an image as uploaded and returns its public access path.
func (s *ImageService) Upload(ctx context.Context, reader io.Reader) (ImageUploadResult, error) {
	body, err := s.readLimited(reader)
	if err != nil {
		return ImageUploadResult{}, err
	}

	contentType := http.DetectContentType(body)
	ext := imageExtension(contentType)
	if ext == "" {
		return ImageUploadResult{}, ErrInvalidImage
	}

	imageID, err := randomID()
	if err != nil {
		return ImageUploadResult{}, err
	}

	now := time.Now().UTC()
	basePath := filepath.ToSlash(filepath.Join(
		"uploads",
		"images",
		now.Format("2006"),
		now.Format("01"),
		imageID,
	))
	key := strings.TrimRight(basePath, "/") + "/original" + ext
	if err := s.objectStore.Put(ctx, storage.Object{
		Key:         key,
		Content:     body,
		ContentType: contentType,
	}); err != nil {
		return ImageUploadResult{}, err
	}

	return ImageUploadResult{
		ID:          imageID,
		Key:         key,
		URL:         s.objectStore.PublicURL(key),
		ContentType: contentType,
	}, nil
}

func (s *ImageService) readLimited(reader io.Reader) ([]byte, error) {
	if s.maxBytes <= 0 {
		return nil, errors.New("image upload max bytes must be positive")
	}

	body, err := io.ReadAll(io.LimitReader(reader, s.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image upload: %w", err)
	}
	if int64(len(body)) > s.maxBytes {
		return nil, ErrImageTooLarge
	}

	return body, nil
}

func imageExtension(contentType string) string {
	switch strings.ToLower(contentType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
