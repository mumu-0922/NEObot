package memoryworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
)

const (
	errorPersonaSourceDrift       = "L3_PERSONA_SOURCE_DRIFT"
	errorPersonaProfileDrift      = "L3_PERSONA_PROFILE_DRIFT"
	errorPersonaHydrate           = "L3_PERSONA_HYDRATE_FAILED"
	errorPersonaProvider          = "L3_PERSONA_PROVIDER_FAILED"
	errorPersonaOutputInvalid     = "L3_PERSONA_OUTPUT_INVALID"
	errorPersonaComplete          = "L3_PERSONA_COMPLETE_FAILED"
	errorPersonaPurge             = "L3_PERSONA_PURGE_FAILED"
	errorPersonaEmbeddingDrift    = "L3_PERSONA_EMBEDDING_SOURCE_DRIFT"
	errorPersonaEmbeddingHydrate  = "L3_PERSONA_EMBEDDING_HYDRATE_FAILED"
	errorPersonaEmbeddingProvider = "L3_PERSONA_EMBEDDING_PROVIDER_FAILED"
	errorPersonaEmbeddingInvalid  = "L3_PERSONA_EMBEDDING_VECTOR_INVALID"
	errorPersonaEmbeddingComplete = "L3_PERSONA_EMBEDDING_COMPLETE_FAILED"
	errorPersonaEmbeddingRedacted = "L3_PERSONA_EMBEDDING_SECRET_REDACTED"
)

func (w *Worker) processPersonaOne(
	ctx context.Context,
	refreshEnabled bool,
) (bool, error) {
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.personaRepository.ClaimPersona(
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
		if err := w.personaRepository.CompletePersonaPurge(ctx, job); err != nil {
			return true, w.retryPersona(ctx, job, errorPersonaPurge, terminalPersonaError(err))
		}
		w.logger.InfoContext(
			ctx,
			"memory_l3_persona_purged",
			slog.String("job_id", job.JobID),
			slog.String("persona_id", job.TargetPersonaID),
		)
		return true, nil
	}
	if job.Stage != "refresh" || !refreshEnabled {
		return true, w.retryPersona(ctx, job, errorUnsupportedStage, true)
	}
	capture, err := w.personaRepository.HydratePersonaRefresh(ctx, job)
	if err != nil {
		code, terminal := classifyPersonaHydrationError(err)
		return true, w.retryPersona(ctx, job, code, terminal)
	}
	if personaCaptureDrifted(capture, job) {
		return true, w.retryPersona(ctx, job, errorPersonaProfileDrift, true)
	}
	memories, authority := preparePersonaProviderMemories(capture)
	if len(memories) < personaMemberMinimum {
		if err := w.personaRepository.CompletePersonaRefresh(ctx, job, nil); err != nil {
			return true, w.retryPersona(ctx, job, errorPersonaComplete, terminalPersonaError(err))
		}
		return true, nil
	}
	provider, err := w.providerResolver.Resolve(ctx, Capture{
		UserID: capture.UserID, ProviderRecordID: capture.ProviderRecordID,
		ProviderID: capture.ProviderID, ProviderLabel: capture.ProviderLabel,
		EncryptedSecretRef: capture.EncryptedSecretRef,
		ProviderConfig:     capture.ProviderConfig, ModelID: capture.ModelID,
		ProcessingProfile: PersonaSynthesisProfileID,
	})
	if err != nil {
		return true, w.retryPersona(ctx, job, errorPersonaProvider, true)
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	proposal, err := synthesizePersona(
		providerCtx,
		provider,
		capture,
		memories,
		authority,
	)
	providerTimedOut := errors.Is(providerCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil || providerTimedOut {
		code := errorPersonaProvider
		if isPersonaOutputError(err) {
			code = errorPersonaOutputInvalid
		}
		return true, w.retryPersona(ctx, job, code, false)
	}
	if err := w.personaRepository.CompletePersonaRefresh(ctx, job, proposal); err != nil {
		return true, w.retryPersona(ctx, job, errorPersonaComplete, terminalPersonaError(err))
	}
	w.logger.InfoContext(
		ctx,
		"memory_l3_persona_refreshed",
		slog.String("job_id", job.JobID),
		slog.Int("member_count", len(proposal.MemberMemoryIDs)),
		slog.Int("token_count", estimatePersonaTokens(proposal.Content)),
	)
	return true, nil
}

func (w *Worker) processPersonaEmbeddingOne(ctx context.Context) (bool, error) {
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.personaRepository.ClaimPersonaEmbedding(
		ctx,
		w.workerID,
		leaseToken,
		w.leaseDuration,
	)
	if err != nil || !found {
		return false, err
	}
	capture, err := w.personaRepository.HydratePersonaEmbedding(ctx, job)
	if err != nil {
		code, terminal := classifyPersonaEmbeddingHydrationError(err)
		return true, w.retryPersonaEmbedding(ctx, job, code, terminal)
	}
	if personaEmbeddingCaptureDrifted(capture, job) {
		return true, w.retryPersonaEmbedding(ctx, job, errorPersonaEmbeddingDrift, true)
	}
	embeddingCapture := EmbeddingCapture{
		UserID: capture.UserID, MemoryID: capture.PersonaID,
		Content: capture.Content, ContentHash: capture.ContentHash,
		MemoryRevision:       capture.PersonaRevision,
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
		return true, w.retryPersonaEmbedding(
			ctx,
			job,
			errorPersonaEmbeddingRedacted,
			true,
		)
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	vector, err := w.embeddingProvider.EmbedMemory(providerCtx, providerCapture)
	providerTimedOut := errors.Is(providerCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil || providerTimedOut {
		return true, w.retryPersonaEmbedding(
			ctx,
			job,
			errorPersonaEmbeddingProvider,
			false,
		)
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return true, w.retryPersonaEmbedding(
			ctx,
			job,
			errorPersonaEmbeddingInvalid,
			true,
		)
	}
	if err := w.personaRepository.CompletePersonaEmbedding(ctx, job, vector); err != nil {
		return true, w.retryPersonaEmbedding(
			ctx,
			job,
			errorPersonaEmbeddingComplete,
			terminalPersonaError(err),
		)
	}
	w.logger.InfoContext(
		ctx,
		"memory_l3_persona_embedding_completed",
		slog.String("job_id", job.JobID),
		slog.String("persona_id", job.PersonaID),
	)
	return true, nil
}

func personaCaptureDrifted(capture PersonaCapture, job PersonaJob) bool {
	return capture.UserID != job.UserID ||
		capture.VisibilityEpoch != job.VisibilityEpoch ||
		capture.Generation != job.Generation || capture.ProfileID != job.ProfileID ||
		capture.SourceWatermark != job.SourceWatermark ||
		capture.ProviderRecordID != job.ProviderRecordID ||
		capture.ModelID != job.ModelID ||
		!capture.ProviderConfigUpdatedAt.Equal(job.ProviderConfigUpdatedAt)
}

func personaEmbeddingCaptureDrifted(
	capture PersonaEmbeddingCapture,
	job PersonaEmbeddingJob,
) bool {
	return capture.UserID != job.UserID || capture.PersonaID != job.PersonaID ||
		capture.ContentHash != job.ContentHash ||
		capture.PersonaRevision != job.PersonaRevision ||
		capture.SourceWatermark != job.SourceWatermark ||
		capture.VisibilityEpoch != job.VisibilityEpoch ||
		capture.Generation != job.Generation ||
		capture.EmbeddingProfileID != job.EmbeddingProfileID ||
		capture.EmbeddingModelID != job.EmbeddingModelID ||
		capture.EmbeddingDimensions != job.EmbeddingDimensions ||
		capture.ProviderRecordID != job.ProviderRecordID ||
		!capture.ProviderConfigUpdatedAt.Equal(job.ProviderConfigUpdatedAt)
}

func classifyPersonaHydrationError(err error) (string, bool) {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "HYDRATION_REQUIRED") {
		return errorPersonaSourceDrift, true
	}
	return errorPersonaHydrate, false
}

func classifyPersonaEmbeddingHydrationError(err error) (string, bool) {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") {
		return errorPersonaEmbeddingDrift, true
	}
	return errorPersonaEmbeddingHydrate, false
}

func terminalPersonaError(err error) bool {
	value := strings.ToUpper(err.Error())
	return strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "LEASE_LOST") ||
		strings.Contains(value, "MEMBER_DRIFT") ||
		strings.Contains(value, "GENERATION_STALE") ||
		strings.Contains(value, "VERSION_CONFLICT")
}

func isPersonaOutputError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "decode l3 persona") ||
		strings.Contains(value, "l3 persona proposal") ||
		strings.Contains(value, "l3 persona member") ||
		strings.Contains(value, "secret-like")
}

func (w *Worker) retryPersona(
	ctx context.Context,
	job PersonaJob,
	errorCode string,
	terminal bool,
) error {
	availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
	status, retryErr := w.personaRepository.RetryPersona(
		ctx,
		job,
		errorCode,
		availableAt,
		terminal,
	)
	w.logger.WarnContext(
		ctx,
		"memory_l3_persona_job_failed",
		slog.String("job_id", job.JobID),
		slog.String("persona_id", job.TargetPersonaID),
		slog.String("error_code", errorCode),
		slog.String("status", status),
	)
	return retryErr
}

func (w *Worker) retryPersonaEmbedding(
	ctx context.Context,
	job PersonaEmbeddingJob,
	errorCode string,
	terminal bool,
) error {
	availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
	status, retryErr := w.personaRepository.RetryPersonaEmbedding(
		ctx,
		job,
		errorCode,
		availableAt,
		terminal,
	)
	w.logger.WarnContext(
		ctx,
		"memory_l3_persona_embedding_failed",
		slog.String("job_id", job.JobID),
		slog.String("persona_id", job.PersonaID),
		slog.String("error_code", errorCode),
		slog.String("status", status),
	)
	return retryErr
}
