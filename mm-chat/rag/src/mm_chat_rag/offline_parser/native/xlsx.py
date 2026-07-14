"""Deterministic XLSX Native Parser over one validated OPC capability."""

from __future__ import annotations

import heapq
import re
from dataclasses import dataclass
from typing import Final, Never

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeBytePosition,
    NativeDocument,
    NativeFragment,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeTransformKind,
    attributes,
)
from mm_chat_rag.offline_parser.native.opc import (
    OpcRelationship,
    ValidatedOpcPackage,
)
from mm_chat_rag.offline_parser.native.xml_source import (
    ParsedXmlSource,
    XmlElement,
    XmlText,
    expanded_name,
)

_SHEET_NAMESPACE: Final = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
_STRICT_SHEET_NAMESPACE: Final = "http://purl.oclc.org/ooxml/spreadsheetml/main"
_RELATIONSHIP_NAMESPACES: Final = (
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships",
    "http://purl.oclc.org/ooxml/officeDocument/relationships",
)
_XML_NAMESPACE: Final = "http://www.w3.org/XML/1998/namespace"
_WORKBOOK_URI: Final = "/xl/workbook.xml"
_MAX_XLSX_ROW: Final = 1_048_576
_MAX_XLSX_COLUMN: Final = 16_384
_MAX_SHEET_NAME: Final = 31
_MAX_INTEGER_DIGITS: Final = 19
_RANGE_PARTS: Final = 2
_A1_RE: Final = re.compile(r"^([A-Z]{1,3})([1-9][0-9]*)$")
_UNSIGNED_RE: Final = re.compile(r"^(?:0|[1-9][0-9]*)$")
_NUMBER_RE: Final = re.compile(
    r"^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[Ee][+-]?[0-9]+)?$"
)
_SUPPORTED_CELL_TYPES: Final = frozenset({"n", "s", "inlineStr", "str", "b", "e"})
_WORKBOOK_RELATIONSHIP_KINDS: Final = frozenset(
    {"worksheet", "sharedStrings", "styles"}
)


@dataclass(frozen=True, slots=True)
class _TextPiece:
    text: str
    xml_text: XmlText


@dataclass(frozen=True, slots=True)
class _SharedStrings:
    values: tuple[tuple[_TextPiece, ...], ...]
    declared_count: int


@dataclass(frozen=True, slots=True)
class _WorkbookSheet:
    ordinal: int
    name: str
    state: str
    target_uri: str


@dataclass(frozen=True, slots=True)
class _Cell:
    element: XmlElement
    row_index: int
    column_index: int
    reference: str
    value_kind: str
    pieces: tuple[_TextPiece, ...]
    fragment_role: NativeFragmentRole
    formula: XmlElement | None
    formula_pieces: tuple[_TextPiece, ...]
    style_index: int | None
    number_format_id: int
    hidden: bool
    path: str
    shared_string_reference: bool


@dataclass(frozen=True, slots=True)
class _CellContent:
    pieces: tuple[_TextPiece, ...]
    value_kind: str
    formula: XmlElement | None
    formula_pieces: tuple[_TextPiece, ...]
    shared_string_reference: bool


@dataclass(frozen=True, slots=True)
class _Row:
    element: XmlElement
    row_index: int
    hidden: bool
    cells: tuple[_Cell, ...]
    path: str


@dataclass(frozen=True, slots=True)
class _MergeRange:
    start_row: int
    start_column: int
    end_row: int
    end_column: int
    start_cell: str
    end_cell: str


@dataclass(frozen=True, slots=True)
class _Worksheet:
    parsed: ParsedXmlSource
    sheet_data: XmlElement
    rows: tuple[_Row, ...]
    merges: tuple[_MergeRange, ...]
    row_count: int
    column_count: int
    start_cell: str | None
    end_cell: str | None
    shared_string_references: int


class _ColumnOccupancy:
    """Fixed-size range tree for O(log columns) merge-overlap checks."""

    def __init__(self) -> None:
        self._tree = [False] * (_MAX_XLSX_COLUMN * 4)
        self._lazy: list[bool | None] = [None] * (_MAX_XLSX_COLUMN * 4)

    def any(self, start: int, end: int) -> bool:
        return self._query(1, 0, _MAX_XLSX_COLUMN - 1, start, end)

    def set(self, start: int, end: int, *, value: bool) -> None:
        self._set(1, 0, _MAX_XLSX_COLUMN - 1, start, end, value)

    def _push(self, node: int) -> None:
        value = self._lazy[node]
        if value is None:
            return
        left = node * 2
        self._tree[left] = value
        self._tree[left + 1] = value
        self._lazy[left] = value
        self._lazy[left + 1] = value
        self._lazy[node] = None

    def _query(
        self,
        node: int,
        left: int,
        right: int,
        start: int,
        end: int,
    ) -> bool:
        if start <= left and right <= end:
            return self._tree[node]
        self._push(node)
        middle = (left + right) // 2
        if start <= middle and self._query(
            node * 2,
            left,
            middle,
            start,
            end,
        ):
            return True
        return end > middle and self._query(
            node * 2 + 1,
            middle + 1,
            right,
            start,
            end,
        )

    def _set(
        self,
        node: int,
        left: int,
        right: int,
        start: int,
        end: int,
        value: bool,
    ) -> None:
        if start <= left and right <= end:
            self._tree[node] = value
            self._lazy[node] = value
            return
        self._push(node)
        middle = (left + right) // 2
        if start <= middle:
            self._set(node * 2, left, middle, start, end, value)
        if end > middle:
            self._set(node * 2 + 1, middle + 1, right, start, end, value)
        self._tree[node] = self._tree[node * 2] or self._tree[node * 2 + 1]


def parse_xlsx(
    package: ValidatedOpcPackage,
    limits: NativeParserLimits,
) -> NativeDocument:
    """Parse one admitted XLSX without reopening ZIPs or evaluating formulas."""
    if package.parser_format is not ParserFormat.XLSX:
        raise NativeParseFailure(StableErrorCode.FORMAT_MISMATCH)

    workbook = package.parse_xml_part(_WORKBOOK_URI)
    sheets, shared_uri, styles_uri = _parse_workbook(package, workbook, limits)
    shared = _parse_shared_strings(package, shared_uri, limits)
    style_formats = _parse_styles(package, styles_uri)

    worksheets: list[_Worksheet] = []
    cell_count = 0
    merge_count = 0
    shared_references = 0
    for sheet in sheets:
        worksheet = _parse_worksheet(
            package,
            sheet,
            shared,
            style_formats,
            limits,
        )
        worksheets.append(worksheet)
        cell_count += sum(len(row.cells) for row in worksheet.rows)
        merge_count += len(worksheet.merges)
        shared_references += worksheet.shared_string_references
        if cell_count > limits.cells or merge_count > limits.merged_ranges:
            _invalid()
    if shared is None:
        if shared_references:
            _invalid()
    elif shared_references != shared.declared_count:
        _invalid()

    nodes = _build_nodes(package.source_bytes, sheets, tuple(worksheets))
    return NativeDocument(
        source_format=ParserFormat.XLSX,
        source_bytes=package.source_bytes,
        source_sha256=package.source_sha256,
        source_units=package.source_units,
        nodes=nodes,
    )


def _parse_workbook(
    package: ValidatedOpcPackage,
    parsed: ParsedXmlSource,
    limits: NativeParserLimits,
) -> tuple[tuple[_WorkbookSheet, ...], str | None, str | None]:
    namespace = _sheet_namespace(parsed.root)
    if parsed.root.name != expanded_name(namespace, "workbook"):
        _invalid()
    children = _element_children(parsed.root)
    sheets_elements = [
        item for item in children if item.name == _n(namespace, "sheets")
    ]
    allowed = {_n(namespace, "sheets"), _n(namespace, "calcPr")}
    if len(sheets_elements) != 1:
        _invalid()
    if any(item.name not in allowed for item in children):
        _unsupported()

    relationships = package.relationships_from(_WORKBOOK_URI)
    shared_uri = _single_related_uri(relationships, "sharedStrings")
    styles_uri = _single_related_uri(relationships, "styles")
    if any(
        _relationship_kind(item) not in _WORKBOOK_RELATIONSHIP_KINDS
        for item in relationships
    ):
        _unsupported()

    result, referenced_relationships = _workbook_sheets(
        package,
        namespace,
        _element_children(sheets_elements[0]),
        limits,
    )
    worksheet_relationships = {
        item.relationship_id
        for item in relationships
        if _relationship_kind(item) == "worksheet"
    }
    if worksheet_relationships != referenced_relationships:
        _invalid()
    return result, shared_uri, styles_uri


def _workbook_sheets(
    package: ValidatedOpcPackage,
    namespace: str,
    sheet_items: tuple[XmlElement, ...],
    limits: NativeParserLimits,
) -> tuple[tuple[_WorkbookSheet, ...], set[str]]:
    if len(sheet_items) > limits.sheets:
        _invalid()
    result: list[_WorkbookSheet] = []
    names: set[str] = set()
    ids: set[int] = set()
    targets: set[str] = set()
    relationship_ids: set[str] = set()
    for ordinal, item in enumerate(sheet_items):
        if item.name != _n(namespace, "sheet") or _element_children(item):
            _invalid()
        values = _attribute_map(item)
        relationship_id = _relationship_attribute(values)
        allowed_attributes = {"name", "sheetId", "state", relationship_id[0]}
        if set(values) - allowed_attributes:
            _unsupported()
        name = values.get("name")
        sheet_id_text = values.get("sheetId")
        if (
            name is None
            or not name
            or len(name) > _MAX_SHEET_NAME
            or sheet_id_text is None
        ):
            _invalid()
        sheet_id = _positive_integer(sheet_id_text)
        if name.casefold() in names or sheet_id in ids:
            _invalid()
        names.add(name.casefold())
        ids.add(sheet_id)
        state = values.get("state", "visible")
        if state not in {"visible", "hidden", "veryHidden"}:
            _invalid()
        relation = package.resolve_relationship(
            _WORKBOOK_URI,
            relationship_id[1],
        )
        if (
            relation.is_external
            or _relationship_kind(relation) != "worksheet"
            or relation.target_part_uri is None
            or relation.target_part_uri in targets
        ):
            _invalid()
        target = package.part(relation.target_part_uri)
        if not target.is_xml or not target.content_type.endswith("worksheet+xml"):
            _invalid()
        targets.add(relation.target_part_uri)
        relationship_ids.add(relation.relationship_id)
        result.append(
            _WorkbookSheet(
                ordinal=ordinal,
                name=name,
                state=state,
                target_uri=relation.target_part_uri,
            )
        )
    return tuple(result), relationship_ids


def _parse_shared_strings(
    package: ValidatedOpcPackage,
    uri: str | None,
    limits: NativeParserLimits,
) -> _SharedStrings | None:
    if uri is None:
        return None
    parsed = package.parse_xml_part(uri)
    namespace = _sheet_namespace(parsed.root)
    if parsed.root.name != _n(namespace, "sst"):
        _invalid()
    values = _attribute_map(parsed.root)
    if set(values) != {"count", "uniqueCount"}:
        _invalid()
    declared_count = _unsigned_integer(values["count"])
    declared_unique = _unsigned_integer(values["uniqueCount"])
    elements = _element_children(parsed.root)
    if (
        any(item.name != _n(namespace, "si") for item in elements)
        or len(elements) != declared_unique
        or len(elements) > limits.shared_strings
    ):
        _invalid()
    return _SharedStrings(
        values=tuple(_string_item(item, namespace) for item in elements),
        declared_count=declared_count,
    )


def _parse_styles(
    package: ValidatedOpcPackage,
    uri: str | None,
) -> tuple[int, ...]:
    if uri is None:
        return ()
    parsed = package.parse_xml_part(uri)
    namespace = _sheet_namespace(parsed.root)
    if parsed.root.name != _n(namespace, "styleSheet"):
        _invalid()
    cell_xfs = [
        item
        for item in _element_children(parsed.root)
        if item.name == _n(namespace, "cellXfs")
    ]
    if len(cell_xfs) > 1:
        _invalid()
    if not cell_xfs:
        return ()
    values = _attribute_map(cell_xfs[0])
    if set(values) != {"count"}:
        _invalid()
    xfs = _element_children(cell_xfs[0])
    if _unsigned_integer(values["count"]) != len(xfs):
        _invalid()
    result: list[int] = []
    for xf in xfs:
        if xf.name != _n(namespace, "xf") or _element_children(xf):
            _invalid()
        num_format = xf.attribute("numFmtId")
        if num_format is None:
            _invalid()
        result.append(_unsigned_integer(num_format))
    return tuple(result)


def _parse_worksheet(
    package: ValidatedOpcPackage,
    sheet: _WorkbookSheet,
    shared: _SharedStrings | None,
    style_formats: tuple[int, ...],
    limits: NativeParserLimits,
) -> _Worksheet:
    parsed = package.parse_xml_part(sheet.target_uri)
    namespace = _sheet_namespace(parsed.root)
    if parsed.root.name != _n(namespace, "worksheet"):
        _invalid()
    children = _element_children(parsed.root)
    allowed = {
        _n(namespace, "cols"),
        _n(namespace, "sheetData"),
        _n(namespace, "mergeCells"),
    }
    if any(item.name not in allowed for item in children):
        _unsupported()
    sheet_data_items = [
        item for item in children if item.name == _n(namespace, "sheetData")
    ]
    cols_items = [item for item in children if item.name == _n(namespace, "cols")]
    merge_items = [
        item for item in children if item.name == _n(namespace, "mergeCells")
    ]
    if len(sheet_data_items) != 1 or len(cols_items) > 1 or len(merge_items) > 1:
        _invalid()
    hidden_columns = (
        _hidden_columns(cols_items[0], namespace) if cols_items else frozenset()
    )
    rows, shared_references = _rows(
        sheet_data_items[0],
        namespace,
        sheet,
        shared,
        style_formats,
        hidden_columns,
    )
    merges = _merge_ranges(merge_items[0], namespace, limits) if merge_items else ()
    cells = {
        (cell.row_index, cell.column_index): cell for row in rows for cell in row.cells
    }
    _validate_merge_anchors(merges, cells)

    row_count = max(
        [row.row_index + 1 for row in rows]
        + [item.end_row + 1 for item in merges]
        + [0]
    )
    column_count = max(
        [cell.column_index + 1 for cell in cells.values()]
        + [item.end_column + 1 for item in merges]
        + [0]
    )
    occupied = list(cells)
    if occupied:
        start_row = min(row for row, _column in occupied)
        start_column = min(column for _row, column in occupied)
        start_cell = _a1(start_row, start_column)
        end_cell = _a1(row_count - 1, column_count - 1)
    else:
        start_cell = None
        end_cell = None
    return _Worksheet(
        parsed=parsed,
        sheet_data=sheet_data_items[0],
        rows=rows,
        merges=merges,
        row_count=row_count,
        column_count=column_count,
        start_cell=start_cell,
        end_cell=end_cell,
        shared_string_references=shared_references,
    )


def _rows(
    sheet_data: XmlElement,
    namespace: str,
    sheet: _WorkbookSheet,
    shared: _SharedStrings | None,
    style_formats: tuple[int, ...],
    hidden_columns: frozenset[int],
) -> tuple[tuple[_Row, ...], int]:
    elements = _element_children(sheet_data)
    result: list[_Row] = []
    previous_row = -1
    shared_references = 0
    for row_ordinal, element in enumerate(elements, start=1):
        if element.name != _n(namespace, "row"):
            _invalid()
        values = _attribute_map(element)
        if set(values) - {"r", "hidden"} or "r" not in values:
            _unsupported()
        row_index = _positive_integer(values["r"]) - 1
        if row_index <= previous_row or row_index >= _MAX_XLSX_ROW:
            _invalid()
        previous_row = row_index
        hidden = _boolean(values.get("hidden", "0"))
        cells: list[_Cell] = []
        previous_column = -1
        for cell_ordinal, cell_element in enumerate(
            _element_children(element),
            start=1,
        ):
            cell = _cell(
                cell_element,
                namespace,
                sheet,
                row_index,
                hidden,
                hidden_columns,
                shared,
                style_formats,
                row_ordinal,
                cell_ordinal,
            )
            if cell.column_index <= previous_column:
                _invalid()
            previous_column = cell.column_index
            cells.append(cell)
            shared_references += int(cell.shared_string_reference)
        result.append(
            _Row(
                element=element,
                row_index=row_index,
                hidden=hidden,
                cells=tuple(cells),
                path=f"/worksheet[1]/sheetData[1]/row[{row_ordinal}]",
            )
        )
    return tuple(result), shared_references


def _cell(
    element: XmlElement,
    namespace: str,
    sheet: _WorkbookSheet,
    expected_row: int,
    row_hidden: bool,
    hidden_columns: frozenset[int],
    shared: _SharedStrings | None,
    style_formats: tuple[int, ...],
    row_ordinal: int,
    cell_ordinal: int,
) -> _Cell:
    if element.name != _n(namespace, "c"):
        _invalid()
    values = _attribute_map(element)
    if set(values) - {"r", "s", "t"} or "r" not in values:
        _unsupported()
    row_index, column_index, reference = _cell_reference(values["r"])
    if row_index != expected_row:
        _invalid()
    cell_type = values.get("t", "n")
    if cell_type not in _SUPPORTED_CELL_TYPES:
        _invalid()
    style_text = values.get("s")
    style_index = None if style_text is None else _unsigned_integer(style_text)
    if style_index is not None:
        if style_index >= len(style_formats):
            _invalid()
        number_format_id = style_formats[style_index]
    else:
        number_format_id = 0

    content = _cell_content(element, namespace, cell_type, shared)

    hidden = sheet.state != "visible" or row_hidden or column_index in hidden_columns
    return _Cell(
        element=element,
        row_index=row_index,
        column_index=column_index,
        reference=reference,
        value_kind=content.value_kind,
        pieces=content.pieces,
        fragment_role=(
            NativeFragmentRole.CACHED_VALUE
            if content.formula is not None
            else NativeFragmentRole.CELL_VALUE
        ),
        formula=content.formula,
        formula_pieces=content.formula_pieces,
        style_index=style_index,
        number_format_id=number_format_id,
        hidden=hidden,
        path=(f"/worksheet[1]/sheetData[1]/row[{row_ordinal}]/c[{cell_ordinal}]"),
        shared_string_reference=content.shared_string_reference,
    )


def _cell_content(
    element: XmlElement,
    namespace: str,
    cell_type: str,
    shared: _SharedStrings | None,
) -> _CellContent:
    children = _element_children(element)
    child_ranks = {
        _n(namespace, "f"): 0,
        _n(namespace, "v"): 1,
        _n(namespace, "is"): 2,
    }
    if any(item.name not in child_ranks for item in children):
        _unsupported()
    observed_ranks = [child_ranks[item.name] for item in children]
    if observed_ranks != sorted(observed_ranks) or len(observed_ranks) != len(
        set(observed_ranks)
    ):
        _invalid()
    formulas = [item for item in children if item.name == _n(namespace, "f")]
    values_xml = [item for item in children if item.name == _n(namespace, "v")]
    inline = [item for item in children if item.name == _n(namespace, "is")]
    formula = formulas[0] if formulas else None
    formula_pieces = _formula_pieces(formula, inline, cell_type)

    if cell_type == "s":
        if formula is not None or inline or len(values_xml) != 1 or shared is None:
            _invalid()
        index = _unsigned_integer(_single_text(values_xml[0]))
        if index >= len(shared.values):
            _invalid()
        return _CellContent(
            shared.values[index],
            "shared_string",
            None,
            (),
            shared_string_reference=True,
        )
    if cell_type == "inlineStr":
        if formula is not None or values_xml or len(inline) != 1:
            _invalid()
        return _CellContent(
            _string_item(inline[0], namespace),
            "inline_string",
            None,
            (),
            shared_string_reference=False,
        )
    if inline:
        _invalid()
    value_element = values_xml[0] if values_xml else None
    pieces = () if value_element is None else _direct_text_pieces(value_element)
    text = "".join(item.text for item in pieces)
    _validate_scalar_cell_value(cell_type, text, value_element is not None)
    value_kind = {
        "n": "number",
        "str": "formula_cached_string" if formula is not None else "string",
        "b": "boolean",
        "e": "error",
    }[cell_type]
    if value_element is None:
        value_kind = "blank"
    return _CellContent(
        pieces,
        value_kind,
        formula,
        formula_pieces,
        shared_string_reference=False,
    )


def _formula_pieces(
    formula: XmlElement | None,
    inline: list[XmlElement],
    cell_type: str,
) -> tuple[_TextPiece, ...]:
    if formula is None:
        return ()
    if formula.attributes or inline or cell_type in {"s", "inlineStr"}:
        _unsupported()
    pieces = _direct_text_pieces(formula)
    if not pieces or not "".join(item.text for item in pieces):
        _invalid()
    return pieces


def _merge_ranges(
    container: XmlElement,
    namespace: str,
    limits: NativeParserLimits,
) -> tuple[_MergeRange, ...]:
    values = _attribute_map(container)
    if set(values) != {"count"}:
        _invalid()
    elements = _element_children(container)
    if _unsigned_integer(values["count"]) != len(elements):
        _invalid()
    if len(elements) > limits.merged_ranges:
        _invalid()
    result: list[_MergeRange] = []
    previous_key: tuple[int, int, int, int] | None = None
    for element in elements:
        if element.name != _n(namespace, "mergeCell") or _element_children(element):
            _invalid()
        attrs = _attribute_map(element)
        if set(attrs) != {"ref"}:
            _invalid()
        item = _merge_range(attrs["ref"])
        key = (item.start_row, item.start_column, item.end_row, item.end_column)
        if previous_key is not None and key <= previous_key:
            _invalid()
        previous_key = key
        result.append(item)
    _validate_merge_overlap(tuple(result))
    return tuple(result)


def _validate_merge_overlap(ranges: tuple[_MergeRange, ...]) -> None:
    occupancy = _ColumnOccupancy()
    active: list[tuple[int, int, int]] = []
    for item in ranges:
        while active and active[0][0] < item.start_row:
            _end_row, start_column, end_column = heapq.heappop(active)
            occupancy.set(start_column, end_column, value=False)
        if occupancy.any(item.start_column, item.end_column):
            _invalid()
        occupancy.set(item.start_column, item.end_column, value=True)
        heapq.heappush(
            active,
            (item.end_row, item.start_column, item.end_column),
        )


def _validate_merge_anchors(
    ranges: tuple[_MergeRange, ...],
    cells: dict[tuple[int, int], _Cell],
) -> None:
    for item in ranges:
        if (item.start_row, item.start_column) not in cells:
            _invalid()
        for row, column in cells:
            if (
                (row, column) != (item.start_row, item.start_column)
                and item.start_row <= row <= item.end_row
                and item.start_column <= column <= item.end_column
            ):
                _invalid()


def _build_nodes(
    source_bytes: int,
    sheets: tuple[_WorkbookSheet, ...],
    worksheets: tuple[_Worksheet, ...],
) -> tuple[NativeNode, ...]:
    nodes: list[NativeNode] = [
        NativeNode(
            ordinal=0,
            kind=NativeNodeKind.DOCUMENT,
            parent_ordinal=None,
            source_position=NativeBytePosition(0, 0, source_bytes),
        ),
    ]
    for sheet, worksheet in zip(sheets, worksheets, strict=True):
        sheet_ordinal = len(nodes)
        sheet_non_indexable = sheet.state != "visible"
        worksheet_part_ordinal = worksheet.parsed.decoded.source_unit_ordinal
        nodes.append(
            NativeNode(
                ordinal=sheet_ordinal,
                kind=NativeNodeKind.SHEET,
                parent_ordinal=0,
                source_position=worksheet.parsed.root.source_position,
                attributes=attributes(
                    nonIndexable=sheet_non_indexable,
                    ooxmlPath="/worksheet[1]",
                    sheetName=sheet.name,
                    sheetOrdinal=sheet.ordinal,
                    sheetState=sheet.state,
                    worksheetPartOrdinal=worksheet_part_ordinal,
                ),
            )
        )
        table_ordinal = len(nodes)
        nodes.append(
            NativeNode(
                ordinal=table_ordinal,
                kind=NativeNodeKind.TABLE,
                parent_ordinal=sheet_ordinal,
                source_position=worksheet.sheet_data.source_position,
                attributes=attributes(
                    columnCount=worksheet.column_count,
                    endCell=worksheet.end_cell,
                    nonIndexable=sheet_non_indexable,
                    ooxmlPath="/worksheet[1]/sheetData[1]",
                    rowCount=worksheet.row_count,
                    sheetOrdinal=sheet.ordinal,
                    startCell=worksheet.start_cell,
                ),
            )
        )
        merge_by_anchor = {
            (item.start_row, item.start_column): item for item in worksheet.merges
        }
        for row in worksheet.rows:
            row_ordinal = len(nodes)
            row_non_indexable = sheet_non_indexable or row.hidden
            nodes.append(
                NativeNode(
                    ordinal=row_ordinal,
                    kind=NativeNodeKind.TABLE_ROW,
                    parent_ordinal=table_ordinal,
                    source_position=row.element.source_position,
                    attributes=attributes(
                        hidden=row.hidden,
                        nonIndexable=row_non_indexable,
                        ooxmlPath=row.path,
                        rowIndex=row.row_index,
                        sheetOrdinal=sheet.ordinal,
                    ),
                )
            )
            for cell in row.cells:
                merge = merge_by_anchor.get((cell.row_index, cell.column_index))
                end_cell = merge.end_cell if merge is not None else cell.reference
                row_span = (
                    merge.end_row - merge.start_row + 1 if merge is not None else 1
                )
                column_span = (
                    merge.end_column - merge.start_column + 1
                    if merge is not None
                    else 1
                )
                cell_ordinal = len(nodes)
                nodes.append(
                    NativeNode(
                        ordinal=cell_ordinal,
                        kind=NativeNodeKind.TABLE_CELL,
                        parent_ordinal=row_ordinal,
                        source_position=cell.element.source_position,
                        fragments=_fragments(cell.pieces, cell.fragment_role),
                        attributes=attributes(
                            columnIndex=cell.column_index,
                            columnSpan=column_span,
                            endCell=end_cell,
                            hidden=cell.hidden,
                            nonIndexable=cell.hidden,
                            numberFormatId=cell.number_format_id,
                            ooxmlPath=cell.path,
                            rowIndex=cell.row_index,
                            rowSpan=row_span,
                            sheetOrdinal=sheet.ordinal,
                            startCell=cell.reference,
                            styleIndex=cell.style_index,
                            valueKind=cell.value_kind,
                        ),
                    )
                )
                if cell.formula is not None:
                    nodes.append(
                        NativeNode(
                            ordinal=len(nodes),
                            kind=NativeNodeKind.FORMULA,
                            parent_ordinal=cell_ordinal,
                            source_position=cell.formula.source_position,
                            fragments=_fragments(
                                cell.formula_pieces,
                                NativeFragmentRole.FORMULA,
                            ),
                            attributes=attributes(
                                cachedValuePresent=bool(cell.pieces),
                                endCell=end_cell,
                                formulaType="normal",
                                ooxmlPath=f"{cell.path}/f[1]",
                                sheetOrdinal=sheet.ordinal,
                                startCell=cell.reference,
                            ),
                        )
                    )
    return tuple(nodes)


def _fragments(
    pieces: tuple[_TextPiece, ...],
    role: NativeFragmentRole,
) -> tuple[NativeFragment, ...]:
    return tuple(
        NativeFragment(
            ordinal=ordinal,
            role=role,
            text=piece.text,
            transform=NativeTransformKind.SYNTAX_DECODE,
            source_position=piece.xml_text.source_position,
        )
        for ordinal, piece in enumerate(pieces)
        if piece.text
    )


def _string_item(element: XmlElement, namespace: str) -> tuple[_TextPiece, ...]:
    children = _element_children(element)
    direct = [item for item in children if item.name == _n(namespace, "t")]
    runs = [item for item in children if item.name == _n(namespace, "r")]
    if direct and runs:
        _unsupported()
    if len(direct) == 1 and len(children) == 1:
        return _text_element(direct[0])
    if runs and len(runs) == len(children):
        result: list[_TextPiece] = []
        for run in runs:
            run_children = _element_children(run)
            texts = [item for item in run_children if item.name == _n(namespace, "t")]
            if len(texts) != 1 or any(
                item.name not in {_n(namespace, "rPr"), _n(namespace, "t")}
                for item in run_children
            ):
                _unsupported()
            result.extend(_text_element(texts[0]))
        return tuple(result)
    if not children:
        return ()
    return _unsupported()


def _text_element(element: XmlElement) -> tuple[_TextPiece, ...]:
    attrs = _attribute_map(element)
    space_name = expanded_name(_XML_NAMESPACE, "space")
    if set(attrs) - {space_name} or attrs.get(space_name, "preserve") not in {
        "default",
        "preserve",
    }:
        _invalid()
    if _has_element_children(element):
        _invalid()
    return _direct_text_pieces(element)


def _direct_text_pieces(element: XmlElement) -> tuple[_TextPiece, ...]:
    if _has_element_children(element):
        _invalid()
    return tuple(
        _TextPiece(item.text, item)
        for item in element.content
        if isinstance(item, XmlText) and item.text
    )


def _single_text(element: XmlElement) -> str:
    pieces = _direct_text_pieces(element)
    return "".join(item.text for item in pieces)


def _hidden_columns(container: XmlElement, namespace: str) -> frozenset[int]:
    result: set[int] = set()
    previous_end = 0
    for item in _element_children(container):
        if item.name != _n(namespace, "col") or _element_children(item):
            _invalid()
        values = _attribute_map(item)
        if set(values) - {"min", "max", "hidden"} or not {"min", "max"} <= set(values):
            _unsupported()
        minimum = _positive_integer(values["min"])
        maximum = _positive_integer(values["max"])
        if minimum <= previous_end or minimum > maximum or maximum > _MAX_XLSX_COLUMN:
            _invalid()
        previous_end = maximum
        if _boolean(values.get("hidden", "0")):
            result.update(range(minimum - 1, maximum))
    return frozenset(result)


def _validate_scalar_cell_value(
    cell_type: str,
    text: str,
    present: bool,
) -> None:
    if not present:
        return
    if cell_type == "n" and (not text or not _NUMBER_RE.fullmatch(text)):
        _invalid()
    if cell_type == "b" and text not in {"0", "1"}:
        _invalid()
    if cell_type in {"str", "e"} and not text:
        _invalid()


def _single_related_uri(
    relationships: tuple[OpcRelationship, ...],
    kind: str,
) -> str | None:
    matches = [item for item in relationships if _relationship_kind(item) == kind]
    if len(matches) > 1:
        _invalid()
    if not matches:
        return None
    item = matches[0]
    if item.is_external or item.target_part_uri is None:
        _invalid()
    return item.target_part_uri


def _relationship_attribute(values: dict[str, str]) -> tuple[str, str]:
    names = [expanded_name(namespace, "id") for namespace in _RELATIONSHIP_NAMESPACES]
    matches = [(name, values[name]) for name in names if name in values]
    if len(matches) != 1:
        _invalid()
    return matches[0]


def _relationship_kind(relationship: OpcRelationship) -> str:
    return relationship.relationship_type.rsplit("/", 1)[-1]


def _sheet_namespace(root: XmlElement) -> str:
    for namespace in (_SHEET_NAMESPACE, _STRICT_SHEET_NAMESPACE):
        if root.name.startswith(f"{{{namespace}}}"):
            return namespace
    return _invalid()


def _element_children(element: XmlElement) -> tuple[XmlElement, ...]:
    result: list[XmlElement] = []
    for item in element.content:
        if isinstance(item, XmlElement):
            result.append(item)
        elif item.text.strip():
            _invalid()
    return tuple(result)


def _attribute_map(element: XmlElement) -> dict[str, str]:
    return {item.name: item.value for item in element.attributes}


def _has_element_children(element: XmlElement) -> bool:
    return any(isinstance(item, XmlElement) for item in element.content)


def _n(namespace: str, local_name: str) -> str:
    return expanded_name(namespace, local_name)


def _cell_reference(value: str) -> tuple[int, int, str]:
    match = _A1_RE.fullmatch(value)
    if match is None:
        _invalid()
    column_text, row_text = match.groups()
    column = 0
    for character in column_text:
        column = column * 26 + ord(character) - ord("A") + 1
    row = _positive_integer(row_text)
    if column < 1 or column > _MAX_XLSX_COLUMN or row > _MAX_XLSX_ROW:
        _invalid()
    return row - 1, column - 1, value


def _merge_range(value: str) -> _MergeRange:
    parts = value.split(":")
    if len(parts) != _RANGE_PARTS:
        _invalid()
    start_row, start_column, start_cell = _cell_reference(parts[0])
    end_row, end_column, end_cell = _cell_reference(parts[1])
    if (
        (end_row, end_column) < (start_row, start_column)
        or (start_row, start_column) == (end_row, end_column)
        or start_row > end_row
        or start_column > end_column
    ):
        _invalid()
    return _MergeRange(
        start_row,
        start_column,
        end_row,
        end_column,
        start_cell,
        end_cell,
    )


def _a1(row: int, column: int) -> str:
    value = column + 1
    letters = ""
    while value:
        value, remainder = divmod(value - 1, 26)
        letters = chr(ord("A") + remainder) + letters
    return f"{letters}{row + 1}"


def _unsigned_integer(value: str) -> int:
    if not _UNSIGNED_RE.fullmatch(value) or len(value) > _MAX_INTEGER_DIGITS:
        _invalid()
    try:
        return int(value)
    except ValueError:
        return _invalid()


def _positive_integer(value: str) -> int:
    result = _unsigned_integer(value)
    if result < 1:
        _invalid()
    return result


def _boolean(value: str) -> bool:
    if value in {"1", "true"}:
        return True
    if value in {"0", "false"}:
        return False
    return _invalid()


def _invalid() -> Never:
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _unsupported() -> Never:
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)
