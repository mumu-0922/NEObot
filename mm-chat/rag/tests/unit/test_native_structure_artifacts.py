from __future__ import annotations

import hashlib
import uuid
from pathlib import Path
from typing import cast

from mm_chat_rag.job_context import ProcessingJobContext, ProviderAuthority
from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.native_structure_artifacts import (
    NATIVE_STRUCTURE_CHUNK_PROFILE_HASH,
    build_native_structure_artifacts,
)
from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.docx import parse_docx
from mm_chat_rag.offline_parser.native.markdown import parse_markdown
from mm_chat_rag.offline_parser.native.model import NativeDocument
from mm_chat_rag.offline_parser.native.opc import admit_ooxml_package
from mm_chat_rag.offline_parser.native.txt import parse_txt
from mm_chat_rag.projection import ProjectionContext, build_postgres_projection_batch
from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    load_packaged_schemas,
    validate_schema_instance,
)

_CORPUS = Path(__file__).parents[1] / "fixtures" / "parser_corpus" / "golden"
_DOCX_MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
_MATERIALIZATION_ID = uuid.UUID("60000000-0000-0000-0000-000000000001")


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
        materialization_id=_MATERIALIZATION_ID,
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=ProviderAuthority(
            processor="native",
            endpoint_id="local",
            model_id="native-parser-v1",
            governance_profile_id=uuid.UUID("80000000-0000-0000-0000-000000000001"),
            governance_revision=1,
            governance_head_revision=1,
            collection_consent_id=uuid.UUID("90000000-0000-0000-0000-000000000001"),
            collection_consent_revision=1,
        ),
    )


def _source(body: bytes, content_type: str) -> DocumentSource:
    return DocumentSource(
        body=body,
        source_sha256=hashlib.sha256(body).hexdigest(),
        content_type=content_type,
    )


def _markdown_document(body: bytes) -> NativeDocument:
    return parse_markdown(decode_source(body), DEFAULT_CONFIG.native)


def _build(
    body: bytes, artifact: NativeDocument, content_type: str
) -> tuple[JsonObject, JsonObject, ProjectionContext]:
    context = _context()
    artifacts = build_native_structure_artifacts(
        context,
        _source(body, content_type),
        artifact,
        parser_model="native-parser-v1",
    )
    projection_context = ProjectionContext(
        collection_id=context.collection_id,
        document_id=context.document_id,
        document_version_id=context.document_version_id,
        file_id=context.file_id,
        artifact_set_id=artifacts.artifact_set_id,
        materialization_id=_MATERIALIZATION_ID,
        index_generation_id=context.index_generation_id,
    )
    return artifacts.canonical_ir, artifacts.chunk_manifest, projection_context


def _objects(value: JsonValue) -> list[JsonObject]:
    assert isinstance(value, list)
    assert all(isinstance(item, dict) for item in value)
    return cast("list[JsonObject]", value)


def _validate_contracts(canonical: JsonObject, chunks: JsonObject) -> None:
    schemas = load_packaged_schemas()
    validate_schema_instance(schemas, "canonical-ir.v2.schema.json", canonical)
    validate_schema_instance(schemas, "chunk-manifest.v2.schema.json", chunks)


def test_minimal_docx_maps_heading_paragraph_and_table_row_without_loss() -> None:
    body = (_CORPUS / "docx" / "minimal.docx").read_bytes()
    artifact = parse_docx(
        admit_ooxml_package(body, DEFAULT_CONFIG.native),
        DEFAULT_CONFIG.native,
    )

    canonical, chunks, projection_context = _build(body, artifact, _DOCX_MIME)
    replay_canonical, replay_chunks, _ = _build(body, artifact, _DOCX_MIME)

    blocks = _objects(canonical["blocks"])
    assert (replay_canonical, replay_chunks) == (canonical, chunks)
    assert [block["blockType"] for block in blocks] == [
        "heading",
        "paragraph",
        "table",
    ]
    assert canonical["textBuffer"]["text"] == ("Minimal DOCX\nUnicode: 文档 café\nCell")
    assert blocks[1]["headingPath"] == [blocks[0]["logicalBlockId"]]
    assert blocks[2]["structureRef"]["structureKind"] == "table_row"
    assert chunks["chunkProfileHash"] == NATIVE_STRUCTURE_CHUNK_PROFILE_HASH
    _validate_contracts(canonical, chunks)

    batch = build_postgres_projection_batch(canonical, chunks, projection_context)
    assert [row.text_content for row in batch.blocks] == [
        "Minimal DOCX",
        "Unicode: 文档 café",
        "Cell",
    ]
    assert len(batch.parent_chunks) == chunks["parentCount"]
    assert len(batch.child_chunks) == chunks["childCount"]
    assert batch.parent_chunks[0].locator_summary["fragments"]
    assert batch.child_search_projections[0].content_hash == (
        batch.child_chunks[0].content_hash
    )


def test_markdown_maps_heading_lists_table_rows_and_projection_heading_path() -> None:
    body = (_CORPUS / "markdown" / "representative.md").read_bytes()
    artifact = _markdown_document(body)

    canonical, chunks, projection_context = _build(
        body,
        artifact,
        "text/markdown",
    )

    blocks = _objects(canonical["blocks"])
    block_types = [block["blockType"] for block in blocks]
    assert block_types == [
        "heading",
        "list_item",
        "list_item",
        "table",
        "table",
        "code",
        "paragraph",
    ]
    text = cast("str", canonical["textBuffer"]["text"])
    assert "- first item" in text
    assert "key | value" in text
    assert "café | 中文" in text
    assert text.count('<span data-kind="raw">raw HTML</span>') == 1
    heading_id = blocks[0]["logicalBlockId"]
    assert all(block["headingPath"] == [heading_id] for block in blocks[1:])
    _validate_contracts(canonical, chunks)

    batch = build_postgres_projection_batch(canonical, chunks, projection_context)
    assert batch.blocks[1].heading_path == (heading_id,)
    assert batch.parent_chunks[0].content.startswith("# Corpus heading")
    assert "café | 中文" in batch.child_search_projections[0].lexical_text


def test_long_multilingual_markdown_preserves_utf8_ranges_and_exact_overlap() -> None:
    paragraph = "多语言 café alpha beta gamma delta. " * 900
    body = f"# 长文\n\n{paragraph}\n".encode()
    artifact = _markdown_document(body)

    canonical, chunks, projection_context = _build(
        body,
        artifact,
        "text/markdown",
    )

    parents = _objects(chunks["parents"])
    children = _objects(chunks["children"])
    assert len(parents) > 1
    assert len(children) > len(parents)
    assert any(
        fragment["fragmentKind"] == "window_overlap"
        for child in children
        for fragment in _objects(child["spanFragments"])
    )
    block_text = {
        cast("str", block["logicalBlockId"]): _block_text(canonical, block)
        for block in _objects(canonical["blocks"])
    }
    previous_fragments: set[tuple[str, int, int, str]] = set()
    for child in children:
        current: set[tuple[str, int, int, str]] = set()
        for fragment in _objects(child["spanFragments"]):
            block_id = cast("str", fragment["blockLogicalId"])
            start = cast("int", fragment["blockStartByte"])
            end = cast("int", fragment["blockEndByte"])
            block_text[block_id].encode()[start:end].decode("utf-8")
            identity = (
                block_id,
                start,
                end,
                cast("str", fragment["fragmentSourceSpanHash"]),
            )
            if fragment["fragmentKind"] == "window_overlap":
                assert identity in previous_fragments
                assert fragment["previousChildOrdinal"] == child["childOrdinal"] - 1
            current.add(identity)
        previous_fragments = current
    _validate_contracts(canonical, chunks)

    batch = build_postgres_projection_batch(canonical, chunks, projection_context)
    assert len(batch.parent_chunks) == len(parents)
    assert len(batch.child_chunks) == len(children)
    assert all(row.locator_summary["primary"] for row in batch.parent_chunks)
    assert [row.ordinal for row in batch.child_chunks] == list(range(len(children)))
    assert all(
        row.lexical_text == batch.child_chunks[index].content
        for index, row in enumerate(batch.child_search_projections)
    )


def test_identity_locator_clipping_preserves_crlf_and_cr_line_positions() -> None:
    body = "alpha\r\n中文\rfinal".encode()
    artifact = parse_txt(decode_source(body))

    canonical, chunks, projection_context = _build(body, artifact, "text/plain")

    block = _objects(canonical["blocks"])[0]
    locator = cast("JsonObject", block["locatorSet"])
    anchors = _objects(locator["textAnchors"])
    views = _objects(_objects(anchors[0]["sourceFragments"])[0]["views"])
    source_position = views[0]
    assert source_position["kind"] == "source_text_position"
    assert source_position["startLine"] == 0
    assert source_position["startColumn"] == 0
    assert source_position["endLine"] == 2
    assert source_position["endColumn"] == 5
    assert source_position["rawByteEnd"] == len(body)
    _validate_contracts(canonical, chunks)
    batch = build_postgres_projection_batch(canonical, chunks, projection_context)
    assert batch.blocks[0].locator == {
        "kind": "line_range",
        "startLine": 0,
        "endLine": 2,
    }


def _block_text(canonical: JsonObject, block: JsonObject) -> str:
    text_buffer = cast("JsonObject", canonical["textBuffer"])
    text = cast("str", text_buffer["text"])
    text_range = cast("JsonObject", block["textRange"])
    start = cast("int", text_range["startByte"])
    end = cast("int", text_range["endByte"])
    return text.encode()[start:end].decode("utf-8")
