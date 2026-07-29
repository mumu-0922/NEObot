package ragproviders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	ErrMemoryIntentUnavailable   = errors.New("memory intent unavailable")
	ErrMemoryIntentInvalid       = errors.New("memory intent invalid")
)

const MemoryIntentAnchorVersion = "memory-intent-bilingual-anchors-v1"

var memoryIntentAnchorDocuments = []string{
	"需要结合用户已保存的个人记忆才能更好回答，包括用户偏好、身份与经历、长期目标、正在进行的项目、先前决定、约束、纠正或个性化建议。 A request where stored personal memory about the user, their preferences, history, goals, projects, prior decisions, constraints, corrections, or personalization is useful.",
	"不需要任何用户个人记忆即可回答，包括通用知识、公开事实、翻译、计算、代码语法、实时信息或与用户历史无关的全新任务。 A general request answerable without any stored personal memory, such as public facts, translation, calculation, code syntax, real-time information, or a new task unrelated to user history.",
}

var MemoryIntentAnchorSHA256 = func() string {
	digest := sha256.Sum256([]byte(strings.Join(memoryIntentAnchorDocuments, "\x00")))
	return hex.EncodeToString(digest[:])
}()

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

type MemoryIntentSignal struct {
	AnchorVersion string
	AnchorSHA256  string
	PositiveScore float64
	NegativeScore float64
	Margin        float64
}

type MemoryIntentClassifier interface {
	ClassifyMemoryIntent(context.Context, string) (MemoryIntentSignal, error)
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
