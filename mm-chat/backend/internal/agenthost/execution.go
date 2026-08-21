package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	ToolTerminal     = "terminal"
	ToolFileRead     = "file_read"
	ToolFileWrite    = "file_write"
	ToolFileEdit     = "file_edit"
	ToolFileSearch   = "file_search"
	ToolArtifactRead = "artifact_read"
	ToolJobStart     = "job_start"
	ToolJobList      = "job_list"
	ToolJobOutput    = "job_output"
	ToolJobKill      = "job_kill"
	ToolJobNotices   = "job_notices"

	maxExecutionWorkspaces    = 128
	maxExecutionArtifactBytes = 50 << 20
)

type ExecutionConfig struct {
	SkillsRoot    string
	ShellPath     string
	ApprovalMode  string
	CallTimeout   time.Duration
	RunTimeout    time.Duration
	MaxOutput     int64
	MaxConcurrent int
	SandboxPath   string
}

type ExecutionManager struct {
	resolver  WorkspaceResolver
	config    ExecutionConfig
	mu        sync.Mutex
	executors map[string]*localskills.Executor
	modes     []PermissionMode
	closed    bool
}

type ToolExecutionError struct {
	Code string
	Err  error
}

func (failure *ToolExecutionError) Error() string {
	if failure == nil || failure.Err == nil {
		return "agent Host Tool execution failed"
	}
	return failure.Err.Error()
}

func (failure *ToolExecutionError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Err
}

func NewExecutionManager(
	resolver WorkspaceResolver,
	config ExecutionConfig,
) (*ExecutionManager, error) {
	config.SkillsRoot = filepath.Clean(strings.TrimSpace(config.SkillsRoot))
	config.ShellPath = filepath.Clean(strings.TrimSpace(config.ShellPath))
	config.ApprovalMode = strings.TrimSpace(config.ApprovalMode)
	info, err := os.Stat(config.SkillsRoot)
	if resolver == nil || err != nil || !info.IsDir() || !filepath.IsAbs(config.SkillsRoot) ||
		!filepath.IsAbs(config.ShellPath) ||
		(config.ApprovalMode != localskills.ApprovalSmart && config.ApprovalMode != localskills.ApprovalOff) ||
		config.CallTimeout < time.Second || config.CallTimeout > 10*time.Minute ||
		config.RunTimeout < config.CallTimeout || config.RunTimeout > 30*time.Minute ||
		config.MaxOutput < 1024 || config.MaxOutput > 8<<20 ||
		config.MaxConcurrent < 1 || config.MaxConcurrent > 32 {
		return nil, ErrWorkspacePathInvalid
	}
	resolvedSkillsRoot, err := filepath.EvalSymlinks(config.SkillsRoot)
	if err != nil || resolvedSkillsRoot != config.SkillsRoot {
		return nil, ErrWorkspacePathInvalid
	}
	sandbox, err := probeSandboxRuntime(config.SandboxPath)
	if err != nil {
		return nil, err
	}
	config.SandboxPath = sandbox.command
	return &ExecutionManager{
		resolver: resolver, config: config, executors: make(map[string]*localskills.Executor),
		modes: append([]PermissionMode(nil), sandbox.modes...),
	}, nil
}

func (manager *ExecutionManager) PermissionModes() []PermissionMode {
	if manager == nil {
		return []PermissionMode{}
	}
	return append([]PermissionMode(nil), manager.modes...)
}

func (manager *ExecutionManager) Execute(
	ctx context.Context,
	request ToolExecuteRequest,
) (any, error) {
	if !manager.supportsPermissionMode(request.PermissionMode) {
		return nil, executionFailure(localskills.ErrPermissionDenied)
	}
	executor, err := manager.executorFor(ctx, request.Workspace)
	if err != nil {
		return nil, err
	}
	switch request.Tool {
	case ToolTerminal:
		var arguments TerminalToolArguments
		if !decodeExecutionArguments(request.Arguments, &arguments) {
			return nil, executionFailure(localskills.ErrInvalidCommand)
		}
		input := localskills.Request{
			Command: arguments.Command, WorkingDir: arguments.WorkingDir,
			TimeoutSeconds: arguments.TimeoutSeconds,
		}
		input.Approved = request.Approved || request.PermissionMode == PermissionFullAccess
		input.PermissionMode = string(request.PermissionMode)
		input.SkillsRoot = manager.config.SkillsRoot
		if request.ActiveSkillRoot != "" {
			activeRoot, ok := manager.activeSkillRoot(request.ActiveSkillRoot)
			if !ok {
				return nil, executionFailure(localskills.ErrInvalidCommand)
			}
			input.ActiveSkillRoot = activeRoot
		}
		result, executeErr := executor.Execute(ctx, input)
		return result, executionFailure(executeErr)
	case ToolFileRead:
		var input localskills.FileReadRequest
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.ReadWorkspaceFile(ctx, input)
		return result, executionFailure(executeErr)
	case ToolFileWrite:
		if request.PermissionMode == PermissionReadOnly {
			return nil, executionFailure(localskills.ErrPermissionDenied)
		}
		var input localskills.FileWriteRequest
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.WriteWorkspaceFile(ctx, input)
		return result, executionFailure(executeErr)
	case ToolFileEdit:
		if request.PermissionMode == PermissionReadOnly {
			return nil, executionFailure(localskills.ErrPermissionDenied)
		}
		var input localskills.FileEditRequest
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.EditWorkspaceFile(ctx, input)
		return result, executionFailure(executeErr)
	case ToolFileSearch:
		var input localskills.FileSearchRequest
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.SearchWorkspaceFiles(ctx, input)
		return result, executionFailure(executeErr)
	case ToolArtifactRead:
		var input struct {
			Path     string `json:"path"`
			MaxBytes int64  `json:"maxBytes"`
		}
		if !decodeExecutionArguments(request.Arguments, &input) ||
			input.MaxBytes < 1 || input.MaxBytes > maxExecutionArtifactBytes {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.ReadWorkspaceArtifact(ctx, input.Path, input.MaxBytes)
		return result, executionFailure(executeErr)
	case ToolJobStart:
		if !validExecutionScope(request.Scope) {
			return nil, executionFailure(localskills.ErrJobScopeInvalid)
		}
		var arguments TerminalToolArguments
		if !decodeExecutionArguments(request.Arguments, &arguments) {
			return nil, executionFailure(localskills.ErrInvalidCommand)
		}
		input := localskills.Request{
			Command: arguments.Command, WorkingDir: arguments.WorkingDir,
			TimeoutSeconds: arguments.TimeoutSeconds,
		}
		input.Approved = request.Approved || request.PermissionMode == PermissionFullAccess
		input.PermissionMode = string(request.PermissionMode)
		input.SkillsRoot = manager.config.SkillsRoot
		if request.ActiveSkillRoot != "" {
			activeRoot, ok := manager.activeSkillRoot(request.ActiveSkillRoot)
			if !ok {
				return nil, executionFailure(localskills.ErrInvalidCommand)
			}
			input.ActiveSkillRoot = activeRoot
		}
		result, executeErr := executor.StartBackgroundJob(ctx, localskills.JobStartRequest{
			Scope: localJobScope(request.Scope), Command: input,
		})
		return result, executionFailure(executeErr)
	case ToolJobList:
		if !validExecutionScope(request.Scope) {
			return nil, executionFailure(localskills.ErrJobScopeInvalid)
		}
		if !emptyExecutionArguments(request.Arguments) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.ListBackgroundJobs(localJobScope(request.Scope))
		return result, executionFailure(executeErr)
	case ToolJobOutput:
		var input struct {
			JobID       string        `json:"jobId"`
			Wait        bool          `json:"wait"`
			WaitTimeout time.Duration `json:"waitTimeout"`
		}
		if !validExecutionScope(request.Scope) {
			return nil, executionFailure(localskills.ErrJobScopeInvalid)
		}
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.BackgroundJobOutput(
			ctx, localJobScope(request.Scope), input.JobID, input.Wait, input.WaitTimeout,
		)
		return result, executionFailure(executeErr)
	case ToolJobKill:
		var input struct {
			JobID string `json:"jobId"`
		}
		if !validExecutionScope(request.Scope) {
			return nil, executionFailure(localskills.ErrJobScopeInvalid)
		}
		if !decodeExecutionArguments(request.Arguments, &input) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		result, executeErr := executor.KillBackgroundJob(localJobScope(request.Scope), input.JobID)
		return result, executionFailure(executeErr)
	case ToolJobNotices:
		if !validExecutionScope(request.Scope) {
			return nil, executionFailure(localskills.ErrJobScopeInvalid)
		}
		if !emptyExecutionArguments(request.Arguments) {
			return nil, executionFailure(localskills.ErrWorkspaceInvalidInput)
		}
		return executor.ConsumeJobNotices(localJobScope(request.Scope)), nil
	default:
		return nil, &ToolExecutionError{Code: "TOOL_NOT_AVAILABLE", Err: localskills.ErrRuntimeFailed}
	}
}

func (manager *ExecutionManager) Close() error {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil
	}
	manager.closed = true
	executors := make([]*localskills.Executor, 0, len(manager.executors))
	for _, executor := range manager.executors {
		executors = append(executors, executor)
	}
	manager.mu.Unlock()
	var closeErr error
	for _, executor := range executors {
		closeErr = errors.Join(closeErr, executor.Close())
	}
	return closeErr
}

func (manager *ExecutionManager) executorFor(
	ctx context.Context,
	workspace ExecutionWorkspace,
) (*localskills.Executor, error) {
	if manager == nil || manager.resolver == nil ||
		!validWorkspacePathInput(workspace.CanonicalPath) ||
		!validExecutionFingerprint(workspace.DirectoryFingerprint) {
		return nil, &ToolExecutionError{Code: "WORKSPACE_AUTHORITY_INVALID", Err: ErrWorkspacePathInvalid}
	}
	descriptor, err := manager.resolver.ResolveWorkspace(ctx, workspace.CanonicalPath)
	if err != nil || descriptor.CanonicalPath != workspace.CanonicalPath ||
		descriptor.DirectoryFingerprint != workspace.DirectoryFingerprint {
		return nil, &ToolExecutionError{Code: "WORKSPACE_AUTHORITY_INVALID", Err: ErrWorkspacePathUnavailable}
	}
	key := descriptor.DirectoryFingerprint + "\x00" + descriptor.CanonicalPath
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return nil, executionFailure(localskills.ErrExecutorClosed)
	}
	if executor := manager.executors[key]; executor != nil {
		return executor, nil
	}
	if len(manager.executors) >= maxExecutionWorkspaces {
		return nil, executionFailure(localskills.ErrRuntimeBusy)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeMode: localskills.RuntimeHostWorkspace,
		RuntimeRoot:   manager.config.SkillsRoot,
		WorkspaceRoot: descriptor.CanonicalPath, ShellPath: manager.config.ShellPath,
		ApprovalMode: manager.config.ApprovalMode, CallTimeout: manager.config.CallTimeout,
		RunTimeout: manager.config.RunTimeout, MaxOutput: manager.config.MaxOutput,
		MaxCalls: 128, MaxRounds: 32, MaxConcurrent: manager.config.MaxConcurrent,
		SandboxCommand: manager.config.SandboxPath,
	})
	if err != nil {
		return nil, &ToolExecutionError{Code: "HOST_EXECUTION_UNAVAILABLE", Err: err}
	}
	manager.executors[key] = executor
	return executor, nil
}

func (manager *ExecutionManager) supportsPermissionMode(mode PermissionMode) bool {
	for _, available := range manager.modes {
		if available == mode {
			return true
		}
	}
	return false
}

func (manager *ExecutionManager) activeSkillRoot(value string) (string, bool) {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "." || filepath.IsAbs(value) || strings.HasPrefix(value, "..") ||
		strings.ContainsRune(value, '\x00') {
		return "", false
	}
	root := filepath.Join(manager.config.SkillsRoot, value)
	relative, err := filepath.Rel(manager.config.SkillsRoot, root)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return "", false
	}
	info, err := os.Stat(resolved)
	return resolved, err == nil && info.IsDir()
}

func decodeExecutionArguments(raw json.RawMessage, output any) bool {
	return len(raw) > 0 && strictjson.Decode(raw, int(maxExecutionRequestBytes), output) == nil
}

func emptyExecutionArguments(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return strictjson.Decode(raw, int(maxExecutionRequestBytes), &value) == nil && len(value) == 0
}

func localJobScope(scope ExecutionScope) localskills.JobScope {
	return localskills.JobScope{
		UserID: scope.UserID, ConversationID: scope.ConversationID,
	}
}

func validExecutionScope(scope ExecutionScope) bool {
	for _, value := range []string{scope.UserID, scope.ConversationID} {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 || strings.ContainsRune(value, '\x00') {
			return false
		}
		for _, character := range value {
			if character < 0x20 || character == 0x7f {
				return false
			}
		}
	}
	return true
}

func validExecutionFingerprint(value string) bool {
	digest := strings.TrimPrefix(value, "sha256:")
	if len(value) != 71 || len(digest) != 64 || digest != strings.ToLower(digest) {
		return false
	}
	for _, character := range digest {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func executionFailure(err error) error {
	if err == nil {
		return nil
	}
	code := "EXECUTION_FAILED"
	switch {
	case errors.Is(err, localskills.ErrApprovalRequired):
		code = "APPROVAL_REQUIRED"
	case errors.Is(err, localskills.ErrCommandBlocked):
		code = "COMMAND_BLOCKED"
	case errors.Is(err, localskills.ErrPermissionDenied):
		code = "PERMISSION_DENIED"
	case errors.Is(err, localskills.ErrInvalidCommand),
		errors.Is(err, localskills.ErrWorkspaceInvalidInput):
		code = "ARGUMENTS_INVALID"
	case errors.Is(err, localskills.ErrRuntimeBusy), errors.Is(err, localskills.ErrJobLimit):
		code = "RUNTIME_BUSY"
	case errors.Is(err, localskills.ErrJobNotFound):
		code = "JOB_NOT_FOUND"
	case errors.Is(err, localskills.ErrJobScopeInvalid):
		code = "JOB_SCOPE_INVALID"
	case errors.Is(err, localskills.ErrWorkspaceFileNotFound):
		code = "FILE_NOT_FOUND"
	case errors.Is(err, localskills.ErrWorkspaceFileTooLarge):
		code = "FILE_TOO_LARGE"
	case errors.Is(err, localskills.ErrWorkspaceInvalidUTF8):
		code = "INVALID_UTF8"
	case errors.Is(err, localskills.ErrWorkspaceVersionConflict):
		code = "VERSION_CONFLICT"
	case errors.Is(err, localskills.ErrWorkspaceEditConflict):
		code = "EDIT_CONFLICT"
	case errors.Is(err, localskills.ErrWorkspaceInvalidPath):
		code = "PATH_INVALID"
	case errors.Is(err, localskills.ErrExecutorClosed):
		code = "HOST_EXECUTION_UNAVAILABLE"
	}
	return &ToolExecutionError{Code: code, Err: err}
}
