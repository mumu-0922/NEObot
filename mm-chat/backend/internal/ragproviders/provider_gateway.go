package ragproviders

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const (
	InternalProviderPathPrefix  = "/internal/rag/providers/"
	InternalMinerUAllocatePath  = "/internal/rag/providers/mineru/allocate"
	InternalMinerUPollPath      = "/internal/rag/providers/mineru/poll"
	InternalJinaEmbeddingsPath  = "/internal/rag/providers/jina/embeddings"
	InternalProviderTokenHeader = "X-MM-Chat-Internal-Token"

	providerIDMinerU = "mineru"
	providerIDJina   = "jina"
)

var (
	ErrProviderGatewayInvalid              = errors.New("rag provider gateway request is invalid")
	ErrProviderGatewayOperationUnsupported = errors.New("rag provider gateway operation is unsupported")
	ErrProviderGatewayNotFound             = errors.New("rag provider configuration was not found")
	ErrProviderGatewayActivationRequired   = errors.New("rag provider activation is required")
	ErrProviderGatewayUnavailable          = errors.New("rag provider gateway is unavailable")
	ErrProviderGatewayUpstream             = errors.New("rag provider upstream failed")
)

type ProviderCredentialResolver interface {
	ResolveRAGProviderCredential(context.Context, string) (string, error)
}

type ProviderHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ProviderGatewayOption func(*ProviderGateway)

func WithProviderGatewayHTTPClient(client ProviderHTTPDoer) ProviderGatewayOption {
	return func(gateway *ProviderGateway) {
		gateway.httpClient = client
	}
}

type ProviderGateway struct {
	credentials ProviderCredentialResolver
	httpClient  ProviderHTTPDoer
}

func NewProviderGateway(
	credentials ProviderCredentialResolver,
	opts ...ProviderGatewayOption,
) *ProviderGateway {
	gateway := &ProviderGateway{credentials: credentials}
	for _, opt := range opts {
		if opt != nil {
			opt(gateway)
		}
	}
	if gateway.httpClient == nil {
		gateway.httpClient = newProviderGatewayHTTPClient()
	}
	return gateway
}

type MinerUAllocateRequest struct {
	Filename string `json:"filename"`
}

type MinerUAllocation struct {
	BatchID   string `json:"batchId"`
	Filename  string `json:"filename"`
	UploadURL string `json:"uploadUrl"`
}

type MinerUPollRequest struct {
	BatchID  string `json:"batchId"`
	Filename string `json:"filename"`
}

type MinerUPollResult struct {
	BatchID   string `json:"batchId"`
	Filename  string `json:"filename"`
	State     string `json:"state"`
	ResultURL string `json:"resultUrl,omitempty"`
}

type PassageEmbeddingInput struct {
	PassageID string `json:"passageId"`
	Text      string `json:"text"`
}

type PassageEmbeddingRequest struct {
	Passages []PassageEmbeddingInput `json:"passages"`
}

type PassageEmbeddingVector struct {
	PassageID string    `json:"passageId"`
	Embedding []float32 `json:"embedding"`
}

type PassageEmbeddingResponse struct {
	Model      string                   `json:"model"`
	Dimensions int                      `json:"dimensions"`
	Vectors    []PassageEmbeddingVector `json:"vectors"`
}

type ProviderOperations interface {
	AllocateMinerU(context.Context, MinerUAllocateRequest) (MinerUAllocation, error)
	PollMinerU(context.Context, MinerUPollRequest) (MinerUPollResult, error)
	EmbedPassages(context.Context, PassageEmbeddingRequest) (PassageEmbeddingResponse, error)
}

func (gateway *ProviderGateway) resolveCredential(
	ctx context.Context,
	providerID string,
) (string, error) {
	if gateway == nil || gateway.credentials == nil {
		return "", ErrProviderGatewayUnavailable
	}
	credential, err := gateway.credentials.ResolveRAGProviderCredential(ctx, providerID)
	if err != nil {
		return "", err
	}
	if !validProviderCredential(credential) {
		credential = ""
		return "", ErrProviderGatewayUnavailable
	}
	return credential, nil
}

func validProviderCredential(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 4096 {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

func validInternalToken(token string) bool {
	if token == "" || len([]byte(token)) > 4096 {
		return false
	}
	for _, character := range token {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

var (
	_ ProviderOperations = (*ProviderGateway)(nil)
	_ QueryEmbedder      = (*ProviderGateway)(nil)
	_ Reranker           = (*ProviderGateway)(nil)
)
