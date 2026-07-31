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
	FixedMemoryJudgeProviderID          = "SERVER_DEFAULT"
	FixedMemoryJudgeProviderType        = "openai_compatible"
	FixedMemoryJudgeBaseURL             = "https://sub.mumubuku.top/v1"
	FixedMemoryJudgeBaseURLSHA256       = "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671"
	FixedMemoryJudgeModelID             = usermemory.HybridFixedMemoryJudgeModelID
	FixedMemoryJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v11"
	FixedMemoryJudgeAdmissionMode       = "development_fixed_memory_judge_only"
	FixedMemoryJudgeArtifactName        = "fixed-memory-judge-development.json"
)

type FixedMemoryJudgeDevelopmentReport struct {
	CloudJudgeDevelopmentReport
	JudgeProviderID           string              `json:"judgeProviderId"`
	JudgeProviderType         string              `json:"judgeProviderType"`
	JudgeBaseURLSHA256        string              `json:"judgeBaseUrlSha256"`
	JudgeAdapter              string              `json:"judgeAdapter"`
	EvaluationCriteriaVersion string              `json:"evaluationCriteriaVersion"`
	EvaluationCriteria        memoryeval.Criteria `json:"evaluationCriteria"`
}

func FixedMemoryJudgeAuthority() ConfiguredCandidateJudgeProfileAuthority {
	return ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    FixedMemoryJudgeProviderID,
		ProviderType:  FixedMemoryJudgeProviderType,
		BaseURLSHA256: FixedMemoryJudgeBaseURLSHA256,
		ModelID:       FixedMemoryJudgeModelID,
	}
}

func validFixedMemoryJudgeAuthority(authority ConfiguredCandidateJudgeProfileAuthority) bool {
	return authority == FixedMemoryJudgeAuthority()
}

func BuildFixedMemoryJudgeDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	if !validFixedMemoryJudgeAuthority(authority) ||
		providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	_, config, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeFixedMemoryJudge,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowFixedMemoryJudgeDevelopmentPolicy(),
		providerCostPolicy,
		nil,
	)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.ConfiguredCandidateJudgeProviderID = authority.ProviderID
	config.ConfiguredCandidateJudgeProviderType = authority.ProviderType
	config.ConfiguredCandidateJudgeBaseURLSHA256 = authority.BaseURLSHA256
	config.ConfiguredCandidateJudgeAdapter = memoryjudge.ChatAdapterVersion
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeDevelopmentCriteriaVersionV2
	config.MaximumP95LatencyMillis = int(memoryeval.MemoryJudgeDevelopmentMaximumP95MillisV2)
	config.MaximumP99LatencyMillis = int(memoryeval.MemoryJudgeDevelopmentMaximumP99MillisV2)
	return config, nil
}

func CaptureFixedMemoryJudgeDevelopment(
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
	if !validFixedMemoryJudgeAuthority(authority) || judge == nil {
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
		usermemory.HybridShadowFixedMemoryJudgeDevelopmentPolicy(),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
}

func BuildFixedMemoryJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (FixedMemoryJudgeDevelopmentReport, []byte, error) {
	if profile.Profile.ReaderVersion != FixedMemoryJudgeReaderVersion ||
		!validFixedMemoryJudgeAuthority(authority) ||
		ValidateFixedMemoryJudgeCostAuthority(costBasis, authority) != nil {
		return FixedMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	criteria, err := memoryeval.MemoryJudgeDevelopmentCriteriaV2(pool.Corpus.Criteria)
	if err != nil {
		return FixedMemoryJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}
	legacyProfile := profile
	legacyProfile.Profile.ReaderVersion = CloudJudgeReaderVersion
	base, _, err := buildCloudJudgeDevelopmentReportForPolicy(
		pool,
		legacyProfile,
		usermemory.HybridShadowFixedMemoryJudgeDevelopmentPolicy(),
		criteria,
		fixedMemoryJudgeLegacyCostBasis(costBasis),
		true,
	)
	if err != nil {
		return FixedMemoryJudgeDevelopmentReport{}, nil, err
	}
	base.SchemaVersion = FixedMemoryJudgeReportSchemaVersion
	base.AdmissionMode = FixedMemoryJudgeAdmissionMode
	report := FixedMemoryJudgeDevelopmentReport{
		CloudJudgeDevelopmentReport: base,
		JudgeProviderID:             authority.ProviderID,
		JudgeProviderType:           authority.ProviderType,
		JudgeBaseURLSHA256:          authority.BaseURLSHA256,
		JudgeAdapter:                memoryjudge.ChatAdapterVersion,
		EvaluationCriteriaVersion:   memoryeval.MemoryJudgeDevelopmentCriteriaVersionV2,
		EvaluationCriteria:          criteria,
	}
	body, err := json.Marshal(report)
	if err != nil {
		return FixedMemoryJudgeDevelopmentReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func fixedMemoryJudgeLegacyCostBasis(cost CostBasis) CostBasis {
	authority := cost.ConfiguredCandidateJudgeAuthority
	mapped := cost
	mapped.SchemaVersion = "neo-chat.memory-regression-cost-basis.v3"
	mapped.ConfiguredCandidateJudgeAuthority = nil
	mapped.CloudJudgeAuthority = &CloudJudgeCostAuthority{
		ModelID:                          authority.ModelID,
		RequestCount:                     authority.RequestCount,
		MaximumInputTokens:               authority.MaximumInputTokens,
		MaximumOutputTokens:              authority.MaximumOutputTokens,
		InputMicrounitsPerMillionTokens:  authority.InputMicrounitsPerMillionTokens,
		OutputMicrounitsPerMillionTokens: authority.OutputMicrounitsPerMillionTokens,
		MaximumCostMicrounits:            authority.MaximumCostMicrounits,
	}
	return mapped
}

func BuildFixedMemoryJudgeDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report FixedMemoryJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !validFixedMemoryJudgeDevelopmentReport(report) ||
		!runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) != 1 {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(costBasisSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(report.ConfigurationSHA256); err != nil {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	expectedProfileID, err := candidateProfileID(providerMode)
	if err != nil || report.ProfileID != expectedProfileID {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest, err := buildRunArtifactManifest(artifacts)
	if err != nil || artifactManifest[0].Name != FixedMemoryJudgeArtifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion:       RelevanceRunManifestSchemaVersion,
		RunID:               runID,
		CaptureID:           captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       FixedMemoryJudgeAdmissionMode,
		PromotionEligible:   false,
		CaptureMode:         CaptureModeFixedMemoryJudge,
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
			"%w: encode fixed Memory Judge run manifest",
			ErrCaptureInvalid,
		)
	}
	return manifest, append(body, '\n'), nil
}

func validFixedMemoryJudgeDevelopmentReport(report FixedMemoryJudgeDevelopmentReport) bool {
	if err := memoryeval.ValidateMemoryJudgeDevelopmentCriteriaV2(
		report.EvaluationCriteria,
	); err != nil {
		return false
	}
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    report.JudgeProviderID,
		ProviderType:  report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256,
		ModelID:       report.JudgeModelID,
	}
	return report.SchemaVersion == FixedMemoryJudgeReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == FixedMemoryJudgeAdmissionMode &&
		!report.PromotionEligible && report.Split == DevelopmentCalibrationSplit &&
		report.CaseCount == 300 &&
		report.PolicyID == usermemory.HybridRelevanceFixedMemoryJudgePolicyID &&
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
			memoryeval.MemoryJudgeDevelopmentCriteriaVersionV2 &&
		len(report.ConfigurationSHA256) == 64 &&
		report.Evaluation.ProviderCostPassed == nil &&
		report.Diagnostics.FailureCodeCounts != nil &&
		report.Diagnostics.EmptyCandidateCaseCount >= 0 &&
		report.Diagnostics.JudgeCompletedCaseCount >= 0 &&
		report.Diagnostics.JudgeAbstainedCaseCount >= 0 &&
		report.Diagnostics.JudgeAbstainedCaseCount <=
			report.Diagnostics.JudgeCompletedCaseCount &&
		report.Diagnostics.FailedCaseCount >= 0 &&
		report.Diagnostics.EmptyCandidateCaseCount+
			report.Diagnostics.JudgeCompletedCaseCount+
			report.Diagnostics.FailedCaseCount == report.CaseCount &&
		report.CostAuthority.AuthorizedRequestCount == 300 &&
		report.CostAuthority.ActualRequestCount >= 0 &&
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
