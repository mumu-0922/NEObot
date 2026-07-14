"""Closed semantic validator for the parser hash-DAG artifact fixture.

The packaged schemas prove local shape.  This test-only validator proves the
remaining C1.1 contract: exact artifact bytes, manifest ledger bindings,
quality-report bindings, and every logical ID/hash that is present in the
A--F fixture.  Logical values are always recomputed through the packaged
``logical-hash-envelope.v1`` schema and its domain-separated hash helper.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass
from pathlib import Path
from typing import Final, cast

from mm_chat_rag.contracts.resources import read_schema_bytes
from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    domain_separated_sha256,
    load_packaged_schemas,
    load_strict_json_bytes,
    logical_hash_envelope_sha256,
    validate_schema_instance,
)
from tests.support.parser_lineage_semantics import (
    LineageSemanticsError,
    validate_parser_lineage_semantics,
)

NORMALIZATION_MAP_DOMAIN: Final = "mm-chat.normalization-map.v1\n"
SOURCE_UNIT_RESOLVER_DOMAIN: Final = "mm-chat.source-unit-resolver.v1\n"
SOURCE_LOCATOR_DOMAIN: Final = "mm-chat.source-locator.v2\n"

_ARTIFACT_SPECS: Final = (
    ("canonical_ir", "canonical-ir.v2.json", "canonical-ir.v2.schema.json"),
    (
        "normalization_map",
        "normalization-map.v1.json",
        "normalization-map.v1.schema.json",
    ),
    (
        "source_unit_resolver",
        "source-unit-resolver.v1.json",
        "source-unit-resolver.v1.schema.json",
    ),
    (
        "quality_report",
        "quality-report.v2.json",
        "quality-report.v2.schema.json",
    ),
    (
        "chunk_manifest",
        "chunk-manifest.v2.json",
        "chunk-manifest.v2.schema.json",
    ),
    (
        "canonical_manifest",
        "canonical-manifest.v2.json",
        "canonical-manifest.v2.schema.json",
    ),
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
_DOMAINS: Final = {
    "opaque_source_unit_id": "mm-chat.opaque-source-unit-id.v1\n",
    "source_unit_payload_ref": "mm-chat.source-unit-payload-ref.v1\n",
    "flow_seed_id": "mm-chat.flow-seed-id.v1\n",
    "owner_seed_id": "mm-chat.owner-seed-id.v1\n",
    "opaque_structure_id": "mm-chat.opaque-structure-id.v1\n",
    "text_source_span_hash": "mm-chat.text-source-span.v1\n",
    "provenance_hash": "mm-chat.provenance.v1\n",
    "provenance_id": "mm-chat.provenance-id.v1\n",
    "logical_block_id": "mm-chat.logical-block-id.v1\n",
    "logical_flow_id": "mm-chat.logical-flow-id.v1\n",
    "parent_chunk_seed_id": "mm-chat.parent-chunk-seed-id.v1\n",
    "logical_parent_chunk_id": "mm-chat.parent-chunk-id.v1\n",
    "logical_child_chunk_id": "mm-chat.child-chunk-id.v1\n",
    "chunk_source_span_hash": "mm-chat.chunk-source-span.v1\n",
}
_STRUCTURE_KIND_RANK: Final = {
    kind: rank
    for rank, kind in enumerate(
        (
            "document",
            "page",
            "heading",
            "paragraph",
            "list",
            "list_item",
            "quote",
            "code",
            "table",
            "table_row",
            "table_cell",
            "formula",
            "caption",
            "footnote",
            "header",
            "footer",
            "page_break",
            "asset_ref",
            "slide",
            "sheet",
            "shape",
        )
    )
}
_COUNT_FIELDS: Final = {
    "pages": "pageCount",
    "readingFlows": "readingFlowCount",
    "blocks": "blockCount",
    "tables": "tableCount",
    "cells": "cellCount",
    "formulas": "formulaCount",
    "assets": "assetCount",
    "provenance": "provenanceCount",
}


class HashDagSemanticsError(ValueError):
    """A schema-valid hash-DAG artifact set is not cryptographically closed."""


@dataclass(slots=True)
class ExactArtifact:
    """One parsed artifact together with the exact bytes being validated."""

    document: JsonObject
    content: bytes


@dataclass(slots=True)
class ParserHashDagArtifactSet:
    """The six independently persisted artifacts in the hash-DAG fixture."""

    canonical_ir: ExactArtifact
    normalization_map: ExactArtifact
    source_unit_resolver: ExactArtifact
    quality_report: ExactArtifact
    chunk_manifest: ExactArtifact
    canonical_manifest: ExactArtifact


@dataclass(frozen=True, slots=True)
class _HashContext:
    packaged: PackagedSchemas
    source_hash: str
    normalization_profile_hash: str
    parser_build_hash: str
    config_hash: str
    parser_profile_hash: str


@dataclass(frozen=True, slots=True)
class _LogicalInventory:
    source_unit_ids: frozenset[str]
    source_unit_payload_refs: frozenset[str]
    owner_seed_ids: frozenset[str]
    provenance_ids: frozenset[str]
    block_ids: frozenset[str]
    flow_ids: frozenset[str]


def load_parser_hash_dag_artifact_set(root: Path) -> ParserHashDagArtifactSet:
    """Load all six exact-JCS files from one semantic fixture directory."""
    loaded: dict[str, ExactArtifact] = {}
    for attribute, filename, _schema_name in _ARTIFACT_SPECS:
        content = (root / filename).read_bytes()
        value = load_strict_json_bytes(content)
        loaded[attribute] = ExactArtifact(_object(value, filename), content)
    return ParserHashDagArtifactSet(
        canonical_ir=loaded["canonical_ir"],
        normalization_map=loaded["normalization_map"],
        source_unit_resolver=loaded["source_unit_resolver"],
        quality_report=loaded["quality_report"],
        chunk_manifest=loaded["chunk_manifest"],
        canonical_manifest=loaded["canonical_manifest"],
    )


def validate_parser_hash_dag_semantics(
    artifacts: ParserHashDagArtifactSet,
    *,
    packaged_schemas: PackagedSchemas | None = None,
) -> None:
    """Validate exact bytes, all A--F hashes, and every artifact ledger edge."""
    packaged = packaged_schemas or load_packaged_schemas()
    _validate_exact_artifacts(artifacts, packaged)

    ir = artifacts.canonical_ir.document
    normalization_map = artifacts.normalization_map.document
    resolver = artifacts.source_unit_resolver.document
    chunks = artifacts.chunk_manifest.document
    manifest = artifacts.canonical_manifest.document

    context = _hash_context(ir, packaged)
    source_ids, payload_refs = _validate_stage_a_and_normalization(
        context,
        ir,
        normalization_map,
        resolver,
    )
    owner_seed_ids = _validate_stages_b_c1_c2_c3(context, ir, chunks, source_ids)
    provenance_ids = _validate_stage_c4(
        context,
        ir,
        source_ids,
        payload_refs,
        owner_seed_ids,
    )
    block_ids, flow_ids = _validate_stages_d_e(
        context,
        ir,
        owner_seed_ids,
        provenance_ids,
    )
    _validate_stage_f(context, ir, chunks, owner_seed_ids, block_ids, flow_ids)
    inventory = _LogicalInventory(
        source_unit_ids=frozenset(source_ids),
        source_unit_payload_refs=frozenset(payload_refs),
        owner_seed_ids=frozenset(owner_seed_ids),
        provenance_ids=frozenset(provenance_ids),
        block_ids=frozenset(block_ids),
        flow_ids=frozenset(flow_ids),
    )
    _validate_quality_report(artifacts, context, inventory)
    _validate_manifest(artifacts, context, inventory)

    try:
        validate_parser_lineage_semantics(packaged, ir, chunks, manifest)
    except LineageSemanticsError as error:
        raise HashDagSemanticsError(f"lineage: {error}") from error


def _validate_exact_artifacts(
    artifacts: ParserHashDagArtifactSet,
    packaged: PackagedSchemas,
) -> None:
    for attribute, _filename, schema_name in _ARTIFACT_SPECS:
        artifact = cast("ExactArtifact", getattr(artifacts, attribute))
        expected = canonical_json_bytes(artifact.document)
        if artifact.content != expected:
            _fail(attribute, "exact bytes do not equal the document's RFC 8785 JCS")
        validate_schema_instance(packaged, schema_name, artifact.document)


def _hash_context(ir: JsonObject, packaged: PackagedSchemas) -> _HashContext:
    source = _object(ir["source"], "canonicalIr.source")
    normalization = _object(
        ir["normalizationProfile"], "canonicalIr.normalizationProfile"
    )
    parser = _object(ir["parser"], "canonicalIr.parser")

    normalization_profile_hash = _jcs_sha256(
        {"contract": "hash-dag-normalization", "revision": 1}
    )
    parser_build_hash = _jcs_sha256({"build": "hash-dag-parser", "revision": 1})
    config_hash = _jcs_sha256({"config": "hash-dag-parser", "revision": 1})
    parser_profile_hash = _jcs_sha256(
        {
            "configHash": config_hash,
            "idDag": ["A", "B", "C1", "C2", "C3", "C4", "D", "E", "F"],
            "normalizationProfileHash": normalization_profile_hash,
            "parserBuildHash": parser_build_hash,
        }
    )
    _equal(
        normalization["profileHash"],
        normalization_profile_hash,
        "canonicalIr.normalizationProfile.profileHash",
    )
    _equal(parser["parserBuildHash"], parser_build_hash, "parser.parserBuildHash")
    _equal(parser["configHash"], config_hash, "parser.configHash")
    _equal(parser["profileHash"], parser_profile_hash, "parser.profileHash")
    return _HashContext(
        packaged=packaged,
        source_hash=_string(source["sha256"], "canonicalIr.source.sha256"),
        normalization_profile_hash=normalization_profile_hash,
        parser_build_hash=parser_build_hash,
        config_hash=config_hash,
        parser_profile_hash=parser_profile_hash,
    )


def _validate_stage_a_and_normalization(
    context: _HashContext,
    ir: JsonObject,
    normalization_map: JsonObject,
    resolver: JsonObject,
) -> tuple[set[str], set[str]]:
    text_buffer = _object(ir["textBuffer"], "canonicalIr.textBuffer")
    text = _string(text_buffer["text"], "canonicalIr.textBuffer.text")
    text_bytes = text.encode()
    source = _object(ir["source"], "canonicalIr.source")
    _equal(source["bytes"], len(text_bytes), "canonicalIr.source.bytes")
    _equal(context.source_hash, _sha256(text_bytes), "canonicalIr.source.sha256")
    _equal(text_buffer["bytes"], len(text_bytes), "canonicalIr.textBuffer.bytes")
    _equal(text_buffer["sha256"], _sha256(text_bytes), "textBuffer.sha256")
    _equal(
        normalization_map["normalizationProfileHash"],
        context.normalization_profile_hash,
        "normalizationMap.normalizationProfileHash",
    )
    _equal(
        normalization_map["textBufferBytes"],
        len(text_bytes),
        "normalizationMap.textBufferBytes",
    )
    _equal(
        normalization_map["textBufferSha256"],
        _sha256(text_bytes),
        "normalizationMap.textBufferSha256",
    )
    _validate_aggregate(normalization_map, NORMALIZATION_MAP_DOMAIN, "normalizationMap")

    source_units = _objects(normalization_map["sourceUnits"], "sourceUnits")
    source_unit_ids: set[str] = set()
    units_by_ordinal: dict[int, JsonObject] = {}
    for ordinal, unit in enumerate(source_units):
        label = f"normalizationMap.sourceUnits[{ordinal}]"
        _equal(unit["sourceUnitOrdinal"], ordinal, f"{label}.sourceUnitOrdinal")
        _equal(unit["sourceSha256"], context.source_hash, f"{label}.sourceSha256")
        if unit["kind"] == "raw_file":
            _equal(unit["sourceBytes"], source["bytes"], f"{label}.sourceBytes")
        expected = _logical_hash(
            context,
            "opaque_source_unit_id",
            {
                "sourceHash": context.source_hash,
                "sourceUnitBytesHash": unit["sourceSha256"],
                "sourceUnitKind": unit["kind"],
                "sourceUnitKindRank": _source_unit_kind_rank(unit["kind"], label),
                "sourceUnitOrdinal": ordinal,
            },
        )
        observed = _string(unit["opaqueSourceUnitId"], f"{label}.opaqueSourceUnitId")
        _equal(observed, expected, f"{label}.opaque_source_unit_id")
        if observed in source_unit_ids:
            _fail(label, "duplicate opaqueSourceUnitId")
        source_unit_ids.add(observed)
        units_by_ordinal[ordinal] = unit

    segments = _objects(normalization_map["segments"], "segments")
    expected_start = 0
    for ordinal, segment in enumerate(segments):
        label = f"normalizationMap.segments[{ordinal}]"
        _equal(segment["segmentOrdinal"], ordinal, f"{label}.segmentOrdinal")
        _equal(segment["canonicalStartByte"], expected_start, f"{label}.start")
        expected_start = _integer(segment["canonicalEndByte"], f"{label}.end")
        transform = _object(segment["transform"], f"{label}.transform")
        for position in _objects(transform.get("sourcePositions", []), label):
            if position["opaqueSourceUnitId"] not in source_unit_ids:
                _fail(label, "segment references a missing Stage-A source unit")
    _equal(expected_start, len(text_bytes), "normalizationMap segment exact cover")

    payload_refs = _validate_resolver_stage_a(
        context,
        resolver,
        units_by_ordinal,
    )
    return source_unit_ids, payload_refs


def _validate_resolver_stage_a(
    context: _HashContext,
    resolver: JsonObject,
    units_by_ordinal: dict[int, JsonObject],
) -> set[str]:
    """Recompute resolver payload hashes and their Stage-A logical refs."""

    _equal(resolver["sourceSha256"], context.source_hash, "resolver.sourceSha256")
    _validate_aggregate(resolver, SOURCE_UNIT_RESOLVER_DOMAIN, "sourceUnitResolver")
    entries = _objects(resolver["entries"], "resolver.entries")
    _equal(resolver["entryCount"], len(entries), "resolver.entryCount")
    payload_refs: set[str] = set()
    for index, entry in enumerate(entries):
        label = f"sourceUnitResolver.entries[{index}]"
        ordinal = _integer(entry["sourceUnitOrdinal"], f"{label}.sourceUnitOrdinal")
        try:
            unit = units_by_ordinal[ordinal]
        except KeyError as error:
            raise HashDagSemanticsError(
                f"{label}: sourceUnitOrdinal has no normalization-map unit"
            ) from error
        for field in ("opaqueSourceUnitId", "sourceUnitKind"):
            _equal(entry[field], unit[_resolver_unit_field(field)], f"{label}.{field}")
        payload = _object(entry["payload"], f"{label}.payload")
        payload_hash = _jcs_sha256(payload)
        _equal(entry["payloadHash"], payload_hash, f"{label}.payloadHash")
        expected = _logical_hash(
            context,
            "source_unit_payload_ref",
            {
                "opaqueSourceUnitId": entry["opaqueSourceUnitId"],
                "payloadHash": payload_hash,
                "payloadKind": entry["payloadKind"],
                "payloadKindRank": entry["payloadKindRank"],
                "payloadOrdinal": entry["payloadOrdinal"],
            },
        )
        observed = _string(
            entry["sourceUnitPayloadRef"], f"{label}.sourceUnitPayloadRef"
        )
        _equal(observed, expected, f"{label}.source_unit_payload_ref")
        _equal(
            unit["displayMetadataPayloadRef"],
            observed,
            f"{label}.displayMetadataPayloadRef",
        )
        payload_refs.add(observed)
    return payload_refs


def _validate_stages_b_c1_c2_c3(
    context: _HashContext,
    ir: JsonObject,
    chunks: JsonObject,
    source_unit_ids: set[str],
) -> set[str]:
    owner_seed_ids, blocks, block_by_id = _validate_stage_b(context, ir)
    _validate_stages_c1_c2(
        context,
        ir,
        chunks,
        block_by_id,
        source_unit_ids,
    )
    _validate_stage_c3(context, ir, chunks, blocks, block_by_id)
    return owner_seed_ids


def _validate_stage_b(
    context: _HashContext,
    ir: JsonObject,
) -> tuple[set[str], list[JsonObject], dict[str, JsonObject]]:
    """Recompute flow and owner seeds without consuming later-stage IDs."""
    pages = _objects(ir["pages"], "canonicalIr.pages")
    geometry = [
        {
            "heightMilliPoint": page["heightMilliPoint"],
            "pageIndex": page["pageIndex"],
            "rotationDegrees": page["rotationDegrees"],
            "widthMilliPoint": page["widthMilliPoint"],
        }
        for page in pages
    ]
    flows = _objects(ir["readingFlows"], "canonicalIr.readingFlows")
    flow_seed_ids: set[str] = set()
    for ordinal, flow in enumerate(flows):
        label = f"canonicalIr.readingFlows[{ordinal}]"
        _equal(flow["flowOrdinal"], ordinal, f"{label}.flowOrdinal")
        expected = _logical_hash(
            context,
            "flow_seed_id",
            {
                "flowOrdinal": ordinal,
                "geometryHash": _jcs_sha256(cast("JsonValue", geometry)),
                "parserProfileHash": context.parser_profile_hash,
                "sourceHash": context.source_hash,
            },
        )
        observed = _string(flow["flowSeedId"], f"{label}.flowSeedId")
        _equal(observed, expected, f"{label}.flow_seed_id")
        flow_seed_ids.add(observed)

    blocks = _objects(ir["blocks"], "canonicalIr.blocks")
    owner_seed_ids: set[str] = set()
    block_by_id = {
        _string(block["logicalBlockId"], "block.logicalBlockId"): block
        for block in blocks
    }
    for index, block in enumerate(blocks):
        label = f"canonicalIr.blocks[{index}]"
        structure = _object(block["structureRef"], f"{label}.structureRef")
        kind = _string(structure["structureKind"], f"{label}.structureKind")
        ordinal = _integer(structure["structureOrdinal"], f"{label}.structureOrdinal")
        structural_path: list[JsonValue] = [
            {"nodeKind": "document", "nodeOrdinal": 0, "pathOrdinal": 0},
            {"nodeKind": kind, "nodeOrdinal": ordinal, "pathOrdinal": 1},
        ]
        expected = _logical_hash(
            context,
            "owner_seed_id",
            {
                "nodeKind": kind,
                "nodeOrdinal": ordinal,
                "parserProfileHash": context.parser_profile_hash,
                "sourceHash": context.source_hash,
                "structuralPath": structural_path,
            },
        )
        observed = _string(structure["ownerSeedId"], f"{label}.ownerSeedId")
        _equal(observed, expected, f"{label}.owner_seed_id")
        owner_seed_ids.add(observed)
        if block["flowSeedId"] not in flow_seed_ids:
            _fail(label, "block references a missing Stage-B flowSeedId")
    return owner_seed_ids, blocks, block_by_id


def _validate_stages_c1_c2(
    context: _HashContext,
    ir: JsonObject,
    chunks: JsonObject,
    block_by_id: dict[str, JsonObject],
    source_unit_ids: set[str],
) -> None:
    """Validate locator aggregates and all Stage-A/B/C1 references in C2."""
    for label, locator, block in _locator_contexts(ir, chunks, block_by_id):
        _validate_aggregate(locator, SOURCE_LOCATOR_DOMAIN, label)
        structure = _object(block["structureRef"], f"{label}.structureRef")
        owner_seed_id = _string(structure["ownerSeedId"], f"{label}.ownerSeedId")
        structure_kind = _string(structure["structureKind"], f"{label}.structureKind")
        structure_ordinal = _integer(
            structure["structureOrdinal"], f"{label}.structureOrdinal"
        )
        for fragment in _ordered_source_fragments(locator, label):
            for view in _objects(fragment["views"], f"{label}.views"):
                view_kind = view["kind"]
                if view_kind == "source_text_position":
                    if view["opaqueSourceUnitId"] not in source_unit_ids:
                        _fail(
                            label, "C2 locator references a missing Stage-A source unit"
                        )
                elif view_kind == "derived_structure":
                    if view["structureKind"] != structure_kind:
                        _fail(label, "C2 derived structure kind does not match owner")
                    expected = _logical_hash(
                        context,
                        "opaque_structure_id",
                        {
                            "ownerSeedId": owner_seed_id,
                            "sourceHash": context.source_hash,
                            "structureKind": structure_kind,
                            "structureKindRank": _structure_kind_rank(structure_kind),
                            "structureOrdinal": structure_ordinal,
                        },
                    )
                    _equal(
                        view["opaqueStructureId"],
                        expected,
                        f"{label}.opaque_structure_id",
                    )


def _validate_stage_c3(
    context: _HashContext,
    ir: JsonObject,
    chunks: JsonObject,
    blocks: list[JsonObject],
    block_by_id: dict[str, JsonObject],
) -> None:
    """Recompute every block and chunk-fragment text source-span hash."""
    text = _string(
        _object(ir["textBuffer"], "textBuffer")["text"], "textBuffer.text"
    ).encode()
    for index, block in enumerate(blocks):
        label = f"canonicalIr.blocks[{index}]"
        text_range = _object(block["textRange"], f"{label}.textRange")
        start = _integer(text_range["startByte"], f"{label}.startByte")
        end = _integer(text_range["endByte"], f"{label}.endByte")
        locator = _object(block["locatorSet"], f"{label}.locatorSet")
        expected = _text_source_span_hash(context, locator, text, start, end)
        source_span = _object(block["sourceSpanHash"], f"{label}.sourceSpanHash")
        _equal(
            source_span["textSourceSpanHash"],
            expected,
            f"{label}.text_source_span_hash",
        )
        _equal(block["contentHash"], _sha256(text[start:end]), f"{label}.contentHash")

    for chunk_label, chunk in _all_chunks(chunks):
        for index, fragment in enumerate(
            _objects(chunk["spanFragments"], f"{chunk_label}.spanFragments")
        ):
            label = f"{chunk_label}.spanFragments[{index}]"
            block_id = _string(fragment["blockLogicalId"], f"{label}.blockLogicalId")
            try:
                block = block_by_id[block_id]
            except KeyError as error:
                raise HashDagSemanticsError(
                    f"{label}: fragment references a missing Stage-D block"
                ) from error
            block_range = _object(block["textRange"], f"{label}.block.textRange")
            start = _integer(
                block_range["startByte"], f"{label}.block.start"
            ) + _integer(fragment["blockStartByte"], f"{label}.blockStartByte")
            end = _integer(block_range["startByte"], f"{label}.block.start") + _integer(
                fragment["blockEndByte"], f"{label}.blockEndByte"
            )
            locator = _object(fragment["clippedLocatorSet"], f"{label}.locator")
            expected = _text_source_span_hash(context, locator, text, start, end)
            _equal(
                fragment["fragmentSourceSpanHash"],
                expected,
                f"{label}.text_source_span_hash",
            )


def _validate_stage_c4(
    context: _HashContext,
    ir: JsonObject,
    source_unit_ids: set[str],
    payload_refs: set[str],
    owner_seed_ids: set[str],
) -> set[str]:
    provenance_ids: set[str] = set()
    provenance = _objects(ir["provenance"], "canonicalIr.provenance")
    for index, record in enumerate(provenance):
        label = f"canonicalIr.provenance[{index}]"
        if record["sourceUnitRef"] not in source_unit_ids:
            _fail(label, "C4 sourceUnitRef is not a completed Stage-A ID")
        if record["payloadRef"] not in payload_refs:
            _fail(label, "C4 payloadRef is not a completed Stage-A payload ref")
        if record["targetOwnerSeedId"] not in owner_seed_ids:
            _fail(label, "C4 target owner is not a completed Stage-B seed")
        provenance_payload: JsonObject = {
            field: record[field]
            for field in (
                "derivationProfileHash",
                "payloadRef",
                "provenanceKind",
                "provenanceOrdinal",
                "sourceUnitRef",
                "targetKind",
                "targetKindRank",
                "targetOwnerSeedId",
            )
        }
        expected_hash = _logical_hash(context, "provenance_hash", provenance_payload)
        _equal(record["provenanceHash"], expected_hash, f"{label}.provenance_hash")
        expected_id = _logical_hash(
            context,
            "provenance_id",
            {
                "provenanceHash": expected_hash,
                "provenanceOrdinal": record["provenanceOrdinal"],
                "targetOwnerSeedId": record["targetOwnerSeedId"],
            },
        )
        observed = _string(record["provenanceId"], f"{label}.provenanceId")
        _equal(observed, expected_id, f"{label}.provenance_id")
        provenance_ids.add(observed)
    return provenance_ids


def _validate_stages_d_e(
    context: _HashContext,
    ir: JsonObject,
    owner_seed_ids: set[str],
    provenance_ids: set[str],
) -> tuple[set[str], set[str]]:
    completed_blocks: set[str] = set()
    blocks = _objects(ir["blocks"], "canonicalIr.blocks")
    for index, block in enumerate(blocks):
        label = f"canonicalIr.blocks[{index}]"
        owner = _object(block["structureRef"], f"{label}.structureRef")["ownerSeedId"]
        if owner not in owner_seed_ids:
            _fail(label, "D block owner is not a completed Stage-B seed")
        for ref in _strings(block["provenanceRefs"], f"{label}.provenanceRefs"):
            if ref not in provenance_ids:
                _fail(label, "D block references a missing Stage-C4 provenance ID")
        parent_id = block["parentBlockId"]
        if parent_id is not None and parent_id not in completed_blocks:
            _fail(label, "D parentBlockId is not an already completed block")
        for heading_id in _strings(block["headingPath"], f"{label}.headingPath"):
            if heading_id not in completed_blocks:
                _fail(label, "D headingPath has a forward or missing block reference")
        block_envelope = block.copy()
        observed = _string(
            block_envelope.pop("logicalBlockId"), f"{label}.logicalBlockId"
        )
        expected = _logical_hash(
            context,
            "logical_block_id",
            {
                "blockEnvelopeHash": _jcs_sha256(block_envelope),
                "blockOrdinal": block["ordinal"],
                "parserProfileHash": context.parser_profile_hash,
                "sourceHash": context.source_hash,
            },
        )
        _equal(observed, expected, f"{label}.logical_block_id")
        completed_blocks.add(observed)

    flow_ids: set[str] = set()
    for index, flow in enumerate(_objects(ir["readingFlows"], "readingFlows")):
        label = f"canonicalIr.readingFlows[{index}]"
        ordered_ids = _strings(
            flow["orderedLogicalBlockIds"], f"{label}.orderedLogicalBlockIds"
        )
        if any(block_id not in completed_blocks for block_id in ordered_ids):
            _fail(label, "E flow references a missing Stage-D block")
        expected = _logical_hash(
            context,
            "logical_flow_id",
            {
                "flowSeedId": flow["flowSeedId"],
                "orderedLogicalBlockIds": cast("JsonValue", ordered_ids),
            },
        )
        observed = _string(flow["logicalFlowId"], f"{label}.logicalFlowId")
        _equal(observed, expected, f"{label}.logical_flow_id")
        flow_ids.add(observed)
    return completed_blocks, flow_ids


def _validate_stage_f(
    context: _HashContext,
    ir: JsonObject,
    chunks: JsonObject,
    owner_seed_ids: set[str],
    block_ids: set[str],
    flow_ids: set[str],
) -> None:
    source = _object(ir["source"], "canonicalIr.source")
    _equal(chunks["sourceSha256"], context.source_hash, "chunks.sourceSha256")
    chunk_profile_hash = _expected_chunk_profile_hash()
    _equal(chunks["chunkProfileHash"], chunk_profile_hash, "chunks.chunkProfileHash")

    parents = _objects(chunks["parents"], "chunks.parents")
    parent_ids: set[str] = set()
    parents_by_id: dict[str, JsonObject] = {}
    for index, parent in enumerate(parents):
        label = f"chunks.parents[{index}]"
        owner = _string(parent["sectionOwnerSeedId"], f"{label}.sectionOwnerSeedId")
        if owner not in owner_seed_ids:
            _fail(label, "F parent section owner is not a completed Stage-B seed")
        expected_seed = _logical_hash(
            context,
            "parent_chunk_seed_id",
            {
                "chunkProfileHash": chunk_profile_hash,
                "parentOrdinal": parent["parentOrdinal"],
                "sectionOwnerSeedId": owner,
                "sourceHash": source["sha256"],
            },
        )
        _equal(
            parent["parentChunkSeedId"], expected_seed, f"{label}.parent_chunk_seed_id"
        )
        _validate_chunk_dependencies(parent, label, block_ids, flow_ids)
        _validate_chunk_source_span(context, parent, label, chunk_profile_hash)
        expected = _logical_hash(
            context,
            "logical_parent_chunk_id",
            _chunk_id_payload(parent),
        )
        observed = _string(parent["logicalChunkId"], f"{label}.logicalChunkId")
        _equal(observed, expected, f"{label}.logical_parent_chunk_id")
        parent_ids.add(observed)
        parents_by_id[observed] = parent

    for index, child in enumerate(_objects(chunks["children"], "chunks.children")):
        label = f"chunks.children[{index}]"
        parent_id = _string(
            child["logicalParentChunkId"], f"{label}.logicalParentChunkId"
        )
        try:
            parent = parents_by_id[parent_id]
        except KeyError as error:
            raise HashDagSemanticsError(
                f"{label}: child references a missing completed parent chunk"
            ) from error
        _equal(
            child["parentChunkSeedId"],
            parent["parentChunkSeedId"],
            f"{label}.parentChunkSeedId",
        )
        _validate_chunk_dependencies(child, label, block_ids, flow_ids)
        _validate_chunk_source_span(context, child, label, chunk_profile_hash)
        expected = _logical_hash(
            context,
            "logical_child_chunk_id",
            _chunk_id_payload(child),
        )
        _equal(child["logicalChunkId"], expected, f"{label}.logical_child_chunk_id")

    spans = [
        fragment
        for _label, chunk in _all_chunks(chunks)
        for fragment in _objects(chunk["spanFragments"], "chunk.spanFragments")
    ]
    joiners = [
        joiner
        for _label, chunk in _all_chunks(chunks)
        for joiner in _objects(chunk["joiners"], "chunk.joiners")
    ]
    aggregate_bindings = (
        ("parentAggregateHash", parents),
        ("childAggregateHash", _objects(chunks["children"], "children")),
        ("spanAggregateHash", spans),
        ("joinerAggregateHash", joiners),
    )
    _equal(chunks["parentCount"], len(parents), "chunks.parentCount")
    _equal(
        chunks["childCount"],
        len(_objects(chunks["children"], "children")),
        "chunks.childCount",
    )
    _equal(chunks["spanCount"], len(spans), "chunks.spanCount")
    _equal(chunks["joinerCount"], len(joiners), "chunks.joinerCount")
    for field, records in aggregate_bindings:
        _equal(
            chunks[field],
            _jcs_sha256(cast("JsonValue", records)),
            f"chunks.{field}",
        )


def _validate_quality_report(
    artifacts: ParserHashDagArtifactSet,
    context: _HashContext,
    inventory: _LogicalInventory,
) -> None:
    quality = artifacts.quality_report.document
    ir = artifacts.canonical_ir.document
    chunks = artifacts.chunk_manifest.document
    _equal(quality["sourceSha256"], context.source_hash, "quality.sourceSha256")
    _equal(
        quality["normalizationProfileHash"],
        context.normalization_profile_hash,
        "quality.normalizationProfileHash",
    )
    _equal(
        quality["parserProfileHash"],
        context.parser_profile_hash,
        "quality.parserProfileHash",
    )
    artifact_hashes = _object(quality["artifactHashes"], "quality.artifactHashes")
    expected_artifact_hashes: JsonObject = {
        "canonicalIrSha256": _sha256(artifacts.canonical_ir.content),
        "chunkManifestSha256": _sha256(artifacts.chunk_manifest.content),
        "normalizationMapSha256": _sha256(artifacts.normalization_map.content),
    }
    for field, expected in expected_artifact_hashes.items():
        _equal(artifact_hashes[field], expected, f"quality.artifactHashes.{field}")
    determinism_runs: list[JsonValue] = [
        {"runOrdinal": ordinal, **expected_artifact_hashes} for ordinal in range(10)
    ]
    _equal(
        artifact_hashes["determinismAggregateHash"],
        _jcs_sha256(determinism_runs),
        "quality.artifactHashes.determinismAggregateHash",
    )
    for index, gate in enumerate(_objects(quality["gateResults"], "quality.gates")):
        payload = gate.copy()
        observed = payload.pop("evidenceHash")
        _equal(
            observed, _jcs_sha256(payload), f"quality.gateResults[{index}].evidenceHash"
        )

    entity_arrays = _entity_arrays(ir)
    locators = [
        _object(block["locatorSet"], "block.locatorSet")
        for block in _objects(ir["blocks"], "blocks")
    ]
    metrics = _object(quality["metrics"], "quality.metrics")
    expected_metrics: dict[str, int | bool] = {
        "sourceBytes": _integer(
            _object(ir["source"], "source")["bytes"], "source.bytes"
        ),
        "textBufferBytes": _integer(
            _object(ir["textBuffer"], "textBuffer")["bytes"], "textBuffer.bytes"
        ),
        "pageCount": len(entity_arrays["pages"]),
        "readingFlowCount": len(entity_arrays["readingFlows"]),
        "blockCount": len(inventory.block_ids),
        "tableCount": len(entity_arrays["tables"]),
        "cellCount": len(entity_arrays["cells"]),
        "formulaCount": len(entity_arrays["formulas"]),
        "assetCount": len(entity_arrays["assets"]),
        "textAnchorCount": sum(
            len(_objects(locator["textAnchors"], "textAnchors")) for locator in locators
        ),
        "structuralAnchorCount": sum(
            len(_objects(locator["structuralAnchors"], "structuralAnchors"))
            for locator in locators
        ),
        "parentChunkCount": _integer(chunks["parentCount"], "chunks.parentCount"),
        "childChunkCount": _integer(chunks["childCount"], "chunks.childCount"),
        "spanCount": _integer(chunks["spanCount"], "chunks.spanCount"),
        "joinerCount": _integer(chunks["joinerCount"], "chunks.joinerCount"),
        "determinismRunCount": 10,
        "sourceProvenEmpty": False,
        "structuralContentEmpty": False,
    }
    for field, expected in expected_metrics.items():
        _equal(metrics[field], expected, f"quality.metrics.{field}")


def _validate_manifest(
    artifacts: ParserHashDagArtifactSet,
    context: _HashContext,
    inventory: _LogicalInventory,
) -> None:
    manifest = artifacts.canonical_manifest.document
    ir = artifacts.canonical_ir.document
    normalization_map = artifacts.normalization_map.document
    resolver = artifacts.source_unit_resolver.document
    quality = artifacts.quality_report.document
    chunks = artifacts.chunk_manifest.document

    _equal(manifest["source"], ir["source"], "manifest.source")
    _equal(manifest["configHash"], context.config_hash, "manifest.configHash")
    _equal(
        manifest["parserProfileHash"],
        context.parser_profile_hash,
        "manifest.parserProfileHash",
    )
    _equal(
        manifest["tokenizerProfileHash"],
        _expected_tokenizer_profile_hash(),
        "manifest.tokenizerProfileHash",
    )
    _equal(
        manifest["chunkProfileHash"],
        _expected_chunk_profile_hash(),
        "manifest.chunkProfileHash",
    )
    _bind_file_descriptor(
        _object(manifest["canonicalIr"], "manifest.canonicalIr"),
        artifacts.canonical_ir,
        "canonical-ir.v2",
        "manifest.canonicalIr",
    )
    _bind_file_descriptor(
        _object(manifest["normalizationMap"], "manifest.normalizationMap"),
        artifacts.normalization_map,
        "normalization-map.v1",
        "manifest.normalizationMap",
    )
    _bind_file_descriptor(
        _object(manifest["sourceUnitResolver"], "manifest.sourceUnitResolver"),
        artifacts.source_unit_resolver,
        "source-unit-resolver.v1",
        "manifest.sourceUnitResolver",
    )
    _bind_file_descriptor(
        _object(manifest["qualityReport"], "manifest.qualityReport"),
        artifacts.quality_report,
        "quality-report.v2",
        "manifest.qualityReport",
    )
    _bind_file_descriptor(
        _object(manifest["chunks"], "manifest.chunks"),
        artifacts.chunk_manifest,
        "chunk-manifest.v2",
        "manifest.chunks",
    )

    text = _string(_object(ir["textBuffer"], "textBuffer")["text"], "text").encode()
    text_descriptor = _object(manifest["textBuffer"], "manifest.textBuffer")
    _equal(text_descriptor["bytes"], len(text), "manifest.textBuffer.bytes")
    _equal(text_descriptor["sha256"], _sha256(text), "manifest.textBuffer.sha256")

    normalization_descriptor = _object(
        manifest["normalizationMap"], "manifest.normalizationMap"
    )
    _equal(
        normalization_descriptor["sourceUnitCount"],
        len(inventory.source_unit_ids),
        "manifest.normalizationMap.sourceUnitCount",
    )
    _equal(
        normalization_descriptor["segmentCount"],
        len(_objects(normalization_map["segments"], "segments")),
        "manifest.normalizationMap.segmentCount",
    )
    _equal(
        normalization_descriptor["aggregateHash"],
        normalization_map["aggregateHash"],
        "manifest.normalizationMap.aggregateHash",
    )
    resolver_descriptor = _object(
        manifest["sourceUnitResolver"], "manifest.sourceUnitResolver"
    )
    _equal(
        resolver_descriptor["entryCount"],
        len(_objects(resolver["entries"], "resolver.entries")),
        "manifest.sourceUnitResolver.entryCount",
    )
    _equal(
        resolver_descriptor["aggregateHash"],
        resolver["aggregateHash"],
        "manifest.sourceUnitResolver.aggregateHash",
    )
    quality_descriptor = _object(manifest["qualityReport"], "manifest.qualityReport")
    _equal(quality_descriptor["outcome"], quality["outcome"], "qualityReport.outcome")

    chunk_descriptor = _object(manifest["chunks"], "manifest.chunks")
    for field in (
        "parentCount",
        "childCount",
        "spanCount",
        "joinerCount",
        "parentAggregateHash",
        "childAggregateHash",
        "spanAggregateHash",
        "joinerAggregateHash",
    ):
        _equal(chunk_descriptor[field], chunks[field], f"manifest.chunks.{field}")

    entities = _entity_arrays(ir)
    counts = _object(manifest["entityCounts"], "manifest.entityCounts")
    aggregates = _object(
        manifest["entityAggregateHashes"], "manifest.entityAggregateHashes"
    )
    for name, records in entities.items():
        _equal(counts[_COUNT_FIELDS[name]], len(records), f"entityCounts.{name}")
        _equal(
            aggregates[name],
            _jcs_sha256(cast("JsonValue", records)),
            f"entityAggregateHashes.{name}",
        )

    schema_hashes = _object(manifest["schemaHashes"], "manifest.schemaHashes")
    for field, schema_name in _SCHEMA_HASH_FIELDS.items():
        _equal(
            schema_hashes[field],
            _sha256(read_schema_bytes(schema_name)),
            f"manifest.schemaHashes.{field}",
        )
    normalization_ref = _object(ir["normalizationMapRef"], "normalizationMapRef")
    for field in ("schemaVersion", "bytes", "sha256"):
        _equal(
            normalization_ref[field],
            normalization_descriptor[field],
            f"canonicalIr.normalizationMapRef.{field}",
        )


def _logical_hash(context: _HashContext, kind: str, payload: JsonObject) -> str:
    envelope: JsonObject = {
        "canonicalization": "rfc8785",
        "configHash": context.config_hash,
        "domain": _DOMAINS[kind],
        "envelopeKind": kind,
        "hashAlgorithm": "sha256",
        "normalizationProfileHash": context.normalization_profile_hash,
        "parserBuildHash": context.parser_build_hash,
        "payload": payload,
        "schemaVersion": "logical-hash-envelope.v1",
    }
    validate_schema_instance(
        context.packaged,
        "logical-hash-envelope.v1.schema.json",
        envelope,
    )
    return logical_hash_envelope_sha256(envelope)


def _text_source_span_hash(
    context: _HashContext,
    locator: JsonObject,
    text: bytes,
    start: int,
    end: int,
) -> str:
    fragments = _ordered_source_fragments(locator, "text source span")
    return _logical_hash(
        context,
        "text_source_span_hash",
        {
            "canonicalEndByte": end,
            "canonicalStartByte": start,
            "canonicalTextHash": _sha256(text[start:end]),
            "locatorSetHash": locator["aggregateHash"],
            "orderedSourceFragmentHashes": [
                _jcs_sha256(fragment) for fragment in fragments
            ],
            "sourceHash": context.source_hash,
        },
    )


def _validate_chunk_source_span(
    context: _HashContext,
    chunk: JsonObject,
    label: str,
    chunk_profile_hash: str,
) -> None:
    fragments = _objects(chunk["spanFragments"], f"{label}.spanFragments")
    joiners = _objects(chunk["joiners"], f"{label}.joiners")
    expected = _logical_hash(
        context,
        "chunk_source_span_hash",
        {
            "chunkProfileHash": chunk_profile_hash,
            "fragmentSourceSpanHashes": cast(
                "list[JsonValue]",
                [fragment["fragmentSourceSpanHash"] for fragment in fragments],
            ),
            "joiners": cast("JsonValue", joiners),
        },
    )
    _equal(chunk["chunkSourceSpanHash"], expected, f"{label}.chunk_source_span_hash")


def _validate_chunk_dependencies(
    chunk: JsonObject,
    label: str,
    block_ids: set[str],
    flow_ids: set[str],
) -> None:
    if chunk["logicalFlowId"] not in flow_ids:
        _fail(label, "F chunk references a missing Stage-E flow")
    for fragment in _objects(chunk["spanFragments"], f"{label}.spanFragments"):
        if fragment["blockLogicalId"] not in block_ids:
            _fail(label, "F chunk fragment references a missing Stage-D block")


def _chunk_id_payload(chunk: JsonObject) -> JsonObject:
    payload: JsonObject = {
        "chunkKind": chunk["chunkKind"],
        "logicalFlowId": chunk["logicalFlowId"],
        "parentChunkSeedId": chunk["parentChunkSeedId"],
        "chunkProfileHash": chunk["chunkProfileHash"],
        "spanFragments": [
            _logical_fragment(fragment)
            for fragment in _objects(chunk["spanFragments"], "spanFragments")
        ],
        "joiners": chunk["joiners"],
        "contentHash": chunk["contentHash"],
    }
    ordinal_field = (
        "parentOrdinal" if chunk["chunkKind"] == "parent" else "childOrdinal"
    )
    payload[ordinal_field] = chunk[ordinal_field]
    return payload


def _logical_fragment(fragment: JsonObject) -> JsonObject:
    result: JsonObject = {
        "blockEndByte": fragment["blockEndByte"],
        "blockLogicalId": fragment["blockLogicalId"],
        "blockStartByte": fragment["blockStartByte"],
        "clippedLocatorSetHash": _object(
            fragment["clippedLocatorSet"], "clippedLocatorSet"
        )["aggregateHash"],
        "fragmentKind": fragment["fragmentKind"],
        "fragmentSourceSpanHash": fragment["fragmentSourceSpanHash"],
    }
    if fragment["fragmentKind"] == "window_overlap":
        for field in ("previousChildOrdinal", "overlapGroupId", "overlapTokenCount"):
            result[field] = fragment[field]
    elif fragment["fragmentKind"] == "derived_context":
        for field in (
            "derived",
            "derivedReason",
            "sourceReuseGroupId",
            "originalFragmentSourceSpanHash",
        ):
            result[field] = fragment[field]
    return result


def _locator_contexts(
    ir: JsonObject,
    chunks: JsonObject,
    block_by_id: dict[str, JsonObject],
) -> list[tuple[str, JsonObject, JsonObject]]:
    result = [
        (
            f"canonicalIr.blocks[{index}].locatorSet",
            _object(block["locatorSet"], "block.locatorSet"),
            block,
        )
        for index, block in enumerate(_objects(ir["blocks"], "blocks"))
    ]
    for chunk_label, chunk in _all_chunks(chunks):
        for index, fragment in enumerate(
            _objects(chunk["spanFragments"], f"{chunk_label}.spanFragments")
        ):
            block_id = _string(fragment["blockLogicalId"], "fragment.blockLogicalId")
            if block_id not in block_by_id:
                continue
            result.append(
                (
                    f"{chunk_label}.spanFragments[{index}].clippedLocatorSet",
                    _object(fragment["clippedLocatorSet"], "clippedLocatorSet"),
                    block_by_id[block_id],
                )
            )
    return result


def _ordered_source_fragments(locator: JsonObject, label: str) -> list[JsonObject]:
    result: list[JsonObject] = []
    for anchor in [
        *_objects(locator["textAnchors"], f"{label}.textAnchors"),
        *_objects(locator["structuralAnchors"], f"{label}.structuralAnchors"),
    ]:
        result.extend(_objects(anchor["sourceFragments"], f"{label}.sourceFragments"))
    return result


def _all_chunks(chunks: JsonObject) -> list[tuple[str, JsonObject]]:
    return [
        *(
            (f"chunks.parents[{index}]", chunk)
            for index, chunk in enumerate(_objects(chunks["parents"], "parents"))
        ),
        *(
            (f"chunks.children[{index}]", chunk)
            for index, chunk in enumerate(_objects(chunks["children"], "children"))
        ),
    ]


def _entity_arrays(ir: JsonObject) -> dict[str, list[JsonObject]]:
    tables = _objects(ir["tables"], "tables")
    return {
        "pages": _objects(ir["pages"], "pages"),
        "readingFlows": _objects(ir["readingFlows"], "readingFlows"),
        "blocks": _objects(ir["blocks"], "blocks"),
        "tables": tables,
        "cells": [
            cell for table in tables for cell in _objects(table["cells"], "cells")
        ],
        "formulas": _objects(ir["formulas"], "formulas"),
        "assets": _objects(ir["assets"], "assets"),
        "provenance": _objects(ir["provenance"], "provenance"),
    }


def _bind_file_descriptor(
    descriptor: JsonObject,
    artifact: ExactArtifact,
    schema_version: str,
    label: str,
) -> None:
    _equal(descriptor["schemaVersion"], schema_version, f"{label}.schemaVersion")
    _equal(descriptor["bytes"], len(artifact.content), f"{label}.bytes")
    _equal(descriptor["sha256"], _sha256(artifact.content), f"{label}.sha256")


def _validate_aggregate(value: JsonObject, domain: str, label: str) -> None:
    payload = value.copy()
    observed = payload.pop("aggregateHash")
    _equal(observed, domain_separated_sha256(domain, payload), f"{label}.aggregateHash")


def _expected_tokenizer_profile_hash() -> str:
    return _jcs_sha256({"tokenizer": "hash-dag-character-count", "revision": 1})


def _expected_chunk_profile_hash() -> str:
    return _jcs_sha256(
        {
            "chunker": "hash-dag-two-block",
            "revision": 1,
            "tokenizerProfileHash": _expected_tokenizer_profile_hash(),
        }
    )


def _source_unit_kind_rank(value: JsonValue, label: str) -> int:
    ranks = {"raw_file": 0, "ooxml_part": 1, "synthetic_mineru_artifact": 2}
    kind = _string(value, label)
    if kind not in ranks:
        _fail(label, "unknown source-unit kind")
    return ranks[kind]


def _structure_kind_rank(kind: str) -> int:
    try:
        return _STRUCTURE_KIND_RANK[kind]
    except KeyError as error:
        raise HashDagSemanticsError(f"unknown structure kind: {kind}") from error


def _resolver_unit_field(field: str) -> str:
    return "kind" if field == "sourceUnitKind" else field


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _jcs_sha256(value: JsonValue) -> str:
    return _sha256(canonical_json_bytes(value))


def _object(value: JsonValue, label: str) -> JsonObject:
    if not isinstance(value, dict):
        _fail(label, "must be an object")
    return cast("JsonObject", value)


def _objects(value: JsonValue, label: str) -> list[JsonObject]:
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        _fail(label, "must be an array of objects")
    return cast("list[JsonObject]", value)


def _strings(value: JsonValue, label: str) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        _fail(label, "must be an array of strings")
    return cast("list[str]", value)


def _string(value: JsonValue, label: str) -> str:
    if not isinstance(value, str):
        _fail(label, "must be a string")
    return cast("str", value)


def _integer(value: JsonValue, label: str) -> int:
    if type(value) is not int:
        _fail(label, "must be an integer")
    return cast("int", value)


def _equal(observed: object, expected: object, label: str) -> None:
    if observed != expected:
        _fail(label, "does not match its recomputed binding")


def _fail(label: str, message: str) -> None:
    raise HashDagSemanticsError(f"{label}: {message}")
