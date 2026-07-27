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

## 4. Review and freeze

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
When all 500 cases are reviewed, set `lifecycle.state=frozen`, set
`lifecycle.frozenAt`, precommit a new `lifecycle.holdoutRunId`, and leave
`lifecycle.frozenContentSha256` empty while calculating the canonical hash:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden /secure/eval/memory-golden.json \
  -print-freeze-hash \
  -pretty
```

Copy the returned hash into `lifecycle.frozenContentSha256`. The hash clears
only its own field before canonical Go JSON encoding; it binds every other
field, including review records, freeze time, Holdout UUID, criteria, and case
order. The hash report always says `promotionEligible=false`.

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

## 7. Keep promotion separate

A passing report is evidence, not authority. It does not toggle a feature flag,
change an active reader pointer, enable Learn/Use, start a worker, run a
migration, or activate Hindsight. Promotion remains a separately authorized
operator action after the owning implementation phase passes its shadow,
canary, deletion, resource, and rollback gates.
