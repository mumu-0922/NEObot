from __future__ import annotations

import copy
import uuid
from pathlib import Path

import pytest

from mm_chat_rag.projection import (
    PostgresProjectionBatch,
    ProjectionContext,
    ProjectionError,
    build_postgres_projection_batch,
    extract_exact_terms,
    stable_projection_uuid,
)
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
)
from tests.support.parser_contracts import JsonObject, load_strict_json_bytes

_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "hash_dag"
)
_CONTEXT = ProjectionContext(
    collection_id=uuid.UUID("10000000-0000-0000-0000-000000000001"),
    document_id=uuid.UUID("20000000-0000-0000-0000-000000000001"),
    document_version_id=uuid.UUID("30000000-0000-0000-0000-000000000001"),
    file_id=uuid.UUID("40000000-0000-0000-0000-000000000001"),
    artifact_set_id=uuid.UUID("50000000-0000-0000-0000-000000000001"),
    materialization_id=uuid.UUID("60000000-0000-0000-0000-000000000001"),
    index_generation_id=uuid.UUID("70000000-0000-0000-0000-000000000001"),
)


def _fixture(name: str) -> JsonObject:
    value = load_strict_json_bytes((_FIXTURE_ROOT / name).read_bytes())
    assert isinstance(value, dict)
    return value


def _batch() -> PostgresProjectionBatch:
    return build_postgres_projection_batch(
        _fixture("canonical-ir.v2.json"),
        _fixture("chunk-manifest.v2.json"),
        _CONTEXT,
    )


def test_canonical_ir_and_chunk_manifest_project_to_complete_postgres_rows() -> None:
    batch = _batch()

    assert batch.source_sha256 == (
        "3a413cf18e813c868e5859350b4a6e02fe271e2bf4224b92eb14cf3829cb9a9e"
    )
    assert len(batch.blocks) == 2
    assert len(batch.parent_chunks) == 1
    assert len(batch.child_chunks) == 1
    assert len(batch.chunk_block_spans) == 4
    assert len(batch.child_search_projections) == 1

    assert [block.text_content for block in batch.blocks] == ["Hash DAG", "Semantics."]
    assert [span.chunk_kind for span in batch.chunk_block_spans] == [
        "parent",
        "parent",
        "child",
        "child",
    ]

    parent = batch.parent_chunks[0]
    child = batch.child_chunks[0]
    search = batch.child_search_projections[0]
    assert parent.content == "Hash DAG\n\nSemantics."
    assert child.content == parent.content
    assert child.parent_chunk_id == parent.id
    assert search.child_chunk_id == child.id
    assert search.parent_chunk_id == parent.id
    assert search.embedding_model_id == DEFAULT_JINA_EMBEDDING_MODEL
    assert search.embedding_dimensions == DEFAULT_JINA_EMBEDDING_DIMENSIONS
    assert search.exact_terms == ("dag", "hash", "semantics")
    assert search.locator_summary["schemaVersion"] == "g7.4-locator-summary.v1"
    assert search.locator_summary["primary"] == parent.locator_summary["primary"]


def test_projection_ids_are_stable_within_artifact_and_materialization_scope() -> None:
    first = _batch()
    second = _batch()
    assert first.blocks[0].id == second.blocks[0].id
    assert first.parent_chunks[0].id == second.parent_chunks[0].id
    assert first.child_chunks[0].id == second.child_chunks[0].id

    changed_context = ProjectionContext(
        collection_id=_CONTEXT.collection_id,
        document_id=_CONTEXT.document_id,
        document_version_id=_CONTEXT.document_version_id,
        file_id=_CONTEXT.file_id,
        artifact_set_id=_CONTEXT.artifact_set_id,
        materialization_id=uuid.UUID("60000000-0000-0000-0000-000000000002"),
        index_generation_id=_CONTEXT.index_generation_id,
    )
    changed = build_postgres_projection_batch(
        _fixture("canonical-ir.v2.json"),
        _fixture("chunk-manifest.v2.json"),
        changed_context,
    )
    assert changed.blocks[0].id == first.blocks[0].id
    assert changed.parent_chunks[0].id != first.parent_chunks[0].id
    assert changed.child_chunks[0].id != first.child_chunks[0].id


def test_exact_lane_terms_are_deterministic_and_lowercase() -> None:
    assert extract_exact_terms("ERR_AUTH_401 err_auth_401 RAG-Down x 12 123") == (
        "123",
        "err_auth_401",
        "rag-down",
    )


def test_projection_rejects_stale_chunk_content_hash() -> None:
    canonical = _fixture("canonical-ir.v2.json")
    chunks = _fixture("chunk-manifest.v2.json")
    mutated = copy.deepcopy(chunks)
    assert isinstance(mutated, dict)
    parents = mutated["parents"]
    assert isinstance(parents, list)
    parent = parents[0]
    assert isinstance(parent, dict)
    parent["contentHash"] = "0" * 64

    with pytest.raises(ProjectionError, match="content hash"):
        build_postgres_projection_batch(canonical, mutated, _CONTEXT)


def test_projection_rejects_missing_child_parent() -> None:
    canonical = _fixture("canonical-ir.v2.json")
    chunks = _fixture("chunk-manifest.v2.json")
    mutated = copy.deepcopy(chunks)
    assert isinstance(mutated, dict)
    children = mutated["children"]
    assert isinstance(children, list)
    child = children[0]
    assert isinstance(child, dict)
    child["logicalParentChunkId"] = "1" * 64

    with pytest.raises(ProjectionError, match="missing parent"):
        build_postgres_projection_batch(canonical, mutated, _CONTEXT)


def test_projection_rejects_source_mismatch_before_staging_rows() -> None:
    canonical = _fixture("canonical-ir.v2.json")
    chunks = _fixture("chunk-manifest.v2.json")
    mutated = copy.deepcopy(chunks)
    assert isinstance(mutated, dict)
    mutated["sourceSha256"] = "2" * 64

    with pytest.raises(ProjectionError, match="sourceSha256"):
        build_postgres_projection_batch(canonical, mutated, _CONTEXT)


def test_projection_uuid_rejects_non_hash_inputs() -> None:
    with pytest.raises(ProjectionError, match="sha256"):
        stable_projection_uuid(_CONTEXT.artifact_set_id, "block", "not-a-hash")
