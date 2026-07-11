package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresKnowledgeOutboxSourceRecoveryInvariants(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	lowTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lowTx.Rollback()
	var lowID int64
	if err := lowTx.QueryRowContext(ctx, `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ('81000000-0000-4000-8000-000000000001','test','low','test.low','{}')
RETURNING id
`).Scan(&lowID); err != nil {
		t.Fatalf("allocate low outbox id: %v", err)
	}

	highTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var highID int64
	if err := highTx.QueryRowContext(ctx, `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ('81000000-0000-4000-8000-000000000002','test','high','test.high','{}')
RETURNING id
`).Scan(&highID); err != nil {
		_ = highTx.Rollback()
		t.Fatalf("allocate high outbox id: %v", err)
	}
	if highID <= lowID {
		_ = highTx.Rollback()
		t.Fatalf("outbox allocation ids low/high = %d/%d", lowID, highID)
	}
	if err := highTx.Commit(); err != nil {
		t.Fatalf("commit high outbox id first: %v", err)
	}

	var visibleBeforeLow []int64
	rows, err := db.QueryContext(ctx, `SELECT id FROM knowledge_outbox WHERE aggregate_type='test' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		visibleBeforeLow = append(visibleBeforeLow, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(visibleBeforeLow) != 1 || visibleBeforeLow[0] != highID {
		t.Fatalf("visible before low commit = %v, want only high id %d", visibleBeforeLow, highID)
	}
	if err := lowTx.Commit(); err != nil {
		t.Fatalf("commit delayed low outbox id: %v", err)
	}

	rescanConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rescanConn.Close()
	rows, err = rescanConn.QueryContext(ctx, `
SELECT id FROM knowledge_outbox
WHERE aggregate_type='test' AND status='pending'
ORDER BY id
`)
	if err != nil {
		t.Fatal(err)
	}
	var rescanned []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rescanned = append(rescanned, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(rescanned) != 2 || rescanned[0] != lowID || rescanned[1] != highID {
		t.Fatalf("post-commit rescan ids = %v, want [%d %d]", rescanned, lowID, highID)
	}

	_, err = db.ExecContext(ctx, `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ('81000000-0000-4000-8000-000000000001','test','duplicate','test.duplicate','{}')
`)
	assertKnowledgeOutboxConstraint(t, err, "23505", "knowledge_outbox_event_id_key")
	for name, testCase := range map[string]struct {
		query, constraint string
	}{
		"payload array": {query: `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload)
VALUES ('81000000-0000-4000-8000-000000000003','test','invalid','test.invalid','[]')`, constraint: "knowledge_outbox_payload_object"},
		"processing without lock": {query: `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload,status)
VALUES ('81000000-0000-4000-8000-000000000004','test','invalid','test.invalid','{}','processing')`, constraint: "knowledge_outbox_status_timestamps_check"},
		"published without timestamp": {query: `
INSERT INTO knowledge_outbox(event_id,aggregate_type,aggregate_key,event_type,payload,status)
VALUES ('81000000-0000-4000-8000-000000000005','test','invalid','test.invalid','{}','published')`, constraint: "knowledge_outbox_status_timestamps_check"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, testCase.query)
			assertKnowledgeOutboxConstraint(t, err, "23514", testCase.constraint)
		})
	}
}

func assertKnowledgeOutboxConstraint(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("constraint %s error = %v, want PostgreSQL error", constraint, err)
	}
	if pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf("constraint error = code %s constraint %s, want %s/%s", pgErr.Code, pgErr.ConstraintName, code, constraint)
	}
}
