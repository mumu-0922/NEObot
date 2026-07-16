"""Single-slot durable job execution with lease heartbeats and DLQ fencing."""

from __future__ import annotations

import asyncio
import contextlib
import logging
import uuid
from collections.abc import Mapping
from typing import Protocol

from mm_chat_rag.handlers import JobHandler
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import JobClaim
from mm_chat_rag.retry import PermanentJobError, RetryableJobError, retry_or_dlq
from mm_chat_rag.settings import Settings

logger = logging.getLogger(__name__)


class JobDatabase(Protocol):
    """The token-fenced stored-function surface used by the job runner."""

    async def claim_job(self, lease_token: uuid.UUID) -> JobClaim | None: ...

    async def heartbeat_job(self, job: JobClaim, lease_token: uuid.UUID) -> bool: ...

    async def finish_job(
        self,
        job: JobClaim,
        lease_token: uuid.UUID,
        *,
        outcome: str,
        error_code: str | None,
        retry_after_seconds: int,
    ) -> bool: ...


class JobRunner:
    """Run at most one allowlisted job and abandon work when its lease is lost."""

    def __init__(
        self,
        database: JobDatabase,
        settings: Settings,
        metrics: Metrics,
        handlers: Mapping[str, JobHandler],
    ) -> None:
        self._database = database
        self._settings = settings
        self._metrics = metrics
        self._handlers = handlers

    async def run(self, stop: asyncio.Event) -> None:
        """Stop new claims on shutdown while allowing the current job to finish."""
        while not stop.is_set():
            claimed = await self.process_one()
            if not claimed:
                with contextlib.suppress(TimeoutError):
                    await asyncio.wait_for(
                        stop.wait(), timeout=self._settings.poll_interval_seconds
                    )

    async def process_one(self) -> bool:
        """Claim, heartbeat, execute, and token-CAS finish one job."""
        lease_token = uuid.uuid4()
        job = await self._database.claim_job(lease_token)
        if job is None:
            self._metrics.job_claims.labels(outcome="empty").inc()
            return False
        self._metrics.job_claims.labels(outcome="claimed").inc()
        handler = self._handlers.get(job.stage)
        if handler is None:
            await self._finish_failure(
                job,
                lease_token,
                error_code="JOB_STAGE_UNSUPPORTED",
                retry_after_seconds=30,
                permanent=False,
            )
            return True

        self._metrics.active_job.set(1)
        lease_lost = asyncio.Event()
        heartbeat = asyncio.create_task(
            self._heartbeat(job, lease_token, lease_lost), name="job-heartbeat"
        )
        execution = asyncio.create_task(handler(job), name="job-handler")
        lease_wait = asyncio.create_task(lease_lost.wait(), name="job-lease-fence")
        try:
            done, _ = await asyncio.wait(
                {execution, lease_wait}, return_when=asyncio.FIRST_COMPLETED
            )
            if execution not in done or lease_lost.is_set():
                self._metrics.job_results.labels(outcome="lease_lost").inc()
                logger.warning("job_lease_lost")
                return True
            heartbeat.cancel()
            lease_wait.cancel()
            await asyncio.gather(heartbeat, lease_wait, return_exceptions=True)
            if lease_lost.is_set():
                self._metrics.job_results.labels(outcome="lease_lost").inc()
                logger.warning("job_lease_lost")
                return True
            try:
                result = execution.result()
                if result.terminal_committed:
                    outcome = result.outcome
                else:
                    changed = await self._database.finish_job(
                        job,
                        lease_token,
                        outcome=result.outcome,
                        error_code=result.error_code,
                        retry_after_seconds=0,
                    )
                    outcome = result.outcome if changed else "stale_lease"
                self._metrics.job_results.labels(outcome=outcome).inc()
                logger.info("job_resolved", extra={"fields": {"outcome": outcome}})
            except RetryableJobError as error:
                await self._finish_failure(
                    job,
                    lease_token,
                    error_code=error.error_code,
                    retry_after_seconds=error.retry_after_seconds,
                    permanent=False,
                )
            except PermanentJobError as error:
                await self._finish_failure(
                    job,
                    lease_token,
                    error_code=error.error_code,
                    retry_after_seconds=0,
                    permanent=True,
                )
            except Exception:  # noqa: BLE001 - boundary converts to stable code
                await self._finish_failure(
                    job,
                    lease_token,
                    error_code="JOB_HANDLER_ERROR",
                    retry_after_seconds=30,
                    permanent=False,
                )
            return True
        finally:
            execution.cancel()
            heartbeat.cancel()
            lease_wait.cancel()
            await asyncio.gather(
                execution, heartbeat, lease_wait, return_exceptions=True
            )
            self._metrics.active_job.set(0)

    async def _heartbeat(
        self, job: JobClaim, lease_token: uuid.UUID, lease_lost: asyncio.Event
    ) -> None:
        while True:
            await asyncio.sleep(self._settings.heartbeat_seconds)
            try:
                lease_valid = await self._database.heartbeat_job(job, lease_token)
            except Exception:  # noqa: BLE001 - renewal errors must fail closed
                logger.warning("job_heartbeat_failed")
                lease_lost.set()
                return
            if not lease_valid:
                lease_lost.set()
                return

    async def _finish_failure(
        self,
        job: JobClaim,
        lease_token: uuid.UUID,
        *,
        error_code: str,
        retry_after_seconds: int,
        permanent: bool,
    ) -> None:
        decision = retry_or_dlq(
            attempt_count=job.max_attempts if permanent else job.attempt_count,
            max_attempts=job.max_attempts,
            error_code=error_code,
            retry_after_seconds=retry_after_seconds,
        )
        changed = await self._database.finish_job(
            job,
            lease_token,
            outcome=decision.outcome,
            error_code=decision.error_code,
            retry_after_seconds=decision.retry_after_seconds,
        )
        outcome = decision.outcome if changed else "stale_lease"
        self._metrics.job_results.labels(outcome=outcome).inc()
        logger.warning(
            "job_failed",
            extra={"fields": {"outcome": outcome, "error_code": error_code}},
        )
