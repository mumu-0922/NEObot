from __future__ import annotations

import asyncio
import json
import uuid
from typing import cast

import pytest

import mm_chat_rag.replay as replay_module
from mm_chat_rag.handlers import DispatchPlan, JobHandler, JobResult
from mm_chat_rag.models import FunctionReadiness, JobClaim, OutboxClaim
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.replay import run
from mm_chat_rag.settings import Settings
from mm_chat_rag.worker import Worker, WorkerStartupError

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"

EVENT_ID = uuid.UUID("10000000-0000-0000-0000-000000000001")
JOB_ID = uuid.UUID("20000000-0000-0000-0000-000000000002")
SUCCESSOR_ID = uuid.UUID("30000000-0000-0000-0000-000000000003")
OPERATOR_ID = uuid.UUID("40000000-0000-0000-0000-000000000004")


async def test_replay_is_dry_run_by_default(capsys: pytest.CaptureFixture[str]) -> None:
    result = await run(
        ["outbox", "--id", str(EVENT_ID), "--expected-error-code", "FAILED_EVENT"]
    )
    assert result == 0
    output = json.loads(capsys.readouterr().out)
    assert output == {
        "kind": "outbox",
        "id": str(EVENT_ID),
        "expected_error_code": "FAILED_EVENT",
        "mode": "dry-run",
    }


async def test_execute_outbox_calls_operator_function(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    captured: dict[str, object] = {}

    async def fake_replay(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return True

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator:secret@db/rag")
    monkeypatch.setattr(replay_module, "replay_outbox", fake_replay)
    result = await run(
        [
            "outbox",
            "--id",
            str(EVENT_ID),
            "--expected-error-code",
            "FAILED_EVENT",
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "incident approved",
            "--execute",
        ]
    )
    assert result == 0
    assert captured["event_id"] == EVENT_ID
    assert captured["operator_id"] == OPERATOR_ID
    assert "secret" not in capsys.readouterr().out


async def test_execute_job_requires_and_passes_successor(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured: dict[str, object] = {}

    async def fake_replay(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return False

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator@db/rag")
    monkeypatch.setattr(replay_module, "replay_job", fake_replay)
    result = await run(
        [
            "job",
            "--id",
            str(JOB_ID),
            "--expected-error-code",
            "FAILED_JOB",
            "--successor-job-id",
            str(SUCCESSOR_ID),
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "approved",
            "--execute",
        ]
    )
    assert result == 1
    assert captured["successor_job_id"] == SUCCESSOR_ID


@pytest.mark.parametrize(
    "arguments",
    [
        ["outbox", "--id", str(EVENT_ID), "--expected-error-code", "bad text"],
        [
            "outbox",
            "--id",
            str(EVENT_ID),
            "--expected-error-code",
            "FAILED_EVENT",
            "--execute",
        ],
        [
            "job",
            "--id",
            str(JOB_ID),
            "--expected-error-code",
            "FAILED_JOB",
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "approved",
            "--execute",
        ],
    ],
)
async def test_replay_invalid_or_incomplete_inputs_fail_closed(
    arguments: list[str],
) -> None:
    with pytest.raises(SystemExit, match="2"):
        await run(arguments)


def event_planner(_: OutboxClaim) -> DispatchPlan:
    return DispatchPlan("global", None, "dispatch", {"synthetic_action": "apply"})


async def job_handler(_: JobClaim) -> JobResult:
    return JobResult()


def provider_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def test_worker_default_is_dark_and_enabled_empty_registry_is_rejected() -> None:
    dark = Worker(Settings(database_url="postgresql://test"))
    dark.validate_promotion_gate()
    enabled = Worker(Settings(database_url="postgresql://test", dispatch_enabled=True))
    with pytest.raises(WorkerStartupError, match="empty event registry"):
        enabled.validate_promotion_gate()


def test_worker_rejects_missing_stage_handler_and_accepts_synthetic_gate() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    with pytest.raises(WorkerStartupError, match="no promoted handler"):
        Worker(
            settings, dispatch_registry={"synthetic": event_planner}
        ).validate_promotion_gate()
    Worker(
        settings,
        dispatch_registry={"synthetic": event_planner},
        job_handlers={"parse": cast("JobHandler", job_handler)},
    ).validate_promotion_gate()


def test_worker_accepts_job_only_promotion_without_outbox_registry() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("purge",),
    )
    worker = Worker(settings, job_handlers={"purge": cast("JobHandler", job_handler)})

    worker.validate_promotion_gate()

    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_accepts_outbox_only_promotion_without_job_stages() -> None:
    settings = Settings(database_url="postgresql://test", dispatch_enabled=True)
    worker = Worker(settings, dispatch_registry={"synthetic": event_planner})

    worker.validate_promotion_gate()

    assert worker.job_handlers == {}
    assert worker.state.consumer == "ready"


def test_worker_auto_promotes_only_purge_stage_from_settings() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("purge",),
    )
    worker = Worker(settings)

    worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"purge"}
    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_auto_promotes_passage_embedding_stage_from_settings() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("passage_embedding",),
        jina_api_key="fake-jina-key",
        provider_profile=provider_profile(),
    )
    worker = Worker(settings)

    worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"passage_embedding"}
    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_does_not_auto_promote_parse_stage() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse", "passage_embedding", "purge"),
        mineru_api_key="fake-mineru-token",
        jina_api_key="fake-jina-key",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    worker = Worker(settings)

    with pytest.raises(WorkerStartupError, match="no promoted handler"):
        worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"passage_embedding", "purge"}


async def test_worker_readiness_refresh_preserves_dark_run_consumer() -> None:
    worker = Worker(Settings(database_url="postgresql://test"))

    class ReadyDatabase:
        async def readiness(self) -> FunctionReadiness:
            return FunctionReadiness(
                database=True,
                functions=True,
                consumer="ready",
                projection="not_ready",
            )

    worker.database = ReadyDatabase()  # type: ignore[assignment]
    await worker._refresh_readiness()
    assert worker.state.database == "ready"
    assert worker.state.functions == "ready"
    assert worker.state.consumer == "disabled"


async def test_worker_readiness_failure_is_bounded() -> None:
    worker = Worker(Settings(database_url="postgresql://test"))

    class FailedDatabase:
        async def readiness(self) -> FunctionReadiness:
            raise RuntimeError("postgresql://user:secret@db")

    worker.database = FailedDatabase()  # type: ignore[assignment]
    await worker._refresh_readiness()
    assert worker.state.database == "not_ready"
    assert worker.state.functions == "not_ready"


class LifecycleDatabase:
    def __init__(self, acquired: bool = True) -> None:
        self.acquired = acquired
        self.opened = False
        self.closed = False
        self.claim_count = 0

    async def open(self) -> None:
        self.opened = True

    async def acquire_worker_lock(self) -> bool:
        return self.acquired

    async def readiness(self) -> FunctionReadiness:
        return FunctionReadiness(
            database=True,
            functions=True,
            consumer="disabled",
            projection="not_ready",
        )

    async def close(self) -> None:
        self.closed = True

    async def claim_outbox(self, lock_token: uuid.UUID) -> None:
        self.claim_count += 1


async def test_dark_run_lifecycle_never_claims() -> None:
    worker = Worker(Settings(database_url="postgresql://test", health_port=0))
    database = LifecycleDatabase()
    worker.database = database  # type: ignore[assignment]
    task = asyncio.create_task(worker.run())
    for _ in range(50):
        if worker.state.worker_lock == "ready":
            break
        await asyncio.sleep(0.01)
    worker.request_stop()
    await task
    assert database.opened
    assert database.closed
    assert worker.state.consumer == "disabled"


async def test_lock_contention_exits_before_services() -> None:
    worker = Worker(Settings(database_url="postgresql://test", health_port=0))
    database = LifecycleDatabase(acquired=False)
    worker.database = database  # type: ignore[assignment]
    with pytest.raises(WorkerStartupError, match="already held"):
        await worker.run()
    assert database.closed


async def test_promoted_synthetic_lifecycle_polls_without_real_stages() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        poll_interval_seconds=0.005,  # type: ignore[arg-type]
        health_port=0,
    )
    worker = Worker(settings, dispatch_registry={"synthetic.created": event_planner})
    database = LifecycleDatabase()
    worker.database = database  # type: ignore[assignment]
    task = asyncio.create_task(worker.run())
    for _ in range(50):
        if database.claim_count:
            break
        await asyncio.sleep(0.01)
    worker.request_stop()
    await task
    assert database.claim_count > 0
