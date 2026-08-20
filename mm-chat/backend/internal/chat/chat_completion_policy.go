package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxChatAgentVerificationSummary = 4096

const chatAgentCompletionSystemInstruction = `Completion is evidence-gated after a successful structured state mutation such as file_write, file_edit, job_kill, or an MCP write. A successful foreground terminal call is a synchronous execution boundary: when no earlier mutation is outstanding, answer from its result and do not call verify_completion. A background terminal start remains outstanding until a later job_output result has status=completed. When verification is required, run a later read or foreground terminal check, observe its successful Tool result, then call verify_completion with the exact evidenceToolCallId returned by that result and a truthful summary. A foreground terminal check may verify an earlier structured mutation. Goal state Tools do not count as product verification. If verification fails, repair the work and verify again.`

type chatCompletionEvidence struct {
	Sequence int
	CallID   string
	ToolName string
	Risk     chatToolRiskClass
	JobID    string
}

type chatCompletionPolicy struct {
	sequence              int
	lastMutation          chatCompletionEvidence
	verifiedThrough       int
	successful            map[string]chatCompletionEvidence
	pendingBackgroundJobs map[string]chatCompletionEvidence
}

func renderChatAgentVerificationPrompt(policy *chatCompletionPolicy) string {
	if len(policy.pendingBackgroundJobs) > 0 {
		return fmt.Sprintf(`<completion_verification_required>
%d background Terminal Job(s) still require completion evidence. Do not claim success. Call job_output with wait=true for each Job. Only a successful result with status=completed can be passed to verify_completion using that result's exact evidenceToolCallId. A foreground Terminal call, Job start/list/kill, running output, or narration cannot verify a background Job.
</completion_verification_required>`, len(policy.pendingBackgroundJobs))
	}
	mutation := policy.lastMutation
	return fmt.Sprintf(`<completion_verification_required>
The latest successful %s Tool call %q (%s) has no later recorded verification evidence. Do not claim success. Run a concrete check of the changed state, observe its successful Tool result, then call verify_completion with that exact evidence Tool call id and a truthful summary. If the check fails, repair and repeat.
</completion_verification_required>`, mutation.Risk, mutation.CallID, mutation.ToolName)
}

func newChatCompletionPolicy() *chatCompletionPolicy {
	return &chatCompletionPolicy{
		successful:            make(map[string]chatCompletionEvidence),
		pendingBackgroundJobs: make(map[string]chatCompletionEvidence),
	}
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
			JobID: chatToolResultJobID(result),
		}
		if evidence.CallID != "" && !registration.MutationResultNeedsFollowup &&
			chatToolResultCanVerify(registration.Name, result) {
			policy.successful[evidence.CallID] = evidence
		}
		if chatToolResultCreatesOutstandingMutation(registration, result) {
			policy.lastMutation = evidence
			if registration.Name == localTerminalToolName {
				jobKey := evidence.JobID
				if jobKey == "" {
					jobKey = "call:" + evidence.CallID
				}
				policy.pendingBackgroundJobs[jobKey] = evidence
			}
		}
	}
}

func chatToolResultCreatesOutstandingMutation(
	registration chatToolRegistration,
	result ProviderToolResult,
) bool {
	switch registration.RiskClass {
	case chatToolRiskWrite:
		return true
	case chatToolRiskExecute:
		// Foreground Terminal already has a synchronous success boundary. Do
		// not guess whether arbitrary Shell text is read-only or mutating.
		// Background Terminal is different: starting the process is not proof
		// that its requested work finished.
		return registration.Name != localTerminalToolName ||
			chatToolResultStatus(result) == "running"
	default:
		return false
	}
}

func chatToolResultCanVerify(name string, result ProviderToolResult) bool {
	switch name {
	case localJobListToolName, localJobKillToolName:
		return false
	case localJobOutputToolName:
		return chatToolResultStatus(result) == "completed"
	case localTerminalToolName:
		return chatToolResultStatus(result) != "running"
	default:
		return true
	}
}

func chatToolResultStatus(result ProviderToolResult) string {
	return chatToolResultState(result).Status
}

type chatToolResultJobState struct {
	Status string
	JobID  string
}

func chatToolResultState(result ProviderToolResult) chatToolResultJobState {
	var payload struct {
		Result struct {
			Status string `json:"status"`
			JobID  string `json:"jobId"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(result.Content), &payload) != nil {
		return chatToolResultJobState{}
	}
	return chatToolResultJobState{
		Status: strings.TrimSpace(payload.Result.Status),
		JobID:  strings.TrimSpace(payload.Result.JobID),
	}
}

func chatToolResultJobID(result ProviderToolResult) string {
	return chatToolResultState(result).JobID
}

func (policy *chatCompletionPolicy) requiresVerification() bool {
	return policy != nil && (policy.lastMutation.Sequence > policy.verifiedThrough ||
		len(policy.pendingBackgroundJobs) > 0)
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
	pendingJobID := ""
	if len(policy.pendingBackgroundJobs) > 0 {
		if evidence.ToolName != localJobOutputToolName || evidence.JobID == "" {
			return chatCompletionEvidence{}, errors.New("verification_evidence_invalid")
		}
		if _, ok := policy.pendingBackgroundJobs[evidence.JobID]; !ok {
			return chatCompletionEvidence{}, errors.New("verification_evidence_invalid")
		}
		pendingJobID = evidence.JobID
	}
	policy.verifiedThrough = evidence.Sequence
	if pendingJobID != "" {
		delete(policy.pendingBackgroundJobs, pendingJobID)
	}
	return evidence, nil
}
