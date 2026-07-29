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
- Run `go test -race ./internal/memoryauthor ./cmd/memory-benchmark-author
  ./internal/memoryeval ./cmd/memory-eval`, `go test ./...`, and `go vet ./...`
  from `mm-chat/backend`.

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
