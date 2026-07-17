"""Projection-ready basic text baseline for sandboxed Native Artifacts."""

from __future__ import annotations

import hashlib
import json
import math
import uuid
from collections.abc import Sequence
from typing import Final, NoReturn, cast

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.mineru_gateway import MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.offline_parser.canonical import JsonObject, JsonValue
from mm_chat_rag.offline_parser.native.model import NativeDocument
from mm_chat_rag.retry import PermanentJobError

NATIVE_TEXT_BASELINE_CONTEXT_INVALID: Final = "NATIVE_TEXT_BASELINE_CONTEXT_INVALID"
NATIVE_TEXT_BASELINE_ARTIFACT_INVALID: Final = "NATIVE_TEXT_BASELINE_ARTIFACT_INVALID"
_ARTIFACT_NAMESPACE: Final = uuid.UUID("c643e725-a95b-5339-9ec9-14d2dd6a68ae")
_CHUNK_MAX_BYTES: Final = 2400
_TOKEN_BYTES: Final = 4
_CHILD_MAX_TOKENS: Final = 650


def build_native_text_baseline_artifacts(
    context: ProcessingJobContext,
    source: DocumentSource,
    artifact: NativeDocument,
    text: str,
    *,
    parser_model: str,
) -> ParsedDocumentArtifacts:
    """Convert verified Native text into the current basic projection contract."""
    materialization_id = context.materialization_id
    if materialization_id is None:
        _reject(NATIVE_TEXT_BASELINE_CONTEXT_INVALID)
    text_bytes = text.encode("utf-8")
    artifact_set_id = uuid.uuid5(
        _ARTIFACT_NAMESPACE,
        ":".join(
            (
                str(materialization_id),
                source.source_sha256,
                artifact.artifact_sha256,
                MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            )
        ),
    )
    source_unit_id = _hash_seed(
        "native_text_baseline.source_unit.v1",
        artifact.artifact_sha256,
    )
    flow_seed_id = _hash_seed(
        "native_text_baseline.flow_seed.v1",
        source.source_sha256,
        artifact.artifact_sha256,
    )
    logical_flow_id = _hash_seed("native_text_baseline.flow.v1", flow_seed_id)
    structure_id = _hash_seed(
        "native_text_baseline.structure.v1",
        source.source_sha256,
        artifact.artifact_sha256,
    )
    block_id = _hash_seed("native_text_baseline.block.v1", structure_id)
    block_span_hash = _hash_seed(
        "native_text_baseline.text_span.v1",
        source_unit_id,
        0,
        len(text_bytes),
    )
    locator = _locator_set(
        text,
        start_byte=0,
        end_byte=len(text_bytes),
        source_unit_id=source_unit_id,
        structure_id=structure_id,
    )
    provenance_id = _hash_seed(
        "native_text_baseline.provenance_id.v1",
        block_id,
        artifact.artifact_sha256,
    )
    provenance = _provenance(
        provenance_id=provenance_id,
        target_owner_seed_id=structure_id,
        source_unit_id=source_unit_id,
        payload_ref=artifact.artifact_sha256,
    )
    block: JsonObject = {
        "blockType": "paragraph",
        "confidence": 10000,
        "contentHash": hashlib.sha256(text_bytes).hexdigest(),
        "flags": {"derived": True, "nonIndexable": False},
        "flowSeedId": flow_seed_id,
        "headingPath": [],
        "locatorSet": locator,
        "logicalBlockId": block_id,
        "ordinal": 0,
        "parentBlockId": None,
        "provenanceRefs": [provenance_id],
        "readingFlowOrdinal": 0,
        "sourceSpanHash": {
            "kind": "text",
            "textSourceSpanHash": block_span_hash,
        },
        "structureRef": {
            "ownerSeedId": structure_id,
            "structureKind": "paragraph",
            "structureOrdinal": 0,
        },
        "textRange": {"endByte": len(text_bytes), "startByte": 0},
    }
    normalization_value = {
        "nativeArtifactSha256": artifact.artifact_sha256,
        "profile": "mm-chat.native.text-baseline.v1",
        "sourceSha256": source.source_sha256,
    }
    normalization_bytes = _canonical_json_bytes(normalization_value)
    canonical_ir: JsonObject = {
        "assets": [],
        "blocks": [block],
        "formulas": [],
        "normalizationMapRef": {
            "bytes": len(normalization_bytes),
            "schemaVersion": "normalization-map.v1",
            "sha256": hashlib.sha256(normalization_bytes).hexdigest(),
        },
        "normalizationProfile": {
            "profileHash": _hash_seed("native_text_baseline.normalization_profile.v1"),
            "schemaVersion": "normalization-profile.v1",
        },
        "pages": [],
        "parser": {
            "configHash": _hash_seed(
                "native_text_baseline.parser_config.v1",
                artifact.schema_version,
            ),
            "parserBuildHash": _hash_seed(
                "native_text_baseline.parser_build.v1",
                parser_model,
            ),
            "profileHash": _hash_seed("native_text_baseline.parser_profile.v1"),
            "schemaVersion": "parser-profile.v1",
        },
        "provenance": [provenance],
        "readingFlows": [
            {
                "flowOrdinal": 0,
                "flowSeedId": flow_seed_id,
                "logicalFlowId": logical_flow_id,
                "orderedLogicalBlockIds": [block_id],
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
    chunk_manifest = _chunk_manifest(
        text,
        block_id=block_id,
        logical_flow_id=logical_flow_id,
        source_unit_id=source_unit_id,
        structure_id=structure_id,
        source_sha256=source.source_sha256,
    )
    return ParsedDocumentArtifacts(
        artifact_set_id=artifact_set_id,
        canonical_ir=canonical_ir,
        chunk_manifest=chunk_manifest,
    )


def _provenance(
    *,
    provenance_id: str,
    target_owner_seed_id: str,
    source_unit_id: str,
    payload_ref: str,
) -> JsonObject:
    base: JsonObject = {
        "derivationProfileHash": _hash_seed(
            "native_text_baseline.derivation_profile.v1"
        ),
        "payloadRef": payload_ref,
        "provenanceId": provenance_id,
        "provenanceKind": "parser_derivation",
        "provenanceOrdinal": 0,
        "sourceUnitRef": source_unit_id,
        "targetKind": "block",
        "targetKindRank": 0,
        "targetOwnerSeedId": target_owner_seed_id,
    }
    return {**base, "provenanceHash": _hash_json(base)}


def _chunk_manifest(
    text: str,
    *,
    block_id: str,
    logical_flow_id: str,
    source_unit_id: str,
    structure_id: str,
    source_sha256: str,
) -> JsonObject:
    parents: list[JsonObject] = []
    children: list[JsonObject] = []
    span_hashes: list[str] = []
    for ordinal, (start_byte, end_byte, content) in enumerate(_chunk_ranges(text)):
        content_bytes = content.encode("utf-8")
        content_hash = hashlib.sha256(content_bytes).hexdigest()
        span_hash = _hash_seed(
            "native_text_baseline.chunk_span.v1",
            block_id,
            start_byte,
            end_byte,
        )
        parent_id = _hash_seed(
            "native_text_baseline.parent_chunk.v1",
            block_id,
            start_byte,
            end_byte,
        )
        child_id = _hash_seed(
            "native_text_baseline.child_chunk.v1",
            parent_id,
            ordinal,
        )
        parent_seed_id = _hash_seed(
            "native_text_baseline.parent_seed.v1",
            parent_id,
        )
        fragment = _chunk_fragment(
            text,
            block_id=block_id,
            start_byte=start_byte,
            end_byte=end_byte,
            source_unit_id=source_unit_id,
            structure_id=structure_id,
            span_hash=span_hash,
        )
        common: JsonObject = {
            "chunkProfileHash": MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
            "chunkSourceSpanHash": span_hash,
            "contentBytes": len(content_bytes),
            "contentHash": content_hash,
            "joiners": [],
            "logicalFlowId": logical_flow_id,
            "parentChunkSeedId": parent_seed_id,
            "spanFragments": [fragment],
            "tokenCount": _estimated_token_count(content_bytes),
        }
        parents.append(
            {
                **common,
                "chunkKind": "parent",
                "logicalChunkId": parent_id,
                "parentOrdinal": ordinal,
                "sectionOwnerSeedId": structure_id,
            }
        )
        children.append(
            {
                **common,
                "childOrdinal": ordinal,
                "chunkKind": "child",
                "logicalChunkId": child_id,
                "logicalParentChunkId": parent_id,
            }
        )
        span_hashes.extend((span_hash, span_hash))
    return {
        "childAggregateHash": _hash_sequence(
            "native_text_baseline.children.v1",
            [cast("str", child["logicalChunkId"]) for child in children],
        ),
        "childCount": len(children),
        "children": cast("list[JsonValue]", children),
        "chunkProfileHash": MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH,
        "joinerAggregateHash": _hash_sequence("native_text_baseline.joiners.v1", []),
        "joinerCount": 0,
        "parentAggregateHash": _hash_sequence(
            "native_text_baseline.parents.v1",
            [cast("str", parent["logicalChunkId"]) for parent in parents],
        ),
        "parentCount": len(parents),
        "parents": cast("list[JsonValue]", parents),
        "schemaVersion": "chunk-manifest.v2",
        "sourceSha256": source_sha256,
        "spanAggregateHash": _hash_sequence(
            "native_text_baseline.spans.v1", span_hashes
        ),
        "spanCount": len(span_hashes),
    }


def _chunk_fragment(
    text: str,
    *,
    block_id: str,
    start_byte: int,
    end_byte: int,
    source_unit_id: str,
    structure_id: str,
    span_hash: str,
) -> JsonObject:
    return {
        "blockEndByte": end_byte,
        "blockLogicalId": block_id,
        "blockStartByte": start_byte,
        "clippedLocatorSet": _locator_set(
            text,
            start_byte=start_byte,
            end_byte=end_byte,
            source_unit_id=source_unit_id,
            structure_id=structure_id,
        ),
        "fragmentKind": "primary",
        "fragmentSourceSpanHash": span_hash,
    }


def _locator_set(
    text: str,
    *,
    start_byte: int,
    end_byte: int,
    source_unit_id: str,
    structure_id: str,
) -> JsonObject:
    start_line, start_column, start_scalar = _line_column(text, start_byte)
    end_line, end_column, end_scalar = _line_column(text, end_byte)
    base: JsonObject = {
        "structuralAnchors": [],
        "textAnchors": [
            {
                "anchorOrdinal": 0,
                "canonicalEndByte": end_byte,
                "canonicalStartByte": start_byte,
                "sourceFragments": [
                    {
                        "fragmentOrdinal": 0,
                        "views": [
                            {
                                "decodedScalarEnd": end_scalar,
                                "decodedScalarStart": start_scalar,
                                "endColumn": end_column,
                                "endLine": end_line,
                                "kind": "source_text_position",
                                "opaqueSourceUnitId": source_unit_id,
                                "rawByteEnd": end_byte,
                                "rawByteStart": start_byte,
                                "startColumn": start_column,
                                "startLine": start_line,
                            },
                            {
                                "kind": "derived_structure",
                                "opaqueStructureId": structure_id,
                                "structureKind": "paragraph",
                            },
                        ],
                    }
                ],
            }
        ],
        "version": 2,
    }
    return {**base, "aggregateHash": _hash_json(base)}


def _chunk_ranges(text: str) -> tuple[tuple[int, int, str], ...]:
    ranges: list[tuple[int, int, str]] = []
    current: list[str] = []
    current_bytes = 0
    start_byte = 0
    total_bytes = 0
    for character in text:
        character_bytes = len(character.encode("utf-8"))
        if current and current_bytes + character_bytes > _CHUNK_MAX_BYTES:
            chunk_text = "".join(current)
            end_byte = start_byte + current_bytes
            ranges.append((start_byte, end_byte, chunk_text))
            start_byte = end_byte
            current = []
            current_bytes = 0
        current.append(character)
        current_bytes += character_bytes
        total_bytes += character_bytes
    if current:
        ranges.append((start_byte, start_byte + current_bytes, "".join(current)))
    if not ranges or ranges[-1][1] != total_bytes:
        _reject(NATIVE_TEXT_BASELINE_ARTIFACT_INVALID)
    return tuple(ranges)


def _line_column(text: str, offset: int) -> tuple[int, int, int]:
    encoded = text.encode("utf-8")
    if offset < 0 or offset > len(encoded):
        _reject(NATIVE_TEXT_BASELINE_ARTIFACT_INVALID)
    try:
        prefix = encoded[:offset].decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        _reject_from(NATIVE_TEXT_BASELINE_ARTIFACT_INVALID, error)
    return prefix.count("\n"), len(prefix.rsplit("\n", 1)[-1]), len(prefix)


def _estimated_token_count(content: bytes) -> int:
    return max(1, min(_CHILD_MAX_TOKENS, math.ceil(len(content) / _TOKEN_BYTES)))


def _hash_seed(domain: str, *parts: object) -> str:
    digest = hashlib.sha256(domain.encode("utf-8"))
    for part in parts:
        digest.update(b"\x00")
        digest.update(str(part).encode("utf-8"))
    return digest.hexdigest()


def _hash_sequence(domain: str, values: Sequence[str]) -> str:
    return _hash_seed(domain, *values)


def _hash_json(value: object) -> str:
    return hashlib.sha256(_canonical_json_bytes(value)).hexdigest()


def _canonical_json_bytes(value: object) -> bytes:
    try:
        return json.dumps(
            value,
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    except (TypeError, ValueError, UnicodeEncodeError) as error:
        _reject_from(NATIVE_TEXT_BASELINE_ARTIFACT_INVALID, error)


def _reject(code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(code))


def _reject_from(code: str, cause: Exception) -> NoReturn:
    try:
        _reject(code)
    except PermanentJobError as error:
        raise error from cause
