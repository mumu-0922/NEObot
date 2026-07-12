"""Lossy Redis wake hints; Postgres polling remains authoritative."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Callable

import redis.asyncio as redis
from redis.exceptions import RedisError

from mm_chat_rag.metrics import Metrics

logger = logging.getLogger(__name__)


class RedisWakeSubscriber:
    """Subscribe to one constant-payload channel and set an in-process event."""

    def __init__(
        self,
        url: str,
        channel: str,
        metrics: Metrics,
        status_callback: Callable[[str], None] | None = None,
    ) -> None:
        self._url = url
        self._channel = channel
        self._metrics = metrics
        self._status_callback = status_callback

    def _set_status(self, connected: bool) -> None:
        self._metrics.redis_connected.set(int(connected))
        if self._status_callback is not None:
            self._status_callback("ready" if connected else "degraded")

    async def run(self, wake: asyncio.Event, stop: asyncio.Event) -> None:
        """Reconnect forever; Redis failure never stops Postgres polling."""
        delay = 1
        while not stop.is_set():
            client = redis.from_url(  # type: ignore[no-untyped-call]
                self._url,
                decode_responses=False,
                socket_connect_timeout=2,
                socket_timeout=2,
                health_check_interval=15,
            )
            try:
                async with client.pubsub(ignore_subscribe_messages=True) as pubsub:
                    await pubsub.subscribe(self._channel)
                    self._set_status(connected=True)
                    delay = 1
                    while not stop.is_set():
                        message = await pubsub.get_message(timeout=1)
                        if (
                            message
                            and message.get("type") == "message"
                            and message.get("data") == b"1"
                        ):
                            wake.set()
            except (RedisError, OSError):
                self._set_status(connected=False)
                logger.warning("redis_wakeup_degraded")
                try:
                    await asyncio.wait_for(stop.wait(), timeout=delay)
                except TimeoutError:
                    delay = min(delay * 2, 30)
            finally:
                self._set_status(connected=False)
                await client.aclose()
