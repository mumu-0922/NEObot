package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestJinaRuntimeRetirementMigrationMatchesAppliedManifest(t *testing.T) {
	migrations, err := Load(migrationfiles.FS)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}

	const appliedChecksum = "87302c2cf0dee5ce11388795891db4e64dfba4a7086a9906bcbaeab5397519e6"
	for _, migration := range migrations {
		if migration.ID() != "050_jina_runtime_retirement" {
			continue
		}
		if migration.Checksum != appliedChecksum {
			t.Fatalf("migration 050 checksum = %q, want applied manifest %q", migration.Checksum, appliedChecksum)
		}
		return
	}

	t.Fatal("migration 050 is not embedded")
}

func TestJinaRuntimeRetirementMigrationPermanentlyFencesExecution(t *testing.T) {
	up := readPhase15SQL(t, "050_jina_runtime_retirement.up.sql")
	for _, contract := range []string{
		"encrypted_secret_ref = NULL",
		"RAG:JINA",
		"RAG_JINA_PROVIDER_SECRET_PURGE_FAILED",
		"DELETE FROM plugin_registry",
		"jina-web-reader",
		"RAG_JINA_PLUGIN_REGISTRY_PURGE_FAILED",
		"knowledge_enforce_bge_active_generation_transition",
		"siliconflow_bge_m3_v1",
		"Pro/BAAI/bge-m3",
		"Pro/BAAI/bge-reranker-v2-m3",
		"RAG_RETIRED_RETRIEVAL_GENERATION_ACTIVATION_FORBIDDEN",
		"BEFORE UPDATE OF status , index_profile_id",
		"knowledge_fetch_fenced_profiled_candidates_v49_base",
		"knowledge_fetch_profiled_query_evidence_candidates_v49_base",
		"knowledge_fetch_generation_evaluation_candidates_v49_base",
		"knowledge_fetch_hybrid_shadow_diagnostics_v49_base",
		"RAG_RETIRED_RETRIEVAL_PROFILE_NON_EXECUTABLE",
		"FROM knowledge_fetch_query_evidence_candidates",
		"FROM PUBLIC , go_api_runtime , rag_worker_executor , rag_replay_operator",
	} {
		if !strings.Contains(up, strings.ToLower(contract)) {
			t.Fatalf("migration 050 missing contract %q", contract)
		}
	}

	down := readPhase15SQL(t, "050_jina_runtime_retirement.down.sql")
	if !strings.Contains(down, strings.ToLower("RAG_JINA_RUNTIME_RETIREMENT_IS_IRREVERSIBLE")) {
		t.Fatal("migration 050 must not expose a Jina reactivation rollback")
	}
}
