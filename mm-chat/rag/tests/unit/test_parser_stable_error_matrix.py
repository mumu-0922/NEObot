from __future__ import annotations

from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from typing import Final

import pytest
from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    load_packaged_schemas,
    load_strict_json_bytes,
    validate_schema_instance,
)
from tests.support.parser_corpus import (
    CORPUS_EXPECTED_ERROR_CODES,
    EXPECTATIONS_PATH,
    load_expectations,
    load_manifest,
)

_STABLE_ERROR_SCHEMA: Final = "parser-stable-error.v1.schema.json"
_WIRE_RESPONSE_SCHEMA: Final = "parser-protocol-response-header.v1.schema.json"
_STABLE_ERROR_BRANCH_COUNT: Final = 22
_BRANCH_FIELDS: Final = (
    "code",
    "outcome",
    "disposition",
    "retryable",
    "stageable",
)
_DECISION_FIELDS: Final = _BRANCH_FIELDS[1:]
_CONTROLLER_ONLY_CODES: Final = frozenset(
    {"PARSER_CANCELLED", "PARSER_SANDBOX_UNAVAILABLE"}
)


@dataclass(frozen=True, slots=True)
class _StableErrorBranch:
    outcome: str
    disposition: str
    retryable: bool
    stageable: bool


@pytest.fixture(scope="module")
def packaged_schemas() -> PackagedSchemas:
    return load_packaged_schemas()


def _object(value: JsonValue, label: str) -> JsonObject:
    assert isinstance(value, dict), f"{label} must be an object"
    return value


def _array(value: JsonValue, label: str) -> list[JsonValue]:
    assert isinstance(value, list), f"{label} must be an array"
    return value


def _string_enum(rule: JsonObject, label: str) -> tuple[str, ...]:
    raw_values = _array(rule.get("enum"), f"{label}.enum")
    values: list[str] = []
    for value in raw_values:
        assert isinstance(value, str), f"{label}.enum must contain only strings"
        values.append(value)
    assert len(values) == len(set(values)), f"{label}.enum contains duplicates"
    return tuple(values)


def _const(properties: JsonObject, field: str) -> JsonValue:
    rule = _object(properties.get(field), f"oneOf.properties.{field}")
    assert "const" in rule, f"oneOf branch does not freeze {field}"
    return rule["const"]


def _stable_error_matrix(
    packaged_schemas: PackagedSchemas,
) -> dict[str, _StableErrorBranch]:
    schema = packaged_schemas.by_name[_STABLE_ERROR_SCHEMA]
    properties = _object(schema.get("properties"), "stable-error properties")
    required = _array(schema.get("required"), "stable-error required fields")
    assert all(isinstance(field, str) for field in required)
    assert len(required) == len(set(required))
    assert set(required) == {"schemaVersion", *_BRANCH_FIELDS}
    assert schema.get("additionalProperties") is False
    inventory = _string_enum(
        _object(properties.get("code"), "stable-error code"),
        "stable-error code",
    )
    raw_branches = _array(schema.get("oneOf"), "stable-error oneOf")

    assert len(inventory) == _STABLE_ERROR_BRANCH_COUNT
    assert len(raw_branches) == _STABLE_ERROR_BRANCH_COUNT

    matrix: dict[str, _StableErrorBranch] = {}
    branch_order: list[str] = []
    for index, raw_branch in enumerate(raw_branches):
        branch = _object(raw_branch, f"oneOf[{index}]")
        branch_properties = _object(
            branch.get("properties"), f"oneOf[{index}].properties"
        )
        assert set(branch_properties) == set(_BRANCH_FIELDS)

        code = _const(branch_properties, "code")
        outcome = _const(branch_properties, "outcome")
        disposition = _const(branch_properties, "disposition")
        retryable = _const(branch_properties, "retryable")
        stageable = _const(branch_properties, "stageable")
        assert isinstance(code, str)
        assert isinstance(outcome, str)
        assert isinstance(disposition, str)
        assert type(retryable) is bool
        assert type(stageable) is bool
        assert branch.get("title") == code
        assert code not in matrix, f"duplicate oneOf branch for {code}"

        matrix[code] = _StableErrorBranch(
            outcome=outcome,
            disposition=disposition,
            retryable=retryable,
            stageable=stageable,
        )
        branch_order.append(code)

    assert tuple(branch_order) == inventory
    return matrix


def _canonical_object(value: JsonObject) -> JsonObject:
    content = canonical_json_bytes(value)
    decoded = load_strict_json_bytes(content)
    assert decoded == value
    return _object(decoded, "canonical JSON instance")


def _stable_error_instance(code: str, branch: _StableErrorBranch) -> JsonObject:
    return _canonical_object(
        {
            "schemaVersion": "parser-stable-error.v1",
            "code": code,
            "outcome": branch.outcome,
            "disposition": branch.disposition,
            "retryable": branch.retryable,
            "stageable": branch.stageable,
        }
    )


def _wire_error_response(code: str, outcome: str) -> JsonObject:
    return _canonical_object(
        {
            "invocationId": f"stable-error-{code.lower()}",
            "outcome": outcome,
            "canonicalSchemaVersion": None,
            "resultBytes": 0,
            "resultSha256": None,
            "stableErrorCode": code,
        }
    )


def _alternate_decision(
    schema: JsonObject,
    field: str,
    current: JsonValue,
) -> JsonValue:
    if type(current) is bool:
        return not current

    properties = _object(schema.get("properties"), "stable-error properties")
    choices = _string_enum(
        _object(properties.get(field), f"stable-error {field}"),
        f"stable-error {field}",
    )
    return next(choice for choice in choices if choice != current)


def _expectation_entries() -> list[JsonObject]:
    content = EXPECTATIONS_PATH.read_bytes()
    document = load_strict_json_bytes(content)
    assert canonical_json_bytes(document) == content
    root = _object(document, "parser corpus expectations")
    return [
        _object(entry, f"expectations.entries[{index}]")
        for index, entry in enumerate(
            _array(root.get("entries"), "expectations.entries")
        )
    ]


def _expected_error_codes(
    entries: Iterable[Mapping[str, object]],
) -> frozenset[str]:
    result: set[str] = set()
    for index, entry in enumerate(entries):
        error = entry.get("expectedError")
        if error is None:
            continue
        assert isinstance(error, str), f"expectedError[{index}] must be a string"
        result.add(error)
    return frozenset(result)


def test_stable_error_inventory_is_the_exact_22_branch_matrix(
    packaged_schemas: PackagedSchemas,
) -> None:
    matrix = _stable_error_matrix(packaged_schemas)

    assert len(matrix) == _STABLE_ERROR_BRANCH_COUNT
    assert all(branch.stageable is False for branch in matrix.values())
    assert {
        code for code, branch in matrix.items() if branch.outcome == "route_required"
    } == {"MINERU_REQUIRED"}
    assert {
        code
        for code, branch in matrix.items()
        if branch.disposition == "route_required"
    } == {"MINERU_REQUIRED"}
    assert matrix["MINERU_REQUIRED"] == _StableErrorBranch(
        outcome="route_required",
        disposition="route_required",
        retryable=False,
        stageable=False,
    )


def test_every_branch_rejects_each_invalid_decision_combination(
    packaged_schemas: PackagedSchemas,
) -> None:
    schema = packaged_schemas.by_name[_STABLE_ERROR_SCHEMA]
    matrix = _stable_error_matrix(packaged_schemas)

    for code, branch in matrix.items():
        valid_instance = _stable_error_instance(code, branch)
        validate_schema_instance(
            packaged_schemas,
            _STABLE_ERROR_SCHEMA,
            valid_instance,
        )

        for field in _DECISION_FIELDS:
            invalid_instance = valid_instance.copy()
            invalid_instance[field] = _alternate_decision(
                schema,
                field,
                valid_instance[field],
            )
            with pytest.raises(ValidationError):
                validate_schema_instance(
                    packaged_schemas,
                    _STABLE_ERROR_SCHEMA,
                    _canonical_object(invalid_instance),
                )


def test_wire_response_excludes_only_controller_synthesized_errors(
    packaged_schemas: PackagedSchemas,
) -> None:
    matrix = _stable_error_matrix(packaged_schemas)
    response_schema = packaged_schemas.by_name[_WIRE_RESPONSE_SCHEMA]
    definitions = _object(response_schema.get("$defs"), "wire response $defs")
    wire_failure_codes = _string_enum(
        _object(definitions.get("wireFailureCode"), "wire failure code"),
        "wire failure code",
    )
    route_required = _object(
        definitions.get("routeRequired"), "wire route-required branch"
    )
    route_properties = _object(
        route_required.get("properties"), "wire route-required properties"
    )
    route_error = _const(route_properties, "stableErrorCode")

    assert isinstance(route_error, str)
    assert route_error == "MINERU_REQUIRED"
    assert set(matrix) - set(wire_failure_codes) - {route_error} == (
        _CONTROLLER_ONLY_CODES
    )
    assert set(wire_failure_codes) == {
        code
        for code, branch in matrix.items()
        if branch.outcome == "failure" and code not in _CONTROLLER_ONLY_CODES
    }

    validate_schema_instance(
        packaged_schemas,
        _WIRE_RESPONSE_SCHEMA,
        _wire_error_response("MINERU_REQUIRED", "route_required"),
    )
    for code in wire_failure_codes:
        validate_schema_instance(
            packaged_schemas,
            _WIRE_RESPONSE_SCHEMA,
            _wire_error_response(code, "failure"),
        )
    for code in _CONTROLLER_ONLY_CODES:
        with pytest.raises(ValidationError):
            validate_schema_instance(
                packaged_schemas,
                _WIRE_RESPONSE_SCHEMA,
                _wire_error_response(code, "failure"),
            )


def test_corpus_errors_and_support_allowlist_cannot_drift_from_schema(
    packaged_schemas: PackagedSchemas,
) -> None:
    inventory = frozenset(_stable_error_matrix(packaged_schemas))
    expected_errors = _expected_error_codes(_expectation_entries())

    assert expected_errors <= inventory

    loaded_expectations = load_expectations(load_manifest())
    loaded_errors = _expected_error_codes(loaded_expectations.values())
    assert loaded_errors == expected_errors

    assert expected_errors == CORPUS_EXPECTED_ERROR_CODES
    assert inventory >= CORPUS_EXPECTED_ERROR_CODES
