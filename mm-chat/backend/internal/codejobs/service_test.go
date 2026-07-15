package codejobs

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestServiceAuditsUnavailableCodeExecutionWithoutCodePayload(t *testing.T) {
	var got jobaudit.Event
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
		got = event
		return nil
	})))
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Execute(ctx, ExecuteRequest{
		ModelRef: ModelRef{ProviderID: "gemini", ModelID: "gemini-code"},
		Language: "python",
		Code:     "print('secret')",
	})

	if !errors.Is(err, ErrCodeExecutionUnavailable) {
		t.Fatalf("Execute() error = %v, want ErrCodeExecutionUnavailable", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindCodeExecute,
		Status:     jobaudit.StatusUnavailable,
		UserID:     "user-1",
		ProviderID: "gemini",
		ModelID:    "gemini-code",
		Language:   "python",
		Reason:     "CODE_EXECUTION_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceFailsClosedWhenCodeAuditUnavailable(t *testing.T) {
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	})))

	_, err := service.Execute(context.Background(), ExecuteRequest{
		ModelRef: ModelRef{ProviderID: "gemini", ModelID: "gemini-code"},
		Language: "python",
		Code:     "print('hi')",
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Execute() error = %v, want ErrAuditUnavailable", err)
	}
}
