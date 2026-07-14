"""Executable cross-artifact semantics for the C1.1 parser contracts.

JSON Schema deliberately stops at local shape.  This module is test-only and
closes the lineage gap between Canonical IR v2, Chunk Manifest v2, and the
Canonical Manifest v2 ledger.  Validation is fail-closed and always applies
the packaged schemas before inspecting cross-record semantics.
"""

from __future__ import annotations

import hashlib
from collections import defaultdict
from collections.abc import Callable, Iterable, Sequence
from dataclasses import dataclass
from typing import Final, cast

from mm_chat_rag.contracts.resources import read_schema_bytes
from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    validate_schema_instance,
)

_SCHEMA_HASH_FIELDS: Final = {
    "canonicalManifest": "canonical-manifest.v2.schema.json",
    "canonicalIr": "canonical-ir.v2.schema.json",
    "sourceLocator": "source-locator.v2.schema.json",
    "normalizationMap": "normalization-map.v1.schema.json",
    "qualityReport": "quality-report.v2.schema.json",
    "chunkManifest": "chunk-manifest.v2.schema.json",
    "sourceUnitResolver": "source-unit-resolver.v1.schema.json",
}
_TEXT_BLOCK_TYPES: Final = frozenset(
    {
        "heading",
        "paragraph",
        "list",
        "list_item",
        "quote",
        "code",
        "table",
        "formula",
        "caption",
        "footnote",
        "header",
        "footer",
    }
)


class LineageSemanticsError(ValueError):
    """A schema-valid parser artifact set has inconsistent lineage."""


@dataclass(frozen=True, slots=True)
class _IrIndex:
    document: JsonObject
    text_bytes: bytes
    pages: frozenset[int]
    blocks: tuple[JsonObject, ...]
    blocks_by_id: dict[str, JsonObject]
    blocks_by_ordinal: dict[int, JsonObject]
    flows_by_ordinal: dict[int, JsonObject]
    flows_by_id: dict[str, JsonObject]
    provenance_by_id: dict[str, JsonObject]
    owner_targets: dict[tuple[str, str], JsonObject]


@dataclass(frozen=True, slots=True)
class _ChunkFragment:
    block_id: str
    order: tuple[int, int, int]
    start: int
    end: int
    absolute_start: int
    absolute_end: int


def jcs_sha256(value: JsonValue) -> str:
    """Hash one semantic-order value as exact RFC 8785 bytes."""
    return hashlib.sha256(canonical_json_bytes(value)).hexdigest()


def validate_parser_lineage_semantics(
    packaged: PackagedSchemas,
    canonical_ir: JsonObject,
    chunk_manifest: JsonObject,
    canonical_manifest: JsonObject,
) -> None:
    """Validate the three parser artifacts, schema first and semantics second."""
    validate_schema_instance(
        packaged,
        "canonical-ir.v2.schema.json",
        canonical_ir,
    )
    validate_schema_instance(
        packaged,
        "chunk-manifest.v2.schema.json",
        chunk_manifest,
    )
    validate_schema_instance(
        packaged,
        "canonical-manifest.v2.schema.json",
        canonical_manifest,
    )

    ir_index = _validate_canonical_ir(canonical_ir)
    _validate_chunk_manifest(chunk_manifest, ir_index)
    _validate_canonical_manifest(
        packaged,
        canonical_manifest,
        canonical_ir,
        chunk_manifest,
        ir_index,
    )


def _validate_canonical_ir(document: JsonObject) -> _IrIndex:
    text_buffer = _object(document["textBuffer"], "canonical IR textBuffer")
    text = _string(text_buffer["text"], "canonical IR textBuffer.text")
    text_bytes = text.encode()
    _equal(text_buffer["bytes"], len(text_bytes), "textBuffer.bytes")
    _equal(
        text_buffer["sha256"],
        hashlib.sha256(text_bytes).hexdigest(),
        "textBuffer.sha256",
    )
    utf8_boundaries = _utf8_boundaries(text)

    pages = _objects(document["pages"], "pages")
    _require_contiguous(pages, "pageIndex", "page")
    _require_sorted(
        pages, lambda page: (_integer(page["pageIndex"], "pageIndex"),), "pages"
    )
    page_indexes = frozenset(_integer(page["pageIndex"], "pageIndex") for page in pages)

    flows = _objects(document["readingFlows"], "readingFlows")
    _require_contiguous(flows, "flowOrdinal", "flow")
    _require_sorted(
        flows,
        lambda flow: (
            _integer(flow["flowOrdinal"], "flowOrdinal"),
            _string(flow["flowSeedId"], "flowSeedId"),
        ),
        "readingFlows",
    )
    _require_unique(flows, "flowSeedId", "reading flow seed")
    _require_unique(flows, "logicalFlowId", "logical flow")
    flows_by_ordinal = {
        _integer(flow["flowOrdinal"], "flowOrdinal"): flow for flow in flows
    }
    flows_by_id = {
        _string(flow["logicalFlowId"], "logicalFlowId"): flow for flow in flows
    }

    blocks = _objects(document["blocks"], "blocks")
    _require_contiguous(blocks, "ordinal", "block")
    _require_sorted(
        blocks,
        lambda block: (
            _integer(block["readingFlowOrdinal"], "readingFlowOrdinal"),
            _integer(block["ordinal"], "ordinal"),
            _string(block["logicalBlockId"], "logicalBlockId"),
        ),
        "blocks",
    )
    _require_unique(blocks, "logicalBlockId", "logical block")
    blocks_by_id = {
        _string(block["logicalBlockId"], "logicalBlockId"): block for block in blocks
    }
    blocks_by_ordinal = {
        _integer(block["ordinal"], "ordinal"): block for block in blocks
    }

    for page in pages:
        _validate_locator_page_references(
            _object(page["locatorSet"], "page locatorSet"),
            page_indexes,
            "page locatorSet",
        )

    for block in blocks:
        _validate_block(
            block,
            flows_by_ordinal,
            text_bytes,
            utf8_boundaries,
            page_indexes,
        )
    _validate_block_ancestry(blocks, blocks_by_id)
    _validate_flow_membership(flows, blocks)

    tables, all_cells, formulas, assets = _validate_ir_entities(
        document,
        blocks_by_ordinal,
        text_bytes,
        utf8_boundaries,
        page_indexes,
    )
    provenance_by_id, owner_targets = _validate_provenance(
        document,
        blocks,
        tables,
        all_cells,
        formulas,
        assets,
    )

    return _IrIndex(
        document=document,
        text_bytes=text_bytes,
        pages=page_indexes,
        blocks=tuple(blocks),
        blocks_by_id=blocks_by_id,
        blocks_by_ordinal=blocks_by_ordinal,
        flows_by_ordinal=flows_by_ordinal,
        flows_by_id=flows_by_id,
        provenance_by_id=provenance_by_id,
        owner_targets=owner_targets,
    )


def _validate_ir_entities(
    document: JsonObject,
    blocks: dict[int, JsonObject],
    text_bytes: bytes,
    utf8_boundaries: frozenset[int],
    pages: frozenset[int],
) -> tuple[
    list[JsonObject],
    list[JsonObject],
    list[JsonObject],
    list[JsonObject],
]:
    tables = _objects(document["tables"], "tables")
    _validate_owned_entities(
        tables,
        blocks,
        ordinal_field="tableOrdinal",
        id_field="logicalTableId",
        owner_block_type="table",
        label="table",
    )
    _require_sorted(
        tables,
        lambda table: (
            _integer(table["owningBlockOrdinal"], "owningBlockOrdinal"),
            _integer(table["tableOrdinal"], "tableOrdinal"),
            _string(table["logicalTableId"], "logicalTableId"),
        ),
        "tables",
    )
    all_cells: list[JsonObject] = []
    for table in tables:
        cells = _objects(table["cells"], "table.cells")
        _require_contiguous(cells, "readingOrdinal", "cell reading")
        _require_sorted(
            cells,
            lambda cell: (
                _integer(cell["rowIndex"], "rowIndex"),
                _integer(cell["columnIndex"], "columnIndex"),
                _integer(cell["rowSpan"], "rowSpan"),
                _integer(cell["columnSpan"], "columnSpan"),
            ),
            "table.cells",
        )
        _validate_table_cells(
            table,
            cells,
            blocks,
            text_bytes,
            utf8_boundaries,
            pages,
        )
        _validate_text_variant(table, "table", pages)
        all_cells.extend(cells)
    _require_unique(all_cells, "logicalCellId", "logical cell")

    formulas = _objects(document["formulas"], "formulas")
    _validate_owned_entities(
        formulas,
        blocks,
        ordinal_field="formulaOrdinal",
        id_field="logicalFormulaId",
        owner_block_type="formula",
        label="formula",
    )
    _require_sorted(
        formulas,
        lambda formula: (
            _integer(formula["owningBlockOrdinal"], "owningBlockOrdinal"),
            _integer(formula["formulaOrdinal"], "formulaOrdinal"),
            _string(formula["logicalFormulaId"], "logicalFormulaId"),
        ),
        "formulas",
    )
    for formula in formulas:
        _validate_text_variant(formula, "formula", pages)

    assets = _objects(document["assets"], "assets")
    _validate_owned_entities(
        assets,
        blocks,
        ordinal_field="assetOrdinal",
        id_field="logicalAssetId",
        owner_block_type="asset_ref",
        label="asset",
    )
    _require_sorted(
        assets,
        lambda asset: (
            _integer(asset["owningBlockOrdinal"], "owningBlockOrdinal"),
            _integer(asset["assetOrdinal"], "assetOrdinal"),
            _string(asset["logicalAssetId"], "logicalAssetId"),
        ),
        "assets",
    )
    for asset in assets:
        _validate_structural_variant(asset, "asset", pages)
    return tables, all_cells, formulas, assets


def _validate_provenance(
    document: JsonObject,
    blocks: Sequence[JsonObject],
    tables: Sequence[JsonObject],
    cells: Sequence[JsonObject],
    formulas: Sequence[JsonObject],
    assets: Sequence[JsonObject],
) -> tuple[dict[str, JsonObject], dict[tuple[str, str], JsonObject]]:
    provenance = _objects(document["provenance"], "provenance")
    _require_contiguous(provenance, "provenanceOrdinal", "provenance")
    _require_sorted(
        provenance,
        lambda item: (
            _integer(item["targetKindRank"], "targetKindRank"),
            _string(item["targetOwnerSeedId"], "targetOwnerSeedId"),
            _integer(item["provenanceOrdinal"], "provenanceOrdinal"),
        ),
        "provenance",
    )
    _require_unique(provenance, "provenanceId", "provenance")
    provenance_by_id = {
        _string(item["provenanceId"], "provenanceId"): item for item in provenance
    }
    owner_targets = _owner_target_index(blocks, tables, cells, formulas, assets)
    for item in provenance:
        target_key = (
            _string(item["targetKind"], "targetKind"),
            _string(item["targetOwnerSeedId"], "targetOwnerSeedId"),
        )
        if target_key not in owner_targets:
            raise LineageSemanticsError(
                "provenance targetOwnerSeedId has no target entity"
            )
    for target_key, target in owner_targets.items():
        for provenance_id in _strings(target["provenanceRefs"], "provenanceRefs"):
            try:
                item = provenance_by_id[provenance_id]
            except KeyError as error:
                raise LineageSemanticsError(
                    "provenanceRefs contains a missing reference"
                ) from error
            actual_key = (
                _string(item["targetKind"], "targetKind"),
                _string(item["targetOwnerSeedId"], "targetOwnerSeedId"),
            )
            if actual_key != target_key:
                raise LineageSemanticsError(
                    "provenanceRefs points at provenance for another target"
                )
    return provenance_by_id, owner_targets


def _validate_block(
    block: JsonObject,
    flows: dict[int, JsonObject],
    text_bytes: bytes,
    utf8_boundaries: frozenset[int],
    pages: frozenset[int],
) -> None:
    flow_ordinal = _integer(block["readingFlowOrdinal"], "readingFlowOrdinal")
    try:
        flow = flows[flow_ordinal]
    except KeyError as error:
        raise LineageSemanticsError(
            "block references a missing reading flow"
        ) from error
    _equal(block["flowSeedId"], flow["flowSeedId"], "block flowSeedId")

    block_type = _string(block["blockType"], "blockType")
    structure = _object(block["structureRef"], "block.structureRef")
    _equal(structure["structureKind"], block_type, "block structure kind")
    if block_type in _TEXT_BLOCK_TYPES:
        _validate_text_variant(block, f"{block_type} block", pages)
        _validate_text_range(
            _object(block["textRange"], "block.textRange"),
            text_bytes,
            utf8_boundaries,
            f"{block_type} block",
        )
    else:
        _validate_structural_variant(block, f"{block_type} block", pages)
        locator = _object(block["locatorSet"], "block.locatorSet")
        anchors = _objects(locator["structuralAnchors"], "structuralAnchors")
        expected = (
            block_type,
            _string(structure["ownerSeedId"], "ownerSeedId"),
            _integer(structure["structureOrdinal"], "structureOrdinal"),
        )
        observed = {
            (
                _string(anchor["nodeKind"], "nodeKind"),
                _string(anchor["ownerSeedId"], "ownerSeedId"),
                _integer(anchor["structureOrdinal"], "structureOrdinal"),
            )
            for anchor in anchors
        }
        if expected not in observed:
            raise LineageSemanticsError(
                "structural block locator does not match structureRef"
            )


def _validate_block_ancestry(
    blocks: Sequence[JsonObject],
    blocks_by_id: dict[str, JsonObject],
) -> None:
    positions = {
        _string(block["logicalBlockId"], "logicalBlockId"): index
        for index, block in enumerate(blocks)
    }
    for block in blocks:
        block_id = _string(block["logicalBlockId"], "logicalBlockId")
        seen = {block_id}
        ancestor_headings: list[str] = []
        parent_id = block["parentBlockId"]
        direct_parent_id = parent_id
        while parent_id is not None:
            parent_ref = _string(parent_id, "parentBlockId")
            if parent_ref in seen:
                raise LineageSemanticsError("block parent cycle detected")
            seen.add(parent_ref)
            try:
                parent = blocks_by_id[parent_ref]
            except KeyError as error:
                raise LineageSemanticsError(
                    "block parentBlockId is a missing reference"
                ) from error
            if parent["readingFlowOrdinal"] != block["readingFlowOrdinal"]:
                raise LineageSemanticsError("block parent crosses reading flows")
            if parent["blockType"] == "heading":
                ancestor_headings.append(parent_ref)
            parent_id = parent["parentBlockId"]
        if direct_parent_id is not None:
            direct_parent_ref = _string(direct_parent_id, "parentBlockId")
            if positions[direct_parent_ref] >= positions[block_id]:
                raise LineageSemanticsError(
                    "block parent must be an already completed ancestor"
                )
        ancestor_headings.reverse()
        if _strings(block["headingPath"], "headingPath") != ancestor_headings:
            raise LineageSemanticsError(
                "headingPath must be the root-to-leaf heading ancestor chain"
            )


def _validate_flow_membership(
    flows: Sequence[JsonObject],
    blocks: Sequence[JsonObject],
) -> None:
    for flow in flows:
        ordinal = _integer(flow["flowOrdinal"], "flowOrdinal")
        expected = [
            _string(block["logicalBlockId"], "logicalBlockId")
            for block in blocks
            if block["readingFlowOrdinal"] == ordinal
        ]
        observed = _strings(
            flow["orderedLogicalBlockIds"],
            "orderedLogicalBlockIds",
        )
        if observed != expected:
            raise LineageSemanticsError(
                "flow orderedLogicalBlockIds does not exactly match block order"
            )


def _validate_owned_entities(
    entities: Sequence[JsonObject],
    blocks: dict[int, JsonObject],
    *,
    ordinal_field: str,
    id_field: str,
    owner_block_type: str,
    label: str,
) -> None:
    _require_contiguous(entities, ordinal_field, label)
    _require_unique(entities, id_field, f"logical {label}")
    for entity in entities:
        owner_ordinal = _integer(entity["owningBlockOrdinal"], "owningBlockOrdinal")
        try:
            owner = blocks[owner_ordinal]
        except KeyError as error:
            raise LineageSemanticsError(
                f"{label} references a missing owning block"
            ) from error
        if owner["blockType"] != owner_block_type:
            raise LineageSemanticsError(f"{label} owning block has the wrong blockType")
        structure = _object(owner["structureRef"], "owner block structureRef")
        _equal(entity["ownerSeedId"], structure["ownerSeedId"], f"{label} ownerSeedId")


def _validate_table_cells(
    table: JsonObject,
    cells: Sequence[JsonObject],
    blocks: dict[int, JsonObject],
    text_bytes: bytes,
    utf8_boundaries: frozenset[int],
    pages: frozenset[int],
) -> None:
    row_count = _integer(table["rowCount"], "rowCount")
    column_count = _integer(table["columnCount"], "columnCount")
    owner = blocks[_integer(table["owningBlockOrdinal"], "owningBlockOrdinal")]
    owner_range = _object(owner["textRange"], "table block textRange")
    owner_start = _integer(owner_range["startByte"], "startByte")
    owner_end = _integer(owner_range["endByte"], "endByte")

    rectangles: list[tuple[int, int, int, int]] = []
    for cell in cells:
        row_start = _integer(cell["rowIndex"], "rowIndex")
        column_start = _integer(cell["columnIndex"], "columnIndex")
        row_end = row_start + _integer(cell["rowSpan"], "rowSpan")
        column_end = column_start + _integer(cell["columnSpan"], "columnSpan")
        if row_end > row_count or column_end > column_count:
            raise LineageSemanticsError("table cell lies outside the declared grid")
        rectangle = (row_start, row_end, column_start, column_end)
        if any(_rectangles_overlap(rectangle, other) for other in rectangles):
            raise LineageSemanticsError("table cells overlap in the declared grid")
        rectangles.append(rectangle)

        if cell["textRange"] is None:
            _validate_structural_variant(cell, "cell", pages)
        else:
            _validate_text_variant(cell, "cell", pages)
            cell_range = _object(cell["textRange"], "cell.textRange")
            _validate_text_range(
                cell_range,
                text_bytes,
                utf8_boundaries,
                "cell",
            )
            start = _integer(cell_range["startByte"], "startByte")
            end = _integer(cell_range["endByte"], "endByte")
            if start < owner_start or end > owner_end:
                raise LineageSemanticsError("cell textRange escapes its table block")


def _validate_text_variant(
    node: JsonObject,
    label: str,
    pages: frozenset[int],
) -> None:
    source_span = _object(node["sourceSpanHash"], f"{label}.sourceSpanHash")
    if source_span["kind"] != "text":
        raise LineageSemanticsError(f"{label} must use the text source-span variant")
    locator = _object(node["locatorSet"], f"{label}.locatorSet")
    if not _objects(locator["textAnchors"], "textAnchors"):
        raise LineageSemanticsError(f"{label} text variant has no text anchors")
    if _objects(locator["structuralAnchors"], "structuralAnchors"):
        raise LineageSemanticsError(f"{label} text variant mixes structural anchors")
    _validate_locator_page_references(locator, pages, f"{label}.locatorSet")


def _validate_structural_variant(
    node: JsonObject,
    label: str,
    pages: frozenset[int],
) -> None:
    source_span = _object(node["sourceSpanHash"], f"{label}.sourceSpanHash")
    if source_span["kind"] != "structural":
        raise LineageSemanticsError(
            f"{label} must use the structural source-span variant"
        )
    locator = _object(node["locatorSet"], f"{label}.locatorSet")
    if _objects(locator["textAnchors"], "textAnchors"):
        raise LineageSemanticsError(f"{label} structural variant mixes text anchors")
    if not _objects(locator["structuralAnchors"], "structuralAnchors"):
        raise LineageSemanticsError(
            f"{label} structural variant has no structural anchors"
        )
    _validate_locator_page_references(locator, pages, f"{label}.locatorSet")


def _owner_target_index(
    blocks: Sequence[JsonObject],
    tables: Sequence[JsonObject],
    cells: Sequence[JsonObject],
    formulas: Sequence[JsonObject],
    assets: Sequence[JsonObject],
) -> dict[tuple[str, str], JsonObject]:
    result: dict[tuple[str, str], JsonObject] = {}
    groups = (
        (
            "block",
            blocks,
            lambda node: _object(node["structureRef"], "structureRef")["ownerSeedId"],
        ),
        ("table", tables, lambda node: node["ownerSeedId"]),
        ("cell", cells, lambda node: node["ownerSeedId"]),
        ("formula", formulas, lambda node: node["ownerSeedId"]),
        ("asset", assets, lambda node: node["ownerSeedId"]),
    )
    for kind, nodes, owner_value in groups:
        for node in nodes:
            key = (kind, _string(owner_value(node), "ownerSeedId"))
            if key in result:
                raise LineageSemanticsError(
                    f"duplicate {kind} target ownerSeedId is ambiguous"
                )
            result[key] = node
    return result


def _validate_chunk_manifest(manifest: JsonObject, ir: _IrIndex) -> None:
    source = _object(ir.document["source"], "canonical IR source")
    _equal(
        manifest["sourceSha256"],
        source["sha256"],
        "chunk sourceSha256",
    )
    parents = _objects(manifest["parents"], "parents")
    children = _objects(manifest["children"], "children")
    _require_contiguous(parents, "parentOrdinal", "parent chunk")
    _require_unique(parents, "logicalChunkId", "logical parent chunk")
    _require_unique(children, "logicalChunkId", "logical child chunk")
    parent_by_id = {
        _string(parent["logicalChunkId"], "logicalChunkId"): parent
        for parent in parents
    }

    chunk_profile_hash = manifest["chunkProfileHash"]
    for parent in parents:
        _equal(
            parent["chunkProfileHash"], chunk_profile_hash, "parent chunkProfileHash"
        )
        section_owner = (
            "block",
            _string(parent["sectionOwnerSeedId"], "sectionOwnerSeedId"),
        )
        if section_owner not in ir.owner_targets:
            raise LineageSemanticsError(
                "parent sectionOwnerSeedId has no block reference"
            )
        _validate_chunk_record(parent, ir, "parent")

    children_by_parent: dict[str, list[JsonObject]] = defaultdict(list)
    for child in children:
        parent_id = _string(child["logicalParentChunkId"], "logicalParentChunkId")
        try:
            parent = parent_by_id[parent_id]
        except KeyError as error:
            raise LineageSemanticsError(
                "child references a missing logical parent chunk"
            ) from error
        _equal(child["chunkProfileHash"], chunk_profile_hash, "child chunkProfileHash")
        _equal(
            child["parentChunkSeedId"],
            parent["parentChunkSeedId"],
            "child parentChunkSeedId",
        )
        _equal(child["logicalFlowId"], parent["logicalFlowId"], "child logicalFlowId")
        _validate_chunk_record(child, ir, "child")
        _validate_child_containment(child, parent)
        children_by_parent[parent_id].append(child)

    expected_children = sorted(
        children,
        key=lambda child: (
            _integer(
                parent_by_id[
                    _string(child["logicalParentChunkId"], "logicalParentChunkId")
                ]["parentOrdinal"],
                "parentOrdinal",
            ),
            _integer(child["childOrdinal"], "childOrdinal"),
            _string(child["logicalChunkId"], "logicalChunkId"),
        ),
    )
    if list(children) != expected_children:
        raise LineageSemanticsError("children are not in canonical parent/child order")

    for parent_id in parent_by_id:
        siblings = children_by_parent.get(parent_id, [])
        if not siblings:
            raise LineageSemanticsError("parent chunk has no child chunks")
        _require_contiguous(siblings, "childOrdinal", "child chunk")
        _validate_sibling_overlaps(siblings)

    spans = [
        fragment
        for chunk in [*parents, *children]
        for fragment in _objects(chunk["spanFragments"], "spanFragments")
    ]
    joiners = [
        joiner
        for chunk in [*parents, *children]
        for joiner in _objects(chunk["joiners"], "joiners")
    ]
    _equal(manifest["parentCount"], len(parents), "parentCount")
    _equal(manifest["childCount"], len(children), "childCount")
    _equal(manifest["spanCount"], len(spans), "spanCount")
    _equal(manifest["joinerCount"], len(joiners), "joinerCount")
    _equal(
        manifest["parentAggregateHash"],
        jcs_sha256(cast("JsonValue", list(parents))),
        "parentAggregateHash",
    )
    _equal(
        manifest["childAggregateHash"],
        jcs_sha256(cast("JsonValue", list(children))),
        "childAggregateHash",
    )
    _equal(
        manifest["spanAggregateHash"],
        jcs_sha256(cast("JsonValue", spans)),
        "spanAggregateHash",
    )
    _equal(
        manifest["joinerAggregateHash"],
        jcs_sha256(cast("JsonValue", joiners)),
        "joinerAggregateHash",
    )


def _validate_chunk_record(
    chunk: JsonObject,
    ir: _IrIndex,
    label: str,
) -> None:
    flow_id = _string(chunk["logicalFlowId"], "logicalFlowId")
    if flow_id not in ir.flows_by_id:
        raise LineageSemanticsError(f"{label} chunk references a missing logical flow")
    fragments = _objects(chunk["spanFragments"], "spanFragments")
    joiners = _objects(chunk["joiners"], "joiners")
    if len(joiners) != len(fragments) - 1:
        raise LineageSemanticsError(f"{label} chunk joiner count must be fragment n-1")

    previous_order: tuple[int, int, int] | None = None
    previous_block_id: str | None = None
    previous_end = -1
    reconstructed = bytearray()
    for index, fragment in enumerate(fragments):
        validated = _validate_chunk_fragment(fragment, ir, flow_id, label)
        if previous_order is not None and validated.order <= previous_order:
            raise LineageSemanticsError(
                f"{label} chunk fragments are not in strict reading order"
            )
        if previous_block_id == validated.block_id and validated.start < previous_end:
            raise LineageSemanticsError(f"{label} chunk fragments overlap")
        previous_order = validated.order
        previous_block_id = validated.block_id
        previous_end = validated.end

        if index:
            reconstructed.extend(
                _string(joiners[index - 1]["utf8Bytes"], "joiner.utf8Bytes").encode()
            )
        reconstructed.extend(
            ir.text_bytes[validated.absolute_start : validated.absolute_end]
        )

    _equal(chunk["contentBytes"], len(reconstructed), f"{label} contentBytes")
    _equal(
        chunk["contentHash"],
        hashlib.sha256(reconstructed).hexdigest(),
        f"{label} contentHash",
    )


def _validate_chunk_fragment(
    fragment: JsonObject,
    ir: _IrIndex,
    flow_id: str,
    label: str,
) -> _ChunkFragment:
    block_id = _string(fragment["blockLogicalId"], "blockLogicalId")
    try:
        block = ir.blocks_by_id[block_id]
    except KeyError as error:
        raise LineageSemanticsError(
            f"{label} chunk fragment references a missing block"
        ) from error
    flow = ir.flows_by_ordinal[
        _integer(block["readingFlowOrdinal"], "readingFlowOrdinal")
    ]
    if flow["logicalFlowId"] != flow_id:
        raise LineageSemanticsError(f"{label} chunk fragment crosses reading flows")
    if block["textRange"] is None:
        raise LineageSemanticsError(f"{label} chunk references a structural block")
    block_range = _object(block["textRange"], "block.textRange")
    block_start = _integer(block_range["startByte"], "startByte")
    block_end = _integer(block_range["endByte"], "endByte")
    start = _integer(fragment["blockStartByte"], "blockStartByte")
    end = _integer(fragment["blockEndByte"], "blockEndByte")
    if start >= end:
        raise LineageSemanticsError(f"{label} chunk contains an empty range")
    if end > block_end - block_start:
        raise LineageSemanticsError(f"{label} chunk fragment exceeds its block range")

    absolute_start = block_start + start
    absolute_end = block_start + end
    locator = _object(fragment["clippedLocatorSet"], "clippedLocatorSet")
    anchors = _objects(locator["textAnchors"], "textAnchors")
    if not anchors or _objects(locator["structuralAnchors"], "structuralAnchors"):
        raise LineageSemanticsError(
            f"{label} chunk fragment must have text-only clipped locators"
        )
    for anchor in anchors:
        anchor_start = _integer(anchor["canonicalStartByte"], "canonicalStartByte")
        anchor_end = _integer(anchor["canonicalEndByte"], "canonicalEndByte")
        if anchor_start < absolute_start or anchor_end > absolute_end:
            raise LineageSemanticsError(
                f"{label} chunk clipped locator escapes its fragment"
            )
    _validate_locator_page_references(locator, ir.pages, "clippedLocatorSet")
    return _ChunkFragment(
        block_id=block_id,
        order=(_integer(block["ordinal"], "ordinal"), start, end),
        start=start,
        end=end,
        absolute_start=absolute_start,
        absolute_end=absolute_end,
    )


def _validate_child_containment(child: JsonObject, parent: JsonObject) -> None:
    parent_fragments = _objects(parent["spanFragments"], "parent.spanFragments")
    cursor = 0
    for child_fragment in _objects(child["spanFragments"], "child.spanFragments"):
        while cursor < len(parent_fragments) and not _fragment_contains(
            parent_fragments[cursor],
            child_fragment,
        ):
            cursor += 1
        if cursor == len(parent_fragments):
            raise LineageSemanticsError(
                "child fragment is not an ordered subset of its parent"
            )


def _validate_sibling_overlaps(siblings: Sequence[JsonObject]) -> None:
    occurrences: dict[tuple[str, int, int, str, bytes], tuple[int, str]] = {}
    for sibling in siblings:
        ordinal = _integer(sibling["childOrdinal"], "childOrdinal")
        for fragment in _objects(sibling["spanFragments"], "spanFragments"):
            kind = _string(fragment["fragmentKind"], "fragmentKind")
            key = _fragment_identity(fragment)
            previous = occurrences.get(key)
            if kind == "window_overlap":
                previous_ordinal = _integer(
                    fragment["previousChildOrdinal"],
                    "previousChildOrdinal",
                )
                if previous_ordinal != ordinal - 1:
                    raise LineageSemanticsError(
                        "window overlap must reference the adjacent previous child"
                    )
                if previous is None or previous[0] != previous_ordinal:
                    raise LineageSemanticsError(
                        "window overlap has no identical fragment in the previous child"
                    )
            elif kind != "derived_context" and previous is not None:
                raise LineageSemanticsError(
                    "reused child source must be marked window_overlap"
                )
            occurrences[key] = (ordinal, kind)


def _fragment_contains(parent: JsonObject, child: JsonObject) -> bool:
    return (
        parent["blockLogicalId"] == child["blockLogicalId"]
        and _integer(parent["blockStartByte"], "blockStartByte")
        <= _integer(child["blockStartByte"], "blockStartByte")
        and _integer(child["blockEndByte"], "blockEndByte")
        <= _integer(parent["blockEndByte"], "blockEndByte")
    )


def _fragment_identity(fragment: JsonObject) -> tuple[str, int, int, str, bytes]:
    return (
        _string(fragment["blockLogicalId"], "blockLogicalId"),
        _integer(fragment["blockStartByte"], "blockStartByte"),
        _integer(fragment["blockEndByte"], "blockEndByte"),
        _string(fragment["fragmentSourceSpanHash"], "fragmentSourceSpanHash"),
        canonical_json_bytes(fragment["clippedLocatorSet"]),
    )


def _validate_canonical_manifest(
    packaged: PackagedSchemas,
    manifest: JsonObject,
    canonical_ir: JsonObject,
    chunk_manifest: JsonObject,
    ir: _IrIndex,
) -> None:
    source = _object(manifest["source"], "manifest.source")
    if source != _object(canonical_ir["source"], "canonical IR source"):
        raise LineageSemanticsError("manifest source descriptor is inconsistent")
    parser = _object(canonical_ir["parser"], "canonical IR parser")
    _equal(manifest["configHash"], parser["configHash"], "manifest configHash")
    _equal(
        manifest["parserProfileHash"],
        parser["profileHash"],
        "manifest parserProfileHash",
    )
    _equal(
        manifest["chunkProfileHash"],
        chunk_manifest["chunkProfileHash"],
        "manifest chunkProfileHash",
    )

    ir_bytes = canonical_json_bytes(canonical_ir)
    ir_descriptor = _object(manifest["canonicalIr"], "manifest.canonicalIr")
    _equal(ir_descriptor["bytes"], len(ir_bytes), "canonicalIr.bytes")
    _equal(
        ir_descriptor["sha256"],
        hashlib.sha256(ir_bytes).hexdigest(),
        "canonicalIr.sha256",
    )
    text_descriptor = _object(manifest["textBuffer"], "manifest.textBuffer")
    _equal(text_descriptor["bytes"], len(ir.text_bytes), "manifest textBuffer.bytes")
    _equal(
        text_descriptor["sha256"],
        hashlib.sha256(ir.text_bytes).hexdigest(),
        "manifest textBuffer.sha256",
    )
    normalization_descriptor = _object(
        manifest["normalizationMap"],
        "manifest.normalizationMap",
    )
    normalization_ref = _object(
        canonical_ir["normalizationMapRef"],
        "canonical IR normalizationMapRef",
    )
    for field in ("schemaVersion", "bytes", "sha256"):
        _equal(
            normalization_descriptor[field],
            normalization_ref[field],
            f"normalizationMap.{field}",
        )
    resolver_descriptor = _object(
        manifest["sourceUnitResolver"],
        "manifest.sourceUnitResolver",
    )
    _equal(
        resolver_descriptor["entryCount"],
        normalization_descriptor["sourceUnitCount"],
        "sourceUnitResolver.entryCount",
    )

    counts = _object(manifest["entityCounts"], "entityCounts")
    tables = _objects(canonical_ir["tables"], "tables")
    entity_arrays: dict[str, list[JsonObject]] = {
        "pages": _objects(canonical_ir["pages"], "pages"),
        "readingFlows": _objects(canonical_ir["readingFlows"], "readingFlows"),
        "blocks": _objects(canonical_ir["blocks"], "blocks"),
        "tables": tables,
        "cells": [
            cell for table in tables for cell in _objects(table["cells"], "cells")
        ],
        "formulas": _objects(canonical_ir["formulas"], "formulas"),
        "assets": _objects(canonical_ir["assets"], "assets"),
        "provenance": _objects(canonical_ir["provenance"], "provenance"),
    }
    count_fields = {
        "pages": "pageCount",
        "readingFlows": "readingFlowCount",
        "blocks": "blockCount",
        "tables": "tableCount",
        "cells": "cellCount",
        "formulas": "formulaCount",
        "assets": "assetCount",
        "provenance": "provenanceCount",
    }
    aggregates = _object(manifest["entityAggregateHashes"], "entityAggregateHashes")
    for entity_name, records in entity_arrays.items():
        _equal(
            counts[count_fields[entity_name]], len(records), count_fields[entity_name]
        )
        _equal(
            aggregates[entity_name],
            jcs_sha256(cast("JsonValue", records)),
            f"entityAggregateHashes.{entity_name}",
        )

    chunk_bytes = canonical_json_bytes(chunk_manifest)
    chunk_descriptor = _object(manifest["chunks"], "manifest.chunks")
    _equal(chunk_descriptor["bytes"], len(chunk_bytes), "chunks.bytes")
    _equal(
        chunk_descriptor["sha256"],
        hashlib.sha256(chunk_bytes).hexdigest(),
        "chunks.sha256",
    )
    for field in (
        "schemaVersion",
        "parentCount",
        "childCount",
        "spanCount",
        "joinerCount",
        "parentAggregateHash",
        "childAggregateHash",
        "spanAggregateHash",
        "joinerAggregateHash",
    ):
        _equal(chunk_descriptor[field], chunk_manifest[field], f"chunks.{field}")

    schema_hashes = _object(manifest["schemaHashes"], "schemaHashes")
    for field, schema_name in _SCHEMA_HASH_FIELDS.items():
        if schema_name not in packaged.by_name:
            raise LineageSemanticsError(
                f"required packaged schema is missing: {schema_name}"
            )
        expected = hashlib.sha256(read_schema_bytes(schema_name)).hexdigest()
        _equal(schema_hashes[field], expected, f"schemaHashes.{field}")


def _validate_text_range(
    text_range: JsonObject,
    text_bytes: bytes,
    utf8_boundaries: frozenset[int],
    label: str,
) -> None:
    start = _integer(text_range["startByte"], "startByte")
    end = _integer(text_range["endByte"], "endByte")
    if start >= end:
        raise LineageSemanticsError(f"{label} has an empty textRange")
    if end > len(text_bytes):
        raise LineageSemanticsError(f"{label} textRange exceeds textBuffer")
    if start not in utf8_boundaries or end not in utf8_boundaries:
        raise LineageSemanticsError(f"{label} textRange splits a UTF-8 code point")


def _validate_locator_page_references(
    locator: JsonObject,
    pages: frozenset[int],
    label: str,
) -> None:
    anchors = [
        *_objects(locator["textAnchors"], "textAnchors"),
        *_objects(locator["structuralAnchors"], "structuralAnchors"),
    ]
    for anchor in anchors:
        for fragment in _objects(anchor["sourceFragments"], "sourceFragments"):
            for view in _objects(fragment["views"], "views"):
                if view["kind"] == "page_region":
                    page_index = _integer(view["pageIndex"], "pageIndex")
                    if page_index not in pages:
                        raise LineageSemanticsError(
                            f"{label} has a missing page reference"
                        )


def _require_contiguous(
    records: Sequence[JsonObject],
    field: str,
    label: str,
) -> None:
    observed = [_integer(record[field], field) for record in records]
    expected = list(range(len(records)))
    if observed != expected:
        raise LineageSemanticsError(
            f"{label} ordinals must be contiguous from zero: {observed!r}"
        )


def _require_sorted(
    records: Sequence[JsonObject],
    key: Callable[[JsonObject], tuple[object, ...]],
    label: str,
) -> None:
    if list(records) != sorted(records, key=key):
        raise LineageSemanticsError(f"{label} are not in canonical sorted order")


def _require_unique(
    records: Iterable[JsonObject],
    field: str,
    label: str,
) -> None:
    values = [_string(record[field], field) for record in records]
    if len(values) != len(set(values)):
        raise LineageSemanticsError(f"{label} identifiers must be unique")


def _rectangles_overlap(
    left: tuple[int, int, int, int],
    right: tuple[int, int, int, int],
) -> bool:
    return (
        left[0] < right[1]
        and right[0] < left[1]
        and left[2] < right[3]
        and right[2] < left[3]
    )


def _utf8_boundaries(text: str) -> frozenset[int]:
    offsets = {0}
    byte_offset = 0
    for character in text:
        byte_offset += len(character.encode())
        offsets.add(byte_offset)
    return frozenset(offsets)


def _equal(observed: JsonValue, expected: object, label: str) -> None:
    if observed != expected:
        raise LineageSemanticsError(f"{label} is inconsistent")


def _object(value: JsonValue, label: str) -> JsonObject:
    if not isinstance(value, dict):
        raise LineageSemanticsError(f"{label} is not an object after schema validation")
    return value


def _objects(value: JsonValue, label: str) -> list[JsonObject]:
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise LineageSemanticsError(
            f"{label} is not an object array after schema validation"
        )
    return cast("list[JsonObject]", value)


def _strings(value: JsonValue, label: str) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise LineageSemanticsError(
            f"{label} is not a string array after schema validation"
        )
    return cast("list[str]", value)


def _string(value: JsonValue, label: str) -> str:
    if not isinstance(value, str):
        raise LineageSemanticsError(f"{label} is not a string after schema validation")
    return value


def _integer(value: JsonValue, label: str) -> int:
    if type(value) is not int:
        raise LineageSemanticsError(
            f"{label} is not an integer after schema validation"
        )
    return value
