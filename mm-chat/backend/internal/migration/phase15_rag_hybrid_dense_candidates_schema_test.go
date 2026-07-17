package migration

import "testing"

func TestPhase15RAGHybridDenseCandidatesMigration(t *testing.T) {
	up := readPhase15SQL(t, "027_rag_hybrid_dense_candidates.up.sql")
	down := readPhase15SQL(t, "027_rag_hybrid_dense_candidates.down.sql")

	assertPhase15Fragments(t, up,
		"027 must expose bounded Jina 1024 hybrid retrieval",
		"create function knowledge_fetch_hybrid_query_evidence_candidates",
		"p_collection_ids uuid",
		"p_query_text text",
		"p_query_embedding real",
		"cardinality ( p_query_embedding ) <> 1024",
		"rag_hybrid_query_embedding_invalid",
		"knowledge_fetch_query_evidence_candidates",
		"cosine_similarity",
		"minimum_dense_similarity",
		"minimum_dense_query_characters",
		"char_length ( btrim ( p_query_text ) ) >= minimum_dense_query_characters",
		"sum ( 1.0 / ( rrf_constant + lanes.lane_rank ) )",
		"grant execute on function knowledge_fetch_hybrid_query_evidence_candidates")
	assertPhase15Fragments(t, up,
		"027 must retain selected active published reference-only fences",
		"search.collection_id = selected.id",
		"search.index_generation_id = corpus.active_index_generation_id",
		"head.active_materialization_id = search.materialization_id",
		"materialization.status = 'published'",
		"collection.deleted_at is null",
		"document.status = 'active'",
		"version.status = 'active'",
		"search.status = 'ready'",
		"search.embedding_model_id = 'jina-embeddings-v4'",
		"search.embedding_dimensions = 1024")
	if containsPhase15String([]string{up}, "child.content") ||
		containsPhase15String([]string{up}, "lexical_text") {
		t.Fatal("027 hybrid function must return references without source body text")
	}

	assertPhase15Fragments(t, down,
		"027 rollback must remove only the hybrid candidate function",
		"drop function if exists knowledge_fetch_hybrid_query_evidence_candidates",
		"from go_api_runtime",
		"from rag_worker_executor")
}
