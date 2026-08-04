package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMemoryAutoCapturePromotionLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 68))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 068: %v", err)
	}

	const (
		userID         = "16000000-0000-4000-8000-000000000066"
		projectID      = "16000000-0000-4000-8000-000000000067"
		conversationID = "26000000-0000-4000-8000-000000000066"
		sourceID       = "36000000-0000-4000-8000-000000000066"
		assistantID    = "36000000-0000-4000-8000-000000000067"
		providerID     = "46000000-0000-4000-8000-000000000066"
		eventID        = "66000000-0000-4000-8000-000000000066"
		jobID          = "76000000-0000-4000-8000-000000000066"
		expiryJobID    = "76000000-0000-4000-8000-000000000067"
		workerID       = "86000000-0000-4000-8000-000000000066"
		leaseID        = "96000000-0000-4000-8000-000000000066"
		firstReviewID  = "a6000000-0000-4000-8000-000000000066"
		secondReviewID = "a6000000-0000-4000-8000-000000000067"
		tombReviewID   = "a6000000-0000-4000-8000-000000000068"
		tempReviewID   = "a6000000-0000-4000-8000-000000000069"
		sensitiveID    = "a6000000-0000-4000-8000-000000000070"
		tombMemoryID   = "56000000-0000-4000-8000-000000000066"
		tombstoneID    = "b6000000-0000-4000-8000-000000000066"
	)
	observedAt := time.Date(2026, 8, 4, 2, 30, 0, 0, time.UTC)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR14 Auto Capture');
INSERT INTO projects(id, user_id, name) VALUES ($7, $1, 'Auto Capture');
INSERT INTO conversations(id, user_id, title, project_id)
VALUES ($2, $1, 'Auto Capture', $7);
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at, created_at, updated_at
) VALUES (
  $3, $2, $1, 1, 'user', 'completed', '西北工业大学', $6, $6, $6
);
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content, completed_at, created_at, updated_at
) VALUES (
  $4, $2, $1, $3, 2, 'assistant', 'completed',
  '你是西北工业大学的。', $6, $6, $6
);
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES ($1, true, true, true, false);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $5, $1, 'fixture', 'Fixture', '{}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
`, userID, conversationID, sourceID, assistantID, providerID, observedAt, projectID)

	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, eventID, jobID, userID, conversationID, sourceID, assistantID); err != nil {
		t.Fatalf("append capture: %v", err)
	}
	claimMemoryJob(t, ctx, db, workerID, leaseID, jobID, 1)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 69))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 69 {
		t.Fatalf("apply 069 = %#v/%v", applied, err)
	}
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 69 {
		t.Fatalf("clean down 069 to 068 = %#v/%v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 69 {
		t.Fatalf("re-up 069 = %#v/%v", reapplied, err)
	}
	assertAutoCaptureWorkerTableDenied(t, ctx, db)

	first := memoryReviewProposal(
		firstReviewID, sourceID, observedAt,
		"用户就读于西北工业大学", "用户就读于西北工业大学",
		"global", nil, nil, "normal", "ADD", nil,
	)
	first["type"] = "fact"
	first["tags"] = []string{"education"}
	first["factKey"] = "user.school"
	second := memoryReviewProposal(
		secondReviewID, sourceID, observedAt,
		"用户就读于另一所学校", "用户就读于另一所学校",
		"global", nil, nil, "normal", "ADD", nil,
	)
	second["type"] = "fact"
	second["tags"] = []string{"education"}
	second["factKey"] = "user.school"
	second["confirmationKind"] = "confirmed_assistant"
	second["contextMessageIds"] = []string{assistantID}
	tomb := memoryReviewProposal(
		tombReviewID, sourceID, observedAt,
		"临时访问代码已经失效", "临时访问代码已经失效",
		"global", nil, nil, "normal", "ADD", nil,
	)
	tomb["type"] = "fact"
	tomb["tags"] = []string{"temporary"}
	tomb["factKey"] = "temporary.access-code"
	temporary := memoryReviewProposal(
		tempReviewID, sourceID, observedAt,
		"本周临时住在学校招待所", "本周临时住在学校招待所",
		"global", nil, nil, "normal", "ADD", nil,
	)
	temporary["type"] = "context"
	temporary["tags"] = []string{"temporary"}
	temporary["factKey"] = "temporary.location"
	temporary["temporalBasis"] = "explicit_absolute"
	temporary["temporalParserVersion"] = "rfc3339-v1"
	temporary["validFrom"] = observedAt.UTC().Format(time.RFC3339Nano)
	temporary["factExpiresAt"] = observedAt.Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	sensitive := memoryReviewProposal(
		sensitiveID, sourceID, observedAt,
		"用户患有糖尿病", "用户患有糖尿病",
		"global", nil, nil, "sensitive", "ADD", nil,
	)
	sensitive["type"] = "fact"
	sensitive["tags"] = []string{"health"}
	sensitive["factKey"] = "user.health"
	proposalJSON, err := json.Marshal(
		[]map[string]any{first, second, tomb, temporary, sensitive},
	)
	if err != nil {
		t.Fatal(err)
	}
	var processingProfile string
	if err := db.QueryRowContext(ctx, `
SELECT processing_profile FROM memory_jobs WHERE job_id = $1::uuid
`, jobID).Scan(&processingProfile); err != nil {
		t.Fatalf("query processing profile: %v", err)
	}
	extractionDigest := sha256.Sum256([]byte(
		processingProfile + "\x1f" + "memory-capture-candidate-tool-v5",
	))
	decisionDigest := sha256.Sum256([]byte(
		processingProfile + "\x1f" + "memory-capture-decision-tool-v2",
	))
	expectedExtractionProfile := hex.EncodeToString(extractionDigest[:])
	expectedDecisionProfile := hex.EncodeToString(decisionDigest[:])
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, $5, $6, $7::jsonb
)
`, jobID, workerID, leaseID, expiryJobID,
		expectedExtractionProfile, expectedDecisionProfile, string(proposalJSON)); err != nil {
		t.Fatalf("propose auto-capture fixtures: %v", err)
	}

	// Create a deleted exact row only after proposal routing. The promotion
	// boundary must observe its tombstone and must not resurrect it.
	if _, err := db.ExecContext(ctx, `
SELECT id FROM memory_upsert_global_manual(
  $1::uuid, $2::uuid, 'fact', $3, $4,
  3::smallint, ARRAY['temporary']::text[], NULL, NULL, true
)
`, tombMemoryID, userID, tomb["content"], tomb["normalizedContent"]); err != nil {
		t.Fatalf("seed tombstoned memory: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories
SET enabled = false, deleted_at = clock_timestamp()
WHERE id = $1::uuid AND user_id = $2::uuid;
INSERT INTO user_memory_tombstones(
  id, user_id, memory_id, content_hash, fact_key, reason
)
SELECT $3::uuid, user_id, id, content_hash, 'temporary.access-code', 'user_delete'
FROM user_memories WHERE id = $1::uuid AND user_id = $2::uuid;
`, tombMemoryID, userID, tombstoneID)

	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID,
		"96000000-0000-4000-8000-000000000067",
		"MEMORY_JOB_LEASE_LOST",
	)

	// Every authority that may drift after the candidate batch commits is
	// rechecked at promotion time. Restore one variable at a time so the final
	// happy path starts from the exact original authority state.
	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET content = 'stale source' WHERE id = $1::uuid;
`, sourceID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_CAPTURE_SOURCE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET content = '西北工业大学' WHERE id = $1::uuid;
`, sourceID)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_evidence
SET source_content_hash = repeat('f', 64)
WHERE suggestion_id = $1::uuid AND evidence_role = 'user';
`, firstReviewID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_CAPTURE_SOURCE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_evidence evidence
SET source_content_hash = job.source_hash
FROM memory_jobs job
WHERE evidence.suggestion_id = $1::uuid
  AND evidence.evidence_role = 'user'
  AND job.job_id = $2::uuid;
`, firstReviewID, jobID)

	mustExecPhase151C(t, ctx, db, `
UPDATE memory_capture_candidate_batches
SET extraction_profile_id = repeat('f', 64)
WHERE capture_job_id = $1::uuid;
`, jobID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_PROFILE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_capture_candidate_batches
SET extraction_profile_id = $2
WHERE capture_job_id = $1::uuid;
`, jobID, expectedExtractionProfile)

	mustExecPhase151C(t, ctx, db, `
UPDATE memory_capture_candidate_batches
SET proposal_count = proposal_count - 1
WHERE capture_job_id = $1::uuid;
`, jobID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_PROFILE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_capture_candidate_batches
SET proposal_count = proposal_count + 1
WHERE capture_job_id = $1::uuid;
`, jobID)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_suggestions
SET candidate_content = candidate_content || '（漂移）'
WHERE id = $1::uuid;
`, firstReviewID)
	assertAutoCaptureReviewRollback(
		t, ctx, db, jobID, workerID, leaseID,
		firstReviewID, "AUTO_PROMOTION_CANDIDATE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_suggestions
SET candidate_content = '用户就读于西北工业大学'
WHERE id = $1::uuid;
`, firstReviewID)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_evidence
SET observed_at = observed_at + interval '1 second'
WHERE suggestion_id = $1::uuid AND evidence_role = 'user';
`, firstReviewID)
	assertAutoCaptureReviewRollback(
		t, ctx, db, jobID, workerID, leaseID,
		firstReviewID, "AUTO_PROMOTION_EVIDENCE_STALE",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_review_evidence
SET observed_at = $2
WHERE suggestion_id = $1::uuid AND evidence_role = 'user';
`, firstReviewID, observedAt)

	mustExecPhase151C(t, ctx, db, `
UPDATE conversations
SET memory_scope_generation = memory_scope_generation + 1
WHERE id = $1::uuid;
`, conversationID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_CAPTURE_SOURCE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE conversations
SET memory_scope_generation = memory_scope_generation - 1
WHERE id = $1::uuid;
`, conversationID)

	mustExecPhase151C(t, ctx, db, `
UPDATE projects SET lifecycle_status = 'archived' WHERE id = $1::uuid;
`, projectID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_CAPTURE_SOURCE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE projects SET lifecycle_status = 'active' WHERE id = $1::uuid;
`, projectID)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state
SET visibility_epoch = visibility_epoch + 1
WHERE user_id = $1::uuid;
`, userID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_VISIBILITY_EPOCH_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state
SET visibility_epoch = visibility_epoch - 1
WHERE user_id = $1::uuid;
`, userID)

	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs
SET provider_config_updated_at = provider_config_updated_at - interval '1 second'
WHERE job_id = $1::uuid;
`, jobID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_PROFILE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs job
SET provider_config_updated_at = provider.updated_at
FROM provider_configs provider
WHERE job.job_id = $1::uuid AND provider.id = job.provider_record_id;
`, jobID)

	// A Conversation-level Learn override can keep legacy hydration eligible,
	// but auto-promotion still requires the owner's automatic-recording switch.
	mustExecPhase151C(t, ctx, db, `
UPDATE conversations SET memory_learn_mode = 'on' WHERE id = $1::uuid;
UPDATE user_memory_settings SET auto_record_enabled = false WHERE user_id = $2::uuid;
`, conversationID, userID)
	disabledPromoted, disabledReviews, disabledRejected := callAutoCapturePromotion(
		t, ctx, db, jobID, workerID, leaseID,
	)
	if disabledPromoted != 0 || disabledReviews != 0 || disabledRejected != 0 {
		t.Fatalf("disabled promotion summary = %d/%d/%d",
			disabledPromoted, disabledReviews, disabledRejected)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE conversations SET memory_learn_mode = 'inherit' WHERE id = $1::uuid;
UPDATE user_memory_settings SET auto_record_enabled = true WHERE user_id = $2::uuid;
`, conversationID, userID)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_settings SET enabled = false WHERE user_id = $1::uuid;
`, userID)
	assertAutoCapturePromotionError(
		t, ctx, db, jobID, workerID, leaseID, "MEMORY_CAPTURE_SOURCE_DRIFT",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_settings SET enabled = true WHERE user_id = $1::uuid;
`, userID)

	// Every assistant/user evidence row is content-hash and completion bound,
	// not merely the primary source row stored on the job.
	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET content = 'stale assistant evidence' WHERE id = $1::uuid;
`, assistantID)
	assertAutoCaptureReviewRollback(
		t, ctx, db, jobID, workerID, leaseID,
		secondReviewID, "AUTO_PROMOTION_EVIDENCE_STALE",
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET content = '你是西北工业大学的。' WHERE id = $1::uuid;
`, assistantID)

	promoted, reviews, rejected := callAutoCapturePromotion(
		t, ctx, db, jobID, workerID, leaseID,
	)
	if promoted != 1 || reviews != 3 || rejected != 0 {
		t.Fatalf("promotion summary = %d/%d/%d", promoted, reviews, rejected)
	}
	// Crash replay under the same live lease reports the same committed state
	// and cannot create another canonical row.
	replayPromoted, replayReviews, replayRejected := callAutoCapturePromotion(
		t, ctx, db, jobID, workerID, leaseID,
	)
	if replayPromoted != 1 || replayReviews != 3 || replayRejected != 0 {
		t.Fatalf("replay summary = %d/%d/%d", replayPromoted, replayReviews, replayRejected)
	}

	var memoryCount, evidenceCount, decisionCount, activityCount int
	var authorityKind, storedExtractionProfile, suggestionStatus, decisionKind string
	if err := db.QueryRowContext(ctx, `
SELECT count(*), max(authority_kind), max(extraction_profile_id)
FROM user_memories
WHERE user_id = $1::uuid AND deleted_at IS NULL
  AND normalized_content = '用户就读于西北工业大学'
`, userID).Scan(&memoryCount, &authorityKind, &storedExtractionProfile); err != nil {
		t.Fatalf("query promoted Memory: %v", err)
	}
	if memoryCount != 1 || authorityKind != "auto" ||
		storedExtractionProfile != expectedExtractionProfile {
		t.Fatalf("promoted Memory = %d/%q/%q", memoryCount, authorityKind, storedExtractionProfile)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_evidence evidence
JOIN user_memories memory
  ON memory.id = evidence.memory_id AND memory.user_id = evidence.user_id
WHERE evidence.user_id = $1::uuid
  AND evidence.source_message_id = $2::uuid
  AND memory.normalized_content = '用户就读于西北工业大学'
`, userID, sourceID).Scan(&evidenceCount); err != nil || evidenceCount != 1 {
		t.Fatalf("promoted evidence = %d/%v", evidenceCount, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT status, decision_kind FROM user_memory_review_suggestions
WHERE id = $1::uuid AND user_id = $2::uuid
`, firstReviewID, userID).Scan(&suggestionStatus, &decisionKind); err != nil ||
		suggestionStatus != "accepted" || decisionKind != "auto_accept" {
		t.Fatalf("accepted suggestion = %q/%q/%v", suggestionStatus, decisionKind, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_review_decisions
WHERE suggestion_id = $1::uuid AND user_id = $2::uuid
  AND decision_kind = 'auto_accept' AND result_code = 'AUTO_CAPTURED'
`, firstReviewID, userID).Scan(&decisionCount); err != nil || decisionCount != 1 {
		t.Fatalf("auto decision audit = %d/%v", decisionCount, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM message_memory_activities
WHERE assistant_message_id = $1::uuid AND user_id = $2::uuid
  AND action = 'created' AND status = 'completed'
  AND reason_code = 'AUTO_CAPTURED'
`, assistantID, userID).Scan(&activityCount); err != nil || activityCount != 1 {
		t.Fatalf("auto Activity = %d/%v", activityCount, err)
	}

	var pendingConflict, pendingTombstone, pendingTemporary, sensitiveRejected int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE decision_reason_code = 'AUTO_PROMOTION_CONFLICT'),
  count(*) FILTER (WHERE decision_reason_code = 'AUTO_PROMOTION_TOMBSTONED'),
  count(*) FILTER (WHERE decision_reason_code = 'AUTO_PROMOTION_REVIEW_REQUIRED'),
  count(*) FILTER (
    WHERE status = 'rejected' AND result_code = 'SENSITIVE_DISABLED'
  )
FROM user_memory_review_suggestions
WHERE capture_job_id = $1::uuid
`, jobID).Scan(
		&pendingConflict, &pendingTombstone, &pendingTemporary, &sensitiveRejected,
	); err != nil || pendingConflict != 1 || pendingTombstone != 1 ||
		pendingTemporary != 1 || sensitiveRejected != 1 {
		t.Fatalf("safeguards = %d/%d/%d/%d/%v", pendingConflict,
			pendingTombstone, pendingTemporary, sensitiveRejected, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memories
WHERE user_id = $1::uuid AND deleted_at IS NULL
`, userID).Scan(&memoryCount); err != nil || memoryCount != 1 {
		t.Fatalf("canonical active count = %d/%v", memoryCount, err)
	}
	var projectionCount, embeddingJobCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections projection
JOIN user_memories memory
  ON memory.id = projection.memory_id AND memory.user_id = projection.user_id
WHERE memory.user_id = $1::uuid AND memory.deleted_at IS NULL
`, userID).Scan(&projectionCount); err != nil || projectionCount != 1 {
		t.Fatalf("projection count = %d/%v", projectionCount, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_embedding_jobs embedding
JOIN user_memories memory
  ON memory.id = embedding.memory_id AND memory.user_id = embedding.user_id
WHERE memory.user_id = $1::uuid AND memory.deleted_at IS NULL
`, userID).Scan(&embeddingJobCount); err != nil || embeddingJobCount != 1 {
		t.Fatalf("embedding job count = %d/%v", embeddingJobCount, err)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_AUTO_CAPTURE_COMPATIBLE_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS") {
		t.Fatalf("069 rollback unexpectedly accepted promoted history: %v", err)
	}
}

func callAutoCapturePromotion(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	jobID string,
	workerID string,
	leaseID string,
) (int, int, int) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatalf("set worker role: %v", err)
	}
	var promoted, reviews, rejected int
	if err := tx.QueryRowContext(ctx, `
SELECT promoted_count, review_count, rejected_count
FROM memory_worker_promote_capture_candidates($1, $2, $3)
`, jobID, workerID, leaseID).Scan(&promoted, &reviews, &rejected); err != nil {
		t.Fatalf("call auto-capture promotion: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit auto-capture promotion: %v", err)
	}
	return promoted, reviews, rejected
}

func assertAutoCapturePromotionError(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	jobID string,
	workerID string,
	leaseID string,
	want string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatalf("set worker role: %v", err)
	}
	var promoted, reviews, rejected int
	err = tx.QueryRowContext(ctx, `
SELECT promoted_count, review_count, rejected_count
FROM memory_worker_promote_capture_candidates($1, $2, $3)
`, jobID, workerID, leaseID).Scan(&promoted, &reviews, &rejected)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("promotion error = %v, want %s", err, want)
	}
}

func assertAutoCaptureReviewRollback(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	jobID string,
	workerID string,
	leaseID string,
	suggestionID string,
	wantReason string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatalf("set worker role: %v", err)
	}
	var promoted, reviews, rejected int
	if err := tx.QueryRowContext(ctx, `
SELECT promoted_count, review_count, rejected_count
FROM memory_worker_promote_capture_candidates($1, $2, $3)
`, jobID, workerID, leaseID).Scan(
		&promoted, &reviews, &rejected,
	); err != nil {
		t.Fatalf("inspect auto-capture review: %v", err)
	}
	if promoted != 1 || reviews != 3 || rejected != 0 {
		t.Fatalf("review inspection summary = %d/%d/%d",
			promoted, reviews, rejected)
	}
	if _, err := tx.ExecContext(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("reset worker role: %v", err)
	}
	var reason string
	if err := tx.QueryRowContext(ctx, `
SELECT decision_reason_code
FROM user_memory_review_suggestions
WHERE id = $1::uuid
`, suggestionID).Scan(&reason); err != nil || reason != wantReason {
		t.Fatalf("review reason = %q/%v, want %q", reason, err, wantReason)
	}
}

func assertAutoCaptureWorkerTableDenied(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatalf("set worker role: %v", err)
	}
	for _, table := range []string{
		"user_memories",
		"user_memory_review_suggestions",
		"user_memory_review_decisions",
		"message_memory_activities",
	} {
		if _, err := tx.ExecContext(ctx, `SELECT count(*) FROM `+table); err == nil {
			t.Fatalf("memory_worker_runtime unexpectedly read %s directly", table)
		}
	}
}
