"""MMCP v1 framed request/response protocol and outcome gates."""

from __future__ import annotations

import hashlib
import re
import struct
import time
from dataclasses import dataclass
from enum import IntEnum
from typing import Final, Self

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    JsonObject,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import (
    CONTROLLER_ONLY_ERRORS,
    WIRE_FAILURE_ERRORS,
    StableErrorCode,
)

MAGIC: Final = b"MMCP"
PROTOCOL_MAJOR: Final = 1
MAX_HEADER_BYTES: Final = 16_384
MAX_REQUEST_BODY_BYTES: Final = 52_428_800
MAX_RESULT_BYTES: Final = 67_108_864
_PREFIX: Final = struct.Struct(">4sBBHIQ")
_BINDING_DOMAIN: Final = b"mm-chat.parser-request-binding.v1\n"
_INVOCATION_RE: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
_MIME_RE: Final = re.compile(r"^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$")
_EXTENSION_RE: Final = re.compile(r"^\.[a-z0-9][a-z0-9._+-]{0,30}$")
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")


class FrameType(IntEnum):
    """Closed MMCP frame-type byte."""

    REQUEST = 1
    RESPONSE = 2


class ProtocolError(ValueError):
    """A frame or header violates MMCP v1."""


@dataclass(frozen=True, slots=True)
class Frame:
    """One decoded exact-length MMCP frame."""

    frame_type: FrameType
    header: JsonObject
    body: bytes


@dataclass(frozen=True, slots=True)
class RequestHeader:
    """Validated parser-protocol-request-header.v1."""

    invocation_id: str
    parser_config_hash: str
    expected_source_bytes: int
    expected_source_sha256: str
    request_binding_hash: str
    deadline_unix_millis: int
    max_result_bytes: int
    declared_mime: str | None = None
    declared_extension: str | None = None

    def to_object(self) -> JsonObject:
        """Return the exact schema-shaped header object."""
        value: JsonObject = {
            "deadlineUnixMillis": self.deadline_unix_millis,
            "expectedSourceBytes": self.expected_source_bytes,
            "expectedSourceSha256": self.expected_source_sha256,
            "invocationId": self.invocation_id,
            "maxResultBytes": self.max_result_bytes,
            "parserConfigHash": self.parser_config_hash,
            "requestBindingHash": self.request_binding_hash,
        }
        if self.declared_extension is not None:
            value["declaredExtension"] = self.declared_extension
        if self.declared_mime is not None:
            value["declaredMime"] = self.declared_mime
        return value

    @classmethod
    def from_object(cls, value: JsonObject) -> Self:
        """Validate the closed request shape without a runtime schema library."""
        required = {
            "invocationId",
            "parserConfigHash",
            "expectedSourceBytes",
            "expectedSourceSha256",
            "requestBindingHash",
            "deadlineUnixMillis",
            "maxResultBytes",
        }
        optional = {"declaredMime", "declaredExtension"}
        if set(value) - required - optional or not required.issubset(value):
            raise ProtocolError("request header fields are not closed")
        invocation_id = _text(value, "invocationId")
        parser_config_hash = _text(value, "parserConfigHash")
        source_hash = _text(value, "expectedSourceSha256")
        binding_hash = _text(value, "requestBindingHash")
        source_bytes = _integer(value, "expectedSourceBytes")
        deadline = _integer(value, "deadlineUnixMillis")
        max_result = _integer(value, "maxResultBytes")
        mime = _optional_text(value, "declaredMime")
        extension = _optional_text(value, "declaredExtension")
        if not _INVOCATION_RE.fullmatch(invocation_id):
            raise ProtocolError("invalid invocationId")
        if not all(
            _SHA256_RE.fullmatch(item)
            for item in (parser_config_hash, source_hash, binding_hash)
        ):
            raise ProtocolError("invalid SHA-256 header field")
        if source_bytes < 0 or source_bytes > MAX_REQUEST_BODY_BYTES:
            raise ProtocolError("expectedSourceBytes exceeds the protocol limit")
        if deadline < 0:
            raise ProtocolError("deadlineUnixMillis must be non-negative")
        if max_result < 1 or max_result > MAX_RESULT_BYTES:
            raise ProtocolError("maxResultBytes exceeds the protocol limit")
        if mime is not None and not _MIME_RE.fullmatch(mime):
            raise ProtocolError("declaredMime is not canonical")
        if extension is not None and not _EXTENSION_RE.fullmatch(extension):
            raise ProtocolError("declaredExtension is not canonical")
        return cls(
            invocation_id=invocation_id,
            parser_config_hash=parser_config_hash,
            expected_source_bytes=source_bytes,
            expected_source_sha256=source_hash,
            request_binding_hash=binding_hash,
            deadline_unix_millis=deadline,
            max_result_bytes=max_result,
            declared_mime=mime,
            declared_extension=extension,
        )

    def validate_body(
        self,
        body: bytes,
        *,
        expected_config_hash: str | None = None,
        now_unix_millis: int | None = None,
    ) -> None:
        """Independently verify source, binding, config, and deadline fences."""
        if len(body) != self.expected_source_bytes:
            raise ProtocolError("request body length does not match its header")
        observed_hash = hashlib.sha256(body).hexdigest()
        if observed_hash != self.expected_source_sha256:
            raise ProtocolError("request source hash does not match its header")
        if request_binding_hash(self, body) != self.request_binding_hash:
            raise ProtocolError("requestBindingHash does not match the request")
        if (
            expected_config_hash is not None
            and self.parser_config_hash != expected_config_hash
        ):
            raise ProtocolError("parserConfigHash does not match this sidecar")
        observed_now = (
            int(time.time() * 1000) if now_unix_millis is None else now_unix_millis
        )
        if self.deadline_unix_millis <= observed_now:
            raise ProtocolError("request deadline has expired")


@dataclass(frozen=True, slots=True)
class ResponseHeader:
    """Validated parser-protocol-response-header.v1."""

    invocation_id: str
    outcome: str
    canonical_schema_version: str | None
    result_bytes: int
    result_sha256: str | None
    stable_error_code: StableErrorCode | None

    def to_object(self) -> JsonObject:
        """Return the exact response-schema object."""
        return {
            "canonicalSchemaVersion": self.canonical_schema_version,
            "invocationId": self.invocation_id,
            "outcome": self.outcome,
            "resultBytes": self.result_bytes,
            "resultSha256": self.result_sha256,
            "stableErrorCode": (
                self.stable_error_code.value
                if self.stable_error_code is not None
                else None
            ),
        }

    @classmethod
    def success(cls, invocation_id: str, body: bytes) -> Self:
        """Build a success header for independently validated candidate bytes."""
        if not body:
            raise ProtocolError("success response body must be non-empty")
        return cls(
            invocation_id=invocation_id,
            outcome="success",
            canonical_schema_version="canonical-ir.v2",
            result_bytes=len(body),
            result_sha256=hashlib.sha256(body).hexdigest(),
            stable_error_code=None,
        )

    @classmethod
    def failure(cls, invocation_id: str, code: StableErrorCode) -> Self:
        """Build a legal zero-body wire failure or route-required outcome."""
        if code in CONTROLLER_ONLY_ERRORS:
            raise ProtocolError("controller-only outcomes are forbidden on the wire")
        return cls(
            invocation_id=invocation_id,
            outcome=(
                "route_required"
                if code is StableErrorCode.MINERU_REQUIRED
                else "failure"
            ),
            canonical_schema_version=None,
            result_bytes=0,
            result_sha256=None,
            stable_error_code=code,
        )

    @classmethod
    def from_object(cls, value: JsonObject) -> Self:
        """Validate the exact discriminated response shape."""
        fields = {
            "invocationId",
            "outcome",
            "canonicalSchemaVersion",
            "resultBytes",
            "resultSha256",
            "stableErrorCode",
        }
        if set(value) != fields:
            raise ProtocolError("response header fields are not closed")
        invocation_id = _text(value, "invocationId")
        if not _INVOCATION_RE.fullmatch(invocation_id):
            raise ProtocolError("invalid response invocationId")
        outcome = _text(value, "outcome")
        schema = _nullable_text(value, "canonicalSchemaVersion")
        result_bytes = _integer(value, "resultBytes")
        result_hash = _nullable_text(value, "resultSha256")
        raw_code = _nullable_text(value, "stableErrorCode")
        try:
            code = StableErrorCode(raw_code) if raw_code is not None else None
        except ValueError as error:
            raise ProtocolError("unknown stableErrorCode") from error
        response = cls(
            invocation_id=invocation_id,
            outcome=outcome,
            canonical_schema_version=schema,
            result_bytes=result_bytes,
            result_sha256=result_hash,
            stable_error_code=code,
        )
        response._validate_discriminator()
        return response

    def validate_body(self, body: bytes, *, invocation_id: str) -> None:
        """Verify invocation, body length, result hash, and outcome gates."""
        self._validate_discriminator()
        if self.invocation_id != invocation_id:
            raise ProtocolError("response invocationId does not match request")
        if len(body) != self.result_bytes:
            raise ProtocolError("response body length does not match its header")
        if self.outcome == "success":
            observed_hash = hashlib.sha256(body).hexdigest()
            if observed_hash != self.result_sha256:
                raise ProtocolError("response result hash does not match its body")
        elif body:
            raise ProtocolError("non-success response must have an empty body")

    def _validate_discriminator(self) -> None:
        if self.outcome == "success":
            if (
                self.canonical_schema_version != "canonical-ir.v2"
                or self.result_bytes < 1
                or self.result_bytes > MAX_RESULT_BYTES
                or self.result_sha256 is None
                or not _SHA256_RE.fullmatch(self.result_sha256)
                or self.stable_error_code is not None
            ):
                raise ProtocolError("invalid success response field combination")
            return
        if self.outcome == "route_required":
            if (
                self.canonical_schema_version is not None
                or self.result_bytes != 0
                or self.result_sha256 is not None
                or self.stable_error_code is not StableErrorCode.MINERU_REQUIRED
            ):
                raise ProtocolError("invalid route-required field combination")
            return
        if self.outcome == "failure":
            if (
                self.canonical_schema_version is not None
                or self.result_bytes != 0
                or self.result_sha256 is not None
                or self.stable_error_code not in WIRE_FAILURE_ERRORS
            ):
                raise ProtocolError("invalid failure response field combination")
            return
        raise ProtocolError("unknown response outcome")


def build_request_header(
    *,
    invocation_id: str,
    source: bytes,
    parser_config_hash: str,
    deadline_unix_millis: int,
    max_result_bytes: int,
    declared_mime: str | None = None,
    declared_extension: str | None = None,
) -> RequestHeader:
    """Build and bind a request header to exact source bytes."""
    provisional = RequestHeader(
        invocation_id=invocation_id,
        parser_config_hash=parser_config_hash,
        expected_source_bytes=len(source),
        expected_source_sha256=hashlib.sha256(source).hexdigest(),
        request_binding_hash="0" * 64,
        deadline_unix_millis=deadline_unix_millis,
        max_result_bytes=max_result_bytes,
        declared_mime=declared_mime,
        declared_extension=declared_extension,
    )
    validated = RequestHeader.from_object(provisional.to_object())
    return RequestHeader(
        invocation_id=validated.invocation_id,
        parser_config_hash=validated.parser_config_hash,
        expected_source_bytes=validated.expected_source_bytes,
        expected_source_sha256=validated.expected_source_sha256,
        request_binding_hash=request_binding_hash(validated, source),
        deadline_unix_millis=validated.deadline_unix_millis,
        max_result_bytes=validated.max_result_bytes,
        declared_mime=validated.declared_mime,
        declared_extension=validated.declared_extension,
    )


def request_binding_hash(header: RequestHeader, source: bytes) -> str:
    """Hash domain + JCS(header without binding) + raw source SHA-256 bytes."""
    value = header.to_object()
    del value["requestBindingHash"]
    source_digest = hashlib.sha256(source).digest()
    return hashlib.sha256(
        _BINDING_DOMAIN + canonical_json_bytes(value) + source_digest
    ).hexdigest()


def encode_frame(frame_type: FrameType, header: JsonObject, body: bytes) -> bytes:
    """Encode one exact MMCP v1 frame."""
    header_bytes = canonical_json_bytes(header)
    if not header_bytes or len(header_bytes) > MAX_HEADER_BYTES:
        raise ProtocolError("frame header length is outside the protocol limit")
    body_limit = (
        MAX_REQUEST_BODY_BYTES if frame_type is FrameType.REQUEST else MAX_RESULT_BYTES
    )
    if len(body) > body_limit:
        raise ProtocolError("frame body exceeds the protocol limit")
    return (
        _PREFIX.pack(
            MAGIC,
            PROTOCOL_MAJOR,
            frame_type.value,
            0,
            len(header_bytes),
            len(body),
        )
        + header_bytes
        + body
    )


def decode_frame_bytes(
    content: bytes,
    *,
    expected_type: FrameType,
) -> Frame:
    """Decode exactly one frame and reject every trailing byte."""
    if len(content) < _PREFIX.size:
        raise ProtocolError("short MMCP prefix")
    magic, major, raw_type, flags, header_length, body_length = _PREFIX.unpack_from(
        content
    )
    if magic != MAGIC or major != PROTOCOL_MAJOR:
        raise ProtocolError("invalid MMCP magic or protocol major")
    try:
        frame_type = FrameType(raw_type)
    except ValueError as error:
        raise ProtocolError("unknown MMCP frame type") from error
    if frame_type is not expected_type or flags != 0:
        raise ProtocolError("unexpected MMCP frame type or reserved flags")
    body_limit = (
        MAX_REQUEST_BODY_BYTES if frame_type is FrameType.REQUEST else MAX_RESULT_BYTES
    )
    if header_length < 1 or header_length > MAX_HEADER_BYTES:
        raise ProtocolError("MMCP header length is outside the protocol limit")
    if body_length > body_limit:
        raise ProtocolError("MMCP body length is outside the protocol limit")
    expected_length = _PREFIX.size + header_length + body_length
    if len(content) != expected_length:
        raise ProtocolError("MMCP frame is short or has trailing bytes")
    header_start = _PREFIX.size
    header_end = header_start + header_length
    try:
        header = load_canonical_json_object(content[header_start:header_end])
    except CanonicalJsonError as error:
        raise ProtocolError("MMCP header is not canonical JSON") from error
    return Frame(frame_type=frame_type, header=header, body=content[header_end:])


def decode_request(
    content: bytes,
    *,
    expected_config_hash: str | None = None,
    now_unix_millis: int | None = None,
) -> tuple[RequestHeader, bytes]:
    """Decode and independently validate one request frame."""
    frame = decode_frame_bytes(content, expected_type=FrameType.REQUEST)
    header = RequestHeader.from_object(frame.header)
    header.validate_body(
        frame.body,
        expected_config_hash=expected_config_hash,
        now_unix_millis=now_unix_millis,
    )
    return header, frame.body


def decode_response(
    content: bytes, *, invocation_id: str
) -> tuple[ResponseHeader, bytes]:
    """Decode and independently validate one response frame."""
    frame = decode_frame_bytes(content, expected_type=FrameType.RESPONSE)
    header = ResponseHeader.from_object(frame.header)
    header.validate_body(frame.body, invocation_id=invocation_id)
    return header, frame.body


def _text(value: JsonObject, key: str) -> str:
    item = value.get(key)
    if not isinstance(item, str):
        raise ProtocolError(f"{key} must be a string")
    return item


def _optional_text(value: JsonObject, key: str) -> str | None:
    if key not in value:
        return None
    return _text(value, key)


def _nullable_text(value: JsonObject, key: str) -> str | None:
    item = value.get(key)
    if item is None:
        return None
    if not isinstance(item, str):
        raise ProtocolError(f"{key} must be a string or null")
    return item


def _integer(value: JsonObject, key: str) -> int:
    item = value.get(key)
    if type(item) is not int:
        raise ProtocolError(f"{key} must be an integer")
    return item
