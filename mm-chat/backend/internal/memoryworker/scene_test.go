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
	testSceneJobID   = "c1000000-0000-4000-8000-000000000001"
	testSceneID      = "c2000000-0000-4000-8000-000000000001"
	testSceneMemoryA = "c3000000-0000-4000-8000-000000000001"
	testSceneMemoryB = "c3000000-0000-4000-8000-000000000002"
)

func TestSynthesizeScenesAcceptsOnlyStrictHydratedMemberSubset(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	memories, authority := prepareSceneProviderMemories(repository.sceneCapture)
	provider := &workerTestProvider{output: `{"scenes":[{` +
		`"topicKey":"project.aster","content":"Aster uses PostgreSQL 17.",` +
		`"memberMemoryIds":["` + testSceneMemoryA + `","` + testSceneMemoryB + `"]}]}`}
	proposals, err := synthesizeScenes(
		context.Background(),
		provider,
		repository.sceneJob,
		repository.sceneCapture,
		memories,
		authority,
	)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("Scene proposals = %#v/%v", proposals, err)
	}
	proposal := proposals[0]
	if !uuidRE.MatchString(proposal.SceneID) ||
		proposal.TopicKey != "project.aster" ||
		proposal.ContentHash != sceneContentHash(proposal.Content) ||
		proposal.Sensitivity != "normal" || len(proposal.MemberMemoryIDs) != 2 {
		t.Fatalf("Scene proposal = %#v", proposal)
	}
	if provider.request.Metadata["purpose"] != "durable-memory-l2-scene-shadow" ||
		provider.request.Metadata["profile"] != SceneSynthesisProfileID {
		t.Fatalf("Scene provider metadata = %#v", provider.request.Metadata)
	}
}

func TestSynthesizeScenesRejectsStrictOutputViolations(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	memories, authority := prepareSceneProviderMemories(repository.sceneCapture)
	tests := []struct {
		name   string
		output string
	}{
		{name: "unknown outer field", output: `{"scenes":[],"unknown":true}`},
		{name: "unknown Scene field", output: `{"scenes":[{` +
			`"topicKey":"project.aster","content":"Aster",` +
			`"memberMemoryIds":["` + testSceneMemoryA + `","` + testSceneMemoryB + `"],` +
			`"sensitivity":"normal"}]}`},
		{name: "member spoof", output: `{"scenes":[{` +
			`"topicKey":"project.aster","content":"Aster",` +
			`"memberMemoryIds":["` + testSceneMemoryA + `",` +
			`"c3000000-0000-4000-8000-000000000099"]}]}`},
		{name: "duplicate topic", output: `{"scenes":[{` +
			`"topicKey":"project.aster","content":"Aster",` +
			`"memberMemoryIds":["` + testSceneMemoryA + `","` + testSceneMemoryB + `"]},{` +
			`"topicKey":"project.aster","content":"Aster again",` +
			`"memberMemoryIds":["` + testSceneMemoryA + `","` + testSceneMemoryB + `"]}]}`},
		{name: "secret output", output: `{"scenes":[{` +
			`"topicKey":"project.aster","content":"password: fixture-private-value",` +
			`"memberMemoryIds":["` + testSceneMemoryA + `","` + testSceneMemoryB + `"]}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &workerTestProvider{output: test.output}
			proposals, err := synthesizeScenes(
				context.Background(),
				provider,
				repository.sceneJob,
				repository.sceneCapture,
				memories,
				authority,
			)
			if err == nil || len(proposals) != 0 {
				t.Fatalf("invalid output accepted = %#v/%v", proposals, err)
			}
		})
	}
}

func TestWorkerL2SceneDefaultOffStillPurgesWithoutProvider(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.sceneJob.Stage = "purge"
	repository.sceneJob.TargetSceneID = testSceneID
	provider := &workerTestProvider{err: errors.New("must not be called")}
	worker := newSceneWorkerTestInstance(t, repository, provider, false, nil)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.scenePurges != 1 ||
		provider.calls != 0 || len(repository.sceneClaimFlags) != 1 ||
		repository.sceneClaimFlags[0] {
		t.Fatalf("default-off purge = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerL2SceneSecretOnlyScopeMakesZeroProviderCalls(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.sceneCapture.Memories[0].Content = "password: fixture-secret-one"
	repository.sceneCapture.Memories[1].Content = "api_key=fixture-secret-two"
	provider := &workerTestProvider{err: errors.New("must not be called")}
	embeddingProvider := &workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()}
	worker := newSceneWorkerTestInstance(t, repository, provider, true, embeddingProvider)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		len(repository.sceneCompleted) != 0 || repository.sceneRefreshCompletes != 1 ||
		repository.sceneRetries != 0 {
		t.Fatalf("secret-only Scene = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

func TestWorkerRejectsL2SceneReturnedAfterDeadline(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	repository.workerTestRepository.found = false
	provider := &lateSceneProvider{output: `{"scenes":[]}`}
	worker := newSceneWorkerTestInstance(
		t,
		repository,
		provider,
		true,
		&workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()},
	)
	worker.providerTimeout = time.Millisecond

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.sceneRefreshCompletes != 0 ||
		repository.sceneRetryCode != errorSceneProvider || repository.sceneRetryTerminal {
		t.Fatalf("late Scene response = repository:%#v err:%v", repository, err)
	}
}

func TestWorkerRejectsL2SceneAuthorityDriftBeforeProvider(t *testing.T) {
	repository := newSceneWorkerTestRepository()
	repository.workerTestRepository.found = false
	repository.sceneCapture.Generation++
	provider := &workerTestProvider{output: `{"scenes":[]}`}
	worker := newSceneWorkerTestInstance(
		t,
		repository,
		provider,
		true,
		&workerTestEmbeddingProvider{vector: validWorkerEmbeddingVector()},
	)

	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || provider.calls != 0 ||
		repository.sceneRetryCode != errorSceneProfileDrift ||
		!repository.sceneRetryTerminal {
		t.Fatalf("Scene drift = repository:%#v calls:%d err:%v",
			repository, provider.calls, err)
	}
}

type sceneWorkerTestRepository struct {
	*workerTestRepository
	sceneJob                SceneJob
	sceneCapture            SceneCapture
	sceneFound              bool
	sceneClaimFlags         []bool
	sceneRefreshCompletes   int
	sceneCompleted          []SceneProposal
	scenePurges             int
	sceneRetries            int
	sceneRetryCode          string
	sceneRetryTerminal      bool
	sceneEmbeddingFound     bool
	sceneEmbeddingJob       SceneEmbeddingJob
	sceneEmbeddingCapture   SceneEmbeddingCapture
	sceneEmbeddingCompletes int
	sceneEmbeddingRetries   int
}

func newSceneWorkerTestRepository() *sceneWorkerTestRepository {
	base := newWorkerTestRepository()
	providerUpdatedAt := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	job := SceneJob{
		JobID: testSceneJobID, Stage: "refresh", UserID: testUserID,
		ScopeType: "project", ProjectID: testConversationID,
		ScopeGeneration: 1, VisibilityEpoch: 1, Generation: 2,
		ProfileID: "memory_l2_scene_v1", SourceWatermark: strings.Repeat("a", 64),
		AttemptCount: 1, MaxAttempts: 8, ProviderRecordID: testProviderRecord,
		ProviderConfigUpdatedAt: providerUpdatedAt, ModelID: "fixture-model",
	}
	capture := SceneCapture{
		UserID: job.UserID, ScopeType: job.ScopeType, ProjectID: job.ProjectID,
		ScopeGeneration: job.ScopeGeneration, VisibilityEpoch: job.VisibilityEpoch,
		Generation: job.Generation, ProfileID: job.ProfileID,
		SourceWatermark: job.SourceWatermark, ProviderRecordID: job.ProviderRecordID,
		ProviderID: testProviderID, ProviderLabel: "Fixture",
		EncryptedSecretRef: "fixture-secret", ProviderConfig: []byte(`{"enabled":true}`),
		ProviderConfigUpdatedAt: job.ProviderConfigUpdatedAt, ModelID: job.ModelID,
		Memories: []SceneMemory{
			{ID: testSceneMemoryA, Revision: 1, Type: "project",
				Content: "Project codename is Aster", ContentHash: strings.Repeat("b", 64),
				Sensitivity: "normal", Importance: 5},
			{ID: testSceneMemoryB, Revision: 2, Type: "project",
				Content: "Project uses PostgreSQL 17", ContentHash: strings.Repeat("c", 64),
				Sensitivity: "normal", Importance: 4},
		},
	}
	return &sceneWorkerTestRepository{
		workerTestRepository: base,
		sceneJob:             job,
		sceneCapture:         capture,
		sceneFound:           true,
	}
}

func (r *sceneWorkerTestRepository) ClaimScene(
	_ context.Context,
	workerID string,
	leaseToken string,
	_ time.Duration,
	refreshEnabled bool,
) (SceneJob, bool, error) {
	r.sceneClaimFlags = append(r.sceneClaimFlags, refreshEnabled)
	if !r.sceneFound || r.sceneJob.Stage == "refresh" && !refreshEnabled {
		return SceneJob{}, false, nil
	}
	job := r.sceneJob
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *sceneWorkerTestRepository) HydrateSceneRefresh(
	context.Context,
	SceneJob,
) (SceneCapture, error) {
	return r.sceneCapture, nil
}

func (r *sceneWorkerTestRepository) CompleteSceneRefresh(
	_ context.Context,
	_ SceneJob,
	proposals []SceneProposal,
) error {
	r.sceneRefreshCompletes++
	r.sceneCompleted = append([]SceneProposal(nil), proposals...)
	return nil
}

func (r *sceneWorkerTestRepository) CompleteScenePurge(context.Context, SceneJob) error {
	r.scenePurges++
	return nil
}

func (r *sceneWorkerTestRepository) RetryScene(
	_ context.Context,
	_ SceneJob,
	code string,
	_ time.Time,
	terminal bool,
) (string, error) {
	r.sceneRetries++
	r.sceneRetryCode = code
	r.sceneRetryTerminal = terminal
	if terminal {
		return "dead_letter", nil
	}
	return "pending", nil
}

func (r *sceneWorkerTestRepository) ClaimSceneEmbedding(
	context.Context,
	string,
	string,
	time.Duration,
) (SceneEmbeddingJob, bool, error) {
	return r.sceneEmbeddingJob, r.sceneEmbeddingFound, nil
}

func (r *sceneWorkerTestRepository) HydrateSceneEmbedding(
	context.Context,
	SceneEmbeddingJob,
) (SceneEmbeddingCapture, error) {
	return r.sceneEmbeddingCapture, nil
}

func (r *sceneWorkerTestRepository) CompleteSceneEmbedding(
	context.Context,
	SceneEmbeddingJob,
	[]float32,
) error {
	r.sceneEmbeddingCompletes++
	return nil
}

func (r *sceneWorkerTestRepository) RetrySceneEmbedding(
	context.Context,
	SceneEmbeddingJob,
	string,
	time.Time,
	bool,
) (string, error) {
	r.sceneEmbeddingRetries++
	return "pending", nil
}

func newSceneWorkerTestInstance(
	t *testing.T,
	repository *sceneWorkerTestRepository,
	provider chat.Provider,
	enabled bool,
	embeddingProvider MemoryEmbeddingProvider,
) *Worker {
	t.Helper()
	options := []Option{
		WithSceneShadowEnabled(enabled),
		WithWorkerID("d1000000-0000-4000-8000-000000000001"),
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

type lateSceneProvider struct {
	output string
}

func (p *lateSceneProvider) StreamChat(
	ctx context.Context,
	_ chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	<-ctx.Done()
	events := make(chan chat.ProviderEvent, 1)
	events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: p.output}
	close(events)
	return events, nil
}
