"""Deterministic no-network MinerU lifecycle fixtures shared by unit tests."""

from __future__ import annotations

import io
import json
import zipfile
from collections.abc import Callable, Iterator
from datetime import UTC, datetime
from typing import Any, cast

import httpx
from tools.provider_capture_mineru_lifecycle import LifecycleRuntime, capture_lifecycle

OBSERVED_AT = datetime(2026, 7, 13, 6, 7, 8, tzinfo=UTC)
KEY = "unit-test-mineru-lifecycle-credential"
UPLOAD_URL = (
    "https://mineru.oss-cn-shanghai.aliyuncs.com/"
    "api-upload/object.pdf?OSSAccessKeyId=fixture&Expires=1&Signature=fixture"
)
RESULT_URL = "https://cdn-mineru.openxlab.org.cn/pdf/fixture-result.zip"


def response(
    content: bytes,
    *,
    content_type: str | None = None,
    status: int = 200,
    location: str | None = None,
) -> httpx.Response:
    class StaticStream(httpx.SyncByteStream):
        def __iter__(self) -> Iterator[bytes]:
            yield content

    response_headers: dict[str, str] = {}
    if content_type is not None:
        response_headers["Content-Type"] = content_type
    if location is not None:
        response_headers["Location"] = location
    return httpx.Response(status, headers=response_headers, stream=StaticStream())


def json_response(payload: object, *, status: int = 200) -> httpx.Response:
    return response(
        json.dumps(payload, separators=(",", ":")).encode(),
        content_type="application/json; charset=utf-8",
        status=status,
    )


def archive_bytes(entries: dict[str, bytes] | None = None) -> bytes:
    contents = entries or {
        "full.md": b"synthetic markdown",
        "fixture_content_list.json": b"[]",
        "fixture_middle.json": b"{}",
        "fixture_model.json": b"{}",
    }
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        for name, content in contents.items():
            entry = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
            entry.compress_type = zipfile.ZIP_STORED
            archive.writestr(entry, content)
    return output.getvalue()


def allocate_payload(upload_url: str = UPLOAD_URL) -> dict[str, Any]:
    return {
        "code": 0,
        "data": {
            "batch_id": "sensitive-lifecycle-batch-id",
            "file_urls": [upload_url],
        },
        "msg": "ok",
        "trace_id": "sensitive-lifecycle-trace-id",
    }


def poll_payload(
    state: str,
    *,
    result_url: str = RESULT_URL,
    error: str = "",
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "err_msg": error,
        "file_name": "mm-chat-synthetic-capture.pdf",
        "state": state,
    }
    if state == "running":
        result["extract_progress"] = {
            "extracted_pages": 1,
            "start_time": "2026-07-13 06:07:08",
            "total_pages": 2,
        }
    if state == "done":
        result["full_zip_url"] = result_url
    return {
        "code": 0,
        "data": {
            "batch_id": "sensitive-lifecycle-batch-id",
            "extract_result": [result],
        },
        "msg": "ok",
        "trace_id": "sensitive-lifecycle-poll-trace-id",
    }


def lifecycle_transport(
    requests: list[httpx.Request] | None = None,
    *,
    poll_states: list[str] | None = None,
    result_entries: dict[str, bytes] | None = None,
    upload_url: str = UPLOAD_URL,
    result_url: str = RESULT_URL,
) -> httpx.MockTransport:
    states = iter(poll_states or ["running", "done"])

    def handler(request: httpx.Request) -> httpx.Response:
        if requests is not None:
            requests.append(request)
        if request.method == "POST":
            return json_response(allocate_payload(upload_url))
        if request.method == "PUT":
            return response(b"")
        if request.url.host == "mineru.net":
            state = next(states)
            return json_response(
                poll_payload(
                    state,
                    result_url=result_url,
                    error="sensitive parse failure" if state == "failed" else "",
                )
            )
        if request.url.host == "cdn-mineru.openxlab.org.cn":
            return response(
                archive_bytes(result_entries),
                content_type="application/zip",
            )
        raise AssertionError("unexpected lifecycle request")

    return httpx.MockTransport(handler)


def capture_snapshot(
    transport: httpx.BaseTransport,
    *,
    sleeper: Callable[[float], None] | None = None,
) -> dict[str, Any]:
    return cast(
        "dict[str, Any]",
        capture_lifecycle(
            observed_at=OBSERVED_AT,
            runtime=LifecycleRuntime(
                environ={"MINERU_API_KEY": KEY},
                transport=transport,
                sleeper=sleeper or (lambda _: None),
            ),
        ),
    )
