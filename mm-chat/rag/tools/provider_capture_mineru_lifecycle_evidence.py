"""Closed redacted Evidence v2 validator for MinerU lifecycle capture."""

from __future__ import annotations

from typing import cast

from tools.provider_capture_common import (
    HTTP_OK,
    SYNTHETIC_PDF_NAME,
    CaptureError,
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    format_observed_at,
    is_nonnegative_int,
    is_positive_int,
    is_sha256,
    json_list,
    json_object,
    parse_observed_at,
    require_exact_keys,
)
from tools.provider_capture_mineru_archive import (
    MAX_ARCHIVE_BYTES,
    MAX_ARCHIVE_ENTRIES,
)
from tools.provider_capture_mineru_lifecycle_http import (
    DOWNLOAD_FAILURE_CLASSES,
    LIFECYCLE_SCHEMA_VERSION,
    POLL_CALL_LIMIT,
    TRANSPORT_FAILURE_CLASSES,
)

_OUTCOME_STATES = {
    "allocate_failed": (
        "allocate_failed",
        "not_attempted",
        "not_attempted",
        "not_attempted",
    ),
    "unknown_submission": (
        "unknown_submission",
        "not_attempted",
        "not_attempted",
        "not_attempted",
    ),
    "upload_target_rejected": (
        "success",
        "target_rejected",
        "not_attempted",
        "not_attempted",
    ),
    "unknown_upload": ("success", "unknown", "not_attempted", "not_attempted"),
    "upload_failed": ("success", "failed", "not_attempted", "not_attempted"),
    "unknown_poll": ("success", "success", "unknown_poll", "not_attempted"),
    "poll_failed": ("success", "success", "poll_failed", "not_attempted"),
    "poll_exhausted": ("success", "success", "poll_exhausted", "not_attempted"),
    "parse_failed": ("success", "success", "parse_failed", "not_attempted"),
    "download_target_rejected": ("success", "success", "done", "target_rejected"),
    "unknown_download": ("success", "success", "done", "unknown"),
    "download_failed": ("success", "success", "done", "failed"),
    "lifecycle_complete": ("success", "success", "done", "success"),
}
_POLL_STATES = {
    "waiting-file",
    "pending",
    "running",
    "converting",
    "done",
    "failed",
}
_OPERATION_COUNT = 4
_ARCHIVE_CONTENT_TYPES = {
    "application/octet-stream",
    "application/x-zip-compressed",
    "application/zip",
    "binary/octet-stream",
}


def validate_lifecycle_evidence_snapshot(snapshot: JsonObject) -> None:
    """Reject any open, inconsistent, dynamic, or secret-bearing v2 evidence."""
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
    outcome = snapshot["captureOutcome"]
    observed_at = snapshot["observedAt"]
    if (
        snapshot["schemaVersion"] != LIFECYCLE_SCHEMA_VERSION
        or snapshot["captureMode"] != "authorized_execute"
        or not isinstance(outcome, str)
        or outcome not in _OUTCOME_STATES
        or not isinstance(observed_at, str)
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if format_observed_at(parse_observed_at(observed_at)) != observed_at:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")

    providers = json_list(snapshot["providers"], "EVIDENCE_SCHEMA_INVALID")
    budgets = json_object(snapshot["budgets"], "EVIDENCE_SCHEMA_INVALID")
    artifacts = json_list(snapshot["syntheticArtifacts"], "EVIDENCE_SCHEMA_INVALID")
    if len(providers) != 1 or set(budgets) != {"mineru"} or len(artifacts) != 1:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    provider = json_object(providers[0], "EVIDENCE_SCHEMA_INVALID")
    budget = json_object(budgets["mineru"], "EVIDENCE_SCHEMA_INVALID")
    _validate_provider(provider, outcome, budget)
    _validate_budget(budget)
    _validate_artifact(artifacts[0], provider)
    _reject_dynamic_strings(snapshot)
    canonical_json_bytes(snapshot)


def _validate_provider(
    provider: JsonObject,
    outcome: str,
    budget: JsonObject,
) -> None:
    require_exact_keys(
        provider,
        {
            "operationCount",
            "operations",
            "provider",
            "state",
            "syntheticPdfByteCount",
            "syntheticPdfSha256",
        },
    )
    operations = json_list(provider["operations"], "EVIDENCE_SCHEMA_INVALID")
    if (
        provider["provider"] != "mineru"
        or provider["state"] != outcome
        or provider["operationCount"] != _OPERATION_COUNT
        or len(operations) != _OPERATION_COUNT
        or not is_positive_int(provider["syntheticPdfByteCount"])
        or not is_sha256(provider["syntheticPdfSha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    parsed = [json_object(item, "EVIDENCE_SCHEMA_INVALID") for item in operations]
    if [item.get("operation") for item in parsed] != [
        "allocate_upload",
        "upload",
        "poll_batch",
        "download_result",
    ]:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    expected_states = _OUTCOME_STATES[outcome]
    if tuple(item.get("state") for item in parsed) != expected_states:
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    _validate_allocate(parsed[0])
    _validate_upload(parsed[1])
    _validate_poll(parsed[2])
    _validate_download(parsed[3])
    if (
        budget.get("usedAllocateCalls") != 1
        or budget.get("usedUploadCalls") != _used_upload(parsed[1])
        or budget.get("usedPollCalls") != parsed[2].get("usedCalls")
        or budget.get("usedDownloadCalls") != parsed[3].get("usedCalls")
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_allocate(operation: JsonObject) -> None:
    base = {"method", "operation", "path", "requestBodySha256", "state"}
    if operation["state"] == "success":
        expected = base | {
            "httpStatus",
            "response",
            "responseContentType",
            "responseHeaderNames",
        }
    else:
        expected = base
    require_exact_keys(operation, expected)
    if (
        operation["method"] != "POST"
        or operation["path"] != "/api/v4/file-urls/batch"
        or not is_sha256(operation["requestBodySha256"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if operation["state"] == "success":
        _validate_json_metadata(operation)
        response = json_object(operation["response"], "EVIDENCE_SCHEMA_INVALID")
        require_exact_keys(response, {"batchIdPresent", "signedUploadUrlCount"})
        if response != {"batchIdPresent": True, "signedUploadUrlCount": 1}:
            raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_upload(operation: JsonObject) -> None:
    base = {
        "method",
        "operation",
        "requestBodySha256",
        "requestByteCount",
        "state",
        "targetKind",
    }
    expected = base
    if operation["state"] == "success":
        expected |= {"httpStatus", "responseBodyByteCount"}
    elif operation["state"] == "unknown" and "transportFailureClass" in operation:
        expected |= {"transportFailureClass"}
    require_exact_keys(operation, expected)
    if (
        operation["method"] != "PUT"
        or operation["targetKind"] != "provider_signed_upload_url"
        or not is_sha256(operation["requestBodySha256"])
        or not is_positive_int(operation["requestByteCount"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if operation["state"] == "success" and (
        operation["httpStatus"] != HTTP_OK
        or not is_nonnegative_int(operation["responseBodyByteCount"])
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    _validate_transport_failure_class(operation)


def _validate_poll(operation: JsonObject) -> None:
    base = {
        "allowedCalls",
        "method",
        "operation",
        "path",
        "state",
        "stateCounts",
        "usedCalls",
    }
    expected = base if operation["state"] == "not_attempted" else base | {"response"}
    if operation["state"] == "unknown_poll" and "transportFailureClass" in operation:
        expected |= {"transportFailureClass"}
    require_exact_keys(operation, expected)
    used = operation["usedCalls"]
    counts = json_object(operation["stateCounts"], "EVIDENCE_SCHEMA_INVALID")
    if (
        operation["method"] != "GET"
        or operation["path"] != "/api/v4/extract-results/batch/{batch_id}"
        or operation["allowedCalls"] != POLL_CALL_LIMIT
        or not is_nonnegative_int(used)
        or cast("int", used) > POLL_CALL_LIMIT
        or set(counts) != _POLL_STATES
        or any(not is_nonnegative_int(value) for value in counts.values())
        or sum(cast("dict[str, int]", counts).values()) > cast("int", used)
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if operation["state"] == "not_attempted":
        if used != 0 or any(counts.values()):
            raise CaptureError("EVIDENCE_SCHEMA_INVALID")
        return
    _validate_poll_response(operation, counts, cast("int", used))
    _validate_transport_failure_class(operation)


def _validate_poll_response(
    operation: JsonObject,
    counts: JsonObject,
    used: int,
) -> None:
    response = json_object(operation["response"], "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(
        response,
        {
            "finalState",
            "identityMatchedResponseCount",
            "resultUrlPresent",
        },
    )
    matched_count = response["identityMatchedResponseCount"]
    if (
        not is_nonnegative_int(matched_count)
        or matched_count != sum(cast("dict[str, int]", counts).values())
        or response["finalState"] != operation["state"]
        or response["resultUrlPresent"] is not (operation["state"] == "done")
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    _validate_poll_transition(operation["state"], cast("int", matched_count), used)


def _validate_poll_transition(state: JsonValue, matched: int, used: int) -> None:
    if (
        (state == "poll_exhausted" and used != POLL_CALL_LIMIT)
        or (state in {"done", "parse_failed", "poll_exhausted"} and matched != used)
        or (state in {"unknown_poll", "poll_failed"} and matched + 1 != used)
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_download(operation: JsonObject) -> None:
    base = {
        "allowedCalls",
        "method",
        "operation",
        "state",
        "targetKind",
        "usedCalls",
    }
    expected = base
    if operation["state"] == "success":
        expected |= {"httpStatus", "response", "responseContentType"}
    elif operation["state"] == "unknown" and "transportFailureClass" in operation:
        expected |= {"transportFailureClass"}
    elif operation["state"] == "failed" and "downloadFailureClass" in operation:
        expected |= {"downloadFailureClass"}
    require_exact_keys(operation, expected)
    if (
        operation["method"] != "GET"
        or operation["targetKind"] != "provider_result_url"
        or operation["allowedCalls"] != 1
        or operation["usedCalls"] not in {0, 1}
        or (
            operation["state"] in {"not_attempted", "target_rejected"}
            and operation["usedCalls"] != 0
        )
        or (
            operation["state"] in {"success", "unknown", "failed"}
            and operation["usedCalls"] != 1
        )
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    if operation["state"] == "success":
        _validate_download_success(operation)
    _validate_transport_failure_class(operation)
    _validate_download_failure_class(operation)


def _validate_transport_failure_class(operation: JsonObject) -> None:
    if "transportFailureClass" not in operation:
        return
    failure_class = operation["transportFailureClass"]
    if (
        operation["state"] not in {"unknown", "unknown_poll"}
        or not isinstance(failure_class, str)
        or failure_class not in TRANSPORT_FAILURE_CLASSES
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_download_failure_class(operation: JsonObject) -> None:
    if "downloadFailureClass" not in operation:
        return
    failure_class = operation["downloadFailureClass"]
    if (
        operation["state"] != "failed"
        or not isinstance(failure_class, str)
        or failure_class not in DOWNLOAD_FAILURE_CLASSES
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_download_success(operation: JsonObject) -> None:
    if (
        operation["httpStatus"] != HTTP_OK
        or operation["responseContentType"] not in _ARCHIVE_CONTENT_TYPES
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")
    response = json_object(operation["response"], "EVIDENCE_SCHEMA_INVALID")
    presence_fields = {
        "contentListPresent",
        "fullMarkdownPresent",
        "middleJsonPresent",
        "modelJsonPresent",
    }
    require_exact_keys(
        response,
        {"archiveByteCount", "archiveSha256", "entryCount"} | presence_fields,
    )
    if (
        not is_positive_int(response["archiveByteCount"])
        or cast("int", response["archiveByteCount"]) > MAX_ARCHIVE_BYTES
        or not is_sha256(response["archiveSha256"])
        or not is_positive_int(response["entryCount"])
        or cast("int", response["entryCount"]) > MAX_ARCHIVE_ENTRIES
        or any(response[name] is not True for name in presence_fields)
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_budget(budget: JsonObject) -> None:
    require_exact_keys(
        budget,
        {
            "allowedAllocateCalls",
            "allowedDownloadCalls",
            "allowedPollCalls",
            "allowedUploadCalls",
            "usedAllocateCalls",
            "usedDownloadCalls",
            "usedPollCalls",
            "usedUploadCalls",
        },
    )
    if (
        budget["allowedAllocateCalls"] != 1
        or budget["allowedUploadCalls"] != 1
        or budget["allowedPollCalls"] != POLL_CALL_LIMIT
        or budget["allowedDownloadCalls"] != 1
        or any(
            not is_nonnegative_int(budget[name])
            for name in (
                "usedAllocateCalls",
                "usedUploadCalls",
                "usedPollCalls",
                "usedDownloadCalls",
            )
        )
        or cast("int", budget["usedAllocateCalls"]) > 1
        or cast("int", budget["usedUploadCalls"]) > 1
        or cast("int", budget["usedPollCalls"]) > POLL_CALL_LIMIT
        or cast("int", budget["usedDownloadCalls"]) > 1
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_artifact(raw: JsonValue, provider: JsonObject) -> None:
    artifact = json_object(raw, "EVIDENCE_SCHEMA_INVALID")
    require_exact_keys(artifact, {"byteCount", "kind", "sha256"})
    if (
        artifact["kind"] != "deterministic_synthetic_pdf"
        or artifact["byteCount"] != provider["syntheticPdfByteCount"]
        or artifact["sha256"] != provider["syntheticPdfSha256"]
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _validate_json_metadata(operation: JsonObject) -> None:
    if (
        operation["httpStatus"] != HTTP_OK
        or operation["responseContentType"] != "application/json"
        or operation["responseHeaderNames"] != ["content-type"]
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


def _used_upload(operation: JsonObject) -> int:
    return 1 if operation["state"] in {"success", "unknown", "failed"} else 0


def _reject_dynamic_strings(value: object) -> None:
    if isinstance(value, dict):
        for item in value.values():
            _reject_dynamic_strings(item)
    elif isinstance(value, list):
        for item in value:
            _reject_dynamic_strings(item)
    elif isinstance(value, str) and (
        "://" in value
        or SYNTHETIC_PDF_NAME in value
        or "sensitive" in value.lower()
        or "bearer " in value.lower()
    ):
        raise CaptureError("EVIDENCE_SCHEMA_INVALID")


__all__ = ["validate_lifecycle_evidence_snapshot"]
