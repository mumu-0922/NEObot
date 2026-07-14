"""C1.2 magic/container-first router and no-fallback corpus gates."""

from __future__ import annotations

import io
import json
import zipfile
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser import router
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.router import route_source

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_EXPECTATIONS = json.loads(
    (_CORPUS / "recipes" / "expectations.v1.json").read_text(encoding="utf-8")
)["entries"]


def _extension_hint(path: Path) -> str | None:
    suffix = path.suffix.casefold()
    return suffix if suffix not in {".bin", ".xml"} else None


@pytest.mark.parametrize(
    ("entry"),
    _EXPECTATIONS,
    ids=[entry["path"] for entry in _EXPECTATIONS],
)
def test_router_matches_frozen_corpus_expectations(entry: dict[str, object]) -> None:
    relative = str(entry["path"])
    path = _CORPUS / relative

    decision = route_source(
        path.read_bytes(),
        declared_extension=_extension_hint(path),
    )

    assert (
        decision.parser_format.value if decision.parser_format is not None else None
    ) == entry["expectedRoute"]
    assert (
        decision.stable_error_code.value
        if decision.stable_error_code is not None
        else None
    ) == entry["expectedError"]


@pytest.mark.parametrize(
    ("source", "mime", "extension", "expected"),
    [
        (b"plain", "text/plain", ".md", StableErrorCode.FORMAT_MISMATCH),
        (b"plain", None, None, StableErrorCode.FORMAT_AMBIGUOUS),
        (b"plain", "text/x-generic", None, StableErrorCode.FORMAT_AMBIGUOUS),
        (b"plain", None, ".bin", StableErrorCode.FORMAT_MISMATCH),
        (b"\x00binary", None, ".txt", StableErrorCode.INPUT_INVALID),
        (b"\xff", None, ".txt", StableErrorCode.ENCODING_AMBIGUOUS),
    ],
)
def test_non_self_describing_text_requires_one_unique_registered_hint(
    source: bytes,
    mime: str | None,
    extension: str | None,
    expected: StableErrorCode,
) -> None:
    decision = route_source(
        source,
        declared_mime=mime,
        declared_extension=extension,
    )

    assert decision.parser_format is None
    assert decision.stable_error_code is expected


def test_selected_text_route_never_guesses_from_content() -> None:
    markdown_like = b"# heading\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"

    decision = route_source(
        markdown_like,
        declared_mime="text/plain",
        declared_extension=".txt",
    )

    assert decision.parser_format is ParserFormat.TXT
    assert decision.stable_error_code is None


def test_binary_parse_failure_never_falls_back_to_text() -> None:
    decision = route_source(b"PK\x03\x04not-a-zip", declared_extension=".txt")

    assert decision.parser_format is None
    assert decision.stable_error_code is StableErrorCode.INPUT_INVALID


def test_structured_magic_and_hints_are_independently_reconciled() -> None:
    pdf = (_CORPUS / "golden" / "pdf_native" / "representative.pdf").read_bytes()

    matching = route_source(
        pdf,
        declared_mime="application/pdf",
        declared_extension=".pdf",
    )
    mismatch = route_source(pdf, declared_extension=".docx")

    assert matching.parser_format is ParserFormat.PDF
    assert mismatch.stable_error_code is StableErrorCode.FORMAT_MISMATCH


def test_route_decision_acceptance_and_hard_source_limit(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    accepted = route_source(b"x", declared_extension=".txt")
    rejected = route_source(b"x")
    monkeypatch.setattr(router, "MAX_SOURCE_BYTES", 1)

    assert accepted.accepted
    assert not rejected.accepted
    assert route_source(b"xx").stable_error_code is StableErrorCode.INPUT_TOO_LARGE


@pytest.mark.parametrize(
    ("source", "mime", "extension", "error"),
    [
        (
            b"<?xml version='1.0'?><root/>",
            None,
            None,
            StableErrorCode.FORMAT_UNSUPPORTED,
        ),
        (b'{"not":"canonical"} ', None, None, StableErrorCode.FORMAT_AMBIGUOUS),
        (b"\x01\x02\x03\x04", None, None, StableErrorCode.FORMAT_UNSUPPORTED),
        (
            b"<!doctype html><html onload='x'>",
            "text/html",
            ".html",
            StableErrorCode.INPUT_INVALID,
        ),
        (b"\xff<html>", "text/html", ".html", StableErrorCode.ENCODING_AMBIGUOUS),
        (b"x", None, ".docm", StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED),
    ],
)
def test_additional_router_preflight_failures(
    source: bytes,
    mime: str | None,
    extension: str | None,
    error: StableErrorCode,
) -> None:
    decision = route_source(
        source,
        declared_mime=mime,
        declared_extension=extension,
    )
    assert decision.stable_error_code is error


def test_synthetic_mineru_hints_and_closed_shape_failures() -> None:
    layout = (
        _CORPUS / "golden" / "mineru_artifact_synthetic" / "layout.json"
    ).read_bytes()
    mismatch = route_source(layout, declared_extension=".txt")
    document = json.loads(layout)
    document["role"] = "unknown"
    bad_role = route_source(
        json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
    )
    document = json.loads(layout)
    document["pages"] = []
    no_pages = route_source(
        json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
    )

    assert mismatch.stable_error_code is StableErrorCode.FORMAT_MISMATCH
    assert bad_role.stable_error_code is StableErrorCode.PARSER_SCHEMA_MISMATCH
    assert no_pages.stable_error_code is StableErrorCode.PARSER_SCHEMA_MISMATCH


@pytest.mark.parametrize(
    ("relative", "expected"),
    [
        ("golden/docx/minimal.docx", ParserFormat.DOCX),
        ("golden/pptx/minimal.pptx", ParserFormat.PPTX),
        ("golden/xlsx/representative.xlsx", ParserFormat.XLSX),
    ],
)
def test_ooxml_routes_return_the_single_admitted_package_capability(
    relative: str,
    expected: ParserFormat,
) -> None:
    source = (_CORPUS / relative).read_bytes()

    decision = route_source(source)

    assert decision.parser_format is expected
    assert decision.opc_package is not None
    assert decision.opc_package.parser_format is expected
    assert decision.opc_package.source_bytes == len(source)


def test_non_ooxml_routes_never_return_an_opc_capability() -> None:
    text = route_source(b"plain", declared_extension=".txt")
    pdf = route_source((_CORPUS / "golden/pdf_native/representative.pdf").read_bytes())

    assert text.opc_package is None
    assert pdf.opc_package is None


def test_raw_xml_uses_the_shared_hardened_xml_policy() -> None:
    valid = route_source(b"<?xml version='1.0'?><root/>")
    invalid = route_source(b"<?xml version='1.0'?><!DOCTYPE x><root/>")

    assert valid.stable_error_code is StableErrorCode.FORMAT_UNSUPPORTED
    assert invalid.stable_error_code is StableErrorCode.INPUT_INVALID


def test_router_low_level_text_html_and_pdf_defenses() -> None:
    pdf = (_CORPUS / "golden/pdf_native/representative.pdf").read_bytes()
    assert (
        route_source(pdf, declared_mime="text/plain").stable_error_code
        is StableErrorCode.FORMAT_MISMATCH
    )
    with pytest.raises(router.RouteFailure):
        router._decode_text(b"\x00")
    assert router._looks_binary(b"\x00")
    assert not router._looks_binary(b"")
    with pytest.raises(ValueError, match="script"):
        router._SafeHtmlPreflight().handle_starttag("script", [])
    with pytest.raises(router.RouteFailure):
        router._preflight_pdf(b"%PDF-1.4\nxref\ntrailer\n%%EOF")

    huge_count = (
        b"%PDF-1.4\n/Type /Pages /Count "
        + b"9" * 5000
        + b"\n/Type /Page\nxref\ntrailer\n%%EOF"
    )
    assert (
        route_source(huge_count).stable_error_code
        is StableErrorCode.PAGE_LIMIT_EXCEEDED
    )


def test_ooxml_missing_content_types_and_macro_preflight() -> None:
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr("word/document.xml", "<document/>")
    assert (
        route_source(buffer.getvalue(), declared_extension=".docx").stable_error_code
        is StableErrorCode.INPUT_INVALID
    )

    macro = (_CORPUS / "adversarial" / "ooxml" / "macro.docm").read_bytes()
    assert (
        route_source(macro).stable_error_code
        is StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED
    )
