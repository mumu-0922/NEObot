package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestProcessTraceSanitizesDetailsAndReasoning(t *testing.T) {
	startedAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	trace := newProcessTrace("message-1")
	trace.start(
		ProcessStepKindTool,
		"attacker-controlled-label",
		startedAt,
		map[string]any{
			"query":        "Authorization: Bearer abcdefghijklmnop",
			"redactedArgs": "token=fixture-secret-token",
			"headers":      map[string]any{"Authorization": "Bearer leak"},
			"rawPayload":   "must not persist",
		},
	)
	finishProcessTrace(trace, "failed", startedAt.Add(time.Second), websearch.Result{})

	metadata := withProcessTraceMessageMetadata(
		map[string]any{"runId": "run-1"},
		"Reasoning used sk-1234567890abcdefghijkl and password=hunter2",
		trace,
	)
	encoded := metadataString(t, metadata)
	for _, forbidden := range []string{
		"abcdefghijklmnop",
		"fixture-secret-token",
		"must not persist",
		"sk-1234567890abcdefghijkl",
		"hunter2",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("metadata leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{"[REDACTED]", `"labelKey":"process.tool"`} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("metadata missing %q: %s", required, encoded)
		}
	}
}

func TestProcessTraceOmitsOrdinaryCompletedGeneration(t *testing.T) {
	trace := newProcessTrace("message-1")
	trace.start(
		ProcessStepKindGeneration,
		"process.generation",
		time.Now(),
		nil,
	)
	finishProcessTrace(trace, "completed", time.Now(), websearch.Result{})
	metadata := withProcessTraceMessageMetadata(
		map[string]any{"runId": "run-1"},
		"",
		trace,
	)
	if _, ok := metadata[processTraceMetadataKey]; ok {
		t.Fatalf("ordinary generation persisted process trace: %#v", metadata)
	}
}

func TestProcessReasoningStreamRedactsSecretsSplitAcrossProviderChunks(t *testing.T) {
	stream := newProcessReasoningStream()
	var rendered strings.Builder
	for _, chunk := range []string{
		"Checking api",
		"Key=split-super-",
		"secret-value before answering. ",
		"Bearer abcdefgh",
		"ijklmnop is also private.",
	} {
		rendered.WriteString(stream.append(chunk))
	}
	rendered.WriteString(stream.flush())

	for _, value := range []string{rendered.String(), stream.String()} {
		if strings.Contains(value, "split-super-secret-value") ||
			strings.Contains(value, "abcdefghijklmnop") {
			t.Fatalf("split provider secret leaked: %q", value)
		}
		if strings.Count(value, "[REDACTED]") != 2 {
			t.Fatalf("redacted reasoning = %q, want two markers", value)
		}
	}
}

func metadataString(t *testing.T, value map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
	return string(encoded)
}
