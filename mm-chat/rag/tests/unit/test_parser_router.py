"""C1.2 magic/container-first router and no-fallback corpus gates."""

from __future__ import annotations

import io
import json
import struct
import zipfile
from pathlib import Path
from types import SimpleNamespace

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
    ("name", "flags", "error"),
    [
        ("", 0, StableErrorCode.INPUT_INVALID),
        ("/absolute", 0, StableErrorCode.INPUT_INVALID),
        ("C:/drive", 0, StableErrorCode.INPUT_INVALID),
        ("a//b", 0, StableErrorCode.INPUT_INVALID),
        ("a/%2F/b", 0, StableErrorCode.INPUT_INVALID),
        ("café.xml", 0, StableErrorCode.INPUT_INVALID),
    ],
)
def test_archive_entry_name_gate_rejects_cross_platform_ambiguity(
    name: str,
    flags: int,
    error: StableErrorCode,
) -> None:
    with pytest.raises(router.RouteFailure) as observed:
        router._validate_entry_name(name, flags)
    assert observed.value.code is error


def test_data_descriptor_gate_rejects_short_mismatch_and_trailing_bytes() -> None:
    info = SimpleNamespace(CRC=1, compress_size=2, file_size=3)
    valid = struct.pack("<III", 1, 2, 3)
    for source in (
        b"short",
        struct.pack("<III", 9, 2, 3),
        valid + b"FAIL",
    ):
        with pytest.raises(router.RouteFailure):
            router._validate_data_descriptor(source, 0, info)
    router._validate_data_descriptor(b"PK\x07\x08" + valid + b"PK\x01\x02", 0, info)


def test_archive_stream_and_read_helpers_fail_closed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    info = SimpleNamespace(file_size=2, CRC=0)

    class BrokenArchive:
        def open(self, _info: object, *, mode: str) -> object:
            raise zipfile.BadZipFile

        def read(self, _name: str) -> bytes:
            raise KeyError

    with pytest.raises(router.RouteFailure):
        router._verify_entry_stream(BrokenArchive(), info)
    with pytest.raises(router.RouteFailure):
        router._read_entry(BrokenArchive(), "missing")

    payload = b"xx"
    info = SimpleNamespace(file_size=3, CRC=0)

    class ShortArchive:
        def open(self, _info: object, *, mode: str) -> io.BytesIO:
            return io.BytesIO(payload)

    with pytest.raises(router.RouteFailure):
        router._verify_entry_stream(ShortArchive(), info)

    monkeypatch.setattr(router, "MAX_XML_BYTES", 1)

    class LargeArchive:
        def read(self, _name: str) -> bytes:
            return b"xx"

    with pytest.raises(router.RouteFailure):
        router._read_entry(LargeArchive(), "large")


def test_ooxml_xml_relationship_and_shared_string_negative_helpers(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    with pytest.raises(router.RouteFailure):
        router._ooxml_format(b"no known content type")
    with pytest.raises(router.RouteFailure):
        router._validate_xml_bytes(b"<broken>")
    monkeypatch.setattr(router, "MAX_XML_NODES", 1)
    with pytest.raises(router.RouteFailure):
        router._validate_xml_bytes(b"<root><child/></root>")

    duplicate_ids = (
        b'<Relationships><Relationship Id="x" Target="a"/>'
        b'<Relationship Id="x" Target="b"/></Relationships>'
    )
    traversal = (
        b'<Relationships><Relationship Id="x" Target="../../x"/></Relationships>'
    )
    for content in (duplicate_ids, traversal):
        with pytest.raises(router.RouteFailure):
            router._validate_relationships("word/_rels/document.xml.rels", content)
    with pytest.raises(router.RouteFailure):
        router._relationship_source_part("invalid.rels")

    oversized = b"<sst><si><t>abcd</t></si></sst>"
    monkeypatch.setattr(router, "MAX_XLSX_CELL_TEXT_BYTES", 3)
    with pytest.raises(router.RouteFailure):
        router._validate_xlsx_shared_strings(oversized)


def test_router_low_level_defensive_branches(monkeypatch: pytest.MonkeyPatch) -> None:
    pdf = (_CORPUS / "golden" / "pdf_native" / "representative.pdf").read_bytes()
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

    with pytest.raises(router.RouteFailure):
        router._validate_entry_name("\ud800", 0)
    for segments in (["%FF"], ["%2e%2e"], ["same", "same"]):
        with pytest.raises(router.RouteFailure):
            router._validate_percent_encoded_path(segments)

    info = SimpleNamespace(
        header_offset=0,
        filename="x",
        flag_bits=0,
        compress_type=0,
        CRC=0,
        compress_size=0,
        file_size=0,
    )
    with pytest.raises(router.RouteFailure):
        router._validate_local_header(b"short", info)
    bad_magic = b"FAIL" + b"\x00" * (router._ZIP_LOCAL_HEADER.size - 4)
    with pytest.raises(router.RouteFailure):
        router._validate_local_header(bad_magic, info)

    descriptor_info = SimpleNamespace(CRC=1, compress_size=2, file_size=3)
    with pytest.raises(router.RouteFailure):
        router._validate_data_descriptor(b"PK\x07\x08short", 0, descriptor_info)

    monkeypatch.setattr(router, "MAX_XML_BYTES", 1)
    with pytest.raises(router.RouteFailure):
        router._validate_xml_bytes(b"<x/>")
    with pytest.raises(router.RouteFailure):
        router._validate_relationships("_rels/.rels", b"<broken>")


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


def test_archive_stream_limit_branch(monkeypatch: pytest.MonkeyPatch) -> None:
    payload = b"xx"
    info = SimpleNamespace(file_size=2, CRC=0)

    class Archive:
        def open(self, _info: object, *, mode: str) -> io.BytesIO:
            return io.BytesIO(payload)

    monkeypatch.setattr(router, "MAX_ARCHIVE_ENTRY_BYTES", 1)
    with pytest.raises(router.RouteFailure) as observed:
        router._verify_entry_stream(Archive(), info)
    assert observed.value.code is StableErrorCode.ARCHIVE_LIMIT_EXCEEDED
