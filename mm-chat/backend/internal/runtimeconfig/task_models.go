package runtimeconfig

import (
	"context"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const maxTaskModelRefBytes = 512

type TaskModels struct {
	TitleGeneration    string `json:"titleGeneration"`
	RelatedQuestions   string `json:"relatedQuestions"`
	ContextCompression string `json:"contextCompression"`
	PromptOptimization string `json:"promptOptimization"`
	RAGQuery           string `json:"ragQuery"`
	Memory             string `json:"memory"`
}

type TaskModelSettingsPatch struct {
	TitleGeneration    *string `json:"titleGeneration"`
	RelatedQuestions   *string `json:"relatedQuestions"`
	ContextCompression *string `json:"contextCompression"`
	PromptOptimization *string `json:"promptOptimization"`
	RAGQuery           *string `json:"ragQuery"`
	Memory             *string `json:"memory"`
}

type StoredTaskModelSettings struct {
	Models    TaskModels
	UpdatedAt time.Time
}

type ResolvedTaskModel struct {
	Provider ResolvedProvider
	ModelID  string
}

type AdminTaskModelSettingsResponse struct {
	Models     TaskModels `json:"models"`
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

type TaskModelSettingsRepository interface {
	GetTaskModelSettings(
		ctx context.Context,
		userID string,
	) (StoredTaskModelSettings, bool, error)
	UpsertTaskModelSettings(
		ctx context.Context,
		userID string,
		models TaskModels,
	) (StoredTaskModelSettings, error)
}

func (s *Service) AdminTaskModelSettings(
	ctx context.Context,
) (AdminTaskModelSettingsResponse, error) {
	if s.taskModelRepo == nil {
		return AdminTaskModelSettingsResponse{}, ErrDatabaseRequired
	}
	stored, found, err := s.taskModelRepo.GetTaskModelSettings(
		ctx,
		auth.UserOrDevelopment(ctx).ID,
	)
	if err != nil {
		return AdminTaskModelSettingsResponse{}, err
	}
	if !found {
		return AdminTaskModelSettingsResponse{
			Models: TaskModels{}, Configured: false,
		}, nil
	}
	updatedAt := stored.UpdatedAt.UTC()
	return AdminTaskModelSettingsResponse{
		Models: stored.Models, Configured: true, UpdatedAt: &updatedAt,
	}, nil
}

func (s *Service) ResolveMemoryTaskModel(
	ctx context.Context,
) (ResolvedTaskModel, bool, error) {
	if s.taskModelRepo == nil {
		return ResolvedTaskModel{}, false, nil
	}
	stored, found, err := s.taskModelRepo.GetTaskModelSettings(
		ctx, auth.UserOrDevelopment(ctx).ID,
	)
	if err != nil {
		return ResolvedTaskModel{}, false, err
	}
	modelRef := strings.TrimSpace(stored.Models.Memory)
	if !found || modelRef == "" {
		return ResolvedTaskModel{}, false, nil
	}
	if len(modelRef) > maxTaskModelRefBytes {
		return ResolvedTaskModel{}, false, ErrTaskModelSettingsInvalid
	}
	separator := strings.Index(modelRef, ":")
	if separator <= 0 || separator >= len(modelRef)-1 {
		return ResolvedTaskModel{}, false, ErrTaskModelSettingsInvalid
	}
	providerID := strings.TrimSpace(modelRef[:separator])
	modelID := strings.TrimSpace(modelRef[separator+1:])
	if providerID == "" || modelID == "" {
		return ResolvedTaskModel{}, false, ErrTaskModelSettingsInvalid
	}
	var provider ResolvedProvider
	if providerID == serverDefaultProviderID {
		provider, err = s.ResolveServerDefaultProvider(ctx)
	} else {
		provider, err = s.ResolveStoredProvider(ctx, providerID)
	}
	if err != nil {
		return ResolvedTaskModel{}, false, err
	}
	available := false
	for _, candidate := range provider.Models {
		if strings.TrimSpace(candidate) == modelID {
			available = true
			break
		}
	}
	if !available {
		return ResolvedTaskModel{}, false, ErrTaskModelUnavailable
	}
	return ResolvedTaskModel{Provider: provider, ModelID: modelID}, true, nil
}

func (s *Service) UpdateAdminTaskModelSettings(
	ctx context.Context,
	patch TaskModelSettingsPatch,
) (AdminTaskModelSettingsResponse, error) {
	if s.taskModelRepo == nil || s.repo == nil {
		return AdminTaskModelSettingsResponse{}, ErrDatabaseRequired
	}
	if taskModelPatchEmpty(patch) {
		return AdminTaskModelSettingsResponse{}, ErrTaskModelSettingsInvalid
	}
	user := auth.UserOrDevelopment(ctx)
	stored, found, err := s.taskModelRepo.GetTaskModelSettings(ctx, user.ID)
	if err != nil {
		return AdminTaskModelSettingsResponse{}, err
	}
	models := TaskModels{}
	if found {
		models = stored.Models
	}
	if err := s.applyTaskModelPatch(ctx, user.ID, &models, patch); err != nil {
		return AdminTaskModelSettingsResponse{}, err
	}
	stored, err = s.taskModelRepo.UpsertTaskModelSettings(ctx, user.ID, models)
	if err != nil {
		return AdminTaskModelSettingsResponse{}, err
	}
	updatedAt := stored.UpdatedAt.UTC()
	return AdminTaskModelSettingsResponse{
		Models: stored.Models, Configured: true, UpdatedAt: &updatedAt,
	}, nil
}

func (s *Service) publicTaskModels(
	ctx context.Context,
) (map[string]string, *bool) {
	if s.taskModelRepo == nil {
		return map[string]string{}, nil
	}
	stored, found, err := s.taskModelRepo.GetTaskModelSettings(
		ctx,
		auth.UserOrDevelopment(ctx).ID,
	)
	if err != nil {
		return map[string]string{}, nil
	}
	configured := found
	if !found {
		return map[string]string{}, &configured
	}
	return stored.Models.asMap(), &configured
}

func (s *Service) applyTaskModelPatch(
	ctx context.Context,
	userID string,
	models *TaskModels,
	patch TaskModelSettingsPatch,
) error {
	updates := []struct {
		value  *string
		assign func(string)
	}{
		{patch.TitleGeneration, func(value string) { models.TitleGeneration = value }},
		{patch.RelatedQuestions, func(value string) { models.RelatedQuestions = value }},
		{patch.ContextCompression, func(value string) { models.ContextCompression = value }},
		{patch.PromptOptimization, func(value string) { models.PromptOptimization = value }},
		{patch.RAGQuery, func(value string) { models.RAGQuery = value }},
		{patch.Memory, func(value string) { models.Memory = value }},
	}
	for _, update := range updates {
		if update.value == nil {
			continue
		}
		value := strings.TrimSpace(*update.value)
		if value != "" {
			if err := s.validateTaskModelRef(ctx, userID, value); err != nil {
				return err
			}
		}
		update.assign(value)
	}
	return nil
}

func (s *Service) validateTaskModelRef(
	ctx context.Context,
	userID string,
	value string,
) error {
	if len(value) > maxTaskModelRefBytes {
		return ErrTaskModelSettingsInvalid
	}
	separator := strings.Index(value, ":")
	if separator <= 0 || separator >= len(value)-1 {
		return ErrTaskModelSettingsInvalid
	}
	providerID := strings.TrimSpace(value[:separator])
	modelID := strings.TrimSpace(value[separator+1:])
	if providerID == "" || modelID == "" {
		return ErrTaskModelSettingsInvalid
	}
	stored, found, err := s.repo.GetProviderConfig(ctx, userID, providerID)
	if err != nil {
		return err
	}
	if !found || !IsModelProviderConfig(stored) || !stored.Config.Enabled {
		return ErrTaskModelUnavailable
	}
	for _, configuredModel := range stored.Config.Models {
		if configuredModel == modelID {
			return nil
		}
	}
	return ErrTaskModelUnavailable
}

func taskModelPatchEmpty(patch TaskModelSettingsPatch) bool {
	return patch.TitleGeneration == nil &&
		patch.RelatedQuestions == nil &&
		patch.ContextCompression == nil &&
		patch.PromptOptimization == nil &&
		patch.RAGQuery == nil &&
		patch.Memory == nil
}

func (models TaskModels) asMap() map[string]string {
	return map[string]string{
		"titleGeneration":    models.TitleGeneration,
		"relatedQuestions":   models.RelatedQuestions,
		"contextCompression": models.ContextCompression,
		"promptOptimization": models.PromptOptimization,
		"ragQuery":           models.RAGQuery,
		"memory":             models.Memory,
	}
}
