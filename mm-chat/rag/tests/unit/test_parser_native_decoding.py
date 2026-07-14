"""Focused invariants for deterministic native text decoding and locators."""

from __future__ import annotations

from array import array

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native import decoding
from mm_chat_rag.offline_parser.native.decoding import (
    CompactOffsets,
    DecodedSource,
    DecodedText,
    decode_source,
    decode_text,
)
from mm_chat_rag.offline_parser.native.model import NativeParseFailure


def _offsets(*values: int) -> CompactOffsets:
    return CompactOffsets(array("I", values))


def _assert_failure(source: bytes, code: StableErrorCode) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        decode_source(source)
    assert observed.value.code is code


def test_compact_offsets_rejects_non_uint_storage_and_supports_slices() -> None:
    with pytest.raises(ValueError, match="unsigned 32-bit"):
        CompactOffsets(array("B", [0]))

    offsets = _offsets(0, 2, 7)
    assert len(offsets) == 3
    assert offsets[1] == 2
    assert offsets[1:] == array("I", [2, 7])


@pytest.mark.parametrize(
    ("source", "text", "encoding", "raw_boundaries", "line_starts", "message"),
    [
        (b"x", "x", "utf-8", (0,), (0,), "cardinality"),
        (b"x", "x", "utf-8", (1, 1), (0,), "do not cover"),
        (b"x", "x", "utf-8", (0, 0), (0,), "do not cover"),
        (b"x", "x", "utf-8", (0, 1), (), "line index"),
        (b"x", "x", "utf-8", (0, 1), (1,), "line index"),
        (b"\xef\xbb\xbfx", "x", "utf-8-bom", (0, 4), (0,), "do not cover"),
    ],
)
def test_decoded_source_rejects_inconsistent_indexes(
    source: bytes,
    text: str,
    encoding: str,
    raw_boundaries: tuple[int, ...],
    line_starts: tuple[int, ...],
    message: str,
) -> None:
    with pytest.raises(ValueError, match=message):
        DecodedSource(
            source=source,
            text=text,
            encoding=encoding,
            raw_boundaries=_offsets(*raw_boundaries),
            line_starts=_offsets(*line_starts),
        )


@pytest.mark.parametrize(
    ("source", "encoding", "raw_offset"),
    [
        (b"plain", "utf-8", 0),
        (b"\xef\xbb\xbfbom", "utf-8-bom", 3),
        ("中文😀".encode("gb18030"), "gb18030", 0),
    ],
)
def test_decode_text_follows_frozen_encoding_precedence(
    source: bytes,
    encoding: str,
    raw_offset: int,
) -> None:
    decoded = decode_text(source)

    assert decoded.encoding == encoding
    assert decoded.raw_offset == raw_offset
    codec = "utf-8" if encoding.startswith("utf-8") else encoding
    assert decoded.text == source[raw_offset:].decode(codec)


def test_utf8_bom_and_multibyte_boundaries_preserve_raw_byte_offsets() -> None:
    source = b"\xef\xbb\xbf" + "A😀é".encode()
    decoded = decode_source(source)

    assert decoded.decoded_scalars == 3
    assert list(decoded.raw_boundaries[:]) == [3, 4, 8, 10]
    assert decoded.position(1, 2).raw_byte_start == 4
    assert decoded.position(1, 2).raw_byte_end == 8

    document = decoded.document_position()
    assert (document.raw_byte_start, document.raw_byte_end) == (0, len(source))
    assert (document.end_line, document.end_column) == (0, 3)


def test_gb18030_two_and_four_byte_characters_have_exact_boundaries() -> None:
    text = "A中😀B"
    source = text.encode("gb18030")
    decoded = decode_source(source)
    expected = [0]
    for character in text:
        expected.append(expected[-1] + len(character.encode("gb18030")))

    assert decoded.encoding == "gb18030"
    assert list(decoded.raw_boundaries[:]) == expected
    emoji = decoded.position(2, 3)
    assert emoji.raw_byte_end - emoji.raw_byte_start == 4


@pytest.mark.parametrize(
    ("source", "expected"),
    [
        (b"embedded\x00nul", StableErrorCode.INPUT_INVALID),
        ("\ufffd".encode(), StableErrorCode.INPUT_INVALID),
        (b"\xef\xbb\xbf\xff", StableErrorCode.ENCODING_AMBIGUOUS),
        (b"\xff", StableErrorCode.ENCODING_AMBIGUOUS),
    ],
)
def test_decode_source_rejects_forbidden_or_ambiguous_bytes(
    source: bytes,
    expected: StableErrorCode,
) -> None:
    _assert_failure(source, expected)


def test_decode_text_requires_an_immutable_bytes_source() -> None:
    with pytest.raises(TypeError, match="must be bytes"):
        decode_text(bytearray(b"text"))  # type: ignore[arg-type]


def test_decode_source_enforces_utf8_text_bytes_and_logical_line_limits() -> None:
    decoded = decode_source("é".encode(), limits=NativeParserLimits(text_bytes=2))
    assert decoded.text == "é"
    with pytest.raises(NativeParseFailure) as text_error:
        decode_source("é".encode(), limits=NativeParserLimits(text_bytes=1))
    assert text_error.value.code is StableErrorCode.RESULT_TOO_LARGE

    assert decode_source(b"a\r\nb\rc\n", limits=NativeParserLimits(lines=4)).text
    with pytest.raises(NativeParseFailure) as line_error:
        decode_source(b"a\r\nb\rc\n", limits=NativeParserLimits(lines=3))
    assert line_error.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_mixed_newlines_map_line_columns_and_scalar_boundaries() -> None:
    decoded = decode_source(b"ab\r\ncd\ref\ngh")

    assert list(decoded.line_starts[:]) == [0, 4, 7, 10]
    assert decoded.line_column(0) == (0, 0)
    assert decoded.line_column(4) == (1, 0)
    assert decoded.line_column(len(decoded.text)) == (3, 2)
    assert decoded.scalar_at(0, 4) == 4
    assert decoded.scalar_at(3, 2) == len(decoded.text)

    middle = decoded.line_span(1, 3)
    assert (middle.decoded_scalar_start, middle.decoded_scalar_end) == (4, 10)
    complete = decoded.line_span(0, len(decoded.line_starts))
    assert (complete.raw_byte_start, complete.raw_byte_end) == (0, len(decoded.source))


@pytest.mark.parametrize(
    ("start", "end"),
    [(-1, 0), (1, 0), (0, 4)],
)
def test_position_rejects_ranges_outside_decoded_source(start: int, end: int) -> None:
    with pytest.raises(ValueError, match="range exceeds"):
        decode_source(b"abc").position(start, end)


@pytest.mark.parametrize("offset", [-1, 4])
def test_line_column_rejects_offsets_outside_decoded_source(offset: int) -> None:
    with pytest.raises(ValueError, match="offset exceeds"):
        decode_source(b"abc").line_column(offset)


@pytest.mark.parametrize(
    ("line", "column", "message"),
    [
        (True, 0, "non-negative integers"),
        (-1, 0, "non-negative integers"),
        (0, -1, "non-negative integers"),
        (2, 0, "line exceeds"),
        (0, 3, "column exceeds"),
    ],
)
def test_scalar_at_rejects_invalid_line_columns(
    line: int,
    column: int,
    message: str,
) -> None:
    with pytest.raises(ValueError, match=message):
        decode_source(b"a\nb").scalar_at(line, column)


@pytest.mark.parametrize(
    ("start_line", "end_line", "message"),
    [
        (-1, 0, "exceeds"),
        (1, 0, "exceeds"),
        (0, 3, "exceeds"),
        (2, 2, "starts beyond"),
    ],
)
def test_line_span_rejects_ranges_outside_line_index(
    start_line: int,
    end_line: int,
    message: str,
) -> None:
    with pytest.raises(ValueError, match=message):
        decode_source(b"a\nb").line_span(start_line, end_line)


class _NonCanonicalText(str):
    __slots__ = ()

    def encode(self, *args: object, **kwargs: object) -> bytes:
        return b"canonical"


class _NonCanonicalBytes(bytes):
    def decode(self, *args: object, **kwargs: object) -> str:
        return _NonCanonicalText("decoded")


def test_strict_decoder_rejects_a_non_round_tripping_codec_result() -> None:
    with pytest.raises(NativeParseFailure) as observed:
        decoding._strict_decode(_NonCanonicalBytes(b"wire"), codec="utf-8")
    assert observed.value.code is StableErrorCode.ENCODING_AMBIGUOUS


def test_index_builder_rejects_text_that_does_not_cover_source_bytes() -> None:
    decoded = DecodedText(text="x", encoding="utf-8", codec="utf-8", raw_offset=0)

    with pytest.raises(NativeParseFailure) as observed:
        decoding._index_decoded_source(b"\xc3\xa9", decoded, _offsets(0))
    assert observed.value.code is StableErrorCode.ENCODING_AMBIGUOUS
