from __future__ import annotations

import hashlib
import io
import json
import uuid
import zipfile
from pathlib import Path
from typing import cast

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.mineru_gateway import (
    MinerULocalBatchCanonicalMappingInput,
    MinerULocalBatchGateway,
)
from mm_chat_rag.mineru_structure_artifacts import (
    MINERU_STRUCTURE_CHUNK_PROFILE_HASH,
    build_mineru_structure_artifacts,
)
from mm_chat_rag.offline_parser.canonical import JsonObject, JsonValue
from mm_chat_rag.projection import ProjectionContext, build_postgres_projection_batch
from tests.support.parser_contracts import (
    load_packaged_schemas,
    validate_schema_instance,
)

_PDF = b"%PDF-1.7\nstructure fixture\n%%EOF\n"
_MIDDLE = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_corpus"
    / "golden"
    / "mineru_artifact_synthetic"
    / "middle.json"
).read_bytes()


def _context() -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.UUID("10000000-0000-0000-0000-000000000001"),
        stage="parse",
        operation="initial",
        collection_id=uuid.UUID("20000000-0000-0000-0000-000000000001"),
        document_id=uuid.UUID("30000000-0000-0000-0000-000000000001"),
        document_version_id=uuid.UUID("40000000-0000-0000-0000-000000000001"),
        file_id=uuid.UUID("50000000-0000-0000-0000-000000000001"),
        index_generation_id=uuid.UUID("70000000-0000-0000-0000-000000000001"),
        materialization_id=uuid.UUID("60000000-0000-0000-0000-000000000001"),
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=None,
    )


def _mapping(
    middle: bytes,
    *,
    full_markdown: str = "compatibility",
) -> MinerULocalBatchCanonicalMappingInput:
    archive = io.BytesIO()
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_STORED) as output:
        output.writestr("full.md", full_markdown)
        output.writestr(
            "fixture_content_list.json",
            '[{"type":"text","text":"admitted"}]',
        )
        output.writestr("layout.json", middle)
        output.writestr("fixture_model.json", '[[{"type":"text"}]]')
    source = DocumentSource(
        body=_PDF,
        source_sha256=hashlib.sha256(_PDF).hexdigest(),
        content_type="application/pdf",
    )
    gateway = MinerULocalBatchGateway("unit-test-mineru-token")
    artifacts = gateway.extract_result_archive_artifacts(object(), archive.getvalue())
    return gateway.prepare_canonical_mapping_input(object(), source, artifacts)


def _objects(value: JsonValue) -> list[JsonObject]:
    assert isinstance(value, list)
    assert all(isinstance(item, dict) for item in value)
    return cast("list[JsonObject]", value)


def _validate(canonical: JsonObject, chunks: JsonObject) -> None:
    schemas = load_packaged_schemas()
    validate_schema_instance(schemas, "canonical-ir.v2.schema.json", canonical)
    validate_schema_instance(schemas, "chunk-manifest.v2.schema.json", chunks)


def test_synthetic_mineru_structure_maps_blocks_tables_pages_and_projection() -> None:
    mapping = _mapping(_MIDDLE)
    context = _context()

    artifacts = build_mineru_structure_artifacts(context, mapping)
    replay = build_mineru_structure_artifacts(context, mapping)

    assert replay == artifacts
    blocks = _objects(artifacts.canonical_ir["blocks"])
    assert [block["blockType"] for block in blocks] == [
        "heading",
        "paragraph",
        "table",
        "formula",
    ]
    assert artifacts.canonical_ir["textBuffer"]["text"] == (
        "Synthetic heading\nSynthetic MinerU 文本\nkey | value\ncafé | 中文\nE = mc²"
    )
    assert blocks[1]["headingPath"] == [blocks[0]["logicalBlockId"]]
    assert artifacts.chunk_manifest["chunkProfileHash"] == (
        MINERU_STRUCTURE_CHUNK_PROFILE_HASH
    )
    _validate(artifacts.canonical_ir, artifacts.chunk_manifest)

    materialization_id = context.materialization_id
    assert materialization_id is not None
    batch = build_postgres_projection_batch(
        artifacts.canonical_ir,
        artifacts.chunk_manifest,
        ProjectionContext(
            collection_id=context.collection_id,
            document_id=context.document_id,
            document_version_id=context.document_version_id,
            file_id=context.file_id,
            artifact_set_id=artifacts.artifact_set_id,
            materialization_id=materialization_id,
            index_generation_id=context.index_generation_id,
        ),
    )
    assert [block.locator_kind for block in batch.blocks] == ["page_bbox"] * 4
    assert batch.blocks[2].text_content == "key | value\ncafé | 中文"
    assert batch.parent_chunks[0].locator_summary["primary"]["locator"] == {
        "kind": "page_bbox",
        "page": 0,
        "x1": 72000,
        "y1": 72000,
        "x2": 540000,
        "y2": 108000,
    }


def test_long_mineru_text_uses_utf8_safe_planner_overlap() -> None:
    text = "多语言 café alpha beta gamma. " * 900
    middle = json.dumps(
        {
            "pages": [
                {
                    "elements": [
                        {
                            "bboxMilliPoint": [1000, 1000, 500000, 700000],
                            "kind": "text",
                            "text": text,
                        }
                    ],
                    "heightMilliPoint": 792000,
                    "pageIndex": 0,
                    "widthMilliPoint": 612000,
                }
            ]
        },
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode()

    artifacts = build_mineru_structure_artifacts(_context(), _mapping(middle))

    parents = _objects(artifacts.chunk_manifest["parents"])
    children = _objects(artifacts.chunk_manifest["children"])
    assert len(parents) > 1
    assert len(children) > len(parents)
    previous: set[tuple[JsonValue, ...]] = set()
    for child in children:
        current: set[tuple[JsonValue, ...]] = set()
        for fragment in _objects(child["spanFragments"]):
            identity = (
                fragment["blockLogicalId"],
                fragment["blockStartByte"],
                fragment["blockEndByte"],
                fragment["fragmentSourceSpanHash"],
            )
            if fragment["fragmentKind"] == "window_overlap":
                assert identity in previous
            current.add(identity)
        previous = current
    _validate(artifacts.canonical_ir, artifacts.chunk_manifest)
