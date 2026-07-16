from __future__ import annotations

import hashlib
import io
import json
import stat
import zipfile
from typing import Any

import httpx
import pytest

import mm_chat_rag.mineru_gateway as mineru_module
from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.mineru_gateway import (
    MINERU_ALLOCATE_UPLOAD_URL,
    MINERU_GATEWAY_ARCHIVE_INVALID,
    MINERU_GATEWAY_ARTIFACT_INVALID,
    MINERU_GATEWAY_CREDENTIALS_MISSING,
    MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID,
    MINERU_GATEWAY_REQUEST_FAILED,
    MINERU_GATEWAY_RESPONSE_INVALID,
    MINERU_GATEWAY_RESPONSE_TOO_LARGE,
    MINERU_GATEWAY_RESULT_URL_INVALID,
    MINERU_GATEWAY_SOURCE_UNSUPPORTED,
    MINERU_GATEWAY_STATUS_INVALID,
    MINERU_GATEWAY_UPLOAD_STATUS_INVALID,
    MINERU_GATEWAY_UPLOAD_URL_INVALID,
    MINERU_POLL_URL_PREFIX,
    MinerULocalBatchAllocation,
    MinerULocalBatchGateway,
    MinerULocalBatchPollResult,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

SECRET = "unit-test-mineru-token"
PDF_BODY = b"%PDF-1.7\nfixture\n%%EOF\n"
ZIP_BODY = b"PK\x03\x04fixture-zip-bytes"
SIGNED_UPLOAD_URL = (
    "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/fixture.pdf"
    "?Expires=1&Signature=redacted"
)
RESULT_URL = "https://cdn-mineru.openxlab.org.cn/pdf/fixture-result.zip"
ARCHIVE_ENTRIES = (
    ("full.md", b"# full\n"),
    ("fixture_content_list.json", b"[]"),
    ("layout.json", b"{}"),
    ("fixture_model.json", b"{}"),
)


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
            ("fixture_model.json", b"[]"),
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
