package usermemory

import "context"

const (
	HybridMemoryToolName                = "search_memory"
	HybridMemoryToolContractVersion     = "memory-search-tool-v1"
	HybridMemoryToolContractSHA256      = "f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6"
	HybridMemoryToolDecodingProfile     = "memory-search-tool-decoding-v1"
	HybridMemoryToolMaximumOutputTokens = 128
	HybridMemoryToolTemperature         = 0.0
	HybridMemoryToolDisableThinking     = true
)

// HybridMemoryToolRouter represents the current chat model's automatic Tool
// decision. The router receives only the already-secret-redacted query and
// the fixed Tool definition; Memory candidate bodies are never part of this
// boundary.
type HybridMemoryToolRouter interface {
	RouteHybridMemory(
		context.Context,
		HybridMemoryToolRouteInput,
	) (HybridMemoryToolRouteResult, error)
}

type HybridMemoryToolRouteInput struct {
	Query string
}

type HybridMemoryToolRouteResult struct {
	UseMemory       bool
	ModelID         string
	ContractVersion string
	ContractSHA256  string
}
