"""Fixed-dialect, source-aware CSV Native Parser."""

from __future__ import annotations

from dataclasses import dataclass

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import (
    DecodedSource,
    source_unit_from_decoded,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeFragment,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeSourceUnitKind,
    NativeTransformKind,
    attributes,
)


@dataclass(frozen=True, slots=True)
class _CsvField:
    """One decoded field and its complete source syntax span."""

    start: int
    end: int
    text: str
    quoted: bool


@dataclass(frozen=True, slots=True)
class _CsvRecord:
    """One source record including its exact record terminator."""

    start: int
    end: int
    terminator: str
    fields: tuple[_CsvField, ...]


def parse_csv(decoded: DecodedSource, limits: NativeParserLimits) -> NativeDocument:
    """Parse one comma-delimited CSV without sniffing or fallback."""
    records = _scan_records(decoded, limits)
    _validate_shape(records, limits)

    source_unit = source_unit_from_decoded(
        decoded,
        kind=NativeSourceUnitKind.RAW_FILE,
        canonical_uri=None,
    )
    nodes: list[NativeNode] = [
        NativeNode(
            ordinal=0,
            kind=NativeNodeKind.DOCUMENT,
            parent_ordinal=None,
            source_position=decoded.document_position(),
        ),
        NativeNode(
            ordinal=1,
            kind=NativeNodeKind.TABLE,
            parent_ordinal=0,
            source_position=decoded.position(0, len(decoded.text)),
            attributes=attributes(
                columnCount=len(records[0].fields),
                delimiter=",",
                rowCount=len(records),
            ),
        ),
    ]
    fragment_count = 0
    for row_index, record in enumerate(records):
        row_ordinal = len(nodes)
        nodes.append(
            NativeNode(
                ordinal=row_ordinal,
                kind=NativeNodeKind.TABLE_ROW,
                parent_ordinal=1,
                source_position=decoded.position(record.start, record.end),
                attributes=attributes(
                    recordTerminator=record.terminator,
                    rowIndex=row_index,
                ),
            )
        )
        for column_index, field in enumerate(record.fields):
            position = decoded.position(field.start, field.end)
            fragments: tuple[NativeFragment, ...] = ()
            if field.text:
                fragments = (
                    NativeFragment(
                        ordinal=0,
                        role=NativeFragmentRole.CELL_VALUE,
                        text=field.text,
                        transform=(
                            NativeTransformKind.SYNTAX_DECODE
                            if field.quoted
                            else NativeTransformKind.IDENTITY
                        ),
                        source_position=position,
                    ),
                )
                fragment_count += 1
            nodes.append(
                NativeNode(
                    ordinal=len(nodes),
                    kind=NativeNodeKind.TABLE_CELL,
                    parent_ordinal=row_ordinal,
                    source_position=position,
                    fragments=fragments,
                    attributes=attributes(
                        columnIndex=column_index,
                        empty=not field.text,
                        quoted=field.quoted,
                        rowIndex=row_index,
                    ),
                )
            )

    if len(nodes) > limits.nodes or fragment_count > limits.fragments:
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
    return NativeDocument(
        source_format=ParserFormat.CSV,
        source_bytes=len(decoded.source),
        source_sha256=source_unit.source_sha256,
        source_units=(source_unit,),
        nodes=tuple(nodes),
    )


def _scan_records(
    decoded: DecodedSource,
    limits: NativeParserLimits,
) -> tuple[_CsvRecord, ...]:
    text = decoded.text
    if not text:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    records: list[_CsvRecord] = []
    cursor = 0
    while cursor < len(text):
        record_start = cursor
        fields: list[_CsvField] = []
        while True:
            field, cursor = _scan_field(decoded, cursor, limits)
            fields.append(field)
            if len(fields) > limits.csv_columns:
                raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

            if cursor == len(text):
                terminator = ""
                break
            character = text[cursor]
            if character == ",":
                cursor += 1
                continue
            terminator, cursor = _consume_record_terminator(text, cursor)
            break

        records.append(
            _CsvRecord(
                start=record_start,
                end=cursor,
                terminator=terminator,
                fields=tuple(fields),
            )
        )
        if len(records) > limits.csv_rows:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    return tuple(records)


def _scan_field(
    decoded: DecodedSource,
    start: int,
    limits: NativeParserLimits,
) -> tuple[_CsvField, int]:
    text = decoded.text
    if start < len(text) and text[start] == '"':
        return _scan_quoted_field(decoded, start, limits)

    cursor = start
    while cursor < len(text) and text[cursor] not in ",\r\n":
        if text[cursor] == '"':
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        cursor += 1
    value = text[start:cursor]
    _enforce_field_limit(decoded, start, cursor, value, limits)
    return _CsvField(start, cursor, value, quoted=False), cursor


def _scan_quoted_field(
    decoded: DecodedSource,
    start: int,
    limits: NativeParserLimits,
) -> tuple[_CsvField, int]:
    text = decoded.text
    cursor = start + 1
    segment_start = cursor
    segments: list[str] = []
    while cursor < len(text):
        if text[cursor] != '"':
            cursor += 1
            _enforce_raw_field_limit(decoded, start, cursor, limits)
            continue
        if cursor + 1 < len(text) and text[cursor + 1] == '"':
            segments.append(text[segment_start:cursor])
            segments.append('"')
            cursor += 2
            segment_start = cursor
            _enforce_raw_field_limit(decoded, start, cursor, limits)
            continue

        segments.append(text[segment_start:cursor])
        cursor += 1
        value = "".join(segments)
        _enforce_field_limit(decoded, start, cursor, value, limits)
        if cursor < len(text) and text[cursor] not in ",\r\n":
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        return _CsvField(start, cursor, value, quoted=True), cursor
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _consume_record_terminator(text: str, cursor: int) -> tuple[str, int]:
    character = text[cursor]
    if character == "\n":
        return "\n", cursor + 1
    if character == "\r":
        if cursor + 1 < len(text) and text[cursor + 1] == "\n":
            return "\r\n", cursor + 2
        return "\r", cursor + 1
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _enforce_field_limit(
    decoded: DecodedSource,
    start: int,
    end: int,
    value: str,
    limits: NativeParserLimits,
) -> None:
    _enforce_raw_field_limit(decoded, start, end, limits)
    if len(value.encode("utf-8")) > limits.csv_field_bytes:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _enforce_raw_field_limit(
    decoded: DecodedSource,
    start: int,
    end: int,
    limits: NativeParserLimits,
) -> None:
    raw_bytes = decoded.raw_boundaries[end] - decoded.raw_boundaries[start]
    if raw_bytes > limits.csv_field_bytes:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _validate_shape(
    records: tuple[_CsvRecord, ...],
    limits: NativeParserLimits,
) -> None:
    columns = len(records[0].fields)
    if columns > limits.csv_columns or len(records) > limits.csv_rows:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    if any(len(record.fields) != columns for record in records[1:]):
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    node_count = 2 + len(records) + sum(len(record.fields) for record in records)
    fragment_count = sum(
        bool(field.text) for record in records for field in record.fields
    )
    if node_count > limits.nodes or fragment_count > limits.fragments:
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
