from __future__ import annotations

import uuid
from dataclasses import replace
from typing import cast

import pytest

from mm_chat_rag.job_context import ProcessingJobContext, ProviderAuthority
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    PassageEmbeddingCandidate,
    PassageEmbeddingVector,
)
from mm_chat_rag.provider_profile import (
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
    GenerationEmbeddingProfile,
)
from mm_chat_rag.retrieval_embedding_gateway import (
    RETRIEVAL_EMBEDDING_AUTHORITY_INVALID,
    AuthorityRoutingPassageEmbeddingGateway,
    build_profiled_passage_embedding_handler_dependencies,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.siliconflow_gateway import SiliconFlowPassageEmbeddingGateway

INTERNAL_TOKEN = "unit-test-retrieval-gateway-token"


class _RecordingGateway:
    def __init__(self, model_id: str) -> None:
        self.model_id = model_id
        self.calls = 0

    async def embed_passages(
        self,
        context: ProcessingJobContext,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> tuple[PassageEmbeddingVector, ...]:
        _ = context
        self.calls += 1
        return tuple(
            PassageEmbeddingVector(
                child_chunk_id=candidate.child_chunk_id,
                embedding=(1.0, *([0.0] * 1023)),
                model_id=self.model_id,
                dimensions=1024,
            )
            for candidate in candidates
        )


def _context(stage: str) -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage=stage,
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
        lease_token=uuid.uuid4(),
    )


def _authority(processor: str, model_id: str) -> ProviderAuthority:
    return ProviderAuthority(
        processor=processor,
        endpoint_id="admin-vault",
        model_id=model_id,
        governance_profile_id=uuid.uuid4(),
        governance_revision=1,
        governance_head_revision=1,
        collection_consent_id=uuid.uuid4(),
        collection_consent_revision=1,
    )


def _candidate() -> PassageEmbeddingCandidate:
    return PassageEmbeddingCandidate(
        child_chunk_id=uuid.uuid4(),
        content="bounded source",
        content_hash="b" * 64,
    )


async def test_router_uses_passage_job_authority_without_cross_space_fallback() -> None:
    siliconflow = _RecordingGateway(DEFAULT_SILICONFLOW_EMBEDDING_MODEL)
    router = AuthorityRoutingPassageEmbeddingGateway(
        siliconflow=cast("SiliconFlowPassageEmbeddingGateway", siliconflow),
    )
    context = replace(
        _context("passage_embedding"),
        authority=_authority("siliconflow", DEFAULT_SILICONFLOW_EMBEDDING_MODEL),
    )

    vectors = await router.embed_passages(context, (_candidate(),))

    assert vectors[0].model_id == DEFAULT_SILICONFLOW_EMBEDDING_MODEL
    assert siliconflow.calls == 1


async def test_router_uses_parse_generation_profile_for_semantic_embeddings() -> None:
    siliconflow = _RecordingGateway(DEFAULT_SILICONFLOW_EMBEDDING_MODEL)
    router = AuthorityRoutingPassageEmbeddingGateway(
        siliconflow=cast("SiliconFlowPassageEmbeddingGateway", siliconflow),
    )
    context = replace(
        _context("parse"),
        authority=_authority("mineru", "mineru-parser"),
        generation_embedding_profile=GenerationEmbeddingProfile(
            processor="siliconflow",
            model_id=DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
            dimensions=1024,
        ),
    )

    vectors = await router.embed_passages(context, (_candidate(),))

    assert vectors[0].model_id == DEFAULT_SILICONFLOW_EMBEDDING_MODEL
    assert siliconflow.calls == 1


async def test_router_rejects_parse_without_generation_profile() -> None:
    gateway = _RecordingGateway(DEFAULT_SILICONFLOW_EMBEDDING_MODEL)
    router = AuthorityRoutingPassageEmbeddingGateway(
        siliconflow=cast("SiliconFlowPassageEmbeddingGateway", gateway),
    )

    with pytest.raises(PermanentJobError) as raised:
        await router.embed_passages(_context("parse"), (_candidate(),))

    assert raised.value.error_code == RETRIEVAL_EMBEDDING_AUTHORITY_INVALID
    assert gateway.calls == 0


@pytest.mark.parametrize(
    "context",
    [
        _context("passage_embedding"),
        replace(
            _context("passage_embedding"),
            authority=_authority("jina", "jina-embeddings-v4"),
        ),
        _context("unknown"),
        replace(
            _context("parse"),
            generation_embedding_profile=GenerationEmbeddingProfile(
                processor="siliconflow",
                model_id="wrong-model",
                dimensions=1024,
            ),
        ),
    ],
)
async def test_router_rejects_missing_stale_or_unsupported_authority(
    context: ProcessingJobContext,
) -> None:
    gateway = _RecordingGateway(DEFAULT_SILICONFLOW_EMBEDDING_MODEL)
    router = AuthorityRoutingPassageEmbeddingGateway(
        siliconflow=cast("SiliconFlowPassageEmbeddingGateway", gateway),
    )

    with pytest.raises(PermanentJobError) as raised:
        await router.embed_passages(context, (_candidate(),))

    assert raised.value.error_code == RETRIEVAL_EMBEDDING_AUTHORITY_INVALID
    assert gateway.calls == 0


def test_profiled_dependency_builder_requires_projection() -> None:
    with pytest.raises(PermanentJobError) as raised:
        build_profiled_passage_embedding_handler_dependencies(
            provider_gateway_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            projection=None,
        )
    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
