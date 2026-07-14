"""C1.3B deterministic PPTX slide, shape, table, and geometry tests."""

from __future__ import annotations

import zipfile
from dataclasses import replace
from io import BytesIO
from pathlib import Path
from typing import Any, cast

import pytest

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
)
from mm_chat_rag.offline_parser.native.opc import admit_ooxml_package
from mm_chat_rag.offline_parser.native.pptx import parse_pptx
from tests.support.parser_corpus import deterministic_zip

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_PPTX = _CORPUS / "golden" / "pptx" / "minimal.pptx"
_DOCX = _CORPUS / "golden" / "docx" / "minimal.docx"
_P = "http://schemas.openxmlformats.org/presentationml/2006/main"
_A = "http://schemas.openxmlformats.org/drawingml/2006/main"
_R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


def _parts(source: bytes) -> dict[str, bytes]:
    with zipfile.ZipFile(BytesIO(source)) as archive:
        return {name: archive.read(name) for name in archive.namelist()}


def _package(parts: dict[str, bytes]) -> bytes:
    return deterministic_zip(parts.items())


def _updated(source: bytes, **updates: bytes) -> bytes:
    parts = _parts(source)
    parts.update(updates)
    return _package(parts)


def _parse(
    source: bytes,
    *,
    limits: NativeParserLimits = DEFAULT_CONFIG.native,
) -> NativeDocument:
    return parse_pptx(admit_ooxml_package(source, limits), limits)


def _attributes(node: NativeNode) -> dict[str, object]:
    return {item.name: item.value for item in node.attributes}


def _replace_shape(parts: dict[str, bytes], replacement: bytes) -> None:
    slide = parts["ppt/slides/slide1.xml"]
    shape_start = slide.index(b"<p:sp>")
    shape_end = slide.index(b"</p:sp>") + len(b"</p:sp>")
    parts["ppt/slides/slide1.xml"] = (
        slide[:shape_start] + replacement + slide[shape_end:]
    )


def _graphic_frame(table_children: bytes) -> bytes:
    return (
        b'<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Table"/>'
        b"<p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm>"
        b'<a:off x="914400" y="914400"/><a:ext cx="4000000" cy="2000000"/>'
        b'</p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.'
        b'org/drawingml/2006/table"><a:tbl>'
        + table_children
        + b"</a:tbl></a:graphicData></a:graphic></p:graphicFrame>"
    )


def _picture(*, embed: bytes = b'r:embed="rId2"') -> bytes:
    return (
        b'<p:pic><p:nvPicPr><p:cNvPr id="2" name="Picture"/>'
        b"<p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip "
        + embed
        + b"/></p:blipFill><p:spPr><a:xfrm>"
        b'<a:off x="914400" y="914400"/><a:ext cx="1000000" cy="1000000"/>'
        b"</a:xfrm></p:spPr></p:pic>"
    )


def _add_internal_notes(parts: dict[str, bytes], notes: bytes) -> None:
    parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace(
        b"</Types>",
        b'<Override PartName="/ppt/notesSlides/notesSlide1.xml" ContentType="'
        b"application/vnd.openxmlformats-officedocument.presentationml.notesSlide+"
        b'xml"/></Types>',
    )
    parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
        "ppt/slides/_rels/slide1.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
        b"officeDocument/2006/relationships/notesSlide"
        b'" Target="../notesSlides/notesSlide1.xml"/></Relationships>',
    )
    parts["ppt/notesSlides/notesSlide1.xml"] = notes


def _assert_parse_failure(
    source: bytes,
    code: StableErrorCode,
    *,
    limits: NativeParserLimits = DEFAULT_CONFIG.native,
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source, limits=limits)
    assert observed.value.code is code


def test_minimal_pptx_preserves_slide_shape_text_and_exact_geometry() -> None:
    artifact = _parse(_PPTX.read_bytes())

    assert artifact.source_format is ParserFormat.PPTX
    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.SLIDE,
        NativeNodeKind.SHAPE,
        NativeNodeKind.PARAGRAPH,
    ]
    assert _attributes(artifact.nodes[1]) == {
        "heightMilliPoint": 540000,
        "hidden": False,
        "slideId": 256,
        "slideIndex": 0,
        "widthMilliPoint": 960000,
    }
    shape = _attributes(artifact.nodes[2])
    assert shape["shapeId"] == 2
    assert shape["shapeOrdinal"] == 0
    assert shape["name"] == "Title"
    assert [
        shape["bboxX1MilliPoint"],
        shape["bboxY1MilliPoint"],
        shape["bboxX2MilliPoint"],
        shape["bboxY2MilliPoint"],
    ] == [72000, 72000, 859402, 150740]
    fragment = artifact.nodes[3].fragments[0]
    assert fragment.text == "Minimal PPTX"
    assert (
        fragment.source_position.raw_byte_start,
        fragment.source_position.raw_byte_end,
        fragment.source_position.decoded_scalar_start,
        fragment.source_position.decoded_scalar_end,
    ) == (626, 638, 626, 638)
    assert fragment.source_position.start_line == 0


def test_pptx_slide_order_comes_from_presentation_relationships() -> None:
    source = _PPTX.read_bytes()
    parts = _parts(source)
    parts["ppt/slides/slide2.xml"] = parts["ppt/slides/slide1.xml"].replace(
        b"Minimal PPTX",
        b"Second PPTX",
    )
    parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace(
        b"</Types>",
        b'<Override PartName="/ppt/slides/slide2.xml" ContentType="application/'
        b"vnd.openxmlformats-officedocument.presentationml.slide+xml"
        b'"/></Types>',
    )
    parts["ppt/_rels/presentation.xml.rels"] = parts[
        "ppt/_rels/presentation.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/'
        b"officeDocument/2006/relationships/slide"
        b'" Target="slides/slide2.xml"/></Relationships>',
    )
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b'<p:sldId id="256" r:id="rId2"/>',
        b'<p:sldId id="257" r:id="rId3"/><p:sldId id="256" r:id="rId2"/>',
    )

    artifact = _parse(_package(parts))

    slides = [node for node in artifact.nodes if node.kind is NativeNodeKind.SLIDE]
    paragraphs = [
        node for node in artifact.nodes if node.kind is NativeNodeKind.PARAGRAPH
    ]
    assert [_attributes(node)["slideId"] for node in slides] == [257, 256]
    assert [node.fragments[0].text for node in paragraphs] == [
        "Second PPTX",
        "Minimal PPTX",
    ]


def test_pptx_duplicate_slide_and_shape_ids_fail_closed() -> None:
    source = _PPTX.read_bytes()
    parts = _parts(source)
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b'<p:sldId id="256" r:id="rId2"/>',
        b'<p:sldId id="256" r:id="rId2"/><p:sldId id="256" r:id="rId2"/>',
    )
    duplicate_slide = admit_ooxml_package(_package(parts), DEFAULT_CONFIG.native)
    with pytest.raises(NativeParseFailure) as observed_slide:
        parse_pptx(duplicate_slide, DEFAULT_CONFIG.native)
    assert observed_slide.value.code is StableErrorCode.INPUT_INVALID

    parts = _parts(source)
    slide = parts["ppt/slides/slide1.xml"]
    shape_start = slide.index(b"<p:sp>")
    shape_end = slide.index(b"</p:sp>") + len(b"</p:sp>")
    parts["ppt/slides/slide1.xml"] = slide.replace(
        b"</p:spTree>",
        slide[shape_start:shape_end] + b"</p:spTree>",
    )
    duplicate_shape = admit_ooxml_package(_package(parts), DEFAULT_CONFIG.native)
    with pytest.raises(NativeParseFailure) as observed_shape:
        parse_pptx(duplicate_shape, DEFAULT_CONFIG.native)
    assert observed_shape.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    ("old", "new", "code"),
    [
        (
            b"<a:xfrm>",
            b'<a:xfrm rot="60000">',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<a:off x="914400" y="914400"/>',
            b'<a:off x="12192000" y="914400"/>',
            StableErrorCode.QUALITY_LOCATOR_FAILED,
        ),
    ],
)
def test_pptx_unsupported_rotation_and_out_of_bounds_geometry_fail_closed(
    old: bytes,
    new: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/slides/slide1.xml"] = parts["ppt/slides/slide1.xml"].replace(
        old,
        new,
        1,
    )
    package = admit_ooxml_package(_package(parts), DEFAULT_CONFIG.native)

    with pytest.raises(NativeParseFailure) as observed:
        parse_pptx(package, DEFAULT_CONFIG.native)

    assert observed.value.code is code


def test_pptx_basic_table_is_row_major_and_keeps_cell_text() -> None:
    parts = _parts(_PPTX.read_bytes())
    slide = parts["ppt/slides/slide1.xml"]
    shape_start = slide.index(b"<p:sp>")
    shape_end = slide.index(b"</p:sp>") + len(b"</p:sp>")
    table = (
        b'<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Table"/>'
        b"<p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm>"
        b'<a:off x="914400" y="914400"/><a:ext cx="4000000" cy="2000000"/>'
        b'</p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.'
        b'org/drawingml/2006/table"><a:tbl><a:tblPr/><a:tblGrid>'
        b'<a:gridCol w="2000000"/><a:gridCol w="2000000"/></a:tblGrid>'
        b'<a:tr h="1000000"><a:tc><a:txBody><a:bodyPr/><a:lstStyle/>'
        b"<a:p><a:r><a:t>A</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>"
        b"<a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>B</a:t>"
        b"</a:r></a:p></a:txBody><a:tcPr/></a:tc></a:tr></a:tbl>"
        b"</a:graphicData></a:graphic></p:graphicFrame>"
    )
    parts["ppt/slides/slide1.xml"] = slide[:shape_start] + table + slide[shape_end:]

    artifact = _parse(_package(parts))

    assert [node.kind for node in artifact.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.SLIDE,
        NativeNodeKind.SHAPE,
        NativeNodeKind.TABLE,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.PARAGRAPH,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.PARAGRAPH,
    ]
    assert [
        node.fragments[0].text
        for node in artifact.nodes
        if node.kind is NativeNodeKind.PARAGRAPH
    ] == ["A", "B"]


def test_pptx_notes_are_retained_non_indexable_under_document_root() -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace(
        b"</Types>",
        b'<Override PartName="/ppt/notesSlides/notesSlide1.xml" ContentType="'
        b"application/vnd.openxmlformats-officedocument.presentationml.notesSlide+"
        b'xml"/></Types>',
    )
    parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
        "ppt/slides/_rels/slide1.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
        b"officeDocument/2006/relationships/notesSlide"
        b'" Target="../notesSlides/notesSlide1.xml"/></Relationships>',
    )
    parts["ppt/notesSlides/notesSlide1.xml"] = (
        f'<?xml version="1.0"?><p:notes xmlns:p="{_P}" xmlns:a="{_A}" '
        f'xmlns:r="{_R}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" '
        'name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>'
        '<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Body"/><p:cNvSpPr/>'
        '<p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody>'
        "<a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Speaker note</a:t></a:r>"
        "</a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>"
    ).encode()

    artifact = _parse(_package(parts))

    note = next(node for node in artifact.nodes if node.kind is NativeNodeKind.FOOTNOTE)
    assert note.parent_ordinal == 0
    assert _attributes(note) == {
        "nonIndexable": True,
        "noteKind": "slideNotes",
        "slideIndex": 0,
    }
    note_text = next(
        node
        for node in artifact.nodes[note.ordinal + 1 :]
        if node.parent_ordinal == note.ordinal
    )
    assert note_text.fragments[0].text == "Speaker note"


def test_pptx_shape_limit_and_output_determinism() -> None:
    source = _PPTX.read_bytes()
    limits = replace(DEFAULT_CONFIG.native, shapes=0)
    package = admit_ooxml_package(source, limits)
    with pytest.raises(NativeParseFailure) as limited:
        parse_pptx(package, limits)
    assert limited.value.code is StableErrorCode.RESULT_TOO_LARGE

    assert _parse(source).canonical_bytes == _parse(source).canonical_bytes


@pytest.mark.parametrize(
    ("part_name", "old", "new", "code"),
    [
        (
            "ppt/presentation.xml",
            b"p:presentation",
            b"p:notPresentation",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/presentation.xml",
            b'<p:sldId id="256" r:id="rId2"/>',
            b"",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/_rels/presentation.xml.rels",
            b'relationships/slide" Target="slides/slide1.xml',
            b'relationships/slideLayout" Target="slides/slide1.xml',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/slides/slide1.xml",
            b"p:sld",
            b"p:notSlide",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/slides/slide1.xml",
            b"</p:spTree>",
            b"<p:unknown/></p:spTree>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            "ppt/slides/slide1.xml",
            b"<p:cSld>",
            b"<p:control/><p:cSld>",
            StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED,
        ),
    ],
)
def test_pptx_rejects_invalid_roots_relationships_and_slide_content(
    part_name: str,
    old: bytes,
    new: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    assert old in parts[part_name]
    parts[part_name] = parts[part_name].replace(old, new)

    _assert_parse_failure(_package(parts), code)


def test_pptx_slide_limit_is_enforced_before_shape_parsing() -> None:
    limits = replace(DEFAULT_CONFIG.native, slides=0)

    _assert_parse_failure(
        _PPTX.read_bytes(),
        StableErrorCode.RESULT_TOO_LARGE,
        limits=limits,
    )


def test_pptx_group_transform_scales_child_geometry_and_preserves_flips() -> None:
    parts = _parts(_PPTX.read_bytes())
    group = (
        b'<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/>'
        b"<p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm>"
        b'<a:off x="914400" y="914400"/><a:ext cx="4000000" cy="2000000"/>'
        b'<a:chOff x="0" y="0"/><a:chExt cx="2000000" cy="2000000"/>'
        b'</a:xfrm></p:grpSpPr><p:sp><p:nvSpPr><p:cNvPr id="3" name="Child"/>'
        b"<p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>"
        b'<a:xfrm flipH="true" flipV="on"><a:off x="100000" y="100000"/>'
        b'<a:ext cx="200000" cy="200000"/></a:xfrm></p:spPr>'
        b"</p:sp></p:grpSp>"
    )
    _replace_shape(parts, group)

    artifact = _parse(_package(parts))

    shapes = [node for node in artifact.nodes if node.kind is NativeNodeKind.SHAPE]
    assert len(shapes) == 2
    assert shapes[1].parent_ordinal == shapes[0].ordinal
    child = _attributes(shapes[1])
    assert child["name"] == "Child"
    assert child["flipHorizontal"] is True
    assert child["flipVertical"] is True
    assert [
        child["bboxX1MilliPoint"],
        child["bboxY1MilliPoint"],
        child["bboxX2MilliPoint"],
        child["bboxY2MilliPoint"],
    ] == [87748, 79874, 119244, 95622]


def test_pptx_shape_without_transform_has_no_synthetic_geometry() -> None:
    parts = _parts(_PPTX.read_bytes())
    slide = parts["ppt/slides/slide1.xml"]
    start = slide.index(b"<p:spPr>")
    end = slide.index(b"</p:spPr>") + len(b"</p:spPr>")
    parts["ppt/slides/slide1.xml"] = slide[:start] + slide[end:]

    artifact = _parse(_package(parts))

    shape = next(node for node in artifact.nodes if node.kind is NativeNodeKind.SHAPE)
    assert "bboxX1MilliPoint" not in _attributes(shape)


def test_pptx_shape_properties_without_transform_have_no_geometry() -> None:
    parts = _parts(_PPTX.read_bytes())
    slide = parts["ppt/slides/slide1.xml"]
    transform_start = slide.index(b"<a:xfrm>")
    transform_end = slide.index(b"</a:xfrm>") + len(b"</a:xfrm>")
    parts["ppt/slides/slide1.xml"] = slide[:transform_start] + slide[transform_end:]

    artifact = _parse(_package(parts))

    shape = next(node for node in artifact.nodes if node.kind is NativeNodeKind.SHAPE)
    assert "bboxX1MilliPoint" not in _attributes(shape)


@pytest.mark.parametrize(
    ("group_body", "code"),
    [
        (b"", StableErrorCode.INPUT_INVALID),
        (
            b"<p:grpSpPr><a:xfrm>"
            b'<a:off x="0" y="0"/><a:ext cx="1000000" cy="1000000"/>'
            b'<a:chOff x="0" y="0"/><a:chExt cx="1000000" cy="1000000"/>'
            b"</a:xfrm></p:grpSpPr><p:unknown/>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
    ],
)
def test_pptx_groups_require_transform_and_supported_children(
    group_body: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    group = (
        b'<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/>'
        b"<p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>" + group_body + b"</p:grpSp>"
    )
    _replace_shape(parts, group)

    _assert_parse_failure(_package(parts), code)


def test_pptx_group_fractional_geometry_uses_bankers_rounding() -> None:
    parts = _parts(_PPTX.read_bytes())
    group = (
        b'<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/>'
        b"<p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm>"
        b'<a:off x="0" y="0"/><a:ext cx="127" cy="1270"/>'
        b'<a:chOff x="0" y="0"/><a:chExt cx="20" cy="1270"/>'
        b'</a:xfrm></p:grpSpPr><p:sp><p:nvSpPr><p:cNvPr id="3" name="Child"/>'
        b"<p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm>"
        b'<a:off x="1" y="0"/><a:ext cx="20" cy="1270"/>'
        b"</a:xfrm></p:spPr></p:sp></p:grpSp>"
    )
    _replace_shape(parts, group)

    artifact = _parse(_package(parts))

    child = next(
        node
        for node in artifact.nodes
        if node.kind is NativeNodeKind.SHAPE and _attributes(node)["name"] == "Child"
    )
    assert _attributes(child)["bboxX1MilliPoint"] == 0
    assert _attributes(child)["bboxX2MilliPoint"] == 10


@pytest.mark.parametrize(
    ("old", "new", "code"),
    [
        (
            b"<p:grpSpPr><a:xfrm>",
            b'<p:grpSpPr><a:xfrm flipH="true">',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b'<a:ext cx="10000000" cy="1000000"/>',
            b'<a:ext cx="1" cy="1000000"/>',
            StableErrorCode.QUALITY_LOCATOR_FAILED,
        ),
    ],
)
def test_pptx_rejects_group_flip_and_geometry_lost_during_rounding(
    old: bytes,
    new: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    if old.startswith(b"<p:grpSpPr"):
        group = (
            b'<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/>'
            b"<p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm>"
            b'<a:off x="0" y="0"/><a:ext cx="1000000" cy="1000000"/>'
            b'<a:chOff x="0" y="0"/><a:chExt cx="1000000" cy="1000000"/>'
            b"</a:xfrm></p:grpSpPr></p:grpSp>"
        )
        _replace_shape(parts, group)
    assert old in parts["ppt/slides/slide1.xml"]
    parts["ppt/slides/slide1.xml"] = parts["ppt/slides/slide1.xml"].replace(
        old,
        new,
        1,
    )

    _assert_parse_failure(_package(parts), code)


def test_pptx_fields_empty_runs_and_line_breaks_keep_semantic_order() -> None:
    parts = _parts(_PPTX.read_bytes())
    slide = parts["ppt/slides/slide1.xml"]
    paragraph_start = slide.index(b"<a:p>")
    paragraph_end = slide.index(b"</a:p>") + len(b"</a:p>")
    paragraph = (
        b'<a:p><a:pPr/><a:fld><a:t xml:space="preserve">Field value</a:t>'
        b"</a:fld><a:br/><a:r><a:rPr/></a:r><a:r><a:t></a:t></a:r>"
        b"<a:endParaRPr/></a:p>"
    )
    parts["ppt/slides/slide1.xml"] = (
        slide[:paragraph_start] + paragraph + slide[paragraph_end:]
    )

    artifact = _parse(_package(parts))

    paragraph_node = next(
        node for node in artifact.nodes if node.kind is NativeNodeKind.PARAGRAPH
    )
    assert [fragment.text for fragment in paragraph_node.fragments] == ["Field value"]
    line_break = next(
        node for node in artifact.nodes if node.kind is NativeNodeKind.LINE_BREAK
    )
    assert line_break.parent_ordinal == paragraph_node.ordinal
    assert _attributes(line_break) == {"breakKind": "line"}


@pytest.mark.parametrize(
    ("old", "new", "code"),
    [
        (
            b"<a:bodyPr/>",
            b"<a:bodyPr/><a:unknown/>",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<a:t>Minimal PPTX</a:t>",
            b"<a:t><a:r/></a:t>",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b"<a:t>Minimal PPTX</a:t>",
            b'<a:t lang="en">Minimal PPTX</a:t>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            b'<a:rPr lang="en-US"/>',
            b'<a:rPr lang="en-US"/><a:unknown/>',
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (
            b"<a:endParaRPr",
            b"<a:unknown/><a:endParaRPr",
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
    ],
)
def test_pptx_rejects_unsupported_text_body_and_run_grammar(
    old: bytes,
    new: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    assert old in parts["ppt/slides/slide1.xml"]
    parts["ppt/slides/slide1.xml"] = parts["ppt/slides/slide1.xml"].replace(
        old,
        new,
        1,
    )

    _assert_parse_failure(_package(parts), code)


@pytest.mark.parametrize(
    ("table_children", "limits"),
    [
        (
            b"<a:tblPr/><a:tblGrid/><a:unknown/>",
            DEFAULT_CONFIG.native,
        ),
        (
            b"<a:tblPr/><a:tblGrid/><a:tr><a:unknown/></a:tr>",
            DEFAULT_CONFIG.native,
        ),
        (
            b"<a:tblPr/><a:tblGrid/><a:tr><a:tc><a:unknown/></a:tc></a:tr>",
            DEFAULT_CONFIG.native,
        ),
        (
            b"<a:tblPr/><a:tblGrid/><a:tr><a:tc><a:tcPr/></a:tc></a:tr>",
            replace(DEFAULT_CONFIG.native, cells=0),
        ),
    ],
)
def test_pptx_table_grammar_and_cell_limit_fail_closed(
    table_children: bytes,
    limits: NativeParserLimits,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    _replace_shape(parts, _graphic_frame(table_children))

    _assert_parse_failure(
        _package(parts),
        StableErrorCode.RESULT_TOO_LARGE
        if limits.cells == 0
        else StableErrorCode.FORMAT_UNSUPPORTED,
        limits=limits,
    )


def test_pptx_table_spans_advance_columns_without_filling_phantom_cells() -> None:
    parts = _parts(_PPTX.read_bytes())
    table = (
        b"<a:tblPr/><a:tblGrid/><a:tr>"
        b'<a:tc gridSpan="2" rowSpan="3"><a:tcPr/></a:tc>'
        b"<a:tc><a:tcPr/></a:tc></a:tr>"
    )
    _replace_shape(parts, _graphic_frame(table))

    artifact = _parse(_package(parts))

    cells = [node for node in artifact.nodes if node.kind is NativeNodeKind.TABLE_CELL]
    assert [_attributes(node) for node in cells] == [
        {"columnIndex": 0, "columnSpan": 2, "rowIndex": 0, "rowSpan": 3},
        {"columnIndex": 2, "columnSpan": 1, "rowIndex": 0, "rowSpan": 1},
    ]


def test_pptx_internal_picture_becomes_non_indexable_asset_reference() -> None:
    parts = _parts(_PPTX.read_bytes())
    _replace_shape(parts, _picture())
    parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace(
        b"</Types>",
        b'<Default Extension="png" ContentType="image/png"/></Types>',
    )
    parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
        "ppt/slides/_rels/slide1.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/image" Target="../media/image1.png"/>'
        b"</Relationships>",
    )
    parts["ppt/media/image1.png"] = b"\x89PNG\r\n\x1a\n"

    artifact = _parse(_package(parts))

    asset = next(
        node for node in artifact.nodes if node.kind is NativeNodeKind.ASSET_REF
    )
    assert asset.parent_ordinal == next(
        node.ordinal for node in artifact.nodes if node.kind is NativeNodeKind.SHAPE
    )
    assert _attributes(asset)["contentType"] == "image/png"
    assert _attributes(asset)["relationshipId"] == "rId2"
    assert _attributes(asset)["nonIndexable"] is True


@pytest.mark.parametrize(
    ("embed", "relationship", "code"),
    [
        (b"", None, StableErrorCode.FORMAT_UNSUPPORTED),
        (
            b'r:embed="rId2"',
            b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
            b'officeDocument/2006/relationships/image" Target="https://example.test/'
            b'image.png" TargetMode="External"/>',
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_pptx_picture_requires_an_internal_embedded_relationship(
    embed: bytes,
    relationship: bytes | None,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    _replace_shape(parts, _picture(embed=embed))
    if relationship is not None:
        parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
            "ppt/slides/_rels/slide1.xml.rels"
        ].replace(
            b"</Relationships>",
            relationship + b"</Relationships>",
        )

    _assert_parse_failure(_package(parts), code)


@pytest.mark.parametrize("uri", [b"urn:example:custom", b"urn:example:chart"])
def test_pptx_graphic_frames_reject_non_table_payloads(uri: bytes) -> None:
    parts = _parts(_PPTX.read_bytes())
    frame = (
        b'<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Graphic"/>'
        b"<p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>"
        b'<a:graphic><a:graphicData uri="' + uri + b'"/></a:graphic></p:graphicFrame>'
    )
    _replace_shape(parts, frame)

    _assert_parse_failure(_package(parts), StableErrorCode.FORMAT_UNSUPPORTED)


def test_pptx_multiple_notes_relationships_are_ambiguous() -> None:
    parts = _parts(_PPTX.read_bytes())
    notes = (
        f'<?xml version="1.0"?><p:notes xmlns:p="{_P}" xmlns:a="{_A}" '
        f'xmlns:r="{_R}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" '
        'name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>'
        "</p:spTree></p:cSld></p:notes>"
    ).encode()
    _add_internal_notes(parts, notes)
    parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace(
        b"</Types>",
        b'<Override PartName="/ppt/notesSlides/notesSlide2.xml" ContentType="'
        b"application/vnd.openxmlformats-officedocument.presentationml.notesSlide+"
        b'xml"/></Types>',
    )
    parts["ppt/notesSlides/notesSlide2.xml"] = notes
    parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
        "ppt/slides/_rels/slide1.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId3" Type="http://schemas.'
        b'openxmlformats.org/officeDocument/2006/relationships/notesSlide" '
        b'Target="../notesSlides/notesSlide2.xml"/></Relationships>',
    )

    _assert_parse_failure(_package(parts), StableErrorCode.INPUT_INVALID)


def test_pptx_external_notes_relationship_is_not_dereferenced() -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/slides/_rels/slide1.xml.rels"] = parts[
        "ppt/slides/_rels/slide1.xml.rels"
    ].replace(
        b"</Relationships>",
        b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/notesSlide" Target="https://notes.test" '
        b'TargetMode="External"/></Relationships>',
    )

    _assert_parse_failure(_package(parts), StableErrorCode.INPUT_INVALID)


def test_pptx_notes_root_and_shape_grammar_fail_closed() -> None:
    parts = _parts(_PPTX.read_bytes())
    wrong_root = (
        f'<?xml version="1.0"?><p:notNotes xmlns:p="{_P}" xmlns:a="{_A}" '
        f'xmlns:r="{_R}"/>'
    ).encode()
    _add_internal_notes(parts, wrong_root)
    _assert_parse_failure(_package(parts), StableErrorCode.INPUT_INVALID)

    parts = _parts(_PPTX.read_bytes())
    unsupported_shape = (
        f'<?xml version="1.0"?><p:notes xmlns:p="{_P}" xmlns:a="{_A}" '
        f'xmlns:r="{_R}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" '
        'name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>'
        "<p:pic/></p:spTree></p:cSld></p:notes>"
    ).encode()
    _add_internal_notes(parts, unsupported_shape)
    _assert_parse_failure(_package(parts), StableErrorCode.FORMAT_UNSUPPORTED)


def test_pptx_notes_skip_non_body_placeholders_and_empty_shapes() -> None:
    parts = _parts(_PPTX.read_bytes())
    notes = (
        f'<?xml version="1.0"?><p:notes xmlns:p="{_P}" xmlns:a="{_A}" '
        f'xmlns:r="{_R}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" '
        'name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>'
        '<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/>'
        '<p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr></p:sp>'
        '<p:sp><p:nvSpPr><p:cNvPr id="3" name="Empty body"/><p:cNvSpPr/>'
        "<p:nvPr/></p:nvSpPr></p:sp></p:spTree></p:cSld></p:notes>"
    ).encode()
    _add_internal_notes(parts, notes)

    artifact = _parse(_package(parts))

    footnote = next(
        node for node in artifact.nodes if node.kind is NativeNodeKind.FOOTNOTE
    )
    assert all(node.parent_ordinal != footnote.ordinal for node in artifact.nodes)


@pytest.mark.parametrize(
    ("part_name", "old", "new", "code"),
    [
        (
            "ppt/presentation.xml",
            b'<p:sldId id="256"',
            b'<p:sldId id="0"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/presentation.xml",
            b'cx="12192000"',
            b'cx="not-an-integer"',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/presentation.xml",
            b"<p:notesSz",
            b'<p:sldSz cx="1" cy="1"/><p:notesSz',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/slides/slide1.xml",
            b'<p:cNvPr id="2" name="Title"/>',
            b'<p:cNvPr id="2"/>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/slides/slide1.xml",
            b'<p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/>',
            b'<p:cNvPr id="2" name="Title"/><p:cNvSpPr/>'
            b'<p:nvPr><p:ph idx="-1"/></p:nvPr>',
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "ppt/slides/slide1.xml",
            b"<a:xfrm>",
            b'<a:xfrm flipH="maybe">',
            StableErrorCode.INPUT_INVALID,
        ),
    ],
)
def test_pptx_rejects_invalid_required_numbers_names_and_booleans(
    part_name: str,
    old: bytes,
    new: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    assert old in parts[part_name]
    parts[part_name] = parts[part_name].replace(old, new, 1)

    _assert_parse_failure(_package(parts), code)


@pytest.mark.parametrize(
    ("show", "hidden"),
    [(b"0", True), (b"false", True), (b"off", True), (b"true", False)],
)
def test_pptx_slide_visibility_uses_strict_ooxml_booleans(
    show: bytes,
    hidden: bool,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b'<p:sldId id="256"',
        b'<p:sldId show="' + show + b'" id="256"',
    )

    artifact = _parse(_package(parts))

    slide = next(node for node in artifact.nodes if node.kind is NativeNodeKind.SLIDE)
    assert _attributes(slide)["hidden"] is hidden


def test_pptx_rejects_unknown_slide_visibility_value() -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b'<p:sldId id="256"',
        b'<p:sldId show="sometimes" id="256"',
    )

    _assert_parse_failure(_package(parts), StableErrorCode.INPUT_INVALID)


def test_pptx_placeholder_index_is_preserved_as_non_negative_integer() -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/slides/slide1.xml"] = parts["ppt/slides/slide1.xml"].replace(
        b'<p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/>',
        b'<p:cNvPr id="2" name="Title"/><p:cNvSpPr/>'
        b'<p:nvPr><p:ph type="title" idx="7"/></p:nvPr>',
    )

    artifact = _parse(_package(parts))

    shape = next(node for node in artifact.nodes if node.kind is NativeNodeKind.SHAPE)
    assert _attributes(shape)["placeholderType"] == "title"
    assert _attributes(shape)["placeholderIndex"] == 7


@pytest.mark.parametrize(
    "limits",
    [
        replace(DEFAULT_CONFIG.native, nodes=1),
        replace(DEFAULT_CONFIG.native, fragments=0),
        replace(DEFAULT_CONFIG.native, attributes=0),
    ],
)
def test_pptx_global_output_limits_cover_nodes_fragments_and_attributes(
    limits: NativeParserLimits,
) -> None:
    _assert_parse_failure(
        _PPTX.read_bytes(),
        StableErrorCode.RESULT_TOO_LARGE,
        limits=limits,
    )


def test_pptx_parser_rejects_invalid_call_types_and_other_ooxml_formats() -> None:
    with pytest.raises(TypeError, match="validated OPC package"):
        parse_pptx(cast("Any", object()), DEFAULT_CONFIG.native)

    package = admit_ooxml_package(_PPTX.read_bytes(), DEFAULT_CONFIG.native)
    with pytest.raises(TypeError, match="limits have an invalid type"):
        parse_pptx(package, cast("Any", object()))

    docx_package = admit_ooxml_package(_DOCX.read_bytes(), DEFAULT_CONFIG.native)
    with pytest.raises(NativeParseFailure) as mismatch:
        parse_pptx(docx_package, DEFAULT_CONFIG.native)
    assert mismatch.value.code is StableErrorCode.FORMAT_MISMATCH


@pytest.mark.parametrize(
    ("element", "code"),
    [
        (b"<p:control/>", StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED),
        (b"<p:unknown/>", StableErrorCode.FORMAT_UNSUPPORTED),
    ],
)
def test_pptx_presentation_root_is_closed_and_rejects_active_content(
    element: bytes,
    code: StableErrorCode,
) -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b"</p:presentation>",
        element + b"</p:presentation>",
    )

    _assert_parse_failure(_package(parts), code)


def test_pptx_oversized_decimal_maps_to_input_invalid() -> None:
    parts = _parts(_PPTX.read_bytes())
    parts["ppt/presentation.xml"] = parts["ppt/presentation.xml"].replace(
        b'id="256"',
        b'id="' + b"9" * 5000 + b'"',
    )

    _assert_parse_failure(_package(parts), StableErrorCode.INPUT_INVALID)
