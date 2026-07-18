from __future__ import annotations

import json
import logging

import httpx
from prometheus_client import generate_latest

from mm_chat_rag.health import ReadinessState, create_health_app
from mm_chat_rag.logging import RedactedJsonFormatter, redact
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.retry import RetryableJobError

INTERNAL_TOKEN = "unit-test-internal-token"


class FakeQueryEmbeddingGateway:
    def __init__(self, *, fail: bool = False) -> None:
        self.fail = fail
        self.queries: list[str] = []

    async def embed_query(self, query: str) -> tuple[float, ...]:
        self.queries.append(query)
        if self.fail:
            raise RetryableJobError("JINA_GATEWAY_REQUEST_FAILED")
        return tuple(0.001 for _ in range(1024))


class FakeRerankGateway:
    def __init__(self, *, fail: bool = False) -> None:
        self.fail = fail
        self.calls: list[tuple[str, tuple[str, ...]]] = []

    async def rerank(
        self,
        query: str,
        documents: tuple[str, ...],
    ) -> tuple[tuple[int, float], ...]:
        self.calls.append((query, documents))
        if self.fail:
            raise RetryableJobError("JINA_GATEWAY_REQUEST_FAILED")
        return tuple((index, 0.75 - index) for index in range(len(documents)))


def test_recursive_redaction_removes_credentials_and_payloads() -> None:
    result = redact(
        {
            "database_url": "postgresql://user:password@db/private?sslkey=x",
            "nested": {"apiKey": "super-secret", "safe": "ok"},
            "url": "https://user:pass@example.test/path?token=secret",
            "payload": {"text": "private"},
            "items": ["safe"],
        }
    )
    assert result == {
        "database_url": "[REDACTED]",
        "nested": {"apiKey": "[REDACTED]", "safe": "ok"},
        "url": "https://example.test/path",
        "payload": "[REDACTED]",
        "items": ["safe"],
    }
    assert "TRUNCATED" in str(redact("x" * 600))
    assert redact("token=secret safe") == "token=[REDACTED] safe"
    assert (
        redact(
            'HTTP Request: PUT https://cdn.example.test/file.zip?Signature=secret "200"'
        )
        == 'HTTP Request: PUT https://cdn.example.test/file.zip "200"'
    )
    assert redact("http://host:invalid/path") == "[REDACTED]"


def test_json_formatter_never_serializes_exception_text() -> None:
    formatter = RedactedJsonFormatter()
    try:
        raise RuntimeError("token=must-not-leak")
    except RuntimeError:
        record = logging.LogRecord(
            "test",
            logging.ERROR,
            __file__,
            1,
            "operation_failed",
            (),
            exc_info=__import__("sys").exc_info(),
        )
    record.fields = {"token": "must-not-leak", "outcome": "failed"}
    parsed = json.loads(formatter.format(record))
    assert parsed["event"] == "operation_failed"
    assert parsed["error_type"] == "RuntimeError"
    assert parsed["fields"]["token"] == "[REDACTED]"
    assert "must-not-leak" not in json.dumps(parsed)


async def test_health_readiness_and_metrics_endpoints() -> None:
    metrics = Metrics.create()
    state = ReadinessState()
    app = create_health_app(state, metrics)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport, base_url="http://worker.test"
    ) as client:
        assert (await client.get("/health")).json() == {"status": "alive"}
        assert (await client.get("/ready")).status_code == 503
        state.database = "ready"
        state.functions = "ready"
        state.worker_lock = "ready"
        state.projection = "not_ready"
        response = await client.get("/ready")
        assert response.status_code == 200
        assert response.json()["projection"] == "not_ready"
        assert "rag_worker_advisory_lock" in (await client.get("/metrics")).text


async def test_private_query_embedding_route_authenticates_and_bounds_output() -> None:
    metrics = Metrics.create()
    gateway = FakeQueryEmbeddingGateway()
    app = create_health_app(
        ReadinessState(),
        metrics,
        query_embedding=gateway,
        internal_token=INTERNAL_TOKEN,
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://worker.test",
    ) as client:
        assert (
            await client.post(
                "/internal/retrieval/query-embedding",
                json={"query": "private question"},
            )
        ).status_code == 401
        response = await client.post(
            "/internal/retrieval/query-embedding",
            headers={"Authorization": f"Bearer {INTERNAL_TOKEN}"},
            json={"query": "private question"},
        )

    assert response.status_code == 200
    assert response.headers["cache-control"] == "no-store"
    assert response.json()["model"] == "jina-embeddings-v4"
    assert response.json()["dimensions"] == 1024
    assert len(response.json()["embedding"]) == 1024
    assert gateway.queries == ["private question"]


async def test_private_query_embedding_route_fails_closed_without_leaks() -> None:
    metrics = Metrics.create()
    gateway = FakeQueryEmbeddingGateway(fail=True)
    app = create_health_app(
        ReadinessState(),
        metrics,
        query_embedding=gateway,
        internal_token=INTERNAL_TOKEN,
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://worker.test",
    ) as client:
        response = await client.post(
            "/internal/retrieval/query-embedding",
            headers={"Authorization": f"Bearer {INTERNAL_TOKEN}"},
            json={"query": "must-not-leak"},
        )

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "QUERY_EMBEDDING_UNAVAILABLE"
    assert "must-not-leak" not in response.text


async def test_private_rerank_route_authenticates_and_bounds_output() -> None:
    metrics = Metrics.create()
    gateway = FakeRerankGateway()
    app = create_health_app(
        ReadinessState(),
        metrics,
        reranker=gateway,
        internal_token=INTERNAL_TOKEN,
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://worker.test",
    ) as client:
        assert (
            await client.post(
                "/internal/retrieval/rerank",
                json={"query": "private", "documents": ["source"]},
            )
        ).status_code == 401
        response = await client.post(
            "/internal/retrieval/rerank",
            headers={"Authorization": f"Bearer {INTERNAL_TOKEN}"},
            json={"query": "private", "documents": ["first", "second"]},
        )

    assert response.status_code == 200
    assert response.headers["cache-control"] == "no-store"
    assert response.json() == {
        "model": "jina-reranker-v3",
        "results": [
            {"index": 0, "relevanceScore": 0.75},
            {"index": 1, "relevanceScore": -0.25},
        ],
    }
    assert gateway.calls == [("private", ("first", "second"))]


async def test_private_rerank_route_fails_closed_without_source_leaks() -> None:
    metrics = Metrics.create()
    gateway = FakeRerankGateway(fail=True)
    app = create_health_app(
        ReadinessState(),
        metrics,
        reranker=gateway,
        internal_token=INTERNAL_TOKEN,
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://worker.test",
    ) as client:
        response = await client.post(
            "/internal/retrieval/rerank",
            headers={"Authorization": f"Bearer {INTERNAL_TOKEN}"},
            json={"query": "private-query", "documents": ["private-source"]},
        )

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "RERANK_UNAVAILABLE"
    assert "private-query" not in response.text
    assert "private-source" not in response.text


def test_metrics_have_only_bounded_labels() -> None:
    metrics = Metrics.create()
    metrics.loop_iterations.labels(kind="poll").inc()
    metrics.outbox_results.labels(outcome="applied").inc()
    output = generate_latest(metrics.registry).decode()
    assert 'kind="poll"' in output
    assert 'outcome="applied"' in output
    assert "event_id" not in output
    assert "document_id" not in output
