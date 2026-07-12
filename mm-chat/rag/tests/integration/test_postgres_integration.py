from __future__ import annotations

import os

import pytest

from mm_chat_rag.metrics import Metrics
from mm_chat_rag.postgres import PostgresAdapter
from mm_chat_rag.settings import Settings

pytestmark = pytest.mark.integration


def database_url() -> str:
    value = os.environ.get("RAG_TEST_DATABASE_URL", "").strip()
    if not value:
        pytest.skip("RAG_TEST_DATABASE_URL is not set")
    return value


async def test_frozen_readiness_and_single_worker_lock() -> None:
    settings = Settings(database_url=database_url())
    first = PostgresAdapter(settings, Metrics.create())
    second = PostgresAdapter(settings, Metrics.create())
    await first.open()
    await second.open()
    try:
        assert await first.acquire_worker_lock()
        assert not await second.acquire_worker_lock()
        readiness = await first.readiness()
        assert readiness.database
        assert readiness.consumer in {"ready", "disabled", "not_ready"}
    finally:
        await second.close()
        await first.close()
