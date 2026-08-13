package skillsupply

import "errors"

var (
	ErrUnavailable          = errors.New("skill supply chain unavailable")
	ErrAdministratorNeeded  = errors.New("skill administrator access is required")
	ErrInvalidSource        = errors.New("skill source is invalid")
	ErrSourceUnavailable    = errors.New("skill source is unavailable")
	ErrSourceDrift          = errors.New("immutable skill source drifted")
	ErrArchiveInvalid       = errors.New("skill archive is invalid")
	ErrManifestInvalid      = errors.New("skill manifest is invalid")
	ErrCandidateNotFound    = errors.New("skill candidate not found")
	ErrAdmissionDenied      = errors.New("skill candidate is not admitted")
	ErrAdmissionIneligible  = errors.New("skill candidate is not eligible for admission")
	ErrPackageChanged       = errors.New("skill package changed")
	ErrRevisionConflict     = errors.New("skill revision changed")
	ErrInstallationConflict = errors.New("skill is already installed")
	ErrInstallationNotFound = errors.New("skill installation not found")
	ErrPackageCollision     = errors.New("skill package fingerprint collision")
)

type ValidationError struct {
	Code    string
	Message string
}

func (err ValidationError) Error() string { return err.Message }

func validationError(code, message string) error {
	return ValidationError{Code: code, Message: message}
}
