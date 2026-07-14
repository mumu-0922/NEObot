"""Deterministic text decoding with exact Raw Byte/Scalar/Line indexes."""

from __future__ import annotations

import hashlib
from array import array
from bisect import bisect_left, bisect_right
from dataclasses import dataclass
from typing import TYPE_CHECKING, Final, overload

from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeParseFailure,
    NativeSourcePosition,
    NativeSourceUnit,
    NativeSourceUnitKind,
)

if TYPE_CHECKING:
    from mm_chat_rag.offline_parser.config import NativeParserLimits

_UTF8_BOM: Final = b"\xef\xbb\xbf"


@dataclass(frozen=True, slots=True)
class CompactOffsets:
    """Read-only facade over compact unsigned 32-bit offsets."""

    _values: array[int]

    def __post_init__(self) -> None:
        if self._values.typecode != "I":
            raise ValueError("compact offsets require unsigned 32-bit storage")

    def __len__(self) -> int:
        return len(self._values)

    @overload
    def __getitem__(self, index: int) -> int: ...

    @overload
    def __getitem__(self, index: slice) -> array[int]: ...

    def __getitem__(self, index: int | slice) -> int | array[int]:
        return self._values[index]


@dataclass(frozen=True, slots=True)
class DecodedText:
    """Lightweight deterministic decode result without Locator indexes."""

    text: str
    encoding: str
    codec: str
    raw_offset: int


@dataclass(frozen=True, slots=True)
class DecodedSource:
    """Strict decoded source plus exact scalar-to-byte and line indexes."""

    source: bytes
    text: str
    encoding: str
    raw_boundaries: CompactOffsets
    line_starts: CompactOffsets
    source_unit_ordinal: int = 0

    def __post_init__(self) -> None:
        if type(self.source_unit_ordinal) is not int or self.source_unit_ordinal < 0:
            raise ValueError("decoded source-unit ordinal must be non-negative")
        if len(self.raw_boundaries) != len(self.text) + 1:
            raise ValueError("decoded source boundary cardinality is invalid")
        if not self.raw_boundaries:
            raise ValueError("decoded source raw boundaries are empty")
        expected_start = 3 if self.encoding == "utf-8-bom" else 0
        if self.raw_boundaries[0] != expected_start or self.raw_boundaries[-1] != len(
            self.source
        ):
            raise ValueError("decoded source raw boundaries do not cover the source")
        if not self.line_starts or self.line_starts[0] != 0:
            raise ValueError("decoded source line index must start at scalar zero")

    @property
    def decoded_scalars(self) -> int:
        """Return the decoded Unicode Scalar count before normalization."""
        return len(self.text)

    def position(self, scalar_start: int, scalar_end: int) -> NativeSourcePosition:
        """Project one half-open scalar range to exact source coordinates."""
        if not 0 <= scalar_start <= scalar_end <= len(self.text):
            raise ValueError("decoded scalar range exceeds the source")
        start_line, start_column = self.line_column(scalar_start)
        end_line, end_column = self.line_column(scalar_end)
        return NativeSourcePosition(
            raw_byte_start=self.raw_boundaries[scalar_start],
            raw_byte_end=self.raw_boundaries[scalar_end],
            decoded_scalar_start=scalar_start,
            decoded_scalar_end=scalar_end,
            start_line=start_line,
            start_column=start_column,
            end_line=end_line,
            end_column=end_column,
            source_unit_ordinal=self.source_unit_ordinal,
        )

    def document_position(self) -> NativeSourcePosition:
        """Return a structural span covering all raw source bytes."""
        end_line, end_column = self.line_column(len(self.text))
        return NativeSourcePosition(
            raw_byte_start=0,
            raw_byte_end=len(self.source),
            decoded_scalar_start=0,
            decoded_scalar_end=len(self.text),
            start_line=0,
            start_column=0,
            end_line=end_line,
            end_column=end_column,
            source_unit_ordinal=self.source_unit_ordinal,
        )

    def scalar_at_raw_boundary(self, raw_offset: int) -> int:
        """Resolve an exact raw byte boundary to its decoded scalar offset."""
        if type(raw_offset) is not int or raw_offset < 0:
            raise ValueError("raw byte boundary must be a non-negative integer")
        index = bisect_left(self.raw_boundaries._values, raw_offset)  # noqa: SLF001
        if (
            index >= len(self.raw_boundaries)
            or self.raw_boundaries[index] != raw_offset
        ):
            raise ValueError("raw byte offset is not a decoded scalar boundary")
        return index

    def line_column(self, scalar_offset: int) -> tuple[int, int]:
        """Return a zero-based line and Unicode-Scalar column."""
        if not 0 <= scalar_offset <= len(self.text):
            raise ValueError("decoded scalar offset exceeds the source")
        line = bisect_right(self.line_starts, scalar_offset) - 1
        return line, scalar_offset - self.line_starts[line]

    def scalar_at(self, line: int, column: int) -> int:
        """Resolve one zero-based line/column pair to a scalar boundary."""
        if type(line) is not int or type(column) is not int or line < 0 or column < 0:
            raise ValueError("line and column must be non-negative integers")
        if line >= len(self.line_starts):
            raise ValueError("line exceeds the decoded source")
        scalar = self.line_starts[line] + column
        line_end = (
            self.line_starts[line + 1]
            if line + 1 < len(self.line_starts)
            else len(self.text)
        )
        if scalar > line_end:
            raise ValueError("column exceeds the decoded source line")
        return scalar

    def line_span(self, start_line: int, end_line: int) -> NativeSourcePosition:
        """Return the exact source position for a half-open line range."""
        if not 0 <= start_line <= end_line < len(self.line_starts) + 1:
            raise ValueError("line span exceeds the decoded source")
        if start_line >= len(self.line_starts):
            raise ValueError("line span starts beyond the decoded source")
        scalar_start = self.line_starts[start_line]
        scalar_end = (
            self.line_starts[end_line]
            if end_line < len(self.line_starts)
            else len(self.text)
        )
        return self.position(scalar_start, scalar_end)


def decode_source(
    source: bytes,
    *,
    limits: NativeParserLimits | None = None,
    source_unit_ordinal: int = 0,
) -> DecodedSource:
    """Decode and build compact exact Locator indexes."""
    decoded = decode_text(source)
    if limits is not None and len(decoded.text.encode("utf-8")) > limits.text_bytes:
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
    line_starts = _line_starts(decoded.text)
    if limits is not None and len(line_starts) > limits.lines:
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
    return _index_decoded_source(
        source,
        decoded,
        line_starts,
        source_unit_ordinal=source_unit_ordinal,
    )


def decode_xml_source(
    source: bytes,
    *,
    source_unit_ordinal: int,
    limits: NativeParserLimits | None = None,
) -> DecodedSource:
    """Decode an OOXML Part using only the frozen UTF-8/BOM profile."""
    decoded = decode_text(source)
    if decoded.encoding == "gb18030":
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    if limits is not None and len(decoded.text.encode("utf-8")) > limits.xml_text_bytes:
        raise NativeParseFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
    line_starts = _line_starts(decoded.text)
    if limits is not None and len(line_starts) > limits.lines:
        raise NativeParseFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
    return _index_decoded_source(
        source,
        decoded,
        line_starts,
        source_unit_ordinal=source_unit_ordinal,
    )


def source_unit_from_decoded(
    decoded: DecodedSource,
    *,
    kind: NativeSourceUnitKind,
    canonical_uri: str | None,
) -> NativeSourceUnit:
    """Build source-unit metadata bound to one decoded byte sequence."""
    return NativeSourceUnit(
        ordinal=decoded.source_unit_ordinal,
        kind=kind,
        canonical_uri=canonical_uri,
        source_bytes=len(decoded.source),
        source_sha256=hashlib.sha256(decoded.source).hexdigest(),
        encoding=decoded.encoding,
        decoded_scalars=decoded.decoded_scalars,
    )


def decode_text(source: bytes) -> DecodedText:
    """Decode by BOM -> strict UTF-8 -> gb18030 without allocating indexes."""
    if not isinstance(source, bytes):
        raise TypeError("native parser source must be bytes")
    if b"\x00" in source:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    if source.startswith(_UTF8_BOM):
        try:
            text = _strict_decode(
                source[len(_UTF8_BOM) :],
                codec="utf-8",
            )
        except UnicodeDecodeError as error:
            raise NativeParseFailure(StableErrorCode.ENCODING_AMBIGUOUS) from error
        return DecodedText(text, "utf-8-bom", "utf-8", len(_UTF8_BOM))
    try:
        text = _strict_decode(source, codec="utf-8")
    except UnicodeDecodeError:
        pass
    else:
        return DecodedText(text, "utf-8", "utf-8", 0)
    try:
        text = _strict_decode(source, codec="gb18030")
    except UnicodeDecodeError as error:
        raise NativeParseFailure(StableErrorCode.ENCODING_AMBIGUOUS) from error
    return DecodedText(text, "gb18030", "gb18030", 0)


def _strict_decode(payload: bytes, *, codec: str) -> str:
    text = payload.decode(codec, errors="strict")
    if "\ufffd" in text:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    if text.encode(codec, errors="strict") != payload:
        raise NativeParseFailure(StableErrorCode.ENCODING_AMBIGUOUS)
    return text


def _index_decoded_source(
    source: bytes,
    decoded: DecodedText,
    line_starts: CompactOffsets,
    *,
    source_unit_ordinal: int = 0,
) -> DecodedSource:
    payload = source[decoded.raw_offset :]
    if payload.isascii():
        boundaries = array(
            "I",
            range(decoded.raw_offset, decoded.raw_offset + len(payload) + 1),
        )
        observed = decoded.raw_offset + len(payload)
    else:
        boundaries = array("I", [decoded.raw_offset])
        observed = decoded.raw_offset
        for character in decoded.text:
            observed += len(character.encode(decoded.codec, errors="strict"))
            boundaries.append(observed)
    if observed != len(source):
        raise NativeParseFailure(StableErrorCode.ENCODING_AMBIGUOUS)
    return DecodedSource(
        source=source,
        text=decoded.text,
        encoding=decoded.encoding,
        raw_boundaries=CompactOffsets(boundaries),
        line_starts=line_starts,
        source_unit_ordinal=source_unit_ordinal,
    )


def _line_starts(text: str) -> CompactOffsets:
    starts = array("I", [0])
    if "\n" not in text and "\r" not in text:
        return CompactOffsets(starts)
    index = 0
    while index < len(text):
        character = text[index]
        if character == "\r":
            index += 1
            if index < len(text) and text[index] == "\n":
                index += 1
            starts.append(index)
            continue
        index += 1
        if character == "\n":
            starts.append(index)
    return CompactOffsets(starts)
