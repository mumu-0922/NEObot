package memorycapture

import (
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	LiveApproval = "I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA"

	LiveAuthorizationDisabled       = "MEMORY_REGRESSION_LIVE_DISABLED"
	LiveAuthorizationApproval       = "MEMORY_REGRESSION_LIVE_APPROVAL_REQUIRED"
	LiveAuthorizationRunID          = "MEMORY_REGRESSION_LIVE_RUN_ID_MISMATCH"
	LiveAuthorizationProviderTarget = "MEMORY_REGRESSION_LIVE_TARGET_DENIED"
)

var ErrLiveNotAuthorized = errors.New("native Memory live capture is not authorized")

type LiveAuthorization struct {
	Enabled          bool
	Approval         string
	RunID            string
	ProviderID       string
	EmbeddingModelID string
	RerankModelID    string
}

type LiveAuthorizationError struct{ Code string }

func (e LiveAuthorizationError) Error() string {
	return fmt.Sprintf("%s: %s", ErrLiveNotAuthorized, e.Code)
}

func (e LiveAuthorizationError) Unwrap() error { return ErrLiveNotAuthorized }

// AuthorizeProviderMode keeps the deterministic fake protocol offline and
// requires exact, run-bound SiliconFlow approval before any live Provider is
// constructed. It deliberately does not accept a credential value.
func AuthorizeProviderMode(providerMode, runID string, authorization LiveAuthorization) error {
	switch providerMode {
	case ProviderModeFakeProtocol:
		return nil
	case ProviderModeLiveSiliconFlow:
	default:
		return fmt.Errorf("%w: provider mode", ErrCaptureInvalid)
	}
	if !authorization.Enabled {
		return LiveAuthorizationError{Code: LiveAuthorizationDisabled}
	}
	if strings.TrimSpace(authorization.Approval) != LiveApproval {
		return LiveAuthorizationError{Code: LiveAuthorizationApproval}
	}
	if !runIDPattern.MatchString(runID) || authorization.RunID != runID {
		return LiveAuthorizationError{Code: LiveAuthorizationRunID}
	}
	if strings.TrimSpace(authorization.ProviderID) != "siliconflow" ||
		strings.TrimSpace(authorization.EmbeddingModelID) != ragproviders.SiliconFlowEmbeddingModel ||
		strings.TrimSpace(authorization.RerankModelID) != ragproviders.SiliconFlowRerankModel {
		return LiveAuthorizationError{Code: LiveAuthorizationProviderTarget}
	}
	return nil
}
