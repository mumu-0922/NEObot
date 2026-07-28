package memoryworker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
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

func TestWorkerProposesLeasedCaptureWithoutCanonicalApply(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{output: validCandidateOutput()}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.completed != 1 || repository.retried != 0 ||
		len(repository.proposed) != 1 {
		t.Fatalf("processed=%t completed=%d retried=%d proposed=%#v",
			processed, repository.completed, repository.retried, repository.proposed)
	}
	if repository.proposed[0].Content == nil ||
		*repository.proposed[0].Content != "Use concise answers" ||
		repository.proposed[0].ProposedScopeType != "global" ||
		repository.proposed[0].ProposedAction != "ADD" {
		t.Fatalf("proposal = %#v", repository.proposed[0])
	}
	if provider.request.Metadata["purpose"] != "durable-memory-candidate-shadow" ||
		provider.request.ModelRef.ModelID != "fixture-model" {
		t.Fatalf("provider request = %#v", provider.request)
	}
}

func TestWorkerResumesCommittedProposalWithoutProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.ProposalCommitted = true
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.completed != 1 ||
		len(repository.proposed) != 0 || provider.calls != 0 {
		t.Fatalf("processed=%t repository=%#v calls=%d error=%v",
			processed, repository, provider.calls, err)
	}
}

func TestWorkerUsesBoundedDecisionProposalForCurrentMemory(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.CurrentMemories = []CaptureMemory{{
		ID: "99999999-9999-4999-8999-999999999999", Revision: 3,
		Type: "preference", Content: "Use detailed answers",
		AuthorityKind: "manual", ScopeType: "global", Sensitivity: "normal",
	}}
	provider := &workerTestProvider{outputs: []string{
		validCandidateOutput(),
		`{"decisions":[{"ordinal":1,"action":"SUPERSEDE",` +
			`"targetMemoryIds":["99999999-9999-4999-8999-999999999999"]}]}`,
	}}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 2 || len(repository.proposed) != 1 {
		t.Fatalf("processed=%t calls=%d proposals=%#v error=%v",
			processed, provider.calls, repository.proposed, err)
	}
	proposal := repository.proposed[0]
	if proposal.ProposedAction != "SUPERSEDE" || len(proposal.TargetMemoryIDs) != 1 ||
		proposal.TargetMemoryIDs[0] != repository.capture.CurrentMemories[0].ID {
		t.Fatalf("decision proposal = %#v", proposal)
	}
}

func TestWorkerRejectsDecisionTargetSpoofWithoutProposal(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.CurrentMemories = []CaptureMemory{{
		ID: "99999999-9999-4999-8999-999999999999", Revision: 1,
		Type: "preference", Content: "Use detailed answers",
		AuthorityKind: "manual", ScopeType: "global", Sensitivity: "normal",
	}}
	provider := &workerTestProvider{outputs: []string{
		validCandidateOutput(),
		`{"decisions":[{"ordinal":1,"action":"SUPERSEDE",` +
			`"targetMemoryIds":["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"]}]}`,
	}}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retryCode != errorExtractionInvalid ||
		repository.retryTerminal || len(repository.proposed) != 0 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerRetriesProviderFailureWithoutProposal(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{err: errors.New("provider unavailable")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retried != 1 || repository.retryTerminal ||
		repository.retryCode != errorProviderFailed || len(repository.proposed) != 0 ||
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

func TestWorkerDeadLettersSourceFenceRaisedAfterProviderResponse(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.proposalErr = errors.New("MEMORY_CAPTURE_SOURCE_TOMBSTONED")
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{output: validCandidateOutput()})

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
	clearProviderJobFields(&repository.job)
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.purged != 1 || repository.hydrated != 0 ||
		repository.completed != 1 || provider.calls != 0 {
		t.Fatalf("repository=%#v calls=%d", repository, provider.calls)
	}
}

func TestWorkerExpiresReviewsWithoutHydratingProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.job.Stage = "review_expire"
	clearProviderJobFields(&repository.job)
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.expired != 1 || repository.hydrated != 0 ||
		repository.completed != 1 || provider.calls != 0 {
		t.Fatalf("repository=%#v calls=%d", repository, provider.calls)
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
	outputs []string
	err     error
	request chat.ProviderRequest
	calls   int
}

func (p *workerTestProvider) StreamChat(
	_ context.Context,
	request chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	p.calls++
	p.request = request
	if p.err != nil {
		return nil, p.err
	}
	events := make(chan chat.ProviderEvent, 1)
	output := p.output
	if len(p.outputs) >= p.calls {
		output = p.outputs[p.calls-1]
	}
	events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: output}
	close(events)
	return events, nil
}

type workerTestRepository struct {
	job           Job
	capture       Capture
	found         bool
	hydrateErr    error
	proposalErr   error
	hydrated      int
	proposed      []CaptureProposal
	purged        int
	expired       int
	completed     int
	retried       int
	retryCode     string
	retryTerminal bool
	retryAt       time.Time
}

func newWorkerTestRepository() *workerTestRepository {
	observedAt := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
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
			UserID: testUserID,
			Messages: []CaptureMessage{
				{ID: testMessageID, Role: "user", Content: "Remember that I prefer concise answers", ObservedAt: observedAt},
				{ID: testAssistantID, Role: "assistant", Content: "Understood", ObservedAt: observedAt.Add(time.Second)},
			},
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

func (r *workerTestRepository) ProposeCandidates(
	_ context.Context,
	_ Job,
	batch ProposalBatch,
) (ProposalSummary, error) {
	r.proposed = append(r.proposed, batch.Candidates...)
	if r.proposalErr != nil {
		return ProposalSummary{}, r.proposalErr
	}
	return ProposalSummary{ProposalCount: len(batch.Candidates), ShadowCount: len(batch.Candidates)}, nil
}

func (r *workerTestRepository) Purge(context.Context, Job) error {
	r.purged++
	return nil
}

func (r *workerTestRepository) ExpireReviews(context.Context, Job) (int, error) {
	r.expired++
	return 1, nil
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

func validCandidateOutput() string {
	return `{"memories":[{` +
		`"type":"preference","content":"Use concise answers","importance":5,` +
		`"confidence":0.95,"tags":["style"],"subjectKey":"user",` +
		`"factKey":"response.style","sensitivity":"normal",` +
		`"authorityUserMessageIds":["` + testMessageID + `"],` +
		`"contextMessageIds":[],"confirmationKind":"explicit_user",` +
		`"proposedScopeType":"global","scopeConfidence":0.98,` +
		`"temporalBasis":"none","validFrom":null,"validTo":null,` +
		`"factExpiresAt":null}]}`
}

func clearProviderJobFields(job *Job) {
	job.SourceConversationID = ""
	job.SourceMessageID = ""
	job.AssistantMessageID = ""
	job.ProviderID = ""
	job.ProviderRecordID = ""
	job.ModelID = ""
	job.ProcessingProfile = ""
}
