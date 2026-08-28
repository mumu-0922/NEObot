package skillsupply

import (
	"context"
	"net/http"
	"time"
)

const (
	SourceOfficial = "official"
	SourceLobeHub  = "lobehub"
	SourceGit      = "git"
	SourceZIP      = "zip"
	// SourceLearning is reserved for the held Agent Draft promotion path. It has
	// no public ingestion handler and never becomes admissible without the
	// agentlearning check and human-promotion transaction.
	SourceLearning = "learning"

	StatusValidated = "validated"
	StatusAdmitted  = "admitted"
	StatusRejected  = "rejected"
)

type ArchiveSource struct {
	Type              string
	Ref               string
	Identifier        string
	Version           string
	StripPrefix       string
	ExpectedName      string
	Data              []byte
	ExecutableAllowed bool
}

type SkillMetadata struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	AllowedTools  []string          `json:"allowedTools"`
}

type RuntimePackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type RuntimeUser struct {
	UID int64 `json:"uid"`
	GID int64 `json:"gid"`
}

type RuntimeSpec struct {
	Kind     string      `json:"kind"`
	Image    string      `json:"image"`
	Platform string      `json:"platform"`
	User     RuntimeUser `json:"user"`
}

type Entrypoint struct {
	Name             string   `json:"name"`
	Argv             []string `json:"argv"`
	WorkingDirectory string   `json:"workingDirectory"`
}

type CapabilityRequest struct {
	Capability string   `json:"capability"`
	Actions    []string `json:"actions"`
	Reason     string   `json:"reason"`
}

type EgressRequest struct {
	ID      string   `json:"id"`
	Mode    string   `json:"mode"`
	Schemes []string `json:"schemes"`
	Hosts   []string `json:"hosts"`
	Ports   []int    `json:"ports"`
	Reason  string   `json:"reason"`
}

type SecretSlot struct {
	Name     string `json:"name"`
	Purpose  string `json:"purpose"`
	Delivery string `json:"delivery"`
	Required bool   `json:"required"`
}

type RuntimeResources struct {
	CPUMillis  int64 `json:"cpuMillis"`
	MemoryMiB  int64 `json:"memoryMiB"`
	PIDs       int64 `json:"pids"`
	WallSecond int64 `json:"wallSeconds"`
}

type RuntimeLimits struct {
	MaxPackageFiles  int64 `json:"maxPackageFiles"`
	MaxPackageBytes  int64 `json:"maxPackageBytes"`
	MaxExpandedBytes int64 `json:"maxExpandedBytes"`
	MaxStdoutBytes   int64 `json:"maxStdoutBytes"`
	MaxStderrBytes   int64 `json:"maxStderrBytes"`
	MaxArtifactBytes int64 `json:"maxArtifactBytes"`
}

type RuntimeDependency struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type RuntimeManifest struct {
	SchemaVersion      string              `json:"schemaVersion"`
	Package            RuntimePackage      `json:"package"`
	Runtime            RuntimeSpec         `json:"runtime"`
	Entrypoints        []Entrypoint        `json:"entrypoints"`
	Dependencies       []RuntimeDependency `json:"dependencies"`
	CapabilityRequests []CapabilityRequest `json:"capabilityRequests"`
	EgressRequests     []EgressRequest     `json:"egressRequests"`
	SecretSlots        []SecretSlot        `json:"secretSlots"`
	Resources          RuntimeResources    `json:"resources"`
	Limits             RuntimeLimits       `json:"limits"`
}

type FileInventory struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type PackageVersion struct {
	PackageFingerprint       string              `json:"packageFingerprint"`
	RuntimeBundleFingerprint string              `json:"runtimeBundleFingerprint,omitempty"`
	SBOMFingerprint          string              `json:"sbomFingerprint"`
	Name                     string              `json:"name"`
	Version                  string              `json:"version"`
	Description              string              `json:"description"`
	License                  string              `json:"license,omitempty"`
	Compatibility            string              `json:"compatibility,omitempty"`
	AllowedTools             []string            `json:"allowedTools"`
	CapabilityRequests       []CapabilityRequest `json:"capabilityRequests"`
	HasRuntime               bool                `json:"hasRuntime"`
	FileCount                int                 `json:"fileCount"`
	PackageBytes             int64               `json:"packageBytes"`
	ExpandedBytes            int64               `json:"expandedBytes"`
	PackageObjectKey         string              `json:"-"`
	SBOMObjectKey            string              `json:"-"`
	Manifest                 *RuntimeManifest    `json:"-"`
	CreatedAt                time.Time           `json:"createdAt"`
}

type Candidate struct {
	ID                   string         `json:"id"`
	SourceType           string         `json:"sourceType"`
	SourceRef            string         `json:"sourceRef"`
	OwnerUserID          string         `json:"-"`
	SourceArtifactSHA256 string         `json:"sourceArtifactSha256"`
	SourceObjectKey      string         `json:"-"`
	Package              PackageVersion `json:"package"`
	Status               string         `json:"status"`
	AdmissionEligible    bool           `json:"admissionEligible"`
	ValidationSummary    string         `json:"validationSummary"`
	ReviewedByUserID     string         `json:"-"`
	ReviewReason         string         `json:"reviewReason,omitempty"`
	Revision             int64          `json:"revision"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

type Installation struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"-"`
	AdmissionID        string    `json:"admissionId"`
	PackageFingerprint string    `json:"packageFingerprint"`
	Name               string    `json:"name"`
	Version            string    `json:"version"`
	Description        string    `json:"description"`
	AllowedTools       []string  `json:"allowedTools"`
	Revision           int64     `json:"revision"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ConversationSelection is the durable set of owner-installed Skills pinned
// to one conversation. Revision zero represents an authorized conversation
// that has not stored an explicit selection yet.
type ConversationSelection struct {
	ConversationID string         `json:"conversationId"`
	Revision       int64          `json:"revision"`
	Skills         []Installation `json:"skills"`
}

type StoreResult struct {
	Items      []Candidate `json:"items"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
	TotalCount int         `json:"totalCount"`
	TotalPages int         `json:"totalPages"`
}

// CatalogSkillSummary is the bounded public projection of one Skill directory
// in the fixed Codex-compatible curated source. Source coordinates are display
// data only; installation re-resolves and validates immutable package bytes.
type CatalogSkillSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Repository    string `json:"repository"`
	Ref           string `json:"ref"`
	Path          string `json:"path"`
	SourceURL     string `json:"sourceUrl"`
	CatalogSource string `json:"catalogSource"`
}

type CatalogSkill struct {
	CatalogSkillSummary
	ResolvedCommit     string   `json:"resolvedCommit"`
	PackageFingerprint string   `json:"packageFingerprint"`
	Version            string   `json:"version"`
	Description        string   `json:"description"`
	License            string   `json:"license,omitempty"`
	Compatibility      string   `json:"compatibility,omitempty"`
	AllowedTools       []string `json:"allowedTools"`
	HasRuntime         bool     `json:"hasRuntime"`
}

type CatalogResult struct {
	Items      []CatalogSkillSummary `json:"items"`
	TotalCount int                   `json:"totalCount"`
	Source     string                `json:"source"`
}

type CatalogInstallInput struct {
	ResolvedCommit     string `json:"resolvedCommit"`
	PackageFingerprint string `json:"packageFingerprint"`
}

type ValidatedPackage struct {
	SourceArtifactSHA256 string
	CanonicalArchive     []byte
	SBOM                 []byte
	Package              PackageVersion
	Inventory            []FileInventory
}

type ReviewInput struct {
	Status             string
	ExpectedRevision   int64
	PackageFingerprint string
	Reason             string
}

type Repository interface {
	CreateCandidate(context.Context, Candidate) (Candidate, error)
	GetCandidate(context.Context, string) (Candidate, error)
	GetCandidateBySource(context.Context, string, string, string) (Candidate, error)
	ReviewCandidate(context.Context, string, string, ReviewInput) (Candidate, error)
	ListStore(context.Context, int, int) (StoreResult, error)
	GetStoreItem(context.Context, string) (Candidate, error)
	GetInstallableCandidate(context.Context, string, string) (Candidate, error)
	Install(context.Context, string, string, string) (Installation, error)
	ListLibrary(context.Context, string) ([]Installation, error)
	Uninstall(context.Context, string, string, int64) error
	AuthorizeConversation(context.Context, string, string) error
	GetConversationSelection(context.Context, string, string) (ConversationSelection, bool, error)
	ReplaceConversationSelection(context.Context, string, ConversationSelection) (ConversationSelection, error)
}

type LobeHubFetcher interface {
	FetchSkillPackage(context.Context, string, string, int64) ([]byte, error)
}

type LobeHubMarketplaceFetcher interface {
	FetchSkillMarketJSON(context.Context, string, int64) ([]byte, error)
	FetchPublicSkillDetailJSON(context.Context, string, int64) ([]byte, error)
}

type SourceHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
