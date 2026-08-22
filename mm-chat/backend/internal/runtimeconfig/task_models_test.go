package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestTaskModelSettingsStartUnconfiguredAndPersistValidPatch(t *testing.T) {
	providerRepo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID:     authDevelopmentUserID(),
			ProviderID: "CUSTOM",
			Label:      "Custom",
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindModel, Type: ProviderTypeOpenAICompatible,
				Models: []string{"gpt-task", "gpt-other", RecallFilteringModelID}, Enabled: true,
			},
		},
	}
	taskRepo := &fakeTaskModelSettingsRepository{}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(providerRepo),
		WithTaskModelSettingsRepository(taskRepo),
	)

	initial, err := service.AdminTaskModelSettings(context.Background())
	if err != nil || initial.Configured || initial.Models.TitleGeneration != "" {
		t.Fatalf("initial task models = %#v, %v", initial, err)
	}

	title := " CUSTOM:gpt-task "
	related := "CUSTOM:gpt-other"
	recallFiltering := "CUSTOM:" + RecallFilteringModelID
	saved, err := service.UpdateAdminTaskModelSettings(
		context.Background(),
		TaskModelSettingsPatch{
			TitleGeneration:  &title,
			RelatedQuestions: &related,
			RecallFiltering:  &recallFiltering,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Configured || saved.Models.TitleGeneration != "CUSTOM:gpt-task" ||
		saved.Models.RelatedQuestions != related ||
		saved.Models.RecallFiltering != recallFiltering || saved.UpdatedAt == nil {
		t.Fatalf("saved task models = %#v", saved)
	}

	public := service.PublicConfigForContext(context.Background())
	if public.ModelProvider.DefaultModels["titleGeneration"] != "CUSTOM:gpt-task" ||
		public.ModelProvider.DefaultModelsConfigured == nil ||
		!*public.ModelProvider.DefaultModelsConfigured ||
		public.ModelProvider.DefaultModels["recallFiltering"] != recallFiltering {
		t.Fatalf("public task models = %#v", public.ModelProvider)
	}
}

func TestTaskModelSettingsRejectUnknownOrDisabledModels(t *testing.T) {
	tests := []struct {
		name    string
		stored  StoredProviderConfig
		value   string
		wantErr error
	}{
		{
			name: "unknown model",
			stored: StoredProviderConfig{
				UserID: authDevelopmentUserID(), ProviderID: "CUSTOM",
				Config: StoredProviderConfigPayload{
					Kind: providerConfigKindModel, Models: []string{"known"}, Enabled: true,
				},
			},
			value: "CUSTOM:missing", wantErr: ErrTaskModelUnavailable,
		},
		{
			name: "disabled provider",
			stored: StoredProviderConfig{
				UserID: authDevelopmentUserID(), ProviderID: "CUSTOM",
				Config: StoredProviderConfigPayload{
					Kind: providerConfigKindModel, Models: []string{"known"}, Enabled: false,
				},
			},
			value: "CUSTOM:known", wantErr: ErrTaskModelUnavailable,
		},
		{
			name: "malformed reference",
			stored: StoredProviderConfig{
				UserID: authDevelopmentUserID(), ProviderID: "CUSTOM",
			},
			value: "known", wantErr: ErrTaskModelSettingsInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.value
			service := NewService(
				config.Config{},
				WithProviderConfigRepository(&fakeProviderConfigRepository{
					ok: true, stored: test.stored,
				}),
				WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{}),
			)
			_, err := service.UpdateAdminTaskModelSettings(
				context.Background(),
				TaskModelSettingsPatch{Memory: &value},
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}

	value := "CUSTOM:" + RecallFilteringModelID
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			ok: true,
			stored: StoredProviderConfig{
				UserID: authDevelopmentUserID(), ProviderID: "CUSTOM",
				Config: StoredProviderConfigPayload{
					Kind: providerConfigKindModel, Type: ProviderTypeOpenAI,
					Models: []string{RecallFilteringModelID}, Enabled: true,
				},
			},
		}),
		WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{}),
	)
	if _, err := service.UpdateAdminTaskModelSettings(
		context.Background(),
		TaskModelSettingsPatch{RecallFiltering: &value},
	); err != nil {
		t.Fatalf("OpenAI recall filtering Provider was rejected: %v", err)
	}
}

func TestResolveMemoryTaskModelPrefersConfiguredProvider(t *testing.T) {
	vault := testProviderSecretVault(t, "task-model-v1", 21)
	stored := testStoredVaultProvider(
		t, vault, "CUSTOM", ProviderTypeOpenAICompatible,
		"https://provider.example/v1", "fixture-secret",
	)
	stored.Config.Models = []string{"chat-model", "memory-model"}
	attestStoredProvider(&stored, true)
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			ok: true, stored: stored,
		}),
		WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{
			stored: StoredTaskModelSettings{Models: TaskModels{
				Memory: "CUSTOM:memory-model",
			}},
			found: true,
		}),
		WithProviderSecretVault(vault),
	)
	resolved, found, err := service.ResolveMemoryTaskModel(context.Background())
	if err != nil || !found || resolved.ModelID != "memory-model" ||
		resolved.Provider.ID != "CUSTOM" || resolved.Provider.APIKey != "fixture-secret" {
		t.Fatalf("ResolveMemoryTaskModel() = %#v/%t/%v", resolved, found, err)
	}
}

func TestResolveMemoryTaskModelFallsBackOnlyWhenUnconfigured(t *testing.T) {
	tests := []struct {
		name    string
		repo    TaskModelSettingsRepository
		found   bool
		wantErr error
	}{
		{name: "repository unavailable", repo: nil, found: false},
		{
			name: "memory task model empty",
			repo: &fakeTaskModelSettingsRepository{
				stored: StoredTaskModelSettings{Models: TaskModels{Memory: ""}},
				found:  true,
			},
			found: false,
		},
		{
			name: "configured model malformed",
			repo: &fakeTaskModelSettingsRepository{
				stored: StoredTaskModelSettings{Models: TaskModels{Memory: "malformed"}},
				found:  true,
			},
			found:   false,
			wantErr: ErrTaskModelSettingsInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := []ServiceOption{}
			if test.repo != nil {
				options = append(options, WithTaskModelSettingsRepository(test.repo))
			}
			service := NewService(config.Config{}, options...)
			_, found, err := service.ResolveMemoryTaskModel(context.Background())
			if found != test.found || !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolveMemoryTaskModel() = found:%t err:%v", found, err)
			}
		})
	}
}

func TestRecallFilteringTaskModelPinsLunaAndOpenAIProvider(t *testing.T) {
	tests := []struct {
		name   string
		config StoredProviderConfigPayload
		value  string
	}{
		{
			name: "different model",
			config: StoredProviderConfigPayload{
				Kind: providerConfigKindModel, Type: ProviderTypeOpenAICompatible,
				Models: []string{"gpt-5.6-sol"}, Enabled: true,
			},
			value: "CUSTOM:gpt-5.6-sol",
		},
		{
			name: "non OpenAI provider",
			config: StoredProviderConfigPayload{
				Kind: providerConfigKindModel, Type: ProviderTypeGemini,
				Models: []string{RecallFilteringModelID}, Enabled: true,
			},
			value: "CUSTOM:" + RecallFilteringModelID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.value
			service := NewService(
				config.Config{},
				WithProviderConfigRepository(&fakeProviderConfigRepository{
					ok: true,
					stored: StoredProviderConfig{
						UserID: authDevelopmentUserID(), ProviderID: "CUSTOM",
						Config: test.config,
					},
				}),
				WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{}),
			)
			_, err := service.UpdateAdminTaskModelSettings(
				context.Background(),
				TaskModelSettingsPatch{RecallFiltering: &value},
			)
			if !errors.Is(err, ErrTaskModelUnavailable) {
				t.Fatalf("error = %v, want %v", err, ErrTaskModelUnavailable)
			}
		})
	}
}

func TestResolveRecallFilteringTaskModelUsesExplicitProviderOnly(t *testing.T) {
	vault := testProviderSecretVault(t, "recall-filter-provider-v1", 22)
	stored := testStoredVaultProvider(
		t, vault, "CUSTOM", ProviderTypeOpenAI,
		"https://provider.example/v1", "fixture-secret",
	)
	stored.Config.Models = []string{RecallFilteringModelID}
	attestStoredProvider(&stored, true)
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{ok: true, stored: stored}),
		WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{
			stored: StoredTaskModelSettings{Models: TaskModels{
				RecallFiltering: "CUSTOM:" + RecallFilteringModelID,
			}},
			found: true,
		}),
		WithProviderSecretVault(vault),
	)
	resolved, configured, err := service.ResolveRecallFilteringTaskModel(
		context.Background(),
	)
	if err != nil || !configured || resolved.Provider.ID != "CUSTOM" ||
		resolved.ModelID != RecallFilteringModelID ||
		resolved.Provider.APIKey != "fixture-secret" {
		t.Fatalf("ResolveRecallFilteringTaskModel() = %#v/%t/%v", resolved, configured, err)
	}
}

func TestRecallFilteringTaskModelAuthorityPreservesLegacyDefault(t *testing.T) {
	service := NewService(
		config.Config{},
		WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{
			stored: StoredTaskModelSettings{Models: TaskModels{}},
			found:  true,
		}),
	)
	authority, err := service.RecallFilteringTaskModelAuthority(context.Background())
	if err != nil || authority.ProviderID != serverDefaultProviderID ||
		authority.ModelID != RecallFilteringModelID || authority.Configured {
		t.Fatalf("legacy recall filtering authority = %#v/%v", authority, err)
	}
	_, configured, err := service.ResolveRecallFilteringTaskModel(context.Background())
	if err != nil || configured {
		t.Fatalf("legacy resolution configured=%t err=%v", configured, err)
	}
}

func TestPostgresTaskModelSettingsSurviveRepositoryReload(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	userID := authDevelopmentUserID()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, 'Task Model Test')
ON CONFLICT (id) DO NOTHING;
DELETE FROM task_model_settings WHERE user_id = $1;
`, userID); err != nil {
		t.Fatal(err)
	}

	first := NewPostgresTaskModelSettingsRepository(db)
	want := TaskModels{
		TitleGeneration: "CUSTOM:gpt-title",
		Memory:          "CUSTOM:gpt-memory",
		RecallFiltering: "CUSTOM:" + RecallFilteringModelID,
	}
	if _, err := first.UpsertTaskModelSettings(ctx, userID, want); err != nil {
		t.Fatal(err)
	}

	restarted := NewPostgresTaskModelSettingsRepository(db)
	got, found, err := restarted.GetTaskModelSettings(ctx, userID)
	if err != nil || !found || got.Models != want || got.UpdatedAt.IsZero() {
		t.Fatalf("reloaded task models = %#v/%t/%v", got, found, err)
	}
	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM task_model_settings WHERE user_id = $1`,
		userID,
	); err != nil {
		t.Fatal(err)
	}
}

type fakeTaskModelSettingsRepository struct {
	stored StoredTaskModelSettings
	found  bool
	err    error
}

func (r *fakeTaskModelSettingsRepository) GetTaskModelSettings(
	_ context.Context,
	_ string,
) (StoredTaskModelSettings, bool, error) {
	return r.stored, r.found, r.err
}

func (r *fakeTaskModelSettingsRepository) UpsertTaskModelSettings(
	_ context.Context,
	_ string,
	models TaskModels,
) (StoredTaskModelSettings, error) {
	if r.err != nil {
		return StoredTaskModelSettings{}, r.err
	}
	r.stored = StoredTaskModelSettings{Models: models, UpdatedAt: time.Now().UTC()}
	r.found = true
	return r.stored, nil
}

var _ TaskModelSettingsRepository = (*fakeTaskModelSettingsRepository)(nil)
