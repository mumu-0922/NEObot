# Memory benchmark Golden workflow

This contract establishes the offline evidence gate for Neo Chat Memory v2.
It deliberately precedes every Memory v2 migration, worker, Provider adapter,
projection, and UI change. Running `cmd/memory-eval` performs no network or
database operation and cannot read or mutate live user Memory.

The checked-in
[`memory-benchmark-golden-draft-template.json`](./memory-benchmark-golden-draft-template.json)
contains ten synthetic examples only. It is incomplete, unreviewed, and
explicitly `promotionEligible=false`. It must never be presented as a frozen
500-case corpus, a passing report, or reader-promotion authority.

## 1. Artifact chain

The v1 chain mirrors the proven RAG promotion lifecycle without reusing RAG
business metrics:

```text
synthetic fixture manifest
  -> draft Golden corpus
  -> case-by-case human review
  -> frozen canonical content SHA-256
  -> complete Development + Validation observations
  -> one precommitted Holdout run
  -> one ordered 500-case observation artifact
  -> one exclusive deterministic report
  -> separate operator promotion decision
```

The versioned schemas are:

```text
neo-chat.memory-benchmark-golden.v1
neo-chat.memory-benchmark-observations.v1
neo-chat.memory-benchmark-report.v1
neo-chat.memory-benchmark-evaluator.v1
```

Machine-reviewed regression is a second, non-promotional chain:

```text
fixed synthetic v2 profile
  -> 500 bound fixtures/cases
  -> deterministic semantic audit
  -> private immutable manifest
  -> ordered regression observations
  -> exclusive regression-only report
```

Its schemas are deliberately not Golden lifecycle states:

```text
neo-chat.memory-benchmark-regression-fixtures.v1
neo-chat.memory-benchmark-regression-corpus.v1
neo-chat.memory-benchmark-regression-audit.v1
neo-chat.memory-benchmark-regression-manifest.v1
neo-chat.memory-benchmark-regression-observations.v1
neo-chat.memory-benchmark-regression-report.v1
neo-chat.memory-benchmark-regression-evaluator.v1
```

Regression declares `corpusClass=machine_reviewed_regression`,
`admissionMode=regression_only`, and `promotionEligible=false`. Strict decoding
rejects a regression artifact on the Golden path and rejects a Golden artifact
on the regression path. Both paths share scoring code only after their
different admission and binding checks pass.

All Golden and observation JSON is decoded with a 64 MiB limit, duplicate-key
rejection, unknown-field rejection, one-value enforcement, bounded identifiers,
and exact enum validation.

## 2. Data boundary

Every corpus must declare:

```json
{
  "promotionEligible": false,
  "dataPolicy": {
    "syntheticOnly": true,
    "containsRealUserData": false,
    "containsSensitiveData": false
  }
}
```

Golden files contain queries, opaque Memory IDs, fixture aliases, scope aliases,
and exclusion reasons. They do not contain chat transcripts, real user names,
real Memory bodies, credentials, Provider requests, embeddings, or sensitive
facts. Secret tests use non-secret synthetic fixture sentinels and opaque IDs;
they are tests of rejection behavior, not copies of a credential.

The external synthetic fixture manifest is a separate artifact and is bound by
`fixtureManifestSha256`. Formal admission rejects a missing or changed manifest
hash. Observation files repeat that exact hash.

## 3. Curate exactly 500 cases

The frozen v1 corpus contains exactly 500 cases with this fixed split:

```text
Development: 300
Validation:   100
Holdout:      100
```

Every critical slice must have at least 50 reviewed cases and must include at
least 30 Development, 10 Validation, and 10 Holdout cases:

```text
stable_fact
preference_instruction
project_decision
chinese_paraphrase
mixed_language_entity
temporal_correction
unrelated_negative
untrusted_source
secret_rejection
scope_isolation
deletion
failure_fallback
multi_hop
```

Cases may carry multiple compatible slice labels. The validator also prevents
label-only coverage:

- `chinese_paraphrase` must be `zh` or `mixed`;
- `mixed_language_entity` must be `mixed`;
- `project_decision` must bind a Project alias;
- `temporal_correction` must define current IDs and a `superseded` exclusion;
- `unrelated_negative` must expect no Memory;
- untrusted-source, secret, deletion, and isolation slices must include their
  matching typed exclusions.

Expected relevant IDs are limited to five so Final Recall@5 remains achievable.
`expectedCurrentMemoryIds` must be a subset of the relevant IDs. A Memory ID
cannot be both relevant and excluded in the same case.

### 3.1 Generate the protected candidate pool

The supported authoring command creates a deterministic 650-case pool before
human review:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author generate
```

Its default root is the gitignored, standalone-excluded directory
`mm-chat/data/memory-benchmark/v1/`. The command creates a new root only and
never reads, merges, or overwrites an existing authoring root. Inside this
repository it rejects every output outside
`mm-chat/data/memory-benchmark/<version>/`; it also rejects another Git
repository, `secrets/`, `backup/`, symlink components, loose permissions, and
non-regular artifacts.

The v1 candidate profile is fixed and reproducible:

```text
Candidate cases:                  650
Development/Validation/Holdout:   390 / 130 / 130
Chinese/mixed/English:            455 / 130 / 65
Per critical slice:               >=65 total and >=39 / 13 / 13 by split
Generator/profile/seed:           versioned constants
Model, Provider, DB, live Memory:  none
```

The pool includes a hashed diagnostic witness proving that an exact
`300/100/100`, `350/100/50`, and per-slice `30/10/10` selection exists. The
witness is not accepted automatically and is not human review authority.
Candidate fixture/Golden content and review events remain outside Git. A
committed status may contain only versions, counts, states, and hashes.

### 3.2 Generate the machine-reviewed v2 regression corpus

Use this lane when deterministic regression coverage is useful but genuine
human-review and hidden Holdout authority do not exist:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author regression-generate
go run ./cmd/memory-benchmark-author regression-status
go run ./cmd/memory-benchmark-author regression-verify
```

The default private root is
`mm-chat/data/memory-benchmark/v2-regression/`. Generation creates it once with
directory mode `0700` and file mode `0600`; it never overwrites or repairs an
existing/partial root. `verify` regenerates all fixture, corpus, audit, and
manifest bytes in memory and requires exact equality.

The fixed profile is:

```text
Cases:                           500
Development/Validation/Holdout: 300 / 100 / 100
Chinese/mixed/English:           350 / 100 / 50
Every critical slice:            >=50 and >=30 / 10 / 10 by split
Entity/topic-normalized query skeletons: >=100 (current profile: 431)
Model, Provider, DB, live Memory: none
```

Case, fixture, and Memory IDs are deterministic opaque hashes. Query and
Memory text contains no case ordinal or logical ID. The content-free audit
traverses all 500 cases and fails on any normalized duplicate, shared numeric
or identifier shortcut, fixture/scope/state binding drift, language/scope-text
mismatch, or weak slice semantics. In particular:

- preference/instruction must appear in both the query and relevant Memory;
- failure/fallback must express timeout/error/degradation and an authorized
  saved fallback in both surfaces;
- multi-hop must require at least two distinct active relevant Memories and
  composition of a constraint with an action;
- temporal correction requires current and superseded contradictory evidence;
- negative slices require the matching rejected/deleted/cross-user evidence;
- Global text says Global/account-wide, Project text names the Project, and
  Conversation text names the current Project conversation;
- English stays English, Chinese contains Chinese, and mixed-language cases
  deliberately contain both instead of accidental query/content mismatch.

The complete fixtures/corpus/audit remain Git-external. The committed
[`memory-benchmark-regression-v2-status.json`](../tracking/memory-benchmark-regression-v2-status.json)
contains only counts, verdict, and hashes.

## 4. Review and freeze

The current protected v1 review attempt is not formal authority: its 650
accept events were explicitly recanted because the cases were not inspected.
Do not freeze, rewrite, truncate, or relabel that ledger. Its content-free
disposition is recorded in
[`memory-benchmark-v1-review-disposition.json`](../tracking/memory-benchmark-v1-review-disposition.json).
Any future formal corpus requires a new clean human-review attempt; the v2
regression lane cannot repair or replace this requirement.

New cases begin with:

```json
{"state":"draft"}
```

After a human checks the fixture setup, query, scope, expected current facts,
relevant IDs, exclusions, language, and slice labels, record:

```json
{
  "state": "human_reviewed",
  "reviewerId": "<reviewer UUID>",
  "reviewedAt": "<RFC3339 timestamp>"
}
```

Do not copy reviewer identities or timestamps onto machine-generated cases.
Start the loopback-only review workflow with the actual human reviewer's UUID:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author review \
  -reviewer '<reviewer-uuid>'
```

The printed URL contains a random bootstrap token in its fragment. The server
listens only on `127.0.0.1` with an OS-selected port and is not part of the
production frontend, API, or Compose deployment. It has no bulk approval
endpoint. Each accept/reject records one explicit case-bound action and its
server timestamp. Saving an edit is a separate action: it changes the content
hash, invalidates the previous effective decision, and requires another
accept/reject.

Review events are private immutable files with an exact sequence and previous
event hash. The ledger, not its replaceable checkpoint, is authority. Restart
replays complete events; unknown/partial published entries, a gap, fork,
tamper, stale request, invalid edit, symlink, or loose permission fails closed.

Every edit is also checked against the materialized 650-case current draft
before its event is published. Resume repeats that global validation, including
case/Memory ID uniqueness, fixture binding, normalized query duplicates, and
slice semantics. `status` reports split/language/slice counts from current
review snapshots while preserving the immutable candidate-manifest hash.

Inspect only content-free progress with:

```bash
go run ./cmd/memory-benchmark-author status
go run ./cmd/memory-benchmark-author verify
```

`verify` regenerates the fixed source profile in memory and requires the
fixture, Golden, and candidate-manifest bytes to match the protected pool
exactly before it emits the content-free status.

Freeze requires all 650 candidates to have a current decision, exactly 500
accepted and 150 rejected, exact split/language counts, every evaluator gate,
and a new precommitted Holdout UUID:

```bash
go run ./cmd/memory-benchmark-author freeze \
  -holdout-run-id '<new-precommitted-holdout-uuid>'
```

The command derives review records from the ledger, writes the accepted
fixture/Golden/freeze manifest privately and exclusively, calculates the
canonical hash, and calls `memoryeval.ValidateGoldenAdmission`. It never fills
missing decisions, reallocates a split, widens a threshold, or overwrites an
existing/partial freeze. The review server refuses to expose cases after a
frozen directory exists.

Independently replay the resulting canonical hash when required:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden ../data/memory-benchmark/v1/frozen/golden.json \
  -print-freeze-hash \
  -pretty
```

The hash clears only its own field before canonical Go JSON encoding; it binds
every other field, including review records, freeze time, Holdout UUID,
criteria, and case order. The hash report always says
`promotionEligible=false`.

## 5. Produce observations

PR1 defines the observation contract but intentionally includes no runtime
capture adapter. Each future reader phase must add its own no-network unit
producer and separately authorized live/shadow producer without weakening this
contract.

One observation file contains the exact 500 Golden cases in Golden order and
binds:

- Golden corpus ID and frozen content SHA-256;
- synthetic fixture manifest SHA-256;
- capture UUID and timestamps;
- profile role (`baseline`, `candidate`, or `shadow`), reader version, and
  configuration SHA-256;
- exact Candidate limit 20 and Final limit 5;
- the precommitted Holdout UUID with `ordinal=1`;
- same-unit integer Memory and chat Provider-cost microunits.

Each case records the real pipeline stages:

```text
candidateMemoryIds -> finalMemoryIds -> injectedMemoryIds
persistedMemoryIds
providerSentMemoryIds
latencyMilliseconds
promptMemoryTokens
hardCutoffApplied
fallback = none | exact_bm25 | lexical_v1 | no_memory
```

Final IDs must be a subset of Candidate IDs, and injected IDs must be a subset
of Final IDs. The evaluator derives authority failures from typed Golden
exclusions and these observed surfaces; a producer cannot self-attest
`leaked=false`.

Development and Validation may be inspected repeatedly by future capture
tools. Holdout is run only after those profiles are fixed. The one ordered
formal observation is then assembled without changing prior observations.

Immediately before the future Native observation producer receives Holdout
input, consume the exact precommitted run once:

```bash
go run ./cmd/memory-benchmark-author holdout-begin \
  -holdout-run-id '<same-precommitted-holdout-uuid>' \
  -output ../data/memory-benchmark/v1/holdout/run-input.json
```

All output-path and frozen-artifact preflights finish first. The command then
publishes an exclusive `consumed.json` marker with ordinal one before it
publishes the bounded 100-case bundle. Marker existence permanently refuses a
second attempt. A crash or downstream failure after marker creation taints the
Holdout and requires a new corpus version and new review/freeze evidence; do
not delete or roll back the marker to retry.

This read-once property is an operational toolchain contract backed by
`0700`/`0600` storage. It does not claim to prevent the local machine owner
from bypassing the tool and copying files directly.

## 6. Evaluate once into a new path

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden /secure/eval/memory-golden.json \
  -observations /secure/eval/memory-candidate-observations.json \
  -output /secure/eval/memory-candidate-report.json \
  -pretty
```

The output path is published exclusively and is never replaced. A failed gate
still leaves a complete immutable report and then returns a non-zero status.
The command prints the exact report SHA-256 for review.

The evaluator applies the same absolute criteria globally and to every
critical slice:

- Candidate Recall@20 `>= 0.95`;
- Final Recall@5 `>= 0.90`;
- current-fact accuracy `>= 0.95`;
- false-injection case rate `<= 0.02`;
- P95 `<= 900 ms`, P99 `<= 1500 ms`, and zero 2-second cutoff violations;
- average prompt Memory `<= 600` tokens and per-case maximum `<= 900`;
- Memory Provider cost / corresponding chat Provider cost `<= 0.15` globally;
- zero cross-user/out-of-scope, deletion, secret, untrusted-source, or
  unauthorized Provider-egress cases.

Current-fact accuracy requires all expected current IDs in the injected set and
no `superseded` ID. False injection is case-level: any injected ID outside the
case's relevant allowlist makes that case false. nDCG@5 and MRR@5 are emitted as
diagnostics so a later reranker must prove benefit against the same frozen
baseline; v1 does not fabricate an absolute gain before baseline evidence
exists.

### 6.1 Evaluate machine regression without formal Holdout authority

Regression evaluation requires the exact admitted corpus and its passed audit:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -regression-corpus ../data/memory-benchmark/v2-regression/corpus.json \
  -regression-audit ../data/memory-benchmark/v2-regression/audit.json \
  -observations /secure/eval/memory-regression-observations.json \
  -output /secure/eval/memory-regression-report.json \
  -pretty
```

Regression observations bind the corpus-content, audit-content, fixture, raw
input, profile configuration, capture, and exact ordered 500-case IDs. They use
the same Candidate limit 20, Final limit 5, stage-subset rules, typed leakage
surfaces, budgets, costs, and metric implementation as formal evaluation.

They do not contain a Holdout UUID or ordinal. The visible `holdout` split is
only regression stratification. A passing report always repeats the regression
class/mode and `promotionEligible=false`; it cannot satisfy
`ValidateGoldenAdmission` or authorize a reader change.

## 7. Keep promotion separate

A passing report is evidence, not authority. It does not toggle a feature flag,
change an active reader pointer, enable Learn/Use, start a worker, run a
migration, or activate Hindsight. Promotion remains a separately authorized
operator action after the owning implementation phase passes its shadow,
canary, deletion, resource, and rollback gates.
