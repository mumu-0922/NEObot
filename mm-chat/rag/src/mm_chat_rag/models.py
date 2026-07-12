"""Strict in-process models for stored-function results."""

from __future__ import annotations

import re
import uuid
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any, Final, Self

_ERROR_CODE_RE: Final = re.compile(r"^[A-Z][A-Z0-9_]{2,63}$")
_STAGE_RE: Final = re.compile(r"^[a-z][a-z0-9_]{0,62}$")


def stable_error_code(value: str) -> str:
    """Validate an operator-safe error code that cannot carry arbitrary text."""
    if not _ERROR_CODE_RE.fullmatch(value):
        raise ValueError("error code must be stable uppercase snake case")
    return value


def _uuid(row: Mapping[str, Any], key: str) -> uuid.UUID:
    value = row.get(key)
    if isinstance(value, uuid.UUID):
        return value
    try:
        return uuid.UUID(str(value))
    except (TypeError, ValueError) as error:
        raise ValueError(f"stored function returned invalid {key}") from error


def _integer(row: Mapping[str, Any], key: str, default: int | None = None) -> int:
    value = row.get(key, default)
    if isinstance(value, bool) or not isinstance(value, int):
        raise TypeError(f"stored function returned invalid {key}")
    return int(value)


def _text(row: Mapping[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not value:
        raise ValueError(f"stored function returned invalid {key}")
    return value


@dataclass(frozen=True, slots=True)
class OutboxClaim:
    """One leased outbox event; the database function claims only one row."""

    outbox_id: int
    event_id: uuid.UUID
    event_type: str
    attempt_count: int
    max_attempts: int
    values: Mapping[str, Any]

    @classmethod
    def from_row(cls, row: Mapping[str, Any]) -> Self:
        """Validate a claim row without interpreting payload as authority."""
        return cls(
            outbox_id=_integer(row, "outbox_id", row.get("id")),
            event_id=_uuid(row, "event_id"),
            event_type=_text(row, "event_type"),
            attempt_count=_integer(row, "attempt_count", 1),
            max_attempts=_integer(row, "max_attempts", 8),
            values=row,
        )


@dataclass(frozen=True, slots=True)
class JobClaim:
    """One leased generation-bound processing job."""

    job_id: uuid.UUID
    stage: str
    attempt_count: int
    max_attempts: int
    values: Mapping[str, Any]

    @classmethod
    def from_row(cls, row: Mapping[str, Any]) -> Self:
        """Validate the function result and reject non-canonical stages."""
        stage = _text(row, "stage")
        if not _STAGE_RE.fullmatch(stage):
            raise ValueError("stored function returned invalid stage")
        return cls(
            job_id=_uuid(row, "job_id" if "job_id" in row else "id"),
            stage=stage,
            attempt_count=_integer(row, "attempt_count", 1),
            max_attempts=_integer(row, "max_attempts", 8),
            values=row,
        )


@dataclass(frozen=True, slots=True)
class FunctionReadiness:
    """Sanitized readiness view returned by the database contract."""

    database: bool
    functions: bool
    consumer: str
    projection: str

    @classmethod
    def from_row(cls, row: Mapping[str, Any]) -> Self:
        """Accept explicit frozen fields while keeping output bounded."""
        detail = row.get("detail")
        detail_values = detail if isinstance(detail, Mapping) else {}
        function_value = row.get(
            "functions_ready",
            row.get("functions", row.get("consumer_ready", False)),
        )
        consumer_value = row.get(
            "consumer_status",
            row.get(
                "consumer",
                detail_values.get(
                    "consumer",
                    "ready" if row.get("consumer_ready") is True else "not_ready",
                ),
            ),
        )
        projection_value = row.get(
            "projection_status",
            row.get(
                "projection",
                detail_values.get(
                    "projection",
                    "ready" if row.get("projection_ready") is True else "not_ready",
                ),
            ),
        )
        functions = function_value is True or function_value in {"ready", "ok"}
        consumer = str(consumer_value)
        projection = str(projection_value)
        allowed_consumer = {"ready", "disabled", "not_ready"}
        allowed_projection = {
            "ready",
            "not_ready",
            "building",
            "catching_up",
            "degraded",
            "retired",
            "failed",
        }
        if consumer not in allowed_consumer:
            consumer = "not_ready"
        if projection not in allowed_projection:
            projection = "not_ready"
        return cls(
            database=True,
            functions=functions,
            consumer=consumer,
            projection=projection,
        )
