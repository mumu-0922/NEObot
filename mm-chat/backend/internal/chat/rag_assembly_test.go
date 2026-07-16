package chat

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestRAGAnswerAssemblerReturnsDependencyUnavailableWhenMissingSeams(t *testing.T) {
	_, err := (*RAGAnswerAssembler)(nil).AssembleStrict(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("error = %v, want ErrRAGDependencyUnavailable", err)
	}

	_, err = (&RAGAnswerAssembler{}).AssembleStrict(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGDependencyUnavailable) {
		t.Fatalf("error = %v, want ErrRAGDependencyUnavailable", err)
	}
}

func TestRAGAnswerAssemblerReturnsInsufficientEvidenceWithoutCandidates(t *testing.T) {
	hydrator := &fakeRAGHydrator{}
	assembler := NewRAGAnswerAssembler(&fakeRAGCandidateSource{}, hydrator)

	_, err := assembler.AssembleStrict(context.Background(), validRAGAssemblyInput())

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

	_, err := assembler.AssembleStrict(context.Background(), validRAGAssemblyInput())

	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

func TestRAGAnswerAssemblerStopsAtAnswerGateWhenEvidenceHydrates(t *testing.T) {
	source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
	hydrator := &fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}}
	assembler := NewRAGAnswerAssembler(source, hydrator)

	result, err := assembler.AssembleStrict(context.Background(), validRAGAssemblyInput())

	if !errors.Is(err, ErrRAGAnswerGatePending) {
		t.Fatalf("error = %v, want ErrRAGAnswerGatePending", err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].SourceText != "alpha evidence source" {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
	if source.query.QueryText != "What does alpha say?" || source.query.Limit != defaultRAGCandidateLimit {
		t.Fatalf("candidate query = %#v", source.query)
	}
	if hydrator.input.ActorUserID != DevUserID || hydrator.input.SessionID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
		t.Fatalf("hydration input = %#v", hydrator.input)
	}
}

func TestRAGAnswerAssemblerRejectsIncompleteInput(t *testing.T) {
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}},
		&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
	)
	input := validRAGAssemblyInput()
	input.QueryText = "   "

	_, err := assembler.AssembleStrict(context.Background(), input)

	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

type fakeRAGCandidateSource struct {
	refs  []knowledge.EvidenceCandidateReference
	err   error
	calls int
	query RAGCandidateQuery
}

func (f *fakeRAGCandidateSource) FetchEvidenceCandidates(
	_ context.Context,
	query RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	f.calls++
	f.query = query
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
