"""Liveness, operational readiness, and Prometheus endpoints."""

from __future__ import annotations

from dataclasses import dataclass

from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Route

from mm_chat_rag.metrics import Metrics


@dataclass(slots=True)
class ReadinessState:
    """Bounded status fields; projection unready is legal in Phase B."""

    database: str = "not_ready"
    functions: str = "not_ready"
    worker_lock: str = "not_ready"
    consumer: str = "disabled"
    projection: str = "not_ready"
    redis: str = "disabled"

    def core_ready(self) -> bool:
        """Projection and Redis do not make dark-run mechanics unready."""
        return (
            self.database == "ready"
            and self.functions == "ready"
            and self.worker_lock == "ready"
            and self.consumer in {"ready", "disabled"}
        )

    def snapshot(self) -> dict[str, str]:
        """Return only fixed-cardinality operator fields."""
        return {
            "database": self.database,
            "functions": self.functions,
            "worker_lock": self.worker_lock,
            "consumer": self.consumer,
            "projection": self.projection,
            "redis": self.redis,
        }


def create_health_app(state: ReadinessState, metrics: Metrics) -> Starlette:
    """Create a small operational app with no provider or credential routes."""

    async def health(_: Request) -> JSONResponse:
        return JSONResponse({"status": "alive"})

    async def ready(_: Request) -> JSONResponse:
        is_ready = state.core_ready()
        return JSONResponse(
            {"status": "ready" if is_ready else "not_ready", **state.snapshot()},
            status_code=200 if is_ready else 503,
        )

    async def prometheus(_: Request) -> Response:
        return Response(
            generate_latest(metrics.registry),
            media_type=CONTENT_TYPE_LATEST,
        )

    return Starlette(
        debug=False,
        routes=[
            Route("/health", health, methods=["GET"]),
            Route("/ready", ready, methods=["GET"]),
            Route("/metrics", prometheus, methods=["GET"]),
        ],
    )
