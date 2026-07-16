"""Async psycopg adapter restricted to frozen stored-function calls."""

from __future__ import annotations

import time
import uuid
from collections.abc import Mapping, Sequence
from typing import Any, Final, cast

import psycopg
from psycopg.conninfo import make_conninfo
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

from mm_chat_rag.job_context import (
    JOB_CONTEXT_LEASE_FENCE_MISSING,
    ProcessingJobContext,
)
from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID,
    JOB_HANDLER_EMBEDDING_VECTOR_INVALID,
    JOB_HANDLER_PURGE_PROJECTION_INVALID,
    JOB_HANDLER_PURGE_VISIBILITY_INVALID,
    PassageEmbeddingCandidate,
    PurgeInvisibilityResult,
    PurgeProjectionResult,
    StagedPassageEmbedding,
)
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import (
    FunctionReadiness,
    JobClaim,
    OutboxClaim,
    stable_error_code,
)
from mm_chat_rag.retry import PermanentJobError
from mm_chat_rag.settings import Settings

_SQL: Final[Mapping[str, str]] = {
    "claim_outbox": ("SELECT * FROM knowledge_claim_outbox(%s, %s, %s, %s)"),
    "apply_outbox": (
        "SELECT * FROM knowledge_apply_and_ack_outbox(%s, %s, %s, %s, %s, %s, %s, %s)"
    ),
    "retry_outbox": ("SELECT * FROM knowledge_retry_outbox(%s, %s, %s, %s, %s)"),
    "fail_outbox": ("SELECT * FROM knowledge_fail_outbox(%s, %s, %s, %s)"),
    "claim_job": ("SELECT * FROM knowledge_claim_processing_job(%s, %s, %s, %s)"),
    "heartbeat_job": (
        "SELECT * FROM knowledge_heartbeat_processing_job(%s, %s, %s, %s)"
    ),
    "finish_job": (
        "SELECT * FROM knowledge_finish_processing_job(%s, %s, %s, %s, %s, %s)"
    ),
    "readiness": "SELECT * FROM knowledge_rag_worker_readiness()",
    "assert_search_complete": (
        "SELECT * FROM knowledge_assert_materialization_search_complete(%s, %s, %s, %s)"
    ),
    "fetch_passage_embedding_candidates": (
        "SELECT * FROM knowledge_fetch_passage_embedding_candidates(%s, %s, %s, %s)"
    ),
    "stage_passage_embedding": (
        "SELECT * FROM knowledge_stage_passage_embedding"
        "(%s, %s, %s, %s, %s, %s, %s)"
    ),
    "mark_purge_invisible": (
        "SELECT * FROM knowledge_mark_purge_invisible(%s, %s, %s, %s, %s, %s, %s, %s)"
    ),
    "purge_search_projection": (
        "SELECT * FROM knowledge_purge_search_projection"
        "(%s, %s, %s, %s, %s, %s, %s, %s)"
    ),
    "assert_purge_complete": (
        "SELECT * FROM knowledge_assert_purge_complete"
        "(%s, %s, %s, %s, %s, %s, %s, %s, %s)"
    ),
    "replay_outbox": ("SELECT * FROM knowledge_replay_outbox(%s, %s, %s, %s)"),
    "replay_job": ("SELECT * FROM knowledge_replay_processing_job(%s, %s, %s, %s, %s)"),
}


class PostgresAdapter:
    """Own short transactions plus one dedicated session advisory-lock connection."""

    def __init__(self, settings: Settings, metrics: Metrics) -> None:
        options = (
            f"-c statement_timeout={settings.statement_timeout_ms} "
            f"-c lock_timeout={settings.lock_timeout_ms} "
            "-c idle_in_transaction_session_timeout="
            f"{settings.idle_transaction_timeout_ms}"
        )
        self._conninfo = make_conninfo(
            settings.database_url,
            application_name="mm-chat-rag-worker",
            options=options,
        )
        self._pool = AsyncConnectionPool(
            conninfo=self._conninfo,
            min_size=1,
            max_size=2,
            open=False,
            kwargs={"row_factory": dict_row},
            check=AsyncConnectionPool.check_connection,
        )
        self._settings = settings
        self._metrics = metrics
        self._lock_connection: psycopg.AsyncConnection[dict[str, Any]] | None = None

    async def open(self) -> None:
        """Open and verify the bounded pool."""
        await self._pool.open(wait=True, timeout=10)
        await self._pool.check()

    async def close(self) -> None:
        """Release the session lock and close all connections."""
        await self.release_worker_lock()
        await self._pool.close()

    async def acquire_worker_lock(self) -> bool:
        """Acquire the process-wide single-worker session advisory lock."""
        if self._lock_connection is not None:
            return True
        connection = await psycopg.AsyncConnection.connect(
            self._conninfo,
            autocommit=True,
            row_factory=dict_row,
        )
        cursor = await connection.execute(
            "SELECT pg_try_advisory_lock(%s) AS acquired",
            (self._settings.advisory_lock_key,),
        )
        row = await cursor.fetchone()
        acquired = bool(row and row.get("acquired") is True)
        if not acquired:
            await connection.close()
            return False
        self._lock_connection = connection
        self._metrics.worker_lock.set(1)
        return True

    async def release_worker_lock(self) -> None:
        """Best-effort explicit unlock; connection close is the final fence."""
        connection = self._lock_connection
        if connection is None:
            return
        self._lock_connection = None
        try:
            await connection.execute(
                "SELECT pg_advisory_unlock(%s)",
                (self._settings.advisory_lock_key,),
            )
        finally:
            await connection.close()
            self._metrics.worker_lock.set(0)

    async def _call(
        self, function: str, parameters: Sequence[object] = ()
    ) -> Mapping[str, Any] | None:
        started = time.monotonic()
        try:
            async with (
                self._pool.connection() as connection,
                connection.transaction(),
            ):
                cursor = await connection.execute(_SQL[function], parameters)
                return cast("Mapping[str, Any] | None", await cursor.fetchone())
        finally:
            self._metrics.function_seconds.labels(function=function).observe(
                time.monotonic() - started
            )

    async def _call_all(
        self, function: str, parameters: Sequence[object] = ()
    ) -> tuple[Mapping[str, Any], ...]:
        started = time.monotonic()
        try:
            async with (
                self._pool.connection() as connection,
                connection.transaction(),
            ):
                cursor = await connection.execute(_SQL[function], parameters)
                rows = await cursor.fetchall()
                return tuple(cast("Sequence[Mapping[str, Any]]", rows))
        finally:
            self._metrics.function_seconds.labels(function=function).observe(
                time.monotonic() - started
            )

    async def readiness(self) -> FunctionReadiness:
        """Call the sole database readiness contract."""
        row = await self._call("readiness")
        if row is None:
            raise RuntimeError("readiness function returned no row")
        return FunctionReadiness.from_row(row)

    async def claim_outbox(self, lock_token: uuid.UUID) -> OutboxClaim | None:
        """Claim exactly one outbox row using a fresh token."""
        row = await self._call(
            "claim_outbox",
            (
                self._settings.consumer_name,
                self._settings.worker_id,
                lock_token,
                self._settings.outbox_lease_seconds,
            ),
        )
        return None if row is None else OutboxClaim.from_row(row)

    async def apply_and_ack_outbox(
        self,
        claim: OutboxClaim,
        lock_token: uuid.UUID,
        *,
        scope_kind: str,
        index_generation_id: uuid.UUID | None,
        action: str,
        result_hash: str,
    ) -> bool:
        """Atomically apply the plan, ledger it, and token-CAS ack."""
        try:
            row = await self._call(
                "apply_outbox",
                (
                    self._settings.consumer_name,
                    claim.event_id,
                    self._settings.worker_id,
                    lock_token,
                    scope_kind,
                    index_generation_id,
                    action,
                    result_hash,
                ),
            )
        except psycopg.Error as error:
            if _has_database_code(error, "RAG_STALE_OUTBOX_LEASE"):
                return False
            raise
        return _function_succeeded(row)

    async def retry_outbox(
        self,
        claim: OutboxClaim,
        lock_token: uuid.UUID,
        error_code: str,
        retry_after_seconds: int,
    ) -> bool:
        """Release a leased event for bounded retry by token CAS."""
        try:
            row = await self._call(
                "retry_outbox",
                (
                    claim.event_id,
                    self._settings.worker_id,
                    lock_token,
                    error_code,
                    retry_after_seconds,
                ),
            )
        except psycopg.Error as error:
            if _has_database_code(error, "RAG_STALE_OUTBOX_LEASE"):
                return False
            raise
        return _function_succeeded(row)

    async def fail_outbox(
        self, claim: OutboxClaim, lock_token: uuid.UUID, error_code: str
    ) -> bool:
        """Move an event to Postgres failed-state DLQ by token CAS."""
        try:
            row = await self._call(
                "fail_outbox",
                (claim.event_id, self._settings.worker_id, lock_token, error_code),
            )
        except psycopg.Error as error:
            if _has_database_code(error, "RAG_STALE_OUTBOX_LEASE"):
                return False
            raise
        return _function_succeeded(row)

    async def claim_job(self, lease_token: uuid.UUID) -> JobClaim | None:
        """Claim one allowlisted, non-legacy job."""
        row = await self._call(
            "claim_job",
            (
                self._settings.worker_id,
                lease_token,
                self._settings.job_lease_seconds,
                list(self._settings.job_stages),
            ),
        )
        return None if row is None else JobClaim.from_row(row)

    async def heartbeat_job(self, job: JobClaim, lease_token: uuid.UUID) -> bool:
        """Extend a live job lease using owner and token fencing."""
        try:
            row = await self._call(
                "heartbeat_job",
                (
                    job.job_id,
                    self._settings.worker_id,
                    lease_token,
                    self._settings.job_lease_seconds,
                ),
            )
        except psycopg.Error as error:
            if _has_database_code(error, "RAG_STALE_JOB_LEASE"):
                return False
            raise
        return _function_succeeded(row)

    async def finish_job(
        self,
        job: JobClaim,
        lease_token: uuid.UUID,
        *,
        outcome: str,
        error_code: str | None,
        retry_after_seconds: int,
    ) -> bool:
        """Finish, retry, or DLQ one job through its frozen function."""
        try:
            row = await self._call(
                "finish_job",
                (
                    job.job_id,
                    self._settings.worker_id,
                    lease_token,
                    outcome,
                    error_code,
                    retry_after_seconds,
                ),
            )
        except psycopg.Error as error:
            if _has_database_code(error, "RAG_STALE_JOB_LEASE"):
                return False
            raise
        return _function_succeeded(row)

    async def fetch_passage_embedding_candidates(
        self, context: ProcessingJobContext
    ) -> tuple[PassageEmbeddingCandidate, ...]:
        """Fetch child search rows that require or can safely replay embeddings."""
        lease_token = _require_context_lease_token(context)
        rows = await self._call_all(
            "fetch_passage_embedding_candidates",
            (
                context.job_id,
                self._settings.worker_id,
                lease_token,
                context.materialization_id,
            ),
        )
        return tuple(_passage_embedding_candidate_from_row(row) for row in rows)

    async def stage_passage_embeddings(
        self,
        context: ProcessingJobContext,
        embeddings: tuple[StagedPassageEmbedding, ...],
    ) -> None:
        """Stage Jina passage embeddings through one token-fenced function per row."""
        lease_token = _require_context_lease_token(context)
        for embedding in embeddings:
            row = await self._call(
                "stage_passage_embedding",
                (
                    context.job_id,
                    self._settings.worker_id,
                    lease_token,
                    context.materialization_id,
                    embedding.child_chunk_id,
                    list(embedding.embedding_vector),
                    embedding.embedding_vector_sha256,
                ),
            )
            if not _function_succeeded(row):
                raise PermanentJobError(
                    stable_error_code(JOB_HANDLER_EMBEDDING_VECTOR_INVALID)
                )

    async def assert_materialization_search_complete(
        self,
        materialization_or_context: uuid.UUID | ProcessingJobContext,
        *,
        expected_child_count: int,
    ) -> bool:
        """Call the G7.4 search projection completeness gate."""
        materialization_id = (
            materialization_or_context.materialization_id
            if isinstance(materialization_or_context, ProcessingJobContext)
            else materialization_or_context
        )
        if materialization_id is None:
            raise PermanentJobError(
                stable_error_code(JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID)
            )
        row = await self._call(
            "assert_search_complete",
            (
                materialization_id,
                expected_child_count,
                "jina-embeddings-v4",
                1024,
            ),
        )
        return _function_succeeded(row)

    async def mark_purge_invisible(
        self, context: ProcessingJobContext
    ) -> PurgeInvisibilityResult:
        """Verify a purge job is already query-invisible through a fenced function."""
        lease_token = _require_context_lease_token(context)
        row = await self._call(
            "mark_purge_invisible",
            (
                context.job_id,
                self._settings.worker_id,
                lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.collection_visibility_epoch,
                context.document_visibility_epoch,
            ),
        )
        return _purge_invisibility_from_row(row)

    async def purge_search_projection(
        self, context: ProcessingJobContext
    ) -> PurgeProjectionResult:
        """Purge the admitted search projection scope through Postgres."""
        lease_token = _require_context_lease_token(context)
        row = await self._call(
            "purge_search_projection",
            (
                context.job_id,
                self._settings.worker_id,
                lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.index_generation_id,
                context.materialization_id,
            ),
        )
        return _purge_projection_from_row(row)

    async def assert_purge_complete(
        self,
        context: ProcessingJobContext,
        result: PurgeProjectionResult,
    ) -> bool:
        """Call the final purge projection completion gate."""
        lease_token = _require_context_lease_token(context)
        row = await self._call(
            "assert_purge_complete",
            (
                context.job_id,
                self._settings.worker_id,
                lease_token,
                context.collection_id,
                context.document_id,
                context.document_version_id,
                context.index_generation_id,
                result.materialization_id,
                result.purged_child_search_rows,
            ),
        )
        return _function_succeeded(row)


def _function_succeeded(row: Mapping[str, Any] | None) -> bool:
    if row is None:
        return False
    if len(row) == 1:
        return next(iter(row.values())) is True
    return row.get("succeeded") is True or row.get("applied") is True


def _has_database_code(error: psycopg.Error, code: str) -> bool:
    """Match only stable server messages; never expose arbitrary DB text."""
    return error.diag.message_primary == code or str(error) == code


def _require_context_lease_token(context: ProcessingJobContext) -> uuid.UUID:
    if context.lease_token is None:
        raise PermanentJobError(stable_error_code(JOB_CONTEXT_LEASE_FENCE_MISSING))
    return context.lease_token


def _purge_invisibility_from_row(
    row: Mapping[str, Any] | None,
) -> PurgeInvisibilityResult:
    if row is None:
        raise PermanentJobError(stable_error_code(JOB_HANDLER_PURGE_VISIBILITY_INVALID))
    return PurgeInvisibilityResult(
        collection_id=_row_uuid(
            row, "collection_id", JOB_HANDLER_PURGE_VISIBILITY_INVALID
        ),
        document_id=_row_uuid(row, "document_id", JOB_HANDLER_PURGE_VISIBILITY_INVALID),
        document_version_id=_row_uuid(
            row, "document_version_id", JOB_HANDLER_PURGE_VISIBILITY_INVALID
        ),
        collection_visibility_epoch=_row_positive_int(
            row,
            "collection_visibility_epoch",
            JOB_HANDLER_PURGE_VISIBILITY_INVALID,
        ),
        document_visibility_epoch=_row_positive_int(
            row,
            "document_visibility_epoch",
            JOB_HANDLER_PURGE_VISIBILITY_INVALID,
        ),
        query_visible=_row_bool(row, "query_visible"),
    )


def _passage_embedding_candidate_from_row(
    row: Mapping[str, Any],
) -> PassageEmbeddingCandidate:
    return PassageEmbeddingCandidate(
        child_chunk_id=_row_uuid(
            row, "child_chunk_id", JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID
        ),
        content=_row_text(row, "content", JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID),
        content_hash=_row_text(
            row, "content_hash", JOB_HANDLER_EMBEDDING_CANDIDATE_INVALID
        ),
    )


def _purge_projection_from_row(
    row: Mapping[str, Any] | None,
) -> PurgeProjectionResult:
    if row is None:
        raise PermanentJobError(stable_error_code(JOB_HANDLER_PURGE_PROJECTION_INVALID))
    return PurgeProjectionResult(
        collection_id=_row_uuid(
            row, "collection_id", JOB_HANDLER_PURGE_PROJECTION_INVALID
        ),
        document_id=_row_uuid(
            row, "document_id", JOB_HANDLER_PURGE_PROJECTION_INVALID
        ),
        document_version_id=_row_uuid(
            row, "document_version_id", JOB_HANDLER_PURGE_PROJECTION_INVALID
        ),
        index_generation_id=_row_uuid(
            row, "index_generation_id", JOB_HANDLER_PURGE_PROJECTION_INVALID
        ),
        materialization_id=_row_optional_uuid(row, "materialization_id"),
        purged_child_search_rows=_row_non_negative_int(
            row, "purged_child_search_rows"
        ),
        remaining_ready_child_search_rows=_row_non_negative_int(
            row, "remaining_ready_child_search_rows"
        ),
    )


def _row_uuid(row: Mapping[str, Any], key: str, error_code: str) -> uuid.UUID:
    value = row.get(key)
    if isinstance(value, uuid.UUID):
        return value
    try:
        return uuid.UUID(str(value))
    except (TypeError, ValueError) as error:
        raise PermanentJobError(stable_error_code(error_code)) from error


def _row_optional_uuid(row: Mapping[str, Any], key: str) -> uuid.UUID | None:
    if row.get(key) is None:
        return None
    return _row_uuid(row, key, JOB_HANDLER_PURGE_PROJECTION_INVALID)


def _row_text(row: Mapping[str, Any], key: str, error_code: str) -> str:
    value = row.get(key)
    if not isinstance(value, str):
        raise PermanentJobError(stable_error_code(error_code))
    return value


def _row_positive_int(row: Mapping[str, Any], key: str, error_code: str) -> int:
    value = row.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        raise PermanentJobError(stable_error_code(error_code))
    return value


def _row_non_negative_int(row: Mapping[str, Any], key: str) -> int:
    value = row.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise PermanentJobError(stable_error_code(JOB_HANDLER_PURGE_PROJECTION_INVALID))
    return value


def _row_bool(row: Mapping[str, Any], key: str) -> bool:
    value = row.get(key)
    if not isinstance(value, bool):
        raise PermanentJobError(stable_error_code(JOB_HANDLER_PURGE_VISIBILITY_INVALID))
    return value


async def replay_outbox(
    database_url: str,
    *,
    event_id: uuid.UUID,
    expected_error_code: str,
    operator_id: uuid.UUID,
    reason: str,
) -> bool:
    """Execute an operator-only outbox replay function."""
    return await _replay_call(
        database_url,
        "replay_outbox",
        (event_id, expected_error_code, operator_id, reason),
    )


async def replay_job(
    database_url: str,
    *,
    job_id: uuid.UUID,
    expected_error_code: str,
    successor_job_id: uuid.UUID,
    operator_id: uuid.UUID,
    reason: str,
) -> bool:
    """Create a successor through the operator-only job replay function."""
    return await _replay_call(
        database_url,
        "replay_job",
        (job_id, expected_error_code, successor_job_id, operator_id, reason),
        expected_result=successor_job_id,
    )


async def _replay_call(
    database_url: str,
    function: str,
    parameters: Sequence[object],
    *,
    expected_result: object = True,
) -> bool:
    conninfo = make_conninfo(
        database_url,
        application_name="mm-chat-rag-replay",
        options="-c statement_timeout=10000 -c lock_timeout=2000",
    )
    try:
        async with (
            await psycopg.AsyncConnection.connect(
                conninfo, row_factory=dict_row
            ) as connection,
            connection.transaction(),
        ):
            cursor = await connection.execute(_SQL[function], parameters)
            row = cast("Mapping[str, Any] | None", await cursor.fetchone())
            return row is not None and next(iter(row.values()), None) == expected_result
    except psycopg.Error as error:
        if _has_database_code(error, "RAG_REPLAY_PRECONDITION_FAILED"):
            return False
        raise
