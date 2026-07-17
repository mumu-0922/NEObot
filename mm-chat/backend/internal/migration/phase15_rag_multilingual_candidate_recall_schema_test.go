package migration

import "testing"

func TestPhase15RAGMultilingualCandidateRecallContract(t *testing.T) {
	up := readPhase15SQL(t, "025_rag_multilingual_candidate_recall.up.sql")
	down := readPhase15SQL(t, "025_rag_multilingual_candidate_recall.down.sql")

	assertPhase15Fragments(t, up,
		"025 must preserve lexical recall and add bounded multilingual signals",
		"create or replace function knowledge_fetch_query_evidence_candidates",
		"generate_series ( 1 , greatest ( char_length ( normalized_query ) - 1 , 0 ) )",
		"limit 64",
		"position ( lower ( normalized_query ) in lower ( child.content ) )",
		"bigram_signal.hit_count >= least ( 2 , cardinality ( query_bigrams ) )")
	assertPhase15Fragments(t, up,
		"025 must retain selected active projection and reference-only boundaries",
		"search.collection_id = selected.id",
		"head.active_materialization_id = search.materialization_id",
		"materialization.status = 'published'",
		"document.status = 'active'",
		"search.status = 'ready'",
		"to rag_worker_executor",
		"to go_api_runtime")
	if containsPhase15String([]string{up}, "lexical_text") {
		t.Fatal("025 query candidate function must not return document body text")
	}

	assertPhase15Fragments(t, down,
		"025 rollback must restore the prior lexical/exact-term function",
		"create or replace function knowledge_fetch_query_evidence_candidates",
		"search.lexical_tsv @@ plainto_tsquery",
		"search.exact_terms && query_terms",
		"to go_api_runtime")
	if containsPhase15String([]string{down}, "query_bigrams") {
		t.Fatal("025 rollback must remove multilingual bigram recall")
	}
}
