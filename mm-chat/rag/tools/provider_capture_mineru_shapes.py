"""Closed Allocate and Poll response shapes for MinerU lifecycle capture."""

from __future__ import annotations

import re
from collections.abc import Mapping
from typing import Final, cast

from tools.provider_capture_common import (
    SYNTHETIC_PDF_NAME,
    CaptureError,
    JsonObject,
    is_nonnegative_int,
    json_list,
    json_object,
)
from tools.provider_capture_mineru_targets import valid_batch_id

POLL_STATES: Final = (
    "waiting-file",
    "pending",
    "running",
    "converting",
    "done",
    "failed",
)
CONTINUE_STATES: Final = frozenset(POLL_STATES[:4])
_DATA_ID_RE: Final = re.compile(r"^[A-Za-z0-9._-]{1,128}$")
_START_TIME_RE: Final = re.compile(r"^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$")


def empty_state_counts() -> JsonObject:
    """Return the complete fixed Poll state counter shape."""
    return dict.fromkeys(POLL_STATES, 0)


def parse_allocate_response(payload: JsonObject) -> tuple[str, str]:
    """Return validated ephemeral Batch ID and Signed Upload URL."""
    _closed_fields(payload, {"code", "data", "msg"}, {"trace_id"})
    if (
        payload["code"] != 0
        or not isinstance(payload["msg"], str)
        or not payload["msg"]
        or ("trace_id" in payload and not isinstance(payload["trace_id"], str))
    ):
        raise CaptureError("MINERU_LIFECYCLE_SHAPE_INVALID")
    data = json_object(payload["data"], "MINERU_LIFECYCLE_SHAPE_INVALID")
    _closed_fields(data, {"batch_id", "file_urls"}, set())
    batch_id = data["batch_id"]
    urls = json_list(data["file_urls"], "MINERU_LIFECYCLE_SHAPE_INVALID")
    if not valid_batch_id(batch_id) or len(urls) != 1 or not isinstance(urls[0], str):
        raise CaptureError("MINERU_LIFECYCLE_SHAPE_INVALID")
    return cast("str", batch_id), urls[0]


def parse_poll_response(
    payload: JsonObject,
    *,
    batch_id: str,
) -> tuple[str, str | None]:
    """Validate one single-file Poll response without retaining dynamic values."""
    _closed_fields(payload, {"code", "data", "msg"}, {"trace_id"})
    if (
        payload["code"] != 0
        or not isinstance(payload["msg"], str)
        or not payload["msg"]
        or ("trace_id" in payload and not isinstance(payload["trace_id"], str))
    ):
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    data = json_object(payload["data"], "MINERU_POLL_SHAPE_INVALID")
    _closed_fields(data, {"batch_id", "extract_result"}, set())
    results = json_list(data["extract_result"], "MINERU_POLL_SHAPE_INVALID")
    if data["batch_id"] != batch_id or len(results) != 1:
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    result = json_object(results[0], "MINERU_POLL_SHAPE_INVALID")
    _closed_fields(
        result,
        {"err_msg", "file_name", "state"},
        {"data_id", "extract_progress", "full_zip_url"},
    )
    state = result["state"]
    if (
        state not in POLL_STATES
        or result["file_name"] != SYNTHETIC_PDF_NAME
        or not isinstance(result["err_msg"], str)
        or not _valid_optional_data_id(result)
    ):
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    if state == "running":
        _validate_progress(result.get("extract_progress"))
    elif "extract_progress" in result:
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    if state == "done":
        result_url = result.get("full_zip_url")
        if not isinstance(result_url, str) or not result_url:
            raise CaptureError("MINERU_POLL_SHAPE_INVALID")
        return cast("str", state), result_url
    if "full_zip_url" in result or (state == "failed" and not result["err_msg"]):
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")
    return cast("str", state), None


def _valid_optional_data_id(result: JsonObject) -> bool:
    if "data_id" not in result:
        return True
    value = result["data_id"]
    return isinstance(value, str) and _DATA_ID_RE.fullmatch(value) is not None


def _validate_progress(value: object) -> None:
    progress = json_object(value, "MINERU_POLL_SHAPE_INVALID")
    _closed_fields(
        progress,
        {"extracted_pages", "start_time", "total_pages"},
        set(),
    )
    extracted = progress["extracted_pages"]
    total = progress["total_pages"]
    if (
        not is_nonnegative_int(extracted)
        or not is_nonnegative_int(total)
        or cast("int", total) == 0
        or cast("int", extracted) > cast("int", total)
        or not isinstance(progress["start_time"], str)
        or _START_TIME_RE.fullmatch(progress["start_time"]) is None
    ):
        raise CaptureError("MINERU_POLL_SHAPE_INVALID")


def _closed_fields(
    value: Mapping[str, object],
    required: set[str],
    optional: set[str],
) -> None:
    if not required <= set(value) or not set(value) <= required | optional:
        raise CaptureError("MINERU_LIFECYCLE_SHAPE_INVALID")


__all__ = [
    "CONTINUE_STATES",
    "POLL_STATES",
    "empty_state_counts",
    "parse_allocate_response",
    "parse_poll_response",
]
