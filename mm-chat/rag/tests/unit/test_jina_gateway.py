from __future__ import annotations

import json
import uuid
from typing import Any

import httpx
import pytest

from mm_chat_rag.jina_gateway import (
    JINA_EMBEDDINGS_URL,
    JINA_GATEWAY_CREDENTIALS_MISSING,
    JINA_GATEWAY_REQUEST_FAILED,
    JINA_GATEWAY_STATUS_INVALID,
    JinaPassageEmbeddingGateway,
)
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

CONTENT_HASH_A = "a" * 64
CONTENT_HASH_B = "b" * 64
SECRET = "unit-test-jina-key"


def _candidate(content: str, content_hash: str) -> PassageEmbeddingCandidate:
    return PassageEmbeddingCandidate(
        child_chunk_id=uuid.uuid4(),
        content=content,
        content_hash=content_hash,
    )


def _candidates() -> tuple[PassageEmbeddingCandidate, ...]:
    return (
        _candidate("First passage", CONTENT_HASH_A),
        _candidate("Second passage", CONTENT_HASH_B),
    )


def _embedding_payload(
    count: int,
    *,
    dimensions: int = 1024,
    model: str = "jina-embeddings-v4",
) -> dict[str, Any]:
    return {
        "data": [
            {
                "embedding": [0.125 + index / 1000] * dimensions,
                "index": index,
                "object": "embedding",
            }
            for index in range(count)
        ],
        "model": model,
        "object": "list",
        "request_id": "sensitive-provider-request-id",
        "usage": {"prompt_tokens": 11, "total_tokens": 11},
    }


def _json_response(payload: object, *, status: int = 200) -> httpx.Response:
    content = json.dumps(payload, separators=(",", ":")).encode()
    return httpx.Response(
        status,
        headers={"Content-Type": "application/json; charset=utf-8"},
        content=content,
    )


async def test_jina_gateway_missing_key_fails_before_http() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("missing key reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        with pytest.raises(PermanentJobError) as raised:
            JinaPassageEmbeddingGateway(None, client=client)

    assert raised.value.error_code == JINA_GATEWAY_CREDENTIALS_MISSING
    assert calls == 0


async def test_jina_gateway_sends_locked_passage_request_and_maps_vectors() -> None:
    requests: list[httpx.Request] = []
    candidates = _candidates()

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(_embedding_payload(len(candidates)))

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        vectors = await gateway.embed_passages(object(), candidates)

    assert [vector.child_chunk_id for vector in vectors] == [
        candidate.child_chunk_id for candidate in candidates
    ]
    assert {len(vector.embedding) for vector in vectors} == {1024}
    assert [vector.embedding[0] for vector in vectors] == [0.125, 0.126]
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(JINA_EMBEDDINGS_URL)
    assert request.headers["authorization"] == f"Bearer {SECRET}"
    body = json.loads(request.content)
    assert body == {
        "dimensions": 1024,
        "embedding_type": "float",
        "input": [{"text": "First passage"}, {"text": "Second passage"}],
        "late_chunking": False,
        "model": "jina-embeddings-v4",
        "return_multivector": False,
        "return_tokenized_input": False,
        "task": "retrieval.passage",
        "truncate": False,
    }
    assert SECRET.encode() not in request.content


async def test_jina_gateway_empty_candidates_make_no_http_call() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("empty batch reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        assert await gateway.embed_passages(object(), ()) == ()

    assert calls == 0


async def test_jina_gateway_rejects_provider_count_mismatch() -> None:
    candidates = _candidates()

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _json_response(_embedding_payload(1)))
    ) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), candidates)

    assert raised.value.error_code == JOB_HANDLER_EMBEDDING_COUNT_MISMATCH
    assert SECRET not in str(raised.value)


@pytest.mark.parametrize(
    "payload",
    [
        _embedding_payload(1, dimensions=1023),
        {
            **_embedding_payload(1),
            "data": [
                {
                    "embedding": [0.1] * 1023 + [float("nan")],
                    "index": 0,
                    "object": "embedding",
                }
            ],
        },
    ],
)
async def test_jina_gateway_rejects_invalid_vectors(payload: dict[str, Any]) -> None:
    candidates = (_candidate("Only passage", CONTENT_HASH_A),)

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _json_response(payload))
    ) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), candidates)

    assert raised.value.error_code == JOB_HANDLER_EMBEDDING_VECTOR_INVALID
    assert "sensitive-provider-request-id" not in str(raised.value)


async def test_jina_gateway_retries_redacted_provider_status() -> None:
    transport = httpx.MockTransport(
        lambda _: _json_response({"error": SECRET}, status=503)
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.embed_passages(
                object(),
                (_candidate("Only", CONTENT_HASH_A),),
            )

    assert raised.value.error_code == JINA_GATEWAY_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30
    assert SECRET not in str(raised.value)


async def test_jina_gateway_retries_redacted_transport_failure() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadError(f"sensitive transport detail {SECRET}", request=request)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = JinaPassageEmbeddingGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.embed_passages(
                object(),
                (_candidate("Only", CONTENT_HASH_A),),
            )

    assert raised.value.error_code == JINA_GATEWAY_REQUEST_FAILED
    assert SECRET not in str(raised.value)
