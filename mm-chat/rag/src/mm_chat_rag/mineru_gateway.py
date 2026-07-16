"""Default-off MinerU local-batch gateway seams for G7.5.

This module deliberately keeps MinerU behind default-off seams: local-batch
allocate/upload/poll/download, ZIP validation/extraction/decode, hash-bound
mapping input, and a full-Markdown text-baseline parser adapter. The baseline
emits projection-ready artifacts from ``full.md`` only; it does not interpret
MinerU ``content_list``/layout/model fields for citation-grade locators.
Importing this module does not register production parse handlers or spend
provider quota.
"""

from __future__ import annotations

import hashlib
import io
import json
import math
import re
import stat
import uuid
import zipfile
from collections.abc import Coroutine, Sequence
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Any, Final, NoReturn, Protocol, cast
from urllib.parse import SplitResult, urlsplit

import httpx

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

MINERU_GATEWAY_CREDENTIALS_MISSING: Final = "MINERU_GATEWAY_CREDENTIALS_MISSING"
MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED: Final = (
    "MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED"
)
MINERU_GATEWAY_CONTEXT_INVALID: Final = "MINERU_GATEWAY_CONTEXT_INVALID"
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
MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH: Final = (
    "48ac1810a92dcdd61db73646f3c8780e8ebc76b1525145452df7e3c0a819bb03"
)
MINERU_TEXT_BASELINE_ARTIFACT_SET_NAMESPACE: Final = uuid.UUID(
    "7e3ad35b-7ef5-5ab9-b490-38a43bdfad4a"
)
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
_ZERO_UUID: Final = uuid.UUID(int=0)
_TEXT_BASELINE_CHUNK_MAX_BYTES: Final = 2400
_TEXT_BASELINE_TOKEN_BYTES: Final = 4
_TEXT_BASELINE_CHILD_MAX_TOKENS: Final = 650
_BBOX_COORDINATES: Final = 4


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


@dataclass(frozen=True, slots=True)
class MinerUPageRegionLocator:
    """One admitted MinerU page+bbox locator for the full.md baseline."""

    page_index: int
    bbox_milli_point: tuple[int, int, int, int]

    def __post_init__(self) -> None:
        if (
            not _is_nonnegative_int(self.page_index)
            or not isinstance(self.bbox_milli_point, tuple)
            or len(self.bbox_milli_point) != _BBOX_COORDINATES
            or not all(_is_nonnegative_int(value) for value in self.bbox_milli_point)
            or self.bbox_milli_point[0] >= self.bbox_milli_point[2]
            or self.bbox_milli_point[1] >= self.bbox_milli_point[3]
        ):
            _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


class MinerUResultArchiveProvider(Protocol):
    """Default-off provider that supplies already downloaded MinerU ZIP bytes."""

    def fetch_result_archive(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> Coroutine[Any, Any, bytes]: ...


class MinerUTextBaselineArchiveParserGateway:
    """ParserGateway-shaped adapter for the MinerU full.md baseline mapper."""

    def __init__(
        self,
        archive_provider: MinerUResultArchiveProvider | None = None,
    ) -> None:
        self._archive_provider = archive_provider

    async def parse_document(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> ParsedDocumentArtifacts:
        """Parse one source via an injected archive provider, disabled by default."""
        admitted = _validate_parser_context(context)
        if self._archive_provider is None:
            _reject_permanent(MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED)
        _validate_pdf_source(source)
        _validate_source_hash(source)
        archive_body = await self._archive_provider.fetch_result_archive(
            admitted,
            source,
        )
        return _build_text_baseline_parse_artifacts_from_archive(
            source,
            archive_body,
            artifact_set_id=_text_baseline_artifact_set_id(admitted, source),
        )


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

    def build_text_baseline_parse_artifacts(
        self,
        context: object,
        mapping_input: MinerULocalBatchCanonicalMappingInput,
        *,
        artifact_set_id: uuid.UUID,
    ) -> ParsedDocumentArtifacts:
        """Map full.md text into projection-ready baseline parser artifacts."""
        _ = context
        return _build_text_baseline_parse_artifacts(
            mapping_input,
            artifact_set_id=artifact_set_id,
        )

    def build_text_baseline_parse_artifacts_from_archive(
        self,
        context: object,
        source: DocumentSource,
        archive_body: bytes,
        *,
        artifact_set_id: uuid.UUID,
    ) -> ParsedDocumentArtifacts:
        """Compose archive extraction through the full.md baseline mapper."""
        _ = context
        return _build_text_baseline_parse_artifacts_from_archive(
            source,
            archive_body,
            artifact_set_id=artifact_set_id,
        )


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


def _text_baseline_page_region_locator(
    mapping_input: MinerULocalBatchCanonicalMappingInput,
    text: str,
) -> MinerUPageRegionLocator | None:
    if not _content_list_has_single_full_text_match(
        mapping_input.decoded.content_list_json,
        text,
    ):
        return None
    candidates = _middle_page_region_candidates(mapping_input.decoded.middle_json, text)
    if len(candidates) > 1:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    if not candidates:
        return None
    return candidates[0]


def _content_list_has_single_full_text_match(
    content_list: list[JsonValue],
    text: str,
) -> bool:
    matches = 0
    for item in content_list:
        if not isinstance(item, dict):
            continue
        if _mineru_text_field_matches(item, text):
            matches += 1
    if matches > 1:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return matches == 1


def _middle_page_region_candidates(
    middle_json: JsonObject,
    text: str,
) -> tuple[MinerUPageRegionLocator, ...]:
    pages = middle_json.get("pages")
    if not isinstance(pages, list):
        return ()
    candidates: list[MinerUPageRegionLocator] = []
    for page in pages:
        if not isinstance(page, dict):
            continue
        elements = page.get("elements")
        if not isinstance(elements, list):
            continue
        for element in elements:
            if not isinstance(element, dict) or not _mineru_element_matches_text(
                element,
                text,
            ):
                continue
            candidates.append(
                MinerUPageRegionLocator(
                    page_index=_mineru_page_index(page),
                    bbox_milli_point=_mineru_bbox_milli_point(element),
                )
            )
    return tuple(candidates)


def _mineru_element_matches_text(element: JsonObject, text: str) -> bool:
    return _mineru_text_field_matches(element, text)


def _mineru_text_field_matches(value: JsonObject, text: str) -> bool:
    return value.get("text") == text or value.get("sourceText") == text


def _mineru_page_index(page: JsonObject) -> int:
    value = page.get("pageIndex")
    if value is None:
        value = page.get("page_idx")
    if not _is_nonnegative_int(value):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return cast("int", value)


def _mineru_bbox_milli_point(element: JsonObject) -> tuple[int, int, int, int]:
    value = element.get("bboxMilliPoint")
    if value is None:
        value = element.get("bbox")
    if not isinstance(value, list) or len(value) != _BBOX_COORDINATES:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    bbox = tuple(value)
    if not all(_is_nonnegative_int(coordinate) for coordinate in bbox):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return cast("tuple[int, int, int, int]", bbox)


def _build_text_baseline_parse_artifacts(
    mapping_input: MinerULocalBatchCanonicalMappingInput,
    *,
    artifact_set_id: uuid.UUID,
) -> ParsedDocumentArtifacts:
    if not isinstance(mapping_input, MinerULocalBatchCanonicalMappingInput):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    text = mapping_input.decoded.full_markdown
    text_bytes = text.encode("utf-8")
    if not text_bytes:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)

    page_region = _text_baseline_page_region_locator(mapping_input, text)
    full_markdown_digest = _role_digest(mapping_input, "full_markdown")
    source_unit_id = _hash_seed(
        "mineru_text_baseline.source_unit.v1",
        mapping_input.source_sha256,
        full_markdown_digest.sha256,
    )
    flow_seed_id = _hash_seed(
        "mineru_text_baseline.flow_seed.v1",
        mapping_input.source_sha256,
        mapping_input.archive_sha256,
    )
    logical_flow_id = _hash_seed("mineru_text_baseline.flow.v1", flow_seed_id)
    structure_id = _hash_seed(
        "mineru_text_baseline.structure.v1",
        mapping_input.source_sha256,
        mapping_input.archive_sha256,
        full_markdown_digest.sha256,
    )
    block_id = _hash_seed("mineru_text_baseline.block.v1", structure_id)
    block_source_span_hash = _hash_seed(
        "mineru_text_baseline.text_span.v1",
        source_unit_id,
        0,
        len(text_bytes),
    )
    block_locator = _text_baseline_locator_set(
        text,
        start_byte=0,
        end_byte=len(text_bytes),
        page_region=page_region,
        source_unit_id=source_unit_id,
        structure_id=structure_id,
    )
    provenance_id = _hash_seed(
        "mineru_text_baseline.provenance_id.v1",
        block_id,
        full_markdown_digest.sha256,
    )
    provenance = _text_baseline_provenance(
        provenance_id=provenance_id,
        target_owner_seed_id=structure_id,
        source_unit_id=source_unit_id,
        payload_ref=full_markdown_digest.sha256,
    )
    block: JsonObject = {
        "blockType": "paragraph",
        "confidence": 10000,
        "contentHash": hashlib.sha256(text_bytes).hexdigest(),
        "flags": {"derived": False, "nonIndexable": False},
        "flowSeedId": flow_seed_id,
        "headingPath": [],
        "locatorSet": block_locator,
        "logicalBlockId": block_id,
        "ordinal": 0,
        "parentBlockId": None,
        "provenanceRefs": [provenance_id],
        "readingFlowOrdinal": 0,
        "sourceSpanHash": {
            "kind": "text",
            "textSourceSpanHash": block_source_span_hash,
        },
        "structureRef": {
            "ownerSeedId": structure_id,
            "structureKind": "paragraph",
            "structureOrdinal": 0,
        },
        "textRange": {"endByte": len(text_bytes), "startByte": 0},
    }
    canonical_ir: JsonObject = {
        "assets": [],
        "blocks": [block],
        "formulas": [],
        "normalizationMapRef": _normalization_map_ref(mapping_input),
        "normalizationProfile": {
            "profileHash": _hash_seed(
                "mineru_text_baseline.normalization_profile.v1",
                "identity-full-markdown",
            ),
            "schemaVersion": "normalization-profile.v1",
        },
        "pages": [],
        "parser": {
            "configHash": _hash_seed(
                "mineru_text_baseline.parser_config.v1",
                mapping_input.archive_sha256,
                _role_digest(mapping_input, "model_json").sha256,
            ),
            "parserBuildHash": _hash_seed(
                "mineru_text_baseline.parser_build.v1",
                MINERU_MODEL_VERSION,
            ),
            "profileHash": _hash_seed("mineru_text_baseline.parser_profile.v1"),
            "schemaVersion": "parser-profile.v1",
        },
        "provenance": [provenance],
        "readingFlows": [
            {
                "flowOrdinal": 0,
                "flowSeedId": flow_seed_id,
                "logicalFlowId": logical_flow_id,
                "orderedLogicalBlockIds": [block_id],
            }
        ],
        "schemaVersion": "canonical-ir.v2",
        "source": {
            "bytes": mapping_input.source_byte_count,
            "format": "pdf",
            "sha256": mapping_input.source_sha256,
        },
        "tables": [],
        "textBuffer": {
            "bytes": len(text_bytes),
            "encoding": "utf-8",
            "sha256": hashlib.sha256(text_bytes).hexdigest(),
            "text": text,
        },
    }
    chunk_manifest = _text_baseline_chunk_manifest(
        text,
        block_id=block_id,
        logical_flow_id=logical_flow_id,
        page_region=page_region,
        source_unit_id=source_unit_id,
        structure_id=structure_id,
        source_sha256=mapping_input.source_sha256,
    )
    return ParsedDocumentArtifacts(
        artifact_set_id=artifact_set_id,
        canonical_ir=cast("Any", canonical_ir),
        chunk_manifest=cast("Any", chunk_manifest),
    )


def _build_text_baseline_parse_artifacts_from_archive(
    source: DocumentSource,
    archive_body: bytes,
    *,
    artifact_set_id: uuid.UUID,
) -> ParsedDocumentArtifacts:
    artifacts = _extract_result_archive_artifacts(archive_body)
    mapping_input = _prepare_canonical_mapping_input(source, artifacts)
    return _build_text_baseline_parse_artifacts(
        mapping_input,
        artifact_set_id=artifact_set_id,
    )


def _validate_parser_context(context: object) -> ProcessingJobContext:
    if not isinstance(context, ProcessingJobContext):
        _reject_permanent(MINERU_GATEWAY_CONTEXT_INVALID)
    if context.stage != "parse" or context.materialization_id in {None, _ZERO_UUID}:
        _reject_permanent(MINERU_GATEWAY_CONTEXT_INVALID)
    return context


def _text_baseline_artifact_set_id(
    context: ProcessingJobContext,
    source: DocumentSource,
) -> uuid.UUID:
    materialization_id = context.materialization_id
    if materialization_id is None or materialization_id == _ZERO_UUID:
        _reject_permanent(MINERU_GATEWAY_CONTEXT_INVALID)
    _validate_source_hash(source)
    return uuid.uuid5(
        MINERU_TEXT_BASELINE_ARTIFACT_SET_NAMESPACE,
        ":".join(
            (
                str(materialization_id),
                source.source_sha256,
                MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            )
        ),
    )


def _normalization_map_ref(
    mapping_input: MinerULocalBatchCanonicalMappingInput,
) -> JsonObject:
    value: JsonObject = {
        "archiveSha256": mapping_input.archive_sha256,
        "profile": "mm-chat.mineru.full-md.identity.v1",
        "sourceSha256": mapping_input.source_sha256,
    }
    content = _canonicalish_json_bytes(value)
    return {
        "bytes": len(content),
        "schemaVersion": "normalization-map.v1",
        "sha256": hashlib.sha256(content).hexdigest(),
    }


def _text_baseline_provenance(
    *,
    provenance_id: str,
    target_owner_seed_id: str,
    source_unit_id: str,
    payload_ref: str,
) -> JsonObject:
    base: JsonObject = {
        "derivationProfileHash": _hash_seed(
            "mineru_text_baseline.derivation_profile.v1"
        ),
        "payloadRef": payload_ref,
        "provenanceId": provenance_id,
        "provenanceKind": "parser_derivation",
        "provenanceOrdinal": 0,
        "sourceUnitRef": source_unit_id,
        "targetKind": "block",
        "targetKindRank": 0,
        "targetOwnerSeedId": target_owner_seed_id,
    }
    return {
        **base,
        "provenanceHash": _hash_json(base),
    }


def _text_baseline_chunk_manifest(
    text: str,
    *,
    block_id: str,
    logical_flow_id: str,
    page_region: MinerUPageRegionLocator | None,
    source_unit_id: str,
    structure_id: str,
    source_sha256: str,
) -> JsonObject:
    parents: list[JsonObject] = []
    children: list[JsonObject] = []
    span_hashes: list[str] = []
    for ordinal, (start_byte, end_byte, content) in enumerate(
        _text_chunk_ranges(text)
    ):
        content_bytes = content.encode("utf-8")
        content_hash = hashlib.sha256(content_bytes).hexdigest()
        span_hash = _hash_seed(
            "mineru_text_baseline.chunk_span.v1",
            block_id,
            start_byte,
            end_byte,
        )
        parent_id = _hash_seed(
            "mineru_text_baseline.parent_chunk.v1",
            block_id,
            start_byte,
            end_byte,
        )
        child_id = _hash_seed(
            "mineru_text_baseline.child_chunk.v1",
            parent_id,
            ordinal,
        )
        parent_seed_id = _hash_seed(
            "mineru_text_baseline.parent_seed.v1",
            parent_id,
        )
        fragment = _text_baseline_chunk_fragment(
            text,
            block_id=block_id,
            start_byte=start_byte,
            end_byte=end_byte,
            page_region=page_region,
            source_unit_id=source_unit_id,
            structure_id=structure_id,
            fragment_source_span_hash=span_hash,
        )
        parent_common = {
            "chunkKind": "parent",
            "chunkProfileHash": MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            "chunkSourceSpanHash": span_hash,
            "contentBytes": len(content_bytes),
            "contentHash": content_hash,
            "joiners": [],
            "logicalChunkId": parent_id,
            "logicalFlowId": logical_flow_id,
            "parentChunkSeedId": parent_seed_id,
            "parentOrdinal": ordinal,
            "sectionOwnerSeedId": structure_id,
            "spanFragments": [fragment],
            "tokenCount": _estimated_token_count(content_bytes),
        }
        child_common = {
            "childOrdinal": ordinal,
            "chunkKind": "child",
            "chunkProfileHash": MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            "chunkSourceSpanHash": span_hash,
            "contentBytes": len(content_bytes),
            "contentHash": content_hash,
            "joiners": [],
            "logicalChunkId": child_id,
            "logicalFlowId": logical_flow_id,
            "logicalParentChunkId": parent_id,
            "parentChunkSeedId": parent_seed_id,
            "spanFragments": [fragment],
            "tokenCount": _estimated_token_count(content_bytes),
        }
        parents.append(cast("JsonObject", parent_common))
        children.append(cast("JsonObject", child_common))
        span_hashes.extend([span_hash, span_hash])
    return {
        "childAggregateHash": _hash_sequence(
            "mineru_text_baseline.children.v1",
            [cast("str", child["logicalChunkId"]) for child in children],
        ),
        "childCount": len(children),
        "children": cast("list[JsonValue]", children),
        "chunkProfileHash": MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
        "joinerAggregateHash": _hash_sequence(
            "mineru_text_baseline.joiners.v1",
            [],
        ),
        "joinerCount": 0,
        "parentAggregateHash": _hash_sequence(
            "mineru_text_baseline.parents.v1",
            [cast("str", parent["logicalChunkId"]) for parent in parents],
        ),
        "parentCount": len(parents),
        "parents": cast("list[JsonValue]", parents),
        "schemaVersion": "chunk-manifest.v2",
        "sourceSha256": source_sha256,
        "spanAggregateHash": _hash_sequence(
            "mineru_text_baseline.spans.v1",
            span_hashes,
        ),
        "spanCount": len(span_hashes),
    }


def _text_baseline_chunk_fragment(
    text: str,
    *,
    block_id: str,
    start_byte: int,
    end_byte: int,
    page_region: MinerUPageRegionLocator | None,
    source_unit_id: str,
    structure_id: str,
    fragment_source_span_hash: str,
) -> JsonObject:
    return {
        "blockEndByte": end_byte,
        "blockLogicalId": block_id,
        "blockStartByte": start_byte,
        "clippedLocatorSet": _text_baseline_locator_set(
            text,
            start_byte=start_byte,
            end_byte=end_byte,
            page_region=page_region,
            source_unit_id=source_unit_id,
            structure_id=structure_id,
        ),
        "fragmentKind": "primary",
        "fragmentSourceSpanHash": fragment_source_span_hash,
    }


def _text_baseline_locator_set(
    text: str,
    *,
    start_byte: int,
    end_byte: int,
    page_region: MinerUPageRegionLocator | None,
    source_unit_id: str,
    structure_id: str,
) -> JsonObject:
    start_line, start_column, start_scalar = _line_column_for_byte_offset(
        text,
        start_byte,
    )
    end_line, end_column, end_scalar = _line_column_for_byte_offset(text, end_byte)
    views: list[JsonObject] = []
    if page_region is not None:
        views.append(_page_region_view(page_region))
    views.extend(
        [
            {
                "decodedScalarEnd": end_scalar,
                "decodedScalarStart": start_scalar,
                "endColumn": end_column,
                "endLine": end_line,
                "kind": "source_text_position",
                "opaqueSourceUnitId": source_unit_id,
                "rawByteEnd": end_byte,
                "rawByteStart": start_byte,
                "startColumn": start_column,
                "startLine": start_line,
            },
            {
                "kind": "derived_structure",
                "opaqueStructureId": structure_id,
                "structureKind": "paragraph",
            },
        ]
    )
    text_anchor: JsonObject = {
        "anchorOrdinal": 0,
        "canonicalEndByte": end_byte,
        "canonicalStartByte": start_byte,
        "sourceFragments": [
            {
                "fragmentOrdinal": 0,
                "views": cast("list[JsonValue]", views),
            }
        ],
    }
    base: JsonObject = {
        "structuralAnchors": [],
        "textAnchors": [text_anchor],
        "version": 2,
    }
    return {
        **base,
        "aggregateHash": _hash_json(base),
    }


def _page_region_view(page_region: MinerUPageRegionLocator) -> JsonObject:
    return {
        "bboxMilliPoint": list(page_region.bbox_milli_point),
        "kind": "page_region",
        "pageIndex": page_region.page_index,
    }


def _text_chunk_ranges(text: str) -> tuple[tuple[int, int, str], ...]:
    ranges: list[tuple[int, int, str]] = []
    current: list[str] = []
    current_bytes = 0
    start_byte = 0
    offset = 0
    for character in text:
        character_bytes = len(character.encode("utf-8"))
        if current and current_bytes + character_bytes > _TEXT_BASELINE_CHUNK_MAX_BYTES:
            chunk_text = "".join(current)
            end_byte = start_byte + current_bytes
            ranges.append((start_byte, end_byte, chunk_text))
            start_byte = end_byte
            current = []
            current_bytes = 0
        current.append(character)
        current_bytes += character_bytes
        offset += character_bytes
    if current:
        ranges.append((start_byte, start_byte + current_bytes, "".join(current)))
    if not ranges or ranges[-1][1] != offset:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    return tuple(ranges)


def _line_column_for_byte_offset(text: str, offset: int) -> tuple[int, int, int]:
    encoded = text.encode("utf-8")
    if offset < 0 or offset > len(encoded):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    try:
        prefix = encoded[:offset].decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    line = prefix.count("\n")
    column = len(prefix.rsplit("\n", 1)[-1])
    return line, column, len(prefix)


def _estimated_token_count(content: bytes) -> int:
    return max(
        1,
        min(
            _TEXT_BASELINE_CHILD_MAX_TOKENS,
            math.ceil(len(content) / _TEXT_BASELINE_TOKEN_BYTES),
        ),
    )


def _role_digest(
    mapping_input: MinerULocalBatchCanonicalMappingInput,
    role: str,
) -> MinerULocalBatchArtifactDigest:
    for digest in mapping_input.role_digests:
        if digest.role == role:
            return digest
    _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)
    raise AssertionError("unreachable")


def _hash_seed(domain: str, *parts: object) -> str:
    digest = hashlib.sha256()
    digest.update(domain.encode("utf-8"))
    for part in parts:
        digest.update(b"\x00")
        digest.update(str(part).encode("utf-8"))
    return digest.hexdigest()


def _hash_sequence(domain: str, values: Sequence[str]) -> str:
    return _hash_seed(domain, *values)


def _hash_json(value: object) -> str:
    return hashlib.sha256(_canonicalish_json_bytes(value)).hexdigest()


def _canonicalish_json_bytes(value: object) -> bytes:
    try:
        return json.dumps(
            value,
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    except (TypeError, ValueError, UnicodeEncodeError):
        _reject_permanent(MINERU_GATEWAY_ARTIFACT_INVALID)


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
