from __future__ import annotations

import asyncio
import hashlib
import io
import json
import uuid
import zipfile
from typing import cast

import pytest

import mm_chat_rag.replay as replay_module
import mm_chat_rag.worker as worker_module
from mm_chat_rag.handlers import DispatchPlan, JobHandler, JobResult
from mm_chat_rag.job_context import ProcessingJobContext
from mm_chat_rag.job_handler_dependencies import DocumentSource
from mm_chat_rag.mineru_gateway import MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH
from mm_chat_rag.models import FunctionReadiness, JobClaim, OutboxClaim
from mm_chat_rag.projection import PostgresProjectionBatch
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.replay import run
from mm_chat_rag.settings import Settings
from mm_chat_rag.source_gateway import FileSourceMetadata
from mm_chat_rag.worker import (
    Worker,
    WorkerStartupError,
    build_promoted_job_handler_registry,
)

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"
MINERU_PDF_BODY = b"%PDF-1.7\nworker factory fixture\n%%EOF\n"

EVENT_ID = uuid.UUID("10000000-0000-0000-0000-000000000001")
JOB_ID = uuid.UUID("20000000-0000-0000-0000-000000000002")
SUCCESSOR_ID = uuid.UUID("30000000-0000-0000-0000-000000000003")
OPERATOR_ID = uuid.UUID("40000000-0000-0000-0000-000000000004")


async def test_replay_is_dry_run_by_default(capsys: pytest.CaptureFixture[str]) -> None:
    result = await run(
        ["outbox", "--id", str(EVENT_ID), "--expected-error-code", "FAILED_EVENT"]
    )
    assert result == 0
    output = json.loads(capsys.readouterr().out)
    assert output == {
        "kind": "outbox",
        "id": str(EVENT_ID),
        "expected_error_code": "FAILED_EVENT",
        "mode": "dry-run",
    }


async def test_execute_outbox_calls_operator_function(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    captured: dict[str, object] = {}

    async def fake_replay(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return True

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator:secret@db/rag")
    monkeypatch.setattr(replay_module, "replay_outbox", fake_replay)
    result = await run(
        [
            "outbox",
            "--id",
            str(EVENT_ID),
            "--expected-error-code",
            "FAILED_EVENT",
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "incident approved",
            "--execute",
        ]
    )
    assert result == 0
    assert captured["event_id"] == EVENT_ID
    assert captured["operator_id"] == OPERATOR_ID
    assert "secret" not in capsys.readouterr().out


async def test_execute_job_requires_and_passes_successor(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured: dict[str, object] = {}

    async def fake_replay(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return False

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator@db/rag")
    monkeypatch.setattr(replay_module, "replay_job", fake_replay)
    result = await run(
        [
            "job",
            "--id",
            str(JOB_ID),
            "--expected-error-code",
            "FAILED_JOB",
            "--successor-job-id",
            str(SUCCESSOR_ID),
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "approved",
            "--execute",
        ]
    )
    assert result == 1
    assert captured["successor_job_id"] == SUCCESSOR_ID


@pytest.mark.parametrize(
    "arguments",
    [
        ["outbox", "--id", str(EVENT_ID), "--expected-error-code", "bad text"],
        [
            "outbox",
            "--id",
            str(EVENT_ID),
            "--expected-error-code",
            "FAILED_EVENT",
            "--execute",
        ],
        [
            "job",
            "--id",
            str(JOB_ID),
            "--expected-error-code",
            "FAILED_JOB",
            "--operator-id",
            str(OPERATOR_ID),
            "--reason",
            "approved",
            "--execute",
        ],
    ],
)
async def test_replay_invalid_or_incomplete_inputs_fail_closed(
    arguments: list[str],
) -> None:
    with pytest.raises(SystemExit, match="2"):
        await run(arguments)


def event_planner(_: OutboxClaim) -> DispatchPlan:
    return DispatchPlan("global", None, "dispatch", {"synthetic_action": "apply"})


async def job_handler(_: JobClaim) -> JobResult:
    return JobResult()


def provider_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def test_worker_default_is_dark_and_enabled_empty_registry_is_rejected() -> None:
    dark = Worker(Settings(database_url="postgresql://test"))
    dark.validate_promotion_gate()
    enabled = Worker(Settings(database_url="postgresql://test", dispatch_enabled=True))
    with pytest.raises(WorkerStartupError, match="empty event registry"):
        enabled.validate_promotion_gate()


def test_worker_rejects_missing_stage_handler_and_accepts_synthetic_gate() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    with pytest.raises(WorkerStartupError, match="no promoted handler"):
        Worker(
            settings,
            dispatch_registry={"synthetic": event_planner},
            job_handlers={},
        ).validate_promotion_gate()
    Worker(
        settings,
        dispatch_registry={"synthetic": event_planner},
        job_handlers={"parse": cast("JobHandler", job_handler)},
    ).validate_promotion_gate()


def test_worker_accepts_job_only_promotion_without_outbox_registry() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("purge",),
    )
    worker = Worker(settings, job_handlers={"purge": cast("JobHandler", job_handler)})

    worker.validate_promotion_gate()

    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_accepts_outbox_only_promotion_without_job_stages() -> None:
    settings = Settings(database_url="postgresql://test", dispatch_enabled=True)
    worker = Worker(settings, dispatch_registry={"synthetic": event_planner})

    worker.validate_promotion_gate()

    assert worker.job_handlers == {}
    assert worker.state.consumer == "ready"


def test_worker_auto_promotes_only_purge_stage_from_settings() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("purge",),
    )
    worker = Worker(settings)

    worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"purge"}
    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_auto_promotes_passage_embedding_stage_from_settings() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("passage_embedding",),
        jina_api_key="fake-jina-key",
        provider_profile=provider_profile(),
    )
    worker = Worker(settings)

    worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"passage_embedding"}
    assert worker.dispatch_registry == {}
    assert worker.state.consumer == "disabled"


def test_worker_auto_promotes_parse_stage_from_settings(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    mineru_tokens: list[str | None] = []

    class FakeMinerULocalBatchGateway:
        def __init__(
            self,
            api_token: str | None,
            *,
            result_proxy_url: str | None = None,
        ) -> None:
            assert result_proxy_url is None
            mineru_tokens.append(api_token)

    monkeypatch.setattr(
        worker_module,
        "MinerULocalBatchGateway",
        FakeMinerULocalBatchGateway,
    )
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse", "passage_embedding", "purge"),
        mineru_api_key="fake-mineru-token",
        jina_api_key="fake-jina-key",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )
    worker = Worker(settings)

    worker.validate_promotion_gate()

    assert set(worker.job_handlers) == {"parse", "passage_embedding", "purge"}
    assert mineru_tokens == ["fake-mineru-token"]


class FakeParseSourceMetadataGateway:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls

    async def fetch_source_metadata(
        self,
        context: ProcessingJobContext,
    ) -> FileSourceMetadata:
        self._calls.append("metadata")
        return FileSourceMetadata(
            file_id=context.file_id,
            storage_backend="minio",
            object_key=f"knowledge/{context.file_id}.pdf",
            sha256=hashlib.sha256(MINERU_PDF_BODY).hexdigest(),
            byte_size=len(MINERU_PDF_BODY),
            content_type="application/pdf",
        )


class FakeParseChunkProfileGateway:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls

    async def resolve_parse_chunk_profile(self, context: ProcessingJobContext) -> str:
        _ = context
        self._calls.append("profile")
        return MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH


class FakeSourceObjectGateway:
    def __init__(
        self,
        *,
        calls: list[str],
        base_url: str,
        internal_token: str,
        worker_id: uuid.UUID,
    ) -> None:
        self._calls = calls
        assert base_url == "http://backend:8080"
        assert internal_token == SOURCE_GATEWAY_TOKEN
        assert worker_id != uuid.UUID(int=0)

    async def fetch_object_bytes(
        self,
        context: ProcessingJobContext,
        metadata: FileSourceMetadata,
    ) -> bytes:
        self._calls.append("object")
        assert metadata.file_id == context.file_id
        return MINERU_PDF_BODY


class FakeMinerUArchiveProvider:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls

    async def fetch_result_archive(
        self,
        context: ProcessingJobContext,
        source: DocumentSource,
    ) -> bytes:
        self._calls.append("archive")
        assert context.stage == "parse"
        assert source.content_type == "application/pdf"
        return mineru_archive("Worker factory parse\n\nMinerU baseline")


class FakeParseProjectionGateway:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls
        self.batches: list[PostgresProjectionBatch] = []
        self.embedding_job_ids: list[uuid.UUID] = []

    async def stage_parse_projection(
        self,
        context: ProcessingJobContext,
        batch: PostgresProjectionBatch,
    ) -> None:
        self._calls.append("stage")
        assert context.stage == "parse"
        self.batches.append(batch)

    async def complete_parse_and_enqueue_embedding(
        self,
        context: ProcessingJobContext,
        *,
        embedding_job_id: uuid.UUID,
    ) -> bool:
        self._calls.append("complete_parse")
        assert context.stage == "parse"
        assert embedding_job_id != uuid.UUID(int=0)
        self.embedding_job_ids.append(embedding_job_id)
        return True


def mineru_archive(text: str) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr("full.md", text.encode())
        archive.writestr("fixture_content_list.json", b'[{"type":"text"}]')
        archive.writestr("layout.json", b'{"pages":[{"page":0}]}')
        archive.writestr("fixture_model.json", b'{"model":"vlm"}')
    return output.getvalue()


def parse_claim() -> JobClaim:
    job_id = uuid.uuid4()
    return JobClaim(
        job_id=job_id,
        stage="parse",
        attempt_count=1,
        max_attempts=3,
        values={
            "id": job_id,
            "collection_id": uuid.uuid4(),
            "document_id": uuid.uuid4(),
            "document_version_id": uuid.uuid4(),
            "file_id": uuid.uuid4(),
            "stage": "parse",
            "operation": "initial",
            "processor": "mineru",
            "endpoint_id": "admin-env",
            "model_id": "mineru-parser-v20260716",
            "governance_profile_id": uuid.uuid4(),
            "governance_revision": 1,
            "governance_head_revision": 1,
            "collection_consent_id": uuid.uuid4(),
            "collection_consent_revision": 1,
            "collection_acl_revision": 1,
            "collection_visibility_epoch": 1,
            "collection_processing_revision": 1,
            "document_visibility_epoch": 1,
            "request_hash": hashlib.sha256(str(job_id).encode()).hexdigest(),
            "attempt_count": 1,
            "max_attempts": 3,
            "lease_token": uuid.uuid4(),
            "index_generation_id": uuid.uuid4(),
            "materialization_id": uuid.uuid4(),
            "legacy_projection_unbound": False,
        },
    )


async def test_worker_factory_promotes_parse_when_dependencies_are_supplied(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []
    projection = FakeParseProjectionGateway(calls)

    def fake_source_object_gateway(
        *,
        base_url: str,
        internal_token: str,
        worker_id: uuid.UUID,
    ) -> FakeSourceObjectGateway:
        return FakeSourceObjectGateway(
            calls=calls,
            base_url=base_url,
            internal_token=internal_token,
            worker_id=worker_id,
        )

    monkeypatch.setattr(
        worker_module,
        "GoSourceObjectBytesGateway",
        fake_source_object_gateway,
    )
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        job_stages=("parse",),
        mineru_api_key="fake-mineru-token",
        source_gateway_url="http://backend:8080",
        source_gateway_token=SOURCE_GATEWAY_TOKEN,
        provider_profile=provider_profile(),
    )

    registry = build_promoted_job_handler_registry(
        settings,
        parse_source_metadata=FakeParseSourceMetadataGateway(calls),
        parse_chunk_profiles=FakeParseChunkProfileGateway(calls),
        parse_projection=projection,
        parse_archive_provider=FakeMinerUArchiveProvider(calls),
        passage_embedding_projection=cast("object", object()),
        purge_projection=cast("object", object()),
    )
    result = await registry["parse"](parse_claim())

    assert result.outcome == "succeeded"
    assert result.terminal_committed is True
    assert calls == [
        "metadata",
        "object",
        "profile",
        "archive",
        "stage",
        "complete_parse",
    ]
    assert len(projection.batches) == 1
    assert len(projection.embedding_job_ids) == 1
    assert projection.batches[0].parent_chunks[0].content == (
        "Worker factory parse\n\nMinerU baseline"
    )
    assert (
        projection.batches[0].source_sha256
        == hashlib.sha256(MINERU_PDF_BODY).hexdigest()
    )


async def test_worker_readiness_refresh_preserves_dark_run_consumer() -> None:
    worker = Worker(Settings(database_url="postgresql://test"))

    class ReadyDatabase:
        async def readiness(self) -> FunctionReadiness:
            return FunctionReadiness(
                database=True,
                functions=True,
                consumer="ready",
                projection="not_ready",
            )

    worker.database = ReadyDatabase()  # type: ignore[assignment]
    await worker._refresh_readiness()
    assert worker.state.database == "ready"
    assert worker.state.functions == "ready"
    assert worker.state.consumer == "disabled"


async def test_worker_readiness_failure_is_bounded() -> None:
    worker = Worker(Settings(database_url="postgresql://test"))

    class FailedDatabase:
        async def readiness(self) -> FunctionReadiness:
            raise RuntimeError("postgresql://user:secret@db")

    worker.database = FailedDatabase()  # type: ignore[assignment]
    await worker._refresh_readiness()
    assert worker.state.database == "not_ready"
    assert worker.state.functions == "not_ready"


class LifecycleDatabase:
    def __init__(self, acquired: bool = True) -> None:
        self.acquired = acquired
        self.opened = False
        self.closed = False
        self.claim_count = 0

    async def open(self) -> None:
        self.opened = True

    async def acquire_worker_lock(self) -> bool:
        return self.acquired

    async def readiness(self) -> FunctionReadiness:
        return FunctionReadiness(
            database=True,
            functions=True,
            consumer="disabled",
            projection="not_ready",
        )

    async def close(self) -> None:
        self.closed = True

    async def claim_outbox(self, lock_token: uuid.UUID) -> None:
        self.claim_count += 1


async def test_dark_run_lifecycle_never_claims() -> None:
    worker = Worker(Settings(database_url="postgresql://test", health_port=0))
    database = LifecycleDatabase()
    worker.database = database  # type: ignore[assignment]
    task = asyncio.create_task(worker.run())
    for _ in range(50):
        if worker.state.worker_lock == "ready":
            break
        await asyncio.sleep(0.01)
    worker.request_stop()
    await task
    assert database.opened
    assert database.closed
    assert worker.state.consumer == "disabled"


async def test_lock_contention_exits_before_services() -> None:
    worker = Worker(Settings(database_url="postgresql://test", health_port=0))
    database = LifecycleDatabase(acquired=False)
    worker.database = database  # type: ignore[assignment]
    with pytest.raises(WorkerStartupError, match="already held"):
        await worker.run()
    assert database.closed


async def test_promoted_synthetic_lifecycle_polls_without_real_stages() -> None:
    settings = Settings(
        database_url="postgresql://test",
        dispatch_enabled=True,
        poll_interval_seconds=0.005,  # type: ignore[arg-type]
        health_port=0,
    )
    worker = Worker(settings, dispatch_registry={"synthetic.created": event_planner})
    database = LifecycleDatabase()
    worker.database = database  # type: ignore[assignment]
    task = asyncio.create_task(worker.run())
    for _ in range(50):
        if database.claim_count:
            break
        await asyncio.sleep(0.01)
    worker.request_stop()
    await task
    assert database.claim_count > 0
