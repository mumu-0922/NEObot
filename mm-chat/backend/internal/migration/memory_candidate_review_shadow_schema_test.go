package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryCandidateReviewShadowMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "056_memory_candidate_review_shadow.up.sql")
	down := readPhase15SQL(t, "056_memory_candidate_review_shadow.down.sql")

	for _, table := range []string{
		"memory_capture_candidate_batches",
		"user_memory_review_suggestions",
		"user_memory_review_targets",
		"user_memory_review_evidence",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("056 is missing %s", table)
		}
	}
	assertPhase15Fragments(t, phase15AlterTableDDL(up, "user_memories"),
		"056 canonical metadata must remain additive and temporal/conflict aware",
		"add column lifecycle_status text not null default 'active'",
		"add column subject_key text",
		"add column fact_key text",
		"add column confidence double precision",
		"add column observed_at timestamptz",
		"add column valid_from timestamptz",
		"add column valid_to timestamptz",
		"add column expires_at timestamptz",
		"add column superseded_by_memory_id uuid",
		"add column sensitivity text not null default 'normal'",
		"add column temporal_basis text not null default 'none'")
	assertPhase15Fragments(t, phase15TableDDL(t, up, "user_memory_review_suggestions"),
		"056 suggestions must be bounded, scope-owned, non-recall proposal state",
		"unique ( capture_job_id , ordinal )",
		"foreign key ( proposed_project_id , user_id )",
		"foreign key ( proposed_conversation_id , user_id )",
		"status in ( 'shadow' , 'pending' , 'rejected' , 'expired' )",
		"review_expires_at = created_at + interval '30 days'",
		"candidate_content is null",
		"plaintext_expired")
	assertPhase15Fragments(t, up,
		"056 must disable canonical auto-apply and expose only lease-fenced proposal/expiry capabilities",
		"revoke execute on function memory_worker_apply_capture_candidate",
		"memory_worker_propose_capture_candidates",
		"memory_worker_expire_capture_reviews",
		"stage = 'review_expire'",
		"max_attempts = 128",
		"memory_candidate_target_invalid",
		"memory_secret_plaintext_forbidden",
		"to memory_worker_runtime")
	assertPhase15Fragments(t, up,
		"056 candidate authority must require complete objects and retain the exact target revision",
		"not ( v_candidate ?& array[",
		"when v_exact_found and memory.id = v_exact.id then 0")
	assertPhase15Fragments(t, down,
		"056 rollback must preserve used Review and temporal authority",
		"memory_review_rollback_requires_empty_history",
		"memory_review_rollback_requires_default_canonical_metadata",
		"grant execute on function memory_worker_hydrate_capture")
}

func TestMemoryCandidateReviewShadowLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	through55 := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
		"054_memory_outbox_jobs_worker.up.sql",
		"054_memory_outbox_jobs_worker.down.sql",
		"055_memory_provenance_deletion.up.sql",
		"055_memory_provenance_deletion.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		through55[path] = &fstest.MapFile{Data: data}
	}
	baseRunner := NewRunner(db, through55)
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply migrations through 055: %v", err)
	}

	const (
		userID       = "12000000-0000-4000-8000-000000000001"
		conversation = "22000000-0000-4000-8000-000000000001"
		source       = "32000000-0000-4000-8000-000000000001"
		assistant    = "32000000-0000-4000-8000-000000000002"
		provider     = "42000000-0000-4000-8000-000000000001"
		manualMemory = "52000000-0000-4000-8000-000000000001"
		eventID      = "62000000-0000-4000-8000-000000000001"
		extractJob   = "72000000-0000-4000-8000-000000000001"
		expiryJob    = "72000000-0000-4000-8000-000000000002"
		workerID     = "82000000-0000-4000-8000-000000000001"
		extractLease = "92000000-0000-4000-8000-000000000001"
		expiryLease  = "92000000-0000-4000-8000-000000000002"
		addID        = "a2000000-0000-4000-8000-000000000001"
		conflictID   = "a2000000-0000-4000-8000-000000000002"
		exactID      = "a2000000-0000-4000-8000-000000000003"
		temporalID   = "a2000000-0000-4000-8000-000000000004"
		secretID     = "a2000000-0000-4000-8000-000000000005"
	)
	observedAt := time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR5');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'PR5');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status,
  content, completed_at, created_at, updated_at
) VALUES
  ($3, $2, $1, 1, 'user', 'completed', 'Remember concise answers', $6, $6, $6),
  ($4, $2, $1, 2, 'assistant', 'completed', 'Understood', $6, $6, $6);
UPDATE messages SET parent_message_id = $3 WHERE id = $4;
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES ($1, true, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $5, $1, 'fixture', 'Fixture', '{}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
SELECT id FROM memory_upsert_global_manual(
  $7, $1, 'preference', 'Use detailed answers', 'use detailed answers',
  5::smallint, ARRAY['manual']::text[], NULL, NULL, true
);
`, userID, conversation, source, assistant, provider, observedAt, manualMemory)

	through56 := make(fstest.MapFS, len(through55)+2)
	for path, data := range through55 {
		through56[path] = data
	}
	for _, path := range []string{
		"056_memory_candidate_review_shadow.up.sql",
		"056_memory_candidate_review_shadow.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		through56[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, through56)
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 56 {
		t.Fatalf("apply 056 = %#v/%v", applied, err)
	}

	var lifecycle, sensitivity, temporal string
	var confidence float64
	if err := db.QueryRowContext(ctx, `
SELECT lifecycle_status, sensitivity, temporal_basis, confidence
FROM user_memories WHERE id = $1
`, manualMemory).Scan(&lifecycle, &sensitivity, &temporal, &confidence); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "active" || sensitivity != "normal" || temporal != "none" || confidence != 1 {
		t.Fatalf("canonical backfill = %q/%q/%q/%v", lifecycle, sensitivity, temporal, confidence)
	}

	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, eventID, extractJob, userID, conversation, source, assistant); err != nil {
		t.Fatalf("append PR5 capture: %v", err)
	}
	claimMemoryJob(t, ctx, db, workerID, extractLease, extractJob, 1)

	var proposalCommitted bool
	var contextCount int
	if err := db.QueryRowContext(ctx, `
SELECT jsonb_array_length(context_messages), proposal_committed
FROM memory_worker_hydrate_capture_v2($1, $2, $3)
`, extractJob, workerID, extractLease).Scan(&contextCount, &proposalCommitted); err != nil {
		t.Fatal(err)
	}
	if contextCount != 2 || proposalCommitted {
		t.Fatalf("hydrate = context:%d committed:%t", contextCount, proposalCommitted)
	}

	invalid := memoryReviewProposal(
		addID, source, observedAt, "Invalid project", "invalid project",
		"project", "32000000-0000-4000-8000-000000000099", nil,
		"normal", "ADD", nil,
	)
	invalidBody, _ := json.Marshal([]map[string]any{invalid})
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, extractJob, workerID, extractLease, expiryJob, string(invalidBody)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_CANDIDATE_SCOPE_INVALID") {
		t.Fatalf("invalid scope error = %v", err)
	}
	var batchCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM memory_capture_candidate_batches WHERE capture_job_id = $1
`, extractJob).Scan(&batchCount); err != nil || batchCount != 0 {
		t.Fatalf("invalid candidate batch count = %d/%v", batchCount, err)
	}
	invalidTarget := memoryReviewProposal(
		addID, source, observedAt, "Spoof target", "spoof target",
		"global", nil, nil, "normal", "SUPERSEDE",
		[]string{"52000000-0000-4000-8000-000000000099"},
	)
	invalidTargetBody, _ := json.Marshal([]map[string]any{invalidTarget})
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, extractJob, workerID, extractLease, expiryJob, string(invalidTargetBody)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_CANDIDATE_TARGET_INVALID") {
		t.Fatalf("invalid target error = %v", err)
	}

	add := memoryReviewProposal(
		addID, source, observedAt, "Use concise answers", "use concise answers",
		"global", nil, nil, "normal", "ADD", nil,
	)
	conflict := memoryReviewProposal(
		conflictID, source, observedAt, "Prefer brief replies", "prefer brief replies",
		"global", nil, nil, "normal", "SUPERSEDE", []string{manualMemory},
	)
	exact := memoryReviewProposal(
		exactID, source, observedAt, "Use detailed answers", "use detailed answers",
		"global", nil, nil, "normal", "SUPERSEDE", []string{manualMemory},
	)
	temporalProposal := memoryReviewProposal(
		temporalID, source, observedAt, "Use concise answers tomorrow", "use concise answers tomorrow",
		"global", nil, nil, "normal", "ADD", nil,
	)
	temporalProposal["temporalBasis"] = "relative_ambiguous"
	temporalProposal["temporalParserVersion"] = "unresolved-v1"
	secretHash := sha256.Sum256([]byte("credential plaintext never persisted"))
	secret := memoryReviewProposal(
		secretID, source, observedAt, "", "", "global", nil, nil,
		"secret", "REJECT", nil,
	)
	secret["content"] = nil
	secret["normalizedContent"] = nil
	secret["candidateHash"] = hex.EncodeToString(secretHash[:])
	secret["tags"] = []string{}
	secret["subjectKey"] = nil
	secret["factKey"] = nil
	proposals, _ := json.Marshal([]map[string]any{add, conflict, exact, temporalProposal, secret})

	var proposalCount, shadowCount, reviewCount, rejectedCount int
	if err := db.QueryRowContext(ctx, `
SELECT proposal_count, shadow_count, review_count, rejected_count
FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, extractJob, workerID, extractLease, expiryJob, string(proposals)).Scan(
		&proposalCount, &shadowCount, &reviewCount, &rejectedCount,
	); err != nil {
		t.Fatalf("propose candidates: %v", err)
	}
	if proposalCount != 5 || shadowCount != 2 || reviewCount != 2 || rejectedCount != 1 {
		t.Fatalf("proposal result = %d/%d/%d/%d", proposalCount, shadowCount, reviewCount, rejectedCount)
	}

	var canonicalCount, targetCount, evidenceCount int
	var secretContent sql.NullString
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memories WHERE user_id = $1),
  (SELECT count(*) FROM user_memory_review_targets WHERE suggestion_id = $2),
  (SELECT count(*) FROM user_memory_review_evidence WHERE suggestion_id = $3),
  (SELECT candidate_content FROM user_memory_review_suggestions WHERE id = $4)
`, userID, conflictID, addID, secretID).Scan(
		&canonicalCount, &targetCount, &evidenceCount, &secretContent,
	); err != nil {
		t.Fatal(err)
	}
	if canonicalCount != 1 || targetCount != 1 || evidenceCount != 1 || secretContent.Valid {
		t.Fatalf("authority result = %d/%d/%d/%#v", canonicalCount, targetCount, evidenceCount, secretContent)
	}

	if err := db.QueryRowContext(ctx, `
SELECT proposal_committed FROM memory_worker_hydrate_capture_v2($1, $2, $3)
`, extractJob, workerID, extractLease).Scan(&proposalCommitted); err != nil || !proposalCommitted {
		t.Fatalf("committed hydrate = %t/%v", proposalCommitted, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT proposal_count FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, extractJob, workerID, extractLease, expiryJob, string(proposals)).Scan(&proposalCount); err != nil || proposalCount != 5 {
		t.Fatalf("idempotent proposal = %d/%v", proposalCount, err)
	}

	var directTable, oldApply, newPropose bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_table_privilege('memory_worker_runtime', 'user_memory_review_suggestions', 'SELECT'),
  has_function_privilege('memory_worker_runtime',
    'memory_worker_apply_capture_candidate(uuid,uuid,uuid,uuid,text,text,text,smallint,text[])', 'EXECUTE'),
  has_function_privilege('memory_worker_runtime',
    'memory_worker_propose_capture_candidates(uuid,uuid,uuid,uuid,smallint,text,text,jsonb)', 'EXECUTE')
`).Scan(&directTable, &oldApply, &newPropose); err != nil {
		t.Fatal(err)
	}
	if directTable || oldApply || !newPropose {
		t.Fatalf("worker privileges = table:%t old:%t new:%t", directTable, oldApply, newPropose)
	}

	mustExecPhase151C(t, ctx, db, `
SELECT memory_worker_complete_job($1, $2, $3);
UPDATE memory_capture_candidate_batches
SET created_at = now() - interval '31 days',
    review_expires_at = now() - interval '1 day'
WHERE capture_job_id = $1;
UPDATE user_memory_review_suggestions
SET created_at = now() - interval '31 days',
    review_expires_at = now() - interval '1 day'
WHERE capture_job_id = $1;
UPDATE memory_jobs SET available_at = now() WHERE job_id = $4;
`, extractJob, workerID, extractLease, expiryJob)
	claimMemoryJob(t, ctx, db, workerID, expiryLease, expiryJob, 1)
	var expired int
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_expire_capture_reviews($1, $2, $3)
`, expiryJob, workerID, expiryLease).Scan(&expired); err != nil || expired != 4 {
		t.Fatalf("expire reviews = %d/%v", expired, err)
	}
	var plaintextCount, expiredCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE candidate_content IS NOT NULL OR normalized_content IS NOT NULL
    OR cardinality(tags) > 0 OR subject_key IS NOT NULL OR fact_key IS NOT NULL),
  count(*) FILTER (WHERE status = 'expired' AND result_code = 'PLAINTEXT_EXPIRED')
FROM user_memory_review_suggestions WHERE capture_job_id = $1
`, extractJob).Scan(&plaintextCount, &expiredCount); err != nil {
		t.Fatal(err)
	}
	if plaintextCount != 0 || expiredCount != 4 {
		t.Fatalf("expired plaintext = %d/%d", plaintextCount, expiredCount)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_REVIEW_ROLLBACK_REQUIRES_EMPTY_HISTORY") {
		t.Fatalf("guarded 056 rollback error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
DELETE FROM memory_jobs WHERE stage = 'review_expire';
DELETE FROM memory_capture_candidate_batches;
UPDATE memory_outbox SET max_attempts = 8 WHERE event_id = $1;
`, eventID)
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 56 {
		t.Fatalf("clean 056 rollback = %#v/%v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 56 {
		t.Fatalf("056 re-up = %#v/%v", reapplied, err)
	}
}

func memoryReviewProposal(
	id string,
	sourceID string,
	observedAt time.Time,
	content string,
	normalized string,
	scope string,
	projectID any,
	conversationID any,
	sensitivity string,
	action string,
	targets []string,
) map[string]any {
	digest := sha256.Sum256([]byte(content))
	if targets == nil {
		targets = []string{}
	}
	return map[string]any{
		"id":                      id,
		"type":                    "preference",
		"content":                 content,
		"normalizedContent":       normalized,
		"candidateHash":           hex.EncodeToString(digest[:]),
		"importance":              5,
		"tags":                    []string{"style"},
		"subjectKey":              "user",
		"factKey":                 "response.style",
		"sensitivity":             sensitivity,
		"confidence":              0.95,
		"confidenceBand":          "high",
		"authorityUserMessageIds": []string{sourceID},
		"contextMessageIds":       []string{},
		"confirmationKind":        "explicit_user",
		"proposedScopeType":       scope,
		"proposedProjectId":       projectID,
		"proposedConversationId":  conversationID,
		"scopeConfidence":         0.95,
		"temporalBasis":           "none",
		"temporalParserVersion":   nil,
		"observedAt":              observedAt.UTC().Format(time.RFC3339Nano),
		"validFrom":               nil,
		"validTo":                 nil,
		"factExpiresAt":           nil,
		"proposedAction":          action,
		"targetMemoryIds":         targets,
	}
}
