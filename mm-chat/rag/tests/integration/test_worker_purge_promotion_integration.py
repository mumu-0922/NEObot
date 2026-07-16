from __future__ import annotations

import hashlib
import os
import uuid
from dataclasses import dataclass
from typing import Final

import psycopg
import pytest

from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.settings import Settings
from mm_chat_rag.worker import Worker

pytestmark = pytest.mark.integration


_HASH_A: Final = "a" * 64
_HASH_B: Final = "b" * 64
_HASH_C: Final = "c" * 64
_HASH_D: Final = "d" * 64
_HASH_E: Final = "e" * 64
_HASH_F: Final = "f" * 64
_HASH_0: Final = "0" * 64


@dataclass(frozen=True, slots=True)
class PurgePromotionFixture:
    user_id: uuid.UUID
    file_id: uuid.UUID
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    index_profile_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID
    search_profile_id: uuid.UUID
    parent_chunk_id: uuid.UUID
    child_chunk_id: uuid.UUID
    job_id: uuid.UUID
    worker_id: uuid.UUID
    request_hash: str


def database_url() -> str:
    value = os.environ.get("RAG_TEST_DATABASE_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_DATABASE_URL is not set")
    return value


def _hash_hex(label: str, seed: uuid.UUID) -> str:
    return hashlib.sha256(f"{label}:{seed}".encode()).hexdigest()


def _fixture() -> PurgePromotionFixture:
    seed = uuid.uuid4()
    return PurgePromotionFixture(
        user_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        index_profile_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        search_profile_id=uuid.uuid4(),
        parent_chunk_id=uuid.uuid4(),
        child_chunk_id=uuid.uuid4(),
        job_id=uuid.uuid4(),
        worker_id=uuid.uuid4(),
        request_hash=_hash_hex("purge-promotion", seed),
    )


async def test_promoted_purge_job_runner_finishes_live_postgres_job() -> None:
    url = database_url()
    fixture = _fixture()
    await _seed_fixture(url, fixture)

    settings = Settings(
        database_url=url,
        dispatch_enabled=True,
        job_stages=("purge",),
        worker_id=fixture.worker_id,
    )
    worker = Worker(settings)
    worker.validate_promotion_gate()
    assert set(worker.job_handlers) == {"purge"}

    await worker.database.open()
    try:
        runner = JobRunner(
            worker.database,
            worker.settings,
            worker.metrics,
            worker.job_handlers,
        )
        assert await runner.process_one()
        assert not await runner.process_one()
    finally:
        await worker.database.close()

    assert await _job_projection_state(url, fixture) == (
        "succeeded",
        None,
        1,
        True,
        True,
        1,
        0,
        1,
    )


async def _seed_fixture(url: str, fixture: PurgePromotionFixture) -> None:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        await connection.execute(
            "INSERT INTO users (id, email, display_name) VALUES (%s, %s, %s)",
            (fixture.user_id, f"{fixture.user_id}@example.test", "G7.5F Purge"),
        )
        await connection.execute(
            """
            INSERT INTO files (
              id, user_id, original_filename, mime_type, byte_size, sha256,
              object_key
            ) VALUES (
              %s, %s, 'g7-5f.pdf', 'application/pdf', 16, %s,
              'knowledge/g7-5f.pdf'
            )
            """,
            (fixture.file_id, fixture.user_id, _HASH_A),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_collections (
              id, name, scope, owner_user_id, created_by_user_id
            ) VALUES (%s, 'G7.5F Promoted Purge', 'personal', %s, %s)
            """,
            (fixture.collection_id, fixture.user_id, fixture.user_id),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_documents (
              id, collection_id, status, visibility_epoch, created_by_user_id
            ) VALUES (%s, %s, 'processing', 1, %s)
            """,
            (fixture.document_id, fixture.collection_id, fixture.user_id),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_document_versions (
              id, document_id, file_id, source_version, visibility_epoch, status,
              content_hash, created_by_user_id
            ) VALUES (%s, %s, %s, 1, 1, 'active', %s, %s)
            """,
            (
                fixture.document_version_id,
                fixture.document_id,
                fixture.file_id,
                _HASH_B,
                fixture.user_id,
            ),
        )
        await connection.execute(
            """
            UPDATE knowledge_documents
            SET current_version_id = %s,
                status = 'active',
                updated_at = clock_timestamp()
            WHERE id = %s
            """,
            (fixture.document_version_id, fixture.document_id),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_index_profiles (
              id, contract_version, canonical_schema_version, parser_manifest,
              parser_manifest_hash, chunk_manifest, chunk_profile_hash,
              embedding_processor, embedding_endpoint_id, embedding_model_id,
              embedding_api_version, embedding_role, rerank_processor,
              rerank_endpoint_id, rerank_model_id, rerank_api_version,
              base_profile_hash
            ) VALUES (
              %s, 1, 'canonical-ir-v2', '{}'::jsonb, %s, '{}'::jsonb, %s,
              'jina', 'admin-env', 'jina-embeddings-v4', 'api-20260623',
              'passage', 'jina', 'admin-env', 'jina-reranker-v3',
              'api-20260623', %s
            )
            """,
            (fixture.index_profile_id, _HASH_C, _HASH_D, _HASH_E),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_index_generations (
              id, index_profile_id, generation_seq, status, build_snapshot,
              build_snapshot_hash, artifact_manifest_hash, verified_at,
              activated_at
            ) VALUES (
              %s, %s, 1, 'active', '{}'::jsonb, %s, %s,
              clock_timestamp(), clock_timestamp()
            )
            """,
            (
                fixture.index_generation_id,
                fixture.index_profile_id,
                _HASH_A,
                _HASH_B,
            ),
        )
        await connection.execute(
            """
            UPDATE knowledge_corpus_projection_head
            SET active_index_generation_id = %s,
                corpus_projection_revision = 2,
                head_revision = 2,
                updated_at = clock_timestamp()
            WHERE singleton_id = 1
            """,
            (fixture.index_generation_id,),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_projection_state (
              index_generation_id, readiness, projection_revision,
              required_outbox_floor, contiguous_applied_outbox_id,
              manifest_hash, document_count, parent_count, child_count,
              verified_at
            ) VALUES (%s, 'ready', 1, 0, 0, %s, 1, 1, 1, clock_timestamp())
            """,
            (fixture.index_generation_id, _HASH_C),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_document_materializations (
              id, index_generation_id, collection_id, document_id,
              document_version_id, file_id, materialization_seq,
              source_content_hash, base_profile_hash, collection_acl_revision,
              collection_visibility_epoch, collection_processing_revision,
              document_visibility_epoch, status, manifest_hash, result_hash,
              verified_at, published_at
            ) VALUES (
              %s, %s, %s, %s, %s, %s, 1, %s, %s, 1, 1, 1, 1,
              'published', %s, %s, clock_timestamp(), clock_timestamp()
            )
            """,
            (
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                _HASH_B,
                _HASH_E,
                _HASH_D,
                _HASH_F,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_document_projection_heads (
              index_generation_id, document_id, active_materialization_id,
              last_corpus_projection_revision
            ) VALUES (%s, %s, %s, 1)
            """,
            (
                fixture.index_generation_id,
                fixture.document_id,
                fixture.materialization_id,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_search_profiles (
              id, index_profile_id, provider_profile_id, embedding_processor,
              embedding_model_id, embedding_dimensions, rerank_processor,
              rerank_model_id, lexical_config, exact_config, profile_hash
            ) VALUES (
              %s, %s, 'mineru_jina_postgres_v1', 'jina',
              'jina-embeddings-v4', 1024, 'jina', 'jina-reranker-v3',
              '{}'::jsonb, '{}'::jsonb, %s
            )
            """,
            (fixture.search_profile_id, fixture.index_profile_id, _HASH_0),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_parent_chunks (
              id, materialization_id, index_generation_id, document_id,
              document_version_id, ordinal, chunk_profile_hash, source_span_hash,
              content_hash, content, token_count, heading_path, locator_summary
            ) VALUES (
              %s, %s, %s, %s, %s, 0, %s, %s, %s,
              'Parent chunk for promoted purge smoke.', 6,
              ARRAY['G7.5F']::TEXT[], '{"kind":"page","page":1}'::jsonb
            )
            """,
            (
                fixture.parent_chunk_id,
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.document_id,
                fixture.document_version_id,
                _HASH_D,
                _HASH_E,
                _HASH_F,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_child_chunks (
              id, parent_chunk_id, materialization_id, index_generation_id,
              document_id, document_version_id, ordinal, chunk_profile_hash,
              source_span_hash, content_hash, content, token_count
            ) VALUES (
              %s, %s, %s, %s, %s, %s, 0, %s, %s, %s,
              'Child chunk for promoted purge smoke.', 6
            )
            """,
            (
                fixture.child_chunk_id,
                fixture.parent_chunk_id,
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.document_id,
                fixture.document_version_id,
                _HASH_D,
                _HASH_E,
                _HASH_F,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_child_search_projections (
              child_chunk_id, parent_chunk_id, materialization_id,
              index_generation_id, collection_id, document_id,
              document_version_id, search_profile_id, embedding_model_id,
              embedding_dimensions, embedding_vector, embedding_vector_sha256,
              lexical_text, exact_terms, source_span_hash, chunk_profile_hash,
              content_hash, locator_summary, status, ready_at
            ) VALUES (
              %s, %s, %s, %s, %s, %s, %s, %s,
              'jina-embeddings-v4', 1024, array_fill(0.001::real, ARRAY[1024]),
              %s, 'Child chunk for promoted purge smoke.',
              ARRAY['purge','promotion']::TEXT[], %s, %s, %s,
              '{"kind":"page","page":1}'::jsonb, 'ready', clock_timestamp()
            )
            """,
            (
                fixture.child_chunk_id,
                fixture.parent_chunk_id,
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.search_profile_id,
                _HASH_A,
                _HASH_E,
                _HASH_D,
                _HASH_F,
            ),
        )
        await connection.execute(
            """
            UPDATE knowledge_documents
            SET status = 'tombstoned',
                deleted_at = clock_timestamp(),
                updated_at = clock_timestamp()
            WHERE id = %s
            """,
            (fixture.document_id,),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_processing_jobs (
              id, collection_id, document_id, document_version_id, file_id,
              stage, operation, collection_acl_revision,
              collection_visibility_epoch, collection_processing_revision,
              document_visibility_epoch, requested_by_user_id,
              idempotency_scope, idempotency_key, request_hash, status,
              attempt_count, max_attempts, available_at, index_generation_id,
              materialization_id, legacy_projection_unbound
            ) VALUES (
              %s, %s, %s, %s, %s, 'purge', 'purge', 1, 1, 1, 1, %s,
              'g7.5f-purge-promotion', 'job-runner-smoke', %s, 'pending',
              0, 3, clock_timestamp(), %s, %s, false
            )
            """,
            (
                fixture.job_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                fixture.user_id,
                fixture.request_hash,
                fixture.index_generation_id,
                fixture.materialization_id,
            ),
        )
        await connection.commit()


async def _job_projection_state(
    url: str,
    fixture: PurgePromotionFixture,
) -> tuple[str, str | None, int, bool, bool, int, int, int]:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        cursor = await connection.execute(
            """
            SELECT
              (
                SELECT status FROM knowledge_processing_jobs WHERE id = %s
              )::TEXT,
              (
                SELECT error_code FROM knowledge_processing_jobs WHERE id = %s
              )::TEXT,
              (
                SELECT attempt_count FROM knowledge_processing_jobs WHERE id = %s
              )::INTEGER,
              (
                SELECT lease_owner IS NULL FROM knowledge_processing_jobs
                WHERE id = %s
              )::BOOLEAN,
              (
                SELECT completed_at IS NOT NULL FROM knowledge_processing_jobs
                WHERE id = %s
              )::BOOLEAN,
              (
                SELECT count(*) FROM knowledge_child_search_projections
                WHERE materialization_id = %s AND status = 'purged'
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_child_search_projections
                WHERE materialization_id = %s AND status = 'ready'
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_child_search_projections
                WHERE materialization_id = %s AND purged_at IS NOT NULL
              )::INTEGER
            """,
            (
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.materialization_id,
            ),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("job projection state query returned no row")
    return (
        str(row[0]),
        None if row[1] is None else str(row[1]),
        int(row[2]),
        bool(row[3]),
        bool(row[4]),
        int(row[5]),
        int(row[6]),
        int(row[7]),
    )
