package jobaudit

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestRecordNormalizesAndAttachesUser(t *testing.T) {
	var got Event
	recorder := RecorderFunc(func(_ context.Context, event Event) error {
		got = event
		return nil
	})
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	err := Record(ctx, recorder, Event{
		Kind:       KindCodeExecute,
		Status:     StatusUnavailable,
		ProviderID: " gemini ",
		ModelID:    " code-model ",
		Language:   " Python ",
		Reason:     " CODE_EXECUTION_UNAVAILABLE ",
	})

	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	want := Event{
		Kind:       KindCodeExecute,
		Status:     StatusUnavailable,
		UserID:     "user-1",
		ProviderID: "gemini",
		ModelID:    "code-model",
		Language:   "python",
		Reason:     "CODE_EXECUTION_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
}

func TestRecordWrapsRecorderFailures(t *testing.T) {
	err := Record(context.Background(), RecorderFunc(func(context.Context, Event) error {
		return errors.New("sink down")
	}), Event{Kind: KindImageGenerate, Status: StatusUnavailable})

	if !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("Record() error = %v, want ErrAuditUnavailable", err)
	}
}
