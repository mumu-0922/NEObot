package teams

import "errors"

const (
	ErrorCodeInvalidTeamPayload       = "INVALID_TEAM_PAYLOAD"
	ErrorCodeInvalidInvitePayload     = "INVALID_INVITE_PAYLOAD"
	ErrorCodeInvalidMembershipPayload = "INVALID_MEMBERSHIP_PAYLOAD"
	ErrorCodeForbiddenIdentityField   = "FORBIDDEN_IDENTITY_FIELD"
)

var (
	ErrDatabaseRequired          = errors.New("database is required")
	ErrCursorCodecRequired       = errors.New("cursor codec is required")
	ErrUnauthenticated           = errors.New("unauthenticated")
	ErrTeamNotFound              = errors.New("team not found")
	ErrTeamMemberNotFound        = errors.New("team member not found")
	ErrInviteNotFound            = errors.New("invite not found")
	ErrTeamAdminRequired         = errors.New("team admin required")
	ErrLastTeamAdmin             = errors.New("last team admin")
	ErrInviteConflict            = errors.New("invite conflict")
	ErrIdempotencyConflict       = errors.New("idempotency key conflict")
	ErrInviteNotActive           = errors.New("invite is not active")
	ErrInviteDeliveryUnavailable = errors.New("invite delivery is unavailable")
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

func invalidTeamPayload(message string) error {
	return newValidationError(ErrorCodeInvalidTeamPayload, message)
}

func invalidInvitePayload(message string) error {
	return newValidationError(ErrorCodeInvalidInvitePayload, message)
}

func invalidMembershipPayload(message string) error {
	return newValidationError(ErrorCodeInvalidMembershipPayload, message)
}

func forbiddenIdentityPayload() error {
	return newValidationError(
		ErrorCodeForbiddenIdentityField,
		"caller identity fields are not accepted",
	)
}
