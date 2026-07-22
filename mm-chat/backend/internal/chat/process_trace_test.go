package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/knowledge"
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

func TestReconcileProcessTraceCitationsKeepsOnlyMarkersUsedByAnswer(t *testing.T) {
	trace := newProcessTrace("message-1")
	startedAt := time.Now()
	step := trace.startNext(
		ProcessStepKindWeb,
		"process.web",
		startedAt,
		map[string]any{"citationMarkers": []string{"[W1]", "[W2]"}},
	)
	trace.transitionID(
		step.ID,
		ProcessStepStatusCompleted,
		startedAt.Add(time.Second),
		map[string]any{
			"outcome":         "completed",
			"sourceCount":     2,
			"citationMarkers": []string{"[W1]", "[W2]"},
		},
	)

	updates := reconcileProcessTraceCitations(trace, "answer [W2]")
	if len(updates) != 1 {
		t.Fatalf("updates = %#v", updates)
	}
	markers, ok := updates[0].Detail["citationMarkers"].([]string)
	if !ok || len(markers) != 1 || markers[0] != "[W2]" {
		t.Fatalf("citation markers = %#v", updates[0].Detail)
	}

	updates = reconcileProcessTraceCitations(trace, "answer without citation")
	if len(updates) != 1 || updates[0].Detail["outcome"] != "completed_unreferenced" {
		t.Fatalf("unreferenced update = %#v", updates)
	}
	if _, exists := updates[0].Detail["citationMarkers"]; exists {
		t.Fatalf("unused markers survived = %#v", updates[0].Detail)
	}
}

func TestToolProcessTraceCreatesIndependentKnowledgeAndToolSteps(t *testing.T) {
	trace := newProcessTrace("message-1")
	runtime := newToolProcessTrace(trace)
	startedAt := time.Now()
	running := &ProviderToolExecutionEvent{
		ExecutionID: "knowledge-1",
		Name:        searchKnowledgeToolName,
		Status:      ProcessStepStatusRunning,
		Round:       2,
		Query:       "standalone fixture",
		Mode:        "native",
	}
	updates := runtime.apply(running, startedAt)
	if len(updates) != 2 || updates[0].Kind != ProcessStepKindTool ||
		updates[1].Kind != ProcessStepKindKnowledge {
		t.Fatalf("running updates = %#v", updates)
	}
	authority := &RAGAnswerAuthority{
		Processor: "fixture", ModelID: "fixture", CollectionCount: 1,
	}
	completed := *running
	completed.Status = ProcessStepStatusCompleted
	completed.CitationMarkers = []string{"[K1]"}
	completed.Knowledge = &autoRAGDecision{
		Outcome:      "evidence_ready",
		Citations:    []RAGCitation{{Marker: "[K1]"}},
		Authority:    authority,
		Evidence:     []knowledge.HydratedEvidence{{SourceText: "fixture"}},
		RerankStatus: ragRerankStatusApplied,
	}
	updates = runtime.apply(&completed, startedAt.Add(time.Second))
	if len(updates) != 2 {
		t.Fatalf("completed updates = %#v", updates)
	}
	for _, step := range updates {
		if step.Status != ProcessStepStatusCompleted ||
			step.Detail["outcome"] != "evidence_ready" ||
			step.Detail["hitCount"] != 1 ||
			step.Detail["rerankStatus"] != ragRerankStatusApplied {
			t.Fatalf("completed step = %#v", step)
		}
	}

	reconciled := reconcileProcessTraceCitations(trace, "answer without marker")
	if len(reconciled) != 2 {
		t.Fatalf("reconciled = %#v", reconciled)
	}
	for _, step := range reconciled {
		if step.Detail["outcome"] != "completed_unreferenced" {
			t.Fatalf("unreferenced step = %#v", step)
		}
	}
}

func TestToolProcessTracePreservesCancelledOutcome(t *testing.T) {
	trace := newProcessTrace("message-1")
	runtime := newToolProcessTrace(trace)
	cancelledAt := time.Now()
	updates := runtime.apply(&ProviderToolExecutionEvent{
		ExecutionID: "compatibility-plan",
		Name:        searchWebToolName,
		Status:      ProcessStepStatusCancelled,
		Round:       1,
		Mode:        "compatibility",
	}, cancelledAt)
	if len(updates) != 4 {
		t.Fatalf("cancelled updates = %#v", updates)
	}
	for _, step := range updates[2:] {
		if step.Status != ProcessStepStatusCancelled ||
			step.Detail["outcome"] != "cancelled" {
			t.Fatalf("cancelled step = %#v", step)
		}
		if _, ok := step.Detail["failureCategory"]; ok {
			t.Fatalf("cancelled step retained failure = %#v", step)
		}
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
