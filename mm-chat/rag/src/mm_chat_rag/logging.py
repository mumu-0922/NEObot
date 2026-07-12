"""Structured JSON logging with recursive secret redaction."""

from __future__ import annotations

import json
import logging
import re
import sys
from collections.abc import Mapping
from datetime import UTC, datetime
from typing import Final, override
from urllib.parse import urlsplit, urlunsplit

_SECRET_KEY_RE: Final = re.compile(
    r"(?:authorization|cookie|credential|password|secret|token|api[_-]?key|"
    r"database_url|redis_url|object_key|payload|query|content|body)",
    re.IGNORECASE,
)
_URL_RE: Final = re.compile(r"^[a-z][a-z0-9+.-]*://", re.IGNORECASE)
_INLINE_SECRET_RE: Final = re.compile(
    r"(?i)\b(authorization|password|secret|token|api[_-]?key)=([^\s,;]+)"
)
_MAX_VALUE_LENGTH: Final = 512


def _redact_url(value: str) -> str:
    try:
        parts = urlsplit(value)
    except ValueError:
        return "[REDACTED]"
    if not parts.scheme or not parts.netloc:
        return value
    host = parts.hostname or ""
    try:
        port = parts.port
    except ValueError:
        return "[REDACTED]"
    if port is not None:
        host = f"{host}:{port}"
    return urlunsplit((parts.scheme, host, parts.path, "", ""))


def redact(value: object, key: str = "") -> object:
    """Recursively redact sensitive keys, credentials, and oversized values."""
    if _SECRET_KEY_RE.search(key):
        return "[REDACTED]"
    if isinstance(value, Mapping):
        return {
            str(item_key): redact(item, str(item_key))
            for item_key, item in value.items()
        }
    if isinstance(value, (list, tuple)):
        return [redact(item) for item in value]
    if isinstance(value, str):
        safe = _redact_url(value) if _URL_RE.match(value) else value
        safe = _INLINE_SECRET_RE.sub(r"\1=[REDACTED]", safe)
        return safe if len(safe) <= _MAX_VALUE_LENGTH else f"{safe[:64]}...[TRUNCATED]"
    if value is None or isinstance(value, (bool, int, float)):
        return value
    return str(value)[:_MAX_VALUE_LENGTH]


class RedactedJsonFormatter(logging.Formatter):
    """Emit one bounded JSON object per log record."""

    @override
    def format(self, record: logging.LogRecord) -> str:
        event: dict[str, object] = {
            "timestamp": datetime.now(tz=UTC).isoformat(timespec="milliseconds"),
            "level": record.levelname,
            "logger": record.name,
            "event": redact(record.getMessage()),
        }
        fields = getattr(record, "fields", None)
        if isinstance(fields, Mapping):
            event["fields"] = redact(fields)
        if record.exc_info and record.exc_info[0] is not None:
            event["error_type"] = record.exc_info[0].__name__
        return json.dumps(event, separators=(",", ":"), ensure_ascii=False)


def configure_logging(level: str) -> None:
    """Replace root handlers with the redacting JSON formatter."""
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(RedactedJsonFormatter())
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(handler)
    root.setLevel(level)
