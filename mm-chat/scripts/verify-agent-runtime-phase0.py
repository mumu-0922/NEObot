#!/usr/bin/env python3
"""Offline structural verification for the Agent Runtime Phase 0 contracts."""

from __future__ import annotations

import hashlib
import json
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker


PROJECT_DIR = Path(__file__).resolve().parents[1]
CONTRACT_DIR = PROJECT_DIR / "docs" / "contracts"
SCHEMA_DIR = CONTRACT_DIR / "schemas"
FIXTURE_DIR = CONTRACT_DIR / "fixtures" / "agent-runtime"
REPO_ROOT = PROJECT_DIR.parent

SCHEMA_NAMES = (
    "neo-skill-runtime-manifest",
    "neo-capability-grant",
    "neo-runner-rpc",
    "neo-run-event",
    "neo-cron-template",
    "neo-skill-draft",
    "neo-agent-production-policy",
    "neo-agent-production-closure",
)
JsonObject = dict[str, Any]


class VerificationError(RuntimeError):
    """Raised when a Phase 0 invariant is not met."""


def load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise VerificationError(f"cannot load JSON {path}: {error}") from error


def check_schemas_and_fixtures() -> dict[str, dict[str, Any]]:
    valid_instances: dict[str, dict[str, Any]] = {}
    for name in SCHEMA_NAMES:
        schema_path = SCHEMA_DIR / f"{name}.schema.json"
        valid_path = FIXTURE_DIR / f"{name}.valid.json"
        invalid_path = FIXTURE_DIR / f"{name}.invalid.json"
        schema = load_json(schema_path)
        valid_instance = load_json(valid_path)
        invalid_instance = load_json(invalid_path)

        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema, format_checker=FormatChecker())
        valid_errors = sorted(
            validator.iter_errors(valid_instance),
            key=lambda item: [str(part) for part in item.absolute_path],
        )
        if valid_errors:
            first = valid_errors[0]
            location = "/".join(str(part) for part in first.absolute_path)
            raise VerificationError(
                f"valid fixture failed {name} at {location or '<root>'}: {first.message}"
            )
        if not list(validator.iter_errors(invalid_instance)):
            raise VerificationError(f"invalid fixture unexpectedly passed: {name}")
        unknown_instance = dict(valid_instance)
        unknown_instance["unexpectedPhase0Field"] = True
        if not list(validator.iter_errors(unknown_instance)):
            raise VerificationError(f"schema accepts an unknown root field: {name}")
        for supplemental_path in sorted(FIXTURE_DIR.glob(f"{name}.*.valid.json")):
            supplemental = load_json(supplemental_path)
            supplemental_errors = sorted(
                validator.iter_errors(supplemental),
                key=lambda item: [str(part) for part in item.absolute_path],
            )
            if supplemental_errors:
                first = supplemental_errors[0]
                location = "/".join(str(part) for part in first.absolute_path)
                raise VerificationError(
                    f"valid fixture failed {supplemental_path.name} at "
                    f"{location or '<root>'}: {first.message}"
                )
            unknown_supplemental = dict(supplemental)
            unknown_supplemental["unexpectedPhase0Field"] = True
            if not list(validator.iter_errors(unknown_supplemental)):
                raise VerificationError(
                    "schema accepts an unknown root field: "
                    f"{supplemental_path.name}"
                )
        valid_instances[name] = valid_instance
    return valid_instances


def require_equal(left: Any, right: Any, message: str) -> None:
    if left != right:
        raise VerificationError(message)


def check_fingerprint_bindings(grant: JsonObject, launch: JsonObject) -> None:
    sandbox = launch["body"]["sandbox"]
    require_equal(
        sandbox["packageFingerprint"],
        grant["packageFingerprint"],
        "launch/grant package fingerprints differ",
    )
    require_equal(
        sandbox["runtimeBundleFingerprint"],
        grant["runtimeBundleFingerprint"],
        "launch/grant runtime fingerprints differ",
    )


def check_child_registry(grant: JsonObject, launch: JsonObject) -> None:
    registry = launch["body"]["toolRegistry"]
    require_equal(
        registry["depth"],
        grant["run"]["depth"],
        "launch/grant delegation depths differ",
    )
    forbidden = {
        "delegate_task",
        "cron_manage",
        "grant_manage",
        "secret_manage",
        "runtime_manage",
    }
    if registry["depth"] == 1 and forbidden.intersection(registry["tools"]):
        raise VerificationError("child registry contains a forbidden management tool")


def check_event_bindings(launch: JsonObject, event: JsonObject) -> None:
    attempt = launch["body"]["attempt"]
    for field in ("runId", "stepId", "attemptId"):
        require_equal(
            event[field], attempt[field], f"event/launch {field} values differ"
        )
    require_equal(
        event["lease"]["generation"],
        attempt["leaseGeneration"],
        "event/launch lease generations differ",
    )


def check_cross_contracts(instances: dict[str, dict[str, Any]]) -> None:
    grant = instances["neo-capability-grant"]
    launch = instances["neo-runner-rpc"]
    event = instances["neo-run-event"]
    cron_template = instances["neo-cron-template"]
    draft = instances["neo-skill-draft"]
    production_policy = instances["neo-agent-production-policy"]
    production_closure = instances["neo-agent-production-closure"]

    check_fingerprint_bindings(grant, launch)
    require_equal(
        launch["body"]["attempt"]["runId"],
        grant["run"]["runId"],
        "launch/grant run IDs differ",
    )
    lineage = launch["body"]["lineage"]
    require_equal(
        lineage["depth"], grant["run"]["depth"], "launch/grant lineage depths differ"
    )
    if lineage["depth"] == 1:
        require_equal(
            lineage["parentRunId"],
            grant["run"]["parentRunId"],
            "launch/grant Parent Run IDs differ",
        )
        require_equal(
            lineage["rootRunId"],
            lineage["parentRunId"],
            "depth-1 root and Parent Run IDs differ",
        )
        if lineage["rootRunId"] == launch["body"]["attempt"]["runId"]:
            raise VerificationError("depth-1 Child Run equals its root Run")
    check_child_registry(grant, launch)
    check_event_bindings(launch, event)

    require_equal(
        cron_template["owner"], grant["subject"], "Cron/grant subjects differ"
    )
    require_equal(
        cron_template["skill"]["packageFingerprint"],
        grant["packageFingerprint"],
        "Cron/grant package fingerprints differ",
    )
    require_equal(
        cron_template["skill"]["runtimeBundleFingerprint"],
        grant["runtimeBundleFingerprint"],
        "Cron/grant runtime fingerprints differ",
    )
    require_equal(
        cron_template["grant"]["grantId"], grant["grantId"], "Cron/grant IDs differ"
    )
    require_equal(
        cron_template["grant"]["grantFingerprint"],
        launch["body"]["grantFingerprint"],
        "Cron/launch Grant fingerprints differ",
    )
    require_equal(
        cron_template["grant"]["registryFingerprint"],
        launch["body"]["toolRegistry"]["registryFingerprint"],
        "Cron/launch Registry fingerprints differ",
    )
    require_equal(cron_template["budget"], grant["budget"], "Cron/grant budgets differ")
    if cron_template["schedule"]["calculator"] != "robfig-cron/v3.0.1+go-tzdata":
        raise VerificationError("Cron calculator version is not frozen")

    issued = datetime.fromisoformat(grant["issuedAt"].replace("Z", "+00:00"))
    expires = datetime.fromisoformat(grant["expiresAt"].replace("Z", "+00:00"))
    if expires <= issued:
        raise VerificationError("grant expiry does not follow issuance")

    require_equal(
        draft["sourceRunId"], event["runId"], "Draft/event Run IDs differ"
    )
    require_equal(
        draft["basePackageFingerprint"],
        grant["packageFingerprint"],
        "Draft/grant base package fingerprints differ",
    )
    require_equal(
        draft["runtimeBundleFingerprint"],
        grant["runtimeBundleFingerprint"],
        "Draft/grant runtime fingerprints differ",
    )
    if draft["proposedPackageFingerprint"] == draft["basePackageFingerprint"]:
        raise VerificationError("Draft proposed package equals its immutable base")
    source_evidence = [
        item for item in draft["evidence"] if item["kind"] == "source_package"
    ]
    run_evidence = [
        item for item in draft["evidence"] if item["kind"] == "run_event"
    ]
    if len(source_evidence) != 1 or source_evidence[0]["ref"] != draft["basePackageFingerprint"]:
        raise VerificationError("Draft source-package evidence does not bind the base")
    if not any(item["ref"] == event["eventId"] for item in run_evidence):
        raise VerificationError("Draft Run evidence does not bind the source event")
    test_paths = {item["path"] for item in draft["tests"]}
    changed_paths = set(draft["changedPaths"])
    if not test_paths.issubset(changed_paths):
        raise VerificationError("Draft test inventory is not bound to changed paths")
    covered_paths = {
        path
        for item in draft["evidence"]
        if item["kind"] == "run_event"
        for path in item["paths"]
    }
    if not changed_paths.issubset(covered_paths):
        raise VerificationError("Draft changed paths are not covered by Run evidence")

    policy_path = PROJECT_DIR / "config" / "agent-runner" / "production-policy.json"
    production_policy_bytes = policy_path.read_bytes()
    require_equal(
        production_policy,
        json.loads(production_policy_bytes),
        "production policy fixture differs from the frozen policy",
    )
    policy_fingerprint = "sha256:" + hashlib.sha256(production_policy_bytes).hexdigest()
    require_equal(
        production_closure["release"]["operationsPolicySha256"],
        policy_fingerprint,
        "production closure does not bind the frozen operations policy",
    )
    required_checks = {
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
    }
    check_ids = [item["id"] for item in production_closure["checks"]]
    if len(check_ids) != len(set(check_ids)) or set(check_ids) != required_checks:
        raise VerificationError("production closure check set is incomplete or duplicated")
    isolation = next(
        item for item in production_closure["checks"]
        if item["id"] == "exact_host_isolation"
    )
    if (
        production_closure["evidenceClass"] != "template"
        or isolation["result"] != "isolation_unavailable"
        or isolation["detailCode"] != "ISOLATION_UNAVAILABLE"
    ):
        raise VerificationError("committed production closure fixture is not honestly held")
    if any(
        value != 0
        for key, value in production_closure["cleanup"].items()
        if key != "promotionRecordRetained"
    ):
        raise VerificationError("committed production closure fixture has cleanup residue")


def check_document_anchors() -> None:
    requirements: dict[Path, tuple[str, ...]] = {
        PROJECT_DIR / "docs" / "architecture" / "agent-runtime.md": (
            "C4 — Context",
            "ArchiMate cross-layer blueprint",
            "Trust boundaries and STRIDE",
            "Kill Switch hierarchy",
            "migration `088`",
            "migration `089`",
            "migration `090`",
            "Agent Center and held Shadow control",
            "旧版技能已退役",
            "G20.10",
            "promotion-evidence gate",
        ),
        CONTRACT_DIR / "agent-runtime.md": (
            "Durable state machine",
            "Prepare / Commit protocol",
            "Child Agent contract",
            "G20.5 implementation signatures",
            "G20.6 implementation signatures",
            "G20.7 implementation signatures",
            "G20.8 implementation signatures",
            "Agent Center and held Shadow",
            "Isolation Acceptance Suite",
            "CODE_EXECUTION_UNAVAILABLE",
            "Production closure contract",
            "verify-agent-production-closure.sh",
        ),
        PROJECT_DIR / "docs" / "deployment" / "agent-runtime.md": (
            "AGENT_RUNTIME_ENABLED=false",
            "rootless OCI",
            "Kill Switch operations",
            "Child delegation operations boundary",
            "Cron scheduling operations boundary",
            "Draft learning operations boundary",
            "Agent Center and Shadow operations boundary",
            "Legacy Skill cutover and rollback",
            "Production closure evidence gate",
            "`outcome_unknown` operator workflow",
        ),
        PROJECT_DIR / "docs" / "tracking" / "g20-agent-runtime-plan.md": (
            "G20.0",
            "G20.9",
            "G20.5",
            "G20.6",
            "G20.7",
            "G20.8",
            "migration `090`",
            "delegate_task",
            "hard delete",
            "source/operations closure complete",
            "PROMOTION_READY",
        ),
    }
    for path, anchors in requirements.items():
        try:
            text = path.read_text(encoding="utf-8")
        except OSError as error:
            raise VerificationError(
                f"cannot read required document {path}: {error}"
            ) from error
        for anchor in anchors:
            if anchor not in text:
                raise VerificationError(f"missing anchor {anchor!r} in {path}")


def check_markdown_links() -> None:
    documents = (
        PROJECT_DIR / "docs" / "architecture" / "agent-runtime.md",
        CONTRACT_DIR / "agent-runtime.md",
        PROJECT_DIR / "docs" / "deployment" / "agent-runtime.md",
        PROJECT_DIR / "docs" / "tracking" / "g20-agent-runtime-plan.md",
        PROJECT_DIR / "docs" / "README.md",
        PROJECT_DIR / "docs" / "architecture" / "README.md",
        CONTRACT_DIR / "README.md",
        PROJECT_DIR / "docs" / "deployment" / "README.md",
    )
    link_pattern = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
    for document in documents:
        text = document.read_text(encoding="utf-8")
        for raw_target in link_pattern.findall(text):
            target = raw_target.split("#", 1)[0]
            if not target or "://" in target or target.startswith("mailto:"):
                continue
            resolved = (document.parent / target).resolve()
            try:
                resolved.relative_to(REPO_ROOT.resolve())
            except ValueError as error:
                raise VerificationError(
                    f"link escapes repository in {document}: {raw_target}"
                ) from error
            if not resolved.exists():
                raise VerificationError(f"broken link in {document}: {raw_target}")


def check_fail_closed_source() -> None:
    service = (
        PROJECT_DIR / "backend" / "internal" / "codejobs" / "service.go"
    ).read_text(encoding="utf-8")
    handler = (
        PROJECT_DIR / "backend" / "internal" / "codejobs" / "handler.go"
    ).read_text(encoding="utf-8")
    if "ErrCodeExecutionUnavailable" not in service:
        raise VerificationError("code execution service no longer fails closed")
    if "CODE_EXECUTION_UNAVAILABLE" not in handler:
        raise VerificationError("code execution unavailable response is missing")


def check_product_migration_source() -> None:
    migration = (
        PROJECT_DIR / "backend" / "migrations" / "090_agent_product_shadow.up.sql"
    ).read_text(encoding="utf-8")
    down = (
        PROJECT_DIR / "backend" / "migrations" / "090_agent_product_shadow.down.sql"
    ).read_text(encoding="utf-8")
    for signature in (
        "CREATE VIEW agent_product_runs",
        "CREATE FUNCTION agent_product_get_artifact(",
        "CREATE FUNCTION agent_product_cancel_run(",
        "CREATE FUNCTION agent_product_shadow_snapshot(",
        "CREATE FUNCTION agent_product_append_shadow_observation(",
        "mode IN ('synthetic','read_only')",
        "'ISOLATION_UNAVAILABLE'",
    ):
        if signature not in migration:
            raise VerificationError(f"migration 090 is missing {signature!r}")
    for forbidden in (
        "GRANT INSERT ON agent_shadow_",
        "GRANT UPDATE ON agent_shadow_",
        "GRANT DELETE ON agent_shadow_",
    ):
        if forbidden.lower() in migration.lower():
            raise VerificationError(f"migration 090 grants direct Shadow DML: {forbidden}")
    if "AGENT_PRODUCT_DOWN_DATA_EXISTS" not in down:
        raise VerificationError("migration 090 down is not data guarded")
    if "DROP FUNCTION agent_product_append_shadow_observation(" not in down:
        raise VerificationError("migration 090 down omits the Shadow observation function")


def check_product_service_source() -> None:
    service = (
        PROJECT_DIR / "backend" / "internal" / "agentcontrol" / "service.go"
    ).read_text(encoding="utf-8")
    if "return ErrIsolationUnavailable" not in service:
        raise VerificationError("Agent product Runtime no longer fails closed")
    if "shadowAdapter.Observe" not in service:
        raise VerificationError("held Shadow does not use the injected adapter seam")
    if "os/exec" in service or "exec.Command" in service or "podman" in service.lower():
        raise VerificationError("Agent product service contains an in-process executor")


def check_legacy_retirement_source() -> None:
    retirement = (
        PROJECT_DIR
        / "frontend"
        / "src"
        / "store"
        / "storage"
        / "legacySkillRetirement.ts"
    ).read_text(encoding="utf-8")
    cutover = (PROJECT_DIR / "scripts" / "cutover-legacy-skills.sql").read_text(
        encoding="utf-8"
    )
    for retired_field in (
        "installedSkills",
        "customSkills",
        "activeSkillIds",
        "skillAutoSelect",
        "skillCatalogs",
        "skillCatalogTimestamps",
        "skillDefinitions",
        "skillDefinitionTimestamps",
    ):
        if retired_field not in retirement:
            raise VerificationError(
                f"legacy Skill retirement omits field: {retired_field}"
            )
    if not (PROJECT_DIR / "scripts" / "verify-agent-legacy-cutover.sh").is_file():
        raise VerificationError("legacy Skill cutover gate is missing")
    if "metadata = metadata - 'activeSkills'" not in cutover:
        raise VerificationError("legacy Skill database selection is not retired")
    if "LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED" not in cutover:
        raise VerificationError("legacy Skill database cutover is not backup-gated")
    if (PROJECT_DIR / "frontend" / "src" / "lib" / "skills").exists():
        raise VerificationError("legacy Skill executable library still exists")


def check_product_shadow_source() -> None:
    check_product_migration_source()
    check_product_service_source()
    check_legacy_retirement_source()


def main() -> int:
    try:
        instances = check_schemas_and_fixtures()
        check_cross_contracts(instances)
        check_document_anchors()
        check_markdown_links()
        check_fail_closed_source()
        check_product_shadow_source()
    except VerificationError as error:
        print(f"Agent Runtime Phase 0 verification: FAILED: {error}", file=sys.stderr)
        return 1
    print(
        "Agent Runtime Phase 0 verification: passed "
        "(schemas, fixtures, lineage, Agent product/Shadow, production closure, "
        "docs, fail-closed routes)"
    )
    print(
        "Agent Runtime Phase 0 verification: production Runtime remains disabled; "
        "rootless OCI Isolation Acceptance was not executed"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
