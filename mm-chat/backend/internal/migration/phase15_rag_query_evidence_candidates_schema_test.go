package migration

import "testing"

func TestPhase15RAGQueryEvidenceCandidatesContract(t *testing.T) {
	up := readPhase15SQL(t, "022_rag_query_evidence_candidates.up.sql")
	down := readPhase15SQL(t, "022_rag_query_evidence_candidates.down.sql")

	assertPhase15Fragments(t, up,
		"022 must expose a selected-collection evidence candidate function",
		"create function knowledge_fetch_query_evidence_candidates",
		"p_collection_ids uuid",
		"p_query_text text",
		"p_limit integer",
		"returns table",
		"collection_id uuid",
		"child_chunk_id uuid",
		"source_span_hash text",
		"rank_score real")
	assertPhase15Fragments(t, up,
		"022 must fail closed on broad or malformed query input",
		"cardinality ( p_collection_ids ) not between 1 and 32",
		"array_position ( p_collection_ids , null ) is not null",
		"octet_length ( normalized_query ) not between 1 and 2048",
		"p_limit not between 1 and 50",
		"rag_query_candidate_argument_invalid")
	assertPhase15Fragments(t, up,
		"022 must only return published active ready rows from selected collections",
		"select distinct unnest ( p_collection_ids ) as id",
		"search.collection_id = selected.id",
		"search.index_generation_id = corpus.active_index_generation_id",
		"head.active_materialization_id = search.materialization_id",
		"materialization.status = 'published'",
		"document.status = 'active'",
		"document.current_version_id = search.document_version_id",
		"version.status = 'active'",
		"search.status = 'ready'",
		"embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024")
	assertPhase15Fragments(t, up,
		"022 must return references only and stay Python-worker execute only",
		"ts_rank",
		"plainto_tsquery ( 'simple' , normalized_query )",
		"search.exact_terms && query_terms",
		"revoke all on function knowledge_fetch_query_evidence_candidates",
		"to rag_worker_executor")
	if containsPhase15String([]string{up}, "lexical_text") {
		t.Fatal("022 query candidate function must not return document body text")
	}

	assertPhase15Fragments(t, down,
		"022 rollback must remove only the query candidate function",
		"drop function if exists knowledge_fetch_query_evidence_candidates",
		"from rag_worker_executor")
}
