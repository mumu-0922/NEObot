"""Dependency-injected handler seams for G7.5 worker promotion.

This module wires the first real parse execution path without registering it in
production. Gateways are explicit Protocols so storage, provider, and Postgres
projection implementations can be promoted one at a time behind tests. The
default dependency bundle is inert and fails closed before any external I/O.
"""

from __future__ import annotations

import re
import uuid
from collections.abc import Coroutine
from dataclasses import dataclass
from typing import Any, Final, NoReturn, Protocol

from mm_chat_rag.handlers import JobHandler, JobResult, with_job_context_admission
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handlers import require_parse_context
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.offline_parser.canonical import JsonObject
from mm_chat_rag.projection import (
    PostgresProjectionBatch,
    ProjectionContext,
    ProjectionError,
    build_postgres_projection_batch,
)
from mm_chat_rag.provider_profile import ProviderRuntimeProfile
from mm_chat_rag.retry import PermanentJobError

JOB_HANDLER_DEPENDENCY_UNCONFIGURED: Final = (
    "JOB_HANDLER_DEPENDENCY_UNCONFIGURED"
)
JOB_HANDLER_SOURCE_INVALID: Final = "JOB_HANDLER_SOURCE_INVALID"
JOB_HANDLER_PARSE_ARTIFACT_INVALID: Final = "JOB_HANDLER_PARSE_ARTIFACT_INVALID"
JOB_HANDLER_SOURCE_HASH_MISMATCH: Final = "JOB_HANDLER_SOURCE_HASH_MISMATCH"
JOB_HANDLER_DEPENDENCY_ERROR_CODES: Final[frozenset[str]] = frozenset(
    {
        JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
        JOB_HANDLER_SOURCE_INVALID,
        JOB_HANDLER_PARSE_ARTIFACT_INVALID,
        JOB_HANDLER_SOURCE_HASH_MISMATCH,
    }
)

_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_CONTENT_TYPE_RE: Final = re.compile(r"^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9.+-]{1,64}$")
_ZERO_UUID: Final = uuid.UUID(int=0)


@dataclass(frozen=True, slots=True)
class DocumentSource:
    """Opaque object-storage payload plus the expected parser source hash."""

    body: bytes
    source_sha256: str
    content_type: str

    def __post_init__(self) -> None:
        if not isinstance(self.body, bytes) or not self.body:
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if not _SHA256_RE.fullmatch(self.source_sha256):
            _reject(JOB_HANDLER_SOURCE_INVALID)
        if not _CONTENT_TYPE_RE.fullmatch(self.content_type):
            _reject(JOB_HANDLER_SOURCE_INVALID)


@dataclass(frozen=True, slots=True)
class ParsedDocumentArtifacts:
    """Provider parser output needed before deterministic Postgres projection."""

    artifact_set_id: uuid.UUID
    canonical_ir: JsonObject
    chunk_manifest: JsonObject

    def __post_init__(self) -> None:
        if self.artifact_set_id == _ZERO_UUID:
            _reject(JOB_HANDLER_PARSE_ARTIFACT_INVALID)
        if not isinstance(self.canonical_ir, dict) or not isinstance(
            self.chunk_manifest, dict
        ):
            _reject(JOB_HANDLER_PARSE_ARTIFACT_INVALID)


class DocumentSourceGateway(Protocol):
    """Storage gateway that returns document bytes for one admitted parse job."""

    def fetch_document_source(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, DocumentSource]: ...


class ParserGateway(Protocol):
    """Provider parser gateway for MinerU-compatible Canonical IR artifacts."""

    def parse_document(
        self, context: ProcessingJobContext, source: DocumentSource
    ) -> Coroutine[Any, Any, ParsedDocumentArtifacts]: ...


class ParseProjectionGateway(Protocol):
    """Postgres projection writer used after parser artifacts validate."""

    def stage_parse_projection(
        self, context: ProcessingJobContext, batch: PostgresProjectionBatch
    ) -> Coroutine[Any, Any, None]: ...


@dataclass(frozen=True, slots=True)
class ParseHandlerDependencies:
    """Explicit parse-side dependencies; absent dependencies fail closed."""

    document_source: DocumentSourceGateway | None = None
    parser: ParserGateway | None = None
    projection: ParseProjectionGateway | None = None

    def require_ready(
        self,
    ) -> tuple[DocumentSourceGateway, ParserGateway, ParseProjectionGateway]:
        """Return configured gateways or fail before any external side effect."""
        if (
            self.document_source is None
            or self.parser is None
            or self.projection is None
        ):
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        return self.document_source, self.parser, self.projection


async def parse_handler_with_dependencies(
    context: ProcessingJobContext,
    dependencies: ParseHandlerDependencies,
) -> JobResult:
    """Execute the admitted parse seam through storage, parser, and projection."""
    admitted = require_parse_context(context)
    materialization_id = admitted.materialization_id
    if materialization_id is None:  # pragma: no cover - require_parse_context fence
        _reject(JOB_HANDLER_PARSE_ARTIFACT_INVALID)
    source_gateway, parser_gateway, projection_gateway = dependencies.require_ready()

    source = await source_gateway.fetch_document_source(admitted)
    parsed = await parser_gateway.parse_document(admitted, source)
    try:
        batch = build_postgres_projection_batch(
            parsed.canonical_ir,
            parsed.chunk_manifest,
            ProjectionContext(
                collection_id=admitted.collection_id,
                document_id=admitted.document_id,
                document_version_id=admitted.document_version_id,
                file_id=admitted.file_id,
                artifact_set_id=parsed.artifact_set_id,
                materialization_id=materialization_id,
                index_generation_id=admitted.index_generation_id,
            ),
        )
    except ProjectionError as error:
        _reject_from(JOB_HANDLER_PARSE_ARTIFACT_INVALID, error)
    if batch.source_sha256 != source.source_sha256:
        _reject(JOB_HANDLER_SOURCE_HASH_MISMATCH)
    await projection_gateway.stage_parse_projection(admitted, batch)
    return JobResult()


def admitted_parse_handler_with_dependencies(
    dependencies: ParseHandlerDependencies,
    provider_profile: ProviderRuntimeProfile,
) -> JobHandler:
    """Build a claim-level parse handler through admission and dependencies."""

    async def contextual(context: ProcessingJobContext) -> JobResult:
        return await parse_handler_with_dependencies(context, dependencies)

    return with_job_context_admission(
        contextual,
        provider_profile=provider_profile,
    )


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_from(error_code: str, cause: Exception) -> NoReturn:
    try:
        _reject(error_code)
    except PermanentJobError as error:
        raise error from cause
