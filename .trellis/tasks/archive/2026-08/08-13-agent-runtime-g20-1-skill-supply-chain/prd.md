# Agent Runtime G20.1 — Skill Supply Chain and Store Authority

## Goal

Implement the first production-code slice of the Neo Agent Runtime: a separate
server-authoritative Skill package supply chain and Store authority. Official,
LobeHub, exact-Git-commit, and ZIP candidates enter a no-execute quarantine,
are validated and fingerprinted, receive a deterministic SBOM, and require
administrator admission before an authenticated user can create an immutable
install reference. Runtime execution remains impossible.

## Shared Baseline

- Assistant, Skill, and Tool remain separate stores and authorization domains.
- Go/PostgreSQL is durable authority; MinIO/S3 stores opaque immutable bytes.
- `SKILL.md` follows Agent Skills; `allowed-tools` is untrusted declarative
  metadata and never grants or enables a Tool.
- Package/runtime/SBOM identities are content-addressed and immutable.
- Administrator review is fingerprint-bound and revision-CAS protected.
- Current browser-owned pure-text Skills remain untouched through G20.8 and are
  hard deleted without migration/wrapping only in G20.9.
- `/v1/code/executions` remains `CODE_EXECUTION_UNAVAILABLE`.
- G20.1 adds no Orchestrator, Runner, Sandbox, Cron, Child Agent, learning, Tool
  grant, Egress Broker, Secret Broker, or Runtime execution.
- User already confirmed all recommended design choices and authorized direct
  implementation/commit after verification.

## Requirements

### Source adapters

- Add server-owned adapters for:
  - an allowlisted official catalog containing only a synthetic read-only
    fixture initially;
  - LobeHub exact versions through the existing Marketplace M2M token/cache
    owner, without a second credential path;
  - GitHub repositories at an exact lowercase 40-hex commit plus an optional
    validated package subdirectory;
  - authenticated raw ZIP upload.
- LobeHub and Git reject omitted, `latest`, branch, or tag references.
- Every adapter returns bounded ZIP bytes plus a server-derived immutable source
  coordinate. A repeated coordinate whose raw bytes differ is
  `SKILL_SOURCE_DRIFT`; no existing record is updated.
- The browser never supplies an arbitrary download URL or upstream credential.

### No-execute quarantine and archive validation

- Maximum source ZIP is 32 MiB; maximum expanded package is 128 MiB; maximum
  individual file is 16 MiB; maximum regular files is 2,048; expansion ratio
  is bounded.
- Inspect with Go libraries in memory. Never extract into the Backend working
  tree or call a package script, interpreter, installer, hook, build, or test.
- Reject traversal, absolute/Windows/NUL/invalid UTF-8 paths, symlink/hardlink
  escapes, devices/FIFOs/sockets, unsupported modes/encodings, duplicate and
  NFC/case-fold-colliding paths, oversized/decompression-bomb content, and
  ambiguous package roots.
- Persist the exact source ZIP under quarantine and a separately generated
  canonical sorted ZIP under the package fingerprint.

### Package validation

- Require exactly one root `SKILL.md` after the adapter's explicit prefix is
  removed.
- Validate Agent Skills frontmatter: bounded `name`, `description`, optional
  license/compatibility/string metadata, parent-name match, and bounded
  experimental `allowed-tools`.
- `neo.runtime.json` is optional for instruction-only Skills. If present, use
  strict duplicate/unknown/trailing-field rejection and validate the Phase 0
  runtime schema plus cross-field rules.
- Correct the Phase 0 manifest circularity: candidate manifest declarations do
  not contain their own package/runtime/SBOM fingerprints or admission ID.
  Those bindings belong to the server-generated immutable version/admission
  record, while the complete manifest bytes remain in the package fingerprint.
- Runtime images and declared dependencies must use immutable SHA-256 digests.
  Instruction-only packages receive no runtime fingerprint and cannot launch.
- G20.1 admission policy accepts instruction-only packages and the official
  synthetic read-only runtime fixture only. Any egress, secret slot, mutable
  capability, or non-official executable package remains non-admittable.

### Fingerprints and SBOM

- Derive independent `sourceArtifactSHA256`, `packageFingerprint`, optional
  `runtimeBundleFingerprint`, and `sbomFingerprint` values.
- Package identity uses domain-separated sorted canonical path/size/file-hash
  inventory, not ZIP metadata, source labels, or filename.
- Generate deterministic content-free CycloneDX 1.6 JSON for the Skill, file
  inventory, pinned runtime image, and declared immutable dependencies.
- Revalidation of equivalent archives with different order/timestamps/
  compression reproduces all package/runtime/SBOM identities.

### PostgreSQL authority

- Add additive migration `083` with separate immutable package versions,
  source candidates/admission decisions, and owner-bound installation rows.
- Preserve exact source provenance, validation report, object coordinates,
  fingerprints, requested Tool display metadata, admission actor/reason,
  revision, and timestamps.
- Candidate review supports `admitted|rejected` with expected-revision CAS.
- Install requires a current admitted candidate and exact confirmed package
  fingerprint. Install creates only an owner-bound package reference; it never
  enables Tools, models, credentials, legacy Skills, or execution.
- Uninstall uses owner plus revision CAS and deletes no immutable package,
  candidate, admission, SBOM, or history row.
- Runtime database role gets only the table privileges required by these APIs.
- Down migration refuses to discard non-empty G20.1 authority tables.

### API

Authenticated routes are under `/v1/skills` and use strict camelCase JSON,
bounded bodies, stable error envelopes, and `Cache-Control: no-store`:

```text
POST   /v1/skills/candidates/official
POST   /v1/skills/candidates/lobehub
POST   /v1/skills/candidates/git
POST   /v1/skills/candidates/zip
GET    /v1/skills/candidates/{id}
POST   /v1/skills/candidates/{id}/review
GET    /v1/skills/store?page=&pageSize=20
GET    /v1/skills/store/items/{admissionId}
POST   /v1/skills/store/items/{admissionId}/install
GET    /v1/skills/library
DELETE /v1/skills/library/{id}?revision=
```

- Candidate creation/detail/review requires the exact deployment
  administrator. Store browse/detail and library operations require an
  authenticated owner.
- API DTOs expose fingerprints, bounded metadata, requested Tool names and
  validation/admission state, but never raw package/SBOM bytes, host paths,
  credentials, object-store coordinates, or package instruction content.

### Backup, restore, and offline replay

- Existing paired PostgreSQL/MinIO backup remains the capture mechanism.
- Extend restore acceptance/sample generation so all three Skill object
  prefixes and their PostgreSQL coordinates are checked in a temporary
  database/bucket drill.
- Add a focused PostgreSQL 17 gate for fresh migration, replay, grants, source
  drift, review CAS, ownership, install/uninstall, guarded down/up, and cleanup.
- Add an offline supply-chain verifier/gate that runs the malicious archive
  corpus, deterministic fingerprint/SBOM replay, no-execution proof, Phase 0
  schema fixtures, and current `CODE_EXECUTION_UNAVAILABLE` assertion.

## Acceptance Criteria

- [ ] All four source adapters produce only bounded server-derived candidates;
      floating Git/LobeHub refs and arbitrary URLs are rejected.
- [ ] Malicious archive corpus fails closed before any object becomes admitted.
- [ ] A marker script inside a candidate is never executed during ingestion,
      validation, review, install, replay, or tests.
- [ ] Equivalent archive representations reproduce package/runtime/SBOM
      fingerprints; any file/manifest/dependency change changes authority.
- [ ] Reusing an immutable source ref with changed source bytes returns
      `SKILL_SOURCE_DRIFT` and preserves the first record.
- [ ] Admission/rejection is administrator-only, fingerprint-bound, CAS-fenced,
      and restricted to the initial instruction-only/synthetic-read-only policy.
- [ ] Cross-user candidate mutation, installation access, library access, and
      uninstall fail; stale revisions cannot overwrite or delete state.
- [ ] Installing a Skill creates no Tool, MCP, provider, credential, model,
      Assistant, legacy Skill, Conversation, Run, or Sandbox state.
- [ ] PostgreSQL 17 fresh/replay/down/up and paired backup/restore sampling pass.
- [ ] `/v1/code/executions` still returns `CODE_EXECUTION_UNAVAILABLE`; legacy
      browser Skill behavior and persistence are byte-unchanged.
- [ ] Focused Go tests, race tests, `go vet ./...`, `go test ./...`, Phase 0
      verifier, supply-chain verifier, preflight/Compose checks, and applicable
      standalone gate pass.

## Deliverables

- `mm-chat/backend/internal/skillsupply/` domain, validators, adapters,
  repository, service, handler, tests, `README.md`, and `DESIGN.md`.
- `mm-chat/backend/migrations/083_skill_supply_chain.{up,down}.sql` plus schema
  and PostgreSQL integration coverage.
- API/main/httpserver wiring and narrowly scoped LobeHub transport extension.
- `mm-chat/scripts/verify-skill-supply-chain.sh` and
  `mm-chat/scripts/verify-skill-supply-chain-postgres17.sh`.
- Agent Runtime contract/architecture/deployment/tracking/spec and backup/
  restore documentation updates.

## Out of Scope

- Skill Store frontend or composer integration (G20.8).
- Applying package instructions to Chat or migrating current pure-text Skills.
- OCI Runtime Bundle build, package dynamic tests, or any candidate execution.
- Run/Step/Attempt persistence, leases, Runner RPC, rootless OCI, workspace,
  artifacts, grants, Tool/Egress/Secret brokers, Prepare/Commit, Child Agents,
  Cron, learning, or production Kill Switches.
- User-supplied source credentials, private Git repositories, arbitrary Git
  hosts, tar archives, package publishing, ratings/comments, or auto-update.

## Research References

- [`research/current-admission-reuse.md`](research/current-admission-reuse.md)
- [`research/archive-supply-chain.md`](research/archive-supply-chain.md)
- [`research/sbom-and-fingerprint.md`](research/sbom-and-fingerprint.md)
- [`research/lobehub-skill-source.md`](research/lobehub-skill-source.md)

