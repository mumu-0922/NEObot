"""C1.3 fixed-dialect CSV structure, Locator, and failure tests."""

from __future__ import annotations

from pathlib import Path

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.csv import parse_csv
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeSourceUnitKind,
    NativeTransformKind,
)

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"


def _parse(
    source: bytes,
    limits: NativeParserLimits | None = None,
) -> NativeDocument:
    selected = limits or NativeParserLimits()
    return parse_csv(decode_source(source, limits=selected), selected)


def _attributes(node: NativeNode) -> dict[str, object]:
    return {attribute.name: attribute.value for attribute in node.attributes}


def _cells(artifact: NativeDocument) -> list[NativeNode]:
    return [node for node in artifact.nodes if node.kind is NativeNodeKind.TABLE_CELL]


def test_representative_csv_preserves_table_shape_quotes_and_exact_bytes() -> None:
    source = (_CORPUS / "golden" / "csv" / "representative.csv").read_bytes()

    artifact = _parse(source)

    assert artifact.source_format is ParserFormat.CSV
    assert artifact.source_units[0].kind is NativeSourceUnitKind.RAW_FILE
    assert artifact.source_units[0].encoding == "utf-8"
    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.TABLE,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
    ]
    assert _attributes(artifact.nodes[1]) == {
        "columnCount": 3,
        "delimiter": ",",
        "rowCount": 3,
    }
    cells = _cells(artifact)
    assert len(cells) == 9
    assert cells[0].fragments[0].text == "name"
    assert cells[0].fragments[0].role is NativeFragmentRole.CELL_VALUE
    assert cells[0].fragments[0].transform is NativeTransformKind.IDENTITY

    quoted = cells[4]
    quoted_fragment = quoted.fragments[0]
    assert quoted_fragment.text == 'comma, and "quote"'
    assert quoted_fragment.transform is NativeTransformKind.SYNTAX_DECODE
    position = quoted_fragment.source_position
    assert source[position.raw_byte_start : position.raw_byte_end] == (
        b'"comma, and ""quote"""'
    )
    assert _attributes(quoted) == {
        "columnIndex": 1,
        "empty": False,
        "quoted": True,
        "rowIndex": 1,
    }
    assert NativeNodeKind.HEADING not in {node.kind for node in artifact.nodes}


@pytest.mark.parametrize(
    ("source", "encoding", "raw_start", "expected"),
    [
        (b'\xef\xbb\xbf"value",x\r\n', "utf-8-bom", 3, "value"),
        ('"中文",值\r\n'.encode("gb18030"), "gb18030", 0, "中文"),
    ],
)
def test_csv_bom_and_gb18030_keep_exact_raw_boundaries(
    source: bytes,
    encoding: str,
    raw_start: int,
    expected: str,
) -> None:
    artifact = _parse(source)
    first = _cells(artifact)[0]
    fragment = first.fragments[0]

    assert artifact.source_units[0].encoding == encoding
    assert first.source_position.raw_byte_start == raw_start
    assert fragment.text == expected
    assert fragment.source_position == first.source_position
    assert artifact.nodes[0].source_position.raw_byte_start == 0
    assert artifact.nodes[0].source_position.raw_byte_end == len(source)


def test_csv_quoted_multiline_has_physical_line_coordinates() -> None:
    source = b'name,note\r\none,"line1\r\nline2"\r\n'

    artifact = _parse(source)
    multiline = _cells(artifact)[3]
    fragment = multiline.fragments[0]

    assert fragment.text == "line1\r\nline2"
    assert fragment.transform is NativeTransformKind.SYNTAX_DECODE
    assert (
        fragment.source_position.start_line,
        fragment.source_position.start_column,
        fragment.source_position.end_line,
        fragment.source_position.end_column,
    ) == (1, 4, 2, 6)
    position = fragment.source_position
    assert (
        source[position.raw_byte_start : position.raw_byte_end] == b'"line1\r\nline2"'
    )


def test_csv_double_quote_decodes_atomically_without_losing_source_syntax() -> None:
    source = b'"a""b",c\n'

    artifact = _parse(source)
    fragment = _cells(artifact)[0].fragments[0]

    assert fragment.text == 'a"b'
    assert fragment.transform is NativeTransformKind.SYNTAX_DECODE
    position = fragment.source_position
    assert source[position.raw_byte_start : position.raw_byte_end] == b'"a""b"'


def test_csv_empty_and_trailing_cells_are_structural_without_empty_fragments() -> None:
    artifact = _parse(b'a,,,\n,"",z,')
    cells = _cells(artifact)

    assert _attributes(artifact.nodes[1])["columnCount"] == 4
    assert len(cells) == 8
    assert [len(cell.fragments) for cell in cells] == [1, 0, 0, 0, 0, 0, 1, 0]
    assert [_attributes(cell)["empty"] for cell in cells] == [
        False,
        True,
        True,
        True,
        True,
        True,
        False,
        True,
    ]
    assert _attributes(cells[5])["quoted"] is True
    assert (
        cells[5].source_position.raw_byte_end - cells[5].source_position.raw_byte_start
        == 2
    )


@pytest.mark.parametrize("terminator", ["\r\n", "\n", "\r"])
def test_csv_accepts_each_frozen_record_terminator(terminator: str) -> None:
    source = f"a,b{terminator}c,d".encode()

    artifact = _parse(source)
    rows = [node for node in artifact.nodes if node.kind is NativeNodeKind.TABLE_ROW]

    assert [_attributes(row)["recordTerminator"] for row in rows] == [
        terminator,
        "",
    ]
    assert _attributes(artifact.nodes[1])["rowCount"] == 2


@pytest.mark.parametrize(
    "source",
    [
        b"",
        b'"unterminated',
        b'a"b,c',
        b'"a" x,b',
        b'"a""',
        b'"a"x',
    ],
)
def test_csv_rejects_empty_or_malformed_quote_grammar(source: bytes) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_csv_rejects_ragged_rows_without_padding_or_truncation() -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(b"a,b\r\nc\r\n")

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    ("source", "limits"),
    [
        (b"a\nb\n", NativeParserLimits(csv_rows=1)),
        (b"a,b\n", NativeParserLimits(csv_columns=1)),
        ("éé\n".encode(), NativeParserLimits(csv_field_bytes=3)),
        ("中文\n".encode("gb18030"), NativeParserLimits(csv_field_bytes=4)),
        (b'"abcd"\n', NativeParserLimits(csv_field_bytes=5)),
    ],
)
def test_csv_admission_limits_fail_as_input_invalid(
    source: bytes,
    limits: NativeParserLimits,
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source, limits)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    "limits",
    [
        NativeParserLimits(nodes=3),
        NativeParserLimits(fragments=0),
    ],
)
def test_csv_native_artifact_cardinality_limits_remain_result_too_large(
    limits: NativeParserLimits,
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(b"value", limits)

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_csv_parser_is_byte_deterministic() -> None:
    source = b'alpha,"comma, and ""quote"""\r\nbeta,value\r\n'

    first = _parse(source)
    second = _parse(source)

    assert first.canonical_bytes == second.canonical_bytes
    assert first.artifact_sha256 == second.artifact_sha256
