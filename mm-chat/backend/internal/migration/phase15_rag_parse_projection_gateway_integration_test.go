package migration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

type phase15RAGParseProjectionFixture struct {
	userID                    string
	fileID                    string
	collectionID              string
	documentID                string
	documentVersionID         string
	governanceProfileID       string
	consentID                 string
	indexProfileID            string
	indexGenerationID         string
	materializationID         string
	searchProfileID           string
	artifactSetID             string
	blockID                   string
	parentChunkID             string
	childChunkID              string
	jobID                     string
	workerID                  string
	leaseToken                string
	staleLeaseToken           string
	collectionVisibilityEpoch int64
	documentVisibilityEpoch   int64
	sourceSHA256              string
	chunkProfileHash          string
	baseProfileHash           string
}

func TestPhase15RAGParseProjectionGatewayLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	assertPhase15Postgres16(t, ctx, db)

	runner := NewRunner(db, migrationfiles.FS)
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("fresh up through parse projection gateway: %v", err)
	}
	if len(applied) < 17 || applied[16].Version != 17 {
		t.Fatalf("fresh applied = %#v, want migration 017 present", applied)
	}

	fixture := seedPhase15RAGParseProjectionFixture(t, ctx, db)
	assertPhase15RAGParseGatewayWorkerBoundary(t, ctx, db, fixture)
	assertPhase15RAGParseGatewayStaleLease(t, ctx, db, fixture)
	assertPhase15RAGParseGatewayProfileMismatch(t, ctx, db, fixture)

	var staged bool
	if err := db.QueryRowContext(ctx, phase15RAGParseStageSQL(),
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.materializationID,
		fixture.artifactSetID,
		fixture.sourceSHA256,
		fixture.chunkProfileHash,
		fixture.blockID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.parentChunkID,
		fixture.indexGenerationID,
		fixture.childChunkID,
		fixture.collectionID,
	).Scan(&staged); err != nil {
		t.Fatalf("stage parse projection: %v", err)
	}
	if !staged {
		t.Fatal("parse projection staging returned false")
	}

	assertPhase15RAGParseProjectionRows(t, ctx, db, fixture)
}

func seedPhase15RAGParseProjectionFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) phase15RAGParseProjectionFixture {
	t.Helper()

	fixture := phase15RAGParseProjectionFixture{
		userID:                    "00000000-0000-0000-0000-0000000b1001",
		fileID:                    "00000000-0000-0000-0000-0000000b1002",
		collectionID:              "00000000-0000-0000-0000-0000000b1003",
		documentID:                "00000000-0000-0000-0000-0000000b1004",
		documentVersionID:         "00000000-0000-0000-0000-0000000b1005",
		governanceProfileID:       "00000000-0000-0000-0000-0000000b1006",
		consentID:                 "00000000-0000-0000-0000-0000000b1007",
		indexProfileID:            "00000000-0000-0000-0000-0000000b1008",
		indexGenerationID:         "00000000-0000-0000-0000-0000000b1009",
		materializationID:         "00000000-0000-0000-0000-0000000b1010",
		searchProfileID:           "00000000-0000-0000-0000-0000000b1011",
		artifactSetID:             "00000000-0000-0000-0000-0000000b1012",
		blockID:                   "00000000-0000-0000-0000-0000000b1013",
		parentChunkID:             "00000000-0000-0000-0000-0000000b1014",
		childChunkID:              "00000000-0000-0000-0000-0000000b1015",
		jobID:                     "00000000-0000-0000-0000-0000000b1016",
		workerID:                  "00000000-0000-0000-0000-0000000b1017",
		leaseToken:                "00000000-0000-0000-0000-0000000b1018",
		staleLeaseToken:           "00000000-0000-0000-0000-0000000b1019",
		collectionVisibilityEpoch: 1,
		documentVisibilityEpoch:   1,
		sourceSHA256:              "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		chunkProfileHash:          "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		baseProfileHash:           "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	}

	const (
		hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hashC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		hashF = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		hash0 = "0000000000000000000000000000000000000000000000000000000000000000"
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users (id, email, display_name)
VALUES ($1, 'g7519-parse@example.test', 'G7.5.19 Parse')`,
		fixture.userID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES (
  $1, $2, 'g7-5-19.pdf', 'application/pdf', 16, $3,
  'knowledge/g7-5-19.pdf'
)`,
		fixture.fileID,
		fixture.userID,
		fixture.sourceSHA256,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_collections (
  id, name, scope, owner_user_id, created_by_user_id
) VALUES ($1, 'G7.5.19 Parse Gateway', 'personal', $2, $2)`,
		fixture.collectionID,
		fixture.userID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_documents (
  id, collection_id, status, visibility_epoch, created_by_user_id
) VALUES ($1, $2, 'processing', $3, $4)`,
		fixture.documentID,
		fixture.collectionID,
		fixture.documentVisibilityEpoch,
		fixture.userID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, visibility_epoch, status,
  content_hash, created_by_user_id
) VALUES ($1, $2, $3, 1, $4, 'processing', $5, $6)`,
		fixture.documentVersionID,
		fixture.documentID,
		fixture.fileID,
		fixture.documentVisibilityEpoch,
		fixture.sourceSHA256,
		fixture.userID,
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_profiles (
  id, processor, endpoint_id, model_id, model_api_version,
  profile_contract_hash, allowed_purposes, allowed_data_types, region,
  retention_policy, deletion_contract, training_use, status,
  governance_revision, manifest_hash
) VALUES (
  $1, 'mineru', 'hosted-main', 'mineru-parser-v20260716', 'api-20260623',
  $2, ARRAY['parse'], ARRAY['application/pdf'], 'global', 'none', 'delete',
  'disabled', 'approved', 1, $3
)`,
		fixture.governanceProfileID,
		hashF,
		hashA,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processor_governance_heads (
  processor, endpoint_id, model_id, status, active_profile_id,
  active_governance_revision, head_revision
) VALUES (
  'mineru', 'hosted-main', 'mineru-parser-v20260716', 'active', $1, 1, 1
)`,
		fixture.governanceProfileID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO processing_consents (
  id, scope, collection_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id
) VALUES (
  $1, 'collection', $2, 'mineru', 'hosted-main', 'mineru-parser-v20260716',
  $3, 1, 1, ARRAY['parse'], ARRAY['application/pdf'], 'g7.5.19',
  'granted', 1, $4
)`,
		fixture.consentID,
		fixture.collectionID,
		fixture.governanceProfileID,
		fixture.userID,
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_index_profiles (
  id, contract_version, canonical_schema_version, parser_manifest,
  parser_manifest_hash, chunk_manifest, chunk_profile_hash,
  embedding_processor, embedding_endpoint_id, embedding_model_id,
  embedding_api_version, embedding_role, rerank_processor, rerank_endpoint_id,
  rerank_model_id, rerank_api_version, base_profile_hash
) VALUES (
  $1, 1, 'canonical-ir-v2', '{}'::jsonb, $2, '{}'::jsonb, $3,
  'jina', 'admin-env', 'jina-embeddings-v4', 'api-20260623', 'passage',
  'jina', 'admin-env', 'jina-reranker-v3', 'api-20260623', $4
)`,
		fixture.indexProfileID,
		hashC,
		fixture.chunkProfileHash,
		fixture.baseProfileHash,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_index_generations (
  id, index_profile_id, generation_seq, status, build_snapshot,
  build_snapshot_hash
) VALUES ($1, $2, 1, 'building', '{}'::jsonb, $3)`,
		fixture.indexGenerationID,
		fixture.indexProfileID,
		hashA,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_search_profiles (
  id, index_profile_id, provider_profile_id, embedding_processor,
  embedding_model_id, embedding_dimensions, rerank_processor, rerank_model_id,
  lexical_config, exact_config, profile_hash
) VALUES (
  $1, $2, 'mineru_jina_postgres_v1', 'jina', 'jina-embeddings-v4',
  1024, 'jina', 'jina-reranker-v3', '{}'::jsonb, '{}'::jsonb, $3
)`,
		fixture.searchProfileID,
		fixture.indexProfileID,
		hash0,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status
) VALUES (
  $1, $2, $3, $4, $5, $6, 1, $7, $8,
  1, $9, 1, $10, 'staging'
)`,
		fixture.materializationID,
		fixture.indexGenerationID,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.fileID,
		fixture.sourceSHA256,
		fixture.baseProfileHash,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage,
  operation, processor, endpoint_id, model_id, governance_profile_id,
  governance_revision, governance_head_revision, collection_consent_id,
  collection_consent_revision, collection_acl_revision,
  collection_visibility_epoch, collection_processing_revision,
  document_visibility_epoch, requested_by_user_id, idempotency_scope,
  idempotency_key, request_hash, status, attempt_count, max_attempts,
  available_at, lease_owner, lease_token, lease_expires_at,
  index_generation_id, materialization_id, legacy_projection_unbound
) VALUES (
  $1, $2, $3, $4, $5, 'parse', 'initial', 'mineru', 'hosted-main',
  'mineru-parser-v20260716', $6, 1, 1, $7, 1, 1, $8, 1, $9,
  $10, 'g7.5.19-parse-gateway', 'stage-parse-projection', $11,
  'processing', 1, 3, clock_timestamp(), $12, $13,
  clock_timestamp() + interval '15 minutes', $14, $15, false
)`,
		fixture.jobID,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.fileID,
		fixture.governanceProfileID,
		fixture.consentID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
		fixture.userID,
		fixture.sourceSHA256,
		fixture.workerID,
		fixture.leaseToken,
		fixture.indexGenerationID,
		fixture.materializationID,
	)

	return fixture
}

func phase15RAGParseStageSQL() string {
	return `
SELECT knowledge_stage_parse_projection(
  $1, $2, $3, $4, $5, $6, $7,
  jsonb_build_array(jsonb_build_object(
    'id', $8::uuid,
    'artifact_set_id', $5::uuid,
    'document_id', $9::uuid,
    'document_version_id', $10::uuid,
    'parent_block_id', NULL,
    'ordinal', 0,
    'block_type', 'paragraph',
    'heading_path', ARRAY['G7.5.19']::text[],
    'text_content', 'Parse projection fixture text.',
    'locator_kind', 'line_range',
    'locator', jsonb_build_object('kind','line_range','startLine',0,'endLine',0),
    'reading_order', 0,
    'provenance', jsonb_build_object('source','integration-test'),
    'confidence', 1.0,
    'content_hash', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff',
    'source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
    'derived', false,
    'non_indexable', false,
    'needs_review', false
  )),
  jsonb_build_array(jsonb_build_object(
    'id', $11::uuid,
    'materialization_id', $4::uuid,
    'index_generation_id', $12::uuid,
    'document_id', $9::uuid,
    'document_version_id', $10::uuid,
    'ordinal', 0,
    'chunk_profile_hash', $7,
    'source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
    'content_hash', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff',
    'content', 'Parse projection fixture parent chunk.',
    'token_count', 8,
    'heading_path', ARRAY['G7.5.19']::text[],
    'locator_summary', jsonb_build_object('kind','line_range','startLine',0,'endLine',0)
  )),
  jsonb_build_array(jsonb_build_object(
    'id', $13::uuid,
    'parent_chunk_id', $11::uuid,
    'materialization_id', $4::uuid,
    'index_generation_id', $12::uuid,
    'document_id', $9::uuid,
    'document_version_id', $10::uuid,
    'ordinal', 0,
    'chunk_profile_hash', $7,
    'source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
    'content_hash', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff',
    'content', 'Parse projection fixture child chunk.',
    'token_count', 8,
    'overlap_before_tokens', 0,
    'overlap_after_tokens', 0
  )),
  jsonb_build_array(
    jsonb_build_object(
      'chunk_kind', 'parent',
      'chunk_id', $11::uuid,
      'block_id', $8::uuid,
      'span_ordinal', 0,
      'start_offset', 0,
      'end_offset', 36,
      'fragment_source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee'
    ),
    jsonb_build_object(
      'chunk_kind', 'child',
      'chunk_id', $13::uuid,
      'block_id', $8::uuid,
      'span_ordinal', 0,
      'start_offset', 0,
      'end_offset', 35,
      'fragment_source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee'
    )
  ),
  jsonb_build_array(jsonb_build_object(
    'child_chunk_id', $13::uuid,
    'parent_chunk_id', $11::uuid,
    'materialization_id', $4::uuid,
    'index_generation_id', $12::uuid,
    'collection_id', $14::uuid,
    'document_id', $9::uuid,
    'document_version_id', $10::uuid,
    'embedding_model_id', 'jina-embeddings-v4',
    'embedding_dimensions', 1024,
    'lexical_text', 'Parse projection fixture child chunk.',
    'exact_terms', ARRAY['parse','projection']::text[],
    'source_span_hash', 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
    'chunk_profile_hash', $7,
    'content_hash', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff',
    'locator_summary', jsonb_build_object('kind','line_range','startLine',0,'endLine',0)
  ))
)`
}

func assertPhase15RAGParseProjectionRows(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGParseProjectionFixture,
) {
	t.Helper()

	var parseArtifactSetID string
	if err := db.QueryRowContext(ctx, `
SELECT parse_artifact_set_id
FROM knowledge_document_materializations
WHERE id = $1`, fixture.materializationID).Scan(&parseArtifactSetID); err != nil {
		t.Fatalf("read materialization parse artifact set: %v", err)
	}
	if parseArtifactSetID != fixture.artifactSetID {
		t.Fatalf("parse_artifact_set_id = %s, want %s", parseArtifactSetID, fixture.artifactSetID)
	}

	var artifactSets, blocks, parents, children, spans, searches int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM knowledge_parser_artifact_sets WHERE id = $1)::INTEGER,
  (SELECT count(*) FROM knowledge_blocks WHERE artifact_set_id = $1)::INTEGER,
  (SELECT count(*) FROM knowledge_parent_chunks WHERE materialization_id = $2)::INTEGER,
  (SELECT count(*) FROM knowledge_child_chunks WHERE materialization_id = $2)::INTEGER,
  (SELECT count(*) FROM knowledge_chunk_block_spans WHERE block_id = $3)::INTEGER,
  (SELECT count(*) FROM knowledge_child_search_projections WHERE materialization_id = $2)::INTEGER`,
		fixture.artifactSetID,
		fixture.materializationID,
		fixture.blockID,
	).Scan(&artifactSets, &blocks, &parents, &children, &spans, &searches); err != nil {
		t.Fatalf("read staged projection row counts: %v", err)
	}
	if artifactSets != 1 || blocks != 1 || parents != 1 || children != 1 || spans != 2 || searches != 1 {
		t.Fatalf(
			"staged counts artifact=%d block=%d parent=%d child=%d span=%d search=%d, want 1/1/1/1/2/1",
			artifactSets,
			blocks,
			parents,
			children,
			spans,
			searches,
		)
	}

	var searchStatus, embeddingModel string
	var embeddingDimensions int
	if err := db.QueryRowContext(ctx, `
SELECT status, embedding_model_id, embedding_dimensions
FROM knowledge_child_search_projections
WHERE child_chunk_id = $1`, fixture.childChunkID).Scan(
		&searchStatus,
		&embeddingModel,
		&embeddingDimensions,
	); err != nil {
		t.Fatalf("read staged search projection: %v", err)
	}
	if searchStatus != "staging" || embeddingModel != "jina-embeddings-v4" || embeddingDimensions != 1024 {
		t.Fatalf(
			"search projection = %s/%s/%d, want staging/jina-embeddings-v4/1024",
			searchStatus,
			embeddingModel,
			embeddingDimensions,
		)
	}
}

func assertPhase15RAGParseGatewayWorkerBoundary(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGParseProjectionFixture,
) {
	t.Helper()

	if err := execPhase15AsRoleWithArgs(ctx, db, "rag_worker_executor",
		phase15RAGParseStageSQL(),
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.materializationID,
		fixture.artifactSetID,
		fixture.sourceSHA256,
		fixture.chunkProfileHash,
		fixture.blockID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.parentChunkID,
		fixture.indexGenerationID,
		fixture.childChunkID,
		fixture.collectionID,
	); err != nil {
		t.Fatalf("rag_worker_executor execute parse projection gateway: %v", err)
	}

	err := execPhase15AsRoleWithArgs(ctx, db, "rag_worker_executor", `
UPDATE knowledge_child_search_projections
SET status = 'ready'
WHERE child_chunk_id = $1`, fixture.childChunkID)
	if !phase15InsufficientPrivilege(err) {
		t.Fatalf("rag_worker_executor base-table DML error = %v, want 42501", err)
	}
}

func assertPhase15RAGParseGatewayStaleLease(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGParseProjectionFixture,
) {
	t.Helper()

	var staged bool
	err := db.QueryRowContext(ctx, phase15RAGParseStageSQL(),
		fixture.jobID,
		fixture.workerID,
		fixture.staleLeaseToken,
		fixture.materializationID,
		fixture.artifactSetID,
		fixture.sourceSHA256,
		fixture.chunkProfileHash,
		fixture.blockID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.parentChunkID,
		fixture.indexGenerationID,
		fixture.childChunkID,
		fixture.collectionID,
	).Scan(&staged)
	assertPhase15RAGParseGatewayPGError(t, err, "RAG_STALE_JOB_LEASE")
}

func assertPhase15RAGParseGatewayProfileMismatch(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGParseProjectionFixture,
) {
	t.Helper()

	var staged bool
	err := db.QueryRowContext(ctx, phase15RAGParseStageSQL(),
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.materializationID,
		fixture.artifactSetID,
		fixture.sourceSHA256,
		"9999999999999999999999999999999999999999999999999999999999999999",
		fixture.blockID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.parentChunkID,
		fixture.indexGenerationID,
		fixture.childChunkID,
		fixture.collectionID,
	).Scan(&staged)
	assertPhase15RAGParseGatewayPGError(t, err, "RAG_PARSE_PROJECTION_PROFILE_MISMATCH")
}

func assertPhase15RAGParseGatewayPGError(
	t *testing.T,
	err error,
	message string,
) {
	t.Helper()
	assertPhase15RAGPurgeGatewayPGError(t, err, message)
}
