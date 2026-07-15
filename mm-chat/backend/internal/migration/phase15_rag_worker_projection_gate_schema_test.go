package migration

import "testing"

func TestPhase15RAGWorkerProjectionGateReadinessContract(t *testing.T) {
	up := readPhase15SQL(t, "013_rag_worker_projection_gate.up.sql")
	down := readPhase15SQL(t, "013_rag_worker_projection_gate.down.sql")

	assertPhase15Fragments(t, up,
		"G7.5 readiness must include the G7.4 search completeness function",
		"create or replace function knowledge_rag_worker_readiness",
		"knowledge_assert_materialization_search_complete ( uuid , bigint , text , integer )",
		"searchcompletenessgate",
		"revoke all on function knowledge_rag_worker_readiness",
		"to rag_worker_executor , rag_api_reader , go_api_runtime")
	assertPhase15Fragments(t, down,
		"G7.5 rollback must restore the previous readiness surface",
		"create or replace function knowledge_rag_worker_readiness",
		"revoke all on function knowledge_rag_worker_readiness")
	if containsPhase15String([]string{down}, "searchcompletenessgate") {
		t.Fatal("013 down migration must remove the G7.5 readiness detail")
	}
}
