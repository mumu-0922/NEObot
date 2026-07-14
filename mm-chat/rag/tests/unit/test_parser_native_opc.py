"""Shared hardened OPC package admission and capability tests for C1.3B."""

from __future__ import annotations

import io
import stat
import struct
import zipfile
from dataclasses import replace
from pathlib import Path
from typing import Any, cast

import pytest

from mm_chat_rag.offline_parser.config import NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native import opc
from mm_chat_rag.offline_parser.native.model import (
    NativeParseFailure,
    NativeSourceUnitKind,
)
from mm_chat_rag.offline_parser.native.opc import admit_ooxml_package
from mm_chat_rag.offline_parser.native.xml_source import parse_xml_source

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
_TIMESTAMP = (2026, 1, 1, 0, 0, 0)
_MAIN_TYPE = (
    b"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
)
_MAIN_OVERRIDE = (
    b'<Override PartName="/word/document.xml" ContentType="' + _MAIN_TYPE + b'"/>'
)
_CONTENT_TYPES = (
    b'<?xml version="1.0" encoding="UTF-8"?>'
    b'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
    b'<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.'
    b'relationships+xml"/>'
    b'<Default Extension="xml" ContentType="application/xml"/>'
    + _MAIN_OVERRIDE
    + b"</Types>"
)
_ROOT_RELS = (
    b'<?xml version="1.0" encoding="UTF-8"?>'
    b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/'
    b'relationships">'
    b'<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/'
    b'officeDocument/2006/relationships/officeDocument" '
    b'Target="word/document.xml"/>'
    b"</Relationships>"
)
_DOCUMENT = (
    b'<w:document xmlns:w="http://schemas.openxmlformats.org/'
    b'wordprocessingml/2006/main">'
    b"<w:body><w:p><w:r><w:t>x</w:t></w:r></w:p></w:body></w:document>"
)


def _zip(
    parts: list[tuple[str, bytes]],
    *,
    compression: int = zipfile.ZIP_STORED,
    special_name: str | None = None,
) -> bytes:
    target = io.BytesIO()
    with zipfile.ZipFile(target, mode="w") as archive:
        for name, content in sorted(parts):
            info = zipfile.ZipInfo(name, _TIMESTAMP)
            info.compress_type = compression
            info.create_system = 3
            info.external_attr = (
                (stat.S_IFLNK | 0o777) << 16
                if name == special_name
                else (stat.S_IFREG | 0o644) << 16
            )
            archive.writestr(info, content)
    return target.getvalue()


def _descriptor_zip(parts: list[tuple[str, bytes]]) -> bytes:
    class _Unseekable(io.BytesIO):
        def seekable(self) -> bool:
            return False

        def seek(self, _offset: int, _whence: int = 0) -> int:
            raise io.UnsupportedOperation

    target = _Unseekable()
    with zipfile.ZipFile(target, mode="w") as archive:
        for name, content in sorted(parts):
            archive.writestr(name, content, compress_type=zipfile.ZIP_DEFLATED)
    return target.getvalue()


def _docx(
    *extra_parts: tuple[str, bytes],
    content_types: bytes = _CONTENT_TYPES,
    root_relationships: bytes = _ROOT_RELS,
) -> bytes:
    return _zip(
        [
            ("[Content_Types].xml", content_types),
            ("_rels/.rels", root_relationships),
            ("word/document.xml", _DOCUMENT),
            *extra_parts,
        ]
    )


def _assert_failure(source: bytes, code: StableErrorCode) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        admit_ooxml_package(source, NativeParserLimits())
    assert observed.value.code is code


@pytest.mark.parametrize(
    ("relative", "expected"),
    [
        ("golden/docx/minimal.docx", ParserFormat.DOCX),
        ("golden/pptx/minimal.pptx", ParserFormat.PPTX),
        ("golden/xlsx/representative.xlsx", ParserFormat.XLSX),
    ],
)
def test_golden_packages_admit_with_canonical_source_units(
    relative: str,
    expected: ParserFormat,
) -> None:
    source = (_CORPUS / relative).read_bytes()
    package = admit_ooxml_package(source, NativeParserLimits())

    assert package.parser_format is expected
    assert package.source_bytes == len(source)
    assert package.source_units[0].kind is NativeSourceUnitKind.RAW_FILE
    assert package.source_units[0].encoding is None
    assert [part.canonical_uri for part in package.parts] == sorted(
        (part.canonical_uri for part in package.parts),
        key=lambda value: value.encode("utf-8"),
    )
    assert [unit.ordinal for unit in package.source_units] == list(
        range(len(package.source_units))
    )


def test_capability_rechecks_part_bytes_and_parses_bound_xml() -> None:
    package = admit_ooxml_package(_docx(), NativeParserLimits())
    content = package.read_part("/word/document.xml")
    parsed = package.parse_xml_part("/word/document.xml")
    part = package.part("/word/document.xml")

    assert content == _DOCUMENT
    assert parsed.decoded.source_unit_ordinal == part.source_unit_ordinal
    assert parsed.root.source_position.source_unit_ordinal == part.source_unit_ordinal
    assert parsed.root.name.endswith("}document")
    with pytest.raises(NativeParseFailure):
        package.read_part("/missing.xml")
    with pytest.raises(NativeParseFailure):
        package.parse_xml_part("/[Content_Types].xml.bin")


def test_external_relationship_is_metadata_only_and_never_a_part_target() -> None:
    source = (_CORPUS / "adversarial/ooxml/external-rel.docx").read_bytes()
    package = admit_ooxml_package(source, NativeParserLimits())
    external = package.external_relationships

    assert len(external) == 1
    assert external[0].is_external
    assert external[0].target_part_uri is None
    assert external[0].target.startswith("https://example.invalid/")
    assert (
        package.resolve_relationship(
            "/word/document.xml",
            "rId9",
        )
        == external[0]
    )


@pytest.mark.parametrize(
    ("relative", "expected"),
    [
        ("adversarial/archive/duplicate-entry.docx", StableErrorCode.INPUT_INVALID),
        ("adversarial/archive/traversal.docx", StableErrorCode.INPUT_INVALID),
        ("adversarial/archive/header-drift.docx", StableErrorCode.INPUT_INVALID),
        ("adversarial/archive/encrypted-entry.docx", StableErrorCode.INPUT_INVALID),
        (
            "adversarial/archive/case-collision.docx",
            StableErrorCode.INPUT_INVALID,
        ),
        ("adversarial/archive/non-nfc-name.docx", StableErrorCode.INPUT_INVALID),
        (
            "adversarial/archive/zip-bomb.docx",
            StableErrorCode.ARCHIVE_LIMIT_EXCEEDED,
        ),
        (
            "adversarial/limits/path-513.docx",
            StableErrorCode.ARCHIVE_LIMIT_EXCEEDED,
        ),
        (
            "adversarial/limits/oversized-cell.xlsx",
            StableErrorCode.INPUT_INVALID,
        ),
        (
            "adversarial/ooxml/missing-part.docx",
            StableErrorCode.INPUT_INVALID,
        ),
        ("adversarial/ooxml/xxe.docx", StableErrorCode.INPUT_INVALID),
        (
            "adversarial/ooxml/macro.docm",
            StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED,
        ),
        (
            "adversarial/ooxml/ole.docx",
            StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED,
        ),
    ],
)
def test_frozen_archive_and_ooxml_adversarial_cases_fail_closed(
    relative: str,
    expected: StableErrorCode,
) -> None:
    _assert_failure((_CORPUS / relative).read_bytes(), expected)


def test_percent_equivalent_part_collision_is_rejected_before_selection() -> None:
    _assert_failure(
        _docx(("word/%64ocument.xml", b"<x/>")),
        StableErrorCode.INPUT_INVALID,
    )


def test_unsupported_compression_and_special_files_are_rejected() -> None:
    parts = [
        ("[Content_Types].xml", _CONTENT_TYPES),
        ("_rels/.rels", _ROOT_RELS),
        ("word/document.xml", _DOCUMENT),
    ]
    _assert_failure(
        _zip(parts, compression=zipfile.ZIP_BZIP2),
        StableErrorCode.INPUT_INVALID,
    )
    _assert_failure(
        _zip(parts, special_name="word/document.xml"),
        StableErrorCode.INPUT_INVALID,
    )


def test_content_type_marker_in_comment_is_not_format_authority() -> None:
    decoy = _CONTENT_TYPES.replace(
        _MAIN_OVERRIDE,
        b"<!-- " + _MAIN_TYPE + b" -->",
    )
    _assert_failure(
        _zip(
            [
                ("[Content_Types].xml", decoy),
                ("_rels/.rels", _ROOT_RELS),
                ("word/document.xml", _DOCUMENT),
            ]
        ),
        StableErrorCode.INPUT_INVALID,
    )


def test_internal_relationship_must_resolve_inside_the_admitted_package() -> None:
    relationships = (
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        b'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/'
        b'officeDocument/2006/relationships/image" Target="../../escape.bin"/>'
        b"</Relationships>"
    )
    _assert_failure(
        _docx(("word/_rels/document.xml.rels", relationships)),
        StableErrorCode.INPUT_INVALID,
    )


def test_external_relationship_type_is_closed_to_non_fetching_profiles() -> None:
    relationships = (
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        b'<Relationship Id="rId2" Type="urn:unsafe:template" '
        b'Target="file:///etc/passwd" TargetMode="External"/>'
        b"</Relationships>"
    )
    _assert_failure(
        _docx(("word/_rels/document.xml.rels", relationships)),
        StableErrorCode.INPUT_INVALID,
    )


def test_package_wide_xml_and_archive_limits_are_hash_bound() -> None:
    baseline = NativeParserLimits()
    source = _docx()
    for limits in (
        replace(baseline, archive_entries=2),
        replace(baseline, source_units=3),
        replace(baseline, xml_package_nodes=1),
        replace(baseline, xml_package_attributes=1),
        replace(baseline, xml_package_text_bytes=0),
    ):
        with pytest.raises(NativeParseFailure) as observed:
            admit_ooxml_package(source, limits)
        assert observed.value.code is StableErrorCode.ARCHIVE_LIMIT_EXCEEDED


def test_xlsx_cell_text_limit_is_hash_bound() -> None:
    source = (_CORPUS / "golden/xlsx/representative.xlsx").read_bytes()
    limits = replace(NativeParserLimits(), xlsx_cell_text_bytes=5)

    with pytest.raises(NativeParseFailure) as observed:
        admit_ooxml_package(source, limits)

    assert observed.value.code is StableErrorCode.INPUT_INVALID


def test_capability_fails_closed_for_missing_non_xml_and_corrupt_parts() -> None:
    content_types = _CONTENT_TYPES.replace(
        b"</Types>",
        b'<Default Extension="bin" ContentType="application/octet-stream"/></Types>',
    )
    package = admit_ooxml_package(
        _docx(
            ("word/media/image.bin", b"image"),
            content_types=content_types,
        ),
        NativeParserLimits(),
    )

    assert package.relationships_from("/")
    with pytest.raises(NativeParseFailure):
        package.resolve_relationship("/", "missing")
    with pytest.raises(NativeParseFailure):
        package.parse_xml_part("/word/media/image.bin")

    corrupt_archive = replace(package, _source=b"not-a-zip")
    with pytest.raises(NativeParseFailure):
        corrupt_archive.read_part("/word/document.xml")

    document = package.part("/word/document.xml")
    corrupt_unit = replace(document.source_unit, source_sha256="0" * 64)
    corrupt_part = replace(document, source_unit=corrupt_unit)
    corrupt_metadata = replace(
        package,
        parts=tuple(
            corrupt_part if part == document else part for part in package.parts
        ),
    )
    with pytest.raises(NativeParseFailure):
        corrupt_metadata.read_part("/word/document.xml")


def test_admission_rejects_wrong_type_size_empty_and_nested_archives(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    with pytest.raises(TypeError, match="must be bytes"):
        admit_ooxml_package(cast("Any", "not-bytes"), NativeParserLimits())
    monkeypatch.setattr(opc, "_MAX_SOURCE_BYTES", 1)
    with pytest.raises(NativeParseFailure) as observed:
        admit_ooxml_package(b"xx", NativeParserLimits())
    assert observed.value.code is StableErrorCode.INPUT_TOO_LARGE
    monkeypatch.undo()

    _assert_failure(_zip([]), StableErrorCode.INPUT_INVALID)
    _assert_failure(
        (_CORPUS / "adversarial/archive/nested-archive.docx").read_bytes(),
        StableErrorCode.ARCHIVE_LIMIT_EXCEEDED,
    )


@pytest.mark.parametrize(
    "limits",
    [
        replace(NativeParserLimits(), archive_entry_bytes=1),
        replace(NativeParserLimits(), archive_expanded_bytes=1),
        replace(NativeParserLimits(), relationships=0),
    ],
)
def test_independent_archive_and_relationship_limits_are_enforced(
    limits: NativeParserLimits,
) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        admit_ooxml_package(_docx(), limits)

    assert observed.value.code is StableErrorCode.ARCHIVE_LIMIT_EXCEEDED


@pytest.mark.parametrize(
    "content_types",
    [
        _CONTENT_TYPES.replace(b"<Types ", b'<Types unexpected="1" '),
        _CONTENT_TYPES.replace(b">", b">unexpected", 1),
        _CONTENT_TYPES.replace(
            b"<Default ", b"<Default><Nested/></Default><Default ", 1
        ),
        _CONTENT_TYPES.replace(
            b"</Types>",
            b'<Default Extension="xml" ContentType="application/xml"/></Types>',
        ),
        _CONTENT_TYPES.replace(b"<Default ", b"<Unknown ", 1),
        _CONTENT_TYPES.replace(
            b"wordprocessingml.document.main+xml",
            b"wordprocessingml.document.macroEnabled.main+xml",
        ),
    ],
)
def test_content_types_semantics_reject_open_or_active_shapes(
    content_types: bytes,
) -> None:
    _assert_failure(
        _docx(content_types=content_types),
        (
            StableErrorCode.ACTIVE_CONTENT_UNSUPPORTED
            if b"macroEnabled" in content_types
            else StableErrorCode.INPUT_INVALID
        ),
    )


def test_content_types_require_one_main_type_at_the_expected_uri() -> None:
    ppt_type = (
        b"application/vnd.openxmlformats-officedocument."
        b"presentationml.presentation.main+xml"
    )
    two_mains = _CONTENT_TYPES.replace(
        b"</Types>",
        b'<Override PartName="/ppt/presentation.xml" ContentType="'
        + ppt_type
        + b'"/></Types>',
    )
    _assert_failure(
        _docx(
            ("ppt/presentation.xml", b"<presentation/>"),
            content_types=two_mains,
        ),
        StableErrorCode.INPUT_INVALID,
    )

    wrong_uri = _CONTENT_TYPES.replace(
        b'<Override PartName="/word/document.xml"',
        b'<Override PartName="/word/other.xml"',
    )
    _assert_failure(
        _docx(
            ("word/other.xml", b"<document/>"),
            content_types=wrong_uri,
        ),
        StableErrorCode.INPUT_INVALID,
    )


@pytest.mark.parametrize(
    "root_relationships",
    [
        _ROOT_RELS.replace(b"<Relationships ", b'<Relationships bad="1" '),
        _ROOT_RELS.replace(b">", b">unexpected", 1),
        _ROOT_RELS.replace(b"<Relationship ", b"<Unknown ", 1),
        _ROOT_RELS.replace(b'Id="rId1"', b'Id="1bad"'),
        _ROOT_RELS.replace(
            b"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument",
            b"not-a-uri",
        ),
        _ROOT_RELS.replace(b'Target="word/document.xml"', b'Target="missing.xml"'),
        _ROOT_RELS.replace(
            b'Target="word/document.xml"',
            b'Target="word/document.xml" TargetMode="Unknown"',
        ),
        _ROOT_RELS.replace(
            b"<Relationship ",
            b"<Relationship><Nested/></Relationship><Relationship ",
            1,
        ),
    ],
)
def test_relationship_semantics_reject_open_or_dangling_shapes(
    root_relationships: bytes,
) -> None:
    _assert_failure(
        _docx(root_relationships=root_relationships),
        StableErrorCode.INPUT_INVALID,
    )


def test_relationship_source_and_root_office_document_are_closed() -> None:
    without_root = _zip(
        [
            ("[Content_Types].xml", _CONTENT_TYPES),
            ("word/document.xml", _DOCUMENT),
        ]
    )
    _assert_failure(without_root, StableErrorCode.INPUT_INVALID)

    orphan_relationship = (
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/'
        b'relationships"></Relationships>'
    )
    _assert_failure(
        _docx(("ghost/_rels/missing.xml.rels", orphan_relationship)),
        StableErrorCode.INPUT_INVALID,
    )

    external_root = _ROOT_RELS.replace(
        b'Target="word/document.xml"',
        b'Target="https://example.invalid/document.xml" TargetMode="External"',
    )
    _assert_failure(
        _docx(root_relationships=external_root),
        StableErrorCode.INPUT_INVALID,
    )


@pytest.mark.parametrize(
    "value",
    ["", "/absolute", "trailing/", "a\\b", "a\x00b", "e\u0301.xml", "C:/x"],
)
def test_canonical_part_uri_rejects_cross_platform_ambiguity(value: str) -> None:
    with pytest.raises(NativeParseFailure) as observed:
        opc._canonical_part_uri(value, NativeParserLimits())

    assert observed.value.code is StableErrorCode.INPUT_INVALID


@pytest.mark.parametrize(
    "value",
    [
        "",
        "space here",
        "bad?query",
        "bad#fragment",
        "%",
        "%2F",
        "%00",
        "%C3%A9",
        "..",
    ],
)
def test_canonical_segments_reject_ambiguous_or_unsafe_values(value: str) -> None:
    with pytest.raises(NativeParseFailure):
        opc._canonical_segment(value, allow_dot=False)


@pytest.mark.parametrize(
    "target",
    [
        "",
        "/absolute",
        "https://example.invalid/x",
        "a?query",
        "a#fragment",
        "\\x",
        "../../x",
    ],
)
def test_internal_targets_cannot_escape_or_become_urls(target: str) -> None:
    with pytest.raises(NativeParseFailure):
        opc._resolve_internal_target("/word/document.xml", target)

    assert opc._resolve_internal_target(
        "/word/document.xml", "./media/../image.xml"
    ) == ("/word/image.xml")


def test_low_level_zip_and_content_type_defenses() -> None:
    for ranges, central_start in (([(1, 2)], 2), ([(0, 1)], 2)):
        with pytest.raises(NativeParseFailure):
            opc._validate_local_ranges(ranges, central_start)
    with pytest.raises(NativeParseFailure):
        opc._has_zip64_extra(b"\x01")
    with pytest.raises(NativeParseFailure):
        opc._has_zip64_extra(struct.pack("<HH", 2, 1))
    assert opc._has_zip64_extra(struct.pack("<HH", 1, 0))
    assert not opc._has_zip64_extra(struct.pack("<HH", 2, 0))
    for value in ("", " text/xml", "text xml", "text"):
        with pytest.raises(NativeParseFailure):
            opc._content_type(value)
    assert opc._part_extension("/no-extension") == ""
    assert opc._part_extension("/UPPER.XML") == "xml"


def test_data_descriptor_packages_are_fully_reconciled() -> None:
    source = _descriptor_zip(
        [
            ("[Content_Types].xml", _CONTENT_TYPES),
            ("_rels/.rels", _ROOT_RELS),
            ("word/document.xml", _DOCUMENT),
        ]
    )

    package = admit_ooxml_package(source, NativeParserLimits())

    assert package.parser_format is ParserFormat.DOCX


def test_eocd_local_header_and_zip64_defensive_branches() -> None:
    with pytest.raises(NativeParseFailure):
        opc._validate_eocd(b"")

    source = bytearray(_docx())
    eocd = source.rfind(b"PK\x05\x06")
    source[eocd + 4 : eocd + 6] = (1).to_bytes(2, "little")
    with pytest.raises(NativeParseFailure):
        opc._validate_eocd(bytes(source))

    valid = _docx()
    central_start = opc._validate_eocd(valid)
    with zipfile.ZipFile(io.BytesIO(valid)) as archive:
        info = archive.infolist()[0]
        with pytest.raises(NativeParseFailure):
            opc._validate_local_header(b"", info, 0)
        bad_magic = bytearray(valid)
        bad_magic[info.header_offset : info.header_offset + 4] = b"FAIL"
        with pytest.raises(NativeParseFailure):
            opc._validate_local_header(bytes(bad_magic), info, central_start)
        bad_crc = bytearray(valid)
        bad_crc[info.header_offset + 14 : info.header_offset + 18] = b"\x00" * 4
        with pytest.raises(NativeParseFailure):
            opc._validate_local_header(bytes(bad_crc), info, central_start)

    zip64 = zipfile.ZipInfo("x")
    zip64.create_system = 3
    zip64.external_attr = (stat.S_IFREG | 0o644) << 16
    zip64.extra = struct.pack("<HH", 1, 0)
    with pytest.raises(NativeParseFailure):
        opc._validate_zip_info(zip64)
    multi_disk = zipfile.ZipInfo("x")
    multi_disk.create_system = 3
    multi_disk.external_attr = (stat.S_IFREG | 0o644) << 16
    multi_disk.volume = 1
    with pytest.raises(NativeParseFailure):
        opc._validate_zip_info(multi_disk)


def test_invalid_utf8_central_directory_name_maps_to_input_invalid() -> None:
    source = bytearray(_docx())
    local = source.find(b"PK\x03\x04")
    central = source.find(b"PK\x01\x02")
    for flag_offset in (local + 6, central + 8):
        flags = int.from_bytes(source[flag_offset : flag_offset + 2], "little")
        source[flag_offset : flag_offset + 2] = (flags | 0x0800).to_bytes(2, "little")
    source[local + 30] = 0xFF
    source[central + 46] = 0xFF

    _assert_failure(bytes(source), StableErrorCode.INPUT_INVALID)


def test_missing_content_types_and_unresolved_part_types_fail_closed() -> None:
    _assert_failure(
        _zip(
            [
                ("_rels/.rels", _ROOT_RELS),
                ("word/document.xml", _DOCUMENT),
            ]
        ),
        StableErrorCode.INPUT_INVALID,
    )
    _assert_failure(
        _docx(("word/data.unknown", b"data")),
        StableErrorCode.INPUT_INVALID,
    )
    non_whitespace = _CONTENT_TYPES.replace(
        b'content-types">',
        b'content-types">unexpected',
    )
    _assert_failure(
        _docx(content_types=non_whitespace),
        StableErrorCode.INPUT_INVALID,
    )


def test_relationship_parts_and_root_office_type_are_exact() -> None:
    wrong_relationship_content_type = _CONTENT_TYPES.replace(
        b"application/vnd.openxmlformats-package.relationships+xml",
        b"application/xml",
    )
    _assert_failure(
        _docx(content_types=wrong_relationship_content_type),
        StableErrorCode.INPUT_INVALID,
    )

    custom_office_type = _ROOT_RELS.replace(
        b"http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
        b"officeDocument",
        b"urn:fixture/officeDocument",
    )
    _assert_failure(
        _docx(root_relationships=custom_office_type),
        StableErrorCode.INPUT_INVALID,
    )


def test_relationship_whitespace_and_source_uri_helpers_fail_closed() -> None:
    non_whitespace = _ROOT_RELS.replace(
        b'relationships">',
        b'relationships">unexpected',
    )
    _assert_failure(
        _docx(root_relationships=non_whitespace),
        StableErrorCode.INPUT_INVALID,
    )
    for value in ("invalid.rels", "/_rels/x.rels", "/_rels/.rels.rels"):
        with pytest.raises(NativeParseFailure):
            opc._relationship_source_uri(value)
    with pytest.raises(NativeParseFailure):
        opc._resolve_internal_target("/", ".")


def test_override_and_shared_string_helper_branches_are_closed() -> None:
    for value in ("word/document.xml", "/word/%64ocument.xml"):
        with pytest.raises(NativeParseFailure):
            opc._canonical_override_uri(value)

    wrong_namespace = parse_xml_source(
        b"<sst/>",
        source_unit_ordinal=1,
        limits=NativeParserLimits(),
    )
    with pytest.raises(NativeParseFailure):
        opc._validate_xlsx_shared_string_lengths(
            wrong_namespace.root,
            NativeParserLimits(),
        )

    sheet_namespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    other_child = parse_xml_source(
        f'<sst xmlns="{sheet_namespace}"><other/></sst>'.encode(),
        source_unit_ordinal=1,
        limits=NativeParserLimits(),
    )
    opc._validate_xlsx_shared_string_lengths(
        other_child.root,
        NativeParserLimits(),
    )
