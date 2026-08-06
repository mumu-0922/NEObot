package memorycapture

import (
	"context"
	"database/sql"
	"fmt"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type ProfileHashes struct {
	Baseline           string
	Candidate          string
	BaselineProfileID  string
	CandidateProfileID string
}

func BuildProfileConfigs(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
) (ProfileConfig, ProfileConfig, error) {
	return buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeFullRegression,
		"all",
		usermemory.HybridShadowCalibrationPolicy(),
		"",
		nil,
	)
}

func BuildDevelopmentCalibrationProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
) (ProfileConfig, error) {
	_, candidate, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeCalibration,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowIntentCalibrationPolicy(),
		"",
		nil,
	)
	return candidate, err
}

func BuildCloudJudgeDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	judgeModelID string,
	providerCostPolicy string,
) (ProfileConfig, error) {
	_, candidate, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeCloudJudgeDevelopment,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowCloudJudgeCalibrationPolicy(judgeModelID),
		providerCostPolicy,
		nil,
	)
	return candidate, err
}

func BuildMemoryToolRouteDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority MemoryToolRouteProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	_, candidate, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeMemoryToolRouteDevelopment,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowMemoryFirstToolRoundCalibrationPolicy(authority.ModelID),
		providerCostPolicy,
		&authority,
	)
	return candidate, err
}

func BuildMemoryToolRouteDiagnosticProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority MemoryToolRouteProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	_, candidate, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeMemoryToolRouteDiagnostic,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowMemoryFirstToolRoundCalibrationPolicy(authority.ModelID),
		providerCostPolicy,
		&authority,
	)
	return candidate, err
}

func BuildFrozenValidationProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
) (ProfileConfig, error) {
	policy, ready := usermemory.HybridShadowFrozenPolicy()
	if !ready {
		return ProfileConfig{}, fmt.Errorf("%w: frozen relevance policy unavailable", ErrCaptureInvalid)
	}
	_, candidate, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeFrozenValidation,
		FrozenValidationSplit,
		policy,
		"",
		nil,
	)
	return candidate, err
}

func BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
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
		CaptureModeAccuracyFirstMemoryJudge,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
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
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3
	executionPolicy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.AccuracyFirstExecutionPolicy = &executionPolicy
	return config, nil
}

func BuildJudgeFailureDiagnosticDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	if !validFixedMemoryJudgeAuthority(authority) ||
		providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	_, config, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeJudgeFailureDiagnostic,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
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
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3
	executionPolicy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.AccuracyFirstExecutionPolicy = &executionPolicy
	return config, nil
}

func BuildTransportStableMemoryJudgeDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	if !validFixedMemoryJudgeAuthority(authority) ||
		providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	_, config, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeTransportStableMemoryJudge,
		DevelopmentCalibrationSplit,
		usermemory.HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy(),
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
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3
	executionPolicy, err := TransportStableDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.AccuracyFirstExecutionPolicy = &executionPolicy
	return config, nil
}

func BuildNegativePolicyGuardMemoryJudgeDevelopmentProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	if !validFixedMemoryJudgeAuthority(authority) ||
		providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	policy := usermemory.HybridShadowNegativePolicyGuardDevelopmentPolicy()
	_, config, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeNegativePolicyGuardMemoryJudge,
		DevelopmentCalibrationSplit,
		policy,
		providerCostPolicy,
		nil,
	)
	if err != nil {
		return ProfileConfig{}, err
	}
	descriptorSHA256, err := relevancePolicyDescriptorSHA256(policy)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.ConfiguredCandidateJudgeProviderID = authority.ProviderID
	config.ConfiguredCandidateJudgeProviderType = authority.ProviderType
	config.ConfiguredCandidateJudgeBaseURLSHA256 = authority.BaseURLSHA256
	config.ConfiguredCandidateJudgeAdapter = memoryjudge.ChatAdapterVersion
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3
	config.NegativePolicyQueryGuardRequired = true
	config.NegativePolicyQueryGuardVersion = usermemory.NegativePolicyQueryGuardVersion
	config.NegativePolicyQueryGuardSHA256 = usermemory.NegativePolicyQueryGuardSHA256
	config.RelevancePolicyDescriptorSHA256 = descriptorSHA256
	executionPolicy, err := TransportStableDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.AccuracyFirstExecutionPolicy = &executionPolicy
	return config, nil
}

func BuildProductionMemoryJudgeValidationProfileConfig(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	authority ConfiguredCandidateJudgeProfileAuthority,
	providerCostPolicy string,
) (ProfileConfig, error) {
	if !validFixedMemoryJudgeAuthority(authority) ||
		providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		return ProfileConfig{}, ErrCaptureInvalid
	}
	_, config, err := buildProfileConfigs(
		protected,
		costBasisSHA256,
		providerMode,
		CaptureModeProductionMemoryJudgeValidation,
		FrozenValidationSplit,
		usermemory.HybridShadowFixedMemoryJudgeProductionPolicy(),
		providerCostPolicy,
		nil,
	)
	if err != nil {
		return ProfileConfig{}, err
	}
	caseOrderSHA256, err := validationCaseOrderSHA256(protected.Pool)
	if err != nil {
		return ProfileConfig{}, err
	}
	criteriaSHA256, err := productionValidationCriteriaSHA256(protected.Pool.Corpus.Criteria)
	if err != nil {
		return ProfileConfig{}, err
	}
	policySHA256, err := productionRelevancePolicySHA256()
	if err != nil {
		return ProfileConfig{}, err
	}
	config.ConfiguredCandidateJudgeProviderID = authority.ProviderID
	config.ConfiguredCandidateJudgeProviderType = authority.ProviderType
	config.ConfiguredCandidateJudgeBaseURLSHA256 = authority.BaseURLSHA256
	config.ConfiguredCandidateJudgeAdapter = memoryjudge.ChatAdapterVersion
	config.EvaluationCriteriaVersion = memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3
	config.ValidationCaseOrderSHA256 = caseOrderSHA256
	config.EvaluationCriteriaSHA256 = criteriaSHA256
	config.ProductionRelevancePolicySHA256 = policySHA256
	config.MemoryReadIntentPolicyVersion = chat.MemoryReadIntentPolicyVersion
	config.MemoryReadIntentPolicySHA256 = chat.MemoryReadIntentPolicySHA256
	executionPolicy, err := ProductionMemoryJudgeValidationExecutionPolicy(providerMode)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.AccuracyFirstExecutionPolicy = &executionPolicy
	return config, nil
}

func buildProfileConfigs(
	protected ProtectedRegression,
	costBasisSHA256 string,
	providerMode string,
	captureMode string,
	evaluationSplit string,
	policy usermemory.HybridShadowRelevancePolicy,
	providerCostPolicy string,
	memoryToolRouteAuthority *MemoryToolRouteProfileAuthority,
) (ProfileConfig, ProfileConfig, error) {
	candidateProfileID, err := candidateProfileID(providerMode)
	if err != nil {
		return ProfileConfig{}, ProfileConfig{}, err
	}
	policyDescriptor, ok := usermemory.DescribeHybridShadowRelevancePolicy(policy)
	if !ok {
		return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
	}
	readerVersion := ReaderVersion
	profileSchemaVersion := "neo-chat.memory-regression-profile-config.v3"
	if captureMode == CaptureModeCloudJudgeDevelopment {
		readerVersion = CloudJudgeReaderVersion
		switch providerCostPolicy {
		case "":
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v4"
		case ProviderCostPolicyOwnerAuthorizedAbsoluteV1:
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v5"
		default:
			return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
		}
	} else if captureMode == CaptureModeFixedMemoryJudge {
		readerVersion = FixedMemoryJudgeReaderVersion
		profileSchemaVersion = "neo-chat.memory-regression-profile-config.v11"
		if providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			policy.ID != usermemory.HybridRelevanceFixedMemoryJudgePolicyID ||
			policyDescriptor.HardCutoffMilliseconds !=
				int(memoryeval.MemoryJudgeDevelopmentHardCutoffMillisV2) {
			return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
		}
	} else if captureMode == CaptureModeAccuracyFirstMemoryJudge ||
		captureMode == CaptureModeJudgeFailureDiagnostic ||
		captureMode == CaptureModeTransportStableMemoryJudge ||
		captureMode == CaptureModeNegativePolicyGuardMemoryJudge ||
		captureMode == CaptureModeProductionMemoryJudgeValidation {
		if captureMode == CaptureModeJudgeFailureDiagnostic {
			readerVersion = JudgeFailureDiagnosticReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v13"
			if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
				return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
			}
		} else if captureMode == CaptureModeTransportStableMemoryJudge {
			readerVersion = TransportStableMemoryJudgeReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v14"
			if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
				return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
			}
		} else if captureMode == CaptureModeProductionMemoryJudgeValidation {
			readerVersion = ProductionMemoryJudgeValidationReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v15"
			if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
				return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
			}
		} else if captureMode == CaptureModeNegativePolicyGuardMemoryJudge {
			readerVersion = NegativePolicyGuardMemoryJudgeReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v16"
			if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
				return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
			}
		} else {
			readerVersion = AccuracyFirstMemoryJudgeReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v12"
		}
		expectedPolicyID := usermemory.HybridRelevanceAccuracyFirstJudgePolicyID
		if captureMode == CaptureModeProductionMemoryJudgeValidation {
			expectedPolicyID = usermemory.HybridRelevanceProductionJudgePolicyID
		} else if captureMode == CaptureModeNegativePolicyGuardMemoryJudge {
			expectedPolicyID = usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID
		}
		if providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			policy.ID != expectedPolicyID ||
			policyDescriptor.HardCutoffMilliseconds != 0 {
			return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
		}
	} else if captureMode == CaptureModeMemoryToolRouteDevelopment ||
		captureMode == CaptureModeMemoryToolRouteDiagnostic {
		if captureMode == CaptureModeMemoryToolRouteDiagnostic {
			if memoryToolRouteFailureTaxonomySHA256() !=
				MemoryToolRouteFailureTaxonomySHA256 {
				return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
			}
			readerVersion = MemoryToolFirstRoundDiagnosticReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v9"
		} else {
			readerVersion = MemoryToolFirstRoundReaderVersion
			profileSchemaVersion = "neo-chat.memory-regression-profile-config.v7"
		}
		if providerCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			memoryToolRouteAuthority == nil ||
			memoryToolRouteAuthority.ProviderID == "" ||
			memoryToolRouteAuthority.ProviderType == "" ||
			len(memoryToolRouteAuthority.BaseURLSHA256) != 64 ||
			memoryToolRouteAuthority.ModelID != policy.MemoryToolRouteModelID ||
			policy.ID != usermemory.HybridRelevanceMemoryFirstToolRoundPolicyID ||
			policyDescriptor.MemoryToolRouteDecodingProfile != "none" ||
			policyDescriptor.MemoryToolRouteMaximumOutputTokens != 0 ||
			policyDescriptor.MemoryToolRouteTemperature != 0 ||
			policyDescriptor.MemoryToolRouteDisableThinking {
			return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
		}
	} else if providerCostPolicy != "" || memoryToolRouteAuthority != nil {
		return ProfileConfig{}, ProfileConfig{}, ErrCaptureInvalid
	}
	common := ProfileConfig{
		SchemaVersion:     profileSchemaVersion,
		ReaderVersion:     readerVersion,
		FixtureRawSHA256:  protected.FixtureRawSHA256,
		CorpusRawSHA256:   protected.CorpusRawSHA256,
		AuditRawSHA256:    protected.AuditRawSHA256,
		ManifestRawSHA256: protected.ManifestRawSHA256,
		CostBasisSHA256:   costBasisSHA256,
		CandidateLimit:    usermemory.MaxHybridShadowResults,
		FinalLimit:        usermemory.HybridShadowFinalLimit,
		TargetTokens:      usermemory.HybridShadowTargetTokens,
		MaximumTokens:     usermemory.HybridShadowMaximumTokens,
		HardCutoffMillis:  policyDescriptor.HardCutoffMilliseconds,
		FixtureMapping:    fixtureMappingVersion,
		CaptureMode:       captureMode,
		EvaluationSplit:   evaluationSplit,
	}
	if captureMode == CaptureModeMemoryToolRouteDiagnostic {
		common.MemoryToolRouteFailureTaxonomyVersion =
			MemoryToolRouteFailureTaxonomyVersion
		common.MemoryToolRouteFailureTaxonomySHA256 =
			MemoryToolRouteFailureTaxonomySHA256
		common.MemoryToolRouteDiagnosticCompleteness =
			MemoryToolRouteDiagnosticCompletenessPolicy
	}
	if captureMode == CaptureModeJudgeFailureDiagnostic {
		common.CandidateJudgeFailureTaxonomyVersion = memoryjudge.FailureTaxonomyVersion
		common.CandidateJudgeFailureTaxonomySHA256 = memoryjudge.FailureTaxonomySHA256
		common.CandidateJudgeDiagnosticCompleteness =
			JudgeFailureDiagnosticCompletenessPolicy
	}
	if captureMode == CaptureModeTransportStableMemoryJudge {
		common.CandidateJudgeFailureTaxonomyVersion = memoryjudge.FailureTaxonomyVersion
		common.CandidateJudgeFailureTaxonomySHA256 = memoryjudge.FailureTaxonomySHA256
		common.CandidateJudgeDiagnosticCompleteness =
			JudgeFailureDiagnosticCompletenessPolicy
	}
	if captureMode == CaptureModeNegativePolicyGuardMemoryJudge {
		common.CandidateJudgeFailureTaxonomyVersion = memoryjudge.FailureTaxonomyVersion
		common.CandidateJudgeFailureTaxonomySHA256 = memoryjudge.FailureTaxonomySHA256
		common.CandidateJudgeDiagnosticCompleteness =
			JudgeFailureDiagnosticCompletenessPolicy
	}
	if captureMode == CaptureModeProductionMemoryJudgeValidation {
		common.CandidateJudgeFailureTaxonomyVersion = memoryjudge.FailureTaxonomyVersion
		common.CandidateJudgeFailureTaxonomySHA256 = memoryjudge.FailureTaxonomySHA256
		common.CandidateJudgeDiagnosticCompleteness =
			JudgeFailureDiagnosticCompletenessPolicy
	}
	if captureMode == CaptureModeCalibration {
		common.CalibrationPlan = developmentCalibrationPlan()
	}
	baseline := common
	baseline.ProfileID = BaselineProfileID
	baseline.ProviderMode = ProviderModeNone
	baseline.CounterfactualInject = false
	baseline.RelevancePolicyID = "none"
	baseline.RelevancePolicyMode = "none"
	candidate := common
	candidate.ProfileID = candidateProfileID
	candidate.ProviderMode = providerMode
	candidate.EmbeddingProfileID = usermemory.HybridEmbeddingProfile
	candidate.EmbeddingModelID = ragproviders.SiliconFlowEmbeddingModel
	candidate.EmbeddingDimensions = ragproviders.SiliconFlowEmbeddingDimensions
	candidate.RerankModelID = ragproviders.SiliconFlowRerankModel
	candidate.CounterfactualInject = true
	candidate.RelevancePolicyID = policyDescriptor.ID
	candidate.RelevancePolicyMode = policyDescriptor.Mode
	candidate.MemoryIntentRequired = policyDescriptor.MemoryIntentRequired
	candidate.MemoryIntentAnchorVersion = policyDescriptor.MemoryIntentAnchorVersion
	candidate.MemoryIntentAnchorSHA256 = policyDescriptor.MemoryIntentAnchorSHA256
	candidate.MinimumMemoryIntentMarginBasisPoints =
		policyDescriptor.MinimumMemoryIntentMarginBasisPoints
	candidate.MinimumProviderSimilarityBasisPoints =
		policyDescriptor.MinimumProviderSimilarityBasisPoints
	candidate.MinimumFinalRelevanceBasisPoints =
		policyDescriptor.MinimumFinalRelevanceBasisPoints
	candidate.CloudCandidateJudgeRequired = policyDescriptor.CloudCandidateJudgeRequired
	candidate.CloudCandidateJudgeModelID = policyDescriptor.CloudCandidateJudgeModelID
	candidate.CloudCandidateJudgePromptVersion =
		policyDescriptor.CloudCandidateJudgePromptVersion
	candidate.CloudCandidateJudgePromptSHA256 =
		policyDescriptor.CloudCandidateJudgePromptSHA256
	candidate.CloudCandidateJudgeDecodingProfile =
		policyDescriptor.CloudCandidateJudgeDecodingProfile
	candidate.MemoryToolRouteRequired = policyDescriptor.MemoryToolRouteRequired
	candidate.MemoryToolRouteModelID = policyDescriptor.MemoryToolRouteModelID
	candidate.MemoryToolRouteContractVersion =
		policyDescriptor.MemoryToolRouteContractVersion
	candidate.MemoryToolRouteContractSHA256 =
		policyDescriptor.MemoryToolRouteContractSHA256
	if policyDescriptor.CloudCandidateJudgeRequired {
		candidate.ProviderEgressPolicy =
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1
		candidate.ProviderCostPolicy = providerCostPolicy
	} else if policyDescriptor.MemoryToolRouteRequired && memoryToolRouteAuthority != nil {
		temperature := policyDescriptor.MemoryToolRouteTemperature
		candidate.MemoryToolRouteProviderID = memoryToolRouteAuthority.ProviderID
		candidate.MemoryToolRouteProviderType = memoryToolRouteAuthority.ProviderType
		candidate.MemoryToolRouteBaseURLSHA256 = memoryToolRouteAuthority.BaseURLSHA256
		candidate.MemoryToolRouteAdapterVersion = chat.MemoryToolFirstRoundAdapterVersion
		if policyDescriptor.MemoryToolRouteDecodingProfile != "none" {
			candidate.MemoryToolRouteDecodingProfile =
				policyDescriptor.MemoryToolRouteDecodingProfile
			candidate.MemoryToolRouteMaximumOutputTokens =
				policyDescriptor.MemoryToolRouteMaximumOutputTokens
			candidate.MemoryToolRouteTemperature = &temperature
			candidate.MemoryToolRouteDisableThinking =
				policyDescriptor.MemoryToolRouteDisableThinking
		}
		candidate.ProviderEgressPolicy =
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1
		candidate.ProviderCostPolicy = providerCostPolicy
	}
	return baseline, candidate, nil
}

func HashProfileConfigs(baseline, candidate ProfileConfig) (ProfileHashes, error) {
	if baseline.ProfileID != BaselineProfileID || baseline.ProviderMode != ProviderModeNone {
		return ProfileHashes{}, ErrCaptureInvalid
	}
	expectedCandidateProfileID, err := candidateProfileID(candidate.ProviderMode)
	if err != nil || candidate.ProfileID != expectedCandidateProfileID {
		return ProfileHashes{}, ErrCaptureInvalid
	}
	baselineHash, err := ConfigurationSHA256(baseline)
	if err != nil {
		return ProfileHashes{}, err
	}
	candidateHash, err := ConfigurationSHA256(candidate)
	if err != nil {
		return ProfileHashes{}, err
	}
	return ProfileHashes{
		Baseline: baselineHash, Candidate: candidateHash,
		BaselineProfileID: baseline.ProfileID, CandidateProfileID: candidate.ProfileID,
	}, nil
}

func candidateProfileID(providerMode string) (string, error) {
	switch providerMode {
	case ProviderModeFakeProtocol:
		return FakeCandidateProfileID, nil
	case ProviderModeLiveSiliconFlow:
		return CandidateProfileID, nil
	default:
		return "", fmt.Errorf("%w: provider mode", ErrCaptureInvalid)
	}
}

// CaptureProfiles executes both native profiles against the same ephemeral
// state. It does not score or publish; callers must assemble and validate both
// complete observation sets before exclusive publication.
func CaptureProfiles(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	hashes ProfileHashes,
	cost CostBasis,
) (CapturedProfile, CapturedProfile, error) {
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	if provider == nil || len(seed.Cases) == 0 || hashes.Baseline == "" || hashes.Candidate == "" ||
		hashes.BaselineProfileID != BaselineProfileID || hashes.CandidateProfileID == "" {
		return CapturedProfile{}, CapturedProfile{}, ErrCaptureInvalid
	}
	repository := usermemory.NewPostgresRepository(runtimeDB)
	baselineService := usermemory.NewService(repository)
	baselineCases := make([]memoryeval.CaseObservation, 0, len(seed.Cases))
	for _, item := range seed.Cases {
		observed, err := CaptureBaseline(ctx, baselineService, index, item)
		if err != nil {
			return CapturedProfile{}, CapturedProfile{}, fmt.Errorf("capture baseline case: %w", err)
		}
		baselineCases = append(baselineCases, observed)
	}
	if err := resetEphemeralReaderState(ctx, seedDB, runID); err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}

	baseline := CapturedProfile{
		Profile: memoryeval.Profile{
			ID: hashes.BaselineProfileID, Role: "baseline", ReaderVersion: ReaderVersion,
			ConfigurationSHA256: hashes.Baseline,
			CandidateLimit:      usermemory.MaxHybridShadowResults,
			FinalLimit:          usermemory.HybridShadowFinalLimit,
		},
		Costs: cost.Baseline, Cases: baselineCases,
	}
	candidate, err := captureCandidateProfile(
		ctx, runtimeDB, index, seed.Cases, provider,
		usermemory.HybridShadowCalibrationPolicy(), hashes.CandidateProfileID,
		hashes.Candidate, cost.Candidate, nil, nil,
	)
	if err != nil {
		return CapturedProfile{}, CapturedProfile{}, err
	}
	return baseline, candidate, nil
}

// CaptureDevelopmentCalibration executes only the exact 300-case Development
// seed view with the all-scores calibration policy. Validation and the visible
// machine holdout cannot enter through this API.
func CaptureDevelopmentCalibration(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
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
	return captureCandidateProfile(
		ctx, runtimeDB, index, seed.Cases, provider,
		usermemory.HybridShadowIntentCalibrationPolicy(), profileID,
		configurationSHA256, cost, nil, nil,
	)
}

func CaptureCloudJudgeDevelopment(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
	judgeModelID string,
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
	if judge == nil {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	return captureCandidateProfile(
		ctx,
		runtimeDB,
		index,
		seed.Cases,
		provider,
		usermemory.HybridShadowCloudJudgeCalibrationPolicy(judgeModelID),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
}

func CaptureMemoryToolRouteDevelopment(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	router usermemory.HybridMemoryToolRouter,
	routerModelID string,
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
	if router == nil {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	return captureCandidateProfile(
		ctx,
		runtimeDB,
		index,
		seed.Cases,
		provider,
		usermemory.HybridShadowMemoryFirstToolRoundCalibrationPolicy(routerModelID),
		profileID,
		configurationSHA256,
		cost,
		nil,
		router,
	)
}

func CaptureMemoryToolRouteDiagnostic(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	router usermemory.HybridMemoryToolRouter,
	routerModelID string,
	profileID string,
	configurationSHA256 string,
	cost memoryeval.ProviderCosts,
) (CapturedProfile, error) {
	captured, err := CaptureMemoryToolRouteDevelopment(
		ctx,
		seedDB,
		runtimeDB,
		runID,
		fullPool,
		index,
		seed,
		provider,
		router,
		routerModelID,
		profileID,
		configurationSHA256,
		cost,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	captured.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	return captured, nil
}

func CaptureFrozenValidation(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	fullPool memoryauthor.RegressionPool,
	index FixtureIndex,
	seed SeedResult,
	provider usermemory.HybridShadowProvider,
	profileID string,
	configurationSHA256 string,
	cost memoryeval.ProviderCosts,
) (CapturedProfile, error) {
	if err := validateCaptureDatabases(ctx, seedDB, runtimeDB, runID, seed); err != nil {
		return CapturedProfile{}, err
	}
	if err := validateSeedSplit(fullPool, seed.Cases, FrozenValidationSplit); err != nil {
		return CapturedProfile{}, err
	}
	policy, ready := usermemory.HybridShadowFrozenPolicy()
	if !ready {
		return CapturedProfile{}, fmt.Errorf("%w: frozen relevance policy unavailable", ErrCaptureInvalid)
	}
	return captureCandidateProfile(
		ctx, runtimeDB, index, seed.Cases, provider, policy, profileID,
		configurationSHA256, cost, nil, nil,
	)
}

// CaptureProductionMemoryJudgeValidation executes only the frozen 100-case
// Validation split with the exact production BGE/Luna selection policy. A
// terminal per-case Provider failure is recorded fail-closed and the remaining
// ordered cases continue; only caller cancellation aborts the batch.
func CaptureProductionMemoryJudgeValidation(
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
	if err := validateSeedSplit(fullPool, seed.Cases, FrozenValidationSplit); err != nil {
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
		usermemory.HybridShadowFixedMemoryJudgeProductionPolicy(),
		profileID,
		configurationSHA256,
		cost,
		judge,
		nil,
	)
	if err != nil {
		return CapturedProfile{}, err
	}
	profile.Profile.ReaderVersion = ProductionMemoryJudgeValidationReaderVersion
	return profile, nil
}

func validateCaptureDatabases(
	ctx context.Context,
	seedDB *sql.DB,
	runtimeDB *sql.DB,
	runID string,
	seed SeedResult,
) error {
	if seed.RunID != runID || seed.DatabaseName == "" {
		return ErrCaptureInvalid
	}
	if err := verifySeedDatabase(ctx, seedDB, runID); err != nil {
		return err
	}
	if err := VerifyRuntimeDatabase(ctx, runtimeDB, runID); err != nil {
		return err
	}
	for _, db := range []*sql.DB{seedDB, runtimeDB} {
		var databaseName string
		if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil ||
			databaseName != seed.DatabaseName {
			return fmt.Errorf("%w: seeded database binding", ErrCaptureInvalid)
		}
	}
	return nil
}

func validateSeedSplit(
	pool memoryauthor.RegressionPool,
	cases []RuntimeCase,
	split string,
) error {
	expected := make([]string, 0, len(cases))
	for _, item := range pool.Corpus.Cases {
		if item.Split == split {
			expected = append(expected, item.ID)
		}
	}
	if len(expected) != len(cases) {
		return fmt.Errorf("%w: seeded split count", ErrCaptureInvalid)
	}
	for index, item := range cases {
		if item.CaseID != expected[index] {
			return fmt.Errorf("%w: seeded split order", ErrCaptureInvalid)
		}
	}
	return nil
}

func captureCandidateProfile(
	ctx context.Context,
	runtimeDB *sql.DB,
	index FixtureIndex,
	cases []RuntimeCase,
	provider usermemory.HybridShadowProvider,
	policy usermemory.HybridShadowRelevancePolicy,
	profileID string,
	configurationSHA256 string,
	cost memoryeval.ProviderCosts,
	judge usermemory.HybridCandidateJudge,
	router usermemory.HybridMemoryToolRouter,
) (CapturedProfile, error) {
	if runtimeDB == nil || provider == nil || len(cases) == 0 ||
		(profileID != CandidateProfileID && profileID != FakeCandidateProfileID) ||
		len(configurationSHA256) != 64 {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	policyDescriptor, ok := usermemory.DescribeHybridShadowRelevancePolicy(policy)
	if !ok || policyDescriptor.HardCutoffMilliseconds < 0 ||
		(policyDescriptor.HardCutoffMilliseconds == 0 &&
			policy.ID != usermemory.HybridRelevanceAccuracyFirstJudgePolicyID &&
			policy.ID != usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID &&
			policy.ID != usermemory.HybridRelevanceProductionJudgePolicyID) {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	repository := usermemory.NewPostgresRepository(runtimeDB)
	recorder := &Recorder{}
	decoratedRepository, err := NewRepositoryDecorator(repository, repository, recorder)
	if err != nil {
		return CapturedProfile{}, err
	}
	decoratedProvider, err := NewProviderDecorator(provider, recorder)
	if err != nil {
		return CapturedProfile{}, err
	}
	serviceOptions := []usermemory.ServiceOption{
		usermemory.WithHybridShadowProvider(decoratedProvider),
		usermemory.WithHybridShadowRelevancePolicy(policy),
	}
	if policy.CloudCandidateJudgeRequired {
		decoratedJudge, judgeErr := NewCandidateJudgeDecorator(
			judge,
			recorder,
			policy.CloudCandidateJudgeModelID,
		)
		if judgeErr != nil {
			return CapturedProfile{}, judgeErr
		}
		serviceOptions = append(
			serviceOptions,
			usermemory.WithHybridCandidateJudge(decoratedJudge),
		)
	} else if judge != nil {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	if policy.MemoryToolRouteRequired {
		decoratedRouter, routerErr := NewMemoryToolRouterDecorator(
			router,
			recorder,
			policy.MemoryToolRouteModelID,
		)
		if routerErr != nil {
			return CapturedProfile{}, routerErr
		}
		serviceOptions = append(
			serviceOptions,
			usermemory.WithHybridMemoryToolRouter(decoratedRouter),
		)
	} else if router != nil {
		return CapturedProfile{}, ErrCaptureInvalid
	}
	candidateService := usermemory.NewService(decoratedRepository, serviceOptions...)
	candidateCases := make([]memoryeval.CaseObservation, 0, len(cases))
	calibrationCases := make([]CandidateCalibrationTrace, 0, len(cases))
	for caseIndex, item := range cases {
		observed, calibration, err := captureCandidateWithCalibrationCutoff(
			ctx,
			candidateService,
			recorder,
			index,
			item,
			policyDescriptor.HardCutoffMilliseconds,
		)
		if err != nil {
			return CapturedProfile{}, fmt.Errorf("capture candidate case: %w", err)
		}
		candidateCases = append(candidateCases, observed)
		calibrationCases = append(calibrationCases, calibration)
		if (policy.ID == usermemory.HybridRelevanceAccuracyFirstJudgePolicyID ||
			policy.ID == usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID ||
			policy.ID == usermemory.HybridRelevanceProductionJudgePolicyID) &&
			caseIndex+1 < len(cases) {
			accuracyProvider, ok := provider.(*accuracyFirstHybridProvider)
			if !ok || accuracyProvider.controller == nil {
				return CapturedProfile{}, ErrCaptureInvalid
			}
			if err := accuracyProvider.controller.waitInterCaseCooldown(ctx); err != nil {
				return CapturedProfile{}, fmt.Errorf("accuracy-first inter-case cooldown: %w", err)
			}
		}
	}
	readerVersion := ReaderVersion
	providerEgressPolicy := ""
	if policy.CloudCandidateJudgeRequired {
		readerVersion = CloudJudgeReaderVersion
		if policy.ID == usermemory.HybridRelevanceFixedMemoryJudgePolicyID {
			readerVersion = FixedMemoryJudgeReaderVersion
		} else if policy.ID == usermemory.HybridRelevanceAccuracyFirstJudgePolicyID {
			readerVersion = AccuracyFirstMemoryJudgeReaderVersion
		} else if policy.ID == usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID {
			readerVersion = NegativePolicyGuardMemoryJudgeReaderVersion
		} else if policy.ID == usermemory.HybridRelevanceProductionJudgePolicyID {
			readerVersion = ProductionMemoryJudgeValidationReaderVersion
		}
		providerEgressPolicy =
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1
	} else if policy.MemoryToolRouteRequired {
		readerVersion = MemoryToolFirstRoundReaderVersion
		providerEgressPolicy =
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1
	}
	return CapturedProfile{
		Profile: memoryeval.Profile{
			ID: profileID, Role: "candidate", ReaderVersion: readerVersion,
			ConfigurationSHA256:  configurationSHA256,
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: providerEgressPolicy,
		},
		Costs: cost, Cases: candidateCases, Calibration: calibrationCases,
		ProviderAttempts: func() AccuracyFirstProviderTelemetry {
			if accuracyProvider, ok := provider.(*accuracyFirstHybridProvider); ok {
				return accuracyProvider.controller.Snapshot()
			}
			return AccuracyFirstProviderTelemetry{}
		}(),
	}, nil
}

func resetEphemeralReaderState(ctx context.Context, db *sql.DB, runID string) error {
	if err := verifySeedDatabase(ctx, db, runID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memories SET last_used_at = NULL WHERE last_used_at IS NOT NULL
`); err != nil {
		return fmt.Errorf("%w: reset baseline reader side effects", ErrCaptureStateConflict)
	}
	return nil
}
