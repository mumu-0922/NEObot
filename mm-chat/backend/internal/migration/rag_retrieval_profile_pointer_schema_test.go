package migration

import "testing"

func TestRAGRetrievalProfilePointerMigration(t *testing.T) {
	up := readPhase15SQL(t, "037_rag_retrieval_profile_pointer.up.sql")
	down := readPhase15SQL(t, "037_rag_retrieval_profile_pointer.down.sql")

	assertPhase15Fragments(t, up,
		"037 must place the candidate reader behind a PG16-compatible pointer",
		"quote_ident ( current_schema ( ) ) || ' , pg_catalog , pg_temp'",
		"create table knowledge_retrieval_profile_head",
		"active_profile in ( 'legacy' , 'pg17_bm25_pgvector_v1' )",
		"insert into knowledge_retrieval_profile_head",
		"values ( 1 , 'legacy' , 1 )",
		"create table knowledge_retrieval_profile_transitions",
		"create function knowledge_set_retrieval_profile",
		"message = 'rag_retrieval_profile_conflict'",
		"message = 'rag_retrieval_profile_unavailable'",
		"create function knowledge_fetch_profiled_query_evidence_candidates",
		"from knowledge_fetch_hybrid_query_evidence_candidates",
		"grant execute on function knowledge_set_retrieval_profile",
		"to rag_replay_operator",
		"grant execute on function knowledge_fetch_profiled_query_evidence_candidates",
		"to rag_worker_executor , go_api_runtime")
	assertPhase15Fragments(t, down,
		"037 rollback must fail closed unless the pointer is legacy",
		"message = 'rag_retrieval_profile_rollback_requires_legacy'",
		"drop function knowledge_fetch_profiled_query_evidence_candidates",
		"drop function knowledge_set_retrieval_profile",
		"drop table knowledge_retrieval_profile_transitions",
		"drop table knowledge_retrieval_profile_head")
}
