package browserimport

import "errors"

var (
	ErrDatabaseRequired    = errors.New("database is required")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
	ErrBatchNotFound       = errors.New("import batch not found")
	ErrBatchModified       = errors.New("import batch modified")
)

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func newValidationError(code string, message string) ValidationError {
	return ValidationError{Code: code, Message: message}
}
