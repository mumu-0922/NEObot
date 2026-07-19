package chat

import (
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestPlanSourceFusionMatrix(t *testing.T) {
	ready := autoRAGDecision{
		Outcome:   "evidence_ready",
		Evidence:  []knowledge.HydratedEvidence{validHydratedEvidence()},
		Citations: []RAGCitation{{ID: "cit_1", Marker: "[K1]"}},
		Authority: &RAGAnswerAuthority{Processor: "fixture"},
	}
	tests := []struct {
		name             string
		question         string
		searchEnabled    bool
		knowledge        autoRAGDecision
		wantClass        sourceQuestionClass
		wantAuthority    sourceAuthority
		wantSearch       bool
		wantSearchReason sourceSearchReason
	}{
		{
			name:     "disabled with knowledge",
			question: "What does the internal document say?", searchEnabled: false,
			knowledge: ready, wantClass: sourceQuestionKnowledge,
			wantAuthority: sourceAuthorityKnowledge, wantSearchReason: sourceSearchDisabled,
		},
		{
			name:     "knowledge is sufficient",
			question: "研究方向是什么", searchEnabled: true,
			knowledge: ready, wantClass: sourceQuestionKnowledge,
			wantAuthority:    sourceAuthorityKnowledge,
			wantSearchReason: sourceSearchKnowledgeSufficient,
		},
		{
			name:     "english marker does not match inside a word",
			question: "Explain this unknown internal behavior", searchEnabled: true,
			knowledge: ready, wantClass: sourceQuestionKnowledge,
			wantAuthority:    sourceAuthorityKnowledge,
			wantSearchReason: sourceSearchKnowledgeSufficient,
		},
		{
			name:     "current question combines sources",
			question: "这个研究方向的最新公开进展是什么", searchEnabled: true,
			knowledge: ready, wantClass: sourceQuestionCurrentPublic,
			wantAuthority: sourceAuthorityMixed, wantSearch: true,
			wantSearchReason: sourceSearchCurrentPublic,
		},
		{
			name:     "knowledge miss uses web",
			question: "Explain retrieval augmented generation", searchEnabled: true,
			knowledge: autoRAGDecision{Outcome: "no_evidence"},
			wantClass: sourceQuestionGeneral, wantAuthority: sourceAuthorityWeb,
			wantSearch: true, wantSearchReason: sourceSearchKnowledgeUnavailable,
		},
		{
			name:     "neither source uses model",
			question: "Write a greeting", searchEnabled: false,
			wantClass: sourceQuestionGeneral, wantAuthority: sourceAuthorityModel,
			wantSearchReason: sourceSearchDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planSourceFusion(tt.question, tt.searchEnabled, tt.knowledge)
			if got.QuestionClass != tt.wantClass || got.Authority != tt.wantAuthority ||
				got.SearchRequested != tt.wantSearch ||
				got.SearchReason != tt.wantSearchReason {
				t.Fatalf("planSourceFusion() = %#v", got)
			}
		})
	}
}

func TestSourceFusionMetadataContainsOnlyBoundedDecisionFields(t *testing.T) {
	metadata := withSourceFusionMessageMetadata(
		map[string]any{"runId": "run"},
		planSourceFusion("latest private fixture", true, autoRAGDecision{Outcome: "no_evidence"}),
		autoRAGDecision{Outcome: "no_evidence"},
	)
	fusion, ok := metadata["fusion"].(map[string]any)
	if !ok || len(fusion) != 7 || fusion["version"] != sourceFusionVersion ||
		fusion["authority"] != sourceAuthorityWeb ||
		fusion["searchRequested"] != true || fusion["knowledgeOutcome"] != "no_evidence" {
		t.Fatalf("fusion metadata = %#v", metadata["fusion"])
	}
	for _, forbidden := range []string{"question", "query", "evidence", "sources", "secret"} {
		if _, present := fusion[forbidden]; present {
			t.Fatalf("fusion metadata contains %q: %#v", forbidden, fusion)
		}
	}
}
