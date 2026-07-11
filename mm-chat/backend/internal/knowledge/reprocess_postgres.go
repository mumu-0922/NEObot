package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (r *PostgresRepository) ReprocessDocument(ctx context.Context, input ReprocessDocumentRepositoryInput) (Document, error) {
	if err := r.requireDB(); err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, fmt.Errorf("begin reprocess document: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var collectionID string
	err = tx.QueryRowContext(ctx, `SELECT collection_id FROM knowledge_documents WHERE id=$1`, input.DocumentID).
		Scan(&collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("resolve reprocess collection: %w", err)
	}
	collection, _, err := lockCollectionForManage(ctx, tx, collectionID, input.ActorUserID)
	if errors.Is(err, ErrCollectionNotFound) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, err
	}

	var document Document
	var currentVersionID sql.NullString
	var visibilityEpoch int64
	err = tx.QueryRowContext(ctx, `
SELECT id,collection_id,status,current_version_id,visibility_epoch,created_at,updated_at
FROM knowledge_documents
WHERE id=$1 AND collection_id=$2 AND status IN ('processing','active') AND deleted_at IS NULL
FOR UPDATE
`, input.DocumentID, collectionID).Scan(
		&document.ID, &document.CollectionID, &document.Status, &currentVersionID,
		&visibilityEpoch, &document.CreatedAt, &document.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("lock document for reprocess: %w", err)
	}

	if replay, found, replayErr := findReprocessReplay(ctx, tx, input, document, currentVersionID); replayErr != nil {
		return Document{}, replayErr
	} else if found {
		return replay, nil
	}
	if processing, checkErr := hasDocumentProcessingWork(ctx, tx, input.DocumentID); checkErr != nil {
		return Document{}, checkErr
	} else if processing {
		return Document{}, ErrDocumentProcessing
	}

	target, targetWasFailed, err := resolveReprocessTarget(ctx, tx, input.DocumentID, currentVersionID)
	if err != nil {
		return Document{}, err
	}
	if err := lockReprocessFile(ctx, tx, target.File.ID); err != nil {
		return Document{}, err
	}
	var contentHash string
	if err := tx.QueryRowContext(ctx, `
SELECT content_hash FROM knowledge_document_versions WHERE document_id=$1 AND id=$2
`, input.DocumentID, target.ID).Scan(&contentHash); err != nil {
		return Document{}, fmt.Errorf("resolve reprocess content hash: %w", err)
	}
	authority, err := resolveParseAuthority(ctx, tx, collectionID, input.ParseProcessor, target.File.MIMEType)
	if err != nil {
		return Document{}, err
	}

	var causedByJobID string
	if err := tx.QueryRowContext(ctx, `
SELECT id FROM knowledge_processing_jobs
WHERE document_id=$1 AND document_version_id=$2 AND stage='parse'
ORDER BY created_at DESC,id DESC LIMIT 1
`, input.DocumentID, target.ID).Scan(&causedByJobID); err != nil {
		return Document{}, fmt.Errorf("resolve reprocess cause: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
 id,collection_id,document_id,document_version_id,file_id,stage,operation,
 processor,endpoint_id,governance_profile_id,governance_revision,
 governance_head_revision,collection_consent_id,collection_consent_revision,
 collection_acl_revision,collection_visibility_epoch,collection_processing_revision,
 document_visibility_epoch,requested_by_user_id,caused_by_job_id,
 idempotency_scope,idempotency_key,request_hash
) VALUES (
 $1,$2,$3,$4,$5,'parse','reprocess',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
 $16,$17,$18,$19,$20,$21
)
`, input.JobID, collectionID, input.DocumentID, target.ID, target.File.ID,
		input.ParseProcessor, authority.EndpointID, authority.ProfileID,
		authority.GovernanceRevision, authority.HeadRevision, authority.ConsentID,
		authority.ConsentRevision, collection.ACLRevision, collection.VisibilityEpoch,
		collection.ProcessingRevision, visibilityEpoch, input.ActorUserID, causedByJobID,
		documentOperationIdempotencyScope(input.DocumentID, "reprocess", input.ActorUserID),
		input.IdempotencyKey, input.RequestHash)
	if err != nil {
		if isConstraint(err, "knowledge_processing_jobs_idempotency_unique") {
			return Document{}, ErrIdempotencyConflict
		}
		return Document{}, fmt.Errorf("insert reprocess job: %w", err)
	}
	if targetWasFailed {
		if err := tx.QueryRowContext(ctx, `
UPDATE knowledge_document_versions
SET status='uploaded',error_code=NULL,updated_at=now()
WHERE id=$1 AND document_id=$2 AND status='failed'
RETURNING status,updated_at
`, target.ID, input.DocumentID).Scan(&target.Status, &target.UpdatedAt); err != nil {
			return Document{}, fmt.Errorf("reopen failed version for reprocess: %w", err)
		}
		target.ErrorCode = ""
	}
	if err := tx.QueryRowContext(ctx, `
UPDATE knowledge_documents SET updated_at=now() WHERE id=$1 RETURNING updated_at
`, input.DocumentID).Scan(&document.UpdatedAt); err != nil {
		return Document{}, fmt.Errorf("touch document reprocess time: %w", err)
	}
	if currentVersionID.Valid {
		document.CurrentVersion, err = queryVersionByID(ctx, tx, input.DocumentID, currentVersionID.String)
		if err != nil {
			return Document{}, err
		}
	}
	if !currentVersionID.Valid || target.ID != currentVersionID.String {
		document.PendingVersion = target
	}
	if err := r.insertReprocessOutbox(
		ctx, tx, document, *target, contentHash, input.JobID, causedByJobID,
		authority, collection, visibilityEpoch,
	); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, fmt.Errorf("commit reprocess document: %w", err)
	}
	return document, nil
}

func hasDocumentProcessingWork(ctx context.Context, tx *sql.Tx, documentID string) (bool, error) {
	var processing bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
 SELECT 1 FROM knowledge_document_versions
 WHERE document_id=$1 AND status IN ('uploaded','processing')
) OR EXISTS (
 SELECT 1 FROM knowledge_processing_jobs
 WHERE document_id=$1 AND status IN ('pending','processing')
)
`, documentID).Scan(&processing)
	if err != nil {
		return false, fmt.Errorf("check document processing work: %w", err)
	}
	return processing, nil
}

func resolveReprocessTarget(ctx context.Context, tx *sql.Tx, documentID string, currentVersionID sql.NullString) (*DocumentVersion, bool, error) {
	var failedVersionID string
	query := `
SELECT id FROM knowledge_document_versions
WHERE document_id=$1 AND status='failed'
`
	args := []any{documentID}
	if currentVersionID.Valid {
		query += `  AND source_version > (
    SELECT source_version FROM knowledge_document_versions
    WHERE document_id=$1 AND id=$2 AND status='active'
  )
`
		args = append(args, currentVersionID.String)
	}
	query += "ORDER BY source_version DESC LIMIT 1"
	err := tx.QueryRowContext(ctx, query, args...).Scan(&failedVersionID)
	if err == nil {
		version, queryErr := queryVersionByID(ctx, tx, documentID, failedVersionID)
		return version, true, queryErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("resolve failed reprocess target: %w", err)
	}
	if !currentVersionID.Valid {
		return nil, false, ErrDocumentNotFound
	}
	version, err := queryVersionByID(ctx, tx, documentID, currentVersionID.String)
	return version, false, err
}

func lockReprocessFile(ctx context.Context, tx *sql.Tx, fileID string) error {
	var lockedID string
	err := tx.QueryRowContext(ctx, `
SELECT id FROM files WHERE id=$1 AND upload_status='available' AND deleted_at IS NULL FOR UPDATE
`, fileID).Scan(&lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("lock reprocess file: %w", err)
	}
	return nil
}

func findReprocessReplay(
	ctx context.Context,
	tx *sql.Tx,
	input ReprocessDocumentRepositoryInput,
	document Document,
	currentVersionID sql.NullString,
) (Document, bool, error) {
	var targetVersionID, storedHash string
	err := tx.QueryRowContext(ctx, `
SELECT document_version_id,request_hash FROM knowledge_processing_jobs
WHERE idempotency_scope=$1 AND idempotency_key=$2
`, documentOperationIdempotencyScope(input.DocumentID, "reprocess", input.ActorUserID), input.IdempotencyKey).Scan(
		&targetVersionID, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("check reprocess replay: %w", err)
	}
	if storedHash != input.RequestHash {
		return Document{}, false, ErrIdempotencyConflict
	}
	if currentVersionID.Valid {
		document.CurrentVersion, err = queryVersionByID(ctx, tx, input.DocumentID, currentVersionID.String)
		if err != nil {
			return Document{}, false, err
		}
	}
	if !currentVersionID.Valid || targetVersionID != currentVersionID.String {
		document.PendingVersion, err = queryVersionByID(ctx, tx, input.DocumentID, targetVersionID)
		if err != nil {
			return Document{}, false, err
		}
	}
	return document, true, nil
}

func (r *PostgresRepository) insertReprocessOutbox(
	ctx context.Context,
	tx *sql.Tx,
	document Document,
	version DocumentVersion,
	contentHash, jobID, causedByJobID string,
	authority parseAuthority,
	collection collectionRow,
	documentVisibilityEpoch int64,
) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate reprocess outbox event id: %w", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "collectionId": document.CollectionID,
		"documentId": document.ID, "documentVersionId": version.ID,
		"sourceVersion": version.SourceVersion, "fileId": version.File.ID,
		"contentHash": contentHash, "jobId": jobID,
		"causedByJobId": causedByJobID, "operation": "reprocess",
		"processor": authority.Processor, "endpointId": authority.EndpointID,
		"governanceProfileId":          authority.ProfileID,
		"governanceRevision":           authority.GovernanceRevision,
		"governanceHeadRevision":       authority.HeadRevision,
		"collectionConsentId":          authority.ConsentID,
		"collectionConsentRevision":    authority.ConsentRevision,
		"collectionAclRevision":        collection.ACLRevision,
		"collectionVisibilityEpoch":    collection.VisibilityEpoch,
		"collectionProcessingRevision": collection.ProcessingRevision,
		"documentVisibilityEpoch":      documentVisibilityEpoch,
	})
	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_document',$2,'knowledge.document.reprocess.requested',$3::jsonb)
`, eventID, document.ID, string(payload))
	if err != nil {
		return fmt.Errorf("insert reprocess outbox: %w", err)
	}
	return nil
}
