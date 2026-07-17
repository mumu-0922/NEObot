"""Bounded Jina passage, query embedding, and rerank gateways."""

from __future__ import annotations

import json
import math
from typing import Final, NoReturn, Protocol, cast

import httpx

from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
    PassageEmbeddingHandlerDependencies,
    PassageEmbeddingProjectionGateway,
    PassageEmbeddingVector,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
    DEFAULT_JINA_RERANK_MODEL,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

JINA_GATEWAY_CREDENTIALS_MISSING: Final = "JINA_GATEWAY_CREDENTIALS_MISSING"
JINA_GATEWAY_REQUEST_FAILED: Final = "JINA_GATEWAY_REQUEST_FAILED"
JINA_GATEWAY_STATUS_INVALID: Final = "JINA_GATEWAY_STATUS_INVALID"
JINA_GATEWAY_RESPONSE_INVALID: Final = "JINA_GATEWAY_RESPONSE_INVALID"
JINA_GATEWAY_RESPONSE_TOO_LARGE: Final = "JINA_GATEWAY_RESPONSE_TOO_LARGE"
JINA_EMBEDDINGS_URL: Final = "https://api.jina.ai/v1/embeddings"
JINA_RERANK_URL: Final = "https://api.jina.ai/v1/rerank"
JINA_PASSAGE_EMBEDDING_TASK: Final = "retrieval.passage"
JINA_QUERY_EMBEDDING_TASK: Final = "retrieval.query"
JINA_TIMEOUT: Final = httpx.Timeout(connect=5.0, read=30.0, write=15.0, pool=5.0)
JINA_LIMITS: Final = httpx.Limits(max_connections=1, max_keepalive_connections=1)
JINA_RETRY_AFTER_SECONDS: Final = 30
MAX_JINA_API_KEY_BYTES: Final = 4096
MAX_JINA_RESPONSE_BYTES: Final = 16 * 1024 * 1024
MAX_JINA_QUERY_BYTES: Final = 2048
MAX_JINA_RERANK_DOCUMENTS: Final = 20
MAX_JINA_RERANK_DOCUMENT_BYTES: Final = 64 * 1024
MAX_JINA_RERANK_TOTAL_DOCUMENT_BYTES: Final = 1024 * 1024
HTTP_OK: Final = 200
_VISIBLE_ASCII_MIN: Final = 33
_VISIBLE_ASCII_MAX: Final = 126


class QueryEmbeddingGateway(Protocol):
    """Private query-to-vector seam used by the worker HTTP boundary."""

    async def embed_query(self, query: str) -> tuple[float, ...]:
        """Return one validated 1024-dimensional retrieval query vector."""
        ...


class RerankGateway(Protocol):
    """Private query-and-document rerank seam used by the HTTP boundary."""

    async def rerank(
        self,
        query: str,
        documents: tuple[str, ...],
    ) -> tuple[tuple[int, float], ...]:
        """Return one validated score for every input document index."""
        ...


def build_jina_passage_embedding_handler_dependencies(
    *,
    api_key: str | None,
    projection: PassageEmbeddingProjectionGateway | None,
    client: httpx.AsyncClient | None = None,
) -> PassageEmbeddingHandlerDependencies:
    """Build the default-off Jina + projection dependency bundle.

    This is only a composition seam. Importing or calling it does not register a
    production handler; callers must still pass the returned bundle to an
    explicitly promoted handler in a later gate.
    """
    if projection is None:
        _reject_permanent(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    return PassageEmbeddingHandlerDependencies(
        embedding=JinaPassageEmbeddingGateway(api_key, client=client),
        projection=projection,
    )


class JinaPassageEmbeddingGateway:
    """Jina-compatible implementation of ``PassageEmbeddingGateway``."""

    def __init__(
        self,
        api_key: str | None,
        *,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._api_key = _validate_api_key(api_key)
        self._client = client

    async def embed_passages(
        self,
        context: object,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> tuple[PassageEmbeddingVector, ...]:
        """Return 1024-dimensional Jina passage embeddings for candidates."""
        _ = context
        if not candidates:
            return ()
        body = _embedding_request_body(candidates)
        if self._client is not None:
            payload = await _post_embeddings(self._client, self._api_key, body)
            return _embedding_vectors_from_payload(payload, candidates)
        async with httpx.AsyncClient(
            timeout=JINA_TIMEOUT,
            limits=JINA_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _post_embeddings(client, self._api_key, body)
            return _embedding_vectors_from_payload(payload, candidates)


class JinaQueryEmbeddingGateway:
    """Jina-compatible query embedding gateway for private Go retrieval."""

    def __init__(
        self,
        api_key: str | None,
        *,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._api_key = _validate_api_key(api_key)
        self._client = client

    async def embed_query(self, query: str) -> tuple[float, ...]:
        normalized_query = _normalize_query(query)
        body = _query_embedding_request_body(normalized_query)
        if self._client is not None:
            payload = await _post_embeddings(self._client, self._api_key, body)
            return _query_embedding_from_payload(payload)
        async with httpx.AsyncClient(
            timeout=JINA_TIMEOUT,
            limits=JINA_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _post_embeddings(client, self._api_key, body)
            return _query_embedding_from_payload(payload)


class JinaRerankGateway:
    """Jina reranker-v3 gateway for already-authorized source text."""

    def __init__(
        self,
        api_key: str | None,
        *,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._api_key = _validate_api_key(api_key)
        self._client = client

    async def rerank(
        self,
        query: str,
        documents: tuple[str, ...],
    ) -> tuple[tuple[int, float], ...]:
        normalized_query = _normalize_query(query)
        normalized_documents = _validate_rerank_documents(documents)
        body = _rerank_request_body(normalized_query, normalized_documents)
        if self._client is not None:
            payload = await _post_json(
                self._client,
                self._api_key,
                JINA_RERANK_URL,
                body,
            )
            return _rerank_results_from_payload(payload, len(normalized_documents))
        async with httpx.AsyncClient(
            timeout=JINA_TIMEOUT,
            limits=JINA_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _post_json(
                client,
                self._api_key,
                JINA_RERANK_URL,
                body,
            )
            return _rerank_results_from_payload(payload, len(normalized_documents))


def _embedding_request_body(
    candidates: tuple[PassageEmbeddingCandidate, ...],
) -> JsonObject:
    return {
        "dimensions": DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        "embedding_type": "float",
        "input": [{"text": candidate.content} for candidate in candidates],
        "late_chunking": False,
        "model": DEFAULT_JINA_EMBEDDING_MODEL,
        "return_multivector": False,
        "return_tokenized_input": False,
        "task": JINA_PASSAGE_EMBEDDING_TASK,
        "truncate": False,
    }


def _query_embedding_request_body(query: str) -> JsonObject:
    return {
        "dimensions": DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        "embedding_type": "float",
        "input": [{"text": query}],
        "late_chunking": False,
        "model": DEFAULT_JINA_EMBEDDING_MODEL,
        "return_multivector": False,
        "return_tokenized_input": False,
        "task": JINA_QUERY_EMBEDDING_TASK,
        "truncate": False,
    }


def _rerank_request_body(query: str, documents: tuple[str, ...]) -> JsonObject:
    return {
        "documents": list(documents),
        "model": DEFAULT_JINA_RERANK_MODEL,
        "query": query,
        "return_documents": False,
        "return_embeddings": False,
        "top_n": len(documents),
    }


async def _post_embeddings(
    client: httpx.AsyncClient,
    api_key: str,
    body: JsonObject,
) -> JsonObject:
    return await _post_json(client, api_key, JINA_EMBEDDINGS_URL, body)


async def _post_json(
    client: httpx.AsyncClient,
    api_key: str,
    url: str,
    body: JsonObject,
) -> JsonObject:
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    try:
        async with client.stream(
            "POST",
            url,
            headers=headers,
            json=body,
        ) as response:
            if response.status_code != HTTP_OK:
                _reject_retryable(JINA_GATEWAY_STATUS_INVALID)
            return _decode_json_response(await _read_bounded_response(response))
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(JINA_GATEWAY_REQUEST_FAILED)


def _embedding_vectors_from_payload(
    payload: JsonObject,
    candidates: tuple[PassageEmbeddingCandidate, ...],
) -> tuple[PassageEmbeddingVector, ...]:
    if payload.get("model") != DEFAULT_JINA_EMBEDDING_MODEL:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    data = _json_list(payload.get("data"), JINA_GATEWAY_RESPONSE_INVALID)
    if len(data) != len(candidates):
        _reject_permanent(JOB_HANDLER_EMBEDDING_COUNT_MISMATCH)
    by_index: dict[int, PassageEmbeddingVector] = {}
    for raw_item in data:
        item = _json_object(raw_item, JINA_GATEWAY_RESPONSE_INVALID)
        index = _non_negative_int(item.get("index"), JINA_GATEWAY_RESPONSE_INVALID)
        if index >= len(candidates) or index in by_index:
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
        vector = _embedding_vector(item.get("embedding"))
        by_index[index] = PassageEmbeddingVector(
            child_chunk_id=candidates[index].child_chunk_id,
            embedding=vector,
            model_id=DEFAULT_JINA_EMBEDDING_MODEL,
            dimensions=DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        )
    if set(by_index) != set(range(len(candidates))):
        _reject_permanent(JOB_HANDLER_EMBEDDING_COUNT_MISMATCH)
    return tuple(by_index[index] for index in range(len(candidates)))


def _query_embedding_from_payload(payload: JsonObject) -> tuple[float, ...]:
    if payload.get("model") != DEFAULT_JINA_EMBEDDING_MODEL:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    data = _json_list(payload.get("data"), JINA_GATEWAY_RESPONSE_INVALID)
    if len(data) != 1:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    item = _json_object(data[0], JINA_GATEWAY_RESPONSE_INVALID)
    if _non_negative_int(item.get("index"), JINA_GATEWAY_RESPONSE_INVALID) != 0:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    return _embedding_vector(item.get("embedding"))


def _rerank_results_from_payload(
    payload: JsonObject,
    document_count: int,
) -> tuple[tuple[int, float], ...]:
    if payload.get("model") != DEFAULT_JINA_RERANK_MODEL:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    raw_results = _json_list(
        payload.get("results"),
        JINA_GATEWAY_RESPONSE_INVALID,
    )
    if len(raw_results) != document_count:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    results: list[tuple[int, float]] = []
    seen: set[int] = set()
    for raw_result in raw_results:
        result = _json_object(raw_result, JINA_GATEWAY_RESPONSE_INVALID)
        if set(result) != {"index", "relevance_score"}:
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
        index = _non_negative_int(
            result.get("index"),
            JINA_GATEWAY_RESPONSE_INVALID,
        )
        if index >= document_count or index in seen:
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
        seen.add(index)
        results.append(
            (
                index,
                _finite_number(
                    result.get("relevance_score"),
                    JINA_GATEWAY_RESPONSE_INVALID,
                ),
            )
        )
    if seen != set(range(document_count)):
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    return tuple(results)


def _embedding_vector(value: JsonValue | None) -> tuple[float, ...]:
    raw = _json_list(value, JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
    if len(raw) != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
        _reject_permanent(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
    return tuple(
        _finite_number(item, JOB_HANDLER_EMBEDDING_VECTOR_INVALID) for item in raw
    )


async def _read_bounded_response(response: httpx.Response) -> bytes:
    if response.is_stream_consumed:
        raw_content = response.content
        if len(raw_content) > MAX_JINA_RESPONSE_BYTES:
            _reject_permanent(JINA_GATEWAY_RESPONSE_TOO_LARGE)
        return raw_content
    raw = bytearray()
    async for chunk in response.aiter_raw():
        if len(raw) + len(chunk) > MAX_JINA_RESPONSE_BYTES:
            _reject_permanent(JINA_GATEWAY_RESPONSE_TOO_LARGE)
        raw.extend(chunk)
    return bytes(raw)


def _decode_json_response(raw: bytes) -> JsonObject:
    try:
        parsed = json.loads(raw.decode("utf-8", errors="strict"))
    except (json.JSONDecodeError, UnicodeError):
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    return _json_object(parsed, JINA_GATEWAY_RESPONSE_INVALID)


def _json_object(value: object, error_code: str) -> JsonObject:
    if not isinstance(value, dict):
        _reject_permanent(error_code)
    return cast("JsonObject", value)


def _json_list(value: object, error_code: str) -> list[JsonValue]:
    if not isinstance(value, list):
        _reject_permanent(error_code)
    return cast("list[JsonValue]", value)


def _non_negative_int(value: object, error_code: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        _reject_permanent(error_code)
    return value


def _finite_number(value: JsonValue | None, error_code: str) -> float:
    if isinstance(value, bool) or not isinstance(value, int | float):
        _reject_permanent(error_code)
    number = float(value)
    if not math.isfinite(number):
        _reject_permanent(error_code)
    return number


def _validate_api_key(value: str | None) -> str:
    if value is None or not isinstance(value, str):
        _reject_permanent(JINA_GATEWAY_CREDENTIALS_MISSING)
    if (
        not value
        or value != value.strip()
        or len(value.encode("utf-8")) > MAX_JINA_API_KEY_BYTES
        or any(
            ord(character) < _VISIBLE_ASCII_MIN or ord(character) > _VISIBLE_ASCII_MAX
            for character in value
        )
    ):
        _reject_permanent(JINA_GATEWAY_CREDENTIALS_MISSING)
    return value


def _normalize_query(value: str) -> str:
    if not isinstance(value, str):
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    normalized = value.strip()
    if not normalized or len(normalized.encode("utf-8")) > MAX_JINA_QUERY_BYTES:
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    return normalized


def _validate_rerank_documents(documents: tuple[str, ...]) -> tuple[str, ...]:
    if (
        not isinstance(documents, tuple)
        or not 1 <= len(documents) <= MAX_JINA_RERANK_DOCUMENTS
    ):
        _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    total_bytes = 0
    for document in documents:
        if not isinstance(document, str) or not document.strip():
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
        document_bytes = len(document.encode("utf-8"))
        if document_bytes > MAX_JINA_RERANK_DOCUMENT_BYTES:
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
        total_bytes += document_bytes
        if total_bytes > MAX_JINA_RERANK_TOTAL_DOCUMENT_BYTES:
            _reject_permanent(JINA_GATEWAY_RESPONSE_INVALID)
    return documents


def _reject_permanent(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_retryable(error_code: str) -> NoReturn:
    raise RetryableJobError(
        stable_error_code(error_code),
        retry_after_seconds=JINA_RETRY_AFTER_SECONDS,
    )
