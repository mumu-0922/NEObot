"""Structure-preserving projection artifacts for validated Native documents.

This module is intentionally not wired into the production parser gateway yet.
It proves the deterministic Native -> Canonical IR -> Chunk Manifest boundary
needed before a new Search Profile and Index Generation can be staged.
"""

from __future__ import annotations

import hashlib
import uuid
from collections.abc import Sequence
from dataclasses import dataclass
from itertools import pairwise
from typing import Final, NoReturn, cast

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.offline_parser.canonical import (
    JsonObject,
    JsonValue,
    canonical_json_bytes,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeArtifactError,
    NativeDocument,
    NativeFragment,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeSourcePosition,
    NativeTransformKind,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.structure_chunking import (
    ChunkFragmentPlan,
    StructureChunkPlan,
    StructuredTextUnit,
    plan_structure_chunks,
)

NATIVE_STRUCTURE_CONTEXT_INVALID: Final = "NATIVE_STRUCTURE_CONTEXT_INVALID"
NATIVE_STRUCTURE_ARTIFACT_INVALID: Final = "NATIVE_STRUCTURE_ARTIFACT_INVALID"
NATIVE_STRUCTURE_CHUNK_PROFILE_HASH: Final = hashlib.sha256(
    b"mm-chat.native.structure-chunk-profile.v1"
).hexdigest()

_ARTIFACT_NAMESPACE: Final = uuid.UUID("b497f9f9-0e8a-5682-85ab-43c6d631997a")
_MIN_OVERLAP_TOKENS: Final = 60
_AGGREGATE_KINDS: Final = frozenset(
    {NativeNodeKind.LIST_ITEM, NativeNodeKind.TABLE_ROW}
)
_SKIPPED_FRAGMENT_ROLES: Final = frozenset({NativeFragmentRole.EXTERNAL_TARGET})
_BLOCK_TYPES: Final[dict[NativeNodeKind, str]] = {
    NativeNodeKind.HEADING: "heading",
    NativeNodeKind.PARAGRAPH: "paragraph",
    NativeNodeKind.LIST: "list",
    NativeNodeKind.LIST_ITEM: "list_item",
    NativeNodeKind.QUOTE: "quote",
    NativeNodeKind.CODE: "code",
    NativeNodeKind.TABLE: "table",
    NativeNodeKind.TABLE_ROW: "table",
    NativeNodeKind.TABLE_CELL: "table",
    NativeNodeKind.RAW_HTML: "paragraph",
    NativeNodeKind.FOOTNOTE: "footnote",
    NativeNodeKind.ENDNOTE: "footnote",
    NativeNodeKind.HEADER: "header",
    NativeNodeKind.FOOTER: "footer",
    NativeNodeKind.SLIDE: "paragraph",
    NativeNodeKind.SHAPE: "paragraph",
    NativeNodeKind.NOTES: "paragraph",
    NativeNodeKind.SHEET: "paragraph",
    NativeNodeKind.FORMULA: "formula",
}
_LOCATOR_STRUCTURE_KINDS: Final = frozenset(
    {
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
        "footnote",
        "header",
        "footer",
        "slide",
        "sheet",
        "shape",
    }
)


@dataclass(frozen=True, slots=True)
class _SourceSegment:
    start_byte: int
    end_byte: int
    fragment: NativeFragment


@dataclass(frozen=True, slots=True)
class _UnitDraft:
    ordinal: int
    node_ordinal: int
    kind: NativeNodeKind
    text: str
    source_segments: tuple[_SourceSegment, ...]
    heading_level: int | None


@dataclass(frozen=True, slots=True)
class _ProjectedUnit:
    draft: _UnitDraft
    block_id: str
    owner_seed_id: str
    heading_path: tuple[str, ...]
    planner_heading_path: tuple[str, ...]
    parent_block_id: str | None
    text_buffer_start: int
    text_buffer_end: int


def build_native_structure_artifacts(
    context: ProcessingJobContext,
    source: DocumentSource,
    artifact: NativeDocument,
    *,
    parser_model: str,
) -> ParsedDocumentArtifacts:
    """Map one validated Native document into structure-aware parser artifacts."""
    materialization_id = context.materialization_id
    if (
        context.stage != "parse"
        or materialization_id is None
        or not isinstance(parser_model, str)
        or not parser_model.strip()
    ):
        _reject(NATIVE_STRUCTURE_CONTEXT_INVALID)
    try:
        artifact.validate_source_binding(
            source.body,
            expected_format=artifact.source_format,
        )
    except NativeArtifactError as error:
        _reject_from(NATIVE_STRUCTURE_ARTIFACT_INVALID, error)
    if source.source_sha256 != artifact.source_sha256:
        _reject(NATIVE_STRUCTURE_ARTIFACT_INVALID)

    drafts = _extract_units(artifact)
    if not drafts:
        _reject(NATIVE_STRUCTURE_ARTIFACT_INVALID)
    projected, text = _project_units(artifact, drafts)
    planner_units = tuple(
        StructuredTextUnit(
            ordinal=unit.draft.ordinal,
            kind=unit.draft.kind.value,
            text=unit.draft.text,
            heading_path=unit.planner_heading_path,
        )
        for unit in projected
    )
    plan = plan_structure_chunks(planner_units)
    artifact_set_id = uuid.uuid5(
        _ARTIFACT_NAMESPACE,
        ":".join(
            (
                str(materialization_id),
                source.source_sha256,
                artifact.artifact_sha256,
                NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
            )
        ),
    )
    flow_seed_id = _hash_seed(
        "native_structure.flow_seed.v1",
        source.source_sha256,
        artifact.artifact_sha256,
    )
    logical_flow_id = _hash_seed("native_structure.flow.v1", flow_seed_id)
    canonical_ir = _canonical_ir(
        source,
        artifact,
        projected,
        text,
        parser_model=parser_model,
        flow_seed_id=flow_seed_id,
        logical_flow_id=logical_flow_id,
    )
    chunk_manifest = _chunk_manifest(
        artifact,
        projected,
        plan,
        logical_flow_id=logical_flow_id,
        source_sha256=source.source_sha256,
    )
    return ParsedDocumentArtifacts(
        artifact_set_id=artifact_set_id,
        canonical_ir=canonical_ir,
        chunk_manifest=chunk_manifest,
    )


def _extract_units(artifact: NativeDocument) -> tuple[_UnitDraft, ...]:
    children: dict[int, list[NativeNode]] = {}
    for node in artifact.nodes[1:]:
        if node.parent_ordinal is not None:
            children.setdefault(node.parent_ordinal, []).append(node)

    drafts: list[_UnitDraft] = []

    def visit(node: NativeNode) -> None:
        groups: tuple[tuple[NativeFragment, ...], ...] = ()
        separator = ""
        if node.kind is NativeNodeKind.TABLE_ROW:
            groups = _table_row_groups(node, children)
            separator = " | "
        elif node.kind is NativeNodeKind.LIST_ITEM:
            groups = _descendant_text_groups(node, children)
            separator = "\n"
        else:
            direct = _eligible_fragments(node.fragments)
            if direct:
                groups = (direct,)
        if groups:
            text, segments = _unit_text(groups, separator=separator)
            if text.strip():
                drafts.append(
                    _UnitDraft(
                        ordinal=len(drafts),
                        node_ordinal=node.ordinal,
                        kind=node.kind,
                        text=text,
                        source_segments=segments,
                        heading_level=_heading_level(node),
                    )
                )
                return
        for child in children.get(node.ordinal, []):
            visit(child)

    for child in children.get(0, []):
        visit(child)
    return tuple(drafts)


def _table_row_groups(
    row: NativeNode,
    children: dict[int, list[NativeNode]],
) -> tuple[tuple[NativeFragment, ...], ...]:
    groups: list[tuple[NativeFragment, ...]] = []
    for child in children.get(row.ordinal, []):
        if child.kind is NativeNodeKind.TABLE_CELL:
            fragments = tuple(
                fragment
                for group in _descendant_text_groups(child, children)
                for fragment in group
            )
            if fragments:
                groups.append(fragments)
        else:
            groups.extend(_descendant_text_groups(child, children))
    return tuple(groups)


def _descendant_text_groups(
    node: NativeNode,
    children: dict[int, list[NativeNode]],
) -> tuple[tuple[NativeFragment, ...], ...]:
    direct = _eligible_fragments(node.fragments)
    if direct:
        return (direct,)
    return tuple(
        group
        for child in children.get(node.ordinal, [])
        for group in _descendant_text_groups(child, children)
    )


def _eligible_fragments(
    fragments: tuple[NativeFragment, ...],
) -> tuple[NativeFragment, ...]:
    return tuple(
        fragment
        for fragment in fragments
        if fragment.role not in _SKIPPED_FRAGMENT_ROLES and fragment.text
    )


def _unit_text(
    groups: tuple[tuple[NativeFragment, ...], ...],
    *,
    separator: str,
) -> tuple[str, tuple[_SourceSegment, ...]]:
    parts: list[str] = []
    segments: list[_SourceSegment] = []
    offset = 0
    for group_index, group in enumerate(groups):
        if group_index:
            parts.append(separator)
            offset += len(separator.encode("utf-8"))
        for fragment in group:
            encoded = fragment.text.encode("utf-8")
            parts.append(fragment.text)
            segments.append(
                _SourceSegment(
                    start_byte=offset,
                    end_byte=offset + len(encoded),
                    fragment=fragment,
                )
            )
            offset += len(encoded)
    return "".join(parts), tuple(segments)


def _heading_level(node: NativeNode) -> int | None:
    if node.kind is not NativeNodeKind.HEADING:
        return None
    for attribute in node.attributes:
        if attribute.name == "level" and type(attribute.value) is int:
            return max(1, min(9, attribute.value))
    return 1


def _project_units(
    artifact: NativeDocument,
    drafts: tuple[_UnitDraft, ...],
) -> tuple[tuple[_ProjectedUnit, ...], str]:
    heading_stack: list[tuple[int, str]] = []
    projected: list[_ProjectedUnit] = []
    text_parts: list[str] = []
    text_offset = 0
    for draft in drafts:
        if projected:
            text_parts.append("\n")
            text_offset += 1
        owner_seed_id = _hash_seed(
            "native_structure.owner.v1",
            artifact.artifact_sha256,
            draft.node_ordinal,
            draft.kind.value,
        )
        block_id = _hash_seed("native_structure.block.v1", owner_seed_id)
        if draft.heading_level is not None:
            while heading_stack and heading_stack[-1][0] >= draft.heading_level:
                heading_stack.pop()
            heading_path = tuple(item[1] for item in heading_stack)
            parent_block_id = heading_path[-1] if heading_path else None
            planner_heading_path = (*heading_path, block_id)
            heading_stack.append((draft.heading_level, block_id))
        else:
            heading_path = tuple(item[1] for item in heading_stack)
            parent_block_id = heading_path[-1] if heading_path else None
            planner_heading_path = heading_path
        text_bytes = draft.text.encode("utf-8")
        projected.append(
            _ProjectedUnit(
                draft=draft,
                block_id=block_id,
                owner_seed_id=owner_seed_id,
                heading_path=heading_path,
                planner_heading_path=planner_heading_path,
                parent_block_id=parent_block_id,
                text_buffer_start=text_offset,
                text_buffer_end=text_offset + len(text_bytes),
            )
        )
        text_parts.append(draft.text)
        text_offset += len(text_bytes)
    return tuple(projected), "".join(text_parts)


def _canonical_ir(
    source: DocumentSource,
    artifact: NativeDocument,
    units: tuple[_ProjectedUnit, ...],
    text: str,
    *,
    parser_model: str,
    flow_seed_id: str,
    logical_flow_id: str,
) -> JsonObject:
    blocks: list[JsonValue] = []
    provenance: list[JsonValue] = []
    source_unit_ids = _source_unit_ids(artifact)
    for unit in units:
        source_span_hash = _block_source_span_hash(unit, source_unit_ids)
        provenance_id = _hash_seed(
            "native_structure.provenance_id.v1",
            unit.owner_seed_id,
            source_span_hash,
            artifact.artifact_sha256,
        )
        provenance_record = _provenance(
            provenance_id=provenance_id,
            provenance_ordinal=len(provenance),
            owner_seed_id=unit.owner_seed_id,
            source_unit_id=_primary_source_unit_id(unit, source_unit_ids),
            payload_ref=artifact.artifact_sha256,
        )
        provenance.append(cast("JsonValue", provenance_record))
        block_bytes = unit.draft.text.encode("utf-8")
        block: JsonObject = {
            "blockType": _block_type(unit.draft.kind),
            "confidence": 10000,
            "contentHash": hashlib.sha256(block_bytes).hexdigest(),
            "flags": {
                "derived": unit.draft.kind in _AGGREGATE_KINDS,
                "nonIndexable": False,
            },
            "flowSeedId": flow_seed_id,
            "headingPath": list(unit.heading_path),
            "locatorSet": _locator_set(
                artifact,
                unit,
                start_byte=0,
                end_byte=len(block_bytes),
                source_unit_ids=source_unit_ids,
            ),
            "logicalBlockId": unit.block_id,
            "ordinal": unit.draft.ordinal,
            "parentBlockId": unit.parent_block_id,
            "provenanceRefs": [provenance_id],
            "readingFlowOrdinal": 0,
            "sourceSpanHash": {
                "kind": "text",
                "textSourceSpanHash": source_span_hash,
            },
            "structureRef": {
                "ownerSeedId": unit.owner_seed_id,
                "structureKind": _locator_structure_kind(unit.draft.kind),
                "structureOrdinal": unit.draft.ordinal,
            },
            "textRange": {
                "endByte": unit.text_buffer_end,
                "startByte": unit.text_buffer_start,
            },
        }
        blocks.append(cast("JsonValue", block))

    normalization_value: JsonObject = {
        "nativeArtifactSha256": artifact.artifact_sha256,
        "profile": "mm-chat.native.structure-projection.v1",
        "sourceSha256": source.source_sha256,
    }
    normalization_bytes = canonical_json_bytes(normalization_value)
    text_bytes = text.encode("utf-8")
    return {
        "assets": [],
        "blocks": blocks,
        "formulas": [],
        "normalizationMapRef": {
            "bytes": len(normalization_bytes),
            "schemaVersion": "normalization-map.v1",
            "sha256": hashlib.sha256(normalization_bytes).hexdigest(),
        },
        "normalizationProfile": {
            "profileHash": _hash_seed("native_structure.normalization_profile.v1"),
            "schemaVersion": "normalization-profile.v1",
        },
        "pages": [],
        "parser": {
            "configHash": _hash_seed(
                "native_structure.parser_config.v1",
                artifact.schema_version,
            ),
            "parserBuildHash": _hash_seed(
                "native_structure.parser_build.v1",
                parser_model,
            ),
            "profileHash": _hash_seed("native_structure.parser_profile.v1"),
            "schemaVersion": "parser-profile.v1",
        },
        "provenance": provenance,
        "readingFlows": [
            {
                "flowOrdinal": 0,
                "flowSeedId": flow_seed_id,
                "logicalFlowId": logical_flow_id,
                "orderedLogicalBlockIds": [unit.block_id for unit in units],
            }
        ],
        "schemaVersion": "canonical-ir.v2",
        "source": {
            "bytes": len(source.body),
            "format": artifact.source_format.value,
            "sha256": source.source_sha256,
        },
        "tables": [],
        "textBuffer": {
            "bytes": len(text_bytes),
            "encoding": "utf-8",
            "sha256": hashlib.sha256(text_bytes).hexdigest(),
            "text": text,
        },
    }


def _provenance(
    *,
    provenance_id: str,
    provenance_ordinal: int,
    owner_seed_id: str,
    source_unit_id: str,
    payload_ref: str,
) -> JsonObject:
    base: JsonObject = {
        "derivationProfileHash": _hash_seed("native_structure.derivation_profile.v1"),
        "payloadRef": payload_ref,
        "provenanceId": provenance_id,
        "provenanceKind": "source_structure",
        "provenanceOrdinal": provenance_ordinal,
        "sourceUnitRef": source_unit_id,
        "targetKind": "block",
        "targetKindRank": 0,
        "targetOwnerSeedId": owner_seed_id,
    }
    return {**base, "provenanceHash": _hash_json(base)}


def _chunk_manifest(
    artifact: NativeDocument,
    units: tuple[_ProjectedUnit, ...],
    plan: StructureChunkPlan,
    *,
    logical_flow_id: str,
    source_sha256: str,
) -> JsonObject:
    source_unit_ids = _source_unit_ids(artifact)
    parents: list[JsonObject] = []
    parent_ids: dict[int, str] = {}
    parent_seed_ids: dict[int, str] = {}
    for parent in plan.parents:
        fragments = _chunk_fragments(
            artifact,
            units,
            parent.fragments,
            source_unit_ids=source_unit_ids,
        )
        joiners = _chunk_joiners(parent.fragments)
        content = _chunk_content(units, parent.fragments, joiners)
        parent_seed_id = _hash_seed(
            "native_structure.parent_seed.v1",
            source_sha256,
            parent.ordinal,
            _hash_json(cast("JsonValue", fragments)),
        )
        logical_id = _hash_seed(
            "native_structure.parent_chunk.v1",
            parent_seed_id,
            NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
        )
        parent_ids[parent.ordinal] = logical_id
        parent_seed_ids[parent.ordinal] = parent_seed_id
        parents.append(
            {
                "chunkKind": "parent",
                "chunkProfileHash": NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
                "chunkSourceSpanHash": _chunk_source_span_hash(fragments, joiners),
                "contentBytes": len(content.encode("utf-8")),
                "contentHash": hashlib.sha256(content.encode("utf-8")).hexdigest(),
                "joiners": cast("list[JsonValue]", joiners),
                "logicalChunkId": logical_id,
                "logicalFlowId": logical_flow_id,
                "parentChunkSeedId": parent_seed_id,
                "parentOrdinal": parent.ordinal,
                "sectionOwnerSeedId": units[
                    parent.fragments[0].unit_ordinal
                ].owner_seed_id,
                "spanFragments": cast("list[JsonValue]", fragments),
                "tokenCount": parent.token_count,
            }
        )

    children: list[JsonObject] = []
    for child in plan.children:
        fragments = _chunk_fragments(
            artifact,
            units,
            child.fragments,
            source_unit_ids=source_unit_ids,
            child_ordinal=child.ordinal,
            overlap_token_count=child.overlap_before_tokens,
        )
        joiners = _chunk_joiners(child.fragments)
        content = _chunk_content(units, child.fragments, joiners)
        parent_id = parent_ids[child.parent_ordinal]
        child_id = _hash_seed(
            "native_structure.child_chunk.v1",
            parent_id,
            child.ordinal,
            _hash_json(cast("JsonValue", fragments)),
            NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
        )
        children.append(
            {
                "childOrdinal": child.ordinal,
                "chunkKind": "child",
                "chunkProfileHash": NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
                "chunkSourceSpanHash": _chunk_source_span_hash(fragments, joiners),
                "contentBytes": len(content.encode("utf-8")),
                "contentHash": hashlib.sha256(content.encode("utf-8")).hexdigest(),
                "joiners": cast("list[JsonValue]", joiners),
                "logicalChunkId": child_id,
                "logicalFlowId": logical_flow_id,
                "logicalParentChunkId": parent_id,
                "parentChunkSeedId": parent_seed_ids[child.parent_ordinal],
                "spanFragments": cast("list[JsonValue]", fragments),
                "tokenCount": child.token_count,
            }
        )

    all_chunks = [*parents, *children]
    aggregate_spans: list[JsonValue] = []
    aggregate_joiners: list[JsonValue] = []
    for chunk in all_chunks:
        aggregate_spans.extend(cast("list[JsonValue]", chunk["spanFragments"]))
        aggregate_joiners.extend(cast("list[JsonValue]", chunk["joiners"]))
    return {
        "childAggregateHash": _hash_json(cast("JsonValue", children)),
        "childCount": len(children),
        "children": cast("list[JsonValue]", children),
        "chunkProfileHash": NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
        "joinerAggregateHash": _hash_json(cast("JsonValue", aggregate_joiners)),
        "joinerCount": len(aggregate_joiners),
        "parentAggregateHash": _hash_json(cast("JsonValue", parents)),
        "parentCount": len(parents),
        "parents": cast("list[JsonValue]", parents),
        "schemaVersion": "chunk-manifest.v2",
        "sourceSha256": source_sha256,
        "spanAggregateHash": _hash_json(cast("JsonValue", aggregate_spans)),
        "spanCount": len(aggregate_spans),
    }


def _chunk_fragments(
    artifact: NativeDocument,
    units: tuple[_ProjectedUnit, ...],
    plans: tuple[ChunkFragmentPlan, ...],
    *,
    source_unit_ids: tuple[str, ...],
    child_ordinal: int | None = None,
    overlap_token_count: int = 0,
) -> list[JsonObject]:
    fragments: list[JsonObject] = []
    overlap_group_id = _hash_seed(
        "native_structure.overlap_group.v1",
        artifact.artifact_sha256,
        child_ordinal,
        overlap_token_count,
    )
    for plan in plans:
        unit = units[plan.unit_ordinal]
        locator = _locator_set(
            artifact,
            unit,
            start_byte=plan.start_byte,
            end_byte=plan.end_byte,
            source_unit_ids=source_unit_ids,
        )
        fragment_hash = _hash_seed(
            "native_structure.chunk_fragment.v1",
            unit.block_id,
            plan.start_byte,
            plan.end_byte,
            cast("str", locator["aggregateHash"]),
        )
        fragment: JsonObject = {
            "blockEndByte": plan.end_byte,
            "blockLogicalId": unit.block_id,
            "blockStartByte": plan.start_byte,
            "clippedLocatorSet": locator,
            "fragmentKind": "window_overlap" if plan.overlap else "primary",
            "fragmentSourceSpanHash": fragment_hash,
        }
        if plan.overlap:
            if (
                child_ordinal is None
                or child_ordinal <= 0
                or overlap_token_count < _MIN_OVERLAP_TOKENS
            ):
                _reject(NATIVE_STRUCTURE_ARTIFACT_INVALID)
            fragment.update(
                {
                    "overlapGroupId": overlap_group_id,
                    "overlapTokenCount": overlap_token_count,
                    "previousChildOrdinal": child_ordinal - 1,
                }
            )
        fragments.append(fragment)
    return fragments


def _chunk_joiners(
    fragments: tuple[ChunkFragmentPlan, ...],
) -> list[JsonObject]:
    result: list[JsonObject] = []
    for previous, current in pairwise(fragments):
        adjacent = (
            previous.unit_ordinal == current.unit_ordinal
            and previous.end_byte == current.start_byte
        )
        result.append(
            {
                "kind": "adjacent" if adjacent else "block_separator",
                "utf8Bytes": "" if adjacent else "\n\n",
            }
        )
    return result


def _chunk_content(
    units: tuple[_ProjectedUnit, ...],
    fragments: tuple[ChunkFragmentPlan, ...],
    joiners: Sequence[JsonObject],
) -> str:
    parts: list[str] = []
    for index, fragment in enumerate(fragments):
        if index:
            parts.append(cast("str", joiners[index - 1]["utf8Bytes"]))
        encoded = units[fragment.unit_ordinal].draft.text.encode("utf-8")
        try:
            parts.append(
                encoded[fragment.start_byte : fragment.end_byte].decode(
                    "utf-8", errors="strict"
                )
            )
        except UnicodeDecodeError as error:
            _reject_from(NATIVE_STRUCTURE_ARTIFACT_INVALID, error)
    return "".join(parts)


def _chunk_source_span_hash(
    fragments: Sequence[JsonObject],
    joiners: Sequence[JsonObject],
) -> str:
    return _hash_seed(
        "native_structure.chunk_span.v1",
        NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
        _hash_json(cast("JsonValue", list(fragments))),
        _hash_json(cast("JsonValue", list(joiners))),
    )


def _locator_set(
    artifact: NativeDocument,
    unit: _ProjectedUnit,
    *,
    start_byte: int,
    end_byte: int,
    source_unit_ids: tuple[str, ...],
) -> JsonObject:
    unit_bytes = unit.draft.text.encode("utf-8")
    if not 0 <= start_byte < end_byte <= len(unit_bytes):
        _reject(NATIVE_STRUCTURE_ARTIFACT_INVALID)
    anchors: list[JsonValue] = []
    for segment in unit.draft.source_segments:
        overlap_start = max(start_byte, segment.start_byte)
        overlap_end = min(end_byte, segment.end_byte)
        if overlap_start >= overlap_end:
            continue
        view = _source_position_view(
            artifact,
            segment,
            overlap_start=overlap_start,
            overlap_end=overlap_end,
            source_unit_ids=source_unit_ids,
        )
        anchors.append(
            {
                "anchorOrdinal": len(anchors),
                "canonicalEndByte": unit.text_buffer_start + overlap_end,
                "canonicalStartByte": unit.text_buffer_start + overlap_start,
                "sourceFragments": [
                    {
                        "fragmentOrdinal": 0,
                        "views": [
                            view,
                            {
                                "kind": "derived_structure",
                                "opaqueStructureId": unit.owner_seed_id,
                                "structureKind": _locator_structure_kind(
                                    unit.draft.kind
                                ),
                            },
                        ],
                    }
                ],
            }
        )
    if not anchors:
        segment = _nearest_segment(unit.draft.source_segments, start_byte)
        anchors.append(
            {
                "anchorOrdinal": 0,
                "canonicalEndByte": unit.text_buffer_start + end_byte,
                "canonicalStartByte": unit.text_buffer_start + start_byte,
                "sourceFragments": [
                    {
                        "fragmentOrdinal": 0,
                        "views": [
                            _coarse_source_position_view(
                                segment.fragment.source_position,
                                source_unit_ids,
                            ),
                            {
                                "kind": "derived_structure",
                                "opaqueStructureId": unit.owner_seed_id,
                                "structureKind": _locator_structure_kind(
                                    unit.draft.kind
                                ),
                            },
                        ],
                    }
                ],
            }
        )
    base: JsonObject = {
        "structuralAnchors": [],
        "textAnchors": anchors,
        "version": 2,
    }
    return {**base, "aggregateHash": _hash_json(base)}


def _source_position_view(
    artifact: NativeDocument,
    segment: _SourceSegment,
    *,
    overlap_start: int,
    overlap_end: int,
    source_unit_ids: tuple[str, ...],
) -> JsonObject:
    position = segment.fragment.source_position
    if segment.fragment.transform is not NativeTransformKind.IDENTITY:
        return _coarse_source_position_view(position, source_unit_ids)
    fragment_bytes = segment.fragment.text.encode("utf-8")
    relative_start = overlap_start - segment.start_byte
    relative_end = overlap_end - segment.start_byte
    try:
        prefix = fragment_bytes[:relative_start].decode("utf-8", errors="strict")
        selected = fragment_bytes[relative_start:relative_end].decode(
            "utf-8", errors="strict"
        )
    except UnicodeDecodeError:
        return _coarse_source_position_view(position, source_unit_ids)
    source_unit = artifact.source_units[position.source_unit_ordinal]
    codec = source_unit.encoding
    if codec is None:
        return _coarse_source_position_view(position, source_unit_ids)
    codec = "utf-8" if codec == "utf-8-bom" else codec
    try:
        raw_start = position.raw_byte_start + len(prefix.encode(codec))
        raw_end = raw_start + len(selected.encode(codec))
    except (LookupError, UnicodeEncodeError):
        return _coarse_source_position_view(position, source_unit_ids)
    if raw_end > position.raw_byte_end:
        return _coarse_source_position_view(position, source_unit_ids)
    start_line, start_column = _advance_position(
        position.start_line,
        position.start_column,
        prefix,
    )
    end_line, end_column = _advance_position(start_line, start_column, selected)
    return {
        "decodedScalarEnd": position.decoded_scalar_start + len(prefix) + len(selected),
        "decodedScalarStart": position.decoded_scalar_start + len(prefix),
        "endColumn": end_column,
        "endLine": end_line,
        "kind": "source_text_position",
        "opaqueSourceUnitId": source_unit_ids[position.source_unit_ordinal],
        "rawByteEnd": raw_end,
        "rawByteStart": raw_start,
        "startColumn": start_column,
        "startLine": start_line,
    }


def _coarse_source_position_view(
    position: NativeSourcePosition,
    source_unit_ids: tuple[str, ...],
) -> JsonObject:
    return {
        "decodedScalarEnd": position.decoded_scalar_end,
        "decodedScalarStart": position.decoded_scalar_start,
        "endColumn": position.end_column,
        "endLine": position.end_line,
        "kind": "source_text_position",
        "opaqueSourceUnitId": source_unit_ids[position.source_unit_ordinal],
        "rawByteEnd": position.raw_byte_end,
        "rawByteStart": position.raw_byte_start,
        "startColumn": position.start_column,
        "startLine": position.start_line,
    }


def _advance_position(line: int, column: int, text: str) -> tuple[int, int]:
    index = 0
    while index < len(text):
        character = text[index]
        if character == "\r":
            line += 1
            column = 0
            index += 1
            if index < len(text) and text[index] == "\n":
                index += 1
            continue
        if character == "\n":
            line += 1
            column = 0
        else:
            column += 1
        index += 1
    return line, column


def _nearest_segment(
    segments: tuple[_SourceSegment, ...], offset: int
) -> _SourceSegment:
    if not segments:
        _reject(NATIVE_STRUCTURE_ARTIFACT_INVALID)
    return min(
        segments,
        key=lambda item: min(
            abs(offset - item.start_byte),
            abs(offset - item.end_byte),
        ),
    )


def _source_unit_ids(artifact: NativeDocument) -> tuple[str, ...]:
    return tuple(
        _hash_seed(
            "native_structure.source_unit.v1",
            artifact.artifact_sha256,
            unit.ordinal,
            unit.kind.value,
            unit.canonical_uri or "",
            unit.source_sha256,
        )
        for unit in artifact.source_units
    )


def _primary_source_unit_id(
    unit: _ProjectedUnit, source_unit_ids: tuple[str, ...]
) -> str:
    position = unit.draft.source_segments[0].fragment.source_position
    return source_unit_ids[position.source_unit_ordinal]


def _block_source_span_hash(
    unit: _ProjectedUnit, source_unit_ids: tuple[str, ...]
) -> str:
    values = [
        _hash_seed(
            "native_structure.source_segment.v1",
            source_unit_ids[segment.fragment.source_position.source_unit_ordinal],
            segment.fragment.source_position.raw_byte_start,
            segment.fragment.source_position.raw_byte_end,
            segment.start_byte,
            segment.end_byte,
            segment.fragment.transform.value,
        )
        for segment in unit.draft.source_segments
    ]
    return _hash_seed("native_structure.block_span.v1", *values)


def _block_type(kind: NativeNodeKind) -> str:
    try:
        return _BLOCK_TYPES[kind]
    except KeyError as error:
        _reject_from(NATIVE_STRUCTURE_ARTIFACT_INVALID, error)


def _locator_structure_kind(kind: NativeNodeKind) -> str:
    value = kind.value
    if value in _LOCATOR_STRUCTURE_KINDS:
        return value
    return _block_type(kind)


def _hash_seed(domain: str, *parts: object) -> str:
    digest = hashlib.sha256(domain.encode("utf-8"))
    for part in parts:
        digest.update(b"\x00")
        digest.update(str(part).encode("utf-8"))
    return digest.hexdigest()


def _hash_json(value: JsonValue) -> str:
    return hashlib.sha256(canonical_json_bytes(value)).hexdigest()


def _reject(code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(code))


def _reject_from(code: str, cause: Exception) -> NoReturn:
    try:
        _reject(code)
    except PermanentJobError as error:
        raise error from cause
