"""Dependency-injected handler seams for G7.5 worker promotion.

This module wires real handler-shaped seams without registering them in
production. Gateways are explicit Protocols so storage, provider, and Postgres
projection implementations can be promoted one at a time behind tests. The
default dependency bundles are inert and fail closed before any external I/O.
"""

from __future__ import annotations

import hashlib
import math
import re
import struct
import uuid
from collections.abc import Coroutine
from dataclasses import dataclass
from typing import Any, Final, NoReturn, Protocol

from mm_chat_rag.handlers import JobHandler, JobResult, with_job_context_admission
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handlers import (
    require_parse_context,
    require_passage_embedding_context,
    require_purge_context,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.offline_parser.canonical import JsonObject
from mm_chat_rag.projection import (
    PostgresProjectionBatch,
    ProjectionContext,
    ProjectionError,
    build_postgres_projection_batch,
)
from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError

JOB_HANDLER_DEPENDENCY_UNCONFIGURED: Final = (
    "JOB_HANDLER_DEPENDENCY_UNCONFIGURED"
)
JOB_HANDLER_SOURCE_INVALID: Final = "JOB_HANDLER_SOURCE_INVALID"
JOB_HANDLER_PARSE_ARTIFACT_INVALID: Final = "JOB_HANDLER_PARSE_ARTIFACT_INVALID"
JOB_HANDLER_PARSE_COMPLETION_FAILED: Final = "JOB_HANDLER_PARSE_COMPLETION_FAILED"
JOB_HANDLER_SOURCE_HASH_MISMATCH: Final = "JOB_HANDLER_SOURCE_HASH_MISMATCH"
JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID: Final = (
    "JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID"
)
JOB_HANDLER_EMBEDDING_VECTOR_INVALID: Final = "JOB_HANDLER_EMBEDDING_VECTOR_INVALID"
JOB_HANDLER_EMBEDDING_COUNT_MISMATCH: Final = (
    "JOB_HANDLER_EMBEDDING_COUNT_MISMATCH"
)
JOB_HANDLER_EMBEDDING_CHILD_MISMATCH: Final = (
    "JOB_HANDLER_EMBEDDING_CHILD_MISMATCH"
)
JOB_HANDLER_EMBEDDING_COMPLETENESS_FAILED: Final = (
    "JOB_HANDLER_EMBEDDING_COMPLETENESS_FAILED"
)
JOB_HANDLER_EMBEDDING_COMPLETION_FAILED: Final = (
    "JOB_HANDLER_EMBEDDING_COMPLETION_FAILED"
)
JOB_HANDLER_PURGE_VISIBILITY_INVALID: Final = "JOB_HANDLER_PURGE_VISIBILITY_INVALID"
JOB_HANDLER_PURGE_PROJECTION_INVALID: Final = "JOB_HANDLER_PURGE_PROJECTION_INVALID"
JOB_HANDLER_PURGE_COMPLETENESS_FAILED: Final = (
    "JOB_HANDLER_PURGE_COMPLETENESS_FAILED"
)
JOB_HANDLER_DEPENDENCY_ERROR_CODES: Final[frozenset[str]] = frozenset(
    {
        JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
        JOB_HANDLER_SOURCE_INVALID,
        JOB_HANDLER_PARSE_ARTIFACT_INVALID,
        JOB_HANDLER_PARSE_COMPLETION_FAILED,
        JOB_HANDLER_SOURCE_HASH_MISMATCH,
        JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID,
        JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
        JOB_HANDLER_EMBEDDING_COUNT_MISMATCH,
        JOB_HANDLER_EMBEDDING_CHILD_MISMATCH,
        JOB_HANDLER_EMBEDDING_COMPLETENESS_FAILED,
        JOB_HANDLER_EMBEDDING_COMPLETION_FAILED,
        JOB_HANDLER_PURGE_VISIBILITY_INVALID,
        JOB_HANDLER_PURGE_PROJECTION_INVALID,
        JOB_HANDLER_PURGE_COMPLETENESS_FAILED,
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


@dataclass(frozen=True, slots=True)
class PassageEmbeddingCandidate:
    """One child chunk whose lexical row needs a Jina passage embedding."""

    child_chunk_id: uuid.UUID
    content: str
    content_hash: str

    def __post_init__(self) -> None:
        if self.child_chunk_id == _ZERO_UUID:
            _reject(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)
        if not isinstance(self.content, str) or not self.content.strip():
            _reject(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)
        if not _SHA256_RE.fullmatch(self.content_hash):
            _reject(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)


@dataclass(frozen=True, slots=True)
class PassageEmbeddingVector:
    """Raw provider vector returned for a child chunk."""

    child_chunk_id: uuid.UUID
    embedding: tuple[float, ...]
    model_id: str = DEFAULT_JINA_EMBEDDING_MODEL
    dimensions: int = DEFAULT_JINA_EMBEDDING_DIMENSIONS

    def __post_init__(self) -> None:
        if self.child_chunk_id == _ZERO_UUID:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if self.model_id != DEFAULT_JINA_EMBEDDING_MODEL:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if self.dimensions != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        _validate_embedding_vector(self.embedding)


@dataclass(frozen=True, slots=True)
class StagedPassageEmbedding:
    """Embedding projection update ready for Postgres staging."""

    child_chunk_id: uuid.UUID
    embedding_model_id: str
    embedding_dimensions: int
    embedding_vector: tuple[float, ...]
    embedding_vector_sha256: str

    def __post_init__(self) -> None:
        if self.child_chunk_id == _ZERO_UUID:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if self.embedding_model_id != DEFAULT_JINA_EMBEDDING_MODEL:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if self.embedding_dimensions != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if not _SHA256_RE.fullmatch(self.embedding_vector_sha256):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if self.embedding_vector_sha256 != embedding_vector_sha256(
            self.embedding_vector
        ):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)


@dataclass(frozen=True, slots=True)
class PurgeInvisibilityResult:
    """Proof that a tombstoned version is no longer query-visible."""

    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    collection_visibility_epoch: int
    document_visibility_epoch: int
    query_visible: bool

    def __post_init__(self) -> None:
        if _ZERO_UUID in {
            self.collection_id,
            self.document_id,
            self.document_version_id,
        }:
            _reject(JOB_HANDLER_PURGE_VISIBILITY_INVALID)
        if (
            isinstance(self.collection_visibility_epoch, bool)
            or not isinstance(self.collection_visibility_epoch, int)
            or self.collection_visibility_epoch < 1
            or isinstance(self.document_visibility_epoch, bool)
            or not isinstance(self.document_visibility_epoch, int)
            or self.document_visibility_epoch < 1
        ):
            _reject(JOB_HANDLER_PURGE_VISIBILITY_INVALID)
        if not isinstance(self.query_visible, bool):
            _reject(JOB_HANDLER_PURGE_VISIBILITY_INVALID)


@dataclass(frozen=True, slots=True)
class PurgeProjectionResult:
    """Projection cleanup proof for one admitted purge job."""

    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID | None
    purged_child_search_rows: int
    remaining_ready_child_search_rows: int

    def __post_init__(self) -> None:
        if _ZERO_UUID in {
            self.collection_id,
            self.document_id,
            self.document_version_id,
            self.index_generation_id,
        }:
            _reject(JOB_HANDLER_PURGE_PROJECTION_INVALID)
        if self.materialization_id == _ZERO_UUID:
            _reject(JOB_HANDLER_PURGE_PROJECTION_INVALID)
        if (
            isinstance(self.purged_child_search_rows, bool)
            or not isinstance(self.purged_child_search_rows, int)
            or self.purged_child_search_rows < 0
            or isinstance(self.remaining_ready_child_search_rows, bool)
            or not isinstance(self.remaining_ready_child_search_rows, int)
            or self.remaining_ready_child_search_rows < 0
        ):
            _reject(JOB_HANDLER_PURGE_PROJECTION_INVALID)


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
    """Postgres parse projection and stage-finalizer gateway."""

    def stage_parse_projection(
        self, context: ProcessingJobContext, batch: PostgresProjectionBatch
    ) -> Coroutine[Any, Any, None]: ...

    def complete_parse_and_enqueue_embedding(
        self, context: ProcessingJobContext, *, embedding_job_id: uuid.UUID
    ) -> Coroutine[Any, Any, bool]: ...


class PassageEmbeddingGateway(Protocol):
    """Provider gateway for Jina passage embeddings."""

    def embed_passages(
        self,
        context: ProcessingJobContext,
        candidates: tuple[PassageEmbeddingCandidate, ...],
    ) -> Coroutine[Any, Any, tuple[PassageEmbeddingVector, ...]]: ...


class PassageEmbeddingProjectionGateway(Protocol):
    """Postgres gateway for candidate fetch, vector stage, and completeness."""

    def fetch_passage_embedding_candidates(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, tuple[PassageEmbeddingCandidate, ...]]: ...

    def stage_passage_embeddings(
        self,
        context: ProcessingJobContext,
        embeddings: tuple[StagedPassageEmbedding, ...],
    ) -> Coroutine[Any, Any, None]: ...

    def assert_materialization_search_complete(
        self,
        context: ProcessingJobContext,
        *,
        expected_child_count: int,
    ) -> Coroutine[Any, Any, bool]: ...

    def complete_embedding_and_publish(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, bool]: ...


class PurgeProjectionGateway(Protocol):
    """Postgres gateway for immediate invisibility and projection cleanup."""

    def mark_purge_invisible(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, PurgeInvisibilityResult]: ...

    def purge_search_projection(
        self, context: ProcessingJobContext
    ) -> Coroutine[Any, Any, PurgeProjectionResult]: ...

    def assert_purge_complete(
        self,
        context: ProcessingJobContext,
        result: PurgeProjectionResult,
    ) -> Coroutine[Any, Any, bool]: ...


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


@dataclass(frozen=True, slots=True)
class PassageEmbeddingHandlerDependencies:
    """Explicit passage-embedding dependencies; absent values fail closed."""

    embedding: PassageEmbeddingGateway | None = None
    projection: PassageEmbeddingProjectionGateway | None = None

    def require_ready(
        self,
    ) -> tuple[PassageEmbeddingGateway, PassageEmbeddingProjectionGateway]:
        """Return configured gateways or fail before any external side effect."""
        if self.embedding is None or self.projection is None:
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        return self.embedding, self.projection


@dataclass(frozen=True, slots=True)
class PurgeHandlerDependencies:
    """Explicit purge dependencies; absent values fail closed."""

    projection: PurgeProjectionGateway | None = None

    def require_ready(self) -> PurgeProjectionGateway:
        """Return the projection gateway or fail before any external side effect."""
        if self.projection is None:
            _reject(JOB_HANDLER_DEPENDENCY_UNCONFIGURED)
        return self.projection


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
    committed = await projection_gateway.complete_parse_and_enqueue_embedding(
        admitted,
        embedding_job_id=uuid.uuid4(),
    )
    if not committed:
        _reject(JOB_HANDLER_PARSE_COMPLETION_FAILED)
    return JobResult(terminal_committed=True)


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


async def passage_embedding_handler_with_dependencies(
    context: ProcessingJobContext,
    dependencies: PassageEmbeddingHandlerDependencies,
) -> JobResult:
    """Execute the admitted Jina passage-embedding seam."""
    admitted = require_passage_embedding_context(context)
    embedding_gateway, projection_gateway = dependencies.require_ready()

    candidates = await projection_gateway.fetch_passage_embedding_candidates(admitted)
    _validate_embedding_candidates(candidates)
    vectors = await embedding_gateway.embed_passages(admitted, candidates)
    staged = _stage_passage_embedding_vectors(candidates, vectors)
    await projection_gateway.stage_passage_embeddings(admitted, staged)
    complete = await projection_gateway.assert_materialization_search_complete(
        admitted,
        expected_child_count=len(candidates),
    )
    if not complete:
        _reject(JOB_HANDLER_EMBEDDING_COMPLETENESS_FAILED)
    committed = await projection_gateway.complete_embedding_and_publish(admitted)
    if not committed:
        _reject(JOB_HANDLER_EMBEDDING_COMPLETION_FAILED)
    return JobResult(terminal_committed=True)


def admitted_passage_embedding_handler_with_dependencies(
    dependencies: PassageEmbeddingHandlerDependencies,
    provider_profile: ProviderRuntimeProfile,
) -> JobHandler:
    """Build a claim-level passage-embedding handler through admission."""

    async def contextual(context: ProcessingJobContext) -> JobResult:
        return await passage_embedding_handler_with_dependencies(context, dependencies)

    return with_job_context_admission(
        contextual,
        provider_profile=provider_profile,
    )


async def purge_handler_with_dependencies(
    context: ProcessingJobContext,
    dependencies: PurgeHandlerDependencies,
) -> JobResult:
    """Execute the admitted purge seam with immediate invisibility first."""
    admitted = require_purge_context(context)
    projection_gateway = dependencies.require_ready()

    invisibility = await projection_gateway.mark_purge_invisible(admitted)
    _validate_purge_invisibility(admitted, invisibility)
    projection = await projection_gateway.purge_search_projection(admitted)
    _validate_purge_projection(admitted, projection)
    complete = await projection_gateway.assert_purge_complete(admitted, projection)
    if not complete:
        _reject(JOB_HANDLER_PURGE_COMPLETENESS_FAILED)
    return JobResult()


def admitted_purge_handler_with_dependencies(
    dependencies: PurgeHandlerDependencies,
) -> JobHandler:
    """Build a claim-level purge handler through admission."""

    async def contextual(context: ProcessingJobContext) -> JobResult:
        return await purge_handler_with_dependencies(context, dependencies)

    return with_job_context_admission(contextual)


def embedding_vector_sha256(embedding: tuple[float, ...]) -> str:
    """Hash the exact float32 lane bytes that will be stored in REAL[]."""
    digest = hashlib.sha256()
    for value in _validate_embedding_vector(embedding):
        digest.update(struct.pack("!f", value))
    return digest.hexdigest()


def _validate_embedding_candidates(
    candidates: tuple[PassageEmbeddingCandidate, ...]
) -> None:
    seen: set[uuid.UUID] = set()
    for candidate in candidates:
        if not isinstance(candidate, PassageEmbeddingCandidate):
            _reject(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)
        if candidate.child_chunk_id in seen:
            _reject(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)
        seen.add(candidate.child_chunk_id)


def _stage_passage_embedding_vectors(
    candidates: tuple[PassageEmbeddingCandidate, ...],
    vectors: tuple[PassageEmbeddingVector, ...],
) -> tuple[StagedPassageEmbedding, ...]:
    if len(vectors) != len(candidates):
        _reject(JOB_HANDLER_EMBEDDING_COUNT_MISMATCH)
    staged: list[StagedPassageEmbedding] = []
    for candidate, vector in zip(candidates, vectors, strict=True):
        if not isinstance(vector, PassageEmbeddingVector):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        if vector.child_chunk_id != candidate.child_chunk_id:
            _reject(JOB_HANDLER_EMBEDDING_CHILD_MISMATCH)
        staged.append(
            StagedPassageEmbedding(
                child_chunk_id=vector.child_chunk_id,
                embedding_model_id=DEFAULT_JINA_EMBEDDING_MODEL,
                embedding_dimensions=DEFAULT_JINA_EMBEDDING_DIMENSIONS,
                embedding_vector=vector.embedding,
                embedding_vector_sha256=embedding_vector_sha256(vector.embedding),
            )
        )
    return tuple(staged)


def _validate_embedding_vector(embedding: tuple[float, ...]) -> tuple[float, ...]:
    if len(embedding) != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
        _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
    values: list[float] = []
    for value in embedding:
        if isinstance(value, bool) or not isinstance(value, int | float):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        number = float(value)
        if not math.isfinite(number):
            _reject(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
        values.append(number)
    return tuple(values)


def _validate_purge_invisibility(
    context: ProcessingJobContext, result: PurgeInvisibilityResult
) -> None:
    if not isinstance(result, PurgeInvisibilityResult):
        _reject(JOB_HANDLER_PURGE_VISIBILITY_INVALID)
    if (
        result.collection_id != context.collection_id
        or result.document_id != context.document_id
        or result.document_version_id != context.document_version_id
        or result.collection_visibility_epoch != context.collection_visibility_epoch
        or result.document_visibility_epoch != context.document_visibility_epoch
        or result.query_visible
    ):
        _reject(JOB_HANDLER_PURGE_VISIBILITY_INVALID)


def _validate_purge_projection(
    context: ProcessingJobContext, result: PurgeProjectionResult
) -> None:
    if not isinstance(result, PurgeProjectionResult):
        _reject(JOB_HANDLER_PURGE_PROJECTION_INVALID)
    if (
        result.collection_id != context.collection_id
        or result.document_id != context.document_id
        or result.document_version_id != context.document_version_id
        or result.index_generation_id != context.index_generation_id
        or result.materialization_id != context.materialization_id
        or result.remaining_ready_child_search_rows != 0
    ):
        _reject(JOB_HANDLER_PURGE_PROJECTION_INVALID)


def _reject(error_code: str) -> NoReturn:
    raise PermanentJobError(stable_error_code(error_code))


def _reject_from(error_code: str, cause: Exception) -> NoReturn:
    try:
        _reject(error_code)
    except PermanentJobError as error:
        raise error from cause
