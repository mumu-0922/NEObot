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

func TestPostgresQueryConsentIsolationRevisionGovernanceAndRollback(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userA = "16000000-0000-4000-8000-000000000001"
	const userB = "16000000-0000-4000-8000-000000000002"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES
($1,'query-a@example.test','A'),($2,'query-b@example.test','B')`, userA, userB)
	manifest := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"query_embedding", "rerank"}, AllowedDataTypes: []string{"text/plain"},
		Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	governance := NewGovernanceService(NewPostgresRepository(db))
	if _, err := governance.Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	ctxA := auth.WithUser(ctx, auth.User{ID: userA})
	ctxB := auth.WithUser(ctx, auth.User{ID: userB})
	service := NewService(NewPostgresRepository(db))
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	input := PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: expiry.Format(time.RFC3339Nano)}
	if _, err := service.PutQueryConsent(ctxA, "jina", input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutQueryConsent(ctxA, "jina", input); err != nil {
		t.Fatal(err)
	}
	if values, err := service.ListQueryConsents(ctxB); err != nil || len(values) != 0 {
		t.Fatalf("user B list = %#v err=%v", values, err)
	}
	var stateRevision, events int
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userA).Scan(&stateRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.user.query-consent.changed'`, userA).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if stateRevision != 2 || events != 1 {
		t.Fatalf("initial/no-op revision/events = %d/%d", stateRevision, events)
	}
	var tupleOK bool
	if err := db.QueryRowContext(ctx, `SELECT payload->>'endpointId'='default'
AND (payload->>'governanceRevision')::bigint=1 AND (payload->>'governanceHeadRevision')::bigint=1
AND (payload->>'consentRevision')::bigint=1 AND (payload->>'queryConsentRevision')::bigint=2
AND length(payload->>'governanceProfileId')=36 FROM knowledge_outbox
WHERE aggregate_key=$1 AND event_type='knowledge.user.query-consent.changed' ORDER BY id LIMIT 1`, userA).Scan(&tupleOK); err != nil {
		t.Fatal(err)
	}
	if !tupleOK {
		t.Fatal("query consent outbox authority tuple is inconsistent")
	}
	manifest.ModelAPIVersion = "v2"
	if _, err := governance.Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutQueryConsent(ctxA, "jina", input); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userA).Scan(&stateRevision); err != nil {
		t.Fatal(err)
	}
	if stateRevision != 3 {
		t.Fatalf("governance reconsent revision = %d", stateRevision)
	}
	if err := service.RevokeQueryConsent(ctxA, "jina"); err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeQueryConsent(ctxA, "jina"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userA).Scan(&stateRevision); err != nil {
		t.Fatal(err)
	}
	if stateRevision != 4 {
		t.Fatalf("revoke/no-op revision = %d", stateRevision)
	}
	var existingEventID string
	if _, err := service.PutQueryConsent(ctxA, "jina", input); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userA).Scan(&stateRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox WHERE aggregate_key=$1 ORDER BY id LIMIT 1`, userA).Scan(&existingEventID); err != nil {
		t.Fatal(err)
	}
	failingRevoke := NewPostgresRepository(db)
	generatedRevoke := 0
	failingRevoke.newEventID = func() (string, error) {
		generatedRevoke++
		if generatedRevoke == 2 {
			return existingEventID, nil
		}
		return fmt.Sprintf("96000000-0000-4000-8000-%012d", generatedRevoke), nil
	}
	if err := NewService(failingRevoke).RevokeQueryConsent(ctxA, "jina"); err == nil {
		t.Fatal("query revoke outbox failure error = nil")
	}
	var decision string
	var afterFailedRevoke int
	if err := db.QueryRowContext(ctx, `SELECT decision FROM processing_consents WHERE user_id=$1 AND processor='jina' AND superseded_at IS NULL`, userA).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userA).Scan(&afterFailedRevoke); err != nil {
		t.Fatal(err)
	}
	if decision != "granted" || afterFailedRevoke != stateRevision {
		t.Fatalf("failed revoke committed = %s/%d->%d", decision, stateRevision, afterFailedRevoke)
	}
	// A separate User proves failed first grant rolls back both state and history.
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
		return fmt.Sprintf("95000000-0000-4000-8000-%012d", generated), nil
	}
	if _, err := NewService(failing).PutQueryConsent(ctxB, "jina", input); err == nil {
		t.Fatal("query outbox failure error = nil")
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE user_id=$1`, userB).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("failed query consent committed rows = %d", rows)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM user_query_consent_state WHERE user_id=$1`, userB).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("failed query consent committed state = %d", rows)
	}
	if _, err := service.PutQueryConsent(ctxA, "jina", PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1", ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)}); err == nil {
		t.Fatal("expired query grant error = nil")
	}
	var endpoint string
	if err := db.QueryRowContext(ctx, `SELECT payload->>'endpointId' FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.user.query-consent.changed' LIMIT 1`, userA).Scan(&endpoint); err != nil {
		t.Fatal(err)
	}
	if endpoint != "default" {
		t.Fatalf("query consent event endpoint = %q", endpoint)
	}
}

func TestPostgresConcurrentQueryConsentGrantIsOneTransition(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "17000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'query-race@example.test','Race')`, userID)
	manifest := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"query_embedding", "rerank"}, AllowedDataTypes: []string{"text/plain"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	input := PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1"}
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := NewService(NewPostgresRepository(db)).PutQueryConsent(actorCtx, "jina", input)
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
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.user.query-consent.changed'`, userID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || events != 1 {
		t.Fatalf("concurrent query revision/events = %d/%d", revision, events)
	}
	changed := PutConsentInput{Purposes: []string{"rerank"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1"}
	errorsOut = make(chan error, 2)
	wait = sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, err := NewService(NewPostgresRepository(db)).PutQueryConsent(actorCtx, "jina", changed)
		errorsOut <- err
	}()
	go func() {
		defer wait.Done()
		errorsOut <- NewService(NewPostgresRepository(db)).RevokeQueryConsent(actorCtx, "jina")
	}()
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewService(NewPostgresRepository(db)).PutQueryConsent(actorCtx, "jina", input); err != nil {
		t.Fatal(err)
	}
	errorsOut = make(chan error, 2)
	wait = sync.WaitGroup{}
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsOut <- NewService(NewPostgresRepository(db)).RevokeQueryConsent(actorCtx, "jina")
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT query_consent_revision FROM user_query_consent_state WHERE user_id=$1`, userID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key=$1 AND event_type='knowledge.user.query-consent.changed'`, userID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if revision != 6 || events != 5 {
		t.Fatalf("mixed query revision/events = %d/%d", revision, events)
	}
}

func TestPostgresAccountDisableWinsQueuedQueryConsentGrant(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	const userID = "18000000-0000-4000-8000-000000000001"
	mustKnowledgeExec(t, ctx, db, `INSERT INTO users(id,email,display_name) VALUES ($1,'query-disable@example.test','Disable')`, userID)
	manifest := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelAPIVersion: "v1", AllowedPurposes: []string{"query_embedding"}, AllowedDataTypes: []string{"text/plain"}, Region: "global", RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	if _, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lockedUser string
	if err := blocker.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedUser); err != nil {
		t.Fatal(err)
	}
	disableDone := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(ctx, `UPDATE users SET account_status='disabled',updated_at=clock_timestamp() WHERE id=$1`, userID)
		disableDone <- err
	}()
	waitForDatabaseLockWaiters(t, ctx, db, 1)
	grantDone := make(chan error, 1)
	actorCtx := auth.WithUser(ctx, auth.User{ID: userID})
	go func() {
		_, err := NewService(NewPostgresRepository(db)).PutQueryConsent(actorCtx, "jina", PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1"})
		grantDone <- err
	}()
	waitForDatabaseLockWaiters(t, ctx, db, 2)
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-disableDone; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if err := <-grantDone; err != ErrUnauthenticated {
		t.Fatalf("grant after disable error = %v", err)
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_consents WHERE user_id=$1`, userID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("disabled user inserted query consents: %d", rows)
	}
}

func waitForDatabaseLockWaiters(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("database lock waiters did not reach %d", want)
}
