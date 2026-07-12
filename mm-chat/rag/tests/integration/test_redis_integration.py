from __future__ import annotations

import asyncio
import os

import pytest
import redis.asyncio as redis

from mm_chat_rag.metrics import Metrics
from mm_chat_rag.redis_wakeup import RedisWakeSubscriber

pytestmark = pytest.mark.integration


def redis_url() -> str:
    value = os.environ.get("RAG_TEST_REDIS_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_REDIS_URL is not set")
    return value


async def test_only_constant_payload_wakes_consumer() -> None:
    url = redis_url()
    channel = "mm-chat-test:rag:outbox:v1"
    metrics = Metrics.create()
    wake = asyncio.Event()
    stop = asyncio.Event()
    subscriber = asyncio.create_task(
        RedisWakeSubscriber(url, channel, metrics).run(wake, stop)
    )
    client = redis.from_url(url, decode_responses=False)  # type: ignore[no-untyped-call]
    try:
        for _ in range(50):
            if metrics.redis_connected._value.get() == 1:
                break
            await asyncio.sleep(0.02)
        await client.publish(channel, b"not-authoritative")
        await asyncio.sleep(0.05)
        assert not wake.is_set()
        await client.publish(channel, b"1")
        await asyncio.wait_for(wake.wait(), timeout=2)
    finally:
        stop.set()
        await subscriber
        await client.aclose()
