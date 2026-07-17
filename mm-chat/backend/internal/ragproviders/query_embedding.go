package ragproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	queryEmbeddingPath           = "/internal/retrieval/query-embedding"
	queryEmbeddingModel          = "jina-embeddings-v4"
	queryEmbeddingDimensions     = 1024
	maxQueryEmbeddingQueryBytes  = 2048
	maxQueryEmbeddingTokenBytes  = 4096
	maxQueryEmbeddingBodyBytes   = 128 * 1024
	defaultQueryEmbeddingTimeout = 40 * time.Second
)

var (
	ErrQueryEmbeddingUnavailable = errors.New("query embedding unavailable")
	ErrQueryEmbeddingInvalid     = errors.New("query embedding invalid")
)

type QueryEmbedding struct {
	ModelID    string
	Dimensions int
	Vector     []float32
}

type QueryEmbedder interface {
	EmbedQuery(context.Context, string) (QueryEmbedding, error)
}

type QueryEmbeddingClient struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

type queryEmbeddingResponse struct {
	Model      string    `json:"model"`
	Dimensions int       `json:"dimensions"`
	Embedding  []float32 `json:"embedding"`
}

func NewQueryEmbeddingClient(
	rawBaseURL string,
	token string,
	timeout time.Duration,
) (*QueryEmbeddingClient, error) {
	if timeout <= 0 {
		timeout = defaultQueryEmbeddingTimeout
	}
	return newQueryEmbeddingClient(rawBaseURL, token, &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

func newQueryEmbeddingClient(
	rawBaseURL string,
	token string,
	httpClient *http.Client,
) (*QueryEmbeddingClient, error) {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	if rawBaseURL == "" || !validInternalToken(token) || httpClient == nil {
		return nil, ErrQueryEmbeddingInvalid
	}
	parsed, err := url.Parse(rawBaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrQueryEmbeddingInvalid
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + queryEmbeddingPath
	parsed.RawPath = ""
	return &QueryEmbeddingClient{
		endpoint: parsed.String(), token: token, httpClient: httpClient,
	}, nil
}

func (client *QueryEmbeddingClient) EmbedQuery(
	ctx context.Context,
	query string,
) (QueryEmbedding, error) {
	if client == nil || client.httpClient == nil {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]byte(query)) > maxQueryEmbeddingQueryBytes {
		return QueryEmbedding{}, ErrQueryEmbeddingInvalid
	}
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return QueryEmbedding{}, ErrQueryEmbeddingInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	limited := io.LimitReader(response.Body, maxQueryEmbeddingBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil || len(raw) > maxQueryEmbeddingBodyBytes {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload queryEmbeddingResponse
	if err := decoder.Decode(&payload); err != nil {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	if payload.Model != queryEmbeddingModel ||
		payload.Dimensions != queryEmbeddingDimensions ||
		len(payload.Embedding) != queryEmbeddingDimensions {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	normSquared := 0.0
	for _, component := range payload.Embedding {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
		}
		normSquared += value * value
	}
	if normSquared <= 0 || math.IsInf(normSquared, 0) {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	return QueryEmbedding{
		ModelID: payload.Model, Dimensions: payload.Dimensions,
		Vector: append([]float32(nil), payload.Embedding...),
	}, nil
}

func validInternalToken(token string) bool {
	if token == "" || len([]byte(token)) > maxQueryEmbeddingTokenBytes {
		return false
	}
	for _, character := range token {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}
