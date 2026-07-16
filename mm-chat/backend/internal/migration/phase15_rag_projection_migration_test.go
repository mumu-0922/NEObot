package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPhase15RAGProjectionSQLIsExtensionIndependent(t *testing.T) {
	contents, err := fs.ReadFile(
		migrationfiles.FS,
		"010_phase15_rag_projection_consistency.up.sql",
	)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(contents))
	for _, forbidden := range []string{
		"create extension",
		"pg_search",
		"halfvec",
		"vector(",
		"vector (",
		"tokenizer",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("010 contains extension-specific DDL marker %q", forbidden)
		}
	}
	for _, required := range []string{
		"set search_path from current",
		"revoke all on function knowledge_claim_outbox",
		"knowledge_outbox_applied_events",
		"knowledge_collection_purge_items",
		"legacy_projection_unbound",
		"rag_stale_outbox_lease",
		"rag_stale_job_lease",
		"go_api_runtime",
		"rolcanlogin or rolsuper or rolcreatedb or rolcreaterole",
		"or rolreplication or rolbypassrls",
		"grant usage, select on sequence knowledge_outbox_id_seq",
		"to rag_api_reader, go_api_runtime",
		") to go_evidence_hydrator, go_api_runtime",
	} {
		if !strings.Contains(lower, required) {
			t.Errorf("010 missing %q", required)
		}
	}
}

func TestPhase15RAGProjectionCapabilityRolePreflightFailsClosed(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	assertPhase15Postgres16(t, ctx, db)

	var superuser bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT rolsuper FROM pg_roles WHERE rolname = current_user`, &superuser)
	if !superuser {
		t.Skip("capability role attribute mutation requires a PostgreSQL superuser")
	}

	mustExecPhase151C(t, ctx, db, `
DO $roles$
DECLARE role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY ARRAY[
    'rag_projection_owner', 'rag_worker_executor', 'rag_replay_operator',
    'rag_api_reader', 'go_evidence_hydrator', 'go_api_runtime'
  ] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name
      );
    END IF;
  END LOOP;
END
$roles$`)
	assertPhase15CapabilityRolesRestricted(t, ctx, db)
	assertPhase15GoAPINoDangerousMembership(t, ctx, db)

	restoreReader := func() {
		restoreCtx, restoreCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer restoreCancel()
		if _, err := db.ExecContext(restoreCtx, `
ALTER ROLE rag_api_reader
  NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS`); err != nil {
			t.Errorf("restore rag_api_reader attributes: %v", err)
		}
	}
	t.Cleanup(restoreReader)
	mustExecPhase151C(t, ctx, db, `
ALTER ROLE rag_api_reader
  NOLOGIN SUPERUSER CREATEDB CREATEROLE REPLICATION BYPASSRLS`)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 10))
	if _, err := runner.Up(ctx); err == nil ||
		!strings.Contains(err.Error(), "RAG_REQUIRED_ROLE_MUST_BE_RESTRICTED") {
		t.Fatalf("unsafe role attributes error = %v", err)
	}
	restoreReader()

	const inheritedRole = "phase15_test_inherited_capability"
	mustExecPhase151C(t, ctx, db, `DROP ROLE IF EXISTS `+inheritedRole)
	mustExecPhase151C(t, ctx, db, `CREATE ROLE `+inheritedRole+` NOLOGIN`)
	dropInheritedRole := func() {
		restoreCtx, restoreCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer restoreCancel()
		_, _ = db.ExecContext(restoreCtx,
			`REVOKE `+inheritedRole+` FROM rag_api_reader`)
		_, _ = db.ExecContext(restoreCtx, `DROP ROLE IF EXISTS `+inheritedRole)
	}
	t.Cleanup(dropInheritedRole)
	mustExecPhase151C(t, ctx, db,
		`GRANT `+inheritedRole+` TO rag_api_reader`)
	if _, err := runner.Up(ctx); err == nil ||
		!strings.Contains(err.Error(), "RAG_REQUIRED_ROLE_MUST_NOT_INHERIT_MEMBERSHIP") {
		t.Fatalf("inherited capability membership error = %v", err)
	}
	dropInheritedRole()

	mustExecPhase151C(t, ctx, db,
		`GRANT rag_worker_executor TO go_api_runtime`)
	revokeWorker := func() {
		revokeCtx, revokeCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer revokeCancel()
		if _, err := db.ExecContext(revokeCtx,
			`REVOKE rag_worker_executor FROM go_api_runtime`); err != nil {
			t.Errorf("revoke forbidden test membership: %v", err)
		}
	}
	t.Cleanup(revokeWorker)
	if _, err := runner.Up(ctx); err == nil ||
		!strings.Contains(err.Error(), "RAG_GO_API_RUNTIME_FORBIDDEN_MEMBERSHIP") {
		t.Fatalf("forbidden membership error = %v", err)
	}
	revokeWorker()

	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("up after safe role restoration: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != 10 {
		t.Fatalf("applied after role restoration = %#v", applied)
	}
	assertPhase15CapabilityRolesRestricted(t, ctx, db)
	assertPhase15GoAPINoDangerousMembership(t, ctx, db)
	assertPhase15ReadinessCapability(t, ctx, db, "rag_worker_executor", true)
	assertPhase15ReadinessCapability(t, ctx, db, "go_api_runtime", false)
}

func TestPhase15RAGProjectionGoAPIRuntimePermissionsSurviveRollback(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	assertPhase15Postgres16(t, ctx, db)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 10))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatalf("configure zero-value mapping: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("fresh up: %v", err)
	}
	assertPhase15CapabilityRolesRestricted(t, ctx, db)
	assertPhase15GoAPINoDangerousMembership(t, ctx, db)
	assertPhase15GoAPIBasePrivileges(t, ctx, db)
	assertPhase15GoAPIProjectionBoundary(t, ctx, db)

	rolledBack, err := runner.Down(ctx, false)
	if err != nil {
		t.Fatalf("rollback 010: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].Version != 10 {
		t.Fatalf("rolled back = %#v", rolledBack)
	}
	assertPhase15GoAPIBasePrivileges(t, ctx, db)
	if err := execPhase15AsRole(ctx, db, "go_api_runtime", `
SELECT * FROM knowledge_rag_worker_readiness()`); !phase15UndefinedFunction(err) {
		t.Fatalf("projection function survived down: %v", err)
	}

	reapplied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("reapply 010: %v", err)
	}
	if len(reapplied) != 1 || reapplied[0].Version != 10 {
		t.Fatalf("reapplied = %#v", reapplied)
	}
	assertPhase15GoAPIBasePrivileges(t, ctx, db)
	assertPhase15GoAPIProjectionBoundary(t, ctx, db)
}

func TestPhase15RAGProjectionFreshLeaseLedgerReplayAndRollback(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	runner := NewRunner(db, phase15MigrationFSThrough(t, 10))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatalf("configure zero-value mapping: %v", err)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("fresh up: %v", err)
	}
	if len(applied) != 10 || applied[9].Version != 10 {
		t.Fatalf("fresh applied = %#v, want 001-010", applied)
	}
	var publicExecuteCount int
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT count(*)
FROM pg_proc function
JOIN pg_namespace namespace ON namespace.oid = function.pronamespace,
LATERAL aclexplode(COALESCE(
  function.proacl, acldefault('f', function.proowner)
)) privilege
WHERE namespace.nspname = current_schema()
  AND function.proname IN (
    'knowledge_claim_outbox',
    'knowledge_apply_and_ack_outbox',
    'knowledge_replay_outbox',
    'knowledge_reauthorize_and_hydrate_evidence'
  )
  AND privilege.grantee = 0
  AND privilege.privilege_type = 'EXECUTE'`, &publicExecuteCount)
	if publicExecuteCount != 0 {
		t.Fatalf("PUBLIC retains EXECUTE on %d protected functions", publicExecuteCount)
	}
	var securityDefinerCount, safeSearchPathCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*), count(*) FILTER (
  WHERE COALESCE(function.proconfig @> ARRAY[
    'search_path=' || quote_ident(current_schema()) ||
      ', pg_catalog, pg_temp'
  ], false)
)
FROM pg_proc function
JOIN pg_namespace namespace ON namespace.oid = function.pronamespace
WHERE namespace.nspname = current_schema()
  AND function.prosecdef
  AND function.proname LIKE 'knowledge_%'`).Scan(
		&securityDefinerCount,
		&safeSearchPathCount,
	); err != nil {
		t.Fatalf("read function proconfig: %v", err)
	}
	if securityDefinerCount == 0 || safeSearchPathCount != securityDefinerCount {
		t.Fatalf(
			"safe SECURITY DEFINER proconfig = %d/%d",
			safeSearchPathCount,
			securityDefinerCount,
		)
	}
	roleTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roleTx.ExecContext(ctx, `SET LOCAL ROLE rag_worker_executor`); err != nil {
		_ = roleTx.Rollback()
		t.Fatalf("set worker role: %v", err)
	}
	if _, err := roleTx.ExecContext(ctx, `SELECT count(*) FROM knowledge_outbox`); err == nil {
		_ = roleTx.Rollback()
		t.Fatal("rag_worker_executor can read the outbox base table")
	}
	_ = roleTx.Rollback()

	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_outbox(
  event_id, aggregate_type, aggregate_key, event_type, payload, available_at
)
VALUES
 ('00000000-0000-0000-0000-000000001001', 'document', 'd1', 'changed', '{}', now()),
 ('00000000-0000-0000-0000-000000001002', 'document', 'd2', 'changed', '{}', now() + interval '1 day'),
 ('00000000-0000-0000-0000-000000001003', 'document', 'd3', 'changed', '{}', now() + interval '1 day')`)

	worker := "00000000-0000-0000-0000-000000002001"
	oldToken := "00000000-0000-0000-0000-000000003001"
	newToken := "00000000-0000-0000-0000-000000003002"
	var eventID string
	attackConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := attackConn.ExecContext(ctx, `
CREATE TEMP TABLE knowledge_outbox(id BIGINT)`); err != nil {
		_ = attackConn.Close()
		t.Fatalf("create temp attack table: %v", err)
	}
	if err := attackConn.QueryRowContext(ctx, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		worker, oldToken,
	).Scan(&eventID); err != nil {
		_ = attackConn.Close()
		t.Fatalf("claim outbox: %v", err)
	}
	if _, err := attackConn.ExecContext(ctx, `DROP TABLE pg_temp.knowledge_outbox`); err != nil {
		_ = attackConn.Close()
		t.Fatalf("drop temp attack table: %v", err)
	}
	if err := attackConn.Close(); err != nil {
		t.Fatalf("close temp attack connection: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_outbox
SET locked_at = created_at,
    lock_expires_at = created_at + interval '1 microsecond'
WHERE event_id = $1`, eventID)
	if err := db.QueryRowContext(ctx, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		worker, newToken,
	).Scan(&eventID); err != nil {
		t.Fatalf("reclaim outbox: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT knowledge_retry_outbox($1, $2, $3, 'TRANSIENT', 1)`,
		eventID, worker, oldToken,
	); err == nil || !strings.Contains(err.Error(), "RAG_STALE_OUTBOX_LEASE") {
		t.Fatalf("old token error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_apply_and_ack_outbox(
  'rag-v1', $1, $2, $3, 'global', NULL, 'dispatch', $4
)`, eventID, worker, newToken, strings.Repeat("a", 64))

	// Duplicate delivery with the same ledger hash is idempotent; a different
	// hash is isolated and cannot acknowledge the event.
	sameHashToken := "00000000-0000-0000-0000-000000003004"
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_outbox SET status = 'pending', published_at = NULL,
  available_at = clock_timestamp(), updated_at = clock_timestamp()
WHERE event_id = $1`, eventID)
	var duplicateEvent string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		&duplicateEvent, worker, sameHashToken)
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_apply_and_ack_outbox(
  'rag-v1', $1, $2, $3, 'global', NULL, 'dispatch', $4
)`, duplicateEvent, worker, sameHashToken, strings.Repeat("a", 64))

	conflictToken := "00000000-0000-0000-0000-000000003005"
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_outbox SET status = 'pending', published_at = NULL,
  available_at = clock_timestamp(), updated_at = clock_timestamp()
WHERE event_id = $1`, eventID)
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		&duplicateEvent, worker, conflictToken)
	if _, err := db.ExecContext(ctx, `
SELECT knowledge_apply_and_ack_outbox(
  'rag-v1', $1, $2, $3, 'global', NULL, 'dispatch', $4
)`, duplicateEvent, worker, conflictToken, strings.Repeat("f", 64)); err == nil || !strings.Contains(err.Error(), "RAG_APPLIED_LEDGER_CONFLICT") {
		t.Fatalf("ledger conflict error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_fail_outbox($1, $2, $3, 'RESULT_CONFLICT')`,
		duplicateEvent, worker, conflictToken)

	var firstID, secondID int64
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_outbox WHERE event_id =
  '00000000-0000-0000-0000-000000001001'`, &firstID)
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_outbox WHERE event_id =
  '00000000-0000-0000-0000-000000001002'`, &secondID)
	if firstID == secondID {
		t.Fatal("outbox ids unexpectedly equal")
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO knowledge_outbox_applied_events(
  consumer_name, event_id, scope_kind, result_hash, outbox_id
) VALUES ('pair-test', '00000000-0000-0000-0000-000000001001',
  'global', $1, $2)`, strings.Repeat("b", 64), secondID); err == nil {
		t.Fatal("ledger accepted mismatched outbox id/event id")
	}

	replayToken := "00000000-0000-0000-0000-000000003003"
	var replayEvent string
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_outbox SET available_at = clock_timestamp()
WHERE event_id = '00000000-0000-0000-0000-000000001003'`)
	if err := db.QueryRowContext(ctx, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		worker, replayToken,
	).Scan(&replayEvent); err != nil {
		t.Fatalf("claim replay event: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_fail_outbox($1, $2, $3, 'PROVIDER_DOWN')`,
		replayEvent, worker, replayToken)
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_replay_outbox($1, 'PROVIDER_DOWN', $2, 'operator replay')`,
		replayEvent, "00000000-0000-0000-0000-000000004001")
	var status string
	mustQueryPhase15RAGRow(t, ctx, db,
		`SELECT status FROM knowledge_outbox WHERE event_id = $1`, &status, replayEvent)
	if status != "pending" {
		t.Fatalf("replayed status = %q", status)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_outbox SET available_at = clock_timestamp() + interval '1 day'
WHERE event_id = $1;
INSERT INTO users(id, display_name)
VALUES ('00000000-0000-0000-0000-000000004010', 'purge owner');
INSERT INTO knowledge_collections(
  id, name, scope, owner_user_id, visibility_epoch, deleted_at
) VALUES (
  '00000000-0000-0000-0000-000000004011', 'purged', 'personal',
  '00000000-0000-0000-0000-000000004010', 2, clock_timestamp()
);
INSERT INTO knowledge_outbox(
  event_id, aggregate_type, aggregate_key, event_type, payload
) VALUES (
  '00000000-0000-0000-0000-000000004012', 'knowledge_collection',
  '00000000-0000-0000-0000-000000004011',
  'knowledge.collection.tombstoned',
  '{"collectionId":"00000000-0000-0000-0000-000000004011","visibilityEpoch":2}'
)`, replayEvent)
	purgeToken := "00000000-0000-0000-0000-000000004013"
	var purgeEvent string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT event_id FROM knowledge_claim_outbox('rag-v1', $1, $2, 30)`,
		&purgeEvent, worker, purgeToken)
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_apply_and_ack_outbox(
  'rag-v1', $1, $2, $3, 'global', NULL, 'collection_purge', $4
)`, purgeEvent, worker, purgeToken, strings.Repeat("e", 64))
	var purgeRoots int
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT count(*) FROM knowledge_collection_purges WHERE source_event_id = $1`,
		&purgeRoots, purgeEvent)
	if purgeRoots != 1 {
		t.Fatalf("purge roots = %d", purgeRoots)
	}
	purgeRootToken := "00000000-0000-0000-0000-000000004014"
	var purgeRootID string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_claim_collection_purge($1, $2, 30)`,
		&purgeRootID, worker, purgeRootToken)
	if purgeRootID != purgeEvent {
		t.Fatalf("claimed purge root = %s, want %s", purgeRootID, purgeEvent)
	}
	var enumerationComplete bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT knowledge_enumerate_collection_purge($1, $2, $3, 30, 100)`,
		&enumerationComplete, purgeRootID, worker, purgeRootToken)
	if !enumerationComplete {
		t.Fatal("empty collection purge enumeration did not complete")
	}
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_complete_collection_purge($1)`, purgeRootID)
	var purgeStatus string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT status FROM knowledge_collection_purges WHERE id=$1`,
		&purgeStatus, purgeRootID)
	if purgeStatus != "succeeded" {
		t.Fatalf("completed purge root status = %q", purgeStatus)
	}

	// Clear test effects so conservative down can prove a lossless rollback.
	mustExecPhase151C(t, ctx, db, `DELETE FROM knowledge_collection_purges`)
	mustExecPhase151C(t, ctx, db, `DELETE FROM knowledge_outbox_replays`)
	mustExecPhase151C(t, ctx, db, `DELETE FROM knowledge_outbox_applied_events`)
	mustExecPhase151C(t, ctx, db, `DELETE FROM knowledge_outbox`)
	mustExecPhase151C(t, ctx, db, `
DELETE FROM knowledge_collections
WHERE id = '00000000-0000-0000-0000-000000004011';
DELETE FROM users WHERE id = '00000000-0000-0000-0000-000000004010'`)
	mustExecPhase151C(t, ctx, db, `CREATE TEMP TABLE knowledge_index_profiles(id UUID)`)
	rolledBack, err := runner.Down(ctx, false)
	if err != nil {
		t.Fatalf("rollback 010: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].Version != 10 {
		t.Fatalf("rolled back = %#v", rolledBack)
	}
	var tempAttackTableExists bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT to_regclass('pg_temp.knowledge_index_profiles') IS NOT NULL`,
		&tempAttackTableExists)
	if !tempAttackTableExists {
		t.Fatal("down followed pg_temp shadow instead of the trusted schema")
	}
	mustExecPhase151C(t, ctx, db, `DROP TABLE pg_temp.knowledge_index_profiles`)
}

func TestPhase15RAGProjectionJobLeaseRejectsStaleToken(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	runner := NewRunner(db, phase15MigrationFSThrough(t, 10))
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("fresh up: %v", err)
	}

	const (
		userID       = "00000000-0000-0000-0000-000000010001"
		fileID       = "00000000-0000-0000-0000-000000010002"
		collectionID = "00000000-0000-0000-0000-000000010003"
		documentID   = "00000000-0000-0000-0000-000000010004"
		versionID    = "00000000-0000-0000-0000-000000010005"
		profileID    = "00000000-0000-0000-0000-000000010006"
		consentID    = "00000000-0000-0000-0000-000000010007"
		indexProfile = "00000000-0000-0000-0000-000000010008"
		generationID = "00000000-0000-0000-0000-000000010009"
		materialID   = "00000000-0000-0000-0000-000000010010"
		jobID        = "00000000-0000-0000-0000-000000010011"
	)
	hash := strings.Repeat("a", 64)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'worker fixture');
INSERT INTO files(id, user_id, original_filename, byte_size, sha256, object_key)
  VALUES ($2, $1, 'fixture.txt', 1, $12, 'fixture/phase15.txt');
INSERT INTO knowledge_collections(id, name, scope, owner_user_id)
  VALUES ($3, 'fixture', 'personal', $1);
INSERT INTO knowledge_documents(id, collection_id) VALUES ($4, $3);
INSERT INTO knowledge_document_versions(
  id, document_id, file_id, source_version, content_hash
) VALUES ($5, $4, $2, 1, $12);
INSERT INTO processor_governance_profiles(
  id, processor, endpoint_id, model_id, model_api_version,
  profile_contract_hash, allowed_purposes, allowed_data_types, region,
  retention_policy, deletion_contract, training_use, status,
  governance_revision, manifest_hash
) VALUES (
  $6, 'jina', 'hosted', 'embedding-v4', 'v1', $12,
  ARRAY['parse'], ARRAY['text'], 'global', 'none', 'delete', 'none',
  'approved', 1, $12
);
INSERT INTO processor_governance_heads(
  processor, endpoint_id, model_id, status, active_profile_id,
  active_governance_revision, head_revision
) VALUES ('jina', 'hosted', 'embedding-v4', 'active', $6, 1, 1);
INSERT INTO processing_consents(
  id, scope, collection_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id
) VALUES (
  $7, 'collection', $3, 'jina', 'hosted', 'embedding-v4',
  $6, 1, 1, ARRAY['parse'], ARRAY['text'], 'v1', 'granted', 1, $1
);
INSERT INTO knowledge_index_profiles(
  id, contract_version, canonical_schema_version,
  parser_manifest, parser_manifest_hash, chunk_manifest, chunk_profile_hash,
  embedding_processor, embedding_endpoint_id, embedding_model_id,
  embedding_api_version, embedding_role, rerank_processor,
  rerank_endpoint_id, rerank_model_id, rerank_api_version, base_profile_hash
) VALUES (
  $8, 1, 'v1', '{}', $12, '{}', $12, 'jina', 'hosted', 'embedding-v4',
  'v1', 'passage', 'jina', 'hosted', 'rerank-v1', 'v1', $12
);
INSERT INTO knowledge_index_generations(
  id, index_profile_id, generation_seq, status,
  build_snapshot, build_snapshot_hash
) VALUES ($9, $8, 1, 'building', '{}', $12);
INSERT INTO knowledge_document_materializations(
  id, index_generation_id, collection_id, document_id,
  document_version_id, file_id, materialization_seq,
  source_content_hash, base_profile_hash, collection_acl_revision,
  collection_visibility_epoch, collection_processing_revision,
  document_visibility_epoch, status
) VALUES ($10, $9, $3, $4, $5, $2, 1, $12, $12, 1, 1, 1, 1, 'staging');
INSERT INTO knowledge_processing_jobs(
  id, collection_id, document_id, document_version_id, file_id,
  stage, operation, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  collection_consent_id, collection_consent_revision,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  idempotency_scope, idempotency_key, request_hash,
  index_generation_id, materialization_id, legacy_projection_unbound
) VALUES (
  $11, $3, $4, $5, $2, 'parse', 'initial', 'jina', 'hosted',
  'embedding-v4', $6, 1, 1, $7, 1, 1, 1, 1, 1,
  'fixture', 'job-1', $12, $9, $10, false
)`, userID, fileID, collectionID, documentID, versionID, profileID,
		consentID, indexProfile, generationID, materialID, jobID, hash)
	purgeRootID := "00000000-0000-0000-0000-000000010012"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_outbox(
  event_id, aggregate_type, aggregate_key, event_type, payload
) VALUES ($1, 'knowledge_collection', $2, 'knowledge.collection.tombstoned', '{}');
INSERT INTO knowledge_collection_purges(
  id, collection_id, collection_visibility_epoch, source_event_id
) VALUES ($1, $2, 2, $1)`, purgeRootID, collectionID)
	purgeRootWorker := "00000000-0000-0000-0000-000000010013"
	purgeRootToken := "00000000-0000-0000-0000-000000010014"
	var claimedPurgeRoot string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_claim_collection_purge($1, $2, 30)`,
		&claimedPurgeRoot, purgeRootWorker, purgeRootToken)
	var purgeEnumerationComplete bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT knowledge_enumerate_collection_purge($1, $2, $3, 30, 1)`,
		&purgeEnumerationComplete, purgeRootID, purgeRootWorker, purgeRootToken)
	if !purgeEnumerationComplete {
		t.Fatal("single-materialization purge enumeration did not complete")
	}
	var purgeItems int
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT count(*) FROM knowledge_collection_purge_items
WHERE purge_id=$1 AND materialization_id=$2`, &purgeItems, purgeRootID, materialID)
	if purgeItems != 1 {
		t.Fatalf("enumerated purge items = %d, want 1", purgeItems)
	}

	worker := "00000000-0000-0000-0000-000000010020"
	oldToken := "00000000-0000-0000-0000-000000010021"
	newToken := "00000000-0000-0000-0000-000000010022"
	var claimed string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_claim_processing_job($1, $2, 30, ARRAY['parse'])`,
		&claimed, worker, oldToken)
	if claimed != jobID {
		t.Fatalf("claimed job = %s", claimed)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_processing_jobs
SET lease_expires_at = created_at + interval '1 microsecond'
WHERE id = $1`, jobID)
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT id FROM knowledge_claim_processing_job($1, $2, 30, ARRAY['parse'])`,
		&claimed, worker, newToken)
	if _, err := db.ExecContext(ctx, `
SELECT knowledge_heartbeat_processing_job($1, $2, $3, 30)`,
		jobID, worker, oldToken); err == nil ||
		!strings.Contains(err.Error(), "RAG_STALE_JOB_LEASE") {
		t.Fatalf("old job token error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_finish_processing_job($1, $2, $3, 'succeeded', NULL, 0)`,
		jobID, worker, newToken)
	legacyJobID := "00000000-0000-0000-0000-000000010030"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_processing_jobs(
  id, collection_id, document_id, document_version_id, file_id,
  stage, operation, collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  idempotency_scope, idempotency_key, request_hash, status,
  attempt_count, max_attempts, completed_at, error_code,
  legacy_projection_unbound
) VALUES (
  $1, $2, $3, $4, $5, 'purge', 'purge', 1, 1, 1, 1,
  'legacy-replay-test', 'legacy-replay-test', $6, 'failed',
  1, 8, clock_timestamp(), 'LEGACY_FAILED', true
)`, legacyJobID, collectionID, documentID, versionID, fileID, hash)
	if _, err := db.ExecContext(ctx, `
SELECT knowledge_replay_processing_job(
  $1, 'LEGACY_FAILED', $2, $3, 'legacy jobs must reconcile instead'
)`, legacyJobID,
		"00000000-0000-0000-0000-000000010031",
		"00000000-0000-0000-0000-000000010032"); err == nil ||
		!strings.Contains(err.Error(), "RAG_REPLAY_PRECONDITION_FAILED") {
		t.Fatalf("legacy replay error = %v", err)
	}
	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "RAG_DOWN_BOUND_JOB_EXISTS") {
		t.Fatalf("unsafe rollback error = %v", err)
	}
	var migrationCount int
	mustQueryPhase15RAGRow(t, ctx, db,
		`SELECT count(*) FROM schema_migrations WHERE version = 10`,
		&migrationCount)
	if migrationCount != 1 {
		t.Fatal("failed rollback removed migration 010 record")
	}
}

func TestPhase15RAGProjectionPublishedMappingIsAtomic(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if _, err := NewRunner(db, phase15RAGBaseMigrationFS(t)).Up(ctx); err != nil {
		t.Fatalf("base up: %v", err)
	}
	profileID := "00000000-0000-0000-0000-000000005001"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_profiles(
  id, processor, endpoint_id, model_api_version, allowed_purposes,
  allowed_data_types, region, retention_policy, deletion_contract,
  training_use, status, governance_revision, manifest_hash
) VALUES (
  $1, 'jina', 'hosted-default', 'v1', ARRAY['parse'], ARRAY['text'],
  'global', 'none', 'delete', 'none', 'approved', 1, $2
)`, profileID, strings.Repeat("c", 64))
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_heads(
  processor, endpoint_id, status, active_profile_id,
  active_governance_revision, head_revision
) VALUES ('jina', 'hosted-default', 'active', $1, 1, 1)`, profileID)

	fullRunner := NewRunner(db, phase15MigrationFSThrough(t, 10))
	if _, err := fullRunner.Up(ctx); err == nil ||
		!strings.Contains(err.Error(), "RAG_GOVERNANCE_PROFILE_MAPPING_COVERAGE_MISMATCH") {
		t.Fatalf("missing mapping error = %v", err)
	}
	assertPhase151CColumnAbsent(t, ctx, db,
		"processor_governance_profiles", "model_id")

	mapping, err := ParsePhase15GovernanceMapping([]byte(fmt.Sprintf(`{
	  "profiles":[{"profileId":%q,"modelId":"jina-embeddings-v4",
	    "profileContractHash":"%s"}],
  "heads":[{"processor":"jina","endpointId":"hosted-default",
    "modelId":"jina-embeddings-v4"}]
}`, profileID, strings.Repeat("d", 64))))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fullRunner.WithPhase15GovernanceMapping(mapping); err != nil {
		t.Fatal(err)
	}
	if _, err := fullRunner.Up(ctx); err == nil ||
		!strings.Contains(err.Error(), "RAG_GOVERNANCE_PROFILE_HASH_MISMATCH") {
		t.Fatalf("profile contract hash mismatch error = %v", err)
	}
	assertPhase151CColumnAbsent(t, ctx, db,
		"processor_governance_profiles", "model_id")

	contractHash := testPhase15ProfileContractHash(
		"jina", "hosted-default", "jina-embeddings-v4", "v1",
		[]string{"parse"}, []string{"text"}, "global", "none", "delete",
		"none", 1, strings.Repeat("c", 64),
	)
	mapping.Profiles[0].ProfileContractHash = contractHash
	if _, err := fullRunner.WithPhase15GovernanceMapping(mapping); err != nil {
		t.Fatal(err)
	}
	applied, err := fullRunner.Up(ctx)
	if err != nil {
		t.Fatalf("published up: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != 10 {
		t.Fatalf("published applied = %#v", applied)
	}
	var modelID string
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT model_id FROM processor_governance_profiles WHERE id = $1`,
		&modelID, profileID)
	if modelID != "jina-embeddings-v4" {
		t.Fatalf("model_id = %q", modelID)
	}
	if _, err := fullRunner.Down(ctx, false); err != nil {
		t.Fatalf("published rollback: %v", err)
	}
	assertPhase151CColumnAbsent(t, ctx, db,
		"processor_governance_profiles", "model_id")
}

func testPhase15ProfileContractHash(
	processor, endpointID, modelID, modelAPIVersion string,
	allowedPurposes, allowedDataTypes []string,
	region, retentionPolicy, deletionContract, trainingUse string,
	governanceRevision int64,
	manifestHash string,
) string {
	canonical := struct {
		ContractVersion    int      `json:"contractVersion"`
		Processor          string   `json:"processor"`
		EndpointID         string   `json:"endpointId"`
		ModelID            string   `json:"modelId"`
		ModelAPIVersion    string   `json:"modelApiVersion"`
		AllowedPurposes    []string `json:"allowedPurposes"`
		AllowedDataTypes   []string `json:"allowedDataTypes"`
		Region             string   `json:"region"`
		RetentionPolicy    string   `json:"retentionPolicy"`
		DeletionContract   string   `json:"deletionContract"`
		TrainingUse        string   `json:"trainingUse"`
		GovernanceRevision int64    `json:"governanceRevision"`
		ManifestHash       string   `json:"manifestHash"`
	}{
		1, processor, endpointID, modelID, modelAPIVersion,
		allowedPurposes, allowedDataTypes, region, retentionPolicy,
		deletionContract, trainingUse, governanceRevision, manifestHash,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func assertPhase15Postgres16(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var version int
	mustQueryPhase15RAGRow(t, ctx, db,
		`SELECT current_setting('server_version_num')::INTEGER`, &version)
	if version < 160000 || version >= 170000 {
		t.Fatalf("PostgreSQL server_version_num = %d, want PG16", version)
	}
}

func assertPhase15CapabilityRolesRestricted(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	var safeCount int
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT count(*)
FROM pg_roles
WHERE rolname = ANY(ARRAY[
    'rag_projection_owner', 'rag_worker_executor', 'rag_replay_operator',
    'rag_api_reader', 'go_evidence_hydrator', 'go_api_runtime'
  ])
  AND NOT rolcanlogin
  AND NOT rolsuper
  AND NOT rolcreatedb
  AND NOT rolcreaterole
  AND NOT rolreplication
  AND NOT rolbypassrls`, &safeCount)
	if safeCount != 6 {
		t.Fatalf("restricted capability roles = %d/6", safeCount)
	}
}

func assertPhase15GoAPINoDangerousMembership(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	for _, role := range []string{
		"rag_projection_owner",
		"rag_worker_executor",
		"rag_replay_operator",
	} {
		var member bool
		mustQueryPhase15RAGRow(t, ctx, db,
			`SELECT pg_has_role('go_api_runtime', $1, 'MEMBER')`,
			&member, role)
		if member {
			t.Fatalf("go_api_runtime is a member of %s", role)
		}
	}
}

func assertPhase15ReadinessCapability(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	role string,
	want bool,
) {
	t.Helper()
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	quotedRole := `"` + strings.ReplaceAll(role, `"`, `""`) + `"`
	if _, err := connection.ExecContext(ctx,
		`SET SESSION AUTHORIZATION `+quotedRole); err != nil {
		t.Fatalf("set session authorization %s: %v", role, err)
	}
	defer func() {
		_, _ = connection.ExecContext(context.Background(),
			`RESET SESSION AUTHORIZATION`)
	}()
	var ready bool
	if err := connection.QueryRowContext(ctx, `
SELECT consumer_ready FROM knowledge_rag_worker_readiness()`).Scan(&ready); err != nil {
		t.Fatalf("read readiness as %s: %v", role, err)
	}
	if ready != want {
		t.Fatalf("readiness capability as %s = %t, want %t", role, ready, want)
	}
}

func assertPhase15GoAPIBasePrivileges(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	baseTables := []string{
		"users",
		"sessions",
		"conversations",
		"provider_configs",
		"files",
		"messages",
		"message_attachments",
		"audit_logs",
		"import_batches",
		"user_credentials",
		"credential_recovery_tokens",
		"teams",
		"team_memberships",
		"team_invites",
		"knowledge_collections",
		"knowledge_documents",
		"knowledge_document_versions",
		"user_query_consent_state",
		"processor_governance_profiles",
		"processor_governance_heads",
		"processing_consents",
		"knowledge_outbox",
		"identity_mail_outbox",
		"knowledge_processing_jobs",
	}
	for _, table := range baseTables {
		var allowed bool
		mustQueryPhase15RAGRow(t, ctx, db, `
SELECT has_table_privilege(
  'go_api_runtime', format('%I.%I', current_schema(), $1),
  'SELECT,INSERT,UPDATE,DELETE'
)`, &allowed, table)
		if !allowed {
			t.Errorf("go_api_runtime lacks DML on %s", table)
		}
	}

	var sequenceAllowed bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT has_sequence_privilege(
  'go_api_runtime',
  format('%I.%I', current_schema(), 'knowledge_outbox_id_seq'),
  'USAGE,SELECT'
)`, &sequenceAllowed)
	if !sequenceAllowed {
		t.Error("go_api_runtime lacks knowledge_outbox sequence capability")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatalf("set go_api_runtime: %v", err)
	}
	const userID = "00000000-0000-0000-0000-000000090001"
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users(id, display_name) VALUES ($1, 'runtime insert')`, userID); err != nil {
		t.Fatalf("go_api_runtime insert user: %v", err)
	}
	var displayName string
	if err := tx.QueryRowContext(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName); err != nil {
		t.Fatalf("go_api_runtime select user: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE users SET display_name = 'runtime update' WHERE id = $1`, userID); err != nil {
		t.Fatalf("go_api_runtime update user: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT nextval('knowledge_outbox_id_seq')`); err != nil {
		t.Fatalf("go_api_runtime allocate outbox sequence: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("go_api_runtime delete user: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback go_api_runtime DML probe: %v", err)
	}
}

func assertPhase15GoAPIProjectionBoundary(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	if err := execPhase15AsRole(ctx, db, "go_api_runtime", `
SELECT * FROM knowledge_rag_worker_readiness()`); err != nil {
		t.Fatalf("go_api_runtime readiness capability: %v", err)
	}
	hydrationErr := execPhase15AsRole(ctx, db, "go_api_runtime", `
SELECT * FROM knowledge_reauthorize_and_hydrate_evidence(
  '00000000-0000-0000-0000-000000090010',
  '00000000-0000-0000-0000-000000090011',
  '00000000-0000-0000-0000-000000090012',
  '[]'::jsonb
)`)
	if hydrationErr == nil ||
		!strings.Contains(hydrationErr.Error(), "RAG_HYDRATION_NOT_AUTHORIZED") {
		t.Fatalf("go_api_runtime hydration capability error = %v", hydrationErr)
	}

	deniedQueries := map[string]string{
		"schema_migrations": `SELECT * FROM schema_migrations`,
		"schema DDL":        `CREATE TABLE go_api_runtime_forbidden(id INTEGER)`,
		"table DDL":         `ALTER TABLE users ADD COLUMN forbidden INTEGER`,
		"projection owner":  `SELECT * FROM knowledge_projection_state`,
		"worker": `SELECT * FROM knowledge_claim_outbox(
  'api-forbidden',
  '00000000-0000-0000-0000-000000090020',
  '00000000-0000-0000-0000-000000090021',
  30
)`,
		"replay": `SELECT knowledge_replay_outbox(
  '00000000-0000-0000-0000-000000090022',
  'NOT_FOUND',
  '00000000-0000-0000-0000-000000090023',
  'api forbidden'
)`,
	}
	for name, query := range deniedQueries {
		err := execPhase15AsRole(ctx, db, "go_api_runtime", query)
		if !phase15InsufficientPrivilege(err) {
			t.Errorf("go_api_runtime %s error = %v, want SQLSTATE 42501", name, err)
		}
	}

	var ownedObjects int
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT
  (SELECT count(*)
   FROM pg_class relation
   WHERE relation.relnamespace = current_schema()::regnamespace
     AND relation.relowner = 'go_api_runtime'::regrole)
  +
  (SELECT count(*)
   FROM pg_proc function
   WHERE function.pronamespace = current_schema()::regnamespace
     AND function.proowner = 'go_api_runtime'::regrole)`, &ownedObjects)
	if ownedObjects != 0 {
		t.Fatalf("go_api_runtime owns %d schema objects", ownedObjects)
	}
}

func execPhase15AsRole(
	ctx context.Context,
	db *sql.DB,
	role string,
	query string,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	quotedRole := `"` + strings.ReplaceAll(role, `"`, `""`) + `"`
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE `+quotedRole); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query)
	return err
}

func phase15InsufficientPrivilege(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}

func phase15UndefinedFunction(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42883"
}

func phase15RAGBaseMigrationFS(t *testing.T) fstest.MapFS {
	return phase15MigrationFSThrough(t, 9)
}

func phase15MigrationFSThrough(t *testing.T, maxVersion int) fstest.MapFS {
	t.Helper()
	loaded, err := Load(migrationfiles.FS)
	if err != nil {
		t.Fatal(err)
	}
	result := fstest.MapFS{}
	for _, migration := range loaded {
		if migration.Version > int64(maxVersion) {
			continue
		}
		for _, path := range []string{migration.UpPath, migration.DownPath} {
			data, err := fs.ReadFile(migrationfiles.FS, path)
			if err != nil {
				t.Fatal(err)
			}
			result[path] = &fstest.MapFile{Data: data}
		}
	}
	return result
}

func mustQueryPhase15RAGRow(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	query string,
	destination any,
	args ...any,
) {
	t.Helper()
	if err := db.QueryRowContext(ctx, query, args...).Scan(destination); err != nil {
		t.Fatalf("query failed: %v\nquery:\n%s", err, query)
	}
}
