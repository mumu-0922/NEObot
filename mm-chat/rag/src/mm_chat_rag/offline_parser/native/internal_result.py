"""Closed non-MMCP Child/Supervisor Native Artifact result envelope."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from typing import Final, Self

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    JsonObject,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import (
    CONTROLLER_ONLY_ERRORS,
    ParserFormat,
    StableErrorCode,
)
from mm_chat_rag.offline_parser.native.model import NATIVE_ARTIFACT_SCHEMA_VERSION

_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")


class InternalResultError(ValueError):
    """Child/Supervisor Native Result violates its closed discriminator."""


@dataclass(frozen=True, slots=True)
class NativeResultHeader:
    """Bounded child-internal header; this is not an MMCP response."""

    outcome: str
    parser_format: ParserFormat | None
    native_artifact_version: str | None
    result_bytes: int
    result_sha256: str | None
    stable_error_code: StableErrorCode | None

    @classmethod
    def success(cls, parser_format: ParserFormat, body: bytes) -> Self:
        """Build a native-success header bound to exact internal bytes."""
        if not body:
            raise InternalResultError("native success body must be non-empty")
        return cls(
            outcome="native_success",
            parser_format=parser_format,
            native_artifact_version=NATIVE_ARTIFACT_SCHEMA_VERSION,
            result_bytes=len(body),
            result_sha256=hashlib.sha256(body).hexdigest(),
            stable_error_code=None,
        )

    @classmethod
    def failure(cls, code: StableErrorCode) -> Self:
        """Build a stable zero-body internal failure."""
        if code in CONTROLLER_ONLY_ERRORS:
            raise InternalResultError(
                "controller-only errors are forbidden in child results"
            )
        return cls(
            outcome="failure",
            parser_format=None,
            native_artifact_version=None,
            result_bytes=0,
            result_sha256=None,
            stable_error_code=code,
        )

    @classmethod
    def from_bytes(cls, content: bytes) -> Self:
        """Decode one canonical, duplicate-free, closed header."""
        try:
            value = load_canonical_json_object(content)
        except CanonicalJsonError as error:
            message = "native result header is not canonical"
            raise InternalResultError(message) from error
        fields = {
            "format",
            "nativeArtifactVersion",
            "outcome",
            "resultBytes",
            "resultSha256",
            "stableErrorCode",
        }
        if set(value) != fields:
            raise InternalResultError("native result header fields are not closed")
        raw_format = value["format"]
        raw_code = value["stableErrorCode"]
        raw_version = value["nativeArtifactVersion"]
        raw_outcome = value["outcome"]
        raw_bytes = value["resultBytes"]
        raw_hash = value["resultSha256"]
        if raw_format is not None and not isinstance(raw_format, str):
            raise InternalResultError("native result format is not nullable text")
        if raw_code is not None and not isinstance(raw_code, str):
            raise InternalResultError("native stable error is not nullable text")
        try:
            parser_format = (
                ParserFormat(raw_format) if isinstance(raw_format, str) else None
            )
            code = StableErrorCode(raw_code) if isinstance(raw_code, str) else None
        except ValueError as error:
            raise InternalResultError("native result enum is unknown") from error
        if not isinstance(raw_outcome, str):
            raise InternalResultError("native result outcome is not text")
        if raw_version is not None and not isinstance(raw_version, str):
            raise InternalResultError("native artifact version is not nullable text")
        if type(raw_bytes) is not int:
            raise InternalResultError("native result byte count is not an integer")
        if raw_hash is not None and not isinstance(raw_hash, str):
            raise InternalResultError("native result hash is not nullable text")
        header = cls(
            outcome=raw_outcome,
            parser_format=parser_format,
            native_artifact_version=raw_version,
            result_bytes=raw_bytes,
            result_sha256=raw_hash,
            stable_error_code=code,
        )
        header._validate_discriminator()
        return header

    def to_object(self) -> JsonObject:
        """Return the exact child-internal JSON shape."""
        return {
            "format": self.parser_format.value if self.parser_format else None,
            "nativeArtifactVersion": self.native_artifact_version,
            "outcome": self.outcome,
            "resultBytes": self.result_bytes,
            "resultSha256": self.result_sha256,
            "stableErrorCode": (
                self.stable_error_code.value if self.stable_error_code else None
            ),
        }

    @property
    def canonical_bytes(self) -> bytes:
        """Return deterministic header bytes."""
        return canonical_json_bytes(self.to_object())

    def validate_body(self, body: bytes, *, body_limit: int) -> None:
        """Verify length, limit, hash, and failure zero-body behavior."""
        self._validate_discriminator()
        if self.result_bytes > body_limit or len(body) != self.result_bytes:
            raise InternalResultError("native result body length is invalid")
        if self.outcome == "native_success":
            if hashlib.sha256(body).hexdigest() != self.result_sha256:
                raise InternalResultError("native result body hash does not match")
        elif body:
            raise InternalResultError("native failure body must be empty")

    def _validate_discriminator(self) -> None:
        if self.outcome == "native_success":
            if (
                self.parser_format
                not in {ParserFormat.TXT, ParserFormat.MARKDOWN, ParserFormat.HTML}
                or self.native_artifact_version != NATIVE_ARTIFACT_SCHEMA_VERSION
                or self.result_bytes < 1
                or self.result_sha256 is None
                or not _SHA256_RE.fullmatch(self.result_sha256)
                or self.stable_error_code is not None
            ):
                raise InternalResultError("invalid native-success field combination")
            return
        if self.outcome == "failure":
            if (
                self.parser_format is not None
                or self.native_artifact_version is not None
                or self.result_bytes != 0
                or self.result_sha256 is not None
                or self.stable_error_code is None
                or self.stable_error_code in CONTROLLER_ONLY_ERRORS
            ):
                raise InternalResultError("invalid native-failure field combination")
            return
        raise InternalResultError("unknown native result outcome")
