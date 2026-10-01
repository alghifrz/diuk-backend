// Package storage is the object-storage boundary used by domain services.
// Supabase Storage is the current implementation; callers depend on ObjectStorage.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Errors returned by object storage operations. Callers map these to HTTP
// codes; raw provider errors never leave this package.
var (
	ErrNotConfigured  = errors.New("object storage is not configured")
	ErrNotFound       = errors.New("object not found")
	ErrUploadFailed   = errors.New("object upload failed")
	ErrDownloadFailed = errors.New("object download failed")
	ErrDeleteFailed   = errors.New("object delete failed")
	ErrSignFailed     = errors.New("signed url failed")
)

const DefaultSignedURLTTL = 15 * time.Minute

// ObjectStorage is the backend-owned file store. Paths are always generated
// by the caller; this interface never accepts a public URL from a client.
type ObjectStorage interface {
	Upload(ctx context.Context, path string, contentType string, data io.Reader) error
	Download(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
	CreateSignedURL(ctx context.Context, path string, expiresIn time.Duration) (string, error)
	CreateSignedURLs(ctx context.Context, paths []string, expiresIn time.Duration) (map[string]string, error)
}
