package migration

import (
	"strings"
	"testing"
)

func TestRAGGenerationEvaluationBoundaryIsReadOnlyAndOperatorOnly(t *testing.T) {
	up := strings.ToLower(readPhase15SQL(
		t,
		"047_rag_generation_evaluation_boundary.up.sql",
	))
	down := strings.ToLower(readPhase15SQL(
		t,
		"047_rag_generation_evaluation_boundary.down.sql",
	))

	for _, required := range []string{
		"knowledge_fetch_generation_evaluation_candidates",
		"knowledge_hydrate_generation_evaluation_evidence",
		"security definer",
		"owner to rag_projection_owner",
		"to rag_replay_operator",
		"knowledge_bm25_shadow_build_sources",
		"p_index_generation_id",
		"generation.status = 'verified'",
		"head.active_index_generation_id = generation.id",
		"knowledge_locator_summary_is_valid",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration 047 missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"to go_api_runtime",
		"to rag_worker_executor",
		"insert into",
		"update knowledge_",
		"delete from knowledge_",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("migration 047 contains forbidden boundary %q", forbidden)
		}
	}
	for _, required := range []string{
		"drop function if exists knowledge_fetch_generation_evaluation_candidates",
		"drop function if exists knowledge_hydrate_generation_evaluation_evidence",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("migration 047 down missing %q", required)
		}
	}
}
