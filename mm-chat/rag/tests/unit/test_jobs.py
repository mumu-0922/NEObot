from __future__ import annotations

import asyncio
import uuid

import pytest

from mm_chat_rag.handlers import JobResult
from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import JobClaim
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError
from mm_chat_rag.settings import Settings

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"

class FakeJobDatabase:
    def __init__(
        self,
        jobs: list[JobClaim | None],
        heartbeats: list[bool | Exception] | None = None,
    ) -> None:
        self.jobs = jobs
        self.heartbeats = heartbeats if heartbeats is not None else [True]
        self.tokens: list[uuid.UUID] = []
        self.finishes: list[tuple[str, str | None, int]] = []

    async def claim_job(self, lease_token: uuid.UUID) -> JobClaim | None:
        self.tokens.append(lease_token)
        return self.jobs.pop(0) if self.jobs else None

    async def heartbeat_job(self, job: JobClaim, lease_token: uuid.UUID) -> bool:
        heartbeat = self.heartbeats.pop(0) if self.heartbeats else True
        if isinstance(heartbeat, Exception):
            raise heartbeat
        return heartbeat

    async def finish_job(
        self,
        job: JobClaim,
        lease_token: uuid.UUID,
        *,
        outcome: str,
        error_code: str | None,
        retry_after_seconds: int,
    ) -> bool:
        self.finishes.append((outcome, error_code, retry_after_seconds))
        return True


def job(attempt: int = 1, maximum: int = 3) -> JobClaim:
    return JobClaim(uuid.uuid4(), "parse", attempt, maximum, {})


def provider_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def settings() -> Settings:
    return Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )


async def success_handler(_: JobClaim) -> JobResult:
    return JobResult()


async def retry_handler(_: JobClaim) -> JobResult:
    raise RetryableJobError("PROVIDER_TIMEOUT", 17)


async def permanent_handler(_: JobClaim) -> JobResult:
    raise PermanentJobError("MALWARE_DETECTED")


async def broken_handler(_: JobClaim) -> JobResult:
    raise RuntimeError("provider body must never become error code")


async def test_successful_job_is_finished() -> None:
    database = FakeJobDatabase([job()])
    runner = JobRunner(
        database, settings(), Metrics.create(), {"parse": success_handler}
    )
    assert await runner.process_one()
    assert database.finishes == [("succeeded", None, 0)]


async def test_retryable_failure_retries_then_dlqs_at_limit() -> None:
    retry_db = FakeJobDatabase([job(1, 2)])
    runner = JobRunner(retry_db, settings(), Metrics.create(), {"parse": retry_handler})
    await runner.process_one()
    assert retry_db.finishes == [("retry", "PROVIDER_TIMEOUT", 17)]
    dlq_db = FakeJobDatabase([job(2, 2)])
    runner = JobRunner(dlq_db, settings(), Metrics.create(), {"parse": retry_handler})
    await runner.process_one()
    assert dlq_db.finishes == [("failed", "PROVIDER_TIMEOUT", 0)]


async def test_permanent_and_unknown_exceptions_use_stable_codes() -> None:
    permanent_db = FakeJobDatabase([job()])
    await JobRunner(
        permanent_db,
        settings(),
        Metrics.create(),
        {"parse": permanent_handler},
    ).process_one()
    assert permanent_db.finishes == [("failed", "MALWARE_DETECTED", 0)]
    broken_db = FakeJobDatabase([job()])
    await JobRunner(
        broken_db,
        settings(),
        Metrics.create(),
        {"parse": broken_handler},
    ).process_one()
    assert broken_db.finishes == [("retry", "JOB_HANDLER_ERROR", 30)]


async def test_missing_handler_is_not_executed() -> None:
    database = FakeJobDatabase([job()])
    await JobRunner(database, settings(), Metrics.create(), {}).process_one()
    assert database.finishes == [("retry", "JOB_STAGE_UNSUPPORTED", 30)]


async def test_lease_loss_cancels_handler_without_finish() -> None:
    cancelled = asyncio.Event()

    async def slow_handler(_: JobClaim) -> JobResult:
        try:
            await asyncio.sleep(10)
        finally:
            cancelled.set()
        return JobResult()

    database = FakeJobDatabase([job()], heartbeats=[False])
    fast = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        heartbeat_seconds=0,
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    await JobRunner(
        database, fast, Metrics.create(), {"parse": slow_handler}
    ).process_one()
    assert cancelled.is_set()
    assert database.finishes == []


async def test_heartbeat_exception_cancels_handler_without_finish() -> None:
    cancelled = asyncio.Event()

    async def slow_handler(_: JobClaim) -> JobResult:
        try:
            await asyncio.Event().wait()
        finally:
            cancelled.set()
        return JobResult()

    database = FakeJobDatabase([job()], heartbeats=[RuntimeError("database down")])
    fast = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        heartbeat_seconds=0,
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    await JobRunner(
        database, fast, Metrics.create(), {"parse": slow_handler}
    ).process_one()
    assert cancelled.is_set()
    assert database.finishes == []


async def test_process_cancellation_awaits_children_without_finish() -> None:
    handler_started = asyncio.Event()
    handler_cancelled = asyncio.Event()
    heartbeat_started = asyncio.Event()
    heartbeat_cancelled = asyncio.Event()

    class BlockingHeartbeatDatabase(FakeJobDatabase):
        async def heartbeat_job(self, job: JobClaim, lease_token: uuid.UUID) -> bool:
            heartbeat_started.set()
            try:
                await asyncio.Event().wait()
            finally:
                heartbeat_cancelled.set()
            return True

    async def blocking_handler(_: JobClaim) -> JobResult:
        handler_started.set()
        try:
            await asyncio.Event().wait()
        finally:
            handler_cancelled.set()
        return JobResult()

    database = BlockingHeartbeatDatabase([job()])
    fast = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        heartbeat_seconds=0,
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    process = asyncio.create_task(
        JobRunner(
            database, fast, Metrics.create(), {"parse": blocking_handler}
        ).process_one(),
        name="test-process-one",
    )
    await handler_started.wait()
    await heartbeat_started.wait()

    process.cancel()
    with pytest.raises(asyncio.CancelledError):
        await process

    assert handler_cancelled.is_set()
    assert heartbeat_cancelled.is_set()
    assert database.finishes == []
    child_names = {"job-handler", "job-heartbeat", "job-lease-fence"}
    assert not [
        task
        for task in asyncio.all_tasks()
        if task is not asyncio.current_task()
        and task.get_name() in child_names
        and not task.done()
    ]


async def test_runner_stops_without_claiming_more_work() -> None:
    database = FakeJobDatabase([None])
    runner = JobRunner(database, settings(), Metrics.create(), {})
    stop = asyncio.Event()
    stop.set()
    await runner.run(stop)
    assert database.tokens == []
