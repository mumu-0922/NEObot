"""Process lifecycle for the Phase 15.2B durable dark-run worker."""

from __future__ import annotations

import asyncio
import contextlib
import logging
import signal
from collections.abc import Iterator, Mapping
from typing import override

import uvicorn

from mm_chat_rag.consumer import DurableConsumer
from mm_chat_rag.handlers import (
    DISPATCH_REGISTRY,
    JOB_HANDLER_REGISTRY,
    DispatchPlanner,
    JobHandler,
)
from mm_chat_rag.health import ReadinessState, create_health_app
from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.logging import configure_logging
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.postgres import PostgresAdapter
from mm_chat_rag.redis_wakeup import RedisWakeSubscriber
from mm_chat_rag.settings import Settings, SettingsError

logger = logging.getLogger(__name__)


class WorkerStartupError(RuntimeError):
    """Raised before claims when a safety gate or singleton lock fails."""


class _NoSignalServer(uvicorn.Server):
    @contextlib.contextmanager
    @override
    def capture_signals(self) -> Iterator[None]:
        yield


class Worker:
    """Own all worker services and enforce graceful claim shutdown."""

    def __init__(
        self,
        settings: Settings,
        *,
        dispatch_registry: Mapping[str, DispatchPlanner] = DISPATCH_REGISTRY,
        job_handlers: Mapping[str, JobHandler] = JOB_HANDLER_REGISTRY,
    ) -> None:
        self.settings = settings
        self.metrics = Metrics.create()
        self.state = ReadinessState(
            consumer="ready" if settings.dispatch_enabled else "disabled",
            redis="degraded" if settings.redis_url else "disabled",
        )
        self.database = PostgresAdapter(settings, self.metrics)
        self.dispatch_registry = dispatch_registry
        self.job_handlers = job_handlers
        self.stop = asyncio.Event()
        self.wake = asyncio.Event()

    def validate_promotion_gate(self) -> None:
        """Refuse all claims unless explicitly enabled registries are complete."""
        if not self.settings.dispatch_enabled:
            return
        if not self.dispatch_registry:
            raise WorkerStartupError("dispatch enabled with empty event registry")
        missing = set(self.settings.job_stages) - self.job_handlers.keys()
        if missing:
            raise WorkerStartupError("enabled job stage has no promoted handler")

    async def run(self) -> None:
        """Start, supervise, gracefully drain, and close the worker."""
        self.validate_promotion_gate()
        self.metrics.dispatch_enabled.set(int(self.settings.dispatch_enabled))
        await self.database.open()
        if not await self.database.acquire_worker_lock():
            await self.database.close()
            raise WorkerStartupError("single-worker advisory lock is already held")
        self.state.worker_lock = "ready"
        await self._refresh_readiness()

        app = create_health_app(self.state, self.metrics)
        server = _NoSignalServer(
            uvicorn.Config(
                app,
                host=self.settings.health_host,
                port=self.settings.health_port,
                log_config=None,
                access_log=False,
                lifespan="off",
            )
        )
        tasks: list[asyncio.Task[None]] = [
            asyncio.create_task(server.serve(), name="health-server"),
            asyncio.create_task(self._readiness_loop(), name="readiness-loop"),
        ]
        redis_task: asyncio.Task[None] | None = None
        if self.settings.redis_url:
            redis_task = asyncio.create_task(
                RedisWakeSubscriber(
                    self.settings.redis_url,
                    self.settings.redis_channel,
                    self.metrics,
                    lambda status: setattr(self.state, "redis", status),
                ).run(self.wake, self.stop),
                name="redis-wakeup",
            )
            tasks.append(redis_task)

        durable_tasks: list[asyncio.Task[None]] = []
        if self.settings.dispatch_enabled:
            durable_tasks = [
                asyncio.create_task(
                    DurableConsumer(
                        self.database,
                        self.settings,
                        self.metrics,
                        self.dispatch_registry,
                    ).run(self.wake, self.stop),
                    name="outbox-consumer",
                )
            ]
            if self.settings.job_stages:
                durable_tasks.append(
                    asyncio.create_task(
                        JobRunner(
                            self.database,
                            self.settings,
                            self.metrics,
                            self.job_handlers,
                        ).run(self.stop),
                        name="job-runner",
                    )
                )
            tasks.extend(durable_tasks)

        for task in tasks:
            task.add_done_callback(self._stop_on_failure)
        logger.info(
            "worker_started",
            extra={
                "fields": {
                    "dispatch_enabled": self.settings.dispatch_enabled,
                    "redis": "configured" if self.settings.redis_url else "disabled",
                }
            },
        )
        try:
            await self.stop.wait()
            if durable_tasks:
                try:
                    await asyncio.wait_for(
                        asyncio.gather(*durable_tasks),
                        timeout=self.settings.shutdown_grace_seconds,
                    )
                except TimeoutError:
                    logger.warning("shutdown_grace_expired")
                    for task in durable_tasks:
                        task.cancel()
                    await asyncio.gather(*durable_tasks, return_exceptions=True)
        finally:
            server.should_exit = True
            for task in tasks:
                if task not in durable_tasks and not task.done():
                    task.cancel()
            results = await asyncio.gather(*tasks, return_exceptions=True)
            self.state.worker_lock = "not_ready"
            await self.database.close()
            logger.info("worker_stopped")
            for result in results:
                if isinstance(result, Exception) and not isinstance(
                    result, asyncio.CancelledError
                ):
                    raise result

    def request_stop(self) -> None:
        """Stop new claims; in-flight work receives the configured grace period."""
        self.stop.set()

    def _stop_on_failure(self, task: asyncio.Task[None]) -> None:
        if task.cancelled():
            return
        if task.exception() is not None:
            self.stop.set()

    async def _readiness_loop(self) -> None:
        while not self.stop.is_set():
            await self._refresh_readiness()
            with contextlib.suppress(TimeoutError):
                await asyncio.wait_for(self.stop.wait(), timeout=5)

    async def _refresh_readiness(self) -> None:
        try:
            readiness = await self.database.readiness()
        except Exception:  # noqa: BLE001 - status boundary must not leak DB errors
            self.state.database = "not_ready"
            self.state.functions = "not_ready"
            logger.warning("database_readiness_failed")
            return
        self.state.database = "ready" if readiness.database else "not_ready"
        self.state.functions = "ready" if readiness.functions else "not_ready"
        self.state.projection = readiness.projection
        if self.settings.dispatch_enabled:
            self.state.consumer = readiness.consumer


async def async_main() -> int:
    """Construct settings and run until SIGINT/SIGTERM."""
    try:
        settings = Settings.from_env()
    except SettingsError as error:
        configure_logging("INFO")
        logger.error("settings_invalid", extra={"fields": {"reason": str(error)}})
        return 2
    configure_logging(settings.log_level)
    worker = Worker(settings)
    loop = asyncio.get_running_loop()
    for signum in (signal.SIGINT, signal.SIGTERM):
        with contextlib.suppress(NotImplementedError):
            loop.add_signal_handler(signum, worker.request_stop)
    try:
        await worker.run()
    except WorkerStartupError as error:
        logger.error("worker_startup_failed", extra={"fields": {"reason": str(error)}})
        return 1
    return 0


def main() -> None:
    """Console entry point."""
    raise SystemExit(asyncio.run(async_main()))
