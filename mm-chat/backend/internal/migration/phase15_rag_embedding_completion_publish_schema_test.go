package migration

import "testing"

func TestPhase15RAGEmbeddingCompletionPublishContract(t *testing.T) {
	up := readPhase15SQL(t, "021_rag_embedding_completion_publish.up.sql")
	down := readPhase15SQL(t, "021_rag_embedding_completion_publish.down.sql")

	assertPhase15Fragments(t, up,
		"021 must expose one token-fenced passage-embedding finalizer",
		"create function knowledge_complete_embedding_and_publish",
		"stage = 'passage_embedding'",
		"operation in ( 'initial' , 'replace' , 'reprocess' )",
		"processor = 'jina'",
		"model_id = 'jina-embeddings-v4'",
		"not processing_job.legacy_projection_unbound",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"021 must verify ready 1024-dimensional search rows before publish",
		"knowledge_assert_materialization_search_complete",
		"embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024",
		"embedding_vector_sha256 is not null",
		"rag_embedding_completion_hash_missing")
	assertPhase15Fragments(t, up,
		"021 must publish materialization and move query-visible heads atomically",
		"status = 'published'",
		"verified_at = clock_timestamp ( )",
		"published_at = clock_timestamp ( )",
		"insert into knowledge_document_projection_heads",
		"active_materialization_id",
		"corpus_projection_revision = corpus_projection_revision + 1")
	assertPhase15Fragments(t, up,
		"021 must activate the published version and tombstone replaced active versions",
		"previous_current_version_id",
		"status = 'tombstoned'",
		"current_version_id = materialization.document_version_id",
		"version.status in ( 'uploaded' , 'processing' , 'active' )")
	assertPhase15Fragments(t, up,
		"021 must terminally commit the embedding job and advertise readiness",
		"status = 'succeeded'",
		"lease_owner = null",
		"lease_token = null",
		"completed_at = clock_timestamp ( )",
		"knowledge_complete_embedding_and_publish ( uuid , uuid , uuid , uuid )",
		"embeddingpublishgate",
		"to rag_worker_executor",
		"grant update ( status , current_version_id , updated_at ) on knowledge_documents",
		"grant update ( status , error_code , updated_at ) on knowledge_document_versions")

	assertPhase15Fragments(t, down,
		"021 rollback must remove the embedding publish finalizer and readiness gate",
		"drop function if exists knowledge_complete_embedding_and_publish",
		"create or replace function knowledge_rag_worker_readiness",
		"searchcompletenessgate",
		"revoke update ( status , current_version_id , updated_at ) on knowledge_documents",
		"revoke update ( status , error_code , updated_at ) on knowledge_document_versions")
	if containsPhase15String([]string{down}, "embeddingpublishgate") {
		t.Fatal("021 down migration must remove the embedding publish readiness detail")
	}
}
