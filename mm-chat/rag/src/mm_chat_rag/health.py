"""Liveness, operational readiness, and Prometheus endpoints."""

from __future__ import annotations

import hmac
import json
from dataclasses import dataclass
from typing import Final

from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Route

from mm_chat_rag.jina_gateway import QueryEmbeddingGateway
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

_MAX_QUERY_REQUEST_BYTES: Final = 4096


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


def create_health_app(
    state: ReadinessState,
    metrics: Metrics,
    *,
    query_embedding: QueryEmbeddingGateway | None = None,
    internal_token: str | None = None,
) -> Starlette:
    """Create a small private operational HTTP application."""

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

    async def query_embedding_route(request: Request) -> JSONResponse:
        if query_embedding is None or not internal_token:
            return _fixed_error(503, "QUERY_EMBEDDING_UNAVAILABLE")
        query, request_error = await _decode_query_request(request, internal_token)
        if request_error is not None:
            return _fixed_error(*request_error)
        if query is None:
            return _fixed_error(400, "QUERY_REQUEST_INVALID")
        try:
            vector = await query_embedding.embed_query(query)
        except (PermanentJobError, RetryableJobError):
            return _fixed_error(503, "QUERY_EMBEDDING_UNAVAILABLE")
        return JSONResponse(
            {
                "model": DEFAULT_JINA_EMBEDDING_MODEL,
                "dimensions": DEFAULT_JINA_EMBEDDING_DIMENSIONS,
                "embedding": list(vector),
            },
            headers={"Cache-Control": "no-store"},
        )

    return Starlette(
        debug=False,
        routes=[
            Route("/health", health, methods=["GET"]),
            Route("/ready", ready, methods=["GET"]),
            Route("/metrics", prometheus, methods=["GET"]),
            Route(
                "/internal/retrieval/query-embedding",
                query_embedding_route,
                methods=["POST"],
            ),
        ],
    )


def _fixed_error(status: int, code: str) -> JSONResponse:
    return JSONResponse(
        {"error": {"code": code, "message": "request failed"}},
        status_code=status,
        headers={"Cache-Control": "no-store"},
    )


async def _decode_query_request(
    request: Request,
    internal_token: str,
) -> tuple[str | None, tuple[int, str] | None]:
    authorization = request.headers.get("authorization", "")
    if not hmac.compare_digest(authorization, f"Bearer {internal_token}"):
        return None, (401, "UNAUTHORIZED")
    content_type = request.headers.get("content-type", "").split(";", 1)[0]
    if content_type.strip() != "application/json":
        return None, (415, "UNSUPPORTED_MEDIA_TYPE")
    raw = bytearray()
    async for chunk in request.stream():
        if len(raw) + len(chunk) > _MAX_QUERY_REQUEST_BYTES:
            return None, (413, "QUERY_REQUEST_TOO_LARGE")
        raw.extend(chunk)
    try:
        payload = json.loads(raw.decode("utf-8", errors="strict"))
    except (json.JSONDecodeError, UnicodeError):
        return None, (400, "QUERY_REQUEST_INVALID")
    if (
        not isinstance(payload, dict)
        or set(payload) != {"query"}
        or not isinstance(payload.get("query"), str)
    ):
        return None, (400, "QUERY_REQUEST_INVALID")
    return payload["query"], None
