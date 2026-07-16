"""Default-off document source gateway composition for parse jobs.

The gateway joins two explicit authorities: a parse-job-scoped metadata lookup
and an object-byte reader. It never reads deployment secrets or object-store
configuration by itself, and it is not registered in production handlers.
"""

from __future__ import annotations

import hashlib
import re
import uuid
from collections.abc import Coroutine
from dataclasses import dataclass
from pathlib import Path
from stat import S_ISREG
from typing import Any, Final, NoReturn, Protocol
from urllib.parse import urlsplit

import httpx

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    JOB_HANDLER_SOURCE_INVALID,
    DocumentSource,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

_SUPPORTED_STORAGE_BACKENDS: Final = frozenset({"local", "minio", "s3"})
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_CONTENT_TYPE_RE: Final = re.compile(r"^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9.+-]{1,64}$")
_MAX_OBJECT_KEY_BYTES: Final = 1024
_MAX_SOURCE_BYTES: Final = 512 * 1024 * 1024
_MAX_INTERNAL_TOKEN_BYTES: Final = 4096
_VISIBLE_ASCII_MIN: Final = 33
_VISIBLE_ASCII_MAX: Final = 126
_ZERO_UUID: Final = uuid.UUID(int=0)
GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED: Final = (
    "GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED"
)
GO_SOURCE_OBJECT_GATEWAY_TIMEOUT: Final = httpx.Timeout(
    connect=5.0,
    read=120.0,
    write=15.0,
    pool=5.0,
)
GO_SOURCE_OBJECT_GATEWAY_LIMITS: Final = httpx.Limits(
    max_connections=1,
    max_keepalive_connections=1,
)
GO_SOURCE_OBJECT_GATEWAY_RETRY_AFTER_SECONDS: Final = 30
GO_SOURCE_OBJECT_PATH: Final = "/internal/rag/source-object"
_HTTP_OK: Final = 200
_HTTP_INTERNAL_SERVER_ERROR: Final = 500


@dataclass(frozen=True, slots=True)
class FileSourceMetadata:
    """Storage metadata for the source file bound to one parse job."""

    file_id: uuid.UUID
    storage_backend: str
    object_key: str
    sha256: str
    byte_size: int
    content_type: str

    def __post_init__(self) -> None:
        if not isinstance(self.file_id, uuid.UUID) or self.file_id == _ZERO_UUID:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if (
            not isinstance(self.storage_backend, str)
            or self.storage_backend not in _SUPPORTED_STORAGE_BACKENDS
        ):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        _validate_object_key(self.object_key)
        if not _SHA256_RE.fullmatch(self.sha256):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if (
            isinstance(self.byte_size, bool)
            or not isinstance(self.byte_size, int)
            or not 1 <= self.byte_size <= _MAX_SOURCE_BYTES
        ):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if not _CONTENT_TYPE_RE.fullmatch(self.content_type):
            _reject(JOB_HANDLER_SOURCE_INVALID)


class SourceMetadataGateway(Protocol):
    """Fetch token-fenced file metadata for an admitted parse job."""

    def fetch_source_metadata(
        self,
        context: ProcessingJobContext,
    ) -> Coroutine[Any, Any, FileSourceMetadata]: ...


class ObjectBytesGateway(Protocol):
    """Fetch object bytes by opaque metadata, never by browser-visible URL."""

    def fetch_object_bytes(
        self,
        context: ProcessingJobContext,
        metadata: FileSourceMetadata,
    ) -> Coroutine[Any, Any, bytes]: ...


class GoSourceObjectBytesGateway:
    """Fetch source bytes through Go's private, token-gated object gateway."""

    def __init__(
        self,
        *,
        base_url: str,
        internal_token: str,
        worker_id: uuid.UUID,
        client: httpx.AsyncClient | None = None,
        max_bytes: int = _MAX_SOURCE_BYTES,
    ) -> None:
        self._url = _source_gateway_url(base_url)
        self._internal_token = _validate_internal_token(internal_token)
        if not isinstance(worker_id, uuid.UUID) or worker_id == _ZERO_UUID:
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        if (
            isinstance(max_bytes, bool)
            or not isinstance(max_bytes, int)
            or not 1 <= max_bytes <= _MAX_SOURCE_BYTES
        ):
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        self._worker_id = worker_id
        self._client = client
        self._max_bytes = max_bytes

    async def fetch_object_bytes(
        self,
        context: ProcessingJobContext,
        metadata: FileSourceMetadata,
    ) -> bytes:
        """Return raw source bytes after Go revalidates the leased job fence."""
        if not isinstance(context, ProcessingJobContext) or not isinstance(
            metadata, FileSourceMetadata
        ):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if context.lease_token is None or context.materialization_id is None:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if metadata.file_id != context.file_id:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if metadata.byte_size > self._max_bytes:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        payload: dict[str, str] = {
            "jobId": str(context.job_id),
            "workerId": str(self._worker_id),
            "leaseToken": str(context.lease_token),
            "fileId": str(metadata.file_id),
            "materializationId": str(context.materialization_id),
        }
        if self._client is not None:
            return await _post_go_source_object(
                self._client,
                self._url,
                self._internal_token,
                payload,
                metadata,
                self._max_bytes,
            )
        async with httpx.AsyncClient(
            timeout=GO_SOURCE_OBJECT_GATEWAY_TIMEOUT,
            limits=GO_SOURCE_OBJECT_GATEWAY_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            return await _post_go_source_object(
                client,
                self._url,
                self._internal_token,
                payload,
                metadata,
                self._max_bytes,
            )


class ObjectStoreDocumentSourceGateway:
    """Compose source metadata and object bytes into ``DocumentSource``."""

    def __init__(
        self,
        *,
        metadata: SourceMetadataGateway | None,
        objects: ObjectBytesGateway | None,
    ) -> None:
        self._metadata = metadata
        self._objects = objects

    async def fetch_document_source(
        self,
        context: ProcessingJobContext,
    ) -> DocumentSource:
        """Fetch, size-check, hash-check, and return parse source bytes."""
        if self._metadata is None or self._objects is None:
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        if not isinstance(context, ProcessingJobContext):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        metadata = await self._metadata.fetch_source_metadata(context)
        if not isinstance(metadata, FileSourceMetadata):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if metadata.file_id != context.file_id:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        body = await self._objects.fetch_object_bytes(context, metadata)
        if not isinstance(body, bytes) or len(body) != metadata.byte_size:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if hashlib.sha256(body).hexdigest() != metadata.sha256:
            _reject(JOB_HANDLER_SOURCE_HASH_MISMATCH)
        return DocumentSource(
            body=body,
            source_sha256=metadata.sha256,
            content_type=metadata.content_type,
        )


class LocalObjectBytesGateway:
    """Default-off local filesystem object reader for `storage_backend=local`."""

    def __init__(
        self,
        *,
        root: str | Path,
        max_bytes: int = _MAX_SOURCE_BYTES,
    ) -> None:
        if (
            isinstance(max_bytes, bool)
            or not isinstance(max_bytes, int)
            or not 1 <= max_bytes <= _MAX_SOURCE_BYTES
        ):
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        self._root = _resolve_local_root(root)
        self._max_bytes = max_bytes

    async def fetch_object_bytes(
        self,
        context: ProcessingJobContext,
        metadata: FileSourceMetadata,
    ) -> bytes:
        """Read bounded local object bytes for already-fenced metadata."""
        _ = context
        if not isinstance(metadata, FileSourceMetadata):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if metadata.storage_backend != "local":
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        if metadata.byte_size > self._max_bytes:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        object_path = _resolve_local_object_path(self._root, metadata.object_key)
        try:
            file_stat = object_path.lstat()
        except OSError as error:
            _reject_from(JOB_HANDLER_SOURCE_INVALID, error)
        if object_path.is_symlink() or not S_ISREG(file_stat.st_mode):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if file_stat.st_size != metadata.byte_size:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        try:
            body = object_path.read_bytes()
        except OSError as error:
            _reject_from(JOB_HANDLER_SOURCE_INVALID, error)
        if len(body) != metadata.byte_size:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        return body


async def _post_go_source_object(
    client: httpx.AsyncClient,
    url: str,
    internal_token: str,
    payload: dict[str, str],
    metadata: FileSourceMetadata,
    max_bytes: int,
) -> bytes:
    headers = {
        "Accept": "application/octet-stream",
        "Accept-Encoding": "identity",
        "Content-Type": "application/json",
        "X-MM-Chat-Internal-Token": internal_token,
    }
    try:
        async with client.stream(
            "POST", url, headers=headers, json=payload
        ) as response:
            if response.status_code != _HTTP_OK:
                _reject_for_go_source_status(response.status_code)
            _validate_go_source_headers(response, metadata)
            body = await _read_bounded_go_source_response(response, max_bytes)
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED)
    if len(body) != metadata.byte_size:
        _reject(JOB_HANDLER_SOURCE_INVALID)
    if hashlib.sha256(body).hexdigest() != metadata.sha256:
        _reject(JOB_HANDLER_SOURCE_HASH_MISMATCH)
    return body


def _validate_go_source_headers(
    response: httpx.Response,
    metadata: FileSourceMetadata,
) -> None:
    if response.headers.get("X-MM-Chat-File-ID") != str(metadata.file_id):
        _reject(JOB_HANDLER_SOURCE_INVALID)
    if response.headers.get("X-MM-Chat-Source-SHA256") != metadata.sha256:
        _reject(JOB_HANDLER_SOURCE_HASH_MISMATCH)
    content_length = response.headers.get("Content-Length")
    if content_length is None:
        return
    try:
        parsed_length = int(content_length)
    except ValueError as error:
        _reject_from(JOB_HANDLER_SOURCE_INVALID, error)
    if parsed_length != metadata.byte_size:
        _reject(JOB_HANDLER_SOURCE_INVALID)


async def _read_bounded_go_source_response(
    response: httpx.Response,
    max_bytes: int,
) -> bytes:
    if response.is_stream_consumed:
        raw_content = response.content
        if len(raw_content) > max_bytes:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        return raw_content
    raw = bytearray()
    async for chunk in response.aiter_raw():
        if len(raw) + len(chunk) > max_bytes:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        raw.extend(chunk)
    return bytes(raw)


def _reject_for_go_source_status(status_code: int) -> NoReturn:
    if status_code in {401, 403}:
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    if status_code >= _HTTP_INTERNAL_SERVER_ERROR:
        _reject_retryable(GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED)
    _reject(JOB_HANDLER_SOURCE_INVALID)


def _validate_object_key(value: str) -> None:
    if (
        not isinstance(value, str)
        or not value
        or value != value.strip()
        or len(value.encode("utf-8")) > _MAX_OBJECT_KEY_BYTES
        or value.startswith("/")
        or value.endswith("/")
        or "\\" in value
        or ":" in value
    ):
        _reject(JOB_HANDLER_SOURCE_INVALID)
    segments = value.split("/")
    if any(segment in {"", ".", ".."} for segment in segments):
        _reject(JOB_HANDLER_SOURCE_INVALID)


def _resolve_local_root(root: str | Path) -> Path:
    raw_root = str(root)
    if not raw_root or raw_root != raw_root.strip():
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    try:
        resolved = Path(root).resolve(strict=True)
    except OSError as error:
        _reject_from(JOB_HANDLER_DEPENDENCY_UNCONFIGURED, error)
    if not resolved.is_dir():
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    return resolved


def _resolve_local_object_path(root: Path, object_key: str) -> Path:
    _validate_object_key(object_key)
    candidate = root.joinpath(*object_key.split("/"))
    try:
        candidate.resolve(strict=False).relative_to(root)
    except (OSError, ValueError) as error:
        _reject_from(JOB_HANDLER_SOURCE_INVALID, error)
    return candidate


def _source_gateway_url(base_url: str) -> str:
    if not isinstance(base_url, str) or not base_url or base_url != base_url.strip():
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    try:
        parsed = urlsplit(base_url)
    except ValueError as error:
        _reject_from(JOB_HANDLER_DEPENDENCY_UNCONFIGURED, error)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    if parsed.query or parsed.fragment:
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    return f"{base_url.rstrip('/')}{GO_SOURCE_OBJECT_PATH}"


def _validate_internal_token(value: str) -> str:
    if not isinstance(value, str):
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    if (
        not value
        or value != value.strip()
        or len(value.encode("utf-8")) > _MAX_INTERNAL_TOKEN_BYTES
        or any(
            ord(character) < _VISIBLE_ASCII_MIN or ord(character) > _VISIBLE_ASCII_MAX
            for character in value
        )
    ):
        _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
    return value


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_from(error_code: str, cause: Exception) -> NoReturn:
    try:
        _reject(error_code)
    except PermanentJobError as error:
        raise error from cause


def _reject_retryable(error_code: str) -> NoReturn:
    raise RetryableJobError(
        stable_error_code(error_code),
        retry_after_seconds=GO_SOURCE_OBJECT_GATEWAY_RETRY_AFTER_SECONDS,
    )
