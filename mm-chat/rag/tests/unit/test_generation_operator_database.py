"""Database-bound generation operator unit tests."""

from __future__ import annotations

import uuid
from collections.abc import Callable, Mapping, Sequence
from typing import Any

import pytest
from psycopg.types.json import Jsonb

import mm_chat_rag.generation_operator as operator
from mm_chat_rag.generation_gate_report import GenerationOperatorError
from mm_chat_rag.structure_chunking import SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH

ACTIVE_ID = uuid.UUID("61000000-0000-4000-8000-000000000001")
CANDIDATE_ID = uuid.UUID("61000000-0000-4000-8000-000000000002")
TARGET_ID = uuid.UUID("61000000-0000-4000-8000-000000000003")
OPERATOR_ID = uuid.UUID("61000000-0000-4000-8000-000000000004")
MANIFEST_HASH = "a" * 64
GATE_HASH = "b" * 64


class _Cursor:
    def __init__(
        self,
        *,
        row: Mapping[str, Any] | None = None,
        rows: Sequence[Mapping[str, Any]] = (),
    ) -> None:
        self._row = row
        self._rows = rows

    async def fetchone(self) -> Mapping[str, Any] | None:
        return self._row

    async def fetchall(self) -> Sequence[Mapping[str, Any]]:
        return self._rows


class _AsyncContext:
    async def __aenter__(self) -> None:
        return None

    async def __aexit__(self, *_: object) -> None:
        return None


class _Connection:
    def __init__(self, cursors: Sequence[_Cursor]) -> None:
        self._cursors = list(cursors)
        self.calls: list[tuple[str, object | None]] = []
        self.entered = False

    async def __aenter__(self) -> _Connection:
        self.entered = True
        return self

    async def __aexit__(self, *_: object) -> None:
        return None

    def transaction(self) -> _AsyncContext:
        return _AsyncContext()

    async def execute(
        self,
        sql: str,
        params: object | None = None,
    ) -> _Cursor:
        self.calls.append((sql, params))
        if not self._cursors:
            raise AssertionError(f"unexpected SQL: {sql}")
        return self._cursors.pop(0)


def _install_connection(
    monkeypatch: pytest.MonkeyPatch,
    connection: _Connection,
) -> None:
    async def fake_connect(_: str) -> _Connection:
        return connection

    monkeypatch.setattr(operator, "_connect", fake_connect)


async def test_generation_status_serializes_uuid_and_rejects_missing_head(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    connection = _Connection(
        [_Cursor(row={"active_generation_id": ACTIVE_ID, "head_revision": 4})]
    )
    _install_connection(monkeypatch, connection)

    assert await operator.generation_status("postgresql://operator@db/rag") == {
        "active_generation_id": str(ACTIVE_ID),
        "head_revision": 4,
    }
    assert connection.calls == [(operator._STATUS_SQL, None)]

    missing = _Connection([_Cursor(row=None)])
    _install_connection(monkeypatch, missing)
    with pytest.raises(GenerationOperatorError, match="no active head"):
        await operator.generation_status("postgresql://operator@db/rag")


async def test_begin_structure_generation_binds_sorted_snapshot_and_allocations(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    first_document = uuid.UUID("61000000-0000-4000-8000-000000000010")
    second_document = uuid.UUID("61000000-0000-4000-8000-000000000011")
    generated = iter(
        uuid.UUID(f"61000000-0000-4000-8000-{value:012d}") for value in range(20, 27)
    )
    monkeypatch.setattr(operator.uuid, "uuid4", lambda: next(generated))
    expected_generation = uuid.UUID("61000000-0000-4000-8000-000000000022")
    connection = _Connection(
        [
            _Cursor(
                row={
                    "active_generation_id": ACTIVE_ID,
                    "candidate_generation_id": None,
                    "head_revision": 4,
                }
            ),
            _Cursor(
                rows=(
                    {"document_id": second_document},
                    {"document_id": first_document},
                )
            ),
            _Cursor(
                row={
                    "candidate_generation_id": expected_generation,
                    "allocated_document_count": 2,
                }
            ),
        ]
    )
    _install_connection(monkeypatch, connection)

    result = await operator.begin_structure_generation("postgresql://operator@db/rag")

    assert result["candidateGenerationId"] == str(expected_generation)
    assert result["activeGenerationId"] == str(ACTIVE_ID)
    assert result["allocatedDocumentCount"] == 2
    assert result["headRevision"] == 4
    assert result["chunkProfileHash"] == SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
    assert result["mode"] == "executed"
    begin_sql, raw_params = connection.calls[-1]
    assert begin_sql == operator._BEGIN_SQL
    assert isinstance(raw_params, tuple)
    allocations = raw_params[-1]
    assert isinstance(allocations, Jsonb)
    assert [item["documentId"] for item in allocations.obj] == [
        str(first_document),
        str(second_document),
    ]
    assert all(len(item["requestHash"]) == 64 for item in allocations.obj)


@pytest.mark.parametrize(
    ("status", "documents", "result", "match"),
    [
        (None, (), None, "no active head"),
        (
            {
                "active_generation_id": ACTIVE_ID,
                "candidate_generation_id": CANDIDATE_ID,
                "head_revision": 4,
            },
            (),
            None,
            "candidate already exists",
        ),
        (
            {
                "active_generation_id": ACTIVE_ID,
                "candidate_generation_id": None,
                "head_revision": 4,
            },
            (),
            None,
            "snapshot is empty",
        ),
        (
            {
                "active_generation_id": ACTIVE_ID,
                "candidate_generation_id": None,
                "head_revision": 4,
            },
            ({"document_id": TARGET_ID},),
            None,
            "invalid result",
        ),
    ],
)
async def test_begin_structure_generation_rejects_incomplete_database_state(
    monkeypatch: pytest.MonkeyPatch,
    status: Mapping[str, Any] | None,
    documents: Sequence[Mapping[str, Any]],
    result: Mapping[str, Any] | None,
    match: str,
) -> None:
    cursors = [_Cursor(row=status)]
    if status is not None and status.get("candidate_generation_id") is None:
        cursors.append(_Cursor(rows=documents))
        if documents:
            cursors.append(_Cursor(row=result))
    connection = _Connection(cursors)
    _install_connection(monkeypatch, connection)

    with pytest.raises(GenerationOperatorError, match=match):
        await operator.begin_structure_generation("postgresql://operator@db/rag")


async def test_verify_structure_generation_returns_hash_bound_report(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    connection = _Connection(
        [
            _Cursor(
                row={
                    "candidate_generation_id": CANDIDATE_ID,
                    "candidate_chunk_profile_hash": (
                        SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
                    ),
                    "head_revision": 4,
                }
            ),
            _Cursor(
                row={
                    "artifact_manifest_hash": MANIFEST_HASH,
                    "block_count": 0,
                    "child_count": 3,
                    "document_count": 2,
                    "parent_count": 2,
                }
            ),
        ]
    )
    _install_connection(monkeypatch, connection)

    report = await operator.verify_structure_generation("postgresql://operator@db/rag")

    assert report["candidateGenerationId"] == str(CANDIDATE_ID)
    assert report["artifactManifestHash"] == MANIFEST_HASH
    assert report["promotionEligible"] is False
    digest = report.pop("verificationReportSha256")
    assert digest == operator.canonical_sha256(report)


@pytest.mark.parametrize(
    ("status", "verification", "match"),
    [
        (None, None, "no active head"),
        (
            {
                "candidate_generation_id": CANDIDATE_ID,
                "candidate_chunk_profile_hash": "c" * 64,
                "head_revision": 4,
            },
            None,
            "not the frozen SiliconFlow",
        ),
        (
            {
                "candidate_generation_id": CANDIDATE_ID,
                "candidate_chunk_profile_hash": (
                    SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
                ),
                "head_revision": 4,
            },
            None,
            "returned no result",
        ),
    ],
)
async def test_verify_structure_generation_fails_closed_on_drift(
    monkeypatch: pytest.MonkeyPatch,
    status: Mapping[str, Any] | None,
    verification: Mapping[str, Any] | None,
    match: str,
) -> None:
    cursors = [_Cursor(row=status)]
    if status is not None and status.get("candidate_chunk_profile_hash") == (
        SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
    ):
        cursors.append(_Cursor(row=verification))
    connection = _Connection(cursors)
    _install_connection(monkeypatch, connection)

    with pytest.raises(GenerationOperatorError, match=match):
        await operator.verify_structure_generation("postgresql://operator@db/rag")


async def test_mutation_gateways_forward_exact_authority_and_boolean_result(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    connection = _Connection(
        [
            _Cursor(row={"abandoned": True}),
            _Cursor(
                row={
                    "candidate_generation_id": CANDIDATE_ID,
                    "candidate_status": "verified",
                    "candidate_artifact_manifest_hash": MANIFEST_HASH,
                    "head_revision": 4,
                }
            ),
            _Cursor(row={"activated": True}),
            _Cursor(row={"rolled_back": False}),
        ]
    )
    _install_connection(monkeypatch, connection)

    assert await operator.abandon_structure_generation(
        "postgresql://operator@db/rag",
        candidate_generation_id=CANDIDATE_ID,
        expected_head_revision=4,
        expected_manifest_hash=MANIFEST_HASH,
        operator_id=OPERATOR_ID,
        reason="mixed worker image",
    )
    assert await operator.activate_structure_generation(
        "postgresql://operator@db/rag",
        gate_report={
            "candidateGenerationId": str(CANDIDATE_ID),
            "artifactManifestHash": MANIFEST_HASH,
        },
        gate_report_sha256=GATE_HASH,
        operator_id=OPERATOR_ID,
    )
    assert not await operator.rollback_structure_generation(
        "postgresql://operator@db/rag",
        active_generation_id=CANDIDATE_ID,
        target_generation_id=ACTIVE_ID,
        expected_head_revision=5,
        active_manifest_hash=MANIFEST_HASH,
        target_manifest_hash="c" * 64,
    )
    assert connection.calls[0][1] == (
        CANDIDATE_ID,
        4,
        MANIFEST_HASH,
        OPERATOR_ID,
        "mixed worker image",
    )
    assert connection.calls[2][1] == (
        CANDIDATE_ID,
        4,
        MANIFEST_HASH,
        GATE_HASH,
        OPERATOR_ID,
    )


@pytest.mark.parametrize(
    ("status", "match"),
    [
        (None, "no active head"),
        (
            {
                "candidate_generation_id": TARGET_ID,
                "candidate_status": "verified",
                "candidate_artifact_manifest_hash": MANIFEST_HASH,
                "head_revision": 4,
            },
            "candidate is not current",
        ),
        (
            {
                "candidate_generation_id": CANDIDATE_ID,
                "candidate_status": "building",
                "candidate_artifact_manifest_hash": MANIFEST_HASH,
                "head_revision": 4,
            },
            "candidate is not verified",
        ),
        (
            {
                "candidate_generation_id": CANDIDATE_ID,
                "candidate_status": "verified",
                "candidate_artifact_manifest_hash": "c" * 64,
                "head_revision": 4,
            },
            "manifest is stale",
        ),
    ],
)
async def test_activation_rejects_stale_or_unverified_status(
    monkeypatch: pytest.MonkeyPatch,
    status: Mapping[str, Any] | None,
    match: str,
) -> None:
    connection = _Connection([_Cursor(row=status)])
    _install_connection(monkeypatch, connection)

    with pytest.raises(GenerationOperatorError, match=match):
        await operator.activate_structure_generation(
            "postgresql://operator@db/rag",
            gate_report={
                "candidateGenerationId": str(CANDIDATE_ID),
                "artifactManifestHash": MANIFEST_HASH,
            },
            gate_report_sha256=GATE_HASH,
            operator_id=OPERATOR_ID,
        )


@pytest.mark.parametrize(
    ("call", "row", "key"),
    [
        (operator._required_uuid, {"value": None}, "value"),
        (operator._required_text, {"value": ""}, "value"),
        (operator._required_hash, {"value": "z" * 64}, "value"),
        (operator._required_positive_int, {"value": True}, "value"),
        (operator._required_positive_int, {"value": 0}, "value"),
        (operator._required_non_negative_int, {"value": -1}, "value"),
    ],
)
def test_required_database_values_reject_invalid_types_and_bounds(
    call: Callable[[Mapping[str, Any], str], object],
    row: Mapping[str, Any],
    key: str,
) -> None:
    with pytest.raises(GenerationOperatorError, match="database result value"):
        call(row, key)


def test_required_database_values_accept_canonical_values() -> None:
    assert operator._required_uuid({"value": str(CANDIDATE_ID)}, "value") == (
        CANDIDATE_ID
    )
    assert operator._required_uuid({"value": CANDIDATE_ID}, "value") == CANDIDATE_ID
    assert operator._required_text({"value": "ready"}, "value") == "ready"
    assert operator._required_hash({"value": MANIFEST_HASH.upper()}, "value") == (
        MANIFEST_HASH
    )
    assert operator._required_positive_int({"value": 1}, "value") == 1
    assert operator._required_non_negative_int({"value": 0}, "value") == 0
