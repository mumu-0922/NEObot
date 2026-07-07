package chat

import "errors"

var (
	ErrDatabaseRequired     = errors.New("database is required")
	ErrProviderRequired     = errors.New("provider is required")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrIdempotencyConflict  = errors.New("idempotency key conflict")
	ErrRunNotFound          = errors.New("run not found")
	ErrRunNotCancellable    = errors.New("run is not cancellable")
)

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func newValidationError(code string, message string) error {
	return ValidationError{Code: code, Message: message}
}
