package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMemoryGovernanceUILivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 59))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 059: %v", err)
	}

	const (
		userID         = "16000000-0000-4000-8000-000000000001"
		otherUserID    = "16000000-0000-4000-8000-000000000002"
		conversationID = "26000000-0000-4000-8000-000000000001"
		otherChatID    = "26000000-0000-4000-8000-000000000002"
		sourceID       = "36000000-0000-4000-8000-000000000001"
		assistantID    = "36000000-0000-4000-8000-000000000002"
		eventID        = "66000000-0000-4000-8000-000000000001"
		jobID          = "76000000-0000-4000-8000-000000000001"
		expiryJobID    = "76000000-0000-4000-8000-000000000002"
		workerID       = "86000000-0000-4000-8000-000000000001"
		leaseID        = "96000000-0000-4000-8000-000000000001"
		providerID     = "46000000-0000-4000-8000-000000000099"
		projectID      = "46000000-0000-4000-8000-000000000001"
		scopedMemoryID = "56000000-0000-4000-8000-000000000099"
	)
	manualIDs := []string{
		"56000000-0000-4000-8000-000000000001",
		"56000000-0000-4000-8000-000000000002",
		"56000000-0000-4000-8000-000000000003",
		"56000000-0000-4000-8000-000000000004",
		"56000000-0000-4000-8000-000000000005",
	}
	reviewIDs := []string{
		"a6000000-0000-4000-8000-000000000001",
		"a6000000-0000-4000-8000-000000000002",
		"a6000000-0000-4000-8000-000000000003",
		"a6000000-0000-4000-8000-000000000004",
		"a6000000-0000-4000-8000-000000000005",
	}
	observedAt := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR9'), ($2, 'PR9 Other');
INSERT INTO conversations(id, user_id, title) VALUES
  ($3, $1, 'PR9 Conversation'), ($8, $1, 'PR9 Other Conversation');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at, created_at, updated_at
) VALUES ($4, $3, $1, 1, 'user', 'completed',
  'Please update my reply preference', $7, $7, $7);
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, completed_at, created_at, updated_at
) VALUES ($5, $3, $1, $4, 2, 'assistant', 'completed',
  'Understood', $7, $7, $7);
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES
  ($1, true, true, true, true),
  ($2, true, true, false, false);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $6, $1, 'fixture', 'Fixture', '{}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
`, userID, otherUserID, conversationID, sourceID, assistantID, providerID, observedAt, otherChatID)

	for index, memoryID := range manualIDs {
		content := "Current preference " + string(rune('A'+index))
		if _, err := db.ExecContext(ctx, `
SELECT id FROM memory_upsert_global_manual(
  $1::uuid, $2::uuid, 'preference', $3, lower($3),
  4::smallint, ARRAY['style']::text[], NULL, NULL, true
)
`, memoryID, userID, content); err != nil {
			t.Fatalf("seed manual memory %d: %v", index, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, eventID, jobID, userID, conversationID, sourceID, assistantID); err != nil {
		t.Fatalf("append PR9 capture: %v", err)
	}
	claimMemoryJob(t, ctx, db, workerID, leaseID, jobID, 1)
	proposals := make([]map[string]any, 0, len(reviewIDs))
	for index, reviewID := range reviewIDs {
		content := "Proposed preference " + string(rune('A'+index))
		proposals = append(proposals, memoryReviewProposal(
			reviewID, sourceID, observedAt, content, strings.ToLower(content),
			"global", nil, nil, "normal", "SUPERSEDE", []string{manualIDs[index]},
		))
	}
	proposalJSON, err := json.Marshal(proposals)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, jobID, workerID, leaseID, expiryJobID, string(proposalJSON)); err != nil {
		t.Fatalf("create PR9 review fixtures: %v", err)
	}

	runner := NewRunner(db, phase15MigrationFSThrough(t, 60))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 60 {
		t.Fatalf("apply 060 = %#v/%v", applied, err)
	}
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 60 {
		t.Fatalf("clean down 060 = %#v/%v", rolledBack, err)
	}
	assertGovernanceRuntimeLegacyFunctionAllowed(t, ctx, db, userID)
	applied, err = runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 60 {
		t.Fatalf("re-up 060 = %#v/%v", applied, err)
	}
	assertGovernanceRuntimeLegacyFunctionDenied(t, ctx, db, `
SELECT id FROM memory_upsert_global_manual(
  '56000000-0000-4000-8000-000000000090'::uuid, $1::uuid,
  'fact', 'legacy bypass', 'legacy bypass', 3::smallint,
  '{}'::text[], NULL, NULL, true
)
`, userID)
	assertGovernanceRuntimeLegacyFunctionDenied(t, ctx, db, `
SELECT id FROM memory_update_global_manual(
  '56000000-0000-4000-8000-000000000001'::uuid, $1::uuid,
  'fact', 'legacy bypass', 'legacy bypass', 3::smallint,
  '{}'::text[], true
)
`, userID)
	assertGovernanceRuntimeLegacyWrappers(t, ctx, db, userID)

	assertGovernanceRuntimeTableDenied(t, ctx, db, "projects")
	assertGovernanceRuntimeTableDenied(t, ctx, db, "user_memory_review_suggestions")
	assertGovernanceRuntimeTableDenied(t, ctx, db, "user_memory_evidence")
	assertGovernanceRuntimeTableDenied(t, ctx, db, "user_memory_revisions")
	assertGovernanceRuntimeTableDenied(t, ctx, db, "message_memory_hybrid_shadow_observations")

	var projectJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_create_project($1, $2, 'Neo Chat', 'Memory v2')
`, userID, projectID).Scan(&projectJSON); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if !strings.Contains(string(projectJSON), `"name": "Neo Chat"`) {
		t.Fatalf("project JSON = %s", projectJSON)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_update_project(
  $1, $2, 1, 'stolen', '', 'active'
)
`, otherUserID, projectID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_PROJECT_NOT_FOUND") {
		t.Fatalf("cross-user project update = %v", err)
	}

	var policyJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_conversation_policy(
  $1, $2, 1, $3, 'on', 'on'
)
`, userID, conversationID, projectID).Scan(&policyJSON); err != nil {
		t.Fatalf("assign conversation policy: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_update_conversation_policy(
  $1, $2, 1, NULL, 'off', 'off'
)
`, userID, conversationID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_SCOPE_STALE") {
		t.Fatalf("stale conversation policy = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_update_conversation_policy(
  $1, $2, 2, $3, 'on', 'on'
)
`, otherUserID, conversationID, projectID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_CONVERSATION_NOT_FOUND") {
		t.Fatalf("cross-user conversation policy = %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_project(
  $1, $2, 1, 'Neo Chat', 'Memory v2', 'archived'
)
`, userID, projectID).Scan(&projectJSON); err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_update_conversation_policy(
  $1, $2, 1, $3, 'inherit', 'inherit'
)
`, userID, otherChatID, projectID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_POLICY_INVALID") {
		t.Fatalf("new assignment to archived Project = %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_get_conversation_policy($1, $2)
`, userID, conversationID).Scan(&policyJSON); err != nil ||
		!strings.Contains(string(policyJSON), `"effectiveLearn": false`) ||
		!strings.Contains(string(policyJSON), `"learnForcedOff": true`) {
		t.Fatalf("archive effective policy = %s/%v", policyJSON, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_project(
  $1, $2, 2, 'Neo Chat', 'Memory v2', 'active'
)
`, userID, projectID).Scan(&projectJSON); err != nil {
		t.Fatalf("restore project: %v", err)
	}

	var memoryJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_create_memory(
  $1, $2, 'project', 'Neo Chat uses Go', 'neo chat uses go',
  4::smallint, ARRAY['stack']::text[], 'project', $3, NULL, 'normal'
)
`, userID, scopedMemoryID, projectID).Scan(&memoryJSON); err != nil {
		t.Fatalf("create scoped memory: %v", err)
	}
	secretFixtures := []struct {
		id      string
		content string
	}{
		{"56000000-0000-4000-8000-000000000098", "API key: sk-secretvalue"},
		{"56000000-0000-4000-8000-000000000097", "cookie: fixture-cookie-value"},
		{"56000000-0000-4000-8000-000000000096", "session_id is fixture-session-value"},
		{"56000000-0000-4000-8000-000000000095", "Authorization: Bearer fixture-bearer-value"},
		{"56000000-0000-4000-8000-000000000094", "cvv=1234"},
		{"56000000-0000-4000-8000-000000000093", "eyJabcdefgh.eyJijklmnop.qrstuvwx"},
	}
	secretIDs := make([]string, 0, len(secretFixtures))
	for _, fixture := range secretFixtures {
		secretIDs = append(secretIDs, fixture.id)
		if _, err := db.ExecContext(ctx, `
SELECT memory_governance_create_memory(
  $1, $2, 'fact', $3, lower($3),
  3::smallint, '{}'::text[], 'global', NULL, NULL, 'normal'
)
`, userID, fixture.id, fixture.content); err == nil ||
			!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_MEMORY_INVALID") {
			t.Fatalf("secret manual memory %q = %v", fixture.content, err)
		}
	}
	var secretCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memories
WHERE user_id = $1 AND id = ANY($2::uuid[])
`, userID, secretIDs).Scan(&secretCount); err != nil || secretCount != 0 {
		t.Fatalf("secret plaintext count = %d/%v", secretCount, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_memory(
  $1, $2, 1, 'project', 'Neo Chat uses Go and PostgreSQL',
  'neo chat uses go and postgresql', 5::smallint, ARRAY['stack']::text[],
  'conversation', NULL, $3, 'normal'
)
`, userID, scopedMemoryID, conversationID).Scan(&memoryJSON); err != nil ||
		!strings.Contains(string(memoryJSON), `"scopeType": "conversation"`) ||
		!strings.Contains(string(memoryJSON), `"revision": 2`) {
		t.Fatalf("move scoped memory = %s/%v", memoryJSON, err)
	}
	var sensitiveJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_create_memory(
  $1, '56000000-0000-4000-8000-000000000092'::uuid,
  'fact', '我患有糖尿病', '我患有糖尿病',
  4::smallint, '{}'::text[], 'global', NULL, NULL, 'normal'
)
`, userID).Scan(&sensitiveJSON); err != nil ||
		!strings.Contains(string(sensitiveJSON), `"sensitivity": "sensitive"`) {
		t.Fatalf("sensitive downgrade defense = %s/%v", sensitiveJSON, err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_settings
SET sensitive_memory_enabled = false
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("disable Sensitive Memory: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_create_memory(
  $1, '56000000-0000-4000-8000-000000000091'::uuid,
  'fact', '我的家庭住址在测试路九号', '我的家庭住址在测试路九号',
  4::smallint, '{}'::text[], 'global', NULL, NULL, 'normal'
)
`, userID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_SENSITIVE_DISABLED") {
		t.Fatalf("disabled Sensitive downgrade = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_settings
SET sensitive_memory_enabled = true
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("restore Sensitive Memory: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_memory_detail($1, $2)
`, otherUserID, scopedMemoryID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_MEMORY_NOT_FOUND") {
		t.Fatalf("cross-user Memory detail = %v", err)
	}
	var detailJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_memory_detail($1, $2)
`, userID, scopedMemoryID).Scan(&detailJSON); err != nil ||
		!strings.Contains(string(detailJSON), `"operation": "update"`) {
		t.Fatalf("Memory detail = %s/%v", detailJSON, err)
	}
	const (
		governanceDeleteEventID     = "66000000-0000-4000-8000-000000000009"
		governanceDeleteJobID       = "76000000-0000-4000-8000-000000000009"
		governanceDeleteTombstoneID = "86000000-0000-4000-8000-000000000009"
		governanceDeleteManifestID  = "96000000-0000-4000-8000-000000000009"
		governanceDeleteWorkerID    = "a6000000-0000-4000-8000-000000000009"
		governanceDeleteLeaseID     = "b6000000-0000-4000-8000-000000000009"
	)
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_delete_memory($1, $2, 2, $3, $4, $5, $6)
`, otherUserID, scopedMemoryID, governanceDeleteEventID, governanceDeleteJobID,
		governanceDeleteTombstoneID, governanceDeleteManifestID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_REVISION_STALE") {
		t.Fatalf("cross-user scoped Memory forget = %v", err)
	}
	var deletionJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_delete_memory($1, $2, 2, $3, $4, $5, $6)
`, userID, scopedMemoryID, governanceDeleteEventID, governanceDeleteJobID,
		governanceDeleteTombstoneID, governanceDeleteManifestID).Scan(&deletionJSON); err != nil ||
		!strings.Contains(string(deletionJSON), `"immediateHidden": true`) ||
		!strings.Contains(string(deletionJSON), `"onlinePurgeStatus": "pending"`) {
		t.Fatalf("scoped Memory forget = %s/%v", deletionJSON, err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE memory_jobs
SET status = 'processing', attempt_count = 1,
    lease_owner = $2, lease_token = $3,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
WHERE job_id = $1;
UPDATE memory_outbox outbox
SET status = 'processing', attempt_count = 1,
    lease_owner = $2, lease_token = $3,
    lease_expires_at = now() + interval '1 minute', updated_at = now()
FROM memory_jobs job
WHERE job.job_id = $1 AND outbox.event_id = job.event_id;
`, governanceDeleteJobID, governanceDeleteWorkerID, governanceDeleteLeaseID); err != nil {
		t.Fatalf("lease governance purge: %v", err)
	}
	var purged bool
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_purge_memory($1, $2, $3)
`, governanceDeleteJobID, governanceDeleteWorkerID,
		governanceDeleteLeaseID).Scan(&purged); err != nil || !purged {
		t.Fatalf("governance provider-free purge = %t/%v", purged, err)
	}
	var purgedPlaintext int
	var purgedResult string
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memories memory
   WHERE memory.id = $1 AND (
     memory.content <> '' OR memory.normalized_content <> ''
     OR cardinality(memory.tags) > 0
   )),
  (SELECT result_code FROM user_memory_deletion_manifests
   WHERE manifest_id = $2)
`, scopedMemoryID, governanceDeleteManifestID).Scan(
		&purgedPlaintext, &purgedResult,
	); err != nil || purgedPlaintext != 0 || purgedResult != "ONLINE_PURGED" {
		t.Fatalf("governance purge state = %d/%q/%v",
			purgedPlaintext, purgedResult, err)
	}

	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_memory(
  $1, $2, 1, 'preference', 'Current preference A revised',
  'current preference a revised', 4::smallint, ARRAY['style']::text[],
  'global', NULL, NULL, 'normal'
)
`, userID, manualIDs[0]).Scan(&memoryJSON); err != nil {
		t.Fatalf("drift Review target: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, 'b6000000-0000-4000-8000-000000000096'::uuid,
  'keep_current', NULL, NULL, NULL, repeat('d',64)
)
`, userID, reviewIDs[0]); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_REVIEW_STALE") {
		t.Fatalf("stale keep-current Review = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_review_targets SET expected_revision = 2
WHERE suggestion_id = $1 AND user_id = $2
`, reviewIDs[0], userID); err != nil {
		t.Fatalf("refresh Review target fixture: %v", err)
	}

	var concurrentDecisions sync.WaitGroup
	concurrentErrors := make(chan error, 2)
	for range 2 {
		concurrentDecisions.Add(1)
		go func() {
			defer concurrentDecisions.Done()
			var result []byte
			err := db.QueryRowContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, 'b6000000-0000-4000-8000-000000000002'::uuid,
  'accept_new', '57000000-0000-4000-8000-000000000002'::uuid,
  NULL, NULL, repeat('2',64)
)
`, userID, reviewIDs[1]).Scan(&result)
			if err == nil && !strings.Contains(string(result),
				`"memoryId": "57000000-0000-4000-8000-000000000002"`) {
				err = errors.New("concurrent Review returned the wrong Memory")
			}
			concurrentErrors <- err
		}()
	}
	concurrentDecisions.Wait()
	close(concurrentErrors)
	for err := range concurrentErrors {
		if err != nil {
			t.Fatalf("concurrent Review replay: %v", err)
		}
	}

	decisions := []struct {
		kind       string
		memoryID   sql.NullString
		edited     sql.NullString
		normalized sql.NullString
		hash       string
	}{
		{"keep_current", sql.NullString{}, sql.NullString{}, sql.NullString{}, strings.Repeat("1", 64)},
		{"accept_new", sql.NullString{String: "57000000-0000-4000-8000-000000000002", Valid: true}, sql.NullString{}, sql.NullString{}, strings.Repeat("2", 64)},
		{"edit_merge", sql.NullString{String: "57000000-0000-4000-8000-000000000003", Valid: true}, sql.NullString{String: "我患有糖尿病，回复请简洁", Valid: true}, sql.NullString{String: "我患有糖尿病 回复请简洁", Valid: true}, strings.Repeat("3", 64)},
		{"keep_both", sql.NullString{String: "57000000-0000-4000-8000-000000000004", Valid: true}, sql.NullString{}, sql.NullString{}, strings.Repeat("4", 64)},
		{"reject", sql.NullString{}, sql.NullString{}, sql.NullString{}, strings.Repeat("5", 64)},
	}
	for index, decision := range decisions {
		decisionID := "b6000000-0000-4000-8000-00000000000" + string(rune('1'+index))
		var result []byte
		if err := db.QueryRowContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, $3, $4, $5, $6, $7, $8
)
`, userID, reviewIDs[index], decisionID, decision.kind,
			decision.memoryID, decision.edited, decision.normalized, decision.hash).Scan(&result); err != nil {
			t.Fatalf("decision %s: %v", decision.kind, err)
		}
		if !strings.Contains(string(result), `"decision": "`+decision.kind+`"`) {
			t.Fatalf("decision %s result = %s", decision.kind, result)
		}
	}
	var mergedSensitivity string
	if err := db.QueryRowContext(ctx, `
SELECT sensitivity FROM user_memories
WHERE id = '57000000-0000-4000-8000-000000000003'::uuid
  AND user_id = $1
`, userID).Scan(&mergedSensitivity); err != nil || mergedSensitivity != "sensitive" {
		t.Fatalf("edit-merge sensitive downgrade = %q/%v", mergedSensitivity, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_memory(
  $1, '57000000-0000-4000-8000-000000000002'::uuid, 1,
  'preference', 'Proposed preference B', 'proposed preference b',
  5::smallint, ARRAY['style']::text[], 'project', $2, NULL, 'normal'
)
`, userID, projectID).Scan(&memoryJSON); err != nil {
		t.Fatalf("move confirmed Review Memory: %v", err)
	}
	var movedSource, movedAuthority, movedFactKey, movedScope string
	var movedRevision int64
	if err := db.QueryRowContext(ctx, `
SELECT source, authority_kind, fact_key, scope_type, revision
FROM user_memories
WHERE id = '57000000-0000-4000-8000-000000000002'::uuid
  AND user_id = $1
`, userID).Scan(
		&movedSource, &movedAuthority, &movedFactKey, &movedScope, &movedRevision,
	); err != nil || movedSource != "ai" || movedAuthority != "confirmed" ||
		movedFactKey != "response.style" || movedScope != "project" || movedRevision != 2 {
		t.Fatalf("pure move authority = %q/%q/%q/%q/r%d err=%v",
			movedSource, movedAuthority, movedFactKey, movedScope, movedRevision, err)
	}
	var pendingPlaintext int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_review_suggestions
WHERE id = ANY($1::uuid[])
  AND (candidate_content IS NOT NULL OR normalized_content IS NOT NULL)
`, reviewIDs).Scan(&pendingPlaintext); err != nil || pendingPlaintext != 0 {
		t.Fatalf("decided Review plaintext = %d/%v", pendingPlaintext, err)
	}

	var replay []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, 'b6000000-0000-4000-8000-000000000099'::uuid,
  'accept_new', '57000000-0000-4000-8000-000000000099'::uuid,
  NULL, NULL, $3
)
`, userID, reviewIDs[1], decisions[1].hash).Scan(&replay); err != nil ||
		!strings.Contains(string(replay), `"memoryId": "57000000-0000-4000-8000-000000000002"`) {
		t.Fatalf("Review replay = %s/%v", replay, err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, 'b6000000-0000-4000-8000-000000000098'::uuid,
  'accept_new', '57000000-0000-4000-8000-000000000098'::uuid,
  NULL, NULL, repeat('f',64)
)
`, userID, reviewIDs[1]); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_REPLAY_CONFLICT") {
		t.Fatalf("Review replay conflict = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, 'b6000000-0000-4000-8000-000000000097'::uuid,
  'reject', NULL, NULL, NULL, repeat('e',64)
)
`, otherUserID, reviewIDs[0]); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_REVIEW_NOT_FOUND") {
		t.Fatalf("cross-user Review = %v", err)
	}

	var activitiesJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_list_message_activities($1, $2, 20)
`, userID, assistantID).Scan(&activitiesJSON); err != nil {
		t.Fatalf("message Activities: %v", err)
	}
	var activities []map[string]any
	if err := json.Unmarshal(activitiesJSON, &activities); err != nil || len(activities) == 0 {
		t.Fatalf("decode Activities = %#v/%v", activities, err)
	}
	if _, ok := activities[0]["createdAt"].(float64); !ok {
		t.Fatalf("Activity createdAt is not epoch millis: %#v", activities[0]["createdAt"])
	}
	if activities[0]["sourceKind"] != "review_suggestion" ||
		activities[0]["scopeType"] != "global" {
		t.Fatalf("Activity governance metadata = %#v", activities[0])
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memories SET enabled = false
WHERE id = '57000000-0000-4000-8000-000000000002'::uuid
  AND user_id = $1
`, userID); err != nil {
		t.Fatalf("hide accepted Review Memory: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_list_message_activities($1, $2, 20)
`, userID, assistantID).Scan(&activitiesJSON); err != nil {
		t.Fatalf("hidden message Activities: %v", err)
	}
	activities = nil
	if err := json.Unmarshal(activitiesJSON, &activities); err != nil {
		t.Fatalf("decode hidden Activities = %v", err)
	}
	foundHidden := false
	for _, activity := range activities {
		if activity["subjectId"] != "57000000-0000-4000-8000-000000000002" {
			continue
		}
		foundHidden = true
		if activity["memoryDeleted"] != true || activity["memoryContent"] != "" ||
			activity["memoryType"] != "" || activity["memoryRevision"] != nil {
			t.Fatalf("hidden Activity leaked Memory state = %#v", activity)
		}
	}
	if !foundHidden {
		t.Fatal("accepted Review Activity was not returned")
	}
	if _, err := db.ExecContext(ctx, `
UPDATE conversations SET deleted_at = clock_timestamp()
WHERE id = $1 AND user_id = $2
`, conversationID, userID); err != nil {
		t.Fatalf("soft-delete evidence Conversation: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_memory_detail(
  $1, '57000000-0000-4000-8000-000000000002'::uuid
)
`, userID).Scan(&detailJSON); err != nil ||
		!strings.Contains(string(detailJSON), `"sourceDeleted": true`) ||
		strings.Contains(string(detailJSON), "Please update my reply preference") {
		t.Fatalf("deleted source detail = %s/%v", detailJSON, err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_governance_list_message_activities($1, $2, 20)
`, userID, assistantID); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_ACTIVITY_INVALID") {
		t.Fatalf("deleted Conversation Activity = %v", err)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_NO_DECISIONS") {
		t.Fatalf("guarded down 060 = %v", err)
	}
}

func assertGovernanceRuntimeTableDenied(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	table string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatalf("set go_api_runtime: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT * FROM `+table+` LIMIT 1`); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("go_api_runtime direct %s read = %v", table, err)
	}
}

func assertGovernanceRuntimeLegacyFunctionAllowed(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatalf("set go_api_runtime: %v", err)
	}
	var memoryID string
	if err := tx.QueryRowContext(ctx, `
SELECT id FROM memory_upsert_global_manual(
  '56000000-0000-4000-8000-000000000090'::uuid, $1::uuid,
  'fact', 'rollback grant fixture', 'rollback grant fixture', 3::smallint,
  '{}'::text[], NULL, NULL, true
)
`, userID).Scan(&memoryID); err != nil || memoryID == "" {
		t.Fatalf("060 down did not restore legacy execute = %q/%v", memoryID, err)
	}
}

func assertGovernanceRuntimeLegacyFunctionDenied(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	query string,
	args ...any,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatalf("set go_api_runtime: %v", err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("go_api_runtime legacy function bypass = %v", err)
	}
}

func assertGovernanceRuntimeLegacyWrappers(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
) {
	t.Helper()
	create := func(content string) ([]byte, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
			return nil, err
		}
		var payload []byte
		err = tx.QueryRowContext(ctx, `
SELECT memory_governance_upsert_global_legacy(
  '56000000-0000-4000-8000-000000000089'::uuid, $1::uuid,
  'fact', $2, lower($2), 3::smallint, '{}'::text[], NULL, NULL, true
)
`, userID, content).Scan(&payload)
		return payload, err
	}
	update := func(content string) ([]byte, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
SELECT memory_governance_upsert_global_legacy(
  '56000000-0000-4000-8000-000000000088'::uuid, $1::uuid,
  'fact', 'ordinary update fixture', 'ordinary update fixture',
  3::smallint, '{}'::text[], NULL, NULL, true
)
`, userID); err != nil {
			return nil, err
		}
		var payload []byte
		err = tx.QueryRowContext(ctx, `
SELECT memory_governance_update_global_legacy(
  '56000000-0000-4000-8000-000000000088'::uuid, $1::uuid,
  'fact', $2, lower($2), 4::smallint, '{}'::text[], true
)
`, userID, content).Scan(&payload)
		return payload, err
	}

	payload, err := create("ordinary governed fixture")
	if err != nil || !strings.Contains(string(payload), `"content": "ordinary governed fixture"`) {
		t.Fatalf("governed legacy wrapper = %s/%v", payload, err)
	}
	payload, err = update("ordinary updated fixture")
	if err != nil || !strings.Contains(string(payload), `"content": "ordinary updated fixture"`) {
		t.Fatalf("governed legacy update wrapper = %s/%v", payload, err)
	}
	if _, err := create("password: fixture-secret"); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_MEMORY_INVALID") {
		t.Fatalf("governed legacy secret rejection = %v", err)
	}
	if _, err := update("password: fixture-secret"); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_MEMORY_INVALID") {
		t.Fatalf("governed legacy update secret rejection = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_settings SET sensitive_memory_enabled = false
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("disable Sensitive Memory for wrapper proof: %v", err)
	}
	if _, err := create("我患有糖尿病"); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_SENSITIVE_DISABLED") {
		t.Fatalf("governed legacy Sensitive gate = %v", err)
	}
	if _, err := update("我的工资是测试金额"); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_GOVERNANCE_SENSITIVE_DISABLED") {
		t.Fatalf("governed legacy update Sensitive gate = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE user_memory_settings SET sensitive_memory_enabled = true
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("restore Sensitive Memory after wrapper proof: %v", err)
	}
	payload, err = create("我患有糖尿病")
	if err != nil || !strings.Contains(string(payload), `"sensitivity": "sensitive"`) {
		t.Fatalf("governed legacy Sensitive wrapper = %s/%v", payload, err)
	}
	payload, err = update("我的工资是测试金额")
	if err != nil || !strings.Contains(string(payload), `"sensitivity": "sensitive"`) {
		t.Fatalf("governed legacy Sensitive update wrapper = %s/%v", payload, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatalf("set go_api_runtime: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
SELECT memory_governance_upsert_global_legacy(
  '56000000-0000-4000-8000-000000000087'::uuid, $1::uuid,
  'fact', '我患有糖尿病', '我患有糖尿病', 3::smallint,
  '{}'::text[], NULL, NULL, true
)
`, userID); err != nil {
		t.Fatalf("seed governed Sensitive legacy Memory: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_governance_update_global_legacy(
  '56000000-0000-4000-8000-000000000087'::uuid, $1::uuid,
  'fact', 'ordinary reclassified fixture', 'ordinary reclassified fixture',
  4::smallint, '{}'::text[], true
)
`, userID).Scan(&payload); err != nil ||
		!strings.Contains(string(payload), `"sensitivity": "normal"`) {
		t.Fatalf("governed legacy declassification = %s/%v", payload, err)
	}
}
