# Offline Parser Contracts Design

## Goals

- Freeze deterministic parser artifact shapes before Native Parser work.
- Make every object and discriminated union closed and versioned.
- Keep JCS, logical hashes, Opaque IDs, locators, and stable errors portable
  across Python, Go, and JavaScript.
- Package schemas without activating any production RAG execution path.

## Non-goals

This module does not parse documents, validate database rows, call Providers,
write object storage, or register worker handlers. Runtime semantic validation
and migration `012` staging are later C1/C-stage work.

## Architecture

```text
installed wheel
  -> contracts.resources fixed allowlist
  -> Draft 2020-12 schema bytes
  -> test-only strict JSON/JCS validator
  -> fixture, hash, corpus, and cross-runtime gates
```

Schema `$id` values use the reserved `.invalid` namespace. Test validators load
all references from an in-memory registry and never retrieve a URI.

## Decisions

| Decision                                | Reason                                                   |
| --------------------------------------- | -------------------------------------------------------- |
| Safe integers only; no float tokens     | Prevent cross-runtime number drift.                      |
| RFC 8785 canonical bytes                | Give hashes one byte-level representation.               |
| Domain-prefixed SHA-256                 | Prevent cross-kind hash substitution.                    |
| Arrays instead of entity maps           | Preserve explicit semantic order.                        |
| Source-derived strings use payload refs | Keep immutable lineage separable from deletable payload. |
| Validators remain test-only in C1.1     | Avoid changing the dark-run runtime dependency graph.    |

## Security and limitations

JSON Schema proves local shape, not semantic consistency. Tests must separately
reject duplicate keys, unsafe integers, invalid Unicode, non-canonical bytes,
hash mismatches, path escape, bad ordering, missing references, cycles, range
overlap, and corpus provenance drift. C1.1 fixtures freeze inputs and contracts;
they do not claim parser accuracy, fresh-container determinism, or Provider
compatibility. Production registries remain empty until later promotion gates.

## Change policy

Published schema semantics are immutable. A breaking field, comparator, rank,
hash envelope, or error mapping requires a new schema/profile version and
regenerated golden vectors; expected hashes must never be silently updated.
