package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationAtomicCutoverMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "033_structure_generation_atomic_cutover.up.sql")
	down := readPhase15SQL(t, "033_structure_generation_atomic_cutover.down.sql")
	for _, fragment := range []string{
		"grant execute on function knowledge_promote_index_generation",
		"create function knowledge_rollback_index_generation",
		"for update of head",
		"g11.9d-structure-rebuild-snapshot.v1",
		"sourcegenerationid' ) is distinct from p_target_generation_id::text",
		"rag_generation_rollback_coverage_invalid",
		"rag_generation_rollback_projection_incomplete",
		"set status='retired' , retired_at=transition_time",
		"set status='active' , retired_at=null",
		"set active_index_generation_id=p_target_generation_id",
		"to go_api_runtime",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("033 migration missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"drop function knowledge_rollback_index_generation",
		"revoke execute on function knowledge_promote_index_generation",
		"from go_api_runtime",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("033 rollback missing %q", fragment)
		}
	}
}
