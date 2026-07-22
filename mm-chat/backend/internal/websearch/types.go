package websearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	MaxQueryBytes         = 2048
	MaxResults            = 10
	MaxAPIKeyBytes        = 4096
	MaxResponseBytes      = 5 << 20
	MaxSourceTitleBytes   = 1024
	MaxSourceContentBytes = 64 << 10
	MaxSourceURLBytes     = 4096
	MaxImageDescription   = 2048
)

type ProviderID string

const (
	ProviderTavily    ProviderID = "tavily"
	ProviderFirecrawl ProviderID = "firecrawl"
	ProviderExa       ProviderID = "exa"
	ProviderBocha     ProviderID = "bocha"
)

type Scope string

const (
	ScopeGeneral  Scope = "general"
	ScopeNews     Scope = "news"
	ScopeAcademic Scope = "academic"
)

var (
	ErrInvalidConfig            = errors.New("web search provider config is invalid")
	ErrInvalidRequest           = errors.New("web search request is invalid")
	ErrNotConfigured            = errors.New("web search is not configured")
	ErrResolutionFailed         = errors.New("web search provider resolution failed")
	ErrModelBuiltInRequiresChat = errors.New("model built-in search requires chat execution")
)

type ExecutionMode string

const (
	ExecutionExternal     ExecutionMode = "external"
	ExecutionModelBuiltIn ExecutionMode = "model-built-in"
)

type ModelBuiltInProviderID string

const (
	ModelBuiltInOpenAI    ModelBuiltInProviderID = "openai"
	ModelBuiltInGemini    ModelBuiltInProviderID = "gemini"
	ModelBuiltInAnthropic ModelBuiltInProviderID = "anthropic"
)

type ModelBuiltInResolutionRequest struct {
	ProviderID string
	ModelID    string
	Protocol   ModelBuiltInProviderID
}

type Request struct {
	Query      string
	Scope      Scope
	MaxResults int
}

type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type Image struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Result struct {
	Sources []Source `json:"sources"`
	Images  []Image  `json:"images"`
}

type Provider interface {
	ID() ProviderID
	Search(context.Context, Request) (Result, error)
}

// ActiveExecution is resolved only from trusted server configuration. Exactly
// one of External or ModelBuiltIn is admitted for the selected mode.
type ActiveExecution struct {
	Mode         ExecutionMode
	External     Provider
	ModelBuiltIn ModelBuiltInProviderID
}

type Resolver interface {
	ResolveActive(context.Context) (ActiveExecution, error)
}

// ModeResolver keeps external and model-built-in configuration authority on
// separate paths. A caller that selects one mode must never observe or fall
// back to an execution configured for the other mode.
type ModeResolver interface {
	ResolveExternal(context.Context) (ActiveExecution, error)
	ResolveModelBuiltIn(context.Context, ModelBuiltInResolutionRequest) (ActiveExecution, error)
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	APIKey  string
	BaseURL string
	Client  HTTPDoer
}

type ProviderError struct {
	Provider ProviderID
	Code     string
	Status   int
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "web search provider failed"
	}
	return fmt.Sprintf("web search provider %s failed: %s", e.Provider, e.Code)
}

func normalizeRequest(input Request) (Request, error) {
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len(input.Query) > MaxQueryBytes {
		return Request{}, ErrInvalidRequest
	}
	if input.MaxResults == 0 {
		input.MaxResults = 5
	}
	if input.MaxResults < 1 || input.MaxResults > MaxResults {
		return Request{}, ErrInvalidRequest
	}
	switch input.Scope {
	case "":
		input.Scope = ScopeGeneral
	case ScopeGeneral, ScopeNews, ScopeAcademic:
	default:
		return Request{}, ErrInvalidRequest
	}
	return input, nil
}

func normalizeAPIKey(value string, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > MaxAPIKeyBytes || (required && value == "") {
		return "", ErrInvalidConfig
	}
	return value, nil
}
