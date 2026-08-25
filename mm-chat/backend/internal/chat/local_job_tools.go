package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func backgroundJobListDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name:        localJobListToolName,
		Description: "List process-local background Jobs owned by this user and conversation. Command text and output are omitted.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{}, "properties": map[string]any{},
		},
		Strict: true,
	}}
}

func backgroundJobOutputDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localJobOutputToolName,
		Description: "Read one owned background Job. Set wait=true to wait up to timeoutSeconds " +
			"without busy-polling. Completed output is bounded by the local runtime limit.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"jobId", "wait", "timeoutSeconds"},
			"properties": map[string]any{
				"jobId": map[string]any{"type": "string", "minLength": 36, "maxLength": 36},
				"wait":  map[string]any{"type": "boolean"},
				"timeoutSeconds": map[string]any{
					"type": []string{"integer", "null"}, "minimum": 1,
					"maximum": int(localskills.MaxJobOutputWait / time.Second),
				},
			},
		},
		Strict: true,
	}}
}

func backgroundJobKillDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name:        localJobKillToolName,
		Description: "Stop one owned process-local background Job and its complete process group.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"jobId"},
			"properties": map[string]any{
				"jobId": map[string]any{"type": "string", "minLength": 36, "maxLength": 36},
			},
		},
		Strict: true,
	}}
}

func (runtime *localSkillToolRuntime) executeBackgroundJobToolCall(
	ctx context.Context,
	call ProviderToolCall,
) (ProviderToolResult, string, error) {
	switch call.Name {
	case localJobListToolName:
		var arguments struct{}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		jobs, err := runtime.executor.ListBackgroundJobs(runtime.jobScope)
		return backgroundJobToolResult(call, map[string]any{
			"jobs": jobs, "durability": "process_local",
		}, err)
	case localJobOutputToolName:
		var arguments struct {
			JobID          string `json:"jobId"`
			Wait           bool   `json:"wait"`
			TimeoutSeconds int    `json:"timeoutSeconds"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		waitTimeout := time.Duration(arguments.TimeoutSeconds) * time.Second
		if arguments.Wait && waitTimeout == 0 {
			waitTimeout = localskills.MaxJobOutputWait
		}
		job, err := runtime.executor.BackgroundJobOutput(
			ctx, runtime.jobScope, arguments.JobID, arguments.Wait, waitTimeout,
		)
		if err != nil {
			return backgroundJobToolResult(call, job, err)
		}
		paths := runtime.pendingFilesForJob(job.ID)
		if job.Status == localskills.JobStatusCompleted && len(paths) > 0 {
			files, fileErr := runtime.captureWorkspaceFiles(ctx, paths)
			if fileErr != nil {
				category := workspaceToolFailureCategory(fileErr)
				if category == "file_not_found" {
					category = "output_file_not_found"
				}
				return localSkillFailureResult(call, category), category, nil
			}
			runtime.clearPendingJobFiles(job.ID)
			return backgroundJobToolResultWithWorkspaceFiles(call, job, files), "", nil
		}
		if job.Status == localskills.JobStatusFailed || job.Status == localskills.JobStatusKilled {
			runtime.clearPendingJobFiles(job.ID)
		}
		return backgroundJobToolResult(call, job, nil)
	case localJobKillToolName:
		var arguments struct {
			JobID string `json:"jobId"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		job, err := runtime.executor.KillBackgroundJob(runtime.jobScope, arguments.JobID)
		return backgroundJobToolResult(call, job, err)
	default:
		return localSkillFailureResult(call, "tool_not_available"), "tool_not_available", nil
	}
}

func backgroundJobToolResultWithWorkspaceFiles(
	call ProviderToolCall,
	job localskills.JobSnapshot,
	files []WorkspaceFileReference,
) ProviderToolResult {
	payload := map[string]any{
		"result": job, "durability": "process_local",
	}
	if len(files) > 0 {
		payload["workspaceFiles"] = files
	}
	return localSkillSuccessResult(call, payload)
}

func backgroundJobToolResult(
	call ProviderToolCall,
	payload any,
	err error,
) (ProviderToolResult, string, error) {
	if err == nil {
		return localSkillSuccessResult(call, map[string]any{
			"result": payload, "durability": "process_local",
		}), "", nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ProviderToolResult{}, "", err
	}
	category := backgroundJobToolFailureCategory(err)
	return localSkillFailureResult(call, category), category, nil
}

func backgroundJobToolFailureCategory(err error) string {
	switch {
	case errors.Is(err, localskills.ErrJobNotFound):
		return "job_not_found"
	case errors.Is(err, localskills.ErrJobLimit):
		return "job_limit_reached"
	case errors.Is(err, localskills.ErrJobScopeInvalid),
		errors.Is(err, localskills.ErrWorkspaceInvalidInput):
		return "arguments_invalid"
	case errors.Is(err, localskills.ErrExecutorClosed):
		return "runtime_closed"
	default:
		return localTerminalFailureCategory(err)
	}
}

func (runtime *localSkillToolRuntime) consumeJobCompletionPrompt() string {
	if !runtime.enabled() {
		return ""
	}
	notices := runtime.executor.ConsumeJobNotices(runtime.jobScope)
	if len(notices) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(map[string]any{
		"untrustedJobCompletionNotices": true,
		"durability":                    "process_local",
		"jobs":                          notices,
	})
	return `<background_job_completion_notices>` + string(encoded) +
		`</background_job_completion_notices>` +
		"\nUse job_output to inspect a Job only when its result is needed. Do not infer output from this notice."
}

func appendAgentFollowupPrompt(current, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if current == "" {
		return addition
	}
	if addition == "" {
		return current
	}
	return current + "\n\n" + addition
}
