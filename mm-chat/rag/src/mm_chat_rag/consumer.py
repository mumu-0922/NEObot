"""Durable outbox poll, forced rescan, plan, ledger, and ack orchestration."""

from __future__ import annotations

import asyncio
import logging
import time
import uuid
from collections.abc import Mapping
from typing import Protocol

from mm_chat_rag.handlers import DispatchPlanner
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import OutboxClaim
from mm_chat_rag.retry import retry_or_dlq
from mm_chat_rag.settings import Settings

logger = logging.getLogger(__name__)


class OutboxDatabase(Protocol):
    """The stored-function-only surface used by the consumer."""

    async def claim_outbox(self, lock_token: uuid.UUID) -> OutboxClaim | None: ...

    async def apply_and_ack_outbox(
        self,
        claim: OutboxClaim,
        lock_token: uuid.UUID,
        *,
        scope_kind: str,
        index_generation_id: uuid.UUID | None,
        action: str,
        result_hash: str,
    ) -> bool: ...

    async def retry_outbox(
        self,
        claim: OutboxClaim,
        lock_token: uuid.UUID,
        error_code: str,
        retry_after_seconds: int,
    ) -> bool: ...

    async def fail_outbox(
        self, claim: OutboxClaim, lock_token: uuid.UUID, error_code: str
    ) -> bool: ...


class DurableConsumer:
    """Consume without checkpoints; forced scans rely on DB ledger anti-join."""

    def __init__(
        self,
        database: OutboxDatabase,
        settings: Settings,
        metrics: Metrics,
        registry: Mapping[str, DispatchPlanner],
    ) -> None:
        self._database = database
        self._settings = settings
        self._metrics = metrics
        self._registry = registry

    async def run(self, wake: asyncio.Event, stop: asyncio.Event) -> None:
        """Poll every second and force an exhaustive scan every 30 seconds."""
        last_rescan = time.monotonic()
        while not stop.is_set():
            woke = False
            try:
                await asyncio.wait_for(
                    wake.wait(), timeout=self._settings.poll_interval_seconds
                )
                woke = True
                wake.clear()
            except TimeoutError:
                pass

            force = (
                time.monotonic() - last_rescan >= self._settings.forced_rescan_seconds
            )
            kind = "rescan" if force else ("wake" if woke else "poll")
            self._metrics.loop_iterations.labels(kind=kind).inc()
            limit = self._settings.max_rescan_claims if force else 1
            for _ in range(limit):
                if stop.is_set():
                    break
                claimed = await self.process_one()
                if not claimed:
                    break
            if force:
                last_rescan = time.monotonic()

    async def process_one(self) -> bool:
        """Claim and resolve one event with a never-reused lock token."""
        lock_token = uuid.uuid4()
        claim = await self._database.claim_outbox(lock_token)
        if claim is None:
            self._metrics.outbox_claims.labels(outcome="empty").inc()
            return False
        self._metrics.outbox_claims.labels(outcome="claimed").inc()
        planner = self._registry.get(claim.event_type)
        if planner is None:
            await self._resolve_failure(
                claim, lock_token, error_code="DISPATCH_EVENT_UNSUPPORTED"
            )
            return True
        try:
            plan = planner(claim)
            applied = await self._database.apply_and_ack_outbox(
                claim,
                lock_token,
                scope_kind=plan.scope_kind,
                index_generation_id=plan.index_generation_id,
                action=plan.action,
                result_hash=plan.result_hash(),
            )
        except (TypeError, ValueError):
            await self._resolve_failure(
                claim, lock_token, error_code="DISPATCH_PLAN_INVALID"
            )
            return True
        outcome = "applied" if applied else "stale_lease"
        self._metrics.outbox_results.labels(outcome=outcome).inc()
        logger.info(
            "outbox_resolved",
            extra={"fields": {"outcome": outcome, "event_type": claim.event_type}},
        )
        return True

    async def _resolve_failure(
        self, claim: OutboxClaim, lock_token: uuid.UUID, *, error_code: str
    ) -> None:
        decision = retry_or_dlq(
            attempt_count=claim.attempt_count,
            max_attempts=claim.max_attempts,
            error_code=error_code,
            retry_after_seconds=30,
        )
        if decision.outcome == "failed":
            changed = await self._database.fail_outbox(
                claim, lock_token, decision.error_code
            )
        else:
            changed = await self._database.retry_outbox(
                claim,
                lock_token,
                decision.error_code,
                decision.retry_after_seconds,
            )
        outcome = decision.outcome if changed else "stale_lease"
        self._metrics.outbox_results.labels(outcome=outcome).inc()
        logger.warning(
            "outbox_plan_rejected",
            extra={"fields": {"outcome": outcome, "error_code": error_code}},
        )
