"""Canonical IR and chunk-manifest projection rows for G7 RAG indexing.

G7.4 keeps this module pure and deterministic: it converts already-validated
Canonical IR v2 plus Chunk Manifest v2 artifacts into the rows that later worker
slices insert into Postgres.  It does not call providers, read object storage, or
claim jobs.
"""

from __future__ import annotations

import hashlib
import re
import uuid
from dataclasses import dataclass
from decimal import Decimal
from typing import Final, cast

from mm_chat_rag.offline_parser.canonical import JsonObject, JsonValue
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
)

_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_EXACT_TERM_RE: Final = re.compile(r"[\w:-]{3,64}", re.UNICODE)
_MIN_EXACT_DIGITS: Final = 3
_BBOX_COORDINATES: Final = 4
_MAX_CONFIDENCE_BASIS_POINTS: Final = 10_000
_PROJECTION_NAMESPACE: Final = uuid.UUID("1df8f73e-24e4-5b37-a523-bb23724ebc81")
_LOCATOR_SUMMARY_VERSION: Final = "g7.4-locator-summary.v1"


class ProjectionError(ValueError):
    """Canonical parser artifacts cannot be projected safely."""


@dataclass(frozen=True, slots=True)
class ProjectionContext:
    """Stable database identity inputs owned by the worker/job transaction."""

    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    file_id: uuid.UUID
    artifact_set_id: uuid.UUID
    materialization_id: uuid.UUID
    index_generation_id: uuid.UUID


@dataclass(frozen=True, slots=True)
class BlockProjectionRow:
    """Row shape for `knowledge_blocks`."""

    id: uuid.UUID
    artifact_set_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    parent_block_id: uuid.UUID | None
    ordinal: int
    block_type: str
    heading_path: tuple[str, ...]
    text_content: str | None
    locator_kind: str
    locator: JsonObject
    reading_order: int
    provenance: JsonObject
    confidence: Decimal | None
    content_hash: str
    source_span_hash: str
    derived: bool
    non_indexable: bool
    needs_review: bool
    logical_block_id: str


@dataclass(frozen=True, slots=True)
class ParentChunkProjectionRow:
    """Row shape for `knowledge_parent_chunks`."""

    id: uuid.UUID
    materialization_id: uuid.UUID
    index_generation_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    ordinal: int
    chunk_profile_hash: str
    source_span_hash: str
    content_hash: str
    content: str
    token_count: int
    heading_path: tuple[str, ...]
    locator_summary: JsonObject
    logical_chunk_id: str


@dataclass(frozen=True, slots=True)
class ChildChunkProjectionRow:
    """Row shape for `knowledge_child_chunks`."""

    id: uuid.UUID
    parent_chunk_id: uuid.UUID
    materialization_id: uuid.UUID
    index_generation_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    ordinal: int
    chunk_profile_hash: str
    source_span_hash: str
    content_hash: str
    content: str
    token_count: int
    overlap_before_tokens: int
    overlap_after_tokens: int
    logical_chunk_id: str


@dataclass(frozen=True, slots=True)
class ChunkBlockSpanProjectionRow:
    """Row shape for `knowledge_chunk_block_spans`."""

    chunk_kind: str
    chunk_id: uuid.UUID
    block_id: uuid.UUID
    span_ordinal: int
    start_offset: int
    end_offset: int
    fragment_source_span_hash: str


@dataclass(frozen=True, slots=True)
class ChildSearchProjectionRow:
    """Extension-independent search lane row for G7 Postgres projection."""

    child_chunk_id: uuid.UUID
    parent_chunk_id: uuid.UUID
    materialization_id: uuid.UUID
    index_generation_id: uuid.UUID
    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    embedding_model_id: str
    embedding_dimensions: int
    lexical_text: str
    exact_terms: tuple[str, ...]
    source_span_hash: str
    chunk_profile_hash: str
    content_hash: str
    locator_summary: JsonObject


@dataclass(frozen=True, slots=True)
class PostgresProjectionBatch:
    """Deterministic row batch staged before any Postgres write."""

    blocks: tuple[BlockProjectionRow, ...]
    parent_chunks: tuple[ParentChunkProjectionRow, ...]
    child_chunks: tuple[ChildChunkProjectionRow, ...]
    chunk_block_spans: tuple[ChunkBlockSpanProjectionRow, ...]
    child_search_projections: tuple[ChildSearchProjectionRow, ...]
    source_sha256: str
    chunk_profile_hash: str


def stable_projection_uuid(
    scope_id: uuid.UUID, kind: str, logical_id: str
) -> uuid.UUID:
    """Derive stable UUIDs from immutable artifact/materialization scope."""
    if not kind or not kind.isascii() or any(char.isspace() for char in kind):
        raise ProjectionError("projection UUID kind is invalid")
    if not _SHA256_RE.fullmatch(logical_id):
        raise ProjectionError("projection logical id must be a sha256 hex value")
    return uuid.uuid5(_PROJECTION_NAMESPACE, f"{scope_id}:{kind}:{logical_id}")


def build_postgres_projection_batch(
    canonical_ir: JsonObject,
    chunk_manifest: JsonObject,
    context: ProjectionContext,
) -> PostgresProjectionBatch:
    """Project Canonical IR v2 and Chunk Manifest v2 into Postgres row DTOs."""
    _require_literal(canonical_ir, "schemaVersion", "canonical-ir.v2", "canonicalIr")
    _require_literal(
        chunk_manifest, "schemaVersion", "chunk-manifest.v2", "chunkManifest"
    )
    source = _object(canonical_ir.get("source"), "canonicalIr.source")
    source_sha256 = _sha256(source.get("sha256"), "canonicalIr.source.sha256")
    if chunk_manifest.get("sourceSha256") != source_sha256:
        raise ProjectionError("chunk manifest sourceSha256 does not match Canonical IR")
    chunk_profile_hash = _sha256(
        chunk_manifest.get("chunkProfileHash"), "chunkManifest.chunkProfileHash"
    )

    text_buffer = _object(canonical_ir.get("textBuffer"), "canonicalIr.textBuffer")
    text = _string(text_buffer.get("text"), "canonicalIr.textBuffer.text")
    if hashlib.sha256(text.encode("utf-8")).hexdigest() != text_buffer.get("sha256"):
        raise ProjectionError("Canonical IR textBuffer hash is stale")

    block_objects = _objects(canonical_ir.get("blocks"), "canonicalIr.blocks")
    blocks, block_id_by_logical, block_text_by_logical, heading_by_logical = (
        _project_blocks(block_objects, text, context)
    )

    parent_objects = _objects(chunk_manifest.get("parents"), "chunkManifest.parents")
    child_objects = _objects(chunk_manifest.get("children"), "chunkManifest.children")
    parent_chunks, parent_spans, parent_id_by_logical = _project_parent_chunks(
        parent_objects,
        block_id_by_logical,
        block_text_by_logical,
        heading_by_logical,
        context,
    )
    child_chunks, child_spans, search_rows = _project_child_chunks(
        child_objects,
        block_id_by_logical,
        block_text_by_logical,
        parent_id_by_logical,
        context,
    )

    _validate_manifest_counts(
        chunk_manifest,
        parent_count=len(parent_chunks),
        child_count=len(child_chunks),
        span_count=len(parent_spans) + len(child_spans),
    )
    return PostgresProjectionBatch(
        blocks=blocks,
        parent_chunks=parent_chunks,
        child_chunks=child_chunks,
        chunk_block_spans=parent_spans + child_spans,
        child_search_projections=search_rows,
        source_sha256=source_sha256,
        chunk_profile_hash=chunk_profile_hash,
    )


def extract_exact_terms(content: str) -> tuple[str, ...]:
    """Build the deterministic exact lane seed from child text."""
    terms = {
        match.group(0).casefold()
        for match in _EXACT_TERM_RE.finditer(content)
        if not match.group(0).isdigit() or len(match.group(0)) >= _MIN_EXACT_DIGITS
    }
    return tuple(sorted(terms))


def _project_blocks(
    block_objects: tuple[JsonObject, ...],
    text: str,
    context: ProjectionContext,
) -> tuple[
    tuple[BlockProjectionRow, ...],
    dict[str, uuid.UUID],
    dict[str, str],
    dict[str, tuple[str, ...]],
]:
    rows: list[BlockProjectionRow] = []
    block_id_by_logical: dict[str, uuid.UUID] = {}
    block_text_by_logical: dict[str, str] = {}
    heading_by_logical: dict[str, tuple[str, ...]] = {}

    for block in block_objects:
        logical_id = _sha256(block.get("logicalBlockId"), "block.logicalBlockId")
        block_id_by_logical[logical_id] = stable_projection_uuid(
            context.artifact_set_id, "block", logical_id
        )

    for expected_reading_order, block in enumerate(block_objects):
        logical_id = _sha256(block.get("logicalBlockId"), "block.logicalBlockId")
        block_id = block_id_by_logical[logical_id]
        parent_logical = block.get("parentBlockId")
        parent_block_id = None
        if parent_logical is not None:
            parent_block_id = block_id_by_logical[
                _sha256(parent_logical, "block.parentBlockId")
            ]
        text_range = block.get("textRange")
        text_content = None
        if text_range is not None:
            text_range_object = _object(text_range, "block.textRange")
            text_content = _slice_utf8(
                text,
                _integer(
                    text_range_object.get("startByte"), "block.textRange.startByte"
                ),
                _integer(text_range_object.get("endByte"), "block.textRange.endByte"),
            )
            block_text_by_logical[logical_id] = text_content
        locator_kind, locator = _best_locator(
            _object(block.get("locatorSet"), "block.locatorSet")
        )
        flags = _object(block.get("flags"), "block.flags")
        heading_path = tuple(
            _sha256(value, "block.headingPath[]")
            for value in _array(block.get("headingPath"), "block.headingPath")
        )
        heading_by_logical[logical_id] = heading_path
        rows.append(
            BlockProjectionRow(
                id=block_id,
                artifact_set_id=context.artifact_set_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                parent_block_id=parent_block_id,
                ordinal=_integer(block.get("ordinal"), "block.ordinal"),
                block_type=_string(block.get("blockType"), "block.blockType"),
                heading_path=heading_path,
                text_content=text_content,
                locator_kind=locator_kind,
                locator=locator,
                reading_order=expected_reading_order,
                provenance={
                    "provenanceRefs": block.get("provenanceRefs", []),
                    "sourceSpanHash": block.get("sourceSpanHash"),
                    "structureRef": block.get("structureRef"),
                },
                confidence=_confidence(block.get("confidence")),
                content_hash=_sha256(block.get("contentHash"), "block.contentHash"),
                source_span_hash=_source_span_hash(block, "block.sourceSpanHash"),
                derived=_boolean(flags.get("derived"), "block.flags.derived"),
                non_indexable=_boolean(
                    flags.get("nonIndexable"), "block.flags.nonIndexable"
                ),
                needs_review=False,
                logical_block_id=logical_id,
            )
        )
    return tuple(rows), block_id_by_logical, block_text_by_logical, heading_by_logical


def _project_parent_chunks(
    chunks: tuple[JsonObject, ...],
    block_id_by_logical: dict[str, uuid.UUID],
    block_text_by_logical: dict[str, str],
    heading_by_logical: dict[str, tuple[str, ...]],
    context: ProjectionContext,
) -> tuple[
    tuple[ParentChunkProjectionRow, ...],
    tuple[ChunkBlockSpanProjectionRow, ...],
    dict[str, uuid.UUID],
]:
    rows: list[ParentChunkProjectionRow] = []
    spans: list[ChunkBlockSpanProjectionRow] = []
    parent_id_by_logical: dict[str, uuid.UUID] = {}
    for expected_ordinal, chunk in enumerate(chunks):
        _require_literal(chunk, "chunkKind", "parent", "parentChunk")
        ordinal = _integer(chunk.get("parentOrdinal"), "parentChunk.parentOrdinal")
        if ordinal != expected_ordinal:
            raise ProjectionError("parent chunk ordinals must be contiguous")
        logical_id = _sha256(chunk.get("logicalChunkId"), "parentChunk.logicalChunkId")
        chunk_id = stable_projection_uuid(
            context.materialization_id, "parent", logical_id
        )
        parent_id_by_logical[logical_id] = chunk_id
        content = _chunk_content(chunk, block_text_by_logical)
        _assert_content_binding(chunk, content, "parentChunk")
        chunk_spans = _chunk_spans(
            chunk,
            chunk_kind="parent",
            chunk_id=chunk_id,
            block_id_by_logical=block_id_by_logical,
        )
        spans.extend(chunk_spans)
        first_block = _first_span_block_logical_id(chunk)
        rows.append(
            ParentChunkProjectionRow(
                id=chunk_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                ordinal=ordinal,
                chunk_profile_hash=_sha256(
                    chunk.get("chunkProfileHash"), "parentChunk.chunkProfileHash"
                ),
                source_span_hash=_sha256(
                    chunk.get("chunkSourceSpanHash"), "parentChunk.chunkSourceSpanHash"
                ),
                content_hash=_sha256(
                    chunk.get("contentHash"), "parentChunk.contentHash"
                ),
                content=content,
                token_count=_positive_integer(
                    chunk.get("tokenCount"), "parentChunk.tokenCount"
                ),
                heading_path=heading_by_logical.get(first_block, ()),
                locator_summary=_locator_summary(chunk),
                logical_chunk_id=logical_id,
            )
        )
    return tuple(rows), tuple(spans), parent_id_by_logical


def _project_child_chunks(
    chunks: tuple[JsonObject, ...],
    block_id_by_logical: dict[str, uuid.UUID],
    block_text_by_logical: dict[str, str],
    parent_id_by_logical: dict[str, uuid.UUID],
    context: ProjectionContext,
) -> tuple[
    tuple[ChildChunkProjectionRow, ...],
    tuple[ChunkBlockSpanProjectionRow, ...],
    tuple[ChildSearchProjectionRow, ...],
]:
    rows: list[ChildChunkProjectionRow] = []
    spans: list[ChunkBlockSpanProjectionRow] = []
    search_rows: list[ChildSearchProjectionRow] = []
    for expected_ordinal, chunk in enumerate(chunks):
        _require_literal(chunk, "chunkKind", "child", "childChunk")
        ordinal = _integer(chunk.get("childOrdinal"), "childChunk.childOrdinal")
        if ordinal != expected_ordinal:
            raise ProjectionError("child chunk ordinals must be contiguous")
        logical_id = _sha256(chunk.get("logicalChunkId"), "childChunk.logicalChunkId")
        parent_logical_id = _sha256(
            chunk.get("logicalParentChunkId"), "childChunk.logicalParentChunkId"
        )
        try:
            parent_id = parent_id_by_logical[parent_logical_id]
        except KeyError as error:
            raise ProjectionError("child chunk references a missing parent") from error
        chunk_id = stable_projection_uuid(
            context.materialization_id, "child", logical_id
        )
        content = _chunk_content(chunk, block_text_by_logical)
        _assert_content_binding(chunk, content, "childChunk")
        locator_summary = _locator_summary(chunk)
        chunk_spans = _chunk_spans(
            chunk,
            chunk_kind="child",
            chunk_id=chunk_id,
            block_id_by_logical=block_id_by_logical,
        )
        spans.extend(chunk_spans)
        chunk_profile_hash = _sha256(
            chunk.get("chunkProfileHash"), "childChunk.chunkProfileHash"
        )
        source_span_hash = _sha256(
            chunk.get("chunkSourceSpanHash"), "childChunk.chunkSourceSpanHash"
        )
        content_hash = _sha256(chunk.get("contentHash"), "childChunk.contentHash")
        rows.append(
            ChildChunkProjectionRow(
                id=chunk_id,
                parent_chunk_id=parent_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                ordinal=ordinal,
                chunk_profile_hash=chunk_profile_hash,
                source_span_hash=source_span_hash,
                content_hash=content_hash,
                content=content,
                token_count=_positive_integer(
                    chunk.get("tokenCount"), "childChunk.tokenCount"
                ),
                overlap_before_tokens=_overlap_before(chunk),
                overlap_after_tokens=0,
                logical_chunk_id=logical_id,
            )
        )
        search_rows.append(
            ChildSearchProjectionRow(
                child_chunk_id=chunk_id,
                parent_chunk_id=parent_id,
                materialization_id=context.materialization_id,
                index_generation_id=context.index_generation_id,
                collection_id=context.collection_id,
                document_id=context.document_id,
                document_version_id=context.document_version_id,
                embedding_model_id=DEFAULT_JINA_EMBEDDING_MODEL,
                embedding_dimensions=DEFAULT_JINA_EMBEDDING_DIMENSIONS,
                lexical_text=content,
                exact_terms=extract_exact_terms(content),
                source_span_hash=source_span_hash,
                chunk_profile_hash=chunk_profile_hash,
                content_hash=content_hash,
                locator_summary=locator_summary,
            )
        )
    return tuple(rows), tuple(spans), tuple(search_rows)


def _chunk_content(chunk: JsonObject, block_text_by_logical: dict[str, str]) -> str:
    fragments = _objects(chunk.get("spanFragments"), "chunk.spanFragments")
    joiners = _objects(chunk.get("joiners"), "chunk.joiners")
    if len(joiners) not in {0, max(0, len(fragments) - 1)}:
        raise ProjectionError("chunk joiner count does not match span fragments")
    parts: list[str] = []
    for index, fragment in enumerate(fragments):
        logical_block_id = _sha256(
            fragment.get("blockLogicalId"), "chunk.spanFragments[].blockLogicalId"
        )
        try:
            block_text = block_text_by_logical[logical_block_id]
        except KeyError as error:
            raise ProjectionError(
                "chunk references a non-text or missing block"
            ) from error
        parts.append(
            _slice_utf8(
                block_text,
                _integer(fragment.get("blockStartByte"), "fragment.blockStartByte"),
                _integer(fragment.get("blockEndByte"), "fragment.blockEndByte"),
            )
        )
        if index < len(fragments) - 1:
            joiner = joiners[index]
            parts.append(_string(joiner.get("utf8Bytes"), "chunk.joiners[].utf8Bytes"))
    return "".join(parts)


def _chunk_spans(
    chunk: JsonObject,
    *,
    chunk_kind: str,
    chunk_id: uuid.UUID,
    block_id_by_logical: dict[str, uuid.UUID],
) -> tuple[ChunkBlockSpanProjectionRow, ...]:
    rows: list[ChunkBlockSpanProjectionRow] = []
    fragments = _objects(chunk.get("spanFragments"), "chunk.spanFragments")
    for span_ordinal, fragment in enumerate(fragments):
        logical_block_id = _sha256(
            fragment.get("blockLogicalId"), "chunk.spanFragments[].blockLogicalId"
        )
        try:
            block_id = block_id_by_logical[logical_block_id]
        except KeyError as error:
            raise ProjectionError("chunk span references a missing block") from error
        rows.append(
            ChunkBlockSpanProjectionRow(
                chunk_kind=chunk_kind,
                chunk_id=chunk_id,
                block_id=block_id,
                span_ordinal=span_ordinal,
                start_offset=_integer(
                    fragment.get("blockStartByte"), "fragment.blockStartByte"
                ),
                end_offset=_integer(
                    fragment.get("blockEndByte"), "fragment.blockEndByte"
                ),
                fragment_source_span_hash=_sha256(
                    fragment.get("fragmentSourceSpanHash"),
                    "fragment.fragmentSourceSpanHash",
                ),
            )
        )
    return tuple(rows)


def _locator_summary(chunk: JsonObject) -> JsonObject:
    fragments = _objects(chunk.get("spanFragments"), "chunk.spanFragments")
    locators: list[JsonValue] = []
    aggregate_hashes: list[JsonValue] = []
    primary: JsonObject | None = None
    for fragment in fragments:
        locator_set = _object(
            fragment.get("clippedLocatorSet"), "fragment.clippedLocatorSet"
        )
        locator_kind, locator = _best_locator(locator_set)
        item: JsonObject = {
            "kind": locator_kind,
            "locator": locator,
            "locatorAggregateHash": _sha256(
                locator_set.get("aggregateHash"), "locatorSet.aggregateHash"
            ),
        }
        locators.append(item)
        aggregate_hashes.append(item["locatorAggregateHash"])
        primary = primary or item
    if primary is None:
        raise ProjectionError("chunk requires at least one locator fragment")
    return {
        "schemaVersion": _LOCATOR_SUMMARY_VERSION,
        "primary": primary,
        "fragments": locators,
        "locatorAggregateHashes": aggregate_hashes,
    }


def _best_locator(locator_set: JsonObject) -> tuple[str, JsonObject]:
    for anchor in _objects(locator_set.get("textAnchors"), "locatorSet.textAnchors"):
        for fragment in _objects(
            anchor.get("sourceFragments"), "textAnchor.sourceFragments"
        ):
            for view in _objects(fragment.get("views"), "sourceFragment.views"):
                kind = _string(view.get("kind"), "locator view kind")
                if kind == "page_region":
                    bbox = _array(
                        view.get("bboxMilliPoint"), "page_region.bboxMilliPoint"
                    )
                    if len(bbox) != _BBOX_COORDINATES:
                        raise ProjectionError("page bbox must have four coordinates")
                    return (
                        "page_bbox",
                        {
                            "kind": "page_bbox",
                            "page": _integer(
                                view.get("pageIndex"), "page_region.pageIndex"
                            ),
                            "x1": _integer(bbox[0], "page_region.x1"),
                            "y1": _integer(bbox[1], "page_region.y1"),
                            "x2": _integer(bbox[2], "page_region.x2"),
                            "y2": _integer(bbox[3], "page_region.y2"),
                        },
                    )
                if kind == "sheet_range":
                    return (
                        "sheet_cell",
                        {
                            "kind": "sheet_cell",
                            "sheet": _sha256(
                                view.get("opaqueSheetId"), "sheet_range.opaqueSheetId"
                            ),
                            "startCell": _string(
                                view.get("startCell"), "sheet_range.startCell"
                            ),
                            "endCell": _string(
                                view.get("endCell"), "sheet_range.endCell"
                            ),
                        },
                    )
                if kind == "slide_shape":
                    return (
                        "slide_shape",
                        {
                            "kind": "slide_shape",
                            "slide": _integer(
                                view.get("slideIndex"), "slide_shape.slideIndex"
                            ),
                            "shape": _shape_integer(
                                view.get("opaqueShapeId"), "slide_shape.opaqueShapeId"
                            ),
                        },
                    )
                if kind == "ooxml_path":
                    return (
                        "ooxml_part_xpath",
                        {
                            "kind": "ooxml_part_xpath",
                            "part": _sha256(
                                view.get("opaqueSourceUnitId"),
                                "ooxml_path.opaqueSourceUnitId",
                            ),
                            "xpath": _string(
                                view.get("canonicalXPathPayloadRef"),
                                "ooxml_path.canonicalXPathPayloadRef",
                            ),
                        },
                    )
                if kind == "source_text_position":
                    return (
                        "line_range",
                        {
                            "kind": "line_range",
                            "startLine": _integer(
                                view.get("startLine"), "source_text_position.startLine"
                            ),
                            "endLine": _integer(
                                view.get("endLine"), "source_text_position.endLine"
                            ),
                        },
                    )
    for anchor in _objects(
        locator_set.get("structuralAnchors"), "locatorSet.structuralAnchors"
    ):
        for fragment in _objects(
            anchor.get("sourceFragments"), "structural.sourceFragments"
        ):
            for view in _objects(fragment.get("views"), "sourceFragment.views"):
                if view.get("kind") == "derived_structure":
                    return (
                        "text_offset",
                        {
                            "kind": "text_offset",
                            "start": _integer(
                                anchor.get("structureOrdinal"),
                                "structural.structureOrdinal",
                            ),
                            "end": _integer(
                                anchor.get("structureOrdinal"),
                                "structural.structureOrdinal",
                            )
                            + 1,
                        },
                    )
    raise ProjectionError("locatorSet has no projectable locator view")


def _source_span_hash(block: JsonObject, label: str) -> str:
    value = _object(block.get("sourceSpanHash"), label)
    kind = _string(value.get("kind"), f"{label}.kind")
    if kind == "text":
        return _sha256(value.get("textSourceSpanHash"), f"{label}.textSourceSpanHash")
    if kind == "structural":
        return _sha256(
            value.get("structuralSourceSpanHash"), f"{label}.structuralSourceSpanHash"
        )
    raise ProjectionError("sourceSpanHash kind is unsupported")


def _first_span_block_logical_id(chunk: JsonObject) -> str:
    fragments = _objects(chunk.get("spanFragments"), "chunk.spanFragments")
    if not fragments:
        raise ProjectionError("chunk requires at least one span fragment")
    return _sha256(fragments[0].get("blockLogicalId"), "fragment.blockLogicalId")


def _overlap_before(chunk: JsonObject) -> int:
    total = 0
    for fragment in _objects(chunk.get("spanFragments"), "chunk.spanFragments"):
        if fragment.get("fragmentKind") == "window_overlap":
            total += _integer(
                fragment.get("overlapTokenCount"), "window_overlap.overlapTokenCount"
            )
    return total


def _assert_content_binding(chunk: JsonObject, content: str, label: str) -> None:
    content_bytes = content.encode("utf-8")
    if len(content_bytes) != _integer(
        chunk.get("contentBytes"), f"{label}.contentBytes"
    ):
        raise ProjectionError(f"{label} content byte count is stale")
    if hashlib.sha256(content_bytes).hexdigest() != chunk.get("contentHash"):
        raise ProjectionError(f"{label} content hash is stale")


def _validate_manifest_counts(
    chunk_manifest: JsonObject,
    *,
    parent_count: int,
    child_count: int,
    span_count: int,
) -> None:
    if (
        _integer(chunk_manifest.get("parentCount"), "chunkManifest.parentCount")
        != parent_count
    ):
        raise ProjectionError("chunk manifest parentCount is stale")
    if (
        _integer(chunk_manifest.get("childCount"), "chunkManifest.childCount")
        != child_count
    ):
        raise ProjectionError("chunk manifest childCount is stale")
    if (
        _integer(chunk_manifest.get("spanCount"), "chunkManifest.spanCount")
        != span_count
    ):
        raise ProjectionError("chunk manifest spanCount is stale")


def _slice_utf8(text: str, start: int, end: int) -> str:
    if start < 0 or end < start:
        raise ProjectionError("UTF-8 byte range is invalid")
    encoded = text.encode("utf-8")
    if end > len(encoded):
        raise ProjectionError("UTF-8 byte range exceeds text buffer")
    try:
        return encoded[start:end].decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise ProjectionError("UTF-8 byte range splits a code point") from error


def _confidence(value: JsonValue | object) -> Decimal | None:
    if value is None:
        return None
    basis_points = _integer(value, "block.confidence")
    if not 0 <= basis_points <= _MAX_CONFIDENCE_BASIS_POINTS:
        raise ProjectionError("block confidence is outside basis-point range")
    return Decimal(basis_points) / Decimal(_MAX_CONFIDENCE_BASIS_POINTS)


def _shape_integer(value: JsonValue | object, label: str) -> int:
    digest = _sha256(value, label)
    return int(digest[:15], 16)


def _require_literal(
    document: JsonObject, field: str, expected: str, label: str
) -> None:
    if document.get(field) != expected:
        raise ProjectionError(f"{label}.{field} is unsupported")


def _object(value: JsonValue | object, label: str) -> JsonObject:
    if not isinstance(value, dict):
        raise ProjectionError(f"{label} must be an object")
    return cast("JsonObject", value)


def _objects(value: JsonValue | object, label: str) -> tuple[JsonObject, ...]:
    return tuple(_object(item, f"{label}[]") for item in _array(value, label))


def _array(value: JsonValue | object, label: str) -> tuple[JsonValue, ...]:
    if not isinstance(value, list):
        raise ProjectionError(f"{label} must be an array")
    return tuple(cast("list[JsonValue]", value))


def _string(value: JsonValue | object, label: str) -> str:
    if not isinstance(value, str):
        raise ProjectionError(f"{label} must be a string")
    return value


def _sha256(value: JsonValue | object, label: str) -> str:
    text = _string(value, label)
    if not _SHA256_RE.fullmatch(text):
        raise ProjectionError(f"{label} must be a sha256 hex string")
    return text


def _integer(value: JsonValue | object, label: str) -> int:
    if type(value) is not int:
        raise ProjectionError(f"{label} must be an integer")
    return value


def _positive_integer(value: JsonValue | object, label: str) -> int:
    integer = _integer(value, label)
    if integer <= 0:
        raise ProjectionError(f"{label} must be positive")
    return integer


def _boolean(value: JsonValue | object, label: str) -> bool:
    if type(value) is not bool:
        raise ProjectionError(f"{label} must be a boolean")
    return value
