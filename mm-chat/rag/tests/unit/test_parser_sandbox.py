"""Real C1.2 process-group, seccomp, resource, and residual-process gates."""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import time
from io import BytesIO
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser import child, sandbox
from mm_chat_rag.offline_parser.config import ParserHarnessConfig, SandboxLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.internal_result import (
    InternalResultError,
    NativeResultHeader,
)
from mm_chat_rag.offline_parser.sandbox import SandboxSupervisor
from mm_chat_rag.offline_parser.seccomp import (
    CHILD_POLICY,
    child_filter_hash,
    container_profile_bytes,
    seccomp_manifest,
)


def _sandbox_with_fork_headroom() -> SandboxSupervisor:
    current_uid_tasks = sum(
        _process_task_count(int(entry.name))
        for entry in os.scandir("/proc")
        if entry.name.isdecimal() and _process_owner(int(entry.name)) == os.geteuid()
    )
    # RLIMIT_NPROC is charged against the host UID, while /proc may expose only
    # this PID namespace. Keep a bounded margin for invisible host-UID tasks.
    limits = SandboxLimits(processes=max(current_uid_tasks + 128, 256))
    return SandboxSupervisor(ParserHarnessConfig(sandbox=limits))


def _process_owner(pid: int) -> int | None:
    try:
        return Path(f"/proc/{pid}").stat().st_uid
    except (FileNotFoundError, PermissionError):
        return None


def _process_task_count(pid: int) -> int:
    try:
        with os.scandir(f"/proc/{pid}/task") as entries:
            return sum(1 for entry in entries if entry.name.isdecimal())
    except (FileNotFoundError, PermissionError):
        return 0


def test_seccomp_source_compiled_hash_and_install_stages_are_config_bound() -> None:
    profile = json.loads(container_profile_bytes())
    manifest = seccomp_manifest()

    assert profile["defaultAction"] == "SCMP_ACT_ALLOW"
    assert (
        hashlib.sha256(container_profile_bytes()).hexdigest()
        == manifest["containerPolicySha256"]
    )
    assert child_filter_hash() == manifest["childCompiledFilterSha256"]
    assert manifest["childInstallStage"].endswith("before_source")
    assert CHILD_POLICY["clone3Action"] == "SCMP_ACT_ERRNO_ENOSYS"
    assert CHILD_POLICY["cloneNamespaceMask"] == 0x7E020080


def test_supervisor_routes_only_after_real_seccomp_handshake() -> None:
    supervisor = SandboxSupervisor()

    decision = supervisor.route(b"hello", declared_extension=".txt")
    seccomp_probe = supervisor.run_test_probe("seccomp")

    assert supervisor.subreaper_enabled
    assert decision.parser_format is ParserFormat.TXT
    assert not decision.requires_restart
    assert seccomp_probe.parser_format is ParserFormat.TXT
    assert not seccomp_probe.requires_restart


def test_double_fork_and_bounded_fork_bomb_trip_restart_gate() -> None:
    supervisor = _sandbox_with_fork_headroom()

    double_fork = supervisor.run_test_probe("double_fork")
    fork_bomb = supervisor.run_test_probe(
        "fork_bomb",
        deadline_monotonic=time.monotonic() + 5,
    )

    assert double_fork.requires_restart
    assert fork_bomb.requires_restart


def test_oom_timeout_and_cancel_map_to_closed_non_stageable_errors() -> None:
    supervisor = SandboxSupervisor()

    memory = supervisor.run_test_probe("oom")
    timeout = supervisor.run_test_probe(
        "timeout",
        deadline_monotonic=time.monotonic() + 0.2,
    )
    started = time.monotonic()
    cancelled = supervisor.run_test_probe(
        "timeout",
        deadline_monotonic=time.monotonic() + 5,
        cancelled=lambda: time.monotonic() - started > 0.1,
    )

    assert memory.stable_error_code is StableErrorCode.PARSER_MEMORY_LIMIT
    assert timeout.stable_error_code is StableErrorCode.PARSER_TIMEOUT
    assert cancelled.stable_error_code is StableErrorCode.PARSER_CANCELLED
    assert not memory.requires_restart
    assert not timeout.requires_restart
    assert not cancelled.requires_restart


@pytest.mark.parametrize(
    "header",
    [
        None,
        {},
        {"declaredExtension": None, "declaredMime": None, "extra": None},
        {"declaredExtension": None, "declaredMime": 1},
        {"declaredExtension": 1, "declaredMime": None},
    ],
)
def test_child_internal_route_header_is_closed(header: object) -> None:
    with pytest.raises(ValueError, match="invalid"):
        child._route(header, b"x")


def test_child_internal_route_returns_format_and_stable_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("MM_CHAT_PARSER_TEST_PROBE", raising=False)
    accepted, accepted_body = child._route(
        {"declaredExtension": ".txt", "declaredMime": None},
        b"x",
    )
    rejected, rejected_body = child._route(
        {"declaredExtension": None, "declaredMime": None},
        b"x",
    )

    assert accepted.outcome == "native_success"
    assert accepted.parser_format is ParserFormat.TXT
    accepted.validate_body(accepted_body, body_limit=len(accepted_body))
    assert rejected.stable_error_code is StableErrorCode.FORMAT_AMBIGUOUS
    assert rejected_body == b""


@pytest.mark.parametrize(
    "raw",
    [
        "{}",
        '{"addressSpaceBytes":-1,"coreBytes":0,"cpuSeconds":1,"fileBytes":1,'
        '"openFiles":1,"processes":1,"wallTimeoutMillis":1}',
        '{"addressSpaceBytes":1,"coreBytes":1,"cpuSeconds":1,"fileBytes":1,'
        '"openFiles":1,"processes":1,"wallTimeoutMillis":1}',
    ],
)
def test_child_resource_limit_environment_is_closed(
    raw: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    default = SandboxLimits()
    monkeypatch.setenv("MM_CHAT_PARSER_SANDBOX_LIMITS", raw)
    with pytest.raises(ValueError, match="limits|non-negative|core"):
        child._limits_from_environment(SandboxLimits, default)


def test_child_resource_limit_environment_default_and_valid_paths(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    default = SandboxLimits()
    monkeypatch.delenv("MM_CHAT_PARSER_SANDBOX_LIMITS", raising=False)
    assert child._limits_from_environment(SandboxLimits, default) is default
    raw = json.dumps(
        {
            "addressSpaceBytes": 2,
            "coreBytes": 0,
            "cpuSeconds": 3,
            "fileBytes": 4,
            "openFiles": 5,
            "processes": 6,
            "wallTimeoutMillis": 7,
        }
    )
    monkeypatch.setenv("MM_CHAT_PARSER_SANDBOX_LIMITS", raw)
    observed = child._limits_from_environment(SandboxLimits, default)
    assert observed.address_space_bytes == 2
    assert observed.wall_timeout_millis == 7


def test_child_read_exact_and_unknown_probe_fail_closed() -> None:
    assert child._read_exact(BytesIO(b"abc"), 3) == b"abc"
    with pytest.raises(EOFError):
        child._read_exact(BytesIO(b"a"), 2)
    with pytest.raises(ValueError, match="unknown"):
        child._run_test_probe("unknown")


def test_child_limit_installer_sets_every_frozen_rlimit(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    observed: list[tuple[int, tuple[int, int]]] = []
    monkeypatch.setattr(
        child.resource,
        "setrlimit",
        lambda kind, value: observed.append((kind, value)),
    )
    limits = SandboxLimits()

    child._apply_limits(limits)

    assert len(observed) == 6
    assert all(soft == hard for _kind, (soft, hard) in observed)


def test_supervisor_rejects_unknown_probe() -> None:
    with pytest.raises(ValueError, match="unknown"):
        SandboxSupervisor().run_test_probe("unknown")


def test_supervisor_rejects_source_over_configured_request_limit() -> None:
    supervisor = SandboxSupervisor(ParserHarnessConfig(max_request_bytes=1))
    result = supervisor.route(b"xx")
    assert result.stable_error_code is StableErrorCode.INPUT_TOO_LARGE


@pytest.mark.parametrize(
    "content",
    [
        b"[]",
        b"{}",
        b'{"format":null,"stableErrorCode":null}',
        b'{"format":"unknown","stableErrorCode":null}',
    ],
)
def test_supervisor_rejects_invalid_child_result_shapes(content: bytes) -> None:
    with pytest.raises(InternalResultError):
        NativeResultHeader.from_bytes(content)


def test_supervisor_decodes_closed_child_error_result() -> None:
    observed = sandbox._decode_native_result(
        NativeResultHeader.failure(StableErrorCode.INPUT_INVALID),
        b"",
        source=b"",
    )
    assert observed.stable_error_code is StableErrorCode.INPUT_INVALID


def test_supervisor_read_helpers_cover_eof_timeout_cancel_and_line_paths() -> None:
    read_fd, write_fd = os.pipe()
    try:
        os.write(write_fd, b"READY\n")
        os.close(write_fd)
        assert sandbox._read_line_until(read_fd, time.monotonic() + 1, 32) == b"READY\n"
    finally:
        os.close(read_fd)

    read_fd, write_fd = os.pipe()
    try:
        os.close(write_fd)
        assert sandbox._read_exact_until(read_fd, 1, time.monotonic() + 1) == b""
    finally:
        os.close(read_fd)

    read_fd, write_fd = os.pipe()
    try:
        with pytest.raises(subprocess.TimeoutExpired):
            sandbox._read_exact_until(read_fd, 1, time.monotonic() - 1)
        assert (
            sandbox._read_with_cancel(
                read_fd,
                length=1,
                deadline=time.monotonic() + 1,
                cancelled=lambda: True,
            )
            is None
        )
    finally:
        os.close(read_fd)
        os.close(write_fd)


def test_supervisor_proc_and_subreaper_compatibility_helpers(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    assert os.getpid() in sandbox._group_members(os.getpgrp())
    sandbox._reap_group(999_999)
    monkeypatch.setattr(sandbox.sys, "platform", "not-linux")
    assert not sandbox._enable_subreaper()
