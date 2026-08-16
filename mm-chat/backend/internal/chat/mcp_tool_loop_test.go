package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

func TestMCPPerToolTimeoutReturnsStructuredRecoverableFailure(t *testing.T) {
	if fatal := fatalMCPExecutionError(context.Background(), context.DeadlineExceeded); fatal != nil {
		t.Fatalf("per-Tool timeout became fatal: %v", fatal)
	}
	if category := nonEmptyMCPFailure("", context.DeadlineExceeded); category != "tool_timeout" {
		t.Fatalf("timeout category=%q", category)
	}
}

func TestMCPParentRunDeadlineRemainsFatal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fatal := fatalMCPExecutionError(ctx, context.DeadlineExceeded)
	var failure *mcpRunFailure
	if !errors.As(fatal, &failure) || failure.code != "MCP_BUDGET_EXHAUSTED" {
		t.Fatalf("parent deadline failure=%#v", fatal)
	}
}

func TestMCPPerToolTimeoutContinuesTheSameModel(t *testing.T) {
	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "timeout-fixture"}
	tool := mcpclient.Tool{
		ServerRef: ref, Name: "wait", Alias: "mcp__timeout_fixture__wait",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
		},
		Classification: mcpclient.ClassificationRead, Supported: true,
	}
	config := mcpclient.DefaultConfig()
	config.Enabled = true
	config.RemoteEnabled = true
	config.CallTimeout = 20 * time.Millisecond
	config.RunTimeout = time.Second
	service, err := mcpclient.NewService(
		config,
		newMCPChatRepository(DevUserID, testConversationID, ref),
		timeoutMCPConnector{},
		nil,
		nil,
		mcpclient.Catalog{},
		[]mcpclient.Server{{
			Ref: ref, Name: "Timeout Fixture", Transport: mcpclient.TransportStreamableHTTP,
			AuthType: mcpclient.AuthNone, Status: mcpclient.ServerStatusReady,
			Tools: []mcpclient.Tool{tool}, Grants: []mcpclient.Grant{{ScopeType: "global"}},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareRun(
		context.Background(),
		DevUserID,
		testConversationID,
		"",
		"44444444-4444-4444-8444-444444444444",
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newMCPToolRuntime(service, prepared, DevUserID, "wait")
	registration, ok := newChatToolRegistry(externalWebToolLoopInput{MCP: runtime}).lookup(tool.Alias)
	if !ok || registration.RiskClass != chatToolRiskRead || !registration.AllowParallel ||
		registration.Timeout != config.CallTimeout ||
		registration.MaxOutputBytes != config.MaxResultCallBytes {
		t.Fatalf("MCP registry policy=%#v", registration)
	}
	runtime.searchRequired = true
	searchRegistration, ok := newChatToolRegistry(
		externalWebToolLoopInput{MCP: runtime},
	).lookup(mcpclient.ToolSearchAlias)
	if !ok || searchRegistration.AllowParallel {
		t.Fatalf("mcp_tool_search must remain a catalog mutation barrier: %#v / %v", searchRegistration, ok)
	}
	runtime.searchRequired = false
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "timeout", Name: tool.Alias, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "reported timeout"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "wait", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		MCP: runtime,
	})
	var answer strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
	}
	if answer.String() != "reported timeout" || len(provider.inputs) != 2 {
		t.Fatalf("answer/steps=%q/%d", answer.String(), len(provider.inputs))
	}
	result := provider.inputs[1].Continuation[0].Results[0]
	if !result.IsError || !strings.Contains(result.Content, "tool_timeout") {
		t.Fatalf("timeout result=%#v", result)
	}
}

type timeoutMCPConnector struct{}

func (timeoutMCPConnector) Connect(
	context.Context,
	mcpclient.Server,
	string,
) (mcpclient.Session, error) {
	return timeoutMCPSession{}, nil
}

type timeoutMCPSession struct{}

func (timeoutMCPSession) ListTools(context.Context) ([]mcpclient.Tool, error) {
	return nil, nil
}

func (timeoutMCPSession) CallTool(
	ctx context.Context,
	_ string,
	_ map[string]any,
) (mcpclient.CallResult, error) {
	<-ctx.Done()
	return mcpclient.CallResult{}, ctx.Err()
}

func (timeoutMCPSession) Close() error { return nil }
