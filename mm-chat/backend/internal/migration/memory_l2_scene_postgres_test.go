package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMemoryL2SceneLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 61))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 061: %v", err)
	}

	const (
		userID             = "1b000000-0000-4000-8000-000000000001"
		otherUserID        = "1b000000-0000-4000-8000-000000000002"
		projectID          = "2b000000-0000-4000-8000-000000000001"
		conversationID     = "3b000000-0000-4000-8000-000000000001"
		sourceID           = "4b000000-0000-4000-8000-000000000001"
		assistantID        = "4b000000-0000-4000-8000-000000000002"
		globalMemoryA      = "5b000000-0000-4000-8000-000000000001"
		globalMemoryB      = "5b000000-0000-4000-8000-000000000002"
		projectMemoryA     = "5b000000-0000-4000-8000-000000000003"
		projectMemoryB     = "5b000000-0000-4000-8000-000000000004"
		conversationMemory = "5b000000-0000-4000-8000-000000000005"
		synthesisProvider  = "6b000000-0000-4000-8000-000000000001"
		ragProvider        = "6b000000-0000-4000-8000-000000000002"
		workerID           = "7b000000-0000-4000-8000-000000000001"
		globalSceneID      = "8b000000-0000-4000-8000-000000000001"
		projectSceneID     = "8b000000-0000-4000-8000-000000000002"
		observationID      = "9b000000-0000-4000-8000-000000000001"
	)
	query := "concise answer style"
	ragSecretRef := "fixture-rag-encrypted-secret-reference"
	ragAttestation := sha256Hex(strings.Join([]string{
		"rag-provider-connection/v1",
		"RAG:SILICONFLOW",
		"siliconflow",
		"https://api.siliconflow.cn/v1/embeddings",
		"Pro/BAAI/bge-m3",
		"1024",
		"https://api.siliconflow.cn/v1/rerank",
		"Pro/BAAI/bge-reranker-v2-m3",
		ragSecretRef,
	}, "\x00"))
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR11'), ($2, 'PR11 Other');
INSERT INTO projects(id, user_id, name) VALUES ($3, $1, 'PR11 Project');
INSERT INTO conversations(id, user_id, project_id, title)
VALUES ($4, $1, $3, 'PR11');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES ($5, $4, $1, 1, 'user', 'completed', $7, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role, status, content
) VALUES ($6, $4, $1, $5, 2, 'assistant', 'streaming', '');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled, sensitive_memory_enabled
) VALUES
  ($1, true, true, false, false),
  ($2, true, true, false, false);
INSERT INTO user_memory_state(user_id) VALUES ($1), ($2) ON CONFLICT DO NOTHING;
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source, scope_type,
  project_id, scope_conversation_id, scope_generation, content_hash,
  authority_kind, importance
) VALUES
  ($8, $1, 'preference', 'Concise answer style', 'concise answer style',
   'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('Concise answer style', 'UTF8')), 'hex'), 'manual', 5),
  ($9, $1, 'fact', 'Uses Go for backend services', 'uses go for backend services',
   'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('Uses Go for backend services', 'UTF8')), 'hex'), 'manual', 4),
  ($10, $1, 'project', 'Project codename is Aster', 'project codename is aster',
   'manual', 'project', $3, NULL, 1,
   encode(sha256(convert_to('Project codename is Aster', 'UTF8')), 'hex'), 'manual', 5),
  ($11, $1, 'project', 'Project uses PostgreSQL 17', 'project uses postgresql 17',
   'manual', 'project', $3, NULL, 1,
   encode(sha256(convert_to('Project uses PostgreSQL 17', 'UTF8')), 'hex'), 'manual', 4),
  ($12, $1, 'context', 'Conversation-only marker', 'conversation-only marker',
   'manual', 'conversation', NULL, $4, 1,
   encode(sha256(convert_to('Conversation-only marker', 'UTF8')), 'hex'), 'manual', 3);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config, created_at, updated_at
) VALUES
  ($13, $1, 'CUSTOM', 'Fixture synthesis', 'fixture-synthesis-secret',
   '{"enabled":true}'::jsonb, '2026-07-28T00:00:00Z', '2026-07-28T00:00:00Z'),
  ($14, $1, 'RAG:SILICONFLOW', 'Fixture RAG', $15,
   jsonb_build_object(
     'kind', 'rag', 'ragProvider', 'siliconflow', 'enabled', true,
     'connectionTestedAt', '2026-07-28T00:00:00Z',
     'connectionTestSha256', $16
   ), '2026-07-28T00:00:00Z', '2026-07-28T00:00:00Z');
INSERT INTO task_model_settings(user_id, memory) VALUES ($1, 'CUSTOM:scene-model');
`, userID, otherUserID, projectID, conversationID, sourceID, assistantID,
		query, globalMemoryA, globalMemoryB, projectMemoryA, projectMemoryB,
		conversationMemory, synthesisProvider, ragProvider, ragSecretRef, ragAttestation)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 62))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 62 {
		t.Fatalf("apply 062 over L1 fixture = %#v/%v", applied, err)
	}

	var refreshJobs, conversationJobs int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE stage = 'refresh'),
       count(*) FILTER (WHERE scope_type = 'conversation')
FROM user_memory_scene_jobs WHERE user_id = $1
`, userID).Scan(&refreshJobs, &conversationJobs); err != nil ||
		refreshJobs != 2 || conversationJobs != 0 {
		t.Fatalf("062 backfill jobs = refresh:%d conversation:%d err:%v",
			refreshJobs, conversationJobs, err)
	}
	if _, found, err := claimMemoryL2SceneJob(
		ctx, db, workerID, "ab000000-0000-4000-8000-000000000001", false,
	); err != nil || found {
		t.Fatalf("default-off refresh claim = found:%t err:%v", found, err)
	}

	leases := make(map[string]memoryL2SceneLease)
	for len(leases) < 2 {
		lease, found, claimErr := claimMemoryL2SceneJob(
			ctx,
			db,
			workerID,
			fmt.Sprintf("ab000000-0000-4000-8000-%012d", len(leases)+2),
			true,
		)
		if claimErr != nil || !found || lease.Stage != "refresh" {
			t.Fatalf("claim refresh %d = %#v/%t/%v", len(leases), lease, found, claimErr)
		}
		leases[lease.ScopeType] = lease
	}
	for scopeType, lease := range leases {
		memories, err := hydrateMemoryL2Scene(t, ctx, db, lease, workerID)
		if err != nil || len(memories) != 2 {
			t.Fatalf("hydrate %s = %#v/%v", scopeType, memories, err)
		}
		for _, memory := range memories {
			if memory.ID == conversationMemory {
				t.Fatalf("Conversation Memory widened into %s Scene input", scopeType)
			}
		}
		sceneID := globalSceneID
		topicKey := "global.preferences"
		content := "Prefers concise answers and uses Go services."
		if scopeType == "project" {
			sceneID = projectSceneID
			topicKey = "project.aster"
			content = "Aster uses PostgreSQL 17."
		}
		payload := []map[string]any{{
			"sceneId":         sceneID,
			"topicKey":        topicKey,
			"content":         content,
			"contentHash":     sha256Hex(content),
			"sensitivity":     "normal",
			"memberMemoryIds": []string{memories[0].ID, memories[1].ID},
		}}
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_refresh($1, $2, $3, $4::jsonb)
`, lease.JobID, workerID, lease.LeaseToken, string(encoded)).Scan(new([]byte)); err != nil {
			t.Fatalf("complete %s Scene: %v", scopeType, err)
		}
	}

	var sceneCount, projectionCount, embeddingJobCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*),
  (SELECT count(*) FROM user_memory_derived_search_projections WHERE user_id = $1),
  (SELECT count(*) FROM user_memory_derived_embedding_jobs WHERE user_id = $1)
FROM user_memory_scenes WHERE user_id = $1
`, userID).Scan(&sceneCount, &projectionCount, &embeddingJobCount); err != nil ||
		sceneCount != 2 || projectionCount != 2 || embeddingJobCount != 2 {
		t.Fatalf("Scene apply = scenes:%d projections:%d embedding-jobs:%d err:%v",
			sceneCount, projectionCount, embeddingJobCount, err)
	}

	for index := 0; index < 2; index++ {
		embedding, found, claimErr := claimMemoryL2Embedding(
			ctx,
			db,
			workerID,
			fmt.Sprintf("bb000000-0000-4000-8000-%012d", index+1),
		)
		if claimErr != nil || !found {
			t.Fatalf("claim derived embedding %d = %#v/%t/%v",
				index, embedding, found, claimErr)
		}
		var content string
		if err := db.QueryRowContext(ctx, `
SELECT content FROM memory_worker_hydrate_l2_scene_embedding_job($1, $2, $3)
`, embedding.JobID, workerID, embedding.LeaseToken).Scan(&content); err != nil || content == "" {
			t.Fatalf("hydrate derived embedding %d = %q/%v", index, content, err)
		}
		if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_embedding_job($1, $2, $3, $4::real[])
`, embedding.JobID, workerID, embedding.LeaseToken,
			memoryHybridVectorLiteral(index)).Scan(new(bool)); err != nil {
			t.Fatalf("complete derived embedding %d: %v", index, err)
		}
	}

	var candidatesJSON []byte
	var status, resultCode, fallbackCode string
	var exactCount, bm25Count, vectorCount, rrfCount int
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, fallback_code, exact_count, bm25_count,
       vector_count, rrf_count, candidates
FROM memory_prepare_l2_scene_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', false
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query), query,
		memoryHybridVectorLiteral(0)).Scan(
		&status, &resultCode, &fallbackCode, &exactCount, &bm25Count,
		&vectorCount, &rrfCount, &candidatesJSON,
	); err != nil || status != "pending" || resultCode != "CANDIDATES_READY" ||
		rrfCount == 0 || vectorCount != 2 {
		t.Fatalf("Scene search = %q/%q/%q exact:%d bm25:%d vector:%d rrf:%d/%v",
			status, resultCode, fallbackCode, exactCount, bm25Count,
			vectorCount, rrfCount, err)
	}
	var candidates []struct {
		SceneID  string `json:"sceneId"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil || len(candidates) == 0 {
		t.Fatalf("decode Scene candidates = %#v/%v raw=%s", candidates, err, candidatesJSON)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"sceneId":  candidates[0].SceneID,
		"revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var injectedCount, finalCount int
	if err := db.QueryRowContext(ctx, `
SELECT status, final_count, injected_count
FROM memory_record_l2_scene_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, 20, 25
)
`, observationID, userID, assistantID, string(finalJSON)).Scan(
		&status, &finalCount, &injectedCount,
	); err != nil || status != "completed" || finalCount != 1 || injectedCount != 0 {
		t.Fatalf("shadow record = %q final:%d injected:%d/%v",
			status, finalCount, injectedCount, err)
	}

	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_l2_scene_detail($1, $2)
`, otherUserID, globalSceneID).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L2_SCENE_NOT_FOUND") {
		t.Fatalf("cross-user Scene detail error = %v", err)
	}
	testMemoryL2StaleAndPurge(
		t, ctx, db, userID, globalMemoryA, globalSceneID, workerID,
	)
	testMemoryL2PromotionLifecycle(
		t, ctx, db, userID, conversationID, sourceID, globalSceneID,
		query,
	)
	assertMemoryL2RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT count(*) FROM user_memory_scenes`)
	assertMemoryL2RoleDenied(t, ctx, db, "memory_worker_runtime",
		`SELECT memory_governance_l2_scene_snapshot('1b000000-0000-4000-8000-000000000001')`)
	assertMemoryL2RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT memory_operator_rollback_l2_scene(gen_random_uuid(), 'TEST')`)

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L2_SCENE_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS") {
		t.Fatalf("guarded 062 down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
DELETE FROM message_memory_l2_scene_observations;
DELETE FROM user_memory_scenes;
`)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 62 {
		t.Fatalf("clean 062 down = %#v/%v", down, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 62 {
		t.Fatalf("clean 061 -> 062 replay = %#v/%v", reapplied, err)
	}
}

func testMemoryL2StaleAndPurge(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	memoryID string,
	sceneID string,
	workerID string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	updatedContent := "Concise answer style corrected"
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memories
SET content = $2, normalized_content = lower($2), revision = revision + 1,
    content_hash = encode(sha256(convert_to($2, 'UTF8')), 'hex'),
    updated_at = clock_timestamp()
WHERE id = $1 AND user_id = $3
`, memoryID, updatedContent, userID); err != nil {
		t.Fatalf("update L1 for Scene invalidation: %v", err)
	}
	var lifecycle string
	var purgeAfter time.Time
	var projectionCount int
	if err := tx.QueryRowContext(ctx, `
SELECT lifecycle_status, purge_after,
  (SELECT count(*) FROM user_memory_derived_search_projections
   WHERE entity_type = 'l2_scene' AND entity_id = $1)
FROM user_memory_scenes WHERE id = $1 AND user_id = $2
`, sceneID, userID).Scan(&lifecycle, &purgeAfter, &projectionCount); err != nil ||
		lifecycle != "stale" || purgeAfter.IsZero() || projectionCount != 0 {
		t.Fatalf("L1 invalidation = lifecycle:%q purge:%s projection:%d err:%v",
			lifecycle, purgeAfter, projectionCount, err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_scenes
SET stale_at = clock_timestamp() - interval '2 seconds',
    purge_after = clock_timestamp() - interval '1 second'
WHERE id = $1
`, sceneID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_scene_jobs
SET available_at = clock_timestamp() - interval '1 second',
    updated_at = clock_timestamp()
WHERE target_scene_id = $1 AND stage = 'purge' AND status = 'pending'
`, sceneID); err != nil {
		t.Fatal(err)
	}
	leaseToken := "cb000000-0000-4000-8000-000000000001"
	var jobID, stage, targetSceneID string
	if err := tx.QueryRowContext(ctx, `
SELECT job_id, stage, target_scene_id
FROM memory_worker_claim_l2_scene_job($1, $2, 60, false)
`, workerID, leaseToken).Scan(&jobID, &stage, &targetSceneID); err != nil ||
		stage != "purge" || targetSceneID != sceneID {
		t.Fatalf("claim provider-free Scene purge = %q/%q/%q/%v",
			jobID, stage, targetSceneID, err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_purge($1, $2, $3)
`, jobID, workerID, leaseToken).Scan(new(bool)); err != nil {
		t.Fatalf("complete provider-free Scene purge: %v", err)
	}
	var content string
	var memberCount int
	if err := tx.QueryRowContext(ctx, `
SELECT content,
  (SELECT count(*) FROM user_memory_scene_members WHERE scene_id = $1)
FROM user_memory_scenes WHERE id = $1
`, sceneID).Scan(&content, &memberCount); err != nil ||
		content != "" || memberCount != 0 {
		t.Fatalf("purged Scene = content:%q members:%d err:%v",
			content, memberCount, err)
	}
}

func testMemoryL2PromotionLifecycle(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	conversationID string,
	sourceID string,
	sceneID string,
	query string,
) {
	t.Helper()
	if err := db.QueryRowContext(ctx, `
SELECT memory_operator_promote_l2_scene(gen_random_uuid(), '{}'::jsonb, '{}'::jsonb)
`).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L2_SCENE_PROMOTION_EVIDENCE_INVALID") {
		t.Fatalf("malformed promotion evidence error = %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_state
SET active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1',
    updated_at = clock_timestamp()
WHERE user_id = $1
`, userID); err != nil {
		t.Fatal(err)
	}
	// PostgreSQL stores timestamptz at microsecond precision. Keep the SQL
	// parameters and RFC3339 canary evidence on that same boundary so the first
	// eligible observation cannot be rounded outside the inclusive window.
	windowEnd := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	windowStart := windowEnd.Add(-8 * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `
WITH inserted AS (
  INSERT INTO messages(
    id, conversation_id, user_id, parent_message_id, sequence_no,
    role, status, content, created_at, updated_at, completed_at
  )
  SELECT gen_random_uuid(), $1, $2, $3, 1000 + ordinal,
    'assistant', 'completed', 'synthetic canary', sampled_at, sampled_at, sampled_at
  FROM generate_series(1, 100) ordinal
  CROSS JOIN LATERAL (
    SELECT $4::timestamptz + ($5::timestamptz - $4::timestamptz)
      * ((ordinal - 1)::double precision / 99.0) sampled_at
  ) sample
  RETURNING id, completed_at
)
INSERT INTO message_memory_l2_scene_observations(
  id, assistant_message_id, user_id, conversation_id, mode, profile_id,
  retrieval_profile_id, generation, query_sha256, result_sha256,
  status, result_code, query_embedding_status, rerank_status,
  fallback_code, created_at, updated_at
)
SELECT gen_random_uuid(), inserted.id, $2, $1, 'shadow',
  'memory_l2_scene_v1', 'memory_l2_scene_hybrid_bge_m3_rrf60_v1',
  state.active_l2_generation, repeat('a', 64), repeat('b', 64),
  'completed', 'COMPLETED', 'ready', 'applied', 'NONE',
  inserted.completed_at, inserted.completed_at
FROM inserted
JOIN user_memory_state state ON state.user_id = $2
`, conversationID, userID, sourceID, windowStart, windowEnd); err != nil {
		t.Fatalf("insert synthetic Scene canary: %v", err)
	}

	report := map[string]any{
		"schemaVersion": "neo-chat.memory-benchmark-report.v1",
		"passed":        true,
		"golden": map[string]any{
			"totalReviewed": 500, "developmentCount": 300,
			"validationCount": 100, "holdoutCount": 100,
		},
		"profile": map[string]any{
			"profileId": "memory_l2_scene_hybrid_bge_m3_rrf60_v1",
			"metrics": map[string]any{
				"candidateRecallAt20": 0.96, "finalRecallAt5": 0.92,
				"currentFactAccuracy": 0.97, "falseInjectionRate": 0.01,
			},
			"budgets": map[string]any{
				"p95LatencyMilliseconds": 800, "p99LatencyMilliseconds": 1400,
				"maximumPromptMemoryTokens": 500, "hardCutoffViolationCount": 0,
			},
			"safety": map[string]any{
				"crossUserLeakCount": 0, "deletedMemoryLeakCount": 0,
				"secretLeakCount": 0, "untrustedSourceLeakCount": 0,
				"unauthorizedProviderEgressCount": 0,
			},
			"providerCostRatio": 0.10,
		},
	}
	benchmarkJSON, err := json.Marshal(map[string]any{
		"report": report, "reportSha256": strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	canaryJSON, err := json.Marshal(map[string]any{
		"currentFactAccuracy": 0.97, "crossUserLeakCount": 0,
		"deletedMemoryLeakCount": 0, "eligibleTurns": 100,
		"falseInjectionRate": 0.01, "reportSha256": strings.Repeat("d", 64),
		"reviewedTurns": 100, "secretLeakCount": 0,
		"unauthorizedProviderEgressCount": 0,
		"windowEndedAt":                   windowEnd.Format(time.RFC3339Nano),
		"windowStartedAt":                 windowStart.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	var observationCount, sceneDeadLetters, embeddingDeadLetters int
	var hybridStateCount, badObservationStateCount int
	var firstObservation, lastObservation time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM message_memory_l2_scene_observations
   WHERE mode = 'shadow' AND status = 'completed'
     AND created_at BETWEEN $1 AND $2),
  (SELECT min(created_at) FROM message_memory_l2_scene_observations
   WHERE mode = 'shadow' AND status = 'completed'
     AND created_at BETWEEN $1 AND $2),
  (SELECT max(created_at) FROM message_memory_l2_scene_observations
   WHERE mode = 'shadow' AND status = 'completed'
     AND created_at BETWEEN $1 AND $2),
  (SELECT count(*) FROM user_memory_scene_jobs WHERE status = 'dead_letter'),
  (SELECT count(*) FROM user_memory_derived_embedding_jobs WHERE status = 'dead_letter'),
  (SELECT count(*) FROM user_memory_state
   WHERE active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'),
  (SELECT count(*) FROM message_memory_l2_scene_observations observation
   LEFT JOIN user_memory_state state ON state.user_id = observation.user_id
   WHERE observation.mode = 'shadow' AND observation.status = 'completed'
     AND observation.created_at BETWEEN $1 AND $2
     AND (state.user_id IS NULL OR state.active_retrieval_profile_id IS DISTINCT FROM
       'memory_hybrid_bge_m3_rrf60_v1'))
`, windowStart, windowEnd).Scan(
		&observationCount, &firstObservation, &lastObservation,
		&sceneDeadLetters, &embeddingDeadLetters, &hybridStateCount,
		&badObservationStateCount,
	); err != nil {
		t.Fatal(err)
	}
	if observationCount < 100 ||
		lastObservation.Sub(firstObservation) < 7*24*time.Hour ||
		sceneDeadLetters != 0 || embeddingDeadLetters != 0 ||
		hybridStateCount == 0 || badObservationStateCount != 0 {
		t.Fatalf(
			"synthetic promotion runtime fixture = observations:%d span:%s scene-dead:%d embedding-dead:%d hybrid-states:%d bad-observations:%d",
			observationCount, lastObservation.Sub(firstObservation),
			sceneDeadLetters, embeddingDeadLetters, hybridStateCount,
			badObservationStateCount,
		)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_operator_promote_l2_scene($1, $2::jsonb, $3::jsonb)
`, "db000000-0000-4000-8000-000000000001", string(benchmarkJSON),
		string(canaryJSON)).Scan(new([]byte)); err != nil {
		t.Fatalf("synthetic Scene promotion: %v", err)
	}
	var profileStatus, sceneStatus string
	if err := tx.QueryRowContext(ctx, `
SELECT profile.lifecycle_status, scene.lifecycle_status
FROM memory_l2_scene_profiles profile
JOIN user_memory_scenes scene ON scene.id = $1
WHERE profile.profile_id = 'memory_l2_scene_v1'
`, sceneID).Scan(&profileStatus, &sceneStatus); err != nil ||
		profileStatus != "active" || sceneStatus != "active" {
		t.Fatalf("promoted Scene lifecycle = profile:%q scene:%q err:%v",
			profileStatus, sceneStatus, err)
	}

	activeAssistantID := "eb000000-0000-4000-8000-000000000001"
	activeObservationID := "fb000000-0000-4000-8000-000000000001"
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content
) VALUES ($1, $2, $3, $4, 2200, 'assistant', 'streaming', '')
`, activeAssistantID, conversationID, userID, sourceID); err != nil {
		t.Fatal(err)
	}
	var candidatesJSON []byte
	if err := tx.QueryRowContext(ctx, `
SELECT candidates FROM memory_prepare_l2_scene_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', true
)
`, activeObservationID, userID, conversationID, activeAssistantID,
		sha256Hex(query), query, memoryHybridVectorLiteral(0)).Scan(&candidatesJSON); err != nil {
		t.Fatalf("active Scene prepare: %v", err)
	}
	var candidates []struct {
		SceneID  string `json:"sceneId"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil || len(candidates) == 0 {
		t.Fatalf("active Scene candidates = %#v/%v", candidates, err)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"sceneId": candidates[0].SceneID, "revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var injectedCount int
	var finalScenes []byte
	if err := tx.QueryRowContext(ctx, `
SELECT injected_count, final_scenes FROM memory_record_l2_scene_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, 20, 25
)
`, activeObservationID, userID, activeAssistantID, string(finalJSON)).Scan(
		&injectedCount, &finalScenes,
	); err != nil || injectedCount != 1 || !strings.Contains(string(finalScenes), "content") {
		t.Fatalf("active Scene final = injected:%d scenes:%s err:%v",
			injectedCount, finalScenes, err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_operator_rollback_l2_scene($1, 'SYNTHETIC_TEST')
`, "db000000-0000-4000-8000-000000000002").Scan(new([]byte)); err != nil {
		t.Fatalf("Scene rollback: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT lifecycle_status FROM memory_l2_scene_profiles
WHERE profile_id = 'memory_l2_scene_v1'
`).Scan(&profileStatus); err != nil || profileStatus != "rolled_back" {
		t.Fatalf("rolled-back profile = %q/%v", profileStatus, err)
	}
}

type memoryL2SceneLease struct {
	JobID      string
	Stage      string
	ScopeType  string
	LeaseToken string
	ProjectID  sql.NullString
	Generation int64
}

func claimMemoryL2SceneJob(
	ctx context.Context,
	db *sql.DB,
	workerID string,
	leaseToken string,
	refreshEnabled bool,
) (memoryL2SceneLease, bool, error) {
	var lease memoryL2SceneLease
	lease.LeaseToken = leaseToken
	var userID string
	var targetSceneID, sourceWatermark, providerID, modelID sql.NullString
	var scopeGeneration sql.NullInt64
	var visibilityEpoch int64
	var profileID string
	var attemptCount, maxAttempts int
	var providerUpdatedAt sql.NullTime
	var leaseExpiresAt time.Time
	err := db.QueryRowContext(ctx, `
SELECT job_id, stage, user_id, scope_type, project_id, target_scene_id,
       scope_generation, visibility_epoch, generation, profile_id,
       source_watermark, attempt_count, max_attempts, provider_record_id,
       provider_config_updated_at, model_id, lease_expires_at
FROM memory_worker_claim_l2_scene_job($1, $2, 60, $3)
`, workerID, leaseToken, refreshEnabled).Scan(
		&lease.JobID, &lease.Stage, &userID, &lease.ScopeType, &lease.ProjectID,
		&targetSceneID, &scopeGeneration, &visibilityEpoch, &lease.Generation,
		&profileID, &sourceWatermark, &attemptCount, &maxAttempts, &providerID,
		&providerUpdatedAt, &modelID, &leaseExpiresAt,
	)
	if err == sql.ErrNoRows {
		return memoryL2SceneLease{}, false, nil
	}
	return lease, err == nil, err
}

type memoryL2HydratedMemory struct {
	ID string `json:"id"`
}

func hydrateMemoryL2Scene(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	lease memoryL2SceneLease,
	workerID string,
) ([]memoryL2HydratedMemory, error) {
	t.Helper()
	var memoriesJSON []byte
	var userID, scopeType, profileID, sourceWatermark string
	var projectID, providerRecordID, providerID, providerLabel sql.NullString
	var encryptedSecretRef, modelID sql.NullString
	var scopeGeneration, visibilityEpoch, generation int64
	var sensitive bool
	var providerConfig []byte
	var providerUpdatedAt sql.NullTime
	err := db.QueryRowContext(ctx, `
SELECT user_id, scope_type, project_id, scope_generation, visibility_epoch,
       generation, profile_id, source_watermark, sensitive_memory_enabled,
       memories, provider_record_id, provider_id, provider_label,
       encrypted_secret_ref, provider_config, provider_config_updated_at, model_id
FROM memory_worker_hydrate_l2_scene_refresh($1, $2, $3)
`, lease.JobID, workerID, lease.LeaseToken).Scan(
		&userID, &scopeType, &projectID, &scopeGeneration, &visibilityEpoch,
		&generation, &profileID, &sourceWatermark, &sensitive, &memoriesJSON,
		&providerRecordID, &providerID, &providerLabel, &encryptedSecretRef,
		&providerConfig, &providerUpdatedAt, &modelID,
	)
	if err != nil {
		return nil, err
	}
	var memories []memoryL2HydratedMemory
	if err := json.Unmarshal(memoriesJSON, &memories); err != nil {
		return nil, err
	}
	return memories, nil
}

type memoryL2EmbeddingLease struct {
	JobID      string
	SceneID    string
	LeaseToken string
}

func claimMemoryL2Embedding(
	ctx context.Context,
	db *sql.DB,
	workerID string,
	leaseToken string,
) (memoryL2EmbeddingLease, bool, error) {
	lease := memoryL2EmbeddingLease{LeaseToken: leaseToken}
	var userID, contentHash, sourceWatermark, profileID, modelID, providerID string
	var sceneRevision, visibilityEpoch, generation int64
	var dimensions, attemptCount, maxAttempts int
	var providerUpdatedAt, leaseExpiresAt time.Time
	err := db.QueryRowContext(ctx, `
SELECT job_id, user_id, scene_id, scene_revision, content_hash, source_watermark,
       visibility_epoch, generation, embedding_profile_id, embedding_model_id,
       embedding_dimensions, attempt_count, max_attempts, provider_record_id,
       provider_config_updated_at, lease_expires_at
FROM memory_worker_claim_l2_scene_embedding_job($1, $2, 60)
`, workerID, leaseToken).Scan(
		&lease.JobID, &userID, &lease.SceneID, &sceneRevision, &contentHash,
		&sourceWatermark, &visibilityEpoch, &generation, &profileID, &modelID,
		&dimensions, &attemptCount, &maxAttempts, &providerID,
		&providerUpdatedAt, &leaseExpiresAt,
	)
	if err == sql.ErrNoRows {
		return memoryL2EmbeddingLease{}, false, nil
	}
	return lease, err == nil, err
}

func assertMemoryL2RoleDenied(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	role string,
	query string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("set role %s: %v", role, err)
	}
	if _, err := tx.ExecContext(ctx, query); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("role %s query denial = %v", role, err)
	}
}
