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

func TestPostgresConcurrentFirstDocumentBindCreatesOneTransition(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const (
		ownerID      = "15000000-0000-4000-8000-000000000001"
		collectionID = "35000000-0000-4000-8000-000000000001"
		fileID       = "55000000-0000-4000-8000-000000000001"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users(id,email,display_name)
VALUES ($1,'first-bind@example.test','First Bind');
INSERT INTO knowledge_collections(id,name,scope,owner_user_id)
VALUES ($2,'First Bind','personal',$1);
INSERT INTO files(
  id,user_id,original_filename,mime_type,byte_size,sha256,upload_status,
  storage_backend,object_key,metadata
) VALUES ($3,$1,'first-bind.pdf','application/pdf',10,$4,'available','local',$5,'{"purpose":"knowledge"}')
`, ownerID, collectionID, fileID, strings.Repeat("a", 64), "users/"+ownerID+"/files/"+fileID)
	repo := NewPostgresRepository(db)
	manifest := GovernanceManifest{
		Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled",
	}
	if _, err := NewGovernanceService(repo).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
		CollectionID: collectionID, ActorUserID: ownerID, Processor: "mineru",
		Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1",
	}); err != nil {
		t.Fatal(err)
	}
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
				t.Errorf("rollback first-bind gate: %v", err)
			}
		}
		if err := gateConn.Close(); err != nil {
			t.Errorf("close first-bind gate connection: %v", err)
		}
	})
	var gatePID int
	if err := gateTx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatal(err)
	}
	var lockedCollection string
	if err := gateTx.QueryRowContext(ctx, `SELECT id FROM knowledge_collections WHERE id=$1 FOR UPDATE`, collectionID).Scan(&lockedCollection); err != nil {
		t.Fatal(err)
	}
	inputs := []CreateDocumentRepositoryInput{
		{
			DocumentID: "45000000-0000-4000-8000-000000000001", VersionID: "65000000-0000-4000-8000-000000000001",
			JobID: "75000000-0000-4000-8000-000000000001", CollectionID: collectionID,
			ActorUserID: ownerID, FileID: fileID, IdempotencyKey: "concurrent-first-bind",
			RequestHash: strings.Repeat("b", 64), ParseProcessor: "mineru",
		},
		{
			DocumentID: "45000000-0000-4000-8000-000000000002", VersionID: "65000000-0000-4000-8000-000000000002",
			JobID: "75000000-0000-4000-8000-000000000002", CollectionID: collectionID,
			ActorUserID: ownerID, FileID: fileID, IdempotencyKey: "concurrent-first-bind",
			RequestHash: strings.Repeat("b", 64), ParseProcessor: "mineru",
		},
	}
	type result struct {
		document Document
		err      error
	}
	results := make(chan result, len(inputs))
	go func() {
		document, err := repo.CreateDocument(ctx, inputs[0])
		results <- result{document: document, err: err}
	}()
	firstPID := waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, gatePID, "FROM knowledge_collections", "FOR UPDATE")
	go func() {
		document, err := repo.CreateDocument(ctx, inputs[1])
		results <- result{document: document, err: err}
	}()
	_ = waitForKnowledgeBlockedByPID(t, ctx, db, applicationName, firstPID, "FROM users", "FOR UPDATE")
	if err := gateTx.Commit(); err != nil {
		t.Fatalf("release first-bind gate: %v", err)
	}
	gateFinished = true
	values := make([]result, 0, len(inputs))
	for range inputs {
		select {
		case value := <-results:
			values = append(values, value)
		case <-ctx.Done():
			t.Fatalf("concurrent first bind timed out: %v", ctx.Err())
		}
	}
	winningDocumentID := ""
	winningVersionID := ""
	for _, value := range values {
		if value.err != nil || value.document.PendingVersion == nil {
			t.Fatalf("concurrent first bind = %#v, err=%v", value.document, value.err)
		}
		if winningDocumentID == "" {
			winningDocumentID = value.document.ID
			winningVersionID = value.document.PendingVersion.ID
		} else if value.document.ID != winningDocumentID {
			t.Fatalf("concurrent first bind document IDs = %s/%s", winningDocumentID, value.document.ID)
		} else if value.document.PendingVersion.ID != winningVersionID {
			t.Fatalf("concurrent first bind version IDs = %s/%s", winningVersionID, value.document.PendingVersion.ID)
		}
	}

	var documents, versions, jobs, events, exactEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_documents WHERE collection_id=$1 AND idempotency_key='concurrent-first-bind'`, collectionID).Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_document_versions WHERE document_id=$1 AND idempotency_key='concurrent-first-bind'`, winningDocumentID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND operation='initial' AND idempotency_key='concurrent-first-bind'`, winningDocumentID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.version.requested'`, winningDocumentID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.version.requested' AND payload->>'operation'='initial' AND payload->>'fileId'=$2 AND (payload->>'collectionAclRevision')::bigint=1 AND (payload->>'collectionVisibilityEpoch')::bigint=1 AND (payload->>'collectionProcessingRevision')::bigint=2 AND (payload->>'documentVisibilityEpoch')::bigint=1`, winningDocumentID, fileID).Scan(&exactEvents); err != nil {
		t.Fatal(err)
	}
	if documents != 1 || versions != 1 || jobs != 1 || events != 1 || exactEvents != 1 {
		t.Fatalf("first-bind documents/versions/jobs/events/exact = %d/%d/%d/%d/%d", documents, versions, jobs, events, exactEvents)
	}
	var uploadStatus string
	if err := db.QueryRowContext(ctx, `SELECT upload_status FROM files WHERE id=$1 AND deleted_at IS NULL`, fileID).Scan(&uploadStatus); err != nil {
		t.Fatal(err)
	}
	if uploadStatus != "available" {
		t.Fatalf("bound source file status = %s", uploadStatus)
	}
}
