"""Frozen C1.2 protocol, sandbox, and output-root configuration."""

from __future__ import annotations

import hashlib
from dataclasses import asdict, dataclass
from typing import Final, cast

from mm_chat_rag.offline_parser.canonical import JsonValue, canonical_json_bytes
from mm_chat_rag.offline_parser.seccomp import seccomp_manifest


@dataclass(frozen=True, slots=True)
class SandboxLimits:
    """Per-child limits below the sidecar cgroup candidate ceiling."""

    address_space_bytes: int = 536_870_912
    cpu_seconds: int = 15
    processes: int = 64
    open_files: int = 64
    file_bytes: int = 67_108_864
    core_bytes: int = 0
    wall_timeout_millis: int = 30_000


@dataclass(frozen=True, slots=True)
class OutputLimits:
    """Owned harness-root quotas reserved below the fixed tmpfs ceiling."""

    parent_bytes: int = 536_870_912
    parent_inodes: int = 20_000
    max_active_runs: int = 1
    aggregate_bytes: int = 268_435_456
    files: int = 10_000
    artifact_bytes: int = 67_108_864
    reserved_bytes: int = 268_435_456
    reserved_inodes: int = 10_000


@dataclass(frozen=True, slots=True)
class NativeParserLimits:
    """Hash-bound C1.3 internal artifact and structure ceilings."""

    artifact_bytes: int = 67_108_864
    nodes: int = 100_000
    fragments: int = 1_000_000
    lines: int = 1_000_000
    nesting_depth: int = 128
    attributes: int = 1_000_000
    text_bytes: int = 33_000_000


@dataclass(frozen=True, slots=True)
class ParserHarnessConfig:
    """Hash-bound parser config; environment variables cannot override limits."""

    schema_version: str = "parser-harness-config.v1"
    protocol_major: int = 1
    max_header_bytes: int = 16_384
    max_request_bytes: int = 52_428_800
    max_result_bytes: int = 67_108_864
    max_concurrent_invocations: int = 1
    max_waiting_invocations: int = 1
    sidecar_uid: int = 10_002
    sidecar_gid: int = 10_001
    supervisor_install_stage: str = "container_start_before_socket_bind"
    child_install_stage: str = (
        "after_parent_child_process_group_handshake_before_source_bytes"
    )
    sandbox: SandboxLimits = SandboxLimits()
    output: OutputLimits = OutputLimits()
    native: NativeParserLimits = NativeParserLimits()

    def canonical_object(self) -> dict[str, JsonValue]:
        """Return the complete closed config including seccomp source hashes."""
        from mm_chat_rag.offline_parser.native.profile import (  # noqa: PLC0415
            native_parser_profile_manifest,
        )

        value = asdict(self)
        value["nativeParserProfile"] = native_parser_profile_manifest()
        value["seccomp"] = seccomp_manifest()
        return cast("dict[str, JsonValue]", value)

    @property
    def config_hash(self) -> str:
        """Return the SHA-256 of the exact canonical config bytes."""
        return hashlib.sha256(canonical_json_bytes(self.canonical_object())).hexdigest()


DEFAULT_CONFIG: Final = ParserHarnessConfig()
