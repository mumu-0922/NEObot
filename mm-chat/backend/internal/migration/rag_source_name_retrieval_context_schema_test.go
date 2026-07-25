package migration

import (
	"strings"
	"testing"
)

func TestRAGSourceNameRetrievalContextKeepsMetadataOutOfCitationTruth(t *testing.T) {
	up := strings.ToLower(readPhase15SQL(
		t,
		"048_rag_source_name_retrieval_context.up.sql",
	))
	down := strings.ToLower(readPhase15SQL(
		t,
		"048_rag_source_name_retrieval_context.down.sql",
	))

	for _, required := range []string{
		"knowledge_source_name_key",
		"knowledge_fetch_source_name_evidence_candidates",
		"file_record.original_filename",
		"in normalized_query_key ) > 0",
		"cross join lateral",
		"offset 0",
		"scored.exact_overlap desc",
		"source_name_weight constant double precision := 2.0",
		"knowledge_fetch_profiled_query_evidence_candidates_v47_base",
		"knowledge_fetch_generation_evaluation_candidates_v47_base",
		"knowledge_reauthorize_and_hydrate_evidence_v47_base",
		"source_name text",
		"to rag_replay_operator",
		"to rag_worker_executor , go_api_runtime",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration 048 missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"insert into",
		"update knowledge_",
		"delete from knowledge_",
		"source_name as source_text",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("migration 048 contains forbidden mutation/truth %q", forbidden)
		}
	}
	for _, required := range []string{
		"drop function knowledge_fetch_source_name_evidence_candidates",
		"rename to knowledge_fetch_profiled_query_evidence_candidates",
		"rename to knowledge_fetch_generation_evaluation_candidates",
		"rename to knowledge_reauthorize_and_hydrate_evidence",
		"to go_evidence_hydrator , go_api_runtime",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("migration 048 down missing %q", required)
		}
	}
}
