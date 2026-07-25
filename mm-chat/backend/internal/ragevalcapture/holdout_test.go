package ragevalcapture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	holdoutTestGenerationID = "11111111-1111-4111-8111-111111111111"
	holdoutTestRunID        = "22222222-2222-4222-8222-222222222222"
	holdoutTestCollectionID = "33333333-3333-4333-8333-333333333333"
)

type frozenHoldoutStore struct {
	status GenerationStatus
}

func (store frozenHoldoutStore) Status(context.Context) (GenerationStatus, error) {
	return store.status, nil
}

func (frozenHoldoutStore) FetchCandidates(
	context.Context,
	string,
	[]string,
	string,
	[]float32,
	int,
) ([]CandidateReference, error) {
	return []CandidateReference{{
		CollectionID:  holdoutTestCollectionID,
		DocumentID:    "fixture-document",
		ParentChunkID: "fixture-parent",
		ChildChunkID:  "fixture-child",
	}}, nil
}

func (frozenHoldoutStore) Hydrate(
	_ context.Context,
	_ string,
	_ []string,
	references []CandidateReference,
) ([]HydratedEvidence, error) {
	return []HydratedEvidence{{
		CandidateReference: references[0],
		SourceName:         "fixture.xlsx",
		SourceText:         "F01 exact answer",
		ChildTokenCount:    4,
		ParentSourceText:   "F01 exact answer",
		ParentTokenCount:   4,
		Locator: []byte(
			`{"primary":{"kind":"sheet_cell","locator":{"kind":"sheet_cell","sheet":"Data","startCell":"A1","endCell":"A1"}}}`,
		),
		ProvenanceValid:  true,
		CellLineageValid: true,
	}}, nil
}

type frozenHoldoutEmbedder struct {
	sealed *atomic.Bool
	calls  atomic.Int64
}

func (embedder *frozenHoldoutEmbedder) EmbedQuery(
	context.Context,
	string,
) (ragproviders.QueryEmbedding, error) {
	if embedder.sealed != nil && !embedder.sealed.Load() {
		return ragproviders.QueryEmbedding{}, errors.New("provider called before seal")
	}
	embedder.calls.Add(1)
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = 1
	return ragproviders.QueryEmbedding{
		ModelID:    ragproviders.SiliconFlowEmbeddingModel,
		Dimensions: len(vector),
		Vector:     vector,
	}, nil
}

type frozenHoldoutReranker struct{}

func (frozenHoldoutReranker) Rerank(
	context.Context,
	string,
	[]string,
) ([]ragproviders.RerankResult, error) {
	return []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}}, nil
}

type frozenHoldoutAnswerer struct{}

func (frozenHoldoutAnswerer) Answer(
	context.Context,
	string,
	string,
) (AnswerResult, error) {
	return AnswerResult{Content: "exact answer [K1]"}, nil
}

func TestCapturePreflightStillRejectsHoldoutSplit(t *testing.T) {
	fixture := newFrozenHoldoutFixture(t)
	input := fixture.CaptureInput
	input.Splits = []string{"holdout"}
	if err := validateCaptureInput(input); err == nil ||
		!strings.Contains(err.Error(), "cannot execute Holdout") {
		t.Fatalf("validateCaptureInput() error = %v", err)
	}
}

func TestCaptureFrozenHoldoutSealsBeforeProviderAndOrdersAllCases(t *testing.T) {
	input := newFrozenHoldoutFixture(t)
	var sealed atomic.Bool
	embedder := input.CandidateProvider.Embedder.(*frozenHoldoutEmbedder)
	embedder.sealed = &sealed
	sealCalls := 0
	input.Seal = func(seal HoldoutSeal) error {
		sealCalls++
		if seal.HoldoutRunID != holdoutTestRunID || seal.Ordinal != 1 ||
			seal.CandidateGenerationID != holdoutTestGenerationID {
			t.Fatalf("seal = %#v", seal)
		}
		sealed.Store(true)
		return nil
	}

	observations, err := CaptureFrozenHoldout(context.Background(), input)
	if err != nil {
		t.Fatalf("CaptureFrozenHoldout() error = %v", err)
	}
	if sealCalls != 1 || embedder.calls.Load() != 100 || len(observations.Cases) != 500 {
		t.Fatalf(
			"seal/provider/cases = %d/%d/%d",
			sealCalls,
			embedder.calls.Load(),
			len(observations.Cases),
		)
	}
	seen := make(map[string]struct{}, len(observations.Cases))
	for index, item := range observations.Cases {
		if item.CaseID != input.Golden.Cases[index].ID {
			t.Fatalf("case %d = %q", index, item.CaseID)
		}
		if _, duplicate := seen[item.CaseID]; duplicate {
			t.Fatalf("duplicate case %q", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
	}
	if observations.ProfileRole != "candidate" ||
		observations.ProfileID != string(ragproviders.RetrievalProfileSiliconFlow) ||
		observations.HoldoutRun.ID != holdoutTestRunID ||
		observations.HoldoutRun.Ordinal != 1 {
		t.Fatalf("observation header = %#v", observations)
	}
}

func TestCaptureFrozenHoldoutRejectsRunIDMismatchBeforeSeal(t *testing.T) {
	input := newFrozenHoldoutFixture(t)
	input.Validation.Report.Holdout.PrecommittedRunID =
		"99999999-9999-4999-8999-999999999999"
	sealed := false
	input.Seal = func(HoldoutSeal) error {
		sealed = true
		return nil
	}
	if _, err := CaptureFrozenHoldout(context.Background(), input); err == nil {
		t.Fatal("Holdout run ID mismatch was accepted")
	}
	if sealed {
		t.Fatal("invalid Holdout consumed the one-shot seal")
	}
}

func TestCaptureFrozenHoldoutRejectsIncompleteOrDriftedPreflight(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*FrozenHoldoutInput)
	}{
		{
			name: "incomplete Development",
			mutate: func(input *FrozenHoldoutInput) {
				input.Development.Report.Complete = false
			},
		},
		{
			name: "manifest drift",
			mutate: func(input *FrozenHoldoutInput) {
				input.Validation.Report.Candidate.ArtifactManifestHash = strings.Repeat("9", 64)
			},
		},
		{
			name: "revision drift",
			mutate: func(input *FrozenHoldoutInput) {
				input.Development.Report.Configuration.CorpusProjectionRevision++
			},
		},
		{
			name: "future preflight",
			mutate: func(input *FrozenHoldoutInput) {
				input.Validation.Report.CapturedAt = "2026-07-25T00:00:00Z"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newFrozenHoldoutFixture(t)
			test.mutate(&input)
			sealed := false
			input.Seal = func(HoldoutSeal) error {
				sealed = true
				return nil
			}
			if _, err := CaptureFrozenHoldout(context.Background(), input); err == nil {
				t.Fatal("invalid preflight was accepted")
			}
			if sealed {
				t.Fatal("invalid preflight consumed the one-shot seal")
			}
		})
	}
}

func TestCreateHoldoutSealAndObservationOutputNeverOverwrite(t *testing.T) {
	directory := t.TempDir()
	sealPath := filepath.Join(directory, "holdout.seal.json")
	seal := validTestHoldoutSeal()
	if err := CreateHoldoutSeal(sealPath, seal); err != nil {
		t.Fatalf("CreateHoldoutSeal() error = %v", err)
	}
	original, err := os.ReadFile(sealPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateHoldoutSeal(sealPath, seal); err == nil ||
		!strings.Contains(err.Error(), "permanently refused") {
		t.Fatalf("duplicate CreateHoldoutSeal() error = %v", err)
	}
	after, err := os.ReadFile(sealPath)
	if err != nil || string(after) != string(original) {
		t.Fatal("existing Holdout seal was modified")
	}

	input := newFrozenHoldoutFixture(t)
	input.Seal = func(HoldoutSeal) error { return nil }
	observations, err := CaptureFrozenHoldout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(directory, "observations.json")
	if err := WritePromotionObservationsExclusive(outputPath, observations, true); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(outputPath)
	if err := WritePromotionObservationsExclusive(outputPath, observations, true); err == nil {
		t.Fatal("existing promotion observations were overwritten")
	}
	after, _ = os.ReadFile(outputPath)
	if string(after) != string(before) {
		t.Fatal("existing promotion observations changed")
	}
}

func newFrozenHoldoutFixture(t *testing.T) FrozenHoldoutInput {
	t.Helper()
	frozenAt := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	captureAt := frozenAt.Add(time.Hour)
	executeAt := frozenAt.Add(2 * time.Hour)
	reviewerID := "44444444-4444-4444-8444-444444444444"
	criticalSlices := rageval.PromotionCriticalSlices()
	golden := rageval.PromotionGoldenSet{
		SchemaVersion: rageval.PromotionGoldenSchemaVersion,
		ID:            "frozen-holdout-test",
		Description:   "frozen Holdout test corpus",
		Lifecycle: rageval.PromotionGoldenLifecycle{
			State:        "frozen",
			FrozenAt:     frozenAt.Format(time.RFC3339),
			HoldoutRunID: holdoutTestRunID,
		},
		Criteria: rageval.PromotionCriteria{
			MaximumP95LatencyMilliseconds: 1000,
			MaximumAverageContextTokens:   4096,
		},
		Cases: make([]rageval.PromotionGoldenCase, 500),
	}
	curated := make(map[string]CurationCase, len(golden.Cases))
	for index := range golden.Cases {
		split := "development"
		if index >= 300 {
			split = "validation"
		}
		if index >= 400 {
			split = "holdout"
		}
		caseID := fmtCaseID(index)
		golden.Cases[index] = rageval.PromotionGoldenCase{
			ID:                          caseID,
			Query:                       "What is the exact answer?",
			Split:                       split,
			Slices:                      append([]string(nil), criticalSlices...),
			SelectedCollectionAliases:   []string{"fixture"},
			ExpectedRelevantEvidenceIDs: []string{"fixture-source:F01"},
			TableExactAnswerRequired:    true,
			Review: rageval.PromotionReview{
				State:      "human_reviewed",
				ReviewerID: reviewerID,
				ReviewedAt: frozenAt.Add(-time.Hour).Format(time.RFC3339),
			},
		}
		curated[caseID] = CurationCase{
			PromotionCase: golden.Cases[index],
			SourceBinding: CurationSourceBinding{
				SourceID: "fixture-source",
				Anchor:   "F01",
			},
			ExpectedAnswer: "exact answer",
		}
	}
	contentHash, err := rageval.PromotionGoldenContentSHA256(golden)
	if err != nil {
		t.Fatal(err)
	}
	golden.Lifecycle.FrozenContentSHA256 = contentHash
	status := GenerationStatus{
		HeadRevision:                  4,
		CorpusProjectionRevision:      298,
		CandidateGenerationID:         holdoutTestGenerationID,
		CandidateStatus:               "verified",
		CandidateChunkProfileHash:     strings.Repeat("c", 64),
		CandidateArtifactManifestHash: strings.Repeat("d", 64),
		CandidateReadiness:            "ready",
	}
	embedder := &frozenHoldoutEmbedder{}
	base := CaptureInput{
		LoadedInputs: LoadedInputs{
			Golden:          golden,
			GoldenRawSHA256: strings.Repeat("a", 64),
			Curation: CurationQueue{CollectionBinding: CurationCollection{
				Alias: "fixture", CollectionID: holdoutTestCollectionID,
			}},
			CurationRawSHA256:  strings.Repeat("b", 64),
			ReviewRawSHA256:    strings.Repeat("e", 64),
			ImportRawSHA256:    strings.Repeat("f", 64),
			CuratedByCaseID:    curated,
			SourceByDocumentID: map[string]string{"fixture-document": "fixture-source"},
		},
		Store: frozenHoldoutStore{status: status},
		CandidateProvider: CaptureRetrievalProvider{
			Profile:  ragproviders.SiliconFlowRetrievalProfile,
			Embedder: embedder,
			Reranker: frozenHoldoutReranker{},
		},
		Answerer:              frozenHoldoutAnswerer{},
		CandidateGenerationID: holdoutTestGenerationID,
		CandidateManifestHash: status.CandidateArtifactManifestHash,
		AnswerProviderID:      "SERVER_DEFAULT",
		AnswerModelID:         "answer-model",
		Concurrency:           4,
		Clock:                 func() time.Time { return executeAt },
		NewUUID: func() string {
			return "55555555-5555-4555-8555-555555555555"
		},
	}
	development := frozenPreflightReport(t, base, status, "development", captureAt,
		"66666666-6666-4666-8666-666666666666")
	validation := frozenPreflightReport(t, base, status, "validation", captureAt,
		"77777777-7777-4777-8777-777777777777")
	return FrozenHoldoutInput{
		CaptureInput: base,
		Development: LoadedPreflightReport{
			Report: development, RawSHA256: strings.Repeat("1", 64),
		},
		Validation: LoadedPreflightReport{
			Report: validation, RawSHA256: strings.Repeat("2", 64),
		},
		ObservationOutputPath: "/tmp/frozen-observations.json",
		Seal:                  func(HoldoutSeal) error { return nil },
	}
}

func frozenPreflightReport(
	t *testing.T,
	input CaptureInput,
	status GenerationStatus,
	split string,
	capturedAt time.Time,
	captureID string,
) PreflightReport {
	t.Helper()
	cases, complete, err := selectCaptureCases(input.Golden.Cases, []string{split}, "", 0)
	if err != nil || !complete {
		t.Fatalf("selectCaptureCases() = %v/%v", complete, err)
	}
	observations := make([]PreflightObservation, len(cases))
	for index, item := range cases {
		observations[index] = PreflightObservation{
			CaseID:               item.ID,
			RetrievedEvidenceIDs: []string{"fixture-source:F01"},
			FinalEvidenceIDs:     []string{"fixture-source:F01"},
			CitationEvidenceIDs:  []string{"fixture-source:F01"},
			AnswerSHA256:         hashCaptureText("exact answer [K1]"),
			Answered:             true,
			AnswerCorrectness:    1,
			Faithfulness:         1,
			TableExactAnswer:     true,
			LatencyMilliseconds:  100,
			LatencyBreakdown: PreflightLatencyBreakdown{
				EmbedQueryMilliseconds:      20,
				FetchCandidatesMilliseconds: 20,
				HydrateEvidenceMilliseconds: 20,
				RerankMilliseconds:          20,
				PipelineTotalMilliseconds:   100,
			},
			ContextTokens: 100,
			Integrity: rageval.PromotionCaseIntegrity{
				CitationLocatorValid: true,
				ProvenanceValid:      true,
				CellLineageValid:     true,
			},
		}
	}
	metrics, err := summarizePreflightMetrics(cases, observations, input.Golden.Criteria)
	if err != nil {
		t.Fatal(err)
	}
	return PreflightReport{
		SchemaVersion:     PreflightSchemaVersion,
		CaptureVersion:    CaptureVersion,
		PromotionEligible: false,
		Complete:          true,
		CapturedAt:        capturedAt.Format(time.RFC3339),
		Inputs: PreflightInputHashes{
			GoldenRawSHA256:       input.GoldenRawSHA256,
			GoldenContentSHA256:   input.Golden.Lifecycle.FrozenContentSHA256,
			CurationRawSHA256:     input.CurationRawSHA256,
			HumanReviewRawSHA256:  input.ReviewRawSHA256,
			SourceImportRawSHA256: input.ImportRawSHA256,
		},
		Configuration: PreflightConfiguration{
			Splits:                   []string{split},
			CandidateRetrieval:       captureProviderConfiguration(input.CandidateProvider),
			AnswerProviderID:         input.AnswerProviderID,
			AnswerModelID:            input.AnswerModelID,
			ProviderMaximumAttempts:  captureProviderAttempts,
			ProviderInitialRetryMS:   captureInitialRetryDelay.Milliseconds(),
			ProviderMaximumRetryMS:   captureMaximumRetryDelay.Milliseconds(),
			CandidateLimit:           captureCandidateLimit,
			RerankLimit:              captureRerankLimit,
			FinalLimit:               captureFinalLimit,
			MaximumContextTokens:     captureMaximumContextTokens,
			Concurrency:              input.Concurrency,
			ScoringPolicy:            captureScoringPolicy,
			GenerationHeadRevision:   status.HeadRevision,
			CorpusProjectionRevision: status.CorpusProjectionRevision,
		},
		Holdout: PreflightHoldout{
			State:             "not_executed",
			PrecommittedRunID: input.Golden.Lifecycle.HoldoutRunID,
		},
		Candidate: PreflightProfileCapture{
			CaptureID:            captureID,
			ProfileRole:          "candidate",
			GenerationID:         status.CandidateGenerationID,
			ArtifactManifestHash: status.CandidateArtifactManifestHash,
			ChunkProfileHash:     status.CandidateChunkProfileHash,
			Summary:              metrics.candidate,
			Cases:                observations,
		},
		Slices:  metrics.slices,
		Budgets: metrics.budgets,
	}
}

func validTestHoldoutSeal() HoldoutSeal {
	return HoldoutSeal{
		SchemaVersion:         HoldoutSealVersion,
		CaptureVersion:        CaptureVersion,
		State:                 "execution_started",
		HoldoutRunID:          holdoutTestRunID,
		Ordinal:               1,
		ExecutedAt:            "2026-07-25T00:00:00Z",
		CaptureID:             "55555555-5555-4555-8555-555555555555",
		GoldenSetID:           "golden",
		GoldenRawSHA256:       strings.Repeat("a", 64),
		GoldenContentSHA256:   strings.Repeat("b", 64),
		CurationRawSHA256:     strings.Repeat("6", 64),
		HumanReviewRawSHA256:  strings.Repeat("7", 64),
		SourceImportRawSHA256: strings.Repeat("8", 64),
		DevelopmentRawSHA256:  strings.Repeat("c", 64),
		ValidationRawSHA256:   strings.Repeat("d", 64),
		CandidateGenerationID: holdoutTestGenerationID,
		ArtifactManifestHash:  strings.Repeat("e", 64),
		ChunkProfileHash:      strings.Repeat("f", 64),
		RetrievalProfile: captureProviderConfiguration(CaptureRetrievalProvider{
			Profile: ragproviders.SiliconFlowRetrievalProfile,
		}),
		AnswerProviderID:         "SERVER_DEFAULT",
		AnswerModelID:            "answer-model",
		GenerationHeadRevision:   4,
		CorpusProjectionRevision: 298,
		ObservationOutputPath:    "/tmp/observations.json",
	}
}

func fmtCaseID(index int) string {
	const digits = "0123456789"
	return "case-" + string([]byte{
		digits[(index/100)%10],
		digits[(index/10)%10],
		digits[index%10],
	})
}
