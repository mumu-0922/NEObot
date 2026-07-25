"""Operator-only structure generation lifecycle and activation gate."""

from __future__ import annotations

import hashlib
import json
import uuid
from collections.abc import Mapping, Sequence
from typing import Any, Final, cast

import psycopg
from psycopg.conninfo import make_conninfo
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb

from mm_chat_rag.generation_gate_report import (
    GATE_REPORT_SCHEMA_VERSION,
    GenerationOperatorError,
)
from mm_chat_rag.generation_gate_report import (
    report_hash as _report_hash,
)
from mm_chat_rag.generation_gate_report import (
    report_uuid as _report_uuid,
)
from mm_chat_rag.provider_profile import (
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
    DEFAULT_SILICONFLOW_RERANK_MODEL,
)
from mm_chat_rag.structure_chunking import (
    SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
    structure_chunk_profile,
)

_SHA256_HEX_LENGTH: Final = 64

_STATUS_SQL: Final = "SELECT * FROM knowledge_structure_generation_operator_status()"
_DOCUMENTS_SQL: Final = (
    "SELECT * FROM knowledge_list_structure_generation_rebuild_documents(%s, %s)"
)
_BEGIN_SQL: Final = (
    "SELECT * FROM knowledge_begin_registered_structure_generation_rebuild"
    "(%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb)"
)
_VERIFY_SQL: Final = "SELECT * FROM knowledge_verify_structure_generation(%s, %s, %s)"
_ABANDON_SQL: Final = (
    "SELECT knowledge_abandon_structure_generation_candidate"
    "(%s, %s, %s, %s, %s) AS abandoned"
)
_ACTIVATE_SQL: Final = (
    "SELECT knowledge_activate_structure_generation_candidate"
    "(%s, %s, %s, %s, %s) AS activated"
)
_ROLLBACK_SQL: Final = (
    "SELECT knowledge_rollback_index_generation(%s, %s, %s, %s, %s) AS rolled_back"
)


def canonical_sha256(value: object) -> str:
    """Hash one bounded canonical JSON value."""
    encoded = json.dumps(
        value,
        ensure_ascii=True,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()
    return hashlib.sha256(encoded).hexdigest()


async def generation_status(database_url: str) -> dict[str, object]:
    """Read source-text-free active/candidate lifecycle state."""
    async with await _connect(database_url) as connection:
        cursor = await connection.execute(_STATUS_SQL)
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
    if row is None:
        raise GenerationOperatorError("generation status returned no active head")
    return _jsonable_mapping(row)


async def begin_structure_generation(database_url: str) -> dict[str, object]:
    """Allocate a complete non-active SiliconFlow BGE candidate."""
    async with (
        await _connect(database_url) as connection,
        connection.transaction(),
    ):
        status_cursor = await connection.execute(_STATUS_SQL)
        status = cast("Mapping[str, Any] | None", await status_cursor.fetchone())
        if status is None:
            raise GenerationOperatorError("generation status returned no active head")
        if status.get("candidate_generation_id") is not None:
            raise GenerationOperatorError(
                "a building or verified candidate already exists"
            )
        active_generation_id = _required_uuid(status, "active_generation_id")
        head_revision = _required_positive_int(status, "head_revision")

        documents_cursor = await connection.execute(
            _DOCUMENTS_SQL,
            (active_generation_id, head_revision),
        )
        document_rows = cast(
            "Sequence[Mapping[str, Any]]",
            await documents_cursor.fetchall(),
        )
        document_ids = tuple(
            sorted(_required_uuid(row, "document_id") for row in document_rows)
        )
        if not document_ids:
            raise GenerationOperatorError("candidate corpus snapshot is empty")

        index_profile_id = uuid.uuid4()
        search_profile_id = uuid.uuid4()
        generation_id = uuid.uuid4()
        parser_manifest = {
            "mineru": "structure",
            "native": "structure",
            "schemaVersion": "g11.9d-structure-parser-manifest.v1",
        }
        parser_manifest_hash = canonical_sha256(parser_manifest)
        base_profile_hash = canonical_sha256(
            {
                "candidateGenerationId": str(generation_id),
                "chunkProfile": structure_chunk_profile(
                    SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
                ),
                "parserManifestHash": parser_manifest_hash,
                "schemaVersion": "mm-chat.index-profile.v3",
            }
        )
        search_profile_hash = canonical_sha256(
            {
                "baseProfileHash": base_profile_hash,
                "embeddingModel": DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
                "providerProfile": "siliconflow_bge_m3_v1",
                "rerankModel": DEFAULT_SILICONFLOW_RERANK_MODEL,
                "schemaVersion": "mm-chat.search-profile.v3",
            }
        )
        build_snapshot = {
            "activeGenerationId": str(active_generation_id),
            "candidateGenerationId": str(generation_id),
            "chunkProfileHash": SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
            "documentIds": [str(document_id) for document_id in document_ids],
            "headRevision": head_revision,
            "schemaVersion": "mm-chat.structure-rebuild-snapshot.v3",
        }
        build_snapshot_hash = canonical_sha256(build_snapshot)
        allocations: list[dict[str, str]] = []
        for document_id in document_ids:
            materialization_id = uuid.uuid4()
            job_id = uuid.uuid4()
            request_hash = canonical_sha256(
                {
                    "chunkProfileHash": SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
                    "documentId": str(document_id),
                    "generationId": str(generation_id),
                    "jobId": str(job_id),
                    "materializationId": str(materialization_id),
                }
            )
            allocations.append(
                {
                    "documentId": str(document_id),
                    "jobId": str(job_id),
                    "materializationId": str(materialization_id),
                    "requestHash": request_hash,
                }
            )

        cursor = await connection.execute(
            _BEGIN_SQL,
            (
                index_profile_id,
                search_profile_id,
                generation_id,
                SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
                base_profile_hash,
                parser_manifest_hash,
                search_profile_hash,
                build_snapshot_hash,
                Jsonb(allocations),
            ),
        )
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
        if (
            row is None
            or _required_uuid(
                row,
                "candidate_generation_id",
            )
            != generation_id
        ):
            raise GenerationOperatorError(
                "candidate allocation returned an invalid result"
            )

    return {
        "activeGenerationId": str(active_generation_id),
        "allocatedDocumentCount": _required_positive_int(
            row,
            "allocated_document_count",
        ),
        "baseProfileHash": base_profile_hash,
        "buildSnapshotHash": build_snapshot_hash,
        "candidateGenerationId": str(generation_id),
        "chunkProfileHash": SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
        "headRevision": head_revision,
        "indexProfileId": str(index_profile_id),
        "mode": "executed",
        "parserManifestHash": parser_manifest_hash,
        "searchProfileHash": search_profile_hash,
        "searchProfileId": str(search_profile_id),
    }


async def verify_structure_generation(database_url: str) -> dict[str, object]:
    """Freeze candidate integrity without changing the active head."""
    async with (
        await _connect(database_url) as connection,
        connection.transaction(),
    ):
        status_cursor = await connection.execute(_STATUS_SQL)
        status = cast("Mapping[str, Any] | None", await status_cursor.fetchone())
        if status is None:
            raise GenerationOperatorError("generation status returned no active head")
        candidate_generation_id = _required_uuid(status, "candidate_generation_id")
        head_revision = _required_positive_int(status, "head_revision")
        candidate_profile_hash = _required_text(
            status,
            "candidate_chunk_profile_hash",
        )
        if candidate_profile_hash != SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH:
            raise GenerationOperatorError(
                "candidate chunk profile is not the frozen SiliconFlow v3 profile"
            )
        cursor = await connection.execute(
            _VERIFY_SQL,
            (
                candidate_generation_id,
                head_revision,
                SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
            ),
        )
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
        if row is None:
            raise GenerationOperatorError("candidate verification returned no result")
    report = {
        "artifactManifestHash": _required_hash(row, "artifact_manifest_hash"),
        "blockCount": _required_non_negative_int(row, "block_count"),
        "candidateGenerationId": str(candidate_generation_id),
        "childCount": _required_positive_int(row, "child_count"),
        "chunkProfileHash": SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
        "documentCount": _required_positive_int(row, "document_count"),
        "headRevision": head_revision,
        "parentCount": _required_positive_int(row, "parent_count"),
        "promotionEligible": False,
        "requiredGateReportSchema": GATE_REPORT_SCHEMA_VERSION,
    }
    report["verificationReportSha256"] = canonical_sha256(report)
    return report


async def abandon_structure_generation(
    database_url: str,
    *,
    candidate_generation_id: uuid.UUID,
    expected_head_revision: int,
    expected_manifest_hash: str,
    operator_id: uuid.UUID,
    reason: str,
) -> bool:
    """Fail one exact verified Candidate and append its immutable operator audit."""
    async with (
        await _connect(database_url) as connection,
        connection.transaction(),
    ):
        cursor = await connection.execute(
            _ABANDON_SQL,
            (
                candidate_generation_id,
                expected_head_revision,
                expected_manifest_hash,
                operator_id,
                reason,
            ),
        )
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
        return bool(row and row.get("abandoned") is True)


async def activate_structure_generation(
    database_url: str,
    *,
    gate_report: Mapping[str, object],
    gate_report_sha256: str,
    operator_id: uuid.UUID,
) -> bool:
    """Activate only the exact currently verified report-bound candidate."""
    candidate_generation_id = _report_uuid(
        gate_report,
        "candidateGenerationId",
    )
    manifest_hash = _report_hash(gate_report, "artifactManifestHash")
    async with (
        await _connect(database_url) as connection,
        connection.transaction(),
    ):
        status_cursor = await connection.execute(_STATUS_SQL)
        status = cast("Mapping[str, Any] | None", await status_cursor.fetchone())
        if status is None:
            raise GenerationOperatorError("generation status returned no active head")
        if _required_uuid(status, "candidate_generation_id") != candidate_generation_id:
            raise GenerationOperatorError("gate report candidate is not current")
        if status.get("candidate_status") != "verified":
            raise GenerationOperatorError("candidate is not verified")
        if _required_hash(status, "candidate_artifact_manifest_hash") != manifest_hash:
            raise GenerationOperatorError("gate report manifest is stale")
        head_revision = _required_positive_int(status, "head_revision")
        cursor = await connection.execute(
            _ACTIVATE_SQL,
            (
                candidate_generation_id,
                head_revision,
                manifest_hash,
                gate_report_sha256,
                operator_id,
            ),
        )
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
        return bool(row and row.get("activated") is True)


async def rollback_structure_generation(
    database_url: str,
    *,
    active_generation_id: uuid.UUID,
    target_generation_id: uuid.UUID,
    expected_head_revision: int,
    active_manifest_hash: str,
    target_manifest_hash: str,
) -> bool:
    """Execute the existing fenced pointer rollback through the operator role."""
    async with (
        await _connect(database_url) as connection,
        connection.transaction(),
    ):
        cursor = await connection.execute(
            _ROLLBACK_SQL,
            (
                active_generation_id,
                target_generation_id,
                expected_head_revision,
                active_manifest_hash,
                target_manifest_hash,
            ),
        )
        row = cast("Mapping[str, Any] | None", await cursor.fetchone())
        return bool(row and row.get("rolled_back") is True)


async def _connect(
    database_url: str,
) -> psycopg.AsyncConnection[dict[str, Any]]:
    conninfo = make_conninfo(
        database_url,
        application_name="mm-chat-rag-generation-operator",
        options=(
            "-c statement_timeout=30000 -c lock_timeout=3000 "
            "-c idle_in_transaction_session_timeout=30000"
        ),
    )
    return await psycopg.AsyncConnection.connect(conninfo, row_factory=dict_row)


def _jsonable_mapping(row: Mapping[str, Any]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in row.items():
        result[str(key)] = (
            str(value)
            if isinstance(value, uuid.UUID)
            else cast(
                "object",
                value,
            )
        )
    return result


def _required_uuid(row: Mapping[str, Any], key: str) -> uuid.UUID:
    value = row.get(key)
    if isinstance(value, uuid.UUID):
        return value
    try:
        return uuid.UUID(str(value))
    except (TypeError, ValueError) as error:
        raise GenerationOperatorError(f"database result {key} is invalid") from error


def _required_text(row: Mapping[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not value:
        raise GenerationOperatorError(f"database result {key} is invalid")
    return value


def _required_hash(row: Mapping[str, Any], key: str) -> str:
    value = _required_text(row, key).lower()
    if len(value) != _SHA256_HEX_LENGTH or any(
        character not in "0123456789abcdef" for character in value
    ):
        raise GenerationOperatorError(f"database result {key} is invalid")
    return value


def _required_positive_int(row: Mapping[str, Any], key: str) -> int:
    value = row.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        raise GenerationOperatorError(f"database result {key} is invalid")
    return value


def _required_non_negative_int(row: Mapping[str, Any], key: str) -> int:
    value = row.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise GenerationOperatorError(f"database result {key} is invalid")
    return value
