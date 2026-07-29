package memorycapture

import (
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/ragproviders"
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
		EmbeddingModelID: ragproviders.SiliconFlowEmbeddingModel,
		RerankModelID:    ragproviders.SiliconFlowRerankModel,
	}
	if err := AuthorizeProviderMode(ProviderModeLiveSiliconFlow, runID, valid); err != nil {
		t.Fatalf("AuthorizeProviderMode(live) error = %v", err)
	}

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
