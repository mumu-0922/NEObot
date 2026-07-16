from __future__ import annotations

import uuid

import pytest

from mm_chat_rag.query import QUERY_CANDIDATE_INVALID, EvidenceCandidate
from mm_chat_rag.retry import PermanentJobError


def candidate_row(**updates: object) -> dict[str, object]:
    row: dict[str, object] = {
        "collection_id": uuid.uuid4(),
        "document_id": uuid.uuid4(),
        "document_version_id": uuid.uuid4(),
        "index_generation_id": uuid.uuid4(),
        "materialization_id": uuid.uuid4(),
        "parent_chunk_id": uuid.uuid4(),
        "child_chunk_id": uuid.uuid4(),
        "source_span_hash": "a" * 64,
        "content_hash": "b" * 64,
        "rank_score": 0.5,
    }
    row.update(updates)
    return row


def test_evidence_candidate_decodes_reference_only_row() -> None:
    row = candidate_row()

    candidate = EvidenceCandidate.from_row(row)

    assert candidate.collection_id == row["collection_id"]
    assert candidate.source_span_hash == "a" * 64
    assert candidate.content_hash == "b" * 64
    assert candidate.rank_score == 0.5


@pytest.mark.parametrize(
    "updates",
    [
        {"collection_id": uuid.UUID(int=0)},
        {"source_span_hash": "not-a-hash"},
        {"content_hash": "c" * 63},
        {"rank_score": -0.1},
        {"rank_score": True},
    ],
)
def test_evidence_candidate_rejects_invalid_reference_rows(
    updates: dict[str, object],
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        EvidenceCandidate.from_row(candidate_row(**updates))

    assert raised.value.error_code == QUERY_CANDIDATE_INVALID
