"""Optional ingestion-only semantic boundary detection with safe fallback."""

from __future__ import annotations

import hashlib
import math
import uuid
from collections import OrderedDict
from collections.abc import Coroutine
from dataclasses import dataclass
from typing import Any, Final, Protocol, cast

from mm_chat_rag.frozen_tokenizer import FROZEN_TOKENIZER
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    PassageEmbeddingCandidate,
    PassageEmbeddingGateway,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError
from mm_chat_rag.semantic_profile import (
    SEMANTIC_BREAK_PERCENTILE,
    SEMANTIC_CACHE_MAX_ENTRIES,
    SEMANTIC_EMBED_BATCH_SIZE,
    SEMANTIC_EMBEDDING_DIMENSIONS,
    SEMANTIC_MAX_BOUNDARIES,
    SEMANTIC_MAX_SENTENCE_BYTES,
    SEMANTIC_MAX_SENTENCES,
    SEMANTIC_MIN_DISTANCE,
    SEMANTIC_MIN_SENTENCES,
    SEMANTIC_MIN_SOURCE_TOKENS,
    semantic_boundary_profile_hash,
)
from mm_chat_rag.structure_chunking import SemanticBoundaryHints, StructuredTextUnit

_SEMANTIC_NAMESPACE: Final = uuid.UUID("517e472e-00ed-5bfa-9d86-cad3c2e430c8")
_NARRATIVE_KINDS: Final = frozenset(
    {"paragraph", "list", "list_item", "quote", "raw_html"}
)
_SENTENCE_ENDS: Final = frozenset("\n.!?\u3002\uff01\uff1f\uff1b;")


@dataclass(frozen=True, slots=True)
class SemanticSentence:
    """One exact source sentence admitted for embedding."""

    sentence_id: uuid.UUID
    text: str
    end_byte: int


class SemanticEmbeddingGateway(Protocol):
    """Embedding seam that cannot author or rewrite source text."""

    def embed_sentences(
        self,
        context: object,
        sentences: tuple[SemanticSentence, ...],
    ) -> Coroutine[Any, Any, tuple[tuple[float, ...], ...]]: ...


class PassageSemanticEmbeddingGateway:
    """Reuse the generation-routed passage endpoint for bounded sentences."""

    def __init__(self, passage_gateway: PassageEmbeddingGateway) -> None:
        self._passage_gateway = passage_gateway

    async def embed_sentences(
        self,
        context: object,
        sentences: tuple[SemanticSentence, ...],
    ) -> tuple[tuple[float, ...], ...]:
        candidates = tuple(
            PassageEmbeddingCandidate(
                child_chunk_id=sentence.sentence_id,
                content=sentence.text,
                content_hash=hashlib.sha256(sentence.text.encode()).hexdigest(),
            )
            for sentence in sentences
        )
        vectors = await self._passage_gateway.embed_passages(
            cast("ProcessingJobContext", context),
            candidates,
        )
        return tuple(vector.embedding for vector in vectors)


class SemanticBoundaryPlanner:
    """Plan cached semantic hints; any provider failure returns no hints."""

    def __init__(
        self,
        gateway: SemanticEmbeddingGateway,
        *,
        cache_max_entries: int = SEMANTIC_CACHE_MAX_ENTRIES,
    ) -> None:
        if not 1 <= cache_max_entries <= SEMANTIC_CACHE_MAX_ENTRIES:
            raise ValueError("semantic cache bound is invalid")
        self._gateway = gateway
        self._cache_max_entries = cache_max_entries
        self._cache: OrderedDict[tuple[str, str], tuple[int, ...]] = OrderedDict()

    async def plan(
        self,
        context: object,
        units: tuple[StructuredTextUnit, ...],
    ) -> tuple[SemanticBoundaryHints, ...]:
        """Return only valid successful hints; fallback is an empty tuple."""
        hints: list[SemanticBoundaryHints] = []
        try:
            generation_profile = (
                context.generation_embedding_profile
                if isinstance(context, ProcessingJobContext)
                else None
            )
            profile_hash = semantic_boundary_profile_hash(generation_profile)
            for unit in units:
                hint = await self._plan_unit(context, unit, profile_hash)
                if hint is not None:
                    hints.append(hint)
        except (PermanentJobError, RetryableJobError, TimeoutError, ValueError):
            return ()
        return tuple(hints)

    async def _plan_unit(
        self,
        context: object,
        unit: StructuredTextUnit,
        profile_hash: str,
    ) -> SemanticBoundaryHints | None:
        if (
            not unit.indexable
            or unit.kind not in _NARRATIVE_KINDS
            or FROZEN_TOKENIZER.count(unit.text) < SEMANTIC_MIN_SOURCE_TOKENS
        ):
            return None
        content_hash = hashlib.sha256(unit.text.encode()).hexdigest()
        cache_key = (profile_hash, content_hash)
        cached = self._cache.get(cache_key)
        if cached is not None:
            self._cache.move_to_end(cache_key)
            return _hint(unit, content_hash, profile_hash, cached)

        sentences = _sentences(unit.text, content_hash)
        if not SEMANTIC_MIN_SENTENCES <= len(sentences) <= SEMANTIC_MAX_SENTENCES:
            return None
        vectors: list[tuple[float, ...]] = []
        for start in range(0, len(sentences), SEMANTIC_EMBED_BATCH_SIZE):
            batch = sentences[start : start + SEMANTIC_EMBED_BATCH_SIZE]
            vectors.extend(await self._gateway.embed_sentences(context, batch))
        if len(vectors) != len(sentences):
            return None
        boundaries = _semantic_boundaries(sentences, tuple(vectors))
        if not boundaries:
            return None
        self._cache[cache_key] = boundaries
        self._cache.move_to_end(cache_key)
        while len(self._cache) > self._cache_max_entries:
            self._cache.popitem(last=False)
        return _hint(unit, content_hash, profile_hash, boundaries)


def _hint(
    unit: StructuredTextUnit,
    content_hash: str,
    profile_hash: str,
    boundaries: tuple[int, ...],
) -> SemanticBoundaryHints:
    return SemanticBoundaryHints(
        unit_ordinal=unit.ordinal,
        content_sha256=content_hash,
        embedding_profile_hash=profile_hash,
        boundary_bytes=boundaries,
    )


def _sentences(text: str, content_hash: str) -> tuple[SemanticSentence, ...]:
    sentences: list[SemanticSentence] = []
    start_character = 0
    byte_offset = 0
    for index, character in enumerate(text):
        byte_offset += len(character.encode("utf-8"))
        if character not in _SENTENCE_ENDS:
            continue
        sentence_text = text[start_character : index + 1]
        if sentence_text.strip():
            _append_sentence(sentences, sentence_text, byte_offset, content_hash)
        start_character = index + 1
    tail = text[start_character:]
    if tail.strip():
        _append_sentence(sentences, tail, len(text.encode()), content_hash)
    return tuple(sentences)


def _append_sentence(
    sentences: list[SemanticSentence],
    text: str,
    end_byte: int,
    content_hash: str,
) -> None:
    if len(text.encode()) > SEMANTIC_MAX_SENTENCE_BYTES:
        return
    sentence_id = uuid.uuid5(
        _SEMANTIC_NAMESPACE,
        f"{content_hash}:{len(sentences)}:{end_byte}",
    )
    sentences.append(SemanticSentence(sentence_id, text, end_byte))


def _semantic_boundaries(
    sentences: tuple[SemanticSentence, ...],
    vectors: tuple[tuple[float, ...], ...],
) -> tuple[int, ...]:
    distances = tuple(
        _cosine_distance(vectors[index], vectors[index + 1])
        for index in range(len(vectors) - 1)
    )
    if not distances:
        return ()
    ranked = sorted(distances)
    threshold_index = math.ceil(len(ranked) * SEMANTIC_BREAK_PERCENTILE) - 1
    threshold = max(SEMANTIC_MIN_DISTANCE, ranked[max(0, threshold_index)])
    candidates = [
        (distance, sentences[index].end_byte)
        for index, distance in enumerate(distances)
        if distance >= threshold
    ]
    selected = sorted(
        candidates,
        key=lambda item: (-item[0], item[1]),
    )[:SEMANTIC_MAX_BOUNDARIES]
    return tuple(sorted(boundary for _distance, boundary in selected))


def _cosine_distance(left: tuple[float, ...], right: tuple[float, ...]) -> float:
    if (
        len(left) != SEMANTIC_EMBEDDING_DIMENSIONS
        or len(right) != SEMANTIC_EMBEDDING_DIMENSIONS
    ):
        return 0.0
    dot = sum(a * b for a, b in zip(left, right, strict=True))
    left_norm = math.sqrt(sum(value * value for value in left))
    right_norm = math.sqrt(sum(value * value for value in right))
    if left_norm <= 0 or right_norm <= 0:
        return 0.0
    similarity = max(-1.0, min(1.0, dot / (left_norm * right_norm)))
    return 1.0 - similarity
