package jobaudit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

type JobKind string

type Status string

const (
	KindVoiceTranscribe JobKind = "voice.transcribe"
	KindVoiceSynthesize JobKind = "voice.synthesize"
	KindImageGenerate   JobKind = "image.generate"
	KindCodeExecute     JobKind = "code.execute"

	StatusUnavailable Status = "unavailable"
	StatusAdmitted    Status = "admitted"
)

var ErrAuditUnavailable = errors.New("job audit recorder unavailable")

type Event struct {
	Kind       JobKind `json:"kind"`
	Status     Status  `json:"status"`
	UserID     string  `json:"userId,omitempty"`
	ProviderID string  `json:"providerId,omitempty"`
	ModelID    string  `json:"modelId,omitempty"`
	Language   string  `json:"language,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

type Recorder interface {
	RecordJobEvent(context.Context, Event) error
}

type RecorderFunc func(context.Context, Event) error

func (f RecorderFunc) RecordJobEvent(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

func Record(ctx context.Context, recorder Recorder, event Event) error {
	if recorder == nil {
		return nil
	}
	event = NormalizeEvent(ctx, event)
	if err := recorder.RecordJobEvent(ctx, event); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	return nil
}

func NormalizeEvent(ctx context.Context, event Event) Event {
	if user, ok := auth.UserFromContext(ctx); ok {
		event.UserID = user.ID
	}
	event.ProviderID = strings.TrimSpace(event.ProviderID)
	event.ModelID = strings.TrimSpace(event.ModelID)
	event.Language = strings.ToLower(strings.TrimSpace(event.Language))
	event.Reason = strings.TrimSpace(event.Reason)
	return event
}
