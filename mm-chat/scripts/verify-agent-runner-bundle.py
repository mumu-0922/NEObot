#!/usr/bin/env python3
"""Verify one content-addressed exact-host Agent Runner deployment bundle."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import struct
import sys
from pathlib import Path
from typing import Any, NoReturn


MANIFEST_NAME = "neo-agent-runner-bundle.json"
MAX_MANIFEST_BYTES = 256 << 10
MAX_PAYLOAD_BYTES = 256 << 20
FINGERPRINT_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
GO_VERSION_RE = re.compile(r"^go1\.25(?:\.[0-9]+)?$")
IDENTITY_RE = re.compile(r"^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$")
EXPECTED_FILES = {
    "README.md": 0o644,
    "bin/neo-runner-probe": 0o755,
    "bin/neo-runnerd": 0o755,
    "config/release-manifest.json": 0o644,
    "config/seccomp-agent-v1.json": 0o644,
    "systemd/neo-runnerd.env.example": 0o644,
    "systemd/neo-runnerd.service": 0o644,
}
EXPECTED_DIRECTORIES = {"bin", "config", "systemd"}
ZERO_FINGERPRINT = "sha256:" + "0" * 64


class BundleError(RuntimeError):
    """A stable, content-free bundle verification failure."""


def fail(code: str) -> NoReturn:
    raise BundleError(code)


def duplicate_safe_json(raw: bytes, code: str) -> dict[str, Any]:
    def reject_duplicate(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                fail("DUPLICATE_JSON_KEY")
            result[key] = value
        return result

    try:
        value = json.loads(raw, object_pairs_hook=reject_duplicate)
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail(code)
    if not isinstance(value, dict):
        fail(code)
    return value


def read_regular(path: Path, maximum: int, code: str) -> tuple[bytes, os.stat_result]:
    descriptor = -1
    try:
        descriptor = os.open(
            path,
            os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0),
        )
        metadata = os.fstat(descriptor)
        if (
            not stat.S_ISREG(metadata.st_mode)
            or metadata.st_size < 1
            or metadata.st_size > maximum
        ):
            fail(code)
        chunks: list[bytes] = []
        remaining = maximum + 1
        while remaining > 0:
            chunk = os.read(descriptor, min(1 << 20, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        raw = b"".join(chunks)
    except OSError:
        fail(code)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    if len(raw) != metadata.st_size or len(raw) > maximum:
        fail(code)
    return raw, metadata


def require_keys(value: Any, expected: set[str], code: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != expected:
        fail(code)
    return value


def require_static_elf(raw: bytes, target_arch: str, code: str) -> None:
    if len(raw) < 64 or raw[:4] != b"\x7fELF" or raw[4] != 2 or raw[5] != 1:
        fail(code)
    header_format = "<HHIQQQIHHHHHH"
    header_size = struct.calcsize(header_format)
    if len(raw) < 16 + header_size:
        fail(code)
    header = struct.unpack_from(header_format, raw, 16)
    program_offset = header[4]
    program_entry_size = header[8]
    program_count = header[9]
    expected_machine = {"amd64": 62, "arm64": 183}[target_arch]
    if header[1] != expected_machine:
        fail(code)
    if program_count < 1 or program_count > 256 or program_entry_size < 4:
        fail(code)
    if program_offset + program_entry_size * program_count > len(raw):
        fail(code)
    for index in range(program_count):
        program_type = struct.unpack_from(
            "<I", raw, program_offset + index * program_entry_size
        )[0]
        if program_type == 3:  # PT_INTERP means a dynamic loader is required.
            fail(code)


def validate_release_file(value: Any) -> str:
    item = require_keys(value, {"path", "sha256"}, "RELEASE_MANIFEST_INVALID")
    path = item["path"]
    digest = item["sha256"]
    if (
        not isinstance(path, str)
        or not 1 <= len(path) <= 512
        or not path.startswith("/")
        or ".." in path
        or "\0" in path
        or not isinstance(digest, str)
        or FINGERPRINT_RE.fullmatch(digest) is None
    ):
        fail("RELEASE_MANIFEST_INVALID")
    return digest


def validate_release_identity(
    manifest: dict[str, Any], expected_approval: bool
) -> None:
    if manifest["schemaVersion"] != "neo.agent-runner-release/v1":
        fail("RELEASE_MANIFEST_INVALID")
    if manifest["approved"] is not expected_approval:
        fail("RELEASE_MANIFEST_INVALID")
    runner_id = manifest["runnerId"]
    if not isinstance(runner_id, str) or IDENTITY_RE.fullmatch(runner_id) is None:
        fail("RELEASE_MANIFEST_INVALID")
    version = manifest["runnerVersion"]
    if not isinstance(version, str) or not 1 <= len(version) <= 128:
        fail("RELEASE_MANIFEST_INVALID")


def validate_release_controllers(value: Any) -> None:
    if not isinstance(value, list) or len(value) != 3:
        fail("RELEASE_MANIFEST_INVALID")
    if any(not isinstance(item, str) for item in value):
        fail("RELEASE_MANIFEST_INVALID")
    if set(value) != {"cpu", "memory", "pids"}:
        fail("RELEASE_MANIFEST_INVALID")


def validate_release_runtime(manifest: dict[str, Any]) -> None:
    if manifest["protocolVersion"] != "neo.runner-rpc/v1":
        fail("RELEASE_MANIFEST_INVALID")
    if manifest["storageDriver"] != "overlay":
        fail("RELEASE_MANIFEST_INVALID")
    if manifest["networkMode"] != "none":
        fail("RELEASE_MANIFEST_INVALID")
    namespace_size = manifest["userNamespaceSize"]
    if isinstance(namespace_size, bool) or not isinstance(namespace_size, int):
        fail("RELEASE_MANIFEST_INVALID")
    if namespace_size < 65536:
        fail("RELEASE_MANIFEST_INVALID")
    validate_release_controllers(manifest["requiredControllers"])


def release_binary_fingerprints(value: Any) -> list[str]:
    if not isinstance(value, list) or len(value) != 5:
        fail("RELEASE_MANIFEST_INVALID")
    allowed = {"podman", "crun", "conmon", "newuidmap", "newgidmap"}
    names: set[str] = set()
    fingerprints: list[str] = []
    for binary in value:
        item = require_keys(
            binary,
            {"name", "path", "version", "sha256"},
            "RELEASE_MANIFEST_INVALID",
        )
        name = item["name"]
        version = item["version"]
        if name not in allowed or name in names:
            fail("RELEASE_MANIFEST_INVALID")
        if not isinstance(version, str) or not 1 <= len(version) <= 128:
            fail("RELEASE_MANIFEST_INVALID")
        names.add(name)
        fingerprints.append(
            validate_release_file({"path": item["path"], "sha256": item["sha256"]})
        )
    return fingerprints


def validate_release_manifest(raw: bytes, evidence_class: str) -> None:
    manifest = duplicate_safe_json(raw, "RELEASE_MANIFEST_INVALID")
    require_keys(
        manifest,
        {
            "schemaVersion",
            "approved",
            "runnerId",
            "runnerVersion",
            "protocolVersion",
            "binaries",
            "storageDriver",
            "networkMode",
            "userNamespaceSize",
            "requiredControllers",
            "seccompProfile",
            "probeSuiteFingerprint",
            "isolationAcceptance",
        },
        "RELEASE_MANIFEST_INVALID",
    )
    validate_release_identity(manifest, evidence_class == "production")
    validate_release_runtime(manifest)
    fingerprints = [manifest["probeSuiteFingerprint"]]
    for name in ("seccompProfile", "isolationAcceptance"):
        fingerprints.append(validate_release_file(manifest[name]))
    fingerprints.extend(release_binary_fingerprints(manifest["binaries"]))
    if any(
        not isinstance(value, str) or FINGERPRINT_RE.fullmatch(value) is None
        for value in fingerprints
    ):
        fail("RELEASE_MANIFEST_INVALID")
    if evidence_class == "production" and ZERO_FINGERPRINT in fingerprints:
        fail("PRODUCTION_RELEASE_PLACEHOLDER")


def inventory_fingerprint(manifest: dict[str, Any]) -> str:
    payload = {
        "schemaVersion": manifest["schemaVersion"],
        "evidenceClass": manifest["evidenceClass"],
        "release": manifest["release"],
        "files": manifest["files"],
    }
    canonical = json.dumps(
        payload, ensure_ascii=True, separators=(",", ":"), sort_keys=True
    ).encode("utf-8")
    return (
        "sha256:"
        + hashlib.sha256(b"neo-agent-runner-bundle-v1\0" + canonical).hexdigest()
    )


def bundle_directory_path(bundle: Path, root_path: Path, name: str) -> str:
    candidate = root_path / name
    if candidate.is_symlink():
        fail("BUNDLE_SYMLINK_FORBIDDEN")
    try:
        metadata = candidate.lstat()
    except OSError:
        fail("BUNDLE_ENTRY_UNREADABLE")
    if not stat.S_ISDIR(metadata.st_mode):
        fail("BUNDLE_ENTRY_INVALID")
    if stat.S_IMODE(metadata.st_mode) != 0o755:
        fail("BUNDLE_DIRECTORY_MODE_DRIFT")
    return candidate.relative_to(bundle).as_posix()


def bundle_file_path(bundle: Path, root_path: Path, name: str) -> str:
    candidate = root_path / name
    if candidate.is_symlink():
        fail("BUNDLE_SYMLINK_FORBIDDEN")
    return candidate.relative_to(bundle).as_posix()


def verify_bundle_tree(bundle: Path) -> None:
    try:
        root_metadata = bundle.lstat()
    except OSError:
        fail("BUNDLE_UNREADABLE")
    if not stat.S_ISDIR(root_metadata.st_mode) or bundle.is_symlink():
        fail("BUNDLE_ROOT_UNSAFE")
    if stat.S_IMODE(root_metadata.st_mode) != 0o700:
        fail("BUNDLE_ROOT_MODE_DRIFT")

    actual_directories: set[str] = set()
    actual_files: set[str] = set()
    for root, directories, files in os.walk(bundle, topdown=True, followlinks=False):
        root_path = Path(root)
        for name in directories:
            actual_directories.add(bundle_directory_path(bundle, root_path, name))
        for name in files:
            actual_files.add(bundle_file_path(bundle, root_path, name))
    if actual_directories != EXPECTED_DIRECTORIES:
        fail("BUNDLE_DIRECTORY_SET_INVALID")
    if actual_files != set(EXPECTED_FILES) | {MANIFEST_NAME}:
        fail("BUNDLE_FILE_SET_INVALID")


def load_bundle_manifest(
    bundle: Path, expected_class: str | None
) -> tuple[dict[str, Any], str, dict[str, Any]]:
    manifest_raw, manifest_metadata = read_regular(
        bundle / MANIFEST_NAME, MAX_MANIFEST_BYTES, "BUNDLE_MANIFEST_UNSAFE"
    )
    if stat.S_IMODE(manifest_metadata.st_mode) != 0o644:
        fail("BUNDLE_MANIFEST_MODE_DRIFT")
    manifest = duplicate_safe_json(manifest_raw, "BUNDLE_MANIFEST_INVALID")
    require_keys(
        manifest,
        {"schemaVersion", "evidenceClass", "release", "files", "inventorySha256"},
        "BUNDLE_MANIFEST_INVALID",
    )
    if manifest["schemaVersion"] != "neo.agent-runner-bundle/v1":
        fail("BUNDLE_VERSION_INVALID")
    evidence_class = manifest["evidenceClass"]
    if evidence_class not in {"template", "production"}:
        fail("BUNDLE_EVIDENCE_CLASS_INVALID")
    if expected_class is not None and evidence_class != expected_class:
        fail("BUNDLE_EVIDENCE_CLASS_MISMATCH")
    release = require_keys(
        manifest["release"],
        {"gitCommit", "migrationHead", "goVersion", "targetOS", "targetArch"},
        "BUNDLE_RELEASE_INVALID",
    )
    if (
        not isinstance(release["gitCommit"], str)
        or COMMIT_RE.fullmatch(release["gitCommit"]) is None
        or release["migrationHead"] != 91
        or not isinstance(release["goVersion"], str)
        or GO_VERSION_RE.fullmatch(release["goVersion"]) is None
        or release["targetOS"] != "linux"
        or release["targetArch"] not in {"amd64", "arm64"}
    ):
        fail("BUNDLE_RELEASE_INVALID")
    return manifest, evidence_class, release


def verify_payload_entry(bundle: Path, item: Any, seen: set[str]) -> tuple[str, bytes]:
    entry = require_keys(
        item, {"path", "mode", "size", "sha256"}, "BUNDLE_INVENTORY_INVALID"
    )
    path = entry["path"]
    if not isinstance(path, str) or path not in EXPECTED_FILES or path in seen:
        fail("BUNDLE_INVENTORY_INVALID")
    seen.add(path)
    expected_mode = EXPECTED_FILES[path]
    if entry["mode"] != f"{expected_mode:04o}":
        fail("BUNDLE_MODE_DRIFT")
    raw, metadata = read_regular(
        bundle / path, MAX_PAYLOAD_BYTES, "BUNDLE_PAYLOAD_UNSAFE"
    )
    if stat.S_IMODE(metadata.st_mode) != expected_mode:
        fail("BUNDLE_MODE_DRIFT")
    if entry["size"] != len(raw):
        fail("BUNDLE_SIZE_DRIFT")
    digest = "sha256:" + hashlib.sha256(raw).hexdigest()
    if entry["sha256"] != digest:
        fail("BUNDLE_HASH_DRIFT")
    return path, raw


def verify_payload_inventory(
    bundle: Path, manifest: dict[str, Any]
) -> tuple[dict[str, bytes], str, int]:
    files = manifest["files"]
    if not isinstance(files, list) or len(files) != len(EXPECTED_FILES):
        fail("BUNDLE_INVENTORY_INVALID")
    if files != sorted(
        files, key=lambda item: item.get("path", "") if isinstance(item, dict) else ""
    ):
        fail("BUNDLE_INVENTORY_ORDER_INVALID")
    seen: set[str] = set()
    payloads: dict[str, bytes] = {}
    for item in files:
        path, raw = verify_payload_entry(bundle, item, seen)
        payloads[path] = raw
    if seen != set(EXPECTED_FILES):
        fail("BUNDLE_INVENTORY_INVALID")
    expected_inventory = inventory_fingerprint(manifest)
    if manifest["inventorySha256"] != expected_inventory:
        fail("BUNDLE_INVENTORY_HASH_DRIFT")
    return payloads, expected_inventory, len(files)


def verify(bundle: Path, expected_class: str | None) -> dict[str, Any]:
    verify_bundle_tree(bundle)
    manifest, evidence_class, release = load_bundle_manifest(bundle, expected_class)
    payloads, expected_inventory, file_count = verify_payload_inventory(
        bundle, manifest
    )
    require_static_elf(
        payloads["bin/neo-runnerd"],
        release["targetArch"],
        "RUNNER_BINARY_NOT_STATIC_ELF",
    )
    require_static_elf(
        payloads["bin/neo-runner-probe"],
        release["targetArch"],
        "PROBE_BINARY_NOT_STATIC_ELF",
    )
    validate_release_manifest(payloads["config/release-manifest.json"], evidence_class)
    return {
        "schemaVersion": "neo.agent-runner-bundle-verdict/v1",
        "verdict": "BUNDLE_VERIFIED",
        "evidenceClass": evidence_class,
        "inventorySha256": expected_inventory,
        "fileCount": file_count,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--bundle", required=True, type=Path)
    parser.add_argument("--expected-class", choices=("template", "production"))
    arguments = parser.parse_args()
    try:
        result = verify(arguments.bundle, arguments.expected_class)
    except BundleError as error:
        result = {
            "schemaVersion": "neo.agent-runner-bundle-verdict/v1",
            "verdict": "BUNDLE_INVALID",
            "reasonCode": str(error),
        }
        print(json.dumps(result, sort_keys=True, separators=(",", ":")))
        return 2
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
