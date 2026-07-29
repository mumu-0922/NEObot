package migration

import (
	"strings"
	"testing"
)

func TestMemoryHybridRelevanceAdmissionMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "064_memory_hybrid_relevance_admission.up.sql")
	down := readPhase15SQL(t, "064_memory_hybrid_relevance_admission.down.sql")
	assertPhase15Fragments(t, up,
		"064 must add a local pre-rerank authority gate",
		"memory_authorize_hybrid_rerank",
		"security definer",
		"p_query_embedding real[]",
		"cardinality ( p_query_embedding ) <> 1024",
		"observation.status = 'pending'",
		"observation.result_code = 'candidates_ready'",
		"result.lane = 'rrf'",
		"projection.embedding_vector <=> p_query_embedding::vector ( 1024 )",
		"v_vector_count <> v_expected",
		"memory_hybrid_rerank_admission_authority_stale")
	assertPhase15Fragments(t, up,
		"064 must expose only the narrow API capability",
		"revoke all on function memory_authorize_hybrid_rerank",
		"from public",
		"grant execute on function memory_authorize_hybrid_rerank",
		"to go_api_runtime")
	for _, forbidden := range []string{
		"insert into message_memory_hybrid_shadow",
		"update message_memory_hybrid_shadow",
		"query_text", "memory_content", "raw_score", "rerank_score",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("064 persists forbidden admission payload %q", forbidden)
		}
	}
	assertPhase15Fragments(t, down,
		"064 rollback must remove only the additive capability",
		"revoke all on function memory_authorize_hybrid_rerank",
		"from go_api_runtime",
		"drop function memory_authorize_hybrid_rerank")
}
