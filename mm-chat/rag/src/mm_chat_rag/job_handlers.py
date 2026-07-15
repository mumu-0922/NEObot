"""G7.5 admitted job handler skeletons.

These handlers are intentionally side-effect free and are not registered for
production dispatch. They prove the promoted handler boundary: every future
parse, embedding, or purge implementation must receive a typed
``ProcessingJobContext`` only, must re-check stage-specific authority, and must
fail with stable redacted error codes until a real implementation replaces the
skeleton.
"""

from __future__ import annotations

import uuid
from typing import Final, NoReturn

from mm_chat_rag.handlers import JobHandler, JobResult, with_job_context_admission
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError

JOB_HANDLER_CONTEXT_INVALID: Final = "JOB_HANDLER_CONTEXT_INVALID"
JOB_HANDLER_STAGE_MISMATCH: Final = "JOB_HANDLER_STAGE_MISMATCH"
JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED: Final = (
    "JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED"
)
JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN: Final = (
    "JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN"
)
JOB_HANDLER_PROVIDER_PROFILE_REQUIRED: Final = (
    "JOB_HANDLER_PROVIDER_PROFILE_REQUIRED"
)
JOB_HANDLER_MATERIALIZATION_REQUIRED: Final = (
    "JOB_HANDLER_MATERIALIZATION_REQUIRED"
)
JOB_HANDLER_GENERATION_REQUIRED: Final = "JOB_HANDLER_GENERATION_REQUIRED"
JOB_HANDLER_SKELETON_UNPROMOTED: Final = "JOB_HANDLER_SKELETON_UNPROMOTED"
JOB_HANDLER_ERROR_CODES: Final[frozenset[str]] = frozenset(
    {
        JOB_HANDLER_CONTEXT_INVALID,
        JOB_HANDLER_STAGE_MISMATCH,
        JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED,
        JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN,
        JOB_HANDLER_PROVIDER_PROFILE_REQUIRED,
        JOB_HANDLER_MATERIALIZATION_REQUIRED,
        JOB_HANDLER_GENERATION_REQUIRED,
        JOB_HANDLER_SKELETON_UNPROMOTED,
    }
)

_ZERO_UUID: Final = uuid.UUID(int=0)


async def parse_handler_skeleton(context: ProcessingJobContext) -> JobResult:
    """Validate parse authority and stop before provider or storage work."""
    _require_provider_job_context(context, stage="parse")
    _reject(JOB_HANDLER_SKELETON_UNPROMOTED)


async def passage_embedding_handler_skeleton(
    context: ProcessingJobContext,
) -> JobResult:
    """Validate embedding authority and stop before Jina/Postgres writes."""
    _require_provider_job_context(context, stage="passage_embedding")
    _reject(JOB_HANDLER_SKELETON_UNPROMOTED)


async def purge_handler_skeleton(context: ProcessingJobContext) -> JobResult:
    """Validate purge authority and stop before projection mutation."""
    admitted = _require_context(context)
    _require_stage(admitted, "purge")
    _require_generation(admitted)
    if admitted.authority is not None:
        _reject(JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN)
    _reject(JOB_HANDLER_SKELETON_UNPROMOTED)


def admitted_parse_handler_skeleton(
    provider_profile: ProviderRuntimeProfile,
) -> JobHandler:
    """Build a claim-level parse skeleton through the admission wrapper."""
    return with_job_context_admission(
        parse_handler_skeleton,
        provider_profile=provider_profile,
    )


def admitted_passage_embedding_handler_skeleton(
    provider_profile: ProviderRuntimeProfile,
) -> JobHandler:
    """Build a claim-level embedding skeleton through the admission wrapper."""
    return with_job_context_admission(
        passage_embedding_handler_skeleton,
        provider_profile=provider_profile,
    )


def admitted_purge_handler_skeleton() -> JobHandler:
    """Build a claim-level purge skeleton through the admission wrapper."""
    return with_job_context_admission(purge_handler_skeleton)


def _require_provider_job_context(
    context: ProcessingJobContext, *, stage: str
) -> ProcessingJobContext:
    admitted = _require_context(context)
    _require_stage(admitted, stage)
    _require_generation(admitted)
    if admitted.materialization_id is None or admitted.materialization_id == _ZERO_UUID:
        _reject(JOB_HANDLER_MATERIALIZATION_REQUIRED)
    if admitted.authority is None:
        _reject(JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED)
    if admitted.runtime_provider_profile_id != MINERU_JINA_POSTGRES_PROFILE:
        _reject(JOB_HANDLER_PROVIDER_PROFILE_REQUIRED)
    return admitted


def _require_context(context: ProcessingJobContext) -> ProcessingJobContext:
    if not isinstance(context, ProcessingJobContext):
        _reject(JOB_HANDLER_CONTEXT_INVALID)
    return context


def _require_stage(context: ProcessingJobContext, stage: str) -> None:
    if context.stage != stage:
        _reject(JOB_HANDLER_STAGE_MISMATCH)


def _require_generation(context: ProcessingJobContext) -> None:
    if context.index_generation_id == _ZERO_UUID:
        _reject(JOB_HANDLER_GENERATION_REQUIRED)


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))
