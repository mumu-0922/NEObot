from __future__ import annotations

import json
import uuid
from typing import Any

import httpx
import pytest

from mm_chat_rag.jina_gateway import (
    JINA_GATEWAY_RESPONSE_INVALID,
    JINA_PASSAGE_EMBEDDINGS_PATH,
    JinaPassageEmbeddingGateway,
    build_jina_passage_embedding_handler_dependencies,
)
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
)
from mm_chat_rag.provider_gateway import (
    GO_PROVIDER_GATEWAY_STATUS_INVALID,
    GO_PROVIDER_INTERNAL_TOKEN_HEADER,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

BASE_URL = "http://backend:8080"
INTERNAL_TOKEN = "unit-test-provider-gateway-token"


def _candidate(value: int, content: str) -> PassageEmbeddingCandidate:
    return PassageEmbeddingCandidate(
        child_chunk_id=uuid.UUID(f"{value:08d}-1111-4111-8111-111111111111"),
        content=content,
        content_hash=f"{value:064x}",
    )


def _vector(first: float = 0.25) -> list[float]:
    return [first, *([0.0] * 1023)]


def _response(payload: object, status: int = 200) -> httpx.Response:
    return httpx.Response(
        status,
        headers={"Content-Type": "application/json", "Content-Encoding": "identity"},
        content=json.dumps(payload).encode(),
    )


async def test_passage_gateway_uses_closed_go_dto_and_restores_candidate_order() -> (
    None
):
    candidates = (_candidate(1, "first"), _candidate(2, "second"))
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _response(
            {
                "model": "jina-embeddings-v4",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(candidates[1].child_chunk_id),
                        "embedding": _vector(2.0),
                    },
                    {
                        "passageId": str(candidates[0].child_chunk_id),
                        "embedding": _vector(1.0),
                    },
                ],
            }
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        vectors = await JinaPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        ).embed_passages(object(), candidates)

    assert [item.child_chunk_id for item in vectors] == [
        item.child_chunk_id for item in candidates
    ]
    assert vectors[0].embedding[0] == 1.0
    assert vectors[1].embedding[0] == 2.0
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(f"{BASE_URL}{JINA_PASSAGE_EMBEDDINGS_PATH}")
    assert request.headers[GO_PROVIDER_INTERNAL_TOKEN_HEADER] == INTERNAL_TOKEN
    assert "authorization" not in request.headers
    assert json.loads(request.content) == {
        "passages": [
            {"passageId": str(item.child_chunk_id), "text": item.content}
            for item in candidates
        ]
    }


async def test_passage_gateway_empty_batch_avoids_go_call() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("empty batch reached Go")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        result = await JinaPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        ).embed_passages(object(), ())
    assert result == ()
    assert calls == 0


@pytest.mark.parametrize(
    ("status", "error_type"),
    [
        (400, PermanentJobError),
        (401, PermanentJobError),
        (409, RetryableJobError),
        (503, RetryableJobError),
    ],
)
async def test_passage_gateway_maps_go_status_without_body_leaks(
    status: int,
    error_type: type[Exception],
) -> None:
    private_body = "private-provider-body"

    def handler(_: httpx.Request) -> httpx.Response:
        return _response({"error": {"private": private_body}}, status=status)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = JinaPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        with pytest.raises(error_type) as raised:
            await gateway.embed_passages(object(), (_candidate(1, "private text"),))
    assert raised.value.args[0] == GO_PROVIDER_GATEWAY_STATUS_INVALID
    assert private_body not in str(raised.value)
    assert INTERNAL_TOKEN not in str(raised.value)


@pytest.mark.parametrize(
    "payload",
    [
        {"model": "wrong", "dimensions": 1024, "vectors": []},
        {
            "model": "jina-embeddings-v4",
            "dimensions": 1024,
            "vectors": [
                {
                    "passageId": str(_candidate(1, "x").child_chunk_id),
                    "embedding": [0.1],
                }
            ],
        },
        {
            "model": "jina-embeddings-v4",
            "dimensions": 1024,
            "vectors": [
                {
                    "passageId": str(_candidate(1, "x").child_chunk_id),
                    "embedding": _vector(0.0),
                }
            ],
        },
    ],
)
async def test_passage_gateway_rejects_invalid_normalized_vectors(
    payload: dict[str, Any],
) -> None:
    def handler(_: httpx.Request) -> httpx.Response:
        return _response(payload)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = JinaPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), (_candidate(1, "source"),))
    assert raised.value.error_code in {
        JINA_GATEWAY_RESPONSE_INVALID,
        JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    }


def test_dependency_builder_requires_projection() -> None:
    with pytest.raises(PermanentJobError) as raised:
        build_jina_passage_embedding_handler_dependencies(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            projection=None,
        )
    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
