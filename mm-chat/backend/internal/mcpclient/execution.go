package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxModelResultBytes = 32 << 10

func (s *Service) Execute(
	ctx context.Context,
	userID string,
	run PreparedRun,
	input ExecuteInput,
	sink EventSink,
) (CallResult, error) {
	if err := s.available(); err != nil {
		return CallResult{}, err
	}
	if run.Snapshot.UserID != userID || run.Snapshot.RunID == "" || input.Round < 1 || input.Call < 1 {
		return CallResult{}, ErrSelectionInvalid
	}
	tool, ok := run.aliases[input.Alias]
	if !ok {
		return CallResult{}, ErrToolNotFound
	}
	server, ok := run.servers[tool.ServerRef.Key()]
	if !ok {
		return CallResult{}, ErrServerNotFound
	}
	now := s.now().UTC()
	call := CallRecord{
		ConversationID:   run.Snapshot.ConversationID,
		MessageID:        run.Snapshot.MessageID,
		RunID:            run.Snapshot.RunID,
		ServerRef:        tool.ServerRef,
		ToolName:         tool.Name,
		ToolAlias:        tool.Alias,
		Classification:   tool.Classification,
		Status:           CallStatusQueued,
		Round:            input.Round,
		Call:             input.Call,
		ArgumentsSummary: summarizeArguments(input.Arguments),
		RetainUntil:      now.Add(s.config.AuditRetention),
	}
	created, err := s.repo.CreateCall(ctx, userID, call)
	if err != nil {
		return CallResult{}, err
	}
	call = created
	s.emit(ctx, sink, eventFromCall(call, server.Name, input.Arguments))
	if strings.TrimSpace(input.ValidationFailure) != "" {
		return s.finishExecutionFailure(
			ctx, userID, call, server.Name, input.Arguments, sink, ErrToolArgumentsInvalid, false, now,
		)
	}
	if err := ValidateToolArguments(tool, input.Arguments); err != nil {
		return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, false, now)
	}
	release, err := s.acquireUserCall(ctx, userID)
	if err != nil {
		return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, false, now)
	}
	defer release()
	writeRelease := func() {}
	if tool.Classification != ClassificationRead {
		writeRelease, err = s.acquireUserWrite(ctx, userID)
		if err != nil {
			return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, false, now)
		}
	}
	defer writeRelease()
	call.Status = CallStatusRunning
	started := s.now().UTC()
	call.StartedAt = &started
	s.emit(ctx, sink, eventFromCall(call, server.Name, input.Arguments))

	callCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
	defer cancel()
	credential, err := s.connectionCredential(callCtx, userID, server)
	if err != nil {
		return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, false, started)
	}

	var result CallResult
	dispatched := false
	maximumAttempts := 1
	if tool.Classification == ClassificationRead {
		maximumAttempts = 2
	}
	for attempt := 1; attempt <= maximumAttempts; attempt++ {
		session, connectErr := s.connector.Connect(callCtx, server, credential)
		if connectErr != nil {
			err = connectErr
			if attempt < maximumAttempts && callCtx.Err() == nil {
				continue
			}
			break
		}
		dispatched = true
		result, err = session.CallTool(callCtx, tool.Name, input.Arguments)
		// The MCP result, when present, is decisive. A close failure must never
		// turn a completed write into an ambiguous automatic retry.
		_ = session.Close()
		if err == nil || attempt == maximumAttempts || callCtx.Err() != nil {
			break
		}
	}
	if err != nil {
		outcomeUnknown := dispatched && tool.Classification != ClassificationRead &&
			!errors.Is(err, ErrToolArgumentsInvalid)
		return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, outcomeUnknown, started)
	}

	bounded, objectKeys, byteSize, err := s.boundAndStoreResult(ctx, call, result)
	if err != nil {
		return s.finishExecutionFailure(ctx, userID, call, server.Name, input.Arguments, sink, err, false, started)
	}
	completed := s.now().UTC()
	call.CompletedAt = &completed
	call.DurationMillis = max(completed.Sub(started).Milliseconds(), 0)
	call.Status = CallStatusSucceeded
	if bounded.IsError {
		call.Status = CallStatusFailed
		call.ErrorCode = "tool_error"
	}
	call.ResultSummary = bounded.Summary
	if err := s.repo.FinishCall(ctx, userID, call, bounded.Content, objectKeys, byteSize); err != nil {
		for _, key := range objectKeys {
			_ = s.objects.Delete(context.WithoutCancel(ctx), key)
		}
		return CallResult{}, err
	}
	s.emit(ctx, sink, eventFromCall(call, server.Name, nil))
	return bounded, nil
}

func (s *Service) finishExecutionFailure(
	ctx context.Context,
	userID string,
	call CallRecord,
	serverName string,
	arguments map[string]any,
	sink EventSink,
	cause error,
	outcomeUnknown bool,
	started time.Time,
) (CallResult, error) {
	completed := s.now().UTC()
	if call.StartedAt == nil {
		value := started.UTC()
		call.StartedAt = &value
	}
	call.CompletedAt = &completed
	call.DurationMillis = max(completed.Sub(call.StartedAt.UTC()).Milliseconds(), 0)
	call.Status = CallStatusFailed
	call.ErrorCode = executionFailureCode(cause)
	if errors.Is(cause, context.Canceled) {
		call.Status = CallStatusCanceled
	}
	if outcomeUnknown {
		call.Status = CallStatusOutcomeUnknown
		call.ErrorCode = "outcome_unknown"
	}
	call.ResultSummary = call.ErrorCode
	finishCtx := ctx
	finishCancel := func() {}
	if ctx.Err() != nil {
		finishCtx, finishCancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	defer finishCancel()
	if err := s.repo.FinishCall(finishCtx, userID, call, nil, nil, 0); err != nil {
		return CallResult{}, err
	}
	event := eventFromCall(call, serverName, arguments)
	event.FailureCategory = call.ErrorCode
	s.emit(context.WithoutCancel(ctx), sink, event)
	if outcomeUnknown {
		return CallResult{FailureCategory: call.ErrorCode, OutcomeUnknown: true, IsError: true}, ErrOutcomeUnknown
	}
	return CallResult{FailureCategory: call.ErrorCode, IsError: true}, cause
}

func (s *Service) boundAndStoreResult(
	ctx context.Context,
	call CallRecord,
	result CallResult,
) (CallResult, []string, int64, error) {
	bounded := CallResult{IsError: result.IsError}
	objectKeys := []string{}
	var totalBytes int64
	var inlineBytes int64
	modelItems := make([]map[string]any, 0, len(result.Content))
	cleanup := func() {
		if s.objects == nil {
			return
		}
		for _, key := range objectKeys {
			_ = s.objects.Delete(context.WithoutCancel(ctx), key)
		}
	}
	for index, item := range result.Content {
		normalized, raw, err := normalizeResultItem(item)
		if err != nil {
			cleanup()
			return CallResult{}, nil, 0, err
		}
		itemBytes := int64(len(raw))
		if itemBytes > s.config.MaxResultItemBytes || totalBytes+itemBytes > s.config.MaxResultCallBytes {
			cleanup()
			return CallResult{}, nil, 0, ErrResponseTooLarge
		}
		totalBytes += itemBytes
		store := normalized.Type == "image" || normalized.Type == "audio" ||
			((normalized.Type == "text" || normalized.Type == "json" || normalized.Type == "resource") &&
				inlineBytes+itemBytes > s.config.MaxInlineResultBytes)
		if store && itemBytes > 0 {
			if s.objects == nil {
				cleanup()
				return CallResult{}, nil, 0, ErrServerUnavailable
			}
			key := fmt.Sprintf("mcp-results/%s/%s/%03d", call.ConversationID, call.ID, index+1)
			contentType := normalized.MIMEType
			if contentType == "" {
				if normalized.Type == "json" {
					contentType = "application/json"
				} else {
					contentType = "text/plain; charset=utf-8"
				}
			}
			if err := s.objects.Put(ctx, key, bytes.NewReader(raw), itemBytes, contentType); err != nil {
				cleanup()
				return CallResult{}, nil, 0, ErrServerUnavailable
			}
			objectKeys = append(objectKeys, key)
			normalized.ObjectKey = key
			normalized.Text = ""
			normalized.JSON = nil
			normalized.Data = nil
		} else {
			inlineBytes += itemBytes
		}
		bounded.Content = append(bounded.Content, normalized)
		modelItems = append(modelItems, modelProjection(item, raw))
	}
	modelEnvelope := map[string]any{
		"untrustedMcpToolResult": true,
		"isError":                result.IsError,
		"content":                modelItems,
	}
	encoded, err := json.Marshal(modelEnvelope)
	if err != nil {
		cleanup()
		return CallResult{}, nil, 0, ErrResponseTooLarge
	}
	if len(encoded) > maxModelResultBytes {
		encoded = encoded[:maxModelResultBytes]
		bounded.ModelContent = string(encoded) + `\n[truncated]`
	} else {
		bounded.ModelContent = string(encoded)
	}
	bounded.Summary = fmt.Sprintf("content_items=%d bytes=%d artifacts=%d", len(bounded.Content), totalBytes, len(objectKeys))
	return bounded, objectKeys, totalBytes, nil
}

func normalizeResultItem(item Content) (Content, []byte, error) {
	item.Type = strings.TrimSpace(item.Type)
	item.MIMEType = strings.TrimSpace(item.MIMEType)
	item.URI = strings.TrimSpace(item.URI)
	item.Name = strings.TrimSpace(item.Name)
	if len(item.MIMEType) > 256 || strings.ContainsAny(item.MIMEType, "\r\n") ||
		len(item.URI) > 8192 || len(item.Name) > 1024 {
		return Content{}, nil, ErrResponseTooLarge
	}
	var raw []byte
	switch item.Type {
	case "text":
		raw = []byte(item.Text)
	case "json":
		var err error
		raw, err = json.Marshal(objectOrEmpty(item.JSON))
		if err != nil {
			return Content{}, nil, ErrResponseTooLarge
		}
	case "image", "audio":
		if item.MIMEType == "" {
			return Content{}, nil, ErrResponseTooLarge
		}
		raw = append([]byte(nil), item.Data...)
	case "resource":
		if len(item.Data) > 0 {
			raw = append([]byte(nil), item.Data...)
		} else {
			raw = []byte(item.Text)
		}
	case "resource_link":
		// Resource Links remain metadata only. Neo Chat never fetches them.
		raw = nil
	default:
		return Content{}, nil, ErrResponseTooLarge
	}
	item.ByteSize = int64(len(raw))
	return item, raw, nil
}

func modelProjection(item Content, raw []byte) map[string]any {
	projection := map[string]any{"type": item.Type}
	switch item.Type {
	case "text", "resource":
		projection["text"] = boundedUTF8(string(raw), 8192)
	case "json":
		projection["json"] = boundedUTF8(string(raw), 8192)
	case "image", "audio":
		projection["mimeType"] = item.MIMEType
		projection["byteSize"] = len(raw)
		projection["note"] = "binary artifact omitted from model context"
	case "resource_link":
		projection["uri"] = item.URI
		projection["name"] = item.Name
		projection["note"] = "link not fetched"
	}
	return projection
}

func boundedUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value + "…"
}

func summarizeArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return map[string]any{}
	}
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 32 {
		keys = keys[:32]
	}
	summary := make(map[string]any, len(keys))
	for _, key := range keys {
		if len(key) > 256 {
			continue
		}
		summary[key] = jsonValueKind(arguments[key])
	}
	return summary
}

func jsonValueKind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func executionFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrToolArgumentsInvalid):
		return "arguments_invalid"
	case errors.Is(err, ErrCredentialRequired), errors.Is(err, ErrCredentialInvalid):
		return "authentication"
	case errors.Is(err, ErrResponseTooLarge):
		return "result_too_large"
	case errors.Is(err, ErrServerUnavailable):
		return "server_unavailable"
	default:
		return "tool_failed"
	}
}

func eventFromCall(call CallRecord, serverName string, arguments map[string]any) ExecutionEvent {
	return ExecutionEvent{
		CallID: call.ID, ServerRef: call.ServerRef,
		ServerName: boundedServerName(serverName), ToolName: call.ToolName,
		ToolAlias: call.ToolAlias, Classification: call.Classification,
		Status: call.Status, Round: call.Round, Call: call.Call,
		Arguments: summarizeArguments(arguments), DurationMillis: call.DurationMillis,
	}
}

func boundedServerName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxPrivateServerNameBytes {
		return value
	}
	return boundedUTF8(value, maxPrivateServerNameBytes-len("…"))
}

func (s *Service) emit(ctx context.Context, sink EventSink, event ExecutionEvent) bool {
	if sink == nil {
		return true
	}
	return sink(ctx, event)
}
