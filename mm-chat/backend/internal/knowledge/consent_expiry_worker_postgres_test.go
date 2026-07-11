package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresConsentExpiryWorkersMaterializeExactlyOnce(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "19000000-0000-4000-8000-000000000001"
	const collectionID = "39000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'expiry@example.test','Expiry'); INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Expiry','personal',$1)`, userID, collectionID)
	manifest := GovernanceManifest{Processor: "mixed", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse", "query_embedding"}, AllowedDataTypes: []string{"text/plain"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	service := NewService(NewPostgresRepository(db))
	expires := time.Now().UTC().Add(250 * time.Millisecond).Format(time.RFC3339Nano)
	if _, err := service.PutCollectionConsent(actorCtx, collectionID, "mixed", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutQueryConsent(actorCtx, "mixed", PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	workerA, _ := NewConsentExpiryWorker(NewPostgresRepository(db), 10, time.Millisecond)
	workerB, _ := NewConsentExpiryWorker(NewPostgresRepository(db), 10, time.Millisecond)
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, worker := range []*ConsentExpiryWorker{workerA, workerB} {
		wait.Add(1)
		go func(worker *ConsentExpiryWorker) { defer wait.Done(); _, err := worker.RunOnce(ctx); results <- err }(worker)
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var materialized, collectionRevision, queryRevision int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE expiry_materialized_at IS NOT NULL`).Scan(&materialized); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&collectionRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userID).Scan(&queryRevision); err != nil {
		t.Fatal(err)
	}
	if materialized != 2 || collectionRevision != 3 || queryRevision != 3 {
		t.Fatalf("materialized/revisions = %d/%d/%d", materialized, collectionRevision, queryRevision)
	}
	collectionValues, err := service.ListCollectionConsents(actorCtx, collectionID)
	if err != nil || len(collectionValues) != 1 || collectionValues[0].EffectiveStatus != "expired" {
		t.Fatalf("expired collection DTO = %#v err=%v", collectionValues, err)
	}
	queryValues, err := service.ListQueryConsents(actorCtx)
	if err != nil || len(queryValues) != 1 || queryValues[0].EffectiveStatus != "expired" {
		t.Fatalf("expired query DTO = %#v err=%v", queryValues, err)
	}
	var expiryEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox
WHERE payload->>'reason'='expired' AND payload->>'effectiveStatus'='expired'
AND payload->>'decision'='granted' AND payload ? 'expiredAt' AND payload ? 'materializedAt'`).Scan(&expiryEvents); err != nil {
		t.Fatal(err)
	}
	if expiryEvents != 2 {
		t.Fatalf("expiry events = %d", expiryEvents)
	}
	runCtx, stop := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- workerA.Run(runCtx) }()
	stop()
	if err := <-runDone; err != context.Canceled {
		t.Fatalf("worker shutdown error = %v", err)
	}
}

func TestPostgresConsentExpiryOutboxFailureRollsBackMarkerAndRevision(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "1a000000-0000-4000-8000-000000000001"
	const collectionID = "3a000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'expiry-rollback@example.test','Rollback'); INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Rollback','personal',$1)`, userID, collectionID)
	manifest := GovernanceManifest{Processor: "mixed", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"text/plain"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	expires := time.Now().UTC().Add(150 * time.Millisecond).Format(time.RFC3339Nano)
	if _, err := NewService(NewPostgresRepository(db)).PutCollectionConsent(actorCtx, collectionID, "mixed", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	var existingEvent string
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox LIMIT 1`).Scan(&existingEvent); err != nil {
		t.Fatal(err)
	}
	failing := NewPostgresRepository(db)
	failing.newEventID = func() (string, error) { return existingEvent, nil }
	worker, _ := NewConsentExpiryWorker(failing, 10, time.Millisecond)
	if _, err := worker.RunOnce(ctx); err == nil {
		t.Fatal("expiry outbox failure error = nil")
	}
	var marker bool
	var revision int
	if err := db.QueryRowContext(ctx, `SELECT expiry_materialized_at IS NOT NULL FROM processing_consents WHERE collection_id=$1 AND superseded_at IS NULL`, collectionID).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if marker || revision != 2 {
		t.Fatalf("failed expiry committed marker/revision = %v/%d", marker, revision)
	}
}

func TestPostgresConsentMutationMaterializesElapsedExpiryFirst(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "1b000000-0000-4000-8000-000000000001"
	const collectionID = "3b000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'expiry-mutation@example.test','Mutation'); INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Mutation','personal',$1)`, userID, collectionID)
	manifest := GovernanceManifest{Processor: "mixed", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"parse", "query_embedding"}, AllowedDataTypes: []string{"text/plain"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	service := NewService(NewPostgresRepository(db))
	expires := time.Now().UTC().Add(150 * time.Millisecond).Format(time.RFC3339Nano)
	if _, err := service.PutCollectionConsent(actorCtx, collectionID, "mixed", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutQueryConsent(actorCtx, "mixed", PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	replayedCollection, err := service.PutCollectionConsent(actorCtx, collectionID, "mixed", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires})
	if err != nil || replayedCollection.EffectiveStatus != "expired" {
		t.Fatalf("elapsed collection replay = %#v err=%v", replayedCollection, err)
	}
	replayedQuery, err := service.PutQueryConsent(actorCtx, "mixed", PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expires})
	if err != nil || replayedQuery.EffectiveStatus != "expired" {
		t.Fatalf("elapsed query replay = %#v err=%v", replayedQuery, err)
	}
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := service.PutCollectionConsent(actorCtx, collectionID, "mixed", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v2", ExpiresAt: future}); err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeQueryConsent(actorCtx, "mixed"); err != nil {
		t.Fatal(err)
	}
	var collectionRevision, queryRevision, expiryEvents int
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&collectionRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userID).Scan(&queryRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE payload->>'reason'='expired'`).Scan(&expiryEvents); err != nil {
		t.Fatal(err)
	}
	if collectionRevision != 4 || queryRevision != 4 || expiryEvents != 2 {
		t.Fatalf("mutation expiry revisions/events = %d/%d/%d", collectionRevision, queryRevision, expiryEvents)
	}
}
