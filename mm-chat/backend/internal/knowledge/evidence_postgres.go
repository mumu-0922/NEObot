package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxEvidenceHydrationReferences   = 16
	maxEvidenceSelectedCollections   = 32
	defaultEvidenceCandidateLimit    = 8
	maxEvidenceCandidateLimit        = 50
	maxEvidenceQueryBytes            = 2048
	evidenceQueryEmbeddingDimensions = 1024
)

var (
	ErrEvidenceHydrationRejected = errors.New("evidence hydration rejected")
	evidenceHashPattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type EvidenceCandidateReference struct {
	CollectionID      string  `json:"collection_id"`
	DocumentID        string  `json:"document_id"`
	DocumentVersionID string  `json:"document_version_id"`
	IndexGenerationID string  `json:"index_generation_id"`
	MaterializationID string  `json:"materialization_id"`
	ParentChunkID     string  `json:"parent_chunk_id"`
	ChildChunkID      string  `json:"child_chunk_id"`
	SourceSpanHash    string  `json:"source_span_hash"`
	ContentHash       string  `json:"content_hash"`
	RankScore         float64 `json:"-"`
}

type QueryEvidenceCandidatesInput struct {
	CollectionIDs []string
	QueryText     string
	Limit         int
}

type HybridQueryEvidenceCandidatesInput struct {
	CollectionIDs  []string
	QueryText      string
	QueryEmbedding []float32
	Limit          int
}

type ReauthorizeEvidenceInput struct {
	ActorUserID           string
	SessionID             string
	ConversationID        string
	SelectedCollectionIDs []string
	References            []EvidenceCandidateReference
}

type HydratedEvidence struct {
	CollectionID      string          `json:"collectionId"`
	DocumentID        string          `json:"documentId"`
	DocumentVersionID string          `json:"documentVersionId"`
	IndexGenerationID string          `json:"indexGenerationId"`
	MaterializationID string          `json:"materializationId"`
	ParentChunkID     string          `json:"parentChunkId"`
	ChildChunkID      string          `json:"childChunkId"`
	SourceSpanHash    string          `json:"sourceSpanHash"`
	ContentHash       string          `json:"contentHash"`
	SourceText        string          `json:"sourceText"`
	Locator           json.RawMessage `json:"locator"`
	RankScore         float64         `json:"rankScore"`
}

func (r *PostgresRepository) FetchQueryEvidenceCandidates(
	ctx context.Context,
	input QueryEvidenceCandidatesInput,
) ([]EvidenceCandidateReference, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	input, err := normalizeQueryEvidenceCandidatesInput(input)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT collection_id, document_id, document_version_id, index_generation_id,
  materialization_id, parent_chunk_id, child_chunk_id, source_span_hash,
  content_hash, rank_score
FROM knowledge_fetch_query_evidence_candidates($1::uuid[], $2, $3)
`, evidenceUUIDArrayLiteral(input.CollectionIDs), input.QueryText, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("fetch query evidence candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]EvidenceCandidateReference, 0, input.Limit)
	for rows.Next() {
		var candidate EvidenceCandidateReference
		if err := rows.Scan(
			&candidate.CollectionID,
			&candidate.DocumentID,
			&candidate.DocumentVersionID,
			&candidate.IndexGenerationID,
			&candidate.MaterializationID,
			&candidate.ParentChunkID,
			&candidate.ChildChunkID,
			&candidate.SourceSpanHash,
			&candidate.ContentHash,
			&candidate.RankScore,
		); err != nil {
			return nil, fmt.Errorf("scan query evidence candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate query evidence candidates: %w", err)
	}
	return candidates, nil
}

func (r *PostgresRepository) FetchHybridQueryEvidenceCandidates(
	ctx context.Context,
	input HybridQueryEvidenceCandidatesInput,
) ([]EvidenceCandidateReference, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	input, err := normalizeHybridQueryEvidenceCandidatesInput(input)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT collection_id, document_id, document_version_id, index_generation_id,
  materialization_id, parent_chunk_id, child_chunk_id, source_span_hash,
  content_hash, rank_score
FROM knowledge_fetch_profiled_query_evidence_candidates(
  $1::uuid[], $2, $3::real[], $4
)
`, evidenceUUIDArrayLiteral(input.CollectionIDs), input.QueryText,
		evidenceRealArrayLiteral(input.QueryEmbedding), input.Limit)
	if err != nil {
		return nil, fmt.Errorf("fetch hybrid query evidence candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]EvidenceCandidateReference, 0, input.Limit)
	for rows.Next() {
		var candidate EvidenceCandidateReference
		if err := rows.Scan(
			&candidate.CollectionID,
			&candidate.DocumentID,
			&candidate.DocumentVersionID,
			&candidate.IndexGenerationID,
			&candidate.MaterializationID,
			&candidate.ParentChunkID,
			&candidate.ChildChunkID,
			&candidate.SourceSpanHash,
			&candidate.ContentHash,
			&candidate.RankScore,
		); err != nil {
			return nil, fmt.Errorf("scan hybrid query evidence candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hybrid query evidence candidates: %w", err)
	}
	return candidates, nil
}

func (r *PostgresRepository) ReauthorizeAndHydrateEvidence(
	ctx context.Context,
	input ReauthorizeEvidenceInput,
) ([]HydratedEvidence, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	input, err := normalizeReauthorizeEvidenceInput(input)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(input.References)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence references: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT collection_id, document_id, document_version_id, index_generation_id,
  materialization_id, parent_chunk_id, child_chunk_id, source_span_hash,
  content_hash, source_text, locator
FROM knowledge_reauthorize_and_hydrate_evidence($1, $2, $3, $4::jsonb)
`, input.ActorUserID, input.SessionID, input.ConversationID, string(payload))
	if err != nil {
		if isEvidenceAuthorizationError(err) {
			return nil, ErrEvidenceHydrationRejected
		}
		return nil, fmt.Errorf("hydrate evidence: %w", err)
	}
	defer rows.Close()

	byKey := make(map[string]HydratedEvidence, len(input.References))
	for rows.Next() {
		var evidence HydratedEvidence
		var locator []byte
		if err := rows.Scan(
			&evidence.CollectionID,
			&evidence.DocumentID,
			&evidence.DocumentVersionID,
			&evidence.IndexGenerationID,
			&evidence.MaterializationID,
			&evidence.ParentChunkID,
			&evidence.ChildChunkID,
			&evidence.SourceSpanHash,
			&evidence.ContentHash,
			&evidence.SourceText,
			&locator,
		); err != nil {
			return nil, fmt.Errorf("scan evidence hydration: %w", err)
		}
		evidence.Locator = append(json.RawMessage(nil), locator...)
		byKey[evidence.referenceKey()] = evidence
	}
	if err := rows.Err(); err != nil {
		if isEvidenceAuthorizationError(err) {
			return nil, ErrEvidenceHydrationRejected
		}
		return nil, fmt.Errorf("iterate evidence hydration: %w", err)
	}
	if len(byKey) != len(input.References) {
		return nil, ErrEvidenceHydrationRejected
	}

	hydrated := make([]HydratedEvidence, 0, len(input.References))
	for _, reference := range input.References {
		evidence, ok := byKey[reference.referenceKey()]
		if !ok || evidence.SourceText == "" || !json.Valid(evidence.Locator) {
			return nil, ErrEvidenceHydrationRejected
		}
		evidence.RankScore = reference.RankScore
		hydrated = append(hydrated, evidence)
	}
	return hydrated, nil
}

func normalizeQueryEvidenceCandidatesInput(
	input QueryEvidenceCandidatesInput,
) (QueryEvidenceCandidatesInput, error) {
	if len(input.CollectionIDs) < 1 || len(input.CollectionIDs) > maxEvidenceSelectedCollections {
		return input, ErrEvidenceHydrationRejected
	}
	seen := make(map[string]struct{}, len(input.CollectionIDs))
	normalizedCollectionIDs := make([]string, 0, len(input.CollectionIDs))
	for _, collectionID := range input.CollectionIDs {
		normalized, err := normalizeEvidenceUUID(collectionID)
		if err != nil {
			return input, err
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		normalizedCollectionIDs = append(normalizedCollectionIDs, normalized)
	}
	if len(normalizedCollectionIDs) == 0 {
		return input, ErrEvidenceHydrationRejected
	}
	input.CollectionIDs = normalizedCollectionIDs
	input.QueryText = strings.TrimSpace(input.QueryText)
	if input.QueryText == "" || len(input.QueryText) > maxEvidenceQueryBytes {
		return input, ErrEvidenceHydrationRejected
	}
	if input.Limit == 0 {
		input.Limit = defaultEvidenceCandidateLimit
	}
	if input.Limit < 1 || input.Limit > maxEvidenceCandidateLimit {
		return input, ErrEvidenceHydrationRejected
	}
	return input, nil
}

func normalizeHybridQueryEvidenceCandidatesInput(
	input HybridQueryEvidenceCandidatesInput,
) (HybridQueryEvidenceCandidatesInput, error) {
	normalized, err := normalizeQueryEvidenceCandidatesInput(QueryEvidenceCandidatesInput{
		CollectionIDs: input.CollectionIDs,
		QueryText:     input.QueryText,
		Limit:         input.Limit,
	})
	if err != nil {
		return input, err
	}
	if len(input.QueryEmbedding) != evidenceQueryEmbeddingDimensions {
		return input, ErrEvidenceHydrationRejected
	}
	normSquared := 0.0
	for _, component := range input.QueryEmbedding {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return input, ErrEvidenceHydrationRejected
		}
		normSquared += value * value
	}
	if normSquared <= 0 || math.IsInf(normSquared, 0) {
		return input, ErrEvidenceHydrationRejected
	}
	input.CollectionIDs = normalized.CollectionIDs
	input.QueryText = normalized.QueryText
	input.Limit = normalized.Limit
	input.QueryEmbedding = append([]float32(nil), input.QueryEmbedding...)
	return input, nil
}

func evidenceUUIDArrayLiteral(ids []string) string {
	if len(ids) == 0 {
		return "{}"
	}
	return "{" + strings.Join(ids, ",") + "}"
}

func evidenceRealArrayLiteral(values []float32) string {
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

func normalizeReauthorizeEvidenceInput(input ReauthorizeEvidenceInput) (ReauthorizeEvidenceInput, error) {
	var err error
	input.ActorUserID, err = normalizeEvidenceUUID(input.ActorUserID)
	if err != nil {
		return input, err
	}
	input.SessionID, err = normalizeEvidenceUUID(input.SessionID)
	if err != nil {
		return input, err
	}
	input.ConversationID, err = normalizeEvidenceUUID(input.ConversationID)
	if err != nil {
		return input, err
	}
	if len(input.SelectedCollectionIDs) < 1 || len(input.SelectedCollectionIDs) > maxEvidenceSelectedCollections {
		return input, ErrEvidenceHydrationRejected
	}
	selected := make(map[string]struct{}, len(input.SelectedCollectionIDs))
	for index, collectionID := range input.SelectedCollectionIDs {
		normalized, err := normalizeEvidenceUUID(collectionID)
		if err != nil {
			return input, err
		}
		input.SelectedCollectionIDs[index] = normalized
		selected[normalized] = struct{}{}
	}
	if len(input.References) < 1 || len(input.References) > maxEvidenceHydrationReferences {
		return input, ErrEvidenceHydrationRejected
	}
	seen := make(map[string]struct{}, len(input.References))
	for index, reference := range input.References {
		reference, err = normalizeEvidenceReference(reference)
		if err != nil {
			return input, err
		}
		if _, ok := selected[reference.CollectionID]; !ok {
			return input, ErrEvidenceHydrationRejected
		}
		key := reference.referenceKey()
		if _, ok := seen[key]; ok {
			return input, ErrEvidenceHydrationRejected
		}
		seen[key] = struct{}{}
		input.References[index] = reference
	}
	return input, nil
}

func normalizeEvidenceReference(reference EvidenceCandidateReference) (EvidenceCandidateReference, error) {
	fields := []*string{
		&reference.CollectionID,
		&reference.DocumentID,
		&reference.DocumentVersionID,
		&reference.IndexGenerationID,
		&reference.MaterializationID,
		&reference.ParentChunkID,
		&reference.ChildChunkID,
	}
	for _, field := range fields {
		normalized, err := normalizeEvidenceUUID(*field)
		if err != nil {
			return reference, err
		}
		*field = normalized
	}
	reference.SourceSpanHash = strings.ToLower(strings.TrimSpace(reference.SourceSpanHash))
	reference.ContentHash = strings.ToLower(strings.TrimSpace(reference.ContentHash))
	if !evidenceHashPattern.MatchString(reference.SourceSpanHash) ||
		!evidenceHashPattern.MatchString(reference.ContentHash) ||
		math.IsNaN(reference.RankScore) || math.IsInf(reference.RankScore, 0) ||
		reference.RankScore < 0 {
		return reference, ErrEvidenceHydrationRejected
	}
	return reference, nil
}

func normalizeEvidenceUUID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "00000000-0000-0000-0000-000000000000" || !isUUID(value) {
		return "", ErrEvidenceHydrationRejected
	}
	return value, nil
}

func (reference EvidenceCandidateReference) referenceKey() string {
	parts := []string{
		reference.CollectionID,
		reference.DocumentID,
		reference.DocumentVersionID,
		reference.IndexGenerationID,
		reference.MaterializationID,
		reference.ParentChunkID,
		reference.ChildChunkID,
		reference.SourceSpanHash,
		reference.ContentHash,
	}
	return strings.Join(parts, "|")
}

func (evidence HydratedEvidence) referenceKey() string {
	parts := []string{
		evidence.CollectionID,
		evidence.DocumentID,
		evidence.DocumentVersionID,
		evidence.IndexGenerationID,
		evidence.MaterializationID,
		evidence.ParentChunkID,
		evidence.ChildChunkID,
		evidence.SourceSpanHash,
		evidence.ContentHash,
	}
	return strings.Join(parts, "|")
}

func isEvidenceAuthorizationError(err error) bool {
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42501" || pgErr.Message == "RAG_HYDRATION_NOT_AUTHORIZED"
	}
	return false
}
