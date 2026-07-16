from __future__ import annotations

import hashlib
import uuid
from collections.abc import Mapping
from typing import Any

import pytest

from mm_chat_rag.job_context import ProcessingJobContext, admit_processing_job_context
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    JOB_HANDLER_SOURCE_INVALID,
)
from mm_chat_rag.models import JobClaim
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.source_gateway import (
    FileSourceMetadata,
    ObjectStoreDocumentSourceGateway,
)

BODY = b"%PDF-1.7 mm-chat source fixture"
SHA256 = hashlib.sha256(BODY).hexdigest()
HASH = "c" * 64


def _profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def _row(**updates: object) -> dict[str, object]:
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


def _context(row: Mapping[str, Any] | None = None) -> ProcessingJobContext:
    claim = JobClaim.from_row(row or _row())
    return admit_processing_job_context(claim, provider_profile=_profile())


def _metadata(
    context: ProcessingJobContext,
    *,
    body: bytes = BODY,
    file_id: uuid.UUID | None = None,
    object_key: str = "users/user-1/files/file-1",
    storage_backend: str = "minio",
    content_type: str = "application/pdf",
) -> FileSourceMetadata:
    return FileSourceMetadata(
        file_id=file_id or context.file_id,
        storage_backend=storage_backend,
        object_key=object_key,
        sha256=hashlib.sha256(body).hexdigest(),
        byte_size=len(body),
        content_type=content_type,
    )


class FakeMetadataGateway:
    def __init__(self, metadata: FileSourceMetadata | object) -> None:
        self._metadata = metadata
        self.calls: list[str] = []

    async def fetch_source_metadata(self, context: ProcessingJobContext) -> object:
        self.calls.append("metadata")
        assert context is not None
        return self._metadata


class FakeObjectGateway:
    def __init__(self, body: bytes | object) -> None:
        self._body = body
        self.calls: list[str] = []
        self.keys: list[str] = []

    async def fetch_object_bytes(self, metadata: FileSourceMetadata) -> object:
        self.calls.append("object")
        self.keys.append(metadata.object_key)
        return self._body


async def test_object_store_document_source_gateway_returns_verified_source() -> None:
    context = _context()
    metadata_gateway = FakeMetadataGateway(_metadata(context))
    object_gateway = FakeObjectGateway(BODY)
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=metadata_gateway,
        objects=object_gateway,
    )

    source = await gateway.fetch_document_source(context)

    assert source.body == BODY
    assert source.source_sha256 == SHA256
    assert source.content_type == "application/pdf"
    assert metadata_gateway.calls == ["metadata"]
    assert object_gateway.calls == ["object"]
    assert object_gateway.keys == ["users/user-1/files/file-1"]


async def test_object_store_document_source_gateway_default_off_before_calls() -> None:
    context = _context()
    gateway = ObjectStoreDocumentSourceGateway(metadata=None, objects=None)

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_document_source(context)

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


@pytest.mark.parametrize(
    "object_key",
    [
        "",
        " /trimmed",
        "/absolute",
        "trailing/",
        "a//b",
        "a/../b",
        "a/./b",
        "a\\b",
        "a:b",
    ],
)
def test_file_source_metadata_rejects_unsafe_object_keys(object_key: str) -> None:
    context = _context()

    with pytest.raises(PermanentJobError) as raised:
        _metadata(context, object_key=object_key)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID




@pytest.mark.parametrize(
    "updates",
    [
        {"file_id": uuid.UUID(int=0)},
        {"storage_backend": "ftp"},
        {"sha256": "not-sha"},
        {"byte_size": 0},
        {"content_type": "Application/PDF"},
    ],
)
def test_file_source_metadata_rejects_invalid_fields(
    updates: dict[str, object],
) -> None:
    context = _context()
    values: dict[str, object] = {
        "file_id": context.file_id,
        "storage_backend": "minio",
        "object_key": "users/user-1/files/file-1",
        "sha256": SHA256,
        "byte_size": len(BODY),
        "content_type": "application/pdf",
    }
    values.update(updates)

    with pytest.raises(PermanentJobError) as raised:
        FileSourceMetadata(**values)  # type: ignore[arg-type]

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_object_source_rejects_file_id_mismatch_before_object() -> None:
    context = _context()
    metadata_gateway = FakeMetadataGateway(_metadata(context, file_id=uuid.uuid4()))
    object_gateway = FakeObjectGateway(BODY)
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=metadata_gateway,
        objects=object_gateway,
    )

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_document_source(context)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID
    assert metadata_gateway.calls == ["metadata"]
    assert object_gateway.calls == []


async def test_object_store_document_source_gateway_rejects_size_mismatch() -> None:
    context = _context()
    metadata_gateway = FakeMetadataGateway(_metadata(context))
    object_gateway = FakeObjectGateway(BODY + b"x")
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=metadata_gateway,
        objects=object_gateway,
    )

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_document_source(context)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_object_store_document_source_gateway_rejects_hash_mismatch() -> None:
    context = _context()
    metadata_gateway = FakeMetadataGateway(_metadata(context, body=b"x" * len(BODY)))
    object_gateway = FakeObjectGateway(BODY)
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=metadata_gateway,
        objects=object_gateway,
    )

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_document_source(context)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_HASH_MISMATCH
    assert "users/user-1/files/file-1" not in str(raised.value)
