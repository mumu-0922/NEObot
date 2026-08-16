package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/mcpclient"
)

const schedulerMCPAlias = "mcp__scheduler__signal"

func TestChatToolSchedulerRunsCrossBackendSafeReadsInParallel(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: 2 * time.Second,
		RunTimeout: 3 * time.Second, MaxOutput: 64 << 10, MaxCalls: 8,
		MaxRounds: 4, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	localRuntime := newLocalSkillToolRuntime(executor, nil)
	localRuntime.bindJobScope("scheduler-user", "scheduler-conversation")
	job, err := executor.StartBackgroundJob(context.Background(), localskills.JobStartRequest{
		Scope: localRuntime.jobScope,
		Command: localskills.Request{
			Command:        `while [ ! -f mcp-started ]; do sleep 0.01; done; printf local-ready`,
			TimeoutSeconds: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connector := &schedulerMCPConnector{
		marker: filepath.Join(workspace, "mcp-started"),
	}
	mcpRuntime := newSchedulerMCPRuntime(t, connector)
	input := externalWebToolLoopInput{MCP: mcpRuntime, LocalSkills: localRuntime}
	registry := newChatToolRegistry(input)
	calls := []ProviderToolCall{
		{
			ID: "local-wait", Name: localJobOutputToolName,
			Arguments: `{"jobId":"` + job.ID + `","wait":true,"timeoutSeconds":1}`,
		},
		{
			ID: "mcp-signal", Name: schedulerMCPAlias,
			Arguments: `{"phase":"signal"}`,
		},
	}
	execution, err := registry.executeToolBatch(
		context.Background(), make(chan ProviderEvent, 32), input, calls, 1, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Results) != 2 ||
		!strings.Contains(execution.Results[0].Content, `"status":"completed"`) ||
		!strings.Contains(execution.Results[0].Content, `"stdout":"local-ready"`) ||
		!strings.Contains(execution.Results[1].Content, `"text":"signal"`) {
		t.Fatalf("parallel results = %#v", execution.Results)
	}
}

func TestChatToolSchedulerKeepsWriteAsOrderedBarrier(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "state.txt"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 3 * time.Second, MaxOutput: 64 << 10, MaxCalls: 8,
		MaxRounds: 4, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	initial, err := executor.ReadWorkspaceFile(context.Background(), localskills.FileReadRequest{
		Path: "state.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	localRuntime := newLocalSkillToolRuntime(executor, nil)
	input := externalWebToolLoopInput{LocalSkills: localRuntime}
	registry := newChatToolRegistry(input)
	calls := []ProviderToolCall{
		{
			ID: "read-before", Name: localFileReadToolName,
			Arguments: `{"path":"state.txt","offset":null,"limit":null}`,
		},
		{
			ID: "write", Name: localFileWriteToolName,
			Arguments: `{"path":"state.txt","content":"after","expectedVersion":"` +
				initial.Version + `"}`,
		},
		{
			ID: "read-after", Name: localFileReadToolName,
			Arguments: `{"path":"state.txt","offset":null,"limit":null}`,
		},
	}
	execution, err := registry.executeToolBatch(
		context.Background(), make(chan ProviderEvent, 32), input, calls, 1, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Results[0].CallID != "read-before" ||
		!strings.Contains(execution.Results[0].Content, `"content":"before"`) ||
		execution.Results[1].CallID != "write" || execution.Results[1].IsError ||
		execution.Results[2].CallID != "read-after" ||
		!strings.Contains(execution.Results[2].Content, `"content":"after"`) {
		t.Fatalf("barrier results = %#v", execution.Results)
	}
}

func TestChatAgentGoalConclusionPreservesEarlierToolOrderAndRejectsOnlyLaterCalls(t *testing.T) {
	repository := newGoalTestRepository(t)
	repository.goal = &ChatAgentGoal{
		ID: goalTestGoalID, ConversationID: testConversationID,
		Objective: "finish in order", Phase: ChatAgentGoalActive, Revision: 1,
		MaxGoalRounds: 4, CreatedAt: testNow(), UpdatedAt: testNow(),
	}
	goalRuntime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "proof.txt"), []byte("present"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 8,
		MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	localRuntime := newLocalSkillToolRuntime(executor, nil)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "before", Name: localFileReadToolName,
				Arguments: `{"path":"proof.txt","offset":null,"limit":null}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "complete", Name: chatAgentUpdateGoalToolName,
				Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"complete","objective":null,"maxGoalRounds":null,"blockedReason":null}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "after", Name: localFileReadToolName,
				Arguments: `{"path":"proof.txt","offset":null,"limit":null}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "ordered wrap-up"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "finish in order",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		LocalSkills: localRuntime, Goals: goalRuntime,
	})
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	if len(provider.inputs) != 2 || len(provider.inputs[1].Continuation) != 1 {
		t.Fatalf("provider inputs = %#v", provider.inputs)
	}
	results := provider.inputs[1].Continuation[0].Results
	if len(results) != 3 || results[0].CallID != "before" || results[0].IsError ||
		!strings.Contains(results[0].Content, `"content":"present"`) ||
		results[1].CallID != "complete" || results[1].IsError ||
		results[2].CallID != "after" || !results[2].IsError ||
		!strings.Contains(results[2].Content, "goal_concluded") {
		t.Fatalf("ordered Goal results = %#v", results)
	}
	if localRuntime.calls != 1 {
		t.Fatalf("local calls = %d, want only the pre-conclusion read", localRuntime.calls)
	}
}

func TestChatToolSchedulerReturnsParallelResultsInModelOrder(t *testing.T) {
	connector := &schedulerMCPConnector{delays: map[string]time.Duration{
		"first": 120 * time.Millisecond,
	}}
	runtime := newSchedulerMCPRuntime(t, connector)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "first", Name: schedulerMCPAlias, Arguments: `{"phase":"first"}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "second", Name: schedulerMCPAlias, Arguments: `{"phase":"second"}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "ordered"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "run both", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		MCP: runtime,
	})
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	if len(provider.inputs) != 2 || len(provider.inputs[1].Continuation) != 1 {
		t.Fatalf("provider inputs = %#v", provider.inputs)
	}
	results := provider.inputs[1].Continuation[0].Results
	if len(results) != 2 || results[0].CallID != "first" ||
		!strings.Contains(results[0].Content, `"text":"first"`) ||
		results[1].CallID != "second" ||
		!strings.Contains(results[1].Content, `"text":"second"`) {
		t.Fatalf("ordered results = %#v", results)
	}
	if completed := connector.completedPhases(); len(completed) != 2 ||
		completed[0] != "second" || completed[1] != "first" {
		t.Fatalf("completion order = %#v", completed)
	}
}

func TestChatToolSchedulerCapsContiguousReadGroupAtFour(t *testing.T) {
	connector := &schedulerMCPConnector{
		gateSize: maxParallelChatReadTools,
		gate:     make(chan struct{}),
	}
	runtime := newSchedulerMCPRuntime(t, connector)
	input := externalWebToolLoopInput{MCP: runtime}
	registry := newChatToolRegistry(input)
	calls := make([]ProviderToolCall, 0, maxParallelChatReadTools+1)
	for index := 0; index < maxParallelChatReadTools+1; index++ {
		phase := string(rune('a' + index))
		if index == maxParallelChatReadTools {
			phase = "fifth"
		}
		calls = append(calls, ProviderToolCall{
			ID: phase, Name: schedulerMCPAlias,
			Arguments: `{"phase":"` + phase + `"}`,
		})
	}
	execution, err := registry.executeToolBatch(
		context.Background(), make(chan ProviderEvent, 64), input, calls, 1, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	maximum, completedBeforeFifth := connector.concurrencyStats()
	if len(execution.Results) != len(calls) || maximum != maxParallelChatReadTools ||
		completedBeforeFifth != maxParallelChatReadTools {
		t.Fatalf(
			"read cap results=%d maximum=%d completedBeforeFifth=%d",
			len(execution.Results), maximum, completedBeforeFifth,
		)
	}
}

func newSchedulerMCPRuntime(t *testing.T, connector mcpclient.Connector) *mcpToolRuntime {
	t.Helper()
	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "scheduler"}
	tool := mcpclient.Tool{
		ServerRef: ref, Name: "signal", Alias: schedulerMCPAlias,
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"phase"},
			"properties": map[string]any{
				"phase": map[string]any{"type": "string"},
			},
		},
		Classification: mcpclient.ClassificationRead, Supported: true,
	}
	config := mcpclient.DefaultConfig()
	config.Enabled = true
	config.RemoteEnabled = true
	config.CallTimeout = 2 * time.Second
	config.RunTimeout = 3 * time.Second
	service, err := mcpclient.NewService(
		config,
		newMCPChatRepository(DevUserID, testConversationID, ref),
		connector,
		nil,
		nil,
		mcpclient.Catalog{},
		[]mcpclient.Server{{
			Ref: ref, Name: "Scheduler Fixture",
			Transport: mcpclient.TransportStreamableHTTP,
			AuthType:  mcpclient.AuthNone, Status: mcpclient.ServerStatusReady,
			Tools: []mcpclient.Tool{tool}, Grants: []mcpclient.Grant{{ScopeType: "global"}},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareRun(
		context.Background(), DevUserID, testConversationID, "",
		"45454545-4545-4545-8545-454545454545",
	)
	if err != nil {
		t.Fatal(err)
	}
	return newMCPToolRuntime(service, prepared, DevUserID, "signal")
}

type schedulerMCPConnector struct {
	marker   string
	delays   map[string]time.Duration
	gateSize int
	gate     chan struct{}
	gateOnce sync.Once

	mu                   sync.Mutex
	active               int
	maximumActive        int
	completed            []string
	completedBeforeFifth int
}

func (connector *schedulerMCPConnector) Connect(
	context.Context,
	mcpclient.Server,
	string,
) (mcpclient.Session, error) {
	return schedulerMCPSession{connector: connector}, nil
}

func (connector *schedulerMCPConnector) completedPhases() []string {
	connector.mu.Lock()
	defer connector.mu.Unlock()
	return append([]string(nil), connector.completed...)
}

func (connector *schedulerMCPConnector) concurrencyStats() (int, int) {
	connector.mu.Lock()
	defer connector.mu.Unlock()
	return connector.maximumActive, connector.completedBeforeFifth
}

type schedulerMCPSession struct {
	connector *schedulerMCPConnector
}

func (schedulerMCPSession) ListTools(context.Context) ([]mcpclient.Tool, error) {
	return nil, nil
}

func (session schedulerMCPSession) CallTool(
	ctx context.Context,
	_ string,
	arguments map[string]any,
) (mcpclient.CallResult, error) {
	phase, _ := arguments["phase"].(string)
	if session.connector.marker != "" {
		if err := os.WriteFile(session.connector.marker, []byte("started"), 0o600); err != nil {
			return mcpclient.CallResult{}, err
		}
	}
	session.connector.mu.Lock()
	session.connector.active++
	if session.connector.active > session.connector.maximumActive {
		session.connector.maximumActive = session.connector.active
	}
	if phase == "fifth" {
		session.connector.completedBeforeFifth = len(session.connector.completed)
	}
	if session.connector.gate != nil &&
		session.connector.active >= session.connector.gateSize {
		session.connector.gateOnce.Do(func() { close(session.connector.gate) })
	}
	session.connector.mu.Unlock()
	if session.connector.gate != nil {
		select {
		case <-ctx.Done():
			return mcpclient.CallResult{}, ctx.Err()
		case <-session.connector.gate:
		}
	}
	if delay := session.connector.delays[phase]; delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return mcpclient.CallResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	session.connector.mu.Lock()
	session.connector.active--
	session.connector.completed = append(session.connector.completed, phase)
	session.connector.mu.Unlock()
	return mcpclient.CallResult{Content: []mcpclient.Content{{
		Type: "text", Text: phase,
	}}}, nil
}

func (schedulerMCPSession) Close() error { return nil }
