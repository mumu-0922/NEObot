"""Read-only RAG query candidate contracts.

G7.6 keeps Python search as an untrusted candidate generator.  Candidates carry
only citation references and ranking metadata; Go must reauthorize and hydrate
content before answer generation.
"""

from __future__ import annotations

import re
import uuid
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any, Final, NoReturn

from mm_chat_rag.models import stable_error_code
from mm_chat_rag.retry import PermanentJobError

QUERY_CANDIDATE_INVALID: Final = "QUERY_CANDIDATE_INVALID"
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_ZERO_UUID: Final = uuid.UUID(int=0)


@dataclass(frozen=True, slots=True)
class EvidenceCandidate:
    """A citation reference returned by private selected-collection search."""

    collection_id: uuid.UUID
    document_id: uuid.UUID
    document_version_id: uuid.UUID
    index_generation_id: uuid.UUID
    materialization_id: uuid.UUID
    parent_chunk_id: uuid.UUID
    child_chunk_id: uuid.UUID
    source_span_hash: str
    content_hash: str
    rank_score: float

    def __post_init__(self) -> None:
        if _ZERO_UUID in {
            self.collection_id,
            self.document_id,
            self.document_version_id,
            self.index_generation_id,
            self.materialization_id,
            self.parent_chunk_id,
            self.child_chunk_id,
        }:
            _reject()
        if not _SHA256_RE.fullmatch(self.source_span_hash):
            _reject()
        if not _SHA256_RE.fullmatch(self.content_hash):
            _reject()
        if isinstance(self.rank_score, bool) or self.rank_score < 0:
            _reject()

    @classmethod
    def from_row(cls, row: Mapping[str, Any]) -> EvidenceCandidate:
        """Decode a DB candidate row without accepting body text."""
        try:
            rank_score = row["rank_score"]
            if isinstance(rank_score, bool):
                _reject()
            return cls(
                collection_id=_uuid(row["collection_id"]),
                document_id=_uuid(row["document_id"]),
                document_version_id=_uuid(row["document_version_id"]),
                index_generation_id=_uuid(row["index_generation_id"]),
                materialization_id=_uuid(row["materialization_id"]),
                parent_chunk_id=_uuid(row["parent_chunk_id"]),
                child_chunk_id=_uuid(row["child_chunk_id"]),
                source_span_hash=str(row["source_span_hash"]),
                content_hash=str(row["content_hash"]),
                rank_score=float(rank_score),
            )
        except (KeyError, TypeError, ValueError):
            _reject()


def _uuid(value: object) -> uuid.UUID:
    if isinstance(value, uuid.UUID):
        return value
    return uuid.UUID(str(value))


def _reject() -> NoReturn:
    raise PermanentJobError(stable_error_code(QUERY_CANDIDATE_INVALID))
