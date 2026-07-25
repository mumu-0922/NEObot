package ragevalcapture

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestAnswerMatchesExpectedUsesExactTextOrCompleteNumericTuple(t *testing.T) {
	for _, test := range []struct {
		name     string
		answer   string
		expected string
		want     bool
	}{
		{name: "exact Chinese", answer: "核定容量是 170 标准单元 [K1]", expected: "170 标准单元", want: true},
		{name: "exact English", answer: "Atlas Operations [K1]", expected: "Atlas Operations", want: true},
		{name: "cross section", answer: "容量为 170 标准单元，阈值为 68% [K1]", expected: "容量 170 标准单元，触发阈值 68%", want: true},
		{name: "Chinese date", answer: "生效日期为 2026年8月22日 [K1]", expected: "2026-08-22", want: true},
		{name: "wrong Chinese date", answer: "生效日期为 2026年8月23日 [K1]", expected: "2026-08-22", want: false},
		{name: "partial tuple", answer: "容量为 170 标准单元 [K1]", expected: "容量 170 标准单元，触发阈值 68%", want: false},
		{name: "wrong", answer: "Atlas Operations [K1]", expected: "Atlas Safety", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := answerMatchesExpected(test.answer, test.expected); got != test.want {
				t.Fatalf("answerMatchesExpected() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestObservedEvidenceIDsUseCuratorBoundAnswerWhenParserDropsAnchor(t *testing.T) {
	curated := CurationCase{
		PromotionCase: rageval.PromotionGoldenCase{
			ExpectedRelevantEvidenceIDs: []string{"RAGEVAL-CODE-ZH-05:F08"},
		},
		SourceBinding: CurationSourceBinding{
			SourceID: "RAGEVAL-CODE-ZH-05",
			Anchor:   "F08",
		},
		ExpectedAnswer: "容量 370 标准单元，触发阈值 84%",
	}
	evidence := []HydratedEvidence{
		{
			CandidateReference: CandidateReference{DocumentID: "expected-document"},
			SourceText:         `{"capacity":"370 标准单元","trigger_threshold":"84%"}`,
		},
		{
			CandidateReference: CandidateReference{DocumentID: "expected-document"},
			SourceText:         "F08 | 容量 370 标准单元，触发阈值 84%",
		},
		{
			CandidateReference: CandidateReference{DocumentID: "other-document"},
			SourceText:         "F08 | 容量 370 标准单元，触发阈值 84%",
		},
	}
	ids := observedEvidenceIDs(
		evidence,
		curated,
		map[string]string{
			"expected-document": "RAGEVAL-CODE-ZH-05",
			"other-document":    "RAGEVAL-CODE-ZH-04",
		},
		10,
	)
	if len(ids) != 2 || ids[0] != "RAGEVAL-CODE-ZH-05:F08" ||
		ids[1] != "RAGEVAL-CODE-ZH-04:F08" {
		t.Fatalf("observed evidence IDs = %#v", ids)
	}
	if !citedEvidenceSupportsExpected(
		evidence[:1],
		curated,
		map[string]string{"expected-document": "RAGEVAL-CODE-ZH-05"},
	) {
		t.Fatal("curator-bound answer evidence was not accepted as support")
	}
}

func TestCaptureAnswerPromptRequiresSmallestSufficientCitationSet(t *testing.T) {
	system, _, err := captureAnswerPrompt("question", []HydratedEvidence{{
		SourceName: "source.md",
		SourceText: "answer",
		Locator: json.RawMessage(
			`{"primary":{"kind":"text_line","locator":{"kind":"text_line","startLine":1,"endLine":1}}}`,
		),
	}})
	if err != nil {
		t.Fatalf("captureAnswerPrompt() error = %v", err)
	}
	for _, expected := range []string{
		"smallest sufficient citation set",
		"exactly one directly supporting source",
		"Never cite a merely similar source",
	} {
		if !strings.Contains(system, expected) {
			t.Fatalf("system prompt missing %q: %s", expected, system)
		}
	}
}

func TestApplyCaptureRerankIsDeterministicAndBounded(t *testing.T) {
	evidence := []HydratedEvidence{
		{CandidateReference: CandidateReference{ChildChunkID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}},
		{CandidateReference: CandidateReference{ChildChunkID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}},
		{CandidateReference: CandidateReference{ChildChunkID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc"}},
	}
	ranked, err := applyCaptureRerank(evidence, []ragproviders.RerankResult{
		{Index: 0, RelevanceScore: 0.5},
		{Index: 2, RelevanceScore: 0.9},
		{Index: 1, RelevanceScore: 0.5},
	}, 2)
	if err != nil {
		t.Fatalf("applyCaptureRerank() error = %v", err)
	}
	if len(ranked) != 2 ||
		ranked[0].ChildChunkID != "cccccccc-cccc-4ccc-8ccc-cccccccccccc" ||
		ranked[1].ChildChunkID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("ranked evidence = %#v", ranked)
	}
}

func TestApplyCaptureRerankPreservesExplicitSourceNameRouting(t *testing.T) {
	evidence := []HydratedEvidence{
		{
			CandidateReference: CandidateReference{ChildChunkID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
			SourceName:         "unrelated.xlsx",
		},
		{
			CandidateReference: CandidateReference{ChildChunkID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
			SourceName:         "rag-eval-xlsx-zh-04.xlsx",
		},
	}
	ranked, err := applyCaptureRerank(
		evidence,
		[]ragproviders.RerankResult{
			{Index: 0, RelevanceScore: 0.99},
			{Index: 1, RelevanceScore: 0.01},
		},
		2,
		"编号 RAGEVAL-XLSX-ZH-04 的例外代码是什么？",
	)
	if err != nil || len(ranked) != 2 ||
		ranked[0].SourceName != "rag-eval-xlsx-zh-04.xlsx" ||
		ranked[0].RankScore <= ranked[1].RankScore {
		t.Fatalf("source-routed capture rerank = %#v/%v", ranked, err)
	}
}

func TestCaptureCitationIndexesRejectsUnissuedMarker(t *testing.T) {
	indexes, valid := captureCitationIndexes("answer [K2] [K9] [K2]", 2)
	if valid || len(indexes) != 1 || indexes[0] != 1 {
		t.Fatalf("citation indexes = %#v/%v", indexes, valid)
	}
}

func TestCaptureIntegrityRequiresSheetCellLineageForTable(t *testing.T) {
	lineage := HydratedEvidence{
		Locator:          json.RawMessage(`{"primary":{"kind":"sheet_cell","locator":{"kind":"sheet_cell","sheet":"Data","startCell":"A1","endCell":"B2"}}}`),
		ProvenanceValid:  true,
		CellLineageValid: true,
	}
	locator, provenance, cells := captureIntegrity([]HydratedEvidence{lineage}, true)
	if !locator || !provenance || !cells {
		t.Fatalf("integrity = %v/%v/%v", locator, provenance, cells)
	}
	lineage.CellLineageValid = false
	_, _, cells = captureIntegrity([]HydratedEvidence{lineage}, true)
	if cells {
		t.Fatal("missing cell lineage was accepted")
	}
}

func TestCaptureRerankDocumentLabelsSourceMetadataSeparately(t *testing.T) {
	document := captureRerankDocument(HydratedEvidence{
		SourceName: "rag-eval-xlsx-zh-01.xlsx",
		SourceText: "F03 | 核定容量 | 281 标准单元",
	})
	for _, expected := range []string{
		`Source file metadata (not Citation evidence): "rag-eval-xlsx-zh-01.xlsx"`,
		"Matched Child source:",
		"281 标准单元",
	} {
		if !strings.Contains(document, expected) {
			t.Fatalf("capture rerank document missing %q: %s", expected, document)
		}
	}
}

func TestSummarizePreflightMetricsUsesFormalEvaluatorMath(t *testing.T) {
	cases := []rageval.PromotionGoldenCase{{
		ID:                          "case-1",
		Slices:                      []string{"pdf", "pdf"},
		ExpectedRelevantEvidenceIDs: []string{"source:F01"},
	}}
	perfect := PreflightObservation{
		CaseID:               "case-1",
		RetrievedEvidenceIDs: []string{"source:F01"},
		FinalEvidenceIDs:     []string{"source:F01"},
		CitationEvidenceIDs:  []string{"source:F01"},
		Answered:             true,
		AnswerCorrectness:    1,
		Faithfulness:         1,
		LatencyMilliseconds:  100,
		ContextTokens:        200,
		Integrity: rageval.PromotionCaseIntegrity{
			CitationLocatorValid: true,
			ProvenanceValid:      true,
			CellLineageValid:     true,
		},
	}
	regressed := perfect
	regressed.FinalEvidenceIDs = nil
	regressed.CitationEvidenceIDs = nil
	regressed.AnswerCorrectness = 0
	regressed.Faithfulness = 0
	regressed.LatencyMilliseconds = 1200
	regressed.ContextTokens = 5000

	metrics, err := summarizePreflightMetrics(
		cases,
		[]PreflightObservation{regressed},
		rageval.PromotionCriteria{
			MaximumP95LatencyMilliseconds:      1000,
			MaximumAverageContextTokens:        4096,
			MinimumAggregateQualityImprovement: 0.005,
		},
	)
	if err != nil {
		t.Fatalf("summarizePreflightMetrics() error = %v", err)
	}
	if metrics.candidate.QualityScore >= 1 {
		t.Fatalf("candidate summary = %#v", metrics.candidate)
	}
	if metrics.slices["pdf"].Cases != 1 ||
		len(metrics.slices["pdf"].Failures) == 0 ||
		metrics.slices["pdf"].Passed {
		t.Fatalf("pdf slice = %#v", metrics.slices["pdf"])
	}
	if metrics.budgets.LatencyPassed || metrics.budgets.ContextTokenCostPassed {
		t.Fatalf("budgets = %#v", metrics.budgets)
	}
}

func TestSelectCaptureCasesSupportsExplicitIncompleteSmokeCase(t *testing.T) {
	golden := []rageval.PromotionGoldenCase{
		{ID: "development-pdf", Split: "development"},
		{ID: "validation-xlsx", Split: "validation"},
		{ID: "holdout-xlsx", Split: "holdout"},
	}
	selected, complete, err := selectCaptureCases(
		golden,
		[]string{"development", "validation"},
		"validation-xlsx",
		0,
	)
	if err != nil {
		t.Fatalf("selectCaptureCases() error = %v", err)
	}
	if complete || len(selected) != 1 || selected[0].ID != "validation-xlsx" {
		t.Fatalf("selected = %#v, complete = %v", selected, complete)
	}
	if _, _, err := selectCaptureCases(
		golden,
		[]string{"development", "validation"},
		"holdout-xlsx",
		0,
	); err == nil {
		t.Fatal("Holdout case escaped the selected split boundary")
	}
}

func TestRetryCaptureOperationRetriesTransientFailureAndHonorsCancellation(t *testing.T) {
	attempts := 0
	value, err := retryCaptureOperation(
		context.Background(),
		4,
		time.Nanosecond,
		time.Millisecond,
		func() (string, error) {
			attempts++
			if attempts < 3 {
				return "", errors.New("transient")
			}
			return "ready", nil
		},
	)
	if err != nil || value != "ready" || attempts != 3 {
		t.Fatalf("retry result = %q/%v after %d attempts", value, err, attempts)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = retryCaptureOperation(ctx, 4, time.Second, time.Second, func() (string, error) {
		return "", errors.New("unavailable")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retry error = %v", err)
	}
}

type captureProfileEmbedder struct {
	modelID string
	marker  float32
	calls   int
}

func (embedder *captureProfileEmbedder) EmbedQuery(
	_ context.Context,
	_ string,
) (ragproviders.QueryEmbedding, error) {
	embedder.calls++
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = embedder.marker
	return ragproviders.QueryEmbedding{
		ModelID: embedder.modelID, Dimensions: len(vector), Vector: vector,
	}, nil
}

type captureProfileReranker struct {
	calls int
}

func (reranker *captureProfileReranker) Rerank(
	_ context.Context,
	_ string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	reranker.calls++
	results := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		results[index] = ragproviders.RerankResult{
			Index: index, RelevanceScore: 0.9,
		}
	}
	return results, nil
}

type captureProfileStore struct {
	generations []string
	markers     []float32
}

func (*captureProfileStore) Status(context.Context) (GenerationStatus, error) {
	return GenerationStatus{}, nil
}

func (store *captureProfileStore) FetchCandidates(
	_ context.Context,
	generationID string,
	_ []string,
	_ string,
	embedding []float32,
	_ int,
) ([]CandidateReference, error) {
	store.generations = append(store.generations, generationID)
	store.markers = append(store.markers, embedding[0])
	return []CandidateReference{{
		DocumentID:    "document-1",
		ParentChunkID: "parent-1",
		ChildChunkID:  "child-1",
	}}, nil
}

func (*captureProfileStore) Hydrate(
	_ context.Context,
	_ string,
	_ []string,
	references []CandidateReference,
) ([]HydratedEvidence, error) {
	return []HydratedEvidence{{
		CandidateReference: references[0],
		SourceName:         "source.md",
		SourceText:         "F01 exact answer",
		ChildTokenCount:    4,
		ParentSourceText:   "F01 exact answer",
		ParentTokenCount:   4,
		Locator: json.RawMessage(
			`{"primary":{"kind":"text_line","locator":{"kind":"text_line","startLine":1,"endLine":1}}}`,
		),
		ProvenanceValid: true,
	}}, nil
}

type captureProfileAnswerer struct{}

func (captureProfileAnswerer) Answer(
	_ context.Context,
	_ string,
	_ string,
) (AnswerResult, error) {
	return AnswerResult{Content: "exact answer [K1]"}, nil
}

func TestCaptureCandidateCaseUsesOnlySiliconFlowProviderSpace(t *testing.T) {
	candidateEmbedder := &captureProfileEmbedder{
		modelID: ragproviders.SiliconFlowEmbeddingModel, marker: 2,
	}
	candidateReranker := &captureProfileReranker{}
	store := &captureProfileStore{}
	input := CaptureInput{
		LoadedInputs: LoadedInputs{
			Curation: CurationQueue{CollectionBinding: CurationCollection{
				Alias: "fixture", CollectionID: "collection-1",
			}},
			CuratedByCaseID: map[string]CurationCase{
				"case-1": {
					PromotionCase: rageval.PromotionGoldenCase{
						ExpectedRelevantEvidenceIDs: []string{"source-1:F01"},
					},
					SourceBinding: CurationSourceBinding{
						SourceID: "source-1", Anchor: "F01",
					},
					ExpectedAnswer: "exact answer",
				},
			},
			SourceByDocumentID: map[string]string{"document-1": "source-1"},
		},
		Store: store,
		CandidateProvider: CaptureRetrievalProvider{
			Profile:  ragproviders.SiliconFlowRetrievalProfile,
			Embedder: candidateEmbedder,
			Reranker: candidateReranker,
		},
		Answerer:              captureProfileAnswerer{},
		CandidateGenerationID: "candidate-generation",
	}
	observation, err := captureCandidateCase(
		context.Background(),
		input,
		rageval.PromotionGoldenCase{
			ID: "case-1", Query: "query",
			SelectedCollectionAliases: []string{"fixture"},
		},
	)
	if err != nil {
		t.Fatalf("captureCandidateCase() error = %v", err)
	}
	if candidateEmbedder.calls != 1 || candidateReranker.calls != 1 ||
		strings.Join(store.generations, ",") != "candidate-generation" ||
		len(store.markers) != 1 || store.markers[0] != 2 ||
		observation.CaseID != "case-1" {
		t.Fatalf(
			"candidate provider failed: candidate=%d/%d generations=%v markers=%v observation=%#v",
			candidateEmbedder.calls,
			candidateReranker.calls,
			store.generations,
			store.markers,
			observation,
		)
	}
}
