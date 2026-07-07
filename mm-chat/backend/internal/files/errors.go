package files

import "errors"

var (
	ErrDatabaseRequired = errors.New("database is required")
	ErrStorageRequired  = errors.New("storage is required")
	ErrFileNotFound     = errors.New("file not found")
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

func newValidationError(code string, message string) ValidationError {
	return ValidationError{Code: code, Message: message}
}
