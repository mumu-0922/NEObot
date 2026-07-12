package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const (
	deleteRaceOwnerID      = "14000000-0000-4000-8000-000000000001"
	deleteRaceCollectionID = "34000000-0000-4000-8000-000000000001"
	deleteRaceFileID       = "54000000-0000-4000-8000-000000000001"
	deleteRaceDocumentID   = "44000000-0000-4000-8000-000000000001"
	deleteRaceVersionID    = "64000000-0000-4000-8000-000000000001"
	deleteRaceInitialJobID = "74000000-0000-4000-8000-000000000001"
	deleteRaceReprocessJob = "74000000-0000-4000-8000-000000000002"
)

func TestPostgresDocumentDeleteAndReprocessSerialize(t *testing.T) {
	for _, deleteFirst := range []bool{true, false} {
		name := "reprocess-first"
		if deleteFirst {
			name = "delete-first"
		}
		t.Run(name, func(t *testing.T) {
			runDocumentDeleteReprocessRace(t, deleteFirst)
		})
	}
}

func runDocumentDeleteReprocessRace(t *testing.T, deleteFirst bool) {
	t.Helper()
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	seedDocumentDeleteReprocessRace(t, ctx, db)
	var applicationName string
	if err := db.QueryRowContext(ctx, `SHOW application_name`).Scan(&applicationName); err != nil {
		t.Fatal(err)
	}

	gateConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gateTx, err := gateConn.BeginTx(ctx, nil)
	if err != nil {
		_ = gateConn.Close()
		t.Fatal(err)
	}
	gateFinished := false
	t.Cleanup(func() {
		if !gateFinished {
			if err := gateTx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("rollback delete/reprocess gate: %v", err)
			}
		}
		if err := gateConn.Close(); err != nil {
			t.Errorf("close delete/reprocess gate connection: %v", err)
		}
	})
	var gatePID int
	if err := gateTx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatal(err)
	}
	var collectionID string
	if err := gateTx.QueryRowContext(ctx, `SELECT id FROM knowledge_collections WHERE id=$1 FOR UPDATE`, deleteRaceCollectionID).Scan(&collectionID); err != nil {
		t.Fatal(err)
	}

	repo := NewPostgresRepository(db)
	deleteDone := make(chan error, 1)
	reprocessDone := make(chan error, 1)
	deleteOperation := func() error {
		return repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
			DocumentID: deleteRaceDocumentID, ActorUserID: deleteRaceOwnerID,
		})
	}
	reprocessOperation := func() error {
		_, err := repo.ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
			JobID: deleteRaceReprocessJob, DocumentID: deleteRaceDocumentID,
			ActorUserID: deleteRaceOwnerID, IdempotencyKey: "delete-race-reprocess",
			RequestHash: strings.Repeat("d", 64), ParseProcessor: "mineru",
		})
		return err
	}

	if deleteFirst {
		go func() { deleteDone <- deleteOperation() }()
		deletePID := waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, gatePID, "FROM knowledge_collections", "FOR UPDATE")
		go func() { reprocessDone <- reprocessOperation() }()
		_ = waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, deletePID, "FROM users", "FOR UPDATE")
	} else {
		go func() { reprocessDone <- reprocessOperation() }()
		reprocessPID := waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, gatePID, "FROM knowledge_collections", "FOR UPDATE")
		go func() { deleteDone <- deleteOperation() }()
		_ = waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, reprocessPID, "FROM users", "FOR UPDATE")
	}
	if err := gateTx.Commit(); err != nil {
		t.Fatalf("release collection gate: %v", err)
	}
	gateFinished = true
	if err := waitForKnowledgeRaceResult(t, ctx, deleteDone); err != nil {
		t.Fatalf("document delete: %v", err)
	}
	reprocessErr := waitForKnowledgeRaceResult(t, ctx, reprocessDone)
	if deleteFirst {
		if !errors.Is(reprocessErr, ErrDocumentNotFound) {
			t.Fatalf("delete-first reprocess error = %v, want ErrDocumentNotFound", reprocessErr)
		}
	} else if reprocessErr != nil {
		t.Fatalf("reprocess-first reprocess error = %v", reprocessErr)
	}
	assertDocumentDeleteReprocessRace(t, ctx, db, !deleteFirst)
}

func seedDocumentDeleteReprocessRace(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users(id,email,display_name)
VALUES ($1,'delete-race@example.test','Delete Race');
INSERT INTO knowledge_collections(id,name,scope,owner_user_id)
VALUES ($2,'Delete Race','personal',$1);
INSERT INTO files(
  id,user_id,original_filename,mime_type,byte_size,sha256,upload_status,
  storage_backend,object_key,metadata
) VALUES ($3,$1,'delete-race.pdf','application/pdf',10,$4,'available','local',$5,'{"purpose":"knowledge"}')
`, deleteRaceOwnerID, deleteRaceCollectionID, deleteRaceFileID, strings.Repeat("a", 64),
		"users/"+deleteRaceOwnerID+"/files/"+deleteRaceFileID)
	manifest := GovernanceManifest{
		Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled",
	}
	repo := NewPostgresRepository(db)
	if _, err := NewGovernanceService(repo).Apply(ctx, manifest); err != nil {
		t.Fatalf("seed governance: %v", err)
	}
	if _, err := repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
		CollectionID: deleteRaceCollectionID, ActorUserID: deleteRaceOwnerID,
		Processor: "mineru", Purposes: []string{"parse"},
		DataTypes: []string{"application/pdf"}, PolicyVersion: "v1",
	}); err != nil {
		t.Fatalf("seed collection consent: %v", err)
	}
	if _, err := repo.CreateDocument(ctx, CreateDocumentRepositoryInput{
		DocumentID: deleteRaceDocumentID, VersionID: deleteRaceVersionID,
		JobID: deleteRaceInitialJobID, CollectionID: deleteRaceCollectionID,
		ActorUserID: deleteRaceOwnerID, FileID: deleteRaceFileID,
		IdempotencyKey: "delete-race-initial", RequestHash: strings.Repeat("b", 64),
		ParseProcessor: "mineru",
	}); err != nil {
		t.Fatalf("seed document: %v", err)
	}
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_document_versions SET status='active' WHERE id=$1;
UPDATE knowledge_documents SET status='active',current_version_id=$1 WHERE id=$2;
UPDATE knowledge_processing_jobs
SET status='succeeded',attempt_count=1,completed_at=clock_timestamp(),updated_at=clock_timestamp()
WHERE id=$3
`, deleteRaceVersionID, deleteRaceDocumentID, deleteRaceInitialJobID)
}

func assertDocumentDeleteReprocessRace(t *testing.T, ctx context.Context, db *sql.DB, reprocessFirst bool) {
	t.Helper()
	var documentStatus, versionStatus string
	var documentDeleted bool
	var documentEpoch, versionEpoch int64
	if err := db.QueryRowContext(ctx, `SELECT status,deleted_at IS NOT NULL,visibility_epoch FROM knowledge_documents WHERE id=$1`, deleteRaceDocumentID).Scan(&documentStatus, &documentDeleted, &documentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status,visibility_epoch FROM knowledge_document_versions WHERE id=$1`, deleteRaceVersionID).Scan(&versionStatus, &versionEpoch); err != nil {
		t.Fatal(err)
	}
	if documentStatus != "tombstoned" || !documentDeleted || documentEpoch != 2 || versionStatus != "tombstoned" || versionEpoch != 2 {
		t.Fatalf("document/version tombstone = %s/%v/%d %s/%d", documentStatus, documentDeleted, documentEpoch, versionStatus, versionEpoch)
	}
	var purgeJobs, tombstoneEvents, exactTombstones int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND stage='purge' AND operation='purge' AND status='pending' AND document_version_id=$2 AND document_visibility_epoch=2 AND collection_acl_revision=1 AND collection_visibility_epoch=1 AND collection_processing_revision=2`, deleteRaceDocumentID, deleteRaceVersionID).Scan(&purgeJobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.tombstoned'`, deleteRaceDocumentID).Scan(&tombstoneEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.tombstoned' AND (payload->>'documentVisibilityEpoch')::bigint=2 AND (payload->>'versionVisibilityEpoch')::bigint=2 AND (payload->>'collectionAclRevision')::bigint=1 AND (payload->>'collectionVisibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=2`, deleteRaceDocumentID).Scan(&exactTombstones); err != nil {
		t.Fatal(err)
	}
	if purgeJobs != 1 || tombstoneEvents != 1 || exactTombstones != 1 {
		t.Fatalf("purge/tombstone/exact events = %d/%d/%d", purgeJobs, tombstoneEvents, exactTombstones)
	}

	var reprocessJobs, reprocessEvents, cancellationEvents, exactCancellations, exactReprocessEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE id=$1 AND operation='reprocess'`, deleteRaceReprocessJob).Scan(&reprocessJobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.reprocess.requested'`, deleteRaceDocumentID).Scan(&reprocessEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.reprocess.requested' AND payload->>'jobId'=$2 AND payload->>'documentVersionId'=$3 AND (payload->>'documentVisibilityEpoch')::bigint=1 AND (payload->>'collectionAclRevision')::bigint=1 AND (payload->>'collectionVisibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=2`, deleteRaceDocumentID, deleteRaceReprocessJob, deleteRaceVersionID).Scan(&exactReprocessEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.processing.cancelled'`, deleteRaceDocumentID).Scan(&cancellationEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.processing.cancelled' AND payload->>'jobId'=$2 AND payload->>'documentVersionId'=$3 AND (payload->>'documentVisibilityEpoch')::bigint=2 AND (payload->>'collectionAclRevision')::bigint=1 AND (payload->>'collectionVisibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=2`, deleteRaceDocumentID, deleteRaceReprocessJob, deleteRaceVersionID).Scan(&exactCancellations); err != nil {
		t.Fatal(err)
	}
	var initialStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM knowledge_processing_jobs WHERE id=$1`, deleteRaceInitialJobID).Scan(&initialStatus); err != nil {
		t.Fatal(err)
	}
	if initialStatus != "succeeded" {
		t.Fatalf("initial job status = %s, want succeeded", initialStatus)
	}
	var collectionACL, collectionVisibility, collectionProcessing int64
	if err := db.QueryRowContext(ctx, `SELECT acl_revision,visibility_epoch,collection_processing_revision FROM knowledge_collections WHERE id=$1`, deleteRaceCollectionID).Scan(&collectionACL, &collectionVisibility, &collectionProcessing); err != nil {
		t.Fatal(err)
	}
	if collectionACL != 1 || collectionVisibility != 1 || collectionProcessing != 2 {
		t.Fatalf("collection revisions = %d/%d/%d", collectionACL, collectionVisibility, collectionProcessing)
	}
	if reprocessFirst {
		var status, versionID string
		var completed, leaseCleared bool
		var documentEpoch, aclRevision, visibilityEpoch, processingRevision int64
		if err := db.QueryRowContext(ctx, `SELECT status,completed_at IS NOT NULL,lease_owner IS NULL AND lease_expires_at IS NULL,document_version_id,document_visibility_epoch,collection_acl_revision,collection_visibility_epoch,collection_processing_revision FROM knowledge_processing_jobs WHERE id=$1`, deleteRaceReprocessJob).Scan(&status, &completed, &leaseCleared, &versionID, &documentEpoch, &aclRevision, &visibilityEpoch, &processingRevision); err != nil {
			t.Fatal(err)
		}
		if status != "cancelled" || !completed || !leaseCleared || versionID != deleteRaceVersionID || documentEpoch != 1 || aclRevision != 1 || visibilityEpoch != 1 || processingRevision != 2 || reprocessJobs != 1 || reprocessEvents != 1 || exactReprocessEvents != 1 || cancellationEvents != 1 || exactCancellations != 1 {
			t.Fatalf("reprocess-first job=%d/%s/%v/%v version=%s fences=%d/%d/%d/%d events=%d/%d/%d", reprocessJobs, status, completed, leaseCleared, versionID, documentEpoch, aclRevision, visibilityEpoch, processingRevision, reprocessEvents, exactReprocessEvents, cancellationEvents)
		}
	} else if reprocessJobs != 0 || reprocessEvents != 0 || exactReprocessEvents != 0 || cancellationEvents != 0 || exactCancellations != 0 {
		t.Fatalf("delete-first reprocess side effects = %d/%d/%d/%d/%d", reprocessJobs, reprocessEvents, exactReprocessEvents, cancellationEvents, exactCancellations)
	}
}
