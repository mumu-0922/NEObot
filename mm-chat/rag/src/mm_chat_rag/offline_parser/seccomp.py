"""Deterministic Linux seccomp policies for supervisor and isolated child."""

from __future__ import annotations

import ctypes
import errno
import hashlib
import platform
import struct
from dataclasses import dataclass
from importlib.resources import files
from typing import Final

from mm_chat_rag.offline_parser.canonical import JsonValue, canonical_json_bytes

_PR_SET_NO_NEW_PRIVS: Final = 38
_SECCOMP_SET_MODE_FILTER: Final = 1
_SECCOMP_RET_KILL_PROCESS: Final = 0x80000000
_SECCOMP_RET_ERRNO: Final = 0x00050000
_SECCOMP_RET_ALLOW: Final = 0x7FFF0000
_BPF_LD: Final = 0x00
_BPF_W: Final = 0x00
_BPF_ABS: Final = 0x20
_BPF_JMP: Final = 0x05
_BPF_JEQ: Final = 0x10
_BPF_JSET: Final = 0x40
_BPF_K: Final = 0x00
_BPF_RET: Final = 0x06
_NAMESPACE_CLONE_MASK: Final = 0x7E020080


@dataclass(frozen=True, slots=True)
class _Architecture:
    audit_arch: int
    seccomp_syscall: int
    syscalls: dict[str, int]


_ARCHITECTURES: Final = {
    "x86_64": _Architecture(
        audit_arch=0xC000003E,
        seccomp_syscall=317,
        syscalls={
            "clone": 56,
            "clone3": 435,
            "setsid": 112,
            "setpgid": 109,
            "unshare": 272,
            "setns": 308,
            "ptrace": 101,
            "socket": 41,
            "connect": 42,
            "bind": 49,
            "listen": 50,
            "accept": 43,
            "accept4": 288,
        },
    ),
    "aarch64": _Architecture(
        audit_arch=0xC00000B7,
        seccomp_syscall=277,
        syscalls={
            "clone": 220,
            "clone3": 435,
            "setsid": 157,
            "setpgid": 154,
            "unshare": 97,
            "setns": 268,
            "ptrace": 117,
            "socket": 198,
            "connect": 203,
            "bind": 200,
            "listen": 201,
            "accept": 202,
            "accept4": 242,
        },
    ),
}

CONTAINER_POLICY: Final[dict[str, JsonValue]] = {
    "schemaVersion": "parser-container-seccomp.v1",
    "defaultAction": "SCMP_ACT_ALLOW",
    "denyErrno": "EPERM",
    "allowSupervisorSetpgid": True,
    "denySyscalls": [
        "bpf",
        "kexec_load",
        "mount",
        "open_by_handle_at",
        "perf_event_open",
        "ptrace",
        "setns",
        "setsid",
        "umount2",
        "unshare",
    ],
    "denySocketDomains": [2, 10],
    "denyCloneNamespaceMask": _NAMESPACE_CLONE_MASK,
}

CHILD_POLICY: Final[dict[str, JsonValue]] = {
    "schemaVersion": "parser-child-seccomp.v1",
    "defaultAction": "SCMP_ACT_ALLOW",
    "clone3Action": "SCMP_ACT_ERRNO_ENOSYS",
    "cloneNamespaceMask": _NAMESPACE_CLONE_MASK,
    "denyErrno": "EPERM",
    "denySyscalls": [
        "accept",
        "accept4",
        "bind",
        "connect",
        "listen",
        "ptrace",
        "setns",
        "setpgid",
        "setsid",
        "socket",
        "unshare",
    ],
}


class SeccompUnavailable(RuntimeError):  # noqa: N818
    """The current kernel/architecture cannot install the frozen policy."""


class _SockFilter(ctypes.Structure):
    _fields_ = [  # noqa: RUF012
        ("code", ctypes.c_ushort),
        ("jt", ctypes.c_ubyte),
        ("jf", ctypes.c_ubyte),
        ("k", ctypes.c_uint32),
    ]


class _SockFprog(ctypes.Structure):
    _fields_ = [  # noqa: RUF012
        ("len", ctypes.c_ushort),
        ("filter", ctypes.POINTER(_SockFilter)),
    ]


def seccomp_manifest() -> dict[str, JsonValue]:
    """Return config-bound source/compiled hashes and install stages."""
    machine = platform.machine().casefold()
    normalized = "aarch64" if machine in {"arm64", "aarch64"} else machine
    compiled_hash = None
    if normalized in _ARCHITECTURES:
        compiled_hash = child_filter_hash(normalized)
    return {
        "childCompiledFilterSha256": compiled_hash,
        "childInstallStage": "after_process_group_handshake_before_source",
        "childPolicySha256": _policy_hash(CHILD_POLICY),
        "containerInstallStage": "container_start_before_supervisor",
        "containerPolicySha256": hashlib.sha256(container_profile_bytes()).hexdigest(),
        "kernelArchitecture": normalized,
    }


def container_profile_bytes() -> bytes:
    """Read the exact Docker seccomp source referenced by Compose."""
    return (
        files("mm_chat_rag.offline_parser.profiles")
        .joinpath("parser-sidecar.json")
        .read_bytes()
    )


def child_filter_hash(machine: str | None = None) -> str:
    """Hash the exact classic-BPF instruction bytes for an architecture."""
    instructions = _child_filter(machine)
    encoded = b"".join(
        struct.pack(
            "<HBBI", instruction.code, instruction.jt, instruction.jf, instruction.k
        )
        for instruction in instructions
    )
    return hashlib.sha256(encoded).hexdigest()


def install_child_filter() -> str:
    """Install no-new-privileges plus the frozen per-child seccomp filter."""
    machine = platform.machine().casefold()
    normalized = "aarch64" if machine in {"arm64", "aarch64"} else machine
    architecture = _ARCHITECTURES.get(normalized)
    if architecture is None:
        raise SeccompUnavailable("unsupported seccomp architecture")
    instructions = _child_filter(normalized)
    array_type = _SockFilter * len(instructions)
    array = array_type(*instructions)
    program = _SockFprog(len=len(instructions), filter=array)
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(_PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0:
        observed = ctypes.get_errno()
        raise SeccompUnavailable(f"PR_SET_NO_NEW_PRIVS failed with errno {observed}")
    result = libc.syscall(
        architecture.seccomp_syscall,
        _SECCOMP_SET_MODE_FILTER,
        0,
        ctypes.byref(program),
    )
    if result != 0:
        observed = ctypes.get_errno()
        raise SeccompUnavailable(f"seccomp filter install failed with errno {observed}")
    return child_filter_hash(normalized)


def _child_filter(machine: str | None = None) -> list[_SockFilter]:
    observed = (machine or platform.machine()).casefold()
    normalized = "aarch64" if observed in {"arm64", "aarch64"} else observed
    architecture = _ARCHITECTURES.get(normalized)
    if architecture is None:
        raise SeccompUnavailable("unsupported seccomp architecture")
    load = _BPF_LD | _BPF_W | _BPF_ABS
    jump_equal = _BPF_JMP | _BPF_JEQ | _BPF_K
    jump_set = _BPF_JMP | _BPF_JSET | _BPF_K
    ret = _BPF_RET | _BPF_K
    deny = _SECCOMP_RET_ERRNO | errno.EPERM
    enosys = _SECCOMP_RET_ERRNO | errno.ENOSYS
    instructions = [
        _SockFilter(load, 0, 0, 4),
        _SockFilter(jump_equal, 1, 0, architecture.audit_arch),
        _SockFilter(ret, 0, 0, _SECCOMP_RET_KILL_PROCESS),
        _SockFilter(load, 0, 0, 0),
    ]
    for syscall_name in (
        "setsid",
        "setpgid",
        "unshare",
        "setns",
        "ptrace",
        "socket",
        "connect",
        "bind",
        "listen",
        "accept",
        "accept4",
    ):
        instructions.extend(
            (
                _SockFilter(jump_equal, 0, 1, architecture.syscalls[syscall_name]),
                _SockFilter(ret, 0, 0, deny),
            )
        )
    instructions.extend(
        (
            _SockFilter(jump_equal, 0, 1, architecture.syscalls["clone3"]),
            _SockFilter(ret, 0, 0, enosys),
            _SockFilter(jump_equal, 0, 3, architecture.syscalls["clone"]),
            _SockFilter(load, 0, 0, 16),
            _SockFilter(jump_set, 0, 1, _NAMESPACE_CLONE_MASK),
            _SockFilter(ret, 0, 0, deny),
            _SockFilter(ret, 0, 0, _SECCOMP_RET_ALLOW),
        )
    )
    return instructions


def _policy_hash(policy: dict[str, JsonValue]) -> str:
    return hashlib.sha256(canonical_json_bytes(policy)).hexdigest()
