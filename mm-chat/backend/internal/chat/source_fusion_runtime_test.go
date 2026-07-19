package chat

import (
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestBuildFusionWebSearchQueryIsBoundedAndSourceMinimal(t *testing.T) {
	evidence := validHydratedEvidence()
	knowledgeDecision := autoRAGDecision{
		Outcome:  "evidence_ready",
		Evidence: []knowledge.HydratedEvidence{evidence},
		Citations: []RAGCitation{{
			ID: "cit_1", Marker: "[K1]", Snippet: strings.Repeat("知识 ", 500),
			SourceSpanHash: strings.Repeat("a", 64),
			ContentHash:    strings.Repeat("b", 64),
		}},
		Authority: &RAGAnswerAuthority{Processor: "fixture"},
	}
	plan := planSourceFusion("最新公开进展", true, knowledgeDecision)
	query, derived := buildFusionWebSearchQuery("最新公开进展", plan, knowledgeDecision)
	if !derived || !strings.Contains(query, "Relevant internal context") ||
		len(query) > websearch.MaxQueryBytes {
		t.Fatalf("derived query = %q (%d bytes)", query, len(query))
	}
	if strings.Contains(query, knowledgeDecision.Citations[0].SourceSpanHash) ||
		strings.Contains(query, knowledgeDecision.Citations[0].ContentHash) {
		t.Fatalf("derived query leaked source identity: %q", query)
	}
}

func TestSourceSearchDegradationReasonIsStableAndRedacted(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: websearch.ErrNotConfigured, want: "not_configured"},
		{err: websearch.ErrResolutionFailed, want: "resolution_failed"},
		{err: errModelBuiltInSearchUnsupported, want: "model_builtin_unsupported"},
		{err: &websearch.ProviderError{
			Provider: websearch.ProviderTavily, Code: "secret-upstream-detail",
		}, want: "provider_failed"},
		{err: errors.New("credential-shaped-private-detail"), want: "unavailable"},
	}
	for _, tt := range tests {
		if got := sourceSearchDegradationReason(tt.err); got != tt.want {
			t.Fatalf("degradation reason = %q, want %q", got, tt.want)
		}
	}
}

func TestSourceFusionDurationMillisClampsInvalidAndOversizedValues(t *testing.T) {
	if got := sourceFusionDurationMillis(time.Now().Add(time.Second)); got != 0 {
		t.Fatalf("future duration = %d", got)
	}
	if got := sourceFusionDurationMillis(time.Now().Add(-time.Hour)); got != maxFusionStageDurationMillis {
		t.Fatalf("oversized duration = %d", got)
	}
}
