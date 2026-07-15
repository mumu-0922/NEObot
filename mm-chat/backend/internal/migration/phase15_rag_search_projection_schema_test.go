package migration

import "testing"

func TestPhase15RAGSearchProjectionSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "012_rag_search_projection.up.sql")
	down := readPhase15SQL(t, "012_rag_search_projection.down.sql")

	profileBody := mustPhase15TableBody(t, up, "knowledge_search_profiles")
	assertPhase15Columns(t, profileBody, "knowledge_search_profiles",
		"id", "index_profile_id", "provider_profile_id", "embedding_processor",
		"embedding_model_id", "embedding_dimensions", "rerank_model_id",
		"lexical_config", "exact_config", "profile_hash")
	assertPhase15Fragments(t, profileBody,
		"search profile must lock the owner-selected MinerU/Jina/Postgres profile",
		"provider_profile_id = 'mineru_jina_postgres_v1'",
		"embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024",
		"rerank_model_id = 'jina-reranker-v3'",
		"profile_hash ~ '^[0-9a-f]{64}$'")
	assertPhase15ReferenceOnDeleteRestrict(t, profileBody,
		"index_profile_id", "knowledge_index_profiles",
		"search profile must bind to the immutable index profile")

	projectionDDL := phase15TableDDL(t, up, "knowledge_child_search_projections")
	assertPhase15Columns(t, projectionDDL, "knowledge_child_search_projections",
		"child_chunk_id", "parent_chunk_id", "materialization_id",
		"index_generation_id", "collection_id", "document_id",
		"document_version_id", "search_profile_id", "embedding_vector",
		"lexical_text", "lexical_tsv", "exact_terms", "locator_summary",
		"status")
	assertPhase15Fragments(t, projectionDDL,
		"child search projection must carry Jina 1024 dense, lexical, exact, and citation lanes",
		"embedding_dimensions = 1024",
		"cardinality ( embedding_vector ) = 1024",
		"to_tsvector ( 'simple'::regconfig , lexical_text )",
		"jsonb_typeof ( locator_summary ) = 'object'",
		"status in ( 'staging' , 'ready' , 'purging' , 'purged' )")
	assertPhase15Fragments(t, up,
		"child search projection must index lexical and exact lanes",
		"using gin ( lexical_tsv )",
		"using gin ( exact_terms )")
	assertPhase15CompositeForeignKey(t, projectionDDL, "knowledge_child_chunks",
		map[string]string{"child_chunk_id": "id", "materialization_id": "materialization_id"},
		"search projection must bind each row to its immutable child chunk")
	assertPhase15CompositeForeignKey(t, projectionDDL, "knowledge_parent_chunks",
		map[string]string{
			"parent_chunk_id": "id", "materialization_id": "materialization_id",
			"index_generation_id": "index_generation_id", "document_id": "document_id",
			"document_version_id": "document_version_id",
		},
		"search projection parent fence must match materialization/generation/document")

	assertPhase15Fragments(t, up,
		"search completeness function must fail closed before publish/promotion use",
		"create function knowledge_assert_materialization_search_complete",
		"rag_search_projection_incomplete",
		"revoke all on function knowledge_assert_materialization_search_complete",
		"to rag_worker_executor")
	assertPhase15Fragments(t, up,
		"G7.4 migration must remain extension independent for the current compose Postgres image",
		"tsvector", "real[]")
	for _, forbidden := range []string{"create extension", "pg_search", "halfvec", "vector ("} {
		if containsPhase15String([]string{up}, forbidden) {
			t.Fatalf("012 contains extension-specific marker %q", forbidden)
		}
	}

	if !phase15DropsTable(down, "knowledge_child_search_projections") ||
		!phase15DropsTable(down, "knowledge_search_profiles") {
		t.Fatal("search projection down migration must drop both G7.4 tables")
	}
	assertPhase15Fragments(t, down,
		"search projection rollback must refuse silent data loss",
		"rag_down_search_projection_state_exists")
}
