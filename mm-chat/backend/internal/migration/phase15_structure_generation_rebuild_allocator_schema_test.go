package migration

import (
	"strings"
	"testing"
)

func TestStructureGenerationRebuildAllocatorMigrationContract(t *testing.T) {
	sql := readPhase15SQL(t, "028_structure_generation_rebuild_allocator.up.sql")
	down := readPhase15SQL(t, "028_structure_generation_rebuild_allocator.down.sql")
	required := []string{
		"knowledge_begin_structure_generation_rebuild",
		"status in ( 'building' , 'verified' )",
		"rag_structure_rebuild_candidate_exists",
		"rag_structure_rebuild_allocation_coverage_invalid",
		"where item->>'documentid'=document.id::text",
		"c.collection_processing_revision processing_revision",
		"'parse' , 'reprocess'",
		"p_generation_id , ( allocation->>'materializationid' ) ::uuid , false",
		"to go_api_runtime",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("028 migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"knowledge_promote_index_generation(",
		"set active_index_generation_id",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("028 migration contains forbidden active mutation %q", forbidden)
		}
	}
	for _, fragment := range []string{
		"drop function knowledge_begin_structure_generation_rebuild",
		"revoke select , insert on knowledge_index_profiles from rag_projection_owner",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("028 rollback missing %q", fragment)
		}
	}
}
