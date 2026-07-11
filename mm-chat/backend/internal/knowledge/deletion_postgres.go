package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type tombstoneVersion struct {
	ID, FileID, ContentHash, Status string
	SourceVersion                   int64
	VisibilityEpoch                 int64
	PurgeJobID                      string
}

type cancelledJob struct {
	ID, VersionID string
}

func (r *PostgresRepository) DeleteDocument(ctx context.Context, input DeleteDocumentRepositoryInput) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete document: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var collectionID string
	err = tx.QueryRowContext(ctx, `SELECT collection_id FROM knowledge_documents WHERE id=$1`, input.DocumentID).
		Scan(&collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve deletion collection: %w", err)
	}
	collection, _, err := lockCollectionForManage(ctx, tx, collectionID, input.ActorUserID)
	if errors.Is(err, ErrCollectionNotFound) {
		return ErrDocumentNotFound
	}
	if err != nil {
		return err
	}

	var status string
	var deletedAt sql.NullTime
	var visibilityEpoch int64
	err = tx.QueryRowContext(ctx, `
SELECT status,visibility_epoch,deleted_at FROM knowledge_documents
WHERE id=$1 AND collection_id=$2 FOR UPDATE
`, input.DocumentID, collectionID).Scan(&status, &visibilityEpoch, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("lock document for deletion: %w", err)
	}
	if deletedAt.Valid || status == "tombstoned" || status == "deleted" {
		return nil
	}

	versions, err := lockDocumentVersionsForDeletion(ctx, tx, input.DocumentID)
	if err != nil {
		return err
	}
	cancelled, err := lockCancelableDocumentJobs(ctx, tx, input.DocumentID)
	if err != nil {
		return err
	}
	var now time.Time
	// now() is fixed at transaction start in PostgreSQL. This transaction may
	// wait behind replace/reprocess while holding no Document lock yet, so using
	// now() could move updated_at/completed_at behind a just-committed writer.
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("read document deletion time: %w", err)
	}
	newVisibilityEpoch := visibilityEpoch + 1
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_processing_jobs
SET status='cancelled',lease_owner=NULL,lease_expires_at=NULL,
    completed_at=$2,updated_at=$2
WHERE document_id=$1 AND status IN ('pending','processing')
`, input.DocumentID, now); err != nil {
		return fmt.Errorf("cancel document jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_document_versions
SET status='tombstoned',visibility_epoch=visibility_epoch+1,updated_at=$2
WHERE document_id=$1 AND status NOT IN ('tombstoned','deleted')
`, input.DocumentID, now); err != nil {
		return fmt.Errorf("tombstone document versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_documents
SET status='tombstoned',visibility_epoch=$2,deleted_at=$3,updated_at=$3
WHERE id=$1
`, input.DocumentID, newVisibilityEpoch, now); err != nil {
		return fmt.Errorf("tombstone document: %w", err)
	}

	for index := range versions {
		if versions[index].Status != "tombstoned" && versions[index].Status != "deleted" {
			versions[index].VisibilityEpoch++
			versions[index].Status = "tombstoned"
		}
		versions[index].PurgeJobID, err = r.newEventID()
		if err != nil {
			return fmt.Errorf("generate purge job id: %w", err)
		}
		requestHash := sha256.Sum256([]byte(
			input.DocumentID + "\n" + versions[index].ID + "\n" + fmt.Sprint(newVisibilityEpoch),
		))
		_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
 id,collection_id,document_id,document_version_id,file_id,stage,operation,
 collection_acl_revision,collection_visibility_epoch,collection_processing_revision,
 document_visibility_epoch,requested_by_user_id,idempotency_scope,idempotency_key,request_hash
) VALUES ($1,$2,$3,$4,$5,'purge','purge',$6,$7,$8,$9,$10,$11,$12,$13)
`, versions[index].PurgeJobID, collectionID, input.DocumentID, versions[index].ID,
			versions[index].FileID, collection.ACLRevision, collection.VisibilityEpoch,
			collection.ProcessingRevision, newVisibilityEpoch, input.ActorUserID,
			"document:"+input.DocumentID+":version:"+versions[index].ID+":purge",
			fmt.Sprintf("visibility:%d", newVisibilityEpoch), hex.EncodeToString(requestHash[:]))
		if err != nil {
			return fmt.Errorf("insert document purge job: %w", err)
		}
	}
	if err := r.insertDocumentDeletionOutbox(
		ctx, tx, input.DocumentID, collectionID, newVisibilityEpoch,
		collection, versions, cancelled,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete document: %w", err)
	}
	return nil
}

func lockDocumentVersionsForDeletion(ctx context.Context, tx *sql.Tx, documentID string) ([]tombstoneVersion, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id,file_id,source_version,content_hash,status,visibility_epoch
FROM knowledge_document_versions
WHERE document_id=$1 AND status<>'deleted' ORDER BY id FOR UPDATE
`, documentID)
	if err != nil {
		return nil, fmt.Errorf("lock document versions for deletion: %w", err)
	}
	defer rows.Close()
	versions := make([]tombstoneVersion, 0)
	for rows.Next() {
		var version tombstoneVersion
		if err := rows.Scan(&version.ID, &version.FileID, &version.SourceVersion,
			&version.ContentHash, &version.Status, &version.VisibilityEpoch); err != nil {
			return nil, fmt.Errorf("scan document version for deletion: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document versions for deletion: %w", err)
	}
	return versions, nil
}

func lockCancelableDocumentJobs(ctx context.Context, tx *sql.Tx, documentID string) ([]cancelledJob, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id,document_version_id FROM knowledge_processing_jobs
WHERE document_id=$1 AND status IN ('pending','processing') ORDER BY id FOR UPDATE
`, documentID)
	if err != nil {
		return nil, fmt.Errorf("lock cancelable document jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]cancelledJob, 0)
	for rows.Next() {
		var job cancelledJob
		if err := rows.Scan(&job.ID, &job.VersionID); err != nil {
			return nil, fmt.Errorf("scan cancelable document job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cancelable document jobs: %w", err)
	}
	return jobs, nil
}

func (r *PostgresRepository) insertDocumentDeletionOutbox(
	ctx context.Context,
	tx *sql.Tx,
	documentID, collectionID string,
	documentVisibilityEpoch int64,
	collection collectionRow,
	versions []tombstoneVersion,
	cancelled []cancelledJob,
) error {
	for _, job := range cancelled {
		if err := r.insertKnowledgeEvent(ctx, tx, documentID, "knowledge.processing.cancelled", map[string]any{
			"schemaVersion": 1, "collectionId": collectionID, "documentId": documentID,
			"documentVersionId": job.VersionID, "jobId": job.ID,
			"documentVisibilityEpoch":      documentVisibilityEpoch,
			"collectionAclRevision":        collection.ACLRevision,
			"collectionVisibilityEpoch":    collection.VisibilityEpoch,
			"collectionProcessingRevision": collection.ProcessingRevision,
		}); err != nil {
			return err
		}
	}
	for _, version := range versions {
		if err := r.insertKnowledgeEvent(ctx, tx, documentID, "knowledge.document.tombstoned", map[string]any{
			"schemaVersion": 1, "collectionId": collectionID, "documentId": documentID,
			"documentVersionId": version.ID, "sourceVersion": version.SourceVersion,
			"fileId": version.FileID, "contentHash": version.ContentHash,
			"purgeJobId":                   version.PurgeJobID,
			"documentVisibilityEpoch":      documentVisibilityEpoch,
			"versionVisibilityEpoch":       version.VisibilityEpoch,
			"collectionAclRevision":        collection.ACLRevision,
			"collectionVisibilityEpoch":    collection.VisibilityEpoch,
			"collectionProcessingRevision": collection.ProcessingRevision,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) insertKnowledgeEvent(
	ctx context.Context,
	tx *sql.Tx,
	documentID, eventType string,
	payload map[string]any,
) error {
	eventID, err := r.newEventID()
	if err != nil {
		return fmt.Errorf("generate knowledge event id: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal knowledge event: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($1,'knowledge_document',$2,$3,$4::jsonb)
`, eventID, documentID, eventType, string(encoded))
	if err != nil {
		return fmt.Errorf("insert knowledge event: %w", err)
	}
	return nil
}
