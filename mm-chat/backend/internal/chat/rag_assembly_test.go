package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestRAGAnswerAssemblerAppliesGoldenRelevancePolicyAndTopK(t *testing.T) {
	refs, evidence := rerankFixture(6)
	source := &fakeRAGCandidateSource{refs: refs}
	hydrator := &mappedRAGHydrator{evidence: evidence}
	reranker := &fakeRAGEvidenceReranker{results: []RAGRerankResult{
		{Index: 0, RelevanceScore: -0.1},
		{Index: 1, RelevanceScore: 0.2},
		{Index: 2, RelevanceScore: 0.9},
		{Index: 3, RelevanceScore: 0.4},
		{Index: 4, RelevanceScore: -0.2},
		{Index: 5, RelevanceScore: 0.8},
	}}
	assembler := NewRAGAnswerAssembler(
		source,
		hydrator,
		WithRAGEvidenceReranker(reranker, fakeRAGRerankGate{}),
	)
	assembler.EvidenceLimit = 2
	input := validRAGAssemblyInput()
	input.SelectedCollectionIDs = []string{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	}
	input.RewrittenQueryText = "  standalone semantic query  "

	result, err := assembler.Assemble(context.Background(), input)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if result.RerankStatus != ragRerankStatusApplied || len(result.Evidence) != 2 ||
		result.Evidence[0].ChildChunkID != refs[2].ChildChunkID ||
		result.Evidence[1].ChildChunkID != refs[5].ChildChunkID {
		t.Fatalf("reranked result = %#v", result)
	}
	if reranker.query != strings.TrimSpace(input.RewrittenQueryText) || len(reranker.documents) != 6 {
		t.Fatalf("reranker query/documents = %q/%d", reranker.query, len(reranker.documents))
	}
	if !strings.Contains(reranker.documents[0], `Source file metadata (not Citation evidence): "source-0.md"`) ||
		!strings.Contains(reranker.documents[0], "authorized source 0") {
		t.Fatalf("reranker document = %q", reranker.documents[0])
	}
	if len(hydrator.inputs) != 1 || len(hydrator.inputs[0].References) != 6 {
		t.Fatalf("hydration inputs = %#v", hydrator.inputs)
	}
}

func TestApplyRAGRerankPreservesExplicitSourceNameRouting(t *testing.T) {
	evidence := []knowledge.HydratedEvidence{
		{
			ChildChunkID: "11111111-1111-4111-8111-111111111111",
			SourceName:   "unrelated.xlsx",
		},
		{
			ChildChunkID: "22222222-2222-4222-8222-222222222222",
			SourceName:   "rag-eval-xlsx-zh-04.xlsx",
		},
	}
	ranked, ok := applyRAGRerank(
		evidence,
		[]RAGRerankResult{
			{Index: 0, RelevanceScore: 0.99},
			{Index: 1, RelevanceScore: 0.01},
		},
		2,
		ragGoldenRelevancePolicyV2,
		"编号 RAGEVAL-XLSX-ZH-04 的例外代码是什么？",
	)
	if !ok || len(ranked) != 2 ||
		ranked[0].SourceName != "rag-eval-xlsx-zh-04.xlsx" ||
		ranked[0].RankScore <= ranked[1].RankScore {
		t.Fatalf("source-routed rerank = %#v/%v", ranked, ok)
	}
}

func TestRAGAnswerAssemblerFailsClosedOnRerankerFailure(t *testing.T) {
	refs, evidence := rerankFixture(7)
	hydrator := &mappedRAGHydrator{evidence: evidence}
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		hydrator,
		WithRAGEvidenceReranker(
			&fakeRAGEvidenceReranker{err: errors.New("reranker unavailable")},
			fakeRAGRerankGate{},
		),
	)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("Assemble() result/error = %#v/%v, want fail-closed no evidence", result, err)
	}
	if len(result.Evidence) != 0 || len(result.Citations) != 0 || len(hydrator.inputs) != 1 {
		t.Fatalf("fail-closed result/hydration = %#v/%#v", result, hydrator.inputs)
	}
}

func TestRAGAnswerAssemblerTreatsSuccessfulBelowThresholdRerankAsNoEvidence(t *testing.T) {
	refs, evidence := rerankFixture(2)
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		&mappedRAGHydrator{evidence: evidence},
		WithRAGEvidenceReranker(
			&fakeRAGEvidenceReranker{results: []RAGRerankResult{
				{Index: 0, RelevanceScore: -0.1},
				{Index: 1, RelevanceScore: -0.2},
			}},
			fakeRAGRerankGate{},
		),
	)

	_, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

func TestRAGAnswerAssemblerFailsClosedWithoutRerankGovernance(t *testing.T) {
	refs, evidence := rerankFixture(7)
	hydrator := &mappedRAGHydrator{evidence: evidence}
	reranker := &fakeRAGEvidenceReranker{}
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		hydrator,
		WithRAGEvidenceReranker(
			reranker,
			fakeRAGRerankGate{err: ErrRAGRerankGovernanceRequired},
		),
	)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGInsufficientEvidence) {
		t.Fatalf("Assemble() result/error = %#v/%v, want fail-closed no evidence", result, err)
	}
	if reranker.calls != 0 || len(hydrator.inputs) != 0 || len(result.Citations) != 0 {
		t.Fatalf("governance fail-closed = %#v/%d/%#v", result, reranker.calls, hydrator.inputs)
	}
}

func TestRAGAnswerAssemblerFailsClosedOnPartialRerankConfiguration(t *testing.T) {
	refs, evidence := rerankFixture(2)
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		&mappedRAGHydrator{evidence: evidence},
		WithRAGEvidenceReranker(&fakeRAGEvidenceReranker{}, nil),
	)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGInsufficientEvidence) || len(result.Citations) != 0 {
		t.Fatalf("Assemble() result/error = %#v/%v, want fail-closed no evidence", result, err)
	}
}

func TestRAGAnswerAssemblerFailsClosedOnMalformedRerankResult(t *testing.T) {
	refs, evidence := rerankFixture(2)
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		&mappedRAGHydrator{evidence: evidence},
		WithRAGEvidenceReranker(
			&fakeRAGEvidenceReranker{results: []RAGRerankResult{
				{Index: 0, RelevanceScore: 0.8},
			}},
			fakeRAGRerankGate{},
		),
	)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if !errors.Is(err, ErrRAGInsufficientEvidence) || len(result.Citations) != 0 {
		t.Fatalf("Assemble() result/error = %#v/%v, want fail-closed no evidence", result, err)
	}
}

func TestRAGAnswerAssemblerHydratesTwentyCandidatesInBoundedBatches(t *testing.T) {
	refs, evidence := rerankFixture(defaultRAGCandidateLimit)
	hydrator := &mappedRAGHydrator{evidence: evidence}
	results := make([]RAGRerankResult, len(refs))
	for index := range results {
		results[index] = RAGRerankResult{Index: index, RelevanceScore: float64(index + 1)}
	}
	assembler := NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{refs: refs},
		hydrator,
		WithRAGEvidenceReranker(
			&fakeRAGEvidenceReranker{results: results},
			fakeRAGRerankGate{},
		),
	)

	result, err := assembler.Assemble(context.Background(), validRAGAssemblyInput())
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(hydrator.inputs) != 2 || len(hydrator.inputs[0].References) != 16 ||
		len(hydrator.inputs[1].References) != 4 || len(result.Evidence) != defaultRAGEvidenceLimit {
		t.Fatalf("batch sizes/result = %d/%d/%d", len(hydrator.inputs), len(hydrator.inputs[0].References), len(result.Evidence))
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

type mappedRAGHydrator struct {
	evidence map[string]knowledge.HydratedEvidence
	inputs   []knowledge.ReauthorizeEvidenceInput
}

func (f *mappedRAGHydrator) ReauthorizeAndHydrateEvidence(
	_ context.Context,
	input knowledge.ReauthorizeEvidenceInput,
) ([]knowledge.HydratedEvidence, error) {
	f.inputs = append(f.inputs, input)
	result := make([]knowledge.HydratedEvidence, 0, len(input.References))
	for _, reference := range input.References {
		evidence, ok := f.evidence[reference.ChildChunkID]
		if !ok {
			return nil, knowledge.ErrEvidenceHydrationRejected
		}
		result = append(result, evidence)
	}
	return result, nil
}

type fakeRAGEvidenceReranker struct {
	results      []RAGRerankResult
	err          error
	calls        int
	generationID string
	query        string
	documents    []string
}

func (f *fakeRAGEvidenceReranker) Rerank(
	_ context.Context,
	generationID string,
	query string,
	documents []string,
) ([]RAGRerankResult, error) {
	f.calls++
	f.generationID = generationID
	f.query = query
	f.documents = append([]string(nil), documents...)
	if f.err != nil {
		return nil, f.err
	}
	return append([]RAGRerankResult(nil), f.results...), nil
}

type fakeRAGRerankGate struct {
	err error
}

func (g fakeRAGRerankGate) AuthorizeRAGRerank(
	_ context.Context,
	_ []string,
	_ string,
) error {
	return g.err
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
		SourceName:        "alpha-source.md",
		SourceText:        "alpha evidence source",
		ChildTokenCount:   3,
		ParentSourceText:  "alpha evidence parent with broader source context",
		ParentTokenCount:  8,
		Locator:           []byte(`{"page":1}`),
		RankScore:         candidate.RankScore,
	}
}

func rerankFixture(count int) (
	[]knowledge.EvidenceCandidateReference,
	map[string]knowledge.HydratedEvidence,
) {
	references := make([]knowledge.EvidenceCandidateReference, 0, count)
	evidence := make(map[string]knowledge.HydratedEvidence, count)
	for index := 0; index < count; index++ {
		reference := validRAGCandidate()
		reference.CollectionID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		if index%2 == 1 {
			reference.CollectionID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		}
		reference.ChildChunkID = fmt.Sprintf("11111111-1111-4111-8111-%012d", index+100)
		reference.ContentHash = strings.Repeat(fmt.Sprintf("%x", index%15+1), 64)
		reference.RankScore = 1.0 / float64(index+1)
		references = append(references, reference)
		evidence[reference.ChildChunkID] = knowledge.HydratedEvidence{
			CollectionID:      reference.CollectionID,
			DocumentID:        reference.DocumentID,
			DocumentVersionID: reference.DocumentVersionID,
			IndexGenerationID: reference.IndexGenerationID,
			MaterializationID: reference.MaterializationID,
			ParentChunkID:     reference.ParentChunkID,
			ChildChunkID:      reference.ChildChunkID,
			SourceSpanHash:    reference.SourceSpanHash,
			ContentHash:       reference.ContentHash,
			SourceName:        fmt.Sprintf("source-%d.md", index),
			SourceText:        fmt.Sprintf("authorized source %d", index),
			Locator:           []byte(`{"page":1}`),
			RankScore:         reference.RankScore,
		}
	}
	return references, evidence
}
