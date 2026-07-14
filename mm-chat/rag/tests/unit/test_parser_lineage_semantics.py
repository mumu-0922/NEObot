from __future__ import annotations

from copy import deepcopy
from pathlib import Path
from typing import cast

import pytest
from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    load_packaged_schemas,
    load_strict_json_bytes,
)
from tests.support.parser_lineage_semantics import (
    LineageSemanticsError,
    validate_parser_lineage_semantics,
)

_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "lineage"
    / "integrated"
)
_FIXTURE_NAMES = (
    "canonical-ir.v2.json",
    "chunk-manifest.v2.json",
    "canonical-manifest.v2.json",
)


@pytest.fixture(scope="module")
def packaged_schemas() -> PackagedSchemas:
    return load_packaged_schemas()


@pytest.fixture
def artifacts() -> tuple[JsonObject, JsonObject, JsonObject]:
    canonical_ir, chunk_manifest, canonical_manifest = (
        _load_object(name) for name in _FIXTURE_NAMES
    )
    return canonical_ir, chunk_manifest, canonical_manifest


def _load_object(name: str) -> JsonObject:
    content = (_FIXTURE_ROOT / name).read_bytes()
    value = load_strict_json_bytes(content)
    assert canonical_json_bytes(value) == content
    assert isinstance(value, dict)
    return value


def _object(value: JsonValue, *path: str | int) -> JsonObject:
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


def _array(value: JsonValue, *path: str | int) -> list[JsonValue]:
    current = value
    for part in path:
        if isinstance(part, int):
            assert isinstance(current, list)
            current = current[part]
        else:
            assert isinstance(current, dict)
            current = current[part]
    assert isinstance(current, list)
    return current


def _apply_mutation(
    mutation: str,
    canonical_ir: JsonObject,
    chunk_manifest: JsonObject,
    canonical_manifest: JsonObject,
) -> None:
    if _apply_ir_mutation(mutation, canonical_ir):
        return
    if _apply_chunk_mutation(mutation, chunk_manifest):
        return
    if _apply_manifest_mutation(mutation, canonical_manifest):
        return
    raise AssertionError(f"unknown mutation: {mutation}")


def _apply_ir_mutation(mutation: str, canonical_ir: JsonObject) -> bool:
    blocks = _array(canonical_ir, "blocks")
    if mutation == "missing-parent-reference":
        _object(canonical_ir, "blocks", 1)["parentBlockId"] = "f" * 64
    elif mutation == "block-parent-cycle":
        heading_id = _object(canonical_ir, "blocks", 0)["logicalBlockId"]
        paragraph_id = _object(canonical_ir, "blocks", 1)["logicalBlockId"]
        _object(canonical_ir, "blocks", 0)["parentBlockId"] = paragraph_id
        _object(canonical_ir, "blocks", 1)["parentBlockId"] = heading_id
    elif mutation == "forward-parent-reference":
        later_block_id = _object(canonical_ir, "blocks", 2)["logicalBlockId"]
        _object(canonical_ir, "blocks", 1)["parentBlockId"] = later_block_id
    elif mutation == "block-array-order":
        blocks[1], blocks[2] = blocks[2], blocks[1]
    elif mutation == "flow-block-order":
        flow_ids = _array(
            canonical_ir,
            "readingFlows",
            0,
            "orderedLogicalBlockIds",
        )
        flow_ids[1], flow_ids[2] = flow_ids[2], flow_ids[1]
    elif mutation == "page-ordinal":
        _object(canonical_ir, "pages", 0)["pageIndex"] = 1
    elif mutation == "entity-ordinal":
        _object(canonical_ir, "formulas", 0)["formulaOrdinal"] = 1
    elif mutation == "structure-kind-variant":
        _object(canonical_ir, "blocks", 2, "structureRef")["structureKind"] = (
            "paragraph"
        )
    elif mutation == "table-grid-overlap":
        _object(canonical_ir, "tables", 0, "cells", 1)["columnIndex"] = 0
    else:
        return False
    return True


def _apply_chunk_mutation(mutation: str, chunk_manifest: JsonObject) -> bool:
    if mutation == "chunk-span-count":
        span_count = chunk_manifest["spanCount"]
        assert type(span_count) is int
        chunk_manifest["spanCount"] = span_count + 1
    elif mutation == "child-ordinal":
        _object(chunk_manifest, "children", 1)["childOrdinal"] = 2
    elif mutation == "chunk-joiner-cardinality":
        _array(chunk_manifest, "children", 0, "joiners").pop()
    elif mutation == "window-overlap-adjacency":
        overlap = _object(chunk_manifest, "children", 1, "spanFragments", 1)
        assert overlap["fragmentKind"] == "window_overlap"
        overlap["previousChildOrdinal"] = 1
    elif mutation == "chunk-empty-range":
        fragment = _object(chunk_manifest, "children", 0, "spanFragments", 1)
        fragment["blockEndByte"] = fragment["blockStartByte"]
    elif mutation == "chunk-content-hash":
        _object(chunk_manifest, "children", 0)["contentHash"] = "0" * 64
    elif mutation == "missing-chunk-parent-reference":
        _object(chunk_manifest, "children", 0)["logicalParentChunkId"] = "f" * 64
    else:
        return False
    return True


def _apply_manifest_mutation(
    mutation: str,
    canonical_manifest: JsonObject,
) -> bool:
    if mutation == "entity-aggregate-hash":
        _object(canonical_manifest, "entityAggregateHashes")["blocks"] = "0" * 64
    elif mutation == "artifact-descriptor-hash":
        _object(canonical_manifest, "canonicalIr")["sha256"] = "0" * 64
    elif mutation == "schema-descriptor-hash":
        _object(canonical_manifest, "schemaHashes")["canonicalIr"] = "0" * 64
    elif mutation == "resolver-entry-count":
        _object(canonical_manifest, "sourceUnitResolver")["entryCount"] = 2
    else:
        return False
    return True


def test_integrated_lineage_fixture_is_canonical_and_semantically_complete(
    packaged_schemas: PackagedSchemas,
    artifacts: tuple[JsonObject, JsonObject, JsonObject],
) -> None:
    assert {path.name for path in _FIXTURE_ROOT.glob("*.json")} == set(_FIXTURE_NAMES)
    canonical_ir, chunk_manifest, canonical_manifest = artifacts

    validate_parser_lineage_semantics(
        packaged_schemas,
        canonical_ir,
        chunk_manifest,
        canonical_manifest,
    )

    block_types = {
        block["blockType"]
        for block in cast("list[JsonObject]", _array(canonical_ir, "blocks"))
    }
    assert {"heading", "paragraph", "table", "formula", "asset_ref"} <= block_types
    assert _array(canonical_ir, "pages")
    assert _array(canonical_ir, "readingFlows")
    assert _array(canonical_ir, "tables", 0, "cells")
    assert _array(canonical_ir, "formulas")
    assert _array(canonical_ir, "assets")
    assert _array(canonical_ir, "provenance")

    source_span_kinds = {
        _object(canonical_ir, "blocks", index, "sourceSpanHash")["kind"]
        for index in range(len(_array(canonical_ir, "blocks")))
    }
    assert source_span_kinds == {"text", "structural"}
    assert _array(chunk_manifest, "parents")
    assert _array(chunk_manifest, "children")
    fragment_kinds = {
        fragment["fragmentKind"]
        for chunk_kind in ("parents", "children")
        for chunk in cast("list[JsonObject]", _array(chunk_manifest, chunk_kind))
        for fragment in cast(
            "list[JsonObject]",
            _array(chunk, "spanFragments"),
        )
    }
    assert fragment_kinds == {"primary", "window_overlap", "derived_context"}


@pytest.mark.parametrize(
    ("mutation", "message"),
    [
        pytest.param(
            "missing-parent-reference",
            "missing reference",
            id="missing-ir-reference",
        ),
        pytest.param(
            "missing-chunk-parent-reference",
            "missing logical parent",
            id="missing-chunk-reference",
        ),
        pytest.param("block-parent-cycle", "cycle", id="cycle"),
        pytest.param(
            "forward-parent-reference",
            "completed ancestor",
            id="ancestor-only",
        ),
        pytest.param("block-array-order", "contiguous", id="array-order"),
        pytest.param("flow-block-order", "block order", id="order"),
        pytest.param("page-ordinal", "contiguous", id="page-ordinal"),
        pytest.param("entity-ordinal", "contiguous", id="entity-ordinal"),
        pytest.param("structure-kind-variant", "structure kind", id="variant"),
        pytest.param("table-grid-overlap", "overlap", id="table-overlap"),
        pytest.param("chunk-span-count", "spanCount", id="count"),
        pytest.param("child-ordinal", "contiguous", id="child-ordinal"),
        pytest.param(
            "chunk-joiner-cardinality",
            "fragment n-1",
            id="joiner",
        ),
        pytest.param(
            "window-overlap-adjacency",
            "adjacent previous child",
            id="window-overlap",
        ),
        pytest.param("chunk-empty-range", "empty range", id="empty-range"),
        pytest.param("chunk-content-hash", "contentHash", id="chunk-hash"),
        pytest.param(
            "entity-aggregate-hash",
            "entityAggregateHashes.blocks",
            id="aggregate-hash",
        ),
        pytest.param(
            "artifact-descriptor-hash",
            "canonicalIr.sha256",
            id="artifact-hash",
        ),
        pytest.param(
            "schema-descriptor-hash",
            "schemaHashes.canonicalIr",
            id="schema-hash",
        ),
        pytest.param(
            "resolver-entry-count",
            "sourceUnitResolver.entryCount",
            id="resolver-count",
        ),
    ],
)
def test_schema_valid_lineage_mutations_fail_closed(
    packaged_schemas: PackagedSchemas,
    artifacts: tuple[JsonObject, JsonObject, JsonObject],
    mutation: str,
    message: str,
) -> None:
    canonical_ir, chunk_manifest, canonical_manifest = deepcopy(artifacts)
    _apply_mutation(
        mutation,
        canonical_ir,
        chunk_manifest,
        canonical_manifest,
    )

    with pytest.raises(LineageSemanticsError, match=message):
        validate_parser_lineage_semantics(
            packaged_schemas,
            canonical_ir,
            chunk_manifest,
            canonical_manifest,
        )


def test_schema_validation_precedes_semantic_validation(
    packaged_schemas: PackagedSchemas,
    artifacts: tuple[JsonObject, JsonObject, JsonObject],
) -> None:
    canonical_ir, chunk_manifest, canonical_manifest = deepcopy(artifacts)
    canonical_ir["runtimePath"] = "/forbidden"
    _object(canonical_ir, "blocks", 1)["parentBlockId"] = "f" * 64

    with pytest.raises(ValidationError):
        validate_parser_lineage_semantics(
            packaged_schemas,
            canonical_ir,
            chunk_manifest,
            canonical_manifest,
        )
