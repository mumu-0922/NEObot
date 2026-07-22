package knowledge

import (
	"context"
	"database/sql"
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
	var generationID, materializationID sql.NullString
	var legacyUnbound bool
	var maxAttempts int
	err := tx.QueryRowContext(ctx, `
SELECT index_generation_id, materialization_id,
  legacy_projection_unbound, max_attempts
FROM knowledge_allocate_parse_materialization($1, $2, $3)
`, input.MaterializationID, input.DocumentID, input.VersionID).Scan(
		&generationID,
		&materializationID,
		&legacyUnbound,
		&maxAttempts,
	)
	if err != nil {
		return parseMaterializationBinding{}, fmt.Errorf("allocate parse materialization: %w", err)
	}
	return parseMaterializationBinding{
		IndexGenerationID: generationID.String,
		MaterializationID: materializationID.String,
		LegacyUnbound:     legacyUnbound,
		MaxAttempts:       maxAttempts,
	}, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
