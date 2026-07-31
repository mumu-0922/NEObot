package memorycapture

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	ConfiguredCandidateJudgeReportSchemaVersion = "neo-chat.memory-regression-relevance-calibration.v10"
	ConfiguredCandidateJudgeAdmissionMode       = "development_configured_candidate_judge_only"
	ConfiguredCandidateJudgeArtifactName        = "configured-candidate-judge-development.json"
)

type ConfiguredCandidateJudgeDevelopmentReport struct {
	CloudJudgeDevelopmentReport
	JudgeProviderID    string `json:"judgeProviderId"`
	JudgeProviderType  string `json:"judgeProviderType"`
	JudgeBaseURLSHA256 string `json:"judgeBaseUrlSha256"`
	JudgeAdapter       string `json:"judgeAdapter"`
}

func BuildConfiguredCandidateJudgeDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	config, err := BuildCloudJudgeDevelopmentProfileConfig(
		protected,
		costBasisSHA256,
		providerMode,
		authority.ModelID,
		providerCostPolicy,
	)
	if err != nil || providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		!validConfiguredCandidateJudgeProfileAuthority(authority) {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	config.SchemaVersion = "neo-chat.memory-regression-profile-config.v10"
	config.ReaderVersion = ConfiguredCandidateJudgeReaderVersion
	config.CaptureMode = CaptureModeConfiguredCandidateJudge
	config.ConfiguredCandidateJudgeProviderID = authority.ProviderID
	config.ConfiguredCandidateJudgeProviderType = authority.ProviderType
	config.ConfiguredCandidateJudgeBaseURLSHA256 = authority.BaseURLSHA256
	config.ConfiguredCandidateJudgeAdapter = memoryjudge.ChatAdapterVersion
	return config, nil
}

func validConfiguredCandidateJudgeProfileAuthority(
	authority ConfiguredCandidateJudgeProfileAuthority,
) bool {
	if authority.ProviderID == "" || authority.ProviderID != strings.TrimSpace(authority.ProviderID) ||
		authority.ProviderType == "" || authority.ProviderType != strings.TrimSpace(authority.ProviderType) ||
		len(authority.BaseURLSHA256) != 64 ||
		authority.ModelID == "" || authority.ModelID != strings.TrimSpace(authority.ModelID) {
		return false
	}
	_, err := hex.DecodeString(authority.BaseURLSHA256)
	return err == nil
}

func CaptureConfiguredCandidateJudgeDevelopment(
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
	if !validConfiguredCandidateJudgeProfileAuthority(authority) {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	profile, err := CaptureCloudJudgeDevelopment(
		ctx,
		seedDB,
		runtimeDB,
		runID,
		fullPool,
		index,
		seed,
		provider,
		judge,
		authority.ModelID,
		profileID,
		configurationSHA256,
		cost,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = ConfiguredCandidateJudgeReaderVersion
	return profile, nil
}

// BuildConfiguredCandidateJudgeDevelopmentReport reuses the established
// strict ordinal/BGE evaluation while binding the exact configured chat
// Provider separately from historical SiliconFlow candidate-judge evidence.
func BuildConfiguredCandidateJudgeDevelopmentReport(
	pool memoryauthor.RegressionPool,
	profile CapturedProfile,
	authority ConfiguredCandidateJudgeProfileAuthority,
	costBasis CostBasis,
) (ConfiguredCandidateJudgeDevelopmentReport, []byte, error) {
	if profile.Profile.ReaderVersion != ConfiguredCandidateJudgeReaderVersion ||
		!validConfiguredCandidateJudgeProfileAuthority(authority) ||
		ValidateConfiguredCandidateJudgeCostAuthority(costBasis, authority) != nil {
		return ConfiguredCandidateJudgeDevelopmentReport{}, nil, ErrCaptureInvalid
	}

	legacyProfile := profile
	legacyProfile.Profile.ReaderVersion = CloudJudgeReaderVersion
	mappedCost := configuredCandidateJudgeLegacyCostBasis(costBasis)
	base, _, err := BuildCloudJudgeDevelopmentReport(
		pool,
		legacyProfile,
		authority.ModelID,
		mappedCost,
	)
	if err != nil {
		return ConfiguredCandidateJudgeDevelopmentReport{}, nil, err
	}
	base.SchemaVersion = ConfiguredCandidateJudgeReportSchemaVersion
	base.AdmissionMode = ConfiguredCandidateJudgeAdmissionMode
	report := ConfiguredCandidateJudgeDevelopmentReport{
		CloudJudgeDevelopmentReport: base,
		JudgeProviderID:             authority.ProviderID,
		JudgeProviderType:           authority.ProviderType,
		JudgeBaseURLSHA256:          authority.BaseURLSHA256,
		JudgeAdapter:                memoryjudge.ChatAdapterVersion,
	}
	body, err := json.Marshal(report)
	if err != nil {
		return ConfiguredCandidateJudgeDevelopmentReport{}, nil,
			errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func configuredCandidateJudgeLegacyCostBasis(cost CostBasis) CostBasis {
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

func BuildConfiguredCandidateJudgeDevelopmentRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	report ConfiguredCandidateJudgeDevelopmentReport,
	artifacts []Artifact,
) (RelevanceRunManifest, []byte, error) {
	if !validConfiguredCandidateJudgeDevelopmentReport(report) ||
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
	if err != nil || artifactManifest[0].Name != ConfiguredCandidateJudgeArtifactName {
		return RelevanceRunManifest{}, nil, ErrCaptureInvalid
	}
	manifest := RelevanceRunManifest{
		SchemaVersion:       RelevanceRunManifestSchemaVersion,
		RunID:               runID,
		CaptureID:           captureID,
		CorpusClass:         memoryeval.RegressionCorpusClass,
		AdmissionMode:       ConfiguredCandidateJudgeAdmissionMode,
		PromotionEligible:   false,
		CaptureMode:         CaptureModeConfiguredCandidateJudge,
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
			"%w: encode configured candidate-judge run manifest",
			ErrCaptureInvalid,
		)
	}
	return manifest, append(body, '\n'), nil
}

func validConfiguredCandidateJudgeDevelopmentReport(
	report ConfiguredCandidateJudgeDevelopmentReport,
) bool {
	authority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    report.JudgeProviderID,
		ProviderType:  report.JudgeProviderType,
		BaseURLSHA256: report.JudgeBaseURLSHA256,
		ModelID:       report.JudgeModelID,
	}
	return report.SchemaVersion == ConfiguredCandidateJudgeReportSchemaVersion &&
		report.CorpusClass == memoryeval.RegressionCorpusClass &&
		report.AdmissionMode == ConfiguredCandidateJudgeAdmissionMode &&
		!report.PromotionEligible && report.Split == DevelopmentCalibrationSplit &&
		report.CaseCount == 300 &&
		report.PolicyID == usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID &&
		report.ProviderEgressPolicy ==
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		report.ProviderCostPolicy == ProviderCostPolicyOwnerAuthorizedAbsoluteV1 &&
		report.ProviderCostAuthorized && report.Passed == report.Evaluation.Passed &&
		validConfiguredCandidateJudgeProfileAuthority(authority) &&
		report.JudgeAdapter == memoryjudge.ChatAdapterVersion &&
		report.JudgePromptVersion == usermemory.HybridCandidateJudgePromptVersion &&
		report.JudgePromptSHA256 == usermemory.HybridCandidateJudgePromptSHA256 &&
		report.JudgeDecodingProfile == usermemory.HybridCandidateJudgeDecodingProfile &&
		report.SelectionAlgorithm == cloudJudgeSelectionAlgorithm &&
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
