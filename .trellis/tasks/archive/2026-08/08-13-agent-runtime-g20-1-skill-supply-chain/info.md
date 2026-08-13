# G20.1 Skill Supply Chain — Technical Design

## Decision

Build one separate `skillsupply` bounded context. It owns untrusted source
fetch, no-execute validation, immutable package/SBOM objects, administrator
admission, Store projection, and owner-bound install references. It does not
own runtime execution or Tool authorization.

## Flow

```text
official | LobeHub exact version | GitHub exact commit | ZIP upload
  -> bounded source ZIP + immutable source ref
  -> in-memory no-execute validator
  -> Agent Skills / optional Neo Manifest validation
  -> canonical inventory + canonical ZIP
  -> package/runtime fingerprints + CycloneDX SBOM
  -> content-addressed quarantine/package/SBOM objects
  -> PostgreSQL validated candidate
  -> administrator fingerprint+revision review
  -> admitted Store projection
  -> authenticated owner install reference
```

## Durable model

```text
skill_package_versions
  immutable package/runtime/SBOM metadata and object coordinates

skill_package_candidates
  exact source ref/artifact hash, validation report, admission status,
  reviewer, reason, revision; unique source coordinate detects drift

skill_installations
  user + exact admission/package reference + revision; unique user/Skill name
```

Package bytes are never updated. Re-review changes only candidate decision
state under CAS. Uninstall deletes only the owner's reference.

## Failure ordering

1. Fetch and validate entirely within memory/byte limits.
2. Write deterministic content-addressed objects. Duplicate writes are safe.
3. Transactionally insert/reuse the immutable package and exact source
   candidate. A source-ref/hash mismatch is rejected as drift.
4. If the database write fails after object writes, only unreachable
   content-addressed objects can remain; no admitted state exists. Later
   retention may remove unreferenced objects, but G20.1 never executes them.

## Schema correction

`neo.runtime.json` is the candidate declaration, not its own admission
attestation. Self-referential fingerprints/admission ID move to the generated
database/API envelope. This allows the manifest bytes to remain fully covered
by the package hash and makes offline replay deterministic.

## Initial admission policy

- Instruction-only: optional `allowed-tools` remains display/request metadata;
  no runtime fingerprint and no executable path.
- Executable: only the server-owned official synthetic fixture with read-only
  capability requests, no Egress, and no Secret slots is review-admittable.
- Everything else may be quarantined/validated for operator inspection but
  cannot enter `admitted` in G20.1.

## Rollback

- Feature routes fail closed when repository/object storage is unavailable.
- No current Chat/Skill path is modified, so application rollback restores the
  previous backend image while legacy Skills remain authoritative.
- Migration down is allowed only after G20.1 installs/candidates/versions are
  explicitly removed in the rehearsed rollback window; it refuses data loss.
- PostgreSQL and MinIO/S3 are backed up and restored as one checked set.

