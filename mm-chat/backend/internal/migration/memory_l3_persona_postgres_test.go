package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMemoryL3PersonaLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 62))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 062: %v", err)
	}

	const (
		userID             = "1c000000-0000-4000-8000-000000000001"
		otherUserID        = "1c000000-0000-4000-8000-000000000002"
		projectID          = "2c000000-0000-4000-8000-000000000001"
		conversationID     = "3c000000-0000-4000-8000-000000000001"
		sourceID           = "4c000000-0000-4000-8000-000000000001"
		assistantID        = "4c000000-0000-4000-8000-000000000002"
		globalMemoryA      = "5c000000-0000-4000-8000-000000000001"
		globalMemoryB      = "5c000000-0000-4000-8000-000000000002"
		projectMemory      = "5c000000-0000-4000-8000-000000000003"
		conversationMemory = "5c000000-0000-4000-8000-000000000004"
		unstableMemory     = "5c000000-0000-4000-8000-000000000005"
		secretMemory       = "5c000000-0000-4000-8000-000000000006"
		synthesisProvider  = "6c000000-0000-4000-8000-000000000001"
		ragProvider        = "6c000000-0000-4000-8000-000000000002"
		workerID           = "7c000000-0000-4000-8000-000000000001"
		observationID      = "9c000000-0000-4000-8000-000000000001"
	)
	query := "concise Go answers"
	ragSecretRef := "fixture-pr12-rag-encrypted-secret-reference"
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
INSERT INTO users(id, display_name) VALUES ($1, 'PR12'), ($2, 'PR12 Other');
INSERT INTO projects(id, user_id, name) VALUES ($3, $1, 'PR12 Project');
INSERT INTO conversations(id, user_id, project_id, title)
VALUES ($4, $1, $3, 'PR12');
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
  ($8, $1, 'preference', 'Prefers concise answers', 'prefers concise answers',
   'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('Prefers concise answers', 'UTF8')), 'hex'), 'manual', 5),
  ($9, $1, 'fact', 'Uses Go for backend services', 'uses go for backend services',
   'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('Uses Go for backend services', 'UTF8')), 'hex'), 'manual', 4),
  ($10, $1, 'decision', 'Project uses PostgreSQL 17', 'project uses postgresql 17',
   'manual', 'project', $3, NULL, 1,
   encode(sha256(convert_to('Project uses PostgreSQL 17', 'UTF8')), 'hex'), 'manual', 5),
  ($11, $1, 'instruction', 'Conversation-only instruction', 'conversation-only instruction',
   'manual', 'conversation', NULL, $4, 1,
   encode(sha256(convert_to('Conversation-only instruction', 'UTF8')), 'hex'), 'manual', 3),
  ($12, $1, 'project', 'Global project context is not Persona authority',
   'global project context is not persona authority', 'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('Global project context is not Persona authority', 'UTF8')), 'hex'),
   'manual', 3),
  ($13, $1, 'fact', 'api_key=sk-fixture-secret', 'api_key=sk-fixture-secret',
   'manual', 'global', NULL, NULL, 1,
   encode(sha256(convert_to('api_key=sk-fixture-secret', 'UTF8')), 'hex'), 'manual', 5);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config, created_at, updated_at
) VALUES
  ($14, $1, 'CUSTOM', 'Fixture synthesis', 'fixture-synthesis-secret',
   '{"enabled":true}'::jsonb, '2026-07-28T00:00:00Z', '2026-07-28T00:00:00Z'),
  ($15, $1, 'RAG:SILICONFLOW', 'Fixture RAG', $16,
   jsonb_build_object(
     'kind', 'rag', 'ragProvider', 'siliconflow', 'enabled', true,
     'connectionTestedAt', '2026-07-28T00:00:00Z',
     'connectionTestSha256', $17
   ), '2026-07-28T00:00:00Z', '2026-07-28T00:00:00Z');
INSERT INTO task_model_settings(user_id, memory) VALUES ($1, 'CUSTOM:persona-model');
`, userID, otherUserID, projectID, conversationID, sourceID, assistantID,
		query, globalMemoryA, globalMemoryB, projectMemory, conversationMemory,
		unstableMemory, secretMemory, synthesisProvider, ragProvider, ragSecretRef,
		ragAttestation)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 63))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 63 {
		t.Fatalf("apply 063 over L1 fixture = %#v/%v", applied, err)
	}

	var refreshJobs int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_persona_jobs
WHERE user_id = $1 AND stage = 'refresh'
`, userID).Scan(&refreshJobs); err != nil || refreshJobs != 1 {
		t.Fatalf("063 backfill refresh jobs = %d/%v", refreshJobs, err)
	}
	if _, found, err := claimMemoryL3PersonaJob(
		ctx, db, workerID, "ac000000-0000-4000-8000-000000000001", false,
	); err != nil || found {
		t.Fatalf("default-off Persona refresh claim = found:%t err:%v", found, err)
	}
	lease, found, err := claimMemoryL3PersonaJob(
		ctx, db, workerID, "ac000000-0000-4000-8000-000000000002", true,
	)
	if err != nil || !found || lease.Stage != "refresh" {
		t.Fatalf("claim Persona refresh = %#v/%t/%v", lease, found, err)
	}
	memories, err := hydrateMemoryL3Persona(ctx, db, lease, workerID)
	if err != nil || len(memories) != 2 {
		t.Fatalf("hydrate Persona = %#v/%v", memories, err)
	}
	memberIDs := []string{memories[0].ID, memories[1].ID}
	for _, forbiddenID := range []string{
		projectMemory, conversationMemory, unstableMemory, secretMemory,
	} {
		if slicesContain(memberIDs, forbiddenID) {
			t.Fatalf("non-Global-stable Memory %s widened into Persona input", forbiddenID)
		}
	}

	invalidPayload, err := json.Marshal(map[string]any{
		"content": "Forged Persona", "memberMemoryIds": []string{globalMemoryA, projectMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, lease.JobID, workerID, lease.LeaseToken, string(invalidPayload)).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L3_PERSONA_MEMBER_INVALID") {
		t.Fatalf("forged Persona member error = %v", err)
	}

	content := "Prefers concise answers and uses Go for backend services."
	payload, err := json.Marshal(map[string]any{
		"content": content, "memberMemoryIds": memberIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	var applyResult []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, lease.JobID, workerID, lease.LeaseToken, string(payload)).Scan(&applyResult); err != nil {
		t.Fatalf("complete Persona refresh: %v", err)
	}
	var personaID string
	var personaGeneration, personaRevision int64
	var tokenCount, projectionCount, embeddingJobCount int
	var sensitiveInput bool
	if err := db.QueryRowContext(ctx, `
SELECT persona.id, persona.generation, persona.revision, persona.token_count,
       persona.sensitive_input_included,
       (SELECT count(*) FROM user_memory_persona_search_projections
        WHERE user_id = $1),
       (SELECT count(*) FROM user_memory_persona_embedding_jobs
        WHERE user_id = $1)
FROM user_memory_persona_versions persona WHERE persona.user_id = $1
`, userID).Scan(
		&personaID, &personaGeneration, &personaRevision, &tokenCount,
		&sensitiveInput, &projectionCount, &embeddingJobCount,
	); err != nil || tokenCount < 1 || tokenCount > 300 || sensitiveInput ||
		projectionCount != 1 || embeddingJobCount != 1 {
		t.Fatalf("Persona apply = id:%s generation:%d revision:%d tokens:%d sensitive:%t projections:%d embedding-jobs:%d err:%v result:%s",
			personaID, personaGeneration, personaRevision, tokenCount, sensitiveInput,
			projectionCount, embeddingJobCount, err, applyResult)
	}

	embedding, found, err := claimMemoryL3PersonaEmbedding(
		ctx, db, workerID, "bc000000-0000-4000-8000-000000000001",
	)
	if err != nil || !found || embedding.PersonaID != personaID {
		t.Fatalf("claim Persona embedding = %#v/%t/%v", embedding, found, err)
	}
	var embeddedContent string
	if err := db.QueryRowContext(ctx, `
SELECT content FROM memory_worker_hydrate_l3_persona_embedding_job($1, $2, $3)
`, embedding.JobID, workerID, embedding.LeaseToken).Scan(&embeddedContent); err != nil ||
		embeddedContent != content {
		t.Fatalf("hydrate Persona embedding = %q/%v", embeddedContent, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_embedding_job($1, $2, $3, $4::real[])
`, embedding.JobID, workerID, embedding.LeaseToken,
		memoryHybridVectorLiteral(0)).Scan(new(bool)); err != nil {
		t.Fatalf("complete Persona embedding: %v", err)
	}

	var candidatesJSON []byte
	var status, resultCode, fallbackCode string
	var exactCount, bm25Count, vectorCount, rrfCount int
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, fallback_code, exact_count, bm25_count,
       vector_count, rrf_count, candidates
FROM memory_prepare_l3_persona_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', false
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query), query,
		memoryHybridVectorLiteral(0)).Scan(
		&status, &resultCode, &fallbackCode, &exactCount, &bm25Count,
		&vectorCount, &rrfCount, &candidatesJSON,
	); err != nil || status != "pending" || resultCode != "CANDIDATES_READY" ||
		rrfCount != 1 || vectorCount != 1 {
		t.Fatalf("Persona search = %q/%q/%q exact:%d bm25:%d vector:%d rrf:%d/%v",
			status, resultCode, fallbackCode, exactCount, bm25Count,
			vectorCount, rrfCount, err)
	}
	var candidates []struct {
		PersonaID string `json:"personaId"`
		Revision  int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil ||
		len(candidates) != 1 || candidates[0].PersonaID != personaID {
		t.Fatalf("decode Persona candidates = %#v/%v raw=%s", candidates, err, candidatesJSON)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"personaId": candidates[0].PersonaID, "revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	spoofedTokenCount := tokenCount + 1
	if spoofedTokenCount > 300 {
		spoofedTokenCount = tokenCount - 1
	}
	if err := db.QueryRowContext(ctx, `
SELECT status FROM memory_record_l3_persona_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, $5, 25
)
`, observationID, userID, assistantID, string(finalJSON), spoofedTokenCount).Scan(
		new(string),
	); err == nil || !strings.Contains(err.Error(), "MEMORY_L3_PERSONA_TOKEN_COUNT_INVALID") {
		t.Fatalf("spoofed Persona token count error = %v", err)
	}
	var injectedCount, finalCount int
	var finalPersonas []byte
	if err := db.QueryRowContext(ctx, `
SELECT status, final_count, injected_count, final_personas
FROM memory_record_l3_persona_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, $5, 25
)
`, observationID, userID, assistantID, string(finalJSON), tokenCount).Scan(
		&status, &finalCount, &injectedCount, &finalPersonas,
	); err != nil || status != "completed" || finalCount != 1 || injectedCount != 0 ||
		string(finalPersonas) != "[]" {
		t.Fatalf("shadow Persona record = %q final:%d injected:%d personas:%s/%v",
			status, finalCount, injectedCount, finalPersonas, err)
	}

	if err := db.QueryRowContext(ctx, `
SELECT memory_governance_l3_persona_detail($1, $2)
`, otherUserID, personaID).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L3_PERSONA_NOT_FOUND") {
		t.Fatalf("cross-user Persona detail error = %v", err)
	}
	assertMemoryL3RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT count(*) FROM user_memory_persona_versions`)
	assertMemoryL3RoleDenied(t, ctx, db, "memory_worker_runtime",
		`SELECT memory_governance_l3_persona_snapshot('1c000000-0000-4000-8000-000000000001')`)
	assertMemoryL3RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT memory_operator_rollback_l3_persona(gen_random_uuid(), 'TEST')`)
	assertMemoryL3RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT memory_operator_promote_l3_persona(gen_random_uuid(), '{}'::jsonb, '{}'::jsonb)`)
	assertMemoryL3RoleDenied(t, ctx, db, "memory_worker_runtime",
		`SELECT memory_operator_promote_l3_persona(gen_random_uuid(), '{}'::jsonb, '{}'::jsonb)`)

	testMemoryL3LexicalFallback(
		t, ctx, db, userID, conversationID, sourceID, personaID,
		personaRevision, tokenCount,
	)
	testMemoryL3SensitivePolicyRejectsOldResponse(
		t, ctx, db, userID, globalMemoryA, globalMemoryB, workerID,
	)
	testMemoryL3LeaseReclaimRejectsOldResponse(
		t, ctx, db, userID, workerID,
	)
	testMemoryL3DisabledPreservedAcrossRebuild(
		t, ctx, db, userID, personaID, personaRevision,
		globalMemoryA, globalMemoryB, workerID,
	)
	testMemoryL3AccountCascade(t, ctx, db)

	testMemoryL3PromotionLifecycle(
		t, ctx, db, userID, conversationID, sourceID, personaID, tokenCount, query,
	)
	testMemoryL3StaleAndPurge(
		t, ctx, db, userID, globalMemoryA, personaID, personaGeneration, workerID,
	)

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L3_PERSONA_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS") {
		t.Fatalf("guarded 063 down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
DELETE FROM message_memory_l3_persona_observations;
DELETE FROM user_memory_persona_versions;
`)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 63 {
		t.Fatalf("clean 063 down = %#v/%v", down, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 63 {
		t.Fatalf("clean 062 -> 063 replay = %#v/%v", reapplied, err)
	}
}

func testMemoryL3LexicalFallback(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	conversationID string,
	sourceID string,
	personaID string,
	personaRevision int64,
	tokenCount int,
) {
	t.Helper()
	const (
		assistantID   = "4c000000-0000-4000-8000-000000000003"
		observationID = "9c000000-0000-4000-8000-000000000002"
	)
	query := "concise answers"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role, status, content
) VALUES ($1, $2, $3, $4, 3, 'assistant', 'streaming', '')
`, assistantID, conversationID, userID, sourceID)
	var candidatesJSON []byte
	var fallbackCode string
	var exactCount, bm25Count, vectorCount, rrfCount int
	if err := db.QueryRowContext(ctx, `
SELECT fallback_code, exact_count, bm25_count, vector_count, rrf_count, candidates
FROM memory_prepare_l3_persona_search(
  $1, $2, $3, $4, $5, $6, NULL::real[], 'failed', false
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query), query).Scan(
		&fallbackCode, &exactCount, &bm25Count, &vectorCount, &rrfCount,
		&candidatesJSON,
	); err != nil || fallbackCode != "QUERY_EMBEDDING_FAILED" || vectorCount != 0 ||
		rrfCount != 1 || exactCount+bm25Count < 1 {
		t.Fatalf("Persona lexical fallback = fallback:%q exact:%d bm25:%d vector:%d rrf:%d err:%v",
			fallbackCode, exactCount, bm25Count, vectorCount, rrfCount, err)
	}
	var candidates []struct {
		PersonaID string `json:"personaId"`
		Revision  int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil ||
		len(candidates) != 1 || candidates[0].PersonaID != personaID ||
		candidates[0].Revision != personaRevision {
		t.Fatalf("Persona lexical candidates = %#v/%v raw=%s",
			candidates, err, candidatesJSON)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"personaId": personaID, "revision": personaRevision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `
SELECT status FROM memory_record_l3_persona_search(
  $1, $2, $3, 'fallback', 'QUERY_EMBEDDING_FAILED',
  '[]'::jsonb, $4::jsonb, $5, 30
)
`, observationID, userID, assistantID, string(finalJSON), tokenCount).Scan(
		&status,
	); err != nil || status != "completed" {
		t.Fatalf("record Persona lexical fallback = %q/%v", status, err)
	}
}

func testMemoryL3SensitivePolicyRejectsOldResponse(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	globalMemoryA string,
	globalMemoryB string,
	workerID string,
) {
	t.Helper()
	const (
		sensitiveMemoryID = "5c000000-0000-4000-8000-000000000007"
		leaseToken        = "ac000000-0000-4000-8000-000000000010"
	)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_settings
SET sensitive_memory_enabled = true, updated_at = clock_timestamp()
WHERE user_id = $1;
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source, scope_type,
  scope_generation, content_hash, authority_kind, importance, sensitivity
) VALUES (
  $2, $1, 'fact', 'Home address is fixture district',
  'home address is fixture district', 'manual', 'global', 1,
  encode(sha256(convert_to('Home address is fixture district', 'UTF8')), 'hex'),
  'manual', 5, 'sensitive'
);
`, userID, sensitiveMemoryID); err != nil {
		t.Fatalf("prepare Sensitive Persona generation: %v", err)
	}
	lease, found, err := claimMemoryL3PersonaJob(ctx, tx, workerID, leaseToken, true)
	if err != nil || !found || lease.Stage != "refresh" {
		t.Fatalf("claim Sensitive Persona refresh = %#v/%t/%v", lease, found, err)
	}
	memories, err := hydrateMemoryL3Persona(ctx, tx, lease, workerID)
	if err != nil || len(memories) != 3 {
		t.Fatalf("hydrate Sensitive Persona = %#v/%v", memories, err)
	}
	memberIDs := make([]string, 0, len(memories))
	for _, memory := range memories {
		memberIDs = append(memberIDs, memory.ID)
	}
	for _, requiredID := range []string{globalMemoryA, globalMemoryB, sensitiveMemoryID} {
		if !slicesContain(memberIDs, requiredID) {
			t.Fatalf("Sensitive Persona hydration omitted %s: %#v", requiredID, memberIDs)
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_settings
SET sensitive_memory_enabled = false, updated_at = clock_timestamp()
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("disable Sensitive Memory before Persona completion: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"content":         "Old response still includes sensitive source authority.",
		"memberMemoryIds": memberIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, lease.JobID, workerID, lease.LeaseToken, string(payload)).Scan(new([]byte)); err == nil || !strings.Contains(err.Error(), "MEMORY_L3_PERSONA_SOURCE_DRIFT") {
		t.Fatalf("Sensitive off old Persona response error = %v", err)
	}
}

func testMemoryL3LeaseReclaimRejectsOldResponse(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	workerID string,
) {
	t.Helper()
	const (
		oldLeaseToken = "ac000000-0000-4000-8000-000000000020"
		newLeaseToken = "ac000000-0000-4000-8000-000000000021"
	)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `
SELECT memory_governance_rebuild_l3_personas($1)
`, userID).Scan(new([]byte)); err != nil {
		t.Fatalf("enqueue Persona lease reclaim fixture: %v", err)
	}
	oldLease, found, err := claimMemoryL3PersonaJob(
		ctx, tx, workerID, oldLeaseToken, true,
	)
	if err != nil || !found || oldLease.Stage != "refresh" {
		t.Fatalf("claim old Persona lease = %#v/%t/%v", oldLease, found, err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_persona_jobs
SET lease_expires_at = clock_timestamp() - interval '1 second'
WHERE job_id = $1
`, oldLease.JobID); err != nil {
		t.Fatalf("expire Persona lease: %v", err)
	}
	newLease, found, err := claimMemoryL3PersonaJob(
		ctx, tx, workerID, newLeaseToken, true,
	)
	if err != nil || !found || newLease.JobID != oldLease.JobID ||
		newLease.AttemptCount != oldLease.AttemptCount+1 {
		t.Fatalf("reclaim Persona lease = old:%#v new:%#v found:%t err:%v",
			oldLease, newLease, found, err)
	}
	payload, err := json.Marshal(map[string]any{
		"content":         "An old provider response must not win a reclaimed lease.",
		"memberMemoryIds": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, oldLease.JobID, workerID, oldLease.LeaseToken, string(payload)).Scan(new([]byte)); err == nil || !strings.Contains(err.Error(), "MEMORY_L3_PERSONA_SOURCE_DRIFT") {
		t.Fatalf("old Persona lease response error = %v", err)
	}
}

func testMemoryL3DisabledPreservedAcrossRebuild(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	personaID string,
	personaRevision int64,
	globalMemoryA string,
	globalMemoryB string,
	workerID string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `
SELECT memory_governance_set_l3_persona_enabled($1, $2, $3, false)
`, userID, personaID, personaRevision).Scan(new([]byte)); err != nil {
		t.Fatalf("disable Persona before rebuild: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_governance_rebuild_l3_persona($1, $2)
`, userID, personaID).Scan(new([]byte)); err != nil {
		t.Fatalf("rebuild disabled Persona: %v", err)
	}
	lease, found, err := claimMemoryL3PersonaJob(
		ctx, tx, workerID, "ac000000-0000-4000-8000-000000000030", true,
	)
	if err != nil || !found || lease.Stage != "refresh" {
		t.Fatalf("claim disabled Persona rebuild = %#v/%t/%v", lease, found, err)
	}
	if _, err := hydrateMemoryL3Persona(ctx, tx, lease, workerID); err != nil {
		t.Fatalf("hydrate disabled Persona rebuild: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"content":         "Disabled Persona remains disabled after background refresh.",
		"memberMemoryIds": []string{globalMemoryA, globalMemoryB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, lease.JobID, workerID, lease.LeaseToken, string(payload)).Scan(new([]byte)); err != nil {
		t.Fatalf("complete disabled Persona rebuild: %v", err)
	}
	var lifecycle string
	var userDisabled bool
	var projectionCount int
	if err := tx.QueryRowContext(ctx, `
SELECT persona.lifecycle_status, persona.user_disabled,
  (SELECT count(*) FROM user_memory_persona_search_projections projection
   WHERE projection.entity_id = persona.id)
FROM user_memory_persona_versions persona
WHERE persona.user_id = $1 AND persona.generation = $2
`, userID, lease.Generation).Scan(
		&lifecycle, &userDisabled, &projectionCount,
	); err != nil || lifecycle != "disabled" || !userDisabled || projectionCount != 0 {
		t.Fatalf("disabled Persona rebuild = lifecycle:%q disabled:%t projection:%d err:%v",
			lifecycle, userDisabled, projectionCount, err)
	}
}

func testMemoryL3AccountCascade(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	const (
		userID         = "1d000000-0000-4000-8000-000000000001"
		conversationID = "3d000000-0000-4000-8000-000000000001"
		sourceID       = "4d000000-0000-4000-8000-000000000001"
		assistantID    = "4d000000-0000-4000-8000-000000000002"
		memoryA        = "5d000000-0000-4000-8000-000000000001"
		memoryB        = "5d000000-0000-4000-8000-000000000002"
		personaID      = "8d000000-0000-4000-8000-000000000001"
		observationID  = "9d000000-0000-4000-8000-000000000001"
	)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR12 Cascade');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled, sensitive_memory_enabled
) VALUES ($1, true, true, false, false)
ON CONFLICT (user_id) DO NOTHING;
INSERT INTO user_memory_state(user_id) VALUES ($1) ON CONFLICT DO NOTHING;
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'Cascade');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed', 'cascade fixture', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role, status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'streaming', '');
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source, scope_type,
  scope_generation, content_hash, authority_kind, importance
) VALUES
  ($5, $1, 'preference', 'Prefers cascade proof', 'prefers cascade proof',
   'manual', 'global', 1,
   encode(sha256(convert_to('Prefers cascade proof', 'UTF8')), 'hex'), 'manual', 5),
  ($6, $1, 'fact', 'Uses cascade fixtures', 'uses cascade fixtures',
   'manual', 'global', 1,
   encode(sha256(convert_to('Uses cascade fixtures', 'UTF8')), 'hex'), 'manual', 4);
`, userID, conversationID, sourceID, assistantID, memoryA, memoryB); err != nil {
		t.Fatalf("seed Persona account cascade: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
WITH authority AS (
  SELECT state.visibility_epoch, state.active_l3_generation generation,
    memory_l3_persona_source_watermark($1) source_watermark
  FROM user_memory_state state WHERE state.user_id = $1
), inserted AS (
  INSERT INTO user_memory_persona_versions(
    id, user_id, content, content_hash, token_count, sensitivity,
    sensitive_input_included, lifecycle_status, profile_id, generation,
    visibility_epoch, source_watermark
  )
  SELECT $2, $1, 'Cascade Persona fixture',
    encode(sha256(convert_to('Cascade Persona fixture', 'UTF8')), 'hex'),
    memory_l3_persona_estimated_tokens('Cascade Persona fixture'),
    'normal', false, 'shadow', 'memory_l3_persona_v1', generation,
    visibility_epoch, source_watermark
  FROM authority
)
INSERT INTO user_memory_persona_members(
  persona_id, memory_id, user_id, memory_revision, memory_content_hash
)
SELECT $2, memory.id, $1, memory.revision, memory.content_hash
FROM user_memories memory WHERE memory.user_id = $1 AND memory.id IN ($3, $4);

WITH persona AS (
  SELECT * FROM user_memory_persona_versions WHERE id = $2 AND user_id = $1
)
INSERT INTO user_memory_persona_search_projections(
  entity_type, entity_id, user_id, entity_revision, sensitivity,
  visibility_epoch, generation, retrieval_profile_id, content_hash,
  source_watermark, exact_terms, bm25_text, lexical_status, embedding_status
)
SELECT 'l3_persona', id, user_id, revision, sensitivity, visibility_epoch,
  generation, 'memory_l3_persona_hybrid_bge_m3_rrf60_v1', content_hash,
  source_watermark,
  knowledge_bm25_shadow_query_terms(content),
  knowledge_build_bm25_shadow_text(
    content, knowledge_bm25_shadow_query_terms(content)
  ), 'ready', 'pending'
FROM persona;

INSERT INTO message_memory_l3_persona_observations(
  id, assistant_message_id, user_id, conversation_id, mode, profile_id,
  retrieval_profile_id, generation, query_sha256, result_sha256,
  status, result_code, query_embedding_status, rerank_status, fallback_code,
  exact_count, rrf_count, final_count
)
SELECT $5, $6, $1, $7, 'shadow', 'memory_l3_persona_v1',
  'memory_l3_persona_hybrid_bge_m3_rrf60_v1', persona.generation,
  repeat('a', 64), repeat('b', 64), 'completed', 'COMPLETED', 'failed',
  'fallback', 'QUERY_EMBEDDING_FAILED', 1, 1, 1
FROM user_memory_persona_versions persona WHERE persona.id = $2;
INSERT INTO message_memory_l3_persona_results(
  observation_id, user_id, lane, ordinal, persona_id, persona_revision
)
SELECT $5, $1, 'final', 1, $2, revision
FROM user_memory_persona_versions WHERE id = $2;
`, userID, personaID, memoryA, memoryB, observationID, assistantID,
		conversationID); err != nil {
		t.Fatalf("seed derived Persona account cascade: %v", err)
	}
	var before int
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memory_persona_versions WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_members WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_search_projections WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_jobs WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_embedding_jobs WHERE user_id = $1)
  + (SELECT count(*) FROM message_memory_l3_persona_observations WHERE user_id = $1)
  + (SELECT count(*) FROM message_memory_l3_persona_results WHERE user_id = $1)
`, userID).Scan(&before); err != nil || before < 7 {
		t.Fatalf("Persona account cascade fixture count = %d/%v", before, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete Persona account cascade fixture: %v", err)
	}
	var after int
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM user_memory_persona_versions WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_members WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_search_projections WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_jobs WHERE user_id = $1)
  + (SELECT count(*) FROM user_memory_persona_embedding_jobs WHERE user_id = $1)
  + (SELECT count(*) FROM message_memory_l3_persona_observations WHERE user_id = $1)
  + (SELECT count(*) FROM message_memory_l3_persona_results WHERE user_id = $1)
`, userID).Scan(&after); err != nil || after != 0 {
		t.Fatalf("Persona account cascade residue = %d/%v", after, err)
	}
}

func testMemoryL3StaleAndPurge(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	memoryID string,
	personaID string,
	originalGeneration int64,
	workerID string,
) {
	t.Helper()
	updatedContent := "Prefers very concise answers"
	if _, err := db.ExecContext(ctx, `
UPDATE user_memories
SET content = $2, normalized_content = lower($2), revision = revision + 1,
    content_hash = encode(sha256(convert_to($2, 'UTF8')), 'hex'),
    updated_at = clock_timestamp()
WHERE id = $1 AND user_id = $3
`, memoryID, updatedContent, userID); err != nil {
		t.Fatalf("update L1 for Persona invalidation: %v", err)
	}
	var lifecycle string
	var purgeAfter time.Time
	var storedGeneration int64
	var projectionCount int
	if err := db.QueryRowContext(ctx, `
SELECT lifecycle_status, purge_after, generation,
  (SELECT count(*) FROM user_memory_persona_search_projections
   WHERE entity_type = 'l3_persona' AND entity_id = $1)
FROM user_memory_persona_versions WHERE id = $1 AND user_id = $2
`, personaID, userID).Scan(
		&lifecycle, &purgeAfter, &storedGeneration, &projectionCount,
	); err != nil || lifecycle != "stale" || purgeAfter.IsZero() ||
		storedGeneration != originalGeneration || projectionCount != 0 {
		t.Fatalf("L1 invalidation = lifecycle:%q purge:%s generation:%d/%d projection:%d err:%v",
			lifecycle, purgeAfter, storedGeneration, originalGeneration, projectionCount, err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_persona_versions
SET stale_at = clock_timestamp() - interval '2 seconds',
    purge_after = clock_timestamp() - interval '1 second'
WHERE id = $1;
UPDATE user_memory_persona_jobs
SET available_at = clock_timestamp() - interval '1 second',
    updated_at = clock_timestamp()
WHERE target_persona_id = $1 AND stage = 'purge' AND status = 'pending';
`, personaID)
	leaseToken := "cc000000-0000-4000-8000-000000000001"
	lease, found, err := claimMemoryL3PersonaJob(ctx, db, workerID, leaseToken, false)
	if err != nil || !found || lease.Stage != "purge" ||
		!lease.TargetPersonaID.Valid || lease.TargetPersonaID.String != personaID {
		t.Fatalf("claim provider-free Persona purge = %#v/%t/%v", lease, found, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_purge($1, $2, $3)
`, lease.JobID, workerID, leaseToken).Scan(new(bool)); err != nil {
		t.Fatalf("complete provider-free Persona purge: %v", err)
	}
	var content string
	var memberCount int
	if err := db.QueryRowContext(ctx, `
SELECT content,
  (SELECT count(*) FROM user_memory_persona_members WHERE persona_id = $1)
FROM user_memory_persona_versions WHERE id = $1
`, personaID).Scan(&content, &memberCount); err != nil || content != "" || memberCount != 0 {
		t.Fatalf("purged Persona = content:%q members:%d err:%v", content, memberCount, err)
	}
}

func testMemoryL3PromotionLifecycle(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	conversationID string,
	sourceID string,
	personaID string,
	tokenCount int,
	query string,
) {
	t.Helper()
	if err := db.QueryRowContext(ctx, `
SELECT memory_operator_promote_l3_persona(gen_random_uuid(), '{}'::jsonb, '{}'::jsonb)
`).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_L3_PERSONA_PROMOTION_EVIDENCE_INVALID") {
		t.Fatalf("malformed L3 promotion evidence error = %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var l2GenerationBefore int64
	if err := tx.QueryRowContext(ctx, `
SELECT active_l2_generation FROM user_memory_state WHERE user_id = $1
`, userID).Scan(&l2GenerationBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_memory_state
SET active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1',
    updated_at = clock_timestamp()
WHERE user_id = $1
`, userID); err != nil {
		t.Fatal(err)
	}
	windowEnd := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	windowStart := windowEnd.Add(-8 * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `
WITH inserted AS (
  INSERT INTO messages(
    id, conversation_id, user_id, parent_message_id, sequence_no,
    role, status, content, created_at, updated_at, completed_at
  )
  SELECT gen_random_uuid(), $1, $2, $3, 1000 + ordinal,
    'assistant', 'completed', 'synthetic Persona canary', sampled_at, sampled_at, sampled_at
  FROM generate_series(1, 100) ordinal
  CROSS JOIN LATERAL (
    SELECT $4::timestamptz + ($5::timestamptz - $4::timestamptz)
      * ((ordinal - 1)::double precision / 99.0) sampled_at
  ) sample
  RETURNING id, completed_at
)
INSERT INTO message_memory_l3_persona_observations(
  id, assistant_message_id, user_id, conversation_id, mode, profile_id,
  retrieval_profile_id, generation, query_sha256, result_sha256,
  status, result_code, query_embedding_status, rerank_status,
  fallback_code, created_at, updated_at
)
SELECT gen_random_uuid(), inserted.id, $2, $1, 'shadow',
  'memory_l3_persona_v1', 'memory_l3_persona_hybrid_bge_m3_rrf60_v1',
  state.active_l3_generation, repeat('a', 64), repeat('b', 64),
  'completed', 'COMPLETED', 'ready', 'applied', 'NONE',
  inserted.completed_at, inserted.completed_at
FROM inserted
JOIN user_memory_state state ON state.user_id = $2
`, conversationID, userID, sourceID, windowStart, windowEnd); err != nil {
		t.Fatalf("insert synthetic Persona canary: %v", err)
	}

	report := map[string]any{
		"schemaVersion": "neo-chat.memory-benchmark-report.v1",
		"passed":        true,
		"golden": map[string]any{
			"totalReviewed": 500, "developmentCount": 300,
			"validationCount": 100, "holdoutCount": 100,
		},
		"profile": map[string]any{
			"profileId": "memory_l3_persona_hybrid_bge_m3_rrf60_v1",
			"metrics": map[string]any{
				"personaConsistency": 0.97, "falseInjectionRate": 0.01,
				"tokenSavingRatio": 0.25,
			},
			"budgets": map[string]any{
				"p95LatencyMilliseconds": 800, "p99LatencyMilliseconds": 1400,
				"maximumPromptMemoryTokens": 300, "hardCutoffViolationCount": 0,
			},
			"safety": map[string]any{
				"crossUserLeakCount": 0, "deletedMemoryLeakCount": 0,
				"secretLeakCount": 0, "untrustedSourceLeakCount": 0,
				"unauthorizedProviderEgressCount": 0,
			},
		},
	}
	benchmarkJSON, err := json.Marshal(map[string]any{
		"report": report, "reportSha256": strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	canaryJSON, err := json.Marshal(map[string]any{
		"crossUserLeakCount": 0, "deletedMemoryLeakCount": 0,
		"eligibleTurns": 100, "falseInjectionRate": 0.01,
		"personaConsistency": 0.97, "reportSha256": strings.Repeat("d", 64),
		"reviewedTurns": 100, "secretLeakCount": 0, "tokenSavingRatio": 0.25,
		"unauthorizedProviderEgressCount": 0,
		"windowEndedAt":                   windowEnd.Format(time.RFC3339Nano),
		"windowStartedAt":                 windowStart.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_operator_promote_l3_persona($1, $2::jsonb, $3::jsonb)
`, "dc000000-0000-4000-8000-000000000001", string(benchmarkJSON),
		string(canaryJSON)).Scan(new([]byte)); err != nil {
		t.Fatalf("synthetic Persona promotion: %v", err)
	}
	var profileStatus, personaStatus string
	if err := tx.QueryRowContext(ctx, `
SELECT profile.lifecycle_status, persona.lifecycle_status
FROM memory_l3_persona_profiles profile
JOIN user_memory_persona_versions persona ON persona.id = $1
WHERE profile.profile_id = 'memory_l3_persona_v1'
`, personaID).Scan(&profileStatus, &personaStatus); err != nil ||
		profileStatus != "active" || personaStatus != "active" {
		t.Fatalf("promoted Persona lifecycle = profile:%q persona:%q err:%v",
			profileStatus, personaStatus, err)
	}

	activeAssistantID := "ec000000-0000-4000-8000-000000000001"
	activeObservationID := "fc000000-0000-4000-8000-000000000001"
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
SELECT candidates FROM memory_prepare_l3_persona_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', true
)
`, activeObservationID, userID, conversationID, activeAssistantID,
		sha256Hex(query), query, memoryHybridVectorLiteral(0)).Scan(&candidatesJSON); err != nil {
		t.Fatalf("active Persona prepare: %v", err)
	}
	var candidates []struct {
		PersonaID string `json:"personaId"`
		Revision  int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil || len(candidates) != 1 {
		t.Fatalf("active Persona candidates = %#v/%v", candidates, err)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"personaId": candidates[0].PersonaID, "revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var injectedCount int
	var finalPersonas []byte
	if err := tx.QueryRowContext(ctx, `
SELECT injected_count, final_personas FROM memory_record_l3_persona_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, $5, 25
)
`, activeObservationID, userID, activeAssistantID, string(finalJSON), tokenCount).Scan(
		&injectedCount, &finalPersonas,
	); err != nil || injectedCount != 1 || !strings.Contains(string(finalPersonas), "content") {
		t.Fatalf("active Persona final = injected:%d personas:%s err:%v",
			injectedCount, finalPersonas, err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_operator_rollback_l3_persona($1, 'SYNTHETIC_TEST')
`, "dc000000-0000-4000-8000-000000000002").Scan(new([]byte)); err != nil {
		t.Fatalf("Persona rollback: %v", err)
	}
	var l2GenerationAfter int64
	if err := tx.QueryRowContext(ctx, `
SELECT profile.lifecycle_status, state.active_l2_generation
FROM memory_l3_persona_profiles profile
CROSS JOIN user_memory_state state
WHERE profile.profile_id = 'memory_l3_persona_v1' AND state.user_id = $1
`, userID).Scan(&profileStatus, &l2GenerationAfter); err != nil ||
		profileStatus != "rolled_back" || l2GenerationAfter != l2GenerationBefore {
		t.Fatalf("rolled-back Persona/L2 independence = profile:%q l2:%d/%d err:%v",
			profileStatus, l2GenerationAfter, l2GenerationBefore, err)
	}
}

type memoryL3PersonaLease struct {
	JobID           string
	Stage           string
	LeaseToken      string
	TargetPersonaID sql.NullString
	Generation      int64
	AttemptCount    int
}

type memoryL3QueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func claimMemoryL3PersonaJob(
	ctx context.Context,
	db memoryL3QueryRower,
	workerID string,
	leaseToken string,
	refreshEnabled bool,
) (memoryL3PersonaLease, bool, error) {
	lease := memoryL3PersonaLease{LeaseToken: leaseToken}
	var userID, profileID string
	var sourceWatermark, providerID, modelID sql.NullString
	var visibilityEpoch int64
	var maxAttempts int
	var providerUpdatedAt sql.NullTime
	var leaseExpiresAt time.Time
	err := db.QueryRowContext(ctx, `
SELECT job_id, stage, user_id, target_persona_id, visibility_epoch, generation,
       profile_id, source_watermark, attempt_count, max_attempts,
       provider_record_id, provider_config_updated_at, model_id, lease_expires_at
FROM memory_worker_claim_l3_persona_job($1, $2, 60, $3)
`, workerID, leaseToken, refreshEnabled).Scan(
		&lease.JobID, &lease.Stage, &userID, &lease.TargetPersonaID,
		&visibilityEpoch, &lease.Generation, &profileID, &sourceWatermark,
		&lease.AttemptCount, &maxAttempts, &providerID, &providerUpdatedAt,
		&modelID, &leaseExpiresAt,
	)
	if err == sql.ErrNoRows {
		return memoryL3PersonaLease{}, false, nil
	}
	return lease, err == nil, err
}

type memoryL3HydratedMemory struct {
	ID string `json:"id"`
}

func hydrateMemoryL3Persona(
	ctx context.Context,
	db memoryL3QueryRower,
	lease memoryL3PersonaLease,
	workerID string,
) ([]memoryL3HydratedMemory, error) {
	var memoriesJSON []byte
	var userID, profileID, sourceWatermark string
	var providerRecordID, providerID, providerLabel sql.NullString
	var encryptedSecretRef, modelID sql.NullString
	var visibilityEpoch, generation int64
	var sensitive bool
	var providerConfig []byte
	var providerUpdatedAt sql.NullTime
	err := db.QueryRowContext(ctx, `
SELECT user_id, visibility_epoch, generation, profile_id, source_watermark,
       sensitive_memory_enabled, memories, provider_record_id, provider_id,
       provider_label, encrypted_secret_ref, provider_config,
       provider_config_updated_at, model_id
FROM memory_worker_hydrate_l3_persona_refresh($1, $2, $3)
`, lease.JobID, workerID, lease.LeaseToken).Scan(
		&userID, &visibilityEpoch, &generation, &profileID, &sourceWatermark,
		&sensitive, &memoriesJSON, &providerRecordID, &providerID, &providerLabel,
		&encryptedSecretRef, &providerConfig, &providerUpdatedAt, &modelID,
	)
	if err != nil {
		return nil, err
	}
	var memories []memoryL3HydratedMemory
	if err := json.Unmarshal(memoriesJSON, &memories); err != nil {
		return nil, err
	}
	return memories, nil
}

type memoryL3EmbeddingLease struct {
	JobID      string
	PersonaID  string
	LeaseToken string
}

func claimMemoryL3PersonaEmbedding(
	ctx context.Context,
	db *sql.DB,
	workerID string,
	leaseToken string,
) (memoryL3EmbeddingLease, bool, error) {
	lease := memoryL3EmbeddingLease{LeaseToken: leaseToken}
	var userID, contentHash, sourceWatermark, profileID, modelID, providerID string
	var personaRevision, visibilityEpoch, generation int64
	var dimensions, attemptCount, maxAttempts int
	var providerUpdatedAt, leaseExpiresAt time.Time
	err := db.QueryRowContext(ctx, `
SELECT job_id, user_id, persona_id, persona_revision, content_hash, source_watermark,
       visibility_epoch, generation, embedding_profile_id, embedding_model_id,
       embedding_dimensions, attempt_count, max_attempts, provider_record_id,
       provider_config_updated_at, lease_expires_at
FROM memory_worker_claim_l3_persona_embedding_job($1, $2, 60)
`, workerID, leaseToken).Scan(
		&lease.JobID, &userID, &lease.PersonaID, &personaRevision, &contentHash,
		&sourceWatermark, &visibilityEpoch, &generation, &profileID, &modelID,
		&dimensions, &attemptCount, &maxAttempts, &providerID,
		&providerUpdatedAt, &leaseExpiresAt,
	)
	if err == sql.ErrNoRows {
		return memoryL3EmbeddingLease{}, false, nil
	}
	return lease, err == nil, err
}

func assertMemoryL3RoleDenied(
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

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
