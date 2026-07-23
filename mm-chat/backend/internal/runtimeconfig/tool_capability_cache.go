package runtimeconfig

import (
	"context"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type ToolCapabilityStatus string

const (
	ToolCapabilitySupported   ToolCapabilityStatus = "supported"
	ToolCapabilityUnsupported ToolCapabilityStatus = "unsupported"
	ToolCapabilityUnknown     ToolCapabilityStatus = "unknown"

	toolCapabilitySupportedTTL   = 7 * 24 * time.Hour
	toolCapabilityUnsupportedTTL = 24 * time.Hour
	toolCapabilityUnknownTTL     = 5 * time.Minute
)

type ToolCapabilityCacheEntry struct {
	ProviderConfigHash string
	ModelID            string
	Status             ToolCapabilityStatus
	Category           string
	CheckedAt          time.Time
	ExpiresAt          time.Time
}

// ToolCapabilityWarmupRequest contains only server-owned provider identity and
// bounded model identifiers. It intentionally carries no API key, prompt,
// conversation, catalog, or provider payload.
type ToolCapabilityWarmupRequest struct {
	Provider ProviderRuntimeConfig
	ModelIDs []string
}

type ToolCapabilityCacheRepository interface {
	GetToolCapabilityCache(
		context.Context,
		string,
		string,
		time.Time,
	) (ToolCapabilityCacheEntry, bool, error)
	UpsertToolCapabilityCache(
		context.Context,
		ToolCapabilityCacheEntry,
	) error
}

func (s *Service) LookupToolCapability(
	ctx context.Context,
	providerConfigHash string,
	modelID string,
) (ToolCapabilityCacheEntry, bool, error) {
	if s == nil || s.toolCapabilityRepo == nil {
		return ToolCapabilityCacheEntry{}, false, nil
	}
	providerConfigHash = strings.ToLower(strings.TrimSpace(providerConfigHash))
	modelID = strings.TrimSpace(modelID)
	if !validToolCapabilityCacheKey(providerConfigHash, modelID) {
		return ToolCapabilityCacheEntry{}, false, nil
	}
	return s.toolCapabilityRepo.GetToolCapabilityCache(
		ctx,
		providerConfigHash,
		modelID,
		time.Now().UTC(),
	)
}

func (s *Service) StoreToolCapability(
	ctx context.Context,
	providerConfigHash string,
	modelID string,
	status ToolCapabilityStatus,
	category string,
) error {
	if s == nil || s.toolCapabilityRepo == nil {
		return nil
	}
	providerConfigHash = strings.ToLower(strings.TrimSpace(providerConfigHash))
	modelID = strings.TrimSpace(modelID)
	category = strings.TrimSpace(category)
	if !validToolCapabilityCacheKey(providerConfigHash, modelID) || len(category) > 64 {
		return ErrProviderConfigUnsupported
	}
	ttl := toolCapabilityUnknownTTL
	switch status {
	case ToolCapabilitySupported:
		ttl = toolCapabilitySupportedTTL
	case ToolCapabilityUnsupported:
		ttl = toolCapabilityUnsupportedTTL
	case ToolCapabilityUnknown:
	default:
		return ErrProviderConfigUnsupported
	}
	now := time.Now().UTC()
	return s.toolCapabilityRepo.UpsertToolCapabilityCache(ctx, ToolCapabilityCacheEntry{
		ProviderConfigHash: providerConfigHash,
		ModelID:            modelID,
		Status:             status,
		Category:           category,
		CheckedAt:          now,
		ExpiresAt:          now.Add(ttl),
	})
}

func validToolCapabilityCacheKey(hash string, modelID string) bool {
	if len(hash) != 64 || modelID == "" || len(modelID) > 512 {
		return false
	}
	for _, char := range hash {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func (s *Service) scheduleToolCapabilityWarmup(
	ctx context.Context,
	stored StoredProviderConfig,
) {
	if s == nil || s.toolCapabilityWarmup == nil || !stored.Config.Enabled ||
		!IsModelProviderConfig(stored) {
		return
	}
	modelIDs := s.toolCapabilityWarmupModelIDs(ctx, stored)
	if len(modelIDs) == 0 {
		return
	}
	source := "server-stored"
	if stored.ProviderID == serverDefaultProviderID {
		source = "server-default"
	}
	request := ToolCapabilityWarmupRequest{
		Provider: ProviderRuntimeConfig{
			ID:     stored.ProviderID,
			Source: source,
		},
		ModelIDs: append([]string(nil), modelIDs...),
	}
	actor := auth.UserOrDevelopment(ctx)
	detached := auth.WithUser(context.Background(), actor)
	go s.toolCapabilityWarmup(detached, request)
}

func (s *Service) toolCapabilityWarmupModelIDs(
	ctx context.Context,
	stored StoredProviderConfig,
) []string {
	available := make(map[string]struct{}, len(stored.Config.Models))
	for _, modelID := range stored.Config.Models {
		modelID = strings.TrimSpace(modelID)
		if modelID != "" {
			available[modelID] = struct{}{}
		}
	}
	result := make([]string, 0, 7)
	seen := make(map[string]struct{}, 7)
	add := func(modelID string) {
		modelID = strings.TrimSpace(modelID)
		if len(result) >= 7 || modelID == "" {
			return
		}
		if _, ok := available[modelID]; !ok {
			return
		}
		if _, ok := seen[modelID]; ok {
			return
		}
		seen[modelID] = struct{}{}
		result = append(result, modelID)
	}
	if len(stored.Config.Models) > 0 {
		add(stored.Config.Models[0])
	}
	if s.taskModelRepo == nil {
		return result
	}
	taskModels, found, err := s.taskModelRepo.GetTaskModelSettings(
		ctx,
		stored.UserID,
	)
	if err != nil || !found {
		return result
	}
	for _, modelRef := range []string{
		taskModels.Models.TitleGeneration,
		taskModels.Models.RelatedQuestions,
		taskModels.Models.ContextCompression,
		taskModels.Models.PromptOptimization,
		taskModels.Models.RAGQuery,
		taskModels.Models.Memory,
	} {
		separator := strings.Index(modelRef, ":")
		if separator <= 0 || strings.TrimSpace(modelRef[:separator]) != stored.ProviderID {
			continue
		}
		add(modelRef[separator+1:])
	}
	return result
}
