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
    JinaQueryEmbeddingGateway,
    build_jina_passage_embedding_handler_dependencies,
)
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
    StagedPassageEmbedding,
    admitted_passage_embedding_handler_with_dependencies,
)
from mm_chat_rag.models import JobClaim
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
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


async def test_jina_gateway_sends_locked_query_request_and_maps_vector() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(_embedding_payload(1))

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        vector = await JinaQueryEmbeddingGateway(
            SECRET,
            client=client,
        ).embed_query("  semantic question  ")

    assert len(vector) == 1024
    assert vector[0] == 0.125
    assert len(requests) == 1
    body = json.loads(requests[0].content)
    assert body == {
        "dimensions": 1024,
        "embedding_type": "float",
        "input": [{"text": "semantic question"}],
        "late_chunking": False,
        "model": "jina-embeddings-v4",
        "return_multivector": False,
        "return_tokenized_input": False,
        "task": "retrieval.query",
        "truncate": False,
    }


@pytest.mark.parametrize("query", ["", "   ", "界" * 2049])
async def test_jina_query_gateway_rejects_unsafe_query_before_http(
    query: str,
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid query reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = JinaQueryEmbeddingGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError):
            await gateway.embed_query(query)
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


class FakePassageEmbeddingProjectionGateway:
    def __init__(self, candidates: tuple[PassageEmbeddingCandidate, ...]) -> None:
        self._candidates = candidates
        self.calls: list[str] = []
        self.staged: list[tuple[StagedPassageEmbedding, ...]] = []
        self.expected_child_count: int | None = None

    async def fetch_passage_embedding_candidates(
        self, context: object
    ) -> tuple[PassageEmbeddingCandidate, ...]:
        self.calls.append("fetch_candidates")
        assert context is not None
        return self._candidates

    async def stage_passage_embeddings(
        self,
        context: object,
        embeddings: tuple[StagedPassageEmbedding, ...],
    ) -> None:
        self.calls.append("stage_embeddings")
        assert context is not None
        self.staged.append(embeddings)

    async def assert_materialization_search_complete(
        self,
        context: object,
        *,
        expected_child_count: int,
    ) -> bool:
        self.calls.append("assert_complete")
        assert context is not None
        self.expected_child_count = expected_child_count
        return True

    async def complete_embedding_and_publish(self, context: object) -> bool:
        self.calls.append("complete_publish")
        assert context is not None
        return True


def _valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def _embedding_claim() -> JobClaim:
    return JobClaim.from_row(
        {
            "id": uuid.uuid4(),
            "stage": "passage_embedding",
            "operation": "initial",
            "collection_id": uuid.uuid4(),
            "document_id": uuid.uuid4(),
            "document_version_id": uuid.uuid4(),
            "file_id": uuid.uuid4(),
            "index_generation_id": uuid.uuid4(),
            "materialization_id": uuid.uuid4(),
            "processor": "jina",
            "endpoint_id": "hosted",
            "model_id": "jina-embeddings-v4",
            "governance_profile_id": uuid.uuid4(),
            "governance_revision": 1,
            "governance_head_revision": 1,
            "collection_consent_id": uuid.uuid4(),
            "collection_consent_revision": 1,
            "collection_acl_revision": 1,
            "collection_visibility_epoch": 1,
            "collection_processing_revision": 1,
            "document_visibility_epoch": 1,
            "attempt_count": 1,
            "max_attempts": 3,
            "request_hash": "c" * 64,
            "legacy_projection_unbound": False,
        }
    )


async def test_jina_dependency_bundle_runs_admitted_embedding_handler() -> None:
    requests: list[httpx.Request] = []
    candidates = _candidates()
    projection = FakePassageEmbeddingProjectionGateway(candidates)

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(_embedding_payload(len(candidates)))

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        dependencies = build_jina_passage_embedding_handler_dependencies(
            api_key=SECRET,
            projection=projection,
            client=client,
        )
        admitted_handler = admitted_passage_embedding_handler_with_dependencies(
            dependencies,
            _valid_profile(),
        )
        result = await admitted_handler(_embedding_claim())

    assert result.outcome == "succeeded"
    assert projection.calls == [
        "fetch_candidates",
        "stage_embeddings",
        "assert_complete",
        "complete_publish",
    ]
    assert projection.expected_child_count == 2
    assert len(projection.staged) == 1
    staged = projection.staged[0]
    assert [item.child_chunk_id for item in staged] == [
        candidate.child_chunk_id for candidate in candidates
    ]
    assert {item.embedding_dimensions for item in staged} == {1024}
    assert {item.embedding_model_id for item in staged} == {"jina-embeddings-v4"}
    assert all(len(item.embedding_vector_sha256) == 64 for item in staged)
    assert len(requests) == 1
    assert requests[0].url == httpx.URL(JINA_EMBEDDINGS_URL)


async def test_jina_dependency_bundle_requires_projection_before_http() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("unconfigured projection reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        with pytest.raises(PermanentJobError) as raised:
            build_jina_passage_embedding_handler_dependencies(
                api_key=SECRET,
                projection=None,
                client=client,
            )

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
    assert calls == 0
