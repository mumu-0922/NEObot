package memorycapture

import (
	"context"
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
	TransportStableMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v14"
	TransportStableMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_transport_stable_only"
	TransportStableMemoryJudgeArtifactName        = "fixed-memory-judge-transport-stable-development.json"
)

// TransportStableMemoryJudgeDevelopmentReport retains the schema-v13 bounded
// failure maps while restoring ordinary Development pass/fail semantics. A
// terminal Judge failure always prevents Passed even if the remaining scored
// cases independently satisfy the quality criteria.
type TransportStableMemoryJudgeDevelopmentReport JudgeFailureDiagnosticDevelopmentReport

type transportStableMemoryJudgeReportSpec struct {
	readerVersion                   string
	reportSchemaVersion             string
	admissionMode                   string
	policyID                        string
	allowNegativeGuardAbstention    bool
	negativeGuardRequired           bool
	negativeGuardVersion            string
	negativeGuardSHA256             string
	relevancePolicyDescriptorSHA256 string
	judgeAdapter                    string
	executionPolicy                 func(string) (AccuracyFirstExecutionPolicy, error)
	validateCostAuthority           func(CostBasis, ConfiguredCandidateJudgeProfileAuthority) error
}

func transportStableMemoryJudgeDevelopmentReportSpec() transportStableMemoryJudgeReportSpec {
	return transportStableMemoryJudgeReportSpec{
		readerVersion:         TransportStableMemoryJudgeReaderVersion,
		reportSchemaVersion:   TransportStableMemoryJudgeReportSchemaVersion,
		admissionMode:         TransportStableMemoryJudgeAdmissionMode,
		policyID:              usermemory.HybridRelevanceAccuracyFirstJudgePolicyID,
		judgeAdapter:          memoryjudge.ChatAdapterVersion,
		executionPolicy:       TransportStableDevelopmentExecutionPolicy,
		validateCostAuthority: ValidateTransportStableMemoryJudgeCostAuthority,
	}
}

func CaptureTransportStableMemoryJudgeDevelopment(
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
	if !hybridOK || !judgeOK || hybrid.controller == nil ||
		hybrid.controller != candidateJudge.controller ||
		hybrid.controller.maximumJudgeRetries != 2 ||
		!hybrid.controller.judgeFailureDiagnostics {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	profile, err := CaptureJudgeFailureDiagnosticDevelopment(
		ctx,
		seedDB,
		runtimeDB,
		runID,
		fullPool,
		index,
		seed,
		provider,
		judge,
		authority,
		profileID,
		configurationSHA256,
		cost,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = TransportStableMemoryJudgeReaderVersion
	return profile, nil
}

func BuildTransportStableMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (TransportStableMemoryJudgeDevelopmentReport, []byte, error) {
	report, body, err := buildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		transportStableMemoryJudgeDevelopmentReportSpec(),
	)
	return TransportStableMemoryJudgeDevelopmentReport(report), body, err
}

func buildTransportStableMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
	spec transportStableMemoryJudgeReportSpec,
) (JudgeFailureDiagnosticDevelopmentReport, []byte, error) {
	if profile.Profile.ReaderVersion != spec.readerVersion ||
		!validFixedMemoryJudgeAuthority(authority) ||
		spec.judgeAdapter == "" || spec.executionPolicy == nil ||
		spec.validateCostAuthority == nil ||
		spec.validateCostAuthority(costBasis, authority) != nil ||
		profile.Profile.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		profile.Costs != costBasis.Candidate ||
		len(profile.Profile.ConfigurationSHA256) != 64 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, fmt.Errorf(
			"%w: transport-stable report preflight",
			ErrCaptureInvalid,
		)
	}
	if _, err := hex.DecodeString(profile.Profile.ConfigurationSHA256); err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	executionPolicy, err := spec.executionPolicy(providerMode)
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
				"%w: transport-stable Memory Judge trace contains a hard cutoff",
				ErrCaptureInvalid,
			)
		}
	}
	aggregate, err := aggregateCloudJudgeCaptureSplit(
		pool,
		profile,
		DevelopmentCalibrationSplit,
		300,
		true,
		spec.allowNegativeGuardAbstention,
	)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	terminalCounts, terminalAttemptFailures, err :=
		aggregateJudgeTerminalFailureCategories(profile.Calibration, aggregate.diagnostics)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, err
	}
	telemetry := profile.ProviderAttempts
	if err := validateTransportStableMemoryJudgeTelemetry(
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
			"%w: transport-stable Memory Judge input telemetry drifted",
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
			"%w: transport-stable Memory Judge cost authority exceeded",
			ErrCaptureInvalid,
		)
	}
	report := JudgeFailureDiagnosticDevelopmentReport{
		SchemaVersion:             spec.reportSchemaVersion,
		CorpusClass:               memoryeval.RegressionCorpusClass,
		AdmissionMode:             spec.admissionMode,
		PromotionEligible:         false,
		PolicySelected:            false,
		DiagnosticComplete:        true,
		Split:                     DevelopmentCalibrationSplit,
		CaseCount:                 len(aggregate.development),
		PolicyID:                  spec.policyID,
		ProfileID:                 profile.Profile.ID,
		ConfigurationSHA256:       profile.Profile.ConfigurationSHA256,
		ProviderEgressPolicy:      memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		ProviderCostPolicy:        costBasis.ProviderCostPolicy,
		ProviderCostAuthorized:    true,
		JudgeProviderID:           authority.ProviderID,
		JudgeProviderType:         authority.ProviderType,
		JudgeBaseURLSHA256:        authority.BaseURLSHA256,
		JudgeModelID:              authority.ModelID,
		JudgeAdapter:              spec.judgeAdapter,
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
		Passed:                    evaluation.Passed && aggregate.diagnostics.FailedCaseCount == 0,
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
		NegativePolicyQueryGuardRequired: spec.negativeGuardRequired,
		NegativePolicyQueryGuardVersion:  spec.negativeGuardVersion,
		NegativePolicyQueryGuardSHA256:   spec.negativeGuardSHA256,
		RelevancePolicyDescriptorSHA256:  spec.relevancePolicyDescriptorSHA256,
	}
	if !validTransportStableMemoryJudgeDevelopmentReportForSpec(report, spec) {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil, fmt.Errorf(
			"%w: transport-stable report final validation",
			ErrCaptureInvalid,
		)
	}
	body, err := json.Marshal(report)
	if err != nil {
		return JudgeFailureDiagnosticDevelopmentReport{}, nil,
			errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func validateTransportStableMemoryJudgeTelemetry(
	value AccuracyFirstProviderTelemetry,
	caseCount int,
	logicalJudgeRequests int,
	terminalAttemptFailures int,
) error {
	if err := validateAccuracyFirstProviderTelemetryWithJudgeRetryLimit(
		value,
		caseCount,
		logicalJudgeRequests,
		2,
	); err != nil || value.JudgeAttemptFailureCategoryCounts == nil ||
		terminalAttemptFailures < 0 {
		return fmt.Errorf("%w: transport-stable Provider telemetry", ErrCaptureInvalid)
	}
	if !validJudgeAttemptFailureCategoryCounts(value.JudgeAttemptFailureCategoryCounts) ||
		sumDiagnosticCounts(value.JudgeAttemptFailureCategoryCounts) !=
			value.JudgeRetries+terminalAttemptFailures {
		return fmt.Errorf(
			"%w: transport-stable Judge attempt reconciliation",
			ErrCaptureInvalid,
		)
	}
	return nil
}

func validTransportStableMemoryJudgeDevelopmentReport(
	report TransportStableMemoryJudgeDevelopmentReport,
) bool {
	return validTransportStableMemoryJudgeDevelopmentReportForSpec(
		JudgeFailureDiagnosticDevelopmentReport(report),
		transportStableMemoryJudgeDevelopmentReportSpec(),
	)
}

func validTransportStableMemoryJudgeDevelopmentReportForSpec(
	report JudgeFailureDiagnosticDevelopmentReport,
	spec transportStableMemoryJudgeReportSpec,
) bool {
	if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 ||
		memoryeval.ValidateMemoryJudgeAccuracyFirstCriteriaV3(
			report.EvaluationCriteria,
		) != nil {
		return false
	}
	providerMode, err := providerModeForProfileID(report.ProfileID)
	if err != nil {
		return false
	}
	if spec.judgeAdapter == "" || spec.executionPolicy == nil {
		return false
	}
	expectedExecution, err := spec.executionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedExecution ||
		(providerMode == ProviderModeFakeProtocol &&
			report.ProviderAttempts.InterCaseCooldownElapsedMillis != 0) {
		return false
	}
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID: report.JudgeProviderID, ProviderType: report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256, ModelID: report.JudgeModelID,
	}
	if !validJudgeFailureCategoryCounts(
		report.Diagnostics.JudgeTerminalFailureCategoryCounts,
	) {
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
	guardProvenanceValid := report.Diagnostics.NegativePolicyQueryAbstainedCaseCount == 0 &&
		!report.NegativePolicyQueryGuardRequired &&
		report.NegativePolicyQueryGuardVersion == "" &&
		report.NegativePolicyQueryGuardSHA256 == "" &&
		report.RelevancePolicyDescriptorSHA256 == ""
	if spec.negativeGuardRequired {
		guardProvenanceValid = report.Diagnostics.NegativePolicyQueryAbstainedCaseCount > 0 &&
			report.NegativePolicyQueryGuardRequired &&
			report.NegativePolicyQueryGuardVersion == spec.negativeGuardVersion &&
			report.NegativePolicyQueryGuardSHA256 == spec.negativeGuardSHA256 &&
			report.RelevancePolicyDescriptorSHA256 == spec.relevancePolicyDescriptorSHA256 &&
			validSHA256String(report.NegativePolicyQueryGuardSHA256) &&
			validSHA256String(report.RelevancePolicyDescriptorSHA256)
	}
	return report.SchemaVersion == spec.reportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == spec.admissionMode &&
		!report.PromotionEligible && !report.PolicySelected && report.DiagnosticComplete &&
		report.Passed == (report.Evaluation.Passed &&
			report.Diagnostics.FailedCaseCount == 0) &&
		report.Split == DevelopmentCalibrationSplit && report.CaseCount == 300 &&
		report.PolicyID == spec.policyID && guardProvenanceValid &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && validFixedMemoryJudgeAuthority(authority) &&
		report.JudgeAdapter == spec.judgeAdapter &&
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
		report.CostAuthority.AuthorizedRequestCount == 900 &&
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
