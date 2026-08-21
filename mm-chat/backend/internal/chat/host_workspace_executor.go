package chat

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/hostworkspace"
	"neo-chat/mm-chat/backend/internal/localskills"
)

var (
	errHostExecutionUnavailable = errors.New("Host Workspace execution is unavailable")
	errHostWorkspaceBinding     = errors.New("Host Workspace binding is unavailable")
)

type hostWorkspaceExecutionService interface {
	HostStatus(context.Context) hostworkspace.HostStatus
	LockConversationExecutionWorkspace(context.Context, string, string) (hostworkspace.ExecutionBinding, error)
	ExecuteTool(context.Context, agenthost.ToolExecuteRequest, any) error
	SetConversationPermission(context.Context, string, agenthost.PermissionMode, bool) error
}

type hostWorkspaceExecutor struct {
	service hostWorkspaceExecutionService
	binding hostworkspace.ExecutionBinding
	config  localskills.Config
}

func (h *Handler) localToolExecutorForConversation(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) (localToolExecutor, error) {
	if h == nil || h.localSkillExecutor == nil || !h.localSkillExecutor.Enabled() {
		return nil, errHostExecutionUnavailable
	}
	if strings.TrimSpace(workspaceID) == "" {
		return h.localSkillExecutor, nil
	}
	if h.hostWorkspaceService == nil {
		return nil, errHostExecutionUnavailable
	}
	status := h.hostWorkspaceService.HostStatus(ctx)
	if status.Status != "ready" || !status.Features.Execution {
		return nil, errHostExecutionUnavailable
	}
	binding, err := h.hostWorkspaceService.LockConversationExecutionWorkspace(
		ctx, conversationID, workspaceID,
	)
	if err != nil || binding.RunnerID != status.RunnerID {
		return nil, errHostWorkspaceBinding
	}
	if !hostPermissionModeAvailable(status.Features.PermissionModes, binding.PermissionMode) {
		return nil, errHostExecutionUnavailable
	}
	executor := newHostWorkspaceExecutor(
		h.hostWorkspaceService, binding, h.localSkillExecutor.Config(),
	)
	if executor == nil {
		return nil, errHostExecutionUnavailable
	}
	return executor, nil
}

func newHostWorkspaceExecutor(
	service hostWorkspaceExecutionService,
	binding hostworkspace.ExecutionBinding,
	config localskills.Config,
) *hostWorkspaceExecutor {
	if service == nil || binding.CanonicalPath == "" || binding.DirectoryFingerprint == "" {
		return nil
	}
	config.RuntimeMode = localskills.RuntimeHostWorkspace
	config.WorkspaceRoot = ""
	config.WorkspaceHostRoot = ""
	if binding.PermissionMode == agenthost.PermissionFullAccess {
		config.ApprovalMode = localskills.ApprovalOff
	}
	return &hostWorkspaceExecutor{service: service, binding: binding, config: config}
}

func (executor *hostWorkspaceExecutor) Enabled() bool {
	return executor != nil && executor.service != nil
}
func (executor *hostWorkspaceExecutor) Config() localskills.Config {
	if executor == nil {
		return localskills.Config{}
	}
	return executor.config
}

func (executor *hostWorkspaceExecutor) Execute(
	ctx context.Context,
	request localskills.Request,
) (localskills.Result, error) {
	callback := request.OnOutput
	request.OnOutput = nil
	activeRoot, ok := hostActiveSkillRoot(request.SkillsRoot, request.ActiveSkillRoot)
	if !ok {
		return localskills.Result{}, localskills.ErrInvalidCommand
	}
	request.SkillsRoot = ""
	request.ActiveSkillRoot = ""
	var result localskills.Result
	err := executor.call(ctx, agenthost.ToolTerminal, agenthost.TerminalToolArguments{
		Command: request.Command, WorkingDir: request.WorkingDir,
		TimeoutSeconds: request.TimeoutSeconds,
	}, request.Approved, activeRoot, &result)
	if err == nil && callback != nil {
		sequence := 0
		if result.Stdout != "" {
			sequence++
			callback(localskills.OutputChunk{Sequence: sequence, Stream: "stdout", Content: result.Stdout})
		}
		if result.Stderr != "" {
			sequence++
			callback(localskills.OutputChunk{Sequence: sequence, Stream: "stderr", Content: result.Stderr})
		}
	}
	return result, err
}

func (executor *hostWorkspaceExecutor) StartBackgroundJob(
	ctx context.Context,
	request localskills.JobStartRequest,
) (localskills.JobSnapshot, error) {
	activeRoot, ok := hostActiveSkillRoot(request.Command.SkillsRoot, request.Command.ActiveSkillRoot)
	if !ok {
		return localskills.JobSnapshot{}, localskills.ErrInvalidCommand
	}
	request.Command.OnOutput = nil
	request.Command.SkillsRoot = ""
	request.Command.ActiveSkillRoot = ""
	var result localskills.JobSnapshot
	err := executor.callWithScope(
		ctx, request.Scope, agenthost.ToolJobStart, agenthost.TerminalToolArguments{
			Command: request.Command.Command, WorkingDir: request.Command.WorkingDir,
			TimeoutSeconds: request.Command.TimeoutSeconds,
		},
		request.Command.Approved, activeRoot, &result,
	)
	return result, err
}

func (executor *hostWorkspaceExecutor) ListBackgroundJobs(
	scope localskills.JobScope,
) ([]localskills.JobSnapshot, error) {
	result := []localskills.JobSnapshot{}
	err := executor.callWithScope(context.Background(), scope, agenthost.ToolJobList, struct{}{}, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) BackgroundJobOutput(
	ctx context.Context,
	scope localskills.JobScope,
	jobID string,
	wait bool,
	waitTimeout time.Duration,
) (localskills.JobSnapshot, error) {
	var result localskills.JobSnapshot
	err := executor.callWithScope(ctx, scope, agenthost.ToolJobOutput, struct {
		JobID       string        `json:"jobId"`
		Wait        bool          `json:"wait"`
		WaitTimeout time.Duration `json:"waitTimeout"`
	}{jobID, wait, waitTimeout}, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) KillBackgroundJob(
	scope localskills.JobScope,
	jobID string,
) (localskills.JobSnapshot, error) {
	var result localskills.JobSnapshot
	err := executor.callWithScope(context.Background(), scope, agenthost.ToolJobKill, struct {
		JobID string `json:"jobId"`
	}{jobID}, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) ConsumeJobNotices(scope localskills.JobScope) []localskills.JobNotice {
	result := []localskills.JobNotice{}
	_ = executor.callWithScope(context.Background(), scope, agenthost.ToolJobNotices, struct{}{}, false, "", &result)
	return result
}

func (executor *hostWorkspaceExecutor) ReadWorkspaceFile(ctx context.Context, input localskills.FileReadRequest) (localskills.FileReadResult, error) {
	var result localskills.FileReadResult
	err := executor.call(ctx, agenthost.ToolFileRead, input, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) WriteWorkspaceFile(ctx context.Context, input localskills.FileWriteRequest) (localskills.FileWriteResult, error) {
	var result localskills.FileWriteResult
	err := executor.call(ctx, agenthost.ToolFileWrite, input, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) EditWorkspaceFile(ctx context.Context, input localskills.FileEditRequest) (localskills.FileWriteResult, error) {
	var result localskills.FileWriteResult
	err := executor.call(ctx, agenthost.ToolFileEdit, input, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) SearchWorkspaceFiles(ctx context.Context, input localskills.FileSearchRequest) (localskills.FileSearchResult, error) {
	var result localskills.FileSearchResult
	err := executor.call(ctx, agenthost.ToolFileSearch, input, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) ReadWorkspaceArtifact(ctx context.Context, path string, maxBytes int64) (localskills.WorkspaceArtifactSnapshot, error) {
	var result localskills.WorkspaceArtifactSnapshot
	err := executor.call(ctx, agenthost.ToolArtifactRead, struct {
		Path     string `json:"path"`
		MaxBytes int64  `json:"maxBytes"`
	}{path, maxBytes}, false, "", &result)
	return result, err
}

func (executor *hostWorkspaceExecutor) TerminalPresentation(
	request localskills.Request,
	_ bool,
) (string, string, bool) {
	command := strings.TrimSpace(request.Command)
	if command == "" || len(command) > 64<<10 || !utf8.ValidString(command) ||
		strings.ContainsRune(command, '\x00') {
		return "", "", false
	}
	for _, character := range command {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return "", "", false
		}
	}
	cwd := strings.TrimSpace(request.WorkingDir)
	if cwd == "" {
		cwd = "$NEO_CHAT_WORKSPACE"
	} else {
		clean := filepath.Clean(cwd)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", "", false
		}
		cwd = "$NEO_CHAT_WORKSPACE/" + filepath.ToSlash(clean)
	}
	return executor.RedactExecutionPaths(command, request.SkillsRoot, request.ActiveSkillRoot), cwd, true
}

func (executor *hostWorkspaceExecutor) RedactExecutionPaths(value, skillsRoot, activeSkillRoot string) string {
	for _, replacement := range []struct{ path, label string }{
		{activeSkillRoot, "$NEO_CHAT_ACTIVE_SKILL_ROOT"},
		{skillsRoot, "<skill-cache>"},
		{executor.binding.CanonicalPath, "$NEO_CHAT_WORKSPACE"},
	} {
		path := filepath.Clean(strings.TrimSpace(replacement.path))
		if filepath.IsAbs(path) && path != string(filepath.Separator) {
			value = strings.ReplaceAll(value, path, replacement.label)
		}
	}
	return value
}

func (executor *hostWorkspaceExecutor) call(
	ctx context.Context,
	tool string,
	arguments any,
	approved bool,
	activeSkillRoot string,
	output any,
) error {
	return executor.callWithScope(ctx, localskills.JobScope{}, tool, arguments, approved, activeSkillRoot, output)
}

func (executor *hostWorkspaceExecutor) callWithScope(
	ctx context.Context,
	scope localskills.JobScope,
	tool string,
	arguments any,
	approved bool,
	activeSkillRoot string,
	output any,
) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return localskills.ErrWorkspaceInvalidInput
	}
	return executor.service.ExecuteTool(ctx, agenthost.ToolExecuteRequest{
		ProtocolVersion: agenthost.ProtocolVersion,
		Workspace: agenthost.ExecutionWorkspace{
			CanonicalPath:        executor.binding.CanonicalPath,
			DirectoryFingerprint: executor.binding.DirectoryFingerprint,
		},
		Scope: agenthost.ExecutionScope{
			UserID: scope.UserID, ConversationID: scope.ConversationID,
		}, Tool: tool, Arguments: encoded, Approved: approved,
		PermissionMode:  executor.binding.PermissionMode,
		ActiveSkillRoot: activeSkillRoot,
	}, output)
}

func hostPermissionModeAvailable(modes []agenthost.PermissionMode, wanted agenthost.PermissionMode) bool {
	for _, mode := range modes {
		if mode == wanted {
			return true
		}
	}
	return false
}

func hostActiveSkillRoot(skillsRoot, activeRoot string) (string, bool) {
	activeRoot = strings.TrimSpace(activeRoot)
	if activeRoot == "" {
		return "", true
	}
	activeRoot = filepath.Clean(activeRoot)
	skillsRoot = filepath.Clean(strings.TrimSpace(skillsRoot))
	relative, err := filepath.Rel(skillsRoot, activeRoot)
	return filepath.ToSlash(relative), err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
