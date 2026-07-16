package migration

import "testing"

func TestPhase15RAGParseSourceMetadataGatewayContract(t *testing.T) {
	up := readPhase15SQL(t, "016_rag_parse_source_metadata_gateway.up.sql")
	down := readPhase15SQL(t, "016_rag_parse_source_metadata_gateway.down.sql")

	assertPhase15Fragments(t, up,
		"G7.5 parse source metadata gateway must expose one token-fenced worker function",
		"create function knowledge_fetch_parse_source_metadata",
		"returns table ( file_id uuid , storage_backend text , object_key text , sha256 text , byte_size bigint , content_type text )",
		"stage = 'parse'",
		"operation in ( 'initial' , 'replace' , 'reprocess' )",
		"not processing_job.legacy_projection_unbound",
		"lease_owner = p_worker_id",
		"lease_token = p_lease_token",
		"lease_expires_at > clock_timestamp ( )",
		"processing_job.file_id = p_file_id",
		"processing_job.materialization_id = p_materialization_id",
		"rag_stale_job_lease")
	assertPhase15Fragments(t, up,
		"source metadata fetch must bind file, version, materialization, and visibility fences",
		"from files file_record",
		"file_record.upload_status = 'available'",
		"file_record.deleted_at is null",
		"file_record.byte_size > 0",
		"file_record.sha256 = version.content_hash",
		"materialization.source_content_hash = file_record.sha256",
		"materialization.status = 'staging'",
		"collection.acl_revision = job.collection_acl_revision",
		"collection.collection_processing_revision = job.collection_processing_revision",
		"document.visibility_epoch = job.document_visibility_epoch",
		"rag_parse_source_metadata_missing")
	assertPhase15Fragments(t, up,
		"parse source metadata function must stay worker-execute only",
		"revoke all on function knowledge_fetch_parse_source_metadata",
		"to rag_worker_executor")

	assertPhase15Fragments(t, down,
		"016 rollback must remove the default-off parse source metadata gateway function",
		"drop function if exists knowledge_fetch_parse_source_metadata")
}
