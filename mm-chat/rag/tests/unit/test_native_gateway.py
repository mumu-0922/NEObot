"""Non-PDF production parser routing and projection tests."""

from __future__ import annotations

import hashlib
import uuid
from dataclasses import replace
from pathlib import Path
from typing import cast

import pytest

from mm_chat_rag.job_context import ProcessingJobContext, ProviderAuthority
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.mineru_gateway import MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH
from mm_chat_rag.native_gateway import (
    AuthorityRoutingParserGateway,
    NativeSandboxParserGateway,
    NativeStructureSandboxParserGateway,
)
from mm_chat_rag.offline_parser.native.dispatch import parse_native_source
from mm_chat_rag.offline_parser.sandbox import SandboxRouteResult
from mm_chat_rag.projection import ProjectionContext, build_postgres_projection_batch
from mm_chat_rag.provider_profile import GenerationEmbeddingProfile
from mm_chat_rag.structure_chunking import (
    SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
    STRUCTURE_CHUNK_PROFILE_HASH,
)

_DOCX_MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
_DOCX = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_corpus"
    / "golden"
    / "docx"
    / "minimal.docx"
).read_bytes()


def _context(
    *,
    processor: str = "native",
    model_id: str = "native-parser-v1",
) -> ProcessingJobContext:
    return ProcessingJobContext(
        job_id=uuid.uuid4(),
        stage="parse",
        operation="initial",
        collection_id=uuid.uuid4(),
        document_id=uuid.uuid4(),
        document_version_id=uuid.uuid4(),
        file_id=uuid.uuid4(),
        index_generation_id=uuid.uuid4(),
        materialization_id=uuid.uuid4(),
        collection_acl_revision=1,
        collection_visibility_epoch=1,
        collection_processing_revision=1,
        document_visibility_epoch=1,
        attempt_count=1,
        max_attempts=3,
        request_hash="a" * 64,
        authority=ProviderAuthority(
            processor=processor,
            endpoint_id="local" if processor == "native" else "hosted-main",
            model_id=model_id,
            governance_profile_id=uuid.uuid4(),
            governance_revision=1,
            governance_head_revision=1,
            collection_consent_id=uuid.uuid4(),
            collection_consent_revision=1,
        ),
    )


class _StaticSupervisor:
    def route(self, source: bytes, **_: object) -> SandboxRouteResult:
        outcome = parse_native_source(source, declared_mime=_DOCX_MIME)
        assert outcome.artifact is not None
        assert outcome.parser_format is not None
        return SandboxRouteResult(
            parser_format=outcome.parser_format,
            stable_error_code=None,
            native_artifact=outcome.artifact.canonical_bytes,
        )


@pytest.mark.asyncio
async def test_native_docx_gateway_builds_projection_ready_text_baseline() -> None:
    context = _context()
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )
    gateway = NativeSandboxParserGateway(_StaticSupervisor())

    artifacts = await gateway.parse_document(context, source)
    assert artifacts.canonical_ir["source"] == {
        "bytes": len(_DOCX),
        "format": "docx",
        "sha256": source.source_sha256,
    }
    assert artifacts.canonical_ir["textBuffer"]["text"] == (
        "Minimal DOCX\nUnicode: 文档 café\nCell"
    )
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
    assert batch.parent_chunks[0].content == "Minimal DOCX\nUnicode: 文档 café\nCell"


@pytest.mark.asyncio
async def test_native_docx_structure_gateway_preserves_multiple_blocks() -> None:
    context = _context()
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    artifacts = await NativeStructureSandboxParserGateway(
        _StaticSupervisor()
    ).parse_document(context, source)

    assert artifacts.chunk_manifest["chunkProfileHash"] == (
        STRUCTURE_CHUNK_PROFILE_HASH
    )
    assert len(artifacts.canonical_ir["blocks"]) == 3


@pytest.mark.asyncio
async def test_native_structure_artifact_uses_bge_generation_profile_hash() -> None:
    context = replace(
        _context(),
        generation_embedding_profile=GenerationEmbeddingProfile(
            processor="siliconflow",
            model_id="Pro/BAAI/bge-m3",
            dimensions=1024,
        ),
        generation_chunk_profile_hash=SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
    )
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    artifacts = await NativeStructureSandboxParserGateway(
        _StaticSupervisor()
    ).parse_document(context, source)

    assert artifacts.chunk_manifest["chunkProfileHash"] == (
        SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
    )


class _FakeParser:
    def __init__(self, marker: str) -> None:
        self.marker = marker
        self.calls = 0
        self.context: ProcessingJobContext | None = None

    async def parse_document(
        self, context: ProcessingJobContext, source: DocumentSource
    ) -> ParsedDocumentArtifacts:
        _ = source
        self.context = context
        self.calls += 1
        return cast("ParsedDocumentArtifacts", self.marker)


class _FakeProfiles:
    def __init__(self, chunk_profile_hash: str) -> None:
        self.chunk_profile_hash = chunk_profile_hash
        self.calls = 0

    async def resolve_parse_chunk_profile(self, context: ProcessingJobContext) -> str:
        _ = context
        self.calls += 1
        return self.chunk_profile_hash


@pytest.mark.asyncio
async def test_authority_router_uses_pinned_processor_not_mime_guessing() -> None:
    mineru = _FakeParser("mineru")
    native = _FakeParser("native")
    structure_mineru = _FakeParser("structure-mineru")
    structure_native = _FakeParser("structure-native")
    profiles = _FakeProfiles(MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH)
    gateway = AuthorityRoutingParserGateway(
        profiles=profiles,
        mineru=mineru,
        native=native,
        structure_mineru=structure_mineru,
        structure_native=structure_native,
    )
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    observed = await gateway.parse_document(_context(), source)

    assert observed == "native"
    assert native.calls == 1
    assert mineru.calls == 0
    assert structure_native.calls == 0
    assert profiles.calls == 1


@pytest.mark.asyncio
async def test_authority_router_uses_structure_gateway_only_for_shared_hash() -> None:
    mineru = _FakeParser("mineru")
    native = _FakeParser("native")
    structure_mineru = _FakeParser("structure-mineru")
    structure_native = _FakeParser("structure-native")
    gateway = AuthorityRoutingParserGateway(
        profiles=_FakeProfiles(STRUCTURE_CHUNK_PROFILE_HASH),
        mineru=mineru,
        native=native,
        structure_mineru=structure_mineru,
        structure_native=structure_native,
    )
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    observed = await gateway.parse_document(_context(), source)

    assert observed == "structure-native"
    assert structure_native.calls == 1
    assert native.calls == 0


@pytest.mark.asyncio
async def test_authority_router_keeps_bge_chunk_and_embedding_profiles_together() -> (
    None
):
    structure_native = _FakeParser("structure-native")
    gateway = AuthorityRoutingParserGateway(
        profiles=_FakeProfiles(SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH),
        mineru=_FakeParser("mineru"),
        native=_FakeParser("native"),
        structure_mineru=_FakeParser("structure-mineru"),
        structure_native=structure_native,
    )
    context = replace(
        _context(),
        generation_embedding_profile=GenerationEmbeddingProfile(
            processor="siliconflow",
            model_id="Pro/BAAI/bge-m3",
            dimensions=1024,
        ),
    )
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    assert await gateway.parse_document(context, source) == "structure-native"
    assert structure_native.context is not None
    assert structure_native.context.generation_chunk_profile_hash == (
        SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
    )
