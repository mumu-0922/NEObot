package voicejobs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var (
	ErrVoiceJobsUnavailable          = errors.New("voice jobs are not configured")
	ErrVoiceArtifactStoreUnavailable = errors.New("voice artifact store is not configured")
	ErrVoiceCacheUnavailable         = errors.New("voice synthesis cache is unavailable")
	ErrVoiceSourceMessageNotFound    = errors.New("voice synthesis source message was not found")
	ErrVoiceSourceMessageChanged     = errors.New("voice synthesis source message changed")
)

const (
	voiceCacheIdleTTL       = 72 * time.Hour
	voiceCacheMaxUserBytes  = int64(100 << 20)
	voiceCleanupClaimTTL    = 10 * time.Minute
	defaultCleanupBatchSize = 64
)

type ServiceOption func(*Service)

type Service struct {
	auditRecorder      jobaudit.Recorder
	transcribeExecutor Executor
	synthesisExecutor  Executor
	synthesisResolver  SynthesisExecutorResolver
	artifactStore      ArtifactStore
	artifactDeleter    ArtifactDeleter
	cache              SynthesisCacheRepository
	now                func() time.Time

	flightMu sync.Mutex
	flights  map[string]*synthesisFlight
}

type synthesisFlight struct {
	done     chan struct{}
	response SynthesizeResponse
	err      error
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{now: time.Now, flights: make(map[string]*synthesisFlight)}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithAuditRecorder(recorder jobaudit.Recorder) ServiceOption {
	return func(service *Service) {
		service.auditRecorder = recorder
	}
}

func WithExecutor(executor Executor) ServiceOption {
	return func(service *Service) {
		service.transcribeExecutor = executor
		service.synthesisExecutor = executor
	}
}

func WithSynthesisExecutorResolver(resolver SynthesisExecutorResolver) ServiceOption {
	return func(service *Service) {
		service.synthesisResolver = resolver
	}
}

func WithArtifactStore(store ArtifactStore) ServiceOption {
	return func(service *Service) {
		service.artifactStore = store
	}
}

func WithArtifactDeleter(deleter ArtifactDeleter) ServiceOption {
	return func(service *Service) {
		service.artifactDeleter = deleter
	}
}

func WithSynthesisCache(cache SynthesisCacheRepository) ServiceOption {
	return func(service *Service) {
		service.cache = cache
	}
}

func (s *Service) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	if err := ctx.Err(); err != nil {
		return TranscribeResponse{}, err
	}
	if s == nil || s.transcribeExecutor == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceTranscribe, request, "VOICE_JOBS_UNAVAILABLE"); err != nil {
			return TranscribeResponse{}, err
		}
		return TranscribeResponse{}, ErrVoiceJobsUnavailable
	}
	if err := s.recordAdmitted(ctx, jobaudit.KindVoiceTranscribe, request); err != nil {
		return TranscribeResponse{}, err
	}
	return s.transcribeExecutor.Transcribe(ctx, request)
}

func (s *Service) Synthesize(ctx context.Context, request SynthesizeRequest) (SynthesizeResponse, error) {
	if err := ctx.Err(); err != nil {
		return SynthesizeResponse{}, err
	}
	if s == nil || (s.synthesisExecutor == nil && s.synthesisResolver == nil) {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_JOBS_UNAVAILABLE"); err != nil {
			return SynthesizeResponse{}, err
		}
		return SynthesizeResponse{}, ErrVoiceJobsUnavailable
	}
	if s.artifactStore == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_ARTIFACT_STORE_UNAVAILABLE"); err != nil {
			return SynthesizeResponse{}, err
		}
		return SynthesizeResponse{}, ErrVoiceArtifactStoreUnavailable
	}
	if s.cache != nil && s.artifactDeleter == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_CACHE_UNAVAILABLE"); err != nil {
			return SynthesizeResponse{}, err
		}
		return SynthesizeResponse{}, ErrVoiceCacheUnavailable
	}

	execution, err := s.resolveSynthesisExecution(ctx, request)
	if err != nil {
		if auditErr := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_JOBS_UNAVAILABLE"); auditErr != nil {
			return SynthesizeResponse{}, auditErr
		}
		return SynthesizeResponse{}, ErrVoiceJobsUnavailable
	}
	request.Provider = ProviderDefault
	request.ModelID = execution.ModelID
	request.VoiceID = execution.VoiceID

	if err := s.recordAdmitted(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
		Provider: Provider(execution.ProviderID),
		ModelID:  execution.ModelID,
	}); err != nil {
		return SynthesizeResponse{}, err
	}
	if s.cache == nil {
		return s.generateAndStore(ctx, execution.Executor, request)
	}
	request.MessageID = strings.TrimSpace(request.MessageID)
	if request.MessageID == "" {
		return SynthesizeResponse{}, ErrVoiceSourceMessageNotFound
	}
	flightKey := strings.Join([]string{
		auth.UserOrDevelopment(ctx).ID,
		request.MessageID,
		synthesisTextDigest(strings.TrimSpace(request.Text)),
		execution.ProviderID,
		execution.ModelID,
		execution.VoiceID,
	}, "\x00")
	return s.runSynthesisFlight(ctx, flightKey, func() (SynthesizeResponse, error) {
		return s.synthesizeCached(ctx, execution, request)
	})
}

func (s *Service) resolveSynthesisExecution(
	ctx context.Context,
	request SynthesizeRequest,
) (SynthesisExecution, error) {
	if s == nil {
		return SynthesisExecution{}, ErrVoiceJobsUnavailable
	}
	if s.synthesisResolver != nil {
		execution, err := s.synthesisResolver.ResolveSynthesisExecutor(ctx)
		if err != nil || execution.Executor == nil ||
			strings.TrimSpace(execution.ProviderID) == "" ||
			strings.TrimSpace(execution.ModelID) == "" ||
			strings.TrimSpace(execution.VoiceID) == "" {
			return SynthesisExecution{}, ErrVoiceJobsUnavailable
		}
		return execution, nil
	}
	if s.synthesisExecutor == nil {
		return SynthesisExecution{}, ErrVoiceJobsUnavailable
	}
	return SynthesisExecution{
		Executor:   s.synthesisExecutor,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		VoiceID:    request.VoiceID,
	}, nil
}

func (s *Service) synthesizeCached(
	ctx context.Context,
	execution SynthesisExecution,
	request SynthesizeRequest,
) (SynthesizeResponse, error) {
	source, err := s.cache.ResolveSynthesisSource(ctx, request.MessageID)
	if err != nil {
		return SynthesizeResponse{}, err
	}
	text := strings.TrimSpace(source.Text)
	if text == "" {
		return SynthesizeResponse{}, ErrVoiceSourceMessageNotFound
	}
	if strings.TrimSpace(request.Text) != text {
		return SynthesizeResponse{}, ErrVoiceSourceMessageChanged
	}
	request.Text = text
	key := SynthesisCacheKey{
		MessageID:       source.MessageID,
		TextSHA256:      synthesisTextDigest(text),
		SourceUpdatedAt: source.UpdatedAt.UTC(),
		ProviderID:      execution.ProviderID,
		ModelID:         execution.ModelID,
		VoiceID:         execution.VoiceID,
	}
	now := s.currentTime()
	if cached, ok, err := s.cache.GetCachedSynthesis(ctx, key, now); err != nil {
		return SynthesizeResponse{}, err
	} else if ok {
		return SynthesizeResponse{
			FileID: cached.FileID, Purpose: "audio", ContentType: cached.ContentType,
			Size: cached.Size, Cached: true,
		}, nil
	}

	response, err := s.generateAndStore(ctx, execution.Executor, request)
	if err != nil {
		return SynthesizeResponse{}, err
	}
	cacheID, err := newVoiceUUID()
	if err != nil {
		s.rollbackArtifact(ctx, response.FileID)
		return SynthesizeResponse{}, err
	}
	err = s.cache.CommitCachedSynthesis(ctx, CommitCachedSynthesisInput{
		ID:          cacheID,
		Key:         key,
		FileID:      response.FileID,
		ContentType: response.ContentType,
		Size:        response.Size,
		AccessedAt:  now,
		MaxBytes:    voiceCacheMaxUserBytes,
	})
	if err != nil {
		s.rollbackArtifact(ctx, response.FileID)
		return SynthesizeResponse{}, err
	}
	if cleanupErr := s.CleanupArtifacts(context.WithoutCancel(ctx), defaultCleanupBatchSize); cleanupErr != nil {
		// The durable cleanup queue owns retry. A successful synthesis remains
		// usable even when object reclamation is temporarily unavailable.
		_ = cleanupErr
	}
	return response, nil
}

func (s *Service) generateAndStore(
	ctx context.Context,
	executor Executor,
	request SynthesizeRequest,
) (SynthesizeResponse, error) {
	result, err := executor.Synthesize(ctx, request)
	if err != nil {
		return SynthesizeResponse{}, err
	}
	artifact, err := s.artifactStore.Store(ctx, jobartifacts.StoreInput{
		JobID:       result.JobID,
		Kind:        jobartifacts.KindAudio,
		Filename:    result.Filename,
		ContentType: result.ContentType,
		Size:        result.Size,
		Body:        result.Body,
	})
	if err != nil {
		return SynthesizeResponse{}, err
	}
	return SynthesizeResponse{
		FileID:      artifact.FileID,
		Purpose:     artifact.Purpose,
		ContentType: artifact.ContentType,
		Size:        artifact.Size,
	}, nil
}

func (s *Service) runSynthesisFlight(
	ctx context.Context,
	key string,
	run func() (SynthesizeResponse, error),
) (SynthesizeResponse, error) {
	s.flightMu.Lock()
	if existing, ok := s.flights[key]; ok {
		s.flightMu.Unlock()
		select {
		case <-ctx.Done():
			return SynthesizeResponse{}, ctx.Err()
		case <-existing.done:
			response := existing.response
			if existing.err == nil {
				response.Cached = true
			}
			return response, existing.err
		}
	}
	flight := &synthesisFlight{done: make(chan struct{})}
	s.flights[key] = flight
	s.flightMu.Unlock()

	flight.response, flight.err = run()
	close(flight.done)
	s.flightMu.Lock()
	delete(s.flights, key)
	s.flightMu.Unlock()
	return flight.response, flight.err
}

func (s *Service) rollbackArtifact(ctx context.Context, fileID string) {
	if s == nil || s.artifactDeleter == nil || strings.TrimSpace(fileID) == "" {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := s.artifactDeleter.Delete(cleanupCtx, fileID); err == nil || errors.Is(err, files.ErrFileNotFound) {
		return
	}
	if s.cache != nil {
		_ = s.cache.QueueArtifactCleanup(cleanupCtx, fileID, "rollback")
	}
}

func (s *Service) CleanupArtifacts(ctx context.Context, batchSize int) error {
	if s == nil || s.cache == nil || s.artifactDeleter == nil {
		return nil
	}
	if batchSize <= 0 || batchSize > 256 {
		batchSize = defaultCleanupBatchSize
	}
	now := s.currentTime()
	if err := s.cache.PrepareArtifactCleanup(
		ctx,
		now.Add(-voiceCacheIdleTTL),
		voiceCacheMaxUserBytes,
		batchSize,
	); err != nil {
		return err
	}
	claimID, err := newVoiceUUID()
	if err != nil {
		return err
	}
	claimed, err := s.cache.ClaimArtifactCleanup(
		ctx,
		claimID,
		now.Add(-voiceCleanupClaimTTL),
		batchSize,
	)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, item := range claimed {
		deleteCtx := auth.WithUser(ctx, auth.User{ID: item.UserID})
		err := s.artifactDeleter.Delete(deleteCtx, item.FileID)
		if err == nil || errors.Is(err, files.ErrFileNotFound) {
			if completeErr := s.cache.CompleteArtifactCleanup(ctx, item.ID, claimID); completeErr != nil {
				cleanupErr = errors.Join(cleanupErr, completeErr)
			}
			continue
		}
		cleanupErr = errors.Join(cleanupErr, err)
		if releaseErr := s.cache.ReleaseArtifactCleanup(ctx, item.ID, claimID); releaseErr != nil {
			cleanupErr = errors.Join(cleanupErr, releaseErr)
		}
	}
	return cleanupErr
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func synthesisTextDigest(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func newVoiceUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate voice cache id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	), nil
}

func (s *Service) recordAdmitted(ctx context.Context, kind jobaudit.JobKind, request TranscribeRequest) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	if recorder == nil {
		return jobaudit.ErrAuditUnavailable
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       kind,
		Status:     jobaudit.StatusAdmitted,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Language:   request.Language,
	})
}

func (s *Service) recordUnavailable(
	ctx context.Context,
	kind jobaudit.JobKind,
	request TranscribeRequest,
	reason string,
) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       kind,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Language:   request.Language,
		Reason:     reason,
	})
}
