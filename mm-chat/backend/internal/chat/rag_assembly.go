package chat

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const (
	defaultRAGCandidateLimit = 20
	defaultRAGEvidenceLimit  = 5
	ragRRFConstant           = 60.0
)

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

type RAGAnswerAssembler struct {
	Candidates     RAGCandidateSource
	Hydrator       RAGEvidenceHydrator
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
	Evidence  []knowledge.HydratedEvidence
	Citations []RAGCitation
}

func NewRAGAnswerAssembler(candidates RAGCandidateSource, hydrator RAGEvidenceHydrator) *RAGAnswerAssembler {
	return &RAGAnswerAssembler{
		Candidates:     candidates,
		Hydrator:       hydrator,
		CandidateLimit: defaultRAGCandidateLimit,
		EvidenceLimit:  defaultRAGEvidenceLimit,
	}
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
	if len(candidates) > evidenceLimit {
		candidates = candidates[:evidenceLimit]
	}
	evidence, err := a.Hydrator.ReauthorizeAndHydrateEvidence(ctx, knowledge.ReauthorizeEvidenceInput{
		ActorUserID:           input.ActorUserID,
		SessionID:             input.SessionID,
		ConversationID:        input.ConversationID,
		SelectedCollectionIDs: append([]string(nil), input.SelectedCollectionIDs...),
		References:            candidates,
	})
	if err != nil {
		if errors.Is(err, knowledge.ErrEvidenceHydrationRejected) {
			return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
		}
		return RAGAssemblyResult{}, fmt.Errorf("hydrate rag evidence: %w", err)
	}
	if len(evidence) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}

	citations, err := mintRAGCitations(evidence)
	if err != nil {
		return RAGAssemblyResult{}, err
	}

	return RAGAssemblyResult{Evidence: evidence, Citations: citations}, nil
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
