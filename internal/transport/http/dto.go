package httptransport

import (
	"time"

	"editorial-content-api/internal/domain"
	"editorial-content-api/internal/service"
)

// PostResponse is the JSON shape returned for a single post.
type PostResponse struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	Excerpt          string     `json:"excerpt"`
	MarkdownPath     string     `json:"markdownPath"`
	RenderedHTMLPath string     `json:"renderedHtmlPath"`
	CoverImagePath   string     `json:"coverImagePath,omitempty"`
	Status           string     `json:"status"`
	AuthorID         string     `json:"authorId,omitempty"`
	SEOTitle         string     `json:"seoTitle,omitempty"`
	SEODescription   string     `json:"seoDescription,omitempty"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// PublicPostResponse extends PostResponse with rendered HTML.
type PublicPostResponse struct {
	PostResponse
	HTML string `json:"html"`
}

// PostListResponse is the paginated list payload.
type PostListResponse struct {
	Items  []PostResponse `json:"items"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// UserResponse is the JSON shape returned for an administrator user.
type UserResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"isActive"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// LoginResponse is the JSON shape returned after a successful login.
type LoginResponse struct {
	AccessToken string       `json:"accessToken"`
	TokenType   string       `json:"tokenType"`
	ExpiresAt   time.Time    `json:"expiresAt"`
	User        UserResponse `json:"user"`
}

// AuthenticatedUserResponse exposes the trusted identity from the JWT.
type AuthenticatedUserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func toPostResponse(post domain.Post) PostResponse {
	return PostResponse{
		ID:               post.ID,
		Title:            post.Title,
		Slug:             post.Slug,
		Excerpt:          post.Excerpt,
		MarkdownPath:     post.MarkdownPath,
		RenderedHTMLPath: post.RenderedHTMLPath,
		CoverImagePath:   post.CoverImagePath,
		Status:           string(post.Status),
		AuthorID:         post.AuthorID,
		SEOTitle:         post.SEOTitle,
		SEODescription:   post.SEODescription,
		PublishedAt:      post.PublishedAt,
		CreatedAt:        post.CreatedAt,
		UpdatedAt:        post.UpdatedAt,
	}
}

func toPublicPostResponse(post domain.PublicPost) PublicPostResponse {
	return PublicPostResponse{
		PostResponse: toPostResponse(post.Post),
		HTML:         post.HTML,
	}
}

func toPostListResponse(list domain.PostList) PostListResponse {
	items := make([]PostResponse, 0, len(list.Items))
	for _, post := range list.Items {
		items = append(items, toPostResponse(post))
	}

	return PostListResponse{
		Items:  items,
		Total:  list.Total,
		Limit:  list.Limit,
		Offset: list.Offset,
	}
}

func toUserResponse(user domain.User) UserResponse {
	return UserResponse{
		ID:          user.ID,
		Email:       user.Email,
		Role:        string(user.Role),
		IsActive:    user.IsActive,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func toLoginResponse(result service.LoginResult) LoginResponse {
	return LoginResponse{
		AccessToken: result.AccessToken,
		TokenType:   result.TokenType,
		ExpiresAt:   result.ExpiresAt,
		User:        toUserResponse(result.User),
	}
}

func toAuthenticatedUserResponse(user service.AuthenticatedUser) AuthenticatedUserResponse {
	return AuthenticatedUserResponse{
		ID:    user.ID,
		Email: user.Email,
		Role:  string(user.Role),
	}
}
