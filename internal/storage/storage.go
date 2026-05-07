package storage

import "context"

// Object represents a stored file body and its metadata.
type Object struct {
	Key         string
	Content     []byte
	ContentType string
}

// ObjectStore defines the storage contract used by the article service.
type ObjectStore interface {
	Put(ctx context.Context, object Object) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	PublicURL(key string) string
}
