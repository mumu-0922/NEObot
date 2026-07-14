from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import DecodedSource, decode_source
from mm_chat_rag.offline_parser.native.markdown import (
    _protected_inline_ranges,
    parse_markdown,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeNodeKind,
    NativeParseFailure,
    NativeTransformKind,
)

_CORPUS_ROOT = Path(__file__).parents[1] / "fixtures" / "parser_corpus"


def _parse(
    source: bytes,
    limits: NativeParserLimits | None = None,
) -> tuple[DecodedSource, NativeDocument]:
    decoded = decode_source(source)
    return decoded, parse_markdown(decoded, limits or NativeParserLimits())


def test_representative_markdown_preserves_structure_and_exact_positions() -> None:
    source = (_CORPUS_ROOT / "golden" / "markdown" / "representative.md").read_bytes()
    decoded, document = _parse(source)

    assert [node.kind for node in document.nodes] == [
        NativeNodeKind.DOCUMENT,
        NativeNodeKind.HEADING,
        NativeNodeKind.LIST,
        NativeNodeKind.LIST_ITEM,
        NativeNodeKind.PARAGRAPH,
        NativeNodeKind.LIST_ITEM,
        NativeNodeKind.PARAGRAPH,
        NativeNodeKind.TABLE,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_ROW,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.TABLE_CELL,
        NativeNodeKind.CODE,
        NativeNodeKind.PARAGRAPH,
        NativeNodeKind.RAW_HTML,
        NativeNodeKind.RAW_HTML,
    ]
    assert [
        fragment.text
        for node in document.nodes
        if node.kind is NativeNodeKind.TABLE_CELL
        for fragment in node.fragments
    ] == ["key", "value", "café", "中文"]
    assert [
        fragment.text
        for node in document.nodes
        if node.kind is NativeNodeKind.RAW_HTML
        for fragment in node.fragments
    ] == ['<span data-kind="raw">', "</span>"]

    for node in document.nodes:
        assert node.parent_ordinal is None or node.parent_ordinal < node.ordinal
        position = node.source_position
        assert (
            source[position.raw_byte_start : position.raw_byte_end].decode("utf-8")
            == decoded.text[position.decoded_scalar_start : position.decoded_scalar_end]
        )
        for fragment in node.fragments:
            assert fragment.transform is NativeTransformKind.SYNTAX_DECODE
            fragment_position = fragment.source_position
            fragment_start = fragment_position.decoded_scalar_start
            fragment_end = fragment_position.decoded_scalar_end
            assert fragment.text == decoded.text[fragment_start:fragment_end]


def test_escape_link_entity_and_multibyte_source_remain_unmodified() -> None:
    source = (
        "## Héading\n\n"
        "Escaped \\*literal\\*, [label](https://example.invalid), &amp;.\n"
    ).encode()
    decoded, document = _parse(source)
    heading = next(
        node for node in document.nodes if node.kind is NativeNodeKind.HEADING
    )
    paragraph = next(
        node for node in document.nodes if node.kind is NativeNodeKind.PARAGRAPH
    )

    assert heading.fragments[0].text == "## Héading\n"
    assert paragraph.fragments[0].text == (
        "Escaped \\*literal\\*, [label](https://example.invalid), &amp;.\n"
    )
    assert paragraph.source_position.raw_byte_start == len("## Héading\n\n".encode())
    assert paragraph.source_position.raw_byte_end == len(source)
    assert paragraph.source_position.end_line == 3
    assert paragraph.source_position.end_column == 0
    assert (
        document.canonical_bytes
        == parse_markdown(decoded, NativeParserLimits()).canonical_bytes
    )


def test_table_cells_use_exact_raw_spans_for_escape_and_code_pipe() -> None:
    source = b"| left | right |\n| --- | --- |\n| x\\|y | `a|b` |\n"
    _decoded, document = _parse(source)
    cells = [node for node in document.nodes if node.kind is NativeNodeKind.TABLE_CELL]

    assert [node.fragments[0].text for node in cells] == [
        "left",
        "right",
        "x\\|y",
        "`a|b`",
    ]
    for node in cells:
        position = node.source_position
        assert source[position.raw_byte_start : position.raw_byte_end].decode() == (
            node.fragments[0].text
        )


def test_safe_external_html_is_preserved_without_becoming_a_fetch() -> None:
    source = (
        b'<img src="https://example.invalid/never-fetch">\n\n'
        b'<span data-kind="safe">text</span>\n'
    )
    _decoded, document = _parse(source)
    raw_nodes = [
        node for node in document.nodes if node.kind is NativeNodeKind.RAW_HTML
    ]

    assert [node.fragments[0].text for node in raw_nodes] == [
        '<img src="https://example.invalid/never-fetch">\n',
        '<span data-kind="safe">',
        "</span>",
    ]


@pytest.mark.parametrize(
    "source",
    [
        b"<script>never()</script>\n",
        b'<iframe src="https://example.invalid/never-fetch"></iframe>\n',
        b'<object data="file:///etc/passwd"></object>\n',
        b'<embed src="https://example.invalid/never-fetch">\n',
        b'<div OnLoad="never()">text</div>\n',
        b'<a href="JaVaScRiPt:never()">text</a>\n',
        b'<a href="vBsCrIpT:never()">text</a>\n',
        b'<img srcdoc="&lt;script&gt;never()&lt;/script&gt;">\n',
        b'<div id="one" id="two">duplicate</div>\n',
        b"<!DOCTYPE html>\n",
        b'<!ENTITY xxe SYSTEM "file:///etc/passwd">\n',
        b'<xi:include href="file:///etc/passwd"/>\n',
    ],
)
def test_active_raw_html_fails_closed(source: bytes) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_html_looking_text_inside_code_is_not_executed_or_reclassified() -> None:
    _decoded, document = _parse(
        b"```html\n<script>literal only</script>\n```\n\n"
        b"`<script>also literal</script>`\n"
    )

    assert [node.kind for node in document.nodes].count(NativeNodeKind.CODE) == 1
    assert NativeNodeKind.RAW_HTML not in {node.kind for node in document.nodes}


def test_inline_html_locator_skips_identical_markup_inside_code_span() -> None:
    source = b"`<b>` <b>x</b>\n"
    _decoded, document = _parse(source)
    raw_nodes = [
        node for node in document.nodes if node.kind is NativeNodeKind.RAW_HTML
    ]

    assert [node.fragments[0].text for node in raw_nodes] == ["<b>", "</b>"]
    assert raw_nodes[0].source_position.raw_byte_start == 6
    assert (
        source[
            raw_nodes[0].source_position.raw_byte_start : raw_nodes[
                0
            ].source_position.raw_byte_end
        ]
        == b"<b>"
    )


def test_unmatched_backtick_run_is_scanned_once_not_quadratically() -> None:
    class CountingText(str):
        __slots__ = ()

        find_calls = 0

        def find(
            self,
            sub: str,
            start: int = 0,
            end: int | None = None,
        ) -> int:
            type(self).find_calls += 1
            return super().find(sub, start, len(self) if end is None else end)

    content = CountingText("`" * 10_000)

    assert _protected_inline_ranges(content) == ()
    assert CountingText.find_calls == 1


def test_markdown_artifact_is_deterministic_across_repeated_parses() -> None:
    source = b"# title\n\n- one\n- two\n\n> quote\n\n---\n"
    outputs = {_parse(source)[1].canonical_bytes for _unused in range(10)}

    assert len(outputs) == 1


@pytest.mark.parametrize(
    ("source", "limits"),
    [
        (b"", replace(NativeParserLimits(), nodes=0)),
        (b"paragraph\n", replace(NativeParserLimits(), nodes=1)),
        (b"paragraph\n", replace(NativeParserLimits(), fragments=0)),
        (b"> > nested\n", replace(NativeParserLimits(), nesting_depth=1)),
        (b"# heading\n", replace(NativeParserLimits(), attributes=0)),
        (b"one\ntwo\n", replace(NativeParserLimits(), lines=1)),
        (b"long\n", replace(NativeParserLimits(), text_bytes=1)),
        (b"paragraph\n", replace(NativeParserLimits(), artifact_bytes=1)),
    ],
)
def test_native_structure_and_artifact_limits_fail_stably(
    source: bytes,
    limits: NativeParserLimits,
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source, limits)

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_empty_markdown_is_a_document_only_artifact() -> None:
    _decoded, document = _parse(b"")

    assert len(document.nodes) == 1
    assert document.nodes[0].kind is NativeNodeKind.DOCUMENT
    assert document.nodes[0].source_position.raw_byte_start == 0
    assert document.nodes[0].source_position.raw_byte_end == 0
