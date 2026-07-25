from __future__ import annotations

import uuid
from collections.abc import Mapping
from dataclasses import replace
from typing import Any, cast

import pytest

from mm_chat_rag.handlers import JOB_HANDLER_REGISTRY, with_job_context_admission
from mm_chat_rag.job_context import (
    JOB_CONTEXT_LEGACY_UNBOUND,
    ProcessingJobContext,
    ProviderAuthority,
    admit_processing_job_context,
)
from mm_chat_rag.job_handlers import (
    JOB_HANDLER_CONTEXT_INVALID,
    JOB_HANDLER_ERROR_CODES,
    JOB_HANDLER_GENERATION_REQUIRED,
    JOB_HANDLER_MATERIALIZATION_REQUIRED,
    JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN,
    JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED,
    JOB_HANDLER_PROVIDER_PROFILE_REQUIRED,
    JOB_HANDLER_SKELETON_UNPROMOTED,
    JOB_HANDLER_STAGE_MISMATCH,
    admitted_parse_handler_skeleton,
    admitted_passage_embedding_handler_skeleton,
    admitted_purge_handler_skeleton,
    parse_handler_skeleton,
    passage_embedding_handler_skeleton,
    purge_handler_skeleton,
)
from mm_chat_rag.models import JobClaim, stable_error_code
from mm_chat_rag.provider_profile import (
    MINERU_SILICONFLOW_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError

HASH = "b" * 64


def valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def provider_row(**updates: object) -> dict[str, object]:
    row: dict[str, object] = {
        "id": uuid.uuid4(),
        "stage": "parse",
        "operation": "initial",
        "collection_id": uuid.uuid4(),
        "document_id": uuid.uuid4(),
        "document_version_id": uuid.uuid4(),
        "file_id": uuid.uuid4(),
        "index_generation_id": uuid.uuid4(),
        "materialization_id": uuid.uuid4(),
        "processor": "mineru",
        "endpoint_id": "hosted",
        "model_id": "mineru",
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
        "request_hash": HASH,
        "legacy_projection_unbound": False,
    }
    row.update(updates)
    return row


def purge_row(**updates: object) -> dict[str, object]:
    row = provider_row(
        stage="purge",
        operation="purge",
        materialization_id=None,
        processor=None,
        endpoint_id=None,
        model_id=None,
        governance_profile_id=None,
        governance_revision=None,
        governance_head_revision=None,
        collection_consent_id=None,
        collection_consent_revision=None,
    )
    row.update(updates)
    return row


def claim(row: Mapping[str, Any]) -> JobClaim:
    return JobClaim.from_row(row)


def provider_context(**updates: object) -> ProcessingJobContext:
    return admit_processing_job_context(
        claim(provider_row(**updates)),
        provider_profile=valid_profile(),
    )


def embedding_context() -> ProcessingJobContext:
    return provider_context(
        stage="passage_embedding",
        processor="siliconflow",
        model_id="Pro/BAAI/bge-m3",
    )


def purge_context(**updates: object) -> ProcessingJobContext:
    return admit_processing_job_context(claim(purge_row(**updates)))


def test_job_handler_error_codes_are_stable() -> None:
    for code in JOB_HANDLER_ERROR_CODES:
        assert stable_error_code(code) == code


async def test_parse_skeleton_is_admitted_from_claim_but_not_promoted() -> None:
    handler = admitted_parse_handler_skeleton(valid_profile())

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row()))

    assert raised.value.error_code == JOB_HANDLER_SKELETON_UNPROMOTED
    assert JOB_HANDLER_REGISTRY == {}


async def test_parse_skeleton_needs_wrapper_profile_before_real_work() -> None:
    handler = with_job_context_admission(parse_handler_skeleton)

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row()))

    assert raised.value.error_code == JOB_HANDLER_PROVIDER_PROFILE_REQUIRED


async def test_wrapper_rejects_unsafe_claim_before_skeleton_validation() -> None:
    handler = with_job_context_admission(
        parse_handler_skeleton,
        provider_profile=valid_profile(),
    )

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row(legacy_projection_unbound=True)))

    assert raised.value.error_code == JOB_CONTEXT_LEGACY_UNBOUND


async def test_parse_skeleton_rejects_non_context_and_wrong_stage() -> None:
    wrong_type = cast("ProcessingJobContext", claim(provider_row()))

    with pytest.raises(PermanentJobError) as raised:
        await parse_handler_skeleton(wrong_type)
    assert raised.value.error_code == JOB_HANDLER_CONTEXT_INVALID

    with pytest.raises(PermanentJobError) as raised:
        await parse_handler_skeleton(purge_context())
    assert raised.value.error_code == JOB_HANDLER_STAGE_MISMATCH


@pytest.mark.parametrize(
    ("context", "error_code"),
    [
        (
            replace(provider_context(), authority=None),
            JOB_HANDLER_PROVIDER_AUTHORITY_REQUIRED,
        ),
        (
            replace(provider_context(), materialization_id=None),
            JOB_HANDLER_MATERIALIZATION_REQUIRED,
        ),
        (
            replace(provider_context(), runtime_provider_profile_id=None),
            JOB_HANDLER_PROVIDER_PROFILE_REQUIRED,
        ),
        (
            replace(provider_context(), index_generation_id=uuid.UUID(int=0)),
            JOB_HANDLER_GENERATION_REQUIRED,
        ),
    ],
)
async def test_provider_skeleton_rechecks_authority(
    context: ProcessingJobContext, error_code: str
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        await parse_handler_skeleton(context)
    assert raised.value.error_code == error_code


async def test_passage_embedding_skeleton_uses_same_provider_fence() -> None:
    context = embedding_context()
    handler = admitted_passage_embedding_handler_skeleton(valid_profile())

    with pytest.raises(PermanentJobError) as raised:
        await handler(
            claim(
                provider_row(
                    stage="passage_embedding",
                    processor="siliconflow",
                    model_id="Pro/BAAI/bge-m3",
                )
            )
        )
    assert raised.value.error_code == JOB_HANDLER_SKELETON_UNPROMOTED

    with pytest.raises(PermanentJobError) as raised:
        await passage_embedding_handler_skeleton(context)
    assert raised.value.error_code == JOB_HANDLER_SKELETON_UNPROMOTED

    with pytest.raises(PermanentJobError) as raised:
        await passage_embedding_handler_skeleton(
            replace(context, materialization_id=None)
        )
    assert raised.value.error_code == JOB_HANDLER_MATERIALIZATION_REQUIRED


async def test_purge_skeleton_forbids_authority_but_allows_null_materialization() -> (
    None
):
    context = purge_context()
    handler = admitted_purge_handler_skeleton()

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(purge_row()))
    assert raised.value.error_code == JOB_HANDLER_SKELETON_UNPROMOTED

    with pytest.raises(PermanentJobError) as raised:
        await purge_handler_skeleton(context)
    assert raised.value.error_code == JOB_HANDLER_SKELETON_UNPROMOTED

    forbidden = replace(
        context,
        authority=ProviderAuthority(
            processor="siliconflow",
            endpoint_id="hosted",
            model_id="Pro/BAAI/bge-m3",
            governance_profile_id=uuid.uuid4(),
            governance_revision=1,
            governance_head_revision=1,
            collection_consent_id=uuid.uuid4(),
            collection_consent_revision=1,
        ),
    )
    with pytest.raises(PermanentJobError) as raised:
        await purge_handler_skeleton(forbidden)
    assert raised.value.error_code == JOB_HANDLER_PROVIDER_AUTHORITY_FORBIDDEN

    with pytest.raises(PermanentJobError) as raised:
        await purge_handler_skeleton(
            replace(context, index_generation_id=uuid.UUID(int=0))
        )
    assert raised.value.error_code == JOB_HANDLER_GENERATION_REQUIRED
