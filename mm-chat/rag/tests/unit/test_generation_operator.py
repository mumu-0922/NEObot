from __future__ import annotations

import hashlib
import json
import uuid
from collections.abc import Callable, Mapping
from pathlib import Path
from typing import Any

import pytest

import mm_chat_rag.generation_gate_report as gate_report_module
import mm_chat_rag.replay as replay_module
from mm_chat_rag.generation_gate_report import (
    CRITICAL_GATE_SLICES,
    GATE_REPORT_EVALUATOR_VERSION,
    GATE_REPORT_SCHEMA_VERSION,
    GenerationOperatorError,
    load_and_validate_gate_report,
)
from mm_chat_rag.generation_operator import canonical_sha256
from mm_chat_rag.replay import run
from mm_chat_rag.structure_chunking import SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH

GENERATION_ID = uuid.UUID("50000000-0000-4000-8000-000000000001")
OPERATOR_ID = uuid.UUID("50000000-0000-4000-8000-000000000002")
MANIFEST_HASH = "a" * 64


def _valid_gate_report() -> dict[str, object]:
    candidate_metrics = {
        "recallAt50": 0.95,
        "finalRecallAt10": 0.90,
        "ndcgAt10": 0.85,
        "mrrAt10": 0.80,
        "citationCorrectness": 0.95,
        "citationCompleteness": 0.90,
        "faithfulness": 0.95,
        "answerCorrectness": 0.95,
        "noAnswerFalseAnswerRate": 0.02,
        "tableExactAnswer": 0.95,
        "provenanceCellLineage": 1.0,
        "aclLeakCount": 0,
        "deletionLeakCount": 0,
        "secretLeakCount": 0,
        "unauthorizedEvidenceLeakCount": 0,
    }
    integrity = {
        "passed": True,
        "citationLocatorRate": 1.0,
        "provenanceRate": 1.0,
        "cellLineageRate": 1.0,
    }
    return {
        "schemaVersion": GATE_REPORT_SCHEMA_VERSION,
        "candidateGenerationId": str(GENERATION_ID),
        "artifactManifestHash": MANIFEST_HASH,
        "passed": True,
        "evaluation": {
            "evaluatorVersion": GATE_REPORT_EVALUATOR_VERSION,
            "goldenCorpusRawSha256": "b" * 64,
            "goldenFrozenContentSha256": "c" * 64,
            "candidateObservationsSha256": "e" * 64,
            "candidateCaptureId": "50000000-0000-4000-8000-000000000011",
            "holdoutRunId": "50000000-0000-4000-8000-000000000012",
        },
        "golden": {
            "corpusId": "structure-candidate-golden-v1",
            "state": "frozen",
            "frozenAt": "2026-07-24T01:00:00Z",
            "totalReviewed": 500,
            "developmentCount": 300,
            "validationCount": 100,
            "holdoutCount": 100,
            "holdoutRuns": 1,
        },
        "slices": {
            name: {
                "metrics": dict(candidate_metrics),
                "integrity": dict(integrity),
                "cases": 50,
                "passed": True,
                "failures": [],
            }
            for name in CRITICAL_GATE_SLICES
        },
        "metrics": candidate_metrics,
        "budgets": {
            "candidateP95LatencyMilliseconds": 500,
            "maximumP95LatencyMilliseconds": 1000,
            "candidateAverageContextTokens": 2048,
            "maximumAverageContextTokens": 4096,
            "latencyPassed": True,
            "contextTokenCostPassed": True,
        },
        "integrity": integrity,
        "failures": [],
    }


def _write_gate_report(tmp_path: Path) -> tuple[Path, str]:
    path = tmp_path / "gate-report.json"
    path.write_text(
        json.dumps(_valid_gate_report(), separators=(",", ":"), sort_keys=True),
        encoding="utf-8",
    )
    return path, hashlib.sha256(path.read_bytes()).hexdigest()


def test_gate_report_requires_exact_hash_and_every_frozen_gate(
    tmp_path: Path,
) -> None:
    path, digest = _write_gate_report(tmp_path)
    report, actual = load_and_validate_gate_report(path, digest)
    assert actual == digest
    assert report["candidateGenerationId"] == str(GENERATION_ID)

    report["slices"]["pdf"]["metrics"]["recallAt50"] = 0.89  # type: ignore[index]
    path.write_text(json.dumps(report), encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    with pytest.raises(GenerationOperatorError, match="slice failed: pdf"):
        load_and_validate_gate_report(path, digest)
    with pytest.raises(GenerationOperatorError, match="SHA-256"):
        load_and_validate_gate_report(path, "b" * 64)


@pytest.mark.parametrize(
    ("path", "value", "match"),
    [
        (("golden", "totalReviewed"), 499, "Golden corpus is incomplete"),
        (("golden", "developmentCount"), 299, "Golden corpus is incomplete"),
        (("golden", "holdoutRuns"), 2, "Golden corpus is incomplete"),
        (("golden", "state"), "draft", "Golden corpus is not frozen"),
        (("slices", "pdf", "cases"), 49, "slice failed: pdf"),
        (
            ("integrity", "citationLocatorRate"),
            0.998,
            "citation/locator integrity failed",
        ),
        (
            ("integrity", "provenanceRate"),
            0.998,
            "citation/locator integrity failed",
        ),
        (
            ("integrity", "cellLineageRate"),
            0.998,
            "citation/locator integrity failed",
        ),
        (("metrics", "answerCorrectness"), 0.94, "metric failed"),
    ],
)
def test_gate_report_rejects_incomplete_promotion_evidence(
    tmp_path: Path,
    path: tuple[str, ...],
    value: object,
    match: str,
) -> None:
    report = _valid_gate_report()
    cursor: Any = report
    for key in path[:-1]:
        cursor = cursor[key]
    cursor[path[-1]] = value
    report_path = tmp_path / "invalid-gate-report.json"
    report_path.write_text(json.dumps(report), encoding="utf-8")
    digest = hashlib.sha256(report_path.read_bytes()).hexdigest()
    with pytest.raises(GenerationOperatorError, match=match):
        load_and_validate_gate_report(report_path, digest)


def test_gate_report_rejects_unknown_fields_and_non_finite_numbers(
    tmp_path: Path,
) -> None:
    report = _valid_gate_report()
    report["machineClaimsHumanReview"] = True
    path = tmp_path / "unknown-field.json"
    path.write_text(json.dumps(report), encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    with pytest.raises(GenerationOperatorError, match="root fields"):
        load_and_validate_gate_report(path, digest)

    report = _valid_gate_report()
    report["metrics"]["recallAt50"] = float("nan")  # type: ignore[index]
    path = tmp_path / "nan.json"
    path.write_text(json.dumps(report), encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    with pytest.raises(GenerationOperatorError, match="not valid JSON"):
        load_and_validate_gate_report(path, digest)

    valid_body = json.dumps(_valid_gate_report(), separators=(",", ":"))
    duplicate_body = valid_body.replace(
        '"schemaVersion":',
        '"schemaVersion":"shadowed","schemaVersion":',
        1,
    )
    path = tmp_path / "duplicate-key.json"
    path.write_text(duplicate_body, encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    with pytest.raises(GenerationOperatorError, match="not valid JSON"):
        load_and_validate_gate_report(path, digest)


def test_gate_report_rejects_non_object_root(tmp_path: Path) -> None:
    path = tmp_path / "array.json"
    path.write_text("[]", encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()

    with pytest.raises(GenerationOperatorError, match="must be a JSON object"):
        load_and_validate_gate_report(path, digest)


@pytest.mark.parametrize(
    ("report", "key"),
    [
        ({"id": None}, "id"),
        ({"id": "AAAAAAAA-0000-4000-8000-000000000001"}, "id"),
        ({"id": "not-a-uuid"}, "id"),
        ({"id": "{50000000-0000-4000-8000-000000000001}"}, "id"),
    ],
)
def test_gate_report_uuid_requires_canonical_lowercase(
    report: dict[str, object],
    key: str,
) -> None:
    with pytest.raises(GenerationOperatorError, match="gate report id is invalid"):
        gate_report_module.report_uuid(report, key)


@pytest.mark.parametrize(
    "value",
    [None, "A" * 64, "a" * 63, "z" * 64],
)
def test_gate_report_hash_requires_exact_lowercase_sha256(value: object) -> None:
    with pytest.raises(
        GenerationOperatorError,
        match="gate report digest is invalid",
    ):
        gate_report_module.report_hash({"digest": value}, "digest")


@pytest.mark.parametrize(
    ("mutate", "match"),
    [
        (
            lambda report: report.update(schemaVersion="v1"),
            "schema version is invalid",
        ),
        (
            lambda report: report.update(candidateGenerationId="invalid"),
            "candidateGenerationId is invalid",
        ),
        (lambda report: report.update(passed=False), "did not pass"),
        (lambda report: report.update(failures=["failed"]), "contains failures"),
        (
            lambda report: report["evaluation"].update(evaluatorVersion="v1"),  # type: ignore[union-attr]
            "evaluator version is invalid",
        ),
        (
            lambda report: report["metrics"].update(aclLeakCount=1),  # type: ignore[union-attr]
            "security metric failed",
        ),
        (
            lambda report: report["metrics"].update(  # type: ignore[union-attr]
                noAnswerFalseAnswerRate=0.03
            ),
            "no-answer metric failed",
        ),
        (
            lambda report: report.update(budgets="invalid"),
            "budgets is invalid",
        ),
        (
            lambda report: report["budgets"].update(latencyPassed=False),  # type: ignore[union-attr]
            "performance budget failed",
        ),
    ],
)
def test_gate_report_rejects_closed_contract_failures(
    tmp_path: Path,
    mutate: Callable[[dict[str, object]], None],
    match: str,
) -> None:
    report = _valid_gate_report()
    mutate(report)
    path = tmp_path / "closed-contract.json"
    path.write_text(json.dumps(report), encoding="utf-8")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()

    with pytest.raises(GenerationOperatorError, match=match):
        load_and_validate_gate_report(path, digest)


@pytest.mark.parametrize(
    ("call", "value"),
    [
        (gate_report_module._report_number, True),
        (gate_report_module._report_number, "1"),
        (gate_report_module._report_number, float("inf")),
        (gate_report_module._report_int, True),
        (gate_report_module._report_int, -1),
        (gate_report_module._report_rate, -0.1),
        (gate_report_module._report_rate, 1.1),
    ],
)
def test_gate_report_numeric_decoders_reject_invalid_values(
    call: Callable[[Mapping[str, object], str], object],
    value: object,
) -> None:
    with pytest.raises(GenerationOperatorError, match="gate report value is invalid"):
        call({"value": value}, "value")


@pytest.mark.parametrize(
    "value",
    [None, "", " 2026-07-25T00:00:00Z", "invalid", "2026-07-25T00:00:00"],
)
def test_gate_report_timestamp_requires_bounded_timezone(value: object) -> None:
    with pytest.raises(GenerationOperatorError, match="gate report at is invalid"):
        gate_report_module._report_timestamp({"at": value}, "at")


def test_canonical_hash_is_order_independent_and_profile_is_frozen() -> None:
    assert canonical_sha256({"b": 2, "a": 1}) == canonical_sha256({"a": 1, "b": 2})
    assert SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH == (
        "36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73"
    )


async def test_generation_status_uses_read_only_operator_gateway(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    captured: dict[str, str] = {}

    async def fake_status(database_url: str) -> dict[str, object]:
        captured["database_url"] = database_url
        return {"active_generation_id": str(GENERATION_ID), "head_revision": 7}

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator@db/rag")
    monkeypatch.setattr(replay_module, "generation_status", fake_status)
    assert await run(["generation-status"]) == 0
    assert captured["database_url"] == "postgresql://operator@db/rag"
    assert json.loads(capsys.readouterr().out)["head_revision"] == 7


async def test_generation_abandon_is_exact_dry_run_and_requires_confirmation(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    arguments = [
        "generation-abandon",
        "--candidate-generation-id",
        str(GENERATION_ID),
        "--expected-head-revision",
        "7",
        "--expected-manifest-hash",
        MANIFEST_HASH,
        "--operator-id",
        str(OPERATOR_ID),
        "--reason",
        "corpus snapshot is stale",
    ]
    assert await run(arguments) == 0
    intent = json.loads(capsys.readouterr().out)
    assert intent == {
        "candidateGenerationId": str(GENERATION_ID),
        "expectedHeadRevision": 7,
        "expectedManifestHash": MANIFEST_HASH,
        "kind": "generation-abandon",
        "mode": "dry-run",
        "operatorId": str(OPERATOR_ID),
        "reason": "corpus snapshot is stale",
    }

    with pytest.raises(SystemExit, match="2"):
        await run([*arguments, "--execute"])

    captured: dict[str, object] = {}

    async def fake_abandon(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return True

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator@db/rag")
    monkeypatch.setattr(
        replay_module,
        "abandon_structure_generation",
        fake_abandon,
    )
    assert await run([*arguments, "--confirm-abandon", "--execute"]) == 0
    output = json.loads(capsys.readouterr().out)
    assert output["abandoned"] is True
    assert captured == {
        "candidate_generation_id": GENERATION_ID,
        "database_url": "postgresql://operator@db/rag",
        "expected_head_revision": 7,
        "expected_manifest_hash": MANIFEST_HASH,
        "operator_id": OPERATOR_ID,
        "reason": "corpus snapshot is stale",
    }


@pytest.mark.parametrize(
    "arguments",
    [
        ["--expected-head-revision", "0"],
        ["--reason", " "],
        ["--reason", "x" * 1025],
    ],
)
async def test_generation_abandon_rejects_unbounded_inputs(
    arguments: list[str],
) -> None:
    values = {
        "--candidate-generation-id": str(GENERATION_ID),
        "--expected-head-revision": "7",
        "--expected-manifest-hash": MANIFEST_HASH,
        "--operator-id": str(OPERATOR_ID),
        "--reason": "stale corpus",
    }
    values[arguments[0]] = arguments[1]
    argv = ["generation-abandon"]
    for name, value in values.items():
        argv.extend((name, value))
    with pytest.raises(SystemExit, match="2"):
        await run(argv)


async def test_generation_activation_requires_manual_confirmation_and_gate_report(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    path, digest = _write_gate_report(tmp_path)
    arguments = [
        "generation-activate",
        "--gate-report",
        str(path),
        "--gate-report-sha256",
        digest,
        "--operator-id",
        str(OPERATOR_ID),
        "--execute",
    ]
    with pytest.raises(SystemExit, match="2"):
        await run(arguments)

    captured: dict[str, object] = {}

    async def fake_activate(database_url: str, **kwargs: object) -> bool:
        captured.update(database_url=database_url, **kwargs)
        return True

    monkeypatch.setenv("RAG_REPLAY_DATABASE_URL", "postgresql://operator@db/rag")
    monkeypatch.setattr(
        replay_module,
        "activate_structure_generation",
        fake_activate,
    )
    assert await run([*arguments, "--confirm-activation"]) == 0
    output = json.loads(capsys.readouterr().out)
    assert output["activated"] is True
    assert captured["operator_id"] == OPERATOR_ID
