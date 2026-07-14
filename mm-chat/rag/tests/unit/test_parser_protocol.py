"""Exact-byte MMCP v1 request, response, binding, and outcome tests."""

from __future__ import annotations

import hashlib
import json
import struct
import time
from typing import cast

import pytest

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.protocol import (
    FrameType,
    ProtocolError,
    RequestHeader,
    ResponseHeader,
    build_request_header,
    decode_request,
    decode_response,
    encode_frame,
)

_PREFIX = struct.Struct(">4sBBHIQ")


def _request(source: bytes = b"source") -> tuple[RequestHeader, bytes]:
    header = build_request_header(
        invocation_id="protocol-test",
        source=source,
        parser_config_hash=DEFAULT_CONFIG.config_hash,
        deadline_unix_millis=int(time.time() * 1000) + 60_000,
        max_result_bytes=4096,
        declared_mime="text/plain",
        declared_extension=".txt",
    )
    return header, encode_frame(FrameType.REQUEST, header.to_object(), source)


def test_request_round_trip_recomputes_source_and_binding_hashes() -> None:
    source = b"exact source bytes"
    header, frame = _request(source)

    decoded, body = decode_request(
        frame,
        expected_config_hash=DEFAULT_CONFIG.config_hash,
    )

    assert decoded == header
    assert body == source
    assert decoded.expected_source_sha256 == hashlib.sha256(source).hexdigest()
    assert decoded.request_binding_hash != decoded.expected_source_sha256


@pytest.mark.parametrize(
    ("offset", "replacement"),
    [
        (0, b"FAIL"),
        (4, b"\x02"),
        (5, b"\x02"),
        (6, b"\x00\x01"),
    ],
)
def test_prefix_magic_version_type_and_reserved_bits_are_closed(
    offset: int,
    replacement: bytes,
) -> None:
    _header, frame = _request()
    changed = bytearray(frame)
    changed[offset : offset + len(replacement)] = replacement

    with pytest.raises(ProtocolError):
        decode_request(bytes(changed))


@pytest.mark.parametrize(
    "mutation",
    ["short", "trailing", "body_length", "header_length"],
)
def test_short_overlong_and_trailing_frames_fail_closed(mutation: str) -> None:
    _header, frame = _request()
    if mutation == "short":
        changed = frame[:-1]
    elif mutation == "trailing":
        changed = frame + b"x"
    else:
        fields = list(_PREFIX.unpack(frame[: _PREFIX.size]))
        if mutation == "body_length":
            fields[5] += 1
        else:
            fields[4] = 16_385
        changed = _PREFIX.pack(*fields) + frame[_PREFIX.size :]

    with pytest.raises(ProtocolError):
        decode_request(changed)


def test_noncanonical_and_duplicate_request_headers_fail_closed() -> None:
    header, _frame = _request()
    compact = json.dumps(header.to_object(), separators=(",", ":"), sort_keys=True)
    duplicate = compact[:-1] + ',"invocationId":"duplicate"}'
    for raw in (f" {compact}".encode(), duplicate.encode()):
        frame = _PREFIX.pack(b"MMCP", 1, 1, 0, len(raw), 0) + raw
        with pytest.raises(ProtocolError):
            decode_request(frame)


@pytest.mark.parametrize("field", ["source", "binding", "config", "deadline"])
def test_request_integrity_and_deadline_fences_are_independent(field: str) -> None:
    header, frame = _request()
    expected_config = DEFAULT_CONFIG.config_hash
    if field == "source":
        changed = frame[:-1] + bytes([frame[-1] ^ 1])
    else:
        value = header.to_object()
        if field == "binding":
            value["requestBindingHash"] = "0" * 64
        elif field == "config":
            expected_config = "f" * 64
        else:
            value["deadlineUnixMillis"] = 0
        changed = encode_frame(FrameType.REQUEST, value, b"source")

    with pytest.raises(ProtocolError):
        decode_request(changed, expected_config_hash=expected_config)


def test_response_success_and_failure_discriminators_are_exact() -> None:
    body = b'{"schemaVersion":"canonical-ir.v2"}'
    success = ResponseHeader.success("response-test", body)
    failure = ResponseHeader.failure("response-test", StableErrorCode.INPUT_INVALID)
    route = ResponseHeader.failure("response-test", StableErrorCode.MINERU_REQUIRED)

    observed_success, observed_body = decode_response(
        encode_frame(FrameType.RESPONSE, success.to_object(), body),
        invocation_id="response-test",
    )
    observed_failure, _ = decode_response(
        encode_frame(FrameType.RESPONSE, failure.to_object(), b""),
        invocation_id="response-test",
    )
    observed_route, _ = decode_response(
        encode_frame(FrameType.RESPONSE, route.to_object(), b""),
        invocation_id="response-test",
    )

    assert observed_success.outcome == "success"
    assert observed_body == body
    assert observed_failure.outcome == "failure"
    assert observed_route.outcome == "route_required"


@pytest.mark.parametrize(
    "code",
    [
        StableErrorCode.PARSER_CANCELLED,
        StableErrorCode.PARSER_SANDBOX_UNAVAILABLE,
    ],
)
def test_controller_synthesized_errors_are_forbidden_on_wire(
    code: StableErrorCode,
) -> None:
    with pytest.raises(ProtocolError, match="controller-only"):
        ResponseHeader.failure("response-test", code)


def test_response_invocation_hash_and_body_combinations_fail_closed() -> None:
    body = b"candidate"
    success = ResponseHeader.success("response-test", body)
    wrong_hash = success.to_object()
    wrong_hash["resultSha256"] = "0" * 64

    with pytest.raises(ProtocolError):
        decode_response(
            encode_frame(FrameType.RESPONSE, wrong_hash, body),
            invocation_id="response-test",
        )
    with pytest.raises(ProtocolError):
        decode_response(
            encode_frame(FrameType.RESPONSE, success.to_object(), body),
            invocation_id="different",
        )


@pytest.mark.parametrize(
    "content",
    [
        b"\xef\xbb\xbf{}",
        b'{"x":"a\x00b"}',
        b"{\xff}",
        b"{",
        b"[]",
        b'{"x":1.5}',
        b'{"x":NaN}',
        b'{"x":9007199254740992}',
        b'{"x":' + b"9" * 5000 + b"}",
        b'{"x":1,"x":2}',
        b'{ "x":1}',
    ],
)
def test_protocol_canonical_json_loader_rejects_non_contract_bytes(
    content: bytes,
) -> None:
    with pytest.raises(CanonicalJsonError):
        load_canonical_json_object(content)


def test_protocol_canonical_json_loader_requires_bytes() -> None:
    with pytest.raises(TypeError):
        load_canonical_json_object(cast("bytes", "{}"))


@pytest.mark.parametrize(
    "value",
    [
        {"x": "\x00"},
        {"x": "\ud800"},
        {"x": 9_007_199_254_740_992},
        {"x": 1.5},
        {1: "x"},
        {"非ascii": "x"},
        {"x": {1, 2}},
    ],
)
def test_protocol_canonical_encoder_rejects_values_outside_narrow_profile(
    value: object,
) -> None:
    with pytest.raises(CanonicalJsonError):
        canonical_json_bytes(cast("JsonObject", value))


def test_protocol_canonical_encoder_walks_nested_lists_and_scalars() -> None:
    assert canonical_json_bytes({"x": [None, True, 1, "ok"]}) == (
        b'{"x":[null,true,1,"ok"]}'
    )


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("invocationId", "bad space"),
        ("parserConfigHash", "x"),
        ("expectedSourceBytes", -1),
        ("deadlineUnixMillis", -1),
        ("maxResultBytes", 0),
        ("declaredMime", "Text/Plain"),
        ("declaredExtension", "txt"),
    ],
)
def test_request_header_shape_and_scalar_constraints_are_closed(
    field: str,
    value: JsonValue,
) -> None:
    header, _frame = _request()
    changed = header.to_object()
    changed[field] = value

    with pytest.raises(ProtocolError):
        RequestHeader.from_object(changed)


def test_request_header_rejects_unknown_missing_and_wrong_scalar_types() -> None:
    header, _frame = _request()
    for changed in (
        {**header.to_object(), "unknown": 1},
        {
            key: value
            for key, value in header.to_object().items()
            if key != "invocationId"
        },
        {**header.to_object(), "expectedSourceBytes": "6"},
    ):
        with pytest.raises(ProtocolError):
            RequestHeader.from_object(changed)


def test_request_body_length_and_explicit_now_fences_fail_closed() -> None:
    header, _frame = _request()
    with pytest.raises(ProtocolError, match="body length"):
        header.validate_body(b"")
    with pytest.raises(ProtocolError, match="deadline"):
        header.validate_body(b"source", now_unix_millis=header.deadline_unix_millis)


@pytest.mark.parametrize(
    "changed",
    [
        {"outcome": "success", "resultBytes": 0},
        {"outcome": "route_required", "stableErrorCode": "INPUT_INVALID"},
        {"outcome": "failure", "stableErrorCode": "MINERU_REQUIRED"},
        {"outcome": "unknown"},
    ],
)
def test_response_discriminator_rejects_every_forbidden_combination(
    changed: dict[str, object],
) -> None:
    base = ResponseHeader.failure("response-test", StableErrorCode.INPUT_INVALID)
    value = base.to_object()
    value.update(cast("JsonObject", changed))
    with pytest.raises(ProtocolError):
        ResponseHeader.from_object(value)


def test_response_shape_scalar_error_and_nonempty_failure_body_are_rejected() -> None:
    base = ResponseHeader.failure("response-test", StableErrorCode.INPUT_INVALID)
    missing = base.to_object()
    del missing["outcome"]
    bad_invocation = {**base.to_object(), "invocationId": "bad space"}
    bad_code = {**base.to_object(), "stableErrorCode": "UNKNOWN"}
    bad_type = {**base.to_object(), "resultSha256": 1}
    for value in (missing, bad_invocation, bad_code, bad_type):
        with pytest.raises(ProtocolError):
            ResponseHeader.from_object(value)
    with pytest.raises(ProtocolError, match="body length"):
        base.validate_body(b"x", invocation_id="response-test")
    with pytest.raises(ProtocolError, match="non-empty"):
        ResponseHeader.success("response-test", b"")


def test_frame_decoder_rejects_unknown_type_body_limit_and_empty_header() -> None:
    _header, frame = _request()
    fields = list(_PREFIX.unpack(frame[: _PREFIX.size]))
    for index, value in ((2, 99), (4, 0), (5, 52_428_801)):
        changed_fields = fields.copy()
        changed_fields[index] = value
        changed = _PREFIX.pack(*changed_fields) + frame[_PREFIX.size :]
        with pytest.raises(ProtocolError):
            decode_request(changed)
