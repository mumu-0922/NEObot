package memorycapture

import (
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestAuthorizeProviderModeKeepsFakeProtocolOffline(t *testing.T) {
	if err := AuthorizeProviderMode(ProviderModeFakeProtocol, "run-1", LiveAuthorization{}); err != nil {
		t.Fatalf("AuthorizeProviderMode(fake) error = %v", err)
	}
}

func TestAuthorizeProviderModeRequiresExactLiveAuthority(t *testing.T) {
	runID := "run-20260729"
	valid := LiveAuthorization{
		Enabled: true, Approval: LiveApproval, RunID: runID, ProviderID: "siliconflow",
		EmbeddingModelID:  ragproviders.SiliconFlowEmbeddingModel,
		RerankModelID:     ragproviders.SiliconFlowRerankModel,
		CloudJudgeModelID: DefaultSiliconFlowCloudJudgeModelID,
	}
	if err := AuthorizeProviderMode(ProviderModeLiveSiliconFlow, runID, valid); err != nil {
		t.Fatalf("AuthorizeProviderMode(live) error = %v", err)
	}
	if err := AuthorizeCloudJudgeTarget(
		ProviderModeLiveSiliconFlow,
		DefaultSiliconFlowCloudJudgeModelID,
		valid,
	); err != nil {
		t.Fatal(err)
	}
	driftedJudge := valid
	driftedJudge.CloudJudgeModelID = "other"
	assertLiveAuthorizationError(
		t,
		AuthorizeCloudJudgeTarget(
			ProviderModeLiveSiliconFlow,
			DefaultSiliconFlowCloudJudgeModelID,
			driftedJudge,
		),
		LiveAuthorizationCloudJudgeTarget,
	)

	tests := []struct {
		name string
		edit func(*LiveAuthorization)
		code string
	}{
		{name: "disabled", edit: func(value *LiveAuthorization) { value.Enabled = false }, code: LiveAuthorizationDisabled},
		{name: "approval", edit: func(value *LiveAuthorization) { value.Approval = "yes" }, code: LiveAuthorizationApproval},
		{name: "run", edit: func(value *LiveAuthorization) { value.RunID = "other" }, code: LiveAuthorizationRunID},
		{name: "provider", edit: func(value *LiveAuthorization) { value.ProviderID = "other" }, code: LiveAuthorizationProviderTarget},
		{name: "embedding", edit: func(value *LiveAuthorization) { value.EmbeddingModelID = "other" }, code: LiveAuthorizationProviderTarget},
		{name: "rerank", edit: func(value *LiveAuthorization) { value.RerankModelID = "other" }, code: LiveAuthorizationProviderTarget},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.edit(&candidate)
			assertLiveAuthorizationError(t, AuthorizeProviderMode(ProviderModeLiveSiliconFlow, runID, candidate), test.code)
		})
	}
}

func TestAuthorizeMemoryToolRouteTargetRequiresExactIndependentAuthority(t *testing.T) {
	authority := memoryToolRouteTestAuthority()
	valid := LiveAuthorization{
		MemoryToolRouteApproval:      LiveMemoryToolRouteApproval,
		MemoryToolRouteProviderID:    authority.ProviderID,
		MemoryToolRouteProviderType:  authority.ProviderType,
		MemoryToolRouteBaseURLSHA256: authority.BaseURLSHA256,
		MemoryToolRouteModelID:       authority.ModelID,
	}
	if err := AuthorizeMemoryToolRouteTarget(
		ProviderModeLiveSiliconFlow,
		authority,
		valid,
	); err != nil {
		t.Fatal(err)
	}
	tests := []func(*LiveAuthorization){
		func(value *LiveAuthorization) { value.MemoryToolRouteApproval = "yes" },
		func(value *LiveAuthorization) { value.MemoryToolRouteProviderID = "other" },
		func(value *LiveAuthorization) { value.MemoryToolRouteProviderType = "other" },
		func(value *LiveAuthorization) { value.MemoryToolRouteBaseURLSHA256 = strings.Repeat("c", 64) },
		func(value *LiveAuthorization) { value.MemoryToolRouteModelID = "other" },
	}
	for index, edit := range tests {
		candidate := valid
		edit(&candidate)
		assertLiveAuthorizationError(
			t,
			AuthorizeMemoryToolRouteTarget(
				ProviderModeLiveSiliconFlow,
				authority,
				candidate,
			),
			LiveAuthorizationMemoryToolRouteTarget,
		)
		_ = index
	}
}

func TestBuildProfileConfigsCannotMislabelFakeProtocol(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256: "fixture", CorpusRawSHA256: "corpus",
		AuditRawSHA256: "audit", ManifestRawSHA256: "manifest",
	}
	baseline, fake, err := BuildProfileConfigs(protected, "cost", ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ProfileID != BaselineProfileID || baseline.ProviderMode != ProviderModeNone ||
		fake.ProfileID != FakeCandidateProfileID || fake.ProviderMode != ProviderModeFakeProtocol {
		t.Fatalf("fake profiles = %#v / %#v", baseline, fake)
	}
	if _, _, err := BuildProfileConfigs(protected, "cost", "unknown"); err == nil {
		t.Fatal("unknown Provider mode was accepted")
	}
	calibration, err := BuildDevelopmentCalibrationProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calibration.SchemaVersion != "neo-chat.memory-regression-profile-config.v3" ||
		calibration.CaptureMode != CaptureModeCalibration ||
		calibration.EvaluationSplit != DevelopmentCalibrationSplit ||
		calibration.RelevancePolicyID != "memory_hybrid_relevance_intent_calibration_v1" ||
		calibration.RelevancePolicyMode != "intent_calibration" ||
		!calibration.MemoryIntentRequired ||
		calibration.MemoryIntentAnchorVersion != "memory-intent-bilingual-anchors-v1" ||
		len(calibration.MemoryIntentAnchorSHA256) != 64 ||
		calibration.MinimumMemoryIntentMarginBasisPoints != -100 ||
		calibration.MinimumProviderSimilarityBasisPoints != -100 ||
		calibration.MinimumFinalRelevanceBasisPoints != 0 ||
		calibration.CalibrationPlan == nil ||
		calibration.CalibrationPlan.SelectionAlgorithm != calibrationSelectionAlgorithm ||
		calibration.CalibrationPlan.IntentSelectionAlgorithm != calibrationIntentSelectionAlgorithm ||
		calibration.CalibrationPlan.DiagnosticsVersion != calibrationDiagnosticsVersion ||
		calibration.CalibrationPlan.Grid.AdmissionMinimumBasisPoints != -100 ||
		calibration.CalibrationPlan.Grid.IntentMinimumBasisPoints != -100 ||
		calibration.CalibrationPlan.Grid.IntentMaximumBasisPoints != 100 ||
		calibration.CalibrationPlan.Grid.AdmissionMaximumBasisPoints != 100 ||
		calibration.CalibrationPlan.Grid.FinalMinimumBasisPoints != 0 ||
		calibration.CalibrationPlan.Grid.FinalMaximumBasisPoints != 100 ||
		calibration.CalibrationPlan.Grid.StepBasisPoints != 1 {
		t.Fatalf("calibration profile config = %#v", calibration)
	}
	firstHash, err := ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	calibration.CalibrationPlan.Grid.StepBasisPoints = 2
	secondHash, err := ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("calibration grid drift did not change configuration hash")
	}
	calibration, err = BuildDevelopmentCalibrationProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, err = ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	calibration.CalibrationPlan.DiagnosticsVersion = "drifted"
	secondHash, err = ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("calibration diagnostics drift did not change configuration hash")
	}
	cloud, err := BuildCloudJudgeDevelopmentProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
		DefaultSiliconFlowCloudJudgeModelID,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cloud.SchemaVersion != "neo-chat.memory-regression-profile-config.v5" ||
		cloud.ReaderVersion != CloudJudgeReaderVersion ||
		cloud.CaptureMode != CaptureModeCloudJudgeDevelopment ||
		cloud.EvaluationSplit != DevelopmentCalibrationSplit ||
		cloud.RelevancePolicyID != usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID ||
		!cloud.CloudCandidateJudgeRequired ||
		cloud.CloudCandidateJudgeModelID != DefaultSiliconFlowCloudJudgeModelID ||
		cloud.CloudCandidateJudgePromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		cloud.CloudCandidateJudgePromptSHA256 != usermemory.HybridCandidateJudgePromptSHA256 ||
		cloud.CloudCandidateJudgeDecodingProfile != usermemory.HybridCandidateJudgeDecodingProfile ||
		cloud.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		cloud.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cloud.CalibrationPlan != nil {
		t.Fatalf("cloud profile config = %#v", cloud)
	}
	firstHash, err = ConfigurationSHA256(cloud)
	if err != nil {
		t.Fatal(err)
	}
	cloud.CloudCandidateJudgePromptSHA256 = strings.Repeat("f", 64)
	secondHash, err = ConfigurationSHA256(cloud)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("cloud judge prompt drift did not change configuration hash")
	}
	legacyCloud, err := BuildCloudJudgeDevelopmentProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
		DefaultSiliconFlowCloudJudgeModelID,
		"",
	)
	if err != nil || legacyCloud.SchemaVersion != "neo-chat.memory-regression-profile-config.v4" ||
		legacyCloud.ProviderCostPolicy != "" {
		t.Fatalf("legacy cloud profile config = %#v/%v", legacyCloud, err)
	}
	calibration, err = BuildDevelopmentCalibrationProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, err = ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	calibration.MemoryIntentAnchorVersion = "drifted"
	secondHash, err = ConfigurationSHA256(calibration)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("Memory intent anchor drift did not change configuration hash")
	}
	if _, err := BuildFrozenValidationProfileConfig(
		protected,
		"cost",
		ProviderModeFakeProtocol,
	); err == nil {
		t.Fatal("validation config was built before a policy was frozen")
	}
}

func TestLiveAuthorizationErrorDoesNotEchoDeniedTarget(t *testing.T) {
	err := AuthorizeProviderMode(
		ProviderModeLiveSiliconFlow,
		"run-1",
		LiveAuthorization{
			Enabled: true, Approval: LiveApproval, RunID: "run-1",
			ProviderID:       "sk-private-provider-value",
			EmbeddingModelID: "private-embedding-value",
			RerankModelID:    "private-rerank-value",
		},
	)
	for _, forbidden := range []string{
		"sk-private-provider-value", "private-embedding-value", "private-rerank-value",
	} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("authorization error leaked denied target: %v", err)
		}
	}
}

func assertLiveAuthorizationError(t *testing.T, err error, code string) {
	t.Helper()
	if !errors.Is(err, ErrLiveNotAuthorized) {
		t.Fatalf("error = %v, want ErrLiveNotAuthorized", err)
	}
	var authorizationError LiveAuthorizationError
	if !errors.As(err, &authorizationError) || authorizationError.Code != code {
		t.Fatalf("error = %#v, want code %q", err, code)
	}
}
