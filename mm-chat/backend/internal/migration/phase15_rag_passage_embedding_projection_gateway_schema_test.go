package migration

import "testing"

func TestPhase15RAGPassageEmbeddingProjectionGatewayContract(t *testing.T) {
	up := readPhase15SQL(t, "015_rag_passage_embedding_projection_gateway.up.sql")
	down := readPhase15SQL(t, "015_rag_passage_embedding_projection_gateway.down.sql")

	assertPhase15Fragments(t, up,
		"G7.5 passage embedding gateway must expose token-fenced worker functions only",
		"create function knowledge_fetch_passage_embedding_candidates",
		"create function knowledge_stage_passage_embedding",
		"stage = 'passage_embedding'",
		"operation in ( 'initial' , 'replace' , 'reprocess' )",
		"not processing_job.legacy_projection_unbound",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"fetch must return bounded child text candidates from the admitted materialization",
		"returns table ( child_chunk_id uuid , content text , content_hash text )",
		"search.lexical_text",
		"search.status in ( 'staging' , 'ready' )",
		"order by child.ordinal , child.id")
	assertPhase15Fragments(t, up,
		"stage must pin Jina 1024 vectors and mark rows ready",
		"cardinality ( p_embedding_vector ) <> 1024",
		"embedding_model_id = 'jina-embeddings-v4'",
		"embedding_dimensions = 1024",
		"embedding_vector = p_embedding_vector",
		"embedding_vector_sha256 = p_embedding_vector_sha256",
		"status = 'ready'",
		"rag_passage_embedding_target_missing")
	assertPhase15Fragments(t, up,
		"passage embedding gateway functions must stay worker-execute only",
		"revoke all on function knowledge_fetch_passage_embedding_candidates",
		"revoke all on function knowledge_stage_passage_embedding",
		"to rag_worker_executor")

	assertPhase15Fragments(t, down,
		"015 rollback must remove default-off passage embedding gateway functions",
		"drop function if exists knowledge_stage_passage_embedding",
		"drop function if exists knowledge_fetch_passage_embedding_candidates")
}
