package voicejobs

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
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
