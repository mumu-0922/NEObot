package migration

import "testing"

func TestPhase15RAGCJKBigramNormalizationMigration(t *testing.T) {
	up := readPhase15SQL(t, "026_rag_cjk_bigram_normalization.up.sql")
	down := readPhase15SQL(t, "026_rag_cjk_bigram_normalization.down.sql")

	assertPhase15Fragments(t, up,
		"026 must normalize CJK query text without locale-dependent alnum classes",
		"create or replace function knowledge_fetch_query_evidence_candidates",
		"compact_query text",
		"regexp_replace",
		"substr ( compact_query , ordinal , 2 )",
		"query_bigrams",
		"bigram_signal.hit_count >= least ( 2 , cardinality ( query_bigrams ) )",
		"grant execute on function knowledge_fetch_query_evidence_candidates")
	if containsPhase15String([]string{up}, "^[[:alnum:]]{2}$") {
		t.Fatal("026 must not gate CJK bigrams through locale-dependent [:alnum:]")
	}
	assertPhase15Fragments(t, down,
		"026 rollback must restore migration 025 candidate behavior",
		"create or replace function knowledge_fetch_query_evidence_candidates",
		"^[[:alnum:]]{2}$")
}
