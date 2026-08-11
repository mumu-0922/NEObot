package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMemoryReviewStaleRejectLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 59))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 059: %v", err)
	}

	const (
		userID         = "17200000-0000-4000-8000-000000000001"
		conversationID = "27200000-0000-4000-8000-000000000001"
		sourceID       = "37200000-0000-4000-8000-000000000001"
		assistantID    = "37200000-0000-4000-8000-000000000002"
		providerID     = "47200000-0000-4000-8000-000000000001"
		eventID        = "57200000-0000-4000-8000-000000000001"
		jobID          = "67200000-0000-4000-8000-000000000001"
		expiryJobID    = "67200000-0000-4000-8000-000000000002"
		workerID       = "77200000-0000-4000-8000-000000000001"
		leaseID        = "87200000-0000-4000-8000-000000000001"
	)
	memoryIDs := []string{
		"97200000-0000-4000-8000-000000000001",
		"97200000-0000-4000-8000-000000000002",
		"97200000-0000-4000-8000-000000000003",
	}
	reviewIDs := []string{
		"a7200000-0000-4000-8000-000000000001",
		"a7200000-0000-4000-8000-000000000002",
		"a7200000-0000-4000-8000-000000000003",
	}
	observedAt := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'Stale Reject');
INSERT INTO conversations(id, user_id, title)
VALUES ($2, $1, 'Stale Review');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at, created_at, updated_at
) VALUES (
  $3, $2, $1, 1, 'user', 'completed', 'Prefer concise replies',
  $6, $6, $6
);
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, completed_at, created_at, updated_at
) VALUES (
  $4, $2, $1, $3, 2, 'assistant', 'completed', 'Understood',
  $6, $6, $6
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
`, userID, conversationID, sourceID, assistantID, providerID, observedAt)

	proposals := make([]map[string]any, 0, len(reviewIDs))
	for index, memoryID := range memoryIDs {
		content := "Current stale target " + string(rune('A'+index))
		if _, err := db.ExecContext(ctx, `
SELECT id FROM memory_upsert_global_manual(
  $1::uuid, $2::uuid, 'preference', $3, lower($3),
  4::smallint, ARRAY['style']::text[], NULL, NULL, true
)
`, memoryID, userID, content); err != nil {
			t.Fatalf("seed target %d: %v", index, err)
		}
		proposed := "Proposed stale target " + string(rune('A'+index))
		proposals = append(proposals, memoryReviewProposal(
			reviewIDs[index], sourceID, observedAt, proposed,
			strings.ToLower(proposed), "global", nil, nil, "normal",
			"MERGE", []string{memoryID},
		))
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, eventID, jobID, userID, conversationID, sourceID, assistantID); err != nil {
		t.Fatalf("append stale Review capture: %v", err)
	}
	claimMemoryJob(t, ctx, db, workerID, leaseID, jobID, 1)
	proposalJSON, err := json.Marshal(proposals)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_propose_capture_candidates(
  $1, $2, $3, $4, 1::smallint, repeat('a',64), repeat('b',64), $5::jsonb
)
`, jobID, workerID, leaseID, expiryJobID, string(proposalJSON)); err != nil {
		t.Fatalf("create stale Review fixtures: %v", err)
	}

	head071 := NewRunner(db, phase15MigrationFSThrough(t, 71))
	if _, err := head071.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := head071.Up(ctx); err != nil {
		t.Fatalf("apply through 071: %v", err)
	}

	for index, memoryID := range memoryIDs {
		updated := "Changed stale target " + string(rune('A'+index))
		var memoryJSON []byte
		if err := db.QueryRowContext(ctx, `
SELECT memory_governance_update_memory(
  $1, $2, 1, 'preference', $3, lower($3),
  4::smallint, ARRAY['style']::text[], 'global', NULL, NULL, 'normal'
)
`, userID, memoryID, updated).Scan(&memoryJSON); err != nil {
			t.Fatalf("drift target %d: %v", index, err)
		}
	}

	callStaleReviewDecisionExpectError(
		t, ctx, db, userID, reviewIDs[0],
		"b7200000-0000-4000-8000-000000000001", "reject", nil,
		strings.Repeat("1", 64),
	)

	head072 := NewRunner(db, phase15MigrationFSThrough(t, 72))
	if _, err := head072.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := head072.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 72 {
		t.Fatalf("apply 072 = %#v/%v", applied, err)
	}

	var beforeHash, afterHash string
	var beforeRevision, afterRevision int64
	if err := db.QueryRowContext(ctx, `
SELECT content_hash, revision FROM user_memories
WHERE id = $1::uuid AND user_id = $2::uuid
`, memoryIDs[0], userID).Scan(&beforeHash, &beforeRevision); err != nil {
		t.Fatal(err)
	}
	result := callStaleReviewDecision(
		t, ctx, db, userID, reviewIDs[0],
		"b7200000-0000-4000-8000-000000000002", "reject", nil,
		strings.Repeat("2", 64),
	)
	if !strings.Contains(result, `"status": "rejected"`) ||
		!strings.Contains(result, `"resultCode": "USER_REJECTED"`) {
		t.Fatalf("stale reject result = %s", result)
	}
	if err := db.QueryRowContext(ctx, `
SELECT content_hash, revision FROM user_memories
WHERE id = $1::uuid AND user_id = $2::uuid
`, memoryIDs[0], userID).Scan(&afterHash, &afterRevision); err != nil {
		t.Fatal(err)
	}
	if afterHash != beforeHash || afterRevision != beforeRevision {
		t.Fatalf("stale reject changed target = %s/r%d -> %s/r%d",
			beforeHash, beforeRevision, afterHash, afterRevision)
	}
	assertStaleReviewRejected(t, ctx, db, userID, reviewIDs[0])

	callStaleReviewDecisionExpectError(
		t, ctx, db, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000003", "keep_current", nil,
		strings.Repeat("3", 64),
	)
	acceptMemoryID := "c7200000-0000-4000-8000-000000000001"
	callStaleReviewDecisionExpectError(
		t, ctx, db, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000004", "accept_new", &acceptMemoryID,
		strings.Repeat("4", 64),
	)

	rolledBack, err := head072.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 72 {
		t.Fatalf("down 072 = %#v/%v", rolledBack, err)
	}
	callStaleReviewDecisionExpectError(
		t, ctx, db, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000005", "reject", nil,
		strings.Repeat("5", 64),
	)
	reapplied, err := head072.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 72 {
		t.Fatalf("re-up 072 = %#v/%v", reapplied, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	var runtimeResult []byte
	if err := tx.QueryRowContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, $3, 'reject', NULL, NULL, NULL, $4
)
`, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000006",
		strings.Repeat("6", 64)).Scan(&runtimeResult); err != nil {
		_ = tx.Rollback()
		t.Fatalf("runtime stale reject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertStaleReviewRejected(t, ctx, db, userID, reviewIDs[1])

	replay := callStaleReviewDecision(
		t, ctx, db, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000007", "reject", nil,
		strings.Repeat("6", 64),
	)
	if string(runtimeResult) != replay {
		t.Fatalf("stale reject replay drift = %s/%s", runtimeResult, replay)
	}
	_, replayErr := db.ExecContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, $3, 'reject', NULL, NULL, NULL, $4
)
`, userID, reviewIDs[1],
		"b7200000-0000-4000-8000-000000000008",
		strings.Repeat("8", 64))
	if replayErr == nil || !strings.Contains(
		replayErr.Error(), "MEMORY_GOVERNANCE_REPLAY_CONFLICT",
	) {
		t.Fatalf("stale reject replay conflict = %v", replayErr)
	}
}

func callStaleReviewDecision(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
	reviewID string,
	decisionID string,
	decision string,
	memoryID *string,
	hash string,
) string {
	t.Helper()
	var result []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, $3, $4, $5, NULL, NULL, $6
)
`, userID, reviewID, decisionID, decision, memoryID, hash).Scan(&result); err != nil {
		t.Fatalf("Review decision %s: %v", decision, err)
	}
	return string(result)
}

func callStaleReviewDecisionExpectError(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	reviewID string,
	decisionID string,
	decision string,
	memoryID *string,
	hash string,
) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
SELECT memory_governance_decide_review(
  $1, $2, $3, $4, $5, NULL, NULL, $6
)
`, userID, reviewID, decisionID, decision, memoryID, hash)
	if err == nil || !strings.Contains(err.Error(), "MEMORY_GOVERNANCE_REVIEW_STALE") {
		t.Fatalf("stale Review decision %s = %v", decision, err)
	}
}

func assertStaleReviewRejected(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	reviewID string,
) {
	t.Helper()
	var status, disposition, decisionKind, resultCode string
	var plaintextCount, decisionCount int
	if err := db.QueryRowContext(ctx, `
SELECT status, disposition, decision_kind, result_code,
  (candidate_content IS NOT NULL)::int
    + (normalized_content IS NOT NULL)::int
    + cardinality(tags),
  (SELECT count(*) FROM user_memory_review_decisions decision
   WHERE decision.suggestion_id = suggestion.id
     AND decision.user_id = suggestion.user_id)
FROM user_memory_review_suggestions suggestion
WHERE suggestion.id = $1::uuid AND suggestion.user_id = $2::uuid
`, reviewID, userID).Scan(
		&status, &disposition, &decisionKind, &resultCode,
		&plaintextCount, &decisionCount,
	); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" || disposition != "rejected" ||
		decisionKind != "reject" || resultCode != "USER_REJECTED" ||
		plaintextCount != 0 || decisionCount != 1 {
		t.Fatalf("stale Review rejection = %s/%s/%s/%s plaintext=%d decisions=%d",
			status, disposition, decisionKind, resultCode,
			plaintextCount, decisionCount)
	}
}
