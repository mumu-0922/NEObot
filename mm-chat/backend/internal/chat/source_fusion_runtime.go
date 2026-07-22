package chat

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	maxFusionWebKnowledgeContextBytes = 512
	maxFusionStageDurationMillis      = int64(10 * 60 * 1000)
)

var reservedSourceMarkerPattern = regexp.MustCompile(
	`[\t ]*\[(?:K|W)[0-9]+\]`,
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
		plan.Authority != sourceAuthorityMixed || !knowledge.ReadyForAnswer() ||
		!shouldRewriteRAGQuery(question) {
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

// reconcileProviderSourceMarkers treats Knowledge/Web markers as minted,
// turn-scoped capabilities. A model may see markers in conversation history
// and copy them into a later answer, but an unissued marker must never survive
// as a citation-looking claim.
func reconcileProviderSourceMarkers(
	content string,
	knowledge autoRAGDecision,
	webResult websearch.Result,
) string {
	allowed := make(map[string]struct{}, len(knowledge.Citations)+len(webResult.Sources))
	for _, citation := range knowledge.Citations {
		if marker := strings.TrimSpace(citation.Marker); marker != "" {
			allowed[marker] = struct{}{}
		}
	}
	_, webCitations := prepareWebSearchResult(webResult)
	for _, citation := range webCitations {
		if marker := strings.TrimSpace(citation.Marker); marker != "" {
			allowed[marker] = struct{}{}
		}
	}
	return filterReservedSourceMarkers(content, allowed)
}

func stripReservedSourceMarkers(content string) string {
	return filterReservedSourceMarkers(content, nil)
}

func filterReservedSourceMarkers(
	content string,
	allowed map[string]struct{},
) string {
	return reservedSourceMarkerPattern.ReplaceAllStringFunc(content, func(match string) string {
		marker := strings.TrimSpace(match)
		if _, ok := allowed[marker]; ok {
			return match
		}
		return ""
	})
}

func reconcileCompletedSourceFusionAuthority(
	plan sourceFusionPlan,
	content string,
	knowledge autoRAGDecision,
	webResult websearch.Result,
) sourceFusionPlan {
	knowledgeUsed := false
	for _, citation := range knowledge.Citations {
		if strings.Contains(content, citation.Marker) {
			knowledgeUsed = true
			break
		}
	}
	_, webCitations := prepareWebSearchResult(webResult)
	webUsed := false
	for _, citation := range webCitations {
		if strings.Contains(content, citation.Marker) {
			webUsed = true
			break
		}
	}

	switch {
	case knowledgeUsed && webUsed:
		plan.Authority = sourceAuthorityMixed
	case knowledgeUsed:
		plan.Authority = sourceAuthorityKnowledge
	case webUsed:
		plan.Authority = sourceAuthorityWeb
	default:
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
