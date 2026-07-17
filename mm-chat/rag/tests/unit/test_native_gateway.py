"""Non-PDF production parser routing and projection tests."""

from __future__ import annotations

import hashlib
import uuid
from pathlib import Path
from typing import cast

import pytest

from mm_chat_rag.job_context import ProcessingJobContext, ProviderAuthority
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.native_gateway import (
    AuthorityRoutingParserGateway,
    NativeSandboxParserGateway,
)
from mm_chat_rag.offline_parser.native.dispatch import parse_native_source
from mm_chat_rag.offline_parser.sandbox import SandboxRouteResult
from mm_chat_rag.projection import ProjectionContext, build_postgres_projection_batch

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


class _FakeParser:
    def __init__(self, marker: str) -> None:
        self.marker = marker
        self.calls = 0

    async def parse_document(
        self, context: ProcessingJobContext, source: DocumentSource
    ) -> ParsedDocumentArtifacts:
        _ = context, source
        self.calls += 1
        return cast("ParsedDocumentArtifacts", self.marker)


@pytest.mark.asyncio
async def test_authority_router_uses_pinned_processor_not_mime_guessing() -> None:
    mineru = _FakeParser("mineru")
    native = _FakeParser("native")
    gateway = AuthorityRoutingParserGateway(mineru=mineru, native=native)
    source = DocumentSource(
        body=_DOCX,
        source_sha256=hashlib.sha256(_DOCX).hexdigest(),
        content_type=_DOCX_MIME,
    )

    observed = await gateway.parse_document(_context(), source)

    assert observed == "native"
    assert native.calls == 1
    assert mineru.calls == 0
