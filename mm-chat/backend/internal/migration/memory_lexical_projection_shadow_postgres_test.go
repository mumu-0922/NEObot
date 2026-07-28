package migration

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMemoryLexicalProjectionShadowLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 57))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 057: %v", err)
	}

	const (
		userID               = "14000000-0000-4000-8000-000000000001"
		otherUserID          = "14000000-0000-4000-8000-000000000002"
		conversation         = "24000000-0000-4000-8000-000000000001"
		otherConv            = "24000000-0000-4000-8000-000000000002"
		source               = "34000000-0000-4000-8000-000000000001"
		assistant            = "34000000-0000-4000-8000-000000000002"
		latinSource          = "34000000-0000-4000-8000-000000000003"
		latinAssistant       = "34000000-0000-4000-8000-000000000004"
		sensitiveSource      = "34000000-0000-4000-8000-000000000005"
		sensitiveAssistant   = "34000000-0000-4000-8000-000000000006"
		expiredSource        = "34000000-0000-4000-8000-000000000007"
		expiredAssistant     = "34000000-0000-4000-8000-000000000008"
		staleSource          = "34000000-0000-4000-8000-000000000009"
		staleAssistant       = "34000000-0000-4000-8000-000000000010"
		memoryID             = "54000000-0000-4000-8000-000000000001"
		otherMemory          = "54000000-0000-4000-8000-000000000002"
		projectID            = "44000000-0000-4000-8000-000000000001"
		projectMemory        = "54000000-0000-4000-8000-000000000003"
		conversationMemory   = "54000000-0000-4000-8000-000000000004"
		latinMemory          = "54000000-0000-4000-8000-000000000005"
		sensitiveMemory      = "54000000-0000-4000-8000-000000000006"
		expiredMemory        = "54000000-0000-4000-8000-000000000007"
		observation          = "64000000-0000-4000-8000-000000000001"
		latinObservation     = "64000000-0000-4000-8000-000000000002"
		sensitiveObservation = "64000000-0000-4000-8000-000000000003"
		expiredObservation   = "64000000-0000-4000-8000-000000000004"
		staleObservation     = "64000000-0000-4000-8000-000000000005"
	)
	query := "请继续用简洁风格回答"
	queryHash := sha256Hex(query)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES
  ($1, 'PR7'), ($2, 'PR7 Other');
INSERT INTO conversations(id, user_id, title) VALUES
  ($3, $1, 'PR7'), ($4, $2, 'PR7 Other');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($5, $3, $1, 1, 'user', 'completed', $7, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($6, $3, $1, $5, 2, 'assistant', 'streaming', '');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES
  ($1, true, true, false, false),
  ($2, true, true, false, false);
SELECT id FROM memory_upsert_global_manual(
  $8, $1, 'preference', '用户偏好简洁回答', '用户偏好简洁回答',
  5::smallint, ARRAY['风格']::text[], NULL, NULL, true
);
SELECT id FROM memory_upsert_global_manual(
  $9, $2, 'preference', '另一个用户偏好简洁回答', '另一个用户偏好简洁回答',
  5::smallint, ARRAY['风格']::text[], NULL, NULL, true
);
INSERT INTO projects(id, user_id, name) VALUES ($10, $1, 'PR7 Project');
UPDATE conversations SET project_id = $10 WHERE id = $3;
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, project_id, content_hash, authority_kind
) VALUES (
  $11, $1, 'project', 'Project marker AURORA', 'project marker aurora',
  'manual', 'project', $10,
  encode(sha256(convert_to('Project marker AURORA', 'UTF8')), 'hex'), 'manual'
);
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, scope_conversation_id, content_hash, authority_kind
) VALUES (
  $12, $1, 'context', 'Conversation marker DELTA', 'conversation marker delta',
  'manual', 'conversation', $3,
  encode(sha256(convert_to('Conversation marker DELTA', 'UTF8')), 'hex'), 'manual'
);
SELECT id FROM memory_upsert_global_manual(
  $13, $1, 'preference', 'Use concise code answers', 'use concise code answers',
  5::smallint, ARRAY['code-style']::text[], NULL, NULL, true
);
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, sensitivity, content_hash, authority_kind
) VALUES (
  $14, $1, 'fact', 'Sensitive marker OMEGA', 'sensitive marker omega',
  'manual', 'global', 'sensitive',
  encode(sha256(convert_to('Sensitive marker OMEGA', 'UTF8')), 'hex'), 'manual'
);
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, expires_at, content_hash, authority_kind
) VALUES (
  $15, $1, 'context', 'Expired marker TAU', 'expired marker tau',
  'manual', 'global', now() - interval '1 minute',
  encode(sha256(convert_to('Expired marker TAU', 'UTF8')), 'hex'), 'manual'
);
`, userID, otherUserID, conversation, otherConv, source, assistant, query,
		memoryID, otherMemory, projectID, projectMemory, conversationMemory,
		latinMemory, sensitiveMemory, expiredMemory)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 58))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 58 {
		t.Fatalf("apply 058 over canonical fixture = %#v/%v", applied, err)
	}

	var projectionCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections
WHERE memory_id IN ($1, $2, $3, $4, $5, $6, $7) AND lexical_status = 'ready'
`, memoryID, otherMemory, projectMemory, conversationMemory, latinMemory,
		sensitiveMemory, expiredMemory).Scan(&projectionCount); err != nil || projectionCount != 7 {
		t.Fatalf("projection backfill/trigger count = %d/%v", projectionCount, err)
	}

	var gotObservation, profile, status, resultCode string
	var generation int64
	var baselineCount, exactCount, bm25Count, lexicalCount, overlapCount, duration int
	if err := db.QueryRowContext(ctx, `
SELECT observation_id::text, profile_id, projection_generation,
       status, result_code, baseline_count, exact_count, bm25_count,
       lexical_count, overlap_count, duration_millis
FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, '[]'::jsonb, 20
)
`, observation, userID, conversation, assistant, queryHash, query).Scan(
		&gotObservation, &profile, &generation, &status, &resultCode,
		&baselineCount, &exactCount, &bm25Count, &lexicalCount,
		&overlapCount, &duration,
	); err != nil {
		t.Fatalf("compare lexical shadow: %v", err)
	}
	if gotObservation != observation || profile != "memory_lexical_cjk_bm25_v1" ||
		generation != 1 || status != "completed" || resultCode != "OK" ||
		baselineCount != 0 || exactCount != 0 || bm25Count < 1 ||
		lexicalCount < 1 || overlapCount != 0 || duration < 0 {
		t.Fatalf("shadow summary = %q/%q/%d/%q/%q/%d/%d/%d/%d/%d/%d",
			gotObservation, profile, generation, status, resultCode, baselineCount,
			exactCount, bm25Count, lexicalCount, overlapCount, duration)
	}

	var ownHits, otherHits int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE memory_id = $2),
  count(*) FILTER (WHERE memory_id = $3)
FROM message_memory_lexical_shadow_results
WHERE observation_id = $1 AND lane = 'lexical'
`, observation, memoryID, otherMemory).Scan(&ownHits, &otherHits); err != nil {
		t.Fatal(err)
	}
	if ownHits != 1 || otherHits != 0 {
		t.Fatalf("authorized lexical hits = own:%d other:%d", ownHits, otherHits)
	}

	// Latin terms and punctuation exercise the exact lane independently of the
	// CJK bigram BM25 proof above.
	latinQuery := "Please, use concise code answers!"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, 3, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $3, $1, 4, 'assistant', 'streaming', '');
`, latinSource, conversation, userID, latinAssistant, latinQuery)
	if err := db.QueryRowContext(ctx, `
SELECT status, exact_count, bm25_count, lexical_count
FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, '[]'::jsonb, 20
)
`, latinObservation, userID, conversation, latinAssistant,
		sha256Hex(latinQuery), latinQuery).Scan(
		&status, &exactCount, &bm25Count, &lexicalCount,
	); err != nil || status != "completed" || exactCount < 1 ||
		bm25Count < 1 || lexicalCount < 1 {
		t.Fatalf("Latin/punctuation lanes = %q/%d/%d/%d/%v",
			status, exactCount, bm25Count, lexicalCount, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM message_memory_lexical_shadow_results
WHERE observation_id = $1 AND lane = 'exact' AND memory_id = $2
`, latinObservation, latinMemory).Scan(&ownHits); err != nil || ownHits != 1 {
		t.Fatalf("Latin exact hit = %d/%v", ownHits, err)
	}

	// Sensitive and expired canonical rows may retain rebuildable projection
	// text, but authorization must exclude them before either rank lane.
	for _, probe := range []struct {
		sourceID, assistantID, observationID, query, forbiddenMemory string
		sequence                                                     int
	}{
		{sensitiveSource, sensitiveAssistant, sensitiveObservation, "OMEGA", sensitiveMemory, 5},
		{expiredSource, expiredAssistant, expiredObservation, "TAU", expiredMemory, 7},
	} {
		mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, $4, 'user', 'completed', $6, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($5, $2, $3, $1, $4 + 1, 'assistant', 'streaming', '');
`, probe.sourceID, conversation, userID, probe.sequence, probe.assistantID, probe.query)
		if err := db.QueryRowContext(ctx, `
SELECT status FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, '[]'::jsonb, 20
)
`, probe.observationID, userID, conversation, probe.assistantID,
			sha256Hex(probe.query), probe.query).Scan(&status); err != nil || status != "completed" {
			t.Fatalf("authority probe %q = %q/%v", probe.query, status, err)
		}
		if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM message_memory_lexical_shadow_results
WHERE observation_id = $1 AND lane IN ('exact', 'bm25', 'lexical')
  AND memory_id = $2
`, probe.observationID, probe.forbiddenMemory).Scan(&ownHits); err != nil || ownHits != 0 {
			t.Fatalf("forbidden authority hit %q = %d/%v", probe.query, ownHits, err)
		}
	}

	// Project and Conversation lifecycle/generation changes physically remove
	// scoped derived plaintext and a valid reactivation rebuilds it.
	for _, lifecycle := range []struct {
		removeSQL, restoreSQL string
		memoryID              string
	}{
		{
			removeSQL:  `UPDATE projects SET lifecycle_status = 'archived' WHERE id = $1`,
			restoreSQL: `UPDATE projects SET lifecycle_status = 'active' WHERE id = $1`,
			memoryID:   projectMemory,
		},
		{
			removeSQL:  `UPDATE conversations SET deleted_at = now() WHERE id = $1`,
			restoreSQL: `UPDATE conversations SET deleted_at = NULL WHERE id = $1`,
			memoryID:   conversationMemory,
		},
	} {
		ownerID := projectID
		if lifecycle.memoryID == conversationMemory {
			ownerID = conversation
		}
		mustExecPhase151C(t, ctx, db, lifecycle.removeSQL, ownerID)
		if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections WHERE memory_id = $1
`, lifecycle.memoryID).Scan(&projectionCount); err != nil || projectionCount != 0 {
			t.Fatalf("removed scoped projection %s = %d/%v",
				lifecycle.memoryID, projectionCount, err)
		}
		mustExecPhase151C(t, ctx, db, lifecycle.restoreSQL, ownerID)
		if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections WHERE memory_id = $1
`, lifecycle.memoryID).Scan(&projectionCount); err != nil || projectionCount != 1 {
			t.Fatalf("rebuilt scoped projection %s = %d/%v",
				lifecycle.memoryID, projectionCount, err)
		}
	}

	// A projection whose canonical hash fence drifts is excluded before rank;
	// a later canonical mutation rebuilds the authoritative projection.
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_search_projections
SET content_hash = repeat('a', 64)
WHERE memory_id = $1;
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($2, $3, $4, 9, 'user', 'completed', 'AURORA', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($5, $3, $4, $2, 10, 'assistant', 'streaming', '');
`, projectMemory, staleSource, conversation, userID, staleAssistant)
	if err := db.QueryRowContext(ctx, `
SELECT status FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'AURORA', '[]'::jsonb, 20
)
`, staleObservation, userID, conversation, staleAssistant,
		sha256Hex("AURORA")).Scan(&status); err != nil || status != "completed" {
		t.Fatalf("stale projection compare = %q/%v", status, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM message_memory_lexical_shadow_results
WHERE observation_id = $1 AND memory_id = $2
  AND lane IN ('exact', 'bm25', 'lexical')
`, staleObservation, projectMemory).Scan(&ownHits); err != nil || ownHits != 0 {
		t.Fatalf("stale projection ranked = %d/%v", ownHits, err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET tags = ARRAY['project', 'refreshed'] WHERE id = $1
`, projectMemory)
	var projectionHash, canonicalHash string
	if err := db.QueryRowContext(ctx, `
SELECT projection.content_hash, memory.content_hash
FROM user_memory_search_projections projection
JOIN user_memories memory ON memory.id = projection.memory_id
WHERE projection.memory_id = $1
`, projectMemory).Scan(&projectionHash, &canonicalHash); err != nil ||
		projectionHash != canonicalHash {
		t.Fatalf("repaired projection hash = %q/%q/%v",
			projectionHash, canonicalHash, err)
	}

	// Same identity/query/baseline is immutable and idempotent.
	var replayStatus string
	err = db.QueryRowContext(ctx, `
SELECT status FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, '[]'::jsonb, 20
)
	`, observation, userID, conversation, assistant, queryHash, query).Scan(&replayStatus)
	if err != nil || replayStatus != "completed" {
		t.Fatalf("idempotent replay = %q/%v", replayStatus, err)
	}
	changedQuery := query + "。"
	err = db.QueryRowContext(ctx, `
SELECT status FROM memory_compare_lexical_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, '[]'::jsonb, 20
)
	`, observation, userID, conversation, assistant,
		sha256Hex(changedQuery), changedQuery).Scan(&replayStatus)
	if err == nil ||
		!strings.Contains(err.Error(), "MEMORY_LEXICAL_SHADOW_REPLAY_CONFLICT") {
		t.Fatalf("changed replay error = %v", err)
	}

	// A canonical edit refreshes the derived plaintext; logical deletion removes it.
	mustExecPhase151C(t, ctx, db, `
SELECT id FROM memory_update_global_manual(
  $1, $2, 'preference', '用户偏好详细回答', '用户偏好详细回答',
  5::smallint, ARRAY['风格']::text[], true
)
`, memoryID, userID)
	var revision int64
	if err := db.QueryRowContext(ctx, `
SELECT memory_revision FROM user_memory_search_projections WHERE memory_id = $1
`, memoryID).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("updated projection revision = %d/%v", revision, err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET deleted_at = now(), enabled = false WHERE id = $1
`, memoryID)
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections WHERE memory_id = $1
`, memoryID).Scan(&projectionCount); err != nil || projectionCount != 0 {
		t.Fatalf("deleted projection count = %d/%v", projectionCount, err)
	}

	epochTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := epochTx.ExecContext(ctx, `
UPDATE user_memory_state
SET visibility_epoch = visibility_epoch + 1 WHERE user_id = $1
`, userID); err != nil {
		_ = epochTx.Rollback()
		t.Fatal(err)
	}
	if err := epochTx.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections WHERE user_id = $1
`, userID).Scan(&projectionCount); err != nil || projectionCount != 0 {
		_ = epochTx.Rollback()
		t.Fatalf("epoch-drift projections = %d/%v", projectionCount, err)
	}
	if err := epochTx.Rollback(); err != nil {
		t.Fatal(err)
	}

	generationTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generationTx.ExecContext(ctx, `
UPDATE user_memory_state
SET active_projection_generation = active_projection_generation + 1
WHERE user_id = $1
`, userID); err != nil {
		_ = generationTx.Rollback()
		t.Fatal(err)
	}
	var driftedGenerationCount int
	if err := generationTx.QueryRowContext(ctx, `
SELECT count(*), count(*) FILTER (WHERE projection_generation = 2)
FROM user_memory_search_projections WHERE user_id = $1
`, userID).Scan(&projectionCount, &driftedGenerationCount); err != nil ||
		projectionCount == 0 || driftedGenerationCount != projectionCount {
		_ = generationTx.Rollback()
		t.Fatalf("generation refresh = %d/%d/%v",
			projectionCount, driftedGenerationCount, err)
	}
	if err := generationTx.Rollback(); err != nil {
		t.Fatal(err)
	}

	for _, check := range []struct {
		role, table, privilege string
	}{
		{"go_api_runtime", "user_memory_search_projections", "SELECT"},
		{"go_api_runtime", "message_memory_lexical_shadow_observations", "INSERT"},
		{"memory_worker_runtime", "message_memory_lexical_shadow_results", "SELECT"},
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
	var compareAllowed, workerCompareAllowed bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege(
    'go_api_runtime',
    'memory_compare_lexical_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,integer)',
    'EXECUTE'
  ),
  has_function_privilege(
    'memory_worker_runtime',
    'memory_compare_lexical_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,integer)',
    'EXECUTE'
  )
`).Scan(&compareAllowed, &workerCompareAllowed); err != nil {
		t.Fatal(err)
	}
	if !compareAllowed || workerCompareAllowed {
		t.Fatalf("compare privileges = api:%t worker:%t", compareAllowed, workerCompareAllowed)
	}
	if err := execPhase15AsRole(
		ctx,
		db,
		"go_api_runtime",
		`SELECT * FROM user_memory_search_projections LIMIT 1`,
	); !phase15InsufficientPrivilege(err) {
		t.Fatalf("go_api_runtime projection read error = %v", err)
	}

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state
SET active_retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
WHERE user_id = $1
`, userID)
	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_LEXICAL_ROLLBACK_REQUIRES_V1_READER") {
		t.Fatalf("reader guarded down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state SET active_retrieval_profile_id = NULL WHERE user_id = $1
`, userID)
	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_LEXICAL_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS") {
		t.Fatalf("guarded down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `DELETE FROM users WHERE id IN ($1, $2)`, userID, otherUserID)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 58 {
		t.Fatalf("clean down = %#v/%v", down, err)
	}
	up, err := runner.Up(ctx)
	if err != nil || len(up) != 1 || up[0].Version != 58 {
		t.Fatalf("re-up = %#v/%v", up, err)
	}
}
