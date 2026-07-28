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

func TestMemoryHybridVectorShadowLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	baseRunner := NewRunner(db, phase15MigrationFSThrough(t, 58))
	if _, err := baseRunner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := baseRunner.Up(ctx); err != nil {
		t.Fatalf("apply through 058: %v", err)
	}

	const (
		userID         = "15000000-0000-4000-8000-000000000001"
		otherUserID    = "15000000-0000-4000-8000-000000000002"
		conversationID = "25000000-0000-4000-8000-000000000001"
		projectID      = "45000000-0000-4000-8000-000000000001"
		otherProjectID = "45000000-0000-4000-8000-000000000002"
		sourceID       = "35000000-0000-4000-8000-000000000001"
		assistantID    = "35000000-0000-4000-8000-000000000002"
		exactMemoryID  = "55000000-0000-4000-8000-000000000001"
		vectorMemoryID = "55000000-0000-4000-8000-000000000002"
		projectMemory  = "55000000-0000-4000-8000-000000000003"
		leaseExpired   = "55000000-0000-4000-8000-000000000004"
		currentProject = "55000000-0000-4000-8000-000000000005"
		currentConv    = "55000000-0000-4000-8000-000000000006"
		foreignProject = "55000000-0000-4000-8000-000000000007"
		sensitiveID    = "55000000-0000-4000-8000-000000000008"
		expiredID      = "55000000-0000-4000-8000-000000000009"
		otherUserMem   = "55000000-0000-4000-8000-000000000010"
		providerDrift  = "55000000-0000-4000-8000-000000000011"
		providerID     = "75000000-0000-4000-8000-000000000001"
		duplicateRAG   = "75000000-0000-4000-8000-000000000002"
		workerID       = "85000000-0000-4000-8000-000000000001"
		observationID  = "65000000-0000-4000-8000-000000000001"
	)
	query := "Please keep concise answers"
	secretRef := "fixture-encrypted-secret-reference"
	attestation := sha256Hex(strings.Join([]string{
		"rag-provider-connection/v1",
		"RAG:SILICONFLOW",
		"siliconflow",
		"https://api.siliconflow.cn/v1/embeddings",
		"Pro/BAAI/bge-m3",
		"1024",
		"https://api.siliconflow.cn/v1/rerank",
		"Pro/BAAI/bge-reranker-v2-m3",
		secretRef,
	}, "\x00"))
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'PR8'), ($2, 'PR8 Other');
INSERT INTO projects(id, user_id, name) VALUES ($3, $1, 'PR8 Project');
INSERT INTO conversations(id, user_id, project_id, title)
VALUES ($4, $1, $3, 'PR8');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($5, $4, $1, 1, 'user', 'completed', $7, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($6, $4, $1, $5, 2, 'assistant', 'streaming', '');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled
) VALUES
  ($1, true, true, false, false),
  ($2, true, true, false, false);
SELECT id FROM memory_upsert_global_manual(
  $8, $1, 'preference', 'Keep concise answers', 'keep concise answers',
  5::smallint, ARRAY['style']::text[], NULL, NULL, true
);
SELECT id FROM memory_upsert_global_manual(
  $9, $1, 'preference', 'Nebula semantic marker', 'nebula semantic marker',
  4::smallint, ARRAY['semantic']::text[], NULL, NULL, true
);
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, project_id, scope_generation, content_hash, authority_kind
) VALUES (
  $10, $1, 'project', 'Project marker ORBIT', 'project marker orbit',
  'manual', 'project', $3, 1,
  encode(sha256(convert_to('Project marker ORBIT', 'UTF8')), 'hex'), 'manual'
);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config,
  created_at, updated_at
) VALUES (
  $11, $1, 'RAG:SILICONFLOW', 'SiliconFlow', $12,
  jsonb_build_object(
    'kind', 'rag', 'ragProvider', 'siliconflow', 'enabled', true,
    'connectionTestedAt', '2026-07-28T00:00:00Z',
    'connectionTestSha256', $13
  ),
  '2026-07-28T00:00:00Z'::timestamptz,
  '2026-07-28T00:00:00Z'::timestamptz
);
`, userID, otherUserID, projectID, conversationID, sourceID, assistantID,
		query, exactMemoryID, vectorMemoryID, projectMemory, providerID,
		secretRef, attestation)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 59))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 59 {
		t.Fatalf("apply 059 over projection fixture = %#v/%v", applied, err)
	}

	var pendingProjectionCount, pendingJobCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE embedding_status = 'pending'),
  (SELECT count(*) FROM user_memory_embedding_jobs WHERE status = 'pending')
FROM user_memory_search_projections
WHERE user_id = $1
`, userID).Scan(&pendingProjectionCount, &pendingJobCount); err != nil ||
		pendingProjectionCount != 3 || pendingJobCount != 3 {
		t.Fatalf("embedding backfill = projections:%d jobs:%d err:%v",
			pendingProjectionCount, pendingJobCount, err)
	}

	jobs := make(map[string]memoryHybridEmbeddingLease)
	for len(jobs) < 3 {
		leaseToken := fmt.Sprintf("95000000-0000-4000-8000-%012d", len(jobs)+1)
		job, found, claimErr := claimMemoryHybridEmbedding(
			ctx,
			db,
			workerID,
			leaseToken,
		)
		if claimErr != nil || !found {
			t.Fatalf("claim embedding %d = %#v/%t/%v", len(jobs), job, found, claimErr)
		}
		jobs[job.MemoryID] = job
	}

	for _, memoryID := range []string{exactMemoryID, vectorMemoryID} {
		job := jobs[memoryID]
		var content, scopeType string
		var visibilityEpoch, scopeGeneration int64
		if err := db.QueryRowContext(ctx, `
SELECT content, scope_type, visibility_epoch, scope_generation
FROM memory_worker_hydrate_embedding_job($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, workerID, job.LeaseToken).Scan(
			&content,
			&scopeType,
			&visibilityEpoch,
			&scopeGeneration,
		); err != nil || content == "" || scopeType != "global" ||
			visibilityEpoch != 1 || scopeGeneration != 1 {
			t.Fatalf("hydrate %s = %q/%q/%d/%d/%v", memoryID, content,
				scopeType, visibilityEpoch, scopeGeneration, err)
		}
		vector := memoryHybridVectorLiteral(0)
		if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, job.JobID, workerID, job.LeaseToken, vector).Scan(new(bool)); err != nil {
			t.Fatalf("complete fake vector %s: %v", memoryID, err)
		}
	}

	var readyCount, completedJobCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE embedding_status = 'ready'),
  (SELECT count(*) FROM user_memory_embedding_jobs WHERE status = 'completed')
FROM user_memory_search_projections WHERE user_id = $1
`, userID).Scan(&readyCount, &completedJobCount); err != nil ||
		readyCount != 2 || completedJobCount != 2 {
		t.Fatalf("fake vector completion = ready:%d completed:%d err:%v",
			readyCount, completedJobCount, err)
	}

	// A scope-generation change removes the old projection/job. The leased old
	// response cannot write into a later authority generation.
	projectJob := jobs[projectMemory]
	mustExecPhase151C(t, ctx, db, `
UPDATE projects SET scope_generation = scope_generation + 1 WHERE id = $1
`, projectID)
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, projectJob.JobID, workerID, projectJob.LeaseToken,
		memoryHybridVectorLiteral(1)).Scan(new(bool)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_EMBEDDING_LEASE_LOST") {
		t.Fatalf("old scope response error = %v", err)
	}

	// Provider configuration is part of the lease authority. A response from
	// before an updated_at change cannot complete, while the retry capability
	// can terminally close that exact live lease without exposing the secret.
	mustExecPhase151C(t, ctx, db, `
SELECT id FROM memory_upsert_global_manual(
  $1, $2, 'fact', 'Provider drift projection probe',
  'provider drift projection probe', 3::smallint,
  ARRAY['provider-drift']::text[], NULL, NULL, true
)
`, providerDrift, userID)
	providerDriftLease, found, err := claimMemoryHybridEmbedding(
		ctx,
		db,
		workerID,
		"95000000-0000-4000-8000-000000000008",
	)
	if err != nil || !found || providerDriftLease.MemoryID != providerDrift {
		t.Fatalf("claim provider-drift probe = %#v/%t/%v",
			providerDriftLease, found, err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE provider_configs
SET label = 'SiliconFlow updated', updated_at = clock_timestamp() + interval '1 second'
WHERE id = $1
`, providerID)
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_complete_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, providerDriftLease.JobID, workerID, providerDriftLease.LeaseToken,
		memoryHybridVectorLiteral(2)).Scan(new(bool)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_EMBEDDING_LEASE_LOST") {
		t.Fatalf("old Provider response error = %v", err)
	}
	var providerDriftStatus string
	if err := db.QueryRowContext(ctx, `
SELECT memory_worker_retry_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, 'EMBEDDING_SOURCE_DRIFT',
  clock_timestamp() + interval '1 minute', true
)
`, providerDriftLease.JobID, workerID, providerDriftLease.LeaseToken).Scan(
		&providerDriftStatus,
	); err != nil || providerDriftStatus != "dead_letter" {
		t.Fatalf("provider-drift retry = %q/%v", providerDriftStatus, err)
	}

	// Exhausting the last lease terminally fails both the job and its matching
	// projection. Leaving the projection pending would create an unreclaimable
	// false backlog entry.
	mustExecPhase151C(t, ctx, db, `
SELECT id FROM memory_upsert_global_manual(
  $1, $2, 'fact', 'Lease expiry projection probe',
  'lease expiry projection probe', 3::smallint,
  ARRAY['lease-expiry']::text[], NULL, NULL, true
)
`, leaseExpired, userID)
	expiredLease, found, err := claimMemoryHybridEmbedding(
		ctx,
		db,
		workerID,
		"95000000-0000-4000-8000-000000000010",
	)
	if err != nil || !found || expiredLease.MemoryID != leaseExpired {
		t.Fatalf("claim lease-expiry probe = %#v/%t/%v", expiredLease, found, err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_embedding_jobs
SET max_attempts = attempt_count, lease_expires_at = clock_timestamp()
WHERE job_id = $1
`, expiredLease.JobID)
	if _, found, err := claimMemoryHybridEmbedding(
		ctx,
		db,
		workerID,
		"95000000-0000-4000-8000-000000000011",
	); err != nil || found {
		t.Fatalf("claim after final lease expiry = found:%t err:%v", found, err)
	}
	var expiredProjectionStatus, expiredProjectionCode string
	var expiredJobStatus, expiredJobCode string
	if err := db.QueryRowContext(ctx, `
SELECT projection.embedding_status, projection.embedding_error_code,
       job.status, job.error_code
FROM user_memory_search_projections projection
JOIN user_memory_embedding_jobs job ON job.memory_id = projection.memory_id
WHERE projection.memory_id = $1
`, leaseExpired).Scan(
		&expiredProjectionStatus,
		&expiredProjectionCode,
		&expiredJobStatus,
		&expiredJobCode,
	); err != nil || expiredProjectionStatus != "failed" ||
		expiredProjectionCode != "LEASE_EXPIRED" ||
		expiredJobStatus != "dead_letter" || expiredJobCode != "LEASE_EXPIRED" {
		t.Fatalf("final lease expiry = projection:%q/%q job:%q/%q err:%v",
			expiredProjectionStatus, expiredProjectionCode,
			expiredJobStatus, expiredJobCode, err)
	}

	// Add matching current Project/Conversation rows plus forbidden scope,
	// Sensitive, expired, and cross-user rows. Every one has a lexical
	// projection, so absence from RRF proves candidate-time authority rather
	// than fixture absence.
	mustExecPhase151C(t, ctx, db, `
INSERT INTO projects(id, user_id, name) VALUES ($1, $2, 'Other Project');
INSERT INTO user_memory_state(user_id) VALUES ($11) ON CONFLICT DO NOTHING;
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, project_id, scope_conversation_id, scope_generation,
  content_hash, authority_kind, sensitivity, expires_at
) VALUES
  ($4, $2, 'project', 'Keep concise answers current project',
    'keep concise answers current project', 'manual', 'project', $3, NULL, 2,
    encode(sha256(convert_to('Keep concise answers current project', 'UTF8')), 'hex'),
    'manual', 'normal', NULL),
  ($5, $2, 'fact', 'Keep concise answers current conversation',
    'keep concise answers current conversation', 'manual', 'conversation', NULL, $6, 1,
    encode(sha256(convert_to('Keep concise answers current conversation', 'UTF8')), 'hex'),
    'manual', 'normal', NULL),
  ($7, $2, 'project', 'Keep concise answers foreign project',
    'keep concise answers foreign project', 'manual', 'project', $1, NULL, 1,
    encode(sha256(convert_to('Keep concise answers foreign project', 'UTF8')), 'hex'),
    'manual', 'normal', NULL),
  ($8, $2, 'fact', 'Keep concise answers sensitive marker',
    'keep concise answers sensitive marker', 'manual', 'global', NULL, NULL, 1,
    encode(sha256(convert_to('Keep concise answers sensitive marker', 'UTF8')), 'hex'),
    'manual', 'sensitive', NULL),
  ($9, $2, 'fact', 'Keep concise answers expired marker',
    'keep concise answers expired marker', 'manual', 'global', NULL, NULL, 1,
    encode(sha256(convert_to('Keep concise answers expired marker', 'UTF8')), 'hex'),
    'manual', 'normal', clock_timestamp() - interval '1 minute'),
  ($10, $11, 'fact', 'Keep concise answers cross user marker',
    'keep concise answers cross user marker', 'manual', 'global', NULL, NULL, 1,
    encode(sha256(convert_to('Keep concise answers cross user marker', 'UTF8')), 'hex'),
    'manual', 'normal', NULL);
`, otherProjectID, userID, projectID, currentProject, currentConv,
		conversationID, foreignProject, sensitiveID, expiredID, otherUserMem,
		otherUserID)

	var status, resultCode, fallbackCode string
	var exactCount, bm25Count, vectorCount, rrfCount int
	var candidateJSON []byte
	queryVector := memoryHybridVectorLiteral(0)
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, fallback_code, exact_count, bm25_count,
       vector_count, rrf_count, candidates
FROM memory_prepare_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6,
  '[]'::jsonb, $7::real[], 'ready'
)
`, observationID, userID, conversationID, assistantID, sha256Hex(query),
		query, queryVector).Scan(
		&status,
		&resultCode,
		&fallbackCode,
		&exactCount,
		&bm25Count,
		&vectorCount,
		&rrfCount,
		&candidateJSON,
	); err != nil {
		t.Fatalf("prepare hybrid shadow: %v", err)
	}
	if status != "pending" || resultCode != "CANDIDATES_READY" ||
		fallbackCode != "NONE" || exactCount < 1 || bm25Count < 1 ||
		vectorCount != 2 || rrfCount < 2 {
		t.Fatalf("hybrid lanes = %q/%q/%q exact:%d bm25:%d vector:%d rrf:%d",
			status, resultCode, fallbackCode, exactCount, bm25Count,
			vectorCount, rrfCount)
	}

	var exactHit, semanticVectorHit int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE lane = 'exact' AND memory_id = $2),
  count(*) FILTER (WHERE lane = 'vector' AND memory_id = $3)
FROM message_memory_hybrid_shadow_results WHERE observation_id = $1
`, observationID, exactMemoryID, vectorMemoryID).Scan(
		&exactHit,
		&semanticVectorHit,
	); err != nil || exactHit != 1 || semanticVectorHit != 1 {
		t.Fatalf("independent lanes = exact:%d semantic-vector:%d err:%v",
			exactHit, semanticVectorHit, err)
	}
	var currentProjectHit, currentConversationHit, forbiddenHit int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE lane = 'rrf' AND memory_id = $2),
  count(*) FILTER (WHERE lane = 'rrf' AND memory_id = $3),
  count(*) FILTER (
    WHERE lane = 'rrf' AND memory_id = ANY($4::uuid[])
  )
FROM message_memory_hybrid_shadow_results WHERE observation_id = $1
`, observationID, currentProject, currentConv,
		fmt.Sprintf("{%s,%s,%s,%s}", foreignProject, sensitiveID, expiredID, otherUserMem),
	).Scan(
		&currentProjectHit,
		&currentConversationHit,
		&forbiddenHit,
	); err != nil || currentProjectHit != 1 || currentConversationHit != 1 ||
		forbiddenHit != 0 {
		t.Fatalf("scope/privacy/time authority = project:%d conversation:%d forbidden:%d err:%v",
			currentProjectHit, currentConversationHit, forbiddenHit, err)
	}

	var candidates []struct {
		MemoryID string `json:"memoryId"`
		Revision int64  `json:"revision"`
		Scope    string `json:"scopeType"`
	}
	if err := json.Unmarshal(candidateJSON, &candidates); err != nil || len(candidates) < 2 {
		t.Fatalf("decode hybrid candidates = %#v/%v raw=%s", candidates, err, candidateJSON)
	}
	finalPayload, err := json.Marshal([]map[string]any{{
		"memoryId":  candidates[0].MemoryID,
		"revision":  candidates[0].Revision,
		"scopeType": candidates[0].Scope,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var finalCount, overlapCount, estimatedTokens int
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code, fallback_code, final_count, overlap_count,
       estimated_tokens
FROM memory_record_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, 'fallback', 'RERANK_FAILED',
  '[]'::jsonb, $4::jsonb, 100, false, 25
)
`, observationID, userID, assistantID, string(finalPayload)).Scan(
		&status,
		&resultCode,
		&fallbackCode,
		&finalCount,
		&overlapCount,
		&estimatedTokens,
	); err != nil || status != "completed" || resultCode != "OK" ||
		fallbackCode != "RERANK_FAILED" || finalCount != 1 ||
		overlapCount != 0 || estimatedTokens != 100 {
		t.Fatalf("record hybrid = %q/%q/%q/%d/%d/%d/%v",
			status, resultCode, fallbackCode, finalCount, overlapCount,
			estimatedTokens, err)
	}

	var replayed bool
	if err := db.QueryRowContext(ctx, `
SELECT replayed FROM memory_prepare_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6,
  '[]'::jsonb, $7::real[], 'ready'
)
`, "65000000-0000-4000-8000-000000000099", userID, conversationID,
		assistantID, sha256Hex(query), query, queryVector).Scan(&replayed); err != nil || !replayed {
		t.Fatalf("prepare replay = %t/%v", replayed, err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT status FROM memory_record_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, 'fallback', 'RERANK_FAILED',
  '[]'::jsonb, '[]'::jsonb, 0, false, 25
)
`, observationID, userID, assistantID).Scan(&status); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT") {
		t.Fatalf("changed result replay error = %v", err)
	}

	// A secret-only query is still hash/source-authorized with its raw text in
	// SQL, but the Provider vector lane is explicitly absent and the durable
	// observation records only a bounded redaction fallback.
	const (
		redactedSource      = "35000000-0000-4000-8000-000000000030"
		redactedAssistant   = "35000000-0000-4000-8000-000000000031"
		redactedObservation = "65000000-0000-4000-8000-000000000030"
	)
	redactedQuery := "password: fixture-secret-value"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, 30, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $3, $1, 31, 'assistant', 'streaming', '');
`, redactedSource, conversationID, userID, redactedAssistant, redactedQuery)
	if err := db.QueryRowContext(ctx, `
SELECT status, fallback_code, vector_count
FROM memory_prepare_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6,
  '[]'::jsonb, NULL::real[], 'redacted'
)
`, redactedObservation, userID, conversationID, redactedAssistant,
		sha256Hex(redactedQuery), redactedQuery).Scan(
		&status,
		&fallbackCode,
		&vectorCount,
	); err != nil || status != "pending" || fallbackCode != "SECRET_REDACTED" ||
		vectorCount != 0 {
		t.Fatalf("redacted query lane = %q/%q/vector:%d/%v",
			status, fallbackCode, vectorCount, err)
	}

	// Canonical authority is revalidated after transient rerank work. A logical
	// delete between prepare and record must produce ID-only RESULT_STALE.
	const (
		staleSource      = "35000000-0000-4000-8000-000000000003"
		staleAssistant   = "35000000-0000-4000-8000-000000000004"
		staleObservation = "65000000-0000-4000-8000-000000000002"
	)
	staleQuery := "Nebula semantic marker"
	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at
) VALUES ($1, $2, $3, 3, 'user', 'completed', $5, now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content
) VALUES ($4, $2, $3, $1, 4, 'assistant', 'streaming', '');
`, staleSource, conversationID, userID, staleAssistant, staleQuery)
	if err := db.QueryRowContext(ctx, `
SELECT candidates FROM memory_prepare_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6,
  '[]'::jsonb, NULL::real[], 'unavailable'
)
`, staleObservation, userID, conversationID, staleAssistant,
		sha256Hex(staleQuery), staleQuery).Scan(&candidateJSON); err != nil {
		t.Fatalf("prepare stale-result probe: %v", err)
	}
	candidates = nil
	if err := json.Unmarshal(candidateJSON, &candidates); err != nil || len(candidates) == 0 {
		t.Fatalf("decode stale-result candidates = %#v/%v raw=%s",
			candidates, err, candidateJSON)
	}
	staleFinal, err := json.Marshal([]map[string]any{{
		"memoryId":  candidates[0].MemoryID,
		"revision":  candidates[0].Revision,
		"scopeType": candidates[0].Scope,
	}})
	if err != nil {
		t.Fatal(err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memories SET deleted_at = now(), enabled = false WHERE id = $1
`, candidates[0].MemoryID)
	if err := db.QueryRowContext(ctx, `
SELECT status, result_code FROM memory_record_hybrid_shadow(
  $1::uuid, $2::uuid, $3::uuid, 'fallback', 'PROVIDER_UNAVAILABLE',
  '[]'::jsonb, $4::jsonb, 50, false, 20
)
`, staleObservation, userID, staleAssistant, string(staleFinal)).Scan(
		&status,
		&resultCode,
	); err != nil || status != "failed" || resultCode != "RESULT_STALE" {
		t.Fatalf("stale record fence = %q/%q/%v", status, resultCode, err)
	}

	// Multiple simultaneously eligible records are ambiguous Provider
	// authority. Claim must fail closed instead of selecting one by row order.
	mustExecPhase151C(t, ctx, db, `
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config,
  created_at, updated_at
) SELECT $1, user_id, provider_id, 'Duplicate SiliconFlow',
         encrypted_secret_ref, config, clock_timestamp(), clock_timestamp()
  FROM provider_configs WHERE id = $2
`, duplicateRAG, providerID)
	if _, found, err := claimMemoryHybridEmbedding(
		ctx,
		db,
		workerID,
		"95000000-0000-4000-8000-000000000012",
	); err != nil || found {
		t.Fatalf("duplicate Provider claim = found:%t err:%v", found, err)
	}

	for _, check := range []struct {
		role, table, privilege string
	}{
		{"go_api_runtime", "user_memory_embedding_jobs", "SELECT"},
		{"go_api_runtime", "message_memory_hybrid_shadow_observations", "INSERT"},
		{"memory_worker_runtime", "user_memory_search_projections", "UPDATE"},
	} {
		var allowed bool
		if err := db.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,$3)`,
			check.role, check.table, check.privilege).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if allowed {
			t.Fatalf("%s unexpectedly has %s on %s",
				check.role, check.privilege, check.table)
		}
	}
	var apiPrepare, workerPrepare, workerClaim, apiClaim bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege(
    'go_api_runtime',
    'memory_prepare_hybrid_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,real[],text)',
    'EXECUTE'
  ),
  has_function_privilege(
    'memory_worker_runtime',
    'memory_prepare_hybrid_shadow(uuid,uuid,uuid,uuid,text,text,jsonb,real[],text)',
    'EXECUTE'
  ),
  has_function_privilege(
    'memory_worker_runtime',
    'memory_worker_claim_embedding_job(uuid,uuid,integer)',
    'EXECUTE'
  ),
  has_function_privilege(
    'go_api_runtime',
    'memory_worker_claim_embedding_job(uuid,uuid,integer)',
    'EXECUTE'
  )
`).Scan(&apiPrepare, &workerPrepare, &workerClaim, &apiClaim); err != nil {
		t.Fatal(err)
	}
	if !apiPrepare || workerPrepare || !workerClaim || apiClaim {
		t.Fatalf("hybrid privileges = apiPrepare:%t workerPrepare:%t workerClaim:%t apiClaim:%t",
			apiPrepare, workerPrepare, workerClaim, apiClaim)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_HYBRID_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS") {
		t.Fatalf("observation guarded down error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `DELETE FROM users WHERE id IN ($1, $2)`,
		userID, otherUserID)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 59 {
		t.Fatalf("clean down = %#v/%v", down, err)
	}
	up, err := runner.Up(ctx)
	if err != nil || len(up) != 1 || up[0].Version != 59 {
		t.Fatalf("re-up = %#v/%v", up, err)
	}
}

type memoryHybridEmbeddingLease struct {
	JobID      string
	MemoryID   string
	LeaseToken string
	Revision   int64
	Epoch      int64
	ScopeType  string
	ScopeGen   int64
}

func claimMemoryHybridEmbedding(
	ctx context.Context,
	db *sql.DB,
	workerID string,
	leaseToken string,
) (memoryHybridEmbeddingLease, bool, error) {
	var job memoryHybridEmbeddingLease
	err := db.QueryRowContext(ctx, `
SELECT job_id::text, memory_id::text, memory_revision, visibility_epoch,
       scope_type, scope_generation
FROM memory_worker_claim_embedding_job($1::uuid, $2::uuid, 60)
`, workerID, leaseToken).Scan(
		&job.JobID,
		&job.MemoryID,
		&job.Revision,
		&job.Epoch,
		&job.ScopeType,
		&job.ScopeGen,
	)
	if err == sql.ErrNoRows {
		return memoryHybridEmbeddingLease{}, false, nil
	}
	job.LeaseToken = leaseToken
	return job, err == nil, err
}

func memoryHybridVectorLiteral(nonZeroIndex int) string {
	var builder strings.Builder
	builder.Grow(4097)
	builder.WriteByte('{')
	for index := 0; index < 1024; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		if index == nonZeroIndex {
			builder.WriteByte('1')
		} else {
			builder.WriteByte('0')
		}
	}
	builder.WriteByte('}')
	return builder.String()
}
