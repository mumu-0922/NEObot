"""Strict, deterministic helpers for the checked-in offline parser corpus."""

# XML package parts are deliberately kept as visible exact literals. Splitting
# every tag to satisfy line length would make byte review materially harder.
# PDF Standard Security Handler revision 2 itself mandates MD5/RC4; this module
# creates a rejection fixture and never uses either primitive for trust.
# ruff: noqa: E501, PLC0415

from __future__ import annotations

import hashlib
import json
import struct
import unicodedata
import warnings
import zipfile
from collections.abc import Callable, Iterable, Mapping, Sequence
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path, PurePosixPath
from typing import Final, cast

import rfc8785

type JsonScalar = None | bool | str | int
type JsonValue = JsonScalar | list[JsonValue] | dict[str, JsonValue]

SAFE_INTEGER_MAX: Final = (1 << 53) - 1
CORPUS_ROOT: Final = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
MANIFEST_PATH: Final = CORPUS_ROOT / "manifest.v1.json"
EXPECTATIONS_PATH: Final = CORPUS_ROOT / "recipes" / "expectations.v1.json"
MANIFEST_SCHEMA_ID: Final = (
    "https://schemas.mm-chat.invalid/parser/parser-corpus-manifest.v1.schema.json"
)
SYNTHETIC_MINERU_SCHEMA_ID: Final = (
    "https://schemas.mm-chat.invalid/parser/synthetic-mineru-artifact.v1.schema.json"
)
AGGREGATE_DOMAIN: Final = b"mm-chat.parser-corpus-manifest.v1\n"
CORPUS_EXPECTED_ERROR_CODES: Final = frozenset(
    {
        "ACTIVE_CONTENT_UNSUPPORTED",
        "ARCHIVE_LIMIT_EXCEEDED",
        "ENCODING_AMBIGUOUS",
        "INPUT_INVALID",
        "MINERU_REQUIRED",
        "PAGE_LIMIT_EXCEEDED",
        "PARSER_SCHEMA_MISMATCH",
        "PDF_ENCRYPTED_UNSUPPORTED",
        "QUALITY_LOCATOR_FAILED",
    }
)
_ZIP_TIMESTAMP: Final = (1980, 1, 1, 0, 0, 0)
_PDF_BINARY_MARKER: Final = b"%\xe2\xe3\xcf\xd3\n"


class CorpusValidationError(ValueError):
    """Raised when checked-in corpus bytes violate the frozen test contract."""


@dataclass(frozen=True)
class CorpusEntry:
    """One immutable manifest entry with its exact checked-in bytes."""

    path: str
    role: str
    raw_bytes: int
    raw_sha256: str
    recipe_path: str | None
    raw: Mapping[str, JsonValue]


def _reject_float(value: str) -> None:
    raise CorpusValidationError(f"floating-point JSON number is forbidden: {value}")


def _reject_constant(value: str) -> None:
    raise CorpusValidationError(f"non-finite JSON number is forbidden: {value}")


def _safe_integer(value: str) -> int:
    parsed = int(value)
    if not -SAFE_INTEGER_MAX <= parsed <= SAFE_INTEGER_MAX:
        raise CorpusValidationError(f"unsafe JSON integer is forbidden: {value}")
    return parsed


def _closed_object(pairs: list[tuple[str, JsonValue]]) -> dict[str, JsonValue]:
    result: dict[str, JsonValue] = {}
    for key, value in pairs:
        if key in result:
            raise CorpusValidationError(f"duplicate JSON key is forbidden: {key}")
        result[key] = value
    return result


def _validate_scalar_strings(value: JsonValue, location: str = "$") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            _validate_unicode_scalar_string(key, f"{location}.<key>")
            _validate_scalar_strings(child, f"{location}.{key}")
        return
    if isinstance(value, list):
        for index, child in enumerate(value):
            _validate_scalar_strings(child, f"{location}[{index}]")
        return
    if isinstance(value, str):
        _validate_unicode_scalar_string(value, location)
        return
    if value is None or isinstance(value, bool):
        return
    if type(value) is int and -SAFE_INTEGER_MAX <= value <= SAFE_INTEGER_MAX:
        return
    raise CorpusValidationError(f"unsupported JSON scalar at {location}")


def _validate_unicode_scalar_string(value: str, location: str) -> None:
    if "\x00" in value:
        raise CorpusValidationError(f"NUL is forbidden in JSON string at {location}")
    if any(0xD800 <= ord(char) <= 0xDFFF for char in value):
        raise CorpusValidationError(
            f"surrogate is forbidden in JSON string at {location}"
        )


def canonical_json_bytes(value: JsonValue) -> bytes:
    """Encode contract JSON as RFC 8785 after enforcing the scalar profile."""

    _validate_scalar_strings(value)
    try:
        return rfc8785.dumps(value)
    except (FloatDomainError, IntegerDomainError) as exc:  # pragma: no cover
        raise CorpusValidationError(
            "JSON value is outside the contract profile"
        ) from exc


def parse_canonical_json(raw: bytes, *, label: str) -> JsonValue:
    """Parse exact JCS bytes while rejecting ambiguous cross-runtime JSON."""

    if raw.startswith(b"\xef\xbb\xbf"):
        raise CorpusValidationError(f"{label} has a forbidden UTF-8 BOM")
    if b"\x00" in raw:
        raise CorpusValidationError(f"{label} has a forbidden NUL byte")
    try:
        text = raw.decode("utf-8", errors="strict")
        parsed = json.loads(
            text,
            object_pairs_hook=_closed_object,
            parse_float=_reject_float,
            parse_int=_safe_integer,
            parse_constant=_reject_constant,
        )
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise CorpusValidationError(f"{label} is not strict UTF-8 JSON") from exc
    value = cast("JsonValue", parsed)
    _validate_scalar_strings(value)
    if canonical_json_bytes(value) != raw:
        raise CorpusValidationError(f"{label} is not canonical JCS")
    return value


def _mapping(value: JsonValue, label: str) -> dict[str, JsonValue]:
    if not isinstance(value, dict):
        raise CorpusValidationError(f"{label} must be an object")
    return value


def _sequence(value: JsonValue, label: str) -> list[JsonValue]:
    if not isinstance(value, list):
        raise CorpusValidationError(f"{label} must be an array")
    return value


def _string(value: JsonValue, label: str) -> str:
    if not isinstance(value, str):
        raise CorpusValidationError(f"{label} must be a string")
    return value


def _integer(value: JsonValue, label: str) -> int:
    if type(value) is not int:
        raise CorpusValidationError(f"{label} must be an integer")
    return value


def _require_fields(
    value: Mapping[str, JsonValue], expected: set[str], label: str
) -> None:
    actual = set(value)
    if actual != expected:
        raise CorpusValidationError(
            f"{label} fields are not closed: expected {sorted(expected)}, "
            f"got {sorted(actual)}"
        )


def validate_relative_path(value: str, *, label: str) -> PurePosixPath:
    """Validate one portable, NFC, corpus-relative POSIX path."""

    _validate_unicode_scalar_string(value, label)
    if (
        not value
        or "\\" in value
        or "//" in value
        or value.endswith("/")
        or unicodedata.normalize("NFC", value) != value
    ):
        raise CorpusValidationError(f"{label} is not a canonical POSIX path")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in {"", ".", ".."} for part in value.split("/")):
        raise CorpusValidationError(f"{label} escapes the corpus root")
    return path


def validate_packaged_schema(instance: JsonValue, schema_id: str) -> None:
    """Validate an instance against the complete in-memory parser registry."""

    from jsonschema import Draft202012Validator
    from referencing import Registry, Resource

    from mm_chat_rag.contracts import read_schema_bytes, schema_names

    resources: dict[str, Resource[dict[str, object]]] = {}
    target_schema: dict[str, object] | None = None
    for name in schema_names():
        raw_schema = json.loads(read_schema_bytes(name))
        if not isinstance(raw_schema, dict) or not isinstance(
            raw_schema.get("$id"), str
        ):
            raise CorpusValidationError(f"invalid packaged schema resource: {name}")
        resource_id = raw_schema["$id"]
        resource = Resource.from_contents(raw_schema)
        resources[resource_id] = resource
        if resource_id == schema_id:
            target_schema = raw_schema
    if target_schema is None:
        raise CorpusValidationError(f"parser schema is not packaged: {schema_id}")
    registry = Registry().with_resources(resources.items())
    validator = Draft202012Validator(target_schema, registry=registry)
    errors = sorted(validator.iter_errors(instance), key=lambda error: list(error.path))
    if errors:
        detail = "; ".join(error.message for error in errors[:3])
        raise CorpusValidationError(f"schema validation failed: {detail}")


def validate_synthetic_mineru_pair(
    artifacts: Sequence[JsonValue],
) -> dict[str, JsonValue]:
    """Validate one layout/middle pair and return a test-only semantic summary.

    The result is deliberately not Canonical IR and does not model a live MinerU
    wire. Artifact order does not affect the summary; page and element order are
    part of the synthetic contract and therefore validated rather than sorted.
    """

    if len(artifacts) != 2:
        raise CorpusValidationError(
            "synthetic MinerU pair requires exactly one layout and one middle role"
        )

    by_role: dict[str, dict[str, JsonValue]] = {}
    for index, value in enumerate(artifacts):
        validate_packaged_schema(value, SYNTHETIC_MINERU_SCHEMA_ID)
        artifact = _mapping(value, f"synthetic MinerU artifact[{index}]")
        role = _string(artifact.get("role"), f"artifact[{index}].role")
        if role in by_role:
            raise CorpusValidationError(f"duplicate synthetic MinerU role: {role}")
        by_role[role] = artifact
    if set(by_role) != {"layout", "middle"}:
        raise CorpusValidationError(
            "synthetic MinerU pair requires exactly one layout and one middle role"
        )

    layout = by_role["layout"]
    middle = by_role["middle"]
    _validate_synthetic_mineru_pair_metadata(layout, middle)

    layout_pages = _synthetic_mineru_pages(layout, "layout")
    middle_pages = _synthetic_mineru_pages(middle, "middle")
    _validate_synthetic_mineru_page_indexes(layout_pages, middle_pages)

    summary_pages: list[JsonValue] = []
    seen_element_ids: set[str] = set()
    for layout_page, middle_page in zip(layout_pages, middle_pages, strict=True):
        page_index = _integer(layout_page["pageIndex"], "pageIndex")
        width = _integer(layout_page["widthMilliPoint"], "widthMilliPoint")
        height = _integer(layout_page["heightMilliPoint"], "heightMilliPoint")
        if (
            middle_page["widthMilliPoint"] != width
            or middle_page["heightMilliPoint"] != height
        ):
            raise CorpusValidationError(
                f"synthetic MinerU pair page geometry mismatch: page {page_index}"
            )

        layout_elements = _synthetic_mineru_elements(
            layout_page, role="layout", page_index=page_index
        )
        middle_elements = _synthetic_mineru_elements(
            middle_page, role="middle", page_index=page_index
        )
        if len(layout_elements) != len(middle_elements):
            raise CorpusValidationError(
                f"synthetic MinerU pair element count mismatch: page {page_index}"
            )

        summary_elements: list[JsonValue] = []
        for expected_ordinal, (layout_element, middle_element) in enumerate(
            zip(layout_elements, middle_elements, strict=True)
        ):
            for role, element in (
                ("layout", layout_element),
                ("middle", middle_element),
            ):
                ordinal = _integer(
                    element["elementOrdinal"],
                    f"{role} page {page_index} elementOrdinal",
                )
                if ordinal != expected_ordinal:
                    raise CorpusValidationError(
                        "synthetic MinerU element ordinals must be contiguous "
                        f"from zero: {role} page {page_index}"
                    )

            element_id = _string(layout_element["elementId"], "elementId")
            if element_id in seen_element_ids:
                raise CorpusValidationError(
                    f"duplicate synthetic MinerU elementId: {element_id}"
                )
            seen_element_ids.add(element_id)
            for field in ("elementOrdinal", "elementId", "kind"):
                if layout_element[field] != middle_element[field]:
                    raise CorpusValidationError(
                        "synthetic MinerU pair element identity mismatch: "
                        f"page {page_index} ordinal {expected_ordinal}"
                    )

            layout_bbox = _synthetic_mineru_bbox(
                layout_element["bboxMilliPoint"],
                width=width,
                height=height,
                label=f"layout page {page_index} element {expected_ordinal}",
            )
            middle_bbox = _synthetic_mineru_bbox(
                middle_element["bboxMilliPoint"],
                width=width,
                height=height,
                label=f"middle page {page_index} element {expected_ordinal}",
            )
            if layout_bbox != middle_bbox:
                raise CorpusValidationError(
                    "synthetic MinerU pair bbox mismatch: "
                    f"page {page_index} ordinal {expected_ordinal}"
                )

            content = {
                key: value
                for key, value in middle_element.items()
                if key
                not in {
                    "bboxMilliPoint",
                    "elementId",
                    "elementOrdinal",
                    "kind",
                }
            }
            summary_elements.append(
                {
                    "bboxMilliPoint": layout_bbox,
                    "content": content,
                    "elementId": element_id,
                    "elementOrdinal": expected_ordinal,
                    "kind": middle_element["kind"],
                }
            )

        summary_pages.append(
            {
                "elements": summary_elements,
                "heightMilliPoint": height,
                "pageIndex": page_index,
                "widthMilliPoint": width,
            }
        )

    return {
        "configNamespace": layout["configNamespace"],
        "goldenNamespace": layout["goldenNamespace"],
        "pages": summary_pages,
        "profile": layout["profile"],
        "source": layout["source"],
        "summaryKind": "synthetic-mineru-pair-semantic-summary.v1",
        "testOnly": True,
    }


def _validate_synthetic_mineru_pair_metadata(
    layout: Mapping[str, JsonValue], middle: Mapping[str, JsonValue]
) -> None:
    for field in (
        "configNamespace",
        "goldenNamespace",
        "source",
        "profile",
    ):
        if layout[field] != middle[field]:
            raise CorpusValidationError(f"synthetic MinerU pair {field} mismatch")


def _validate_synthetic_mineru_page_indexes(
    layout_pages: Sequence[Mapping[str, JsonValue]],
    middle_pages: Sequence[Mapping[str, JsonValue]],
) -> None:
    layout_indexes = [
        _integer(page["pageIndex"], "layout pageIndex") for page in layout_pages
    ]
    middle_indexes = [
        _integer(page["pageIndex"], "middle pageIndex") for page in middle_pages
    ]
    if layout_indexes != middle_indexes:
        raise CorpusValidationError("synthetic MinerU pair page set mismatch")
    if layout_indexes != list(range(len(layout_indexes))):
        raise CorpusValidationError(
            "synthetic MinerU page indexes must be contiguous from zero"
        )


def _synthetic_mineru_pages(
    artifact: Mapping[str, JsonValue], role: str
) -> list[dict[str, JsonValue]]:
    pages = _sequence(artifact.get("pages"), f"{role}.pages")
    return [
        _mapping(page, f"{role}.pages[{index}]") for index, page in enumerate(pages)
    ]


def _synthetic_mineru_elements(
    page: Mapping[str, JsonValue], *, role: str, page_index: int
) -> list[dict[str, JsonValue]]:
    elements = _sequence(page.get("elements"), f"{role} page {page_index}.elements")
    return [
        _mapping(element, f"{role} page {page_index}.elements[{index}]")
        for index, element in enumerate(elements)
    ]


def _synthetic_mineru_bbox(
    value: JsonValue, *, width: int, height: int, label: str
) -> list[JsonValue]:
    coordinates = _sequence(value, f"{label}.bboxMilliPoint")
    if len(coordinates) != 4:
        raise CorpusValidationError(f"{label} bbox must contain four coordinates")
    x1, y1, x2, y2 = (
        _integer(coordinate, f"{label}.bboxMilliPoint[{index}]")
        for index, coordinate in enumerate(coordinates)
    )
    if not (0 <= x1 < x2 <= width and 0 <= y1 < y2 <= height):
        raise CorpusValidationError(
            f"{label} bbox violates top-left half-open page bounds"
        )
    return [x1, y1, x2, y2]


def manifest_aggregate_hash(manifest: Mapping[str, JsonValue]) -> str:
    """Hash the closed manifest payload, excluding only its aggregate hash."""

    payload = dict(manifest)
    payload.pop("aggregateHash", None)
    return hashlib.sha256(
        AGGREGATE_DOMAIN + canonical_json_bytes(cast("JsonValue", payload))
    ).hexdigest()


def load_manifest(path: Path = MANIFEST_PATH) -> dict[str, JsonValue]:
    """Load the canonical corpus manifest and enforce its packaged schema."""

    manifest = _mapping(
        parse_canonical_json(path.read_bytes(), label=path.name), path.name
    )
    validate_packaged_schema(manifest, MANIFEST_SCHEMA_ID)
    expected = _string(manifest.get("aggregateHash"), "manifest.aggregateHash")
    if manifest_aggregate_hash(manifest) != expected:
        raise CorpusValidationError("manifest aggregate hash mismatch")
    return manifest


def manifest_entries(manifest: Mapping[str, JsonValue]) -> tuple[CorpusEntry, ...]:
    """Project schema-validated manifest entries into immutable test records."""

    raw_entries = _sequence(manifest.get("entries"), "manifest.entries")
    entries: list[CorpusEntry] = []
    for index, raw_entry in enumerate(raw_entries):
        entry = _mapping(raw_entry, f"manifest.entries[{index}]")
        recipe_value = entry.get("recipePath")
        recipe_path = (
            None
            if recipe_value is None
            else _string(recipe_value, f"manifest.entries[{index}].recipePath")
        )
        entries.append(
            CorpusEntry(
                path=_string(entry.get("path"), f"manifest.entries[{index}].path"),
                role=_string(entry.get("role"), f"manifest.entries[{index}].role"),
                raw_bytes=_integer(
                    entry.get("rawBytes"), f"manifest.entries[{index}].rawBytes"
                ),
                raw_sha256=_string(
                    entry.get("rawSha256"),
                    f"manifest.entries[{index}].rawSha256",
                ),
                recipe_path=recipe_path,
                raw=entry,
            )
        )
    return tuple(entries)


def load_expectations(
    manifest: Mapping[str, JsonValue], path: Path = EXPECTATIONS_PATH
) -> dict[str, Mapping[str, JsonValue]]:
    """Load the closed route/license ledger transitively hashed by the manifest."""

    document = _mapping(
        parse_canonical_json(path.read_bytes(), label=path.name), path.name
    )
    _require_fields(
        document,
        {"schemaVersion", "license", "entries"},
        "expectation document",
    )
    if document["schemaVersion"] != "parser-corpus-expectations.v1":
        raise CorpusValidationError("unsupported expectation schemaVersion")
    license_record = _mapping(document["license"], "expectation license")
    _require_fields(
        license_record,
        {"redistributionAllowed", "spdxId"},
        "expectation license",
    )
    if license_record != {"redistributionAllowed": True, "spdxId": "MIT"}:
        raise CorpusValidationError("corpus license is not the project MIT license")

    expected_fixture_paths = {
        entry.path for entry in manifest_entries(manifest) if entry.role != "recipe"
    }
    results: dict[str, Mapping[str, JsonValue]] = {}
    allowed_routes = {
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
    for index, raw_expectation in enumerate(
        _sequence(document["entries"], "expectation entries")
    ):
        expectation = _mapping(raw_expectation, f"expectation[{index}]")
        _require_fields(
            expectation,
            {"expectedError", "expectedRoute", "path"},
            f"expectation[{index}]",
        )
        fixture_path = _string(expectation["path"], f"expectation[{index}].path")
        validate_relative_path(fixture_path, label=f"expectation[{index}].path")
        if fixture_path in results:
            raise CorpusValidationError(f"duplicate expectation path: {fixture_path}")
        route = expectation["expectedRoute"]
        error = expectation["expectedError"]
        if route is not None and route not in allowed_routes:
            raise CorpusValidationError(f"unknown expected route: {route}")
        if error is not None and error not in CORPUS_EXPECTED_ERROR_CODES:
            raise CorpusValidationError(f"unknown expected error: {error}")
        if (route is None) == (error is None):
            raise CorpusValidationError(
                f"expectation must select exactly one route or error: {fixture_path}"
            )
        results[fixture_path] = expectation
    if set(results) != expected_fixture_paths:
        raise CorpusValidationError(
            "expectation coverage does not match fixture coverage"
        )
    return results


def _walk_corpus(root: Path) -> set[str]:
    if root.is_symlink():
        raise CorpusValidationError("corpus root cannot be a symlink")
    root = root.resolve(strict=True)
    files: set[str] = set()
    stack = [root]
    seen_casefold: dict[str, str] = {}
    while stack:
        directory = stack.pop()
        for child in directory.iterdir():
            relative = child.relative_to(root).as_posix()
            collision = seen_casefold.setdefault(relative.casefold(), relative)
            if collision != relative:
                raise CorpusValidationError(
                    f"case-folding path collision: {collision!r} and {relative!r}"
                )
            if child.is_symlink():
                raise CorpusValidationError(
                    f"symlink is forbidden in corpus: {relative}"
                )
            if child.is_dir():
                stack.append(child)
            elif child.is_file():
                files.add(relative)
            else:
                raise CorpusValidationError(f"non-regular corpus entry: {relative}")
    return files


def validate_corpus_tree(
    manifest: Mapping[str, JsonValue], root: Path = CORPUS_ROOT
) -> tuple[CorpusEntry, ...]:
    """Verify exact coverage, portable paths, raw sizes, and SHA-256 digests."""

    actual_paths = _walk_corpus(root)
    actual_paths.remove(MANIFEST_PATH.name)
    entries = manifest_entries(manifest)
    entry_paths: set[str] = set()
    path_casefold: dict[str, str] = {}
    for entry in entries:
        relative = validate_relative_path(entry.path, label="manifest entry path")
        if entry.path == MANIFEST_PATH.name:
            raise CorpusValidationError("manifest cannot recursively list itself")
        if entry.path in entry_paths:
            raise CorpusValidationError(f"duplicate manifest path: {entry.path}")
        entry_paths.add(entry.path)
        folded = entry.path.casefold()
        collision = path_casefold.setdefault(folded, entry.path)
        if collision != entry.path:
            raise CorpusValidationError(
                f"case-folding manifest collision: {collision!r} and {entry.path!r}"
            )
        candidate = root.joinpath(*relative.parts)
        raw = candidate.read_bytes()
        if len(raw) != entry.raw_bytes:
            raise CorpusValidationError(f"raw byte size mismatch: {entry.path}")
        if hashlib.sha256(raw).hexdigest() != entry.raw_sha256:
            raise CorpusValidationError(f"raw SHA-256 mismatch: {entry.path}")

    if actual_paths != entry_paths:
        missing = sorted(actual_paths - entry_paths)
        absent = sorted(entry_paths - actual_paths)
        raise CorpusValidationError(
            f"manifest coverage mismatch: unlisted={missing}, missing={absent}"
        )
    return entries


def _zip_info(name: str, *, compression: int = zipfile.ZIP_STORED) -> zipfile.ZipInfo:
    info = zipfile.ZipInfo(name, _ZIP_TIMESTAMP)
    info.compress_type = compression
    info.create_system = 3
    info.external_attr = 0o100644 << 16
    return info


def deterministic_zip(
    parts: Iterable[tuple[str, bytes]], *, sort_parts: bool = True
) -> bytes:
    """Build byte-stable ZIP bytes with fixed order, metadata, and permissions."""

    ordered = list(parts)
    if sort_parts:
        ordered.sort(key=lambda item: item[0].encode("utf-8"))
    target = BytesIO()
    with zipfile.ZipFile(target, "w") as archive:
        for name, data in ordered:
            archive.writestr(_zip_info(name), data)
    return target.getvalue()


def _xml(value: str) -> bytes:
    return value.encode("utf-8")


def _minimal_docx_parts(
    *,
    document_xml: bytes | None = None,
    relationships: bytes | None = None,
) -> list[tuple[str, bytes]]:
    content_types = _xml(
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-'
        'package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-'
        'officedocument.wordprocessingml.document.main+xml"/></Types>'
    )
    root_rels = _xml(
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/'
        '2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>'
    )
    document = document_xml or _xml(
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
        '<w:body><w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>Minimal DOCX</w:t>'
        '</w:r></w:p><w:p><w:r><w:t xml:space="preserve">Unicode: 文档 café</w:t></w:r>'
        '</w:p><w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="2400"/></w:tblGrid>'
        "<w:tr><w:tc><w:p><w:r><w:t>Cell</w:t></w:r></w:p></w:tc></w:tr>"
        '</w:tbl><w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>'
    )
    parts = [
        ("[Content_Types].xml", content_types),
        ("_rels/.rels", root_rels),
        ("word/document.xml", document),
    ]
    if relationships is not None:
        parts.append(("word/_rels/document.xml.rels", relationships))
    return parts


def _minimal_docx() -> bytes:
    return deterministic_zip(_minimal_docx_parts())


def _minimal_pptx() -> bytes:
    parts = {
        "[Content_Types].xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
            '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
            '<Default Extension="xml" ContentType="application/xml"/>'
            '<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.presentationml.presentation.main+xml"/>'
            '<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.presentationml.slide+xml"/>'
            '<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.'
            'openxmlformats-officedocument.presentationml.slideMaster+xml"/>'
            '<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.'
            'openxmlformats-officedocument.presentationml.slideLayout+xml"/>'
            '<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.theme+xml"/></Types>'
        ),
        "_rels/.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>'
        ),
        "ppt/presentation.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" '
            'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">'
            '<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>'
            '<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>'
            '<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>'
            "</p:presentation>"
        ),
        "ppt/_rels/presentation.xml.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>'
            '<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/slide" Target="slides/slide1.xml"/></Relationships>'
        ),
        "ppt/slides/slide1.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" '
            'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">'
            '<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/>'
            '<p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/>'
            '<p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="914400" y="914400"/>'
            '<a:ext cx="10000000" cy="1000000"/></a:xfrm></p:spPr><p:txBody><a:bodyPr/>'
            '<a:lstStyle/><a:p><a:r><a:rPr lang="en-US"/><a:t>Minimal PPTX</a:t></a:r>'
            '<a:endParaRPr lang="en-US"/></a:p></p:txBody></p:sp></p:spTree></p:cSld>'
            "<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>"
        ),
        "ppt/slides/_rels/slide1.xml.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/></Relationships>'
        ),
        "ppt/slideMasters/slideMaster1.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" '
            'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">'
            '<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/>'
            "<p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>"
            '<p:clrMap accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" '
            'accent5="accent5" accent6="accent6" bg1="lt1" bg2="lt2" folHlink="folHlink" '
            'hlink="hlink" tx1="dk1" tx2="dk2"/><p:sldLayoutIdLst>'
            '<p:sldLayoutId id="1" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles>'
            "<p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>"
        ),
        "ppt/slideMasters/_rels/slideMaster1.xml.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>'
            '<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/theme" Target="../theme/theme1.xml"/></Relationships>'
        ),
        "ppt/slideLayouts/slideLayout1.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" '
            'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" type="blank">'
            '<p:cSld name="Blank"><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/>'
            "<p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>"
            "<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>"
        ),
        "ppt/slideLayouts/_rels/slideLayout1.xml.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>'
        ),
        "ppt/theme/theme1.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Minimal">'
            '<a:themeElements><a:clrScheme name="Minimal"><a:dk1><a:sysClr val="windowText" '
            'lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>'
            '<a:dk2><a:srgbClr val="000000"/></a:dk2><a:lt2><a:srgbClr val="FFFFFF"/></a:lt2>'
            '<a:accent1><a:srgbClr val="4472C4"/></a:accent1><a:accent2><a:srgbClr val="ED7D31"/>'
            '</a:accent2><a:accent3><a:srgbClr val="A5A5A5"/></a:accent3><a:accent4><a:srgbClr '
            'val="FFC000"/></a:accent4><a:accent5><a:srgbClr val="5B9BD5"/></a:accent5>'
            '<a:accent6><a:srgbClr val="70AD47"/></a:accent6><a:hlink><a:srgbClr val="0563C1"/>'
            '</a:hlink><a:folHlink><a:srgbClr val="954F72"/></a:folHlink></a:clrScheme>'
            '<a:fontScheme name="Minimal"><a:majorFont><a:latin typeface="Arial"/><a:ea '
            'typeface=""/><a:cs typeface=""/></a:majorFont><a:minorFont><a:latin '
            'typeface="Arial"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>'
            '<a:fmtScheme name="Minimal"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/>'
            '</a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill>'
            '<a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst><a:lnStyleLst>'
            '<a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>'
            '<a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>'
            '<a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>'
            "</a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle>"
            "<a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/>"
            "</a:effectStyle></a:effectStyleLst><a:bgFillStyleLst><a:solidFill>"
            '<a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/>'
            '</a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>'
            "</a:bgFillStyleLst></a:fmtScheme>"
            "</a:themeElements></a:theme>"
        ),
    }
    return deterministic_zip((name, _xml(value)) for name, value in parts.items())


def _representative_xlsx(*, oversized_cell: bool = False) -> bytes:
    shared_value = "X" * 32768 if oversized_cell else "Minimal XLSX"
    parts = {
        "[Content_Types].xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
            '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
            '<Default Extension="xml" ContentType="application/xml"/>'
            '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.spreadsheetml.sheet.main+xml"/>'
            '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.spreadsheetml.worksheet+xml"/>'
            '<Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.spreadsheetml.worksheet+xml"/>'
            '<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.spreadsheetml.sharedStrings+xml"/>'
            '<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-'
            'officedocument.spreadsheetml.styles+xml"/></Types>'
        ),
        "_rels/.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>'
        ),
        "xl/workbook.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
            '<sheets><sheet name="Visible" sheetId="1" r:id="rId1"/>'
            '<sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/></sheets>'
            '<calcPr calcId="191029" fullCalcOnLoad="1"/></workbook>'
        ),
        "xl/_rels/workbook.xml.rels": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/worksheet" Target="worksheets/sheet1.xml"/>'
            '<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/worksheet" Target="worksheets/sheet2.xml"/>'
            '<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/sharedStrings" Target="sharedStrings.xml"/>'
            '<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
            'relationships/styles" Target="styles.xml"/></Relationships>'
        ),
        "xl/worksheets/sheet1.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><v>1</v></c>'
            '<c r="C1"><f>SUM(B1,1)</f><v>2</v></c></row><row r="2" hidden="1">'
            '<c r="A2" t="s"><v>1</v></c></row><row r="3"><c r="A3" t="s"><v>2</v></c>'
            '</row></sheetData><mergeCells count="1"><mergeCell ref="A3:B3"/></mergeCells>'
            "</worksheet>"
        ),
        "xl/worksheets/sheet2.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<sheetData><row r="1"><c r="A1" t="s"><v>1</v></c></row></sheetData></worksheet>'
        ),
        "xl/sharedStrings.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="4" uniqueCount="3">'
            f"<si><t>{shared_value}</t></si><si><t>hidden</t></si><si><t>merged</t></si></sst>"
        ),
        "xl/styles.xml": (
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<fonts count="1"><font><sz val="11"/><name val="Arial"/></font></fonts>'
            '<fills count="2"><fill><patternFill patternType="none"/></fill><fill>'
            '<patternFill patternType="gray125"/></fill></fills><borders count="1"><border/></borders>'
            '<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>'
            '<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>'
            "</styleSheet>"
        ),
    }
    return deterministic_zip((name, _xml(value)) for name, value in parts.items())


def _pdf_literal(value: str) -> bytes:
    return value.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)").encode()


def _assemble_pdf(
    objects: Sequence[bytes], *, trailer_fields: bytes = b"", version: bytes = b"1.4"
) -> bytes:
    result = bytearray(b"%PDF-" + version + b"\n" + _PDF_BINARY_MARKER)
    offsets = [0]
    for number, body in enumerate(objects, start=1):
        offsets.append(len(result))
        result.extend(f"{number} 0 obj\n".encode())
        result.extend(body)
        if not body.endswith(b"\n"):
            result.extend(b"\n")
        result.extend(b"endobj\n")
    xref_offset = len(result)
    result.extend(f"xref\n0 {len(objects) + 1}\n".encode())
    result.extend(b"0000000000 65535 f \n")
    for offset in offsets[1:]:
        result.extend(f"{offset:010d} 00000 n \n".encode())
    result.extend(
        b"trailer\n<< /Size "
        + str(len(objects) + 1).encode()
        + b" /Root 1 0 R"
        + trailer_fields
        + b" >>\nstartxref\n"
        + str(xref_offset).encode()
        + b"\n%%EOF\n"
    )
    return bytes(result)


def _text_pdf(*, page_count: int = 2, rotations: bool = False) -> bytes:
    first_page_object = 5
    page_ids = list(range(first_page_object, first_page_object + page_count))
    kids = b" ".join(f"{page_id} 0 R".encode() for page_id in page_ids)
    stream = b"BT /F1 12 Tf 72 720 Td (Native PDF page) Tj ET\n"
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Count "
        + str(page_count).encode()
        + b" /Kids [ "
        + kids
        + b" ] >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        b"<< /Length "
        + str(len(stream)).encode()
        + b" >>\nstream\n"
        + stream
        + b"endstream",
    ]
    for index in range(page_count):
        rotation = (index % 4) * 90 if rotations else 0
        rotate = f" /Rotate {rotation}".encode() if rotation else b""
        objects.append(
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]"
            + rotate
            + b" /Resources << /Font << /F1 3 0 R >> >> /Contents 4 0 R >>"
        )
    return _assemble_pdf(objects)


def _complex_pdf() -> bytes:
    stream = (
        b"BT /F1 10 Tf 50 740 Td (Left column) Tj 310 0 Td (Right column) Tj ET\n"
        b"50 700 m 560 700 l 560 620 l 50 620 l h S\n"
        b"50 660 m 560 660 l S 300 700 m 300 620 l S\n"
    )
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Count 1 /Kids [5 0 R] >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        b"<< /Length "
        + str(len(stream)).encode()
        + b" >>\nstream\n"
        + stream
        + b"endstream",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Resources << /Font << /F1 3 0 R >> >> /Contents 4 0 R >>",
    ]
    return _assemble_pdf(objects)


def _image_pdf(*, mixed: bool = False) -> bytes:
    image = b"\x80"
    image_stream = (
        b"<< /Type /XObject /Subtype /Image /Width 1 /Height 1 "
        b"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n"
        + image
        + b"\nendstream"
    )
    draw = b"q 100 0 0 100 72 600 cm /Im1 Do Q\n"
    objects: list[bytes] = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"",
        image_stream,
        b"<< /Length "
        + str(len(draw)).encode()
        + b" >>\nstream\n"
        + draw
        + b"endstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ]
    scanned_page_id = 7
    scanned = (
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Resources << /XObject << /Im1 3 0 R >> >> /Contents 4 0 R >>"
    )
    if mixed:
        text = b"BT /F1 12 Tf 72 720 Td (Text page) Tj ET\n"
        objects.append(
            b"<< /Length "
            + str(len(text)).encode()
            + b" >>\nstream\n"
            + text
            + b"endstream"
        )
        objects.append(scanned)
        objects.append(
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
            b"/Resources << /Font << /F1 5 0 R >> >> /Contents 6 0 R >>"
        )
        objects[1] = b"<< /Type /Pages /Count 2 /Kids [8 0 R 7 0 R] >>"
    else:
        objects.append(b"<<>>")
        objects.append(scanned)
        objects[1] = (
            b"<< /Type /Pages /Count 1 /Kids ["
            + str(scanned_page_id).encode()
            + b" 0 R] >>"
        )
    return _assemble_pdf(objects)


_PASSWORD_PADDING: Final = bytes.fromhex(
    "28bf4e5e4e758a4164004e56fffa01082e2e00b6d0683e802f0ca9fe6453697a"
)


def _rc4(key: bytes, value: bytes) -> bytes:
    state = list(range(256))
    cursor = 0
    for index in range(256):
        cursor = (cursor + state[index] + key[index % len(key)]) % 256
        state[index], state[cursor] = state[cursor], state[index]
    output = bytearray()
    left = right = 0
    for byte in value:
        left = (left + 1) % 256
        right = (right + state[left]) % 256
        state[left], state[right] = state[right], state[left]
        output.append(byte ^ state[(state[left] + state[right]) % 256])
    return bytes(output)


def _padded_password(value: bytes) -> bytes:
    return (value + _PASSWORD_PADDING)[:32]


def _legacy_pdf_md5(value: bytes) -> bytes:
    """Implement the mandated PDF R2 digest, never a trust/security digest."""

    return hashlib.new("md5", value, usedforsecurity=False).digest()


def _encrypted_pdf() -> bytes:
    permissions = -4
    document_id = _legacy_pdf_md5(b"mm-chat deterministic encrypted fixture")
    owner_key = _legacy_pdf_md5(_padded_password(b"owner"))[:5]
    owner = _rc4(owner_key, _padded_password(b"fixture"))
    file_key = _legacy_pdf_md5(
        _padded_password(b"fixture")
        + owner
        + struct.pack("<i", permissions)
        + document_id
    )[:5]
    user = _rc4(file_key, _PASSWORD_PADDING)
    plain_stream = b"BT /F1 12 Tf 72 720 Td (Encrypted fixture) Tj ET\n"
    object_key = _legacy_pdf_md5(file_key + b"\x04\x00\x00\x00\x00")[:10]
    encrypted_stream = _rc4(object_key, plain_stream)
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Count 1 /Kids [6 0 R] >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        b"<< /Length "
        + str(len(encrypted_stream)).encode()
        + b" >>\nstream\n"
        + encrypted_stream
        + b"\nendstream",
        b"<< /Filter /Standard /V 1 /R 2 /O <"
        + owner.hex().encode()
        + b"> /U <"
        + user.hex().encode()
        + b"> /P -4 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Resources << /Font << /F1 3 0 R >> >> /Contents 4 0 R >>",
    ]
    trailer = (
        b" /Encrypt 5 0 R /ID [<"
        + document_id.hex().encode()
        + b"><"
        + document_id.hex().encode()
        + b">]"
    )
    return _assemble_pdf(objects, trailer_fields=trailer)


def _patch_zip_flag(raw: bytes, flag: int) -> bytes:
    patched = bytearray(raw)
    local = patched.find(b"PK\x03\x04")
    central = patched.find(b"PK\x01\x02")
    if local < 0 or central < 0:
        raise AssertionError("recipe ZIP signatures are missing")
    struct.pack_into("<H", patched, local + 6, flag)
    struct.pack_into("<H", patched, central + 8, flag)
    return bytes(patched)


def _header_drift_zip() -> bytes:
    raw = bytearray(deterministic_zip([("safe.xml", b"<safe/>")]))
    central = raw.find(b"PK\x01\x02")
    name_offset = central + 46
    raw[name_offset : name_offset + len("safe.xml")] = b"evil.xml"
    return bytes(raw)


def _compressed_bomb_zip() -> bytes:
    target = BytesIO()
    with zipfile.ZipFile(target, "w") as archive:
        archive.writestr(
            _zip_info("word/document.xml", compression=zipfile.ZIP_DEFLATED),
            b"0" * (1024 * 1024),
            compresslevel=9,
        )
    return target.getvalue()


def _macro_docm() -> bytes:
    parts = _minimal_docx_parts()
    parts[0] = (
        "[Content_Types].xml",
        parts[0][1].replace(
            b"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
            b"application/vnd.ms-word.document.macroEnabled.main+xml",
        ),
    )
    parts.append(("word/vbaProject.bin", b"synthetic-vba-marker"))
    return deterministic_zip(parts)


def _ole_docx() -> bytes:
    parts = _minimal_docx_parts()
    parts.append(("word/embeddings/oleObject1.bin", b"synthetic-ole-marker"))
    return deterministic_zip(parts)


def _external_rel_docx() -> bytes:
    relationships = _xml(
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/'
        'relationships/hyperlink" Target="https://example.invalid/never-fetch" TargetMode="External"/>'
        "</Relationships>"
    )
    return deterministic_zip(_minimal_docx_parts(relationships=relationships))


def _missing_part_docx() -> bytes:
    return deterministic_zip(_minimal_docx_parts()[:2])


def _xxe_docx() -> bytes:
    document = _xml(
        '<?xml version="1.0"?><!DOCTYPE w:document [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>'
        '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
        "<w:body><w:p><w:r><w:t>&xxe;</w:t></w:r></w:p></w:body></w:document>"
    )
    return deterministic_zip(_minimal_docx_parts(document_xml=document))


def _many_entries_docx() -> bytes:
    parts = [(f"word/filler/{index:05d}.bin", b"") for index in range(10001)]
    return deterministic_zip(parts)


def _duplicate_entry_zip() -> bytes:
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", UserWarning)
        return deterministic_zip(
            [("word/document.xml", b"first"), ("word/document.xml", b"second")],
            sort_parts=False,
        )


_SIMPLE_RECIPE_BUILDERS: Final[Mapping[str, Callable[[], bytes]]] = {
    "complex_pdf": _complex_pdf,
    "encrypted_pdf": _encrypted_pdf,
    "external_rel_docx": _external_rel_docx,
    "macro_docm": _macro_docm,
    "many_entries_docx": _many_entries_docx,
    "minimal_docx": _minimal_docx,
    "minimal_pptx": _minimal_pptx,
    "missing_part_docx": _missing_part_docx,
    "mixed_pdf": lambda: _image_pdf(mixed=True),
    "native_pdf": lambda: _text_pdf(page_count=4, rotations=True),
    "ole_docx": _ole_docx,
    "oversized_cell_xlsx": lambda: _representative_xlsx(oversized_cell=True),
    "representative_xlsx": _representative_xlsx,
    "scanned_pdf": _image_pdf,
    "truncated_pdf": lambda: _text_pdf(page_count=1)[:-24],
    "xxe_docx": _xxe_docx,
    "zip_bomb": _compressed_bomb_zip,
    "zip_case_collision": lambda: deterministic_zip(
        [("word/A.xml", b"A"), ("word/a.xml", b"a")]
    ),
    "zip_duplicate": _duplicate_entry_zip,
    "zip_encrypted_flag": lambda: _patch_zip_flag(
        deterministic_zip([("word/document.xml", b"x")]), 1
    ),
    "zip_header_drift": _header_drift_zip,
    "zip_long_path": lambda: deterministic_zip([("a" * 509 + ".xml", b"long")]),
    "zip_nested": lambda: deterministic_zip(
        [("word/embedded.zip", deterministic_zip([("payload.txt", b"nested")]))]
    ),
    "zip_non_nfc": lambda: deterministic_zip([("word/cafe\u0301.xml", b"nfd")]),
    "zip_traversal": lambda: deterministic_zip([("../escape.xml", b"escape")]),
}


def _recipe_bytes(kind: str, parameters: Mapping[str, JsonValue]) -> bytes:
    if kind == "page_count_pdf":
        _require_fields(parameters, {"pages"}, "page_count_pdf parameters")
        pages = _integer(parameters.get("pages"), "page_count_pdf pages")
        if pages not in {500, 501}:
            raise CorpusValidationError("page_count_pdf pages must be 500 or 501")
        return _text_pdf(page_count=pages)
    builder = _SIMPLE_RECIPE_BUILDERS.get(kind)
    if builder is None:
        raise CorpusValidationError(f"unknown deterministic recipe kind: {kind}")
    _require_fields(parameters, set(), f"{kind} parameters")
    return builder()


def generate_recipe_outputs(
    recipe_path: Path, root: Path = CORPUS_ROOT
) -> dict[str, bytes]:
    """Generate every declared recipe output without touching the filesystem."""

    document = _mapping(
        parse_canonical_json(recipe_path.read_bytes(), label=recipe_path.name),
        recipe_path.name,
    )
    _require_fields(document, {"schemaVersion", "recipes"}, "recipe document")
    if document["schemaVersion"] != "parser-corpus-recipes.v1":
        raise CorpusValidationError("unsupported recipe schemaVersion")
    outputs: dict[str, bytes] = {}
    for index, raw_recipe in enumerate(
        _sequence(document["recipes"], "recipe document recipes")
    ):
        recipe = _mapping(raw_recipe, f"recipe[{index}]")
        _require_fields(
            recipe, {"kind", "outputPath", "parameters"}, f"recipe[{index}]"
        )
        kind = _string(recipe["kind"], f"recipe[{index}].kind")
        output = _string(recipe["outputPath"], f"recipe[{index}].outputPath")
        parameters = _mapping(recipe["parameters"], f"recipe[{index}].parameters")
        validate_relative_path(output, label=f"recipe[{index}].outputPath")
        if output in outputs:
            raise CorpusValidationError(f"duplicate recipe output: {output}")
        outputs[output] = _recipe_bytes(kind, parameters)
    recipe_relative = recipe_path.relative_to(root).as_posix()
    if recipe_relative in outputs:
        raise CorpusValidationError("recipe cannot overwrite itself")
    return outputs


def validate_recipe_identity(
    manifest: Mapping[str, JsonValue], root: Path = CORPUS_ROOT
) -> None:
    """Regenerate all binary outputs and require exact checked-in byte identity."""

    entries = manifest_entries(manifest)
    expected_by_recipe: dict[str, set[str]] = {}
    for entry in entries:
        if entry.recipe_path is None:
            continue
        validate_relative_path(entry.recipe_path, label="manifest recipePath")
        expected_by_recipe.setdefault(entry.recipe_path, set()).add(entry.path)
    for recipe_relative, expected_outputs in expected_by_recipe.items():
        outputs = generate_recipe_outputs(root / recipe_relative, root)
        if set(outputs) != expected_outputs:
            raise CorpusValidationError(
                f"recipe output coverage mismatch for {recipe_relative}"
            )
        for output_path, generated in outputs.items():
            if (root / output_path).read_bytes() != generated:
                raise CorpusValidationError(
                    f"recipe byte identity mismatch: {output_path}"
                )


# rfc8785 exposes these exceptions from its implementation module but does not
# include them in a stable __all__; aliases keep the catch narrow and typed.
FloatDomainError = rfc8785.FloatDomainError
IntegerDomainError = rfc8785.IntegerDomainError
