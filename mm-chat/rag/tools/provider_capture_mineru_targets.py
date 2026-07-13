"""Exact dynamic and batch-target gates for MinerU lifecycle capture."""

from __future__ import annotations

import re
from typing import Final
from urllib.parse import SplitResult, urlsplit

from tools.provider_capture_common import CaptureError

UPLOAD_HOST: Final = "mineru.oss-cn-shanghai.aliyuncs.com"
RESULT_HOST: Final = "cdn-mineru.openxlab.org.cn"
_MAX_DYNAMIC_URL_BYTES: Final = 4096
_HTTPS_PORT: Final = 443
_VISIBLE_URL_MIN: Final = 0x21
_DELETE: Final = 0x7F
_BATCH_ID_RE: Final = re.compile(r"^[A-Za-z0-9._-]{1,128}$")
_POLL_PATH_PREFIX: Final = "/api/v4/extract-results/batch/"


def valid_batch_id(value: object) -> bool:
    """Return whether Provider batch identity can safely enter one path segment."""
    return isinstance(value, str) and _BATCH_ID_RE.fullmatch(value) is not None


def poll_url(batch_id: str) -> str:
    """Construct the only authorized dynamic Poll target."""
    if not valid_batch_id(batch_id):
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    return f"https://mineru.net{_POLL_PATH_PREFIX}{batch_id}"


def validate_poll_target(url: str) -> None:
    """Reject authority, encoding, query, fragment, and batch-segment drift."""
    try:
        parsed = urlsplit(url)
        port = parsed.port or _HTTPS_PORT
    except (UnicodeError, ValueError):
        raise CaptureError("TARGET_NOT_ALLOWLISTED") from None
    batch_id = parsed.path.removeprefix(_POLL_PATH_PREFIX)
    if (
        parsed.scheme != "https"
        or parsed.hostname != "mineru.net"
        or port != _HTTPS_PORT
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
        or not parsed.path.startswith(_POLL_PATH_PREFIX)
        or not valid_batch_id(batch_id)
    ):
        raise CaptureError("TARGET_NOT_ALLOWLISTED")


def validate_upload_target(url: str) -> None:
    """Accept only the documented signed OSS upload authority and path."""
    parsed = _dynamic_url(url, "MINERU_UPLOAD_TARGET_INVALID")
    if (
        parsed.hostname != UPLOAD_HOST
        or not parsed.path.startswith("/api-upload/")
        or not _safe_dynamic_path(parsed.path)
        or not parsed.query
    ):
        raise CaptureError("MINERU_UPLOAD_TARGET_INVALID")


def validate_result_target(url: str) -> None:
    """Accept only the documented MinerU result authority and ZIP path."""
    parsed = _dynamic_url(url, "MINERU_RESULT_TARGET_INVALID")
    if (
        parsed.hostname != RESULT_HOST
        or not parsed.path.startswith("/pdf/")
        or not parsed.path.endswith(".zip")
        or not _safe_dynamic_path(parsed.path)
        or parsed.query
    ):
        raise CaptureError("MINERU_RESULT_TARGET_INVALID")


def _dynamic_url(url: str, code: str) -> SplitResult:
    try:
        encoded = url.encode("utf-8", errors="strict")
    except UnicodeError:
        raise CaptureError(code) from None
    if len(encoded) > _MAX_DYNAMIC_URL_BYTES or any(
        ord(character) < _VISIBLE_URL_MIN or ord(character) == _DELETE
        for character in url
    ):
        raise CaptureError(code)
    try:
        parsed = urlsplit(url)
        port = parsed.port or _HTTPS_PORT
    except (UnicodeError, ValueError):
        raise CaptureError(code) from None
    if (
        parsed.scheme != "https"
        or port != _HTTPS_PORT
        or parsed.username is not None
        or parsed.password is not None
        or parsed.fragment
    ):
        raise CaptureError(code)
    return parsed


def _safe_dynamic_path(path: str) -> bool:
    return (
        bool(path)
        and "%" not in path
        and "\\" not in path
        and all(segment not in {"", ".", ".."} for segment in path.split("/")[1:])
    )


__all__ = [
    "RESULT_HOST",
    "UPLOAD_HOST",
    "poll_url",
    "valid_batch_id",
    "validate_poll_target",
    "validate_result_target",
    "validate_upload_target",
]
