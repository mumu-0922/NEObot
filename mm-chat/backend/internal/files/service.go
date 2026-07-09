package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	DefaultStorageBackend = "local"
)

type Service struct {
	repo           Repository
	store          storage.ObjectStore
	storageBackend string
	newID          func() (string, error)
}

type ServiceOption func(*Service)

func WithStorageBackend(backend string) ServiceOption {
	return func(s *Service) {
		backend = strings.ToLower(strings.TrimSpace(backend))
		if backend != "" {
			s.storageBackend = backend
		}
	}
}

func NewService(repo Repository, store storage.ObjectStore, opts ...ServiceOption) *Service {
	service := &Service{
		repo:           repo,
		store:          store,
		storageBackend: DefaultStorageBackend,
		newID:          newUUID,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (FileRecord, error) {
	if err := s.requireReady(); err != nil {
		return FileRecord{}, err
	}
	input.OriginalFilename = safeDisplayFilename(input.OriginalFilename)
	input.MimeType = normalizeMimeType(input.MimeType)
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	if !isValidPurpose(input.Purpose) {
		return FileRecord{}, newValidationError("INVALID_FILE_PURPOSE", "file purpose is required or unsupported")
	}
	if input.Body == nil {
		return FileRecord{}, newValidationError("FILE_REQUIRED", "file is required")
	}
	if input.Size < 0 {
		return FileRecord{}, newValidationError("FILE_TOO_LARGE", "file size is invalid")
	}

	id, err := s.generateID()
	if err != nil {
		return FileRecord{}, err
	}
	objectKey := objectKeyForUser(auth.UserOrDevelopment(ctx).ID, id)

	hasher := sha256.New()
	tee := io.TeeReader(input.Body, hasher)
	if err := s.store.Put(ctx, objectKey, tee, input.Size, input.MimeType); err != nil {
		return FileRecord{}, fmt.Errorf("store file object: %w", err)
	}
	shaHex := hex.EncodeToString(hasher.Sum(nil))

	metadata := map[string]any{
		"purpose": input.Purpose,
	}
	addOptionalMetadata(metadata, "conversationId", input.ConversationID)
	addOptionalMetadata(metadata, "workspaceId", input.WorkspaceID)
	addOptionalMetadata(metadata, "knowledgeCollectionId", input.CollectionID)
	addOptionalMetadata(metadata, "clientFileId", input.ClientFileID)

	record, err := s.repo.CreateFile(ctx, CreateFileInput{
		ID:               id,
		OriginalFilename: input.OriginalFilename,
		MimeType:         input.MimeType,
		ByteSize:         input.Size,
		SHA256:           shaHex,
		StorageBackend:   s.storageBackend,
		ObjectKey:        objectKey,
		Metadata:         metadata,
	})
	if err != nil {
		_ = s.store.Delete(context.Background(), objectKey)
		return FileRecord{}, err
	}

	return record, nil
}

func (s *Service) GetMetadata(ctx context.Context, fileID string) (FileRecord, error) {
	if err := s.requireRepository(); err != nil {
		return FileRecord{}, err
	}
	fileID = strings.TrimSpace(fileID)
	if !isUUID(fileID) {
		return FileRecord{}, newValidationError("INVALID_FILE_ID", "file id must be a UUID")
	}

	return s.repo.GetFile(ctx, fileID)
}

func (s *Service) GetContent(ctx context.Context, fileID string) (FileRecord, io.ReadCloser, error) {
	if err := s.requireReady(); err != nil {
		return FileRecord{}, nil, err
	}
	record, err := s.GetMetadata(ctx, fileID)
	if err != nil {
		return FileRecord{}, nil, err
	}
	reader, _, err := s.store.Get(ctx, record.ObjectKey)
	if err != nil {
		return FileRecord{}, nil, fmt.Errorf("read file object: %w", err)
	}

	return record, reader, nil
}

func (s *Service) Delete(ctx context.Context, fileID string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	fileID = strings.TrimSpace(fileID)
	if !isUUID(fileID) {
		return newValidationError("INVALID_FILE_ID", "file id must be a UUID")
	}

	record, err := s.repo.MarkFileDeleted(ctx, fileID)
	if err != nil {
		return err
	}
	if err := s.store.Delete(ctx, record.ObjectKey); err != nil {
		return fmt.Errorf("delete file object: %w", err)
	}

	return nil
}

func (s *Service) requireReady() error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return ErrStorageRequired
	}
	return nil
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (s *Service) generateID() (string, error) {
	newID := s.newID
	if newID == nil {
		newID = newUUID
	}
	return newID()
}

func objectKeyFor(fileID string) string {
	return objectKeyForUser(DevUserID, fileID)
}

func objectKeyForUser(userID string, fileID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = DevUserID
	}
	return "users/" + userID + "/files/" + strings.TrimSpace(fileID)
}

func safeDisplayFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload.bin"
	}
	return name
}

func normalizeMimeType(mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func isValidPurpose(purpose string) bool {
	switch purpose {
	case "chat", "workspace", "knowledge", "image", "audio", "export":
		return true
	default:
		return false
	}
}

func addOptionalMetadata(metadata map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		metadata[key] = value
	}
}
