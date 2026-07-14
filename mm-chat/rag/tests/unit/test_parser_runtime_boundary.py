"""Regression gates for the C1.1 contract-only runtime boundary."""

from __future__ import annotations

import json
import subprocess
import sys

import pytest

from mm_chat_rag import handlers
from mm_chat_rag.contracts import read_schema_bytes, schema_names


def test_contract_resources_do_not_enable_runtime_registries() -> None:
    assert handlers.DISPATCH_REGISTRY == {}
    assert handlers.JOB_HANDLER_REGISTRY == {}

    names = schema_names()

    assert names == tuple(sorted(names))
    assert len(names) == len(set(names))
    assert all(read_schema_bytes(name) for name in names)
    assert handlers.DISPATCH_REGISTRY == {}
    assert handlers.JOB_HANDLER_REGISTRY == {}


def test_contract_resource_reader_rejects_paths_and_unknown_names() -> None:
    for name in (
        "../canonical-ir.v2.schema.json",
        "/etc/passwd",
        "provider-contract-v1.schema.json",
        "",
    ):
        with pytest.raises(ValueError, match="unknown parser contract schema"):
            read_schema_bytes(name)


def test_contract_package_has_no_test_only_runtime_imports() -> None:
    command = (
        "import json,sys; import mm_chat_rag.contracts; "
        "print(json.dumps(sorted(set(sys.modules) & {'jsonschema','rfc8785'})))"
    )
    completed = subprocess.run(  # noqa: S603
        [sys.executable, "-I", "-c", command],
        check=False,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0, completed.stderr
    assert json.loads(completed.stdout) == []
