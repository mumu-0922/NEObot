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

	JinaEmbeddingsEndpoint  = "https://api.jina.ai/v1/embeddings"
	JinaRerankEndpoint      = "https://api.jina.ai/v1/rerank"
	JinaEmbeddingModel      = "jina-embeddings-v4"
	JinaRerankModel         = "jina-reranker-v3"
	JinaEmbeddingDimensions = 1024
)
