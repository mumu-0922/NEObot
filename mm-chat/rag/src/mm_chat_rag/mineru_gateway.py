"""Default-off MinerU local-batch gateway seams for G7.5.

This module deliberately implements only the evidence-backed local-batch
``allocate_upload`` step plus derived signed-upload and poll/result transport
seams plus a bounded result ZIP download transport seam. Result archive
validation/extraction is structural only; the canonical-mapping input seam binds
raw role hashes to decoded payloads but still does not normalize Provider fields
or emit Canonical IR. Importing this module does not register production parse
handlers or spend provider quota.
"""

from __future__ import annotations

import hashlib
import io
import json
import math
import re
import stat
import zipfile
from collections.abc import Sequence
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final, NoReturn, cast
from urllib.parse import SplitResult, urlsplit

import httpx

from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

MINERU_GATEWAY_CREDENTIALS_MISSING: Final = "MINERU_GATEWAY_CREDENTIALS_MISSING"
MINERU_GATEWAY_SOURCE_UNSUPPORTED: Final = "MINERU_GATEWAY_SOURCE_UNSUPPORTED"
MINERU_GATEWAY_SOURCE_HASH_MISMATCH: Final = (
    "MINERU_GATEWAY_SOURCE_HASH_MISMATCH"
)
MINERU_GATEWAY_REQUEST_FAILED: Final = "MINERU_GATEWAY_REQUEST_FAILED"
MINERU_GATEWAY_STATUS_INVALID: Final = "MINERU_GATEWAY_STATUS_INVALID"
MINERU_GATEWAY_RESPONSE_INVALID: Final = "MINERU_GATEWAY_RESPONSE_INVALID"
MINERU_GATEWAY_RESPONSE_TOO_LARGE: Final = "MINERU_GATEWAY_RESPONSE_TOO_LARGE"
MINERU_GATEWAY_UPLOAD_URL_INVALID: Final = "MINERU_GATEWAY_UPLOAD_URL_INVALID"
MINERU_GATEWAY_UPLOAD_STATUS_INVALID: Final = (
    "MINERU_GATEWAY_UPLOAD_STATUS_INVALID"
)
MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID: Final = (
    "MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID"
)
MINERU_GATEWAY_RESULT_URL_INVALID: Final = "MINERU_GATEWAY_RESULT_URL_INVALID"
MINERU_GATEWAY_ARCHIVE_INVALID: Final = "MINERU_GATEWAY_ARCHIVE_INVALID"
MINERU_GATEWAY_ARTIFACT_INVALID: Final = "MINERU_GATEWAY_ARTIFACT_INVALID"
MINERU_ALLOCATE_UPLOAD_URL: Final = "https://mineru.net/api/v4/file-urls/batch"
MINERU_POLL_PATH_PREFIX: Final = "/api/v4/extract-results/batch/"
MINERU_POLL_URL_PREFIX: Final = f"https://mineru.net{MINERU_POLL_PATH_PREFIX}"
MINERU_MODEL_VERSION: Final = "vlm"
MINERU_PDF_CONTENT_TYPE: Final = "application/pdf"
MINERU_TIMEOUT: Final = httpx.Timeout(connect=5.0, read=30.0, write=15.0, pool=5.0)
MINERU_LIMITS: Final = httpx.Limits(max_connections=1, max_keepalive_connections=1)
MINERU_RETRY_AFTER_SECONDS: Final = 30
MAX_MINERU_API_TOKEN_BYTES: Final = 4096
MAX_MINERU_SOURCE_BYTES: Final = 200 * 1024 * 1024
MAX_MINERU_RESPONSE_BYTES: Final = 1024 * 1024
MAX_MINERU_UPLOAD_URL_BYTES: Final = 4096
MAX_MINERU_RESULT_ARCHIVE_BYTES: Final = 32 * 1024 * 1024
MAX_MINERU_ARCHIVE_ENTRIES: Final = 256
MAX_MINERU_ARCHIVE_ENTRY_BYTES: Final = 64 * 1024 * 1024
MAX_MINERU_ARCHIVE_TOTAL_BYTES: Final = 128 * 1024 * 1024
MAX_MINERU_ARCHIVE_COMPRESSION_RATIO: Final = 200
MINERU_UPLOAD_TARGET_HOST: Final = "mineru.oss-cn-shanghai.aliyuncs.com"
MINERU_UPLOAD_PATH_PREFIX: Final = "/api-upload/"
MINERU_RESULT_TARGET_HOST: Final = "cdn-mineru.openxlab.org.cn"
MINERU_RESULT_PATH_PREFIX: Final = "/pdf/"
MINERU_RESULT_PATH_SUFFIX: Final = ".zip"
_MAX_FILENAME_BYTES: Final = 255
HTTP_OK: Final = 200
HTTP_NO_CONTENT: Final = 204
_ASCII_CONTROL_CEILING: Final = 32
_ASCII_DELETE: Final = 127
_VISIBLE_ASCII_MIN: Final = 33
_VISIBLE_ASCII_MAX: Final = 126
_SAFE_FILENAME_RE: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$")
_ID_RE: Final = re.compile(r"^[A-Za-z0-9._-]{1,128}$")
_DATA_ID_RE: Final = re.compile(r"^[A-Za-z0-9._-]{1,128}$")
_START_TIME_RE: Final = re.compile(r"^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$")
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_MINERU_CANONICAL_ROLE_ORDER: Final = (
    "full_markdown",
    "content_list_json",
    "middle_json",
    "model_json",
)
_MINERU_ARTIFACT_ROLES: Final[frozenset[str]] = frozenset(
    _MINERU_CANONICAL_ROLE_ORDER
)
_POLL_STATES: Final[frozenset[str]] = frozenset(
    {"waiting-file", "pending", "running", "converting", "done", "failed"}
)
_ZIP_CONTENT_TYPES: Final[frozenset[str]] = frozenset(
    {
        "application/octet-stream",
        "application/x-zip-compressed",
        "application/zip",
        "binary/octet-stream",
    }
)
_ZIP_ENCRYPTED_FLAG: Final = 0x1
_ZIP_MODE_SHIFT: Final = 16


@dataclass(frozen=True, slots=True)
class MinerULocalBatchAllocation:
    """Transient MinerU allocate response used only by later gated steps."""

    batch_id: str
    upload_urls: tuple[str, ...]
    filename: str = "document.pdf"

    def __post_init__(self) -> None:
        if not _ID_RE.fullmatch(self.batch_id):
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        _validate_filename(self.filename)
        if not self.upload_urls:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        for upload_url in self.upload_urls:
            _validate_signed_upload_url(upload_url)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchPollResult:
    """One redacted MinerU poll outcome; dynamic result URL remains transient."""

    batch_id: str
    filename: str
    state: str
    result_url: str | None = None

    def __post_init__(self) -> None:
        if not _ID_RE.fullmatch(self.batch_id):
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        _validate_filename(self.filename)
        if self.state not in _POLL_STATES:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
        if self.state == "done":
            if self.result_url is None:
                _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
            _validate_result_target_url(self.result_url)
        elif self.result_url is not None:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchArchiveSummary:
    """Redacted validated ZIP summary; entry names and content are not retained."""

    archive_byte_count: int
    archive_sha256: str
    entry_count: int
    full_markdown_present: bool
    content_list_present: bool
    middle_json_present: bool
    model_json_present: bool

    def __post_init__(self) -> None:
        if (
            isinstance(self.archive_byte_count, bool)
            or not isinstance(self.archive_byte_count, int)
            or self.archive_byte_count < 1
            or self.archive_byte_count > MAX_MINERU_RESULT_ARCHIVE_BYTES
            or not _SHA256_RE.fullmatch(self.archive_sha256)
            or isinstance(self.entry_count, bool)
            or not isinstance(self.entry_count, int)
            or not 1 <= self.entry_count <= MAX_MINERU_ARCHIVE_ENTRIES
            or self.full_markdown_present is not True
            or self.content_list_present is not True
            or self.middle_json_present is not True
            or self.model_json_present is not True
        ):
            _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchArchiveArtifacts:
    """Validated MinerU role bytes without retained ZIP entry names."""

    summary: MinerULocalBatchArchiveSummary
    full_markdown: bytes
    content_list_json: bytes
    middle_json: bytes
    model_json: bytes

    def __post_init__(self) -> None:
        if not isinstance(self.summary, MinerULocalBatchArchiveSummary):
            _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
        for content in (
            self.full_markdown,
            self.content_list_json,
            self.middle_json,
            self.model_json,
        ):
            if not isinstance(content, bytes) or not content:
                _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchDecodedArtifacts:
    """Decoded MinerU payloads admitted for a later Canonical IR mapper."""

    summary: MinerULocalBatchArchiveSummary
    full_markdown: str
    content_list_json: list[JsonValue]
    middle_json: JsonObject
    model_json: JsonObject

    def __post_init__(self) -> None:
        if not isinstance(self.summary, MinerULocalBatchArchiveSummary):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
        if not isinstance(self.full_markdown, str) or not self.full_markdown:
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
        if not isinstance(self.content_list_json, list):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
        if not isinstance(self.middle_json, dict) or not isinstance(
            self.model_json,
            dict,
        ):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchArtifactDigest:
    """Hash-bound metadata for one required MinerU semantic role payload."""

    role: str
    byte_count: int
    sha256: str

    def __post_init__(self) -> None:
        if (
            self.role not in _MINERU_ARTIFACT_ROLES
            or isinstance(self.byte_count, bool)
            or not isinstance(self.byte_count, int)
            or self.byte_count < 1
            or self.byte_count > MAX_MINERU_ARCHIVE_ENTRY_BYTES
            or not _SHA256_RE.fullmatch(self.sha256)
        ):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


@dataclass(frozen=True, slots=True)
class MinerULocalBatchCanonicalMappingInput:
    """Hash-bound MinerU decoded payload bundle for a future IR mapper."""

    source_sha256: str
    source_byte_count: int
    source_content_type: str
    archive_sha256: str
    archive_byte_count: int
    role_digests: tuple[MinerULocalBatchArtifactDigest, ...]
    decoded: MinerULocalBatchDecodedArtifacts

    def __post_init__(self) -> None:
        if (
            not _SHA256_RE.fullmatch(self.source_sha256)
            or isinstance(self.source_byte_count, bool)
            or not isinstance(self.source_byte_count, int)
            or self.source_byte_count < 1
            or self.source_byte_count > MAX_MINERU_SOURCE_BYTES
            or self.source_content_type != MINERU_PDF_CONTENT_TYPE
            or not _SHA256_RE.fullmatch(self.archive_sha256)
            or isinstance(self.archive_byte_count, bool)
            or not isinstance(self.archive_byte_count, int)
            or self.archive_byte_count < 1
            or self.archive_byte_count > MAX_MINERU_RESULT_ARCHIVE_BYTES
            or not isinstance(self.decoded, MinerULocalBatchDecodedArtifacts)
            or self.archive_sha256 != self.decoded.summary.archive_sha256
            or self.archive_byte_count != self.decoded.summary.archive_byte_count
            or not isinstance(self.role_digests, tuple)
            or not all(
                isinstance(item, MinerULocalBatchArtifactDigest)
                for item in self.role_digests
            )
            or tuple(item.role for item in self.role_digests)
            != _MINERU_CANONICAL_ROLE_ORDER
        ):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


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
            return _allocation_from_payload(payload, filename=safe_filename)
        async with httpx.AsyncClient(
            timeout=MINERU_TIMEOUT,
            limits=MINERU_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _post_allocate(client, self._api_token, body)
            return _allocation_from_payload(payload, filename=safe_filename)

    async def upload_document(
        self,
        context: object,
        source: DocumentSource,
        allocation: MinerULocalBatchAllocation,
    ) -> None:
        """Upload one admitted PDF body to the provider-signed URL."""
        _ = context
        _validate_pdf_source(source)
        upload_url = _single_upload_url(allocation)
        if self._client is not None:
            await _put_signed_upload(self._client, upload_url, source.body)
            return
        async with httpx.AsyncClient(
            timeout=MINERU_TIMEOUT,
            limits=MINERU_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            await _put_signed_upload(client, upload_url, source.body)

    async def poll_batch_result(
        self,
        context: object,
        allocation: MinerULocalBatchAllocation,
    ) -> MinerULocalBatchPollResult:
        """Fetch one redacted MinerU batch poll state without downloading ZIPs."""
        _ = context
        poll_url = _poll_url(allocation)
        if self._client is not None:
            payload = await _get_poll_result(self._client, self._api_token, poll_url)
            return _poll_result_from_payload(payload, allocation=allocation)
        async with httpx.AsyncClient(
            timeout=MINERU_TIMEOUT,
            limits=MINERU_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            payload = await _get_poll_result(client, self._api_token, poll_url)
            return _poll_result_from_payload(payload, allocation=allocation)

    async def download_result_archive(
        self,
        context: object,
        poll_result: MinerULocalBatchPollResult,
    ) -> bytes:
        """Download one bounded result ZIP body without parsing archive entries."""
        _ = context
        result_url = _done_result_url(poll_result)
        if self._client is not None:
            return await _get_result_archive(self._client, result_url)
        async with httpx.AsyncClient(
            timeout=MINERU_TIMEOUT,
            limits=MINERU_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            return await _get_result_archive(client, result_url)

    def validate_result_archive(
        self,
        context: object,
        archive_body: bytes,
    ) -> MinerULocalBatchArchiveSummary:
        """Validate one MinerU result ZIP without retaining entry names/content."""
        _ = context
        return _validate_result_archive_body(archive_body)

    def extract_result_archive_artifacts(
        self,
        context: object,
        archive_body: bytes,
    ) -> MinerULocalBatchArchiveArtifacts:
        """Extract validated MinerU role bytes without normalizing content."""
        _ = context
        return _extract_result_archive_artifacts(archive_body)

    def decode_result_archive_artifacts(
        self,
        context: object,
        artifacts: MinerULocalBatchArchiveArtifacts,
    ) -> MinerULocalBatchDecodedArtifacts:
        """Decode MinerU role payloads without Canonical IR normalization."""
        _ = context
        return _decode_result_archive_artifacts(artifacts)

    def prepare_canonical_mapping_input(
        self,
        context: object,
        source: DocumentSource,
        artifacts: MinerULocalBatchArchiveArtifacts,
    ) -> MinerULocalBatchCanonicalMappingInput:
        """Bind source and role digests to decoded payloads for later mapping."""
        _ = context
        return _prepare_canonical_mapping_input(source, artifacts)


def _allocate_request_body(filename: str) -> JsonObject:
    return {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": filename}],
        "is_ocr": True,
        "model_version": MINERU_MODEL_VERSION,
    }


async def _put_signed_upload(
    client: httpx.AsyncClient,
    upload_url: str,
    body: bytes,
) -> None:
    try:
        _clear_client_cookies(client)
        async with client.stream("PUT", upload_url, content=body) as response:
            if response.status_code not in {HTTP_OK, HTTP_NO_CONTENT}:
                _reject_retryable(MINERU_GATEWAY_UPLOAD_STATUS_INVALID)
            await _read_bounded_response(response)
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(MINERU_GATEWAY_REQUEST_FAILED)


async def _get_result_archive(
    client: httpx.AsyncClient,
    result_url: str,
) -> bytes:
    headers = {
        "Accept": "application/zip",
        "Accept-Encoding": "identity",
    }
    try:
        _clear_client_cookies(client)
        async with client.stream(
            "GET",
            result_url,
            headers=headers,
        ) as response:
            _validate_result_target_url(str(response.request.url))
            if response.status_code != HTTP_OK:
                _reject_retryable(MINERU_GATEWAY_DOWNLOAD_STATUS_INVALID)
            _validate_identity_encoding(response)
            _validate_archive_content_type(response.headers.get("content-type"))
            return await _read_bounded_archive_response(response)
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(MINERU_GATEWAY_REQUEST_FAILED)


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


async def _get_poll_result(
    client: httpx.AsyncClient,
    api_token: str,
    poll_url: str,
) -> JsonObject:
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_token}",
    }
    try:
        async with client.stream(
            "GET",
            poll_url,
            headers=headers,
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


def _allocation_from_payload(
    payload: JsonObject,
    *,
    filename: str,
) -> MinerULocalBatchAllocation:
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
    return MinerULocalBatchAllocation(
        batch_id=batch_id,
        upload_urls=upload_urls,
        filename=filename,
    )


def _poll_result_from_payload(
    payload: JsonObject,
    *,
    allocation: MinerULocalBatchAllocation,
) -> MinerULocalBatchPollResult:
    if payload.get("code") != 0:
        _reject_retryable(MINERU_GATEWAY_STATUS_INVALID)
    _closed_fields(payload, {"code", "data", "msg"}, {"trace_id"})
    if (
        not isinstance(payload["msg"], str)
        or not payload["msg"]
        or ("trace_id" in payload and not isinstance(payload["trace_id"], str))
    ):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    data = _json_object(payload["data"], MINERU_GATEWAY_RESPONSE_INVALID)
    _closed_fields(data, {"batch_id", "extract_result"}, set())
    results = _json_list(data["extract_result"], MINERU_GATEWAY_RESPONSE_INVALID)
    if data["batch_id"] != allocation.batch_id or len(results) != 1:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    result = _json_object(results[0], MINERU_GATEWAY_RESPONSE_INVALID)
    _closed_fields(
        result,
        {"err_msg", "file_name", "state"},
        {"data_id", "extract_progress", "full_zip_url"},
    )
    state = _text(result["state"], MINERU_GATEWAY_RESPONSE_INVALID)
    if (
        state not in _POLL_STATES
        or result["file_name"] != allocation.filename
        or not isinstance(result["err_msg"], str)
        or not _valid_optional_data_id(result)
    ):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    if state == "running":
        _validate_progress(result.get("extract_progress"))
    elif "extract_progress" in result:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    if state == "done":
        result_url = _text(
            result.get("full_zip_url"),
            MINERU_GATEWAY_RESULT_URL_INVALID,
        )
        _validate_result_target_url(result_url)
        return MinerULocalBatchPollResult(
            batch_id=allocation.batch_id,
            filename=allocation.filename,
            state=state,
            result_url=result_url,
        )
    if "full_zip_url" in result or (state == "failed" and not result["err_msg"]):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    return MinerULocalBatchPollResult(
        batch_id=allocation.batch_id,
        filename=allocation.filename,
        state=state,
    )


def _json_object(value: object, error_code: str) -> JsonObject:
    if not isinstance(value, dict):
        _reject_permanent(error_code)
    return cast("JsonObject", value)


def _json_list(value: object, error_code: str) -> list[JsonValue]:
    if not isinstance(value, list):
        _reject_permanent(error_code)
    return cast("list[JsonValue]", value)


def _closed_fields(
    value: JsonObject,
    required: set[str],
    optional: set[str],
) -> None:
    fields = set(value)
    if not required <= fields or not fields <= required | optional:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)


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


def _validate_source_hash(source: DocumentSource) -> None:
    if hashlib.sha256(source.body).hexdigest() != source.source_sha256:
        _reject_permanent(MINERU_GATEWAY_SOURCE_HASH_MISMATCH)


def _validate_filename(value: str) -> str:
    if (
        not isinstance(value, str)
        or not _SAFE_FILENAME_RE.fullmatch(value)
        or len(value.encode("utf-8")) > _MAX_FILENAME_BYTES
    ):
        _reject_permanent(MINERU_GATEWAY_SOURCE_UNSUPPORTED)
    return value


def _single_upload_url(allocation: MinerULocalBatchAllocation) -> str:
    if not isinstance(allocation, MinerULocalBatchAllocation):
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if len(allocation.upload_urls) != 1:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    upload_url = allocation.upload_urls[0]
    _validate_signed_upload_url(upload_url)
    _validate_upload_target_url(upload_url)
    return upload_url


def _done_result_url(poll_result: MinerULocalBatchPollResult) -> str:
    if not isinstance(poll_result, MinerULocalBatchPollResult):
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if poll_result.state != "done" or poll_result.result_url is None:
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    _validate_result_target_url(poll_result.result_url)
    return poll_result.result_url


def _poll_url(allocation: MinerULocalBatchAllocation) -> str:
    if not isinstance(allocation, MinerULocalBatchAllocation):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    if not _ID_RE.fullmatch(allocation.batch_id):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    return f"{MINERU_POLL_URL_PREFIX}{allocation.batch_id}"


def _validate_signed_upload_url(value: str) -> None:
    parsed = _parse_url(value)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if parsed.password or parsed.fragment:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)


def _validate_upload_target_url(value: str) -> None:
    parsed = _parse_url(value)
    if len(value.encode("utf-8")) > MAX_MINERU_UPLOAD_URL_BYTES:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if parsed.hostname != MINERU_UPLOAD_TARGET_HOST:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if _url_port(parsed, error_code=MINERU_GATEWAY_UPLOAD_URL_INVALID) not in {
        None,
        443,
    }:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if not parsed.path.startswith(MINERU_UPLOAD_PATH_PREFIX):
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if not _safe_dynamic_path(parsed.path):
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if not parsed.query:
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)
    if not _is_visible_url(value):
        _reject_permanent(MINERU_GATEWAY_UPLOAD_URL_INVALID)


def _validate_result_target_url(value: str) -> None:
    parsed = _parse_url(value, error_code=MINERU_GATEWAY_RESULT_URL_INVALID)
    if len(value.encode("utf-8")) > MAX_MINERU_UPLOAD_URL_BYTES:
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if parsed.hostname != MINERU_RESULT_TARGET_HOST:
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if _url_port(parsed, error_code=MINERU_GATEWAY_RESULT_URL_INVALID) not in {
        None,
        443,
    }:
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if not parsed.path.startswith(MINERU_RESULT_PATH_PREFIX):
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if not parsed.path.endswith(MINERU_RESULT_PATH_SUFFIX):
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if not _safe_dynamic_path(parsed.path):
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)
    if parsed.query or not _is_visible_url(value):
        _reject_permanent(MINERU_GATEWAY_RESULT_URL_INVALID)


def _parse_url(
    value: str,
    *,
    error_code: str = MINERU_GATEWAY_UPLOAD_URL_INVALID,
) -> SplitResult:
    if not isinstance(value, str) or not value:
        _reject_permanent(error_code)
    if not _is_visible_url(value):
        _reject_permanent(error_code)
    try:
        return urlsplit(value)
    except ValueError:
        _reject_permanent(error_code)


def _url_port(
    parsed: SplitResult,
    *,
    error_code: str,
) -> int | None:
    try:
        return parsed.port
    except ValueError:
        _reject_permanent(error_code)


def _valid_optional_data_id(result: JsonObject) -> bool:
    if "data_id" not in result:
        return True
    value = result["data_id"]
    return isinstance(value, str) and _DATA_ID_RE.fullmatch(value) is not None


def _validate_progress(value: object) -> None:
    progress = _json_object(value, MINERU_GATEWAY_RESPONSE_INVALID)
    _closed_fields(
        progress,
        {"extracted_pages", "start_time", "total_pages"},
        set(),
    )
    extracted = progress["extracted_pages"]
    total = progress["total_pages"]
    if (
        not _is_nonnegative_int(extracted)
        or not _is_nonnegative_int(total)
        or cast("int", total) == 0
        or cast("int", extracted) > cast("int", total)
        or not isinstance(progress["start_time"], str)
        or _START_TIME_RE.fullmatch(progress["start_time"]) is None
    ):
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)


def _is_nonnegative_int(value: object) -> bool:
    return not isinstance(value, bool) and isinstance(value, int) and value >= 0


def _validate_result_archive_body(content: bytes) -> MinerULocalBatchArchiveSummary:
    if not isinstance(content, bytes) or not content:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if len(content) > MAX_MINERU_RESULT_ARCHIVE_BYTES:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)
    entries = _read_valid_archive_entries(content)
    required = _required_archive_artifacts(entries)
    _validate_required_archive_artifacts(required)
    return MinerULocalBatchArchiveSummary(
        archive_byte_count=len(content),
        archive_sha256=hashlib.sha256(content).hexdigest(),
        entry_count=len(entries),
        full_markdown_present=required["full_markdown_present"],
        content_list_present=required["content_list_present"],
        middle_json_present=required["middle_json_present"],
        model_json_present=required["model_json_present"],
    )


def _extract_result_archive_artifacts(
    content: bytes,
) -> MinerULocalBatchArchiveArtifacts:
    summary = _validate_result_archive_body(content)
    try:
        with zipfile.ZipFile(io.BytesIO(content)) as archive:
            role_entries = _required_archive_role_entries(archive.infolist())
            return MinerULocalBatchArchiveArtifacts(
                summary=summary,
                full_markdown=archive.read(role_entries["full_markdown"]),
                content_list_json=archive.read(role_entries["content_list_json"]),
                middle_json=archive.read(role_entries["middle_json"]),
                model_json=archive.read(role_entries["model_json"]),
            )
    except PermanentJobError:
        raise
    except NotImplementedError:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    except (OSError, RuntimeError, zipfile.BadZipFile, zipfile.LargeZipFile):
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


def _decode_result_archive_artifacts(
    artifacts: MinerULocalBatchArchiveArtifacts,
) -> MinerULocalBatchDecodedArtifacts:
    if not isinstance(artifacts, MinerULocalBatchArchiveArtifacts):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    full_markdown = _decode_text_artifact(artifacts.full_markdown)
    content_list = _json_list(
        _decode_json_artifact(artifacts.content_list_json),
        MINERU_GATEWAY_ARTIFACT_INVALID,
    )
    middle = _json_object(
        _decode_json_artifact(artifacts.middle_json),
        MINERU_GATEWAY_ARTIFACT_INVALID,
    )
    model = _json_object(
        _decode_json_artifact(artifacts.model_json),
        MINERU_GATEWAY_ARTIFACT_INVALID,
    )
    return MinerULocalBatchDecodedArtifacts(
        summary=artifacts.summary,
        full_markdown=full_markdown,
        content_list_json=content_list,
        middle_json=middle,
        model_json=model,
    )


def _prepare_canonical_mapping_input(
    source: DocumentSource,
    artifacts: MinerULocalBatchArchiveArtifacts,
) -> MinerULocalBatchCanonicalMappingInput:
    _validate_pdf_source(source)
    _validate_source_hash(source)
    if not isinstance(artifacts, MinerULocalBatchArchiveArtifacts):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    decoded = _decode_result_archive_artifacts(artifacts)
    return MinerULocalBatchCanonicalMappingInput(
        source_sha256=source.source_sha256,
        source_byte_count=len(source.body),
        source_content_type=source.content_type,
        archive_sha256=artifacts.summary.archive_sha256,
        archive_byte_count=artifacts.summary.archive_byte_count,
        role_digests=(
            _artifact_digest("full_markdown", artifacts.full_markdown),
            _artifact_digest("content_list_json", artifacts.content_list_json),
            _artifact_digest("middle_json", artifacts.middle_json),
            _artifact_digest("model_json", artifacts.model_json),
        ),
        decoded=decoded,
    )


def _artifact_digest(
    role: str,
    content: bytes,
) -> MinerULocalBatchArtifactDigest:
    if not isinstance(content, bytes) or not content:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return MinerULocalBatchArtifactDigest(
        role=role,
        byte_count=len(content),
        sha256=hashlib.sha256(content).hexdigest(),
    )


def _decode_text_artifact(value: bytes) -> str:
    try:
        decoded = value.decode("utf-8", errors="strict")
    except UnicodeError:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    if not decoded:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return decoded


def _decode_json_artifact(value: bytes) -> JsonValue:
    try:
        parsed = json.loads(
            value.decode("utf-8", errors="strict"),
            object_pairs_hook=_json_object_no_duplicate_keys,
            parse_constant=_reject_json_constant,
        )
    except (json.JSONDecodeError, UnicodeError):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return _validate_json_artifact_value(parsed)


def _json_object_no_duplicate_keys(
    pairs: Sequence[tuple[str, object]],
) -> dict[str, object]:
    observed: dict[str, object] = {}
    for key, value in pairs:
        if key in observed:
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
        observed[key] = value
    return observed


def _reject_json_constant(_: str) -> NoReturn:
    _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


def _validate_json_artifact_value(value: object) -> JsonValue:
    if value is None or isinstance(value, bool | str | int):
        return cast("JsonValue", value)
    if isinstance(value, float):
        if not math.isfinite(value):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
        return value
    if isinstance(value, list):
        return [_validate_json_artifact_value(item) for item in value]
    if isinstance(value, dict):
        result: JsonObject = {}
        for key, child in value.items():
            if not isinstance(key, str):
                _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
            result[key] = _validate_json_artifact_value(child)
        return result
    _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    raise AssertionError("unreachable")


def _read_valid_archive_entries(content: bytes) -> list[zipfile.ZipInfo]:
    try:
        with zipfile.ZipFile(io.BytesIO(content)) as archive:
            entries = archive.infolist()
            _validate_archive_entries(entries)
            _validate_archive_crc(archive)
            return entries
    except PermanentJobError:
        raise
    except NotImplementedError:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    except (OSError, RuntimeError, zipfile.BadZipFile, zipfile.LargeZipFile):
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


def _validate_archive_crc(archive: zipfile.ZipFile) -> None:
    if archive.testzip() is not None:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


def _validate_archive_entries(entries: list[zipfile.ZipInfo]) -> None:
    if not entries:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if len(entries) > MAX_MINERU_ARCHIVE_ENTRIES:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    total = 0
    seen: set[str] = set()
    for entry in entries:
        _validate_archive_entry(entry, seen)
        seen.add(entry.filename)
        total += entry.file_size
        if total > MAX_MINERU_ARCHIVE_TOTAL_BYTES:
            _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


def _validate_archive_entry(entry: zipfile.ZipInfo, seen: set[str]) -> None:
    name = entry.filename
    path = PurePosixPath(name)
    path_text = name.removesuffix("/")
    raw_parts = path_text.split("/")
    mode = entry.external_attr >> _ZIP_MODE_SHIFT
    if not name:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if (
        "\\" in name
        or path.is_absolute()
        or not path_text
        or any(part in {"", ".", ".."} for part in raw_parts)
    ):
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if name in seen:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if entry.flag_bits & _ZIP_ENCRYPTED_FLAG:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if stat.S_ISLNK(mode):
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if entry.file_size > MAX_MINERU_ARCHIVE_ENTRY_BYTES:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if entry.file_size and entry.compress_size == 0:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    if (
        entry.compress_size
        and entry.file_size > entry.compress_size * MAX_MINERU_ARCHIVE_COMPRESSION_RATIO
    ):
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


def _required_archive_artifacts(
    entries: list[zipfile.ZipInfo],
) -> dict[str, bool]:
    basenames = [
        PurePosixPath(entry.filename).name for entry in entries if not entry.is_dir()
    ]
    return {
        "content_list_present": any(
            name == "content_list.json" or name.endswith("_content_list.json")
            for name in basenames
        ),
        "full_markdown_present": "full.md" in basenames,
        "middle_json_present": any(
            name in {"layout.json", "middle.json"} or name.endswith("_middle.json")
            for name in basenames
        ),
        "model_json_present": any(
            name == "model.json" or name.endswith("_model.json") for name in basenames
        ),
    }


def _required_archive_role_entries(
    entries: list[zipfile.ZipInfo],
) -> dict[str, zipfile.ZipInfo]:
    role_entries: dict[str, zipfile.ZipInfo] = {}
    for entry in entries:
        if entry.is_dir():
            continue
        role = _archive_entry_role(entry.filename)
        if role is None:
            continue
        if role in role_entries:
            _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
        role_entries[role] = entry
    if set(role_entries) != {
        "content_list_json",
        "full_markdown",
        "middle_json",
        "model_json",
    }:
        _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)
    return role_entries


def _archive_entry_role(filename: str) -> str | None:
    name = PurePosixPath(filename).name
    if name == "full.md":
        return "full_markdown"
    if name == "content_list.json" or name.endswith("_content_list.json"):
        return "content_list_json"
    if name in {"layout.json", "middle.json"} or name.endswith("_middle.json"):
        return "middle_json"
    if name == "model.json" or name.endswith("_model.json"):
        return "model_json"
    return None


def _validate_required_archive_artifacts(required: dict[str, bool]) -> None:
    for present in required.values():
        if present is not True:
            _reject_permanent(MINERU_GATEWAY_ARCHIVE_INVALID)


async def _read_bounded_archive_response(response: httpx.Response) -> bytes:
    _validate_content_length(response.headers.get("content-length"))
    if response.is_stream_consumed:
        raw_content = response.content
        if len(raw_content) > MAX_MINERU_RESULT_ARCHIVE_BYTES:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)
        return raw_content
    raw = bytearray()
    async for chunk in response.aiter_raw():
        if len(raw) + len(chunk) > MAX_MINERU_RESULT_ARCHIVE_BYTES:
            _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)
        raw.extend(chunk)
    return bytes(raw)


def _validate_content_length(value: str | None) -> None:
    if value is None:
        return
    if not value.isascii() or not value.isdecimal():
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    if int(value) > MAX_MINERU_RESULT_ARCHIVE_BYTES:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_TOO_LARGE)


def _validate_identity_encoding(response: httpx.Response) -> None:
    encoding = response.headers.get("content-encoding")
    if encoding is not None and encoding.strip().lower() != "identity":
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)


def _validate_archive_content_type(value: str | None) -> None:
    if value is None:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)
    normalized = value.split(";", 1)[0].strip().lower()
    if normalized not in _ZIP_CONTENT_TYPES:
        _reject_permanent(MINERU_GATEWAY_RESPONSE_INVALID)


def _clear_client_cookies(client: httpx.AsyncClient) -> None:
    client.cookies.clear()


def _is_visible_url(value: str) -> bool:
    return all(
        _VISIBLE_ASCII_MIN <= ord(character) < _ASCII_DELETE for character in value
    )


def _safe_dynamic_path(path: str) -> bool:
    return (
        bool(path)
        and "%" not in path
        and "\\" not in path
        and all(segment not in {"", ".", ".."} for segment in path.split("/")[1:])
    )


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
