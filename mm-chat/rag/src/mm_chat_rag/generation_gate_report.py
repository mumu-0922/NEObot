"""Closed validation contract for structure-generation promotion evidence."""

from __future__ import annotations

import hashlib
import json
import math
import uuid
from collections.abc import Mapping
from datetime import datetime
from pathlib import Path
from typing import Final, cast

GATE_REPORT_SCHEMA_VERSION: Final = "neo-chat-rag-candidate-gate-report.v2"
GATE_REPORT_EVALUATOR_VERSION: Final = "neo-chat.rag-promotion-evaluator.v2"
CRITICAL_GATE_SLICES: Final = (
    "pdf",
    "text_markdown_docx",
    "pptx",
    "xlsx_table",
    "json_code",
    "chinese",
    "english",
    "short_fact",
    "cross_section",
    "exact_numeric",
)

_SHA256_HEX_LENGTH: Final = 64
_MINIMUM_GOLDEN_TOTAL: Final = 500
_MINIMUM_DEVELOPMENT_CASES: Final = 300
_MINIMUM_VALIDATION_CASES: Final = 100
_MINIMUM_HOLDOUT_CASES: Final = 100
_MINIMUM_SLICE_CASES: Final = 50
_MAXIMUM_NO_ANSWER_FALSE_ANSWER_RATE: Final = 0.02


class GenerationOperatorError(RuntimeError):
    """An operator precondition or frozen gate failed."""


def load_and_validate_gate_report(
    path: Path,
    expected_sha256: str,
) -> tuple[dict[str, object], str]:
    """Validate the frozen human-reviewed promotion report and exact file hash."""
    body = path.read_bytes()
    actual_sha256 = hashlib.sha256(body).hexdigest()
    if actual_sha256 != expected_sha256.lower():
        raise GenerationOperatorError("gate report SHA-256 does not match the file")
    try:
        report = json.loads(
            body,
            object_pairs_hook=_strict_json_object,
            parse_constant=_reject_json_constant,
        )
    except (json.JSONDecodeError, ValueError) as error:
        raise GenerationOperatorError("gate report is not valid JSON") from error
    if not isinstance(report, dict):
        raise GenerationOperatorError("gate report must be a JSON object")
    _validate_gate_report(cast("dict[str, object]", report))
    return cast("dict[str, object]", report), actual_sha256


def report_uuid(report: Mapping[str, object], key: str) -> uuid.UUID:
    """Read one report UUID after rejecting malformed values."""
    value = report.get(key)
    if not isinstance(value, str) or value.lower() != value:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    try:
        parsed = uuid.UUID(value)
    except (TypeError, ValueError) as error:
        raise GenerationOperatorError(f"gate report {key} is invalid") from error
    if str(parsed) != value:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return parsed


def report_hash(report: Mapping[str, object], key: str) -> str:
    """Read one lowercase SHA-256 after rejecting malformed values."""
    value = report.get(key)
    if not isinstance(value, str):
        raise GenerationOperatorError(f"gate report {key} is invalid")
    normalized = value.lower()
    if (
        value != normalized
        or len(normalized) != _SHA256_HEX_LENGTH
        or any(character not in "0123456789abcdef" for character in normalized)
    ):
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return normalized


def _validate_gate_report(report: Mapping[str, object]) -> None:
    _require_exact_keys(
        report,
        {
            "schemaVersion",
            "candidateGenerationId",
            "artifactManifestHash",
            "passed",
            "evaluation",
            "golden",
            "slices",
            "metrics",
            "budgets",
            "integrity",
            "failures",
        },
        "root",
    )
    if report.get("schemaVersion") != GATE_REPORT_SCHEMA_VERSION:
        raise GenerationOperatorError("gate report schema version is invalid")
    report_uuid(report, "candidateGenerationId")
    report_hash(report, "artifactManifestHash")
    if report.get("passed") is not True:
        raise GenerationOperatorError("gate report did not pass")
    failures = report.get("failures")
    if not isinstance(failures, list) or failures:
        raise GenerationOperatorError("gate report contains failures")

    _validate_gate_evaluation(_report_mapping(report, "evaluation"))
    _validate_gate_golden(_report_mapping(report, "golden"))
    _validate_gate_slices(_report_mapping(report, "slices"))
    metrics = _report_mapping(report, "metrics")
    _validate_gate_metrics(metrics)
    _validate_gate_budgets(_report_mapping(report, "budgets"))
    _validate_gate_integrity(_report_mapping(report, "integrity"))


def _validate_gate_evaluation(evaluation: Mapping[str, object]) -> None:
    _require_exact_keys(
        evaluation,
        {
            "evaluatorVersion",
            "goldenCorpusRawSha256",
            "goldenFrozenContentSha256",
            "candidateObservationsSha256",
            "candidateCaptureId",
            "holdoutRunId",
        },
        "evaluation",
    )
    if evaluation.get("evaluatorVersion") != GATE_REPORT_EVALUATOR_VERSION:
        raise GenerationOperatorError("gate report evaluator version is invalid")
    for name in (
        "goldenCorpusRawSha256",
        "goldenFrozenContentSha256",
        "candidateObservationsSha256",
    ):
        report_hash(evaluation, name)
    report_uuid(evaluation, "candidateCaptureId")
    report_uuid(evaluation, "holdoutRunId")


def _validate_gate_golden(golden: Mapping[str, object]) -> None:
    _require_exact_keys(
        golden,
        {
            "corpusId",
            "state",
            "frozenAt",
            "totalReviewed",
            "developmentCount",
            "validationCount",
            "holdoutCount",
            "holdoutRuns",
        },
        "golden",
    )
    if (
        not isinstance(golden.get("corpusId"), str)
        or not str(golden["corpusId"]).strip()
        or golden.get("state") != "frozen"
    ):
        raise GenerationOperatorError("gate report Golden corpus is not frozen")
    _report_timestamp(golden, "frozenAt")
    total_reviewed = _report_int(golden, "totalReviewed")
    development_count = _report_int(golden, "developmentCount")
    validation_count = _report_int(golden, "validationCount")
    holdout_count = _report_int(golden, "holdoutCount")
    holdout_runs = _report_int(golden, "holdoutRuns")
    if (
        total_reviewed < _MINIMUM_GOLDEN_TOTAL
        or development_count < _MINIMUM_DEVELOPMENT_CASES
        or validation_count < _MINIMUM_VALIDATION_CASES
        or holdout_count < _MINIMUM_HOLDOUT_CASES
        or development_count + validation_count + holdout_count != total_reviewed
        or development_count * 100 != total_reviewed * 60
        or validation_count * 100 != total_reviewed * 20
        or holdout_count * 100 != total_reviewed * 20
        or holdout_runs != 1
    ):
        raise GenerationOperatorError("gate report Golden corpus is incomplete")


def _validate_gate_slices(slices: Mapping[str, object]) -> None:
    _require_exact_keys(slices, set(CRITICAL_GATE_SLICES), "slices")
    for name in CRITICAL_GATE_SLICES:
        result = _report_mapping(slices, name)
        _require_exact_keys(
            result,
            {"metrics", "integrity", "cases", "passed", "failures"},
            f"slice {name}",
        )
        failures = result.get("failures")
        if (
            result.get("passed") is not True
            or _report_int(result, "cases") < _MINIMUM_SLICE_CASES
            or not isinstance(failures, list)
            or failures
        ):
            raise GenerationOperatorError(f"gate report slice failed: {name}")
        try:
            _validate_gate_metrics(_report_mapping(result, "metrics"))
            _validate_gate_integrity(_report_mapping(result, "integrity"))
        except GenerationOperatorError as error:
            raise GenerationOperatorError(
                f"gate report slice failed: {name}"
            ) from error


def _validate_gate_metrics(
    metrics: Mapping[str, object],
) -> None:
    metric_names = {
        "recallAt50",
        "finalRecallAt10",
        "ndcgAt10",
        "mrrAt10",
        "citationCorrectness",
        "citationCompleteness",
        "faithfulness",
        "answerCorrectness",
        "noAnswerFalseAnswerRate",
        "tableExactAnswer",
        "provenanceCellLineage",
        "aclLeakCount",
        "deletionLeakCount",
        "secretLeakCount",
        "unauthorizedEvidenceLeakCount",
    }
    _require_exact_keys(metrics, metric_names, "metrics")
    leak_names = {
        "aclLeakCount",
        "deletionLeakCount",
        "secretLeakCount",
        "unauthorizedEvidenceLeakCount",
    }
    for name in metric_names - leak_names:
        _report_rate(metrics, name)
    for name in leak_names:
        if _report_int(metrics, name) != 0:
            raise GenerationOperatorError(f"gate report security metric failed: {name}")
    minimums = {
        "answerCorrectness": 0.95,
        "citationCompleteness": 0.90,
        "citationCorrectness": 0.95,
        "faithfulness": 0.95,
        "finalRecallAt10": 0.90,
        "mrrAt10": 0.80,
        "ndcgAt10": 0.85,
        "provenanceCellLineage": 1.0,
        "recallAt50": 0.95,
        "tableExactAnswer": 0.95,
    }
    for name, minimum in minimums.items():
        if _report_rate(metrics, name) < minimum:
            raise GenerationOperatorError(f"gate report metric failed: {name}")
    if (
        _report_rate(metrics, "noAnswerFalseAnswerRate")
        > _MAXIMUM_NO_ANSWER_FALSE_ANSWER_RATE
    ):
        raise GenerationOperatorError("gate report no-answer metric failed")


def _validate_gate_budgets(budgets: Mapping[str, object]) -> None:
    _require_exact_keys(
        budgets,
        {
            "candidateP95LatencyMilliseconds",
            "maximumP95LatencyMilliseconds",
            "candidateAverageContextTokens",
            "maximumAverageContextTokens",
            "latencyPassed",
            "contextTokenCostPassed",
        },
        "budgets",
    )
    candidate_latency = _report_number(
        budgets,
        "candidateP95LatencyMilliseconds",
    )
    maximum_latency = _report_number(budgets, "maximumP95LatencyMilliseconds")
    candidate_context_tokens = _report_number(
        budgets,
        "candidateAverageContextTokens",
    )
    maximum_context_tokens = _report_number(
        budgets,
        "maximumAverageContextTokens",
    )
    if (
        budgets.get("latencyPassed") is not True
        or budgets.get("contextTokenCostPassed") is not True
        or candidate_latency < 0
        or maximum_latency <= 0
        or candidate_latency > maximum_latency
        or candidate_context_tokens < 0
        or maximum_context_tokens <= 0
        or candidate_context_tokens > maximum_context_tokens
    ):
        raise GenerationOperatorError("gate report performance budget failed")


def _validate_gate_integrity(integrity: Mapping[str, object]) -> None:
    _require_exact_keys(
        integrity,
        {
            "passed",
            "citationLocatorRate",
            "provenanceRate",
            "cellLineageRate",
        },
        "integrity",
    )
    if (
        integrity.get("passed") is not True
        or _report_rate(integrity, "citationLocatorRate") != 1
        or _report_rate(integrity, "provenanceRate") != 1
        or _report_rate(integrity, "cellLineageRate") != 1
    ):
        raise GenerationOperatorError("gate report citation/locator integrity failed")


def _report_mapping(
    report: Mapping[str, object],
    key: str,
) -> Mapping[str, object]:
    value = report.get(key)
    if not isinstance(value, dict):
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return cast("Mapping[str, object]", value)


def _report_number(report: Mapping[str, object], key: str) -> float:
    value = report.get(key)
    if isinstance(value, bool) or not isinstance(value, int | float):
        raise GenerationOperatorError(f"gate report {key} is invalid")
    normalized = float(value)
    if not math.isfinite(normalized):
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return normalized


def _report_int(report: Mapping[str, object], key: str) -> int:
    value = report.get(key)
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return value


def _report_rate(report: Mapping[str, object], key: str) -> float:
    value = _report_number(report, key)
    if value < 0 or value > 1:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return value


def _report_timestamp(report: Mapping[str, object], key: str) -> datetime:
    value = report.get(key)
    if not isinstance(value, str) or not value or value.strip() != value:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    try:
        parsed = datetime.fromisoformat(value)
    except ValueError as error:
        raise GenerationOperatorError(f"gate report {key} is invalid") from error
    if parsed.tzinfo is None:
        raise GenerationOperatorError(f"gate report {key} is invalid")
    return parsed


def _require_exact_keys(
    value: Mapping[str, object],
    expected: set[str],
    label: str,
) -> None:
    if set(value) != expected:
        raise GenerationOperatorError(f"gate report {label} fields are invalid")


def _reject_json_constant(value: str) -> object:
    raise ValueError(f"non-finite JSON number: {value}")


def _strict_json_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON object key: {key}")
        result[key] = value
    return result
