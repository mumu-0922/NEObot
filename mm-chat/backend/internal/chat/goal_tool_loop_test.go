package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

const (
	goalTestTurnID    = "77777777-7777-4777-8777-777777777777"
	goalTestMessageID = "88888888-8888-4888-8888-888888888888"
	goalTestRunID     = "99999999-9999-4999-8999-999999999999"
)

func TestChatCompletionPolicyRequiresExplicitLaterEvidence(t *testing.T) {
	registry := &chatToolRegistry{
		ordered: make([]chatToolRegistration, 0, 2),
		byName:  map[string]chatToolRegistration{}, colliding: map[string]struct{}{},
	}
	registry.register(chatToolRegistration{
		Name: "terminal", Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskExecute, ProjectForModel: identityChatToolResult,
	})
	registry.register(chatToolRegistration{
		Name: "read_file", Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskRead, ProjectForModel: identityChatToolResult,
	})
	policy := newChatCompletionPolicy()
	policy.observe(registry, []ProviderToolCall{{ID: "write-1", Name: "terminal"}},
		[]ProviderToolResult{{CallID: "write-1", Name: "terminal"}})
	if !policy.requiresVerification() {
		t.Fatal("successful execute must require verification")
	}
	if _, err := policy.verify("write-1", "command said success"); err != nil {
		t.Fatalf("latest successful execute may serve as explicit check evidence: %v", err)
	}
	if policy.requiresVerification() {
		t.Fatal("explicit evidence should satisfy the current mutation")
	}
	policy.observe(registry, []ProviderToolCall{{ID: "write-2", Name: "terminal"}},
		[]ProviderToolResult{{CallID: "write-2", Name: "terminal"}})
	policy.observe(registry, []ProviderToolCall{{ID: "read-2", Name: "read_file"}},
		[]ProviderToolResult{{CallID: "read-2", Name: "read_file"}})
	if _, err := policy.verify("write-1", "stale proof"); err == nil ||
		err.Error() != "verification_evidence_invalid" {
		t.Fatalf("stale evidence error = %v", err)
	}
	if _, err := policy.verify("read-2", "read-back matched expected content"); err != nil {
		t.Fatalf("later read evidence: %v", err)
	}
}

func TestChatCompletionPolicyRejectsFileMutationResultAsItsOwnEvidence(t *testing.T) {
	registry := &chatToolRegistry{
		ordered: make([]chatToolRegistration, 0, 2),
		byName:  map[string]chatToolRegistration{}, colliding: map[string]struct{}{},
	}
	registry.register(chatToolRegistration{
		Name: localFileWriteToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskWrite, ProjectForModel: identityChatToolResult,
		MutationResultNeedsFollowup: true,
	})
	registry.register(chatToolRegistration{
		Name: localFileReadToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskRead, ProjectForModel: identityChatToolResult,
	})
	policy := newChatCompletionPolicy()
	policy.observe(registry, []ProviderToolCall{{ID: "write", Name: localFileWriteToolName}},
		[]ProviderToolResult{{CallID: "write", Name: localFileWriteToolName}})
	if _, err := policy.verify("write", "write returned success"); err == nil ||
		err.Error() != "verification_evidence_invalid" {
		t.Fatalf("file mutation self-evidence error=%v", err)
	}
	policy.observe(registry, []ProviderToolCall{{ID: "read", Name: localFileReadToolName}},
		[]ProviderToolResult{{CallID: "read", Name: localFileReadToolName}})
	if _, err := policy.verify("read", "read-back matched"); err != nil {
		t.Fatalf("read-back evidence error=%v", err)
	}
}

func TestChatAgentGoalToolsAreStrictAndDefaultRegistryHasNoSubagent(t *testing.T) {
	repository := newGoalTestRepository(t)
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	registry := newChatToolRegistry(externalWebToolLoopInput{Goals: runtime})
	definitions := registry.definitions(1)
	if len(definitions) != 4 {
		t.Fatalf("Goal definitions = %d", len(definitions))
	}
	for _, definition := range definitions {
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

func TestChatAgentCompletionGateContinuesUntilExplicitToolEvidence(t *testing.T) {
	repository := newGoalTestRepository(t)
	goalRuntime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	workspace := t.TempDir()
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
	localRuntime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Description: "fixture",
	}})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "mutation", Name: localTerminalToolName,
			Arguments: `{"command":"printf changed > artifact.txt","skill":null,"workingDir":null,"timeoutSeconds":1}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "unverified narration"}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "evidence", Name: localTerminalToolName,
			Arguments: `{"command":"test -s artifact.txt","skill":null,"workingDir":null,"timeoutSeconds":1}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "verification", Name: chatAgentVerifyCompletionToolName,
			Arguments: `{"evidenceToolCallId":"evidence","summary":"artifact exists and is non-empty"}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "verified final answer"}},
	}}

	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "produce and verify the artifact",
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
	if content.String() != "verified final answer" || len(provider.inputs) != 5 {
		t.Fatalf("content=%q rounds=%d", content.String(), len(provider.inputs))
	}
	if _, err := os.Stat(filepath.Join(workspace, "artifact.txt")); err != nil {
		t.Fatal(err)
	}
	followup := provider.inputs[2].Continuation[1].FollowupPrompt
	if !strings.Contains(followup, "<completion_verification_required>") ||
		!strings.Contains(followup, `"mutation"`) {
		t.Fatalf("verification follow-up = %q", followup)
	}
	verificationResult := provider.inputs[4].Continuation[3].Results
	if len(verificationResult) != 1 || verificationResult[0].IsError ||
		!strings.Contains(verificationResult[0].Content, `"verified":true`) {
		t.Fatalf("verification result = %#v", verificationResult)
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
		{{Type: ProviderEventDelta, Delta: "verified final answer"}},
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
	if content.String() != "verified final answer" {
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

func TestChatAgentGoalCompleteRejectsUnverifiedMutation(t *testing.T) {
	repository := newGoalTestRepository(t)
	runtime := newChatAgentGoalToolRuntime(
		NewService(repository), goalTestTurnID, testConversationID,
	)
	policy := newChatCompletionPolicy()
	runtime.bindCompletionPolicy(policy)
	registry := &chatToolRegistry{
		ordered: make([]chatToolRegistration, 0, 1),
		byName:  map[string]chatToolRegistration{}, colliding: map[string]struct{}{},
	}
	registry.register(chatToolRegistration{
		Name: "terminal", Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskExecute, ProjectForModel: identityChatToolResult,
	})
	policy.observe(registry, []ProviderToolCall{{ID: "mutation", Name: "terminal"}},
		[]ProviderToolResult{{CallID: "mutation", Name: "terminal"}})
	repository.goal = &ChatAgentGoal{
		ID: goalTestGoalID, ConversationID: testConversationID,
		Objective: "prove it", Phase: ChatAgentGoalActive, Revision: 1,
		MaxGoalRounds: 4, CreatedAt: testNow(), UpdatedAt: testNow(),
	}
	runtime.current = cloneChatAgentGoalPointer(repository.goal)
	runtime.armed = true
	result, concludes, failure, fatal := runtime.executeCall(context.Background(), ProviderToolCall{
		ID: "complete", Name: chatAgentUpdateGoalToolName,
		Arguments: `{"goalId":"` + goalTestGoalID + `","revision":1,"action":"complete","objective":null,"maxGoalRounds":null,"blockedReason":null}`,
	})
	if fatal != nil || concludes || failure != "verification_required" || !result.IsError {
		t.Fatalf("complete result=%#v concludes=%v failure=%q fatal=%v", result, concludes, failure, fatal)
	}
	if repository.goal.Phase != ChatAgentGoalActive || repository.goal.Revision != 1 {
		t.Fatalf("unverified completion mutated Goal = %#v", repository.goal)
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
