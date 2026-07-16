package ragsource

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	InternalSourceObjectPath = "/internal/rag/source-object"
	InternalTokenHeader      = "X-MM-Chat-Internal-Token"
	DefaultMaxSourceBytes    = int64(512 << 20)
)

var (
	ErrServiceUnavailable = errors.New("rag source object service unavailable")
	ErrUnauthorized       = errors.New("rag source object unauthorized")
	ErrInvalidRequest     = errors.New("invalid rag source object request")
	ErrSourceUnavailable  = errors.New("rag source object unavailable")
	ErrSourceMismatch     = errors.New("rag source object mismatch")
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

type SourceObjectRequest struct {
	JobID             string `json:"jobId"`
	WorkerID          string `json:"workerId"`
	LeaseToken        string `json:"leaseToken"`
	FileID            string `json:"fileId"`
	MaterializationID string `json:"materializationId"`
}

type SourceObjectInput struct {
	JobID             string
	WorkerID          string
	LeaseToken        string
	FileID            string
	MaterializationID string
}

type SourceMetadata struct {
	FileID         string
	StorageBackend string
	ObjectKey      string
	SHA256         string
	ByteSize       int64
	ContentType    string
}

type SourceObject struct {
	FileID      string
	Body        []byte
	SHA256      string
	ByteSize    int64
	ContentType string
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newError(code string, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func normalizeInput(input SourceObjectInput) (SourceObjectInput, error) {
	input.JobID = strings.TrimSpace(input.JobID)
	input.WorkerID = strings.TrimSpace(input.WorkerID)
	input.LeaseToken = strings.TrimSpace(input.LeaseToken)
	input.FileID = strings.TrimSpace(input.FileID)
	input.MaterializationID = strings.TrimSpace(input.MaterializationID)
	for name, value := range map[string]string{
		"jobId":             input.JobID,
		"workerId":          input.WorkerID,
		"leaseToken":        input.LeaseToken,
		"fileId":            input.FileID,
		"materializationId": input.MaterializationID,
	} {
		if !uuidRE.MatchString(value) {
			return SourceObjectInput{}, fmt.Errorf("%s must be a UUID", name)
		}
	}
	return input, nil
}

func normalizeMetadata(metadata SourceMetadata) (SourceMetadata, error) {
	metadata.FileID = strings.TrimSpace(metadata.FileID)
	metadata.StorageBackend = strings.ToLower(strings.TrimSpace(metadata.StorageBackend))
	objectKey := strings.TrimSpace(metadata.ObjectKey)
	sha256Value := strings.TrimSpace(metadata.SHA256)
	contentType := strings.ToLower(strings.TrimSpace(metadata.ContentType))
	if !uuidRE.MatchString(metadata.FileID) {
		return SourceMetadata{}, ErrSourceMismatch
	}
	if metadata.StorageBackend != "local" && metadata.StorageBackend != "minio" && metadata.StorageBackend != "s3" {
		return SourceMetadata{}, ErrSourceMismatch
	}
	if objectKey == "" ||
		objectKey != metadata.ObjectKey ||
		strings.HasPrefix(objectKey, "/") ||
		strings.HasSuffix(objectKey, "/") ||
		strings.Contains(objectKey, "\\") ||
		strings.Contains(objectKey, ":") ||
		hasUnsafeObjectKeySegment(objectKey) {
		return SourceMetadata{}, ErrSourceMismatch
	}
	if sha256Value != metadata.SHA256 || !sha256RE.MatchString(sha256Value) {
		return SourceMetadata{}, ErrSourceMismatch
	}
	if metadata.ByteSize < 1 || metadata.ByteSize > DefaultMaxSourceBytes {
		return SourceMetadata{}, ErrSourceMismatch
	}
	if contentType != metadata.ContentType ||
		contentType == "" ||
		strings.ContainsAny(contentType, " \t\r\n") ||
		!strings.Contains(contentType, "/") {
		return SourceMetadata{}, ErrSourceMismatch
	}
	metadata.ObjectKey = objectKey
	metadata.SHA256 = sha256Value
	metadata.ContentType = contentType
	return metadata, nil
}

func validateMetadata(metadata SourceMetadata) error {
	_, err := normalizeMetadata(metadata)
	return err
}

func hasUnsafeObjectKeySegment(objectKey string) bool {
	for _, segment := range strings.Split(objectKey, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return true
		}
	}
	return false
}
