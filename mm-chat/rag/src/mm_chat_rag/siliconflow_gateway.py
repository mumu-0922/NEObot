"""Bounded SiliconFlow BGE passage embeddings through Go's provider gateway."""

from __future__ import annotations

import math
import uuid
from typing import Final, NoReturn, cast

import httpx

from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    PassageEmbeddingCandidate,
    PassageEmbeddingVector,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.provider_gateway import GoProviderGateway, JsonObject, JsonValue
from mm_chat_rag.provider_profile import (
    DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
)
from mm_chat_rag.retry import PermanentJobError

SILICONFLOW_GATEWAY_RESPONSE_INVALID: Final = "SILICONFLOW_GATEWAY_RESPONSE_INVALID"
SILICONFLOW_PASSAGE_EMBEDDINGS_PATH: Final = (
    "/internal/rag/providers/siliconflow/embeddings"
)
MAX_SILICONFLOW_RESPONSE_BYTES: Final = 4 * 1024 * 1024


class SiliconFlowPassageEmbeddingGateway:
    """Passage adapter that never receives a reusable SiliconFlow API Key."""

    def __init__(
        self,
        *,
        provider_gateway_url: str,
        internal_token: str,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._gateway = GoProviderGateway(
            base_url=provider_gateway_url,
            internal_token=internal_token,
            client=client,
        )

    async def embed_passages(
        self,
        context: object,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> tuple[PassageEmbeddingVector, ...]:
        """Return ordered fixed 1024-dimensional Pro BGE-M3 vectors."""
        _ = context
        if not candidates:
            return ()
        payload = await self._gateway.post_json(
            SILICONFLOW_PASSAGE_EMBEDDINGS_PATH,
            _embedding_request_body(candidates),
            max_response_bytes=MAX_SILICONFLOW_RESPONSE_BYTES,
        )
        return _embedding_vectors_from_payload(payload, candidates)


def _embedding_request_body(
    candidates: tuple[PassageEmbeddingCandidate, ...],
) -> JsonObject:
    return {
        "passages": [
            {"passageId": str(candidate.child_chunk_id), "text": candidate.content}
            for candidate in candidates
        ]
    }


def _embedding_vectors_from_payload(
    payload: JsonObject,
    candidates: tuple[PassageEmbeddingCandidate, ...],
) -> tuple[PassageEmbeddingVector, ...]:
    if set(payload) != {"model", "dimensions", "vectors"}:
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    if (
        payload.get("model") != DEFAULT_SILICONFLOW_EMBEDDING_MODEL
        or payload.get("dimensions") != DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS
    ):
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    raw_vectors = _json_list(payload.get("vectors"))
    if len(raw_vectors) != len(candidates):
        _reject(JOB_HANDLER_EMBEDDING_COUNT_MISMATCH)
    by_id = {candidate.child_chunk_id: candidate for candidate in candidates}
    vectors: dict[uuid.UUID, PassageEmbeddingVector] = {}
    for raw_vector in raw_vectors:
        item = _json_object(raw_vector)
        if set(item) != {"passageId", "embedding"}:
            _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
        passage_id = _uuid(item.get("passageId"))
        if passage_id not in by_id or passage_id in vectors:
            _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
        vectors[passage_id] = PassageEmbeddingVector(
            child_chunk_id=passage_id,
            embedding=_embedding_vector(item.get("embedding")),
            model_id=DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
            dimensions=DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
        )
    if set(vectors) != set(by_id):
        _reject(JOB_HANDLER_EMBEDDING_COUNT_MISMATCH)
    return tuple(vectors[candidate.child_chunk_id] for candidate in candidates)


def _embedding_vector(value: JsonValue | None) -> tuple[float, ...]:
    raw = _json_list(value)
    if len(raw) != DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS:
        _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
    vector: list[float] = []
    norm = 0.0
    for item in raw:
        if isinstance(item, bool) or not isinstance(item, int | float):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        number = float(item)
        if not math.isfinite(number):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        vector.append(number)
        norm += number * number
    if not math.isfinite(norm) or norm <= 0:
        _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
    return tuple(vector)


def _json_object(value: object) -> JsonObject:
    if not isinstance(value, dict):
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    return cast("JsonObject", value)


def _json_list(value: object) -> list[JsonValue]:
    if not isinstance(value, list):
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    return cast("list[JsonValue]", value)


def _uuid(value: object) -> uuid.UUID:
    if not isinstance(value, str):
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    try:
        parsed = uuid.UUID(value)
    except ValueError:
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    if parsed.int == 0 or str(parsed) != value.lower():
        _reject(SILICONFLOW_GATEWAY_RESPONSE_INVALID)
    return parsed


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))
