package memoryworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
)

const (
	errorSceneSourceDrift       = "L2_SCENE_SOURCE_DRIFT"
	errorSceneProfileDrift      = "L2_SCENE_PROFILE_DRIFT"
	errorSceneHydrate           = "L2_SCENE_HYDRATE_FAILED"
	errorSceneProvider          = "L2_SCENE_PROVIDER_FAILED"
	errorSceneOutputInvalid     = "L2_SCENE_OUTPUT_INVALID"
	errorSceneComplete          = "L2_SCENE_COMPLETE_FAILED"
	errorScenePurge             = "L2_SCENE_PURGE_FAILED"
	errorSceneEmbeddingDrift    = "L2_SCENE_EMBEDDING_SOURCE_DRIFT"
	errorSceneEmbeddingHydrate  = "L2_SCENE_EMBEDDING_HYDRATE_FAILED"
	errorSceneEmbeddingProvider = "L2_SCENE_EMBEDDING_PROVIDER_FAILED"
	errorSceneEmbeddingInvalid  = "L2_SCENE_EMBEDDING_VECTOR_INVALID"
	errorSceneEmbeddingComplete = "L2_SCENE_EMBEDDING_COMPLETE_FAILED"
	errorSceneEmbeddingRedacted = "L2_SCENE_EMBEDDING_SECRET_REDACTED"
)

func (w *Worker) processSceneOne(
	ctx context.Context,
	refreshEnabled bool,
) (bool, error) {
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.sceneRepository.ClaimScene(
		ctx,
		w.workerID,
		leaseToken,
		w.leaseDuration,
		refreshEnabled,
	)
	if err != nil || !found {
		return false, err
	}
	if job.Stage == "purge" {
		if err := w.sceneRepository.CompleteScenePurge(ctx, job); err != nil {
			return true, w.retryScene(ctx, job, errorScenePurge, terminalSceneError(err))
		}
		w.logger.InfoContext(
			ctx,
			"memory_l2_scene_purged",
			slog.String("job_id", job.JobID),
			slog.String("scene_id", job.TargetSceneID),
		)
		return true, nil
	}
	if job.Stage != "refresh" || !refreshEnabled {
		return true, w.retryScene(ctx, job, errorUnsupportedStage, true)
	}
	capture, err := w.sceneRepository.HydrateSceneRefresh(ctx, job)
	if err != nil {
		code, terminal := classifySceneHydrationError(err)
		return true, w.retryScene(ctx, job, code, terminal)
	}
	if sceneCaptureDrifted(capture, job) {
		return true, w.retryScene(ctx, job, errorSceneProfileDrift, true)
	}
	memories, authority := prepareSceneProviderMemories(capture)
	if len(memories) < sceneMemberMinimum {
		if err := w.sceneRepository.CompleteSceneRefresh(
			ctx,
			job,
			[]SceneProposal{},
		); err != nil {
			return true, w.retryScene(ctx, job, errorSceneComplete, terminalSceneError(err))
		}
		return true, nil
	}
	provider, err := w.providerResolver.Resolve(ctx, Capture{
		UserID: capture.UserID, ProviderRecordID: capture.ProviderRecordID,
		ProviderID: capture.ProviderID, ProviderLabel: capture.ProviderLabel,
		EncryptedSecretRef: capture.EncryptedSecretRef,
		ProviderConfig:     capture.ProviderConfig, ModelID: capture.ModelID,
		ProcessingProfile: SceneSynthesisProfileID,
	})
	if err != nil {
		return true, w.retryScene(ctx, job, errorSceneProvider, true)
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	proposals, err := synthesizeScenes(
		providerCtx,
		provider,
		job,
		capture,
		memories,
		authority,
	)
	providerTimedOut := errors.Is(providerCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil || providerTimedOut {
		code := errorSceneProvider
		if isSceneOutputError(err) {
			code = errorSceneOutputInvalid
		}
		return true, w.retryScene(ctx, job, code, false)
	}
	if err := w.sceneRepository.CompleteSceneRefresh(ctx, job, proposals); err != nil {
		return true, w.retryScene(ctx, job, errorSceneComplete, terminalSceneError(err))
	}
	w.logger.InfoContext(
		ctx,
		"memory_l2_scene_refreshed",
		slog.String("job_id", job.JobID),
		slog.Int("scene_count", len(proposals)),
	)
	return true, nil
}

func (w *Worker) processSceneEmbeddingOne(ctx context.Context) (bool, error) {
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.sceneRepository.ClaimSceneEmbedding(
		ctx,
		w.workerID,
		leaseToken,
		w.leaseDuration,
	)
	if err != nil || !found {
		return false, err
	}
	capture, err := w.sceneRepository.HydrateSceneEmbedding(ctx, job)
	if err != nil {
		code, terminal := classifySceneEmbeddingHydrationError(err)
		return true, w.retrySceneEmbedding(ctx, job, code, terminal)
	}
	if sceneEmbeddingCaptureDrifted(capture, job) {
		return true, w.retrySceneEmbedding(ctx, job, errorSceneEmbeddingDrift, true)
	}
	embeddingCapture := EmbeddingCapture{
		UserID: capture.UserID, MemoryID: capture.SceneID,
		Content: capture.Content, ContentHash: capture.ContentHash,
		MemoryRevision:       capture.SceneRevision,
		ProjectionGeneration: capture.Generation,
		VisibilityEpoch:      capture.VisibilityEpoch,
		EmbeddingProfileID:   capture.EmbeddingProfileID,
		EmbeddingModelID:     capture.EmbeddingModelID,
		EmbeddingDimensions:  capture.EmbeddingDimensions,
		ProviderRecordID:     capture.ProviderRecordID,
		ProviderID:           capture.ProviderID, ProviderLabel: capture.ProviderLabel,
		EncryptedSecretRef:      capture.EncryptedSecretRef,
		ProviderConfig:          capture.ProviderConfig,
		ProviderConfigUpdatedAt: capture.ProviderConfigUpdatedAt,
	}
	providerCapture, contentAvailable := prepareEmbeddingProviderCapture(embeddingCapture)
	if !contentAvailable {
		return true, w.retrySceneEmbedding(
			ctx,
			job,
			errorSceneEmbeddingRedacted,
			true,
		)
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	vector, err := w.embeddingProvider.EmbedMemory(providerCtx, providerCapture)
	providerTimedOut := errors.Is(providerCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil || providerTimedOut {
		return true, w.retrySceneEmbedding(
			ctx,
			job,
			errorSceneEmbeddingProvider,
			false,
		)
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return true, w.retrySceneEmbedding(
			ctx,
			job,
			errorSceneEmbeddingInvalid,
			true,
		)
	}
	if err := w.sceneRepository.CompleteSceneEmbedding(ctx, job, vector); err != nil {
		return true, w.retrySceneEmbedding(
			ctx,
			job,
			errorSceneEmbeddingComplete,
			terminalSceneError(err),
		)
	}
	w.logger.InfoContext(
		ctx,
		"memory_l2_scene_embedding_completed",
		slog.String("job_id", job.JobID),
		slog.String("scene_id", job.SceneID),
	)
	return true, nil
}

func sceneCaptureDrifted(capture SceneCapture, job SceneJob) bool {
	return capture.UserID != job.UserID || capture.ScopeType != job.ScopeType ||
		capture.ProjectID != job.ProjectID ||
		capture.ScopeGeneration != job.ScopeGeneration ||
		capture.VisibilityEpoch != job.VisibilityEpoch ||
		capture.Generation != job.Generation || capture.ProfileID != job.ProfileID ||
		capture.SourceWatermark != job.SourceWatermark ||
		capture.ProviderRecordID != job.ProviderRecordID ||
		capture.ModelID != job.ModelID ||
		!capture.ProviderConfigUpdatedAt.Equal(job.ProviderConfigUpdatedAt)
}

func sceneEmbeddingCaptureDrifted(
	capture SceneEmbeddingCapture,
	job SceneEmbeddingJob,
) bool {
	return capture.UserID != job.UserID || capture.SceneID != job.SceneID ||
		capture.ContentHash != job.ContentHash ||
		capture.SceneRevision != job.SceneRevision ||
		capture.SourceWatermark != job.SourceWatermark ||
		capture.VisibilityEpoch != job.VisibilityEpoch ||
		capture.Generation != job.Generation ||
		capture.EmbeddingProfileID != job.EmbeddingProfileID ||
		capture.EmbeddingModelID != job.EmbeddingModelID ||
		capture.EmbeddingDimensions != job.EmbeddingDimensions ||
		capture.ProviderRecordID != job.ProviderRecordID ||
		!capture.ProviderConfigUpdatedAt.Equal(job.ProviderConfigUpdatedAt)
}

func classifySceneHydrationError(err error) (string, bool) {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "HYDRATION_REQUIRED") {
		return errorSceneSourceDrift, true
	}
	return errorSceneHydrate, false
}

func classifySceneEmbeddingHydrationError(err error) (string, bool) {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") {
		return errorSceneEmbeddingDrift, true
	}
	return errorSceneEmbeddingHydrate, false
}

func terminalSceneError(err error) bool {
	value := strings.ToUpper(err.Error())
	return strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "LEASE_LOST") ||
		strings.Contains(value, "MEMBER_DRIFT") ||
		strings.Contains(value, "GENERATION_STALE")
}

func isSceneOutputError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "decode l2 scene") ||
		strings.Contains(value, "l2 scene count") ||
		strings.Contains(value, "l2 scene proposal") ||
		strings.Contains(value, "l2 scene topic") ||
		strings.Contains(value, "l2 scene member") ||
		strings.Contains(value, "l2 scene membership") ||
		strings.Contains(value, "secret-like")
}

func (w *Worker) retryScene(
	ctx context.Context,
	job SceneJob,
	errorCode string,
	terminal bool,
) error {
	availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
	status, retryErr := w.sceneRepository.RetryScene(
		ctx,
		job,
		errorCode,
		availableAt,
		terminal,
	)
	w.logger.WarnContext(
		ctx,
		"memory_l2_scene_job_failed",
		slog.String("job_id", job.JobID),
		slog.String("scene_id", job.TargetSceneID),
		slog.String("error_code", errorCode),
		slog.String("status", status),
	)
	return retryErr
}

func (w *Worker) retrySceneEmbedding(
	ctx context.Context,
	job SceneEmbeddingJob,
	errorCode string,
	terminal bool,
) error {
	availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
	status, retryErr := w.sceneRepository.RetrySceneEmbedding(
		ctx,
		job,
		errorCode,
		availableAt,
		terminal,
	)
	w.logger.WarnContext(
		ctx,
		"memory_l2_scene_embedding_failed",
		slog.String("job_id", job.JobID),
		slog.String("scene_id", job.SceneID),
		slog.String("error_code", errorCode),
		slog.String("status", status),
	)
	return retryErr
}
