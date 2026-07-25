"""Fail-closed operator replay CLI; dry-run unless ``--execute`` is explicit."""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import uuid
from collections.abc import Sequence
from pathlib import Path

from mm_chat_rag.generation_gate_report import (
    GenerationOperatorError,
    load_and_validate_gate_report,
)
from mm_chat_rag.generation_operator import (
    abandon_structure_generation,
    activate_structure_generation,
    begin_structure_generation,
    generation_status,
    rollback_structure_generation,
    verify_structure_generation,
)
from mm_chat_rag.models import stable_error_code
from mm_chat_rag.postgres import replay_job, replay_outbox
from mm_chat_rag.structure_chunking import SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH

_MAX_REASON_BYTES = 1024
_SHA256_HEX_LENGTH = 64


def _uuid(value: str) -> uuid.UUID:
    try:
        return uuid.UUID(value)
    except ValueError as error:
        raise argparse.ArgumentTypeError("must be a UUID") from error


def _error_code(value: str) -> str:
    try:
        return stable_error_code(value)
    except ValueError as error:
        raise argparse.ArgumentTypeError(str(error)) from error


def _sha256(value: str) -> str:
    normalized = value.strip().lower()
    if len(normalized) != _SHA256_HEX_LENGTH or any(
        character not in "0123456789abcdef" for character in normalized
    ):
        raise argparse.ArgumentTypeError("must be a lower-case SHA-256")
    return normalized


def _positive_int(value: str) -> int:
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("must be positive")
    return parsed


def _reason(value: str) -> str:
    normalized = value.strip()
    if not normalized or len(normalized.encode()) > _MAX_REASON_BYTES:
        raise argparse.ArgumentTypeError("must contain 1..1024 UTF-8 bytes")
    return normalized


def build_parser() -> argparse.ArgumentParser:
    """Build the intentionally explicit replay command surface."""
    parser = argparse.ArgumentParser(prog="rag-replay")
    subparsers = parser.add_subparsers(dest="kind", required=True)
    for kind in ("outbox", "job"):
        command = subparsers.add_parser(kind)
        command.add_argument("--id", required=True, type=_uuid)
        command.add_argument("--expected-error-code", required=True, type=_error_code)
        command.add_argument("--operator-id", type=_uuid)
        command.add_argument("--reason")
        command.add_argument("--execute", action="store_true")
        if kind == "job":
            command.add_argument("--successor-job-id", type=_uuid)
    subparsers.add_parser("generation-status")
    for kind in ("generation-begin", "generation-verify"):
        command = subparsers.add_parser(kind)
        command.add_argument("--execute", action="store_true")
    abandon = subparsers.add_parser("generation-abandon")
    abandon.add_argument("--candidate-generation-id", required=True, type=_uuid)
    abandon.add_argument("--expected-head-revision", required=True, type=_positive_int)
    abandon.add_argument("--expected-manifest-hash", required=True, type=_sha256)
    abandon.add_argument("--operator-id", required=True, type=_uuid)
    abandon.add_argument("--reason", required=True, type=_reason)
    abandon.add_argument("--confirm-abandon", action="store_true")
    abandon.add_argument("--execute", action="store_true")
    activate = subparsers.add_parser("generation-activate")
    activate.add_argument("--gate-report", required=True, type=Path)
    activate.add_argument("--gate-report-sha256", required=True, type=_sha256)
    activate.add_argument("--operator-id", required=True, type=_uuid)
    activate.add_argument("--confirm-activation", action="store_true")
    activate.add_argument("--execute", action="store_true")
    rollback = subparsers.add_parser("generation-rollback")
    rollback.add_argument("--active-generation-id", required=True, type=_uuid)
    rollback.add_argument("--target-generation-id", required=True, type=_uuid)
    rollback.add_argument("--expected-head-revision", required=True, type=int)
    rollback.add_argument("--active-manifest-hash", required=True, type=_sha256)
    rollback.add_argument("--target-manifest-hash", required=True, type=_sha256)
    rollback.add_argument("--confirm-rollback", action="store_true")
    rollback.add_argument("--execute", action="store_true")
    return parser


def _execution_inputs(
    parser: argparse.ArgumentParser, args: argparse.Namespace
) -> None:
    if not getattr(args, "execute", False):
        return
    if args.kind in {"outbox", "job"}:
        if args.operator_id is None:
            parser.error("--operator-id is required with --execute")
        if (
            args.reason is None
            or not args.reason.strip()
            or len(args.reason.encode()) > _MAX_REASON_BYTES
        ):
            parser.error("--reason (1..1024 bytes) is required with --execute")
        if args.kind == "job" and args.successor_job_id is None:
            parser.error("--successor-job-id is required with --execute")
    if args.kind == "generation-activate" and not args.confirm_activation:
        parser.error("--confirm-activation is required with --execute")
    if args.kind == "generation-abandon" and not args.confirm_abandon:
        parser.error("--confirm-abandon is required with --execute")
    if args.kind == "generation-rollback":
        if not args.confirm_rollback:
            parser.error("--confirm-rollback is required with --execute")
        if args.expected_head_revision < 1:
            parser.error("--expected-head-revision must be positive")


async def run(argv: Sequence[str] | None = None) -> int:
    """Validate an intent or execute exactly one replay function."""
    parser = build_parser()
    args = parser.parse_args(argv)
    _execution_inputs(parser, args)
    if args.kind.startswith("generation-"):
        return await _run_generation(parser, args)
    intent = {
        "kind": args.kind,
        "id": str(args.id),
        "expected_error_code": args.expected_error_code,
        "mode": "execute" if args.execute else "dry-run",
    }
    if not args.execute:
        print(json.dumps(intent, separators=(",", ":")))  # noqa: T201
        return 0
    database_url = os.environ.get("RAG_REPLAY_DATABASE_URL", "").strip()
    if not database_url:
        parser.error("RAG_REPLAY_DATABASE_URL is required with --execute")

    if args.kind == "outbox":
        succeeded = await replay_outbox(
            database_url,
            event_id=args.id,
            expected_error_code=args.expected_error_code,
            operator_id=args.operator_id,
            reason=args.reason.strip(),
        )
    else:
        succeeded = await replay_job(
            database_url,
            job_id=args.id,
            expected_error_code=args.expected_error_code,
            successor_job_id=args.successor_job_id,
            operator_id=args.operator_id,
            reason=args.reason.strip(),
        )
    print(  # noqa: T201
        json.dumps({**intent, "succeeded": succeeded}, separators=(",", ":"))
    )
    return 0 if succeeded else 1


async def _run_generation(
    parser: argparse.ArgumentParser,
    args: argparse.Namespace,
) -> int:
    if args.kind != "generation-status" and not args.execute:
        intent: dict[str, object] = {
            "kind": args.kind,
            "mode": "dry-run",
        }
        if args.kind in {"generation-begin", "generation-verify"}:
            intent["chunkProfileHash"] = SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH
        if args.kind == "generation-abandon":
            intent.update(
                {
                    "candidateGenerationId": str(args.candidate_generation_id),
                    "expectedHeadRevision": args.expected_head_revision,
                    "expectedManifestHash": args.expected_manifest_hash,
                    "operatorId": str(args.operator_id),
                    "reason": args.reason,
                }
            )
        print(  # noqa: T201
            json.dumps(
                intent,
                separators=(",", ":"),
                sort_keys=True,
            )
        )
        return 0
    database_url = os.environ.get("RAG_REPLAY_DATABASE_URL", "").strip()
    if not database_url:
        parser.error("RAG_REPLAY_DATABASE_URL is required")

    try:
        if args.kind == "generation-status":
            result: object = await generation_status(database_url)
        elif args.kind == "generation-begin":
            result = await begin_structure_generation(database_url)
        elif args.kind == "generation-verify":
            result = await verify_structure_generation(database_url)
        elif args.kind == "generation-abandon":
            result = {
                "abandoned": await abandon_structure_generation(
                    database_url,
                    candidate_generation_id=args.candidate_generation_id,
                    expected_head_revision=args.expected_head_revision,
                    expected_manifest_hash=args.expected_manifest_hash,
                    operator_id=args.operator_id,
                    reason=args.reason,
                ),
                "candidateGenerationId": str(args.candidate_generation_id),
                "kind": args.kind,
            }
        elif args.kind == "generation-activate":
            report, report_hash = load_and_validate_gate_report(
                args.gate_report,
                args.gate_report_sha256,
            )
            result = {
                "activated": await activate_structure_generation(
                    database_url,
                    gate_report=report,
                    gate_report_sha256=report_hash,
                    operator_id=args.operator_id,
                ),
                "gateReportSha256": report_hash,
                "kind": args.kind,
            }
        elif args.kind == "generation-rollback":
            result = {
                "kind": args.kind,
                "rolledBack": await rollback_structure_generation(
                    database_url,
                    active_generation_id=args.active_generation_id,
                    target_generation_id=args.target_generation_id,
                    expected_head_revision=args.expected_head_revision,
                    active_manifest_hash=args.active_manifest_hash,
                    target_manifest_hash=args.target_manifest_hash,
                ),
            }
        else:  # pragma: no cover - argparse owns the closed command set.
            parser.error("unknown generation operator command")
    except GenerationOperatorError as error:
        print(  # noqa: T201
            json.dumps(
                {"error": str(error), "kind": args.kind, "succeeded": False},
                separators=(",", ":"),
            )
        )
        return 1
    print(json.dumps(result, separators=(",", ":"), sort_keys=True))  # noqa: T201
    if isinstance(result, dict) and (
        result.get("abandoned") is False
        or result.get("activated") is False
        or result.get("rolledBack") is False
    ):
        return 1
    return 0


def main() -> None:
    """Console entry point."""
    raise SystemExit(asyncio.run(run()))
