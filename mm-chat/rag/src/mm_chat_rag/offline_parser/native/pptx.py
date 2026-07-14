"""Deterministic PPTX slide/shape parser over one admitted OPC package."""

from __future__ import annotations

import re
from dataclasses import dataclass
from fractions import Fraction
from typing import Final, Never

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeAttribute,
    NativeBytePosition,
    NativeDocument,
    NativeFragment,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeSourcePosition,
    attributes,
)
from mm_chat_rag.offline_parser.native.opc import ValidatedOpcPackage
from mm_chat_rag.offline_parser.native.xml_source import (
    XmlElement,
    XmlText,
    expanded_name,
)

_P: Final = "http://schemas.openxmlformats.org/presentationml/2006/main"
_A: Final = "http://schemas.openxmlformats.org/drawingml/2006/main"
_R: Final = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
_XML: Final = "http://www.w3.org/XML/1998/namespace"
_PRESENTATION_URI: Final = "/ppt/presentation.xml"
_TABLE_URI: Final = "http://schemas.openxmlformats.org/drawingml/2006/table"
_REL_BASE: Final = (
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)
_SHAPE_NAMES: Final = frozenset(
    {
        expanded_name(_P, "cxnSp"),
        expanded_name(_P, "graphicFrame"),
        expanded_name(_P, "grpSp"),
        expanded_name(_P, "pic"),
        expanded_name(_P, "sp"),
    }
)
_ACTIVE_NAMES: Final = frozenset(
    {
        expanded_name(_P, "control"),
        expanded_name(_P, "oleObj"),
        expanded_name(_P, "snd"),
        expanded_name(_P, "video"),
    }
)
_PRESENTATION_CHILD_NAMES: Final = frozenset(
    {
        expanded_name(_P, "notesSz"),
        expanded_name(_P, "sldIdLst"),
        expanded_name(_P, "sldMasterIdLst"),
        expanded_name(_P, "sldSz"),
    }
)
_MAX_INTEGER_DIGITS: Final = 19
_GRAPHIC_UNSUPPORTED_TOKENS: Final = (
    "chart",
    "diagram",
    "smartart",
)


@dataclass(frozen=True, slots=True)
class _AxisTransform:
    scale: Fraction = Fraction(1)
    offset: Fraction = Fraction(0)

    def apply(self, value: int | Fraction) -> Fraction:
        return self.offset + self.scale * value


@dataclass(frozen=True, slots=True)
class _Transform:
    x: _AxisTransform = _AxisTransform()
    y: _AxisTransform = _AxisTransform()


@dataclass(frozen=True, slots=True)
class _Geometry:
    x1: int
    y1: int
    x2: int
    y2: int
    flip_horizontal: bool
    flip_vertical: bool


class _PptxBuilder:
    def __init__(
        self,
        package: ValidatedOpcPackage,
        limits: NativeParserLimits,
    ) -> None:
        self.package = package
        self.limits = limits
        self.nodes: list[NativeNode] = [
            NativeNode(
                ordinal=0,
                kind=NativeNodeKind.DOCUMENT,
                parent_ordinal=None,
                source_position=NativeBytePosition(0, 0, package.source_bytes),
            )
        ]
        self.fragment_count = 0
        self.attribute_count = 0
        self.shape_count = 0
        self.cell_count = 0
        self.slide_width_emu = 0
        self.slide_height_emu = 0
        self.slide_width_milli_point = 0
        self.slide_height_milli_point = 0

    def build(self) -> NativeDocument:
        presentation = self.package.parse_xml_part(_PRESENTATION_URI).root
        if presentation.name != _p("presentation"):
            _invalid()
        _reject_active(presentation)
        if any(
            child.name not in _PRESENTATION_CHILD_NAMES
            for child in presentation.child_elements()
        ):
            _unsupported()
        self._read_slide_size(presentation)
        slide_list = _one_child(presentation, _p("sldIdLst"), required=True)
        if slide_list is None:
            _invalid()
        slide_entries = slide_list.child_elements()
        if (
            not slide_entries
            or len(slide_entries) > self.limits.slides
            or any(item.name != _p("sldId") for item in slide_entries)
        ):
            if len(slide_entries) > self.limits.slides:
                _result_limit()
            _invalid()
        slide_ids: set[int] = set()
        slide_parts: set[str] = set()
        for slide_index, slide_reference in enumerate(slide_entries):
            slide_id = _required_positive_int(slide_reference.attribute("id"))
            relationship_id = slide_reference.attribute(expanded_name(_R, "id"))
            if relationship_id is None or slide_id in slide_ids:
                _invalid()
            relationship = self.package.resolve_relationship(
                _PRESENTATION_URI,
                relationship_id,
            )
            if (
                relationship.is_external
                or relationship.target_part_uri is None
                or relationship.relationship_type != _REL_BASE + "slide"
                or relationship.target_part_uri in slide_parts
            ):
                _invalid()
            slide_ids.add(slide_id)
            slide_parts.add(relationship.target_part_uri)
            self._slide(
                relationship.target_part_uri,
                slide_index=slide_index,
                slide_id=slide_id,
                hidden=_hidden(slide_reference.attribute("show")),
            )
        return NativeDocument(
            source_format=ParserFormat.PPTX,
            source_bytes=self.package.source_bytes,
            source_sha256=self.package.source_sha256,
            source_units=self.package.source_units,
            nodes=tuple(self.nodes),
        )

    def _read_slide_size(self, presentation: XmlElement) -> None:
        size = _one_child(presentation, _p("sldSz"), required=True)
        if size is None:
            _invalid()
        self.slide_width_emu = _required_positive_int(size.attribute("cx"))
        self.slide_height_emu = _required_positive_int(size.attribute("cy"))
        self.slide_width_milli_point = _emu_to_milli_point(self.slide_width_emu)
        self.slide_height_milli_point = _emu_to_milli_point(self.slide_height_emu)

    def _slide(
        self,
        slide_uri: str,
        *,
        slide_index: int,
        slide_id: int,
        hidden: bool,
    ) -> None:
        slide = self.package.parse_xml_part(slide_uri).root
        if slide.name != _p("sld"):
            _invalid()
        _reject_active(slide)
        common = _one_child(slide, _p("cSld"), required=True)
        if common is None:
            _invalid()
        shape_tree = _one_child(common, _p("spTree"), required=True)
        if shape_tree is None:
            _invalid()
        slide_ordinal = self._append(
            NativeNodeKind.SLIDE,
            0,
            slide.source_position,
            node_attributes=attributes(
                heightMilliPoint=self.slide_height_milli_point,
                hidden=hidden,
                slideId=slide_id,
                slideIndex=slide_index,
                widthMilliPoint=self.slide_width_milli_point,
            ),
        )
        shape_ids: set[int] = set()
        shape_counter = [0]
        for element in shape_tree.child_elements():
            if element.name in {_p("grpSpPr"), _p("nvGrpSpPr")}:
                continue
            if element.name not in _SHAPE_NAMES:
                _unsupported()
            self._shape(
                element,
                parent_ordinal=slide_ordinal,
                slide_index=slide_index,
                slide_uri=slide_uri,
                parent_transform=_Transform(),
                shape_ids=shape_ids,
                shape_counter=shape_counter,
                inherited_non_indexable=hidden,
            )
        self._notes(slide_uri, slide_index=slide_index)

    def _shape(
        self,
        element: XmlElement,
        *,
        parent_ordinal: int,
        slide_index: int,
        slide_uri: str,
        parent_transform: _Transform,
        shape_ids: set[int],
        shape_counter: list[int],
        inherited_non_indexable: bool,
    ) -> None:
        shape_id, shape_name, placeholder_type, placeholder_index = _shape_identity(
            element
        )
        if shape_id in shape_ids:
            _invalid()
        shape_ids.add(shape_id)
        shape_ordinal = shape_counter[0]
        shape_counter[0] += 1
        self.shape_count += 1
        if self.shape_count > self.limits.shapes:
            _result_limit()

        geometry, child_transform = self._geometry(element, parent_transform)
        is_group = element.name == _p("grpSp")
        shape_kind = element.name.rsplit("}", 1)[-1]
        values: dict[str, bool | int | str | None] = {
            "group": is_group,
            "name": shape_name,
            "nonIndexable": inherited_non_indexable,
            "placeholderIndex": placeholder_index,
            "placeholderType": placeholder_type,
            "shapeId": shape_id,
            "shapeKind": shape_kind,
            "shapeOrdinal": shape_ordinal,
            "slideIndex": slide_index,
        }
        if geometry is not None:
            values.update(
                {
                    "bboxX1MilliPoint": geometry.x1,
                    "bboxX2MilliPoint": geometry.x2,
                    "bboxY1MilliPoint": geometry.y1,
                    "bboxY2MilliPoint": geometry.y2,
                    "flipHorizontal": geometry.flip_horizontal,
                    "flipVertical": geometry.flip_vertical,
                }
            )
        shape_node = self._append(
            NativeNodeKind.SHAPE,
            parent_ordinal,
            element.source_position,
            node_attributes=attributes(**values),
        )

        if is_group:
            if child_transform is None:
                _invalid()
            for child in element.child_elements():
                if child.name in {_p("grpSpPr"), _p("nvGrpSpPr")}:
                    continue
                if child.name not in _SHAPE_NAMES:
                    _unsupported()
                self._shape(
                    child,
                    parent_ordinal=shape_node,
                    slide_index=slide_index,
                    slide_uri=slide_uri,
                    parent_transform=child_transform,
                    shape_ids=shape_ids,
                    shape_counter=shape_counter,
                    inherited_non_indexable=inherited_non_indexable,
                )
            return

        text_body = _one_child(element, _p("txBody"), required=False)
        if text_body is not None:
            self._text_body(text_body, parent_ordinal=shape_node)
        if element.name == _p("graphicFrame"):
            self._graphic_frame(element, parent_ordinal=shape_node)
        elif element.name == _p("pic"):
            self._picture(
                element,
                parent_ordinal=shape_node,
                slide_uri=slide_uri,
            )

    def _geometry(
        self,
        element: XmlElement,
        parent: _Transform,
    ) -> tuple[_Geometry | None, _Transform | None]:
        properties_name: str
        transform_name: str
        if element.name == _p("grpSp"):
            properties_name = _p("grpSpPr")
            transform_name = _a("xfrm")
        elif element.name == _p("graphicFrame"):
            properties_name = _p("xfrm")
            transform_name = _p("xfrm")
        else:
            properties_name = _p("spPr")
            transform_name = _a("xfrm")
        properties = _one_child(element, properties_name, required=False)
        if properties is None:
            return None, None
        transform = (
            properties
            if properties.name == transform_name
            else _one_child(properties, transform_name, required=False)
        )
        if transform is None:
            return None, None
        rotation = _required_int(transform.attribute("rot") or "0")
        if rotation != 0:
            _unsupported()
        flip_horizontal = _boolean(transform.attribute("flipH"))
        flip_vertical = _boolean(transform.attribute("flipV"))
        if element.name == _p("grpSp") and (flip_horizontal or flip_vertical):
            _unsupported()
        offset = _one_child(transform, _a("off"), required=True)
        extent = _one_child(transform, _a("ext"), required=True)
        if offset is None or extent is None:
            _invalid()
        x = _required_int(offset.attribute("x"))
        y = _required_int(offset.attribute("y"))
        cx = _required_positive_int(extent.attribute("cx"))
        cy = _required_positive_int(extent.attribute("cy"))
        left = parent.x.apply(x)
        top = parent.y.apply(y)
        right = parent.x.apply(x + cx)
        bottom = parent.y.apply(y + cy)
        geometry = self._bounded_geometry(
            left,
            top,
            right,
            bottom,
            flip_horizontal=flip_horizontal,
            flip_vertical=flip_vertical,
        )
        if element.name != _p("grpSp"):
            return geometry, None
        child_offset = _one_child(transform, _a("chOff"), required=True)
        child_extent = _one_child(transform, _a("chExt"), required=True)
        if child_offset is None or child_extent is None:
            _invalid()
        child_x = _required_int(child_offset.attribute("x"))
        child_y = _required_int(child_offset.attribute("y"))
        child_cx = _required_positive_int(child_extent.attribute("cx"))
        child_cy = _required_positive_int(child_extent.attribute("cy"))
        x_scale = Fraction(cx, child_cx)
        y_scale = Fraction(cy, child_cy)
        local_x = _AxisTransform(x_scale, Fraction(x) - x_scale * child_x)
        local_y = _AxisTransform(y_scale, Fraction(y) - y_scale * child_y)
        child_transform = _Transform(
            x=_AxisTransform(
                parent.x.scale * local_x.scale,
                parent.x.apply(local_x.offset),
            ),
            y=_AxisTransform(
                parent.y.scale * local_y.scale,
                parent.y.apply(local_y.offset),
            ),
        )
        return geometry, child_transform

    def _bounded_geometry(
        self,
        left: Fraction,
        top: Fraction,
        right: Fraction,
        bottom: Fraction,
        *,
        flip_horizontal: bool,
        flip_vertical: bool,
    ) -> _Geometry:
        x1, x2 = sorted((left, right))
        y1, y2 = sorted((top, bottom))
        if not (
            0 <= x1 < x2 <= self.slide_width_emu
            and 0 <= y1 < y2 <= self.slide_height_emu
        ):
            _locator_failure()
        result = _Geometry(
            _emu_to_milli_point(x1),
            _emu_to_milli_point(y1),
            _emu_to_milli_point(x2),
            _emu_to_milli_point(y2),
            flip_horizontal,
            flip_vertical,
        )
        if result.x1 >= result.x2 or result.y1 >= result.y2:
            _locator_failure()
        return result

    def _text_body(self, text_body: XmlElement, *, parent_ordinal: int) -> None:
        for child in text_body.child_elements():
            if child.name in {_a("bodyPr"), _a("lstStyle")}:
                continue
            if child.name != _a("p"):
                _unsupported()
            self._text_paragraph(child, parent_ordinal=parent_ordinal)

    def _text_paragraph(self, paragraph: XmlElement, *, parent_ordinal: int) -> None:
        runs: list[XmlText] = []
        breaks: list[XmlElement] = []
        for child in paragraph.child_elements():
            if child.name in {_a("endParaRPr"), _a("pPr")}:
                continue
            if child.name in {_a("r"), _a("fld")}:
                text_node = _one_child(child, _a("t"), required=False)
                if text_node is not None:
                    if text_node.child_elements() or any(
                        item.name != expanded_name(_XML, "space")
                        for item in text_node.attributes
                    ):
                        _invalid()
                    runs.extend(text_node.text_runs())
                if any(
                    item.name not in {_a("pPr"), _a("rPr"), _a("t")}
                    for item in child.child_elements()
                ):
                    _unsupported()
                continue
            if child.name == _a("br"):
                breaks.append(child)
                continue
            _unsupported()
        fragments = tuple(
            NativeFragment(
                ordinal=index,
                role=NativeFragmentRole.TEXT,
                text=run.text,
                transform=run.transform,
                source_position=run.source_position,
            )
            for index, run in enumerate(runs)
            if run.text
        )
        self.fragment_count += len(fragments)
        paragraph_node = self._append(
            NativeNodeKind.PARAGRAPH,
            parent_ordinal,
            paragraph.source_position,
            fragments=fragments,
        )
        for line_break in breaks:
            self._append(
                NativeNodeKind.LINE_BREAK,
                paragraph_node,
                line_break.source_position,
                node_attributes=attributes(breakKind="line"),
            )

    def _graphic_frame(self, frame: XmlElement, *, parent_ordinal: int) -> None:
        graphic = _one_child(frame, _a("graphic"), required=True)
        if graphic is None:
            _invalid()
        data = _one_child(graphic, _a("graphicData"), required=True)
        if data is None:
            _invalid()
        uri = data.attribute("uri")
        if uri != _TABLE_URI:
            if uri is not None and any(
                token in uri.casefold() for token in _GRAPHIC_UNSUPPORTED_TOKENS
            ):
                _unsupported()
            _unsupported()
        table = _one_child(data, _a("tbl"), required=True)
        if table is None:
            _invalid()
        self._table(table, parent_ordinal=parent_ordinal)

    def _table(self, table: XmlElement, *, parent_ordinal: int) -> None:
        rows = [item for item in table.child_elements() if item.name == _a("tr")]
        if any(
            item.name not in {_a("tblGrid"), _a("tblPr"), _a("tr")}
            for item in table.child_elements()
        ):
            _unsupported()
        table_node = self._append(
            NativeNodeKind.TABLE,
            parent_ordinal,
            table.source_position,
            node_attributes=attributes(rowCount=len(rows)),
        )
        for row_index, row in enumerate(rows):
            cells = [item for item in row.child_elements() if item.name == _a("tc")]
            if any(item.name != _a("tc") for item in row.child_elements()):
                _unsupported()
            row_node = self._append(
                NativeNodeKind.TABLE_ROW,
                table_node,
                row.source_position,
                node_attributes=attributes(rowIndex=row_index),
            )
            column_index = 0
            for cell in cells:
                self.cell_count += 1
                if self.cell_count > self.limits.cells:
                    _result_limit()
                column_span = _optional_positive_int(
                    cell.attribute("gridSpan"),
                    default=1,
                )
                cell_node = self._append(
                    NativeNodeKind.TABLE_CELL,
                    row_node,
                    cell.source_position,
                    node_attributes=attributes(
                        columnIndex=column_index,
                        columnSpan=column_span,
                        rowIndex=row_index,
                        rowSpan=_optional_positive_int(
                            cell.attribute("rowSpan"),
                            default=1,
                        ),
                    ),
                )
                column_index += column_span
                for child in cell.child_elements():
                    if child.name == _a("tcPr"):
                        continue
                    if child.name == _a("txBody"):
                        self._text_body(child, parent_ordinal=cell_node)
                    else:
                        _unsupported()

    def _picture(
        self,
        picture: XmlElement,
        *,
        parent_ordinal: int,
        slide_uri: str,
    ) -> None:
        blip_fill = _one_child(picture, _p("blipFill"), required=True)
        if blip_fill is None:
            _invalid()
        blip = _one_child(blip_fill, _a("blip"), required=True)
        if blip is None:
            _invalid()
        relationship_id = blip.attribute(expanded_name(_R, "embed"))
        if relationship_id is None:
            _unsupported()
        relationship = self.package.resolve_relationship(slide_uri, relationship_id)
        if relationship.is_external or relationship.target_part_uri is None:
            _unsupported()
        part = self.package.part(relationship.target_part_uri)
        self._append(
            NativeNodeKind.ASSET_REF,
            parent_ordinal,
            picture.source_position,
            node_attributes=attributes(
                contentType=part.content_type,
                nonIndexable=True,
                relationshipId=relationship_id,
                sourceUnitOrdinal=part.source_unit_ordinal,
            ),
        )

    def _notes(self, slide_uri: str, *, slide_index: int) -> None:
        relationships = [
            item
            for item in self.package.relationships_from(slide_uri)
            if item.relationship_type == _REL_BASE + "notesSlide"
        ]
        if len(relationships) > 1:
            _invalid()
        if not relationships:
            return
        relationship = relationships[0]
        if relationship.is_external or relationship.target_part_uri is None:
            _invalid()
        notes = self.package.parse_xml_part(relationship.target_part_uri).root
        if notes.name != _p("notes"):
            _invalid()
        _reject_active(notes)
        common = _one_child(notes, _p("cSld"), required=True)
        if common is None:
            _invalid()
        shape_tree = _one_child(common, _p("spTree"), required=True)
        if shape_tree is None:
            _invalid()
        note_node = self._append(
            NativeNodeKind.FOOTNOTE,
            0,
            notes.source_position,
            node_attributes=attributes(
                nonIndexable=True,
                noteKind="slideNotes",
                slideIndex=slide_index,
            ),
        )
        for shape in shape_tree.child_elements():
            if shape.name != _p("sp"):
                if shape.name in {_p("grpSpPr"), _p("nvGrpSpPr")}:
                    continue
                _unsupported()
            _shape_id, _name, placeholder_type, _index = _shape_identity(shape)
            if placeholder_type not in {None, "body"}:
                continue
            text_body = _one_child(shape, _p("txBody"), required=False)
            if text_body is not None:
                self._text_body(text_body, parent_ordinal=note_node)

    def _append(
        self,
        kind: NativeNodeKind,
        parent_ordinal: int,
        position: NativeSourcePosition,
        *,
        fragments: tuple[NativeFragment, ...] = (),
        node_attributes: tuple[NativeAttribute, ...] = (),
    ) -> int:
        ordinal = len(self.nodes)
        self.nodes.append(
            NativeNode(
                ordinal=ordinal,
                kind=kind,
                parent_ordinal=parent_ordinal,
                source_position=position,
                fragments=fragments,
                attributes=node_attributes,
            )
        )
        self.attribute_count += len(node_attributes)
        if (
            len(self.nodes) > self.limits.nodes
            or self.fragment_count > self.limits.fragments
            or self.attribute_count > self.limits.attributes
        ):
            _result_limit()
        return ordinal


def parse_pptx(
    package: ValidatedOpcPackage,
    limits: NativeParserLimits,
) -> NativeDocument:
    """Parse one pre-admitted PPTX without reopening or guessing its package."""
    if not isinstance(package, ValidatedOpcPackage):
        raise TypeError("PPTX parser requires a validated OPC package")
    if not isinstance(limits, NativeParserLimits):
        raise TypeError("PPTX parser limits have an invalid type")
    if package.parser_format is not ParserFormat.PPTX:
        raise NativeParseFailure(StableErrorCode.FORMAT_MISMATCH)
    return _PptxBuilder(package, limits).build()


def _shape_identity(
    shape: XmlElement,
) -> tuple[int, str, str | None, int | None]:
    non_visual_name = {
        _p("sp"): _p("nvSpPr"),
        _p("grpSp"): _p("nvGrpSpPr"),
        _p("graphicFrame"): _p("nvGraphicFramePr"),
        _p("pic"): _p("nvPicPr"),
        _p("cxnSp"): _p("nvCxnSpPr"),
    }.get(shape.name)
    if non_visual_name is None:
        _unsupported()
    non_visual = _one_child(shape, non_visual_name, required=True)
    if non_visual is None:
        _invalid()
    properties = _one_child(non_visual, _p("cNvPr"), required=True)
    application = _one_child(non_visual, _p("nvPr"), required=True)
    if properties is None or application is None:
        _invalid()
    shape_id = _required_positive_int(properties.attribute("id"))
    shape_name = properties.attribute("name")
    if shape_name is None:
        _invalid()
    placeholder = _one_child(application, _p("ph"), required=False)
    placeholder_type = placeholder.attribute("type") if placeholder else None
    placeholder_index = (
        _required_non_negative_int(placeholder.attribute("idx"))
        if placeholder is not None and placeholder.attribute("idx") is not None
        else None
    )
    return shape_id, shape_name, placeholder_type, placeholder_index


def _reject_active(root: XmlElement) -> None:
    stack = [root]
    while stack:
        element = stack.pop()
        if element.name in _ACTIVE_NAMES:
            _active()
        stack.extend(reversed(element.child_elements()))


def _hidden(value: str | None) -> bool:
    if value is None or value in {"1", "true", "on"}:
        return False
    if value in {"0", "false", "off"}:
        return True
    return _invalid()


def _boolean(value: str | None) -> bool:
    if value is None or value in {"0", "false", "off"}:
        return False
    if value in {"1", "true", "on"}:
        return True
    return _invalid()


def _emu_to_milli_point(value: int | Fraction) -> int:
    scaled = Fraction(value) * 10 / 127
    quotient, remainder = divmod(scaled.numerator, scaled.denominator)
    doubled = remainder * 2
    if doubled < scaled.denominator:
        return quotient
    if doubled > scaled.denominator:
        return quotient + 1
    return quotient + quotient % 2


def _one_child(
    element: XmlElement,
    name: str,
    *,
    required: bool,
) -> XmlElement | None:
    matches = [item for item in element.child_elements() if item.name == name]
    if len(matches) > 1 or (required and not matches):
        _invalid()
    return matches[0] if matches else None


def _required_int(value: str | None) -> int:
    if value is None or re.fullmatch(r"-?[0-9]+", value) is None:
        _invalid()
    digits = value.removeprefix("-")
    if len(digits) > _MAX_INTEGER_DIGITS:
        _invalid()
    try:
        return int(value)
    except ValueError:
        return _invalid()


def _required_non_negative_int(value: str | None) -> int:
    result = _required_int(value)
    if result < 0:
        _invalid()
    return result


def _required_positive_int(value: str | None) -> int:
    result = _required_int(value)
    if result < 1:
        _invalid()
    return result


def _optional_positive_int(value: str | None, *, default: int) -> int:
    return default if value is None else _required_positive_int(value)


def _p(local_name: str) -> str:
    return expanded_name(_P, local_name)


def _a(local_name: str) -> str:
    return expanded_name(_A, local_name)


def _invalid() -> Never:
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _unsupported() -> Never:
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _active() -> Never:
    raise NativeParseFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)


def _result_limit() -> Never:
    raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)


def _locator_failure() -> Never:
    raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
