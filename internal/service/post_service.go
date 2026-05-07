package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"editorial-content-api/internal/domain"
	"editorial-content-api/internal/markdown"
	"editorial-content-api/internal/storage"
)

// PostRepository defines persistence operations required by PostService.
type PostRepository interface {
	Create(ctx context.Context, post domain.Post) (domain.Post, error)
	Update(ctx context.Context, post domain.Post) (domain.Post, error)
	FindByID(ctx context.Context, id string) (domain.Post, error)
	FindPublishedBySlug(ctx context.Context, slug string) (domain.Post, error)
	List(ctx context.Context, filter domain.ListPostsFilter) ([]domain.Post, error)
}

// Renderer defines Markdown rendering behavior.
type Renderer interface {
	Render(ctx context.Context, source string) (markdown.Result, error)
}

// SavePostInput represents editable article fields from the admin frontend.
type SavePostInput struct {
	ID             string `json:"id,omitempty"`
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Excerpt        string `json:"excerpt"`
	Markdown       string `json:"markdown"`
	CoverImagePath string `json:"coverImagePath,omitempty"`
	AuthorID       string `json:"authorId,omitempty"`
	SEOTitle       string `json:"seoTitle,omitempty"`
	SEODescription string `json:"seoDescription,omitempty"`
}

// PostService coordinates article metadata, Markdown rendering, and object storage.
type PostService struct {
	repo          PostRepository
	objectStore   storage.ObjectStore
	renderer      Renderer
	publicBaseURL string
	httpClient    *http.Client
}

// NewPostService creates a PostService.
func NewPostService(
	repo PostRepository,
	objectStore storage.ObjectStore,
	renderer Renderer,
	publicBaseURL string,
) *PostService {
	return &PostService{
		repo:          repo,
		objectStore:   objectStore,
		renderer:      renderer,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SaveDraft stores article metadata and rendered content without publishing it.
func (s *PostService) SaveDraft(ctx context.Context, input SavePostInput) (domain.Post, error) {
	if err := validateSavePostInput(input); err != nil {
		return domain.Post{}, err
	}

	postID := input.ID
	if postID == "" {
		generatedID, err := randomID()
		if err != nil {
			return domain.Post{}, err
		}
		postID = generatedID
	}

	rendered, err := s.renderer.Render(ctx, input.Markdown)
	if err != nil {
		return domain.Post{}, fmt.Errorf("render post markdown: %w", err)
	}

	markdownPath := fmt.Sprintf("posts/%s/source.md", postID)
	renderedHTMLPath := fmt.Sprintf("posts/%s/rendered.html", postID)

	if err := s.objectStore.Put(ctx, storage.Object{
		Key:         markdownPath,
		Content:     []byte(input.Markdown),
		ContentType: "text/markdown; charset=utf-8",
	}); err != nil {
		return domain.Post{}, fmt.Errorf("store markdown: %w", err)
	}

	if err := s.objectStore.Put(ctx, storage.Object{
		Key:         renderedHTMLPath,
		Content:     []byte(rendered.HTML),
		ContentType: "text/html; charset=utf-8",
	}); err != nil {
		return domain.Post{}, fmt.Errorf("store rendered html: %w", err)
	}

	post := domain.Post{
		ID:               postID,
		Title:            strings.TrimSpace(input.Title),
		Slug:             strings.TrimSpace(input.Slug),
		Excerpt:          strings.TrimSpace(input.Excerpt),
		MarkdownPath:     markdownPath,
		RenderedHTMLPath: renderedHTMLPath,
		CoverImagePath:   strings.TrimSpace(input.CoverImagePath),
		Status:           domain.PostStatusDraft,
		AuthorID:         strings.TrimSpace(input.AuthorID),
		SEOTitle:         strings.TrimSpace(input.SEOTitle),
		SEODescription:   strings.TrimSpace(input.SEODescription),
	}

	if input.ID == "" {
		return s.repo.Create(ctx, post)
	}

	existing, err := s.repo.FindByID(ctx, input.ID)
	if err != nil {
		return domain.Post{}, fmt.Errorf("find existing post: %w", err)
	}
	post.Status = existing.Status
	post.PublishedAt = existing.PublishedAt

	return s.repo.Update(ctx, post)
}

// Publish marks a draft as public after validating required fields.
func (s *PostService) Publish(ctx context.Context, id string) (domain.Post, error) {
	post, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return domain.Post{}, fmt.Errorf("find post: %w", err)
	}
	if !post.IsPublishable() {
		return domain.Post{}, errors.New("post is missing title, slug, excerpt, markdown, or rendered html")
	}

	now := time.Now().UTC()
	post.Status = domain.PostStatusPublished
	if post.PublishedAt == nil {
		post.PublishedAt = &now
	}

	return s.repo.Update(ctx, post)
}

// GetPublicBySlug loads a published post and its rendered HTML.
func (s *PostService) GetPublicBySlug(ctx context.Context, slug string) (domain.PublicPost, error) {
	post, err := s.repo.FindPublishedBySlug(ctx, slug)
	if err != nil {
		return domain.PublicPost{}, fmt.Errorf("find published post: %w", err)
	}

	htmlBody, err := s.objectStore.Get(ctx, post.RenderedHTMLPath)
	if err != nil {
		return domain.PublicPost{}, fmt.Errorf("load rendered html: %w", err)
	}

	return domain.PublicPost{
		Post: post,
		HTML: string(htmlBody),
	}, nil
}

// List returns posts for admin or public listing views.
func (s *PostService) List(ctx context.Context, filter domain.ListPostsFilter) ([]domain.Post, error) {
	return s.repo.List(ctx, filter)
}

func validateSavePostInput(input SavePostInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(input.Slug) == "" {
		return errors.New("slug is required")
	}
	if strings.TrimSpace(input.Excerpt) == "" {
		return errors.New("excerpt is required")
	}
	if strings.TrimSpace(input.Markdown) == "" {
		return errors.New("markdown is required")
	}

	return nil
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}
