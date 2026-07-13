"""Fail-closed, operator-invoked Provider Capture Harness.

The module is intentionally outside ``mm_chat_rag`` and is not installed as a
console script. Dry-run is the default. Network access requires ``--execute``
and credentials supplied only through the process environment.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import sys
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Never, cast

import httpx

from tools.provider_capture_common import (
    CAPTURE_PROXY_ENV,
    CaptureError,
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    deterministic_synthetic_pdf,
    evidence_sha256,
    format_observed_at,
    parse_observed_at,
    selected_providers,
    validate_capture_proxy_url,
    validate_request_target,
)
from tools.provider_capture_evidence import (
    validate_evidence_snapshot,
    validate_output_destination,
    write_evidence_snapshot,
    write_evidence_snapshot_at,
)
from tools.provider_capture_http import (
    LIMITS,
    TIMEOUT,
    capture_jina,
    capture_mineru_submit,
)

type Environment = Mapping[str, str]
type Transport = httpx.BaseTransport | None

_VISIBLE_ASCII_MIN = 33
_VISIBLE_ASCII_MAX = 126
_MAX_CREDENTIAL_BYTES = 4096
_UNKNOWN_SUBMISSION_EXIT = 3


@dataclass(frozen=True, slots=True)
class CaptureRuntime:
    """Injectable process boundaries without widening the public call budget."""

    environ: Environment | None = None
    transport: Transport = None
    output_base: Path | None = None
    now: datetime | None = None


class SafeArgumentParser(argparse.ArgumentParser):
    """Prevent argparse from echoing arbitrary argv values on failure."""

    def error(self, message: str) -> Never:  # noqa: ARG002
        raise CaptureError("CLI_ARGUMENT_INVALID")


def dry_run_plan(provider: str = "all") -> JsonObject:
    """Describe the fixed plan without inspecting credentials or networking."""
    providers = selected_providers(provider)
    return {
        "concurrency": 1,
        "credentials": {
            "cliArgumentsAccepted": False,
            "dotenvLoaded": False,
            "requiredEnvironmentNames": _required_environment(providers),
        },
        "evidenceFilesCreated": False,
        "mode": "dry-run",
        "networkEnabled": False,
        "operations": _dry_run_operations(providers),
        "proxy": {
            "automaticEnvironmentProxyLoaded": False,
            "explicitEnvironmentName": CAPTURE_PROXY_ENV,
            "privateAddressOnly": True,
        },
        "retries": 0,
        "syntheticInputsOnly": True,
    }


def capture(
    provider: str,
    *,
    observed_at: datetime,
    runtime: CaptureRuntime,
) -> JsonObject:
    """Execute the selected fixed-budget captures and return safe evidence."""
    providers = selected_providers(provider)
    environ = os.environ if runtime.environ is None else runtime.environ
    keys = _load_credentials(providers, environ)
    proxy_url = validate_capture_proxy_url(environ.get(CAPTURE_PROXY_ENV))
    records: list[JsonValue] = []
    budgets: JsonObject = {}
    artifacts: list[JsonValue] = []
    with httpx.Client(
        transport=runtime.transport,
        timeout=TIMEOUT,
        limits=LIMITS,
        trust_env=False,
        follow_redirects=False,
        proxy=proxy_url,
    ) as client:
        if "jina" in providers:
            records.append(capture_jina(client, keys["jina"]))
            budgets["jina"] = {"allowedCalls": 3, "usedCalls": 3}
        if "mineru" in providers:
            pdf = deterministic_synthetic_pdf()
            records.append(capture_mineru_submit(client, keys["mineru"], pdf))
            budgets["mineru"] = _mineru_budget()
            artifacts.append(_synthetic_pdf_evidence(pdf))
    snapshot = _build_snapshot(observed_at, records, budgets, artifacts)
    validate_evidence_snapshot(snapshot)
    return snapshot


def build_parser() -> argparse.ArgumentParser:
    """Build the intentionally small, fixed-budget CLI parser."""
    parser = SafeArgumentParser(description="Capture redacted provider evidence")
    parser.add_argument(
        "--execute",
        action="store_true",
        help="enable the fixed network plan; omitted means no-network dry-run",
    )
    parser.add_argument(
        "--provider",
        choices=("all", "jina", "mineru"),
        default="all",
        help="fixed provider subset (default: all)",
    )
    parser.add_argument(
        "--observed-at",
        help="UTC RFC3339 evidence time; defaults to current UTC second",
    )
    parser.add_argument(
        "--output-dir",
        default="provider-captures",
        help="new direct child directory for one evidence snapshot",
    )
    return parser


def main(
    argv: Sequence[str] | None = None,
    *,
    runtime: CaptureRuntime | None = None,
) -> int:
    """Run the CLI without printing credentials or provider error details."""
    active_runtime = runtime or CaptureRuntime()
    try:
        args = build_parser().parse_args(argv)
        if not args.execute:
            plan = dry_run_plan(cast("str", args.provider))
            _write_stdout(plan)
            return 0
        return _execute_cli(args, active_runtime)
    except CaptureError as error:
        sys.stderr.write(f"{error}\n")
        return 2
    except Exception:  # noqa: BLE001 - never expose unexpected details.
        sys.stderr.write("CAPTURE_FAILED\n")
        return 2


def _execute_cli(args: argparse.Namespace, runtime: CaptureRuntime) -> int:
    observed_at = (
        parse_observed_at(args.observed_at)
        if args.observed_at is not None
        else (runtime.now or datetime.now(tz=UTC))
    )
    output_name = cast("str", args.output_dir)
    validate_output_destination(runtime.output_base, output_name)
    snapshot = capture(
        cast("str", args.provider),
        observed_at=observed_at,
        runtime=runtime,
    )
    output_path = write_evidence_snapshot_at(
        runtime.output_base,
        output_name,
        snapshot,
    )
    outcome = cast("str", snapshot["captureOutcome"])
    result: JsonObject = {
        "captureOutcome": outcome,
        "evidenceFile": output_path.name,
        "evidenceSha256": evidence_sha256(canonical_json_bytes(snapshot)),
        "status": "evidence_written",
    }
    _write_stdout(result)
    return _UNKNOWN_SUBMISSION_EXIT if outcome == "unknown_submission" else 0


def _dry_run_operations(providers: tuple[str, ...]) -> list[JsonValue]:
    operations: list[JsonValue] = []
    if "jina" in providers:
        operations.extend(_jina_dry_run_operations())
    if "mineru" in providers:
        operations.append(
            {
                "count": 1,
                "method": "POST",
                "path": "/api/v4/file-urls/batch",
                "provider": "mineru",
                "stage": "submit_only_no_signed_upload_or_poll",
            }
        )
    return operations


def _jina_dry_run_operations() -> list[JsonValue]:
    operations: list[JsonValue] = [
        {
            "count": 1,
            "dimensions": dimensions,
            "method": "POST",
            "path": "/v1/embeddings",
            "provider": "jina",
        }
        for dimensions in (1024, 2048)
    ]
    operations.append(
        {
            "count": 1,
            "documentCount": 2,
            "method": "POST",
            "path": "/v1/rerank",
            "provider": "jina",
        }
    )
    return operations


def _required_environment(providers: tuple[str, ...]) -> list[JsonValue]:
    return [
        name
        for name, selected in (
            ("JINA_API_KEY", "jina"),
            ("MINERU_API_KEY", "mineru"),
        )
        if selected in providers
    ]


def _load_credentials(
    providers: tuple[str, ...],
    environ: Environment,
) -> dict[str, str]:
    names = {"jina": "JINA_API_KEY", "mineru": "MINERU_API_KEY"}
    result: dict[str, str] = {}
    for provider in providers:
        value = environ.get(names[provider], "")
        if not _valid_credential(value):
            raise CaptureError("CAPTURE_CREDENTIALS_MISSING_OR_INVALID")
        result[provider] = value
    return result


def _valid_credential(value: str) -> bool:
    return (
        bool(value)
        and len(value) <= _MAX_CREDENTIAL_BYTES
        and all(
            _VISIBLE_ASCII_MIN <= ord(character) <= _VISIBLE_ASCII_MAX
            for character in value
        )
    )


def _mineru_budget() -> JsonObject:
    return {
        "allowedPollCalls": 0,
        "allowedSignedUploadCalls": 0,
        "allowedSubmitCalls": 1,
        "usedPollCalls": 0,
        "usedSignedUploadCalls": 0,
        "usedSubmitCalls": 1,
    }


def _synthetic_pdf_evidence(pdf: bytes) -> JsonObject:
    return {
        "byteCount": len(pdf),
        "kind": "deterministic_synthetic_pdf",
        "sha256": hashlib.sha256(pdf).hexdigest(),
    }


def _build_snapshot(
    observed_at: datetime,
    records: list[JsonValue],
    budgets: JsonObject,
    artifacts: list[JsonValue],
) -> JsonObject:
    outcome = (
        "unknown_submission"
        if any(
            isinstance(record, dict) and record.get("state") == "unknown_submission"
            for record in records
        )
        else "fixed_plan_complete"
    )
    return {
        "budgets": budgets,
        "captureMode": "authorized_execute",
        "captureOutcome": outcome,
        "observedAt": format_observed_at(observed_at),
        "providers": records,
        "schemaVersion": "mm-chat.provider-capture-evidence.v1",
        "syntheticArtifacts": artifacts,
    }


def _write_stdout(value: JsonObject) -> None:
    sys.stdout.buffer.write(canonical_json_bytes(value))


__all__ = [
    "CaptureError",
    "CaptureRuntime",
    "canonical_json_bytes",
    "capture",
    "deterministic_synthetic_pdf",
    "dry_run_plan",
    "evidence_sha256",
    "main",
    "validate_capture_proxy_url",
    "validate_evidence_snapshot",
    "validate_request_target",
    "write_evidence_snapshot",
]


if __name__ == "__main__":
    raise SystemExit(main())
