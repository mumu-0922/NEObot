"""Narrow standard-library JCS support for closed ASCII protocol headers."""

from __future__ import annotations

import json
from typing import Any, Final, cast

type JsonScalar = None | bool | str | int
type JsonValue = JsonScalar | list[JsonValue] | dict[str, JsonValue]
type JsonObject = dict[str, JsonValue]

MAX_SAFE_INTEGER: Final = (1 << 53) - 1
_MAX_SAFE_INTEGER_DIGITS: Final = len(str(MAX_SAFE_INTEGER))
_SURROGATE_MIN: Final = 0xD800
_SURROGATE_MAX: Final = 0xDFFF
_BOMS: Final = (
    b"\x00\x00\xfe\xff",
    b"\xff\xfe\x00\x00",
    b"\xef\xbb\xbf",
    b"\xfe\xff",
    b"\xff\xfe",
)


class CanonicalJsonError(ValueError):
    """Protocol JSON violates the narrow deterministic profile."""


def canonical_json_bytes(value: JsonValue) -> bytes:
    """Encode the integer-only protocol profile as RFC 8785-equivalent bytes.

    Protocol keys and patterned string values are ASCII. The profile forbids
    floats, so CPython's compact sorted JSON form is byte-equivalent to JCS for
    every value admitted by the request and response validators.
    """
    _validate_value(value, path="$")
    try:
        return json.dumps(
            value,
            ensure_ascii=False,
            allow_nan=False,
            check_circular=True,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    except (TypeError, ValueError, UnicodeEncodeError) as error:
        raise CanonicalJsonError("value cannot be canonicalized") from error


def load_canonical_json_object(content: bytes) -> JsonObject:
    """Load a duplicate-free canonical object from exact UTF-8 bytes."""
    if not isinstance(content, bytes):
        raise TypeError("canonical JSON input must be bytes")
    if any(content.startswith(bom) for bom in _BOMS):
        raise CanonicalJsonError("canonical JSON must not contain a BOM")
    if b"\x00" in content:
        raise CanonicalJsonError("canonical JSON must not contain NUL")
    try:
        text = content.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise CanonicalJsonError("canonical JSON is not strict UTF-8") from error
    try:
        value = json.loads(
            text,
            object_pairs_hook=_object_without_duplicates,
            parse_constant=_reject_constant,
            parse_float=_reject_float,
            parse_int=_parse_safe_integer,
        )
    except json.JSONDecodeError as error:
        raise CanonicalJsonError("bytes are not valid JSON") from error
    if not isinstance(value, dict):
        raise CanonicalJsonError("protocol header must be a JSON object")
    result = cast("JsonObject", value)
    _validate_value(result, path="$")
    if canonical_json_bytes(result) != content:
        raise CanonicalJsonError("protocol header is not canonical JCS")
    return result


def _validate_value(value: object, *, path: str) -> None:
    if value is None or isinstance(value, bool):
        return
    if isinstance(value, str):
        if "\x00" in value or any(
            _SURROGATE_MIN <= ord(char) <= _SURROGATE_MAX for char in value
        ):
            raise CanonicalJsonError(f"{path} is not a Unicode scalar string")
        return
    if type(value) is int:
        if abs(value) > MAX_SAFE_INTEGER:
            raise CanonicalJsonError(f"{path} integer exceeds the safe range")
        return
    if isinstance(value, float):
        raise CanonicalJsonError(f"{path} contains a forbidden float")
    if isinstance(value, list):
        for index, item in enumerate(value):
            _validate_value(item, path=f"{path}[{index}]")
        return
    if isinstance(value, dict):
        for key, item in value.items():
            if not isinstance(key, str):
                raise CanonicalJsonError(f"{path} contains a non-string key")
            if not key.isascii():
                raise CanonicalJsonError(f"{path} contains a non-ASCII protocol key")
            _validate_value(item, path=f"{path}.{key}")
        return
    raise CanonicalJsonError(f"{path} contains an unsupported value")


def _object_without_duplicates(pairs: list[tuple[str, Any]]) -> JsonObject:
    result: JsonObject = {}
    for key, value in pairs:
        if key in result:
            raise CanonicalJsonError("protocol header contains a duplicate key")
        result[key] = cast("JsonValue", value)
    return result


def _reject_constant(_value: str) -> None:
    raise CanonicalJsonError("non-finite JSON constants are forbidden")


def _reject_float(_value: str) -> None:
    raise CanonicalJsonError("float JSON numbers are forbidden")


def _parse_safe_integer(value: str) -> int:
    digits = value.removeprefix("-")
    if len(digits) > _MAX_SAFE_INTEGER_DIGITS:
        raise CanonicalJsonError("JSON integer exceeds the safe range")
    try:
        parsed = int(value, 10)
    except ValueError as error:
        raise CanonicalJsonError("JSON integer is invalid") from error
    if abs(parsed) > MAX_SAFE_INTEGER:
        raise CanonicalJsonError("JSON integer exceeds the safe range")
    return parsed
