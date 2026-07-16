package migration

import "testing"

func TestPhase15RAGParseCompletionEnqueueEmbeddingContract(t *testing.T) {
	up := readPhase15SQL(t, "020_rag_parse_completion_enqueue_embedding.up.sql")
	down := readPhase15SQL(t, "020_rag_parse_completion_enqueue_embedding.down.sql")

	assertPhase15Fragments(t, up,
		"020 must expose one token-fenced parse finalizer",
		"create function knowledge_complete_parse_and_enqueue_embedding",
		"stage = 'parse'",
		"operation in ( 'initial' , 'replace' , 'reprocess' )",
		"not processing_job.legacy_projection_unbound",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"processing_job.materialization_id = p_materialization_id",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"020 must verify staged parse projection before enqueue",
		"status = 'staging'",
		"parse_artifact_set_id is not null",
		"knowledge_child_search_projections",
		"embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024",
		"rag_parse_completion_search_staging_missing")
	assertPhase15Fragments(t, up,
		"020 must bind embedding job to active Jina authority",
		"processor_governance_heads",
		"processor_governance_profiles",
		"processing_consents",
		"'passage_embedding' = any ( profile.allowed_purposes )",
		"'text/plain' = any ( consent.data_types )",
		"rag_parse_completion_consent_missing")
	assertPhase15Fragments(t, up,
		"020 must enqueue one pending passage embedding job and commit parse terminally",
		"insert into knowledge_processing_jobs",
		"'passage_embedding'",
		"caused_by_job_id",
		"status = 'succeeded'",
		"lease_owner = null",
		"lease_token = null",
		"completed_at = clock_timestamp ( )")
	assertPhase15Fragments(t, up,
		"020 must stay worker-execute only while granting least privilege to the owner",
		"revoke all on function knowledge_complete_parse_and_enqueue_embedding",
		"to rag_worker_executor",
		"grant select on processor_governance_profiles",
		"to rag_projection_owner")

	assertPhase15Fragments(t, down,
		"020 rollback must remove only the parse finalizer and added grants",
		"drop function if exists knowledge_complete_parse_and_enqueue_embedding",
		"revoke select on processor_governance_profiles",
		"from rag_projection_owner")
}
