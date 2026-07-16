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

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    JOB_HANDLER_SOURCE_INVALID,
    DocumentSource,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError

_SUPPORTED_STORAGE_BACKENDS: Final = frozenset({"local", "minio", "s3"})
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_CONTENT_TYPE_RE: Final = re.compile(r"^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9.+-]{1,64}$")
_MAX_OBJECT_KEY_BYTES: Final = 1024
_MAX_SOURCE_BYTES: Final = 512 * 1024 * 1024
_ZERO_UUID: Final = uuid.UUID(int=0)


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
        metadata: FileSourceMetadata,
    ) -> Coroutine[Any, Any, bytes]: ...


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
        body = await self._objects.fetch_object_bytes(metadata)
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

    async def fetch_object_bytes(self, metadata: FileSourceMetadata) -> bytes:
        """Read bounded local object bytes for already-fenced metadata."""
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


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_from(error_code: str, cause: Exception) -> NoReturn:
    try:
        _reject(error_code)
    except PermanentJobError as error:
        raise error from cause
