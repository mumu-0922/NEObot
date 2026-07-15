package voicejobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

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
