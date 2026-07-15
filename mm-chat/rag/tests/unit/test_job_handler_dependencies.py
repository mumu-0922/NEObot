from __future__ import annotations

import copy
import uuid
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import pytest

from mm_chat_rag.job_handler_dependencies import (
    JOB_HANDLER_DEPENDENCY_ERROR_CODES,
    JOB_HANDLER_DEPENDENCY_UNCONFIGURED,
    JOB_HANDLER_PARSE_ARTIFACT_INVALID,
    JOB_HANDLER_SOURCE_HASH_MISMATCH,
    DocumentSource,
    ParsedDocumentArtifacts,
    ParseHandlerDependencies,
    admitted_parse_handler_with_dependencies,
    parse_handler_with_dependencies,
)
from mm_chat_rag.job_handlers import JOB_HANDLER_STAGE_MISMATCH
from mm_chat_rag.models import JobClaim, stable_error_code
from mm_chat_rag.projection import PostgresProjectionBatch
from mm_chat_rag.provider_profile import (
    MINERU_JINA_POSTGRES_PROFILE,
    ProviderRuntimeProfile,
)
from mm_chat_rag.retry import PermanentJobError
from tests.support.parser_contracts import JsonObject, load_strict_json_bytes

HASH = "c" * 64
SOURCE_SHA256 = "3a413cf18e813c868e5859350b4a6e02fe271e2bf4224b92eb14cf3829cb9a9e"
_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "hash_dag"
)


def valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def provider_row(**updates: object) -> dict[str, object]:
    row: dict[str, object] = {
        "id": uuid.uuid4(),
        "stage": "parse",
        "operation": "initial",
        "collection_id": uuid.uuid4(),
        "document_id": uuid.uuid4(),
        "document_version_id": uuid.uuid4(),
        "file_id": uuid.uuid4(),
        "index_generation_id": uuid.uuid4(),
        "materialization_id": uuid.uuid4(),
        "processor": "mineru",
        "endpoint_id": "hosted",
        "model_id": "mineru",
        "governance_profile_id": uuid.uuid4(),
        "governance_revision": 1,
        "governance_head_revision": 1,
        "collection_consent_id": uuid.uuid4(),
        "collection_consent_revision": 1,
        "collection_acl_revision": 1,
        "collection_visibility_epoch": 1,
        "collection_processing_revision": 1,
        "document_visibility_epoch": 1,
        "attempt_count": 1,
        "max_attempts": 3,
        "request_hash": HASH,
        "legacy_projection_unbound": False,
    }
    row.update(updates)
    return row


def purge_row(**updates: object) -> dict[str, object]:
    row = provider_row(
        stage="purge",
        operation="purge",
        materialization_id=None,
        processor=None,
        endpoint_id=None,
        model_id=None,
        governance_profile_id=None,
        governance_revision=None,
        governance_head_revision=None,
        collection_consent_id=None,
        collection_consent_revision=None,
    )
    row.update(updates)
    return row


def claim(row: Mapping[str, Any]) -> JobClaim:
    return JobClaim.from_row(row)


def fixture(name: str) -> JsonObject:
    value = load_strict_json_bytes((_FIXTURE_ROOT / name).read_bytes())
    assert isinstance(value, dict)
    return value


class FakeDocumentSourceGateway:
    def __init__(self, calls: list[str], source_sha256: str = SOURCE_SHA256) -> None:
        self._calls = calls
        self._source_sha256 = source_sha256

    async def fetch_document_source(self, context: object) -> DocumentSource:
        self._calls.append("fetch")
        assert context is not None
        return DocumentSource(
            body=b"%PDF-1.7 test fixture",
            source_sha256=self._source_sha256,
            content_type="application/pdf",
        )


class FakeParserGateway:
    def __init__(
        self,
        calls: list[str],
        *,
        chunk_source_sha256: str | None = None,
    ) -> None:
        self._calls = calls
        self._chunk_source_sha256 = chunk_source_sha256

    async def parse_document(
        self, context: object, source: DocumentSource
    ) -> ParsedDocumentArtifacts:
        self._calls.append("parse")
        assert context is not None
        assert source.content_type == "application/pdf"
        canonical_ir = fixture("canonical-ir.v2.json")
        chunk_manifest = fixture("chunk-manifest.v2.json")
        if self._chunk_source_sha256 is not None:
            chunk_manifest = copy.deepcopy(chunk_manifest)
            chunk_manifest["sourceSha256"] = self._chunk_source_sha256
        return ParsedDocumentArtifacts(
            artifact_set_id=uuid.uuid4(),
            canonical_ir=canonical_ir,
            chunk_manifest=chunk_manifest,
        )


class FakeParseProjectionGateway:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls
        self.batches: list[PostgresProjectionBatch] = []

    async def stage_parse_projection(
        self, context: object, batch: PostgresProjectionBatch
    ) -> None:
        self._calls.append("stage")
        assert context is not None
        self.batches.append(batch)


def dependencies(
    calls: list[str],
    *,
    source_sha256: str = SOURCE_SHA256,
    chunk_source_sha256: str | None = None,
) -> tuple[ParseHandlerDependencies, FakeParseProjectionGateway]:
    projection = FakeParseProjectionGateway(calls)
    return (
        ParseHandlerDependencies(
            document_source=FakeDocumentSourceGateway(calls, source_sha256),
            parser=FakeParserGateway(
                calls,
                chunk_source_sha256=chunk_source_sha256,
            ),
            projection=projection,
        ),
        projection,
    )


def test_parse_dependency_error_codes_are_stable() -> None:
    for code in JOB_HANDLER_DEPENDENCY_ERROR_CODES:
        assert stable_error_code(code) == code


async def test_admitted_parse_dependency_handler_stages_projection_with_fakes() -> None:
    calls: list[str] = []
    deps, projection = dependencies(calls)
    handler = admitted_parse_handler_with_dependencies(deps, valid_profile())

    result = await handler(claim(provider_row()))

    assert result.outcome == "succeeded"
    assert result.error_code is None
    assert calls == ["fetch", "parse", "stage"]
    assert len(projection.batches) == 1
    batch = projection.batches[0]
    assert len(batch.blocks) == 2
    assert len(batch.parent_chunks) == 1
    assert len(batch.child_chunks) == 1
    assert batch.source_sha256 == SOURCE_SHA256


async def test_parse_dependency_handler_default_off_before_external_calls() -> None:
    context = admitted_parse_handler_with_dependencies(
        ParseHandlerDependencies(),
        valid_profile(),
    )

    with pytest.raises(PermanentJobError) as raised:
        await context(claim(provider_row()))

    assert raised.value.error_code == JOB_HANDLER_DEPENDENCY_UNCONFIGURED


async def test_parse_dependency_handler_rejects_stage_before_dependencies() -> None:
    calls: list[str] = []
    deps, _ = dependencies(calls)
    handler = admitted_parse_handler_with_dependencies(deps, valid_profile())

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(purge_row()))

    assert raised.value.error_code == JOB_HANDLER_STAGE_MISMATCH
    assert calls == []


async def test_parse_dependency_handler_rejects_source_mismatch_before_stage() -> None:
    calls: list[str] = []
    deps, projection = dependencies(calls, source_sha256="0" * 64)
    handler = admitted_parse_handler_with_dependencies(deps, valid_profile())

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row()))

    assert raised.value.error_code == JOB_HANDLER_SOURCE_HASH_MISMATCH
    assert calls == ["fetch", "parse"]
    assert projection.batches == []


async def test_parse_dependency_handler_redacts_projection_errors() -> None:
    calls: list[str] = []
    deps, projection = dependencies(calls, chunk_source_sha256="1" * 64)
    handler = admitted_parse_handler_with_dependencies(deps, valid_profile())

    with pytest.raises(PermanentJobError) as raised:
        await handler(claim(provider_row()))

    assert raised.value.error_code == JOB_HANDLER_PARSE_ARTIFACT_INVALID
    assert str(raised.value) == JOB_HANDLER_PARSE_ARTIFACT_INVALID
    assert calls == ["fetch", "parse"]
    assert projection.batches == []


async def test_parse_dependency_contextual_handler_after_admission() -> None:
    calls: list[str] = []
    deps, projection = dependencies(calls)
    handler = admitted_parse_handler_with_dependencies(deps, valid_profile())
    admitted = await handler(claim(provider_row()))

    assert admitted.outcome == "succeeded"
    search = projection.batches[0].child_search_projections[0]
    assert search.embedding_dimensions == 1024

    with pytest.raises(PermanentJobError) as raised:
        await parse_handler_with_dependencies(
            object(),  # type: ignore[arg-type]
            deps,
        )
    assert raised.value.error_code == "JOB_HANDLER_CONTEXT_INVALID"
