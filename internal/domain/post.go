package domain

import "time"

// PostStatus describes the publication state of a post.
type PostStatus string

const (
	// PostStatusDraft means the post is editable but not publicly visible.
	PostStatusDraft PostStatus = "draft"
	// PostStatusPublished means the post is publicly visible.
	PostStatusPublished PostStatus = "published"
	// PostStatusArchived means the post is hidden without being deleted.
	PostStatusArchived PostStatus = "archived"
)

// Post stores article metadata. Markdown and rendered HTML live in object storage.
type Post struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	Excerpt          string     `json:"excerpt"`
	MarkdownPath     string     `json:"markdownPath"`
	RenderedHTMLPath string     `json:"renderedHtmlPath"`
	CoverImagePath   string     `json:"coverImagePath,omitempty"`
	Status           PostStatus `json:"status"`
	AuthorID         string     `json:"authorId,omitempty"`
	SEOTitle         string     `json:"seoTitle,omitempty"`
	SEODescription   string     `json:"seoDescription,omitempty"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// PublicPost is the API shape consumed by the public Next.js blog frontend.
type PublicPost struct {
	Post
	HTML string `json:"html"`
}

// ListPostsFilter contains optional filters for querying posts.
type ListPostsFilter struct {
	Status PostStatus
	Limit  int
	Offset int
}

// IsPublishable returns true when required publishing fields are present.
func (p Post) IsPublishable() bool {
	return p.Title != "" && p.Slug != "" && p.Excerpt != "" && p.MarkdownPath != "" && p.RenderedHTMLPath != ""
}
