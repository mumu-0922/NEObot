package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationVerifierMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "031_structure_generation_verifier.up.sql")
	down := readPhase15SQL(t, "031_structure_generation_verifier.down.sql")
	for _, fragment := range []string{
		"knowledge_verify_structure_generation",
		"rag_structure_verify_head_stale",
		"rag_structure_verify_coverage_invalid",
		"rag_structure_verify_jobs_incomplete",
		"rag_structure_verify_projection_incomplete",
		"rag_structure_verify_replay_mismatch",
		"status='verified' , artifact_manifest_hash=computed_manifest_hash",
		"readiness='ready' , projection_revision=projection_revision+1",
		"g11.9d.3a:structure-generation-manifest:v1",
		"to go_api_runtime",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("031 migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"knowledge_promote_index_generation (",
		"set active_index_generation_id",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("031 verifier contains forbidden promotion path %q", forbidden)
		}
	}
	for _, fragment := range []string{
		"drop function knowledge_verify_structure_generation",
		"revoke update ( status , verified_at ) on knowledge_parser_artifact_sets",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("031 rollback missing %q", fragment)
		}
	}
}
