from __future__ import annotations

import uuid
from collections.abc import Sequence
from dataclasses import replace
from decimal import Decimal
from typing import cast

import psycopg
import pytest
from psycopg.types.json import Jsonb

import mm_chat_rag.postgres as postgres_module
from mm_chat_rag.job_context import (
    JOB_CONTEXT_LEASE_FENCE_MISSING,
    ProcessingJobContext,
)
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_PARSE_ARTIFACT_INVALID,
    JOB_HANDLER_PARSE_PROFILE_INVALID,
    JOB_HANDLER_SOURCE_INVALID,
    PassageEmbeddingCandidate,
    PassageEmbeddingVector,
    PurgeProjectionResult,
    StagedPassageEmbedding,
    embedding_vector_sha256,
)
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import JobClaim, OutboxClaim
from mm_chat_rag.postgres import (
    _SQL,
    PostgresAdapter,
    _function_succeeded,
    _raise_stable_database_error,
    replay_job,
    replay_outbox,
)
from mm_chat_rag.projection import (
    BlockProjectionRow,
    ChildChunkProjectionRow,
    ChildSearchProjectionRow,
    ChunkBlockSpanProjectionRow,
    ParentChunkProjectionRow,
    PostgresProjectionBatch,
)
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.settings import Settings
from mm_chat_rag.source_gateway import FileSourceMetadata

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"

FakeRow = dict[str, object]


class FakeCursor:
    def __init__(self, row: FakeRow | list[FakeRow] | None) -> None:
        self.row = row

    async def fetchone(self) -> FakeRow | None:
        if isinstance(self.row, list):
            return self.row[0] if self.row else None
        return self.row

    async def fetchall(self) -> list[FakeRow]:
        if self.row is None:
            return []
        if isinstance(self.row, list):
            return self.row
        return [self.row]


class AsyncContext:
    def __init__(self, value: object) -> None:
        self.value = value

    async def __aenter__(self) -> object:
        return self.value

    async def __aexit__(self, *_: object) -> None:
        return None


class FakeConnection:
    def __init__(self, rows: list[FakeRow | list[FakeRow] | None]) -> None:
        self.rows = rows
        self.calls: list[tuple[str, Sequence[object]]] = []

    def transaction(self) -> AsyncContext:
        return AsyncContext(None)

    async def execute(self, sql: str, parameters: Sequence[object]) -> FakeCursor:
        self.calls.append((sql, parameters))
        return FakeCursor(self.rows.pop(0))

    async def __aenter__(self) -> FakeConnection:
        return self

    async def __aexit__(self, *_: object) -> None:
        return None


class FakePool:
    def __init__(self, connection: FakeConnection) -> None:
        self.fake_connection = connection

    def connection(self) -> AsyncContext:
        return AsyncContext(self.fake_connection)


def adapter_with_rows(
    rows: list[FakeRow | list[FakeRow] | None], settings: Settings | None = None
) -> tuple[PostgresAdapter, FakeConnection]:
    adapter = PostgresAdapter(
        settings or Settings(database_url="postgresql://worker:secret@db/rag"),
        Metrics.create(),
    )
    connection = FakeConnection(rows)
    adapter._pool = FakePool(connection)  # type: ignore[assignment]
    return adapter, connection


def purge_context(**updates: object) -> ProcessingJobContext:
    context = ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="purge",
        operation="purge",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=None,
        collection_acl_revision=1,
        collection_visibility_epoch=2,
        collection_processing_revision=1,
        document_visibility_epoch=3,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=None,
        lease_token=uuid.uuid4(),
    )
    return replace(context, **updates)


def passage_embedding_context(**updates: object) -> ProcessingJobContext:
    context = ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="passage_embedding",
        operation="initial",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        collection_acl_revision=1,
        collection_visibility_epoch=2,
        collection_processing_revision=1,
        document_visibility_epoch=3,
        attempt_count=1,
        max_attempts=3,
        request_hash="b" * 64,
        authority=None,
        lease_token=uuid.uuid4(),
    )
    return replace(context, **updates)


def parse_context(**updates: object) -> ProcessingJobContext:
    context = ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="parse",
        operation="initial",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        collection_acl_revision=1,
        collection_visibility_epoch=2,
        collection_processing_revision=1,
        document_visibility_epoch=3,
        attempt_count=1,
        max_attempts=3,
        request_hash="d" * 64,
        authority=None,
        lease_token=uuid.uuid4(),
    )
    return replace(context, **updates)


def projection_batch(context: ProcessingJobContext) -> PostgresProjectionBatch:
    if context.materialization_id is None:
        raise AssertionError("parse projection fixture requires materialization")
    artifact_set_id = uuid.uuid4()
    block_id = uuid.uuid4()
    parent_chunk_id = uuid.uuid4()
    child_chunk_id = uuid.uuid4()
    source_sha256 = "e" * 64
    chunk_profile_hash = "f" * 64
    source_span_hash = "1" * 64
    content_hash = "2" * 64
    locator = {"kind": "text_offset", "start": 0, "end": 13}
    locator_summary = {
        "schemaVersion": "g7.4-locator-summary.v1",
        "primary": {"kind": "text_offset", "locator": locator},
        "fragments": [],
        "locatorAggregateHashes": [],
    }
    return PostgresProjectionBatch(
        blocks=(
            BlockProjectionRow(
                id=block_id,
                artifact_set_id=artifact_set_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                parent_block_id=None,
                ordinal=0,
                block_type="paragraph",
                heading_path=("heading",),
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
                logical_block_id="3" * 64,
            ),
        ),
        parent_chunks=(
            ParentChunkProjectionRow(
                id=parent_chunk_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                ordinal=0,
                chunk_profile_hash=chunk_profile_hash,
                source_span_hash=source_span_hash,
                content_hash=content_hash,
                content="hello fixture",
                token_count=2,
                heading_path=("heading",),
                locator_summary=locator_summary,
                logical_chunk_id="4" * 64,
            ),
        ),
        child_chunks=(
            ChildChunkProjectionRow(
                id=child_chunk_id,
                parent_chunk_id=parent_chunk_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                ordinal=0,
                chunk_profile_hash=chunk_profile_hash,
                source_span_hash=source_span_hash,
                content_hash=content_hash,
                content="hello fixture",
                token_count=2,
                overlap_before_tokens=0,
                overlap_after_tokens=0,
                logical_chunk_id="5" * 64,
            ),
        ),
        chunk_block_spans=(
            ChunkBlockSpanProjectionRow(
                chunk_kind="child",
                chunk_id=child_chunk_id,
                block_id=block_id,
                span_ordinal=0,
                start_offset=0,
                end_offset=13,
                fragment_source_span_hash=source_span_hash,
            ),
        ),
        child_search_projections=(
            ChildSearchProjectionRow(
                child_chunk_id=child_chunk_id,
                parent_chunk_id=parent_chunk_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                collection_id=context.collection_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                embedding_model_id="jina-embeddings-v4",
                embedding_dimensions=1024,
                lexical_text="hello fixture",
                exact_terms=("fixture", "hello"),
                source_span_hash=source_span_hash,
                chunk_profile_hash=chunk_profile_hash,
                content_hash=content_hash,
                locator_summary=locator_summary,
            ),
        ),
        source_sha256=source_sha256,
        chunk_profile_hash=chunk_profile_hash,
    )


def jsonb_payload(value: object) -> list[dict[str, object]]:
    assert isinstance(value, Jsonb)
    assert isinstance(value.obj, list)
    return cast("list[dict[str, object]]", value.obj)


def test_sql_surface_is_select_only_and_function_allowlisted() -> None:
    assert set(_SQL) == {
        "claim_outbox",
        "apply_outbox",
        "retry_outbox",
        "fail_outbox",
        "claim_job",
        "heartbeat_job",
        "finish_job",
        "readiness",
        "assert_search_complete",
        "fetch_passage_embedding_candidates",
        "stage_passage_embedding",
        "fetch_parse_source_metadata",
        "resolve_parse_chunk_profile",
        "stage_parse_projection",
        "complete_parse_and_enqueue_embedding",
        "complete_embedding_and_publish",
        "mark_purge_invisible",
        "purge_search_projection",
        "assert_purge_complete",
        "replay_outbox",
        "replay_job",
        "fetch_query_evidence_candidates",
    }
    for sql in _SQL.values():
        assert sql.startswith("SELECT * FROM knowledge_")
        assert not any(
            word in sql.upper() for word in ("INSERT ", "UPDATE ", "DELETE ", "CREATE ")
        )


@pytest.mark.parametrize(
    ("row", "expected"),
    [
        ({"succeeded": True}, True),
        ({"applied": True}, True),
        ({"result": True}, True),
        ({"result": False}, False),
        ({"knowledge_assert_materialization_search_complete": True}, True),
        (None, False),
    ],
)
def test_function_result_normalization(
    row: dict[str, object] | None, expected: bool
) -> None:
    assert _function_succeeded(row) is expected


async def test_adapter_calls_only_frozen_functions() -> None:
    event_id = uuid.uuid4()
    job_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        dispatch_enabled=True,
        job_stages=("parse",),
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=ProviderRuntimeProfile(
            profile_id=MINERU_JINA_POSTGRES_PROFILE,
            accepted_draft_wire_contracts=True,
        ),
    )
    rows: list[dict[str, object] | None] = [
        {"functions_ready": True, "consumer_status": "ready"},
        {"outbox_id": 1, "event_id": event_id, "event_type": "synthetic"},
        {"applied": True},
        {"succeeded": True},
        {"succeeded": True},
        {"job_id": job_id, "stage": "parse"},
        {"succeeded": True},
        {"succeeded": True},
        {"result": True},
        {"result": True},
    ]
    adapter, connection = adapter_with_rows(rows, settings)
    assert (await adapter.readiness()).functions
    outbox_token = uuid.uuid4()
    outbox = await adapter.claim_outbox(outbox_token)
    assert isinstance(outbox, OutboxClaim)
    assert await adapter.apply_and_ack_outbox(
        outbox,
        outbox_token,
        scope_kind="global",
        index_generation_id=None,
        action="dispatch",
        result_hash="a" * 64,
    )
    assert await adapter.retry_outbox(outbox, outbox_token, "PLAN_INVALID", 30)
    assert await adapter.fail_outbox(outbox, outbox_token, "PLAN_INVALID")
    lease_token = uuid.uuid4()
    job = await adapter.claim_job(lease_token)
    assert isinstance(job, JobClaim)
    assert await adapter.heartbeat_job(job, lease_token)
    assert await adapter.finish_job(
        job,
        lease_token,
        outcome="succeeded",
        error_code=None,
        retry_after_seconds=0,
    )
    assert await adapter.assert_materialization_search_complete(
        uuid.uuid4(), expected_child_count=1
    )
    assert await adapter.complete_embedding_and_publish(passage_embedding_context())
    assert [call[0] for call in connection.calls] == [
        _SQL["readiness"],
        _SQL["claim_outbox"],
        _SQL["apply_outbox"],
        _SQL["retry_outbox"],
        _SQL["fail_outbox"],
        _SQL["claim_job"],
        _SQL["heartbeat_job"],
        _SQL["finish_job"],
        _SQL["assert_search_complete"],
        _SQL["complete_embedding_and_publish"],
    ]


async def test_passage_embedding_projection_gateway_calls_functions() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = passage_embedding_context()
    child_a = uuid.uuid4()
    child_b = uuid.uuid4()
    candidate_rows: list[FakeRow] = [
        {
            "child_chunk_id": child_a,
            "content": "first passage candidate",
            "content_hash": "a" * 64,
        },
        {
            "child_chunk_id": child_b,
            "content": "second passage candidate",
            "content_hash": "b" * 64,
        },
    ]
    adapter, connection = adapter_with_rows(
        [
            candidate_rows,
            {"result": True},
            {"result": True},
            {"result": True},
            {"result": True},
        ],
        settings,
    )

    candidates = await adapter.fetch_passage_embedding_candidates(context)
    vector_a = PassageEmbeddingVector(
        child_chunk_id=child_a,
        embedding=tuple([0.125] * 1024),
    )
    vector_b = PassageEmbeddingVector(
        child_chunk_id=child_b,
        embedding=tuple([0.25] * 1024),
    )
    embeddings = (
        StagedPassageEmbedding(
            child_chunk_id=vector_a.child_chunk_id,
            embedding_model_id=vector_a.model_id,
            embedding_dimensions=vector_a.dimensions,
            embedding_vector=vector_a.embedding,
            embedding_vector_sha256=embedding_vector_sha256(vector_a.embedding),
        ),
        StagedPassageEmbedding(
            child_chunk_id=vector_b.child_chunk_id,
            embedding_model_id=vector_b.model_id,
            embedding_dimensions=vector_b.dimensions,
            embedding_vector=vector_b.embedding,
            embedding_vector_sha256=embedding_vector_sha256(vector_b.embedding),
        ),
    )
    await adapter.stage_passage_embeddings(context, embeddings)
    assert await adapter.assert_materialization_search_complete(
        context,
        expected_child_count=len(candidates),
    )
    assert await adapter.complete_embedding_and_publish(context)

    assert candidates == (
        PassageEmbeddingCandidate(
            child_chunk_id=child_a,
            content="first passage candidate",
            content_hash="a" * 64,
        ),
        PassageEmbeddingCandidate(
            child_chunk_id=child_b,
            content="second passage candidate",
            content_hash="b" * 64,
        ),
    )
    assert connection.calls == [
        (
            _SQL["fetch_passage_embedding_candidates"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.materialization_id,
            ),
        ),
        (
            _SQL["stage_passage_embedding"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.materialization_id,
                child_a,
                list(embeddings[0].embedding_vector),
                embeddings[0].embedding_vector_sha256,
            ),
        ),
        (
            _SQL["stage_passage_embedding"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.materialization_id,
                child_b,
                list(embeddings[1].embedding_vector),
                embeddings[1].embedding_vector_sha256,
            ),
        ),
        (
            _SQL["assert_search_complete"],
            (
                context.materialization_id,
                len(candidates),
                "jina-embeddings-v4",
                1024,
            ),
        ),
        (
            _SQL["complete_embedding_and_publish"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.materialization_id,
            ),
        ),
    ]


async def test_passage_embedding_gateway_requires_claim_lease_token() -> None:
    adapter, connection = adapter_with_rows([])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.fetch_passage_embedding_candidates(
            passage_embedding_context(lease_token=None)
        )

    assert raised.value.error_code == JOB_CONTEXT_LEASE_FENCE_MISSING
    assert connection.calls == []


async def test_parse_source_metadata_gateway_calls_token_fenced_function() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = parse_context()
    row = {
        "file_id": context.file_id,
        "storage_backend": "minio",
        "object_key": "users/user-1/files/file-1",
        "sha256": "e" * 64,
        "byte_size": 4096,
        "content_type": "application/pdf",
    }
    adapter, connection = adapter_with_rows([row], settings)

    metadata = await adapter.fetch_source_metadata(context)

    assert metadata == FileSourceMetadata(
        file_id=context.file_id,
        storage_backend="minio",
        object_key="users/user-1/files/file-1",
        sha256="e" * 64,
        byte_size=4096,
        content_type="application/pdf",
    )
    assert connection.calls == [
        (
            _SQL["fetch_parse_source_metadata"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.file_id,
                context.materialization_id,
            ),
        )
    ]


async def test_parse_source_metadata_gateway_requires_claim_lease_token() -> None:
    adapter, connection = adapter_with_rows([])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.fetch_source_metadata(parse_context(lease_token=None))

    assert raised.value.error_code == JOB_CONTEXT_LEASE_FENCE_MISSING
    assert connection.calls == []


async def test_parse_source_metadata_gateway_rejects_unbound_materialization() -> None:
    adapter, connection = adapter_with_rows([])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.fetch_source_metadata(parse_context(materialization_id=None))

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID
    assert connection.calls == []


async def test_parse_source_metadata_gateway_rejects_invalid_row() -> None:
    context = parse_context()
    adapter, connection = adapter_with_rows(
        [
            {
                "file_id": context.file_id,
                "storage_backend": "minio",
                "object_key": "users/user-1/files/file-1",
                "sha256": "not-a-sha",
                "byte_size": 4096,
                "content_type": "application/pdf",
            }
        ]
    )

    with pytest.raises(PermanentJobError) as raised:
        await adapter.fetch_source_metadata(context)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID
    assert connection.calls == [
        (
            _SQL["fetch_parse_source_metadata"],
            (
                context.job_id,
                adapter._settings.worker_id,
                context.lease_token,
                context.file_id,
                context.materialization_id,
            ),
        )
    ]


async def test_parse_chunk_profile_resolver_calls_token_fenced_function() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = parse_context()
    adapter, connection = adapter_with_rows(
        [{"chunk_profile_hash": "f" * 64}], settings
    )

    observed = await adapter.resolve_parse_chunk_profile(context)

    assert observed == "f" * 64
    assert connection.calls == [
        (
            _SQL["resolve_parse_chunk_profile"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.index_generation_id,
                context.materialization_id,
            ),
        )
    ]


async def test_parse_chunk_profile_resolver_rejects_missing_row() -> None:
    context = parse_context()
    adapter, connection = adapter_with_rows([None])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.resolve_parse_chunk_profile(context)

    assert raised.value.error_code == JOB_HANDLER_PARSE_PROFILE_INVALID
    assert len(connection.calls) == 1


async def test_parse_projection_gateway_calls_token_fenced_function() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = parse_context()
    batch = projection_batch(context)
    adapter, connection = adapter_with_rows([{"result": True}], settings)

    await adapter.stage_parse_projection(context, batch)

    assert len(connection.calls) == 1
    sql, parameters = connection.calls[0]
    assert sql == _SQL["stage_parse_projection"]
    assert parameters[:7] == (
        context.job_id,
        worker_id,
        context.lease_token,
        context.materialization_id,
        batch.blocks[0].artifact_set_id,
        batch.source_sha256,
        batch.chunk_profile_hash,
    )
    payloads = parameters[7:]
    assert all(isinstance(payload, Jsonb) for payload in payloads)
    blocks = jsonb_payload(payloads[0])
    parent_chunks = jsonb_payload(payloads[1])
    child_chunks = jsonb_payload(payloads[2])
    spans = jsonb_payload(payloads[3])
    search_rows = jsonb_payload(payloads[4])
    assert blocks[0]["id"] == str(batch.blocks[0].id)
    assert blocks[0]["confidence"] == 0.9
    assert parent_chunks[0]["materialization_id"] == str(context.materialization_id)
    assert child_chunks[0]["parent_chunk_id"] == str(batch.parent_chunks[0].id)
    assert (
        spans[0]["fragment_source_span_hash"]
        == batch.chunk_block_spans[0].fragment_source_span_hash
    )
    assert search_rows[0]["exact_terms"] == ["fixture", "hello"]


async def test_parse_completion_gateway_calls_terminal_finalizer() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = parse_context()
    embedding_job_id = uuid.uuid4()
    adapter, connection = adapter_with_rows([{"result": True}], settings)

    assert await adapter.complete_parse_and_enqueue_embedding(
        context,
        embedding_job_id=embedding_job_id,
    )

    assert connection.calls == [
        (
            _SQL["complete_parse_and_enqueue_embedding"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.materialization_id,
                embedding_job_id,
            ),
        )
    ]


async def test_parse_projection_gateway_requires_claim_lease_token() -> None:
    adapter, connection = adapter_with_rows([])
    context = parse_context(lease_token=None)

    with pytest.raises(PermanentJobError) as raised:
        await adapter.stage_parse_projection(context, projection_batch(context))

    assert raised.value.error_code == JOB_CONTEXT_LEASE_FENCE_MISSING
    assert connection.calls == []


async def test_parse_projection_gateway_rejects_unbound_materialization() -> None:
    adapter, connection = adapter_with_rows([])
    context = parse_context(materialization_id=None)

    with pytest.raises(PermanentJobError) as raised:
        await adapter.stage_parse_projection(context, object())  # type: ignore[arg-type]

    assert raised.value.error_code == JOB_HANDLER_PARSE_ARTIFACT_INVALID
    assert connection.calls == []


async def test_parse_projection_gateway_rejects_mismatched_batch_before_db() -> None:
    context = parse_context()
    batch = projection_batch(context)
    mismatched = replace(
        batch,
        parent_chunks=(
            replace(batch.parent_chunks[0], materialization_id=uuid.uuid4()),
        ),
    )
    adapter, connection = adapter_with_rows([])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.stage_parse_projection(context, mismatched)

    assert raised.value.error_code == JOB_HANDLER_PARSE_ARTIFACT_INVALID
    assert connection.calls == []


def test_parse_projection_gateway_preserves_only_stable_database_error_codes() -> None:
    with pytest.raises(PermanentJobError) as raised:
        _raise_stable_database_error(
            psycopg.errors.RaiseException("RAG_PARSE_PARENT_CHUNK_PROJECTION_MISMATCH")
        )
    assert raised.value.error_code == "RAG_PARSE_PARENT_CHUNK_PROJECTION_MISMATCH"

    _raise_stable_database_error(psycopg.Error("arbitrary database detail"))


async def test_purge_projection_gateway_calls_token_fenced_functions() -> None:
    worker_id = uuid.uuid4()
    settings = Settings(
        database_url="postgresql://worker:secret@db/rag",
        worker_id=worker_id,
    )
    context = purge_context()
    materialization_id = uuid.uuid4()
    projected = {
        "collection_id": context.collection_id,
        "document_id": context.document_id,
        "document_version_id": context.document_version_id,
        "index_generation_id": context.index_generation_id,
        "materialization_id": materialization_id,
        "purged_child_search_rows": 2,
        "remaining_ready_child_search_rows": 0,
    }
    adapter, connection = adapter_with_rows(
        [
            {
                "collection_id": context.collection_id,
                "document_id": context.document_id,
                "document_version_id": context.document_version_id,
                "collection_visibility_epoch": context.collection_visibility_epoch,
                "document_visibility_epoch": context.document_visibility_epoch,
                "query_visible": False,
            },
            projected,
            {"result": True},
        ],
        settings,
    )

    invisible = await adapter.mark_purge_invisible(context)
    projection = await adapter.purge_search_projection(context)
    assert await adapter.assert_purge_complete(context, projection)

    assert not invisible.query_visible
    assert projection == PurgeProjectionResult(**projected)
    assert connection.calls == [
        (
            _SQL["mark_purge_invisible"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.collection_visibility_epoch,
                context.document_visibility_epoch,
            ),
        ),
        (
            _SQL["purge_search_projection"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.index_generation_id,
                context.materialization_id,
            ),
        ),
        (
            _SQL["assert_purge_complete"],
            (
                context.job_id,
                worker_id,
                context.lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.index_generation_id,
                materialization_id,
                projection.purged_child_search_rows,
            ),
        ),
    ]


async def test_purge_projection_gateway_requires_claim_lease_token() -> None:
    adapter, connection = adapter_with_rows([])

    with pytest.raises(PermanentJobError) as raised:
        await adapter.mark_purge_invisible(purge_context(lease_token=None))

    assert raised.value.error_code == JOB_CONTEXT_LEASE_FENCE_MISSING
    assert connection.calls == []


async def test_query_evidence_gateway_calls_selected_function() -> None:
    collection_id = uuid.uuid4()
    row = {
        "collection_id": collection_id,
        "document_id": uuid.uuid4(),
        "document_version_id": uuid.uuid4(),
        "index_generation_id": uuid.uuid4(),
        "materialization_id": uuid.uuid4(),
        "parent_chunk_id": uuid.uuid4(),
        "child_chunk_id": uuid.uuid4(),
        "source_span_hash": "a" * 64,
        "content_hash": "b" * 64,
        "rank_score": 0.75,
    }
    adapter, connection = adapter_with_rows([[row]])

    candidates = await adapter.fetch_query_evidence_candidates(
        collection_ids=(collection_id,),
        query_text="selected collection query",
        limit=5,
    )

    assert len(candidates) == 1
    assert candidates[0].collection_id == collection_id
    assert candidates[0].rank_score == 0.75
    assert connection.calls == [
        (
            _SQL["fetch_query_evidence_candidates"],
            ([collection_id], "selected collection query", 5),
        )
    ]


async def test_empty_function_claims_remain_empty() -> None:
    adapter, _ = adapter_with_rows([None, None])
    assert await adapter.claim_outbox(uuid.uuid4()) is None
    assert await adapter.claim_job(uuid.uuid4()) is None


async def test_readiness_requires_one_row() -> None:
    adapter, _ = adapter_with_rows([None])
    with pytest.raises(RuntimeError, match="no row"):
        await adapter.readiness()


class FakeManagedPool:
    def __init__(self) -> None:
        self.opened = False
        self.closed = False

    async def open(self, *, wait: bool, timeout: int) -> None:
        self.opened = wait and timeout == 10

    async def check(self) -> None:
        return None

    async def close(self) -> None:
        self.closed = True


class FakeLockConnection(FakeConnection):
    def __init__(
        self,
        acquired: bool,
        rows: list[dict[str, object] | None] | None = None,
    ) -> None:
        super().__init__(rows or [])
        self.acquired = acquired
        self.closed = False

    async def execute(self, sql: str, parameters: Sequence[object]) -> FakeCursor:
        self.calls.append((sql, parameters))
        if "pg_try" in sql:
            return FakeCursor({"acquired": self.acquired})
        if "pg_advisory_unlock" in sql:
            return FakeCursor({"unlocked": True})
        return FakeCursor(self.rows.pop(0))

    async def close(self) -> None:
        self.closed = True


async def test_pool_lifecycle_and_advisory_lock(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    connection = FakeLockConnection(acquired=True)

    async def connect(*_: object, **__: object) -> FakeLockConnection:
        return connection

    monkeypatch.setattr(postgres_module.psycopg.AsyncConnection, "connect", connect)
    adapter = PostgresAdapter(
        Settings(database_url="postgresql://test"), Metrics.create()
    )
    pool = FakeManagedPool()
    adapter._pool = pool  # type: ignore[assignment]
    await adapter.open()
    assert pool.opened
    assert await adapter.acquire_worker_lock()
    assert await adapter.acquire_worker_lock()
    await adapter.close()
    assert connection.closed
    assert pool.closed


async def test_advisory_lock_contention_closes_connection(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    connection = FakeLockConnection(acquired=False)

    async def connect(*_: object, **__: object) -> FakeLockConnection:
        return connection

    monkeypatch.setattr(postgres_module.psycopg.AsyncConnection, "connect", connect)
    adapter = PostgresAdapter(
        Settings(database_url="postgresql://test"), Metrics.create()
    )
    assert not await adapter.acquire_worker_lock()
    assert connection.closed
    await adapter.release_worker_lock()


async def test_replay_adapters_call_only_operator_functions(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    successor_id = uuid.uuid4()
    connection = FakeLockConnection(
        acquired=True, rows=[{"succeeded": True}, {"result": successor_id}]
    )

    async def connect(*_: object, **__: object) -> FakeLockConnection:
        return connection

    monkeypatch.setattr(postgres_module.psycopg.AsyncConnection, "connect", connect)
    assert await replay_outbox(
        "postgresql://operator@db/rag",
        event_id=uuid.uuid4(),
        expected_error_code="FAILED_EVENT",
        operator_id=uuid.uuid4(),
        reason="approved",
    )
    assert await replay_job(
        "postgresql://operator@db/rag",
        job_id=uuid.uuid4(),
        expected_error_code="FAILED_JOB",
        successor_job_id=successor_id,
        operator_id=uuid.uuid4(),
        reason="approved",
    )
    assert connection.calls[-2][0] == _SQL["replay_outbox"]
    assert connection.calls[-1][0] == _SQL["replay_job"]


@pytest.mark.parametrize(
    ("method", "code"),
    [
        ("apply_and_ack_outbox", "RAG_STALE_OUTBOX_LEASE"),
        ("retry_outbox", "RAG_STALE_OUTBOX_LEASE"),
        ("fail_outbox", "RAG_STALE_OUTBOX_LEASE"),
        ("heartbeat_job", "RAG_STALE_JOB_LEASE"),
        ("finish_job", "RAG_STALE_JOB_LEASE"),
        ("complete_parse_and_enqueue_embedding", "RAG_STALE_JOB_LEASE"),
        ("complete_embedding_and_publish", "RAG_STALE_JOB_LEASE"),
    ],
)
async def test_stale_lease_database_codes_become_false(
    monkeypatch: pytest.MonkeyPatch, method: str, code: str
) -> None:
    adapter = PostgresAdapter(
        Settings(database_url="postgresql://test"), Metrics.create()
    )

    async def stale(*_: object, **__: object) -> None:
        raise postgres_module.psycopg.errors.RaiseException(code)

    monkeypatch.setattr(adapter, "_call", stale)
    outbox = OutboxClaim(1, uuid.uuid4(), "synthetic", 1, 2, {})
    job = JobClaim(uuid.uuid4(), "parse", 1, 2, {})
    token = uuid.uuid4()
    if method == "apply_and_ack_outbox":
        result = await adapter.apply_and_ack_outbox(
            outbox,
            token,
            scope_kind="global",
            index_generation_id=None,
            action="dispatch",
            result_hash="a" * 64,
        )
    elif method == "retry_outbox":
        result = await adapter.retry_outbox(outbox, token, "FAILED_EVENT", 30)
    elif method == "fail_outbox":
        result = await adapter.fail_outbox(outbox, token, "FAILED_EVENT")
    elif method == "heartbeat_job":
        result = await adapter.heartbeat_job(job, token)
    elif method == "finish_job":
        result = await adapter.finish_job(
            job,
            token,
            outcome="succeeded",
            error_code=None,
            retry_after_seconds=0,
        )
    elif method == "complete_parse_and_enqueue_embedding":
        result = await adapter.complete_parse_and_enqueue_embedding(
            parse_context(),
            embedding_job_id=uuid.uuid4(),
        )
    else:
        result = await adapter.complete_embedding_and_publish(
            passage_embedding_context()
        )
    assert result is False
