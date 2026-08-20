#!/usr/bin/env python3
"""Validate content-free Harness timeline focused-canary evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sys
import tempfile
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, NoReturn

MAX_EVIDENCE_BYTES = 64 * 1024
MINIMUM_AGENT_TURNS = 5
MINIMUM_TOOL_CALLS = 5
MAXIMUM_VISIBLE_UPDATE_P95_MS = 300.0
MAXIMUM_DURABLE_RELOAD_RATIO = 1.2

REQUIRED_CARD_TYPES = {"terminal", "search", "file", "mcp"}
REQUIRED_EVENT_TYPES = {
    "turn.started",
    "tool.called",
    "tool.result",
    "assistant.message",
    "turn.ended",
}
REQUIRED_SECURITY_PROBES = {
    "secret",
    "host_path",
    "ansi_control",
    "oversized_payload",
    "unknown_mcp",
    "artifact_authorization",
}

TOP_LEVEL_KEYS = {
    "schemaVersion",
    "templateOnly",
    "candidate",
    "window",
    "rollout",
    "coverage",
    "performance",
    "security",
    "controls",
}
CANDIDATE_KEYS = {"commit", "backendImage", "frontendImage"}
WINDOW_KEYS = {"startedAt", "endedAt", "continuouslyDeployed"}
ROLLOUT_KEYS = {
    "timelineEnabled",
    "canaryUserIds",
    "controlUserIds",
    "canaryAgentEventsExposed",
    "controlAgentEventsExposed",
    "legacyProjectionAvailable",
    "candidateUnchanged",
    "rollbackRehearsed",
    "rollbackPreservedEvents",
    "unrelatedServicesStable",
}
COVERAGE_KEYS = {"agentTurns", "toolCalls", "cardTypes", "eventTypes"}
PERFORMANCE_KEYS = {
    "visibleUpdateP95Ms",
    "legacyReloadP95Ms",
    "durableReloadP95Ms",
}
SECURITY_KEYS = {"probeTypes", "leakCount", "rawPayloadLeakCount"}
CONTROL_KEYS = {
    "liveReloadParity",
    "reconnectConverged",
    "approvalCASPassed",
    "cancelPassed",
    "safeRetryPassed",
}


class EvidenceError(Exception):
    """Stable, content-free validation failure."""


def fail(code: str) -> NoReturn:
    raise EvidenceError(code)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate content-free Agent timeline focused-canary evidence.",
    )
    parser.add_argument("evidence", type=Path)
    parser.add_argument("--report", type=Path)
    return parser.parse_args()


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            fail("json.duplicate_key")
        result[key] = value
    return result


def read_evidence(path: Path) -> dict[str, Any]:
    try:
        if path.is_symlink() or not path.is_file():
            fail("evidence.path")
        body = path.read_bytes()
    except EvidenceError:
        raise
    except OSError:
        fail("evidence.read")
    if not body or len(body) > MAX_EVIDENCE_BYTES:
        fail("evidence.size")
    try:
        value = json.loads(body, object_pairs_hook=reject_duplicate_keys)
    except EvidenceError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail("evidence.json")
    if not isinstance(value, dict):
        fail("evidence.object")
    return value


def require_object(value: Any, keys: set[str], path: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != keys:
        fail(f"{path}.shape")
    return value


def require_bool(value: Any, expected: bool, path: str) -> None:
    if type(value) is not bool or value is not expected:
        fail(path)


def require_count(value: Any, minimum: int, path: str) -> int:
    if type(value) is not int or value < minimum:
        fail(path)
    return value


def require_metric(value: Any, path: str, *, positive: bool = False) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        fail(path)
    result = float(value)
    if not math.isfinite(result) or result < 0 or (positive and result <= 0):
        fail(path)
    return result


def require_string_set(value: Any, path: str) -> set[str]:
    if (
        not isinstance(value, list)
        or not value
        or any(
            not isinstance(item, str) or not item or len(item) > 64 for item in value
        )
        or len(value) != len(set(value))
    ):
        fail(path)
    return set(value)


def require_uuid_list(value: Any, path: str, *, exact: int | None = None) -> set[str]:
    if not isinstance(value, list) or (exact is not None and len(value) != exact):
        fail(path)
    result: set[str] = set()
    for item in value:
        if not isinstance(item, str):
            fail(path)
        try:
            normalized = str(uuid.UUID(item))
        except ValueError:
            fail(path)
        if item != normalized or item in result:
            fail(path)
        result.add(item)
    if not result:
        fail(path)
    return result


def require_timestamp(value: Any, path: str) -> datetime:
    if not isinstance(value, str) or not re.fullmatch(
        r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", value
    ):
        fail(path)
    try:
        return datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=timezone.utc
        )
    except ValueError:
        fail(path)


def validate_candidate(value: Any) -> tuple[str, str, str]:
    candidate = require_object(value, CANDIDATE_KEYS, "candidate")
    commit = candidate["commit"]
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        fail("candidate.commit")
    image_pattern = re.compile(r"[^\s@]{1,430}@sha256:[0-9a-f]{64}")
    for key in ("backendImage", "frontendImage"):
        image = candidate[key]
        if not isinstance(image, str) or not image_pattern.fullmatch(image):
            fail(f"candidate.{key}")
    return commit, candidate["backendImage"], candidate["frontendImage"]


def validate_window(value: Any) -> tuple[float, str, str]:
    window = require_object(value, WINDOW_KEYS, "window")
    require_bool(window["continuouslyDeployed"], True, "window.continuouslyDeployed")
    started_at = require_timestamp(window["startedAt"], "window.startedAt")
    ended_at = require_timestamp(window["endedAt"], "window.endedAt")
    window_seconds = (ended_at - started_at).total_seconds()
    if window_seconds <= 0:
        fail("window.duration")
    return window_seconds, window["startedAt"], window["endedAt"]


def validate_rollout(value: Any) -> tuple[int, int]:
    rollout = require_object(value, ROLLOUT_KEYS, "rollout")
    canary_ids = require_uuid_list(
        rollout["canaryUserIds"], "rollout.canaryUserIds", exact=1
    )
    control_ids = require_uuid_list(rollout["controlUserIds"], "rollout.controlUserIds")
    if canary_ids & control_ids:
        fail("rollout.userIsolation")
    for key in (
        "timelineEnabled",
        "canaryAgentEventsExposed",
        "legacyProjectionAvailable",
        "candidateUnchanged",
        "rollbackRehearsed",
        "rollbackPreservedEvents",
        "unrelatedServicesStable",
    ):
        require_bool(rollout[key], True, f"rollout.{key}")
    require_bool(
        rollout["controlAgentEventsExposed"], False, "rollout.controlAgentEventsExposed"
    )
    return len(canary_ids), len(control_ids)


def validate_coverage(value: Any) -> tuple[int, int]:
    coverage = require_object(value, COVERAGE_KEYS, "coverage")
    agent_turns = require_count(
        coverage["agentTurns"], MINIMUM_AGENT_TURNS, "coverage.agentTurns"
    )
    tool_calls = require_count(
        coverage["toolCalls"], MINIMUM_TOOL_CALLS, "coverage.toolCalls"
    )
    card_types = require_string_set(coverage["cardTypes"], "coverage.cardTypes")
    if not REQUIRED_CARD_TYPES <= card_types:
        fail("coverage.cardTypes.required")
    event_types = require_string_set(coverage["eventTypes"], "coverage.eventTypes")
    if not REQUIRED_EVENT_TYPES <= event_types:
        fail("coverage.eventTypes.required")
    return agent_turns, tool_calls


def validate_performance(value: Any) -> tuple[float, float]:
    performance = require_object(value, PERFORMANCE_KEYS, "performance")
    visible_p95 = require_metric(
        performance["visibleUpdateP95Ms"], "performance.visibleUpdateP95Ms"
    )
    legacy_p95 = require_metric(
        performance["legacyReloadP95Ms"], "performance.legacyReloadP95Ms", positive=True
    )
    durable_p95 = require_metric(
        performance["durableReloadP95Ms"], "performance.durableReloadP95Ms"
    )
    if visible_p95 > MAXIMUM_VISIBLE_UPDATE_P95_MS:
        fail("performance.visibleUpdateP95Ms.limit")
    reload_ratio = durable_p95 / legacy_p95
    if reload_ratio > MAXIMUM_DURABLE_RELOAD_RATIO:
        fail("performance.durableReloadRatio")
    return visible_p95, reload_ratio


def validate_security(value: Any) -> None:
    security = require_object(value, SECURITY_KEYS, "security")
    probes = require_string_set(security["probeTypes"], "security.probeTypes")
    if not REQUIRED_SECURITY_PROBES <= probes:
        fail("security.probeTypes.required")
    require_count(security["leakCount"], 0, "security.leakCount")
    require_count(security["rawPayloadLeakCount"], 0, "security.rawPayloadLeakCount")
    if security["leakCount"] != 0 or security["rawPayloadLeakCount"] != 0:
        fail("security.leaks")


def validate_controls(value: Any) -> None:
    controls = require_object(value, CONTROL_KEYS, "controls")
    for key in sorted(CONTROL_KEYS):
        require_bool(controls[key], True, f"controls.{key}")


def validate_evidence(value: dict[str, Any]) -> dict[str, Any]:
    require_object(value, TOP_LEVEL_KEYS, "evidence")
    if value["schemaVersion"] != 1:
        fail("schemaVersion")
    require_bool(value["templateOnly"], False, "templateOnly")
    commit, backend_image, frontend_image = validate_candidate(value["candidate"])
    window_seconds, window_started_at, window_ended_at = validate_window(
        value["window"]
    )
    canary_count, control_count = validate_rollout(value["rollout"])
    agent_turns, tool_calls = validate_coverage(value["coverage"])
    visible_p95, reload_ratio = validate_performance(value["performance"])
    validate_security(value["security"])
    validate_controls(value["controls"])
    canonical_evidence = json.dumps(
        value, sort_keys=True, separators=(",", ":"), ensure_ascii=False
    ).encode()
    return {
        "schemaVersion": 1,
        "status": "eligible",
        "candidateCommit": commit,
        "backendImage": backend_image,
        "frontendImage": frontend_image,
        "windowStartedAt": window_started_at,
        "windowEndedAt": window_ended_at,
        "windowDurationSeconds": int(window_seconds),
        "canaryUserCount": canary_count,
        "controlUserCount": control_count,
        "agentTurns": agent_turns,
        "toolCalls": tool_calls,
        "visibleUpdateP95Ms": visible_p95,
        "durableReloadRatio": round(reload_ratio, 6),
        "securityLeakCount": 0,
        "evidenceSha256": hashlib.sha256(canonical_evidence).hexdigest(),
    }


def write_report(path: Path, report: dict[str, Any]) -> None:
    if path.is_symlink() or not path.parent.is_dir():
        fail("report.path")
    body = (json.dumps(report, sort_keys=True, separators=(",", ":")) + "\n").encode()
    try:
        descriptor, temporary = tempfile.mkstemp(
            prefix=".agent-timeline-", dir=path.parent
        )
        try:
            os.fchmod(descriptor, 0o600)
            with os.fdopen(descriptor, "wb") as handle:
                handle.write(body)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
        except BaseException:
            try:
                os.unlink(temporary)
            except FileNotFoundError:
                pass
            raise
    except EvidenceError:
        raise
    except OSError:
        fail("report.write")


def main() -> int:
    arguments = parse_arguments()
    try:
        report = validate_evidence(read_evidence(arguments.evidence))
        if arguments.report is not None:
            write_report(arguments.report, report)
        else:
            print(json.dumps(report, sort_keys=True, separators=(",", ":")))
    except EvidenceError as error:
        print(f"agent timeline canary evidence invalid: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
