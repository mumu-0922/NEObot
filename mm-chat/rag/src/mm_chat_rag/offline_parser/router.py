"""Magic/container-first, no-fallback format routing and safety preflight."""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from html.parser import HTMLParser
from typing import TYPE_CHECKING, Final

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode

if TYPE_CHECKING:
    from mm_chat_rag.offline_parser.config import NativeParserLimits
    from mm_chat_rag.offline_parser.native.opc import ValidatedOpcPackage

MAX_SOURCE_BYTES: Final = 52_428_800
MAX_PDF_PAGES: Final = 500

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
    opc_package: ValidatedOpcPackage | None = None

    @property
    def accepted(self) -> bool:
        """Return whether routing selected exactly one local format."""
        return self.parser_format is not None and self.stable_error_code is None


def route_source(
    source: bytes,
    *,
    declared_mime: str | None = None,
    declared_extension: str | None = None,
    native_limits: NativeParserLimits | None = None,
) -> RouteDecision:
    """Route exact source bytes without parser guessing or fallback."""
    try:
        parser_format, opc_package = _route_or_raise(
            source,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
            native_limits=native_limits,
        )
    except RouteFailure as error:
        return RouteDecision(parser_format=None, stable_error_code=error.code)
    return RouteDecision(
        parser_format=parser_format,
        stable_error_code=None,
        opc_package=opc_package,
    )


def _route_or_raise(
    source: bytes,
    *,
    declared_mime: str | None,
    declared_extension: str | None,
    native_limits: NativeParserLimits | None,
) -> tuple[ParserFormat, ValidatedOpcPackage | None]:
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

    structured = _structured_route(source, native_limits=native_limits)
    if structured is not None:
        structured_format, opc_package = structured
        _assert_structured_hints(
            structured_format,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
            mime_format=mime_format,
            extension_format=extension_format,
        )
        return structured_format, opc_package

    text = _decode_text(source)
    if (
        _looks_like_html(text)
        or mime_format is ParserFormat.HTML
        or extension_format is ParserFormat.HTML
    ):
        _validate_html(source, text)
        _assert_selected_hints(
            ParserFormat.HTML,
            declared_mime=declared_mime,
            declared_extension=declared_extension,
            mime_format=mime_format,
            extension_format=extension_format,
        )
        return ParserFormat.HTML, None

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
    return selected, None


def _structured_route(
    source: bytes,
    *,
    native_limits: NativeParserLimits | None,
) -> tuple[ParserFormat, ValidatedOpcPackage | None] | None:
    stripped = source.lstrip()
    if source.startswith(_PDF_MAGIC):
        _preflight_pdf(source)
        return ParserFormat.PDF, None
    if source.startswith(_ZIP_MAGICS):
        from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG  # noqa: PLC0415
        from mm_chat_rag.offline_parser.native.model import (  # noqa: PLC0415
            NativeParseFailure,
        )
        from mm_chat_rag.offline_parser.native.opc import (  # noqa: PLC0415
            admit_ooxml_package,
        )

        try:
            package = admit_ooxml_package(
                source,
                native_limits or DEFAULT_CONFIG.native,
            )
        except NativeParseFailure as error:
            raise RouteFailure(error.code) from error
        return package.parser_format, package
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
            return ParserFormat.SYNTHETIC_MINERU_ARTIFACT, None
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
    from mm_chat_rag.offline_parser.native.decoding import (  # noqa: PLC0415
        decode_text,
    )
    from mm_chat_rag.offline_parser.native.model import (  # noqa: PLC0415
        NativeParseFailure,
    )

    try:
        return decode_text(source).text
    except NativeParseFailure as error:
        raise RouteFailure(error.code) from error


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


def _validate_html(source: bytes, text: str) -> None:
    lowered = source.lower()
    if any(token in lowered for token in _HTML_DANGEROUS_TOKENS):
        raise RouteFailure(StableErrorCode.INPUT_INVALID)
    try:
        parser = _SafeHtmlPreflight()
        parser.feed(text)
        parser.close()
    except ValueError as error:
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
    counts = re.findall(rb"/Count\s+(\d+)", source)
    page_count = max((_pdf_page_count(value) for value in counts), default=0)
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


def _pdf_page_count(value: bytes) -> int:
    normalized = value.lstrip(b"0") or b"0"
    limit = str(MAX_PDF_PAGES).encode("ascii")
    if len(normalized) > len(limit) or (
        len(normalized) == len(limit) and normalized > limit
    ):
        return MAX_PDF_PAGES + 1
    return int(normalized)


def _validate_xml_bytes(content: bytes) -> None:
    from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG  # noqa: PLC0415
    from mm_chat_rag.offline_parser.native.model import (  # noqa: PLC0415
        NativeParseFailure,
    )
    from mm_chat_rag.offline_parser.native.xml_source import (  # noqa: PLC0415
        parse_xml_source,
    )

    try:
        parse_xml_source(
            content,
            source_unit_ordinal=0,
            limits=DEFAULT_CONFIG.native,
        )
    except NativeParseFailure as error:
        raise RouteFailure(error.code) from error


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
