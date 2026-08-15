#!/usr/bin/env python3
"""Fail-closed helpers for the WSL2 Agent Runner local-test profile."""

from __future__ import annotations

import configparser
import hashlib
import json
import os
import platform
import pwd
import re
import shlex
import shutil
import stat
import subprocess
import tarfile
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, NoReturn, Sequence

LOCK_SCHEMA = "neo.agent-runner-local-test-toolchain/v1"
RECEIPT_SCHEMA = "neo.agent-runner-local-test-receipt/v1"
BOOTSTRAP_SCHEMA = "neo.agent-runner-local-test-bootstrap/v1"
REPORT_SCHEMA = "neo.agent-runner-local-test-report/v1"
EVIDENCE_CLASS = "local_test"
DEFAULT_STATE_ROOT = Path("/var/lib/neo-chat-agent-runner-local-test")
FINGERPRINT_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
PACKAGE_RE = re.compile(r"^[a-z0-9][a-z0-9+.-]{0,63}$")
VERSION_RE = re.compile(r"^[0-9][0-9A-Za-z.+-]{0,31}$")
SECTION_RE = re.compile(r"^\s*\[([^]\r\n]+)]\s*(?:[#;].*)?$")
SYSTEMD_RE = re.compile(r"^(\s*)systemd\s*=.*$", re.IGNORECASE)
EXPECTED_HOST = {
    "architecture": "amd64",
    "distribution": "ubuntu",
    "distributionVersion": "22.04",
    "environment": "wsl2",
}
EXPECTED_ARTIFACTS = {"go", "podman-source", "crun", "conmon"}
EXPECTED_APT_PACKAGES = (
    "btrfs-progs",
    "build-essential",
    "ca-certificates",
    "curl",
    "fuse-overlayfs",
    "git",
    "iptables",
    "libassuan-dev",
    "libbtrfs-dev",
    "libc6-dev",
    "libdevmapper-dev",
    "libglib2.0-dev",
    "libgpg-error-dev",
    "libgpgme-dev",
    "libprotobuf-c-dev",
    "libprotobuf-dev",
    "libseccomp-dev",
    "libselinux1-dev",
    "libsystemd-dev",
    "make",
    "pkg-config",
    "slirp4netns",
    "uidmap",
)
EXPECTED_ARTIFACT_LOCKS = {
    "go": (
        "1.25.9",
        "https://go.dev/dl/go1.25.9.linux-amd64.tar.gz",
        59_795_542,
        "sha256:00859d7bd6defe8bf84d9db9e57b9a4467b2887c18cd93ae7460e713db774bc1",
    ),
    "podman-source": (
        "6.1.0",
        "https://github.com/podman-container-tools/podman/archive/refs/tags/v6.1.0.tar.gz",
        20_956_524,
        "sha256:e086183db2f852476a7fa2580d0276cef32086b4cf17ae7020948f06eb613e0d",
    ),
    "crun": (
        "1.29.1",
        "https://github.com/containers/crun/releases/download/1.29.1/crun-1.29.1-linux-amd64",
        3_580_152,
        "sha256:0a5ea25cafe618bbfbf1c747871155063619f18025ccdd8ad648c97633f35d57",
    ),
    "conmon": (
        "2.2.1",
        "https://github.com/containers/conmon/releases/download/v2.2.1/conmon.amd64",
        1_986_352,
        "sha256:1d97294c14c43d477e0a0826e9cd0f2a2af373ddfafe6f10252e8a3c43f32be6",
    ),
}
ALLOWED_DOWNLOAD_HOSTS = {"go.dev", "github.com"}
ALLOWED_DOWNLOAD_REDIRECT_HOSTS = {
    "codeload.github.com",
    "dl.google.com",
    "github.com",
    "go.dev",
    "release-assets.githubusercontent.com",
}
PROTECTED_PROJECT_NAMES = {"data", "secrets", "backup", ".env.single-server"}
BOOTSTRAP_STATE_KEYS = {
    "schemaVersion",
    "evidenceClass",
    "productionEligible",
    "status",
    "operatorUser",
    "createdAt",
    "wslConfExisted",
    "wslConfBeforeSha256",
    "wslConfAfterSha256",
    "packagesRequested",
    "packagesInitiallyMissing",
}
LOCAL_WORKLOAD_CHECKS = [
    "nonzero_identity",
    "empty_capabilities",
    "no_new_privileges",
    "network_none",
    "readonly_rootfs",
    "readonly_workspace",
    "bounded_scratch",
    "no_secrets",
]


class LocalTestError(RuntimeError):
    """A stable local-test failure that contains no sensitive values."""


class PinnedDownloadRedirectHandler(urllib.request.HTTPRedirectHandler):
    """Allow only the reviewed HTTPS redirect hosts for pinned artifacts."""

    def redirect_request(
        self,
        request: urllib.request.Request,
        file_pointer: Any,
        code: int,
        message: str,
        headers: Any,
        new_url: str,
    ) -> urllib.request.Request | None:
        if not download_url_allowed(new_url, ALLOWED_DOWNLOAD_REDIRECT_HOSTS):
            fail("DOWNLOAD_REDIRECT_REJECTED")
        return super().redirect_request(
            request, file_pointer, code, message, headers, new_url
        )


def fail(code: str) -> NoReturn:
    raise LocalTestError(code)


def download_url_allowed(raw: str, hosts: set[str]) -> bool:
    try:
        target = urllib.parse.urlsplit(raw)
        port = target.port
    except (TypeError, ValueError):
        return False
    return (
        target.scheme == "https"
        and target.hostname in hosts
        and port in {None, 443}
        and target.username is None
        and target.password is None
        and not target.fragment
    )


def duplicate_safe_json(raw: bytes, code: str) -> dict[str, Any]:
    def reject_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        for key, item in pairs:
            if key in value:
                fail("DUPLICATE_JSON_KEY")
            value[key] = item
        return value

    try:
        parsed = json.loads(raw, object_pairs_hook=reject_duplicates)
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail(code)
    if not isinstance(parsed, dict):
        fail(code)
    return parsed


def require_keys(value: Any, expected: set[str], code: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != expected:
        fail(code)
    return value


def read_regular(path: Path, maximum: int, code: str) -> bytes:
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
    return raw


def sha256_bytes(raw: bytes) -> str:
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def sha256_file(path: Path, maximum: int = 512 << 20) -> str:
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
            fail("FILE_INVALID")
        digest = hashlib.sha256()
        while True:
            chunk = os.read(descriptor, 1 << 20)
            if not chunk:
                break
            digest.update(chunk)
    except OSError:
        fail("FILE_INVALID")
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    return "sha256:" + digest.hexdigest()


def canonical_json(value: Any) -> bytes:
    return json.dumps(
        value, ensure_ascii=True, separators=(",", ":"), sort_keys=True
    ).encode("utf-8")


@dataclass(frozen=True)
class ArtifactLock:
    name: str
    version: str
    url: str
    size: int
    sha256: str


@dataclass(frozen=True)
class ToolchainLock:
    path: Path
    fingerprint: str
    apt_packages: tuple[str, ...]
    artifacts: tuple[ArtifactLock, ...]

    def artifact(self, name: str) -> ArtifactLock:
        for artifact in self.artifacts:
            if artifact.name == name:
                return artifact
        fail("TOOLCHAIN_LOCK_INVALID")


def load_toolchain_lock(path: Path) -> ToolchainLock:
    raw = read_regular(path, 64 << 10, "TOOLCHAIN_LOCK_INVALID")
    value = duplicate_safe_json(raw, "TOOLCHAIN_LOCK_INVALID")
    require_keys(
        value,
        {"schemaVersion", "evidenceClass", "host", "aptPackages", "artifacts"},
        "TOOLCHAIN_LOCK_INVALID",
    )
    if (
        value["schemaVersion"] != LOCK_SCHEMA
        or value["evidenceClass"] != EVIDENCE_CLASS
    ):
        fail("TOOLCHAIN_LOCK_INVALID")
    if (
        require_keys(value["host"], set(EXPECTED_HOST), "TOOLCHAIN_LOCK_INVALID")
        != EXPECTED_HOST
    ):
        fail("TOOLCHAIN_LOCK_INVALID")
    packages = value["aptPackages"]
    if (
        not isinstance(packages, list)
        or not 1 <= len(packages) <= 64
        or tuple(packages) != EXPECTED_APT_PACKAGES
        or any(
            not isinstance(item, str) or not PACKAGE_RE.fullmatch(item)
            for item in packages
        )
    ):
        fail("TOOLCHAIN_LOCK_INVALID")
    raw_artifacts = value["artifacts"]
    if not isinstance(raw_artifacts, list) or len(raw_artifacts) != 4:
        fail("TOOLCHAIN_LOCK_INVALID")
    artifacts: list[ArtifactLock] = []
    names: set[str] = set()
    for raw_artifact in raw_artifacts:
        item = require_keys(
            raw_artifact,
            {"name", "version", "url", "size", "sha256"},
            "TOOLCHAIN_LOCK_INVALID",
        )
        name, version, url = item["name"], item["version"], item["url"]
        size, digest = item["size"], item["sha256"]
        parsed = urllib.parse.urlsplit(url) if isinstance(url, str) else None
        expected_artifact = (
            EXPECTED_ARTIFACT_LOCKS.get(name) if isinstance(name, str) else None
        )
        if (
            not isinstance(name, str)
            or name not in EXPECTED_ARTIFACTS
            or name in names
            or not isinstance(version, str)
            or not VERSION_RE.fullmatch(version)
            or parsed is None
            or parsed.scheme != "https"
            or parsed.hostname not in ALLOWED_DOWNLOAD_HOSTS
            or parsed.username is not None
            or parsed.password is not None
            or parsed.fragment
            or not isinstance(size, int)
            or isinstance(size, bool)
            or size < 1
            or size > 256 << 20
            or not isinstance(digest, str)
            or not FINGERPRINT_RE.fullmatch(digest)
            or expected_artifact is None
            or (version, url, size, digest) != expected_artifact
        ):
            fail("TOOLCHAIN_LOCK_INVALID")
        names.add(name)
        artifacts.append(ArtifactLock(name, version, url, size, digest))
    if names != EXPECTED_ARTIFACTS:
        fail("TOOLCHAIN_LOCK_INVALID")
    return ToolchainLock(
        path=path.resolve(),
        fingerprint=sha256_bytes(canonical_json(value)),
        apt_packages=tuple(packages),
        artifacts=tuple(artifacts),
    )


def parse_os_release(path: Path = Path("/usr/lib/os-release")) -> dict[str, str]:
    raw = read_regular(path, 64 << 10, "HOST_UNSUPPORTED").decode("utf-8")
    result: dict[str, str] = {}
    for line in raw.splitlines():
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        if value.startswith(('"', "'")) and value.endswith(value[0]):
            value = value[1:-1]
        result[key] = value
    return result


def is_wsl2(release: str | None = None) -> bool:
    value = release if release is not None else platform.release()
    return "microsoft" in value.lower() and "wsl2" in value.lower()


def host_architecture() -> str:
    return {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine(), "")


def pid1_name(proc_root: Path = Path("/proc")) -> str:
    try:
        return (proc_root / "1" / "comm").read_text(encoding="utf-8").strip()
    except OSError:
        return ""


def wsl_systemd_enabled(path: Path = Path("/etc/wsl.conf")) -> bool:
    try:
        raw = read_regular(path, 64 << 10, "WSL_CONF_INVALID").decode("utf-8")
    except LocalTestError:
        if not path.exists():
            return False
        raise
    parser = configparser.ConfigParser(interpolation=None, strict=True)
    try:
        parser.read_string(raw)
        return parser.getboolean("boot", "systemd", fallback=False)
    except (configparser.Error, ValueError):
        fail("WSL_CONF_INVALID")


def render_wsl_conf(raw: bytes) -> bytes:
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        fail("WSL_CONF_INVALID")
    if "\x00" in text or len(raw) > 64 << 10:
        fail("WSL_CONF_INVALID")
    lines = text.splitlines(keepends=True)
    sections: list[tuple[str, int]] = []
    for index, line in enumerate(lines):
        match = SECTION_RE.match(line.rstrip("\r\n"))
        if match:
            sections.append((match.group(1).strip().lower(), index))
    if sum(1 for name, _ in sections if name == "boot") > 1:
        fail("WSL_CONF_INVALID")
    newline = "\r\n" if "\r\n" in text else "\n"
    boot = next((index for name, index in sections if name == "boot"), None)
    if boot is None:
        prefix = "" if not text or text.endswith(("\n", "\r")) else newline
        suffix = f"{prefix}[boot]{newline}systemd=true{newline}"
        return (text + suffix).encode("utf-8")
    end = len(lines)
    for _, index in sections:
        if index > boot:
            end = index
            break
    matches = [
        index
        for index in range(boot + 1, end)
        if SYSTEMD_RE.match(lines[index].rstrip("\r\n"))
    ]
    if len(matches) > 1:
        fail("WSL_CONF_INVALID")
    if matches:
        line = lines[matches[0]]
        indentation = SYSTEMD_RE.match(line.rstrip("\r\n")).group(1)  # type: ignore[union-attr]
        ending = (
            "\r\n" if line.endswith("\r\n") else "\n" if line.endswith("\n") else ""
        )
        lines[matches[0]] = f"{indentation}systemd=true{ending or newline}"
    else:
        lines.insert(end, f"systemd=true{newline}")
    return "".join(lines).encode("utf-8")


def parse_subid(path: Path, username: str) -> tuple[int, int] | None:
    raw = read_regular(path, 1 << 20, "SUBORDINATE_IDS_INVALID").decode("utf-8")
    ranges: list[tuple[int, int, str]] = []
    for line in raw.splitlines():
        if not line or line.startswith("#"):
            continue
        parts = line.split(":")
        if len(parts) != 3:
            fail("SUBORDINATE_IDS_INVALID")
        try:
            start, count = int(parts[1]), int(parts[2])
        except ValueError:
            fail("SUBORDINATE_IDS_INVALID")
        if start < 1 or count < 1 or start + count > 2**32:
            fail("SUBORDINATE_IDS_INVALID")
        ranges.append((start, count, parts[0]))
    ordered = sorted(ranges)
    for left, right in zip(ordered, ordered[1:]):
        if left[0] + left[1] > right[0]:
            fail("SUBORDINATE_IDS_OVERLAP")
    owned = [(start, count) for start, count, owner in ranges if owner == username]
    if len(owned) != 1 or owned[0][1] < 65536:
        return None
    return owned[0]


def package_installed(package: str) -> bool:
    try:
        result = subprocess.run(
            ["/usr/bin/dpkg-query", "-W", "-f=${db:Status-Abbrev}", package],
            check=False,
            capture_output=True,
            text=True,
            env={"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"},
            timeout=10,
        )
    except (OSError, subprocess.SubprocessError):
        return False
    return result.returncode == 0 and result.stdout.startswith("ii ")


def mapping_helper_ready(path: Path) -> bool:
    try:
        metadata = path.stat(follow_symlinks=False)
    except OSError:
        return False
    return (
        stat.S_ISREG(metadata.st_mode)
        and metadata.st_uid == 0
        and metadata.st_mode & stat.S_ISUID != 0
        and metadata.st_mode & 0o022 == 0
    )


def delegated_cgroup_ready(
    proc_root: Path = Path("/proc"), cgroup_root: Path = Path("/sys/fs/cgroup")
) -> bool:
    try:
        membership = (proc_root / "self" / "cgroup").read_text(encoding="utf-8")
        controllers = set(
            (cgroup_root / "cgroup.controllers").read_text(encoding="utf-8").split()
        )
    except OSError:
        return False
    if not {"cpu", "memory", "pids"}.issubset(controllers):
        return False
    for line in membership.splitlines():
        parts = line.split(":", 2)
        if len(parts) != 3 or parts[0] != "0" or parts[1] or parts[2] == "/":
            continue
        target = cgroup_root / parts[2].lstrip("/") / "cgroup.procs"
        if os.access(target, os.W_OK):
            return True
    return False


def default_install_root(home: Path | None = None) -> Path:
    base = home if home is not None else Path.home()
    return base / ".local" / "share" / "neo-chat" / "agent-runner-local-test"


def validate_install_root(path: Path, home: Path | None = None) -> Path:
    owner_home = (home if home is not None else Path.home()).resolve()
    candidate = path.expanduser().resolve(strict=False)
    try:
        candidate.relative_to(owner_home)
    except ValueError:
        fail("INSTALL_ROOT_UNSAFE")
    if candidate == owner_home or any(
        part in PROTECTED_PROJECT_NAMES for part in candidate.parts
    ):
        fail("INSTALL_ROOT_UNSAFE")
    return candidate


def require_private_directory(path: Path, code: str, *, create: bool = False) -> None:
    if create and not path.exists():
        try:
            path.mkdir(mode=0o700)
        except OSError:
            fail(code)
    try:
        metadata = path.stat(follow_symlinks=False)
    except OSError:
        fail(code)
    if (
        not stat.S_ISDIR(metadata.st_mode)
        or metadata.st_uid != os.geteuid()
        or metadata.st_mode & 0o077 != 0
    ):
        fail(code)


def _validate_receipt_shape(
    value: dict[str, Any], lock: ToolchainLock, root: Path
) -> list[dict[str, Any]]:
    require_keys(
        value,
        {
            "schemaVersion",
            "evidenceClass",
            "productionEligible",
            "lockFingerprint",
            "host",
            "installRootFingerprint",
            "builtAt",
            "files",
        },
        "TOOLCHAIN_RECEIPT_INVALID",
    )
    if (
        value["schemaVersion"] != RECEIPT_SCHEMA
        or value["evidenceClass"] != EVIDENCE_CLASS
        or value["productionEligible"] is not False
        or value["lockFingerprint"] != lock.fingerprint
        or value["host"] != EXPECTED_HOST
        or value["installRootFingerprint"] != sha256_bytes(str(root).encode("utf-8"))
        or not isinstance(value["builtAt"], str)
    ):
        fail("TOOLCHAIN_RECEIPT_INVALID")
    files = value["files"]
    if not isinstance(files, list) or not 6 <= len(files) <= 16:
        fail("TOOLCHAIN_RECEIPT_INVALID")
    seen: set[str] = set()
    for entry in files:
        item = require_keys(
            entry, {"path", "mode", "size", "sha256"}, "TOOLCHAIN_RECEIPT_INVALID"
        )
        relative, mode, size, digest = (
            item["path"],
            item["mode"],
            item["size"],
            item["sha256"],
        )
        if (
            not isinstance(relative, str)
            or relative.startswith("/")
            or ".." in Path(relative).parts
            or relative in seen
            or not isinstance(mode, str)
            or not re.fullmatch(r"0[0-7]{3}", mode)
            or not isinstance(size, int)
            or isinstance(size, bool)
            or size < 1
            or not isinstance(digest, str)
            or not FINGERPRINT_RE.fullmatch(digest)
        ):
            fail("TOOLCHAIN_RECEIPT_INVALID")
        seen.add(relative)
    required = {
        "bin/agent-runtime-local-test-smoke",
        "bin/conmon",
        "bin/crun",
        "bin/neo-skill-local-test-workload",
        "bin/podman",
        "bin/podman-engine",
        "config/containers.conf",
        "config/seccomp-agent-v1.json",
        "config/storage.conf",
    }
    if not required.issubset(seen):
        fail("TOOLCHAIN_RECEIPT_INVALID")
    return files


def validate_toolchain_receipt(lock: ToolchainLock, root: Path) -> dict[str, Any]:
    for directory in (root, root / "bin", root / "config", root / "storage"):
        require_private_directory(directory, "TOOLCHAIN_RECEIPT_INVALID")
    receipt_path = root / "toolchain-receipt.json"
    try:
        receipt_metadata = receipt_path.stat(follow_symlinks=False)
    except OSError:
        fail("TOOLCHAIN_RECEIPT_INVALID")
    if (
        not stat.S_ISREG(receipt_metadata.st_mode)
        or receipt_metadata.st_uid != os.geteuid()
        or stat.S_IMODE(receipt_metadata.st_mode) != 0o600
    ):
        fail("TOOLCHAIN_RECEIPT_INVALID")
    value = duplicate_safe_json(
        read_regular(receipt_path, 64 << 10, "TOOLCHAIN_RECEIPT_INVALID"),
        "TOOLCHAIN_RECEIPT_INVALID",
    )
    files = _validate_receipt_shape(value, lock, root)
    for item in files:
        path = root / item["path"]
        try:
            metadata = path.stat(follow_symlinks=False)
        except OSError:
            fail("TOOLCHAIN_FILE_DRIFT")
        if (
            not stat.S_ISREG(metadata.st_mode)
            or metadata.st_uid != os.geteuid()
            or f"0{stat.S_IMODE(metadata.st_mode):03o}" != item["mode"]
            or metadata.st_size != item["size"]
            or metadata.st_mode & 0o022 != 0
            or sha256_file(path) != item["sha256"]
        ):
            fail("TOOLCHAIN_FILE_DRIFT")
    return value


def host_status(lock: ToolchainLock, root: Path) -> dict[str, Any]:
    os_release = parse_os_release()
    username = pwd.getpwuid(os.geteuid()).pw_name
    checks = {
        "nonRoot": os.geteuid() > 0,
        "ubuntu2204": os_release.get("ID") == "ubuntu"
        and os_release.get("VERSION_ID") == "22.04",
        "wsl2": is_wsl2(),
        "amd64": host_architecture() == "amd64",
        "systemdConfigured": wsl_systemd_enabled(),
        "systemdRunning": pid1_name() == "systemd",
        "packages": all(package_installed(package) for package in lock.apt_packages),
        "subuid": parse_subid(Path("/etc/subuid"), username) is not None,
        "subgid": parse_subid(Path("/etc/subgid"), username) is not None,
        "newuidmap": mapping_helper_ready(Path("/usr/bin/newuidmap")),
        "newgidmap": mapping_helper_ready(Path("/usr/bin/newgidmap")),
        "delegatedCgroup": delegated_cgroup_ready(),
        "toolchain": False,
    }
    try:
        validate_toolchain_receipt(lock, root)
        checks["toolchain"] = True
    except LocalTestError:
        pass
    base = (
        checks["nonRoot"]
        and checks["ubuntu2204"]
        and checks["wsl2"]
        and checks["amd64"]
    )
    if not base:
        code = "HOST_UNSUPPORTED"
    elif (
        not checks["systemdConfigured"]
        or not checks["packages"]
        or not checks["subuid"]
        or not checks["subgid"]
        or not checks["newuidmap"]
        or not checks["newgidmap"]
    ):
        code = "SYSTEM_BOOTSTRAP_REQUIRED"
    elif not checks["systemdRunning"]:
        code = "RESTART_REQUIRED"
    elif not checks["delegatedCgroup"]:
        code = "CGROUP_DELEGATION_REQUIRED"
    elif not checks["toolchain"]:
        code = "TOOLCHAIN_INSTALL_REQUIRED"
    else:
        code = "LOCAL_SMOKE_READY"
    return {
        "schemaVersion": "neo.agent-runner-local-test-status/v1",
        "evidenceClass": EVIDENCE_CLASS,
        "productionEligible": False,
        "code": code,
        "checks": checks,
    }


def atomic_write(path: Path, raw: bytes, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        os.fchmod(descriptor, mode)
        with os.fdopen(descriptor, "wb", closefd=True) as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def run_checked(
    arguments: Sequence[str],
    *,
    timeout: int = 1800,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            list(arguments),
            check=True,
            cwd=cwd,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
        )
    except (OSError, subprocess.SubprocessError):
        fail("COMMAND_FAILED")


def _state_path(state_root: Path) -> Path:
    return state_root / "bootstrap.json"


def _write_bootstrap_state(state_root: Path, value: dict[str, Any]) -> None:
    atomic_write(
        _state_path(state_root),
        json.dumps(value, indent=2, sort_keys=True).encode("utf-8") + b"\n",
        0o600,
    )


def _validate_bootstrap_state(
    value: dict[str, Any], operator_user: str | None = None
) -> dict[str, Any]:
    require_keys(value, BOOTSTRAP_STATE_KEYS, "BOOTSTRAP_STATE_INVALID")
    initially_missing = value["packagesInitiallyMissing"]
    if (
        value["schemaVersion"] != BOOTSTRAP_SCHEMA
        or value["evidenceClass"] != EVIDENCE_CLASS
        or value["productionEligible"] is not False
        or not isinstance(value["status"], str)
        or value["status"] not in {"applying", "applied", "rolling_back", "rolled_back"}
        or not isinstance(value["operatorUser"], str)
        or not value["operatorUser"]
        or (operator_user is not None and value["operatorUser"] != operator_user)
        or not isinstance(value["createdAt"], str)
        or not isinstance(value["wslConfExisted"], bool)
        or not isinstance(value["wslConfBeforeSha256"], str)
        or not FINGERPRINT_RE.fullmatch(value["wslConfBeforeSha256"])
        or not isinstance(value["wslConfAfterSha256"], str)
        or not FINGERPRINT_RE.fullmatch(value["wslConfAfterSha256"])
        or value["packagesRequested"] != list(EXPECTED_APT_PACKAGES)
        or not isinstance(initially_missing, list)
        or any(
            not isinstance(package, str) or package not in EXPECTED_APT_PACKAGES
            for package in initially_missing
        )
        or len(initially_missing) != len(set(initially_missing))
    ):
        fail("BOOTSTRAP_STATE_INVALID")
    return value


def _bootstrap_before_bytes(state: dict[str, Any], state_root: Path) -> bytes:
    if not state["wslConfExisted"]:
        before = b""
    else:
        before = read_regular(
            state_root / "wsl.conf.before", 64 << 10, "BOOTSTRAP_STATE_INVALID"
        )
    if sha256_bytes(before) != state["wslConfBeforeSha256"]:
        fail("BOOTSTRAP_STATE_INVALID")
    if sha256_bytes(render_wsl_conf(before)) != state["wslConfAfterSha256"]:
        fail("BOOTSTRAP_STATE_INVALID")
    return before


def bootstrap_system(
    lock: ToolchainLock,
    operator_user: str,
    state_root: Path = DEFAULT_STATE_ROOT,
    *,
    wsl_conf: Path = Path("/etc/wsl.conf"),
) -> str:
    if os.geteuid() != 0 or not operator_user or operator_user == "root":
        fail("ROOT_BOOTSTRAP_REQUIRED")
    try:
        account = pwd.getpwnam(operator_user)
    except KeyError:
        fail("OPERATOR_INVALID")
    if account.pw_uid <= 0 or account.pw_gid <= 0:
        fail("OPERATOR_INVALID")
    os_release = parse_os_release()
    if (
        os_release.get("ID") != "ubuntu"
        or os_release.get("VERSION_ID") != "22.04"
        or not is_wsl2()
        or host_architecture() != "amd64"
    ):
        fail("HOST_UNSUPPORTED")
    state_path = _state_path(state_root)
    if state_path.exists():
        state = _validate_bootstrap_state(
            duplicate_safe_json(
                read_regular(state_path, 64 << 10, "BOOTSTRAP_STATE_INVALID"),
                "BOOTSTRAP_STATE_INVALID",
            ),
            operator_user,
        )
        if state["status"] not in {"applying", "applied"}:
            fail("BOOTSTRAP_STATE_INVALID")
        before = _bootstrap_before_bytes(state, state_root)
        after = render_wsl_conf(before)
        current = (
            read_regular(wsl_conf, 64 << 10, "BOOTSTRAP_STATE_DRIFT")
            if wsl_conf.exists()
            else b""
        )
        current_fingerprint = sha256_bytes(current)
        if state["status"] == "applied":
            if current_fingerprint != state["wslConfAfterSha256"] or any(
                not package_installed(package) for package in lock.apt_packages
            ):
                fail("BOOTSTRAP_STATE_DRIFT")
            return (
                "RESTART_REQUIRED"
                if pid1_name() != "systemd"
                else "SYSTEM_BOOTSTRAP_READY"
            )
        if current_fingerprint not in {
            state["wslConfBeforeSha256"],
            state["wslConfAfterSha256"],
        }:
            fail("BOOTSTRAP_STATE_DRIFT")
        missing = [
            package for package in lock.apt_packages if not package_installed(package)
        ]
        if (
            state["wslConfBeforeSha256"] != state["wslConfAfterSha256"]
            and current_fingerprint == state["wslConfAfterSha256"]
            and missing
        ):
            fail("BOOTSTRAP_STATE_DRIFT")
        if missing:
            run_checked(["/usr/bin/apt-get", "update"], timeout=900)
            run_checked(
                [
                    "/usr/bin/apt-get",
                    "install",
                    "--yes",
                    "--no-install-recommends",
                    *missing,
                ],
                timeout=1800,
            )
        if current_fingerprint == state["wslConfBeforeSha256"]:
            atomic_write(wsl_conf, after, 0o644)
        state["status"] = "applied"
        _write_bootstrap_state(state_root, state)
        return (
            "RESTART_REQUIRED" if pid1_name() != "systemd" else "SYSTEM_BOOTSTRAP_READY"
        )
    before = (
        read_regular(wsl_conf, 64 << 10, "WSL_CONF_INVALID")
        if wsl_conf.exists()
        else b""
    )
    after = render_wsl_conf(before)
    missing = [
        package for package in lock.apt_packages if not package_installed(package)
    ]
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    state = {
        "schemaVersion": BOOTSTRAP_SCHEMA,
        "evidenceClass": EVIDENCE_CLASS,
        "productionEligible": False,
        "status": "applying",
        "operatorUser": operator_user,
        "createdAt": now,
        "wslConfExisted": wsl_conf.exists(),
        "wslConfBeforeSha256": sha256_bytes(before),
        "wslConfAfterSha256": sha256_bytes(after),
        "packagesRequested": list(lock.apt_packages),
        "packagesInitiallyMissing": missing,
    }
    if state_root.exists():
        fail("BOOTSTRAP_STATE_INVALID")
    state_root.mkdir(parents=True, exist_ok=False, mode=0o700)
    os.chmod(state_root, 0o700)
    atomic_write(
        state_root / "wsl.conf.before", before or b"# file did not exist\n", 0o600
    )
    _write_bootstrap_state(state_root, state)
    if missing:
        run_checked(["/usr/bin/apt-get", "update"], timeout=900)
        run_checked(
            [
                "/usr/bin/apt-get",
                "install",
                "--yes",
                "--no-install-recommends",
                *missing,
            ],
            timeout=1800,
        )
    atomic_write(wsl_conf, after, 0o644)
    state["status"] = "applied"
    _write_bootstrap_state(state_root, state)
    return "RESTART_REQUIRED" if pid1_name() != "systemd" else "SYSTEM_BOOTSTRAP_READY"


def rollback_system(
    state_root: Path = DEFAULT_STATE_ROOT,
    *,
    wsl_conf: Path = Path("/etc/wsl.conf"),
) -> str:
    if os.geteuid() != 0:
        fail("ROOT_BOOTSTRAP_REQUIRED")
    state_path = _state_path(state_root)
    state = _validate_bootstrap_state(
        duplicate_safe_json(
            read_regular(state_path, 64 << 10, "BOOTSTRAP_STATE_INVALID"),
            "BOOTSTRAP_STATE_INVALID",
        )
    )
    if state["status"] == "rolled_back":
        return "ROLLBACK_COMPLETE"
    before = _bootstrap_before_bytes(state, state_root)
    current = (
        read_regular(wsl_conf, 64 << 10, "ROLLBACK_WSL_CONF_DRIFT")
        if wsl_conf.exists()
        else b""
    )
    current_fingerprint = sha256_bytes(current)
    if (
        state["status"] == "applied"
        and current_fingerprint != state["wslConfAfterSha256"]
    ):
        fail("ROLLBACK_WSL_CONF_DRIFT")
    if state["status"] == "applying" and current_fingerprint not in {
        state["wslConfBeforeSha256"],
        state["wslConfAfterSha256"],
    }:
        fail("ROLLBACK_WSL_CONF_DRIFT")
    if state["status"] == "rolling_back" and current_fingerprint not in {
        state["wslConfBeforeSha256"],
        state["wslConfAfterSha256"],
    }:
        fail("ROLLBACK_WSL_CONF_DRIFT")
    state["status"] = "rolling_back"
    _write_bootstrap_state(state_root, state)
    if current_fingerprint == state["wslConfAfterSha256"]:
        if state["wslConfExisted"]:
            atomic_write(wsl_conf, before, 0o644)
        else:
            try:
                wsl_conf.unlink(missing_ok=False)
            except OSError:
                fail("ROLLBACK_WSL_CONF_FAILED")
    removable = [
        package
        for package in state["packagesInitiallyMissing"]
        if package in state["packagesRequested"]
        and package in lock_package_allowlist()
        and package_installed(package)
    ]
    if removable:
        run_checked(["/usr/bin/apt-get", "remove", "--yes", *removable], timeout=1800)
    state["status"] = "rolled_back"
    _write_bootstrap_state(state_root, state)
    return "ROLLBACK_COMPLETE"


def lock_package_allowlist() -> set[str]:
    # This fixed ceiling prevents a modified bootstrap state from becoming an
    # arbitrary apt-removal instruction. The checked-in lock must match it.
    return set(EXPECTED_APT_PACKAGES)


def download_artifact(artifact: ArtifactLock, destination: Path) -> None:
    request = urllib.request.Request(
        artifact.url, headers={"User-Agent": "neo-chat-agent-runner-local-test/1"}
    )
    digest = hashlib.sha256()
    written = 0
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), PinnedDownloadRedirectHandler()
    )
    open_request = opener.open
    try:
        with (
            open_request(request, timeout=60) as response,
            destination.open("xb") as output,
        ):
            if not download_url_allowed(
                response.geturl(), ALLOWED_DOWNLOAD_REDIRECT_HOSTS
            ):
                fail("DOWNLOAD_REDIRECT_REJECTED")
            while True:
                chunk = response.read(1 << 20)
                if not chunk:
                    break
                written += len(chunk)
                if written > artifact.size:
                    fail("DOWNLOAD_SIZE_DRIFT")
                digest.update(chunk)
                output.write(chunk)
            output.flush()
            os.fsync(output.fileno())
    except (OSError, urllib.error.URLError):
        fail("DOWNLOAD_FAILED")
    if written != artifact.size or "sha256:" + digest.hexdigest() != artifact.sha256:
        fail("DOWNLOAD_HASH_DRIFT")


def safe_extract_tar(archive: Path, destination: Path) -> None:
    try:
        with tarfile.open(archive, "r:gz") as source:
            members = source.getmembers()
            if len(members) > 200_000:
                fail("ARCHIVE_INVALID")
            expanded_size = 0
            for member in members:
                path = Path(member.name)
                if (
                    path.is_absolute()
                    or not path.parts
                    or ".." in path.parts
                    or not (
                        member.isfile()
                        or member.isdir()
                        or member.issym()
                        or member.islnk()
                    )
                ):
                    fail("ARCHIVE_INVALID")
                if member.isfile():
                    expanded_size += member.size
                    if member.size < 0 or expanded_size > 2 << 30:
                        fail("ARCHIVE_INVALID")
                if member.issym() or member.islnk():
                    link = Path(member.linkname)
                    if link.is_absolute() or ".." in link.parts:
                        fail("ARCHIVE_INVALID")
            source.extractall(destination, filter="data")
    except (OSError, tarfile.TarError, ValueError):
        fail("ARCHIVE_INVALID")


def _file_inventory(root: Path, relative_paths: Sequence[str]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for relative in sorted(relative_paths):
        path = root / relative
        metadata = path.stat(follow_symlinks=False)
        if not stat.S_ISREG(metadata.st_mode) or metadata.st_mode & 0o022:
            fail("TOOLCHAIN_FILE_INVALID")
        result.append(
            {
                "path": relative,
                "mode": f"0{stat.S_IMODE(metadata.st_mode):03o}",
                "size": metadata.st_size,
                "sha256": sha256_file(path),
            }
        )
    return result


def _podman_wrapper(root: Path, engine_digest: str) -> str:
    quoted_root = shlex.quote(str(root))
    quoted_home = shlex.quote(str(Path.home()))
    return f"""#!/usr/bin/env bash
set -euo pipefail
umask 077
root={quoted_root}
engine=\"$root/bin/podman-engine\"
expected='{engine_digest}'
actual=\"sha256:$(/usr/bin/sha256sum \"$engine\" | /usr/bin/awk '{{print $1}}')\"
[[ \"$actual\" == \"$expected\" ]] || {{ echo TOOLCHAIN_FILE_DRIFT >&2; exit 1; }}
uid=\"$(/usr/bin/id -u)\"
runtime_root=\"/run/user/$uid/neo-chat-agent-runner-local-test\"
/usr/bin/mkdir -p \"$runtime_root\" \"$runtime_root/storage\" \"$runtime_root/tmp\"
/usr/bin/chmod 0700 \"$runtime_root\" \"$runtime_root/storage\" \"$runtime_root/tmp\"
export HOME={quoted_home}
export XDG_RUNTIME_DIR=\"/run/user/$uid\"
export CONTAINERS_CONF=\"$root/config/containers.conf\"
export CONTAINERS_STORAGE_CONF=\"$root/config/storage.conf\"
export TMPDIR=\"$runtime_root/tmp\"
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy
exec \"$engine\" \"$@\"
"""


def _containers_config(root: Path) -> str:
    return f"""\
[engine]
cgroup_manager = "systemd"
conmon_path = ["{root}/bin/conmon"]
events_logger = "file"
runtime = "crun"
runtime_supports_json = ["crun"]
runtime_supports_nocgroups = ["crun"]

[engine.runtimes]
crun = ["{root}/bin/crun"]
"""


def _storage_config(root: Path, user_id: int) -> str:
    return f"""\
[storage]
driver = "overlay"
runroot = "/run/user/{user_id}/neo-chat-agent-runner-local-test/storage"
graphroot = "{root}/storage"

[storage.options.overlay]
mount_program = "/usr/bin/fuse-overlayfs"
mountopt = "nodev"
"""


def _publish_toolchain_release(
    release: Path, root: Path, receipt: dict[str, Any]
) -> None:
    (release / "storage").mkdir(mode=0o700)
    atomic_write(
        release / "toolchain-receipt.json",
        json.dumps(receipt, indent=2, sort_keys=True).encode("utf-8") + b"\n",
        0o600,
    )
    try:
        os.replace(release, root)
        parent = os.open(root.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)
    except OSError:
        fail("TOOLCHAIN_PUBLISH_FAILED")


def install_toolchain(lock: ToolchainLock, root: Path, project_dir: Path) -> str:
    if os.geteuid() == 0:
        fail("NON_ROOT_REQUIRED")
    root = validate_install_root(root)
    status = host_status(lock, root)
    if status["code"] not in {"TOOLCHAIN_INSTALL_REQUIRED", "LOCAL_SMOKE_READY"}:
        fail(status["code"])
    if status["code"] == "LOCAL_SMOKE_READY":
        return "TOOLCHAIN_READY"
    if root.exists():
        if any(root.iterdir()):
            fail("INSTALL_ROOT_NOT_EMPTY")
        root.rmdir()
    root.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    staging = Path(
        tempfile.mkdtemp(prefix=".agent-runner-local-test-install-", dir=root.parent)
    )
    try:
        downloads = staging / "downloads"
        extract = staging / "extract"
        release = staging / "release"
        for directory in (
            downloads,
            extract,
            release,
            release / "bin",
            release / "config",
        ):
            directory.mkdir(parents=True, exist_ok=True, mode=0o700)
        downloaded: dict[str, Path] = {}
        for artifact in lock.artifacts:
            destination = downloads / artifact.name
            download_artifact(artifact, destination)
            downloaded[artifact.name] = destination
        safe_extract_tar(downloaded["go"], extract / "go-toolchain")
        safe_extract_tar(downloaded["podman-source"], extract / "podman-source")
        go_binary = extract / "go-toolchain" / "go" / "bin" / "go"
        podman_source = extract / "podman-source" / "podman-6.1.0"
        build_env = {
            "PATH": f"{go_binary.parent}:/usr/bin:/bin",
            "HOME": str(staging / "build-home"),
            "LANG": "C",
            "LC_ALL": "C",
            "CGO_ENABLED": "1",
            "GOFLAGS": "-mod=vendor -trimpath",
            "GOTOOLCHAIN": "local",
            "SOURCE_DATE_EPOCH": "0",
        }
        Path(build_env["HOME"]).mkdir(mode=0o700)
        run_checked(
            [
                "make",
                "-j2",
                "podman",
                f"GO={go_binary}",
                "GIT_COMMIT=cade97a52ebdf9dbf9e81de8009015776837a074",
                "BUILD_INFO=0",
                "BUILD_ORIGIN=neo-chat-local-test",
                f"PREFIX={release}",
            ],
            timeout=3600,
            cwd=podman_source,
            env=build_env,
        )
        shutil.copy2(
            podman_source / "bin" / "podman", release / "bin" / "podman-engine"
        )
        shutil.copy2(downloaded["crun"], release / "bin" / "crun")
        shutil.copy2(downloaded["conmon"], release / "bin" / "conmon")
        backend = project_dir / "backend"
        backend_env = dict(build_env)
        backend_env["CGO_ENABLED"] = "0"
        backend_env["GOFLAGS"] = "-mod=readonly -trimpath"
        backend_env["GOPROXY"] = "https://proxy.golang.org,direct"
        backend_env["GOSUMDB"] = "sum.golang.org"
        for package, output in (
            ("./cmd/agent-runtime-local-test-smoke", "agent-runtime-local-test-smoke"),
            ("./cmd/neo-skill-local-test-workload", "neo-skill-local-test-workload"),
        ):
            run_checked(
                [
                    str(go_binary),
                    "build",
                    "-buildvcs=false",
                    "-ldflags=-s -w -buildid=",
                    "-o",
                    str(release / "bin" / output),
                    package,
                ],
                timeout=1800,
                cwd=backend,
                env=backend_env,
            )
        for path in (release / "bin").iterdir():
            path.chmod(0o755)
        engine_digest = sha256_file(release / "bin" / "podman-engine")
        (release / "bin" / "podman").write_text(
            _podman_wrapper(root, engine_digest), encoding="utf-8"
        )
        (release / "bin" / "podman").chmod(0o755)
        containers_conf = _containers_config(root)
        storage_conf = _storage_config(root, os.geteuid())
        (release / "config" / "containers.conf").write_text(
            containers_conf, encoding="utf-8"
        )
        (release / "config" / "storage.conf").write_text(storage_conf, encoding="utf-8")
        shutil.copy2(
            project_dir / "config" / "agent-runner" / "seccomp-agent-v1.json",
            release / "config" / "seccomp-agent-v1.json",
        )
        for path in (release / "config").iterdir():
            path.chmod(0o600)
        relative_files = [
            "bin/agent-runtime-local-test-smoke",
            "bin/conmon",
            "bin/crun",
            "bin/neo-skill-local-test-workload",
            "bin/podman",
            "bin/podman-engine",
            "config/containers.conf",
            "config/seccomp-agent-v1.json",
            "config/storage.conf",
        ]
        receipt = {
            "schemaVersion": RECEIPT_SCHEMA,
            "evidenceClass": EVIDENCE_CLASS,
            "productionEligible": False,
            "lockFingerprint": lock.fingerprint,
            "host": EXPECTED_HOST,
            "installRootFingerprint": sha256_bytes(str(root).encode("utf-8")),
            "builtAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "files": _file_inventory(release, relative_files),
        }
        _publish_toolchain_release(release, root, receipt)
        validate_toolchain_receipt(lock, root)
    finally:
        shutil.rmtree(staging, ignore_errors=True)
    return "TOOLCHAIN_READY"


def run_smoke(lock: ToolchainLock, root: Path) -> dict[str, Any]:
    if os.geteuid() == 0:
        fail("NON_ROOT_REQUIRED")
    root = validate_install_root(root)
    validate_toolchain_receipt(lock, root)
    status = host_status(lock, root)
    if status["code"] != "LOCAL_SMOKE_READY":
        fail(status["code"])
    reports = root / "reports"
    smoke_state = root / "smoke-state"
    require_private_directory(reports, "LOCAL_STATE_DRIFT", create=True)
    require_private_directory(smoke_state, "LOCAL_STATE_DRIFT", create=True)
    result = run_checked(
        [
            str(root / "bin" / "agent-runtime-local-test-smoke"),
            "--podman",
            str(root / "bin" / "podman"),
            "--workload",
            str(root / "bin" / "neo-skill-local-test-workload"),
            "--seccomp",
            str(root / "config" / "seccomp-agent-v1.json"),
            "--state-root",
            str(smoke_state),
        ],
        timeout=180,
        env={"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"},
    )
    report = duplicate_safe_json(
        result.stdout.encode("utf-8"), "LOCAL_SMOKE_REPORT_INVALID"
    )
    require_keys(
        report,
        {
            "schemaVersion",
            "evidenceClass",
            "productionEligible",
            "outcome",
            "runtimeVersion",
            "checks",
            "cleanup",
            "generatedAt",
        },
        "LOCAL_SMOKE_REPORT_INVALID",
    )
    if (
        report["schemaVersion"] != REPORT_SCHEMA
        or report["evidenceClass"] != EVIDENCE_CLASS
        or report["productionEligible"] is not False
        or report["outcome"] != "LOCAL_SKILL_SMOKE_PASSED"
        or report["runtimeVersion"] != "6.1.0"
        or report["checks"] != LOCAL_WORKLOAD_CHECKS
        or report["cleanup"]
        != {
            "managedSandboxes": 0,
            "scratchResidue": False,
            "artifactResidue": False,
        }
        or not isinstance(report["generatedAt"], str)
    ):
        fail("LOCAL_SMOKE_REPORT_INVALID")
    filename = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + ".json"
    atomic_write(
        reports / filename,
        json.dumps(report, indent=2, sort_keys=True).encode("utf-8") + b"\n",
        0o600,
    )
    return report


def rollback_user(lock: ToolchainLock, root: Path) -> str:
    if os.geteuid() == 0:
        fail("NON_ROOT_REQUIRED")
    root = validate_install_root(root)
    validate_toolchain_receipt(lock, root)
    allowed_top_level = {
        "bin",
        "config",
        "reports",
        "smoke-state",
        "storage",
        "toolchain-receipt.json",
    }
    if any(entry.name not in allowed_top_level for entry in root.iterdir()):
        fail("ROLLBACK_ROOT_DRIFT")
    podman = root / "bin" / "podman"
    try:
        result = subprocess.run(
            [
                str(podman),
                "ps",
                "--all",
                "--filter",
                "label=neo.runner.managed=true",
                "--format={{.ID}}",
            ],
            check=False,
            capture_output=True,
            text=True,
            env={"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"},
            timeout=30,
        )
    except (OSError, subprocess.SubprocessError):
        fail("ROLLBACK_STORAGE_FAILED")
    if result.returncode != 0 or result.stdout.strip():
        fail("ROLLBACK_MANAGED_SANDBOX_PRESENT")
    try:
        reset = subprocess.run(
            [str(podman), "system", "reset", "--force"],
            check=False,
            capture_output=True,
            text=True,
            env={"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"},
            timeout=120,
        )
    except (OSError, subprocess.SubprocessError):
        fail("ROLLBACK_STORAGE_FAILED")
    if reset.returncode != 0:
        fail("ROLLBACK_STORAGE_FAILED")
    shutil.rmtree(root)
    return "LOCAL_PROFILE_REMOVED"
