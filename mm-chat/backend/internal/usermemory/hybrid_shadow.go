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
	hybridShadowHardCutoff                = 2 * time.Second
	hybridShadowEmbedCutoff               = 750 * time.Millisecond
	hybridShadowIntentCutoff              = 500 * time.Millisecond
	hybridShadowRecordReserve             = 150 * time.Millisecond
	HybridShadowTargetTokens              = 600
	HybridShadowMaximumTokens             = 900
	HybridShadowFinalLimit                = 5
	hybridShadowTargetTokens              = HybridShadowTargetTokens
	hybridShadowMaximumTokens             = HybridShadowMaximumTokens
	hybridShadowFinalLimit                = HybridShadowFinalLimit
	hybridShadowTokenOverhead             = 24
	hybridPolicyModeCalibration           = "calibration"
	hybridPolicyModeIntentCalibration     = "intent_calibration"
	hybridPolicyModeCloudJudgeCalibration = "cloud_judge_calibration"
	hybridPolicyModeFixedMemoryJudge      = "fixed_cloud_candidate_judge_development"
	hybridPolicyModeAccuracyFirstJudge    = "fixed_cloud_candidate_judge_accuracy_development"
	hybridPolicyModeProductionJudge       = "fixed_cloud_candidate_judge_production"
	hybridPolicyModeMemoryToolRoute       = "main_model_tool_route_calibration"
	hybridPolicyModeMemoryFirstToolRound  = "main_model_first_tool_round_calibration"
	hybridPolicyModeFrozen                = "frozen"
	// These values are changed only after a successful Development calibration
	// artifact has been reviewed. Validation refuses to run while ready=false.
	hybridFrozenPolicyReady          = false
	hybridFrozenProviderSimilarityBP = 0
	hybridFrozenFinalRelevanceBP     = 0
	hybridFrozenMemoryIntentMarginBP = 0
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
	policy, ok := validHybridShadowRelevancePolicy(s.hybridPolicy)
	if !ok {
		return items, hybridShadowFailure("POLICY_UNAVAILABLE", "POLICY_UNAVAILABLE"), nil
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
	execution := s.executeHybridShadow(
		ctx,
		query,
		conversationID,
		assistantMessageID,
		baseline,
		policy,
	)
	return items, execution.Summary, nil
}

type hybridShadowExecution struct {
	ObservationID string
	Final         []HybridShadowRankedItem
	Summary       HybridShadowSummary
}

// SearchRelevantAfterMemoryToolCall executes the hybrid reader only after the
// product's first Tool round has authorized relevance. It never calls the v1
// reader and never falls back to v1 or unscored RRF candidates.
func (s *Service) SearchRelevantAfterMemoryToolCall(
	ctx context.Context,
	input HybridMemoryToolSearchInput,
) HybridMemoryToolSearchResult {
	if s == nil || input.ContractVersion != HybridMemoryToolContractVersion ||
		input.ContractSHA256 != HybridMemoryToolContractSHA256 {
		return hybridMemoryToolFailure("contract_drift")
	}
	query := input.Query
	conversationID := strings.TrimSpace(input.ConversationID)
	assistantMessageID := strings.TrimSpace(input.AssistantMessageID)
	if strings.TrimSpace(query) == "" || !uuidRE.MatchString(conversationID) ||
		!uuidRE.MatchString(assistantMessageID) {
		return hybridMemoryToolFailure("invalid_authority")
	}
	if s.hybridProvider == nil {
		return hybridMemoryToolFailure("dependency_unavailable")
	}
	policy, ok := validHybridShadowRelevancePolicy(s.hybridToolPolicy)
	if !ok || policy.Mode != hybridPolicyModeProductionJudge {
		return hybridMemoryToolFailure("policy_unavailable")
	}
	hydrator, ok := s.repo.(HybridFinalRepository)
	if !ok || hydrator == nil {
		return hybridMemoryToolFailure("dependency_unavailable")
	}
	execution := s.executeHybridShadow(
		ctx,
		query,
		conversationID,
		assistantMessageID,
		[]LexicalShadowBaseline{},
		policy,
	)
	result := HybridMemoryToolSearchResult{
		Memories: []Memory{}, Summary: execution.Summary,
	}
	if execution.Summary.Status != "completed" ||
		execution.Summary.ResultCode != "OK" ||
		len(execution.Final) == 0 ||
		execution.Summary.FinalCount != len(execution.Final) {
		if execution.Summary.Status == "completed" &&
			execution.Summary.ResultCode == "NO_CANDIDATES" {
			result.Memories = []Memory{}
			result.FailureCategory = s.memoryEmptyResultFailure(ctx)
			return result
		}
		result.FailureCategory = "retrieval_unavailable"
		return result
	}
	memories, err := hydrator.HydrateHybridFinal(ctx, HybridFinalHydrationInput{
		ObservationID:      execution.ObservationID,
		AssistantMessageID: assistantMessageID,
	})
	if err != nil || len(memories) != len(execution.Final) {
		result.FailureCategory = "authority_stale"
		return result
	}
	for index := range memories {
		expected := execution.Final[index]
		memory := &memories[index]
		scopeType := strings.TrimSpace(memory.ScopeType)
		if scopeType == "" {
			scopeType = "global"
		}
		if memory.ID != expected.MemoryID || memory.Revision != expected.Revision ||
			scopeType != expected.ScopeType || strings.TrimSpace(memory.Type) == "" {
			result.FailureCategory = "authority_stale"
			return result
		}
		memory.ScopeType = scopeType
		memory.Content = RedactMemoryProviderText(memory.Content, true)
		if strings.TrimSpace(memory.Content) == "" {
			result.FailureCategory = "secret_redacted"
			return result
		}
	}
	result.Memories = memories
	return result
}

func (s *Service) memoryEmptyResultFailure(ctx context.Context) string {
	health, err := s.GetMemoryHealth(ctx)
	if err != nil {
		return "memory_status_unavailable"
	}
	switch health.Status {
	case "ready":
		return ""
	case "indexing":
		return "memory_indexing"
	case "degraded":
		return "memory_service_unavailable"
	case "disabled":
		return "memory_disabled"
	default:
		return "memory_status_unavailable"
	}
}

func hybridMemoryToolFailure(category string) HybridMemoryToolSearchResult {
	return HybridMemoryToolSearchResult{
		Memories:        []Memory{},
		Summary:         hybridShadowFailure("HYBRID_FAILED", "NONE"),
		FailureCategory: category,
	}
}

func (s *Service) executeHybridShadow(
	ctx context.Context,
	query string,
	conversationID string,
	assistantMessageID string,
	baseline []LexicalShadowBaseline,
	policy HybridShadowRelevancePolicy,
) hybridShadowExecution {
	failure := hybridShadowFailure("PREPARE_UNAVAILABLE", "PROVIDER_UNAVAILABLE")
	repository, ok := s.repo.(HybridShadowRepository)
	if !ok || repository == nil {
		return hybridShadowExecution{Summary: failure}
	}
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(conversationID) || !uuidRE.MatchString(assistantMessageID) || query == "" {
		return hybridShadowExecution{Summary: hybridShadowFailure("ARGUMENT_INVALID", "NONE")}
	}
	observationID, err := newUUID()
	if err != nil {
		return hybridShadowExecution{Summary: hybridShadowFailure("OBSERVATION_ID_FAILED", "NONE")}
	}
	digest := sha256.Sum256([]byte(query))
	startedAt := time.Now()
	hardCutoffMilliseconds, ok := HybridShadowHardCutoffMilliseconds(policy)
	if !ok {
		return hybridShadowExecution{Summary: hybridShadowFailure("POLICY_UNAVAILABLE", "POLICY_UNAVAILABLE")}
	}
	var shadowCtx context.Context
	var cancel context.CancelFunc
	if hardCutoffMilliseconds == 0 {
		shadowCtx, cancel = context.WithCancel(ctx)
	} else {
		shadowCtx, cancel = context.WithTimeout(
			ctx,
			time.Duration(hardCutoffMilliseconds)*time.Millisecond,
		)
	}
	defer cancel()
	memoryToolRoute := startHybridMemoryToolRoute(
		shadowCtx,
		s.hybridRouter,
		policy,
		query,
	)
	// A route starts before retrieval so both Provider stages can overlap. It
	// must still be observed on every early return; otherwise capture can close
	// the current case while the route is still publishing its bounded result.
	if memoryToolRoute != nil {
		defer func() {
			_, _ = awaitHybridMemoryToolRoute(shadowCtx, memoryToolRoute)
		}()
	}

	embedCtx := shadowCtx
	embedCancel := func() {}
	if !hybridPolicyRunsAccuracyFirst(policy.Mode) {
		embedCtx, embedCancel = context.WithTimeout(shadowCtx, hybridShadowEmbedCutoff)
	}
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
			return hybridShadowExecution{Summary: hybridShadowFailure("HARD_CUTOFF", "HARD_CUTOFF")}
		}
		return hybridShadowExecution{Summary: hybridShadowFailure("PREPARE_FAILED", "NONE")}
	}
	if prepared.Replayed || prepared.Summary.Status != "pending" {
		return hybridShadowExecution{
			ObservationID: prepared.ObservationID,
			Summary:       sanitizeHybridShadowSummary(prepared.Summary),
		}
	}
	if uuidRE.MatchString(prepared.ObservationID) {
		observationID = prepared.ObservationID
	}

	rerankStatus := "skipped"
	fallbackCode := prepared.Summary.FallbackCode
	reranked := []HybridShadowRankedItem{}
	scored := []hybridShadowScoredCandidate{}
	if len(prepared.Candidates) > 0 {
		var rerankErr error
		intentAdmitted := true
		if policy.MemoryIntentRequired {
			intentCtx, intentCancel := context.WithTimeout(shadowCtx, hybridShadowIntentCutoff)
			intent, intentErr := classifyHybridMemoryIntent(
				intentCtx,
				s.hybridProvider,
				query,
			)
			intentCutoff := errors.Is(intentCtx.Err(), context.DeadlineExceeded)
			intentCancel()
			if intentErr != nil || intentCutoff {
				rerankErr = ragproviders.ErrMemoryIntentUnavailable
				fallbackCode = "MEMORY_INTENT_FAILED"
				intentAdmitted = false
			} else if intent.Margin < policy.MinimumMemoryIntentMargin {
				fallbackCode = "MEMORY_INTENT_ABSTAINED"
				intentAdmitted = false
			}
		}
		admissionRepository, admissionOK := s.repo.(HybridShadowAdmissionRepository)
		if !intentAdmitted {
			// Query-only intent classification sends no Memory document. Every
			// failure or low-margin result remains a fail-closed no-memory path.
		} else if !admissionOK || admissionRepository == nil || len(queryEmbedding) == 0 {
			rerankErr = ragproviders.ErrRerankUnavailable
			fallbackCode = "RELEVANCE_ADMISSION_UNAVAILABLE"
		} else {
			admission, admissionErr := admissionRepository.AuthorizeHybridRerank(
				shadowCtx,
				HybridShadowAdmissionInput{
					ObservationID: observationID, AssistantMessageID: assistantMessageID,
					QueryHash: hex.EncodeToString(digest[:]), QueryEmbedding: queryEmbedding,
				},
			)
			if admissionErr != nil || !validHybridShadowAdmission(admission, len(prepared.Candidates)) {
				rerankErr = ragproviders.ErrRerankUnavailable
				fallbackCode = "RELEVANCE_ADMISSION_FAILED"
			} else if admission.MaximumVectorSimilarity < policy.MinimumProviderSimilarity {
				fallbackCode = "RELEVANCE_ABSTAINED"
			} else {
				rerankCtx := shadowCtx
				rerankCancel := func() {}
				if !hybridPolicyRunsAccuracyFirst(policy.Mode) {
					rerankCtx, rerankCancel = hybridProviderStageContext(
						shadowCtx,
						hybridShadowRecordReserve,
					)
				}
				if s.hybridProvider == nil {
					rerankErr = ragproviders.ErrRerankUnavailable
				} else {
					var stageFallback string
					scored, reranked, stageFallback, rerankErr = executeHybridCandidateStages(
						rerankCtx,
						s.hybridProvider,
						s.hybridJudge,
						memoryToolRoute,
						policy,
						query,
						prepared.Candidates,
					)
					if stageFallback != "" {
						fallbackCode = stageFallback
					}
				}
				rerankCutoff := errors.Is(rerankCtx.Err(), context.DeadlineExceeded)
				rerankCancel()
				if rerankCutoff || errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
					rerankErr = context.DeadlineExceeded
				}
			}
		}
		if rerankErr != nil || errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
			rerankStatus = "fallback"
			if errors.Is(rerankErr, context.DeadlineExceeded) ||
				errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
				fallbackCode = "HARD_CUTOFF"
			} else if errors.Is(rerankErr, errHybridProviderTextRedacted) {
				fallbackCode = "SECRET_REDACTED"
			} else if fallbackCode == "NONE" {
				fallbackCode = "RERANK_FAILED"
			}
			scored = []hybridShadowScoredCandidate{}
			reranked = []HybridShadowRankedItem{}
		} else if len(reranked) > 0 {
			rerankStatus = "applied"
		}
	} else if policy.MemoryToolRouteRequired {
		routed, routeErr := awaitHybridMemoryToolRoute(shadowCtx, memoryToolRoute)
		switch {
		case routeErr != nil:
			fallbackCode = "MEMORY_TOOL_ROUTE_FAILED"
		case routed:
			fallbackCode = "MEMORY_TOOL_ROUTE_EMPTY"
		default:
			fallbackCode = "MEMORY_TOOL_ROUTE_ABSTAINED"
		}
	}
	if policy.MemoryToolRouteRequired {
		// Candidate-less and pre-rerank fail-closed paths still consume the route
		// lifecycle. The cached stage result makes this an immediate read when
		// executeHybridCandidateStages already observed it.
		_, _ = awaitHybridMemoryToolRoute(shadowCtx, memoryToolRoute)
		if errors.Is(shadowCtx.Err(), context.DeadlineExceeded) {
			rerankStatus = "fallback"
			fallbackCode = "HARD_CUTOFF"
			scored = []hybridShadowScoredCandidate{}
			reranked = []HybridShadowRankedItem{}
		}
	}
	final, estimatedTokens := selectHybridShadowFinal(
		scored,
		policy.MinimumFinalRelevanceScore,
	)
	if rerankStatus == "applied" && len(reranked) > 0 && len(final) == 0 &&
		fallbackCode == "NONE" {
		fallbackCode = "RELEVANCE_FINAL_ABSTAINED"
	}
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
			return hybridShadowExecution{Summary: hybridShadowFailure("HARD_CUTOFF", "HARD_CUTOFF")}
		}
		return hybridShadowExecution{
			Summary: hybridShadowFailure("RECORD_FAILED", normalizeHybridFallback(fallbackCode)),
		}
	}
	return hybridShadowExecution{
		ObservationID: observationID,
		Final:         append([]HybridShadowRankedItem(nil), final...),
		Summary:       sanitizeHybridShadowSummary(summary),
	}
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
) ([]hybridShadowScoredCandidate, []HybridShadowRankedItem, error) {
	redactedQuery, documents, err := redactHybridProviderPayload(query, candidates)
	if err != nil {
		return nil, nil, err
	}
	return rerankHybridCandidatesWithPayload(
		ctx,
		provider,
		redactedQuery,
		documents,
		candidates,
	)
}

func rerankHybridCandidatesWithPayload(
	ctx context.Context,
	provider HybridShadowProvider,
	query string,
	documents []string,
	candidates []HybridShadowCandidate,
) ([]hybridShadowScoredCandidate, []HybridShadowRankedItem, error) {
	results, err := provider.Rerank(ctx, query, documents)
	if err != nil || ctx.Err() != nil || len(results) != len(candidates) {
		return nil, nil, ragproviders.ErrRerankUnavailable
	}
	seen := make(map[int]struct{}, len(results))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(candidates) ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) ||
			result.RelevanceScore < 0 || result.RelevanceScore > 1 {
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
	ordered := make([]hybridShadowScoredCandidate, 0, len(results))
	reranked := make([]HybridShadowRankedItem, 0, len(results))
	for _, result := range results {
		candidate := candidates[result.Index]
		ordered = append(ordered, hybridShadowScoredCandidate{
			Candidate:        candidate,
			CandidateOrdinal: result.Index,
			RelevanceScore:   result.RelevanceScore,
		})
		reranked = append(reranked, hybridRankedItem(candidate))
	}
	return ordered, reranked, nil
}

type hybridShadowScoredCandidate struct {
	Candidate        HybridShadowCandidate
	CandidateOrdinal int
	RelevanceScore   float64
}

type hybridRerankStageResult struct {
	scored   []hybridShadowScoredCandidate
	reranked []HybridShadowRankedItem
	err      error
}

type hybridJudgeStageResult struct {
	selected []int
	err      error
}

type hybridMemoryToolRouteStageResult struct {
	useMemory bool
	err       error
}

type hybridMemoryToolRouteStage struct {
	done   chan struct{}
	result hybridMemoryToolRouteStageResult
}

func executeHybridCandidateStages(
	ctx context.Context,
	provider HybridShadowProvider,
	judge HybridCandidateJudge,
	memoryToolRoute *hybridMemoryToolRouteStage,
	policy HybridShadowRelevancePolicy,
	query string,
	candidates []HybridShadowCandidate,
) ([]hybridShadowScoredCandidate, []HybridShadowRankedItem, string, error) {
	redactedQuery, documents, err := redactHybridProviderPayload(query, candidates)
	if err != nil {
		return nil, nil, "", err
	}
	if !policy.CloudCandidateJudgeRequired && !policy.MemoryToolRouteRequired {
		scored, reranked, rerankErr := rerankHybridCandidatesWithPayload(
			ctx,
			provider,
			redactedQuery,
			documents,
			candidates,
		)
		return scored, reranked, "", rerankErr
	}
	if policy.MemoryToolRouteRequired {
		rerankResult := make(chan hybridRerankStageResult, 1)
		go func() {
			scored, reranked, stageErr := rerankHybridCandidatesWithPayload(
				ctx,
				provider,
				redactedQuery,
				documents,
				candidates,
			)
			rerankResult <- hybridRerankStageResult{
				scored: scored, reranked: reranked, err: stageErr,
			}
		}()
		var reranked hybridRerankStageResult
		var routed hybridMemoryToolRouteStageResult
		routeDone := memoryToolRoute.done
		for completed := 0; completed < 2; completed++ {
			select {
			case reranked = <-rerankResult:
			case <-routeDone:
				routed = memoryToolRoute.result
				routeDone = nil
			case <-ctx.Done():
				return nil, nil, "MEMORY_TOOL_ROUTE_FAILED", context.DeadlineExceeded
			}
		}
		if ctx.Err() != nil {
			return nil, nil, "MEMORY_TOOL_ROUTE_FAILED", context.DeadlineExceeded
		}
		if reranked.err != nil {
			return nil, nil, "RERANK_FAILED", reranked.err
		}
		if routed.err != nil {
			return nil, nil, "MEMORY_TOOL_ROUTE_FAILED", routed.err
		}
		if !routed.useMemory {
			return []hybridShadowScoredCandidate{}, reranked.reranked,
				"MEMORY_TOOL_ROUTE_ABSTAINED", nil
		}
		return reranked.scored, reranked.reranked, "", nil
	}
	if judge == nil {
		return nil, nil, "CANDIDATE_JUDGE_FAILED", errors.New("hybrid candidate judge is unavailable")
	}
	if hybridPolicyRunsAccuracyFirst(policy.Mode) {
		scored, reranked, rerankErr := rerankHybridCandidatesWithPayload(
			ctx,
			provider,
			redactedQuery,
			documents,
			candidates,
		)
		if rerankErr != nil {
			return nil, nil, "RERANK_FAILED", rerankErr
		}
		selectedOrdinals, judgeErr := judgeHybridCandidates(
			ctx,
			judge,
			policy.CloudCandidateJudgeModelID,
			redactedQuery,
			documents,
		)
		if judgeErr != nil {
			return nil, nil, "CANDIDATE_JUDGE_FAILED", judgeErr
		}
		selected := intersectHybridJudgeSelection(scored, selectedOrdinals)
		if len(selected) == 0 {
			return selected, reranked, "CANDIDATE_JUDGE_ABSTAINED", nil
		}
		return selected, reranked, "", nil
	}

	rerankResult := make(chan hybridRerankStageResult, 1)
	judgeResult := make(chan hybridJudgeStageResult, 1)
	go func() {
		scored, reranked, stageErr := rerankHybridCandidatesWithPayload(
			ctx,
			provider,
			redactedQuery,
			documents,
			candidates,
		)
		rerankResult <- hybridRerankStageResult{
			scored: scored, reranked: reranked, err: stageErr,
		}
	}()
	go func() {
		selected, stageErr := judgeHybridCandidates(
			ctx,
			judge,
			policy.CloudCandidateJudgeModelID,
			redactedQuery,
			documents,
		)
		judgeResult <- hybridJudgeStageResult{selected: selected, err: stageErr}
	}()

	var reranked hybridRerankStageResult
	var judged hybridJudgeStageResult
	for completed := 0; completed < 2; completed++ {
		select {
		case reranked = <-rerankResult:
		case judged = <-judgeResult:
		case <-ctx.Done():
			return nil, nil, "CANDIDATE_JUDGE_FAILED", context.DeadlineExceeded
		}
	}
	if ctx.Err() != nil {
		return nil, nil, "CANDIDATE_JUDGE_FAILED", context.DeadlineExceeded
	}
	if reranked.err != nil {
		return nil, nil, "RERANK_FAILED", reranked.err
	}
	if judged.err != nil {
		return nil, nil, "CANDIDATE_JUDGE_FAILED", judged.err
	}
	selected := intersectHybridJudgeSelection(reranked.scored, judged.selected)
	if len(selected) == 0 {
		return selected, reranked.reranked, "CANDIDATE_JUDGE_ABSTAINED", nil
	}
	return selected, reranked.reranked, "", nil
}

func startHybridMemoryToolRoute(
	ctx context.Context,
	router HybridMemoryToolRouter,
	policy HybridShadowRelevancePolicy,
	query string,
) *hybridMemoryToolRouteStage {
	if !policy.MemoryToolRouteRequired {
		return nil
	}
	stage := &hybridMemoryToolRouteStage{done: make(chan struct{})}
	go func() {
		useMemory, err := routeHybridMemory(
			ctx,
			router,
			policy.MemoryToolRouteModelID,
			query,
		)
		stage.result = hybridMemoryToolRouteStageResult{useMemory: useMemory, err: err}
		close(stage.done)
	}()
	return stage
}

func awaitHybridMemoryToolRoute(
	ctx context.Context,
	stage *hybridMemoryToolRouteStage,
) (bool, error) {
	if stage == nil {
		return false, errors.New("hybrid Memory Tool route is unavailable")
	}
	select {
	case <-stage.done:
		routed := stage.result
		return routed.useMemory, routed.err
	case <-ctx.Done():
		return false, context.DeadlineExceeded
	}
}

func routeHybridMemory(
	ctx context.Context,
	router HybridMemoryToolRouter,
	expectedModelID string,
	query string,
) (bool, error) {
	query = RedactMemoryProviderText(query, true)
	if router == nil || strings.TrimSpace(query) == "" {
		return false, errors.New("hybrid Memory Tool route is unavailable")
	}
	result, err := router.RouteHybridMemory(ctx, HybridMemoryToolRouteInput{Query: query})
	if err != nil || ctx.Err() != nil {
		return false, errors.New("hybrid Memory Tool route request failed")
	}
	if result.ModelID != expectedModelID ||
		result.ContractVersion != HybridMemoryToolContractVersion ||
		result.ContractSHA256 != HybridMemoryToolContractSHA256 {
		return false, errors.New("hybrid Memory Tool route provenance drifted")
	}
	return result.UseMemory, nil
}

func redactHybridProviderPayload(
	query string,
	candidates []HybridShadowCandidate,
) (string, []string, error) {
	query = RedactMemoryProviderText(query, true)
	if strings.TrimSpace(query) == "" {
		return "", nil, errHybridProviderTextRedacted
	}
	documents := make([]string, len(candidates))
	for index, candidate := range candidates {
		documents[index] = RedactMemoryProviderText(candidate.Content, true)
		if strings.TrimSpace(documents[index]) == "" {
			return "", nil, errHybridProviderTextRedacted
		}
	}
	return query, documents, nil
}

func judgeHybridCandidates(
	ctx context.Context,
	judge HybridCandidateJudge,
	expectedModelID string,
	query string,
	documents []string,
) ([]int, error) {
	candidates := make([]HybridCandidateJudgeCandidate, len(documents))
	for ordinal, content := range documents {
		candidates[ordinal] = HybridCandidateJudgeCandidate{
			Ordinal: ordinal,
			Content: content,
		}
	}
	result, err := judge.JudgeHybridCandidates(ctx, HybridCandidateJudgeInput{
		Query: query, Candidates: candidates,
	})
	if err != nil || ctx.Err() != nil {
		return nil, errors.New("hybrid candidate judge request failed")
	}
	if result.ModelID != expectedModelID ||
		result.PromptVersion != HybridCandidateJudgePromptVersion ||
		result.PromptSHA256 != HybridCandidateJudgePromptSHA256 {
		return nil, errors.New("hybrid candidate judge provenance drifted")
	}
	return DecodeHybridCandidateJudgeOutput(result.RawOutput, len(documents))
}

func intersectHybridJudgeSelection(
	ordered []hybridShadowScoredCandidate,
	selectedOrdinals []int,
) []hybridShadowScoredCandidate {
	selected := make(map[int]struct{}, len(selectedOrdinals))
	for _, ordinal := range selectedOrdinals {
		selected[ordinal] = struct{}{}
	}
	result := make([]hybridShadowScoredCandidate, 0, len(selected))
	for _, candidate := range ordered {
		if _, ok := selected[candidate.CandidateOrdinal]; ok {
			result = append(result, candidate)
		}
	}
	return result
}

func selectHybridShadowFinal(
	candidates []hybridShadowScoredCandidate,
	minimumRelevanceScore float64,
) ([]HybridShadowRankedItem, int) {
	selected := make([]HybridShadowRankedItem, 0, min(len(candidates), HybridShadowFinalLimit))
	tokens := 0
	for _, scored := range candidates {
		if len(selected) == HybridShadowFinalLimit {
			break
		}
		if scored.RelevanceScore < minimumRelevanceScore {
			continue
		}
		candidate := scored.Candidate
		cost := estimateHybridMemoryTokens(candidate.Content)
		if cost <= 0 || cost > HybridShadowMaximumTokens || tokens+cost > HybridShadowMaximumTokens {
			continue
		}
		selected = append(selected, hybridRankedItem(candidate))
		tokens += cost
	}
	return selected, tokens
}

func HybridShadowCalibrationPolicy() HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID: HybridRelevanceCalibrationPolicyID, Mode: hybridPolicyModeCalibration,
		MinimumProviderSimilarity: -1, MinimumFinalRelevanceScore: 0,
	}
}

func HybridShadowIntentCalibrationPolicy() HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                         HybridRelevanceIntentCalibrationPolicyID,
		Mode:                       hybridPolicyModeIntentCalibration,
		MemoryIntentRequired:       true,
		MinimumMemoryIntentMargin:  -1,
		MinimumProviderSimilarity:  -1,
		MinimumFinalRelevanceScore: 0,
	}
}

func HybridShadowCloudJudgeCalibrationPolicy(
	modelID string,
) HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                          HybridRelevanceCloudJudgeCalibrationPolicyID,
		Mode:                        hybridPolicyModeCloudJudgeCalibration,
		CloudCandidateJudgeRequired: true,
		CloudCandidateJudgeModelID:  strings.TrimSpace(modelID),
		MinimumProviderSimilarity:   -1,
		MinimumFinalRelevanceScore:  0,
	}
}

// HybridShadowFixedMemoryJudgeDevelopmentPolicy is the separately versioned
// schema-v11 candidate-aware policy. It changes only the policy identity and
// complete-flow hard cutoff; the strict prompt/decoder and BGE selection
// contract remain shared with the historical cloud-judge policies.
func HybridShadowFixedMemoryJudgeDevelopmentPolicy() HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                          HybridRelevanceFixedMemoryJudgePolicyID,
		Mode:                        hybridPolicyModeFixedMemoryJudge,
		CloudCandidateJudgeRequired: true,
		CloudCandidateJudgeModelID:  HybridFixedMemoryJudgeModelID,
		MinimumProviderSimilarity:   -1,
		MinimumFinalRelevanceScore:  0,
	}
}

// HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy is a Development-only
// successor that removes application deadlines and executes BGE rerank before
// the fixed judge. It is never installed by the Server composition root.
func HybridShadowAccuracyFirstMemoryJudgeDevelopmentPolicy() HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                          HybridRelevanceAccuracyFirstJudgePolicyID,
		Mode:                        hybridPolicyModeAccuracyFirstJudge,
		CloudCandidateJudgeRequired: true,
		CloudCandidateJudgeModelID:  HybridFixedMemoryJudgeModelID,
		MinimumProviderSimilarity:   -1,
		MinimumFinalRelevanceScore:  0,
	}
}

// HybridShadowFixedMemoryJudgeProductionPolicy is the owner-promoted product
// reader policy. It preserves the passing schema-v14 accuracy-first selection
// order while giving product observations a production-only identity.
func HybridShadowFixedMemoryJudgeProductionPolicy() HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                          HybridRelevanceProductionJudgePolicyID,
		Mode:                        hybridPolicyModeProductionJudge,
		CloudCandidateJudgeRequired: true,
		CloudCandidateJudgeModelID:  HybridFixedMemoryJudgeModelID,
		MinimumProviderSimilarity:   -1,
		MinimumFinalRelevanceScore:  0,
	}
}

func HybridShadowMemoryToolRouteCalibrationPolicy(
	modelID string,
) HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                         HybridRelevanceMemoryToolRoutePolicyID,
		Mode:                       hybridPolicyModeMemoryToolRoute,
		MemoryToolRouteRequired:    true,
		MemoryToolRouteModelID:     strings.TrimSpace(modelID),
		MinimumProviderSimilarity:  -1,
		MinimumFinalRelevanceScore: 0,
	}
}

func HybridShadowMemoryFirstToolRoundCalibrationPolicy(
	modelID string,
) HybridShadowRelevancePolicy {
	return HybridShadowRelevancePolicy{
		ID:                         HybridRelevanceMemoryFirstToolRoundPolicyID,
		Mode:                       hybridPolicyModeMemoryFirstToolRound,
		MemoryToolRouteRequired:    true,
		MemoryToolRouteModelID:     strings.TrimSpace(modelID),
		MinimumProviderSimilarity:  -1,
		MinimumFinalRelevanceScore: 0,
	}
}

func HybridShadowFrozenPolicy() (HybridShadowRelevancePolicy, bool) {
	if !hybridFrozenPolicyReady {
		return HybridShadowRelevancePolicy{}, false
	}
	return HybridShadowRelevancePolicy{
		ID: HybridRelevanceFrozenPolicyID, Mode: hybridPolicyModeFrozen,
		MemoryIntentRequired:       true,
		MinimumMemoryIntentMargin:  float64(hybridFrozenMemoryIntentMarginBP) / 100,
		MinimumProviderSimilarity:  float64(hybridFrozenProviderSimilarityBP) / 100,
		MinimumFinalRelevanceScore: float64(hybridFrozenFinalRelevanceBP) / 100,
	}, true
}

func DescribeHybridShadowRelevancePolicy(
	policy HybridShadowRelevancePolicy,
) (HybridShadowRelevancePolicyDescriptor, bool) {
	policy, ok := validHybridShadowRelevancePolicy(policy)
	if !ok {
		return HybridShadowRelevancePolicyDescriptor{}, false
	}
	providerBasisPoints := int(math.Round(policy.MinimumProviderSimilarity * 100))
	finalBasisPoints := int(math.Round(policy.MinimumFinalRelevanceScore * 100))
	intentBasisPoints := int(math.Round(policy.MinimumMemoryIntentMargin * 100))
	if math.Abs(float64(providerBasisPoints)/100-policy.MinimumProviderSimilarity) > 1e-9 ||
		math.Abs(float64(finalBasisPoints)/100-policy.MinimumFinalRelevanceScore) > 1e-9 ||
		math.Abs(float64(intentBasisPoints)/100-policy.MinimumMemoryIntentMargin) > 1e-9 {
		return HybridShadowRelevancePolicyDescriptor{}, false
	}
	return HybridShadowRelevancePolicyDescriptor{
		ID: policy.ID, Mode: policy.Mode,
		HardCutoffMilliseconds:      hybridShadowHardCutoffMillisecondsForMode(policy.Mode),
		MemoryIntentRequired:        policy.MemoryIntentRequired,
		CloudCandidateJudgeRequired: policy.CloudCandidateJudgeRequired,
		CloudCandidateJudgeModelID:  policy.CloudCandidateJudgeModelID,
		CloudCandidateJudgePromptVersion: func() string {
			if policy.CloudCandidateJudgeRequired {
				return HybridCandidateJudgePromptVersion
			}
			return "none"
		}(),
		CloudCandidateJudgePromptSHA256: func() string {
			if policy.CloudCandidateJudgeRequired {
				return HybridCandidateJudgePromptSHA256
			}
			return "none"
		}(),
		CloudCandidateJudgeDecodingProfile: func() string {
			if policy.CloudCandidateJudgeRequired {
				return HybridCandidateJudgeDecodingProfile
			}
			return "none"
		}(),
		MemoryToolRouteRequired: policy.MemoryToolRouteRequired,
		MemoryToolRouteModelID:  policy.MemoryToolRouteModelID,
		MemoryToolRouteContractVersion: func() string {
			if policy.MemoryToolRouteRequired {
				return HybridMemoryToolContractVersion
			}
			return "none"
		}(),
		MemoryToolRouteContractSHA256: func() string {
			if policy.MemoryToolRouteRequired {
				return HybridMemoryToolContractSHA256
			}
			return "none"
		}(),
		MemoryToolRouteDecodingProfile: func() string {
			if policy.Mode == hybridPolicyModeMemoryToolRoute {
				return HybridMemoryToolDecodingProfile
			}
			return "none"
		}(),
		MemoryToolRouteMaximumOutputTokens: func() int {
			if policy.Mode == hybridPolicyModeMemoryToolRoute {
				return HybridMemoryToolMaximumOutputTokens
			}
			return 0
		}(),
		MemoryToolRouteTemperature: func() float64 {
			if policy.Mode == hybridPolicyModeMemoryToolRoute {
				return HybridMemoryToolTemperature
			}
			return 0
		}(),
		MemoryToolRouteDisableThinking: policy.Mode == hybridPolicyModeMemoryToolRoute &&
			HybridMemoryToolDisableThinking,
		MemoryIntentAnchorVersion: func() string {
			if policy.MemoryIntentRequired {
				return ragproviders.MemoryIntentAnchorVersion
			}
			return "none"
		}(),
		MemoryIntentAnchorSHA256: func() string {
			if policy.MemoryIntentRequired {
				return ragproviders.MemoryIntentAnchorSHA256
			}
			return "none"
		}(),
		MinimumMemoryIntentMarginBasisPoints: intentBasisPoints,
		MinimumProviderSimilarityBasisPoints: providerBasisPoints,
		MinimumFinalRelevanceBasisPoints:     finalBasisPoints,
	}, true
}

func validHybridShadowRelevancePolicy(
	policy HybridShadowRelevancePolicy,
) (HybridShadowRelevancePolicy, bool) {
	if !lexicalShadowResultCodeRE.MatchString(strings.ToUpper(policy.ID)) ||
		math.IsNaN(policy.MinimumMemoryIntentMargin) ||
		math.IsInf(policy.MinimumMemoryIntentMargin, 0) ||
		policy.MinimumMemoryIntentMargin < -1 || policy.MinimumMemoryIntentMargin > 1 ||
		math.IsNaN(policy.MinimumProviderSimilarity) ||
		math.IsInf(policy.MinimumProviderSimilarity, 0) ||
		policy.MinimumProviderSimilarity < -1 || policy.MinimumProviderSimilarity > 1 ||
		math.IsNaN(policy.MinimumFinalRelevanceScore) ||
		math.IsInf(policy.MinimumFinalRelevanceScore, 0) ||
		policy.MinimumFinalRelevanceScore < 0 || policy.MinimumFinalRelevanceScore > 1 {
		return HybridShadowRelevancePolicy{}, false
	}
	switch policy.Mode {
	case hybridPolicyModeCalibration:
		if policy.ID != HybridRelevanceCalibrationPolicyID ||
			policy.MemoryIntentRequired || policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != "" || policy.MemoryToolRouteRequired ||
			policy.MemoryToolRouteModelID != "" || policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 || policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeIntentCalibration:
		if policy.ID != HybridRelevanceIntentCalibrationPolicyID ||
			!policy.MemoryIntentRequired || policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != "" || policy.MemoryToolRouteRequired ||
			policy.MemoryToolRouteModelID != "" || policy.MinimumMemoryIntentMargin != -1 ||
			policy.MinimumProviderSimilarity != -1 || policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeCloudJudgeCalibration:
		if policy.ID != HybridRelevanceCloudJudgeCalibrationPolicyID ||
			policy.MemoryIntentRequired || !policy.CloudCandidateJudgeRequired ||
			!validHybridCandidateJudgeModelID(policy.CloudCandidateJudgeModelID) ||
			policy.MemoryToolRouteRequired || policy.MemoryToolRouteModelID != "" ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeFixedMemoryJudge:
		if policy.ID != HybridRelevanceFixedMemoryJudgePolicyID ||
			policy.MemoryIntentRequired || !policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID ||
			policy.MemoryToolRouteRequired || policy.MemoryToolRouteModelID != "" ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeAccuracyFirstJudge:
		if policy.ID != HybridRelevanceAccuracyFirstJudgePolicyID ||
			policy.MemoryIntentRequired || !policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID ||
			policy.MemoryToolRouteRequired || policy.MemoryToolRouteModelID != "" ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeProductionJudge:
		if policy.ID != HybridRelevanceProductionJudgePolicyID ||
			policy.MemoryIntentRequired || !policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != HybridFixedMemoryJudgeModelID ||
			policy.MemoryToolRouteRequired || policy.MemoryToolRouteModelID != "" ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeMemoryToolRoute:
		if policy.ID != HybridRelevanceMemoryToolRoutePolicyID ||
			policy.MemoryIntentRequired || policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != "" || !policy.MemoryToolRouteRequired ||
			!validHybridCandidateJudgeModelID(policy.MemoryToolRouteModelID) ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeMemoryFirstToolRound:
		if policy.ID != HybridRelevanceMemoryFirstToolRoundPolicyID ||
			policy.MemoryIntentRequired || policy.CloudCandidateJudgeRequired ||
			policy.CloudCandidateJudgeModelID != "" || !policy.MemoryToolRouteRequired ||
			!validHybridCandidateJudgeModelID(policy.MemoryToolRouteModelID) ||
			policy.MinimumMemoryIntentMargin != 0 ||
			policy.MinimumProviderSimilarity != -1 ||
			policy.MinimumFinalRelevanceScore != 0 {
			return HybridShadowRelevancePolicy{}, false
		}
	case hybridPolicyModeFrozen:
		if policy.ID != HybridRelevanceFrozenPolicyID || !policy.MemoryIntentRequired ||
			policy.CloudCandidateJudgeRequired || policy.CloudCandidateJudgeModelID != "" ||
			policy.MemoryToolRouteRequired || policy.MemoryToolRouteModelID != "" {
			return HybridShadowRelevancePolicy{}, false
		}
	default:
		return HybridShadowRelevancePolicy{}, false
	}
	return policy, true
}

// HybridShadowHardCutoffMilliseconds exposes the exact complete-flow cutoff
// bound to an admitted policy. Historical policies retain 2000 ms, schema v11
// receives 3000 ms, and accuracy-first Development/production policies return
// zero to mean that they add no application deadline beyond the caller and
// Provider transport bounds.
func HybridShadowHardCutoffMilliseconds(policy HybridShadowRelevancePolicy) (int, bool) {
	policy, ok := validHybridShadowRelevancePolicy(policy)
	if !ok {
		return 0, false
	}
	return hybridShadowHardCutoffMillisecondsForMode(policy.Mode), true
}

func hybridShadowHardCutoffMillisecondsForMode(mode string) int {
	if hybridPolicyRunsAccuracyFirst(mode) {
		return 0
	}
	if mode == hybridPolicyModeFixedMemoryJudge {
		return HybridFixedMemoryJudgeHardCutoffMilliseconds
	}
	return int(hybridShadowHardCutoff / time.Millisecond)
}

func hybridPolicyRunsAccuracyFirst(mode string) bool {
	return mode == hybridPolicyModeAccuracyFirstJudge ||
		mode == hybridPolicyModeProductionJudge
}

func validHybridCandidateJudgeModelID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func classifyHybridMemoryIntent(
	ctx context.Context,
	provider HybridShadowProvider,
	query string,
) (ragproviders.MemoryIntentSignal, error) {
	query = RedactMemoryProviderText(query, true)
	if strings.TrimSpace(query) == "" || provider == nil {
		return ragproviders.MemoryIntentSignal{}, ragproviders.ErrMemoryIntentUnavailable
	}
	classifier, ok := provider.(ragproviders.MemoryIntentClassifier)
	if !ok || classifier == nil {
		return ragproviders.MemoryIntentSignal{}, ragproviders.ErrMemoryIntentUnavailable
	}
	signal, err := classifier.ClassifyMemoryIntent(ctx, query)
	if err != nil || ctx.Err() != nil ||
		signal.AnchorVersion != ragproviders.MemoryIntentAnchorVersion ||
		signal.AnchorSHA256 != ragproviders.MemoryIntentAnchorSHA256 ||
		math.IsNaN(signal.PositiveScore) || math.IsInf(signal.PositiveScore, 0) ||
		signal.PositiveScore < 0 || signal.PositiveScore > 1 ||
		math.IsNaN(signal.NegativeScore) || math.IsInf(signal.NegativeScore, 0) ||
		signal.NegativeScore < 0 || signal.NegativeScore > 1 ||
		math.IsNaN(signal.Margin) || math.IsInf(signal.Margin, 0) ||
		signal.Margin < -1 || signal.Margin > 1 ||
		math.Abs(signal.Margin-(signal.PositiveScore-signal.NegativeScore)) > 1e-9 {
		return ragproviders.MemoryIntentSignal{}, ragproviders.ErrMemoryIntentInvalid
	}
	return signal, nil
}

func validHybridShadowAdmission(value HybridShadowAdmission, expected int) bool {
	return expected > 0 && value.CandidateCount == expected &&
		value.VectorCandidateCount == expected &&
		!math.IsNaN(value.MaximumVectorSimilarity) &&
		!math.IsInf(value.MaximumVectorSimilarity, 0) &&
		value.MaximumVectorSimilarity >= -1 && value.MaximumVectorSimilarity <= 1
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
