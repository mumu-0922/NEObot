package migration

import (
	"strings"
	"testing"
)

func TestMemoryL3PersonaMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "063_memory_l3_persona.up.sql")
	down := readPhase15SQL(t, "063_memory_l3_persona.down.sql")
	assertPhase15Fragments(t, up,
		"L3 Persona must require the preceding hybrid, governance, and L2 contracts",
		"memory_l3_persona_requires_pr8_pr9_and_pr11",
		"memory_governance_l2_scene_snapshot ( uuid )")

	for _, table := range []string{
		"memory_l3_persona_profiles",
		"user_memory_persona_versions",
		"user_memory_persona_members",
		"user_memory_persona_search_projections",
		"user_memory_persona_jobs",
		"user_memory_persona_embedding_jobs",
		"message_memory_l3_persona_observations",
		"message_memory_l3_persona_results",
		"memory_l3_persona_promotion_events",
	} {
		if _, ok := phase15TableBody(up, table); !ok {
			t.Errorf("063 is missing %s", table)
		}
	}

	personaDDL := phase15TableDDL(t, up, "user_memory_persona_versions")
	assertPhase15Fragments(t, personaDDL,
		"Persona versions must be one Global-only derived version per user generation",
		"unique ( user_id , generation )",
		"token_count smallint not null check ( token_count between 1 and 300 )",
		"sensitive_input_included boolean not null",
		"lifecycle_status in ( 'shadow' , 'active' , 'disabled' , 'stale' )",
		"source_watermark text not null",
		"purge_after timestamptz")
	for _, forbidden := range []string{"scope_type", "project_id", "topic_key"} {
		if strings.Contains(personaDDL, forbidden) {
			t.Errorf("063 Persona version retained L2 field %q", forbidden)
		}
	}

	memberDDL := phase15TableDDL(t, up, "user_memory_persona_members")
	assertPhase15Fragments(t, memberDDL,
		"Persona members must pin current L1 owner, revision, and hash",
		"primary key ( persona_id , memory_id )",
		"memory_revision bigint not null",
		"memory_content_hash text not null",
		"foreign key ( persona_id , user_id ) references user_memory_persona_versions ( id , user_id ) on delete cascade",
		"foreign key ( memory_id , user_id ) references user_memories ( id , user_id ) on delete cascade")

	projectionDDL := phase15TableDDL(t, up, "user_memory_persona_search_projections")
	assertPhase15Fragments(t, projectionDDL,
		"Persona projection must be independent and pin the fixed retrieval tuple",
		"entity_type text not null check ( entity_type = 'l3_persona' )",
		"retrieval_profile_id = 'memory_l3_persona_hybrid_bge_m3_rrf60_v1'",
		"embedding_profile_id = 'siliconflow_bge_m3_v1'",
		"embedding_model_id = 'pro/baai/bge-m3'",
		"embedding_dimensions = 1024",
		"embedding_vector vector ( 1024 )")
	for _, forbidden := range []string{"scope_type", "project_id", "scope_generation"} {
		if strings.Contains(projectionDDL, forbidden) {
			t.Errorf("063 Persona projection retained L2 scope field %q", forbidden)
		}
	}
	assertPhase15Fragments(t, up,
		"Persona source authority must accept only stable current Global L1",
		"memory.scope_type = 'global'",
		"memory.project_id is null",
		"memory.scope_conversation_id is null",
		"memory.scope_generation = 1",
		"memory.memory_type in ( 'fact' , 'preference' , 'instruction' , 'warning' , 'decision' )",
		"memory_governance_classify_sensitivity ( memory.content ) <> 'secret'",
		"tg_op = 'delete' and not exists",
		"select 1 from users where id = old.user_id",
		"create trigger user_memories_l3_persona_delete")
	assertPhase15Fragments(t, up,
		"Provider proposals must contain only content and authoritative member IDs",
		"array ( select key from jsonb_object_keys ( p_persona ) key order by key ) <> array['content' , 'membermemoryids']::text[]",
		"cardinality ( v_member_ids ) not between 2 and 50",
		"not v_job.input_memory_ids @> v_member_ids",
		"memory_l3_persona_estimated_tokens ( v_content )",
		"memory_l3_persona_token_budget_exceeded",
		"memory_l3_persona_secret_rejected")
	assertPhase15Fragments(t, up,
		"generation invalidation must stale old versions without rewriting history",
		"active_l3_generation = state.active_l3_generation + 1",
		"create function memory_l3_persona_invalidate_user_at_generation",
		"lifecycle_status = 'stale'",
		"v_now + interval '24 hours'",
		"perform memory_l3_persona_enqueue_user ( p_user_id )")
	if strings.Contains(up, "set generation = p_generation") ||
		strings.Contains(up, "set generation = v_generation") {
		t.Error("063 rewrites the generation of an existing Persona version")
	}

	assertPhase15Fragments(t, up,
		"Persona retrieval must use independent Exact/BM25/BGE-M3/RRF(60) with 5/1/300 bounds",
		"using gin ( exact_terms )",
		"using bm25 ( bm25_text )",
		"using hnsw ( embedding_vector vector_cosine_ops )",
		"sum ( 1.0 / ( 60.0 + result.ordinal::double precision ) )",
		"rrf_count smallint not null default 0 check ( rrf_count between 0 and 5 )",
		"final_count smallint not null default 0 check ( final_count between 0 and 1 )",
		"estimated_tokens smallint not null default 0 check ( estimated_tokens between 0 and 300 )",
		"v_recomputed_tokens <> p_estimated_tokens")

	observationDDL := phase15TableDDL(t, up, "message_memory_l3_persona_observations")
	for _, forbidden := range []string{
		"query text", "content text", "prompt text", "raw_score", "embedding_vector",
		"provider_record_id", "encrypted_secret_ref",
	} {
		if strings.Contains(observationDDL, forbidden) {
			t.Errorf("063 L3 observation persists forbidden payload %q", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"promotion must be independent, evidence-gated, and owner-only",
		"lifecycle_status text not null default 'shadow'",
		"persona_consistency >= 0.95",
		"false_injection_rate <= 0.02",
		"token_saving_ratio >= 0.20",
		"v_total_reviewed , 0 ) <> 500",
		"v_db_count < v_eligible",
		"v_db_end < v_db_start + interval '7 days'",
		"maximum_prompt_tokens , 999999 ) > 300",
		"memory_l3_persona_promotion_runtime_gate_failed",
		"create trigger memory_l3_persona_promotion_events_append_only")
	for _, forbidden := range []string{
		"to memory_l3_operator",
		"memory_operator_promote_l3_persona ( uuid , jsonb , jsonb ) to go_api_runtime",
		"memory_operator_promote_l3_persona ( uuid , jsonb , jsonb ) to memory_worker_runtime",
		"grant select on user_memory_persona_versions",
		"grant insert on user_memory_persona_versions",
		"grant update on user_memory_persona_versions",
		"grant delete on user_memory_persona_versions",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("063 grants forbidden L3 authority: %s", forbidden)
		}
	}

	assertPhase15Fragments(t, up,
		"application functions must be pinned SECURITY DEFINER and least privilege",
		"security definer",
		"alter function %i.%s set search_path to %i , pg_catalog , pg_temp",
		"alter function %i.%s owner to memory_runtime_owner",
		"revoke all on memory_l3_persona_profiles",
		"to memory_worker_runtime",
		"to go_api_runtime")
	assertPhase15Fragments(t, down,
		"063 rollback must fail closed after promotion, observations, or derived content",
		"memory_l3_persona_rollback_requires_shadow_profile",
		"memory_l3_persona_rollback_requires_no_promotion_history",
		"memory_l3_persona_rollback_requires_empty_observations",
		"memory_l3_persona_rollback_requires_empty_derived_state",
		"drop trigger user_memory_persona_search_embedding_queue",
		"drop function memory_operator_promote_l3_persona",
		"drop function memory_worker_claim_l3_persona_job",
		"drop function memory_prepare_l3_persona_search",
		"drop table user_memory_persona_versions",
		"drop table memory_l3_persona_profiles")
}
