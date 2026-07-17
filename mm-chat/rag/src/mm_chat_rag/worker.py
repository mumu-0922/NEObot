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
from mm_chat_rag.jina_gateway import build_jina_passage_embedding_handler_dependencies
from mm_chat_rag.job_handler_dependencies import (
    ParseHandlerDependencies,
    ParseProjectionGateway,
    PassageEmbeddingProjectionGateway,
    PurgeHandlerDependencies,
    PurgeProjectionGateway,
    admitted_parse_handler_with_dependencies,
    admitted_passage_embedding_handler_with_dependencies,
    admitted_purge_handler_with_dependencies,
)
from mm_chat_rag.jobs import JobRunner
from mm_chat_rag.logging import configure_logging
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.mineru_gateway import (
    MinerULocalBatchGateway,
    MinerULocalBatchResultArchiveProvider,
    MinerUResultArchiveProvider,
    MinerUTextBaselineArchiveParserGateway,
)
from mm_chat_rag.native_gateway import (
    AuthorityRoutingParserGateway,
    NativeSandboxParserGateway,
)
from mm_chat_rag.postgres import PostgresAdapter
from mm_chat_rag.redis_wakeup import RedisWakeSubscriber
from mm_chat_rag.settings import Settings, SettingsError
from mm_chat_rag.source_gateway import (
    GoSourceObjectBytesGateway,
    ObjectStoreDocumentSourceGateway,
    SourceMetadataGateway,
)

logger = logging.getLogger(__name__)


class WorkerStartupError(RuntimeError):
    """Raised before claims when a safety gate or singleton lock fails."""


def build_promoted_job_handler_registry(
    settings: Settings,
    *,
    parse_source_metadata: SourceMetadataGateway | None = None,
    parse_projection: ParseProjectionGateway | None = None,
    parse_archive_provider: MinerUResultArchiveProvider | None = None,
    passage_embedding_projection: PassageEmbeddingProjectionGateway,
    purge_projection: PurgeProjectionGateway,
) -> Mapping[str, JobHandler]:
    """Build explicitly enabled job handlers without mutating frozen registries."""
    handlers: dict[str, JobHandler] = {}
    if (
        "parse" in settings.job_stages
        and parse_source_metadata is not None
        and parse_projection is not None
        and parse_archive_provider is not None
    ):
        parse_dependencies = ParseHandlerDependencies(
            document_source=ObjectStoreDocumentSourceGateway(
                metadata=parse_source_metadata,
                objects=GoSourceObjectBytesGateway(
                    base_url=settings.source_gateway_url or "",
                    internal_token=settings.source_gateway_token or "",
                    worker_id=settings.worker_id,
                ),
            ),
            parser=AuthorityRoutingParserGateway(
                mineru=MinerUTextBaselineArchiveParserGateway(parse_archive_provider),
                native=NativeSandboxParserGateway(),
            ),
            projection=parse_projection,
        )
        handlers["parse"] = admitted_parse_handler_with_dependencies(
            parse_dependencies,
            settings.provider_profile,
        )
    if "passage_embedding" in settings.job_stages:
        embedding_dependencies = build_jina_passage_embedding_handler_dependencies(
            api_key=settings.jina_api_key,
            projection=passage_embedding_projection,
        )
        handlers["passage_embedding"] = (
            admitted_passage_embedding_handler_with_dependencies(
                embedding_dependencies,
                settings.provider_profile,
            )
        )
    if "purge" in settings.job_stages:
        handlers["purge"] = admitted_purge_handler_with_dependencies(
            PurgeHandlerDependencies(projection=purge_projection)
        )
    return handlers


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
        consumer_status = (
            "ready" if settings.dispatch_enabled and dispatch_registry else "disabled"
        )
        self.state = ReadinessState(
            consumer=consumer_status,
            redis="degraded" if settings.redis_url else "disabled",
        )
        self.database = PostgresAdapter(settings, self.metrics)
        self.dispatch_registry = dispatch_registry
        if job_handlers is JOB_HANDLER_REGISTRY and settings.job_stages:
            parse_source_metadata: SourceMetadataGateway | None = None
            parse_projection: ParseProjectionGateway | None = None
            parse_archive_provider: MinerUResultArchiveProvider | None = None
            if "parse" in settings.job_stages:
                parse_source_metadata = self.database
                parse_projection = self.database
                parse_archive_provider = MinerULocalBatchResultArchiveProvider(
                    MinerULocalBatchGateway(
                        settings.mineru_api_key,
                        result_proxy_url=settings.mineru_result_proxy_url,
                    )
                )
            self.job_handlers = build_promoted_job_handler_registry(
                settings,
                parse_source_metadata=parse_source_metadata,
                parse_projection=parse_projection,
                parse_archive_provider=parse_archive_provider,
                passage_embedding_projection=self.database,
                purge_projection=self.database,
            )
        else:
            self.job_handlers = job_handlers
        self.stop = asyncio.Event()
        self.wake = asyncio.Event()

    def validate_promotion_gate(self) -> None:
        """Refuse all claims unless explicitly enabled registries are complete."""
        if not self.settings.dispatch_enabled:
            return
        if not self.dispatch_registry and not self.settings.job_stages:
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
            if self.dispatch_registry:
                durable_tasks.append(
                    asyncio.create_task(
                        DurableConsumer(
                            self.database,
                            self.settings,
                            self.metrics,
                            self.dispatch_registry,
                        ).run(self.wake, self.stop),
                        name="outbox-consumer",
                    )
                )
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
