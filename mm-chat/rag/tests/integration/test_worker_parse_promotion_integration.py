from __future__ import annotations

import hashlib
import io
import json
import os
import uuid
import zipfile
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
    DocumentSource,
    PassageEmbeddingHandlerDependencies,
    PassageEmbeddingProjectionGateway,
    embedding_vector_sha256,
)
from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.mineru_gateway import (
    MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
    MinerULocalBatchAllocation,
    MinerULocalBatchGateway,
    MinerULocalBatchPollResult,
)
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
_HASH_C: Final = "c" * 64
_HASH_0: Final = "0" * 64
_MODEL_ID: Final = "mineru-parser-v20260716"
_ENDPOINT_ID: Final = "hosted-main"
_JINA_ENDPOINT_ID: Final = "admin-env"
_JINA_MODEL_ID: Final = DEFAULT_JINA_EMBEDDING_MODEL
_JINA_API_KEY: Final = "unit-test-jina-key"
_MINERU_API_KEY: Final = "unit-test-mineru-key"
_SOURCE_GATEWAY_TOKEN: Final = "unit-test-source-gateway-token"
_SOURCE_GATEWAY_URL: Final = "http://backend.internal"
_SIGNED_UPLOAD_URL: Final = (
    "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/g7-5l.pdf"
    "?Expires=1&Signature=redacted"
)
_RESULT_URL: Final = "https://cdn-mineru.openxlab.org.cn/pdf/g7-5l-result.zip"


@dataclass(frozen=True, slots=True)
class ParsePromotionFixture:
    user_id: uuid.UUID
    file_id: uuid.UUID
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    governance_profile_id: uuid.UUID
    consent_id: uuid.UUID
    embedding_governance_profile_id: uuid.UUID
    embedding_consent_id: uuid.UUID
    index_profile_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID
    search_profile_id: uuid.UUID
    job_id: uuid.UUID
    worker_id: uuid.UUID
    request_hash: str
    source_body: bytes
    source_sha256: str
    base_profile_hash: str
    generation_seq: int


class FakeMinerULocalBatchGateway(MinerULocalBatchGateway):
    def __init__(
        self,
        fixture: ParsePromotionFixture,
        calls: list[str],
        archive_body: bytes,
    ) -> None:
        self._fixture = fixture
        self._calls = calls
        self._archive_body = archive_body

    async def allocate_upload(
        self,
        context: object,
        source: DocumentSource,
        *,
        filename: str = "document.pdf",
    ) -> MinerULocalBatchAllocation:
        self._calls.append("mineru_allocate")
        assert context is not None
        assert source.body == self._fixture.source_body
        assert source.source_sha256 == self._fixture.source_sha256
        return MinerULocalBatchAllocation(
            batch_id="g7-5l-batch",
            upload_urls=(_SIGNED_UPLOAD_URL,),
            filename=filename,
        )

    async def upload_document(
        self,
        context: object,
        source: DocumentSource,
        allocation: MinerULocalBatchAllocation,
    ) -> None:
        self._calls.append("mineru_upload")
        assert context is not None
        assert source.body == self._fixture.source_body
        assert allocation.batch_id == "g7-5l-batch"

    async def poll_batch_result(
        self,
        context: object,
        allocation: MinerULocalBatchAllocation,
    ) -> MinerULocalBatchPollResult:
        self._calls.append("mineru_poll")
        assert context is not None
        assert allocation.batch_id == "g7-5l-batch"
        return MinerULocalBatchPollResult(
            batch_id=allocation.batch_id,
            filename=allocation.filename,
            state="done",
            result_url=_RESULT_URL,
        )

    async def download_result_archive(
        self,
        context: object,
        poll_result: MinerULocalBatchPollResult,
    ) -> bytes:
        self._calls.append("mineru_download")
        assert context is not None
        assert poll_result.state == "done"
        return self._archive_body


def database_url() -> str:
    value = os.environ.get("RAG_TEST_DATABASE_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_DATABASE_URL is not set")
    return value


def _hash_hex(label: str, seed: uuid.UUID) -> str:
    return hashlib.sha256(f"{label}:{seed}".encode()).hexdigest()


def _mineru_endpoint_id(fixture: ParsePromotionFixture) -> str:
    return f"{_ENDPOINT_ID}-{fixture.job_id.hex[:8]}"


def _jina_endpoint_id(fixture: ParsePromotionFixture) -> str:
    return f"{_JINA_ENDPOINT_ID}-{fixture.job_id.hex[:8]}"


def _embedding_vector() -> tuple[float, ...]:
    return tuple(
        0.125 + float(index % 17) / 1000
        for index in range(DEFAULT_JINA_EMBEDDING_DIMENSIONS)
    )


def _json_response(payload: object, *, status: int = 200) -> httpx.Response:
    content = json.dumps(payload, separators=(",", ":")).encode()
    return httpx.Response(
        status,
        headers={"Content-Type": "application/json; charset=utf-8"},
        content=content,
    )


def _fixture() -> ParsePromotionFixture:
    seed = uuid.uuid4()
    source_body = (
        b"%PDF-1.7\n"
        b"G7.5L promoted parse job runner fixture\n"
        b"%%EOF\n"
    )
    return ParsePromotionFixture(
        user_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        governance_profile_id=uuid.uuid4(),
        consent_id=uuid.uuid4(),
        embedding_governance_profile_id=uuid.uuid4(),
        embedding_consent_id=uuid.uuid4(),
        index_profile_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        search_profile_id=uuid.uuid4(),
        job_id=uuid.uuid4(),
        worker_id=uuid.uuid4(),
        request_hash=_hash_hex("parse-promotion", seed),
        source_body=source_body,
        source_sha256=hashlib.sha256(source_body).hexdigest(),
        base_profile_hash=_hash_hex("base-profile", seed),
        generation_seq=1 + seed.int % 9_000_000_000_000_000,
    )


def _source_handler(
    fixture: ParsePromotionFixture,
    calls: list[str],
    source_requests: list[httpx.Request],
) -> httpx.MockTransport:
    def handle(request: httpx.Request) -> httpx.Response:
        calls.append("go_source_object")
        source_requests.append(request)
        payload = json.loads(request.content)
        assert payload == {
            "fileId": str(fixture.file_id),
            "jobId": str(fixture.job_id),
            "leaseToken": payload["leaseToken"],
            "materializationId": str(fixture.materialization_id),
            "workerId": str(fixture.worker_id),
        }
        assert uuid.UUID(payload["leaseToken"]) != uuid.UUID(int=0)
        assert request.headers["x-mm-chat-internal-token"] == _SOURCE_GATEWAY_TOKEN
        return httpx.Response(
            200,
            headers={
                "Content-Length": str(len(fixture.source_body)),
                "Content-Type": "application/octet-stream",
                "X-MM-Chat-File-ID": str(fixture.file_id),
                "X-MM-Chat-Source-SHA256": fixture.source_sha256,
            },
            content=fixture.source_body,
        )

    return httpx.MockTransport(handle)


def _patch_parse_gateways(
    monkeypatch: pytest.MonkeyPatch,
    *,
    fixture: ParsePromotionFixture,
    calls: list[str],
    archive_body: bytes,
    source_client: httpx.AsyncClient,
) -> None:
    original_source_gateway = worker_module.GoSourceObjectBytesGateway

    def source_gateway_with_mocked_http(
        *,
        base_url: str,
        internal_token: str,
        worker_id: uuid.UUID,
    ) -> object:
        return original_source_gateway(
            base_url=base_url,
            internal_token=internal_token,
            worker_id=worker_id,
            client=source_client,
        )

    class WorkerFakeMinerULocalBatchGateway(FakeMinerULocalBatchGateway):
        def __init__(self, api_token: str | None) -> None:
            assert api_token == _MINERU_API_KEY
            super().__init__(fixture, calls, archive_body)

    monkeypatch.setattr(
        worker_module,
        "GoSourceObjectBytesGateway",
        source_gateway_with_mocked_http,
    )
    monkeypatch.setattr(
        worker_module,
        "MinerULocalBatchGateway",
        WorkerFakeMinerULocalBatchGateway,
    )


def _patch_jina_gateway(
    monkeypatch: pytest.MonkeyPatch,
    jina_client: httpx.AsyncClient,
) -> None:
    original_jina_builder = build_jina_passage_embedding_handler_dependencies

    def build_with_mocked_jina(
        *,
        api_key: str | None,
        projection: PassageEmbeddingProjectionGateway | None,
    ) -> PassageEmbeddingHandlerDependencies:
        assert api_key == _JINA_API_KEY
        return original_jina_builder(
            api_key=api_key,
            projection=projection,
            client=jina_client,
        )

    monkeypatch.setattr(
        worker_module,
        "build_jina_passage_embedding_handler_dependencies",
        build_with_mocked_jina,
    )


def _settings(
    fixture: ParsePromotionFixture,
    *,
    job_stages: tuple[str, ...],
) -> Settings:
    return Settings(
        database_url=database_url(),
        dispatch_enabled=True,
        job_stages=job_stages,
        worker_id=fixture.worker_id,
        mineru_api_key=_MINERU_API_KEY,
        jina_api_key=_JINA_API_KEY if "passage_embedding" in job_stages else None,
        source_gateway_url=_SOURCE_GATEWAY_URL,
        source_gateway_token=_SOURCE_GATEWAY_TOKEN,
        provider_profile=ProviderRuntimeProfile(
            profile_id=MINERU_JINA_POSTGRES_PROFILE,
            accepted_draft_wire_contracts=True,
        ),
    )


async def test_promoted_parse_job_runner_finishes_live_postgres_job(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    url = database_url()
    fixture = _fixture()
    await _seed_fixture(url, fixture)
    calls: list[str] = []
    source_requests: list[httpx.Request] = []

    archive_body = _mineru_archive("G7.5L promoted parse\n\nMinerU baseline smoke")

    async with httpx.AsyncClient(
        transport=_source_handler(fixture, calls, source_requests)
    ) as source_client:
        _patch_parse_gateways(
            monkeypatch,
            fixture=fixture,
            calls=calls,
            archive_body=archive_body,
            source_client=source_client,
        )
        worker = Worker(_settings(fixture, job_stages=("parse",)))
        worker.validate_promotion_gate()
        assert set(worker.job_handlers) == {"parse"}

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

    assert calls == [
        "go_source_object",
        "mineru_allocate",
        "mineru_upload",
        "mineru_poll",
        "mineru_download",
    ]
    assert len(source_requests) == 1
    request = source_requests[0]
    assert request.method == "POST"
    assert str(request.url) == f"{_SOURCE_GATEWAY_URL}/internal/rag/source-object"
    assert _SOURCE_GATEWAY_TOKEN.encode() not in request.content
    assert _MINERU_API_KEY.encode() not in request.content
    state = await _job_projection_state(url, fixture)
    assert state[:5] == ("succeeded", None, 1, True, True)
    assert all(count > 0 for count in state[5:10])
    assert state[10] == "staging"
    assert "MinerU baseline smoke" in state[11]
    assert await _embedding_job_state(url, fixture) == (
        "pending",
        None,
        0,
        True,
        "passage_embedding",
        "initial",
        "jina",
        _jina_endpoint_id(fixture),
        _JINA_MODEL_ID,
        3,
        fixture.job_id,
        fixture.materialization_id,
    )


async def test_promoted_parse_then_embedding_runner_finishes_live_postgres_chain(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    url = database_url()
    fixture = _fixture()
    await _seed_fixture(url, fixture)
    calls: list[str] = []
    source_requests: list[httpx.Request] = []
    jina_requests: list[httpx.Request] = []
    chain_text = "G7.5O promoted parse\n\nMinerU to Jina chain smoke"
    expected_vector_hash = embedding_vector_sha256(_embedding_vector())

    def jina_handler(request: httpx.Request) -> httpx.Response:
        calls.append("jina_embeddings")
        jina_requests.append(request)
        payload = json.loads(request.content)
        assert payload["model"] == _JINA_MODEL_ID
        assert payload["dimensions"] == DEFAULT_JINA_EMBEDDING_DIMENSIONS
        assert payload["input"] == [{"text": chain_text}]
        return _json_response(
            {
                "data": [
                    {
                        "embedding": list(_embedding_vector()),
                        "index": 0,
                        "object": "embedding",
                    }
                ],
                "model": _JINA_MODEL_ID,
                "object": "list",
            }
        )

    archive_body = _mineru_archive(chain_text)

    async with (
        httpx.AsyncClient(
            transport=_source_handler(fixture, calls, source_requests)
        ) as source_client,
        httpx.AsyncClient(transport=httpx.MockTransport(jina_handler)) as jina_client,
    ):
        _patch_parse_gateways(
            monkeypatch,
            fixture=fixture,
            calls=calls,
            archive_body=archive_body,
            source_client=source_client,
        )
        _patch_jina_gateway(monkeypatch, jina_client)
        worker = Worker(_settings(fixture, job_stages=("parse", "passage_embedding")))
        worker.validate_promotion_gate()
        assert set(worker.job_handlers) == {"parse", "passage_embedding"}

        await worker.database.open()
        try:
            runner = JobRunner(
                worker.database,
                worker.settings,
                worker.metrics,
                worker.job_handlers,
            )
            assert await runner.process_one()
            assert await _embedding_job_state(url, fixture) == (
                "pending",
                None,
                0,
                True,
                "passage_embedding",
                "initial",
                "jina",
                _jina_endpoint_id(fixture),
                _JINA_MODEL_ID,
                3,
                fixture.job_id,
                fixture.materialization_id,
            )
            assert await runner.process_one()
            assert not await runner.process_one()
        finally:
            await worker.database.close()

    assert calls == [
        "go_source_object",
        "mineru_allocate",
        "mineru_upload",
        "mineru_poll",
        "mineru_download",
        "jina_embeddings",
    ]
    assert len(source_requests) == 1
    assert len(jina_requests) == 1
    jina_request = jina_requests[0]
    assert jina_request.method == "POST"
    assert jina_request.url == httpx.URL(JINA_EMBEDDINGS_URL)
    assert jina_request.headers["authorization"] == f"Bearer {_JINA_API_KEY}"
    assert _JINA_API_KEY.encode() not in jina_request.content
    state = await _job_projection_state(url, fixture)
    assert state[:5] == ("succeeded", None, 1, True, True)
    assert all(count > 0 for count in state[5:10])
    assert state[10] == "ready"
    assert "MinerU to Jina chain smoke" in state[11]
    assert await _embedding_job_state(url, fixture) == (
        "succeeded",
        None,
        1,
        True,
        "passage_embedding",
        "initial",
        "jina",
        _jina_endpoint_id(fixture),
        _JINA_MODEL_ID,
        3,
        fixture.job_id,
        fixture.materialization_id,
    )
    assert await _search_embedding_state(url, fixture) == (
        "ready",
        True,
        DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        expected_vector_hash,
    )
    assert await _published_materialization_state(url, fixture) == (
        "published",
        True,
        "active",
        fixture.document_version_id,
        fixture.materialization_id,
    )


def _mineru_archive(text: str) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr("full.md", text.encode())
        archive.writestr("fixture_content_list.json", b'[{"type":"text"}]')
        archive.writestr("layout.json", b'{"pages":[{"page":0}]}')
        archive.writestr("fixture_model.json", b'{"model":"vlm"}')
    return output.getvalue()


async def _seed_fixture(url: str, fixture: ParsePromotionFixture) -> None:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        await connection.execute(
            """
            UPDATE knowledge_processing_jobs
            SET status = 'cancelled',
                lease_owner = NULL,
                lease_token = NULL,
                lease_expires_at = NULL,
                completed_at = COALESCE(completed_at, clock_timestamp()),
                updated_at = clock_timestamp()
            WHERE status IN ('pending', 'processing')
            """
        )
        await connection.execute(
            "INSERT INTO users (id, email, display_name) VALUES (%s, %s, %s)",
            (fixture.user_id, f"{fixture.user_id}@example.test", "G7.5L Parse"),
        )
        await connection.execute(
            """
            INSERT INTO files (
              id, user_id, original_filename, mime_type, byte_size, sha256,
              object_key
            ) VALUES (%s, %s, 'g7-5l.pdf', 'application/pdf', %s, %s, %s)
            """,
            (
                fixture.file_id,
                fixture.user_id,
                len(fixture.source_body),
                fixture.source_sha256,
                f"knowledge/{fixture.file_id}.pdf",
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_collections (
              id, name, scope, owner_user_id, created_by_user_id
            ) VALUES (%s, 'G7.5L Promoted Parse', 'personal', %s, %s)
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
                _mineru_endpoint_id(fixture),
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
            (_mineru_endpoint_id(fixture), _MODEL_ID, fixture.governance_profile_id),
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
              ARRAY['parse'], ARRAY['application/pdf'], 'g7.5l-python',
              'granted', 1, %s
            )
            """,
            (
                fixture.consent_id,
                fixture.collection_id,
                _mineru_endpoint_id(fixture),
                _MODEL_ID,
                fixture.governance_profile_id,
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
                fixture.embedding_governance_profile_id,
                _jina_endpoint_id(fixture),
                _JINA_MODEL_ID,
                _hash_hex("jina-profile-contract", fixture.governance_profile_id),
                _hash_hex("jina-manifest", fixture.governance_profile_id),
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
                _jina_endpoint_id(fixture),
                _JINA_MODEL_ID,
                fixture.embedding_governance_profile_id,
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
              ARRAY['passage_embedding'], ARRAY['text/plain'], 'g7.5n-python',
              'granted', 1, %s
            )
            """,
            (
                fixture.embedding_consent_id,
                fixture.collection_id,
                _jina_endpoint_id(fixture),
                _JINA_MODEL_ID,
                fixture.embedding_governance_profile_id,
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
              'jina', %s, 'jina-embeddings-v4', 'api-20260623',
              'passage', 'jina', %s, 'jina-reranker-v3',
              'api-20260623', %s
            )
            """,
            (
                fixture.index_profile_id,
                _hash_hex("parser-manifest", fixture.index_profile_id),
                MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
                _jina_endpoint_id(fixture),
                _jina_endpoint_id(fixture),
                fixture.base_profile_hash,
            ),
        )
        await connection.execute(
            """
            INSERT INTO knowledge_index_generations (
              id, index_profile_id, generation_seq, status, build_snapshot,
              build_snapshot_hash, failure_code, failed_at
            ) VALUES (
              %s, %s, %s, 'failed', '{}'::jsonb, %s,
              'TEST_GENERATION', clock_timestamp()
            )
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
              available_at, index_generation_id, materialization_id,
              legacy_projection_unbound
            ) VALUES (
              %s, %s, %s, %s, %s, 'parse', 'initial', 'mineru', %s, %s,
              %s, 1, 1, %s, 1, 1, 1, 1, 1, %s, 'g7.5l-parse-promotion',
              %s, %s, 'pending', 0, 3, clock_timestamp(),
              %s, %s, false
            )
            """,
            (
                fixture.job_id,
                fixture.collection_id,
                fixture.document_id,
                fixture.document_version_id,
                fixture.file_id,
                _mineru_endpoint_id(fixture),
                _MODEL_ID,
                fixture.governance_profile_id,
                fixture.consent_id,
                fixture.user_id,
                str(fixture.job_id),
                fixture.request_hash,
                fixture.index_generation_id,
                fixture.materialization_id,
            ),
        )
        await connection.commit()


async def _job_projection_state(
    url: str,
    fixture: ParsePromotionFixture,
) -> tuple[str, str | None, int, bool, bool, int, int, int, int, int, str, str]:
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
                SELECT lease_token IS NULL FROM knowledge_processing_jobs
                WHERE id = %s
              )::BOOLEAN,
              (
                SELECT count(*) FROM knowledge_parser_artifact_sets
                WHERE document_version_id = %s
              )::INTEGER,
              (
                SELECT count(*) FROM knowledge_blocks
                WHERE document_version_id = %s
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
                SELECT count(*) FROM knowledge_child_search_projections
                WHERE materialization_id = %s
              )::INTEGER,
              (
                SELECT status FROM knowledge_child_search_projections
                WHERE materialization_id = %s
                ORDER BY child_chunk_id LIMIT 1
              )::TEXT,
              (
                SELECT lexical_text FROM knowledge_child_search_projections
                WHERE materialization_id = %s
                ORDER BY child_chunk_id LIMIT 1
              )::TEXT
            """,
            (
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.job_id,
                fixture.document_version_id,
                fixture.document_version_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.materialization_id,
                fixture.materialization_id,
            ),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("job projection query returned no row")
    return (
        str(row[0]),
        None if row[1] is None else str(row[1]),
        int(row[2]),
        bool(row[3]),
        bool(row[4]),
        int(row[5]),
        int(row[6]),
        int(row[7]),
        int(row[8]),
        int(row[9]),
        str(row[10]),
        str(row[11]),
    )


async def _embedding_job_state(
    url: str,
    fixture: ParsePromotionFixture,
) -> tuple[
    str,
    str | None,
    int,
    bool,
    str,
    str,
    str,
    str,
    str,
    int,
    uuid.UUID,
    uuid.UUID,
]:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        cursor = await connection.execute(
            """
            SELECT
              status::TEXT,
              error_code::TEXT,
              attempt_count::INTEGER,
              lease_owner IS NULL,
              stage::TEXT,
              operation::TEXT,
              processor::TEXT,
              endpoint_id::TEXT,
              model_id::TEXT,
              max_attempts::INTEGER,
              caused_by_job_id,
              materialization_id
            FROM knowledge_processing_jobs
            WHERE caused_by_job_id = %s
              AND stage = 'passage_embedding'
            """,
            (fixture.job_id,),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("embedding job query returned no row")
    return (
        str(row[0]),
        None if row[1] is None else str(row[1]),
        int(row[2]),
        bool(row[3]),
        str(row[4]),
        str(row[5]),
        str(row[6]),
        str(row[7]),
        str(row[8]),
        int(row[9]),
        row[10],
        row[11],
    )


async def _search_embedding_state(
    url: str,
    fixture: ParsePromotionFixture,
) -> tuple[str, bool, int, str | None]:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        cursor = await connection.execute(
            """
            SELECT
              status::TEXT,
              ready_at IS NOT NULL,
              cardinality(embedding_vector)::INTEGER,
              embedding_vector_sha256::TEXT
            FROM knowledge_child_search_projections
            WHERE materialization_id = %s
            ORDER BY child_chunk_id
            LIMIT 1
            """,
            (fixture.materialization_id,),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("search embedding state query returned no row")
    return (
        str(row[0]),
        bool(row[1]),
        int(row[2]),
        None if row[3] is None else str(row[3]),
    )


async def _published_materialization_state(
    url: str,
    fixture: ParsePromotionFixture,
) -> tuple[str, bool, str, uuid.UUID | None, uuid.UUID | None]:
    async with await psycopg.AsyncConnection.connect(url) as connection:
        cursor = await connection.execute(
            """
            SELECT
              materialization.status::TEXT,
              materialization.published_at IS NOT NULL,
              document.status::TEXT,
              document.current_version_id,
              projection_head.active_materialization_id
            FROM knowledge_document_materializations materialization
            JOIN knowledge_documents document
              ON document.id = materialization.document_id
            LEFT JOIN knowledge_document_projection_heads projection_head
              ON projection_head.index_generation_id =
                materialization.index_generation_id
             AND projection_head.document_id = materialization.document_id
            WHERE materialization.id = %s
            """,
            (fixture.materialization_id,),
        )
        row = await cursor.fetchone()
    if row is None:
        raise AssertionError("published materialization state query returned no row")
    return (
        str(row[0]),
        bool(row[1]),
        str(row[2]),
        None if row[3] is None else uuid.UUID(str(row[3])),
        None if row[4] is None else uuid.UUID(str(row[4])),
    )
