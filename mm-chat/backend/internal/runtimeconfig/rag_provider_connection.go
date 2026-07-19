package runtimeconfig

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) testRAGProviderConnection(
	ctx context.Context,
	providerID RAGProviderID,
	apiKey string,
) ([]string, error) {
	switch providerID {
	case RAGProviderMinerU:
		if err := s.testMinerUConnection(ctx, apiKey); err != nil {
			return nil, err
		}
		return []string{"allocate"}, nil
	case RAGProviderJina:
		if err := s.testJinaEmbeddingConnection(ctx, apiKey); err != nil {
			return nil, err
		}
		if err := s.testJinaRerankConnection(ctx, apiKey); err != nil {
			return nil, err
		}
		return []string{"embedding", "rerank"}, nil
	default:
		return nil, ErrRAGProviderConfigUnsupported
	}
}

func (s *Service) testMinerUConnection(ctx context.Context, apiKey string) error {
	body, err := json.Marshal(map[string]any{
		"enable_formula": true,
		"enable_table":   true,
		"files":          []map[string]string{{"name": "mm-chat-provider-test.pdf"}},
		"is_ocr":         true,
		"model_version":  minerUModelVersion,
	})
	if err != nil {
		return err
	}
	payload, err := s.postRAGProviderJSON(
		ctx,
		minerUAllocateURL,
		apiKey,
		body,
		minerUMaxResponseBytes,
	)
	if err != nil {
		return err
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			BatchID  string   `json:"batch_id"`
			FileURLs []string `json:"file_urls"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Code != 0 ||
		!validRAGProviderIdentifier(response.Data.BatchID) ||
		len(response.Data.FileURLs) != 1 ||
		!validMinerUSignedUploadURL(response.Data.FileURLs[0]) {
		return ErrRAGProviderConnectionFailed
	}
	return nil
}

func (s *Service) testJinaEmbeddingConnection(ctx context.Context, apiKey string) error {
	body, err := json.Marshal(map[string]any{
		"dimensions":             jinaDimensions,
		"embedding_type":         "float",
		"input":                  []map[string]string{{"text": ragConnectionSentinel}},
		"late_chunking":          false,
		"model":                  jinaEmbeddingModel,
		"return_multivector":     false,
		"return_tokenized_input": false,
		"task":                   "retrieval.query",
		"truncate":               false,
	})
	if err != nil {
		return err
	}
	payload, err := s.postRAGProviderJSON(
		ctx,
		jinaEmbeddingsURL,
		apiKey,
		body,
		jinaMaxResponseBytes,
	)
	if err != nil {
		return err
	}
	var response struct {
		Model string `json:"model"`
		Data  []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &response) != nil ||
		response.Model != jinaEmbeddingModel || len(response.Data) != 1 ||
		response.Data[0].Index != 0 || len(response.Data[0].Embedding) != jinaDimensions {
		return ErrRAGProviderConnectionFailed
	}
	norm := 0.0
	for _, component := range response.Data[0].Embedding {
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return ErrRAGProviderConnectionFailed
		}
		norm += component * component
	}
	if norm <= 0 || math.IsInf(norm, 0) {
		return ErrRAGProviderConnectionFailed
	}
	return nil
}

func (s *Service) testJinaRerankConnection(ctx context.Context, apiKey string) error {
	body, err := json.Marshal(map[string]any{
		"documents":         []string{ragConnectionSentinel},
		"model":             jinaRerankModel,
		"query":             ragConnectionSentinel,
		"return_documents":  false,
		"return_embeddings": false,
		"top_n":             1,
	})
	if err != nil {
		return err
	}
	payload, err := s.postRAGProviderJSON(
		ctx,
		jinaRerankURL,
		apiKey,
		body,
		jinaMaxResponseBytes,
	)
	if err != nil {
		return err
	}
	var response struct {
		Model   string `json:"model"`
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Model != jinaRerankModel ||
		len(response.Results) != 1 || response.Results[0].Index != 0 ||
		math.IsNaN(response.Results[0].RelevanceScore) ||
		math.IsInf(response.Results[0].RelevanceScore, 0) {
		return ErrRAGProviderConnectionFailed
	}
	return nil
}

func (s *Service) postRAGProviderJSON(
	ctx context.Context,
	endpoint string,
	apiKey string,
	body []byte,
	maxResponseBytes int64,
) ([]byte, error) {
	if maxResponseBytes < 1 || maxResponseBytes > jinaMaxResponseBytes {
		return nil, ErrRAGProviderConnectionFailed
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, ErrRAGProviderConnectionFailed
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := s.ragHTTPClient
	if client == nil {
		client = newRAGProviderHTTPClient()
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, ErrRAGProviderConnectionFailed
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !isJSONContentType(response.Header.Get("Content-Type")) {
		return nil, ErrRAGProviderConnectionFailed
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return nil, ErrRAGProviderConnectionFailed
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > maxResponseBytes {
		return nil, ErrRAGProviderConnectionFailed
	}
	return raw, nil
}

func newRAGProviderHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			ForceAttemptHTTP2:     true,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
		Timeout: ragConnectionTestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func validRAGAPIKey(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > ragMaxAPIKeyBytes {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

func validRAGProviderIdentifier(value string) bool {
	if value == "" || len([]byte(value)) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._-", character) {
			continue
		}
		return false
	}
	return true
}

func validMinerUSignedUploadURL(value string) bool {
	if value == "" || len([]byte(value)) > 4096 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != minerUUploadHost ||
		parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery == "" ||
		(parsed.Port() != "" && parsed.Port() != "443") ||
		!strings.HasPrefix(parsed.EscapedPath(), minerUUploadPathPrefix) {
		return false
	}
	for _, segment := range strings.Split(strings.ToLower(parsed.EscapedPath()), "/") {
		if segment == "." || segment == ".." || segment == "%2e" || segment == "%2e%2e" {
			return false
		}
	}
	return true
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}
