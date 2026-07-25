"""Generation-authority routing for the sole SiliconFlow vector space."""

from __future__ import annotations

from typing import Final, NoReturn

import httpx

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    PassageEmbeddingCandidate,
    PassageEmbeddingHandlerDependencies,
    PassageEmbeddingProjectionGateway,
    PassageEmbeddingVector,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.provider_profile import (
    GenerationEmbeddingProfile,
    ProviderProfileError,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.siliconflow_gateway import SiliconFlowPassageEmbeddingGateway

RETRIEVAL_EMBEDDING_AUTHORITY_INVALID: Final = "RETRIEVAL_EMBEDDING_AUTHORITY_INVALID"


class AuthorityRoutingPassageEmbeddingGateway:
    """Select one provider only from the generation/job authority."""

    def __init__(
        self,
        *,
        siliconflow: SiliconFlowPassageEmbeddingGateway,
    ) -> None:
        self._siliconflow = siliconflow

    async def embed_passages(
        self,
        context: ProcessingJobContext,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> tuple[PassageEmbeddingVector, ...]:
        profile = _context_embedding_profile(context)
        if profile.processor == "siliconflow":
            return await self._siliconflow.embed_passages(context, candidates)
        return _reject(RETRIEVAL_EMBEDDING_AUTHORITY_INVALID)


def build_profiled_passage_embedding_handler_dependencies(
    *,
    provider_gateway_url: str,
    internal_token: str,
    projection: PassageEmbeddingProjectionGateway | None,
    client: httpx.AsyncClient | None = None,
) -> PassageEmbeddingHandlerDependencies:
    """Build a generation-authority routed provider dependency bundle."""
    if projection is None:
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    return PassageEmbeddingHandlerDependencies(
        embedding=AuthorityRoutingPassageEmbeddingGateway(
            siliconflow=SiliconFlowPassageEmbeddingGateway(
                provider_gateway_url=provider_gateway_url,
                internal_token=internal_token,
                client=client,
            ),
        ),
        projection=projection,
    )


def _context_embedding_profile(
    context: ProcessingJobContext,
) -> GenerationEmbeddingProfile:
    if context.stage == "passage_embedding":
        authority = context.authority
        if authority is None:
            return _reject(RETRIEVAL_EMBEDDING_AUTHORITY_INVALID)
        return _validated_profile(
            GenerationEmbeddingProfile(
                processor=authority.processor,
                model_id=authority.model_id,
                dimensions=1024,
            )
        )
    if context.stage == "parse":
        profile = context.generation_embedding_profile
        if profile is None:
            return _reject(RETRIEVAL_EMBEDDING_AUTHORITY_INVALID)
        return _validated_profile(profile)
    return _reject(RETRIEVAL_EMBEDDING_AUTHORITY_INVALID)


def _validated_profile(
    profile: GenerationEmbeddingProfile,
) -> GenerationEmbeddingProfile:
    try:
        profile.validate()
    except ProviderProfileError as error:
        try:
            _reject(RETRIEVAL_EMBEDDING_AUTHORITY_INVALID)
        except PermanentJobError as routed:
            raise routed from error
    return profile


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))
