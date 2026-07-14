"""Source-aware hardened XML parsing for validated OOXML package Parts."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Final, Never
from xml.parsers import expat

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import (
    DecodedSource,
    decode_xml_source,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeParseFailure,
    NativeSourcePosition,
    NativeTransformKind,
)

_XINCLUDE_NAMESPACE: Final = "http://www.w3.org/2001/XInclude"
_UTF8_BOM: Final = b"\xef\xbb\xbf"


@dataclass(frozen=True, slots=True)
class XmlAttribute:
    """One expanded-name XML attribute with its decoded scalar value."""

    name: str
    value: str


@dataclass(frozen=True, slots=True)
class XmlText:
    """One source-derived XML character-data run."""

    text: str
    source_position: NativeSourcePosition
    transform: NativeTransformKind


type XmlContent = XmlElement | XmlText


@dataclass(frozen=True, slots=True)
class XmlElement:
    """One immutable expanded-name XML element in source order."""

    name: str
    attributes: tuple[XmlAttribute, ...]
    content: tuple[XmlContent, ...]
    source_position: NativeSourcePosition
    start_tag_position: NativeSourcePosition
    end_tag_position: NativeSourcePosition | None

    def attribute(self, name: str) -> str | None:
        """Return one exact expanded-name attribute, if present."""
        for attribute in self.attributes:
            if attribute.name == name:
                return attribute.value
        return None

    def child_elements(self) -> tuple[XmlElement, ...]:
        """Return direct child elements without losing source order."""
        return tuple(item for item in self.content if isinstance(item, XmlElement))

    def text_runs(self) -> tuple[XmlText, ...]:
        """Return direct character-data runs without recursive flattening."""
        return tuple(item for item in self.content if isinstance(item, XmlText))


@dataclass(frozen=True, slots=True)
class ParsedXmlSource:
    """A hardened XML tree bound to one decoded Native Source Unit."""

    decoded: DecodedSource
    root: XmlElement
    node_count: int
    attribute_count: int
    text_bytes: int


@dataclass(slots=True)
class _ElementDraft:
    name: str
    attributes: tuple[XmlAttribute, ...]
    start_byte: int
    start_tag_end: int
    self_closing: bool
    content: list[XmlContent] = field(default_factory=list)


@dataclass(slots=True)
class _PendingText:
    text: str
    start_byte: int


def parse_xml_source(
    source: bytes,
    *,
    source_unit_ordinal: int,
    limits: NativeParserLimits,
) -> ParsedXmlSource:
    """Parse strict UTF-8 XML without DTD, entities, PI, or XInclude."""
    _preflight_utf8(source)
    try:
        decoded = decode_xml_source(
            source,
            source_unit_ordinal=source_unit_ordinal,
            limits=limits,
        )
    except NativeParseFailure as error:
        if error.code is StableErrorCode.ENCODING_AMBIGUOUS:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
        raise
    builder = _ExpatBuilder(decoded, limits)
    return builder.parse()


def expanded_name(namespace: str, local_name: str) -> str:
    """Build the Clark name used by the hardened XML capability."""
    return f"{{{namespace}}}{local_name}" if namespace else local_name


def split_expanded_name(name: str) -> tuple[str, str]:
    """Split one Clark name into namespace URI and local name."""
    if not name.startswith("{"):
        return "", name
    closing = name.find("}")
    if closing <= 1 or closing == len(name) - 1:
        raise ValueError("invalid expanded XML name")
    return name[1:closing], name[closing + 1 :]


class _ExpatBuilder:
    def __init__(self, decoded: DecodedSource, limits: NativeParserLimits) -> None:
        self._decoded = decoded
        self._limits = limits
        self._parser = expat.ParserCreate(namespace_separator="}")
        self._parser.buffer_text = False
        self._parser.ordered_attributes = False
        self._parser.specified_attributes = True
        self._parser.SetParamEntityParsing(expat.XML_PARAM_ENTITY_PARSING_NEVER)
        self._stack: list[_ElementDraft] = []
        self._root: XmlElement | None = None
        self._pending_text: _PendingText | None = None
        self._node_count = 0
        self._attribute_count = 0
        self._text_bytes = 0
        self._install_handlers()

    def parse(self) -> ParsedXmlSource:
        try:
            self._parser.Parse(self._decoded.source, True)  # noqa: FBT003
            self._flush_text(len(self._decoded.source))
        except NativeParseFailure:
            raise
        except (UnicodeError, ValueError, expat.ExpatError) as error:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
        if self._root is None or self._stack:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        return ParsedXmlSource(
            decoded=self._decoded,
            root=self._root,
            node_count=self._node_count,
            attribute_count=self._attribute_count,
            text_bytes=self._text_bytes,
        )

    def _install_handlers(self) -> None:
        parser = self._parser
        parser.XmlDeclHandler = self._xml_decl
        parser.StartElementHandler = self._start_element
        parser.EndElementHandler = self._end_element
        parser.CharacterDataHandler = self._character_data
        parser.ProcessingInstructionHandler = self._forbidden_processing_instruction
        parser.StartDoctypeDeclHandler = self._forbidden_doctype
        parser.EntityDeclHandler = self._forbidden_entity
        parser.UnparsedEntityDeclHandler = self._forbidden_unparsed_entity
        parser.NotationDeclHandler = self._forbidden_notation
        parser.ExternalEntityRefHandler = self._forbidden_external_entity
        parser.SkippedEntityHandler = self._forbidden_skipped_entity
        parser.CommentHandler = self._comment
        parser.StartCdataSectionHandler = self._start_cdata
        parser.EndCdataSectionHandler = self._end_cdata

    def _xml_decl(
        self,
        version: str,
        encoding: str | None,
        _standalone: int,
    ) -> None:
        if version != "1.0" or (
            encoding is not None and encoding.casefold().replace("_", "-") != "utf-8"
        ):
            self._invalid()

    def _start_element(self, name: str, attributes: dict[str, str]) -> None:
        start = self._parser.CurrentByteIndex
        self._flush_text(start)
        normalized = _expanded_name_from_expat(name)
        namespace, _local_name = split_expanded_name(normalized)
        if namespace == _XINCLUDE_NAMESPACE:
            self._invalid()
        if len(self._stack) + 1 > self._limits.xml_depth:
            self._limit()
        self._node_count += 1
        self._attribute_count += len(attributes)
        if (
            self._node_count > self._limits.xml_nodes
            or self._attribute_count > self._limits.xml_attributes
        ):
            self._limit()
        normalized_attributes = tuple(
            sorted(
                (
                    XmlAttribute(_expanded_name_from_expat(key), value)
                    for key, value in attributes.items()
                ),
                key=lambda item: item.name.encode("utf-8"),
            )
        )
        start_tag_end = _scan_markup_end(self._decoded.source, start)
        self._stack.append(
            _ElementDraft(
                name=normalized,
                attributes=normalized_attributes,
                start_byte=start,
                start_tag_end=start_tag_end,
                self_closing=self._decoded.source[start:start_tag_end]
                .rstrip()
                .endswith(b"/>"),
            )
        )

    def _end_element(self, name: str) -> None:
        event_start = self._parser.CurrentByteIndex
        self._flush_text(event_start)
        if not self._stack:
            self._invalid()
        draft = self._stack.pop()
        if draft.name != _expanded_name_from_expat(name):
            self._invalid()
        if draft.self_closing:
            if event_start != draft.start_tag_end:
                self._invalid()
            end = event_start
            end_tag_position = None
        else:
            if event_start < draft.start_tag_end:
                self._invalid()
            end = _scan_markup_end(self._decoded.source, event_start)
            end_tag_position = self._position(event_start, end)
        element = XmlElement(
            name=draft.name,
            attributes=draft.attributes,
            content=tuple(draft.content),
            source_position=self._position(draft.start_byte, end),
            start_tag_position=self._position(draft.start_byte, draft.start_tag_end),
            end_tag_position=end_tag_position,
        )
        if self._stack:
            self._stack[-1].content.append(element)
        elif self._root is None:
            self._root = element
        else:
            self._invalid()

    def _character_data(self, text: str) -> None:
        start = self._parser.CurrentByteIndex
        self._flush_text(start)
        if text:
            self._pending_text = _PendingText(text=text, start_byte=start)

    def _flush_text(self, end_byte: int) -> None:
        pending = self._pending_text
        if pending is None:
            return
        self._pending_text = None
        if end_byte < pending.start_byte:
            self._invalid()
        if not self._stack:
            if pending.text.strip():
                self._invalid()
            return
        emitted = len(pending.text.encode("utf-8"))
        self._text_bytes += emitted
        if self._text_bytes > self._limits.xml_text_bytes:
            self._limit()
        raw = self._decoded.source[pending.start_byte : end_byte]
        try:
            raw_text = raw.decode("utf-8", errors="strict")
        except UnicodeDecodeError:
            raw_text = ""
        transform = (
            NativeTransformKind.IDENTITY
            if raw_text == pending.text
            else NativeTransformKind.SYNTAX_DECODE
        )
        self._stack[-1].content.append(
            XmlText(
                text=pending.text,
                source_position=self._position(pending.start_byte, end_byte),
                transform=transform,
            )
        )

    def _position(self, raw_start: int, raw_end: int) -> NativeSourcePosition:
        try:
            scalar_start = self._decoded.scalar_at_raw_boundary(raw_start)
            scalar_end = self._decoded.scalar_at_raw_boundary(raw_end)
            return self._decoded.position(scalar_start, scalar_end)
        except ValueError as error:
            raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED) from error

    def _comment(self, _text: str) -> None:
        self._flush_text(self._parser.CurrentByteIndex)

    def _start_cdata(self) -> None:
        self._flush_text(self._parser.CurrentByteIndex)

    def _end_cdata(self) -> None:
        self._flush_text(self._parser.CurrentByteIndex)

    def _forbidden_processing_instruction(self, _target: str, _data: str) -> None:
        self._invalid()

    def _forbidden_doctype(
        self,
        _name: str,
        _system_id: str | None,
        _public_id: str | None,
        _has_internal_subset: bool,
    ) -> None:
        self._invalid()

    def _forbidden_entity(
        self,
        _entity_name: str,
        _is_parameter_entity: int,
        _value: str | None,
        _base: str | None,
        _system_id: str | None,
        _public_id: str | None,
        _notation_name: str | None,
    ) -> None:
        self._invalid()

    def _forbidden_unparsed_entity(
        self,
        _entity_name: str,
        _base: str | None,
        _system_id: str,
        _public_id: str | None,
        _notation_name: str,
    ) -> None:
        self._invalid()

    def _forbidden_notation(
        self,
        _notation_name: str,
        _base: str | None,
        _system_id: str | None,
        _public_id: str | None,
    ) -> None:
        self._invalid()

    def _forbidden_external_entity(
        self,
        _context: str,
        _base: str | None,
        _system_id: str | None,
        _public_id: str | None,
    ) -> int:
        self._invalid()

    def _forbidden_skipped_entity(
        self,
        _entity_name: str,
        _is_parameter_entity: int,
    ) -> None:
        self._invalid()

    @staticmethod
    def _invalid() -> Never:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    @staticmethod
    def _limit() -> Never:
        raise NativeParseFailure(StableErrorCode.ARCHIVE_LIMIT_EXCEEDED)


def _preflight_utf8(source: bytes) -> None:
    if not isinstance(source, bytes):
        raise TypeError("XML source must be bytes")
    payload = source.removeprefix(_UTF8_BOM)
    try:
        text = payload.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error
    if "\x00" in text or "\ufffd" in text or text.encode("utf-8") != payload:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)


def _expanded_name_from_expat(name: str) -> str:
    if "}" not in name:
        return name
    namespace, local_name = name.split("}", 1)
    if not namespace or not local_name:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
    return expanded_name(namespace, local_name)


def _scan_markup_end(source: bytes, start: int) -> int:
    if start < 0 or start >= len(source) or source[start] != ord("<"):
        raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
    quote: int | None = None
    cursor = start + 1
    while cursor < len(source):
        byte = source[cursor]
        if quote is None:
            if byte in {ord("'"), ord('"')}:
                quote = byte
            elif byte == ord(">"):
                return cursor + 1
        elif byte == quote:
            quote = None
        cursor += 1
    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
