package usermemory

import (
	"context"
	"encoding/json"
	"fmt"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) CompareLexicalShadow(
	ctx context.Context,
	input LexicalShadowInput,
) (LexicalShadowSummary, error) {
	if err := r.requireDB(); err != nil {
		return LexicalShadowSummary{}, err
	}
	baseline, err := json.Marshal(input.Baseline)
	if err != nil {
		return LexicalShadowSummary{}, fmt.Errorf("encode lexical shadow baseline: %w", err)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID string
	var projectionGeneration int64
	var summary LexicalShadowSummary
	err = r.db.QueryRowContext(ctx, `
SELECT observation_id, profile_id, projection_generation,
       status, result_code, baseline_count, exact_count, bm25_count,
       lexical_count, overlap_count, duration_millis
FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7::jsonb, $8
)
`,
		input.ObservationID,
		user.ID,
		input.ConversationID,
		input.AssistantMessageID,
		input.QueryHash,
		input.QueryText,
		string(baseline),
		input.LexicalLimit,
	).Scan(
		&observationID,
		&summary.ProfileID,
		&projectionGeneration,
		&summary.Status,
		&summary.ResultCode,
		&summary.BaselineCount,
		&summary.ExactCount,
		&summary.BM25Count,
		&summary.LexicalCount,
		&summary.OverlapCount,
		&summary.DurationMillis,
	)
	if err != nil {
		return LexicalShadowSummary{}, fmt.Errorf("compare lexical memory shadow: %w", err)
	}
	if !uuidRE.MatchString(observationID) || projectionGeneration < 1 {
		return LexicalShadowSummary{}, fmt.Errorf("compare lexical memory shadow: invalid response authority")
	}
	return summary, nil
}

var _ LexicalShadowRepository = (*PostgresRepository)(nil)
