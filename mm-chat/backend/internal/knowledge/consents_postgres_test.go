package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresCollectionConsentACLRevisionIdempotencyAndRollback(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const (
		ownerID              = "13000000-0000-4000-8000-000000000001"
		memberID             = "13000000-0000-4000-8000-000000000002"
		outsiderID           = "13000000-0000-4000-8000-000000000003"
		teamID               = "23000000-0000-4000-8000-000000000001"
		personalID           = "33000000-0000-4000-8000-000000000001"
		teamCollectionID     = "33000000-0000-4000-8000-000000000002"
		rollbackCollectionID = "33000000-0000-4000-8000-000000000003"
	)
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES
($1,'consent-owner@example.test','Owner'),($2,'consent-member@example.test','Member'),($3,'consent-outsider@example.test','Outsider');
INSERT INTO teams(id,name,created_by_user_id) VALUES ($4,'Consent Team',$1);
INSERT INTO team_memberships(team_id,user_id,role) VALUES ($4,$1,'admin'),($4,$2,'member');
INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES
($5,'Personal','personal',$1),($7,'Rollback','personal',$1);
INSERT INTO knowledge_collections(id,name,scope,team_id) VALUES ($6,'Team','team',$4)`,
		ownerID, memberID, outsiderID, teamID, personalID, teamCollectionID, rollbackCollectionID)
	manifest := GovernanceManifest{Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse", "rerank"}, AllowedDataTypes: []string{"application/pdf", "text/plain"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	ownerCtx := auth.WithUser(ctx, auth.User{ID: ownerID})
	memberCtx := auth.WithUser(ctx, auth.User{ID: memberID})
	outsiderCtx := auth.WithUser(ctx, auth.User{ID: outsiderID})
	service := NewService(NewPostgresRepository(db))
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	put := PutConsentInput{Purposes: []string{"rerank", "parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1", ExpiresAt: expires}
	granted, err := service.PutCollectionConsent(ownerCtx, personalID, "mineru", put)
	if err != nil || granted.Decision != "granted" {
		t.Fatalf("grant = %#v err=%v", granted, err)
	}
	if _, err := service.PutCollectionConsent(ownerCtx, personalID, "mineru", put); err != nil {
		t.Fatal(err)
	}
	var processingRevision, events, currentRows int
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, personalID).Scan(&processingRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.consent.changed'`, personalID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if processingRevision != 2 || events != 1 {
		t.Fatalf("no-op revision/events = %d/%d", processingRevision, events)
	}
	var endpointID string
	if err := db.QueryRowContext(ctx, `SELECT payload->>'endpointId' FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.consent.changed'`, personalID).Scan(&endpointID); err != nil {
		t.Fatal(err)
	}
	if endpointID != "default" {
		t.Fatalf("consent event endpoint = %q", endpointID)
	}
	if _, err := service.PutCollectionConsent(ownerCtx, personalID, "mineru", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1", ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)}); err == nil {
		t.Fatal("expired grant error = nil")
	}
	if _, err := service.ListCollectionConsents(outsiderCtx, personalID); err != ErrCollectionNotFound {
		t.Fatalf("outsider list error = %v", err)
	}
	if values, err := service.ListCollectionConsents(memberCtx, teamCollectionID); err != nil || len(values) != 0 {
		t.Fatalf("member list = %#v err=%v", values, err)
	}
	if _, err := service.PutCollectionConsent(memberCtx, teamCollectionID, "mineru", put); err != ErrTeamAdminRequired {
		t.Fatalf("member put error = %v", err)
	}
	if _, err := service.PutCollectionConsent(ownerCtx, teamCollectionID, "mineru", put); err != nil {
		t.Fatalf("admin put: %v", err)
	}
	if values, err := service.ListCollectionConsents(memberCtx, teamCollectionID); err != nil || len(values) != 1 {
		t.Fatalf("member populated list = %#v err=%v", values, err)
	}
	mustKnowledgeExec(t, ctx, db, `UPDATE team_memberships SET status='removed',removed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE team_id=$1 AND user_id=$2`, teamID, memberID)
	if _, err := service.ListCollectionConsents(memberCtx, teamCollectionID); err != ErrCollectionNotFound {
		t.Fatalf("removed member list error = %v", err)
	}
	if err := service.RevokeCollectionConsent(ownerCtx, personalID, "mineru"); err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeCollectionConsent(ownerCtx, personalID, "mineru"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, personalID).Scan(&processingRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE collection_id=$1 AND superseded_at IS NULL`, personalID).Scan(&currentRows); err != nil {
		t.Fatal(err)
	}
	if processingRevision != 3 || currentRows != 1 {
		t.Fatalf("revoke revision/current = %d/%d", processingRevision, currentRows)
	}

	var existingEventID string
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox LIMIT 1`).Scan(&existingEventID); err != nil {
		t.Fatal(err)
	}
	failing := NewPostgresRepository(db)
	generated := 0
	failing.newEventID = func() (string, error) {
		generated++
		if generated == 2 {
			return existingEventID, nil
		}
		return fmt.Sprintf("93000000-0000-4000-8000-%012d", generated), nil
	}
	if _, err := NewService(failing).PutCollectionConsent(ownerCtx, rollbackCollectionID, "mineru", put); err == nil {
		t.Fatal("outbox failure error = nil")
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, rollbackCollectionID).Scan(&processingRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE collection_id=$1`, rollbackCollectionID).Scan(&currentRows); err != nil {
		t.Fatal(err)
	}
	if processingRevision != 1 || currentRows != 0 {
		t.Fatalf("failed grant committed = %d/%d", processingRevision, currentRows)
	}
	if _, err := service.PutCollectionConsent(ownerCtx, rollbackCollectionID, "mineru", put); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox WHERE aggregate_key=$1 LIMIT 1`, rollbackCollectionID).Scan(&existingEventID); err != nil {
		t.Fatal(err)
	}
	failing = NewPostgresRepository(db)
	generated = 0
	failing.newEventID = func() (string, error) {
		generated++
		if generated == 2 {
			return existingEventID, nil
		}
		return fmt.Sprintf("94000000-0000-4000-8000-%012d", generated), nil
	}
	if err := NewService(failing).RevokeCollectionConsent(ownerCtx, rollbackCollectionID, "mineru"); err == nil {
		t.Fatal("revoke outbox failure error = nil")
	}
	var decision string
	if err := db.QueryRowContext(ctx, `SELECT decision FROM processing_consents WHERE collection_id=$1 AND superseded_at IS NULL`, rollbackCollectionID).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, rollbackCollectionID).Scan(&processingRevision); err != nil {
		t.Fatal(err)
	}
	if decision != "granted" || processingRevision != 2 {
		t.Fatalf("failed revoke committed = %s/%d", decision, processingRevision)
	}
}

func TestPostgresConcurrentCollectionConsentGrantIsOneTransition(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "14000000-0000-4000-8000-000000000001"
	const collectionID = "34000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'race-consent@example.test','Owner'); INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Race','personal',$1)`, userID, collectionID)
	manifest := GovernanceManifest{Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"parse", "rerank"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	input := PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1"}
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := NewService(NewPostgresRepository(db)).PutCollectionConsent(actorCtx, collectionID, "mineru", input)
			errorsOut <- err
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	var revision, events int
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.consent.changed'`, collectionID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || events != 1 {
		t.Fatalf("concurrent revision/events = %d/%d", revision, events)
	}
	changed := PutConsentInput{Purposes: []string{"rerank"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1"}
	errorsOut = make(chan error, 2)
	wait = sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, err := NewService(NewPostgresRepository(db)).PutCollectionConsent(actorCtx, collectionID, "mineru", changed)
		errorsOut <- err
	}()
	go func() {
		defer wait.Done()
		errorsOut <- NewService(NewPostgresRepository(db)).RevokeCollectionConsent(actorCtx, collectionID, "mineru")
	}()
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewService(NewPostgresRepository(db)).PutCollectionConsent(actorCtx, collectionID, "mineru", input); err != nil {
		t.Fatal(err)
	}
	errorsOut = make(chan error, 2)
	wait = sync.WaitGroup{}
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsOut <- NewService(NewPostgresRepository(db)).RevokeCollectionConsent(actorCtx, collectionID, "mineru")
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT collection_processing_revision FROM knowledge_collections WHERE id=$1`, collectionID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.collection.consent.changed'`, collectionID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if revision != 6 || events != 5 {
		t.Fatalf("mixed/concurrent revoke revision/events = %d/%d", revision, events)
	}
}

func TestPostgresConsentGrantSeesConcurrentSecondGovernanceEndpoint(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "15000000-0000-4000-8000-000000000001"
	const collectionID = "35000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'phantom-consent@example.test','Owner'); INSERT INTO knowledge_collections(id,name,scope,owner_user_id) VALUES ($2,'Phantom','personal',$1)`, userID, collectionID)
	manifest := GovernanceManifest{Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	governance := NewGovernanceService(NewPostgresRepository(db))
	if _, err := governance.Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockGovernanceProcessor(ctx, blocker, "mineru"); err != nil {
		t.Fatal(err)
	}
	applyDone := make(chan error, 1)
	manifest.EndpointID = "secondary"
	go func() { _, err := governance.Apply(ctx, manifest); applyDone <- err }()
	waitForAdvisoryWaiters(t, ctx, db, 1)
	grantDone := make(chan error, 1)
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	go func() {
		_, err := NewService(NewPostgresRepository(db)).PutCollectionConsent(actorCtx, collectionID, "mineru", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1"})
		grantDone <- err
	}()
	waitForAdvisoryWaiters(t, ctx, db, 2)
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-applyDone; err != nil {
		t.Fatalf("second endpoint apply: %v", err)
	}
	if err := <-grantDone; err != ErrKnowledgeProcessorUnavailable {
		t.Fatalf("grant after endpoint phantom error = %v", err)
	}
	var consents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE collection_id=$1`, collectionID).Scan(&consents); err != nil {
		t.Fatal(err)
	}
	if consents != 0 {
		t.Fatalf("ambiguous endpoint grant inserted consents: %d", consents)
	}
}

func waitForAdvisoryWaiters(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("advisory waiters did not reach %d", want)
}
