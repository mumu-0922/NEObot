package memorycapture

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	ProductionBufferedMemoryJudgeValidationReportSchemaVersion = "neo-chat.memory-regression-relevance-validation.v18"
	ProductionBufferedMemoryJudgeValidationRunSchemaVersion    = "neo-chat.memory-regression-relevance-validation-run.v18"
	ProductionBufferedMemoryJudgeValidationAdmissionMode       = "frozen_production_fixed_memory_judge_negative_guard_buffered_validation_only"
	ProductionBufferedMemoryJudgeValidationArtifactName        = "fixed-memory-judge-negative-guard-buffered-production-validation.json"
)

// ProductionBufferedMemoryJudgeValidationReport is aggregate-only frozen Validation
// evidence. Passing does not grant Release or promotion authority.
type ProductionBufferedMemoryJudgeValidationReport struct {
	SchemaVersion                   string                                        `json:"schemaVersion"`
	CorpusClass                     string                                        `json:"corpusClass"`
	AdmissionMode                   string                                        `json:"admissionMode"`
	PromotionEligible               bool                                          `json:"promotionEligible"`
	ReleaseEligible                 bool                                          `json:"releaseEligible"`
	PolicySelected                  bool                                          `json:"policySelected"`
	ExecutionComplete               bool                                          `json:"executionComplete"`
	EvidenceClass                   string                                        `json:"evidenceClass"`
	Split                           string                                        `json:"split"`
	CaseCount                       int                                           `json:"caseCount"`
	PolicyID                        string                                        `json:"policyId"`
	ProfileID                       string                                        `json:"profileId"`
	ConfigurationSHA256             string                                        `json:"configurationSha256"`
	ValidationCaseOrderSHA256       string                                        `json:"validationCaseOrderSha256"`
	CorpusRawSHA256                 string                                        `json:"corpusRawSha256"`
	ProductionRelevancePolicySHA256 string                                        `json:"productionRelevancePolicySha256"`
	MemoryReadIntentPolicyVersion   string                                        `json:"memoryReadIntentPolicyVersion"`
	MemoryReadIntentPolicySHA256    string                                        `json:"memoryReadIntentPolicySha256"`
	NegativePolicyQueryGuardVersion string                                        `json:"negativePolicyQueryGuardVersion"`
	NegativePolicyQueryGuardSHA256  string                                        `json:"negativePolicyQueryGuardSha256"`
	ProviderEgressPolicy            string                                        `json:"providerEgressPolicy"`
	ProviderCostPolicy              string                                        `json:"providerCostPolicy"`
	ProviderCostAuthorized          bool                                          `json:"providerCostAuthorized"`
	JudgeProviderID                 string                                        `json:"judgeProviderId"`
	JudgeProviderType               string                                        `json:"judgeProviderType"`
	JudgeBaseURLSHA256              string                                        `json:"judgeBaseUrlSha256"`
	JudgeModelID                    string                                        `json:"judgeModelId"`
	JudgeAdapter                    string                                        `json:"judgeAdapter"`
	JudgePromptVersion              string                                        `json:"judgePromptVersion"`
	JudgePromptSHA256               string                                        `json:"judgePromptSha256"`
	JudgeDecodingProfile            string                                        `json:"judgeDecodingProfile"`
	FailureTaxonomyVersion          string                                        `json:"failureTaxonomyVersion"`
	FailureTaxonomySHA256           string                                        `json:"failureTaxonomySha256"`
	DiagnosticCompleteness          string                                        `json:"diagnosticCompleteness"`
	SelectionAlgorithm              string                                        `json:"selectionAlgorithm"`
	EvaluationCriteriaVersion       string                                        `json:"evaluationCriteriaVersion"`
	EvaluationCriteriaSHA256        string                                        `json:"evaluationCriteriaSha256"`
	EvaluationCriteria              memoryeval.AccuracyFirstCriteria              `json:"evaluationCriteria"`
	ExecutionPolicy                 AccuracyFirstExecutionPolicy                  `json:"executionPolicy"`
	Passed                          bool                                          `json:"passed"`
	Outcome                         ProductionValidationOutcome                   `json:"outcome"`
	Evaluation                      memoryeval.AccuracyFirstCalibrationEvaluation `json:"evaluation"`
	Diagnostics                     JudgeFailureDiagnosticDiagnostics             `json:"diagnostics"`
	ProviderAttempts                JudgeFailureDiagnosticProviderTelemetry       `json:"providerAttempts"`
	CostAuthority                   CloudJudgeDevelopmentCostAuthority            `json:"costAuthority"`
}

type ProductionBufferedMemoryJudgeValidationRunManifest struct {
	SchemaVersion                   string                      `json:"schemaVersion"`
	RunID                           string                      `json:"runId"`
	CaptureID                       string                      `json:"captureId"`
	CorpusClass                     string                      `json:"corpusClass"`
	AdmissionMode                   string                      `json:"admissionMode"`
	PromotionEligible               bool                        `json:"promotionEligible"`
	ReleaseEligible                 bool                        `json:"releaseEligible"`
	CaptureMode                     string                      `json:"captureMode"`
	Split                           string                      `json:"split"`
	ProviderMode                    string                      `json:"providerMode"`
	EvidenceClass                   string                      `json:"evidenceClass"`
	ProfileID                       string                      `json:"profileId"`
	PolicyID                        string                      `json:"policyId"`
	ConfigurationSHA256             string                      `json:"configurationSha256"`
	ValidationCaseOrderSHA256       string                      `json:"validationCaseOrderSha256"`
	ProductionRelevancePolicySHA256 string                      `json:"productionRelevancePolicySha256"`
	MemoryReadIntentPolicySHA256    string                      `json:"memoryReadIntentPolicySha256"`
	EvaluationCriteriaSHA256        string                      `json:"evaluationCriteriaSha256"`
	Passed                          bool                        `json:"passed"`
	Outcome                         ProductionValidationOutcome `json:"outcome"`
	StartedAt                       string                      `json:"startedAt"`
	CompletedAt                     string                      `json:"completedAt"`
	CostBasisSHA256                 string                      `json:"costBasisSha256"`
	ProviderCostPolicy              string                      `json:"providerCostPolicy"`
	Inputs                          RunInputHashes              `json:"inputs"`
	Artifacts                       []RunArtifactManifest       `json:"artifacts"`
}

func BuildProductionBufferedMemoryJudgeValidationReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	config ProfileConfig,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (ProductionBufferedMemoryJudgeValidationReport, []byte, error) {
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	costBasisSHA256, err := CostBasisSHA256(costBasis)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	caseOrderSHA256, err := validationCaseOrderSHA256(pool)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	criteria, err := memoryeval.MemoryJudgeAccuracyFirstCriteriaV3(pool.Corpus.Criteria)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	criteriaSHA256, err := productionValidationCriteriaSHA256(pool.Corpus.Criteria)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	policySHA256, err := productionBufferedRelevancePolicySHA256()
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	executionPolicy, err := ProductionBufferedMemoryJudgeValidationExecutionPolicy(providerMode)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	if profile.Profile.ReaderVersion != ProductionBufferedMemoryJudgeValidationReaderVersion ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Profile.ConfigurationSHA256 != configurationSHA256 ||
		profile.Costs != costBasis.Candidate ||
		!validProductionBufferedMemoryJudgeValidationConfig(
			config,
			providerMode,
			authority,
			costBasisSHA256,
			caseOrderSHA256,
			criteriaSHA256,
			policySHA256,
			executionPolicy,
		) ||
		ValidateProductionBufferedMemoryJudgeValidationCostAuthority(costBasis, authority) != nil ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	for _, trace := range profile.Calibration {
		if trace.FullObservation.HardCutoffApplied ||
			normalizeCalibrationCode(trace.AbstentionCode) == "HARD_CUTOFF" ||
			normalizeCalibrationCode(trace.ResultCode) == "HARD_CUTOFF" {
			return ProductionBufferedMemoryJudgeValidationReport{}, nil, fmt.Errorf(
				"%w: production Validation trace contains a hard cutoff",
				ErrCaptureInvalid,
			)
		}
	}
	aggregate, err := aggregateCloudJudgeCaptureSplit(
		pool,
		profile,
		FrozenValidationSplit,
		100,
		true,
		true,
	)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	terminalCounts, terminalAttemptFailures, err :=
		aggregateJudgeTerminalFailureCategories(profile.Calibration, aggregate.diagnostics)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateTransportStableMemoryJudgeTelemetry(
		telemetry,
		len(aggregate.development),
		aggregate.logicalJudgeRequests,
		terminalAttemptFailures,
	); err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	evaluation, err := memoryeval.EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy(
		aggregate.development,
		aggregate.ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, err
	}
	expectedInputTokenUpperBound := aggregate.logicalInputTokenUpperBound +
		uint64(telemetry.JudgeRetryInputTokenUpperBound)
	if uint64(telemetry.JudgeInputTokenUpperBound) != expectedInputTokenUpperBound {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, fmt.Errorf(
			"%w: production Validation input telemetry drifted",
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
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, fmt.Errorf(
			"%w: production Validation cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	evidenceClass := ProductionValidationEvidenceLive
	if providerMode == ProviderModeFakeProtocol {
		evidenceClass = ProductionValidationEvidenceFake
	}
	outcome := productionValidationOutcome(
		evidenceClass,
		evaluation,
		aggregate.diagnostics.FailedCaseCount,
		pool.Corpus.Criteria.MaximumFalseInjectionRate,
	)
	passed := evidenceClass == ProductionValidationEvidenceLive &&
		evaluation.Passed && aggregate.diagnostics.FailedCaseCount == 0 &&
		outcome.Severity == ProductionValidationSeverityNone
	report := ProductionBufferedMemoryJudgeValidationReport{
		SchemaVersion:                   ProductionBufferedMemoryJudgeValidationReportSchemaVersion,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   ProductionBufferedMemoryJudgeValidationAdmissionMode,
		PromotionEligible:               false,
		ReleaseEligible:                 false,
		PolicySelected:                  false,
		ExecutionComplete:               true,
		EvidenceClass:                   evidenceClass,
		Split:                           FrozenValidationSplit,
		CaseCount:                       len(aggregate.development),
		PolicyID:                        usermemory.HybridRelevanceNegativePolicyGuardProductionPolicyID,
		ProfileID:                       profile.Profile.ID,
		ConfigurationSHA256:             profile.Profile.ConfigurationSHA256,
		ValidationCaseOrderSHA256:       caseOrderSHA256,
		CorpusRawSHA256:                 config.CorpusRawSHA256,
		ProductionRelevancePolicySHA256: policySHA256,
		MemoryReadIntentPolicyVersion:   chat.MemoryReadIntentPolicyVersion,
		MemoryReadIntentPolicySHA256:    chat.MemoryReadIntentPolicySHA256,
		NegativePolicyQueryGuardVersion: usermemory.NegativePolicyQueryGuardVersion,
		NegativePolicyQueryGuardSHA256:  usermemory.NegativePolicyQueryGuardSHA256,
		ProviderEgressPolicy:            memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:              costBasis.ProviderCostPolicy,
		ProviderCostAuthorized:          true,
		JudgeProviderID:                 authority.ProviderID,
		JudgeProviderType:               authority.ProviderType,
		JudgeBaseURLSHA256:              authority.BaseURLSHA256,
		JudgeModelID:                    authority.ModelID,
		JudgeAdapter:                    memoryjudge.BufferedChatAdapterVersion,
		JudgePromptVersion:              usermemory.HybridCandidateJudgePromptVersion,
		JudgePromptSHA256:               usermemory.HybridCandidateJudgePromptSHA256,
		JudgeDecodingProfile:            usermemory.HybridCandidateJudgeDecodingProfile,
		FailureTaxonomyVersion:          memoryjudge.FailureTaxonomyVersion,
		FailureTaxonomySHA256:           memoryjudge.FailureTaxonomySHA256,
		DiagnosticCompleteness:          JudgeFailureDiagnosticCompletenessPolicy,
		SelectionAlgorithm:              cloudJudgeSelectionAlgorithm,
		EvaluationCriteriaVersion:       memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3,
		EvaluationCriteriaSHA256:        criteriaSHA256,
		EvaluationCriteria:              criteria,
		ExecutionPolicy:                 executionPolicy,
		Passed:                          passed,
		Outcome:                         outcome,
		Evaluation:                      evaluation,
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
	if !validProductionBufferedMemoryJudgeValidationReport(report) {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	body, err := json.Marshal(report)
	if err != nil {
		return ProductionBufferedMemoryJudgeValidationReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func BuildProductionBufferedMemoryJudgeValidationRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report ProductionBufferedMemoryJudgeValidationReport,
	artifacts []Artifact,
) (ProductionBufferedMemoryJudgeValidationRunManifest, []byte, error) {
	if !validProductionBufferedMemoryJudgeValidationReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		protected.CorpusRawSHA256 != report.CorpusRawSHA256 {
		return ProductionBufferedMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return ProductionBufferedMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || expectedProfileID != report.ProfileID {
		return ProductionBufferedMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != ProductionBufferedMemoryJudgeValidationArtifactName {
		return ProductionBufferedMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := ProductionBufferedMemoryJudgeValidationRunManifest{
		SchemaVersion:                   ProductionBufferedMemoryJudgeValidationRunSchemaVersion,
		RunID:                           runID,
		CaptureID:                       captureID,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   ProductionBufferedMemoryJudgeValidationAdmissionMode,
		PromotionEligible:               false,
		ReleaseEligible:                 false,
		CaptureMode:                     CaptureModeProductionBufferedMemoryJudgeValidation,
		Split:                           FrozenValidationSplit,
		ProviderMode:                    providerMode,
		EvidenceClass:                   report.EvidenceClass,
		ProfileID:                       report.ProfileID,
		PolicyID:                        report.PolicyID,
		ConfigurationSHA256:             report.ConfigurationSHA256,
		ValidationCaseOrderSHA256:       report.ValidationCaseOrderSHA256,
		ProductionRelevancePolicySHA256: report.ProductionRelevancePolicySHA256,
		MemoryReadIntentPolicySHA256:    report.MemoryReadIntentPolicySHA256,
		EvaluationCriteriaSHA256:        report.EvaluationCriteriaSHA256,
		Passed:                          report.Passed,
		Outcome:                         report.Outcome,
		StartedAt:                       startedAt.UTC().Format(time.RFC3339),
		CompletedAt:                     completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256:                 costBasisSHA256,
		ProviderCostPolicy:              report.ProviderCostPolicy,
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
		return ProductionBufferedMemoryJudgeValidationRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}

func validProductionBufferedMemoryJudgeValidationConfig(
	config ProfileConfig,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasisSHA256 string,
	caseOrderSHA256 string,
	criteriaSHA256 string,
	policySHA256 string,
	executionPolicy AccuracyFirstExecutionPolicy,
) bool {
	expectedProfileID, err := candidateProfileID(providerMode)
	return err == nil && config.SchemaVersion == "neo-chat.memory-regression-profile-config.v18" &&
		config.ProfileID == expectedProfileID &&
		config.ReaderVersion == ProductionBufferedMemoryJudgeValidationReaderVersion &&
		config.CostBasisSHA256 == costBasisSHA256 &&
		config.ProviderMode == providerMode &&
		config.CaptureMode == CaptureModeProductionBufferedMemoryJudgeValidation &&
		config.EvaluationSplit == FrozenValidationSplit &&
		config.RelevancePolicyID == usermemory.HybridRelevanceNegativePolicyGuardProductionPolicyID &&
		config.RelevancePolicyMode == "fixed_cloud_candidate_judge_negative_guard_production" &&
		config.CloudCandidateJudgeRequired &&
		config.CloudCandidateJudgeModelID == usermemory.HybridFixedMemoryJudgeModelID &&
		config.ConfiguredCandidateJudgeProviderID == authority.ProviderID &&
		config.ConfiguredCandidateJudgeProviderType == authority.ProviderType &&
		config.ConfiguredCandidateJudgeBaseURLSHA256 == authority.BaseURLSHA256 &&
		config.ConfiguredCandidateJudgeAdapter == memoryjudge.BufferedChatAdapterVersion &&
		config.NegativePolicyQueryGuardRequired &&
		config.NegativePolicyQueryGuardVersion == usermemory.NegativePolicyQueryGuardVersion &&
		config.NegativePolicyQueryGuardSHA256 == usermemory.NegativePolicyQueryGuardSHA256 &&
		config.EvaluationCriteriaVersion == memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 &&
		config.ValidationCaseOrderSHA256 == caseOrderSHA256 &&
		config.EvaluationCriteriaSHA256 == criteriaSHA256 &&
		config.ProductionRelevancePolicySHA256 == policySHA256 &&
		config.MemoryReadIntentPolicyVersion == chat.MemoryReadIntentPolicyVersion &&
		config.MemoryReadIntentPolicySHA256 == chat.MemoryReadIntentPolicySHA256 &&
		config.AccuracyFirstExecutionPolicy != nil &&
		*config.AccuracyFirstExecutionPolicy == executionPolicy &&
		config.CandidateJudgeFailureTaxonomyVersion == memoryjudge.FailureTaxonomyVersion &&
		config.CandidateJudgeFailureTaxonomySHA256 == memoryjudge.FailureTaxonomySHA256 &&
		config.CandidateJudgeDiagnosticCompleteness == JudgeFailureDiagnosticCompletenessPolicy
}

func validProductionBufferedMemoryJudgeValidationReport(
	report ProductionBufferedMemoryJudgeValidationReport,
) bool {
	providerMode, err := providerModeForProfileID(report.ProfileID)
	if err != nil || memoryeval.ValidateMemoryJudgeAccuracyFirstCriteriaV3(
		report.EvaluationCriteria,
	) != nil {
		return false
	}
	expectedExecution, err := ProductionBufferedMemoryJudgeValidationExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedExecution ||
		(providerMode == ProviderModeFakeProtocol &&
			report.ProviderAttempts.InterCaseCooldownElapsedMillis != 0) {
		return false
	}
	expectedEvidence := ProductionValidationEvidenceLive
	if providerMode == ProviderModeFakeProtocol {
		expectedEvidence = ProductionValidationEvidenceFake
	}
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID: report.JudgeProviderID, ProviderType: report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256, ModelID: report.JudgeModelID,
	}
	if !validFixedMemoryJudgeAuthority(authority) ||
		!validJudgeFailureCategoryCounts(report.Diagnostics.JudgeTerminalFailureCategoryCounts) {
		return false
	}
	terminalAttemptFailures := 0
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
	expectedOutcome := productionValidationOutcome(
		report.EvidenceClass,
		report.Evaluation,
		report.Diagnostics.FailedCaseCount,
		report.EvaluationCriteria.MaximumFalseInjectionRate,
	)
	expectedPassed := report.EvidenceClass == ProductionValidationEvidenceLive &&
		report.Evaluation.Passed && report.Diagnostics.FailedCaseCount == 0 &&
		expectedOutcome.Severity == ProductionValidationSeverityNone
	return report.SchemaVersion == ProductionBufferedMemoryJudgeValidationReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == ProductionBufferedMemoryJudgeValidationAdmissionMode &&
		!report.PromotionEligible && !report.ReleaseEligible && !report.PolicySelected &&
		report.ExecutionComplete && report.EvidenceClass == expectedEvidence &&
		report.Split == FrozenValidationSplit && report.CaseCount == 100 &&
		report.PolicyID == usermemory.HybridRelevanceNegativePolicyGuardProductionPolicyID &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && report.JudgeAdapter == memoryjudge.BufferedChatAdapterVersion &&
		report.NegativePolicyQueryGuardVersion == usermemory.NegativePolicyQueryGuardVersion &&
		report.NegativePolicyQueryGuardSHA256 == usermemory.NegativePolicyQueryGuardSHA256 &&
		report.JudgePromptVersion == usermemory.HybridCandidateJudgePromptVersion &&
		report.JudgePromptSHA256 == usermemory.HybridCandidateJudgePromptSHA256 &&
		report.JudgeDecodingProfile == usermemory.HybridCandidateJudgeDecodingProfile &&
		report.FailureTaxonomyVersion == memoryjudge.FailureTaxonomyVersion &&
		report.FailureTaxonomySHA256 == memoryjudge.FailureTaxonomySHA256 &&
		report.DiagnosticCompleteness == JudgeFailureDiagnosticCompletenessPolicy &&
		report.SelectionAlgorithm == cloudJudgeSelectionAlgorithm &&
		report.EvaluationCriteriaVersion == memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 &&
		report.MemoryReadIntentPolicyVersion == chat.MemoryReadIntentPolicyVersion &&
		report.MemoryReadIntentPolicySHA256 == chat.MemoryReadIntentPolicySHA256 &&
		validSHA256String(report.ConfigurationSHA256) &&
		validSHA256String(report.ValidationCaseOrderSHA256) &&
		validSHA256String(report.CorpusRawSHA256) &&
		validSHA256String(report.ProductionRelevancePolicySHA256) &&
		validSHA256String(report.EvaluationCriteriaSHA256) &&
		report.Diagnostics.FailureCodeCounts != nil &&
		report.Diagnostics.EmptyCandidateCaseCount+
			report.Diagnostics.NegativePolicyQueryAbstainedCaseCount+
			report.Diagnostics.JudgeCompletedCaseCount+
			report.Diagnostics.FailedCaseCount == report.CaseCount &&
		sumDiagnosticCounts(report.Diagnostics.JudgeTerminalFailureCategoryCounts) ==
			report.Diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"] &&
		validateTransportStableMemoryJudgeTelemetry(
			telemetry,
			report.CaseCount,
			logicalJudgeRequests,
			terminalAttemptFailures,
		) == nil &&
		report.CostAuthority.AuthorizedRequestCount == 300 &&
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
			report.CostAuthority.MaximumJudgeCostMicrounits &&
		report.Passed == expectedPassed &&
		equalProductionValidationOutcome(report.Outcome, expectedOutcome)
}

func productionBufferedRelevancePolicySHA256() (string, error) {
	return relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowNegativePolicyGuardProductionPolicy(),
	)
}
