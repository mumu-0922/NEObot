"""Strict, test-only loader for redacted provider wire contracts."""

from __future__ import annotations

import hashlib
import json
import math
import re
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from types import MappingProxyType
from typing import Any, Final, cast
from urllib.parse import unquote, urlsplit

import rfc8785
from jsonschema import Draft202012Validator, FormatChecker

_FIXTURE_ROOT: Final = Path(__file__).parents[1] / "fixtures" / "provider_contracts"
_SCHEMA_NAME: Final = "provider-contract-v1.schema.json"
_FIXTURE_NAME_RE: Final = re.compile(r"^[a-z][a-z0-9-]{2,95}\.json$")
_PLACEHOLDER_RE: Final = re.compile(
    r"^(?:default|model-v1|v1|tbd|todo|unknown|unverified|change-me|"
    r"64-lowercase-hex|<[^>]+>)$",
    re.IGNORECASE,
)
_SECRET_VALUE_RE: Final = re.compile(
    r"(?:\bBearer\s+[A-Za-z0-9._~+/=-]{8,}|AKIA[0-9A-Z]{16}|"
    r"\bsk-[A-Za-z0-9_-]{8,}|-----BEGIN(?: [A-Z]+)? PRIVATE KEY-----)",
)
_HASH_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_SECRET_FIELD_NAMES: Final = frozenset(
    {
        "api_key",
        "apikey",
        "authorization",
        "cookie",
        "password",
        "secret",
        "set-cookie",
        "token",
    }
)
_NORMALIZED_SECRET_FIELD_NAMES: Final = frozenset(
    re.sub(r"[^a-z0-9]", "", name.lower()) for name in _SECRET_FIELD_NAMES
)
_PLACEHOLDER_FIELD_NAMES: Final = frozenset(
    {"apiversion", "endpoint", "endpointid", "model", "modelid", "modelversion"}
)
_MAX_SAFE_INTEGER: Final = (1 << 53) - 1
_MIME_RE: Final = re.compile(
    r"^[a-z0-9][a-z0-9!#$&^_.+-]{0,126}/"
    r"(?:[a-z0-9][a-z0-9!#$&^_.+-]{0,126}|\*)$"
)
_URL_TOKEN_RE: Final = re.compile(r"(?i)https?://|(?<!:)//(?=[^\s/?#])")
_MINERU_START_TIME_RE: Final = re.compile(r"^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$")
_MINERU_PHASES: Final = frozenset(
    {"submit", "poll", "result", "cancel", "query_by_key"}
)


class ContractValidationError(ValueError):
    """A fixture is malformed, unsafe, or not eligible for promotion."""


@dataclass(frozen=True, slots=True)
class ResponseCase:
    """One deterministic response exposed by the in-memory fake provider."""

    case_id: str
    status: int
    headers: Mapping[str, str]
    body: Mapping[str, Any]
    classification: str
    stable_error_code: str | None


@dataclass(frozen=True, slots=True)
class Operation:
    """One observed or explicitly unresolved provider operation."""

    operation_id: str
    phase: str
    support_state: str
    method: str | None
    path_template: str | None
    request: Mapping[str, Any] | None
    response_cases: tuple[ResponseCase, ...]


@dataclass(frozen=True, slots=True)
class ProviderContract:
    """Validated fixture envelope; it is not production Provider authority."""

    fixture_name: str
    fixture_set_id: str
    fixture_kind: str
    provider_kind: str
    lifecycle_state: str
    raw: Mapping[str, Any]
    operations: tuple[Operation, ...]

    def operation(self, operation_id: str) -> Operation:
        """Return one exact operation without accepting caller-controlled paths."""
        for operation in self.operations:
            if operation.operation_id == operation_id:
                return operation
        raise KeyError(operation_id)

    def mutable_copy(self) -> dict[str, Any]:
        """Return a detached mutable copy for negative tests and review tooling."""
        return cast("dict[str, Any]", _thaw(self.raw))

    def wire_contract_hash(self) -> str:
        """Hash identity, capability, operations, and redaction wire semantics."""
        boundary = {
            "schemaVersion": self.raw["schemaVersion"],
            "providerKind": self.raw["providerKind"],
            "observedAt": self.raw["observedAt"],
            "identity": self.raw["identity"],
            "capabilities": self.raw["capabilities"],
            "operations": self.raw["operations"],
            "redactionPolicy": self.raw["redactionPolicy"],
        }
        return _canonical_hash(boundary)

    def terms_snapshot_hash(self) -> str:
        """Hash reviewed governance terms and their exact evidence metadata."""
        evidence_refs: set[str] = set()
        for fact in _mapping(self.raw["governanceTerms"], "governanceTerms").values():
            fact_mapping = _mapping(fact, "governance term")
            if "evidenceRefs" in fact_mapping:
                evidence_refs.update(
                    _string_list(fact_mapping["evidenceRefs"], "evidenceRefs")
                )
        boundary = {
            "schemaVersion": self.raw["schemaVersion"],
            "providerKind": self.raw["providerKind"],
            "observedAt": self.raw["observedAt"],
            "governanceTerms": self.raw["governanceTerms"],
            "evidence": [
                evidence
                for evidence in _mapping_list(self.raw["evidence"], "evidence")
                if evidence["evidenceId"] in evidence_refs
            ],
        }
        return _canonical_hash(boundary)

    def fixture_set_hash(self) -> str:
        """Hash fixture identity, exchanges, evidence, and redaction policy."""
        boundary = {
            "schemaVersion": self.raw["schemaVersion"],
            "fixtureSetId": self.raw["fixtureSetId"],
            "fixtureKind": self.raw["fixtureKind"],
            "observedAt": self.raw["observedAt"],
            "operations": self.raw["operations"],
            "redactionPolicy": self.raw["redactionPolicy"],
            "evidence": self.raw["evidence"],
        }
        return _canonical_hash(boundary)

    def require_frozen(
        self,
        freeze_report: bytes,
        evidence_snapshots: Mapping[str, bytes],
        *,
        now: datetime | None = None,
    ) -> None:
        """Fail closed unless all wire, governance, review, and hash gates pass."""
        raw = self.mutable_copy()
        _validate_fixed_schema_and_semantics(raw)
        lifecycle = _mapping(self.raw["lifecycle"], "lifecycle")
        if self.fixture_kind == "synthetic_test":
            raise ContractValidationError("synthetic fixtures cannot be frozen")
        if self.lifecycle_state != "frozen":
            raise ContractValidationError("provider contract is not frozen")
        if lifecycle["blockedBy"] or self.raw["unresolved"]:
            raise ContractValidationError("frozen contract still has blockers")
        if not isinstance(lifecycle.get("frozenAt"), str):
            raise ContractValidationError("frozen contract lacks frozenAt")
        reviewers = set(_string_list(lifecycle["reviewers"], "reviewers"))
        if len(reviewers) < 2 or "governance_security" not in reviewers:
            raise ContractValidationError("frozen contract lacks independent review")
        _reject_unresolved_facts(self.raw["identity"])
        _reject_unresolved_facts(self.raw["capabilities"])
        _reject_unresolved_facts(self.raw["governanceTerms"])
        _validate_frozen_capability_states(self.raw)
        for fact in _mapping(self.raw["identity"], "identity").values():
            if _mapping(fact, "identity fact")["state"] not in {
                "observed",
                "terms_verified",
            }:
                raise ContractValidationError("frozen identity is incomplete")
        for fact in _mapping(self.raw["governanceTerms"], "terms").values():
            terms_fact = _mapping(fact, "terms fact")
            if terms_fact["state"] != "terms_verified":
                raise ContractValidationError("frozen terms lack review evidence")
            for evidence_ref in _string_list(
                terms_fact["evidenceRefs"], "terms evidence"
            ):
                evidence = _evidence_by_id(self.raw, evidence_ref)
                if evidence["sourceKind"] != "reviewed_terms":
                    raise ContractValidationError(
                        "terms evidence is not reviewed terms"
                    )
        for operation in self.operations:
            if operation.support_state == "unknown":
                raise ContractValidationError("frozen contract has unknown operations")
        integrity = _mapping(self.raw.get("integrity"), "integrity")
        wire_hash = _text(integrity.get("wireContractHash"), "wireContractHash")
        terms_hash = _text(integrity.get("termsSnapshotHash"), "termsSnapshotHash")
        fixture_hash = _text(integrity.get("fixtureSetHash"), "fixtureSetHash")
        freeze_hash = _text(integrity.get("freezeReportHash"), "freezeReportHash")
        if (
            wire_hash != self.wire_contract_hash()
            or terms_hash != self.terms_snapshot_hash()
            or fixture_hash != self.fixture_set_hash()
            or freeze_hash != hashlib.sha256(freeze_report).hexdigest()
        ):
            raise ContractValidationError("frozen contract integrity mismatch")
        _validate_evidence_freshness(
            self.raw,
            evidence_snapshots,
            now or datetime.now(tz=UTC),
        )


def load_provider_contract(fixture_name: str) -> ProviderContract:
    """Load only a checked-in basename from the fixed fixture directory."""
    if not _FIXTURE_NAME_RE.fullmatch(fixture_name) or fixture_name == _SCHEMA_NAME:
        raise ContractValidationError("fixture name is not allowlisted")
    raw = _load_json(_FIXTURE_ROOT / fixture_name)
    return validate_provider_contract(raw, fixture_name=fixture_name)


def parse_provider_contract(
    content: str, *, fixture_name: str = "in-memory.json"
) -> ProviderContract:
    """Parse strict JSON bytes without granting filesystem path access."""
    raw = _parse_json(content)
    return validate_provider_contract(raw, fixture_name=fixture_name)


def validate_provider_contract(
    raw: dict[str, Any],
    *,
    fixture_name: str = "in-memory.json",
) -> ProviderContract:
    """Validate an in-memory fixture against the checked-in immutable schema."""
    _validate_fixed_schema_and_semantics(raw)
    return _to_contract(fixture_name, raw)


def _validate_fixed_schema_and_semantics(raw: dict[str, Any]) -> None:
    """Re-run the sole authoritative schema and every semantic validator."""
    schema = _load_json(_FIXTURE_ROOT / _SCHEMA_NAME)
    Draft202012Validator.check_schema(schema)
    errors = sorted(
        Draft202012Validator(schema, format_checker=FormatChecker()).iter_errors(raw),
        key=lambda error: tuple(str(item) for item in error.absolute_path),
    )
    if errors:
        path = ".".join(str(item) for item in errors[0].absolute_path) or "$"
        raise ContractValidationError(f"fixture violates schema at {path}")
    _validate_json_tree(raw)
    _canonical_hash(raw)
    _validate_semantics(raw)


def _load_json(path: Path) -> dict[str, Any]:
    try:
        content = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        raise ContractValidationError("fixture cannot be read as UTF-8") from error
    if "\x00" in content:
        raise ContractValidationError("fixture contains a NUL byte")
    return _parse_json(content)


def _parse_json(content: str) -> dict[str, Any]:
    if "\x00" in content:
        raise ContractValidationError("fixture contains a NUL byte")
    try:
        parsed = json.loads(
            content,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_json_constant,
            parse_float=_parse_json_float,
            parse_int=_parse_json_int,
        )
    except (json.JSONDecodeError, ContractValidationError) as error:
        raise ContractValidationError("fixture is not strict JSON") from error
    return cast("dict[str, Any]", _mapping(parsed, "fixture"))


def _unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ContractValidationError("fixture contains a duplicate JSON key")
        result[key] = value
    return result


def _reject_json_constant(value: str) -> None:
    raise ContractValidationError(f"non-finite JSON constant is forbidden: {value}")


def _parse_json_float(value: str) -> float:
    parsed = float(value)
    if not math.isfinite(parsed):
        raise ContractValidationError("non-finite JSON number is forbidden")
    return parsed


def _parse_json_int(value: str) -> int:
    parsed = int(value)
    if abs(parsed) > _MAX_SAFE_INTEGER:
        raise ContractValidationError("JSON integer exceeds the JCS safe range")
    return parsed


def _validate_semantics(raw: dict[str, Any]) -> None:  # noqa: PLR0915
    provider_kind = _text(raw["providerKind"], "providerKind")
    evidence = cast("list[dict[str, Any]]", raw["evidence"])
    evidence_ids = _unique_ids(evidence, "evidenceId", "evidence")
    for item in evidence:
        _validate_https_url(_text(item["sourceUrl"], "sourceUrl"), "evidence URL")
        _parse_timestamp(_text(item["observedAt"], "evidence observedAt"))
    _parse_timestamp(_text(raw["observedAt"], "observedAt"))

    for section_name in ("identity", "capabilities", "governanceTerms"):
        section = _mapping(raw[section_name], section_name)
        for fact_name, fact_value in section.items():
            fact = _mapping(fact_value, f"{section_name}.{fact_name}")
            if fact["state"] in {"observed", "terms_verified"}:
                refs = set(_string_list(fact["evidenceRefs"], "evidenceRefs"))
                if not refs or not refs <= evidence_ids:
                    raise ContractValidationError("fact references unknown evidence")
                _reject_placeholder_fact(fact)
                _validate_fact_value(
                    provider_kind,
                    section_name,
                    str(fact_name),
                    fact["value"],
                )
            if fact["state"] == "terms_verified":
                if section_name != "governanceTerms":
                    raise ContractValidationError(
                        "terms_verified is reserved for governance terms"
                    )
                _parse_timestamp(_text(fact["reviewedAt"], "reviewedAt"))
            if section_name == "governanceTerms" and fact["state"] == "observed":
                raise ContractValidationError(
                    "governance terms require reviewed terms evidence"
                )
            if fact["state"] == "not_applicable":
                _validate_not_applicable_fact(
                    provider_kind, section_name, str(fact_name)
                )

    identity = _mapping(raw["identity"], "identity")
    base_url = _observed_text(identity["baseUrl"])
    if base_url is not None:
        _validate_https_url(base_url, "provider base URL")

    operations = cast("list[dict[str, Any]]", raw["operations"])
    _unique_ids(operations, "operationId", "operation")
    case_ids: set[str] = set()
    phases: set[str] = set()
    maximum_recorded_bytes = cast(
        "int",
        _mapping(raw["redactionPolicy"], "redactionPolicy")["maximumRecordedBodyBytes"],
    )
    forbidden_headers = {
        name.lower()
        for name in _string_list(
            _mapping(raw["redactionPolicy"], "redactionPolicy")[
                "forbiddenResponseHeaderNames"
            ],
            "forbiddenResponseHeaderNames",
        )
    }
    for operation in operations:
        support = _mapping(operation["support"], "support")
        support_state = _text(support["state"], "support.state")
        operation_id = _text(operation["operationId"], "operationId")
        phase = _text(operation["phase"], "phase")
        if phase in phases:
            raise ContractValidationError("operation phases must be unique")
        phases.add(phase)
        if operation_id != phase:
            raise ContractValidationError("operation ID must equal its phase")
        if support_state == "observed":
            refs = set(_string_list(support.get("evidenceRefs"), "support evidence"))
            if not refs or not refs <= evidence_ids:
                raise ContractValidationError("observed operation lacks evidence")
            for field in ("method", "pathTemplate", "request"):
                if field not in operation:
                    raise ContractValidationError("observed operation is incomplete")
            _validate_path(_text(operation["pathTemplate"], "pathTemplate"))
            request = _mapping(operation["request"], "request")
            request_bytes = _body_bytes(_mapping(request["body"], "request body"))
            if request_bytes > cast("int", request["maximumBytes"]):
                raise ContractValidationError("fixture request exceeds its replay cap")
            if request_bytes > maximum_recorded_bytes:
                raise ContractValidationError(
                    "recorded request body exceeds redaction cap"
                )
        elif any(field in operation for field in ("method", "pathTemplate", "request")):
            raise ContractValidationError(
                "unknown operation contains invented wire fields"
            )
        for response in cast("list[dict[str, Any]]", operation["responseCases"]):
            case_id = _text(response["caseId"], "caseId")
            if case_id in case_ids:
                raise ContractValidationError(
                    "response case IDs must be globally unique"
                )
            case_ids.add(case_id)
            classification = _text(response["classification"], "classification")
            status = cast("int", response["status"])
            _validate_response_classification(status, classification)
            source = _text(response["source"], "response source")
            if source == "redacted_capture":
                if "evidenceRefs" not in response:
                    raise ContractValidationError(
                        "redacted capture response lacks evidence"
                    )
                capture_refs = set(
                    _string_list(response.get("evidenceRefs"), "capture evidence")
                )
                if not capture_refs or not capture_refs <= evidence_ids:
                    raise ContractValidationError(
                        "redacted capture response lacks evidence"
                    )
                for capture_ref in capture_refs:
                    if _evidence_by_id(raw, capture_ref)["sourceKind"] != (
                        "redacted_capture"
                    ):
                        raise ContractValidationError(
                            "response capture evidence has the wrong source kind"
                        )
            headers = _mapping(response["headers"], "response headers")
            if {name.lower() for name in headers} & forbidden_headers:
                raise ContractValidationError(
                    "fixture retains a forbidden response header"
                )
            if (
                response["classification"] != "success"
                and "stableErrorCode" not in response
            ):
                raise ContractValidationError(
                    "non-success case lacks a stable error code"
                )
            if _body_bytes(_mapping(response["body"], "response body")) > (
                maximum_recorded_bytes
            ):
                raise ContractValidationError(
                    "recorded response body exceeds redaction cap"
                )
        if support_state == "observed" and not any(
            response["classification"] == "success"
            for response in cast("list[dict[str, Any]]", operation["responseCases"])
        ):
            raise ContractValidationError("observed operation lacks a success case")
        if (
            _mapping(raw["lifecycle"], "lifecycle")["state"] == "frozen"
            and support_state == "observed"
        ):
            classes = {
                _text(response["classification"], "classification")
                for response in cast("list[dict[str, Any]]", operation["responseCases"])
                if response["source"] == "redacted_capture"
            }
            if not {"success", "retryable", "permanent"} <= classes:
                raise ContractValidationError(
                    "frozen operation lacks success/retryable/permanent coverage"
                )

    if raw["providerKind"] == "mineru_async" and phases != _MINERU_PHASES:
        raise ContractValidationError("MinerU fixture must cover every recovery phase")
    if raw["providerKind"] == "jina_embedding" and phases != {"embed"}:
        raise ContractValidationError(
            "Jina embedding fixture has an invalid operation set"
        )
    if raw["providerKind"] == "jina_rerank" and phases != {"rerank"}:
        raise ContractValidationError(
            "Jina rerank fixture has an invalid operation set"
        )

    redaction = _mapping(raw["redactionPolicy"], "redactionPolicy")
    policy_secret_fields = {
        _normalized_key(name)
        for name in (
            *_string_list(redaction["secretHeaderNames"], "secretHeaderNames"),
            *_string_list(
                redaction["sensitiveQueryParameterNames"],
                "sensitiveQueryParameterNames",
            ),
            *_string_list(
                redaction["forbiddenResponseHeaderNames"],
                "forbiddenResponseHeaderNames",
            ),
        )
    }
    _scan_unsafe_values(raw, policy_secret_fields=policy_secret_fields)
    _validate_provider_shape(raw)


def _validate_response_classification(status: int, classification: str) -> None:
    if classification in {"success", "terminal_failure", "malformed_success"}:
        valid = 200 <= status < 300
    elif classification == "retryable":
        valid = status in {408, 425, 429} or 500 <= status < 600
    else:
        valid = 400 <= status < 500 and status not in {408, 425, 429}
    if not valid:
        raise ContractValidationError(
            "response classification contradicts its HTTP status"
        )


def _scan_unsafe_values(value: object, *, policy_secret_fields: set[str]) -> None:
    if isinstance(value, Mapping):
        for key, item in value.items():
            key_text = str(key)
            normalized = _normalized_key(key_text)
            if (
                _is_secret_field(normalized) or normalized in policy_secret_fields
            ) and item is not None:
                raise ContractValidationError("fixture contains a secret-like field")
            if (
                normalized in _PLACEHOLDER_FIELD_NAMES
                and isinstance(item, str)
                and _PLACEHOLDER_RE.fullmatch(item)
            ):
                raise ContractValidationError(
                    "wire field contains a placeholder literal"
                )
            _scan_unsafe_values(item, policy_secret_fields=policy_secret_fields)
        return
    if isinstance(value, (list, tuple)):
        for item in value:
            _scan_unsafe_values(item, policy_secret_fields=policy_secret_fields)
        return
    if not isinstance(value, str):
        return
    if _SECRET_VALUE_RE.search(value):
        raise ContractValidationError("fixture contains a secret-like value")
    stripped = value.strip()
    url_token = _URL_TOKEN_RE.search(value)
    if url_token is None:
        return
    if re.fullmatch(r"(?i)https?://\S+", stripped):
        if stripped != value:
            raise ContractValidationError("fixture URL contains surrounding whitespace")
        _validate_https_url(stripped, "fixture URL")
        return
    raise ContractValidationError(
        "embedded or scheme-relative fixture URL is forbidden"
    )


def _normalized_key(value: str) -> str:
    return re.sub(r"[^a-z0-9]", "", value.lower())


def _is_secret_field(normalized: str) -> bool:
    return (
        normalized in _NORMALIZED_SECRET_FIELD_NAMES
        or normalized.endswith(
            (
                "accesskey",
                "apikey",
                "authorization",
                "cookie",
                "password",
                "secret",
                "signature",
                "token",
            )
        )
        or normalized.startswith(("authorization", "signature"))
    )


def _validate_json_tree(value: object) -> None:
    if isinstance(value, bool) or value is None:
        return
    if isinstance(value, int):
        if abs(value) > _MAX_SAFE_INTEGER:
            raise ContractValidationError("JSON integer exceeds the JCS safe range")
        return
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ContractValidationError("non-finite JSON number is forbidden")
        return
    if isinstance(value, str):
        try:
            value.encode("utf-8")
        except UnicodeEncodeError as error:
            raise ContractValidationError(
                "JSON string contains a non-Unicode scalar"
            ) from error
        return
    if isinstance(value, Mapping):
        for key, item in value.items():
            if not isinstance(key, str):
                raise ContractValidationError("JSON object key must be text")
            _validate_json_tree(key)
            _validate_json_tree(item)
        return
    if isinstance(value, (list, tuple)):
        for item in value:
            _validate_json_tree(item)
        return
    raise ContractValidationError("fixture contains a non-JSON value")


def _reject_placeholder_fact(fact: Mapping[str, Any]) -> None:
    override = fact.get("literalOverride") is True
    for text in _walk_strings(fact["value"]):
        if _PLACEHOLDER_RE.fullmatch(text) and not override:
            raise ContractValidationError(
                "observed fact contains a placeholder literal"
            )


def _walk_strings(value: object) -> tuple[str, ...]:
    if isinstance(value, str):
        return (value,)
    if isinstance(value, Mapping):
        return tuple(text for item in value.values() for text in _walk_strings(item))
    if isinstance(value, (list, tuple)):
        return tuple(text for item in value for text in _walk_strings(item))
    return ()


def _validate_https_url(value: str, label: str) -> None:
    parsed = urlsplit(value)
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise ContractValidationError(f"{label} must be credential-free HTTPS")


def _validate_path(value: str) -> None:
    if (
        not value.startswith("/")
        or value.startswith("//")
        or "\\" in value
        or "%" in value
        or "?" in value
        or "#" in value
        or any(ord(character) < 0x20 for character in value)
    ):
        raise ContractValidationError("operation path is unsafe")
    decoded = unquote(value)
    if any(segment in {".", ".."} for segment in decoded.split("/")):
        raise ContractValidationError("operation path is unsafe")


def _validate_fact_value(
    provider_kind: str, section: str, field: str, value: object
) -> None:
    if section == "identity":
        if field == "allowedDataTypes":
            if (
                not isinstance(value, list)
                or not value
                or len(set(value)) != len(value)
                or any(
                    not isinstance(item, str) or not _MIME_RE.fullmatch(item)
                    for item in value
                )
            ):
                raise ContractValidationError(
                    "identity.allowedDataTypes must be a non-empty MIME list"
                )
            return
        text = _text(value, f"identity.{field}.value")
        if text != text.strip() or len(text.encode()) > 256:
            raise ContractValidationError(f"identity.{field}.value is invalid")
        return
    if section == "capabilities":
        _validate_capability_value(provider_kind, field, value)
        return
    if section == "governanceTerms":
        _validate_governance_term_value(field, value)


def _validate_capability_value(provider_kind: str, field: str, value: object) -> None:
    capability = _mapping(value, f"capabilities.{field}.value")
    if field == "idempotency":
        _require_closed_fields(
            capability,
            {"supported", "mechanism", "replayWindowSeconds"},
            "idempotency",
        )
        _boolean(capability["supported"], "idempotency.supported")
        _bounded_text(capability["mechanism"], "idempotency.mechanism")
        _nonnegative_int(
            capability["replayWindowSeconds"], "idempotency.replayWindowSeconds"
        )
        return
    if field == "rateLimits":
        mineru_fields = {
            "maximumFileBytes",
            "maximumPages",
            "maximumBatchItemsFromDetailedSection",
            "batchLimitConflictPresent",
        }
        jina_fields = {
            "requestsPerMinute",
            "tokensPerMinute",
            "concurrency",
            "scope",
        }
        keys = set(capability)
        if provider_kind == "mineru_async" and keys == mineru_fields:
            for name in mineru_fields - {"batchLimitConflictPresent"}:
                _positive_int(capability[name], f"rateLimits.{name}")
            _boolean(
                capability["batchLimitConflictPresent"],
                "rateLimits.batchLimitConflictPresent",
            )
            return
        if provider_kind in {"jina_embedding", "jina_rerank"} and keys == (jina_fields):
            for name in jina_fields - {"scope"}:
                _positive_int(capability[name], f"rateLimits.{name}")
            _bounded_text(capability["scope"], "rateLimits.scope")
            return
        raise ContractValidationError("capabilities.rateLimits.value is not closed")
    if field == "spatial":
        if provider_kind != "mineru_async":
            raise ContractValidationError(
                f"{provider_kind} cannot declare observed spatial capability"
            )
        spatial_fields = {
            "pageIndexBasis",
            "bboxOrder",
            "coordinateUnit",
            "origin",
            "axisDirection",
            "bounds",
            "rotationSemantics",
            "pageDimensionsPath",
        }
        _require_closed_fields(capability, spatial_fields, "spatial")
        if capability["pageIndexBasis"] not in {0, 1}:
            raise ContractValidationError("spatial.pageIndexBasis is invalid")
        for name in spatial_fields - {"pageIndexBasis"}:
            _bounded_text(capability[name], f"spatial.{name}")
        return
    if field == "embedding":
        if provider_kind != "jina_embedding":
            raise ContractValidationError(
                f"{provider_kind} cannot declare observed embedding capability"
            )
        embedding_fields = {
            "dimensions",
            "normalized",
            "maxBatchItems",
            "maxBatchTokens",
            "maxBatchBytes",
        }
        _require_closed_fields(capability, embedding_fields, "embedding")
        for name in embedding_fields - {"normalized"}:
            _positive_int(capability[name], f"embedding.{name}")
        _boolean(capability["normalized"], "embedding.normalized")
        return
    raise ContractValidationError(f"unsupported capability fact: {field}")


def _validate_not_applicable_fact(provider_kind: str, section: str, field: str) -> None:
    allowed = {
        ("mineru_async", "capabilities", "embedding"),
        ("jina_embedding", "capabilities", "spatial"),
        ("jina_rerank", "capabilities", "spatial"),
        ("jina_rerank", "capabilities", "embedding"),
    }
    if (provider_kind, section, field) not in allowed:
        raise ContractValidationError(
            f"{provider_kind}.{section}.{field} cannot be not_applicable"
        )


def _validate_frozen_capability_states(raw: Mapping[str, Any]) -> None:
    provider_kind = _text(raw["providerKind"], "providerKind")
    required: dict[str, dict[str, str]] = {
        "mineru_async": {
            "idempotency": "observed",
            "rateLimits": "observed",
            "spatial": "observed",
            "embedding": "not_applicable",
        },
        "jina_embedding": {
            "idempotency": "observed",
            "rateLimits": "observed",
            "spatial": "not_applicable",
            "embedding": "observed",
        },
        "jina_rerank": {
            "idempotency": "observed",
            "rateLimits": "observed",
            "spatial": "not_applicable",
            "embedding": "not_applicable",
        },
    }
    capabilities = _mapping(raw["capabilities"], "capabilities")
    for field, expected_state in required[provider_kind].items():
        fact = _mapping(capabilities[field], f"capabilities.{field}")
        if fact["state"] != expected_state:
            raise ContractValidationError(
                f"frozen {provider_kind} capability state is incomplete"
            )


def _validate_governance_term_value(field: str, value: object) -> None:
    term = _mapping(value, f"governanceTerms.{field}.value")
    expected: dict[str, set[str]] = {
        "license": {"identifier", "commercialUseAllowed", "sourceVersion"},
        "retention": {"maximumSeconds", "scope"},
        "deletion": {"supported", "maximumCompletionSeconds", "mechanism"},
        "trainingUse": {"allowed", "scope"},
        "sla": {"availabilityBasisPoints", "supportTier"},
    }
    fields = expected.get(field)
    if fields is None:
        raise ContractValidationError(f"unsupported governance term: {field}")
    _require_closed_fields(term, fields, f"governanceTerms.{field}")
    if field == "license":
        _bounded_text(term["identifier"], "license.identifier")
        _boolean(term["commercialUseAllowed"], "license.commercialUseAllowed")
        _bounded_text(term["sourceVersion"], "license.sourceVersion")
    elif field == "retention":
        _nonnegative_int(term["maximumSeconds"], "retention.maximumSeconds")
        _bounded_text(term["scope"], "retention.scope")
    elif field == "deletion":
        _boolean(term["supported"], "deletion.supported")
        _nonnegative_int(
            term["maximumCompletionSeconds"], "deletion.maximumCompletionSeconds"
        )
        _bounded_text(term["mechanism"], "deletion.mechanism")
    elif field == "trainingUse":
        _boolean(term["allowed"], "trainingUse.allowed")
        _bounded_text(term["scope"], "trainingUse.scope")
    else:
        availability = _nonnegative_int(
            term["availabilityBasisPoints"], "sla.availabilityBasisPoints"
        )
        if availability > 10_000:
            raise ContractValidationError("sla.availabilityBasisPoints is invalid")
        _bounded_text(term["supportTier"], "sla.supportTier")


def _require_closed_fields(
    value: Mapping[str, Any], expected: set[str], label: str
) -> None:
    if set(value) != expected:
        raise ContractValidationError(f"{label} value has an invalid shape")


def _require_required_allowed_fields(
    value: Mapping[str, Any],
    required: set[str],
    allowed: set[str],
    label: str,
) -> None:
    keys = set(value)
    if not required <= keys or not keys <= allowed:
        raise ContractValidationError(f"{label} value has an invalid shape")


def _boolean(value: object, label: str) -> bool:
    if not isinstance(value, bool):
        raise ContractValidationError(f"{label} must be boolean")
    return value


def _nonnegative_int(value: object, label: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ContractValidationError(f"{label} must be a non-negative integer")
    return value


def _positive_int(value: object, label: str) -> int:
    result = _nonnegative_int(value, label)
    if result == 0:
        raise ContractValidationError(f"{label} must be positive")
    return result


def _bounded_text(value: object, label: str, *, allow_empty: bool = False) -> str:
    text = _text(value, label, allow_empty=allow_empty)
    if text != text.strip() or len(text.encode()) > 256:
        raise ContractValidationError(f"{label} is invalid")
    return text


def _body_bytes(body: Mapping[str, Any]) -> int:
    if "rawBodyUtf8" in body:
        return len(_text(body["rawBodyUtf8"], "rawBodyUtf8", allow_empty=True).encode())
    try:
        encoded = json.dumps(
            body["json"],
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode()
    except (TypeError, ValueError) as error:
        raise ContractValidationError(
            "fixture body is not canonical JSON data"
        ) from error
    return len(encoded)


def _validate_provider_shape(raw: dict[str, Any]) -> None:
    provider_kind = _text(raw["providerKind"], "providerKind")
    operations = {
        _text(operation["phase"], "phase"): operation
        for operation in cast("list[dict[str, Any]]", raw["operations"])
    }
    if provider_kind == "mineru_async":
        _validate_mineru_shape(raw, operations)
    elif provider_kind == "jina_embedding":
        _validate_jina_embedding_shape(raw, operations["embed"])
    elif provider_kind == "jina_rerank":
        _validate_jina_rerank_shape(raw, operations["rerank"])
    elif raw["fixtureKind"] != "synthetic_test":
        raise ContractValidationError(
            "synthetic HTTP provider requires a synthetic fixture"
        )


def _validate_mineru_shape(
    raw: dict[str, Any], operations: dict[str, dict[str, Any]]
) -> None:
    expected_wire = {
        "submit": ("POST", "/api/v4/extract/task"),
        "poll": ("GET", "/api/v4/extract/task/{task_id}"),
        "result": ("GET", "/api/v4/extract/task/{task_id}"),
    }
    for phase, (method, path) in expected_wire.items():
        operation = operations[phase]
        support = _mapping(operation["support"], "support")
        if support["state"] != "observed":
            continue
        if operation.get("method") != method or operation.get("pathTemplate") != path:
            raise ContractValidationError(f"MinerU {phase} wire shape is invalid")
        _validate_mineru_request(raw, phase, operation)
        for response in _success_responses(operation):
            _validate_mineru_success(phase, response)
        for response in cast("list[dict[str, Any]]", operation["responseCases"]):
            if response["classification"] == "success":
                continue
            if response["classification"] == "terminal_failure":
                _validate_mineru_terminal_failure(phase, response)
            else:
                _validate_mineru_error(phase, response)


def _validate_mineru_request(
    raw: dict[str, Any], phase: str, operation: dict[str, Any]
) -> None:
    request = _mapping(operation["request"], f"MinerU {phase} request")
    request_body = _mapping(request["body"], f"MinerU {phase} request body")
    if phase != "submit":
        if request_body.get("rawBodyUtf8") != "":
            raise ContractValidationError(
                "MinerU poll/result request body must be empty"
            )
        return
    submit = _mapping(request_body.get("json"), "MinerU submit JSON")
    submit_fields = {
        "url",
        "is_ocr",
        "enable_formula",
        "enable_table",
        "model_version",
    }
    _require_closed_fields(submit, submit_fields, "MinerU submit")
    _validate_https_url(_text(submit["url"], "MinerU source URL"), "source URL")
    for name in ("is_ocr", "enable_formula", "enable_table"):
        _boolean(submit[name], f"MinerU submit.{name}")
    model = _text(submit["model_version"], "MinerU model_version")
    identity_model = _observed_text(_mapping(raw["identity"], "identity")["modelId"])
    if identity_model is not None and model != identity_model:
        raise ContractValidationError("MinerU identity/request model mismatch")


def _validate_mineru_success(phase: str, response: dict[str, Any]) -> None:
    payload = _json_body(response, f"MinerU {phase} response")
    if not {"code", "data", "msg"} <= set(payload) or not set(payload) <= {
        "code",
        "data",
        "msg",
        "trace_id",
    }:
        raise ContractValidationError("MinerU success response shape is invalid")
    if payload.get("code") != 0:
        raise ContractValidationError("MinerU success response code must be zero")
    data = _mapping(payload.get("data"), "MinerU response data")
    _text(data.get("task_id"), "MinerU task_id")
    state = data.get("state")
    if phase == "poll" and state not in {"pending", "running"}:
        raise ContractValidationError("MinerU poll state is invalid")
    if phase == "result" and state != "done":
        raise ContractValidationError("MinerU result must be done")
    if phase == "submit":
        expected_fields = {"task_id"}
    elif data.get("state") == "running":
        expected_fields = {"task_id", "state", "err_msg", "extract_progress"}
    elif data.get("state") == "pending":
        expected_fields = {"task_id", "state", "err_msg"}
    elif data.get("state") == "done":
        expected_fields = {"task_id", "state", "full_zip_url", "err_msg"}
    else:
        raise ContractValidationError("MinerU success state is invalid")
    _require_closed_fields(data, expected_fields, f"MinerU {phase} response data")
    _bounded_text(payload["msg"], "MinerU response msg")
    if "trace_id" in payload:
        _bounded_text(payload["trace_id"], "MinerU trace_id")
    if phase == "poll" and data.get("state") == "running":
        _validate_mineru_progress(data)
    if phase == "result":
        _validate_https_url(
            _text(data.get("full_zip_url"), "MinerU result URL"),
            "MinerU result URL",
        )
    if phase in {"poll", "result"}:
        _bounded_text(data["err_msg"], "MinerU err_msg", allow_empty=True)


def _validate_mineru_progress(data: Mapping[str, Any]) -> None:
    state = _text(data.get("state"), "MinerU poll state")
    if state != "running":
        raise ContractValidationError("MinerU poll state is invalid")
    progress = _mapping(data.get("extract_progress"), "MinerU extract_progress")
    _require_closed_fields(
        progress,
        {"extracted_pages", "total_pages", "start_time"},
        "MinerU extract_progress",
    )
    extracted = _nonnegative_int(progress["extracted_pages"], "MinerU extracted_pages")
    total = _positive_int(progress["total_pages"], "MinerU total_pages")
    if extracted > total:
        raise ContractValidationError("MinerU progress exceeds total pages")
    start_time = _text(progress["start_time"], "MinerU start_time")
    if not _MINERU_START_TIME_RE.fullmatch(start_time):
        raise ContractValidationError("MinerU start_time is invalid")


def _validate_mineru_terminal_failure(phase: str, response: dict[str, Any]) -> None:
    if phase not in {"poll", "result"}:
        raise ContractValidationError("MinerU terminal failure phase is invalid")
    payload = _json_body(response, f"MinerU {phase} terminal response")
    _require_closed_fields(
        payload,
        {"code", "data", "msg", "trace_id"},
        f"MinerU {phase} terminal response",
    )
    if payload["code"] != 0:
        raise ContractValidationError("MinerU terminal envelope code is invalid")
    data = _mapping(payload["data"], "MinerU terminal response data")
    _require_closed_fields(
        data,
        {"task_id", "state", "err_msg"},
        "MinerU terminal response data",
    )
    if data["state"] != "failed":
        raise ContractValidationError("MinerU terminal state must be failed")
    _bounded_text(data["task_id"], "MinerU task_id")
    _bounded_text(data["err_msg"], "MinerU err_msg")
    _bounded_text(payload["msg"], "MinerU response msg")
    _bounded_text(payload["trace_id"], "MinerU trace_id")


def _validate_mineru_error(phase: str, response: dict[str, Any]) -> None:
    error_payload = _json_body(response, f"MinerU {phase} error response")
    _require_closed_fields(
        error_payload, {"code", "msg"}, f"MinerU {phase} error response"
    )
    if isinstance(error_payload["code"], bool) or not isinstance(
        error_payload["code"], (int, str)
    ):
        raise ContractValidationError("MinerU error code is invalid")
    _bounded_text(error_payload["msg"], "MinerU error msg")


def _validate_jina_embedding_shape(
    raw: dict[str, Any], operation: dict[str, Any]
) -> None:
    request_json = _request_json(operation)
    _require_closed_fields(
        request_json,
        {
            "model",
            "input",
            "task",
            "dimensions",
            "embedding_type",
            "truncate",
            "late_chunking",
            "return_multivector",
            "return_tokenized_input",
        },
        "Jina embedding request",
    )
    model = _text(request_json.get("model"), "Jina embedding request model")
    if model != "jina-embeddings-v4":
        raise ContractValidationError("Jina passage model is invalid")
    dimensions = request_json.get("dimensions")
    if (
        isinstance(dimensions, bool)
        or not isinstance(dimensions, int)
        or dimensions not in {1024, 2048}
    ):
        raise ContractValidationError("Jina embedding dimension is invalid")
    inputs = request_json.get("input")
    if not isinstance(inputs, list) or not inputs:
        raise ContractValidationError("Jina embedding input must be non-empty")
    for item_raw in inputs:
        item = _mapping(item_raw, "Jina embedding input")
        _require_closed_fields(item, {"text"}, "Jina embedding input")
        _bounded_text(item["text"], "Jina embedding input text")
    if (
        request_json["task"] != "retrieval.passage"
        or request_json["embedding_type"] != "float"
        or request_json["truncate"] is not False
        or request_json["late_chunking"] is not False
        or request_json["return_multivector"] is not False
        or request_json["return_tokenized_input"] is not False
    ):
        raise ContractValidationError("Jina passage request semantics are invalid")
    identity_model = _observed_text(_mapping(raw["identity"], "identity")["modelId"])
    if identity_model is not None and identity_model != model:
        raise ContractValidationError("Jina identity/request model mismatch")
    embedding_fact = _mapping(
        _mapping(raw["capabilities"], "capabilities")["embedding"],
        "capabilities.embedding",
    )
    if embedding_fact["state"] == "observed":
        embedding_value = _mapping(
            embedding_fact["value"], "capabilities.embedding.value"
        )
        if embedding_value["dimensions"] != dimensions:
            raise ContractValidationError("Jina capability/request dimension mismatch")

    for response in _success_responses(operation):
        payload = _json_body(response, "Jina embedding response")
        _require_required_allowed_fields(
            payload,
            {"model", "data", "usage"},
            {"object", "model", "data", "usage"},
            "Jina embedding response",
        )
        if "object" in payload and payload["object"] != "list":
            raise ContractValidationError("Jina embedding object is invalid")
        if payload.get("model") != model:
            raise ContractValidationError("Jina request/response model mismatch")
        data = payload.get("data")
        if not isinstance(data, list) or len(data) != len(inputs):
            raise ContractValidationError(
                "Jina embedding response cardinality mismatch"
            )
        indexes: list[int] = []
        for item_raw in data:
            item = _mapping(item_raw, "Jina embedding item")
            _require_required_allowed_fields(
                item,
                {"index", "embedding"},
                {"object", "index", "embedding"},
                "Jina embedding item",
            )
            if "object" in item and item["object"] != "embedding":
                raise ContractValidationError("Jina embedding item object is invalid")
            index = item.get("index")
            if isinstance(index, bool) or not isinstance(index, int):
                raise ContractValidationError("Jina embedding index is invalid")
            indexes.append(index)
            vector = item.get("embedding")
            if not isinstance(vector, list) or len(vector) != dimensions:
                raise ContractValidationError("Jina embedding dimension mismatch")
            if any(not _is_finite_number(value) for value in vector):
                raise ContractValidationError(
                    "Jina embedding contains a non-finite value"
                )
        if sorted(indexes) != list(range(len(inputs))):
            raise ContractValidationError("Jina embedding indexes are incomplete")
        _validate_usage(
            payload.get("usage"),
            "Jina embedding usage",
            {"total_tokens", "prompt_tokens"},
            {"image_tokens", "audio_tokens", "video_tokens"},
        )


def _validate_jina_rerank_shape(raw: dict[str, Any], operation: dict[str, Any]) -> None:
    request_json = _request_json(operation)
    _require_closed_fields(
        request_json,
        {"model", "query", "documents", "top_n", "return_documents"},
        "Jina rerank request",
    )
    model = _text(request_json.get("model"), "Jina rerank request model")
    _bounded_text(request_json.get("query"), "Jina rerank query")
    documents = request_json.get("documents")
    if not isinstance(documents, list) or not documents:
        raise ContractValidationError("Jina rerank documents must be non-empty")
    for document in documents:
        _bounded_text(document, "Jina rerank document")
    top_n = request_json.get("top_n")
    if (
        isinstance(top_n, bool)
        or not isinstance(top_n, int)
        or top_n <= 0
        or top_n > len(documents)
    ):
        raise ContractValidationError("Jina rerank top_n is invalid")
    if request_json["return_documents"] is not False:
        raise ContractValidationError("Jina rerank fixture must not return documents")
    identity_model = _observed_text(_mapping(raw["identity"], "identity")["modelId"])
    if identity_model is not None and identity_model != model:
        raise ContractValidationError("Jina rerank identity/request model mismatch")
    if (
        _mapping(raw["lifecycle"], "lifecycle")["state"] == "frozen"
        and identity_model is None
    ):
        raise ContractValidationError("frozen Jina rerank identity is incomplete")
    for response in _success_responses(operation):
        payload = _json_body(response, "Jina rerank response")
        _require_required_allowed_fields(
            payload,
            {"model", "usage", "results"},
            {"model", "object", "usage", "results"},
            "Jina rerank response",
        )
        if "object" in payload and payload["object"] != "list":
            raise ContractValidationError("Jina rerank object is invalid")
        response_model = payload.get("model")
        if response_model != model:
            raise ContractValidationError("Jina rerank request/response model mismatch")
        results = payload.get("results")
        if not isinstance(results, list) or not 0 < len(results) <= top_n:
            raise ContractValidationError("Jina rerank result cardinality is invalid")
        indexes: set[int] = set()
        for result_raw in results:
            result = _mapping(result_raw, "Jina rerank result")
            _require_closed_fields(
                result, {"index", "relevance_score"}, "Jina rerank result"
            )
            index = result.get("index")
            if (
                isinstance(index, bool)
                or not isinstance(index, int)
                or index < 0
                or index >= len(documents)
                or index in indexes
            ):
                raise ContractValidationError("Jina rerank index is invalid")
            indexes.add(index)
            if not _is_finite_number(result.get("relevance_score")):
                raise ContractValidationError("Jina rerank score is invalid")
        _validate_usage(
            payload.get("usage"), "Jina rerank usage", {"total_tokens"}, set()
        )


def _request_json(operation: dict[str, Any]) -> Mapping[str, Any]:
    request = _mapping(operation.get("request"), "request")
    body = _mapping(request.get("body"), "request body")
    return _mapping(body.get("json"), "request JSON")


def _success_responses(operation: dict[str, Any]) -> tuple[dict[str, Any], ...]:
    return tuple(
        response
        for response in cast("list[dict[str, Any]]", operation["responseCases"])
        if response["classification"] == "success"
    )


def _json_body(response: dict[str, Any], label: str) -> Mapping[str, Any]:
    body = _mapping(response["body"], f"{label} body")
    return _mapping(body.get("json"), label)


def _validate_usage(
    value: object,
    label: str,
    required_fields: set[str],
    optional_fields: set[str],
) -> None:
    usage = _mapping(value, label)
    _require_required_allowed_fields(
        usage, required_fields, required_fields | optional_fields, label
    )
    if any(
        item is not None
        and (isinstance(item, bool) or not isinstance(item, int) or item < 0)
        for item in usage.values()
    ):
        raise ContractValidationError(f"{label} values must be non-negative integers")


def _is_finite_number(value: object) -> bool:
    return (
        not isinstance(value, bool)
        and isinstance(value, (int, float))
        and math.isfinite(value)
    )


def _to_contract(fixture_name: str, raw: dict[str, Any]) -> ProviderContract:
    frozen_raw = _mapping(_freeze(raw), "fixture")
    operations: list[Operation] = []
    for operation_raw in _mapping_list(frozen_raw["operations"], "operations"):
        response_cases = [
            _to_response_case(response_raw)
            for response_raw in _mapping_list(
                operation_raw["responseCases"], "responseCases"
            )
        ]
        support = _mapping(operation_raw["support"], "support")
        operations.append(
            Operation(
                operation_id=_text(operation_raw["operationId"], "operationId"),
                phase=_text(operation_raw["phase"], "phase"),
                support_state=_text(support["state"], "support.state"),
                method=cast("str | None", operation_raw.get("method")),
                path_template=cast("str | None", operation_raw.get("pathTemplate")),
                request=cast("Mapping[str, Any] | None", operation_raw.get("request")),
                response_cases=tuple(response_cases),
            )
        )
    lifecycle = _mapping(frozen_raw["lifecycle"], "lifecycle")
    return ProviderContract(
        fixture_name=fixture_name,
        fixture_set_id=_text(frozen_raw["fixtureSetId"], "fixtureSetId"),
        fixture_kind=_text(frozen_raw["fixtureKind"], "fixtureKind"),
        provider_kind=_text(frozen_raw["providerKind"], "providerKind"),
        lifecycle_state=_text(lifecycle["state"], "lifecycle.state"),
        raw=frozen_raw,
        operations=tuple(operations),
    )


def _to_response_case(raw: Mapping[str, Any]) -> ResponseCase:
    return ResponseCase(
        case_id=_text(raw["caseId"], "caseId"),
        status=cast("int", raw["status"]),
        headers=cast("Mapping[str, str]", raw["headers"]),
        body=_mapping(raw["body"], "body"),
        classification=_text(raw["classification"], "classification"),
        stable_error_code=cast("str | None", raw.get("stableErrorCode")),
    )


def _reject_unresolved_facts(value: object) -> None:
    if isinstance(value, Mapping):
        if value.get("state") == "unknown":
            raise ContractValidationError("frozen contract contains unknown facts")
        for item in value.values():
            _reject_unresolved_facts(item)
    elif isinstance(value, (list, tuple)):
        for item in value:
            _reject_unresolved_facts(item)


def _observed_text(value: object) -> str | None:
    fact = _mapping(value, "fact")
    if fact["state"] not in {"observed", "terms_verified"}:
        return None
    return _text(fact["value"], "fact.value")


def _unique_ids(values: list[dict[str, Any]], key: str, label: str) -> set[str]:
    result: set[str] = set()
    for value in values:
        item_id = _text(value[key], key)
        if item_id in result:
            raise ContractValidationError(f"duplicate {label} ID")
        result.add(item_id)
    return result


def _mapping(value: object, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ContractValidationError(f"{label} must be an object")
    return cast("Mapping[str, Any]", value)


def _mapping_list(value: object, label: str) -> tuple[Mapping[str, Any], ...]:
    if not isinstance(value, (list, tuple)):
        raise ContractValidationError(f"{label} must be an object list")
    return tuple(_mapping(item, label) for item in value)


def _string_list(value: object, label: str) -> tuple[str, ...]:
    if not isinstance(value, (list, tuple)) or any(
        not isinstance(item, str) for item in value
    ):
        raise ContractValidationError(f"{label} must be a string list")
    return tuple(cast("list[str] | tuple[str, ...]", value))


def _text(value: object, label: str, *, allow_empty: bool = False) -> str:
    if not isinstance(value, str) or (not value and not allow_empty):
        raise ContractValidationError(f"{label} must be non-empty text")
    return value


def _freeze(value: object) -> object:
    if isinstance(value, dict):
        return MappingProxyType({key: _freeze(item) for key, item in value.items()})
    if isinstance(value, list):
        return tuple(_freeze(item) for item in value)
    return value


def _thaw(value: object) -> object:
    if isinstance(value, Mapping):
        return {str(key): _thaw(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [_thaw(item) for item in value]
    return value


def _canonical_hash(value: object) -> str:
    try:
        canonical = rfc8785.dumps(_thaw(value))  # type: ignore[arg-type]
    except (TypeError, ValueError) as error:
        raise ContractValidationError("contract cannot be JCS canonicalized") from error
    return hashlib.sha256(canonical).hexdigest()


def _evidence_by_id(raw: Mapping[str, Any], evidence_id: str) -> Mapping[str, Any]:
    for evidence in _mapping_list(raw["evidence"], "evidence"):
        if evidence["evidenceId"] == evidence_id:
            return evidence
    raise ContractValidationError("fact references unknown evidence")


def _validate_evidence_freshness(
    raw: Mapping[str, Any],
    evidence_snapshots: Mapping[str, bytes],
    now: datetime,
) -> None:
    if now.tzinfo is None:
        raise ContractValidationError("freeze validation time must be timezone-aware")
    active_now = now.astimezone(UTC)
    if _parse_timestamp(_text(raw["observedAt"], "observedAt")) > active_now:
        raise ContractValidationError("frozen contract is future-dated")
    evidence_items = _mapping_list(raw["evidence"], "evidence")
    expected_ids = {
        _text(evidence["evidenceId"], "evidenceId") for evidence in evidence_items
    }
    if set(evidence_snapshots) != expected_ids:
        raise ContractValidationError("evidence snapshot set is incomplete or extra")
    for evidence in evidence_items:
        evidence_id = _text(evidence["evidenceId"], "evidenceId")
        content_hash = _text(evidence.get("contentHash"), "evidence contentHash")
        if not _HASH_RE.fullmatch(content_hash):
            raise ContractValidationError("evidence contentHash is invalid")
        snapshot = evidence_snapshots[evidence_id]
        if not isinstance(snapshot, bytes):
            raise ContractValidationError("evidence snapshot must be exact bytes")
        if hashlib.sha256(snapshot).hexdigest() != content_hash:
            raise ContractValidationError("evidence snapshot content hash mismatch")
        observed_at = _parse_timestamp(
            _text(evidence["observedAt"], "evidence observedAt")
        )
        valid_until = _parse_timestamp(
            _text(evidence.get("validUntil"), "evidence validUntil")
        )
        if (
            observed_at > active_now
            or valid_until <= active_now
            or valid_until <= observed_at
        ):
            raise ContractValidationError("frozen evidence is stale or future-dated")
    for fact in _mapping(raw["governanceTerms"], "governanceTerms").values():
        terms_fact = _mapping(fact, "governance term")
        reviewed_at = _parse_timestamp(
            _text(terms_fact.get("reviewedAt"), "reviewedAt")
        )
        if reviewed_at > active_now:
            raise ContractValidationError("reviewed terms are future-dated")
        for evidence_ref in _string_list(terms_fact["evidenceRefs"], "terms evidence"):
            evidence = _evidence_by_id(raw, evidence_ref)
            evidence_observed_at = _parse_timestamp(
                _text(evidence["observedAt"], "evidence observedAt")
            )
            evidence_valid_until = _parse_timestamp(
                _text(evidence.get("validUntil"), "evidence validUntil")
            )
            if not evidence_observed_at <= reviewed_at < evidence_valid_until:
                raise ContractValidationError(
                    "terms review falls outside the evidence validity window"
                )


def _parse_timestamp(value: str) -> datetime:
    try:
        parsed = datetime.fromisoformat(value)
    except ValueError as error:
        raise ContractValidationError("timestamp is invalid") from error
    if parsed.tzinfo is None:
        raise ContractValidationError("timestamp must include a timezone")
    return parsed.astimezone(UTC)
