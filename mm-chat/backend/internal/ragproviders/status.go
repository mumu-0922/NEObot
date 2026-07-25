package ragproviders

const (
	ProviderStatusReady              = "ready"
	ProviderStatusMissingSecret      = "missing_secret"
	ProviderStatusActivationRequired = "activation_required"
	ProviderStatusUnavailable        = "unavailable"
	ServiceStatusReady               = "ready"
	ServiceStatusPartial             = "partial"
	ServiceStatusUnavailable         = "unavailable"
)

type ProviderState struct {
	Configured          bool   `json:"configured"`
	Status              string `json:"status"`
	EmbeddingDimensions int    `json:"embeddingDimensions,omitempty"`
}

type ProviderStatuses struct {
	MinerU      ProviderState `json:"mineru"`
	SiliconFlow ProviderState `json:"siliconflow"`
}

type Capabilities struct {
	PDFParsing     bool `json:"pdfParsing"`
	NativeIndexing bool `json:"nativeIndexing"`
	Retrieval      bool `json:"retrieval"`
}

type StatusResponse struct {
	Providers    ProviderStatuses `json:"providers"`
	Status       string           `json:"status"`
	Capabilities Capabilities     `json:"capabilities"`
	Ready        bool             `json:"ready"`
}

func Status() StatusResponse {
	minerU := providerState(false, 0)
	siliconFlow := providerState(false, SiliconFlowEmbeddingDimensions)

	return StatusResponse{
		Providers: ProviderStatuses{
			MinerU:      minerU,
			SiliconFlow: siliconFlow,
		},
		Status: ServiceStatusUnavailable,
		Ready:  false,
	}
}

func providerState(configured bool, dimensions int) ProviderState {
	status := ProviderStatusMissingSecret
	if configured {
		status = ProviderStatusReady
	}
	state := ProviderState{
		Configured: configured,
		Status:     status,
	}
	if dimensions > 0 {
		state.EmbeddingDimensions = dimensions
	}
	return state
}
