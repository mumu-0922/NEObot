package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryActionsActivityUsageMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "057_memory_actions_activity_usage.up.sql")
	down := readPhase15SQL(t, "057_memory_actions_activity_usage.down.sql")

	for _, table := range []string{
		"memory_user_actions",
		"memory_user_action_targets",
		"message_memory_activities",
		"message_memory_usages",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("057 is missing %s", table)
		}
	}
	for _, table := range []string{
		"memory_user_actions", "memory_user_action_targets",
		"message_memory_activities", "message_memory_usages",
	} {
		body := phase15TableDDL(t, up, table)
		for _, forbidden := range []string{"query text", "prompt text", "embedding", "raw_score"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains forbidden durable payload %q", table, forbidden)
			}
		}
	}
	assertPhase15Fragments(t, phase15AlterTableDDL(up, "user_memory_revisions"),
		"057 must retain a complete typed prior snapshot for safe correction undo",
		"prior_snapshot_schema_major smallint",
		"prior_memory_type text",
		"prior_normalized_content text",
		"prior_tags text[]",
		"prior_scope_type text",
		"prior_authority_kind text",
		"prior_sensitivity text")
	assertPhase15Fragments(t, up,
		"057 must expose only narrow user-scoped action, usage, polling, and undo capabilities",
		"create function memory_hydrate_direct_user_action",
		"create function memory_apply_direct_user_action",
		"create function memory_record_message_usages",
		"create function memory_list_activities",
		"create function memory_undo_activity",
		"revoke all on memory_user_actions",
		"from public , go_api_runtime , memory_worker_runtime",
		"to go_api_runtime")
	assertPhase15Fragments(t, up,
		"057 activities must include Review/rejection/dead-letter and keep exact NOOP silent",
		"user_memory_review_suggestions_activity",
		"memory_jobs_dead_letter_activity",
		"if v_status <> 'noop'",
		"exact_noop")
	assertPhase15Fragments(t, up,
		"057 Usage links must be immutable and permit only exact idempotent replay",
		"v_existing_count",
		"memory_usage_replay_conflict",
		"pg_advisory_xact_lock")
	assertPhase15Fragments(t, down,
		"057 rollback must preserve action, activity, usage, and typed revision authority",
		"memory_action_rollback_requires_empty_history",
		"memory_action_rollback_requires_v1_source")
}

func TestMemoryActionsActivityUsageLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	files := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
		"054_memory_outbox_jobs_worker.up.sql",
		"054_memory_outbox_jobs_worker.down.sql",
		"055_memory_provenance_deletion.up.sql",
		"055_memory_provenance_deletion.down.sql",
		"056_memory_candidate_review_shadow.up.sql",
		"056_memory_candidate_review_shadow.down.sql",
		"057_memory_actions_activity_usage.up.sql",
		"057_memory_actions_activity_usage.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, files)
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("apply through 057: %v", err)
	}

	const (
		userID       = "13000000-0000-4000-8000-000000000001"
		conversation = "23000000-0000-4000-8000-000000000001"
		source       = "33000000-0000-4000-8000-000000000001"
		assistant    = "33000000-0000-4000-8000-000000000002"
		actionID     = "43000000-0000-4000-8000-000000000001"
		activityID   = "44000000-0000-4000-8000-000000000001"
		memoryID     = "53000000-0000-4000-8000-000000000001"
		providerID   = "42000000-0000-4000-8000-000000000001"
		captureEvent = "65000000-0000-4000-8000-000000000001"
		extractJob   = "75000000-0000-4000-8000-000000000001"
		expiryJob    = "75000000-0000-4000-8000-000000000002"
		workerID     = "85000000-0000-4000-8000-000000000001"
		extractLease = "95000000-0000-4000-8000-000000000001"
		pendingID    = "a5000000-0000-4000-8000-000000000001"
		rejectedID   = "a5000000-0000-4000-8000-000000000002"
		reviewSource = "35000000-0000-4000-8000-000000000003"
		reviewAnswer = "35000000-0000-4000-8000-000000000004"
		failedSource = "35000000-0000-4000-8000-000000000001"
		failedAnswer = "35000000-0000-4000-8000-000000000002"
		failedEvent  = "65000000-0000-4000-8000-000000000002"
		failedJob    = "75000000-0000-4000-8000-000000000003"
		failedLease  = "95000000-0000-4000-8000-000000000002"
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR6');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'PR6');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed',
  'Remember concise replies', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'pending', '');
`, userID, conversation, source, assistant)

	var status, resultCode, resultMemory string
	var revision int64
	if err := db.QueryRowContext(ctx, `
SELECT action_status, action_result_code, result_memory_id::text,
       result_memory_revision
FROM memory_apply_direct_user_action(
  $1::uuid, $2::uuid, $3::uuid,
  '63000000-0000-4000-8000-000000000001'::uuid,
  '73000000-0000-4000-8000-000000000001'::uuid,
  '83000000-0000-4000-8000-000000000001'::uuid,
  '93000000-0000-4000-8000-000000000001'::uuid,
  $4::uuid, $5::uuid, $6::uuid, $7::uuid, 1::smallint,
  'remember', 'preference', 'Use concise replies', 'use concise replies',
  encode(sha256(convert_to('Use concise replies', 'UTF8')), 'hex'),
  5::smallint, ARRAY['reply']::text[], 'normal', 'global',
  0.99::double precision, '[]'::jsonb, NULL, NULL
)
`, actionID, activityID, memoryID, userID, conversation, source, assistant).Scan(
		&status, &resultCode, &resultMemory, &revision,
	); err != nil {
		t.Fatalf("apply direct remember: %v", err)
	}
	if status != "applied" || resultCode != "DIRECT_CREATED" ||
		resultMemory != memoryID || revision != 1 {
		t.Fatalf("direct result = %q/%q/%q/%d", status, resultCode, resultMemory, revision)
	}

	var sourceKind, authorityKind string
	if err := db.QueryRowContext(ctx, `
SELECT source, authority_kind FROM user_memories WHERE id = $1
`, memoryID).Scan(&sourceKind, &authorityKind); err != nil {
		t.Fatal(err)
	}
	if sourceKind != "direct_user" || authorityKind != "direct_user" {
		t.Fatalf("direct authority = %q/%q", sourceKind, authorityKind)
	}

	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET status = 'completed', content = 'Saved', completed_at = now()
WHERE id = $1;
SELECT memory_record_message_usages(
  $2::uuid, $3::uuid, $1::uuid,
  jsonb_build_array(jsonb_build_object(
    'memoryId', $4::text, 'revision', 1, 'scopeType', 'global'
  ))
);
`, assistant, userID, conversation, memoryID)

	var usageContent string
	var usageDeleted bool
	if err := db.QueryRowContext(ctx, `
SELECT memory_content, memory_deleted
FROM memory_list_message_usages($1, $2)
`, userID, assistant).Scan(&usageContent, &usageDeleted); err != nil {
		t.Fatal(err)
	}
	if usageContent != "Use concise replies" || usageDeleted {
		t.Fatalf("usage = %q/%t", usageContent, usageDeleted)
	}
	visibilityTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := visibilityTx.ExecContext(ctx, `
UPDATE user_memory_state
SET visibility_epoch = visibility_epoch + 1, updated_at = now()
WHERE user_id = $1
`, userID); err != nil {
		_ = visibilityTx.Rollback()
		t.Fatal(err)
	}
	var hiddenUsageContent sql.NullString
	if err := visibilityTx.QueryRowContext(ctx, `
SELECT memory_content, memory_deleted
FROM memory_list_message_usages($1, $2)
`, userID, assistant).Scan(&hiddenUsageContent, &usageDeleted); err != nil {
		_ = visibilityTx.Rollback()
		t.Fatal(err)
	}
	if hiddenUsageContent.Valid || !usageDeleted {
		_ = visibilityTx.Rollback()
		t.Fatalf("epoch-hidden usage = %#v/%t", hiddenUsageContent, usageDeleted)
	}
	if err := visibilityTx.Rollback(); err != nil {
		t.Fatal(err)
	}

	var undoStatus string
	if err := db.QueryRowContext(ctx, `
SELECT undo_status FROM memory_undo_activity(
  $1, $2, 1,
  '64000000-0000-4000-8000-000000000001',
  '74000000-0000-4000-8000-000000000001',
  '84000000-0000-4000-8000-000000000001',
  '94000000-0000-4000-8000-000000000001'
)
`, userID, activityID).Scan(&undoStatus); err != nil {
		t.Fatalf("undo created: %v", err)
	}
	if undoStatus != "undone" {
		t.Fatalf("undo status = %q", undoStatus)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_deleted FROM memory_list_message_usages($1, $2)
`, userID, assistant).Scan(&usageDeleted); err != nil || !usageDeleted {
		t.Fatalf("deleted usage marker = %t/%v", usageDeleted, err)
	}

	// PR5 Review outcomes and terminal worker failures become ID-only Activity
	// links on the assistant message; no candidate or source plaintext is copied.
	mustExecPhase151C(t, ctx, db, `
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES ($1, true, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $2, $1, 'fixture', 'Fixture', '{}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($6, $5, $1, 3, 'user', 'completed', 'review this candidate', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, completed_at
) VALUES ($7, $5, $1, $6, 4, 'assistant', 'completed', 'review queued', now());
SELECT memory_append_turn_completed_event(
  $3, $4, $1, $5, $6, $7,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
);
UPDATE memory_jobs
SET status = 'processing', attempt_count = 1,
    lease_owner = $8, lease_token = $9,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
WHERE job_id = $4;
UPDATE memory_outbox outbox
SET status = 'processing', attempt_count = 1,
    lease_owner = $8, lease_token = $9,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
FROM memory_jobs job
WHERE job.job_id = $4 AND outbox.event_id = job.event_id;
`, userID, providerID, captureEvent, extractJob, conversation, reviewSource, reviewAnswer, workerID, extractLease)
	var contextCount int
	var proposalCommitted bool
	if err := db.QueryRowContext(ctx, `
SELECT jsonb_array_length(context_messages), proposal_committed
FROM memory_worker_hydrate_capture_v2($1, $2, $3)
`, extractJob, workerID, extractLease).Scan(&contextCount, &proposalCommitted); err != nil ||
		contextCount < 2 || proposalCommitted {
		t.Fatalf("PR6 Review activity hydrate = %d/%t/%v", contextCount, proposalCommitted, err)
	}
	var observedAt time.Time
	if err := db.QueryRowContext(ctx, `
SELECT completed_at FROM messages WHERE id = $1
`, reviewSource).Scan(&observedAt); err != nil {
		t.Fatal(err)
	}
	pending := memoryReviewProposal(
		pendingID, reviewSource, observedAt,
		"Use concise answers tomorrow", "use concise answers tomorrow",
		"global", nil, nil, "normal", "ADD", nil,
	)
	pending["temporalBasis"] = "relative_ambiguous"
	pending["temporalParserVersion"] = "unresolved-v1"
	rejected := memoryReviewProposal(
		rejectedID, reviewSource, observedAt, "", "",
		"global", nil, nil, "secret", "REJECT", nil,
	)
	rejected["content"] = nil
	rejected["normalizedContent"] = nil
	rejected["tags"] = []string{}
	rejected["subjectKey"] = nil
	rejected["factKey"] = nil
	proposals, err := json.Marshal([]map[string]any{pending, rejected})
	if err != nil {
		t.Fatal(err)
	}
	var proposalCount, shadowCount, reviewCount, rejectedCount int
	if err := db.QueryRowContext(ctx, `
SELECT proposal_count, shadow_count, review_count, rejected_count
FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, extractJob, workerID, extractLease, expiryJob, string(proposals)).Scan(
		&proposalCount, &shadowCount, &reviewCount, &rejectedCount,
	); err != nil || proposalCount != 2 || shadowCount != 0 || reviewCount != 1 || rejectedCount != 1 {
		t.Fatalf("PR6 Review activities proposal = %d/%d/%d/%d/%v",
			proposalCount, shadowCount, reviewCount, rejectedCount, err)
	}
	var pendingActivities, rejectedActivities int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE action = 'review_required' AND status = 'pending'),
  count(*) FILTER (WHERE action = 'rejected' AND status = 'completed')
FROM message_memory_activities
WHERE assistant_message_id = $1 AND source_kind = 'review_suggestion'
`, reviewAnswer).Scan(&pendingActivities, &rejectedActivities); err != nil ||
		pendingActivities != 1 || rejectedActivities != 1 {
		t.Fatalf("PR6 Review activity links = %d/%d/%v",
			pendingActivities, rejectedActivities, err)
	}

	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, 5, 'user', 'completed', 'capture failure', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, completed_at
) VALUES ($4, $2, $3, $1, 6, 'assistant', 'completed', 'failed', now());
SELECT memory_append_turn_completed_event(
  $5, $6, $3, $2, $1, $4,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
);
UPDATE memory_jobs
SET status = 'processing', attempt_count = 1,
    lease_owner = $7, lease_token = $8,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
WHERE job_id = $6;
UPDATE memory_outbox outbox
SET status = 'processing', attempt_count = 1,
    lease_owner = $7, lease_token = $8,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
FROM memory_jobs job
WHERE job.job_id = $6 AND outbox.event_id = job.event_id;
SELECT memory_worker_retry_job(
  $6, $7, $8, 'TEST_DEAD_LETTER', now(), true
);
`, failedSource, conversation, userID, failedAnswer, failedEvent, failedJob, workerID, failedLease)
	var deadLetterActivities int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM message_memory_activities
WHERE assistant_message_id = $1 AND subject_id = $2
  AND source_kind = 'memory_job' AND action = 'failed'
  AND status = 'failed' AND reason_code = 'TEST_DEAD_LETTER'
`, failedAnswer, failedJob).Scan(&deadLetterActivities); err != nil || deadLetterActivities != 1 {
		t.Fatalf("dead-letter Activity = %d/%v", deadLetterActivities, err)
	}

	for _, check := range []struct {
		role, table, privilege string
	}{
		{"go_api_runtime", "memory_user_actions", "INSERT"},
		{"go_api_runtime", "message_memory_activities", "UPDATE"},
		{"go_api_runtime", "message_memory_usages", "INSERT"},
		{"memory_worker_runtime", "memory_user_actions", "SELECT"},
	} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,$3)`,
			check.role, check.table, check.privilege).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if allowed {
			t.Fatalf("%s unexpectedly has %s on %s", check.role, check.privilege, check.table)
		}
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_ACTION_ROLLBACK_REQUIRES_EMPTY_HISTORY") {
		t.Fatalf("guarded down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `DELETE FROM users WHERE id = $1`, userID)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 57 {
		t.Fatalf("clean down = %#v/%v", down, err)
	}
	up, err := runner.Up(ctx)
	if err != nil || len(up) != 1 || up[0].Version != 57 {
		t.Fatalf("re-up = %#v/%v", up, err)
	}
}
