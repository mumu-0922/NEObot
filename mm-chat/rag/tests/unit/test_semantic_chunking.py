from __future__ import annotations

import uuid

import pytest

import mm_chat_rag.semantic_chunking as semantic_module
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    PassageEmbeddingCandidate,
    PassageEmbeddingVector,
)
from mm_chat_rag.provider_profile import GenerationEmbeddingProfile
from mm_chat_rag.retry import RetryableJobError
from mm_chat_rag.semantic_chunking import (
    PassageSemanticEmbeddingGateway,
    SemanticBoundaryPlanner,
    SemanticSentence,
)
from mm_chat_rag.semantic_profile import (
    SEMANTIC_CACHE_MAX_ENTRIES,
    SEMANTIC_EMBEDDING_DIMENSIONS,
    SEMANTIC_MAX_SENTENCE_BYTES,
    SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH,
)
from mm_chat_rag.structure_chunking import StructuredTextUnit


class _TopicGateway:
    def __init__(self) -> None:
        self.calls = 0

    async def embed_sentences(
        self,
        context: object,
        sentences: tuple[SemanticSentence, ...],
    ) -> tuple[tuple[float, ...], ...]:
        _ = context
        self.calls += 1
        result: list[tuple[float, ...]] = []
        for sentence in sentences:
            vector = [0.0] * 1024
            vector[0 if "alpha" in sentence.text else 1] = 1.0
            result.append(tuple(vector))
        return tuple(result)


class _FailingGateway:
    async def embed_sentences(
        self,
        context: object,
        sentences: tuple[SemanticSentence, ...],
    ) -> tuple[tuple[float, ...], ...]:
        _ = context, sentences
        raise RetryableJobError("SEMANTIC_PROVIDER_UNAVAILABLE")


class _StaticGateway:
    def __init__(self, vectors: tuple[tuple[float, ...], ...]) -> None:
        self._vectors = vectors

    async def embed_sentences(
        self,
        context: object,
        sentences: tuple[SemanticSentence, ...],
    ) -> tuple[tuple[float, ...], ...]:
        _ = context, sentences
        return self._vectors


class _PassageGateway:
    def __init__(self) -> None:
        self.candidates: tuple[PassageEmbeddingCandidate, ...] = ()

    async def embed_passages(
        self,
        context: ProcessingJobContext,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> tuple[PassageEmbeddingVector, ...]:
        _ = context
        self.candidates = candidates
        return tuple(
            PassageEmbeddingVector(
                child_chunk_id=item.child_chunk_id,
                embedding=(1.0, *([0.0] * 1023)),
                model_id="Pro/BAAI/bge-m3",
                dimensions=1024,
            )
            for item in candidates
        )


def _long_narrative() -> str:
    alpha = [f"alpha topic {index} " + "alpha " * 100 + "." for index in range(10)]
    beta = [f"beta topic {index} " + "beta " * 100 + "." for index in range(10)]
    return " ".join((*alpha, *beta))


def _bge_context() -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="parse",
        operation="initial",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=None,
        generation_embedding_profile=GenerationEmbeddingProfile(
            processor="siliconflow",
            model_id="Pro/BAAI/bge-m3",
            dimensions=1024,
        ),
    )


async def test_semantic_planner_detects_topic_shift_and_caches_success() -> None:
    gateway = _TopicGateway()
    planner = SemanticBoundaryPlanner(gateway)
    text = _long_narrative()
    units = (StructuredTextUnit(0, "paragraph", text),)

    first = await planner.plan(object(), units)
    second = await planner.plan(object(), units)

    assert first == second
    assert gateway.calls == 1
    assert len(first) == 1
    expected = len(" ".join(text.split(".")[:10]).encode()) + 1
    assert abs(first[0].boundary_bytes[0] - expected) <= 1


async def test_semantic_planner_binds_cache_and_hint_to_bge_profile() -> None:
    planner = SemanticBoundaryPlanner(_TopicGateway())

    hints = await planner.plan(
        _bge_context(),
        (StructuredTextUnit(0, "paragraph", _long_narrative()),),
    )

    assert hints[0].embedding_profile_hash == (
        SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH
    )


async def test_semantic_planner_fails_open_without_partial_hints() -> None:
    planner = SemanticBoundaryPlanner(_FailingGateway())
    units = (StructuredTextUnit(0, "paragraph", _long_narrative()),)

    assert await planner.plan(object(), units) == ()


async def test_semantic_planner_skips_short_and_structured_units() -> None:
    gateway = _TopicGateway()
    planner = SemanticBoundaryPlanner(gateway)
    units = (
        StructuredTextUnit(0, "paragraph", "short sentence."),
        StructuredTextUnit(1, "code", _long_narrative()),
    )

    assert await planner.plan(object(), units) == ()
    assert gateway.calls == 0


@pytest.mark.parametrize("bound", [0, SEMANTIC_CACHE_MAX_ENTRIES + 1])
def test_semantic_planner_rejects_unbounded_cache(bound: int) -> None:
    with pytest.raises(ValueError, match="cache bound"):
        SemanticBoundaryPlanner(_TopicGateway(), cache_max_entries=bound)


async def test_passage_semantic_gateway_preserves_sentence_identity_and_hash() -> None:
    passage = _PassageGateway()
    gateway = PassageSemanticEmbeddingGateway(passage)
    sentences = (
        SemanticSentence(uuid.uuid4(), "first sentence.", 15),
        SemanticSentence(uuid.uuid4(), "second sentence.", 31),
    )

    vectors = await gateway.embed_sentences(_bge_context(), sentences)

    assert len(vectors) == 2
    assert [item.child_chunk_id for item in passage.candidates] == [
        item.sentence_id for item in sentences
    ]
    assert [item.content for item in passage.candidates] == [
        item.text for item in sentences
    ]
    assert all(len(item.content_hash) == 64 for item in passage.candidates)


async def test_semantic_planner_skips_nonindexable_and_invalid_provider_shapes() -> (
    None
):
    long_single_sentence = "alpha " * 1000
    nonindexable = StructuredTextUnit(
        0,
        "paragraph",
        _long_narrative(),
        indexable=False,
    )
    assert (
        await SemanticBoundaryPlanner(_TopicGateway()).plan(
            object(),
            (nonindexable,),
        )
        == ()
    )
    assert (
        await SemanticBoundaryPlanner(_TopicGateway()).plan(
            object(),
            (StructuredTextUnit(1, "paragraph", long_single_sentence),),
        )
        == ()
    )

    assert (
        await SemanticBoundaryPlanner(_StaticGateway(())).plan(
            object(),
            (StructuredTextUnit(2, "paragraph", _long_narrative()),),
        )
        == ()
    )
    identical = tuple(
        (1.0, *([0.0] * 1023))
        for _ in semantic_module._sentences(_long_narrative(), "a" * 64)
    )
    assert (
        await SemanticBoundaryPlanner(_StaticGateway(identical)).plan(
            object(),
            (StructuredTextUnit(3, "paragraph", _long_narrative()),),
        )
        == ()
    )


async def test_semantic_planner_evicts_oldest_bounded_cache_entry() -> None:
    gateway = _TopicGateway()
    planner = SemanticBoundaryPlanner(gateway, cache_max_entries=1)
    first = StructuredTextUnit(0, "paragraph", _long_narrative())
    second = StructuredTextUnit(1, "paragraph", _long_narrative() + " gamma.")

    assert await planner.plan(object(), (first,))
    assert await planner.plan(object(), (second,))
    assert await planner.plan(object(), (first,))
    assert gateway.calls == 3


def test_semantic_sentence_and_vector_boundaries_fail_closed() -> None:
    content_hash = "a" * 64
    sentences = semantic_module._sentences("\n valid tail", content_hash)
    assert [item.text for item in sentences] == [" valid tail"]

    collected: list[SemanticSentence] = []
    semantic_module._append_sentence(
        collected,
        "x" * (SEMANTIC_MAX_SENTENCE_BYTES + 1),
        SEMANTIC_MAX_SENTENCE_BYTES + 1,
        content_hash,
    )
    assert collected == []
    assert semantic_module._semantic_boundaries(sentences, ((1.0,),)) == ()
    assert semantic_module._cosine_distance((1.0,), (1.0,)) == 0
    zero = (0.0,) * SEMANTIC_EMBEDDING_DIMENSIONS
    assert semantic_module._cosine_distance(zero, zero) == 0
