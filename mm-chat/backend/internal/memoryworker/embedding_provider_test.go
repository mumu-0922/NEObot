package memoryworker

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestPrepareEmbeddingProviderCaptureRedactsOnlyTransientBody(t *testing.T) {
	raw := EmbeddingCapture{
		Content:     "token: fixture-private-value. Keep answers concise.",
		ContentHash: strings.Repeat("a", 64),
	}
	prepared, ok := prepareEmbeddingProviderCapture(raw)
	if !ok || strings.Contains(prepared.Content, "fixture-private-value") ||
		!strings.Contains(prepared.Content, "Keep answers concise") ||
		prepared.ContentHash != raw.ContentHash ||
		!strings.Contains(raw.Content, "fixture-private-value") {
		t.Fatalf("prepared embedding capture = raw:%#v prepared:%#v", raw, prepared)
	}
	_, ok = prepareEmbeddingProviderCapture(EmbeddingCapture{
		Content: "password: fixture-secret-value",
	})
	if ok {
		t.Fatal("secret-only embedding body remained provider eligible")
	}
}

func TestStoredRAGEmbeddingProviderRejectsUnboundCaptureBeforeNetwork(t *testing.T) {
	provider := NewStoredRAGEmbeddingProvider(nil)
	_, err := provider.EmbedMemory(context.Background(), EmbeddingCapture{
		UserID:              "11111111-1111-4111-8111-111111111111",
		MemoryID:            "22222222-2222-4222-8222-222222222222",
		Content:             "private memory",
		EmbeddingProfileID:  string(ragproviders.RetrievalProfileSiliconFlow),
		EmbeddingModelID:    ragproviders.SiliconFlowEmbeddingModel,
		EmbeddingDimensions: ragproviders.SiliconFlowEmbeddingDimensions,
	})
	if !errors.Is(err, ErrEmbeddingProviderInvalid) {
		t.Fatalf("nil runtime service error = %v", err)
	}
}

func TestValidMemoryEmbeddingVectorRequiresFiniteNonZeroBGEVector(t *testing.T) {
	valid := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	valid[0] = 1
	if !validMemoryEmbeddingVector(valid, ragproviders.SiliconFlowEmbeddingDimensions) {
		t.Fatal("valid BGE vector was rejected")
	}
	for name, vector := range map[string][]float32{
		"zero":  make([]float32, ragproviders.SiliconFlowEmbeddingDimensions),
		"short": {1},
		"nan": func() []float32 {
			value := append([]float32(nil), valid...)
			value[0] = float32(math.NaN())
			return value
		}(),
		"infinite": func() []float32 {
			value := append([]float32(nil), valid...)
			value[0] = float32(math.Inf(1))
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if validMemoryEmbeddingVector(vector, ragproviders.SiliconFlowEmbeddingDimensions) {
				t.Fatalf("%s vector was accepted", name)
			}
		})
	}
}
