package markdown

import (
	"bytes"
	"context"
	"fmt"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// Result contains sanitized HTML generated from Markdown.
type Result struct {
	HTML string
}

// Renderer converts Markdown source into sanitized HTML.
type Renderer struct {
	markdown  goldmark.Markdown
	sanitizer *bluemonday.Policy
}

// NewRenderer creates a Markdown renderer with GitHub-flavored Markdown support.
func NewRenderer() *Renderer {
	return &Renderer{
		markdown: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
			goldmark.WithRendererOptions(html.WithUnsafe()),
		),
		sanitizer: bluemonday.UGCPolicy(),
	}
}

// Render converts Markdown to HTML and removes unsafe markup.
func (r *Renderer) Render(ctx context.Context, source string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("render markdown cancelled: %w", err)
	}

	var buf bytes.Buffer
	if err := r.markdown.Convert([]byte(source), &buf); err != nil {
		return Result{}, fmt.Errorf("convert markdown: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("render markdown cancelled: %w", err)
	}

	return Result{
		HTML: r.sanitizer.Sanitize(buf.String()),
	}, nil
}
