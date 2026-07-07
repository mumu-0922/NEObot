package files

import (
	"context"
	"io"
	"time"
)

const DevUserID = "00000000-0000-0000-0000-000000000001"

type Repository interface {
	CreateFile(ctx context.Context, input CreateFileInput) (FileRecord, error)
	GetFile(ctx context.Context, fileID string) (FileRecord, error)
	MarkFileDeleted(ctx context.Context, fileID string) (FileRecord, error)
}

type CreateFileInput struct {
	ID               string
	OriginalFilename string
	MimeType         string
	ByteSize         int64
	SHA256           string
	StorageBackend   string
	ObjectKey        string
	Metadata         map[string]any
}

type UploadInput struct {
	OriginalFilename string
	MimeType         string
	Size             int64
	Purpose          string
	ConversationID   string
	WorkspaceID      string
	CollectionID     string
	ClientFileID     string
	Body             io.Reader
}

type FileRecord struct {
	ID               string
	UserID           string
	OriginalFilename string
	MimeType         string
	ByteSize         int64
	SHA256           string
	StorageBackend   string
	ObjectKey        string
	UploadStatus     string
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
