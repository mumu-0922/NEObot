package chat

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

const (
	goalTestTurnID    = "77777777-7777-4777-8777-777777777777"
	goalTestMessageID = "88888888-8888-4888-8888-888888888888"
	goalTestRunID     = "99999999-9999-4999-8999-999999999999"
)

func TestChatAgentGoalToolsAreStrictAndDefaultRegistryHasNoSubagent(t *testing.T) {
	repository := newGoalTestRepository(t)
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	registry := newChatToolRegistry(externalWebToolLoopInput{Goals: runtime})
	definitions := registry.definitions(1)
	if len(definitions) != 3 {
		t.Fatalf("Goal definitions = %d", len(definitions))
	}
	for _, definition := range definitions {
		if definition.Function.Name == "verify_completion" {
			t.Fatal("verify_completion leaked into new Agent Tool definitions")
		}
		if !definition.Function.Strict {
			t.Fatalf("Goal Tool %s is not strict", definition.Function.Name)
		}
		if strings.Contains(definition.Function.Name, "agent") &&
			strings.Contains(definition.Function.Name, "delegate") {
			t.Fatalf("Subagent Tool leaked: %s", definition.Function.Name)
		}
	}
	if registration, ok := registry.lookup(chatAgentUpdateGoalToolName); !ok ||
		registration.RiskClass != chatToolRiskWrite ||
		registration.Backend != chatToolBackendGoal {
		t.Fatalf("update_goal registration = %#v / %v", registration, ok)
	}
}

func TestChatAgentNaturallyCompletesAfterForegroundBash(t *testing.T) {
	repository := newGoalTestRepository(t)
	goalRuntime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	workspace := t.TempDir()
	if output, err := exec.Command("git", "-C", workspace, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("initialize fixture Git repository: %v: %s", err, output)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	localRuntime := newLocalSkillToolRuntime(executor, nil)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "status", Name: localTerminalToolName,
			Arguments: `{"command":"pwd && git status --short","skill":null,"workingDir":null,"timeoutSeconds":1,"runInBackground":false}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "当前目录是干净的 Git 工作区。"}},
	}}

	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "在当前项目执行 pwd 和 git status --short，然后解释结果，不要修改文件。",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		LocalSkills: localRuntime, Goals: goalRuntime,
	})
	var content strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "当前目录是干净的 Git 工作区。" || len(provider.inputs) != 2 {
		t.Fatalf("content=%q rounds=%d", content.String(), len(provider.inputs))
	}
	result := provider.inputs[1].Continuation[0].Results
	if len(result) != 1 || result[0].IsError ||
		!strings.Contains(result[0].Content, `"stdout":"$NEO_CHAT_WORKSPACE\n"`) ||
		strings.Contains(result[0].Content, "evidenceToolCallId") {
		t.Fatalf("Terminal result=%#v", result)
	}
	if provider.inputs[1].Continuation[0].FollowupPrompt != "" {
		t.Fatalf("natural continuation injected a follow-up prompt: %#v", provider.inputs[1].Continuation)
	}
}

func TestChatAgentGoalLoopAutomaticallyContinuesAndWrapsUp(t *testing.T) {
	repository := newGoalTestRepository(t)
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "goal-create", Name: chatAgentCreateGoalToolName,
				Arguments: `{"objective":"finish the fixture","maxGoalRounds":3}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "intermediate narration"}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "goal-complete", Name: chatAgentUpdateGoalToolName,
				Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"complete","objective":null,"maxGoalRounds":null,"blockedReason":null}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "final answer"}},
	}}

	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "finish the fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		Goals: runtime,
	})
	var content strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "final answer" {
		t.Fatalf("visible content = %q", content.String())
	}
	if len(provider.inputs) != 4 {
		t.Fatalf("provider rounds = %d", len(provider.inputs))
	}
	goalRoundContinuation := provider.inputs[2].Continuation
	if len(goalRoundContinuation) != 2 ||
		!strings.Contains(goalRoundContinuation[1].FollowupPrompt, "<goal_round>") ||
		!strings.Contains(goalRoundContinuation[1].FollowupPrompt, "Round: 1/3") {
		t.Fatalf("Goal continuation = %#v", goalRoundContinuation)
	}
	if len(provider.inputs[3].Tools) != 0 {
		t.Fatalf("wrap-up exposed Tools = %#v", provider.inputs[3].Tools)
	}
	if repository.goal == nil || repository.goal.Phase != ChatAgentGoalComplete ||
		repository.goal.RoundsStarted != 1 || repository.goal.Revision != 2 {
		t.Fatalf("stored Goal = %#v", repository.goal)
	}
	if repository.eventTypesString() != "goal.changed,goal.round.started,goal.changed" {
		t.Fatalf("Goal events = %s", repository.eventTypesString())
	}
}

func TestChatAgentGoalWrapupKeepsToolsDisabledAfterHallucinatedCall(t *testing.T) {
	repository := newGoalTestRepository(t)
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "goal-create", Name: chatAgentCreateGoalToolName,
				Arguments: `{"objective":"finish the fixture","maxGoalRounds":3}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "intermediate narration"}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "goal-complete", Name: chatAgentUpdateGoalToolName,
				Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"complete","objective":null,"maxGoalRounds":null,"blockedReason":null}`,
			},
		}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "hallucinated-goal-read", Name: chatAgentGetGoalToolName,
				Arguments: `{}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "final answer"}},
	}}

	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "finish the fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		Goals: runtime,
	})
	var content strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "final answer" || len(provider.inputs) != 5 {
		t.Fatalf("content=%q rounds=%d", content.String(), len(provider.inputs))
	}
	for _, round := range []int{3, 4} {
		if len(provider.inputs[round].Tools) != 0 {
			t.Fatalf("wrap-up round %d exposed Tools = %#v", round+1, provider.inputs[round].Tools)
		}
	}
	hallucinated := provider.inputs[4].Continuation[3]
	if len(hallucinated.Results) != 1 || !hallucinated.Results[0].IsError ||
		!strings.Contains(hallucinated.Results[0].Content, "goal_concluded") {
		t.Fatalf("hallucinated wrap-up result = %#v", hallucinated.Results)
	}
	if repository.eventTypesString() != "goal.changed,goal.round.started,goal.changed" {
		t.Fatalf("hallucinated Tool mutated Goal events = %s", repository.eventTypesString())
	}
}

const goalTestGoalID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

type goalTestRepository struct {
	*fakeRepository
	goal       *ChatAgentGoal
	goalEvents []string
}

func newGoalTestRepository(t *testing.T) *goalTestRepository {
	t.Helper()
	repository := &goalTestRepository{fakeRepository: newFakeRepository()}
	_, err := repository.StartChatAgentTurn(context.Background(), StartChatAgentTurnInput{
		TurnID: goalTestTurnID, EventID: "66666666-6666-4666-8666-666666666666",
		ConversationID: testConversationID, MessageID: goalTestMessageID,
		RunID: goalTestRunID, OccurredAt: testNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func (repository *goalTestRepository) GetChatAgentGoal(
	context.Context,
	string,
) (*ChatAgentGoal, error) {
	return cloneChatAgentGoalPointer(repository.goal), nil
}

func (repository *goalTestRepository) CreateChatAgentGoal(
	_ context.Context,
	input CreateChatAgentGoalInput,
) (ChatAgentGoal, error) {
	if repository.goal != nil && repository.goal.Phase != ChatAgentGoalComplete {
		return ChatAgentGoal{}, ChatAgentGoalError{Code: "CHAT_AGENT_GOAL_ALREADY_EXISTS"}
	}
	goalID := input.GoalID
	// Stable test identity keeps scripted Provider arguments deterministic.
	if goalID != "" {
		goalID = goalTestGoalID
	}
	goal := ChatAgentGoal{
		ID: goalID, ConversationID: testConversationID,
		Objective: strings.TrimSpace(input.Objective), Phase: ChatAgentGoalActive,
		Revision: 1, MaxGoalRounds: input.MaxGoalRounds,
		CreatedAt: input.OccurredAt, UpdatedAt: input.OccurredAt,
	}
	repository.goal = &goal
	repository.goalEvents = append(repository.goalEvents, ChatAgentEventGoalChanged)
	return goal, nil
}

func (repository *goalTestRepository) ChangeChatAgentGoal(
	_ context.Context,
	input ChangeChatAgentGoalInput,
) (ChatAgentGoal, error) {
	if repository.goal == nil {
		return ChatAgentGoal{}, ChatAgentGoalError{Code: "CHAT_AGENT_GOAL_NOT_FOUND"}
	}
	if repository.goal.ID != input.GoalID || repository.goal.Revision != input.ExpectedRevision {
		return ChatAgentGoal{}, ChatAgentGoalError{Code: "CHAT_AGENT_GOAL_STALE_REVISION"}
	}
	goal := *repository.goal
	goal.Revision++
	goal.UpdatedAt = input.OccurredAt
	switch input.Action {
	case ChatAgentGoalActionEdit:
		if input.Objective != "" {
			goal.Objective = input.Objective
		}
		if input.MaxGoalRounds != nil {
			goal.MaxGoalRounds = *input.MaxGoalRounds
		}
	case ChatAgentGoalActionPause:
		goal.Phase = ChatAgentGoalPaused
	case ChatAgentGoalActionResume:
		goal.Phase = ChatAgentGoalActive
		goal.BlockedReason = nil
	case ChatAgentGoalActionComplete:
		goal.Phase = ChatAgentGoalComplete
		goal.BlockedReason = nil
	case ChatAgentGoalActionBlocked:
		goal.Phase = ChatAgentGoalBlocked
		goal.BlockedReason = &ChatAgentGoalBlockReason{
			Code: "model-reported", Message: input.BlockedReason,
		}
	}
	repository.goal = &goal
	repository.goalEvents = append(repository.goalEvents, ChatAgentEventGoalChanged)
	return goal, nil
}

func (repository *goalTestRepository) CancelChatAgentGoal(
	_ context.Context,
	input CancelChatAgentGoalInput,
) (ChatAgentGoalRef, error) {
	if repository.goal == nil || repository.goal.ID != input.GoalID ||
		repository.goal.Revision != input.ExpectedRevision {
		return ChatAgentGoalRef{}, ChatAgentGoalError{Code: "CHAT_AGENT_GOAL_STALE_REVISION"}
	}
	ref := ChatAgentGoalRef{ID: repository.goal.ID, Revision: repository.goal.Revision + 1}
	repository.goal = nil
	repository.goalEvents = append(repository.goalEvents, ChatAgentEventGoalChanged)
	return ref, nil
}

func (repository *goalTestRepository) StartChatAgentGoalRound(
	_ context.Context,
	input StartChatAgentGoalRoundInput,
) (ChatAgentGoal, error) {
	if repository.goal == nil || repository.goal.ID != input.GoalID ||
		repository.goal.Revision != input.ExpectedRevision ||
		input.Round != repository.goal.RoundsStarted+1 {
		return ChatAgentGoal{}, ChatAgentGoalError{Code: "CHAT_AGENT_GOAL_ROUND_INVALID"}
	}
	repository.goal.RoundsStarted = input.Round
	repository.goal.UpdatedAt = input.OccurredAt
	repository.goalEvents = append(repository.goalEvents, ChatAgentEventGoalRoundStarted)
	return *cloneChatAgentGoalPointer(repository.goal), nil
}

func (repository *goalTestRepository) eventTypesString() string {
	return strings.Join(repository.goalEvents, ",")
}

func TestProviderContinuationRendersGoalFollowupAfterAssistant(t *testing.T) {
	exchange := ProviderToolExchange{
		AssistantContent: "progress", FollowupPrompt: "<goal_round>next</goal_round>",
	}
	openAI := appendOpenAICompatibleContinuation(nil, []ProviderToolExchange{exchange})
	if len(openAI) != 2 || openAI[0].Role != "assistant" || openAI[1].Role != "user" ||
		openAI[1].Content != exchange.FollowupPrompt {
		t.Fatalf("OpenAI Goal continuation = %#v", openAI)
	}
	anthropic, err := appendAnthropicContinuation(nil, []ProviderToolExchange{exchange})
	if err != nil {
		t.Fatal(err)
	}
	if len(anthropic) != 2 || anthropic[0].Role != "assistant" ||
		anthropic[1].Role != "user" || anthropic[1].Content != exchange.FollowupPrompt {
		t.Fatalf("Anthropic Goal continuation = %#v", anthropic)
	}
}

func TestChatAgentGoalBlockingRequiresThreeAutomaticRounds(t *testing.T) {
	repository := newGoalTestRepository(t)
	repository.goal = &ChatAgentGoal{
		ID: goalTestGoalID, ConversationID: testConversationID,
		Objective: "wait for prerequisite", Phase: ChatAgentGoalActive,
		Revision: 1, RoundsStarted: 2, MaxGoalRounds: 8,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	runtime.current = cloneChatAgentGoalPointer(repository.goal)
	runtime.currentRound = 2
	runtime.directHuman = false
	result, _, failure, err := runtime.executeCall(context.Background(), ProviderToolCall{
		ID: "blocked", Name: chatAgentUpdateGoalToolName,
		Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"blocked","objective":null,"maxGoalRounds":null,"blockedReason":"dependency unavailable"}`,
	})
	if err != nil || failure != "blocked_round_threshold" || !result.IsError {
		t.Fatalf("round-two block result=%#v failure=%q err=%v", result, failure, err)
	}
	runtime.currentRound = 3
	repository.goal.RoundsStarted = 3
	result, concludes, failure, err := runtime.executeCall(context.Background(), ProviderToolCall{
		ID: "blocked-3", Name: chatAgentUpdateGoalToolName,
		Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"blocked","objective":null,"maxGoalRounds":null,"blockedReason":"dependency unavailable"}`,
	})
	if err != nil || failure != "" || !concludes || result.IsError ||
		repository.goal.Phase != ChatAgentGoalBlocked {
		t.Fatalf("round-three block result=%#v concludes=%v failure=%q err=%v goal=%#v",
			result, concludes, failure, err, repository.goal)
	}
}
