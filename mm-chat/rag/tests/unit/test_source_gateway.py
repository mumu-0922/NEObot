from __future__ import annotations

import hashlib
import json
import uuid
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import httpx
import pytest

from mm_chat_rag.job_context import ProcessingJobContext, admit_processing_job_context
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    JOB_HANDLER_SOURCE_INVALID,
)
from mm_chat_rag.models import JobClaim
from mm_chat_rag.provider_profile import (
    MINERU_SILICONFLOW_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError
from mm_chat_rag.source_gateway import (
    GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED,
    GO_SOURCE_OBJECT_PATH,
    FileSourceMetadata,
    GoSourceObjectBytesGateway,
    LocalObjectBytesGateway,
    ObjectStoreDocumentSourceGateway,
)

BODY = b"%PDF-1.7 mm-chat source fixture"
SHA256 = hashlib.sha256(BODY).hexdigest()
HASH = "c" * 64
INTERNAL_TOKEN = "unit-test-source-gateway-token"


def _profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
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

    async def fetch_object_bytes(
        self, context: ProcessingJobContext, metadata: FileSourceMetadata
    ) -> object:
        self.calls.append("object")
        assert context is not None
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


async def test_local_object_bytes_gateway_reads_verified_local_object(
    tmp_path: Path,
) -> None:
    context = _context()
    object_path = tmp_path / "users" / "user-1" / "files" / "file-1"
    object_path.parent.mkdir(parents=True)
    object_path.write_bytes(BODY)
    metadata_gateway = FakeMetadataGateway(
        _metadata(
            context,
            storage_backend="local",
        )
    )
    object_gateway = LocalObjectBytesGateway(root=tmp_path)
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=metadata_gateway,
        objects=object_gateway,
    )

    source = await gateway.fetch_document_source(context)

    assert source.body == BODY
    assert source.source_sha256 == SHA256


def test_local_object_bytes_gateway_rejects_missing_root(tmp_path: Path) -> None:
    missing = tmp_path / "missing"

    with pytest.raises(PermanentJobError) as raised:
        LocalObjectBytesGateway(root=missing)

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_local_object_bytes_gateway_rejects_nonlocal_backend(
    tmp_path: Path,
) -> None:
    context = _context()
    gateway = LocalObjectBytesGateway(root=tmp_path)

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_object_bytes(
            context, _metadata(context, storage_backend="minio")
        )

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_local_object_bytes_gateway_rejects_size_mismatch(
    tmp_path: Path,
) -> None:
    context = _context()
    object_path = tmp_path / "users" / "user-1" / "files" / "file-1"
    object_path.parent.mkdir(parents=True)
    object_path.write_bytes(BODY + b"x")
    gateway = LocalObjectBytesGateway(root=tmp_path)

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_object_bytes(
            context, _metadata(context, storage_backend="local")
        )

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_local_object_bytes_gateway_rejects_symlink_escape(
    tmp_path: Path,
) -> None:
    context = _context()
    outside = tmp_path / "outside.pdf"
    outside.write_bytes(BODY)
    object_path = tmp_path / "users" / "user-1" / "files" / "file-1"
    object_path.parent.mkdir(parents=True)
    object_path.symlink_to(outside)
    gateway = LocalObjectBytesGateway(root=tmp_path)

    with pytest.raises(PermanentJobError) as raised:
        await gateway.fetch_object_bytes(
            context, _metadata(context, storage_backend="local")
        )

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_go_source_object_gateway_sends_leased_fence_and_returns_bytes() -> None:
    context = _context(_row(lease_token=uuid.uuid4()))
    metadata = _metadata(context)
    worker_id = uuid.uuid4()
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _source_response(metadata, BODY)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=worker_id,
            client=client,
        )
        body = await gateway.fetch_object_bytes(context, metadata)

    assert body == BODY
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(f"http://backend:8080{GO_SOURCE_OBJECT_PATH}")
    assert request.headers["x-mm-chat-internal-token"] == INTERNAL_TOKEN
    assert request.headers["accept"] == "application/octet-stream"
    assert INTERNAL_TOKEN.encode() not in request.content
    assert json.loads(request.content) == {
        "jobId": str(context.job_id),
        "workerId": str(worker_id),
        "leaseToken": str(context.lease_token),
        "fileId": str(context.file_id),
        "materializationId": str(context.materialization_id),
    }


@pytest.mark.parametrize(
    ("base_url", "token", "worker_id"),
    [
        ("", INTERNAL_TOKEN, uuid.uuid4()),
        ("ftp://backend:8080", INTERNAL_TOKEN, uuid.uuid4()),
        ("http://backend:8080?token=bad", INTERNAL_TOKEN, uuid.uuid4()),
        ("http://backend:8080", "", uuid.uuid4()),
        ("http://backend:8080", " spaced ", uuid.uuid4()),
        ("http://backend:8080", INTERNAL_TOKEN, uuid.UUID(int=0)),
    ],
)
def test_go_source_object_gateway_rejects_unsafe_config(
    base_url: str, token: str, worker_id: uuid.UUID
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        GoSourceObjectBytesGateway(
            base_url=base_url,
            internal_token=token,
            worker_id=worker_id,
        )

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_go_source_object_gateway_requires_lease_token() -> None:
    context = _context()
    metadata = _metadata(context)
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("missing lease reached Go source gateway")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=uuid.uuid4(),
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.fetch_object_bytes(context, metadata)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID
    assert calls == 0


async def test_go_source_object_gateway_maps_unauthorized_to_dependency_error() -> None:
    context = _context(_row(lease_token=uuid.uuid4()))
    metadata = _metadata(context)

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: httpx.Response(401, json={"error": {}}))
    ) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=uuid.uuid4(),
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.fetch_object_bytes(context, metadata)

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
    assert INTERNAL_TOKEN not in str(raised.value)


async def test_go_source_object_gateway_retries_redacted_transport_failure() -> None:
    context = _context(_row(lease_token=uuid.uuid4()))
    metadata = _metadata(context)

    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadError(f"sensitive detail {INTERNAL_TOKEN}", request=request)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=uuid.uuid4(),
            client=client,
        )
        with pytest.raises(RetryableJobError) as raised:
            await gateway.fetch_object_bytes(context, metadata)

    assert raised.value.error_code == GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED
    assert raised.value.retry_after_seconds == 30
    assert INTERNAL_TOKEN not in str(raised.value)


@pytest.mark.parametrize(
    "headers",
    [
        {"X-MM-Chat-File-ID": str(uuid.uuid4()), "X-MM-Chat-Source-SHA256": SHA256},
        {"X-MM-Chat-File-ID": "", "X-MM-Chat-Source-SHA256": SHA256},
    ],
)
async def test_go_source_object_gateway_rejects_file_header_mismatch(
    headers: dict[str, str],
) -> None:
    context = _context(_row(lease_token=uuid.uuid4()))
    metadata = _metadata(context)

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: httpx.Response(200, headers=headers))
    ) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=uuid.uuid4(),
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.fetch_object_bytes(context, metadata)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_go_source_object_gateway_rejects_body_hash_mismatch() -> None:
    context = _context(_row(lease_token=uuid.uuid4()))
    metadata = _metadata(context)
    tampered = b"x" * len(BODY)

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _source_response(metadata, tampered))
    ) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_TOKEN,
            worker_id=uuid.uuid4(),
            client=client,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.fetch_object_bytes(context, metadata)

    assert raised.value.error_code == JOB_HANDLER_SOURCE_HASH_MISMATCH
    assert metadata.object_key not in str(raised.value)


def _source_response(
    metadata: FileSourceMetadata,
    body: bytes,
    *,
    status: int = 200,
) -> httpx.Response:
    return httpx.Response(
        status,
        headers={
            "Content-Type": metadata.content_type,
            "Content-Length": str(metadata.byte_size),
            "X-MM-Chat-File-ID": str(metadata.file_id),
            "X-MM-Chat-Source-SHA256": metadata.sha256,
        },
        content=body,
    )
