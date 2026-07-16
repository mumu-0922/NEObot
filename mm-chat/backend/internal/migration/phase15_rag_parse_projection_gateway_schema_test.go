package migration

import "testing"

func TestPhase15RAGParseProjectionGatewayContract(t *testing.T) {
	up := readPhase15SQL(t, "017_rag_parse_projection_gateway.up.sql")
	down := readPhase15SQL(t, "017_rag_parse_projection_gateway.down.sql")

	assertPhase15Fragments(t, up,
		"G7.5 parse projection gateway must expose one token-fenced worker function",
		"create function knowledge_stage_parse_projection",
		"stage = 'parse'",
		"operation in ( 'initial' , 'replace' , 'reprocess' )",
		"not processing_job.legacy_projection_unbound",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"processing_job.materialization_id = p_materialization_id",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"parse projection gateway must validate admitted materialization and profile fences",
		"source_content_hash = p_source_sha256",
		"status = 'staging'",
		"parse_artifact_set_id is null or parse_artifact_set_id = p_artifact_set_id",
		"chunk_profile_hash = p_chunk_profile_hash",
		"embedding_model_id = 'jina-embeddings-v4'",
		"rag_parse_projection_materialization_mismatch",
		"rag_parse_projection_profile_mismatch")
	assertPhase15Fragments(t, up,
		"parse projection gateway must stage every projection lane through JSONB recordsets",
		"jsonb_to_recordset ( p_blocks )",
		"insert into knowledge_blocks",
		"order by block.ordinal , block.id",
		"insert into knowledge_parent_chunks",
		"insert into knowledge_child_chunks",
		"insert into knowledge_chunk_block_spans",
		"insert into knowledge_child_search_projections",
		"search_count <> child_count",
		"rag_parse_search_projection_mismatch")
	assertPhase15Fragments(t, up,
		"parse projection gateway must bind artifact and search profile references",
		"insert into knowledge_parser_artifact_sets",
		"update knowledge_document_materializations",
		"set parse_artifact_set_id = p_artifact_set_id",
		"search_profile_id",
		"provider_profile_id = 'mineru_jina_postgres_v1'",
		"rag_parse_artifact_set_mismatch",
		"rag_parse_search_profile_missing")
	assertPhase15Fragments(t, up,
		"parse projection gateway function must stay worker-execute only",
		"revoke all on function knowledge_stage_parse_projection",
		"to rag_worker_executor",
		"grant select , insert on knowledge_parser_artifact_sets",
		"to rag_projection_owner")

	assertPhase15Fragments(t, down,
		"017 rollback must remove the default-off parse projection gateway function",
		"drop function if exists knowledge_stage_parse_projection")
}
