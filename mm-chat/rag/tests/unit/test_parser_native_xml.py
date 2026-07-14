"""Hardened source-aware XML parsing gates for C1.3B OOXML Parts."""

from __future__ import annotations

from typing import Any, cast

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native import xml_source
from mm_chat_rag.offline_parser.native.decoding import decode_xml_source
from mm_chat_rag.offline_parser.native.model import (
    NativeParseFailure,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.native.xml_source import (
    ParsedXmlSource,
    XmlElement,
    expanded_name,
    parse_xml_source,
)


def _parse(source: bytes, **limit_values: int) -> ParsedXmlSource:
    return parse_xml_source(
        source,
        source_unit_ordinal=2,
        limits=NativeParserLimits(**limit_values),
    )


def _assert_failure(source: bytes, code: StableErrorCode) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source)
    assert observed.value.code is code


def test_xml_preserves_expanded_names_and_exact_element_positions() -> None:
    source = (
        b'<r xmlns="urn:r" xmlns:a="urn:a" a:id="7">a&amp;b\r\nc<x/>\xe5\xb0\xbe</r>'
    )
    document = _parse(source)
    root = document.root

    assert root.name == expanded_name("urn:r", "r")
    assert root.attribute(expanded_name("urn:a", "id")) == "7"
    assert root.source_position.source_unit_ordinal == 2
    assert root.source_position.raw_byte_start == 0
    assert root.source_position.raw_byte_end == len(source)
    assert source[
        root.start_tag_position.raw_byte_start : root.start_tag_position.raw_byte_end
    ].endswith(b">")
    assert root.end_tag_position is not None
    assert (
        source[
            root.end_tag_position.raw_byte_start : root.end_tag_position.raw_byte_end
        ]
        == b"</r>"
    )

    content = root.content
    assert [item.text for item in content if not isinstance(item, XmlElement)] == [
        "a",
        "&",
        "b",
        "\n",
        "c",
        "尾",
    ]
    transforms = [
        item.transform for item in content if not isinstance(item, XmlElement)
    ]
    assert transforms == [
        NativeTransformKind.IDENTITY,
        NativeTransformKind.SYNTAX_DECODE,
        NativeTransformKind.IDENTITY,
        NativeTransformKind.SYNTAX_DECODE,
        NativeTransformKind.IDENTITY,
        NativeTransformKind.IDENTITY,
    ]
    child = next(item for item in content if isinstance(item, XmlElement))
    assert child.name == expanded_name("urn:r", "x")
    assert child.end_tag_position is None
    assert (
        source[
            child.source_position.raw_byte_start : child.source_position.raw_byte_end
        ]
        == b"<x/>"
    )


def test_xml_utf8_bom_preserves_raw_offsets_and_decoded_metadata() -> None:
    source = b"\xef\xbb\xbf<r>\xe4\xb8\xad</r>"
    document = _parse(source)
    text = document.root.text_runs()[0]

    assert document.decoded.encoding == "utf-8-bom"
    assert document.root.source_position.raw_byte_start == 3
    assert text.text == "中"
    assert (text.source_position.start_line, text.source_position.start_column) == (
        0,
        3,
    )


@pytest.mark.parametrize(
    "source",
    [
        b'<?xml version="1.0"?><!DOCTYPE r><r/>',
        b'<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x "boom">]><r>&x;</r>',
        b'<?xml version="1.0"?><!DOCTYPE r SYSTEM "file:///etc/passwd"><r/>',
        b'<?xml version="1.0"?><?unsafe fetch?><r/>',
        (
            b'<r xmlns:any="http://www.w3.org/2001/XInclude">'
            b'<any:include href="file:///etc/passwd"/></r>'
        ),
    ],
)
def test_xml_rejects_dtd_entities_processing_instructions_and_xinclude(
    source: bytes,
) -> None:
    _assert_failure(source, StableErrorCode.INPUT_INVALID)


@pytest.mark.parametrize(
    "source",
    [
        "<r/>".encode("utf-16"),
        b'<?xml version="1.0" encoding="ISO-8859-1"?><r/>',
        b"<r>\xff</r>",
        b"<r>\x00</r>",
    ],
)
def test_xml_accepts_only_strict_utf8_or_utf8_bom(source: bytes) -> None:
    _assert_failure(source, StableErrorCode.INPUT_INVALID)


@pytest.mark.parametrize(
    ("source", "limits"),
    [
        (b"<r><a/></r>", {"xml_depth": 1}),
        (b"<r><a/></r>", {"xml_nodes": 1}),
        (b'<r a="1" b="2"/>', {"xml_attributes": 1}),
        (b"<r>xx</r>", {"xml_text_bytes": 1}),
    ],
)
def test_xml_resource_limits_fail_with_archive_limit(
    source: bytes,
    limits: dict[str, int],
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        _parse(source, **limits)
    assert observed.value.code is StableErrorCode.ARCHIVE_LIMIT_EXCEEDED


def test_xml_rejects_malformed_and_multiple_roots() -> None:
    _assert_failure(b"<r>", StableErrorCode.INPUT_INVALID)
    _assert_failure(b"<a/><b/>", StableErrorCode.INPUT_INVALID)


def test_explicit_empty_element_includes_its_end_tag() -> None:
    source = b"<r><x></x></r>"
    child = _parse(source).root.child_elements()[0]

    assert child.end_tag_position is not None
    assert (
        source[
            child.source_position.raw_byte_start : child.source_position.raw_byte_end
        ]
        == b"<x></x>"
    )


def test_comments_cdata_and_whitespace_outside_root_preserve_text_order() -> None:
    source = b" \n<r>a<!-- hidden -->b<![CDATA[c]]>d</r>\r\n"

    parsed = _parse(source)

    assert [item.text for item in parsed.root.text_runs()] == ["a", "b", "c", "d"]


def test_expanded_name_helpers_reject_malformed_clark_names() -> None:
    assert xml_source.split_expanded_name("plain") == ("", "plain")
    assert xml_source.split_expanded_name("{urn:test}name") == ("urn:test", "name")
    for value in ("{}name", "{urn:test}"):
        with pytest.raises(ValueError, match="invalid expanded XML name"):
            xml_source.split_expanded_name(value)
    for value in ("}name", "urn:test}"):
        with pytest.raises(NativeParseFailure) as observed:
            xml_source._expanded_name_from_expat(value)
        assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_xml_preflight_and_markup_scanner_defensive_branches() -> None:
    with pytest.raises(TypeError, match="XML source must be bytes"):
        xml_source._preflight_utf8(cast("Any", "<r/>"))
    with pytest.raises(NativeParseFailure) as observed:
        xml_source._scan_markup_end(b"not markup", 0)
    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED
    assert xml_source._scan_markup_end(b'<r a=">">', 0) == 9
    with pytest.raises(NativeParseFailure) as observed:
        xml_source._scan_markup_end(b'<r a="unterminated', 0)
    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_expat_security_handlers_fail_closed_when_invoked() -> None:
    decoded = decode_xml_source(
        b"<r/>",
        source_unit_ordinal=2,
        limits=NativeParserLimits(),
    )
    builder = xml_source._ExpatBuilder(decoded, NativeParserLimits())
    handlers = (
        lambda: builder._forbidden_processing_instruction("target", "data"),
        lambda: builder._forbidden_doctype("r", None, None, False),
        lambda: builder._forbidden_entity("x", 0, "v", None, None, None, None),
        lambda: builder._forbidden_unparsed_entity("x", None, "s", None, "n"),
        lambda: builder._forbidden_notation("x", None, None, None),
        lambda: builder._forbidden_external_entity("x", None, None, None),
        lambda: builder._forbidden_skipped_entity("x", 0),
    )

    for invoke in handlers:
        with pytest.raises(NativeParseFailure) as observed:
            invoke()
        assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_locator_projection_rejects_a_non_boundary_raw_offset() -> None:
    decoded = decode_xml_source(
        "<r>中</r>".encode(),
        source_unit_ordinal=2,
        limits=NativeParserLimits(),
    )
    builder = xml_source._ExpatBuilder(decoded, NativeParserLimits())

    with pytest.raises(NativeParseFailure) as observed:
        builder._position(4, 5)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED
