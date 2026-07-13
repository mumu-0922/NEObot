"""No-network-by-default MinerU lifecycle Evidence Capture CLI."""

from __future__ import annotations

import argparse
import hashlib
import os
import sys
import time
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Never, cast

import httpx

from tools.provider_capture_common import (
    CAPTURE_PROXY_ENV,
    CaptureError,
    JsonObject,
    canonical_json_bytes,
    deterministic_synthetic_pdf,
    evidence_sha256,
    format_observed_at,
    parse_observed_at,
    validate_capture_proxy_url,
)
from tools.provider_capture_evidence import (
    validate_evidence_snapshot,
    validate_output_destination,
    write_evidence_snapshot_at,
)
from tools.provider_capture_http import LIMITS, TIMEOUT
from tools.provider_capture_mineru_lifecycle_http import (
    LIFECYCLE_SCHEMA_VERSION,
    POLL_CALL_LIMIT,
    POLL_INTERVAL_SECONDS,
    capture_mineru_lifecycle,
)
from tools.provider_capture_mineru_targets import RESULT_HOST, UPLOAD_HOST

type Environment = Mapping[str, str]
type Transport = httpx.BaseTransport | None
type Sleeper = Callable[[float], None]

_VISIBLE_ASCII_MIN = 33
_VISIBLE_ASCII_MAX = 126
_MAX_CREDENTIAL_BYTES = 4096
_INCOMPLETE_EXIT = 3


@dataclass(frozen=True, slots=True)
class LifecycleRuntime:
    """Injectable boundaries for deterministic no-network tests."""

    environ: Environment | None = None
    transport: Transport = None
    output_base: Path | None = None
    now: datetime | None = None
    sleeper: Sleeper | None = None


class SafeArgumentParser(argparse.ArgumentParser):
    """Never echo caller-controlled argv values."""

    def error(self, message: str) -> Never:  # noqa: ARG002
        raise CaptureError("CLI_ARGUMENT_INVALID")


def dry_run_plan() -> JsonObject:
    """Return the immutable lifecycle plan without reading Key or network state."""
    return {
        "credentials": {
            "dotenvLoaded": False,
            "requiredEnvironmentNames": ["MINERU_API_KEY"],
        },
        "evidenceFilesCreated": False,
        "mode": "dry-run",
        "networkEnabled": False,
        "operations": [
            {
                "count": 1,
                "method": "POST",
                "path": "/api/v4/file-urls/batch",
                "stage": "allocate_upload",
            },
            {
                "count": 1,
                "method": "PUT",
                "stage": "upload",
                "targetHost": UPLOAD_HOST,
            },
            {
                "count": POLL_CALL_LIMIT,
                "intervalSeconds": int(POLL_INTERVAL_SECONDS),
                "method": "GET",
                "path": "/api/v4/extract-results/batch/{batch_id}",
                "stage": "poll_batch",
            },
            {
                "count": 1,
                "method": "GET",
                "stage": "download_result",
                "targetHost": RESULT_HOST,
            },
        ],
        "proxy": {
            "automaticEnvironmentProxyLoaded": False,
            "explicitEnvironmentName": CAPTURE_PROXY_ENV,
            "privateAddressOnly": True,
        },
        "retries": 0,
        "schemaVersion": LIFECYCLE_SCHEMA_VERSION,
        "syntheticInputsOnly": True,
    }


def capture_lifecycle(
    *,
    observed_at: datetime,
    runtime: LifecycleRuntime,
) -> JsonObject:
    """Execute the fixed lifecycle and return a closed redacted v2 Snapshot."""
    environ = os.environ if runtime.environ is None else runtime.environ
    api_key = _load_credential(environ)
    proxy_url = validate_capture_proxy_url(environ.get(CAPTURE_PROXY_ENV))
    pdf = deterministic_synthetic_pdf()
    with httpx.Client(
        transport=runtime.transport,
        timeout=TIMEOUT,
        limits=LIMITS,
        trust_env=False,
        follow_redirects=False,
        proxy=proxy_url,
    ) as client:
        provider, budget = capture_mineru_lifecycle(
            client,
            api_key,
            pdf,
            sleeper=runtime.sleeper or time.sleep,
        )
    snapshot: JsonObject = {
        "budgets": {"mineru": budget},
        "captureMode": "authorized_execute",
        "captureOutcome": provider["state"],
        "observedAt": format_observed_at(observed_at),
        "providers": [provider],
        "schemaVersion": LIFECYCLE_SCHEMA_VERSION,
        "syntheticArtifacts": [
            {
                "byteCount": len(pdf),
                "kind": "deterministic_synthetic_pdf",
                "sha256": hashlib.sha256(pdf).hexdigest(),
            }
        ],
    }
    validate_evidence_snapshot(snapshot)
    return snapshot


def build_parser() -> argparse.ArgumentParser:
    """Build a fixed CLI with no stage, target, budget, or retry controls."""
    parser = SafeArgumentParser(description="Capture MinerU lifecycle evidence")
    parser.add_argument(
        "--execute",
        action="store_true",
        help="enable one fixed lifecycle; omitted means no-network dry-run",
    )
    parser.add_argument(
        "--observed-at",
        help="UTC RFC3339 evidence time; defaults to current UTC second",
    )
    parser.add_argument(
        "--output-dir",
        default="provider-capture-mineru-lifecycle",
        help="new direct child directory for one evidence snapshot",
    )
    return parser


def main(
    argv: Sequence[str] | None = None,
    *,
    runtime: LifecycleRuntime | None = None,
) -> int:
    """Run without printing credentials, URLs, IDs, Provider errors, or content."""
    active_runtime = runtime or LifecycleRuntime()
    try:
        args = build_parser().parse_args(argv)
        if not args.execute:
            _write_stdout(dry_run_plan())
            return 0
        return _execute(args, active_runtime)
    except CaptureError as error:
        sys.stderr.write(f"{error}\n")
        return 2
    except Exception:  # noqa: BLE001
        sys.stderr.write("CAPTURE_FAILED\n")
        return 2


def _execute(args: argparse.Namespace, runtime: LifecycleRuntime) -> int:
    observed_at = (
        parse_observed_at(args.observed_at)
        if args.observed_at is not None
        else (runtime.now or datetime.now(tz=UTC))
    )
    output_name = cast("str", args.output_dir)
    validate_output_destination(runtime.output_base, output_name)
    snapshot = capture_lifecycle(observed_at=observed_at, runtime=runtime)
    output_path = write_evidence_snapshot_at(
        runtime.output_base,
        output_name,
        snapshot,
    )
    outcome = cast("str", snapshot["captureOutcome"])
    _write_stdout(
        {
            "captureOutcome": outcome,
            "evidenceFile": output_path.name,
            "evidenceSha256": evidence_sha256(canonical_json_bytes(snapshot)),
            "status": "evidence_written",
        }
    )
    return 0 if outcome == "lifecycle_complete" else _INCOMPLETE_EXIT


def _load_credential(environ: Environment) -> str:
    value = environ.get("MINERU_API_KEY", "")
    if (
        not value
        or len(value) > _MAX_CREDENTIAL_BYTES
        or any(
            not _VISIBLE_ASCII_MIN <= ord(character) <= _VISIBLE_ASCII_MAX
            for character in value
        )
    ):
        raise CaptureError("CAPTURE_CREDENTIALS_MISSING_OR_INVALID")
    return value


def _write_stdout(value: JsonObject) -> None:
    sys.stdout.buffer.write(canonical_json_bytes(value))


__all__ = [
    "LifecycleRuntime",
    "capture_lifecycle",
    "dry_run_plan",
    "main",
]


if __name__ == "__main__":
    raise SystemExit(main())
