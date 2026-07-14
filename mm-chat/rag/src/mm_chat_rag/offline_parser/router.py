"""Magic/container-first, no-fallback format routing and safety preflight."""

from __future__ import annotations

import binascii
import io
import posixpath
import re
import stat
import struct
import unicodedata
import xml.etree.ElementTree as ET
import zipfile
import zlib
from collections.abc import Mapping
from dataclasses import dataclass
from html.parser import HTMLParser
from pathlib import PurePosixPath
from typing import Final
from urllib.parse import unquote_to_bytes

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode

MAX_SOURCE_BYTES: Final = 52_428_800
MAX_PDF_PAGES: Final = 500
MAX_ARCHIVE_ENTRIES: Final = 10_000
MAX_ARCHIVE_EXPANDED_BYTES: Final = 536_870_912
MAX_ARCHIVE_ENTRY_BYTES: Final = 67_108_864
MAX_ARCHIVE_COMPRESSION_RATIO: Final = 100
MAX_ENTRY_PATH_BYTES: Final = 512
MAX_XML_BYTES: Final = 67_108_864
MAX_XML_DEPTH: Final = 128
MAX_XML_NODES: Final = 1_000_000
MAX_XML_ATTRIBUTES: Final = 1_000_000
MAX_XML_TEXT_BYTES: Final = 33_000_000
MAX_XLSX_CELL_TEXT_BYTES: Final = 32_767

_ZIP_LOCAL_HEADER: Final = struct.Struct("<4s5H3I2H")
_ZIP_LOCAL_MAGIC: Final = b"PK\x03\x04"
_PDF_MAGIC: Final = b"%PDF-"
_ZIP_MAGICS: Final = (_ZIP_LOCAL_MAGIC, b"PK\x05\x06", b"PK\x07\x08")
_ASCII_TAB: Final = 9
_ASCII_CARRIAGE_RETURN: Final = 13
_ASCII_SPACE: Final = 32
_BBOX_COORDINATES: Final = 4
_ACTIVE_EXTENSIONS: Final = frozenset(
    {".docm", ".dotm", ".xlsm", ".xltm", ".pptm", ".potm"}
)
_FORMAT_MIMES: Final = {
    "text/plain": ParserFormat.TXT,
    "text/markdown": ParserFormat.MARKDOWN,
    "text/csv": ParserFormat.CSV,
    "text/html": ParserFormat.HTML,
    "application/pdf": ParserFormat.PDF,
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": (
        ParserFormat.DOCX
    ),
    "application/vnd.openxmlformats-officedocument.presentationml.presentation": (
        ParserFormat.PPTX
    ),
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": (
        ParserFormat.XLSX
    ),
    "application/vnd.mm-chat.synthetic-mineru+json": (
        ParserFormat.SYNTHETIC_MINERU_ARTIFACT
    ),
}
_FORMAT_EXTENSIONS: Final = {
    ".txt": ParserFormat.TXT,
    ".md": ParserFormat.MARKDOWN,
    ".markdown": ParserFormat.MARKDOWN,
    ".csv": ParserFormat.CSV,
    ".html": ParserFormat.HTML,
    ".htm": ParserFormat.HTML,
    ".pdf": ParserFormat.PDF,
    ".docx": ParserFormat.DOCX,
    ".pptx": ParserFormat.PPTX,
    ".xlsx": ParserFormat.XLSX,
}
_OOXML_REQUIRED_PARTS: Final = {
    ParserFormat.DOCX: "word/document.xml",
    ParserFormat.PPTX: "ppt/presentation.xml",
    ParserFormat.XLSX: "xl/workbook.xml",
}
_OOXML_CONTENT_MARKERS: Final = {
    ParserFormat.DOCX: b"wordprocessingml.document.main+xml",
    ParserFormat.PPTX: b"presentationml.presentation.main+xml",
    ParserFormat.XLSX: b"spreadsheetml.sheet.main+xml",
}
_DANGEROUS_XML_TOKENS: Final = (
    b"<!doctype",
    b"<!entity",
    b"<xi:include",
    b"<xinclude:include",
)
_HTML_DANGEROUS_TOKENS: Final = (
    b"<!entity",
    b"<xi:include",
    b"<xinclude:include",
    b"<script",
    b"javascript:",
)


class RouteFailure(ValueError):  # noqa: N818
    """A stable, no-fallback router or preflight failure."""

    def __init__(self, code: StableErrorCode) -> None:
        super().__init__(code.value)
        self.code = code


@dataclass(frozen=True, slots=True)
class RouteDecision:
    """Closed route result used before Native Parser activation."""

    parser_format: ParserFormat | None
    stable_error_code: StableErrorCode | None

    @property
    def accepted(self) -> bool:
        """Return whether routing selected exactly one local format."""
        return self.parser_format is not None and self.stable_error_code is None


def route_source(
    source: bytes,
    *,
    declared_mime: str | None = None,
    declared_extension: str | None = None,
) -> RouteDecision:
    """Route exact source bytes without parser guessing or fallback."""
    try:
        parser_format = _route_or_raise(
            source,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
        )
    except RouteFailure as error:
        return RouteDecision(parser_format=None, stable_error_code=error.code)
    return RouteDecision(parser_format=parser_format, stable_error_code=None)


def _route_or_raise(
    source: bytes,
    *,
    declared_mime: str | None,
    declared_extension: str | None,
) -> ParserFormat:
    if len(source) > MAX_SOURCE_BYTES:
        raise RouteFailure(StableErrorCode.INPUT_TOO_LARGE)
    if declared_extension in _ACTIVE_EXTENSIONS:
        raise RouteFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)

    mime_format = _FORMAT_MIMES.get(declared_mime) if declared_mime else None
    extension_format = (
        _FORMAT_EXTENSIONS.get(declared_extension) if declared_extension else None
    )
    if (
        mime_format is not None
        and extension_format is not None
        and mime_format is not extension_format
    ):
        raise RouteFailure(StableErrorCode.FORMAT_MISMATCH)

    structured = _structured_route(source)
    if structured is not None:
        _assert_structured_hints(
            structured,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
            mime_format=mime_format,
            extension_format=extension_format,
        )
        return structured

    text = _decode_text(source)
    if (
        _looks_like_html(text)
        or mime_format is ParserFormat.HTML
        or extension_format is ParserFormat.HTML
    ):
        _validate_html(source)
        _assert_selected_hints(
            ParserFormat.HTML,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
            mime_format=mime_format,
            extension_format=extension_format,
        )
        return ParserFormat.HTML

    selected = mime_format or extension_format
    if selected not in {ParserFormat.TXT, ParserFormat.MARKDOWN, ParserFormat.CSV}:
        if declared_mime is not None and declared_mime.startswith("text/"):
            raise RouteFailure(StableErrorCode.FORMAT_AMBIGUOUS)
        if declared_mime is not None or declared_extension is not None:
            raise RouteFailure(StableErrorCode.FORMAT_MISMATCH)
        raise RouteFailure(StableErrorCode.FORMAT_AMBIGUOUS)
    _assert_selected_hints(
        selected,
        declared_mime=declared_mime,
        declared_extension=declared_extension,
        mime_format=mime_format,
        extension_format=extension_format,
    )
    return selected


def _structured_route(source: bytes) -> ParserFormat | None:
    stripped = source.lstrip()
    if source.startswith(_PDF_MAGIC):
        _preflight_pdf(source)
        return ParserFormat.PDF
    if source.startswith(_ZIP_MAGICS):
        return _preflight_ooxml(source)
    lowered_source = source.lower()
    if b"\x00" in source or any(
        token in lowered_source for token in _DANGEROUS_XML_TOKENS[1:]
    ):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    lowered = stripped[:512].lower()
    if lowered.startswith(b"<?xml"):
        _validate_xml_bytes(source)
        raise RouteFailure(StableErrorCode.FORMAT_UNSUPPORTED)
    if lowered.startswith(b"{"):
        try:
            document = load_canonical_json_object(stripped)
        except CanonicalJsonError:
            return None
        if document.get("schemaVersion") == "synthetic-mineru-artifact.v1":
            _preflight_synthetic_mineru(document)
            return ParserFormat.SYNTHETIC_MINERU_ARTIFACT
    if _looks_binary(source):
        raise RouteFailure(StableErrorCode.FORMAT_UNSUPPORTED)
    return None


def _assert_structured_hints(
    parser_format: ParserFormat,
    *,
    declared_mime: str | None,
    declared_extension: str | None,
    mime_format: ParserFormat | None,
    extension_format: ParserFormat | None,
) -> None:
    if parser_format is ParserFormat.SYNTHETIC_MINERU_ARTIFACT:
        allowed_extension = declared_extension in {None, ".json"}
        allowed_mime = declared_mime in {
            None,
            "application/json",
            "application/vnd.mm-chat.synthetic-mineru+json",
        }
        if not allowed_extension or not allowed_mime:
            raise RouteFailure(StableErrorCode.FORMAT_MISMATCH)
        return
    _assert_selected_hints(
        parser_format,
        declared_mime=declared_mime,
        declared_extension=declared_extension,
        mime_format=mime_format,
        extension_format=extension_format,
    )


def _assert_selected_hints(
    parser_format: ParserFormat,
    *,
    declared_mime: str | None,
    declared_extension: str | None,
    mime_format: ParserFormat | None,
    extension_format: ParserFormat | None,
) -> None:
    if declared_mime is not None and mime_format is not parser_format:
        raise RouteFailure(StableErrorCode.FORMAT_MISMATCH)
    if declared_extension is not None and extension_format is not parser_format:
        raise RouteFailure(StableErrorCode.FORMAT_MISMATCH)


def _decode_text(source: bytes) -> str:
    if b"\x00" in source:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    encodings: tuple[str, ...]
    if source.startswith(b"\xef\xbb\xbf"):
        encodings = ("utf-8-sig",)
    else:
        encodings = ("utf-8", "gb18030")
    for encoding in encodings:
        try:
            text = source.decode(encoding, errors="strict")
        except UnicodeDecodeError:
            continue
        if "\ufffd" in text:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        return text
    raise RouteFailure(StableErrorCode.ENCODING_AMBIGUOUS)


def _looks_binary(source: bytes) -> bool:
    if b"\x00" in source:
        return True
    sample = source[:4096]
    if not sample:
        return False
    control = sum(
        byte < _ASCII_TAB or _ASCII_CARRIAGE_RETURN < byte < _ASCII_SPACE
        for byte in sample
    )
    return control * 20 > len(sample)


def _looks_like_html(text: str) -> bool:
    prefix = text.lstrip()[:512].casefold()
    return prefix.startswith(("<!doctype html", "<html"))


def _validate_html(source: bytes) -> None:
    lowered = source.lower()
    if any(token in lowered for token in _HTML_DANGEROUS_TOKENS):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    try:
        text = source.decode("utf-8-sig", errors="strict")
        parser = _SafeHtmlPreflight()
        parser.feed(text)
        parser.close()
    except (UnicodeDecodeError, ValueError) as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error


class _SafeHtmlPreflight(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)

    def handle_starttag(
        self,
        tag: str,
        attrs: list[tuple[str, str | None]],
    ) -> None:
        if tag.casefold() == "script":
            raise ValueError("script is forbidden")
        if any(name.casefold().startswith("on") for name, _value in attrs):
            raise ValueError("event handlers are forbidden")


def _preflight_pdf(source: bytes) -> None:
    stripped = source.rstrip()
    if (
        not stripped.endswith(b"%%EOF")
        or b"xref" not in source
        or b"trailer" not in source
    ):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if re.search(rb"/Encrypt\b", source):
        raise RouteFailure(StableErrorCode.PDF_ENCRYPTED_UNSUPPORTED)
    counts = [int(value) for value in re.findall(rb"/Count\s+(\d+)", source)]
    page_count = max(counts, default=0)
    if page_count > MAX_PDF_PAGES:
        raise RouteFailure(StableErrorCode.PAGE_LIMIT_EXCEEDED)
    if page_count < 1 or not re.search(rb"/Type\s*/Page\b", source):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if re.search(rb"/Subtype\s*/Image\b", source):
        raise RouteFailure(StableErrorCode.MINERU_REQUIRED)
    stream_bytes = b"\n".join(
        re.findall(rb"stream\r?\n(.*?)\r?\nendstream", source, re.DOTALL)
    )
    text_positions = len(re.findall(rb"\bT[dDm]\b", stream_bytes))
    drawing_operators = re.search(
        rb"(?:^|\s)-?\d+(?:\.\d+)?\s+-?\d+(?:\.\d+)?\s+[ml]\b", stream_bytes
    )
    if text_positions > 1 and drawing_operators is not None:
        raise RouteFailure(StableErrorCode.MINERU_REQUIRED)


def _preflight_ooxml(source: bytes) -> ParserFormat:
    try:
        archive = zipfile.ZipFile(io.BytesIO(source), mode="r")
    except (zipfile.BadZipFile, OSError) as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    with archive:
        entries = archive.infolist()
        if len(entries) > MAX_ARCHIVE_ENTRIES:
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        names = _validate_archive_entries(source, archive, entries)
        if "[Content_Types].xml" not in names:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        content_types = _read_entry(archive, "[Content_Types].xml")
        _validate_xml_bytes(content_types)
        lowered_types = content_types.lower()
        if b"macroenabled" in lowered_types or any(
            name.casefold().endswith("vbaproject.bin") for name in names
        ):
            raise RouteFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)
        if any("/embeddings/" in f"/{name.casefold()}" for name in names):
            raise RouteFailure(StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED)

        parser_format = _ooxml_format(lowered_types)
        required_part = _OOXML_REQUIRED_PARTS[parser_format]
        if required_part not in names:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        for name in sorted(names):
            lowered_name = name.casefold()
            if lowered_name.endswith((".xml", ".rels")):
                content = _read_entry(archive, name)
                _validate_xml_bytes(content)
                if parser_format is ParserFormat.XLSX and lowered_name.endswith(
                    "sharedstrings.xml"
                ):
                    _validate_xlsx_shared_strings(content)
                if lowered_name.endswith(".rels"):
                    _validate_relationships(name, content)
        return parser_format


def _validate_archive_entries(
    source: bytes,
    archive: zipfile.ZipFile,
    entries: list[zipfile.ZipInfo],
) -> set[str]:
    names: set[str] = set()
    casefolded: set[str] = set()
    expanded = 0
    for info in entries:
        name = info.filename
        _validate_entry_name(name, info.flag_bits)
        folded = name.casefold()
        if name in names or folded in casefolded:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        names.add(name)
        casefolded.add(folded)
        mode = (info.external_attr >> 16) & 0xFFFF
        if stat.S_ISLNK(mode):
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        if info.flag_bits & 0x1:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        if info.file_size > MAX_ARCHIVE_ENTRY_BYTES:
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        expanded += info.file_size
        if expanded > MAX_ARCHIVE_EXPANDED_BYTES:
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        if info.file_size and (
            info.compress_size == 0
            or info.file_size > info.compress_size * MAX_ARCHIVE_COMPRESSION_RATIO
        ):
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        if PurePosixPath(name).suffix.casefold() in {".zip", ".7z", ".rar", ".tar"}:
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        _validate_local_header(source, info)
        _verify_entry_stream(archive, info)
    return names


def _validate_entry_name(name: str, flags: int) -> None:
    try:
        encoded = name.encode("utf-8", errors="strict")
    except UnicodeEncodeError as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    if len(encoded) > MAX_ENTRY_PATH_BYTES:
        raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
    if not name or name != unicodedata.normalize("NFC", name):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if any(character in name for character in ("\\", "\x00")) or name.startswith("/"):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if not name.isascii() and not flags & 0x800:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    segments = name.split("/")
    if any(segment in {"", ".", ".."} for segment in segments):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if re.fullmatch(r"[A-Za-z]:", segments[0]):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    _validate_percent_encoded_path(segments)


def _validate_percent_encoded_path(segments: list[str]) -> None:
    canonical_segments: list[str] = []
    for segment in segments:
        if re.search(r"%(?:2f|2F|5c|5C|00)", segment):
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        try:
            raw = unquote_to_bytes(segment)
            decoded = raw.decode("utf-8", errors="strict")
        except (UnicodeDecodeError, ValueError) as error:
            raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
        if decoded in {"", ".", ".."}:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        canonical_segments.append(decoded)
    if len(canonical_segments) != len(set(canonical_segments)):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)


def _validate_local_header(source: bytes, info: zipfile.ZipInfo) -> None:
    offset = info.header_offset
    end = offset + _ZIP_LOCAL_HEADER.size
    if offset < 0 or end > len(source):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
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
    ) = _ZIP_LOCAL_HEADER.unpack(source[offset:end])
    if magic != _ZIP_LOCAL_MAGIC:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    name_start = end
    name_end = name_start + name_length
    data_start = name_end + extra_length
    if data_start > len(source):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    encoding = "utf-8" if flags & 0x800 else "cp437"
    try:
        local_name = source[name_start:name_end].decode(encoding, errors="strict")
    except UnicodeDecodeError as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    if (
        local_name != info.filename
        or flags != info.flag_bits
        or compression != info.compress_type
    ):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if not flags & 0x08 and (
        crc != info.CRC
        or compressed_size != info.compress_size
        or uncompressed_size != info.file_size
    ):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    if flags & 0x08:
        if crc not in {0, 0xFFFFFFFF}:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        if compressed_size not in {0, 0xFFFFFFFF} or uncompressed_size not in {
            0,
            0xFFFFFFFF,
        }:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        descriptor_offset = data_start + info.compress_size
        _validate_data_descriptor(source, descriptor_offset, info)


def _validate_data_descriptor(
    source: bytes, offset: int, info: zipfile.ZipInfo
) -> None:
    if offset < 0 or offset + 12 > len(source):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    cursor = offset
    if source[cursor : cursor + 4] == b"PK\x07\x08":
        cursor += 4
    if cursor + 12 > len(source):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    crc, compressed_size, uncompressed_size = struct.unpack_from("<III", source, cursor)
    if (
        crc != info.CRC
        or compressed_size != info.compress_size
        or uncompressed_size != info.file_size
    ):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    trailer = source[cursor + 12 : cursor + 16]
    if trailer and trailer not in {_ZIP_LOCAL_MAGIC, b"PK\x01\x02", b"PK\x05\x06"}:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)


def _verify_entry_stream(archive: zipfile.ZipFile, info: zipfile.ZipInfo) -> None:
    crc = 0
    size = 0
    try:
        with archive.open(info, mode="r") as stream:
            while chunk := stream.read(65_536):
                size += len(chunk)
                if size > MAX_ARCHIVE_ENTRY_BYTES:
                    raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
                crc = zlib.crc32(chunk, crc)
    except (zipfile.BadZipFile, RuntimeError, OSError, binascii.Error) as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    if size != info.file_size or crc & 0xFFFFFFFF != info.CRC:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)


def _read_entry(archive: zipfile.ZipFile, name: str) -> bytes:
    try:
        content = archive.read(name)
    except (KeyError, zipfile.BadZipFile, RuntimeError, OSError) as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    if len(content) > MAX_XML_BYTES:
        raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
    return content


def _ooxml_format(content_types: bytes) -> ParserFormat:
    matches = [
        parser_format
        for parser_format, marker in _OOXML_CONTENT_MARKERS.items()
        if marker in content_types
    ]
    if len(matches) != 1:
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    return matches[0]


def _validate_xml_bytes(content: bytes) -> None:
    if len(content) > MAX_XML_BYTES:
        raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
    lowered = content.lower()
    if any(token in lowered for token in _DANGEROUS_XML_TOKENS):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    try:
        # DTD/entity/XInclude tokens are rejected above before ElementTree sees
        # bytes; stdlib ElementTree performs no external network retrieval.
        root = ET.fromstring(content)  # noqa: S314
    except (ET.ParseError, UnicodeError, ValueError) as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    stack: list[tuple[ET.Element, int]] = [(root, 1)]
    nodes = 0
    attributes = 0
    text_bytes = 0
    while stack:
        node, depth = stack.pop()
        nodes += 1
        attributes += len(node.attrib)
        if node.text:
            text_bytes += len(node.text.encode("utf-8"))
        if node.tail:
            text_bytes += len(node.tail.encode("utf-8"))
        if (
            nodes > MAX_XML_NODES
            or depth > MAX_XML_DEPTH
            or attributes > MAX_XML_ATTRIBUTES
            or text_bytes > MAX_XML_TEXT_BYTES
        ):
            raise RouteFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)
        stack.extend((child, depth + 1) for child in reversed(node))


def _validate_relationships(relationship_part: str, content: bytes) -> None:
    try:
        root = ET.fromstring(content)  # noqa: S314 - preflighted by caller
    except ET.ParseError as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    ids: set[str] = set()
    for relation in root:
        relation_id = relation.attrib.get("Id")
        target = relation.attrib.get("Target")
        if not relation_id or relation_id in ids or target is None:
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        ids.add(relation_id)
        if relation.attrib.get("TargetMode") == "External":
            continue
        if "\\" in target or target.startswith("/"):
            raise RouteFailure(StableErrorCode.INPUT_INVALID)
        source_part = _relationship_source_part(relationship_part)
        resolved = posixpath.normpath(
            posixpath.join(posixpath.dirname(source_part), target)
        )
        if resolved == ".." or resolved.startswith("../"):
            raise RouteFailure(StableErrorCode.INPUT_INVALID)


def _relationship_source_part(relationship_part: str) -> str:
    if relationship_part == "_rels/.rels":
        return ""
    marker = "/_rels/"
    if marker not in relationship_part or not relationship_part.endswith(".rels"):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    prefix, filename = relationship_part.rsplit(marker, 1)
    return f"{prefix}/{filename.removesuffix('.rels')}"


def _validate_xlsx_shared_strings(content: bytes) -> None:
    try:
        root = ET.fromstring(content)  # noqa: S314 - preflighted by caller
    except ET.ParseError as error:
        raise RouteFailure(StableErrorCode.INPUT_INVALID) from error
    for node in root.iter():
        if (
            node.tag.rsplit("}", 1)[-1] == "t"
            and node.text is not None
            and len(node.text.encode("utf-8")) > MAX_XLSX_CELL_TEXT_BYTES
        ):
            raise RouteFailure(StableErrorCode.INPUT_INVALID)


def _preflight_synthetic_mineru(document: Mapping[str, object]) -> None:
    required = {
        "schemaVersion",
        "testOnly",
        "configNamespace",
        "goldenNamespace",
        "source",
        "profile",
        "role",
        "pages",
    }
    if set(document) != required or document.get("testOnly") is not True:
        raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
    if document.get("role") not in {"layout", "middle"}:
        raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
    pages = document.get("pages")
    if not isinstance(pages, list) or not pages:
        raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
    for expected_index, page in enumerate(pages):
        if not isinstance(page, dict) or page.get("pageIndex") != expected_index:
            raise RouteFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
        width = page.get("widthMilliPoint")
        height = page.get("heightMilliPoint")
        elements = page.get("elements")
        if (
            type(width) is not int
            or type(height) is not int
            or not isinstance(elements, list)
        ):
            raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
        for element in elements:
            if not isinstance(element, dict):
                raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
            bbox = element.get("bboxMilliPoint")
            if (
                not isinstance(bbox, list)
                or len(bbox) != _BBOX_COORDINATES
                or not all(type(item) is int for item in bbox)
            ):
                raise RouteFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
            x1, y1, x2, y2 = bbox
            if not (0 <= x1 < x2 <= width and 0 <= y1 < y2 <= height):
                raise RouteFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
