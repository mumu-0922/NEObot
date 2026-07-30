package memorycapture

import (
	"context"
	"fmt"
	"math"
	"sync"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type transientCapture struct {
	assistantMessageID                   string
	candidates                           []string
	final                                []string
	providerSent                         []string
	rerankEgressReady                    bool
	judgeEgressReady                     bool
	cloudJudgeReady                      bool
	cloudJudgeInputTokenUpperBound       int
	memoryToolRouteReady                 bool
	memoryToolRouteUsed                  bool
	memoryToolRouteFailureCategory       string
	memoryToolRouteInputTokenUpperBound  int
	memoryToolRouteOutputTokenUpperBound int
	memoryIntentMargin                   float64
	memoryIntentReady                    bool
	admissionSimilarity                  float64
	admissionReady                       bool
	rerankReady                          bool
	rerankScores                         map[string]float64
}

type memoryToolRouteCaptureToken struct {
	generation uint64
}

func (recorder *Recorder) recordMemoryIntent(value ragproviders.MemoryIntentSignal) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.memoryIntentReady ||
		value.AnchorVersion != ragproviders.MemoryIntentAnchorVersion ||
		value.AnchorSHA256 != ragproviders.MemoryIntentAnchorSHA256 ||
		math.IsNaN(value.Margin) || math.IsInf(value.Margin, 0) ||
		value.Margin < -1 || value.Margin > 1 {
		return ErrCaptureStateConflict
	}
	recorder.current.memoryIntentMargin = value.Margin
	recorder.current.memoryIntentReady = true
	return nil
}

// Recorder coordinates repository and Provider decorators for one sequential
// case. It fails closed if calls overlap or identities drift.
type Recorder struct {
	mu                sync.Mutex
	current           *transientCapture
	currentGeneration uint64
	nextGeneration    uint64
}

func (recorder *Recorder) Begin(assistantMessageID string) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current != nil || assistantMessageID == "" {
		return ErrCaptureStateConflict
	}
	recorder.nextGeneration++
	if recorder.nextGeneration == 0 {
		recorder.nextGeneration++
	}
	recorder.currentGeneration = recorder.nextGeneration
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
	recorder.currentGeneration = 0
	return result, nil
}

func (recorder *Recorder) Abort() {
	recorder.mu.Lock()
	recorder.current = nil
	recorder.currentGeneration = 0
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

func (recorder *Recorder) recordProviderSent(stage string, documentCount int) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || documentCount != len(recorder.current.candidates) {
		return ErrCaptureStateConflict
	}
	switch stage {
	case "rerank":
		if recorder.current.rerankEgressReady {
			return ErrCaptureStateConflict
		}
		recorder.current.rerankEgressReady = true
	case "cloud_judge":
		if recorder.current.judgeEgressReady {
			return ErrCaptureStateConflict
		}
		recorder.current.judgeEgressReady = true
	default:
		return ErrCaptureStateConflict
	}
	if recorder.current.providerSent == nil {
		recorder.current.providerSent = append([]string(nil), recorder.current.candidates...)
	}
	return nil
}

func (recorder *Recorder) recordCloudJudgeResult(
	result usermemory.HybridCandidateJudgeResult,
	candidateCount int,
) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.cloudJudgeReady ||
		!recorder.current.judgeEgressReady ||
		candidateCount != len(recorder.current.candidates) ||
		result.PromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		result.PromptSHA256 != usermemory.HybridCandidateJudgePromptSHA256 {
		return ErrCaptureStateConflict
	}
	if _, err := usermemory.DecodeHybridCandidateJudgeOutput(
		result.RawOutput,
		candidateCount,
	); err != nil {
		return ErrCaptureStateConflict
	}
	recorder.current.cloudJudgeReady = true
	return nil
}

func (recorder *Recorder) recordCloudJudgeInput(
	input usermemory.HybridCandidateJudgeInput,
) error {
	systemPrompt, userPrompt, err := usermemory.BuildHybridCandidateJudgePrompt(input)
	if err != nil {
		return ErrCaptureStateConflict
	}
	// One UTF-8 byte per token plus fixed chat framing is conservative for the
	// Provider tokenizer while retaining no query or candidate plaintext.
	upperBound := len(systemPrompt) + len(userPrompt) + 32
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || !recorder.current.judgeEgressReady ||
		len(input.Candidates) != len(recorder.current.candidates) ||
		recorder.current.cloudJudgeInputTokenUpperBound != 0 || upperBound <= 0 {
		return ErrCaptureStateConflict
	}
	recorder.current.cloudJudgeInputTokenUpperBound = upperBound
	return nil
}

func (recorder *Recorder) recordMemoryToolRouteInput(
	input usermemory.HybridMemoryToolRouteInput,
) (memoryToolRouteCaptureToken, error) {
	// One UTF-8 byte per token plus the fixed Tool contract and chat framing is
	// conservative while retaining no query plaintext.
	upperBound := len(input.Query) + 1024
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil ||
		recorder.current.memoryToolRouteInputTokenUpperBound != 0 ||
		recorder.currentGeneration == 0 || upperBound <= 0 {
		return memoryToolRouteCaptureToken{}, ErrCaptureStateConflict
	}
	recorder.current.memoryToolRouteInputTokenUpperBound = upperBound
	return memoryToolRouteCaptureToken{generation: recorder.currentGeneration}, nil
}

func (recorder *Recorder) recordMemoryToolRouteResult(
	token memoryToolRouteCaptureToken,
	result usermemory.HybridMemoryToolRouteResult,
) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || token.generation == 0 ||
		token.generation != recorder.currentGeneration ||
		recorder.current.memoryToolRouteReady ||
		recorder.current.memoryToolRouteFailureCategory != "" ||
		recorder.current.memoryToolRouteInputTokenUpperBound <= 0 ||
		result.OutputTokenUpperBound <= 0 ||
		result.ContractVersion != usermemory.HybridMemoryToolContractVersion ||
		result.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 {
		return ErrCaptureStateConflict
	}
	recorder.current.memoryToolRouteReady = true
	recorder.current.memoryToolRouteUsed = result.UseMemory
	recorder.current.memoryToolRouteOutputTokenUpperBound = result.OutputTokenUpperBound
	return nil
}

func (recorder *Recorder) recordMemoryToolRouteFailure(
	token memoryToolRouteCaptureToken,
	category string,
) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || token.generation == 0 ||
		token.generation != recorder.currentGeneration ||
		recorder.current.memoryToolRouteReady ||
		recorder.current.memoryToolRouteFailureCategory != "" ||
		recorder.current.memoryToolRouteInputTokenUpperBound <= 0 ||
		!usermemory.ValidHybridMemoryToolRouteFailureCategory(category) {
		return ErrCaptureStateConflict
	}
	recorder.current.memoryToolRouteFailureCategory = category
	return nil
}

func (recorder *Recorder) recordAdmission(value usermemory.HybridShadowAdmission) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.admissionReady ||
		value.CandidateCount != len(recorder.current.candidates) {
		return ErrCaptureStateConflict
	}
	recorder.current.admissionSimilarity = value.MaximumVectorSimilarity
	recorder.current.admissionReady = true
	return nil
}

func (recorder *Recorder) recordRerankResults(values []ragproviders.RerankResult) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.current == nil || recorder.current.rerankScores != nil ||
		len(values) != len(recorder.current.candidates) {
		return ErrCaptureStateConflict
	}
	scores := make(map[string]float64, len(values))
	for _, value := range values {
		if value.Index < 0 || value.Index >= len(recorder.current.candidates) {
			return ErrCaptureStateConflict
		}
		if math.IsNaN(value.RelevanceScore) || math.IsInf(value.RelevanceScore, 0) ||
			value.RelevanceScore < 0 || value.RelevanceScore > 1 {
			return ErrCaptureStateConflict
		}
		memoryID := recorder.current.candidates[value.Index]
		if _, duplicate := scores[memoryID]; duplicate {
			return ErrCaptureStateConflict
		}
		scores[memoryID] = value.RelevanceScore
	}
	recorder.current.rerankScores = scores
	recorder.current.rerankReady = true
	return nil
}

func cloneTransientCapture(value transientCapture) transientCapture {
	if value.candidates != nil {
		value.candidates = append([]string{}, value.candidates...)
	}
	if value.final != nil {
		value.final = append([]string{}, value.final...)
	}
	if value.providerSent != nil {
		value.providerSent = append([]string{}, value.providerSent...)
	}
	if value.rerankScores != nil {
		scores := value.rerankScores
		value.rerankScores = make(map[string]float64, len(scores))
		for key, score := range scores {
			value.rerankScores[key] = score
		}
	}
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

func (decorator *RepositoryDecorator) AuthorizeHybridRerank(
	ctx context.Context,
	input usermemory.HybridShadowAdmissionInput,
) (usermemory.HybridShadowAdmission, error) {
	admission, ok := decorator.hybrid.(usermemory.HybridShadowAdmissionRepository)
	if !ok || admission == nil {
		return usermemory.HybridShadowAdmission{}, fmt.Errorf(
			"capture hybrid admission: %w",
			ErrCaptureUnavailable,
		)
	}
	result, err := admission.AuthorizeHybridRerank(ctx, input)
	if err != nil {
		return usermemory.HybridShadowAdmission{}, err
	}
	if err := decorator.recorder.recordAdmission(result); err != nil {
		return usermemory.HybridShadowAdmission{}, fmt.Errorf(
			"capture hybrid admission: %w",
			err,
		)
	}
	return result, nil
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

func (decorator *ProviderDecorator) ClassifyMemoryIntent(
	ctx context.Context,
	query string,
) (ragproviders.MemoryIntentSignal, error) {
	classifier, ok := decorator.provider.(ragproviders.MemoryIntentClassifier)
	if !ok || classifier == nil {
		return ragproviders.MemoryIntentSignal{}, ragproviders.ErrMemoryIntentUnavailable
	}
	result, err := classifier.ClassifyMemoryIntent(ctx, query)
	if err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, nil
	}
	if err := decorator.recorder.recordMemoryIntent(result); err != nil {
		return ragproviders.MemoryIntentSignal{}, fmt.Errorf(
			"capture hybrid Memory intent: %w",
			err,
		)
	}
	return result, nil
}

func (decorator *ProviderDecorator) Rerank(ctx context.Context, query string, documents []string) ([]ragproviders.RerankResult, error) {
	if err := decorator.recorder.recordProviderSent("rerank", len(documents)); err != nil {
		return nil, fmt.Errorf("capture hybrid Provider egress: %w", err)
	}
	results, err := decorator.provider.Rerank(ctx, query, documents)
	if err != nil {
		return results, err
	}
	// Production discards Provider output returned after the stage deadline.
	// Keep recorder state aligned so a live calibration cannot treat those
	// request-local, unusable scores as threshold authority.
	if ctx.Err() != nil {
		return results, nil
	}
	if err := decorator.recorder.recordRerankResults(results); err != nil {
		return nil, fmt.Errorf("capture hybrid rerank scores: %w", err)
	}
	return results, nil
}

type CandidateJudgeDecorator struct {
	judge           usermemory.HybridCandidateJudge
	recorder        *Recorder
	expectedModelID string
}

func NewCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
) (*CandidateJudgeDecorator, error) {
	if judge == nil || recorder == nil || expectedModelID == "" {
		return nil, ErrCaptureInvalid
	}
	return &CandidateJudgeDecorator{
		judge: judge, recorder: recorder, expectedModelID: expectedModelID,
	}, nil
}

func (decorator *CandidateJudgeDecorator) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if err := decorator.recorder.recordProviderSent(
		"cloud_judge",
		len(input.Candidates),
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, fmt.Errorf(
			"capture hybrid cloud-judge egress: %w",
			err,
		)
	}
	if err := decorator.recorder.recordCloudJudgeInput(input); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, fmt.Errorf(
			"capture hybrid cloud-judge input: %w",
			err,
		)
	}
	result, err := decorator.judge.JudgeHybridCandidates(ctx, input)
	if err != nil || ctx.Err() != nil {
		return result, err
	}
	if result.ModelID != decorator.expectedModelID {
		return usermemory.HybridCandidateJudgeResult{}, fmt.Errorf(
			"capture hybrid cloud-judge model drift: %w",
			ErrCaptureStateConflict,
		)
	}
	if err := decorator.recorder.recordCloudJudgeResult(
		result,
		len(input.Candidates),
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, fmt.Errorf(
			"capture hybrid cloud-judge result: %w",
			err,
		)
	}
	return result, nil
}

var (
	_ usermemory.HybridShadowRepository          = (*RepositoryDecorator)(nil)
	_ usermemory.HybridShadowAdmissionRepository = (*RepositoryDecorator)(nil)
	_ usermemory.HybridShadowProvider            = (*ProviderDecorator)(nil)
	_ usermemory.HybridCandidateJudge            = (*CandidateJudgeDecorator)(nil)
	_ usermemory.HybridMemoryToolRouter          = (*MemoryToolRouterDecorator)(nil)
	_ ragproviders.MemoryIntentClassifier        = (*ProviderDecorator)(nil)
)
