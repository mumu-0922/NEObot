"""Defensive tests for the closed child-internal result envelope."""

from __future__ import annotations

from typing import Any

import pytest

from mm_chat_rag.offline_parser.canonical import canonical_json_bytes
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.internal_result import (
    InternalResultError,
    NativeResultHeader,
)
from mm_chat_rag.offline_parser.native.model import NATIVE_ARTIFACT_SCHEMA_VERSION


def _failure_object() -> dict[str, Any]:
    return dict(NativeResultHeader.failure(StableErrorCode.INPUT_INVALID).to_object())


def _success_object(body: bytes = b"native-artifact") -> dict[str, Any]:
    return dict(NativeResultHeader.success(ParserFormat.TXT, body).to_object())


def _decode_object(value: dict[str, Any]) -> NativeResultHeader:
    return NativeResultHeader.from_bytes(canonical_json_bytes(value))


def test_success_rejects_an_empty_native_artifact() -> None:
    with pytest.raises(InternalResultError, match="non-empty"):
        NativeResultHeader.success(ParserFormat.TXT, b"")


def test_failure_constructor_rejects_controller_only_errors() -> None:
    with pytest.raises(InternalResultError, match="controller-only"):
        NativeResultHeader.failure(StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)


@pytest.mark.parametrize(
    "content",
    [
        b"not-json",
        NativeResultHeader.failure(StableErrorCode.INPUT_INVALID).canonical_bytes
        + b"\n",
    ],
)
def test_header_rejects_noncanonical_wire_bytes(content: bytes) -> None:
    with pytest.raises(InternalResultError, match="not canonical"):
        NativeResultHeader.from_bytes(content)


def test_header_rejects_open_or_incomplete_shapes() -> None:
    extra = _failure_object()
    extra["unexpected"] = None
    missing = _failure_object()
    del missing["format"]

    for value in (extra, missing):
        with pytest.raises(InternalResultError, match="fields are not closed"):
            _decode_object(value)


@pytest.mark.parametrize(
    ("field", "value", "message"),
    [
        ("format", 1, "format is not nullable text"),
        ("stableErrorCode", 1, "stable error is not nullable text"),
        ("outcome", 1, "outcome is not text"),
        (
            "nativeArtifactVersion",
            1,
            "artifact version is not nullable text",
        ),
        ("resultBytes", False, "byte count is not an integer"),
        ("resultSha256", 1, "hash is not nullable text"),
    ],
)
def test_header_rejects_non_closed_field_types(
    field: str,
    value: object,
    message: str,
) -> None:
    header = _failure_object()
    header[field] = value

    with pytest.raises(InternalResultError, match=message):
        _decode_object(header)


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("format", "unknown"),
        ("stableErrorCode", "UNKNOWN_ERROR"),
    ],
)
def test_header_rejects_unknown_enums(field: str, value: str) -> None:
    header = _failure_object()
    header[field] = value

    with pytest.raises(InternalResultError, match="enum is unknown"):
        _decode_object(header)


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("format", ParserFormat.DOCX.value),
        ("nativeArtifactVersion", "parser-native-artifact.v0"),
        ("resultBytes", 0),
        ("resultSha256", None),
        ("resultSha256", "A" * 64),
        ("stableErrorCode", StableErrorCode.INPUT_INVALID.value),
    ],
)
def test_native_success_rejects_every_invalid_discriminator_field(
    field: str,
    value: object,
) -> None:
    header = _success_object()
    header[field] = value

    with pytest.raises(InternalResultError, match="native-success field"):
        _decode_object(header)


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("format", ParserFormat.TXT.value),
        ("nativeArtifactVersion", NATIVE_ARTIFACT_SCHEMA_VERSION),
        ("resultBytes", 1),
        ("resultSha256", "0" * 64),
        ("stableErrorCode", None),
        ("stableErrorCode", StableErrorCode.PARSER_CANCELLED.value),
    ],
)
def test_failure_rejects_every_invalid_discriminator_field(
    field: str,
    value: object,
) -> None:
    header = _failure_object()
    header[field] = value

    with pytest.raises(InternalResultError, match="native-failure field"):
        _decode_object(header)


def test_header_rejects_an_unknown_outcome() -> None:
    header = _failure_object()
    header["outcome"] = "partial"

    with pytest.raises(InternalResultError, match="unknown native result outcome"):
        _decode_object(header)


def test_success_body_is_bound_to_declared_length_limit_and_hash() -> None:
    body = b"native-artifact"
    header = NativeResultHeader.from_bytes(
        NativeResultHeader.success(ParserFormat.HTML, body).canonical_bytes
    )

    header.validate_body(body, body_limit=len(body))
    with pytest.raises(InternalResultError, match="body length"):
        header.validate_body(body, body_limit=len(body) - 1)
    with pytest.raises(InternalResultError, match="body length"):
        header.validate_body(body[:-1], body_limit=len(body))
    with pytest.raises(InternalResultError, match="body hash"):
        header.validate_body(body[:-1] + b"x", body_limit=len(body))


def test_failure_round_trip_has_a_zero_body_and_no_artifact_metadata() -> None:
    header = NativeResultHeader.from_bytes(
        NativeResultHeader.failure(
            StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED
        ).canonical_bytes
    )

    header.validate_body(b"", body_limit=0)
    assert header.to_object() == {
        "format": None,
        "nativeArtifactVersion": None,
        "outcome": "failure",
        "resultBytes": 0,
        "resultSha256": None,
        "stableErrorCode": "ACTIVE_CONTENT_UNSUPPORTED",
    }
