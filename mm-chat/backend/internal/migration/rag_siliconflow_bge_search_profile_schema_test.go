package migration

import (
	"strings"
	"testing"
)

func TestSiliconFlowBGESearchProfileMigrationKeepsVectorSpacesFenced(t *testing.T) {
	up := readPhase15SQL(t, "049_siliconflow_bge_search_profile.up.sql")
	down := readPhase15SQL(t, "049_siliconflow_bge_search_profile.down.sql")

	assertPhase15Fragments(
		t,
		up,
		"049 must bind the exact SiliconFlow models to a separate profile",
		"provider_profile_id = 'siliconflow_bge_m3_v1'",
		"embedding_model_id = 'pro/baai/bge-m3'",
		"rerank_model_id = 'pro/baai/bge-reranker-v2-m3'",
		"then 'siliconflow_bge_m3_v1'",
		"create index idx_knowledge_child_vector_shadow_jina_hnsw",
		"where embedding_model_id = 'jina-embeddings-v4'",
		"create index idx_knowledge_child_vector_shadow_bge_hnsw",
		"where embedding_model_id = 'pro/baai/bge-m3'",
	)
	assertPhase15Fragments(
		t,
		up,
		"049 must preserve legacy readers and fence the PG17 branch",
		"rename to knowledge_fetch_query_evidence_candidates_v48_legacy",
		"selected_retrieval_profile = 'legacy'",
		"knowledge_fetch_profiled_query_evidence_candidates_v47_base",
		"retrieval_head.active_profile = 'pg17_bm25_pgvector_v1'",
		"message = 'rag_retrieval_profile_changed'",
		"shadow.index_generation_id = p_index_generation_id",
		"shadow.search_profile_id = p_search_profile_id",
		"shadow.index_generation_id = active_profile.index_generation_id",
		"shadow.search_profile_id = active_profile.search_profile_id",
	)
	assertPhase15Fragments(
		t,
		up,
		"049 must route query-time providers from immutable generation metadata",
		"create function knowledge_resolve_generation_retrieval_profile",
		"create function knowledge_resolve_active_retrieval_profile",
		"create function knowledge_resolve_generation_embedding_profile",
		"grant execute on function knowledge_resolve_generation_embedding_profile",
		"to rag_worker_executor",
	)
	if strings.Contains(up, "knowledge_set_retrieval_profile(") ||
		strings.Contains(up, "knowledge_activate_structure_generation_candidate(") {
		t.Fatal("049 must not activate a retrieval profile or generation")
	}

	assertPhase15Fragments(
		t,
		down,
		"049 rollback must reject retained BGE evidence and restore legacy names",
		"message = 'rag_siliconflow_rollback_requires_bge_purge'",
		"drop function knowledge_fetch_query_evidence_candidates",
		"rename to knowledge_fetch_query_evidence_candidates",
		"drop function knowledge_resolve_active_retrieval_profile",
		"drop index idx_knowledge_child_vector_shadow_bge_hnsw",
	)
}

func TestSiliconFlowBGEConsentBackfillUsesOneStableTimestamp(t *testing.T) {
	up := readPhase15SQL(t, "049_siliconflow_bge_search_profile.up.sql")
	stableTimestampTuple := strings.Join(strings.Fields(`
		source.granted_by_user_id , current_timestamp , source.expires_at , null ,
		current_timestamp , current_timestamp
	`), " ")

	if count := strings.Count(up, stableTimestampTuple); count != 4 {
		t.Fatalf("049 stable consent timestamp tuples = %d, want 4", count)
	}
	if strings.Contains(
		up,
		"source.granted_by_user_id , clock_timestamp ( ) , source.expires_at",
	) {
		t.Fatal("049 consent backfill must not evaluate decided_at before created_at")
	}
}
