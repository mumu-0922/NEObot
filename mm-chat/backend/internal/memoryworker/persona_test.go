package memoryworker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
)

const (
	testPersonaJobID   = "d2000000-0000-4000-8000-000000000001"
	testPersonaID      = "d3000000-0000-4000-8000-000000000001"
	testPersonaMemoryA = "d4000000-0000-4000-8000-000000000001"
	testPersonaMemoryB = "d4000000-0000-4000-8000-000000000002"
)

func TestSynthesizePersonaAcceptsOnlyStrictHydratedMemberSubset(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	memories, authority := preparePersonaProviderMemories(repository.personaCapture)
	provider := &workerTestProvider{output: `{"persona":{` +
		`"content":"Prefers concise answers and uses Go services.",` +
		`"memberMemoryIds":["` + testPersonaMemoryA + `","` + testPersonaMemoryB + `"]}}`}
	proposal, err := synthesizePersona(
		context.Background(), provider, repository.personaCapture, memories, authority,
	)
	if err != nil || proposal == nil {
		t.Fatalf("Persona proposal = %#v/%v", proposal, err)
	}
	if proposal.Content != "Prefers concise answers and uses Go services." ||
		len(proposal.MemberMemoryIDs) != 2 || estimatePersonaTokens(proposal.Content) > 300 {
		t.Fatalf("Persona proposal = %#v", proposal)
	}
	if provider.request.Metadata["purpose"] != "durable-memory-l3-persona-shadow" ||
		provider.request.Metadata["profile"] != PersonaSynthesisProfileID {
		t.Fatalf("Persona provider metadata = %#v", provider.request.Metadata)
	}
	for _, forbidden := range []string{"contentHash", "revision", "sensitivity", "userId"} {
		if strings.Contains(provider.request.Prompt, forbidden) {
			t.Fatalf("Persona provider prompt leaked authority field %q: %s",
				forbidden, provider.request.Prompt)
		}
	}
}

func TestSynthesizePersonaRejectsStrictOutputViolations(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	memories, authority := preparePersonaProviderMemories(repository.personaCapture)
	tests := []struct {
		name   string
		output string
	}{
		{name: "unknown outer field", output: `{"persona":{"content":"A","memberMemoryIds":[]},"unknown":true}`},
		{name: "unknown Persona field", output: `{"persona":{` +
			`"content":"A","memberMemoryIds":["` + testPersonaMemoryA + `","` + testPersonaMemoryB + `"],` +
			`"sensitivity":"normal"}}`},
		{name: "missing Persona", output: `{"persona":null}`},
		{name: "member spoof", output: `{"persona":{` +
			`"content":"A","memberMemoryIds":["` + testPersonaMemoryA + `",` +
			`"d4000000-0000-4000-8000-000000000099"]}}`},
		{name: "duplicate member", output: `{"persona":{` +
			`"content":"A","memberMemoryIds":["` + testPersonaMemoryA + `","` + testPersonaMemoryA + `"]}}`},
		{name: "secret output", output: `{"persona":{` +
			`"content":"password: fixture-private-value","memberMemoryIds":["` +
			testPersonaMemoryA + `","` + testPersonaMemoryB + `"]}}`},
		{name: "over token budget", output: `{"persona":{` +
			`"content":"` + strings.Repeat("a", 1200) + `","memberMemoryIds":["` +
			testPersonaMemoryA + `","` + testPersonaMemoryB + `"]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &workerTestProvider{output: test.output}
			proposal, err := synthesizePersona(
				context.Background(), provider, repository.personaCapture, memories, authority,
			)
			if err == nil || proposal != nil {
				t.Fatalf("invalid Persona output accepted = %#v/%v", proposal, err)
			}
		})
	}
}

func TestWorkerL3PersonaDefaultOffStillPurgesWithoutProvider(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.personaJob.Stage = "purge"
	repository.personaJob.TargetPersonaID = testPersonaID
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newPersonaWorkerTestInstance(t, repository, provider, false, nil)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.personaPurges != 1 ||
		provider.calls != 0 || len(repository.personaClaimFlags) != 1 ||
		repository.personaClaimFlags[0] {
		t.Fatalf("default-off Persona purge = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerL3PersonaSecretOnlyInputMakesZeroProviderCalls(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.personaCapture.Memories[0].Content = "password: fixture-secret-one"
	repository.personaCapture.Memories[1].Content = "api_key=fixture-secret-two"
	provider := &workerTestProvider{err: errors.New("must not be called")}
	embeddingProvider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newPersonaWorkerTestInstance(t, repository, provider, true, embeddingProvider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.personaCompleted != nil || repository.personaRefreshCompletes != 1 ||
		repository.personaRetries != 0 {
		t.Fatalf("secret-only Persona = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerRejectsL3PersonaReturnedAfterDeadline(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	repository.workerTestRepository.found = false
	provider := &lateSceneProvider{output: `{"persona":{` +
		`"content":"A","memberMemoryIds":["` + testPersonaMemoryA + `","` + testPersonaMemoryB + `"]}}`}
	worker := newPersonaWorkerTestInstance(
		t,
		repository,
		provider,
		true,
		&workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()},
	)
	worker.providerTimeout = time.Millisecond

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.personaRefreshCompletes != 0 ||
		repository.personaRetryCode != errorPersonaProvider ||
		repository.personaRetryTerminal {
		t.Fatalf("late Persona response = repository:%#v err:%v", repository, err)
	}
}

func TestWorkerRejectsL3PersonaAuthorityDriftBeforeProvider(t *testing.T) {
	repository := newPersonaWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.personaCapture.Generation++
	provider := &workerTestProvider{output: `{"persona":{` +
		`"content":"A","memberMemoryIds":["` + testPersonaMemoryA + `","` + testPersonaMemoryB + `"]}}`}
	worker := newPersonaWorkerTestInstance(
		t,
		repository,
		provider,
		true,
		&workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()},
	)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.personaRetryCode != errorPersonaProfileDrift ||
		!repository.personaRetryTerminal {
		t.Fatalf("Persona drift = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

type personaWorkerTestRepository struct {
	*workerTestRepository
	personaJob                PersonaJob
	personaCapture            PersonaCapture
	personaFound              bool
	personaClaimFlags         []bool
	personaRefreshCompletes   int
	personaCompleted          *PersonaProposal
	personaPurges             int
	personaRetries            int
	personaRetryCode          string
	personaRetryTerminal      bool
	personaEmbeddingFound     bool
	personaEmbeddingJob       PersonaEmbeddingJob
	personaEmbeddingCapture   PersonaEmbeddingCapture
	personaEmbeddingCompletes int
	personaEmbeddingRetries   int
}

func newPersonaWorkerTestRepository() *personaWorkerTestRepository {
	base := newWorkerTestRepository()
	providerUpdatedAt := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	job := PersonaJob{
		JobID: testPersonaJobID, Stage: "refresh", UserID: testUserID,
		VisibilityEpoch: 1, Generation: 2,
		ProfileID: PersonaProfileID, SourceWatermark: strings.Repeat("a", 64),
		AttemptCount: 1, MaxAttempts: 8, ProviderRecordID: testProviderRecord,
		ProviderConfigUpdatedAt: providerUpdatedAt, ModelID: "fixture-model",
	}
	capture := PersonaCapture{
		UserID: job.UserID, VisibilityEpoch: job.VisibilityEpoch,
		Generation: job.Generation, ProfileID: job.ProfileID,
		SourceWatermark: job.SourceWatermark, ProviderRecordID: job.ProviderRecordID,
		ProviderID: testProviderID, ProviderLabel: "Fixture",
		EncryptedSecretRef: "fixture-secret", ProviderConfig: []byte(`{"enabled":true}`),
		ProviderConfigUpdatedAt: job.ProviderConfigUpdatedAt, ModelID: job.ModelID,
		Memories: []PersonaMemory{
			{ID: testPersonaMemoryA, Revision: 1, Type: "preference",
				Content: "Prefers concise answers", ContentHash: strings.Repeat("b", 64),
				Sensitivity: "normal", Importance: 5},
			{ID: testPersonaMemoryB, Revision: 2, Type: "fact",
				Content: "Uses Go for backend services", ContentHash: strings.Repeat("c", 64),
				Sensitivity: "normal", Importance: 4},
		},
	}
	return &personaWorkerTestRepository{
		workerTestRepository: base,
		personaJob:           job,
		personaCapture:       capture,
		personaFound:         true,
	}
}

func (r *personaWorkerTestRepository) ClaimPersona(
	_ context.Context,
	workerID string,
	leaseToken string,
	_ time.Duration,
	refreshEnabled bool,
) (PersonaJob, bool, error) {
	r.personaClaimFlags = append(r.personaClaimFlags, refreshEnabled)
	if !r.personaFound || r.personaJob.Stage == "refresh" && !refreshEnabled {
		return PersonaJob{}, false, nil
	}
	job := r.personaJob
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *personaWorkerTestRepository) HydratePersonaRefresh(
	context.Context,
	PersonaJob,
) (PersonaCapture, error) {
	return r.personaCapture, nil
}

func (r *personaWorkerTestRepository) CompletePersonaRefresh(
	_ context.Context,
	_ PersonaJob,
	proposal *PersonaProposal,
) error {
	r.personaRefreshCompletes++
	if proposal == nil {
		r.personaCompleted = nil
		return nil
	}
	copied := *proposal
	copied.MemberMemoryIDs = append([]string(nil), proposal.MemberMemoryIDs...)
	r.personaCompleted = &copied
	return nil
}

func (r *personaWorkerTestRepository) CompletePersonaPurge(context.Context, PersonaJob) error {
	r.personaPurges++
	return nil
}

func (r *personaWorkerTestRepository) RetryPersona(
	_ context.Context,
	_ PersonaJob,
	code string,
	_ time.Time,
	terminal bool,
) (string, error) {
	r.personaRetries++
	r.personaRetryCode = code
	r.personaRetryTerminal = terminal
	if terminal {
		return "dead_letter", nil
	}
	return "pending", nil
}

func (r *personaWorkerTestRepository) ClaimPersonaEmbedding(
	context.Context,
	string,
	string,
	time.Duration,
) (PersonaEmbeddingJob, bool, error) {
	return r.personaEmbeddingJob, r.personaEmbeddingFound, nil
}

func (r *personaWorkerTestRepository) HydratePersonaEmbedding(
	context.Context,
	PersonaEmbeddingJob,
) (PersonaEmbeddingCapture, error) {
	return r.personaEmbeddingCapture, nil
}

func (r *personaWorkerTestRepository) CompletePersonaEmbedding(
	context.Context,
	PersonaEmbeddingJob,
	[]float32,
) error {
	r.personaEmbeddingCompletes++
	return nil
}

func (r *personaWorkerTestRepository) RetryPersonaEmbedding(
	context.Context,
	PersonaEmbeddingJob,
	string,
	time.Time,
	bool,
) (string, error) {
	r.personaEmbeddingRetries++
	return "pending", nil
}

func newPersonaWorkerTestInstance(
	t *testing.T,
	repository *personaWorkerTestRepository,
	provider chat.Provider,
	enabled bool,
	embeddingProvider MemoryEmbeddingProvider,
) *Worker {
	t.Helper()
	options := []Option{
		WithPersonaShadowEnabled(enabled),
		WithWorkerID("d5000000-0000-4000-8000-000000000001"),
		WithLeaseDuration(time.Minute),
		WithProviderTimeout(10 * time.Second),
		WithClock(func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	}
	if embeddingProvider != nil {
		options = append(options, WithEmbeddingProvider(embeddingProvider))
	}
	worker, err := New(
		repository,
		workerTestProviderResolver{provider: provider},
		options...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}
