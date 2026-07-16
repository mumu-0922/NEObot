package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const (
	defaultRAGCandidateLimit = 8
	rawRAGRefusalText        = "I don't have enough verified knowledge-base evidence to answer that."
)

var (
	ErrRAGDependencyUnavailable = errors.New("rag dependency unavailable")
	ErrRAGInsufficientEvidence  = errors.New("rag insufficient evidence")
	ErrRAGAnswerGatePending     = errors.New("rag answer gate pending")
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
	Candidates RAGCandidateSource
	Hydrator   RAGEvidenceHydrator
	Limit      int
}

type RAGAssemblyInput struct {
	ActorUserID           string
	SessionID             string
	ConversationID        string
	QueryText             string
	SelectedCollectionIDs []string
}

type RAGAssemblyResult struct {
	Evidence  []knowledge.HydratedEvidence
	Citations []RAGCitation
}

func NewRAGAnswerAssembler(candidates RAGCandidateSource, hydrator RAGEvidenceHydrator) *RAGAnswerAssembler {
	return &RAGAnswerAssembler{Candidates: candidates, Hydrator: hydrator, Limit: defaultRAGCandidateLimit}
}

func (a *RAGAnswerAssembler) AssembleStrict(ctx context.Context, input RAGAssemblyInput) (RAGAssemblyResult, error) {
	if a == nil || a.Candidates == nil || a.Hydrator == nil {
		return RAGAssemblyResult{}, ErrRAGDependencyUnavailable
	}
	input.QueryText = strings.TrimSpace(input.QueryText)
	if input.ActorUserID == "" || input.SessionID == "" || input.ConversationID == "" || input.QueryText == "" || len(input.SelectedCollectionIDs) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	limit := a.Limit
	if limit <= 0 || limit > 16 {
		limit = defaultRAGCandidateLimit
	}
	candidates, err := a.Candidates.FetchEvidenceCandidates(ctx, RAGCandidateQuery{
		CollectionIDs: append([]string(nil), input.SelectedCollectionIDs...),
		QueryText:     input.QueryText,
		Limit:         limit,
	})
	if err != nil {
		return RAGAssemblyResult{}, fmt.Errorf("fetch rag candidates: %w", err)
	}
	if len(candidates) == 0 {
		return RAGAssemblyResult{}, ErrRAGInsufficientEvidence
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
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

	// G7.7B deliberately stops before answer-provider context injection.  The
	// next slice must add answer-purpose BYOK/governance consent before hydrated
	// source text is sent to a model provider.
	return RAGAssemblyResult{Evidence: evidence, Citations: citations}, ErrRAGAnswerGatePending
}

func ragRefusalText() string {
	return rawRAGRefusalText
}
