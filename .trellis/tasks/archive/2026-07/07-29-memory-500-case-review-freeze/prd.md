# Rebuild the Memory v2 machine-reviewed regression benchmark

## Goal

Preserve the recanted v1 authoring attempt as immutable audit evidence, then
build a new deterministic 500-case synthetic regression corpus that can be
generated, machine-audited, validated, and evaluated offline without ever
claiming human review or becoming reader-promotion evidence.

## Confirmed Decision

The user selected the assistant-operated path on 2026-07-29:

- rebuild a v2 machine-reviewed regression benchmark end to end;
- do not require another 500/650 manual browser-review session;
- label every v2 artifact and report as regression-only and non-promotional;
- keep formal human-reviewed Golden admission unchanged for any future
  promotion-grade benchmark.

## Preserved v1 Incident Evidence

- Protected root: `mm-chat/data/memory-benchmark/v1/`.
- Candidate-manifest SHA-256:
  `bfca1b829ab4f886c558cad1be3c5f1e7c218492621405ac6d9fd8033113559c`.
- Fixture-content SHA-256:
  `642015e9baea206366112874cc50d0fb73c001a62424ed86d805c023abe5aee5`.
- Initial status SHA-256:
  `1e69314f8bfd1053af9627a3b4be6ec3ec2fd9b0a2ddf30889f194984dddb517`.
- Recanted reviewer UUID:
  `ac7fe75b-9046-4281-9af6-673e14293c7b`.
- Current replay is structurally valid at sequence 650 with `650/0/0`, but the
  user stated the accepts were clicked without inspecting the cases. They have
  no human-review authority.
- Protected machine audit:
  `mm-chat/data/memory-benchmark/v1/machine-review/audit-20260729-v2.json`,
  SHA-256
  `58ef912676a13e2f1c1e08617d575ff471e19a9754438913da5cf00c737ed927`.
- The v1 audit verdict is `not_suitable_for_formal_freeze`: ordinal shortcuts
  occur in 650/650 cases, queries collapse to six skeletons, and the
  preference, fallback, multi-hop, language, and scope semantics are weak.
- v1 remains unfrozen; no Holdout UUID was created or consumed.

The v1 directory, ledger, reports, permissions, hashes, and status must not be
deleted, overwritten, truncated, repaired, or relabeled.

## Functional Requirements

### 1. Separate corpus class and admission path

- Add a dedicated regression corpus schema and decoder. It must declare:
  `corpusClass=machine_reviewed_regression`,
  `admissionMode=regression_only`, and `promotionEligible=false`.
- Regression cases must carry no reviewer UUID, review timestamp, copied human
  attestation, frozen lifecycle, or precommitted Holdout authority.
- The existing `GoldenSet`, `ValidateGoldenAdmission`, `Evaluate`, human review
  ledger, freeze, and one-shot Holdout contracts remain strict and unchanged.
- Formal Golden decoding/admission must reject regression artifacts, and the
  regression decoder/admission path must reject formal/draft Golden artifacts.
- A regression report must repeat the corpus class, admission mode, and
  `promotionEligible=false`. Passing means only that the tested reader passed
  this regression corpus; it must not authorize promotion or mutate runtime.

### 2. Deterministic v2 corpus

- Publish to a new protected root under
  `mm-chat/data/memory-benchmark/v2-regression/`; never reuse or overwrite v1.
- Generate exactly 500 synthetic cases and 500 bound fixtures with exact split
  `300/100/100`, language `350/100/50`, and every critical slice at least 50
  total with at least `30/10/10` split coverage.
- Generation is deterministic and offline: fixed version/profile/seed/order,
  stable JSON bytes, and zero Provider, model, DB, clock, network, Live Memory,
  Hindsight, or production-reader calls.
- IDs are opaque deterministic values. Neither query nor fixture content may
  reveal case order, case ID, fixture alias, Memory ID, or a shared ordinal.
- Queries are unique after normalization and provide broad semantic/template
  diversity rather than a small ordinal-parameterized skeleton set.
- Chinese, English, and mixed-language cases use natural query/content pairs;
  cross-language behavior is explicitly represented by `mixed_language_entity`
  rather than accidental English-query/Chinese-memory mismatch.

### 3. Slice semantics

- `stable_fact`: query and active Memory express one stable fact.
- `preference_instruction`: query requests an explicit user preference or
  standing instruction; the expected Memory states that preference/instruction.
- `project_decision`: query wording and relevant Memory identify the same
  Project and require non-empty Project scope.
- `chinese_paraphrase`: Chinese query paraphrases rather than copies Memory.
- `mixed_language_entity`: query/content deliberately bridge Chinese/English
  while preserving the same named entity.
- `temporal_correction`: current active Memory and superseded exclusion are
  both present, ordered, and semantically contradictory on the same fact.
- Negative slices contain the required rejected/deleted/cross-scope evidence
  and expect no Memory.
- `failure_fallback`: query and fixture evidence explicitly describe a
  timeout/error/degradation condition and the authorized fallback behavior.
- `multi_hop`: each case requires at least two distinct active Memories whose
  facts must be composed to answer the query; merely attaching a second record
  is insufficient.
- Scope wording must match Global/Project/Conversation authority. Global cases
  must not falsely describe a project decision; Project and Conversation cases
  must name the matching synthetic entity.

### 4. Machine semantic audit

- Audit all 500 cases and fixtures before publication/admission.
- Fail closed on any fixture/reference/state/scope binding error, normalized
  duplicate, ordinal/identifier shortcut, language mismatch, scope-text
  mismatch, missing slice evidence, weak preference/fallback/multi-hop
  semantics, or insufficient split/language/slice coverage.
- Require at least 100 normalized semantic query skeletons and record the exact
  observed count.
- Emit a deterministic, content-free audit report with counts and verdict but
  no query text, Memory body, fixture content, or reviewer identity.
- Admission requires an audit verdict of `passed` and exact hash bindings among
  fixtures, corpus, manifest, and audit.

### 5. Protected publication and replay

- Add explicit regression generate/status/verify operator entrypoints. Generate
  creates a new root only; verify regenerates in memory and requires byte-for-
  byte equality of every protected artifact.
- Enforce repository-local versioned path restrictions, no symlink traversal,
  private directories `0700`, files `0600`, exclusive creation, and no in-place
  retry/overwrite.
- Keep complete fixture/corpus/audit content outside Git. Commit only a
  content-free tracking status containing schema/profile/counts/verdict/hashes.
- `mm-chat/data/`, `secrets/`, `backup/`, and the active environment remain
  untouched except for the new v2 regression root.

### 6. Regression evaluation

- Add a strict regression observation schema bound to the exact corpus-content
  and fixture hashes, immutable profile configuration hash, exact ordered 500
  case IDs, Candidate limit 20, and Final limit 5.
- Do not require or simulate a formal Holdout UUID/ordinal. The `holdout` split
  label is regression stratification only and is fully machine-visible.
- Reuse the existing metric/safety/budget implementation rather than copying
  scoring logic: Recall@20, Recall@5, current fact, false injection, nDCG/MRR,
  P95/P99/cutoff, prompt tokens, Provider cost, cross-user/deleted/secret/
  untrusted leakage, and unauthorized Provider egress.
- Add an independent `EvaluateRegression` entrypoint and CLI mode/command that
  always produces the regression report schema. Existing report files are
  never overwritten; failed gates still publish the report before non-zero.
- Evaluation remains offline with respect to providers and runtime state. It
  only consumes already captured observation JSON.

## Acceptance Criteria

- [x] v1 recantation, protected hashes, no-freeze state, and failing machine
      audit are preserved and documented.
- [x] The user chose the non-human v2 regression path and delegated the full
      rebuild/review to the assistant.
- [x] Existing formal Golden admission and human-authority tests remain green
      without a machine-review bypass.
- [x] v2 generation is byte deterministic and produces exact 500, 300/100/100,
      350/100/50, and every-slice `>=50` plus `30/10/10` coverage.
- [x] Machine audit reports zero shortcuts and zero semantic/binding failures,
      at least 100 query skeletons, and verdict `passed`.
- [x] Regression admission rejects any missing/true promotion flag, wrong class
      or mode, draft/human attestation, bad count/coverage, bad audit/hash, or
      formal Golden artifact.
- [x] Regression evaluation accepts exact valid observations, rejects binding/
      order/schema drift, emits explicit non-promotional provenance, and shares
      the established scoring implementation.
- [x] The new protected v2 root replays byte-for-byte with `verify`; permissions
      are `0700/0600`; no v1 or unrelated runtime path changes.
- [x] A content-free v2 tracking status is ready for commit while complete artifacts
      remain ignored and absent from standalone copies.
- [x] Focused race, all backend tests, `go vet`, standalone full, security,
      quality, module, and change checks pass.

## Technical Approach

1. Keep v1 authoring and formal evaluation code paths intact.
2. Add regression-specific corpus/observation/report types and admission
   validation inside `memoryeval`, sharing only case validation and scoring
   primitives.
3. Add a v2 regression generator, semantic auditor, secure publisher, and
   verifier beside `memoryauthor` without changing `Generate()` output.
4. Expose explicit regression subcommands and an explicit regression evaluator
   entrypoint so operators cannot confuse them with formal Golden freeze.
5. Generate the new protected corpus, replay it, publish content-free evidence,
   then run the complete quality gates.

## Decision (ADR-lite)

**Context**: The original v1 corpus is structurally deterministic but
semantically weak, and its 650 accepts were recanted. Reusing its ledger or
loosening formal admission would fabricate review authority. Requiring the user
to click another 500 cases would also not create reliable review evidence.

**Decision**: Create a separate machine-reviewed regression corpus and evaluator
contract. Reuse scoring internals, but keep artifact schemas, admission rules,
reports, and operator commands visibly distinct from formal human-reviewed
Golden evaluation.

**Why**: Type/schema separation makes accidental promotion harder than a mode
flag on the formal path; shared scoring prevents metric drift; a content-free
semantic audit closes the exact shortcut and slice-quality failures found in
v1; immutable protected publication keeps the result replayable without
committing synthetic corpus content.

**Consequences**: v2 can catch deterministic reader regressions and support
baseline/candidate comparisons, but it cannot justify reader promotion. A
future promotion-grade benchmark still needs a fresh corpus, genuine human
review, formal freeze, and one-shot Holdout.

## Out of Scope

- Repairing, freezing, deleting, or relabeling v1.
- Machine-generated human reviewer identities/timestamps or bulk human accepts.
- Formal reader promotion or changing any active Memory reader.
- Producing observations from Live Memory, a Provider, DB, Hindsight, or chat.
- Hiding the regression-only boundary behind documentation alone.

## Required Documentation

- `.trellis/spec/backend/memory-v2-benchmark.md`
- `mm-chat/docs/contracts/memory-benchmark-workflow.md`
- `mm-chat/backend/internal/memoryauthor/README.md`
- `mm-chat/backend/internal/memoryauthor/DESIGN.md`
- `mm-chat/backend/internal/memoryeval/README.md`
- `mm-chat/backend/internal/memoryeval/DESIGN.md`
- content-free tracking status under `mm-chat/docs/tracking/`

## Operational Evidence

- Protected v2 root: `mm-chat/data/memory-benchmark/v2-regression/`.
- Permissions: root `0700`; `fixtures.json`, `corpus.json`, `audit.json`, and
  `manifest.json` all `0600`.
- Machine audit: 500/500 cases, 431 entity/topic-normalized query skeletons,
  zero shortcut/binding/language/scope/preference/fallback/multi-hop failures,
  verdict `passed`.
- Content hashes: fixture
  `965072e0e5b36d687c1838aaa675beb8a81475e0794d7fdded34f99d7673c7af`,
  corpus
  `5ae712163fc7bf81efe754b877cd02b1d0de74450a04ea7e119402320e4ba144`,
  audit
  `c0f5b9406acc707f087afc7caf6f5f3e1dbc50aa6b1492b7ef1ec5479fe051f3`,
  manifest raw
  `22148741a0063554ee3566c658dfa99bab9e55b8b1b85e7e6db0ae3a7752162a`.
- `regression-verify` regenerated every artifact byte-for-byte after code
  formatting/refactoring. v1 candidate/audit hashes remained unchanged.
- A clearly labeled `fixture-oracle-protocol-smoke` exercised the real CLI
  over all 500 ordered cases, produced report SHA-256
  `e1b79a7f4fca75e171dc7c2e8e459b693259633b82cfc0740c17a3b011f1db7a`,
  declared `promotionEligible=false`, and was deleted. It proves evaluator
  wiring only, not reader quality.
- Focused race passed for `memoryauthor`, `memoryeval`, and both commands; all
  backend tests and `go vet ./...` passed.
- Full standalone passed: frontend 198 files / 961 tests plus production build,
  backend all packages, RAG 1906 passed / 7 skipped, and standalone isolation.
- Changed-file security scan: zero findings. Quality scanners: zero warnings
  for both changed backend modules. Module scanners passed; only generic
  existing flat-Go-package organization warnings remain.
