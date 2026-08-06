package memorycapture

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	AccuracyFirstMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v12"
	AccuracyFirstMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_accuracy_only"
	AccuracyFirstMemoryJudgeArtifactName        = "fixed-memory-judge-accuracy-development.json"
)

// AccuracyFirstMemoryJudgeDevelopmentReport deliberately does not embed the
// historical latency-gated evaluation shape. Latency remains aggregate
// diagnostics, while Passed is controlled only by quality, safety, token, and
// slice gates under criteria-v3.
type AccuracyFirstMemoryJudgeDevelopmentReport struct {
	SchemaVersion             string                                        `json:"schemaVersion"`
	CorpusClass               string                                        `json:"corpusClass"`
	AdmissionMode             string                                        `json:"admissionMode"`
	PromotionEligible         bool                                          `json:"promotionEligible"`
	Split                     string                                        `json:"split"`
	CaseCount                 int                                           `json:"caseCount"`
	PolicyID                  string                                        `json:"policyId"`
	ProfileID                 string                                        `json:"profileId"`
	ConfigurationSHA256       string                                        `json:"configurationSha256"`
	ProviderEgressPolicy      string                                        `json:"providerEgressPolicy"`
	ProviderCostPolicy        string                                        `json:"providerCostPolicy"`
	ProviderCostAuthorized    bool                                          `json:"providerCostAuthorized"`
	JudgeProviderID           string                                        `json:"judgeProviderId"`
	JudgeProviderType         string                                        `json:"judgeProviderType"`
	JudgeBaseURLSHA256        string                                        `json:"judgeBaseUrlSha256"`
	JudgeModelID              string                                        `json:"judgeModelId"`
	JudgeAdapter              string                                        `json:"judgeAdapter"`
	JudgePromptVersion        string                                        `json:"judgePromptVersion"`
	JudgePromptSHA256         string                                        `json:"judgePromptSha256"`
	JudgeDecodingProfile      string                                        `json:"judgeDecodingProfile"`
	SelectionAlgorithm        string                                        `json:"selectionAlgorithm"`
	EvaluationCriteriaVersion string                                        `json:"evaluationCriteriaVersion"`
	EvaluationCriteria        memoryeval.AccuracyFirstCriteria              `json:"evaluationCriteria"`
	ExecutionPolicy           AccuracyFirstExecutionPolicy                  `json:"executionPolicy"`
	Passed                    bool                                          `json:"passed"`
	Evaluation                memoryeval.AccuracyFirstCalibrationEvaluation `json:"evaluation"`
	Diagnostics               CloudJudgeDevelopmentDiagnostics              `json:"diagnostics"`
	ProviderAttempts          AccuracyFirstProviderTelemetry                `json:"providerAttempts"`
	CostAuthority             CloudJudgeDevelopmentCostAuthority            `json:"costAuthority"`
}

func CaptureAccuracyFirstMemoryJudgeDevelopment(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
	authority ConfiguredCandidateJudgeProfileAuthority,
	profileID string,
	configurationSHA256 string,
	cost memoryeval.ProviderCosts,
) (CapturedProfile, error) {
	hybrid, hybridOK := provider.(*accuracyFirstHybridProvider)
	candidateJudge, judgeOK := judge.(*accuracyFirstCandidateJudge)
	if !validFixedMemoryJudgeAuthority(authority) || !hybridOK || !judgeOK ||
		hybrid.controller == nil || hybrid.controller != candidateJudge.controller ||
		hybrid.controller.judgeFailureDiagnostics {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, err
	}
	if err := validateSeedSplit(fullPool, seed.Cases, DevelopmentCalibrationSplit); err != nil {
		return CapturedProfile{}, err
	}
	return captureCandidateProfile(
		ctx,
		runtimeDB,
		index,
		seed.Cases,
		provider,
		usermemory.HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
}

func BuildAccuracyFirstMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (AccuracyFirstMemoryJudgeDevelopmentReport, []byte, error) {
	if profile.Profile.ReaderVersion != AccuracyFirstMemoryJudgeReaderVersion ||
		!validFixedMemoryJudgeAuthority(authority) ||
		ValidateAccuracyFirstMemoryJudgeCostAuthority(costBasis, authority) != nil ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs != costBasis.Candidate ||
		len(profile.Profile.ConfigurationSHA256) != 64 {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, err
	}
	executionPolicy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, err
	}
	criteria, err := memoryeval.MemoryJudgeAccuracyFirstCriteriaV3(pool.Corpus.Criteria)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	for _, trace := range profile.Calibration {
		if trace.FullObservation.HardCutoffApplied ||
			normalizeCalibrationCode(trace.AbstentionCode) == "HARD_CUTOFF" ||
			normalizeCalibrationCode(trace.ResultCode) == "HARD_CUTOFF" {
			return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, fmt.Errorf(
				"%w: accuracy-first trace contains a hard cutoff",
				ErrCaptureInvalid,
			)
		}
	}
	aggregate, err := aggregateCloudJudgeDevelopment(pool, profile, true)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, err
	}
	evaluation, err := memoryeval.EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy(
		aggregate.development,
		aggregate.ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateAccuracyFirstProviderTelemetry(
		telemetry,
		len(aggregate.development),
		aggregate.logicalJudgeRequests,
	); err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, err
	}
	authorityCost := costBasis.ConfiguredCandidateJudgeAuthority
	expectedInputTokenUpperBound := aggregate.logicalInputTokenUpperBound +
		uint64(telemetry.JudgeRetryInputTokenUpperBound)
	if uint64(telemetry.JudgeInputTokenUpperBound) != expectedInputTokenUpperBound {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, fmt.Errorf(
			"%w: accuracy-first Memory Judge input telemetry drifted",
			ErrCaptureInvalid,
		)
	}
	actualInputTokenUpperBound := uint64(telemetry.JudgeInputTokenUpperBound)
	actualOutputTokenUpperBound := uint64(telemetry.JudgeAttempts) *
		usermemory.HybridCandidateJudgeMaximumOutputTokens
	if telemetry.JudgeAttempts > authorityCost.RequestCount ||
		actualInputTokenUpperBound > authorityCost.MaximumInputTokens ||
		actualOutputTokenUpperBound > authorityCost.MaximumOutputTokens {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, fmt.Errorf(
			"%w: accuracy-first Memory Judge cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	report := AccuracyFirstMemoryJudgeDevelopmentReport{
		SchemaVersion:             AccuracyFirstMemoryJudgeReportSchemaVersion,
		CorpusClass:               memoryeval.RegressionCorpusClass,
		AdmissionMode:             AccuracyFirstMemoryJudgeAdmissionMode,
		PromotionEligible:         false,
		Split:                     DevelopmentCalibrationSplit,
		CaseCount:                 len(aggregate.development),
		PolicyID:                  usermemory.HybridRelevanceAccuracyFirstJudgePolicyID,
		ProfileID:                 profile.Profile.ID,
		ConfigurationSHA256:       profile.Profile.ConfigurationSHA256,
		ProviderEgressPolicy:      memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:        costBasis.ProviderCostPolicy,
		ProviderCostAuthorized:    true,
		JudgeProviderID:           authority.ProviderID,
		JudgeProviderType:         authority.ProviderType,
		JudgeBaseURLSHA256:        authority.BaseURLSHA256,
		JudgeModelID:              authority.ModelID,
		JudgeAdapter:              memoryjudge.ChatAdapterVersion,
		JudgePromptVersion:        usermemory.HybridCandidateJudgePromptVersion,
		JudgePromptSHA256:         usermemory.HybridCandidateJudgePromptSHA256,
		JudgeDecodingProfile:      usermemory.HybridCandidateJudgeDecodingProfile,
		SelectionAlgorithm:        cloudJudgeSelectionAlgorithm,
		EvaluationCriteriaVersion: memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3,
		EvaluationCriteria:        criteria,
		ExecutionPolicy:           executionPolicy,
		Passed:                    evaluation.Passed,
		Evaluation:                evaluation,
		Diagnostics:               aggregate.diagnostics,
		ProviderAttempts:          telemetry,
		CostAuthority: CloudJudgeDevelopmentCostAuthority{
			Unit:                                costBasis.Candidate.Unit,
			AuthorizedRequestCount:              authorityCost.RequestCount,
			ActualRequestCount:                  telemetry.JudgeAttempts,
			AuthorizedMaximumInputTokens:        authorityCost.MaximumInputTokens,
			ActualInputTokenUpperBound:          actualInputTokenUpperBound,
			AuthorizedMaximumOutputTokens:       authorityCost.MaximumOutputTokens,
			ActualOutputTokenUpperBound:         actualOutputTokenUpperBound,
			MaximumJudgeCostMicrounits:          authorityCost.MaximumCostMicrounits,
			MaximumMemoryProviderCostMicrounits: costBasis.Candidate.MemoryProviderCostMicrounits,
		},
	}
	if !validAccuracyFirstMemoryJudgeDevelopmentReport(report) {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	body, err := json.Marshal(report)
	if err != nil {
		return AccuracyFirstMemoryJudgeDevelopmentReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func validateAccuracyFirstProviderTelemetry(
	value AccuracyFirstProviderTelemetry,
	caseCount int,
	logicalJudgeRequests int,
) error {
	if value.JudgeAttemptFailureCategoryCounts != nil {
		return fmt.Errorf("%w: accuracy-first Provider telemetry schema drift", ErrCaptureInvalid)
	}
	return validateAccuracyFirstProviderTelemetryBase(
		value,
		caseCount,
		logicalJudgeRequests,
	)
}

func validateAccuracyFirstProviderTelemetryBase(
	value AccuracyFirstProviderTelemetry,
	caseCount int,
	logicalJudgeRequests int,
) error {
	return validateAccuracyFirstProviderTelemetryWithJudgeRetryLimit(
		value,
		caseCount,
		logicalJudgeRequests,
		1,
	)
}

func validateAccuracyFirstProviderTelemetryWithJudgeRetryLimit(
	value AccuracyFirstProviderTelemetry,
	caseCount int,
	logicalJudgeRequests int,
	maximumJudgeRetries int,
) error {
	if (caseCount != 300 && caseCount != 100) ||
		logicalJudgeRequests < 0 || logicalJudgeRequests > caseCount ||
		maximumJudgeRetries < 1 || maximumJudgeRetries > 2 ||
		value.PassageEmbeddingAttempts <= 0 ||
		value.PassageEmbeddingRetries < 0 ||
		value.PassageEmbeddingRetries*2 > value.PassageEmbeddingAttempts ||
		value.QueryEmbeddingAttempts != caseCount+value.QueryEmbeddingRetries ||
		value.QueryEmbeddingRetries < 0 || value.QueryEmbeddingRetries > caseCount ||
		value.RerankAttempts < value.RerankRetries || value.RerankRetries < 0 ||
		value.RerankRetries*2 > value.RerankAttempts ||
		value.RerankAttempts-value.RerankRetries > caseCount ||
		value.RerankAttempts-value.RerankRetries < logicalJudgeRequests ||
		value.JudgeAttempts != logicalJudgeRequests+value.JudgeRetries ||
		value.JudgeRetries < 0 ||
		value.JudgeRetries > logicalJudgeRequests*maximumJudgeRetries ||
		value.JudgeInputTokenUpperBound < 0 ||
		(value.JudgeAttempts == 0 && value.JudgeInputTokenUpperBound != 0) ||
		(value.JudgeAttempts > 0 && value.JudgeInputTokenUpperBound == 0) ||
		value.JudgeRetryInputTokenUpperBound < 0 ||
		value.JudgeRetryInputTokenUpperBound > value.JudgeInputTokenUpperBound ||
		(value.JudgeRetries == 0 && value.JudgeRetryInputTokenUpperBound != 0) ||
		(value.JudgeRetries > 0 && value.JudgeRetryInputTokenUpperBound == 0) ||
		value.InterCaseCooldownCount != caseCount-1 ||
		value.InterCaseCooldownMilliseconds !=
			(caseCount-1)*int(AccuracyFirstInterCaseCooldown/time.Millisecond) ||
		value.InterCaseCooldownElapsedMillis < 0 ||
		value.PassageEmbeddingLatency.SampleCount != value.PassageEmbeddingAttempts ||
		value.QueryEmbeddingLatency.SampleCount != value.QueryEmbeddingAttempts ||
		value.RerankLatency.SampleCount != value.RerankAttempts ||
		value.JudgeLatency.SampleCount != value.JudgeAttempts ||
		!validAccuracyFirstLatencyDiagnostics(value.PassageEmbeddingLatency) ||
		!validAccuracyFirstLatencyDiagnostics(value.QueryEmbeddingLatency) ||
		!validAccuracyFirstLatencyDiagnostics(value.RerankLatency) ||
		!validAccuracyFirstLatencyDiagnostics(value.JudgeLatency) {
		return fmt.Errorf("%w: accuracy-first Provider telemetry", ErrCaptureInvalid)
	}
	return nil
}

func validAccuracyFirstLatencyDiagnostics(value AccuracyFirstLatencyDiagnostics) bool {
	if value.SampleCount == 0 {
		return value == (AccuracyFirstLatencyDiagnostics{})
	}
	return value.SampleCount > 0 && value.TotalMilliseconds >= 0 &&
		value.P95LatencyMilliseconds >= 0 && value.P99LatencyMilliseconds >= 0 &&
		value.MaximumLatencyMilliseconds >= 0 &&
		value.P95LatencyMilliseconds <= value.P99LatencyMilliseconds &&
		value.P99LatencyMilliseconds <= value.MaximumLatencyMilliseconds &&
		value.TotalMilliseconds >= value.MaximumLatencyMilliseconds
}

func providerModeForProfileID(profileID string) (string, error) {
	switch profileID {
	case FakeCandidateProfileID:
		return ProviderModeFakeProtocol, nil
	case CandidateProfileID:
		return ProviderModeLiveSiliconFlow, nil
	default:
		return "", ErrCaptureInvalid
	}
}

func BuildAccuracyFirstMemoryJudgeDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report AccuracyFirstMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !validAccuracyFirstMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != AccuracyFirstMemoryJudgeArtifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion:       RelevanceRunManifestSchemaVersion,
		RunID:               runID,
		CaptureID:           captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       AccuracyFirstMemoryJudgeAdmissionMode,
		PromotionEligible:   false,
		CaptureMode:         CaptureModeAccuracyFirstMemoryJudge,
		Split:               DevelopmentCalibrationSplit,
		ProviderMode:        providerMode,
		ProfileID:           report.ProfileID,
		PolicyID:            report.PolicyID,
		ConfigurationSHA256: report.ConfigurationSHA256,
		Passed:              report.Passed,
		StartedAt:           startedAt.UTC().Format(time.RFC3339),
		CompletedAt:         completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256:     costBasisSHA256,
		ProviderCostPolicy:  report.ProviderCostPolicy,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, fmt.Errorf(
			"%w: encode accuracy-first Memory Judge run manifest",
			ErrCaptureInvalid,
		)
	}
	return manifest, append(body, '\n'), nil
}

func validAccuracyFirstMemoryJudgeDevelopmentReport(
	report AccuracyFirstMemoryJudgeDevelopmentReport,
) bool {
	if err := memoryeval.ValidateMemoryJudgeAccuracyFirstCriteriaV3(
		report.EvaluationCriteria,
	); err != nil {
		return false
	}
	providerMode, err := providerModeForProfileID(report.ProfileID)
	if err != nil {
		return false
	}
	expectedExecution, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedExecution {
		return false
	}
	if providerMode == ProviderModeFakeProtocol &&
		report.ProviderAttempts.InterCaseCooldownElapsedMillis != 0 {
		return false
	}
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    report.JudgeProviderID,
		ProviderType:  report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256,
		ModelID:       report.JudgeModelID,
	}
	return report.SchemaVersion == AccuracyFirstMemoryJudgeReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == AccuracyFirstMemoryJudgeAdmissionMode &&
		!report.PromotionEligible && report.Split == DevelopmentCalibrationSplit &&
		report.CaseCount == 300 &&
		report.PolicyID == usermemory.HybridRelevanceAccuracyFirstJudgePolicyID &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && report.Passed == report.Evaluation.Passed &&
		validFixedMemoryJudgeAuthority(authority) &&
		report.JudgeAdapter == memoryjudge.ChatAdapterVersion &&
		report.JudgePromptVersion == usermemory.HybridCandidateJudgePromptVersion &&
		report.JudgePromptSHA256 == usermemory.HybridCandidateJudgePromptSHA256 &&
		report.JudgeDecodingProfile == usermemory.HybridCandidateJudgeDecodingProfile &&
		report.SelectionAlgorithm == cloudJudgeSelectionAlgorithm &&
		report.EvaluationCriteriaVersion ==
			memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 &&
		len(report.ConfigurationSHA256) == 64 &&
		report.Diagnostics.FailureCodeCounts != nil &&
		report.Diagnostics.EmptyCandidateCaseCount+
			report.Diagnostics.JudgeCompletedCaseCount+
			report.Diagnostics.FailedCaseCount == report.CaseCount &&
		validateAccuracyFirstProviderTelemetry(
			report.ProviderAttempts,
			report.CaseCount,
			report.CostAuthority.ActualRequestCount-report.ProviderAttempts.JudgeRetries,
		) == nil &&
		report.CostAuthority.AuthorizedRequestCount == 600 &&
		report.CostAuthority.ActualRequestCount == report.ProviderAttempts.JudgeAttempts &&
		report.CostAuthority.ActualInputTokenUpperBound ==
			uint64(report.ProviderAttempts.JudgeInputTokenUpperBound) &&
		report.CostAuthority.ActualRequestCount <=
			report.CostAuthority.AuthorizedRequestCount &&
		report.CostAuthority.ActualInputTokenUpperBound <=
			report.CostAuthority.AuthorizedMaximumInputTokens &&
		report.CostAuthority.ActualOutputTokenUpperBound ==
			uint64(report.CostAuthority.ActualRequestCount)*
				usermemory.HybridCandidateJudgeMaximumOutputTokens &&
		report.CostAuthority.ActualOutputTokenUpperBound <=
			report.CostAuthority.AuthorizedMaximumOutputTokens &&
		report.CostAuthority.Unit != "" &&
		report.CostAuthority.MaximumMemoryProviderCostMicrounits >=
			report.CostAuthority.MaximumJudgeCostMicrounits
}
