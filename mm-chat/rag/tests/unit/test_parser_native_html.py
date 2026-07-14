"""C1.3 deterministic HTML Native Parser and exact-locator gates."""

from __future__ import annotations

import socket
from dataclasses import replace
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.html import parse_html
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeTransformKind,
)

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_LIMITS = NativeParserLimits()


def _parse(
    source: bytes,
    *,
    limits: NativeParserLimits = _LIMITS,
) -> NativeDocument:
    return parse_html(decode_source(source), limits)


def _attributes(node: NativeNode) -> dict[str, object]:
    return {item.name: item.value for item in node.attributes}


def test_representative_html_preserves_structure_and_exact_source_spans() -> None:
    source = (_CORPUS / "golden" / "html" / "representative.html").read_bytes()

    document = _parse(source)
    kinds = [node.kind for node in document.nodes]

    assert document.source_format is ParserFormat.HTML
    assert document.nodes[0].source_position.raw_byte_end == len(source)
    assert NativeNodeKind.HEADING in kinds
    assert kinds.count(NativeNodeKind.LIST_ITEM) == 2
    assert kinds.count(NativeNodeKind.TABLE_ROW) == 2
    assert kinds.count(NativeNodeKind.TABLE_CELL) == 4
    assert NativeNodeKind.CODE in kinds
    heading = next(
        node for node in document.nodes if node.kind is NativeNodeKind.HEADING
    )
    assert _attributes(heading) == {"headingLevel": 1}
    assert [fragment.text for fragment in heading.fragments] == ["Heading"]
    for node in document.nodes:
        for fragment in node.fragments:
            position = fragment.source_position
            if fragment.transform is NativeTransformKind.IDENTITY:
                raw = source[position.raw_byte_start : position.raw_byte_end]
                assert raw.decode("utf-8") == fragment.text


def test_entities_charrefs_multibyte_and_line_positions_are_exact() -> None:
    source = ("<!doctype html>\r\n<p>café &amp; &#x4E2D;&#25991;<br>尾</p>").encode()

    document = _parse(source)
    paragraph = next(
        node for node in document.nodes if node.kind is NativeNodeKind.PARAGRAPH
    )
    transformed = [
        fragment
        for fragment in paragraph.fragments
        if fragment.transform is NativeTransformKind.SYNTAX_DECODE
    ]

    assert [fragment.text for fragment in transformed] == ["&", "中", "文"]
    raw_tokens = []
    for fragment in transformed:
        position = fragment.source_position
        raw_tokens.append(source[position.raw_byte_start : position.raw_byte_end])
    assert raw_tokens == [b"&amp;", b"&#x4E2D;", b"&#25991;"]
    assert all(fragment.source_position.start_line == 1 for fragment in transformed)
    line_break = next(
        node for node in document.nodes if node.kind is NativeNodeKind.LINE_BREAK
    )
    assert line_break.fragments[0].text == "\n"
    position = line_break.source_position
    assert source[position.raw_byte_start : position.raw_byte_end] == b"<br>"


def test_external_reference_is_recorded_without_any_network_operation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = (_CORPUS / "adversarial" / "xml" / "external-resource.html").read_bytes()

    def fail_socket(*_args: object, **_kwargs: object) -> socket.socket:
        raise AssertionError("HTML parsing attempted a network socket")

    monkeypatch.setattr(socket, "socket", fail_socket)
    document = _parse(source)
    asset = next(
        node for node in document.nodes if node.kind is NativeNodeKind.ASSET_REF
    )

    assert _attributes(asset) == {
        "externalRef": "https://example.invalid/never-fetch",
        "tag": "img",
    }
    assert asset.fragments == ()


@pytest.mark.parametrize(
    "source",
    [
        b"<!doctype html><html><body><script>x</script></body></html>",
        b"<!doctype html><html><body onload='x'>x</body></html>",
        b"<a href='java&#x73;cript:alert(1)'>x</a>",
        b"<!DOCTYPE html [<!ENTITY x 'y'>]><p>&x;</p>",
        b"<xi:include href='file:///etc/passwd'></xi:include>",
        b"<p><div>x</div></p>",
        b"<ul><p>x</p></ul>",
        b"<table>visible text</table>",
        b"<table><p>x</p></table>",
        b"<html><p>x</p></html>",
        b"<html></html><p>x</p>",
        b"<p>unterminated",
        b"<p>x</div>",
        b"<p>&unknown;</p>",
        b"<p>&#0;</p>",
    ],
)
def test_active_ambiguous_and_illegally_nested_html_fails_closed(
    source: bytes,
) -> None:
    with pytest.raises(NativeParseFailure) as raised:
        _parse(source)

    assert raised.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    ("source", "limits"),
    [
        (b"<p>x</p>", replace(_LIMITS, nodes=1)),
        (b"<div><span>x</span></div>", replace(_LIMITS, nesting_depth=1)),
        (b"<p id='x'>x</p>", replace(_LIMITS, attributes=0)),
        (b"<p>x</p>", replace(_LIMITS, fragments=0)),
        (b"<p>xy</p>", replace(_LIMITS, text_bytes=1)),
        (b"<p>x</p>\n", replace(_LIMITS, lines=1)),
    ],
)
def test_html_structure_and_text_limits_are_stable(
    source: bytes,
    limits: NativeParserLimits,
) -> None:
    with pytest.raises(NativeParseFailure) as raised:
        _parse(source, limits=limits)

    assert raised.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_raw_html_quote_code_and_semantic_attributes_are_closed() -> None:
    source = (
        b"<main><blockquote><p>x</p></blockquote><ol><li>y</li></ol>"
        b"<code>z</code><a href='https://example.invalid'>link</a></main>"
    )

    document = _parse(source)
    raw = [node for node in document.nodes if node.kind is NativeNodeKind.RAW_HTML]
    quote = next(node for node in document.nodes if node.kind is NativeNodeKind.QUOTE)
    ordered = next(node for node in document.nodes if node.kind is NativeNodeKind.LIST)
    code = next(node for node in document.nodes if node.kind is NativeNodeKind.CODE)

    assert _attributes(raw[0]) == {"tag": "main"}
    assert _attributes(raw[-1]) == {
        "externalRef": "https://example.invalid",
        "tag": "a",
    }
    assert quote.parent_ordinal == raw[0].ordinal
    assert _attributes(ordered) == {"ordered": True}
    assert code.fragments[0].text == "z"


def test_html_artifact_bytes_are_deterministic() -> None:
    source = b"<!doctype html><p>A &amp; B</p>"

    first = _parse(source)
    second = _parse(source)

    assert first.canonical_bytes == second.canonical_bytes
    assert first.artifact_sha256 == second.artifact_sha256
