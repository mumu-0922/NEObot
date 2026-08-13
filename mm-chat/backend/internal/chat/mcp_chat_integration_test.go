package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/mcprunner"
)

const mcpChatStdioHelperEnvironment = "MM_CHAT_MCP_CHAT_STDIO_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(mcpChatStdioHelperEnvironment) == "1" {
		runMCPChatStdioHelper()
		return
	}
	os.Exit(m.Run())
}

func runMCPChatStdioHelper() {
	server := protocol.NewServer(
		&protocol.Implementation{Name: "chat-stdio-fixture", Version: "1"},
		nil,
	)
	server.AddTool(&protocol.Tool{
		Name: "echo",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"value"},
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
		},
	}, func(_ context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
		var arguments struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &protocol.CallToolResult{Content: []protocol.Content{
			&protocol.TextContent{Text: "stdio:" + arguments.Value},
		}}, nil
	})
	if err := server.Run(context.Background(), &protocol.StdioTransport{}); err != nil {
		log.Print(err)
	}
}

func TestHandlerCompletesNativeMultiRoundMCPThroughRemoteStreamableHTTP(t *testing.T) {
	remote := protocol.NewServer(
		&protocol.Implementation{Name: "chat-mcp-fixture", Version: "1"},
		nil,
	)
	remote.AddTool(&protocol.Tool{
		Name:        "echo",
		Description: "Return the supplied fixture value.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"value"},
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
		},
	}, func(_ context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
		var arguments struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &protocol.CallToolResult{Content: []protocol.Content{
			&protocol.TextContent{Text: "remote:" + arguments.Value},
		}}, nil
	})
	remoteHTTP := httptest.NewServer(protocol.NewStreamableHTTPHandler(
		func(*http.Request) *protocol.Server { return remote },
		&protocol.StreamableHTTPOptions{JSONResponse: true},
	))
	defer remoteHTTP.Close()

	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "remote-fixture"}
	mcpRepo := newMCPChatRepository(DevUserID, testConversationID, ref)
	mcpConfig := mcpclient.DefaultConfig()
	mcpConfig.Enabled = true
	mcpConfig.RemoteEnabled = true
	mcpService, err := mcpclient.NewService(
		mcpConfig,
		mcpRepo,
		mcpclient.NewDirectConnector("neo-chat-test", "1"),
		nil,
		nil,
		mcpclient.Catalog{},
		[]mcpclient.Server{{
			Ref:         ref,
			Name:        "Remote Fixture",
			Transport:   mcpclient.TransportStreamableHTTP,
			EndpointURL: remoteHTTP.URL,
			AuthType:    mcpclient.AuthNone,
			Grants:      []mcpclient.Grant{{ScopeType: "global"}},
			Metadata: map[string]any{
				"toolPolicy": map[string]string{"echo": mcpclient.ClassificationRead},
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := mcpService.Preflight(context.Background(), DevUserID, testConversationID)
	if err != nil {
		t.Fatalf("MCP preflight: %v", err)
	}
	tools, _ := mcpService.ToolsForProvider(preflight, "echo fixture")
	if len(tools) != 1 {
		t.Fatalf("preflight tools = %#v", tools)
	}

	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "mcp-call-1", Name: tools[0].Alias, Arguments: `{"value":"hello"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "Remote MCP completed."}},
	}}
	chatRepo := newFakeRepository()
	chatRepo.conversations = append(
		chatRepo.conversations,
		fakeConversation(testConversationID, "MCP", 0),
	)
	chatRepo.messages[testConversationID] = append(
		chatRepo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "echo fixture"),
	)
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(provider),
		WithMCPService(mcpService),
	)

	recorder := performAuthenticatedRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mcp-native"},"idempotencyKey":"mcp-remote-stream"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if !strings.Contains(recorder.Body.String(), "event: tool.call.updated") ||
		!strings.Contains(recorder.Body.String(), `"server":"manifest:remote-fixture"`) ||
		!strings.Contains(recorder.Body.String(), `"serverName":"Remote Fixture"`) ||
		strings.Contains(recorder.Body.String(), `"value":"hello"`) {
		t.Fatalf("MCP stream = %s", recorder.Body.String())
	}
	if len(provider.inputs) != 2 || len(provider.inputs[1].Continuation) != 1 ||
		len(provider.inputs[1].Continuation[0].Results) != 1 ||
		!strings.Contains(provider.inputs[1].Continuation[0].Results[0].Content, "remote:hello") ||
		!strings.Contains(provider.inputs[1].Continuation[0].Results[0].Content, "untrustedMcpToolResult") {
		t.Fatalf("MCP continuation = %#v", provider.inputs)
	}

	messages := chatRepo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "completed" ||
		messages[1].Content != "Remote MCP completed." {
		t.Fatalf("persisted messages = %#v", messages)
	}
	steps, ok := messages[1].Metadata[processTraceMetadataKey].([]ProcessStep)
	if !ok || !hasCompletedMCPToolStep(steps, "manifest:remote-fixture", "Remote Fixture") {
		t.Fatalf("persisted MCP process trace = %#v", messages[1].Metadata)
	}
	calls := mcpRepo.callRecords()
	if len(calls) != 1 || calls[0].Status != mcpclient.CallStatusSucceeded ||
		calls[0].ArgumentsSummary["value"] != "string" ||
		calls[0].ConversationID != testConversationID {
		t.Fatalf("persisted MCP calls = %#v", calls)
	}
}

func TestHandlerCompletesNativeMultiRoundMCPThroughStdioRunner(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "stdio-fixture"}
	server := mcpclient.Server{
		Ref:       ref,
		Name:      "Stdio Fixture",
		Transport: mcpclient.TransportStdio,
		AuthType:  mcpclient.AuthNone,
		Grants:    []mcpclient.Grant{{ScopeType: "global"}},
		Command: &mcpclient.Command{
			Argv: []string{executable},
			Env: map[string]string{
				mcpChatStdioHelperEnvironment: "1",
			},
			IdleTimeout: 15 * time.Minute,
			MaxLifetime: 24 * time.Hour,
		},
		Metadata: map[string]any{
			"toolPolicy": map[string]string{"echo": mcpclient.ClassificationRead},
		},
	}
	runnerManager, err := mcprunner.NewManager(
		mcprunner.Config{MaxProcesses: 1, WorkRoot: t.TempDir()},
		[]mcpclient.Server{server},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runnerManager.Close() })
	const runnerToken = "chat-stdio-runner-token-with-thirty-two-bytes"
	runnerHandler, err := mcprunner.NewHandler(runnerManager, runnerToken)
	if err != nil {
		t.Fatal(err)
	}
	runnerHTTP := httptest.NewServer(runnerHandler)
	defer runnerHTTP.Close()
	runnerConnector, err := mcpclient.NewRunnerConnector(runnerHTTP.URL, runnerToken)
	if err != nil {
		t.Fatal(err)
	}

	mcpRepo := newMCPChatRepository(DevUserID, testConversationID, ref)
	mcpConfig := mcpclient.DefaultConfig()
	mcpConfig.Enabled = true
	mcpConfig.StdioEnabled = true
	mcpService, err := mcpclient.NewService(
		mcpConfig,
		mcpRepo,
		mcpclient.NewRoutingConnector(nil, runnerConnector),
		nil,
		nil,
		mcpclient.Catalog{},
		[]mcpclient.Server{server},
	)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := mcpService.Preflight(context.Background(), DevUserID, testConversationID)
	if err != nil {
		t.Fatalf("MCP stdio preflight: %v", err)
	}
	tools, _ := mcpService.ToolsForProvider(preflight, "echo fixture")
	if len(tools) != 1 {
		t.Fatalf("stdio preflight tools = %#v", tools)
	}

	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "mcp-stdio-call-1", Name: tools[0].Alias, Arguments: `{"value":"hello"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "Stdio MCP completed."}},
	}}
	chatRepo := newFakeRepository()
	chatRepo.conversations = append(
		chatRepo.conversations,
		fakeConversation(testConversationID, "MCP stdio", 0),
	)
	chatRepo.messages[testConversationID] = append(
		chatRepo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "echo fixture"),
	)
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(provider),
		WithMCPService(mcpService),
	)
	recorder := performAuthenticatedRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mcp-native"},"idempotencyKey":"mcp-stdio-stream"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if !strings.Contains(recorder.Body.String(), "event: tool.call.updated") ||
		!strings.Contains(recorder.Body.String(), `"server":"manifest:stdio-fixture"`) ||
		!strings.Contains(recorder.Body.String(), `"serverName":"Stdio Fixture"`) ||
		strings.Contains(recorder.Body.String(), `"value":"hello"`) {
		t.Fatalf("MCP stdio stream = %s", recorder.Body.String())
	}
	if len(provider.inputs) != 2 || len(provider.inputs[1].Continuation) != 1 ||
		len(provider.inputs[1].Continuation[0].Results) != 1 ||
		!strings.Contains(provider.inputs[1].Continuation[0].Results[0].Content, "stdio:hello") {
		t.Fatalf("MCP stdio continuation = %#v", provider.inputs)
	}
	messages := chatRepo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "completed" ||
		messages[1].Content != "Stdio MCP completed." {
		t.Fatalf("persisted stdio messages = %#v", messages)
	}
	steps, ok := messages[1].Metadata[processTraceMetadataKey].([]ProcessStep)
	if !ok || !hasCompletedMCPToolStep(steps, "manifest:stdio-fixture", "Stdio Fixture") {
		t.Fatalf("persisted stdio process trace = %#v", messages[1].Metadata)
	}
	calls := mcpRepo.callRecords()
	if len(calls) != 1 || calls[0].Status != mcpclient.CallStatusSucceeded {
		t.Fatalf("persisted stdio calls = %#v", calls)
	}
}

func TestMCPProviderToolDefinitionDoesNotClaimStrictSchemaCompatibility(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repoName": map[string]any{"type": "string"},
		},
		"required": []string{"repoName"},
	}
	definition := mcpProviderToolDefinition(mcpclient.Tool{
		Alias:       "mcp_deepwiki_read_wiki_structure",
		Description: "Return repository documentation topics.",
		InputSchema: schema,
	})
	if definition.Function.Strict {
		t.Fatal("arbitrary MCP schema was incorrectly advertised as strict-compatible")
	}
	if definition.Function.Parameters["additionalProperties"] != nil {
		t.Fatalf("MCP schema was rewritten = %#v", definition.Function.Parameters)
	}
}

func TestMCPProviderStartFailureCodeDistinguishesCapabilityFromRequestFailure(t *testing.T) {
	if got := mcpProviderStartFailureCode(errors.New("tools are not supported by this model")); got != "MCP_MODEL_UNSUPPORTED" {
		t.Fatalf("explicit incompatibility code = %q", got)
	}
	if got := mcpProviderStartFailureCode(newProviderFailure(
		ProviderFailureRequestRejected,
		"openai-compatible provider returned status 400",
	)); got != "MCP_PROVIDER_FAILED" {
		t.Fatalf("generic provider rejection code = %q", got)
	}
}

func TestMCPToolLoopEnforcesWallClockBudget(t *testing.T) {
	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "budget-fixture"}
	tool := mcpclient.Tool{
		ServerRef: ref,
		Name:      "echo",
		Alias:     "mcp_budget_echo",
		InputSchema: map[string]any{
			"type": "object",
		},
		Classification: mcpclient.ClassificationRead,
		Supported:      true,
	}
	repo := newMCPChatRepository(DevUserID, testConversationID, ref)
	config := mcpclient.DefaultConfig()
	config.Enabled = true
	config.RemoteEnabled = true
	config.RunTimeout = 25 * time.Millisecond
	service, err := mcpclient.NewService(
		config,
		repo,
		nil,
		nil,
		nil,
		mcpclient.Catalog{},
		[]mcpclient.Server{{
			Ref: ref, Name: "Budget Fixture", Transport: mcpclient.TransportStreamableHTTP,
			AuthType: mcpclient.AuthNone, Status: mcpclient.ServerStatusReady,
			Tools: []mcpclient.Tool{tool}, Grants: []mcpclient.Grant{{ScopeType: "global"}},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Preflight(context.Background(), DevUserID, testConversationID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newMCPToolRuntime(service, prepared, DevUserID, "wait")
	started := time.Now()
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: &blockingMCPRoundProvider{},
		Request: ProviderRequest{
			Prompt: "wait", ModelRef: ModelRef{ProviderID: "mock", ModelID: "mcp"},
		},
		MCP: runtime,
	})
	var failure *mcpRunFailure
	for event := range events {
		if event.Error != nil {
			if !errors.As(event.Error, &failure) {
				t.Fatalf("budget error = %v", event.Error)
			}
		}
	}
	if failure == nil || failure.code != "MCP_BUDGET_EXHAUSTED" ||
		!errors.Is(failure, mcpclient.ErrToolBudget) {
		t.Fatalf("budget failure = %#v", failure)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("wall-clock budget took %s", elapsed)
	}
}

func TestHandlerKeepsChatAvailableAndCleansMCPDataWhenKillSwitchOff(t *testing.T) {
	ref := mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "disabled-fixture"}
	mcpRepo := newMCPChatRepository(DevUserID, testConversationID, ref)
	mcpService, err := mcpclient.NewService(
		mcpclient.DefaultConfig(),
		mcpRepo,
		nil,
		nil,
		nil,
		mcpclient.Catalog{},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	chatRepo := newFakeRepository()
	chatRepo.conversations = append(
		chatRepo.conversations,
		fakeConversation(testConversationID, "MCP disabled", 0),
	)
	chatRepo.messages[testConversationID] = append(
		chatRepo.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "ordinary chat"),
	)
	handler := NewHandler(
		NewService(chatRepo),
		WithProvider(NewMockProvider()),
		WithMCPService(mcpService),
	)

	stream := performAuthenticatedRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"mcp-disabled-chat"}`,
	)
	assertStreamStatus(t, stream, http.StatusOK)
	if !strings.Contains(stream.Body.String(), "event: message.completed") {
		t.Fatalf("disabled MCP chat stream = %s", stream.Body.String())
	}

	deleted := performAuthenticatedRequest(
		handler,
		http.MethodDelete,
		conversationsPath+"/"+testConversationID,
		"",
	)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("disabled MCP delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	mcpRepo.mu.Lock()
	lifecycleDeleted := mcpRepo.lifecycleDeleted
	mcpRepo.mu.Unlock()
	if !lifecycleDeleted {
		t.Fatal("MCP lifecycle data was not cleaned while execution was disabled")
	}
}

type blockingMCPRoundProvider struct{}

func (*blockingMCPRoundProvider) StreamToolRound(ctx context.Context, _ ProviderRoundRequest) (<-chan ProviderEvent, error) {
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		<-ctx.Done()
	}()
	return events, nil
}

func (*blockingMCPRoundProvider) StreamChat(ctx context.Context, _ ProviderRequest) (<-chan ProviderEvent, error) {
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		<-ctx.Done()
	}()
	return events, nil
}

func hasCompletedMCPToolStep(steps []ProcessStep, server string, serverName string) bool {
	for _, step := range steps {
		if step.Kind == ProcessStepKindTool && step.Status == ProcessStepStatusCompleted &&
			step.Detail["mode"] == "mcp" && step.Detail["server"] == server &&
			step.Detail["serverName"] == serverName {
			return true
		}
	}
	return false
}

type mcpChatRepository struct {
	mu               sync.Mutex
	userID           string
	conversationID   string
	selection        mcpclient.Selection
	snapshots        []mcpclient.RunSnapshot
	calls            map[string]mcpclient.CallRecord
	results          map[string][]mcpclient.Content
	lifecycleDeleted bool
}

func newMCPChatRepository(userID, conversationID string, ref mcpclient.ServerRef) *mcpChatRepository {
	return &mcpChatRepository{
		userID: userID, conversationID: conversationID,
		selection: mcpclient.Selection{
			ConversationID: conversationID,
			Mode:           mcpclient.SelectionModeCustom,
			Revision:       1,
			Servers:        []mcpclient.SelectionServer{{Ref: ref}},
		},
		calls: map[string]mcpclient.CallRecord{}, results: map[string][]mcpclient.Content{},
	}
}

func (r *mcpChatRepository) CountPrivateServers(context.Context, string) (int, error) {
	return 0, nil
}
func (r *mcpChatRepository) CreatePrivateServer(context.Context, string, mcpclient.CreateServerInput) (mcpclient.Server, error) {
	return mcpclient.Server{}, errors.New("unexpected private server create")
}
func (r *mcpChatRepository) ListPrivateServers(context.Context, string) ([]mcpclient.Server, error) {
	return []mcpclient.Server{}, nil
}
func (r *mcpChatRepository) ListSharedServers(context.Context, string, string) ([]mcpclient.Server, error) {
	return []mcpclient.Server{}, nil
}
func (r *mcpChatRepository) GetPrivateServer(context.Context, string, string) (mcpclient.Server, error) {
	return mcpclient.Server{}, mcpclient.ErrServerNotFound
}
func (r *mcpChatRepository) GetAccessiblePrivateServer(context.Context, string, string, string) (mcpclient.Server, error) {
	return mcpclient.Server{}, mcpclient.ErrServerNotFound
}
func (r *mcpChatRepository) UpdateServerValidation(context.Context, string, string, string, []mcpclient.Tool, string, string, *time.Time) (mcpclient.Server, error) {
	return mcpclient.Server{}, errors.New("unexpected private server validation")
}
func (r *mcpChatRepository) DeletePrivateServer(context.Context, string, string) error {
	return errors.New("unexpected private server delete")
}
func (r *mcpChatRepository) GetCredential(context.Context, string, mcpclient.ServerRef) (mcpclient.Credential, bool, error) {
	return mcpclient.Credential{}, false, nil
}
func (r *mcpChatRepository) UpsertCredential(context.Context, mcpclient.Credential) (mcpclient.Credential, error) {
	return mcpclient.Credential{}, errors.New("unexpected credential upsert")
}
func (r *mcpChatRepository) DeleteCredential(context.Context, string, mcpclient.ServerRef) error {
	return errors.New("unexpected credential delete")
}
func (r *mcpChatRepository) ConversationScope(_ context.Context, userID, conversationID string) (mcpclient.ConversationScope, error) {
	if userID != r.userID || conversationID != r.conversationID {
		return mcpclient.ConversationScope{}, mcpclient.ErrSelectionInvalid
	}
	return mcpclient.ConversationScope{UserID: userID, ConversationID: conversationID}, nil
}
func (r *mcpChatRepository) GetSelection(_ context.Context, userID, conversationID string) (mcpclient.Selection, bool, error) {
	if userID != r.userID || conversationID != r.conversationID {
		return mcpclient.Selection{}, false, mcpclient.ErrSelectionInvalid
	}
	return r.selection, true, nil
}
func (r *mcpChatRepository) ReplaceSelection(context.Context, string, mcpclient.Selection) (mcpclient.Selection, error) {
	return mcpclient.Selection{}, errors.New("unexpected selection replace")
}
func (r *mcpChatRepository) GetWorkspaceSelection(context.Context, string, string) (mcpclient.WorkspaceSelection, bool, error) {
	return mcpclient.WorkspaceSelection{}, false, nil
}
func (r *mcpChatRepository) ReplaceWorkspaceSelection(context.Context, string, mcpclient.WorkspaceSelection) (mcpclient.WorkspaceSelection, error) {
	return mcpclient.WorkspaceSelection{}, errors.New("unexpected workspace selection replace")
}
func (r *mcpChatRepository) CreateRunSnapshot(_ context.Context, snapshot mcpclient.RunSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots = append(r.snapshots, snapshot)
	return nil
}
func (r *mcpChatRepository) CountPendingOAuthStates(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (r *mcpChatRepository) CreateOAuthState(context.Context, mcpclient.OAuthState) error {
	return errors.New("unexpected oauth state create")
}
func (r *mcpChatRepository) ConsumeOAuthState(context.Context, string, time.Time) (mcpclient.OAuthState, error) {
	return mcpclient.OAuthState{}, errors.New("unexpected oauth state consume")
}
func (r *mcpChatRepository) CreateCall(_ context.Context, userID string, call mcpclient.CallRecord) (mcpclient.CallRecord, error) {
	if userID != r.userID {
		return mcpclient.CallRecord{}, mcpclient.ErrSelectionInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	call.ID = uuid.NewString()
	r.calls[call.ID] = call
	return call, nil
}
func (r *mcpChatRepository) FinishCall(_ context.Context, userID string, call mcpclient.CallRecord, result []mcpclient.Content, _ []string, _ int64) error {
	if userID != r.userID {
		return mcpclient.ErrSelectionInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[call.ID] = call
	r.results[call.ID] = append([]mcpclient.Content(nil), result...)
	return nil
}
func (r *mcpChatRepository) ListCalls(_ context.Context, userID, conversationID, runID string) ([]mcpclient.CallRecord, error) {
	if userID != r.userID || conversationID != r.conversationID {
		return nil, mcpclient.ErrSelectionInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]mcpclient.CallRecord, 0, len(r.calls))
	for _, call := range r.calls {
		if runID == "" || call.RunID == runID {
			result = append(result, call)
		}
	}
	return result, nil
}
func (r *mcpChatRepository) callRecords() []mcpclient.CallRecord {
	records, _ := r.ListCalls(context.Background(), r.userID, r.conversationID, "")
	return records
}

func (r *mcpChatRepository) ListConversationObjectKeys(_ context.Context, userID, conversationID string) ([]string, error) {
	if userID != r.userID || conversationID != r.conversationID {
		return nil, mcpclient.ErrSelectionInvalid
	}
	return []string{}, nil
}

func (r *mcpChatRepository) DeleteConversationData(_ context.Context, userID, conversationID string) error {
	if userID != r.userID || conversationID != r.conversationID {
		return mcpclient.ErrSelectionInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lifecycleDeleted = true
	return nil
}

var _ mcpclient.Repository = (*mcpChatRepository)(nil)
var _ mcpclient.ConversationLifecycleRepository = (*mcpChatRepository)(nil)
