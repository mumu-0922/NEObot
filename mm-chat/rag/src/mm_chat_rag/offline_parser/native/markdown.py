"""C1.3 deterministic, source-preserving Markdown Native Parser."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from html.parser import HTMLParser
from itertools import pairwise
from typing import Final

from markdown_it import MarkdownIt
from markdown_it.token import Token

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
    NativeSourcePosition,
    NativeTransformKind,
    attributes,
)

_OPEN_KINDS: Final = {
    "heading_open": NativeNodeKind.HEADING,
    "paragraph_open": NativeNodeKind.PARAGRAPH,
    "bullet_list_open": NativeNodeKind.LIST,
    "ordered_list_open": NativeNodeKind.LIST,
    "list_item_open": NativeNodeKind.LIST_ITEM,
    "blockquote_open": NativeNodeKind.QUOTE,
    "table_open": NativeNodeKind.TABLE,
    "tr_open": NativeNodeKind.TABLE_ROW,
    "th_open": NativeNodeKind.TABLE_CELL,
    "td_open": NativeNodeKind.TABLE_CELL,
}
_CLOSE_TO_OPEN: Final = {
    "heading_close": "heading_open",
    "paragraph_close": "paragraph_open",
    "bullet_list_close": "bullet_list_open",
    "ordered_list_close": "ordered_list_open",
    "list_item_close": "list_item_open",
    "blockquote_close": "blockquote_open",
    "table_close": "table_open",
    "tr_close": "tr_open",
    "th_close": "th_open",
    "td_close": "td_open",
}
_LEAF_KINDS: Final = {
    "fence": NativeNodeKind.CODE,
    "code_block": NativeNodeKind.CODE,
    "html_block": NativeNodeKind.RAW_HTML,
    "hr": NativeNodeKind.THEMATIC_BREAK,
}
_FRAGMENT_OPEN_TYPES: Final = frozenset({"heading_open", "paragraph_open"})
_TOKEN_MAP_LENGTH: Final = 2
_DANGEROUS_RAW_HTML: Final = (
    "<!doctype",
    "<!entity",
    "<script",
    "</script",
    "<xi:include",
    "<xinclude:include",
    "javascript:",
    "vbscript:",
)
_FORBIDDEN_RAW_HTML_TAGS: Final = frozenset(
    {"applet", "embed", "iframe", "object", "script", "xi:include", "xinclude:include"}
)
_REFERENCE_RAW_HTML_ATTRIBUTES: Final = frozenset(
    {"action", "cite", "data", "formaction", "href", "poster", "src", "srcset"}
)
_CONTAINER_PREFIX: Final = re.compile(
    r"^(?:[ \t]{0,3}>[ \t]?)*(?:[ \t]{0,3}(?:[-+*]|\d{1,9}[.)])[ \t]+)?"
)


@dataclass(frozen=True, slots=True)
class _OpenNode:
    token_type: str
    ordinal: int


class _RawHtmlPolicy(HTMLParser):
    """Validate raw Markdown HTML without resolving or fetching anything."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=False)

    def handle_starttag(
        self,
        tag: str,
        attrs: list[tuple[str, str | None]],
    ) -> None:
        normalized_tag = tag.casefold()
        if normalized_tag in _FORBIDDEN_RAW_HTML_TAGS:
            raise ValueError("active raw HTML tag")
        names: set[str] = set()
        for name, value in attrs:
            normalized_name = name.casefold()
            if (
                normalized_name in names
                or normalized_name.startswith("on")
                or normalized_name == "srcdoc"
            ):
                raise ValueError("active raw HTML attribute")
            names.add(normalized_name)
            if value is None:
                continue
            compact = "".join(
                character
                for character in value.casefold()
                if character not in "\t\n\r\f "
            )
            if (
                normalized_name in _REFERENCE_RAW_HTML_ATTRIBUTES
                or normalized_name == "style"
            ) and ("javascript:" in compact or "vbscript:" in compact):
                raise ValueError("active raw HTML attribute")

    def handle_startendtag(
        self,
        tag: str,
        attrs: list[tuple[str, str | None]],
    ) -> None:
        self.handle_starttag(tag, attrs)

    def handle_decl(self, _decl: str) -> None:
        raise ValueError("raw HTML declarations are forbidden")

    def unknown_decl(self, _data: str) -> None:
        raise ValueError("raw HTML declarations are forbidden")


class _MarkdownBuilder:
    def __init__(
        self,
        decoded: DecodedSource,
        limits: NativeParserLimits,
        tokens: list[Token],
    ) -> None:
        if limits.nodes < 1:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        self.decoded = decoded
        self.limits = limits
        self.tokens = tokens
        self.nodes: list[NativeNode] = [
            NativeNode(
                ordinal=0,
                kind=NativeNodeKind.DOCUMENT,
                parent_ordinal=None,
                source_position=decoded.document_position(),
            )
        ]
        self.stack: list[_OpenNode] = []
        self.fragment_count = 0
        self.attribute_count = 0
        self.row_cell_positions: list[NativeSourcePosition] = []
        self.row_cell_cursor = 0

    def build(self) -> NativeDocument:
        for index, token in enumerate(self.tokens):
            if token.type in _OPEN_KINDS:
                self._open(index, token)
            elif token.type in _CLOSE_TO_OPEN:
                self._close(token)
            elif token.type in _LEAF_KINDS:
                self._leaf(token)
            elif token.type == "inline":
                self._inline_html(token)
        if self.stack:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        document = NativeDocument(
            source_format=ParserFormat.MARKDOWN,
            source_encoding=self.decoded.encoding,
            source_bytes=len(self.decoded.source),
            source_sha256=hashlib.sha256(self.decoded.source).hexdigest(),
            decoded_scalars=self.decoded.decoded_scalars,
            nodes=tuple(self.nodes),
        )
        if len(document.canonical_bytes) > self.limits.artifact_bytes:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        return document

    def _open(self, index: int, token: Token) -> None:
        if len(self.stack) + 1 > self.limits.nesting_depth:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        if token.type == "tr_open":
            self._prepare_row(index, token)
        position = self._open_position(token)
        node_attributes = self._attributes(token)
        fragments = (
            self._fragment(position)
            if token.type in _FRAGMENT_OPEN_TYPES
            or token.type in {"th_open", "td_open"}
            else ()
        )
        ordinal = self._append_node(
            kind=_OPEN_KINDS[token.type],
            position=position,
            fragments=fragments,
            node_attributes=node_attributes,
        )
        self.stack.append(_OpenNode(token.type, ordinal))

    def _close(self, token: Token) -> None:
        expected = _CLOSE_TO_OPEN[token.type]
        if not self.stack or self.stack[-1].token_type != expected:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        self.stack.pop()
        if token.type == "tr_close":
            self.row_cell_positions = []
            self.row_cell_cursor = 0

    def _leaf(self, token: Token) -> None:
        position = self._mapped_position(token)
        if token.type == "html_block":
            self._validate_raw_html(
                self.decoded.text[
                    position.decoded_scalar_start : position.decoded_scalar_end
                ]
            )
        self._append_node(
            kind=_LEAF_KINDS[token.type],
            position=position,
            fragments=self._fragment(position),
            node_attributes=self._attributes(token),
        )

    def _inline_html(self, token: Token) -> None:
        if not token.children:
            return
        parent_position = self._mapped_position(token)
        parent_start = parent_position.decoded_scalar_start
        parent_text = self.decoded.text[
            parent_start : parent_position.decoded_scalar_end
        ]
        protected = _protected_inline_ranges(parent_text)
        cursor = 0
        for child in token.children:
            if child.type != "html_inline":
                if (
                    child.type == "text"
                    and child.content.casefold()
                    .lstrip()
                    .startswith(("<xi:include", "<xinclude:include"))
                ):
                    raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
                continue
            start = self.decoded.text.find(
                child.content,
                parent_start + cursor,
                parent_position.decoded_scalar_end,
            )
            while start >= 0 and _range_is_protected(
                start - parent_start,
                start - parent_start + len(child.content),
                protected,
            ):
                start = self.decoded.text.find(
                    child.content,
                    start + 1,
                    parent_position.decoded_scalar_end,
                )
            if start < 0:
                raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
            end = start + len(child.content)
            position = self.decoded.position(start, end)
            self._validate_raw_html(child.content)
            self._append_node(
                kind=NativeNodeKind.RAW_HTML,
                position=position,
                fragments=self._fragment(position),
                node_attributes=attributes(block=False),
            )
            cursor = end - parent_start

    def _open_position(self, token: Token) -> NativeSourcePosition:
        if token.type in {"th_open", "td_open"}:
            if self.row_cell_cursor >= len(self.row_cell_positions):
                raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
            position = self.row_cell_positions[self.row_cell_cursor]
            self.row_cell_cursor += 1
            return position
        return self._mapped_position(token)

    def _mapped_position(self, token: Token) -> NativeSourcePosition:
        if token.map is None or len(token.map) != _TOKEN_MAP_LENGTH:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        start_line, end_line = token.map
        try:
            return self.decoded.line_span(start_line, end_line)
        except ValueError as error:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error

    def _prepare_row(self, index: int, token: Token) -> None:
        row_position = self._mapped_position(token)
        expected_cells = 0
        cursor = index + 1
        while cursor < len(self.tokens) and self.tokens[cursor].type != "tr_close":
            if self.tokens[cursor].type in {"th_open", "td_open"}:
                expected_cells += 1
            cursor += 1
        if cursor >= len(self.tokens) or expected_cells == 0:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        self.row_cell_positions = _table_cell_positions(
            self.decoded,
            row_position,
            expected_cells,
        )
        self.row_cell_cursor = 0

    def _attributes(self, token: Token) -> tuple[NativeAttribute, ...]:
        result: tuple[NativeAttribute, ...] = ()
        if token.type == "heading_open":
            result = attributes(level=int(token.tag.removeprefix("h")))
        elif token.type == "bullet_list_open":
            result = attributes(ordered=False)
        elif token.type == "ordered_list_open":
            start = token.attrGet("start")
            result = attributes(ordered=True, start=int(start) if start else 1)
        elif token.type == "th_open":
            result = attributes(header=True)
        elif token.type == "td_open":
            result = attributes(header=False)
        elif token.type in {"fence", "code_block"}:
            result = attributes(
                fenced=token.type == "fence",
                info=token.info.strip(),
            )
        elif token.type == "html_block":
            result = attributes(block=True)
        return result

    def _fragment(
        self,
        position: NativeSourcePosition,
    ) -> tuple[NativeFragment, ...]:
        text = self.decoded.text[
            position.decoded_scalar_start : position.decoded_scalar_end
        ]
        if not text:
            return ()
        if self.fragment_count >= self.limits.fragments:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        self.fragment_count += 1
        return (
            NativeFragment(
                ordinal=0,
                text=text,
                transform=NativeTransformKind.SYNTAX_DECODE,
                source_position=position,
            ),
        )

    def _append_node(
        self,
        *,
        kind: NativeNodeKind,
        position: NativeSourcePosition,
        fragments: tuple[NativeFragment, ...],
        node_attributes: tuple[NativeAttribute, ...],
    ) -> int:
        if len(self.nodes) >= self.limits.nodes:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        if self.attribute_count + len(node_attributes) > self.limits.attributes:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
        self.attribute_count += len(node_attributes)
        ordinal = len(self.nodes)
        self.nodes.append(
            NativeNode(
                ordinal=ordinal,
                kind=kind,
                parent_ordinal=self.stack[-1].ordinal if self.stack else 0,
                source_position=position,
                fragments=fragments,
                attributes=node_attributes,
            )
        )
        return ordinal

    @staticmethod
    def _validate_raw_html(content: str) -> None:
        lowered = content.casefold()
        if any(marker in lowered for marker in _DANGEROUS_RAW_HTML):
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID)
        parser = _RawHtmlPolicy()
        try:
            parser.feed(content)
            parser.close()
        except ValueError as error:
            raise NativeParseFailure(StableErrorCode.INPUT_INVALID) from error


def parse_markdown(
    decoded: DecodedSource,
    limits: NativeParserLimits,
) -> NativeDocument:
    """Parse CommonMark plus frozen table support into a native artifact."""
    if not isinstance(decoded, DecodedSource):
        raise TypeError("decoded Markdown source has an invalid type")
    if not isinstance(limits, NativeParserLimits):
        raise TypeError("Markdown parser limits have an invalid type")
    if (
        len(decoded.line_starts) > limits.lines
        or len(decoded.text.encode("utf-8")) > limits.text_bytes
    ):
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
    parser = MarkdownIt(
        "commonmark",
        {
            "html": True,
            "linkify": False,
            "maxNesting": limits.nesting_depth + 1,
            "typographer": False,
        },
    ).enable("table")
    tokens = parser.parse(decoded.text)
    return _MarkdownBuilder(decoded, limits, tokens).build()


def _protected_inline_ranges(content: str) -> tuple[tuple[int, int], ...]:
    """Return code, escape, and link-destination ranges that cannot be HTML."""
    ranges: list[tuple[int, int]] = []
    index = 0
    while index < len(content):
        if content[index] == "\\" and index + 1 < len(content):
            ranges.append((index, index + 2))
            index += 2
            continue
        if content[index] == "`":
            run_end = index + 1
            while run_end < len(content) and content[run_end] == "`":
                run_end += 1
            marker = content[index:run_end]
            close = content.find(marker, run_end)
            if close >= 0:
                ranges.append((index, close + len(marker)))
                index = close + len(marker)
                continue
            index = run_end
            continue
        if content.startswith("](", index):
            destination_end = _link_destination_end(content, index + 2)
            if destination_end is not None:
                ranges.append((index + 1, destination_end))
                index = destination_end
                continue
        index += 1
    return tuple(ranges)


def _link_destination_end(content: str, start: int) -> int | None:
    depth = 1
    index = start
    while index < len(content):
        if content[index] == "\\" and index + 1 < len(content):
            index += 2
            continue
        if content[index] == "(":
            depth += 1
        elif content[index] == ")":
            depth -= 1
            if depth == 0:
                return index + 1
        index += 1
    return None


def _range_is_protected(
    start: int,
    end: int,
    protected: tuple[tuple[int, int], ...],
) -> bool:
    return any(left < end and start < right for left, right in protected)


def _table_cell_positions(
    decoded: DecodedSource,
    row_position: NativeSourcePosition,
    expected_cells: int,
) -> list[NativeSourcePosition]:
    row_start = row_position.decoded_scalar_start
    row_end = row_position.decoded_scalar_end
    while row_end > row_start and decoded.text[row_end - 1] in "\r\n":
        row_end -= 1
    line = decoded.text[row_start:row_end]
    content_start = _container_content_start(line)
    delimiters = _table_delimiters(line, content_start)
    content_end = len(line.rstrip(" \t"))
    leading = content_start < len(line) and line[content_start] == "|"
    trailing = bool(delimiters) and delimiters[-1] == content_end - 1
    boundaries = [content_start, *delimiters, content_end]
    ranges = list(pairwise(boundaries))
    if leading:
        ranges = ranges[1:]
    if trailing:
        ranges = ranges[:-1]
    positions: list[NativeSourcePosition] = []
    for range_start, range_end in ranges[:expected_cells]:
        trimmed_start = range_start
        trimmed_end = range_end
        while trimmed_start < trimmed_end and line[trimmed_start] in " \t|":
            trimmed_start += 1
        while trimmed_end > trimmed_start and line[trimmed_end - 1] in " \t|":
            trimmed_end -= 1
        positions.append(
            decoded.position(row_start + trimmed_start, row_start + trimmed_end)
        )
    fill = row_start + content_end
    positions.extend(
        decoded.position(fill, fill)
        for _unused in range(expected_cells - len(positions))
    )
    return positions


def _container_content_start(line: str) -> int:
    match = _CONTAINER_PREFIX.match(line)
    return match.end() if match is not None else 0


def _table_delimiters(line: str, start: int) -> list[int]:
    delimiters: list[int] = []
    code_ticks = 0
    index = start
    while index < len(line):
        character = line[index]
        if character == "`":
            run_end = index + 1
            while run_end < len(line) and line[run_end] == "`":
                run_end += 1
            run = run_end - index
            if code_ticks == 0:
                code_ticks = run
            elif code_ticks == run:
                code_ticks = 0
            index = run_end
            continue
        if character == "|" and code_ticks == 0:
            backslashes = 0
            cursor = index - 1
            while cursor >= start and line[cursor] == "\\":
                backslashes += 1
                cursor -= 1
            if backslashes % 2 == 0:
                delimiters.append(index)
        index += 1
    return delimiters
