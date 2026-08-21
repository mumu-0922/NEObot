package hostworkspace

import (
	"context"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

var (
	ErrDisabled                   = errors.New("Host Workspaces are unavailable")
	ErrInvalid                    = errors.New("Host Workspace input is invalid")
	ErrNotFound                   = errors.New("Host Workspace was not found")
	ErrRevisionConflict           = errors.New("Host Workspace revision conflict")
	ErrAlreadyBound               = errors.New("Host Workspace is already bound")
	ErrDirectoryAlreadyRegistered = errors.New("Host directory is already registered")
	ErrConversationLocked         = errors.New("conversation Agent Workspace is locked")
	ErrConversationNotFound       = errors.New("conversation was not found")
	ErrWorkspaceUnbound           = errors.New("Host Workspace is unbound")
	ErrWorkspaceInUse             = errors.New("Host Workspace is in use")
	ErrPermissionUnavailable      = errors.New("Agent permission mode is unavailable")
	ErrPermissionAcknowledgement  = errors.New("Full access acknowledgement is required")
	ErrPermissionLocked           = errors.New("Agent permission mode is locked by an active Turn")
)

type WorkspaceFile struct {
	ID       string `json:"id"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	FileName string `json:"fileName"`
	Source   string `json:"source,omitempty"`
	FileID   string `json:"fileId,omitempty"`
	Size     int64  `json:"size,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
}

type Settings struct {
	Name            string          `json:"name"`
	SystemPrompt    string          `json:"systemPrompt,omitempty"`
	Files           []WorkspaceFile `json:"files"`
	Color           string          `json:"color,omitempty"`
	EnableSearch    *bool           `json:"enableSearch,omitempty"`
	EnableReasoning *bool           `json:"enableReasoning,omitempty"`
}

type Workspace struct {
	ID                   string
	UserID               string
	Settings             Settings
	Revision             int64
	RunnerID             string
	CanonicalPath        string
	DisplayPath          string
	PathKind             string
	DirectoryFingerprint string
	BoundAt              *time.Time
	LegacyImportedAt     *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
}

func (workspace Workspace) Bound() bool { return workspace.RunnerID != "" }

type ExecutionBinding struct {
	ConversationID       string
	WorkspaceID          string
	RunnerID             string
	CanonicalPath        string
	DirectoryFingerprint string
	BoundAt              time.Time
	PermissionMode       agenthost.PermissionMode
}

type Repository interface {
	List(context.Context) ([]Workspace, error)
	Get(context.Context, string) (Workspace, error)
	ImportLegacy(context.Context, string, Settings, time.Time) (Workspace, error)
	UpdateSettings(context.Context, string, int64, Settings) (Workspace, error)
	Bind(context.Context, string, int64, agenthost.WorkspaceDescriptor, string, time.Time) (Workspace, error)
	Delete(context.Context, string, int64, time.Time) error
	SetConversationWorkspace(context.Context, string, string) error
	ClearConversationWorkspace(context.Context, string, string) error
	LockConversationExecutionWorkspace(context.Context, string, string, time.Time) (ExecutionBinding, error)
	SetConversationPermission(context.Context, string, agenthost.PermissionMode) error
}

type PathResolver interface {
	ResolveWorkspace(context.Context, string) (agenthost.WorkspaceDescriptor, error)
	RunnerID() string
}

type capabilityResolver interface {
	Capabilities(context.Context) (agenthost.Capabilities, error)
}

type directoryBrowser interface {
	BrowseDirectories(context.Context, string) (agenthost.DirectoryBrowseResponse, error)
}

type nativeDirectoryPicker interface {
	PickNativeDirectory(context.Context) (agenthost.NativeDirectoryPickResponse, error)
}

type hostToolExecutor interface {
	ExecuteTool(context.Context, agenthost.ToolExecuteRequest, any) error
}

type HostStatus struct {
	Enabled      bool
	Status       string
	RunnerID     string
	Platform     string
	Architecture string
	Features     agenthost.HostFeatures
}
