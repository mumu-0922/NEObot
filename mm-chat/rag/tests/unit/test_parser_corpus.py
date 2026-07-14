from __future__ import annotations

import hashlib
import re
import zipfile
from copy import deepcopy
from pathlib import Path
from xml.etree import ElementTree as ET

import pytest

from tests.support.parser_corpus import (
    CORPUS_ROOT,
    CorpusValidationError,
    JsonValue,
    canonical_json_bytes,
    load_expectations,
    load_manifest,
    manifest_entries,
    parse_canonical_json,
    validate_corpus_tree,
    validate_packaged_schema,
    validate_recipe_identity,
    validate_synthetic_mineru_pair,
)

SYNTHETIC_MINERU_SCHEMA_ID = (
    "https://schemas.mm-chat.invalid/parser/synthetic-mineru-artifact.v1.schema.json"
)

EXPECTED_PATHS = {
    "adversarial/archive/case-collision.docx",
    "adversarial/archive/duplicate-entry.docx",
    "adversarial/archive/encrypted-entry.docx",
    "adversarial/archive/header-drift.docx",
    "adversarial/archive/nested-archive.docx",
    "adversarial/archive/non-nfc-name.docx",
    "adversarial/archive/traversal.docx",
    "adversarial/archive/zip-bomb.docx",
    "adversarial/encoding/ambiguous.bin",
    "adversarial/encoding/invalid-utf8.txt",
    "adversarial/encoding/nul.txt",
    "adversarial/encoding/replacement-character.txt",
    "adversarial/limits/entry-count-10001.docx",
    "adversarial/limits/mineru-missing-structure.json",
    "adversarial/limits/mineru-page-mismatch.json",
    "adversarial/limits/oversized-cell.xlsx",
    "adversarial/limits/page-500.pdf",
    "adversarial/limits/page-501.pdf",
    "adversarial/limits/path-513.docx",
    "adversarial/ooxml/external-rel.docx",
    "adversarial/ooxml/macro.docm",
    "adversarial/ooxml/missing-part.docx",
    "adversarial/ooxml/ole.docx",
    "adversarial/ooxml/xxe.docx",
    "adversarial/pdf/complex-layout.pdf",
    "adversarial/pdf/encrypted.pdf",
    "adversarial/pdf/mixed.pdf",
    "adversarial/pdf/scanned.pdf",
    "adversarial/pdf/truncated.pdf",
    "adversarial/xml/dtd.xml",
    "adversarial/xml/external-resource.html",
    "adversarial/xml/script.html",
    "adversarial/xml/xinclude.xml",
    "adversarial/xml/xxe.xml",
    "golden/csv/representative.csv",
    "golden/docx/minimal.docx",
    "golden/html/representative.html",
    "golden/markdown/representative.md",
    "golden/mineru_artifact_synthetic/layout.json",
    "golden/mineru_artifact_synthetic/middle.json",
    "golden/pdf_native/representative.pdf",
    "golden/pptx/minimal.pptx",
    "golden/text/cr.txt",
    "golden/text/crlf.txt",
    "golden/text/gb18030.txt",
    "golden/text/utf8-bom.txt",
    "golden/text/utf8-nfc.txt",
    "golden/text/utf8-nfd.txt",
    "golden/xlsx/representative.xlsx",
    "recipes/binary-recipes.v1.json",
    "recipes/expectations.v1.json",
}


def _object_at(value: JsonValue, *path: str | int) -> dict[str, JsonValue]:
    current = value
    for part in path:
        if isinstance(part, int):
            assert isinstance(current, list)
            current = current[part]
        else:
            assert isinstance(current, dict)
            current = current[part]
    assert isinstance(current, dict)
    return current


def test_manifest_schema_hashes_and_exact_coverage_are_frozen() -> None:
    manifest = load_manifest()
    entries = validate_corpus_tree(manifest)

    assert {entry.path for entry in entries} == EXPECTED_PATHS
    assert len(entries) == len(EXPECTED_PATHS)
    assert all(entry.raw_bytes > 0 for entry in entries)
    assert all(re.fullmatch(r"[0-9a-f]{64}", entry.raw_sha256) for entry in entries)


def test_every_entry_has_redistributable_license_and_synthetic_provenance() -> None:
    manifest = load_manifest()
    entries = manifest_entries(manifest)
    expectations = load_expectations(manifest)

    for entry in entries:
        expected_kind = (
            "generated" if entry.recipe_path is not None else "project_synthetic"
        )
        assert entry.raw["provenance"] == {"kind": expected_kind}
    assert len(expectations) == len(entries) - 2


def test_expected_routes_and_errors_cover_success_and_adversarial_cases() -> None:
    entries = load_expectations(load_manifest())

    expected_errors = {
        "adversarial/archive/case-collision.docx": "INPUT_INVALID",
        "adversarial/archive/duplicate-entry.docx": "INPUT_INVALID",
        "adversarial/archive/encrypted-entry.docx": "INPUT_INVALID",
        "adversarial/archive/header-drift.docx": "INPUT_INVALID",
        "adversarial/archive/nested-archive.docx": "ARCHIVE_LIMIT_EXCEEDED",
        "adversarial/archive/non-nfc-name.docx": "INPUT_INVALID",
        "adversarial/archive/traversal.docx": "INPUT_INVALID",
        "adversarial/archive/zip-bomb.docx": "ARCHIVE_LIMIT_EXCEEDED",
        "adversarial/encoding/ambiguous.bin": "ENCODING_AMBIGUOUS",
        "adversarial/encoding/invalid-utf8.txt": "ENCODING_AMBIGUOUS",
        "adversarial/encoding/nul.txt": "INPUT_INVALID",
        "adversarial/encoding/replacement-character.txt": "INPUT_INVALID",
        "adversarial/limits/entry-count-10001.docx": "ARCHIVE_LIMIT_EXCEEDED",
        "adversarial/limits/mineru-missing-structure.json": "PARSER_SCHEMA_MISMATCH",
        "adversarial/limits/mineru-page-mismatch.json": "QUALITY_LOCATOR_FAILED",
        "adversarial/limits/oversized-cell.xlsx": "INPUT_INVALID",
        "adversarial/limits/page-501.pdf": "PAGE_LIMIT_EXCEEDED",
        "adversarial/limits/path-513.docx": "ARCHIVE_LIMIT_EXCEEDED",
        "adversarial/ooxml/macro.docm": "ACTIVE_CONTENT_UNSUPPORTED",
        "adversarial/ooxml/missing-part.docx": "INPUT_INVALID",
        "adversarial/ooxml/ole.docx": "ACTIVE_CONTENT_UNSUPPORTED",
        "adversarial/ooxml/xxe.docx": "INPUT_INVALID",
        "adversarial/pdf/complex-layout.pdf": "MINERU_REQUIRED",
        "adversarial/pdf/encrypted.pdf": "PDF_ENCRYPTED_UNSUPPORTED",
        "adversarial/pdf/mixed.pdf": "MINERU_REQUIRED",
        "adversarial/pdf/scanned.pdf": "MINERU_REQUIRED",
        "adversarial/pdf/truncated.pdf": "INPUT_INVALID",
        "adversarial/xml/dtd.xml": "INPUT_INVALID",
        "adversarial/xml/script.html": "INPUT_INVALID",
        "adversarial/xml/xinclude.xml": "INPUT_INVALID",
        "adversarial/xml/xxe.xml": "INPUT_INVALID",
    }
    actual_errors = {
        path: raw["expectedError"]
        for path, raw in entries.items()
        if raw["expectedError"] is not None
    }
    assert actual_errors == expected_errors

    success_routes = {
        path: raw["expectedRoute"]
        for path, raw in entries.items()
        if raw["expectedError"] is None
    }
    assert set(success_routes.values()) == {
        "csv",
        "docx",
        "html",
        "markdown",
        "pdf",
        "pptx",
        "synthetic_mineru_artifact",
        "txt",
        "xlsx",
    }
    assert success_routes["adversarial/ooxml/external-rel.docx"] == "docx"
    assert success_routes["adversarial/xml/external-resource.html"] == "html"
    assert success_routes["adversarial/limits/page-500.pdf"] == "pdf"


def test_binary_recipes_reproduce_every_declared_output_byte_for_byte() -> None:
    manifest = load_manifest()
    recipe_outputs = {
        entry.path
        for entry in manifest_entries(manifest)
        if entry.recipe_path is not None
    }

    assert len(recipe_outputs) == 27
    assert all(
        path.rsplit(".", 1)[-1] in {"docm", "docx", "pdf", "pptx", "xlsx"}
        for path in recipe_outputs
    )
    validate_recipe_identity(manifest)


def test_representative_text_and_markup_preserve_raw_encoding_boundaries() -> None:
    text_root = CORPUS_ROOT / "golden/text"

    assert (text_root / "utf8-nfc.txt").read_text() == "UTF-8 NFC: café — 中文 — 😀\n"
    assert (text_root / "utf8-nfd.txt").read_text() == (
        "UTF-8 NFD: cafe\u0301 — A\u030a\n"
    )
    assert (text_root / "crlf.txt").read_bytes().count(b"\r\n") == 2
    assert b"\n" not in (text_root / "cr.txt").read_bytes()
    assert (text_root / "utf8-bom.txt").read_bytes().startswith(b"\xef\xbb\xbf")
    assert (text_root / "gb18030.txt").read_bytes().decode("gb18030") == (
        "GB18030: 中文\n"
    )

    markdown = (CORPUS_ROOT / "golden/markdown/representative.md").read_text()
    markdown_tokens = ("# Corpus", "- first", "| key |", "```python", "<span")
    assert all(token in markdown for token in markdown_tokens)
    html = (CORPUS_ROOT / "golden/html/representative.html").read_text()
    assert all(token in html for token in ("<h1>", "<ul>", "<table>", "<pre>"))
    csv = (CORPUS_ROOT / "golden/csv/representative.csv").read_bytes()
    assert csv.count(b"\r\n") == 3
    assert b'"comma, and ""quote"""' in csv


@pytest.mark.parametrize(
    ("raw", "message"),
    [
        (b'{"value":1.0}', "floating-point"),
        (b'{"value":1,"value":2}', "duplicate"),
        (b'\xef\xbb\xbf{"value":1}', "BOM"),
        (b'{"value":"a\x00b"}', "NUL"),
        (b'{"value":"\\ud800"}', "surrogate"),
        (b'{"value":9007199254740992}', "unsafe"),
        (b'{ "value":1}', "canonical JCS"),
        (b'{"z":null,"a":true}', "canonical JCS"),
    ],
)
def test_contract_json_rejects_ambiguous_or_noncanonical_bytes(
    raw: bytes, message: str
) -> None:
    with pytest.raises(CorpusValidationError, match=message):
        parse_canonical_json(raw, label="negative vector")


def test_contract_json_accepts_only_profile_scalars_at_safe_boundaries() -> None:
    value = {
        "bool": True,
        "max": 9007199254740991,
        "min": -9007199254740991,
        "null": None,
        "unicode": "文😀",
    }
    raw = canonical_json_bytes(value)

    assert parse_canonical_json(raw, label="positive vector") == value


def test_path_escape_symlink_and_case_collisions_are_rejected(tmp_path: Path) -> None:
    manifest_file = tmp_path / "manifest.v1.json"
    manifest_file.write_bytes(b"{}")
    outside = tmp_path.parent / "outside-corpus-fixture"
    outside.write_bytes(b"outside")
    try:
        escape = {
            "entries": [
                {
                    "path": "../outside-corpus-fixture",
                    "rawBytes": 7,
                    "rawSha256": hashlib.sha256(b"outside").hexdigest(),
                    "recipePath": None,
                    "role": "fixture",
                }
            ]
        }
        with pytest.raises(CorpusValidationError, match="escapes"):
            validate_corpus_tree(escape, tmp_path)

        symlink = tmp_path / "linked"
        symlink.symlink_to(outside)
        with pytest.raises(CorpusValidationError, match="symlink"):
            validate_corpus_tree({"entries": []}, tmp_path)
        symlink.unlink()

        for relative in ("Case/a", "case/b"):
            path = tmp_path / relative
            path.parent.mkdir(exist_ok=True)
            path.write_bytes(relative.encode())
        collision_entries = [
            {
                "path": relative,
                "rawBytes": len(relative),
                "rawSha256": hashlib.sha256(relative.encode()).hexdigest(),
                "recipePath": None,
                "role": "fixture",
            }
            for relative in ("Case/a", "case/b")
        ]
        with pytest.raises(CorpusValidationError, match="case-folding path collision"):
            validate_corpus_tree({"entries": collision_entries}, tmp_path)
    finally:
        outside.unlink(missing_ok=True)


@pytest.mark.parametrize(
    ("relative", "required_parts"),
    [
        (
            "golden/docx/minimal.docx",
            {"[Content_Types].xml", "_rels/.rels", "word/document.xml"},
        ),
        (
            "golden/pptx/minimal.pptx",
            {
                "[Content_Types].xml",
                "_rels/.rels",
                "ppt/presentation.xml",
                "ppt/slides/slide1.xml",
                "ppt/slideMasters/slideMaster1.xml",
                "ppt/slideLayouts/slideLayout1.xml",
                "ppt/theme/theme1.xml",
            },
        ),
        (
            "golden/xlsx/representative.xlsx",
            {
                "[Content_Types].xml",
                "_rels/.rels",
                "xl/workbook.xml",
                "xl/worksheets/sheet1.xml",
                "xl/worksheets/sheet2.xml",
                "xl/sharedStrings.xml",
                "xl/styles.xml",
            },
        ),
    ],
)
def test_golden_ooxml_packages_are_openable_and_all_xml_parts_are_well_formed(
    relative: str, required_parts: set[str]
) -> None:
    with zipfile.ZipFile(CORPUS_ROOT / relative) as archive:
        assert archive.testzip() is None
        assert required_parts <= set(archive.namelist())
        for name in archive.namelist():
            if name.endswith((".xml", ".rels")):
                # These are fixed golden package parts, never caller-controlled XML.
                ET.fromstring(archive.read(name))  # noqa: S314


def test_xlsx_preserves_formula_cache_merge_and_hidden_structures() -> None:
    with zipfile.ZipFile(CORPUS_ROOT / "golden/xlsx/representative.xlsx") as archive:
        sheet = archive.read("xl/worksheets/sheet1.xml")
        workbook = archive.read("xl/workbook.xml")
        strings = archive.read("xl/sharedStrings.xml")

    assert b"<f>SUM(B1,1)</f><v>2</v>" in sheet
    assert b'<mergeCell ref="A3:B3"/>' in sheet
    assert b'hidden="1"' in sheet
    assert b'state="hidden"' in workbook
    assert b"Minimal XLSX" in strings


def test_docx_table_grid_and_pptx_theme_have_required_open_package_structures() -> None:
    with zipfile.ZipFile(CORPUS_ROOT / "golden/docx/minimal.docx") as archive:
        document = ET.fromstring(archive.read("word/document.xml"))  # noqa: S314
    word_ns = {"w": "http://schemas.openxmlformats.org/wordprocessingml/2006/main"}
    assert document.find(".//w:tbl/w:tblGrid/w:gridCol", word_ns) is not None

    with zipfile.ZipFile(CORPUS_ROOT / "golden/pptx/minimal.pptx") as archive:
        theme = ET.fromstring(archive.read("ppt/theme/theme1.xml"))  # noqa: S314
    drawing_ns = {"a": "http://schemas.openxmlformats.org/drawingml/2006/main"}
    for collection in (
        "fillStyleLst",
        "lnStyleLst",
        "effectStyleLst",
        "bgFillStyleLst",
    ):
        node = theme.find(f".//a:{collection}", drawing_ns)
        assert node is not None
        assert len(node) == 3
    for font in ("majorFont", "minorFont"):
        assert theme.find(f".//a:{font}/a:latin", drawing_ns) is not None
        assert theme.find(f".//a:{font}/a:ea", drawing_ns) is not None
        assert theme.find(f".//a:{font}/a:cs", drawing_ns) is not None


def test_native_and_limit_pdfs_have_self_consistent_xref_and_page_counts() -> None:
    expected_pages = {
        "golden/pdf_native/representative.pdf": 4,
        "adversarial/limits/page-500.pdf": 500,
        "adversarial/limits/page-501.pdf": 501,
    }
    for relative, page_count in expected_pages.items():
        raw = (CORPUS_ROOT / relative).read_bytes()
        startxref = int(raw.rsplit(b"startxref\n", 1)[1].splitlines()[0])
        assert raw[startxref:].startswith(b"xref\n")
        assert raw.endswith(b"%%EOF\n")
        assert raw.count(b"/Type /Page ") == page_count
        xref_lines = raw[startxref:].split(b"trailer\n", 1)[0].splitlines()
        object_count = int(xref_lines[1].split()[1])
        assert len(xref_lines[2:]) == object_count
        for object_number, line in enumerate(xref_lines[3:], start=1):
            offset = int(line.split()[0])
            assert raw[offset:].startswith(f"{object_number} 0 obj\n".encode())

    encrypted = (CORPUS_ROOT / "adversarial/pdf/encrypted.pdf").read_bytes()
    assert b"/Encrypt 5 0 R" in encrypted
    assert b"/Filter /Standard /V 1 /R 2" in encrypted
    truncated = (CORPUS_ROOT / "adversarial/pdf/truncated.pdf").read_bytes()
    assert not truncated.endswith(b"%%EOF\n")


def test_synthetic_mineru_layout_and_middle_are_closed_offline_artifacts() -> None:
    artifact_root = CORPUS_ROOT / "golden/mineru_artifact_synthetic"
    raw_layout = (artifact_root / "layout.json").read_bytes()
    raw_middle = (artifact_root / "middle.json").read_bytes()

    assert raw_layout != raw_middle
    layout = parse_canonical_json(raw_layout, label="synthetic MinerU layout")
    middle = parse_canonical_json(raw_middle, label="synthetic MinerU middle")
    validate_packaged_schema(layout, SYNTHETIC_MINERU_SCHEMA_ID)
    validate_packaged_schema(middle, SYNTHETIC_MINERU_SCHEMA_ID)
    assert isinstance(layout, dict)
    assert isinstance(middle, dict)
    assert layout["role"] == "layout"
    assert middle["role"] == "middle"
    assert {"layout", "middle"}.isdisjoint(layout)
    assert {"layout", "middle"}.isdisjoint(middle)

    expected_summary: JsonValue = {
        "configNamespace": "mm-chat.synthetic-mineru.config.v1",
        "goldenNamespace": "mm-chat.synthetic-mineru.golden.v1",
        "pages": [
            {
                "elements": [
                    {
                        "bboxMilliPoint": [72000, 72000, 540000, 108000],
                        "content": {
                            "level": 1,
                            "text": "Synthetic heading",
                        },
                        "elementId": "heading-0",
                        "elementOrdinal": 0,
                        "kind": "heading",
                    },
                    {
                        "bboxMilliPoint": [72000, 120000, 540000, 180000],
                        "content": {"text": "Synthetic MinerU 文本"},
                        "elementId": "text-0",
                        "elementOrdinal": 1,
                        "kind": "text",
                    },
                    {
                        "bboxMilliPoint": [72000, 200000, 540000, 340000],
                        "content": {
                            "rows": [
                                {
                                    "cells": [
                                        {
                                            "columnIndex": 0,
                                            "columnSpan": 1,
                                            "rowSpan": 1,
                                            "text": "key",
                                        },
                                        {
                                            "columnIndex": 1,
                                            "columnSpan": 1,
                                            "rowSpan": 1,
                                            "text": "value",
                                        },
                                    ],
                                    "rowIndex": 0,
                                },
                                {
                                    "cells": [
                                        {
                                            "columnIndex": 0,
                                            "columnSpan": 1,
                                            "rowSpan": 1,
                                            "text": "café",
                                        },
                                        {
                                            "columnIndex": 1,
                                            "columnSpan": 1,
                                            "rowSpan": 1,
                                            "text": "中文",
                                        },
                                    ],
                                    "rowIndex": 1,
                                },
                            ]
                        },
                        "elementId": "table-0",
                        "elementOrdinal": 2,
                        "kind": "table",
                    },
                    {
                        "bboxMilliPoint": [72000, 360000, 300000, 400000],
                        "content": {"sourceText": "E = mc²"},
                        "elementId": "formula-0",
                        "elementOrdinal": 3,
                        "kind": "formula",
                    },
                ],
                "heightMilliPoint": 792000,
                "pageIndex": 0,
                "widthMilliPoint": 612000,
            }
        ],
        "profile": {
            "bboxConvention": "zero-based-half-open-xyxy",
            "coordinateOrigin": "top-left",
            "coordinateUnit": "milli-point",
            "pageIndexBase": 0,
            "profileId": "mm-chat.synthetic-mineru.offline-profile.v1",
        },
        "source": {
            "documentId": "offline-mineru-document-v1",
            "kind": "project_synthetic",
        },
        "summaryKind": "synthetic-mineru-pair-semantic-summary.v1",
        "testOnly": True,
    }
    assert validate_synthetic_mineru_pair([layout, middle]) == expected_summary
    assert validate_synthetic_mineru_pair([middle, layout]) == expected_summary
    assert canonical_json_bytes(expected_summary) == canonical_json_bytes(
        validate_synthetic_mineru_pair([middle, layout])
    )


def test_synthetic_mineru_missing_role_and_page_bbox_mismatches_fail() -> None:
    layout = parse_canonical_json(
        (CORPUS_ROOT / "golden/mineru_artifact_synthetic/layout.json").read_bytes(),
        label="layout",
    )
    middle = parse_canonical_json(
        (CORPUS_ROOT / "golden/mineru_artifact_synthetic/middle.json").read_bytes(),
        label="middle",
    )
    missing = parse_canonical_json(
        (CORPUS_ROOT / "adversarial/limits/mineru-missing-structure.json").read_bytes(),
        label="missing role",
    )
    with pytest.raises(CorpusValidationError, match="'role' is a required property"):
        validate_packaged_schema(missing, SYNTHETIC_MINERU_SCHEMA_ID)
    with pytest.raises(CorpusValidationError, match="schema validation"):
        validate_synthetic_mineru_pair([layout, missing])

    mismatch = parse_canonical_json(
        (CORPUS_ROOT / "adversarial/limits/mineru-page-mismatch.json").read_bytes(),
        label="page mismatch",
    )
    validate_packaged_schema(mismatch, SYNTHETIC_MINERU_SCHEMA_ID)
    with pytest.raises(CorpusValidationError, match="page set mismatch"):
        validate_synthetic_mineru_pair([layout, mismatch])

    assert isinstance(mismatch, dict)
    bbox_mismatch = deepcopy(mismatch)
    _object_at(bbox_mismatch, "pages", 0)["pageIndex"] = 0
    with pytest.raises(CorpusValidationError, match="bbox mismatch"):
        validate_synthetic_mineru_pair([layout, bbox_mismatch])

    with pytest.raises(CorpusValidationError, match="exactly one"):
        validate_synthetic_mineru_pair([layout])
    with pytest.raises(CorpusValidationError, match="duplicate.*layout"):
        validate_synthetic_mineru_pair([layout, layout])

    assert isinstance(middle, dict)
    source_mismatch = deepcopy(middle)
    _object_at(source_mismatch, "source")["documentId"] = "different-document-v1"
    with pytest.raises(CorpusValidationError, match="source mismatch"):
        validate_synthetic_mineru_pair([layout, source_mismatch])

    profile_mismatch = deepcopy(middle)
    _object_at(profile_mismatch, "profile")["profileId"] = "different-profile-v1"
    with pytest.raises(CorpusValidationError, match="profile mismatch"):
        validate_synthetic_mineru_pair([layout, profile_mismatch])

    geometry_mismatch = deepcopy(middle)
    _object_at(geometry_mismatch, "pages", 0)["widthMilliPoint"] = 611999
    with pytest.raises(CorpusValidationError, match="page geometry mismatch"):
        validate_synthetic_mineru_pair([layout, geometry_mismatch])


def test_corpus_contains_no_fabricated_canonical_ir_or_golden_output() -> None:
    forbidden_names = {"canonical-ir.json", "canonical.json", "chunks.json"}
    actual_names = {path.name for path in CORPUS_ROOT.rglob("*") if path.is_file()}

    assert forbidden_names.isdisjoint(actual_names)
    assert not any("canonical_ir" in path for path in EXPECTED_PATHS)
