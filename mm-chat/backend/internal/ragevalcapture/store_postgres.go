package ragevalcapture

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	captureEmbeddingDimensions = 1024
	maximumCaptureCollections  = 32
	maximumCaptureCandidates   = 50
	maximumCaptureHydration    = 16
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (store *PostgresStore) Status(ctx context.Context) (GenerationStatus, error) {
	if store == nil || store.db == nil {
		return GenerationStatus{}, errors.New("evaluation operator database is required")
	}
	var result GenerationStatus
	var candidateID, candidateStatus, candidateChunkHash sql.NullString
	var candidateManifest, candidateReadiness sql.NullString
	var candidateSequence sql.NullInt64
	err := store.db.QueryRowContext(ctx, `
SELECT head_revision, corpus_projection_revision, active_generation_id,
  active_generation_seq, active_chunk_profile_hash,
  active_artifact_manifest_hash, candidate_generation_id,
  candidate_generation_seq, candidate_status, candidate_chunk_profile_hash,
  candidate_artifact_manifest_hash, candidate_readiness
FROM knowledge_structure_generation_operator_status()
`).Scan(
		&result.HeadRevision,
		&result.CorpusProjectionRevision,
		&result.ActiveGenerationID,
		&result.ActiveGenerationSequence,
		&result.ActiveChunkProfileHash,
		&result.ActiveArtifactManifestHash,
		&candidateID,
		&candidateSequence,
		&candidateStatus,
		&candidateChunkHash,
		&candidateManifest,
		&candidateReadiness,
	)
	if err != nil {
		return GenerationStatus{}, fmt.Errorf("read generation status: %w", err)
	}
	result.CandidateGenerationID = candidateID.String
	result.CandidateGenerationSequence = candidateSequence.Int64
	result.CandidateStatus = candidateStatus.String
	result.CandidateChunkProfileHash = candidateChunkHash.String
	result.CandidateArtifactManifestHash = candidateManifest.String
	result.CandidateReadiness = candidateReadiness.String
	return result, nil
}

func (store *PostgresStore) FetchCandidates(
	ctx context.Context,
	generationID string,
	collectionIDs []string,
	query string,
	embedding []float32,
	limit int,
) ([]CandidateReference, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("evaluation operator database is required")
	}
	if err := validateCaptureQuery(
		generationID,
		collectionIDs,
		query,
		embedding,
		limit,
	); err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT collection_id, document_id, document_version_id, index_generation_id,
  materialization_id, parent_chunk_id, child_chunk_id, source_span_hash,
  content_hash, rank_score
FROM knowledge_fetch_generation_evaluation_candidates(
  $1, $2::uuid[], $3, $4::real[], $5
)
`, generationID, captureUUIDArrayLiteral(collectionIDs), strings.TrimSpace(query),
		captureRealArrayLiteral(embedding), limit)
	if err != nil {
		return nil, fmt.Errorf("fetch generation evaluation candidates: %w", err)
	}
	defer rows.Close()
	result := make([]CandidateReference, 0, limit)
	for rows.Next() {
		var item CandidateReference
		if err := rows.Scan(
			&item.CollectionID,
			&item.DocumentID,
			&item.DocumentVersionID,
			&item.IndexGenerationID,
			&item.MaterializationID,
			&item.ParentChunkID,
			&item.ChildChunkID,
			&item.SourceSpanHash,
			&item.ContentHash,
			&item.RankScore,
		); err != nil {
			return nil, fmt.Errorf("scan generation evaluation candidate: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation evaluation candidates: %w", err)
	}
	return result, nil
}

func (store *PostgresStore) Hydrate(
	ctx context.Context,
	generationID string,
	collectionIDs []string,
	references []CandidateReference,
) ([]HydratedEvidence, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("evaluation operator database is required")
	}
	if !validCaptureUUID(generationID) || len(collectionIDs) < 1 ||
		len(collectionIDs) > maximumCaptureCollections || len(references) < 1 ||
		len(references) > maximumCaptureHydration {
		return nil, errors.New("generation evaluation hydration input is invalid")
	}
	for _, collectionID := range collectionIDs {
		if !validCaptureUUID(collectionID) {
			return nil, errors.New("generation evaluation hydration input is invalid")
		}
	}
	for _, reference := range references {
		if err := validateCandidateReference(reference, generationID); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(references)
	if err != nil {
		return nil, fmt.Errorf("marshal generation evaluation references: %w", err)
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT collection_id, document_id, document_version_id, index_generation_id,
  materialization_id, parent_chunk_id, child_chunk_id, source_span_hash,
  content_hash, source_name, source_text, child_token_count, parent_source_text,
  parent_token_count, locator, provenance_valid, cell_lineage_valid
FROM knowledge_hydrate_generation_evaluation_evidence(
  $1, $2::uuid[], $3::jsonb
)
`, generationID, captureUUIDArrayLiteral(collectionIDs), string(payload))
	if err != nil {
		return nil, fmt.Errorf("hydrate generation evaluation evidence: %w", err)
	}
	defer rows.Close()
	byKey := make(map[string]HydratedEvidence, len(references))
	for rows.Next() {
		var item HydratedEvidence
		var locator []byte
		if err := rows.Scan(
			&item.CollectionID,
			&item.DocumentID,
			&item.DocumentVersionID,
			&item.IndexGenerationID,
			&item.MaterializationID,
			&item.ParentChunkID,
			&item.ChildChunkID,
			&item.SourceSpanHash,
			&item.ContentHash,
			&item.SourceName,
			&item.SourceText,
			&item.ChildTokenCount,
			&item.ParentSourceText,
			&item.ParentTokenCount,
			&locator,
			&item.ProvenanceValid,
			&item.CellLineageValid,
		); err != nil {
			return nil, fmt.Errorf("scan generation evaluation evidence: %w", err)
		}
		item.Locator = append(json.RawMessage(nil), locator...)
		byKey[captureReferenceKey(item.CandidateReference)] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation evaluation evidence: %w", err)
	}
	if len(byKey) != len(references) {
		return nil, errors.New("generation evaluation hydration was incomplete")
	}
	result := make([]HydratedEvidence, 0, len(references))
	for _, reference := range references {
		item, ok := byKey[captureReferenceKey(reference)]
		if !ok || !validCaptureSourceName(item.SourceName) ||
			strings.TrimSpace(item.SourceText) == "" ||
			item.ChildTokenCount <= 0 || strings.TrimSpace(item.ParentSourceText) == "" ||
			item.ParentTokenCount <= 0 || !json.Valid(item.Locator) ||
			!item.ProvenanceValid {
			return nil, errors.New("generation evaluation hydration was rejected")
		}
		item.RankScore = reference.RankScore
		result = append(result, item)
	}
	return result, nil
}

func validCaptureSourceName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]byte(value)) <= 512 &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func validateCaptureQuery(
	generationID string,
	collectionIDs []string,
	query string,
	embedding []float32,
	limit int,
) error {
	if !validCaptureUUID(generationID) || len(collectionIDs) < 1 ||
		len(collectionIDs) > maximumCaptureCollections ||
		strings.TrimSpace(query) == "" || len([]byte(strings.TrimSpace(query))) > 2048 ||
		len(embedding) != captureEmbeddingDimensions || limit < 1 ||
		limit > maximumCaptureCandidates {
		return errors.New("generation evaluation query is invalid")
	}
	for _, collectionID := range collectionIDs {
		if !validCaptureUUID(collectionID) {
			return errors.New("generation evaluation query is invalid")
		}
	}
	norm := 0.0
	for _, component := range embedding {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("generation evaluation query is invalid")
		}
		norm += value * value
	}
	if norm <= 0 || math.IsInf(norm, 0) {
		return errors.New("generation evaluation query is invalid")
	}
	return nil
}

func validateCandidateReference(reference CandidateReference, generationID string) error {
	for _, value := range []string{
		reference.CollectionID,
		reference.DocumentID,
		reference.DocumentVersionID,
		reference.IndexGenerationID,
		reference.MaterializationID,
		reference.ParentChunkID,
		reference.ChildChunkID,
	} {
		if !validCaptureUUID(value) {
			return errors.New("generation evaluation reference is invalid")
		}
	}
	if reference.IndexGenerationID != generationID ||
		!validCaptureHash(reference.SourceSpanHash) ||
		!validCaptureHash(reference.ContentHash) || reference.RankScore < 0 ||
		math.IsNaN(reference.RankScore) || math.IsInf(reference.RankScore, 0) {
		return errors.New("generation evaluation reference is invalid")
	}
	return nil
}

func captureUUIDArrayLiteral(values []string) string {
	return "{" + strings.Join(values, ",") + "}"
}

func captureRealArrayLiteral(values []float32) string {
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

func captureReferenceKey(reference CandidateReference) string {
	return strings.Join([]string{
		reference.CollectionID,
		reference.DocumentID,
		reference.DocumentVersionID,
		reference.IndexGenerationID,
		reference.MaterializationID,
		reference.ParentChunkID,
		reference.ChildChunkID,
		reference.SourceSpanHash,
		reference.ContentHash,
	}, "|")
}

var _ Store = (*PostgresStore)(nil)
