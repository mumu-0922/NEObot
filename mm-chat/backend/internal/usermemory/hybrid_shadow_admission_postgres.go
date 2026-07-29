package usermemory

import (
	"context"
	"fmt"
	"math"

	"neo-chat/mm-chat/backend/internal/auth"
)

// AuthorizeHybridRerank revalidates the complete prepared RRF surface and
// returns one transient local cosine signal. The SECURITY DEFINER function
// persists neither the query vector nor the similarity.
func (r *PostgresRepository) AuthorizeHybridRerank(
	ctx context.Context,
	input HybridShadowAdmissionInput,
) (HybridShadowAdmission, error) {
	if err := r.requireDB(); err != nil {
		return HybridShadowAdmission{}, err
	}
	if !uuidRE.MatchString(input.ObservationID) ||
		!uuidRE.MatchString(input.AssistantMessageID) ||
		len(input.QueryHash) != 64 || !validHybridVector(input.QueryEmbedding) {
		return HybridShadowAdmission{}, fmt.Errorf("authorize hybrid rerank: invalid input")
	}
	user := auth.UserOrDevelopment(ctx)
	var result HybridShadowAdmission
	err := r.db.QueryRowContext(ctx, `
SELECT candidate_count, vector_candidate_count, maximum_vector_similarity
FROM memory_authorize_hybrid_rerank(
  $1::uuid, $2::uuid, $3::uuid, $4, $5::real[]
)
`,
		input.ObservationID,
		user.ID,
		input.AssistantMessageID,
		input.QueryHash,
		hybridRealArrayLiteral(input.QueryEmbedding),
	).Scan(
		&result.CandidateCount,
		&result.VectorCandidateCount,
		&result.MaximumVectorSimilarity,
	)
	if err != nil {
		return HybridShadowAdmission{}, fmt.Errorf("authorize hybrid rerank: %w", err)
	}
	if result.CandidateCount < 1 || result.CandidateCount > MaxHybridShadowResults ||
		result.VectorCandidateCount < 0 || result.VectorCandidateCount > result.CandidateCount ||
		math.IsNaN(result.MaximumVectorSimilarity) ||
		math.IsInf(result.MaximumVectorSimilarity, 0) ||
		result.MaximumVectorSimilarity < -1 || result.MaximumVectorSimilarity > 1 {
		return HybridShadowAdmission{}, fmt.Errorf("authorize hybrid rerank: invalid response")
	}
	return result, nil
}

var _ HybridShadowAdmissionRepository = (*PostgresRepository)(nil)
