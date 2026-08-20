package chat

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	chatAgentGetGoalToolName          = "get_goal"
	chatAgentCreateGoalToolName       = "create_goal"
	chatAgentUpdateGoalToolName       = "update_goal"
	chatAgentVerifyCompletionToolName = "verify_completion"

	chatAgentGoalBlockedAfterRounds = 3
)

const chatAgentGoalSystemInstruction = `Use Goal Tools for one long-running completion objective in the current conversation. create_goal may infer that intent from the direct human request in any language; do not create a Goal for routine single-turn work. Call get_goal before update_goal and copy its exact goalId and revision. A Goal restored in a later request is disarmed: only when the human asks to continue or resume may update_goal action resume re-arm it. Mark complete only when the whole objective is actually achieved. Mark blocked only after the same blocking condition persists for at least 3 automatic Goal Rounds; difficulty, uncertainty, or useful remaining work is not blocked. pause, resume, edit, and cancel require the direct human turn. There is one Goal per conversation and no Subagent.`

func chatAgentGoalToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        chatAgentGetGoalToolName,
				Description: "Read the current same-conversation Goal, including exact id/revision, lifecycle phase, admitted rounds, blocker, and this request's armed/disarmed activation. Call before updating a Goal.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{}, "properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        chatAgentCreateGoalToolName,
				Description: "Create and arm one persisted Goal only for a long-running objective inferred from the current direct human request. Do not use for routine single-turn work.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"objective", "maxGoalRounds"},
					"properties": map[string]any{
						"objective": map[string]any{
							"type": "string", "minLength": 1,
							"maxLength": maxChatAgentGoalObjective,
						},
						"maxGoalRounds": map[string]any{
							"type":    []string{"integer", "null"},
							"minimum": minChatAgentMaxGoalRounds,
							"maximum": maxChatAgentMaxGoalRounds,
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        chatAgentUpdateGoalToolName,
				Description: "Update the exact current Goal revision. edit, pause, resume, and cancel require the direct human turn. complete and blocked also work in the exact current Goal Round. blocked requires the same condition for three automatic rounds. complete is rejected while a write/execute remains unverified.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{
						"goalId", "revision", "action", "objective",
						"maxGoalRounds", "blockedReason",
					},
					"properties": map[string]any{
						"goalId":   map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
						"revision": map[string]any{"type": "integer", "minimum": 1},
						"action": map[string]any{
							"type": "string",
							"enum": []string{
								ChatAgentGoalActionEdit, ChatAgentGoalActionPause,
								ChatAgentGoalActionResume, ChatAgentGoalActionComplete,
								ChatAgentGoalActionBlocked, ChatAgentGoalActionCancel,
							},
						},
						"objective": map[string]any{
							"type":      []string{"string", "null"},
							"minLength": 1, "maxLength": maxChatAgentGoalObjective,
						},
						"maxGoalRounds": map[string]any{
							"type":    []string{"integer", "null"},
							"minimum": minChatAgentMaxGoalRounds,
							"maximum": maxChatAgentMaxGoalRounds,
						},
						"blockedReason": map[string]any{
							"type":      []string{"string", "null"},
							"minLength": 1, "maxLength": maxChatAgentGoalBlockReason,
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        chatAgentVerifyCompletionToolName,
				Description: "Record evidence only when the completion policy requires verification after a structured mutation or background Terminal start. evidenceToolCallId must be copied exactly from a successful later Tool result that checked the changed state. Do not call this for a foreground Terminal-only task or from narration alone.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"evidenceToolCallId", "summary"},
					"properties": map[string]any{
						"evidenceToolCallId": map[string]any{
							"type": "string", "minLength": 1, "maxLength": 256,
						},
						"summary": map[string]any{
							"type": "string", "minLength": 1,
							"maxLength": maxChatAgentVerificationSummary,
						},
					},
				},
			},
		},
	}
}

func isChatAgentGoalToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case chatAgentGetGoalToolName,
		chatAgentCreateGoalToolName,
		chatAgentUpdateGoalToolName,
		chatAgentVerifyCompletionToolName:
		return true
	default:
		return false
	}
}

func chatAgentGoalSuccessResult(
	call ProviderToolCall,
	goal *ChatAgentGoal,
	armed bool,
) ProviderToolResult {
	payload := map[string]any{"goal": nil, "activation": "disarmed"}
	if goal != nil {
		payload["goal"] = chatAgentGoalProjection(goal)
		if armed && goal.Phase == ChatAgentGoalActive {
			payload["activation"] = "armed"
		}
	}
	return chatAgentGoalPayloadResult(call, payload)
}

func chatAgentGoalPayloadResult(
	call ProviderToolCall,
	payload map[string]any,
) ProviderToolResult {
	payload["untrustedChatAgentGoalResult"] = true
	encoded, _ := json.Marshal(payload)
	return ProviderToolResult{CallID: call.ID, Name: call.Name, Content: string(encoded)}
}

func chatAgentGoalFailureResult(
	call ProviderToolCall,
	category string,
) ProviderToolResult {
	encoded, _ := json.Marshal(map[string]any{
		"untrustedChatAgentGoalResult": true,
		"isError":                      true,
		"error":                        strings.TrimSpace(category),
	})
	return ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: string(encoded), IsError: true,
	}
}

func chatAgentGoalProjection(goal *ChatAgentGoal) map[string]any {
	projected := map[string]any{
		"id": goal.ID, "revision": goal.Revision, "objective": goal.Objective,
		"phase": goal.Phase, "roundsStarted": goal.RoundsStarted,
		"maxGoalRounds": goal.MaxGoalRounds,
	}
	if goal.BlockedReason != nil {
		projected["blockedReason"] = map[string]any{
			"code": goal.BlockedReason.Code, "message": goal.BlockedReason.Message,
		}
	}
	return projected
}

func cloneChatAgentGoalPointer(goal *ChatAgentGoal) *ChatAgentGoal {
	if goal == nil {
		return nil
	}
	cloned := *goal
	if goal.BlockedReason != nil {
		reason := *goal.BlockedReason
		cloned.BlockedReason = &reason
	}
	return &cloned
}

func appendChatAgentGoalSystemInstruction(
	systemPrompt string,
	runtime *chatAgentGoalToolRuntime,
) string {
	if !runtime.enabled() {
		return systemPrompt
	}
	return strings.TrimSpace(strings.TrimSpace(systemPrompt) + "\n\n" +
		chatAgentGoalSystemInstruction + "\n\n" + chatAgentCompletionSystemInstruction)
}

func renderChatAgentGoalRound(goal ChatAgentGoal) string {
	encodedObjective, _ := json.Marshal(goal.Objective)
	return fmt.Sprintf(`<goal_round>
Objective: %s
Round: %d/%d

Continue working toward the objective in this same conversation. Treat the current workspace, Tool results, and durable state as authoritative; inspect them instead of assuming earlier narration is current. Make concrete progress and verify the result. Before claiming completion, read the current Goal, satisfy the completion-evidence gate, and mark it complete. If work remains, leave the Goal active for the next round. Follow the Goal policy before reporting a blocker.
</goal_round>`, encodedObjective, goal.RoundsStarted, goal.MaxGoalRounds)
}

func renderChatAgentGoalWrapup(goal ChatAgentGoal) string {
	encodedObjective, _ := json.Marshal(goal.Objective)
	blocked := ""
	if goal.BlockedReason != nil {
		encodedBlocked, _ := json.Marshal(goal.BlockedReason.Message)
		blocked = "\nBlocked: " + string(encodedBlocked)
	}
	return fmt.Sprintf(`<goal_wrapup>
Objective: %s%s
The Goal is now %s. Write the closing message to the user: state the real outcome, summarize what was done and how it was verified, point to concrete results, and state any remaining review or exact blocker. Use only facts established by this conversation and Tool results. Do not call more Tools in this run.
</goal_wrapup>`, encodedObjective, blocked, goal.Phase)
}
