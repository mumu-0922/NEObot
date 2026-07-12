"""Fail-closed operator replay CLI; dry-run unless ``--execute`` is explicit."""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import uuid
from collections.abc import Sequence

from mm_chat_rag.models import stable_error_code
from mm_chat_rag.postgres import replay_job, replay_outbox

_MAX_REASON_BYTES = 1024


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
    return parser


def _execution_inputs(
    parser: argparse.ArgumentParser, args: argparse.Namespace
) -> None:
    if not args.execute:
        return
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


async def run(argv: Sequence[str] | None = None) -> int:
    """Validate an intent or execute exactly one replay function."""
    parser = build_parser()
    args = parser.parse_args(argv)
    _execution_inputs(parser, args)
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


def main() -> None:
    """Console entry point."""
    raise SystemExit(asyncio.run(run()))
