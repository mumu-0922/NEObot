"""Exec-isolated route child; setup completes before source bytes are read."""

from __future__ import annotations

import ctypes
import errno
import json
import os
import resource
import socket
import struct
import sys
import time
from typing import TYPE_CHECKING, Final

if TYPE_CHECKING:
    from mm_chat_rag.offline_parser.config import SandboxLimits

_MAX_INTERNAL_HEADER: Final = 4096
_MAX_SOURCE: Final = 52_428_800
_MAX_RESULT: Final = 4096
_MIN_INTERNAL_HEADER: Final = 2
_TEST_PROBE_ENV: Final = "MM_CHAT_PARSER_TEST_PROBE"
_LIMITS_ENV: Final = "MM_CHAT_PARSER_SANDBOX_LIMITS"


def main() -> int:  # noqa: PLR0911
    """Run one handshake-bound route invocation and exit."""
    from mm_chat_rag.offline_parser.config import (  # noqa: PLC0415
        DEFAULT_CONFIG,
        SandboxLimits,
    )

    _apply_limits(_limits_from_environment(SandboxLimits, DEFAULT_CONFIG.sandbox))
    os.setpgid(0, 0)
    if os.getpgrp() != os.getpid():
        return 70
    sys.stdout.buffer.write(f"READY {os.getpid()} {os.getpgrp()}\n".encode("ascii"))
    sys.stdout.buffer.flush()
    if _read_exact(sys.stdin.buffer, 1) != b"G":
        return 71

    from mm_chat_rag.offline_parser.seccomp import (  # noqa: PLC0415
        SeccompUnavailable,
        install_child_filter,
    )

    try:
        filter_hash = install_child_filter()
    except SeccompUnavailable:
        return 72
    sys.stdout.buffer.write(b"S" + filter_hash.encode("ascii"))
    sys.stdout.buffer.flush()

    try:
        header_length = struct.unpack(">I", _read_exact(sys.stdin.buffer, 4))[0]
        if header_length < _MIN_INTERNAL_HEADER or header_length > _MAX_INTERNAL_HEADER:
            return 73
        header = json.loads(_read_exact(sys.stdin.buffer, header_length))
        source_length = struct.unpack(">Q", _read_exact(sys.stdin.buffer, 8))[0]
        if source_length > _MAX_SOURCE:
            return 73
        source = _read_exact(sys.stdin.buffer, source_length)
        if sys.stdin.buffer.read(1):
            return 73
        result = _route(header, source)
        encoded = json.dumps(
            result,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
        if len(encoded) > _MAX_RESULT:
            return 73
        sys.stdout.buffer.write(struct.pack(">I", len(encoded)) + encoded)
        sys.stdout.buffer.flush()
        return 0  # noqa: TRY300
    except MemoryError:
        return 75
    except (EOFError, OSError, ValueError, TypeError, json.JSONDecodeError):
        return 73


def _route(header: object, source: bytes) -> dict[str, object]:
    if not isinstance(header, dict) or set(header) != {
        "declaredExtension",
        "declaredMime",
    }:
        raise ValueError("invalid internal header")
    mime = header["declaredMime"]
    extension = header["declaredExtension"]
    if mime is not None and not isinstance(mime, str):
        raise ValueError("invalid MIME")
    if extension is not None and not isinstance(extension, str):
        raise ValueError("invalid extension")
    probe = os.environ.get(_TEST_PROBE_ENV)
    if probe is not None:
        _run_test_probe(probe)
        return {"format": "txt", "stableErrorCode": None}
    from mm_chat_rag.offline_parser.router import route_source  # noqa: PLC0415

    decision = route_source(
        source,
        declared_mime=mime,
        declared_extension=extension,
    )
    return {
        "format": decision.parser_format.value if decision.parser_format else None,
        "stableErrorCode": (
            decision.stable_error_code.value if decision.stable_error_code else None
        ),
    }


def _run_test_probe(probe: str) -> None:
    if probe == "seccomp":
        _probe_seccomp()
        return
    if probe == "double_fork":
        first = os.fork()
        if first == 0:
            second = os.fork()
            if second == 0:
                time.sleep(10)
                os._exit(0)
            os._exit(0)
        os.waitpid(first, 0)
        return
    if probe == "fork_bomb":
        while True:
            try:
                child = os.fork()
            except OSError:
                return
            if child == 0:
                time.sleep(10)
                os._exit(0)
    if probe == "oom":
        _allocation = bytearray(600_000_000)
        raise RuntimeError(len(_allocation))
    if probe == "timeout":
        time.sleep(60)
        return
    raise ValueError("unknown sandbox test probe")


def _probe_seccomp() -> None:
    blocked_calls = (
        lambda: os.setpgid(0, 0),
        os.setsid,
        lambda: socket.socket(socket.AF_INET, socket.SOCK_STREAM),
    )
    for operation in blocked_calls:
        try:
            operation()
        except OSError as error:
            if error.errno != errno.EPERM:
                raise
        else:
            raise RuntimeError("seccomp operation unexpectedly succeeded")
    libc = ctypes.CDLL(None, use_errno=True)
    clone3_syscall = 435
    if libc.syscall(clone3_syscall, 0, 0) != -1 or ctypes.get_errno() != errno.ENOSYS:
        raise RuntimeError("clone3 did not fail with ENOSYS")


def _apply_limits(limits: SandboxLimits) -> None:
    values = {
        resource.RLIMIT_AS: limits.address_space_bytes,
        resource.RLIMIT_CPU: limits.cpu_seconds,
        resource.RLIMIT_NPROC: limits.processes,
        resource.RLIMIT_NOFILE: limits.open_files,
        resource.RLIMIT_FSIZE: limits.file_bytes,
        resource.RLIMIT_CORE: limits.core_bytes,
    }
    for limit_kind, value in values.items():
        resource.setrlimit(limit_kind, (value, value))


def _limits_from_environment(
    limits_type: type[SandboxLimits],
    default: SandboxLimits,
) -> SandboxLimits:
    raw = os.environ.get(_LIMITS_ENV)
    if raw is None:
        return default
    value = json.loads(raw)
    expected = {
        "addressSpaceBytes",
        "coreBytes",
        "cpuSeconds",
        "fileBytes",
        "openFiles",
        "processes",
        "wallTimeoutMillis",
    }
    if not isinstance(value, dict) or set(value) != expected:
        raise ValueError("sandbox limits are not closed")
    if any(type(item) is not int or item < 0 for item in value.values()):
        raise ValueError("sandbox limits must be non-negative integers")
    if value["coreBytes"] != 0:
        raise ValueError("core dump limit must remain zero")
    return limits_type(
        address_space_bytes=value["addressSpaceBytes"],
        cpu_seconds=value["cpuSeconds"],
        processes=value["processes"],
        open_files=value["openFiles"],
        file_bytes=value["fileBytes"],
        core_bytes=value["coreBytes"],
        wall_timeout_millis=value["wallTimeoutMillis"],
    )


def _read_exact(stream: object, length: int) -> bytes:
    chunks: list[bytes] = []
    remaining = length
    while remaining:
        chunk = stream.read(remaining)  # type: ignore[attr-defined]
        if not chunk:
            raise EOFError
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


if __name__ == "__main__":
    raise SystemExit(main())
