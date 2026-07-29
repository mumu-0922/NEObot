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
	L3PersonaProfileID      = "memory_l3_persona_v1"
	L3PersonaCandidateLimit = 5
	L3PersonaFinalLimit     = 1
	L3PersonaMaximumTokens  = 300
	L3PersonaMaximumChars   = 4_000
)

// SearchRelevantL3Persona runs the default-off Persona reader without changing
// canonical L1 results. Shadow mode records only bounded diagnostics. Active
// mode returns at most one current Persona after SQL reauthorization.
func (s *Service) SearchRelevantL3Persona(
	ctx context.Context,
	query string,
	conversationID string,
	assistantMessageID string,
	activeRequested bool,
) (L3PersonaSearchResult, error) {
	repository, ok := s.repo.(L3PersonaRepository)
	if !ok || repository == nil {
		return l3PersonaFailure("PREPARE_UNAVAILABLE", "PROVIDER_UNAVAILABLE"), nil
	}
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(conversationID) || !uuidRE.MatchString(assistantMessageID) ||
		strings.TrimSpace(query) == "" {
		return l3PersonaFailure("ARGUMENT_INVALID", "NONE"), nil
	}
	observationID, err := newUUID()
	if err != nil {
		return l3PersonaFailure("OBSERVATION_ID_FAILED", "NONE"), nil
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
	prepared, err := repository.PrepareL3PersonaSearch(readerCtx, L3PersonaPrepareInput{
		ObservationID: observationID, ConversationID: conversationID,
		AssistantMessageID: assistantMessageID,
		QueryHash:          hex.EncodeToString(digest[:]), QueryText: query,
		QueryEmbedding: queryEmbedding, QueryEmbeddingState: embeddingState,
		ActiveRequested: activeRequested,
	})
	if err != nil {
		if errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
			return l3PersonaFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return l3PersonaFailure("PREPARE_FAILED", "NONE"), nil
	}
	if prepared.Replayed || prepared.Summary.Status != "pending" {
		return L3PersonaSearchResult{Summary: sanitizeL3PersonaSummary(prepared.Summary)}, nil
	}

	rerankStatus := "skipped"
	fallbackCode := prepared.Summary.FallbackCode
	ordered := append([]L3PersonaCandidate(nil), prepared.Candidates...)
	reranked := []L3PersonaRankedItem{}
	if len(ordered) > 0 {
		var rerankErr error
		rerankCtx, rerankCancel := hybridProviderStageContext(
			readerCtx,
			hybridShadowRecordReserve,
		)
		if s.hybridProvider == nil {
			rerankErr = ragproviders.ErrRerankUnavailable
		} else {
			ordered, reranked, rerankErr = rerankL3PersonaCandidates(
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
			ordered = append([]L3PersonaCandidate(nil), prepared.Candidates...)
			reranked = []L3PersonaRankedItem{}
		} else {
			rerankStatus = "applied"
		}
	}
	final, estimatedTokens := selectL3PersonaFinal(ordered)
	result, err := repository.RecordL3PersonaSearch(readerCtx, L3PersonaRecordInput{
		ObservationID: observationID, AssistantMessageID: assistantMessageID,
		RerankStatus: rerankStatus, FallbackCode: normalizeHybridFallback(fallbackCode),
		Reranked: reranked, Final: final, EstimatedTokens: estimatedTokens,
		DurationMillis: boundedHybridDuration(time.Since(startedAt)),
	})
	if err != nil {
		if errors.Is(readerCtx.Err(), context.DeadlineExceeded) {
			return l3PersonaFailure("HARD_CUTOFF", "HARD_CUTOFF"), nil
		}
		return l3PersonaFailure("RECORD_FAILED", normalizeHybridFallback(fallbackCode)), nil
	}
	result.Summary.ExactCount = prepared.Summary.ExactCount
	result.Summary.BM25Count = prepared.Summary.BM25Count
	result.Summary.VectorCount = prepared.Summary.VectorCount
	result.Summary.RRFCount = prepared.Summary.RRFCount
	result.Summary = sanitizeL3PersonaSummary(result.Summary)
	if result.Summary.Mode != "active" ||
		result.Summary.InjectedCount != len(result.Personas) {
		result.Personas = nil
	}
	return result, nil
}

func rerankL3PersonaCandidates(
	ctx context.Context,
	provider HybridShadowProvider,
	query string,
	candidates []L3PersonaCandidate,
) ([]L3PersonaCandidate, []L3PersonaRankedItem, error) {
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
	ordered := make([]L3PersonaCandidate, 0, len(results))
	reranked := make([]L3PersonaRankedItem, 0, len(results))
	for _, result := range results {
		candidate := candidates[result.Index]
		ordered = append(ordered, candidate)
		reranked = append(reranked, l3PersonaRankedItem(candidate))
	}
	return ordered, reranked, nil
}

func selectL3PersonaFinal(candidates []L3PersonaCandidate) ([]L3PersonaRankedItem, int) {
	selected := make([]L3PersonaRankedItem, 0, min(len(candidates), L3PersonaFinalLimit))
	tokens := 0
	for _, candidate := range candidates {
		if len(selected) == L3PersonaFinalLimit {
			break
		}
		cost := estimateHybridMemoryTokens(candidate.Content)
		if cost <= 0 || cost > L3PersonaMaximumTokens ||
			tokens+cost > L3PersonaMaximumTokens {
			continue
		}
		selected = append(selected, l3PersonaRankedItem(candidate))
		tokens += cost
	}
	return selected, tokens
}

func l3PersonaRankedItem(candidate L3PersonaCandidate) L3PersonaRankedItem {
	return L3PersonaRankedItem{PersonaID: candidate.PersonaID, Revision: candidate.Revision}
}

func l3PersonaFailure(resultCode string, fallbackCode string) L3PersonaSearchResult {
	return L3PersonaSearchResult{Summary: L3PersonaSearchSummary{
		ProfileID: L3PersonaProfileID, Status: "failed", ResultCode: resultCode,
		FallbackCode: normalizeHybridFallback(fallbackCode),
	}}
}

func sanitizeL3PersonaSummary(summary L3PersonaSearchSummary) L3PersonaSearchSummary {
	summary.ProfileID = L3PersonaProfileID
	if summary.Mode != "shadow" && summary.Mode != "active" {
		summary.Mode = "shadow"
	}
	switch summary.Status {
	case "pending", "completed", "failed":
	default:
		summary.Status = "failed"
	}
	if !lexicalShadowResultCodeRE.MatchString(summary.ResultCode) {
		summary.ResultCode = "L3_PERSONA_FAILED"
		summary.Status = "failed"
	}
	summary.FallbackCode = normalizeHybridFallback(summary.FallbackCode)
	summary.ExactCount = clampLexicalShadowCount(summary.ExactCount, L3PersonaCandidateLimit)
	summary.BM25Count = clampLexicalShadowCount(summary.BM25Count, L3PersonaCandidateLimit)
	summary.VectorCount = clampLexicalShadowCount(summary.VectorCount, L3PersonaCandidateLimit)
	summary.RRFCount = clampLexicalShadowCount(summary.RRFCount, L3PersonaCandidateLimit)
	summary.RerankCount = clampLexicalShadowCount(summary.RerankCount, L3PersonaCandidateLimit)
	summary.FinalCount = clampLexicalShadowCount(summary.FinalCount, L3PersonaFinalLimit)
	summary.InjectedCount = clampLexicalShadowCount(summary.InjectedCount, L3PersonaFinalLimit)
	summary.EstimatedTokens = clampLexicalShadowCount(
		summary.EstimatedTokens,
		L3PersonaMaximumTokens,
	)
	summary.DurationMillis = clampLexicalShadowCount(summary.DurationMillis, 120000)
	return summary
}
