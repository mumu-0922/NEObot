# Memory benchmark authoring

`memoryauthor` builds and protects the synthetic Memory v2 benchmark before it
is admitted by `internal/memoryeval`. It generates a deterministic oversized
candidate pool, records explicit human decisions, freezes exactly 500 accepted
cases, and consumes the precommitted Holdout once. It also owns a separate
500-case machine-reviewed regression corpus that has no human-review, freeze,
Holdout, or promotion authority.

The package is offline-only. It has no database, Provider, production API,
reader, worker, or feature-flag dependency.

## Responsibilities

- Generate the versioned 650-case Chinese/mixed/English candidate fixture and
  Golden pair byte-for-byte deterministically.
- Strictly validate fixture, candidate, ledger, checkpoint, freeze, and
  Holdout artifacts with bounded input and hash bindings.
- Diagnose duplicate queries, fixture/Golden reference drift, semantic slice
  evidence, and exact split/language/slice feasibility.
- Append hash-chained review events and replay them as the only review-state
  authority; edits invalidate earlier decisions.
- Serve a loopback-only browser workflow with explicit per-case
  accept/edit/reject actions and no bulk approval.
- Freeze only an exact reviewed `300/100/100` and `350/100/50` corpus, then
  reuse `memoryeval.ValidateGoldenAdmission` rather than copying its gates.
- Publish a consumed marker before exposing the one allowed Holdout bundle.
- Generate, semantically audit, publish, and byte-replay the independent v2
  regression corpus without changing the v1 formal-authoring profile.

## Operator command

Run from any directory inside the repository:

```bash
cd mm-chat/backend

# Creates a new root only; never overwrites existing authoring state.
go run ./cmd/memory-benchmark-author generate

# The UUID identifies the actual human reviewer. The command prints one
# loopback URL whose bootstrap token is in the URL fragment.
go run ./cmd/memory-benchmark-author review \
  -reviewer '<reviewer-uuid>'

go run ./cmd/memory-benchmark-author status
go run ./cmd/memory-benchmark-author verify

# These commands are for the later review/freeze operational task.
go run ./cmd/memory-benchmark-author freeze \
  -holdout-run-id '<precommitted-holdout-uuid>'
go run ./cmd/memory-benchmark-author holdout-begin \
  -holdout-run-id '<precommitted-holdout-uuid>' \
  -output ../data/memory-benchmark/v1/holdout/run-input.json

# Separate machine-reviewed regression lane. These commands never create
# human review or formal Holdout authority.
go run ./cmd/memory-benchmark-author regression-generate
go run ./cmd/memory-benchmark-author regression-status
go run ./cmd/memory-benchmark-author regression-verify
```

The default protected root is `mm-chat/data/memory-benchmark/v1/`. A custom
root is accepted only outside a Git repository or under the repository's
`mm-chat/data/memory-benchmark/<version>/` boundary. `secrets/`, `backup/`,
symlinked paths, source paths, non-private files, and existing generation roots
are rejected.

The regression commands default to
`mm-chat/data/memory-benchmark/v2-regression/`. The final path component must
explicitly contain `regression`, generation is exclusive, and every artifact
is bound to the fixed v2 profile. A `holdout` case label in this lane is only a
visible regression stratum; it is not a secret or one-shot formal Holdout.

## Artifact layout

```text
v1/
├── candidate-fixtures.json       # synthetic fixture content, mode 0600
├── candidate-golden.json         # 650 draft cases, mode 0600
├── candidate-manifest.json       # content-free counts and hashes
├── review/
│   ├── events/                    # immutable, ordered, hash-chained events
│   ├── checkpoint.json            # replaceable derived cache
│   └── writer.lock                # process-scoped advisory lock
├── frozen/                        # created only after exact admission
│   ├── fixture-manifest.json
│   ├── golden.json
│   └── freeze-manifest.json
└── holdout/
    ├── consumed.json              # exclusive ordinal=1 marker
    └── run-input.json             # published only after the marker
```

Candidate and formal content remains Git-external. Only content-free hashes,
counts, states, and documentation may be committed.

The regression layout is intentionally simpler and cannot be consumed by the
human-review/freeze commands:

```text
v2-regression/
├── fixtures.json                 # 500 synthetic fixtures, mode 0600
├── corpus.json                   # regression-only cases, mode 0600
├── audit.json                    # content-free semantic audit, mode 0600
└── manifest.json                 # exact raw/content hash bindings, mode 0600
```

Its audit rejects normalized duplicates, identifier/ordinal shortcuts,
language or scope mismatch, weak preference/fallback/multi-hop semantics, and
fixture/slice binding drift. Query skeletons replace known entities and topics
before counting; the admitted profile currently contains 431 distinct
skeletons, not ordinal-parameterized copies.

## Review invariants

- The reviewer UUID is supplied explicitly when starting the server.
- `reviewedAt` is captured only when that reviewer performs one accept/reject
  request; there is no prefilled decision or `approve all` endpoint.
- Every request binds the last ledger sequence and current case-content hash.
- Edit is a separate event. It returns the case to `pending` and removes the
  prior effective reviewer/timestamp until a new decision is submitted.
- Before an edit event is published, the tentative 650-case draft is
  materialized and revalidated globally. Replay performs the same check for
  case/Memory ID uniqueness, fixture bindings, normalized query duplicates,
  and current split/language/slice counts.
- Complete event files survive restart. An invalid filename, partial published
  event, hash mismatch, sequence gap, fork, or malformed checkpoint fails
  closed. A valid stale checkpoint never displaces ledger authority.

## Public package entry points

| API | Purpose |
| --- | --- |
| `Generate` / `ValidatePool` / `Diagnose` | Build and verify the deterministic candidate pool. |
| `ValidateFormalRoot` / `PublishPool` / `LoadPool` | Enforce the protected filesystem boundary. |
| `LoadReviewState` / `ApplyReview` | Replay and append explicit review actions. |
| `StartReviewServer` | Start the temporary loopback-only browser workflow. |
| `Freeze` / `LoadFrozen` | Publish and independently replay the exact frozen corpus. |
| `BeginHoldout` | Commit ordinal one, then publish the bounded Holdout bundle. |
| `CurrentStatus` / `Verify` | Emit content-free state; `Verify` also regenerates and byte-compares the fixed candidate profile. |
| `GenerateRegression` / `AuditRegression` / `ValidateRegressionPool` | Build and machine-audit the separate regression corpus. |
| `ValidateRegressionRoot` / `PublishRegression` / `LoadRegression` | Enforce private, exclusive regression storage. |
| `CurrentRegressionStatus` / `VerifyRegression` | Emit content-free regression status and byte-replay every fixed artifact. |

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/memoryauthor ./cmd/memory-benchmark-author \
  ./internal/memoryeval ./cmd/memory-eval
go test ./...
go vet ./...
```

See [`DESIGN.md`](./DESIGN.md) and the operator contract at
[`../../../docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).
