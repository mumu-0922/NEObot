package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxChatAgentVerificationSummary = 4096

const chatAgentCompletionSystemInstruction = `Completion is evidence-gated. After any successful write or execute Tool, do not claim completion until a later Tool result actually checks the changed state and verify_completion records that exact successful Tool call. A test command may be both the latest execute call and its verification evidence when it genuinely checks the result. Goal state Tools do not count as product verification. If verification fails, repair the work and verify again.`

type chatCompletionEvidence struct {
	Sequence int
	CallID   string
	ToolName string
	Risk     chatToolRiskClass
}

type chatCompletionPolicy struct {
	sequence        int
	lastMutation    chatCompletionEvidence
	verifiedThrough int
	successful      map[string]chatCompletionEvidence
}

func renderChatAgentVerificationPrompt(policy *chatCompletionPolicy) string {
	mutation := policy.lastMutation
	return fmt.Sprintf(`<completion_verification_required>
The latest successful %s Tool call %q (%s) has no later recorded verification evidence. Do not claim success. Run a concrete check of the changed state, observe its successful Tool result, then call verify_completion with that exact evidence Tool call id and a truthful summary. If the check fails, repair and repeat.
</completion_verification_required>`, mutation.Risk, mutation.CallID, mutation.ToolName)
}

func newChatCompletionPolicy() *chatCompletionPolicy {
	return &chatCompletionPolicy{successful: make(map[string]chatCompletionEvidence)}
}

func (policy *chatCompletionPolicy) observe(
	registry *chatToolRegistry,
	calls []ProviderToolCall,
	results []ProviderToolResult,
) {
	if policy == nil {
		return
	}
	for index, call := range calls {
		if index >= len(results) {
			break
		}
		result := results[index]
		policy.sequence++
		if result.IsError {
			continue
		}
		registration, ok := registry.lookup(call.Name)
		if !ok || registration.Backend == chatToolBackendGoal {
			continue
		}
		evidence := chatCompletionEvidence{
			Sequence: policy.sequence, CallID: strings.TrimSpace(call.ID),
			ToolName: registration.Name, Risk: registration.RiskClass,
		}
		if evidence.CallID != "" && !registration.MutationResultNeedsFollowup &&
			chatToolResultCanVerify(registration.Name, result) {
			policy.successful[evidence.CallID] = evidence
		}
		if registration.RiskClass == chatToolRiskWrite ||
			registration.RiskClass == chatToolRiskExecute {
			policy.lastMutation = evidence
		}
	}
}

func chatToolResultCanVerify(name string, result ProviderToolResult) bool {
	switch name {
	case localJobListToolName, localJobKillToolName:
		return false
	case localJobOutputToolName:
		var payload struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		return json.Unmarshal([]byte(result.Content), &payload) == nil &&
			payload.Result.Status == "completed"
	case localTerminalToolName:
		var payload struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(result.Content), &payload) == nil &&
			payload.Result.Status == "running" {
			return false
		}
		return true
	default:
		return true
	}
}

func (policy *chatCompletionPolicy) requiresVerification() bool {
	return policy != nil && policy.lastMutation.Sequence > policy.verifiedThrough
}

func (policy *chatCompletionPolicy) verify(
	evidenceCallID string,
	summary string,
) (chatCompletionEvidence, error) {
	if policy == nil || !policy.requiresVerification() {
		return chatCompletionEvidence{}, errors.New("verification_not_required")
	}
	evidenceCallID = strings.TrimSpace(evidenceCallID)
	summary = strings.TrimSpace(summary)
	if evidenceCallID == "" || summary == "" || len(summary) > maxChatAgentVerificationSummary {
		return chatCompletionEvidence{}, errors.New("verification_arguments_invalid")
	}
	evidence, ok := policy.successful[evidenceCallID]
	if !ok || evidence.Sequence < policy.lastMutation.Sequence {
		return chatCompletionEvidence{}, errors.New("verification_evidence_invalid")
	}
	policy.verifiedThrough = evidence.Sequence
	return evidence, nil
}
