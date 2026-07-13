"""Closed evidence schema and parent-FD-based private output writer."""

from __future__ import annotations

import os
from contextlib import suppress
from pathlib import Path
from typing import cast

from tools.provider_capture_common import (
    HTTP_OK,
    JINA_CALL_COUNT,
    JINA_MODEL,
    JINA_RERANK_MODEL,
    OUTPUT_DIR_RE,
    OUTPUT_FILE,
    RERANK_DOCUMENT_COUNT,
    CaptureError,
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    finite_number,
    format_observed_at,
    is_nonnegative_int,
    is_positive_int,
    is_sha256,
    json_list,
    json_object,
    parse_observed_at,
    require_exact_keys,
)


def validate_evidence_snapshot(snapshot: JsonObject) -> None:
    """Validate the exact closed v1 shape before any evidence write."""
    require_exact_keys(
        snapshot,
        {
            "budgets",
            "captureMode",
            "captureOutcome",
            "observedAt",
            "providers",
            "schemaVersion",
            "syntheticArtifacts",
        },
    )
    _validate_evidence_identity(snapshot)
    providers = json_list(snapshot["providers"], "EVIDENCE_SCHEMA_INVALID")
    budgets = json_object(snapshot["budgets"], "EVIDENCE_SCHEMA_INVALID")
    artifacts = json_list(snapshot["syntheticArtifacts"], "EVIDENCE_SCHEMA_INVALID")
    seen = _validate_evidence_providers(providers)
    if set(budgets) != seen:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    _validate_capture_outcome(snapshot["captureOutcome"], providers)
    _validate_evidence_budgets(budgets)
    _validate_evidence_artifacts(artifacts, has_mineru="mineru" in seen)
    canonical_json_bytes(snapshot)


def write_evidence_snapshot(output_dir: Path, snapshot: JsonObject) -> Path:
    """Write below the existing parent without following any path symlink."""
    return write_evidence_snapshot_at(
        output_dir.parent,
        output_dir.name,
        snapshot,
    )


def validate_output_destination(output_base: Path | None, output_name: str) -> None:
    """Passively verify one direct-child destination before Provider egress."""
    _validate_output_name(output_name)
    parent_fd = _open_directory_fd(output_base)
    try:
        try:
            os.stat(output_name, dir_fd=parent_fd, follow_symlinks=False)
        except FileNotFoundError:
            return
        _raise_target_exists()
    except CaptureError:
        raise
    except OSError as error:
        raise CaptureError("OUTPUT_PARENT_INVALID") from error
    finally:
        os.close(parent_fd)


def write_evidence_snapshot_at(
    output_base: Path | None,
    output_name: str,
    snapshot: JsonObject,
) -> Path:
    """Create one direct-child directory using only parent-relative syscalls."""
    validate_evidence_snapshot(snapshot)
    _validate_output_name(output_name)
    snapshot_bytes = canonical_json_bytes(snapshot)
    parent_fd = _open_directory_fd(output_base)
    directory_fd: int | None = None
    created_directory = False
    completed = False
    try:
        os.mkdir(output_name, mode=0o700, dir_fd=parent_fd)
        created_directory = True
        os.fsync(parent_fd)
        directory_fd = os.open(output_name, _directory_flags(), dir_fd=parent_fd)
        os.fchmod(directory_fd, 0o700)
        _write_new_evidence_file(directory_fd, snapshot_bytes)
        os.fsync(directory_fd)
        os.fsync(parent_fd)
        completed = True
    except FileExistsError as error:
        raise CaptureError("EVIDENCE_TARGET_EXISTS") from error
    except CaptureError:
        raise
    except OSError as error:
        raise CaptureError("EVIDENCE_WRITE_FAILED") from error
    finally:
        if directory_fd is not None:
            os.close(directory_fd)
        if created_directory and not completed:
            with suppress(OSError):
                os.rmdir(output_name, dir_fd=parent_fd)
                os.fsync(parent_fd)
        os.close(parent_fd)
    base = Path.cwd() if output_base is None else output_base
    return base / output_name / OUTPUT_FILE


def _write_new_evidence_file(directory_fd: int, content: bytes) -> None:
    temporary_name = f".{OUTPUT_FILE}.{os.getpid()}.tmp"
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC
    temporary_identity: tuple[int, int] | None = None
    target_linked = False
    try:
        file_fd = os.open(temporary_name, flags, 0o600, dir_fd=directory_fd)
        temporary_identity = _write_and_sync(file_fd, content)
        os.link(
            temporary_name,
            OUTPUT_FILE,
            src_dir_fd=directory_fd,
            dst_dir_fd=directory_fd,
            follow_symlinks=False,
        )
        target_linked = True
        os.unlink(temporary_name, dir_fd=directory_fd)
    except BaseException:
        with suppress(FileNotFoundError):
            os.unlink(temporary_name, dir_fd=directory_fd)
        if target_linked and temporary_identity is not None:
            _unlink_owned_target(directory_fd, temporary_identity)
        raise


def _write_and_sync(file_fd: int, content: bytes) -> tuple[int, int]:
    try:
        os.fchmod(file_fd, 0o600)
        file_stat = os.fstat(file_fd)
        with os.fdopen(file_fd, "wb", closefd=True) as output:
            output.write(content)
            output.flush()
            os.fsync(output.fileno())
    except BaseException:
        with suppress(OSError):
            os.close(file_fd)
        raise
    return (file_stat.st_dev, file_stat.st_ino)


def _unlink_owned_target(directory_fd: int, identity: tuple[int, int]) -> None:
    with suppress(FileNotFoundError):
        target_stat = os.stat(OUTPUT_FILE, dir_fd=directory_fd, follow_symlinks=False)
        if (target_stat.st_dev, target_stat.st_ino) == identity:
            os.unlink(OUTPUT_FILE, dir_fd=directory_fd)


def _open_directory_fd(path: Path | None) -> int:
    if path is None:
        return os.open(".", _directory_flags())
    if ".." in path.parts:
        raise CaptureError("OUTPUT_PARENT_INVALID")
    descriptor = os.open("/" if path.is_absolute() else ".", _directory_flags())
    parts = path.parts[1:] if path.is_absolute() else path.parts
    try:
        for part in parts:
            if part in {"", "."}:
                continue
            child = os.open(part, _directory_flags(), dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
    except OSError as error:
        os.close(descriptor)
        raise CaptureError("OUTPUT_PARENT_INVALID") from error
    return descriptor


def _directory_flags() -> int:
    return os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC


def _raise_target_exists() -> None:
    raise CaptureError("EVIDENCE_TARGET_EXISTS")


def _validate_output_name(value: str) -> None:
    if OUTPUT_DIR_RE.fullmatch(value) is None:
        raise CaptureError("OUTPUT_DIRECTORY_INVALID")


def _validate_evidence_identity(snapshot: JsonObject) -> None:
    if (
        snapshot["schemaVersion"] != "mm-chat.provider-capture-evidence.v1"
        or snapshot["captureMode"] != "authorized_execute"
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    observed_at = snapshot["observedAt"]
    if not isinstance(observed_at, str):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if format_observed_at(parse_observed_at(observed_at)) != observed_at:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_evidence_providers(providers: list[JsonValue]) -> set[str]:
    seen: set[str] = set()
    for raw_provider in providers:
        provider = json_object(raw_provider, "EVIDENCE_SCHEMA_INVALID")
        provider_name = provider.get("provider")
        if provider_name == "jina":
            _validate_jina_evidence(provider)
        elif provider_name == "mineru":
            _validate_mineru_evidence(provider)
        else:
            raise CaptureError("EVIDENCE_SCHEMA_INVALID")
        if provider_name in seen:
            raise CaptureError("EVIDENCE_SCHEMA_INVALID")
        seen.add(cast("str", provider_name))
    if not seen:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    return seen


def _validate_capture_outcome(outcome: JsonValue, providers: list[JsonValue]) -> None:
    states = {
        json_object(provider, "EVIDENCE_SCHEMA_INVALID").get("state")
        for provider in providers
    }
    expected = (
        "unknown_submission"
        if "unknown_submission" in states
        else "fixed_plan_complete"
    )
    if outcome != expected:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_jina_evidence(provider: JsonObject) -> None:
    require_exact_keys(
        provider,
        {"operationCount", "operations", "provider", "state"},
    )
    operations = json_list(provider["operations"], "EVIDENCE_SCHEMA_INVALID")
    if (
        provider["operationCount"] != JINA_CALL_COUNT
        or provider["state"] != "captured"
        or len(operations) != JINA_CALL_COUNT
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    expected = (
        ("embedding_1024", "/v1/embeddings", 1024),
        ("embedding_2048", "/v1/embeddings", 2048),
        ("rerank", "/v1/rerank", None),
    )
    for raw, operation_spec in zip(operations, expected, strict=True):
        _validate_jina_operation(raw, *operation_spec)


def _validate_jina_operation(
    raw_operation: JsonValue,
    name: str,
    path: str,
    dimensions: int | None,
) -> None:
    operation = json_object(raw_operation, "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(
        operation,
        {
            "httpStatus",
            "method",
            "operation",
            "path",
            "requestBodySha256",
            "response",
            "responseContentType",
            "responseHeaderNames",
        },
    )
    if operation["operation"] != name or operation["path"] != path:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    _validate_common_evidence_operation(operation)
    response = json_object(operation["response"], "EVIDENCE_SCHEMA_INVALID")
    if dimensions is None:
        _validate_rerank_evidence_response(response)
    else:
        _validate_embedding_evidence_response(response, dimensions)


def _validate_embedding_evidence_response(
    response: JsonObject,
    dimensions: int,
) -> None:
    require_exact_keys(
        response,
        {"indexes", "itemCount", "model", "usage", "vectorDimension"},
    )
    usage = json_object(response["usage"], "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(usage, {"promptTokens", "totalTokens"})
    if (
        response["model"] != JINA_MODEL
        or response["itemCount"] != 1
        or response["vectorDimension"] != dimensions
        or response["indexes"] != [0]
        or not _optional_nonnegative_int(usage["promptTokens"])
        or not is_nonnegative_int(usage["totalTokens"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_rerank_evidence_response(response: JsonObject) -> None:
    require_exact_keys(
        response,
        {"indexes", "model", "resultCount", "scores", "usage"},
    )
    usage = json_object(response["usage"], "EVIDENCE_SCHEMA_INVALID")
    indexes = json_list(response["indexes"], "EVIDENCE_SCHEMA_INVALID")
    scores = json_list(response["scores"], "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(usage, {"totalTokens"})
    if (
        response["model"] != JINA_RERANK_MODEL
        or response["resultCount"] != RERANK_DOCUMENT_COUNT
        or set(cast("list[int]", indexes)) != {0, 1}
        or len(scores) != RERANK_DOCUMENT_COUNT
        or any(not finite_number(score) for score in scores)
        or not is_nonnegative_int(usage["totalTokens"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_mineru_evidence(provider: JsonObject) -> None:
    state = provider.get("state")
    expected_keys = _mineru_provider_keys(state)
    require_exact_keys(provider, expected_keys)
    operations = json_list(provider["operations"], "EVIDENCE_SCHEMA_INVALID")
    if provider["operationCount"] != 1 or len(operations) != 1:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    operation = json_object(operations[0], "EVIDENCE_SCHEMA_INVALID")
    _validate_mineru_operation(operation, cast("str", state))
    if state == "staged_after_submit":
        _validate_mineru_staged_response(provider, operation)


def _mineru_provider_keys(state: JsonValue) -> set[str]:
    common = {"operationCount", "operations", "provider", "state"}
    if state == "staged_after_submit":
        return common | {"syntheticPdfByteCount", "syntheticPdfSha256"}
    if state == "unknown_submission":
        return common
    raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_mineru_operation(operation: JsonObject, state: str) -> None:
    common = {"method", "operation", "path", "requestBodySha256", "state"}
    extras = (
        {
            "httpStatus",
            "response",
            "responseContentType",
            "responseHeaderNames",
        }
        if state == "staged_after_submit"
        else set()
    )
    require_exact_keys(operation, common | extras)
    if state == "staged_after_submit":
        _validate_common_evidence_operation(operation)
    if (
        operation["operation"] != "local_upload_batch_submit"
        or operation["method"] != "POST"
        or operation["path"] != "/api/v4/file-urls/batch"
        or operation["state"] != state
        or not is_sha256(operation["requestBodySha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_mineru_staged_response(
    provider: JsonObject,
    operation: JsonObject,
) -> None:
    response = json_object(operation["response"], "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(response, {"batchIdPresent", "signedUploadUrlCount"})
    if (
        response["batchIdPresent"] is not True
        or response["signedUploadUrlCount"] != 1
        or not is_positive_int(provider["syntheticPdfByteCount"])
        or not is_sha256(provider["syntheticPdfSha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_common_evidence_operation(operation: JsonObject) -> None:
    if (
        operation["method"] != "POST"
        or operation["httpStatus"] != HTTP_OK
        or operation["responseContentType"] != "application/json"
        or operation["responseHeaderNames"] != ["content-type"]
        or not is_sha256(operation["requestBodySha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_evidence_budgets(budgets: JsonObject) -> None:
    for provider, raw_budget in budgets.items():
        budget = json_object(raw_budget, "EVIDENCE_SCHEMA_INVALID")
        if provider == "jina":
            _validate_jina_budget(budget)
        elif provider == "mineru":
            _validate_mineru_budget(budget)
        else:
            raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_jina_budget(budget: JsonObject) -> None:
    require_exact_keys(budget, {"allowedCalls", "usedCalls"})
    if budget != {"allowedCalls": 3, "usedCalls": 3}:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_mineru_budget(budget: JsonObject) -> None:
    expected: JsonObject = {
        "allowedPollCalls": 0,
        "allowedSignedUploadCalls": 0,
        "allowedSubmitCalls": 1,
        "usedPollCalls": 0,
        "usedSignedUploadCalls": 0,
        "usedSubmitCalls": 1,
    }
    require_exact_keys(budget, set(expected))
    if budget != expected:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_evidence_artifacts(
    artifacts: list[JsonValue],
    *,
    has_mineru: bool,
) -> None:
    if len(artifacts) != int(has_mineru):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if not artifacts:
        return
    artifact = json_object(artifacts[0], "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(artifact, {"byteCount", "kind", "sha256"})
    if (
        artifact["kind"] != "deterministic_synthetic_pdf"
        or not is_positive_int(artifact["byteCount"])
        or not is_sha256(artifact["sha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _optional_nonnegative_int(value: object) -> bool:
    return value is None or is_nonnegative_int(value)
