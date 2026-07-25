package ragproviders

const (
	MinerUAllocateEndpoint   = "https://mineru.net/api/v4/file-urls/batch"
	MinerUPollEndpointPrefix = "https://mineru.net/api/v4/extract-results/batch/"
	MinerUModelVersion       = "vlm"
	MinerUUploadHost         = "mineru.oss-cn-shanghai.aliyuncs.com"
	MinerUUploadPathPrefix   = "/api-upload/"
	MinerUResultHost         = "cdn-mineru.openxlab.org.cn"
	MinerUResultPathPrefix   = "/pdf/"
	MinerUResultPathSuffix   = ".zip"

	SiliconFlowEmbeddingsEndpoint  = "https://api.siliconflow.cn/v1/embeddings"
	SiliconFlowRerankEndpoint      = "https://api.siliconflow.cn/v1/rerank"
	SiliconFlowEmbeddingModel      = "Pro/BAAI/bge-m3"
	SiliconFlowRerankModel         = "Pro/BAAI/bge-reranker-v2-m3"
	SiliconFlowEmbeddingDimensions = 1024
)

type RetrievalProfileID string

const (
	RetrievalProfileSiliconFlow RetrievalProfileID = "siliconflow_bge_m3_v1"
)

type RetrievalProfile struct {
	ID                  RetrievalProfileID
	ProviderID          string
	EmbeddingModelID    string
	EmbeddingDimensions int
	RerankModelID       string
}

var SiliconFlowRetrievalProfile = RetrievalProfile{
	ID:                  RetrievalProfileSiliconFlow,
	ProviderID:          providerIDSiliconFlow,
	EmbeddingModelID:    SiliconFlowEmbeddingModel,
	EmbeddingDimensions: SiliconFlowEmbeddingDimensions,
	RerankModelID:       SiliconFlowRerankModel,
}

func ResolveRetrievalProfile(profileID RetrievalProfileID) (RetrievalProfile, error) {
	switch profileID {
	case RetrievalProfileSiliconFlow:
		return SiliconFlowRetrievalProfile, nil
	default:
		return RetrievalProfile{}, ErrProviderGatewayOperationUnsupported
	}
}
