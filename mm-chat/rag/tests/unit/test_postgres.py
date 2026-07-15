from __future__ import annotations

import uuid
from collections.abc import Sequence
from dataclasses import replace

import pytest

import mm_chat_rag.postgres as postgres_module
from mm_chat_rag.job_context import (
    JOB_CONTEXT_LEASE_FENCE_MISSING,
    ProcessingJobContext,
)
from mm_chat_rag.job_handler_dependencies import PurgeProjectionResult
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import JobClaim, OutboxClaim
from mm_chat_rag.postgres import (
    _SQL,
    PostgresAdapter,
    _function_succeeded,
    replay_job,
    replay_outbox,
)
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.settings import Settings


class FakeCursor:
    def __init__(self, row: dict[str, object] | None) -> None:
        self.row = row

    async def fetchone(self) -> dict[str, object] | None:
        return self.row


class AsyncContext:
    def __init__(self, value: object) -> None:
        self.value = value

    async def __aenter__(self) -> object:
        return self.value

    async def __aexit__(self, *_: object) -> None:
        return None


class FakeConnection:
    def __init__(self, rows: list[dict[str, object] | None]) -> None:
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
    rows: list[dict[str, object] | None], settings: Settings | None = None
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
        "mark_purge_invisible",
        "purge_search_projection",
        "assert_purge_complete",
        "replay_outbox",
        "replay_job",
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
    ]


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
    else:
        result = await adapter.finish_job(
            job,
            token,
            outcome="succeeded",
            error_code=None,
            retry_after_seconds=0,
        )
    assert result is False
