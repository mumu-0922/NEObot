from __future__ import annotations

import hashlib
import os
import uuid
from dataclasses import dataclass
from decimal import Decimal
from typing import Final, cast

import psycopg
import pytest

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.offline_parser.canonical import JsonObject
from mm_chat_rag.postgres import PostgresAdapter
from mm_chat_rag.projection import (
    BlockProjectionRow,
    ChildChunkProjectionRow,
    ChildSearchProjectionRow,
    ChunkBlockSpanProjectionRow,
    ParentChunkProjectionRow,
    PostgresProjectionBatch,
)
from mm_chat_rag.settings import Settings

pytestmark = pytest.mark.integration


_MODEL_ID: Final = "mineru-parser-v20260716"
_ENDPOINT_ID: Final = "hosted-main"
_SOURCE_SHA256: Final = "b" * 64


@dataclass(frozen=True, slots=True)
class ParseProjectionFixture:
    user_id: uuid.UUID
    file_id: uuid.UUID
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    governance_profile_id: uuid.UUID
    consent_id: uuid.UUID
    index_profile_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID
    search_profile_id: uuid.UUID
    artifact_set_id: uuid.UUID
    block_id: uuid.UUID
    parent_chunk_id: uuid.UUID
    child_chunk_id: uuid.UUID
    job_id: uuid.UUID
    worker_id: uuid.UUID
    lease_token: uuid.UUID
    request_hash: str
    source_sha256: str
    chunk_profile_hash: str
    base_profile_hash: str
    generation_seq: int


def database_url() -> str:
    value = os.environ.get("RAG_TEST_DATABASE_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_DATABASE_URL is not set")
    return value


def _hash_hex(label: str, seed: uuid.UUID) -> str:
    return hashlib.sha256(f"{label}:{seed}".encode()).hexdigest()


def _fixture() -> ParseProjectionFixture:
    seed = uuid.uuid4()
    return ParseProjectionFixture(
        user_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        governance_profile_id=uuid.uuid4(),
        consent_id=uuid.uuid4(),
        index_profile_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        search_profile_id=uuid.uuid4(),
        artifact_set_id=uuid.uuid4(),
        block_id=uuid.uuid4(),
        parent_chunk_id=uuid.uuid4(),
        child_chunk_id=uuid.uuid4(),
        job_id=uuid.uuid4(),
        worker_id=uuid.uuid4(),
        lease_token=uuid.uuid4(),
        request_hash=_hash_hex("request", seed),
        source_sha256=_SOURCE_SHA256,
        chunk_profile_hash=_hash_hex("chunk-profile", seed),
        base_profile_hash=_hash_hex("base-profile", seed),
        generation_seq=1 + seed.int % 9_000_000_000_000_000,
    )


def _context(fixture: ParseProjectionFixture) -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=fixture.job_id,
        stage="parse",
        operation="initial",
        collection_id=fixture.collection_id,
        document_id=fixture.document_id,
        document_version_id=fixture.document_version_id,
        file_id=fixture.file_id,
        index_generation_id=fixture.index_generation_id,
        materialization_id=fixture.materialization_id,
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash=fixture.request_hash,
        authority=None,
        lease_token=fixture.lease_token,
    )


def _batch(fixture: ParseProjectionFixture) -> PostgresProjectionBatch:
    source_span_hash = _hash_hex("source-span", fixture.block_id)
    content_hash = _hash_hex("content", fixture.block_id)
    locator = cast("JsonObject", {"kind": "text_offset", "start": 0, "end": 13})
    locator_summary = cast(
        "JsonObject",
        {
            "schemaVersion": "g7.4-locator-summary.v1",
            "primary": {"kind": "text_offset", "locator": locator},
            "fragments": [],
            "locatorAggregateHashes": [],
        },
    )
    return PostgresProjectionBatch(
        blocks=(
            BlockProjectionRow(
                id=fixture.block_id,
                artifact_set_id=fixture.artifact_set_id,
                document_id=fixture.document_id,
                document_version_id=fixture.document_version_id,
                parent_block_id=None,
                ordinal=0,
                block_type="paragraph",
                heading_path=("G7.5C",),
                text_content="hello fixture",
                locator_kind="text_offset",
                locator=locator,
                reading_order=0,
                provenance={"sourceSpanHash": source_span_hash},
                confidence=Decimal("0.9"),
                content_hash=content_hash,
                source_span_hash=source_span_hash,
                derived=False,
                non_indexable=False,
                needs_review=False,
                logical_block_id=_hash_hex("block", fixture.block_id),
            ),
        ),
        parent_chunks=(
            ParentChunkProjectionRow(
                id=fixture.parent_chunk_id,
                materialization_id=fixture.materialization_id,
                index_generation_id=fixture.index_generation_id,
                document_id=fixture.document_id,
                document_version_id=fixture.document_version_id,
                ordinal=0,
                chunk_profile_hash=fixture.chunk_profile_hash,
                source_span_hash=source_span_hash,
                content_hash=content_hash,
                content="hello fixture",
                token_count=2,
                heading_path=("G7.5C",),
                locator_summary=locator_summary,
                logical_chunk_id=_hash_hex("parent", fixture.parent_chunk_id),
            ),
        ),
        child_chunks=(
            ChildChunkProjectionRow(
                id=fixture.child_chunk_id,
                parent_chunk_id=fixture.parent_chunk_id,
                materialization_id=fixture.materialization_id,
                index_generation_id=fixture.index_generation_id,
                document_id=fixture.document_id,
                document_version_id=fixture.document_version_id,
                ordinal=0,
                chunk_profile_hash=fixture.chunk_profile_hash,
                source_span_hash=source_span_hash,
                content_hash=content_hash,
                content="hello fixture",
                token_count=2,
                overlap_before_tokens=0,
                overlap_after_tokens=0,
                logical_chunk_id=_hash_hex("child", fixture.child_chunk_id),
            ),
        ),
        chunk_block_spans=(
            ChunkBlockSpanProjectionRow(
                chunk_kind="child",
                chunk_id=fixture.child_chunk_id,
                block_id=fixture.block_id,
                span_ordinal=0,
                start_offset=0,
                end_offset=13,
                fragment_source_span_hash=source_span_hash,
            ),
        ),
        child_search_projections=(
            ChildSearchProjectionRow(
                child_chunk_id=fixture.child_chunk_id,
                parent_chunk_id=fixture.parent_chunk_id,
                materialization_id=fixture.materialization_id,
                index_generation_id=fixture.index_generation_id,
                collection_id=fixture.collection_id,
                document_id=fixture.document_id,
                document_version_id=fixture.document_version_id,
                embedding_model_id="jina-embeddings-v4",
                embedding_dimensions=1024,
                lexical_text="hello fixture",
                exact_terms=("fixture", "hello"),
                source_span_hash=source_span_hash,
                chunk_profile_hash=fixture.chunk_profile_hash,
                content_hash=content_hash,
                locator_summary=locator_summary,
            ),
        ),
        source_sha256=fixture.source_sha256,
        chunk_profile_hash=fixture.chunk_profile_hash,
    )


async def test_stage_parse_projection_hits_live_017_function() -> None:
    url = database_url()
    fixture = _fixture()
    await _seed_fixture(url, fixture)

    adapter = PostgresAdapter(
        Settings(database_url=url, worker_id=fixture.worker_id), Metrics.create()
    )
    await adapter.open()
    try:
        await adapter.stage_parse_projection(_context(fixture), _batch(fixture))
    finally:
        await adapter.close()

    assert await _projection_counts(url, fixture) == (1, 1, 1, 1, 1, 1)


async def _seed_fixture(url: str, fixture: ParseProjectionFixture) -> None:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        await connection.execute(
            "INSERT INTO users (id, email, display_name) VALUES (%s, %s, %s)",
            (fixture.user_id, f"{fixture.user_id}@example.test", "RAG Parse"),
        )
        await connection.execute(
            """
            INSERT INTO files (
              id, user_id, original_filename, mime_type, byte_size, sha256, object_key
            ) VALUES (%s, %s, 'python-g7-5c.pdf', 'application/pdf', 16, %s, %s)
            """,
            (
                fixture.file_id,
                fixture.user_id,
                fixture.source_sha256,
                f"knowledge/{fixture.file_id}.pdf",
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_collections (
              id, name, scope, owner_user_id, created_by_user_id
            ) VALUES (%s, 'Python G7.5C Parse Gateway', 'personal', %s, %s)
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
            ) VALUES (%s, %s, %s, 1, 1, 'processing', %s, %s)
            """,
            (
                fixture.document_version_id,
                fixture.document_id,
                fixture.file_id,
                fixture.source_sha256,
                fixture.user_id,
            ),
        )
        await connection.execute(
            """
            INSERT INTO processor_governance_profiles (
              id, processor, endpoint_id, model_id, model_api_version,
              profile_contract_hash, allowed_purposes, allowed_data_types, region,
              retention_policy, deletion_contract, training_use, status,
              governance_revision, manifest_hash
            ) VALUES (
              %s, 'mineru', %s, %s, 'api-20260623', %s,
              ARRAY['parse'], ARRAY['application/pdf'], 'global', 'none', 'delete',
              'disabled', 'approved', 1, %s
            )
            """,
            (
                fixture.governance_profile_id,
                _ENDPOINT_ID,
                _MODEL_ID,
                _hash_hex("profile-contract", fixture.governance_profile_id),
                _hash_hex("manifest", fixture.governance_profile_id),
            ),
        )
        await connection.execute(
            """
            INSERT INTO processor_governance_heads (
              processor, endpoint_id, model_id, status, active_profile_id,
              active_governance_revision, head_revision
            ) VALUES ('mineru', %s, %s, 'active', %s, 1, 1)
            """,
            (_ENDPOINT_ID, _MODEL_ID, fixture.governance_profile_id),
        )
        await connection.execute(
            """
            INSERT INTO processing_consents (
              id, scope, collection_id, processor, endpoint_id, model_id,
              governance_profile_id, governance_revision, governance_head_revision,
              purposes, data_types, policy_version, decision, consent_revision,
              granted_by_user_id
            ) VALUES (
              %s, 'collection', %s, 'mineru', %s, %s, %s, 1, 1,
              ARRAY['parse'], ARRAY['application/pdf'], 'g7.5b-python',
              'granted', 1, %s
            )
            """,
            (
                fixture.consent_id,
                fixture.collection_id,
                _ENDPOINT_ID,
                _MODEL_ID,
                fixture.governance_profile_id,
                fixture.user_id,
            ),
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
            (
                fixture.index_profile_id,
                _hash_hex("parser-manifest", fixture.index_profile_id),
                fixture.chunk_profile_hash,
                fixture.base_profile_hash,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_index_generations (
              id, index_profile_id, generation_seq, status, build_snapshot,
              build_snapshot_hash
            ) VALUES (%s, %s, %s, 'building', '{}'::jsonb, %s)
            """,
            (
                fixture.index_generation_id,
                fixture.index_profile_id,
                fixture.generation_seq,
                _hash_hex("build", fixture.index_generation_id),
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_search_profiles (
              id, index_profile_id, provider_profile_id, embedding_processor,
              embedding_model_id, embedding_dimensions, rerank_processor,
              rerank_model_id, lexical_config, exact_config, profile_hash
            ) VALUES (
              %s, %s, 'mineru_jina_postgres_v1', 'jina', 'jina-embeddings-v4',
              1024, 'jina', 'jina-reranker-v3', '{}'::jsonb, '{}'::jsonb, %s
            )
            """,
            (
                fixture.search_profile_id,
                fixture.index_profile_id,
                _hash_hex("search-profile", fixture.search_profile_id),
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_document_materializations (
              id, index_generation_id, collection_id, document_id,
              document_version_id, file_id, materialization_seq,
              source_content_hash, base_profile_hash, collection_acl_revision,
              collection_visibility_epoch, collection_processing_revision,
              document_visibility_epoch, status
            ) VALUES (%s, %s, %s, %s, %s, %s, 1, %s, %s, 1, 1, 1, 1, 'staging')
            """,
            (
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                fixture.source_sha256,
                fixture.base_profile_hash,
            ),
        )
        await connection.execute(
            """
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
              %s, %s, %s, %s, %s, 'parse', 'initial', 'mineru', %s, %s,
              %s, 1, 1, %s, 1, 1, 1, 1, 1, %s, 'g7.5b-python',
              'stage-parse-projection', %s, 'processing', 1, 3,
              clock_timestamp(), %s, %s, clock_timestamp() + interval '15 minutes',
              %s, %s, false
            )
            """,
            (
                fixture.job_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                _ENDPOINT_ID,
                _MODEL_ID,
                fixture.governance_profile_id,
                fixture.consent_id,
                fixture.user_id,
                fixture.request_hash,
                fixture.worker_id,
                fixture.lease_token,
                fixture.index_generation_id,
                fixture.materialization_id,
            ),
        )
        await connection.commit()


async def _projection_counts(
    url: str, fixture: ParseProjectionFixture
) -> tuple[int, int, int, int, int, int]:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        cursor = await connection.execute(
            """
            SELECT
              (
                SELECT count(*) FROM knowledge_parser_artifact_sets
                WHERE id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_blocks
                WHERE artifact_set_id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_parent_chunks
                WHERE materialization_id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_child_chunks
                WHERE materialization_id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_chunk_block_spans
                WHERE block_id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_child_search_projections
                WHERE materialization_id = %s
              )::INTEGER
            """,
            (
                fixture.artifact_set_id,
                fixture.artifact_set_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.block_id,
                fixture.materialization_id,
            ),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("projection count query returned no row")
    values = tuple(int(value) for value in row)
    if len(values) != 6:
        raise AssertionError(f"projection count query returned {len(values)} columns")
    return values
