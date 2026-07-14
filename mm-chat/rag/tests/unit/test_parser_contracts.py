from __future__ import annotations

import re
from pathlib import Path
from typing import cast

import pytest
from jsonschema import Draft202012Validator
from jsonschema.exceptions import ValidationError
from referencing.exceptions import Unresolvable

from mm_chat_rag.contracts.resources import read_schema_bytes, schema_names
from tests.support.parser_contracts import (
    MAX_SAFE_INTEGER,
    SCHEMA_ROOT,
    ContractProfileError,
    JsonObject,
    JsonValue,
    assert_packaged_schema_closure,
    canonical_json_bytes,
    domain_separated_sha256,
    load_packaged_schemas,
    load_strict_json_bytes,
    logical_hash_envelope_sha256,
    validate_contract_value,
    validate_schema_instance,
    validator_for_schema,
)

_FIXTURE_ROOT = Path(__file__).parents[1] / "fixtures" / "parser_contracts"
_VALID_VECTORS = frozenset(
    {
        "arrays-and-scalars.json",
        "nested-object.json",
        "scalar-null.json",
        "scalar-string.json",
    }
)
_INVALID_VECTORS = {
    "bom-utf8.json": "BOM",
    "duplicate-key.json": "duplicate",
    "float-decimal.json": "float",
    "float-exponent.json": "float",
    "invalid-utf8-surrogate.json": "UTF-8",
    "noncanonical-escape.json": "canonical",
    "noncanonical-key-order.json": "canonical",
    "noncanonical-whitespace.json": "canonical",
    "nonfinite-nan.json": "numeric constant",
    "nul-byte.json": "NUL",
    "nul-escaped.json": "NUL",
    "surrogate-high.json": "surrogate",
    "surrogate-low.json": "surrogate",
    "surrogate-pair-escaped.json": "canonical",
    "unsafe-integer-negative.json": "safe range",
    "unsafe-integer-positive.json": "safe range",
}
_SCHEMA_NAMES = (
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
_GOLDEN_HASHES = {
    "opaque_source_unit_id": (
        "bb0f63ffc2c7f5f82173ca249f623a56c645dc08dc6a1608640ad7a4767a2a1e"
    ),
    "source_unit_payload_ref": (
        "298b2a57b973e4b1c1b20877fbe62e4bed9effceb65751d453bb66401904ea16"
    ),
    "flow_seed_id": (
        "370cbe28cba818a39dbaf87d8c1b87cbbd9577f533ddc0ec6e56bad8c6c85ad8"
    ),
    "owner_seed_id": (
        "607d960044f3fb572e1eae589f9a53b441a43430129a238af23289d0ccf50554"
    ),
    "opaque_sheet_id": (
        "cb013db39257fa5b60b7b9bfade703150b14d6a6a37c0c0e14d1791d65cde38e"
    ),
    "opaque_shape_id": (
        "342717d021028e58df0d1ba12ff863f0b4fc7e6d1067244858010704b786d4c3"
    ),
    "opaque_structure_id": (
        "a74c5fae9a2df04f6bbdaa815c8fe4ae01a5e832c10854767938340443b5034c"
    ),
    "node_payload_ref": (
        "2c03172c5641cb37c171fcc5f4df190d092a3f1b8fc2b9ed9a985a06fdc996ce"
    ),
    "text_source_span_hash": (
        "78a5b29e347e65a14d8a99a2e7edad1acc3efca860fbe4d9775aafb0e4ef633c"
    ),
    "structural_source_span_hash": (
        "e25a035c437f1fd3fb9b57eacc2b39890cc7b5d1208471f0785c5eb2b7a21ff8"
    ),
    "provenance_hash": (
        "5ceb7b3cfada1dd721810297d528977b1a5385320edea52a5be540bc6268f7c1"
    ),
    "provenance_id": (
        "adcc24cf9d51656e8f2362890323b3640b786cf92bf4c7e328af6c18a22a4937"
    ),
    "logical_block_id": (
        "9aa017d99703234e7bb98f1d1f42986510c4fb197b50ad82bc9426e306f236e7"
    ),
    "logical_table_id": (
        "b9a52dde5355271aa36839401f5b5ca53ce795073ea49e083653c6839822e331"
    ),
    "logical_cell_id": (
        "bdc1ccfbd92555bb52eed45ccf69e227d69c2fa2b7370085c556563c583f54d9"
    ),
    "logical_formula_id": (
        "9ace070fd89ed81ec892c047bcd06205a36086826644aad7c8b4cf581a8bd4fb"
    ),
    "logical_asset_id": (
        "0a0bd00ebb08f4793e30deda07fc88ea8c0efe89c4e5d72eb4e76c0b0419c57a"
    ),
    "logical_flow_id": (
        "2af56c3fbd8550def175729f1d2d1489feb0bf7f28642727b3db11b9bbbf4cdf"
    ),
    "parent_chunk_seed_id": (
        "569e44a64b020a998529a7f01e46a65c280d454404e7e1aabf58e791466b8d02"
    ),
    "overlap_group_id": (
        "3eeaac3c1e4ab76adbad416da0a89a885d1e89edc2344db9d247aaceaa8f240d"
    ),
    "source_reuse_group_id": (
        "b2e1aa89ccd7a81da0e395472d30d383d96d3467758b2ab0d296d89594d47770"
    ),
    "logical_parent_chunk_id": (
        "b97859047496876b6e61713b89161d3db017f4e78f4545765c7ea35b949fc7d9"
    ),
    "logical_child_chunk_id": (
        "ed085a608e9f4f67eff1e775fadd5dd3509ad65e8333b1372921f20b6d7a368d"
    ),
    "chunk_source_span_hash": (
        "00ca3dd98c40abb29b1235455b830bca5201fe6d26971cebcb2233503bd9aed0"
    ),
}


def _logical_hash_discriminator_inventory() -> tuple[str, ...]:
    schema = load_packaged_schemas().by_name["logical-hash-envelope.v1.schema.json"]
    schema_properties = schema["properties"]
    assert isinstance(schema_properties, dict)
    envelope_kind_schema = schema_properties["envelopeKind"]
    assert isinstance(envelope_kind_schema, dict)
    raw_inventory = envelope_kind_schema["enum"]
    assert isinstance(raw_inventory, list)
    assert all(isinstance(kind, str) for kind in raw_inventory)
    inventory = cast("list[str]", raw_inventory)

    raw_branches = schema["oneOf"]
    assert isinstance(raw_branches, list)

    branch_inventory: list[str] = []
    for raw_branch in raw_branches:
        assert isinstance(raw_branch, dict)
        title = raw_branch["title"]
        properties = raw_branch["properties"]
        assert isinstance(title, str)
        assert isinstance(properties, dict)
        envelope_kind_schema = properties["envelopeKind"]
        assert isinstance(envelope_kind_schema, dict)
        envelope_kind = envelope_kind_schema["const"]
        assert isinstance(envelope_kind, str)
        assert title == envelope_kind
        branch_inventory.append(envelope_kind)

    assert len(inventory) == len(set(inventory))
    assert branch_inventory == inventory
    return tuple(inventory)


def test_valid_contract_profile_vectors_are_exact_canonical_bytes() -> None:
    vector_paths = tuple(sorted((_FIXTURE_ROOT / "valid").glob("*.json")))
    assert {path.name for path in vector_paths} == _VALID_VECTORS

    for path in vector_paths:
        content = path.read_bytes()
        value = load_strict_json_bytes(content)
        assert canonical_json_bytes(value) == content


@pytest.mark.parametrize(
    ("fixture_name", "message"),
    sorted(_INVALID_VECTORS.items()),
)
def test_invalid_contract_profile_vectors_fail_closed(
    fixture_name: str,
    message: str,
) -> None:
    invalid_names = {path.name for path in (_FIXTURE_ROOT / "invalid").glob("*.json")}
    assert invalid_names == set(_INVALID_VECTORS)
    with pytest.raises(ContractProfileError, match=message):
        load_strict_json_bytes((_FIXTURE_ROOT / "invalid" / fixture_name).read_bytes())


@pytest.mark.parametrize(
    ("value", "message"),
    [
        (0.0, "float"),
        (float("inf"), "float"),
        (MAX_SAFE_INTEGER + 1, "safe range"),
        (-(MAX_SAFE_INTEGER + 1), "safe range"),
        ("embedded\x00nul", "NUL"),
        ("unpaired-\ud800", "surrogate"),
        ({"bad\udfff": None}, "surrogate"),
        ({1: "non-string-key"}, "non-string"),
        (("tuple-is-not-json",), "unsupported"),
        (b"bytes-are-not-json", "unsupported"),
    ],
)
def test_recursive_profile_validator_rejects_non_contract_values(
    value: object,
    message: str,
) -> None:
    with pytest.raises(ContractProfileError, match=message):
        validate_contract_value(value)


def test_contract_profile_keeps_safe_boundaries_and_jcs_object_order() -> None:
    value: JsonValue = {
        "z": MAX_SAFE_INTEGER,
        "a": -MAX_SAFE_INTEGER,
        "allowed": [None, False, True, "雪山😀"],
    }
    validate_contract_value(value)
    assert (
        canonical_json_bytes(value)
        == (
            '{"a":-9007199254740991,"allowed":[null,false,true,"雪山😀"],'
            '"z":9007199254740991}'
        ).encode()
    )


def test_logical_hash_vectors_are_stable_and_domain_separated() -> None:
    discriminator_inventory = _logical_hash_discriminator_inventory()
    golden = load_strict_json_bytes(
        (_FIXTURE_ROOT / "logical_hash" / "golden-v1.json").read_bytes()
    )
    assert isinstance(golden, dict)
    assert golden["algorithm"] == "sha-256"
    assert golden["framing"] == (
        "ASCII(domain-with-one-terminal-LF) || RFC8785(envelopeWithoutDomain)"
    )
    vectors = golden["vectors"]
    assert isinstance(vectors, list)

    observed: dict[str, str] = {}
    observed_names: list[str] = []
    observed_envelope_kinds: list[str] = []
    for raw_vector in vectors:
        assert isinstance(raw_vector, dict)
        name = raw_vector["name"]
        domain = raw_vector["domain"]
        expected = raw_vector["expectedSha256"]
        assert isinstance(name, str)
        assert isinstance(domain, str)
        assert isinstance(expected, str)
        assert re.fullmatch(r"[\x21-\x7e]+\n", domain, flags=re.ASCII)
        assert re.fullmatch(r"[0-9a-f]{64}", expected)
        envelope_without_domain = raw_vector["envelopeWithoutDomain"]
        assert isinstance(envelope_without_domain, dict)
        envelope_kind = envelope_without_domain["envelopeKind"]
        assert isinstance(envelope_kind, str)
        assert domain_separated_sha256(domain, envelope_without_domain) == expected
        envelope: JsonObject = {**envelope_without_domain, "domain": domain}
        assert logical_hash_envelope_sha256(envelope) == expected
        observed_names.append(name)
        observed_envelope_kinds.append(envelope_kind)
        observed[name] = expected

    assert tuple(observed_names) == discriminator_inventory
    assert tuple(observed_envelope_kinds) == discriminator_inventory
    assert tuple(_GOLDEN_HASHES) == discriminator_inventory
    assert observed == _GOLDEN_HASHES
    assert len(set(observed.values())) == len(discriminator_inventory)
    assert domain_separated_sha256("mm-chat.normalization-map.v1\n", None) == (
        "48a3f9f452d56d807f1dc9a8db19a7d86a404f614865d48442451de300ac88bf"
    )


@pytest.mark.parametrize(
    "domain",
    [
        "",
        "missing-terminal-lf",
        "space inside\n",
        "tab\tinside\n",
        "embedded\nlf\n",
        "extra-terminal-lf\n\n",
        "非-ascii\n",
        "nul\x00domain\n",
    ],
)
def test_logical_hash_rejects_ambiguous_domains(domain: str) -> None:
    with pytest.raises(ContractProfileError, match="domain|NUL"):
        domain_separated_sha256(domain, None)


def test_packaged_schema_inventory_is_draft_2020_12_closed_and_offline() -> None:
    assert schema_names() == _SCHEMA_NAMES
    packaged = load_packaged_schemas()
    assert tuple(packaged.by_name) == _SCHEMA_NAMES
    assert tuple(packaged.by_id) == tuple(
        f"{SCHEMA_ROOT}{name}" for name in _SCHEMA_NAMES
    )
    assert_packaged_schema_closure(packaged)

    with pytest.raises(Unresolvable):
        packaged.registry.resolver(SCHEMA_ROOT).lookup("not-packaged.v1.schema.json")


def test_every_packaged_schema_builds_an_offline_draft_validator() -> None:
    packaged = load_packaged_schemas()
    for schema_name in _SCHEMA_NAMES:
        validator = validator_for_schema(packaged, schema_name)
        assert isinstance(validator, Draft202012Validator)
        assert validator.schema is packaged.by_name[schema_name]


def test_logical_hash_goldens_are_valid_domain_bound_envelopes() -> None:
    packaged = load_packaged_schemas()
    golden = load_strict_json_bytes(
        (_FIXTURE_ROOT / "logical_hash" / "golden-v1.json").read_bytes()
    )
    assert isinstance(golden, dict)
    vectors = golden["vectors"]
    assert isinstance(vectors, list)

    envelopes: list[JsonObject] = []
    for vector in vectors:
        assert isinstance(vector, dict)
        domain = vector["domain"]
        envelope_without_domain = vector["envelopeWithoutDomain"]
        assert isinstance(domain, str)
        assert isinstance(envelope_without_domain, dict)
        envelope: JsonObject = {**envelope_without_domain, "domain": domain}
        validate_schema_instance(
            packaged,
            "logical-hash-envelope.v1.schema.json",
            envelope,
        )
        envelopes.append(envelope)

    mismatched_domain = envelopes[0].copy()
    mismatched_domain["domain"] = "mm-chat.logical-block-id.v1\n"
    with pytest.raises(ValidationError):
        validate_schema_instance(
            packaged,
            "logical-hash-envelope.v1.schema.json",
            mismatched_domain,
        )


def test_offline_validator_resolves_cross_schema_refs_and_closes_objects() -> None:
    packaged = load_packaged_schemas()
    hash_a = "a" * 64
    hash_b = "b" * 64
    hash_c = "c" * 64
    request: JsonObject = {
        "deadlineUnixMillis": 0,
        "expectedSourceBytes": 0,
        "expectedSourceSha256": hash_a,
        "invocationId": "fixture-invocation",
        "maxResultBytes": 1,
        "parserConfigHash": hash_b,
        "requestBindingHash": hash_c,
    }
    validate_schema_instance(
        packaged,
        "parser-protocol-request-header.v1.schema.json",
        request,
    )

    request["unexpected"] = True
    with pytest.raises(ValidationError, match="Additional properties"):
        validate_schema_instance(
            packaged,
            "parser-protocol-request-header.v1.schema.json",
            request,
        )


def test_schema_loader_is_allowlisted_and_profile_validation_precedes_schema() -> None:
    packaged = load_packaged_schemas()
    with pytest.raises(ValueError, match="unknown parser contract schema"):
        read_schema_bytes("../pyproject.toml")
    with pytest.raises(ContractProfileError, match="float"):
        validate_schema_instance(
            packaged,
            "canonical-common.v1.schema.json",
            cast("JsonValue", 1.0),
        )
