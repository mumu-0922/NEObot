from __future__ import annotations

import re
import uuid

import pytest

from mm_chat_rag.handlers import ALLOWED_DISPATCH_ACTIONS, DispatchPlan, JobResult
from mm_chat_rag.models import (
    FunctionReadiness,
    JobClaim,
    OutboxClaim,
    stable_error_code,
)
from mm_chat_rag.retry import (
    PermanentJobError,
    RetryableJobError,
    retry_or_dlq,
)


def test_claim_models_validate_function_rows() -> None:
    event_id = uuid.uuid4()
    outbox = OutboxClaim.from_row(
        {
            "id": 9,
            "event_id": str(event_id),
            "event_type": "synthetic.created",
            "attempt_count": 2,
            "max_attempts": 4,
        }
    )
    assert outbox.outbox_id == 9
    assert outbox.event_id == event_id
    job_id = uuid.uuid4()
    job = JobClaim.from_row({"id": job_id, "stage": "parse", "attempt_count": 1})
    assert job.job_id == job_id
    assert job.max_attempts == 8


@pytest.mark.parametrize(
    "row",
    [
        {"id": True, "event_id": uuid.uuid4(), "event_type": "x"},
        {"id": 1, "event_id": "bad", "event_type": "x"},
        {"id": 1, "event_id": uuid.uuid4(), "event_type": ""},
    ],
)
def test_invalid_claim_rows_are_rejected(row: dict[str, object]) -> None:
    with pytest.raises((TypeError, ValueError)):
        OutboxClaim.from_row(row)


def test_invalid_job_stage_is_rejected() -> None:
    with pytest.raises(ValueError, match="stage"):
        JobClaim.from_row({"id": uuid.uuid4(), "stage": "Parse Job"})


def test_readiness_is_sanitized() -> None:
    ready = FunctionReadiness.from_row(
        {
            "functions_ready": True,
            "consumer_status": "ready",
            "projection_status": "building",
        }
    )
    assert ready.functions is True
    assert ready.projection == "building"
    actual_contract = FunctionReadiness.from_row(
        {
            "consumer_ready": True,
            "projection_ready": False,
            "detail": {"consumer": "ready", "projection": "catching_up"},
        }
    )
    assert actual_contract.functions is True
    assert actual_contract.consumer == "ready"
    assert actual_contract.projection == "catching_up"
    denied_contract = FunctionReadiness.from_row(
        {
            "consumer_ready": False,
            "projection_ready": False,
            "detail": {"consumer": "not_ready", "projection": "not_ready"},
        }
    )
    assert denied_contract.functions is False
    assert denied_contract.consumer == "not_ready"
    unsafe = FunctionReadiness.from_row(
        {"functions": "bad", "consumer": "payload", "projection": "arbitrary"}
    )
    assert unsafe.functions is False
    assert unsafe.consumer == "not_ready"
    assert unsafe.projection == "not_ready"


def test_dispatch_hash_is_deterministic_and_versioned() -> None:
    generation_id = uuid.uuid4()
    first = DispatchPlan(
        "generation",
        generation_id,
        "dispatch",
        {"synthetic_action": "apply", "z": [2, 1], "a": {"id": generation_id}},
    )
    second = DispatchPlan(
        "generation",
        generation_id,
        "dispatch",
        {"a": {"id": generation_id}, "synthetic_action": "apply", "z": [2, 1]},
    )
    assert first.result_hash() == second.result_hash()
    assert re.fullmatch(r"[0-9a-f]{64}", first.result_hash())
    assert (
        first.result_hash()
        != DispatchPlan(
            "generation", generation_id, "noop", first.parameters
        ).result_hash()
    )


def test_dispatch_action_allowlist_matches_sql_contract() -> None:
    expected = frozenset(
        {"collection_purge", "dispatch", "noop", "generation_reconstruct"}
    )
    assert expected == ALLOWED_DISPATCH_ACTIONS
    for action in ALLOWED_DISPATCH_ACTIONS:
        assert DispatchPlan("global", None, action).action == action


@pytest.mark.parametrize(
    "args",
    [
        ("bad", None, "ok", {}),
        ("generation", None, "ok", {}),
        ("global", uuid.uuid4(), "ok", {}),
        ("global", None, "Bad Action", {}),
        ("global", None, "synthetic.apply", {}),
        ("global", None, "ok", {"bad": 1.2}),
    ],
)
def test_invalid_dispatch_plans_are_rejected(args: tuple[object, ...]) -> None:
    with pytest.raises((TypeError, ValueError)):
        DispatchPlan(*args)  # type: ignore[arg-type]


def test_retry_and_dlq_decisions_use_stable_codes() -> None:
    assert (
        retry_or_dlq(
            attempt_count=1,
            max_attempts=2,
            error_code="PROVIDER_TIMEOUT",
            retry_after_seconds=20,
        ).outcome
        == "retry"
    )
    assert (
        retry_or_dlq(
            attempt_count=2,
            max_attempts=2,
            error_code="PROVIDER_TIMEOUT",
            retry_after_seconds=20,
        ).outcome
        == "failed"
    )
    assert RetryableJobError("PROVIDER_TIMEOUT", 12).retry_after_seconds == 12
    assert PermanentJobError("MALWARE_DETECTED").error_code == "MALWARE_DETECTED"
    assert JobResult().outcome == "succeeded"
    assert JobResult(terminal_committed=True).terminal_committed is True
    with pytest.raises(ValueError, match="successful"):
        JobResult(outcome="retry", error_code="FAILED_JOB")
    with pytest.raises(TypeError, match="terminal_committed"):
        JobResult(terminal_committed=1)  # type: ignore[arg-type]
    with pytest.raises(ValueError, match="uppercase"):
        stable_error_code("raw provider response")
    with pytest.raises(ValueError, match="between"):
        RetryableJobError("VALID_CODE", 0)
