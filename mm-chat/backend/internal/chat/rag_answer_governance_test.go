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
		Citations:             []RAGCitation{{ID: "cit_1", Marker: "[K1]"}},
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
				Citations:             []RAGCitation{{ID: "cit_1", Marker: "[K1]"}},
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

func TestKnowledgeConsentRoutingCatalogGovernanceGateRequiresEveryCollection(t *testing.T) {
	first := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	second := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	reader := &fakeRAGConsentReader{
		query: []knowledge.ProcessingConsent{validAnswerConsent("mock", "mock-chat")},
		collections: map[string][]knowledge.ProcessingConsent{
			first:  {validAnswerConsent("mock", "mock-chat")},
			second: {validAnswerConsent("mock", "mock-chat")},
		},
	}
	gate := NewKnowledgeConsentRoutingCatalogGovernanceGate(reader)
	input := RAGRoutingCatalogGovernanceInput{
		ModelRef:              ModelRef{ProviderID: "mock", ModelID: "mock-chat"},
		SelectedCollectionIDs: []string{first, second},
	}
	if err := gate.AuthorizeRoutingCatalog(context.Background(), input); err != nil {
		t.Fatalf("AuthorizeRoutingCatalog() error = %v", err)
	}
	if len(reader.collectionCalls) != 2 {
		t.Fatalf("collection calls = %#v", reader.collectionCalls)
	}

	reader.collections[second] = nil
	if err := gate.AuthorizeRoutingCatalog(context.Background(), input); !errors.Is(err, ErrRAGRoutingCatalogGovernanceRequired) {
		t.Fatalf("missing collection consent error = %v", err)
	}
	if err := (*KnowledgeConsentRoutingCatalogGovernanceGate)(nil).
		AuthorizeRoutingCatalog(context.Background(), input); !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("nil dependency error = %v", err)
	}
}

func TestKnowledgeConsentRAGRerankGovernanceGateRequiresExactRerankConsent(t *testing.T) {
	collectionID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	reader := &fakeRAGConsentReader{
		query: []knowledge.ProcessingConsent{validRerankConsent()},
		collections: map[string][]knowledge.ProcessingConsent{
			collectionID: {validRerankConsent()},
		},
	}
	gate := NewKnowledgeConsentRAGRerankGovernanceGate(reader)
	if err := gate.AuthorizeRAGRerank(context.Background(), []string{collectionID}); err != nil {
		t.Fatalf("AuthorizeRAGRerank() error = %v", err)
	}

	reader.query[0].Purposes = []string{"answer"}
	if err := gate.AuthorizeRAGRerank(context.Background(), []string{collectionID}); !errors.Is(err, ErrRAGRerankGovernanceRequired) {
		t.Fatalf("wrong-purpose error = %v", err)
	}
}

func TestKnowledgeConsentRAGRerankGovernanceGateFailsClosedWithoutDependency(t *testing.T) {
	err := (*KnowledgeConsentRAGRerankGovernanceGate)(nil).AuthorizeRAGRerank(
		context.Background(),
		[]string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
	)
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

func validRerankConsent() knowledge.ProcessingConsent {
	identity := knowledge.SingleUserRerankIdentity()
	consent := validAnswerConsent(identity.Processor, identity.ModelID)
	consent.EndpointID = identity.EndpointID
	consent.Purposes = []string{"rerank"}
	return consent
}
