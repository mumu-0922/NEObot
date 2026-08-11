package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const memorySingleUserPreviewApproval = "I_ACCEPT_SINGLE_USER_L2_L3_READER_PREVIEW_WITHOUT_FORMAL_PROMOTION"

func TestMemorySingleUserDerivedReaderPreviewLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 61))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 061: %v", err)
	}

	const (
		userID         = "1d000000-0000-4000-8000-000000000001"
		otherUserID    = "1d000000-0000-4000-8000-000000000002"
		conversationID = "3d000000-0000-4000-8000-000000000001"
		sourceID       = "4d000000-0000-4000-8000-000000000001"
		assistantID    = "4d000000-0000-4000-8000-000000000002"
		memoryA        = "5d000000-0000-4000-8000-000000000001"
		memoryB        = "5d000000-0000-4000-8000-000000000002"
		synthesisID    = "6d000000-0000-4000-8000-000000000001"
		ragProviderID  = "6d000000-0000-4000-8000-000000000002"
		workerID       = "7d000000-0000-4000-8000-000000000001"
		sceneID        = "8d000000-0000-4000-8000-000000000001"
		l2Observation  = "9d000000-0000-4000-8000-000000000001"
		l3Observation  = "9d000000-0000-4000-8000-000000000002"
		enableEvent    = "ed000000-0000-4000-8000-000000000001"
		reenableEvent  = "ed000000-0000-4000-8000-000000000002"
		disableEvent   = "ed000000-0000-4000-8000-000000000003"
	)
	query := "concise Go answers"
	ragSecretRef := "fixture-preview-rag-encrypted-secret-reference"
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
INSERT INTO users(id, display_name) VALUES ($1, 'Preview Owner');
INSERT INTO conversations(id, user_id, title) VALUES ($2, $1, 'Preview');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES ($3, $2, $1, 1, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role, status, content
) VALUES ($4, $2, $1, $3, 2, 'assistant', 'streaming', '');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled, sensitive_memory_enabled
) VALUES ($1, true, true, false, false);
INSERT INTO user_memory_state(user_id) VALUES ($1) ON CONFLICT DO NOTHING;
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source, scope_type,
  scope_generation, content_hash, authority_kind, importance
) VALUES
  ($6, $1, 'preference', 'Prefers concise answers', 'prefers concise answers',
   'manual', 'global', 1,
   encode(sha256(convert_to('Prefers concise answers', 'UTF8')), 'hex'), 'manual', 5),
  ($7, $1, 'fact', 'Uses Go for backend services', 'uses go for backend services',
   'manual', 'global', 1,
   encode(sha256(convert_to('Uses Go for backend services', 'UTF8')), 'hex'), 'manual', 4);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config, created_at, updated_at
) VALUES
  ($8, $1, 'CUSTOM', 'Fixture synthesis', 'fixture-synthesis-secret',
   '{"enabled":true}'::jsonb, '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z'),
  ($9, $1, 'RAG:SILICONFLOW', 'Fixture RAG', $10,
   jsonb_build_object(
     'kind', 'rag', 'ragProvider', 'siliconflow', 'enabled', true,
     'connectionTestedAt', '2026-08-01T00:00:00Z',
     'connectionTestSha256', $11
   ), '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z');
INSERT INTO task_model_settings(user_id, memory) VALUES ($1, 'CUSTOM:preview-model');
`, userID, conversationID, sourceID, assistantID, query, memoryA, memoryB,
		synthesisID, ragProviderID, ragSecretRef, ragAttestation)

	head072 := NewRunner(db, phase15MigrationFSThrough(t, 72))
	if _, err := head072.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := head072.Up(ctx); err != nil {
		t.Fatalf("apply 062 through 072: %v", err)
	}
	personaID := buildMemorySingleUserPreviewArtifacts(
		t, ctx, db, userID, workerID, sceneID,
	)

	head073 := NewRunner(db, phase15MigrationFSThrough(t, 73))
	if _, err := head073.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := head073.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 73 {
		t.Fatalf("apply 073 = %#v/%v", applied, err)
	}
	down, err := head073.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 73 {
		t.Fatalf("clean 073 down = %#v/%v", down, err)
	}
	reapplied, err := head073.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 73 {
		t.Fatalf("clean 072 -> 073 replay = %#v/%v", reapplied, err)
	}

	if err := db.QueryRowContext(ctx, `
SELECT memory_operator_set_single_user_derived_reader_preview(
  $1, $2, true, 'WRONG_APPROVAL'
)
`, enableEvent, userID).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_AUTHORITY_INVALID") {
		t.Fatalf("wrong preview approval error = %v", err)
	}
	assertMemoryL2RoleDenied(t, ctx, db, "go_api_runtime",
		`SELECT count(*) FROM memory_single_user_derived_reader_events`)
	assertMemoryL2RoleDenied(t, ctx, db, "memory_worker_runtime",
		`SELECT memory_operator_set_single_user_derived_reader_preview(
          'ed000000-0000-4000-8000-000000000009',
          '1d000000-0000-4000-8000-000000000001', true,
          'I_ACCEPT_SINGLE_USER_L2_L3_READER_PREVIEW_WITHOUT_FORMAL_PROMOTION'
        )`)

	enableResult := setMemorySingleUserPreview(
		t, ctx, db, enableEvent, userID, true, memorySingleUserPreviewApproval,
	)
	if enableResult.Replayed || !enableResult.Enabled {
		t.Fatalf("preview enable result = %#v", enableResult)
	}
	assertMemorySingleUserPreviewState(
		t, ctx, db, userID, "active", true, 1, 0,
	)
	testMemorySingleUserPreviewL2ActiveSearch(
		t, ctx, db, userID, conversationID, assistantID,
		l2Observation, query, sceneID,
	)
	testMemorySingleUserPreviewL3ActiveSearch(
		t, ctx, db, userID, conversationID, assistantID,
		l3Observation, query, personaID,
	)
	if replay := setMemorySingleUserPreview(
		t, ctx, db, enableEvent, userID, true, memorySingleUserPreviewApproval,
	); !replay.Replayed || !replay.Enabled {
		t.Fatalf("preview enable replay = %#v", replay)
	}

	mustExecPhase151C(t, ctx, db,
		`INSERT INTO users(id, display_name) VALUES ($1, 'Second User')`, otherUserID)
	assertMemorySingleUserPreviewState(
		t, ctx, db, userID, "shadow", false, 2, 0,
	)
	var latestReason string
	if err := db.QueryRowContext(ctx, `
SELECT reason_code FROM memory_single_user_derived_reader_events
WHERE user_id = $1 ORDER BY created_at DESC, event_id DESC LIMIT 1
`, userID).Scan(&latestReason); err != nil || latestReason != "USER_POPULATION_CHANGED" {
		t.Fatalf("automatic preview disable reason = %q/%v", latestReason, err)
	}
	mustExecPhase151C(t, ctx, db, `DELETE FROM users WHERE id = $1`, otherUserID)
	assertMemorySingleUserPreviewState(
		t, ctx, db, userID, "shadow", false, 2, 0,
	)

	setMemorySingleUserPreview(
		t, ctx, db, reenableEvent, userID, true, memorySingleUserPreviewApproval,
	)
	disableResult := setMemorySingleUserPreview(
		t, ctx, db, disableEvent, userID, false, "OWNER_DISABLED_PREVIEW",
	)
	if disableResult.Replayed || disableResult.Enabled {
		t.Fatalf("preview disable result = %#v", disableResult)
	}
	assertMemorySingleUserPreviewState(
		t, ctx, db, userID, "shadow", false, 4, 0,
	)
	if replay := setMemorySingleUserPreview(
		t, ctx, db, disableEvent, userID, false, "OWNER_DISABLED_PREVIEW",
	); !replay.Replayed || replay.Enabled {
		t.Fatalf("preview disable replay = %#v", replay)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_operator_set_single_user_derived_reader_preview(
  $1, $2, true, $3
)
`, disableEvent, userID, memorySingleUserPreviewApproval).Scan(new([]byte)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_REPLAY_CONFLICT") {
		t.Fatalf("preview replay conflict error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
DELETE FROM memory_single_user_derived_reader_events WHERE event_id = $1
`, disableEvent); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_SINGLE_USER_DERIVED_READER_EVENTS_APPEND_ONLY") {
		t.Fatalf("direct preview event delete error = %v", err)
	}

	// Account erasure may cascade per-user preview audit, but rolling this
	// transaction back proves normal history remains append-only afterward.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("preview account cascade: %v", err)
	}
	var eventCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM memory_single_user_derived_reader_events`,
	).Scan(&eventCount); err != nil || eventCount != 0 {
		t.Fatalf("preview account cascade events = %d/%v", eventCount, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	if _, err := head073.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_SINGLE_USER_DERIVED_READER_PREVIEW_ROLLBACK_REQUIRES_EMPTY_EVENTS") {
		t.Fatalf("guarded 073 down error = %v", err)
	}
}

type memorySingleUserPreviewResult struct {
	Enabled  bool `json:"enabled"`
	Replayed bool `json:"replayed"`
}

func setMemorySingleUserPreview(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	eventID string,
	userID string,
	enabled bool,
	authority string,
) memorySingleUserPreviewResult {
	t.Helper()
	var raw []byte
	if err := db.QueryRowContext(ctx, `
SELECT memory_operator_set_single_user_derived_reader_preview($1, $2, $3, $4)
`, eventID, userID, enabled, authority).Scan(&raw); err != nil {
		t.Fatalf("set single-user preview enabled=%t: %v", enabled, err)
	}
	var result memorySingleUserPreviewResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode single-user preview result %s: %v", raw, err)
	}
	return result
}

func buildMemorySingleUserPreviewArtifacts(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	workerID string,
	sceneID string,
) string {
	t.Helper()
	l2Lease, found, err := claimMemoryL2SceneJob(
		ctx, db, workerID, "ad000000-0000-4000-8000-000000000001", true,
	)
	if err != nil || !found || l2Lease.Stage != "refresh" {
		t.Fatalf("claim preview L2 refresh = %#v/%t/%v", l2Lease, found, err)
	}
	l2Memories, err := hydrateMemoryL2Scene(t, ctx, db, l2Lease, workerID)
	if err != nil || len(l2Memories) != 2 {
		t.Fatalf("hydrate preview L2 = %#v/%v", l2Memories, err)
	}
	sceneContent := "Prefers concise Go answers for backend services."
	scenePayload, err := json.Marshal([]map[string]any{{
		"sceneId":         sceneID,
		"topicKey":        "global.answer-style",
		"content":         sceneContent,
		"contentHash":     sha256Hex(sceneContent),
		"sensitivity":     "normal",
		"memberMemoryIds": []string{l2Memories[0].ID, l2Memories[1].ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_refresh($1, $2, $3, $4::jsonb)
`, l2Lease.JobID, workerID, l2Lease.LeaseToken, string(scenePayload)).Scan(new([]byte)); err != nil {
		t.Fatalf("complete preview L2 refresh: %v", err)
	}
	l2Embedding, found, err := claimMemoryL2Embedding(
		ctx, db, workerID, "bd000000-0000-4000-8000-000000000001",
	)
	if err != nil || !found || l2Embedding.SceneID != sceneID {
		t.Fatalf("claim preview L2 embedding = %#v/%t/%v", l2Embedding, found, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT content FROM memory_worker_hydrate_l2_scene_embedding_job($1, $2, $3)
`, l2Embedding.JobID, workerID, l2Embedding.LeaseToken).Scan(new(string)); err != nil {
		t.Fatalf("hydrate preview L2 embedding: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_embedding_job($1, $2, $3, $4::real[])
`, l2Embedding.JobID, workerID, l2Embedding.LeaseToken,
		memoryHybridVectorLiteral(0)).Scan(new(bool)); err != nil {
		t.Fatalf("complete preview L2 embedding: %v", err)
	}

	l3Lease, found, err := claimMemoryL3PersonaJob(
		ctx, db, workerID, "ad000000-0000-4000-8000-000000000002", true,
	)
	if err != nil || !found || l3Lease.Stage != "refresh" {
		t.Fatalf("claim preview L3 refresh = %#v/%t/%v", l3Lease, found, err)
	}
	l3Memories, err := hydrateMemoryL3Persona(ctx, db, l3Lease, workerID)
	if err != nil || len(l3Memories) != 2 {
		t.Fatalf("hydrate preview L3 = %#v/%v", l3Memories, err)
	}
	personaContent := "The user prefers concise answers and builds backend services in Go."
	personaPayload, err := json.Marshal(map[string]any{
		"content": personaContent,
		"memberMemoryIds": []string{
			l3Memories[0].ID, l3Memories[1].ID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh($1, $2, $3, $4::jsonb)
`, l3Lease.JobID, workerID, l3Lease.LeaseToken, string(personaPayload)).Scan(new([]byte)); err != nil {
		t.Fatalf("complete preview L3 refresh: %v", err)
	}
	l3Embedding, found, err := claimMemoryL3PersonaEmbedding(
		ctx, db, workerID, "bd000000-0000-4000-8000-000000000002",
	)
	if err != nil || !found {
		t.Fatalf("claim preview L3 embedding = %#v/%t/%v", l3Embedding, found, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT content FROM memory_worker_hydrate_l3_persona_embedding_job($1, $2, $3)
`, l3Embedding.JobID, workerID, l3Embedding.LeaseToken).Scan(new(string)); err != nil {
		t.Fatalf("hydrate preview L3 embedding: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_embedding_job($1, $2, $3, $4::real[])
`, l3Embedding.JobID, workerID, l3Embedding.LeaseToken,
		memoryHybridVectorLiteral(0)).Scan(new(bool)); err != nil {
		t.Fatalf("complete preview L3 embedding: %v", err)
	}
	return l3Embedding.PersonaID
}

func assertMemorySingleUserPreviewState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	wantLifecycle string,
	wantEnabled bool,
	wantEvents int,
	wantFormalPromotions int,
) {
	t.Helper()
	var previewEnabled, pointerIsNull, l2SnapshotActive, l3SnapshotActive bool
	var sceneLifecycle, personaLifecycle, l2Status, l3Status string
	var events, formalPromotions int
	if err := db.QueryRowContext(ctx, `
SELECT memory_single_user_derived_reader_preview_enabled($1),
       state.active_retrieval_profile_id IS NULL,
       scene.lifecycle_status,
       persona.lifecycle_status,
       memory_governance_l2_scene_snapshot($1)->'profile'->>'status',
       (memory_governance_l2_scene_snapshot($1)->'profile'->>'active')::boolean,
       memory_governance_l3_persona_snapshot($1)->'profile'->>'status',
       (memory_governance_l3_persona_snapshot($1)->'profile'->>'active')::boolean,
       (SELECT count(*) FROM memory_single_user_derived_reader_events),
       (SELECT count(*) FROM memory_l2_scene_promotion_events) +
         (SELECT count(*) FROM memory_l3_persona_promotion_events)
FROM user_memory_state state
JOIN user_memory_scenes scene ON scene.user_id = state.user_id AND scene.deleted_at IS NULL
JOIN user_memory_persona_versions persona
  ON persona.user_id = state.user_id AND persona.deleted_at IS NULL
WHERE state.user_id = $1
`, userID).Scan(
		&previewEnabled, &pointerIsNull, &sceneLifecycle, &personaLifecycle,
		&l2Status, &l2SnapshotActive, &l3Status, &l3SnapshotActive,
		&events, &formalPromotions,
	); err != nil {
		t.Fatalf("read single-user preview state: %v", err)
	}
	if previewEnabled != wantEnabled || !pointerIsNull ||
		sceneLifecycle != wantLifecycle || personaLifecycle != wantLifecycle ||
		events != wantEvents || formalPromotions != wantFormalPromotions {
		t.Fatalf("preview state = enabled:%t pointerNull:%t scene:%s persona:%s events:%d promotions:%d",
			previewEnabled, pointerIsNull, sceneLifecycle, personaLifecycle,
			events, formalPromotions)
	}
	wantStatus := "shadow"
	if wantEnabled {
		wantStatus = "active"
	}
	if l2Status != wantStatus || l3Status != wantStatus ||
		l2SnapshotActive != wantEnabled || l3SnapshotActive != wantEnabled {
		t.Fatalf("preview snapshots = L2:%s/%t L3:%s/%t want:%s/%t",
			l2Status, l2SnapshotActive, l3Status, l3SnapshotActive,
			wantStatus, wantEnabled)
	}
}

func testMemorySingleUserPreviewL2ActiveSearch(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	conversationID string,
	assistantID string,
	observationID string,
	query string,
	sceneID string,
) {
	t.Helper()
	var status, resultCode string
	var candidatesJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, candidates FROM memory_prepare_l2_scene_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', true
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query), query,
		memoryHybridVectorLiteral(0)).Scan(&status, &resultCode, &candidatesJSON); err != nil ||
		status != "pending" || resultCode != "CANDIDATES_READY" {
		t.Fatalf("active preview L2 prepare = %q/%q/%v", status, resultCode, err)
	}
	var candidates []struct {
		SceneID  string `json:"sceneId"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil ||
		len(candidates) == 0 || candidates[0].SceneID != sceneID {
		t.Fatalf("active preview L2 candidates = %#v/%v raw=%s", candidates, err, candidatesJSON)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"sceneId": candidates[0].SceneID, "revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var injected int
	var finalScenes []byte
	if err := db.QueryRowContext(ctx, `
SELECT injected_count, final_scenes FROM memory_record_l2_scene_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, 20, 25
)
`, observationID, userID, assistantID, string(finalJSON)).Scan(
		&injected, &finalScenes,
	); err != nil || injected != 1 || !strings.Contains(string(finalScenes), "content") {
		t.Fatalf("active preview L2 record = injected:%d scenes:%s/%v",
			injected, finalScenes, err)
	}
}

func testMemorySingleUserPreviewL3ActiveSearch(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	conversationID string,
	assistantID string,
	observationID string,
	query string,
	personaID string,
) {
	t.Helper()
	var status, resultCode string
	var candidatesJSON []byte
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, candidates FROM memory_prepare_l3_persona_search(
  $1, $2, $3, $4, $5, $6, $7::real[], 'ready', true
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query), query,
		memoryHybridVectorLiteral(0)).Scan(&status, &resultCode, &candidatesJSON); err != nil ||
		status != "pending" || resultCode != "CANDIDATES_READY" {
		t.Fatalf("active preview L3 prepare = %q/%q/%v", status, resultCode, err)
	}
	var candidates []struct {
		PersonaID string `json:"personaId"`
		Revision  int64  `json:"revision"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil ||
		len(candidates) == 0 || candidates[0].PersonaID != personaID {
		t.Fatalf("active preview L3 candidates = %#v/%v raw=%s", candidates, err, candidatesJSON)
	}
	finalJSON, err := json.Marshal([]map[string]any{{
		"personaId": candidates[0].PersonaID, "revision": candidates[0].Revision,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var tokenCount int
	if err := db.QueryRowContext(ctx,
		`SELECT token_count FROM user_memory_persona_versions WHERE id = $1`, personaID,
	).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	var injected int
	var finalPersonas []byte
	if err := db.QueryRowContext(ctx, `
SELECT injected_count, final_personas FROM memory_record_l3_persona_search(
  $1, $2, $3, 'fallback', 'RERANK_FAILED', '[]'::jsonb, $4::jsonb, $5, 25
)
`, observationID, userID, assistantID, string(finalJSON), tokenCount).Scan(
		&injected, &finalPersonas,
	); err != nil || injected != 1 || !strings.Contains(string(finalPersonas), "content") {
		t.Fatalf("active preview L3 record = injected:%d personas:%s/%v",
			injected, finalPersonas, err)
	}
}
