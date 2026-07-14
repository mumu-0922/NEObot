"""C1.3 deterministic, non-fetching HTML Native Parser."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass, field
from html import unescape
from html.parser import HTMLParser
from typing import Final, Never

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import DecodedSource
from mm_chat_rag.offline_parser.native.model import (
    NativeAttribute,
    NativeDocument,
    NativeFragment,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeTransformKind,
    attributes,
)

_TAG_RE: Final = re.compile(r"^[a-z][a-z0-9:-]{0,63}$")
_ATTRIBUTE_RE: Final = re.compile(r"^[a-z][a-z0-9:._-]{0,63}$")
_DOCTYPE_RE: Final = re.compile(r"<!doctype\s+html\s*>", re.IGNORECASE)
_END_TAG_RE: Final = re.compile(r"</\s*([a-z][a-z0-9:-]{0,63})\s*>", re.IGNORECASE)
_ENTITY_RE: Final = re.compile(r"&([A-Za-z][A-Za-z0-9]+);?")
_CHARREF_RE: Final = re.compile(r"&#(?:[xX][0-9A-Fa-f]+|[0-9]+);?")

_FORBIDDEN_TAGS: Final = frozenset({"applet", "embed", "iframe", "object", "script"})
_XINCLUDE_TAGS: Final = frozenset({"xi:include", "xinclude:include"})
_VOID_TAGS: Final = frozenset(
    {
        "area",
        "base",
        "br",
        "col",
        "hr",
        "img",
        "input",
        "link",
        "meta",
        "param",
        "source",
        "track",
        "wbr",
    }
)
_REFERENCE_ATTRIBUTES: Final = frozenset(
    {"action", "cite", "data", "formaction", "href", "poster", "src", "srcset"}
)
_TABLE_SECTIONS: Final = frozenset({"thead", "tbody", "tfoot"})
_TABLE_TEXT_FORBIDDEN: Final = frozenset(
    {"colgroup", "ol", "table", "tbody", "tfoot", "thead", "tr", "ul"}
)
_BLOCK_TAGS: Final = frozenset(
    {
        "address",
        "article",
        "aside",
        "blockquote",
        "details",
        "div",
        "dl",
        "fieldset",
        "figcaption",
        "figure",
        "footer",
        "form",
        "h1",
        "h2",
        "h3",
        "h4",
        "h5",
        "h6",
        "header",
        "hr",
        "main",
        "nav",
        "ol",
        "p",
        "pre",
        "section",
        "table",
        "ul",
    }
)
_HEAD_ALLOWED: Final = frozenset(
    {"base", "link", "meta", "noscript", "style", "template", "title"}
)
_HEADING_TAGS: Final = frozenset({"h1", "h2", "h3", "h4", "h5", "h6"})
_NODE_KIND_BY_TAG: Final = {
    "blockquote": NativeNodeKind.QUOTE,
    "br": NativeNodeKind.LINE_BREAK,
    "code": NativeNodeKind.CODE,
    "h1": NativeNodeKind.HEADING,
    "h2": NativeNodeKind.HEADING,
    "h3": NativeNodeKind.HEADING,
    "h4": NativeNodeKind.HEADING,
    "h5": NativeNodeKind.HEADING,
    "h6": NativeNodeKind.HEADING,
    "hr": NativeNodeKind.THEMATIC_BREAK,
    "img": NativeNodeKind.ASSET_REF,
    "li": NativeNodeKind.LIST_ITEM,
    "ol": NativeNodeKind.LIST,
    "p": NativeNodeKind.PARAGRAPH,
    "pre": NativeNodeKind.CODE,
    "q": NativeNodeKind.QUOTE,
    "source": NativeNodeKind.ASSET_REF,
    "table": NativeNodeKind.TABLE,
    "td": NativeNodeKind.TABLE_CELL,
    "th": NativeNodeKind.TABLE_CELL,
    "tr": NativeNodeKind.TABLE_ROW,
    "ul": NativeNodeKind.LIST,
}


@dataclass(slots=True)
class _NodeDraft:
    ordinal: int
    kind: NativeNodeKind
    parent_ordinal: int | None
    start_scalar: int
    end_scalar: int | None
    attributes: tuple[NativeAttribute, ...]
    fragments: list[NativeFragment] = field(default_factory=list)


@dataclass(frozen=True, slots=True)
class _OpenElement:
    tag: str
    node_ordinal: int


def parse_html(decoded: DecodedSource, limits: NativeParserLimits) -> NativeDocument:
    """Parse one HTML source without fetching or executing referenced content."""
    parser = _NativeHtmlParser(decoded, limits)
    try:
        parser.feed(decoded.text)
        parser.close()
        return parser.document()
    except NativeParseFailure:
        raise
    except (AssertionError, UnicodeError, ValueError) as error:
        raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH) from error


class _NativeHtmlParser(HTMLParser):
    """Strict source-order builder over stdlib's non-fetching tokenizer."""

    def __init__(self, decoded: DecodedSource, limits: NativeParserLimits) -> None:
        super().__init__(convert_charrefs=False)
        self._decoded = decoded
        self._limits = limits
        self._cursor = 0
        self._attribute_count = 0
        self._fragment_count = 0
        self._emitted_text_bytes = 0
        self._doctype_seen = False
        self._element_seen = False
        self._html_seen = False
        self._head_seen = False
        self._body_seen = False
        self._stack: list[_OpenElement] = []
        self._drafts = [
            _NodeDraft(
                ordinal=0,
                kind=NativeNodeKind.DOCUMENT,
                parent_ordinal=None,
                start_scalar=0,
                end_scalar=len(decoded.text),
                attributes=(),
            )
        ]
        if (
            len(decoded.line_starts) > limits.lines
            or len(decoded.text.encode("utf-8")) > limits.text_bytes
        ):
            self._limit_failure()

    def handle_decl(self, decl: str) -> None:
        token, start, end = self._consume_markup()
        del decl, start, end
        if self._doctype_seen or self._element_seen or not _DOCTYPE_RE.fullmatch(token):
            self._input_failure()
        self._doctype_seen = True

    def unknown_decl(self, data: str) -> None:
        del data
        self._input_failure()

    def handle_pi(self, data: str) -> None:
        del data
        self._input_failure()

    def handle_comment(self, data: str) -> None:
        self._consume_exact(f"<!--{data}-->")

    def handle_starttag(
        self,
        tag: str,
        attrs: list[tuple[str, str | None]],
    ) -> None:
        raw = self.get_starttag_text()
        if raw is None:
            self._input_failure()
        start, end = self._consume_exact(raw)
        self._start_element(tag, attrs, start, end, self_closing=False)

    def handle_startendtag(
        self,
        tag: str,
        attrs: list[tuple[str, str | None]],
    ) -> None:
        raw = self.get_starttag_text()
        if raw is None:
            self._input_failure()
        start, end = self._consume_exact(raw)
        normalized = self._validate_tag(tag)
        if normalized not in _VOID_TAGS:
            self._input_failure()
        self._start_element(normalized, attrs, start, end, self_closing=True)

    def handle_endtag(self, tag: str) -> None:
        token, _start, end = self._consume_markup()
        match = _END_TAG_RE.fullmatch(token)
        normalized = self._validate_tag(tag)
        if (
            match is None
            or match.group(1).casefold() != normalized
            or normalized in _VOID_TAGS
            or not self._stack
            or self._stack[-1].tag != normalized
        ):
            self._input_failure()
        opened = self._stack.pop()
        self._drafts[opened.node_ordinal].end_scalar = end

    def handle_data(self, data: str) -> None:
        if not data:
            return
        start, end = self._consume_exact(data)
        if (
            data.strip()
            and self._stack
            and self._stack[-1].tag in _TABLE_TEXT_FORBIDDEN
        ):
            self._input_failure()
        if data.strip() and self._html_seen and not self._stack:
            self._input_failure()
        if not self._element_seen and data.strip():
            self._element_seen = True
        self._append_fragment(data, NativeTransformKind.IDENTITY, start, end)

    def handle_entityref(self, name: str) -> None:
        match = _ENTITY_RE.match(self._decoded.text, self._cursor)
        if match is None or match.group(1) != name:
            self._input_failure()
        token = match.group(0)
        start, end = self._consume_exact(token)
        value = unescape(token)
        if value == token or "\ufffd" in value or not value:
            self._input_failure()
        self._append_fragment(value, NativeTransformKind.SYNTAX_DECODE, start, end)

    def handle_charref(self, name: str) -> None:
        del name
        match = _CHARREF_RE.match(self._decoded.text, self._cursor)
        if match is None:
            self._input_failure()
        token = match.group(0)
        start, end = self._consume_exact(token)
        value = unescape(token)
        if "\ufffd" in value or not value or value == token:
            self._input_failure()
        self._append_fragment(value, NativeTransformKind.SYNTAX_DECODE, start, end)

    def document(self) -> NativeDocument:
        """Finalize the exact source tree after ``HTMLParser.close``."""
        if self._stack or self._cursor != len(self._decoded.text):
            self._input_failure()
        nodes: list[NativeNode] = []
        try:
            for draft in self._drafts:
                if draft.end_scalar is None:
                    self._input_failure()
                nodes.append(
                    NativeNode(
                        ordinal=draft.ordinal,
                        kind=draft.kind,
                        parent_ordinal=draft.parent_ordinal,
                        source_position=self._decoded.position(
                            draft.start_scalar,
                            draft.end_scalar,
                        ),
                        fragments=tuple(draft.fragments),
                        attributes=draft.attributes,
                    )
                )
            return NativeDocument(
                source_format=ParserFormat.HTML,
                source_encoding=self._decoded.encoding,
                source_bytes=len(self._decoded.source),
                source_sha256=hashlib.sha256(self._decoded.source).hexdigest(),
                decoded_scalars=self._decoded.decoded_scalars,
                nodes=tuple(nodes),
            )
        except NativeParseFailure:
            raise
        except ValueError as error:
            raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH) from error

    def _start_element(
        self,
        tag: str,
        raw_attributes: list[tuple[str, str | None]],
        start: int,
        end: int,
        *,
        self_closing: bool,
    ) -> None:
        normalized = self._validate_tag(tag)
        if normalized in _FORBIDDEN_TAGS or normalized in _XINCLUDE_TAGS:
            self._input_failure()
        parsed_attributes = self._validate_attributes(raw_attributes)
        self._validate_nesting(normalized)
        self._element_seen = True
        self._track_document_elements(normalized)
        kind = _node_kind(normalized, self._stack)
        ordinal = len(self._drafts)
        if ordinal >= self._limits.nodes:
            self._limit_failure()
        parent = self._stack[-1].node_ordinal if self._stack else 0
        node_attributes = _native_attributes(normalized, kind, parsed_attributes)
        draft = _NodeDraft(
            ordinal=ordinal,
            kind=kind,
            parent_ordinal=parent,
            start_scalar=start,
            end_scalar=end if normalized in _VOID_TAGS or self_closing else None,
            attributes=node_attributes,
        )
        self._drafts.append(draft)
        if normalized == "br":
            self._append_fragment(
                "\n",
                NativeTransformKind.SYNTAX_DECODE,
                start,
                end,
                owner_ordinal=ordinal,
            )
        if normalized not in _VOID_TAGS and not self_closing:
            if len(self._stack) + 1 > self._limits.nesting_depth:
                self._limit_failure()
            self._stack.append(_OpenElement(normalized, ordinal))

    def _validate_tag(self, tag: str) -> str:
        normalized = tag.casefold()
        if not _TAG_RE.fullmatch(normalized):
            self._input_failure()
        return normalized

    def _validate_attributes(
        self,
        raw_attributes: list[tuple[str, str | None]],
    ) -> dict[str, str | None]:
        self._attribute_count += len(raw_attributes)
        if self._attribute_count > self._limits.attributes:
            self._limit_failure()
        result: dict[str, str | None] = {}
        for raw_name, value in raw_attributes:
            name = raw_name.casefold()
            if not _ATTRIBUTE_RE.fullmatch(name) or name in result:
                self._input_failure()
            if name.startswith("on") or name == "srcdoc":
                self._input_failure()
            if value is not None:
                if "\x00" in value:
                    self._input_failure()
                compact = "".join(
                    character
                    for character in value.casefold()
                    if character not in "\t\n\r\f "
                )
                if (name in _REFERENCE_ATTRIBUTES or name == "style") and (
                    "javascript:" in compact or "vbscript:" in compact
                ):
                    self._input_failure()
            result[name] = value
        return result

    def _validate_nesting(self, tag: str) -> None:
        parent = self._stack[-1].tag if self._stack else None
        ancestors = {item.tag for item in self._stack}
        if tag == "html":
            if parent is not None or self._html_seen or self._element_seen:
                self._input_failure()
            return
        if parent is None and self._html_seen:
            self._input_failure()
        if tag in {"head", "body"} and parent != "html":
            self._input_failure()
        if parent == "html" and tag not in {"head", "body"}:
            self._input_failure()
        if parent == "head" and tag not in _HEAD_ALLOWED:
            self._input_failure()
        if tag == "li" and parent not in {"ol", "ul"}:
            self._input_failure()
        if tag == "tr" and parent not in {"table", *_TABLE_SECTIONS}:
            self._input_failure()
        if tag in {"td", "th"} and parent != "tr":
            self._input_failure()
        if tag in _TABLE_SECTIONS | {"caption", "colgroup"} and parent != "table":
            self._input_failure()
        if tag == "col" and parent != "colgroup":
            self._input_failure()
        if parent == "table" and tag not in {
            "caption",
            "colgroup",
            "tbody",
            "tfoot",
            "thead",
            "tr",
        }:
            self._input_failure()
        if parent in _TABLE_SECTIONS and tag != "tr":
            self._input_failure()
        if parent == "colgroup" and tag != "col":
            self._input_failure()
        if parent in {"ol", "ul"} and tag != "li":
            self._input_failure()
        if parent == "tr" and tag not in {"td", "th"}:
            self._input_failure()
        if parent == "p" and tag in _BLOCK_TAGS:
            self._input_failure()
        if tag in _HEADING_TAGS and ancestors & _HEADING_TAGS:
            self._input_failure()
        if tag == "a" and "a" in ancestors:
            self._input_failure()

    def _track_document_elements(self, tag: str) -> None:
        if tag == "html":
            self._html_seen = True
        elif tag == "head":
            if self._head_seen or self._body_seen:
                self._input_failure()
            self._head_seen = True
        elif tag == "body":
            if self._body_seen:
                self._input_failure()
            self._body_seen = True

    def _append_fragment(
        self,
        text: str,
        transform: NativeTransformKind,
        start: int,
        end: int,
        *,
        owner_ordinal: int | None = None,
    ) -> None:
        self._fragment_count += 1
        self._emitted_text_bytes += len(text.encode("utf-8"))
        if (
            self._fragment_count > self._limits.fragments
            or self._emitted_text_bytes > self._limits.text_bytes
        ):
            self._limit_failure()
        owner = (
            owner_ordinal
            if owner_ordinal is not None
            else self._stack[-1].node_ordinal
            if self._stack
            else 0
        )
        draft = self._drafts[owner]
        draft.fragments.append(
            NativeFragment(
                ordinal=len(draft.fragments),
                text=text,
                transform=transform,
                source_position=self._decoded.position(start, end),
            )
        )

    def _consume_exact(self, token: str) -> tuple[int, int]:
        start = self._cursor
        if not self._decoded.text.startswith(token, start):
            self._input_failure()
        self._cursor += len(token)
        return start, self._cursor

    def _consume_markup(self) -> tuple[str, int, int]:
        start = self._cursor
        quote: str | None = None
        index = start
        while index < len(self._decoded.text):
            character = self._decoded.text[index]
            if quote is not None:
                if character == quote:
                    quote = None
            elif character in {'"', "'"}:
                quote = character
            elif character == ">":
                end = index + 1
                token = self._decoded.text[start:end]
                self._cursor = end
                return token, start, end
            index += 1
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    @staticmethod
    def _input_failure() -> Never:
        raise NativeParseFailure(StableErrorCode.INPUT_INVALID)

    @staticmethod
    def _limit_failure() -> Never:
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)


def _node_kind(tag: str, stack: list[_OpenElement]) -> NativeNodeKind:
    if tag == "code" and any(item.tag == "pre" for item in stack):
        return NativeNodeKind.RAW_HTML
    return _NODE_KIND_BY_TAG.get(tag, NativeNodeKind.RAW_HTML)


def _native_attributes(
    tag: str,
    kind: NativeNodeKind,
    source: dict[str, str | None],
) -> tuple[NativeAttribute, ...]:
    values: dict[str, str | int | bool | None] = {}
    if kind is NativeNodeKind.HEADING:
        values["headingLevel"] = int(tag[1])
    elif kind is NativeNodeKind.LIST:
        values["ordered"] = tag == "ol"
    elif kind is NativeNodeKind.TABLE_CELL:
        values["header"] = tag == "th"
    elif kind in {NativeNodeKind.RAW_HTML, NativeNodeKind.ASSET_REF}:
        values["tag"] = tag
    external = next(
        (
            source[name]
            for name in sorted(_REFERENCE_ATTRIBUTES)
            if source.get(name) is not None
        ),
        None,
    )
    if external is not None:
        values["externalRef"] = external
    if kind is NativeNodeKind.ASSET_REF and source.get("alt") is not None:
        values["alt"] = source["alt"]
    return attributes(**values)
