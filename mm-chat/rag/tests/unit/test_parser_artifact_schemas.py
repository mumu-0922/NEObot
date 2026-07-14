from __future__ import annotations

from pathlib import Path
from typing import cast

import pytest
from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    MAX_SAFE_INTEGER,
    ContractProfileError,
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    load_packaged_schemas,
    load_strict_json_bytes,
    validate_domain_hash_field,
    validate_schema_instance,
)

_FIXTURE_ROOT = (
    Path(__file__).parents[1] / "fixtures" / "parser_contracts" / "schema_instances"
)
_SCHEMA_CASES = (
    ("canonical-ir.v2.json", "canonical-ir.v2.schema.json"),
    ("canonical-manifest.v2.json", "canonical-manifest.v2.schema.json"),
    ("chunk-manifest.v2.json", "chunk-manifest.v2.schema.json"),
    ("chunk-profile.v1.json", "chunk-profile.v1.schema.json"),
    ("normalization-map.v1.json", "normalization-map.v1.schema.json"),
    ("normalization-profile.v1.json", "normalization-profile.v1.schema.json"),
    ("parser-profile.v1.json", "parser-profile.v1.schema.json"),
    (
        "parser-protocol-request-header.v1.json",
        "parser-protocol-request-header.v1.schema.json",
    ),
    (
        "parser-protocol-response-header.failure.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    (
        "parser-protocol-response-header.route-required.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    (
        "parser-protocol-response-header.success.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    ("parser-resource-profile.v1.json", "parser-resource-profile.v1.schema.json"),
    ("parser-stable-error.v1.json", "parser-stable-error.v1.schema.json"),
    ("quality-report.v2.json", "quality-report.v2.schema.json"),
    ("source-locator.v2.json", "source-locator.v2.schema.json"),
    ("source-unit-resolver.v1.json", "source-unit-resolver.v1.schema.json"),
    (
        "synthetic-mineru-artifact.v1.json",
        "synthetic-mineru-artifact.v1.schema.json",
    ),
)
_SCHEMA_NEGATIVE_CASES = (
    (
        "unknown-top-level-field",
        "parser-protocol-request-header.v1.json",
        "parser-protocol-request-header.v1.schema.json",
    ),
    (
        "unknown-nested-field",
        "canonical-ir.v2.json",
        "canonical-ir.v2.schema.json",
    ),
    (
        "identity-with-renderer-field",
        "normalization-map.v1.json",
        "normalization-map.v1.schema.json",
    ),
    (
        "text-view-with-page-field",
        "source-locator.v2.json",
        "source-locator.v2.schema.json",
    ),
    (
        "raw-resolver-with-synthetic-field",
        "source-unit-resolver.v1.json",
        "source-unit-resolver.v1.schema.json",
    ),
    (
        "empty-locator",
        "source-locator.v2.json",
        "source-locator.v2.schema.json",
    ),
    (
        "short-hash",
        "canonical-manifest.v2.json",
        "canonical-manifest.v2.schema.json",
    ),
    (
        "uppercase-hash",
        "parser-protocol-request-header.v1.json",
        "parser-protocol-request-header.v1.schema.json",
    ),
    (
        "stable-error-wrong-retryability",
        "parser-stable-error.v1.json",
        "parser-stable-error.v1.schema.json",
    ),
    (
        "success-with-stable-error",
        "parser-protocol-response-header.success.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    (
        "success-with-empty-result",
        "parser-protocol-response-header.success.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    (
        "route-required-with-result-hash",
        "parser-protocol-response-header.route-required.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
    (
        "failure-with-canonical-version",
        "parser-protocol-response-header.failure.v1.json",
        "parser-protocol-response-header.v1.schema.json",
    ),
)


@pytest.fixture(scope="module")
def packaged_schemas() -> PackagedSchemas:
    return load_packaged_schemas()


def _load_fixture(fixture_name: str) -> JsonObject:
    value = load_strict_json_bytes((_FIXTURE_ROOT / fixture_name).read_bytes())
    assert isinstance(value, dict)
    return value


def _object_at(value: JsonValue, *path: str | int) -> JsonObject:
    current = value
    for part in path:
        if isinstance(part, int):
            assert isinstance(current, list)
            current = current[part]
        else:
            assert isinstance(current, dict)
            current = current[part]
    assert isinstance(current, dict)
    return current


def _apply_schema_mutation(instance: JsonObject, mutation: str) -> None:
    if mutation == "unknown-top-level-field":
        instance["unexpected"] = True
    elif mutation == "unknown-nested-field":
        _object_at(instance, "source")["runtimePath"] = "forbidden-runtime-path"
    elif mutation == "identity-with-renderer-field":
        _object_at(instance, "segments", 0, "transform")["rendererRuleId"] = (
            "paragraph.separator"
        )
    elif mutation == "text-view-with-page-field":
        _object_at(
            instance,
            "textAnchors",
            0,
            "sourceFragments",
            0,
            "views",
            0,
        )["pageIndex"] = 0
    elif mutation == "raw-resolver-with-synthetic-field":
        _object_at(instance, "entries", 0, "payload")["role"] = "layout"
    elif mutation == "empty-locator":
        instance["textAnchors"] = []
        instance["structuralAnchors"] = []
    elif mutation == "short-hash":
        instance["configHash"] = "0" * 63
    elif mutation == "uppercase-hash":
        instance["requestBindingHash"] = "A" * 64
    elif mutation == "stable-error-wrong-retryability":
        instance["retryable"] = True
    elif mutation == "success-with-stable-error":
        instance["stableErrorCode"] = "PARSER_BUSY"
    elif mutation == "success-with-empty-result":
        instance["resultBytes"] = 0
    elif mutation == "route-required-with-result-hash":
        instance["resultSha256"] = "0" * 64
    elif mutation == "failure-with-canonical-version":
        instance["canonicalSchemaVersion"] = "canonical-ir.v2"
    else:
        raise AssertionError(f"unknown schema mutation: {mutation}")


def test_schema_instance_inventory_is_explicit() -> None:
    assert {path.name for path in _FIXTURE_ROOT.glob("*.json")} == {
        fixture_name for fixture_name, _schema_name in _SCHEMA_CASES
    }


@pytest.mark.parametrize(("fixture_name", "schema_name"), _SCHEMA_CASES)
def test_canonical_jcs_instances_validate_with_packaged_offline_registry(
    packaged_schemas: PackagedSchemas,
    fixture_name: str,
    schema_name: str,
) -> None:
    content = (_FIXTURE_ROOT / fixture_name).read_bytes()
    instance = load_strict_json_bytes(content)
    assert canonical_json_bytes(instance) == content
    validate_schema_instance(packaged_schemas, schema_name, instance)


def test_synthetic_mineru_schema_instance_is_one_independent_role_artifact(
    packaged_schemas: PackagedSchemas,
) -> None:
    schema_name = "synthetic-mineru-artifact.v1.schema.json"
    instance = _load_fixture("synthetic-mineru-artifact.v1.json")

    assert instance["role"] == "layout"
    assert {"layout", "middle"}.isdisjoint(instance)
    validate_schema_instance(packaged_schemas, schema_name, instance)

    instance["role"] = "middle"
    with pytest.raises(ValidationError):
        validate_schema_instance(packaged_schemas, schema_name, instance)

    combined = _load_fixture("synthetic-mineru-artifact.v1.json")
    combined["middle"] = {"pages": [], "role": "middle"}
    with pytest.raises(ValidationError):
        validate_schema_instance(packaged_schemas, schema_name, combined)


@pytest.mark.parametrize(
    ("mutation", "fixture_name", "schema_name"),
    _SCHEMA_NEGATIVE_CASES,
    ids=[case[0] for case in _SCHEMA_NEGATIVE_CASES],
)
def test_schema_instances_reject_closed_shape_and_variant_violations(
    packaged_schemas: PackagedSchemas,
    mutation: str,
    fixture_name: str,
    schema_name: str,
) -> None:
    instance = _load_fixture(fixture_name)
    _apply_schema_mutation(instance, mutation)

    with pytest.raises(ValidationError):
        validate_schema_instance(packaged_schemas, schema_name, instance)


@pytest.mark.parametrize(
    ("field", "invalid_value"),
    [
        pytest.param("expectedSourceBytes", 0.5, id="float"),
        pytest.param("deadlineUnixMillis", MAX_SAFE_INTEGER + 1, id="unsafe-integer"),
    ],
)
def test_schema_instances_reject_non_contract_numbers_before_schema_validation(
    packaged_schemas: PackagedSchemas,
    field: str,
    invalid_value: object,
) -> None:
    instance = cast(
        "dict[str, object]",
        _load_fixture("parser-protocol-request-header.v1.json"),
    )
    instance[field] = invalid_value

    with pytest.raises(ContractProfileError, match="float|safe range"):
        validate_schema_instance(
            packaged_schemas,
            "parser-protocol-request-header.v1.schema.json",
            cast("JsonValue", instance),
        )


def test_normalization_map_aggregate_hash_is_domain_separated_and_recomputed() -> None:
    instance = _load_fixture("normalization-map.v1.json")

    validate_domain_hash_field(
        instance,
        domain="mm-chat.normalization-map.v1\n",
        hash_field="aggregateHash",
    )

    instance["textBufferBytes"] = 2
    with pytest.raises(ContractProfileError, match="does not match"):
        validate_domain_hash_field(
            instance,
            domain="mm-chat.normalization-map.v1\n",
            hash_field="aggregateHash",
        )
