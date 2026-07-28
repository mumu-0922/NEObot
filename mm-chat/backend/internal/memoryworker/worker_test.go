package memoryworker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	testJobID          = "11111111-1111-4111-8111-111111111111"
	testUserID         = "22222222-2222-4222-8222-222222222222"
	testEventID        = "33333333-3333-4333-8333-333333333333"
	testConversationID = "44444444-4444-4444-8444-444444444444"
	testMessageID      = "55555555-5555-4555-8555-555555555555"
	testAssistantID    = "66666666-6666-4666-8666-666666666666"
	testProviderID     = "fixture"
	testProviderRecord = "77777777-7777-4777-8777-777777777777"
)

func TestWorkerProcessesLeasedCaptureAndCompletes(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{output: `{"memories":[` +
		`{"type":"preference","content":"Use concise answers",` +
		`"importance":5,"tags":["style"]}]}`}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.completed != 1 || repository.retried != 0 ||
		len(repository.applied) != 1 {
		t.Fatalf("processed=%t completed=%d retried=%d applied=%#v",
			processed, repository.completed, repository.retried, repository.applied)
	}
	if repository.applied[0].Content != "Use concise answers" ||
		repository.applied[0].SourceConversationID != testConversationID ||
		repository.applied[0].SourceMessageID != testMessageID {
		t.Fatalf("applied = %#v", repository.applied[0])
	}
	if provider.request.Metadata["purpose"] != "durable-memory-extraction" ||
		provider.request.ModelRef.ModelID != "fixture-model" {
		t.Fatalf("provider request = %#v", provider.request)
	}
}

func TestWorkerRetriesProviderFailureWithoutApplying(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{err: errors.New("provider unavailable")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retried != 1 || repository.retryTerminal ||
		repository.retryCode != errorProviderFailed || len(repository.applied) != 0 ||
		repository.completed != 0 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerDeadLettersUnknownSchemaWithoutHydration(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.job.EventSchemaMajor = CurrentEventSchemaMajor + 1
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.hydrated != 0 || !repository.retryTerminal ||
		repository.retryCode != errorUnsupportedSchema {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerDeadLettersSourceDrift(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.hydrateErr = errors.New("MEMORY_CAPTURE_SOURCE_DRIFT")
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || !repository.retryTerminal || repository.retryCode != errorSourceDrift {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerDeadLettersTombstoneRaisedAfterProviderResponse(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.applyErr = errors.New("MEMORY_CAPTURE_CANDIDATE_TOMBSTONED")
	provider := &workerTestProvider{output: `{"memories":[` +
		`{"type":"preference","content":"Use concise answers",` +
		`"importance":5,"tags":["style"]}]}`}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || !repository.retryTerminal ||
		repository.retryCode != errorSourceDrift || repository.completed != 0 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerAcceptsPreviousEventSchemaMajor(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.job.EventSchemaMajor = CurrentEventSchemaMajor - 1
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{output: `{"memories":[]}`})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.completed != 1 {
		t.Fatalf("processed=%t completed=%d error=%v", processed, repository.completed, err)
	}
}

func TestWorkerPurgesWithoutHydratingProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.job.Stage = "purge"
	repository.job.SourceConversationID = ""
	repository.job.SourceMessageID = ""
	repository.job.AssistantMessageID = ""
	repository.job.ProviderID = ""
	repository.job.ProviderRecordID = ""
	repository.job.ModelID = ""
	repository.job.ProcessingProfile = ""
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.purged != 1 || repository.hydrated != 0 ||
		repository.completed != 1 || provider.request.Metadata != nil {
		t.Fatalf("repository=%#v provider_request=%#v", repository, provider.request)
	}
}

func newWorkerTestInstance(
	t *testing.T,
	repository *workerTestRepository,
	provider chat.Provider,
) *Worker {
	t.Helper()
	worker, err := New(
		repository,
		workerTestProviderResolver{provider: provider},
		WithWorkerID("88888888-8888-4888-8888-888888888888"),
		WithLeaseDuration(time.Minute),
		WithProviderTimeout(10*time.Second),
		WithClock(func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

type workerTestProviderResolver struct {
	provider chat.Provider
}

func (r workerTestProviderResolver) Resolve(context.Context, Capture) (chat.Provider, error) {
	if r.provider == nil {
		return nil, errors.New("provider missing")
	}
	return r.provider, nil
}

type workerTestProvider struct {
	output  string
	err     error
	request chat.ProviderRequest
}

func (p *workerTestProvider) StreamChat(
	_ context.Context,
	request chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	p.request = request
	if p.err != nil {
		return nil, p.err
	}
	events := make(chan chat.ProviderEvent, 1)
	events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: p.output}
	close(events)
	return events, nil
}

type workerTestRepository struct {
	job           Job
	capture       Capture
	found         bool
	hydrateErr    error
	applyErr      error
	hydrated      int
	applied       []usermemory.CreateInput
	purged        int
	completed     int
	retried       int
	retryCode     string
	retryTerminal bool
	retryAt       time.Time
}

func newWorkerTestRepository() *workerTestRepository {
	job := Job{
		JobID: testJobID, UserID: testUserID, EventID: testEventID,
		EventSchemaMajor: CurrentEventSchemaMajor, Stage: "extract",
		AttemptCount: 1, MaxAttempts: 8,
		SourceConversationID: testConversationID, SourceMessageID: testMessageID,
		AssistantMessageID: testAssistantID, ProviderID: testProviderID,
		ProviderRecordID: testProviderRecord, ModelID: "fixture-model",
		ProcessingProfile: "fixture-profile",
	}
	return &workerTestRepository{
		job: job, found: true,
		capture: Capture{
			UserID: testUserID, UserMessageContent: "Remember that I prefer concise answers",
			ProviderRecordID: testProviderRecord, ProviderID: testProviderID,
			ModelID: "fixture-model", ProcessingProfile: "fixture-profile",
		},
	}
}

func (r *workerTestRepository) Claim(
	_ context.Context,
	workerID string,
	leaseToken string,
	_ time.Duration,
) (Job, bool, error) {
	job := r.job
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, r.found, nil
}

func (r *workerTestRepository) Hydrate(context.Context, Job) (Capture, error) {
	r.hydrated++
	return r.capture, r.hydrateErr
}

func (r *workerTestRepository) ApplyCandidate(
	_ context.Context,
	_ Job,
	input usermemory.CreateInput,
) (usermemory.Memory, error) {
	r.applied = append(r.applied, input)
	if r.applyErr != nil {
		return usermemory.Memory{}, r.applyErr
	}
	return usermemory.Memory{ID: input.ID, Content: input.Content}, nil
}

func (r *workerTestRepository) Purge(context.Context, Job) error {
	r.purged++
	return nil
}

func (r *workerTestRepository) Complete(context.Context, Job) error {
	r.completed++
	return nil
}

func (r *workerTestRepository) Retry(
	_ context.Context,
	_ Job,
	code string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	r.retried++
	r.retryCode = code
	r.retryTerminal = terminal
	r.retryAt = availableAt
	if terminal {
		return "dead_letter", nil
	}
	return "pending", nil
}

func (r *workerTestRepository) CheckReady(context.Context) (Readiness, error) {
	return Readiness{ConsumerReady: true}, nil
}
