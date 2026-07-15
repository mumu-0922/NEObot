package migration

import "testing"

func TestPhase15RAGPurgeProjectionGatewayContract(t *testing.T) {
	up := readPhase15SQL(t, "014_rag_purge_projection_gateway.up.sql")
	down := readPhase15SQL(t, "014_rag_purge_projection_gateway.down.sql")

	assertPhase15Fragments(t, up,
		"G7.5 purge gateway must expose token-fenced stored functions only",
		"create function knowledge_mark_purge_invisible",
		"create function knowledge_purge_search_projection",
		"create function knowledge_assert_purge_complete",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"not legacy_projection_unbound",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"purge gateway must mark search rows purged and prove no ready rows remain",
		"update knowledge_child_search_projections search",
		"set status = 'purged'",
		"remaining_ready_child_search_rows",
		"rag_purge_projection_incomplete")
	assertPhase15Fragments(t, up,
		"purge gateway functions must stay worker-execute only",
		"revoke all on function knowledge_mark_purge_invisible",
		"revoke all on function knowledge_purge_search_projection",
		"revoke all on function knowledge_assert_purge_complete",
		"to rag_worker_executor")

	assertPhase15Fragments(t, down,
		"014 rollback must remove the default-off purge gateway functions",
		"drop function if exists knowledge_mark_purge_invisible",
		"drop function if exists knowledge_purge_search_projection",
		"drop function if exists knowledge_assert_purge_complete")
}
