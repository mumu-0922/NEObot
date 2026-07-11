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
	for _, authorityTest := range []struct {
		collectionID string
		mimeType     string
		want         error
	}{
		{personalID, "application/pdf", nil},
		{"30000000-0000-4000-8000-000000000099", "application/pdf", ErrProcessingConsent},
		{personalID, "text/html", ErrKnowledgeProcessorUnavailable},
	} {
		authorityTx, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		_, authorityErr := resolveParseAuthority(
			ctx, authorityTx, authorityTest.collectionID, "mineru", authorityTest.mimeType,
		)
		_ = authorityTx.Rollback()
		if authorityErr != authorityTest.want {
			t.Fatalf("resolve authority collection=%s mime=%s error=%v, want %v",
				authorityTest.collectionID, authorityTest.mimeType, authorityErr, authorityTest.want)
		}
	}
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

	const (
		replacementFileID    = "50000000-0000-4000-8000-000000000010"
		replacementVersionID = "50000000-0000-4000-8000-000000000011"
		replacementJobID     = "50000000-0000-4000-8000-000000000012"
	)
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_document_versions SET status='active' WHERE id=$1;
UPDATE knowledge_documents SET status='active',current_version_id=$1 WHERE id=$2;
UPDATE knowledge_processing_jobs
SET status='succeeded',attempt_count=1,completed_at=now(),updated_at=now()
WHERE id=$7;
INSERT INTO files (
  id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata
) VALUES ($3,$4,'replacement.pdf','application/pdf',12,$5,'local',$6,'{"purpose":"knowledge"}')
`, versionID, documentID, replacementFileID, adminID, strings.Repeat("c", 64),
		"users/"+adminID+"/files/"+replacementFileID, jobID)
	reprocessInputs := []ReprocessDocumentRepositoryInput{
		{JobID: "50000000-0000-4000-8000-000000000030", DocumentID: documentID,
			ActorUserID: adminID, IdempotencyKey: "reprocess-race",
			RequestHash: strings.Repeat("6", 64), ParseProcessor: "mineru"},
		{JobID: "50000000-0000-4000-8000-000000000031", DocumentID: documentID,
			ActorUserID: adminID, IdempotencyKey: "reprocess-race",
			RequestHash: strings.Repeat("6", 64), ParseProcessor: "mineru"},
	}
	type reprocessResult struct {
		document Document
		err      error
	}
	reprocessResults := make(chan reprocessResult, len(reprocessInputs))
	var reprocessWait sync.WaitGroup
	reprocessRepo := NewPostgresRepository(db)
	for _, input := range reprocessInputs {
		reprocessWait.Add(1)
		go func(input ReprocessDocumentRepositoryInput) {
			defer reprocessWait.Done()
			reprocessed, reprocessErr := reprocessRepo.ReprocessDocument(ctx, input)
			reprocessResults <- reprocessResult{document: reprocessed, err: reprocessErr}
		}(input)
	}
	reprocessWait.Wait()
	close(reprocessResults)
	for result := range reprocessResults {
		if result.err != nil || result.document.CurrentVersion == nil ||
			result.document.CurrentVersion.ID != versionID || result.document.PendingVersion != nil {
			t.Fatalf("concurrent active reprocess = %#v, err=%v", result.document, result.err)
		}
	}
	var reprocessJobID string
	var reprocessScope string
	var reprocessJobs, reprocessEvents, versionsAfterReprocess int
	if err := db.QueryRowContext(ctx, `
SELECT id,idempotency_scope FROM knowledge_processing_jobs
WHERE document_id=$1 AND operation='reprocess' AND idempotency_key='reprocess-race'
`, documentID).Scan(&reprocessJobID, &reprocessScope); err != nil {
		t.Fatal(err)
	}
	if reprocessScope != documentOperationIdempotencyScope(documentID, "reprocess", adminID) {
		t.Fatalf("reprocess idempotency scope = %q", reprocessScope)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND operation='reprocess'`, documentID).Scan(&reprocessJobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.reprocess.requested'`, documentID).Scan(&reprocessEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_document_versions WHERE document_id=$1`, documentID).Scan(&versionsAfterReprocess); err != nil {
		t.Fatal(err)
	}
	if reprocessJobs != 1 || reprocessEvents != 1 || versionsAfterReprocess != 1 {
		t.Fatalf("reprocess jobs/events/versions = %d/%d/%d", reprocessJobs, reprocessEvents, versionsAfterReprocess)
	}
	var reprocessOperation, reprocessSourceVersion, reprocessCause, reprocessContentHash string
	if err := db.QueryRowContext(ctx, `
SELECT payload->>'operation',payload->>'sourceVersion',payload->>'causedByJobId',payload->>'contentHash'
FROM knowledge_outbox WHERE aggregate_key=$1 AND payload->>'jobId'=$2
`, documentID, reprocessJobID).Scan(
		&reprocessOperation, &reprocessSourceVersion, &reprocessCause, &reprocessContentHash,
	); err != nil {
		t.Fatal(err)
	}
	if reprocessOperation != "reprocess" || reprocessSourceVersion != "1" ||
		reprocessCause != jobID || reprocessContentHash != strings.Repeat("a", 64) {
		t.Fatalf("reprocess outbox operation/source/cause/hash = %s/%s/%s/%s",
			reprocessOperation, reprocessSourceVersion, reprocessCause, reprocessContentHash)
	}
	if _, err := reprocessRepo.ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
		JobID: "50000000-0000-4000-8000-000000000032", DocumentID: documentID,
		ActorUserID: adminID, IdempotencyKey: "reprocess-race",
		RequestHash: strings.Repeat("5", 64), ParseProcessor: "mineru",
	}); err != ErrIdempotencyConflict {
		t.Fatalf("changed reprocess replay error = %v", err)
	}
	if _, err := reprocessRepo.ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
		JobID: "50000000-0000-4000-8000-000000000033", DocumentID: documentID,
		ActorUserID: adminID, IdempotencyKey: "reprocess-second",
		RequestHash: strings.Repeat("4", 64), ParseProcessor: "mineru",
	}); err != ErrDocumentProcessing {
		t.Fatalf("second reprocess error = %v", err)
	}
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_processing_jobs
SET status='succeeded',attempt_count=1,completed_at=now(),updated_at=now()
WHERE id=$1
`, reprocessJobID)
	replacementIDs := []string{replacementVersionID, replacementJobID,
		"50000000-0000-4000-8000-000000000013", "50000000-0000-4000-8000-000000000014",
		"50000000-0000-4000-8000-000000000015", "50000000-0000-4000-8000-000000000016",
		"50000000-0000-4000-8000-000000000017", "50000000-0000-4000-8000-000000000018"}
	replacementService := NewService(NewPostgresRepository(db), WithIDGenerator(func() (string, error) {
		id := replacementIDs[0]
		replacementIDs = replacementIDs[1:]
		return id, nil
	}))
	if _, err := NewPostgresRepository(db).CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
		VersionID: "50000000-0000-4000-8000-000000000090",
		JobID:     "50000000-0000-4000-8000-000000000091", DocumentID: documentID,
		ActorUserID: outsiderID, FileID: replacementFileID, IdempotencyKey: "outsider-replace",
		RequestHash: strings.Repeat("9", 64), ParseProcessor: "mineru",
	}); err != ErrDocumentNotFound {
		t.Fatalf("personal outsider replacement error = %v", err)
	}
	replacement, err := replacementService.CreateDocumentVersion(adminCtx, documentID, BindDocumentInput{
		FileID: replacementFileID, IdempotencyKey: "replacement-1",
	})
	if err != nil || replacement.CurrentVersion == nil || replacement.CurrentVersion.ID != versionID ||
		replacement.PendingVersion == nil || replacement.PendingVersion.ID != replacementVersionID ||
		replacement.PendingVersion.SourceVersion != 2 {
		t.Fatalf("replacement = %#v, err=%v", replacement, err)
	}
	activeContent, err := NewPostgresRepository(db).GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{
		DocumentID: documentID, ActorUserID: adminID,
	})
	if err != nil || activeContent.FileID != fileID {
		t.Fatalf("content switched before publish = %#v, err=%v", activeContent, err)
	}
	replayedReplacement, err := replacementService.CreateDocumentVersion(adminCtx, documentID, BindDocumentInput{
		FileID: replacementFileID, IdempotencyKey: "replacement-1",
	})
	if err != nil || replayedReplacement.PendingVersion == nil || replayedReplacement.PendingVersion.ID != replacementVersionID {
		t.Fatalf("replacement replay = %#v, err=%v", replayedReplacement, err)
	}
	if _, err := replacementService.CreateDocumentVersion(adminCtx, documentID, BindDocumentInput{
		FileID: "50000000-0000-4000-8000-000000000099", IdempotencyKey: "replacement-1",
	}); err != ErrIdempotencyConflict {
		t.Fatalf("replacement changed replay error = %v", err)
	}
	if _, err := replacementService.CreateDocumentVersion(adminCtx, documentID, BindDocumentInput{
		FileID: replacementFileID, IdempotencyKey: "replacement-2",
	}); err != ErrDocumentProcessing {
		t.Fatalf("second nonterminal replacement error = %v", err)
	}
	var replacementJobs, replacementEvents int
	var replacementScope string
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND operation='replace'`, documentID).Scan(&replacementJobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT idempotency_scope FROM knowledge_processing_jobs WHERE document_version_id=$1`, replacementVersionID).Scan(&replacementScope); err != nil {
		t.Fatal(err)
	}
	if replacementScope != documentOperationIdempotencyScope(documentID, "replace", adminID) {
		t.Fatalf("replacement idempotency scope = %q", replacementScope)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.version.requested'`, documentID).Scan(&replacementEvents); err != nil {
		t.Fatal(err)
	}
	if replacementJobs != 1 || replacementEvents != 2 {
		t.Fatalf("replacement jobs/total events = %d/%d", replacementJobs, replacementEvents)
	}
	var replacementOperation, replacementSourceVersion, replacementProcessor string
	var replacementACLRevision, replacementDocumentEpoch int64
	if err := db.QueryRowContext(ctx, `
SELECT payload->>'operation',payload->>'sourceVersion',payload->>'processor',
  (payload->>'collectionAclRevision')::bigint,(payload->>'documentVisibilityEpoch')::bigint
FROM knowledge_outbox
WHERE aggregate_key=$1 AND payload->>'documentVersionId'=$2
`, documentID, replacementVersionID).Scan(
		&replacementOperation, &replacementSourceVersion, &replacementProcessor,
		&replacementACLRevision, &replacementDocumentEpoch,
	); err != nil {
		t.Fatal(err)
	}
	if replacementOperation != "replace" || replacementSourceVersion != "2" ||
		replacementProcessor != "mineru" || replacementACLRevision != 1 || replacementDocumentEpoch != 1 {
		t.Fatalf("replacement outbox fences = %s/%s/%s/%d/%d", replacementOperation,
			replacementSourceVersion, replacementProcessor, replacementACLRevision, replacementDocumentEpoch)
	}

	const (
		raceFileA = "50000000-0000-4000-8000-000000000020"
		raceFileB = "50000000-0000-4000-8000-000000000021"
	)
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_document_versions SET status='failed',error_code='PARSE_FAILED' WHERE id=$1;
UPDATE knowledge_processing_jobs
SET status='failed',attempt_count=1,completed_at=now(),error_code='PARSE_FAILED',updated_at=now()
WHERE document_version_id=$1;
INSERT INTO files (
 id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata
) VALUES
 ($2,$4,'race-a.pdf','application/pdf',13,$5,'local',$6,'{"purpose":"knowledge"}'),
 ($3,$4,'race-b.pdf','application/pdf',14,$7,'local',$8,'{"purpose":"knowledge"}')
`, replacementVersionID, raceFileA, raceFileB, adminID, strings.Repeat("d", 64),
		"users/"+adminID+"/files/"+raceFileA, strings.Repeat("e", 64),
		"users/"+adminID+"/files/"+raceFileB)
	failedReprocess, err := NewPostgresRepository(db).ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
		JobID: "50000000-0000-4000-8000-000000000034", DocumentID: documentID,
		ActorUserID: adminID, IdempotencyKey: "failed-reprocess",
		RequestHash: strings.Repeat("3", 64), ParseProcessor: "mineru",
	})
	if err != nil || failedReprocess.CurrentVersion == nil || failedReprocess.CurrentVersion.ID != versionID ||
		failedReprocess.PendingVersion == nil || failedReprocess.PendingVersion.ID != replacementVersionID ||
		failedReprocess.PendingVersion.Status != "uploaded" || failedReprocess.PendingVersion.SourceVersion != 2 {
		t.Fatalf("failed pending reprocess = %#v, err=%v", failedReprocess, err)
	}
	var versionsAfterFailedReprocess int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_document_versions WHERE document_id=$1`, documentID).
		Scan(&versionsAfterFailedReprocess); err != nil {
		t.Fatal(err)
	}
	if versionsAfterFailedReprocess != 2 {
		t.Fatalf("failed reprocess created source version: %d", versionsAfterFailedReprocess)
	}
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_document_versions SET status='failed',error_code='PARSE_FAILED' WHERE id=$1;
UPDATE knowledge_processing_jobs
SET status='failed',attempt_count=1,completed_at=now(),error_code='PARSE_FAILED',updated_at=now()
WHERE id='50000000-0000-4000-8000-000000000034'
`, replacementVersionID)
	replayRaceInputs := []CreateDocumentVersionRepositoryInput{
		{VersionID: "50000000-0000-4000-8000-000000000022", JobID: "50000000-0000-4000-8000-000000000023",
			DocumentID: documentID, ActorUserID: adminID, FileID: raceFileA,
			IdempotencyKey: "race-replay", RequestHash: strings.Repeat("1", 64), ParseProcessor: "mineru"},
		{VersionID: "50000000-0000-4000-8000-000000000024", JobID: "50000000-0000-4000-8000-000000000025",
			DocumentID: documentID, ActorUserID: adminID, FileID: raceFileA,
			IdempotencyKey: "race-replay", RequestHash: strings.Repeat("1", 64), ParseProcessor: "mineru"},
	}
	raceRepo := NewPostgresRepository(db)
	type replacementResult struct {
		document Document
		err      error
	}
	replayResults := make(chan replacementResult, len(replayRaceInputs))
	var raceWait sync.WaitGroup
	for _, input := range replayRaceInputs {
		raceWait.Add(1)
		go func(input CreateDocumentVersionRepositoryInput) {
			defer raceWait.Done()
			replaced, replaceErr := raceRepo.CreateDocumentVersion(ctx, input)
			replayResults <- replacementResult{document: replaced, err: replaceErr}
		}(input)
	}
	raceWait.Wait()
	close(replayResults)
	var replayVersionID string
	for result := range replayResults {
		if result.err != nil || result.document.PendingVersion == nil {
			t.Fatalf("concurrent replacement replay = %#v, err=%v", result.document, result.err)
		}
		if replayVersionID == "" {
			replayVersionID = result.document.PendingVersion.ID
		} else if result.document.PendingVersion.ID != replayVersionID {
			t.Fatalf("concurrent replay version ids = %s/%s", replayVersionID, result.document.PendingVersion.ID)
		}
	}
	var replayVersions, replayJobs int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_document_versions WHERE document_id=$1 AND idempotency_key='race-replay'`, documentID).Scan(&replayVersions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND idempotency_key='race-replay'`, documentID).Scan(&replayJobs); err != nil {
		t.Fatal(err)
	}
	if replayVersions != 1 || replayJobs != 1 {
		t.Fatalf("concurrent replay versions/jobs = %d/%d", replayVersions, replayJobs)
	}
	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_document_versions SET status='failed',error_code='PARSE_FAILED' WHERE id=$1;
UPDATE knowledge_processing_jobs
SET status='failed',attempt_count=1,completed_at=now(),error_code='PARSE_FAILED',updated_at=now()
WHERE document_version_id=$1
`, replayVersionID)

	raceInputs := []CreateDocumentVersionRepositoryInput{
		{VersionID: "50000000-0000-4000-8000-000000000026", JobID: "50000000-0000-4000-8000-000000000027",
			DocumentID: documentID, ActorUserID: adminID, FileID: raceFileA,
			IdempotencyKey: "race-a", RequestHash: strings.Repeat("2", 64), ParseProcessor: "mineru"},
		{VersionID: "50000000-0000-4000-8000-000000000028", JobID: "50000000-0000-4000-8000-000000000029",
			DocumentID: documentID, ActorUserID: adminID, FileID: raceFileB,
			IdempotencyKey: "race-b", RequestHash: strings.Repeat("3", 64), ParseProcessor: "mineru"},
	}
	raceErrors := make(chan error, len(raceInputs))
	raceWait = sync.WaitGroup{}
	for _, input := range raceInputs {
		raceWait.Add(1)
		go func(input CreateDocumentVersionRepositoryInput) {
			defer raceWait.Done()
			_, replaceErr := raceRepo.CreateDocumentVersion(ctx, input)
			raceErrors <- replaceErr
		}(input)
	}
	raceWait.Wait()
	close(raceErrors)
	var winners, blocked int
	for replaceErr := range raceErrors {
		switch replaceErr {
		case nil:
			winners++
		case ErrDocumentProcessing:
			blocked++
		default:
			t.Fatalf("concurrent replacement error = %v", replaceErr)
		}
	}
	if winners != 1 || blocked != 1 {
		t.Fatalf("concurrent replacement winners/blocked = %d/%d", winners, blocked)
	}

	teamCollection, err := service.CreateCollection(adminCtx, CreateCollectionInput{
		Name: "Shared", Scope: ScopeTeam, TeamID: teamID, IdempotencyKey: "team-1",
	})
	if err != nil || teamCollection.ID != teamColID {
		t.Fatalf("create team collection = %#v, err=%v", teamCollection, err)
	}
	const (
		teamDocumentID = "50000000-0000-4000-8000-000000000092"
		teamVersionID  = "50000000-0000-4000-8000-000000000093"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($1,$2,'processing');
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash)
VALUES ($3,$1,$4,1,'active',$5);
UPDATE knowledge_documents SET status='active',current_version_id=$3 WHERE id=$1
`, teamDocumentID, teamColID, teamVersionID, fileID, strings.Repeat("a", 64))
	for _, denied := range []struct {
		actorID string
		want    error
	}{
		{memberID, ErrTeamAdminRequired},
		{outsiderID, ErrDocumentNotFound},
	} {
		_, replaceErr := NewPostgresRepository(db).CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
			VersionID: "50000000-0000-4000-8000-000000000094",
			JobID:     "50000000-0000-4000-8000-000000000095", DocumentID: teamDocumentID,
			ActorUserID: denied.actorID, FileID: replacementFileID, IdempotencyKey: "denied-replace",
			RequestHash: strings.Repeat("8", 64), ParseProcessor: "mineru",
		})
		if replaceErr != denied.want {
			t.Fatalf("team replacement actor=%s error=%v, want %v", denied.actorID, replaceErr, denied.want)
		}
		_, reprocessErr := NewPostgresRepository(db).ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
			JobID: "50000000-0000-4000-8000-000000000099", DocumentID: teamDocumentID,
			ActorUserID: denied.actorID, IdempotencyKey: "denied-reprocess",
			RequestHash: strings.Repeat("6", 64), ParseProcessor: "mineru",
		})
		if reprocessErr != denied.want {
			t.Fatalf("team reprocess actor=%s error=%v, want %v", denied.actorID, reprocessErr, denied.want)
		}
		deleteErr := NewPostgresRepository(db).DeleteDocument(ctx, DeleteDocumentRepositoryInput{
			DocumentID: teamDocumentID, ActorUserID: denied.actorID,
		})
		if deleteErr != denied.want {
			t.Fatalf("team delete actor=%s error=%v, want %v", denied.actorID, deleteErr, denied.want)
		}
	}
	if _, err := NewPostgresRepository(db).CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
		VersionID: "50000000-0000-4000-8000-000000000096",
		JobID:     "50000000-0000-4000-8000-000000000097", DocumentID: teamDocumentID,
		ActorUserID: adminID, FileID: "50000000-0000-4000-8000-000000000098",
		IdempotencyKey: "missing-file-replace", RequestHash: strings.Repeat("7", 64),
		ParseProcessor: "mineru",
	}); err != ErrFileNotFound {
		t.Fatalf("replacement missing file error = %v", err)
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
	publishTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishTx.ExecContext(ctx, `
INSERT INTO knowledge_document_versions (id,document_id,file_id,source_version,status,content_hash)
VALUES ('61000000-0000-4000-8000-000000000005',$1,$2,3,'active',$3);
UPDATE knowledge_documents
SET current_version_id='61000000-0000-4000-8000-000000000005',updated_at=now()
WHERE id=$1;
UPDATE knowledge_document_versions SET status='tombstoned',updated_at=now() WHERE id=$4
`, activeDocID, activeFileID, strings.Repeat("a", 64), activeVersionID); err != nil {
		_ = publishTx.Rollback()
		t.Fatal(err)
	}
	if err := publishTx.Commit(); err != nil {
		t.Fatal(err)
	}
	published, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: activeDocID, ActorUserID: memberID})
	if err != nil || published.CurrentVersion == nil || published.CurrentVersion.SourceVersion != 3 ||
		published.PendingVersion != nil {
		t.Fatalf("historical failed version appeared pending = %#v, err=%v", published, err)
	}
	targetTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, wasFailed, targetErr := resolveReprocessTarget(ctx, targetTx, activeDocID,
		sql.NullString{String: "61000000-0000-4000-8000-000000000005", Valid: true})
	_ = targetTx.Rollback()
	if targetErr != nil || wasFailed || target == nil || target.SourceVersion != 3 {
		t.Fatalf("historical failed reprocess target = %#v/%v, err=%v", target, wasFailed, targetErr)
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

func TestPostgresDocumentDeletionTombstonesAndQueuesPurge(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	const (
		ownerID      = "12000000-0000-4000-8000-000000000001"
		outsiderID   = "12000000-0000-4000-8000-000000000002"
		collectionID = "32000000-0000-4000-8000-000000000001"
		documentID   = "42000000-0000-4000-8000-000000000001"
		fileA        = "52000000-0000-4000-8000-000000000001"
		fileB        = "52000000-0000-4000-8000-000000000002"
		versionA     = "62000000-0000-4000-8000-000000000001"
		versionB     = "62000000-0000-4000-8000-000000000002"
		versionC     = "62000000-0000-4000-8000-000000000003"
		cancelJobID  = "72000000-0000-4000-8000-000000000001"
		seedEventID  = "82000000-0000-4000-8000-000000000000"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users(id,email,display_name) VALUES
 ($1,'delete-owner@example.test','Owner'),($2,'delete-outsider@example.test','Outsider');
INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($3,'Delete','personal',$1);
INSERT INTO files(id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata) VALUES
 ($4,$1,'a.txt','text/plain',1,$6,'local','delete/a','{"purpose":"knowledge"}'),
 ($5,$1,'b.txt','text/plain',1,$7,'local','delete/b','{"purpose":"knowledge"}');
INSERT INTO knowledge_documents(id,collection_id,status) VALUES ($8,$3,'processing');
INSERT INTO knowledge_document_versions(id,document_id,file_id,source_version,status,content_hash) VALUES
 ($9,$8,$4,1,'active',$6),($10,$8,$5,2,'failed',$7),
 ($12,$8,$4,3,'tombstoned',$6);
UPDATE knowledge_document_versions SET visibility_epoch=5 WHERE id=$12;
UPDATE knowledge_documents SET status='active',current_version_id=$9 WHERE id=$8;
INSERT INTO knowledge_processing_jobs(
 id,collection_id,document_id,document_version_id,file_id,stage,operation,
 collection_acl_revision,collection_visibility_epoch,collection_processing_revision,
 document_visibility_epoch,idempotency_scope,idempotency_key,request_hash
) VALUES ($11,$3,$8,$10,$5,'purge','purge',1,1,1,1,'seed:purge','seed',$6);
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ($13,'test','seed','test.seed','{}')
`, ownerID, outsiderID, collectionID, fileA, fileB, strings.Repeat("a", 64),
		strings.Repeat("b", 64), documentID, versionA, versionB, cancelJobID, versionC,
		seedEventID)

	repo := NewPostgresRepository(db)
	if err := repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
		DocumentID: documentID, ActorUserID: outsiderID,
	}); err != ErrDocumentNotFound {
		t.Fatalf("outsider deletion error = %v", err)
	}

	assertDeletionRolledBack := func(label string) {
		t.Helper()
		var status string
		var visibility int64
		var deleted bool
		if err := db.QueryRowContext(ctx, `
SELECT status,visibility_epoch,deleted_at IS NOT NULL FROM knowledge_documents WHERE id=$1
`, documentID).Scan(&status, &visibility, &deleted); err != nil {
			t.Fatal(err)
		}
		if status != "active" || visibility != 1 || deleted {
			t.Fatalf("%s committed deletion = %s/%d/%v", label, status, visibility, deleted)
		}
		var generatedRows int
		if err := db.QueryRowContext(ctx, `
SELECT (SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND document_visibility_epoch=2)
     + (SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1)
`, documentID).Scan(&generatedRows); err != nil {
			t.Fatal(err)
		}
		if generatedRows != 0 {
			t.Fatalf("%s left generated rows = %d", label, generatedRows)
		}
	}

	idFailing := NewPostgresRepository(db)
	generated := 0
	idFailing.newEventID = func() (string, error) {
		generated++
		if generated == 1 {
			return "", errors.New("synthetic purge id failure")
		}
		return fmt.Sprintf("82000000-0000-4000-8000-%012d", generated), nil
	}
	if err := idFailing.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
		DocumentID: documentID, ActorUserID: ownerID,
	}); err == nil {
		t.Fatal("deletion purge id failure error = nil")
	}
	assertDeletionRolledBack("purge id failure")

	outboxFailing := NewPostgresRepository(db)
	generated = 0
	outboxFailing.newEventID = func() (string, error) {
		generated++
		if generated == 4 {
			return seedEventID, nil
		}
		return fmt.Sprintf("83000000-0000-4000-8000-%012d", generated), nil
	}
	if err := outboxFailing.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
		DocumentID: documentID, ActorUserID: ownerID,
	}); err == nil {
		t.Fatal("deletion outbox insert failure error = nil")
	}
	assertDeletionRolledBack("outbox insert failure")

	var status string
	var visibility int64
	var deleted bool

	deleteErrors := make(chan error, 2)
	var deleteWait sync.WaitGroup
	for range 2 {
		deleteWait.Add(1)
		go func() {
			defer deleteWait.Done()
			deleteErrors <- repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
				DocumentID: documentID, ActorUserID: ownerID,
			})
		}()
	}
	deleteWait.Wait()
	close(deleteErrors)
	for deleteErr := range deleteErrors {
		if deleteErr != nil {
			t.Fatalf("concurrent delete error = %v", deleteErr)
		}
	}

	if err := db.QueryRowContext(ctx, `
SELECT status,visibility_epoch,deleted_at IS NOT NULL FROM knowledge_documents WHERE id=$1
`, documentID).Scan(&status, &visibility, &deleted); err != nil {
		t.Fatal(err)
	}
	if status != "tombstoned" || visibility != 2 || !deleted {
		t.Fatalf("document tombstone = %s/%d/%v", status, visibility, deleted)
	}
	var tombstonedVersions, purgeJobs, cancelledJobs, tombstoneEvents, cancellationEvents int
	for query, destination := range map[string]*int{
		`SELECT count(*) FROM knowledge_document_versions WHERE document_id=$1 AND status='tombstoned'`:                  &tombstonedVersions,
		`SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND operation='purge' AND status='pending'`: &purgeJobs,
		`SELECT count(*) FROM knowledge_processing_jobs WHERE document_id=$1 AND status='cancelled'`:                     &cancelledJobs,
		`SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.document.tombstoned'`:    &tombstoneEvents,
		`SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.processing.cancelled'`:   &cancellationEvents,
	} {
		if err := db.QueryRowContext(ctx, query, documentID).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	if tombstonedVersions != 3 || purgeJobs != 3 || cancelledJobs != 1 ||
		tombstoneEvents != 3 || cancellationEvents != 1 {
		t.Fatalf("delete dependents versions/purge/cancel/events = %d/%d/%d/%d/%d",
			tombstonedVersions, purgeJobs, cancelledJobs, tombstoneEvents, cancellationEvents)
	}
	var inconsistentTombstoneEvents int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM knowledge_outbox o
JOIN knowledge_document_versions v
  ON v.id=(o.payload->>'documentVersionId')::uuid
JOIN knowledge_documents d ON d.id=v.document_id
JOIN knowledge_collections c ON c.id=d.collection_id
JOIN knowledge_processing_jobs j ON j.id=(o.payload->>'purgeJobId')::uuid
WHERE o.aggregate_type='knowledge_document' AND o.aggregate_key=$1
  AND o.event_type='knowledge.document.tombstoned'
  AND (
    (o.payload->>'schemaVersion')::int IS DISTINCT FROM 1
    OR o.payload->>'collectionId' IS DISTINCT FROM d.collection_id::text
    OR o.payload->>'documentId' IS DISTINCT FROM d.id::text
    OR (o.payload->>'sourceVersion')::bigint IS DISTINCT FROM v.source_version
    OR o.payload->>'fileId' IS DISTINCT FROM v.file_id::text
    OR o.payload->>'contentHash' IS DISTINCT FROM v.content_hash
    OR (o.payload->>'documentVisibilityEpoch')::bigint IS DISTINCT FROM d.visibility_epoch
    OR (o.payload->>'versionVisibilityEpoch')::bigint IS DISTINCT FROM v.visibility_epoch
    OR (o.payload->>'collectionAclRevision')::bigint IS DISTINCT FROM c.acl_revision
    OR (o.payload->>'collectionVisibilityEpoch')::bigint IS DISTINCT FROM c.visibility_epoch
    OR (o.payload->>'collectionProcessingRevision')::bigint IS DISTINCT FROM c.collection_processing_revision
    OR j.document_id<>d.id OR j.document_version_id<>v.id
    OR j.document_visibility_epoch<>d.visibility_epoch
    OR j.stage<>'purge' OR j.operation<>'purge'
  )
`, documentID).Scan(&inconsistentTombstoneEvents); err != nil {
		t.Fatal(err)
	}
	if inconsistentTombstoneEvents != 0 {
		t.Fatalf("inconsistent document tombstone events = %d", inconsistentTombstoneEvents)
	}

	_, duplicatePurgeErr := db.ExecContext(ctx, `
INSERT INTO knowledge_processing_jobs (
 id,collection_id,document_id,document_version_id,file_id,stage,operation,
 collection_acl_revision,collection_visibility_epoch,collection_processing_revision,
 document_visibility_epoch,requested_by_user_id,idempotency_scope,idempotency_key,request_hash
)
SELECT '84000000-0000-4000-8000-000000000001',collection_id,document_id,
 document_version_id,file_id,stage,operation,collection_acl_revision,
 collection_visibility_epoch,collection_processing_revision,document_visibility_epoch,
 requested_by_user_id,idempotency_scope||':duplicate','duplicate',request_hash
FROM knowledge_processing_jobs
WHERE document_id=$1 AND document_version_id=$2 AND stage='purge' AND operation='purge'
  AND document_visibility_epoch=2
`, documentID, versionA)
	if !isConstraint(duplicatePurgeErr, "idx_knowledge_processing_jobs_purge_fence") {
		t.Fatalf("duplicate purge fence error = %v", duplicatePurgeErr)
	}
	var historicalEpoch int64
	if err := db.QueryRowContext(ctx, `
SELECT visibility_epoch FROM knowledge_document_versions WHERE id=$1
`, versionC).Scan(&historicalEpoch); err != nil {
		t.Fatal(err)
	}
	if historicalEpoch != 5 {
		t.Fatalf("historical tombstone epoch advanced: %d", historicalEpoch)
	}
	var historicalEventEpoch int64
	if err := db.QueryRowContext(ctx, `
SELECT (payload->>'versionVisibilityEpoch')::bigint FROM knowledge_outbox
WHERE aggregate_key=$1 AND event_type='knowledge.document.tombstoned'
  AND payload->>'documentVersionId'=$2
`, documentID, versionC).Scan(&historicalEventEpoch); err != nil {
		t.Fatal(err)
	}
	if historicalEventEpoch != historicalEpoch {
		t.Fatalf("historical tombstone DB/event epoch = %d/%d", historicalEpoch, historicalEventEpoch)
	}
	var availableFiles int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM files WHERE id IN ($1,$2) AND upload_status='available' AND deleted_at IS NULL
`, fileA, fileB).Scan(&availableFiles); err != nil {
		t.Fatal(err)
	}
	if availableFiles != 2 {
		t.Fatalf("source files changed by document deletion: %d", availableFiles)
	}
	var liveBindings int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM knowledge_document_versions
WHERE document_id=$1 AND status IN ('uploaded','processing','failed','active','purging')
`, documentID).Scan(&liveBindings); err != nil {
		t.Fatal(err)
	}
	if liveBindings != 0 {
		t.Fatalf("document deletion left live file bindings: %d", liveBindings)
	}
	var fileDeleteEvents int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM knowledge_outbox
WHERE aggregate_type='file' AND event_type='file.object.delete.requested'
  AND payload->>'fileId' IN ($1,$2)
`, fileA, fileB).Scan(&fileDeleteEvents); err != nil {
		t.Fatal(err)
	}
	if fileDeleteEvents != 0 {
		t.Fatalf("document deletion queued source file cleanup: %d", fileDeleteEvents)
	}
	if _, err := repo.GetDocument(ctx, DocumentLookupInput{
		DocumentID: documentID, ActorUserID: ownerID,
	}); err != ErrDocumentNotFound {
		t.Fatalf("deleted document read error = %v", err)
	}
	beforeReplayEvents := tombstoneEvents + cancellationEvents
	if err := repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{
		DocumentID: documentID, ActorUserID: ownerID,
	}); err != nil {
		t.Fatalf("repeat document delete: %v", err)
	}
	var afterReplayEvents int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM knowledge_outbox
WHERE aggregate_key=$1 AND event_type IN ('knowledge.document.tombstoned','knowledge.processing.cancelled')
`, documentID).Scan(&afterReplayEvents); err != nil {
		t.Fatal(err)
	}
	if afterReplayEvents != beforeReplayEvents {
		t.Fatalf("repeat deletion emitted events: %d -> %d", beforeReplayEvents, afterReplayEvents)
	}
}

func openKnowledgeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("MM_CHAT_REQUIRE_POSTGRES_TESTS") == "true" {
			t.Fatal("MM_CHAT_REQUIRE_POSTGRES_TESTS=true requires MM_CHAT_TEST_DATABASE_URL")
		}
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
	testConfig.RuntimeParams["application_name"] = schema
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
