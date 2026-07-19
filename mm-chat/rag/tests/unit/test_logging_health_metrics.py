from __future__ import annotations

import json
import logging

import httpx
from prometheus_client import generate_latest

from mm_chat_rag.health import ReadinessState, create_health_app
from mm_chat_rag.logging import RedactedJsonFormatter, redact
from mm_chat_rag.metrics import Metrics


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


async def test_old_private_provider_routes_are_retired() -> None:
    app = create_health_app(ReadinessState(), Metrics.create())
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://worker.test",
    ) as client:
        query = await client.post("/internal/retrieval/query-embedding", json={})
        rerank = await client.post("/internal/retrieval/rerank", json={})

    assert query.status_code == 404
    assert rerank.status_code == 404


def test_metrics_have_only_bounded_labels() -> None:
    metrics = Metrics.create()
    metrics.loop_iterations.labels(kind="poll").inc()
    metrics.outbox_results.labels(outcome="applied").inc()
    output = generate_latest(metrics.registry).decode()
    assert 'kind="poll"' in output
    assert 'outcome="applied"' in output
    assert "event_id" not in output
    assert "document_id" not in output
