package chat

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestRAGAnswerAssemblerReturnsDependencyUnavailableWhenMissingSeams(t *testing.T) {
	_, err := (*RAGAnswerAssembler)(nil).Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("error = %v, want ErrRAGDependencyUnavailable", err)
	}

	_, err = (&RAGAnswerAssembler{}).Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("error = %v, want ErrRAGDependencyUnavailable", err)
	}
}

func TestRAGAnswerAssemblerReturnsInsufficientEvidenceWithoutCandidates(t *testing.T) {
	hydrator := &fakeRAGHydrator{}
	assembler := NewRAGAnswerAssembler(&fakeRAGCandidateSource{}, hydrator)

	_, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())

	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
	if hydrator.calls != 0 {
		t.Fatalf("hydrator calls = %d, want 0", hydrator.calls)
	}
}

func TestRAGAnswerAssemblerMapsHydrationRejectionToInsufficientEvidence(t *testing.T) {
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}},
		&fakeRAGHydrator{err: knowledge.ErrEvidenceHydrationRejected},
	)

	_, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())

	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

func TestRAGAnswerAssemblerReturnsHydratedEvidence(t *testing.T) {
	source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
	hydrator := &fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}}
	assembler := NewRAGAnswerAssembler(source, hydrator)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())

	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].SourceText != "alpha evidence source" {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
	if len(result.Citations) != 1 || result.Citations[0].Marker != "[K1]" || result.Citations[0].Snippet != "alpha evidence source" {
		t.Fatalf("citations = %#v", result.Citations)
	}
	if len(source.queries) != 1 || source.queries[0].QueryText != "What does alpha say?" || source.queries[0].Limit != defaultRAGCandidateLimit {
		t.Fatalf("candidate queries = %#v", source.queries)
	}
	if hydrator.input.ActorUserID != DevUserID || hydrator.input.SessionID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
		t.Fatalf("hydration input = %#v", hydrator.input)
	}
}

func TestRAGAnswerAssemblerSearchesOriginalAndRewrittenQueries(t *testing.T) {
	source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
	hydrator := &fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}}
	assembler := NewRAGAnswerAssembler(source, hydrator)
	input := validRAGAssemblyInput()
	input.RewrittenQueryText = "What research direction does the document describe?"

	if _, err := assembler.Assemble(context.Background(), input); err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(source.queries) != 2 || source.queries[0].QueryText != "What does alpha say?" || source.queries[1].QueryText != input.RewrittenQueryText {
		t.Fatalf("candidate queries = %#v", source.queries)
	}
	if len(hydrator.input.References) != 1 {
		t.Fatalf("fused references = %#v, want one deduplicated reference", hydrator.input.References)
	}
}

func TestFuseRAGCandidateLanesUsesGlobalRRFAndDeterministicTopK(t *testing.T) {
	alpha := validRAGCandidate()
	beta := alpha
	beta.ChildChunkID = "22222222-2222-4222-8222-222222222223"
	beta.ContentHash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	gamma := alpha
	gamma.ChildChunkID = "33333333-3333-4333-8333-333333333334"
	gamma.ContentHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	fused := fuseRAGCandidateLanes([][]knowledge.EvidenceCandidateReference{
		{alpha, beta},
		{beta, gamma},
	}, 2)
	if len(fused) != 2 || fused[0].ChildChunkID != beta.ChildChunkID || fused[1].ChildChunkID != alpha.ChildChunkID {
		t.Fatalf("fused candidates = %#v", fused)
	}
	if fused[0].RankScore <= fused[1].RankScore {
		t.Fatalf("RRF scores = %v <= %v", fused[0].RankScore, fused[1].RankScore)
	}
}

func TestRAGAnswerAssemblerRejectsIncompleteInput(t *testing.T) {
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}},
		&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
	)
	input := validRAGAssemblyInput()
	input.QueryText = "   "

	_, err := assembler.Assemble(context.Background(), input)

	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

type fakeRAGCandidateSource struct {
	refs    []knowledge.EvidenceCandidateReference
	err     error
	calls   int
	queries []RAGCandidateQuery
}

func (f *fakeRAGCandidateSource) FetchEvidenceCandidates(
	_ context.Context,
	query RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	f.calls++
	f.queries = append(f.queries, query)
	if f.err != nil {
		return nil, f.err
	}
	return append([]knowledge.EvidenceCandidateReference(nil), f.refs...), nil
}

type fakeRAGHydrator struct {
	evidence []knowledge.HydratedEvidence
	err      error
	calls    int
	input    knowledge.ReauthorizeEvidenceInput
}

func (f *fakeRAGHydrator) ReauthorizeAndHydrateEvidence(
	_ context.Context,
	input knowledge.ReauthorizeEvidenceInput,
) ([]knowledge.HydratedEvidence, error) {
	f.calls++
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return append([]knowledge.HydratedEvidence(nil), f.evidence...), nil
}

func validRAGAssemblyInput() RAGAssemblyInput {
	return RAGAssemblyInput{
		ActorUserID:           DevUserID,
		SessionID:             "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ConversationID:        testConversationID,
		QueryText:             " What does alpha say? ",
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
	}
}

func validRAGCandidate() knowledge.EvidenceCandidateReference {
	return knowledge.EvidenceCandidateReference{
		CollectionID:      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		DocumentID:        "11111111-1111-4111-8111-111111111112",
		DocumentVersionID: "11111111-1111-4111-8111-111111111113",
		IndexGenerationID: "11111111-1111-4111-8111-111111111114",
		MaterializationID: "11111111-1111-4111-8111-111111111115",
		ParentChunkID:     "11111111-1111-4111-8111-111111111116",
		ChildChunkID:      "11111111-1111-4111-8111-111111111117",
		SourceSpanHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContentHash:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RankScore:         0.75,
	}
}

func validHydratedEvidence() knowledge.HydratedEvidence {
	candidate := validRAGCandidate()
	return knowledge.HydratedEvidence{
		CollectionID:      candidate.CollectionID,
		DocumentID:        candidate.DocumentID,
		DocumentVersionID: candidate.DocumentVersionID,
		IndexGenerationID: candidate.IndexGenerationID,
		MaterializationID: candidate.MaterializationID,
		ParentChunkID:     candidate.ParentChunkID,
		ChildChunkID:      candidate.ChildChunkID,
		SourceSpanHash:    candidate.SourceSpanHash,
		ContentHash:       candidate.ContentHash,
		SourceText:        "alpha evidence source",
		Locator:           []byte(`{"page":1}`),
		RankScore:         candidate.RankScore,
	}
}
