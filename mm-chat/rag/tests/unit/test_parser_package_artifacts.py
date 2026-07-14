from __future__ import annotations

import json
from pathlib import Path
from typing import Any
from zipfile import ZIP_STORED, ZipFile

import pytest
from tools.verify_contract_wheel import (
    MAX_SAFE_INTEGER,
    REQUIRED_ARTIFACTS,
    SCHEMA_NAMES,
    WheelVerificationError,
    main,
    verify_wheel,
)

from mm_chat_rag.contracts.resources import schema_names

_PACKAGE_STRUCTURE = frozenset(
    {
        "mm_chat_rag/contracts/__init__.py",
        "mm_chat_rag/contracts/schemas/__init__.py",
    }
)


def _write_synthetic_wheel(
    path: Path,
    *,
    missing: str | None = None,
    extra: tuple[str, ...] = (),
) -> None:
    members = set(REQUIRED_ARTIFACTS | _PACKAGE_STRUCTURE)
    if missing is not None:
        members.remove(missing)
    with ZipFile(path, mode="w", compression=ZIP_STORED) as archive:
        for member in sorted(members):
            archive.writestr(member, f"synthetic:{member}\n".encode())
        for member in extra:
            archive.writestr(member, b"synthetic-extra\n")


def _assert_safe_json(value: Any) -> None:  # noqa: ANN401
    if value is None or isinstance(value, (bool, str)):
        return
    if type(value) is int:
        assert abs(value) <= MAX_SAFE_INTEGER
        return
    if isinstance(value, list):
        for item in value:
            _assert_safe_json(item)
        return
    assert isinstance(value, dict)
    for key, item in value.items():
        assert isinstance(key, str)
        _assert_safe_json(item)


def _error_code(path: Path) -> str:
    with pytest.raises(WheelVerificationError) as captured:
        verify_wheel(path)
    return captured.value.code


def test_wheel_verifier_schema_inventory_matches_the_packaged_allowlist() -> None:
    assert schema_names() == SCHEMA_NAMES


def test_valid_synthetic_wheel_passes_with_canonical_jcs_summary(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    wheel = tmp_path / "mm_chat_rag-0.1.0-py3-none-any.whl"
    _write_synthetic_wheel(wheel)

    summary = verify_wheel(wheel)
    assert summary == {
        "checkedArtifactCount": 21,
        "memberCount": 23,
        "schemaCount": 18,
        "schemaVersion": "mm-chat.contract-wheel-verification.v1",
        "status": "pass",
        "wheelBytes": wheel.stat().st_size,
    }
    assert len(SCHEMA_NAMES) == 18
    assert main([str(wheel)]) == 0
    captured = capsys.readouterr()
    assert captured.err == ""
    assert captured.out.endswith("\n")
    assert captured.out.count("\n") == 1
    body = captured.out[:-1]
    parsed = json.loads(body)
    _assert_safe_json(parsed)
    assert body == json.dumps(
        parsed,
        ensure_ascii=False,
        allow_nan=False,
        sort_keys=True,
        separators=(",", ":"),
    )


@pytest.mark.parametrize("missing", sorted(REQUIRED_ARTIFACTS))
def test_each_required_contract_artifact_is_enforced(
    tmp_path: Path,
    missing: str,
) -> None:
    wheel = tmp_path / "missing.whl"
    _write_synthetic_wheel(wheel, missing=missing)

    assert _error_code(wheel) == "REQUIRED_ARTIFACT_MISSING"


@pytest.mark.parametrize(
    ("extra", "expected_code"),
    [
        (
            "mm_chat_rag/contracts/schemas/unexpected.v1.schema.json",
            "UNEXPECTED_CONTRACT_ARTIFACT",
        ),
        ("mm_chat_rag/contracts", "UNEXPECTED_CONTRACT_ARTIFACT"),
        ("mm_chat_rag/contracts/NOTES.md", "UNEXPECTED_CONTRACT_ARTIFACT"),
        ("tests/test_leak.py", "FORBIDDEN_SOURCE_ARTIFACT"),
        ("tests/", "FORBIDDEN_SOURCE_ARTIFACT"),
        ("tools/audit.py", "FORBIDDEN_SOURCE_ARTIFACT"),
        ("tools/", "FORBIDDEN_SOURCE_ARTIFACT"),
        ("mm_chat_rag/tests/test_leak.py", "FORBIDDEN_SOURCE_ARTIFACT"),
        ("mm_chat_rag/test_leak.py", "FORBIDDEN_SOURCE_ARTIFACT"),
    ],
)
def test_extra_contract_and_source_artifacts_fail_closed(
    tmp_path: Path,
    extra: str,
    expected_code: str,
) -> None:
    wheel = tmp_path / "extra.whl"
    _write_synthetic_wheel(wheel, extra=(extra,))

    assert _error_code(wheel) == expected_code


@pytest.mark.parametrize(
    "member",
    [
        "../tests/leak.py",
        "/absolute.py",
        "C:/escape.py",
        "mm_chat_rag\\contracts\\schemas\\alias.schema.json",
        "mm_chat_rag/contracts/./alias.py",
    ],
)
def test_noncanonical_or_escaping_archive_paths_are_rejected(
    tmp_path: Path,
    member: str,
) -> None:
    wheel = tmp_path / "unsafe-path.whl"
    _write_synthetic_wheel(wheel, extra=(member,))

    assert _error_code(wheel) == "WHEEL_MEMBER_PATH_INVALID"


def test_external_wheel_path_must_exist_be_a_file_and_end_in_whl(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    missing = tmp_path / "missing.whl"
    directory = tmp_path / "directory.whl"
    wrong_extension = tmp_path / "artifact.zip"
    directory.mkdir()
    _write_synthetic_wheel(wrong_extension)

    assert _error_code(missing) == "WHEEL_PATH_NOT_FOUND"
    assert _error_code(directory) == "WHEEL_PATH_NOT_FILE"
    assert _error_code(wrong_extension) == "WHEEL_PATH_EXTENSION_INVALID"

    assert main([str(missing)]) == 1
    captured = capsys.readouterr()
    assert captured.err == ""
    body = captured.out.removesuffix("\n")
    parsed = json.loads(body)
    assert parsed == {
        "error": "WHEEL_PATH_NOT_FOUND",
        "schemaVersion": "mm-chat.contract-wheel-verification.v1",
        "status": "fail",
    }
    _assert_safe_json(parsed)
    assert body == json.dumps(parsed, sort_keys=True, separators=(",", ":"))
