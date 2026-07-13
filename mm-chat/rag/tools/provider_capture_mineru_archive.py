"""Bounded non-extracting ZIP validation for synthetic MinerU results."""

from __future__ import annotations

import hashlib
import io
import stat
import zipfile
from pathlib import PurePosixPath
from typing import Final

from tools.provider_capture_common import CaptureError, JsonObject

MAX_ARCHIVE_BYTES: Final = 32 * 1024 * 1024
MAX_ARCHIVE_ENTRIES: Final = 256
MAX_ARCHIVE_ENTRY_BYTES: Final = 64 * 1024 * 1024
MAX_ARCHIVE_TOTAL_BYTES: Final = 128 * 1024 * 1024
MAX_COMPRESSION_RATIO: Final = 200
ARCHIVE_FAILURE_CLASSES: Final = frozenset(
    {
        "compression_ratio_exceeded",
        "crc_mismatch",
        "duplicate_entry",
        "empty_archive",
        "encrypted_entry",
        "expanded_entry_too_large",
        "expanded_total_too_large",
        "invalid_compression_metadata",
        "invalid_zip",
        "missing_content_list",
        "missing_full_markdown",
        "missing_middle_json",
        "missing_model_json",
        "symlink_entry",
        "too_many_entries",
        "unsupported_compression",
        "unsafe_entry_name",
        "unsafe_entry_path",
    }
)


class ArchiveValidationError(CaptureError):
    """A ZIP rejection carrying only one closed, non-sensitive class."""

    def __init__(self, failure_class: str) -> None:
        if failure_class not in ARCHIVE_FAILURE_CLASSES:
            raise CaptureError("CAPTURE_FAILED")
        super().__init__("MINERU_ARCHIVE_INVALID")
        self.failure_class = failure_class


def validate_result_archive(content: bytes) -> JsonObject:
    """Validate one in-memory ZIP without extraction or retained entry names."""
    if not content:
        raise ArchiveValidationError("empty_archive")
    if len(content) > MAX_ARCHIVE_BYTES:
        raise CaptureError("PROVIDER_ARCHIVE_TOO_LARGE")
    entries = _read_valid_entries(content)
    required = _required_artifacts(entries)
    _validate_required_artifacts(required)
    return {
        "archiveByteCount": len(content),
        "archiveSha256": hashlib.sha256(content).hexdigest(),
        "entryCount": len(entries),
        **required,
    }


def _read_valid_entries(content: bytes) -> list[zipfile.ZipInfo]:
    try:
        with zipfile.ZipFile(io.BytesIO(content)) as archive:
            entries = archive.infolist()
            _validate_archive_entries(entries)
            _validate_archive_crc(archive)
            return entries
    except ArchiveValidationError:
        raise
    except NotImplementedError:
        raise ArchiveValidationError("unsupported_compression") from None
    except (OSError, RuntimeError, zipfile.BadZipFile, zipfile.LargeZipFile):
        raise ArchiveValidationError("invalid_zip") from None


def _validate_archive_crc(archive: zipfile.ZipFile) -> None:
    if archive.testzip() is not None:
        raise ArchiveValidationError("crc_mismatch")


def _validate_archive_entries(entries: list[zipfile.ZipInfo]) -> None:
    if not entries:
        raise ArchiveValidationError("empty_archive")
    if len(entries) > MAX_ARCHIVE_ENTRIES:
        raise ArchiveValidationError("too_many_entries")
    total = 0
    seen: set[str] = set()
    for entry in entries:
        _validate_archive_entry(entry, seen)
        seen.add(entry.filename)
        total += entry.file_size
        if total > MAX_ARCHIVE_TOTAL_BYTES:
            raise ArchiveValidationError("expanded_total_too_large")


def _validate_archive_entry(entry: zipfile.ZipInfo, seen: set[str]) -> None:
    name = entry.filename
    path = PurePosixPath(name)
    path_text = name.removesuffix("/")
    raw_parts = path_text.split("/")
    mode = entry.external_attr >> 16
    if not name:
        raise ArchiveValidationError("unsafe_entry_name")
    if (
        "\\" in name
        or path.is_absolute()
        or not path_text
        or any(part in {"", ".", ".."} for part in raw_parts)
    ):
        raise ArchiveValidationError("unsafe_entry_path")
    if name in seen:
        raise ArchiveValidationError("duplicate_entry")
    if entry.flag_bits & 0x1:
        raise ArchiveValidationError("encrypted_entry")
    if stat.S_ISLNK(mode):
        raise ArchiveValidationError("symlink_entry")
    if entry.file_size > MAX_ARCHIVE_ENTRY_BYTES:
        raise ArchiveValidationError("expanded_entry_too_large")
    if entry.file_size and entry.compress_size == 0:
        raise ArchiveValidationError("invalid_compression_metadata")
    if (
        entry.compress_size
        and entry.file_size > entry.compress_size * MAX_COMPRESSION_RATIO
    ):
        raise ArchiveValidationError("compression_ratio_exceeded")


def _required_artifacts(entries: list[zipfile.ZipInfo]) -> JsonObject:
    basenames = [
        PurePosixPath(entry.filename).name for entry in entries if not entry.is_dir()
    ]
    return {
        "contentListPresent": any(
            name == "content_list.json" or name.endswith("_content_list.json")
            for name in basenames
        ),
        "fullMarkdownPresent": "full.md" in basenames,
        "middleJsonPresent": any(
            name == "middle.json" or name.endswith("_middle.json") for name in basenames
        ),
        "modelJsonPresent": any(
            name == "model.json" or name.endswith("_model.json") for name in basenames
        ),
    }


def _validate_required_artifacts(required: JsonObject) -> None:
    failure_by_field = {
        "fullMarkdownPresent": "missing_full_markdown",
        "contentListPresent": "missing_content_list",
        "middleJsonPresent": "missing_middle_json",
        "modelJsonPresent": "missing_model_json",
    }
    for field, failure_class in failure_by_field.items():
        if required[field] is not True:
            raise ArchiveValidationError(failure_class)


__all__ = [
    "ARCHIVE_FAILURE_CLASSES",
    "MAX_ARCHIVE_BYTES",
    "MAX_ARCHIVE_ENTRIES",
    "ArchiveValidationError",
    "validate_result_archive",
]
