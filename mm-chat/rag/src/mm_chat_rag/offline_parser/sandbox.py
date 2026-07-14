"""One-exec-child supervisor with process-group and residual-process gates."""

from __future__ import annotations

import contextlib
import ctypes
import json
import os
import selectors
import signal
import struct
import subprocess
import sys
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, ParserHarnessConfig
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.internal_result import (
    InternalResultError,
    NativeResultHeader,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeArtifactError,
    NativeDocument,
)
from mm_chat_rag.offline_parser.seccomp import child_filter_hash

_PR_SET_CHILD_SUBREAPER: Final = 36
_READY_LIMIT: Final = 128
_RESULT_HEADER_LIMIT: Final = 4096
_MIN_RESULT_BYTES: Final = 2
_BODY_LENGTH: Final = struct.Struct(">Q")
_MEMORY_EXIT_CODE: Final = 75
_TEST_PROBES: Final = frozenset(
    {"seccomp", "double_fork", "fork_bomb", "oom", "timeout"}
)


@dataclass(frozen=True, slots=True)
class SandboxRouteResult:
    """One isolated Native Parse outcome plus restart fencing state."""

    parser_format: ParserFormat | None
    stable_error_code: StableErrorCode | None
    native_artifact: bytes = b""
    requires_restart: bool = False

    @property
    def native_ready(self) -> bool:
        """Return whether a bounded child-internal artifact was verified."""
        return (
            self.parser_format is not None
            and self.stable_error_code is None
            and bool(self.native_artifact)
            and not self.requires_restart
        )


class SandboxSupervisor:
    """Spawn one clean interpreter per invocation and reap its full group."""

    def __init__(self, config: ParserHarnessConfig = DEFAULT_CONFIG) -> None:
        self._config = config
        self._subreaper_enabled = _enable_subreaper()

    @property
    def subreaper_enabled(self) -> bool:
        """Return whether PR_SET_CHILD_SUBREAPER was installed."""
        return self._subreaper_enabled

    def route(  # noqa: PLR0911, PLR0915
        self,
        source: bytes,
        *,
        declared_mime: str | None = None,
        declared_extension: str | None = None,
        cancelled: Callable[[], bool] | None = None,
        deadline_monotonic: float | None = None,
        _test_probe: str | None = None,
    ) -> SandboxRouteResult:
        """Route bytes only after process-group and seccomp handshakes pass."""
        if len(source) > self._config.max_request_bytes:
            return SandboxRouteResult(None, StableErrorCode.INPUT_TOO_LARGE)
        deadline = deadline_monotonic or (
            time.monotonic() + self._config.sandbox.wall_timeout_millis / 1000
        )
        environment = {
            "HOME": "/nonexistent",
            "LANG": "C.UTF-8",
            "LC_ALL": "C.UTF-8",
            "PYTHONHASHSEED": "1",
            "TZ": "UTC",
            "MM_CHAT_PARSER_SANDBOX_LIMITS": json.dumps(
                {
                    "addressSpaceBytes": self._config.sandbox.address_space_bytes,
                    "coreBytes": self._config.sandbox.core_bytes,
                    "cpuSeconds": self._config.sandbox.cpu_seconds,
                    "fileBytes": self._config.sandbox.file_bytes,
                    "openFiles": self._config.sandbox.open_files,
                    "processes": self._config.sandbox.processes,
                    "wallTimeoutMillis": self._config.sandbox.wall_timeout_millis,
                },
                ensure_ascii=True,
                allow_nan=False,
                separators=(",", ":"),
                sort_keys=True,
            ),
        }
        if _test_probe is not None:
            if _test_probe not in _TEST_PROBES:
                raise ValueError("unknown sandbox test probe")
            environment["MM_CHAT_PARSER_TEST_PROBE"] = _test_probe
        coverage_process_config = os.environ.get("COVERAGE_PROCESS_CONFIG")
        if coverage_process_config is not None:
            # coverage.py is dev-only and absent from the runtime image. Passing
            # its serialized config keeps the exec boundary measurable in CI
            # without inheriting arbitrary environment variables.
            environment["COVERAGE_PROCESS_CONFIG"] = coverage_process_config
            environment["COVERAGE_FILE"] = str(
                Path.cwd().joinpath(".coverage").resolve()
            )
        process = subprocess.Popen(  # noqa: S603
            [
                sys.executable,
                "-I",
                "-B",
                "-m",
                "mm_chat_rag.offline_parser.child",
            ],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            close_fds=True,
            cwd="/",
            env=environment,
            process_group=0,
        )
        pidfd = -1
        try:
            pidfd = _open_pidfd(process.pid)
            if process.stdin is None or process.stdout is None:
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            ready = _read_line_until(process.stdout.fileno(), deadline, _READY_LIMIT)
            expected_ready = f"READY {process.pid} {process.pid}\n".encode("ascii")
            if ready != expected_ready or os.getpgid(process.pid) != process.pid:
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            process.stdin.write(b"G")
            process.stdin.flush()
            installed = _read_exact_until(process.stdout.fileno(), 65, deadline)
            if installed != b"S" + child_filter_hash().encode("ascii"):
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)

            header = json.dumps(
                {
                    "declaredExtension": declared_extension,
                    "declaredMime": declared_mime,
                },
                ensure_ascii=True,
                allow_nan=False,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("ascii")
            process.stdin.write(struct.pack(">I", len(header)))
            process.stdin.write(header)
            process.stdin.write(struct.pack(">Q", len(source)))
            process.stdin.write(source)
            process.stdin.close()

            prefix = _read_with_cancel(
                process.stdout.fileno(),
                length=4,
                deadline=deadline,
                cancelled=cancelled,
            )
            if prefix is None:
                return self._abort(process, StableErrorCode.PARSER_CANCELLED)
            if prefix == b"":
                return self._classify_exit(process)
            result_length = struct.unpack(">I", prefix)[0]
            if (
                result_length < _MIN_RESULT_BYTES
                or result_length > _RESULT_HEADER_LIMIT
            ):
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            result_header_bytes = _read_with_cancel(
                process.stdout.fileno(),
                length=result_length,
                deadline=deadline,
                cancelled=cancelled,
            )
            if result_header_bytes is None:
                return self._abort(process, StableErrorCode.PARSER_CANCELLED)
            if len(result_header_bytes) != result_length:
                return self._classify_exit(process)
            result_header = NativeResultHeader.from_bytes(result_header_bytes)
            body_prefix = _read_with_cancel(
                process.stdout.fileno(),
                length=_BODY_LENGTH.size,
                deadline=deadline,
                cancelled=cancelled,
            )
            if body_prefix is None:
                return self._abort(process, StableErrorCode.PARSER_CANCELLED)
            if len(body_prefix) != _BODY_LENGTH.size:
                return self._classify_exit(process)
            body_length = _BODY_LENGTH.unpack(body_prefix)[0]
            if (
                body_length != result_header.result_bytes
                or body_length > self._config.native.artifact_bytes
                or body_length > self._config.max_result_bytes
            ):
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            result_body = _read_with_cancel(
                process.stdout.fileno(),
                length=body_length,
                deadline=deadline,
                cancelled=cancelled,
            )
            if result_body is None:
                return self._abort(process, StableErrorCode.PARSER_CANCELLED)
            if len(result_body) != body_length:
                return self._classify_exit(process)
            trailing = _read_with_cancel(
                process.stdout.fileno(),
                length=1,
                deadline=deadline,
                cancelled=cancelled,
            )
            if trailing is None:
                return self._abort(process, StableErrorCode.PARSER_CANCELLED)
            if trailing:
                return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            process.wait(timeout=max(0.1, deadline - time.monotonic()))
            if process.returncode != 0:
                return self._classify_exit(process)
            result = _decode_native_result(
                result_header,
                result_body,
                source=source,
                config=self._config,
            )
            had_residual = bool(_group_members(process.pid))
            uncleared_residual = (
                self._terminate_group(process.pid) if had_residual else False
            )
            if had_residual or uncleared_residual:
                return SandboxRouteResult(
                    result.parser_format,
                    result.stable_error_code,
                    native_artifact=b"",
                    requires_restart=True,
                )
            return result  # noqa: TRY300
        except (
            InternalResultError,
            OSError,
            ProcessLookupError,
            subprocess.TimeoutExpired,
            ValueError,
        ):
            code = (
                StableErrorCode.PARSER_TIMEOUT
                if time.monotonic() >= deadline
                else StableErrorCode.PARSER_SANDBOX_UNAVAILABLE
            )
            return self._abort(process, code)
        finally:
            if pidfd >= 0:
                os.close(pidfd)
            if process.poll() is None:
                self._terminate_group(process.pid)
                process.wait()
            _reap_group(process.pid)

    def run_test_probe(
        self,
        probe: str,
        *,
        cancelled: Callable[[], bool] | None = None,
        deadline_monotonic: float | None = None,
    ) -> SandboxRouteResult:
        """Exercise the real child fences without exposing probes through MMCP."""
        return self.route(
            b"",
            cancelled=cancelled,
            deadline_monotonic=deadline_monotonic,
            _test_probe=probe,
        )

    def _classify_exit(self, process: subprocess.Popen[bytes]) -> SandboxRouteResult:
        try:
            return_code = process.wait(timeout=1)
        except subprocess.TimeoutExpired:
            return self._abort(process, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
        if return_code == _MEMORY_EXIT_CODE or (
            return_code < 0 and -return_code in {signal.SIGXCPU, signal.SIGKILL}
        ):
            code = StableErrorCode.PARSER_MEMORY_LIMIT
        else:
            code = StableErrorCode.PARSER_SANDBOX_UNAVAILABLE
        had_descendants = bool(_group_members(process.pid) - {process.pid})
        residual = self._terminate_group(process.pid)
        return SandboxRouteResult(
            None,
            code,
            requires_restart=had_descendants or residual,
        )

    def _abort(
        self,
        process: subprocess.Popen[bytes],
        code: StableErrorCode,
    ) -> SandboxRouteResult:
        had_descendants = bool(_group_members(process.pid) - {process.pid})
        residual = self._terminate_group(process.pid)
        try:
            process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            residual = True
        _reap_group(process.pid)
        residual = _group_members(process.pid) != set() or residual
        return SandboxRouteResult(
            None,
            code,
            requires_restart=had_descendants or residual,
        )

    @staticmethod
    def _terminate_group(process_group: int) -> bool:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process_group, signal.SIGKILL)
        deadline = time.monotonic() + 2
        while time.monotonic() < deadline:
            _reap_group(process_group)
            if not _group_members(process_group):
                return False
            time.sleep(0.01)
        return bool(_group_members(process_group))


def _decode_native_result(
    header: NativeResultHeader,
    body: bytes,
    *,
    source: bytes,
    config: ParserHarnessConfig = DEFAULT_CONFIG,
) -> SandboxRouteResult:
    header.validate_body(
        body,
        body_limit=min(config.native.artifact_bytes, config.max_result_bytes),
    )
    if header.outcome == "native_success":
        if header.parser_format is None:
            raise InternalResultError("native success is missing its format")
        try:
            artifact = NativeDocument.from_bytes(body)
            artifact.validate_source_binding(
                source,
                expected_format=header.parser_format,
            )
        except NativeArtifactError:
            return SandboxRouteResult(
                parser_format=None,
                stable_error_code=StableErrorCode.PARSER_SCHEMA_MISMATCH,
            )
    return SandboxRouteResult(
        parser_format=header.parser_format,
        stable_error_code=header.stable_error_code,
        native_artifact=body,
    )


def _enable_subreaper() -> bool:
    if not sys.platform.startswith("linux"):
        return False
    libc = ctypes.CDLL(None, use_errno=True)
    return bool(libc.prctl(_PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) == 0)


def _open_pidfd(pid: int) -> int:
    if hasattr(os, "pidfd_open"):
        return os.pidfd_open(pid, 0)
    # CPython may be built against older libc headers even when the Linux
    # kernel supports pidfds. The syscall number is shared by x86_64/aarch64.
    libc = ctypes.CDLL(None, use_errno=True)
    file_descriptor = int(libc.syscall(434, pid, 0))
    if file_descriptor < 0:
        observed_errno = ctypes.get_errno()
        raise OSError(observed_errno, "pidfd_open is required by the parser sandbox")
    return file_descriptor


def _read_line_until(file_descriptor: int, deadline: float, limit: int) -> bytes:
    result = bytearray()
    while len(result) < limit:
        chunk = _read_exact_until(file_descriptor, 1, deadline)
        if not chunk:
            break
        result.extend(chunk)
        if chunk == b"\n":
            break
    return bytes(result)


def _read_exact_until(file_descriptor: int, length: int, deadline: float) -> bytes:
    result = bytearray()
    selector = selectors.DefaultSelector()
    selector.register(file_descriptor, selectors.EVENT_READ)
    try:
        while len(result) < length:
            timeout = deadline - time.monotonic()
            if timeout <= 0:
                raise subprocess.TimeoutExpired("parser-child", timeout=0)
            if not selector.select(timeout):
                raise subprocess.TimeoutExpired("parser-child", timeout=timeout)
            chunk = os.read(file_descriptor, length - len(result))
            if not chunk:
                break
            result.extend(chunk)
    finally:
        selector.close()
    return bytes(result)


def _read_with_cancel(
    file_descriptor: int,
    *,
    length: int,
    deadline: float,
    cancelled: Callable[[], bool] | None,
) -> bytes | None:
    result = bytearray()
    selector = selectors.DefaultSelector()
    selector.register(file_descriptor, selectors.EVENT_READ)
    try:
        while len(result) < length:
            if cancelled is not None and cancelled():
                return None
            timeout = deadline - time.monotonic()
            if timeout <= 0:
                raise subprocess.TimeoutExpired("parser-child", timeout=0)
            events = selector.select(min(timeout, 0.05))
            if not events:
                continue
            chunk = os.read(file_descriptor, length - len(result))
            if not chunk:
                break
            result.extend(chunk)
    finally:
        selector.close()
    return bytes(result)


def _group_members(process_group: int) -> set[int]:
    members: set[int] = set()
    try:
        process_entries = os.scandir("/proc")
    except OSError:
        return members
    with process_entries:
        for entry in process_entries:
            if not entry.name.isdecimal():
                continue
            pid = int(entry.name)
            try:
                if os.getpgid(pid) == process_group:
                    members.add(pid)
            except (ProcessLookupError, PermissionError):
                continue
    return members


def _reap_group(process_group: int) -> None:
    while True:
        try:
            pid, _status = os.waitpid(-process_group, os.WNOHANG)
        except ChildProcessError:
            return
        if pid == 0:
            return
