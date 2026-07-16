package migration

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

type phase15RAGPurgeProjectionFixture struct {
	userID                    string
	fileID                    string
	collectionID              string
	documentID                string
	documentVersionID         string
	indexProfileID            string
	indexGenerationID         string
	materializationID         string
	searchProfileID           string
	parentChunkID             string
	childChunkID              string
	jobID                     string
	workerID                  string
	leaseToken                string
	staleLeaseToken           string
	collectionVisibilityEpoch int64
	documentVisibilityEpoch   int64
}

func TestPhase15RAGPurgeProjectionGatewayLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	assertPhase15Postgres16(t, ctx, db)

	runner := NewRunner(db, migrationfiles.FS)
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("fresh up through purge projection gateway: %v", err)
	}
	if len(applied) < 14 || applied[13].Version != 14 {
		t.Fatalf("fresh applied = %#v, want migration 014 present", applied)
	}

	fixture := seedPhase15RAGPurgeProjectionFixture(t, ctx, db)
	assertPhase15RAGPurgeGatewayWorkerBoundary(t, ctx, db, fixture)
	assertPhase15RAGPurgeGatewayStaleLease(t, ctx, db, fixture)

	var queryVisible bool
	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT query_visible
FROM knowledge_mark_purge_invisible(
  $1, $2, $3, $4, $5, $6, $7, $8
)`,
		&queryVisible,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
	)
	if !queryVisible {
		t.Fatal("active document was not query-visible before tombstone fence")
	}

	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_documents
SET status = 'tombstoned',
    deleted_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE id = $1`, fixture.documentID)

	mustQueryPhase15RAGRow(t, ctx, db, `
SELECT query_visible
FROM knowledge_mark_purge_invisible(
  $1, $2, $3, $4, $5, $6, $7, $8
)`,
		&queryVisible,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
	)
	if queryVisible {
		t.Fatal("tombstoned document stayed query-visible through purge gateway")
	}

	assertErr := mustExecPhase151CReturnError(ctx, db, `
SELECT knowledge_assert_purge_complete(
  $1, $2, $3, $4, $5, $6, $7, $8, 0
)`,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.indexGenerationID,
		fixture.materializationID,
	)
	assertPhase15RAGPurgeGatewayPGError(
		t,
		assertErr,
		"RAG_PURGE_PROJECTION_INCOMPLETE",
	)

	var purgedRows, remainingReadyRows int
	if err := db.QueryRowContext(ctx, `
SELECT purged_child_search_rows, remaining_ready_child_search_rows
FROM knowledge_purge_search_projection(
  $1, $2, $3, $4, $5, $6, $7, $8
)`,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.indexGenerationID,
		fixture.materializationID,
	).Scan(&purgedRows, &remainingReadyRows); err != nil {
		t.Fatalf("purge search projection: %v", err)
	}
	if purgedRows != 1 || remainingReadyRows != 0 {
		t.Fatalf(
			"purge result purged=%d remaining_ready=%d, want 1/0",
			purgedRows,
			remainingReadyRows,
		)
	}

	var purgedCount, readyCount, purgedAtCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  count(*) FILTER (WHERE status = 'purged')::INTEGER,
  count(*) FILTER (WHERE status = 'ready')::INTEGER,
  count(*) FILTER (WHERE purged_at IS NOT NULL)::INTEGER
FROM knowledge_child_search_projections
WHERE materialization_id = $1`, fixture.materializationID,
	).Scan(&purgedCount, &readyCount, &purgedAtCount); err != nil {
		t.Fatalf("query purged search projection state: %v", err)
	}
	if purgedCount != 1 || readyCount != 0 || purgedAtCount != 1 {
		t.Fatalf(
			"search projection state purged=%d ready=%d purged_at=%d, want 1/0/1",
			purgedCount,
			readyCount,
			purgedAtCount,
		)
	}

	var complete bool
	if err := db.QueryRowContext(ctx, `
SELECT knowledge_assert_purge_complete(
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)`,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.indexGenerationID,
		fixture.materializationID,
		purgedRows,
	).Scan(&complete); err != nil {
		t.Fatalf("assert purge complete: %v", err)
	}
	if !complete {
		t.Fatal("purge completion returned false")
	}
}

func seedPhase15RAGPurgeProjectionFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) phase15RAGPurgeProjectionFixture {
	t.Helper()

	fixture := phase15RAGPurgeProjectionFixture{
		userID:                    "00000000-0000-0000-0000-0000000a1001",
		fileID:                    "00000000-0000-0000-0000-0000000a1002",
		collectionID:              "00000000-0000-0000-0000-0000000a1003",
		documentID:                "00000000-0000-0000-0000-0000000a1004",
		documentVersionID:         "00000000-0000-0000-0000-0000000a1005",
		indexProfileID:            "00000000-0000-0000-0000-0000000a1006",
		indexGenerationID:         "00000000-0000-0000-0000-0000000a1007",
		materializationID:         "00000000-0000-0000-0000-0000000a1008",
		searchProfileID:           "00000000-0000-0000-0000-0000000a1009",
		parentChunkID:             "00000000-0000-0000-0000-0000000a1010",
		childChunkID:              "00000000-0000-0000-0000-0000000a1011",
		jobID:                     "00000000-0000-0000-0000-0000000a1012",
		workerID:                  "00000000-0000-0000-0000-0000000a1013",
		leaseToken:                "00000000-0000-0000-0000-0000000a1014",
		staleLeaseToken:           "00000000-0000-0000-0000-0000000a1015",
		collectionVisibilityEpoch: 1,
		documentVisibilityEpoch:   1,
	}

	const (
		hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		hashC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		hashD = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		hashE = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		hashF = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		hash0 = "0000000000000000000000000000000000000000000000000000000000000000"
	)

	mustExecPhase151C(t, ctx, db, `
INSERT INTO users (id, email, display_name)
VALUES ($1, 'g7510-purge@example.test', 'G7.5.10 Purge')`,
		fixture.userID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES (
  $1, $2, 'g7-5-10.pdf', 'application/pdf', 16, $3,
  'knowledge/g7-5-10.pdf'
)`,
		fixture.fileID,
		fixture.userID,
		hashA,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_collections (
  id, name, scope, owner_user_id, created_by_user_id
) VALUES ($1, 'G7.5.10 Purge Gateway', 'personal', $2, $2)`,
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
) VALUES ($1, $2, $3, 1, $4, 'active', $5, $6)`,
		fixture.documentVersionID,
		fixture.documentID,
		fixture.fileID,
		fixture.documentVisibilityEpoch,
		hashB,
		fixture.userID,
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_documents
SET current_version_id = $2,
    status = 'active',
    updated_at = clock_timestamp()
WHERE id = $1`,
		fixture.documentID,
		fixture.documentVersionID,
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
  'jina', 'admin-env', 'jina-embeddings-v4', 'v1', 'passage',
  'jina', 'admin-env', 'jina-reranker-v3', 'v1', $4
)`,
		fixture.indexProfileID,
		hashC,
		hashD,
		hashE,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_index_generations (
  id, index_profile_id, generation_seq, status, build_snapshot,
  build_snapshot_hash, artifact_manifest_hash, verified_at, activated_at
) VALUES ($1, $2, 1, 'active', '{}'::jsonb, $3, $4, clock_timestamp(), clock_timestamp())`,
		fixture.indexGenerationID,
		fixture.indexProfileID,
		hashA,
		hashB,
	)
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_corpus_projection_head
SET active_index_generation_id = $1,
    corpus_projection_revision = 2,
    head_revision = 2,
    updated_at = clock_timestamp()
WHERE singleton_id = 1`,
		fixture.indexGenerationID,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_projection_state (
  index_generation_id, readiness, projection_revision, required_outbox_floor,
  contiguous_applied_outbox_id, manifest_hash, document_count, parent_count,
  child_count, verified_at
) VALUES ($1, 'ready', 1, 0, 0, $2, 1, 1, 1, clock_timestamp())`,
		fixture.indexGenerationID,
		hashC,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
) VALUES (
  $1, $2, $3, $4, $5, $6, 1, $7, $8,
  1, $9, 1, $10, 'published', $11, $12, clock_timestamp(), clock_timestamp()
)`,
		fixture.materializationID,
		fixture.indexGenerationID,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.fileID,
		hashB,
		hashE,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
		hashD,
		hashF,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_document_projection_heads (
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES ($1, $2, $3, 1)`,
		fixture.indexGenerationID,
		fixture.documentID,
		fixture.materializationID,
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
INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
) VALUES (
  $1, $2, $3, $4, $5, 0, $6, $7, $8,
  'Parent chunk for purge gateway integration.', 8,
  ARRAY['G7.5.10']::TEXT[], '{"kind":"page","page":1}'::jsonb
)`,
		fixture.parentChunkID,
		fixture.materializationID,
		fixture.indexGenerationID,
		fixture.documentID,
		fixture.documentVersionID,
		hashD,
		hashE,
		hashF,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES (
  $1, $2, $3, $4, $5, $6, 0, $7, $8, $9,
  'Child chunk for purge gateway integration.', 8
)`,
		fixture.childChunkID,
		fixture.parentChunkID,
		fixture.materializationID,
		fixture.indexGenerationID,
		fixture.documentID,
		fixture.documentVersionID,
		hashD,
		hashE,
		hashF,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8,
  'jina-embeddings-v4', 1024, array_fill(0.001::real, ARRAY[1024]),
  $9, 'Child chunk for purge gateway integration.',
  ARRAY['purge','gateway']::TEXT[], $10, $11, $12,
  '{"kind":"page","page":1}'::jsonb, 'ready', clock_timestamp()
)`,
		fixture.childChunkID,
		fixture.parentChunkID,
		fixture.materializationID,
		fixture.indexGenerationID,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.searchProfileID,
		hashA,
		hashE,
		hashD,
		hashF,
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO knowledge_processing_jobs (
  id, collection_id, document_id, document_version_id, file_id, stage,
  operation, collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch,
  requested_by_user_id, idempotency_scope, idempotency_key, request_hash,
  status, attempt_count, max_attempts, available_at, lease_owner,
  lease_token, lease_expires_at, index_generation_id, materialization_id,
  legacy_projection_unbound
) VALUES (
  $1, $2, $3, $4, $5, 'purge', 'purge', 1, $6, 1, $7,
  $8, 'g7.5.10-purge-gateway', 'purge-search-projection', $9,
  'processing', 1, 3, clock_timestamp(), $10, $11,
  clock_timestamp() + interval '15 minutes', $12, $13, false
)`,
		fixture.jobID,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.fileID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
		fixture.userID,
		hashB,
		fixture.workerID,
		fixture.leaseToken,
		fixture.indexGenerationID,
		fixture.materializationID,
	)

	return fixture
}

func assertPhase15RAGPurgeGatewayWorkerBoundary(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGPurgeProjectionFixture,
) {
	t.Helper()

	if err := execPhase15AsRoleWithArgs(ctx, db, "rag_worker_executor", `
SELECT query_visible
FROM knowledge_mark_purge_invisible(
  $1, $2, $3, $4, $5, $6, $7, $8
)`,
		fixture.jobID,
		fixture.workerID,
		fixture.leaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
	); err != nil {
		t.Fatalf("rag_worker_executor execute purge gateway: %v", err)
	}

	err := execPhase15AsRoleWithArgs(ctx, db, "rag_worker_executor", `
UPDATE knowledge_child_search_projections
SET status = 'purged'
WHERE child_chunk_id = $1`, fixture.childChunkID)
	if !phase15InsufficientPrivilege(err) {
		t.Fatalf("rag_worker_executor base-table DML error = %v, want 42501", err)
	}
}

func assertPhase15RAGPurgeGatewayStaleLease(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture phase15RAGPurgeProjectionFixture,
) {
	t.Helper()

	var queryVisible bool
	err := db.QueryRowContext(ctx, `
SELECT query_visible
FROM knowledge_mark_purge_invisible(
  $1, $2, $3, $4, $5, $6, $7, $8
)`,
		fixture.jobID,
		fixture.workerID,
		fixture.staleLeaseToken,
		fixture.collectionID,
		fixture.documentID,
		fixture.documentVersionID,
		fixture.collectionVisibilityEpoch,
		fixture.documentVisibilityEpoch,
	).Scan(&queryVisible)
	assertPhase15RAGPurgeGatewayPGError(t, err, "RAG_STALE_JOB_LEASE")
}

func execPhase15AsRoleWithArgs(
	ctx context.Context,
	db *sql.DB,
	role string,
	query string,
	args ...any,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	quotedRole := `"` + strings.ReplaceAll(role, `"`, `""`) + `"`
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE `+quotedRole); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func assertPhase15RAGPurgeGatewayPGError(
	t *testing.T,
	err error,
	message string,
) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected Postgres error %q, got nil", message)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Message != message {
			t.Fatalf("Postgres error message = %q, want %q", pgErr.Message, message)
		}
		return
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("error = %v, want message %q", err, message)
	}
}
