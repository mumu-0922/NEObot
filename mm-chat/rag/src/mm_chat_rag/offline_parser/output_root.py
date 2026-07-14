"""Dir-FD/no-follow owned output roots with quotas and stale-run scavenging."""

from __future__ import annotations

import contextlib
import fcntl
import os
import stat
import time
import uuid
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Final, Self, cast

from mm_chat_rag.offline_parser.canonical import (
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, OutputLimits

DEFAULT_OUTPUT_PARENT: Final = Path("/run/mm-chat-parser-harness")
_MARKER: Final = ".ownership.json"
_LOCK: Final = ".run.lock"
_LEDGER: Final = ".quota-ledger.json"
_HEARTBEAT: Final = ".heartbeat.json"
_GLOBAL_LOCK: Final = ".admission.lock"
_INTERNAL_FILES: Final = frozenset({_MARKER, _LOCK, _LEDGER, _HEARTBEAT})
_PRIVATE_DIRECTORY_MODE: Final = 0o700
_MAX_METADATA_BYTES: Final = 1_048_576


class OutputRootError(RuntimeError):
    """An output root fails ownership, quota, or safe-cleanup validation."""


@dataclass(frozen=True, slots=True)
class ScavengeResult:
    """Bounded stale-root scan result."""

    removed: tuple[str, ...]
    retained: tuple[str, ...]


class OwnedOutputRoot:
    """One exclusively locked, marker-bound harness output root."""

    def __init__(
        self,
        *,
        parent: Path,
        parent_fd: int,
        global_lock_fd: int,
        root_name: str,
        root_fd: int,
        run_lock_fd: int,
        marker: JsonObject,
        limits: OutputLimits,
    ) -> None:
        self._parent = parent
        self._parent_fd = parent_fd
        self._global_lock_fd = global_lock_fd
        self._root_name = root_name
        self._root_fd = root_fd
        self._run_lock_fd = run_lock_fd
        self._marker = marker
        self._limits = limits
        self._closed = False

    @classmethod
    def create(
        cls,
        *,
        parent: Path = DEFAULT_OUTPUT_PARENT,
        limits: OutputLimits = DEFAULT_CONFIG.output,
        allow_test_parent: bool = False,
    ) -> Self:
        """Atomically create one root under the fixed parent and acquire admission."""
        if parent != DEFAULT_OUTPUT_PARENT and not allow_test_parent:
            raise OutputRootError("nonstandard output parent is test-only")
        parent_fd = _open_parent(parent)
        global_lock_fd = -1
        root_fd = -1
        run_lock_fd = -1
        root_name = ""
        try:
            global_lock_fd = os.open(
                _GLOBAL_LOCK,
                os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW,
                0o600,
                dir_fd=parent_fd,
            )
            try:
                fcntl.flock(global_lock_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError as error:
                raise OutputRootError("another harness run owns admission") from error
            root_name = f"run-{uuid.uuid4().hex}"
            os.mkdir(root_name, 0o700, dir_fd=parent_fd)
            root_fd = os.open(
                root_name,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                dir_fd=parent_fd,
            )
            root_stat = os.fstat(root_fd)
            marker: JsonObject = {
                "bootId": _boot_id(),
                "device": root_stat.st_dev,
                "harnessSchemaVersion": "parser-harness-output.v1",
                "inode": root_stat.st_ino,
                "ownerPid": os.getpid(),
                "ownerProcessStartTime": _process_start_time(os.getpid()),
                "runId": root_name.removeprefix("run-"),
            }
            _create_file(root_fd, _MARKER, canonical_json_bytes(marker))
            run_lock_fd = os.open(
                _LOCK,
                os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
                0o600,
                dir_fd=root_fd,
            )
            fcntl.flock(run_lock_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            _atomic_replace_json(
                root_fd,
                _LEDGER,
                {
                    "aggregateBytes": 0,
                    "directories": [],
                    "files": [],
                    "reservedFiles": [],
                    "schemaVersion": "parser-output-quota-ledger.v1",
                },
            )
            instance = cls(
                parent=parent,
                parent_fd=parent_fd,
                global_lock_fd=global_lock_fd,
                root_name=root_name,
                root_fd=root_fd,
                run_lock_fd=run_lock_fd,
                marker=marker,
                limits=limits,
            )
            instance.heartbeat()
            return instance  # noqa: TRY300
        except Exception:
            for file_descriptor in (run_lock_fd, root_fd, global_lock_fd, parent_fd):
                if file_descriptor >= 0:
                    os.close(file_descriptor)
            if root_name:
                with contextlib.suppress(OSError):
                    cleanup_parent_fd = _open_parent(parent)
                    try:
                        os.rmdir(root_name, dir_fd=cleanup_parent_fd)
                    finally:
                        os.close(cleanup_parent_fd)
            raise

    @property
    def path(self) -> Path:
        """Return the diagnostic path; mutation still uses retained dir FDs."""
        return self._parent / self._root_name

    @property
    def run_id(self) -> str:
        """Return the random marker-bound run identifier."""
        return cast("str", self._marker["runId"])

    def heartbeat(self) -> None:
        """Atomically refresh liveness under the retained root dir FD."""
        self._require_open()
        _atomic_replace_json(
            self._root_fd,
            _HEARTBEAT,
            {
                "heartbeatUnixMillis": int(time.time() * 1000),
                "runId": self.run_id,
                "schemaVersion": "parser-output-heartbeat.v1",
            },
        )

    def write_artifact(  # noqa: PLR0915
        self,
        relative_path: str,
        content: bytes,
    ) -> None:
        """Reserve quota then create one registered regular file without symlinks."""
        self._require_open()
        parts = _validate_relative_path(relative_path)
        if len(content) > self._limits.artifact_bytes:
            raise OutputRootError("artifact exceeds the per-file quota")
        ledger = self._read_ledger()
        files = _string_list(ledger, "files")
        reserved = _string_list(ledger, "reservedFiles")
        directories = _string_list(ledger, "directories")
        if relative_path in files or relative_path in reserved:
            raise OutputRootError("artifact path is already registered")
        aggregate = _integer(ledger, "aggregateBytes")
        if aggregate + len(content) > self._limits.aggregate_bytes:
            raise OutputRootError("run output exceeds the aggregate byte quota")
        if len(files) + len(reserved) + 1 > self._limits.files:
            raise OutputRootError("run output exceeds the file quota")

        parent_fd = self._root_fd
        opened: list[int] = []
        accumulated: list[str] = []
        try:
            for segment in parts[:-1]:
                accumulated.append(segment)
                directory = "/".join(accumulated)
                if directory not in directories:
                    try:
                        os.mkdir(segment, 0o700, dir_fd=parent_fd)
                    except FileExistsError as error:
                        raise OutputRootError(
                            "unregistered directory already exists"
                        ) from error
                    directories.append(directory)
                child_fd = os.open(
                    segment,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=parent_fd,
                )
                opened.append(child_fd)
                parent_fd = child_fd
            reserved.append(relative_path)
            ledger["directories"] = cast("JsonValue", sorted(set(directories)))
            ledger["reservedFiles"] = cast("JsonValue", sorted(reserved))
            ledger["aggregateBytes"] = aggregate + len(content)
            self._write_ledger(ledger)
            _create_file(parent_fd, parts[-1], content)
            reserved.remove(relative_path)
            files.append(relative_path)
            ledger["files"] = cast("JsonValue", sorted(files))
            ledger["reservedFiles"] = cast("JsonValue", sorted(reserved))
            self._write_ledger(ledger)
        except Exception:
            if relative_path in reserved:
                reserved.remove(relative_path)
                ledger["reservedFiles"] = cast("JsonValue", sorted(reserved))
                ledger["aggregateBytes"] = aggregate
                self._write_ledger(ledger)
            raise
        finally:
            for file_descriptor in reversed(opened):
                os.close(file_descriptor)

    def cleanup(self) -> None:
        """Delete only marker-verified registered children, then the owned root."""
        if self._closed:
            return
        _verify_root(self._root_fd, self._marker, require_owner=True)
        _cleanup_root_contents(self._root_fd)
        os.rmdir(self._root_name, dir_fd=self._parent_fd)
        self._closed = True
        for file_descriptor in (
            self._run_lock_fd,
            self._root_fd,
            self._global_lock_fd,
            self._parent_fd,
        ):
            os.close(file_descriptor)

    def __enter__(self) -> Self:
        return self

    def __exit__(self, _type: object, _value: object, _traceback: object) -> None:
        self.cleanup()

    def _read_ledger(self) -> JsonObject:
        return _read_json_at(self._root_fd, _LEDGER)

    def _write_ledger(self, ledger: JsonObject) -> None:
        _atomic_replace_json(self._root_fd, _LEDGER, ledger)

    def _require_open(self) -> None:
        if self._closed:
            raise OutputRootError("output root is closed")


def scavenge_stale_roots(
    *,
    parent: Path = DEFAULT_OUTPUT_PARENT,
    stale_after_seconds: float,
    allow_test_parent: bool = False,
) -> ScavengeResult:
    """Remove only unlocked, marker-valid direct children with dead owners."""
    if parent != DEFAULT_OUTPUT_PARENT and not allow_test_parent:
        raise OutputRootError("nonstandard output parent is test-only")
    parent_fd = _open_parent(parent)
    removed: list[str] = []
    retained: list[str] = []
    try:
        names = sorted(
            entry.name for entry in os.scandir(parent) if entry.name.startswith("run-")
        )
        for name in names:
            root_fd = -1
            lock_fd = -1
            try:
                root_fd = os.open(
                    name,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=parent_fd,
                )
                lock_fd = os.open(
                    _LOCK,
                    os.O_RDWR | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=root_fd,
                )
                try:
                    fcntl.flock(lock_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError:
                    retained.append(name)
                    continue
                marker = _read_json_at(root_fd, _MARKER)
                _verify_root(root_fd, marker, require_owner=False)
                heartbeat = _read_json_at(root_fd, _HEARTBEAT)
                observed_millis = _integer(heartbeat, "heartbeatUnixMillis")
                stale = (
                    time.time() * 1000 - observed_millis > stale_after_seconds * 1000
                )
                boot_changed = marker.get("bootId") != _boot_id()
                owner_alive = _pid_identity_exists(
                    _integer(marker, "ownerPid"),
                    _integer(marker, "ownerProcessStartTime"),
                )
                if not boot_changed and (not stale or owner_alive):
                    retained.append(name)
                    continue
                _cleanup_root_contents(root_fd)
                os.rmdir(name, dir_fd=parent_fd)
                removed.append(name)
            except (OSError, OutputRootError, ValueError):
                retained.append(name)
            finally:
                if lock_fd >= 0:
                    os.close(lock_fd)
                if root_fd >= 0:
                    os.close(root_fd)
    finally:
        os.close(parent_fd)
    return ScavengeResult(tuple(removed), tuple(retained))


def _open_parent(parent: Path) -> int:
    try:
        file_descriptor = os.open(
            parent,
            os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        )
    except OSError as error:
        raise OutputRootError("fixed output parent is unavailable") from error
    observed = os.fstat(file_descriptor)
    if (
        not stat.S_ISDIR(observed.st_mode)
        or stat.S_IMODE(observed.st_mode) != _PRIVATE_DIRECTORY_MODE
    ):
        os.close(file_descriptor)
        raise OutputRootError("output parent must be a mode-0700 directory")
    if observed.st_uid != os.geteuid():
        os.close(file_descriptor)
        raise OutputRootError("output parent owner does not match the harness")
    return file_descriptor


def _verify_root(root_fd: int, marker: JsonObject, *, require_owner: bool) -> None:
    observed = os.fstat(root_fd)
    if (
        not stat.S_ISDIR(observed.st_mode)
        or stat.S_IMODE(observed.st_mode) != _PRIVATE_DIRECTORY_MODE
    ):
        raise OutputRootError("owned root mode or type changed")
    if marker.get("harnessSchemaVersion") != "parser-harness-output.v1":
        raise OutputRootError("ownership marker schema changed")
    if (
        marker.get("device") != observed.st_dev
        or marker.get("inode") != observed.st_ino
    ):
        raise OutputRootError("ownership marker device/inode changed")
    persisted = _read_json_at(root_fd, _MARKER)
    if persisted != marker:
        raise OutputRootError("ownership marker changed")
    if require_owner and not _pid_identity_exists(
        _integer(marker, "ownerPid"),
        _integer(marker, "ownerProcessStartTime"),
    ):
        raise OutputRootError("ownership marker process identity changed")


def _cleanup_root_contents(root_fd: int) -> None:
    ledger = _read_json_at(root_fd, _LEDGER)
    files = set(_string_list(ledger, "files")) | set(
        _string_list(ledger, "reservedFiles")
    )
    directories = set(_string_list(ledger, "directories"))
    expected_top = _INTERNAL_FILES | {
        PurePosixPath(path).parts[0] for path in files | directories
    }
    observed_top = {entry.name for entry in os.scandir(f"/proc/self/fd/{root_fd}")}
    unexpected = observed_top - expected_top
    if unexpected:
        raise OutputRootError("owned root contains an unregistered child")
    for path in sorted(
        files, key=lambda value: len(PurePosixPath(value).parts), reverse=True
    ):
        _unlink_registered_file(root_fd, path)
    for path in sorted(
        directories, key=lambda value: len(PurePosixPath(value).parts), reverse=True
    ):
        _rmdir_registered(root_fd, path)
    for name in (_HEARTBEAT, _LEDGER, _MARKER, _LOCK):
        _unlink_regular(root_fd, name, missing_ok=False)


def _unlink_registered_file(root_fd: int, relative_path: str) -> None:
    parent_fd, leaf, opened = _walk_parent(root_fd, relative_path)
    try:
        _unlink_regular(parent_fd, leaf, missing_ok=True)
    finally:
        for file_descriptor in reversed(opened):
            os.close(file_descriptor)


def _rmdir_registered(root_fd: int, relative_path: str) -> None:
    parent_fd, leaf, opened = _walk_parent(root_fd, relative_path)
    try:
        try:
            observed = os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False)
        except FileNotFoundError:
            return
        if not stat.S_ISDIR(observed.st_mode):
            raise OutputRootError("registered directory changed type")
        os.rmdir(leaf, dir_fd=parent_fd)
    finally:
        for file_descriptor in reversed(opened):
            os.close(file_descriptor)


def _walk_parent(root_fd: int, relative_path: str) -> tuple[int, str, list[int]]:
    parts = _validate_relative_path(relative_path)
    current = root_fd
    opened: list[int] = []
    for segment in parts[:-1]:
        child = os.open(
            segment,
            os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=current,
        )
        opened.append(child)
        current = child
    return current, parts[-1], opened


def _create_file(directory_fd: int, name: str, content: bytes) -> None:
    file_descriptor = os.open(
        name,
        os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
        0o600,
        dir_fd=directory_fd,
    )
    try:
        view = memoryview(content)
        while view:
            written = os.write(file_descriptor, view)
            if written <= 0:
                raise OutputRootError("short artifact write")
            view = view[written:]
        os.fsync(file_descriptor)
    finally:
        os.close(file_descriptor)


def _atomic_replace_json(directory_fd: int, name: str, value: JsonObject) -> None:
    temporary = f".{name}.tmp-{uuid.uuid4().hex}"
    try:
        _create_file(directory_fd, temporary, canonical_json_bytes(value))
        os.replace(temporary, name, src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
        os.fsync(directory_fd)
    except Exception:
        with contextlib.suppress(FileNotFoundError):
            os.unlink(temporary, dir_fd=directory_fd)
        raise


def _read_json_at(directory_fd: int, name: str) -> JsonObject:
    file_descriptor = os.open(
        name,
        os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
        dir_fd=directory_fd,
    )
    try:
        observed = os.fstat(file_descriptor)
        if not stat.S_ISREG(observed.st_mode) or observed.st_size > _MAX_METADATA_BYTES:
            raise OutputRootError("owned metadata is not a bounded regular file")
        content = bytearray()
        while chunk := os.read(file_descriptor, 65_536):
            content.extend(chunk)
    finally:
        os.close(file_descriptor)
    try:
        return load_canonical_json_object(bytes(content))
    except ValueError as error:
        raise OutputRootError("owned metadata is not canonical JSON") from error


def _unlink_regular(directory_fd: int, name: str, *, missing_ok: bool) -> None:
    try:
        observed = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        if missing_ok:
            return
        raise
    if not stat.S_ISREG(observed.st_mode):
        raise OutputRootError("registered file changed type")
    os.unlink(name, dir_fd=directory_fd)


def _validate_relative_path(relative_path: str) -> tuple[str, ...]:
    path = PurePosixPath(relative_path)
    if not relative_path or path.is_absolute():
        raise OutputRootError("artifact path must be relative")
    parts = path.parts
    if not parts or any(
        part in {"", ".", ".."} or "\x00" in part or "\\" in part for part in parts
    ):
        raise OutputRootError("artifact path contains an unsafe segment")
    if parts[0].startswith(".") or any(part in _INTERNAL_FILES for part in parts):
        raise OutputRootError("artifact path collides with harness metadata")
    return parts


def _boot_id() -> str:
    try:
        value = (
            Path("/proc/sys/kernel/random/boot_id").read_text(encoding="ascii").strip()
        )
    except OSError as error:
        raise OutputRootError("host boot ID is unavailable") from error
    try:
        return str(uuid.UUID(value))
    except ValueError as error:
        raise OutputRootError("host boot ID is invalid") from error


def _process_start_time(pid: int) -> int:
    try:
        content = Path(f"/proc/{pid}/stat").read_text(encoding="ascii")
    except OSError as error:
        raise OutputRootError("process identity is unavailable") from error
    closing = content.rfind(")")
    if closing < 0:
        raise OutputRootError("process identity is malformed")
    fields = content[closing + 2 :].split()
    try:
        return int(fields[19])
    except (IndexError, ValueError) as error:
        raise OutputRootError("process start time is malformed") from error


def _pid_identity_exists(pid: int, expected_start_time: int) -> bool:
    try:
        return _process_start_time(pid) == expected_start_time
    except OutputRootError:
        return False


def _string_list(value: JsonObject, key: str) -> list[str]:
    observed = value.get(key)
    if not isinstance(observed, list) or not all(
        isinstance(item, str) for item in observed
    ):
        raise OutputRootError(f"{key} is not a string list")
    return cast("list[str]", observed.copy())


def _integer(value: JsonObject, key: str) -> int:
    observed = value.get(key)
    if type(observed) is not int:
        raise OutputRootError(f"{key} is not an integer")
    return observed
