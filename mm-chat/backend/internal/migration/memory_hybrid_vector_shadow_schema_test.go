package migration

import (
	"strings"
	"testing"
)

func TestMemoryHybridVectorShadowMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "059_memory_hybrid_vector_shadow.up.sql")
	down := readPhase15SQL(t, "059_memory_hybrid_vector_shadow.down.sql")

	projectionDDL := phase15AlterTableDDL(up, "user_memory_search_projections")
	assertPhase15Fragments(t, projectionDDL,
		"059 projection must pin the fixed BGE-M3 vector space",
		"embedding_profile_id text not null default 'siliconflow_bge_m3_v1'",
		"embedding_model_id text not null default 'pro/baai/bge-m3'",
		"embedding_dimensions smallint not null default 1024",
		"embedding_vector vector ( 1024 )",
		"embedding_status in ( 'pending' , 'ready' , 'failed' )")
	assertPhase15Fragments(t, up,
		"059 must add a partial HNSW cosine index",
		"using hnsw ( embedding_vector vector_cosine_ops )",
		"where embedding_status = 'ready'")

	for _, table := range []string{
		"user_memory_embedding_jobs",
		"message_memory_hybrid_shadow_observations",
		"message_memory_hybrid_shadow_results",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("059 is missing %s", table)
		}
	}

	jobDDL := phase15TableDDL(t, up, "user_memory_embedding_jobs")
	assertPhase15Fragments(t, jobDDL,
		"embedding jobs must pin every old-response authority fence",
		"projection_generation bigint not null",
		"memory_revision bigint not null",
		"content_hash text not null",
		"visibility_epoch bigint not null",
		"scope_type text not null",
		"scope_generation bigint not null",
		"provider_record_id uuid",
		"provider_config_updated_at timestamptz",
		"foreign key ( memory_id , user_id )")
	for _, forbidden := range []string{
		"content text", "query_text", "embedding_vector", "encrypted_secret_ref",
	} {
		if strings.Contains(jobDDL, forbidden) {
			t.Errorf("embedding job contains forbidden durable payload %q", forbidden)
		}
	}

	for _, table := range []string{
		"message_memory_hybrid_shadow_observations",
		"message_memory_hybrid_shadow_results",
	} {
		body := phase15TableDDL(t, up, table)
		for _, forbidden := range []string{
			"query_text", "memory_content", "content text", "prompt",
			"embedding_vector", "raw_score", "rerank_score", "rrf_score",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains forbidden durable payload %q", table, forbidden)
			}
		}
	}

	assertPhase15Fragments(t, up,
		"embedding claim/hydrate/complete must revalidate lease and authority",
		"memory_worker_claim_embedding_job",
		"memory_worker_hydrate_embedding_job",
		"memory_worker_complete_embedding_job",
		"memory_worker_retry_embedding_job",
		"projection.visibility_epoch = candidate.visibility_epoch",
		"projection.scope_generation = candidate.scope_generation",
		"provider.updated_at = job.provider_config_updated_at",
		"job.lease_expires_at > v_now",
		"with expired_jobs as materialized",
		"embedding_error_code = 'lease_expired'",
		"memory_embedding_source_drift",
		"memory_embedding_lease_lost")
	assertPhase15Fragments(t, up,
		"hybrid retrieval must keep independent lanes and deterministic RRF(60)",
		"lane in ( 'v1' , 'exact' , 'bm25' , 'vector' , 'rrf' , 'rerank' , 'final' )",
		"lane_rank <= 20",
		"limit 30",
		"1.0 / ( 60.0 + result.ordinal::double precision )",
		"order by score.rrf_score desc",
		"projection.embedding_vector <=> p_query_embedding::vector ( 1024 )")
	assertPhase15Fragments(t, up,
		"secret-only Provider input must be represented without persisting plaintext",
		"'ready' , 'failed' , 'unavailable' , 'cutoff' , 'redacted'",
		"else 'secret_redacted'")
	assertPhase15Fragments(t, up,
		"hybrid record must enforce replay, current authority, and budgets",
		"memory_hybrid_shadow_replay_conflict",
		"state.active_projection_generation = v_existing.projection_generation",
		"memory.revision = projection.memory_revision",
		"memory.content_hash = projection.content_hash",
		"p_target_tokens_exceeded <> ( p_estimated_tokens > 600 )",
		"p_estimated_tokens not between 0 and 900",
		"result_stale")
	assertPhase15Fragments(t, up,
		"runtime roles must receive only narrow capabilities",
		"revoke all on user_memory_embedding_jobs",
		"from public , go_api_runtime , memory_worker_runtime",
		"grant execute on function memory_worker_claim_embedding_job",
		"to memory_worker_runtime",
		"grant execute on function memory_prepare_hybrid_shadow",
		"to go_api_runtime")
	assertPhase15Fragments(t, down,
		"059 rollback must preserve reader and observation authority",
		"memory_hybrid_rollback_requires_v1_reader",
		"memory_hybrid_rollback_requires_empty_observations")
}
