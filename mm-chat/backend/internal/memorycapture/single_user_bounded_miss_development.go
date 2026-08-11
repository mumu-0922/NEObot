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
	SingleUserBoundedMissDevelopmentReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v24-single-user-bounded-miss-development.v1"
	SingleUserBoundedMissDevelopmentRunSchemaVersion    = "neo-chat.memory-regression-relevance-run.v24-single-user-bounded-miss-development.v1"
	SingleUserBoundedMissDevelopmentAdmissionMode       = "development_fixed_memory_judge_negative_guard_double_confirmation_single_user_bounded_miss_only"
	SingleUserBoundedMissDevelopmentArtifactName        = "fixed-memory-judge-negative-guard-double-confirmation-single-user-bounded-miss-development.json"
)

// SingleUserBoundedMissDevelopmentReport overrides only the prospective
// criteria-bound fields of the immutable aggregate Development envelope. The
// embedded v3 fields remain available for internal common validation but are
// shadowed during JSON encoding by the v4 fields below.
type SingleUserBoundedMissDevelopmentReport struct {
	JudgeFailureDiagnosticDevelopmentReport
	EvaluationCriteriaVersion string                                        `json:"evaluationCriteriaVersion"`
	EvaluationCriteriaSHA256  string                                        `json:"evaluationCriteriaSha256"`
	EvaluationCriteria        memoryeval.SingleUserBoundedMissCriteria      `json:"evaluationCriteria"`
	Evaluation                memoryeval.AccuracyFirstCalibrationEvaluation `json:"evaluation"`
	Passed                    bool                                          `json:"passed"`
}

type SingleUserBoundedMissDevelopmentRunManifest struct {
	RelevanceRunManifest
	EvaluationCriteriaSHA256 string `json:"evaluationCriteriaSha256"`
}

func singleUserBoundedMissDevelopmentReportSpec() (
	transportStableMemoryJudgeReportSpec,
	error,
) {
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowDoubleConfirmationDevelopmentPolicy(),
	)
	if err != nil {
		return transportStableMemoryJudgeReportSpec{}, err
	}
	return transportStableMemoryJudgeReportSpec{
		readerVersion:                   SingleUserBoundedMissDevelopmentReaderVersion,
		reportSchemaVersion:             SingleUserBoundedMissDevelopmentReportSchemaVersion,
		admissionMode:                   SingleUserBoundedMissDevelopmentAdmissionMode,
		policyID:                        usermemory.HybridRelevanceDoubleConfirmationDevelopmentPolicyID,
		allowNegativeGuardAbstention:    true,
		negativeGuardRequired:           true,
		negativeGuardVersion:            usermemory.NegativePolicyQueryGuardVersion,
		negativeGuardSHA256:             usermemory.NegativePolicyQueryGuardSHA256,
		relevancePolicyDescriptorSHA256: descriptorSHA256,
		judgeAdapter:                    memoryjudge.BufferedChatAbstentionConfirmationAdapterVersion,
		judgePromptVersion:              usermemory.HybridCandidateJudgeAccuracyPromptVersion,
		judgePromptSHA256:               usermemory.HybridCandidateJudgeAccuracyPromptSHA256,
		judgeConfirmationPromptVersion:  usermemory.HybridCandidateJudgeConfirmationPromptVersion,
		judgeConfirmationPromptSHA256:   usermemory.HybridCandidateJudgeConfirmationPromptSHA256,
		confirmationRequired:            true,
		maximumAbstentionConfirmations:  2,
		authorizedRequestCount:          2700,
		executionPolicy:                 SingleUserBoundedMissDevelopmentExecutionPolicy,
		validateCostAuthority:           ValidateSingleUserBoundedMissDevelopmentCostAuthority,
	}, nil
}

func CaptureSingleUserBoundedMissDevelopment(
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
	profile, err := CaptureDoubleConfirmationMemoryJudgeDevelopment(
		ctx, seedDB, runtimeDB, runID, fullPool, index, seed, provider, judge,
		authority, profileID, configurationSHA256, cost,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = SingleUserBoundedMissDevelopmentReaderVersion
	return profile, nil
}

func BuildSingleUserBoundedMissDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	config ProfileConfig,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (SingleUserBoundedMissDevelopmentReport, []byte, error) {
	spec, err := singleUserBoundedMissDevelopmentReportSpec()
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	costBasisSHA256, err := CostBasisSHA256(costBasis)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	criteria, err := memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaV4(
		pool.Corpus.Criteria,
	)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	criteriaSHA256, err := singleUserBoundedMissCriteriaSHA256(pool.Corpus.Criteria)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	providerMode, err := providerModeForProfileID(profile.Profile.ID)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	executionPolicy, err := SingleUserBoundedMissDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	if profile.Profile.ConfigurationSHA256 != configurationSHA256 ||
		!validSingleUserBoundedMissDevelopmentConfig(
			config,
			providerMode,
			authority,
			costBasisSHA256,
			criteriaSHA256,
			executionPolicy,
		) {
		return SingleUserBoundedMissDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	base, _, err := buildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		spec,
	)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	aggregate, err := aggregateCloudJudgeCaptureSplit(
		pool,
		profile,
		DevelopmentCalibrationSplit,
		300,
		true,
		true,
	)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	evaluation, err := memoryeval.EvaluateSingleUserBoundedMissCalibrationSelectionWithProviderEgressPolicy(
		aggregate.development,
		aggregate.ordered,
		pool.Corpus.Criteria,
		memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
	)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil, err
	}
	report := SingleUserBoundedMissDevelopmentReport{
		JudgeFailureDiagnosticDevelopmentReport: base,
		EvaluationCriteriaVersion:               memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaVersionV4,
		EvaluationCriteriaSHA256:                criteriaSHA256,
		EvaluationCriteria:                      criteria,
		Evaluation:                              evaluation,
		Passed: evaluation.Passed &&
			aggregate.diagnostics.FailedCaseCount == 0,
	}
	if !validSingleUserBoundedMissDevelopmentReport(report) {
		return SingleUserBoundedMissDevelopmentReport{}, nil, fmt.Errorf(
			"%w: single-user bounded-miss Development report validation",
			ErrCaptureInvalid,
		)
	}
	body, err := json.Marshal(report)
	if err != nil {
		return SingleUserBoundedMissDevelopmentReport{}, nil,
			errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func validSingleUserBoundedMissDevelopmentConfig(
	config ProfileConfig,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasisSHA256 string,
	criteriaSHA256 string,
	executionPolicy AccuracyFirstExecutionPolicy,
) bool {
	expectedProfileID, err := candidateProfileID(providerMode)
	return err == nil &&
		config.SchemaVersion == "neo-chat.memory-regression-profile-config.v24-single-user-bounded-miss-development.v1" &&
		config.ProfileID == expectedProfileID &&
		config.ReaderVersion == SingleUserBoundedMissDevelopmentReaderVersion &&
		config.CostBasisSHA256 == costBasisSHA256 &&
		config.ProviderMode == providerMode &&
		config.CaptureMode == CaptureModeSingleUserBoundedMissDevelopment &&
		config.EvaluationSplit == DevelopmentCalibrationSplit &&
		config.RelevancePolicyID == usermemory.HybridRelevanceDoubleConfirmationDevelopmentPolicyID &&
		config.RelevancePolicyMode == "fixed_cloud_candidate_judge_negative_guard_double_confirmation_development" &&
		config.ConfiguredCandidateJudgeProviderID == authority.ProviderID &&
		config.ConfiguredCandidateJudgeProviderType == authority.ProviderType &&
		config.ConfiguredCandidateJudgeBaseURLSHA256 == authority.BaseURLSHA256 &&
		config.ConfiguredCandidateJudgeAdapter == memoryjudge.BufferedChatAbstentionConfirmationAdapterVersion &&
		config.CloudCandidateJudgeAbstentionConfirmationRequired &&
		config.CloudCandidateJudgeMaximumAbstentionConfirmations == 2 &&
		config.NegativePolicyQueryGuardRequired &&
		config.NegativePolicyQueryGuardVersion == usermemory.NegativePolicyQueryGuardVersion &&
		config.NegativePolicyQueryGuardSHA256 == usermemory.NegativePolicyQueryGuardSHA256 &&
		config.EvaluationCriteriaVersion == memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaVersionV4 &&
		config.EvaluationCriteriaSHA256 == criteriaSHA256 &&
		config.AccuracyFirstExecutionPolicy != nil &&
		*config.AccuracyFirstExecutionPolicy == executionPolicy &&
		config.CandidateJudgeFailureTaxonomyVersion == memoryjudge.FailureTaxonomyVersion &&
		config.CandidateJudgeFailureTaxonomySHA256 == memoryjudge.FailureTaxonomySHA256 &&
		config.CandidateJudgeDiagnosticCompleteness == JudgeFailureDiagnosticCompletenessPolicy
}

func validSingleUserBoundedMissDevelopmentReport(
	report SingleUserBoundedMissDevelopmentReport,
) bool {
	spec, err := singleUserBoundedMissDevelopmentReportSpec()
	if err != nil ||
		!validTransportStableMemoryJudgeDevelopmentReportForSpec(
			report.JudgeFailureDiagnosticDevelopmentReport,
			spec,
		) ||
		memoryeval.ValidateMemoryJudgeSingleUserBoundedMissCriteriaV4(
			report.EvaluationCriteria,
		) != nil ||
		memoryeval.ValidateSingleUserBoundedMissCalibrationEvaluation(
			report.Evaluation,
			report.EvaluationCriteria,
		) != nil {
		return false
	}
	criteriaSHA256, err := sha256JSON(report.EvaluationCriteria)
	return err == nil &&
		report.EvaluationCriteriaVersion == memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaVersionV4 &&
		report.EvaluationCriteriaSHA256 == criteriaSHA256 &&
		report.Passed == (report.Evaluation.Passed && report.Diagnostics.FailedCaseCount == 0)
}

func BuildSingleUserBoundedMissDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report SingleUserBoundedMissDevelopmentReport,
	artifacts []Artifact,
) (SingleUserBoundedMissDevelopmentRunManifest, []byte, error) {
	if !validSingleUserBoundedMissDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := SingleUserBoundedMissDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != SingleUserBoundedMissDevelopmentArtifactName {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := SingleUserBoundedMissDevelopmentRunManifest{
		RelevanceRunManifest: RelevanceRunManifest{
			SchemaVersion:                   SingleUserBoundedMissDevelopmentRunSchemaVersion,
			RunID:                           runID,
			CaptureID:                       captureID,
			CorpusClass:                     memoryeval.RegressionCorpusClass,
			AdmissionMode:                   SingleUserBoundedMissDevelopmentAdmissionMode,
			PromotionEligible:               false,
			CaptureMode:                     CaptureModeSingleUserBoundedMissDevelopment,
			Split:                           DevelopmentCalibrationSplit,
			ProviderMode:                    providerMode,
			ProfileID:                       report.ProfileID,
			PolicyID:                        report.PolicyID,
			ConfigurationSHA256:             report.ConfigurationSHA256,
			Passed:                          report.Passed,
			StartedAt:                       startedAt.UTC().Format(time.RFC3339),
			CompletedAt:                     completedAt.UTC().Format(time.RFC3339),
			CostBasisSHA256:                 costBasisSHA256,
			ProviderCostPolicy:              report.ProviderCostPolicy,
			NegativePolicyQueryGuardVersion: report.NegativePolicyQueryGuardVersion,
			NegativePolicyQueryGuardSHA256:  report.NegativePolicyQueryGuardSHA256,
			RelevancePolicyDescriptorSHA256: report.RelevancePolicyDescriptorSHA256,
			Inputs: RunInputHashes{
				FixtureRawSHA256:  protected.FixtureRawSHA256,
				CorpusRawSHA256:   protected.CorpusRawSHA256,
				AuditRawSHA256:    protected.AuditRawSHA256,
				ManifestRawSHA256: protected.ManifestRawSHA256,
			},
			Artifacts: artifactManifest,
		},
		EvaluationCriteriaSHA256: report.EvaluationCriteriaSHA256,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return SingleUserBoundedMissDevelopmentRunManifest{}, nil,
			errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}
