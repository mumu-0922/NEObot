package voicejobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestServiceAuditsUnavailableVoiceTranscribeWithoutAudioPayload(t *testing.T) {
	var got jobaudit.Event
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
		got = event
		return nil
	})))
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Transcribe(ctx, TranscribeRequest{
		Provider: ProviderModel,
		ModelID:  "audio-model",
		Language: "auto",
	})

	if !errors.Is(err, ErrVoiceJobsUnavailable) {
		t.Fatalf("Transcribe() error = %v, want ErrVoiceJobsUnavailable", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindVoiceTranscribe,
		Status:     jobaudit.StatusUnavailable,
		UserID:     "user-1",
		ProviderID: "model",
		ModelID:    "audio-model",
		Language:   "auto",
		Reason:     "VOICE_JOBS_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceAuditsUnavailableVoiceSynthesizeWithoutText(t *testing.T) {
	var got jobaudit.Event
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
		got = event
		return nil
	})))

	_, err := service.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderDefault,
		ModelID:  "tts-model",
		Text:     "private text",
	})

	if !errors.Is(err, ErrVoiceJobsUnavailable) {
		t.Fatalf("Synthesize() error = %v, want ErrVoiceJobsUnavailable", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindVoiceSynthesize,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: "default",
		ModelID:    "tts-model",
		Reason:     "VOICE_JOBS_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceFailsClosedWhenVoiceAuditUnavailable(t *testing.T) {
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	})))

	_, err := service.Transcribe(context.Background(), TranscribeRequest{Provider: ProviderDefault})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Transcribe() error = %v, want ErrAuditUnavailable", err)
	}
}

func TestServiceTranscribeUsesOptInExecutor(t *testing.T) {
	executor := &fakeVoiceExecutor{
		transcribeResponse: TranscribeResponse{Text: "transcribed"},
	}
	service := NewService(WithExecutor(executor), WithAuditRecorder(noopAuditRecorder()))

	response, err := service.Transcribe(context.Background(), TranscribeRequest{
		Provider:         ProviderModel,
		ModelID:          "audio-model",
		Language:         "en",
		AudioFilename:    "sample.webm",
		AudioContentType: "audio/webm",
		AudioSize:        11,
		Audio:            strings.NewReader("audio-bytes"),
	})

	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if response.Text != "transcribed" {
		t.Fatalf("Text = %q, want transcribed", response.Text)
	}
	if !executor.transcribeCalled {
		t.Fatal("executor was not called")
	}
	if executor.transcribeRequest.AudioFilename != "sample.webm" ||
		executor.transcribeRequest.AudioContentType != "audio/webm" ||
		executor.transcribeRequest.AudioSize != 11 {
		t.Fatalf("transcribe request = %#v", executor.transcribeRequest)
	}
}

func TestServiceAuditsAdmittedVoiceTranscribeWithoutAudioPayload(t *testing.T) {
	var got jobaudit.Event
	executor := &fakeVoiceExecutor{transcribeResponse: TranscribeResponse{Text: "ok"}}
	service := NewService(
		WithExecutor(executor),
		WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			got = event
			return nil
		})),
	)
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Transcribe(ctx, TranscribeRequest{
		Provider:         ProviderModel,
		ModelID:          "audio-model",
		Language:         "EN",
		AudioFilename:    "secret.webm",
		AudioContentType: "audio/webm",
		AudioSize:        11,
		Audio:            strings.NewReader("audio-bytes"),
	})

	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindVoiceTranscribe,
		Status:     jobaudit.StatusAdmitted,
		UserID:     "user-1",
		ProviderID: "model",
		ModelID:    "audio-model",
		Language:   "en",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
	if executor.transcribeBody != "audio-bytes" {
		t.Fatalf("executor body = %q, want audio-bytes", executor.transcribeBody)
	}
}

func TestServiceDoesNotCallExecutorWhenAdmissionAuditUnavailable(t *testing.T) {
	executor := &fakeVoiceExecutor{}
	service := NewService(
		WithExecutor(executor),
		WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
			return errors.New("audit sink down")
		})),
	)

	_, err := service.Transcribe(context.Background(), TranscribeRequest{
		Provider: ProviderModel,
		ModelID:  "audio-model",
		Language: "en",
		Audio:    strings.NewReader("audio-bytes"),
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Transcribe() error = %v, want ErrAuditUnavailable", err)
	}
	if executor.transcribeCalled {
		t.Fatal("executor was called after admission audit failed")
	}
}

func TestServiceDoesNotCallExecutorWithoutAdmissionAuditRecorder(t *testing.T) {
	executor := &fakeVoiceExecutor{}
	service := NewService(WithExecutor(executor))

	_, err := service.Transcribe(context.Background(), TranscribeRequest{
		Provider: ProviderModel,
		ModelID:  "audio-model",
		Audio:    strings.NewReader("audio-bytes"),
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Transcribe() error = %v, want ErrAuditUnavailable", err)
	}
	if executor.transcribeCalled {
		t.Fatal("executor was called without an admission audit recorder")
	}
}

func TestServiceSynthesizeRequiresArtifactStoreBeforeExecutor(t *testing.T) {
	var got jobaudit.Event
	executor := &fakeVoiceExecutor{}
	service := NewService(
		WithExecutor(executor),
		WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			got = event
			return nil
		})),
	)

	_, err := service.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderDefault,
		ModelID:  "tts-model",
		Text:     "private text",
	})

	if !errors.Is(err, ErrVoiceArtifactStoreUnavailable) {
		t.Fatalf("Synthesize() error = %v, want ErrVoiceArtifactStoreUnavailable", err)
	}
	if executor.synthesizeCalled {
		t.Fatal("executor was called before artifact store was configured")
	}
	if got.Reason != "VOICE_ARTIFACT_STORE_UNAVAILABLE" || got.ModelID != "tts-model" {
		t.Fatalf("audit event = %#v", got)
	}
}

func TestServiceSynthesizeStoresExecutorAudioArtifact(t *testing.T) {
	executor := &fakeVoiceExecutor{
		synthesizeResult: SynthesizeResult{
			JobID:       "job-1",
			Filename:    "voice.webm",
			ContentType: "audio/webm",
			Size:        5,
			Body:        strings.NewReader("audio"),
		},
	}
	store := &fakeArtifactStore{artifact: jobartifacts.Artifact{
		FileID:      "file-1",
		Purpose:     "audio",
		ContentType: "audio/webm",
		Size:        5,
	}}
	service := NewService(WithExecutor(executor), WithArtifactStore(store), WithAuditRecorder(noopAuditRecorder()))

	response, err := service.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderDefault,
		JobID:    "request-job",
		VoiceID:  "voice-1",
		ModelID:  "tts-model",
		Text:     "private text",
	})

	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if response != (SynthesizeResponse{FileID: "file-1", Purpose: "audio", ContentType: "audio/webm", Size: 5}) {
		t.Fatalf("response = %#v", response)
	}
	if !executor.synthesizeCalled {
		t.Fatal("executor was not called")
	}
	if executor.synthesizeRequest.Text != "private text" || executor.synthesizeRequest.JobID != "request-job" {
		t.Fatalf("synthesize request = %#v", executor.synthesizeRequest)
	}
	if store.input.Kind != jobartifacts.KindAudio || store.input.JobID != "job-1" ||
		store.input.Filename != "voice.webm" || store.input.ContentType != "audio/webm" ||
		store.input.Size != 5 {
		t.Fatalf("artifact input = %#v", store.input)
	}
	body, err := io.ReadAll(store.input.Body)
	if err != nil {
		t.Fatalf("read artifact body: %v", err)
	}
	if string(body) != "audio" {
		t.Fatalf("artifact body = %q, want audio", string(body))
	}
}

func TestServiceCachedSynthesisSingleflightAndReuse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	executor := &blockingSynthesisExecutor{started: started, release: release}
	store := &countingArtifactStore{artifact: jobartifacts.Artifact{
		FileID: "33333333-3333-4333-8333-333333333333", Purpose: "audio",
		ContentType: "audio/mpeg", Size: 5,
	}}
	cache := &memorySynthesisCache{source: SynthesisSource{
		MessageID: "22222222-2222-4222-8222-222222222222",
		Text:      "hello cache",
		UpdatedAt: time.Now().UTC(),
	}}
	service := NewService(
		WithSynthesisExecutorResolver(staticSynthesisResolver{execution: SynthesisExecution{
			Executor: executor, ProviderID: "siliconflow", ModelID: "cosy", VoiceID: "claire",
		}}),
		WithArtifactStore(store),
		WithArtifactDeleter(&recordingArtifactDeleter{}),
		WithSynthesisCache(cache),
		WithAuditRecorder(noopAuditRecorder()),
	)
	request := SynthesizeRequest{
		Provider:  ProviderDefault,
		MessageID: cache.source.MessageID,
		Text:      cache.source.Text,
	}
	type result struct {
		response SynthesizeResponse
		err      error
	}
	results := make(chan result, 2)
	go func() {
		response, err := service.Synthesize(context.Background(), request)
		results <- result{response: response, err: err}
	}()
	<-started
	go func() {
		response, err := service.Synthesize(context.Background(), request)
		results <- result{response: response, err: err}
	}()
	close(release)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil || first.response.FileID != store.artifact.FileID ||
		second.response.FileID != store.artifact.FileID {
		t.Fatalf("singleflight responses = %#v/%v %#v/%v", first.response, first.err, second.response, second.err)
	}
	if executor.callCount() != 1 || store.callCount() != 1 || cache.commitCount() != 1 {
		t.Fatalf("singleflight calls executor=%d store=%d commit=%d", executor.callCount(), store.callCount(), cache.commitCount())
	}

	reused, err := service.Synthesize(context.Background(), request)
	if err != nil || !reused.Cached || reused.FileID != store.artifact.FileID {
		t.Fatalf("cached response = %#v, %v", reused, err)
	}
	if executor.callCount() != 1 || store.callCount() != 1 {
		t.Fatal("cache hit called paid generation or artifact storage")
	}
}

func TestServiceSingleflightDoesNotReuseAudioAcrossDifferentMessageText(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	executor := &blockingSynthesisExecutor{started: started, release: release}
	store := &countingArtifactStore{artifact: jobartifacts.Artifact{
		FileID: "33333333-3333-4333-8333-333333333333", Purpose: "audio",
		ContentType: "audio/mpeg", Size: 5,
	}}
	cache := &memorySynthesisCache{source: SynthesisSource{
		MessageID: "22222222-2222-4222-8222-222222222222",
		Text:      "current text",
		UpdatedAt: time.Now().UTC(),
	}}
	service := NewService(
		WithSynthesisExecutorResolver(staticSynthesisResolver{execution: SynthesisExecution{
			Executor: executor, ProviderID: "siliconflow", ModelID: "cosy", VoiceID: "claire",
		}}),
		WithArtifactStore(store),
		WithArtifactDeleter(&recordingArtifactDeleter{}),
		WithSynthesisCache(cache),
		WithAuditRecorder(noopAuditRecorder()),
	)
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Synthesize(context.Background(), SynthesizeRequest{
			Provider: ProviderDefault, MessageID: cache.source.MessageID, Text: cache.source.Text,
		})
		firstDone <- err
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Synthesize(context.Background(), SynthesizeRequest{
			Provider: ProviderDefault, MessageID: cache.source.MessageID, Text: "stale text",
		})
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrVoiceSourceMessageChanged) {
			t.Fatalf("different-text synthesis error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("different-text synthesis incorrectly joined the active flight")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("current-text synthesis error = %v", err)
	}
}

func TestServiceCachedSynthesisRejectsStaleMessageTextBeforeProvider(t *testing.T) {
	executor := &fakeVoiceExecutor{}
	cache := &memorySynthesisCache{source: SynthesisSource{
		MessageID: "22222222-2222-4222-8222-222222222222",
		Text:      "current text",
		UpdatedAt: time.Now().UTC(),
	}}
	service := NewService(
		WithSynthesisExecutorResolver(staticSynthesisResolver{execution: SynthesisExecution{
			Executor: executor, ProviderID: "siliconflow", ModelID: "cosy", VoiceID: "claire",
		}}),
		WithArtifactStore(&fakeArtifactStore{}),
		WithArtifactDeleter(&recordingArtifactDeleter{}),
		WithSynthesisCache(cache),
		WithAuditRecorder(noopAuditRecorder()),
	)
	_, err := service.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderDefault, MessageID: cache.source.MessageID, Text: "stale text",
	})
	if !errors.Is(err, ErrVoiceSourceMessageChanged) {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if executor.synthesizeCalled {
		t.Fatal("stale source text reached provider executor")
	}
}

func TestServiceCleanupUsesQueuedOwnerAndCompletesDeletion(t *testing.T) {
	cache := &memorySynthesisCache{claimed: []ClaimedArtifactCleanup{{
		ID: "cleanup-1", UserID: "44444444-4444-4444-8444-444444444444",
		FileID: "33333333-3333-4333-8333-333333333333",
	}}}
	deleter := &recordingArtifactDeleter{}
	service := NewService(WithSynthesisCache(cache), WithArtifactDeleter(deleter))
	if err := service.CleanupArtifacts(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if deleter.userID != cache.claimed[0].UserID || deleter.fileID != cache.claimed[0].FileID {
		t.Fatalf("cleanup delete authority = %q/%q", deleter.userID, deleter.fileID)
	}
	if cache.completed != "cleanup-1" || cache.released != "" {
		t.Fatalf("cleanup state completed=%q released=%q", cache.completed, cache.released)
	}
}

type fakeVoiceExecutor struct {
	transcribeCalled   bool
	synthesizeCalled   bool
	transcribeRequest  TranscribeRequest
	synthesizeRequest  SynthesizeRequest
	transcribeBody     string
	transcribeResponse TranscribeResponse
	synthesizeResult   SynthesizeResult
	err                error
}

func noopAuditRecorder() jobaudit.Recorder {
	return jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return nil
	})
}

func (e *fakeVoiceExecutor) Transcribe(_ context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	e.transcribeCalled = true
	e.transcribeRequest = request
	if request.Audio != nil {
		body, err := io.ReadAll(request.Audio)
		if err != nil {
			return TranscribeResponse{}, err
		}
		e.transcribeBody = string(body)
	}
	if e.err != nil {
		return TranscribeResponse{}, e.err
	}
	return e.transcribeResponse, nil
}

func (e *fakeVoiceExecutor) Synthesize(_ context.Context, request SynthesizeRequest) (SynthesizeResult, error) {
	e.synthesizeCalled = true
	e.synthesizeRequest = request
	if e.err != nil {
		return SynthesizeResult{}, e.err
	}
	return e.synthesizeResult, nil
}

type fakeArtifactStore struct {
	input    jobartifacts.StoreInput
	artifact jobartifacts.Artifact
	err      error
}

func (s *fakeArtifactStore) Store(_ context.Context, input jobartifacts.StoreInput) (jobartifacts.Artifact, error) {
	s.input = input
	if s.err != nil {
		return jobartifacts.Artifact{}, s.err
	}
	return s.artifact, nil
}

type staticSynthesisResolver struct {
	execution SynthesisExecution
	err       error
}

func (r staticSynthesisResolver) ResolveSynthesisExecutor(context.Context) (SynthesisExecution, error) {
	return r.execution, r.err
}

type blockingSynthesisExecutor struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (e *blockingSynthesisExecutor) Transcribe(context.Context, TranscribeRequest) (TranscribeResponse, error) {
	return TranscribeResponse{}, ErrVoiceJobsUnavailable
}

func (e *blockingSynthesisExecutor) Synthesize(_ context.Context, request SynthesizeRequest) (SynthesizeResult, error) {
	e.mu.Lock()
	e.calls++
	first := e.calls == 1
	e.mu.Unlock()
	if first {
		close(e.started)
	}
	<-e.release
	return SynthesizeResult{
		JobID: "voice", Filename: "voice.mp3", ContentType: "audio/mpeg", Size: 5,
		Body: strings.NewReader("audio"),
	}, nil
}

func (e *blockingSynthesisExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

type countingArtifactStore struct {
	mu       sync.Mutex
	calls    int
	artifact jobartifacts.Artifact
}

func (s *countingArtifactStore) Store(context.Context, jobartifacts.StoreInput) (jobartifacts.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.artifact, nil
}

func (s *countingArtifactStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type memorySynthesisCache struct {
	mu        sync.Mutex
	source    SynthesisSource
	cached    CachedSynthesis
	key       SynthesisCacheKey
	commits   int
	claimed   []ClaimedArtifactCleanup
	completed string
	released  string
}

func (c *memorySynthesisCache) ResolveSynthesisSource(context.Context, string) (SynthesisSource, error) {
	return c.source, nil
}

func (c *memorySynthesisCache) GetCachedSynthesis(
	_ context.Context,
	key SynthesisCacheKey,
	_ time.Time,
) (CachedSynthesis, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached.FileID != "" && c.key == key {
		return c.cached, true, nil
	}
	return CachedSynthesis{}, false, nil
}

func (c *memorySynthesisCache) CommitCachedSynthesis(
	_ context.Context,
	input CommitCachedSynthesisInput,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commits++
	c.key = input.Key
	c.cached = CachedSynthesis{FileID: input.FileID, ContentType: input.ContentType, Size: input.Size}
	return nil
}

func (c *memorySynthesisCache) QueueArtifactCleanup(context.Context, string, string) error {
	return nil
}

func (c *memorySynthesisCache) PrepareArtifactCleanup(context.Context, time.Time, int64, int) error {
	return nil
}

func (c *memorySynthesisCache) ClaimArtifactCleanup(context.Context, string, time.Time, int) ([]ClaimedArtifactCleanup, error) {
	return append([]ClaimedArtifactCleanup(nil), c.claimed...), nil
}

func (c *memorySynthesisCache) CompleteArtifactCleanup(_ context.Context, cleanupID string, _ string) error {
	c.completed = cleanupID
	return nil
}

func (c *memorySynthesisCache) ReleaseArtifactCleanup(_ context.Context, cleanupID string, _ string) error {
	c.released = cleanupID
	return nil
}

func (c *memorySynthesisCache) commitCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.commits
}

type recordingArtifactDeleter struct {
	userID string
	fileID string
	err    error
}

func (d *recordingArtifactDeleter) Delete(ctx context.Context, fileID string) error {
	d.userID = auth.UserOrDevelopment(ctx).ID
	d.fileID = fileID
	return d.err
}
