from __future__ import annotations

import hashlib
import inspect
import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import httpx
import pytest

from tests.support.fake_provider import FakeProvider
from tests.support.provider_contracts import (
    ContractValidationError,
    ProviderContract,
    load_provider_contract,
    parse_provider_contract,
    validate_provider_contract,
)

_FIXTURES = (
    "mineru-public-draft.json",
    "jina-embedding-v4-1024-public-draft.json",
    "jina-embedding-v4-2048-public-draft.json",
    "jina-rerank-v3-public-draft.json",
)
_NOW = datetime(2026, 7, 13, tzinfo=UTC)


def _raw(fixture_name: str) -> dict[str, Any]:
    return load_provider_contract(fixture_name).mutable_copy()


@pytest.mark.parametrize("fixture_name", _FIXTURES)
def test_public_contract_fixtures_are_valid_but_not_frozen(
    fixture_name: str,
) -> None:
    contract = load_provider_contract(fixture_name)
    assert contract.lifecycle_state == "draft"
    assert contract.raw["lifecycle"]["blockedBy"]
    assert contract.raw["unresolved"]
    assert len(contract.wire_contract_hash()) == 64
    assert len(contract.terms_snapshot_hash()) == 64
    assert len(contract.fixture_set_hash()) == 64
    with pytest.raises(ContractValidationError, match="not frozen"):
        contract.require_frozen(b"draft", {})


def test_contract_is_deeply_immutable_and_mutable_copy_is_detached() -> None:
    raw = _raw("mineru-public-draft.json")
    contract = validate_provider_contract(raw)
    raw["fixtureSetId"] = "mutated-after-validation"
    assert contract.fixture_set_id != raw["fixtureSetId"]
    with pytest.raises(TypeError):
        contract.raw["fixtureSetId"] = "mutation"  # type: ignore[index]
    assert isinstance(contract.raw["operations"], tuple)
    detached = contract.mutable_copy()
    detached["identity"]["provider"]["value"] = "detached"
    assert contract.raw["identity"]["provider"]["value"] == "mineru"


def test_schema_is_fixed_and_cannot_be_overridden() -> None:
    assert "schema" not in inspect.signature(validate_provider_contract).parameters
    extra = _raw("mineru-public-draft.json")
    extra["unexpected"] = True
    with pytest.raises(ContractValidationError, match="schema"):
        validate_provider_contract(extra)


def test_hash_boundaries_change_only_for_owned_data() -> None:
    original = load_provider_contract("mineru-public-draft.json")

    identity = original.mutable_copy()
    identity["identity"]["provider"]["value"] = "mineru-cloud"
    identity_contract = validate_provider_contract(identity)
    assert identity_contract.wire_contract_hash() != original.wire_contract_hash()
    assert identity_contract.terms_snapshot_hash() == original.terms_snapshot_hash()
    assert identity_contract.fixture_set_hash() == original.fixture_set_hash()

    terms = original.mutable_copy()
    terms["governanceTerms"]["license"] = {
        "state": "unknown",
        "reasonCode": "LICENSE_REVIEW_PENDING",
    }
    terms_contract = validate_provider_contract(terms)
    assert terms_contract.wire_contract_hash() == original.wire_contract_hash()
    assert terms_contract.terms_snapshot_hash() != original.terms_snapshot_hash()
    assert terms_contract.fixture_set_hash() == original.fixture_set_hash()


def test_jina_dimension_candidates_are_separate_and_full_width() -> None:
    vectors: dict[int, tuple[float, ...]] = {}
    hashes: set[str] = set()
    for dimensions in (1024, 2048):
        contract = load_provider_contract(
            f"jina-embedding-v4-{dimensions}-public-draft.json"
        )
        request = contract.operation("embed").request
        assert request is not None
        assert request["body"]["json"]["dimensions"] == dimensions
        response = contract.operation("embed").response_cases[0]
        vector = response.body["json"]["data"][0]["embedding"]
        assert isinstance(vector, tuple)
        assert len(vector) == dimensions
        vectors[dimensions] = vector
        hashes.add(contract.wire_contract_hash())
    assert len(hashes) == 2
    assert vectors[1024] != vectors[2048]


def test_mineru_recovery_surface_marks_unpublished_operations_unknown() -> None:
    contract = load_provider_contract("mineru-public-draft.json")
    assert {operation.phase for operation in contract.operations} == {
        "submit",
        "poll",
        "result",
        "cancel",
        "query_by_key",
    }
    assert contract.operation("cancel").support_state == "unknown"
    assert contract.operation("query_by_key").support_state == "unknown"


async def test_fake_provider_replays_fixture_without_network_or_secret_capture() -> (
    None
):
    contract = load_provider_contract("mineru-public-draft.json")
    fake = FakeProvider(contract, {"submit": "submit-success"})
    operation = contract.operation("submit")
    assert operation.request is not None
    request_json = contract.mutable_copy()["operations"][0]["request"]["body"]["json"]
    transport = httpx.ASGITransport(app=fake.app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://provider.invalid",
    ) as client:
        response = await client.post(
            "/api/v4/extract/task",
            headers={"Authorization": "test-only"},
            json=request_json,
        )
    assert response.status_code == 200
    assert response.headers["x-mm-chat-fixture-case"] == "submit-success"
    assert response.json()["data"]["task_id"] == "fixture-task-id"
    assert len(fake.calls) == 1
    assert fake.calls[0].operation_id == "submit"
    assert fake.calls[0].path == operation.path_template
    assert "authorization" in fake.calls[0].header_names
    assert not hasattr(fake.calls[0], "headers")
    assert "application/json" not in repr(fake.calls[0])
    assert len(fake.calls[0].body_sha256) == 64


async def test_fake_provider_rejects_request_drift_without_recording_body() -> None:
    contract = load_provider_contract("mineru-public-draft.json")
    fake = FakeProvider(contract, {"submit": "submit-success"})
    transport = httpx.ASGITransport(app=fake.app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://provider.invalid",
    ) as client:
        response = await client.post(
            "/api/v4/extract/task",
            headers={"Authorization": "test-only"},
            json={"url": "https://different.invalid/file.pdf"},
        )
    assert response.status_code == 400
    assert response.json() == {"error": "FIXTURE_BODY_MISMATCH"}
    assert fake.calls == []


async def test_fake_provider_records_template_not_dynamic_task_id() -> None:
    contract = load_provider_contract("mineru-public-draft.json")
    fake = FakeProvider(contract, {"poll": "poll-running"})
    transport = httpx.ASGITransport(app=fake.app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://provider.invalid",
    ) as client:
        response = await client.get(
            "/api/v4/extract/task/sensitive-dynamic-task-id",
            headers={"Authorization": "test-only"},
        )
    assert response.status_code == 200
    assert fake.calls[0].path == "/api/v4/extract/task/{task_id}"
    assert "sensitive-dynamic-task-id" not in repr(fake.calls[0])


async def test_fake_provider_rejects_wrong_content_type_value() -> None:
    contract = load_provider_contract("mineru-public-draft.json")
    fake = FakeProvider(contract, {"submit": "submit-success"})
    request_json = contract.mutable_copy()["operations"][0]["request"]["body"]["json"]
    transport = httpx.ASGITransport(app=fake.app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://provider.invalid",
    ) as client:
        response = await client.post(
            "/api/v4/extract/task",
            headers={
                "Authorization": "test-only",
                "Content-Type": "text/plain; charset=utf-8",
            },
            content=json.dumps(request_json, separators=(",", ":")).encode(),
        )
    assert response.status_code == 400
    assert response.json() == {"error": "FIXTURE_CONTENT_TYPE_MISMATCH"}
    assert fake.calls == []


@pytest.mark.parametrize(
    "payload",
    [
        '{"schemaVersion":1,"schemaVersion":1}',
        '{"schemaVersion":NaN}',
        '{"schemaVersion":1e999}',
        '{"schemaVersion":9007199254740992}',
        '{"schemaVersion":1}\u0000',
    ],
)
def test_strict_json_rejects_duplicate_non_finite_unsafe_integer_and_nul(
    payload: str,
) -> None:
    with pytest.raises(ContractValidationError):
        parse_provider_contract(payload)


@pytest.mark.parametrize(
    "value",
    [9007199254740992, float("inf"), "\ud800"],
)
def test_direct_validation_rejects_non_jcs_values(value: object) -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["responseCases"][0]["body"]["json"]["value"] = value
    with pytest.raises(ContractValidationError, match="JCS|finite|Unicode"):
        validate_provider_contract(raw)


@pytest.mark.parametrize(
    "url",
    [
        " https://download.invalid/a?signature=fixture",
        "//download.invalid/a?signature=fixture",
        "//例子.测试/path",
    ],
)
def test_url_like_values_reject_whitespace_and_scheme_relative_forms(url: str) -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["responseCases"][0]["body"]["json"]["location"] = url
    with pytest.raises(ContractValidationError, match="URL"):
        validate_provider_contract(raw)


@pytest.mark.parametrize(
    "message",
    [
        "download: https://download.invalid/a?signature=fixture",
        "prefix //download.invalid/a?signature=fixture",
        "prefix //例子.测试/path",
    ],
)
def test_embedded_url_tokens_are_rejected(message: str) -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["responseCases"][0]["body"]["json"]["msg"] = message
    with pytest.raises(ContractValidationError, match="embedded"):
        validate_provider_contract(raw)


def test_direct_credential_free_idn_https_url_is_allowed() -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["request"]["body"]["json"]["url"] = "https://例子.测试/path"
    validate_provider_contract(raw)


@pytest.mark.parametrize(
    "field",
    ["signature", "x-amz-signature", "access_key", "authorization_hash"],
)
def test_policy_and_secret_like_fields_are_rejected(field: str) -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["responseCases"][0]["body"]["json"][field] = (
        "fixture-sensitive-value"
    )
    with pytest.raises(ContractValidationError, match="secret-like field"):
        validate_provider_contract(raw)


@pytest.mark.parametrize(
    "path",
    ["//evil", "/%2e%2e/private", "/safe\\..\\private", "/../private"],
)
def test_operation_paths_reject_authority_encoding_and_traversal(path: str) -> None:
    raw = _raw("mineru-public-draft.json")
    raw["operations"][0]["pathTemplate"] = path
    with pytest.raises(ContractValidationError, match="path"):
        validate_provider_contract(raw)


def test_placeholder_secret_and_any_http_url_fail_closed() -> None:
    placeholder = _raw("mineru-public-draft.json")
    placeholder["operations"][0]["request"]["body"]["json"]["model_version"] = "v1"
    with pytest.raises(ContractValidationError, match="placeholder"):
        validate_provider_contract(placeholder)

    secret_field = _raw("mineru-public-draft.json")
    secret_field["operations"][0]["responseCases"][0]["body"]["json"]["auth_token"] = (
        "redacted"
    )
    with pytest.raises(ContractValidationError, match="secret-like field"):
        validate_provider_contract(secret_field)

    secret_value = _raw("mineru-public-draft.json")
    secret_value["operations"][0]["responseCases"][0]["body"]["json"]["opaque"] = (
        "sk-" + "fixturesecretvalue"
    )
    with pytest.raises(ContractValidationError, match="secret-like value"):
        validate_provider_contract(secret_value)

    insecure_url = _raw("mineru-public-draft.json")
    insecure_url["operations"][0]["responseCases"][0]["body"]["json"]["location"] = (
        "http://provider.invalid/result"
    )
    with pytest.raises(ContractValidationError, match="credential-free HTTPS"):
        validate_provider_contract(insecure_url)

    signed_href = _raw("mineru-public-draft.json")
    signed_href["operations"][0]["responseCases"][0]["body"]["json"]["href"] = (
        "https://provider.invalid/result?signature=fixture"
    )
    with pytest.raises(ContractValidationError, match="credential-free HTTPS"):
        validate_provider_contract(signed_href)


def test_fact_values_use_closed_capability_term_and_mime_shapes() -> None:
    mime = _raw("mineru-public-draft.json")
    mime["identity"]["allowedDataTypes"]["value"] = ["/"]
    with pytest.raises(ContractValidationError, match="MIME"):
        validate_provider_contract(mime)

    capability = _raw("jina-rerank-v3-public-draft.json")
    capability["capabilities"]["idempotency"] = {
        "state": "observed",
        "value": {"x": "y"},
        "evidenceRefs": ["jina-openapi"],
    }
    with pytest.raises(ContractValidationError, match="idempotency"):
        validate_provider_contract(capability)

    term = _raw("jina-rerank-v3-public-draft.json")
    term["governanceTerms"]["license"] = {
        "state": "terms_verified",
        "value": {"status": "reviewed"},
        "evidenceRefs": ["jina-openapi"],
        "reviewedAt": "2026-07-12T00:00:00Z",
    }
    with pytest.raises(ContractValidationError, match="license"):
        validate_provider_contract(term)


def test_signed_urls_utf8_body_limit_and_fixture_path_traversal_are_rejected() -> None:
    signed = _raw("mineru-public-draft.json")
    result = signed["operations"][2]["responseCases"][0]["body"]["json"]
    result["data"]["full_zip_url"] = (
        "https://download.invalid/result.zip?X-Amz-Signature=fixture"
    )
    with pytest.raises(ContractValidationError, match="credential-free HTTPS"):
        validate_provider_contract(signed)

    oversized = _raw("mineru-public-draft.json")
    oversized["redactionPolicy"]["maximumRecordedBodyBytes"] = 16
    with pytest.raises(ContractValidationError, match="redaction cap"):
        validate_provider_contract(oversized)

    with pytest.raises(ContractValidationError, match="allowlisted"):
        load_provider_contract("../private.json")


@pytest.mark.parametrize(
    ("mutation", "message"),
    [
        ("response_model", "model mismatch"),
        ("index", "indexes are incomplete"),
        ("dimension", "dimension mismatch"),
        ("usage", "non-negative integers"),
    ],
)
def test_jina_embedding_shape_mutations_fail_closed(
    mutation: str, message: str
) -> None:
    raw = _raw("jina-embedding-v4-1024-public-draft.json")
    response = raw["operations"][0]["responseCases"][0]["body"]["json"]
    if mutation == "response_model":
        response["model"] = "different-model"
    elif mutation == "index":
        response["data"][0]["index"] = 1
    elif mutation == "dimension":
        response["data"][0]["embedding"].pop()
    else:
        response["usage"]["total_tokens"] = -1
    with pytest.raises(ContractValidationError, match=message):
        validate_provider_contract(raw)


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("task", "retrieval.query"),
        ("embedding_type", "binary"),
        ("truncate", True),
        ("late_chunking", True),
        ("return_multivector", True),
        ("return_tokenized_input", True),
        ("dimensions", 512),
    ],
)
def test_jina_passage_request_semantics_are_fixed(field: str, value: object) -> None:
    raw = _raw("jina-embedding-v4-1024-public-draft.json")
    raw["operations"][0]["request"]["body"]["json"][field] = value
    with pytest.raises(ContractValidationError, match="passage|dimension"):
        validate_provider_contract(raw)


def test_embedding_capability_dimension_must_match_request() -> None:
    raw = _raw("jina-embedding-v4-1024-public-draft.json")
    raw["capabilities"]["embedding"] = {
        "state": "observed",
        "value": {
            "dimensions": 2048,
            "normalized": True,
            "maxBatchItems": 8,
            "maxBatchTokens": 8192,
            "maxBatchBytes": 1048576,
        },
        "evidenceRefs": ["jina-openapi"],
    }
    with pytest.raises(ContractValidationError, match="dimension mismatch"):
        validate_provider_contract(raw)


def test_rerank_response_requires_official_model_field() -> None:
    raw = _raw("jina-rerank-v3-public-draft.json")
    raw["operations"][0]["responseCases"][0]["body"]["json"].pop("model")
    with pytest.raises(ContractValidationError, match="invalid shape"):
        validate_provider_contract(raw)


def test_response_classification_must_match_http_status() -> None:
    raw = _raw("jina-embedding-v4-1024-public-draft.json")
    raw["operations"][0]["responseCases"][1]["status"] = 200
    with pytest.raises(ContractValidationError, match="contradicts"):
        validate_provider_contract(raw)


def test_mineru_shape_and_observed_success_case_fail_closed() -> None:
    state = _raw("mineru-public-draft.json")
    state["operations"][1]["responseCases"][0]["body"]["json"]["data"]["state"] = (
        "invented"
    )
    with pytest.raises(ContractValidationError, match="poll state"):
        validate_provider_contract(state)

    no_success = _raw("mineru-public-draft.json")
    no_success["operations"][0]["responseCases"][0]["classification"] = "retryable"
    no_success["operations"][0]["responseCases"][0]["status"] = 429
    no_success["operations"][0]["responseCases"][0]["stableErrorCode"] = (
        "PROVIDER_RETRY"
    )
    with pytest.raises(ContractValidationError, match="success case"):
        validate_provider_contract(no_success)

    request = _raw("mineru-public-draft.json")
    request["operations"][0]["request"]["body"]["json"]["is_ocr"] = "yes"
    with pytest.raises(ContractValidationError, match="boolean"):
        validate_provider_contract(request)

    start_time = _raw("mineru-public-draft.json")
    progress = start_time["operations"][1]["responseCases"][0]["body"]["json"]["data"][
        "extract_progress"
    ]
    progress["start_time"] = "not-a-provider-time"
    with pytest.raises(ContractValidationError, match="start_time"):
        validate_provider_contract(start_time)

    failed = _raw("mineru-public-draft.json")
    failed_data = failed["operations"][1]["responseCases"][1]["body"]["json"]["data"]
    failed_data["err_msg"] = ""
    with pytest.raises(ContractValidationError, match="non-empty text"):
        validate_provider_contract(failed)


def test_operation_phase_identity_and_non_empty_response_fail_closed() -> None:
    duplicate_phase = _raw("jina-embedding-v4-1024-public-draft.json")
    duplicate = duplicate_phase["operations"][0].copy()
    duplicate["operationId"] = "embed_copy"
    duplicate_phase["operations"].append(duplicate)
    with pytest.raises(ContractValidationError, match="phases|equal its phase"):
        validate_provider_contract(duplicate_phase)

    empty_response = _raw("jina-embedding-v4-1024-public-draft.json")
    empty_response["operations"][0]["responseCases"] = []
    with pytest.raises(ContractValidationError, match="schema|success case"):
        validate_provider_contract(empty_response)


def _frozen_contract() -> tuple[ProviderContract, bytes, dict[str, bytes]]:
    raw = _raw("jina-rerank-v3-public-draft.json")
    wire_evidence = raw["evidence"][0]["evidenceId"]
    terms_evidence = "jina-reviewed-terms"
    capture_evidence = "jina-redacted-capture"
    raw["evidence"].extend(
        [
            {
                "evidenceId": terms_evidence,
                "sourceUrl": "https://jina.ai/legal/terms",
                "observedAt": "2026-07-12T00:00:00Z",
                "sourceKind": "reviewed_terms",
                "sourceVersion": "review-20260712",
            },
            {
                "evidenceId": capture_evidence,
                "sourceUrl": "https://api.jina.ai/v1/rerank",
                "observedAt": "2026-07-12T00:00:00Z",
                "sourceKind": "redacted_capture",
                "sourceVersion": "capture-20260712",
            },
        ]
    )
    snapshots = {
        evidence["evidenceId"]: (
            f"exact evidence snapshot: {evidence['evidenceId']}\n"
        ).encode()
        for evidence in raw["evidence"]
    }
    for evidence in raw["evidence"]:
        evidence["contentHash"] = hashlib.sha256(
            snapshots[evidence["evidenceId"]]
        ).hexdigest()
        evidence["validUntil"] = "2027-07-12T00:00:00Z"

    identity_values: dict[str, object] = {
        "endpointId": "hosted-main",
        "region": "global-processing",
        "modelId": "jina-reranker-v3",
        "immutableBuildId": "jina-rerank-build-20260712",
    }
    for field, value in identity_values.items():
        raw["identity"][field] = {
            "state": "observed",
            "value": value,
            "evidenceRefs": [wire_evidence],
        }
    raw["capabilities"]["idempotency"] = {
        "state": "observed",
        "value": {
            "supported": False,
            "mechanism": "none",
            "replayWindowSeconds": 0,
        },
        "evidenceRefs": [wire_evidence],
    }
    raw["capabilities"]["rateLimits"] = {
        "state": "observed",
        "value": {
            "requestsPerMinute": 60,
            "tokensPerMinute": 100000,
            "concurrency": 2,
            "scope": "account",
        },
        "evidenceRefs": [wire_evidence],
    }
    term_values: dict[str, dict[str, object]] = {
        "license": {
            "identifier": "commercial-api-terms",
            "commercialUseAllowed": True,
            "sourceVersion": "2026-07-12",
        },
        "retention": {"maximumSeconds": 0, "scope": "request"},
        "deletion": {
            "supported": True,
            "maximumCompletionSeconds": 86400,
            "mechanism": "support-request",
        },
        "trainingUse": {"allowed": False, "scope": "api-input"},
        "sla": {"availabilityBasisPoints": 9900, "supportTier": "standard"},
    }
    for field, value in term_values.items():
        raw["governanceTerms"][field] = {
            "state": "terms_verified",
            "value": value,
            "evidenceRefs": [terms_evidence],
            "reviewedAt": "2026-07-12T00:00:00Z",
        }

    operation = raw["operations"][0]
    success = operation["responseCases"][0]
    success["source"] = "redacted_capture"
    success["evidenceRefs"] = [capture_evidence]
    for case_id, status, classification, error_code in (
        ("rerank-retryable", 429, "retryable", "PROVIDER_RATE_LIMITED"),
        ("rerank-permanent", 400, "permanent", "PROVIDER_REQUEST_REJECTED"),
    ):
        operation["responseCases"].append(
            {
                "caseId": case_id,
                "source": "redacted_capture",
                "status": status,
                "headers": {"Content-Type": "application/json"},
                "body": {"json": {"error": {"code": error_code}}},
                "classification": classification,
                "stableErrorCode": error_code,
                "evidenceRefs": [capture_evidence],
            }
        )
    assert success["classification"] == "success"
    raw["lifecycle"] = {
        "state": "frozen",
        "blockedBy": [],
        "reviewers": ["engineering", "governance_security"],
        "frozenAt": "2026-07-12T00:00:00Z",
    }
    raw["unresolved"] = []
    raw.pop("integrity", None)
    candidate = validate_provider_contract(raw)
    freeze_report = b"reviewed freeze report v1"
    raw["integrity"] = {
        "wireContractHash": candidate.wire_contract_hash(),
        "termsSnapshotHash": candidate.terms_snapshot_hash(),
        "fixtureSetHash": candidate.fixture_set_hash(),
        "freezeReportHash": hashlib.sha256(freeze_report).hexdigest(),
    }
    return validate_provider_contract(raw), freeze_report, snapshots


def test_frozen_contract_revalidates_hash_report_evidence_and_terms() -> None:
    contract, freeze_report, snapshots = _frozen_contract()
    contract.require_frozen(freeze_report, snapshots, now=_NOW)

    with pytest.raises(ContractValidationError, match="integrity mismatch"):
        contract.require_frozen(b"tampered report", snapshots, now=_NOW)

    mismatched_snapshots = dict(snapshots)
    mismatched_snapshots["jina-reviewed-terms"] = b"tampered terms snapshot"
    with pytest.raises(ContractValidationError, match="content hash mismatch"):
        contract.require_frozen(freeze_report, mismatched_snapshots, now=_NOW)

    stale = contract.mutable_copy()
    stale["evidence"][0]["validUntil"] = "2026-07-12T12:00:00Z"
    stale_candidate = validate_provider_contract(stale)
    stale["integrity"]["termsSnapshotHash"] = stale_candidate.terms_snapshot_hash()
    stale["integrity"]["fixtureSetHash"] = stale_candidate.fixture_set_hash()
    stale_contract = validate_provider_contract(stale)
    with pytest.raises(ContractValidationError, match="stale"):
        stale_contract.require_frozen(freeze_report, snapshots, now=_NOW)

    future = contract.mutable_copy()
    future["observedAt"] = "2027-07-12T00:00:00Z"
    future_candidate = validate_provider_contract(future)
    future["integrity"]["wireContractHash"] = future_candidate.wire_contract_hash()
    future["integrity"]["termsSnapshotHash"] = future_candidate.terms_snapshot_hash()
    future["integrity"]["fixtureSetHash"] = future_candidate.fixture_set_hash()
    future_contract = validate_provider_contract(future)
    with pytest.raises(ContractValidationError, match="future-dated"):
        future_contract.require_frozen(freeze_report, snapshots, now=_NOW)

    wrong_source = contract.mutable_copy()
    wrong_source["evidence"][1]["sourceKind"] = "official_docs"
    wrong_candidate = validate_provider_contract(wrong_source)
    wrong_source["integrity"]["termsSnapshotHash"] = (
        wrong_candidate.terms_snapshot_hash()
    )
    wrong_source["integrity"]["fixtureSetHash"] = wrong_candidate.fixture_set_hash()
    wrong_contract = validate_provider_contract(wrong_source)
    with pytest.raises(ContractValidationError, match="reviewed terms"):
        wrong_contract.require_frozen(freeze_report, snapshots, now=_NOW)


def test_frozen_model_capture_and_behavior_coverage_cannot_be_faked() -> None:
    contract, _, _ = _frozen_contract()

    model_mismatch = contract.mutable_copy()
    model_mismatch["identity"]["modelId"]["value"] = "other-reranker-v9"
    with pytest.raises(ContractValidationError, match="identity/request model"):
        validate_provider_contract(model_mismatch)

    wrong_provider_capability = contract.mutable_copy()
    wrong_provider_capability["capabilities"]["rateLimits"]["value"] = {
        "maximumFileBytes": 1,
        "maximumPages": 1,
        "maximumBatchItemsFromDetailedSection": 1,
        "batchLimitConflictPresent": False,
    }
    with pytest.raises(ContractValidationError, match="rateLimits"):
        validate_provider_contract(wrong_provider_capability)

    wrong_capability_state = contract.mutable_copy()
    wrong_capability_state["capabilities"]["idempotency"] = {
        "state": "not_applicable",
        "reasonCode": "INCORRECTLY_NOT_APPLICABLE",
    }
    with pytest.raises(ContractValidationError, match="not_applicable"):
        validate_provider_contract(wrong_capability_state)

    synthetic_only = contract.mutable_copy()
    for response in synthetic_only["operations"][0]["responseCases"][1:]:
        response["source"] = "synthetic_test"
        response.pop("evidenceRefs", None)
    with pytest.raises(ContractValidationError, match="coverage"):
        validate_provider_contract(synthetic_only)

    missing_capture_evidence = contract.mutable_copy()
    missing_capture_evidence["operations"][0]["responseCases"][1].pop("evidenceRefs")
    with pytest.raises(ContractValidationError, match="lacks evidence"):
        validate_provider_contract(missing_capture_evidence)


def test_require_frozen_rejects_schema_mutation_and_incomplete_classification() -> None:
    contract, freeze_report, snapshots = _frozen_contract()
    incomplete = contract.mutable_copy()
    incomplete["operations"][0]["responseCases"] = [
        incomplete["operations"][0]["responseCases"][0]
    ]
    with pytest.raises(ContractValidationError, match="coverage"):
        validate_provider_contract(incomplete)

    invalid = contract.mutable_copy()
    invalid["unexpected"] = True
    with pytest.raises(ContractValidationError, match="schema"):
        validate_provider_contract(invalid)
    contract.require_frozen(freeze_report, snapshots, now=_NOW)


def test_mineru_governance_example_is_non_executable() -> None:
    repository = Path(__file__).parents[3]
    blocked_path = repository / "docs/deployment/governance-mineru.blocked.json"
    blocked = json.loads(blocked_path.read_text(encoding="utf-8"))
    assert blocked["doNotApply"] is True
    assert "processor" not in blocked
    assert not (repository / "docs/deployment/governance-mineru.example.json").exists()
    deployment = (repository / "docs/deployment/single-server-compose.md").read_text(
        encoding="utf-8"
    )
    assert "governance-mineru.example.json |" not in deployment
