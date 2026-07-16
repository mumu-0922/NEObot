"""Phase B dispatch and job registries.

The production registries are intentionally empty. Phase 15.2C must add real
parse, embedding, or purge handlers behind its own promotion gate.
"""

from __future__ import annotations

import hashlib
import json
import re
import uuid
from collections.abc import Callable, Coroutine, Mapping
from dataclasses import dataclass, field
from typing import Any, Final, Protocol

from mm_chat_rag.job_context import ProcessingJobContext, admit_processing_job_context
from mm_chat_rag.models import JobClaim, OutboxClaim
from mm_chat_rag.provider_profile import ProviderRuntimeProfile

_HASH_RE: Final = re.compile(r"^[0-9a-f]{64}$")
ALLOWED_DISPATCH_ACTIONS: Final[frozenset[str]] = frozenset(
    {"collection_purge", "dispatch", "noop", "generation_reconstruct"}
)


def _canonical(value: object) -> object:
    if value is None or isinstance(value, (str, bool, int)):
        return value
    if isinstance(value, uuid.UUID):
        return str(value)
    if isinstance(value, Mapping):
        return {str(key): _canonical(value[key]) for key in sorted(value)}
    if isinstance(value, (list, tuple)):
        return [_canonical(item) for item in value]
    raise TypeError(f"unsupported dispatch plan value: {type(value).__name__}")


@dataclass(frozen=True, slots=True)
class DispatchPlan:
    """Deterministic, external-I/O-free outbox application plan."""

    scope_kind: str
    index_generation_id: uuid.UUID | None
    action: str
    parameters: Mapping[str, object] = field(default_factory=dict)
    contract_version: int = 1

    def __post_init__(self) -> None:
        if self.scope_kind not in {"global", "generation"}:
            raise ValueError("scope_kind must be global or generation")
        if (self.scope_kind == "generation") != (self.index_generation_id is not None):
            raise ValueError("generation scope must have exactly one generation id")
        if (
            not isinstance(self.action, str)
            or self.action not in ALLOWED_DISPATCH_ACTIONS
        ):
            raise ValueError("action is invalid")
        if self.contract_version != 1:
            raise ValueError("unsupported dispatch plan contract version")
        _canonical(self.parameters)

    def result_hash(self) -> str:
        """Hash versioned canonical JSON; never hash PostgreSQL jsonb text."""
        envelope = {
            "action": self.action,
            "contract_version": self.contract_version,
            "index_generation_id": self.index_generation_id,
            "parameters": self.parameters,
            "scope_kind": self.scope_kind,
        }
        encoded = json.dumps(
            _canonical(envelope),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode()
        result = hashlib.sha256(encoded).hexdigest()
        if not _HASH_RE.fullmatch(result):  # pragma: no cover - hashlib invariant
            raise AssertionError("sha256 returned an invalid digest")
        return result


DispatchPlanner = Callable[[OutboxClaim], DispatchPlan]


@dataclass(frozen=True, slots=True)
class JobResult:
    """Successful synthetic handler completion."""

    outcome: str = "succeeded"
    error_code: str | None = None
    terminal_committed: bool = False

    def __post_init__(self) -> None:
        """Keep normal returns success-only; failures use typed exceptions."""
        if self.outcome != "succeeded" or self.error_code is not None:
            raise ValueError("job handler results must be successful")
        if not isinstance(self.terminal_committed, bool):
            raise TypeError("terminal_committed must be a boolean")


class JobHandler(Protocol):
    """A promoted stage implementation. Phase B has no implementations."""

    def __call__(self, job: JobClaim) -> Coroutine[Any, Any, JobResult]:
        """Execute one leased job outside any database transaction."""
        ...


class JobContextHandler(Protocol):
    """A handler admitted through the typed processing-job context seam."""

    def __call__(self, context: ProcessingJobContext) -> Coroutine[Any, Any, JobResult]:
        """Execute one admitted job without reading raw claim values."""
        ...


def with_job_context_admission(
    handler: JobContextHandler,
    *,
    provider_profile: ProviderRuntimeProfile | None = None,
) -> JobHandler:
    """Wrap a context handler with fail-closed claim-row admission."""

    async def admitted(job: JobClaim) -> JobResult:
        context = admit_processing_job_context(job, provider_profile=provider_profile)
        return await handler(context)

    return admitted


# Frozen Phase B safety boundary: production cannot claim any real work.
DISPATCH_REGISTRY: Final[Mapping[str, DispatchPlanner]] = {}
JOB_HANDLER_REGISTRY: Final[Mapping[str, JobHandler]] = {}
