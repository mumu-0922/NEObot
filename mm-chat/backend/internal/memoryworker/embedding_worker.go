package memoryworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
)

func (w *Worker) processEmbeddingOne(ctx context.Context) (bool, error) {
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.embeddingRepository.ClaimEmbedding(
		ctx,
		w.workerID,
		leaseToken,
		w.leaseDuration,
	)
	if err != nil || !found {
		return false, err
	}
	capture, err := w.embeddingRepository.HydrateEmbedding(ctx, job)
	if err != nil {
		errorCode, terminal := classifyEmbeddingHydrationError(err)
		return true, w.retryEmbedding(ctx, job, errorCode, terminal)
	}
	if embeddingCaptureDrifted(capture, job) {
		return true, w.retryEmbedding(ctx, job, errorEmbeddingSourceDrift, true)
	}
	providerCapture, providerContentAvailable := prepareEmbeddingProviderCapture(capture)
	if !providerContentAvailable {
		return true, w.retryEmbedding(ctx, job, errorEmbeddingRedacted, true)
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	vector, err := w.embeddingProvider.EmbedMemory(providerCtx, providerCapture)
	providerTimedOut := errors.Is(providerCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil || providerTimedOut {
		return true, w.retryEmbedding(ctx, job, errorEmbeddingProvider, false)
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return true, w.retryEmbedding(ctx, job, errorEmbeddingInvalid, true)
	}
	if err := w.embeddingRepository.CompleteEmbedding(ctx, job, vector); err != nil {
		terminal := strings.Contains(strings.ToUpper(err.Error()), "SOURCE_DRIFT")
		return true, w.retryEmbedding(ctx, job, errorEmbeddingComplete, terminal)
	}
	w.logger.InfoContext(
		ctx,
		"memory_embedding_completed",
		slog.String("job_id", job.JobID),
		slog.String("memory_id", job.MemoryID),
	)
	return true, nil
}

func embeddingCaptureDrifted(capture EmbeddingCapture, job EmbeddingJob) bool {
	return capture.UserID != job.UserID || capture.MemoryID != job.MemoryID ||
		capture.ContentHash != job.ContentHash ||
		capture.MemoryRevision != job.MemoryRevision ||
		capture.ProjectionGeneration != job.ProjectionGeneration ||
		capture.VisibilityEpoch != job.VisibilityEpoch ||
		capture.ScopeType != job.ScopeType ||
		capture.ProjectID != job.ProjectID ||
		capture.ScopeConversationID != job.ScopeConversationID ||
		capture.ScopeGeneration != job.ScopeGeneration ||
		capture.EmbeddingProfileID != job.EmbeddingProfileID ||
		capture.EmbeddingModelID != job.EmbeddingModelID ||
		capture.EmbeddingDimensions != job.EmbeddingDimensions ||
		capture.ProviderRecordID != job.ProviderRecordID ||
		!capture.ProviderConfigUpdatedAt.Equal(job.ProviderConfigUpdatedAt)
}

func classifyEmbeddingHydrationError(err error) (string, bool) {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "TOMBSTONED") {
		return errorEmbeddingSourceDrift, true
	}
	return errorEmbeddingHydrate, false
}

func (w *Worker) retryEmbedding(
	ctx context.Context,
	job EmbeddingJob,
	errorCode string,
	terminal bool,
) error {
	availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
	status, retryErr := w.embeddingRepository.RetryEmbedding(
		ctx,
		job,
		errorCode,
		availableAt,
		terminal,
	)
	w.logger.WarnContext(
		ctx,
		"memory_embedding_failed",
		slog.String("job_id", job.JobID),
		slog.String("memory_id", job.MemoryID),
		slog.String("error_code", errorCode),
		slog.String("status", status),
	)
	return retryErr
}
