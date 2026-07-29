# Memory v2 benchmark contracts

## 1. Scope / Trigger

Apply this contract when changing `internal/memoryauthor`,
`cmd/memory-benchmark-author`, `internal/memoryeval`, `cmd/memory-eval`, a
Memory benchmark fixture/observation producer, Memory retrieval ranking,
current-fact selection, prompt Memory injection, Memory Provider egress, or a
Memory reader promotion gate.

The benchmark authoring/evaluation toolchain is offline-only. It adds no
production API route, database object, Provider call, feature flag, or
active-reader mutation. The complete operator workflow is
documented in
`mm-chat/docs/contracts/memory-benchmark-workflow.md`.

## 2. Signatures

The versioned artifacts are:

```text
neo-chat.memory-benchmark-golden.v1
neo-chat.memory-benchmark-observations.v1
neo-chat.memory-benchmark-report.v1
neo-chat.memory-benchmark-evaluator.v1
neo-chat.memory-benchmark-fixtures.v1
neo-chat.memory-benchmark-candidates.v1
neo-chat.memory-benchmark-review-event.v1
neo-chat.memory-benchmark-freeze-manifest.v1
neo-chat.memory-benchmark-holdout-use.v1
neo-chat.memory-benchmark-regression-fixtures.v1
neo-chat.memory-benchmark-regression-corpus.v1
neo-chat.memory-benchmark-regression-audit.v1
neo-chat.memory-benchmark-regression-manifest.v1
neo-chat.memory-benchmark-regression-observations.v1
neo-chat.memory-benchmark-regression-report.v1
neo-chat.memory-benchmark-regression-evaluator.v1
```

Authoring signatures:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author generate [-root <new-protected-root>]
go run ./cmd/memory-benchmark-author review \
  [-root <protected-root>] -reviewer <human-reviewer-uuid>
go run ./cmd/memory-benchmark-author status|verify [-root <protected-root>]
go run ./cmd/memory-benchmark-author freeze \
  [-root <protected-root>] -holdout-run-id <new-uuid>
go run ./cmd/memory-benchmark-author holdout-begin \
  [-root <protected-root>] -holdout-run-id <same-uuid> \
  -output <protected-root>/holdout/<new-file>.json
```

The Go authoring entrypoints are:

```go
memoryauthor.Generate() (memoryauthor.GeneratedPool, error)
memoryauthor.PublishPool(root string, pool memoryauthor.GeneratedPool) error
memoryauthor.LoadReviewState(root string) (memoryauthor.ReviewState, error)
memoryauthor.ApplyReview(root string, input memoryauthor.ReviewInput) (memoryauthor.ReviewResult, error)
memoryauthor.Freeze(root string, input memoryauthor.FreezeInput) (memoryauthor.FrozenArtifacts, error)
memoryauthor.BeginHoldout(root string, input memoryauthor.HoldoutInput) (memoryauthor.HoldoutUse, error)
```

Machine-reviewed regression authoring is a separate command/API family:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author regression-generate \
  [-root <new-protected-regression-root>]
go run ./cmd/memory-benchmark-author regression-status|regression-verify \
  [-root <protected-regression-root>]
```

```go
memoryauthor.GenerateRegression() (memoryauthor.RegressionPool, error)
memoryauthor.AuditRegression(fixtures, corpus) (memoryeval.RegressionAudit, error)
memoryauthor.PublishRegression(root string, pool memoryauthor.RegressionPool) error
memoryauthor.LoadRegression(root string) (memoryauthor.RegressionPool, error)
memoryauthor.VerifyRegression(root string) (memoryauthor.RegressionStatus, error)
```

Draft/freeze validation:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden <draft-or-pre-frozen.json> \
  -print-freeze-hash \
  [-pretty]
```

Formal evaluation:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden <frozen-500-case.json> \
  -observations <ordered-observations.json> \
  -output <new-exclusive-report.json> \
  [-pretty]
```

The Go entrypoints are:

```go
memoryeval.DecodeGoldenSet(io.Reader) (memoryeval.GoldenSet, error)
memoryeval.DecodeObservationSet(io.Reader) (memoryeval.ObservationSet, error)
memoryeval.GoldenContentSHA256(memoryeval.GoldenSet) (string, error)
memoryeval.ValidateGoldenAdmission(memoryeval.GoldenSet) error
memoryeval.Evaluate(memoryeval.EvaluationInput) (memoryeval.Report, error)
```

Regression evaluation uses explicit inputs and never accepts `-golden` in the
same invocation:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -regression-corpus <regression-corpus.json> \
  -regression-audit <regression-audit.json> \
  -observations <ordered-regression-observations.json> \
  -output <new-exclusive-regression-report.json> \
  [-pretty]
```

```go
memoryeval.DecodeRegressionCorpus(io.Reader) (memoryeval.RegressionCorpus, error)
memoryeval.DecodeRegressionAudit(io.Reader) (memoryeval.RegressionAudit, error)
memoryeval.DecodeRegressionObservationSet(io.Reader) (memoryeval.RegressionObservationSet, error)
memoryeval.ValidateRegressionAdmission(corpus, audit) error
memoryeval.EvaluateRegression(memoryeval.RegressionEvaluationInput) (memoryeval.RegressionReport, error)
```

Native production-reader regression capture is a separate isolated command:

```bash
cd mm-chat
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --cost-basis /secure/eval/memory-regression-cost-basis.json \
  --output-dir /secure/eval/native-memory-runs

bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --credential-file /secure/input/fresh-siliconflow.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --cost-basis /secure/eval/memory-regression-cost-basis.json \
  --output-dir /secure/eval/native-memory-runs
```

```go
memorycapture.LoadProtectedRegression(root string) (memorycapture.ProtectedRegression, error)
memorycapture.SeedEphemeralDatabase(ctx, adminDB, pool, index, runID) (memorycapture.SeedResult, error)
memorycapture.PopulateProjectionVectors(ctx, adminDB, runID, embedder) (int, error)
memorycapture.CaptureProfiles(ctx, adminDB, runtimeDB, runID, index, seed, provider, hashes, cost) (memorycapture.CapturedProfile, memorycapture.CapturedProfile, error)
memorycapture.AssembleRegressionObservations(pool, capturedAt, captureID, profile) (memoryeval.RegressionObservationSet, []byte, error)
memorycapture.PublishArtifactsExclusive(directory, artifacts) (map[string]string, error)
```

## 3. Contracts

- Input is synthetic-only and declares an explicit
  `promotionEligible=false`; omitted and `true` are both invalid.
- Candidate generation is fixed at 650 cases with split `390/130/130`, language
  `455/130/65`, and every critical slice at least `65` total and `39/13/13` by
  split. The version/profile/seed/templates/order/IDs/timestamps/JSON bytes are
  deterministic and make no model, Provider, DB, clock, or network call.
  `verify` regenerates that profile in memory and requires byte equality with
  all three protected candidate artifacts before emitting status.
- The default authoring root is the gitignored
  `mm-chat/data/memory-benchmark/v1/`. The CLI creates a new root only. Inside
  this repository, no other path is valid; another Git repository,
  `secrets/`, `backup/`, symlinked components, non-private files, and in-place
  generation/freeze replacement are rejected.
- Review authority is an ordered hash chain of immutable event files. Each
  action binds its previous event, current sequence, case/content/fixture
  hashes, explicit reviewer UUID, and per-action server timestamp. Edit is
  separate from accept/reject and always clears the effective decision.
- Edit publication and completed replay must materialize all 650 current
  snapshots and revalidate global case/Memory IDs, fixture/Golden bindings,
  normalized query uniqueness, slice semantics, and current counts. Content-
  free status uses those current counts while the candidate-manifest hash
  remains the immutable generator-profile binding.
- Browser review is a temporary `127.0.0.1` server with a random one-use
  bootstrap, `HttpOnly`/`SameSite=Strict` session, CSRF, exact Host/Origin,
  loopback client verification, no CORS, `no-store`, restrictive CSP, bounded
  strict JSON, and no bulk approval endpoint. It is never a production route.
- Freeze requires all 650 current decisions, exactly 500 accepted/150
  rejected, exact language `350/100/50`, evaluator admission, a new Holdout
  UUID, and immutable fixture/Golden/freeze outputs. Any frozen directory,
  including a partial one, permanently blocks review and in-place retry.
- `holdout-begin` preflights and validates the bounded 100-case bundle, then
  exclusively commits ordinal-one `consumed.json` before publishing the
  output. A post-marker crash/failure burns the Holdout; deleting the marker
  to retry is forbidden.
- Golden artifacts contain opaque Memory IDs and aliases, not Memory bodies,
  chat transcripts, credentials, embeddings, or sensitive facts.
- Strict decoding rejects inputs over 64 MiB, duplicate JSON keys, unknown
  fields, trailing values, padded/control identifiers, invalid enums, and
  inconsistent ranking stages.
- Frozen admission requires exactly 500 human-reviewed cases, exact
  `300/100/100` Development/Validation/Holdout counts, at least 50 cases in
  every `memoryeval.CriticalSlices()` slice with at least `30/10/10`
  Development/Validation/Holdout coverage, review times no later than freeze,
  a fixture-manifest SHA-256, a precommitted Holdout UUID, and a matching
  canonical frozen-content SHA-256.
- Observations repeat the frozen/fixture bindings, name an immutable profile
  configuration SHA-256, use Candidate limit 20 and Final limit 5, preserve
  Golden order, and carry the precommitted Holdout UUID at ordinal one.
- `finalMemoryIds` is a subset of `candidateMemoryIds`;
  `injectedMemoryIds` is a subset of `finalMemoryIds`.
  `persistedMemoryIds` and `providerSentMemoryIds` are separate authority
  surfaces and must not be hidden behind self-reported booleans.
- v1 criteria are exact: Candidate Recall@20 `>=0.95`, Final Recall@5
  `>=0.90`, current-fact accuracy `>=0.95`, false injection `<=0.02`, P95
  `<=900ms`, P99 `<=1500ms`, 2-second cutoff, average/maximum prompt Memory
  `<=600/900` tokens, Provider cost ratio `<=0.15`, and zero authority leaks.
- Report failure strings and slice processing are sorted/deterministic.
  nDCG@5/MRR@5 are diagnostic until the same frozen corpus has baseline and
  candidate reports.
- The report is published through a same-directory temporary file plus an
  exclusive hard link. Existing output is never overwritten. A failed gate
  still publishes its report before returning non-zero.
- A passing report is evidence only. Evaluation never changes Memory Use/Learn,
  migration state, reader pointers, workers, or Hindsight.
- Machine regression is a distinct corpus class with
  `corpusClass=machine_reviewed_regression`,
  `admissionMode=regression_only`, and `promotionEligible=false`. Its cases
  remain `review.state=draft` with no reviewer/timestamp. Regression and Golden
  strict decoders reject each other's artifacts, and
  `ValidateGoldenAdmission` is never widened for machine review.
- The regression profile is fixed at 500 cases, split `300/100/100`, language
  `350/100/50`, and every critical slice at least 50 and `30/10/10`. The
  `holdout` label is visible regression stratification only; no UUID, ordinal,
  consumed marker, secrecy, or one-shot claim exists.
- Regression generation uses opaque hash-derived IDs and forbids case order,
  case/fixture/Memory IDs, or shared ordinals in query/Memory text. Its
  deterministic audit traverses all cases and requires zero normalized
  duplicates, ordinal/identifier shortcuts, binding failures, language/scope
  mismatches, slice semantic failures, and preference/fallback/multi-hop
  failures, plus at least 100 entity/topic-normalized query skeletons.
- The default regression root is the gitignored
  `mm-chat/data/memory-benchmark/v2-regression/`. Its final path component must
  explicitly contain `regression`; publication is create-only with `0700/0600`
  permissions. `regression-verify` regenerates and byte-compares fixtures,
  corpus, audit, and manifest. Git receives content-free status/hashes only.
- Regression observations bind corpus-content, audit-content, fixture, capture,
  profile configuration, raw input hashes, and all 500 ordered IDs. They reuse
  Candidate 20, Final 5, stage subsets, metrics, budgets, cost, and typed safety
  scoring through the same internal scorer as formal evaluation.
- A regression report repeats the class/mode and
  `promotionEligible=false`; passing cannot satisfy formal admission or change
  a reader.
- Native capture replays the exact protected four-file regression generator
  bytes before database or Provider work, then executes production
  `usermemory` v1 lexical and v2 hybrid Go/SQL paths in a fresh marked
  PostgreSQL 17 database. The benchmark layer never reimplements ranking.
- Query-time capture must use `current_user=go_api_runtime`; the privileged
  seed/vector connection is allowed only after the database name, migration
  head, empty state, and exact run marker pass. The two DSNs must name the same
  `mm_chat_memory_regression_*` database before any connection is opened.
- The baseline's candidate/final/injected surface is its actual v1 Top 5. The
  candidate's candidate surface is the captured RRF Top 20; final is the
  production rerank/token-budget Top 5; injected mirrors final offline only;
  persisted remains empty; Provider-sent contains the exact rerank documents.
- `fake_protocol` is deterministic and external-network-free. Its candidate
  profile is `native_v2_hybrid_fake_protocol`, never
  `native_v2_hybrid`, and its metrics cannot be used as reader-quality or
  promotion evidence.
- Live SiliconFlow capture requires exact run/provider/BGE/rerank/quota
  authorization plus a regular non-symlink mode-`0600` Key file. The Key value
  never enters argv, environment, Compose config, retained output, or the
  production vault path.
- Cost authority is a strict versioned same-unit document. Baseline Memory
  cost is exactly zero, candidate Memory cost is positive, and both profiles
  share the same non-zero chat denominator. Missing, duplicate, unknown,
  zero-cost, unit-drift, or denominator-drift input fails before Provider work.
- Native output uses a private new run directory and four exclusive evidence
  links followed by `run-manifest.json` as the completion marker. Failed metric
  gates retain valid reports and return non-zero; all other failures remove
  partial output. Success/failure/`SIGINT`/`SIGTERM`/`SIGHUP` destroy the exact
  random Compose database, role, containers, networks, volume, and temporary
  credentials.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Draft omits `promotionEligible=false` | Reject before hashing. |
| Candidate root is tracked, another Git repo, forbidden, symlinked, existing, or loose-permissioned | Reject before reading/writing formal content. |
| Same generator profile produces different bytes/counts/witness hash | Reject as deterministic-profile drift. |
| Review tab submits a stale sequence/content hash or a second writer is active | Reject; reload current ledger authority. |
| Published review event is partial, malformed, tampered, gapped, forked, time-regressing, or semantically invalid | Refuse replay and every subsequent mutation/freeze. |
| Case is edited after accept/reject | Append edit, clear effective decision/reviewer/time, and require a new explicit decision. |
| A locally valid edit creates a cross-case duplicate or broken global binding | Reject before publishing the event; replay independently rejects injected history. |
| Freeze has pending cases, other than 500/150 decisions, wrong language/split/slice coverage, or bad binding | Reject without creating formal authority. |
| Holdout marker exists or first run failed after marker publication | Permanently refuse another Holdout run for that corpus version. |
| Golden contains real/sensitive-data policy flags | Reject. |
| Duplicate/unknown/trailing JSON or input over 64 MiB | Reject before typed evaluation. |
| Corpus is not exactly 500 or split is not `300/100/100` | Reject frozen admission. |
| Critical slice has fewer than 50 cases, lacks `30/10/10` split coverage, or lacks matching semantic exclusions | Reject. |
| Review is draft, malformed, or later than freeze | Reject. |
| Frozen content, fixture manifest, raw file, profile, or Holdout binding drifts | Reject. |
| Observation order differs or a case is missing/unknown | Reject. |
| Final/injected stage contains an ID not present in its parent stage | Reject. |
| Cross-user, deleted, secret, untrusted, or unauthorized Provider ID reaches a forbidden surface | Produce a failing zero-tolerance report. |
| Quality/latency/token/cost criterion fails | Produce a failing report at the new path, then return non-zero. |
| Output already exists | Refuse without changing existing bytes. |
| Regression artifact is passed to the Golden decoder/evaluator, or vice versa | Reject strict decoding/admission before scoring. |
| Regression omits explicit promotion denial, uses another class/mode, or contains human reviewer/timestamp fields | Reject admission. |
| Regression count/split/language/slice or semantic-audit gate fails | Refuse publication/admission; never convert it into a formal review event. |
| Regression corpus/audit/fixture/manifest hash or byte replay drifts | Refuse verify/admission and preserve the existing protected root unchanged. |
| Regression observations contain a formal Holdout simulation, wrong audit/corpus/fixture binding, missing/reordered case, or bad stage subset | Reject before scoring. |
| Regression metric gate fails | Publish the new exclusive regression report, return non-zero, and keep `promotionEligible=false`. |
| Native capture DSN names a live/non-prefixed/different database or runtime lacks `role=go_api_runtime` | Reject before opening either database. |
| Fake protocol is labelled `native_v2_hybrid` or receives a credential file/egress network | Reject; fake output is protocol-only. |
| Live authorization/model target, mode-`0600` Key file, or cost authority is absent/invalid | Reject before network activity without echoing values. |
| Native artifact target already exists or publication races | Preserve existing bytes, remove only new links, and refuse the run. |
| Native run is interrupted before complete validation | Remove partial output and all project-scoped runtime/credential state. |

## 5. Good / Base / Bad Cases

- **Good**: a synthetic, reviewed, hash-bound 500-case corpus produces ordered
  observations for one precommitted Holdout and an exclusive report whose raw
  hashes can be independently replayed.
- **Base**: the authoring command reproducibly generates 650 private draft
  candidates and a content-free status with 650 pending decisions; neither the
  pool nor the checked-in ten-case template can pass frozen admission.
- **Bad**: generate 500 machine-authored rows, copy a fake reviewer UUID/time,
  bulk approve, preserve approval after edit, delete a consumed marker to rerun
  Holdout, omit privacy flags, overwrite evidence, or let a passing evaluator
  change the active reader.
- **Regression good**: generate the fixed private v2 artifacts, replay a passed
  zero-shortcut semantic audit, bind ordered observations, and publish an
  explicit non-promotional regression report through the shared scorer.
- **Regression base**: the corpus and audit verify byte-for-byte, but no reader
  observations exist yet; status can claim readiness only, not a passing
  reader.
- **Regression bad**: relabel machine output as `human_reviewed`, invent a
  Holdout UUID, accept a weak/failed audit, use ordinal hints, or present a
  passing fixture-oracle protocol smoke as reader evidence.

## 6. Tests Required

- Strict decoder tests: duplicate keys, unknown fields, trailing JSON, explicit
  promotion denial, bounded input, enums, identifiers, and stage subsets.
- Golden tests: checked-in draft validation/non-admission, exact count/splits,
  every critical slice, semantic slice labels, human review, timestamps, and
  frozen hash drift.
- Binding tests: raw hashes, fixture hash, ordered exact case set, precommitted
  Holdout ID, ordinal one, and freeze/capture time window.
- Metric tests: Recall@20, Final Recall@5, current-vs-superseded, false
  injection, nDCG/MRR, P95/P99, hard cutoff, average/max tokens, and exact 15%
  integer cost boundary without overflow.
- Safety tests: cross-user/out-of-scope, deletion, secret persistence/exposure,
  untrusted-source persistence, and forbidden Provider egress are all zero.
- Command tests: freeze output remains non-promotional, argument modes are
  exclusive, report mode is `0600`, and an existing output remains byte-identical.
- Authoring generator tests: byte-identical fixture/Golden/manifest replay,
  exact pool counts, normalized duplicate rejection, semantic fixture binding,
  and exact 500-case feasibility witness.
- Authoring storage/ledger tests: path/permission/symlink refusal, exclusive
  publication, single writer, kill-safe event publication, restart replay,
  stale checkpoint tolerance, partial/tamper/gap/fork/time-regression refusal,
  and edit invalidation.
- Authoring HTTP tests: loopback-only listener, exact Host/Origin, one-use
  bootstrap, session/CSRF, no CORS/no-store/CSP, strict bodies, stale action
  refusal, and no content access after freeze.
- Authoring freeze/Holdout tests: test-only explicit 500/150 decisions, exact
  evaluator admission, immutable binding replay, output preflight before
  marker, marker before bundle, and permanent second-run refusal.
- Regression generator/audit tests: byte determinism; exact 500, split,
  language, and slice counts; opaque/no-ordinal text; at least 100 normalized
  skeletons; zero semantic counters; deliberate ordinal, weak fallback, and
  weak multi-hop failures; private exclusive storage; byte replay; and v1
  protected-profile non-regression.
- Regression evaluator tests: cross-schema rejection; explicit promotion
  denial; no human attestation; corpus/audit content binding; exact ordered
  observations; absence of Holdout authority; shared metric/safety results;
  failed-report publication; and exclusive output preservation.
- Native capture tests: fixed generator byte admission; deterministic alias
  mapping; all fixture-state exclusions; real v1/RRF/rerank/final/Provider ID
  decorators; fallback/cutoff; strict cost/live authorization; private
  exclusive publication; plaintext/credential scans; PostgreSQL 17 exact
  extension profile and all 500 cases; fake internal-only network; and
  success/failure/`SIGINT`/`SIGTERM`/`SIGHUP` teardown.
- Run `go test -race ./internal/memoryauthor ./cmd/memory-benchmark-author
  ./internal/memoryeval ./cmd/memory-eval ./internal/memorycapture
  ./cmd/memory-regression-capture`, `bash scripts/test-memory-regression.sh`,
  `go test ./...`, and `go vet ./...` from their owning product directories.

## 7. Wrong vs Correct

### Wrong

```text
machine-generate 500 cases with copied human_reviewed labels
-> preserve approval after edits
-> run Holdout repeatedly or delete the consumed marker
-> overwrite report.json until passed
-> enable the candidate reader
```

This fabricates authority, leaks Holdout into tuning, destroys failed evidence,
and couples scoring to activation.

### Correct

```text
deterministic 650-case private draft (promotionEligible=false)
-> case-by-case accept/edit/reject ledger
-> exact 500 accepted + 150 rejected
-> freeze exact fixture/Golden/review hashes + precommit Holdout UUID
-> complete Dev/Validation
-> commit ordinal-one marker before one ordered Holdout
-> publish a new exclusive report
-> request a separate reader-promotion decision
```

The evaluator proves the artifact chain and metrics while remaining unable to
change production.

### Correct machine regression

```text
fixed 500-case synthetic v2 profile
-> deterministic zero-shortcut semantic audit
-> private create-only corpus/audit/manifest
-> ordered observations without Holdout authority
-> shared scorer
-> explicit regression_only, promotionEligible=false report
```

This lane makes automated regressions useful without laundering them into the
formal human-review lifecycle.
