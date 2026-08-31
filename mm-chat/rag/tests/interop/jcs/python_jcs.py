#!/usr/bin/env python3
# ruff: noqa: ANN401, BLE001, PLR0911, TRY300
"""Independent, standard-library-only JCS fixture implementation."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import struct
import sys
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

_MAX_SAFE_INTEGER = (1 << 53) - 1
_MANIFESTS = ("c1-contract-profile-v1.json", "rfc8785-v1.json")
_LOGICAL_MANIFEST = "logical-hash-golden-v1.json"
_LOGICAL_SOURCE_PATH = "../parser_contracts/logical_hash/golden-v1.json"
_LOGICAL_CASE_COUNT = 24
_LOGICAL_FRAMING = (
    "ASCII(domain-with-one-terminal-LF) || RFC8785(envelopeWithoutDomain)"
)
_HEX = frozenset("0123456789abcdef")
_BOMS = (
    b"\x00\x00\xfe\xff",
    b"\xff\xfe\x00\x00",
    b"\xef\xbb\xbf",
    b"\xfe\xff",
    b"\xff\xfe",
)


class ConformanceError(RuntimeError):
    """Stable fixture or conformance failure."""

    def __init__(self, code: str, case_id: str = "") -> None:
        super().__init__(code)
        self.code = code
        self.case_id = case_id


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _normalize_scalar_string(value: str, *, reject_nul: bool) -> str:
    output: list[str] = []
    index = 0
    while index < len(value):
        codepoint = ord(value[index])
        if codepoint == 0 and reject_nul:
            raise ConformanceError("NUL_FORBIDDEN")
        if 0xD800 <= codepoint <= 0xDBFF:
            if index + 1 >= len(value):
                raise ConformanceError("SURROGATE_FORBIDDEN")
            low = ord(value[index + 1])
            if not 0xDC00 <= low <= 0xDFFF:
                raise ConformanceError("SURROGATE_FORBIDDEN")
            output.append(chr(0x10000 + ((codepoint - 0xD800) << 10) + low - 0xDC00))
            index += 2
            continue
        if 0xDC00 <= codepoint <= 0xDFFF:
            raise ConformanceError("SURROGATE_FORBIDDEN")
        output.append(value[index])
        index += 1
    return "".join(output)


def _normalize_strings(value: Any, *, reject_nul: bool) -> Any:
    if isinstance(value, str):
        return _normalize_scalar_string(value, reject_nul=reject_nul)
    if isinstance(value, list):
        return [_normalize_strings(item, reject_nul=reject_nul) for item in value]
    if isinstance(value, dict):
        normalized: dict[str, Any] = {}
        for key, item in value.items():
            normalized_key = _normalize_scalar_string(key, reject_nul=reject_nul)
            if normalized_key in normalized:
                raise ConformanceError("DUPLICATE_KEY")
            normalized[normalized_key] = _normalize_strings(
                item,
                reject_nul=reject_nul,
            )
        return normalized
    return value


def _parse_json(raw: bytes, *, profile: str) -> Any:
    if any(raw.startswith(bom) for bom in _BOMS):
        raise ConformanceError("BOM_FORBIDDEN")
    if b"\x00" in raw:
        raise ConformanceError("NUL_FORBIDDEN")
    try:
        text = raw.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise ConformanceError("JSON_INVALID") from error
    reject_nul = profile == "c1-contract-profile"

    def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            normalized = _normalize_scalar_string(key, reject_nul=reject_nul)
            if normalized in result:
                raise ConformanceError("DUPLICATE_KEY")
            result[normalized] = value
        return result

    def contract_integer(token: str) -> int:
        value = int(token)
        if abs(value) > _MAX_SAFE_INTEGER:
            raise ConformanceError("UNSAFE_INTEGER")
        return value

    def reject_float(_token: str) -> float:
        raise ConformanceError("FLOAT_FORBIDDEN")

    def reject_constant(_token: str) -> None:
        raise ConformanceError("JSON_INVALID")

    try:
        if profile == "c1-contract-profile":
            parsed = json.loads(
                text,
                object_pairs_hook=unique_object,
                parse_constant=reject_constant,
                parse_float=reject_float,
                parse_int=contract_integer,
            )
        else:
            parsed = json.loads(
                text,
                object_pairs_hook=unique_object,
                parse_constant=reject_constant,
                parse_float=float,
                parse_int=float,
            )
    except ConformanceError:
        raise
    except (json.JSONDecodeError, ValueError) as error:
        raise ConformanceError("JSON_INVALID") from error
    return _normalize_strings(parsed, reject_nul=reject_nul)


def _serialize_string(value: str) -> bytes:
    value = _normalize_scalar_string(value, reject_nul=False)
    output = bytearray(b'"')
    short_escapes = {
        0x08: b"\\b",
        0x09: b"\\t",
        0x0A: b"\\n",
        0x0C: b"\\f",
        0x0D: b"\\r",
        0x22: b'\\"',
        0x5C: b"\\\\",
    }
    for character in value:
        codepoint = ord(character)
        escaped = short_escapes.get(codepoint)
        if escaped is not None:
            output.extend(escaped)
        elif codepoint <= 0x1F:
            output.extend(f"\\u{codepoint:04x}".encode("ascii"))
        else:
            output.extend(character.encode("utf-8"))
    output.extend(b'"')
    return bytes(output)


def _serialize_float(value: float) -> bytes:
    if not math.isfinite(value):
        raise ConformanceError("NON_FINITE")
    if value == 0:
        return b"0"
    if value < 0:
        return b"-" + _serialize_float(-value)

    rendered = repr(value)
    exponent_text = ""
    exponent_value = 0
    exponent_at = rendered.find("e")
    if exponent_at > 0:
        exponent_text = rendered[exponent_at:]
        sign_at = 2 if exponent_text[1:2] in {"+", "-"} else 1
        while len(exponent_text) > sign_at + 1 and exponent_text[sign_at] == "0":
            exponent_text = exponent_text[:sign_at] + exponent_text[sign_at + 1 :]
        rendered = rendered[:exponent_at]
        exponent_value = int(exponent_text[1:])

    if "." in rendered:
        first, last = rendered.split(".", maxsplit=1)
        dot = "."
    else:
        first, last, dot = rendered, "", ""
    if last == "0":
        last, dot = "", ""

    if 0 < exponent_value < 21:
        first += last
        last, dot, exponent_text = "", "", ""
        first += "0" * max(0, exponent_value - len(first) + 1)
    elif -7 < exponent_value < 0:
        last = "0" * (-exponent_value - 1) + first + last
        first, dot, exponent_text = "0", ".", ""
    return f"{first}{dot}{last}{exponent_text}".encode("ascii")


def _canonicalize(value: Any) -> bytes:
    if value is None:
        return b"null"
    if value is True:
        return b"true"
    if value is False:
        return b"false"
    if isinstance(value, int):
        if abs(value) > _MAX_SAFE_INTEGER:
            raise ConformanceError("UNSAFE_INTEGER")
        return str(value).encode("ascii")
    if isinstance(value, float):
        return _serialize_float(value)
    if isinstance(value, str):
        return _serialize_string(value)
    if isinstance(value, list):
        return b"[" + b",".join(_canonicalize(item) for item in value) + b"]"
    if isinstance(value, dict):
        try:
            keys = sorted(value, key=lambda key: key.encode("utf-16-be"))
        except (AttributeError, UnicodeEncodeError) as error:
            raise ConformanceError("JSON_INVALID") from error
        members = (
            _serialize_string(key) + b":" + _canonicalize(value[key]) for key in keys
        )
        return b"{" + b",".join(members) + b"}"
    raise ConformanceError("JSON_INVALID")


def _expect_object(value: Any, fields: set[str]) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != fields:
        raise ConformanceError("MANIFEST_INVALID")
    return value


def _expect_text(value: Any, *, ascii_only: bool = False) -> str:
    if not isinstance(value, str) or not value:
        raise ConformanceError("MANIFEST_INVALID")
    if ascii_only and (not value.isascii() or any(ord(item) < 0x20 for item in value)):
        raise ConformanceError("MANIFEST_INVALID")
    return value


def _expect_sha256(value: Any) -> str:
    text = _expect_text(value, ascii_only=True)
    if len(text) != 64 or any(item not in _HEX for item in text):
        raise ConformanceError("MANIFEST_INVALID")
    return text


def _decode_hex(value: Any) -> bytes:
    text = _expect_text(value, ascii_only=True)
    if len(text) % 2 or any(item not in _HEX for item in text):
        raise ConformanceError("MANIFEST_INVALID")
    try:
        return bytes.fromhex(text)
    except ValueError as error:
        raise ConformanceError("MANIFEST_INVALID") from error


def _validate_case(case: Any, profile: str) -> dict[str, Any]:
    if not isinstance(case, dict):
        raise ConformanceError("MANIFEST_INVALID")
    kind = case.get("kind")
    expect = case.get("expect")
    common = {"caseId", "kind", "expect", "inputSha256"}
    if kind == "json":
        common.add("inputHex")
    elif kind == "ieee754" and profile == "rfc8785":
        common.add("ieee754Hex")
    else:
        raise ConformanceError("MANIFEST_INVALID")
    if expect == "accept":
        common.update({"expectedHex", "expectedSha256"})
    elif expect == "reject":
        common.add("reasonCode")
    else:
        raise ConformanceError("MANIFEST_INVALID")
    _expect_object(case, common)
    _expect_text(case["caseId"], ascii_only=True)
    _expect_sha256(case["inputSha256"])
    if kind == "json":
        input_bytes = _decode_hex(case["inputHex"])
    else:
        input_bytes = _decode_hex(case["ieee754Hex"])
        if len(input_bytes) != 8:
            raise ConformanceError("MANIFEST_INVALID")
    if _sha256(input_bytes) != case["inputSha256"]:
        raise ConformanceError("FIXTURE_HASH_MISMATCH")
    if expect == "accept":
        expected = _decode_hex(case["expectedHex"])
        _expect_sha256(case["expectedSha256"])
        if _sha256(expected) != case["expectedSha256"]:
            raise ConformanceError("FIXTURE_HASH_MISMATCH")
    else:
        _expect_text(case["reasonCode"], ascii_only=True)
    return case


def _load_manifest(path: Path) -> dict[str, Any]:
    raw = path.read_bytes()
    parsed = _parse_json(raw, profile="c1-contract-profile")
    if _canonicalize(parsed) != raw:
        raise ConformanceError("MANIFEST_NOT_CANONICAL")
    manifest = _expect_object(
        parsed,
        {
            "schemaVersion",
            "suiteId",
            "profile",
            "provenance",
            "fixtureSetSha256",
            "cases",
        },
    )
    if manifest["schemaVersion"] != "mm-chat.jcs-vector-manifest.v1":
        raise ConformanceError("MANIFEST_INVALID")
    profile = _expect_text(manifest["profile"], ascii_only=True)
    if profile not in {"c1-contract-profile", "rfc8785"}:
        raise ConformanceError("MANIFEST_INVALID")
    _expect_text(manifest["suiteId"], ascii_only=True)
    provenance = _expect_object(
        manifest["provenance"],
        {
            "source",
            "sourceUrl",
            "revision",
            "materialSha256",
            "license",
            "licenseFile",
        },
    )
    for field in ("source", "sourceUrl", "revision", "license", "licenseFile"):
        _expect_text(provenance[field])
    cases = manifest["cases"]
    if not isinstance(cases, list) or not cases:
        raise ConformanceError("MANIFEST_INVALID")
    validated = [_validate_case(case, profile) for case in cases]
    fixture_hash = _sha256(_canonicalize(validated))
    if fixture_hash != _expect_sha256(manifest["fixtureSetSha256"]):
        raise ConformanceError("FIXTURE_SET_HASH_MISMATCH")
    if fixture_hash != _expect_sha256(provenance["materialSha256"]):
        raise ConformanceError("PROVENANCE_HASH_MISMATCH")
    return manifest


def _expect_logical_name(value: Any) -> str:
    name = _expect_text(value, ascii_only=True)
    if any(ord(character) < 0x21 or ord(character) > 0x7E for character in name):
        raise ConformanceError("LOGICAL_GOLDEN_INVALID")
    return name


def _expect_logical_domain(value: Any) -> bytes:
    if not isinstance(value, str):
        raise ConformanceError("LOGICAL_GOLDEN_INVALID")
    try:
        encoded = value.encode("ascii", errors="strict")
    except UnicodeEncodeError as error:
        raise ConformanceError("LOGICAL_GOLDEN_INVALID") from error
    if (
        not encoded.endswith(b"\n")
        or encoded.count(b"\n") != 1
        or not encoded[:-1]
        or any(character < 0x21 or character > 0x7E for character in encoded[:-1])
    ):
        raise ConformanceError("LOGICAL_GOLDEN_INVALID")
    return encoded


def _load_logical_suite(
    fixtures: Path,
) -> tuple[str, str, list[tuple[str, str]]]:
    manifest_raw = (fixtures / _LOGICAL_MANIFEST).read_bytes()
    manifest = _parse_json(manifest_raw, profile="c1-contract-profile")
    if _canonicalize(manifest) != manifest_raw:
        raise ConformanceError("MANIFEST_NOT_CANONICAL")
    manifest = _expect_object(
        manifest,
        {"caseCount", "profile", "provenance", "schemaVersion", "suiteId"},
    )
    if (
        manifest["schemaVersion"] != "mm-chat.jcs-logical-hash-manifest.v1"
        or manifest["suiteId"] != "logical-hash-golden-v1"
        or manifest["profile"] != "c1-contract-profile"
        or type(manifest["caseCount"]) is not int
        or manifest["caseCount"] != _LOGICAL_CASE_COUNT
    ):
        raise ConformanceError("MANIFEST_INVALID")
    provenance = _expect_object(
        manifest["provenance"],
        {
            "license",
            "licenseFile",
            "materialSha256",
            "revision",
            "source",
            "sourcePath",
        },
    )
    for field in ("license", "licenseFile", "revision", "source"):
        _expect_text(provenance[field])
    if (
        provenance["sourcePath"] != _LOGICAL_SOURCE_PATH
        or provenance["licenseFile"] != "README.md"
    ):
        raise ConformanceError("MANIFEST_INVALID")

    source_raw = (fixtures / _LOGICAL_SOURCE_PATH).read_bytes()
    source_hash = _sha256(source_raw)
    if source_hash != _expect_sha256(provenance["materialSha256"]):
        raise ConformanceError("PROVENANCE_HASH_MISMATCH")
    golden = _parse_json(source_raw, profile="c1-contract-profile")
    if _canonicalize(golden) != source_raw:
        raise ConformanceError("LOGICAL_GOLDEN_NOT_CANONICAL")
    golden = _expect_object(golden, {"algorithm", "framing", "vectors"})
    if golden["algorithm"] != "sha-256" or golden["framing"] != _LOGICAL_FRAMING:
        raise ConformanceError("LOGICAL_GOLDEN_INVALID")
    raw_vectors = golden["vectors"]
    if not isinstance(raw_vectors, list) or len(raw_vectors) != _LOGICAL_CASE_COUNT:
        raise ConformanceError("LOGICAL_GOLDEN_INVALID")

    results: list[tuple[str, str]] = []
    observed_names: set[str] = set()
    for raw_vector in raw_vectors:
        vector = _expect_object(
            raw_vector,
            {"domain", "envelopeWithoutDomain", "expectedSha256", "name"},
        )
        name = _expect_logical_name(vector["name"])
        if name in observed_names:
            raise ConformanceError("LOGICAL_GOLDEN_INVALID")
        observed_names.add(name)
        domain = _expect_logical_domain(vector["domain"])
        expected = _expect_sha256(vector["expectedSha256"])
        envelope = vector["envelopeWithoutDomain"]
        if (
            not isinstance(envelope, dict)
            or "domain" in envelope
            or envelope.get("envelopeKind") != name
        ):
            raise ConformanceError("LOGICAL_GOLDEN_INVALID")
        actual = _sha256(domain + _canonicalize(envelope))
        if actual != expected:
            raise ConformanceError("LOGICAL_HASH_MISMATCH", name)
        results.append((name, actual))
    return str(manifest["suiteId"]), source_hash, results


def _execute_case(case: Mapping[str, Any], profile: str) -> tuple[str, str]:
    case_id = str(case["caseId"])
    try:
        if case["kind"] == "json":
            raw = bytes.fromhex(str(case["inputHex"]))
            value = _parse_json(raw, profile=profile)
            canonical = _canonicalize(value)
            if profile == "c1-contract-profile" and canonical != raw:
                raise ConformanceError("NON_CANONICAL")
        else:
            raw = bytes.fromhex(str(case["ieee754Hex"]))
            canonical = _serialize_float(struct.unpack(">d", raw)[0])
        if case["expect"] == "reject":
            raise ConformanceError("UNEXPECTED_ACCEPT")
        expected = bytes.fromhex(str(case["expectedHex"]))
        if canonical != expected:
            raise ConformanceError("CANONICAL_BYTES_MISMATCH")
        canonical_hash = _sha256(canonical)
        if canonical_hash != case["expectedSha256"]:
            raise ConformanceError("CANONICAL_HASH_MISMATCH")
        return "accept", canonical_hash
    except ConformanceError as error:
        if case["expect"] == "reject" and error.code == case["reasonCode"]:
            return "reject", error.code
        raise ConformanceError(error.code, case_id) from error


def run(fixtures: Path) -> dict[str, Any]:
    """Run the three fixed suites and return an ASCII-only result object."""
    transcript = bytearray()
    case_count = 0
    for filename in _MANIFESTS:
        manifest = _load_manifest(fixtures / filename)
        profile = str(manifest["profile"])
        suite_id = str(manifest["suiteId"])
        for case in manifest["cases"]:
            outcome, digest = _execute_case(case, profile)
            transcript.extend(
                f"{suite_id}\0{case['caseId']}\0{outcome}\0{digest}\n".encode("ascii")
            )
            case_count += 1
    logical_suite_id, logical_raw_hash, logical_results = _load_logical_suite(fixtures)
    for name, digest in logical_results:
        transcript.extend(
            f"{logical_suite_id}\0{name}\0accept\0{digest}\n".encode("ascii")
        )
        case_count += 1
    return {
        "caseCount": case_count,
        "implementation": "python",
        "logicalGoldenRawSha256": logical_raw_hash,
        "resultSha256": _sha256(bytes(transcript)),
        "status": "pass",
        "suiteCount": len(_MANIFESTS) + 1,
        "version": ".".join(str(item) for item in sys.version_info[:3]),
    }


def _write_summary(summary: Mapping[str, Any]) -> None:
    encoded = _canonicalize(dict(summary)) + b"\n"
    encoded.decode("ascii", errors="strict")
    sys.stdout.buffer.write(encoded)


def main(argv: Sequence[str] | None = None) -> int:
    """Run the fixed Python conformance implementation."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixtures", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        _write_summary(run(args.fixtures.resolve(strict=True)))
        return 0
    except ConformanceError as error:
        _write_summary(
            {
                "error": error.code,
                "failedCase": error.case_id,
                "implementation": "python",
                "status": "fail",
                "version": ".".join(str(item) for item in sys.version_info[:3]),
            }
        )
        return 1
    except Exception:
        _write_summary(
            {
                "error": "INTERNAL_ERROR",
                "failedCase": "",
                "implementation": "python",
                "status": "fail",
                "version": ".".join(str(item) for item in sys.version_info[:3]),
            }
        )
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
