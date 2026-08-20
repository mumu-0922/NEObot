package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

func executeLocalSkillBatch(
	ctx context.Context,
	events chan<- ProviderEvent,
	runtime *localSkillToolRuntime,
	calls []ProviderToolCall,
	round int,
) (map[int]ProviderToolResult, bool, error) {
	results := make(map[int]ProviderToolResult)
	if !runtime.enabled() {
		return results, false, nil
	}
	type preparedCall struct {
		index      int
		call       ProviderToolCall
		callNumber int
	}
	type executedCall struct {
		index  int
		result ProviderToolResult
		err    error
	}

	budgetReached := false
	prepared := make([]preparedCall, 0, len(calls))
	for index, call := range calls {
		if !runtime.handles(call.Name) {
			continue
		}
		if runtime.calls >= runtime.config().MaxCalls {
			budgetReached = true
			results[index] = localSkillFailureResult(call, "budget_exhausted")
			continue
		}
		runtime.calls++
		prepared = append(prepared, preparedCall{
			index: index, call: call, callNumber: runtime.calls,
		})
	}

	for offset := 0; offset < len(prepared); {
		if !localSkillToolAllowsParallel(prepared[offset].call.Name) {
			current := prepared[offset]
			result, err := runtime.execute(
				ctx, events, current.call, round, current.callNumber,
			)
			results[current.index] = result
			if err != nil {
				return results, budgetReached, err
			}
			offset++
			continue
		}

		end := offset + 1
		for end < len(prepared) && end-offset < maxParallelChatReadTools &&
			localSkillToolAllowsParallel(prepared[end].call.Name) {
			end++
		}
		groupCtx, cancel := context.WithCancel(ctx)
		completed := make(chan executedCall, end-offset)
		for _, current := range prepared[offset:end] {
			current := current
			go func() {
				result, err := runtime.execute(
					groupCtx, events, current.call, round, current.callNumber,
				)
				completed <- executedCall{
					index: current.index, result: result, err: err,
				}
			}()
		}
		var fatal error
		for count := offset; count < end; count++ {
			current := <-completed
			results[current.index] = current.result
			if current.err != nil && fatal == nil {
				fatal = current.err
				cancel()
			}
		}
		cancel()
		if fatal != nil {
			return results, budgetReached, fatal
		}
		offset = end
	}
	return results, budgetReached, nil
}

func localSkillToolAllowsParallel(name string) bool {
	switch strings.TrimSpace(name) {
	case localFileReadToolName, localFileSearchToolName,
		localJobListToolName, localJobOutputToolName,
		legacySkillViewToolName:
		return true
	default:
		return false
	}
}

// executeRequiredLocalSkillBatch is the fail-closed prelude boundary. A
// Provider may return a name that was not offered, so every non-skill call is
// converted to a Tool error before MCP, retrieval, or terminal dispatch.
func executeRequiredLocalSkillBatch(
	ctx context.Context,
	events chan<- ProviderEvent,
	runtime *localSkillToolRuntime,
	calls []ProviderToolCall,
	round int,
) (map[int]ProviderToolResult, bool, error) {
	results := make(map[int]ProviderToolResult, len(calls))
	if !runtime.enabled() || !runtime.requiresSkillLoad() {
		return results, false, nil
	}
	requiredName := runtime.requiredSkillName()
	budgetReached := false
	for index, call := range calls {
		if runtime.calls >= runtime.config().MaxCalls {
			budgetReached = true
			results[index] = localSkillFailureResult(call, "budget_exhausted")
			continue
		}
		runtime.calls++
		if strings.TrimSpace(call.Name) != localSkillToolName {
			results[index] = localSkillFailureResult(call, "skill_required_before_action")
			continue
		}
		var arguments struct {
			Name string `json:"name"`
		}
		if decodeStrictToolArguments(call.Arguments, &arguments) &&
			strings.TrimSpace(arguments.Name) != requiredName {
			results[index] = localSkillFailureResult(call, "skill_required_before_action")
			continue
		}
		result, err := runtime.execute(ctx, events, call, round, runtime.calls)
		results[index] = result
		if err != nil {
			return results, budgetReached, err
		}
	}
	return results, budgetReached, nil
}

func (runtime *localSkillToolRuntime) execute(
	ctx context.Context,
	events chan<- ProviderEvent,
	call ProviderToolCall,
	round, callNumber int,
) (ProviderToolResult, error) {
	name := strings.TrimSpace(call.Name)
	execution := ProviderToolExecutionEvent{
		ExecutionID:    fmt.Sprintf("local-skill-%d-%d", round, callNumber),
		CallID:         call.ID,
		Name:           name,
		Status:         ProcessStepStatusRunning,
		CallStatus:     "running",
		Round:          round,
		Mode:           "local_direct",
		Classification: localSkillClassification(name),
	}
	if !sendToolExecutionEvent(ctx, events, execution) {
		return ProviderToolResult{}, context.Canceled
	}
	started := time.Now()
	result, failure, fatal := runtime.executeCall(ctx, call, &execution)
	execution.DurationMillis = max(time.Since(started).Milliseconds(), 0)
	if fatal != nil {
		execution.Status = ProcessStepStatusCancelled
		execution.CallStatus = "canceled"
		if errors.Is(fatal, context.DeadlineExceeded) {
			execution.Status = ProcessStepStatusFailed
			execution.CallStatus = "failed"
			execution.FailureCategory = "run_timeout"
		}
		sendLocalSkillTerminalEvent(events, execution)
		return result, &localSkillRunFailure{code: localSkillFatalCode(fatal), err: fatal}
	}
	if failure != "" {
		execution.Status = ProcessStepStatusFailed
		execution.CallStatus = "failed"
		execution.FailureCategory = failure
	} else {
		execution.Status = ProcessStepStatusCompleted
		execution.CallStatus = "succeeded"
	}
	if !sendToolExecutionEvent(ctx, events, execution) {
		return ProviderToolResult{}, context.Canceled
	}
	return result, nil
}

func sendLocalSkillTerminalEvent(
	events chan<- ProviderEvent,
	execution ProviderToolExecutionEvent,
) {
	if events == nil {
		return
	}
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case events <- ProviderEvent{Type: ProviderEventToolExecution, ToolExecution: &execution}:
	case <-timer.C:
	}
}

func (runtime *localSkillToolRuntime) executeCall(
	ctx context.Context,
	call ProviderToolCall,
	execution *ProviderToolExecutionEvent,
) (ProviderToolResult, string, error) {
	if strings.TrimSpace(call.FailureCategory) != "" {
		return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
	}
	switch strings.TrimSpace(call.Name) {
	case localFileReadToolName, localFileWriteToolName, localFileEditToolName,
		localFileSearchToolName, localPublishFileToolName:
		return runtime.executeWorkspaceToolCall(ctx, call)
	case localJobListToolName, localJobOutputToolName, localJobKillToolName:
		execution.Durability = "process_local"
		return runtime.executeBackgroundJobToolCall(ctx, call)
	case localSkillToolName:
		var arguments struct {
			Name string `json:"name"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		arguments.Name = strings.TrimSpace(arguments.Name)
		skill, ok := runtime.byName[arguments.Name]
		if !ok {
			return localSkillFailureResult(call, "skill_not_found"), "skill_not_found", nil
		}
		if runtime.loaded[arguments.Name] == runtime.catalogRevision {
			return localSkillSuccessResult(call, map[string]any{
				"name": arguments.Name, "catalogRevision": runtime.catalogRevision,
				"alreadyLoaded": true,
			}), "", nil
		}
		body, err := skillsupply.ReadRuntimeSkillFile(skill, "SKILL.md")
		if err != nil {
			category := "skill_not_found"
			if errors.Is(err, skillsupply.ErrPackageChanged) ||
				errors.Is(err, skillsupply.ErrRuntimeUnavailable) {
				category = "package_drift"
			}
			return localSkillFailureResult(call, category), category, nil
		}
		if !utf8.Valid(body) {
			return localSkillFailureResult(call, "package_drift"), "package_drift", nil
		}
		runtime.loaded[arguments.Name] = runtime.catalogRevision
		return localSkillSuccessResult(call, map[string]any{
			"name": arguments.Name, "version": skill.Version,
			"catalogRevision": runtime.catalogRevision, "content": string(body),
		}), "", nil
	case legacySkillsListToolName:
		var arguments struct{}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		items := make([]map[string]any, 0, min(len(runtime.skills), maxLocalSkillListItems))
		for index, skill := range runtime.skills {
			if index >= maxLocalSkillListItems {
				break
			}
			items = append(items, map[string]any{
				"name":        truncateProcessUTF8(skill.Name, 128),
				"version":     truncateProcessUTF8(skill.Version, 128),
				"description": truncateProcessUTF8(skill.Description, maxLocalSkillDescriptionBytes),
				"fileCount":   len(skill.Files),
			})
		}
		return localSkillSuccessResult(call, map[string]any{
			"skills": items, "shown": len(items), "total": len(runtime.skills),
		}), "", nil
	case legacySkillViewToolName:
		var arguments struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		arguments.Name = strings.TrimSpace(arguments.Name)
		arguments.Path = strings.TrimSpace(arguments.Path)
		if arguments.Path == "" {
			arguments.Path = "SKILL.md"
		}
		skill, ok := runtime.byName[arguments.Name]
		if !ok || !localSkillViewPathAllowed(arguments.Path) {
			return localSkillFailureResult(call, "skill_or_file_not_found"), "skill_or_file_not_found", nil
		}
		body, err := skillsupply.ReadRuntimeSkillFile(skill, arguments.Path)
		if err != nil {
			category := "skill_or_file_not_found"
			if errors.Is(err, skillsupply.ErrPackageChanged) ||
				errors.Is(err, skillsupply.ErrRuntimeUnavailable) {
				category = "package_drift"
			}
			return localSkillFailureResult(call, category), category, nil
		}
		encoding := "utf-8"
		content := string(body)
		if !utf8.Valid(body) {
			encoding = "base64"
			content = base64.StdEncoding.EncodeToString(body)
		}
		return localSkillSuccessResult(call, map[string]any{
			"name": arguments.Name, "path": arguments.Path,
			"encoding": encoding, "content": content,
		}), "", nil
	case localTerminalToolName:
		var arguments struct {
			Command         string `json:"command"`
			Skill           string `json:"skill"`
			WorkingDir      string `json:"workingDir"`
			TimeoutSeconds  int    `json:"timeoutSeconds"`
			RunInBackground bool   `json:"runInBackground"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		if arguments.TimeoutSeconds > 0 {
			execution.Arguments = map[string]any{"timeoutSeconds": arguments.TimeoutSeconds}
		}
		activeSkillRoot := ""
		if arguments.Skill = strings.TrimSpace(arguments.Skill); arguments.Skill != "" {
			skill, ok := runtime.byName[arguments.Skill]
			if !ok {
				return localSkillFailureResult(call, "skill_not_found"), "skill_not_found", nil
			}
			if err := skillsupply.ValidateRuntimeSkill(skill); err != nil {
				return localSkillFailureResult(call, "package_drift"), "package_drift", nil
			}
			activeSkillRoot = skill.RootPath
		}
		request := localskills.Request{
			Command: arguments.Command, WorkingDir: arguments.WorkingDir,
			TimeoutSeconds:  arguments.TimeoutSeconds,
			SkillsRoot:      runtime.config().RuntimeRoot,
			ActiveSkillRoot: activeSkillRoot,
		}
		if arguments.RunInBackground {
			execution.Durability = "process_local"
			job, err := runtime.executor.StartBackgroundJob(ctx, localskills.JobStartRequest{
				Scope: runtime.jobScope, Command: request,
			})
			return backgroundJobToolResult(call, job, err)
		}
		result, err := runtime.executor.Execute(ctx, request)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ProviderToolResult{}, "", err
			}
			category := localTerminalFailureCategory(err)
			return localSkillFailureResult(call, category), category, nil
		}
		return localSkillSuccessResult(call, map[string]any{
			"exitCode": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr,
			"timedOut": result.TimedOut, "truncated": result.Truncated,
			"durationMillis": result.DurationMillis,
		}), "", nil
	default:
		return localSkillFailureResult(call, "tool_not_available"), "tool_not_available", nil
	}
}

func decodeStrictToolArguments(raw string, destination any) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxToolParamsBytes {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func localSkillViewPathAllowed(value string) bool {
	value = strings.TrimSpace(value)
	return value == "SKILL.md" ||
		(path.Clean(value) == value &&
			(strings.HasPrefix(value, "scripts/") || strings.HasPrefix(value, "references/") ||
				strings.HasPrefix(value, "assets/")))
}

func localSkillClassification(name string) string {
	switch name {
	case localTerminalToolName:
		return "execute"
	case localFileWriteToolName, localFileEditToolName, localPublishFileToolName:
		return "write"
	case localJobKillToolName:
		return "execute"
	default:
		return "read"
	}
}

func localTerminalFailureCategory(err error) string {
	switch {
	case errors.Is(err, localskills.ErrCommandBlocked):
		return "command_blocked"
	case errors.Is(err, localskills.ErrApprovalRequired):
		return "approval_required"
	case errors.Is(err, localskills.ErrRuntimeBusy):
		return "runtime_busy"
	case errors.Is(err, localskills.ErrInvalidCommand):
		return "arguments_invalid"
	default:
		return "execution_failed"
	}
}

func localSkillFatalCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "LOCAL_SKILL_BUDGET_EXHAUSTED"
	}
	return "LOCAL_SKILL_CANCELED"
}

func localSkillSuccessResult(call ProviderToolCall, payload map[string]any) ProviderToolResult {
	payload["evidenceToolCallId"] = strings.TrimSpace(call.ID)
	payload["untrustedLocalSkillResult"] = true
	encoded, _ := json.Marshal(payload)
	if len(encoded) > maxLocalSkillToolResultMetadata && call.Name != localSkillToolName &&
		call.Name != legacySkillViewToolName &&
		call.Name != localTerminalToolName && call.Name != localJobOutputToolName {
		return localSkillFailureResult(call, "result_too_large")
	}
	return ProviderToolResult{CallID: call.ID, Name: call.Name, Content: string(encoded)}
}

func localSkillFailureResult(call ProviderToolCall, category string) ProviderToolResult {
	encoded, _ := json.Marshal(map[string]any{
		"untrustedLocalSkillResult": true,
		"isError":                   true,
		"error":                     strings.TrimSpace(category),
	})
	return ProviderToolResult{
		CallID: call.ID, Name: call.Name, Content: string(encoded), IsError: true,
	}
}
