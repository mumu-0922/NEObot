package chat

import (
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	maxFusionWebKnowledgeContextBytes = 512
	maxFusionStageDurationMillis      = int64(10 * 60 * 1000)
)

type sourceFusionDiagnostics struct {
	KnowledgeDurationMillis  int64
	RouterDurationMillis     int64
	WebResolveOutcome        string
	WebResolveDurationMillis int64
	WebExecuteOutcome        string
	WebExecuteDurationMillis int64
	WebQueryDerived          bool
	DegradationReason        string
}

func newSourceFusionDiagnostics(plan sourceFusionPlan) sourceFusionDiagnostics {
	outcome := "pending"
	if !plan.SearchEnabled {
		outcome = "disabled"
	} else if !plan.SearchRequested {
		outcome = "skipped"
	}
	return sourceFusionDiagnostics{
		WebResolveOutcome: outcome,
		WebExecuteOutcome: outcome,
	}
}

func sourceFusionDurationMillis(started time.Time) int64 {
	duration := time.Since(started).Milliseconds()
	if duration < 0 {
		return 0
	}
	if duration > maxFusionStageDurationMillis {
		return maxFusionStageDurationMillis
	}
	return duration
}

func buildFusionWebSearchQuery(
	question string,
	plan sourceFusionPlan,
	knowledge autoRAGDecision,
) (string, bool) {
	question = boundedWebSearchQuery(strings.Join(strings.Fields(question), " "))
	if question == "" || !plan.SearchRequested ||
		plan.Authority != sourceAuthorityMixed || !knowledge.ReadyForAnswer() {
		return question, false
	}

	remaining := maxFusionWebKnowledgeContextBytes
	parts := make([]string, 0, min(2, len(knowledge.Citations)))
	for _, citation := range knowledge.Citations {
		if remaining <= 0 || len(parts) == 2 {
			break
		}
		snippet := strings.Join(strings.Fields(citation.Snippet), " ")
		snippet = truncateWebUTF8(snippet, remaining)
		if snippet == "" {
			continue
		}
		parts = append(parts, snippet)
		remaining -= len(snippet)
	}
	if len(parts) == 0 {
		return question, false
	}
	combined := question + "\nRelevant internal context: " + strings.Join(parts, " ")
	combined = boundedWebSearchQuery(combined)
	if combined == question {
		return question, false
	}
	return combined, true
}

func fallbackSourceFusionAuthority(
	plan sourceFusionPlan,
	knowledge autoRAGDecision,
) sourceFusionPlan {
	if knowledge.ReadyForAnswer() {
		plan.Authority = sourceAuthorityKnowledge
	} else {
		plan.Authority = sourceAuthorityModel
	}
	return plan
}

func sourceSearchDegradationReason(err error) string {
	var providerError *websearch.ProviderError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, websearch.ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, websearch.ErrResolutionFailed):
		return "resolution_failed"
	case errors.Is(err, websearch.ErrInvalidConfig):
		return "invalid_config"
	case errors.Is(err, websearch.ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, errModelBuiltInSearchUnsupported):
		return "model_builtin_unsupported"
	case errors.As(err, &providerError):
		return "provider_failed"
	default:
		return "unavailable"
	}
}
