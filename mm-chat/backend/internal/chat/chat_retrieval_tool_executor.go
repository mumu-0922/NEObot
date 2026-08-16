package chat

import (
	"context"
	"fmt"

	"neo-chat/mm-chat/backend/internal/websearch"
)

type chatRetrievalExecutionState struct {
	input             *externalWebToolLoopInput
	events            chan<- ProviderEvent
	round             int
	taskStep          int
	memoryBatchValid  bool
	cumulative        *websearch.Result
	knowledgeDecision *autoRAGDecision
}

type chatToolCallExecution struct {
	result ProviderToolResult
	stop   bool
}

func (registry *chatToolRegistry) executeRetrievalCall(
	ctx context.Context,
	call ProviderToolCall,
	callIndex int,
	state *chatRetrievalExecutionState,
) chatToolCallExecution {
	if registry == nil || state == nil || state.input == nil ||
		state.cumulative == nil || state.knowledgeDecision == nil {
		return chatToolCallExecution{result: chatToolFailureResult(call, "tool_not_available")}
	}
	name := normalizedToolName(call.Name)
	registration, registered := registry.lookup(name)
	query, arguments, failure := validateRegisteredRetrievalToolCall(
		call, registration, registered, *state.input, state.taskStep, state.memoryBatchValid,
	)
	executionID := fmt.Sprintf("native-%d-%d", state.round, callIndex+1)
	if failure != "" {
		execution := ProviderToolExecutionEvent{
			ExecutionID: executionID, CallID: call.ID, Name: name,
			Status: ProcessStepStatusFailed, Round: state.round, Arguments: arguments,
			FailureCategory: failure, Mode: "native",
		}
		if !sendToolExecutionEvent(ctx, state.events, execution) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: retrievalToolFailureResult(name, failure), IsError: true,
		})}
	}
	switch registration.Backend {
	case chatToolBackendMemory:
		return registry.executeMemoryCall(ctx, call, executionID, arguments, state)
	case chatToolBackendKnowledge:
		return registry.executeKnowledgeCall(ctx, call, executionID, query, arguments, state)
	case chatToolBackendWeb:
		return registry.executeWebCall(ctx, call, executionID, query, arguments, state)
	default:
		return chatToolCallExecution{result: chatToolFailureResult(call, "tool_not_available")}
	}
}

func (registry *chatToolRegistry) executeMemoryCall(
	ctx context.Context,
	call ProviderToolCall,
	executionID string,
	arguments map[string]any,
	state *chatRetrievalExecutionState,
) chatToolCallExecution {
	running := ProviderToolExecutionEvent{
		ExecutionID: executionID, CallID: call.ID, Name: call.Name,
		Status: ProcessStepStatusRunning, Round: state.round,
		Arguments: arguments, Mode: "native",
	}
	if !sendToolExecutionEvent(ctx, state.events, running) {
		return chatToolCallExecution{stop: true}
	}
	result := executeMemoryTool(ctx, state.input.Memory)
	if toolLoopWasCancelled(ctx, nil) {
		cancelled := running
		cancelled.Status = ProcessStepStatusCancelled
		sendToolExecutionEvent(ctx, state.events, cancelled)
		return chatToolCallExecution{stop: true}
	}
	if result.FailureCategory != "" {
		failed := running
		failed.Status = ProcessStepStatusFailed
		failed.FailureCategory = result.FailureCategory
		if !sendToolExecutionEvent(ctx, state.events, failed) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: memoryToolFailureResult(result.FailureCategory), IsError: true,
		})}
	}
	toolResult, projected, usedTokens, fits := memoryToolSuccessResult(
		result.Memories,
		state.input.ContextBudget.remaining(retrievalEvidenceMemory),
	)
	if !fits {
		failed := running
		failed.Status = ProcessStepStatusFailed
		failed.FailureCategory = "context_budget_exhausted"
		if !sendToolExecutionEvent(ctx, state.events, failed) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: memoryToolFailureResult("context_budget_exhausted"), IsError: true,
		})}
	}
	state.input.ContextBudget.consume(retrievalEvidenceMemory, usedTokens)
	state.input.Memory.setUsedMemories(projected)
	completed := running
	completed.Status = ProcessStepStatusCompleted
	if !sendToolExecutionEvent(ctx, state.events, completed) {
		return chatToolCallExecution{stop: true}
	}
	return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: toolResult,
	})}
}

func (registry *chatToolRegistry) executeKnowledgeCall(
	ctx context.Context,
	call ProviderToolCall,
	executionID string,
	query string,
	arguments map[string]any,
	state *chatRetrievalExecutionState,
) chatToolCallExecution {
	running := ProviderToolExecutionEvent{
		ExecutionID: executionID, CallID: call.ID, Name: call.Name,
		Status: ProcessStepStatusRunning, Round: state.round,
		Arguments: arguments, Query: query, Mode: "native",
	}
	if !sendToolExecutionEvent(ctx, state.events, running) {
		return chatToolCallExecution{stop: true}
	}
	current := executeKnowledgeTool(ctx, state.input.Knowledge, query)
	if toolLoopWasCancelled(ctx, nil) {
		cancelled := running
		cancelled.Status = ProcessStepStatusCancelled
		sendToolExecutionEvent(ctx, state.events, cancelled)
		return chatToolCallExecution{stop: true}
	}
	failure := knowledgeToolFailureCategory(current)
	if failure != "" {
		failed := running
		failed.Status = ProcessStepStatusFailed
		failed.FailureCategory = failure
		if state.knowledgeDecision.ReadyForAnswer() {
			copy := *state.knowledgeDecision
			failed.Knowledge = &copy
		} else {
			copy := current
			failed.Knowledge = &copy
		}
		if !sendToolExecutionEvent(ctx, state.events, failed) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: knowledgeToolFailureResult(failure), IsError: true,
		})}
	}
	current = mapKnowledgeToolDecisionMarkers(*state.knowledgeDecision, current)
	toolResult, projectedCurrent, usedTokens, contextErr := knowledgeToolSuccessResult(
		current,
		state.input.ContextBudget.remaining(retrievalEvidenceKnowledge),
	)
	if contextErr != nil {
		failed := running
		failed.Status = ProcessStepStatusFailed
		failed.FailureCategory = "context_budget_exhausted"
		if !sendToolExecutionEvent(ctx, state.events, failed) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: knowledgeToolFailureResult("context_budget_exhausted"), IsError: true,
		})}
	}
	merged := mergeKnowledgeToolDecision(*state.knowledgeDecision, projectedCurrent)
	*state.knowledgeDecision = merged.Cumulative
	state.input.ContextBudget.consume(retrievalEvidenceKnowledge, usedTokens)
	if merged.Current.ReadyForAnswer() {
		authority := sourceAuthorityKnowledge
		if len(state.cumulative.Sources) > 0 {
			authority = sourceAuthorityMixed
		}
		state.input.Request.SystemPrompt = applySourceFusionSystemInstruction(
			state.input.Request.SystemPrompt,
			sourceFusionPlan{Authority: authority},
		)
	}
	completed := running
	completed.Status = ProcessStepStatusCompleted
	completed.CitationMarkers = ragCitationMarkers(merged.Current.Citations)
	copy := *state.knowledgeDecision
	if !copy.ReadyForAnswer() {
		copy = merged.Current
	}
	completed.Knowledge = &copy
	if !sendToolExecutionEvent(ctx, state.events, completed) {
		return chatToolCallExecution{stop: true}
	}
	return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: toolResult,
	})}
}

func (registry *chatToolRegistry) executeWebCall(
	ctx context.Context,
	call ProviderToolCall,
	executionID string,
	query string,
	arguments map[string]any,
	state *chatRetrievalExecutionState,
) chatToolCallExecution {
	running := ProviderToolExecutionEvent{
		ExecutionID: executionID, CallID: call.ID, Name: call.Name,
		Status: ProcessStepStatusRunning, Round: state.round,
		Arguments: arguments, Query: query, Mode: "native",
	}
	if !sendToolExecutionEvent(ctx, state.events, running) {
		return chatToolCallExecution{stop: true}
	}
	result, searchErr := state.input.SearchService.Execute(
		ctx,
		state.input.Execution,
		websearch.Request{Query: query, MaxResults: state.input.MaxResults},
	)
	if searchErr != nil {
		if toolLoopWasCancelled(ctx, searchErr) {
			cancelled := running
			cancelled.Status = ProcessStepStatusCancelled
			sendToolExecutionEvent(ctx, state.events, cancelled)
			return chatToolCallExecution{stop: true}
		}
		failure := sourceSearchDegradationReason(searchErr)
		failed := running
		failed.Status = ProcessStepStatusFailed
		failed.FailureCategory = failure
		if !sendToolExecutionEvent(ctx, state.events, failed) {
			return chatToolCallExecution{stop: true}
		}
		return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: webSearchFailureToolResult(failure), IsError: true,
		})}
	}
	previous := *state.cumulative
	bounded, cumulativeResult, toolResult, usedTokens := boundedWebSearchSuccessToolResult(
		previous,
		result,
		state.input.ContextBudget.remaining(retrievalEvidenceWeb),
	)
	*state.cumulative = cumulativeResult
	state.input.ContextBudget.consume(retrievalEvidenceWeb, usedTokens)
	if state.input.KnowledgeReady || state.knowledgeDecision.ReadyForAnswer() {
		state.input.Request.SystemPrompt = applySourceFusionSystemInstruction(
			state.input.Request.SystemPrompt,
			sourceFusionPlan{Authority: sourceAuthorityMixed},
		)
	}
	if !sendProviderEvent(ctx, state.events, ProviderEvent{
		Type: ProviderEventSearch, Search: &bounded,
	}) {
		return chatToolCallExecution{stop: true}
	}
	completed := running
	completed.Status = ProcessStepStatusCompleted
	completed.Search = &bounded
	completed.CitationMarkers = newWebCitationMarkers(previous, *state.cumulative)
	if !sendToolExecutionEvent(ctx, state.events, completed) {
		return chatToolCallExecution{stop: true}
	}
	return chatToolCallExecution{result: registry.projectResult(ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: toolResult,
	})}
}
