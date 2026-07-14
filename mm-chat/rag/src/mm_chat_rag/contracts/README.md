# Offline Parser Contracts

Versioned, closed JSON Schemas for the Phase 15.2C offline parser boundary.
They freeze Canonical IR v2, Source Locator v2, normalization, quality, chunk,
manifest, profile, protocol, corpus, and stable-error shapes before parser code
is allowed to depend on them.

## Boundaries

- Resources are package data read through `importlib.resources` and a fixed
  filename allowlist.
- `resources.py` is standard-library only. `jsonschema` and `rfc8785` remain
  test-only dependencies.
- The schemas do not import Postgres, Redis, MinIO, Provider clients, worker
  handlers, or dispatch registries.
- Contract JSON permits only Unicode scalar strings, booleans, null, and safe
  integers. Floats, duplicate keys, BOM, NUL, surrogates, unknown fields, and
  non-canonical JCS bytes are rejected by the test validator.

## Usage

```python
from mm_chat_rag.contracts import read_schema_bytes, schema_names

for name in schema_names():
    raw_schema = read_schema_bytes(name)
```

Shape validation, cross-reference checks, hash recomputation, and corpus gates
live under `tests/`; importing this module never activates runtime behavior.

## Layout

- `resources.py` — allowlisted package-resource access.
- `schemas/` — Draft 2020-12 Closed Schemas.
- `DESIGN.md` — trust boundaries, decisions, and limitations.
