from __future__ import annotations

import json
import uuid
from typing import Any

import httpx
import pytest

from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
)
from mm_chat_rag.provider_gateway import GO_PROVIDER_INTERNAL_TOKEN_HEADER
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.siliconflow_gateway import (
    SILICONFLOW_GATEWAY_RESPONSE_INVALID,
    SILICONFLOW_PASSAGE_EMBEDDINGS_PATH,
    SiliconFlowPassageEmbeddingGateway,
)

BASE_URL = "http://backend:8080"
INTERNAL_TOKEN = "unit-test-siliconflow-gateway-token"


def _candidate(value: int, content: str) -> PassageEmbeddingCandidate:
    return PassageEmbeddingCandidate(
        child_chunk_id=uuid.UUID(f"{value:08d}-1111-4111-8111-111111111111"),
        content=content,
        content_hash=f"{value:064x}",
    )


def _vector(first: float = 0.25) -> list[float]:
    return [first, *([0.0] * 1023)]


def _response(payload: object) -> httpx.Response:
    return httpx.Response(
        200,
        headers={"Content-Type": "application/json", "Content-Encoding": "identity"},
        content=json.dumps(payload).encode(),
    )


async def test_siliconflow_gateway_uses_scoped_go_path_and_restores_order() -> None:
    candidates = (_candidate(1, "first"), _candidate(2, "second"))
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _response(
            {
                "model": "Pro/BAAI/bge-m3",
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
        vectors = await SiliconFlowPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        ).embed_passages(object(), candidates)

    assert [item.child_chunk_id for item in vectors] == [
        item.child_chunk_id for item in candidates
    ]
    assert [item.embedding[0] for item in vectors] == [1.0, 2.0]
    assert {item.model_id for item in vectors} == {"Pro/BAAI/bge-m3"}
    request = requests[0]
    assert request.url == httpx.URL(f"{BASE_URL}{SILICONFLOW_PASSAGE_EMBEDDINGS_PATH}")
    assert request.headers[GO_PROVIDER_INTERNAL_TOKEN_HEADER] == INTERNAL_TOKEN
    assert "authorization" not in request.headers
    assert json.loads(request.content) == {
        "passages": [
            {"passageId": str(item.child_chunk_id), "text": item.content}
            for item in candidates
        ]
    }


@pytest.mark.parametrize(
    "payload",
    [
        {"model": "BAAI/bge-m3", "dimensions": 1024, "vectors": []},
        {
            "model": "Pro/BAAI/bge-m3",
            "dimensions": 1024,
            "vectors": [
                {
                    "passageId": str(_candidate(1, "x").child_chunk_id),
                    "embedding": [0.1],
                }
            ],
        },
        {
            "model": "Pro/BAAI/bge-m3",
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
async def test_siliconflow_gateway_rejects_wrong_profile_or_vector(
    payload: dict[str, Any],
) -> None:
    def handler(_: httpx.Request) -> httpx.Response:
        return _response(payload)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = SiliconFlowPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), (_candidate(1, "source"),))
    assert raised.value.error_code in {
        SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    }


async def test_siliconflow_gateway_returns_empty_without_provider_call() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("empty passage set reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = SiliconFlowPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        assert await gateway.embed_passages(object(), ()) == ()
    assert calls == 0


@pytest.mark.parametrize(
    ("payload", "error_code"),
    [
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [],
                "extra": True,
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": "invalid",
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [],
            },
            JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": ["invalid"],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(1, "x").child_chunk_id),
                        "embedding": _vector(),
                        "extra": True,
                    }
                ],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [{"passageId": None, "embedding": _vector()}],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [{"passageId": "invalid", "embedding": _vector()}],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": "00000000-0000-0000-0000-000000000000",
                        "embedding": _vector(),
                    }
                ],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": "AAAAAAAA-1111-4111-8111-111111111111",
                        "embedding": _vector(),
                    }
                ],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(2, "other").child_chunk_id),
                        "embedding": _vector(),
                    }
                ],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(1, "x").child_chunk_id),
                        "embedding": "invalid",
                    }
                ],
            },
            SILICONFLOW_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(1, "x").child_chunk_id),
                        "embedding": [True, *([0.0] * 1023)],
                    }
                ],
            },
            JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(1, "x").child_chunk_id),
                        "embedding": [float("nan"), *([0.0] * 1023)],
                    }
                ],
            },
            JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
        ),
        (
            {
                "model": "Pro/BAAI/bge-m3",
                "dimensions": 1024,
                "vectors": [
                    {
                        "passageId": str(_candidate(1, "x").child_chunk_id),
                        "embedding": [1e308, *([0.0] * 1023)],
                    }
                ],
            },
            JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
        ),
    ],
)
async def test_siliconflow_gateway_rejects_closed_response_contract(
    payload: object,
    error_code: str,
) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _response(payload))
    ) as client:
        gateway = SiliconFlowPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), (_candidate(1, "source"),))
    assert raised.value.error_code == error_code


async def test_siliconflow_gateway_rejects_duplicate_vector_ids() -> None:
    candidates = (_candidate(1, "first"), _candidate(2, "second"))
    repeated = {
        "passageId": str(candidates[0].child_chunk_id),
        "embedding": _vector(),
    }
    payload = {
        "model": "Pro/BAAI/bge-m3",
        "dimensions": 1024,
        "vectors": [repeated, repeated],
    }
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _response(payload))
    ) as client:
        gateway = SiliconFlowPassageEmbeddingGateway(
            provider_gateway_url=BASE_URL,
            internal_token=INTERNAL_TOKEN,
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.embed_passages(object(), candidates)
    assert raised.value.error_code == SILICONFLOW_GATEWAY_RESPONSE_INVALID
