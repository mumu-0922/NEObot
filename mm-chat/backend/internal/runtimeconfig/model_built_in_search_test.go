package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/websearch"
)

type modelBuiltInSearchTesterFunc func(context.Context, ModelBuiltInSearchTestInput) (int, error)

func (f modelBuiltInSearchTesterFunc) TestModelBuiltInSearch(
	ctx context.Context,
	input ModelBuiltInSearchTestInput,
) (int, error) {
	return f(ctx, input)
}

func TestCustomModelBuiltInSearchRequiresBoundedAttestation(t *testing.T) {
	vault := testProviderSecretVault(t, "built-in-v1", 61)
	stored := testStoredVaultProvider(
		t, vault, "COMPATIBLE", ProviderTypeOpenAICompatible,
		"https://relay.example/v1", "compatible-fixture",
	)
	stored.Config.Models = []string{"gpt-search", "gpt-other"}
	stored.Config.ModelBuiltInSearchProtocol = ModelBuiltInSearchProtocolOpenAIResponses
	stored.Config.ModelBuiltInSearchModel = "gpt-search"
	attestStoredProvider(&stored, true)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	var tested ModelBuiltInSearchTestInput
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
		WithModelBuiltInSearchTester(modelBuiltInSearchTesterFunc(func(
			_ context.Context,
			input ModelBuiltInSearchTestInput,
		) (int, error) {
			tested = input
			return 2, nil
		})),
	)

	if _, err := service.ResolveModelBuiltIn(
		context.Background(),
		websearch.ModelBuiltInResolutionRequest{
			ProviderID: "COMPATIBLE", ModelID: "gpt-search",
			Protocol: websearch.ModelBuiltInOpenAI,
		},
	); !errors.Is(err, websearch.ErrNotConfigured) {
		t.Fatalf("unattested resolver error = %v", err)
	}

	response, err := service.TestAdminModelBuiltInSearchConnection(
		context.Background(),
		"COMPATIBLE",
		TestAdminModelBuiltInSearchRequest{
			Protocol: ModelBuiltInSearchProtocolOpenAIResponses,
			Model:    "gpt-search",
		},
	)
	if err != nil {
		t.Fatalf("TestAdminModelBuiltInSearchConnection() error = %v", err)
	}
	if response.SourceCount != 2 || !response.Provider.ModelBuiltInSearch.ConnectionTestValid ||
		response.Provider.ModelBuiltInSearch.Source != "custom" ||
		tested.APIKey != "compatible-fixture" || tested.Model != "gpt-search" {
		t.Fatalf("test response/input = %#v / %#v", response, tested)
	}

	execution, err := service.ResolveModelBuiltIn(
		context.Background(),
		websearch.ModelBuiltInResolutionRequest{
			ProviderID: "COMPATIBLE", ModelID: "gpt-search",
			Protocol: websearch.ModelBuiltInOpenAI,
		},
	)
	if err != nil || execution.Mode != websearch.ExecutionModelBuiltIn ||
		execution.ModelBuiltIn != websearch.ModelBuiltInOpenAI {
		t.Fatalf("attested execution = %#v, %v", execution, err)
	}
	if _, err := service.ResolveModelBuiltIn(
		context.Background(),
		websearch.ModelBuiltInResolutionRequest{
			ProviderID: "COMPATIBLE", ModelID: "gpt-other",
			Protocol: websearch.ModelBuiltInOpenAI,
		},
	); !errors.Is(err, websearch.ErrNotConfigured) {
		t.Fatalf("unattested model-switch error = %v", err)
	}

	repo.stored.Config.BaseURL = "https://changed.example/v1"
	repo.stored.Config.ConnectionTestSHA256 = ProviderConnectionTestFingerprint(repo.stored)
	repo.stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if ModelBuiltInSearchConnectionTestValid(repo.stored) {
		t.Fatal("base URL change retained built-in Search attestation")
	}
}

func TestCustomModelBuiltInSearchAttestationInvalidatesOnBoundFieldChanges(t *testing.T) {
	base := StoredProviderConfig{
		ProviderID:         "COMPATIBLE",
		EncryptedSecretRef: "encrypted-secret-ref-v1",
		Config: StoredProviderConfigPayload{
			Kind:                       providerConfigKindModel,
			Type:                       ProviderTypeOpenAICompatible,
			BaseURL:                    "https://relay.example/v1",
			Models:                     []string{"gpt-search", "gpt-other"},
			ModelBuiltInSearchProtocol: ModelBuiltInSearchProtocolOpenAIResponses,
			ModelBuiltInSearchModel:    "gpt-search",
			ModelBuiltInSearchTestedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
	base.Config.ModelBuiltInSearchTestSHA256 = modelBuiltInSearchFingerprint(
		base,
		base.Config.ModelBuiltInSearchProtocol,
		base.Config.ModelBuiltInSearchModel,
	)
	if !ModelBuiltInSearchConnectionTestValid(base) {
		t.Fatal("base attestation is invalid")
	}

	mutations := map[string]func(*StoredProviderConfig){
		"provider": func(stored *StoredProviderConfig) { stored.ProviderID = "OTHER" },
		"type": func(stored *StoredProviderConfig) {
			stored.Config.Type = ProviderTypeOpenAI
		},
		"base URL": func(stored *StoredProviderConfig) {
			stored.Config.BaseURL = "https://changed.example/v1"
		},
		"secret": func(stored *StoredProviderConfig) {
			stored.EncryptedSecretRef = "encrypted-secret-ref-v2"
		},
		"protocol": func(stored *StoredProviderConfig) {
			stored.Config.ModelBuiltInSearchProtocol = ModelBuiltInSearchProtocolGeminiGoogle
		},
		"model": func(stored *StoredProviderConfig) {
			stored.Config.ModelBuiltInSearchModel = "gpt-other"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Config.Models = append([]string(nil), base.Config.Models...)
			mutate(&changed)
			if ModelBuiltInSearchConnectionTestValid(changed) {
				t.Fatalf("%s change retained built-in Search attestation", name)
			}
		})
	}
}

func TestOfficialModelBuiltInSearchIsExactProviderModelAndProtocol(t *testing.T) {
	tests := []struct {
		providerType ProviderType
		model        string
		protocol     websearch.ModelBuiltInProviderID
	}{
		{ProviderTypeOpenAI, "gpt-5", websearch.ModelBuiltInOpenAI},
		{ProviderTypeGemini, "gemini-2.5-flash", websearch.ModelBuiltInGemini},
		{ProviderTypeAnthropic, "claude-sonnet-4-5", websearch.ModelBuiltInAnthropic},
	}
	for _, test := range tests {
		t.Run(string(test.providerType), func(t *testing.T) {
			vault := testProviderSecretVault(t, "official-v1", byte(62+len(test.model)))
			stored := testStoredVaultProvider(
				t, vault, "OFFICIAL", test.providerType, "default", "official-fixture",
			)
			stored.Config.Models = []string{test.model}
			attestStoredProvider(&stored, true)
			service := NewService(
				config.Config{},
				WithProviderConfigRepository(&fakeProviderConfigRepository{ok: true, stored: stored}),
				WithProviderSecretVault(vault),
			)
			execution, err := service.ResolveModelBuiltIn(
				context.Background(),
				websearch.ModelBuiltInResolutionRequest{
					ProviderID: "OFFICIAL", ModelID: test.model, Protocol: test.protocol,
				},
			)
			if err != nil || execution.ModelBuiltIn != test.protocol {
				t.Fatalf("official execution = %#v, %v", execution, err)
			}
			if _, err := service.ResolveModelBuiltIn(
				context.Background(),
				websearch.ModelBuiltInResolutionRequest{
					ProviderID: "OTHER", ModelID: test.model, Protocol: test.protocol,
				},
			); !errors.Is(err, websearch.ErrNotConfigured) {
				t.Fatalf("wrong provider error = %v", err)
			}
		})
	}
}

func TestFailedCustomModelBuiltInSearchDoesNotAttest(t *testing.T) {
	vault := testProviderSecretVault(t, "built-in-v1", 71)
	stored := testStoredVaultProvider(
		t, vault, "COMPATIBLE", ProviderTypeOpenAICompatible,
		"https://relay.example/v1", "compatible-fixture",
	)
	stored.Config.Models = []string{"gpt-search"}
	stored.Config.ModelBuiltInSearchProtocol = ModelBuiltInSearchProtocolOpenAIResponses
	stored.Config.ModelBuiltInSearchModel = "gpt-search"
	attestStoredProvider(&stored, true)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
		WithModelBuiltInSearchTester(modelBuiltInSearchTesterFunc(func(
			context.Context, ModelBuiltInSearchTestInput,
		) (int, error) {
			return 0, errors.New("private upstream detail")
		})),
	)
	_, err := service.TestAdminModelBuiltInSearchConnection(
		context.Background(), "COMPATIBLE",
		TestAdminModelBuiltInSearchRequest{
			Protocol: ModelBuiltInSearchProtocolOpenAIResponses, Model: "gpt-search",
		},
	)
	if !errors.Is(err, ErrModelBuiltInSearchTestFailed) ||
		repo.stored.Config.ModelBuiltInSearchTestSHA256 != "" ||
		repo.stored.Config.ModelBuiltInSearchTestedAt != "" {
		t.Fatalf("failed test state/error = %#v / %v", repo.stored.Config, err)
	}
}
