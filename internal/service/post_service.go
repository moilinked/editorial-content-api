package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	UpdateStatus(ctx context.Context, id string, status domain.PostStatus, publishedAt *time.Time) (domain.Post, error)
	FindByID(ctx context.Context, id string) (domain.Post, error)
	FindPublishedBySlug(ctx context.Context, slug string) (domain.Post, error)
	List(ctx context.Context, filter domain.ListPostsFilter) ([]domain.Post, int64, error)
}

// Renderer defines Markdown rendering behavior.
type Renderer interface {
	Render(ctx context.Context, source string) (markdown.Result, error)
}

// Revalidator is a best-effort hook invoked after a post is published.
// Implementations own their own logging and error semantics; PostService treats
// failures as non-fatal side effects.
type Revalidator interface {
	Revalidate(ctx context.Context, post domain.Post) error
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
	repo        PostRepository
	objectStore storage.ObjectStore
	renderer    Renderer
	revalidator Revalidator
}

// NewPostService creates a PostService. A nil revalidator disables the post-publish hook.
func NewPostService(
	repo PostRepository,
	objectStore storage.ObjectStore,
	renderer Renderer,
	revalidator Revalidator,
) *PostService {
	return &PostService{
		repo:        repo,
		objectStore: objectStore,
		renderer:    renderer,
		revalidator: revalidator,
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
	if post.AuthorID == "" {
		post.AuthorID = existing.AuthorID
	}

	return s.repo.Update(ctx, post)
}

// Publish marks a draft as public after validating required fields.
// The revalidator hook is invoked as a best-effort side effect.
func (s *PostService) Publish(ctx context.Context, id string) (domain.Post, error) {
	post, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return domain.Post{}, fmt.Errorf("find post: %w", err)
	}
	if !post.IsPublishable() {
		return domain.Post{}, errors.New("post is missing title, slug, excerpt, markdown, or rendered html")
	}

	now := time.Now().UTC()
	publishedAt := post.PublishedAt
	if publishedAt == nil {
		publishedAt = &now
	}

	published, err := s.repo.UpdateStatus(ctx, id, domain.PostStatusPublished, publishedAt)
	if err != nil {
		return domain.Post{}, err
	}

	if s.revalidator != nil {
		_ = s.revalidator.Revalidate(ctx, published)
	}

	return published, nil
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

// List returns posts for admin or public listing views along with the total
// number of rows matching the filter, suitable for paginated UI.
func (s *PostService) List(ctx context.Context, filter domain.ListPostsFilter) (domain.PostList, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	normalized := domain.ListPostsFilter{
		Status: filter.Status,
		Limit:  limit,
		Offset: offset,
	}

	items, total, err := s.repo.List(ctx, normalized)
	if err != nil {
		return domain.PostList{}, err
	}

	return domain.PostList{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
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
