package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresGovernanceApplyDisableIsAtomicAndIdempotent(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	manifest := GovernanceManifest{Processor: "mineru", EndpointID: "default", ModelID: "model-v1", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	service := NewGovernanceService(NewPostgresRepository(db))
	head, err := service.Apply(ctx, manifest)
	if err != nil || head.HeadRevision != 1 || head.ActiveGovernanceRevision != 1 || head.Status != "active" ||
		head.ModelID != "model-v1" || len(head.ProfileContractHash) != 64 {
		t.Fatalf("first apply = %#v, err=%v", head, err)
	}
	replayed, err := service.Apply(ctx, manifest)
	if err != nil || replayed.ActiveProfileID != head.ActiveProfileID || replayed.HeadRevision != 1 {
		t.Fatalf("replay = %#v, err=%v", replayed, err)
	}
	legacyManifest := manifest
	legacyManifest.ModelID = ""
	legacyReplay, err := service.Apply(ctx, legacyManifest)
	if err != nil || legacyReplay.ActiveProfileID != head.ActiveProfileID || legacyReplay.ModelID != "model-v1" {
		t.Fatalf("unambiguous legacy replay = %#v, err=%v", legacyReplay, err)
	}
	manifest.ModelAPIVersion = "v2"
	head, err = service.Apply(ctx, manifest)
	if err != nil || head.HeadRevision != 2 || head.ActiveGovernanceRevision != 2 {
		t.Fatalf("changed apply = %#v, err=%v", head, err)
	}
	head, err = service.Disable(ctx, "mineru", "default")
	if err != nil || head.Status != "disabled" || head.HeadRevision != 3 || head.ActiveProfileID != "" {
		t.Fatalf("disable = %#v, err=%v", head, err)
	}
	replayed, err = service.Disable(ctx, "mineru", "default")
	if err != nil || replayed.HeadRevision != 3 {
		t.Fatalf("disable replay = %#v, err=%v", replayed, err)
	}
	var profiles, events int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='mineru' AND endpoint_id='default'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_type='processor_governance_head' AND aggregate_key='mineru/default/model-v1'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if profiles != 2 || events != 3 {
		t.Fatalf("profiles/events = %d/%d", profiles, events)
	}
	var exactPayloads int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox
WHERE aggregate_key='mineru/default/model-v1' AND payload->>'modelId'='model-v1'
AND ((payload->>'status'='active' AND length(payload->>'profileContractHash')=64)
  OR (payload->>'status'='disabled' AND NOT payload ? 'profileContractHash'))`).Scan(&exactPayloads); err != nil {
		t.Fatal(err)
	}
	if exactPayloads != 3 {
		t.Fatalf("exact governance payloads = %d", exactPayloads)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processor_governance_profiles SET model_api_version='mutated' WHERE processor='mineru'`); err == nil {
		t.Fatal("governance profile update unexpectedly succeeded")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM processor_governance_profiles WHERE processor='mineru'`); err == nil {
		t.Fatal("governance profile delete unexpectedly succeeded")
	}
	var leaked int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key='mineru/default/model-v1' AND (payload ? 'retentionPolicy' OR payload ? 'deletionContract' OR payload ? 'trainingUse')`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("policy fields leaked into outbox: %d", leaked)
	}

	var existingEventID string
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox WHERE aggregate_key='mineru/default/model-v1' LIMIT 1`).Scan(&existingEventID); err != nil {
		t.Fatal(err)
	}
	failing := NewPostgresRepository(db)
	generated := 0
	failing.newEventID = func() (string, error) {
		generated++
		if generated == 2 {
			return existingEventID, nil
		}
		return fmt.Sprintf("91000000-0000-4000-8000-%012d", generated), nil
	}
	manifest.ModelAPIVersion = "v3"
	if _, err := NewGovernanceService(failing).Apply(ctx, manifest); err == nil {
		t.Fatal("failed outbox apply error = nil")
	}
	var status string
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT status,head_revision FROM processor_governance_heads WHERE processor='mineru' AND endpoint_id='default'`).Scan(&status, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" || revision != 3 {
		t.Fatalf("failed apply committed head = %s/%d", status, revision)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='mineru' AND endpoint_id='default'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if profiles != 2 {
		t.Fatalf("failed apply committed profile count = %d", profiles)
	}
}

func TestPostgresGovernanceConcurrentFirstApplySerializes(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	base := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelID: "model-v1", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"passage_embedding"}, AllowedDataTypes: []string{"text/plain"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for _, version := range []string{"v1", "v2"} {
		wait.Add(1)
		go func(version string) {
			defer wait.Done()
			manifest := base
			manifest.ModelAPIVersion = version
			_, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest)
			errorsOut <- err
		}(version)
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	var profileCount int
	var headRevision int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='jina'`).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT head_revision FROM processor_governance_heads WHERE processor='jina'`).Scan(&headRevision); err != nil {
		t.Fatal(err)
	}
	if profileCount != 2 || headRevision != 2 {
		t.Fatalf("concurrent profiles/head revision = %d/%d", profileCount, headRevision)
	}
	var hashes int
	if err := db.QueryRowContext(ctx, `SELECT count(DISTINCT manifest_hash) FROM processor_governance_profiles WHERE processor='jina' AND manifest_hash<>$1`, strings.Repeat("0", 64)).Scan(&hashes); err != nil {
		t.Fatal(err)
	}
	if hashes != 2 {
		t.Fatalf("distinct manifests = %d", hashes)
	}
}

func TestPostgresRuntimeRemainsCompatibleWithMigration009(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runner := migration.NewRunner(db, migrationfiles.FS)
	if _, err := runner.Up(ctx); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := runner.Down(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolledBack) != 1 || rolledBack[0].Version != 10 {
		t.Fatalf("rolled back migrations = %#v", rolledBack)
	}
	const userID = "99200000-0000-4000-8000-000000000001"
	const collectionID = "99200000-0000-4000-8000-000000000002"
	const fileID = "99200000-0000-4000-8000-000000000003"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name)
VALUES ($1,'runtime-009@example.test','Runtime 009');
INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Runtime 009','personal',$1);
INSERT INTO files(id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata)
VALUES ($3,$1,'runtime.txt','text/plain',1,$4,'local','runtime/009','{"purpose":"knowledge"}')`,
		userID, collectionID, fileID, strings.Repeat("a", 64))
	manifest := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelID: "model-v1",
		ModelAPIVersion: "v1", AllowedPurposes: []string{"parse", "query_embedding"},
		AllowedDataTypes: []string{"text/plain"}, Region: "global", RetentionPolicy: "none",
		DeletionContract: "delete", TrainingUse: "disabled"}
	head, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest)
	if err != nil || head.ModelID != "model-v1" {
		t.Fatalf("009 governance apply = %#v, %v", head, err)
	}
	service := NewService(NewPostgresRepository(db))
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	consent, err := service.PutQueryConsent(actorCtx, "jina", PutConsentInput{
		Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1",
	})
	if err != nil || consent.EndpointID != "default" {
		t.Fatalf("009 query consent = %#v, %v", consent, err)
	}
	if _, err := service.PutCollectionConsent(actorCtx, collectionID, "jina", PutConsentInput{
		Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1",
	}); err != nil {
		t.Fatalf("009 collection consent: %v", err)
	}
	repo := NewPostgresRepository(db)
	if _, err := repo.CreateDocument(ctx, CreateDocumentRepositoryInput{
		DocumentID:   "99200000-0000-4000-8000-000000000004",
		VersionID:    "99200000-0000-4000-8000-000000000005",
		JobID:        "99200000-0000-4000-8000-000000000006",
		CollectionID: collectionID, ActorUserID: userID, FileID: fileID,
		IdempotencyKey: "runtime-009", RequestHash: strings.Repeat("b", 64), ParseProcessor: "jina",
	}); err != nil {
		t.Fatalf("009 compatibility job: %v", err)
	}
	var jobs int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_processing_jobs
WHERE collection_id=$1 AND stage='parse'`, collectionID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("009 compatibility jobs = %d", jobs)
	}
	var exactColumns bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_attribute
WHERE attrelid='processing_consents'::regclass AND attname='model_id' AND NOT attisdropped)`).
		Scan(&exactColumns); err != nil {
		t.Fatal(err)
	}
	if exactColumns {
		t.Fatal("010 exact columns remained after rollback")
	}
}

func TestPostgresGovernanceSameEndpointModelsRemainIndependent(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewGovernanceService(NewPostgresRepository(db))
	manifest := GovernanceManifest{Processor: "jina", EndpointID: "hosted", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"query_embedding"}, AllowedDataTypes: []string{"text/plain"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	for _, modelID := range []string{"embed-a", "embed-b"} {
		manifest.ModelID = modelID
		head, err := service.Apply(ctx, manifest)
		if err != nil || head.ActiveGovernanceRevision != 1 || head.HeadRevision != 1 {
			t.Fatalf("apply %s = %#v, %v", modelID, head, err)
		}
	}
	if _, err := service.Disable(ctx, "jina", "hosted"); err != ErrGovernanceIdentityAmbiguous {
		t.Fatalf("legacy disable error = %v", err)
	}
	head, err := service.DisableModel(ctx, "jina", "hosted", "embed-a")
	if err != nil || head.ModelID != "embed-a" || head.Status != "disabled" {
		t.Fatalf("exact disable = %#v, %v", head, err)
	}
	var activeModels int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_heads
WHERE processor='jina' AND endpoint_id='hosted' AND status='active'`).Scan(&activeModels); err != nil {
		t.Fatal(err)
	}
	if activeModels != 1 {
		t.Fatalf("active models after exact disable = %d", activeModels)
	}
}
