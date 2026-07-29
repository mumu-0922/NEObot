# Implement native Memory regression runner

## Goal

Build a replayable, isolated native Memory capture lane that executes the
current production v1 lexical reader and native v2 hybrid shadow reader against
the protected 500-case machine-reviewed regression corpus, emits strictly
bound observations/reports, and produces the first truthful v1-versus-v2
reader-quality comparison without changing prompt authority or production
state.

## What I already know

- The protected v2 corpus is complete, machine-audited, and replay-verifiable,
  but it is permanently `promotionEligible=false`.
- The existing evaluator shares formal scoring logic but consumes observations
  only; the fixture-oracle smoke proved wiring, not reader quality.
- v1 is the current Global-only Top-5 prompt/Usage reader.
- v2 hybrid is default-off shadow logic using Exact/CJK BM25/BGE-M3/RRF(60)/
  BGE rerank and a 600-target/900-hard token budget.
- Hindsight is no longer a runtime candidate for this step.
- The user authorized implementation of a native v1-versus-v2 real regression
  runner after the prior benchmark task completed.

## Requirements

### Admission and scope

- Accept only the independent regression fixture/corpus/audit/manifest schemas
  and validate their two-way hashes before seeding or Provider calls.
- Preserve exact 500-case order and emit one complete observation per case.
- Keep every output `corpusClass=machine_reviewed_regression`,
  `admissionMode=regression_only`, and `promotionEligible=false`.
- Never call formal Golden admission, freeze/Holdout workflows, profile
  promotion functions, or prompt injection.

### Runtime fidelity

- Reuse production `usermemory` v1 lexical and v2 hybrid Go/SQL paths; do not
  reproduce ranking in the benchmark package.
- Execute against an isolated PostgreSQL 17 database with the pinned
  `pg_textsearch 1.3.1` and `pgvector 0.8.5` extensions and all current
  migrations.
- Seed deterministic synthetic users, Projects, Conversations, messages,
  canonical Memory state, scope generations, and projections from the fixture
  without reading live chat or Memory.
- Build real BGE-M3 fixture vectors for a live candidate capture. Offline
  tests must use a deterministic fake provider through the same interfaces.
- Run under the same API capability boundary used by Server mode after the
  privileged, benchmark-only seed stage.

### Observation semantics

- Emit exclusive baseline `native_v1_lexical` and candidate
  `native_v2_hybrid` observation JSON files.
- Baseline candidate/final/injected IDs equal the actual production v1 Top-5.
- Candidate IDs equal authorized v2 RRF Top-20; final IDs equal actual
  reranked and token-budgeted Top-5.
- Candidate `injectedMemoryIds` mirrors final IDs only as a documented offline
  counterfactual needed by current-fact and false-injection scoring; no runtime
  injection occurs.
- Record exact transient Provider-sent Memory IDs, fallback code, latency,
  prompt token estimate, and hard-cutoff state. Retrieval-only
  `persistedMemoryIds` remains empty.
- Bind `configurationSha256` to inputs, reader/model versions, limits,
  state-mapping rules, and cost basis.

### Safety, isolation, and cleanup

- Never connect the capture runner to the live Neo Chat database. Refuse a
  database that lacks the exact ephemeral benchmark marker/name.
- Use a random Compose project, publish no ports, assign bounded CPU/memory,
  and destroy its database, role, containers, network, volume, and temporary
  credential material on success, error, or signal.
- Take a live SiliconFlow credential only through a temporary mode-0600
  file/stdin boundary; never expose it in argv, Docker inspect, logs, reports,
  Git, or retained output. Do not discover/decrypt the production vault.
- Require an explicit versioned same-unit cost basis; never fabricate provider
  costs or silently use zero-cost placeholders.
- Scan retained artifacts to prove fixture/query/plaintext and credentials did
  not leak outside the protected observation files. Reports/status remain
  content-free where the existing contract requires it.

### Operator flow

- Provide one script/command that validates prerequisites, starts the isolated
  database, migrates, seeds, captures both profiles, evaluates both profiles,
  publishes exclusive artifacts, and tears everything down.
- Provide a deterministic zero-network protocol test for Compose lifecycle,
  partial-output cleanup, exclusive publication, and secret/plaintext scans.
- Update benchmark/backend/operator documentation with exact invocation,
  artifact authority, cost-input semantics, failure handling, and rollback.

## Acceptance Criteria

- [ ] Strict decoders reject Golden artifacts, hash drift, duplicate/unknown
  fields, wrong case order, or promotion-eligible claims before execution.
- [ ] An isolated fake-provider run emits two valid 500-case regression
  observation sets in corpus order and evaluates them through `memory-eval`.
- [ ] v1 output contains only its actual Top-5 surface; v2 output exposes the
  actual production RRF Top-20 and reranked/budgeted Top-5 through capture
  decorators rather than SQL/algorithm duplication.
- [ ] Scope, superseded, deletion, secret-rejected, untrusted-rejected,
  irrelevant, and out-of-scope fixture mappings have focused tests and cannot
  leak excluded IDs into candidate/final/provider surfaces.
- [ ] Provider failure, timeout, redaction, rerank fallback, hard cutoff, and
  interrupted execution produce bounded observations or fail closed without a
  partial final artifact.
- [ ] Live credentials and fixture/query/plaintext are absent from retained
  content-free reports, command output, Docker metadata, and Git.
- [ ] Missing/invalid cost basis or unauthorized live-provider mode fails
  before network activity.
- [ ] Success, failure, SIGINT, SIGTERM, and SIGHUP teardown leave no scoped
  Compose containers, networks, or volumes and remove temporary credentials.
- [ ] Existing formal Golden admission and v1 chat/prompt/Usage behavior remain
  byte-compatible and all Memory shadow flags remain default-off.
- [ ] Focused race tests, backend `go test ./...`, `go vet ./...`, Compose
  static/render tests, provider protocol tests, and the standalone full gate
  pass.
- [ ] With a separately authorized SiliconFlow credential and cost basis, one
  real 500-case v1-versus-v2 capture is produced; failures are reported as
  regression evidence and are not hidden or used for promotion.

## Definition of Done

- Production reader logic is reused, not copied.
- Tests cover strict input, capture mapping, DB authority, provider failure,
  publication, cleanup, and zero-leak boundaries.
- Backend, Compose, scripts, contracts, deployment docs, and tracking status
  agree on the operator flow and non-promotional authority.
- A content-free result summary records hashes, metrics, latency, cost, and
  failed slices without claiming human review or reader promotion.
- The working tree is committed in focused batches and the Trellis task is
  archived only after final verification.

## Technical Approach

Use the selected approach from
[`research/native-runner-design.md`](research/native-runner-design.md): an
isolated PostgreSQL/production-reader capture rather than a live-API run or an
offline ranking reimplementation.

Introduce a small internal native-regression package and command. A fixture
loader maps aliases to deterministic UUIDs, seeds current authority state, and
populates real projections in an ephemeral marked database. The v1 capture
invokes `SearchRelevant`. The v2 capture invokes
`SearchRelevantWithHybridShadow` with repository/provider decorators that copy
only typed ID/ranking/usage surfaces before production code reduces them to
content-free diagnostics. The existing `memoryeval` package remains the only
scorer.

An isolated Compose wrapper applies migrations, establishes separate seed and
API-role connections, supplies the synthetic inputs read-only, gates any live
credential through ephemeral secret material, and publishes outputs only
after both captures and evaluations validate. Cleanup is unconditional and
scoped to the random Compose project.

## Decision (ADR-lite)

**Context:** The evaluator has no reader runner, while production hybrid
diagnostics intentionally hide transient IDs. Running through live chat would
pollute production; reimplementing retrieval would measure a copy.

**Decision:** Execute the actual production Go/SQL readers in a disposable,
hash-bound PostgreSQL environment and capture transient typed surfaces through
decorators.

**Consequences:** The runner is operationally heavier and a real run requires
explicit Provider/cost authority, but results correspond to the deployed
reader, preserve least privilege, and cannot mutate active prompts or user
data.

## Implementation Plan

1. Add strict regression fixture loading, deterministic alias/fixture state
   mapping, capture types, and fake-provider unit tests.
2. Add ephemeral PostgreSQL seed/projection and v1/v2 production-reader capture
   with repository/provider decorators and focused integration tests.
3. Add the command, provider/cost gate, exclusive observation publication, and
   evaluation orchestration.
4. Add isolated Compose/script lifecycle, zero-network protocol tests,
   teardown/leak verification, and operator documentation.
5. Run offline/fake-provider quality gates; perform the real 500-case run only
   with separately authorized live credential and cost basis, then record the
   content-free comparison.

## Out of Scope

- Human review, formal Golden freeze, hidden Holdout, or promotion authority.
- Changing the active L1 reader, prompt blocks, Usage, feature-flag defaults,
  L2/L3 reader pointers, or production data.
- Reintroducing Hindsight, Mem0, Graphiti, or any other Memory engine.
- Evaluating extraction/candidate-writing quality; this task is retrieval-only.
- Automatically reading/decrypting an existing Server provider credential.
- Tuning ranking thresholds against the machine-visible `holdout` split and
  presenting that tuning as formal evidence.

## Research References

- [`research/native-runner-design.md`](research/native-runner-design.md) —
  code-backed approach comparison and selected isolation/capture semantics.
- `mm-chat/docs/contracts/memory-benchmark-workflow.md` — regression admission,
  evaluation, and promotion separation.
- `.trellis/spec/backend/memory-v2-benchmark.md` — executable benchmark and
  safety contract.
- `.trellis/spec/backend/memory-v2-hybrid-shadow.md` — production BGE/RRF/
  rerank authority and fallback contract.

## Technical Notes

- Likely implementation areas: `backend/internal/memoryeval`, a new
  `backend/internal/memorycapture` package, a new backend command,
  `compose.memory-regression.yml`, `scripts/run-memory-regression.sh`, and
  benchmark/deployment/tracking documentation.
- Protected source artifacts stay under
  `mm-chat/data/memory-benchmark/v2-regression/`, mode 0700/0600, and remain
  ignored by Git.
- The first live result may fail metric gates. That is a valid finding, not an
  implementation failure, provided artifacts are valid and failures are
  reported without suppression.

