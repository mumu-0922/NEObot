"""Strict, offline helpers for the parser contract test suite."""

from __future__ import annotations

import hashlib
import json
from collections.abc import Iterator, Mapping
from dataclasses import dataclass
from types import MappingProxyType
from typing import Any, Final, Never, cast
from urllib.parse import urldefrag, urljoin

import rfc8785
from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import SchemaError
from referencing import Registry
from referencing.exceptions import Unresolvable
from referencing.jsonschema import DRAFT202012

from mm_chat_rag.contracts.resources import read_schema_bytes, schema_names

type JsonScalar = None | bool | str | int
type JsonValue = JsonScalar | list[JsonValue] | dict[str, JsonValue]
type JsonObject = dict[str, JsonValue]

SCHEMA_ROOT: Final = "https://schemas.mm-chat.invalid/parser/"
DRAFT_2020_12_ID: Final = "https://json-schema.org/draft/2020-12/schema"
MAX_SAFE_INTEGER: Final = (1 << 53) - 1
_BOMS: Final = (
    b"\x00\x00\xfe\xff",
    b"\xff\xfe\x00\x00",
    b"\xef\xbb\xbf",
    b"\xfe\xff",
    b"\xff\xfe",
)


class ContractProfileError(ValueError):
    """Contract data violates the parser's strict JSON/JCS profile."""


class SchemaClosureError(ValueError):
    """The packaged parser schema graph is incomplete or not self-contained."""


@dataclass(frozen=True, slots=True)
class PackagedSchemas:
    """An immutable, network-disabled view of all packaged parser schemas."""

    by_name: Mapping[str, JsonObject]
    by_id: Mapping[str, JsonObject]
    registry: Registry[Any]


def validate_unicode_scalar(value: str, *, path: str = "$value") -> None:
    """Reject NUL and UTF-16 surrogate code points in one Python string."""
    if "\x00" in value:
        raise ContractProfileError(f"{path} contains NUL")
    if any(0xD800 <= ord(character) <= 0xDFFF for character in value):
        raise ContractProfileError(f"{path} contains a Unicode surrogate")
    try:
        value.encode("utf-8")
    except UnicodeEncodeError as error:
        raise ContractProfileError(f"{path} is not a Unicode scalar string") from error


def validate_contract_value(value: object, *, path: str = "$") -> None:
    """Validate the recursively closed scalar profile used by parser contracts."""
    if value is None or isinstance(value, bool):
        return
    if isinstance(value, str):
        validate_unicode_scalar(value, path=path)
        return
    if type(value) is int:
        if abs(value) > MAX_SAFE_INTEGER:
            raise ContractProfileError(f"{path} integer exceeds the safe range")
        return
    if isinstance(value, float):
        raise ContractProfileError(f"{path} contains a forbidden float")
    if isinstance(value, list):
        _validate_contract_list(value, path)
        return
    if isinstance(value, dict):
        _validate_contract_object(value, path)
        return
    raise ContractProfileError(
        f"{path} contains unsupported value type {type(value).__name__}"
    )


def canonical_json_bytes(value: JsonValue) -> bytes:
    """Return RFC 8785 bytes after enforcing the narrower contract profile."""
    validate_contract_value(value)
    try:
        return rfc8785.dumps(value)
    except (TypeError, ValueError) as error:
        raise ContractProfileError("value cannot be RFC 8785 canonicalized") from error


def load_strict_json_bytes(content: bytes) -> JsonValue:
    """Load strict UTF-8 JSON only when its bytes are exact RFC 8785 JCS."""
    value = _decode_contract_json_bytes(content)
    if canonical_json_bytes(value) != content:
        raise ContractProfileError("JSON bytes are not canonical RFC 8785 JCS")
    return value


def _decode_contract_json_bytes(content: bytes) -> JsonValue:
    if not isinstance(content, bytes):
        raise TypeError("strict JSON input must be bytes")
    if any(content.startswith(bom) for bom in _BOMS):
        raise ContractProfileError("JSON bytes contain a BOM")
    if b"\x00" in content:
        raise ContractProfileError("JSON bytes contain NUL")
    try:
        text = content.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise ContractProfileError("JSON bytes are not strict UTF-8") from error
    try:
        value = json.loads(
            text,
            object_pairs_hook=_object_without_duplicates,
            parse_constant=_reject_json_constant,
            parse_float=_reject_json_float,
            parse_int=_parse_safe_integer,
        )
    except json.JSONDecodeError as error:
        raise ContractProfileError("bytes are not strict JSON") from error
    validate_contract_value(value)
    return cast("JsonValue", value)


def domain_separated_sha256(domain: str, value: JsonValue) -> str:
    """Hash ``ASCII(domain-with-terminal-LF) || JCS(value)`` with SHA-256."""
    validate_unicode_scalar(domain, path="$domain")
    domain_label = domain[:-1]
    if (
        not domain.endswith("\n")
        or not domain_label
        or not domain.isascii()
        or any(
            ord(character) < 0x21 or ord(character) > 0x7E for character in domain_label
        )
    ):
        raise ContractProfileError(
            "hash domain must be printable ASCII terminated by exactly one LF"
        )
    preimage = domain.encode("ascii") + canonical_json_bytes(value)
    return hashlib.sha256(preimage).hexdigest()


def validate_domain_hash_field(
    value: JsonObject,
    *,
    domain: str,
    hash_field: str,
) -> None:
    """Recompute one domain-separated object hash excluding its hash field."""
    expected = value.get(hash_field)
    if not isinstance(expected, str):
        raise ContractProfileError(f"{hash_field} must be a lowercase SHA-256")
    payload = value.copy()
    del payload[hash_field]
    observed = domain_separated_sha256(domain, payload)
    if observed != expected:
        raise ContractProfileError(f"{hash_field} does not match canonical payload")


def logical_hash_envelope_sha256(envelope: JsonObject) -> str:
    """Hash a logical envelope after omitting its in-band ``domain`` member."""
    validate_contract_value(envelope)
    domain = envelope.get("domain")
    if not isinstance(domain, str):
        raise ContractProfileError("logical hash envelope has no textual domain")
    preimage_envelope: JsonObject = {
        key: value for key, value in envelope.items() if key != "domain"
    }
    return domain_separated_sha256(domain, preimage_envelope)


def load_packaged_schemas() -> PackagedSchemas:
    """Load all allowlisted schemas and build a registry with no retriever."""
    names = schema_names()
    if not names or len(names) != len(set(names)):
        raise SchemaClosureError("packaged schema names must be non-empty and unique")

    by_name: dict[str, JsonObject] = {}
    by_id: dict[str, JsonObject] = {}
    registry: Registry[Any] = Registry()
    for name in names:
        value = _decode_contract_json_bytes(read_schema_bytes(name))
        if not isinstance(value, dict):
            raise SchemaClosureError(f"{name} is not a JSON object schema")
        schema = value
        schema_id = schema.get("$id")
        if not isinstance(schema_id, str):
            raise SchemaClosureError(f"{name} has no textual $id")
        if schema_id in by_id:
            raise SchemaClosureError(f"duplicate packaged schema $id: {schema_id}")
        try:
            Draft202012Validator.check_schema(schema)
        except SchemaError as error:
            raise SchemaClosureError(f"{name} is not a Draft 2020-12 schema") from error
        by_name[name] = schema
        by_id[schema_id] = schema
        registry = registry.with_resource(
            schema_id,
            DRAFT202012.create_resource(schema),
        )
    return PackagedSchemas(
        by_name=MappingProxyType(by_name),
        by_id=MappingProxyType(by_id),
        registry=registry,
    )


def assert_packaged_schema_closure(packaged: PackagedSchemas) -> None:
    """Prove IDs, references, and object shapes stay in the packaged graph."""
    for name, schema in packaged.by_name.items():
        expected_id = _assert_schema_identity(name, schema)
        for path, node, base_uri in _walk_schema(schema, expected_id):
            _assert_nested_schema_id(name, path, node, base_uri)
            _assert_closed_object_shape(name, path, node)
            for keyword in ("$ref", "$dynamicRef"):
                _assert_local_reference(
                    packaged,
                    base_uri,
                    node.get(keyword),
                    keyword,
                    f"{name} at {path}",
                )


def validator_for_schema(
    packaged: PackagedSchemas,
    schema_name: str,
) -> Draft202012Validator:
    """Create a Draft 2020-12 validator backed only by the local registry."""
    try:
        schema = packaged.by_name[schema_name]
    except KeyError as error:
        raise SchemaClosureError(f"unknown packaged schema: {schema_name}") from error
    return Draft202012Validator(
        schema,
        registry=packaged.registry,
        format_checker=FormatChecker(),
    )


def validate_schema_instance(
    packaged: PackagedSchemas,
    schema_name: str,
    instance: JsonValue,
) -> None:
    """Apply the contract profile before the selected offline JSON Schema."""
    validate_contract_value(instance)
    validator_for_schema(packaged, schema_name).validate(instance)


def _object_without_duplicates(pairs: list[tuple[str, Any]]) -> JsonObject:
    result: JsonObject = {}
    for key, value in pairs:
        if key in result:
            raise ContractProfileError(f"duplicate JSON object key: {key!r}")
        result[key] = cast("JsonValue", value)
    return result


def _validate_contract_list(value: list[object], path: str) -> None:
    for index, item in enumerate(value):
        validate_contract_value(item, path=f"{path}[{index}]")


def _validate_contract_object(value: dict[object, object], path: str) -> None:
    for key, item in value.items():
        if not isinstance(key, str):
            raise ContractProfileError(f"{path} has a non-string object key")
        validate_unicode_scalar(key, path=f"{path} object key")
        validate_contract_value(item, path=f"{path}.{key}")


def _reject_json_constant(value: str) -> Never:
    raise ContractProfileError(f"non-JSON numeric constant is forbidden: {value}")


def _reject_json_float(value: str) -> Never:
    raise ContractProfileError(f"JSON float is forbidden: {value}")


def _parse_safe_integer(value: str) -> int:
    parsed = int(value)
    if abs(parsed) > MAX_SAFE_INTEGER:
        raise ContractProfileError("JSON integer exceeds the safe range")
    return parsed


def _walk_schema(
    value: JsonValue,
    base_uri: str,
    path: str = "$",
) -> Iterator[tuple[str, JsonObject, str]]:
    if isinstance(value, dict):
        active_base = base_uri
        nested_id = value.get("$id")
        if isinstance(nested_id, str):
            active_base = urljoin(base_uri, nested_id)
        yield path, value, active_base
        for key, child in value.items():
            yield from _walk_schema(child, active_base, f"{path}/{_pointer_token(key)}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            yield from _walk_schema(child, base_uri, f"{path}/{index}")


def _assert_schema_identity(schema_name: str, schema: JsonObject) -> str:
    expected_id = f"{SCHEMA_ROOT}{schema_name}"
    if schema.get("$schema") != DRAFT_2020_12_ID:
        raise SchemaClosureError(f"{schema_name} does not declare Draft 2020-12")
    if schema.get("$id") != expected_id:
        raise SchemaClosureError(f"{schema_name} $id must equal {expected_id}")
    return expected_id


def _assert_nested_schema_id(
    schema_name: str,
    path: str,
    node: JsonObject,
    base_uri: str,
) -> None:
    if not isinstance(node.get("$id"), str):
        return
    absolute_id, _fragment = urldefrag(base_uri)
    if not absolute_id.startswith(SCHEMA_ROOT):
        raise SchemaClosureError(f"{schema_name} has an out-of-root $id at {path}")


def _assert_local_reference(
    packaged: PackagedSchemas,
    base_uri: str,
    reference: object,
    keyword: str,
    label: str,
) -> None:
    if reference is None:
        return
    if not isinstance(reference, str):
        raise SchemaClosureError(f"{label} has a non-string {keyword}")
    absolute_reference, _fragment = urldefrag(urljoin(base_uri, reference))
    if not absolute_reference.startswith(SCHEMA_ROOT):
        raise SchemaClosureError(f"{label} has an external {keyword}")
    try:
        packaged.registry.resolver(base_uri).lookup(reference)
    except Unresolvable as error:
        raise SchemaClosureError(f"{label} has an unresolved {keyword}") from error


def _assert_closed_object_shape(
    schema_name: str,
    path: str,
    node: JsonObject,
) -> None:
    declared_type = node.get("type")
    object_typed = declared_type == "object" or (
        isinstance(declared_type, list) and "object" in declared_type
    )
    if not object_typed:
        return
    if (
        node.get("additionalProperties") is not False
        and node.get("unevaluatedProperties") is not False
    ):
        raise SchemaClosureError(f"{schema_name} has an open object shape at {path}")


def _pointer_token(value: str) -> str:
    return value.replace("~", "~0").replace("/", "~1")
