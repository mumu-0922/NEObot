package usermemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) PrepareL3PersonaSearch(
	ctx context.Context,
	input L3PersonaPrepareInput,
) (L3PersonaPreparation, error) {
	if err := r.requireDB(); err != nil {
		return L3PersonaPreparation{}, err
	}
	var embedding any
	if len(input.QueryEmbedding) > 0 {
		if !validHybridVector(input.QueryEmbedding) {
			return L3PersonaPreparation{}, fmt.Errorf("prepare L3 Persona search: invalid query vector")
		}
		embedding = hybridRealArrayLiteral(input.QueryEmbedding)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID, mode string
	var generation int64
	var candidates []byte
	var prepared L3PersonaPreparation
	err := r.db.QueryRowContext(ctx, `
SELECT observation_id::text, mode, profile_id, generation, status,
       result_code, replayed, exact_count, bm25_count, vector_count,
       rrf_count, fallback_code, candidates
FROM memory_prepare_l3_persona_search(
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
		return L3PersonaPreparation{}, fmt.Errorf("prepare L3 Persona search: %w", err)
	}
	prepared.Summary.Mode = mode
	if !uuidRE.MatchString(observationID) || generation < 1 ||
		prepared.Summary.ProfileID != L3PersonaProfileID ||
		mode != "shadow" && mode != "active" {
		return L3PersonaPreparation{}, fmt.Errorf("prepare L3 Persona search: invalid response authority")
	}
	if err := json.Unmarshal(candidates, &prepared.Candidates); err != nil {
		return L3PersonaPreparation{}, fmt.Errorf("decode L3 Persona candidates: %w", err)
	}
	if len(prepared.Candidates) > L3PersonaCandidateLimit {
		return L3PersonaPreparation{}, fmt.Errorf("decode L3 Persona candidates: count exceeded")
	}
	for _, candidate := range prepared.Candidates {
		if !uuidRE.MatchString(candidate.PersonaID) || candidate.Revision < 1 ||
			strings.TrimSpace(candidate.Content) == "" ||
			len([]rune(candidate.Content)) > L3PersonaMaximumChars {
			return L3PersonaPreparation{}, fmt.Errorf("decode L3 Persona candidates: invalid candidate")
		}
	}
	return prepared, nil
}

func (r *PostgresRepository) RecordL3PersonaSearch(
	ctx context.Context,
	input L3PersonaRecordInput,
) (L3PersonaSearchResult, error) {
	if err := r.requireDB(); err != nil {
		return L3PersonaSearchResult{}, err
	}
	reranked, err := json.Marshal(input.Reranked)
	if err != nil {
		return L3PersonaSearchResult{}, fmt.Errorf("encode L3 Persona rerank results: %w", err)
	}
	final, err := json.Marshal(input.Final)
	if err != nil {
		return L3PersonaSearchResult{}, fmt.Errorf("encode L3 Persona final results: %w", err)
	}
	user := auth.UserOrDevelopment(ctx)
	var observationID, mode string
	var generation int64
	var personas []byte
	var result L3PersonaSearchResult
	err = r.db.QueryRowContext(ctx, `
SELECT observation_id::text, mode, profile_id, generation, status,
       result_code, rerank_count, final_count, injected_count,
       estimated_tokens, fallback_code, duration_millis, final_personas
FROM memory_record_l3_persona_search(
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
		&personas,
	)
	if err != nil {
		return L3PersonaSearchResult{}, fmt.Errorf("record L3 Persona search: %w", err)
	}
	result.Summary.Mode = mode
	if !uuidRE.MatchString(observationID) || generation < 1 ||
		result.Summary.ProfileID != L3PersonaProfileID ||
		mode != "shadow" && mode != "active" {
		return L3PersonaSearchResult{}, fmt.Errorf("record L3 Persona search: invalid response authority")
	}
	if err := json.Unmarshal(personas, &result.Personas); err != nil {
		return L3PersonaSearchResult{}, fmt.Errorf("decode final L3 Personas: %w", err)
	}
	if len(result.Personas) > L3PersonaFinalLimit {
		return L3PersonaSearchResult{}, fmt.Errorf("decode final L3 Personas: count exceeded")
	}
	for _, persona := range result.Personas {
		if !uuidRE.MatchString(persona.PersonaID) || persona.Revision < 1 ||
			strings.TrimSpace(persona.Content) == "" ||
			len([]rune(persona.Content)) > L3PersonaMaximumChars {
			return L3PersonaSearchResult{}, fmt.Errorf("decode final L3 Personas: invalid Persona")
		}
	}
	return result, nil
}

var _ L3PersonaRepository = (*PostgresRepository)(nil)
