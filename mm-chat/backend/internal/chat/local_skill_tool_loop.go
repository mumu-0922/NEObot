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
	"sync"
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
	execution.Presentation = runtime.localProcessPresentation(call)
	if !sendToolExecutionEvent(ctx, events, execution) {
		return ProviderToolResult{}, context.Canceled
	}
	started := time.Now()
	result, failure, fatal := runtime.executeCall(ctx, events, call, &execution)
	execution.Presentation = completeLocalProcessPresentation(execution.Presentation, result)
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
	events chan<- ProviderEvent,
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
		var arguments localTerminalToolArguments
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
			execution.Presentation = terminalResultPresentation(
				execution.Presentation,
				nil,
			)
			job, err := runtime.executor.StartBackgroundJob(ctx, localskills.JobStartRequest{
				Scope: runtime.jobScope, Command: request,
			})
			if errors.Is(err, localskills.ErrApprovalRequired) && runtime.approvals != nil {
				approval, approvalErr := runtime.awaitTerminalApproval(ctx, events, execution)
				if approvalErr != nil {
					return ProviderToolResult{}, "", approvalErr
				}
				if !chatAgentApprovalAllowsExecution(approval) {
					category := approvalFailureCategory(approval)
					return localSkillFailureResult(call, category), category, nil
				}
				request.Approved = true
				job, err = runtime.executor.StartBackgroundJob(ctx, localskills.JobStartRequest{
					Scope: runtime.jobScope, Command: request,
				})
			}
			return backgroundJobToolResult(call, job, err)
		}
		liveOutput := newTerminalLiveOutput(
			ctx, events, *execution, runtime.executor, request,
		)
		request.OnOutput = liveOutput.append
		result, err := runtime.executor.Execute(ctx, request)
		if errors.Is(err, localskills.ErrApprovalRequired) && runtime.approvals != nil {
			approval, approvalErr := runtime.awaitTerminalApproval(ctx, events, execution)
			if approvalErr != nil {
				return ProviderToolResult{}, "", approvalErr
			}
			if !chatAgentApprovalAllowsExecution(approval) {
				category := approvalFailureCategory(approval)
				return localSkillFailureResult(call, category), category, nil
			}
			request.Approved = true
			result, err = runtime.executor.Execute(ctx, request)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ProviderToolResult{}, "", err
			}
			category := localTerminalFailureCategory(err)
			return localSkillFailureResult(call, category), category, nil
		}
		execution.Presentation = terminalResultPresentation(
			execution.Presentation,
			&result,
		)
		return localSkillSuccessResult(call, map[string]any{
			"exitCode": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr,
			"timedOut": result.TimedOut, "truncated": result.Truncated,
			"durationMillis": result.DurationMillis,
		}), "", nil
	default:
		return localSkillFailureResult(call, "tool_not_available"), "tool_not_available", nil
	}
}

func (runtime *localSkillToolRuntime) awaitTerminalApproval(
	ctx context.Context,
	events chan<- ProviderEvent,
	execution *ProviderToolExecutionEvent,
) (ChatAgentApproval, error) {
	approval, decisions, err := runtime.approvals.request(ctx, *execution, true)
	if err != nil {
		return ChatAgentApproval{}, fmt.Errorf("%w: %v", errChatAgentApprovalPersistence, err)
	}
	if approval.Status == ChatAgentApprovalPending {
		execution.Status = ProcessStepStatusAwaitingApproval
		execution.CallStatus = "waiting_approval"
		execution.Presentation = withProcessApprovalPresentation(
			execution.Presentation, approval,
		)
		if !sendToolExecutionEvent(ctx, events, *execution) {
			return ChatAgentApproval{}, context.Canceled
		}
		approval, err = runtime.approvals.wait(ctx, approval, decisions)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ChatAgentApproval{}, err
			}
			return ChatAgentApproval{}, fmt.Errorf("%w: %v", errChatAgentApprovalPersistence, err)
		}
	}
	execution.Presentation = withProcessApprovalPresentation(
		execution.Presentation, approval,
	)
	if chatAgentApprovalAllowsExecution(approval) {
		execution.Status = ProcessStepStatusRunning
		execution.CallStatus = "running"
		if !sendToolExecutionEvent(ctx, events, *execution) {
			return ChatAgentApproval{}, context.Canceled
		}
	}
	return approval, nil
}

func approvalFailureCategory(approval ChatAgentApproval) string {
	if approval.Status == ChatAgentApprovalExpired {
		return "approval_expired"
	}
	return "approval_denied"
}

type terminalLiveOutput struct {
	mu              sync.Mutex
	ctx             context.Context
	events          chan<- ProviderEvent
	execution       ProviderToolExecutionEvent
	executor        *localskills.Executor
	request         localskills.Request
	entries         []ProcessTranscriptEntry
	lastEmit        time.Time
	lastStableBytes int
}

func newTerminalLiveOutput(
	ctx context.Context,
	events chan<- ProviderEvent,
	execution ProviderToolExecutionEvent,
	executor *localskills.Executor,
	request localskills.Request,
) *terminalLiveOutput {
	return &terminalLiveOutput{
		ctx: ctx, events: events, execution: execution, executor: executor, request: request,
	}
}

func (output *terminalLiveOutput) append(chunk localskills.OutputChunk) {
	if output == nil || output.executor == nil || chunk.Content == "" ||
		(chunk.Stream != "stdout" && chunk.Stream != "stderr") {
		return
	}
	output.mu.Lock()
	if last := len(output.entries) - 1; last >= 0 && output.entries[last].Stream == chunk.Stream {
		output.entries[last].Content += chunk.Content
	} else {
		output.entries = append(output.entries, ProcessTranscriptEntry{
			Sequence: len(output.entries) + 1, Stream: chunk.Stream, Content: chunk.Content,
		})
	}
	stable := terminalStableTranscript(output.entries, processReasoningStreamHoldbackBytes)
	stableBytes := processTranscriptBytes(stable)
	now := time.Now()
	if stableBytes == 0 ||
		(stableBytes-output.lastStableBytes < 16<<10 && now.Sub(output.lastEmit) < 75*time.Millisecond) {
		output.mu.Unlock()
		return
	}
	for index := range stable {
		stable[index].Content = output.executor.RedactExecutionPaths(
			stable[index].Content,
			output.request.SkillsRoot,
			output.request.ActiveSkillRoot,
		)
	}
	presentation := cloneProcessStepPresentation(output.execution.Presentation)
	if presentation == nil {
		output.mu.Unlock()
		return
	}
	presentation.Transcript = stable
	presentation.Truncated = presentation.Truncated || stableBytes > maxProcessPresentationTextBytes
	execution := output.execution
	execution.Transient = true
	execution.Presentation = presentation
	output.lastEmit = now
	output.lastStableBytes = stableBytes
	output.mu.Unlock()

	event := ProviderEvent{Type: ProviderEventToolExecution, ToolExecution: &execution}
	select {
	case <-output.ctx.Done():
	case output.events <- event:
	default:
	}
}

func terminalStableTranscript(
	entries []ProcessTranscriptEntry,
	holdback int,
) []ProcessTranscriptEntry {
	stable := append([]ProcessTranscriptEntry(nil), entries...)
	for index := len(stable) - 1; index >= 0 && holdback > 0; index-- {
		length := len(stable[index].Content)
		if length <= holdback {
			holdback -= length
			stable = stable[:index]
			continue
		}
		stable[index].Content = truncateProcessUTF8(
			stable[index].Content, length-holdback,
		)
		holdback = 0
	}
	return stable
}

func processTranscriptBytes(entries []ProcessTranscriptEntry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.Content)
	}
	return total
}

func cloneProcessStepPresentation(
	presentation *ProcessStepPresentation,
) *ProcessStepPresentation {
	if presentation == nil {
		return nil
	}
	cloned := *presentation
	cloned.Transcript = append([]ProcessTranscriptEntry(nil), presentation.Transcript...)
	cloned.Items = append([]ProcessPresentationItem(nil), presentation.Items...)
	if presentation.Approval != nil {
		approval := *presentation.Approval
		cloned.Approval = &approval
	}
	if presentation.ExitCode != nil {
		exitCode := *presentation.ExitCode
		cloned.ExitCode = &exitCode
	}
	return &cloned
}

func withProcessApprovalPresentation(
	presentation *ProcessStepPresentation,
	approval ChatAgentApproval,
) *ProcessStepPresentation {
	completed := cloneProcessStepPresentation(presentation)
	if completed == nil {
		return nil
	}
	completed.Approval = &ProcessApprovalPresentation{
		ID: approval.ID, Revision: approval.Revision, Status: approval.Status,
		Decision: approval.Decision, ExpiresAt: formatTime(approval.ExpiresAt),
		AllowConversation: approval.AllowConversation,
	}
	return completed
}

type localTerminalToolArguments struct {
	Command         string `json:"command"`
	Skill           string `json:"skill"`
	WorkingDir      string `json:"workingDir"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	RunInBackground bool   `json:"runInBackground"`
}

func (runtime *localSkillToolRuntime) terminalProcessPresentation(
	call ProviderToolCall,
) *ProcessStepPresentation {
	if runtime == nil || runtime.executor == nil {
		return nil
	}
	var arguments localTerminalToolArguments
	if !decodeStrictToolArguments(call.Arguments, &arguments) {
		return nil
	}
	activeSkillRoot := ""
	if skillName := strings.TrimSpace(arguments.Skill); skillName != "" {
		if skill, ok := runtime.byName[skillName]; ok {
			activeSkillRoot = skill.RootPath
		}
	}
	command, cwd, ok := runtime.executor.TerminalPresentation(localskills.Request{
		Command: arguments.Command, WorkingDir: arguments.WorkingDir,
		TimeoutSeconds:  arguments.TimeoutSeconds,
		SkillsRoot:      runtime.config().RuntimeRoot,
		ActiveSkillRoot: activeSkillRoot,
	}, arguments.RunInBackground)
	if !ok {
		return nil
	}
	if arguments.RunInBackground {
		return &ProcessStepPresentation{
			Card: "job", Operation: "start", Command: command, CWD: cwd,
			Background: true,
		}
	}
	return &ProcessStepPresentation{
		Card: "terminal", Command: command, CWD: cwd,
	}
}

func terminalResultPresentation(
	presentation *ProcessStepPresentation,
	result *localskills.Result,
) *ProcessStepPresentation {
	if presentation == nil {
		return nil
	}
	completed := *presentation
	if presentation.Approval != nil {
		approval := *presentation.Approval
		completed.Approval = &approval
	}
	if presentation.ExitCode != nil {
		exitCode := *presentation.ExitCode
		completed.ExitCode = &exitCode
	}
	if result != nil {
		exitCode := result.ExitCode
		completed.ExitCode = &exitCode
		completed.TimedOut = result.TimedOut
		completed.Truncated = result.Truncated
	}
	return &completed
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
	if errors.Is(err, errChatAgentApprovalPersistence) {
		return "AGENT_APPROVAL_PERSISTENCE_FAILED"
	}
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
