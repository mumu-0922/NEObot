package memorycapture

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	JudgeFailureDiagnosticReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v13"
	JudgeFailureDiagnosticAdmissionMode       = "development_fixed_memory_judge_failure_diagnostic_only"
	JudgeFailureDiagnosticCompletenessPolicy  = "attempt_terminal_reconciled_fail_closed_v1"
	JudgeFailureDiagnosticArtifactName        = "fixed-memory-judge-failure-diagnostic-development.json"
)

type JudgeFailureDiagnosticProviderTelemetry struct {
	AccuracyFirstProviderTelemetry
	JudgeAttemptFailureCategoryCounts map[string]int `json:"judgeAttemptFailureCategoryCounts"`
}

type JudgeFailureDiagnosticDiagnostics struct {
	CloudJudgeDevelopmentDiagnostics
	JudgeTerminalFailureCategoryCounts map[string]int `json:"judgeTerminalFailureCategoryCounts"`
}

// JudgeFailureDiagnosticDevelopmentReport is measurement-only. Evaluation
// remains visible as aggregate context, but this schema can never select a
// policy, pass a promotion gate, or authorize Validation.
type JudgeFailureDiagnosticDevelopmentReport struct {
	SchemaVersion                    string                                        `json:"schemaVersion"`
	CorpusClass                      string                                        `json:"corpusClass"`
	AdmissionMode                    string                                        `json:"admissionMode"`
	PromotionEligible                bool                                          `json:"promotionEligible"`
	PolicySelected                   bool                                          `json:"policySelected"`
	DiagnosticComplete               bool                                          `json:"diagnosticComplete"`
	Split                            string                                        `json:"split"`
	CaseCount                        int                                           `json:"caseCount"`
	PolicyID                         string                                        `json:"policyId"`
	ProfileID                        string                                        `json:"profileId"`
	ConfigurationSHA256              string                                        `json:"configurationSha256"`
	ProviderEgressPolicy             string                                        `json:"providerEgressPolicy"`
	ProviderCostPolicy               string                                        `json:"providerCostPolicy"`
	ProviderCostAuthorized           bool                                          `json:"providerCostAuthorized"`
	JudgeProviderID                  string                                        `json:"judgeProviderId"`
	JudgeProviderType                string                                        `json:"judgeProviderType"`
	JudgeBaseURLSHA256               string                                        `json:"judgeBaseUrlSha256"`
	JudgeModelID                     string                                        `json:"judgeModelId"`
	JudgeAdapter                     string                                        `json:"judgeAdapter"`
	JudgePromptVersion               string                                        `json:"judgePromptVersion"`
	JudgePromptSHA256                string                                        `json:"judgePromptSha256"`
	JudgeDecodingProfile             string                                        `json:"judgeDecodingProfile"`
	FailureTaxonomyVersion           string                                        `json:"failureTaxonomyVersion"`
	FailureTaxonomySHA256            string                                        `json:"failureTaxonomySha256"`
	DiagnosticCompleteness           string                                        `json:"diagnosticCompleteness"`
	SelectionAlgorithm               string                                        `json:"selectionAlgorithm"`
	EvaluationCriteriaVersion        string                                        `json:"evaluationCriteriaVersion"`
	EvaluationCriteria               memoryeval.AccuracyFirstCriteria              `json:"evaluationCriteria"`
	ExecutionPolicy                  AccuracyFirstExecutionPolicy                  `json:"executionPolicy"`
	Passed                           bool                                          `json:"passed"`
	Evaluation                       memoryeval.AccuracyFirstCalibrationEvaluation `json:"evaluation"`
	Diagnostics                      JudgeFailureDiagnosticDiagnostics             `json:"diagnostics"`
	ProviderAttempts                 JudgeFailureDiagnosticProviderTelemetry       `json:"providerAttempts"`
	CostAuthority                    CloudJudgeDevelopmentCostAuthority            `json:"costAuthority"`
	NegativePolicyQueryGuardRequired bool                                          `json:"negativePolicyQueryGuardRequired,omitempty"`
	NegativePolicyQueryGuardVersion  string                                        `json:"negativePolicyQueryGuardVersion,omitempty"`
	NegativePolicyQueryGuardSHA256   string                                        `json:"negativePolicyQueryGuardSha256,omitempty"`
	RelevancePolicyDescriptorSHA256  string                                        `json:"relevancePolicyDescriptorSha256,omitempty"`
}

func CaptureJudgeFailureDiagnosticDevelopment(
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
		!hybrid.controller.judgeFailureDiagnostics {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, err
	}
	if err := validateSeedSplit(fullPool, seed.Cases, DevelopmentCalibrationSplit); err != nil {
		return CapturedProfile{}, err
	}
	profile, err := captureCandidateProfile(
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
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = JudgeFailureDiagnosticReaderVersion
	return profile, nil
}

func BuildJudgeFailureDiagnosticDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (JudgeFailureDiagnosticDevelopmentReport, []byte, error) {
	if profile.Profile.ReaderVersion != JudgeFailureDiagnosticReaderVersion ||
		!validFixedMemoryJudgeAuthority(authority) ||
		ValidateAccuracyFirstMemoryJudgeCostAuthority(costBasis, authority) != nil ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs != costBasis.Candidate ||
		len(profile.Profile.ConfigurationSHA256) != 64 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	executionPolicy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	criteria, err := memoryeval.MemoryJudgeAccuracyFirstCriteriaV3(pool.Corpus.Criteria)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	for _, trace := range profile.Calibration {
		if trace.FullObservation.HardCutoffApplied ||
			normalizeCalibrationCode(trace.AbstentionCode) == "HARD_CUTOFF" ||
			normalizeCalibrationCode(trace.ResultCode) == "HARD_CUTOFF" {
			return JudgeFailureDiagnosticDevelopmentReport{}, nil, fmt.Errorf(
				"%w: Judge failure diagnostic trace contains a hard cutoff",
				ErrCaptureInvalid,
			)
		}
	}
	aggregate, err := aggregateCloudJudgeDevelopment(pool, profile, true)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	terminalCounts, terminalAttemptFailures, err :=
		aggregateJudgeTerminalFailureCategories(profile.Calibration, aggregate.diagnostics)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateJudgeFailureDiagnosticTelemetry(
		telemetry,
		len(aggregate.development),
		aggregate.logicalJudgeRequests,
		terminalAttemptFailures,
	); err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	evaluation, err := memoryeval.EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy(
		aggregate.development,
		aggregate.ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	expectedInputTokenUpperBound := aggregate.logicalInputTokenUpperBound +
		uint64(telemetry.JudgeRetryInputTokenUpperBound)
	if uint64(telemetry.JudgeInputTokenUpperBound) != expectedInputTokenUpperBound {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, fmt.Errorf(
			"%w: Judge failure diagnostic input telemetry drifted",
			ErrCaptureInvalid,
		)
	}
	authorityCost := costBasis.ConfiguredCandidateJudgeAuthority
	actualInputTokenUpperBound := uint64(telemetry.JudgeInputTokenUpperBound)
	actualOutputTokenUpperBound := uint64(telemetry.JudgeAttempts) *
		usermemory.HybridCandidateJudgeMaximumOutputTokens
	if telemetry.JudgeAttempts > authorityCost.RequestCount ||
		actualInputTokenUpperBound > authorityCost.MaximumInputTokens ||
		actualOutputTokenUpperBound > authorityCost.MaximumOutputTokens {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, fmt.Errorf(
			"%w: Judge failure diagnostic cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	report := JudgeFailureDiagnosticDevelopmentReport{
		SchemaVersion:             JudgeFailureDiagnosticReportSchemaVersion,
		CorpusClass:               memoryeval.RegressionCorpusClass,
		AdmissionMode:             JudgeFailureDiagnosticAdmissionMode,
		PromotionEligible:         false,
		PolicySelected:            false,
		DiagnosticComplete:        true,
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
		FailureTaxonomyVersion:    memoryjudge.FailureTaxonomyVersion,
		FailureTaxonomySHA256:     memoryjudge.FailureTaxonomySHA256,
		DiagnosticCompleteness:    JudgeFailureDiagnosticCompletenessPolicy,
		SelectionAlgorithm:        cloudJudgeSelectionAlgorithm,
		EvaluationCriteriaVersion: memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3,
		EvaluationCriteria:        criteria,
		ExecutionPolicy:           executionPolicy,
		Passed:                    false,
		Evaluation:                evaluation,
		Diagnostics: JudgeFailureDiagnosticDiagnostics{
			CloudJudgeDevelopmentDiagnostics:   aggregate.diagnostics,
			JudgeTerminalFailureCategoryCounts: terminalCounts,
		},
		ProviderAttempts: JudgeFailureDiagnosticProviderTelemetry{
			AccuracyFirstProviderTelemetry: telemetry,
			JudgeAttemptFailureCategoryCounts: cloneDiagnosticCounts(
				telemetry.JudgeAttemptFailureCategoryCounts,
			),
		},
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
	if !validJudgeFailureDiagnosticDevelopmentReport(report) {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	body, err := json.Marshal(report)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func aggregateJudgeTerminalFailureCategories(
	traces []CandidateCalibrationTrace,
	diagnostics CloudJudgeDevelopmentDiagnostics,
) (map[string]int, int, error) {
	counts := make(map[string]int)
	attemptFailures := 0
	for _, trace := range traces {
		code := normalizeCalibrationCode(trace.AbstentionCode)
		if code == "NONE" {
			code = normalizeCalibrationCode(trace.ResultCode)
		}
		if code != "CANDIDATE_JUDGE_FAILED" {
			if trace.CloudJudgeFailureCategory != "" {
				return nil, 0, ErrCaptureInvalid
			}
			continue
		}
		category := trace.CloudJudgeFailureCategory
		if !memoryjudge.ValidFailureCategory(category) {
			return nil, 0, ErrCaptureInvalid
		}
		counts[category]++
		if memoryjudge.AttemptFailureCategory(category) {
			attemptFailures++
		}
	}
	if sumDiagnosticCounts(counts) != diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"] {
		return nil, 0, ErrCaptureInvalid
	}
	return counts, attemptFailures, nil
}

func validateJudgeFailureDiagnosticTelemetry(
	value AccuracyFirstProviderTelemetry,
	caseCount int,
	logicalJudgeRequests int,
	terminalAttemptFailures int,
) error {
	if err := validateAccuracyFirstProviderTelemetryBase(
		value,
		caseCount,
		logicalJudgeRequests,
	); err != nil || value.JudgeAttemptFailureCategoryCounts == nil ||
		terminalAttemptFailures < 0 {
		return fmt.Errorf("%w: Judge failure diagnostic Provider telemetry", ErrCaptureInvalid)
	}
	if !validJudgeAttemptFailureCategoryCounts(
		value.JudgeAttemptFailureCategoryCounts,
	) ||
		sumDiagnosticCounts(value.JudgeAttemptFailureCategoryCounts) !=
			value.JudgeRetries+terminalAttemptFailures {
		return fmt.Errorf("%w: Judge failure diagnostic attempt reconciliation", ErrCaptureInvalid)
	}
	return nil
}

func validJudgeAttemptFailureCategoryCounts(counts map[string]int) bool {
	if counts == nil {
		return false
	}
	for category, count := range counts {
		if count <= 0 || !memoryjudge.AttemptFailureCategory(category) {
			return false
		}
	}
	return true
}

func validJudgeFailureCategoryCounts(counts map[string]int) bool {
	if counts == nil {
		return false
	}
	for category, count := range counts {
		if count <= 0 || !memoryjudge.ValidFailureCategory(category) {
			return false
		}
	}
	return true
}

func cloneDiagnosticCounts(counts map[string]int) map[string]int {
	if counts == nil {
		return nil
	}
	cloned := make(map[string]int, len(counts))
	for category, count := range counts {
		cloned[category] = count
	}
	return cloned
}

func judgeFailureTaxonomySHA256() string {
	body, err := json.Marshal(memoryjudge.FailureCategories())
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validJudgeFailureDiagnosticDevelopmentReport(
	report JudgeFailureDiagnosticDevelopmentReport,
) bool {
	if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 ||
		memoryeval.ValidateMemoryJudgeAccuracyFirstCriteriaV3(report.EvaluationCriteria) != nil {
		return false
	}
	providerMode, err := providerModeForProfileID(report.ProfileID)
	if err != nil {
		return false
	}
	expectedExecution, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedExecution ||
		(providerMode == ProviderModeFakeProtocol &&
			report.ProviderAttempts.InterCaseCooldownElapsedMillis != 0) {
		return false
	}
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID: report.JudgeProviderID, ProviderType: report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256, ModelID: report.JudgeModelID,
	}
	terminalAttemptFailures := 0
	if !validJudgeFailureCategoryCounts(
		report.Diagnostics.JudgeTerminalFailureCategoryCounts,
	) {
		return false
	}
	for category, count := range report.Diagnostics.JudgeTerminalFailureCategoryCounts {
		if memoryjudge.AttemptFailureCategory(category) {
			terminalAttemptFailures += count
		}
	}
	telemetry := report.ProviderAttempts.AccuracyFirstProviderTelemetry
	telemetry.JudgeAttemptFailureCategoryCounts =
		report.ProviderAttempts.JudgeAttemptFailureCategoryCounts
	logicalJudgeRequests := report.Diagnostics.JudgeCompletedCaseCount +
		report.Diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"]
	return report.SchemaVersion == JudgeFailureDiagnosticReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == JudgeFailureDiagnosticAdmissionMode &&
		!report.PromotionEligible && !report.PolicySelected &&
		report.DiagnosticComplete && !report.Passed &&
		report.Split == DevelopmentCalibrationSplit && report.CaseCount == 300 &&
		report.PolicyID == usermemory.HybridRelevanceAccuracyFirstJudgePolicyID &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && validFixedMemoryJudgeAuthority(authority) &&
		report.JudgeAdapter == memoryjudge.ChatAdapterVersion &&
		report.JudgePromptVersion == usermemory.HybridCandidateJudgePromptVersion &&
		report.JudgePromptSHA256 == usermemory.HybridCandidateJudgePromptSHA256 &&
		report.JudgeDecodingProfile == usermemory.HybridCandidateJudgeDecodingProfile &&
		report.FailureTaxonomyVersion == memoryjudge.FailureTaxonomyVersion &&
		report.FailureTaxonomySHA256 == memoryjudge.FailureTaxonomySHA256 &&
		report.DiagnosticCompleteness == JudgeFailureDiagnosticCompletenessPolicy &&
		report.SelectionAlgorithm == cloudJudgeSelectionAlgorithm &&
		report.EvaluationCriteriaVersion ==
			memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 &&
		len(report.ConfigurationSHA256) == 64 &&
		report.Diagnostics.FailureCodeCounts != nil &&
		report.Diagnostics.EmptyCandidateCaseCount+
			report.Diagnostics.JudgeCompletedCaseCount+
			report.Diagnostics.FailedCaseCount == report.CaseCount &&
		sumDiagnosticCounts(report.Diagnostics.JudgeTerminalFailureCategoryCounts) ==
			report.Diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"] &&
		validateJudgeFailureDiagnosticTelemetry(
			telemetry,
			report.CaseCount,
			logicalJudgeRequests,
			terminalAttemptFailures,
		) == nil &&
		report.CostAuthority.AuthorizedRequestCount == 600 &&
		report.CostAuthority.ActualRequestCount == report.ProviderAttempts.JudgeAttempts &&
		report.CostAuthority.ActualInputTokenUpperBound ==
			uint64(report.ProviderAttempts.JudgeInputTokenUpperBound) &&
		report.CostAuthority.ActualRequestCount <= report.CostAuthority.AuthorizedRequestCount &&
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
