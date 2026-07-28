package memoryworker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

var ErrEmbeddingProviderInvalid = errors.New("memory embedding provider is invalid")

type StoredRAGEmbeddingProvider struct {
	service *runtimeconfig.Service
}

func NewStoredRAGEmbeddingProvider(
	service *runtimeconfig.Service,
) *StoredRAGEmbeddingProvider {
	return &StoredRAGEmbeddingProvider{service: service}
}

func (provider *StoredRAGEmbeddingProvider) EmbedMemory(
	ctx context.Context,
	capture EmbeddingCapture,
) ([]float32, error) {
	var prepared bool
	capture, prepared = prepareEmbeddingProviderCapture(capture)
	if provider == nil || provider.service == nil ||
		strings.TrimSpace(capture.UserID) == "" ||
		strings.TrimSpace(capture.MemoryID) == "" ||
		!prepared ||
		capture.EmbeddingProfileID != string(ragproviders.RetrievalProfileSiliconFlow) ||
		capture.EmbeddingModelID != ragproviders.SiliconFlowEmbeddingModel ||
		capture.EmbeddingDimensions != ragproviders.SiliconFlowEmbeddingDimensions {
		return nil, ErrEmbeddingProviderInvalid
	}
	var payload runtimeconfig.StoredProviderConfigPayload
	if err := json.Unmarshal(capture.ProviderConfig, &payload); err != nil {
		return nil, ErrEmbeddingProviderInvalid
	}
	credential, err := provider.service.ResolveHydratedStoredRAGProviderCredential(
		runtimeconfig.StoredProviderConfig{
			ID: capture.ProviderRecordID, UserID: capture.UserID,
			ProviderID: capture.ProviderID, Label: capture.ProviderLabel,
			EncryptedSecretRef: capture.EncryptedSecretRef, Config: payload,
		},
	)
	if err != nil {
		return nil, err
	}
	resolver := &memoryEmbeddingCredentialResolver{credential: credential}
	gateway := ragproviders.NewProviderGateway(resolver)
	response, err := gateway.EmbedSiliconFlowPassages(
		ctx,
		ragproviders.PassageEmbeddingRequest{Passages: []ragproviders.PassageEmbeddingInput{{
			PassageID: capture.MemoryID,
			Text:      capture.Content,
		}}},
	)
	resolver.credential = ""
	credential = ""
	if err != nil || response.Model != ragproviders.SiliconFlowEmbeddingModel ||
		response.Dimensions != ragproviders.SiliconFlowEmbeddingDimensions ||
		len(response.Vectors) != 1 || response.Vectors[0].PassageID != capture.MemoryID ||
		!validMemoryEmbeddingVector(response.Vectors[0].Embedding, capture.EmbeddingDimensions) {
		return nil, ErrEmbeddingProviderInvalid
	}
	return append([]float32(nil), response.Vectors[0].Embedding...), nil
}

// prepareEmbeddingProviderCapture preserves the raw canonical hash and all
// lease fences while replacing the transient Provider body with its
// deterministic secret-redacted form. Sensitive content has already passed
// the SQL settings gate; credentials are never eligible for remote egress.
func prepareEmbeddingProviderCapture(capture EmbeddingCapture) (EmbeddingCapture, bool) {
	capture.Content = usermemory.RedactMemoryProviderText(capture.Content, true)
	return capture, strings.TrimSpace(capture.Content) != ""
}

type memoryEmbeddingCredentialResolver struct {
	credential string
}

func (resolver *memoryEmbeddingCredentialResolver) ResolveRAGProviderCredential(
	_ context.Context,
	providerID string,
) (string, error) {
	if resolver == nil || providerID != "siliconflow" ||
		strings.TrimSpace(resolver.credential) == "" {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	return resolver.credential, nil
}

func validMemoryEmbeddingVector(vector []float32, dimensions int) bool {
	if dimensions != ragproviders.SiliconFlowEmbeddingDimensions || len(vector) != dimensions {
		return false
	}
	norm := 0.0
	for _, component := range vector {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		norm += value * value
	}
	return norm > 0 && !math.IsInf(norm, 0)
}

var _ MemoryEmbeddingProvider = (*StoredRAGEmbeddingProvider)(nil)
