package httpserver

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

type fakeRAGCandidateFetcher struct {
	keyword      []knowledge.EvidenceCandidateReference
	hybrid       []knowledge.EvidenceCandidateReference
	hybridErr    error
	keywordCalls int
	hybridCalls  int
}

func (fetcher *fakeRAGCandidateFetcher) FetchQueryEvidenceCandidates(
	_ context.Context,
	_ knowledge.QueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	fetcher.keywordCalls++
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.keyword...), nil
}

func (fetcher *fakeRAGCandidateFetcher) FetchHybridQueryEvidenceCandidates(
	_ context.Context,
	_ knowledge.HybridQueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	fetcher.hybridCalls++
	if fetcher.hybridErr != nil {
		return nil, fetcher.hybridErr
	}
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.hybrid...), nil
}

type fakeRAGQueryEmbedder struct {
	err error
}

func (embedder fakeRAGQueryEmbedder) EmbedQuery(
	_ context.Context,
	_ string,
) (ragproviders.QueryEmbedding, error) {
	if embedder.err != nil {
		return ragproviders.QueryEmbedding{}, embedder.err
	}
	return ragproviders.QueryEmbedding{
		ModelID: "jina-embeddings-v4", Dimensions: 1024,
		Vector: repeatedRAGQueryVector(0.001),
	}, nil
}

func TestKnowledgeRAGCandidateSourceUsesHybridCandidates(t *testing.T) {
	reference := knowledge.EvidenceCandidateReference{ChildChunkID: "hybrid"}
	fetcher := &fakeRAGCandidateFetcher{hybrid: []knowledge.EvidenceCandidateReference{reference}}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   fakeRAGQueryEmbedder{},
	}

	candidates, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if err != nil {
		t.Fatalf("FetchEvidenceCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ChildChunkID != "hybrid" ||
		fetcher.hybridCalls != 1 || fetcher.keywordCalls != 0 {
		t.Fatalf("candidates/calls = %#v/%d/%d", candidates, fetcher.hybridCalls, fetcher.keywordCalls)
	}
}

func TestKnowledgeRAGCandidateSourceDegradesToKeywordWhenJinaFails(t *testing.T) {
	reference := knowledge.EvidenceCandidateReference{ChildChunkID: "keyword"}
	fetcher := &fakeRAGCandidateFetcher{keyword: []knowledge.EvidenceCandidateReference{reference}}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder: fakeRAGQueryEmbedder{
			err: ragproviders.ErrQueryEmbeddingUnavailable,
		},
	}

	candidates, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if err != nil {
		t.Fatalf("FetchEvidenceCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ChildChunkID != "keyword" ||
		fetcher.hybridCalls != 0 || fetcher.keywordCalls != 1 {
		t.Fatalf("candidates/calls = %#v/%d/%d", candidates, fetcher.hybridCalls, fetcher.keywordCalls)
	}
}

func TestKnowledgeRAGCandidateSourceDoesNotHideHybridDatabaseFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	fetcher := &fakeRAGCandidateFetcher{hybridErr: databaseErr}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   fakeRAGQueryEmbedder{},
	}

	_, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if !errors.Is(err, databaseErr) || fetcher.keywordCalls != 0 {
		t.Fatalf("error/keyword calls = %v/%d", err, fetcher.keywordCalls)
	}
}

func validRAGCandidateQuery() chat.RAGCandidateQuery {
	return chat.RAGCandidateQuery{
		CollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		QueryText:     "semantic question",
		Limit:         20,
	}
}

func repeatedRAGQueryVector(value float32) []float32 {
	vector := make([]float32, 1024)
	for index := range vector {
		vector[index] = value
	}
	return vector
}
