package ragproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	rerankPath                  = "/internal/retrieval/rerank"
	rerankModel                 = "jina-reranker-v3"
	maxRerankDocuments          = 20
	maxRerankQueryBytes         = 2048
	maxRerankDocumentBytes      = 64 * 1024
	maxRerankTotalDocumentBytes = 1024 * 1024
	maxRerankResponseBodyBytes  = 128 * 1024
	defaultRerankRequestTimeout = 40 * time.Second
)

var (
	ErrRerankUnavailable = errors.New("rerank unavailable")
	ErrRerankInvalid     = errors.New("rerank invalid")
)

type RerankResult struct {
	Index          int
	RelevanceScore float64
}

type Reranker interface {
	Rerank(context.Context, string, []string) ([]RerankResult, error)
}

type RerankClient struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type rerankResponse struct {
	Model   string                 `json:"model"`
	Results []rerankResponseResult `json:"results"`
}

type rerankResponseResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevanceScore"`
}

func NewRerankClient(
	rawBaseURL string,
	token string,
	timeout time.Duration,
) (*RerankClient, error) {
	if timeout <= 0 {
		timeout = defaultRerankRequestTimeout
	}
	return newRerankClient(rawBaseURL, token, &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

func newRerankClient(
	rawBaseURL string,
	token string,
	httpClient *http.Client,
) (*RerankClient, error) {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	if rawBaseURL == "" || !validInternalToken(token) || httpClient == nil {
		return nil, ErrRerankInvalid
	}
	endpoint, err := internalGatewayEndpoint(rawBaseURL, rerankPath)
	if err != nil {
		return nil, ErrRerankInvalid
	}
	return &RerankClient{
		endpoint: endpoint, token: token, httpClient: httpClient,
	}, nil
}

func (client *RerankClient) Rerank(
	ctx context.Context,
	query string,
	documents []string,
) ([]RerankResult, error) {
	if client == nil || client.httpClient == nil {
		return nil, ErrRerankUnavailable
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]byte(query)) > maxRerankQueryBytes ||
		len(documents) < 1 || len(documents) > maxRerankDocuments {
		return nil, ErrRerankInvalid
	}
	totalDocumentBytes := 0
	requestDocuments := make([]string, len(documents))
	for index, document := range documents {
		documentBytes := len([]byte(document))
		if strings.TrimSpace(document) == "" || documentBytes > maxRerankDocumentBytes {
			return nil, ErrRerankInvalid
		}
		totalDocumentBytes += documentBytes
		if totalDocumentBytes > maxRerankTotalDocumentBytes {
			return nil, ErrRerankInvalid
		}
		requestDocuments[index] = document
	}
	body, err := json.Marshal(rerankRequest{Query: query, Documents: requestDocuments})
	if err != nil {
		return nil, ErrRerankInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrRerankUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, ErrRerankUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrRerankUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxRerankResponseBodyBytes+1))
	if err != nil || len(raw) > maxRerankResponseBodyBytes {
		return nil, ErrRerankUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload rerankResponse
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, ErrRerankUnavailable
	}
	if payload.Model != rerankModel || len(payload.Results) != len(documents) {
		return nil, ErrRerankUnavailable
	}
	results := make([]RerankResult, 0, len(payload.Results))
	seen := make([]bool, len(documents))
	for _, result := range payload.Results {
		if result.Index < 0 || result.Index >= len(documents) || seen[result.Index] ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) {
			return nil, ErrRerankUnavailable
		}
		seen[result.Index] = true
		results = append(results, RerankResult{
			Index: result.Index, RelevanceScore: result.RelevanceScore,
		})
	}
	return results, nil
}
