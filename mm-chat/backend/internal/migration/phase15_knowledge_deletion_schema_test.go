package migration

import (
	"strings"
	"testing"
)

func TestPhase151DKnowledgeDeletionSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "007_phase15_knowledge_deletion.up.sql")
	down := readPhase15SQL(t, "007_phase15_knowledge_deletion.down.sql")

	assertPhase15Fragments(t, strings.ToLower(up),
		"migration 007 must reconcile databases created by the short-lived 006 variant",
		"create unique index if not exists idx_knowledge_processing_jobs_purge_fence",
		"on knowledge_processing_jobs",
		"document_visibility_epoch",
		"stage = 'purge'",
		"operation = 'purge'",
	)
	if phase15DropsIndex(down, "idx_knowledge_processing_jobs_purge_fence") {
		t.Error("Phase 15.1D deletion down must preserve the migration 006 purge fence")
	}
}
