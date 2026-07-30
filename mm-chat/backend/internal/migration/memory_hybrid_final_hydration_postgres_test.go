package migration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestMemoryHybridFinalHydrationLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 64))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 064: %v", err)
	}
	runner := NewRunner(db, phase15MigrationFSThrough(t, 65))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 65 {
		t.Fatalf("apply 065 = %#v/%v", applied, err)
	}
	assertMemoryHybridFinalHydrationPrivileges(t, ctx, db)

	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 65 {
		t.Fatalf("065 down = %#v/%v", down, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 65 {
		t.Fatalf("065 re-up = %#v/%v", reapplied, err)
	}
	assertMemoryHybridFinalHydrationPrivileges(t, ctx, db)

	const (
		userID         = "16000000-0000-4000-8000-000000000001"
		conversationID = "26000000-0000-4000-8000-000000000001"
		sourceID       = "36000000-0000-4000-8000-000000000001"
		assistantID    = "36000000-0000-4000-8000-000000000002"
		memoryID       = "56000000-0000-4000-8000-000000000001"
		observationID  = "66000000-0000-4000-8000-000000000001"
	)
	query := "How should project answers be written?"
	memoryContent := "Keep project answers concise"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR13');
INSERT INTO conversations(id, user_id, title)
VALUES ($2, $1, 'PR13 hydration');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed', $5, clock_timestamp());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'streaming', '');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES ($1, true, true, false, false)
ON CONFLICT (user_id) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  search_enabled = EXCLUDED.search_enabled,
  sensitive_memory_enabled = EXCLUDED.sensitive_memory_enabled;
`, userID, conversationID, sourceID, assistantID, query)

	var returnedMemoryID string
	if err := db.QueryRowContext(ctx, `
SELECT id::text FROM memory_upsert_global_manual(
  $1::uuid, $2::uuid, 'preference', $3::text, lower($3::text),
  5::smallint, ARRAY['style']::text[], NULL, NULL, true
)
`, memoryID, userID, memoryContent).Scan(&returnedMemoryID); err != nil ||
		returnedMemoryID != memoryID {
		t.Fatalf("create hydration Memory = %q/%v", returnedMemoryID, err)
	}

	queryVector := memoryHybridVectorLiteral(0)
	setMemoryHybridProjectionReady(t, ctx, db, memoryID, queryVector)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO message_memory_hybrid_shadow_observations(
  id, assistant_message_id, user_id, conversation_id,
  retrieval_profile_id, embedding_profile_id, projection_generation,
  query_sha256, baseline_sha256, result_sha256, status, result_code,
  query_embedding_status, rerank_status, fallback_code,
  rrf_count, rerank_count, final_count, estimated_tokens,
  target_tokens_exceeded, duration_millis
) SELECT
  $2::uuid, $3::uuid, $4::uuid, $5::uuid,
  'memory_hybrid_bge_m3_rrf60_v1', 'siliconflow_bge_m3_v1',
  projection.projection_generation,
  encode(sha256(convert_to($6, 'UTF8')), 'hex'),
  repeat('0', 64), repeat('1', 64), 'completed', 'OK',
  'ready', 'applied', 'NONE', 1, 1, 1, 20, false, 10
FROM user_memory_search_projections projection
WHERE projection.memory_id = $1::uuid;

INSERT INTO message_memory_hybrid_shadow_results(
  observation_id, user_id, lane, ordinal, memory_id,
  memory_revision, scope_type
) SELECT $2::uuid, memory.user_id, 'final', 1, memory.id,
         memory.revision, memory.scope_type
FROM user_memories memory
WHERE memory.id = $1::uuid;
`, memoryID, observationID, assistantID, userID, conversationID, query)

	assertMemoryHybridFinalHydrationValue(
		t, ctx, db, observationID, userID, assistantID, memoryID, memoryContent,
	)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories
SET enabled = false, deleted_at = clock_timestamp()
WHERE id = $1::uuid
`, memoryID)
	assertMemoryHybridFinalHydrationRejected(t, ctx, db, observationID, userID, assistantID)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET enabled = true, deleted_at = NULL WHERE id = $1::uuid
`, memoryID)
	setMemoryHybridProjectionReady(t, ctx, db, memoryID, queryVector)
	assertMemoryHybridFinalHydrationValue(
		t, ctx, db, observationID, userID, assistantID, memoryID, memoryContent,
	)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET revision = revision + 1 WHERE id = $1::uuid
`, memoryID)
	assertMemoryHybridFinalHydrationRejected(t, ctx, db, observationID, userID, assistantID)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET revision = revision - 1 WHERE id = $1::uuid
	`, memoryID)
	setMemoryHybridProjectionReady(t, ctx, db, memoryID, queryVector)
	assertMemoryHybridFinalHydrationValue(
		t, ctx, db, observationID, userID, assistantID, memoryID, memoryContent,
	)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_settings SET search_enabled = false WHERE user_id = $1::uuid
`, userID)
	assertMemoryHybridFinalHydrationRejected(t, ctx, db, observationID, userID, assistantID)
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_settings SET search_enabled = true WHERE user_id = $1::uuid
`, userID)

	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_state
SET active_projection_generation = active_projection_generation + 1
WHERE user_id = $1::uuid
`, userID)
	assertMemoryHybridFinalHydrationRejected(t, ctx, db, observationID, userID, assistantID)
}

func setMemoryHybridProjectionReady(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	memoryID string,
	queryVector string,
) {
	t.Helper()
	var embeddingStatus string
	var embeddingReady bool
	if err := db.QueryRowContext(ctx, `
UPDATE user_memory_search_projections
SET embedding_status = 'ready',
    embedding_vector = $2::real[]::vector(1024),
    embedding_error_code = NULL,
    embedding_updated_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE memory_id = $1::uuid
RETURNING embedding_status, embedding_vector IS NOT NULL
`, memoryID, queryVector).Scan(&embeddingStatus, &embeddingReady); err != nil ||
		embeddingStatus != "ready" || !embeddingReady {
		t.Fatalf("prepare ready projection = %q/%t/%v", embeddingStatus, embeddingReady, err)
	}
}

func assertMemoryHybridFinalHydrationPrivileges(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	var apiExecute, workerExecute, publicExecute bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege(
    'go_api_runtime',
    'memory_hydrate_hybrid_final(uuid,uuid,uuid)',
    'EXECUTE'
  ),
  has_function_privilege(
    'memory_worker_runtime',
    'memory_hydrate_hybrid_final(uuid,uuid,uuid)',
    'EXECUTE'
  ),
  EXISTS (
    SELECT 1
    FROM pg_proc function
    CROSS JOIN LATERAL aclexplode(
      coalesce(function.proacl, acldefault('f', function.proowner))
    ) privilege
    WHERE function.oid =
      'memory_hydrate_hybrid_final(uuid,uuid,uuid)'::regprocedure
      AND privilege.grantee = 0
      AND privilege.privilege_type = 'EXECUTE'
  )
`).Scan(&apiExecute, &workerExecute, &publicExecute); err != nil {
		t.Fatal(err)
	}
	if !apiExecute || workerExecute || publicExecute {
		t.Fatalf(
			"065 privileges = api:%t worker:%t public:%t",
			apiExecute, workerExecute, publicExecute,
		)
	}
}

func assertMemoryHybridFinalHydrationValue(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	observationID string,
	userID string,
	assistantID string,
	wantMemoryID string,
	wantContent string,
) {
	t.Helper()
	var ordinal int
	var memoryID, scopeType, memoryType, content string
	var revision int64
	if err := db.QueryRowContext(ctx, `
SELECT ordinal, memory_id::text, memory_revision, scope_type,
       memory_type, content
FROM memory_hydrate_hybrid_final($1::uuid, $2::uuid, $3::uuid)
	`, observationID, userID, assistantID).Scan(
		&ordinal, &memoryID, &revision, &scopeType, &memoryType, &content,
	); err != nil || ordinal != 1 || memoryID != wantMemoryID || revision != 1 ||
		scopeType != "global" || memoryType != "preference" || content != wantContent {
		var state string
		_ = db.QueryRowContext(ctx, `
SELECT jsonb_build_object(
  'memoryRevision', memory.revision,
  'projectionRevision', projection.memory_revision,
  'contentHashMatch', memory.content_hash = projection.content_hash,
  'visibilityMatch', memory.visibility_epoch = projection.visibility_epoch,
  'scopeMatch', memory.scope_type = projection.scope_type,
  'scopeGenerationMatch', memory.scope_generation = projection.scope_generation,
  'memoryEnabled', memory.enabled,
  'memoryDeleted', memory.deleted_at IS NOT NULL,
  'memoryLifecycle', memory.lifecycle_status,
  'memorySensitivity', memory.sensitivity,
  'projectionLexical', projection.lexical_status,
  'projectionEmbedding', projection.embedding_status,
  'projectionVector', projection.embedding_vector IS NOT NULL,
  'projectionGeneration', projection.projection_generation,
  'activeGeneration', state.active_projection_generation,
  'settingsEnabled', settings.enabled,
  'settingsSearch', settings.search_enabled
)::text
FROM user_memories memory
JOIN user_memory_search_projections projection
  ON projection.memory_id = memory.id AND projection.user_id = memory.user_id
JOIN user_memory_state state ON state.user_id = memory.user_id
JOIN user_memory_settings settings ON settings.user_id = memory.user_id
WHERE memory.id = $1::uuid
`, wantMemoryID).Scan(&state)
		t.Fatalf(
			"hybrid final hydration = %d/%q/%d/%q/%q/%q/%v state=%s",
			ordinal, memoryID, revision, scopeType, memoryType, content, err, state,
		)
	}
}

func assertMemoryHybridFinalHydrationRejected(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	observationID string,
	userID string,
	assistantID string,
) {
	t.Helper()
	var memoryID string
	err := db.QueryRowContext(ctx, `
SELECT memory_id::text
FROM memory_hydrate_hybrid_final($1::uuid, $2::uuid, $3::uuid)
`, observationID, userID, assistantID).Scan(&memoryID)
	if err == nil || err == sql.ErrNoRows ||
		!strings.Contains(err.Error(), "MEMORY_HYBRID_FINAL_HYDRATION_") {
		t.Fatalf("stale hybrid final was not rejected: memory=%q err=%v", memoryID, err)
	}
}
