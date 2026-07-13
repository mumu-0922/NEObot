from __future__ import annotations

import io
import stat
import zipfile

import pytest
from tools.provider_capture import CaptureError
from tools.provider_capture_mineru_archive import (
    MAX_ARCHIVE_BYTES,
    ArchiveValidationError,
    validate_result_archive,
)
from tools.provider_capture_mineru_targets import (
    validate_result_target,
    validate_upload_target,
)

_UPLOAD_URL = (
    "https://mineru.oss-cn-shanghai.aliyuncs.com/"
    "api-upload/object.pdf?OSSAccessKeyId=fixture&Expires=1&Signature=fixture"
)
_RESULT_URL = "https://cdn-mineru.openxlab.org.cn/pdf/fixture-result.zip"


def _archive(entries: dict[str, bytes]) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        for name, content in entries.items():
            archive.writestr(name, content)
    return output.getvalue()


@pytest.mark.parametrize(
    "url",
    [
        "http://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/a?x=1",
        "https://evil.invalid/api-upload/a?x=1",
        "https://mineru.oss-cn-shanghai.aliyuncs.com:444/api-upload/a?x=1",
        "https://user:pass@mineru.oss-cn-shanghai.aliyuncs.com/api-upload/a?x=1",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/other/a?x=1",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/%2e%2e/a?x=1",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/a",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/a?x=1#fragment",
        "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/a?x=1\n",
    ],
)
def test_upload_target_is_exactly_allowlisted(url: str) -> None:
    with pytest.raises(CaptureError, match="MINERU_UPLOAD_TARGET_INVALID"):
        validate_upload_target(url)
    validate_upload_target(_UPLOAD_URL)


@pytest.mark.parametrize(
    "url",
    [
        "http://cdn-mineru.openxlab.org.cn/pdf/a.zip",
        "https://evil.invalid/pdf/a.zip",
        "https://cdn-mineru.openxlab.org.cn:444/pdf/a.zip",
        "https://cdn-mineru.openxlab.org.cn/other/a.zip",
        "https://cdn-mineru.openxlab.org.cn/pdf/%2e%2e/a.zip",
        "https://cdn-mineru.openxlab.org.cn/pdf/a.txt",
        "https://cdn-mineru.openxlab.org.cn/pdf/a.zip?token=x",
        "https://cdn-mineru.openxlab.org.cn/pdf/a.zip#fragment",
    ],
)
def test_result_target_is_exactly_allowlisted(url: str) -> None:
    with pytest.raises(CaptureError, match="MINERU_RESULT_TARGET_INVALID"):
        validate_result_target(url)
    validate_result_target(_RESULT_URL)


@pytest.mark.parametrize(
    "entries",
    [
        {"../full.md": b"x"},
        {"nested/./full.md": b"x"},
        {"full.md": b"x"},
        {
            "full.md": b"x",
            "fixture_content_list.json": b"[]",
            "fixture_middle.json": b"{}",
            "fixture_model.json": b"a" * 1000,
        },
    ],
)
def test_result_archive_rejects_traversal_missing_and_accepts_closed_shape(
    entries: dict[str, bytes],
) -> None:
    content = _archive(entries)
    if len(entries) == 4:
        assert validate_result_archive(content)["entryCount"] == 4
    else:
        expected = (
            "unsafe_entry_path"
            if next(iter(entries)) != "full.md"
            else "missing_content_list"
        )
        with pytest.raises(
            ArchiveValidationError,
            match="MINERU_ARCHIVE_INVALID",
        ) as captured:
            validate_result_archive(content)
        assert captured.value.failure_class == expected


def test_result_archive_rejects_oversized_bytes_before_zip_parse() -> None:
    with pytest.raises(CaptureError, match="PROVIDER_ARCHIVE_TOO_LARGE"):
        validate_result_archive(b"x" * (MAX_ARCHIVE_BYTES + 1))


def test_result_archive_rejects_symlink_duplicate_and_compression_bomb() -> None:
    symlink_output = io.BytesIO()
    with zipfile.ZipFile(symlink_output, "w") as archive:
        link = zipfile.ZipInfo("full.md")
        link.create_system = 3
        link.external_attr = (stat.S_IFLNK | 0o777) << 16
        archive.writestr(link, "target")
        archive.writestr("fixture_content_list.json", "[]")
        archive.writestr("fixture_middle.json", "{}")
        archive.writestr("fixture_model.json", "{}")
    with pytest.raises(ArchiveValidationError) as symlink_error:
        validate_result_archive(symlink_output.getvalue())
    assert symlink_error.value.failure_class == "symlink_entry"

    duplicate_output = io.BytesIO()
    with zipfile.ZipFile(duplicate_output, "w") as archive:
        archive.writestr("full.md", "one")
        with pytest.warns(UserWarning, match="Duplicate name"):
            archive.writestr("full.md", "two")
        archive.writestr("fixture_content_list.json", "[]")
        archive.writestr("fixture_middle.json", "{}")
        archive.writestr("fixture_model.json", "{}")
    with pytest.raises(ArchiveValidationError) as duplicate_error:
        validate_result_archive(duplicate_output.getvalue())
    assert duplicate_error.value.failure_class == "duplicate_entry"

    bomb_output = io.BytesIO()
    with zipfile.ZipFile(
        bomb_output,
        "w",
        compression=zipfile.ZIP_DEFLATED,
    ) as archive:
        archive.writestr("full.md", b"a" * 1_000_000)
        archive.writestr("fixture_content_list.json", "[]")
        archive.writestr("fixture_middle.json", "{}")
        archive.writestr("fixture_model.json", "{}")
    with pytest.raises(ArchiveValidationError) as compression_error:
        validate_result_archive(bomb_output.getvalue())
    assert compression_error.value.failure_class == "compression_ratio_exceeded"


@pytest.mark.parametrize(
    ("missing_name", "expected_class"),
    [
        ("full.md", "missing_full_markdown"),
        ("fixture_content_list.json", "missing_content_list"),
        ("fixture_middle.json", "missing_middle_json"),
        ("fixture_model.json", "missing_model_json"),
    ],
)
def test_result_archive_classifies_missing_required_artifact(
    missing_name: str,
    expected_class: str,
) -> None:
    entries = {
        "full.md": b"x",
        "fixture_content_list.json": b"[]",
        "fixture_middle.json": b"{}",
        "fixture_model.json": b"{}",
    }
    entries.pop(missing_name)

    with pytest.raises(ArchiveValidationError) as captured:
        validate_result_archive(_archive(entries))
    assert captured.value.failure_class == expected_class


@pytest.mark.parametrize(
    ("content", "expected_class"),
    [
        (b"", "empty_archive"),
        (b"not a zip", "invalid_zip"),
    ],
)
def test_result_archive_classifies_empty_and_invalid_zip(
    content: bytes,
    expected_class: str,
) -> None:
    with pytest.raises(ArchiveValidationError) as captured:
        validate_result_archive(content)
    assert captured.value.failure_class == expected_class


def test_archive_failure_class_rejects_open_fallback() -> None:
    with pytest.raises(CaptureError, match="CAPTURE_FAILED"):
        ArchiveValidationError("dynamic_archive_detail")


def test_result_archive_directories_cannot_impersonate_required_files() -> None:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        for name in (
            "full.md/",
            "fixture_content_list.json/",
            "fixture_middle.json/",
            "fixture_model.json/",
        ):
            archive.mkdir(name)

    with pytest.raises(ArchiveValidationError) as captured:
        validate_result_archive(output.getvalue())
    assert captured.value.failure_class == "missing_full_markdown"


def test_result_archive_classifies_unsupported_compression(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def unsupported(_: zipfile.ZipFile) -> str | None:
        raise NotImplementedError

    monkeypatch.setattr(zipfile.ZipFile, "testzip", unsupported)
    content = _archive(
        {
            "full.md": b"x",
            "fixture_content_list.json": b"[]",
            "fixture_middle.json": b"{}",
            "fixture_model.json": b"{}",
        }
    )

    with pytest.raises(ArchiveValidationError) as captured:
        validate_result_archive(content)
    assert captured.value.failure_class == "unsupported_compression"
