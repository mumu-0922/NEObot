"""Fail-closed admission context for leased RAG processing jobs.

The database claim row is an authority snapshot, but handlers must not consume
raw ``Mapping[str, Any]`` values directly. This module converts a leased
``JobClaim`` into a small typed context and turns every malformed or unsafe row
into a stable, redacted ``PermanentJobError``.
"""

from __future__ import annotations

import re
import uuid
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any, Final, NoReturn

from mm_chat_rag.models import JobClaim, stable_error_code
from mm_chat_rag.provider_profile import (
    PROVIDER_JOB_STAGES,
    GenerationEmbeddingProfile,
    ProviderProfileError,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError

JOB_CONTEXT_INVALID: Final = "JOB_CONTEXT_INVALID"
JOB_CONTEXT_STAGE_UNSUPPORTED: Final = "JOB_CONTEXT_STAGE_UNSUPPORTED"
JOB_CONTEXT_OPERATION_UNSUPPORTED: Final = "JOB_CONTEXT_OPERATION_UNSUPPORTED"
JOB_CONTEXT_LEGACY_UNBOUND: Final = "JOB_CONTEXT_LEGACY_UNBOUND"
JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING: Final = "JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING"
JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN: Final = "JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN"
JOB_CONTEXT_PROJECTION_BINDING_MISSING: Final = "JOB_CONTEXT_PROJECTION_BINDING_MISSING"
JOB_CONTEXT_LEASE_FENCE_MISSING: Final = "JOB_CONTEXT_LEASE_FENCE_MISSING"
JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH: Final = "JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH"
JOB_CONTEXT_ERROR_CODES: Final[frozenset[str]] = frozenset(
    {
        JOB_CONTEXT_INVALID,
        JOB_CONTEXT_STAGE_UNSUPPORTED,
        JOB_CONTEXT_OPERATION_UNSUPPORTED,
        JOB_CONTEXT_LEGACY_UNBOUND,
        JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING,
        JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN,
        JOB_CONTEXT_PROJECTION_BINDING_MISSING,
        JOB_CONTEXT_LEASE_FENCE_MISSING,
        JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH,
    }
)

_HASH_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_ZERO_UUID: Final = uuid.UUID(int=0)
_MAX_JOB_ATTEMPTS: Final = 32
_STAGE_OPERATIONS: Final[Mapping[str, frozenset[str]]] = {
    "parse": frozenset({"initial", "replace", "reprocess"}),
    "passage_embedding": frozenset({"initial", "replace", "reprocess"}),
    "purge": frozenset({"purge"}),
}
_ALL_OPERATIONS: Final[frozenset[str]] = frozenset(
    operation for operations in _STAGE_OPERATIONS.values() for operation in operations
)
_PROVIDER_AUTHORITY_FIELDS: Final[tuple[str, ...]] = (
    "processor",
    "endpoint_id",
    "model_id",
    "governance_profile_id",
    "governance_revision",
    "governance_head_revision",
    "collection_consent_id",
    "collection_consent_revision",
)


@dataclass(frozen=True, slots=True)
class ProviderAuthority:
    """Exact provider/governance/consent authority pinned by the Go backend."""

    processor: str
    endpoint_id: str
    model_id: str
    governance_profile_id: uuid.UUID
    governance_revision: int
    governance_head_revision: int
    collection_consent_id: uuid.UUID
    collection_consent_revision: int


@dataclass(frozen=True, slots=True)
class ProcessingJobContext:
    """Typed, redacted context passed to promoted worker handlers."""

    job_id: uuid.UUID
    stage: str
    operation: str
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    file_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID | None
    collection_acl_revision: int
    collection_visibility_epoch: int
    collection_processing_revision: int
    document_visibility_epoch: int
    attempt_count: int
    max_attempts: int
    request_hash: str
    authority: ProviderAuthority | None
    lease_token: uuid.UUID | None = None
    runtime_provider_profile_id: str | None = None
    generation_embedding_profile: GenerationEmbeddingProfile | None = None
    generation_chunk_profile_hash: str | None = None

    @property
    def provider_backed(self) -> bool:
        """Return true when the job must use provider/governance authority."""
        return self.authority is not None


def admit_processing_job_context(
    job: JobClaim,
    *,
    provider_profile: ProviderRuntimeProfile | None = None,
) -> ProcessingJobContext:
    """Validate and type a claim row before any handler performs side effects."""
    row = job.values
    job_id = _uuid_alias(row, ("job_id", "id"))
    if job_id != job.job_id:
        _reject(JOB_CONTEXT_INVALID)

    stage = _text(row, "stage")
    if stage != job.stage:
        _reject(JOB_CONTEXT_INVALID)
    if stage not in _STAGE_OPERATIONS:
        _reject(JOB_CONTEXT_STAGE_UNSUPPORTED)

    operation = _text(row, "operation")
    if operation not in _ALL_OPERATIONS or operation not in _STAGE_OPERATIONS[stage]:
        _reject(JOB_CONTEXT_OPERATION_UNSUPPORTED)

    if _bool(row, "legacy_projection_unbound", default=False):
        _reject(JOB_CONTEXT_LEGACY_UNBOUND)

    _validate_claim_counter(row, "attempt_count", job.attempt_count)
    _validate_claim_counter(row, "max_attempts", job.max_attempts)
    if job.attempt_count < 1 or not 1 <= job.max_attempts <= _MAX_JOB_ATTEMPTS:
        _reject(JOB_CONTEXT_INVALID)
    if job.attempt_count > job.max_attempts:
        _reject(JOB_CONTEXT_INVALID)

    index_generation_id = _optional_uuid(row, "index_generation_id")
    materialization_id = _optional_uuid(row, "materialization_id")
    if index_generation_id is None:
        _reject(JOB_CONTEXT_PROJECTION_BINDING_MISSING)
    if stage in PROVIDER_JOB_STAGES and materialization_id is None:
        _reject(JOB_CONTEXT_PROJECTION_BINDING_MISSING)

    authority = _provider_authority(row, stage)
    profile_id = _validate_provider_profile(
        job,
        stage,
        authority,
        provider_profile,
    )

    return ProcessingJobContext(
        job_id=job_id,
        stage=stage,
        operation=operation,
        collection_id=_uuid(row, "collection_id"),
        document_id=_uuid(row, "document_id"),
        document_version_id=_uuid(row, "document_version_id"),
        file_id=_uuid(row, "file_id"),
        index_generation_id=index_generation_id,
        materialization_id=materialization_id,
        collection_acl_revision=_positive_int(row, "collection_acl_revision"),
        collection_visibility_epoch=_positive_int(row, "collection_visibility_epoch"),
        collection_processing_revision=_positive_int(
            row, "collection_processing_revision"
        ),
        document_visibility_epoch=_positive_int(row, "document_visibility_epoch"),
        attempt_count=job.attempt_count,
        max_attempts=job.max_attempts,
        request_hash=_hash(row, "request_hash"),
        authority=authority,
        lease_token=_optional_uuid(row, "lease_token"),
        runtime_provider_profile_id=profile_id,
    )


def _provider_authority(row: Mapping[str, Any], stage: str) -> ProviderAuthority | None:
    if stage not in PROVIDER_JOB_STAGES:
        if any(row.get(key) is not None for key in _PROVIDER_AUTHORITY_FIELDS):
            _reject(JOB_CONTEXT_PURGE_AUTHORITY_FORBIDDEN)
        return None

    try:
        return ProviderAuthority(
            processor=_text(row, "processor"),
            endpoint_id=_text(row, "endpoint_id"),
            model_id=_text(row, "model_id"),
            governance_profile_id=_uuid(row, "governance_profile_id"),
            governance_revision=_positive_int(row, "governance_revision"),
            governance_head_revision=_positive_int(row, "governance_head_revision"),
            collection_consent_id=_uuid(row, "collection_consent_id"),
            collection_consent_revision=_positive_int(
                row, "collection_consent_revision"
            ),
        )
    except PermanentJobError as error:
        if error.error_code == JOB_CONTEXT_INVALID:
            _reject(JOB_CONTEXT_PROVIDER_AUTHORITY_MISSING)
        raise


def _validate_provider_profile(
    job: JobClaim,
    stage: str,
    authority: ProviderAuthority | None,
    provider_profile: ProviderRuntimeProfile | None,
) -> str | None:
    if stage not in PROVIDER_JOB_STAGES or provider_profile is None:
        return None
    try:
        provider_profile.validate_static_contract()
    except ProviderProfileError as error:
        _reject_from(JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH, error)
    if not provider_profile.enabled:
        _reject(JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH)
    if job.max_attempts != provider_profile.retry_max_attempts:
        _reject(JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH)
    if stage == "passage_embedding" and (
        authority is None
        or not provider_profile.admits_embedding_profile(
            GenerationEmbeddingProfile(
                processor=authority.processor,
                model_id=authority.model_id,
                dimensions=1024,
            )
        )
    ):
        _reject(JOB_CONTEXT_PROVIDER_PROFILE_MISMATCH)
    return provider_profile.profile_id


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_from(error_code: str, cause: Exception) -> NoReturn:
    try:
        _reject(error_code)
    except PermanentJobError as error:
        raise error from cause


def _uuid_alias(row: Mapping[str, Any], keys: tuple[str, ...]) -> uuid.UUID:
    values = [
        _uuid_value(row[key]) for key in keys if key in row and row[key] is not None
    ]
    if not values:
        _reject(JOB_CONTEXT_INVALID)
    first = values[0]
    if any(value != first for value in values):
        _reject(JOB_CONTEXT_INVALID)
    return first


def _uuid(row: Mapping[str, Any], key: str) -> uuid.UUID:
    if key not in row or row[key] is None:
        _reject(JOB_CONTEXT_INVALID)
    return _uuid_value(row[key])


def _optional_uuid(row: Mapping[str, Any], key: str) -> uuid.UUID | None:
    if key not in row or row[key] is None:
        return None
    return _uuid_value(row[key])


def _uuid_value(value: object) -> uuid.UUID:
    if isinstance(value, uuid.UUID):
        parsed = value
    else:
        try:
            parsed = uuid.UUID(str(value))
        except (TypeError, ValueError) as error:
            _reject_from(JOB_CONTEXT_INVALID, error)
    if parsed == _ZERO_UUID:
        _reject(JOB_CONTEXT_INVALID)
    return parsed


def _text(row: Mapping[str, Any], key: str) -> str:
    value = row.get(key)
    if (
        not isinstance(value, str)
        or not value
        or value.strip() != value
        or "\x00" in value
    ):
        _reject(JOB_CONTEXT_INVALID)
    return value


def _positive_int(row: Mapping[str, Any], key: str) -> int:
    value = row.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        _reject(JOB_CONTEXT_INVALID)
    return value


def _bool(row: Mapping[str, Any], key: str, *, default: bool) -> bool:
    value = row.get(key, default)
    if not isinstance(value, bool):
        _reject(JOB_CONTEXT_INVALID)
    return value


def _validate_claim_counter(row: Mapping[str, Any], key: str, expected: int) -> None:
    if key not in row or row[key] is None:
        return
    if isinstance(row[key], bool) or not isinstance(row[key], int):
        _reject(JOB_CONTEXT_INVALID)
    if row[key] != expected:
        _reject(JOB_CONTEXT_INVALID)


def _hash(row: Mapping[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not _HASH_RE.fullmatch(value):
        _reject(JOB_CONTEXT_INVALID)
    return value
