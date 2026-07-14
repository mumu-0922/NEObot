"""Single hardened OPC/OOXML package capability shared by Router and Parsers."""

from __future__ import annotations

import binascii
import hashlib
import io
import posixpath
import re
import stat
import struct
import unicodedata
import zipfile
import zlib
from dataclasses import dataclass, field
from pathlib import PurePosixPath
from typing import Final, Never
from urllib.parse import urlsplit

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeParseFailure,
    NativeSourceUnit,
    NativeSourceUnitKind,
)
from mm_chat_rag.offline_parser.native.xml_source import (
    ParsedXmlSource,
    XmlElement,
    XmlText,
    expanded_name,
    parse_xml_source,
)

_MAX_SOURCE_BYTES: Final = 52_428_800
_LOCAL_HEADER: Final = struct.Struct("<4s5H3I2H")
_EOCD: Final = struct.Struct("<4s4H2LH")
_LOCAL_MAGIC: Final = b"PK\x03\x04"
_CENTRAL_MAGIC: Final = b"PK\x01\x02"
_EOCD_MAGIC: Final = b"PK\x05\x06"
_DESCRIPTOR_MAGIC: Final = b"PK\x07\x08"
_ZIP64_EXTRA_ID: Final = 0x0001
_ZIP64_SENTINEL_16: Final = 0xFFFF
_ZIP64_SENTINEL_32: Final = 0xFFFFFFFF
_ASCII_SPACE: Final = 0x20
_NON_ASCII_MIN: Final = 0x80
_ALLOWED_METHODS: Final = frozenset({zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED})
_ALLOWED_COMMON_FLAGS: Final = 0x0808
_DEFLATE_OPTION_FLAGS: Final = 0x0006
_UNRESERVED: Final = frozenset(
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
)
_HEX: Final = frozenset("0123456789ABCDEFabcdef")
_RELATIONSHIP_ID: Final = re.compile(r"^[A-Za-z_][A-Za-z0-9_.-]{0,255}$")
_TYPE_NAMESPACE: Final = "http://schemas.openxmlformats.org/package/2006/content-types"
_RELATIONSHIP_NAMESPACE: Final = (
    "http://schemas.openxmlformats.org/package/2006/relationships"
)
_SPREADSHEET_NAMESPACES: Final = frozenset(
    {
        "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
        "http://purl.oclc.org/ooxml/spreadsheetml/main",
    }
)
_CONTENT_TYPES_URI: Final = "/[Content_Types].xml"
_ROOT_RELS_URI: Final = "/_rels/.rels"
_CONTENT_TYPES_PART_TYPE: Final = (
    "application/vnd.openxmlformats-package.content-types+xml"
)
_RELATIONSHIPS_PART_TYPE: Final = (
    "application/vnd.openxmlformats-package.relationships+xml"
)
_OFFICE_DOCUMENT_RELATIONSHIP_TYPES: Final = frozenset(
    {
        "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
        "officeDocument",
        "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument",
    }
)
_MAIN_TYPES: Final = {
    ParserFormat.DOCX: (
        "/word/document.xml",
        "application/vnd.openxmlformats-officedocument."
        "wordprocessingml.document.main+xml",
    ),
    ParserFormat.PPTX: (
        "/ppt/presentation.xml",
        "application/vnd.openxmlformats-officedocument."
        "presentationml.presentation.main+xml",
    ),
    ParserFormat.XLSX: (
        "/xl/workbook.xml",
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
    ),
}
_ACTIVE_CONTENT_TYPE_TOKENS: Final = (
    "macroenabled",
    "vbaproject",
    "activex",
    "oleobject",
)
_ARCHIVE_SUFFIXES: Final = frozenset({".zip", ".7z", ".rar", ".tar"})
_ALLOWED_EXTERNAL_RELATIONSHIP_TYPES: Final = frozenset(
    {
        "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
        "externalLink",
        "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
        "http://purl.oclc.org/ooxml/officeDocument/relationships/externalLink",
        "http://purl.oclc.org/ooxml/officeDocument/relationships/hyperlink",
    }
)


@dataclass(frozen=True, slots=True)
class OpcPart:
    """One canonical package Part and its immutable Source Unit metadata."""

    canonical_uri: str
    archive_name: str
    source_unit_ordinal: int
    content_type: str
    source_unit: NativeSourceUnit
    is_xml: bool


@dataclass(frozen=True, slots=True)
class OpcRelationship:
    """One closed OPC Relationship that is never dereferenced implicitly."""

    source_uri: str
    relationship_id: str
    relationship_type: str
    target: str
    target_mode: str
    target_part_uri: str | None

    @property
    def is_external(self) -> bool:
        """Return whether this target is intentionally non-dereferenced."""
        return self.target_mode == "External"


@dataclass(frozen=True, slots=True)
class ValidatedOpcPackage:
    """Capability granting bounded access to one fully admitted OOXML package."""

    parser_format: ParserFormat
    parts: tuple[OpcPart, ...]
    source_units: tuple[NativeSourceUnit, ...]
    relationships: tuple[OpcRelationship, ...]
    source_bytes: int
    source_sha256: str
    _source: bytes = field(repr=False, compare=False)
    _limits: NativeParserLimits = field(repr=False, compare=False)

    def part(self, canonical_uri: str) -> OpcPart:
        """Return one admitted Part by exact canonical URI."""
        for part in self.parts:
            if part.canonical_uri == canonical_uri:
                return part
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    def read_part(self, canonical_uri: str) -> bytes:
        """Read one admitted Part with a fresh size and SHA-256 check."""
        part = self.part(canonical_uri)
        try:
            with zipfile.ZipFile(io.BytesIO(self._source), mode="r") as archive:
                info = archive.getinfo(part.archive_name)
                content, digest = _read_entry(archive, info, self._limits, retain=True)
        except (KeyError, OSError, RuntimeError, zipfile.BadZipFile) as error:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
        if (
            content is None
            or len(content) != part.source_unit.source_bytes
            or digest != part.source_unit.source_sha256
        ):
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        return content

    def parse_xml_part(self, canonical_uri: str) -> ParsedXmlSource:
        """Parse an admitted XML Part under the same hardened XML policy."""
        part = self.part(canonical_uri)
        if not part.is_xml:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        return parse_xml_source(
            self.read_part(canonical_uri),
            source_unit_ordinal=part.source_unit_ordinal,
            limits=self._limits,
        )

    def relationships_from(self, source_uri: str) -> tuple[OpcRelationship, ...]:
        """Return source-ordered Relationships for one package or Part source."""
        return tuple(
            relationship
            for relationship in self.relationships
            if relationship.source_uri == source_uri
        )

    def resolve_relationship(
        self,
        source_uri: str,
        relationship_id: str,
    ) -> OpcRelationship:
        """Resolve one Relationship without reading its external target."""
        matches = tuple(
            relationship
            for relationship in self.relationships
            if relationship.source_uri == source_uri
            and relationship.relationship_id == relationship_id
        )
        if len(matches) != 1:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        return matches[0]

    @property
    def external_relationships(self) -> tuple[OpcRelationship, ...]:
        """Return external metadata that callers must never dereference."""
        return tuple(item for item in self.relationships if item.is_external)


@dataclass(frozen=True, slots=True)
class _EntryAdmission:
    info: zipfile.ZipInfo
    canonical_uri: str
    digest: str
    ordinal: int


def admit_ooxml_package(
    source: bytes,
    limits: NativeParserLimits,
) -> ValidatedOpcPackage:
    """Admit one complete DOCX/PPTX/XLSX package without extraction or fetches."""
    if not isinstance(source, bytes):
        raise TypeError("OOXML package source must be bytes")
    if len(source) > _MAX_SOURCE_BYTES:
        raise NativeParseFailure(StableErrorCode.INPUT_TOO_LARGE)
    central_start = _validate_eocd(source)
    try:
        archive = zipfile.ZipFile(io.BytesIO(source), mode="r")
    except (OSError, UnicodeError, ValueError, zipfile.BadZipFile) as error:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
    with archive:
        entries = archive.infolist()
        if not entries:
            _invalid()
        if len(entries) > limits.archive_entries:
            _limit()
        admitted = _admit_entries(source, archive, entries, central_start, limits)
        if any(
            entry.canonical_uri.casefold().endswith("/vbaproject.bin")
            or "/embeddings/" in entry.canonical_uri.casefold()
            for entry in admitted
        ):
            _active()
        by_uri = {entry.canonical_uri: entry for entry in admitted}
        content_types_entry = by_uri.get(_CONTENT_TYPES_URI)
        if content_types_entry is None:
            _invalid()
        content_types_bytes, _digest = _read_entry(
            archive,
            content_types_entry.info,
            limits,
            retain=True,
        )
        if content_types_bytes is None:
            _invalid()
        content_types_xml = parse_xml_source(
            content_types_bytes,
            source_unit_ordinal=content_types_entry.ordinal,
            limits=limits,
        )
        content_types, parser_format = _parse_content_types(
            content_types_xml.root,
            set(by_uri),
        )
        parts, source_units = _build_parts(
            source,
            archive,
            admitted,
            content_types,
            parser_format,
            limits,
        )
        relationships = _parse_relationships(
            archive,
            parts,
            limits,
            parser_format,
        )
    return ValidatedOpcPackage(
        parser_format=parser_format,
        parts=parts,
        source_units=source_units,
        relationships=relationships,
        source_bytes=len(source),
        source_sha256=hashlib.sha256(source).hexdigest(),
        _source=source,
        _limits=limits,
    )


def _validate_eocd(source: bytes) -> int:
    search_start = max(0, len(source) - (_EOCD.size + 65_535))
    offset = source.rfind(_EOCD_MAGIC, search_start)
    if offset < 0 or offset + _EOCD.size > len(source):
        _invalid()
    (
        magic,
        disk_number,
        central_disk,
        disk_entries,
        total_entries,
        central_size,
        central_offset,
        comment_length,
    ) = _EOCD.unpack_from(source, offset)
    if (
        magic != _EOCD_MAGIC
        or (disk_number, central_disk) != (0, 0)
        or disk_entries != total_entries
        or total_entries == _ZIP64_SENTINEL_16
        or _ZIP64_SENTINEL_32 in {central_size, central_offset}
        or offset + _EOCD.size + comment_length != len(source)
        or central_offset + central_size != offset
    ):
        _invalid()
    return int(central_offset)


def _admit_entries(
    source: bytes,
    archive: zipfile.ZipFile,
    entries: list[zipfile.ZipInfo],
    central_start: int,
    limits: NativeParserLimits,
) -> tuple[_EntryAdmission, ...]:
    canonical: dict[str, str] = {}
    folded: dict[str, str] = {}
    expanded = 0
    ranges: list[tuple[int, int]] = []
    records: list[tuple[zipfile.ZipInfo, str, str]] = []
    for info in entries:
        uri = _canonical_part_uri(info.filename, limits)
        if uri in canonical or uri.casefold() in folded:
            _invalid()
        canonical[uri] = info.filename
        folded[uri.casefold()] = uri
        _validate_zip_info(info)
        if info.file_size > limits.archive_entry_bytes:
            _limit()
        expanded += info.file_size
        if expanded > limits.archive_expanded_bytes:
            _limit()
        if info.file_size and (
            info.compress_size == 0
            or info.file_size > info.compress_size * limits.archive_compression_ratio
        ):
            _limit()
        if PurePosixPath(uri).suffix.casefold() in _ARCHIVE_SUFFIXES:
            _limit()
        range_end = _validate_local_header(source, info, central_start)
        ranges.append((info.header_offset, range_end))
        _content, digest = _read_entry(archive, info, limits, retain=False)
        records.append((info, uri, digest))
    _validate_local_ranges(ranges, central_start)
    records.sort(key=lambda item: item[1].encode("utf-8"))
    return tuple(
        _EntryAdmission(info=info, canonical_uri=uri, digest=digest, ordinal=index)
        for index, (info, uri, digest) in enumerate(records, start=1)
    )


def _validate_zip_info(info: zipfile.ZipInfo) -> None:
    allowed_flags = _ALLOWED_COMMON_FLAGS
    if info.compress_type == zipfile.ZIP_DEFLATED:
        allowed_flags |= _DEFLATE_OPTION_FLAGS
    if (
        info.compress_type not in _ALLOWED_METHODS
        or info.flag_bits & ~allowed_flags
        or info.flag_bits & 0x0041
        or _ZIP64_SENTINEL_32 in {info.file_size, info.compress_size}
        or info.volume != 0
    ):
        _invalid()
    mode = (info.external_attr >> 16) & 0xFFFF
    file_type = stat.S_IFMT(mode)
    if file_type not in {0, stat.S_IFREG}:
        _invalid()
    if _has_zip64_extra(info.extra):
        _invalid()


def _validate_local_header(
    source: bytes,
    info: zipfile.ZipInfo,
    central_start: int,
) -> int:
    offset = info.header_offset
    end = offset + _LOCAL_HEADER.size
    if offset < 0 or end > central_start:
        _invalid()
    (
        magic,
        _version,
        flags,
        compression,
        _time,
        _date,
        crc,
        compressed_size,
        uncompressed_size,
        name_length,
        extra_length,
    ) = _LOCAL_HEADER.unpack_from(source, offset)
    name_end = end + name_length
    data_start = name_end + extra_length
    data_end = int(data_start + info.compress_size)
    if magic != _LOCAL_MAGIC or data_end > central_start:
        _invalid()
    encoding = "utf-8" if flags & 0x0800 else "cp437"
    try:
        local_name = source[end:name_end].decode(encoding, errors="strict")
    except UnicodeDecodeError as error:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
    local_extra = source[name_end:data_start]
    if (
        local_name != info.filename
        or flags != info.flag_bits
        or compression != info.compress_type
        or _has_zip64_extra(local_extra)
    ):
        _invalid()
    if not flags & 0x0008:
        if (
            crc != info.CRC
            or compressed_size != info.compress_size
            or uncompressed_size != info.file_size
        ):
            _invalid()
        return data_end
    if crc != 0 or compressed_size != 0 or uncompressed_size != 0:
        _invalid()
    return _validate_descriptor(source, data_end, info, central_start)


def _validate_descriptor(
    source: bytes,
    offset: int,
    info: zipfile.ZipInfo,
    central_start: int,
) -> int:
    cursor = offset
    if source[cursor : cursor + 4] == _DESCRIPTOR_MAGIC:
        if int.from_bytes(_DESCRIPTOR_MAGIC, "little") == info.CRC:
            _invalid()
        cursor += 4
    if cursor + 12 > central_start:
        _invalid()
    crc, compressed_size, uncompressed_size = struct.unpack_from("<III", source, cursor)
    if (
        crc != info.CRC
        or compressed_size != info.compress_size
        or uncompressed_size != info.file_size
    ):
        _invalid()
    return cursor + 12


def _validate_local_ranges(ranges: list[tuple[int, int]], central_start: int) -> None:
    ordered = sorted(ranges)
    cursor = 0
    for start, end in ordered:
        if start != cursor or end <= start:
            _invalid()
        cursor = end
    if cursor != central_start:
        _invalid()


def _read_entry(
    archive: zipfile.ZipFile,
    info: zipfile.ZipInfo,
    limits: NativeParserLimits,
    *,
    retain: bool,
) -> tuple[bytes | None, str]:
    digest = hashlib.sha256()
    crc = 0
    size = 0
    chunks: list[bytes] | None = [] if retain else None
    try:
        with archive.open(info, mode="r") as stream:
            while chunk := stream.read(65_536):
                size += len(chunk)
                if size > limits.archive_entry_bytes:
                    _limit()
                crc = zlib.crc32(chunk, crc)
                digest.update(chunk)
                if chunks is not None:
                    chunks.append(chunk)
    except NativeParseFailure:
        raise
    except (binascii.Error, OSError, RuntimeError, zipfile.BadZipFile) as error:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
    if size != info.file_size or crc & 0xFFFFFFFF != info.CRC:
        _invalid()
    return (b"".join(chunks) if chunks is not None else None), digest.hexdigest()


def _parse_content_types(
    root: XmlElement,
    part_uris: set[str],
) -> tuple[dict[str, str], ParserFormat]:
    types_name = expanded_name(_TYPE_NAMESPACE, "Types")
    default_name = expanded_name(_TYPE_NAMESPACE, "Default")
    override_name = expanded_name(_TYPE_NAMESPACE, "Override")
    if root.name != types_name or root.attributes:
        _invalid()
    defaults: dict[str, str] = {}
    overrides: dict[str, str] = {}
    for item in root.content:
        if isinstance(item, XmlText):
            if item.text.strip():
                _invalid()
            continue
        if item.content:
            _invalid()
        values = {attribute.name: attribute.value for attribute in item.attributes}
        if item.name == default_name and set(values) == {"Extension", "ContentType"}:
            extension = values["Extension"].casefold()
            content_type = _content_type(values["ContentType"])
            if not extension or extension in defaults or not extension.isascii():
                _invalid()
            defaults[extension] = content_type
        elif item.name == override_name and set(values) == {
            "PartName",
            "ContentType",
        }:
            uri = _canonical_override_uri(values["PartName"])
            if uri in overrides or uri not in part_uris or uri == _CONTENT_TYPES_URI:
                _invalid()
            overrides[uri] = _content_type(values["ContentType"])
        else:
            _invalid()
    resolved = {_CONTENT_TYPES_URI: _CONTENT_TYPES_PART_TYPE}
    for uri in part_uris - {_CONTENT_TYPES_URI}:
        resolved_type: str | None = overrides.get(uri)
        if resolved_type is None:
            resolved_type = defaults.get(_part_extension(uri))
        if resolved_type is None:
            _invalid()
        resolved[uri] = resolved_type
    for uri in overrides:
        if uri not in part_uris:
            _invalid()
    lowered_types = tuple(value.casefold() for value in resolved.values())
    if any(
        token in content_type
        for content_type in lowered_types
        for token in _ACTIVE_CONTENT_TYPE_TOKENS
    ):
        _active()
    matches = [
        parser_format
        for parser_format, (_uri, content_type) in _MAIN_TYPES.items()
        if sum(value == content_type for value in resolved.values()) == 1
    ]
    if len(matches) != 1:
        _invalid()
    parser_format = matches[0]
    main_uri, main_type = _MAIN_TYPES[parser_format]
    if resolved.get(main_uri) != main_type:
        _invalid()
    return resolved, parser_format


def _build_parts(
    source: bytes,
    archive: zipfile.ZipFile,
    admitted: tuple[_EntryAdmission, ...],
    content_types: dict[str, str],
    parser_format: ParserFormat,
    limits: NativeParserLimits,
) -> tuple[tuple[OpcPart, ...], tuple[NativeSourceUnit, ...]]:
    raw_unit = NativeSourceUnit(
        ordinal=0,
        kind=NativeSourceUnitKind.RAW_FILE,
        canonical_uri=None,
        source_bytes=len(source),
        source_sha256=hashlib.sha256(source).hexdigest(),
        encoding=None,
        decoded_scalars=None,
    )
    parts: list[OpcPart] = []
    units: list[NativeSourceUnit] = [raw_unit]
    package_nodes = 0
    package_attributes = 0
    package_text_bytes = 0
    for entry in admitted:
        content_type = content_types[entry.canonical_uri]
        is_xml = _is_xml_part(entry.canonical_uri, content_type)
        encoding: str | None = None
        decoded_scalars: int | None = None
        if is_xml:
            content, digest = _read_entry(archive, entry.info, limits, retain=True)
            if content is None or digest != entry.digest:
                _invalid()
            parsed = parse_xml_source(
                content,
                source_unit_ordinal=entry.ordinal,
                limits=limits,
            )
            if (
                parser_format is ParserFormat.XLSX
                and entry.canonical_uri == "/xl/sharedStrings.xml"
            ):
                _validate_xlsx_shared_string_lengths(parsed.root, limits)
            encoding = parsed.decoded.encoding
            decoded_scalars = parsed.decoded.decoded_scalars
            package_nodes += parsed.node_count
            package_attributes += parsed.attribute_count
            package_text_bytes += parsed.text_bytes
            if (
                package_nodes > limits.xml_package_nodes
                or package_attributes > limits.xml_package_attributes
                or package_text_bytes > limits.xml_package_text_bytes
            ):
                _limit()
        unit = NativeSourceUnit(
            ordinal=entry.ordinal,
            kind=NativeSourceUnitKind.OOXML_PART,
            canonical_uri=entry.canonical_uri,
            source_bytes=entry.info.file_size,
            source_sha256=entry.digest,
            encoding=encoding,
            decoded_scalars=decoded_scalars,
        )
        parts.append(
            OpcPart(
                canonical_uri=entry.canonical_uri,
                archive_name=entry.info.filename,
                source_unit_ordinal=entry.ordinal,
                content_type=content_type,
                source_unit=unit,
                is_xml=is_xml,
            )
        )
        units.append(unit)
    if len(units) > limits.source_units:
        _limit()
    return tuple(parts), tuple(units)


def _parse_relationships(
    archive: zipfile.ZipFile,
    parts: tuple[OpcPart, ...],
    limits: NativeParserLimits,
    parser_format: ParserFormat,
) -> tuple[OpcRelationship, ...]:
    part_by_uri = {part.canonical_uri: part for part in parts}
    if _ROOT_RELS_URI not in part_by_uri:
        _invalid()
    relationships: list[OpcRelationship] = []
    seen_sources: set[str] = set()
    for part in parts:
        if not part.canonical_uri.endswith(".rels"):
            continue
        if part.content_type != _RELATIONSHIPS_PART_TYPE:
            _invalid()
        source_uri = _relationship_source_uri(part.canonical_uri)
        if source_uri in seen_sources or (
            source_uri != "/" and source_uri not in part_by_uri
        ):
            _invalid()
        seen_sources.add(source_uri)
        content, digest = _read_entry(
            archive,
            archive.getinfo(part.archive_name),
            limits,
            retain=True,
        )
        if content is None or digest != part.source_unit.source_sha256:
            _invalid()
        parsed = parse_xml_source(
            content,
            source_unit_ordinal=part.source_unit_ordinal,
            limits=limits,
        )
        relationships.extend(
            _relationships_from_xml(parsed.root, source_uri, set(part_by_uri))
        )
        if len(relationships) > limits.relationships:
            _limit()
    root_office = [
        item
        for item in relationships
        if item.source_uri == "/"
        and item.relationship_type in _OFFICE_DOCUMENT_RELATIONSHIP_TYPES
    ]
    expected_main_uri = _MAIN_TYPES[parser_format][0]
    if (
        len(root_office) != 1
        or root_office[0].is_external
        or root_office[0].target_part_uri != expected_main_uri
    ):
        _invalid()
    return tuple(
        sorted(
            relationships,
            key=lambda item: (
                item.source_uri.encode("utf-8"),
                item.relationship_id.encode("utf-8"),
            ),
        )
    )


def _relationships_from_xml(
    root: XmlElement,
    source_uri: str,
    part_uris: set[str],
) -> tuple[OpcRelationship, ...]:
    root_name = expanded_name(_RELATIONSHIP_NAMESPACE, "Relationships")
    item_name = expanded_name(_RELATIONSHIP_NAMESPACE, "Relationship")
    if root.name != root_name or root.attributes:
        _invalid()
    relationships: list[OpcRelationship] = []
    ids: set[str] = set()
    for content in root.content:
        if isinstance(content, XmlText):
            if content.text.strip():
                _invalid()
            continue
        values = {attribute.name: attribute.value for attribute in content.attributes}
        allowed = {"Id", "Type", "Target", "TargetMode"}
        if (
            content.name != item_name
            or content.content
            or not {"Id", "Type", "Target"} <= set(values) <= allowed
        ):
            _invalid()
        relationship_id = values["Id"]
        relationship_type = values["Type"]
        target = values["Target"]
        target_mode = values.get("TargetMode", "Internal")
        if (
            not _RELATIONSHIP_ID.fullmatch(relationship_id)
            or relationship_id in ids
            or not _safe_scalar(relationship_type)
            or not urlsplit(relationship_type).scheme
            or not _safe_scalar(target)
            or target_mode not in {"Internal", "External"}
        ):
            _invalid()
        ids.add(relationship_id)
        target_uri = None
        if target_mode == "Internal":
            target_uri = _resolve_internal_target(source_uri, target)
            if target_uri not in part_uris:
                _invalid()
        elif relationship_type not in _ALLOWED_EXTERNAL_RELATIONSHIP_TYPES:
            _invalid()
        relationships.append(
            OpcRelationship(
                source_uri=source_uri,
                relationship_id=relationship_id,
                relationship_type=relationship_type,
                target=target,
                target_mode=target_mode,
                target_part_uri=target_uri,
            )
        )
    return tuple(relationships)


def _relationship_source_uri(relationship_uri: str) -> str:
    if relationship_uri == _ROOT_RELS_URI:
        return "/"
    marker = "/_rels/"
    if marker not in relationship_uri or not relationship_uri.endswith(".rels"):
        _invalid()
    prefix, filename = relationship_uri.rsplit(marker, 1)
    if not prefix or not filename.removesuffix(".rels"):
        _invalid()
    return f"{prefix}/{filename.removesuffix('.rels')}"


def _resolve_internal_target(source_uri: str, target: str) -> str:
    if "\\" in target:
        _invalid()
    parsed = urlsplit(target)
    if parsed.scheme or parsed.netloc or parsed.query or parsed.fragment:
        _invalid()
    if not parsed.path or parsed.path.startswith("/"):
        _invalid()
    base = "/" if source_uri == "/" else posixpath.dirname(source_uri) + "/"
    segments = [segment for segment in base[1:].split("/") if segment]
    for raw_segment in parsed.path.split("/"):
        segment = _canonical_segment(raw_segment, allow_dot=True)
        if segment == ".":
            continue
        if segment == "..":
            if not segments:
                _invalid()
            segments.pop()
            continue
        segments.append(segment)
    if not segments:
        _invalid()
    return "/" + "/".join(segments)


def _canonical_part_uri(name: str, limits: NativeParserLimits) -> str:
    if len(name.encode("utf-8", errors="strict")) > limits.archive_path_bytes:
        _limit()
    if (
        not name
        or name.startswith("/")
        or name.endswith("/")
        or "\\" in name
        or "\x00" in name
        or name != unicodedata.normalize("NFC", name)
    ):
        _invalid()
    segments = name.split("/")
    if re.fullmatch(r"[A-Za-z]:", segments[0]):
        _invalid()
    canonical = [_canonical_segment(segment, allow_dot=False) for segment in segments]
    return "/" + "/".join(canonical)


def _canonical_override_uri(value: str) -> str:
    if not value.startswith("/"):
        _invalid()
    canonical = "/" + "/".join(
        _canonical_segment(segment, allow_dot=False) for segment in value[1:].split("/")
    )
    if canonical != value:
        _invalid()
    return canonical


def _canonical_segment(value: str, *, allow_dot: bool) -> str:
    if not value or value != unicodedata.normalize("NFC", value):
        _invalid()
    output: list[str] = []
    cursor = 0
    while cursor < len(value):
        character = value[cursor]
        if ord(character) < _ASCII_SPACE or character in {" ", "?", "#", "\\"}:
            _invalid()
        if character != "%":
            output.append(character)
            cursor += 1
            continue
        if (
            cursor + 2 >= len(value)
            or value[cursor + 1] not in _HEX
            or value[cursor + 2] not in _HEX
        ):
            _invalid()
        byte = int(value[cursor + 1 : cursor + 3], 16)
        decoded = chr(byte)
        if byte == 0 or byte >= _NON_ASCII_MIN or decoded in {"/", "\\"}:
            _invalid()
        output.append(decoded if decoded in _UNRESERVED else f"%{byte:02X}")
        cursor += 3
    canonical = "".join(output)
    if not allow_dot and canonical in {".", ".."}:
        _invalid()
    return canonical


def _has_zip64_extra(extra: bytes) -> bool:
    cursor = 0
    while cursor < len(extra):
        if cursor + 4 > len(extra):
            _invalid()
        identifier, size = struct.unpack_from("<HH", extra, cursor)
        cursor += 4
        if cursor + size > len(extra):
            _invalid()
        if identifier == _ZIP64_EXTRA_ID:
            return True
        cursor += size
    return False


def _content_type(value: str) -> str:
    if (
        not _safe_scalar(value)
        or value != value.strip()
        or "/" not in value
        or any(character.isspace() for character in value)
    ):
        _invalid()
    return value


def _part_extension(uri: str) -> str:
    name = uri.rsplit("/", 1)[-1]
    if "." not in name:
        return ""
    return name.rsplit(".", 1)[-1].casefold()


def _is_xml_part(uri: str, content_type: str) -> bool:
    lowered = content_type.casefold()
    return (
        uri == _CONTENT_TYPES_URI
        or uri.endswith((".xml", ".rels"))
        or lowered == "application/xml"
        or lowered.endswith("+xml")
    )


def _validate_xlsx_shared_string_lengths(
    root: XmlElement,
    limits: NativeParserLimits,
) -> None:
    namespace = next(
        (
            namespace
            for namespace in _SPREADSHEET_NAMESPACES
            if root.name == expanded_name(namespace, "sst")
        ),
        None,
    )
    if namespace is None:
        _invalid()
    string_item_name = expanded_name(namespace, "si")
    text_name = expanded_name(namespace, "t")
    for item in root.child_elements():
        if item.name != string_item_name:
            continue
        size = sum(
            len(text.text.encode("utf-8"))
            for element in _walk_xml_elements(item)
            if element.name == text_name
            for text in element.text_runs()
        )
        if size > limits.xlsx_cell_text_bytes:
            _invalid()


def _walk_xml_elements(root: XmlElement) -> tuple[XmlElement, ...]:
    result: list[XmlElement] = []
    pending = [root]
    while pending:
        current = pending.pop()
        result.append(current)
        pending.extend(reversed(current.child_elements()))
    return tuple(result)


def _safe_scalar(value: str) -> bool:
    return (
        bool(value)
        and "\x00" not in value
        and not any(ord(character) < _ASCII_SPACE for character in value)
    )


def _invalid() -> Never:
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _limit() -> Never:
    raise NativeParseFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)


def _active() -> Never:
    raise NativeParseFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)
