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
	L2SceneProfileID      = "memory_l2_scene_v1"
	L2SceneCandidateLimit = 20
	L2SceneFinalLimit     = 2
	L2SceneMaximumTokens  = 500
	L2SceneMaximumChars   = 4_000
)

// SearchRelevantL2Scenes runs the default-off Scene reader without changing
// canonical L1 results. Shadow mode records only bounded diagnostics. Active
// mode returns at most two current Scenes after SQL reauthorization.
func (s *Service) SearchRelevantL2Scenes(
	ctx context.Context,
	query string,
	conversationID string,
	assistantMessageID string,
	activeRequested bool,
) (L2SceneSearchResult, error) {
	repository, ok := s.repo.(L2SceneRepository)
	if !ok || repository == nil {
		return l2SceneFailure("PREPARE_UNAVAILABLE", "PROVIDER_UNAVAILABLE"), nil
	}
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(conversationID) || !uuidRE.MatchString(assistantMessageID) ||
		strings.TrimSpace(query) == "" {
		return l2SceneFailure("ARGUMENT_INVALID", "NONE"), nil
	}
	observationID, err := newUUID()
	if err != nil {
		return l2SceneFailure("OBSERVATION_ID_FAILED", "NONE"), nil
	}
	digest := sha256.Sum256([]byte(query))
	startedAt := time.Now()
	readerCtx, cancel := context.WithTimeout(ctx, hybridShadowHardCutoff)
	defer cancel()

	embedCtx, embedCancel := context.WithTimeout(readerCtx, hybridShadowEmbedCutoff)
	queryEmbedding, embeddingState := s.hybridQueryEmbedding(embedCtx, query)
	embedTimedOut := errors.Is(embedCtx.Err(), context.DeadlineExceeded)
	embedCancel()
	if embedTimedOut {
		queryEmbedding = nil
		embeddingState = "cutoff"
	}
	prepared, err := repository.PrepareL2SceneSearch(readerCtx, L2ScenePrepareInput{
		ObservationID: observationID, ConversationID: conversationID,
		AssistantMessageID: assistantMessageID,
		QueryHash:          hex.EncodeToString(digest[:]), QueryText: query,
		QueryEmbedding: queryEmbedding, QueryEmbeddingState: embeddingState,
		ActiveRequested: activeRequested,
	})
	if err != nil {
		if errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
			return l2SceneFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return l2SceneFailure("PREPARE_FAILED", "NONE"), nil
	}
	if prepared.Replayed || prepared.Summary.Status != "pending" {
		return L2SceneSearchResult{Summary: sanitizeL2SceneSummary(prepared.Summary)}, nil
	}

	rerankStatus := "skipped"
	fallbackCode := prepared.Summary.FallbackCode
	ordered := append([]L2SceneCandidate(nil), prepared.Candidates...)
	reranked := []L2SceneRankedItem{}
	if len(ordered) > 0 {
		var rerankErr error
		rerankCtx, rerankCancel := hybridProviderStageContext(
			readerCtx,
			hybridShadowRecordReserve,
		)
		if s.hybridProvider == nil {
			rerankErr = ragproviders.ErrRerankUnavailable
		} else {
			ordered, reranked, rerankErr = rerankL2SceneCandidates(
				rerankCtx,
				s.hybridProvider,
				query,
				ordered,
			)
		}
		rerankCutoff := errors.Is(rerankCtx.Err(), context.DeadlineExceeded)
		rerankCancel()
		if rerankErr != nil || rerankCutoff ||
			errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
			rerankStatus = "fallback"
			if rerankCutoff || errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
				fallbackCode = "HARD_CUTOFF"
			} else if errors.Is(rerankErr, errHybridProviderTextRedacted) {
				fallbackCode = "SECRET_REDACTED"
			} else if fallbackCode == "NONE" {
				fallbackCode = "RERANK_FAILED"
			}
			ordered = append([]L2SceneCandidate(nil), prepared.Candidates...)
			reranked = []L2SceneRankedItem{}
		} else {
			rerankStatus = "applied"
		}
	}
	final, estimatedTokens := selectL2SceneFinal(ordered)
	result, err := repository.RecordL2SceneSearch(readerCtx, L2SceneRecordInput{
		ObservationID: observationID, AssistantMessageID: assistantMessageID,
		RerankStatus: rerankStatus, FallbackCode: normalizeHybridFallback(fallbackCode),
		Reranked: reranked, Final: final, EstimatedTokens: estimatedTokens,
		DurationMillis: boundedHybridDuration(time.Since(startedAt)),
	})
	if err != nil {
		if errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
			return l2SceneFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return l2SceneFailure("RECORD_FAILED", normalizeHybridFallback(fallbackCode)), nil
	}
	result.Summary.ExactCount = prepared.Summary.ExactCount
	result.Summary.BM25Count = prepared.Summary.BM25Count
	result.Summary.VectorCount = prepared.Summary.VectorCount
	result.Summary.RRFCount = prepared.Summary.RRFCount
	result.Summary = sanitizeL2SceneSummary(result.Summary)
	if result.Summary.Mode != "active" ||
		result.Summary.InjectedCount != len(result.Scenes) {
		result.Scenes = nil
	}
	return result, nil
}

func rerankL2SceneCandidates(
	ctx context.Context,
	provider HybridShadowProvider,
	query string,
	candidates []L2SceneCandidate,
) ([]L2SceneCandidate, []L2SceneRankedItem, error) {
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
	ordered := make([]L2SceneCandidate, 0, len(results))
	reranked := make([]L2SceneRankedItem, 0, len(results))
	for _, result := range results {
		candidate := candidates[result.Index]
		ordered = append(ordered, candidate)
		reranked = append(reranked, l2SceneRankedItem(candidate))
	}
	return ordered, reranked, nil
}

func selectL2SceneFinal(candidates []L2SceneCandidate) ([]L2SceneRankedItem, int) {
	selected := make([]L2SceneRankedItem, 0, min(len(candidates), L2SceneFinalLimit))
	tokens := 0
	for _, candidate := range candidates {
		if len(selected) == L2SceneFinalLimit {
			break
		}
		cost := estimateHybridMemoryTokens(candidate.Content)
		if cost <= 0 || cost > L2SceneMaximumTokens ||
			tokens+cost > L2SceneMaximumTokens {
			continue
		}
		selected = append(selected, l2SceneRankedItem(candidate))
		tokens += cost
	}
	return selected, tokens
}

func l2SceneRankedItem(candidate L2SceneCandidate) L2SceneRankedItem {
	return L2SceneRankedItem{SceneID: candidate.SceneID, Revision: candidate.Revision}
}

func l2SceneFailure(resultCode string, fallbackCode string) L2SceneSearchResult {
	return L2SceneSearchResult{Summary: L2SceneSearchSummary{
		ProfileID: L2SceneProfileID, Status: "failed", ResultCode: resultCode,
		FallbackCode: normalizeHybridFallback(fallbackCode),
	}}
}

func sanitizeL2SceneSummary(summary L2SceneSearchSummary) L2SceneSearchSummary {
	summary.ProfileID = L2SceneProfileID
	if summary.Mode != "shadow" && summary.Mode != "active" {
		summary.Mode = "shadow"
	}
	switch summary.Status {
	case "pending", "completed", "failed":
	default:
		summary.Status = "failed"
	}
	if !lexicalShadowResultCodeRE.MatchString(summary.ResultCode) {
		summary.ResultCode = "L2_SCENE_FAILED"
		summary.Status = "failed"
	}
	summary.FallbackCode = normalizeHybridFallback(summary.FallbackCode)
	summary.ExactCount = clampLexicalShadowCount(summary.ExactCount, 20)
	summary.BM25Count = clampLexicalShadowCount(summary.BM25Count, 30)
	summary.VectorCount = clampLexicalShadowCount(summary.VectorCount, 30)
	summary.RRFCount = clampLexicalShadowCount(summary.RRFCount, L2SceneCandidateLimit)
	summary.RerankCount = clampLexicalShadowCount(summary.RerankCount, L2SceneCandidateLimit)
	summary.FinalCount = clampLexicalShadowCount(summary.FinalCount, L2SceneFinalLimit)
	summary.InjectedCount = clampLexicalShadowCount(summary.InjectedCount, L2SceneFinalLimit)
	summary.EstimatedTokens = clampLexicalShadowCount(
		summary.EstimatedTokens,
		L2SceneMaximumTokens,
	)
	summary.DurationMillis = clampLexicalShadowCount(summary.DurationMillis, 120000)
	return summary
}
