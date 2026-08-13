package agents

import (
	"context"
	"time"
)

type Locale string

const (
	LocaleEnglish  Locale = "en"
	LocaleChinese  Locale = "zh"
	LocaleJapanese Locale = "ja"

	SourceCustom  = "custom"
	SourceLobeHub = "lobehub"

	AdmissionAdmitted = "admitted"
	AdmissionRejected = "rejected"
)

type AgentMeta struct {
	Avatar      string   `json:"avatar"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	SystemRole  string   `json:"systemRole,omitempty"`
}

type AgentConfig struct {
	SystemRole string `json:"systemRole,omitempty"`
}

type Agent struct {
	Identifier    string       `json:"identifier"`
	Meta          AgentMeta    `json:"meta"`
	CreatedAt     string       `json:"createdAt"`
	UpdatedAt     string       `json:"updatedAt,omitempty"`
	Homepage      string       `json:"homepage"`
	Author        string       `json:"author"`
	IsCustom      bool         `json:"isCustom,omitempty"`
	Config        *AgentConfig `json:"config,omitempty"`
	Version       string       `json:"version,omitempty"`
	Fingerprint   string       `json:"fingerprint,omitempty"`
	InstallCount  int          `json:"installCount,omitempty"`
	IsValidated   bool         `json:"isValidated,omitempty"`
	SafetyCheck   string       `json:"safetyCheck,omitempty"`
	RequiredTools []string     `json:"requiredTools,omitempty"`
	Admitted      bool         `json:"admitted,omitempty"`
	Installed     bool         `json:"installed,omitempty"`
	LibraryID     string       `json:"libraryId,omitempty"`
}

// Legacy list response remains for compatibility with older clients.
type ListResponse struct {
	Agents      []Agent `json:"agents"`
	Unavailable bool    `json:"unavailable,omitempty"`
}

type MarketCategory struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type MarketSearchInput struct {
	Query    string
	Category string
	Locale   Locale
	Page     int
	PageSize int
}

type MarketSearchResult struct {
	Agents           []Agent          `json:"agents"`
	Categories       []MarketCategory `json:"categories"`
	Page             int              `json:"page"`
	PageSize         int              `json:"pageSize"`
	TotalCount       int              `json:"totalCount"`
	TotalPages       int              `json:"totalPages"`
	TotalMarketCount int              `json:"totalMarketCount,omitempty"`
	Source           string           `json:"source"`
	Unavailable      bool             `json:"unavailable,omitempty"`
	CanReview        bool             `json:"canReview,omitempty"`
}

type LibraryEntry struct {
	ID                 string    `json:"id"`
	Source             string    `json:"source"`
	SourceIdentifier   string    `json:"sourceIdentifier,omitempty"`
	Avatar             string    `json:"avatar"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Category           string    `json:"category"`
	Tags               []string  `json:"tags"`
	SystemPrompt       string    `json:"systemPrompt"`
	Author             string    `json:"author"`
	Homepage           string    `json:"homepage"`
	SourceVersion      string    `json:"sourceVersion,omitempty"`
	SourceUpdatedAt    string    `json:"sourceUpdatedAt,omitempty"`
	RequiredTools      []string  `json:"requiredTools,omitempty"`
	ContentFingerprint string    `json:"contentFingerprint"`
	Revision           int64     `json:"revision"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	UpdateAvailable    bool      `json:"updateAvailable,omitempty"`
}

type Snapshot struct {
	SourceIdentifier   string
	Avatar             string
	Title              string
	Description        string
	Category           string
	Tags               []string
	SystemPrompt       string
	Author             string
	Homepage           string
	SourceVersion      string
	SourceUpdatedAt    string
	RequiredTools      []string
	ContentFingerprint string
}

type CreateCustomInput struct {
	Avatar       string
	Title        string
	Description  string
	Category     string
	Tags         []string
	SystemPrompt string
}

type UpdateCustomInput struct {
	ExpectedRevision int64
	CreateCustomInput
}

type Admission struct {
	Snapshot
	Status string
}

type Repository interface {
	ListLibrary(context.Context, string) ([]LibraryEntry, error)
	GetLibrary(context.Context, string, string) (LibraryEntry, error)
	CreateCustom(context.Context, string, Snapshot) (LibraryEntry, error)
	UpdateCustom(context.Context, string, string, int64, Snapshot) (LibraryEntry, error)
	DeleteCustom(context.Context, string, string, int64) error
	Install(context.Context, string, Snapshot) (LibraryEntry, error)
	UpdateInstalled(context.Context, string, string, int64, Snapshot) (LibraryEntry, error)
	Uninstall(context.Context, string, string, int64) error
	GetAdmission(context.Context, string) (Admission, error)
	ListAdmissions(context.Context, MarketSearchInput) (MarketSearchResult, error)
	UpsertAdmission(context.Context, string, Admission) error
}
