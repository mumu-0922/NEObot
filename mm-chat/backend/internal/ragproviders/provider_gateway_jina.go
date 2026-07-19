package ragproviders

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
)

const (
	jinaPassageTask             = "retrieval.passage"
	jinaQueryTask               = "retrieval.query"
	jinaMaxPassages             = 256
	jinaMaxPassageBytes         = 64 << 10
	jinaMaxTotalPassageBytes    = 4 << 20
	jinaGatewayMaxResponseBytes = 16 << 20
)

var passageIDRE = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

type jinaEmbeddingWireResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

type jinaRerankWireResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

func (gateway *ProviderGateway) EmbedPassages(
	ctx context.Context,
	input PassageEmbeddingRequest,
) (PassageEmbeddingResponse, error) {
	passageIDs, texts, err := normalizePassageEmbeddingRequest(input)
	if err != nil {
		return PassageEmbeddingResponse{}, err
	}
	vectors, err := gateway.embedJina(ctx, jinaPassageTask, texts)
	if err != nil {
		return PassageEmbeddingResponse{}, err
	}
	response := PassageEmbeddingResponse{
		Model: JinaEmbeddingModel, Dimensions: JinaEmbeddingDimensions,
		Vectors: make([]PassageEmbeddingVector, len(vectors)),
	}
	for index := range vectors {
		response.Vectors[index] = PassageEmbeddingVector{
			PassageID: passageIDs[index], Embedding: vectors[index],
		}
	}
	return response, nil
}

func (gateway *ProviderGateway) EmbedQuery(
	ctx context.Context,
	query string,
) (QueryEmbedding, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]byte(query)) > maxQueryEmbeddingQueryBytes {
		return QueryEmbedding{}, ErrQueryEmbeddingInvalid
	}
	vectors, err := gateway.embedJina(ctx, jinaQueryTask, []string{query})
	if err != nil {
		if errors.Is(err, ErrProviderGatewayInvalid) {
			return QueryEmbedding{}, ErrQueryEmbeddingInvalid
		}
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	return QueryEmbedding{
		ModelID: JinaEmbeddingModel, Dimensions: JinaEmbeddingDimensions,
		Vector: vectors[0],
	}, nil
}

func (gateway *ProviderGateway) Rerank(
	ctx context.Context,
	query string,
	documents []string,
) ([]RerankResult, error) {
	query, documents, err := normalizeDirectRerankRequest(query, documents)
	if err != nil {
		return nil, ErrRerankInvalid
	}
	results, err := gateway.rerankJina(ctx, query, documents)
	if err != nil {
		if errors.Is(err, ErrProviderGatewayInvalid) {
			return nil, ErrRerankInvalid
		}
		return nil, ErrRerankUnavailable
	}
	return results, nil
}

func (gateway *ProviderGateway) embedJina(
	ctx context.Context,
	task string,
	texts []string,
) ([][]float32, error) {
	if (task != jinaPassageTask && task != jinaQueryTask) || len(texts) == 0 {
		return nil, ErrProviderGatewayInvalid
	}
	inputs := make([]map[string]string, len(texts))
	for index, text := range texts {
		inputs[index] = map[string]string{"text": text}
	}
	body, err := json.Marshal(map[string]any{
		"dimensions":             JinaEmbeddingDimensions,
		"embedding_type":         "float",
		"input":                  inputs,
		"late_chunking":          false,
		"model":                  JinaEmbeddingModel,
		"return_multivector":     false,
		"return_tokenized_input": false,
		"task":                   task,
		"truncate":               false,
	})
	if err != nil {
		return nil, ErrProviderGatewayInvalid
	}
	credential, err := gateway.resolveCredential(ctx, providerIDJina)
	if err != nil {
		return nil, err
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodPost,
		JinaEmbeddingsEndpoint,
		credential,
		body,
		jinaGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return nil, err
	}
	return normalizeJinaEmbeddingResponse(raw, len(texts))
}

func (gateway *ProviderGateway) rerankJina(
	ctx context.Context,
	query string,
	documents []string,
) ([]RerankResult, error) {
	body, err := json.Marshal(map[string]any{
		"documents":         documents,
		"model":             JinaRerankModel,
		"query":             query,
		"return_documents":  false,
		"return_embeddings": false,
		"top_n":             len(documents),
	})
	if err != nil {
		return nil, ErrProviderGatewayInvalid
	}
	credential, err := gateway.resolveCredential(ctx, providerIDJina)
	if err != nil {
		return nil, err
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodPost,
		JinaRerankEndpoint,
		credential,
		body,
		jinaGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return nil, err
	}
	return normalizeJinaRerankResponse(raw, len(documents))
}

func normalizePassageEmbeddingRequest(
	input PassageEmbeddingRequest,
) ([]string, []string, error) {
	if len(input.Passages) < 1 || len(input.Passages) > jinaMaxPassages {
		return nil, nil, ErrProviderGatewayInvalid
	}
	passageIDs := make([]string, len(input.Passages))
	texts := make([]string, len(input.Passages))
	seen := make(map[string]struct{}, len(input.Passages))
	totalBytes := 0
	for index, passage := range input.Passages {
		passageID := passage.PassageID
		textBytes := len([]byte(passage.Text))
		if passageID != strings.TrimSpace(passageID) || !passageIDRE.MatchString(passageID) ||
			passageID == "00000000-0000-0000-0000-000000000000" ||
			strings.TrimSpace(passage.Text) == "" || textBytes > jinaMaxPassageBytes {
			return nil, nil, ErrProviderGatewayInvalid
		}
		if _, duplicate := seen[passageID]; duplicate {
			return nil, nil, ErrProviderGatewayInvalid
		}
		totalBytes += textBytes
		if totalBytes > jinaMaxTotalPassageBytes {
			return nil, nil, ErrProviderGatewayInvalid
		}
		seen[passageID] = struct{}{}
		passageIDs[index] = passageID
		texts[index] = passage.Text
	}
	return passageIDs, texts, nil
}

func normalizeJinaEmbeddingResponse(raw []byte, count int) ([][]float32, error) {
	var payload jinaEmbeddingWireResponse
	if decodeProviderJSON(raw, &payload, false) != nil ||
		payload.Model != JinaEmbeddingModel || len(payload.Data) != count {
		return nil, ErrProviderGatewayUpstream
	}
	vectors := make([][]float32, count)
	seen := make([]bool, count)
	for _, item := range payload.Data {
		if item.Index < 0 || item.Index >= count || seen[item.Index] ||
			len(item.Embedding) != JinaEmbeddingDimensions {
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
		seen[item.Index] = true
		vectors[item.Index] = vector
	}
	for _, present := range seen {
		if !present {
			return nil, ErrProviderGatewayUpstream
		}
	}
	return vectors, nil
}

func normalizeDirectRerankRequest(
	query string,
	documents []string,
) (string, []string, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]byte(query)) > maxRerankQueryBytes ||
		len(documents) < 1 || len(documents) > maxRerankDocuments {
		return "", nil, ErrProviderGatewayInvalid
	}
	totalBytes := 0
	copyDocuments := make([]string, len(documents))
	for index, document := range documents {
		documentBytes := len([]byte(document))
		if strings.TrimSpace(document) == "" || documentBytes > maxRerankDocumentBytes {
			return "", nil, ErrProviderGatewayInvalid
		}
		totalBytes += documentBytes
		if totalBytes > maxRerankTotalDocumentBytes {
			return "", nil, ErrProviderGatewayInvalid
		}
		copyDocuments[index] = document
	}
	return query, copyDocuments, nil
}

func normalizeJinaRerankResponse(raw []byte, documentCount int) ([]RerankResult, error) {
	var payload jinaRerankWireResponse
	if decodeProviderJSON(raw, &payload, false) != nil ||
		payload.Model != JinaRerankModel || len(payload.Results) != documentCount {
		return nil, ErrProviderGatewayUpstream
	}
	seen := make([]bool, documentCount)
	results := make([]RerankResult, 0, documentCount)
	for _, result := range payload.Results {
		if result.Index < 0 || result.Index >= documentCount || seen[result.Index] ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) {
			return nil, ErrProviderGatewayUpstream
		}
		seen[result.Index] = true
		results = append(results, RerankResult{
			Index: result.Index, RelevanceScore: result.RelevanceScore,
		})
	}
	for _, present := range seen {
		if !present {
			return nil, ErrProviderGatewayUpstream
		}
	}
	return results, nil
}
