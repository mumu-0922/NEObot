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
	question := "这个研究方向的最新公开进展"
	plan := planSourceFusion(question, true, knowledgeDecision)
	query, derived := buildFusionWebSearchQuery(question, plan, knowledgeDecision)
	if !derived || !strings.Contains(query, "Relevant internal context") ||
		len(query) > websearch.MaxQueryBytes {
		t.Fatalf("derived query = %q (%d bytes)", query, len(query))
	}
	if strings.Contains(query, knowledgeDecision.Citations[0].SourceSpanHash) ||
		strings.Contains(query, knowledgeDecision.Citations[0].ContentHash) {
		t.Fatalf("derived query leaked source identity: %q", query)
	}
}

func TestBuildFusionWebSearchQueryDoesNotPolluteExplicitSubject(t *testing.T) {
	evidence := validHydratedEvidence()
	knowledgeDecision := autoRAGDecision{
		Outcome:  "evidence_ready",
		Evidence: []knowledge.HydratedEvidence{evidence},
		Citations: []RAGCitation{{
			ID: "cit_1", Marker: "[K1]", Snippet: "推荐系统 HSTU HLLM 生成式推荐",
		}},
		Authority: &RAGAnswerAuthority{Processor: "fixture"},
	}
	question := "Kimi最新模型是啥"
	plan := planSourceFusion(question, true, knowledgeDecision)

	query, derived := buildFusionWebSearchQuery(question, plan, knowledgeDecision)

	if derived || query != question || strings.Contains(query, "推荐系统") {
		t.Fatalf("explicit-subject query = %q, derived=%v", query, derived)
	}
}

func TestReconcileCompletedSourceFusionAuthorityUsesActualMarkers(t *testing.T) {
	knowledge := autoRAGDecision{
		Outcome: "answered",
		Citations: []RAGCitation{
			{ID: "cit_1", Marker: "[K1]"},
		},
	}
	webResult := websearch.Result{Sources: []websearch.Source{{
		Title: "Public source", URL: "https://example.test/public", Content: "public evidence",
	}}}
	tests := []struct {
		name      string
		content   string
		knowledge autoRAGDecision
		want      sourceAuthority
	}{
		{name: "both", content: "private [K1] public [W1]", knowledge: knowledge, want: sourceAuthorityMixed},
		{name: "knowledge only", content: "private [K1]", knowledge: knowledge, want: sourceAuthorityKnowledge},
		{name: "web only", content: "public [W1]", knowledge: autoRAGDecision{}, want: sourceAuthorityWeb},
		{name: "neither", content: "model synthesis", knowledge: autoRAGDecision{}, want: sourceAuthorityModel},
		{name: "invented markers", content: "invented [K9] [W9]", knowledge: knowledge, want: sourceAuthorityModel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := reconcileCompletedSourceFusionAuthority(
				sourceFusionPlan{Authority: sourceAuthorityMixed},
				tt.content,
				tt.knowledge,
				webResult,
			)
			if plan.Authority != tt.want {
				t.Fatalf("authority = %q, want %q", plan.Authority, tt.want)
			}
		})
	}
}

func TestReconcileProviderSourceMarkersKeepsOnlyCurrentTurnAuthority(t *testing.T) {
	knowledge := autoRAGDecision{Citations: []RAGCitation{{Marker: "[K1]"}}}
	webResult := websearch.Result{Sources: []websearch.Source{{
		Title: "Public source", URL: "https://example.test/public", Content: "public evidence",
	}}}

	got := reconcileProviderSourceMarkers(
		"grounded [K1] public [W1] invented [K2] [W9]",
		knowledge,
		webResult,
	)
	if got != "grounded [K1] public [W1] invented" {
		t.Fatalf("reconciled content = %q", got)
	}
	if got := reconcileProviderSourceMarkers("model fallback [K1]", autoRAGDecision{}, websearch.Result{}); got != "model fallback" {
		t.Fatalf("no-evidence content = %q", got)
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
