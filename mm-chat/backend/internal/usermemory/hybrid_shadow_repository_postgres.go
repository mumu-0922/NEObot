package usermemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) PrepareHybridShadow(
	ctx context.Context,
	input HybridShadowPrepareInput,
) (HybridShadowPreparation, error) {
	if err := r.requireDB(); err != nil {
		return HybridShadowPreparation{}, err
	}
	baseline, err := json.Marshal(input.Baseline)
	if err != nil {
		return HybridShadowPreparation{}, fmt.Errorf("encode hybrid shadow baseline: %w", err)
	}
	var embedding any
	if len(input.QueryEmbedding) > 0 {
		if !validHybridVector(input.QueryEmbedding) {
			return HybridShadowPreparation{}, fmt.Errorf("prepare hybrid memory shadow: invalid query vector")
		}
		embedding = hybridRealArrayLiteral(input.QueryEmbedding)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID string
	var projectionGeneration int64
	var candidates []byte
	var prepared HybridShadowPreparation
	err = r.db.QueryRowContext(ctx, `
SELECT observation_id, profile_id, projection_generation, status,
       result_code, replayed, baseline_count, exact_count, bm25_count,
       vector_count, rrf_count, rerank_count, final_count, overlap_count,
       estimated_tokens, target_tokens_exceeded, fallback_code,
       duration_millis, candidates
FROM memory_prepare_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7::jsonb,
  $8::real[], $9
)
`,
		input.ObservationID,
		user.ID,
		input.ConversationID,
		input.AssistantMessageID,
		input.QueryHash,
		input.QueryText,
		string(baseline),
		embedding,
		input.QueryEmbeddingState,
	).Scan(
		&observationID,
		&prepared.Summary.ProfileID,
		&projectionGeneration,
		&prepared.Summary.Status,
		&prepared.Summary.ResultCode,
		&prepared.Replayed,
		&prepared.Summary.BaselineCount,
		&prepared.Summary.ExactCount,
		&prepared.Summary.BM25Count,
		&prepared.Summary.VectorCount,
		&prepared.Summary.RRFCount,
		&prepared.Summary.RerankCount,
		&prepared.Summary.FinalCount,
		&prepared.Summary.OverlapCount,
		&prepared.Summary.EstimatedTokens,
		&prepared.Summary.TargetTokensExceeded,
		&prepared.Summary.FallbackCode,
		&prepared.Summary.DurationMillis,
		&candidates,
	)
	if err != nil {
		return HybridShadowPreparation{}, fmt.Errorf("prepare hybrid memory shadow: %w", err)
	}
	if !uuidRE.MatchString(observationID) || projectionGeneration < 1 ||
		prepared.Summary.ProfileID != HybridShadowProfileID {
		return HybridShadowPreparation{}, fmt.Errorf("prepare hybrid memory shadow: invalid response authority")
	}
	prepared.ObservationID = observationID
	if err := json.Unmarshal(candidates, &prepared.Candidates); err != nil {
		return HybridShadowPreparation{}, fmt.Errorf("decode hybrid memory candidates: %w", err)
	}
	if len(prepared.Candidates) > MaxHybridShadowResults {
		return HybridShadowPreparation{}, fmt.Errorf("decode hybrid memory candidates: count exceeded")
	}
	for _, candidate := range prepared.Candidates {
		if !uuidRE.MatchString(candidate.MemoryID) || candidate.Revision < 1 ||
			!validHybridScope(candidate.ScopeType) || strings.TrimSpace(candidate.Content) == "" ||
			len([]rune(candidate.Content)) > MaxContentChars {
			return HybridShadowPreparation{}, fmt.Errorf("decode hybrid memory candidates: invalid candidate")
		}
	}
	return prepared, nil
}

func (r *PostgresRepository) RecordHybridShadow(
	ctx context.Context,
	input HybridShadowRecordInput,
) (HybridShadowSummary, error) {
	if err := r.requireDB(); err != nil {
		return HybridShadowSummary{}, err
	}
	reranked, err := json.Marshal(input.Reranked)
	if err != nil {
		return HybridShadowSummary{}, fmt.Errorf("encode hybrid rerank results: %w", err)
	}
	final, err := json.Marshal(input.Final)
	if err != nil {
		return HybridShadowSummary{}, fmt.Errorf("encode hybrid final results: %w", err)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID string
	var projectionGeneration int64
	var summary HybridShadowSummary
	err = r.db.QueryRowContext(ctx, `
SELECT observation_id, profile_id, projection_generation, status,
       result_code, baseline_count, exact_count, bm25_count, vector_count,
       rrf_count, rerank_count, final_count, overlap_count, estimated_tokens,
       target_tokens_exceeded, fallback_code, duration_millis
FROM memory_record_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6::jsonb, $7::jsonb,
  $8, $9, $10
)
`,
		input.ObservationID,
		user.ID,
		input.AssistantMessageID,
		input.RerankStatus,
		input.FallbackCode,
		string(reranked),
		string(final),
		input.EstimatedTokens,
		input.TargetTokensExceeded,
		input.DurationMillis,
	).Scan(
		&observationID,
		&summary.ProfileID,
		&projectionGeneration,
		&summary.Status,
		&summary.ResultCode,
		&summary.BaselineCount,
		&summary.ExactCount,
		&summary.BM25Count,
		&summary.VectorCount,
		&summary.RRFCount,
		&summary.RerankCount,
		&summary.FinalCount,
		&summary.OverlapCount,
		&summary.EstimatedTokens,
		&summary.TargetTokensExceeded,
		&summary.FallbackCode,
		&summary.DurationMillis,
	)
	if err != nil {
		return HybridShadowSummary{}, fmt.Errorf("record hybrid memory shadow: %w", err)
	}
	if !uuidRE.MatchString(observationID) || projectionGeneration < 1 ||
		summary.ProfileID != HybridShadowProfileID {
		return HybridShadowSummary{}, fmt.Errorf("record hybrid memory shadow: invalid response authority")
	}
	return summary, nil
}

func hybridRealArrayLiteral(values []float32) string {
	var builder strings.Builder
	builder.Grow(len(values) * 12)
	builder.WriteByte('{')
	for index, value := range values {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte('}')
	return builder.String()
}

func validHybridScope(value string) bool {
	switch value {
	case "global", "project", "conversation":
		return true
	default:
		return false
	}
}

var _ HybridShadowRepository = (*PostgresRepository)(nil)
