package ragproviders

import (
	"context"
	"errors"
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
