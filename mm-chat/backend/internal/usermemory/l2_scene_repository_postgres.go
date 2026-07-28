package usermemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) PrepareL2SceneSearch(
	ctx context.Context,
	input L2ScenePrepareInput,
) (L2ScenePreparation, error) {
	if err := r.requireDB(); err != nil {
		return L2ScenePreparation{}, err
	}
	var embedding any
	if len(input.QueryEmbedding) > 0 {
		if !validHybridVector(input.QueryEmbedding) {
			return L2ScenePreparation{}, fmt.Errorf("prepare L2 Scene search: invalid query vector")
		}
		embedding = hybridRealArrayLiteral(input.QueryEmbedding)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID, mode string
	var generation int64
	var candidates []byte
	var prepared L2ScenePreparation
	err := r.db.QueryRowContext(ctx, `
SELECT observation_id::text, mode, profile_id, generation, status,
       result_code, replayed, exact_count, bm25_count, vector_count,
       rrf_count, fallback_code, candidates
FROM memory_prepare_l2_scene_search(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7::real[], $8, $9
)
`, input.ObservationID, user.ID, input.ConversationID, input.AssistantMessageID,
		input.QueryHash, input.QueryText, embedding, input.QueryEmbeddingState,
		input.ActiveRequested).Scan(
		&observationID,
		&mode,
		&prepared.Summary.ProfileID,
		&generation,
		&prepared.Summary.Status,
		&prepared.Summary.ResultCode,
		&prepared.Replayed,
		&prepared.Summary.ExactCount,
		&prepared.Summary.BM25Count,
		&prepared.Summary.VectorCount,
		&prepared.Summary.RRFCount,
		&prepared.Summary.FallbackCode,
		&candidates,
	)
	if err != nil {
		return L2ScenePreparation{}, fmt.Errorf("prepare L2 Scene search: %w", err)
	}
	prepared.Summary.Mode = mode
	if !uuidRE.MatchString(observationID) || generation < 1 ||
		prepared.Summary.ProfileID != L2SceneProfileID ||
		mode != "shadow" && mode != "active" {
		return L2ScenePreparation{}, fmt.Errorf("prepare L2 Scene search: invalid response authority")
	}
	if err := json.Unmarshal(candidates, &prepared.Candidates); err != nil {
		return L2ScenePreparation{}, fmt.Errorf("decode L2 Scene candidates: %w", err)
	}
	if len(prepared.Candidates) > L2SceneCandidateLimit {
		return L2ScenePreparation{}, fmt.Errorf("decode L2 Scene candidates: count exceeded")
	}
	for _, candidate := range prepared.Candidates {
		if !uuidRE.MatchString(candidate.SceneID) || candidate.Revision < 1 ||
			candidate.ScopeType != "global" && candidate.ScopeType != "project" ||
			strings.TrimSpace(candidate.Content) == "" ||
			len([]rune(candidate.Content)) > L2SceneMaximumChars {
			return L2ScenePreparation{}, fmt.Errorf("decode L2 Scene candidates: invalid candidate")
		}
	}
	return prepared, nil
}

func (r *PostgresRepository) RecordL2SceneSearch(
	ctx context.Context,
	input L2SceneRecordInput,
) (L2SceneSearchResult, error) {
	if err := r.requireDB(); err != nil {
		return L2SceneSearchResult{}, err
	}
	reranked, err := json.Marshal(input.Reranked)
	if err != nil {
		return L2SceneSearchResult{}, fmt.Errorf("encode L2 Scene rerank results: %w", err)
	}
	final, err := json.Marshal(input.Final)
	if err != nil {
		return L2SceneSearchResult{}, fmt.Errorf("encode L2 Scene final results: %w", err)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID, mode string
	var generation int64
	var scenes []byte
	var result L2SceneSearchResult
	err = r.db.QueryRowContext(ctx, `
SELECT observation_id::text, mode, profile_id, generation, status,
       result_code, rerank_count, final_count, injected_count,
       estimated_tokens, fallback_code, duration_millis, final_scenes
FROM memory_record_l2_scene_search(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6::jsonb, $7::jsonb, $8, $9
)
`, input.ObservationID, user.ID, input.AssistantMessageID,
		input.RerankStatus, input.FallbackCode, string(reranked), string(final),
		input.EstimatedTokens, input.DurationMillis).Scan(
		&observationID,
		&mode,
		&result.Summary.ProfileID,
		&generation,
		&result.Summary.Status,
		&result.Summary.ResultCode,
		&result.Summary.RerankCount,
		&result.Summary.FinalCount,
		&result.Summary.InjectedCount,
		&result.Summary.EstimatedTokens,
		&result.Summary.FallbackCode,
		&result.Summary.DurationMillis,
		&scenes,
	)
	if err != nil {
		return L2SceneSearchResult{}, fmt.Errorf("record L2 Scene search: %w", err)
	}
	result.Summary.Mode = mode
	if !uuidRE.MatchString(observationID) || generation < 1 ||
		result.Summary.ProfileID != L2SceneProfileID ||
		mode != "shadow" && mode != "active" {
		return L2SceneSearchResult{}, fmt.Errorf("record L2 Scene search: invalid response authority")
	}
	if err := json.Unmarshal(scenes, &result.Scenes); err != nil {
		return L2SceneSearchResult{}, fmt.Errorf("decode final L2 Scenes: %w", err)
	}
	if len(result.Scenes) > L2SceneFinalLimit {
		return L2SceneSearchResult{}, fmt.Errorf("decode final L2 Scenes: count exceeded")
	}
	for _, scene := range result.Scenes {
		if !uuidRE.MatchString(scene.SceneID) || scene.Revision < 1 ||
			scene.ScopeType != "global" && scene.ScopeType != "project" ||
			strings.TrimSpace(scene.Content) == "" ||
			len([]rune(scene.Content)) > L2SceneMaximumChars {
			return L2SceneSearchResult{}, fmt.Errorf("decode final L2 Scenes: invalid Scene")
		}
	}
	return result, nil
}

var _ L2SceneRepository = (*PostgresRepository)(nil)
