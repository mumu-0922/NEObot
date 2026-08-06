# Owner-authorized Vault reuse for schema-v15 Validation

## Goal

Run the already implemented schema-v15 100-case production-policy Memory
Validation using owner-authorized, active Server Vault credentials without
printing, logging, committing, or durably retaining either credential. Clarify
that the operational freshness boundary is a new one-run mode-`0600` copy and
new quota approval, rather than mandatory upstream Key rotation.

## What I already know

- The owner explicitly authorized reuse of the currently configured BGE and
  Luna credentials and accepts a conservative cost ceiling.
- The schema-v15 runner already requires two distinct regular mode-`0600`
  files, rejects symlinks, same files, hard links, and equal bytes, copies the
  inputs into a private temporary directory, scans retained surfaces for both
  secrets, and destroys temporary copies on every exit.
- The live application is healthy. Its backend and Memory worker mount the
  active provider keyring, and active Provider records are stored encrypted in
  PostgreSQL.
- `backend/cmd/operator-credential-export/` exists but contains no command;
  there is no current supported operator export path. The existing Compose
  `admin` service already carries the database configuration, mounted Provider
  keyring, non-root runtime identity, and built `mm-chat-admin` binary, so the
  narrow export must extend that command rather than add a parallel binary.
- Existing runtime seams resolve exact active RAG and model Provider
  credentials in process. They return no credential through browser APIs.
- The v10 cost basis and private output parent have been created outside Git:
  `/home/mumu/.local/state/neo-chat/memory-validation/`.

## Assumptions

- Reused credential material is acceptable only under a new explicit owner
  authorization for this run; historical Development approval remains
  insufficient.
- Credential age is intentionally absent from the v15 report and manifest.
  The auditable boundary is exact Provider identity, separate BGE/Luna files,
  one-run authorization, cleanup, and aggregate-only output.
- The exact active BGE authority is RAG Provider `siliconflow`; the exact Luna
  authority is model Provider `SERVER_DEFAULT`, `openai_compatible`, normalized
  Base URL hash `3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671`,
  model `gpt-5.6-luna`.

## Confirmed Decision

- The owner confirmed that this reused-credential run is formal
  `live_validation` evidence.
- The MVP is the narrow exact-pair operator path only: active
  `RAG:SILICONFLOW` plus active `SERVER_DEFAULT`/Luna. It must not become a
  general Provider-secret export facility or an untested temporary script.

## Requirements

1. Add a bounded operator-only export command; do not add a public API.
2. Resolve only the exact active attested SiliconFlow RAG record and exact
   active `SERVER_DEFAULT` model Provider record through the existing Vault
   and runtime configuration contracts.
3. Write two caller-selected, new, non-symlink files with `O_EXCL` semantics
   and final mode `0600`; never overwrite an existing path.
4. Never emit secret plaintext, secret hashes, Authorization headers, database
   envelopes, or keyring material to stdout, stderr, logs, argv, environment,
   reports, manifests, or Git.
5. Emit only bounded content-free completion metadata. On any partial failure,
   wipe and remove every output created by that invocation.
6. Preserve the schema-v15 Provider tuple, 100-case Validation-only selection,
   BGE/Luna retry limits, global concurrency one, aggregate-only evidence, and
   Red/Orange/Yellow/Pass actions.
7. Update specs/docs so `fresh` means newly materialized one-run files plus a
   fresh explicit Validation authorization. Do not claim upstream Key rotation
   or credential provenance that the manifest cannot attest.
8. Run zero-request export/preflight checks before the live Validation.
9. After export, execute exactly one schema-v15 live Validation with the
   existing independent SiliconFlow and Luna credential values, then wipe the
   operator copies regardless of success, metric failure, or signal.
10. Do not run Holdout, mutate runtime Memory flags automatically, expose case
    detail, Push, or reinterpret historical schema-v12/v13/v14 evidence.
11. Hard-code the exported capability set to the exact BGE/Luna pair required
    by schema-v15. Reject arbitrary Provider IDs, record IDs, secret contexts,
    models, or output cardinality.

## Acceptance Criteria

- [x] Export command rejects wrong/missing/disabled/drifted Provider records,
      unavailable Vault, copied contexts, existing targets, symlinks, equal
      output paths, and partial publication.
- [x] Successful export creates exactly two mode-`0600` regular files and
      outputs no credential-derived material.
- [x] Tests prove success, fail-closed cleanup, no overwrite, no secret output,
      exact Provider selection, and signal-safe orchestration.
- [x] Schema-v15 wrapper preflight accepts only the dedicated live approvals
      and distinct BGE/Luna files.
- [x] One live run produces either a valid aggregate report plus manifest or a
      bounded pre-publication failure with complete cleanup.
- [x] The observed outcome is reported truthfully and never auto-promotes or
      auto-releases Memory.
- [x] Full relevant quality gates pass and the Git working tree contains no
      credential or live output artifact.

## Definition of Done

- Tests, lint, vet, and focused race checks pass.
- Specs and operator documentation describe the credential provenance and
  cleanup boundary accurately.
- Exported credentials are wiped and removed after the one authorized run.
- Live evidence, if validly produced, remains private and aggregate-only.
- Code/docs changes are committed but not pushed without owner confirmation.

## Out of Scope

- New Provider accounts or upstream Key rotation.
- Browser-visible secret export.
- General-purpose credential dump tooling.
- Export of any Provider other than the exact schema-v15 BGE/Luna pair.
- Holdout execution, policy retuning, corpus changes, automatic Release, L2
  Scene, or L3 Persona work.

## Validation Outcome

- Live run: `memory-regression-20260806t013956z-31e67617`
- Result: Orange, `passed=false`, `releaseEligible=false`
- Required action: `disable_memory_recall_preserve_data`
- Cause: nine false-injection cases, rate `0.09` above the frozen `0.02` gate
- Safety: every cross-user, deleted-memory, Secret, untrusted-source, and
  unauthorized-egress counter remained zero
- Report SHA-256:
  `6b2ec1a0cf26b2190302accac384f9fab4fce0898d1b1bad1eaacb5a2ce39c69`
- Manifest SHA-256:
  `3ee114b2991ad2d0de954ad4a5998947567c66672e010dc079f17c73c18ae650`
- Cleanup: exported credentials and all isolated containers, networks, and
  volumes were removed; no runtime Memory flag was changed automatically

## Technical Notes

- Relevant contracts: `.trellis/spec/backend/memory-v2-benchmark.md`,
  `.trellis/spec/backend/memory-v2-hybrid-shadow.md`,
  `.trellis/spec/backend/rag-retrieval-storage.md`, and
  `mm-chat/docs/contracts/provider-secret-vault.md`.
- Likely implementation surfaces: `backend/cmd/admin/`, existing
  `backend/internal/runtimeconfig/` seams, Compose/operator scripts, and Memory
  benchmark workflow documentation. The empty
  `backend/cmd/operator-credential-export/` directory is not an implementation
  authority.
- Live runtime state under `mm-chat/data/`, `mm-chat/secrets/`,
  `mm-chat/backup/`, and `.env.single-server` must not be rewritten.

## Decision (ADR-lite)

**Context:** The schema-v15 manifest deliberately does not retain credential
values, hashes, issuance dates, or Vault envelopes. The earlier wording
"fresh credentials" therefore described an operator procedure, not a
machine-attested report field. The owner prefers the already configured
Provider credentials because upstream server behavior is unchanged.

**Decision:** Count the owner-authorized reuse as formal `live_validation`.
Interpret freshness as a newly authorized run with newly materialized,
independent, one-run mode-`0600` BGE/Luna files. Preserve exact Provider tuple,
distinct-value checks, approval gates, cleanup, and all v15 evaluation rules.

**Consequences:** This avoids unnecessary upstream Key rotation and keeps
credential material out of report identity. Evidence attests exact configured
Provider authority and one-run isolation, but it does not claim that upstream
Key material was newly issued. A general reusable secret-export feature is not
created.

## Technical Approach

1. Add an exact-purpose `memory-validation-credentials-export` subcommand to
   the existing `mm-chat-admin` binary with no arbitrary Provider selector. It
   loads normal Backend configuration, the mounted keyring, and PostgreSQL
   through existing packages and runs under the existing Compose `admin`
   service.
2. Resolve the attested active `siliconflow` RAG credential and
   `SERVER_DEFAULT` model credential through existing runtime configuration
   seams, validating the exact Luna tuple before publication.
3. Create the two requested outputs exclusively with mode `0600`; report only
   fixed success/failure classes. Wipe partial files and in-memory byte slices
   on all exits.
4. Add a small operator wrapper that creates a private temporary export
   directory, invokes the command, passes the two paths into the unchanged
   schema-v15 runner, and wipes the export directory on success, metric
   failure, ordinary failure, and signals.
5. Update schema-v15 documentation/spec wording from upstream Key freshness to
   fresh one-run copy plus independent authorization, without changing report
   schema, Provider tuple, or historical evidence.
6. Run zero-request unit/integration/preflight gates first; only then execute
   the single owner-authorized live 100-case Validation.

## Implementation Plan

- **PR1:** Exact-pair operator export command and focused security/lifecycle
  tests.
- **PR2:** One-run export-to-Validation wrapper, signal cleanup, and contract
  documentation.
- **Operational run:** Zero-request preflight, one live schema-v15 execution,
  evidence inspection, credential wipe proof, and truthful outcome recording.
