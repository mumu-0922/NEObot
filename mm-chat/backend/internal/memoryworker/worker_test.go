package memoryworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/ragproviders"
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

func TestWorkerRunMaintainsAndRetiresHeartbeat(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})
	worker.heartbeatInterval = 5 * time.Millisecond
	worker.heartbeatTTL = 20 * time.Millisecond
	worker.pollInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx, nil) }()
	deadline := time.Now().Add(time.Second)
	for repository.heartbeatCalls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if repository.heartbeatCalls.Load() < 2 || repository.retireCalls.Load() != 1 {
		t.Fatalf("heartbeat=%d retire=%d",
			repository.heartbeatCalls.Load(), repository.retireCalls.Load())
	}
}

func TestWorkerRunStopsAfterHeartbeatFailure(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.heartbeatFailAfter = 1
	repository.heartbeatErr = errors.New("heartbeat unavailable")
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})
	worker.heartbeatInterval = 5 * time.Millisecond
	worker.heartbeatTTL = 20 * time.Millisecond
	worker.pollInterval = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Run(ctx, nil); !errors.Is(err, repository.heartbeatErr) {
		t.Fatalf("Run error = %v", err)
	}
	if repository.retireCalls.Load() != 1 {
		t.Fatalf("retire calls = %d", repository.retireCalls.Load())
	}
}

func TestWorkerProposesAndPromotesLeasedCapture(t *testing.T) {
	repository := newWorkerTestRepository()
	provider := &workerTestProvider{output: validCandidateOutput()}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.completed != 1 || repository.retried != 0 ||
		len(repository.proposed) != 1 || repository.promoted != 1 {
		t.Fatalf("processed=%t completed=%d retried=%d proposed=%#v promoted=%d",
			processed, repository.completed, repository.retried,
			repository.proposed, repository.promoted)
	}
	if repository.proposed[0].Content == nil ||
		*repository.proposed[0].Content != "Use concise answers" ||
		repository.proposed[0].ProposedScopeType != "global" ||
		repository.proposed[0].ProposedAction != "ADD" {
		t.Fatalf("proposal = %#v", repository.proposed[0])
	}
	if provider.request.Metadata["purpose"] != "durable-memory-candidate-extraction" ||
		provider.request.ModelRef.ModelID != "fixture-model" {
		t.Fatalf("provider request = %#v", provider.request)
	}
	if provider.roundRequest.ToolChoice != chat.ProviderToolChoiceRequired ||
		len(provider.roundRequest.Tools) != 1 ||
		provider.roundRequest.Tools[0].Function.Name != memoryExtractionToolName {
		t.Fatalf("tool round request = %#v", provider.roundRequest)
	}
}

func TestWorkerResumesCommittedProposalWithoutProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.ProposalCommitted = true
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.completed != 1 ||
		len(repository.proposed) != 0 || repository.promoted != 1 || provider.calls != 0 {
		t.Fatalf("processed=%t repository=%#v calls=%d error=%v",
			processed, repository, provider.calls, err)
	}
}

func TestWorkerDeadLettersCommittedPromotionAuthorityDrift(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{name: "source", err: errors.New("MEMORY_CAPTURE_SOURCE_DRIFT"), wantCode: errorSourceDrift},
		{name: "profile", err: errors.New("MEMORY_PROFILE_DRIFT"), wantCode: errorProfileDrift},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newWorkerTestRepository()
			repository.capture.ProposalCommitted = true
			repository.promotionErr = test.err
			worker := newWorkerTestInstance(t, repository, &workerTestProvider{})

			processed, err := worker.ProcessOne(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !processed || repository.retryCode != test.wantCode ||
				!repository.retryTerminal || repository.completed != 0 {
				t.Fatalf("repository = %#v", repository)
			}
		})
	}
}

func TestWorkerRetriesTransientCommittedPromotionFailure(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.capture.ProposalCommitted = true
	repository.promotionErr = errors.New("temporary database failure")
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retryCode != errorPromotionFailed ||
		repository.retryTerminal || repository.completed != 0 {
		t.Fatalf("repository = %#v", repository)
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
	if provider.roundRequest.ToolChoice != chat.ProviderToolChoiceRequired ||
		len(provider.roundRequest.Tools) != 1 ||
		provider.roundRequest.Tools[0].Function.Name != memoryDecisionToolName ||
		provider.roundRequest.Metadata["purpose"] != "durable-memory-conflict-decision" {
		t.Fatalf("decision Tool request = %#v", provider.roundRequest)
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

func TestWorkerLogsOnlyBoundedProviderFailureCategory(t *testing.T) {
	repository := newWorkerTestRepository()
	providerErr := fmt.Errorf("sensitive upstream body: %w", context.DeadlineExceeded)
	provider := &workerTestProvider{err: providerErr}
	var logs bytes.Buffer
	worker, err := New(
		repository,
		workerTestProviderResolver{provider: provider},
		WithWorkerID("88888888-8888-4888-8888-888888888888"),
		WithLeaseDuration(time.Minute),
		WithProviderTimeout(10*time.Second),
		WithClock(func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }),
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
	)
	if err != nil {
		t.Fatal(err)
	}

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.retryCode != errorProviderFailed {
		t.Fatalf("processed=%t retry=%q error=%v", processed, repository.retryCode, err)
	}
	output := logs.String()
	if !strings.Contains(output, `"provider_failure_category":"CONTEXT_DEADLINE"`) {
		t.Fatalf("provider failure category log = %q", output)
	}
	if strings.Contains(output, "sensitive upstream body") {
		t.Fatalf("provider failure log exposed raw error: %q", output)
	}
}

func TestWorkerFailsClosedWhenExtractionToolRoundIsUnsupported(t *testing.T) {
	repository := newWorkerTestRepository()
	worker := newWorkerTestInstance(t, repository, providerWithoutToolRound{})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retryCode != errorExtractionInvalid ||
		repository.retryTerminal || repository.promoted != 0 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestWorkerDeadLettersThirdExtractionProtocolFailure(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.job.AttemptCount = 3
	provider := &workerTestProvider{rounds: [][]chat.ProviderEvent{{
		{Type: chat.ProviderEventDelta, Delta: "not a tool call"},
	}}}
	worker := newWorkerTestInstance(t, repository, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retryCode != errorExtractionInvalid ||
		!repository.retryTerminal || repository.promoted != 0 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestExtractionFailureCategoryIsContentFree(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w: decode Tool arguments", errExtractionInvalid), "CANDIDATE_ARGUMENTS_INVALID"},
		{fmt.Errorf("%w: decode decision Tool arguments", errExtractionInvalid), "DECISION_ARGUMENTS_INVALID"},
		{fmt.Errorf("%w: Tool Call count", errExtractionInvalid), "TOOL_CALL_COUNT_INVALID"},
		{fmt.Errorf("%w: proposal validation", errExtractionInvalid), "PROPOSAL_VALIDATION_INVALID"},
		{classifiedExtractionError{category: "PROPOSAL_USER_AUTHORITY_INVALID"},
			"PROPOSAL_USER_AUTHORITY_INVALID"},
	}
	for _, test := range tests {
		if got := extractionFailureCategory(test.err); got != test.want {
			t.Fatalf("category = %q, want %q", got, test.want)
		}
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

func TestWorkerEmbeddingDefaultOffMakesZeroClaimsAndProviderCalls(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerTestInstance(t, repository, &workerTestProvider{})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || processed || repository.embeddingClaims != 0 || provider.calls != 0 {
		t.Fatalf("default-off embedding = processed:%t claims:%d calls:%d err:%v",
			processed, repository.embeddingClaims, provider.calls, err)
	}
}

func TestWorkerEmbedsOnlyAfterCaptureQueueIsIdle(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.embeddingFound = true
	embeddingProvider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(
		t,
		repository,
		&workerTestProvider{output: validCandidateOutput()},
		embeddingProvider,
	)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.completed != 1 ||
		repository.embeddingClaims != 0 || embeddingProvider.calls != 0 {
		t.Fatalf("capture priority = processed:%t completed:%d claims:%d calls:%d err:%v",
			processed, repository.completed, repository.embeddingClaims,
			embeddingProvider.calls, err)
	}
}

func TestWorkerCompletesLeaseFencedEmbedding(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(t, repository, &workerTestProvider{}, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.embeddingHydrates != 1 ||
		repository.embeddingCompletes != 1 || repository.embeddingRetries != 0 ||
		provider.calls != 1 || len(repository.completedEmbedding) !=
		ragproviders.SiliconFlowEmbeddingDimensions {
		t.Fatalf("embedding success = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
	if provider.capture.Content != repository.embeddingCapture.Content ||
		provider.capture.ScopeGeneration != repository.embeddingJob.ScopeGeneration {
		t.Fatalf("provider capture = %#v", provider.capture)
	}
}

func TestWorkerRedactsEmbeddingBodyBeforeProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	repository.embeddingCapture.Content =
		"api_key=fixture-private-value. Keep answers concise."
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(t, repository, &workerTestProvider{}, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 1 ||
		repository.embeddingCompletes != 1 ||
		strings.Contains(provider.capture.Content, "fixture-private-value") ||
		!strings.Contains(provider.capture.Content, "Keep answers concise") {
		t.Fatalf("embedding redaction = repository:%#v provider:%#v err:%v",
			repository, provider, err)
	}
	if !strings.Contains(repository.embeddingCapture.Content, "fixture-private-value") {
		t.Fatalf("test did not preserve raw repository authority: %#v", repository.embeddingCapture)
	}
}

func TestWorkerSecretOnlyEmbeddingMakesZeroProviderCalls(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	repository.embeddingCapture.Content = "password: fixture-secret-value"
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(t, repository, &workerTestProvider{}, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.embeddingCompletes != 0 || repository.embeddingRetries != 1 ||
		repository.embeddingRetryCode != errorEmbeddingRedacted ||
		!repository.embeddingRetryTerminal {
		t.Fatalf("secret embedding egress = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerRejectsEmbeddingAuthorityDriftBeforeProvider(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	repository.embeddingCapture.VisibilityEpoch++
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(t, repository, &workerTestProvider{}, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.embeddingRetryCode != errorEmbeddingSourceDrift ||
		!repository.embeddingRetryTerminal || repository.embeddingCompletes != 0 {
		t.Fatalf("embedding drift = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerRetriesTransientEmbeddingHydrationFailure(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	repository.embeddingHydrateErr = errors.New("temporary database read failure")
	provider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newWorkerEmbeddingTestInstance(t, repository, &workerTestProvider{}, provider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.embeddingRetryCode != errorEmbeddingHydrate ||
		repository.embeddingRetryTerminal {
		t.Fatalf("embedding hydrate retry = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerRetriesProviderAndDeadLettersInvalidEmbedding(t *testing.T) {
	tests := []struct {
		name         string
		provider     *workerTestEmbeddingProvider
		wantCode     string
		wantTerminal bool
	}{
		{
			name: "provider failure", provider: &workerTestEmbeddingProvider{
				err: errors.New("provider unavailable"),
			}, wantCode: errorEmbeddingProvider,
		},
		{
			name: "invalid shape", provider: &workerTestEmbeddingProvider{
				vector: []float32{1},
			}, wantCode: errorEmbeddingInvalid, wantTerminal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newWorkerTestRepository()
			repository.found = false
			repository.embeddingFound = true
			worker := newWorkerEmbeddingTestInstance(
				t,
				repository,
				&workerTestProvider{},
				test.provider,
			)
			processed, err := worker.ProcessOne(context.Background())
			if err != nil || !processed || repository.embeddingCompletes != 0 ||
				repository.embeddingRetryCode != test.wantCode ||
				repository.embeddingRetryTerminal != test.wantTerminal {
				t.Fatalf("embedding failure = repository:%#v err:%v", repository, err)
			}
		})
	}
}

func TestWorkerRejectsEmbeddingReturnedAfterProviderTimeout(t *testing.T) {
	repository := newWorkerTestRepository()
	repository.found = false
	repository.embeddingFound = true
	provider := &workerTestEmbeddingProvider{
		vector:              validWorkerEmbeddingVector(),
		returnAfterDeadline: true,
	}
	worker := newWorkerEmbeddingTestInstance(
		t,
		repository,
		&workerTestProvider{},
		provider,
	)
	worker.providerTimeout = time.Millisecond

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.embeddingCompletes != 0 ||
		repository.embeddingRetryCode != errorEmbeddingProvider ||
		repository.embeddingRetryTerminal {
		t.Fatalf("late embedding = repository:%#v err:%v", repository, err)
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

func newWorkerEmbeddingTestInstance(
	t *testing.T,
	repository *workerTestRepository,
	provider chat.Provider,
	embeddingProvider MemoryEmbeddingProvider,
) *Worker {
	t.Helper()
	worker, err := New(
		repository,
		workerTestProviderResolver{provider: provider},
		WithEmbeddingEnabled(true),
		WithEmbeddingProvider(embeddingProvider),
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
	output       string
	outputs      []string
	rounds       [][]chat.ProviderEvent
	err          error
	request      chat.ProviderRequest
	roundRequest chat.ProviderRoundRequest
	calls        int
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
	output := p.nextOutput()
	events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: output}
	close(events)
	return events, nil
}

func (p *workerTestProvider) StreamToolRound(
	_ context.Context,
	request chat.ProviderRoundRequest,
) (<-chan chat.ProviderEvent, error) {
	p.calls++
	p.request = request.ProviderRequest
	p.roundRequest = request
	if p.err != nil {
		return nil, p.err
	}
	var fixture []chat.ProviderEvent
	if len(p.rounds) >= p.calls {
		fixture = p.rounds[p.calls-1]
	} else {
		toolName := memoryExtractionToolName
		if len(request.Tools) == 1 && request.Tools[0].Function.Name != "" {
			toolName = request.Tools[0].Function.Name
		}
		fixture = []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-memory-candidates", Name: toolName,
				Arguments: p.nextOutput(),
			},
		}}
	}
	events := make(chan chat.ProviderEvent, len(fixture))
	for _, event := range fixture {
		events <- event
	}
	close(events)
	return events, nil
}

func (p *workerTestProvider) nextOutput() string {
	if len(p.outputs) >= p.calls {
		return p.outputs[p.calls-1]
	}
	return p.output
}

type providerWithoutToolRound struct{}

func (providerWithoutToolRound) StreamChat(
	context.Context,
	chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	return nil, errors.New("compatibility stream must not be called")
}

type workerTestRepository struct {
	job                    Job
	capture                Capture
	found                  bool
	hydrateErr             error
	proposalErr            error
	promotionErr           error
	hydrated               int
	proposed               []CaptureProposal
	promoted               int
	purged                 int
	expired                int
	completed              int
	retried                int
	retryCode              string
	retryTerminal          bool
	retryAt                time.Time
	embeddingJob           EmbeddingJob
	embeddingCapture       EmbeddingCapture
	embeddingFound         bool
	embeddingHydrateErr    error
	embeddingCompleteErr   error
	embeddingClaims        int
	embeddingHydrates      int
	embeddingCompletes     int
	embeddingRetries       int
	embeddingRetryCode     string
	embeddingRetryTerminal bool
	embeddingRetryAt       time.Time
	completedEmbedding     []float32
	heartbeatCalls         atomic.Int32
	retireCalls            atomic.Int32
	heartbeatFailAfter     int32
	heartbeatErr           error
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
	embeddingUpdatedAt := observedAt.Add(-time.Hour)
	embeddingJob := EmbeddingJob{
		JobID:  "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		UserID: testUserID, MemoryID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProjectionGeneration: 1, MemoryRevision: 2,
		ContentHash: strings.Repeat("a", 64), VisibilityEpoch: 1,
		ScopeType: "global", ScopeGeneration: 1,
		EmbeddingProfileID:  string(ragproviders.RetrievalProfileSiliconFlow),
		EmbeddingModelID:    ragproviders.SiliconFlowEmbeddingModel,
		EmbeddingDimensions: ragproviders.SiliconFlowEmbeddingDimensions,
		AttemptCount:        1, MaxAttempts: 8,
		ProviderRecordID:        testProviderRecord,
		ProviderConfigUpdatedAt: embeddingUpdatedAt,
	}
	return &workerTestRepository{
		job: job, found: true, embeddingJob: embeddingJob,
		embeddingCapture: EmbeddingCapture{
			UserID: embeddingJob.UserID, MemoryID: embeddingJob.MemoryID,
			Content: "Keep answers concise", ContentHash: embeddingJob.ContentHash,
			MemoryRevision:          embeddingJob.MemoryRevision,
			ProjectionGeneration:    embeddingJob.ProjectionGeneration,
			VisibilityEpoch:         embeddingJob.VisibilityEpoch,
			ScopeType:               embeddingJob.ScopeType,
			ScopeGeneration:         embeddingJob.ScopeGeneration,
			EmbeddingProfileID:      embeddingJob.EmbeddingProfileID,
			EmbeddingModelID:        embeddingJob.EmbeddingModelID,
			EmbeddingDimensions:     embeddingJob.EmbeddingDimensions,
			ProviderRecordID:        embeddingJob.ProviderRecordID,
			ProviderConfigUpdatedAt: embeddingJob.ProviderConfigUpdatedAt,
		},
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

func (r *workerTestRepository) Heartbeat(
	context.Context,
	string,
	time.Duration,
	bool,
) error {
	call := r.heartbeatCalls.Add(1)
	if r.heartbeatErr != nil && call > r.heartbeatFailAfter {
		return r.heartbeatErr
	}
	return nil
}

func (r *workerTestRepository) Retire(context.Context, string) error {
	r.retireCalls.Add(1)
	return nil
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

func (r *workerTestRepository) PromoteCandidates(
	context.Context,
	Job,
) (PromotionSummary, error) {
	if r.promotionErr != nil {
		return PromotionSummary{}, r.promotionErr
	}
	r.promoted++
	return PromotionSummary{PromotedCount: len(r.proposed)}, nil
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

func (r *workerTestRepository) ClaimEmbedding(
	_ context.Context,
	workerID string,
	leaseToken string,
	_ time.Duration,
) (EmbeddingJob, bool, error) {
	r.embeddingClaims++
	job := r.embeddingJob
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, r.embeddingFound, nil
}

func (r *workerTestRepository) HydrateEmbedding(
	_ context.Context,
	_ EmbeddingJob,
) (EmbeddingCapture, error) {
	r.embeddingHydrates++
	return r.embeddingCapture, r.embeddingHydrateErr
}

func (r *workerTestRepository) CompleteEmbedding(
	_ context.Context,
	_ EmbeddingJob,
	vector []float32,
) error {
	r.embeddingCompletes++
	r.completedEmbedding = append([]float32(nil), vector...)
	return r.embeddingCompleteErr
}

func (r *workerTestRepository) RetryEmbedding(
	_ context.Context,
	_ EmbeddingJob,
	code string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	r.embeddingRetries++
	r.embeddingRetryCode = code
	r.embeddingRetryTerminal = terminal
	r.embeddingRetryAt = availableAt
	if terminal {
		return "dead_letter", nil
	}
	return "pending", nil
}

type workerTestEmbeddingProvider struct {
	vector              []float32
	err                 error
	returnAfterDeadline bool
	capture             EmbeddingCapture
	calls               int
}

func (provider *workerTestEmbeddingProvider) EmbedMemory(
	ctx context.Context,
	capture EmbeddingCapture,
) ([]float32, error) {
	provider.calls++
	provider.capture = capture
	if provider.returnAfterDeadline {
		<-ctx.Done()
	}
	return append([]float32(nil), provider.vector...), provider.err
}

func validWorkerEmbeddingVector() []float32 {
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = 1
	return vector
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
