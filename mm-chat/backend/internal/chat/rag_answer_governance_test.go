package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestKnowledgeConsentRAGAnswerGovernanceGateRequiresQueryAndCollectionAnswerConsent(t *testing.T) {
	reader := &fakeRAGConsentReader{
		query: []knowledge.ProcessingConsent{validAnswerConsent("mock", "mock-chat")},
		collections: map[string][]knowledge.ProcessingConsent{
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": {validAnswerConsent("mock", "mock-chat")},
		},
	}
	gate := NewKnowledgeConsentRAGAnswerGovernanceGate(reader)

	authority, err := gate.AuthorizeRAGAnswer(context.Background(), RAGAnswerGovernanceInput{
		ModelRef:              ModelRef{ProviderID: "mock", ModelID: "mock-chat"},
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		Citations:             []RAGCitation{{ID: "cit_1", Marker: "[1]"}},
	})

	if err != nil {
		t.Fatalf("AuthorizeRAGAnswer() error = %v", err)
	}
	if authority.Processor != "mock" || authority.ModelID != "mock-chat" || authority.CollectionCount != 1 {
		t.Fatalf("authority = %#v", authority)
	}
	if len(reader.collectionCalls) != 1 || reader.collectionCalls[0] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("collection calls = %#v", reader.collectionCalls)
	}
}

func TestKnowledgeConsentRAGAnswerGovernanceGateRejectsMissingConsent(t *testing.T) {
	tests := []struct {
		name   string
		reader *fakeRAGConsentReader
	}{
		{name: "missing query", reader: &fakeRAGConsentReader{
			collections: map[string][]knowledge.ProcessingConsent{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": {validAnswerConsent("mock", "mock-chat")}},
		}},
		{name: "missing collection", reader: &fakeRAGConsentReader{
			query:       []knowledge.ProcessingConsent{validAnswerConsent("mock", "mock-chat")},
			collections: map[string][]knowledge.ProcessingConsent{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": nil},
		}},
		{name: "wrong purpose", reader: &fakeRAGConsentReader{
			query: []knowledge.ProcessingConsent{func() knowledge.ProcessingConsent {
				c := validAnswerConsent("mock", "mock-chat")
				c.Purposes = []string{"query_embedding"}
				return c
			}()},
			collections: map[string][]knowledge.ProcessingConsent{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": {validAnswerConsent("mock", "mock-chat")}},
		}},
		{name: "wrong model", reader: &fakeRAGConsentReader{
			query:       []knowledge.ProcessingConsent{validAnswerConsent("mock", "other-model")},
			collections: map[string][]knowledge.ProcessingConsent{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": {validAnswerConsent("mock", "mock-chat")}},
		}},
		{name: "expired", reader: &fakeRAGConsentReader{
			query: []knowledge.ProcessingConsent{func() knowledge.ProcessingConsent {
				c := validAnswerConsent("mock", "mock-chat")
				c.EffectiveStatus = "expired"
				return c
			}()},
			collections: map[string][]knowledge.ProcessingConsent{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": {validAnswerConsent("mock", "mock-chat")}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := NewKnowledgeConsentRAGAnswerGovernanceGate(tt.reader)

			_, err := gate.AuthorizeRAGAnswer(context.Background(), RAGAnswerGovernanceInput{
				ModelRef:              ModelRef{ProviderID: "mock", ModelID: "mock-chat"},
				SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
				Citations:             []RAGCitation{{ID: "cit_1", Marker: "[1]"}},
			})

			if !errors.Is(err, ErrRAGAnswerGovernanceRequired) {
				t.Fatalf("error = %v, want ErrRAGAnswerGovernanceRequired", err)
			}
		})
	}
}

func TestKnowledgeConsentRAGAnswerGovernanceGateFailsClosedWhenDependencyMissing(t *testing.T) {
	_, err := (*KnowledgeConsentRAGAnswerGovernanceGate)(nil).AuthorizeRAGAnswer(context.Background(), RAGAnswerGovernanceInput{})
	if !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("error = %v, want ErrRAGDependencyUnavailable", err)
	}
}

type fakeRAGConsentReader struct {
	query           []knowledge.ProcessingConsent
	collections     map[string][]knowledge.ProcessingConsent
	queryErr        error
	collectionErr   error
	collectionCalls []string
}

func (f *fakeRAGConsentReader) ListQueryConsents(context.Context) ([]knowledge.ProcessingConsent, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return append([]knowledge.ProcessingConsent(nil), f.query...), nil
}

func (f *fakeRAGConsentReader) ListCollectionConsents(_ context.Context, collectionID string) ([]knowledge.ProcessingConsent, error) {
	f.collectionCalls = append(f.collectionCalls, collectionID)
	if f.collectionErr != nil {
		return nil, f.collectionErr
	}
	return append([]knowledge.ProcessingConsent(nil), f.collections[collectionID]...), nil
}

func validAnswerConsent(processor string, modelID string) knowledge.ProcessingConsent {
	return knowledge.ProcessingConsent{
		Processor:           strings.TrimSpace(processor),
		EndpointID:          "default",
		ModelID:             strings.TrimSpace(modelID),
		ProfileContractHash: strings.Repeat("a", 64),
		Decision:            "granted",
		EffectiveStatus:     "granted",
		PolicyVersion:       "v1",
		Purposes:            []string{"answer"},
		DataTypes:           []string{"text/plain"},
	}
}
