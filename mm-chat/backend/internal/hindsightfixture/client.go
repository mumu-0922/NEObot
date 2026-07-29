package hindsightfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maximumResponseBytes = 4 << 20

type Fault struct {
	Code string
}

func (fault *Fault) Error() string {
	return fault.Code
}

func FaultCode(err error) string {
	var fault *Fault
	if errors.As(err, &fault) {
		return fault.Code
	}
	return "adapter_failure"
}

type HTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type RetainItem struct {
	Content    string
	Timestamp  string
	Metadata   map[string]string
	DocumentID string
	Tags       []string
}

func NewHTTPClient(baseURL, apiKey string, client *http.Client) (*HTTPClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("Hindsight API boundary is invalid")
	}
	if len(apiKey) < 32 || strings.TrimSpace(apiKey) != apiKey {
		return nil, errors.New("Hindsight API credential is invalid")
	}
	if client == nil {
		return nil, errors.New("Hindsight HTTP client is required")
	}
	return &HTTPClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		client:  client,
	}, nil
}

func (client *HTTPClient) ConfigureBank(ctx context.Context, bankID string, mode Mode) error {
	extractionMode := ""
	switch mode {
	case ModeEndToEnd:
		extractionMode = "concise"
	case ModeRetrievalOnly:
		extractionMode = "chunks"
	default:
		return &Fault{Code: "invalid_mode"}
	}
	request := struct {
		Updates map[string]any `json:"updates"`
	}{Updates: map[string]any{
		"audit_log_enabled":      false,
		"retain_extraction_mode": extractionMode,
	}}
	var response struct {
		BankID    string          `json:"bank_id"`
		Config    json.RawMessage `json:"config"`
		Overrides json.RawMessage `json:"overrides"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodPatch,
		bankPath(bankID)+"/config",
		request,
		&response,
	); err != nil {
		return err
	}
	if response.BankID != bankID || len(response.Config) == 0 || len(response.Overrides) == 0 {
		return &Fault{Code: "bank_mismatch"}
	}
	return nil
}

func (client *HTTPClient) Retain(ctx context.Context, bankID string, item RetainItem) error {
	request := struct {
		Items []struct {
			Content    string            `json:"content"`
			Timestamp  string            `json:"timestamp"`
			Metadata   map[string]string `json:"metadata"`
			DocumentID string            `json:"document_id"`
			Tags       []string          `json:"tags"`
		} `json:"items"`
		Async bool `json:"async"`
	}{Async: false}
	request.Items = append(request.Items, struct {
		Content    string            `json:"content"`
		Timestamp  string            `json:"timestamp"`
		Metadata   map[string]string `json:"metadata"`
		DocumentID string            `json:"document_id"`
		Tags       []string          `json:"tags"`
	}{
		Content: item.Content, Timestamp: item.Timestamp, Metadata: item.Metadata,
		DocumentID: item.DocumentID, Tags: item.Tags,
	})
	var response struct {
		Success      bool            `json:"success"`
		BankID       string          `json:"bank_id"`
		ItemsCount   int             `json:"items_count"`
		Async        bool            `json:"async"`
		OperationID  *string         `json:"operation_id"`
		OperationIDs []string        `json:"operation_ids"`
		Usage        json.RawMessage `json:"usage"`
	}
	if err := client.doJSON(ctx, http.MethodPost, bankPath(bankID)+"/memories", request, &response); err != nil {
		return err
	}
	if !response.Success || response.BankID != bankID || response.ItemsCount != 1 || response.Async {
		return &Fault{Code: "retain_response_invalid"}
	}
	return nil
}

func (client *HTTPClient) Recall(
	ctx context.Context,
	bankID string,
	query string,
	scope RecallScope,
) ([]string, error) {
	tags, tagsMatch := recallTags(scope)
	request := struct {
		Query     string   `json:"query"`
		Types     []string `json:"types"`
		Budget    string   `json:"budget"`
		MaxTokens int      `json:"max_tokens"`
		Trace     bool     `json:"trace"`
		Tags      []string `json:"tags"`
		TagsMatch string   `json:"tags_match"`
	}{
		Query: query, Types: []string{"world", "experience"}, Budget: "high",
		MaxTokens: 900, Trace: false, Tags: tags, TagsMatch: tagsMatch,
	}
	type recallResult struct {
		ID            string            `json:"id"`
		Text          string            `json:"text"`
		Type          *string           `json:"type"`
		Entities      []string          `json:"entities"`
		Context       *string           `json:"context"`
		OccurredStart *string           `json:"occurred_start"`
		OccurredEnd   *string           `json:"occurred_end"`
		MentionedAt   *string           `json:"mentioned_at"`
		DocumentID    *string           `json:"document_id"`
		Metadata      map[string]string `json:"metadata"`
		ChunkID       *string           `json:"chunk_id"`
		Tags          []string          `json:"tags"`
		SourceFactIDs []string          `json:"source_fact_ids"`
		Scores        json.RawMessage   `json:"scores"`
	}
	var response struct {
		Results     []recallResult  `json:"results"`
		Trace       json.RawMessage `json:"trace"`
		Entities    json.RawMessage `json:"entities"`
		Chunks      json.RawMessage `json:"chunks"`
		SourceFacts json.RawMessage `json:"source_facts"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		bankPath(bankID)+"/memories/recall",
		request,
		&response,
	); err != nil {
		return nil, err
	}
	logicalIDs := make([]string, 0, len(response.Results))
	for _, result := range response.Results {
		logicalID := result.Metadata["neo_memory_id"]
		if result.ID == "" || result.Text == "" || !validIdentifier(logicalID) {
			return nil, &Fault{Code: "recall_response_invalid"}
		}
		logicalIDs = append(logicalIDs, logicalID)
	}
	return logicalIDs, nil
}

func (client *HTTPClient) DeleteDocument(
	ctx context.Context,
	bankID string,
	documentID string,
) error {
	var response struct {
		Success            bool   `json:"success"`
		Message            string `json:"message"`
		DocumentID         string `json:"document_id"`
		MemoryUnitsDeleted int    `json:"memory_units_deleted"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodDelete,
		bankPath(bankID)+"/documents/"+url.PathEscape(documentID),
		nil,
		&response,
	); err != nil {
		return err
	}
	if !response.Success || response.DocumentID != documentID || response.MemoryUnitsDeleted < 0 {
		return &Fault{Code: "delete_response_invalid"}
	}
	return nil
}

func (client *HTTPClient) DeleteBank(ctx context.Context, bankID string) error {
	var response struct {
		Success      bool    `json:"success"`
		Message      *string `json:"message"`
		DeletedCount *int    `json:"deleted_count"`
	}
	if err := client.doJSON(ctx, http.MethodDelete, bankPath(bankID), nil, &response); err != nil {
		return err
	}
	if !response.Success {
		return &Fault{Code: "delete_response_invalid"}
	}
	return nil
}

func (client *HTTPClient) doJSON(
	ctx context.Context,
	method string,
	path string,
	requestValue any,
	responseValue any,
) error {
	var body io.Reader
	if requestValue != nil {
		encoded, err := json.Marshal(requestValue)
		if err != nil {
			return &Fault{Code: "request_invalid"}
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, body)
	if err != nil {
		return &Fault{Code: "request_invalid"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	if requestValue != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return &Fault{Code: "canceled"}
		case errors.Is(err, context.DeadlineExceeded):
			return &Fault{Code: "timeout"}
		default:
			return &Fault{Code: "request_failed"}
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		switch {
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			return &Fault{Code: "unauthorized"}
		case response.StatusCode >= 500:
			return &Fault{Code: "upstream_5xx"}
		default:
			return &Fault{Code: "upstream_4xx"}
		}
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return &Fault{Code: "response_read_failed"}
	}
	if len(responseBody) == 0 {
		return &Fault{Code: "malformed_response"}
	}
	if len(responseBody) > maximumResponseBytes {
		return &Fault{Code: "response_too_large"}
	}
	if err := strictjson.Decode(responseBody, maximumResponseBytes, responseValue); err != nil {
		return &Fault{Code: "malformed_response"}
	}
	return nil
}

func bankPath(bankID string) string {
	return "/v1/default/banks/" + url.PathEscape(bankID)
}

func recallTags(scope RecallScope) ([]string, string) {
	if scope.ConversationAlias != "" {
		return []string{"conversation:" + scope.ConversationAlias}, "any"
	}
	if scope.ProjectAlias != "" {
		return []string{"project:" + scope.ProjectAlias}, "any"
	}
	return []string{}, "exact"
}

func memoryTags(scope MemoryScope) []string {
	if scope.ConversationAlias != "" {
		return []string{"conversation:" + scope.ConversationAlias}
	}
	if scope.ProjectAlias != "" {
		return []string{"project:" + scope.ProjectAlias}
	}
	return []string{}
}
