package migration

import "testing"

func TestPhase15RAGQueryEvidenceGoAPIGrantContract(t *testing.T) {
	up := readPhase15SQL(t, "024_rag_query_evidence_go_api_grant.up.sql")
	down := readPhase15SQL(t, "024_rag_query_evidence_go_api_grant.down.sql")

	assertPhase15Fragments(t, up,
		"024 must let Go API fetch reference-only query candidates",
		"grant execute on function knowledge_fetch_query_evidence_candidates",
		"uuid[] , text , integer",
		"to go_api_runtime")
	assertPhase15Fragments(t, down,
		"024 rollback must remove only the Go API candidate grant",
		"revoke execute on function knowledge_fetch_query_evidence_candidates",
		"from go_api_runtime")
}
