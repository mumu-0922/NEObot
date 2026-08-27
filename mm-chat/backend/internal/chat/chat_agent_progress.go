package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	chatAgentOutcomeBlocked             = "blocked"
	chatAgentBlockRepeatedToolOutcome   = "repeated_tool_outcome"
	chatAgentBlockConsecutiveToolErrors = "consecutive_tool_errors"
	maxRepeatedChatAgentToolOutcomes    = 3
	maxConsecutiveChatAgentErrorRounds  = 5
)

const chatAgentBlockedSystemInstruction = `The Agent run has been stopped by the no-progress guard, not completed successfully.
Give a concise final status report with: what remains incomplete, the last verified result, the blocking reason, and the safest concrete next action. Do not call tools. Do not claim success or imply that an unpublished file is downloadable.`

const chatAgentNarrationSystemInstruction = `Keep the user oriented during Tool-based work with brief user-visible progress updates. Before each meaningful new Tool phase, state in one concise sentence what you are about to do and why. When a Tool result materially changes the plan, briefly state the observed outcome before the next Tool phase. These updates are user-facing narration, not hidden reasoning: never expose chain-of-thought, repeat raw Tool output, invent results, or narrate every trivial call. Keep the final answer concise and separate from the progress updates.`

type chatAgentProgressTracker struct {
	lastFingerprint       string
	repeatedOutcomes      int
	consecutiveErrorRound int
}

func newChatAgentProgressTracker() *chatAgentProgressTracker {
	return &chatAgentProgressTracker{}
}

func (tracker *chatAgentProgressTracker) observe(
	calls []ProviderToolCall,
	results []ProviderToolResult,
) (string, bool) {
	if tracker == nil || len(calls) == 0 || len(results) == 0 {
		return "", false
	}
	fingerprint := chatAgentToolOutcomeFingerprint(calls, results)
	if fingerprint != "" && fingerprint == tracker.lastFingerprint {
		tracker.repeatedOutcomes++
	} else {
		tracker.lastFingerprint = fingerprint
		tracker.repeatedOutcomes = 1
	}

	allErrors := true
	for _, result := range results {
		if !result.IsError {
			allErrors = false
			break
		}
	}
	if allErrors {
		tracker.consecutiveErrorRound++
	} else {
		tracker.consecutiveErrorRound = 0
	}

	if tracker.repeatedOutcomes >= maxRepeatedChatAgentToolOutcomes {
		return chatAgentBlockRepeatedToolOutcome, true
	}
	if tracker.consecutiveErrorRound >= maxConsecutiveChatAgentErrorRounds {
		return chatAgentBlockConsecutiveToolErrors, true
	}
	return "", false
}

func chatAgentToolOutcomeFingerprint(
	calls []ProviderToolCall,
	results []ProviderToolResult,
) string {
	type fingerprintCall struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	type fingerprintResult struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		IsError bool   `json:"isError"`
	}
	payload := struct {
		Calls   []fingerprintCall   `json:"calls"`
		Results []fingerprintResult `json:"results"`
	}{
		Calls:   make([]fingerprintCall, 0, len(calls)),
		Results: make([]fingerprintResult, 0, len(results)),
	}
	for _, call := range calls {
		payload.Calls = append(payload.Calls, fingerprintCall{
			Name: strings.TrimSpace(call.Name), Arguments: compactAgentJSON(call.Arguments),
		})
	}
	for _, result := range results {
		payload.Results = append(payload.Results, fingerprintResult{
			Name: strings.TrimSpace(result.Name), Content: compactAgentOutcomeContent(result.Content),
			IsError: result.IsError,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func compactAgentOutcomeContent(value string) string {
	var decoded any
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &decoded); err != nil {
		return strings.TrimSpace(value)
	}
	removeAgentOutcomeIdentifiers(decoded)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return string(encoded)
}

func removeAgentOutcomeIdentifiers(value any) {
	switch current := value.(type) {
	case map[string]any:
		delete(current, "callId")
		delete(current, "evidenceToolCallId")
		delete(current, "executionId")
		for _, child := range current {
			removeAgentOutcomeIdentifiers(child)
		}
	case []any:
		for _, child := range current {
			removeAgentOutcomeIdentifiers(child)
		}
	}
}

func compactAgentJSON(value string) string {
	var decoded any
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &decoded); err != nil {
		return strings.TrimSpace(value)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return string(encoded)
}

func withChatAgentBlockedInstruction(
	request ProviderRequest,
	reason string,
) ProviderRequest {
	instruction := chatAgentBlockedSystemInstruction + "\nBlocking reason: " + strings.TrimSpace(reason) + "."
	request.SystemPrompt = strings.TrimSpace(request.SystemPrompt)
	if request.SystemPrompt != "" {
		request.SystemPrompt += "\n\n"
	}
	request.SystemPrompt += instruction
	return request
}

func appendChatAgentNarrationSystemInstruction(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if strings.Contains(systemPrompt, chatAgentNarrationSystemInstruction) {
		return systemPrompt
	}
	if systemPrompt != "" {
		systemPrompt += "\n\n"
	}
	return systemPrompt + chatAgentNarrationSystemInstruction
}
