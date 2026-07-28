package migration

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryProvenanceDeletionMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "055_memory_provenance_deletion.up.sql")
	down := readPhase15SQL(t, "055_memory_provenance_deletion.down.sql")

	for _, table := range []string{
		"user_memory_state",
		"user_memory_evidence",
		"user_memory_revisions",
		"user_memory_tombstones",
		"user_memory_deletion_manifests",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("055 is missing %s", table)
		}
	}
	assertPhase15Fragments(t, phase15AlterTableDDL(up, "user_memories"),
		"055 canonical rows must carry revision, epoch, hash, authority, and profile provenance",
		"add column revision bigint",
		"add column visibility_epoch bigint",
		"add column content_hash text",
		"add column authority_kind text",
		"add column extraction_profile_id text")
	assertPhase15Fragments(t, phase15TableDDL(t, up, "user_memory_evidence"),
		"055 evidence must be ID/hash-only and ownership-fenced",
		"primary key ( memory_id , source_message_id )",
		"evidence_role in ( 'user' , 'assistant_context' )",
		"source_content_hash text not null",
		"foreign key ( memory_id , user_id )",
		"foreign key ( source_message_id , user_id )")
	if strings.Contains(phase15TableDDL(t, up, "user_memory_evidence"), "content text") {
		t.Fatal("055 evidence must not copy message plaintext")
	}
	assertPhase15Fragments(t, up,
		"055 deletion must be immediate, durable, lease-fenced, and provider-free",
		"create function memory_delete_global",
		"create function memory_worker_purge_memory",
		"event_type = 'memory.deleted'",
		"stage = 'purge'",
		"max_attempts = 128",
		"memory_capture_source_tombstoned",
		"memory_visibility_epoch_drift",
		"memory_revision_append_only",
		"authority_kind <> 'auto'",
		"to go_api_runtime",
		"to memory_worker_runtime")
	assertPhase15Fragments(t, down,
		"055 rollback must preserve used provenance and deletion authority",
		"memory_provenance_rollback_requires_empty_history",
		"memory_provenance_rollback_requires_default_state",
		"memory_provenance_rollback_requires_v1_compatible_memory")
}

func TestMemoryProvenanceDeletionLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	through54 := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
		"054_memory_outbox_jobs_worker.up.sql",
		"054_memory_outbox_jobs_worker.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		through54[path] = &fstest.MapFile{Data: data}
	}
	baseRunner := NewRunner(db, through54)
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply migrations through 054: %v", err)
	}

	const (
		userA           = "11000000-0000-4000-8000-000000000001"
		userB           = "11000000-0000-4000-8000-000000000002"
		conversationA   = "21000000-0000-4000-8000-000000000001"
		conversationB   = "21000000-0000-4000-8000-000000000002"
		sourceA         = "31000000-0000-4000-8000-000000000001"
		assistantA      = "31000000-0000-4000-8000-000000000002"
		sourceB         = "31000000-0000-4000-8000-000000000003"
		manualMemory    = "41000000-0000-4000-8000-000000000001"
		legacyAIMemory  = "41000000-0000-4000-8000-000000000002"
		missingAIMemory = "41000000-0000-4000-8000-000000000003"
		provider        = "51000000-0000-4000-8000-000000000001"
		eventID         = "61000000-0000-4000-8000-000000000001"
		extractJob      = "71000000-0000-4000-8000-000000000001"
		workerID        = "81000000-0000-4000-8000-000000000001"
		leaseExtract    = "91000000-0000-4000-8000-000000000001"
		autoMemory      = "41000000-0000-4000-8000-000000000004"
		deleteEvent     = "61000000-0000-4000-8000-000000000002"
		purgeJob        = "71000000-0000-4000-8000-000000000002"
		tombstoneID     = "a1000000-0000-4000-8000-000000000001"
		manifestID      = "b1000000-0000-4000-8000-000000000001"
		leasePurge      = "91000000-0000-4000-8000-000000000002"
		manualRebuild   = "41000000-0000-4000-8000-000000000005"
		staleSource     = "31000000-0000-4000-8000-000000000004"
		staleAssistant  = "31000000-0000-4000-8000-000000000005"
		staleEvent      = "61000000-0000-4000-8000-000000000003"
		staleJob        = "71000000-0000-4000-8000-000000000003"
		leaseStale      = "91000000-0000-4000-8000-000000000003"
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR4 A'), ($2, 'PR4 B');
INSERT INTO conversations(id, user_id, title) VALUES
  ($3, $1, 'PR4 source'), ($4, $2, 'PR4 foreign');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES
  ($5, $3, $1, 1, 'user', 'completed', 'Remember concise answers', now()),
  ($6, $3, $1, 2, 'assistant', 'completed', 'Understood', now()),
  ($7, $4, $2, 1, 'user', 'completed', 'Foreign evidence', now());
UPDATE messages SET parent_message_id = $5 WHERE id = $6;
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ($1, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $8, $1, 'fixture', 'Fixture', '{"v":1}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source
) VALUES
  ($9, $1, 'preference', 'Legacy manual', 'legacy manual', 'manual'),
  ($10, $1, 'preference', 'Legacy AI good', 'legacy ai good', 'ai'),
  ($11, $1, 'preference', 'Legacy AI missing', 'legacy ai missing', 'ai');
UPDATE user_memories
SET source_conversation_id = $3, source_message_id = $5
WHERE id = $10;
`, userA, userB, conversationA, conversationB, sourceA, assistantA, sourceB,
		provider, manualMemory, legacyAIMemory, missingAIMemory)

	through55 := make(fstest.MapFS, len(through54)+2)
	for path, data := range through54 {
		through55[path] = data
	}
	for _, path := range []string{
		"055_memory_provenance_deletion.up.sql",
		"055_memory_provenance_deletion.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		through55[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, through55)
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply 055: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != 55 {
		t.Fatalf("applied = %#v", applied)
	}

	var stateCount, evidenceCount int
	var manualAuthority, manualHash string
	var goodEnabled, missingEnabled bool
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memory_state WHERE user_id IN ($1, $2)),
  (SELECT count(*) FROM user_memory_evidence WHERE memory_id = $3),
  (SELECT authority_kind FROM user_memories WHERE id = $4),
  (SELECT content_hash FROM user_memories WHERE id = $4),
  (SELECT enabled FROM user_memories WHERE id = $3),
  (SELECT enabled FROM user_memories WHERE id = $5)
`, userA, userB, legacyAIMemory, manualMemory, missingAIMemory).Scan(
		&stateCount, &evidenceCount, &manualAuthority, &manualHash,
		&goodEnabled, &missingEnabled,
	); err != nil {
		t.Fatal(err)
	}
	if stateCount != 2 || evidenceCount != 1 || manualAuthority != "manual" ||
		len(manualHash) != 64 || !goodEnabled || missingEnabled {
		t.Fatalf("backfill = state:%d evidence:%d manual:%q/%q enabled:%t/%t",
			stateCount, evidenceCount, manualAuthority, manualHash,
			goodEnabled, missingEnabled)
	}
	for _, check := range []struct {
		role      string
		table     string
		privilege string
	}{
		{"memory_worker_runtime", "user_memory_state", "SELECT"},
		{"memory_worker_runtime", "user_memory_evidence", "SELECT"},
		{"memory_worker_runtime", "user_memory_revisions", "UPDATE"},
		{"memory_worker_runtime", "user_memory_tombstones", "SELECT"},
		{"memory_worker_runtime", "user_memory_deletion_manifests", "SELECT"},
		{"go_api_runtime", "user_memory_deletion_manifests", "SELECT"},
		{"go_api_runtime", "user_memories", "DELETE"},
	} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `
SELECT has_table_privilege($1, $2, $3)
`, check.role, check.table, check.privilege).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if allowed {
			t.Fatalf("%s unexpectedly has %s on %s", check.role, check.privilege, check.table)
		}
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_memory_evidence(
  memory_id, source_message_id, user_id, source_conversation_id,
  evidence_role, source_content_hash, observed_at
) VALUES ($1, $2, $3, $4, 'user', repeat('a', 64), now())
`, manualMemory, sourceB, userA, conversationB); err == nil {
		t.Fatal("cross-user evidence unexpectedly succeeded")
	}

	var upserted string
	if err := db.QueryRowContext(ctx, `
SELECT id::text
FROM memory_upsert_global_manual(
  '41000000-0000-4000-8000-000000000099', $1,
  'preference', 'Legacy manual revised', 'legacy manual',
  5::smallint, ARRAY['manual']::text[], NULL, NULL, true
)
`, userA).Scan(&upserted); err != nil {
		t.Fatalf("manual upsert: %v", err)
	}
	var manualRevision, revisionRows int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT revision FROM user_memories WHERE id = $1),
  (SELECT count(*) FROM user_memory_revisions WHERE memory_id = $1)
`, manualMemory).Scan(&manualRevision, &revisionRows); err != nil {
		t.Fatal(err)
	}
	if upserted != manualMemory || manualRevision != 2 || revisionRows != 1 {
		t.Fatalf("manual revision = %q/%d/%d", upserted, manualRevision, revisionRows)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_revisions SET operation = 'merge' WHERE memory_id = $1
`, manualMemory); err == nil || !strings.Contains(err.Error(), "MEMORY_REVISION_APPEND_ONLY") {
		t.Fatalf("revision mutation error = %v", err)
	}

	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, eventID, extractJob, userA, conversationA, sourceA, assistantA); err != nil {
		t.Fatalf("append capture: %v", err)
	}
	claimMemoryJob(t, ctx, db, workerID, leaseExtract, extractJob, 1)
	var precedenceContent string
	if err := db.QueryRowContext(ctx, `
SELECT content
FROM memory_worker_apply_capture_candidate(
  $1, $2, $3, '41000000-0000-4000-8000-000000000098',
  'preference', 'Automatic overwrite attempt', 'legacy manual',
  5::smallint, ARRAY['auto']::text[]
)
`, extractJob, workerID, leaseExtract).Scan(&precedenceContent); err != nil {
		t.Fatalf("manual precedence apply: %v", err)
	}
	if precedenceContent != "Legacy manual revised" {
		t.Fatalf("manual authority overwritten with %q", precedenceContent)
	}

	var appliedAuto string
	if err := db.QueryRowContext(ctx, `
SELECT id::text
FROM memory_worker_apply_capture_candidate(
  $1, $2, $3, $4, 'preference', 'Use concise answers',
  'use concise answers', 5::smallint, ARRAY['style']::text[]
)
`, extractJob, workerID, leaseExtract, autoMemory).Scan(&appliedAuto); err != nil {
		t.Fatalf("apply automatic memory: %v", err)
	}
	if appliedAuto != autoMemory {
		t.Fatalf("automatic memory = %q", appliedAuto)
	}

	var deleted bool
	deleteTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deleteTx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = deleteTx.Rollback()
		t.Fatalf("set API role for delete: %v", err)
	}
	if err := deleteTx.QueryRowContext(ctx, `
SELECT memory_delete_global($1, $2, $3, $4, $5, $6)
`, userA, autoMemory, deleteEvent, purgeJob, tombstoneID, manifestID).Scan(&deleted); err != nil {
		_ = deleteTx.Rollback()
		t.Fatalf("delete automatic memory: %v", err)
	}
	if err := deleteTx.Commit(); err != nil {
		t.Fatal(err)
	}
	var visible, purgeCount, manifestCount int
	var retainedContent string
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memories WHERE id = $1 AND deleted_at IS NULL),
  (SELECT content FROM user_memories WHERE id = $1),
  (SELECT count(*) FROM memory_jobs WHERE job_id = $2 AND stage = 'purge'),
  (SELECT count(*) FROM user_memory_deletion_manifests
    WHERE manifest_id = $3 AND result_code = 'PENDING')
`, autoMemory, purgeJob, manifestID).Scan(
		&visible, &retainedContent, &purgeCount, &manifestCount,
	); err != nil {
		t.Fatal(err)
	}
	if !deleted || visible != 0 || retainedContent != "Use concise answers" ||
		purgeCount != 1 || manifestCount != 1 {
		t.Fatalf("delete = %t/%d/%q/%d/%d", deleted, visible, retainedContent, purgeCount, manifestCount)
	}
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_apply_capture_candidate(
  $1, $2, $3, $4, 'preference', 'Use concise answers',
  'use concise answers', 5::smallint, ARRAY['style']::text[]
)
`, extractJob, workerID, leaseExtract, autoMemory); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_CAPTURE_SOURCE_TOMBSTONED") {
		t.Fatalf("old response apply error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_worker_retry_job($1, $2, $3, 'SOURCE_TOMBSTONED', now(), true)
`, extractJob, workerID, leaseExtract); err != nil {
		t.Fatalf("terminalize old capture: %v", err)
	}

	claimMemoryJob(t, ctx, db, workerID, leasePurge, purgeJob, 1)
	purgeTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := purgeTx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		_ = purgeTx.Rollback()
		t.Fatalf("set worker role for purge: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var purged bool
		if err := purgeTx.QueryRowContext(ctx, `
SELECT memory_worker_purge_memory($1, $2, $3)
`, purgeJob, workerID, leasePurge).Scan(&purged); err != nil || !purged {
			_ = purgeTx.Rollback()
			t.Fatalf("purge attempt %d = %t/%v", attempt+1, purged, err)
		}
	}
	if _, err := purgeTx.ExecContext(ctx, `
SELECT memory_worker_complete_job($1, $2, $3)
`, purgeJob, workerID, leasePurge); err != nil {
		_ = purgeTx.Rollback()
		t.Fatalf("complete purge: %v", err)
	}
	if err := purgeTx.Commit(); err != nil {
		t.Fatal(err)
	}
	var content, normalized, manifestResult string
	var evidenceAfter, snapshotAfter int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT content FROM user_memories WHERE id = $1),
  (SELECT normalized_content FROM user_memories WHERE id = $1),
  (SELECT count(*) FROM user_memory_evidence WHERE memory_id = $1),
  (SELECT count(*) FROM user_memory_revisions
    WHERE memory_id = $1 AND prior_content_snapshot IS NOT NULL),
  (SELECT result_code FROM user_memory_deletion_manifests WHERE manifest_id = $2)
`, autoMemory, manifestID).Scan(
		&content, &normalized, &evidenceAfter, &snapshotAfter, &manifestResult,
	); err != nil {
		t.Fatal(err)
	}
	if content != "" || normalized != "" || evidenceAfter != 0 ||
		snapshotAfter != 0 || manifestResult != "ONLINE_PURGED" {
		t.Fatalf("purged = %q/%q/%d/%d/%q", content, normalized,
			evidenceAfter, snapshotAfter, manifestResult)
	}
	var rebuilt string
	if err := db.QueryRowContext(ctx, `
SELECT id::text
FROM memory_upsert_global_manual(
  $1, $2, 'preference', 'Use concise answers', 'use concise answers',
  5::smallint, ARRAY['manual-rebuild']::text[], NULL, NULL, true
)
`, manualRebuild, userA).Scan(&rebuilt); err != nil {
		t.Fatalf("explicit manual rebuild: %v", err)
	}
	if rebuilt != manualRebuild {
		t.Fatalf("manual rebuild id = %q", rebuilt)
	}

	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES
  ($1, $2, $3, 3, 'user', 'completed', 'Stale epoch source', now()),
  ($4, $2, $3, 4, 'assistant', 'completed', 'Understood', now());
UPDATE messages SET parent_message_id = $1 WHERE id = $4;
`, staleSource, conversationA, userA, staleAssistant)
	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, staleEvent, staleJob, userA, conversationA, staleSource, staleAssistant); err != nil {
		t.Fatalf("append stale capture: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state
SET visibility_epoch = visibility_epoch + 1, updated_at = now()
WHERE user_id = $1;
`, userA)
	claimMemoryJob(t, ctx, db, workerID, leaseStale, staleJob, 1)
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_hydrate_capture($1, $2, $3)
`, staleJob, workerID, leaseStale); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_VISIBILITY_EPOCH_DRIFT") {
		t.Fatalf("stale epoch hydration error = %v", err)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_PROVENANCE_ROLLBACK_REQUIRES_EMPTY_HISTORY") {
		t.Fatalf("guarded 055 rollback error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
DELETE FROM user_memory_deletion_manifests;
DELETE FROM users WHERE id IN ($1, $2);
`, userA, userB)
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 55 {
		t.Fatalf("clean 055 rollback = %#v/%v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 55 {
		t.Fatalf("055 re-up = %#v/%v", reapplied, err)
	}
}
