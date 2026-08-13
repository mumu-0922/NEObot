# Skill Supply Chain Design

## Goals

- Establish a distinct Skill Store and immutable package authority.
- Make source drift, package identity, runtime identity, and SBOM identity
  independently replayable.
- Preserve a hard no-execute boundary until later Agent Runtime groups pass
  their isolation and launch gates.

## Ingestion pipeline

```text
server-derived exact source
  -> bounded ZIP bytes
  -> in-memory archive/path/mode/size validation
  -> root SKILL.md + optional strict neo.runtime.json
  -> sorted path/size/file-hash inventory
  -> deterministic canonical ZIP + CycloneDX 1.6 SBOM
  -> content-addressed object writes
  -> immutable package + exact-source candidate transaction
```

No phase extracts files to a working directory or invokes a shell, package
manager, interpreter, build, hook, test, or candidate entrypoint.

## Identity model

| Identity | Input | Purpose |
| --- | --- | --- |
| source artifact | exact received ZIP bytes | detect immutable-source drift |
| package | domain-separated sorted path/size/file-hash inventory | ignore ZIP timestamps/order/compression |
| runtime bundle | package fingerprint plus canonical validated runtime declaration | bind the future runtime surface |
| SBOM | exact deterministic CycloneDX JSON bytes | verify stored component inventory |

`neo.runtime.json` contains declarations only. Source coordinate,
package/runtime/SBOM fingerprints, candidate/admission ID, reviewer, and
decision revision live in the server-generated envelope. This removes the
self-referential hash problem while keeping every manifest byte in package
identity.

## Durable authority

- `skill_package_versions` is immutable and content-addressed.
- `skill_package_candidates` owns one exact source coordinate and a
  fingerprint-bound, revision-CAS administrator decision.
- `skill_installations` owns only a user's exact admitted package reference.
- A composite foreign key plus admission trigger prevents an installation from
  binding a different or non-admitted package even outside the repository.
- PostgreSQL `jsonb` normalizes stored metadata, so immutable replay compares
  decoded `allowed_tools` and `capability_requests` structures rather than raw
  JSON bytes; equivalent candidates remain idempotent without weakening the
  fingerprint and scalar-field comparisons.
- Uninstall deletes only the current owner's reference; immutable candidates,
  versions, SBOMs, and review history survive.

## Failure ordering and rollback

Objects are written before the database transaction. Failure can therefore
leave unreachable content-addressed bytes, but can never create admitted
state. Repeated object writes are safe. Migration down refuses while any G20.1
row exists. Application rollback keeps migration `083` and the object prefixes
in place; paired PostgreSQL/MinIO backup and restore is the data rollback unit.

## Admission policy

Instruction-only packages are reviewable but have no runtime fingerprint or
launch path. Executable packages are reviewable only when they are the
server-owned official synthetic fixture, request read-only workspace actions,
and request no Egress or Secret slots. Every other executable candidate stays
quarantined/ineligible in G20.1.
