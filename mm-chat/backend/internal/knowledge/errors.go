package knowledge

import "errors"

const (
	ErrorCodeInvalidCollectionPayload = "INVALID_COLLECTION_PAYLOAD"
	ErrorCodeInvalidDocumentPayload   = "INVALID_DOCUMENT_PAYLOAD"
	ErrorCodeForbiddenIdentityField   = "FORBIDDEN_IDENTITY_FIELD"
)

var (
	ErrDatabaseRequired              = errors.New("database is required")
	ErrCursorCodecRequired           = errors.New("cursor codec is required")
	ErrUnauthenticated               = errors.New("unauthenticated")
	ErrCollectionNotFound            = errors.New("collection not found")
	ErrDocumentNotFound              = errors.New("document not found")
	ErrStorageRequired               = errors.New("storage is required")
	ErrTeamAdminRequired             = errors.New("team admin required")
	ErrIdempotencyConflict           = errors.New("idempotency key conflict")
	ErrFileNotFound                  = errors.New("file not found")
	ErrProcessingConsent             = errors.New("processing consent required")
	ErrDocumentProcessing            = errors.New("document processing")
	ErrKnowledgeProcessorUnavailable = errors.New("knowledge processor unavailable")
)

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func invalidCollectionPayload(message string) error {
	return ValidationError{Code: ErrorCodeInvalidCollectionPayload, Message: message}
}

func forbiddenIdentityPayload() error {
	return ValidationError{
		Code:    ErrorCodeForbiddenIdentityField,
		Message: "caller identity and authorization fields are not accepted",
	}
}
