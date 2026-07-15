package jobartifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/files"
)

type Kind string

const (
	KindImage Kind = "image"
	KindAudio Kind = "audio"
)

var (
	ErrUploaderRequired     = errors.New("job artifact uploader is required")
	ErrArtifactBodyRequired = errors.New("job artifact body is required")
	ErrArtifactSizeInvalid  = errors.New("job artifact size is invalid")
	ErrArtifactKindInvalid  = errors.New("job artifact kind is invalid")
	ErrArtifactTypeInvalid  = errors.New("job artifact content type is invalid")
)

type Uploader interface {
	Upload(context.Context, files.UploadInput) (files.FileRecord, error)
}

type StoreInput struct {
	JobID       string
	Kind        Kind
	Filename    string
	ContentType string
	Size        int64
	Body        io.Reader
}

type Artifact struct {
	FileID      string `json:"fileId"`
	Purpose     string `json:"purpose"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type Service struct {
	uploader Uploader
}

func NewService(uploader Uploader) *Service {
	return &Service{uploader: uploader}
}

func (s *Service) Store(ctx context.Context, input StoreInput) (Artifact, error) {
	if s == nil || s.uploader == nil {
		return Artifact{}, ErrUploaderRequired
	}
	if input.Body == nil {
		return Artifact{}, ErrArtifactBodyRequired
	}
	if input.Size <= 0 {
		return Artifact{}, ErrArtifactSizeInvalid
	}
	purpose, err := purposeForKind(input.Kind)
	if err != nil {
		return Artifact{}, err
	}
	contentType, err := normalizeContentType(input.Kind, input.ContentType)
	if err != nil {
		return Artifact{}, err
	}

	record, err := s.uploader.Upload(ctx, files.UploadInput{
		OriginalFilename: safeArtifactFilename(input.Filename, input.JobID, contentType),
		MimeType:         contentType,
		Size:             input.Size,
		Purpose:          purpose,
		ClientFileID:     safeClientFileID(input.JobID),
		Body:             input.Body,
	})
	if err != nil {
		return Artifact{}, fmt.Errorf("store job artifact: %w", err)
	}

	return Artifact{
		FileID:      record.ID,
		Purpose:     purpose,
		ContentType: record.MimeType,
		Size:        record.ByteSize,
	}, nil
}

func purposeForKind(kind Kind) (string, error) {
	switch kind {
	case KindImage:
		return "image", nil
	case KindAudio:
		return "audio", nil
	default:
		return "", ErrArtifactKindInvalid
	}
}

func normalizeContentType(kind Kind, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", ErrArtifactTypeInvalid
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", ErrArtifactTypeInvalid
	}
	mediaType = strings.ToLower(mediaType)
	switch kind {
	case KindImage:
		if strings.HasPrefix(mediaType, "image/") {
			return mediaType, nil
		}
	case KindAudio:
		if strings.HasPrefix(mediaType, "audio/") {
			return mediaType, nil
		}
	}
	return "", ErrArtifactTypeInvalid
}

func safeArtifactFilename(filename string, jobID string, contentType string) string {
	filename = strings.TrimSpace(filepath.Base(strings.ReplaceAll(filename, "\\", "/")))
	if filename != "" && filename != "." && filename != string(filepath.Separator) {
		return filename
	}
	jobID = safeClientFileID(jobID)
	if jobID == "" {
		jobID = "job-artifact"
	}
	return jobID + extensionForContentType(contentType)
}

func safeClientFileID(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	var builder strings.Builder
	for _, current := range jobID {
		if current >= 'a' && current <= 'z' ||
			current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' ||
			current == '-' || current == '_' || current == '.' {
			builder.WriteRune(current)
		}
	}
	return builder.String()
}

func extensionForContentType(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/webm":
		return ".webm"
	default:
		return ".bin"
	}
}
