package migration

import (
	"strings"
	"testing"
)

func TestParseChunkProfileResolverMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "029_rag_parse_chunk_profile_resolver.up.sql")
	down := readPhase15SQL(t, "029_rag_parse_chunk_profile_resolver.down.sql")
	for _, fragment := range []string{
		"knowledge_resolve_parse_chunk_profile",
		"rag_parse_chunk_profile_argument_invalid",
		"rag_parse_chunk_profile_missing",
		"processing_job.lease_token=p_lease_token",
		"generation.status in ( 'building' , 'verified' , 'active' )",
		"materialization.status='staging'",
		"to rag_worker_executor",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("029 migration missing %q", fragment)
		}
	}
	if !strings.Contains(down, "drop function knowledge_resolve_parse_chunk_profile") {
		t.Fatal("029 rollback must drop parse chunk-profile resolver")
	}
}
