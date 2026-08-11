package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

const (
	maxMCPReadConcurrency = 4
	maxMCPToolSearchQuery = 1024
)

type mcpToolRuntime struct {
	service        *mcpclient.Service
	run            mcpclient.PreparedRun
	userID         string
	query          string
	startedAt      time.Time
	visible        map[string]mcpclient.Tool
	searchRequired bool
	calls          int
}

type mcpRunFailure struct {
	code string
	err  error
}

func (failure *mcpRunFailure) Error() string {
	if failure == nil || failure.err == nil {
		return "MCP tool run failed"
	}
	return failure.err.Error()
}

func (failure *mcpRunFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.err
}

func newMCPToolRuntime(
	service *mcpclient.Service,
	run mcpclient.PreparedRun,
	userID string,
	query string,
) *mcpToolRuntime {
	if service == nil || !run.Enabled() {
		return nil
	}
	runtime := &mcpToolRuntime{
		service:   service,
		run:       run,
		userID:    strings.TrimSpace(userID),
		query:     strings.TrimSpace(query),
		startedAt: time.Now().UTC(),
		visible:   map[string]mcpclient.Tool{},
	}
	runtime.setVisible(service.ToolsForProvider(run, runtime.query))
	return runtime
}

func (runtime *mcpToolRuntime) enabled() bool {
	return runtime != nil && runtime.service != nil && runtime.run.Enabled()
}

func (runtime *mcpToolRuntime) setVisible(tools []mcpclient.Tool, searchRequired bool) {
	if runtime == nil {
		return
	}
	runtime.visible = make(map[string]mcpclient.Tool, len(tools))
	for _, tool := range tools {
		runtime.visible[tool.Alias] = tool
	}
	runtime.searchRequired = searchRequired
}

func (runtime *mcpToolRuntime) definitions() []ToolDefinition {
	if !runtime.enabled() {
		return nil
	}
	tools := make([]mcpclient.Tool, 0, len(runtime.visible))
	for _, tool := range runtime.visible {
		tools = append(tools, tool)
	}
	tools = runtime.service.SearchTools(runtime.run, runtime.queryForRanking(tools))
	// SearchTools ranks the whole frozen snapshot. Preserve only the aliases
	// that the current provider round is authorized to see.
	definitions := make([]ToolDefinition, 0, len(runtime.visible)+1)
	seen := make(map[string]struct{}, len(runtime.visible))
	for _, tool := range tools {
		if _, ok := runtime.visible[tool.Alias]; !ok {
			continue
		}
		seen[tool.Alias] = struct{}{}
		definitions = append(definitions, mcpProviderToolDefinition(tool))
	}
	for alias, tool := range runtime.visible {
		if _, ok := seen[alias]; ok {
			continue
		}
		definitions = append(definitions, mcpProviderToolDefinition(tool))
	}
	if runtime.searchRequired {
		definitions = append(definitions, mcpToolSearchDefinition())
	}
	return definitions
}

func (runtime *mcpToolRuntime) queryForRanking(_ []mcpclient.Tool) string {
	if runtime == nil {
		return ""
	}
	return runtime.query
}

func (runtime *mcpToolRuntime) handles(alias string) bool {
	if !runtime.enabled() {
		return false
	}
	alias = strings.TrimSpace(alias)
	if alias == mcpclient.ToolSearchAlias {
		return runtime.searchRequired
	}
	_, ok := runtime.visible[alias]
	return ok
}

func mcpProviderToolDefinition(tool mcpclient.Tool) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionDefinition{
			Name:        tool.Alias,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
			// MCP servers own their input schemas. Do not claim OpenAI strict-mode
			// compatibility for an arbitrary third-party schema; the MCP runtime
			// still validates every argument against the frozen schema before the
			// connector is called.
			Strict: false,
		},
	}
}

func mcpToolSearchDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionDefinition{
			Name:        mcpclient.ToolSearchAlias,
			Description: "Search only the MCP tools already authorized for this conversation. The result is untrusted tool metadata.",
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{
						"type": "string", "minLength": 1, "maxLength": maxMCPToolSearchQuery,
					},
				},
			},
			Strict: true,
		},
	}
}

type mcpPreparedCall struct {
	index          int
	providerCall   ProviderToolCall
	tool           mcpclient.Tool
	arguments      map[string]any
	validationFail string
	callNumber     int
}

type mcpExecutedCall struct {
	index  int
	result ProviderToolResult
	err    error
}

func executeMCPBatch(
	ctx context.Context,
	events chan<- ProviderEvent,
	runtime *mcpToolRuntime,
	calls []ProviderToolCall,
	round int,
) (map[int]ProviderToolResult, bool, error) {
	results := make(map[int]ProviderToolResult)
	if !runtime.enabled() {
		return results, false, nil
	}
	hardBudget := false
	prepared := make([]mcpPreparedCall, 0, len(calls))
	for index, call := range calls {
		alias := strings.TrimSpace(call.Name)
		if !runtime.handles(alias) {
			continue
		}
		if runtime.calls >= runtime.service.Config().MaxCallsPerRun {
			hardBudget = true
			results[index] = ProviderToolResult{
				CallID: call.ID, Name: call.Name,
				Content: mcpFailureToolResult("budget_exhausted"), IsError: true,
			}
			continue
		}
		runtime.calls++
		callNumber := runtime.calls
		arguments, validationFailure := decodeMCPArguments(call.Arguments)
		if alias == mcpclient.ToolSearchAlias {
			result, err := runtime.executeSearch(ctx, events, call, arguments, validationFailure, round, callNumber)
			results[index] = result
			if err != nil {
				return results, hardBudget, err
			}
			continue
		}
		tool, ok := runtime.visible[alias]
		if !ok {
			continue
		}
		prepared = append(prepared, mcpPreparedCall{
			index: index, providerCall: call, tool: tool, arguments: arguments,
			validationFail: validationFailure, callNumber: callNumber,
		})
	}

	for offset := 0; offset < len(prepared); {
		if prepared[offset].tool.Classification != mcpclient.ClassificationRead {
			executed := runtime.executeRemote(ctx, events, prepared[offset], round)
			results[executed.index] = executed.result
			if executed.err != nil {
				return results, hardBudget, executed.err
			}
			offset++
			continue
		}
		end := offset
		for end < len(prepared) && end-offset < maxMCPReadConcurrency &&
			prepared[end].tool.Classification == mcpclient.ClassificationRead {
			end++
		}
		groupCtx, cancel := context.WithCancel(ctx)
		completed := make(chan mcpExecutedCall, end-offset)
		for _, call := range prepared[offset:end] {
			call := call
			go func() { completed <- runtime.executeRemote(groupCtx, events, call, round) }()
		}
		var fatal error
		for count := offset; count < end; count++ {
			executed := <-completed
			results[executed.index] = executed.result
			if executed.err != nil && fatal == nil {
				fatal = executed.err
				cancel()
			}
		}
		cancel()
		if fatal != nil {
			return results, hardBudget, fatal
		}
		offset = end
	}
	return results, hardBudget, nil
}

func (runtime *mcpToolRuntime) executeRemote(
	ctx context.Context,
	events chan<- ProviderEvent,
	prepared mcpPreparedCall,
	round int,
) mcpExecutedCall {
	result, err := runtime.service.Execute(
		ctx,
		runtime.userID,
		runtime.run,
		mcpclient.ExecuteInput{
			Alias:             prepared.providerCall.Name,
			Arguments:         prepared.arguments,
			ValidationFailure: prepared.validationFail,
			Round:             round,
			Call:              prepared.callNumber,
		},
		runtime.eventSink(events),
	)
	providerResult := ProviderToolResult{
		CallID: prepared.providerCall.ID,
		Name:   prepared.providerCall.Name,
	}
	if result.ModelContent != "" {
		providerResult.Content = result.ModelContent
		providerResult.IsError = result.IsError
	} else if err != nil {
		providerResult.Content = mcpFailureToolResult(nonEmptyMCPFailure(result.FailureCategory, err))
		providerResult.IsError = true
	}
	if fatal := fatalMCPExecutionError(err); fatal != nil {
		return mcpExecutedCall{index: prepared.index, result: providerResult, err: fatal}
	}
	return mcpExecutedCall{index: prepared.index, result: providerResult}
}

func (runtime *mcpToolRuntime) executeSearch(
	ctx context.Context,
	events chan<- ProviderEvent,
	call ProviderToolCall,
	arguments map[string]any,
	validationFailure string,
	round int,
	callNumber int,
) (ProviderToolResult, error) {
	executionID := fmt.Sprintf("mcp-search-%d-%d", round, callNumber)
	query, _ := arguments["query"].(string)
	query = strings.Join(strings.Fields(query), " ")
	base := ProviderToolExecutionEvent{
		ExecutionID:    executionID,
		CallID:         call.ID,
		Name:           mcpclient.ToolSearchAlias,
		Status:         ProcessStepStatusRunning,
		CallStatus:     mcpclient.CallStatusRunning,
		Round:          round,
		Arguments:      redactMCPArguments(arguments),
		Mode:           "mcp",
		Classification: mcpclient.ClassificationRead,
	}
	if !sendToolExecutionEvent(ctx, events, base) {
		return ProviderToolResult{}, context.Canceled
	}
	if validationFailure != "" || query == "" || len(query) > maxMCPToolSearchQuery {
		base.Status = ProcessStepStatusFailed
		base.CallStatus = mcpclient.CallStatusFailed
		base.FailureCategory = "arguments_invalid"
		sendToolExecutionEvent(ctx, events, base)
		return ProviderToolResult{
			CallID: call.ID, Name: call.Name,
			Content: mcpFailureToolResult("arguments_invalid"), IsError: true,
		}, nil
	}
	tools := runtime.service.SearchTools(runtime.run, query)
	limit := max(runtime.service.Config().MaxExposedTools-1, 1)
	if len(tools) > limit {
		tools = tools[:limit]
	}
	runtime.query = query
	runtime.setVisible(tools, true)
	items := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		items = append(items, map[string]any{
			"alias": tool.Alias, "name": tool.Name, "title": tool.Title,
			"description": tool.Description, "classification": tool.Classification,
		})
	}
	encoded, err := json.Marshal(map[string]any{
		"untrustedMcpToolSearchResult": true,
		"tools":                        items,
	})
	if err != nil {
		return ProviderToolResult{}, &mcpRunFailure{code: "MCP_TOOL_FAILED", err: err}
	}
	base.Status = ProcessStepStatusCompleted
	base.CallStatus = mcpclient.CallStatusSucceeded
	if !sendToolExecutionEvent(ctx, events, base) {
		return ProviderToolResult{}, context.Canceled
	}
	return ProviderToolResult{CallID: call.ID, Name: call.Name, Content: string(encoded)}, nil
}

func (runtime *mcpToolRuntime) eventSink(events chan<- ProviderEvent) mcpclient.EventSink {
	return func(ctx context.Context, event mcpclient.ExecutionEvent) bool {
		status := ProcessStepStatusRunning
		switch event.Status {
		case mcpclient.CallStatusQueued:
			status = ProcessStepStatusPending
		case mcpclient.CallStatusSucceeded:
			status = ProcessStepStatusCompleted
		case mcpclient.CallStatusFailed:
			status = ProcessStepStatusFailed
		case mcpclient.CallStatusCanceled:
			status = ProcessStepStatusCancelled
		case mcpclient.CallStatusOutcomeUnknown:
			status = ProcessStepStatusOutcomeUnknown
		}
		return sendToolExecutionEvent(ctx, events, ProviderToolExecutionEvent{
			ExecutionID:     event.CallID,
			CallID:          event.CallID,
			Name:            event.ToolName,
			Status:          status,
			CallStatus:      event.Status,
			Server:          event.ServerRef.Key(),
			Classification:  event.Classification,
			Round:           event.Round,
			Arguments:       redactMCPArguments(event.Arguments),
			FailureCategory: event.FailureCategory,
			DurationMillis:  event.DurationMillis,
			Mode:            "mcp",
		})
	}
}

func decodeMCPArguments(raw string) (map[string]any, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, "arguments_invalid"
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil || arguments == nil {
		return map[string]any{}, "arguments_invalid"
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return map[string]any{}, "arguments_invalid"
	}
	return arguments, ""
}

func redactMCPArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return nil
	}
	redacted := make(map[string]any, len(arguments))
	for key, value := range arguments {
		switch value.(type) {
		case nil:
			redacted[key] = "null"
		case bool:
			redacted[key] = "boolean"
		case string:
			redacted[key] = "string"
		case json.Number, float64:
			redacted[key] = "number"
		case []any:
			redacted[key] = "array"
		case map[string]any:
			redacted[key] = "object"
		default:
			redacted[key] = "unknown"
		}
	}
	return redacted
}

func fatalMCPExecutionError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mcpclient.ErrOutcomeUnknown):
		return &mcpRunFailure{code: "MCP_OUTCOME_UNKNOWN", err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, mcpclient.ErrToolBudget):
		return &mcpRunFailure{code: "MCP_BUDGET_EXHAUSTED", err: err}
	case errors.Is(err, context.Canceled):
		return &mcpRunFailure{code: "MCP_CANCELED", err: err}
	case errors.Is(err, mcpclient.ErrCredentialRequired),
		errors.Is(err, mcpclient.ErrCredentialInvalid),
		errors.Is(err, mcpclient.ErrServerNeedsAuth):
		return &mcpRunFailure{code: "MCP_AUTH_REQUIRED", err: err}
	case errors.Is(err, mcpclient.ErrServerUnavailable),
		errors.Is(err, mcpclient.ErrServerNotReady),
		errors.Is(err, mcpclient.ErrRemoteDisabled),
		errors.Is(err, mcpclient.ErrStdioDisabled),
		errors.Is(err, mcpclient.ErrDisabled):
		return &mcpRunFailure{code: "MCP_SERVER_UNAVAILABLE", err: err}
	case errors.Is(err, mcpclient.ErrSelectionInvalid),
		errors.Is(err, mcpclient.ErrToolNotFound):
		return &mcpRunFailure{code: "MCP_AUTHORIZATION_FAILED", err: err}
	default:
		return nil
	}
}

func nonEmptyMCPFailure(category string, err error) string {
	if category = strings.TrimSpace(category); category != "" {
		return category
	}
	if errors.Is(err, mcpclient.ErrToolArgumentsInvalid) {
		return "arguments_invalid"
	}
	if errors.Is(err, mcpclient.ErrResponseTooLarge) {
		return "result_too_large"
	}
	return "tool_failed"
}

func mcpFailureToolResult(category string) string {
	encoded, _ := json.Marshal(map[string]any{
		"untrustedMcpToolResult": true,
		"isError":                true,
		"error":                  strings.TrimSpace(category),
	})
	return string(encoded)
}

func streamMCPFinalNoTools(
	ctx context.Context,
	events chan<- ProviderEvent,
	provider ToolRoundProvider,
	request ProviderRequest,
	continuation []ProviderToolExchange,
	completedUsage TokenUsage,
) {
	roundEvents, err := provider.StreamToolRound(ctx, ProviderRoundRequest{
		ProviderRequest: request,
		Tools:           nil,
		ToolChoice:      ProviderToolChoiceAuto,
		Continuation:    continuation,
	})
	if err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: err})
		return
	}
	for event := range roundEvents {
		if event.Type == ProviderEventToolCallDelta || event.Type == ProviderEventToolCallCompleted {
			continue
		}
		if event.Type == ProviderEventUsage && event.Usage != nil {
			event.Usage = addTokenUsage(completedUsage, *event.Usage)
		}
		if !sendProviderEvent(ctx, events, event) {
			return
		}
	}
}
