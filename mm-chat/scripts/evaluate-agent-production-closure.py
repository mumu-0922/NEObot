#!/usr/bin/env python3
"""Evaluate content-free Agent Runtime production-closure evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, NoReturn


PROJECT_DIR = Path(__file__).resolve().parents[1]
DEFAULT_POLICY = PROJECT_DIR / "config" / "agent-runner" / "production-policy.json"
MAX_DOCUMENT_BYTES = 256 << 10
FINGERPRINT_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
SAFE_ID_RE = re.compile(r"^[a-z][a-z0-9._-]{0,63}$")
DETAIL_RE = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")
ZERO_FINGERPRINT = "sha256:" + "0" * 64

CHECK_IDS = (
    "exact_host_isolation",
    "clean_copy_install",
    "restart_reconcile",
    "host_reboot_reconcile",
    "paired_backup_restore",
    "disaster_recovery",
    "rollback_forward_fix",
    "hierarchical_kill_switch",
    "credential_mtls_rotation",
    "runtime_bundle_rotation",
    "orphan_reconciliation",
    "outcome_unknown_workflow",
    "metrics_alerts",
    "capacity_budgets",
    "bounded_canary",
    "temporary_evidence_cleanup",
)
CHECK_RESULTS = {"passed", "failed", "not_run", "isolation_unavailable"}
REQUIRED_LABELS = {
    "operation",
    "outcome",
    "reason_code",
    "runtime_class",
    "scope_class",
    "switch_mode",
}
REQUIRED_METRICS = {
    "agent_runtime_ready",
    "agent_run_queue_depth",
    "agent_active_sandboxes",
    "agent_orphan_sandboxes",
    "agent_unresolved_outcome_unknown",
    "agent_cleanup_backlog",
    "agent_run_terminal_total",
    "agent_kill_switch_transition_total",
    "agent_effect_commit_total",
    "agent_reconcile_total",
    "agent_run_duration_seconds",
    "agent_queue_wait_seconds",
    "agent_budget_usage_ratio",
}
REQUIRED_ALERTS = {
    "runtime_enabled_not_ready",
    "secret_canary_leak",
    "outcome_unknown_unresolved",
    "kill_switch_enforcement_failed",
    "orphan_after_two_reconciles",
    "queue_capacity_sustained",
    "cleanup_backlog_sustained",
}


class EvidenceError(RuntimeError):
    """A stable, content-free production evidence error."""


def fail(code: str) -> NoReturn:
    raise EvidenceError(code)


def load_document(path: Path) -> tuple[dict[str, Any], bytes]:
    descriptor = -1
    try:
        descriptor = os.open(
            path,
            os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0),
        )
        metadata = os.fstat(descriptor)
        if (
            not stat.S_ISREG(metadata.st_mode)
            or metadata.st_mode & 0o022
            or metadata.st_size < 1
            or metadata.st_size > MAX_DOCUMENT_BYTES
        ):
            fail("DOCUMENT_FILE_UNSAFE")
        chunks: list[bytes] = []
        remaining = MAX_DOCUMENT_BYTES + 1
        while remaining > 0:
            chunk = os.read(descriptor, min(64 << 10, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        raw = b"".join(chunks)
    except OSError:
        fail("DOCUMENT_UNREADABLE")
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    if not raw or len(raw) != metadata.st_size or len(raw) > MAX_DOCUMENT_BYTES:
        fail("DOCUMENT_SIZE_INVALID")

    def reject_duplicate(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                fail("DUPLICATE_JSON_KEY")
            result[key] = value
        return result

    try:
        value = json.loads(raw, object_pairs_hook=reject_duplicate)
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail("DOCUMENT_JSON_INVALID")
    if not isinstance(value, dict):
        fail("DOCUMENT_ROOT_INVALID")
    return value, raw


def require_keys(value: Any, expected: set[str], code: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != expected:
        fail(code)
    return value


def require_integer(value: Any, minimum: int, maximum: int, code: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        fail(code)
    if value < minimum or value > maximum:
        fail(code)
    return value


def require_boolean(value: Any, expected: bool, code: str) -> None:
    if value is not expected:
        fail(code)


def require_safe_id(value: Any, code: str) -> str:
    if not isinstance(value, str) or SAFE_ID_RE.fullmatch(value) is None:
        fail(code)
    return value


def require_fingerprint(value: Any, code: str) -> str:
    if not isinstance(value, str) or FINGERPRINT_RE.fullmatch(value) is None:
        fail(code)
    return value


def require_unique_strings(
    value: Any, minimum: int, maximum: int, pattern: re.Pattern[str], code: str
) -> list[str]:
    if not isinstance(value, list) or not minimum <= len(value) <= maximum:
        fail(code)
    if any(
        not isinstance(item, str) or pattern.fullmatch(item) is None for item in value
    ):
        fail(code)
    if len(set(value)) != len(value):
        fail(code)
    return value


def parse_timestamp(value: Any, code: str) -> datetime:
    if not isinstance(value, str) or not value.endswith("Z"):
        fail(code)
    try:
        parsed = datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError:
        fail(code)
    if parsed.tzinfo is None or parsed.utcoffset() != timedelta(0):
        fail(code)
    return parsed.astimezone(timezone.utc)


def validate_budget(value: Any, code: str) -> dict[str, int]:
    budget = require_keys(
        value,
        {"maxWallSeconds", "maxModelTokens", "maxToolCalls", "maxArtifactBytes"},
        code,
    )
    return {
        "maxWallSeconds": require_integer(budget["maxWallSeconds"], 1, 86400, code),
        "maxModelTokens": require_integer(
            budget["maxModelTokens"], 1, 10_000_000, code
        ),
        "maxToolCalls": require_integer(budget["maxToolCalls"], 1, 10_000, code),
        "maxArtifactBytes": require_integer(
            budget["maxArtifactBytes"], 1, 10 << 30, code
        ),
    }


def validate_policy(policy: dict[str, Any]) -> int:
    require_keys(
        policy,
        {
            "schemaVersion",
            "policyRevision",
            "migrationHead",
            "reviewWindowHours",
            "capacity",
            "budgets",
            "canary",
            "retention",
            "cleanup",
            "observability",
        },
        "POLICY_ROOT_INVALID",
    )
    if policy["schemaVersion"] != "neo.agent-production-policy/v1":
        fail("POLICY_VERSION_INVALID")
    require_safe_id(policy["policyRevision"], "POLICY_REVISION_INVALID")
    if policy["migrationHead"] != 92:
        fail("POLICY_MIGRATION_HEAD_INVALID")
    review_hours = require_integer(
        policy["reviewWindowHours"], 1, 24, "POLICY_REVIEW_WINDOW_INVALID"
    )

    capacity = require_keys(
        policy["capacity"],
        {
            "maxConcurrentRootRuns",
            "maxConcurrentSandboxes",
            "maxQueueDepth",
            "maxConcurrentCanaries",
        },
        "POLICY_CAPACITY_INVALID",
    )
    roots = require_integer(
        capacity["maxConcurrentRootRuns"], 1, 64, "POLICY_CAPACITY_INVALID"
    )
    sandboxes = require_integer(
        capacity["maxConcurrentSandboxes"], 1, 128, "POLICY_CAPACITY_INVALID"
    )
    require_integer(capacity["maxQueueDepth"], 1, 10_000, "POLICY_CAPACITY_INVALID")
    canaries = require_integer(
        capacity["maxConcurrentCanaries"], 1, 8, "POLICY_CAPACITY_INVALID"
    )
    if roots > sandboxes or canaries > sandboxes:
        fail("POLICY_CAPACITY_INVALID")

    budgets = require_keys(
        policy["budgets"], {"root", "child", "cron"}, "POLICY_BUDGET_INVALID"
    )
    root_budget = validate_budget(budgets["root"], "POLICY_BUDGET_INVALID")
    for name in ("child", "cron"):
        budget = validate_budget(budgets[name], "POLICY_BUDGET_INVALID")
        if any(budget[key] > root_budget[key] for key in root_budget):
            fail("POLICY_BUDGET_WIDENED")

    canary = require_keys(
        policy["canary"],
        {
            "allowedModes",
            "maxRunsPerWindow",
            "windowMinutes",
            "maxErrors",
            "maxOutcomeUnknown",
            "egressMode",
            "secretsAllowed",
        },
        "POLICY_CANARY_INVALID",
    )
    modes = require_unique_strings(
        canary["allowedModes"], 1, 2, SAFE_ID_RE, "POLICY_CANARY_INVALID"
    )
    if set(modes) != {"synthetic", "read_only"}:
        fail("POLICY_CANARY_INVALID")
    require_integer(canary["maxRunsPerWindow"], 1, 1000, "POLICY_CANARY_INVALID")
    require_integer(canary["windowMinutes"], 5, 1440, "POLICY_CANARY_INVALID")
    require_integer(canary["maxErrors"], 0, 100, "POLICY_CANARY_INVALID")
    if canary["maxOutcomeUnknown"] != 0 or canary["egressMode"] != "none":
        fail("POLICY_CANARY_INVALID")
    require_boolean(canary["secretsAllowed"], False, "POLICY_CANARY_INVALID")

    retention = require_keys(
        policy["retention"],
        {
            "terminalRunsDays",
            "runnerReplayDays",
            "temporaryArtifactsDays",
            "cronHistoryDays",
            "auditDays",
            "promotionEvidenceDays",
            "preserveUnresolvedIncidents",
        },
        "POLICY_RETENTION_INVALID",
    )
    require_integer(retention["terminalRunsDays"], 7, 365, "POLICY_RETENTION_INVALID")
    require_integer(retention["runnerReplayDays"], 1, 90, "POLICY_RETENTION_INVALID")
    require_integer(
        retention["temporaryArtifactsDays"], 1, 90, "POLICY_RETENTION_INVALID"
    )
    require_integer(retention["cronHistoryDays"], 7, 365, "POLICY_RETENTION_INVALID")
    require_integer(retention["auditDays"], 90, 3650, "POLICY_RETENTION_INVALID")
    require_integer(
        retention["promotionEvidenceDays"], 365, 3650, "POLICY_RETENTION_INVALID"
    )
    require_boolean(
        retention["preserveUnresolvedIncidents"], True, "POLICY_RETENTION_INVALID"
    )

    cleanup = require_keys(
        policy["cleanup"],
        {"batchSize", "continueWhileRuntimeDisabled", "objectBeforeRow"},
        "POLICY_CLEANUP_INVALID",
    )
    require_integer(cleanup["batchSize"], 1, 1000, "POLICY_CLEANUP_INVALID")
    require_boolean(
        cleanup["continueWhileRuntimeDisabled"], True, "POLICY_CLEANUP_INVALID"
    )
    require_boolean(cleanup["objectBeforeRow"], True, "POLICY_CLEANUP_INVALID")

    observability = require_keys(
        policy["observability"],
        {"labelAllowlist", "metrics", "alerts"},
        "POLICY_OBSERVABILITY_INVALID",
    )
    labels = require_unique_strings(
        observability["labelAllowlist"],
        1,
        16,
        SAFE_ID_RE,
        "POLICY_LABELS_INVALID",
    )
    if set(labels) != REQUIRED_LABELS:
        fail("POLICY_LABELS_INVALID")
    metrics = require_unique_strings(
        observability["metrics"],
        1,
        64,
        re.compile(r"^agent_[a-z0-9_]{1,95}$"),
        "POLICY_METRICS_INVALID",
    )
    if set(metrics) != REQUIRED_METRICS:
        fail("POLICY_METRICS_INVALID")
    alerts = observability["alerts"]
    if not isinstance(alerts, list) or not 1 <= len(alerts) <= 32:
        fail("POLICY_ALERTS_INVALID")
    alert_ids: list[str] = []
    for item in alerts:
        alert = require_keys(
            item,
            {"id", "severity", "threshold", "windowMinutes"},
            "POLICY_ALERTS_INVALID",
        )
        alert_ids.append(require_safe_id(alert["id"], "POLICY_ALERTS_INVALID"))
        if alert["severity"] not in {"page", "ticket"}:
            fail("POLICY_ALERTS_INVALID")
        require_integer(alert["threshold"], 0, 1_000_000, "POLICY_ALERTS_INVALID")
        require_integer(alert["windowMinutes"], 1, 1440, "POLICY_ALERTS_INVALID")
    if len(set(alert_ids)) != len(alert_ids) or set(alert_ids) != REQUIRED_ALERTS:
        fail("POLICY_ALERTS_INVALID")
    return review_hours


def validate_record(
    record: dict[str, Any], policy: dict[str, Any], policy_raw: bytes, now: datetime
) -> tuple[str, str]:
    require_keys(
        record,
        {
            "schemaVersion",
            "evidenceClass",
            "release",
            "target",
            "window",
            "checks",
            "cleanup",
            "review",
        },
        "EVIDENCE_ROOT_INVALID",
    )
    if record["schemaVersion"] != "neo.agent-production-closure/v1":
        fail("EVIDENCE_VERSION_INVALID")
    if record["evidenceClass"] not in {"template", "production"}:
        fail("EVIDENCE_CLASS_INVALID")

    release = require_keys(
        record["release"],
        {
            "gitCommit",
            "migrationHead",
            "runnerManifestSha256",
            "runnerBinarySha256",
            "runtimeBundleFingerprint",
            "operationsPolicySha256",
        },
        "RELEASE_BINDING_INVALID",
    )
    if (
        not isinstance(release["gitCommit"], str)
        or COMMIT_RE.fullmatch(release["gitCommit"]) is None
    ):
        fail("RELEASE_BINDING_INVALID")
    if release["migrationHead"] != policy["migrationHead"]:
        fail("MIGRATION_HEAD_MISMATCH")
    release_fingerprints = [
        require_fingerprint(release[key], "RELEASE_BINDING_INVALID")
        for key in (
            "runnerManifestSha256",
            "runnerBinarySha256",
            "runtimeBundleFingerprint",
            "operationsPolicySha256",
        )
    ]
    policy_fingerprint = "sha256:" + hashlib.sha256(policy_raw).hexdigest()
    if release["operationsPolicySha256"] != policy_fingerprint:
        fail("POLICY_FINGERPRINT_MISMATCH")

    target = require_keys(
        record["target"],
        {"deploymentFingerprint", "hostClass", "runnerId", "runnerUid", "runnerGid"},
        "TARGET_BINDING_INVALID",
    )
    target_fingerprint = require_fingerprint(
        target["deploymentFingerprint"], "TARGET_BINDING_INVALID"
    )
    require_safe_id(target["hostClass"], "TARGET_BINDING_INVALID")
    require_safe_id(target["runnerId"], "TARGET_BINDING_INVALID")
    require_integer(target["runnerUid"], 1, 4_294_967_295, "TARGET_BINDING_INVALID")
    require_integer(target["runnerGid"], 1, 4_294_967_295, "TARGET_BINDING_INVALID")

    window = require_keys(
        record["window"], {"startedAt", "completedAt", "expiresAt"}, "WINDOW_INVALID"
    )
    started = parse_timestamp(window["startedAt"], "WINDOW_INVALID")
    completed = parse_timestamp(window["completedAt"], "WINDOW_INVALID")
    expires = parse_timestamp(window["expiresAt"], "WINDOW_INVALID")
    review_hours = int(policy["reviewWindowHours"])
    if not started <= completed < expires or expires - started > timedelta(
        hours=review_hours
    ):
        fail("WINDOW_INVALID")

    checks = record["checks"]
    if not isinstance(checks, list) or len(checks) != len(CHECK_IDS):
        fail("CHECK_SET_INCOMPLETE")
    check_results: dict[str, str] = {}
    evidence_fingerprints: list[str] = []
    for raw_check in checks:
        check = require_keys(
            raw_check,
            {"id", "result", "observedAt", "evidenceSha256", "detailCode"},
            "CHECK_INVALID",
        )
        check_id = check["id"]
        if check_id not in CHECK_IDS or check_id in check_results:
            fail("CHECK_SET_INVALID")
        if check["result"] not in CHECK_RESULTS:
            fail("CHECK_RESULT_INVALID")
        observed = parse_timestamp(check["observedAt"], "CHECK_TIME_INVALID")
        if not started <= observed <= completed:
            fail("CHECK_TIME_INVALID")
        evidence_fingerprints.append(
            require_fingerprint(check["evidenceSha256"], "CHECK_EVIDENCE_INVALID")
        )
        if (
            not isinstance(check["detailCode"], str)
            or DETAIL_RE.fullmatch(check["detailCode"]) is None
        ):
            fail("CHECK_DETAIL_INVALID")
        check_results[check_id] = check["result"]
    if set(check_results) != set(CHECK_IDS):
        fail("CHECK_SET_INCOMPLETE")

    cleanup = require_keys(
        record["cleanup"],
        {
            "temporaryCanaryRuns",
            "temporaryDrafts",
            "temporaryArtifacts",
            "temporaryRunEvidence",
            "orphanSandboxes",
            "scratchResidue",
            "promotionRecordRetained",
        },
        "CLEANUP_EVIDENCE_INVALID",
    )
    cleanup_counts = [
        require_integer(cleanup[key], 0, 1_000_000, "CLEANUP_EVIDENCE_INVALID")
        for key in (
            "temporaryCanaryRuns",
            "temporaryDrafts",
            "temporaryArtifacts",
            "temporaryRunEvidence",
            "orphanSandboxes",
            "scratchResidue",
        )
    ]
    require_boolean(
        cleanup["promotionRecordRetained"], True, "CLEANUP_EVIDENCE_INVALID"
    )

    review = require_keys(
        record["review"],
        {"decision", "reviewedAt", "reviewerFingerprint"},
        "REVIEW_INVALID",
    )
    if review["decision"] not in {"approved", "held"}:
        fail("REVIEW_INVALID")
    reviewed = parse_timestamp(review["reviewedAt"], "REVIEW_INVALID")
    if not completed <= reviewed < expires:
        fail("REVIEW_INVALID")
    reviewer_fingerprint = require_fingerprint(
        review["reviewerFingerprint"], "REVIEW_INVALID"
    )

    if check_results["exact_host_isolation"] == "isolation_unavailable":
        return "PROMOTION_HELD", "ISOLATION_UNAVAILABLE"
    if completed > now or now < started or now >= expires:
        return "PROMOTION_HELD", "EVIDENCE_STALE"
    if record["evidenceClass"] != "production":
        return "PROMOTION_HELD", "NON_PRODUCTION_EVIDENCE"
    if (
        ZERO_FINGERPRINT in release_fingerprints
        or target_fingerprint == ZERO_FINGERPRINT
        or ZERO_FINGERPRINT in evidence_fingerprints
        or reviewer_fingerprint == ZERO_FINGERPRINT
        or release["gitCommit"] == "0" * 40
    ):
        fail("PLACEHOLDER_BINDING_FORBIDDEN")
    for check_id in CHECK_IDS:
        result = check_results[check_id]
        if result != "passed":
            if result == "isolation_unavailable":
                return "PROMOTION_HELD", "ISOLATION_UNAVAILABLE"
            if result == "failed":
                return "PROMOTION_HELD", "LIVE_CHECK_FAILED"
            return "PROMOTION_HELD", "LIVE_CHECK_NOT_RUN"
    if any(cleanup_counts):
        return "PROMOTION_HELD", "TEMPORARY_EVIDENCE_REMAINS"
    if review["decision"] != "approved":
        return "PROMOTION_HELD", "REVIEW_HELD"
    return "PROMOTION_READY", "ALL_PRODUCTION_GATES_PASSED"


def emit_decision(
    verdict: str,
    reason: str,
    policy_raw: bytes,
    record_raw: bytes,
    now: datetime,
    release_commit: str = "",
) -> None:
    decision = {
        "schemaVersion": "neo.agent-production-decision/v1",
        "verdict": verdict,
        "reasonCode": reason,
        "policyFingerprint": "sha256:" + hashlib.sha256(policy_raw).hexdigest(),
        "evidenceFingerprint": "sha256:" + hashlib.sha256(record_raw).hexdigest(),
        "releaseCommit": release_commit,
        "evaluatedAt": now.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    print(json.dumps(decision, sort_keys=True, separators=(",", ":")))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Evaluate Agent Runtime production-closure evidence without mutation."
    )
    parser.add_argument("--record", required=True, type=Path)
    parser.add_argument("--policy", type=Path, default=DEFAULT_POLICY)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    policy_raw = b""
    record_raw = b""
    now = datetime.now(timezone.utc)
    try:
        policy, policy_raw = load_document(args.policy)
        validate_policy(policy)
        record, record_raw = load_document(args.record)
        verdict, reason = validate_record(record, policy, policy_raw, now)
        release = record.get("release")
        release_commit = (
            release.get("gitCommit", "") if isinstance(release, dict) else ""
        )
        emit_decision(verdict, reason, policy_raw, record_raw, now, release_commit)
        return 0 if verdict == "PROMOTION_READY" else 3
    except EvidenceError as error:
        emit_decision(
            "PROMOTION_EVIDENCE_INVALID",
            str(error),
            policy_raw,
            record_raw,
            now,
        )
        return 2


if __name__ == "__main__":
    sys.exit(main())
