package memorycapture

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoricalProfileConfigurationBytesExcludeV12ExecutionPolicy(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	costHash := strings.Repeat("5", 64)
	configuredAuthority := ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    "configured",
		ProviderType:  "openai_compatible",
		BaseURLSHA256: strings.Repeat("6", 64),
		ModelID:       "configured-model",
	}
	routeAuthority := MemoryToolRouteProfileAuthority{
		ProviderID:    "configured",
		ProviderType:  "openai_compatible",
		BaseURLSHA256: strings.Repeat("7", 64),
		ModelID:       "configured-model",
	}

	profiles := make(map[string]ProfileConfig)
	var err error
	profiles["v4"], err = BuildCloudJudgeDevelopmentProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		"Qwen/Qwen3-8B",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles["v5"], err = BuildCloudJudgeDevelopmentProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		"Qwen/Qwen3-8B",
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles["v7"], err = BuildMemoryToolRouteDevelopmentProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		routeAuthority,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles["v9"], err = BuildMemoryToolRouteDiagnosticProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		routeAuthority,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles["v10"], err = BuildConfiguredCandidateJudgeDevelopmentProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		configuredAuthority,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles["v11"], err = BuildFixedMemoryJudgeDevelopmentProfileConfig(
		protected,
		costHash,
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}

	expectedHashes := map[string]string{
		"v4":  "fd58dc9305bb0e2014b619ff51f538fbe799531c0e35f2ce7fbc478ce3909288",
		"v5":  "fabaff6232b68089678aff9d789216fe04bb8023f99ada16a8e857bc566a5e85",
		"v7":  "f223e4a9a38896eae342cc568cd831856e056d4c360baa51914f1cb2b413c6d0",
		"v9":  "f7b1e2c94b02b69f3660161d97142e9a8a8e221891bb7bdf332917bbc2031b7a",
		"v10": "7c3a2efe38cd24092f19387e8c09cd1176a27f826184b377dfd29c5b0bb9b5b6",
		"v11": "81b08afd95bbd8caf28adeedcdfb8625c419b3037ef5cd9874dc0b868f3dc88f",
	}
	for name, profile := range profiles {
		body, marshalErr := json.Marshal(profile)
		if marshalErr != nil {
			t.Fatalf("%s marshal: %v", name, marshalErr)
		}
		if bytes.Contains(body, []byte(`"accuracyFirstExecutionPolicy"`)) {
			t.Fatalf("%s emitted the schema-v12 execution field", name)
		}
		hash, hashErr := ConfigurationSHA256(profile)
		if hashErr != nil {
			t.Fatalf("%s hash: %v", name, hashErr)
		}
		if hash != expectedHashes[name] {
			t.Fatalf("%s configuration hash = %s", name, hash)
		}
	}
}
