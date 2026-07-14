"""C1.3B deterministic DOCX structure and exact Part locator tests."""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path
from typing import cast

import pytest

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.docx import parse_docx
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeNodeKind,
    NativeParseFailure,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.native.opc import (
    ValidatedOpcPackage,
    admit_ooxml_package,
)
from tests.support.parser_corpus import _minimal_docx_parts, deterministic_zip

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
_R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
_REL = "http://schemas.openxmlformats.org/package/2006/relationships"
_REL_BASE = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"


def _document(body: str) -> bytes:
    return (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        f'<w:document xmlns:w="{_W}" xmlns:r="{_R}"><w:body>{body}'
        "<w:sectPr/></w:body></w:document>"
    ).encode()


def _docx(
    body: str,
    *,
    relationships: bytes | None = None,
    extra_parts: tuple[tuple[str, bytes], ...] = (),
) -> bytes:
    parts = _minimal_docx_parts(
        document_xml=_document(body),
        relationships=relationships,
    )
    parts.extend(extra_parts)
    return deterministic_zip(parts)


def _docx_document(
    document_xml: bytes,
    *,
    relationships: bytes | None = None,
    extra_parts: tuple[tuple[str, bytes], ...] = (),
) -> bytes:
    parts = _minimal_docx_parts(
        document_xml=document_xml,
        relationships=relationships,
    )
    parts.extend(extra_parts)
    return deterministic_zip(parts)


def _relationships(*items: str) -> bytes:
    return (
        f'<?xml version="1.0"?><Relationships xmlns="{_REL}">'
        + "".join(items)
        + "</Relationships>"
    ).encode()


def _relationship(
    relationship_id: str,
    relationship_type: str,
    target: str,
    *,
    external: bool = False,
) -> str:
    target_mode = ' TargetMode="External"' if external else ""
    return (
        f'<Relationship Id="{relationship_id}" Type="{_REL_BASE}{relationship_type}" '
        f'Target="{target}"{target_mode}/>'
    )


def _note_part(root_name: str, content: str) -> bytes:
    return (
        f'<?xml version="1.0"?><w:{root_name} xmlns:w="{_W}">{content}</w:{root_name}>'
    ).encode()


def _parse(
    source: bytes,
    *,
    limits: NativeParserLimits = DEFAULT_CONFIG.native,
) -> NativeDocument:
    return parse_docx(admit_ooxml_package(source, limits), limits)


def test_minimal_docx_preserves_structure_and_exact_part_positions() -> None:
    source = (_CORPUS / "golden" / "docx" / "minimal.docx").read_bytes()

    artifact = _parse(source)

    assert artifact.source_format is ParserFormat.DOCX
    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.HEADING,
        NativeNodeKind.PARAGRAPH,
        NativeNodeKind.TABLE,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.PARAGRAPH,
    ]
    assert [item.text for item in artifact.nodes[1].fragments] == ["Minimal DOCX"]
    assert [item.text for item in artifact.nodes[2].fragments] == ["Unicode: 文档 café"]
    assert [item.text for item in artifact.nodes[6].fragments] == ["Cell"]
    expected = [(201, 213, 201, 213), (267, 288, 267, 283), (396, 400, 391, 395)]
    observed = []
    for node in (artifact.nodes[1], artifact.nodes[2], artifact.nodes[6]):
        position = node.fragments[0].source_position
        observed.append(
            (
                position.raw_byte_start,
                position.raw_byte_end,
                position.decoded_scalar_start,
                position.decoded_scalar_end,
            )
        )
        assert position.start_line == position.end_line == 0
        assert position.source_unit_ordinal > 0
    assert observed == expected
    assert {
        attribute.name: attribute.value for attribute in artifact.nodes[1].attributes
    } == {"level": 1, "styleId": "Title"}


def test_docx_entity_tab_and_break_keep_source_syntax() -> None:
    source = _docx(
        "<w:p><w:r><w:t>A&amp;B</w:t><w:tab/><w:br/><w:t>C</w:t></w:r></w:p>"
    )

    artifact = _parse(source)

    paragraph = artifact.nodes[1]
    assert "".join(item.text for item in paragraph.fragments) == "A&BC"
    entity = next(
        item
        for item in paragraph.fragments
        if item.transform is NativeTransformKind.SYNTAX_DECODE
    )
    part = next(
        unit
        for unit in artifact.source_units
        if unit.ordinal == entity.source_position.source_unit_ordinal
    )
    assert part.canonical_uri == "/word/document.xml"
    document_bytes = admit_ooxml_package(source, DEFAULT_CONFIG.native).read_part(
        "/word/document.xml"
    )
    assert (
        document_bytes[
            entity.source_position.raw_byte_start : entity.source_position.raw_byte_end
        ]
        == b"&amp;"
    )
    breaks = [node for node in artifact.nodes if node.kind is NativeNodeKind.LINE_BREAK]
    assert [{item.name: item.value for item in node.attributes} for node in breaks] == [
        {"breakKind": "tab"},
        {"breakKind": "line"},
    ]


def test_docx_consecutive_numbered_paragraphs_form_one_native_list() -> None:
    item = (
        '<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/>'
        "</w:numPr></w:pPr><w:r><w:t>{}</w:t></w:r></w:p>"
    )
    source = _docx(item.format("one") + item.format("two"))

    artifact = _parse(source)

    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.LIST,
        NativeNodeKind.LIST_ITEM,
        NativeNodeKind.LIST_ITEM,
    ]
    assert [node.parent_ordinal for node in artifact.nodes[2:]] == [1, 1]
    assert [node.fragments[0].text for node in artifact.nodes[2:]] == ["one", "two"]
    assert {item.name: item.value for item in artifact.nodes[2].attributes} == {
        "level": 0,
        "numberingId": 7,
        "styleId": None,
    }


def test_docx_heading_run_properties_and_distinct_lists_remain_structural() -> None:
    numbered = (
        '<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="{}"/>'
        "</w:numPr></w:pPr><w:r><w:t>{}</w:t></w:r></w:p>"
    )
    source = _docx(
        '<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr>'
        "<w:r><w:rPr/><w:t>section</w:t></w:r></w:p>"
        + numbered.format(7, "one")
        + numbered.format(8, "two")
    )

    artifact = _parse(source)

    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.HEADING,
        NativeNodeKind.LIST,
        NativeNodeKind.LIST_ITEM,
        NativeNodeKind.LIST,
        NativeNodeKind.LIST_ITEM,
    ]
    assert {item.name: item.value for item in artifact.nodes[1].attributes} == {
        "level": 2,
        "styleId": "Heading2",
    }
    assert [artifact.nodes[index].attributes[0].value for index in (2, 4)] == [7, 8]


@pytest.mark.parametrize(
    ("document_xml", "code"),
    [
        (
            f'<w:notDocument xmlns:w="{_W}"/>'.encode(),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            f'<w:document xmlns:w="{_W}"/>'.encode(),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            f'<w:document xmlns:w="{_W}"><w:body/><w:body/></w:document>'.encode(),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            f'<w:document xmlns:w="{_W}"><w:body><w:custom/></w:body>'
            "</w:document>".encode(),
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
    ],
)
def test_docx_document_shape_fails_closed(
    document_xml: bytes,
    code: StableErrorCode,
) -> None:
    package = admit_ooxml_package(
        _docx_document(document_xml),
        DEFAULT_CONFIG.native,
    )

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


def test_docx_external_hyperlink_is_non_dereferenced_asset_metadata() -> None:
    relationships = (
        b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/'
        b'relationships"><Relationship Id="rId9" Type="http://schemas.'
        b"openxmlformats.org/officeDocument/2006/relationships/hyperlink"
        b'" Target="https://example.invalid/never-fetch" TargetMode="External"/>'
        b"</Relationships>"
    )
    source = _docx(
        '<w:p><w:hyperlink r:id="rId9"><w:r><w:t>link</w:t></w:r></w:hyperlink></w:p>',
        relationships=relationships,
    )

    artifact = _parse(source)

    paragraph = artifact.nodes[1]
    asset = artifact.nodes[2]
    assert paragraph.fragments[0].text == "link"
    assert asset.kind is NativeNodeKind.ASSET_REF
    assert {item.name: item.value for item in asset.attributes} == {
        "external": True,
        "externalTarget": "https://example.invalid/never-fetch",
        "nonIndexable": True,
        "relationshipId": "rId9",
    }


def test_docx_anchor_hyperlink_is_local_non_indexable_metadata() -> None:
    source = _docx(
        '<w:p><w:hyperlink w:anchor="section-1"><w:r><w:t>jump</w:t></w:r>'
        "</w:hyperlink></w:p>"
    )

    artifact = _parse(source)

    assert artifact.nodes[1].fragments[0].text == "jump"
    assert {item.name: item.value for item in artifact.nodes[2].attributes} == {
        "external": False,
        "externalTarget": None,
        "nonIndexable": True,
        "relationshipId": None,
    }


@pytest.mark.parametrize(
    ("body", "relationships", "code"),
    [
        (
            "<w:p><w:hyperlink><w:r><w:t>missing</w:t></w:r></w:hyperlink></w:p>",
            None,
            StableErrorCode.INPUT_INVALID,
        ),
        (
            '<w:p><w:hyperlink r:id="rId9"><w:r><w:t>wrong</w:t></w:r>'
            "</w:hyperlink></w:p>",
            _relationships(
                _relationship(
                    "rId9",
                    "externalLink",
                    "https://example.invalid/external",
                    external=True,
                )
            ),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            '<w:p><w:hyperlink w:anchor="local"><w:bookmarkStart/></w:hyperlink></w:p>',
            None,
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
    ],
)
def test_docx_malformed_hyperlinks_fail_closed(
    body: str,
    relationships: bytes | None,
    code: StableErrorCode,
) -> None:
    package = admit_ooxml_package(
        _docx(body, relationships=relationships),
        DEFAULT_CONFIG.native,
    )

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


def test_docx_footnotes_are_retained_in_numeric_order() -> None:
    relationships = (
        b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/'
        b'relationships"><Relationship Id="rId4" Type="http://schemas.'
        b"openxmlformats.org/officeDocument/2006/relationships/footnotes"
        b'" Target="footnotes.xml"/></Relationships>'
    )
    footnotes = (
        f'<?xml version="1.0"?><w:footnotes xmlns:w="{_W}">'
        '<w:footnote w:id="2"><w:p><w:r><w:t>second</w:t></w:r></w:p>'
        '</w:footnote><w:footnote w:id="1"><w:p><w:r><w:t>first</w:t></w:r>'
        "</w:p></w:footnote></w:footnotes>"
    ).encode()
    source = _docx(
        "<w:p><w:r><w:t>body</w:t></w:r></w:p>",
        relationships=relationships,
        extra_parts=(("word/footnotes.xml", footnotes),),
    )

    artifact = _parse(source)

    notes = [node for node in artifact.nodes if node.kind is NativeNodeKind.FOOTNOTE]
    assert [{item.name: item.value for item in note.attributes} for note in notes] == [
        {"noteId": 1},
        {"noteId": 2},
    ]
    assert [artifact.nodes[note.ordinal + 1].fragments[0].text for note in notes] == [
        "first",
        "second",
    ]


def test_docx_table_properties_nested_table_and_merges_are_retained() -> None:
    source = _docx(
        "<w:tbl><w:tblPr/><w:tblGrid/><w:tr><w:trPr><w:tblHeader/>"
        '</w:trPr><w:tc><w:tcPr><w:gridSpan w:val="2"/>'
        '<w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>outer</w:t>'
        "</w:r></w:p><w:tbl><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr>"
        "<w:p><w:r><w:t>nested</w:t></w:r></w:p></w:tc></w:tr></w:tbl>"
        "</w:tc><w:tc><w:tcPr/><w:p/></w:tc></w:tr></w:tbl>"
    )

    artifact = _parse(source)

    rows = [node for node in artifact.nodes if node.kind is NativeNodeKind.TABLE_ROW]
    cells = [node for node in artifact.nodes if node.kind is NativeNodeKind.TABLE_CELL]
    assert {item.name: item.value for item in rows[0].attributes} == {
        "header": True,
        "rowIndex": 0,
    }
    assert [{item.name: item.value for item in cell.attributes} for cell in cells] == [
        {
            "columnIndex": 0,
            "columnSpan": 2,
            "rowIndex": 0,
            "verticalMerge": "restart",
        },
        {
            "columnIndex": 0,
            "columnSpan": 1,
            "rowIndex": 0,
            "verticalMerge": "continue",
        },
        {
            "columnIndex": 2,
            "columnSpan": 1,
            "rowIndex": 0,
            "verticalMerge": None,
        },
    ]
    assert [
        fragment.text for node in artifact.nodes for fragment in node.fragments
    ] == ["outer", "nested"]


@pytest.mark.parametrize(
    ("body", "code"),
    [
        (
            "<w:tbl><w:custom/></w:tbl>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            "<w:tbl><w:tr><w:custom/></w:tr></w:tbl>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            "<w:tbl><w:tr><w:tc><w:custom/></w:tc></w:tr></w:tbl>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            '<w:tbl><w:tr><w:tc><w:tcPr><w:gridSpan w:val="0"/></w:tcPr>'
            "<w:p/></w:tc></w:tr></w:tbl>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            '<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="invalid"/></w:tcPr>'
            "<w:p/></w:tc></w:tr></w:tbl>",
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_docx_malformed_table_content_fails_closed(
    body: str,
    code: StableErrorCode,
) -> None:
    package = admit_ooxml_package(_docx(body), DEFAULT_CONFIG.native)

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


def test_docx_cell_limit_fails_closed() -> None:
    source = _docx("<w:tbl><w:tr><w:tc><w:p/></w:tc></w:tr></w:tbl>")
    limits = replace(DEFAULT_CONFIG.native, cells=0)
    package = admit_ooxml_package(source, limits)

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, limits)

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_docx_nonpositive_notes_are_ignored_and_note_tables_are_retained() -> None:
    relationships = _relationships(_relationship("rId4", "footnotes", "footnotes.xml"))
    footnotes = _note_part(
        "footnotes",
        '<w:footnote w:id="-1"><w:p/></w:footnote>'
        '<w:footnote w:id="0"><w:p/></w:footnote>'
        '<w:footnote w:id="1"><w:tbl><w:tr><w:tc><w:p><w:r>'
        "<w:t>table note</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:footnote>",
    )
    source = _docx(
        "<w:p/>",
        relationships=relationships,
        extra_parts=(("word/footnotes.xml", footnotes),),
    )

    artifact = _parse(source)

    notes = [node for node in artifact.nodes if node.kind is NativeNodeKind.FOOTNOTE]
    assert [{item.name: item.value for item in note.attributes} for note in notes] == [
        {"noteId": 1}
    ]
    assert any(node.kind is NativeNodeKind.TABLE for node in artifact.nodes)
    assert [
        fragment.text
        for node in artifact.nodes
        for fragment in node.fragments
        if fragment.text
    ] == ["table note"]


@pytest.mark.parametrize(
    ("relationships", "extra_parts", "code"),
    [
        (
            _relationships(
                _relationship("rId4", "footnotes", "footnotes.xml"),
                _relationship("rId5", "footnotes", "footnotes2.xml"),
            ),
            (
                ("word/footnotes.xml", _note_part("footnotes", "")),
                ("word/footnotes2.xml", _note_part("footnotes", "")),
            ),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            _relationships(_relationship("rId4", "footnotes", "footnotes.xml")),
            (("word/footnotes.xml", _note_part("endnotes", "")),),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            _relationships(_relationship("rId4", "footnotes", "footnotes.xml")),
            (("word/footnotes.xml", _note_part("footnotes", "<w:custom/>")),),
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            _relationships(_relationship("rId4", "footnotes", "footnotes.xml")),
            (
                (
                    "word/footnotes.xml",
                    _note_part(
                        "footnotes",
                        '<w:footnote w:id="1"/><w:footnote w:id="1"/>',
                    ),
                ),
            ),
            StableErrorCode.INPUT_INVALID,
        ),
        (
            _relationships(_relationship("rId4", "footnotes", "footnotes.xml")),
            (
                (
                    "word/footnotes.xml",
                    _note_part(
                        "footnotes",
                        '<w:footnote w:id="1"><w:custom/></w:footnote>',
                    ),
                ),
            ),
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            _relationships(_relationship("rId4", "footnotes", "footnotes.xml")),
            (
                (
                    "word/footnotes.xml",
                    _note_part("footnotes", '<w:footnote w:id="bad"/>'),
                ),
            ),
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_docx_malformed_note_parts_fail_closed(
    relationships: bytes,
    extra_parts: tuple[tuple[str, bytes], ...],
    code: StableErrorCode,
) -> None:
    package = admit_ooxml_package(
        _docx("<w:p/>", relationships=relationships, extra_parts=extra_parts),
        DEFAULT_CONFIG.native,
    )

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


@pytest.mark.parametrize(
    ("body", "code"),
    [
        ('<w:altChunk r:id="rId9"/>', StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED),
        (
            "<w:p><w:del><w:r><w:delText>deleted</w:delText></w:r></w:del></w:p>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
    ],
)
def test_docx_active_and_deleted_content_fail_closed(
    body: str,
    code: StableErrorCode,
) -> None:
    package = admit_ooxml_package(_docx(body), DEFAULT_CONFIG.native)

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


@pytest.mark.parametrize(
    "body",
    [
        '<w:p><w:pPr><w:pStyle w:val=""/></w:pPr></w:p>',
        ('<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/></w:numPr></w:pPr></w:p>'),
        (
            '<w:p><w:pPr><w:numPr><w:ilvl w:val="9"/>'
            '<w:numId w:val="1"/></w:numPr></w:pPr></w:p>'
        ),
        (
            '<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/>'
            '<w:numId w:val="-1"/></w:numPr></w:pPr></w:p>'
        ),
        "<w:p><w:pPr/><w:pPr/></w:p>",
        "<w:p><w:r><w:t><w:br/></w:t></w:r></w:p>",
    ],
)
def test_docx_invalid_paragraph_and_run_structure_fails_closed(body: str) -> None:
    package = admit_ooxml_package(_docx(body), DEFAULT_CONFIG.native)

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    "body",
    [
        '<w:p><w:r><w:t w:unexpected="1">text</w:t></w:r></w:p>',
        "<w:p><w:r><w:drawing/></w:r></w:p>",
    ],
)
def test_docx_unknown_run_content_is_unsupported(body: str) -> None:
    package = admit_ooxml_package(
        _docx(body),
        DEFAULT_CONFIG.native,
    )

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED


def test_docx_node_limit_and_format_binding_fail_closed() -> None:
    source = (_CORPUS / "golden" / "docx" / "minimal.docx").read_bytes()
    limits = replace(DEFAULT_CONFIG.native, nodes=2)
    package = admit_ooxml_package(source, limits)

    with pytest.raises(NativeParseFailure) as limited:
        parse_docx(package, limits)

    assert limited.value.code is StableErrorCode.RESULT_TOO_LARGE
    pptx = (_CORPUS / "golden" / "pptx" / "minimal.pptx").read_bytes()
    wrong_package = admit_ooxml_package(pptx, DEFAULT_CONFIG.native)
    with pytest.raises(NativeParseFailure) as mismatch:
        parse_docx(wrong_package, DEFAULT_CONFIG.native)
    assert mismatch.value.code is StableErrorCode.FORMAT_MISMATCH


@pytest.mark.parametrize(
    "limits",
    [
        replace(DEFAULT_CONFIG.native, fragments=0),
        replace(DEFAULT_CONFIG.native, attributes=0),
    ],
)
def test_docx_fragment_and_attribute_limits_fail_closed(
    limits: NativeParserLimits,
) -> None:
    source = _docx("<w:p><w:r><w:t>text</w:t></w:r></w:p>")
    package = admit_ooxml_package(source, limits)

    with pytest.raises(NativeParseFailure) as observed:
        parse_docx(package, limits)

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_docx_rejects_unvalidated_inputs_and_invalid_limit_types() -> None:
    with pytest.raises(TypeError, match="validated OPC package"):
        parse_docx(
            cast("ValidatedOpcPackage", object()),
            DEFAULT_CONFIG.native,
        )

    source = (_CORPUS / "golden" / "docx" / "minimal.docx").read_bytes()
    package = admit_ooxml_package(source, DEFAULT_CONFIG.native)
    with pytest.raises(TypeError, match="limits have an invalid type"):
        parse_docx(package, cast("NativeParserLimits", object()))


def test_docx_output_is_byte_deterministic() -> None:
    source = (_CORPUS / "golden" / "docx" / "minimal.docx").read_bytes()

    first = _parse(source).canonical_bytes
    second = _parse(source).canonical_bytes

    assert first == second


def test_docx_rejects_header_footer_references_instead_of_dropping_text() -> None:
    document = (
        f'<?xml version="1.0"?><w:document xmlns:w="{_W}" xmlns:r="{_R}">'
        "<w:body><w:p><w:r><w:t>body</w:t></w:r></w:p><w:sectPr>"
        '<w:headerReference r:id="rId5"/></w:sectPr></w:body></w:document>'
    ).encode()
    relationships = _relationships(
        _relationship("rId5", "header", "header1.xml"),
    )
    header = (
        f'<?xml version="1.0"?><w:hdr xmlns:w="{_W}">'
        "<w:p><w:r><w:t>header-only-text</w:t></w:r></w:p></w:hdr>"
    ).encode()
    source = _docx_document(
        document,
        relationships=relationships,
        extra_parts=(("word/header1.xml", header),),
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED


def test_docx_oversized_decimal_maps_to_input_invalid() -> None:
    source = _docx(
        '<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/>'
        f'<w:numId w:val="{"9" * 5000}"/></w:numPr></w:pPr>'
        "<w:r><w:t>item</w:t></w:r></w:p>"
    )

    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID
