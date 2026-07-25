package ragproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
)

var passageIDRE = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

const (
	siliconFlowMaxPassages             = 32
	siliconFlowMaxPassageBytes         = 8 << 10
	siliconFlowMaxTotalPassageBytes    = 256 << 10
	siliconFlowGatewayMaxResponseBytes = 4 << 20
)

type siliconFlowEmbeddingWireResponse struct {
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     *int      `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
		TotalTokens      *int `json:"total_tokens"`
	} `json:"usage"`
}

type siliconFlowRerankWireResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Document       json.RawMessage `json:"document,omitempty"`
		Index          *int            `json:"index"`
		RelevanceScore *float64        `json:"relevance_score"`
	} `json:"results"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

func (gateway *ProviderGateway) EmbedSiliconFlowPassages(
	ctx context.Context,
	input PassageEmbeddingRequest,
) (PassageEmbeddingResponse, error) {
	passageIDs, texts, err := normalizeSiliconFlowPassageEmbeddingRequest(input)
	if err != nil {
		return PassageEmbeddingResponse{}, err
	}
	vectors, err := gateway.embedSiliconFlow(ctx, texts)
	if err != nil {
		return PassageEmbeddingResponse{}, err
	}
	response := PassageEmbeddingResponse{
		Model:      SiliconFlowEmbeddingModel,
		Dimensions: SiliconFlowEmbeddingDimensions,
		Vectors:    make([]PassageEmbeddingVector, len(vectors)),
	}
	for index := range vectors {
		response.Vectors[index] = PassageEmbeddingVector{
			PassageID: passageIDs[index],
			Embedding: vectors[index],
		}
	}
	return response, nil
}

func (gateway *ProviderGateway) embedSiliconFlowQuery(
	ctx context.Context,
	query string,
) (QueryEmbedding, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]byte(query)) > maxQueryEmbeddingQueryBytes {
		return QueryEmbedding{}, ErrQueryEmbeddingInvalid
	}
	vectors, err := gateway.embedSiliconFlow(ctx, []string{query})
	if err != nil {
		if errors.Is(err, ErrProviderGatewayInvalid) {
			return QueryEmbedding{}, ErrQueryEmbeddingInvalid
		}
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	return QueryEmbedding{
		ModelID:    SiliconFlowEmbeddingModel,
		Dimensions: SiliconFlowEmbeddingDimensions,
		Vector:     vectors[0],
	}, nil
}

// EmbedQuery deliberately fails without an immutable retrieval-profile bind.
// Legacy Jina Generations must degrade to their same-generation BM25 lane;
// using BGE here would cross vector spaces.
func (gateway *ProviderGateway) EmbedQuery(
	context.Context,
	string,
) (QueryEmbedding, error) {
	return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
}

func (gateway *ProviderGateway) rerankSiliconFlow(
	ctx context.Context,
	query string,
	documents []string,
) ([]RerankResult, error) {
	query, documents, err := normalizeDirectRerankRequest(query, documents)
	if err != nil {
		return nil, ErrRerankInvalid
	}
	body, err := json.Marshal(map[string]any{
		"documents":        documents,
		"model":            SiliconFlowRerankModel,
		"query":            query,
		"return_documents": false,
		"top_n":            len(documents),
	})
	if err != nil {
		return nil, ErrRerankInvalid
	}
	credential, err := gateway.resolveCredential(ctx, providerIDSiliconFlow)
	if err != nil {
		return nil, ErrRerankUnavailable
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodPost,
		SiliconFlowRerankEndpoint,
		credential,
		body,
		siliconFlowGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return nil, ErrRerankUnavailable
	}
	results, err := normalizeSiliconFlowRerankResponse(raw, len(documents))
	if err != nil {
		return nil, ErrRerankUnavailable
	}
	return results, nil
}

// Rerank deliberately fails without an immutable retrieval-profile bind.
func (gateway *ProviderGateway) Rerank(
	context.Context,
	string,
	[]string,
) ([]RerankResult, error) {
	return nil, ErrRerankUnavailable
}

func (gateway *ProviderGateway) embedSiliconFlow(
	ctx context.Context,
	texts []string,
) ([][]float32, error) {
	if len(texts) < 1 || len(texts) > siliconFlowMaxPassages {
		return nil, ErrProviderGatewayInvalid
	}
	for _, value := range texts {
		if strings.TrimSpace(value) == "" ||
			len([]byte(value)) > siliconFlowMaxPassageBytes {
			return nil, ErrProviderGatewayInvalid
		}
	}
	body, err := json.Marshal(map[string]any{
		"encoding_format": "float",
		"input":           texts,
		"model":           SiliconFlowEmbeddingModel,
	})
	if err != nil {
		return nil, ErrProviderGatewayInvalid
	}
	credential, err := gateway.resolveCredential(ctx, providerIDSiliconFlow)
	if err != nil {
		return nil, err
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodPost,
		SiliconFlowEmbeddingsEndpoint,
		credential,
		body,
		siliconFlowGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return nil, err
	}
	return normalizeSiliconFlowEmbeddingResponse(raw, len(texts))
}

func normalizeSiliconFlowPassageEmbeddingRequest(
	input PassageEmbeddingRequest,
) ([]string, []string, error) {
	if len(input.Passages) < 1 || len(input.Passages) > siliconFlowMaxPassages {
		return nil, nil, ErrProviderGatewayInvalid
	}
	passageIDs := make([]string, len(input.Passages))
	texts := make([]string, len(input.Passages))
	seen := make(map[string]struct{}, len(input.Passages))
	totalBytes := 0
	for index, passage := range input.Passages {
		passageID := passage.PassageID
		textBytes := len([]byte(passage.Text))
		if passageID != strings.TrimSpace(passageID) ||
			!passageIDRE.MatchString(passageID) ||
			passageID == "00000000-0000-0000-0000-000000000000" ||
			strings.TrimSpace(passage.Text) == "" ||
			textBytes > siliconFlowMaxPassageBytes {
			return nil, nil, ErrProviderGatewayInvalid
		}
		if _, duplicate := seen[passageID]; duplicate {
			return nil, nil, ErrProviderGatewayInvalid
		}
		totalBytes += textBytes
		if totalBytes > siliconFlowMaxTotalPassageBytes {
			return nil, nil, ErrProviderGatewayInvalid
		}
		seen[passageID] = struct{}{}
		passageIDs[index] = passageID
		texts[index] = passage.Text
	}
	return passageIDs, texts, nil
}

func normalizeSiliconFlowEmbeddingResponse(raw []byte, count int) ([][]float32, error) {
	var payload siliconFlowEmbeddingWireResponse
	if decodeProviderJSON(raw, &payload, true) != nil ||
		payload.Object != "list" ||
		payload.Model != SiliconFlowEmbeddingModel ||
		len(payload.Data) != count ||
		payload.Usage.PromptTokens == nil ||
		payload.Usage.CompletionTokens == nil ||
		payload.Usage.TotalTokens == nil ||
		*payload.Usage.PromptTokens < 0 ||
		*payload.Usage.CompletionTokens < 0 ||
		*payload.Usage.TotalTokens < 0 {
		return nil, ErrProviderGatewayUpstream
	}
	vectors := make([][]float32, count)
	seen := make([]bool, count)
	for _, item := range payload.Data {
		if item.Object != "embedding" || item.Index == nil ||
			*item.Index < 0 || *item.Index >= count || seen[*item.Index] ||
			len(item.Embedding) != SiliconFlowEmbeddingDimensions {
			return nil, ErrProviderGatewayUpstream
		}
		vector := make([]float32, len(item.Embedding))
		norm := 0.0
		for index, component := range item.Embedding {
			converted := float32(component)
			if math.IsNaN(component) || math.IsInf(component, 0) ||
				math.IsInf(float64(converted), 0) {
				return nil, ErrProviderGatewayUpstream
			}
			vector[index] = converted
			norm += float64(converted) * float64(converted)
		}
		if norm <= 0 || math.IsInf(norm, 0) {
			return nil, ErrProviderGatewayUpstream
		}
		seen[*item.Index] = true
		vectors[*item.Index] = vector
	}
	for _, present := range seen {
		if !present {
			return nil, ErrProviderGatewayUpstream
		}
	}
	return vectors, nil
}

func normalizeSiliconFlowRerankResponse(
	raw []byte,
	documentCount int,
) ([]RerankResult, error) {
	var payload siliconFlowRerankWireResponse
	if decodeProviderJSON(raw, &payload, true) != nil ||
		!validSiliconFlowResponseID(payload.ID) ||
		len(payload.Results) != documentCount ||
		(len(payload.Meta) > 0 && !json.Valid(payload.Meta)) {
		return nil, ErrProviderGatewayUpstream
	}
	seen := make([]bool, documentCount)
	results := make([]RerankResult, 0, documentCount)
	for _, result := range payload.Results {
		if result.Index == nil || result.RelevanceScore == nil ||
			!validSiliconFlowOmittedDocument(result.Document) ||
			*result.Index < 0 || *result.Index >= documentCount ||
			seen[*result.Index] || math.IsNaN(*result.RelevanceScore) ||
			math.IsInf(*result.RelevanceScore, 0) ||
			*result.RelevanceScore < 0 || *result.RelevanceScore > 1 {
			return nil, ErrProviderGatewayUpstream
		}
		seen[*result.Index] = true
		results = append(results, RerankResult{
			Index:          *result.Index,
			RelevanceScore: *result.RelevanceScore,
		})
	}
	for _, present := range seen {
		if !present {
			return nil, ErrProviderGatewayUpstream
		}
	}
	return results, nil
}

func validSiliconFlowOmittedDocument(document json.RawMessage) bool {
	return len(document) == 0 || bytes.Equal(bytes.TrimSpace(document), []byte("null"))
}

func validSiliconFlowResponseID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len([]byte(value)) > 256 {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

var _ ProviderOperations = (*ProviderGateway)(nil)
