"""Sandboxed local parser gateway for non-PDF knowledge documents."""

from __future__ import annotations

import hashlib
from collections.abc import Coroutine
from dataclasses import replace
from typing import Any, Final, NoReturn, Protocol

from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource, ParsedDocumentArtifacts
from mm_chat_rag.mineru_gateway import MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.native_structure_artifacts import (
    build_native_structure_artifacts,
    native_structure_planner_units,
)
from mm_chat_rag.native_text_baseline import build_native_text_baseline_artifacts
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.native.model import (
    NativeArtifactError,
    NativeDocument,
    NativeFragmentRole,
)
from mm_chat_rag.offline_parser.sandbox import SandboxRouteResult, SandboxSupervisor
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.semantic_chunking import SemanticBoundaryPlanner
from mm_chat_rag.semantic_profile import semantic_boundary_profile_hash
from mm_chat_rag.structure_chunking import (
    SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
    STRUCTURE_CHUNK_PROFILE_HASH,
    SUPPORTED_STRUCTURE_CHUNK_PROFILE_HASHES,
    SemanticBoundaryHints,
    StructureChunkingError,
    structure_semantic_profile_hash,
)

NATIVE_PARSER_PROCESSOR: Final = "native"
NATIVE_PARSER_MODEL: Final = "native-parser-v1"
NATIVE_PARSER_CONTEXT_INVALID: Final = "NATIVE_PARSER_CONTEXT_INVALID"
NATIVE_PARSER_SOURCE_HASH_MISMATCH: Final = "NATIVE_PARSER_SOURCE_HASH_MISMATCH"
NATIVE_PARSER_ARTIFACT_INVALID: Final = "NATIVE_PARSER_ARTIFACT_INVALID"
NATIVE_PARSER_AUTHORITY_UNSUPPORTED: Final = "NATIVE_PARSER_AUTHORITY_UNSUPPORTED"
NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED: Final = (
    "NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED"
)


class _ParserGateway(Protocol):
    async def parse_document(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> ParsedDocumentArtifacts: ...


class _SandboxGateway(Protocol):
    def route(
        self,
        source: bytes,
        *,
        declared_mime: str | None = None,
    ) -> SandboxRouteResult: ...


class ParseChunkProfileGateway(Protocol):
    """Resolve the generation profile through the current parse-job lease."""

    def resolve_parse_chunk_profile(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, str]: ...


class AuthorityRoutingParserGateway:
    """Route one admitted parse job by its server-pinned processor authority."""

    def __init__(
        self,
        *,
        profiles: ParseChunkProfileGateway,
        mineru: _ParserGateway,
        native: _ParserGateway,
        structure_mineru: _ParserGateway,
        structure_native: _ParserGateway,
    ) -> None:
        self._profiles = profiles
        self._mineru = mineru
        self._native = native
        self._structure_mineru = structure_mineru
        self._structure_native = structure_native

    async def parse_document(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> ParsedDocumentArtifacts:
        authority = context.authority
        if authority is None:
            _reject(NATIVE_PARSER_CONTEXT_INVALID)
        chunk_profile_hash = await self._profiles.resolve_parse_chunk_profile(context)
        if chunk_profile_hash in SUPPORTED_STRUCTURE_CHUNK_PROFILE_HASHES:
            _validate_structure_embedding_profile(context, chunk_profile_hash)
            context = replace(
                context,
                generation_chunk_profile_hash=chunk_profile_hash,
            )
            parser = _authority_parser(
                authority.processor,
                mineru=self._structure_mineru,
                native=self._structure_native,
            )
        elif chunk_profile_hash == MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH:
            parser = _authority_parser(
                authority.processor,
                mineru=self._mineru,
                native=self._native,
            )
        else:
            _reject(NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED)
        return await parser.parse_document(context, source)


def _validate_structure_embedding_profile(
    context: ProcessingJobContext,
    chunk_profile_hash: str,
) -> None:
    embedding_profile = context.generation_embedding_profile
    if embedding_profile is None:
        if chunk_profile_hash == STRUCTURE_CHUNK_PROFILE_HASH:
            return
        _reject(NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED)
    try:
        observed = semantic_boundary_profile_hash(embedding_profile)
        expected = structure_semantic_profile_hash(chunk_profile_hash)
    except (StructureChunkingError, ValueError):
        _reject(NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED)
    if observed != expected:
        _reject(NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED)
    if (
        chunk_profile_hash == SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
        and embedding_profile.processor != "siliconflow"
    ):
        _reject(NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED)


class NativeSandboxParserGateway:
    """Parse untrusted native formats in the existing one-exec seccomp sandbox."""

    def __init__(self, supervisor: _SandboxGateway | None = None) -> None:
        self._supervisor = supervisor or SandboxSupervisor()

    async def parse_document(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> ParsedDocumentArtifacts:
        admitted, artifact = _parse_native_document(context, source, self._supervisor)
        text = _native_text(artifact)
        return build_native_text_baseline_artifacts(
            admitted,
            source,
            artifact,
            text,
            parser_model=NATIVE_PARSER_MODEL,
        )


class NativeStructureSandboxParserGateway:
    """Parse Native documents into the shared structure chunk profile."""

    def __init__(
        self,
        supervisor: _SandboxGateway | None = None,
        *,
        semantic_planner: SemanticBoundaryPlanner | None = None,
    ) -> None:
        self._supervisor = supervisor or SandboxSupervisor()
        self._semantic_planner = semantic_planner

    async def parse_document(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> ParsedDocumentArtifacts:
        admitted, artifact = _parse_native_document(context, source, self._supervisor)
        semantic_hints: tuple[SemanticBoundaryHints, ...] = ()
        if self._semantic_planner is not None:
            semantic_hints = await self._semantic_planner.plan(
                admitted,
                native_structure_planner_units(artifact),
            )
        return build_native_structure_artifacts(
            admitted,
            source,
            artifact,
            parser_model=NATIVE_PARSER_MODEL,
            semantic_hints=semantic_hints,
        )


def _parse_native_document(
    context: ProcessingJobContext,
    source: DocumentSource,
    supervisor: _SandboxGateway,
) -> tuple[ProcessingJobContext, NativeDocument]:
    admitted = _validate_context(context)
    _validate_source_hash(source)
    result = supervisor.route(
        source.body,
        declared_mime=source.content_type,
    )
    return admitted, _admit_sandbox_result(result, source)


def _authority_parser(
    processor: str,
    *,
    mineru: _ParserGateway,
    native: _ParserGateway,
) -> _ParserGateway:
    if processor == "mineru":
        return mineru
    if processor == NATIVE_PARSER_PROCESSOR:
        return native
    return _reject(NATIVE_PARSER_AUTHORITY_UNSUPPORTED)


def _validate_context(context: object) -> ProcessingJobContext:
    if not isinstance(context, ProcessingJobContext):
        _reject(NATIVE_PARSER_CONTEXT_INVALID)
    if context.stage != "parse" or context.materialization_id is None:
        _reject(NATIVE_PARSER_CONTEXT_INVALID)
    authority = context.authority
    if (
        authority is None
        or authority.processor != NATIVE_PARSER_PROCESSOR
        or authority.model_id != NATIVE_PARSER_MODEL
    ):
        _reject(NATIVE_PARSER_CONTEXT_INVALID)
    return context


def _validate_source_hash(source: DocumentSource) -> None:
    if hashlib.sha256(source.body).hexdigest() != source.source_sha256:
        _reject(NATIVE_PARSER_SOURCE_HASH_MISMATCH)


def _admit_sandbox_result(
    result: SandboxRouteResult,
    source: DocumentSource,
) -> NativeDocument:
    if result.requires_restart:
        _reject(StableErrorCode.PARSER_SANDBOX_UNAVAILABLE.value)
    if result.stable_error_code is not None:
        _reject(result.stable_error_code.value)
    if not result.native_ready:
        _reject(NATIVE_PARSER_ARTIFACT_INVALID)
    parser_format = result.parser_format
    if parser_format is None:
        _reject(NATIVE_PARSER_ARTIFACT_INVALID)
    try:
        artifact = NativeDocument.from_bytes(result.native_artifact)
        artifact.validate_source_binding(
            source.body,
            expected_format=parser_format,
        )
    except (NativeArtifactError, ValueError) as error:
        _reject_from(NATIVE_PARSER_ARTIFACT_INVALID, error)
    return artifact


def _native_text(artifact: NativeDocument) -> str:
    lines: list[str] = []
    for node in artifact.nodes[1:]:
        line = "".join(
            fragment.text
            for fragment in node.fragments
            if fragment.role is not NativeFragmentRole.EXTERNAL_TARGET
        )
        if line.strip():
            lines.append(line)
    text = "\n".join(lines)
    if not text:
        _reject(NATIVE_PARSER_ARTIFACT_INVALID)
    return text


def _reject(code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(code))


def _reject_from(code: str, cause: Exception) -> NoReturn:
    try:
        _reject(code)
    except PermanentJobError as error:
        raise error from cause
