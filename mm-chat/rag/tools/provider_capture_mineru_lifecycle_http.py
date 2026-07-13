"""Bounded MinerU local-upload lifecycle capture without persistent identifiers."""

from __future__ import annotations

import hashlib
from collections.abc import Callable
from dataclasses import dataclass
from typing import Final, cast

import httpx

from tools.provider_capture_common import (
    HTTP_OK,
    MINERU_BATCH_URL,
    SYNTHETIC_PDF_NAME,
    CaptureError,
    JsonObject,
    JsonValue,
    json_object,
    request_hash,
    strict_json_object,
)
from tools.provider_capture_http import send_json
from tools.provider_capture_mineru_archive import (
    MAX_ARCHIVE_BYTES,
    validate_result_archive,
)
from tools.provider_capture_mineru_shapes import (
    CONTINUE_STATES,
    empty_state_counts,
    parse_allocate_response,
    parse_poll_response,
)
from tools.provider_capture_mineru_targets import (
    poll_url,
    validate_poll_target,
    validate_result_target,
    validate_upload_target,
)

type Sleeper = Callable[[float], None]

LIFECYCLE_SCHEMA_VERSION: Final = "mm-chat.provider-capture-evidence.v2"
POLL_CALL_LIMIT: Final = 60
POLL_INTERVAL_SECONDS: Final = 5.0
_MAX_UPLOAD_RESPONSE_BYTES: Final = 65_536
_CONTENT_TYPE_PART_LIMIT: Final = 2
_REDIRECT_MIN: Final = 300
_REDIRECT_MAX: Final = 400
_ZIP_CONTENT_TYPES: Final = frozenset(
    {
        "application/octet-stream",
        "application/x-zip-compressed",
        "application/zip",
        "binary/octet-stream",
    }
)
_TRANSPORT_FAILURE_TYPES: Final = (
    (httpx.ConnectTimeout, "connect_timeout"),
    (httpx.ReadTimeout, "read_timeout"),
    (httpx.WriteTimeout, "write_timeout"),
    (httpx.PoolTimeout, "pool_timeout"),
    (httpx.ConnectError, "connect_error"),
    (httpx.ReadError, "read_error"),
    (httpx.WriteError, "write_error"),
    (httpx.CloseError, "close_error"),
    (httpx.LocalProtocolError, "local_protocol_error"),
    (httpx.RemoteProtocolError, "remote_protocol_error"),
    (httpx.ProxyError, "proxy_error"),
    (httpx.UnsupportedProtocol, "unsupported_protocol"),
)
TRANSPORT_FAILURE_CLASSES: Final = frozenset(
    failure_class for _, failure_class in _TRANSPORT_FAILURE_TYPES
) | {"other_transport_error"}


class _TransportResponseLostError(CaptureError):
    """A response-loss marker carrying only one closed, non-sensitive class."""

    def __init__(self, failure_class: str) -> None:
        super().__init__("PROVIDER_RESPONSE_LOST")
        self.failure_class = failure_class


@dataclass(slots=True)
class LifecycleFlow:
    client: httpx.Client
    api_key: str
    pdf: bytes
    operations: list[JsonValue]
    budget: JsonObject
    sleeper: Sleeper


def capture_mineru_lifecycle(
    client: httpx.Client,
    api_key: str,
    pdf: bytes,
    *,
    sleeper: Sleeper,
) -> tuple[JsonObject, JsonObject]:
    """Run one fixed Allocate/Upload/Poll/Download chain with zero retries."""
    operations = _initial_operations(pdf)
    budget = _initial_budget()
    flow = LifecycleFlow(client, api_key, pdf, operations, budget, sleeper)
    outcome, batch_id, upload_url = _allocate_stage(flow)
    if outcome is None:
        if batch_id is None or upload_url is None:
            raise CaptureError("CAPTURE_FAILED")
        outcome = _capture_after_allocate(flow, batch_id, upload_url)
    return _provider(outcome, operations, pdf), budget


def _allocate_stage(
    flow: LifecycleFlow,
) -> tuple[str | None, str | None, str | None]:
    request_body = _allocate_request()
    flow.budget["usedAllocateCalls"] = 1
    try:
        payload, metadata = send_json(
            flow.client,
            "POST",
            MINERU_BATCH_URL,
            request_body,
            flow.api_key,
        )
    except CaptureError as error:
        outcome = (
            "unknown_submission"
            if str(error) == "PROVIDER_RESPONSE_LOST"
            else "allocate_failed"
        )
        _set_state(flow.operations, "allocate_upload", outcome)
        return outcome, None, None

    try:
        batch_id, upload_url = parse_allocate_response(payload)
    except CaptureError:
        _set_state(flow.operations, "allocate_upload", "allocate_failed")
        return "allocate_failed", None, None
    flow.operations[0] = {
        "method": "POST",
        "operation": "allocate_upload",
        "path": "/api/v4/file-urls/batch",
        "requestBodySha256": request_hash(request_body),
        "response": {"batchIdPresent": True, "signedUploadUrlCount": 1},
        "state": "success",
        **metadata,
    }
    return None, batch_id, upload_url


def _capture_after_allocate(
    flow: LifecycleFlow,
    batch_id: str,
    upload_url: str,
) -> str:
    try:
        validate_upload_target(upload_url)
    except CaptureError:
        _set_state(flow.operations, "upload", "target_rejected")
        return "upload_target_rejected"
    flow.budget["usedUploadCalls"] = 1
    upload_state, upload_metadata = _upload_pdf(flow.client, upload_url, flow.pdf)
    _set_state(flow.operations, "upload", upload_state, extra=upload_metadata)
    if upload_state != "success":
        return "unknown_upload" if upload_state == "unknown" else "upload_failed"

    poll_state, result_url = _poll_stage(flow, batch_id)
    if poll_state != "done" or result_url is None:
        return poll_state
    return _download_stage(flow, result_url)


def _poll_stage(
    flow: LifecycleFlow,
    batch_id: str,
) -> tuple[str, str | None]:
    target = poll_url(batch_id)
    state_counts = empty_state_counts()
    result_url: str | None = None
    final_state = "poll_exhausted"
    for call_number in range(1, POLL_CALL_LIMIT + 1):
        if call_number > 1:
            flow.sleeper(POLL_INTERVAL_SECONDS)
        flow.budget["usedPollCalls"] = call_number
        json_object(flow.operations[2], "CAPTURE_FAILED")["usedCalls"] = call_number
        try:
            poll_payload = _get_json(flow.client, target, flow.api_key)
            poll_state, candidate_result = parse_poll_response(
                poll_payload,
                batch_id=batch_id,
            )
        except CaptureError as error:
            final_state = (
                "unknown_poll"
                if str(error) == "PROVIDER_RESPONSE_LOST"
                else "poll_failed"
            )
            if isinstance(error, _TransportResponseLostError):
                json_object(flow.operations[2], "CAPTURE_FAILED")[
                    "transportFailureClass"
                ] = error.failure_class
            break
        state_counts[poll_state] = cast("int", state_counts[poll_state]) + 1
        if poll_state in CONTINUE_STATES:
            continue
        final_state = "parse_failed" if poll_state == "failed" else "done"
        result_url = candidate_result
        break

    matched_count = sum(cast("dict[str, int]", state_counts).values())
    poll_summary: JsonObject = {
        "finalState": final_state,
        "identityMatchedResponseCount": matched_count,
        "resultUrlPresent": result_url is not None,
    }
    _set_state(
        flow.operations,
        "poll_batch",
        final_state,
        extra={"response": poll_summary, "stateCounts": state_counts},
    )
    return final_state, result_url


def _download_stage(
    flow: LifecycleFlow,
    result_url: str,
) -> str:
    try:
        validate_result_target(result_url)
    except CaptureError:
        _set_state(flow.operations, "download_result", "target_rejected")
        return "download_target_rejected"
    flow.budget["usedDownloadCalls"] = 1
    download_state, download_metadata = _download_result(flow.client, result_url)
    _set_state(
        flow.operations,
        "download_result",
        download_state,
        extra=download_metadata,
    )
    return (
        "lifecycle_complete"
        if download_state == "success"
        else ("unknown_download" if download_state == "unknown" else "download_failed")
    )


def _initial_operations(pdf: bytes) -> list[JsonValue]:
    pdf_hash = hashlib.sha256(pdf).hexdigest()
    return [
        {
            "method": "POST",
            "operation": "allocate_upload",
            "path": "/api/v4/file-urls/batch",
            "requestBodySha256": request_hash(_allocate_request()),
            "state": "not_attempted",
        },
        {
            "method": "PUT",
            "operation": "upload",
            "requestBodySha256": pdf_hash,
            "requestByteCount": len(pdf),
            "state": "not_attempted",
            "targetKind": "provider_signed_upload_url",
        },
        {
            "allowedCalls": POLL_CALL_LIMIT,
            "method": "GET",
            "operation": "poll_batch",
            "path": "/api/v4/extract-results/batch/{batch_id}",
            "state": "not_attempted",
            "stateCounts": empty_state_counts(),
            "usedCalls": 0,
        },
        {
            "allowedCalls": 1,
            "method": "GET",
            "operation": "download_result",
            "state": "not_attempted",
            "targetKind": "provider_result_url",
            "usedCalls": 0,
        },
    ]


def _initial_budget() -> JsonObject:
    return {
        "allowedAllocateCalls": 1,
        "allowedDownloadCalls": 1,
        "allowedPollCalls": POLL_CALL_LIMIT,
        "allowedUploadCalls": 1,
        "usedAllocateCalls": 0,
        "usedDownloadCalls": 0,
        "usedPollCalls": 0,
        "usedUploadCalls": 0,
    }


def _allocate_request() -> JsonObject:
    return {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": SYNTHETIC_PDF_NAME}],
        "is_ocr": True,
        "model_version": "vlm",
    }


def _upload_pdf(
    client: httpx.Client,
    url: str,
    pdf: bytes,
) -> tuple[str, JsonObject]:
    client.cookies.clear()
    headers = {"Accept-Encoding": "identity"}
    try:
        with client.stream("PUT", url, headers=headers, content=pdf) as response:
            validate_upload_target(str(response.request.url))
            _require_ok_status(response.status_code)
            _validate_identity_encoding(response)
            body = _read_bounded(response, _MAX_UPLOAD_RESPONSE_BYTES)
    except CaptureError:
        return "failed", {}
    except httpx.TransportError as error:
        return "unknown", {"transportFailureClass": _transport_failure_class(error)}
    return "success", {"httpStatus": HTTP_OK, "responseBodyByteCount": len(body)}


def _get_json(client: httpx.Client, url: str, api_key: str) -> JsonObject:
    validate_poll_target(url)
    client.cookies.clear()
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_key}",
    }
    try:
        with client.stream("GET", url, headers=headers) as response:
            validate_poll_target(str(response.request.url))
            _require_ok_status(response.status_code)
            _validate_identity_encoding(response)
            _validate_json_content_type(response.headers.get("content-type"))
            raw = _read_bounded(response, 1_048_576)
    except CaptureError:
        raise
    except httpx.TransportError as error:
        raise _TransportResponseLostError(_transport_failure_class(error)) from None
    return strict_json_object(raw)


def _download_result(
    client: httpx.Client,
    url: str,
) -> tuple[str, JsonObject]:
    client.cookies.clear()
    headers = {"Accept-Encoding": "identity"}
    try:
        with client.stream("GET", url, headers=headers) as response:
            validate_result_target(str(response.request.url))
            _require_ok_status(response.status_code)
            _validate_identity_encoding(response)
            content_type = _normalized_archive_content_type(
                response.headers.get("content-type")
            )
            archive = _read_bounded(response, MAX_ARCHIVE_BYTES)
    except CaptureError:
        return "failed", {}
    except httpx.TransportError as error:
        return "unknown", {"transportFailureClass": _transport_failure_class(error)}
    try:
        summary = validate_result_archive(archive)
    except CaptureError:
        return "failed", {}
    return "success", {
        "httpStatus": HTTP_OK,
        "responseContentType": content_type,
        "response": summary,
    }


def _require_ok_status(status_code: int) -> None:
    if _REDIRECT_MIN <= status_code < _REDIRECT_MAX:
        raise CaptureError("REDIRECT_FORBIDDEN")
    if status_code != HTTP_OK:
        raise CaptureError("PROVIDER_STATUS_INVALID")


def _read_bounded(response: httpx.Response, limit: int) -> bytes:
    length = response.headers.get("content-length")
    if length is not None:
        if not length.isascii() or not length.isdecimal():
            raise CaptureError("PROVIDER_CONTENT_LENGTH_INVALID")
        if int(length) > limit:
            raise CaptureError("PROVIDER_ARCHIVE_TOO_LARGE")
    content = bytearray()
    for chunk in response.iter_raw():
        if len(content) + len(chunk) > limit:
            raise CaptureError("PROVIDER_ARCHIVE_TOO_LARGE")
        content.extend(chunk)
    return bytes(content)


def _validate_identity_encoding(response: httpx.Response) -> None:
    encoding = response.headers.get("content-encoding")
    if encoding is not None and encoding.strip().lower() != "identity":
        raise CaptureError("PROVIDER_CONTENT_ENCODING_INVALID")


def _validate_json_content_type(value: str | None) -> None:
    if value is None:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    parts = [part.strip().lower() for part in value.split(";")]
    if (
        not parts
        or parts[0] != "application/json"
        or len(parts) > _CONTENT_TYPE_PART_LIMIT
        or (
            len(parts) == _CONTENT_TYPE_PART_LIMIT
            and parts[1] not in {"charset=utf-8", 'charset="utf-8"'}
        )
    ):
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")


def _normalized_archive_content_type(value: str | None) -> str:
    if value is None:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    normalized = value.split(";", 1)[0].strip().lower()
    if normalized not in _ZIP_CONTENT_TYPES:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    return normalized


def _transport_failure_class(error: httpx.TransportError) -> str:
    """Reduce transport failures to a closed class without retaining details."""
    for error_type, failure_class in _TRANSPORT_FAILURE_TYPES:
        if isinstance(error, error_type):
            return failure_class
    return "other_transport_error"


def _set_state(
    operations: list[JsonValue],
    operation_name: str,
    state: str,
    *,
    extra: JsonObject | None = None,
) -> None:
    for raw in operations:
        operation = json_object(raw, "CAPTURE_FAILED")
        if operation["operation"] == operation_name:
            operation["state"] = state
            if operation_name == "download_result" and state != "not_attempted":
                operation["usedCalls"] = 1 if state != "target_rejected" else 0
            if extra:
                operation.update(extra)
            return
    raise CaptureError("CAPTURE_FAILED")


def _provider(state: str, operations: list[JsonValue], pdf: bytes) -> JsonObject:
    return {
        "operationCount": 4,
        "operations": operations,
        "provider": "mineru",
        "state": state,
        "syntheticPdfByteCount": len(pdf),
        "syntheticPdfSha256": hashlib.sha256(pdf).hexdigest(),
    }


__all__ = [
    "LIFECYCLE_SCHEMA_VERSION",
    "POLL_CALL_LIMIT",
    "POLL_INTERVAL_SECONDS",
    "TRANSPORT_FAILURE_CLASSES",
    "capture_mineru_lifecycle",
]
