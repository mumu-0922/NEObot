"""Read allowlisted parser schemas from installed package resources."""

from __future__ import annotations

from importlib.resources import files
from typing import Final

_SCHEMA_PACKAGE: Final = "mm_chat_rag.contracts.schemas"
_SCHEMA_NAMES: Final = (
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
_SCHEMA_NAME_SET: Final = frozenset(_SCHEMA_NAMES)


def schema_names() -> tuple[str, ...]:
    """Return the complete immutable schema-resource allowlist."""
    return _SCHEMA_NAMES


def read_schema_bytes(name: str) -> bytes:
    """Read one allowlisted schema without accepting caller-controlled paths."""
    if name not in _SCHEMA_NAME_SET:
        raise ValueError("unknown parser contract schema")
    return files(_SCHEMA_PACKAGE).joinpath(name).read_bytes()
