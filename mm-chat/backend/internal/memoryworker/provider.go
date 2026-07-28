package memoryworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/providerfactory"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

var ErrProviderProfileInvalid = errors.New("memory provider profile is invalid")

type StoredProviderResolver struct {
	service *runtimeconfig.Service
	timeout time.Duration
}

func NewStoredProviderResolver(
	service *runtimeconfig.Service,
	timeout time.Duration,
) *StoredProviderResolver {
	return &StoredProviderResolver{service: service, timeout: timeout}
}

func (r *StoredProviderResolver) Resolve(
	_ context.Context,
	capture Capture,
) (chat.Provider, error) {
	if r == nil || r.service == nil || strings.TrimSpace(capture.UserID) == "" ||
		strings.TrimSpace(capture.ProviderRecordID) == "" ||
		strings.TrimSpace(capture.ProviderID) == "" {
		return nil, ErrProviderProfileInvalid
	}
	var payload runtimeconfig.StoredProviderConfigPayload
	if err := json.Unmarshal(capture.ProviderConfig, &payload); err != nil {
		return nil, ErrProviderProfileInvalid
	}
	resolved, err := r.service.ResolveHydratedStoredProvider(runtimeconfig.StoredProviderConfig{
		ID:                 capture.ProviderRecordID,
		UserID:             capture.UserID,
		ProviderID:         capture.ProviderID,
		Label:              capture.ProviderLabel,
		EncryptedSecretRef: capture.EncryptedSecretRef,
		Config:             payload,
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.ID) != strings.TrimSpace(capture.ProviderID) {
		return nil, ErrProviderProfileInvalid
	}
	return providerfactory.NewChatProvider(providerfactory.ChatConfig{
		ProviderID: resolved.ID,
		Type:       resolved.Type,
		BaseURL:    resolved.BaseURL,
		APIKey:     resolved.APIKey,
		Timeout:    r.timeout,
	})
}
