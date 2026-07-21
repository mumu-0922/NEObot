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
				Models: []string{"gpt-task", "gpt-other"}, Enabled: true,
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
	saved, err := service.UpdateAdminTaskModelSettings(
		context.Background(),
		TaskModelSettingsPatch{
			TitleGeneration:  &title,
			RelatedQuestions: &related,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Configured || saved.Models.TitleGeneration != "CUSTOM:gpt-task" ||
		saved.Models.RelatedQuestions != related || saved.UpdatedAt == nil {
		t.Fatalf("saved task models = %#v", saved)
	}

	public := service.PublicConfigForContext(context.Background())
	if public.ModelProvider.DefaultModels["titleGeneration"] != "CUSTOM:gpt-task" ||
		public.ModelProvider.DefaultModelsConfigured == nil ||
		!*public.ModelProvider.DefaultModelsConfigured {
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
