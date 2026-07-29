package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	hybridShadowHardCutoff    = 2 * time.Second
	hybridShadowEmbedCutoff   = 750 * time.Millisecond
	hybridShadowRecordReserve = 150 * time.Millisecond
	HybridShadowTargetTokens  = 600
	HybridShadowMaximumTokens = 900
	HybridShadowFinalLimit    = 5
	hybridShadowTargetTokens  = HybridShadowTargetTokens
	hybridShadowMaximumTokens = HybridShadowMaximumTokens
	hybridShadowFinalLimit    = HybridShadowFinalLimit
	hybridShadowTokenOverhead = 24
)

var errHybridProviderTextRedacted = errors.New("memory hybrid provider text redacted")

// HybridShadowProvider must already be bound by the composition root to the
// immutable SiliconFlow BGE retrieval profile. The server resolver rejects a
// raw or differently bound gateway before it can reach this service.
type HybridShadowProvider interface {
	ragproviders.QueryEmbedder
	ragproviders.Reranker
}

// SearchRelevantWithHybridShadow always executes the v1 reader first. Hybrid
// candidates and rerank output are diagnostic only and never replace returned
// items, prompt content, or Usage authority.
func (s *Service) SearchRelevantWithHybridShadow(
	ctx context.Context,
	query string,
	conversationID string,
	assistantMessageID string,
	limit int,
) ([]Memory, HybridShadowSummary, error) {
	items, err := s.SearchRelevant(ctx, query, limit)
	if err != nil {
		return nil, HybridShadowSummary{}, err
	}
	failure := hybridShadowFailure("PREPARE_UNAVAILABLE", "PROVIDER_UNAVAILABLE")
	repository, ok := s.repo.(HybridShadowRepository)
	if !ok || repository == nil {
		return items, failure, nil
	}
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(conversationID) || !uuidRE.MatchString(assistantMessageID) || query == "" {
		return items, hybridShadowFailure("ARGUMENT_INVALID", "NONE"), nil
	}
	observationID, err := newUUID()
	if err != nil {
		return items, hybridShadowFailure("OBSERVATION_ID_FAILED", "NONE"), nil
	}
	baseline := make([]LexicalShadowBaseline, 0, len(items))
	for _, item := range items {
		scopeType := strings.TrimSpace(item.ScopeType)
		if scopeType == "" {
			scopeType = "global"
		}
		baseline = append(baseline, LexicalShadowBaseline{
			MemoryID: item.ID, Revision: item.Revision, ScopeType: scopeType,
		})
	}
	digest := sha256.Sum256([]byte(query))
	startedAt := time.Now()
	shadowCtx, cancel := context.WithTimeout(ctx, hybridShadowHardCutoff)
	defer cancel()

	embedCtx, embedCancel := context.WithTimeout(shadowCtx, hybridShadowEmbedCutoff)
	queryEmbedding, embeddingState := s.hybridQueryEmbedding(embedCtx, query)
	embedCancel()
	prepared, err := repository.PrepareHybridShadow(shadowCtx, HybridShadowPrepareInput{
		ObservationID: observationID, ConversationID: conversationID,
		AssistantMessageID: assistantMessageID,
		QueryHash:          hex.EncodeToString(digest[:]), QueryText: query,
		Baseline: baseline, QueryEmbedding: queryEmbedding,
		QueryEmbeddingState: embeddingState,
	})
	if err != nil {
		if errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
			return items, hybridShadowFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return items, hybridShadowFailure("PREPARE_FAILED", "NONE"), nil
	}
	if prepared.Replayed || prepared.Summary.Status != "pending" {
		return items, sanitizeHybridShadowSummary(prepared.Summary), nil
	}

	rerankStatus := "skipped"
	fallbackCode := prepared.Summary.FallbackCode
	ordered := append([]HybridShadowCandidate(nil), prepared.Candidates...)
	reranked := []HybridShadowRankedItem{}
	if len(ordered) > 0 {
		var rerankErr error
		rerankCtx, rerankCancel := hybridProviderStageContext(
			shadowCtx,
			hybridShadowRecordReserve,
		)
		if s.hybridProvider == nil {
			rerankErr = ragproviders.ErrRerankUnavailable
		} else {
			ordered, reranked, rerankErr = rerankHybridCandidates(
				rerankCtx, s.hybridProvider, query, ordered,
			)
		}
		rerankCutoff := errors.Is(rerankCtx.Err(), context.DeadlineExceeded)
		rerankCancel()
		if rerankErr != nil || rerankCutoff ||
			errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
			rerankStatus = "fallback"
			if rerankCutoff || errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
				fallbackCode = "HARD_CUTOFF"
			} else if errors.Is(rerankErr, errHybridProviderTextRedacted) {
				fallbackCode = "SECRET_REDACTED"
			} else if fallbackCode == "NONE" {
				fallbackCode = "RERANK_FAILED"
			}
			ordered = append([]HybridShadowCandidate(nil), prepared.Candidates...)
			reranked = []HybridShadowRankedItem{}
		} else {
			rerankStatus = "applied"
		}
	}
	final, estimatedTokens := selectHybridShadowFinal(ordered)
	targetExceeded := estimatedTokens > HybridShadowTargetTokens
	durationMillis := boundedHybridDuration(time.Since(startedAt))
	summary, err := repository.RecordHybridShadow(shadowCtx, HybridShadowRecordInput{
		ObservationID: observationID, AssistantMessageID: assistantMessageID,
		RerankStatus: rerankStatus, FallbackCode: normalizeHybridFallback(fallbackCode),
		Reranked: reranked, Final: final, EstimatedTokens: estimatedTokens,
		TargetTokensExceeded: targetExceeded, DurationMillis: durationMillis,
	})
	if err != nil {
		if errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
			return items, hybridShadowFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return items, hybridShadowFailure("RECORD_FAILED", normalizeHybridFallback(fallbackCode)), nil
	}
	return items, sanitizeHybridShadowSummary(summary), nil
}

func hybridProviderStageContext(
	parent context.Context,
	reserve time.Duration,
) (context.Context, context.CancelFunc) {
	deadline, ok := parent.Deadline()
	if !ok || reserve <= 0 {
		return context.WithCancel(parent)
	}
	stageDeadline := deadline.Add(-reserve)
	if stageDeadline.Before(time.Now()) {
		stageDeadline = time.Now()
	}
	return context.WithDeadline(parent, stageDeadline)
}

func (s *Service) hybridQueryEmbedding(ctx context.Context, query string) ([]float32, string) {
	if s == nil || s.hybridProvider == nil {
		return nil, "unavailable"
	}
	query = RedactMemoryProviderText(query, true)
	if strings.TrimSpace(query) == "" {
		return nil, "redacted"
	}
	embedding, err := s.hybridProvider.EmbedQuery(ctx, query)
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, "cutoff"
		}
		return nil, "failed"
	}
	if embedding.ModelID != ragproviders.SiliconFlowEmbeddingModel ||
		embedding.Dimensions != ragproviders.SiliconFlowEmbeddingDimensions ||
		!validHybridVector(embedding.Vector) {
		return nil, "failed"
	}
	return append([]float32(nil), embedding.Vector...), "ready"
}

func rerankHybridCandidates(
	ctx context.Context,
	provider HybridShadowProvider,
	query string,
	candidates []HybridShadowCandidate,
) ([]HybridShadowCandidate, []HybridShadowRankedItem, error) {
	query = RedactMemoryProviderText(query, true)
	if strings.TrimSpace(query) == "" {
		return nil, nil, errHybridProviderTextRedacted
	}
	documents := make([]string, len(candidates))
	for index, candidate := range candidates {
		documents[index] = RedactMemoryProviderText(candidate.Content, true)
		if strings.TrimSpace(documents[index]) == "" {
			return nil, nil, errHybridProviderTextRedacted
		}
	}
	results, err := provider.Rerank(ctx, query, documents)
	if err != nil || len(results) != len(candidates) {
		return nil, nil, ragproviders.ErrRerankUnavailable
	}
	seen := make(map[int]struct{}, len(results))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(candidates) ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) {
			return nil, nil, ragproviders.ErrRerankInvalid
		}
		if _, duplicate := seen[result.Index]; duplicate {
			return nil, nil, ragproviders.ErrRerankInvalid
		}
		seen[result.Index] = struct{}{}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].RelevanceScore == results[j].RelevanceScore {
			return results[i].Index < results[j].Index
		}
		return results[i].RelevanceScore > results[j].RelevanceScore
	})
	ordered := make([]HybridShadowCandidate, 0, len(results))
	reranked := make([]HybridShadowRankedItem, 0, len(results))
	for _, result := range results {
		candidate := candidates[result.Index]
		ordered = append(ordered, candidate)
		reranked = append(reranked, hybridRankedItem(candidate))
	}
	return ordered, reranked, nil
}

func selectHybridShadowFinal(candidates []HybridShadowCandidate) ([]HybridShadowRankedItem, int) {
	selected := make([]HybridShadowRankedItem, 0, min(len(candidates), HybridShadowFinalLimit))
	tokens := 0
	for _, candidate := range candidates {
		if len(selected) == HybridShadowFinalLimit {
			break
		}
		cost := estimateHybridMemoryTokens(candidate.Content)
		if cost <= 0 || cost > HybridShadowMaximumTokens || tokens+cost > HybridShadowMaximumTokens {
			continue
		}
		selected = append(selected, hybridRankedItem(candidate))
		tokens += cost
	}
	return selected, tokens
}

func estimateHybridMemoryTokens(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	ascii, nonASCII := 0, 0
	for _, runeValue := range value {
		if runeValue <= 0x7f {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII*2 + hybridShadowTokenOverhead
}

// EstimatePromptMemoryTokens applies the exact conservative token estimator
// used by the native hybrid final selector to an already-authorized Memory
// list. Offline reader capture uses this to score the current v1 prompt surface
// without copying the estimator into the benchmark package.
func EstimatePromptMemoryTokens(memories []Memory) int {
	total := 0
	for _, memory := range memories {
		total += estimateHybridMemoryTokens(memory.Content)
	}
	return total
}

func hybridRankedItem(candidate HybridShadowCandidate) HybridShadowRankedItem {
	return HybridShadowRankedItem{
		MemoryID: candidate.MemoryID, Revision: candidate.Revision,
		ScopeType: candidate.ScopeType,
	}
}

func validHybridVector(vector []float32) bool {
	if len(vector) != ragproviders.SiliconFlowEmbeddingDimensions {
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

func hybridShadowFailure(resultCode string, fallbackCode string) HybridShadowSummary {
	return HybridShadowSummary{
		ProfileID: HybridShadowProfileID, Status: "failed",
		ResultCode: resultCode, FallbackCode: normalizeHybridFallback(fallbackCode),
	}
}

func sanitizeHybridShadowSummary(summary HybridShadowSummary) HybridShadowSummary {
	summary.ProfileID = HybridShadowProfileID
	switch summary.Status {
	case "pending", "completed", "failed":
	default:
		summary.Status = "failed"
	}
	if !lexicalShadowResultCodeRE.MatchString(summary.ResultCode) {
		summary.ResultCode = "HYBRID_FAILED"
		summary.Status = "failed"
	}
	summary.FallbackCode = normalizeHybridFallback(summary.FallbackCode)
	summary.BaselineCount = clampLexicalShadowCount(summary.BaselineCount, MaxSearchResults)
	summary.ExactCount = clampLexicalShadowCount(summary.ExactCount, 20)
	summary.BM25Count = clampLexicalShadowCount(summary.BM25Count, 30)
	summary.VectorCount = clampLexicalShadowCount(summary.VectorCount, 30)
	summary.RRFCount = clampLexicalShadowCount(summary.RRFCount, MaxHybridShadowResults)
	summary.RerankCount = clampLexicalShadowCount(summary.RerankCount, MaxHybridShadowResults)
	summary.FinalCount = clampLexicalShadowCount(summary.FinalCount, HybridShadowFinalLimit)
	summary.OverlapCount = clampLexicalShadowCount(summary.OverlapCount, MaxSearchResults)
	summary.EstimatedTokens = clampLexicalShadowCount(summary.EstimatedTokens, HybridShadowMaximumTokens)
	summary.DurationMillis = clampLexicalShadowCount(summary.DurationMillis, 120000)
	return summary
}

func normalizeHybridFallback(value string) string {
	value = strings.TrimSpace(value)
	if !lexicalShadowResultCodeRE.MatchString(value) {
		return "NONE"
	}
	return value
}

func boundedHybridDuration(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	millis := duration.Milliseconds()
	if millis > 120000 {
		return 120000
	}
	return int(millis)
}
