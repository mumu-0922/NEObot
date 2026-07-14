"""Owned output-root quota, no-follow cleanup, FD, and scavenger tests."""

from __future__ import annotations

import fcntl
import os
import subprocess
import sys
from pathlib import Path
from unittest.mock import patch

import pytest

from mm_chat_rag.offline_parser import output_root
from mm_chat_rag.offline_parser.config import OutputLimits
from mm_chat_rag.offline_parser.output_root import (
    OutputRootError,
    OwnedOutputRoot,
    scavenge_stale_roots,
)


def _private_parent(path: Path) -> Path:
    path.chmod(0o700)
    return path


def test_owned_root_writes_registered_artifacts_and_cleans_only_itself(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)

    with OwnedOutputRoot.create(parent=parent, allow_test_parent=True) as output:
        output.write_artifact("nested/result.json", b"{}")
        assert (output.path / "nested" / "result.json").read_bytes() == b"{}"
        assert fcntl.fcntl(output._root_fd, fcntl.F_GETFD) & fcntl.FD_CLOEXEC

    assert sorted(path.name for path in parent.iterdir()) == [".admission.lock"]


@pytest.mark.parametrize(
    "relative_path",
    ["", "/absolute", "../escape", "a/../../escape", ".ownership.json", "a\\b"],
)
def test_owned_root_rejects_arbitrary_or_metadata_paths(
    tmp_path: Path,
    relative_path: str,
) -> None:
    parent = _private_parent(tmp_path)

    with (
        OwnedOutputRoot.create(parent=parent, allow_test_parent=True) as output,
        pytest.raises(OutputRootError),
    ):
        output.write_artifact(relative_path, b"x")


def test_quota_reservation_rejects_file_and_aggregate_overflow(tmp_path: Path) -> None:
    parent = _private_parent(tmp_path)
    limits = OutputLimits(aggregate_bytes=4, artifact_bytes=3, files=1)

    with OwnedOutputRoot.create(
        parent=parent,
        limits=limits,
        allow_test_parent=True,
    ) as output:
        with pytest.raises(OutputRootError, match="per-file"):
            output.write_artifact("large.bin", b"1234")
        output.write_artifact("one.bin", b"123")
        with pytest.raises(OutputRootError, match="file quota"):
            output.write_artifact("two.bin", b"1")


def test_unregistered_symlink_blocks_cleanup_instead_of_following_it(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)
    outside = parent / "outside"
    outside.write_text("preserve", encoding="utf-8")
    output = OwnedOutputRoot.create(parent=parent, allow_test_parent=True)
    symlink = output.path / "unexpected"
    symlink.symlink_to(outside)

    with pytest.raises(OutputRootError, match="unregistered"):
        output.cleanup()
    assert outside.read_text(encoding="utf-8") == "preserve"

    symlink.unlink()
    output.cleanup()


def test_scavenger_removes_only_unlocked_stale_dead_owner(tmp_path: Path) -> None:
    parent = _private_parent(tmp_path)
    script = (
        "import os; from pathlib import Path; "
        "from mm_chat_rag.offline_parser.output_root import OwnedOutputRoot; "
        "root=OwnedOutputRoot.create("
        f"parent=Path({str(parent)!r}),allow_test_parent=True); "
        "print(root.path.name, flush=True); os._exit(0)"
    )
    completed = subprocess.run(  # noqa: S603
        [sys.executable, "-c", script],
        check=True,
        capture_output=True,
        text=True,
    )
    root_name = completed.stdout.strip()

    result = scavenge_stale_roots(
        parent=parent,
        stale_after_seconds=-1,
        allow_test_parent=True,
    )

    assert result.removed == (root_name,)
    assert result.retained == ()
    assert not (parent / root_name).exists()


def test_second_active_run_is_rejected_and_cleanup_is_idempotent(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)
    output = OwnedOutputRoot.create(parent=parent, allow_test_parent=True)
    try:
        with pytest.raises(OutputRootError, match="admission"):
            OwnedOutputRoot.create(parent=parent, allow_test_parent=True)
    finally:
        output.cleanup()
    output.cleanup()


def test_duplicate_aggregate_and_unregistered_directory_writes_fail_closed(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)
    limits = OutputLimits(aggregate_bytes=3, artifact_bytes=3, files=3)
    with OwnedOutputRoot.create(
        parent=parent,
        limits=limits,
        allow_test_parent=True,
    ) as output:
        output.write_artifact("one", b"12")
        with pytest.raises(OutputRootError, match="already registered"):
            output.write_artifact("one", b"1")
        with pytest.raises(OutputRootError, match="aggregate"):
            output.write_artifact("two", b"12")
        (output.path / "existing").mkdir()
        with pytest.raises(OutputRootError, match="unregistered directory"):
            output.write_artifact("existing/file", b"1")
        (output.path / "existing").rmdir()


def test_root_mode_marker_and_registered_type_tampering_are_rejected(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)
    output = OwnedOutputRoot.create(parent=parent, allow_test_parent=True)
    output.write_artifact("dir/file", b"x")
    output.path.chmod(0o755)
    with pytest.raises(OutputRootError, match="mode"):
        output.cleanup()
    output.path.chmod(0o700)

    artifact = output.path / "dir" / "file"
    artifact.unlink()
    artifact.symlink_to(parent / "outside")
    with pytest.raises(OutputRootError, match="changed type"):
        output.cleanup()
    artifact.unlink()
    output.cleanup()


def test_open_parent_rejects_missing_and_nonprivate_directories(tmp_path: Path) -> None:
    with pytest.raises(OutputRootError, match="unavailable"):
        output_root._open_parent(tmp_path / "missing")
    tmp_path.chmod(0o755)
    with pytest.raises(OutputRootError, match="mode-0700"):
        output_root._open_parent(tmp_path)


def test_scavenger_retains_locked_active_and_unverified_roots(tmp_path: Path) -> None:
    parent = _private_parent(tmp_path)
    output = OwnedOutputRoot.create(parent=parent, allow_test_parent=True)
    (parent / "run-invalid").mkdir(mode=0o700)
    try:
        result = scavenge_stale_roots(
            parent=parent,
            stale_after_seconds=-1,
            allow_test_parent=True,
        )
        assert output.path.name in result.retained
        assert "run-invalid" in result.retained
    finally:
        (parent / "run-invalid").rmdir()
        output.cleanup()


def test_output_metadata_and_identity_helpers_reject_malformed_state(
    tmp_path: Path,
) -> None:
    parent = _private_parent(tmp_path)
    directory_fd = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        (parent / "bad.json").write_text("not json", encoding="utf-8")
        with pytest.raises(OutputRootError, match="canonical JSON"):
            output_root._read_json_at(directory_fd, "bad.json")
        with pytest.raises(OutputRootError):
            output_root._string_list({"x": [1]}, "x")
        with pytest.raises(OutputRootError):
            output_root._integer({"x": "1"}, "x")
        with pytest.raises(FileNotFoundError):
            output_root._unlink_regular(directory_fd, "missing", missing_ok=False)
        output_root._unlink_regular(directory_fd, "missing", missing_ok=True)
    finally:
        os.close(directory_fd)

    with patch.object(Path, "read_text", side_effect=OSError):
        with pytest.raises(OutputRootError, match="boot ID"):
            output_root._boot_id()
        with pytest.raises(OutputRootError, match="process identity"):
            output_root._process_start_time(os.getpid())


def test_atomic_metadata_replace_cleans_temporary_file_on_failure(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    parent = _private_parent(tmp_path)
    directory_fd = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)

    def fail_replace(*_args: object, **_kwargs: object) -> None:
        raise OSError("replace failed")

    monkeypatch.setattr(output_root.os, "replace", fail_replace)
    try:
        with pytest.raises(OSError, match="replace failed"):
            output_root._atomic_replace_json(directory_fd, "x.json", {"x": 1})
        assert not any(
            path.name.startswith(".x.json.tmp-") for path in parent.iterdir()
        )
    finally:
        os.close(directory_fd)
