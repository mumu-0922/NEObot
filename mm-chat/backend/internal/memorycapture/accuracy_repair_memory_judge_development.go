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
	AccuracyRepairMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v20"
	AccuracyRepairMemoryJudgeRunSchemaVersion    = "neo-chat.memory-regression-relevance-run.v20"
	AccuracyRepairMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_negative_guard_buffered_accuracy_repair_only"
	AccuracyRepairMemoryJudgeArtifactName        = "fixed-memory-judge-negative-guard-buffered-accuracy-repair-development.json"
)

// AccuracyRepairMemoryJudgeDevelopmentReport is the fresh full-Development
// proof for the one schema-v19-selected semantic change: Luna prompt v2.
type AccuracyRepairMemoryJudgeDevelopmentReport JudgeFailureDiagnosticDevelopmentReport

// AccuracyRepairMemoryJudgeRunManifest keeps the historical aggregate shape
// but uses a fresh schema identity so v17 evidence cannot be reinterpreted.
type AccuracyRepairMemoryJudgeRunManifest RelevanceRunManifest

func accuracyRepairMemoryJudgeDevelopmentReportSpec() (
	transportStableMemoryJudgeReportSpec,
	error,
) {
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowAccuracyRepairDevelopmentPolicy(),
	)
	if err != nil {
		return transportStableMemoryJudgeReportSpec{}, err
	}
	return transportStableMemoryJudgeReportSpec{
		readerVersion:                   AccuracyRepairMemoryJudgeReaderVersion,
		reportSchemaVersion:             AccuracyRepairMemoryJudgeReportSchemaVersion,
		admissionMode:                   AccuracyRepairMemoryJudgeAdmissionMode,
		policyID:                        usermemory.HybridRelevanceAccuracyRepairDevelopmentPolicyID,
		allowNegativeGuardAbstention:    true,
		negativeGuardRequired:           true,
		negativeGuardVersion:            usermemory.NegativePolicyQueryGuardVersion,
		negativeGuardSHA256:             usermemory.NegativePolicyQueryGuardSHA256,
		relevancePolicyDescriptorSHA256: descriptorSHA256,
		judgeAdapter:                    memoryjudge.BufferedChatAccuracyAdapterVersion,
		judgePromptVersion:              usermemory.HybridCandidateJudgeAccuracyPromptVersion,
		judgePromptSHA256:               usermemory.HybridCandidateJudgeAccuracyPromptSHA256,
		executionPolicy:                 AccuracyRepairMemoryJudgeDevelopmentExecutionPolicy,
		validateCostAuthority:           ValidateAccuracyRepairMemoryJudgeCostAuthority,
	}, nil
}

func CaptureAccuracyRepairMemoryJudgeDevelopment(
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
		!validFixedMemoryJudgeAuthority(authority) {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	profile, err := captureCandidateProfile(
		ctx,
		runtimeDB,
		index,
		seed.Cases,
		provider,
		usermemory.HybridShadowAccuracyRepairDevelopmentPolicy(),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = AccuracyRepairMemoryJudgeReaderVersion
	return profile, nil
}

func BuildAccuracyRepairMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (AccuracyRepairMemoryJudgeDevelopmentReport, []byte, error) {
	spec, err := accuracyRepairMemoryJudgeDevelopmentReportSpec()
	if err != nil {
		return AccuracyRepairMemoryJudgeDevelopmentReport{}, nil, err
	}
	report, body, err := buildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		spec,
	)
	return AccuracyRepairMemoryJudgeDevelopmentReport(report), body, err
}

func validAccuracyRepairMemoryJudgeDevelopmentReport(
	report AccuracyRepairMemoryJudgeDevelopmentReport,
) bool {
	spec, err := accuracyRepairMemoryJudgeDevelopmentReportSpec()
	return err == nil && validTransportStableMemoryJudgeDevelopmentReportForSpec(
		JudgeFailureDiagnosticDevelopmentReport(report),
		spec,
	)
}

func BuildAccuracyRepairMemoryJudgeRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report AccuracyRepairMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (AccuracyRepairMemoryJudgeRunManifest, []byte, error) {
	if !validAccuracyRepairMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := AccuracyRepairMemoryJudgeDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != AccuracyRepairMemoryJudgeArtifactName {
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := AccuracyRepairMemoryJudgeRunManifest(RelevanceRunManifest{
		SchemaVersion:                   AccuracyRepairMemoryJudgeRunSchemaVersion,
		RunID:                           runID,
		CaptureID:                       captureID,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   AccuracyRepairMemoryJudgeAdmissionMode,
		PromotionEligible:               false,
		CaptureMode:                     CaptureModeAccuracyRepairMemoryJudge,
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
		return AccuracyRepairMemoryJudgeRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}
