"""Default-off MinerU local-batch submit gateway for G7.5.

This module deliberately implements only the evidence-backed local-batch
``allocate_upload`` step.  Signed upload, batch polling, result download, and
Canonical IR normalization remain separate gated slices because their public
wire contracts are still draft/blocked.  Importing this module does not register
production parse handlers or spend provider quota.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from typing import Final, NoReturn, cast
from urllib.parse import urlsplit

import httpx

from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

MINERU_GATEWAY_CREDENTIALS_MISSING: Final = "MINERU_GATEWAY_CREDENTIALS_MISSING"
MINERU_GATEWAY_SOURCE_UNSUPPORTED: Final = "MINERU_GATEWAY_SOURCE_UNSUPPORTED"
MINERU_GATEWAY_REQUEST_FAILED: Final = "MINERU_GATEWAY_REQUEST_FAILED"
MINERU_GATEWAY_STATUS_INVALID: Final = "MINERU_GATEWAY_STATUS_INVALID"
MINERU_GATEWAY_RESPONSE_INVALID: Final = "MINERU_GATEWAY_RESPONSE_INVALID"
MINERU_GATEWAY_RESPONSE_TOO_LARGE: Final = "MINERU_GATEWAY_RESPONSE_TOO_LARGE"
MINERU_GATEWAY_UPLOAD_URL_INVALID: Final = "MINERU_GATEWAY_UPLOAD_URL_INVALID"
MINERU_ALLOCATE_UPLOAD_URL: Final = "https://mineru.net/api/v4/file-urls/batch"
MINERU_MODEL_VERSION: Final = "vlm"
MINERU_PDF_CONTENT_TYPE: Final = "application/pdf"
MINERU_TIMEOUT: Final = httpx.Timeout(connect=5.0, read=30.0, write=15.0, pool=5.0)
MINERU_LIMITS: Final = httpx.Limits(max_connections=1, max_keepalive_connections=1)
MINERU_RETRY_AFTER_SECONDS: Final = 30
MAX_MINERU_API_TOKEN_BYTES: Final = 4096
MAX_MINERU_SOURCE_BYTES: Final = 200 * 1024 * 1024
MAX_MINERU_RESPONSE_BYTES: Final = 1024 * 1024
_MAX_FILENAME_BYTES: Final = 255
HTTP_OK: Final = 200
_VISIBLE_ASCII_MIN: Final = 33
_VISIBLE_ASCII_MAX: Final = 126
_SAFE_FILENAME_RE: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$")
_ID_RE: Final = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")


@dataclass(frozen=True, slots=True)
class MinerULocalBatchAllocation:
    """Transient MinerU allocate response used only by later gated steps."""

    batch_id: str
    upload_urls: tuple[str, ...]

    def __post_init__(self) -> None:
        if not _ID_RE.fullmatch(self.batch_id):
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        if not self.upload_urls:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        for upload_url in self.upload_urls:
            _validate_signed_upload_url(upload_url)


class MinerULocalBatchGateway:
    """MinerU local-batch allocate gateway for PDF parse jobs."""

    def __init__(
        self,
        api_token: str | None,
        *,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._api_token = _validate_api_token(api_token)
        self._client = client

    async def allocate_upload(
        self,
        context: object,
        source: DocumentSource,
        *,
        filename: str = "document.pdf",
    ) -> MinerULocalBatchAllocation:
        """Allocate one signed upload URL for an admitted PDF source."""
        _ = context
        _validate_pdf_source(source)
        safe_filename = _validate_filename(filename)
        body = _allocate_request_body(safe_filename)
        if self._client is not None:
            payload = await _post_allocate(self._client, self._api_token, body)
            return _allocation_from_payload(payload)
        async with httpx.AsyncClient(
            timeout=MINERU_TIMEOUT,
            limits=MINERU_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _post_allocate(client, self._api_token, body)
            return _allocation_from_payload(payload)


def _allocate_request_body(filename: str) -> JsonObject:
    return {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": filename}],
        "is_ocr": True,
        "model_version": MINERU_MODEL_VERSION,
    }


async def _post_allocate(
    client: httpx.AsyncClient,
    api_token: str,
    body: JsonObject,
) -> JsonObject:
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_token}",
        "Content-Type": "application/json",
    }
    try:
        async with client.stream(
            "POST",
            MINERU_ALLOCATE_UPLOAD_URL,
            headers=headers,
            json=body,
        ) as response:
            if response.status_code != HTTP_OK:
                _reject_retryable(MINERU_GATEWAY_STATUS_INVALID)
            return _decode_json_response(await _read_bounded_response(response))
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(MINERU_GATEWAY_REQUEST_FAILED)


async def _read_bounded_response(response: httpx.Response) -> bytes:
    if response.is_stream_consumed:
        raw_content = response.content
        if len(raw_content) > MAX_MINERU_RESPONSE_BYTES:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)
        return raw_content
    raw = bytearray()
    async for chunk in response.aiter_raw():
        if len(raw) + len(chunk) > MAX_MINERU_RESPONSE_BYTES:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)
        raw.extend(chunk)
    return bytes(raw)


def _decode_json_response(raw: bytes) -> JsonObject:
    try:
        parsed = json.loads(raw.decode("utf-8", errors="strict"))
    except (json.JSONDecodeError, UnicodeError):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    return _json_object(parsed, MINERU_GATEWAY_RESPONSE_INVALID)


def _allocation_from_payload(payload: JsonObject) -> MinerULocalBatchAllocation:
    if payload.get("code") != 0:
        _reject_retryable(MINERU_GATEWAY_STATUS_INVALID)
    data = _json_object(payload.get("data"), MINERU_GATEWAY_RESPONSE_INVALID)
    batch_id = _text(data.get("batch_id"), MINERU_GATEWAY_RESPONSE_INVALID)
    raw_urls = _json_list(data.get("file_urls"), MINERU_GATEWAY_RESPONSE_INVALID)
    upload_urls = tuple(
        _text(raw_url, MINERU_GATEWAY_UPLOAD_URL_INVALID) for raw_url in raw_urls
    )
    if len(upload_urls) != 1:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    return MinerULocalBatchAllocation(batch_id=batch_id, upload_urls=upload_urls)


def _json_object(value: object, error_code: str) -> JsonObject:
    if not isinstance(value, dict):
        _reject_permanent(error_code)
    return cast("JsonObject", value)


def _json_list(value: object, error_code: str) -> list[JsonValue]:
    if not isinstance(value, list):
        _reject_permanent(error_code)
    return cast("list[JsonValue]", value)


def _text(value: object, error_code: str) -> str:
    if not isinstance(value, str) or not value:
        _reject_permanent(error_code)
    return value


def _validate_pdf_source(source: DocumentSource) -> None:
    if not isinstance(source, DocumentSource):
        _reject_permanent(MINERU_GATEWAY_SOURCE_UNSUPPORTED)
    if source.content_type != MINERU_PDF_CONTENT_TYPE:
        _reject_permanent(MINERU_GATEWAY_SOURCE_UNSUPPORTED)
    if len(source.body) > MAX_MINERU_SOURCE_BYTES:
        _reject_permanent(MINERU_GATEWAY_SOURCE_UNSUPPORTED)


def _validate_filename(value: str) -> str:
    if (
        not isinstance(value, str)
        or not _SAFE_FILENAME_RE.fullmatch(value)
        or len(value.encode("utf-8")) > _MAX_FILENAME_BYTES
    ):
        _reject_permanent(MINERU_GATEWAY_SOURCE_UNSUPPORTED)
    return value


def _validate_signed_upload_url(value: str) -> None:
    try:
        parsed = urlsplit(value)
    except ValueError:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if parsed.password or parsed.fragment:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)


def _validate_api_token(value: str | None) -> str:
    if value is None or not isinstance(value, str):
        _reject_permanent(MINERU_GATEWAY_CREDENTIALS_MISSING)
    if (
        not value
        or value != value.strip()
        or len(value.encode("utf-8")) > MAX_MINERU_API_TOKEN_BYTES
        or any(
            ord(character) < _VISIBLE_ASCII_MIN
            or ord(character) > _VISIBLE_ASCII_MAX
            for character in value
        )
    ):
        _reject_permanent(MINERU_GATEWAY_CREDENTIALS_MISSING)
    return value


def _reject_permanent(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_retryable(error_code: str) -> NoReturn:
    raise RetryableJobError(
        stable_error_code(error_code),
        retry_after_seconds=MINERU_RETRY_AFTER_SECONDS,
    )
