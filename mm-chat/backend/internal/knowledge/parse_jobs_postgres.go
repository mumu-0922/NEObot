package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const ragProcessingMaxAttempts = 3

type parseJobInsert struct {
	JobID, CollectionID, DocumentID, VersionID, FileID string
	Operation, RequestedByUserID                       string
	IdempotencyScope, IdempotencyKey, RequestHash      string
	SourceContentHash                                  string
	DocumentVisibilityEpoch                            int64
	CausedByJobID                                      string
	MaterializationID                                  string
	Authority                                          parseAuthority
	Collection                                         collectionRow
}

type parseMaterializationBinding struct {
	IndexGenerationID string
	MaterializationID string
	LegacyUnbound     bool
	MaxAttempts       int
}

func insertParseProcessingJob(
	ctx context.Context,
	tx *sql.Tx,
	input parseJobInsert,
) (parseMaterializationBinding, error) {
	binding, err := allocateParseMaterialization(ctx, tx, input)
	if err != nil {
		return parseMaterializationBinding{}, err
	}
	causedByJobID := nullableString(input.CausedByJobID)
	if input.Authority.LegacyProjectionUnbound {
		_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage, operation,
  processor, endpoint_id, model_id, governance_profile_id, governance_revision,
  governance_head_revision, collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, caused_by_job_id, idempotency_scope, idempotency_key,
  request_hash, max_attempts, index_generation_id, materialization_id,
  legacy_projection_unbound
) VALUES (
  $1,$2,$3,$4,$5,'parse',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
  $19,$20,$21,$22,$23,$24,$25,$26,$27
)
`, input.JobID, input.CollectionID, input.DocumentID, input.VersionID, input.FileID,
			input.Operation, input.Authority.Processor, input.Authority.EndpointID,
			input.Authority.ModelID, input.Authority.ProfileID,
			input.Authority.GovernanceRevision, input.Authority.HeadRevision,
			input.Authority.ConsentID, input.Authority.ConsentRevision,
			input.Collection.ACLRevision, input.Collection.VisibilityEpoch,
			input.Collection.ProcessingRevision, input.DocumentVisibilityEpoch,
			input.RequestedByUserID, causedByJobID, input.IdempotencyScope,
			input.IdempotencyKey, input.RequestHash, binding.MaxAttempts,
			nullableString(binding.IndexGenerationID),
			nullableString(binding.MaterializationID), binding.LegacyUnbound)
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage, operation,
  processor, endpoint_id, governance_profile_id, governance_revision,
  governance_head_revision, collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, caused_by_job_id, idempotency_scope, idempotency_key,
  request_hash
) VALUES (
  $1,$2,$3,$4,$5,'parse',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
  $19,$20,$21,$22
)
`, input.JobID, input.CollectionID, input.DocumentID, input.VersionID, input.FileID,
			input.Operation, input.Authority.Processor, input.Authority.EndpointID,
			input.Authority.ProfileID, input.Authority.GovernanceRevision,
			input.Authority.HeadRevision, input.Authority.ConsentID,
			input.Authority.ConsentRevision, input.Collection.ACLRevision,
			input.Collection.VisibilityEpoch, input.Collection.ProcessingRevision,
			input.DocumentVisibilityEpoch, input.RequestedByUserID, causedByJobID,
			input.IdempotencyScope, input.IdempotencyKey, input.RequestHash)
	}
	if err != nil {
		return parseMaterializationBinding{}, err
	}
	return binding, nil
}

func allocateParseMaterialization(
	ctx context.Context,
	tx *sql.Tx,
	input parseJobInsert,
) (parseMaterializationBinding, error) {
	if !input.Authority.LegacyProjectionUnbound {
		return parseMaterializationBinding{LegacyUnbound: false, MaxAttempts: 8}, nil
	}
	var generationID, baseProfileHash string
	err := tx.QueryRowContext(ctx, `
SELECT generation.id, profile.base_profile_hash
FROM knowledge_corpus_projection_head head
JOIN knowledge_index_generations generation
  ON generation.id = head.active_index_generation_id
  AND generation.status = 'active'
JOIN knowledge_index_profiles profile ON profile.id = generation.index_profile_id
WHERE head.singleton_id = 1
FOR UPDATE OF head
`).Scan(&generationID, &baseProfileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return parseMaterializationBinding{LegacyUnbound: true, MaxAttempts: 8}, nil
	}
	if err != nil {
		return parseMaterializationBinding{}, fmt.Errorf("resolve active RAG generation: %w", err)
	}
	var materializationSeq int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(max(materialization_seq), 0) + 1
FROM knowledge_document_materializations
WHERE index_generation_id = $1 AND document_id = $2
`, generationID, input.DocumentID).Scan(&materializationSeq); err != nil {
		return parseMaterializationBinding{}, fmt.Errorf("allocate materialization sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'staging'
)
`, input.MaterializationID, generationID, input.CollectionID, input.DocumentID,
		input.VersionID, input.FileID, materializationSeq, input.SourceContentHash,
		baseProfileHash, input.Collection.ACLRevision, input.Collection.VisibilityEpoch,
		input.Collection.ProcessingRevision, input.DocumentVisibilityEpoch); err != nil {
		return parseMaterializationBinding{}, fmt.Errorf("insert document materialization: %w", err)
	}
	return parseMaterializationBinding{
		IndexGenerationID: generationID,
		MaterializationID: input.MaterializationID,
		LegacyUnbound:     false,
		MaxAttempts:       ragProcessingMaxAttempts,
	}, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
