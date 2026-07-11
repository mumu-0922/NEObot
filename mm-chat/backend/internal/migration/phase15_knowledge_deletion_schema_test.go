package migration

import "testing"

func TestPhase151DKnowledgeDeletionSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "007_phase15_knowledge_deletion.up.sql")
	down := readPhase15SQL(t, "007_phase15_knowledge_deletion.down.sql")

	assertPhase151CPartialUniqueIndex(t, up,
		"a document visibility transition may queue only one purge per version",
		"idx_knowledge_processing_jobs_purge_fence",
		"knowledge_processing_jobs",
		[]string{"document_id", "document_version_id", "document_visibility_epoch"},
		[]string{"stage = 'purge'", "operation = 'purge'"},
	)
	if !phase15DropsIndex(down, "idx_knowledge_processing_jobs_purge_fence") {
		t.Error("Phase 15.1D deletion down does not drop the purge fence index")
	}
}
