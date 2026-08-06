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
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	NegativePolicyGuardMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v16"
	NegativePolicyGuardMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_negative_guard_only"
	NegativePolicyGuardMemoryJudgeArtifactName        = "fixed-memory-judge-negative-guard-development.json"
)

type NegativePolicyGuardMemoryJudgeDevelopmentReport JudgeFailureDiagnosticDevelopmentReport

func negativePolicyGuardMemoryJudgeDevelopmentReportSpec() (
	transportStableMemoryJudgeReportSpec,
	error,
) {
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(
		usermemory.HybridShadowNegativePolicyGuardDevelopmentPolicy(),
	)
	if err != nil {
		return transportStableMemoryJudgeReportSpec{}, err
	}
	return transportStableMemoryJudgeReportSpec{
		readerVersion:                   NegativePolicyGuardMemoryJudgeReaderVersion,
		reportSchemaVersion:             NegativePolicyGuardMemoryJudgeReportSchemaVersion,
		admissionMode:                   NegativePolicyGuardMemoryJudgeAdmissionMode,
		policyID:                        usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID,
		allowNegativeGuardAbstention:    true,
		negativeGuardRequired:           true,
		negativeGuardVersion:            usermemory.NegativePolicyQueryGuardVersion,
		negativeGuardSHA256:             usermemory.NegativePolicyQueryGuardSHA256,
		relevancePolicyDescriptorSHA256: descriptorSHA256,
		validateCostAuthority:           ValidateNegativePolicyGuardMemoryJudgeCostAuthority,
	}, nil
}

func CaptureNegativePolicyGuardMemoryJudgeDevelopment(
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
		usermemory.HybridShadowNegativePolicyGuardDevelopmentPolicy(),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = NegativePolicyGuardMemoryJudgeReaderVersion
	return profile, nil
}

func BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (NegativePolicyGuardMemoryJudgeDevelopmentReport, []byte, error) {
	spec, err := negativePolicyGuardMemoryJudgeDevelopmentReportSpec()
	if err != nil {
		return NegativePolicyGuardMemoryJudgeDevelopmentReport{}, nil, err
	}
	report, body, err := buildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		authority,
		costBasis,
		spec,
	)
	return NegativePolicyGuardMemoryJudgeDevelopmentReport(report), body, err
}

func validNegativePolicyGuardMemoryJudgeDevelopmentReport(
	report NegativePolicyGuardMemoryJudgeDevelopmentReport,
) bool {
	spec, err := negativePolicyGuardMemoryJudgeDevelopmentReportSpec()
	return err == nil && validTransportStableMemoryJudgeDevelopmentReportForSpec(
		JudgeFailureDiagnosticDevelopmentReport(report),
		spec,
	)
}

func BuildNegativePolicyGuardMemoryJudgeRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report NegativePolicyGuardMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !validNegativePolicyGuardMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedPolicy, err := TransportStableDevelopmentExecutionPolicy(providerMode)
	if err != nil || report.ExecutionPolicy != expectedPolicy {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != NegativePolicyGuardMemoryJudgeArtifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion:                   RelevanceRunManifestSchemaVersion,
		RunID:                           runID,
		CaptureID:                       captureID,
		CorpusClass:                     memoryeval.RegressionCorpusClass,
		AdmissionMode:                   NegativePolicyGuardMemoryJudgeAdmissionMode,
		PromotionEligible:               false,
		CaptureMode:                     CaptureModeNegativePolicyGuardMemoryJudge,
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
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RelevanceRunManifest{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return manifest, append(body, '\n'), nil
}
