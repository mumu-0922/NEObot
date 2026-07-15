from __future__ import annotations

import uuid
from collections.abc import Mapping
from typing import Any

import pytest

from mm_chat_rag.handlers import JobResult, with_job_context_admission
from mm_chat_rag.job_context import (
    JOB_CONTEXT_ERROR_CODES,
    JOB_CONTEXT_INVALID,
    JOB_CONTEXT_LEGACY_UNBOUND,
    JOB_CONTEXT_OPERATION_UNSUPPORTED,
    JOB_CONTEXT_PROJECTION_BINDING_MISSING,
    JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING,
    JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH,
    JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN,
    JOB_CONTEXT_STAGE_UNSUPPORTED,
    ProcessingJobContext,
    admit_processing_job_context,
)
from mm_chat_rag.models import JobClaim, stable_error_code
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError

HASH = "a" * 64


def valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def provider_row(**updates: object) -> dict[str, object]:
    job_id = uuid.uuid4()
    row: dict[str, object] = {
        "id": job_id,
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
        max_attempts=8,
    )
    row.update(updates)
    return row


def claim(row: Mapping[str, Any]) -> JobClaim:
    return JobClaim.from_row(row)


def assert_permanent(row: Mapping[str, Any], error_code: str) -> None:
    with pytest.raises(PermanentJobError) as raised:
        admit_processing_job_context(claim(row), provider_profile=valid_profile())
    assert raised.value.error_code == error_code
    assert str(raised.value) == error_code


def test_job_context_error_codes_are_stable() -> None:
    for code in JOB_CONTEXT_ERROR_CODES:
        assert stable_error_code(code) == code


def test_provider_job_claim_is_admitted_to_typed_context() -> None:
    row = provider_row()
    context = admit_processing_job_context(claim(row), provider_profile=valid_profile())

    assert context.job_id == row["id"]
    assert context.stage == "parse"
    assert context.operation == "initial"
    assert context.collection_id == row["collection_id"]
    assert context.index_generation_id == row["index_generation_id"]
    assert context.materialization_id == row["materialization_id"]
    assert context.attempt_count == 1
    assert context.max_attempts == 3
    assert context.request_hash == HASH
    assert context.runtime_provider_profile_id == MINERU_JINA_POSTGRES_PROFILE
    assert context.authority is not None
    assert context.authority.processor == "mineru"
    assert context.provider_backed is True


def test_purge_job_claim_is_admitted_without_provider_authority() -> None:
    row = purge_row()
    context = admit_processing_job_context(claim(row))

    assert context.stage == "purge"
    assert context.operation == "purge"
    assert context.index_generation_id == row["index_generation_id"]
    assert context.materialization_id is None
    assert context.authority is None
    assert context.provider_backed is False


@pytest.mark.parametrize(
    ("updates", "error_code"),
    [
        ({"stage": "unknown"}, JOB_CONTEXT_STAGE_UNSUPPORTED),
        ({"operation": "side_load"}, JOB_CONTEXT_OPERATION_UNSUPPORTED),
        ({"operation": "purge"}, JOB_CONTEXT_OPERATION_UNSUPPORTED),
        ({"legacy_projection_unbound": True}, JOB_CONTEXT_LEGACY_UNBOUND),
        ({"index_generation_id": None}, JOB_CONTEXT_PROJECTION_BINDING_MISSING),
        ({"materialization_id": None}, JOB_CONTEXT_PROJECTION_BINDING_MISSING),
        ({"processor": None}, JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING),
        ({"governance_revision": 0}, JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING),
        ({"request_hash": "raw-provider-body"}, JOB_CONTEXT_INVALID),
    ],
)
def test_provider_context_rejects_unsafe_claims(
    updates: dict[str, object], error_code: str
) -> None:
    assert_permanent(provider_row(**updates), error_code)


def test_provider_context_rejects_claim_counter_mismatch() -> None:
    row = provider_row(attempt_count=2)
    job_id = row["id"]
    assert isinstance(job_id, uuid.UUID)
    mismatched_claim = JobClaim(
        job_id=job_id,
        stage="parse",
        attempt_count=1,
        max_attempts=3,
        values=row,
    )

    with pytest.raises(PermanentJobError) as raised:
        admit_processing_job_context(
            mismatched_claim,
            provider_profile=valid_profile(),
        )
    assert raised.value.error_code == JOB_CONTEXT_INVALID


def test_purge_context_rejects_projection_gap_and_provider_authority() -> None:
    assert_permanent(
        purge_row(index_generation_id=None),
        JOB_CONTEXT_PROJECTION_BINDING_MISSING,
    )
    assert_permanent(
        purge_row(processor="jina"),
        JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN,
    )


def test_provider_context_rejects_disabled_runtime_profile() -> None:
    with pytest.raises(PermanentJobError) as raised:
        admit_processing_job_context(
            claim(provider_row()), provider_profile=ProviderRuntimeProfile()
        )
    assert raised.value.error_code == JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH


async def test_contextual_handler_wrapper_admits_context_before_execution() -> None:
    seen: list[ProcessingJobContext] = []

    async def contextual_handler(context: ProcessingJobContext) -> JobResult:
        seen.append(context)
        return JobResult()

    handler = with_job_context_admission(
        contextual_handler,
        provider_profile=valid_profile(),
    )
    result = await handler(claim(provider_row()))

    assert result == JobResult()
    assert seen[0].authority is not None


async def test_contextual_handler_wrapper_fails_closed_before_execution() -> None:
    called = False

    async def contextual_handler(_: ProcessingJobContext) -> JobResult:
        nonlocal called
        called = True
        return JobResult()

    handler = with_job_context_admission(
        contextual_handler,
        provider_profile=valid_profile(),
    )
    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row(legacy_projection_unbound=True)))

    assert raised.value.error_code == JOB_CONTEXT_LEGACY_UNBOUND
    assert called is False
