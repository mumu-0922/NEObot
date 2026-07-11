package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/teams"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresCollectionACLRevisionIdempotencyAndOutbox(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const (
		adminID    = "10000000-0000-4000-8000-000000000001"
		memberID   = "10000000-0000-4000-8000-000000000002"
		outsiderID = "10000000-0000-4000-8000-000000000003"
		teamID     = "20000000-0000-4000-8000-000000000001"
		personalID = "30000000-0000-4000-8000-000000000001"
		teamColID  = "30000000-0000-4000-8000-000000000002"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users (id, email, display_name) VALUES
  ($1, 'admin@example.test', 'Admin'),
  ($2, 'member@example.test', 'Member'),
  ($3, 'outsider@example.test', 'Outsider')
`, adminID, memberID, outsiderID)
	mustKnowledgeExec(t, ctx, db, `INSERT INTO teams (id, name, created_by_user_id) VALUES ($1, 'Research', $2)`, teamID, adminID)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO team_memberships (team_id, user_id, role) VALUES
  ($1, $2, 'admin'), ($1, $3, 'member')
`, teamID, adminID, memberID)

	ids := []string{personalID,
		"30000000-0000-4000-8000-000000000003",
		"30000000-0000-4000-8000-000000000004",
		teamColID,
		"30000000-0000-4000-8000-000000000005"}
	codec, err := teams.NewCursorCodec(teams.CursorKeyring{
		ActiveKeyID: "test",
		Keys:        map[string][]byte{"test": []byte("01234567890123456789012345678901")},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewPostgresRepository(db), WithCursorCodec(codec), WithIDGenerator(func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}))
	adminCtx := auth.WithUser(ctx, auth.User{ID: adminID})
	memberCtx := auth.WithUser(ctx, auth.User{ID: memberID})
	outsiderCtx := auth.WithUser(ctx, auth.User{ID: outsiderID})

	personal, err := service.CreateCollection(adminCtx, CreateCollectionInput{
		Name: "Private", Scope: ScopePersonal, IdempotencyKey: "personal-1",
	})
	if err != nil || personal.ID != personalID || !personal.Permissions.Manage {
		t.Fatalf("create personal = %#v, err=%v", personal, err)
	}
	replayed, err := service.CreateCollection(adminCtx, CreateCollectionInput{
		Name: "Private", Scope: ScopePersonal, IdempotencyKey: "personal-1",
	})
	if err != nil || replayed.ID != personalID {
		t.Fatalf("replay personal = %#v, err=%v", replayed, err)
	}
	if _, err := service.CreateCollection(adminCtx, CreateCollectionInput{
		Name: "Different", Scope: ScopePersonal, IdempotencyKey: "personal-1",
	}); err != ErrIdempotencyConflict {
		t.Fatalf("changed idempotency error = %v", err)
	}

	const (
		documentID = "50000000-0000-4000-8000-000000000001"
		versionID  = "50000000-0000-4000-8000-000000000002"
		jobID      = "50000000-0000-4000-8000-000000000003"
		fileID     = "50000000-0000-4000-8000-000000000004"
		profileID  = "50000000-0000-4000-8000-000000000005"
		consentID  = "50000000-0000-4000-8000-000000000006"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO files (
  id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata
) VALUES ($1,$2,'source.pdf','application/pdf',10,$3,'local',$4,'{"purpose":"knowledge"}')
`, fileID, adminID, strings.Repeat("a", 64), "users/"+adminID+"/files/"+fileID)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO processor_governance_profiles (
 id,processor,endpoint_id,model_api_version,allowed_purposes,allowed_data_types,
 region,retention_policy,deletion_contract,training_use,status,governance_revision,manifest_hash
) VALUES ($1,'mineru','default','v1',ARRAY['parse'],ARRAY['application/pdf'],
 'global','none','delete','disabled','approved',1,$2);
INSERT INTO processor_governance_heads (
 processor,endpoint_id,status,active_profile_id,active_governance_revision,head_revision
) VALUES ('mineru','default','active',$1,1,1);
INSERT INTO processing_consents (
 id,scope,collection_id,processor,endpoint_id,governance_profile_id,
 governance_revision,governance_head_revision,purposes,data_types,policy_version,
 decision,consent_revision,granted_by_user_id
) VALUES ($3,'collection',$4,'mineru','default',$1,1,1,ARRAY['parse'],
 ARRAY['application/pdf'],'v1','granted',1,$5)
`, profileID, strings.Repeat("b", 64), consentID, personalID, adminID)
	documentIDs := []string{documentID, versionID, jobID,
		"50000000-0000-4000-8000-000000000007",
		"50000000-0000-4000-8000-000000000008",
		"50000000-0000-4000-8000-000000000009"}
	documentService := NewService(NewPostgresRepository(db), WithIDGenerator(func() (string, error) {
		id := documentIDs[0]
		documentIDs = documentIDs[1:]
		return id, nil
	}))
	document, err := documentService.CreateDocument(adminCtx, personalID, BindDocumentInput{
		FileID: fileID, IdempotencyKey: "document-1",
	})
	if err != nil || document.ID != documentID || document.PendingVersion == nil || document.PendingVersion.ID != versionID {
		t.Fatalf("create document = %#v, err=%v", document, err)
	}
	replayedDocument, err := documentService.CreateDocument(adminCtx, personalID, BindDocumentInput{
		FileID: fileID, IdempotencyKey: "document-1",
	})
	if err != nil || replayedDocument.ID != documentID {
		t.Fatalf("replay document = %#v, err=%v", replayedDocument, err)
	}
	var jobs, events int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1`, documentID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.version.requested'`, documentID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || events != 1 {
		t.Fatalf("document jobs/events = %d/%d", jobs, events)
	}

	teamCollection, err := service.CreateCollection(adminCtx, CreateCollectionInput{
		Name: "Shared", Scope: ScopeTeam, TeamID: teamID, IdempotencyKey: "team-1",
	})
	if err != nil || teamCollection.ID != teamColID {
		t.Fatalf("create team collection = %#v, err=%v", teamCollection, err)
	}
	if _, err := service.CreateCollection(memberCtx, CreateCollectionInput{
		Name: "Forbidden", Scope: ScopeTeam, TeamID: teamID, IdempotencyKey: "member-1",
	}); err != ErrTeamAdminRequired {
		t.Fatalf("member create error = %v", err)
	}
	adminPage, err := service.ListCollections(adminCtx, ListCollectionsInput{Limit: 1})
	if err != nil || len(adminPage.Items) != 1 || adminPage.NextCursor == "" {
		t.Fatalf("admin first page = %#v, err=%v", adminPage, err)
	}
	adminSecondPage, err := service.ListCollections(adminCtx, ListCollectionsInput{Limit: 1, Cursor: adminPage.NextCursor})
	if err != nil || len(adminSecondPage.Items) != 1 || adminSecondPage.NextCursor != "" {
		t.Fatalf("admin second page = %#v, err=%v", adminSecondPage, err)
	}
	memberPage, err := service.ListCollections(memberCtx, ListCollectionsInput{Scope: ScopeTeam, TeamID: teamID})
	if err != nil || len(memberPage.Items) != 1 || memberPage.Items[0].ID != teamColID {
		t.Fatalf("member team page = %#v, err=%v", memberPage, err)
	}
	outsiderPage, err := service.ListCollections(outsiderCtx, ListCollectionsInput{})
	if err != nil || len(outsiderPage.Items) != 0 {
		t.Fatalf("outsider page = %#v, err=%v", outsiderPage, err)
	}

	memberView, err := service.GetCollection(memberCtx, teamColID)
	if err != nil || memberView.Permissions.Manage || !memberView.Permissions.Read {
		t.Fatalf("member view = %#v, err=%v", memberView, err)
	}
	for _, attempt := range []struct {
		ctx context.Context
		id  string
	}{
		{memberCtx, personalID}, {outsiderCtx, personalID}, {outsiderCtx, teamColID},
	} {
		if _, err := service.GetCollection(attempt.ctx, attempt.id); err != ErrCollectionNotFound {
			t.Fatalf("cross-scope get %s error = %v", attempt.id, err)
		}
	}

	if _, err := service.UpdateCollection(memberCtx, teamColID, UpdateCollectionInput{Name: stringPtr("No")}); err != ErrTeamAdminRequired {
		t.Fatalf("member update error = %v", err)
	}
	updated, err := service.UpdateCollection(adminCtx, teamColID, UpdateCollectionInput{Name: stringPtr("Shared Docs")})
	if err != nil || updated.Name != "Shared Docs" || updated.ACLRevision != 1 || updated.VisibilityEpoch != 1 {
		t.Fatalf("metadata update = %#v, err=%v", updated, err)
	}
	beforeNoop := knowledgeOutboxCount(t, ctx, db)
	if _, err := service.UpdateCollection(adminCtx, teamColID, UpdateCollectionInput{Name: stringPtr("Shared Docs")}); err != nil {
		t.Fatalf("no-op update error = %v", err)
	}
	if got := knowledgeOutboxCount(t, ctx, db); got != beforeNoop {
		t.Fatalf("no-op outbox count = %d, want %d", got, beforeNoop)
	}

	if err := service.DeleteCollection(adminCtx, teamColID); err != nil {
		t.Fatalf("delete team collection: %v", err)
	}
	afterDelete := knowledgeOutboxCount(t, ctx, db)
	if err := service.DeleteCollection(adminCtx, teamColID); err != nil {
		t.Fatalf("repeat delete: %v", err)
	}
	if got := knowledgeOutboxCount(t, ctx, db); got != afterDelete {
		t.Fatalf("repeat delete outbox count = %d, want %d", got, afterDelete)
	}
	if _, err := service.GetCollection(memberCtx, teamColID); err != ErrCollectionNotFound {
		t.Fatalf("deleted collection get error = %v", err)
	}

	var aclRevision, visibilityEpoch int64
	var deleted bool
	if err := db.QueryRowContext(ctx, `
SELECT acl_revision, visibility_epoch, deleted_at IS NOT NULL
FROM knowledge_collections WHERE id = $1
`, teamColID).Scan(&aclRevision, &visibilityEpoch, &deleted); err != nil {
		t.Fatal(err)
	}
	if aclRevision != 2 || visibilityEpoch != 2 || !deleted {
		t.Fatalf("delete fences = %d/%d/%v", aclRevision, visibilityEpoch, deleted)
	}

	failingRepo := NewPostgresRepository(db)
	failingRepo.newEventID = func() (string, error) { return "", errors.New("synthetic outbox failure") }
	rollbackID := "30000000-0000-4000-8000-000000000009"
	_, err = failingRepo.CreateCollection(ctx, CreateCollectionRepositoryInput{
		ID: rollbackID, ActorUserID: adminID, Name: "Rollback", Icon: "Folder", Color: "blue",
		Scope: ScopePersonal, IdempotencyKey: "rollback", CreateRequestHash: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	})
	if err == nil {
		t.Fatal("outbox failure create error = nil")
	}
	var rollbackRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_collections WHERE id = $1`, rollbackID).Scan(&rollbackRows); err != nil {
		t.Fatal(err)
	}
	if rollbackRows != 0 {
		t.Fatalf("collection committed despite outbox failure: %d", rollbackRows)
	}

	repo := NewPostgresRepository(db)
	concurrentHash := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	concurrentInputs := []CreateCollectionRepositoryInput{
		{ID: "30000000-0000-4000-8000-000000000010", ActorUserID: adminID, Name: "Concurrent", Icon: "Folder", Color: "blue", Scope: ScopePersonal, IdempotencyKey: "concurrent", CreateRequestHash: concurrentHash},
		{ID: "30000000-0000-4000-8000-000000000011", ActorUserID: adminID, Name: "Concurrent", Icon: "Folder", Color: "blue", Scope: ScopePersonal, IdempotencyKey: "concurrent", CreateRequestHash: concurrentHash},
	}
	var wait sync.WaitGroup
	results := make(chan Collection, 2)
	errorsCh := make(chan error, 2)
	for _, input := range concurrentInputs {
		wait.Add(1)
		go func(input CreateCollectionRepositoryInput) {
			defer wait.Done()
			collection, createErr := repo.CreateCollection(ctx, input)
			results <- collection
			errorsCh <- createErr
		}(input)
	}
	wait.Wait()
	close(results)
	close(errorsCh)
	for createErr := range errorsCh {
		if createErr != nil {
			t.Fatalf("concurrent create error = %v", createErr)
		}
	}
	var winningID string
	for collection := range results {
		if winningID == "" {
			winningID = collection.ID
		} else if collection.ID != winningID {
			t.Fatalf("concurrent replay ids = %s and %s", winningID, collection.ID)
		}
	}
	var concurrentRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_collections WHERE created_by_user_id = $1 AND idempotency_key = 'concurrent'`, adminID).Scan(&concurrentRows); err != nil {
		t.Fatal(err)
	}
	if concurrentRows != 1 {
		t.Fatalf("concurrent rows = %d", concurrentRows)
	}
}

func TestPostgresDocumentReadsEnforceACLAndActiveVersion(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	const (
		adminID          = "11000000-0000-4000-8000-000000000001"
		memberID         = "11000000-0000-4000-8000-000000000002"
		outsiderID       = "11000000-0000-4000-8000-000000000003"
		teamID           = "21000000-0000-4000-8000-000000000001"
		teamColID        = "31000000-0000-4000-8000-000000000001"
		personalID       = "31000000-0000-4000-8000-000000000002"
		activeDocID      = "41000000-0000-4000-8000-000000000001"
		pendingDocID     = "41000000-0000-4000-8000-000000000002"
		privateDocID     = "41000000-0000-4000-8000-000000000003"
		activeFileID     = "51000000-0000-4000-8000-000000000001"
		pendingFileID    = "51000000-0000-4000-8000-000000000002"
		activeVersionID  = "61000000-0000-4000-8000-000000000001"
		pendingVersionID = "61000000-0000-4000-8000-000000000002"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users (id,email,display_name) VALUES
 ($1,'read-admin@example.test','Admin'),($2,'read-member@example.test','Member'),
 ($3,'read-outsider@example.test','Outsider');
INSERT INTO teams (id,name,created_by_user_id) VALUES ($4,'Read Team',$1);
INSERT INTO team_memberships (team_id,user_id,role) VALUES ($4,$1,'admin'),($4,$2,'member');
INSERT INTO knowledge_collections (id,name,scope,team_id) VALUES ($5,'Shared','team',$4);
INSERT INTO knowledge_collections (id,name,scope,owner_user_id) VALUES ($6,'Private','personal',$1)
`, adminID, memberID, outsiderID, teamID, teamColID, personalID)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO files (id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata) VALUES
 ($1,$3,'active.txt','text/plain',6,$4,'local','knowledge/active','{"purpose":"knowledge"}'),
 ($2,$3,'pending.txt','text/plain',7,$5,'local','knowledge/pending','{"purpose":"knowledge"}')
`, activeFileID, pendingFileID, adminID, strings.Repeat("a", 64), strings.Repeat("b", 64))
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($1,$2,'processing');
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash)
 VALUES ($3,$1,$4,1,'active',$5);
UPDATE knowledge_documents SET status='active',current_version_id=$3 WHERE id=$1;
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash,error_code)
 VALUES ($6,$1,$7,2,'failed',$8,'PARSE_FAILED');
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($9,$2,'processing');
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash)
 VALUES ('61000000-0000-4000-8000-000000000003',$9,$7,1,'processing',$8);
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($10,$11,'processing');
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash)
 VALUES ('61000000-0000-4000-8000-000000000004',$10,$4,1,'processing',$5)
`, activeDocID, teamColID, activeVersionID, activeFileID, strings.Repeat("a", 64),
		pendingVersionID, pendingFileID, strings.Repeat("b", 64), pendingDocID, privateDocID, personalID)

	repo := NewPostgresRepository(db)
	page, err := repo.ListDocuments(ctx, ListDocumentsRepositoryInput{
		CollectionID: teamColID, ActorUserID: memberID, Limit: 10,
	})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("member list = %#v, err=%v", page, err)
	}
	active, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: activeDocID, ActorUserID: memberID})
	if err != nil || active.CurrentVersion == nil || active.CurrentVersion.ID != activeVersionID ||
		active.PendingVersion == nil || active.PendingVersion.ID != pendingVersionID || active.PendingVersion.ErrorCode != "PARSE_FAILED" {
		t.Fatalf("active metadata = %#v, err=%v", active, err)
	}
	content, err := repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{DocumentID: activeDocID, ActorUserID: memberID})
	if err != nil || content.FileID != activeFileID || content.ObjectKey != "knowledge/active" {
		t.Fatalf("active content metadata = %#v, err=%v", content, err)
	}
	for _, lookup := range []DocumentLookupInput{
		{DocumentID: pendingDocID, ActorUserID: memberID},
		{DocumentID: activeDocID, ActorUserID: outsiderID},
		{DocumentID: privateDocID, ActorUserID: memberID},
	} {
		_, err := repo.GetActiveDocumentContentMetadata(ctx, lookup)
		if err != ErrDocumentNotFound {
			t.Fatalf("content lookup %#v error = %v", lookup, err)
		}
	}
	mustKnowledgeExec(t, ctx, db, `UPDATE team_memberships SET status='removed', removed_at=now() WHERE team_id=$1 AND user_id=$2`, teamID, memberID)
	if _, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: activeDocID, ActorUserID: memberID}); err != ErrDocumentNotFound {
		t.Fatalf("removed member document error = %v", err)
	}
	if _, err := repo.ListDocuments(ctx, ListDocumentsRepositoryInput{CollectionID: teamColID, ActorUserID: memberID, Limit: 10}); err != ErrCollectionNotFound {
		t.Fatalf("removed member list error = %v", err)
	}
}

func openKnowledgeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	adminConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	adminDB := stdlib.OpenDB(*adminConfig)
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := fmt.Sprintf("knowledge_phase15d_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
	})
	testConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	testConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	testConfig.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*testConfig)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustKnowledgeExec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec: %v\n%s", err, query)
	}
}

func knowledgeOutboxCount(t *testing.T, ctx context.Context, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_type = 'knowledge_collection'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func stringPtr(value string) *string { return &value }
