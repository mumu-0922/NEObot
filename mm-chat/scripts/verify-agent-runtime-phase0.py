#!/usr/bin/env python3
"""Offline structural verification for the Agent Runtime Phase 0 contracts."""

from __future__ import annotations

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

    issued = datetime.fromisoformat(grant["issuedAt"].replace("Z", "+00:00"))
    expires = datetime.fromisoformat(grant["expiresAt"].replace("Z", "+00:00"))
    if expires <= issued:
        raise VerificationError("grant expiry does not follow issuance")


def check_document_anchors() -> None:
    requirements: dict[Path, tuple[str, ...]] = {
        PROJECT_DIR / "docs" / "architecture" / "agent-runtime.md": (
            "C4 — Context",
            "ArchiMate cross-layer blueprint",
            "Trust boundaries and STRIDE",
            "Kill Switch hierarchy",
            "migration `087`",
            "旧版技能已退役",
        ),
        CONTRACT_DIR / "agent-runtime.md": (
            "Durable state machine",
            "Prepare / Commit protocol",
            "Child Agent contract",
            "G20.5 implementation signatures",
            "Isolation Acceptance Suite",
            "CODE_EXECUTION_UNAVAILABLE",
        ),
        PROJECT_DIR / "docs" / "deployment" / "agent-runtime.md": (
            "AGENT_RUNTIME_ENABLED=false",
            "rootless OCI",
            "Kill Switch operations",
            "Child delegation operations boundary",
            "Legacy Skill cutover and rollback",
        ),
        PROJECT_DIR / "docs" / "tracking" / "g20-agent-runtime-plan.md": (
            "G20.0",
            "G20.9",
            "G20.5",
            "delegate_task",
            "hard delete",
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


def main() -> int:
    try:
        instances = check_schemas_and_fixtures()
        check_cross_contracts(instances)
        check_document_anchors()
        check_markdown_links()
        check_fail_closed_source()
    except VerificationError as error:
        print(f"Agent Runtime Phase 0 verification: FAILED: {error}", file=sys.stderr)
        return 1
    print(
        "Agent Runtime Phase 0 verification: passed "
        "(schemas, positive/negative fixtures, lineage cross-contracts, docs, fail-closed route)"
    )
    print(
        "Agent Runtime Phase 0 verification: production Runtime remains disabled; "
        "rootless OCI Isolation Acceptance was not executed"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
