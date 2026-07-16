from __future__ import annotations

import hashlib
import json
import os
import uuid
from dataclasses import dataclass
from typing import Final

import httpx
import psycopg
import pytest

import mm_chat_rag.worker as worker_module
from mm_chat_rag.jina_gateway import (
    JINA_EMBEDDINGS_URL,
    build_jina_passage_embedding_handler_dependencies,
)
from mm_chat_rag.job_handler_dependencies import (
    PassageEmbeddingHandlerDependencies,
    PassageEmbeddingProjectionGateway,
    embedding_vector_sha256,
)
from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
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
_ENDPOINT_ID: Final = "admin-env"
_JINA_API_KEY: Final = "unit-test-jina-key"


@dataclass(frozen=True, slots=True)
class EmbeddingPromotionFixture:
    user_id: uuid.UUID
    file_id: uuid.UUID
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    governance_profile_id: uuid.UUID
    consent_id: uuid.UUID
    index_profile_id: uuid.UUID
    index_generation_id: uuid.UUID
    artifact_set_id: uuid.UUID
    materialization_id: uuid.UUID
    search_profile_id: uuid.UUID
    parent_chunk_id: uuid.UUID
    child_chunk_id: uuid.UUID
    job_id: uuid.UUID
    worker_id: uuid.UUID
    request_hash: str
    chunk_text: str
    content_hash: str
    expected_vector_hash: str


def database_url() -> str:
    value = os.environ.get("RAG_TEST_DATABASE_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_DATABASE_URL is not set")
    return value


def _hash_hex(label: str, seed: uuid.UUID) -> str:
    return hashlib.sha256(f"{label}:{seed}".encode()).hexdigest()


def _embedding_vector() -> tuple[float, ...]:
    return tuple(
        0.125 + (index % 17) / 1000
        for index in range(DEFAULT_JINA_EMBEDDING_DIMENSIONS)
    )


def _fixture() -> EmbeddingPromotionFixture:
    seed = uuid.uuid4()
    vector = _embedding_vector()
    return EmbeddingPromotionFixture(
        user_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        governance_profile_id=uuid.uuid4(),
        consent_id=uuid.uuid4(),
        index_profile_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        artifact_set_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        search_profile_id=uuid.uuid4(),
        parent_chunk_id=uuid.uuid4(),
        child_chunk_id=uuid.uuid4(),
        job_id=uuid.uuid4(),
        worker_id=uuid.uuid4(),
        request_hash=_hash_hex("embedding-promotion", seed),
        chunk_text="Child chunk for promoted embedding smoke.",
        content_hash=_hash_hex("content", seed),
        expected_vector_hash=embedding_vector_sha256(vector),
    )


async def test_promoted_embedding_job_runner_finishes_live_postgres_job(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    url = database_url()
    fixture = _fixture()
    await _seed_fixture(url, fixture)
    requests: list[httpx.Request] = []

    def jina_handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(
            {
                "data": [
                    {
                        "embedding": list(_embedding_vector()),
                        "index": 0,
                        "object": "embedding",
                    }
                ],
                "model": DEFAULT_JINA_EMBEDDING_MODEL,
                "object": "list",
            }
        )

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(jina_handler)
    ) as client:
        original_builder = build_jina_passage_embedding_handler_dependencies

        def build_with_mocked_jina(
            *,
            api_key: str | None,
            projection: PassageEmbeddingProjectionGateway | None,
        ) -> PassageEmbeddingHandlerDependencies:
            return original_builder(
                api_key=api_key,
                projection=projection,
                client=client,
            )

        monkeypatch.setattr(
            worker_module,
            "build_jina_passage_embedding_handler_dependencies",
            build_with_mocked_jina,
        )
        settings = Settings(
            database_url=url,
            dispatch_enabled=True,
            job_stages=("passage_embedding",),
            worker_id=fixture.worker_id,
            jina_api_key=_JINA_API_KEY,
            provider_profile=ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
            ),
        )
        worker = Worker(settings)
        worker.validate_promotion_gate()
        assert set(worker.job_handlers) == {"passage_embedding"}

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

    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(JINA_EMBEDDINGS_URL)
    assert request.headers["authorization"] == f"Bearer {_JINA_API_KEY}"
    assert json.loads(request.content)["input"] == [{"text": fixture.chunk_text}]
    assert _JINA_API_KEY.encode() not in request.content
    assert await _job_projection_state(url, fixture) == (
        "succeeded",
        None,
        1,
        True,
        True,
        "ready",
        True,
        DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        fixture.expected_vector_hash,
        "published",
        True,
        "active",
        fixture.materialization_id,
    )


def _json_response(payload: object, *, status: int = 200) -> httpx.Response:
    content = json.dumps(payload, separators=(",", ":")).encode()
    return httpx.Response(
        status,
        headers={"Content-Type": "application/json; charset=utf-8"},
        content=content,
    )


async def _seed_fixture(url: str, fixture: EmbeddingPromotionFixture) -> None:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        await connection.execute(
            "INSERT INTO users (id, email, display_name) VALUES (%s, %s, %s)",
            (fixture.user_id, f"{fixture.user_id}@example.test", "G7.5I Embed"),
        )
        await connection.execute(
            """
            INSERT INTO files (
              id, user_id, original_filename, mime_type, byte_size, sha256,
              object_key
            ) VALUES (
              %s, %s, 'g7-5i.pdf', 'application/pdf', 16, %s,
              'knowledge/g7-5i.pdf'
            )
            """,
            (fixture.file_id, fixture.user_id, _HASH_A),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_collections (
              id, name, scope, owner_user_id, created_by_user_id
            ) VALUES (%s, 'G7.5I Promoted Embedding', 'personal', %s, %s)
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
                _HASH_B,
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
              %s, 'jina', %s, %s, 'api-20260623', %s,
              ARRAY['passage_embedding'], ARRAY['text/plain'], 'global', 'none',
              'delete', 'disabled', 'approved', 1, %s
            )
            """,
            (
                fixture.governance_profile_id,
                _ENDPOINT_ID,
                DEFAULT_JINA_EMBEDDING_MODEL,
                _hash_hex("profile-contract", fixture.governance_profile_id),
                _hash_hex("manifest", fixture.governance_profile_id),
            ),
        )
        await connection.execute(
            """
            INSERT INTO processor_governance_heads (
              processor, endpoint_id, model_id, status, active_profile_id,
              active_governance_revision, head_revision
            ) VALUES ('jina', %s, %s, 'active', %s, 1, 1)
            """,
            (
                _ENDPOINT_ID,
                DEFAULT_JINA_EMBEDDING_MODEL,
                fixture.governance_profile_id,
            ),
        )
        await connection.execute(
            """
            INSERT INTO processing_consents (
              id, scope, collection_id, processor, endpoint_id, model_id,
              governance_profile_id, governance_revision, governance_head_revision,
              purposes, data_types, policy_version, decision, consent_revision,
              granted_by_user_id
            ) VALUES (
              %s, 'collection', %s, 'jina', %s, %s, %s, 1, 1,
              ARRAY['passage_embedding'], ARRAY['text/plain'], 'g7.5i-python',
              'granted', 1, %s
            )
            """,
            (
                fixture.consent_id,
                fixture.collection_id,
                _ENDPOINT_ID,
                DEFAULT_JINA_EMBEDDING_MODEL,
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
            (fixture.index_profile_id, _HASH_C, _HASH_D, _HASH_E),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_index_generations (
              id, index_profile_id, generation_seq, status, build_snapshot,
              build_snapshot_hash
            ) VALUES (%s, %s, 1, 'building', '{}'::jsonb, %s)
            """,
            (fixture.index_generation_id, fixture.index_profile_id, _HASH_A),
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
            INSERT INTO knowledge_parser_artifact_sets (
              id, document_id, document_version_id, file_id, index_profile_id,
              parser_kind, parser_version, source_content_hash, config_hash,
              manifest_hash, status, quality_report, verified_at
            ) VALUES (
              %s, %s, %s, %s, %s, 'mineru', 'unit-test', %s, %s, %s,
              'verified', '{}'::jsonb, clock_timestamp()
            )
            """,
            (
                fixture.artifact_set_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                fixture.index_profile_id,
                _HASH_B,
                _HASH_C,
                _HASH_D,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_document_materializations (
              id, index_generation_id, collection_id, document_id,
              document_version_id, file_id, materialization_seq,
              parse_artifact_set_id, source_content_hash, base_profile_hash,
              collection_acl_revision, collection_visibility_epoch,
              collection_processing_revision, document_visibility_epoch, status
            ) VALUES (
              %s, %s, %s, %s, %s, %s, 1, %s, %s, %s, 1, 1, 1, 1,
              'staging'
            )
            """,
            (
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                fixture.artifact_set_id,
                _HASH_B,
                _HASH_E,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_parent_chunks (
              id, materialization_id, index_generation_id, document_id,
              document_version_id, ordinal, chunk_profile_hash, source_span_hash,
              content_hash, content, token_count, heading_path, locator_summary
            ) VALUES (
              %s, %s, %s, %s, %s, 0, %s, %s, %s,
              'Parent chunk for promoted embedding smoke.', 6,
              ARRAY['G7.5I']::TEXT[], '{"kind":"page","page":1}'::jsonb
            )
            """,
            (
                fixture.parent_chunk_id,
                fixture.materialization_id,
                fixture.index_generation_id,
                fixture.document_id,
                fixture.document_version_id,
                _HASH_D,
                _HASH_F,
                _HASH_A,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_child_chunks (
              id, parent_chunk_id, materialization_id, index_generation_id,
              document_id, document_version_id, ordinal, chunk_profile_hash,
              source_span_hash, content_hash, content, token_count
            ) VALUES (
              %s, %s, %s, %s, %s, %s, 0, %s, %s, %s, %s, 6
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
                _HASH_F,
                fixture.content_hash,
                fixture.chunk_text,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_child_search_projections (
              child_chunk_id, parent_chunk_id, materialization_id,
              index_generation_id, collection_id, document_id,
              document_version_id, search_profile_id, embedding_model_id,
              embedding_dimensions, lexical_text, exact_terms, source_span_hash,
              chunk_profile_hash, content_hash, locator_summary, status
            ) VALUES (
              %s, %s, %s, %s, %s, %s, %s, %s,
              'jina-embeddings-v4', 1024, %s,
              ARRAY['embedding','promotion']::TEXT[], %s, %s, %s,
              '{"kind":"page","page":1}'::jsonb, 'staging'
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
                fixture.chunk_text,
                _HASH_F,
                _HASH_D,
                fixture.content_hash,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_processing_jobs (
              id, collection_id, document_id, document_version_id, file_id,
              stage, operation, processor, endpoint_id, model_id,
              governance_profile_id, governance_revision, governance_head_revision,
              collection_consent_id, collection_consent_revision,
              collection_acl_revision, collection_visibility_epoch,
              collection_processing_revision, document_visibility_epoch,
              requested_by_user_id, idempotency_scope, idempotency_key,
              request_hash, status, attempt_count, max_attempts, available_at,
              index_generation_id, materialization_id, legacy_projection_unbound
            ) VALUES (
              %s, %s, %s, %s, %s, 'passage_embedding', 'initial', 'jina', %s,
              %s, %s, 1, 1, %s, 1, 1, 1, 1, 1, %s,
              'g7.5i-embedding-promotion', 'job-runner-smoke', %s, 'pending',
              0, 3, clock_timestamp(), %s, %s, false
            )
            """,
            (
                fixture.job_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                _ENDPOINT_ID,
                DEFAULT_JINA_EMBEDDING_MODEL,
                fixture.governance_profile_id,
                fixture.consent_id,
                fixture.user_id,
                fixture.request_hash,
                fixture.index_generation_id,
                fixture.materialization_id,
            ),
        )
        await connection.commit()


async def _job_projection_state(
    url: str,
    fixture: EmbeddingPromotionFixture,
) -> tuple[
    str,
    str | None,
    int,
    bool,
    bool,
    str,
    bool,
    int,
    str | None,
    str,
    bool,
    str,
    uuid.UUID | None,
]:
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
                SELECT status FROM knowledge_child_search_projections
                WHERE child_chunk_id = %s
              )::TEXT,
              (
                SELECT ready_at IS NOT NULL FROM knowledge_child_search_projections
                WHERE child_chunk_id = %s
              )::BOOLEAN,
              (
                SELECT cardinality(embedding_vector)
                FROM knowledge_child_search_projections
                WHERE child_chunk_id = %s
              )::INTEGER,
              (
                SELECT embedding_vector_sha256
                FROM knowledge_child_search_projections
                WHERE child_chunk_id = %s
              )::TEXT,
              (
                SELECT status FROM knowledge_document_materializations
                WHERE id = %s
              )::TEXT,
              (
                SELECT published_at IS NOT NULL
                FROM knowledge_document_materializations
                WHERE id = %s
              )::BOOLEAN,
              (
                SELECT status FROM knowledge_documents WHERE id = %s
              )::TEXT,
              (
                SELECT active_materialization_id
                FROM knowledge_document_projection_heads
                WHERE index_generation_id = %s AND document_id = %s
              )::TEXT
            """,
            (
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.child_chunk_id,
                fixture.child_chunk_id,
                fixture.child_chunk_id,
                fixture.child_chunk_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.document_id,
                fixture.index_generation_id,
                fixture.document_id,
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
        str(row[5]),
        bool(row[6]),
        int(row[7]),
        None if row[8] is None else str(row[8]),
        str(row[9]),
        bool(row[10]),
        str(row[11]),
        None if row[12] is None else uuid.UUID(str(row[12])),
    )
