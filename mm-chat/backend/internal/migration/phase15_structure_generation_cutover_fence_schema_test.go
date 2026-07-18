package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationCutoverFenceMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "032_structure_generation_cutover_fence.up.sql")
	down := readPhase15SQL(t, "032_structure_generation_cutover_fence.down.sql")
	for _, fragment := range []string{
		"create or replace function knowledge_promote_index_generation",
		"for update of head",
		"knowledge_verify_structure_generation",
		"verified_manifest_hash is distinct from p_manifest_hash",
		"rag_promotion_fence_mismatch",
		"from go_api_runtime",
		"knowledge_fail_structure_generation",
		"rag_structure_fail_head_stale",
		"rag_structure_fail_replay_mismatch",
		"generation.status in ( 'verified' , 'failed' )",
		"candidate_generation.failure_code<>p_failure_code",
		"status='failed' , failure_code=p_failure_code , failed_at=failure_time",
		"readiness='failed' , updated_at=failure_time",
		"to go_api_runtime",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("032 migration missing %q", fragment)
		}
	}
	if strings.Contains(up, "grant execute on function knowledge_promote_index_generation") {
		t.Fatal("032 must not expose successful promotion before D.3c")
	}
	for _, fragment := range []string{
		"drop function knowledge_fail_structure_generation",
		"create or replace function knowledge_promote_index_generation",
		"rag_promotion_not_ready",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("032 rollback missing %q", fragment)
		}
	}
	if strings.Contains(down, "knowledge_verify_structure_generation") {
		t.Fatal("032 rollback must restore the pre-fence promotion body")
	}
}
