from __future__ import annotations

import asyncio

import pytest
from redis.exceptions import RedisError

import mm_chat_rag.redis_wakeup as wakeup_module
from mm_chat_rag.metrics import Metrics
from mm_chat_rag.redis_wakeup import RedisWakeSubscriber


class FakePubSub:
    def __init__(
        self,
        messages: list[dict[str, object] | None],
        error: Exception | None = None,
    ) -> None:
        self.messages = messages
        self.error = error
        self.channels: list[str] = []

    async def __aenter__(self) -> FakePubSub:
        return self

    async def __aexit__(self, *_: object) -> None:
        return None

    async def subscribe(self, channel: str) -> None:
        if self.error:
            raise self.error
        self.channels.append(channel)

    async def get_message(self, *, timeout: int) -> dict[str, object] | None:
        await asyncio.sleep(0)
        return self.messages.pop(0) if self.messages else None


class FakeRedis:
    def __init__(self, pubsub: FakePubSub) -> None:
        self.fake_pubsub = pubsub
        self.closed = False

    def pubsub(self, *, ignore_subscribe_messages: bool) -> FakePubSub:
        assert ignore_subscribe_messages
        return self.fake_pubsub

    async def aclose(self) -> None:
        self.closed = True


async def test_constant_payload_sets_wake(monkeypatch: pytest.MonkeyPatch) -> None:
    pubsub = FakePubSub(
        [
            {"type": "message", "data": b"bad"},
            {"type": "other", "data": b"1"},
            {"type": "message", "data": b"1"},
        ]
    )
    client = FakeRedis(pubsub)

    def from_url(*_: object, **__: object) -> FakeRedis:
        return client

    monkeypatch.setattr(wakeup_module.redis, "from_url", from_url)
    wake = asyncio.Event()
    stop = asyncio.Event()
    statuses: list[str] = []
    task = asyncio.create_task(
        RedisWakeSubscriber(
            "redis://secret@redis",
            "test:channel",
            Metrics.create(),
            statuses.append,
        ).run(wake, stop)
    )
    await asyncio.wait_for(wake.wait(), timeout=1)
    stop.set()
    await task
    assert pubsub.channels == ["test:channel"]
    assert client.closed
    assert "ready" in statuses
    assert statuses[-1] == "degraded"


async def test_redis_error_is_degraded_and_retried(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    clients: list[FakeRedis] = []

    def from_url(*_: object, **__: object) -> FakeRedis:
        client = FakeRedis(FakePubSub([], RedisError("down")))
        clients.append(client)
        return client

    monkeypatch.setattr(wakeup_module.redis, "from_url", from_url)
    stop = asyncio.Event()
    task = asyncio.create_task(
        RedisWakeSubscriber("redis://redis", "test:channel", Metrics.create()).run(
            asyncio.Event(), stop
        )
    )
    await asyncio.sleep(0.02)
    stop.set()
    await task
    assert clients[0].closed
