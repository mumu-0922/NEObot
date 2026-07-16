package ragsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"neo-chat/mm-chat/backend/internal/storage"
)

type Service struct {
	repo           Repository
	store          storage.ObjectStore
	internalToken  string
	maxSourceBytes int64
}

type ServiceOption func(*Service)

func WithInternalToken(token string) ServiceOption {
	return func(s *Service) {
		s.internalToken = strings.TrimSpace(token)
	}
}

func WithMaxSourceBytes(maxSourceBytes int64) ServiceOption {
	return func(s *Service) {
		if maxSourceBytes > 0 {
			s.maxSourceBytes = maxSourceBytes
		}
	}
}

func NewService(repo Repository, store storage.ObjectStore, opts ...ServiceOption) *Service {
	service := &Service{
		repo:           repo,
		store:          store,
		maxSourceBytes: DefaultMaxSourceBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) InternalTokenConfigured() bool {
	return s != nil && strings.TrimSpace(s.internalToken) != ""
}

func (s *Service) InternalToken() string {
	if s == nil {
		return ""
	}
	return s.internalToken
}

func (s *Service) Fetch(ctx context.Context, input SourceObjectInput) (SourceObject, error) {
	if s == nil || s.repo == nil || s.store == nil || !s.InternalTokenConfigured() {
		return SourceObject{}, ErrServiceUnavailable
	}
	input, err := normalizeInput(input)
	if err != nil {
		return SourceObject{}, newError("INVALID_SOURCE_OBJECT_REQUEST", "source object request is invalid", err)
	}
	metadata, err := s.repo.FetchParseSourceMetadata(ctx, input)
	if err != nil {
		if errors.Is(err, ErrServiceUnavailable) {
			return SourceObject{}, ErrServiceUnavailable
		}
		if errors.Is(err, ErrSourceUnavailable) {
			return SourceObject{}, newError("RAG_SOURCE_OBJECT_UNAVAILABLE", "source object is unavailable", err)
		}
		return SourceObject{}, err
	}
	if metadata.FileID != input.FileID {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_MISMATCH", "source object metadata mismatch", ErrSourceMismatch)
	}
	metadata, err = normalizeMetadata(metadata)
	if err != nil {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_MISMATCH", "source object metadata mismatch", err)
	}
	maxBytes := s.maxSourceBytes
	if maxBytes <= 0 || maxBytes > DefaultMaxSourceBytes {
		maxBytes = DefaultMaxSourceBytes
	}
	if metadata.ByteSize > maxBytes {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_TOO_LARGE", "source object exceeds configured limit", ErrSourceMismatch)
	}

	reader, info, err := s.store.Get(ctx, metadata.ObjectKey)
	if err != nil {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_UNAVAILABLE", "source object is unavailable", err)
	}
	defer reader.Close()
	if info.Key != "" && info.Key != metadata.ObjectKey {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_MISMATCH", "source object metadata mismatch", ErrSourceMismatch)
	}
	if info.Size != 0 && info.Size != metadata.ByteSize {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_MISMATCH", "source object metadata mismatch", ErrSourceMismatch)
	}

	body, err := io.ReadAll(io.LimitReader(reader, metadata.ByteSize+1))
	if err != nil {
		return SourceObject{}, fmt.Errorf("read source object: %w", err)
	}
	if int64(len(body)) != metadata.ByteSize {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_MISMATCH", "source object metadata mismatch", ErrSourceMismatch)
	}
	digest := sha256.Sum256(body)
	shaHex := hex.EncodeToString(digest[:])
	if shaHex != metadata.SHA256 {
		return SourceObject{}, newError("RAG_SOURCE_OBJECT_HASH_MISMATCH", "source object hash mismatch", ErrSourceMismatch)
	}
	return SourceObject{
		FileID:      metadata.FileID,
		Body:        body,
		SHA256:      metadata.SHA256,
		ByteSize:    metadata.ByteSize,
		ContentType: metadata.ContentType,
	}, nil
}
