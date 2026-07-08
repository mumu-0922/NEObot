package browserimport

import (
	"context"
	"time"
)

const (
	FormatName    = "neo-chat-browser-import"
	SchemaVersion = "mm-chat.browser-import.v2"
	DevUserID     = "00000000-0000-0000-0000-000000000001"
)

type Repository interface {
	Commit(ctx context.Context, pkg Package) (CommitResponse, error)
	GetBatchStatus(ctx context.Context, batchID string) (BatchStatusResponse, error)
	Rollback(ctx context.Context, batchID string) error
}

type Package struct {
	Manifest     Manifest
	PackageHash  string
	ManifestHash string
	Warnings     []Issue
}

type Manifest struct {
	Format           string               `json:"format"`
	SchemaVersion    string               `json:"schemaVersion"`
	StorageVersion   int                  `json:"storageVersion"`
	AppExportVersion *int                 `json:"appExportVersion,omitempty"`
	ExportedAt       string               `json:"exportedAt,omitempty"`
	GeneratedAt      string               `json:"generatedAt"`
	IdempotencyKey   string               `json:"idempotencyKey"`
	Source           ImportSource         `json:"source"`
	Counts           ImportCounts         `json:"counts"`
	OPFS             OPFSSummary          `json:"opfs"`
	Options          *ImportOptions       `json:"options,omitempty"`
	Conversations    []ImportConversation `json:"conversations"`
	Messages         []ImportMessage      `json:"messages"`
	Files            []ImportFile         `json:"files"`
	Workspaces       []ImportWorkspace    `json:"workspaces,omitempty"`
	Deferred         *DeferredSummary     `json:"deferred,omitempty"`
}

type ImportSource struct {
	App    string `json:"app"`
	Origin string `json:"origin,omitempty"`
}

type ImportCounts struct {
	Conversations int   `json:"conversations"`
	Messages      int   `json:"messages"`
	Files         int   `json:"files"`
	Bytes         int64 `json:"bytes"`
}

type OPFSSummary struct {
	ReferencedURLs []string `json:"referencedUrls"`
	MissingURLs    []string `json:"missingUrls"`
	OrphanURLs     []string `json:"orphanUrls"`
}

type ImportOptions struct {
	OnDuplicate       string `json:"onDuplicate,omitempty"`
	AllowMissingFiles *bool  `json:"allowMissingFiles,omitempty"`
}

type DeferredSummary struct {
	KnowledgeCollections int `json:"knowledgeCollections,omitempty"`
	Memories             int `json:"memories,omitempty"`
	ProviderSettings     int `json:"providerSettings,omitempty"`
}

type ImportConversation struct {
	ClientID          string         `json:"clientId"`
	Title             string         `json:"title"`
	Status            string         `json:"status,omitempty"`
	ModelRef          *ModelRef      `json:"modelRef,omitempty"`
	SystemInstruction string         `json:"systemInstruction,omitempty"`
	WorkspaceClientID string         `json:"workspaceClientId,omitempty"`
	Pinned            bool           `json:"pinned,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	CreatedAt         string         `json:"createdAt,omitempty"`
	UpdatedAt         string         `json:"updatedAt"`
}

type ModelRef struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type ImportMessage struct {
	ClientID             string             `json:"clientId"`
	ConversationClientID string             `json:"conversationClientId"`
	ParentClientID       string             `json:"parentClientId,omitempty"`
	SequenceNo           int                `json:"sequenceNo"`
	Role                 string             `json:"role"`
	Status               string             `json:"status,omitempty"`
	Content              string             `json:"content"`
	ModelRef             *ModelRef          `json:"modelRef,omitempty"`
	Attachments          []ImportAttachment `json:"attachments,omitempty"`
	OutputBlocks         []any              `json:"outputBlocks,omitempty"`
	Metadata             map[string]any     `json:"metadata,omitempty"`
	CreatedAt            string             `json:"createdAt"`
	CompletedAt          string             `json:"completedAt,omitempty"`
}

type ImportAttachment struct {
	ClientAttachmentID string `json:"clientAttachmentId"`
	Source             string `json:"source"`
	ClientFileID       string `json:"clientFileId,omitempty"`
	FileName           string `json:"fileName"`
	MimeType           string `json:"mimeType"`
	Size               int64  `json:"size,omitempty"`
	SHA256             string `json:"sha256,omitempty"`
	URL                string `json:"url,omitempty"`
	Purpose            string `json:"purpose,omitempty"`
}

type ImportFile struct {
	ClientFileID        string   `json:"clientFileId"`
	Source              string   `json:"source"`
	OriginalURL         string   `json:"originalUrl,omitempty"`
	SourceAttachmentIDs []string `json:"sourceAttachmentIds"`
	FileName            string   `json:"fileName"`
	MimeType            string   `json:"mimeType"`
	Size                int64    `json:"size"`
	SHA256              string   `json:"sha256"`
	BlobPath            string   `json:"blobPath"`
	Purpose             string   `json:"purpose"`
}

type ImportWorkspace struct {
	ClientID     string `json:"clientId"`
	Name         string `json:"name"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
	Color        string `json:"color,omitempty"`
}

type PreviewResponse struct {
	Summary       ImportSummary `json:"summary"`
	Warnings      []Issue       `json:"warnings"`
	Errors        []Issue       `json:"errors"`
	CommitAllowed bool          `json:"commitAllowed"`
}

type ImportSummary struct {
	Conversations     int   `json:"conversations"`
	Messages          int   `json:"messages"`
	Files             int   `json:"files"`
	Bytes             int64 `json:"bytes"`
	SkippedDuplicates int   `json:"skippedDuplicates"`
}

type CommitResponse struct {
	BatchID  string         `json:"batchId"`
	Status   string         `json:"status"`
	Created  CreatedCounts  `json:"created"`
	Mappings ImportMappings `json:"mappings"`
	Warnings []Issue        `json:"warnings"`
}

type CreatedCounts struct {
	Conversations int `json:"conversations"`
	Messages      int `json:"messages"`
	Files         int `json:"files"`
	Attachments   int `json:"attachments"`
}

type ImportMappings struct {
	Conversations map[string]string `json:"conversations"`
	Messages      map[string]string `json:"messages"`
	Files         map[string]string `json:"files"`
}

type BatchStatusResponse struct {
	BatchID   string `json:"batchId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type Issue struct {
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type parsedConversation struct {
	Conversation ImportConversation
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type parsedMessage struct {
	Message     ImportMessage
	CreatedAt   time.Time
	CompletedAt time.Time
}
