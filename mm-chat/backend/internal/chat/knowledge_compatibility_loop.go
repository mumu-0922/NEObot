package chat

import (
	"context"
	"strings"
)

type compatibilityKnowledgeLoopInput struct {
	Provider             Provider
	Request              ProviderRequest
	Runtime              *knowledgeToolRuntime
	ConversationMessages []Message
	BuiltInSearch        ModelBuiltInSearchProvider
	ExternalSearch       *externalWebToolLoopInput
}

func startCompatibilityKnowledgeLoop(
	ctx context.Context,
	input compatibilityKnowledgeLoopInput,
) <-chan ProviderEvent {
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		request, decision, ok := prepareCompatibilityKnowledgeRequest(
			ctx,
			events,
			input,
		)
		if !ok {
			return
		}
		if input.ExternalSearch != nil && externalWebToolEnabled(*input.ExternalSearch) {
			external := *input.ExternalSearch
			external.Request = request
			external.KnowledgeReady = decision.ReadyForAnswer()
			runCompatibilityExternalWebSearch(ctx, events, external)
			return
		}
		if input.BuiltInSearch != nil {
			fallbackRequest := request
			if decision.ReadyForAnswer() {
				request.SystemPrompt = applySourceFusionSystemInstruction(
					request.SystemPrompt,
					sourceFusionPlan{Authority: sourceAuthorityMixed},
				)
			}
			if !sendProviderEvent(ctx, events, ProviderEvent{
				Type: ProviderEventSearchStarted,
			}) {
				return
			}
			roundEvents, err := input.BuiltInSearch.StreamChatWithModelBuiltInSearch(
				ctx,
				request,
			)
			if err == nil {
				for event := range roundEvents {
					if !sendProviderEvent(ctx, events, event) {
						return
					}
				}
				return
			}
			if !sendProviderEvent(ctx, events, ProviderEvent{
				Type:            ProviderEventSearchDegraded,
				FailureCategory: "provider_failed",
			}) {
				return
			}
			streamCompatibilityAnswer(
				ctx,
				events,
				input.Provider,
				withWebUnavailableInstruction(fallbackRequest),
			)
			return
		}
		streamCompatibilityAnswer(ctx, events, input.Provider, request)
	}()
	return events
}

func prepareCompatibilityKnowledgeRequest(
	ctx context.Context,
	events chan<- ProviderEvent,
	input compatibilityKnowledgeLoopInput,
) (ProviderRequest, autoRAGDecision, bool) {
	running := ProviderToolExecutionEvent{
		ExecutionID: "compatibility-knowledge-1",
		Name:        searchKnowledgeToolName,
		Status:      ProcessStepStatusRunning,
		Round:       1,
		Mode:        "compatibility",
	}
	if !sendToolExecutionEvent(ctx, events, running) {
		return input.Request, autoRAGDecision{}, false
	}
	query := ""
	if input.Runtime != nil {
		query = strings.Join(strings.Fields(input.Runtime.OriginalQueryText), " ")
	}
	if query == "" {
		query = strings.Join(strings.Fields(input.Request.Prompt), " ")
	}
	if shouldRewriteRAGQuery(query) {
		if rewritten, err := rewriteRAGQuery(
			ctx,
			input.Provider,
			input.Request.ModelRef,
			input.Request.UserMessageID,
			query,
			input.ConversationMessages,
		); err == nil && strings.TrimSpace(rewritten) != "" {
			query = rewritten
		}
	}
	running.Query = query
	running.Arguments = map[string]any{"query": query}
	decision := executeKnowledgeTool(ctx, input.Runtime, query)
	failure := knowledgeToolFailureCategory(decision)
	request := input.Request
	if failure == "" && decision.ReadyForAnswer() {
		prompt, systemPrompt, err := buildAutoRAGProviderRequest(
			input.Runtime.OriginalQueryText,
			request.SystemPrompt,
			decision.Evidence,
			decision.Citations,
		)
		if err != nil {
			decision = autoRAGDecision{Outcome: "dependency_unavailable"}
			failure = decision.Outcome
		} else {
			request.Prompt = prompt
			request.SystemPrompt = systemPrompt
			request.Messages = replaceLastUserProviderMessage(request.Messages, prompt)
			request.Metadata = mergeAutoRAGProviderMetadata(request.Metadata, decision)
		}
	}
	terminal := running
	terminal.Knowledge = &decision
	if failure != "" {
		terminal.Status = ProcessStepStatusFailed
		terminal.FailureCategory = failure
		request = withKnowledgeUnavailableInstruction(request)
	} else {
		terminal.Status = ProcessStepStatusCompleted
		terminal.CitationMarkers = ragCitationMarkers(decision.Citations)
	}
	if !sendToolExecutionEvent(ctx, events, terminal) {
		return request, decision, false
	}
	return request, decision, true
}
