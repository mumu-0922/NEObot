package ragevalcapture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const frozenHoldoutOrdinal = 1

func CaptureFrozenHoldout(
	ctx context.Context,
	input FrozenHoldoutInput,
) (rageval.PromotionObservationSet, error) {
	if err := validateFrozenHoldoutInput(input); err != nil {
		return rageval.PromotionObservationSet{}, err
	}
	status, err := input.Store.Status(ctx)
	if err != nil {
		return rageval.PromotionObservationSet{}, err
	}
	if err := validateCaptureStatus(input.CaptureInput, status); err != nil {
		return rageval.PromotionObservationSet{}, err
	}
	executedAt := input.Clock().UTC().Truncate(time.Second)
	frozenAt, _ := time.Parse(time.RFC3339, input.Golden.Lifecycle.FrozenAt)
	if executedAt.Before(frozenAt) {
		return rageval.PromotionObservationSet{}, errors.New(
			"frozen Holdout execution precedes the corpus freeze",
		)
	}
	if err := validateFrozenPreflights(input, status, executedAt); err != nil {
		return rageval.PromotionObservationSet{}, err
	}

	holdoutCases, complete, err := selectCaptureCases(
		input.Golden.Cases,
		[]string{"holdout"},
		"",
		0,
	)
	if err != nil || !complete || len(holdoutCases) != 100 {
		return rageval.PromotionObservationSet{}, errors.New(
			"frozen Holdout must contain exactly 100 cases",
		)
	}
	captureID := input.NewUUID()
	if !validCaptureUUID(captureID) {
		return rageval.PromotionObservationSet{}, errors.New(
			"frozen Holdout capture id is invalid",
		)
	}
	seal := HoldoutSeal{
		SchemaVersion:            HoldoutSealVersion,
		CaptureVersion:           CaptureVersion,
		State:                    "execution_started",
		HoldoutRunID:             input.Golden.Lifecycle.HoldoutRunID,
		Ordinal:                  frozenHoldoutOrdinal,
		ExecutedAt:               executedAt.Format(time.RFC3339),
		CaptureID:                captureID,
		GoldenSetID:              input.Golden.ID,
		GoldenRawSHA256:          input.GoldenRawSHA256,
		GoldenContentSHA256:      input.Golden.Lifecycle.FrozenContentSHA256,
		CurationRawSHA256:        input.CurationRawSHA256,
		HumanReviewRawSHA256:     input.ReviewRawSHA256,
		SourceImportRawSHA256:    input.ImportRawSHA256,
		DevelopmentRawSHA256:     input.Development.RawSHA256,
		ValidationRawSHA256:      input.Validation.RawSHA256,
		CandidateGenerationID:    status.CandidateGenerationID,
		ArtifactManifestHash:     status.CandidateArtifactManifestHash,
		ChunkProfileHash:         status.CandidateChunkProfileHash,
		RetrievalProfile:         captureProviderConfiguration(input.CandidateProvider),
		AnswerProviderID:         input.AnswerProviderID,
		AnswerModelID:            input.AnswerModelID,
		GenerationHeadRevision:   status.HeadRevision,
		CorpusProjectionRevision: status.CorpusProjectionRevision,
		ObservationOutputPath:    strings.TrimSpace(input.ObservationOutputPath),
	}
	if err := input.Seal(seal); err != nil {
		return rageval.PromotionObservationSet{}, fmt.Errorf(
			"create one-shot Holdout seal: %w",
			err,
		)
	}

	holdout, err := captureCandidateCases(ctx, input.CaptureInput, holdoutCases)
	if err != nil {
		return rageval.PromotionObservationSet{}, err
	}
	observedByID := make(map[string]rageval.PromotionCaseObservation, len(input.Golden.Cases))
	for _, source := range [][]PreflightObservation{
		input.Development.Report.Candidate.Cases,
		input.Validation.Report.Candidate.Cases,
		holdout,
	} {
		for _, item := range asPromotionObservations(source) {
			if _, duplicate := observedByID[item.CaseID]; duplicate {
				return rageval.PromotionObservationSet{}, fmt.Errorf(
					"duplicate frozen observation case %q",
					item.CaseID,
				)
			}
			observedByID[item.CaseID] = item
		}
	}
	ordered := make([]rageval.PromotionCaseObservation, 0, len(input.Golden.Cases))
	for _, goldenCase := range input.Golden.Cases {
		item, ok := observedByID[goldenCase.ID]
		if !ok {
			return rageval.PromotionObservationSet{}, fmt.Errorf(
				"missing frozen observation case %q",
				goldenCase.ID,
			)
		}
		ordered = append(ordered, item)
		delete(observedByID, goldenCase.ID)
	}
	if len(ordered) != 500 || len(observedByID) != 0 {
		return rageval.PromotionObservationSet{}, errors.New(
			"frozen observations do not exactly match the 500-case corpus",
		)
	}
	capturedAt := input.Clock().UTC().Truncate(time.Second)
	if capturedAt.Before(executedAt) {
		capturedAt = executedAt
	}
	result := rageval.PromotionObservationSet{
		SchemaVersion:        rageval.PromotionObservationSchemaVersion,
		GoldenSetID:          input.Golden.ID,
		GoldenCorpusSHA256:   input.Golden.Lifecycle.FrozenContentSHA256,
		CapturedAt:           capturedAt.Format(time.RFC3339),
		CaptureID:            captureID,
		ProfileRole:          "candidate",
		GenerationID:         status.CandidateGenerationID,
		ArtifactManifestHash: status.CandidateArtifactManifestHash,
		ProfileID:            string(ragproviders.RetrievalProfileSiliconFlow),
		HoldoutRun: rageval.PromotionHoldoutRun{
			ID:         input.Golden.Lifecycle.HoldoutRunID,
			Ordinal:    frozenHoldoutOrdinal,
			ExecutedAt: executedAt.Format(time.RFC3339),
		},
		Cases: ordered,
	}
	if err := validateCompleteObservationSet(result); err != nil {
		return rageval.PromotionObservationSet{}, err
	}
	return result, nil
}

func validateFrozenHoldoutInput(input FrozenHoldoutInput) error {
	base := input.CaptureInput
	if base.Store == nil ||
		!validCaptureProvider(base.CandidateProvider, ragproviders.SiliconFlowRetrievalProfile) ||
		base.Answerer == nil || base.Clock == nil || base.NewUUID == nil ||
		!validCaptureUUID(base.CandidateGenerationID) ||
		!validCaptureHash(base.CandidateManifestHash) ||
		strings.TrimSpace(base.AnswerProviderID) == "" ||
		strings.TrimSpace(base.AnswerModelID) == "" ||
		base.Concurrency < 1 || base.Concurrency > 16 ||
		len(base.CuratedByCaseID) != len(base.Golden.Cases) ||
		len(base.SourceByDocumentID) == 0 || input.Seal == nil ||
		strings.TrimSpace(input.ObservationOutputPath) == "" ||
		len(base.Splits) != 0 || strings.TrimSpace(base.CaseID) != "" ||
		base.MaximumCases != 0 ||
		!validCaptureHash(input.Development.RawSHA256) ||
		!validCaptureHash(input.Validation.RawSHA256) ||
		input.Development.RawSHA256 == input.Validation.RawSHA256 {
		return errors.New("frozen Holdout input is invalid")
	}
	return nil
}

func validateFrozenPreflights(
	input FrozenHoldoutInput,
	status GenerationStatus,
	executedAt time.Time,
) error {
	if err := validateFrozenPreflight(
		input,
		input.Development.Report,
		"development",
		status,
		executedAt,
	); err != nil {
		return fmt.Errorf("Development preflight: %w", err)
	}
	if err := validateFrozenPreflight(
		input,
		input.Validation.Report,
		"validation",
		status,
		executedAt,
	); err != nil {
		return fmt.Errorf("Validation preflight: %w", err)
	}
	development := input.Development.Report.Configuration
	validation := input.Validation.Report.Configuration
	development.Splits = nil
	validation.Splits = nil
	development.CaseID = ""
	validation.CaseID = ""
	if !reflect.DeepEqual(development, validation) {
		return errors.New("Development/Validation configuration drifted")
	}
	if input.Development.Report.Candidate.CaptureID ==
		input.Validation.Report.Candidate.CaptureID {
		return errors.New("Development/Validation capture ids are duplicated")
	}
	return nil
}

func validateFrozenPreflight(
	input FrozenHoldoutInput,
	report PreflightReport,
	split string,
	status GenerationStatus,
	executedAt time.Time,
) error {
	if report.SchemaVersion != PreflightSchemaVersion ||
		report.CaptureVersion != CaptureVersion || report.PromotionEligible ||
		!report.Complete || !validCaptureUUID(report.Candidate.CaptureID) ||
		report.Candidate.ProfileRole != "candidate" ||
		report.Holdout.State != "not_executed" ||
		report.Holdout.PrecommittedRunID != input.Golden.Lifecycle.HoldoutRunID {
		return errors.New("report header or Holdout binding is invalid")
	}
	capturedAt, err := time.Parse(time.RFC3339, report.CapturedAt)
	if err != nil {
		return errors.New("report capturedAt is invalid")
	}
	frozenAt, _ := time.Parse(time.RFC3339, input.Golden.Lifecycle.FrozenAt)
	if capturedAt.Before(frozenAt) || capturedAt.After(executedAt) {
		return errors.New("report is outside the frozen pre-Holdout window")
	}
	expectedInputs := PreflightInputHashes{
		GoldenRawSHA256:       input.GoldenRawSHA256,
		GoldenContentSHA256:   input.Golden.Lifecycle.FrozenContentSHA256,
		CurationRawSHA256:     input.CurationRawSHA256,
		HumanReviewRawSHA256:  input.ReviewRawSHA256,
		SourceImportRawSHA256: input.ImportRawSHA256,
	}
	if report.Inputs != expectedInputs {
		return errors.New("input hash binding drifted")
	}
	configuration := report.Configuration
	if len(configuration.Splits) != 1 || configuration.Splits[0] != split ||
		configuration.CaseID != "" ||
		configuration.CandidateRetrieval !=
			captureProviderConfiguration(input.CandidateProvider) ||
		configuration.AnswerProviderID != input.AnswerProviderID ||
		configuration.AnswerModelID != input.AnswerModelID ||
		configuration.ProviderMaximumAttempts != captureProviderAttempts ||
		configuration.ProviderInitialRetryMS != captureInitialRetryDelay.Milliseconds() ||
		configuration.ProviderMaximumRetryMS != captureMaximumRetryDelay.Milliseconds() ||
		configuration.CandidateLimit != captureCandidateLimit ||
		configuration.RerankLimit != captureRerankLimit ||
		configuration.FinalLimit != captureFinalLimit ||
		configuration.MaximumContextTokens != captureMaximumContextTokens ||
		configuration.Concurrency != input.Concurrency ||
		configuration.ScoringPolicy != captureScoringPolicy ||
		configuration.GenerationHeadRevision != status.HeadRevision ||
		configuration.CorpusProjectionRevision != status.CorpusProjectionRevision {
		return errors.New("configuration or revision binding drifted")
	}
	if report.Candidate.GenerationID != status.CandidateGenerationID ||
		report.Candidate.ArtifactManifestHash != status.CandidateArtifactManifestHash ||
		report.Candidate.ChunkProfileHash != status.CandidateChunkProfileHash {
		return errors.New("Candidate artifact binding drifted")
	}
	expectedCases, complete, err := selectCaptureCases(
		input.Golden.Cases,
		[]string{split},
		"",
		0,
	)
	if err != nil || !complete || len(expectedCases) != len(report.Candidate.Cases) {
		return errors.New("case coverage is incomplete")
	}
	for index, observation := range report.Candidate.Cases {
		if observation.CaseID != expectedCases[index].ID ||
			!validPreflightObservation(observation) {
			return fmt.Errorf("case %d is invalid or out of order", index)
		}
	}
	recomputed, err := summarizePreflightMetrics(
		expectedCases,
		report.Candidate.Cases,
		input.Golden.Criteria,
	)
	if err != nil || !reflect.DeepEqual(report.Candidate.Summary, recomputed.candidate) ||
		!reflect.DeepEqual(report.Slices, recomputed.slices) ||
		!reflect.DeepEqual(report.Budgets, recomputed.budgets) {
		return errors.New("report metrics are incomplete or inconsistent")
	}
	if len(rageval.PromotionAbsoluteFailures(report.Candidate.Summary.Metrics)) != 0 ||
		!report.Budgets.LatencyPassed || !report.Budgets.ContextTokenCostPassed {
		return errors.New("absolute quality or budget gate failed")
	}
	for _, name := range rageval.PromotionCriticalSlices() {
		slice, ok := report.Slices[name]
		if !ok || !slice.Evaluated || !slice.Passed || !slice.Integrity.Passed ||
			len(slice.Failures) != 0 {
			return fmt.Errorf("critical slice %q did not pass", name)
		}
	}
	return nil
}

func validPreflightObservation(item PreflightObservation) bool {
	if strings.TrimSpace(item.CaseID) == "" ||
		!validCaptureHash(item.AnswerSHA256) ||
		item.LatencyMilliseconds < 0 || item.ContextTokens < 0 ||
		item.ContextTokens > captureMaximumContextTokens ||
		!validCaptureRate(item.AnswerCorrectness) ||
		!validCaptureRate(item.Faithfulness) ||
		item.LatencyBreakdown.EmbedQueryMilliseconds < 0 ||
		item.LatencyBreakdown.FetchCandidatesMilliseconds < 0 ||
		item.LatencyBreakdown.HydrateEvidenceMilliseconds < 0 ||
		item.LatencyBreakdown.RerankMilliseconds < 0 ||
		item.LatencyBreakdown.PipelineTotalMilliseconds != item.LatencyMilliseconds ||
		item.AnswerUsage.PromptTokens < 0 || item.AnswerUsage.CompletionTokens < 0 ||
		item.AnswerUsage.TotalTokens < 0 ||
		len(item.RetrievedEvidenceIDs) > captureCandidateLimit ||
		len(item.FinalEvidenceIDs) > captureFinalLimit ||
		len(item.CitationEvidenceIDs) > captureFinalLimit ||
		hasBlankOrDuplicateCaptureIDs(item.RetrievedEvidenceIDs) ||
		hasBlankOrDuplicateCaptureIDs(item.FinalEvidenceIDs) ||
		hasBlankOrDuplicateCaptureIDs(item.CitationEvidenceIDs) {
		return false
	}
	if !item.Answered && (item.AnswerCorrectness != 0 ||
		item.Faithfulness != 0 || item.TableExactAnswer) {
		return false
	}
	if !captureIDsAreSubset(item.FinalEvidenceIDs, item.RetrievedEvidenceIDs) ||
		!captureIDsAreSubset(item.CitationEvidenceIDs, item.FinalEvidenceIDs) {
		return false
	}
	return true
}

func validCaptureRate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func hasBlankOrDuplicateCaptureIDs(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func captureIDsAreSubset(subset []string, superset []string) bool {
	allowed := make(map[string]struct{}, len(superset))
	for _, value := range superset {
		allowed[value] = struct{}{}
	}
	for _, value := range subset {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func validateCompleteObservationSet(value rageval.PromotionObservationSet) error {
	body, err := json.Marshal(value)
	if err != nil {
		return errors.New("encode complete frozen observations")
	}
	if _, err := rageval.DecodePromotionObservationSet(bytes.NewReader(body)); err != nil {
		return fmt.Errorf("validate complete frozen observations: %w", err)
	}
	return nil
}

func CreateHoldoutSeal(path string, seal HoldoutSeal) error {
	path = strings.TrimSpace(path)
	_, timestampErr := time.Parse(time.RFC3339, seal.ExecutedAt)
	if path == "" || seal.SchemaVersion != HoldoutSealVersion ||
		seal.CaptureVersion != CaptureVersion ||
		seal.State != "execution_started" || !validCaptureUUID(seal.HoldoutRunID) ||
		seal.Ordinal != frozenHoldoutOrdinal || timestampErr != nil ||
		!validCaptureUUID(seal.CaptureID) || strings.TrimSpace(seal.GoldenSetID) == "" ||
		!validCaptureHash(seal.GoldenRawSHA256) ||
		!validCaptureHash(seal.GoldenContentSHA256) ||
		!validCaptureHash(seal.CurationRawSHA256) ||
		!validCaptureHash(seal.HumanReviewRawSHA256) ||
		!validCaptureHash(seal.SourceImportRawSHA256) ||
		!validCaptureHash(seal.DevelopmentRawSHA256) ||
		!validCaptureHash(seal.ValidationRawSHA256) ||
		!validCaptureUUID(seal.CandidateGenerationID) ||
		!validCaptureHash(seal.ArtifactManifestHash) ||
		!validCaptureHash(seal.ChunkProfileHash) ||
		seal.RetrievalProfile != captureProviderConfiguration(
			CaptureRetrievalProvider{Profile: ragproviders.SiliconFlowRetrievalProfile},
		) || strings.TrimSpace(seal.AnswerProviderID) == "" ||
		strings.TrimSpace(seal.AnswerModelID) == "" ||
		seal.GenerationHeadRevision < 1 || seal.CorpusProjectionRevision < 1 ||
		strings.TrimSpace(seal.ObservationOutputPath) == "" {
		return errors.New("Holdout seal is invalid")
	}
	body, err := json.MarshalIndent(seal, "", "  ")
	if err != nil {
		return errors.New("encode Holdout seal")
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return errors.New("create Holdout seal directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("Holdout seal already exists; execution is permanently refused")
		}
		return errors.New("create Holdout seal")
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return errors.New("write Holdout seal")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return errors.New("sync Holdout seal")
	}
	if err := file.Close(); err != nil {
		return errors.New("close Holdout seal")
	}
	return nil
}

func WritePromotionObservationsExclusive(
	path string,
	observations rageval.PromotionObservationSet,
	pretty bool,
) error {
	if err := validateCompleteObservationSet(observations); err != nil {
		return err
	}
	return writeJSONExclusive(path, observations, pretty, "promotion observations")
}

func writeJSONExclusive(path string, value any, pretty bool, label string) error {
	path = strings.TrimSpace(path)
	label = strings.TrimSpace(label)
	if path == "" || label == "" {
		return errors.New("exclusive JSON output is invalid")
	}
	var body []byte
	var err error
	if pretty {
		body, err = json.MarshalIndent(value, "", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil {
		return fmt.Errorf("encode %s", label)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s output directory", label)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".exclusive-json-*.tmp")
	if err != nil {
		return fmt.Errorf("create %s temporary file", label)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure %s temporary file", label)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s", label)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync %s", label)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s", label)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s output already exists", label)
		}
		return fmt.Errorf("publish %s", label)
	}
	return nil
}
