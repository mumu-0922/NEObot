package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (r *PostgresRepository) CreateDocument(ctx context.Context, input CreateDocumentRepositoryInput) (Document, error) {
	if err := r.requireDB(); err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, fmt.Errorf("begin bind document: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	collection, _, err := lockCollectionForManage(ctx, tx, input.CollectionID, input.ActorUserID)
	if err != nil {
		return Document{}, err
	}
	if existing, found, err := findDocumentReplay(ctx, tx, input); err != nil {
		return Document{}, err
	} else if found {
		return existing, nil
	}

	var file DocumentFile
	var hash string
	var purpose sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT id, original_filename, mime_type, byte_size, sha256, metadata->>'purpose'
FROM files
WHERE id = $1 AND user_id = $2 AND upload_status = 'available' AND deleted_at IS NULL
FOR UPDATE
`, input.FileID, input.ActorUserID).Scan(&file.ID, &file.Name, &file.MIMEType, &file.ByteSize, &hash, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrFileNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("lock knowledge file: %w", err)
	}
	if !purpose.Valid || purpose.String != "knowledge" {
		return Document{}, ErrFileNotFound
	}

	var consentID, endpointID, profileID string
	var governanceRevision, headRevision, consentRevision int64
	err = tx.QueryRowContext(ctx, `
SELECT pc.id, pc.endpoint_id, pc.governance_profile_id, pc.governance_revision,
  pc.governance_head_revision, pc.consent_revision
FROM processing_consents pc
JOIN processor_governance_heads h
  ON h.processor = pc.processor AND h.endpoint_id = pc.endpoint_id
JOIN processor_governance_profiles p ON p.id = pc.governance_profile_id
WHERE pc.scope = 'collection' AND pc.collection_id = $1 AND pc.processor = $2
  AND pc.superseded_at IS NULL AND pc.decision = 'granted'
  AND (pc.expires_at IS NULL OR pc.expires_at > now())
  AND 'parse' = ANY(pc.purposes)
  AND ($3 = ANY(pc.data_types) OR '*' = ANY(pc.data_types))
  AND h.status = 'active' AND h.active_profile_id = pc.governance_profile_id
  AND h.active_governance_revision = pc.governance_revision
  AND h.head_revision = pc.governance_head_revision AND p.status = 'approved'
FOR UPDATE OF pc, h
`, input.CollectionID, input.ParseProcessor, file.MIMEType).Scan(
		&consentID, &endpointID, &profileID, &governanceRevision, &headRevision, &consentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrProcessingConsent
	}
	if err != nil {
		return Document{}, fmt.Errorf("resolve parse consent: %w", err)
	}

	var document Document
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_documents (
  id, collection_id, created_by_user_id, idempotency_key, create_request_hash
) VALUES ($1, $2, $3, $4, $5)
RETURNING id, collection_id, status, created_at, updated_at
`, input.DocumentID, input.CollectionID, input.ActorUserID, input.IdempotencyKey, input.RequestHash).
		Scan(&document.ID, &document.CollectionID, &document.Status, &document.CreatedAt, &document.UpdatedAt)
	if err != nil {
		return Document{}, mapDocumentInsertError(err)
	}
	version := DocumentVersion{File: file}
	err = tx.QueryRowContext(ctx, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, content_hash,
  created_by_user_id, idempotency_key, request_hash
) VALUES ($1, $2, $3, 1, $4, $5, $6, $7)
RETURNING id, source_version, status, created_at, updated_at
`, input.VersionID, input.DocumentID, input.FileID, hash, input.ActorUserID,
		input.IdempotencyKey, input.RequestHash).Scan(
		&version.ID, &version.SourceVersion, &version.Status, &version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		return Document{}, fmt.Errorf("insert document version: %w", err)
	}
	document.PendingVersion = &version
	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage, operation,
  processor, endpoint_id, governance_profile_id, governance_revision,
  governance_head_revision, collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, idempotency_scope, idempotency_key, request_hash
) VALUES (
  $1,$2,$3,$4,$5,'parse','initial',$6,$7,$8,$9,$10,$11,$12,
  $13,$14,$15,1,$16,$17,$18,$19
)
`, input.JobID, input.CollectionID, input.DocumentID, input.VersionID, input.FileID,
		input.ParseProcessor, endpointID, profileID, governanceRevision, headRevision,
		consentID, consentRevision, collection.ACLRevision, collection.VisibilityEpoch,
		collection.ProcessingRevision, input.ActorUserID,
		"document:"+input.DocumentID+":initial", input.IdempotencyKey, input.RequestHash)
	if err != nil {
		return Document{}, fmt.Errorf("insert parse job: %w", err)
	}
	if err := r.insertDocumentOutbox(ctx, tx, document, version, input.JobID, hash); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, fmt.Errorf("commit bind document: %w", err)
	}
	return document, nil
}

func findDocumentReplay(ctx context.Context, tx *sql.Tx, input CreateDocumentRepositoryInput) (Document, bool, error) {
	var document Document
	var storedHash string
	err := tx.QueryRowContext(ctx, `
SELECT id, collection_id, status, create_request_hash, created_at, updated_at
FROM knowledge_documents
WHERE collection_id = $1 AND created_by_user_id = $2 AND idempotency_key = $3
FOR UPDATE
`, input.CollectionID, input.ActorUserID, input.IdempotencyKey).Scan(
		&document.ID, &document.CollectionID, &document.Status, &storedHash,
		&document.CreatedAt, &document.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return document, false, nil
	}
	if err != nil {
		return document, false, fmt.Errorf("check document replay: %w", err)
	}
	if storedHash != input.RequestHash {
		return Document{}, false, ErrIdempotencyConflict
	}
	version, err := queryPendingVersion(ctx, tx, document.ID)
	if err != nil {
		return Document{}, false, err
	}
	document.PendingVersion = &version
	return document, true, nil
}

func queryPendingVersion(ctx context.Context, tx *sql.Tx, documentID string) (DocumentVersion, error) {
	var version DocumentVersion
	err := tx.QueryRowContext(ctx, `
SELECT v.id, v.source_version, v.status, COALESCE(v.error_code,''),
  f.id, f.original_filename, f.mime_type, f.byte_size, v.created_at, v.updated_at
FROM knowledge_document_versions v JOIN files f ON f.id = v.file_id
WHERE v.document_id = $1 AND v.status IN ('uploaded','processing','failed')
ORDER BY v.source_version DESC LIMIT 1
`, documentID).Scan(&version.ID, &version.SourceVersion, &version.Status, &version.ErrorCode,
		&version.File.ID, &version.File.Name, &version.File.MIMEType, &version.File.ByteSize,
		&version.CreatedAt, &version.UpdatedAt)
	if err != nil {
		return version, fmt.Errorf("query pending version: %w", err)
	}
	return version, nil
}

func (r *PostgresRepository) insertDocumentOutbox(ctx context.Context, tx *sql.Tx, document Document, version DocumentVersion, jobID, hash string) error {
	eventID, err := r.newEventID()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"schemaVersion": 1, "collectionId": document.CollectionID,
		"documentId": document.ID, "documentVersionId": version.ID, "sourceVersion": 1,
		"fileId": version.File.ID, "contentHash": hash, "jobId": jobID})
	_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_outbox
(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_document',$2,'knowledge.document.version.requested',$3::jsonb)`, eventID, document.ID, string(payload))
	if err != nil {
		return fmt.Errorf("insert document outbox: %w", err)
	}
	return nil
}

func mapDocumentInsertError(err error) error {
	if isConstraint(err, "idx_knowledge_documents_collection_creator_idempotency") {
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("insert document: %w", err)
}
