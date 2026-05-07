package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"editorial-content-api/internal/config"
	"editorial-content-api/internal/storage"
)

// Store implements ObjectStore using a SeaweedFS S3-compatible gateway.
type Store struct {
	client        *s3.Client
	bucket        string
	publicBaseURL string
}

// New creates an S3-compatible object store.
func New(ctx context.Context, cfg config.StorageConfig) (*Store, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		options.UsePathStyle = cfg.UsePathStyle
	})

	return &Store{
		client:        client,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

// Put uploads an object.
func (s *Store) Put(ctx context.Context, object storage.Object) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(object.Key),
		Body:        bytes.NewReader(object.Content),
		ContentType: aws.String(object.ContentType),
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", object.Key, err)
	}

	return nil
}

// Get downloads an object into memory.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	defer output.Body.Close()

	body, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", key, err)
	}

	return body, nil
}

// Delete removes an object.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}

	return nil
}

// PublicURL returns the public URL for a stored object key.
func (s *Store) PublicURL(key string) string {
	if s.publicBaseURL == "" {
		return ""
	}

	escapedKey := strings.ReplaceAll(url.PathEscape(key), "%2F", "/")
	return s.publicBaseURL + "/" + escapedKey
}
