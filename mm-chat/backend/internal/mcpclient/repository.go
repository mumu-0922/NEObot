package mcpclient

import (
	"context"
	"time"
)

type ConversationScope struct {
	ConversationID string
	UserID         string
	WorkspaceID    string
	TeamID         string
}

type Repository interface {
	CountPrivateServers(context.Context, string) (int, error)
	CreatePrivateServer(context.Context, string, CreateServerInput) (Server, error)
	ListPrivateServers(context.Context, string) ([]Server, error)
	GetPrivateServer(context.Context, string, string) (Server, error)
	UpdateServerValidation(context.Context, string, string, string, []Tool, string, string, *time.Time) (Server, error)
	DeletePrivateServer(context.Context, string, string) error

	GetCredential(context.Context, string, ServerRef) (Credential, bool, error)
	UpsertCredential(context.Context, Credential) (Credential, error)
	DeleteCredential(context.Context, string, ServerRef) error

	ConversationScope(context.Context, string, string) (ConversationScope, error)
	GetSelection(context.Context, string, string) (Selection, bool, error)
	ReplaceSelection(context.Context, string, Selection) (Selection, error)
	GetWorkspaceSelection(context.Context, string, string) (WorkspaceSelection, bool, error)
	ReplaceWorkspaceSelection(context.Context, string, WorkspaceSelection) (WorkspaceSelection, error)
	CreateRunSnapshot(context.Context, RunSnapshot) error

	CountPendingOAuthStates(context.Context, string, time.Time) (int, error)
	CreateOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, string, time.Time) (OAuthState, error)

	CreateCall(context.Context, string, CallRecord) (CallRecord, error)
	FinishCall(context.Context, string, CallRecord, []Content, []string, int64) error
	ListCalls(context.Context, string, string, string) ([]CallRecord, error)
}

// ConversationLifecycleRepository owns durable MCP data that follows a soft-
// deleted conversation. Object bytes are deleted before these references so a
// failed object-store operation remains retryable without orphaning data.
type ConversationLifecycleRepository interface {
	ListConversationObjectKeys(context.Context, string, string) ([]string, error)
	DeleteConversationData(context.Context, string, string) error
}

// RetentionRepository provides a retry-safe two-phase cleanup boundary. The
// service deletes every validated object first, then acknowledges only those
// expired calls whose object cleanup completed.
type RetentionRepository interface {
	ListExpiredCalls(context.Context, time.Time, int) ([]ExpiredCall, error)
	DeleteExpiredData(context.Context, time.Time, []string) error
}

type ArtifactCleanupRepository interface {
	ListPendingArtifacts(context.Context, int) ([]PendingArtifact, error)
	DeletePendingArtifacts(context.Context, []string) error
}
