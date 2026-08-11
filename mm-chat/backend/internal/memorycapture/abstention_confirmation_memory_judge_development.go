package memorycapture

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	AbstentionConfirmationMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v20-confirmation-development.v1"
	AbstentionConfirmationMemoryJudgeRunSchemaVersion    = "neo-chat.memory-regression-relevance-run.v20-confirmation-development.v1"
	AbstentionConfirmationMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_negative_guard_abstention_confirmation_only"
	AbstentionConfirmationMemoryJudgeArtifactName        = "fixed-memory-judge-negative-guard-abstention-confirmation-development.json"
)

type AbstentionConfirmationMemoryJudgeDevelopmentReport JudgeFailureDiagnosticDevelopmentReport
type AbstentionConfirmationMemoryJudgeRunManifest RelevanceRunManifest

func abstentionConfirmationDevelopmentReportSpec() (
	transportStableMemoryJudgeReportSpec,
	error,
) {
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowAbstentionConfirmationDevelopmentPolicy(),
	)
	if err != nil {
		return transportStableMemoryJudgeReportSpec{}, err
	}
	return transportStableMemoryJudgeReportSpec{
		readerVersion:                   AbstentionConfirmationMemoryJudgeReaderVersion,
		reportSchemaVersion:             AbstentionConfirmationMemoryJudgeReportSchemaVersion,
		admissionMode:                   AbstentionConfirmationMemoryJudgeAdmissionMode,
		policyID:                        usermemory.HybridRelevanceAbstentionConfirmationDevelopmentPolicyID,
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
		maximumAbstentionConfirmations:  1,
		authorizedRequestCount:          1800,
		executionPolicy:                 AbstentionConfirmationDevelopmentExecutionPolicy,
		validateCostAuthority:           ValidateAbstentionConfirmationDevelopmentCostAuthority,
	}, nil
}

func CaptureAbstentionConfirmationMemoryJudgeDevelopment(
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
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, err
	}
	if err := validateSeedSplit(fullPool, seed.Cases, DevelopmentCalibrationSplit); err != nil {
		return CapturedProfile{}, err
	}
	hybrid, hybridOK := provider.(*accuracyFirstHybridProvider)
	candidateJudge, judgeOK := judge.(*accuracyFirstCandidateJudge)
	if !hybridOK || !judgeOK || hybrid.controller == nil ||
		hybrid.controller != candidateJudge.controller ||
		hybrid.controller.maximumJudgeRetries != 2 ||
		!hybrid.controller.judgeFailureDiagnostics ||
		!candidateJudge.confirmationEnabled {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	profile, err := captureCandidateProfile(
		ctx, runtimeDB, index, seed.Cases, provider,
		usermemory.HybridShadowAbstentionConfirmationDevelopmentPolicy(),
		profileID, configurationSHA256, cost, judge, nil,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = AbstentionConfirmationMemoryJudgeReaderVersion
	return profile, nil
}

func BuildAbstentionConfirmationMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (AbstentionConfirmationMemoryJudgeDevelopmentReport, []byte, error) {
	spec, err := abstentionConfirmationDevelopmentReportSpec()
	if err != nil {
		return AbstentionConfirmationMemoryJudgeDevelopmentReport{}, nil, err
	}
	report, body, err := buildTransportStableMemoryJudgeDevelopmentReport(
		pool, profile, authority, costBasis, spec,
	)
	return AbstentionConfirmationMemoryJudgeDevelopmentReport(report), body, err
}

func validAbstentionConfirmationMemoryJudgeDevelopmentReport(
	report AbstentionConfirmationMemoryJudgeDevelopmentReport,
) bool {
	spec, err := abstentionConfirmationDevelopmentReportSpec()
	return err == nil && validTransportStableMemoryJudgeDevelopmentReportForSpec(
		JudgeFailureDiagnosticDevelopmentReport(report), spec,
	)
}

func BuildAbstentionConfirmationMemoryJudgeRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report AbstentionConfirmationMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (AbstentionConfirmationMemoryJudgeRunManifest, []byte, error) {
	if !validAbstentionConfirmationMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := AbstentionConfirmationDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name !=
		AbstentionConfirmationMemoryJudgeArtifactName {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := AbstentionConfirmationMemoryJudgeRunManifest(RelevanceRunManifest{
		SchemaVersion:                   AbstentionConfirmationMemoryJudgeRunSchemaVersion,
		RunID:                           runID,
		CaptureID:                       captureID,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   AbstentionConfirmationMemoryJudgeAdmissionMode,
		PromotionEligible:               false,
		CaptureMode:                     CaptureModeAbstentionConfirmationMemoryJudge,
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
	})
	body, err := json.Marshal(manifest)
	if err != nil {
		return AbstentionConfirmationMemoryJudgeRunManifest{}, nil,
			errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}
