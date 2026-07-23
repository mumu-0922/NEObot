package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
)

func TestToolCapabilityCacheUsesStatusSpecificTTLAndExpiry(t *testing.T) {
	repo := &fakeToolCapabilityCacheRepository{entries: map[string]ToolCapabilityCacheEntry{}}
	service := &Service{toolCapabilityRepo: repo}
	hash := strings.Repeat("a", 64)
	tests := []struct {
		status ToolCapabilityStatus
		want   time.Duration
	}{
		{ToolCapabilitySupported, toolCapabilitySupportedTTL},
		{ToolCapabilityUnsupported, toolCapabilityUnsupportedTTL},
		{ToolCapabilityUnknown, toolCapabilityUnknownTTL},
	}
	for _, test := range tests {
		modelID := "model-" + string(test.status)
		before := time.Now().UTC()
		if err := service.StoreToolCapability(
			context.Background(), hash, modelID, test.status, "fixture",
		); err != nil {
			t.Fatal(err)
		}
		entry := repo.entry(hash, modelID)
		ttl := entry.ExpiresAt.Sub(entry.CheckedAt)
		if ttl != test.want || entry.CheckedAt.Before(before.Add(-time.Second)) {
			t.Fatalf("%s cache ttl/time = %s / %s", test.status, ttl, entry.CheckedAt)
		}
		got, found, err := service.LookupToolCapability(
			context.Background(), hash, modelID,
		)
		if err != nil || !found || got.Status != test.status {
			t.Fatalf("lookup %s = %#v/%v/%v", test.status, got, found, err)
		}
	}

	expired := repo.entry(hash, "model-unknown")
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	repo.set(expired)
	if _, found, err := service.LookupToolCapability(
		context.Background(), hash, expired.ModelID,
	); err != nil || found {
		t.Fatalf("expired lookup found=%v err=%v", found, err)
	}
	if err := service.StoreToolCapability(
		context.Background(), hash, "model", "invalid", "fixture",
	); !errors.Is(err, ErrProviderConfigUnsupported) {
		t.Fatalf("invalid status error = %v", err)
	}
}

func TestToolCapabilityConfigHashInvalidatesOnProviderChanges(t *testing.T) {
	base := StoredProviderConfig{
		UserID:             "00000000-0000-0000-0000-000000000001",
		ProviderID:         "CUSTOM",
		EncryptedSecretRef: "vault-secret-a",
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindModel, Type: ProviderTypeOpenAICompatible,
			BaseURL: "https://provider.example/v1", Models: []string{"model-a"},
			ToolCapabilityDefault: ToolCapabilityAuto,
		},
	}
	baseHash := toolCapabilityConfigHash(base)
	if len(baseHash) != 64 {
		t.Fatalf("hash = %q", baseHash)
	}
	mutations := []func(*StoredProviderConfig){
		func(value *StoredProviderConfig) { value.Config.BaseURL = "https://other.example/v1" },
		func(value *StoredProviderConfig) { value.EncryptedSecretRef = "vault-secret-b" },
		func(value *StoredProviderConfig) { value.Config.Models = []string{"model-b"} },
		func(value *StoredProviderConfig) { value.Config.ToolCapabilityDefault = ToolCapabilityEnabled },
		func(value *StoredProviderConfig) {
			value.Config.ToolCapabilityModelOverrides = map[string]ToolCapabilityOverride{
				"model-a": ToolCapabilityDisabled,
			}
		},
	}
	for index, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		if got := toolCapabilityConfigHash(candidate); got == baseHash {
			t.Fatalf("mutation %d did not invalidate hash", index)
		}
	}
}

func TestProviderToolCapabilityOverrideRoundTripAndValidation(t *testing.T) {
	repo := &fakeProviderConfigRepository{}
	service := NewService(config.Config{}, WithProviderConfigRepository(repo))
	defaultValue := "enabled"
	created, err := service.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{
			Name: "Custom", Type: "OpenAI Compatible", Models: []string{"model-a", "model-b"},
			ToolCapabilityDefault: &defaultValue,
			ToolCapabilityModelOverrides: map[string]string{
				"model-b": "disabled",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.ToolCapability.Default != ToolCapabilityEnabled ||
		created.ToolCapability.ModelOverrides["model-b"] != ToolCapabilityDisabled {
		t.Fatalf("created capability = %#v", created.ToolCapability)
	}
	reloaded, err := service.AdminProviderConfigs(context.Background())
	if err != nil || len(reloaded.Providers) != 2 {
		t.Fatalf("reloaded providers = %#v, err=%v", reloaded, err)
	}
	if got := reloaded.Providers[1].ToolCapability; got.Default != ToolCapabilityEnabled ||
		got.ModelOverrides["model-b"] != ToolCapabilityDisabled {
		t.Fatalf("reloaded capability = %#v", got)
	}

	_, err = service.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{
			Name: "Custom", Type: "OpenAI Compatible", Models: []string{"model-a"},
			ToolCapabilityModelOverrides: map[string]string{"missing-model": "enabled"},
		},
	)
	if !errors.Is(err, ErrProviderConfigUnsupported) {
		t.Fatalf("unknown override model error = %v", err)
	}
}

func TestProviderSaveWarmupSelectsDefaultAndMatchingTaskModelsOffRequestPath(t *testing.T) {
	taskRepo := &fakeTaskModelSettingsRepository{
		found: true,
		stored: StoredTaskModelSettings{Models: TaskModels{
			TitleGeneration:  "CUSTOM:task-model",
			RelatedQuestions: "OTHER:other-model",
			RAGQuery:         "CUSTOM:task-model",
		}},
	}
	received := make(chan ToolCapabilityWarmupRequest, 1)
	contextActive := make(chan bool, 1)
	service := NewService(
		config.Config{},
		WithTaskModelSettingsRepository(taskRepo),
		WithToolCapabilityWarmupScheduler(func(
			ctx context.Context,
			request ToolCapabilityWarmupRequest,
		) {
			contextActive <- ctx.Err() == nil
			received <- request
		}),
	)
	requestCtx, cancel := context.WithCancel(auth.WithUser(
		context.Background(),
		auth.User{ID: "00000000-0000-0000-0000-000000000001"},
	))
	cancel()
	service.scheduleToolCapabilityWarmup(requestCtx, StoredProviderConfig{
		UserID:     "00000000-0000-0000-0000-000000000001",
		ProviderID: "CUSTOM",
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindModel, Enabled: true,
			Models: []string{"chat-model", "task-model", "other-model"},
		},
	})
	select {
	case request := <-received:
		if request.Provider.ID != "CUSTOM" || request.Provider.Source != "server-stored" ||
			len(request.ModelIDs) != 2 || request.ModelIDs[0] != "chat-model" ||
			request.ModelIDs[1] != "task-model" {
			t.Fatalf("warmup request = %#v", request)
		}
		if active := <-contextActive; !active {
			t.Fatal("warmup inherited cancelled request context")
		}
	case <-time.After(time.Second):
		t.Fatal("warmup was not scheduled")
	}
}

func TestPostgresToolCapabilityCacheIsSharedAndExpires(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hash := strings.Repeat("d", 64)
	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM model_tool_capability_cache WHERE provider_config_hash = $1`,
		hash,
	); err != nil {
		t.Fatal(err)
	}
	repoA := NewPostgresProviderConfigRepository(db)
	repoB := NewPostgresProviderConfigRepository(db)
	first := NewService(config.Config{}, WithProviderConfigRepository(repoA))
	second := NewService(config.Config{}, WithProviderConfigRepository(repoB))
	if err := first.StoreToolCapability(
		ctx, hash, "shared-model", ToolCapabilitySupported, "structured_tool_call",
	); err != nil {
		t.Fatal(err)
	}
	entry, found, err := second.LookupToolCapability(ctx, hash, "shared-model")
	if err != nil || !found || entry.Status != ToolCapabilitySupported {
		t.Fatalf("shared cache = %#v/%v/%v", entry, found, err)
	}
	if _, err := db.ExecContext(
		ctx,
		`UPDATE model_tool_capability_cache
SET checked_at = now() - interval '2 seconds',
    expires_at = now() - interval '1 second'
WHERE provider_config_hash = $1`,
		hash,
	); err != nil {
		t.Fatal(err)
	}
	if _, found, err := first.LookupToolCapability(ctx, hash, "shared-model"); err != nil || found {
		t.Fatalf("expired shared cache found=%v err=%v", found, err)
	}
}

type fakeToolCapabilityCacheRepository struct {
	mu      sync.Mutex
	entries map[string]ToolCapabilityCacheEntry
}

func (r *fakeToolCapabilityCacheRepository) GetToolCapabilityCache(
	_ context.Context,
	hash string,
	modelID string,
	now time.Time,
) (ToolCapabilityCacheEntry, bool, error) {
	entry := r.entry(hash, modelID)
	if entry.ProviderConfigHash == "" || !entry.ExpiresAt.After(now) {
		return ToolCapabilityCacheEntry{}, false, nil
	}
	return entry, true, nil
}

func (r *fakeToolCapabilityCacheRepository) UpsertToolCapabilityCache(
	_ context.Context,
	entry ToolCapabilityCacheEntry,
) error {
	r.set(entry)
	return nil
}

func (r *fakeToolCapabilityCacheRepository) entry(
	hash string,
	modelID string,
) ToolCapabilityCacheEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entries[hash+"\x00"+modelID]
}

func (r *fakeToolCapabilityCacheRepository) set(entry ToolCapabilityCacheEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[entry.ProviderConfigHash+"\x00"+entry.ModelID] = entry
}
