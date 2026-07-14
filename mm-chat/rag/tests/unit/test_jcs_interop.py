# ruff: noqa: ANN401
from __future__ import annotations

import hashlib
import json
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any

import pytest
import rfc8785
from jsonschema import Draft202012Validator, FormatChecker
from tools import verify_jcs_interop

_RAG_ROOT = Path(__file__).resolve().parents[2]
_FIXTURES = _RAG_ROOT / "tests" / "fixtures" / "jcs"
_INTEROP = _RAG_ROOT / "tests" / "interop" / "jcs"
_RUNNER = _RAG_ROOT / "tools" / "verify_jcs_interop.py"
_MANIFESTS = ("c1-contract-profile-v1.json", "rfc8785-v1.json")
_LOGICAL_MANIFEST = "logical-hash-golden-v1.json"
_LOGICAL_GOLDEN = (
    _RAG_ROOT
    / "tests"
    / "fixtures"
    / "parser_contracts"
    / "logical_hash"
    / "golden-v1.json"
)
_SCHEMA_ROOT = "https://schemas.mm-chat.invalid/parser/"


def _load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_bytes())
    assert isinstance(value, dict)
    return value


def _assert_all_object_shapes_closed(value: Any, location: str = "$") -> None:
    if isinstance(value, dict):
        if value.get("type") == "object":
            assert value.get("additionalProperties") is False, location
        for key, item in value.items():
            _assert_all_object_shapes_closed(item, f"{location}.{key}")
    elif isinstance(value, list):
        for index, item in enumerate(value):
            _assert_all_object_shapes_closed(item, f"{location}[{index}]")


def _assert_contract_value(value: Any) -> None:
    if value is None or isinstance(value, (bool, str)):
        if isinstance(value, str):
            assert "\x00" not in value
            assert not any(0xD800 <= ord(character) <= 0xDFFF for character in value)
        return
    if isinstance(value, int):
        assert abs(value) <= (1 << 53) - 1
        return
    if isinstance(value, list):
        for item in value:
            _assert_contract_value(item)
        return
    assert isinstance(value, dict)
    for key, item in value.items():
        _assert_contract_value(key)
        _assert_contract_value(item)


def test_vector_and_summary_schemas_are_draft_202012_and_closed() -> None:
    for filename in (
        "jcs-logical-hash-manifest-v1.schema.json",
        "jcs-vector-manifest-v1.schema.json",
        "jcs-gate-summary-v1.schema.json",
    ):
        raw = (_FIXTURES / filename).read_bytes()
        schema = json.loads(raw)
        assert rfc8785.dumps(schema) == raw
        _assert_contract_value(schema)
        assert schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
        assert schema["$id"].startswith(_SCHEMA_ROOT)
        Draft202012Validator.check_schema(schema)
        _assert_all_object_shapes_closed(schema)


def test_manifests_are_canonical_schema_valid_and_hash_bound() -> None:
    schema = _load_json(_FIXTURES / "jcs-vector-manifest-v1.schema.json")
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    for filename in _MANIFESTS:
        raw = (_FIXTURES / filename).read_bytes()
        manifest = json.loads(raw)
        validator.validate(manifest)
        assert rfc8785.dumps(manifest) == raw
        _assert_contract_value(manifest)
        cases_bytes = rfc8785.dumps(manifest["cases"])
        fixture_hash = hashlib.sha256(cases_bytes).hexdigest()
        assert manifest["fixtureSetSha256"] == fixture_hash
        assert manifest["provenance"]["materialSha256"] == fixture_hash
        for test_case in manifest["cases"]:
            input_field = "inputHex" if test_case["kind"] == "json" else "ieee754Hex"
            input_bytes = bytes.fromhex(test_case[input_field])
            assert hashlib.sha256(input_bytes).hexdigest() == test_case["inputSha256"]
            if test_case["expect"] == "accept":
                expected = bytes.fromhex(test_case["expectedHex"])
                assert (
                    hashlib.sha256(expected).hexdigest() == test_case["expectedSha256"]
                )


def test_logical_manifest_loads_canonical_golden_without_copying_vectors() -> None:
    schema = _load_json(_FIXTURES / "jcs-logical-hash-manifest-v1.schema.json")
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    manifest_raw = (_FIXTURES / _LOGICAL_MANIFEST).read_bytes()
    manifest = json.loads(manifest_raw)
    validator.validate(manifest)
    assert rfc8785.dumps(manifest) == manifest_raw
    assert "vectors" not in manifest

    source_path = (_FIXTURES / manifest["provenance"]["sourcePath"]).resolve()
    assert source_path == _LOGICAL_GOLDEN.resolve()
    source_raw = source_path.read_bytes()
    assert (
        hashlib.sha256(source_raw).hexdigest()
        == manifest["provenance"]["materialSha256"]
    )
    golden = json.loads(source_raw)
    assert rfc8785.dumps(golden) == source_raw
    assert golden["algorithm"] == "sha-256"
    assert golden["framing"] == (
        "ASCII(domain-with-one-terminal-LF) || RFC8785(envelopeWithoutDomain)"
    )
    assert len(golden["vectors"]) == manifest["caseCount"] == 24
    _assert_contract_value(golden)

    names: set[str] = set()
    for vector in golden["vectors"]:
        domain = vector["domain"].encode("ascii", errors="strict")
        assert domain.endswith(b"\n")
        assert domain.count(b"\n") == 1
        assert vector["name"] not in names
        names.add(vector["name"])
        assert "domain" not in vector["envelopeWithoutDomain"]
        assert vector["envelopeWithoutDomain"]["envelopeKind"] == vector["name"]
        framed = domain + rfc8785.dumps(vector["envelopeWithoutDomain"])
        assert hashlib.sha256(framed).hexdigest() == vector["expectedSha256"]


def test_contract_profile_is_float_free_and_separate_from_rfc_vectors() -> None:
    contract = _load_json(_FIXTURES / _MANIFESTS[0])
    rfc = _load_json(_FIXTURES / _MANIFESTS[1])

    assert contract["profile"] == "c1-contract-profile"
    assert rfc["profile"] == "rfc8785"
    assert all(test_case["kind"] == "json" for test_case in contract["cases"])
    assert any(test_case["kind"] == "ieee754" for test_case in rfc["cases"])
    rejection_codes = {
        test_case["reasonCode"]
        for test_case in contract["cases"]
        if test_case["expect"] == "reject"
    }
    assert rejection_codes >= {
        "BOM_FORBIDDEN",
        "DUPLICATE_KEY",
        "FLOAT_FORBIDDEN",
        "NON_CANONICAL",
        "NUL_FORBIDDEN",
        "SURROGATE_FORBIDDEN",
        "UNSAFE_INTEGER",
    }
    for test_case in contract["cases"]:
        if test_case["expect"] == "accept":
            _assert_contract_value(json.loads(bytes.fromhex(test_case["inputHex"])))


def test_rfc_vectors_retain_upstream_provenance_and_license() -> None:
    rfc = _load_json(_FIXTURES / "rfc8785-v1.json")
    provenance = rfc["provenance"]
    assert provenance["source"] == "RFC 8785 JSON Canonicalization Scheme"
    assert provenance["revision"].startswith("RFC 8785 (June 2020)")
    assert provenance["licenseFile"] == "LICENSE-IETF-RFC8785.txt"
    assert "IETF Trust" in provenance["license"]
    license_text = (_FIXTURES / provenance["licenseFile"]).read_text(encoding="utf-8")
    assert "They are not\nMM Chat MIT-licensed assets." in license_text
    assert "Copyright (c) 2020 IETF Trust" in license_text
    assert "Simplified BSD" in license_text


def test_require_all_gate_passes_with_ascii_canonical_summary() -> None:
    completed = subprocess.run(  # noqa: S603 - fixed checked-in gate command.
        [sys.executable, "-B", str(_RUNNER), "--require-all"],
        cwd=_RAG_ROOT,
        check=False,
        capture_output=True,
        timeout=120,
    )
    assert completed.returncode == 0
    assert completed.stderr == b""
    completed.stdout.decode("ascii", errors="strict")
    assert completed.stdout.endswith(b"\n")
    assert completed.stdout.count(b"\n") == 1
    summary = json.loads(completed.stdout)
    expected = json.dumps(
        summary,
        ensure_ascii=True,
        allow_nan=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("ascii")
    assert completed.stdout == expected + b"\n"
    assert summary["status"] == "pass"
    assert summary["requireAll"] is True
    assert summary["caseCount"] == 89
    assert summary["suiteCount"] == 3
    assert (
        summary["logicalGoldenRawSha256"]
        == hashlib.sha256(_LOGICAL_GOLDEN.read_bytes()).hexdigest()
    )
    assert [runtime["name"] for runtime in summary["runtimes"]] == [
        "python",
        "go",
        "node",
    ]
    assert {runtime["resultSha256"] for runtime in summary["runtimes"]} == {
        summary["resultSha256"]
    }
    assert {runtime["logicalGoldenRawSha256"] for runtime in summary["runtimes"]} == {
        summary["logicalGoldenRawSha256"]
    }
    summary_schema = _load_json(_FIXTURES / "jcs-gate-summary-v1.schema.json")
    Draft202012Validator(summary_schema).validate(summary)


def test_require_all_fails_closed_when_go_and_node_are_missing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr("tools.verify_jcs_interop.shutil.which", lambda _name: None)
    summary = verify_jcs_interop.verify(_FIXTURES, require_all=True)
    assert summary["status"] == "fail"
    assert summary["runtimes"][0]["status"] == "pass"
    assert [runtime["status"] for runtime in summary["runtimes"][1:]] == [
        "skipped",
        "skipped",
    ]


def test_require_all_gate_fails_on_one_changed_expected_logical_hash(
    tmp_path: Path,
) -> None:
    fixtures = tmp_path / "fixtures" / "jcs"
    shutil.copytree(_FIXTURES, fixtures)
    manifest_path = fixtures / _LOGICAL_MANIFEST
    manifest = _load_json(manifest_path)
    logical_golden = fixtures / manifest["provenance"]["sourcePath"]
    logical_golden.parent.mkdir(parents=True)
    shutil.copy2(_LOGICAL_GOLDEN, logical_golden)

    golden = _load_json(logical_golden)
    golden["vectors"][0]["expectedSha256"] = "0" * 64
    changed_source = rfc8785.dumps(golden)
    logical_golden.write_bytes(changed_source)
    manifest["provenance"]["materialSha256"] = hashlib.sha256(
        changed_source
    ).hexdigest()
    manifest_path.write_bytes(rfc8785.dumps(manifest))

    completed = subprocess.run(  # noqa: S603 - fixed checked-in gate command.
        [
            sys.executable,
            "-B",
            str(_RUNNER),
            "--fixtures",
            str(fixtures),
            "--require-all",
        ],
        cwd=_RAG_ROOT,
        check=False,
        capture_output=True,
        timeout=120,
    )
    assert completed.returncode == 1
    assert completed.stderr == b""
    summary = json.loads(completed.stdout)
    assert summary["status"] == "fail"
    assert [runtime["status"] for runtime in summary["runtimes"]] == [
        "fail",
        "fail",
        "fail",
    ]
    assert {runtime["error"] for runtime in summary["runtimes"]} == {
        "LOGICAL_HASH_MISMATCH"
    }


def test_gate_never_strips_expected_bytes(tmp_path: Path) -> None:
    fixtures = tmp_path / "jcs"
    shutil.copytree(_FIXTURES, fixtures)
    manifest_path = fixtures / "c1-contract-profile-v1.json"
    manifest = _load_json(manifest_path)
    test_case = manifest["cases"][0]
    expected = bytes.fromhex(test_case["expectedHex"]) + b"\n"
    test_case["expectedHex"] = expected.hex()
    test_case["expectedSha256"] = hashlib.sha256(expected).hexdigest()
    fixture_hash = hashlib.sha256(rfc8785.dumps(manifest["cases"])).hexdigest()
    manifest["fixtureSetSha256"] = fixture_hash
    manifest["provenance"]["materialSha256"] = fixture_hash
    manifest_path.write_bytes(rfc8785.dumps(manifest))

    completed = subprocess.run(  # noqa: S603 - fixed checked-in runtime command.
        [
            sys.executable,
            "-B",
            str(_INTEROP / "python_jcs.py"),
            "--fixtures",
            str(fixtures),
        ],
        cwd=_RAG_ROOT,
        check=False,
        capture_output=True,
        timeout=30,
    )
    assert completed.returncode == 1
    assert completed.stderr == b""
    summary = json.loads(completed.stdout)
    assert summary["error"] == "CANONICAL_BYTES_MISMATCH"
    assert summary["failedCase"] == "literal-null"


def test_runtime_commands_are_dependency_free_and_go_is_forced_offline() -> None:
    environment = verify_jcs_interop._environment()
    assert environment["GOTOOLCHAIN"] == "local"
    assert environment["GOPROXY"] == "off"
    assert environment["GOSUMDB"] == "off"
    assert environment["GOENV"] == "off"
    assert not any("proxy" in name.lower() for name in environment if name != "GOPROXY")

    sources = {
        "python": (_INTEROP / "python_jcs.py").read_text(encoding="utf-8"),
        "go": (_INTEROP / "go_jcs.go").read_text(encoding="utf-8"),
        "node": (_INTEROP / "node_jcs.mjs").read_text(encoding="utf-8"),
        "runner": _RUNNER.read_text(encoding="utf-8"),
    }
    for source in sources.values():
        assert "pip install" not in source
        assert "npm install" not in source
        assert "go get" not in source
    assert '"net"' not in sources["go"]
    assert '"net/http"' not in sources["go"]
    assert "node:http" not in sources["node"]
    assert "fetch(" not in sources["node"]
    assert "import socket" not in sources["python"]
    assert "import urllib" not in sources["runner"]
