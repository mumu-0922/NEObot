package migration

import (
	"strings"
	"testing"
)

func TestMemoryL2SceneMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "062_memory_l2_scene.up.sql")
	down := readPhase15SQL(t, "062_memory_l2_scene.down.sql")

	for _, table := range []string{
		"memory_l2_scene_profiles",
		"user_memory_scenes",
		"user_memory_scene_members",
		"user_memory_derived_search_projections",
		"user_memory_scene_jobs",
		"user_memory_derived_embedding_jobs",
		"message_memory_l2_scene_observations",
		"message_memory_l2_scene_results",
		"memory_l2_scene_promotion_events",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("062 is missing %s", table)
		}
	}

	sceneDDL := phase15TableDDL(t, up, "user_memory_scenes")
	assertPhase15Fragments(t, sceneDDL,
		"Scenes must remain same-scope rebuildable derived data",
		"scope_type in ( 'global' , 'project' )",
		"foreign key ( project_id , user_id ) references projects ( id , user_id ) on delete cascade",
		"lifecycle_status in ( 'shadow' , 'active' , 'disabled' , 'stale' )",
		"source_watermark text not null",
		"profile_id text not null references memory_l2_scene_profiles",
		"generation bigint not null",
		"visibility_epoch bigint not null",
		"purge_after timestamptz")
	memberDDL := phase15TableDDL(t, up, "user_memory_scene_members")
	assertPhase15Fragments(t, memberDDL,
		"Scene members must pin current owner, revision, and content hash",
		"primary key ( scene_id , memory_id )",
		"memory_revision bigint not null",
		"memory_content_hash text not null",
		"foreign key ( scene_id , user_id ) references user_memory_scenes ( id , user_id ) on delete cascade",
		"foreign key ( memory_id , user_id ) references user_memories ( id , user_id ) on delete cascade")

	projectionDDL := phase15TableDDL(t, up, "user_memory_derived_search_projections")
	assertPhase15Fragments(t, projectionDDL,
		"derived search must pin the fixed BGE-M3 and Scene authority tuple",
		"entity_type text not null check ( entity_type = 'l2_scene' )",
		"retrieval_profile_id = 'memory_l2_scene_hybrid_bge_m3_rrf60_v1'",
		"embedding_profile_id = 'siliconflow_bge_m3_v1'",
		"embedding_model_id = 'pro/baai/bge-m3'",
		"embedding_dimensions = 1024",
		"embedding_vector vector ( 1024 )",
		"source_watermark text not null")
	assertPhase15Fragments(t, up,
		"derived retrieval must use independent exact, BM25, vector, and deterministic RRF lanes",
		"using gin ( exact_terms )",
		"using bm25 ( bm25_text )",
		"using hnsw ( embedding_vector vector_cosine_ops )",
		"sum ( 1.0 / ( 60.0 + result.ordinal::double precision ) )",
		"lane in ( 'exact' , 'bm25' , 'vector' , 'rrf' , 'rerank' , 'final' )",
		"final_count smallint not null default 0 check ( final_count between 0 and 2 )",
		"estimated_tokens smallint not null default 0 check ( estimated_tokens between 0 and 500 )")

	observationDDL := phase15TableDDL(t, up, "message_memory_l2_scene_observations")
	for _, forbidden := range []string{
		"query text", "content text", "prompt text", "raw_score", "embedding_vector",
		"provider_record_id", "encrypted_secret_ref",
	} {
		if strings.Contains(observationDDL, forbidden) {
			t.Errorf("062 L2 observation persists forbidden payload %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"canonical changes must advance L2 generation, stale the exact scope, and schedule 24-hour purge",
		"create function memory_l2_scene_advance_generation",
		"active_l2_generation = state.active_l2_generation + 1",
		"create function memory_l2_scene_invalidate_scope_at_generation",
		"lifecycle_status = 'stale'",
		"v_now + interval '24 hours'",
		"create trigger user_memories_l2_scene_insert",
		"create trigger user_memories_l2_scene_update",
		"create trigger user_memory_settings_l2_scene_update",
		"create trigger projects_l2_scene_update",
		"create trigger user_memory_state_l2_scene_update")
	assertPhase15Fragments(t, up,
		"refresh completion must reject spoofed batches and preserve disabled Scenes",
		"memory_l2_scene_hydration_required",
		"memory_l2_scene_batch_duplicate",
		"memory_l2_scene_member_invalid",
		"memory_l2_scene_sensitivity_invalid",
		"when scene.user_disabled then 'disabled'",
		"and not scene.user_disabled")
	assertPhase15Fragments(t, up,
		"worker leases and active reads must repeat every current-authority fence",
		"memory_worker_claim_l2_scene_job",
		"memory_worker_hydrate_l2_scene_refresh",
		"memory_worker_complete_l2_scene_refresh",
		"memory_worker_complete_l2_scene_purge",
		"memory_worker_claim_l2_scene_embedding_job",
		"memory_worker_complete_l2_scene_embedding_job",
		"memory.scope_type <> scene.scope_type",
		"memory.project_id is distinct from scene.project_id",
		"memory.scope_generation <> scene.scope_generation",
		"memory.revision <> member.memory_revision",
		"memory.content_hash <> member.memory_content_hash")

	assertPhase15Fragments(t, up,
		"L2 must seed shadow only and promotion must require formal evidence",
		"lifecycle_status text not null default 'shadow'",
		"memory_l2_scene_hybrid_bge_m3_rrf60_v1",
		"array ( select key from jsonb_object_keys ( p_canary ) key order by key ) <> array['crossuserleakcount' , 'currentfactaccuracy'",
		"v_total_reviewed , 0 ) <> 500",
		"v_db_count < v_eligible",
		"v_db_end < v_db_start + interval '7 days'",
		"active_retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'",
		"maximum_prompt_tokens , 999999 ) > 500",
		"memory_l2_scene_promotion_runtime_gate_failed",
		"create trigger memory_l2_scene_promotion_events_append_only")

	assertPhase15Fragments(t, up,
		"application capabilities must be pinned SECURITY DEFINER and least privilege",
		"security definer",
		"alter function %i.%s set search_path to %i , pg_catalog , pg_temp",
		"alter function %i.%s owner to memory_runtime_owner",
		"revoke all on memory_l2_scene_profiles",
		"to memory_worker_runtime",
		"to go_api_runtime",
		"to memory_l2_operator")
	for _, forbidden := range []string{
		"grant select on user_memory_scenes",
		"grant insert on user_memory_scenes",
		"grant update on user_memory_scenes",
		"grant delete on user_memory_scenes",
		"memory_operator_promote_l2_scene ( uuid , jsonb , jsonb ) to go_api_runtime",
		"memory_operator_promote_l2_scene ( uuid , jsonb , jsonb ) to memory_worker_runtime",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("062 grants forbidden L2 authority: %s", forbidden)
		}
	}

	assertPhase15Fragments(t, down,
		"062 rollback must fail closed after promotion, observations, or derived content",
		"memory_l2_scene_rollback_requires_shadow_profile",
		"memory_l2_scene_rollback_requires_no_promotion_history",
		"memory_l2_scene_rollback_requires_empty_observations",
		"memory_l2_scene_rollback_requires_empty_derived_state",
		"drop function memory_operator_promote_l2_scene",
		"drop function memory_worker_claim_l2_scene_job",
		"drop function memory_prepare_l2_scene_search",
		"drop table user_memory_scenes",
		"drop table memory_l2_scene_profiles")
}
