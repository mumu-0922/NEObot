"""Closed Python-to-Go provider operation transport."""

from __future__ import annotations

import json
from typing import Final, NoReturn, cast
from urllib.parse import urlsplit

import httpx

from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

type JsonValue = None | bool | int | float | str | list[JsonValue] | JsonObject
type JsonObject = dict[str, JsonValue]

GO_PROVIDER_GATEWAY_REQUEST_FAILED: Final = "GO_PROVIDER_GATEWAY_REQUEST_FAILED"
GO_PROVIDER_GATEWAY_STATUS_INVALID: Final = "GO_PROVIDER_GATEWAY_STATUS_INVALID"
GO_PROVIDER_GATEWAY_RESPONSE_INVALID: Final = "GO_PROVIDER_GATEWAY_RESPONSE_INVALID"
GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE: Final = "GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE"
GO_PROVIDER_GATEWAY_CONFIG_INVALID: Final = "GO_PROVIDER_GATEWAY_CONFIG_INVALID"
GO_PROVIDER_GATEWAY_TIMEOUT: Final = httpx.Timeout(
    connect=5.0,
    read=40.0,
    write=15.0,
    pool=5.0,
)
GO_PROVIDER_GATEWAY_LIMITS: Final = httpx.Limits(
    max_connections=1,
    max_keepalive_connections=1,
)
GO_PROVIDER_GATEWAY_RETRY_AFTER_SECONDS: Final = 30
GO_PROVIDER_INTERNAL_TOKEN_HEADER: Final = "X-MM-Chat-Internal-Token"  # noqa: S105
MAX_GO_PROVIDER_TOKEN_BYTES: Final = 4096
MAX_GO_PROVIDER_URL_BYTES: Final = 4096
HTTP_OK: Final = 200
_VISIBLE_ASCII_MIN: Final = 33
_VISIBLE_ASCII_MAX: Final = 126
_RETRYABLE_STATUS: Final = frozenset({404, 409, 429, 502, 503, 504})


class GoProviderGateway:
    """Invoke only fixed Go provider operations with the internal token."""

    def __init__(
        self,
        *,
        base_url: str,
        internal_token: str,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self._base_url = _validate_base_url(base_url)
        self._internal_token = _validate_internal_token(internal_token)
        self._client = client

    async def post_json(
        self,
        path: str,
        payload: JsonObject,
        *,
        max_response_bytes: int,
    ) -> JsonObject:
        """POST one code-owned path; callers cannot supply upstream controls."""
        url = _operation_url(self._base_url, path)
        if self._client is not None:
            return await _post_json(
                self._client,
                url,
                self._internal_token,
                payload,
                max_response_bytes,
            )
        async with httpx.AsyncClient(
            timeout=GO_PROVIDER_GATEWAY_TIMEOUT,
            limits=GO_PROVIDER_GATEWAY_LIMITS,
            follow_redirects=False,
            trust_env=False,
        ) as client:
            return await _post_json(
                client,
                url,
                self._internal_token,
                payload,
                max_response_bytes,
            )


async def _post_json(
    client: httpx.AsyncClient,
    url: str,
    internal_token: str,
    payload: JsonObject,
    max_response_bytes: int,
) -> JsonObject:
    if not 1 <= max_response_bytes <= 16 * 1024 * 1024:
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        GO_PROVIDER_INTERNAL_TOKEN_HEADER: internal_token,
        "Content-Type": "application/json",
    }
    try:
        async with client.stream(
            "POST",
            url,
            headers=headers,
            json=payload,
        ) as response:
            if response.status_code != HTTP_OK:
                if response.status_code in _RETRYABLE_STATUS:
                    _reject_retryable(GO_PROVIDER_GATEWAY_STATUS_INVALID)
                _reject_permanent(GO_PROVIDER_GATEWAY_STATUS_INVALID)
            content_type = response.headers.get("content-type", "").split(";", 1)[0]
            if content_type.strip() != "application/json":
                _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_INVALID)
            content_encoding = response.headers.get("content-encoding", "").strip()
            if content_encoding and content_encoding.lower() != "identity":
                _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_INVALID)
            raw = await _read_bounded_response(response, max_response_bytes)
    except PermanentJobError:
        raise
    except RetryableJobError:
        raise
    except (httpx.StreamError, httpx.TransportError):
        _reject_retryable(GO_PROVIDER_GATEWAY_REQUEST_FAILED)
    try:
        decoded = json.loads(raw.decode("utf-8", errors="strict"))
    except (json.JSONDecodeError, UnicodeError):
        _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_INVALID)
    if not isinstance(decoded, dict):
        _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_INVALID)
    return cast("JsonObject", decoded)


async def _read_bounded_response(
    response: httpx.Response,
    max_response_bytes: int,
) -> bytes:
    if response.is_stream_consumed:
        raw = response.content
        if len(raw) > max_response_bytes:
            _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE)
        return raw
    buffered = bytearray()
    async for chunk in response.aiter_raw():
        if len(buffered) + len(chunk) > max_response_bytes:
            _reject_permanent(GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE)
        buffered.extend(chunk)
    return bytes(buffered)


def _validate_base_url(value: str) -> str:
    if not isinstance(value, str) or value != value.strip() or not value:
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    if len(value.encode("utf-8")) > MAX_GO_PROVIDER_URL_BYTES:
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    try:
        parsed = urlsplit(value)
    except ValueError:
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.hostname
        or parsed.username
        or parsed.password
        or parsed.query
        or parsed.fragment
    ):
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    return value.rstrip("/")


def _validate_internal_token(value: str) -> str:
    if (
        not isinstance(value, str)
        or value != value.strip()
        or not value
        or len(value.encode("utf-8")) > MAX_GO_PROVIDER_TOKEN_BYTES
        or any(
            ord(character) < _VISIBLE_ASCII_MIN or ord(character) > _VISIBLE_ASCII_MAX
            for character in value
        )
    ):
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    return value


def _operation_url(base_url: str, path: str) -> str:
    if not path.startswith("/internal/rag/providers/") or any(
        character in path for character in ("?", "#", "\\")
    ):
        _reject_permanent(GO_PROVIDER_GATEWAY_CONFIG_INVALID)
    return f"{base_url}{path}"


def _reject_permanent(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_retryable(error_code: str) -> NoReturn:
    raise RetryableJobError(
        stable_error_code(error_code),
        retry_after_seconds=GO_PROVIDER_GATEWAY_RETRY_AFTER_SECONDS,
    )
