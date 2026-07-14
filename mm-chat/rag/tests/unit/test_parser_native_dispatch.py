"""Dispatch, artifact-limit, and child-side locator defense tests."""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native import dispatch
from mm_chat_rag.offline_parser.native.decoding import DecodedSource, decode_source
from mm_chat_rag.offline_parser.native.dispatch import (
    NativeParseOutcome,
    _enforce_artifact_limits,
    _parse_selected,
    _validate_artifact_positions,
    parse_native_source,
)
from mm_chat_rag.offline_parser.native.markdown import parse_markdown
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeParseFailure,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.native.txt import parse_txt
from mm_chat_rag.offline_parser.router import RouteDecision, route_source

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"


def _txt_document(source: bytes = b"abc") -> tuple[DecodedSource, NativeDocument]:
    decoded = decode_source(source)
    return decoded, parse_txt(decoded)


@pytest.mark.parametrize(
    ("parser_format", "has_artifact", "code"),
    [
        (None, False, None),
        (ParserFormat.TXT, False, None),
        (None, True, StableErrorCode.INPUT_INVALID),
        (ParserFormat.TXT, True, StableErrorCode.INPUT_INVALID),
    ],
)
def test_native_parse_outcome_rejects_ambiguous_or_empty_states(
    parser_format: ParserFormat | None,
    has_artifact: bool,
    code: StableErrorCode | None,
) -> None:
    _, artifact = _txt_document()

    with pytest.raises(ValueError, match="exactly one outcome"):
        NativeParseOutcome(
            parser_format=parser_format,
            artifact=artifact if has_artifact else None,
            stable_error_code=code,
        )


def test_native_parse_outcome_binds_artifact_format_to_dispatch() -> None:
    _, artifact = _txt_document()

    with pytest.raises(ValueError, match="format does not match"):
        NativeParseOutcome(
            parser_format=ParserFormat.HTML,
            artifact=artifact,
            stable_error_code=None,
        )


def test_native_parse_outcome_exposes_artifact_bytes_only_on_success() -> None:
    _, artifact = _txt_document()
    success = NativeParseOutcome(ParserFormat.TXT, artifact, None)
    failure = NativeParseOutcome(None, None, StableErrorCode.INPUT_INVALID)

    assert success.artifact_bytes == artifact.canonical_bytes
    assert failure.artifact_bytes == b""


def test_dispatch_preserves_router_failures_before_parser_activation() -> None:
    outcome = parse_native_source(
        b"plain",
        declared_mime="text/plain",
        declared_extension=".md",
    )

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.FORMAT_MISMATCH


def test_dispatch_activates_csv_without_fallback() -> None:
    outcome = parse_native_source(b"a,b\n1,2\n", declared_extension=".csv")

    assert outcome.parser_format is ParserFormat.CSV
    assert outcome.artifact is not None
    assert outcome.stable_error_code is None


def test_dispatch_maps_parser_failures_without_partial_artifacts() -> None:
    outcome = parse_native_source(
        b"text",
        declared_extension=".txt",
        limits=replace(NativeParserLimits(), text_bytes=1),
    )

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.RESULT_TOO_LARGE
    assert outcome.artifact_bytes == b""


def test_successful_dispatch_returns_an_artifact_bound_to_exact_source() -> None:
    source = "café".encode()

    outcome = parse_native_source(source, declared_extension=".txt")

    assert outcome.parser_format is ParserFormat.TXT
    assert outcome.artifact is not None
    outcome.artifact.validate_source_binding(
        source,
        expected_format=ParserFormat.TXT,
    )


def test_html_dispatch_preserves_the_frozen_gb18030_decode_candidate() -> None:
    source = "<!doctype html><html><body>中文</body></html>".encode("gb18030")

    outcome = parse_native_source(source, declared_extension=".html")

    assert outcome.parser_format is ParserFormat.HTML
    assert outcome.artifact is not None
    assert outcome.artifact.source_encoding == "gb18030"


@pytest.mark.parametrize(
    ("parser_format", "source"),
    [
        (ParserFormat.TXT, b"plain"),
        (ParserFormat.MARKDOWN, b"# heading\n"),
        (ParserFormat.HTML, b"<html><body><p>text</p></body></html>"),
        (ParserFormat.CSV, b"name,value\none,1\n"),
    ],
)
def test_fixed_dispatch_selects_each_supported_native_parser(
    parser_format: ParserFormat,
    source: bytes,
) -> None:
    decoded = decode_source(source)

    artifact = _parse_selected(parser_format, decoded, NativeParserLimits())

    assert artifact.source_format is parser_format
    artifact.validate_source_binding(source, expected_format=parser_format)


def test_fixed_dispatch_has_no_fallback_for_other_router_formats() -> None:
    decoded = decode_source(b"value")

    with pytest.raises(NativeParseFailure) as observed:
        _parse_selected(ParserFormat.PDF, decoded, NativeParserLimits())

    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED


@pytest.mark.parametrize(
    "limits",
    [
        replace(NativeParserLimits(), nodes=1),
        replace(NativeParserLimits(), fragments=0),
        replace(NativeParserLimits(), artifact_bytes=1),
    ],
)
def test_artifact_aggregate_limits_fail_closed(limits: NativeParserLimits) -> None:
    _, artifact = _txt_document()

    with pytest.raises(NativeParseFailure) as observed:
        _enforce_artifact_limits(artifact, limits)

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_artifact_attribute_limit_is_enforced_independently() -> None:
    decoded = decode_source(b"# heading\n")
    artifact = parse_markdown(decoded, NativeParserLimits())

    with pytest.raises(NativeParseFailure) as observed:
        _enforce_artifact_limits(
            artifact,
            replace(NativeParserLimits(), attributes=0),
        )

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_artifact_rejects_a_non_root_node_without_a_parent() -> None:
    _, artifact = _txt_document()
    parentless = replace(artifact.nodes[1], parent_ordinal=None)
    malformed = replace(artifact, nodes=(artifact.nodes[0], parentless))

    with pytest.raises(NativeParseFailure) as observed:
        _enforce_artifact_limits(malformed, NativeParserLimits())

    assert observed.value.code is StableErrorCode.PARSER_SCHEMA_MISMATCH


def test_artifact_nesting_depth_limit_is_enforced() -> None:
    _, artifact = _txt_document()

    with pytest.raises(NativeParseFailure) as observed:
        _enforce_artifact_limits(
            artifact,
            replace(NativeParserLimits(), nesting_depth=0),
        )

    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE


def test_locator_validation_binds_source_encoding() -> None:
    decoded, artifact = _txt_document(b"a")
    malformed_unit = replace(artifact.source_units[0], encoding="gb18030")
    malformed = replace(artifact, source_units=(malformed_unit,))

    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(malformed, decoded)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_locator_validation_binds_decoded_scalar_count() -> None:
    _, artifact = _txt_document(b"a")
    different_source = decode_source(b"ab")

    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(artifact, different_source)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_locator_validation_recomputes_each_node_position() -> None:
    decoded, artifact = _txt_document()
    paragraph = replace(
        artifact.nodes[1],
        source_position=replace(
            artifact.nodes[1].source_position,
            raw_byte_start=1,
        ),
        fragments=(),
    )
    malformed = replace(artifact, nodes=(artifact.nodes[0], paragraph))

    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(malformed, decoded)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_locator_validation_recomputes_each_fragment_position() -> None:
    decoded, artifact = _txt_document()
    paragraph = artifact.nodes[1]
    fragment = paragraph.fragments[0]
    malformed_position = replace(fragment.source_position, raw_byte_start=1)
    malformed_fragment = replace(fragment, source_position=malformed_position)
    malformed_paragraph = replace(paragraph, fragments=(malformed_fragment,))
    malformed = replace(
        artifact,
        nodes=(artifact.nodes[0], malformed_paragraph),
    )

    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(malformed, decoded)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_locator_validation_binds_identity_text_to_its_source_span() -> None:
    decoded, artifact = _txt_document()
    paragraph = artifact.nodes[1]
    fragment = replace(paragraph.fragments[0], text="forged")
    malformed = replace(
        artifact,
        nodes=(
            artifact.nodes[0],
            replace(paragraph, fragments=(fragment,)),
        ),
    )

    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(malformed, decoded)

    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED


def test_locator_validation_allows_declared_syntax_decoding() -> None:
    decoded, artifact = _txt_document()
    paragraph = artifact.nodes[1]
    fragment = replace(
        paragraph.fragments[0],
        text="decoded",
        transform=NativeTransformKind.SYNTAX_DECODE,
    )
    transformed = replace(
        artifact,
        nodes=(
            artifact.nodes[0],
            replace(paragraph, fragments=(fragment,)),
        ),
    )

    _validate_artifact_positions(transformed, decoded)


@pytest.mark.parametrize(
    ("parser_format", "relative", "extension"),
    [
        (ParserFormat.DOCX, "golden/docx/minimal.docx", ".docx"),
        (ParserFormat.PPTX, "golden/pptx/minimal.pptx", ".pptx"),
        (ParserFormat.XLSX, "golden/xlsx/representative.xlsx", ".xlsx"),
    ],
)
def test_dispatch_uses_the_router_admitted_ooxml_capability(
    parser_format: ParserFormat,
    relative: str,
    extension: str,
) -> None:
    source = (_CORPUS / relative).read_bytes()

    outcome = parse_native_source(source, declared_extension=extension)

    assert outcome.parser_format is parser_format
    assert outcome.artifact is not None
    assert outcome.stable_error_code is None
    outcome.artifact.validate_source_binding(source, expected_format=parser_format)


def test_dispatch_passes_hash_bound_limits_into_ooxml_admission() -> None:
    source = (_CORPUS / "golden/xlsx/representative.xlsx").read_bytes()
    limits = replace(NativeParserLimits(), xlsx_cell_text_bytes=5)

    outcome = parse_native_source(
        source,
        declared_extension=".xlsx",
        limits=limits,
    )

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.INPUT_INVALID


def test_dispatch_rejects_inconsistent_router_capability_states(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = (_CORPUS / "golden/docx/minimal.docx").read_bytes()
    package = route_source(source).opc_package
    assert package is not None

    inconsistent = (
        RouteDecision(ParserFormat.TXT, None, package),
        RouteDecision(ParserFormat.DOCX, None, None),
    )
    for decision in inconsistent:
        monkeypatch.setattr(
            dispatch,
            "route_source",
            lambda *_args, _decision=decision, **_kwargs: _decision,
        )
        outcome = parse_native_source(b"text", declared_extension=".txt")
        assert outcome.stable_error_code is StableErrorCode.PARSER_SCHEMA_MISMATCH


def test_selected_parser_rejects_the_wrong_source_capability() -> None:
    decoded = decode_source(b"text")
    source = (_CORPUS / "golden/docx/minimal.docx").read_bytes()
    package = route_source(source).opc_package
    assert package is not None

    for parser_format, parser_source in (
        (ParserFormat.TXT, package),
        (ParserFormat.DOCX, decoded),
    ):
        with pytest.raises(NativeParseFailure) as observed:
            _parse_selected(parser_format, parser_source, NativeParserLimits())
        assert observed.value.code is StableErrorCode.PARSER_SCHEMA_MISMATCH

    with pytest.raises(NativeParseFailure) as observed:
        dispatch._parse_text_selected(
            ParserFormat.PDF,
            decoded,
            NativeParserLimits(),
        )
    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED
    with pytest.raises(NativeParseFailure) as observed:
        dispatch._parse_ooxml_selected(
            ParserFormat.PDF,
            package,
            NativeParserLimits(),
        )
    assert observed.value.code is StableErrorCode.FORMAT_UNSUPPORTED


def test_ooxml_locator_validation_binds_package_format_and_source_units() -> None:
    source = (_CORPUS / "golden/docx/minimal.docx").read_bytes()
    decision = route_source(source)
    package = decision.opc_package
    assert package is not None
    outcome = parse_native_source(source, declared_extension=".docx")
    assert outcome.artifact is not None

    mismatched_format = replace(
        outcome.artifact,
        source_format=ParserFormat.PPTX,
    )
    with pytest.raises(NativeParseFailure) as observed:
        _validate_artifact_positions(mismatched_format, package)
    assert observed.value.code is StableErrorCode.QUALITY_LOCATOR_FAILED

    with pytest.raises(NativeParseFailure) as observed:
        _enforce_artifact_limits(
            outcome.artifact,
            replace(NativeParserLimits(), source_units=1),
        )
    assert observed.value.code is StableErrorCode.RESULT_TOO_LARGE
