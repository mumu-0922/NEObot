"""C1.3 TXT decoding, exact native positions, dispatch, and limits."""

from __future__ import annotations

import unicodedata
from dataclasses import replace
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser import router
from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native import decoding, dispatch
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.dispatch import parse_native_source
from mm_chat_rag.offline_parser.native.model import NativeParseFailure
from mm_chat_rag.offline_parser.native.txt import parse_txt

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"


@pytest.mark.parametrize(
    ("name", "encoding", "raw_start"),
    [
        ("utf8-nfc.txt", "utf-8", 0),
        ("utf8-nfd.txt", "utf-8", 0),
        ("utf8-bom.txt", "utf-8-bom", 3),
        ("crlf.txt", "utf-8", 0),
        ("cr.txt", "utf-8", 0),
        ("gb18030.txt", "gb18030", 0),
    ],
)
def test_txt_corpus_preserves_encoding_and_exact_source_bounds(
    name: str,
    encoding: str,
    raw_start: int,
) -> None:
    source = (_CORPUS / "golden" / "text" / name).read_bytes()

    outcome = parse_native_source(source, declared_extension=".txt")

    assert outcome.parser_format is ParserFormat.TXT
    assert outcome.stable_error_code is None
    assert outcome.artifact is not None
    artifact = outcome.artifact
    assert artifact.source_encoding == encoding
    if artifact.decoded_scalars:
        fragment = artifact.nodes[1].fragments[0]
        assert fragment.source_position.raw_byte_start == raw_start
        assert fragment.source_position.raw_byte_end == len(source)
        codec = "utf-8" if encoding.startswith("utf-8") else encoding
        assert source[raw_start:].decode(codec) == fragment.text


def test_txt_positions_use_scalar_columns_and_crlf_as_one_line_break() -> None:
    decoded = decode_source("A😀\r\n中文\rZ".encode())

    emoji = decoded.position(1, 2)
    after_crlf = decoded.position(4, 6)
    final = decoded.position(7, 8)

    assert (emoji.raw_byte_start, emoji.raw_byte_end) == (1, 5)
    assert (emoji.start_line, emoji.start_column, emoji.end_column) == (0, 1, 2)
    assert (after_crlf.start_line, after_crlf.start_column) == (1, 0)
    assert (final.start_line, final.start_column) == (2, 0)


def test_txt_preserves_nfd_until_c14_normalization() -> None:
    source = (_CORPUS / "golden" / "text" / "utf8-nfd.txt").read_bytes()
    outcome = parse_native_source(source, declared_mime="text/plain")

    assert outcome.artifact is not None
    text = outcome.artifact.nodes[1].fragments[0].text
    assert unicodedata.is_normalized("NFD", text)
    assert not unicodedata.is_normalized("NFC", text)


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        ("ambiguous.bin", StableErrorCode.ENCODING_AMBIGUOUS),
        ("invalid-utf8.txt", StableErrorCode.ENCODING_AMBIGUOUS),
        ("nul.txt", StableErrorCode.INPUT_INVALID),
        ("replacement-character.txt", StableErrorCode.INPUT_INVALID),
    ],
)
def test_txt_adversarial_encoding_errors_remain_stable(
    path: str,
    expected: StableErrorCode,
) -> None:
    source = (_CORPUS / "adversarial" / "encoding" / path).read_bytes()
    extension = None if path.endswith(".bin") else ".txt"

    outcome = parse_native_source(source, declared_extension=extension)

    assert outcome.artifact is None
    assert outcome.stable_error_code is expected


def test_txt_empty_source_is_a_valid_root_only_native_artifact() -> None:
    outcome = parse_native_source(b"", declared_extension=".txt")

    assert outcome.artifact is not None
    assert len(outcome.artifact.nodes) == 1
    assert outcome.artifact.nodes[0].source_position.raw_byte_end == 0


@pytest.mark.parametrize(
    "limits",
    [
        NativeParserLimits(lines=1),
        NativeParserLimits(text_bytes=1),
        NativeParserLimits(artifact_bytes=1),
    ],
)
def test_txt_hash_bound_native_limits_fail_without_truncation(
    limits: NativeParserLimits,
) -> None:
    outcome = parse_native_source(
        "é\nline".encode(),
        declared_extension=".txt",
        limits=limits,
    )

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.RESULT_TOO_LARGE


def test_decode_source_rejects_non_bytes_and_invalid_bom_payload() -> None:
    with pytest.raises(TypeError):
        decode_source("text")  # type: ignore[arg-type]
    with pytest.raises(NativeParseFailure) as observed:
        decode_source(b"\xef\xbb\xbf\xff")
    assert observed.value.code is StableErrorCode.ENCODING_AMBIGUOUS


def test_text_decoder_uses_compact_32_bit_position_indexes() -> None:
    decoded = decode_source(b"a" * 1_000_000)

    assert decoded.raw_boundaries._values.itemsize == 4
    assert len(decoded.raw_boundaries) == 1_000_001
    assert len(decoded.line_starts) == 1


def test_child_stage_rejects_internally_inconsistent_locator_semantics(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    decoded = decode_source(b"abc")
    artifact = parse_txt(decoded)
    paragraph = artifact.nodes[1]
    fragment = paragraph.fragments[0]
    bad_position = replace(
        fragment.source_position,
        decoded_scalar_start=1,
        start_column=1,
    )
    bad_fragment = replace(fragment, source_position=bad_position)
    bad_paragraph = replace(paragraph, fragments=(bad_fragment,))
    bad_artifact = replace(
        artifact,
        nodes=(artifact.nodes[0], bad_paragraph),
    )
    monkeypatch.setattr(
        dispatch,
        "_parse_selected",
        lambda *_args: bad_artifact,
    )

    outcome = parse_native_source(b"abc", declared_extension=".txt")

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_router_text_probe_does_not_allocate_locator_indexes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        decoding,
        "_index_decoded_source",
        lambda *_args: (_ for _ in ()).throw(AssertionError),
    )

    assert router._decode_text(b"lightweight") == "lightweight"
