package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidObjectKey = errors.New("invalid object key")
	ErrObjectNotFound   = errors.New("object not found")
)

// ObjectStore stores opaque file bytes under server-generated object keys.
// Callers own authorization, file metadata, hashing, and MIME validation.
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	UpdatedAt   time.Time
}
