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
	ID               string
	Title            string
	Slug             string
	Excerpt          string
	MarkdownPath     string
	RenderedHTMLPath string
	CoverImagePath   string
	Status           PostStatus
	AuthorID         string
	SEOTitle         string
	SEODescription   string
	PublishedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PublicPost is the post payload exposed to public readers, with rendered HTML.
type PublicPost struct {
	Post
	HTML string
}

// PostList groups paginated post results with the total count for the current filter.
type PostList struct {
	Items  []Post
	Total  int64
	Limit  int
	Offset int
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
