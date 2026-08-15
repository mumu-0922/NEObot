#!/usr/bin/env python3
"""Manage the explicitly non-production Agent Runner local-test profile."""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from agent_runner_local_test import (
    LocalTestError,
    bootstrap_system,
    default_install_root,
    host_status,
    install_toolchain,
    load_toolchain_lock,
    rollback_system,
    rollback_user,
    run_smoke,
    validate_install_root,
)

SCRIPT_DIR = Path(__file__).resolve().parent
PROJECT_DIR = SCRIPT_DIR.parent
DEFAULT_LOCK = (
    PROJECT_DIR / "config" / "agent-runner" / "local-test-toolchain.lock.json"
)


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(
        description="Prepare and verify the WSL2 Agent Runner local-test profile."
    )
    value.add_argument(
        "command",
        choices=(
            "status",
            "bootstrap-system",
            "install-toolchain",
            "smoke",
            "rollback-user",
            "rollback-system",
        ),
    )
    value.add_argument("--lock", type=Path, default=DEFAULT_LOCK)
    value.add_argument("--install-root", type=Path, default=default_install_root())
    value.add_argument("--json", action="store_true")
    return value


def main() -> int:
    args = parser().parse_args()
    try:
        lock = load_toolchain_lock(args.lock)
        root = (
            validate_install_root(args.install_root)
            if args.command not in {"bootstrap-system", "rollback-system"}
            else args.install_root
        )
        if args.command == "status":
            result: object = host_status(lock, root)
        elif args.command == "bootstrap-system":
            operator = os.environ.get("SUDO_USER", "")
            result = {
                "code": bootstrap_system(lock, operator),
                "evidenceClass": "local_test",
                "productionEligible": False,
            }
        elif args.command == "install-toolchain":
            result = {
                "code": install_toolchain(lock, root, PROJECT_DIR),
                "evidenceClass": "local_test",
                "productionEligible": False,
            }
        elif args.command == "smoke":
            result = run_smoke(lock, root)
        elif args.command == "rollback-user":
            result = {
                "code": rollback_user(lock, root),
                "evidenceClass": "local_test",
                "productionEligible": False,
            }
        else:
            result = {
                "code": rollback_system(),
                "evidenceClass": "local_test",
                "productionEligible": False,
            }
        if args.json or isinstance(result, dict):
            print(json.dumps(result, indent=2, sort_keys=True))
        else:
            print(result)
        return 0
    except LocalTestError as error:
        print(str(error), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
