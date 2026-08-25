package chat

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type chatAgentGoalToolRuntime struct {
	service        *Service
	turnID         string
	conversationID string
	current        *ChatAgentGoal
	armed          bool
	directHuman    bool
	currentRound   int
	forceNoTools   bool
	calls          int
}

type chatGoalBatchExecution struct {
	Results       map[int]ProviderToolResult
	ConcludesTurn bool
}

type chatAgentRunFailure struct {
	code string
	err  error
}

func (failure *chatAgentRunFailure) Error() string {
	if failure == nil || failure.err == nil {
		return "chat Agent run failed"
	}
	return failure.err.Error()
}

func (failure *chatAgentRunFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.err
}

func newChatAgentGoalToolRuntime(
	service *Service,
	turnID string,
	conversationID string,
) *chatAgentGoalToolRuntime {
	if service == nil || !service.chatAgentGoalsAvailable() ||
		!isUUID(strings.TrimSpace(turnID)) || !isUUID(strings.TrimSpace(conversationID)) {
		return nil
	}
	return &chatAgentGoalToolRuntime{
		service: service, turnID: strings.TrimSpace(turnID),
		conversationID: strings.TrimSpace(conversationID), directHuman: true,
	}
}

func (runtime *chatAgentGoalToolRuntime) enabled() bool {
	return runtime != nil && runtime.service != nil && runtime.turnID != "" &&
		runtime.conversationID != ""
}

func (runtime *chatAgentGoalToolRuntime) automaticWorkActive() bool {
	return runtime.enabled() && runtime.armed && runtime.current != nil &&
		runtime.current.Phase == ChatAgentGoalActive
}

func (runtime *chatAgentGoalToolRuntime) consumeForceNoTools() bool {
	if runtime == nil || !runtime.forceNoTools {
		return false
	}
	runtime.forceNoTools = false
	return true
}

func (runtime *chatAgentGoalToolRuntime) beginNextRound(
	ctx context.Context,
) (string, bool, error) {
	if !runtime.automaticWorkActive() {
		return "", false, nil
	}
	goal := runtime.current
	if goal.RoundsStarted >= goal.MaxGoalRounds {
		blocked, err := runtime.changeGoal(ctx, ChangeChatAgentGoalInput{
			GoalID: goal.ID, ExpectedRevision: goal.Revision,
			Action: ChatAgentGoalActionBlocked,
			BlockedReason: fmt.Sprintf(
				"Goal reached its configured limit of %d automatic rounds.",
				goal.MaxGoalRounds,
			),
		})
		if err != nil {
			return "", false, err
		}
		runtime.current = &blocked
		runtime.armed = false
		runtime.directHuman = false
		runtime.forceNoTools = true
		return renderChatAgentGoalWrapup(blocked), true, nil
	}
	eventID, err := NewUUID()
	if err != nil {
		return "", false, err
	}
	round := goal.RoundsStarted + 1
	started, err := runtime.service.StartChatAgentGoalRound(
		ctx,
		StartChatAgentGoalRoundInput{
			TurnID: runtime.turnID, EventID: eventID, GoalID: goal.ID,
			ExpectedRevision: goal.Revision, Round: round, OccurredAt: time.Now(),
		},
	)
	if err != nil {
		return "", false, err
	}
	runtime.current = &started
	runtime.directHuman = false
	runtime.currentRound = round
	return renderChatAgentGoalRound(started), true, nil
}

func executeChatAgentGoalBatch(
	ctx context.Context,
	events chan<- ProviderEvent,
	runtime *chatAgentGoalToolRuntime,
	calls []ProviderToolCall,
	round int,
) (chatGoalBatchExecution, error) {
	execution := chatGoalBatchExecution{Results: make(map[int]ProviderToolResult)}
	if !runtime.enabled() {
		return execution, nil
	}
	for index, call := range calls {
		if !isChatAgentGoalToolName(call.Name) {
			continue
		}
		if execution.ConcludesTurn {
			execution.Results[index] = chatAgentGoalFailureResult(call, "goal_concluded")
			continue
		}
		runtime.calls++
		result, concludes, err := runtime.execute(ctx, events, call, round, runtime.calls)
		execution.Results[index] = result
		if err != nil {
			return execution, err
		}
		if concludes {
			execution.ConcludesTurn = true
		}
	}
	return execution, nil
}

func (runtime *chatAgentGoalToolRuntime) execute(
	ctx context.Context,
	events chan<- ProviderEvent,
	call ProviderToolCall,
	round int,
	callNumber int,
) (ProviderToolResult, bool, error) {
	name := strings.TrimSpace(call.Name)
	risk := "read"
	if name == chatAgentCreateGoalToolName || name == chatAgentUpdateGoalToolName {
		risk = "write"
	}
	toolExecution := ProviderToolExecutionEvent{
		ExecutionID: fmt.Sprintf("goal-%d-%d", round, callNumber),
		CallID:      call.ID, Name: name, Status: ProcessStepStatusRunning,
		CallStatus: "running", Round: round, Mode: "goal", Classification: risk,
		Presentation: goalProcessPresentation(call),
	}
	if !sendToolExecutionEvent(ctx, events, toolExecution) {
		return ProviderToolResult{}, false, context.Canceled
	}
	started := time.Now()
	result, concludes, failure, fatal := runtime.executeCall(ctx, call)
	toolExecution.DurationMillis = max(time.Since(started).Milliseconds(), 0)
	if fatal != nil {
		toolExecution.Status = ProcessStepStatusFailed
		toolExecution.CallStatus = "failed"
		toolExecution.FailureCategory = "persistence_failed"
		sendLocalSkillTerminalEvent(events, toolExecution)
		return result, false, &chatAgentRunFailure{
			code: "AGENT_GOAL_PERSISTENCE_FAILED", err: fatal,
		}
	}
	if failure != "" {
		toolExecution.Status = ProcessStepStatusFailed
		toolExecution.CallStatus = "failed"
		toolExecution.FailureCategory = failure
	} else {
		toolExecution.Status = ProcessStepStatusCompleted
		toolExecution.CallStatus = "succeeded"
	}
	if !sendToolExecutionEvent(ctx, events, toolExecution) {
		return ProviderToolResult{}, false, context.Canceled
	}
	return result, concludes && failure == "", nil
}

func (runtime *chatAgentGoalToolRuntime) executeCall(
	ctx context.Context,
	call ProviderToolCall,
) (ProviderToolResult, bool, string, error) {
	if strings.TrimSpace(call.FailureCategory) != "" {
		return chatAgentGoalFailureResult(call, "arguments_invalid"), false, "arguments_invalid", nil
	}
	switch strings.TrimSpace(call.Name) {
	case chatAgentGetGoalToolName:
		var arguments struct{}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return chatAgentGoalFailureResult(call, "arguments_invalid"), false, "arguments_invalid", nil
		}
		goal, err := runtime.service.GetChatAgentGoal(ctx, runtime.conversationID)
		if err != nil {
			return runtime.goalErrorResult(call, err)
		}
		if runtime.current != nil && (goal == nil || goal.ID != runtime.current.ID ||
			goal.Revision != runtime.current.Revision) {
			runtime.armed = false
		}
		runtime.current = cloneChatAgentGoalPointer(goal)
		return chatAgentGoalSuccessResult(call, runtime.current, runtime.armed), false, "", nil
	case chatAgentCreateGoalToolName:
		if !runtime.directHuman {
			return chatAgentGoalFailureResult(call, "human_authority_required"), false, "human_authority_required", nil
		}
		var arguments struct {
			Objective     string `json:"objective"`
			MaxGoalRounds *int   `json:"maxGoalRounds"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return chatAgentGoalFailureResult(call, "arguments_invalid"), false, "arguments_invalid", nil
		}
		maxRounds := defaultChatAgentMaxGoalRounds
		if arguments.MaxGoalRounds != nil {
			maxRounds = *arguments.MaxGoalRounds
		}
		goalID, eventID, err := newChatAgentGoalIDs()
		if err != nil {
			return ProviderToolResult{}, false, "", err
		}
		goal, err := runtime.service.CreateChatAgentGoal(ctx, CreateChatAgentGoalInput{
			TurnID: runtime.turnID, EventID: eventID, GoalID: goalID,
			Objective: arguments.Objective, MaxGoalRounds: maxRounds, OccurredAt: time.Now(),
		})
		if err != nil {
			return runtime.goalErrorResult(call, err)
		}
		runtime.current = &goal
		runtime.armed = true
		return chatAgentGoalSuccessResult(call, runtime.current, true), false, "", nil
	case chatAgentUpdateGoalToolName:
		var arguments struct {
			GoalID        string  `json:"goalId"`
			Revision      int64   `json:"revision"`
			Action        string  `json:"action"`
			Objective     *string `json:"objective"`
			MaxGoalRounds *int    `json:"maxGoalRounds"`
			BlockedReason *string `json:"blockedReason"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return chatAgentGoalFailureResult(call, "arguments_invalid"), false, "arguments_invalid", nil
		}
		return runtime.executeGoalUpdate(ctx, call, arguments)
	default:
		return chatAgentGoalFailureResult(call, "tool_not_available"), false, "tool_not_available", nil
	}
}

func (runtime *chatAgentGoalToolRuntime) executeGoalUpdate(
	ctx context.Context,
	call ProviderToolCall,
	arguments struct {
		GoalID        string  `json:"goalId"`
		Revision      int64   `json:"revision"`
		Action        string  `json:"action"`
		Objective     *string `json:"objective"`
		MaxGoalRounds *int    `json:"maxGoalRounds"`
		BlockedReason *string `json:"blockedReason"`
	},
) (ProviderToolResult, bool, string, error) {
	action := strings.TrimSpace(arguments.Action)
	if action == ChatAgentGoalActionEdit || action == ChatAgentGoalActionPause ||
		action == ChatAgentGoalActionResume || action == ChatAgentGoalActionCancel {
		if !runtime.directHuman {
			return chatAgentGoalFailureResult(call, "human_authority_required"), false, "human_authority_required", nil
		}
	}
	if action == ChatAgentGoalActionBlocked && !runtime.directHuman &&
		(runtime.current == nil || runtime.currentRound < chatAgentGoalBlockedAfterRounds) {
		return chatAgentGoalFailureResult(call, "blocked_round_threshold"), false, "blocked_round_threshold", nil
	}
	if !chatAgentGoalUpdateArgumentsValid(action, arguments.Objective,
		arguments.MaxGoalRounds, arguments.BlockedReason) {
		return chatAgentGoalFailureResult(call, "arguments_invalid"), false, "arguments_invalid", nil
	}
	eventID, err := NewUUID()
	if err != nil {
		return ProviderToolResult{}, false, "", err
	}
	if action == ChatAgentGoalActionCancel {
		ref, cancelErr := runtime.service.CancelChatAgentGoal(ctx, CancelChatAgentGoalInput{
			TurnID: runtime.turnID, EventID: eventID,
			GoalID:           strings.TrimSpace(arguments.GoalID),
			ExpectedRevision: arguments.Revision, OccurredAt: time.Now(),
		})
		if cancelErr != nil {
			return runtime.goalErrorResult(call, cancelErr)
		}
		runtime.current = nil
		runtime.armed = false
		runtime.forceNoTools = true
		return chatAgentGoalPayloadResult(call, map[string]any{
			"goal":       nil,
			"cleared":    map[string]any{"id": ref.ID, "revision": ref.Revision},
			"activation": "disarmed",
		}), true, "", nil
	}
	objective := ""
	if arguments.Objective != nil {
		objective = *arguments.Objective
	}
	blockedReason := ""
	if arguments.BlockedReason != nil {
		blockedReason = *arguments.BlockedReason
	}
	goal, changeErr := runtime.service.ChangeChatAgentGoal(ctx, ChangeChatAgentGoalInput{
		TurnID: runtime.turnID, EventID: eventID,
		GoalID:           strings.TrimSpace(arguments.GoalID),
		ExpectedRevision: arguments.Revision, Action: action,
		Objective: objective, MaxGoalRounds: arguments.MaxGoalRounds,
		BlockedReason: blockedReason, OccurredAt: time.Now(),
	})
	if changeErr != nil {
		return runtime.goalErrorResult(call, changeErr)
	}
	runtime.current = &goal
	concludes := false
	switch action {
	case ChatAgentGoalActionResume:
		runtime.armed = true
	case ChatAgentGoalActionPause, ChatAgentGoalActionComplete, ChatAgentGoalActionBlocked:
		runtime.armed = false
		runtime.forceNoTools = true
		concludes = true
	}
	return chatAgentGoalSuccessResult(call, runtime.current, runtime.armed), concludes, "", nil
}

func (runtime *chatAgentGoalToolRuntime) changeGoal(
	ctx context.Context,
	input ChangeChatAgentGoalInput,
) (ChatAgentGoal, error) {
	eventID, err := NewUUID()
	if err != nil {
		return ChatAgentGoal{}, err
	}
	input.TurnID = runtime.turnID
	input.EventID = eventID
	input.OccurredAt = time.Now()
	return runtime.service.ChangeChatAgentGoal(ctx, input)
}

func (runtime *chatAgentGoalToolRuntime) goalErrorResult(
	call ProviderToolCall,
	err error,
) (ProviderToolResult, bool, string, error) {
	code := chatAgentGoalErrorCode(err)
	if code == "" {
		return ProviderToolResult{}, false, "", err
	}
	category := strings.ToLower(strings.TrimPrefix(code, "CHAT_AGENT_GOAL_"))
	return chatAgentGoalFailureResult(call, category), false, category, nil
}

func chatAgentGoalUpdateArgumentsValid(
	action string,
	objective *string,
	maxGoalRounds *int,
	blockedReason *string,
) bool {
	hasObjective := objective != nil && strings.TrimSpace(*objective) != ""
	hasMaxRounds := maxGoalRounds != nil
	hasBlockedReason := blockedReason != nil && strings.TrimSpace(*blockedReason) != ""
	switch action {
	case ChatAgentGoalActionEdit:
		return (hasObjective || hasMaxRounds) && !hasBlockedReason
	case ChatAgentGoalActionBlocked:
		return !hasObjective && !hasMaxRounds && hasBlockedReason
	case ChatAgentGoalActionPause,
		ChatAgentGoalActionResume,
		ChatAgentGoalActionComplete,
		ChatAgentGoalActionCancel:
		return !hasObjective && !hasMaxRounds && !hasBlockedReason
	default:
		return false
	}
}

func (s *Service) chatAgentGoalsAvailable() bool {
	if s == nil || s.repo == nil {
		return false
	}
	_, ok := s.repo.(ChatAgentGoalRepository)
	return ok
}
