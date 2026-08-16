package chat

import (
	"context"
	"strings"
)

const maxParallelChatReadTools = 4

type chatParallelBackendExecution struct {
	results       map[int]ProviderToolResult
	budgetReached bool
	err           error
}

func (registry *chatToolRegistry) executeToolBatch(
	ctx context.Context,
	events chan<- ProviderEvent,
	input externalWebToolLoopInput,
	calls []ProviderToolCall,
	round int,
	retrievalState *chatRetrievalExecutionState,
) (chatToolBatchExecution, error) {
	execution := chatToolBatchExecution{Results: make(map[int]ProviderToolResult)}
	if registry == nil {
		return execution, nil
	}
	if registry.requiredLocalSkill {
		results, budgetReached, err := executeRequiredLocalSkillBatch(
			ctx, events, input.LocalSkills, calls, round,
		)
		execution.Results = registry.projectResults(results)
		execution.BudgetReached = budgetReached
		return execution, err
	}

	for offset := 0; offset < len(calls); {
		registration, registered := registry.lookup(calls[offset].Name)
		if registered && registration.AllowParallel {
			end := offset + 1
			for end < len(calls) && end-offset < maxParallelChatReadTools {
				next, ok := registry.lookup(calls[end].Name)
				if !ok || !next.AllowParallel {
					break
				}
				end++
			}
			parallel, err := registry.executeParallelReadGroup(
				ctx, events, input, calls[offset:end], round,
			)
			for index, result := range parallel.results {
				execution.Results[offset+index] = registry.projectResult(result)
			}
			execution.BudgetReached = execution.BudgetReached || parallel.budgetReached
			if err != nil {
				return execution, err
			}
			offset = end
			continue
		}

		result, budgetReached, concludesTurn, stop, err := registry.executeSerialToolCall(
			ctx, events, input, calls[offset], round, offset, registration, registered,
			retrievalState,
		)
		if result.Name != "" || result.CallID != "" || result.Content != "" {
			execution.Results[offset] = registry.projectResult(result)
		}
		execution.BudgetReached = execution.BudgetReached || budgetReached
		if err != nil {
			return execution, err
		}
		if stop {
			execution.Stop = true
			return execution, nil
		}
		if concludesTurn {
			for index := offset + 1; index < len(calls); index++ {
				if strings.TrimSpace(calls[index].Name) != "" {
					execution.Results[index] = chatToolFailureResult(
						calls[index], "goal_concluded",
					)
				}
			}
			return execution, nil
		}
		offset++
	}
	return execution, nil
}

func (registry *chatToolRegistry) executeParallelReadGroup(
	ctx context.Context,
	events chan<- ProviderEvent,
	input externalWebToolLoopInput,
	calls []ProviderToolCall,
	round int,
) (chatParallelBackendExecution, error) {
	execution := chatParallelBackendExecution{results: make(map[int]ProviderToolResult)}
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completed := make(chan chatParallelBackendExecution, 2)
	started := 0

	if mcpCalls := registry.callsForBackend(calls, chatToolBackendMCP); hasToolCalls(mcpCalls) {
		started++
		go func() {
			results, budgetReached, err := executeMCPBatch(
				groupCtx, events, input.MCP, mcpCalls, round,
			)
			completed <- chatParallelBackendExecution{
				results: results, budgetReached: budgetReached, err: err,
			}
		}()
	}
	if localCalls := registry.callsForBackend(calls, chatToolBackendLocalSkill); hasToolCalls(localCalls) {
		started++
		go func() {
			results, budgetReached, err := executeLocalSkillBatch(
				groupCtx, events, input.LocalSkills, localCalls, round,
			)
			completed <- chatParallelBackendExecution{
				results: results, budgetReached: budgetReached, err: err,
			}
		}()
	}
	if started == 0 {
		for index, call := range calls {
			execution.results[index] = chatToolFailureResult(call, "tool_not_available")
		}
		return execution, nil
	}
	for count := 0; count < started; count++ {
		current := <-completed
		for index, result := range current.results {
			execution.results[index] = result
		}
		execution.budgetReached = execution.budgetReached || current.budgetReached
		if current.err != nil && execution.err == nil {
			execution.err = current.err
			cancel()
		}
	}
	for index, call := range calls {
		if _, exists := execution.results[index]; !exists {
			execution.results[index] = chatToolFailureResult(call, "tool_not_available")
		}
	}
	return execution, execution.err
}

func (registry *chatToolRegistry) executeSerialToolCall(
	ctx context.Context,
	events chan<- ProviderEvent,
	input externalWebToolLoopInput,
	call ProviderToolCall,
	round int,
	callIndex int,
	registration chatToolRegistration,
	registered bool,
	retrievalState *chatRetrievalExecutionState,
) (ProviderToolResult, bool, bool, bool, error) {
	if !registered {
		executed := registry.executeRetrievalCall(
			ctx, call, callIndex, retrievalState,
		)
		return executed.result, false, false, executed.stop, nil
	}
	single := []ProviderToolCall{call}
	switch registration.Backend {
	case chatToolBackendGoal:
		goal, err := executeChatAgentGoalBatch(ctx, events, input.Goals, single, round)
		return goal.Results[0], false, goal.ConcludesTurn, false, err
	case chatToolBackendMCP:
		results, budgetReached, err := executeMCPBatch(ctx, events, input.MCP, single, round)
		return resultAt(results, call, 0), budgetReached, false, false, err
	case chatToolBackendLocalSkill:
		results, budgetReached, err := executeLocalSkillBatch(
			ctx, events, input.LocalSkills, single, round,
		)
		return resultAt(results, call, 0), budgetReached, false, false, err
	case chatToolBackendWeb, chatToolBackendKnowledge, chatToolBackendMemory:
		executed := registry.executeRetrievalCall(
			ctx, call, callIndex, retrievalState,
		)
		return executed.result, false, false, executed.stop, nil
	default:
		return chatToolFailureResult(call, "tool_not_available"), false, false, false, nil
	}
}

func resultAt(
	results map[int]ProviderToolResult,
	call ProviderToolCall,
	index int,
) ProviderToolResult {
	if result, ok := results[index]; ok {
		return result
	}
	return chatToolFailureResult(call, "tool_not_available")
}

func hasToolCalls(calls []ProviderToolCall) bool {
	for _, call := range calls {
		if strings.TrimSpace(call.Name) != "" {
			return true
		}
	}
	return false
}
