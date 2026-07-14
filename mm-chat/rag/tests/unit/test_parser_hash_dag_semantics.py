from __future__ import annotations

import copy
import hashlib
import re
from collections.abc import Iterator
from pathlib import Path
from typing import cast

import pytest
from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    domain_separated_sha256,
    load_packaged_schemas,
    load_strict_json_bytes,
    logical_hash_envelope_sha256,
    validate_schema_instance,
)
from tests.support.parser_hash_dag_semantics import (
    NORMALIZATION_MAP_DOMAIN,
    SOURCE_LOCATOR_DOMAIN,
    SOURCE_UNIT_RESOLVER_DOMAIN,
    HashDagSemanticsError,
    ParserHashDagArtifactSet,
    load_parser_hash_dag_artifact_set,
    validate_parser_hash_dag_semantics,
)

_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "hash_dag"
)
_FILENAMES = {
    "canonical-ir.v2.json",
    "canonical-manifest.v2.json",
    "chunk-manifest.v2.json",
    "normalization-map.v1.json",
    "quality-report.v2.json",
    "source-unit-resolver.v1.json",
}
_MUTATED_HASH = hashlib.sha256(b"hash-dag-semantic-mutation").hexdigest()
_SHA256 = re.compile(r"^[0-9a-f]{64}$")


@pytest.fixture
def artifacts() -> ParserHashDagArtifactSet:
    return load_parser_hash_dag_artifact_set(_FIXTURE_ROOT)


def test_hash_dag_fixture_is_complete_exact_jcs_and_non_placeholder(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    assert {path.name for path in _FIXTURE_ROOT.glob("*.json")} == _FILENAMES

    for path in sorted(_FIXTURE_ROOT.glob("*.json")):
        content = path.read_bytes()
        document = load_strict_json_bytes(content)
        assert canonical_json_bytes(document) == content
        assert not content.endswith(b"\n")
        for value in _walk(document):
            if isinstance(value, str) and _SHA256.fullmatch(value):
                assert value != "0" * 64

    validate_parser_hash_dag_semantics(
        artifacts,
        packaged_schemas=load_packaged_schemas(),
    )


_MANIFEST_BINDING_MUTATIONS = [
    pytest.param(("source", "bytes"), 42, id="source-bytes"),
    pytest.param(("source", "sha256"), _MUTATED_HASH, id="source-sha256"),
    pytest.param(("source", "format"), "markdown", id="source-format"),
    pytest.param(("configHash",), _MUTATED_HASH, id="config-hash"),
    pytest.param(("parserProfileHash",), _MUTATED_HASH, id="parser-profile"),
    pytest.param(("tokenizerProfileHash",), _MUTATED_HASH, id="tokenizer-profile"),
    pytest.param(("chunkProfileHash",), _MUTATED_HASH, id="chunk-profile"),
    pytest.param(("canonicalIr", "bytes"), 42, id="ir-bytes"),
    pytest.param(("canonicalIr", "sha256"), _MUTATED_HASH, id="ir-sha256"),
    pytest.param(("textBuffer", "bytes"), 42, id="text-bytes"),
    pytest.param(("textBuffer", "sha256"), _MUTATED_HASH, id="text-sha256"),
    pytest.param(("normalizationMap", "bytes"), 42, id="normalization-bytes"),
    pytest.param(
        ("normalizationMap", "sha256"), _MUTATED_HASH, id="normalization-sha256"
    ),
    pytest.param(
        ("normalizationMap", "sourceUnitCount"), 42, id="normalization-unit-count"
    ),
    pytest.param(
        ("normalizationMap", "segmentCount"), 42, id="normalization-segment-count"
    ),
    pytest.param(
        ("normalizationMap", "aggregateHash"),
        _MUTATED_HASH,
        id="normalization-aggregate",
    ),
    pytest.param(("sourceUnitResolver", "bytes"), 42, id="resolver-bytes"),
    pytest.param(("sourceUnitResolver", "sha256"), _MUTATED_HASH, id="resolver-sha256"),
    pytest.param(("sourceUnitResolver", "entryCount"), 42, id="resolver-entry-count"),
    pytest.param(
        ("sourceUnitResolver", "aggregateHash"),
        _MUTATED_HASH,
        id="resolver-aggregate",
    ),
    pytest.param(("qualityReport", "bytes"), 42, id="quality-bytes"),
    pytest.param(("qualityReport", "sha256"), _MUTATED_HASH, id="quality-sha256"),
    pytest.param(("qualityReport", "outcome"), "quarantined", id="quality-outcome"),
    pytest.param(("chunks", "bytes"), 42, id="chunks-bytes"),
    pytest.param(("chunks", "sha256"), _MUTATED_HASH, id="chunks-sha256"),
    *[
        pytest.param(("chunks", field), 42, id=f"chunks-{field}")
        for field in ("parentCount", "childCount", "spanCount", "joinerCount")
    ],
    *[
        pytest.param(("chunks", field), _MUTATED_HASH, id=f"chunks-{field}")
        for field in (
            "parentAggregateHash",
            "childAggregateHash",
            "spanAggregateHash",
            "joinerAggregateHash",
        )
    ],
    *[
        pytest.param(("entityCounts", field), 42, id=f"entity-count-{field}")
        for field in (
            "pageCount",
            "readingFlowCount",
            "blockCount",
            "tableCount",
            "cellCount",
            "formulaCount",
            "assetCount",
            "provenanceCount",
        )
    ],
    *[
        pytest.param(
            ("entityAggregateHashes", field),
            _MUTATED_HASH,
            id=f"entity-aggregate-{field}",
        )
        for field in (
            "pages",
            "readingFlows",
            "blocks",
            "tables",
            "cells",
            "formulas",
            "assets",
            "provenance",
        )
    ],
    *[
        pytest.param(("schemaHashes", field), _MUTATED_HASH, id=f"schema-hash-{field}")
        for field in (
            "canonicalManifest",
            "canonicalIr",
            "sourceLocator",
            "normalizationMap",
            "qualityReport",
            "chunkManifest",
            "sourceUnitResolver",
        )
    ],
]


@pytest.mark.parametrize(("path", "replacement"), _MANIFEST_BINDING_MUTATIONS)
def test_every_manifest_byte_hash_count_and_aggregate_binding_fails_closed(
    artifacts: ParserHashDagArtifactSet,
    path: tuple[str | int, ...],
    replacement: JsonValue,
) -> None:
    _set_path(artifacts.canonical_manifest.document, path, replacement)
    _refresh(artifacts, "canonical_manifest")

    with pytest.raises(HashDagSemanticsError, match="manifest|entity|quality"):
        validate_parser_hash_dag_semantics(artifacts)


_QUALITY_BINDING_MUTATIONS = [
    pytest.param(("sourceSha256",), _MUTATED_HASH, id="source-sha256"),
    pytest.param(
        ("normalizationProfileHash",), _MUTATED_HASH, id="normalization-profile"
    ),
    pytest.param(("parserProfileHash",), _MUTATED_HASH, id="parser-profile"),
    *[
        pytest.param(("artifactHashes", field), _MUTATED_HASH, id=f"artifact-{field}")
        for field in (
            "canonicalIrSha256",
            "normalizationMapSha256",
            "chunkManifestSha256",
            "determinismAggregateHash",
        )
    ],
    *[
        pytest.param(
            ("gateResults", index, "evidenceHash"), _MUTATED_HASH, id=f"gate-{index}"
        )
        for index in range(9)
    ],
    *[
        pytest.param(("metrics", field), 42, id=f"metric-{field}")
        for field in (
            "sourceBytes",
            "textBufferBytes",
            "pageCount",
            "readingFlowCount",
            "blockCount",
            "tableCount",
            "cellCount",
            "formulaCount",
            "assetCount",
            "textAnchorCount",
            "structuralAnchorCount",
            "parentChunkCount",
            "childChunkCount",
            "spanCount",
            "joinerCount",
        )
    ],
    pytest.param(("metrics", "sourceProvenEmpty"), True, id="source-empty"),
    pytest.param(("metrics", "structuralContentEmpty"), True, id="structure-empty"),
]


@pytest.mark.parametrize(("path", "replacement"), _QUALITY_BINDING_MUTATIONS)
def test_quality_report_binds_artifacts_evidence_and_all_metrics(
    artifacts: ParserHashDagArtifactSet,
    path: tuple[str | int, ...],
    replacement: JsonValue,
) -> None:
    _set_path(artifacts.quality_report.document, path, replacement)
    _refresh(artifacts, "quality_report")

    with pytest.raises(HashDagSemanticsError, match="quality"):
        validate_parser_hash_dag_semantics(artifacts)


def test_quality_report_determinism_run_count_mutation_is_schema_rejected(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    metrics = _object(artifacts.quality_report.document["metrics"])
    metrics["determinismRunCount"] = 42
    _refresh(artifacts, "quality_report")

    with pytest.raises(ValidationError):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize(
    ("artifact_name", "field", "replacement", "message"),
    [
        pytest.param(
            "normalization_map",
            "aggregateHash",
            _MUTATED_HASH,
            "normalizationMap.aggregateHash",
            id="normalization-aggregate",
        ),
        pytest.param(
            "source_unit_resolver",
            "aggregateHash",
            _MUTATED_HASH,
            "sourceUnitResolver.aggregateHash",
            id="resolver-aggregate",
        ),
        *[
            pytest.param(
                "chunk_manifest",
                field,
                42,
                f"chunks.{field}",
                id=f"chunk-{field}",
            )
            for field in ("parentCount", "childCount", "spanCount", "joinerCount")
        ],
        *[
            pytest.param(
                "chunk_manifest",
                field,
                _MUTATED_HASH,
                f"chunks.{field}",
                id=f"chunk-{field}",
            )
            for field in (
                "parentAggregateHash",
                "childAggregateHash",
                "spanAggregateHash",
                "joinerAggregateHash",
            )
        ],
    ],
)
def test_artifact_local_counts_and_aggregates_are_recomputed(
    artifacts: ParserHashDagArtifactSet,
    artifact_name: str,
    field: str,
    replacement: JsonValue,
    message: str,
) -> None:
    artifact = getattr(artifacts, artifact_name)
    artifact.document[field] = replacement
    _refresh(artifacts, artifact_name)

    with pytest.raises(HashDagSemanticsError, match=re.escape(message)):
        validate_parser_hash_dag_semantics(artifacts)


def test_stage_a_recomputes_opaque_source_unit_id(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    normalization = artifacts.normalization_map.document
    _objects(normalization["sourceUnits"])[0]["opaqueSourceUnitId"] = _MUTATED_HASH
    _rehash_aggregate(normalization, NORMALIZATION_MAP_DOMAIN)
    _refresh(artifacts, "normalization_map")

    with pytest.raises(HashDagSemanticsError, match="opaque_source_unit_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_stage_a_recomputes_source_unit_payload_ref(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    resolver = artifacts.source_unit_resolver.document
    _objects(resolver["entries"])[0]["sourceUnitPayloadRef"] = _MUTATED_HASH
    _rehash_aggregate(resolver, SOURCE_UNIT_RESOLVER_DOMAIN)
    _refresh(artifacts, "source_unit_resolver")

    with pytest.raises(HashDagSemanticsError, match="source_unit_payload_ref"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("block_index", [0, 1])
def test_stage_b_recomputes_every_owner_seed(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> None:
    block = _objects(artifacts.canonical_ir.document["blocks"])[block_index]
    _object(block["structureRef"])["ownerSeedId"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="owner_seed_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_stage_b_recomputes_flow_seed(artifacts: ParserHashDagArtifactSet) -> None:
    flow = _objects(artifacts.canonical_ir.document["readingFlows"])[0]
    flow["flowSeedId"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="flow_seed_id"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("block_index", [0, 1])
def test_stage_c1_recomputes_every_opaque_structure_id(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> None:
    block = _objects(artifacts.canonical_ir.document["blocks"])[block_index]
    locator = _object(block["locatorSet"])
    derived_view = _objects(
        _objects(_objects(locator["textAnchors"])[0]["sourceFragments"])[0]["views"]
    )[1]
    derived_view["opaqueStructureId"] = _MUTATED_HASH
    _rehash_aggregate(locator, SOURCE_LOCATOR_DOMAIN)
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="opaque_structure_id"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("block_index", [0, 1])
def test_stage_c2_recomputes_every_ir_locator_aggregate(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> None:
    block = _objects(artifacts.canonical_ir.document["blocks"])[block_index]
    _object(block["locatorSet"])["aggregateHash"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="locatorSet.aggregateHash"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("block_index", [0, 1])
def test_stage_c3_recomputes_every_block_source_span(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> None:
    block = _objects(artifacts.canonical_ir.document["blocks"])[block_index]
    _object(block["sourceSpanHash"])["textSourceSpanHash"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="text_source_span_hash"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize(
    ("collection", "chunk_index", "fragment_index"),
    [
        ("parents", 0, 0),
        ("parents", 0, 1),
        ("children", 0, 0),
        ("children", 0, 1),
    ],
)
def test_stage_c3_recomputes_every_chunk_fragment_source_span(
    artifacts: ParserHashDagArtifactSet,
    collection: str,
    chunk_index: int,
    fragment_index: int,
) -> None:
    chunks = artifacts.chunk_manifest.document
    chunk = _objects(chunks[collection])[chunk_index]
    fragment = _objects(chunk["spanFragments"])[fragment_index]
    fragment["fragmentSourceSpanHash"] = _MUTATED_HASH
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="text_source_span_hash"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize(
    ("record_index", "field", "message"),
    [
        (0, "provenanceHash", "provenance_hash"),
        (1, "provenanceHash", "provenance_hash"),
        (0, "provenanceId", "provenance_id"),
        (1, "provenanceId", "provenance_id"),
    ],
)
def test_stage_c4_recomputes_every_provenance_hash_and_id(
    artifacts: ParserHashDagArtifactSet,
    record_index: int,
    field: str,
    message: str,
) -> None:
    record = _objects(artifacts.canonical_ir.document["provenance"])[record_index]
    record[field] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match=message):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("block_index", [0, 1])
def test_stage_d_recomputes_every_logical_block_id(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> None:
    ir = artifacts.canonical_ir.document
    blocks = _objects(ir["blocks"])
    old_id = cast("str", blocks[block_index]["logicalBlockId"])
    blocks[block_index]["logicalBlockId"] = _MUTATED_HASH
    flow = _objects(ir["readingFlows"])[0]
    ordered = cast("list[JsonValue]", flow["orderedLogicalBlockIds"])
    ordered[block_index] = _MUTATED_HASH
    if block_index == 0:
        blocks[1]["parentBlockId"] = _MUTATED_HASH
        cast("list[JsonValue]", blocks[1]["headingPath"])[0] = _MUTATED_HASH
    chunks = artifacts.chunk_manifest.document
    for collection in ("parents", "children"):
        for chunk in _objects(chunks[collection]):
            for fragment in _objects(chunk["spanFragments"]):
                if fragment["blockLogicalId"] == old_id:
                    fragment["blockLogicalId"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir", "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="logical_block_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_stage_e_recomputes_logical_flow_id(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    flow = _objects(artifacts.canonical_ir.document["readingFlows"])[0]
    flow["logicalFlowId"] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="logical_flow_id"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize(
    ("collection", "field", "message"),
    [
        ("parents", "parentChunkSeedId", "parent_chunk_seed_id"),
        ("parents", "chunkSourceSpanHash", "chunk_source_span_hash"),
        ("children", "chunkSourceSpanHash", "chunk_source_span_hash"),
        ("parents", "logicalChunkId", "logical_parent_chunk_id"),
        ("children", "logicalChunkId", "logical_child_chunk_id"),
    ],
)
def test_stage_f_recomputes_every_chunk_seed_span_and_logical_id(
    artifacts: ParserHashDagArtifactSet,
    collection: str,
    field: str,
    message: str,
) -> None:
    chunk = _objects(artifacts.chunk_manifest.document[collection])[0]
    chunk[field] = _MUTATED_HASH
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match=message):
        validate_parser_hash_dag_semantics(artifacts)


def test_stage_f_binds_child_seed_to_parent_seed(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    child = _objects(artifacts.chunk_manifest.document["children"])[0]
    child["parentChunkSeedId"] = _MUTATED_HASH
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="parentChunkSeedId"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_a_to_c2_requires_completed_source_unit(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    locator = _block_locator(artifacts, 0)
    source_view = _source_views(locator)[0]
    source_view["opaqueSourceUnitId"] = _MUTATED_HASH
    _rehash_aggregate(locator, SOURCE_LOCATOR_DOMAIN)
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="Stage-A source unit"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_b_to_c1_requires_the_matching_owner_seed(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    first = _block_locator(artifacts, 0)
    second = _block_locator(artifacts, 1)
    first_derived = _source_views(first)[1]
    second_derived = _source_views(second)[1]
    first_derived["opaqueStructureId"] = second_derived["opaqueStructureId"]
    _rehash_aggregate(first, SOURCE_LOCATOR_DOMAIN)
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="opaque_structure_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_c2_to_c3_binds_the_exact_locator_fragment(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    locator = _block_locator(artifacts, 0)
    source_view = _source_views(locator)[0]
    source_view["rawByteEnd"] = 7
    source_view["decodedScalarEnd"] = 7
    source_view["endColumn"] = 7
    _rehash_aggregate(locator, SOURCE_LOCATOR_DOMAIN)
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="text_source_span_hash"):
        validate_parser_hash_dag_semantics(artifacts)


@pytest.mark.parametrize("field", ["sourceUnitRef", "payloadRef", "targetOwnerSeedId"])
def test_dag_edges_a_and_b_to_c4_require_completed_predecessors(
    artifacts: ParserHashDagArtifactSet,
    field: str,
) -> None:
    record = _objects(artifacts.canonical_ir.document["provenance"])[0]
    record[field] = _MUTATED_HASH
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="C4"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_c4_to_d_is_in_the_block_envelope(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    ir = artifacts.canonical_ir.document
    blocks = _objects(ir["blocks"])
    first_ref = cast("list[JsonValue]", blocks[0]["provenanceRefs"])[0]
    second_ref = cast("list[JsonValue]", blocks[1]["provenanceRefs"])[0]
    cast("list[JsonValue]", blocks[0]["provenanceRefs"])[0] = second_ref
    cast("list[JsonValue]", blocks[1]["provenanceRefs"])[0] = first_ref
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="logical_block_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_d_to_d_rejects_changed_parent_ancestry(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    paragraph = _objects(artifacts.canonical_ir.document["blocks"])[1]
    paragraph["parentBlockId"] = None
    paragraph["headingPath"] = []
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="logical_block_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_d_to_e_binds_ordered_completed_blocks(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    flow = _objects(artifacts.canonical_ir.document["readingFlows"])[0]
    ordered = cast("list[JsonValue]", flow["orderedLogicalBlockIds"])
    ordered.reverse()
    _refresh(artifacts, "canonical_ir")

    with pytest.raises(HashDagSemanticsError, match="logical_flow_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_e_to_f_requires_completed_flow(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    chunks = artifacts.chunk_manifest.document
    _objects(chunks["parents"])[0]["logicalFlowId"] = _MUTATED_HASH
    _objects(chunks["children"])[0]["logicalFlowId"] = _MUTATED_HASH
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="Stage-E flow"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_d_to_f_is_in_the_ordered_span_envelope(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    parent = _objects(artifacts.chunk_manifest.document["parents"])[0]
    fragments = _objects(parent["spanFragments"])
    fragments[1] = copy.deepcopy(fragments[0])
    _rehash_chunk_source_span(artifacts, parent)
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="logical_parent_chunk_id"):
        validate_parser_hash_dag_semantics(artifacts)


def test_dag_edge_parent_f_to_child_f_requires_completed_parent(
    artifacts: ParserHashDagArtifactSet,
) -> None:
    child = _objects(artifacts.chunk_manifest.document["children"])[0]
    child["logicalParentChunkId"] = _MUTATED_HASH
    _refresh(artifacts, "chunk_manifest")

    with pytest.raises(HashDagSemanticsError, match="completed parent chunk"):
        validate_parser_hash_dag_semantics(artifacts)


def _refresh(artifacts: ParserHashDagArtifactSet, *artifact_names: str) -> None:
    for artifact_name in artifact_names:
        artifact = getattr(artifacts, artifact_name)
        artifact.content = canonical_json_bytes(artifact.document)


def _rehash_aggregate(document: JsonObject, domain: str) -> None:
    payload = document.copy()
    payload.pop("aggregateHash", None)
    document["aggregateHash"] = domain_separated_sha256(domain, payload)


def _rehash_chunk_source_span(
    artifacts: ParserHashDagArtifactSet,
    chunk: JsonObject,
) -> None:
    ir = artifacts.canonical_ir.document
    parser = _object(ir["parser"])
    normalization = _object(ir["normalizationProfile"])
    envelope: JsonObject = {
        "canonicalization": "rfc8785",
        "configHash": parser["configHash"],
        "domain": "mm-chat.chunk-source-span.v1\n",
        "envelopeKind": "chunk_source_span_hash",
        "hashAlgorithm": "sha256",
        "normalizationProfileHash": normalization["profileHash"],
        "parserBuildHash": parser["parserBuildHash"],
        "payload": {
            "chunkProfileHash": chunk["chunkProfileHash"],
            "fragmentSourceSpanHashes": [
                fragment["fragmentSourceSpanHash"]
                for fragment in _objects(chunk["spanFragments"])
            ],
            "joiners": chunk["joiners"],
        },
        "schemaVersion": "logical-hash-envelope.v1",
    }
    validate_schema_instance(
        load_packaged_schemas(),
        "logical-hash-envelope.v1.schema.json",
        envelope,
    )
    chunk["chunkSourceSpanHash"] = logical_hash_envelope_sha256(envelope)


def _set_path(
    document: JsonObject, path: tuple[str | int, ...], value: JsonValue
) -> None:
    target: JsonValue = document
    for token in path[:-1]:
        target = target[token]  # type: ignore[index]
    target[path[-1]] = value  # type: ignore[index]


def _block_locator(
    artifacts: ParserHashDagArtifactSet,
    block_index: int,
) -> JsonObject:
    block = _objects(artifacts.canonical_ir.document["blocks"])[block_index]
    return _object(block["locatorSet"])


def _source_views(locator: JsonObject) -> list[JsonObject]:
    anchor = _objects(locator["textAnchors"])[0]
    fragment = _objects(anchor["sourceFragments"])[0]
    return _objects(fragment["views"])


def _walk(value: JsonValue) -> Iterator[JsonValue]:
    yield value
    if isinstance(value, dict):
        for item in value.values():
            yield from _walk(item)
    elif isinstance(value, list):
        for item in value:
            yield from _walk(item)


def _object(value: JsonValue) -> JsonObject:
    assert isinstance(value, dict)
    return value


def _objects(value: JsonValue) -> list[JsonObject]:
    assert isinstance(value, list)
    assert all(isinstance(item, dict) for item in value)
    return cast("list[JsonObject]", value)
