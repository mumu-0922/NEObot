package memoryworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	errorUnsupportedSchema = "UNSUPPORTED_SCHEMA"
	errorUnsupportedStage  = "UNSUPPORTED_STAGE"
	errorSourceDrift       = "SOURCE_DRIFT"
	errorProfileDrift      = "PROFILE_DRIFT"
	errorProviderInvalid   = "PROVIDER_INVALID"
	errorProviderFailed    = "PROVIDER_FAILED"
	errorExtractionInvalid = "EXTRACTION_INVALID"
	errorApplyFailed       = "APPLY_FAILED"
	errorPurgeFailed       = "PURGE_FAILED"
)

type Worker struct {
	repository       Repository
	providerResolver ProviderResolver
	workerID         string
	leaseDuration    time.Duration
	providerTimeout  time.Duration
	pollInterval     time.Duration
	baseBackoff      time.Duration
	maximumBackoff   time.Duration
	concurrency      int
	now              func() time.Time
	logger           *slog.Logger
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
	leaseToken, err := chat.NewUUID()
	if err != nil {
		return false, err
	}
	job, found, err := w.repository.Claim(ctx, w.workerID, leaseToken, w.leaseDuration)
	if err != nil || !found {
		return false, err
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
		capture.UserMessageContent,
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errorProviderFailed, false, err
		}
		if strings.Contains(err.Error(), "response is not JSON") ||
			strings.Contains(err.Error(), "decode memory extraction response") ||
			strings.Contains(err.Error(), "output exceeded limit") {
			return errorExtractionInvalid, false, err
		}
		return errorProviderFailed, false, err
	}
	adapter := &leasedMemoryRepository{repository: w.repository, job: job}
	_, err = usermemory.NewService(adapter).StoreExtracted(ctx, usermemory.ExtractionInput{
		ConversationID: job.SourceConversationID,
		MessageID:      job.SourceMessageID,
		Candidates:     candidates,
	})
	if err != nil {
		return classifyApplyError(err), terminalApplyError(err), err
	}
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
		return errorApplyFailed
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

func classifyApplyError(err error) string {
	value := strings.ToUpper(err.Error())
	if strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "SOURCE_TOMBSTONED") ||
		strings.Contains(value, "CANDIDATE_TOMBSTONED") ||
		strings.Contains(value, "VISIBILITY_EPOCH_DRIFT") {
		return errorSourceDrift
	}
	return errorApplyFailed
}

func terminalApplyError(err error) bool {
	return classifyApplyError(err) == errorSourceDrift
}

func terminalPurgeError(err error) bool {
	value := strings.ToUpper(err.Error())
	return strings.Contains(value, "MEMORY_VISIBILITY_EPOCH_DRIFT") ||
		strings.Contains(value, "MEMORY_PURGE_TARGET_DRIFT")
}

type leasedMemoryRepository struct {
	repository Repository
	job        Job
}

func (r *leasedMemoryRepository) GetSettings(context.Context) (usermemory.Settings, bool, error) {
	return usermemory.Settings{Enabled: true, AutoRecordEnabled: true}, true, nil
}

func (r *leasedMemoryRepository) Create(
	ctx context.Context,
	input usermemory.CreateInput,
) (usermemory.Memory, error) {
	if input.Source != "ai" || input.SourceConversationID != r.job.SourceConversationID ||
		input.SourceMessageID != r.job.SourceMessageID || !input.Enabled {
		return usermemory.Memory{}, errors.New("memory worker candidate source is invalid")
	}
	return r.repository.ApplyCandidate(ctx, r.job, input)
}

func (r *leasedMemoryRepository) UpsertSettings(context.Context, usermemory.Settings) (usermemory.Settings, error) {
	return usermemory.Settings{}, errors.New("memory worker settings mutation is forbidden")
}

func (r *leasedMemoryRepository) List(context.Context) ([]usermemory.Memory, error) {
	return nil, errors.New("memory worker listing is forbidden")
}

func (r *leasedMemoryRepository) Update(context.Context, string, usermemory.UpdateInput) (usermemory.Memory, error) {
	return usermemory.Memory{}, errors.New("memory worker update is forbidden")
}

func (r *leasedMemoryRepository) Delete(context.Context, string) error {
	return errors.New("memory worker delete is forbidden")
}

func (r *leasedMemoryRepository) MarkUsed(context.Context, []string, time.Time) error {
	return errors.New("memory worker mark-used is forbidden")
}

func (w *Worker) CheckReady(ctx context.Context) (Readiness, error) {
	return w.repository.CheckReady(ctx)
}
