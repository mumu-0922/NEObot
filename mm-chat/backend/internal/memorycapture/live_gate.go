package memorycapture

import (
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	LiveApproval                = "I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA"
	LiveMemoryToolRouteApproval = "I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA"

	LiveAuthorizationDisabled                       = "MEMORY_REGRESSION_LIVE_DISABLED"
	LiveAuthorizationApproval                       = "MEMORY_REGRESSION_LIVE_APPROVAL_REQUIRED"
	LiveAuthorizationRunID                          = "MEMORY_REGRESSION_LIVE_RUN_ID_MISMATCH"
	LiveAuthorizationProviderTarget                 = "MEMORY_REGRESSION_LIVE_TARGET_DENIED"
	LiveAuthorizationCloudJudgeTarget               = "MEMORY_REGRESSION_LIVE_CLOUD_JUDGE_TARGET_DENIED"
	LiveAuthorizationMemoryToolRouteTarget          = "MEMORY_REGRESSION_LIVE_MEMORY_TOOL_ROUTE_TARGET_DENIED"
	LiveAuthorizationConfiguredCandidateJudgeTarget = "MEMORY_REGRESSION_LIVE_CONFIGURED_CANDIDATE_JUDGE_TARGET_DENIED"
)

var ErrLiveNotAuthorized = errors.New("native Memory live capture is not authorized")

type LiveAuthorization struct {
	Enabled                               bool
	Approval                              string
	RunID                                 string
	ProviderID                            string
	EmbeddingModelID                      string
	RerankModelID                         string
	CloudJudgeModelID                     string
	MemoryToolRouteApproval               string
	MemoryToolRouteProviderID             string
	MemoryToolRouteProviderType           string
	MemoryToolRouteBaseURLSHA256          string
	MemoryToolRouteModelID                string
	ConfiguredCandidateJudgeApproval      string
	ConfiguredCandidateJudgeProviderID    string
	ConfiguredCandidateJudgeProviderType  string
	ConfiguredCandidateJudgeBaseURLSHA256 string
	ConfiguredCandidateJudgeModelID       string
}

func AuthorizeConfiguredCandidateJudgeTarget(
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	authorization LiveAuthorization,
) error {
	if providerMode == ProviderModeFakeProtocol {
		return nil
	}
	if providerMode != ProviderModeLiveSiliconFlow ||
		strings.TrimSpace(authorization.ConfiguredCandidateJudgeApproval) !=
			LiveMemoryToolRouteApproval ||
		!validConfiguredCandidateJudgeProfileAuthority(authority) ||
		strings.TrimSpace(authorization.ConfiguredCandidateJudgeProviderID) != authority.ProviderID ||
		strings.TrimSpace(authorization.ConfiguredCandidateJudgeProviderType) != authority.ProviderType ||
		strings.TrimSpace(authorization.ConfiguredCandidateJudgeBaseURLSHA256) != authority.BaseURLSHA256 ||
		strings.TrimSpace(authorization.ConfiguredCandidateJudgeModelID) != authority.ModelID {
		return LiveAuthorizationError{Code: LiveAuthorizationConfiguredCandidateJudgeTarget}
	}
	return nil
}

func AuthorizeMemoryToolRouteTarget(
	providerMode string,
	authority MemoryToolRouteProfileAuthority,
	authorization LiveAuthorization,
) error {
	if providerMode == ProviderModeFakeProtocol {
		return nil
	}
	if providerMode != ProviderModeLiveSiliconFlow ||
		strings.TrimSpace(authorization.MemoryToolRouteApproval) !=
			LiveMemoryToolRouteApproval ||
		strings.TrimSpace(authority.ProviderID) == "" ||
		strings.TrimSpace(authority.ProviderType) == "" ||
		len(authority.BaseURLSHA256) != 64 ||
		strings.TrimSpace(authority.ModelID) == "" ||
		strings.TrimSpace(authorization.MemoryToolRouteProviderID) != authority.ProviderID ||
		strings.TrimSpace(authorization.MemoryToolRouteProviderType) != authority.ProviderType ||
		strings.TrimSpace(authorization.MemoryToolRouteBaseURLSHA256) != authority.BaseURLSHA256 ||
		strings.TrimSpace(authorization.MemoryToolRouteModelID) != authority.ModelID {
		return LiveAuthorizationError{Code: LiveAuthorizationMemoryToolRouteTarget}
	}
	return nil
}

func AuthorizeCloudJudgeTarget(
	providerMode string,
	modelID string,
	authorization LiveAuthorization,
) error {
	if providerMode == ProviderModeFakeProtocol {
		return nil
	}
	if providerMode != ProviderModeLiveSiliconFlow ||
		strings.TrimSpace(modelID) == "" ||
		strings.TrimSpace(authorization.CloudJudgeModelID) != modelID {
		return LiveAuthorizationError{Code: LiveAuthorizationCloudJudgeTarget}
	}
	return nil
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
