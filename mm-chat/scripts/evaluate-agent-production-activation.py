#!/usr/bin/env python3
"""Read-only G21 staged Agent Runtime activation evaluator."""

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
from typing import Any

PROJECT_DIR = Path(__file__).resolve().parent.parent
DEFAULT_POLICY = PROJECT_DIR / "config/agent-runner/production-policy.json"
CONTROL_CHECK_IDS = {
    "exact_host_isolation",
    "clean_copy_install",
    "private_mtls_probe",
    "zero_inventory_reconcile",
    "rollback_kill_switch_ready",
}
ROOT_CANARY_CHECK_IDS = {
    "control_plane_ready",
    "exact_host_isolation",
    "private_mtls_canary",
    "restart_recovery_ready",
    "rollback_kill_switch_ready",
    "signed_authority_ready",
    "synthetic_plan_ready",
    "zero_inventory",
}
BROKER_CANARY_CHECK_IDS = {
    "artifact_authority_ready",
    "artifact_cleanup_ready",
    "control_plane_ready",
    "exact_host_isolation",
    "mcp_read_ready",
    "outcome_unknown_no_retry",
    "private_broker_relay_mtls",
    "private_runner_mtls",
    "project_read_ready",
    "restart_recovery_ready",
    "signed_authority_ready",
    "synthetic_plan_ready",
    "workspace_read_ready",
    "zero_inventory",
}
PROJECT_CANARY_CHECK_IDS = {
    "approval_authority_separate",
    "broker_artifact_canary_ready",
    "control_plane_ready",
    "credential_absence",
    "exact_host_isolation",
    "least_privilege_login",
    "not_sent_no_redispatch",
    "outcome_unknown_no_retry",
    "private_project_relay_mtls",
    "private_runner_mtls",
    "project_cas_cleanup",
    "project_receipt_recovery",
    "root_run_canary_ready",
    "signed_approval_ready",
    "signed_authority_ready",
    "synthetic_plan_ready",
    "zero_inventory",
}
CHILD_CANARY_CHECK_IDS = {
    "broker_artifact_canary_ready",
    "child_first_reap_ready",
    "control_plane_ready",
    "credential_absence",
    "delegate_task_removed",
    "depth_one_subset_ready",
    "exact_host_isolation",
    "late_launch_fenced",
    "least_privilege_login",
    "private_runner_mtls",
    "project_mutation_canary_ready",
    "restart_recovery_ready",
    "root_run_canary_ready",
    "signed_authority_ready",
    "synthetic_plan_ready",
    "zero_inventory",
}
WORKER_PREREQUISITES = {
    "broker_artifact_canary",
    "child_run_canary",
    "control_plane",
    "project_mutation_canary",
    "root_run_canary",
}
CRON_WORKER_CHECK_IDS = {
    "broker_artifact_canary_ready",
    "child_run_canary_ready",
    "control_plane_ready",
    "crash_restart_ready",
    "credential_absence",
    "exact_host_isolation",
    "exact_target_plan",
    "least_privilege_login",
    "project_mutation_canary_ready",
    "root_run_canary_ready",
}
DRAFT_LEARNING_WORKER_CHECK_IDS = {
    "broker_artifact_canary_ready",
    "child_run_canary_ready",
    "cleanup_recovery_ready",
    "control_plane_ready",
    "crash_restart_ready",
    "exact_host_isolation",
    "exact_target_plan",
    "human_promote_separate",
    "least_privilege_login",
    "object_store_separation",
    "project_mutation_canary_ready",
    "root_run_canary_ready",
    "runner_result_acl",
    "rootless_isolation_evaluation",
}
PRODUCT_CANARY_CHECK_IDS = {
    "crash_restart_recovery",
    "exact_host_isolation",
    "fixed_plan_ready",
    "prior_activation_chain",
    "private_mtls_canary",
    "product_request_authority",
    "worker_least_privilege",
    "zero_residue",
}
FINGERPRINT = re.compile(r"^sha256:[0-9a-f]{64}$")
IDENTITY = re.compile(r"^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$")
DETAIL = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")
ZERO = "sha256:" + "0" * 64
DecisionTuple = tuple[str, str, str]


class ActivationError(ValueError):
    pass


def fail(code: str) -> None:
    raise ActivationError(code)


def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            fail("DOCUMENT_DUPLICATE_KEY")
        result[key] = value
    return result


def load_document(path: Path) -> tuple[dict[str, Any], bytes]:
    raw = read_regular(path, "DOCUMENT_UNREADABLE")
    try:
        value = json.loads(raw, object_pairs_hook=unique_object)
    except (UnicodeError, json.JSONDecodeError):
        fail("DOCUMENT_INVALID")
    if not isinstance(value, dict):
        fail("DOCUMENT_INVALID")
    return value, raw


def read_regular(path: Path, code: str) -> bytes:
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
            or metadata.st_size < 2
            or metadata.st_size > 1 << 20
        ):
            fail(code)
        chunks: list[bytes] = []
        remaining = (1 << 20) + 1
        while remaining > 0:
            chunk = os.read(descriptor, min(64 << 10, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
    except OSError:
        fail(code)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    raw = b"".join(chunks)
    if len(raw) != metadata.st_size:
        fail(code)
    return raw


def exact(value: Any, keys: set[str], code: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != keys:
        fail(code)
    return value


def fingerprint(value: Any, code: str) -> str:
    if not isinstance(value, str) or FINGERPRINT.fullmatch(value) is None:
        fail(code)
    return value


def identity(value: Any, code: str) -> str:
    if not isinstance(value, str) or IDENTITY.fullmatch(value) is None:
        fail(code)
    return value


def timestamp(value: Any, code: str) -> datetime:
    if not isinstance(value, str) or not value.endswith("Z"):
        fail(code)
    try:
        parsed = datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError:
        fail(code)
    if parsed.tzinfo is None or parsed.microsecond:
        fail(code)
    return parsed.astimezone(timezone.utc)


def file_fingerprint(path: Path, code: str) -> str:
    raw = read_regular(path, code)
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def endpoint_fingerprint(endpoint: str) -> str:
    return (
        "sha256:"
        + hashlib.sha256(
            b"neo-agent-runner-endpoint-v1\0" + endpoint.encode("utf-8")
        ).hexdigest()
    )


def broker_relay_endpoint_fingerprint(endpoint: str) -> str:
    return (
        "sha256:"
        + hashlib.sha256(
            b"neo-agent-broker-relay-endpoint-v1\0" + endpoint.encode("utf-8")
        ).hexdigest()
    )


def project_relay_endpoint_fingerprint(endpoint: str) -> str:
    return (
        "sha256:"
        + hashlib.sha256(
            b"neo-agent-project-relay-endpoint-v1\0" + endpoint.encode("utf-8")
        ).hexdigest()
    )


def validate_policy(policy: dict[str, Any]) -> None:
    if (
        policy.get("schemaVersion") != "neo.agent-production-policy/v1"
        or policy.get("migrationHead") != 95
    ):
        fail("POLICY_INVALID")


def manifest_file_fingerprint(value: Any) -> str:
    item = exact(value, {"path", "sha256"}, "RUNNER_MANIFEST_INVALID")
    path = item["path"]
    if (
        not isinstance(path, str)
        or not 1 <= len(path) <= 512
        or not path.startswith("/")
        or ".." in path
        or "\0" in path
    ):
        fail("RUNNER_MANIFEST_INVALID")
    return fingerprint(item["sha256"], "RUNNER_MANIFEST_INVALID")


def validate_manifest_identity(manifest: dict[str, Any], runner_id: str | None) -> None:
    if manifest["schemaVersion"] != "neo.agent-runner-release/v1":
        fail("RUNNER_MANIFEST_INVALID")
    if manifest["approved"] is not True:
        fail("RUNNER_MANIFEST_INVALID")
    if identity(manifest["runnerId"], "RUNNER_MANIFEST_INVALID") != (
        runner_id or manifest["runnerId"]
    ):
        fail("RUNNER_MANIFEST_INVALID")
    version = manifest["runnerVersion"]
    if not isinstance(version, str) or not 1 <= len(version) <= 128:
        fail("RUNNER_MANIFEST_INVALID")


def validate_manifest_controllers(value: Any) -> None:
    if not isinstance(value, list) or len(value) != 3:
        fail("RUNNER_MANIFEST_INVALID")
    if any(not isinstance(item, str) for item in value):
        fail("RUNNER_MANIFEST_INVALID")
    if set(value) != {"cpu", "memory", "pids"}:
        fail("RUNNER_MANIFEST_INVALID")


def validate_manifest_runtime(manifest: dict[str, Any]) -> None:
    if manifest["protocolVersion"] != "neo.runner-rpc/v1":
        fail("RUNNER_MANIFEST_INVALID")
    if manifest["storageDriver"] != "overlay":
        fail("RUNNER_MANIFEST_INVALID")
    if manifest["networkMode"] != "none":
        fail("RUNNER_MANIFEST_INVALID")
    namespace_size = manifest["userNamespaceSize"]
    if isinstance(namespace_size, bool) or not isinstance(namespace_size, int):
        fail("RUNNER_MANIFEST_INVALID")
    if namespace_size < 65536:
        fail("RUNNER_MANIFEST_INVALID")
    validate_manifest_controllers(manifest["requiredControllers"])


def manifest_binary_fingerprints(value: Any) -> list[str]:
    if not isinstance(value, list) or len(value) != 5:
        fail("RUNNER_MANIFEST_INVALID")
    allowed = {"podman", "crun", "conmon", "newuidmap", "newgidmap"}
    names: set[str] = set()
    fingerprints: list[str] = []
    for binary in value:
        item = exact(
            binary,
            {"name", "path", "version", "sha256"},
            "RUNNER_MANIFEST_INVALID",
        )
        name = item["name"]
        version = item["version"]
        if name not in allowed or name in names:
            fail("RUNNER_MANIFEST_INVALID")
        if not isinstance(version, str) or not 1 <= len(version) <= 128:
            fail("RUNNER_MANIFEST_INVALID")
        names.add(name)
        fingerprints.append(
            manifest_file_fingerprint({"path": item["path"], "sha256": item["sha256"]})
        )
    return fingerprints


def validate_release_manifest(manifest: dict[str, Any], runner_id: str | None) -> None:
    exact(
        manifest,
        {
            "schemaVersion",
            "approved",
            "runnerId",
            "runnerVersion",
            "protocolVersion",
            "binaries",
            "storageDriver",
            "networkMode",
            "userNamespaceSize",
            "requiredControllers",
            "seccompProfile",
            "probeSuiteFingerprint",
            "isolationAcceptance",
        },
        "RUNNER_MANIFEST_INVALID",
    )
    validate_manifest_identity(manifest, runner_id)
    validate_manifest_runtime(manifest)
    fingerprints = [
        fingerprint(manifest["probeSuiteFingerprint"], "RUNNER_MANIFEST_INVALID")
    ]
    for key in ("seccompProfile", "isolationAcceptance"):
        fingerprints.append(manifest_file_fingerprint(manifest[key]))
    fingerprints.extend(manifest_binary_fingerprints(manifest["binaries"]))
    if ZERO in fingerprints:
        fail("RUNNER_MANIFEST_INVALID")


def worker_prerequisite_paths(values: list[str]) -> dict[str, Path]:
    result: dict[str, Path] = {}
    for value in values:
        stage, separator, path = value.partition("=")
        if (
            not separator
            or stage not in WORKER_PREREQUISITES
            or stage in result
            or not path
        ):
            fail("PREREQUISITE_INVALID")
        result[stage] = Path(path)
    return result


def validate_worker_record(
    record: dict[str, Any],
    policy_raw: bytes,
    now: datetime,
    args: argparse.Namespace,
) -> DecisionTuple:
    stage = record.get("stage")
    is_draft = stage == "draft_learning_worker"
    root_keys = {
        "schemaVersion",
        "evidenceClass",
        "stage",
        "release",
        "prerequisites",
        "target",
        "window",
        "checks",
        "authorization",
        "cleanup",
        "review",
    }
    if is_draft:
        root_keys |= {"wiring", "humanDecision"}
    exact(record, root_keys, "ACTIVATION_ROOT_INVALID")
    if (
        record["schemaVersion"] != "neo.agent-production-activation/v1"
        or stage not in {"cron_worker", "draft_learning_worker"}
    ):
        fail("ACTIVATION_VERSION_INVALID")
    if record["evidenceClass"] not in {"template", "production"}:
        fail("EVIDENCE_CLASS_INVALID")

    release = exact(
        record["release"],
        {
            "gitCommit",
            "migrationHead",
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        },
        "RELEASE_INVALID",
    )
    if (
        not isinstance(release["gitCommit"], str)
        or re.fullmatch(r"[0-9a-f]{40}", release["gitCommit"]) is None
        or release["migrationHead"] != 95
    ):
        fail("RELEASE_INVALID")
    release_fingerprints = [
        fingerprint(release[key], "RELEASE_INVALID")
        for key in (
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        )
    ]
    policy_sha = "sha256:" + hashlib.sha256(policy_raw).hexdigest()
    if release["operationsPolicySha256"] != policy_sha:
        fail("POLICY_FINGERPRINT_MISMATCH")

    prerequisites = record["prerequisites"]
    if not isinstance(prerequisites, list) or len(prerequisites) != len(
        WORKER_PREREQUISITES
    ):
        fail("PREREQUISITE_INVALID")
    prerequisite_fingerprints: dict[str, str] = {}
    for value in prerequisites:
        item = exact(value, {"stage", "evidenceSha256"}, "PREREQUISITE_INVALID")
        prerequisite_stage = item["stage"]
        if (
            prerequisite_stage not in WORKER_PREREQUISITES
            or prerequisite_stage in prerequisite_fingerprints
        ):
            fail("PREREQUISITE_INVALID")
        prerequisite_fingerprints[prerequisite_stage] = fingerprint(
            item["evidenceSha256"], "PREREQUISITE_INVALID"
        )
    if set(prerequisite_fingerprints) != WORKER_PREREQUISITES:
        fail("PREREQUISITE_INVALID")

    if is_draft:
        target_keys = {
            "activationId",
            "draftId",
            "draftFingerprint",
            "packageFingerprint",
            "runtimeFingerprint",
            "archiveFingerprint",
            "workspaceFingerprint",
            "planSha256",
        }
    else:
        target_keys = {
            "activationId",
            "templateId",
            "templateRevision",
            "templateFingerprint",
            "planSha256",
        }
    target = exact(record["target"], target_keys, "TARGET_INVALID")
    identity(target["activationId"], "TARGET_INVALID")
    identity(target["draftId" if is_draft else "templateId"], "TARGET_INVALID")
    if not is_draft and (
        isinstance(target["templateRevision"], bool)
        or not isinstance(target["templateRevision"], int)
        or target["templateRevision"] < 1
    ):
        fail("TARGET_INVALID")
    target_fingerprints = [
        fingerprint(value, "TARGET_INVALID")
        for key, value in target.items()
        if key.endswith("Fingerprint") or key == "planSha256"
    ]

    wiring_fingerprints: list[str] = []
    wiring: dict[str, Any] = {}
    if is_draft:
        wiring = exact(
            record["wiring"],
            {
                "endpointSha256",
                "clientCertificateSha256",
                "serverCASha256",
                "serverName",
                "callerIdentity",
                "authorityPublicKeySha256",
                "objectCredentialMetaSha256",
            },
            "WIRING_INVALID",
        )
        wiring_fingerprints = [
            fingerprint(wiring[key], "WIRING_INVALID")
            for key in (
                "endpointSha256",
                "clientCertificateSha256",
                "serverCASha256",
                "authorityPublicKeySha256",
                "objectCredentialMetaSha256",
            )
        ]
        identity(wiring["serverName"], "WIRING_INVALID")
        if wiring["callerIdentity"] != "spiffe://neo-chat/agent-runtime-draft-learning":
            fail("WIRING_INVALID")

    window = exact(
        record["window"], {"startedAt", "completedAt", "expiresAt"}, "WINDOW_INVALID"
    )
    started = timestamp(window["startedAt"], "WINDOW_INVALID")
    completed = timestamp(window["completedAt"], "WINDOW_INVALID")
    expires = timestamp(window["expiresAt"], "WINDOW_INVALID")
    if not started <= completed < expires or expires - started > timedelta(hours=24):
        fail("WINDOW_INVALID")

    check_ids = DRAFT_LEARNING_WORKER_CHECK_IDS if is_draft else CRON_WORKER_CHECK_IDS
    checks = record["checks"]
    if not isinstance(checks, list) or len(checks) != len(check_ids):
        fail("CHECK_SET_INCOMPLETE")
    results: dict[str, str] = {}
    check_fingerprints: list[str] = []
    for value in checks:
        item = exact(
            value,
            {"id", "result", "observedAt", "evidenceSha256", "detailCode"},
            "CHECK_INVALID",
        )
        check_id = item["id"]
        observed = timestamp(item["observedAt"], "CHECK_INVALID")
        if (
            check_id not in check_ids
            or check_id in results
            or item["result"]
            not in {"passed", "failed", "not_run", "isolation_unavailable"}
            or not started <= observed <= completed
            or not isinstance(item["detailCode"], str)
            or DETAIL.fullmatch(item["detailCode"]) is None
        ):
            fail("CHECK_SET_INVALID")
        results[check_id] = item["result"]
        check_fingerprints.append(
            fingerprint(item["evidenceSha256"], "CHECK_INVALID")
        )
    if set(results) != check_ids:
        fail("CHECK_SET_INCOMPLETE")

    if is_draft:
        expected_authorization = {
            "draftChecks": True,
            "cleanup": True,
            "runnerLifecycle": True,
            "runtime": False,
            "genericScheduler": False,
            "genericLearning": False,
            "skillInstall": False,
            "brokerMutation": False,
            "delegation": False,
            "promote": False,
            "administratorCredential": False,
        }
        cleanup_keys = {"orphanSandboxes", "pendingRunnerChecks", "pendingObjects"}
    else:
        expected_authorization = {
            "exactCronTarget": True,
            "runtime": False,
            "genericScheduler": False,
            "learning": False,
            "skillInstall": False,
            "brokerMutation": False,
            "delegation": False,
            "runnerCredential": False,
            "objectCredential": False,
        }
        cleanup_keys = {"staleClaims", "pendingTriggers"}
    authorization = exact(
        record["authorization"], set(expected_authorization), "AUTHORIZATION_INVALID"
    )
    if authorization != expected_authorization:
        fail("AUTHORIZATION_WIDENED")
    cleanup = exact(record["cleanup"], cleanup_keys, "CLEANUP_INVALID")
    if any(
        isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 1_000_000
        for value in cleanup.values()
    ):
        fail("CLEANUP_INVALID")

    human_fingerprints: list[str] = []
    if is_draft:
        human = exact(
            record["humanDecision"],
            {"decisionId", "actorClass", "promotedPackageFingerprint"},
            "HUMAN_DECISION_INVALID",
        )
        identity(human["decisionId"], "HUMAN_DECISION_INVALID")
        if human["actorClass"] != "human_operator":
            fail("HUMAN_DECISION_INVALID")
        human_fingerprints.append(
            fingerprint(
                human["promotedPackageFingerprint"], "HUMAN_DECISION_INVALID"
            )
        )

    review = exact(
        record["review"],
        {"decision", "reviewedAt", "reviewerFingerprint"},
        "REVIEW_INVALID",
    )
    if review["decision"] not in {"approved", "held"}:
        fail("REVIEW_INVALID")
    reviewed = timestamp(review["reviewedAt"], "REVIEW_INVALID")
    reviewer = fingerprint(review["reviewerFingerprint"], "REVIEW_INVALID")
    if not completed <= reviewed <= now or not reviewed < expires:
        fail("REVIEW_INVALID")

    if results["exact_host_isolation"] == "isolation_unavailable":
        return "ACTIVATION_HELD", "ISOLATION_UNAVAILABLE", release["gitCommit"]
    if record["evidenceClass"] != "production":
        return "ACTIVATION_HELD", "NON_PRODUCTION_EVIDENCE", release["gitCommit"]
    if now < started or now >= expires or completed > now:
        return "ACTIVATION_HELD", "EVIDENCE_STALE", release["gitCommit"]

    all_fingerprints = (
        release_fingerprints
        + list(prerequisite_fingerprints.values())
        + target_fingerprints
        + wiring_fingerprints
        + check_fingerprints
        + human_fingerprints
        + [reviewer]
    )
    if ZERO in all_fingerprints or release["gitCommit"] == "0" * 40:
        fail("PLACEHOLDER_BINDING_FORBIDDEN")
    if any(value != "passed" for value in results.values()):
        return "ACTIVATION_HELD", "LIVE_CHECK_NOT_PASSED", release["gitCommit"]
    if any(cleanup.values()):
        return "ACTIVATION_HELD", "RUNTIME_RESIDUE_REMAINS", release["gitCommit"]
    if review["decision"] != "approved":
        return "ACTIVATION_HELD", "REVIEW_HELD", release["gitCommit"]

    if args.release_commit != release["gitCommit"] or args.worker_plan is None:
        fail("WORKER_BINDING_DRIFT")
    if file_fingerprint(args.worker_plan, "WORKER_BINDING_DRIFT") != target["planSha256"]:
        fail("WORKER_BINDING_DRIFT")
    prerequisite_paths = worker_prerequisite_paths(args.prerequisite)
    if set(prerequisite_paths) != WORKER_PREREQUISITES:
        fail("PREREQUISITE_INVALID")
    for prerequisite_stage, path in prerequisite_paths.items():
        if file_fingerprint(path, "WORKER_BINDING_DRIFT") != prerequisite_fingerprints[
            prerequisite_stage
        ]:
            fail("WORKER_BINDING_DRIFT")

    if is_draft:
        required_args = (
            args.release_manifest,
            args.client_certificate,
            args.server_ca,
            args.authority_public_key,
            args.object_credential_meta,
            args.endpoint,
            args.runner_id,
            args.server_name,
            args.caller_identity,
        )
        if any(value is None for value in required_args):
            fail("WIRING_INVALID")
        bindings = (
            (args.release_manifest, release["runnerManifestSha256"], "RUNNER_MANIFEST_DRIFT"),
            (args.client_certificate, wiring["clientCertificateSha256"], "CLIENT_CERTIFICATE_DRIFT"),
            (args.server_ca, wiring["serverCASha256"], "SERVER_CA_DRIFT"),
            (args.authority_public_key, wiring["authorityPublicKeySha256"], "AUTHORITY_KEY_DRIFT"),
            (args.object_credential_meta, wiring["objectCredentialMetaSha256"], "OBJECT_CREDENTIAL_DRIFT"),
        )
        for path, expected, code in bindings:
            assert path is not None
            if file_fingerprint(path, code) != expected:
                fail(code)
        manifest, _ = load_document(args.release_manifest)
        validate_release_manifest(manifest, args.runner_id)
        if endpoint_fingerprint(args.endpoint) != wiring["endpointSha256"]:
            fail("RUNNER_ENDPOINT_DRIFT")
        if args.server_name != wiring["serverName"]:
            fail("SERVER_NAME_DRIFT")
        if args.caller_identity != wiring["callerIdentity"]:
            fail("CALLER_IDENTITY_DRIFT")

    return (
        "ACTIVATION_READY",
        "DRAFT_LEARNING_WORKER_GATES_PASSED" if is_draft else "CRON_WORKER_GATES_PASSED",
        release["gitCommit"],
    )


def validate_product_canary_record(
    record: dict[str, Any],
    policy_raw: bytes,
    now: datetime,
    args: argparse.Namespace,
) -> DecisionTuple:
    exact(
        record,
        {
            "schemaVersion",
            "evidenceClass",
            "stage",
            "release",
            "target",
            "wiring",
            "prerequisites",
            "window",
            "checks",
            "authorization",
            "cleanup",
            "review",
        },
        "ACTIVATION_ROOT_INVALID",
    )
    if (
        record["schemaVersion"] != "neo.agent-production-activation/v1"
        or record["stage"] != "product_canary"
    ):
        fail("ACTIVATION_VERSION_INVALID")
    if record["evidenceClass"] not in {"template", "production"}:
        fail("EVIDENCE_CLASS_INVALID")

    release = exact(
        record["release"],
        {
            "gitCommit",
            "migrationHead",
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        },
        "RELEASE_INVALID",
    )
    if (
        not isinstance(release["gitCommit"], str)
        or re.fullmatch(r"[0-9a-f]{40}", release["gitCommit"]) is None
        or release["migrationHead"] != 95
    ):
        fail("RELEASE_INVALID")
    release_fingerprints = [
        fingerprint(release[key], "RELEASE_INVALID")
        for key in (
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        )
    ]
    policy_sha = "sha256:" + hashlib.sha256(policy_raw).hexdigest()
    if release["operationsPolicySha256"] != policy_sha:
        fail("POLICY_FINGERPRINT_MISMATCH")

    target = exact(
        record["target"], {"deploymentFingerprint", "runnerId"}, "TARGET_INVALID"
    )
    deployment_fingerprint = fingerprint(
        target["deploymentFingerprint"], "TARGET_INVALID"
    )
    runner_id = identity(target["runnerId"], "TARGET_INVALID")

    wiring = exact(
        record["wiring"],
        {
            "activationId",
            "endpointSha256",
            "clientCertificateSha256",
            "serverCASha256",
            "serverName",
            "callerIdentity",
            "canaryPlanSha256",
            "authorityPublicKeySha256",
        },
        "WIRING_INVALID",
    )
    if (
        not isinstance(wiring["activationId"], str)
        or re.fullmatch(r"activation_[a-z0-9]{16,64}", wiring["activationId"])
        is None
        or wiring["callerIdentity"]
        != "spiffe://neo-chat/agent-runtime-product-canary"
    ):
        fail("WIRING_INVALID")
    identity(wiring["serverName"], "WIRING_INVALID")
    wiring_fingerprints = [
        fingerprint(wiring[key], "WIRING_INVALID")
        for key in (
            "endpointSha256",
            "clientCertificateSha256",
            "serverCASha256",
            "canaryPlanSha256",
            "authorityPublicKeySha256",
        )
    ]

    prerequisite_keys = {
        "controlPlane",
        "rootRun",
        "brokerArtifact",
        "projectMutation",
        "depthOneChild",
        "cronWorker",
        "draftLearning",
    }
    prerequisites = exact(
        record["prerequisites"], prerequisite_keys, "PREREQUISITE_INVALID"
    )
    prerequisite_fingerprints = [
        fingerprint(prerequisites[key], "PREREQUISITE_INVALID")
        for key in sorted(prerequisite_keys)
    ]

    window = exact(
        record["window"], {"startedAt", "completedAt", "expiresAt"}, "WINDOW_INVALID"
    )
    started = timestamp(window["startedAt"], "WINDOW_INVALID")
    completed = timestamp(window["completedAt"], "WINDOW_INVALID")
    expires = timestamp(window["expiresAt"], "WINDOW_INVALID")
    if not started <= completed < expires or expires - started > timedelta(hours=24):
        fail("WINDOW_INVALID")

    checks = record["checks"]
    if not isinstance(checks, list) or len(checks) != len(PRODUCT_CANARY_CHECK_IDS):
        fail("CHECK_SET_INCOMPLETE")
    results: dict[str, str] = {}
    check_fingerprints: list[str] = []
    for value in checks:
        item = exact(
            value,
            {"id", "result", "observedAt", "evidenceSha256", "detailCode"},
            "CHECK_INVALID",
        )
        check_id = item["id"]
        if check_id not in PRODUCT_CANARY_CHECK_IDS or check_id in results:
            fail("CHECK_SET_INVALID")
        if item["result"] not in {
            "passed",
            "failed",
            "not_run",
            "isolation_unavailable",
        }:
            fail("CHECK_INVALID")
        observed = timestamp(item["observedAt"], "CHECK_INVALID")
        if (
            not started <= observed <= completed
            or not isinstance(item["detailCode"], str)
            or DETAIL.fullmatch(item["detailCode"]) is None
        ):
            fail("CHECK_INVALID")
        check_fingerprints.append(
            fingerprint(item["evidenceSha256"], "CHECK_INVALID")
        )
        results[check_id] = item["result"]
    if set(results) != PRODUCT_CANARY_CHECK_IDS:
        fail("CHECK_SET_INCOMPLETE")

    authorization = exact(
        record["authorization"],
        {
            "productCanary",
            "genericRuntime",
            "brokerEffects",
            "delegation",
            "scheduler",
            "learning",
            "egress",
            "secrets",
        },
        "AUTHORIZATION_INVALID",
    )
    if authorization != {
        "productCanary": True,
        "genericRuntime": False,
        "brokerEffects": False,
        "delegation": False,
        "scheduler": False,
        "learning": False,
        "egress": False,
        "secrets": False,
    }:
        fail("AUTHORIZATION_WIDENED")

    cleanup = exact(
        record["cleanup"],
        {"queuedRequests", "claimedRequests", "orphanSandboxes", "scratchResidue"},
        "CLEANUP_INVALID",
    )
    for value in cleanup.values():
        if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 1_000_000:
            fail("CLEANUP_INVALID")

    review = exact(
        record["review"],
        {"decision", "reviewedAt", "reviewerFingerprint"},
        "REVIEW_INVALID",
    )
    if review["decision"] not in {"approved", "held"}:
        fail("REVIEW_INVALID")
    reviewed = timestamp(review["reviewedAt"], "REVIEW_INVALID")
    reviewer = fingerprint(review["reviewerFingerprint"], "REVIEW_INVALID")
    if not completed <= reviewed <= now or not reviewed < expires:
        fail("REVIEW_INVALID")

    if results["exact_host_isolation"] == "isolation_unavailable":
        return "ACTIVATION_HELD", "ISOLATION_UNAVAILABLE", release["gitCommit"]
    if record["evidenceClass"] != "production":
        return "ACTIVATION_HELD", "NON_PRODUCTION_EVIDENCE", release["gitCommit"]
    if now < started or now >= expires or completed > now:
        return "ACTIVATION_HELD", "EVIDENCE_STALE", release["gitCommit"]
    if (
        ZERO
        in release_fingerprints
        + wiring_fingerprints
        + prerequisite_fingerprints
        + check_fingerprints
        + [deployment_fingerprint, reviewer]
        or release["gitCommit"] == "0" * 40
    ):
        fail("PLACEHOLDER_BINDING_FORBIDDEN")
    if len(set(prerequisite_fingerprints)) != len(prerequisite_fingerprints):
        fail("PREREQUISITE_INVALID")
    if any(value != "passed" for value in results.values()):
        return "ACTIVATION_HELD", "LIVE_CHECK_NOT_PASSED", release["gitCommit"]
    if any(cleanup.values()):
        return "ACTIVATION_HELD", "RUNTIME_RESIDUE_REMAINS", release["gitCommit"]
    if review["decision"] != "approved":
        return "ACTIVATION_HELD", "REVIEW_HELD", release["gitCommit"]

    required_args = (
        args.release_manifest,
        args.client_certificate,
        args.server_ca,
        args.canary_plan,
        args.authority_public_key,
        args.endpoint,
        args.runner_id,
        args.server_name,
        args.caller_identity,
        args.release_commit,
        args.activation_id,
    )
    if any(value is None for value in required_args):
        fail("WIRING_INVALID")
    bindings = (
        (args.release_manifest, release["runnerManifestSha256"], "RUNNER_MANIFEST_DRIFT"),
        (args.client_certificate, wiring["clientCertificateSha256"], "CLIENT_CERTIFICATE_DRIFT"),
        (args.server_ca, wiring["serverCASha256"], "SERVER_CA_DRIFT"),
        (args.canary_plan, wiring["canaryPlanSha256"], "CANARY_PLAN_DRIFT"),
        (args.authority_public_key, wiring["authorityPublicKeySha256"], "AUTHORITY_KEY_DRIFT"),
    )
    for path, expected, code in bindings:
        assert path is not None
        if file_fingerprint(path, code) != expected:
            fail(code)
    manifest, _ = load_document(args.release_manifest)
    validate_release_manifest(manifest, args.runner_id)
    if endpoint_fingerprint(args.endpoint) != wiring["endpointSha256"]:
        fail("RUNNER_ENDPOINT_DRIFT")
    if args.runner_id != runner_id:
        fail("RUNNER_ID_DRIFT")
    if args.server_name != wiring["serverName"]:
        fail("SERVER_NAME_DRIFT")
    if args.caller_identity != wiring["callerIdentity"]:
        fail("CALLER_IDENTITY_DRIFT")
    if args.release_commit != release["gitCommit"]:
        fail("RELEASE_COMMIT_DRIFT")
    if args.activation_id != wiring["activationId"]:
        fail("ACTIVATION_ID_DRIFT")
    return "ACTIVATION_READY", "PRODUCT_CANARY_GATES_PASSED", release["gitCommit"]


def validate_record(
    record: dict[str, Any],
    policy_raw: bytes,
    now: datetime,
    args: argparse.Namespace,
) -> tuple[str, str, str]:
    if record.get("stage") in {"cron_worker", "draft_learning_worker"}:
        return validate_worker_record(record, policy_raw, now, args)
    if record.get("stage") == "product_canary":
        return validate_product_canary_record(record, policy_raw, now, args)
    exact(
        record,
        {
            "schemaVersion",
            "evidenceClass",
            "stage",
            "release",
            "target",
            "wiring",
            "window",
            "checks",
            "authorization",
            "cleanup",
            "review",
        },
        "ACTIVATION_ROOT_INVALID",
    )
    if record["schemaVersion"] != "neo.agent-production-activation/v1" or record[
        "stage"
    ] not in {
        "control_plane",
        "root_run_canary",
        "broker_artifact_canary",
        "project_mutation_canary",
        "depth_one_child_canary",
    }:
        fail("ACTIVATION_VERSION_INVALID")
    stage = record["stage"]
    check_ids = {
        "control_plane": CONTROL_CHECK_IDS,
        "root_run_canary": ROOT_CANARY_CHECK_IDS,
        "broker_artifact_canary": BROKER_CANARY_CHECK_IDS,
        "project_mutation_canary": PROJECT_CANARY_CHECK_IDS,
        "depth_one_child_canary": CHILD_CANARY_CHECK_IDS,
    }[stage]
    if record["evidenceClass"] not in {"template", "production"}:
        fail("EVIDENCE_CLASS_INVALID")
    if stage != "control_plane" and record["evidenceClass"] == "production":
        if args.canary_plan is None or args.authority_public_key is None:
            fail("WIRING_INVALID")
    if stage in {"broker_artifact_canary", "project_mutation_canary"} and record[
        "evidenceClass"
    ] == "production":
        if (
            args.relay_endpoint is None
            or args.relay_server_certificate is None
            or args.relay_client_ca is None
            or args.runner_relay_identity is None
        ):
            fail("WIRING_INVALID")
    if stage == "project_mutation_canary" and record["evidenceClass"] == "production":
        if args.approval_document is None or args.approval_public_key is None:
            fail("WIRING_INVALID")

    release = exact(
        record["release"],
        {
            "gitCommit",
            "migrationHead",
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        },
        "RELEASE_INVALID",
    )
    if (
        not isinstance(release["gitCommit"], str)
        or re.fullmatch(r"[0-9a-f]{40}", release["gitCommit"]) is None
        or release["migrationHead"] != 95
    ):
        fail("RELEASE_INVALID")
    release_fingerprints = [
        fingerprint(release[key], "RELEASE_INVALID")
        for key in (
            "runnerManifestSha256",
            "runnerBinarySha256",
            "operationsPolicySha256",
        )
    ]
    policy_sha = "sha256:" + hashlib.sha256(policy_raw).hexdigest()
    if release["operationsPolicySha256"] != policy_sha:
        fail("POLICY_FINGERPRINT_MISMATCH")

    target = exact(
        record["target"], {"deploymentFingerprint", "runnerId"}, "TARGET_INVALID"
    )
    deployment_fingerprint = fingerprint(
        target["deploymentFingerprint"], "TARGET_INVALID"
    )
    if (
        args.target_fingerprint is not None
        and args.target_fingerprint != deployment_fingerprint
    ):
        fail("TARGET_FINGERPRINT_DRIFT")
    runner_id = identity(target["runnerId"], "TARGET_INVALID")
    wiring_keys = {
        "endpointSha256",
        "clientCertificateSha256",
        "serverCASha256",
        "serverName",
        "callerIdentity",
    }
    if stage != "control_plane":
        wiring_keys |= {"canaryPlanSha256", "authorityPublicKeySha256"}
    if stage in {"broker_artifact_canary", "project_mutation_canary"}:
        wiring_keys |= {
            "relayEndpointSha256",
            "relayServerCertificateSha256",
            "relayClientCASha256",
            "runnerRelayIdentity",
        }
    if stage == "project_mutation_canary":
        wiring_keys |= {"approvalDocumentSha256", "approvalPublicKeySha256"}
    wiring = exact(record["wiring"], wiring_keys, "WIRING_INVALID")
    wiring_fingerprints = [
        fingerprint(wiring[key], "WIRING_INVALID")
        for key in ("endpointSha256", "clientCertificateSha256", "serverCASha256")
    ]
    if stage != "control_plane":
        wiring_fingerprints.extend(
            fingerprint(wiring[key], "WIRING_INVALID")
            for key in ("canaryPlanSha256", "authorityPublicKeySha256")
        )
    if stage in {"broker_artifact_canary", "project_mutation_canary"}:
        wiring_fingerprints.extend(
            fingerprint(wiring[key], "WIRING_INVALID")
            for key in (
                "relayEndpointSha256",
                "relayServerCertificateSha256",
                "relayClientCASha256",
            )
        )
        identity(wiring["runnerRelayIdentity"], "WIRING_INVALID")
    if stage == "project_mutation_canary":
        wiring_fingerprints.extend(
            fingerprint(wiring[key], "WIRING_INVALID")
            for key in ("approvalDocumentSha256", "approvalPublicKeySha256")
        )
        if wiring["authorityPublicKeySha256"] == wiring["approvalPublicKeySha256"]:
            fail("AUTHORITY_REUSE_FORBIDDEN")
    identity(wiring["serverName"], "WIRING_INVALID")
    identity(wiring["callerIdentity"], "WIRING_INVALID")

    window = exact(
        record["window"], {"startedAt", "completedAt", "expiresAt"}, "WINDOW_INVALID"
    )
    started = timestamp(window["startedAt"], "WINDOW_INVALID")
    completed = timestamp(window["completedAt"], "WINDOW_INVALID")
    expires = timestamp(window["expiresAt"], "WINDOW_INVALID")
    if not started <= completed < expires or expires - started > timedelta(hours=24):
        fail("WINDOW_INVALID")

    checks = record["checks"]
    if not isinstance(checks, list) or len(checks) != len(check_ids):
        fail("CHECK_SET_INCOMPLETE")
    results: dict[str, str] = {}
    evidence: list[str] = []
    for value in checks:
        item = exact(
            value,
            {"id", "result", "observedAt", "evidenceSha256", "detailCode"},
            "CHECK_INVALID",
        )
        check_id = item["id"]
        if check_id not in check_ids or check_id in results:
            fail("CHECK_SET_INVALID")
        if item["result"] not in {
            "passed",
            "failed",
            "not_run",
            "isolation_unavailable",
        }:
            fail("CHECK_INVALID")
        observed = timestamp(item["observedAt"], "CHECK_INVALID")
        if (
            not started <= observed <= completed
            or not isinstance(item["detailCode"], str)
            or DETAIL.fullmatch(item["detailCode"]) is None
        ):
            fail("CHECK_INVALID")
        evidence.append(fingerprint(item["evidenceSha256"], "CHECK_INVALID"))
        results[check_id] = item["result"]
    if set(results) != check_ids:
        fail("CHECK_SET_INCOMPLETE")

    if stage == "depth_one_child_canary":
        authorization_keys = {
            "controlPlane",
            "rootRuns",
            "childAgents",
            "brokerReadOnly",
            "brokerMutable",
            "artifactPublication",
            "projectMutation",
            "scheduler",
            "skillInstall",
            "learning",
            "egress",
            "secrets",
            "provider",
            "mcpWrite",
        }
        expected_authorization = {
            "controlPlane": False,
            "rootRuns": True,
            "childAgents": True,
            "brokerReadOnly": False,
            "brokerMutable": False,
            "artifactPublication": False,
            "projectMutation": False,
            "scheduler": False,
            "skillInstall": False,
            "learning": False,
            "egress": False,
            "secrets": False,
            "provider": False,
            "mcpWrite": False,
        }
    else:
        authorization_keys = {
            "controlPlane",
            "rootRuns",
            "brokerReadOnly",
            "brokerMutable",
            "delegation",
            "scheduler",
            "learning",
        }
        if stage in {"broker_artifact_canary", "project_mutation_canary"}:
            authorization_keys.add("artifactPublication")
        if stage == "project_mutation_canary":
            authorization_keys.add("projectMutation")
        expected_authorization = {
            "controlPlane": stage == "control_plane",
            "rootRuns": stage
            in {"root_run_canary", "broker_artifact_canary", "project_mutation_canary"},
            "brokerReadOnly": stage
            in {"broker_artifact_canary", "project_mutation_canary"},
            "brokerMutable": False,
            "delegation": False,
            "scheduler": False,
            "learning": False,
        }
        if stage in {"broker_artifact_canary", "project_mutation_canary"}:
            expected_authorization["artifactPublication"] = True
        if stage == "project_mutation_canary":
            expected_authorization["projectMutation"] = True
    authorization = exact(
        record["authorization"], authorization_keys, "AUTHORIZATION_INVALID"
    )
    if authorization != expected_authorization:
        fail("AUTHORIZATION_WIDENED")

    if stage == "depth_one_child_canary":
        cleanup_keys = {
            "orphanSandboxes",
            "scratchResidue",
            "pendingReaps",
            "descendants",
            "credentials",
        }
    else:
        cleanup_keys = {"orphanSandboxes", "scratchResidue"}
        if stage in {"broker_artifact_canary", "project_mutation_canary"}:
            cleanup_keys |= {"artifactResidue", "quarantineResidue"}
        if stage == "project_mutation_canary":
            cleanup_keys.add("projectResidue")
    cleanup = exact(record["cleanup"], cleanup_keys, "CLEANUP_INVALID")
    for key in cleanup:
        if (
            isinstance(cleanup[key], bool)
            or not isinstance(cleanup[key], int)
            or not 0 <= cleanup[key] <= 1_000_000
        ):
            fail("CLEANUP_INVALID")

    review = exact(
        record["review"],
        {"decision", "reviewedAt", "reviewerFingerprint"},
        "REVIEW_INVALID",
    )
    if review["decision"] not in {"approved", "held"}:
        fail("REVIEW_INVALID")
    reviewed = timestamp(review["reviewedAt"], "REVIEW_INVALID")
    reviewer = fingerprint(review["reviewerFingerprint"], "REVIEW_INVALID")
    if not completed <= reviewed <= now or not reviewed < expires:
        fail("REVIEW_INVALID")

    bindings = (
        (
            args.release_manifest,
            release["runnerManifestSha256"],
            "RUNNER_MANIFEST_DRIFT",
        ),
        (args.runner_binary, release["runnerBinarySha256"], "RUNNER_BINARY_DRIFT"),
        (
            args.client_certificate,
            wiring["clientCertificateSha256"],
            "CLIENT_CERTIFICATE_DRIFT",
        ),
        (args.server_ca, wiring["serverCASha256"], "SERVER_CA_DRIFT"),
    )
    stage_bindings = list(bindings)
    if stage != "control_plane":
        stage_bindings.extend(
            [
                (args.canary_plan, wiring["canaryPlanSha256"], "CANARY_PLAN_DRIFT"),
                (
                    args.authority_public_key,
                    wiring["authorityPublicKeySha256"],
                    "AUTHORITY_KEY_DRIFT",
                ),
            ]
        )
    if stage in {"broker_artifact_canary", "project_mutation_canary"}:
        stage_bindings.extend(
            [
                (
                    args.relay_server_certificate,
                    wiring["relayServerCertificateSha256"],
                    "RELAY_CERTIFICATE_DRIFT",
                ),
                (
                    args.relay_client_ca,
                    wiring["relayClientCASha256"],
                    "RELAY_CLIENT_CA_DRIFT",
                ),
            ]
        )
    if stage == "project_mutation_canary":
        stage_bindings.extend(
            [
                (
                    args.approval_document,
                    wiring["approvalDocumentSha256"],
                    "APPROVAL_DOCUMENT_DRIFT",
                ),
                (
                    args.approval_public_key,
                    wiring["approvalPublicKeySha256"],
                    "APPROVAL_KEY_DRIFT",
                ),
            ]
        )
    for path, expected, code in stage_bindings:
        if path is not None and file_fingerprint(path, code) != expected:
            fail(code)
    if args.release_manifest is not None:
        release_manifest, _ = load_document(args.release_manifest)
        validate_release_manifest(release_manifest, args.runner_id)
    if (
        args.endpoint is not None
        and endpoint_fingerprint(args.endpoint) != wiring["endpointSha256"]
    ):
        fail("RUNNER_ENDPOINT_DRIFT")
    if (
        stage in {"broker_artifact_canary", "project_mutation_canary"}
        and args.relay_endpoint is not None
        and (
            broker_relay_endpoint_fingerprint(args.relay_endpoint)
            if stage == "broker_artifact_canary"
            else project_relay_endpoint_fingerprint(args.relay_endpoint)
        )
        != wiring["relayEndpointSha256"]
    ):
        fail("RELAY_ENDPOINT_DRIFT")
    if args.runner_id is not None and args.runner_id != runner_id:
        fail("RUNNER_ID_DRIFT")
    if args.server_name is not None and args.server_name != wiring["serverName"]:
        fail("SERVER_NAME_DRIFT")
    if (
        args.caller_identity is not None
        and args.caller_identity != wiring["callerIdentity"]
    ):
        fail("CALLER_IDENTITY_DRIFT")
    if (
        stage in {"broker_artifact_canary", "project_mutation_canary"}
        and args.runner_relay_identity is not None
        and args.runner_relay_identity != wiring["runnerRelayIdentity"]
    ):
        fail("RELAY_IDENTITY_DRIFT")
    if args.release_commit is not None and args.release_commit != release["gitCommit"]:
        fail("RELEASE_COMMIT_DRIFT")

    if results["exact_host_isolation"] == "isolation_unavailable":
        return "ACTIVATION_HELD", "ISOLATION_UNAVAILABLE", release["gitCommit"]
    if record["evidenceClass"] != "production":
        return "ACTIVATION_HELD", "NON_PRODUCTION_EVIDENCE", release["gitCommit"]
    if now < started or now >= expires or completed > now:
        return "ACTIVATION_HELD", "EVIDENCE_STALE", release["gitCommit"]
    if (
        ZERO
        in release_fingerprints
        + wiring_fingerprints
        + evidence
        + [deployment_fingerprint, reviewer]
        or release["gitCommit"] == "0" * 40
    ):
        fail("PLACEHOLDER_BINDING_FORBIDDEN")
    if any(value != "passed" for value in results.values()):
        return "ACTIVATION_HELD", "LIVE_CHECK_NOT_PASSED", release["gitCommit"]
    if any(cleanup.values()):
        return "ACTIVATION_HELD", "RUNTIME_RESIDUE_REMAINS", release["gitCommit"]
    if review["decision"] != "approved":
        return "ACTIVATION_HELD", "REVIEW_HELD", release["gitCommit"]
    reason = {
        "control_plane": "CONTROL_PLANE_GATES_PASSED",
        "root_run_canary": "ROOT_RUN_CANARY_GATES_PASSED",
        "broker_artifact_canary": "BROKER_ARTIFACT_CANARY_GATES_PASSED",
        "project_mutation_canary": "PROJECT_MUTATION_CANARY_GATES_PASSED",
        "depth_one_child_canary": "DEPTH_ONE_CHILD_CANARY_GATES_PASSED",
    }[stage]
    return "ACTIVATION_READY", reason, release["gitCommit"]


def emit(
    decision: DecisionTuple,
    policy_raw: bytes,
    record_raw: bytes,
    now: datetime,
) -> None:
    verdict, reason, release_commit = decision
    print(
        json.dumps(
            {
                "schemaVersion": "neo.agent-production-activation-decision/v1",
                "verdict": verdict,
                "reasonCode": reason,
                "policyFingerprint": "sha256:" + hashlib.sha256(policy_raw).hexdigest(),
                "evidenceFingerprint": "sha256:"
                + hashlib.sha256(record_raw).hexdigest(),
                "releaseCommit": release_commit,
                "evaluatedAt": now.isoformat().replace("+00:00", "Z"),
            },
            sort_keys=True,
            separators=(",", ":"),
        )
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Evaluate staged Agent Runtime activation evidence without mutation"
    )
    parser.add_argument("--record", required=True, type=Path)
    parser.add_argument("--policy", type=Path, default=DEFAULT_POLICY)
    parser.add_argument("--release-manifest", type=Path)
    parser.add_argument("--runner-binary", type=Path)
    parser.add_argument("--client-certificate", type=Path)
    parser.add_argument("--server-ca", type=Path)
    parser.add_argument("--canary-plan", type=Path)
    parser.add_argument("--worker-plan", type=Path)
    parser.add_argument("--prerequisite", action="append", default=[])
    parser.add_argument("--object-credential-meta", type=Path)
    parser.add_argument("--authority-public-key", type=Path)
    parser.add_argument("--approval-document", type=Path)
    parser.add_argument("--approval-public-key", type=Path)
    parser.add_argument("--relay-endpoint")
    parser.add_argument("--relay-server-certificate", type=Path)
    parser.add_argument("--relay-client-ca", type=Path)
    parser.add_argument("--runner-relay-identity")
    parser.add_argument("--endpoint")
    parser.add_argument("--runner-id")
    parser.add_argument("--target-fingerprint")
    parser.add_argument("--server-name")
    parser.add_argument("--caller-identity")
    parser.add_argument("--release-commit")
    parser.add_argument("--activation-id")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    policy_raw = b""
    record_raw = b""
    now = datetime.now(timezone.utc).replace(microsecond=0)
    try:
        policy, policy_raw = load_document(args.policy)
        validate_policy(policy)
        record, record_raw = load_document(args.record)
        decision = validate_record(record, policy_raw, now, args)
        emit(decision, policy_raw, record_raw, now)
        verdict = decision[0]
        return 0 if verdict == "ACTIVATION_READY" else 3
    except ActivationError as error:
        emit(
            ("ACTIVATION_EVIDENCE_INVALID", str(error), ""),
            policy_raw,
            record_raw,
            now,
        )
        return 2


if __name__ == "__main__":
    sys.exit(main())
