package chat

import "strings"

const (
	retrievalContextMaxInputPercent     = 40
	retrievalContextEnvelopeReserve     = 512
	retrievalKnowledgeSharePercent      = 60
	minimumRetrievalContextBudgetTokens = 256
)

type retrievalEvidenceLane string

const (
	retrievalEvidenceKnowledge retrievalEvidenceLane = "knowledge"
	retrievalEvidenceWeb       retrievalEvidenceLane = "web"
)

// retrievalContextBudget is turn-local. It bounds all server-authored
// Knowledge and external-Web bodies against the selected answer model rather
// than letting each source independently fill the model window.
type retrievalContextBudget struct {
	totalTokens       int
	knowledgeTokens   int
	webTokens         int
	knowledgeConsumed int
	webConsumed       int
}

func newRetrievalContextBudget(
	request ProviderRequest,
	knowledgeAvailable bool,
	webAvailable bool,
) *retrievalContextBudget {
	_, inputBudget := defaultContextBudgetPolicy().inputBudgetTokens(
		request.ModelRef.ModelID,
	)
	baseTokens := estimateProviderInputTokens(
		request.SystemPrompt,
		request.Messages,
	)
	if prompt := strings.TrimSpace(request.Prompt); prompt != "" &&
		!providerMessagesContainPrompt(request.Messages, prompt) {
		baseTokens += estimateProviderTextTokens(prompt) + 6
	}
	available := inputBudget - baseTokens - retrievalContextEnvelopeReserve
	maximum := inputBudget * retrievalContextMaxInputPercent / 100
	total := min(available, maximum)
	if total < minimumRetrievalContextBudgetTokens {
		total = 0
	}

	budget := &retrievalContextBudget{totalTokens: total}
	switch {
	case knowledgeAvailable && webAvailable:
		budget.knowledgeTokens = total * retrievalKnowledgeSharePercent / 100
		budget.webTokens = total - budget.knowledgeTokens
	case knowledgeAvailable:
		budget.knowledgeTokens = total
	case webAvailable:
		budget.webTokens = total
	}
	return budget
}

func providerMessagesContainPrompt(messages []ProviderMessage, prompt string) bool {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != "user" {
			continue
		}
		return strings.TrimSpace(messages[index].Content) == prompt
	}
	return false
}

func (budget *retrievalContextBudget) limit(
	lane retrievalEvidenceLane,
) int {
	if budget == nil {
		return 0
	}
	switch lane {
	case retrievalEvidenceKnowledge:
		return budget.knowledgeTokens
	case retrievalEvidenceWeb:
		return budget.webTokens
	default:
		return 0
	}
}

func (budget *retrievalContextBudget) remaining(
	lane retrievalEvidenceLane,
) int {
	if budget == nil {
		return 0
	}
	remaining := 0
	switch lane {
	case retrievalEvidenceKnowledge:
		remaining = budget.knowledgeTokens - budget.knowledgeConsumed
	case retrievalEvidenceWeb:
		remaining = budget.webTokens - budget.webConsumed
	}
	return max(remaining, 0)
}

func (budget *retrievalContextBudget) consume(
	lane retrievalEvidenceLane,
	tokens int,
) {
	if budget == nil || tokens <= 0 {
		return
	}
	tokens = min(tokens, budget.remaining(lane))
	switch lane {
	case retrievalEvidenceKnowledge:
		budget.knowledgeConsumed += tokens
	case retrievalEvidenceWeb:
		budget.webConsumed += tokens
	}
}
