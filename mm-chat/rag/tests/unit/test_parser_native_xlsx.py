"""C1.3 deterministic XLSX structure, Locator, and failure tests."""

from __future__ import annotations

import io
import zipfile
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeSourceUnitKind,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.native.opc import admit_ooxml_package
from mm_chat_rag.offline_parser.native.xlsx import parse_xlsx
from tests.support.parser_corpus import deterministic_zip

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_GOLDEN = _CORPUS / "golden" / "xlsx" / "representative.xlsx"
_DOCX = _CORPUS / "golden" / "docx" / "minimal.docx"


def _parse(
    source: bytes | None = None,
    limits: NativeParserLimits | None = None,
) -> NativeDocument:
    selected = limits or NativeParserLimits()
    content = _GOLDEN.read_bytes() if source is None else source
    package = admit_ooxml_package(content, selected)
    return parse_xlsx(package, selected)


def _attributes(node: NativeNode) -> dict[str, object]:
    return {attribute.name: attribute.value for attribute in node.attributes}


def _nodes(artifact: NativeDocument, kind: NativeNodeKind) -> list[NativeNode]:
    return [node for node in artifact.nodes if node.kind is kind]


def _cell(artifact: NativeDocument, sheet: int, reference: str) -> NativeNode:
    return next(
        node
        for node in _nodes(artifact, NativeNodeKind.TABLE_CELL)
        if _attributes(node)["sheetOrdinal"] == sheet
        and _attributes(node)["startCell"] == reference
    )


def _replace_part(source: bytes, name: str, old: bytes, new: bytes) -> bytes:
    parts: list[tuple[str, bytes]] = []
    replaced = False
    with zipfile.ZipFile(io.BytesIO(source), mode="r") as archive:
        for info in archive.infolist():
            content = archive.read(info.filename)
            if info.filename == name:
                assert old in content
                content = content.replace(old, new, 1)
                replaced = True
            parts.append((info.filename, content))
    assert replaced
    return deterministic_zip(parts)


def test_representative_xlsx_preserves_structure_formula_and_hidden_state() -> None:
    artifact = _parse()

    assert artifact.source_format is ParserFormat.XLSX
    assert artifact.source_units[0].kind is NativeSourceUnitKind.RAW_FILE
    assert artifact.source_units[0].encoding is None
    assert artifact.nodes[0].source_position.raw_byte_end == len(_GOLDEN.read_bytes())

    sheets = _nodes(artifact, NativeNodeKind.SHEET)
    assert [_attributes(node)["sheetName"] for node in sheets] == [
        "Visible",
        "Hidden",
    ]
    assert [_attributes(node)["sheetState"] for node in sheets] == [
        "visible",
        "hidden",
    ]
    assert [_attributes(node)["nonIndexable"] for node in sheets] == [False, True]
    assert [node.parent_ordinal for node in sheets] == [0, 0]

    visible_table = _nodes(artifact, NativeNodeKind.TABLE)[0]
    assert _attributes(visible_table) == {
        "columnCount": 3,
        "endCell": "C3",
        "nonIndexable": False,
        "ooxmlPath": "/worksheet[1]/sheetData[1]",
        "rowCount": 3,
        "sheetOrdinal": 0,
        "startCell": "A1",
    }

    a1 = _cell(artifact, 0, "A1")
    b1 = _cell(artifact, 0, "B1")
    c1 = _cell(artifact, 0, "C1")
    a2 = _cell(artifact, 0, "A2")
    a3 = _cell(artifact, 0, "A3")
    hidden_a1 = _cell(artifact, 1, "A1")
    assert a1.fragments[0].text == "Minimal XLSX"
    assert b1.fragments[0].text == "1"
    assert c1.fragments[0].text == "2"
    assert c1.fragments[0].role is NativeFragmentRole.CACHED_VALUE
    assert _attributes(a2)["nonIndexable"] is True
    assert _attributes(hidden_a1)["nonIndexable"] is True
    assert (_attributes(a3)["endCell"], _attributes(a3)["columnSpan"]) == (
        "B3",
        2,
    )

    formulas = _nodes(artifact, NativeNodeKind.FORMULA)
    assert len(formulas) == 1
    assert formulas[0].parent_ordinal == c1.ordinal
    assert formulas[0].fragments[0].text == "SUM(B1,1)"
    assert formulas[0].fragments[0].role is NativeFragmentRole.FORMULA
    assert _attributes(formulas[0])["cachedValuePresent"] is True


def test_xlsx_shared_string_fragments_bind_exact_part_bytes_and_reuse() -> None:
    source = _GOLDEN.read_bytes()
    limits = NativeParserLimits()
    package = admit_ooxml_package(source, limits)
    artifact = parse_xlsx(package, limits)
    shared_bytes = package.read_part("/xl/sharedStrings.xml")

    a1 = _cell(artifact, 0, "A1")
    a2 = _cell(artifact, 0, "A2")
    hidden_a1 = _cell(artifact, 1, "A1")
    fragment = a1.fragments[0]
    position = fragment.source_position
    assert fragment.transform is NativeTransformKind.SYNTAX_DECODE
    assert artifact.source_units[position.source_unit_ordinal].canonical_uri == (
        "/xl/sharedStrings.xml"
    )
    assert shared_bytes[position.raw_byte_start : position.raw_byte_end] == (
        b"Minimal XLSX"
    )
    assert a2.fragments[0].source_position == hidden_a1.fragments[0].source_position
    assert a2.fragments[0].text == hidden_a1.fragments[0].text == "hidden"


def test_xlsx_formula_and_cached_value_have_distinct_exact_part_positions() -> None:
    source = _GOLDEN.read_bytes()
    limits = NativeParserLimits()
    package = admit_ooxml_package(source, limits)
    artifact = parse_xlsx(package, limits)
    worksheet = package.read_part("/xl/worksheets/sheet1.xml")

    c1 = _cell(artifact, 0, "C1")
    formula = _nodes(artifact, NativeNodeKind.FORMULA)[0]
    cached_position = c1.fragments[0].source_position
    formula_position = formula.fragments[0].source_position
    assert cached_position.source_unit_ordinal == formula_position.source_unit_ordinal
    assert (
        worksheet[cached_position.raw_byte_start : cached_position.raw_byte_end] == b"2"
    )
    assert (
        worksheet[formula_position.raw_byte_start : formula_position.raw_byte_end]
        == b"SUM(B1,1)"
    )


def test_xlsx_inline_string_hidden_column_and_style_baseline() -> None:
    source = _GOLDEN.read_bytes()
    source = _replace_part(
        source,
        "xl/worksheets/sheet1.xml",
        b"<sheetData>",
        b'<cols><col min="2" max="2" hidden="1"/></cols><sheetData>',
    )
    source = _replace_part(
        source,
        "xl/worksheets/sheet1.xml",
        b'<c r="B1"><v>1</v></c>',
        b'<c r="B1" s="0" t="inlineStr"><is><t>inline</t></is></c>',
    )

    artifact = _parse(source)
    b1 = _cell(artifact, 0, "B1")

    assert b1.fragments[0].text == "inline"
    assert b1.fragments[0].transform is NativeTransformKind.SYNTAX_DECODE
    assert _attributes(b1)["hidden"] is True
    assert _attributes(b1)["nonIndexable"] is True
    assert _attributes(b1)["styleIndex"] == 0
    assert _attributes(b1)["numberFormatId"] == 0


def test_xlsx_formula_without_cached_value_remains_explicit_and_unevaluated() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        b"<f>SUM(B1,1)</f><v>2</v>",
        b"<f>SUM(B1,1)</f>",
    )

    artifact = _parse(source)
    c1 = _cell(artifact, 0, "C1")
    formula = _nodes(artifact, NativeNodeKind.FORMULA)[0]

    assert c1.fragments == ()
    assert _attributes(c1)["valueKind"] == "blank"
    assert _attributes(formula)["cachedValuePresent"] is False
    assert formula.fragments[0].text == "SUM(B1,1)"


@pytest.mark.parametrize(
    ("old", "new"),
    [
        (b'<c r="A1" t="s">', b'<c r="a1" t="s">'),
        (b'<c r="A1" t="s"><v>0</v>', b'<c r="A2" t="s"><v>0</v>'),
        (b'<c r="B1"><v>1</v>', b'<c r="A1"><v>1</v>'),
        (b'<c r="A1" t="s"><v>0</v>', b'<c r="A1" t="s"><v>99</v>'),
        (b'<c r="B1"><v>1</v>', b'<c r="B1" t="d"><v>1</v>'),
        (b'<c r="B1"><v>1</v>', b'<c r="B1" s="99"><v>1</v>'),
        (b'<row r="2" hidden="1">', b'<row r="1" hidden="1">'),
    ],
)
def test_xlsx_rejects_invalid_cell_reference_index_type_and_style(
    old: bytes,
    new: bytes,
) -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        old,
        new,
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_rejects_shared_or_array_formula_without_evaluation() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        b"<f>SUM(B1,1)</f>",
        b'<f t="shared" si="0">SUM(B1,1)</f>',
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED


@pytest.mark.parametrize(
    ("old", "new"),
    [
        (b'ref="A3:B3"', b'ref="B3:A3"'),
        (b'ref="A3:B3"', b'ref="B3:C3"'),
        (
            b'<c r="A3" t="s"><v>2</v></c>',
            b'<c r="A3" t="s"><v>2</v></c><c r="B3"><v>4</v></c>',
        ),
        (
            b'<mergeCells count="1"><mergeCell ref="A3:B3"/></mergeCells>',
            b'<mergeCells count="2"><mergeCell ref="A3:B3"/>'
            b'<mergeCell ref="A3:C3"/></mergeCells>',
        ),
    ],
)
def test_xlsx_rejects_invalid_missing_covered_or_overlapping_merge(
    old: bytes,
    new: bytes,
) -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        old,
        new,
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    "limits",
    [
        NativeParserLimits(sheets=1),
        NativeParserLimits(shared_strings=2),
        NativeParserLimits(cells=4),
        NativeParserLimits(merged_ranges=0),
    ],
)
def test_xlsx_structure_limits_fail_closed(limits: NativeParserLimits) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(limits=limits)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_external_worksheet_relationship_is_rejected_before_parse() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/_rels/workbook.xml.rels",
        b'Type="http://schemas.openxmlformats.org/officeDocument/2006/'
        b'relationships/worksheet" Target="worksheets/sheet1.xml"/>',
        b'Type="http://schemas.openxmlformats.org/officeDocument/2006/'
        b'relationships/worksheet" Target="https://example.invalid/sheet.xml" '
        b'TargetMode="External"/>',
    )

    with pytest.raises(NativeParseFailure) as observed:
        admit_ooxml_package(source, NativeParserLimits())

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_rejects_a_valid_non_xlsx_opc_package() -> None:
    limits = NativeParserLimits()
    package = admit_ooxml_package(_DOCX.read_bytes(), limits)

    with pytest.raises(NativeParseFailure) as observed:
        parse_xlsx(package, limits)

    assert observed.value.code is StableErrorCode.FORMAT_MISMATCH


@pytest.mark.parametrize(
    ("old", "new", "expected"),
    [
        (
            b'<workbook xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            b'<notWorkbook xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<sheets>",
            b"<notSheets>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"</sheets><calcPr",
            b"</sheets><definedNames/><calcPr",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<sheet name="Visible" sheetId="1" r:id="rId1"/>',
            b'<sheet name="Visible" sheetId="1" r:id="rId1"><x/></sheet>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<sheet name="Visible" sheetId="1" r:id="rId1"/>',
            b'<sheet name="Visible" sheetId="1" r:id="rId1" foo="x"/>',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<sheet name="Visible" sheetId="1" r:id="rId1"/>',
            b'<sheet name="" sheetId="1" r:id="rId1"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/>',
            b'<sheet name="visible" sheetId="2" state="hidden" r:id="rId2"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/>',
            b'<sheet name="Hidden" sheetId="1" state="hidden" r:id="rId2"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'state="hidden"',
            b'state="archived"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<sheet name="Visible" sheetId="1" r:id="rId1"/>',
            b'<sheet name="Visible" sheetId="1"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"</sheets><calcPr",
            b"</sheets>not-whitespace<calcPr",
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_xlsx_rejects_invalid_or_unsupported_workbook_markup(
    old: bytes,
    new: bytes,
    expected: StableErrorCode,
) -> None:
    source = _replace_part(_GOLDEN.read_bytes(), "xl/workbook.xml", old, new)

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is expected


def test_xlsx_rejects_unreferenced_or_non_worksheet_relationships() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/workbook.xml",
        b'<sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/>',
        b"",
    )
    with pytest.raises(NativeParseFailure) as unreferenced:
        _parse(source)
    assert unreferenced.value.code is StableErrorCode.INPUT_INVALID

    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/_rels/workbook.xml.rels",
        b'/relationships/styles"',
        b'/relationships/theme"',
    )
    with pytest.raises(NativeParseFailure) as unsupported:
        _parse(source)
    assert unsupported.value.code is StableErrorCode.FORMAT_UNSUPPORTED


def test_xlsx_rejects_worksheet_relationship_to_a_non_worksheet_part() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/_rels/workbook.xml.rels",
        b'Target="worksheets/sheet1.xml"',
        b'Target="styles.xml"',
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_accepts_absent_optional_shared_strings_and_styles_parts() -> None:
    source = _GOLDEN.read_bytes()
    source = _replace_part(
        source,
        "xl/_rels/workbook.xml.rels",
        b'<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/sharedStrings" '
        b'Target="sharedStrings.xml"/>',
        b"",
    )
    source = _replace_part(
        source,
        "xl/_rels/workbook.xml.rels",
        b'<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/styles" Target="styles.xml"/>',
        b"",
    )
    for part, count in (
        ("xl/worksheets/sheet1.xml", 3),
        ("xl/worksheets/sheet2.xml", 1),
    ):
        for _index in range(count):
            source = _replace_part(source, part, b't="s"', b't="str"')

    artifact = _parse(source)

    assert _cell(artifact, 0, "A1").fragments[0].text == "0"
    assert _attributes(_cell(artifact, 0, "A1"))["numberFormatId"] == 0


def test_xlsx_rejects_shared_string_reference_count_mismatch() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/sharedStrings.xml",
        b'count="4"',
        b'count="5"',
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    ("old", "new", "expected"),
    [
        (
            b'<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"',
            b'<styleSheet xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'count="4" uniqueCount="3"',
            b'count="4" uniqueCount="3" foo="x"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<si><t>Minimal XLSX</t></si>",
            b"<si><t>Minimal XLSX</t><r><t>x</t></r></si>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<si><t>Minimal XLSX</t></si>",
            b"<si><phoneticPr/></si>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<t>Minimal XLSX</t>",
            b'<t xml:space="collapse">Minimal XLSX</t>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<t>Minimal XLSX</t>",
            b"<t><r/></t>",
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_xlsx_rejects_invalid_or_unsupported_shared_string_markup(
    old: bytes,
    new: bytes,
    expected: StableErrorCode,
) -> None:
    source = _replace_part(_GOLDEN.read_bytes(), "xl/sharedStrings.xml", old, new)

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is expected


def test_xlsx_accepts_rich_and_empty_shared_strings_without_flattening_runs() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/sharedStrings.xml",
        b"<si><t>Minimal XLSX</t></si>",
        b"<si><r><rPr/><t>Mini</t></r><r><t>mal XLSX</t></r></si>",
    )
    rich = _parse(source)
    assert [fragment.text for fragment in _cell(rich, 0, "A1").fragments] == [
        "Mini",
        "mal XLSX",
    ]

    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/sharedStrings.xml",
        b"<si><t>Minimal XLSX</t></si>",
        b"<si/>",
    )
    empty = _parse(source)
    assert _cell(empty, 0, "A1").fragments == ()


@pytest.mark.parametrize(
    ("old", "new"),
    [
        (
            b'<styleSheet xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            b'<notStyleSheet xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
        ),
        (
            b'<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" '
            b'borderId="0" xfId="0"/></cellXfs>',
            b'<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" '
            b'borderId="0" xfId="0"/></cellXfs><cellXfs count="0"/>',
        ),
        (
            b'<cellXfs count="1">',
            b'<cellXfs count="1" foo="x">',
        ),
        (
            b'<cellXfs count="1">',
            b'<cellXfs count="2">',
        ),
        (
            b'<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" '
            b'borderId="0" xfId="0"/></cellXfs>',
            b'<cellXfs count="1"><font numFmtId="0"/></cellXfs>',
        ),
        (
            b'<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" '
            b'borderId="0" xfId="0"/></cellXfs>',
            b'<cellXfs count="1"><xf fontId="0"/></cellXfs>',
        ),
        (
            b'<cellXfs count="1">',
            b'<cellXfs count="01">',
        ),
    ],
)
def test_xlsx_rejects_invalid_style_table(old: bytes, new: bytes) -> None:
    source = _replace_part(_GOLDEN.read_bytes(), "xl/styles.xml", old, new)

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_accepts_styles_part_without_cell_formats() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/styles.xml",
        b'<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" '
        b'borderId="0" xfId="0"/></cellXfs>',
        b"",
    )

    artifact = _parse(source)

    assert _attributes(_cell(artifact, 0, "B1"))["numberFormatId"] == 0


@pytest.mark.parametrize(
    ("old", "new", "expected"),
    [
        (
            b'<worksheet xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            b'<notWorksheet xmlns="http://schemas.openxmlformats.org/'
            b'spreadsheetml/2006/main"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"</sheetData><mergeCells",
            b"</sheetData><sheetViews/><mergeCells",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<sheetData>",
            b"<sheetData/><sheetData>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<row r="1">',
            b'<notRow r="1">',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<row r="1">',
            b'<row r="1" foo="x">',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<row r="1">',
            b'<row r="0">',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<row r="2" hidden="1">',
            b'<row r="2" hidden="maybe">',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<x r="B1"><v>1</v></x>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1">',
            b'<c r="B1" foo="x">',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1"><x/></c>',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<f>SUM(B1,1)</f><v>2</v>",
            b"<v>2</v><f>SUM(B1,1)</f>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="A1" t="s"><v>0</v></c>',
            b'<c r="A1" t="s"><v>0</v><is><t>x</t></is></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1" t="inlineStr"><v>1</v><is><t>x</t></is></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1"><is><t>x</t></is></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<f>SUM(B1,1)</f>",
            b"<f/>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<f>SUM(B1,1)</f>",
            b"<f><x/></f>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1"><v>NaN</v></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1" t="b"><v>2</v></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="B1" t="str"><v/></c>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<c r="B1"><v>1</v></c>',
            b'<c r="XFE1"><v>1</v></c>',
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_xlsx_rejects_invalid_or_unsupported_worksheet_markup(
    old: bytes,
    new: bytes,
    expected: StableErrorCode,
) -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        old,
        new,
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is expected


@pytest.mark.parametrize(
    ("old", "new", "expected"),
    [
        (
            b'<cols><col min="2" max="2" hidden="1"/></cols>',
            b"<cols><x/></cols>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<col min="2" max="2" hidden="1"/>',
            b'<col min="2" max="2" hidden="1" foo="x"/>',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<col min="2" max="2" hidden="1"/>',
            b'<col min="2" max="1" hidden="1"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<col min="2" max="2" hidden="1"/>',
            b'<col min="2" max="2" hidden="maybe"/>',
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_xlsx_rejects_invalid_or_unsupported_column_ranges(
    old: bytes,
    new: bytes,
    expected: StableErrorCode,
) -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        b"<sheetData>",
        b'<cols><col min="2" max="2" hidden="1"/></cols><sheetData>',
    )
    source = _replace_part(source, "xl/worksheets/sheet1.xml", old, new)

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is expected


def test_xlsx_accepts_explicit_visible_column_and_scalar_cell_types() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        b"<sheetData>",
        b'<cols><col min="2" max="2" hidden="0"/></cols><sheetData>',
    )
    source = _replace_part(
        source,
        "xl/worksheets/sheet1.xml",
        b'<c r="B1"><v>1</v></c><c r="C1">',
        b'<c r="B1" t="b"><v>1</v></c><c r="C1" t="str">',
    )
    source = _replace_part(
        source,
        "xl/worksheets/sheet1.xml",
        b"</f><v>2</v></c></row>",
        b'</f><v>cached</v></c><c r="D1" t="e"><v>#DIV/0!</v></c></row>',
    )

    artifact = _parse(source)

    assert _attributes(_cell(artifact, 0, "B1"))["valueKind"] == "boolean"
    assert _attributes(_cell(artifact, 0, "C1"))["valueKind"] == (
        "formula_cached_string"
    )
    assert _attributes(_cell(artifact, 0, "D1"))["valueKind"] == "error"
    assert _attributes(_cell(artifact, 0, "B1"))["hidden"] is False


def test_xlsx_accepts_empty_sheet_and_very_hidden_state() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/workbook.xml",
        b'state="hidden"',
        b'state="veryHidden"',
    )
    source = _replace_part(
        source,
        "xl/worksheets/sheet2.xml",
        b'<sheetData><row r="1"><c r="A1" t="s"><v>1</v></c></row></sheetData>',
        b"<sheetData/>",
    )
    source = _replace_part(
        source,
        "xl/sharedStrings.xml",
        b'count="4"',
        b'count="3"',
    )

    artifact = _parse(source)
    hidden_sheet = _nodes(artifact, NativeNodeKind.SHEET)[1]
    hidden_table = _nodes(artifact, NativeNodeKind.TABLE)[1]

    assert _attributes(hidden_sheet)["sheetState"] == "veryHidden"
    assert _attributes(hidden_table)["rowCount"] == 0
    assert _attributes(hidden_table)["startCell"] is None
    assert _attributes(hidden_table)["endCell"] is None


@pytest.mark.parametrize(
    ("old", "new"),
    [
        (
            b'<mergeCells count="1">',
            b'<mergeCells count="1" foo="x">',
        ),
        (
            b'<mergeCells count="1">',
            b'<mergeCells count="2">',
        ),
        (
            b'<mergeCell ref="A3:B3"/>',
            b'<x ref="A3:B3"/>',
        ),
        (
            b'<mergeCell ref="A3:B3"/>',
            b'<mergeCell ref="A3:B3" foo="x"/>',
        ),
        (
            b'<mergeCell ref="A3:B3"/>',
            b'<mergeCell ref="A3B3"/>',
        ),
        (
            b'<mergeCells count="1"><mergeCell ref="A3:B3"/></mergeCells>',
            b'<mergeCells count="2"><mergeCell ref="B4:C4"/>'
            b'<mergeCell ref="A3:B3"/></mergeCells>',
        ),
    ],
)
def test_xlsx_rejects_invalid_merge_container_and_entries(
    old: bytes,
    new: bytes,
) -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        old,
        new,
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_accepts_disjoint_merges_after_expiring_active_range() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/worksheets/sheet1.xml",
        b"</sheetData>",
        b'<row r="5"><c r="A5"><v>5</v></c></row></sheetData>',
    )
    source = _replace_part(
        source,
        "xl/worksheets/sheet1.xml",
        b'<mergeCells count="1"><mergeCell ref="A3:B3"/></mergeCells>',
        b'<mergeCells count="2"><mergeCell ref="A3:B3"/>'
        b'<mergeCell ref="A5:B5"/></mergeCells>',
    )

    artifact = _parse(source)

    assert _attributes(_cell(artifact, 0, "A5"))["endCell"] == "B5"
    assert _attributes(_cell(artifact, 0, "A5"))["columnSpan"] == 2


def test_xlsx_rejects_duplicate_sheet_target_and_duplicate_styles_relation() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/_rels/workbook.xml.rels",
        b'Target="worksheets/sheet2.xml"',
        b'Target="worksheets/sheet1.xml"',
    )
    with pytest.raises(NativeParseFailure) as duplicate_sheet:
        _parse(source)
    assert duplicate_sheet.value.code is StableErrorCode.INPUT_INVALID

    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/_rels/workbook.xml.rels",
        b"</Relationships>",
        b'<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/styles" Target="styles.xml"/>'
        b"</Relationships>",
    )
    with pytest.raises(NativeParseFailure) as duplicate_styles:
        _parse(source)
    assert duplicate_styles.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_accepts_strict_spreadsheet_and_relationship_namespaces() -> None:
    source = _GOLDEN.read_bytes()
    transitional_sheet = b"http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    strict_sheet = b"http://purl.oclc.org/ooxml/spreadsheetml/main"
    for part in (
        "xl/workbook.xml",
        "xl/sharedStrings.xml",
        "xl/styles.xml",
        "xl/worksheets/sheet1.xml",
        "xl/worksheets/sheet2.xml",
    ):
        source = _replace_part(source, part, transitional_sheet, strict_sheet)
    source = _replace_part(
        source,
        "xl/workbook.xml",
        b"http://schemas.openxmlformats.org/officeDocument/2006/relationships",
        b"http://purl.oclc.org/ooxml/officeDocument/relationships",
    )

    artifact = _parse(source)

    assert _cell(artifact, 0, "A1").fragments[0].text == "Minimal XLSX"


def test_xlsx_rejects_unknown_spreadsheet_namespace() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/workbook.xml",
        b"http://schemas.openxmlformats.org/spreadsheetml/2006/main",
        b"urn:example:unknown-spreadsheet",
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xlsx_native_artifact_round_trip_and_determinism() -> None:
    first = _parse()
    second = _parse()
    restored = NativeDocument.from_bytes(first.canonical_bytes)

    assert first.canonical_bytes == second.canonical_bytes
    assert first.artifact_sha256 == second.artifact_sha256
    assert restored == first


def test_xlsx_oversized_decimal_maps_to_input_invalid() -> None:
    source = _replace_part(
        _GOLDEN.read_bytes(),
        "xl/workbook.xml",
        b'sheetId="1"',
        b'sheetId="' + b"9" * 5000 + b'"',
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID
