package chat

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const (
	defaultRAGCandidateLimit = 20
	defaultRAGEvidenceLimit  = 5
	ragRRFConstant           = 60.0
	ragHydrationBatchLimit   = 16
	ragRerankStatusDisabled  = "disabled"
	ragRerankStatusApplied   = "applied"
)

var ragGoldenRelevancePolicyV2 = ragRelevancePolicy{
	ID: "g18-profiled-reranker-golden-v2", MinimumScore: 0.0,
	ExplicitSourceNameBoost: 2.0,
}

type ragRelevancePolicy struct {
	ID                      string
	MinimumScore            float64
	ExplicitSourceNameBoost float64
}

var (
	ErrRAGDependencyUnavailable = errors.New("rag dependency unavailable")
	ErrRAGInsufficientEvidence  = errors.New("rag insufficient evidence")
)

type RAGCandidateQuery struct {
	CollectionIDs []string
	QueryText     string
	Limit         int
}

type RAGCandidateSource interface {
	FetchEvidenceCandidates(context.Context, RAGCandidateQuery) ([]knowledge.EvidenceCandidateReference, error)
}

type RAGEvidenceHydrator interface {
	ReauthorizeAndHydrateEvidence(context.Context, knowledge.ReauthorizeEvidenceInput) ([]knowledge.HydratedEvidence, error)
}

type RAGRerankResult struct {
	Index          int
	RelevanceScore float64
}

type RAGEvidenceReranker interface {
	Rerank(context.Context, string, string, []string) ([]RAGRerankResult, error)
}

type RAGAnswerAssembler struct {
	Candidates     RAGCandidateSource
	Hydrator       RAGEvidenceHydrator
	Reranker       RAGEvidenceReranker
	RerankGate     RAGRerankGovernanceGate
	CandidateLimit int
	EvidenceLimit  int
}

type RAGAssemblyInput struct {
	ActorUserID           string
	SessionID             string
	ConversationID        string
	QueryText             string
	RewrittenQueryText    string
	SelectedCollectionIDs []string
}

type RAGAssemblyResult struct {
	Evidence     []knowledge.HydratedEvidence
	Citations    []RAGCitation
	RerankStatus string
}

type RAGAnswerAssemblerOption func(*RAGAnswerAssembler)

func WithRAGEvidenceReranker(
	reranker RAGEvidenceReranker,
	gate RAGRerankGovernanceGate,
) RAGAnswerAssemblerOption {
	return func(assembler *RAGAnswerAssembler) {
		assembler.Reranker = reranker
		assembler.RerankGate = gate
	}
}

func NewRAGAnswerAssembler(
	candidates RAGCandidateSource,
	hydrator RAGEvidenceHydrator,
	options ...RAGAnswerAssemblerOption,
) *RAGAnswerAssembler {
	assembler := &RAGAnswerAssembler{
		Candidates:     candidates,
		Hydrator:       hydrator,
		CandidateLimit: defaultRAGCandidateLimit,
		EvidenceLimit:  defaultRAGEvidenceLimit,
	}
	for _, option := range options {
		if option != nil {
			option(assembler)
		}
	}
	return assembler
}

func (a *RAGAnswerAssembler) Assemble(ctx context.Context, input RAGAssemblyInput) (RAGAssemblyResult, error) {
	if a == nil || a.Candidates == nil || a.Hydrator == nil {
		return RAGAssemblyResult{}, ErrRAGDependencyUnavailable
	}
	input.QueryText = strings.TrimSpace(input.QueryText)
	if input.ActorUserID == "" || input.SessionID == "" || input.ConversationID == "" || input.QueryText == "" || len(input.SelectedCollectionIDs) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	candidateLimit := a.CandidateLimit
	if candidateLimit <= 0 || candidateLimit > 50 {
		candidateLimit = defaultRAGCandidateLimit
	}
	evidenceLimit := a.EvidenceLimit
	if evidenceLimit <= 0 || evidenceLimit > 16 {
		evidenceLimit = defaultRAGEvidenceLimit
	}
	queryTexts := []string{input.QueryText}
	if rewritten := strings.TrimSpace(input.RewrittenQueryText); rewritten != "" && !strings.EqualFold(rewritten, input.QueryText) {
		queryTexts = append(queryTexts, rewritten)
	}
	candidateLanes := make([][]knowledge.EvidenceCandidateReference, 0, len(queryTexts))
	for _, queryText := range queryTexts {
		candidates, err := a.Candidates.FetchEvidenceCandidates(ctx, RAGCandidateQuery{
			CollectionIDs: append([]string(nil), input.SelectedCollectionIDs...),
			QueryText:     queryText,
			Limit:         candidateLimit,
		})
		if err != nil {
			return RAGAssemblyResult{}, fmt.Errorf("fetch rag candidates: %w", err)
		}
		if len(candidates) > candidateLimit {
			candidates = candidates[:candidateLimit]
		}
		candidateLanes = append(candidateLanes, candidates)
	}
	candidates := fuseRAGCandidateLanes(candidateLanes, candidateLimit)
	if len(candidates) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	indexGenerationID, ok := singleRAGCandidateGeneration(candidates)
	if !ok {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	rerankStatus := ragRerankStatusDisabled
	rerankConfigured := a.Reranker != nil || a.RerankGate != nil
	useRerank := a.Reranker != nil && a.RerankGate != nil
	if rerankConfigured && !useRerank {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	if useRerank {
		if err := a.RerankGate.AuthorizeRAGRerank(
			ctx,
			input.SelectedCollectionIDs,
			indexGenerationID,
		); err != nil {
			if errors.Is(err, ErrRAGRerankGovernanceRequired) ||
				errors.Is(err, ErrRAGDependencyUnavailable) {
				return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
			} else {
				return RAGAssemblyResult{}, fmt.Errorf("authorize rag rerank: %w", err)
			}
		}
	}
	if !useRerank && len(candidates) > evidenceLimit {
		candidates = candidates[:evidenceLimit]
	}
	evidence, err := a.hydrateCandidateBatches(ctx, input, candidates)
	if err != nil {
		if errors.Is(err, knowledge.ErrEvidenceHydrationRejected) {
			return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
		}
		return RAGAssemblyResult{}, fmt.Errorf("hydrate rag evidence: %w", err)
	}
	if len(evidence) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	if useRerank {
		rerankQuery := input.QueryText
		if rewritten := strings.TrimSpace(input.RewrittenQueryText); rewritten != "" {
			rerankQuery = rewritten
		}
		documents := make([]string, len(evidence))
		for index := range evidence {
			documents[index] = formatRAGRerankDocument(evidence[index])
		}
		scores, rerankErr := a.Reranker.Rerank(
			ctx,
			indexGenerationID,
			rerankQuery,
			documents,
		)
		if rerankErr != nil {
			return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
		} else if ranked, ok := applyRAGRerank(
			evidence,
			scores,
			evidenceLimit,
			ragGoldenRelevancePolicyV2,
			input.QueryText,
			input.RewrittenQueryText,
		); ok {
			evidence = ranked
			rerankStatus = ragRerankStatusApplied
		} else {
			return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
		}
	}
	if !useRerank && len(evidence) > evidenceLimit {
		evidence = evidence[:evidenceLimit]
	}
	if len(evidence) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}

	citations, err := mintRAGCitations(evidence)
	if err != nil {
		return RAGAssemblyResult{}, err
	}

	return RAGAssemblyResult{
		Evidence: evidence, Citations: citations, RerankStatus: rerankStatus,
	}, nil
}

func singleRAGCandidateGeneration(
	candidates []knowledge.EvidenceCandidateReference,
) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}
	generationID := strings.TrimSpace(candidates[0].IndexGenerationID)
	if generationID == "" {
		return "", false
	}
	for _, candidate := range candidates[1:] {
		if strings.TrimSpace(candidate.IndexGenerationID) != generationID {
			return "", false
		}
	}
	return generationID, true
}

func (a *RAGAnswerAssembler) hydrateCandidateBatches(
	ctx context.Context,
	input RAGAssemblyInput,
	candidates []knowledge.EvidenceCandidateReference,
) ([]knowledge.HydratedEvidence, error) {
	hydrated := make([]knowledge.HydratedEvidence, 0, len(candidates))
	for start := 0; start < len(candidates); start += ragHydrationBatchLimit {
		end := min(start+ragHydrationBatchLimit, len(candidates))
		batch, err := a.Hydrator.ReauthorizeAndHydrateEvidence(
			ctx,
			knowledge.ReauthorizeEvidenceInput{
				ActorUserID:           input.ActorUserID,
				SessionID:             input.SessionID,
				ConversationID:        input.ConversationID,
				SelectedCollectionIDs: append([]string(nil), input.SelectedCollectionIDs...),
				References:            append([]knowledge.EvidenceCandidateReference(nil), candidates[start:end]...),
			},
		)
		if err != nil {
			return nil, err
		}
		hydrated = append(hydrated, batch...)
	}
	if len(hydrated) != len(candidates) {
		return nil, knowledge.ErrEvidenceHydrationRejected
	}
	return hydrated, nil
}

func applyRAGRerank(
	evidence []knowledge.HydratedEvidence,
	results []RAGRerankResult,
	limit int,
	policy ragRelevancePolicy,
	sourceQueries ...string,
) ([]knowledge.HydratedEvidence, bool) {
	if len(evidence) == 0 || len(results) != len(evidence) ||
		strings.TrimSpace(policy.ID) == "" || math.IsNaN(policy.MinimumScore) ||
		math.IsInf(policy.MinimumScore, 0) ||
		math.IsNaN(policy.ExplicitSourceNameBoost) ||
		math.IsInf(policy.ExplicitSourceNameBoost, 0) ||
		policy.ExplicitSourceNameBoost < 0 {
		return nil, false
	}
	seen := make([]bool, len(evidence))
	ranked := make([]knowledge.HydratedEvidence, 0, len(evidence))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(evidence) || seen[result.Index] ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) {
			return nil, false
		}
		seen[result.Index] = true
		if result.RelevanceScore < policy.MinimumScore {
			continue
		}
		item := evidence[result.Index]
		item.RankScore = result.RelevanceScore
		for _, query := range sourceQueries {
			if knowledge.QueryExplicitlyNamesSource(query, item.SourceName) {
				item.RankScore += policy.ExplicitSourceNameBoost
				break
			}
		}
		ranked = append(ranked, item)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].RankScore != ranked[j].RankScore {
			return ranked[i].RankScore > ranked[j].RankScore
		}
		return ranked[i].ChildChunkID < ranked[j].ChildChunkID
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, true
}

func fuseRAGCandidateLanes(
	lanes [][]knowledge.EvidenceCandidateReference,
	limit int,
) []knowledge.EvidenceCandidateReference {
	type fusedCandidate struct {
		reference knowledge.EvidenceCandidateReference
		score     float64
		firstSeen int
		key       string
	}
	byKey := map[string]*fusedCandidate{}
	nextSeen := 0
	for _, lane := range lanes {
		laneSeen := map[string]struct{}{}
		for rank, reference := range lane {
			key := ragCandidateReferenceKey(reference)
			if key == "" {
				continue
			}
			if _, duplicate := laneSeen[key]; duplicate {
				continue
			}
			laneSeen[key] = struct{}{}
			candidate, ok := byKey[key]
			if !ok {
				candidate = &fusedCandidate{reference: reference, firstSeen: nextSeen, key: key}
				byKey[key] = candidate
				nextSeen++
			}
			candidate.score += 1.0 / (ragRRFConstant + float64(rank+1))
		}
	}
	fused := make([]fusedCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		candidate.reference.RankScore = candidate.score
		fused = append(fused, *candidate)
	}
	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].score != fused[j].score {
			return fused[i].score > fused[j].score
		}
		if fused[i].firstSeen != fused[j].firstSeen {
			return fused[i].firstSeen < fused[j].firstSeen
		}
		return fused[i].key < fused[j].key
	})
	if limit > 0 && len(fused) > limit {
		fused = fused[:limit]
	}
	result := make([]knowledge.EvidenceCandidateReference, 0, len(fused))
	for _, candidate := range fused {
		result = append(result, candidate.reference)
	}
	return result
}

func ragCandidateReferenceKey(reference knowledge.EvidenceCandidateReference) string {
	if strings.TrimSpace(reference.CollectionID) == "" || strings.TrimSpace(reference.ChildChunkID) == "" {
		return ""
	}
	return strings.Join([]string{
		reference.CollectionID,
		reference.DocumentVersionID,
		reference.IndexGenerationID,
		reference.MaterializationID,
		reference.ChildChunkID,
		reference.SourceSpanHash,
		reference.ContentHash,
	}, "|")
}
