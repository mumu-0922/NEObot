package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

var phase151CBaseMigrationFiles = []string{
	"001_initial_schema.up.sql",
	"001_initial_schema.down.sql",
	"002_messages_run_id_index.up.sql",
	"002_messages_run_id_index.down.sql",
	"003_import_batches.up.sql",
	"003_import_batches.down.sql",
	"004_phase15_identity_knowledge_acl.up.sql",
	"004_phase15_identity_knowledge_acl.down.sql",
}

var phase151CAllMigrationFiles = append(
	append([]string{}, phase151CBaseMigrationFiles...),
	phase151CUpPath,
	phase151CDownPath,
)

type phase151CReplayFixture struct {
	creatorUserID            string
	memberUserID             string
	tempCascadeUserID        string
	teamID                   string
	teamIDConflict           string
	inviteID                 string
	outboxID                 string
	outboxDuplicateID        string
	inviteIDConflict         string
	inviteDuplicatePendingID string
	inviteRevokedID          string
	inviteEmail              string
	inviteTokenHash          string
	inviteTokenHashConflict  string
	inviteTokenHashDuplicate string
	inviteTokenHashRevoked   string
}

func phase151CMigrationFS(t *testing.T, paths ...string) fstest.MapFS {
	t.Helper()

	files := make(fstest.MapFS, len(paths))
	for _, path := range paths {
		contents, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", path, err)
		}
		files[path] = &fstest.MapFile{Data: contents}
	}

	return files
}

func openPhase151CMigrationIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}

	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	adminConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	adminDB := stdlib.OpenDB(*adminConfig)
	t.Cleanup(func() {
		_ = adminDB.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}

	schemaName := fmt.Sprintf("migration_phase15c_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(
		ctx,
		fmt.Sprintf(`CREATE SCHEMA "%s"`, schemaName),
	); err != nil {
		t.Fatalf("create integration schema %s: %v", schemaName, err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		if _, err := adminDB.ExecContext(
			dropCtx,
			fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName),
		); err != nil {
			t.Fatalf("drop integration schema %s: %v", schemaName, err)
		}
	})

	testConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL for schema-bound connection: %v", err)
	}
	testConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	testConfig.RuntimeParams["search_path"] = schemaName

	db := stdlib.OpenDB(*testConfig)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = db.Close()
	})

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		t.Fatalf("ping schema-bound integration database: %v", err)
	}

	return db
}

func seedPhase151CReplayFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) phase151CReplayFixture {
	t.Helper()

	fixture := phase151CReplayFixture{
		creatorUserID:            "00000000-0000-0000-0000-000000000101",
		memberUserID:             "00000000-0000-0000-0000-000000000102",
		tempCascadeUserID:        "00000000-0000-0000-0000-000000000103",
		teamID:                   "00000000-0000-0000-0000-000000000201",
		teamIDConflict:           "00000000-0000-0000-0000-000000000202",
		inviteID:                 "00000000-0000-0000-0000-000000000301",
		outboxID:                 "00000000-0000-0000-0000-000000000401",
		outboxDuplicateID:        "00000000-0000-0000-0000-000000000402",
		inviteIDConflict:         "00000000-0000-0000-0000-000000000302",
		inviteDuplicatePendingID: "00000000-0000-0000-0000-000000000303",
		inviteRevokedID:          "00000000-0000-0000-0000-000000000304",
		inviteEmail:              "invitee@example.test",
		inviteTokenHash:          strings.Repeat("a", 64),
		inviteTokenHashConflict:  strings.Repeat("b", 64),
		inviteTokenHashDuplicate: strings.Repeat("c", 64),
		inviteTokenHashRevoked:   strings.Repeat("d", 64),
	}

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users (id, email, display_name)
VALUES
  ($1, $2, 'Replay Creator'),
  ($3, $4, 'Replay Member')
`,
		fixture.creatorUserID,
		"creator-"+fixture.creatorUserID+"@example.test",
		fixture.memberUserID,
		"member-"+fixture.memberUserID+"@example.test",
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO teams (id, name, created_by_user_id)
VALUES ($1, $2, $3)
`,
		fixture.teamID,
		"Replay Team",
		fixture.creatorUserID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO team_memberships (team_id, user_id, role)
VALUES
  ($1, $2, 'admin'),
  ($1, $3, 'member')
`,
		fixture.teamID,
		fixture.creatorUserID,
		fixture.memberUserID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO team_invites (
  id, team_id, invited_by_user_id, token_hash, email, role, expires_at
) VALUES ($1, $2, $3, $4, $5, 'member', now() + interval '2 hours')
`,
		fixture.inviteID,
		fixture.teamID,
		fixture.creatorUserID,
		fixture.inviteTokenHash,
		fixture.inviteEmail,
	)

	return fixture
}

func mustExecPhase151C(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	query string,
	args ...any,
) {
	t.Helper()

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec query failed: %v\nquery:\n%s", err, query)
	}
}

func mustExecPhase151CReturnError(
	ctx context.Context,
	db *sql.DB,
	query string,
	args ...any,
) error {
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

func assertPhase151CTableExists(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()
	if !phase151CTableExists(ctx, db, table) {
		t.Fatalf("expected table %s to exist in current schema", table)
	}
}

func assertPhase151CTableAbsent(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()
	if phase151CTableExists(ctx, db, table) {
		t.Fatalf("expected table %s to be absent from current schema", table)
	}
}

func phase151CTableExists(ctx context.Context, db *sql.DB, table string) bool {
	var exists bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.tables
  WHERE table_schema = current_schema()
    AND table_name = $1
)
`, table).Scan(&exists); err != nil {
		panic(fmt.Sprintf("query table %s existence: %v", table, err))
	}
	return exists
}

func assertPhase151CColumnExists(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
	column string,
) {
	t.Helper()
	if !phase151CColumnExists(ctx, db, table, column) {
		t.Fatalf("expected %s.%s to exist in current schema", table, column)
	}
}

func assertPhase151CColumnAbsent(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
	column string,
) {
	t.Helper()
	if phase151CColumnExists(ctx, db, table, column) {
		t.Fatalf("expected %s.%s to be absent from current schema", table, column)
	}
}

func phase151CColumnExists(
	ctx context.Context,
	db *sql.DB,
	table string,
	column string,
) bool {
	var exists bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = $1
    AND column_name = $2
)
`, table, column).Scan(&exists); err != nil {
		panic(fmt.Sprintf("query column %s.%s existence: %v", table, column, err))
	}
	return exists
}

func assertPhase151CNullColumnValue(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
	column string,
	id string,
) {
	t.Helper()

	var isNull bool
	query := fmt.Sprintf(
		`SELECT %s IS NULL FROM %s WHERE id = $1`,
		column,
		table,
	)
	if err := db.QueryRowContext(ctx, query, id).Scan(&isNull); err != nil {
		t.Fatalf("query %s.%s nullability for id %s: %v", table, column, id, err)
	}
	if !isNull {
		t.Fatalf("expected %s.%s to stay NULL for pre-005 row %s", table, column, id)
	}
}

func assertPhase151CFKDeleteAction(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
	constraint string,
	want string,
) {
	t.Helper()

	var got string
	if err := db.QueryRowContext(ctx, `
SELECT c.confdeltype::text
FROM pg_constraint c
JOIN pg_class rel ON rel.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = rel.relnamespace
WHERE ns.nspname = current_schema()
  AND rel.relname = $1
  AND c.conname = $2
`, table, constraint).Scan(&got); err != nil {
		t.Fatalf("query delete action for %s.%s: %v", table, constraint, err)
	}
	if got != want {
		t.Fatalf("delete action for %s.%s = %q, want %q", table, constraint, got, want)
	}
}

func assertPhase151CRowCount(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
	want int,
) {
	t.Helper()

	var got int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)
	if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s row count = %d, want %d", table, got, want)
	}
}

func assertPhase151CMembershipAbsent(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	teamID string,
	userID string,
) {
	t.Helper()

	var exists bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM team_memberships
  WHERE team_id = $1
    AND user_id = $2
)
`, teamID, userID).Scan(&exists); err != nil {
		t.Fatalf("query membership existence for %s/%s: %v", teamID, userID, err)
	}
	if exists {
		t.Fatalf("expected membership %s/%s to be absent", teamID, userID)
	}
}

func assertPhase151CUniqueViolation(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected unique violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected unique violation 23505, got %v", err)
	}
}

func assertPhase151CForeignKeyViolation(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("expected foreign key violation 23503, got %v", err)
	}
}

func assertPhase151CPartialUniqueIndex(
	t *testing.T,
	sql string,
	invariant string,
	indexName string,
	table string,
	columns []string,
	predicateFragments []string,
) {
	t.Helper()

	assertPhase151CPartialIndex(t, sql, invariant, indexName, table, columns, predicateFragments)
	if !regexpMustCompile(
		`\bcreate\s+unique\s+index\s+` + regexpQuoteMeta(indexName) + `\b`,
	).MatchString(sql) {
		t.Errorf("%s; index %s must be unique", invariant, indexName)
	}
}

func assertPhase151CPartialIndex(
	t *testing.T,
	sql string,
	invariant string,
	indexName string,
	table string,
	columns []string,
	predicateFragments []string,
) {
	t.Helper()

	pattern := `(?s)\bcreate\s+(?:unique\s+)?index\s+` +
		regexpQuoteMeta(indexName) + `\s+on\s+(?:public\.)?` +
		regexpQuoteMeta(table) + `\s*\(\s*` +
		strings.Join(columns, `\s*,\s*`) + `\s*\)\s+where\s+`
	for index, fragment := range predicateFragments {
		if index > 0 {
			pattern += `[^;]*`
		}
		pattern += regexpQuoteMeta(fragment)
	}
	pattern += `[^;]*;`

	if !regexpMustCompile(pattern).MatchString(sql) {
		t.Errorf("%s; expected partial index %s on %s with predicate fragments %v", invariant, indexName, table, predicateFragments)
	}
}

func regexpMustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

func regexpQuoteMeta(value string) string {
	return regexp.QuoteMeta(value)
}
