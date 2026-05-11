package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"editorial-content-api/internal/domain"
	"gorm.io/gorm"
)

// PostRepository persists article metadata in MySQL.
type PostRepository struct {
	db *gorm.DB
}

// NewPostRepository creates a MySQL-backed post repository.
func NewPostRepository(db *gorm.DB) *PostRepository {
	return &PostRepository{db: db}
}

// Create inserts a new post row.
func (r *PostRepository) Create(ctx context.Context, post domain.Post) (domain.Post, error) {
	record := postRecordFromDomain(post)
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return domain.Post{}, fmt.Errorf("create post: %w", err)
	}

	return record.toDomain(), nil
}

// Update writes editable post metadata and publication state.
func (r *PostRepository) Update(ctx context.Context, post domain.Post) (domain.Post, error) {
	values := map[string]any{
		"title":              post.Title,
		"slug":               post.Slug,
		"excerpt":            post.Excerpt,
		"markdown_path":      post.MarkdownPath,
		"rendered_html_path": post.RenderedHTMLPath,
		"cover_image_path":   stringPointer(post.CoverImagePath),
		"status":             post.Status,
		"author_id":          stringPointer(post.AuthorID),
		"seo_title":          stringPointer(post.SEOTitle),
		"seo_description":    stringPointer(post.SEODescription),
		"published_at":       post.PublishedAt,
	}

	result := r.db.WithContext(ctx).Model(&postRecord{}).Where("id = ?", post.ID).Updates(values)
	if result.Error != nil {
		return domain.Post{}, fmt.Errorf("update post: %w", result.Error)
	}

	return r.FindByID(ctx, post.ID)
}

// UpdateStatus updates only publication status fields for a post.
func (r *PostRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status domain.PostStatus,
	publishedAt *time.Time,
) (domain.Post, error) {
	result := r.db.WithContext(ctx).Model(&postRecord{}).Where("id = ?", id).Updates(map[string]any{
		"status":       status,
		"published_at": publishedAt,
	})
	if result.Error != nil {
		return domain.Post{}, fmt.Errorf("update post status: %w", result.Error)
	}

	return r.FindByID(ctx, id)
}

// FindByID returns a post by ID.
func (r *PostRepository) FindByID(ctx context.Context, id string) (domain.Post, error) {
	var record postRecord
	err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error
	if err != nil {
		return domain.Post{}, fmt.Errorf("find post by id: %w", err)
	}

	return record.toDomain(), nil
}

// FindPublishedBySlug returns a public post by slug.
func (r *PostRepository) FindPublishedBySlug(ctx context.Context, slug string) (domain.Post, error) {
	var record postRecord
	err := r.db.WithContext(ctx).
		Where("slug = ? and status = ?", slug, domain.PostStatusPublished).
		First(&record).
		Error
	if err != nil {
		return domain.Post{}, fmt.Errorf("find published post by slug: %w", err)
	}

	return record.toDomain(), nil
}

// List returns posts sorted by update time and the total number of rows
// matching the filter (ignoring limit/offset) so callers can paginate.
func (r *PostRepository) List(ctx context.Context, filter domain.ListPostsFilter) ([]domain.Post, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	base := r.db.WithContext(ctx).Model(&postRecord{})
	if filter.Status != "" {
		base = base.Where("status = ?", filter.Status)
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count posts: %w", err)
	}

	var records []postRecord
	if err := base.Session(&gorm.Session{}).
		Order("updated_at desc").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}

	posts := make([]domain.Post, 0, len(records))
	for _, record := range records {
		posts = append(posts, record.toDomain())
	}

	return posts, total, nil
}

type postRecord struct {
	SQLModel
	Title            string            `gorm:"column:title;type:varchar(255);not null"`
	Slug             string            `gorm:"column:slug;type:varchar(255);not null;uniqueIndex"`
	Excerpt          string            `gorm:"column:excerpt;type:text;not null"`
	MarkdownPath     string            `gorm:"column:markdown_path;type:varchar(512);not null"`
	RenderedHTMLPath string            `gorm:"column:rendered_html_path;type:varchar(512);not null"`
	CoverImagePath   *string           `gorm:"column:cover_image_path;type:varchar(512)"`
	Status           domain.PostStatus `gorm:"column:status;type:varchar(20);not null;check:status in ('draft','published','archived');index;index:posts_status_published_at_idx,priority:1"`
	AuthorID         *string           `gorm:"column:author_id;type:varchar(255)"`
	SEOTitle         *string           `gorm:"column:seo_title;type:varchar(255)"`
	SEODescription   *string           `gorm:"column:seo_description;type:text"`
	PublishedAt      *time.Time        `gorm:"column:published_at;type:datetime(6);index:posts_status_published_at_idx,priority:2,sort:desc"`
}

func (postRecord) TableName() string {
	return "posts"
}

func postRecordFromDomain(post domain.Post) postRecord {
	return postRecord{
		SQLModel: SQLModel{
			ID: post.ID,
		},
		Title:            post.Title,
		Slug:             post.Slug,
		Excerpt:          post.Excerpt,
		MarkdownPath:     post.MarkdownPath,
		RenderedHTMLPath: post.RenderedHTMLPath,
		CoverImagePath:   stringPointer(post.CoverImagePath),
		Status:           post.Status,
		AuthorID:         stringPointer(post.AuthorID),
		SEOTitle:         stringPointer(post.SEOTitle),
		SEODescription:   stringPointer(post.SEODescription),
		PublishedAt:      post.PublishedAt,
	}
}

func (r postRecord) toDomain() domain.Post {
	post := domain.Post{
		ID:               r.ID,
		Title:            r.Title,
		Slug:             r.Slug,
		Excerpt:          r.Excerpt,
		MarkdownPath:     r.MarkdownPath,
		RenderedHTMLPath: r.RenderedHTMLPath,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
	if r.CoverImagePath != nil {
		post.CoverImagePath = *r.CoverImagePath
	}
	if r.AuthorID != nil {
		post.AuthorID = *r.AuthorID
	}
	if r.SEOTitle != nil {
		post.SEOTitle = *r.SEOTitle
	}
	if r.SEODescription != nil {
		post.SEODescription = *r.SEODescription
	}

	return post
}

func stringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return &value
}
