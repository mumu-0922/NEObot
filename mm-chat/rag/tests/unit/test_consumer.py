from __future__ import annotations

import asyncio
import uuid

from mm_chat_rag.consumer import DurableConsumer
from mm_chat_rag.handlers import DispatchPlan
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.models import OutboxClaim
from mm_chat_rag.settings import Settings


class FakeOutboxDatabase:
    def __init__(self, claims: list[OutboxClaim | None]) -> None:
        self.claims = claims
        self.applied: list[tuple[uuid.UUID, str]] = []
        self.retried: list[str] = []
        self.failed: list[str] = []

    async def claim_outbox(self, lock_token: uuid.UUID) -> OutboxClaim | None:
        return self.claims.pop(0) if self.claims else None

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
        self.applied.append((lock_token, result_hash))
        return True

    async def retry_outbox(
        self,
        claim: OutboxClaim,
        lock_token: uuid.UUID,
        error_code: str,
        retry_after_seconds: int,
    ) -> bool:
        self.retried.append(error_code)
        return True

    async def fail_outbox(
        self, claim: OutboxClaim, lock_token: uuid.UUID, error_code: str
    ) -> bool:
        self.failed.append(error_code)
        return True


def claim(
    event_type: str = "synthetic.created", attempt: int = 1, maximum: int = 3
) -> OutboxClaim:
    return OutboxClaim(
        outbox_id=1,
        event_id=uuid.uuid4(),
        event_type=event_type,
        attempt_count=attempt,
        max_attempts=maximum,
        values={},
    )


def settings() -> Settings:
    return Settings(database_url="postgresql://test", dispatch_enabled=True)


def planner(_: OutboxClaim) -> DispatchPlan:
    return DispatchPlan(
        "global",
        None,
        "dispatch",
        {"synthetic_action": "apply", "version": 1},
    )


async def test_apply_uses_fresh_token_and_deterministic_hash() -> None:
    database = FakeOutboxDatabase([claim(), claim()])
    consumer = DurableConsumer(
        database, settings(), Metrics.create(), {"synthetic.created": planner}
    )
    assert await consumer.process_one()
    assert await consumer.process_one()
    assert database.applied[0][0] != database.applied[1][0]
    assert database.applied[0][1] == database.applied[1][1]
    assert len(database.applied[0][1]) == 64


async def test_empty_claim_returns_false() -> None:
    consumer = DurableConsumer(
        FakeOutboxDatabase([None]), settings(), Metrics.create(), {}
    )
    assert await consumer.process_one() is False


async def test_unknown_event_retries_then_enters_dlq() -> None:
    retry_db = FakeOutboxDatabase([claim("unknown", 1, 2)])
    retry_consumer = DurableConsumer(retry_db, settings(), Metrics.create(), {})
    assert await retry_consumer.process_one()
    assert retry_db.retried == ["DISPATCH_EVENT_UNSUPPORTED"]
    failed_db = FakeOutboxDatabase([claim("unknown", 2, 2)])
    failed_consumer = DurableConsumer(failed_db, settings(), Metrics.create(), {})
    assert await failed_consumer.process_one()
    assert failed_db.failed == ["DISPATCH_EVENT_UNSUPPORTED"]


async def test_invalid_plan_is_never_applied() -> None:
    def invalid(_: OutboxClaim) -> DispatchPlan:
        return DispatchPlan("global", None, "synthetic.apply")

    database = FakeOutboxDatabase([claim()])
    consumer = DurableConsumer(
        database,
        settings(),
        Metrics.create(),
        {"synthetic.created": invalid},
    )
    assert await consumer.process_one()
    assert not database.applied
    assert database.retried == ["DISPATCH_PLAN_INVALID"]


async def test_poll_wake_and_forced_rescan_loop() -> None:
    database = FakeOutboxDatabase([claim(), None, claim(), None])
    fast = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        poll_interval_seconds=0.005,  # type: ignore[arg-type]
        forced_rescan_seconds=0.01,  # type: ignore[arg-type]
    )
    metrics = Metrics.create()
    consumer = DurableConsumer(database, fast, metrics, {"synthetic.created": planner})
    wake = asyncio.Event()
    stop = asyncio.Event()
    task = asyncio.create_task(consumer.run(wake, stop))
    wake.set()
    await asyncio.sleep(0.04)
    stop.set()
    await task
    assert len(database.applied) == 2
    samples = metrics.loop_iterations.collect()[0].samples
    kinds = {
        sample.labels["kind"] for sample in samples if sample.name.endswith("total")
    }
    assert {"wake", "poll", "rescan"} <= kinds
