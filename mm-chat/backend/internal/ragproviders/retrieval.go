package ragproviders

import (
	"context"
	"errors"
	"strings"
)

const (
	maxQueryEmbeddingQueryBytes = 2048
	maxRerankDocuments          = 20
	maxRerankQueryBytes         = 2048
	maxRerankDocumentBytes      = 64 * 1024
	maxRerankTotalDocumentBytes = 1024 * 1024
)

var (
	ErrQueryEmbeddingUnavailable = errors.New("query embedding unavailable")
	ErrQueryEmbeddingInvalid     = errors.New("query embedding invalid")
	ErrRerankUnavailable         = errors.New("rerank unavailable")
	ErrRerankInvalid             = errors.New("rerank invalid")
)

type QueryEmbedding struct {
	ModelID    string
	Dimensions int
	Vector     []float32
}

type QueryEmbedder interface {
	EmbedQuery(context.Context, string) (QueryEmbedding, error)
}

type RerankResult struct {
	Index          int
	RelevanceScore float64
}

type Reranker interface {
	Rerank(context.Context, string, []string) ([]RerankResult, error)
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
		if strings.TrimSpace(document) == "" ||
			documentBytes > maxRerankDocumentBytes {
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
