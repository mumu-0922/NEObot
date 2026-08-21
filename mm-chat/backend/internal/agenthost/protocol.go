package agenthost

import (
	"encoding/json"
	"fmt"
)

const (
	ProtocolVersion = 1

	CapabilitiesPath        = "/internal/v1/capabilities"
	WorkspaceResolvePath    = "/internal/v1/workspaces/resolve"
	DirectoryBrowsePath     = "/internal/v1/directories/browse"
	NativeDirectoryPickPath = "/internal/v1/directories/pick-native"
	ToolExecutePath         = "/internal/v1/tools/execute"
)

type PermissionMode string

const (
	PermissionReadOnly       PermissionMode = "read-only"
	PermissionWorkspaceWrite PermissionMode = "workspace-write"
	PermissionFullAccess     PermissionMode = "danger-full-access"
)

type HostFeatures struct {
	WorkspaceResolve      bool             `json:"workspaceResolve"`
	DirectoryBrowse       bool             `json:"directoryBrowse"`
	NativeDirectoryPicker bool             `json:"nativeDirectoryPicker"`
	WindowsPathInterop    bool             `json:"windowsPathInterop"`
	Execution             bool             `json:"execution"`
	PermissionModes       []PermissionMode `json:"permissionModes"`
}

type HostLimits struct {
	MaxRequestBytes  int64 `json:"maxRequestBytes"`
	MaxResponseBytes int64 `json:"maxResponseBytes"`
	MaxPathBytes     int   `json:"maxPathBytes"`
}

type Capabilities struct {
	ProtocolVersion int          `json:"protocolVersion"`
	RunnerID        string       `json:"runnerId"`
	Version         string       `json:"version"`
	Platform        string       `json:"platform"`
	Architecture    string       `json:"architecture"`
	Features        HostFeatures `json:"features"`
	Limits          HostLimits   `json:"limits"`
}

type WorkspaceResolveRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Path            string `json:"path"`
}

type WorkspaceDescriptor struct {
	CanonicalPath        string `json:"canonicalPath"`
	DisplayPath          string `json:"displayPath"`
	PathKind             string `json:"pathKind"`
	DirectoryFingerprint string `json:"directoryFingerprint"`
}

type WorkspaceResolveResponse struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	RunnerID        string              `json:"runnerId"`
	Workspace       WorkspaceDescriptor `json:"workspace"`
}

type DirectoryBrowseRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Path            string `json:"path,omitempty"`
}

type DirectoryEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DisplayPath string `json:"displayPath"`
	PathKind    string `json:"pathKind"`
}

type DirectoryBrowseResponse struct {
	ProtocolVersion int              `json:"protocolVersion"`
	RunnerID        string           `json:"runnerId"`
	Path            string           `json:"path"`
	DisplayPath     string           `json:"displayPath"`
	PathKind        string           `json:"pathKind"`
	ParentPath      string           `json:"parentPath,omitempty"`
	Entries         []DirectoryEntry `json:"entries"`
}

type NativeDirectoryPickRequest struct {
	ProtocolVersion int `json:"protocolVersion"`
}

type NativeDirectoryPickResponse struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	RunnerID        string               `json:"runnerId"`
	Cancelled       bool                 `json:"cancelled"`
	Workspace       *WorkspaceDescriptor `json:"workspace,omitempty"`
}

type ExecutionWorkspace struct {
	CanonicalPath        string `json:"canonicalPath"`
	DirectoryFingerprint string `json:"directoryFingerprint"`
}

type ExecutionScope struct {
	UserID         string `json:"userId"`
	ConversationID string `json:"conversationId"`
}

type ToolExecuteRequest struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Workspace       ExecutionWorkspace `json:"workspace"`
	Scope           ExecutionScope     `json:"scope"`
	Tool            string             `json:"tool"`
	Arguments       json.RawMessage    `json:"arguments"`
	PermissionMode  PermissionMode     `json:"permissionMode"`
	Approved        bool               `json:"approved"`
	ActiveSkillRoot string             `json:"activeSkillRoot,omitempty"`
}

type ToolExecuteResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RunnerID        string          `json:"runnerId"`
	Result          json.RawMessage `json:"result"`
}

type TerminalToolArguments struct {
	Command        string `json:"command"`
	WorkingDir     string `json:"workingDir"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type RemoteError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e RemoteError) Error() string {
	if e.Code == "" {
		return "agent Host request failed"
	}
	return fmt.Sprintf("agent Host request failed: %s", e.Code)
}
