package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func TestProcessTracePersistsOrdinaryCompletedGenerationForDirectRoute(t *testing.T) {
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
	steps, ok := metadata[processTraceMetadataKey].([]ProcessStep)
	if !ok || len(steps) != 1 || steps[0].Kind != ProcessStepKindGeneration ||
		steps[0].Status != ProcessStepStatusCompleted {
		t.Fatalf("ordinary generation process trace = %#v", metadata)
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

func TestToolProcessTraceUpdatesRunningPresentationWithoutCreatingAnotherStep(t *testing.T) {
	trace := newProcessTrace("message-live")
	runtime := newToolProcessTrace(trace)
	running := &ProviderToolExecutionEvent{
		ExecutionID: "terminal-live", Name: localTerminalToolName,
		Status: ProcessStepStatusRunning, Round: 1, Mode: "local_direct",
		Presentation: &ProcessStepPresentation{
			Version: 1, Card: "terminal", Command: "printf fixture",
		},
	}
	if updates := runtime.apply(running, time.Now()); len(updates) != 1 {
		t.Fatalf("initial updates=%#v", updates)
	}
	progress := *running
	progress.Transient = true
	progress.Presentation = cloneProcessStepPresentation(running.Presentation)
	progress.Presentation.Transcript = []ProcessTranscriptEntry{
		{Sequence: 1, Stream: "stdout", Content: "fixture progress"},
	}
	updates := runtime.apply(&progress, time.Now())
	if len(updates) != 1 || len(updates[0].Presentation.Transcript) != 1 ||
		len(trace.snapshot()) != 1 {
		t.Fatalf("progress updates=%#v snapshot=%#v", updates, trace.snapshot())
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

func TestToolProcessTracePersistsMCPArgumentTypesAndUnknownOutcome(t *testing.T) {
	trace := newProcessTrace("message-1")
	runtime := newToolProcessTrace(trace)
	updates := runtime.apply(&ProviderToolExecutionEvent{
		ExecutionID:    "call-1",
		CallID:         "call-1",
		Name:           "write_file",
		Server:         "manifest:files",
		ServerName:     "Files",
		Classification: "write",
		Status:         ProcessStepStatusOutcomeUnknown,
		CallStatus:     "outcome_unknown",
		Round:          2,
		Arguments:      map[string]any{"path": "string", "overwrite": "boolean"},
		Mode:           "mcp",
		Durability:     "process_local",
	}, time.Now())
	if len(updates) != 2 {
		t.Fatalf("MCP outcome updates = %#v", updates)
	}
	completed := updates[1]
	if completed.Status != ProcessStepStatusOutcomeUnknown ||
		completed.Detail["server"] != "manifest:files" ||
		completed.Detail["serverName"] != "Files" ||
		completed.Detail["classification"] != "write" ||
		completed.Detail["durability"] != "process_local" ||
		completed.Detail["callStatus"] != "outcome_unknown" ||
		completed.Detail["argumentSummary"] != `{"overwrite":"boolean","path":"string"}` {
		t.Fatalf("MCP outcome step = %#v", completed)
	}
}

func TestToolProcessTraceSanitizesTerminalPresentationAndPreservesResultState(t *testing.T) {
	trace := newProcessTrace("message-1")
	runtime := newToolProcessTrace(trace)
	startedAt := time.Now()
	running := &ProviderToolExecutionEvent{
		ExecutionID: "terminal-1", Name: localTerminalToolName,
		Status: ProcessStepStatusRunning, Round: 1, Mode: "local_direct",
		Classification: "execute", Presentation: &ProcessStepPresentation{
			Card:    "terminal",
			Command: "printf token=fixture-secret; echo /workspace/private",
			CWD:     "$NEO_CHAT_WORKSPACE",
		},
	}
	updates := runtime.apply(running, startedAt)
	if len(updates) != 1 || updates[0].Presentation == nil ||
		strings.Contains(updates[0].Presentation.Command, "fixture-secret") ||
		!strings.Contains(updates[0].Presentation.Command, "[REDACTED]") {
		t.Fatalf("running terminal step = %#v", updates)
	}

	exitCode := 17
	completed := *running
	completed.Status = ProcessStepStatusCompleted
	completed.Presentation = &ProcessStepPresentation{
		Card: "terminal", Command: running.Presentation.Command,
		CWD: "$NEO_CHAT_WORKSPACE", ExitCode: &exitCode,
		TimedOut: true, Truncated: true, Background: true,
	}
	updates = runtime.apply(&completed, startedAt.Add(time.Second))
	if len(updates) != 1 || updates[0].Presentation == nil ||
		updates[0].Presentation.ExitCode == nil ||
		*updates[0].Presentation.ExitCode != 17 ||
		!updates[0].Presentation.TimedOut || !updates[0].Presentation.Truncated ||
		!updates[0].Presentation.Background {
		t.Fatalf("completed terminal step = %#v", updates)
	}

	encoded := metadataString(t, chatAgentProcessStepPayload(updates[0]))
	for _, forbidden := range []string{"fixture-secret", "stdout", "stderr"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("terminal presentation leaked %q: %s", forbidden, encoded)
		}
	}
	cloned := cloneProcessStep(updates[0])
	*cloned.Presentation.ExitCode = 0
	if *updates[0].Presentation.ExitCode != 17 {
		t.Fatal("terminal presentation exit code pointer was not cloned")
	}
}

func TestProcessTraceDropsUnauthorizedOrMalformedTerminalPresentation(t *testing.T) {
	trace := newProcessTrace("message-1")
	for index, step := range []ProcessStep{
		{
			ID: "message-1:tool:1", Kind: ProcessStepKindTool,
			Status: ProcessStepStatusRunning, LabelKey: "process.tool",
			Detail:       map[string]any{"toolName": localTerminalToolName, "mode": "mcp"},
			Presentation: &ProcessStepPresentation{Card: "terminal", Command: "pwd"},
		},
		{
			ID: "message-1:tool:2", Kind: ProcessStepKindTool,
			Status: ProcessStepStatusRunning, LabelKey: "process.tool",
			Detail:       map[string]any{"toolName": localTerminalToolName, "mode": "local_direct"},
			Presentation: &ProcessStepPresentation{Card: "unknown", Command: "pwd"},
		},
	} {
		if normalized := trace.add(step); normalized.Presentation != nil {
			t.Fatalf("case %d retained unauthorized presentation: %#v", index, normalized)
		}
	}
	bounded := trace.add(ProcessStep{
		ID: "message-1:tool:3", Kind: ProcessStepKindTool,
		Status: ProcessStepStatusRunning, LabelKey: "process.tool",
		Detail: map[string]any{"toolName": localTerminalToolName, "mode": "local_direct"},
		Presentation: &ProcessStepPresentation{
			Card: "terminal", Command: strings.Repeat("界", maxProcessTerminalCommandBytes),
		},
	})
	if bounded.Presentation == nil ||
		len(bounded.Presentation.Command) > maxProcessTerminalCommandBytes ||
		!utf8.ValidString(bounded.Presentation.Command) {
		t.Fatalf("UTF-8 terminal bound = %#v", bounded.Presentation)
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
