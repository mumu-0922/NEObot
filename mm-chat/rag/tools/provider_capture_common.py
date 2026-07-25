"""Shared fail-closed primitives for the Provider Capture Harness."""

from __future__ import annotations

import hashlib
import ipaddress
import json
import math
import re
from collections.abc import Mapping
from datetime import UTC, datetime
from typing import Final, cast
from urllib.parse import urlsplit

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

MINERU_HOST: Final = "mineru.net"
MINERU_BATCH_URL: Final = f"https://{MINERU_HOST}/api/v4/file-urls/batch"
# Historical evidence decoding constants. They do not authorize network I/O.
JINA_MODEL: Final = "jina-embeddings-v4"
JINA_RERANK_MODEL: Final = "jina-reranker-v3"
SYNTHETIC_PDF_NAME: Final = "mm-chat-synthetic-capture.pdf"
MAX_RESPONSE_BYTES: Final = 1_048_576
MAX_SAFE_INTEGER: Final = (1 << 53) - 1
HTTP_OK: Final = 200
JINA_CALL_COUNT: Final = 3
# Historical evidence shape only; no Jina target is network-allowlisted.
RERANK_DOCUMENT_COUNT: Final = 2
OUTPUT_FILE: Final = "provider-capture-evidence.json"
OUTPUT_DIR_RE: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$")
SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
CAPTURE_PROXY_ENV: Final = "PROVIDER_CAPTURE_PROXY_URL"
_MAX_PROXY_URL_LENGTH: Final = 256
_PRIVATE_IPV4_NETWORKS: Final = tuple(
    ipaddress.ip_network(network)
    for network in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
)
_UNIQUE_LOCAL_IPV6_NETWORK: Final = ipaddress.ip_network("fc00::/7")

_ALLOWED_TARGETS: Final = frozenset(
    {
        ("POST", MINERU_HOST, 443, "/api/v4/file-urls/batch"),
    }
)
_ERROR_CODES: Final = frozenset(
    {
        "CANONICAL_JSON_INVALID",
        "CAPTURE_CREDENTIALS_MISSING_OR_INVALID",
        "CAPTURE_FAILED",
        "CAPTURE_PROXY_INVALID",
        "CLI_ARGUMENT_INVALID",
        "EVIDENCE_SCHEMA_INVALID",
        "EVIDENCE_TARGET_EXISTS",
        "EVIDENCE_WRITE_FAILED",
        "MINERU_SUBMIT_SHAPE_INVALID",
        "MINERU_ARCHIVE_INVALID",
        "MINERU_LIFECYCLE_SHAPE_INVALID",
        "MINERU_POLL_SHAPE_INVALID",
        "MINERU_RESULT_TARGET_INVALID",
        "MINERU_UPLOAD_TARGET_INVALID",
        "OBSERVED_AT_INVALID",
        "OUTPUT_DIRECTORY_INVALID",
        "OUTPUT_PARENT_INVALID",
        "PROVIDER_CONTENT_ENCODING_INVALID",
        "PROVIDER_CONTENT_LENGTH_INVALID",
        "PROVIDER_CONTENT_TYPE_INVALID",
        "PROVIDER_JSON_INVALID",
        "PROVIDER_RESPONSE_LOST",
        "PROVIDER_RESPONSE_TOO_LARGE",
        "PROVIDER_ARCHIVE_TOO_LARGE",
        "PROVIDER_SELECTION_INVALID",
        "PROVIDER_STATUS_INVALID",
        "REDIRECT_FORBIDDEN",
        "TARGET_NOT_ALLOWLISTED",
    }
)


class CaptureError(RuntimeError):
    """A stable capture failure that cannot retain arbitrary exception text."""

    def __init__(self, code: str) -> None:
        super().__init__(code if code in _ERROR_CODES else "CAPTURE_FAILED")


def canonical_json_bytes(value: JsonValue) -> bytes:
    """Return the harness v1 canonical UTF-8 JSON representation."""
    _validate_json_value(value)
    try:
        encoded = json.dumps(
            value,
            ensure_ascii=False,
            allow_nan=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")
    except (TypeError, ValueError, UnicodeError) as error:
        raise CaptureError("CANONICAL_JSON_INVALID") from error
    return encoded + b"\n"


def evidence_sha256(snapshot_bytes: bytes) -> str:
    """Hash exact canonical evidence bytes."""
    return hashlib.sha256(snapshot_bytes).hexdigest()


def deterministic_synthetic_pdf() -> bytes:
    """Build a deterministic one-page PDF without reading any file."""
    stream = (
        b"BT /F1 11 Tf 72 720 Td "
        b"(MM Chat deterministic synthetic provider capture.) Tj ET\n"
    )
    objects = (
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        (
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
            b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
        ),
        b"<< /Length "
        + str(len(stream)).encode("ascii")
        + b" >>\nstream\n"
        + stream
        + b"endstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    )
    result = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = [0]
    for index, body in enumerate(objects, start=1):
        offsets.append(len(result))
        result.extend(f"{index} 0 obj\n".encode())
        result.extend(body)
        result.extend(b"\nendobj\n")
    xref_offset = len(result)
    result.extend(f"xref\n0 {len(objects) + 1}\n".encode())
    result.extend(b"0000000000 65535 f \n")
    for offset in offsets[1:]:
        result.extend(f"{offset:010d} 00000 n \n".encode())
    result.extend(
        (
            f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\n"
            f"startxref\n{xref_offset}\n%%EOF\n"
        ).encode()
    )
    return bytes(result)


def validate_request_target(method: str, url: str) -> None:
    """Enforce exact HTTPS host, port, and path allowlisting."""
    try:
        parsed = urlsplit(url)
        port = parsed.port or 443
    except ValueError as error:
        raise CaptureError("TARGET_NOT_ALLOWLISTED") from error
    target = (method.upper(), parsed.hostname or "", port, parsed.path)
    if (
        parsed.scheme != "https"
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
        or target not in _ALLOWED_TARGETS
    ):
        raise CaptureError("TARGET_NOT_ALLOWLISTED")


def validate_capture_proxy_url(value: str | None) -> str | None:
    """Validate and canonicalize the explicit private HTTP proxy URL."""
    if value is None or value == "":
        return None
    if value != value.strip() or len(value) > _MAX_PROXY_URL_LENGTH:
        raise CaptureError("CAPTURE_PROXY_INVALID")
    try:
        parsed = urlsplit(value)
        host = parsed.hostname
        port = parsed.port
        address = ipaddress.ip_address(host or "")
    except (ValueError, UnicodeError):
        raise CaptureError("CAPTURE_PROXY_INVALID") from None
    if (
        parsed.scheme != "http"
        or host is None
        or port is None
        or port == 0
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
        or not _is_allowed_proxy_address(address)
    ):
        raise CaptureError("CAPTURE_PROXY_INVALID")
    canonical_host = (
        f"[{address.compressed}]"
        if isinstance(address, ipaddress.IPv6Address)
        else address.compressed
    )
    return f"http://{canonical_host}:{port}"


def _is_allowed_proxy_address(
    address: ipaddress.IPv4Address | ipaddress.IPv6Address,
) -> bool:
    if address.is_loopback:
        return True
    if isinstance(address, ipaddress.IPv4Address):
        return any(address in network for network in _PRIVATE_IPV4_NETWORKS)
    return address in _UNIQUE_LOCAL_IPV6_NETWORK


def strict_json_object(content: bytes) -> JsonObject:
    """Decode one strict JSON object without duplicate keys or non-finite values."""
    try:
        text = content.decode("utf-8", errors="strict")
        parsed = json.loads(
            text,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_constant,
            parse_float=_parse_float,
            parse_int=_parse_int,
        )
    except (UnicodeError, json.JSONDecodeError, CaptureError):
        raise CaptureError("PROVIDER_JSON_INVALID") from None
    result = json_object(parsed, "PROVIDER_JSON_INVALID")
    _validate_json_value(result)
    return result


def request_hash(body: JsonObject) -> str:
    """Hash one fixed synthetic request body."""
    content = canonical_json_bytes(body).removesuffix(b"\n")
    return hashlib.sha256(content).hexdigest()


def selected_providers(provider: str) -> tuple[str, ...]:
    """Return the fixed provider subset."""
    if provider == "all":
        return ("mineru",)
    if provider == "mineru":
        return (provider,)
    raise CaptureError("PROVIDER_SELECTION_INVALID")


def parse_observed_at(value: str) -> datetime:
    """Parse an aware timestamp for canonical UTC evidence."""
    try:
        parsed = datetime.fromisoformat(value)
    except ValueError as error:
        raise CaptureError("OBSERVED_AT_INVALID") from error
    if parsed.tzinfo is None:
        raise CaptureError("OBSERVED_AT_INVALID")
    return parsed


def format_observed_at(value: datetime) -> str:
    """Format an aware timestamp to whole-second RFC3339 UTC."""
    if value.tzinfo is None:
        raise CaptureError("OBSERVED_AT_INVALID")
    return (
        value.astimezone(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    )


def require_exact_keys(value: Mapping[str, object], expected: set[str]) -> None:
    """Reject open or incomplete evidence objects."""
    if set(value) != expected:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def is_sha256(value: object) -> bool:
    """Return whether a value is one lowercase SHA-256 hex digest."""
    return isinstance(value, str) and SHA256_RE.fullmatch(value) is not None


def is_nonnegative_int(value: object) -> bool:
    """Return whether a value is a non-boolean nonnegative integer."""
    return not isinstance(value, bool) and isinstance(value, int) and value >= 0


def is_positive_int(value: object) -> bool:
    """Return whether a value is a non-boolean positive integer."""
    return not isinstance(value, bool) and isinstance(value, int) and value > 0


def finite_number(value: object) -> bool:
    """Return whether a value is a finite non-boolean JSON number."""
    return (
        not isinstance(value, bool)
        and isinstance(value, (int, float))
        and math.isfinite(value)
    )


def json_object(value: object, code: str) -> JsonObject:
    """Require one string-keyed JSON object."""
    if not isinstance(value, dict) or any(not isinstance(key, str) for key in value):
        raise CaptureError(code)
    return cast("JsonObject", value)


def json_list(value: object, code: str) -> list[JsonValue]:
    """Require one JSON array."""
    if not isinstance(value, list):
        raise CaptureError(code)
    return cast("list[JsonValue]", value)


def nonnegative_int(value: object, code: str) -> int:
    """Require one nonnegative JSON integer."""
    if not is_nonnegative_int(value):
        raise CaptureError(code)
    return cast("int", value)


def _validate_json_value(value: object) -> None:
    if value is None or isinstance(value, (bool, int, float, str)):
        _validate_json_scalar(value)
        return
    if isinstance(value, list):
        for item in value:
            _validate_json_value(item)
        return
    if isinstance(value, dict):
        _validate_json_object(value)
        return
    raise CaptureError("CANONICAL_JSON_INVALID")


def _validate_json_scalar(value: None | bool | float | str) -> None:
    if isinstance(value, int) and not isinstance(value, bool):
        if abs(value) > MAX_SAFE_INTEGER:
            raise CaptureError("CANONICAL_JSON_INVALID")
    elif isinstance(value, float):
        if not math.isfinite(value):
            raise CaptureError("CANONICAL_JSON_INVALID")
    elif isinstance(value, str):
        _validate_json_string(value)


def _validate_json_string(value: str) -> None:
    try:
        value.encode("utf-8", errors="strict")
    except UnicodeError as error:
        raise CaptureError("CANONICAL_JSON_INVALID") from error


def _validate_json_object(value: dict[object, object]) -> None:
    for key, item in value.items():
        if not isinstance(key, str):
            raise CaptureError("CANONICAL_JSON_INVALID")
        _validate_json_string(key)
        _validate_json_value(item)


def _unique_object(pairs: list[tuple[str, JsonValue]]) -> JsonObject:
    result: JsonObject = {}
    for key, value in pairs:
        if key in result:
            raise CaptureError("PROVIDER_JSON_INVALID")
        result[key] = value
    return result


def _reject_constant(_: str) -> None:
    raise CaptureError("PROVIDER_JSON_INVALID")


def _parse_float(value: str) -> float:
    parsed = float(value)
    if not math.isfinite(parsed):
        raise CaptureError("PROVIDER_JSON_INVALID")
    return parsed


def _parse_int(value: str) -> int:
    parsed = int(value)
    if abs(parsed) > MAX_SAFE_INTEGER:
        raise CaptureError("PROVIDER_JSON_INVALID")
    return parsed
