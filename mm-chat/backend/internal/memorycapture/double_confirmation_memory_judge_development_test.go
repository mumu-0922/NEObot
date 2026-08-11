package memorycapture

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestDoubleConfirmationDevelopmentIdentityAndCostIsolation(t *testing.T) {
	spec, err := doubleConfirmationDevelopmentReportSpec()
	if err != nil {
		t.Fatal(err)
	}
	if spec.readerVersion != DoubleConfirmationMemoryJudgeReaderVersion ||
		spec.reportSchemaVersion != DoubleConfirmationMemoryJudgeReportSchemaVersion ||
		spec.admissionMode != DoubleConfirmationMemoryJudgeAdmissionMode ||
		spec.policyID != usermemory.HybridRelevanceDoubleConfirmationDevelopmentPolicyID ||
		spec.maximumAbstentionConfirmations != 2 ||
		spec.authorizedRequestCount != 2700 {
		t.Fatalf("double-confirmation spec drifted: %#v", spec)
	}
	cost := doubleConfirmationDevelopmentTestCostBasis()
	if err := ValidateDoubleConfirmationDevelopmentCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAbstentionConfirmationDevelopmentCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("v3 accepted v4 cost authority")
	}
	old := abstentionConfirmationDevelopmentTestCostBasis()
	if err := ValidateDoubleConfirmationDevelopmentCostAuthority(
		old, FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("v4 accepted consumed v3 cost authority")
	}
}

func TestDoubleConfirmationDevelopmentProfileBindsV4Policy(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildDoubleConfirmationMemoryJudgeDevelopmentProfileConfig(
		protected, strings.Repeat("5", 64), ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := DoubleConfirmationDevelopmentExecutionPolicy(
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion !=
		"neo-chat.memory-regression-profile-config.v22-double-confirmation-development.v1" ||
		config.ReaderVersion != DoubleConfirmationMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeDoubleConfirmationMemoryJudge ||
		config.RelevancePolicyID !=
			usermemory.HybridRelevanceDoubleConfirmationDevelopmentPolicyID ||
		config.ConfiguredCandidateJudgeAdapter !=
			memoryjudge.BufferedChatAbstentionConfirmationAdapterVersion ||
		!config.CloudCandidateJudgeAbstentionConfirmationRequired ||
		config.CloudCandidateJudgeMaximumAbstentionConfirmations != 2 ||
		config.CloudCandidateJudgeConfirmationPromptVersion !=
			usermemory.HybridCandidateJudgeConfirmationPromptVersion ||
		config.CloudCandidateJudgeConfirmationPromptSHA256 !=
			usermemory.HybridCandidateJudgeConfirmationPromptSHA256 ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != execution ||
		execution.MaximumAbstentionConfirmationsPerLogicalRequest != 2 ||
		len(config.RelevancePolicyDescriptorSHA256) != 64 {
		t.Fatalf("double-confirmation config=%#v execution=%#v", config, execution)
	}

	single, err := BuildAbstentionConfirmationMemoryJudgeDevelopmentProfileConfig(
		protected, strings.Repeat("5", 64), ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if single.CloudCandidateJudgeMaximumAbstentionConfirmations != 0 {
		t.Fatalf("v3 profile serialized v4 maximum: %#v", single)
	}
}

func doubleConfirmationDevelopmentTestCostBasis() CostBasis {
	cost := abstentionConfirmationDevelopmentTestCostBasis()
	cost.SchemaVersion =
		"neo-chat.memory-regression-cost-basis.v22-double-confirmation-development.v1"
	authority := *cost.ConfiguredCandidateJudgeAuthority
	cost.ConfiguredCandidateJudgeAuthority = &authority
	authority.RequestCount = 2700
	authority.MaximumInputTokens = 2_700_000
	authority.MaximumOutputTokens =
		2700 * usermemory.HybridCandidateJudgeMaximumOutputTokens
	inputCost, _ := tokenCostCeiling(
		authority.MaximumInputTokens,
		authority.InputMicrounitsPerMillionTokens,
	)
	outputCost, _ := tokenCostCeiling(
		authority.MaximumOutputTokens,
		authority.OutputMicrounitsPerMillionTokens,
	)
	authority.MaximumCostMicrounits = inputCost + outputCost
	cost.Candidate.MemoryProviderCostMicrounits = authority.MaximumCostMicrounits
	return cost
}
