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


def validate_result_archive(content: bytes) -> JsonObject:
    """Validate one in-memory ZIP without extraction or retained entry names."""
    if not content or len(content) > MAX_ARCHIVE_BYTES:
        raise CaptureError("PROVIDER_ARCHIVE_TOO_LARGE")
    entries = _read_valid_entries(content)
    required = _required_artifacts(entries)
    if not all(required.values()):
        raise CaptureError("MINERU_ARCHIVE_INVALID")
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
    except CaptureError:
        raise
    except (OSError, RuntimeError, zipfile.BadZipFile, zipfile.LargeZipFile):
        raise CaptureError("MINERU_ARCHIVE_INVALID") from None


def _validate_archive_crc(archive: zipfile.ZipFile) -> None:
    if archive.testzip() is not None:
        raise CaptureError("MINERU_ARCHIVE_INVALID")


def _validate_archive_entries(entries: list[zipfile.ZipInfo]) -> None:
    if not entries or len(entries) > MAX_ARCHIVE_ENTRIES:
        raise CaptureError("MINERU_ARCHIVE_INVALID")
    total = 0
    seen: set[str] = set()
    for entry in entries:
        _validate_archive_entry(entry, seen)
        seen.add(entry.filename)
        total += entry.file_size
        if total > MAX_ARCHIVE_TOTAL_BYTES:
            raise CaptureError("MINERU_ARCHIVE_INVALID")


def _validate_archive_entry(entry: zipfile.ZipInfo, seen: set[str]) -> None:
    name = entry.filename
    path = PurePosixPath(name)
    mode = entry.external_attr >> 16
    unsafe = (
        not name
        or "\\" in name
        or path.is_absolute()
        or "//" in name
        or any(part in {".", ".."} for part in path.parts)
        or name in seen
        or bool(entry.flag_bits & 0x1)
        or stat.S_ISLNK(mode)
        or entry.file_size > MAX_ARCHIVE_ENTRY_BYTES
        or bool(entry.file_size and entry.compress_size == 0)
        or bool(
            entry.compress_size
            and entry.file_size > entry.compress_size * MAX_COMPRESSION_RATIO
        )
    )
    if unsafe:
        raise CaptureError("MINERU_ARCHIVE_INVALID")


def _required_artifacts(entries: list[zipfile.ZipInfo]) -> JsonObject:
    basenames = [PurePosixPath(entry.filename).name for entry in entries]
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


__all__ = [
    "MAX_ARCHIVE_BYTES",
    "MAX_ARCHIVE_ENTRIES",
    "validate_result_archive",
]
