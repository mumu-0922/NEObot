package ragproviders

import "neo-chat/mm-chat/backend/internal/config"

const (
	ProviderStatusReady              = "ready"
	ProviderStatusMissingSecret      = "missing_secret"
	ProviderStatusActivationRequired = "activation_required"
	ProviderStatusUnavailable        = "unavailable"
)

type ProviderState struct {
	Configured          bool   `json:"configured"`
	Status              string `json:"status"`
	EmbeddingDimensions int    `json:"embeddingDimensions,omitempty"`
}

type ProviderStatuses struct {
	MinerU ProviderState `json:"mineru"`
	Jina   ProviderState `json:"jina"`
}

type StatusResponse struct {
	Providers ProviderStatuses `json:"providers"`
	Ready     bool             `json:"ready"`
}

func Status(cfg config.RAGConfig) StatusResponse {
	jinaDimensions := cfg.JinaEmbeddingDimensions
	if jinaDimensions <= 0 {
		jinaDimensions = config.DefaultRAGJinaDimensions
	}
	minerU := providerState(cfg.MinerUConfigured(), 0)
	jina := providerState(cfg.JinaConfigured(), jinaDimensions)

	return StatusResponse{
		Providers: ProviderStatuses{
			MinerU: minerU,
			Jina:   jina,
		},
		Ready: minerU.Configured && jina.Configured,
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
