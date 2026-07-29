package memorycapture

import (
	"context"
	"fmt"
	"sync"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type transientCapture struct {
	assistantMessageID string
	candidates         []string
	final              []string
	providerSent       []string
}

// Recorder coordinates repository and Provider decorators for one sequential
// case. It fails closed if calls overlap or identities drift.
type Recorder struct {
	mu      sync.Mutex
	current *transientCapture
}

func (recorder *Recorder) Begin(assistantMessageID string) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current != nil || assistantMessageID == "" {
		return ErrCaptureStateConflict
	}
	recorder.current = &transientCapture{assistantMessageID: assistantMessageID}
	return nil
}

func (recorder *Recorder) Finish(assistantMessageID string) (transientCapture, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.assistantMessageID != assistantMessageID {
		return transientCapture{}, ErrCaptureStateConflict
	}
	result := cloneTransientCapture(*recorder.current)
	recorder.current = nil
	return result, nil
}

func (recorder *Recorder) Abort() {
	recorder.mu.Lock()
	recorder.current = nil
	recorder.mu.Unlock()
}

func (recorder *Recorder) recordPrepared(assistantMessageID string, values []usermemory.HybridShadowCandidate) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.assistantMessageID != assistantMessageID || recorder.current.candidates != nil {
		return ErrCaptureStateConflict
	}
	recorder.current.candidates = make([]string, len(values))
	for index, value := range values {
		recorder.current.candidates[index] = value.MemoryID
	}
	return nil
}

func (recorder *Recorder) recordFinal(assistantMessageID string, values []usermemory.HybridShadowRankedItem) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.assistantMessageID != assistantMessageID || recorder.current.final != nil {
		return ErrCaptureStateConflict
	}
	recorder.current.final = make([]string, len(values))
	for index, value := range values {
		recorder.current.final[index] = value.MemoryID
	}
	return nil
}

func (recorder *Recorder) recordProviderSent(documentCount int) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || documentCount != len(recorder.current.candidates) || recorder.current.providerSent != nil {
		return ErrCaptureStateConflict
	}
	recorder.current.providerSent = append([]string(nil), recorder.current.candidates...)
	return nil
}

func cloneTransientCapture(value transientCapture) transientCapture {
	value.candidates = append([]string(nil), value.candidates...)
	value.final = append([]string(nil), value.final...)
	value.providerSent = append([]string(nil), value.providerSent...)
	return value
}

// RepositoryDecorator preserves the complete v1 Repository contract while
// capturing typed hybrid inputs/outputs around the production implementation.
type RepositoryDecorator struct {
	usermemory.Repository
	hybrid   usermemory.HybridShadowRepository
	recorder *Recorder
}

func NewRepositoryDecorator(
	repository usermemory.Repository,
	hybrid usermemory.HybridShadowRepository,
	recorder *Recorder,
) (*RepositoryDecorator, error) {
	if repository == nil || hybrid == nil || recorder == nil {
		return nil, ErrCaptureInvalid
	}
	return &RepositoryDecorator{Repository: repository, hybrid: hybrid, recorder: recorder}, nil
}

func (decorator *RepositoryDecorator) PrepareHybridShadow(
	ctx context.Context,
	input usermemory.HybridShadowPrepareInput,
) (usermemory.HybridShadowPreparation, error) {
	prepared, err := decorator.hybrid.PrepareHybridShadow(ctx, input)
	if err != nil {
		return prepared, err
	}
	if err := decorator.recorder.recordPrepared(input.AssistantMessageID, prepared.Candidates); err != nil {
		return usermemory.HybridShadowPreparation{}, fmt.Errorf("capture hybrid candidates: %w", err)
	}
	return prepared, nil
}

func (decorator *RepositoryDecorator) RecordHybridShadow(
	ctx context.Context,
	input usermemory.HybridShadowRecordInput,
) (usermemory.HybridShadowSummary, error) {
	if err := decorator.recorder.recordFinal(input.AssistantMessageID, input.Final); err != nil {
		return usermemory.HybridShadowSummary{}, fmt.Errorf("capture hybrid final: %w", err)
	}
	return decorator.hybrid.RecordHybridShadow(ctx, input)
}

// ProviderDecorator records the exact authorized candidate ID set whenever
// the production reranker is actually called. Query embeddings carry no
// Memory ID surface.
type ProviderDecorator struct {
	provider usermemory.HybridShadowProvider
	recorder *Recorder
}

func NewProviderDecorator(provider usermemory.HybridShadowProvider, recorder *Recorder) (*ProviderDecorator, error) {
	if provider == nil || recorder == nil {
		return nil, ErrCaptureInvalid
	}
	return &ProviderDecorator{provider: provider, recorder: recorder}, nil
}

func (decorator *ProviderDecorator) EmbedQuery(ctx context.Context, query string) (ragproviders.QueryEmbedding, error) {
	return decorator.provider.EmbedQuery(ctx, query)
}

func (decorator *ProviderDecorator) Rerank(ctx context.Context, query string, documents []string) ([]ragproviders.RerankResult, error) {
	if err := decorator.recorder.recordProviderSent(len(documents)); err != nil {
		return nil, fmt.Errorf("capture hybrid Provider egress: %w", err)
	}
	return decorator.provider.Rerank(ctx, query, documents)
}

var (
	_ usermemory.HybridShadowRepository = (*RepositoryDecorator)(nil)
	_ usermemory.HybridShadowProvider   = (*ProviderDecorator)(nil)
)
