"""Fixed-budget HTTP capture operations and response-shape validation."""

from __future__ import annotations

import hashlib
from typing import Final
from urllib.parse import urlsplit

import httpx

from tools.provider_capture_common import (
    HTTP_OK,
    MAX_RESPONSE_BYTES,
    MINERU_BATCH_URL,
    SYNTHETIC_PDF_NAME,
    CaptureError,
    JsonObject,
    canonical_json_bytes,
    json_list,
    json_object,
    request_hash,
    strict_json_object,
    validate_request_target,
)

TIMEOUT: Final = httpx.Timeout(connect=5.0, read=15.0, write=15.0, pool=5.0)
LIMITS: Final = httpx.Limits(max_connections=1, max_keepalive_connections=1)
_HTTP_REDIRECT_MIN: Final = 300
_HTTP_REDIRECT_MAX: Final = 400
_CONTENT_TYPE_PART_LIMIT: Final = 2


def capture_mineru_submit(
    client: httpx.Client,
    api_key: str,
    pdf: bytes,
) -> JsonObject:
    """Submit once, never retry, upload, or poll."""
    request_body: JsonObject = {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": SYNTHETIC_PDF_NAME}],
        "is_ocr": True,
        "model_version": "vlm",
    }
    body_hash = request_hash(request_body)
    try:
        response, metadata = send_json(
            client,
            "POST",
            MINERU_BATCH_URL,
            request_body,
            api_key,
        )
    except CaptureError as error:
        if str(error) != "PROVIDER_RESPONSE_LOST":
            raise
        return _unknown_mineru_submission(body_hash)
    shape = _validate_mineru_submit_response(response)
    operation: JsonObject = {
        "method": "POST",
        "operation": "local_upload_batch_submit",
        "path": "/api/v4/file-urls/batch",
        "requestBodySha256": body_hash,
        "response": shape,
        "state": "staged_after_submit",
        **metadata,
    }
    return {
        "operationCount": 1,
        "operations": [operation],
        "provider": "mineru",
        "state": "staged_after_submit",
        "syntheticPdfByteCount": len(pdf),
        "syntheticPdfSha256": hashlib.sha256(pdf).hexdigest(),
    }


def send_json(
    client: httpx.Client,
    method: str,
    url: str,
    body: JsonObject,
    api_key: str,
) -> tuple[JsonObject, JsonObject]:
    """Send one strict JSON request and consume bounded raw response bytes."""
    validate_request_target(method, url)
    content = canonical_json_bytes(body).removesuffix(b"\n")
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    client.cookies.clear()
    try:
        with client.stream(method, url, headers=headers, content=content) as response:
            validate_request_target(response.request.method, str(response.request.url))
            _validate_response_headers(response)
            raw = _read_bounded_raw_response(response)
    except CaptureError:
        raise
    except Exception:  # noqa: BLE001 - never retain transport implementation detail.
        raise CaptureError("PROVIDER_RESPONSE_LOST") from None
    return strict_json_object(raw), _response_metadata()


def _unknown_mineru_submission(body_hash: str) -> JsonObject:
    return {
        "operationCount": 1,
        "operations": [
            {
                "method": "POST",
                "operation": "local_upload_batch_submit",
                "path": "/api/v4/file-urls/batch",
                "requestBodySha256": body_hash,
                "state": "unknown_submission",
            }
        ],
        "provider": "mineru",
        "state": "unknown_submission",
    }


def _validate_response_headers(response: httpx.Response) -> None:
    status_code = response.status_code
    if _HTTP_REDIRECT_MIN <= status_code < _HTTP_REDIRECT_MAX:
        raise CaptureError("REDIRECT_FORBIDDEN")
    if status_code != HTTP_OK:
        raise CaptureError("PROVIDER_STATUS_INVALID")
    _normalized_json_content_type(response.headers.get("content-type"))
    encoding = response.headers.get("content-encoding")
    if encoding is not None and encoding.strip().lower() != "identity":
        raise CaptureError("PROVIDER_CONTENT_ENCODING_INVALID")
    length = response.headers.get("content-length")
    if length is not None:
        _validate_content_length(length)


def _validate_content_length(value: str) -> None:
    if not value.isascii() or not value.isdecimal():
        raise CaptureError("PROVIDER_CONTENT_LENGTH_INVALID")
    if int(value) > MAX_RESPONSE_BYTES:
        raise CaptureError("PROVIDER_RESPONSE_TOO_LARGE")


def _read_bounded_raw_response(response: httpx.Response) -> bytes:
    raw = bytearray()
    for chunk in response.iter_raw():
        if len(raw) + len(chunk) > MAX_RESPONSE_BYTES:
            raise CaptureError("PROVIDER_RESPONSE_TOO_LARGE")
        raw.extend(chunk)
    return bytes(raw)


def _normalized_json_content_type(value: str | None) -> str:
    if value is None:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    parts = [part.strip().lower() for part in value.split(";")]
    if parts[0] != "application/json" or len(parts) > _CONTENT_TYPE_PART_LIMIT:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    if len(parts) == _CONTENT_TYPE_PART_LIMIT and parts[1] not in {
        "charset=utf-8",
        'charset="utf-8"',
    }:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    return "application/json"


def _response_metadata() -> JsonObject:
    return {
        "httpStatus": HTTP_OK,
        "responseContentType": "application/json",
        "responseHeaderNames": ["content-type"],
    }


def _validate_mineru_submit_response(payload: JsonObject) -> JsonObject:
    if payload.get("code") != 0:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
    data = json_object(payload.get("data"), "MINERU_SUBMIT_SHAPE_INVALID")
    batch_id = data.get("batch_id")
    file_urls = json_list(data.get("file_urls"), "MINERU_SUBMIT_SHAPE_INVALID")
    if not isinstance(batch_id, str) or not batch_id or len(file_urls) != 1:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
    for raw_url in file_urls:
        if not isinstance(raw_url, str):
            raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
        _validate_signed_upload_url(raw_url)
    return {"batchIdPresent": True, "signedUploadUrlCount": len(file_urls)}


def _validate_signed_upload_url(url: str) -> None:
    try:
        parsed = urlsplit(url)
        _ = parsed.port
    except ValueError:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID") from None
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.fragment
    ):
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
