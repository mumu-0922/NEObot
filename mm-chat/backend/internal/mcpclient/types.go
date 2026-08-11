package mcpclient

import (
	"context"
	"time"
)

const (
	SourceCatalog  = "catalog"
	SourceManifest = "manifest"
	SourcePrivate  = "private"

	TransportStreamableHTTP = "streamable_http"
	TransportStdio          = "stdio"

	AuthNone   = "none"
	AuthHeader = "header"
	AuthOAuth  = "oauth"

	ServerStatusDraft       = "draft"
	ServerStatusReady       = "ready"
	ServerStatusNeedsAuth   = "needs_auth"
	ServerStatusUnavailable = "unavailable"
	ServerStatusDisabled    = "disabled"

	ClassificationRead    = "read"
	ClassificationWrite   = "write"
	ClassificationUnknown = "unknown"

	CallStatusQueued         = "queued"
	CallStatusRunning        = "running"
	CallStatusSucceeded      = "succeeded"
	CallStatusFailed         = "failed"
	CallStatusCanceled       = "canceled"
	CallStatusOutcomeUnknown = "outcome_unknown"

	SelectionModeInherit = "inherit"
	SelectionModeCustom  = "custom"

	ToolSearchAlias = "neo_mcp_tool_search"
)

type Config struct {
	Enabled              bool
	RemoteEnabled        bool
	StdioEnabled         bool
	ManifestFile         string
	RunnerURL            string
	RunnerToken          string
	OAuthCallbackURL     string
	PrivateServerLimit   int
	ConversationLimit    int
	MaxExposedTools      int
	MaxCallsPerRun       int
	MaxRoundsPerRun      int
	MaxConcurrentPerUser int
	MaxOAuthFlows        int
	CallTimeout          time.Duration
	RunTimeout           time.Duration
	AuditRetention       time.Duration
	CleanupInterval      time.Duration
	MaxInlineResultBytes int64
	MaxResultItemBytes   int64
	MaxResultCallBytes   int64
}

type ServerRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

func (r ServerRef) Key() string {
	return r.Source + ":" + r.ID
}

type HeaderAuth struct {
	Name            string `json:"name,omitempty"`
	EncryptedSecret string `json:"-"`
	SecretFile      string `json:"secretFile,omitempty"`
	EnvRef          string `json:"envRef,omitempty"`
}

type OAuthClient struct {
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"-"`
	Scopes       []string `json:"scopes,omitempty"`
}

type Command struct {
	Argv             []string          `json:"argv"`
	Env              map[string]string `json:"env,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	IdleTimeout      time.Duration     `json:"-"`
	MaxLifetime      time.Duration     `json:"-"`
}

type Grant struct {
	ScopeType     string `json:"scopeType"`
	ScopeID       string `json:"scopeId,omitempty"`
	DefaultEnable bool   `json:"defaultEnabled"`
}

type Server struct {
	Ref              ServerRef      `json:"ref"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Transport        string         `json:"transport"`
	EndpointURL      string         `json:"endpointUrl,omitempty"`
	Command          *Command       `json:"-"`
	AuthType         string         `json:"authType"`
	HeaderAuth       *HeaderAuth    `json:"-"`
	OAuthClient      *OAuthClient   `json:"-"`
	Status           string         `json:"status"`
	HasCredential    bool           `json:"hasCredential"`
	ToolCount        int            `json:"toolCount"`
	UnsupportedCount int            `json:"unsupportedToolCount"`
	LastErrorCode    string         `json:"lastErrorCode,omitempty"`
	ValidatedAt      *time.Time     `json:"validatedAt,omitempty"`
	Grants           []Grant        `json:"grants,omitempty"`
	CreatedAt        time.Time      `json:"createdAt,omitempty"`
	UpdatedAt        time.Time      `json:"updatedAt,omitempty"`
	Tools            []Tool         `json:"-"`
	Metadata         map[string]any `json:"-"`
}

type Tool struct {
	ServerRef      ServerRef      `json:"serverRef"`
	Name           string         `json:"name"`
	Alias          string         `json:"alias"`
	Title          string         `json:"title,omitempty"`
	Description    string         `json:"description,omitempty"`
	InputSchema    map[string]any `json:"inputSchema"`
	Classification string         `json:"classification"`
	Supported      bool           `json:"supported"`
	Unsupported    string         `json:"unsupportedReason,omitempty"`
}

type SelectionServer struct {
	Ref           ServerRef `json:"ref"`
	DisabledTools []string  `json:"disabledTools"`
}

type Selection struct {
	ConversationID string            `json:"conversationId"`
	Mode           string            `json:"mode"`
	Revision       int64             `json:"revision"`
	Servers        []SelectionServer `json:"servers"`
}

type WorkspaceSelection struct {
	WorkspaceID string            `json:"workspaceId"`
	Revision    int64             `json:"revision"`
	Servers     []SelectionServer `json:"servers"`
}

type CreateServerInput struct {
	Name        string
	EndpointURL string
	AuthType    string
	HeaderName  string
	ClientID    string
	Scopes      []string
}

type Credential struct {
	ID                 string
	UserID             string
	ServerRef          ServerRef
	Kind               string
	EncryptedSecretRef string
	Metadata           map[string]any
	ExpiresAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type OAuthState struct {
	ID               string
	StateHash        string
	UserID           string
	ServerRef        ServerRef
	EncryptedFlowRef string
	ReturnURL        string
	ExpiresAt        time.Time
	ConsumedAt       *time.Time
	CreatedAt        time.Time
}

type CallRecord struct {
	ID               string         `json:"id"`
	ConversationID   string         `json:"conversationId"`
	MessageID        string         `json:"messageId"`
	RunID            string         `json:"runId"`
	ServerRef        ServerRef      `json:"serverRef"`
	ToolName         string         `json:"toolName"`
	ToolAlias        string         `json:"toolAlias"`
	Classification   string         `json:"classification"`
	Status           string         `json:"status"`
	Round            int            `json:"round"`
	Call             int            `json:"call"`
	ArgumentsSummary map[string]any `json:"argumentsSummary"`
	ResultSummary    string         `json:"resultSummary"`
	ErrorCode        string         `json:"errorCode"`
	StartedAt        *time.Time     `json:"startedAt,omitempty"`
	CompletedAt      *time.Time     `json:"completedAt,omitempty"`
	DurationMillis   int64          `json:"durationMillis"`
	RetainUntil      time.Time      `json:"-"`
}

// ExpiredCall is the bounded retention projection used to remove object-store
// artifacts before deleting their PostgreSQL references. Object keys are
// untrusted persisted data and must be revalidated by the service.
type ExpiredCall struct {
	ID             string
	ConversationID string
	ObjectKeys     []string
}

// PendingArtifact survives an account-row cascade long enough for the
// cleanup-only worker to delete the corresponding object-store bytes.
type PendingArtifact struct {
	ObjectKey      string
	ConversationID string
	CallID         string
}

type Content struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	JSON      map[string]any `json:"json,omitempty"`
	Data      []byte         `json:"-"`
	MIMEType  string         `json:"mimeType,omitempty"`
	URI       string         `json:"uri,omitempty"`
	Name      string         `json:"name,omitempty"`
	ObjectKey string         `json:"objectKey,omitempty"`
	ByteSize  int64          `json:"byteSize,omitempty"`
}

type CallResult struct {
	Content         []Content
	ModelContent    string
	Summary         string
	IsError         bool
	FailureCategory string
	OutcomeUnknown  bool
}

type ExecuteInput struct {
	Alias             string
	Arguments         map[string]any
	ValidationFailure string
	Round             int
	Call              int
}

type ExecutionEvent struct {
	CallID          string
	ServerRef       ServerRef
	ToolName        string
	ToolAlias       string
	Classification  string
	Status          string
	Round           int
	Call            int
	Arguments       map[string]any
	FailureCategory string
	DurationMillis  int64
}

type EventSink func(context.Context, ExecutionEvent) bool

type SnapshotServer struct {
	Ref       ServerRef `json:"ref"`
	Name      string    `json:"name"`
	Transport string    `json:"transport"`
	Tools     []Tool    `json:"tools"`
}

type RunSnapshot struct {
	RunID             string           `json:"runId"`
	UserID            string           `json:"-"`
	ConversationID    string           `json:"conversationId"`
	MessageID         string           `json:"messageId,omitempty"`
	SelectionRevision int64            `json:"selectionRevision"`
	Servers           []SnapshotServer `json:"servers"`
	Hash              string           `json:"hash"`
	CreatedAt         time.Time        `json:"createdAt"`
	ExpiresAt         time.Time        `json:"expiresAt"`
}

type PreparedRun struct {
	Snapshot RunSnapshot
	servers  map[string]Server
	aliases  map[string]Tool
}

func (r PreparedRun) Enabled() bool {
	return len(r.Snapshot.Servers) > 0
}
