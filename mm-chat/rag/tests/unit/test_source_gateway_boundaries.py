"""Source gateway fail-closed boundary coverage."""

from __future__ import annotations

import hashlib
import uuid
from dataclasses import replace
from pathlib import Path
from typing import cast

import httpx
import pytest

import mm_chat_rag.source_gateway as source_module
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    JOB_HANDLER_SOURCE_INVALID,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError
from mm_chat_rag.source_gateway import (
    GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED,
    FileSourceMetadata,
    GoSourceObjectBytesGateway,
    LocalObjectBytesGateway,
    ObjectStoreDocumentSourceGateway,
)

BODY = b"bounded source"
INTERNAL_HEADER_VALUE = "unit-test-source-boundary-value"


def _context(
    *,
    file_id: uuid.UUID | None = None,
    lease_token: uuid.UUID | None = None,
    materialization_id: uuid.UUID | None = None,
) -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="parse",
        operation="initial",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=file_id or uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=materialization_id,
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=None,
        lease_token=lease_token,
    )


def _metadata(
    file_id: uuid.UUID,
    *,
    body: bytes = BODY,
    storage_backend: str = "minio",
    object_key: str = "users/u/files/f",
) -> FileSourceMetadata:
    return FileSourceMetadata(
        file_id=file_id,
        storage_backend=storage_backend,
        object_key=object_key,
        sha256=hashlib.sha256(body).hexdigest(),
        byte_size=len(body),
        content_type="application/pdf",
    )


def _go_gateway(*, max_bytes: int = 1024) -> GoSourceObjectBytesGateway:
    return GoSourceObjectBytesGateway(
        base_url="http://backend:8080",
        internal_token=INTERNAL_HEADER_VALUE,
        worker_id=uuid.uuid4(),
        max_bytes=max_bytes,
    )


@pytest.mark.parametrize("maximum", [0, True, 512 * 1024 * 1024 + 1])
def test_go_source_gateway_rejects_invalid_maximum(maximum: int) -> None:
    with pytest.raises(PermanentJobError) as raised:
        _go_gateway(max_bytes=maximum)
    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_go_source_gateway_rejects_invalid_fence_inputs_before_http() -> None:
    context = _context(lease_token=uuid.uuid4(), materialization_id=uuid.uuid4())
    gateway = _go_gateway(max_bytes=1)
    checks = [
        (cast("ProcessingJobContext", object()), _metadata(context.file_id)),
        (context, cast("FileSourceMetadata", object())),
        (replace_context(context, materialization_id=None), _metadata(context.file_id)),
        (context, _metadata(uuid.uuid4())),
        (context, _metadata(context.file_id)),
    ]

    for invalid_context, metadata in checks:
        with pytest.raises(PermanentJobError) as raised:
            await gateway.fetch_object_bytes(invalid_context, metadata)
        assert raised.value.error_code == JOB_HANDLER_SOURCE_INVALID


def replace_context(
    context: ProcessingJobContext,
    *,
    materialization_id: uuid.UUID | None,
) -> ProcessingJobContext:
    return replace(context, materialization_id=materialization_id)


class _MetadataGateway:
    def __init__(self, value: object) -> None:
        self.value = value

    async def fetch_source_metadata(self, _: ProcessingJobContext) -> object:
        return self.value


class _ObjectGateway:
    async def fetch_object_bytes(
        self,
        _: ProcessingJobContext,
        metadata: FileSourceMetadata,
    ) -> bytes:
        return BODY[: metadata.byte_size]


async def test_object_store_rejects_invalid_context_and_metadata() -> None:
    context = _context()
    gateway = ObjectStoreDocumentSourceGateway(
        metadata=_MetadataGateway(_metadata(context.file_id)),
        objects=_ObjectGateway(),
    )
    with pytest.raises(PermanentJobError) as invalid_context:
        await gateway.fetch_document_source(cast("ProcessingJobContext", object()))
    assert invalid_context.value.error_code == JOB_HANDLER_SOURCE_INVALID

    gateway = ObjectStoreDocumentSourceGateway(
        metadata=_MetadataGateway(object()),
        objects=_ObjectGateway(),
    )
    with pytest.raises(PermanentJobError) as invalid_metadata:
        await gateway.fetch_document_source(context)
    assert invalid_metadata.value.error_code == JOB_HANDLER_SOURCE_INVALID


@pytest.mark.parametrize("maximum", [0, True, 512 * 1024 * 1024 + 1])
def test_local_source_gateway_rejects_invalid_maximum(
    tmp_path: Path,
    maximum: int,
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        LocalObjectBytesGateway(root=tmp_path, max_bytes=maximum)
    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_local_source_gateway_rejects_invalid_metadata_and_missing_object(
    tmp_path: Path,
) -> None:
    context = _context()
    gateway = LocalObjectBytesGateway(root=tmp_path, max_bytes=1)
    with pytest.raises(PermanentJobError) as invalid_metadata:
        await gateway.fetch_object_bytes(
            context,
            cast("FileSourceMetadata", object()),
        )
    assert invalid_metadata.value.error_code == JOB_HANDLER_SOURCE_INVALID
    with pytest.raises(PermanentJobError) as oversized:
        await gateway.fetch_object_bytes(
            context,
            _metadata(context.file_id, storage_backend="local"),
        )
    assert oversized.value.error_code == JOB_HANDLER_SOURCE_INVALID

    missing_gateway = LocalObjectBytesGateway(root=tmp_path)
    with pytest.raises(PermanentJobError) as missing:
        await missing_gateway.fetch_object_bytes(
            context,
            _metadata(context.file_id, storage_backend="local"),
        )
    assert missing.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_local_source_gateway_rejects_directory_and_read_drift(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    context = _context()
    object_path = tmp_path / "users" / "u" / "files" / "f"
    object_path.mkdir(parents=True)
    gateway = LocalObjectBytesGateway(root=tmp_path)
    metadata = _metadata(context.file_id, storage_backend="local")
    with pytest.raises(PermanentJobError) as directory:
        await gateway.fetch_object_bytes(context, metadata)
    assert directory.value.error_code == JOB_HANDLER_SOURCE_INVALID

    object_path.rmdir()
    object_path.write_bytes(BODY)
    monkeypatch.setattr(Path, "read_bytes", lambda _: BODY[:-1])
    with pytest.raises(PermanentJobError) as short_read:
        await gateway.fetch_object_bytes(context, metadata)
    assert short_read.value.error_code == JOB_HANDLER_SOURCE_INVALID


async def test_go_source_default_client_is_closed_and_returns_verified_bytes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    context = _context(lease_token=uuid.uuid4(), materialization_id=uuid.uuid4())
    metadata = _metadata(context.file_id)
    real_client = httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200,
                headers={
                    "X-MM-Chat-File-ID": str(metadata.file_id),
                    "X-MM-Chat-Source-SHA256": metadata.sha256,
                },
                stream=httpx.ByteStream(BODY),
            )
        )
    )

    def fake_client(**kwargs: object) -> httpx.AsyncClient:
        assert kwargs["follow_redirects"] is False
        assert kwargs["trust_env"] is False
        return real_client

    monkeypatch.setattr(source_module.httpx, "AsyncClient", fake_client)
    assert await _go_gateway().fetch_object_bytes(context, metadata) == BODY


@pytest.mark.parametrize(
    ("status", "error", "retryable"),
    [
        (400, JOB_HANDLER_SOURCE_INVALID, False),
        (500, GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED, True),
    ],
)
async def test_go_source_gateway_maps_closed_status_matrix(
    status: int,
    error: str,
    retryable: bool,
) -> None:
    context = _context(lease_token=uuid.uuid4(), materialization_id=uuid.uuid4())
    metadata = _metadata(context.file_id)
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: httpx.Response(status))
    ) as client:
        gateway = GoSourceObjectBytesGateway(
            base_url="http://backend:8080",
            internal_token=INTERNAL_HEADER_VALUE,
            worker_id=uuid.uuid4(),
            client=client,
        )
        expected = RetryableJobError if retryable else PermanentJobError
        with pytest.raises(expected) as raised:
            await gateway.fetch_object_bytes(context, metadata)
    assert raised.value.error_code == error


@pytest.mark.parametrize(
    ("headers", "error"),
    [
        (
            {
                "X-MM-Chat-File-ID": "placeholder",
                "X-MM-Chat-Source-SHA256": "placeholder",
            },
            JOB_HANDLER_SOURCE_INVALID,
        ),
        (
            {"X-MM-Chat-File-ID": "valid", "X-MM-Chat-Source-SHA256": "wrong"},
            JOB_HANDLER_SOURCE_HASH_MISMATCH,
        ),
        (
            {
                "X-MM-Chat-File-ID": "valid",
                "X-MM-Chat-Source-SHA256": "valid",
                "Content-Length": "invalid",
            },
            JOB_HANDLER_SOURCE_INVALID,
        ),
        (
            {
                "X-MM-Chat-File-ID": "valid",
                "X-MM-Chat-Source-SHA256": "valid",
                "Content-Length": "999",
            },
            JOB_HANDLER_SOURCE_INVALID,
        ),
    ],
)
def test_go_source_headers_reject_identity_and_length_drift(
    headers: dict[str, str],
    error: str,
) -> None:
    metadata = _metadata(uuid.uuid4())
    normalized = {
        key: (
            str(metadata.file_id)
            if value == "valid" and key == "X-MM-Chat-File-ID"
            else metadata.sha256
            if value == "valid"
            else value
        )
        for key, value in headers.items()
    }
    response = httpx.Response(200, headers=normalized)
    with pytest.raises(PermanentJobError) as raised:
        source_module._validate_go_source_headers(response, metadata)
    assert raised.value.error_code == error


async def test_go_source_response_reader_rejects_consumed_and_streamed_overflow() -> (
    None
):
    consumed = httpx.Response(200, content=BODY)
    with pytest.raises(PermanentJobError) as consumed_error:
        await source_module._read_bounded_go_source_response(consumed, 1)
    assert consumed_error.value.error_code == JOB_HANDLER_SOURCE_INVALID

    streamed = httpx.Response(200, stream=httpx.ByteStream(BODY))
    with pytest.raises(PermanentJobError) as streamed_error:
        await source_module._read_bounded_go_source_response(streamed, 1)
    assert streamed_error.value.error_code == JOB_HANDLER_SOURCE_INVALID


def test_source_gateway_rejects_root_url_path_and_token_edge_cases(
    tmp_path: Path,
) -> None:
    file_root = tmp_path / "file"
    file_root.write_text("not a directory", encoding="utf-8")
    for root in (" ", file_root):
        with pytest.raises(PermanentJobError) as raised:
            LocalObjectBytesGateway(root=root)
        assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED

    local_root = tmp_path / "root"
    local_root.mkdir()
    outside = tmp_path / "outside"
    outside.mkdir()
    link = local_root / "link"
    link.symlink_to(outside, target_is_directory=True)
    with pytest.raises(PermanentJobError) as escaped:
        source_module._resolve_local_object_path(local_root, "link/object")
    assert escaped.value.error_code == JOB_HANDLER_SOURCE_INVALID

    with pytest.raises(PermanentJobError) as invalid_url:
        source_module._source_gateway_url("http://[")
    assert invalid_url.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
    with pytest.raises(PermanentJobError) as invalid_token:
        source_module._validate_internal_token(cast("str", 1))
    assert invalid_token.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED
