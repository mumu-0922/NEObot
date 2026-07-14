"""Deterministic DOCX structure parser over one admitted OPC capability."""

from __future__ import annotations

import re
from dataclasses import dataclass
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
from mm_chat_rag.offline_parser.native.opc import (
    OpcRelationship,
    ValidatedOpcPackage,
)
from mm_chat_rag.offline_parser.native.xml_source import (
    XmlElement,
    XmlText,
    expanded_name,
)

_W: Final = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
_R: Final = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
_XML: Final = "http://www.w3.org/XML/1998/namespace"
_DOCUMENT_URI: Final = "/word/document.xml"
_REL_BASE: Final = (
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)
_HEADING_STYLE: Final = re.compile(r"^Heading([1-9])$", re.IGNORECASE)
_MAX_LIST_LEVEL: Final = 8
_MAX_INTEGER_DIGITS: Final = 19
_HEADER_FOOTER_RELATIONSHIP_TYPES: Final = frozenset(
    {_REL_BASE + "footer", _REL_BASE + "header"}
)
_HEADER_FOOTER_REFERENCE_NAMES: Final = frozenset(
    {expanded_name(_W, "footerReference"), expanded_name(_W, "headerReference")}
)
_REVISION_DELETE_NAMES: Final = frozenset(
    {
        expanded_name(_W, "del"),
        expanded_name(_W, "delText"),
        expanded_name(_W, "moveFrom"),
        expanded_name(_W, "moveFromRangeEnd"),
        expanded_name(_W, "moveFromRangeStart"),
    }
)
_ACTIVE_NAMES: Final = frozenset(
    {
        expanded_name(_W, "altChunk"),
        expanded_name(_W, "control"),
        expanded_name(_W, "object"),
        expanded_name(_W, "oleObject"),
    }
)
_IGNORABLE_PARAGRAPH_CHILDREN: Final = frozenset(
    {
        expanded_name(_W, "bookmarkEnd"),
        expanded_name(_W, "bookmarkStart"),
        expanded_name(_W, "commentRangeEnd"),
        expanded_name(_W, "commentRangeStart"),
        expanded_name(_W, "proofErr"),
    }
)


@dataclass(frozen=True, slots=True)
class _ParagraphSpec:
    element: XmlElement
    kind: NativeNodeKind
    style_id: str | None
    numbering_id: int | None
    level: int | None


class _DocxBuilder:
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
                source_position=NativeBytePosition(
                    source_unit_ordinal=0,
                    raw_byte_start=0,
                    raw_byte_end=package.source_bytes,
                ),
            )
        ]
        self.fragment_count = 0
        self.attribute_count = 0
        self.cell_count = 0

    def build(self) -> NativeDocument:
        parsed = self.package.parse_xml_part(_DOCUMENT_URI)
        document = parsed.root
        if document.name != _qn("document"):
            _invalid()
        _reject_forbidden_features(document)
        if any(
            relationship.relationship_type in _HEADER_FOOTER_RELATIONSHIP_TYPES
            for relationship in self.package.relationships_from(_DOCUMENT_URI)
        ):
            _unsupported()
        body = _one_child(document, _qn("body"), required=True)
        if body is None:
            _invalid()
        self._body(
            body,
            parent_ordinal=0,
            relationship_source_uri=_DOCUMENT_URI,
        )
        self._notes(NativeNodeKind.FOOTNOTE, "footnotes", "footnote")
        self._notes(NativeNodeKind.ENDNOTE, "endnotes", "endnote")
        return NativeDocument(
            source_format=ParserFormat.DOCX,
            source_bytes=self.package.source_bytes,
            source_sha256=self.package.source_sha256,
            source_units=self.package.source_units,
            nodes=tuple(self.nodes),
        )

    def _body(
        self,
        body: XmlElement,
        *,
        parent_ordinal: int,
        relationship_source_uri: str,
    ) -> None:
        children = body.child_elements()
        cursor = 0
        while cursor < len(children):
            element = children[cursor]
            if element.name == _qn("p"):
                spec = _paragraph_spec(element)
                if spec.kind is NativeNodeKind.LIST_ITEM:
                    group: list[_ParagraphSpec] = [spec]
                    cursor += 1
                    while cursor < len(children) and children[cursor].name == _qn("p"):
                        candidate = _paragraph_spec(children[cursor])
                        if (
                            candidate.kind is not NativeNodeKind.LIST_ITEM
                            or candidate.numbering_id != spec.numbering_id
                        ):
                            break
                        group.append(candidate)
                        cursor += 1
                    self._list(
                        group,
                        parent_ordinal=parent_ordinal,
                        relationship_source_uri=relationship_source_uri,
                    )
                    continue
                self._paragraph(
                    spec,
                    parent_ordinal=parent_ordinal,
                    relationship_source_uri=relationship_source_uri,
                )
            elif element.name == _qn("tbl"):
                self._table(
                    element,
                    parent_ordinal=parent_ordinal,
                    relationship_source_uri=relationship_source_uri,
                )
            elif element.name == _qn("sectPr"):
                if any(
                    child.name in _HEADER_FOOTER_REFERENCE_NAMES
                    for child in element.child_elements()
                ):
                    _unsupported()
            elif element.name in _ACTIVE_NAMES:
                _active()
            else:
                _unsupported()
            cursor += 1

    def _list(
        self,
        paragraphs: list[_ParagraphSpec],
        *,
        parent_ordinal: int,
        relationship_source_uri: str,
    ) -> None:
        position = _cover(
            paragraphs[0].element.source_position,
            paragraphs[-1].element.source_position,
        )
        list_ordinal = self._append(
            NativeNodeKind.LIST,
            parent_ordinal,
            position,
            node_attributes=attributes(
                numberingId=paragraphs[0].numbering_id,
                ordered=None,
            ),
        )
        for paragraph in paragraphs:
            self._paragraph(
                paragraph,
                parent_ordinal=list_ordinal,
                relationship_source_uri=relationship_source_uri,
            )

    def _paragraph(
        self,
        spec: _ParagraphSpec,
        *,
        parent_ordinal: int,
        relationship_source_uri: str,
    ) -> int:
        fragments, inline = self._paragraph_content(
            spec.element,
            relationship_source_uri=relationship_source_uri,
        )
        values: dict[str, bool | int | str | None] = {"styleId": spec.style_id}
        if spec.kind is NativeNodeKind.HEADING:
            values["level"] = _heading_level(spec.style_id)
        elif spec.kind is NativeNodeKind.LIST_ITEM:
            values["level"] = spec.level
            values["numberingId"] = spec.numbering_id
        ordinal = self._append(
            spec.kind,
            parent_ordinal,
            spec.element.source_position,
            fragments=fragments,
            node_attributes=attributes(**values),
        )
        for element, kind, node_attributes in inline:
            self._append(
                kind,
                ordinal,
                element.source_position,
                node_attributes=node_attributes,
            )
        return ordinal

    def _paragraph_content(
        self,
        paragraph: XmlElement,
        *,
        relationship_source_uri: str,
    ) -> tuple[
        tuple[NativeFragment, ...],
        list[
            tuple[
                XmlElement,
                NativeNodeKind,
                tuple[NativeAttribute, ...],
            ]
        ],
    ]:
        text_runs: list[XmlText] = []
        inline: list[
            tuple[
                XmlElement,
                NativeNodeKind,
                tuple[NativeAttribute, ...],
            ]
        ] = []
        for child in paragraph.child_elements():
            if child.name == _qn("pPr") or child.name in _IGNORABLE_PARAGRAPH_CHILDREN:
                continue
            if child.name == _qn("r"):
                runs, breaks = _word_run(child)
                text_runs.extend(runs)
                inline.extend(
                    (
                        item,
                        NativeNodeKind.LINE_BREAK,
                        attributes(breakKind=break_kind),
                    )
                    for item, break_kind in breaks
                )
                continue
            if child.name == _qn("hyperlink"):
                relationship_id = child.attribute(expanded_name(_R, "id"))
                anchor = child.attribute(_qn("anchor"))
                relationship: OpcRelationship | None = None
                if relationship_id is not None:
                    relationship = self.package.resolve_relationship(
                        relationship_source_uri,
                        relationship_id,
                    )
                    if not relationship.relationship_type.endswith("/hyperlink"):
                        _invalid()
                elif anchor is None:
                    _invalid()
                for nested in child.child_elements():
                    if nested.name != _qn("r"):
                        _unsupported()
                    runs, breaks = _word_run(nested)
                    text_runs.extend(runs)
                    inline.extend(
                        (
                            item,
                            NativeNodeKind.LINE_BREAK,
                            attributes(breakKind=break_kind),
                        )
                        for item, break_kind in breaks
                    )
                inline.append(
                    (
                        child,
                        NativeNodeKind.ASSET_REF,
                        attributes(
                            external=(
                                relationship.is_external
                                if relationship is not None
                                else False
                            ),
                            externalTarget=(
                                relationship.target
                                if relationship is not None and relationship.is_external
                                else None
                            ),
                            nonIndexable=True,
                            relationshipId=relationship_id,
                        ),
                    )
                )
                continue
            if child.name in _ACTIVE_NAMES:
                _active()
            if child.name in _REVISION_DELETE_NAMES:
                _unsupported()
            _unsupported()
        fragments = tuple(
            NativeFragment(
                ordinal=index,
                role=NativeFragmentRole.TEXT,
                text=run.text,
                transform=run.transform,
                source_position=run.source_position,
            )
            for index, run in enumerate(text_runs)
            if run.text
        )
        self.fragment_count += len(fragments)
        self._check_limits()
        return fragments, inline

    def _table(
        self,
        table: XmlElement,
        *,
        parent_ordinal: int,
        relationship_source_uri: str,
    ) -> int:
        rows = [item for item in table.child_elements() if item.name == _qn("tr")]
        if any(
            item.name not in {_qn("tblPr"), _qn("tblGrid"), _qn("tr")}
            for item in table.child_elements()
        ):
            _unsupported()
        table_ordinal = self._append(
            NativeNodeKind.TABLE,
            parent_ordinal,
            table.source_position,
            node_attributes=attributes(rowCount=len(rows)),
        )
        for row_index, row in enumerate(rows):
            cells = [item for item in row.child_elements() if item.name == _qn("tc")]
            if any(
                item.name not in {_qn("trPr"), _qn("tc")}
                for item in row.child_elements()
            ):
                _unsupported()
            row_properties = _one_child(row, _qn("trPr"), required=False)
            header = (
                row_properties is not None
                and _one_child(
                    row_properties,
                    _qn("tblHeader"),
                    required=False,
                )
                is not None
            )
            row_ordinal = self._append(
                NativeNodeKind.TABLE_ROW,
                table_ordinal,
                row.source_position,
                node_attributes=attributes(header=header, rowIndex=row_index),
            )
            column_index = 0
            for cell in cells:
                self.cell_count += 1
                if self.cell_count > self.limits.cells:
                    raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
                span = _cell_span(cell)
                cell_ordinal = self._append(
                    NativeNodeKind.TABLE_CELL,
                    row_ordinal,
                    cell.source_position,
                    node_attributes=attributes(
                        columnIndex=column_index,
                        columnSpan=span,
                        rowIndex=row_index,
                        verticalMerge=_vertical_merge(cell),
                    ),
                )
                column_index += span
                for child in cell.child_elements():
                    if child.name == _qn("tcPr"):
                        continue
                    if child.name == _qn("p"):
                        self._paragraph(
                            _paragraph_spec(child),
                            parent_ordinal=cell_ordinal,
                            relationship_source_uri=relationship_source_uri,
                        )
                    elif child.name == _qn("tbl"):
                        self._table(
                            child,
                            parent_ordinal=cell_ordinal,
                            relationship_source_uri=relationship_source_uri,
                        )
                    else:
                        _unsupported()
        return table_ordinal

    def _notes(
        self,
        node_kind: NativeNodeKind,
        relationship_suffix: str,
        item_name: str,
    ) -> None:
        relationships = [
            item
            for item in self.package.relationships_from(_DOCUMENT_URI)
            if item.relationship_type == _REL_BASE + relationship_suffix
        ]
        if len(relationships) > 1:
            _invalid()
        if not relationships:
            return
        relationship = relationships[0]
        if relationship.is_external or relationship.target_part_uri is None:
            _invalid()
        parsed = self.package.parse_xml_part(relationship.target_part_uri)
        expected_root = _qn(relationship_suffix)
        if parsed.root.name != expected_root:
            _invalid()
        note_id_name = _qn("id")
        notes: list[tuple[int, XmlElement]] = []
        for child in parsed.root.child_elements():
            if child.name != _qn(item_name):
                _unsupported()
            note_id = _required_int(child.attribute(note_id_name))
            if note_id > 0:
                notes.append((note_id, child))
        notes.sort(key=lambda item: item[0])
        if len({item[0] for item in notes}) != len(notes):
            _invalid()
        for note_id, note in notes:
            note_ordinal = self._append(
                node_kind,
                0,
                note.source_position,
                node_attributes=attributes(noteId=note_id),
            )
            for child in note.child_elements():
                if child.name == _qn("p"):
                    self._paragraph(
                        _paragraph_spec(child),
                        parent_ordinal=note_ordinal,
                        relationship_source_uri=relationship.target_part_uri,
                    )
                elif child.name == _qn("tbl"):
                    self._table(
                        child,
                        parent_ordinal=note_ordinal,
                        relationship_source_uri=relationship.target_part_uri,
                    )
                else:
                    _unsupported()

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
        self._check_limits()
        return ordinal

    def _check_limits(self) -> None:
        if (
            len(self.nodes) > self.limits.nodes
            or self.fragment_count > self.limits.fragments
            or self.attribute_count > self.limits.attributes
        ):
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)


def parse_docx(
    package: ValidatedOpcPackage,
    limits: NativeParserLimits,
) -> NativeDocument:
    """Parse one pre-admitted DOCX without reopening or guessing its package."""
    if not isinstance(package, ValidatedOpcPackage):
        raise TypeError("DOCX parser requires a validated OPC package")
    if not isinstance(limits, NativeParserLimits):
        raise TypeError("DOCX parser limits have an invalid type")
    if package.parser_format is not ParserFormat.DOCX:
        raise NativeParseFailure(StableErrorCode.FORMAT_MISMATCH)
    return _DocxBuilder(package, limits).build()


def _paragraph_spec(paragraph: XmlElement) -> _ParagraphSpec:
    properties = _one_child(paragraph, _qn("pPr"), required=False)
    style_id: str | None = None
    numbering_id: int | None = None
    level: int | None = None
    if properties is not None:
        style = _one_child(properties, _qn("pStyle"), required=False)
        if style is not None:
            style_id = style.attribute(_qn("val"))
            if not style_id:
                _invalid()
        numbering = _one_child(properties, _qn("numPr"), required=False)
        if numbering is not None:
            number = _one_child(numbering, _qn("numId"), required=True)
            level_node = _one_child(numbering, _qn("ilvl"), required=True)
            if number is None or level_node is None:
                _invalid()
            numbering_id = _required_non_negative_int(number.attribute(_qn("val")))
            level = _required_non_negative_int(level_node.attribute(_qn("val")))
            if level > _MAX_LIST_LEVEL:
                _invalid()
    if style_id is not None and (
        style_id.casefold() == "title" or _HEADING_STYLE.fullmatch(style_id)
    ):
        kind = NativeNodeKind.HEADING
    elif numbering_id is not None:
        kind = NativeNodeKind.LIST_ITEM
    else:
        kind = NativeNodeKind.PARAGRAPH
    return _ParagraphSpec(paragraph, kind, style_id, numbering_id, level)


def _heading_level(style_id: str | None) -> int:
    if style_id is None or style_id.casefold() == "title":
        return 1
    match = _HEADING_STYLE.fullmatch(style_id)
    if match is None:
        _invalid()
    return int(match.group(1))


def _word_run(
    run: XmlElement,
) -> tuple[list[XmlText], list[tuple[XmlElement, str]]]:
    text: list[XmlText] = []
    breaks: list[tuple[XmlElement, str]] = []
    for child in run.child_elements():
        if child.name == _qn("rPr"):
            continue
        if child.name == _qn("t"):
            allowed_attributes = {expanded_name(_XML, "space")}
            if any(item.name not in allowed_attributes for item in child.attributes):
                _unsupported()
            if child.child_elements():
                _invalid()
            text.extend(child.text_runs())
        elif child.name == _qn("tab"):
            breaks.append((child, "tab"))
        elif child.name in {_qn("br"), _qn("cr")}:
            breaks.append((child, "line"))
        elif child.name in _ACTIVE_NAMES:
            _active()
        elif child.name in _REVISION_DELETE_NAMES:
            _unsupported()
        else:
            _unsupported()
    return text, breaks


def _cell_span(cell: XmlElement) -> int:
    properties = _one_child(cell, _qn("tcPr"), required=False)
    if properties is None:
        return 1
    span = _one_child(properties, _qn("gridSpan"), required=False)
    return 1 if span is None else _required_positive_int(span.attribute(_qn("val")))


def _vertical_merge(cell: XmlElement) -> str | None:
    properties = _one_child(cell, _qn("tcPr"), required=False)
    if properties is None:
        return None
    merge = _one_child(properties, _qn("vMerge"), required=False)
    if merge is None:
        return None
    value = merge.attribute(_qn("val")) or "continue"
    if value not in {"continue", "restart"}:
        _invalid()
    return value


def _reject_forbidden_features(root: XmlElement) -> None:
    stack = [root]
    while stack:
        element = stack.pop()
        if element.name in _ACTIVE_NAMES:
            _active()
        if element.name in _REVISION_DELETE_NAMES:
            _unsupported()
        stack.extend(reversed(element.child_elements()))


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


def _cover(
    start: NativeSourcePosition,
    end: NativeSourcePosition,
) -> NativeSourcePosition:
    if (
        start.source_unit_ordinal != end.source_unit_ordinal
        or start.raw_byte_start > end.raw_byte_end
    ):
        raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
    return NativeSourcePosition(
        source_unit_ordinal=start.source_unit_ordinal,
        raw_byte_start=start.raw_byte_start,
        raw_byte_end=end.raw_byte_end,
        decoded_scalar_start=start.decoded_scalar_start,
        decoded_scalar_end=end.decoded_scalar_end,
        start_line=start.start_line,
        start_column=start.start_column,
        end_line=end.end_line,
        end_column=end.end_column,
    )


def _required_int(value: str | None) -> int:
    if value is None or not re.fullmatch(r"-?[0-9]+", value):
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


def _qn(local_name: str) -> str:
    return expanded_name(_W, local_name)


def _invalid() -> Never:
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _unsupported() -> Never:
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _active() -> Never:
    raise NativeParseFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)
