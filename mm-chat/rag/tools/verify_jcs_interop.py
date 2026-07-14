"""Run the independent Python, Go 1.22, and Node 22 offline JCS gate."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

_RAG_ROOT = Path(__file__).resolve().parents[1]
_DEFAULT_FIXTURES = _RAG_ROOT / "tests" / "fixtures" / "jcs"
_INTEROP_ROOT = _RAG_ROOT / "tests" / "interop" / "jcs"
_SHA256 = re.compile(r"^[0-9a-f]{64}$")
_GO_122 = re.compile(r"go version go(1\.22\.\d+) [\x21-\x7e]+/[\x21-\x7e]+\n\Z")
_NODE_22 = re.compile(r"v(22\.\d+\.\d+)\n\Z")
_GO_RUN_FAILURE_STDERR = b"exit status 1\n"
_TIMEOUT_SECONDS = 120
_MAX_ERROR_CODE_LENGTH = 80
_MAX_FAILED_CASE_LENGTH = 160


@dataclass(frozen=True, slots=True)
class RuntimeSpec:
    """One fixed, dependency-free conformance implementation."""

    name: str
    executable: str | None
    command: tuple[str, ...]
    version_command: tuple[str, ...] | None


def _environment() -> dict[str, str]:
    """Build a small environment with package and toolchain downloads disabled."""
    preserved = {
        key: os.environ[key]
        for key in ("HOME", "PATH", "SYSTEMROOT", "TEMP", "TMP", "TMPDIR")
        if key in os.environ
    }
    preserved.update(
        {
            "GO111MODULE": "off",
            "GOENV": "off",
            "GOPROXY": "off",
            "GOSUMDB": "off",
            "GOTOOLCHAIN": "local",
            "LANG": "C.UTF-8",
            "LC_ALL": "C.UTF-8",
            "PYTHONDONTWRITEBYTECODE": "1",
            "PYTHONNOUSERSITE": "1",
        }
    )
    return preserved


def _runtime_specs(fixtures: Path) -> tuple[RuntimeSpec, ...]:
    python_script = _INTEROP_ROOT / "python_jcs.py"
    go_script = _INTEROP_ROOT / "go_jcs.go"
    node_script = _INTEROP_ROOT / "node_jcs.mjs"
    go = shutil.which("go")
    node = shutil.which("node")
    return (
        RuntimeSpec(
            name="python",
            executable=sys.executable,
            command=(
                sys.executable,
                "-B",
                str(python_script),
                "--fixtures",
                str(fixtures),
            ),
            version_command=None,
        ),
        RuntimeSpec(
            name="go",
            executable=go,
            command=(
                go or "go",
                "run",
                str(go_script),
                "--fixtures",
                str(fixtures),
            ),
            version_command=((go or "go"), "version"),
        ),
        RuntimeSpec(
            name="node",
            executable=node,
            command=(
                node or "node",
                str(node_script),
                "--fixtures",
                str(fixtures),
            ),
            version_command=((node or "node"), "--version"),
        ),
    )


def _run_process(command: Sequence[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(  # noqa: S603 - fixed runtime commands only.
        command,
        cwd=_INTEROP_ROOT,
        env=_environment(),
        check=False,
        capture_output=True,
        timeout=_TIMEOUT_SECONDS,
    )


def _preflight_version(  # noqa: PLR0911 - each preflight failure is distinct.
    spec: RuntimeSpec,
) -> tuple[str, str | None]:
    if spec.version_command is None:
        return ".".join(str(item) for item in sys.version_info[:3]), None
    try:
        completed = _run_process(spec.version_command)
    except (OSError, subprocess.SubprocessError):
        return "", "VERSION_CHECK_FAILED"
    if completed.returncode != 0 or completed.stderr != b"":
        return "", "VERSION_CHECK_FAILED"
    try:
        stdout = completed.stdout.decode("ascii", errors="strict")
    except UnicodeDecodeError:
        return "", "VERSION_CHECK_FAILED"
    if spec.name == "go":
        match = _GO_122.fullmatch(stdout)
        return (match.group(1), None) if match is not None else ("", "VERSION_MISMATCH")
    match = _NODE_22.fullmatch(stdout)
    if match is None:
        return "", "VERSION_MISMATCH"
    return match.group(1), None


def _unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def _parse_child_summary(stdout: bytes) -> dict[str, Any]:
    if not stdout.endswith(b"\n") or stdout.count(b"\n") != 1:
        raise ValueError("SUMMARY_FRAMING_INVALID")
    body = stdout[:-1]
    body.decode("ascii", errors="strict")
    parsed = json.loads(
        body,
        object_pairs_hook=_unique_object,
        parse_constant=lambda _value: (_ for _ in ()).throw(ValueError("constant")),
        parse_float=lambda _value: (_ for _ in ()).throw(ValueError("float")),
    )
    if not isinstance(parsed, dict):
        raise ValueError("SUMMARY_SHAPE_INVALID")  # noqa: TRY004
    canonical = json.dumps(
        parsed,
        ensure_ascii=True,
        allow_nan=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("ascii")
    if canonical != body:
        raise ValueError("SUMMARY_NOT_CANONICAL")
    return parsed


def _validate_child_pass(
    summary: Mapping[str, Any],
    spec: RuntimeSpec,
    version: str,
) -> dict[str, Any]:
    if set(summary) != {
        "caseCount",
        "implementation",
        "logicalGoldenRawSha256",
        "resultSha256",
        "status",
        "suiteCount",
        "version",
    }:
        raise ValueError("SUMMARY_SHAPE_INVALID")
    if summary["status"] != "pass" or summary["implementation"] != spec.name:
        raise ValueError("IMPLEMENTATION_FAILED")
    if summary["version"] != version:
        raise ValueError("VERSION_MISMATCH")
    case_count = summary["caseCount"]
    suite_count = summary["suiteCount"]
    logical_golden_hash = summary["logicalGoldenRawSha256"]
    result_hash = summary["resultSha256"]
    if (
        isinstance(case_count, bool)
        or not isinstance(case_count, int)
        or case_count < 1
        or isinstance(suite_count, bool)
        or not isinstance(suite_count, int)
        or suite_count < 1
        or not isinstance(logical_golden_hash, str)
        or _SHA256.fullmatch(logical_golden_hash) is None
        or not isinstance(result_hash, str)
        or _SHA256.fullmatch(result_hash) is None
    ):
        raise ValueError("SUMMARY_SHAPE_INVALID")
    return {
        "caseCount": case_count,
        "logicalGoldenRawSha256": logical_golden_hash,
        "name": spec.name,
        "resultSha256": result_hash,
        "status": "pass",
        "suiteCount": suite_count,
        "version": version,
    }


def _validate_child_failure(
    summary: Mapping[str, Any],
    spec: RuntimeSpec,
    version: str,
) -> str:
    if set(summary) != {
        "error",
        "failedCase",
        "implementation",
        "status",
        "version",
    }:
        raise ValueError("SUMMARY_SHAPE_INVALID")
    if (
        summary["status"] != "fail"
        or summary["implementation"] != spec.name
        or summary["version"] != version
    ):
        raise ValueError("IMPLEMENTATION_FAILED")
    error = summary["error"]
    failed_case = summary["failedCase"]
    if (
        not isinstance(error, str)
        or not error
        or not error.isascii()
        or len(error) > _MAX_ERROR_CODE_LENGTH
        or not isinstance(failed_case, str)
        or not failed_case.isascii()
        or len(failed_case) > _MAX_FAILED_CASE_LENGTH
    ):
        raise ValueError("SUMMARY_SHAPE_INVALID")
    return error


def _failure(name: str, version: str, error: str) -> dict[str, Any]:
    return {
        "error": error if error.isascii() else "RUNTIME_FAILED",
        "name": name,
        "status": "fail",
        "version": version,
    }


def _run_runtime(  # noqa: PLR0911 - fail-closed process boundaries stay explicit.
    spec: RuntimeSpec,
) -> dict[str, Any]:
    if spec.executable is None:
        return {
            "error": "RUNTIME_NOT_FOUND",
            "name": spec.name,
            "status": "skipped",
            "version": "",
        }
    version, version_error = _preflight_version(spec)
    if version_error is not None:
        return {
            "error": version_error,
            "name": spec.name,
            "status": "skipped",
            "version": version,
        }
    try:
        completed = _run_process(spec.command)
    except subprocess.TimeoutExpired:
        return _failure(spec.name, version, "RUNTIME_TIMEOUT")
    except OSError:
        return _failure(spec.name, version, "RUNTIME_EXEC_FAILED")
    go_wrapper_failure = (
        spec.name == "go"
        and completed.returncode != 0
        and completed.stderr == _GO_RUN_FAILURE_STDERR
    )
    if completed.stderr != b"" and not go_wrapper_failure:
        return _failure(spec.name, version, "RUNTIME_STDERR_NOT_EMPTY")
    try:
        child = _parse_child_summary(completed.stdout)
        if child.get("status") == "fail":
            error = _validate_child_failure(child, spec, version)
            if completed.returncode == 0:
                return _failure(spec.name, version, "RUNTIME_EXIT_ZERO")
            return _failure(spec.name, version, error)
        passed = _validate_child_pass(child, spec, version)
    except (UnicodeError, ValueError) as error:
        code = str(error)
        if not code.isascii() or not code or len(code) > _MAX_ERROR_CODE_LENGTH:
            code = "RUNTIME_SUMMARY_INVALID"
        return _failure(spec.name, version, code)
    if completed.returncode != 0:
        return _failure(spec.name, version, "RUNTIME_EXIT_NONZERO")
    return passed


def verify(fixtures: Path, *, require_all: bool) -> dict[str, Any]:
    """Run all locally available implementations and compare their results."""
    resolved = fixtures.resolve(strict=True)
    if not resolved.is_dir():
        raise ValueError("FIXTURE_ROOT_INVALID")
    runtimes = [_run_runtime(spec) for spec in _runtime_specs(resolved)]
    passed = [item for item in runtimes if item["status"] == "pass"]
    failed = [item for item in runtimes if item["status"] == "fail"]
    skipped = [item for item in runtimes if item["status"] == "skipped"]

    result_hashes = {str(item["resultSha256"]) for item in passed}
    logical_golden_hashes = {str(item["logicalGoldenRawSha256"]) for item in passed}
    case_counts = {int(item["caseCount"]) for item in passed}
    suite_counts = {int(item["suiteCount"]) for item in passed}
    interop_mismatch = (
        len(result_hashes) > 1
        or len(logical_golden_hashes) > 1
        or len(case_counts) > 1
        or len(suite_counts) > 1
    )
    status = "pass"
    if not passed or failed or interop_mismatch or (require_all and skipped):
        status = "fail"
    result_hash = next(iter(result_hashes)) if len(result_hashes) == 1 else None
    logical_golden_hash = (
        next(iter(logical_golden_hashes)) if len(logical_golden_hashes) == 1 else None
    )
    return {
        "caseCount": next(iter(case_counts)) if len(case_counts) == 1 else 0,
        "logicalGoldenRawSha256": logical_golden_hash,
        "requireAll": require_all,
        "resultSha256": result_hash,
        "runtimes": runtimes,
        "schemaVersion": "mm-chat.jcs-gate-summary.v1",
        "status": status,
        "suiteCount": next(iter(suite_counts)) if len(suite_counts) == 1 else 0,
    }


def _write_summary(summary: Mapping[str, Any]) -> None:
    encoded = json.dumps(
        summary,
        ensure_ascii=True,
        allow_nan=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("ascii")
    sys.stdout.buffer.write(encoded + b"\n")


def build_parser() -> argparse.ArgumentParser:
    """Build the fixed offline-gate CLI parser."""
    parser = argparse.ArgumentParser(description="Run the offline JCS interop gate")
    parser.add_argument(
        "--fixtures",
        type=Path,
        default=_DEFAULT_FIXTURES,
        help="JCS fixture root (default: checked-in fixture directory)",
    )
    parser.add_argument(
        "--require-all",
        action="store_true",
        help="fail unless Python, Go 1.22, and Node 22 all pass",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the gate and emit exactly one ASCII JSON summary line."""
    args = build_parser().parse_args(argv)
    try:
        summary = verify(args.fixtures, require_all=args.require_all)
    except (OSError, ValueError):
        summary = {
            "caseCount": 0,
            "logicalGoldenRawSha256": None,
            "requireAll": bool(args.require_all),
            "resultSha256": None,
            "runtimes": [
                _failure(name, "", "FIXTURE_ROOT_INVALID")
                for name in ("python", "go", "node")
            ],
            "schemaVersion": "mm-chat.jcs-gate-summary.v1",
            "status": "fail",
            "suiteCount": 0,
        }
    _write_summary(summary)
    return 0 if summary["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
