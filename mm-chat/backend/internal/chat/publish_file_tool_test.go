package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestPublishFileToolPublishesBinaryOnceAndProjectsAttachment(t *testing.T) {
	workspace := t.TempDir()
	body := []byte{0x00, 0xff, 'p', 'n', 'g'}
	if err := os.WriteFile(filepath.Join(workspace, "chart.png"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	publisher := &fakeWorkspaceArtifactPublisher{}
	runtime := newPublishFileTestRuntime(t, workspace, publisher, 1024)
	definition := findLocalToolDefinition(t, runtime, localPublishFileToolName)
	if !definition.Function.Strict || definition.Function.Parameters["additionalProperties"] != false {
		t.Fatalf("publish definition=%#v", definition)
	}
	if !strings.Contains(runtime.promptInstruction(), publishFileSystemInstruction) {
		t.Fatal("publish instruction missing when publisher is available")
	}
	unavailable := newLocalSkillToolRuntime(runtime.executor, nil)
	if unavailable.handles(localPublishFileToolName) ||
		strings.Contains(unavailable.promptInstruction(), publishFileSystemInstruction) {
		t.Fatal("unavailable publish Tool leaked into definitions or prompt")
	}
	registration, ok := newChatToolRegistry(externalWebToolLoopInput{
		LocalSkills: runtime,
	}).lookup(localPublishFileToolName)
	if !ok || registration.RiskClass != chatToolRiskWrite || registration.AllowParallel ||
		registration.MutationResultNeedsFollowup {
		t.Fatalf("publish registration=%#v/%v", registration, ok)
	}

	call := ProviderToolCall{
		ID: "publish-1", Name: localPublishFileToolName,
		Arguments: `{"path":"chart.png","displayName":"gold-chart.png","contentType":"image/png"}`,
	}
	result, failure, err := runtime.executeWorkspaceToolCall(context.Background(), call)
	if err != nil || failure != "" || result.IsError {
		t.Fatalf("publish result=%#v failure=%q err=%v", result, failure, err)
	}
	if len(publisher.inputs) != 1 || publisher.inputs[0].ConversationID != testConversationID ||
		publisher.inputs[0].FileName != "gold-chart.png" ||
		publisher.inputs[0].MimeType != "image/png" ||
		string(publisher.inputs[0].Body) != string(body) {
		t.Fatalf("publish inputs=%#v", publisher.inputs)
	}
	if !strings.Contains(result.Content, `"alreadyPublished":false`) ||
		!strings.Contains(result.Content, `"fileId":"`+testFileID+`"`) {
		t.Fatalf("publish Tool result=%s", result.Content)
	}

	replay := call
	replay.ID = "publish-2"
	replayed, failure, err := runtime.executeWorkspaceToolCall(context.Background(), replay)
	if err != nil || failure != "" || replayed.IsError || len(publisher.inputs) != 1 ||
		!strings.Contains(replayed.Content, `"alreadyPublished":true`) {
		t.Fatalf("replay=%#v failure=%q err=%v inputs=%d", replayed, failure, err, len(publisher.inputs))
	}
	attachments := runtime.publishedAttachmentInputs()
	if len(attachments) != 1 || attachments[0] != (AttachmentInput{
		Source: "server", FileID: testFileID, Purpose: "output",
	}) {
		t.Fatalf("attachments=%#v", attachments)
	}
}

func TestPublishFileToolRejectsUnsafeEmptyAndOverBudgetArtifacts(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "empty.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "large.bin"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "small.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("large.bin", filepath.Join(workspace, "alias.bin")); err != nil {
		t.Fatal(err)
	}
	runtime := newPublishFileTestRuntime(t, workspace, &fakeWorkspaceArtifactPublisher{}, 4)
	tests := []struct {
		name      string
		arguments string
		failure   string
	}{
		{name: "absolute", arguments: `{"path":"/etc/passwd","displayName":null,"contentType":null}`, failure: "arguments_invalid"},
		{name: "traversal", arguments: `{"path":"../large.bin","displayName":null,"contentType":null}`, failure: "arguments_invalid"},
		{name: "directory", arguments: `{"path":"folder","displayName":null,"contentType":null}`, failure: "path_invalid"},
		{name: "symlink", arguments: `{"path":"alias.bin","displayName":null,"contentType":null}`, failure: "path_invalid"},
		{name: "empty", arguments: `{"path":"empty.txt","displayName":null,"contentType":null}`, failure: "empty_file"},
		{name: "limit plus one", arguments: `{"path":"large.bin","displayName":null,"contentType":null}`, failure: "file_too_large"},
		{name: "bad display name", arguments: `{"path":"small.txt","displayName":"../x","contentType":null}`, failure: "arguments_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, failure, err := runtime.executeWorkspaceToolCall(context.Background(), ProviderToolCall{
				ID: test.name, Name: localPublishFileToolName, Arguments: test.arguments,
			})
			if err != nil || !result.IsError || failure != test.failure {
				t.Fatalf("result=%#v failure=%q err=%v", result, failure, err)
			}
		})
	}
}

func TestPublishFileToolEnforcesTurnCountAndTotalByteLimits(t *testing.T) {
	workspace := t.TempDir()
	for index := 0; index < maxPublishedArtifactsPerTurn+1; index++ {
		if err := os.WriteFile(
			filepath.Join(workspace, fmt.Sprintf("%d.txt", index)), []byte{'x'}, 0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	publisher := &fakeWorkspaceArtifactPublisher{}
	runtime := newPublishFileTestRuntime(t, workspace, publisher, maxPublishedArtifactsPerTurn+1)
	for index := 0; index < maxPublishedArtifactsPerTurn; index++ {
		result, failure, err := runtime.executeWorkspaceToolCall(context.Background(), ProviderToolCall{
			ID: fmt.Sprintf("call-%d", index), Name: localPublishFileToolName,
			Arguments: fmt.Sprintf(`{"path":"%d.txt","displayName":null,"contentType":null}`, index),
		})
		if err != nil || failure != "" || result.IsError {
			t.Fatalf("index=%d result=%#v failure=%q err=%v", index, result, failure, err)
		}
	}
	result, failure, err := runtime.executeWorkspaceToolCall(context.Background(), ProviderToolCall{
		ID: "count-over", Name: localPublishFileToolName,
		Arguments: fmt.Sprintf(`{"path":"%d.txt","displayName":null,"contentType":null}`, maxPublishedArtifactsPerTurn),
	})
	if err != nil || !result.IsError || failure != "artifact_count_exhausted" {
		t.Fatalf("count result=%#v failure=%q err=%v", result, failure, err)
	}

	if err := os.WriteFile(filepath.Join(workspace, "first.bin"), []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "second.bin"), []byte("456"), 0o600); err != nil {
		t.Fatal(err)
	}
	totalRuntime := newPublishFileTestRuntime(t, workspace, &fakeWorkspaceArtifactPublisher{}, 5)
	for index, name := range []string{"first.bin", "second.bin"} {
		result, failure, err := totalRuntime.executeWorkspaceToolCall(context.Background(), ProviderToolCall{
			ID: name, Name: localPublishFileToolName,
			Arguments: fmt.Sprintf(`{"path":%q,"displayName":null,"contentType":null}`, name),
		})
		if index == 0 && (err != nil || failure != "" || result.IsError) {
			t.Fatalf("first result=%#v failure=%q err=%v", result, failure, err)
		}
		if index == 1 && (err != nil || !result.IsError || failure != "artifact_bytes_exhausted") {
			t.Fatalf("second result=%#v failure=%q err=%v", result, failure, err)
		}
	}
}

func TestAssistantOnlyAttachmentPurposeAndArtifactCleanup(t *testing.T) {
	if _, err := normalizeAttachmentInputs([]AttachmentInput{{
		FileID: testFileID, Purpose: "output",
	}}, false); err == nil {
		t.Fatal("user output attachment purpose was accepted")
	}
	attachments, err := normalizeAttachmentInputs([]AttachmentInput{{
		FileID: testFileID, Purpose: "output",
	}}, true)
	if err != nil || len(attachments) != 1 || attachments[0].Purpose != "output" {
		t.Fatalf("assistant attachments=%#v err=%v", attachments, err)
	}

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "result.txt"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	publisher := &fakeWorkspaceArtifactPublisher{}
	runtime := newPublishFileTestRuntime(t, workspace, publisher, 1024)
	result, failure, err := runtime.executeWorkspaceToolCall(context.Background(), ProviderToolCall{
		ID: "publish", Name: localPublishFileToolName,
		Arguments: `{"path":"result.txt","displayName":null,"contentType":null}`,
	})
	if err != nil || failure != "" || result.IsError {
		t.Fatalf("publish result=%#v failure=%q err=%v", result, failure, err)
	}
	runtime.discardUnlinkedPublishedArtifacts(context.Background(), []Attachment{{
		FileID: testFileID, Purpose: "output",
	}})
	if len(publisher.deleted) != 0 || len(runtime.publishedAttachmentInputs()) != 1 {
		t.Fatalf("linked artifact was discarded: deleted=%#v", publisher.deleted)
	}
	runtime.discardUnlinkedPublishedArtifacts(context.Background(), nil)
	if len(publisher.deleted) != 1 || publisher.deleted[0] != testFileID ||
		len(runtime.publishedAttachmentInputs()) != 0 {
		t.Fatalf("deleted=%#v attachments=%#v", publisher.deleted, runtime.publishedAttachmentInputs())
	}
}

func TestHandlerPersistsAndReplaysPublishedArtifactOnAssistantMessage(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: time.Second, MaxOutput: 4096,
		MaxCalls: 8, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &fakeWorkspaceArtifactPublisher{}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "capability-probe", Name: toolCapabilityProbeToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "write", Name: localFileWriteToolName,
			Arguments: `{"path":"result.csv","content":"name,value\ngold,1\n","expectedVersion":"absent"}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "read", Name: localFileReadToolName,
			Arguments: `{"path":"result.csv","offset":null,"limit":null}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "publish", Name: localPublishFileToolName,
			Arguments: `{"path":"result.csv","displayName":"gold.csv","contentType":"text/csv"}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "文件已生成，可在附件中下载。"}},
	}, delays: []time.Duration{0, 0, 0, 1100 * time.Millisecond}}
	repo := newFakeRepository()
	repo.conversations = append(
		repo.conversations,
		fakeConversation(testConversationID, "Artifacts", 1),
	)
	repo.messages[testConversationID] = []Message{
		fakeMessage(testMessageID, testConversationID, 0, "user", "生成 CSV"),
	}
	cache := &capabilityMemoryCache{stored: make(chan capabilityStoredValue, 1)}
	resolver := &fakeRuntimeProviderResolver{
		provider:                 provider,
		toolCapabilityPolicy:     toolCapabilityPolicyAuto,
		toolCapabilityConfigHash: strings.Repeat("9", 64),
	}
	handler := NewHandler(
		NewService(repo),
		WithRuntimeProviderResolver(resolver),
		WithToolCapabilityCache(cache),
		WithLocalSkillRuntime(nil, executor),
		WithWorkspaceArtifactPublisher(publisher, 1024),
	)
	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","modelRef":{"providerId":"mock","modelId":"tool-model"},"provider":{"source":"server-default"},"idempotencyKey":"artifact-stream"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if !strings.Contains(recorder.Body.String(), `"purpose":"output"`) ||
		!strings.Contains(recorder.Body.String(), "event: message.completed") {
		t.Fatalf("stream=%s", recorder.Body.String())
	}
	if len(provider.inputs) != 5 {
		t.Fatalf("provider inputs=%#v", provider.inputs)
	}
	if len(publisher.inputs) != 1 || publisher.inputs[0].FileName != "gold.csv" {
		t.Fatalf("artifact was not published after legacy RunTimeout: %#v", publisher.inputs)
	}
	wantTools := map[string]bool{
		localFileReadToolName:    false,
		localFileWriteToolName:   false,
		localFileEditToolName:    false,
		localFileSearchToolName:  false,
		localTerminalToolName:    false,
		localPublishFileToolName: false,
	}
	for _, definition := range provider.inputs[1].Tools {
		if _, wanted := wantTools[definition.Function.Name]; wanted {
			wantTools[definition.Function.Name] = true
		}
	}
	for name, found := range wantTools {
		if !found {
			t.Fatalf("first Agent round omitted %q: %#v", name, provider.inputs[1].Tools)
		}
	}
	written, err := os.ReadFile(filepath.Join(workspace, "result.csv"))
	if err != nil || string(written) != "name,value\ngold,1\n" {
		t.Fatalf("workspace result=%q err=%v", written, err)
	}
	if len(provider.inputs[4].Continuation) != 3 ||
		!strings.Contains(provider.inputs[4].Continuation[1].Results[0].Content, `"content":"name,value\ngold,1\n"`) {
		t.Fatalf("read-back continuation=%#v", provider.inputs[4].Continuation)
	}
	messages := repo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "completed" ||
		len(messages[1].Attachments) != 1 || messages[1].Attachments[0].Purpose != "output" ||
		messages[1].Attachments[0].FileID != testFileID ||
		messages[1].Metadata["requestedToolMode"] != "agent" ||
		messages[1].Metadata["toolMode"] != "agent" {
		t.Fatalf("messages=%#v", messages)
	}
	select {
	case stored := <-cache.stored:
		if stored.status != ToolCapabilitySupported || stored.category != "structured_tool_call" {
			t.Fatalf("stored capability=%#v", stored)
		}
	case <-time.After(time.Second):
		t.Fatal("supported capability was not stored")
	}
	reloaded := performRequest(
		handler,
		http.MethodGet,
		conversationsPath+"/"+testConversationID+"/messages",
		"",
	)
	assertStatus(t, reloaded, http.StatusOK)
	if !strings.Contains(reloaded.Body.String(), `"purpose":"output"`) ||
		!strings.Contains(reloaded.Body.String(), `"fileId":"`+testFileID+`"`) {
		t.Fatalf("reload=%s", reloaded.Body.String())
	}
}

func TestHandlerPersistsCompletionDrivenBlockedOutcome(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "same.txt"), []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: time.Second, MaxOutput: 4096,
		MaxCalls: 1, MaxRounds: 1, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "read-1", Name: localFileReadToolName,
			Arguments: `{"path":"same.txt","offset":null,"limit":null}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "read-2", Name: localFileReadToolName,
			Arguments: `{"path":"same.txt","offset":null,"limit":null}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "read-3", Name: localFileReadToolName,
			Arguments: `{"path":"same.txt","offset":null,"limit":null}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "任务未完成：读取结果没有变化。"}},
	}}
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "Blocked", 1))
	repo.messages[testConversationID] = []Message{
		fakeMessage(testMessageID, testConversationID, 0, "user", "重复读取"),
	}
	handler := NewHandler(
		NewService(repo),
		WithProvider(provider),
		WithLocalSkillRuntime(nil, executor),
	)
	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","modelRef":{"providerId":"mock","modelId":"tool-model"},"idempotencyKey":"blocked-outcome"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if !strings.Contains(recorder.Body.String(), "event: message.completed") || len(provider.inputs) != 4 {
		t.Fatalf("stream=%s inputs=%#v", recorder.Body.String(), provider.inputs)
	}
	messages := repo.messages[testConversationID]
	if len(messages) != 2 || messages[1].Status != "completed" ||
		messages[1].Metadata["agentOutcome"] != chatAgentOutcomeBlocked ||
		messages[1].Metadata["agentOutcomeReason"] != chatAgentBlockRepeatedToolOutcome {
		t.Fatalf("messages=%#v", messages)
	}
}

func TestHandlerDeletesPublishedArtifactWhenAssistantFinalizeFails(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "result.txt"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 8, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &fakeWorkspaceArtifactPublisher{}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "publish", Name: localPublishFileToolName,
			Arguments: `{"path":"result.txt","displayName":null,"contentType":null}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "done"}},
	}}
	baseRepo := newFakeRepository()
	baseRepo.conversations = append(
		baseRepo.conversations,
		fakeConversation(testConversationID, "Finalize failure", 1),
	)
	baseRepo.messages[testConversationID] = []Message{
		fakeMessage(testMessageID, testConversationID, 0, "user", "生成文件"),
	}
	handler := NewHandler(
		NewService(&finalizeFailingRepository{fakeRepository: baseRepo}),
		WithProvider(provider),
		WithLocalSkillRuntime(nil, executor),
		WithWorkspaceArtifactPublisher(publisher, 1024),
	)
	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","modelRef":{"providerId":"mock","modelId":"tool-model"},"idempotencyKey":"artifact-finalize-failure"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if len(publisher.deleted) != 1 || publisher.deleted[0] != testFileID {
		t.Fatalf("deleted=%#v stream=%s", publisher.deleted, recorder.Body.String())
	}
}

func newPublishFileTestRuntime(
	t *testing.T,
	workspace string,
	publisher WorkspaceArtifactPublisher,
	maxBytes int,
) *localSkillToolRuntime {
	t.Helper()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 3 * time.Second, MaxOutput: 4096,
		MaxCalls: 32, MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindJobScope(DevUserID, testConversationID)
	runtime.bindArtifactPublisher(publisher, int64(maxBytes))
	return runtime
}

func findLocalToolDefinition(
	t *testing.T,
	runtime *localSkillToolRuntime,
	name string,
) ToolDefinition {
	t.Helper()
	for _, definition := range runtime.definitions() {
		if definition.Function.Name == name {
			return definition
		}
	}
	t.Fatalf("Tool definition %q not found", name)
	return ToolDefinition{}
}

type fakeWorkspaceArtifactPublisher struct {
	inputs  []WorkspaceArtifactPublishInput
	deleted []string
	fail    error
}

type finalizeFailingRepository struct {
	*fakeRepository
}

func (repository *finalizeFailingRepository) FinalizeAssistantMessage(
	context.Context,
	string,
	string,
	FinalizeAssistantMessageInput,
) (Message, error) {
	return Message{}, errors.New("fixture finalize failure")
}

func (publisher *fakeWorkspaceArtifactPublisher) PublishWorkspaceArtifact(
	_ context.Context,
	input WorkspaceArtifactPublishInput,
) (WorkspaceArtifact, error) {
	if publisher.fail != nil {
		return WorkspaceArtifact{}, publisher.fail
	}
	input.Body = append([]byte(nil), input.Body...)
	publisher.inputs = append(publisher.inputs, input)
	digest := sha256.Sum256(input.Body)
	fileID := testFileID
	if len(publisher.inputs) > 1 {
		fileID = fmt.Sprintf("55555555-5555-4555-8555-%012d", len(publisher.inputs))
	}
	return WorkspaceArtifact{
		FileID: fileID, FileName: input.FileName, MimeType: input.MimeType,
		Size: int64(len(input.Body)), SHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func (publisher *fakeWorkspaceArtifactPublisher) DeleteWorkspaceArtifact(
	_ context.Context,
	fileID string,
) error {
	publisher.deleted = append(publisher.deleted, fileID)
	return publisher.fail
}

var _ WorkspaceArtifactPublisher = (*fakeWorkspaceArtifactPublisher)(nil)
