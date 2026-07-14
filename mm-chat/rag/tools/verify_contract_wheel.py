"""Verify parser contract artifacts in one existing wheel without building it."""

from __future__ import annotations

import argparse
import json
import stat
import sys
from collections.abc import Sequence
from pathlib import Path
from typing import Final
from zipfile import BadZipFile, LargeZipFile, ZipFile, ZipInfo

type JsonValue = None | bool | int | str | list[JsonValue] | dict[str, JsonValue]

MAX_SAFE_INTEGER: Final = (1 << 53) - 1
SCHEMA_NAMES: Final = (
    "canonical-common.v1.schema.json",
    "canonical-ir.v2.schema.json",
    "canonical-manifest.v2.schema.json",
    "chunk-manifest.v2.schema.json",
    "chunk-profile.v1.schema.json",
    "logical-hash-envelope.v1.schema.json",
    "normalization-map.v1.schema.json",
    "normalization-profile.v1.schema.json",
    "parser-corpus-manifest.v1.schema.json",
    "parser-profile.v1.schema.json",
    "parser-protocol-request-header.v1.schema.json",
    "parser-protocol-response-header.v1.schema.json",
    "parser-resource-profile.v1.schema.json",
    "parser-stable-error.v1.schema.json",
    "quality-report.v2.schema.json",
    "source-locator.v2.schema.json",
    "source-unit-resolver.v1.schema.json",
    "synthetic-mineru-artifact.v1.schema.json",
)

_CONTRACT_ROOT: Final = "mm_chat_rag/contracts"
_SCHEMA_ROOT: Final = f"{_CONTRACT_ROOT}/schemas"
REQUIRED_ARTIFACTS: Final = frozenset(
    {
        f"{_CONTRACT_ROOT}/DESIGN.md",
        f"{_CONTRACT_ROOT}/README.md",
        f"{_CONTRACT_ROOT}/resources.py",
        *(f"{_SCHEMA_ROOT}/{name}" for name in SCHEMA_NAMES),
    }
)
_ALLOWED_CONTRACT_ARTIFACTS: Final = REQUIRED_ARTIFACTS | frozenset(
    {
        f"{_CONTRACT_ROOT}/__init__.py",
        f"{_SCHEMA_ROOT}/__init__.py",
    }
)
_ALLOWED_CONTRACT_DIRECTORIES: Final = frozenset({_CONTRACT_ROOT, _SCHEMA_ROOT})
_FORBIDDEN_DIRECTORY_NAMES: Final = frozenset({"test", "tests", "tool", "tools"})
_ERROR_CODES: Final = frozenset(
    {
        "FORBIDDEN_SOURCE_ARTIFACT",
        "REQUIRED_ARTIFACT_EMPTY",
        "REQUIRED_ARTIFACT_MISSING",
        "UNEXPECTED_CONTRACT_ARTIFACT",
        "WHEEL_ARCHIVE_INVALID",
        "WHEEL_ARCHIVE_LIMIT_EXCEEDED",
        "WHEEL_MEMBER_DUPLICATE",
        "WHEEL_MEMBER_PATH_INVALID",
        "WHEEL_PATH_EXTENSION_INVALID",
        "WHEEL_PATH_INVALID",
        "WHEEL_PATH_NOT_FILE",
        "WHEEL_PATH_NOT_FOUND",
    }
)
_SUMMARY_SCHEMA_VERSION: Final = "mm-chat.contract-wheel-verification.v1"
_MAX_ARCHIVE_MEMBERS: Final = 10_000
_MAX_MEMBER_BYTES: Final = 256 * 1024 * 1024
_MAX_TOTAL_UNCOMPRESSED_BYTES: Final = 1024 * 1024 * 1024
_MAX_MEMBER_PATH_LENGTH: Final = 4096
_ASCII_CONTROL_BOUNDARY: Final = 0x20
_ASCII_DELETE: Final = 0x7F
_SURROGATE_MIN: Final = 0xD800
_SURROGATE_MAX: Final = 0xDFFF


class WheelVerificationError(RuntimeError):
    """A stable, closed failure from local wheel verification."""

    def __init__(self, code: str) -> None:
        stable_code = code if code in _ERROR_CODES else "WHEEL_ARCHIVE_INVALID"
        super().__init__(stable_code)
        self.code = stable_code


def _resolve_wheel_path(wheel_path: Path) -> tuple[Path, int]:
    if wheel_path.is_symlink():
        raise WheelVerificationError("WHEEL_PATH_INVALID")
    try:
        resolved = wheel_path.resolve(strict=True)
    except FileNotFoundError as error:
        raise WheelVerificationError("WHEEL_PATH_NOT_FOUND") from error
    except (OSError, RuntimeError) as error:
        raise WheelVerificationError("WHEEL_PATH_INVALID") from error
    if not resolved.is_file():
        raise WheelVerificationError("WHEEL_PATH_NOT_FILE")
    if resolved.suffix != ".whl":
        raise WheelVerificationError("WHEEL_PATH_EXTENSION_INVALID")
    try:
        wheel_bytes = resolved.stat().st_size
    except OSError as error:
        raise WheelVerificationError("WHEEL_PATH_INVALID") from error
    if wheel_bytes < 1 or wheel_bytes > MAX_SAFE_INTEGER:
        raise WheelVerificationError("WHEEL_ARCHIVE_LIMIT_EXCEEDED")
    return resolved, wheel_bytes


def _validated_member_path(member: ZipInfo) -> tuple[str, bool]:
    raw_name = member.filename
    if (
        not raw_name
        or len(raw_name) > _MAX_MEMBER_PATH_LENGTH
        or "\\" in raw_name
        or any(
            ord(character) < _ASCII_CONTROL_BOUNDARY or ord(character) == _ASCII_DELETE
            for character in raw_name
        )
    ):
        raise WheelVerificationError("WHEEL_MEMBER_PATH_INVALID")

    is_directory = member.is_dir()
    path = raw_name[:-1] if is_directory else raw_name
    parts = path.split("/")
    if (
        not path
        or raw_name.startswith("/")
        or any(part in {"", ".", ".."} for part in parts)
        or parts[0].endswith(":")
        or "/".join(parts) != path
    ):
        raise WheelVerificationError("WHEEL_MEMBER_PATH_INVALID")

    mode = member.external_attr >> 16
    if stat.S_IFMT(mode) == stat.S_IFLNK:
        raise WheelVerificationError("WHEEL_MEMBER_PATH_INVALID")
    return path, is_directory


def _is_forbidden_source_artifact(path: str) -> bool:
    parts = tuple(part.casefold() for part in path.split("/"))
    if any(part in _FORBIDDEN_DIRECTORY_NAMES for part in parts):
        return True
    filename = parts[-1]
    return filename == "conftest.py" or (
        filename.endswith(".py")
        and (filename.startswith("test_") or filename.endswith("_test.py"))
    )


def _inspect_archive(archive: ZipFile) -> int:
    members = archive.infolist()
    if not members or len(members) > _MAX_ARCHIVE_MEMBERS:
        raise WheelVerificationError("WHEEL_ARCHIVE_LIMIT_EXCEEDED")

    seen_paths: set[str] = set()
    files_by_path: dict[str, ZipInfo] = {}
    total_uncompressed_bytes = 0
    for member in members:
        path, is_directory = _validated_member_path(member)
        if path in seen_paths:
            raise WheelVerificationError("WHEEL_MEMBER_DUPLICATE")
        seen_paths.add(path)
        if member.flag_bits & 0x1:
            raise WheelVerificationError("WHEEL_ARCHIVE_INVALID")
        if (
            member.file_size < 0
            or member.compress_size < 0
            or member.file_size > _MAX_MEMBER_BYTES
            or member.file_size > MAX_SAFE_INTEGER
            or member.compress_size > MAX_SAFE_INTEGER
        ):
            raise WheelVerificationError("WHEEL_ARCHIVE_LIMIT_EXCEEDED")
        total_uncompressed_bytes += member.file_size
        if total_uncompressed_bytes > _MAX_TOTAL_UNCOMPRESSED_BYTES:
            raise WheelVerificationError("WHEEL_ARCHIVE_LIMIT_EXCEEDED")
        if _is_forbidden_source_artifact(path):
            raise WheelVerificationError("FORBIDDEN_SOURCE_ARTIFACT")
        if is_directory:
            if member.file_size != 0:
                raise WheelVerificationError("WHEEL_ARCHIVE_INVALID")
            if path.startswith(f"{_CONTRACT_ROOT}/") and (
                path not in _ALLOWED_CONTRACT_DIRECTORIES
            ):
                raise WheelVerificationError("UNEXPECTED_CONTRACT_ARTIFACT")
            continue
        files_by_path[path] = member

    contract_artifacts = {
        path
        for path in files_by_path
        if path == _CONTRACT_ROOT or path.startswith(f"{_CONTRACT_ROOT}/")
    }
    if contract_artifacts - _ALLOWED_CONTRACT_ARTIFACTS:
        raise WheelVerificationError("UNEXPECTED_CONTRACT_ARTIFACT")
    if REQUIRED_ARTIFACTS - files_by_path.keys():
        raise WheelVerificationError("REQUIRED_ARTIFACT_MISSING")
    if any(files_by_path[path].file_size == 0 for path in REQUIRED_ARTIFACTS):
        raise WheelVerificationError("REQUIRED_ARTIFACT_EMPTY")

    if archive.testzip() is not None:
        raise WheelVerificationError("WHEEL_ARCHIVE_INVALID")
    return len(files_by_path)


def verify_wheel(wheel_path: Path) -> dict[str, JsonValue]:
    """Verify one already-built local wheel and return a closed pass summary."""
    resolved, wheel_bytes = _resolve_wheel_path(wheel_path)
    try:
        with ZipFile(resolved, mode="r") as archive:
            member_count = _inspect_archive(archive)
    except WheelVerificationError:
        raise
    except (
        BadZipFile,
        EOFError,
        LargeZipFile,
        NotImplementedError,
        OSError,
        RuntimeError,
        ValueError,
    ) as error:
        raise WheelVerificationError("WHEEL_ARCHIVE_INVALID") from error

    return {
        "checkedArtifactCount": len(REQUIRED_ARTIFACTS),
        "memberCount": member_count,
        "schemaCount": len(SCHEMA_NAMES),
        "schemaVersion": _SUMMARY_SCHEMA_VERSION,
        "status": "pass",
        "wheelBytes": wheel_bytes,
    }


def _validate_json_value(value: JsonValue) -> None:
    if value is None or isinstance(value, bool):
        return
    if isinstance(value, int):
        if abs(value) > MAX_SAFE_INTEGER:
            raise WheelVerificationError("WHEEL_ARCHIVE_LIMIT_EXCEEDED")
        return
    if isinstance(value, str):
        if "\x00" in value or any(
            _SURROGATE_MIN <= ord(character) <= _SURROGATE_MAX for character in value
        ):
            raise WheelVerificationError("WHEEL_ARCHIVE_INVALID")
        return
    if isinstance(value, list):
        for item in value:
            _validate_json_value(item)
        return
    for key, item in value.items():
        _validate_json_value(key)
        _validate_json_value(item)


def _canonical_json_line(value: dict[str, JsonValue]) -> str:
    _validate_json_value(value)
    return (
        json.dumps(
            value,
            ensure_ascii=False,
            allow_nan=False,
            sort_keys=True,
            separators=(",", ":"),
        )
        + "\n"
    )


def _argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Verify contract packaging in an existing local wheel.",
    )
    parser.add_argument("wheel", type=Path, help="path to an existing .whl file")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the local-only wheel verification CLI."""
    arguments = _argument_parser().parse_args(argv)
    try:
        summary = verify_wheel(arguments.wheel)
    except WheelVerificationError as error:
        summary = {
            "error": error.code,
            "schemaVersion": _SUMMARY_SCHEMA_VERSION,
            "status": "fail",
        }
        exit_code = 1
    else:
        exit_code = 0
    sys.stdout.write(_canonical_json_line(summary))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
