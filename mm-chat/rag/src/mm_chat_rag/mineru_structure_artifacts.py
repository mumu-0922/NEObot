"""Structure-aware artifacts from admitted MinerU page elements.

The mapper is offline and has no production gateway caller.  It consumes the
hash-bound archive mapping input, never calls MinerU, Jina, Postgres, or an
Index Generation transition.
"""

from __future__ import annotations

import hashlib
import uuid
from dataclasses import dataclass
from itertools import pairwise
from typing import Final, NoReturn, cast

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import ParsedDocumentArtifacts
from mm_chat_rag.mineru_gateway import MinerULocalBatchCanonicalMappingInput
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.offline_parser.canonical import (
    JsonObject,
    JsonValue,
    canonical_json_bytes,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.structure_chunking import (
    ChunkFragmentPlan,
    StructureChunkPlan,
    StructuredTextUnit,
    plan_structure_chunks,
)

MINERU_STRUCTURE_CONTEXT_INVALID: Final = "MINERU_STRUCTURE_CONTEXT_INVALID"
MINERU_STRUCTURE_ARTIFACT_INVALID: Final = "MINERU_STRUCTURE_ARTIFACT_INVALID"
MINERU_STRUCTURE_CHUNK_PROFILE_HASH: Final = hashlib.sha256(
    b"mm-chat.mineru.structure-chunk-profile.v1"
).hexdigest()

_ARTIFACT_NAMESPACE: Final = uuid.UUID("55c9a965-b2c6-583b-9463-075384a39d7b")
_BBOX_COORDINATES: Final = 4
_MAX_HEADING_LEVEL: Final = 9
_MIN_OVERLAP_TOKENS: Final = 60
_KIND_MAP: Final = {
    "heading": "heading",
    "title": "heading",
    "text": "paragraph",
    "paragraph": "paragraph",
    "list": "list",
    "list_item": "list_item",
    "quote": "quote",
    "code": "code",
    "table": "table",
    "formula": "formula",
    "equation": "formula",
    "footnote": "footnote",
    "header": "header",
    "footer": "footer",
}


@dataclass(frozen=True, slots=True)
class _Unit:
    ordinal: int
    kind: str
    text: str
    page_index: int
    bbox: tuple[int, int, int, int]
    heading_level: int | None
    owner_id: str
    block_id: str
    heading_path: tuple[str, ...]
    planner_heading_path: tuple[str, ...]
    parent_block_id: str | None
    text_start: int
    text_end: int


@dataclass(frozen=True, slots=True)
class _Draft:
    kind: str
    text: str
    page_index: int
    bbox: tuple[int, int, int, int]
    heading_level: int | None


def build_mineru_structure_artifacts(
    context: ProcessingJobContext,
    mapping_input: MinerULocalBatchCanonicalMappingInput,
) -> ParsedDocumentArtifacts:
    """Build deterministic structure-aware artifacts from admitted MinerU JSON."""
    if context.stage != "parse" or context.materialization_id is None:
        _reject(MINERU_STRUCTURE_CONTEXT_INVALID)
    if not isinstance(mapping_input, MinerULocalBatchCanonicalMappingInput):
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    units, text, pages = _units(mapping_input)
    plan = plan_structure_chunks(
        tuple(
            StructuredTextUnit(
                ordinal=unit.ordinal,
                kind=unit.kind,
                text=unit.text,
                heading_path=unit.planner_heading_path,
            )
            for unit in units
        )
    )
    materialization_id = context.materialization_id
    artifact_set_id = uuid.uuid5(
        _ARTIFACT_NAMESPACE,
        ":".join(
            (
                str(materialization_id),
                mapping_input.source_sha256,
                mapping_input.archive_sha256,
                MINERU_STRUCTURE_CHUNK_PROFILE_HASH,
            )
        ),
    )
    flow_seed = _hash("mineru_structure.flow_seed.v1", mapping_input.archive_sha256)
    flow_id = _hash("mineru_structure.flow.v1", flow_seed)
    return ParsedDocumentArtifacts(
        artifact_set_id=artifact_set_id,
        canonical_ir=_canonical_ir(
            mapping_input,
            units,
            text,
            pages,
            flow_seed=flow_seed,
            flow_id=flow_id,
        ),
        chunk_manifest=_chunk_manifest(
            mapping_input,
            units,
            plan,
            flow_id=flow_id,
        ),
    )


def _units(
    mapping: MinerULocalBatchCanonicalMappingInput,
) -> tuple[tuple[_Unit, ...], str, tuple[tuple[int, int, int], ...]]:
    drafts, pages = _drafts(mapping)
    heading_stack: list[tuple[int, str]] = []
    units: list[_Unit] = []
    text_parts: list[str] = []
    offset = 0
    for ordinal, draft in enumerate(drafts):
        if units:
            text_parts.append("\n")
            offset += 1
        owner = _hash(
            "mineru_structure.owner.v1",
            mapping.archive_sha256,
            draft.page_index,
            ordinal,
            draft.kind,
            draft.bbox,
        )
        block_id = _hash("mineru_structure.block.v1", owner)
        heading_path, planner_path = _heading_paths(
            heading_stack,
            draft.heading_level,
            block_id,
        )
        encoded = draft.text.encode()
        units.append(
            _Unit(
                ordinal=ordinal,
                kind=draft.kind,
                text=draft.text,
                page_index=draft.page_index,
                bbox=draft.bbox,
                heading_level=draft.heading_level,
                owner_id=owner,
                block_id=block_id,
                heading_path=heading_path,
                planner_heading_path=planner_path,
                parent_block_id=heading_path[-1] if heading_path else None,
                text_start=offset,
                text_end=offset + len(encoded),
            )
        )
        text_parts.append(draft.text)
        offset += len(encoded)
    return tuple(units), "".join(text_parts), pages


def _drafts(
    mapping: MinerULocalBatchCanonicalMappingInput,
) -> tuple[tuple[_Draft, ...], tuple[tuple[int, int, int], ...]]:
    raw_pages = mapping.decoded.middle_json.get("pages")
    if not isinstance(raw_pages, list) or not raw_pages:
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    drafts: list[_Draft] = []
    pages: list[tuple[int, int, int]] = []
    previous_page = -1
    for raw_page in raw_pages:
        page = _object(raw_page)
        page_index = _integer_alias(page, "pageIndex", "page_idx")
        if page_index != previous_page + 1:
            _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
        previous_page = page_index
        width = _positive_alias(page, "widthMilliPoint", "width")
        height = _positive_alias(page, "heightMilliPoint", "height")
        pages.append((page_index, width, height))
        elements = page.get("elements")
        if not isinstance(elements, list):
            _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
        for raw_element in elements:
            element = _object(raw_element)
            raw_kind = element.get("kind", element.get("type"))
            if not isinstance(raw_kind, str):
                _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
            block_kind = _KIND_MAP.get(raw_kind.casefold())
            text = _element_text(element, block_kind)
            if block_kind is None:
                if text is not None:
                    _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
                continue
            if text is None or not text.strip() or "\x00" in text:
                _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
            bbox = _bbox(element, width=width, height=height)
            heading_level = _heading_level(element) if block_kind == "heading" else None
            drafts.append(
                _Draft(
                    kind=block_kind,
                    text=text,
                    page_index=page_index,
                    bbox=bbox,
                    heading_level=heading_level,
                )
            )
    if not drafts:
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return tuple(drafts), tuple(pages)


def _heading_paths(
    stack: list[tuple[int, str]],
    level: int | None,
    block_id: str,
) -> tuple[tuple[str, ...], tuple[str, ...]]:
    if level is None:
        path = tuple(item[1] for item in stack)
        return path, path
    while stack and stack[-1][0] >= level:
        stack.pop()
    path = tuple(item[1] for item in stack)
    planner_path = (*path, block_id)
    stack.append((level, block_id))
    return path, planner_path


def _element_text(element: JsonObject, block_kind: str | None) -> str | None:
    value = element.get("text")
    if value is None:
        value = element.get("sourceText")
    if isinstance(value, str):
        return value
    if block_kind != "table":
        return None
    rows = element.get("rows")
    if not isinstance(rows, list) or not rows:
        return None
    rendered: list[str] = []
    for raw_row in rows:
        row = _object(raw_row)
        cells = row.get("cells")
        if not isinstance(cells, list) or not cells:
            _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
        values: list[str] = []
        for raw_cell in cells:
            cell = _object(raw_cell)
            cell_text = cell.get("text")
            if not isinstance(cell_text, str) or not cell_text.strip():
                _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
            values.append(cell_text)
        rendered.append(" | ".join(values))
    return "\n".join(rendered)


def _canonical_ir(
    mapping: MinerULocalBatchCanonicalMappingInput,
    units: tuple[_Unit, ...],
    text: str,
    pages: tuple[tuple[int, int, int], ...],
    *,
    flow_seed: str,
    flow_id: str,
) -> JsonObject:
    source_unit_id = _hash(
        "mineru_structure.source_unit.v1",
        mapping.archive_sha256,
        _role_hash(mapping, "middle_json"),
    )
    blocks: list[JsonValue] = []
    provenance: list[JsonValue] = []
    for unit in units:
        span_hash = _hash(
            "mineru_structure.block_span.v1",
            source_unit_id,
            unit.page_index,
            unit.bbox,
            unit.text_start,
            unit.text_end,
        )
        provenance_id = _hash(
            "mineru_structure.provenance_id.v1", unit.owner_id, span_hash
        )
        provenance_base: JsonObject = {
            "derivationProfileHash": _hash("mineru_structure.derivation.v1"),
            "payloadRef": _role_hash(mapping, "middle_json"),
            "provenanceId": provenance_id,
            "provenanceKind": "synthetic_mineru_derivation",
            "provenanceOrdinal": len(provenance),
            "sourceUnitRef": source_unit_id,
            "targetKind": "block",
            "targetKindRank": 0,
            "targetOwnerSeedId": unit.owner_id,
        }
        provenance.append(
            {**provenance_base, "provenanceHash": _hash_json(provenance_base)}
        )
        blocks.append(
            {
                "blockType": unit.kind,
                "confidence": 10000,
                "contentHash": hashlib.sha256(unit.text.encode()).hexdigest(),
                "flags": {"derived": True, "nonIndexable": False},
                "flowSeedId": flow_seed,
                "headingPath": list(unit.heading_path),
                "locatorSet": _locator(unit),
                "logicalBlockId": unit.block_id,
                "ordinal": unit.ordinal,
                "parentBlockId": unit.parent_block_id,
                "provenanceRefs": [provenance_id],
                "readingFlowOrdinal": 0,
                "sourceSpanHash": {"kind": "text", "textSourceSpanHash": span_hash},
                "structureRef": {
                    "ownerSeedId": unit.owner_id,
                    "structureKind": unit.kind,
                    "structureOrdinal": unit.ordinal,
                },
                "textRange": {"endByte": unit.text_end, "startByte": unit.text_start},
            }
        )
    text_bytes = text.encode()
    normalization = canonical_json_bytes(
        {
            "archiveSha256": mapping.archive_sha256,
            "profile": "mm-chat.mineru.structure-projection.v1",
            "sourceSha256": mapping.source_sha256,
        }
    )
    return {
        "assets": [],
        "blocks": blocks,
        "formulas": [],
        "normalizationMapRef": {
            "bytes": len(normalization),
            "schemaVersion": "normalization-map.v1",
            "sha256": hashlib.sha256(normalization).hexdigest(),
        },
        "normalizationProfile": {
            "profileHash": _hash("mineru_structure.normalization_profile.v1"),
            "schemaVersion": "normalization-profile.v1",
        },
        "pages": [
            _page(mapping.archive_sha256, index, width, height)
            for index, width, height in pages
        ],
        "parser": {
            "configHash": _hash(
                "mineru_structure.config.v1", _role_hash(mapping, "model_json")
            ),
            "parserBuildHash": _hash("mineru_structure.build.v1", "pipeline"),
            "profileHash": _hash("mineru_structure.parser_profile.v1"),
            "schemaVersion": "parser-profile.v1",
        },
        "provenance": provenance,
        "readingFlows": [
            {
                "flowOrdinal": 0,
                "flowSeedId": flow_seed,
                "logicalFlowId": flow_id,
                "orderedLogicalBlockIds": [unit.block_id for unit in units],
            }
        ],
        "schemaVersion": "canonical-ir.v2",
        "source": {
            "bytes": mapping.source_byte_count,
            "format": "pdf",
            "sha256": mapping.source_sha256,
        },
        "tables": [],
        "textBuffer": {
            "bytes": len(text_bytes),
            "encoding": "utf-8",
            "sha256": hashlib.sha256(text_bytes).hexdigest(),
            "text": text,
        },
    }


def _page(archive_sha256: str, index: int, width: int, height: int) -> JsonObject:
    owner = _hash("mineru_structure.page.v1", archive_sha256, index)
    base: JsonObject = {
        "structuralAnchors": [
            {
                "anchorOrdinal": 0,
                "nodeKind": "page",
                "ownerSeedId": owner,
                "sourceFragments": [
                    {
                        "fragmentOrdinal": 0,
                        "views": [
                            {
                                "bboxMilliPoint": [0, 0, width, height],
                                "kind": "page_region",
                                "pageIndex": index,
                            }
                        ],
                    }
                ],
                "structureOrdinal": index,
            }
        ],
        "textAnchors": [],
        "version": 2,
    }
    return {
        "heightMilliPoint": height,
        "locatorSet": {**base, "aggregateHash": _hash_json(base)},
        "ownerSeedId": owner,
        "pageIndex": index,
        "rotationDegrees": 0,
        "widthMilliPoint": width,
    }


def _locator(unit: _Unit, start: int = 0, end: int | None = None) -> JsonObject:
    end = len(unit.text.encode()) if end is None else end
    base: JsonObject = {
        "structuralAnchors": [],
        "textAnchors": [
            {
                "anchorOrdinal": 0,
                "canonicalEndByte": unit.text_start + end,
                "canonicalStartByte": unit.text_start + start,
                "sourceFragments": [
                    {
                        "fragmentOrdinal": 0,
                        "views": [
                            {
                                "bboxMilliPoint": list(unit.bbox),
                                "kind": "page_region",
                                "pageIndex": unit.page_index,
                            },
                            {
                                "kind": "derived_structure",
                                "opaqueStructureId": unit.owner_id,
                                "structureKind": unit.kind,
                            },
                        ],
                    }
                ],
            }
        ],
        "version": 2,
    }
    return {**base, "aggregateHash": _hash_json(base)}


def _chunk_manifest(
    mapping: MinerULocalBatchCanonicalMappingInput,
    units: tuple[_Unit, ...],
    plan: StructureChunkPlan,
    *,
    flow_id: str,
) -> JsonObject:
    parents: list[JsonObject] = []
    parent_ids: dict[int, str] = {}
    parent_seeds: dict[int, str] = {}
    for parent in plan.parents:
        fragments, joiners, content = _chunk_parts(units, parent.fragments)
        seed = _hash(
            "mineru_structure.parent_seed.v1", mapping.source_sha256, parent.ordinal
        )
        logical_id = _hash(
            "mineru_structure.parent.v1", seed, _hash_json(cast("JsonValue", fragments))
        )
        parent_ids[parent.ordinal] = logical_id
        parent_seeds[parent.ordinal] = seed
        parents.append(
            {
                "chunkKind": "parent",
                "chunkProfileHash": MINERU_STRUCTURE_CHUNK_PROFILE_HASH,
                "chunkSourceSpanHash": _hash(
                    "mineru_structure.chunk_span.v1",
                    _hash_json(cast("JsonValue", fragments)),
                    _hash_json(cast("JsonValue", joiners)),
                ),
                "contentBytes": len(content.encode()),
                "contentHash": hashlib.sha256(content.encode()).hexdigest(),
                "joiners": cast("list[JsonValue]", joiners),
                "logicalChunkId": logical_id,
                "logicalFlowId": flow_id,
                "parentChunkSeedId": seed,
                "parentOrdinal": parent.ordinal,
                "sectionOwnerSeedId": units[parent.fragments[0].unit_ordinal].owner_id,
                "spanFragments": cast("list[JsonValue]", fragments),
                "tokenCount": parent.token_count,
            }
        )
    children: list[JsonObject] = []
    for child in plan.children:
        fragments, joiners, content = _chunk_parts(
            units,
            child.fragments,
            child_ordinal=child.ordinal,
            overlap_tokens=child.overlap_before_tokens,
        )
        parent_id = parent_ids[child.parent_ordinal]
        child_id = _hash(
            "mineru_structure.child.v1",
            parent_id,
            child.ordinal,
            _hash_json(cast("JsonValue", fragments)),
        )
        children.append(
            {
                "childOrdinal": child.ordinal,
                "chunkKind": "child",
                "chunkProfileHash": MINERU_STRUCTURE_CHUNK_PROFILE_HASH,
                "chunkSourceSpanHash": _hash(
                    "mineru_structure.chunk_span.v1",
                    _hash_json(cast("JsonValue", fragments)),
                    _hash_json(cast("JsonValue", joiners)),
                ),
                "contentBytes": len(content.encode()),
                "contentHash": hashlib.sha256(content.encode()).hexdigest(),
                "joiners": cast("list[JsonValue]", joiners),
                "logicalChunkId": child_id,
                "logicalFlowId": flow_id,
                "logicalParentChunkId": parent_id,
                "parentChunkSeedId": parent_seeds[child.parent_ordinal],
                "spanFragments": cast("list[JsonValue]", fragments),
                "tokenCount": child.token_count,
            }
        )
    all_chunks = [*parents, *children]
    spans: list[JsonValue] = []
    aggregate_joiners: list[JsonValue] = []
    for chunk in all_chunks:
        spans.extend(cast("list[JsonValue]", chunk["spanFragments"]))
        aggregate_joiners.extend(cast("list[JsonValue]", chunk["joiners"]))
    return {
        "childAggregateHash": _hash_json(cast("JsonValue", children)),
        "childCount": len(children),
        "children": cast("list[JsonValue]", children),
        "chunkProfileHash": MINERU_STRUCTURE_CHUNK_PROFILE_HASH,
        "joinerAggregateHash": _hash_json(cast("JsonValue", aggregate_joiners)),
        "joinerCount": len(aggregate_joiners),
        "parentAggregateHash": _hash_json(cast("JsonValue", parents)),
        "parentCount": len(parents),
        "parents": cast("list[JsonValue]", parents),
        "schemaVersion": "chunk-manifest.v2",
        "sourceSha256": mapping.source_sha256,
        "spanAggregateHash": _hash_json(cast("JsonValue", spans)),
        "spanCount": len(spans),
    }


def _chunk_parts(
    units: tuple[_Unit, ...],
    plans: tuple[ChunkFragmentPlan, ...],
    *,
    child_ordinal: int | None = None,
    overlap_tokens: int = 0,
) -> tuple[list[JsonObject], list[JsonObject], str]:
    fragments: list[JsonObject] = []
    parts: list[str] = []
    overlap_group_id = _hash(
        "mineru_structure.overlap.v1",
        child_ordinal,
        overlap_tokens,
        units[plans[0].unit_ordinal].owner_id,
    )
    for plan in plans:
        unit = units[plan.unit_ordinal]
        content = unit.text.encode()[plan.start_byte : plan.end_byte].decode()
        parts.append(content)
        locator = _locator(unit, plan.start_byte, plan.end_byte)
        fragment: JsonObject = {
            "blockEndByte": plan.end_byte,
            "blockLogicalId": unit.block_id,
            "blockStartByte": plan.start_byte,
            "clippedLocatorSet": locator,
            "fragmentKind": "window_overlap" if plan.overlap else "primary",
            "fragmentSourceSpanHash": _hash(
                "mineru_structure.fragment.v1",
                unit.block_id,
                plan.start_byte,
                plan.end_byte,
                locator["aggregateHash"],
            ),
        }
        if plan.overlap:
            if (
                child_ordinal is None
                or child_ordinal < 1
                or overlap_tokens < _MIN_OVERLAP_TOKENS
            ):
                _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
            fragment.update(
                {
                    "overlapGroupId": overlap_group_id,
                    "overlapTokenCount": overlap_tokens,
                    "previousChildOrdinal": child_ordinal - 1,
                }
            )
        fragments.append(fragment)
    joiners: list[JsonObject] = []
    for index, (previous, current) in enumerate(pairwise(plans)):
        adjacent = (
            previous.unit_ordinal == current.unit_ordinal
            and previous.end_byte == current.start_byte
        )
        value = "" if adjacent else "\n\n"
        joiners.append(
            {"kind": "adjacent" if adjacent else "block_separator", "utf8Bytes": value}
        )
        parts.insert(index * 2 + 1, value)
    return fragments, joiners, "".join(parts)


def _bbox(value: JsonObject, *, width: int, height: int) -> tuple[int, int, int, int]:
    raw = value.get("bboxMilliPoint", value.get("bbox"))
    if (
        not isinstance(raw, list)
        or len(raw) != _BBOX_COORDINATES
        or not all(type(item) is int for item in raw)
    ):
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    x1, y1, x2, y2 = cast("tuple[int, int, int, int]", tuple(raw))
    if not (0 <= x1 < x2 <= width and 0 <= y1 < y2 <= height):
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return x1, y1, x2, y2


def _heading_level(value: JsonObject) -> int:
    level = value.get("level", 1)
    if type(level) is not int or not 1 <= level <= _MAX_HEADING_LEVEL:
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return level


def _object(value: object) -> JsonObject:
    if not isinstance(value, dict):
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return cast("JsonObject", value)


def _integer_alias(value: JsonObject, first: str, second: str) -> int:
    observed = value.get(first, value.get(second))
    if type(observed) is not int or observed < 0:
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return observed


def _positive_alias(value: JsonObject, first: str, second: str) -> int:
    observed = _integer_alias(value, first, second)
    if observed < 1:
        _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    return observed


def _role_hash(mapping: MinerULocalBatchCanonicalMappingInput, role: str) -> str:
    for item in mapping.role_digests:
        if item.role == role:
            return item.sha256
    _reject(MINERU_STRUCTURE_ARTIFACT_INVALID)
    raise AssertionError("unreachable")


def _hash(domain: str, *parts: object) -> str:
    digest = hashlib.sha256(domain.encode())
    for part in parts:
        digest.update(b"\x00")
        digest.update(str(part).encode())
    return digest.hexdigest()


def _hash_json(value: JsonValue) -> str:
    return hashlib.sha256(canonical_json_bytes(value)).hexdigest()


def _reject(code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(code))
