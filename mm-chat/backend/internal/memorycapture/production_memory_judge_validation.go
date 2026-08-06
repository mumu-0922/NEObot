package memorycapture

import (
	"crypto/sha256"
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
	ProductionMemoryJudgeValidationReportSchemaVersion = "neo-chat.memory-regression-relevance-validation.v15"
	ProductionMemoryJudgeValidationRunSchemaVersion    = "neo-chat.memory-regression-relevance-validation-run.v15"
	ProductionMemoryJudgeValidationAdmissionMode       = "frozen_production_fixed_memory_judge_validation_only"
	ProductionMemoryJudgeValidationArtifactName        = "fixed-memory-judge-production-validation.json"

	ProductionValidationEvidenceLive = "live_validation"
	ProductionValidationEvidenceFake = "fake_protocol_lifecycle_only"

	ProductionValidationSeverityNone   = "none"
	ProductionValidationSeverityYellow = "yellow"
	ProductionValidationSeverityOrange = "orange"
	ProductionValidationSeverityRed    = "red"

	ProductionValidationActionOwnerReview = "owner_review_no_automatic_release"
	ProductionValidationActionRetainBeta  = "retain_beta"
	ProductionValidationActionDisableRead = "disable_memory_recall_preserve_data"
	ProductionValidationActionDisableTool = "disable_memory_tool_loop"

	productionValidationReasonFake      = "FAKE_PROTOCOL_NON_EVIDENCE"
	productionValidationReasonProvider  = "PROVIDER_STABILITY_FAILURE"
	productionValidationReasonQuality   = "QUALITY_OR_TOKEN_GATE_FAILURE"
	productionValidationReasonInjection = "FALSE_INJECTION_ABOVE_0_02"
	productionValidationReasonPrivacy   = "AUTHORIZATION_OR_PRIVACY_RELEASE"
)

type ProductionValidationOutcome struct {
	Severity       string   `json:"severity"`
	RequiredAction string   `json:"requiredAction"`
	Reasons        []string `json:"reasons"`
}

// ProductionMemoryJudgeValidationReport is aggregate-only frozen Validation
// evidence. Passing does not grant Release or promotion authority.
type ProductionMemoryJudgeValidationReport struct {
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

type ProductionMemoryJudgeValidationRunManifest struct {
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

func BuildProductionMemoryJudgeValidationReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	config ProfileConfig,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (ProductionMemoryJudgeValidationReport, []byte, error) {
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	costBasisSHA256, err := CostBasisSHA256(costBasis)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	caseOrderSHA256, err := validationCaseOrderSHA256(pool)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	criteria, err := memoryeval.MemoryJudgeAccuracyFirstCriteriaV3(pool.Corpus.Criteria)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	criteriaSHA256, err := productionValidationCriteriaSHA256(pool.Corpus.Criteria)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	policySHA256, err := productionRelevancePolicySHA256()
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	executionPolicy, err := ProductionMemoryJudgeValidationExecutionPolicy(providerMode)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	if profile.Profile.ReaderVersion != ProductionMemoryJudgeValidationReaderVersion ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Profile.ConfigurationSHA256 != configurationSHA256 ||
		profile.Costs != costBasis.Candidate ||
		!validProductionMemoryJudgeValidationConfig(
			config,
			providerMode,
			authority,
			costBasisSHA256,
			caseOrderSHA256,
			criteriaSHA256,
			policySHA256,
			executionPolicy,
		) ||
		ValidateProductionMemoryJudgeValidationCostAuthority(costBasis, authority) != nil ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProductionMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	for _, trace := range profile.Calibration {
		if trace.FullObservation.HardCutoffApplied ||
			normalizeCalibrationCode(trace.AbstentionCode) == "HARD_CUTOFF" ||
			normalizeCalibrationCode(trace.ResultCode) == "HARD_CUTOFF" {
			return ProductionMemoryJudgeValidationReport{}, nil, fmt.Errorf(
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
		false,
	)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	terminalCounts, terminalAttemptFailures, err :=
		aggregateJudgeTerminalFailureCategories(profile.Calibration, aggregate.diagnostics)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateTransportStableMemoryJudgeTelemetry(
		telemetry,
		len(aggregate.development),
		aggregate.logicalJudgeRequests,
		terminalAttemptFailures,
	); err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	evaluation, err := memoryeval.EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy(
		aggregate.development,
		aggregate.ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, err
	}
	expectedInputTokenUpperBound := aggregate.logicalInputTokenUpperBound +
		uint64(telemetry.JudgeRetryInputTokenUpperBound)
	if uint64(telemetry.JudgeInputTokenUpperBound) != expectedInputTokenUpperBound {
		return ProductionMemoryJudgeValidationReport{}, nil, fmt.Errorf(
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
		return ProductionMemoryJudgeValidationReport{}, nil, fmt.Errorf(
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
	report := ProductionMemoryJudgeValidationReport{
		SchemaVersion:                   ProductionMemoryJudgeValidationReportSchemaVersion,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   ProductionMemoryJudgeValidationAdmissionMode,
		PromotionEligible:               false,
		ReleaseEligible:                 false,
		PolicySelected:                  false,
		ExecutionComplete:               true,
		EvidenceClass:                   evidenceClass,
		Split:                           FrozenValidationSplit,
		CaseCount:                       len(aggregate.development),
		PolicyID:                        usermemory.HybridRelevanceProductionJudgePolicyID,
		ProfileID:                       profile.Profile.ID,
		ConfigurationSHA256:             profile.Profile.ConfigurationSHA256,
		ValidationCaseOrderSHA256:       caseOrderSHA256,
		CorpusRawSHA256:                 config.CorpusRawSHA256,
		ProductionRelevancePolicySHA256: policySHA256,
		MemoryReadIntentPolicyVersion:   chat.MemoryReadIntentPolicyVersion,
		MemoryReadIntentPolicySHA256:    chat.MemoryReadIntentPolicySHA256,
		ProviderEgressPolicy:            memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:              costBasis.ProviderCostPolicy,
		ProviderCostAuthorized:          true,
		JudgeProviderID:                 authority.ProviderID,
		JudgeProviderType:               authority.ProviderType,
		JudgeBaseURLSHA256:              authority.BaseURLSHA256,
		JudgeModelID:                    authority.ModelID,
		JudgeAdapter:                    memoryjudge.ChatAdapterVersion,
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
	if !validProductionMemoryJudgeValidationReport(report) {
		return ProductionMemoryJudgeValidationReport{}, nil, ErrCaptureInvalid
	}
	body, err := json.Marshal(report)
	if err != nil {
		return ProductionMemoryJudgeValidationReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func BuildProductionMemoryJudgeValidationRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report ProductionMemoryJudgeValidationReport,
	artifacts []Artifact,
) (ProductionMemoryJudgeValidationRunManifest, []byte, error) {
	if !validProductionMemoryJudgeValidationReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 ||
		protected.CorpusRawSHA256 != report.CorpusRawSHA256 {
		return ProductionMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return ProductionMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || expectedProfileID != report.ProfileID {
		return ProductionMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != ProductionMemoryJudgeValidationArtifactName {
		return ProductionMemoryJudgeValidationRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := ProductionMemoryJudgeValidationRunManifest{
		SchemaVersion:                   ProductionMemoryJudgeValidationRunSchemaVersion,
		RunID:                           runID,
		CaptureID:                       captureID,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   ProductionMemoryJudgeValidationAdmissionMode,
		PromotionEligible:               false,
		ReleaseEligible:                 false,
		CaptureMode:                     CaptureModeProductionMemoryJudgeValidation,
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
		return ProductionMemoryJudgeValidationRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}

func validProductionMemoryJudgeValidationConfig(
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
	return err == nil && config.SchemaVersion == "neo-chat.memory-regression-profile-config.v15" &&
		config.ProfileID == expectedProfileID &&
		config.ReaderVersion == ProductionMemoryJudgeValidationReaderVersion &&
		config.CostBasisSHA256 == costBasisSHA256 &&
		config.ProviderMode == providerMode &&
		config.CaptureMode == CaptureModeProductionMemoryJudgeValidation &&
		config.EvaluationSplit == FrozenValidationSplit &&
		config.RelevancePolicyID == usermemory.HybridRelevanceProductionJudgePolicyID &&
		config.RelevancePolicyMode == "fixed_cloud_candidate_judge_production" &&
		config.CloudCandidateJudgeRequired &&
		config.CloudCandidateJudgeModelID == usermemory.HybridFixedMemoryJudgeModelID &&
		config.ConfiguredCandidateJudgeProviderID == authority.ProviderID &&
		config.ConfiguredCandidateJudgeProviderType == authority.ProviderType &&
		config.ConfiguredCandidateJudgeBaseURLSHA256 == authority.BaseURLSHA256 &&
		config.ConfiguredCandidateJudgeAdapter == memoryjudge.ChatAdapterVersion &&
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

func validProductionMemoryJudgeValidationReport(
	report ProductionMemoryJudgeValidationReport,
) bool {
	providerMode, err := providerModeForProfileID(report.ProfileID)
	if err != nil || memoryeval.ValidateMemoryJudgeAccuracyFirstCriteriaV3(
		report.EvaluationCriteria,
	) != nil {
		return false
	}
	expectedExecution, err := ProductionMemoryJudgeValidationExecutionPolicy(providerMode)
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
	return report.SchemaVersion == ProductionMemoryJudgeValidationReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == ProductionMemoryJudgeValidationAdmissionMode &&
		!report.PromotionEligible && !report.ReleaseEligible && !report.PolicySelected &&
		report.ExecutionComplete && report.EvidenceClass == expectedEvidence &&
		report.Split == FrozenValidationSplit && report.CaseCount == 100 &&
		report.PolicyID == usermemory.HybridRelevanceProductionJudgePolicyID &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && report.JudgeAdapter == memoryjudge.ChatAdapterVersion &&
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

func equalProductionValidationOutcome(a, b ProductionValidationOutcome) bool {
	if a.Severity != b.Severity || a.RequiredAction != b.RequiredAction ||
		len(a.Reasons) != len(b.Reasons) {
		return false
	}
	for index := range a.Reasons {
		if a.Reasons[index] != b.Reasons[index] {
			return false
		}
	}
	return true
}

func productionValidationOutcome(
	evidenceClass string,
	evaluation memoryeval.AccuracyFirstCalibrationEvaluation,
	failedCaseCount int,
	maximumFalseInjectionRate float64,
) ProductionValidationOutcome {
	if evidenceClass == ProductionValidationEvidenceFake {
		return ProductionValidationOutcome{
			Severity:       ProductionValidationSeverityYellow,
			RequiredAction: ProductionValidationActionRetainBeta,
			Reasons:        []string{productionValidationReasonFake},
		}
	}
	if !evaluation.Safety.Passed {
		return ProductionValidationOutcome{
			Severity:       ProductionValidationSeverityRed,
			RequiredAction: ProductionValidationActionDisableTool,
			Reasons:        []string{productionValidationReasonPrivacy},
		}
	}
	if evaluation.Metrics.FalseInjectionRate > maximumFalseInjectionRate {
		return ProductionValidationOutcome{
			Severity:       ProductionValidationSeverityOrange,
			RequiredAction: ProductionValidationActionDisableRead,
			Reasons:        []string{productionValidationReasonInjection},
		}
	}
	reasons := make([]string, 0, 2)
	if failedCaseCount > 0 {
		reasons = append(reasons, productionValidationReasonProvider)
	}
	if !evaluation.Passed && failedCaseCount == 0 {
		reasons = append(reasons, productionValidationReasonQuality)
	}
	if len(reasons) > 0 {
		return ProductionValidationOutcome{
			Severity:       ProductionValidationSeverityYellow,
			RequiredAction: ProductionValidationActionRetainBeta,
			Reasons:        reasons,
		}
	}
	return ProductionValidationOutcome{
		Severity:       ProductionValidationSeverityNone,
		RequiredAction: ProductionValidationActionOwnerReview,
		Reasons:        []string{},
	}
}

func validationCaseOrderSHA256(pool memoryauthor.RegressionPool) (string, error) {
	if len(pool.Corpus.Cases) != 500 {
		return "", ErrCaptureInvalid
	}
	caseIDs := make([]string, 0, 100)
	splitCounts := make(map[string]int, 3)
	for _, item := range pool.Corpus.Cases {
		splitCounts[item.Split]++
		if item.Split == FrozenValidationSplit {
			caseIDs = append(caseIDs, item.ID)
		}
	}
	if splitCounts[DevelopmentCalibrationSplit] != 300 ||
		splitCounts[FrozenValidationSplit] != 100 || splitCounts["holdout"] != 100 ||
		len(caseIDs) != 100 {
		return "", ErrCaptureInvalid
	}
	return sha256JSON(caseIDs)
}

func productionValidationCriteriaSHA256(criteria memoryeval.Criteria) (string, error) {
	value, err := memoryeval.MemoryJudgeAccuracyFirstCriteriaV3(criteria)
	if err != nil {
		return "", ErrCaptureInvalid
	}
	return sha256JSON(value)
}

func productionRelevancePolicySHA256() (string, error) {
	return relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowFixedMemoryJudgeProductionPolicy(),
	)
}

func relevancePolicyDescriptorSHA256(
	policy usermemory.HybridShadowRelevancePolicy,
) (string, error) {
	descriptor, ok := usermemory.DescribeHybridShadowRelevancePolicy(policy)
	if !ok {
		return "", ErrCaptureInvalid
	}
	return sha256JSON(descriptor)
}

func sha256JSON(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", errors.Join(ErrCaptureInvalid, err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func validSHA256String(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
