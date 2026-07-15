package imagejobs

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestServiceAuditsUnavailableImageGenerationWithoutPrompt(t *testing.T) {
	var got jobaudit.Event
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
		got = event
		return nil
	})))
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Generate(ctx, GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "paint a private scene",
		Count:    1,
	})

	if !errors.Is(err, ErrImageJobsUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrImageJobsUnavailable", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusUnavailable,
		UserID:     "user-1",
		ProviderID: "openai",
		ModelID:    "gpt-image-1",
		Reason:     "IMAGE_JOBS_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceFailsClosedWhenImageAuditUnavailable(t *testing.T) {
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	})))

	_, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "paint",
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrAuditUnavailable", err)
	}
}
