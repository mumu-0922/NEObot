package memoryworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
)

const (
	errorUnsupportedSchema    = "UNSUPPORTED_SCHEMA"
	errorUnsupportedStage     = "UNSUPPORTED_STAGE"
	errorSourceDrift          = "SOURCE_DRIFT"
	errorProfileDrift         = "PROFILE_DRIFT"
	errorProviderInvalid      = "PROVIDER_INVALID"
	errorProviderFailed       = "PROVIDER_FAILED"
	errorExtractionInvalid    = "EXTRACTION_INVALID"
	errorProposalFailed       = "PROPOSAL_FAILED"
	errorPurgeFailed          = "PURGE_FAILED"
	errorReviewExpiryFailed   = "REVIEW_EXPIRY_FAILED"
	errorEmbeddingSourceDrift = "EMBEDDING_SOURCE_DRIFT"
	errorEmbeddingHydrate     = "EMBEDDING_HYDRATE_FAILED"
	errorEmbeddingProvider    = "EMBEDDING_PROVIDER_FAILED"
	errorEmbeddingInvalid     = "EMBEDDING_VECTOR_INVALID"
	errorEmbeddingComplete    = "EMBEDDING_COMPLETE_FAILED"
	errorEmbeddingRedacted    = "EMBEDDING_SECRET_REDACTED"
)

type Worker struct {
	repository           Repository
	providerResolver     ProviderResolver
	embeddingRepository  EmbeddingRepository
	embeddingProvider    MemoryEmbeddingProvider
	embeddingEnabled     bool
	sceneRepository      SceneRepository
	sceneShadowEnabled   bool
	personaRepository    PersonaRepository
	personaShadowEnabled bool
	workerID             string
	leaseDuration        time.Duration
	providerTimeout      time.Duration
	pollInterval         time.Duration
	baseBackoff          time.Duration
	maximumBackoff       time.Duration
	concurrency          int
	now                  func() time.Time
	logger               *slog.Logger
}

type Option func(*Worker)

func WithWorkerID(value string) Option {
	return func(worker *Worker) { worker.workerID = strings.TrimSpace(value) }
}

func WithLeaseDuration(value time.Duration) Option {
	return func(worker *Worker) { worker.leaseDuration = value }
}

func WithProviderTimeout(value time.Duration) Option {
	return func(worker *Worker) { worker.providerTimeout = value }
}

func WithPollInterval(value time.Duration) Option {
	return func(worker *Worker) { worker.pollInterval = value }
}

func WithBackoff(base time.Duration, maximum time.Duration) Option {
	return func(worker *Worker) {
		worker.baseBackoff = base
		worker.maximumBackoff = maximum
	}
}

func WithConcurrency(value int) Option {
	return func(worker *Worker) { worker.concurrency = value }
}

func WithClock(now func() time.Time) Option {
	return func(worker *Worker) {
		if now != nil {
			worker.now = now
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(worker *Worker) {
		if logger != nil {
			worker.logger = logger
		}
	}
}

func WithEmbeddingProvider(provider MemoryEmbeddingProvider) Option {
	return func(worker *Worker) { worker.embeddingProvider = provider }
}

func WithEmbeddingEnabled(enabled bool) Option {
	return func(worker *Worker) { worker.embeddingEnabled = enabled }
}

func WithSceneShadowEnabled(enabled bool) Option {
	return func(worker *Worker) { worker.sceneShadowEnabled = enabled }
}

func WithPersonaShadowEnabled(enabled bool) Option {
	return func(worker *Worker) { worker.personaShadowEnabled = enabled }
}

func New(
	repository Repository,
	providerResolver ProviderResolver,
	opts ...Option,
) (*Worker, error) {
	workerID, err := chat.NewUUID()
	if err != nil {
		return nil, err
	}
	worker := &Worker{
		repository:       repository,
		providerResolver: providerResolver,
		workerID:         workerID,
		leaseDuration:    2 * time.Minute,
		providerTimeout:  45 * time.Second,
		pollInterval:     time.Second,
		baseBackoff:      5 * time.Second,
		maximumBackoff:   15 * time.Minute,
		concurrency:      2,
		now:              time.Now,
		logger:           slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(worker)
		}
	}
	if worker.repository == nil || worker.providerResolver == nil {
		return nil, errors.New("memory worker repository and provider resolver are required")
	}
	if worker.embeddingEnabled {
		embeddingRepository, ok := worker.repository.(EmbeddingRepository)
		if !ok || embeddingRepository == nil || worker.embeddingProvider == nil {
			return nil, errors.New("memory embedding repository and provider are required")
		}
		worker.embeddingRepository = embeddingRepository
	}
	if sceneRepository, ok := worker.repository.(SceneRepository); ok {
		worker.sceneRepository = sceneRepository
	} else if worker.sceneShadowEnabled {
		return nil, errors.New("L2 Scene repository is required")
	}
	if worker.sceneShadowEnabled && worker.embeddingProvider == nil {
		return nil, errors.New("L2 Scene embedding provider is required")
	}
	if personaRepository, ok := worker.repository.(PersonaRepository); ok {
		worker.personaRepository = personaRepository
	} else if worker.personaShadowEnabled {
		return nil, errors.New("L3 Persona repository is required")
	}
	if worker.personaShadowEnabled && worker.embeddingProvider == nil {
		return nil, errors.New("L3 Persona embedding provider is required")
	}
	if strings.TrimSpace(worker.workerID) == "" || worker.leaseDuration < 5*time.Second ||
		worker.leaseDuration > 15*time.Minute || worker.providerTimeout <= 0 ||
		worker.providerTimeout+5*time.Second >= worker.leaseDuration ||
		worker.pollInterval <= 0 || worker.baseBackoff <= 0 ||
		worker.maximumBackoff < worker.baseBackoff || worker.concurrency < 1 ||
		worker.concurrency > 32 {
		return nil, errors.New("memory worker configuration is invalid")
	}
	return worker, nil
}

func (w *Worker) Run(ctx context.Context, wake <-chan struct{}) error {
	var group sync.WaitGroup
	for lane := 0; lane < w.concurrency; lane++ {
		group.Add(1)
		go func() {
			defer group.Done()
			w.runLane(ctx, wake)
		}()
	}
	<-ctx.Done()
	group.Wait()
	return nil
}

func (w *Worker) runLane(ctx context.Context, wake <-chan struct{}) {
	for ctx.Err() == nil {
		processed, err := w.ProcessOne(ctx)
		if err != nil {
			w.logger.WarnContext(ctx, "memory_worker_iteration_failed")
		}
		if processed {
			continue
		}
		timer := time.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	// Provider-free stale Persona purge has deletion priority and remains active
	// even when all L3 Provider work is disabled.
	if w.personaRepository != nil {
		processed, err := w.processPersonaOne(ctx, false)
		if err != nil || processed {
			return processed, err
		}
	}
	// Provider-free stale Scene purge has deletion priority and remains active
	// even when all L2 Provider work is disabled.
	if w.sceneRepository != nil {
		processed, err := w.processSceneOne(ctx, false)
		if err != nil || processed {
			return processed, err
		}
	}
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.repository.Claim(ctx, w.workerID, leaseToken, w.leaseDuration)
	if err != nil {
		return false, err
	}
	if !found {
		if w.embeddingEnabled {
			processed, embeddingErr := w.processEmbeddingOne(ctx)
			if embeddingErr != nil || processed {
				return processed, embeddingErr
			}
		}
		if w.sceneRepository != nil {
			processed, sceneErr := w.processSceneOne(ctx, w.sceneShadowEnabled)
			if sceneErr != nil || processed {
				return processed, sceneErr
			}
			if w.sceneShadowEnabled {
				processed, sceneEmbeddingErr := w.processSceneEmbeddingOne(ctx)
				if sceneEmbeddingErr != nil || processed {
					return processed, sceneEmbeddingErr
				}
			}
		}
		if w.personaRepository != nil {
			processed, personaErr := w.processPersonaOne(ctx, w.personaShadowEnabled)
			if personaErr != nil || processed {
				return processed, personaErr
			}
			if w.personaShadowEnabled {
				return w.processPersonaEmbeddingOne(ctx)
			}
		}
		return false, nil
	}
	errorCode, terminal, processErr := w.process(ctx, job)
	if processErr != nil {
		availableAt := w.now().UTC().Add(w.retryDelay(job.AttemptCount))
		status, retryErr := w.repository.Retry(
			ctx,
			job,
			errorCode,
			availableAt,
			terminal,
		)
		w.logger.WarnContext(
			ctx,
			"memory_job_failed",
			slog.String("job_id", job.JobID),
			slog.String("event_id", job.EventID),
			slog.String("error_code", errorCode),
			slog.String("status", status),
		)
		return true, retryErr
	}
	if err := w.repository.Complete(ctx, job); err != nil {
		return true, err
	}
	w.logger.InfoContext(
		ctx,
		"memory_job_completed",
		slog.String("job_id", job.JobID),
		slog.String("event_id", job.EventID),
	)
	return true, nil
}

func (w *Worker) process(ctx context.Context, job Job) (string, bool, error) {
	if job.EventSchemaMajor < CurrentEventSchemaMajor-1 ||
		job.EventSchemaMajor > CurrentEventSchemaMajor {
		return errorUnsupportedSchema, true, errors.New("unsupported memory event schema")
	}
	switch job.Stage {
	case "purge":
		if err := w.repository.Purge(ctx, job); err != nil {
			return errorPurgeFailed, terminalPurgeError(err), err
		}
		return "", false, nil
	case "review_expire":
		if _, err := w.repository.ExpireReviews(ctx, job); err != nil {
			return errorReviewExpiryFailed, terminalReviewExpiryError(err), err
		}
		return "", false, nil
	case "extract":
		// Continue through the Provider-backed extraction path below.
	default:
		return errorUnsupportedStage, true, errors.New("unsupported memory job stage")
	}
	capture, err := w.repository.Hydrate(ctx, job)
	if err != nil {
		return classifyHydrationError(err), terminalHydrationError(err), err
	}
	if capture.UserID != job.UserID || capture.ProviderRecordID != job.ProviderRecordID ||
		capture.ProviderID != job.ProviderID || capture.ModelID != job.ModelID ||
		capture.ProcessingProfile != job.ProcessingProfile {
		return errorProfileDrift, true, ErrProviderProfileInvalid
	}
	if capture.ProposalCommitted {
		return "", false, nil
	}
	provider, err := w.providerResolver.Resolve(ctx, capture)
	if err != nil {
		return errorProviderInvalid, true, err
	}
	providerCtx, cancel := context.WithTimeout(ctx, w.providerTimeout)
	defer cancel()
	candidates, err := extractCandidates(
		providerCtx,
		provider,
		chat.ModelRef{ProviderID: capture.ProviderID, ModelID: capture.ModelID},
		job,
		capture,
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errorProviderFailed, false, err
		}
		if strings.Contains(err.Error(), "decode memory extraction response") ||
			strings.Contains(err.Error(), "candidate count is invalid") ||
			strings.Contains(err.Error(), "output exceeded limit") {
			return errorExtractionInvalid, false, err
		}
		return errorProviderFailed, false, err
	}
	decisions, err := decideCandidates(
		providerCtx,
		provider,
		chat.ModelRef{ProviderID: capture.ProviderID, ModelID: capture.ModelID},
		job,
		capture,
		candidates,
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errorProviderFailed, false, err
		}
		if strings.Contains(err.Error(), "decision") ||
			strings.Contains(err.Error(), "provider JSON") {
			return errorExtractionInvalid, false, err
		}
		return errorProviderFailed, false, err
	}
	proposals, err := buildCaptureProposals(job, capture, candidates, decisions)
	if err != nil {
		return errorExtractionInvalid, false, err
	}
	expiryJobID, err := chat.NewUUID()
	if err != nil {
		return errorProposalFailed, false, err
	}
	summary, err := w.repository.ProposeCandidates(ctx, job, ProposalBatch{
		ExpiryJobID:          expiryJobID,
		CandidateSchemaMajor: CandidateSchemaMajor,
		ExtractionProfileID: proposalProfile(
			job.ProcessingProfile, extractionPromptVersion,
		),
		DecisionProfileID: proposalProfile(
			job.ProcessingProfile, decisionPromptVersion,
		),
		Candidates: proposals,
	})
	if err != nil {
		return classifyProposalError(err), terminalProposalError(err), err
	}
	w.logger.InfoContext(
		ctx,
		"memory_capture_proposed",
		slog.String("job_id", job.JobID),
		slog.Int("proposal_count", summary.ProposalCount),
		slog.Int("shadow_count", summary.ShadowCount),
		slog.Int("review_count", summary.ReviewCount),
		slog.Int("rejected_count", summary.RejectedCount),
	)
	return "", false, nil
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := w.baseBackoff
	for index := 1; index < attempt && delay < w.maximumBackoff; index++ {
		if delay > w.maximumBackoff/2 {
			return w.maximumBackoff
		}
		delay *= 2
	}
	if delay > w.maximumBackoff {
		return w.maximumBackoff
	}
	return delay
}

func classifyHydrationError(err error) string {
	value := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "SOURCE_TOMBSTONED") ||
		strings.Contains(value, "VISIBILITY_EPOCH_DRIFT"):
		return errorSourceDrift
	case strings.Contains(value, "PROFILE_DRIFT"):
		return errorProfileDrift
	case strings.Contains(value, "PROVIDER_UNAVAILABLE"):
		return errorProviderInvalid
	default:
		return errorProposalFailed
	}
}

func terminalHydrationError(err error) bool {
	value := strings.ToUpper(err.Error())
	return strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "SOURCE_TOMBSTONED") ||
		strings.Contains(value, "VISIBILITY_EPOCH_DRIFT") ||
		strings.Contains(value, "PROFILE_DRIFT") ||
		strings.Contains(value, "PROVIDER_UNAVAILABLE")
}

func classifyProposalError(err error) string {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "SOURCE_TOMBSTONED") ||
		strings.Contains(value, "CANDIDATE_TOMBSTONED") ||
		strings.Contains(value, "VISIBILITY_EPOCH_DRIFT") {
		return errorSourceDrift
	}
	return errorProposalFailed
}

func terminalProposalError(err error) bool {
	return classifyProposalError(err) == errorSourceDrift
}

func terminalPurgeError(err error) bool {
	value := strings.ToUpper(err.Error())
	return strings.Contains(value, "MEMORY_VISIBILITY_EPOCH_DRIFT") ||
		strings.Contains(value, "MEMORY_PURGE_TARGET_DRIFT")
}

func terminalReviewExpiryError(err error) bool {
	return false
}

func (w *Worker) CheckReady(ctx context.Context) (Readiness, error) {
	return w.repository.CheckReady(ctx)
}
