from __future__ import annotations

import hashlib
import io
import json
import stat
import uuid
import zipfile
from typing import Any

import httpx
import pytest

import mm_chat_rag.mineru_gateway as mineru_module
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.mineru_gateway import (
    MINERU_ALLOCATE_UPLOAD_URL,
    MINERU_GATEWAY_ARCHIVE_INVALID,
    MINERU_GATEWAY_ARTIFACT_INVALID,
    MINERU_GATEWAY_CONTEXT_INVALID,
    MINERU_GATEWAY_CREDENTIALS_MISSING,
    MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED,
    MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID,
    MINERU_GATEWAY_REQUEST_FAILED,
    MINERU_GATEWAY_RESPONSE_INVALID,
    MINERU_GATEWAY_RESPONSE_TOO_LARGE,
    MINERU_GATEWAY_RESULT_FAILED,
    MINERU_GATEWAY_RESULT_NOT_READY,
    MINERU_GATEWAY_RESULT_PROXY_INVALID,
    MINERU_GATEWAY_RESULT_URL_INVALID,
    MINERU_GATEWAY_SOURCE_HASH_MISMATCH,
    MINERU_GATEWAY_SOURCE_UNSUPPORTED,
    MINERU_GATEWAY_STATUS_INVALID,
    MINERU_GATEWAY_UPLOAD_STATUS_INVALID,
    MINERU_GATEWAY_UPLOAD_URL_INVALID,
    MINERU_POLL_URL_PREFIX,
    MINERU_TEXT_BASELINE_ARTIFACT_SET_NAMESPACE,
    MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
    MinerULocalBatchAllocation,
    MinerULocalBatchGateway,
    MinerULocalBatchPollResult,
    MinerULocalBatchResultArchiveProvider,
    MinerUTextBaselineArchiveParserGateway,
)
from mm_chat_rag.projection import ProjectionContext, build_postgres_projection_batch
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

SECRET = "unit-test-mineru-token"
PDF_BODY = b"%PDF-1.7\nfixture\n%%EOF\n"
ZIP_BODY = b"PK\x03\x04fixture-zip-bytes"
SIGNED_UPLOAD_URL = (
    "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf"
    "?Expires=1&Signature=redacted"
)
RESULT_URL = "https://cdn-mineru.openxlab.org.cn/pdf/fixture-result.zip"
RESULT_PROXY_URL = "http://host.docker.internal:18081/mineru-result"
ARCHIVE_ENTRIES = (
    ("full.md", b"# full\n"),
    ("fixture_content_list.json", b"[]"),
    ("layout.json", b"{}"),
    ("fixture_model.json", b"{}"),
)
ARTIFACT_SET_ID = uuid.UUID("50000000-0000-0000-0000-000000000001")
PROJECTION_CONTEXT = ProjectionContext(
    collection_id=uuid.UUID("10000000-0000-0000-0000-000000000001"),
    document_id=uuid.UUID("20000000-0000-0000-0000-000000000001"),
    document_version_id=uuid.UUID("30000000-0000-0000-0000-000000000001"),
    file_id=uuid.UUID("40000000-0000-0000-0000-000000000001"),
    artifact_set_id=ARTIFACT_SET_ID,
    materialization_id=uuid.UUID("60000000-0000-0000-0000-000000000001"),
    index_generation_id=uuid.UUID("70000000-0000-0000-0000-000000000001"),
)
REQUEST_HASH = "8" * 64


def _source(
    *, body: bytes = PDF_BODY, content_type: str = "application/pdf"
) -> DocumentSource:
    return DocumentSource(
        body=body,
        source_sha256=hashlib.sha256(body).hexdigest(),
        content_type=content_type,
    )


def _allocation_payload(
    *,
    batch_id: object = "fixture-batch-id",
    file_urls: object = ("https://upload.invalid/api-upload/fixture.pdf",),
    code: object = 0,
) -> dict[str, object]:
    return {
        "code": code,
        "data": {
            "batch_id": batch_id,
            "file_urls": list(file_urls) if isinstance(file_urls, tuple) else file_urls,
        },
        "msg": "ok",
        "trace_id": "sensitive-trace-id",
    }


def _json_response(payload: object, *, status: int = 200) -> httpx.Response:
    content = json.dumps(payload, separators=(",", ":")).encode()
    return httpx.Response(
        status,
        headers={"Content-Type": "application/json; charset=utf-8"},
        content=content,
    )


def _archive(
    entries: tuple[tuple[str, bytes], ...] = ARCHIVE_ENTRIES,
    *,
    compression: int = zipfile.ZIP_STORED,
) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=compression) as archive:
        for name, content in entries:
            archive.writestr(name, content)
    return output.getvalue()


def _archive_with_extra_info(info: zipfile.ZipInfo, content: bytes) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        for name, entry_content in ARCHIVE_ENTRIES:
            archive.writestr(name, entry_content)
        archive.writestr(info, content)
    return output.getvalue()


def _corrupt_archive_content(content: bytes, needle: bytes = b"# full") -> bytes:
    raw = bytearray(content)
    index = raw.index(needle)
    raw[index] ^= 0xFF
    return bytes(raw)


def _allocation(
    upload_url: str = SIGNED_UPLOAD_URL,
    *,
    batch_id: str = "fixture-batch-id",
    filename: str = "document.pdf",
) -> MinerULocalBatchAllocation:
    return MinerULocalBatchAllocation(
        batch_id=batch_id,
        upload_urls=(upload_url,),
        filename=filename,
    )


def _poll_payload(
    *,
    state: object = "waiting-file",
    batch_id: object = "fixture-batch-id",
    file_name: object = "document.pdf",
    err_msg: object = "",
    full_zip_url: object | None = None,
    code: object = 0,
) -> dict[str, object]:
    result: dict[str, object] = {
        "err_msg": err_msg,
        "file_name": file_name,
        "state": state,
    }
    if state == "done":
        result["full_zip_url"] = RESULT_URL if full_zip_url is None else full_zip_url
    return {
        "code": code,
        "data": {
            "batch_id": batch_id,
            "extract_result": [result],
        },
        "msg": "ok",
        "trace_id": "sensitive-trace-id",
    }


def _done_poll_result(result_url: str = RESULT_URL) -> MinerULocalBatchPollResult:
    return MinerULocalBatchPollResult(
        batch_id="fixture-batch-id",
        filename="document.pdf",
        state="done",
        result_url=result_url,
    )


def _mapping_input(
    *,
    text: str = "Alpha 123\n\nBeta",
) -> mineru_module.MinerULocalBatchCanonicalMappingInput:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            ("fixture_content_list.json", b'[{"type":"text","text":"ok"}]'),
            ("layout.json", b'{"pages":[{"page":0}]}'),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    return gateway.prepare_canonical_mapping_input(object(), _source(), artifacts)


def _locator_mapping_input(
    *,
    text: str,
    content_items: list[dict[str, Any]],
    elements: list[dict[str, Any]],
    page: dict[str, Any] | None = None,
) -> mineru_module.MinerULocalBatchCanonicalMappingInput:
    gateway = MinerULocalBatchGateway(SECRET)
    page_body = {"elements": elements, "pageIndex": 0}
    if page is not None:
        page_body = {"elements": elements, **page}
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    content_items,
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {"pages": [page_body]},
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    return gateway.prepare_canonical_mapping_input(object(), _source(), artifacts)


def _project_locator(
    mapping_input: mineru_module.MinerULocalBatchCanonicalMappingInput,
) -> tuple[str, dict[str, Any]]:
    gateway = MinerULocalBatchGateway(SECRET)
    parsed = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        parsed.canonical_ir,
        parsed.chunk_manifest,
        PROJECTION_CONTEXT,
    )
    return batch.blocks[0].locator_kind, batch.blocks[0].locator


def _parse_context(*, stage: str = "parse") -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.UUID("80000000-0000-0000-0000-000000000001"),
        stage=stage,
        operation="initial",
        collection_id=PROJECTION_CONTEXT.collection_id,
        document_id=PROJECTION_CONTEXT.document_id,
        document_version_id=PROJECTION_CONTEXT.document_version_id,
        file_id=PROJECTION_CONTEXT.file_id,
        index_generation_id=PROJECTION_CONTEXT.index_generation_id,
        materialization_id=PROJECTION_CONTEXT.materialization_id,
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash=REQUEST_HASH,
        authority=None,
    )


class FakeMinerUResultArchiveProvider:
    def __init__(
        self,
        calls: list[str],
        archive_body: bytes,
    ) -> None:
        self._calls = calls
        self._archive_body = archive_body

    async def fetch_result_archive(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> bytes:
        self._calls.append("fetch_archive")
        assert context.stage == "parse"
        assert source.content_type == "application/pdf"
        return self._archive_body


async def test_mineru_gateway_missing_token_fails_before_http() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("missing token reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        with pytest.raises(PermanentJobError) as raised:
            MinerULocalBatchGateway(None, client=client)

    assert raised.value.error_code == MINERU_GATEWAY_CREDENTIALS_MISSING
    assert calls == 0


async def test_mineru_gateway_sends_locked_local_batch_allocate_request() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(_allocation_payload())

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        allocation = await gateway.allocate_upload(
            object(),
            _source(),
            filename="fixture.pdf",
        )

    assert allocation.batch_id == "fixture-batch-id"
    assert allocation.upload_urls == ("https://upload.invalid/api-upload/fixture.pdf",)
    assert allocation.filename == "fixture.pdf"
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(MINERU_ALLOCATE_UPLOAD_URL)
    assert request.headers["authorization"] == f"Bearer {SECRET}"
    assert json.loads(request.content) == {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": "fixture.pdf"}],
        "is_ocr": True,
        "model_version": "vlm",
    }
    assert SECRET.encode() not in request.content


@pytest.mark.parametrize(
    ("source", "filename"),
    [
        (_source(content_type="text/plain"), "fixture.pdf"),
        (_source(), "../fixture.pdf"),
    ],
)
async def test_mineru_gateway_rejects_unsupported_source_before_http(
    source: DocumentSource,
    filename: str,
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid source reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.allocate_upload(object(), source, filename=filename)

    assert raised.value.error_code == MINERU_GATEWAY_SOURCE_UNSUPPORTED
    assert calls == 0


async def test_mineru_gateway_rejects_oversized_pdf_before_http(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(mineru_module, "MAX_MINERU_SOURCE_BYTES", 16)
    source = _source(body=b"%PDF-1.7\n0123456789")
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("oversized source reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.allocate_upload(object(), source)

    assert raised.value.error_code == MINERU_GATEWAY_SOURCE_UNSUPPORTED
    assert calls == 0


async def test_mineru_gateway_retries_redacted_provider_status() -> None:
    transport = httpx.MockTransport(
        lambda _: _json_response({"error": SECRET}, status=503)
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.allocate_upload(object(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_retries_provider_code_failure_redacted() -> None:
    transport = httpx.MockTransport(
        lambda _: _json_response({"code": 429, "msg": SECRET})
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.allocate_upload(object(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_STATUS_INVALID
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_retries_transport_failure_redacted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError(
            f"sensitive transport detail {SECRET}",
            request=request,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.allocate_upload(object(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_REQUEST_FAILED
    assert SECRET not in str(raised.value)


@pytest.mark.parametrize(
    ("payload", "error_code"),
    [
        ({"code": 0, "data": None}, MINERU_GATEWAY_RESPONSE_INVALID),
        (_allocation_payload(batch_id=""), MINERU_GATEWAY_RESPONSE_INVALID),
        (_allocation_payload(file_urls=()), MINERU_GATEWAY_RESPONSE_INVALID),
        (
            _allocation_payload(file_urls=("http://upload.invalid/fixture.pdf",)),
            MINERU_GATEWAY_UPLOAD_URL_INVALID,
        ),
        (
            _allocation_payload(
                file_urls=("https://user@upload.invalid/fixture.pdf",)
            ),
            MINERU_GATEWAY_UPLOAD_URL_INVALID,
        ),
    ],
)
async def test_mineru_gateway_rejects_invalid_allocate_payload(
    payload: dict[str, Any],
    error_code: str,
) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _json_response(payload))
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.allocate_upload(object(), _source())

    assert raised.value.error_code == error_code
    assert "sensitive-trace-id" not in str(raised.value)


async def test_mineru_gateway_uploads_pdf_to_locked_signed_url() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(204, content=b"")

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        await gateway.upload_document(object(), _source(), _allocation())

    assert len(requests) == 1
    request = requests[0]
    assert request.method == "PUT"
    assert request.url == httpx.URL(SIGNED_UPLOAD_URL)
    assert request.content == PDF_BODY
    assert "authorization" not in request.headers
    assert "cookie" not in request.headers
    assert "content-type" not in request.headers


@pytest.mark.parametrize(
    "upload_url",
    [
        "https://evil.example/api-upload/fixture.pdf?Expires=1",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/not-api-upload/fixture.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/%2e%2e/a.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload\\a.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com:444/api-upload/fixture.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com:99999/api-upload/fixture.pdf",
        "https://user@mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf#frag",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf\n",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf?"
        + "x" * 4096,
    ],
)
async def test_mineru_gateway_rejects_upload_target_before_http(
    upload_url: str,
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid signed upload target reached provider")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.upload_document(object(), _source(), _allocation(upload_url))

    assert raised.value.error_code == MINERU_GATEWAY_UPLOAD_URL_INVALID
    assert calls == 0


async def test_mineru_gateway_upload_retries_status_redacted() -> None:
    transport = httpx.MockTransport(
        lambda _: httpx.Response(503, content=SECRET.encode())
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.upload_document(object(), _source(), _allocation())

    assert raised.value.error_code == MINERU_GATEWAY_UPLOAD_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_upload_retries_transport_failure_redacted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError(
            f"sensitive upload transport detail {SECRET}",
            request=request,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.upload_document(object(), _source(), _allocation())

    assert raised.value.error_code == MINERU_GATEWAY_REQUEST_FAILED
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_upload_rejects_non_pdf_before_http() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("non-PDF source reached upload target")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.upload_document(
                object(),
                _source(content_type="text/plain"),
                _allocation(),
            )

    assert raised.value.error_code == MINERU_GATEWAY_SOURCE_UNSUPPORTED
    assert calls == 0


async def test_mineru_gateway_polls_locked_batch_result_request() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return _json_response(_poll_payload())

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        result = await gateway.poll_batch_result(object(), _allocation())

    assert result.batch_id == "fixture-batch-id"
    assert result.filename == "document.pdf"
    assert result.state == "waiting-file"
    assert result.result_url is None
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "GET"
    assert request.url == httpx.URL(f"{MINERU_POLL_URL_PREFIX}fixture-batch-id")
    assert request.headers["authorization"] == f"Bearer {SECRET}"
    assert request.headers["accept"] == "application/json"
    assert request.headers["accept-encoding"] == "identity"
    assert "content-type" not in request.headers


async def test_mineru_gateway_poll_returns_done_result_url() -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: _json_response(_poll_payload(state="done"))
        )
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        result = await gateway.poll_batch_result(object(), _allocation())

    assert result.state == "done"
    assert result.result_url == RESULT_URL


async def test_mineru_gateway_poll_accepts_running_progress() -> None:
    payload = _poll_payload(state="running")
    poll_result = payload["data"]["extract_result"][0]  # type: ignore[index]
    poll_result["extract_progress"] = {
        "extracted_pages": 1,
        "start_time": "2026-07-16 12:00:00",
        "total_pages": 2,
    }
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _json_response(payload))
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        result = await gateway.poll_batch_result(object(), _allocation())

    assert result.state == "running"
    assert result.result_url is None


@pytest.mark.parametrize(
    ("payload", "error_code"),
    [
        (_poll_payload(batch_id="other-batch-id"), MINERU_GATEWAY_RESPONSE_INVALID),
        (
            {
                "code": 0,
                "data": {"batch_id": "fixture-batch-id", "extract_result": []},
                "msg": "ok",
            },
            MINERU_GATEWAY_RESPONSE_INVALID,
        ),
        (_poll_payload(state="unknown"), MINERU_GATEWAY_RESPONSE_INVALID),
        (_poll_payload(file_name="other.pdf"), MINERU_GATEWAY_RESPONSE_INVALID),
        (
            _poll_payload(state="done", full_zip_url=""),
            MINERU_GATEWAY_RESULT_URL_INVALID,
        ),
        (
            _poll_payload(
                state="done",
                full_zip_url="https://evil.example/pdf/fixture-result.zip",
            ),
            MINERU_GATEWAY_RESULT_URL_INVALID,
        ),
        (
            _poll_payload(
                state="done",
                full_zip_url="https://cdn-mineru.openxlab.org.cn/pdf/%2e%2e/a.zip",
            ),
            MINERU_GATEWAY_RESULT_URL_INVALID,
        ),
        (
            _poll_payload(
                state="done",
                full_zip_url=f"{RESULT_URL}?token=redacted",
            ),
            MINERU_GATEWAY_RESULT_URL_INVALID,
        ),
        (_poll_payload(state="failed", err_msg=""), MINERU_GATEWAY_RESPONSE_INVALID),
    ],
)
async def test_mineru_gateway_rejects_invalid_poll_payload(
    payload: dict[str, Any],
    error_code: str,
) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: _json_response(payload))
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.poll_batch_result(object(), _allocation())

    assert raised.value.error_code == error_code
    assert "sensitive-trace-id" not in str(raised.value)


async def test_mineru_gateway_poll_retries_status_redacted() -> None:
    transport = httpx.MockTransport(
        lambda _: _json_response({"error": SECRET}, status=503)
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.poll_batch_result(object(), _allocation())

    assert raised.value.error_code == MINERU_GATEWAY_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_poll_retries_provider_code_failure_redacted() -> None:
    transport = httpx.MockTransport(
        lambda _: _json_response({"code": 429, "msg": SECRET})
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.poll_batch_result(object(), _allocation())

    assert raised.value.error_code == MINERU_GATEWAY_STATUS_INVALID
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_poll_retries_transport_failure_redacted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadError(
            f"sensitive poll transport detail {SECRET}",
            request=request,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.poll_batch_result(object(), _allocation())

    assert raised.value.error_code == MINERU_GATEWAY_REQUEST_FAILED
    assert SECRET not in str(raised.value)


async def test_mineru_gateway_downloads_locked_result_archive() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(
            200,
            headers={
                "Content-Length": str(len(ZIP_BODY)),
                "Content-Type": "application/zip",
            },
            content=ZIP_BODY,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        client.cookies.set(
            "session",
            "leak",
            domain="cdn-mineru.openxlab.org.cn",
            path="/",
        )
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        body = await gateway.download_result_archive(object(), _done_poll_result())

    assert body == ZIP_BODY
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "GET"
    assert request.url == httpx.URL(RESULT_URL)
    assert request.headers["accept"] == "application/zip"
    assert request.headers["accept-encoding"] == "identity"
    assert "authorization" not in request.headers
    assert "cookie" not in request.headers
    assert "content-type" not in request.headers


async def test_mineru_gateway_download_proxy_preserves_result_url_fence() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(
            200,
            headers={
                "Content-Length": str(len(ZIP_BODY)),
                "Content-Type": "application/zip",
            },
            content=ZIP_BODY,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        client.cookies.set(
            "session",
            "leak",
            domain="host.docker.internal",
            path="/",
        )
        gateway = MinerULocalBatchGateway(
            SECRET,
            client=client,
            result_proxy_url=RESULT_PROXY_URL,
        )
        body = await gateway.download_result_archive(object(), _done_poll_result())

    assert body == ZIP_BODY
    assert len(requests) == 1
    request = requests[0]
    assert request.method == "POST"
    assert request.url == httpx.URL(RESULT_PROXY_URL)
    assert request.headers["accept"] == "application/zip"
    assert request.headers["accept-encoding"] == "identity"
    assert request.headers["content-type"] == "application/json"
    assert "authorization" not in request.headers
    assert "cookie" not in request.headers
    assert json.loads(request.content) == {"resultUrl": RESULT_URL}


async def test_mineru_gateway_download_proxy_still_rejects_bad_result_url() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("bad result url reached proxy")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(
            SECRET,
            client=client,
            result_proxy_url=RESULT_PROXY_URL,
        )
        with pytest.raises(PermanentJobError) as raised:
            await gateway.download_result_archive(
                object(),
                _done_poll_result("https://evil.example/pdf/result.zip"),
            )

    assert raised.value.error_code == MINERU_GATEWAY_RESULT_URL_INVALID
    assert calls == 0


@pytest.mark.parametrize(
    "proxy_url",
    [
        "ftp://host/proxy",
        "http://user@host/proxy",
        "http://host/proxy?target=x",
        "http://host/%2e%2e/proxy",
    ],
)
async def test_mineru_gateway_rejects_unsafe_result_proxy_url(
    proxy_url: str,
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        MinerULocalBatchGateway(SECRET, result_proxy_url=proxy_url)

    assert raised.value.error_code == MINERU_GATEWAY_RESULT_PROXY_INVALID


async def test_mineru_gateway_download_rejects_non_done_before_http() -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("non-done poll result reached download target")

    async with httpx.AsyncClient(transport=httpx.MockTransport(forbidden)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.download_result_archive(
                object(),
                MinerULocalBatchPollResult(
                    batch_id="fixture-batch-id",
                    filename="document.pdf",
                    state="running",
                ),
            )

    assert raised.value.error_code == MINERU_GATEWAY_RESULT_URL_INVALID
    assert calls == 0


async def test_mineru_gateway_download_retries_status_redacted() -> None:
    transport = httpx.MockTransport(
        lambda _: httpx.Response(503, content=SECRET.encode())
    )
    async with httpx.AsyncClient(transport=transport) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.download_result_archive(object(), _done_poll_result())

    assert raised.value.error_code == MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30
    assert SECRET not in str(raised.value)


@pytest.mark.parametrize(
    ("headers", "error_code"),
    [
        ({"Content-Type": "text/plain"}, MINERU_GATEWAY_RESPONSE_INVALID),
        (
            {"Content-Encoding": "gzip", "Content-Type": "application/zip"},
            MINERU_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {"Content-Length": "not-a-number", "Content-Type": "application/zip"},
            MINERU_GATEWAY_RESPONSE_INVALID,
        ),
        (
            {
                "Content-Length": str(32 * 1024 * 1024 + 1),
                "Content-Type": "application/zip",
            },
            MINERU_GATEWAY_RESPONSE_TOO_LARGE,
        ),
    ],
)
async def test_mineru_gateway_download_rejects_invalid_response_headers(
    headers: dict[str, str],
    error_code: str,
) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200,
                headers=headers,
                stream=httpx.ByteStream(ZIP_BODY),
            )
        )
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.download_result_archive(object(), _done_poll_result())

    assert raised.value.error_code == error_code


async def test_mineru_gateway_download_rejects_oversized_body(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(mineru_module, "MAX_MINERU_RESULT_ARCHIVE_BYTES", 4)
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200,
                headers={"Content-Type": "application/zip"},
                content=ZIP_BODY,
            )
        )
    ) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(PermanentJobError) as raised:
            await gateway.download_result_archive(object(), _done_poll_result())

    assert raised.value.error_code == MINERU_GATEWAY_RESPONSE_TOO_LARGE


async def test_mineru_gateway_download_retries_transport_failure_redacted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadError(
            f"sensitive download transport detail {SECRET}",
            request=request,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gateway = MinerULocalBatchGateway(SECRET, client=client)
        with pytest.raises(RetryableJobError) as raised:
            await gateway.download_result_archive(object(), _done_poll_result())

    assert raised.value.error_code == MINERU_GATEWAY_REQUEST_FAILED
    assert SECRET not in str(raised.value)


async def test_mineru_result_archive_provider_runs_single_batch_sequence() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST" and request.url == httpx.URL(
            MINERU_ALLOCATE_UPLOAD_URL
        ):
            return _json_response(_allocation_payload(file_urls=(SIGNED_UPLOAD_URL,)))
        if request.method == "PUT" and request.url == httpx.URL(SIGNED_UPLOAD_URL):
            return httpx.Response(204)
        if request.method == "GET" and request.url == httpx.URL(
            f"{MINERU_POLL_URL_PREFIX}fixture-batch-id"
        ):
            return _json_response(_poll_payload(state="done"))
        if request.method == "GET" and request.url == httpx.URL(RESULT_URL):
            return httpx.Response(
                200,
                headers={
                    "Content-Length": str(len(ZIP_BODY)),
                    "Content-Type": "application/zip",
                },
                content=ZIP_BODY,
            )
        raise AssertionError(
            f"unexpected MinerU request {request.method} {request.url}"
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        provider = MinerULocalBatchResultArchiveProvider(
            MinerULocalBatchGateway(SECRET, client=client)
        )
        archive_body = await provider.fetch_result_archive(_parse_context(), _source())

    observed = [
        (request.method, str(request.url).split("?")[0]) for request in requests
    ]
    assert archive_body == ZIP_BODY
    assert observed == [
        ("POST", MINERU_ALLOCATE_UPLOAD_URL),
        ("PUT", SIGNED_UPLOAD_URL.split("?")[0]),
        ("GET", f"{MINERU_POLL_URL_PREFIX}fixture-batch-id"),
        ("GET", RESULT_URL),
    ]
    assert requests[0].headers["authorization"] == f"Bearer {SECRET}"
    assert "authorization" not in requests[1].headers
    assert "authorization" not in requests[3].headers


async def test_mineru_result_archive_provider_polls_same_batch_until_done() -> None:
    requests: list[httpx.Request] = []
    poll_count = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal poll_count
        requests.append(request)
        if request.method == "POST":
            return _json_response(_allocation_payload(file_urls=(SIGNED_UPLOAD_URL,)))
        if request.method == "PUT":
            return httpx.Response(204)
        if request.method == "GET" and str(request.url).startswith(
            MINERU_POLL_URL_PREFIX
        ):
            poll_count += 1
            if poll_count == 1:
                payload = _poll_payload(state="running")
                poll_result = payload["data"]["extract_result"][0]  # type: ignore[index]
                poll_result["extract_progress"] = {
                    "extracted_pages": 1,
                    "start_time": "2026-07-16 12:00:00",
                    "total_pages": 2,
                }
                return _json_response(payload)
            return _json_response(_poll_payload(state="done"))
        if request.method == "GET" and request.url == httpx.URL(RESULT_URL):
            return httpx.Response(
                200,
                headers={
                    "Content-Length": str(len(ZIP_BODY)),
                    "Content-Type": "application/zip",
                },
                content=ZIP_BODY,
            )
        raise AssertionError(f"unexpected MinerU request {request.method}")

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        provider = MinerULocalBatchResultArchiveProvider(
            MinerULocalBatchGateway(SECRET, client=client),
            poll_interval_seconds=0,
            poll_max_attempts=2,
        )
        archive_body = await provider.fetch_result_archive(_parse_context(), _source())

    assert archive_body == ZIP_BODY
    assert [
        (request.method, str(request.url).split("?")[0]) for request in requests
    ] == [
        ("POST", MINERU_ALLOCATE_UPLOAD_URL),
        ("PUT", SIGNED_UPLOAD_URL.split("?")[0]),
        ("GET", f"{MINERU_POLL_URL_PREFIX}fixture-batch-id"),
        ("GET", f"{MINERU_POLL_URL_PREFIX}fixture-batch-id"),
        ("GET", RESULT_URL),
    ]


async def test_mineru_result_archive_provider_retries_after_poll_exhaustion() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST":
            return _json_response(_allocation_payload(file_urls=(SIGNED_UPLOAD_URL,)))
        if request.method == "PUT":
            return httpx.Response(204)
        if request.method == "GET":
            payload = _poll_payload(state="running")
            poll_result = payload["data"]["extract_result"][0]  # type: ignore[index]
            poll_result["extract_progress"] = {
                "extracted_pages": 1,
                "start_time": "2026-07-16 12:00:00",
                "total_pages": 2,
            }
            return _json_response(payload)
        raise AssertionError(f"unexpected MinerU request {request.method}")

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        provider = MinerULocalBatchResultArchiveProvider(
            MinerULocalBatchGateway(SECRET, client=client),
            poll_interval_seconds=0,
            poll_max_attempts=3,
        )
        with pytest.raises(RetryableJobError) as raised:
            await provider.fetch_result_archive(_parse_context(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_RESULT_NOT_READY
    assert raised.value.retry_after_seconds == 30
    assert [request.method for request in requests] == [
        "POST",
        "PUT",
        "GET",
        "GET",
        "GET",
    ]


async def test_mineru_result_archive_provider_rejects_failed_poll_redacted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        if request.method == "POST":
            return _json_response(_allocation_payload(file_urls=(SIGNED_UPLOAD_URL,)))
        if request.method == "PUT":
            return httpx.Response(204)
        return _json_response(_poll_payload(state="failed", err_msg=SECRET))

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        provider = MinerULocalBatchResultArchiveProvider(
            MinerULocalBatchGateway(SECRET, client=client)
        )
        with pytest.raises(PermanentJobError) as raised:
            await provider.fetch_result_archive(_parse_context(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_RESULT_FAILED
    assert SECRET not in str(raised.value)


def test_mineru_result_archive_provider_default_off() -> None:
    with pytest.raises(PermanentJobError) as raised:
        MinerULocalBatchResultArchiveProvider(None)

    assert raised.value.error_code == MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED


def test_mineru_gateway_validates_result_archive_summary() -> None:
    archive_body = _archive()
    gateway = MinerULocalBatchGateway(SECRET)

    summary = gateway.validate_result_archive(object(), archive_body)

    assert summary.archive_byte_count == len(archive_body)
    assert summary.archive_sha256 == hashlib.sha256(archive_body).hexdigest()
    assert summary.entry_count == len(ARCHIVE_ENTRIES)
    assert summary.full_markdown_present is True
    assert summary.content_list_present is True
    assert summary.middle_json_present is True
    assert summary.model_json_present is True


@pytest.mark.parametrize(
    "archive_body",
    [
        b"",
        b"not-a-zip",
        _archive((("full.md", b"# full\n"),)),
        _archive((("../full.md", b"x"), *ARCHIVE_ENTRIES[1:])),
        _archive((("nested\\full.md", b"x"), *ARCHIVE_ENTRIES[1:])),
    ],
)
def test_mineru_gateway_rejects_invalid_result_archive(
    archive_body: bytes,
) -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), archive_body)

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_duplicate_archive_entries() -> None:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr("full.md", b"# full\n")
        with pytest.warns(UserWarning, match="Duplicate name"):
            archive.writestr("full.md", b"# duplicate\n")
        for name, content in ARCHIVE_ENTRIES[1:]:
            archive.writestr(name, content)

    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), output.getvalue())

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_symlink_archive_entry() -> None:
    info = zipfile.ZipInfo("link")
    info.external_attr = (stat.S_IFLNK | 0o777) << 16
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), _archive_with_extra_info(info, b"x"))

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_crc_mismatch_archive() -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), _corrupt_archive_content(_archive()))

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_archive_entry_limits(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(mineru_module, "MAX_MINERU_ARCHIVE_ENTRY_BYTES", 1)
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), _archive())

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_archive_total_limits(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(mineru_module, "MAX_MINERU_ARCHIVE_TOTAL_BYTES", 1)
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(object(), _archive())

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_rejects_archive_compression_ratio(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(mineru_module, "MAX_MINERU_ARCHIVE_COMPRESSION_RATIO", 1)
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.validate_result_archive(
            object(),
            _archive(
                (
                    ("full.md", b"a" * 1024),
                    *ARCHIVE_ENTRIES[1:],
                ),
                compression=zipfile.ZIP_DEFLATED,
            ),
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_extracts_result_archive_artifacts() -> None:
    archive_body = _archive()
    gateway = MinerULocalBatchGateway(SECRET)

    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    assert artifacts.summary.archive_sha256 == hashlib.sha256(archive_body).hexdigest()
    assert artifacts.full_markdown == b"# full\n"
    assert artifacts.content_list_json == b"[]"
    assert artifacts.middle_json == b"{}"
    assert artifacts.model_json == b"{}"
    assert not hasattr(artifacts, "entry_names")


def test_mineru_gateway_extracts_nested_role_artifacts_without_names() -> None:
    archive_body = _archive(
        (
            ("nested/full.md", b"# nested\n"),
            ("nested/content_list.json", b"[]"),
            ("nested/middle.json", b'{"middle":true}'),
            ("nested/model.json", b'{"model":true}'),
            ("nested/extra.txt", b"ignored"),
        )
    )
    gateway = MinerULocalBatchGateway(SECRET)

    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    assert artifacts.summary.entry_count == 5
    assert artifacts.full_markdown == b"# nested\n"
    assert artifacts.middle_json == b'{"middle":true}'
    assert artifacts.model_json == b'{"model":true}'
    assert not hasattr(artifacts, "full_markdown_name")


def test_mineru_gateway_rejects_duplicate_archive_roles() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            *ARCHIVE_ENTRIES,
            ("middle.json", b'{"duplicate":true}'),
        )
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.extract_result_archive_artifacts(object(), archive_body)

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_extract_revalidates_archive() -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.extract_result_archive_artifacts(object(), b"not-a-zip")

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_decodes_result_archive_artifacts() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", "正文\n".encode()),
            ("fixture_content_list.json", b'[{"type":"text","text":"ok"}]'),
            ("layout.json", b'{"pages":[{"page":0}]}'),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    decoded = gateway.decode_result_archive_artifacts(object(), artifacts)

    assert decoded.summary == artifacts.summary
    assert decoded.full_markdown == "正文\n"
    assert decoded.content_list_json == [{"type": "text", "text": "ok"}]
    assert decoded.middle_json == {"pages": [{"page": 0}]}
    assert decoded.model_json == {"model": "vlm"}


def test_mineru_gateway_accepts_array_model_json_from_live_mineru() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", b"Live text\n"),
            ("fixture_content_list.json", b'[{"type":"text","text":"Live text"}]'),
            ("layout.json", b'{"pdf_info":[{"page_idx":0}]}'),
            ("fixture_model.json", b'[[{"type":"text","content":"Live text"}]]'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    decoded = gateway.decode_result_archive_artifacts(object(), artifacts)

    assert decoded.model_json == [[{"type": "text", "content": "Live text"}]]


def test_mineru_gateway_rejects_scalar_model_json() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", b"Live text\n"),
            ("fixture_content_list.json", b'[{"type":"text","text":"Live text"}]'),
            ("layout.json", b'{"pdf_info":[{"page_idx":0}]}'),
            ("fixture_model.json", b'"not-an-object-or-list"'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    with pytest.raises(PermanentJobError) as raised:
        gateway.decode_result_archive_artifacts(object(), artifacts)

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_prepares_hash_bound_canonical_mapping_input() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", "正文\n".encode()),
            ("fixture_content_list.json", b'[{"type":"text","text":"ok"}]'),
            ("layout.json", b'{"pages":[{"page":0}]}'),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)

    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    assert mapping_input.source_sha256 == hashlib.sha256(PDF_BODY).hexdigest()
    assert mapping_input.source_byte_count == len(PDF_BODY)
    assert mapping_input.source_content_type == "application/pdf"
    assert mapping_input.archive_sha256 == hashlib.sha256(archive_body).hexdigest()
    assert mapping_input.archive_byte_count == len(archive_body)
    assert [item.role for item in mapping_input.role_digests] == [
        "full_markdown",
        "content_list_json",
        "middle_json",
        "model_json",
    ]
    assert [item.byte_count for item in mapping_input.role_digests] == [
        len("正文\n".encode()),
        len(b'[{"type":"text","text":"ok"}]'),
        len(b'{"pages":[{"page":0}]}'),
        len(b'{"model":"vlm"}'),
    ]
    assert [item.sha256 for item in mapping_input.role_digests] == [
        hashlib.sha256(artifacts.full_markdown).hexdigest(),
        hashlib.sha256(artifacts.content_list_json).hexdigest(),
        hashlib.sha256(artifacts.middle_json).hexdigest(),
        hashlib.sha256(artifacts.model_json).hexdigest(),
    ]
    assert mapping_input.decoded.full_markdown == "正文\n"
    assert mapping_input.decoded.content_list_json == [{"type": "text", "text": "ok"}]
    assert mapping_input.decoded.middle_json == {"pages": [{"page": 0}]}
    assert mapping_input.decoded.model_json == {"model": "vlm"}
    assert not hasattr(mapping_input, "entry_names")
    assert not hasattr(mapping_input, "canonical_ir")
    assert not hasattr(mapping_input, "chunk_manifest")


def test_mineru_gateway_prepare_mapping_input_rejects_source_hash_mismatch() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    artifacts = gateway.extract_result_archive_artifacts(object(), _archive())
    mismatched_source = DocumentSource(
        body=PDF_BODY,
        source_sha256="0" * 64,
        content_type="application/pdf",
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.prepare_canonical_mapping_input(
            object(),
            mismatched_source,
            artifacts,
        )

    assert raised.value.error_code == MINERU_GATEWAY_SOURCE_HASH_MISMATCH


def test_mineru_gateway_prepare_mapping_input_reuses_decode_gates() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    artifacts = gateway.extract_result_archive_artifacts(
        object(),
        _archive(
            (
                ("full.md", b"# full\n"),
                ("fixture_content_list.json", b"{]"),
                ("layout.json", b"{}"),
                ("fixture_model.json", b"{}"),
            )
        ),
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.prepare_canonical_mapping_input(
            object(),
            _source(),
            artifacts,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_builds_text_baseline_parse_artifacts() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text_body = b"Alpha 123\n\nBeta"
    mapping_input = _mapping_input(text="Alpha 123\n\nBeta")

    artifacts = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        artifacts.canonical_ir,
        artifacts.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert artifacts.artifact_set_id == ARTIFACT_SET_ID
    assert artifacts.canonical_ir["schemaVersion"] == "canonical-ir.v2"
    assert artifacts.canonical_ir["source"] == {
        "bytes": len(PDF_BODY),
        "format": "pdf",
        "sha256": hashlib.sha256(PDF_BODY).hexdigest(),
    }
    assert artifacts.canonical_ir["textBuffer"] == {
        "bytes": len(text_body),
        "encoding": "utf-8",
        "sha256": hashlib.sha256(text_body).hexdigest(),
        "text": "Alpha 123\n\nBeta",
    }
    assert artifacts.chunk_manifest["schemaVersion"] == "chunk-manifest.v2"
    assert artifacts.chunk_manifest["chunkProfileHash"] == (
        MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH
    )
    assert artifacts.chunk_manifest["parentCount"] == 1
    assert artifacts.chunk_manifest["childCount"] == 1
    assert artifacts.chunk_manifest["spanCount"] == 2
    assert len(batch.blocks) == 1
    assert len(batch.parent_chunks) == 1
    assert len(batch.child_chunks) == 1
    assert len(batch.chunk_block_spans) == 2
    assert batch.blocks[0].block_type == "paragraph"
    assert batch.blocks[0].text_content == "Alpha 123\n\nBeta"
    assert batch.parent_chunks[0].content == "Alpha 123\n\nBeta"
    assert batch.child_chunks[0].content == "Alpha 123\n\nBeta"
    assert batch.child_search_projections[0].exact_terms == ("123", "alpha", "beta")
    assert batch.chunk_profile_hash == MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH


def test_mineru_gateway_text_baseline_projects_basic_page_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "Synthetic MinerU 文本"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                b'[{"type":"text","text":"Synthetic MinerU \\u6587\\u672c"}]',
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 0,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [
                                            72000,
                                            120000,
                                            540000,
                                            180000,
                                        ],
                                        "kind": "text",
                                        "text": text,
                                    }
                                ],
                            }
                        ]
                    },
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    parsed = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        parsed.canonical_ir,
        parsed.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert batch.blocks[0].locator_kind == "page_bbox"
    assert batch.blocks[0].locator == {
        "kind": "page_bbox",
        "page": 0,
        "x1": 72000,
        "y1": 120000,
        "x2": 540000,
        "y2": 180000,
    }
    fragments = batch.parent_chunks[0].locator_summary["fragments"]
    assert isinstance(fragments, list)
    first_fragment = fragments[0]
    assert isinstance(first_fragment, dict)
    assert first_fragment["locator"] == batch.blocks[0].locator


def test_mineru_gateway_text_baseline_projects_source_text_page_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "E = mc²"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    [{"sourceText": text, "type": "formula"}],
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 1,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [
                                            72000,
                                            360000,
                                            300000,
                                            400000,
                                        ],
                                        "kind": "formula",
                                        "sourceText": text,
                                    }
                                ],
                            }
                        ]
                    },
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    parsed = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        parsed.canonical_ir,
        parsed.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert batch.blocks[0].locator_kind == "page_bbox"
    assert batch.blocks[0].locator == {
        "kind": "page_bbox",
        "page": 1,
        "x1": 72000,
        "y1": 360000,
        "x2": 300000,
        "y2": 400000,
    }
    assert batch.parent_chunks[0].content == text


def test_mineru_gateway_text_baseline_projects_table_page_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "| key | value |\n| --- | --- |\n| café | 中文 |"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    [{"text": text, "type": "table"}],
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 2,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [
                                            72000,
                                            200000,
                                            540000,
                                            340000,
                                        ],
                                        "kind": "table",
                                        "rows": [
                                            {"cells": [{"text": "not parsed"}]},
                                        ],
                                    }
                                ],
                            }
                        ]
                    },
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    parsed = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        parsed.canonical_ir,
        parsed.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert batch.blocks[0].locator_kind == "page_bbox"
    assert batch.blocks[0].locator == {
        "kind": "page_bbox",
        "page": 2,
        "x1": 72000,
        "y1": 200000,
        "x2": 540000,
        "y2": 340000,
    }
    assert batch.parent_chunks[0].content == text


def test_mineru_gateway_text_baseline_projects_image_page_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "![diagram](images/diagram.png)"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    [{"img_path": "images/diagram.png", "text": text, "type": "image"}],
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 3,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [
                                            144000,
                                            180000,
                                            468000,
                                            396000,
                                        ],
                                        "kind": "image",
                                        "path": "images/diagram.png",
                                    }
                                ],
                            }
                        ]
                    },
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    parsed = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        parsed.canonical_ir,
        parsed.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert batch.blocks[0].locator_kind == "page_bbox"
    assert batch.blocks[0].locator == {
        "kind": "page_bbox",
        "page": 3,
        "x1": 144000,
        "y1": 180000,
        "x2": 468000,
        "y2": 396000,
    }
    assert batch.parent_chunks[0].content == text


def test_mineru_gateway_text_baseline_rejects_ambiguous_page_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "Duplicate locator text"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                b'[{"type":"text","text":"Duplicate locator text"}]',
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 0,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [0, 0, 100, 100],
                                        "text": text,
                                    },
                                    {
                                        "bboxMilliPoint": [100, 100, 200, 200],
                                        "text": text,
                                    },
                                ],
                            }
                        ]
                    },
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_text_baseline_rejects_ambiguous_table_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "| a |\n| - |\n| b |"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    [{"text": text, "type": "table"}],
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 0,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [0, 0, 100, 100],
                                        "kind": "table",
                                    },
                                    {
                                        "bboxMilliPoint": [100, 100, 200, 200],
                                        "kind": "table",
                                    },
                                ],
                            }
                        ]
                    },
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_text_baseline_rejects_ambiguous_image_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "![diagram](images/diagram.png)"
    archive_body = _archive(
        (
            ("full.md", text.encode()),
            (
                "fixture_content_list.json",
                json.dumps(
                    [{"img_path": "images/diagram.png", "text": text, "type": "image"}],
                    separators=(",", ":"),
                ).encode(),
            ),
            (
                "layout.json",
                json.dumps(
                    {
                        "pages": [
                            {
                                "pageIndex": 0,
                                "elements": [
                                    {
                                        "bboxMilliPoint": [0, 0, 100, 100],
                                        "kind": "image",
                                    },
                                    {
                                        "bboxMilliPoint": [100, 100, 200, 200],
                                        "kind": "image",
                                    },
                                ],
                            }
                        ]
                    },
                    separators=(",", ":"),
                ).encode(),
            ),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    artifacts = gateway.extract_result_archive_artifacts(object(), archive_body)
    mapping_input = gateway.prepare_canonical_mapping_input(
        object(),
        _source(),
        artifacts,
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_text_baseline_falls_back_without_content_match() -> None:
    text = "Visible text"
    locator_kind, locator = _project_locator(
        _locator_mapping_input(
            text=text,
            content_items=[{"text": "other text", "type": "text"}],
            elements=[
                {
                    "bboxMilliPoint": [0, 0, 100, 100],
                    "text": text,
                }
            ],
        )
    )

    assert locator_kind == "line_range"
    assert locator == {"endLine": 0, "kind": "line_range", "startLine": 0}


def test_mineru_gateway_text_baseline_rejects_duplicate_content_matches() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "Duplicate content-list text"
    mapping_input = _locator_mapping_input(
        text=text,
        content_items=[
            {"text": text, "type": "text"},
            {"text": text, "type": "text"},
        ],
        elements=[{"bboxMilliPoint": [0, 0, 100, 100], "text": text}],
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


@pytest.mark.parametrize(
    ("page", "element"),
    [
        ({}, {"bboxMilliPoint": [0, 0, 100, 100], "text": "Bad locator"}),
        (
            {"pageIndex": -1},
            {"bboxMilliPoint": [0, 0, 100, 100], "text": "Bad locator"},
        ),
        ({"pageIndex": 0}, {"text": "Bad locator"}),
        (
            {"pageIndex": 0},
            {"bboxMilliPoint": [0, -1, 100, 100], "text": "Bad locator"},
        ),
        (
            {"pageIndex": 0},
            {"bboxMilliPoint": [0, 0, 0, 100], "text": "Bad locator"},
        ),
        (
            {"pageIndex": 0},
            {"bboxMilliPoint": [0, 0, 100, "100"], "text": "Bad locator"},
        ),
    ],
)
def test_mineru_gateway_text_baseline_rejects_malformed_locator(
    page: dict[str, Any],
    element: dict[str, Any],
) -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "Bad locator"
    mapping_input = _locator_mapping_input(
        text=text,
        content_items=[{"text": text, "type": "text"}],
        elements=[element],
        page=page,
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_text_baseline_does_not_infer_formula_by_kind_only() -> None:
    text = "E = mc²"
    locator_kind, locator = _project_locator(
        _locator_mapping_input(
            text=text,
            content_items=[{"sourceText": text, "type": "formula"}],
            elements=[
                {
                    "bboxMilliPoint": [0, 0, 100, 100],
                    "kind": "formula",
                }
            ],
        )
    )

    assert locator_kind == "line_range"
    assert locator == {"endLine": 0, "kind": "line_range", "startLine": 0}


def test_mineru_gateway_text_baseline_rejects_ambiguous_formula_locator() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    text = "E = mc²"
    mapping_input = _locator_mapping_input(
        text=text,
        content_items=[{"sourceText": text, "type": "formula"}],
        elements=[
            {
                "bboxMilliPoint": [0, 0, 100, 100],
                "kind": "formula",
                "sourceText": text,
            },
            {
                "bboxMilliPoint": [100, 100, 200, 200],
                "kind": "formula",
                "sourceText": text,
            },
        ],
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            mapping_input,
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_text_baseline_chunks_long_markdown() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    mapping_input = _mapping_input(text="甲" * 900)

    artifacts = gateway.build_text_baseline_parse_artifacts(
        object(),
        mapping_input,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        artifacts.canonical_ir,
        artifacts.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    assert artifacts.chunk_manifest["parentCount"] == 2
    assert artifacts.chunk_manifest["childCount"] == 2
    assert artifacts.chunk_manifest["spanCount"] == 4
    assert [chunk.token_count for chunk in batch.child_chunks] == [600, 75]
    assert "".join(chunk.content for chunk in batch.child_chunks) == "甲" * 900


def test_mineru_gateway_text_baseline_rejects_wrong_mapping_input() -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts(
            object(),
            object(),  # type: ignore[arg-type]
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_composes_archive_to_text_baseline_parse_artifacts() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    archive_body = _archive(
        (
            ("full.md", b"Composed 456\n\nArtifact"),
            ("fixture_content_list.json", b'[{"type":"text","text":"ok"}]'),
            ("layout.json", b'{"pages":[{"page":0}]}'),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )

    artifacts = gateway.build_text_baseline_parse_artifacts_from_archive(
        object(),
        _source(),
        archive_body,
        artifact_set_id=ARTIFACT_SET_ID,
    )
    batch = build_postgres_projection_batch(
        artifacts.canonical_ir,
        artifacts.chunk_manifest,
        PROJECTION_CONTEXT,
    )

    text_buffer = artifacts.canonical_ir["textBuffer"]
    assert isinstance(text_buffer, dict)
    assert artifacts.artifact_set_id == ARTIFACT_SET_ID
    assert text_buffer["text"] == "Composed 456\n\nArtifact"
    assert artifacts.chunk_manifest["sourceSha256"] == hashlib.sha256(
        PDF_BODY
    ).hexdigest()
    assert batch.parent_chunks[0].content == "Composed 456\n\nArtifact"
    assert batch.child_search_projections[0].exact_terms == (
        "456",
        "artifact",
        "composed",
    )


def test_mineru_gateway_archive_to_text_baseline_rejects_invalid_archive() -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts_from_archive(
            object(),
            _source(),
            b"not-a-zip",
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID


def test_mineru_gateway_archive_to_text_baseline_rejects_source_mismatch() -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    mismatched_source = DocumentSource(
        body=PDF_BODY,
        source_sha256="0" * 64,
        content_type="application/pdf",
    )

    with pytest.raises(PermanentJobError) as raised:
        gateway.build_text_baseline_parse_artifacts_from_archive(
            object(),
            mismatched_source,
            _archive(),
            artifact_set_id=ARTIFACT_SET_ID,
        )

    assert raised.value.error_code == MINERU_GATEWAY_SOURCE_HASH_MISMATCH


async def test_mineru_parser_gateway_default_off_before_archive_fetch() -> None:
    calls: list[str] = []
    provider = FakeMinerUResultArchiveProvider(calls, _archive())
    parser = MinerUTextBaselineArchiveParserGateway()

    with pytest.raises(PermanentJobError) as raised:
        await parser.parse_document(_parse_context(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED
    assert calls == []
    assert isinstance(provider, FakeMinerUResultArchiveProvider)


async def test_mineru_parser_gateway_parses_archive_baseline() -> None:
    calls: list[str] = []
    archive_body = _archive(
        (
            ("full.md", b"Parser 789\n\nGateway"),
            ("fixture_content_list.json", b'[{"type":"text","text":"ok"}]'),
            ("layout.json", b'{"pages":[{"page":0}]}'),
            ("fixture_model.json", b'{"model":"vlm"}'),
        )
    )
    parser = MinerUTextBaselineArchiveParserGateway(
        FakeMinerUResultArchiveProvider(calls, archive_body)
    )
    context = _parse_context()

    first = await parser.parse_document(context, _source())
    second = await parser.parse_document(context, _source())
    batch = build_postgres_projection_batch(
        first.canonical_ir,
        first.chunk_manifest,
        ProjectionContext(
            collection_id=PROJECTION_CONTEXT.collection_id,
            document_id=PROJECTION_CONTEXT.document_id,
            document_version_id=PROJECTION_CONTEXT.document_version_id,
            file_id=PROJECTION_CONTEXT.file_id,
            artifact_set_id=first.artifact_set_id,
            materialization_id=PROJECTION_CONTEXT.materialization_id,
            index_generation_id=PROJECTION_CONTEXT.index_generation_id,
        ),
    )

    expected_artifact_set_id = uuid.uuid5(
        MINERU_TEXT_BASELINE_ARTIFACT_SET_NAMESPACE,
        ":".join(
            (
                str(PROJECTION_CONTEXT.materialization_id),
                hashlib.sha256(PDF_BODY).hexdigest(),
                MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            )
        ),
    )
    assert calls == ["fetch_archive", "fetch_archive"]
    assert first.artifact_set_id == expected_artifact_set_id
    assert second.artifact_set_id == first.artifact_set_id
    assert batch.parent_chunks[0].content == "Parser 789\n\nGateway"
    assert batch.child_search_projections[0].exact_terms == (
        "789",
        "gateway",
        "parser",
    )


async def test_mineru_parser_gateway_rejects_bad_context_before_fetch() -> None:
    calls: list[str] = []
    parser = MinerUTextBaselineArchiveParserGateway(
        FakeMinerUResultArchiveProvider(calls, _archive())
    )

    with pytest.raises(PermanentJobError) as raised:
        await parser.parse_document(_parse_context(stage="purge"), _source())

    assert raised.value.error_code == MINERU_GATEWAY_CONTEXT_INVALID
    assert calls == []


async def test_mineru_parser_gateway_reuses_archive_validation() -> None:
    calls: list[str] = []
    parser = MinerUTextBaselineArchiveParserGateway(
        FakeMinerUResultArchiveProvider(calls, b"not-a-zip")
    )

    with pytest.raises(PermanentJobError) as raised:
        await parser.parse_document(_parse_context(), _source())

    assert raised.value.error_code == MINERU_GATEWAY_ARCHIVE_INVALID
    assert calls == ["fetch_archive"]


@pytest.mark.parametrize(
    "entries",
    [
        (
            ("full.md", b"\xff"),
            ("fixture_content_list.json", b"[]"),
            ("layout.json", b"{}"),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b"{]"),
            ("layout.json", b"{}"),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b"[NaN]"),
            ("layout.json", b"{}"),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b"[]"),
            ("layout.json", b'{"x":1,"x":2}'),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b'{"not":"list"}'),
            ("layout.json", b"{}"),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b"[]"),
            ("layout.json", b"[]"),
            ("fixture_model.json", b"{}"),
        ),
        (
            ("full.md", b"# full\n"),
            ("fixture_content_list.json", b"[]"),
            ("layout.json", b"{}"),
            ("fixture_model.json", b'"not-an-object-or-list"'),
        ),
    ],
)
def test_mineru_gateway_rejects_invalid_decoded_artifacts(
    entries: tuple[tuple[str, bytes], ...],
) -> None:
    gateway = MinerULocalBatchGateway(SECRET)
    artifacts = gateway.extract_result_archive_artifacts(object(), _archive(entries))

    with pytest.raises(PermanentJobError) as raised:
        gateway.decode_result_archive_artifacts(object(), artifacts)

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID


def test_mineru_gateway_decode_rejects_wrong_artifact_object() -> None:
    gateway = MinerULocalBatchGateway(SECRET)

    with pytest.raises(PermanentJobError) as raised:
        gateway.decode_result_archive_artifacts(object(), object())  # type: ignore[arg-type]

    assert raised.value.error_code == MINERU_GATEWAY_ARTIFACT_INVALID
