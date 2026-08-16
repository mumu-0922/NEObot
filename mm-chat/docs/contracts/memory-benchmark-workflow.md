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
neo-chat.memory-regression-profile-config.v3
neo-chat.memory-regression-profile-config.v4
neo-chat.memory-regression-profile-config.v5
neo-chat.memory-regression-profile-config.v6
neo-chat.memory-regression-profile-config.v7
neo-chat.memory-regression-profile-config.v8
neo-chat.memory-regression-profile-config.v9
neo-chat.memory-regression-profile-config.v10
neo-chat.memory-regression-profile-config.v11
neo-chat.memory-regression-profile-config.v12
neo-chat.memory-regression-profile-config.v13
neo-chat.memory-regression-profile-config.v14
neo-chat.memory-regression-profile-config.v15
neo-chat.memory-regression-profile-config.v16
neo-chat.memory-regression-relevance-calibration.v3
neo-chat.memory-regression-relevance-calibration.v4
neo-chat.memory-regression-relevance-calibration.v5
neo-chat.memory-regression-relevance-calibration.v6
neo-chat.memory-regression-relevance-calibration.v7
neo-chat.memory-regression-relevance-calibration.v8
neo-chat.memory-regression-relevance-calibration.v9
neo-chat.memory-regression-relevance-calibration.v10
neo-chat.memory-regression-relevance-calibration.v11
neo-chat.memory-regression-relevance-calibration.v12
neo-chat.memory-regression-relevance-calibration.v13
neo-chat.memory-regression-relevance-calibration.v14
neo-chat.memory-regression-relevance-calibration.v16
neo-chat.memory-regression-relevance-validation.v1
neo-chat.memory-regression-relevance-validation.v15
neo-chat.memory-regression-relevance-run.v1
neo-chat.memory-regression-relevance-validation-run.v15
neo-chat.memory-regression-cost-basis.v2
neo-chat.memory-regression-cost-basis.v3
neo-chat.memory-regression-cost-basis.v4
neo-chat.memory-regression-cost-basis.v5
neo-chat.memory-regression-cost-basis.v6
neo-chat.memory-regression-cost-basis.v7
neo-chat.memory-regression-cost-basis.v8
neo-chat.memory-regression-cost-basis.v9
neo-chat.memory-regression-cost-basis.v10
neo-chat.memory-regression-cost-basis.v11
neo-chat.memory-cloud-candidate-judge-input.v1
neo-chat.memory-cloud-candidate-judge-output.v1
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

### 3.2 Generate the machine-reviewed v2, v3, v4, or v5 regression corpus

Use this lane when deterministic regression coverage is useful but genuine
human-review and hidden Holdout authority do not exist:

```bash
cd mm-chat/backend
go run ./cmd/memory-benchmark-author regression-generate
go run ./cmd/memory-benchmark-author regression-status
go run ./cmd/memory-benchmark-author regression-verify

# Separately versioned repair; these commands never replace the v2 root.
go run ./cmd/memory-benchmark-author regression-v3-generate
go run ./cmd/memory-benchmark-author regression-v3-status
go run ./cmd/memory-benchmark-author regression-v3-verify

# Subject/value and task-event repair; these commands preserve v2/v3.
go run ./cmd/memory-benchmark-author regression-v4-generate
go run ./cmd/memory-benchmark-author regression-v4-status
go run ./cmd/memory-benchmark-author regression-v4-verify

# Universal-negative repair; these commands preserve v2/v3/v4.
go run ./cmd/memory-benchmark-author regression-v5-generate
go run ./cmd/memory-benchmark-author regression-v5-status
go run ./cmd/memory-benchmark-author regression-v5-verify
```

The legacy commands default to the private
`mm-chat/data/memory-benchmark/v2-regression/` root. Explicit v3/v4/v5 commands
default to their matching versioned roots. Generation creates a root once with
directory mode `0700` and file mode `0600`; it never overwrites or repairs an
existing/partial root. `verify` selects the exact generator from the protected
tuple, regenerates all fixture, corpus, audit, and manifest bytes in memory,
and requires exact equality. Unknown tuples and v2/v3/v4/v5 artifact mixing
fail closed.

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

The historical v2 `unrelated_negative` pair is frozen even though its query
asks whether an unrelated record should influence the answer while its only
candidate literally says that it has no bearing. The v3 repair changes only
that contract: the query is a genuine agenda-heading task, while the candidate
is a same-entity/same-scope weather-board observation that cannot answer it.
The v3 semantic audit requires the task/observation markers and rejects
`unrelated`, `无关`, `no bearing`, and `没有关系` from both query and candidate.
V4 preserves that query shape but replaces every positive permutation with an
explicit compatible Subject/current/old value pair. Its same-entity/same-scope
negative is instead a facilities/weather inspection that omits the queried
Subject and any agenda/meeting/task-event claim. The v4 audit rejects missing
or ambiguous Subjects, cross-Subject current/superseded values, repeated query
Subjects, and task-event mutation in the negative. V5 preserves those aligned
positives but replaces the hard negative with a same-entity/same-scope physical
observation: a commemorative mug on the lounge's left third shelf. Its audit
rejects every known Subject/current/old value in both languages and every v3/v4
meeting, agenda, discussion, facilities, weather, and sunshine event marker
from that candidate.
All counts, criteria, draft state, and non-promotional boundaries stay fixed.

The complete fixtures/corpus/audit remain Git-external. The committed
[`memory-benchmark-regression-v2-status.json`](../tracking/memory-benchmark-regression-v2-status.json)
and
[`memory-benchmark-regression-v3-status.json`](../tracking/memory-benchmark-regression-v3-status.json)
and
[`memory-benchmark-regression-v4-status.json`](../tracking/memory-benchmark-regression-v4-status.json)
and
[`memory-benchmark-regression-v5-status.json`](../tracking/memory-benchmark-regression-v5-status.json)
contain only counts, verdicts, and hashes. Existing native-capture commands
default to v2 for compatibility; v3/v4/v5 require an explicit protected root and
their own exact configuration identity. Authoring performs no Provider call
and does not itself authorize a live run, Validation, or promotion.

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

The evaluator can admit any exact known corpus/audit pair, but observations
cannot be reused across v2/v3/v4/v5 because every content and fixture binding
changes. The native-capture wrapper keeps v2 as its compatibility default. To
select the current v5 corpus, pass its protected root explicitly:

```bash
bash scripts/run-memory-regression.sh \
  --regression-root /secure/eval/v5-regression \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_accuracy \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-accuracy-cost-v8.json \
  --output-dir /secure/eval/native-memory-runs
```

The loader accepts only a known exact generator tuple and verifies all raw
artifact bytes. Those raw hashes enter the capture configuration, so v2, v3,
v4, and v5 produce different configuration SHA-256 values and cannot share
observations. The `fake_protocol` example proves lifecycle only. V4's executed
fake gate completed all 300 Development cases and clean teardown but failed the
expected non-quality fake-judge false-injection metric; it is not a live v4
quality result and grants no Provider, Validation, or promotion authority.
The later v4 and v5 live Development runs are separate immutable failed
aggregate-only evidence; neither can be rebound to another corpus version or
used to infer a case ID or Judge response body.

Regression observations bind the corpus-content, audit-content, fixture, raw
input, profile configuration, capture, and exact ordered 500-case IDs. They use
the same Candidate limit 20, Final limit 5, stage-subset rules, typed leakage
surfaces, budgets, costs, and metric implementation as formal evaluation.

They do not contain a Holdout UUID or ordinal. The visible `holdout` split is
only regression stratification. A passing report always repeats the regression
class/mode and `promotionEligible=false`; it cannot satisfy
`ValidateGoldenAdmission` or authorize a reader change.

### 6.2 Capture the production native readers in isolation

`cmd/memory-regression-capture` is the only native regression producer. It
executes the production `usermemory` v1 Global lexical reader and v2 hybrid
shadow reader in a disposable PostgreSQL 17 database. Repository and Provider
decorators capture the transient RRF Top 20, reranked/token-budgeted Top 5, and
exact union of BGE-rerank/cloud-judge Provider-egress Memory IDs; the benchmark
package does not copy the ranking algorithm.

Use the wrapper from the product root:

```bash
cd mm-chat
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode full_regression \
  --cost-basis /secure/eval/memory-regression-cost-basis.json \
  --output-dir /secure/eval/native-memory-runs
```

The deterministic fake Provider has no external network and is permanently
reported as `native_v2_hybrid_fake_protocol`. It validates fixture replay,
PostgreSQL seeding, production SQL/Go call order, capture mapping, evaluation,
exclusive publication, leak checks, and teardown. Its ranking, latency, and
cost results are protocol fixtures and are not native-reader quality evidence.

A live reader-quality comparison is split into separately authorized,
quota-consuming Development and Validation actions. The historical
`development_calibration` lane evaluated fixed scalar and query-intent policies
on exactly 300 `development` cases and retained only
`relevance-calibration.json` plus `run-manifest.json`.

The first schema-v1 run completed all `20,301` scalar pairs with
`providerCostRatio=0.033084` passing but found `feasiblePairCount=0`. The
schema-v2 aggregate rerun ruled out admission similarity, maximum rerank score,
and top-two candidate margin without retaining per-case scores. The schema-v3
query-only bilingual intent run completed all `201` thresholds with
`providerCostRatio=0.056284` passing but found no feasible threshold. Its first
zero-egress threshold retained only `31/165` relevant current-fact cases; its
recall-first threshold produced `26` false injections and unauthorized egress
events. These are valid failed Development results. They selected no policy,
did not use Validation/Holdout, and must not be weakened or retuned.

The owner subsequently authorized ordinary current-user Memory candidates in
this single-user Server-mode deployment to reach the configured cloud Provider.
This authorized the schema-v4 candidate-aware Development experiment, not
blanket egress or answer injection. Its first live run fixed
`Qwen/Qwen3-8B` and completed all 300 cases, but failed the unchanged gates:

```text
Final Recall@5                 = 0.7589743589743589
current-fact accuracy          = 0.7515151515151515
false injection               = 14/300
p95/p99 latency milliseconds  = 1853/1855
hard-cutoff judge failures     = 31/195
authority/privacy leak counts = 0
```

No policy was frozen, Validation remained blocked, and the source Key was
destroyed. Do not increase the cutoff or lower relevance, safety, latency,
token, split, privacy, or promotion gates.

The owner explicitly removed relative Provider expense as a selection
constraint for this single-owner deployment. The next precommitted hypothesis
was `deepseek-ai/DeepSeek-V4-Flash` under schema v5 and
`owner_authorized_absolute_cap_v1`. That run also failed: `164/195` judge
requests hit `HARD_CUTOFF`, Final Recall@5/current-fact was
`0.143590/0.145455`, and p95/p99 was `1856/1865 ms`. False injection and every
authority/privacy leak count were zero. No policy was selected, and its Key
and runtime were destroyed.

The next named Development hypothesis was `Qwen/Qwen3.6-35B-A3B`. It also
failed: Final Recall@5/current-fact was `0.733333/0.733333`, false injection
was `15/300`, p95/p99 was `1854/1856 ms`, and `40/195` judge requests hit
`HARD_CUTOFF`. Its Key/runtime were destroyed and no policy was selected.

The planned `Qwen/Qwen3.5-4B` run was cancelled before Provider construction,
credential creation, or quota use. Tracking records
`cancelled_not_run_architecture_pivot`; there is no fabricated quality result.
The owner stopped hidden-judge model hopping and selected the current configured
GPT or DeepSeek model as a `search_memory` Tool router.

Run each exact GPT or DeepSeek hypothesis separately. The runner requires one
mode-`0600` SiliconFlow file for fixed BGE work and a distinct mode-`0600` file
for the route Provider. For these Development runs, the owner explicitly
authorized the operator to materialize short-lived decrypted copies from the
existing Server Vault. The runner never opens the Vault; the copies are
overwritten and removed after the run. A future Validation run does not inherit
this exception and requires fresh independent credentials.

```bash
chmod 600 /secure/input/transient-siliconflow-bge.key
chmod 600 /secure/input/transient-gpt-route.key
bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode development_memory_tool_route \
  --credential-file /secure/input/transient-siliconflow-bge.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-tool-route-credential-file /secure/input/transient-gpt-route.key \
  --memory-tool-route-provider-id configured-gpt \
  --memory-tool-route-provider-type openai \
  --memory-tool-route-base-url https://api.openai.com/v1 \
  --memory-tool-route-model exact-configured-model \
  --memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  --cost-basis /secure/eval/gpt-memory-first-tool-round-cost-v5.json \
  --output-dir /secure/eval/native-memory-runs
```

For DeepSeek, use its independent credential file, exact configured Provider
ID/Base URL/model, `--memory-tool-route-provider-type openai_compatible`, and a
separate cost-basis v5. A GPT result cannot authorize DeepSeek and vice versa.
The model is not trusted by name; exact live evidence determines whether the
unchanged quality and latency gates pass.

Before Provider construction, schema-v4/v5 `configurationSha256` binds the exact
Development split, reader/policy/model tuple, judge prompt version/SHA-256,
decoding profile, Top-20/Top-5 and 600/900 limits, hard cutoff, version-matched
cost basis,
and Provider-egress policy. The retained files are:

```text
cloud-judge-development.json
run-manifest.json
```

The judge prompt treats query/candidates as untrusted data. Its payload contains
only deterministic secret-redacted text and contiguous request-local ordinals,
never Memory IDs, revisions, scopes, database authority, or RRF/BGE scores. The
exact output contains only schema version plus at most five unique in-range
ordinals; an empty array means `no_memory`. BGE rerank and the judge run
concurrently under the existing hard cutoff. Both must succeed, then judge
ordinals are intersected with BGE order before the unchanged token selector.

The exact `owner_authorized_normal_candidates_v1` policy permits Provider
egress only when a candidate's exclusion reason is `irrelevant`. Cross-user,
out-of-scope, deleted, secret, superseded, Sensitive-disabled, and
untrusted-source egress remains a zero-tolerance failure. False injection is
scored exactly as before; owner egress authorization does not authorize prompt
injection.

The report retains only shared aggregate metrics, bounded failure-code counts,
policy/model/prompt identities, and aggregate token/cost upper bounds. It
contains no case ID, query, Memory plaintext, raw per-case score, raw judge
output, credential, or observation file. A failed latency, quality, safety,
token, or version-matched budget gate is valid evidence and must not be
bypassed.

Historical schema-v6 `development_memory_tool_route` bound reader
`neo-chat.native-memory-reader-capture.v4`, policy
`memory_hybrid_main_model_tool_route_calibration_v1`, exact route Provider ID/
type/normalized Base URL SHA-256/model, and this Tool tuple:

```text
name             = search_memory
contract version = memory-search-tool-v1
contract SHA-256 = f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
decoding profile = memory-search-tool-decoding-v1
temperature      = 0
maximum output   = 128
thinking         = disabled
```

Disabled thinking is a Provider wire contract, not a generic field:

```text
official DeepSeek api.deepseek.com -> {"thinking":{"type":"disabled"}}
other OpenAI-compatible gateways   -> {"enable_thinking":false}
official OpenAI                    -> omit both fields
```

The route request contains only the deterministic secret-redacted current
query plus that Tool. It contains no candidate body, Memory ID, revision,
scope, score, or database authority. No call is `no_memory`. Use Memory requires
one Provider choice and one exact call with a non-empty ID, name
`search_memory`, and explicitly decoded `{}` arguments. Missing, `null`,
malformed, non-empty, unknown, duplicate, late, failed, or drifted output fails
closed.

Tool routing starts concurrently with fixed BGE work. Candidate bodies may
reach only the separately authorized BGE reranker and remain request-local;
they never enter the route model. Route abstention/failure discards speculative
BGE final rows. One valid call releases the unchanged BGE order, Top-5, and
600/900-token selector. This capture does not yet execute the product same-model
answer continuation.

The retained schema-v6 files are:

```text
memory-tool-route-development.json
run-manifest.json
```

They contain aggregate route completion/use/abstention/failure counts, shared
metrics, exact profile authority, and aggregate token/cost ceilings. They
contain no case ID, query, Memory plaintext, raw Tool output, raw score,
credential, or observation file. The 300-case PostgreSQL 17 `fake_protocol`
replay completed with 300 route requests, zero protocol failures, an actual
input-token upper bound of `358533`, mode-`0600` artifacts, and zero remaining
containers/networks/volumes. Its deterministic route fails quality gates by
design and is not model-quality evidence.

Three live Development executions followed:

```text
SERVER_DEFAULT / gpt-5.6-sol
  completed/use/abstain/failed = 41/40/1/259
  failure codes                = HARD_CUTOFF 250, MEMORY_TOOL_ROUTE_FAILED 9
  Final Recall@5/current fact  = 0.087179/0.090909
  false injection             = 2/300
  p95/p99                      = 2002/2003 ms

FOHWSU / deepseek-v4-pro
  completed/failed             = 0/300
  status                       = protocol_mismatch_invalid_quality_evidence
  reason                       = official DeepSeek received enable_thinking=false

FOHWSU / deepseek-v4-flash after protocol correction
  completed/use/abstain/failed = 77/62/15/223
  failure codes                = HARD_CUTOFF 2, MEMORY_TOOL_ROUTE_FAILED 221
  Final Recall@5/current fact  = 0.256410/0.254545
  false injection             = 3/300
  p95/p99                      = 1377/1808 ms
```

All authority/privacy leak counts were zero. The Flash evaluation recorded zero
hard-cutoff violations even though two route failures carried the bounded
`HARD_CUTOFF` failure code. Schema v6 does not retain a stable subtype beneath
`MEMORY_TOOL_ROUTE_FAILED`; do not relabel those `221` failures as quota, rate
limit, overload, transport, or protocol errors without new evidence.

No schema-v6 profile passed and no policy was frozen. The decisive architecture finding
is that `ChatToolAdapter` performs a separate `PlanTools` Provider request
before the normal answer request; it does not reuse the existing first chat
Tool round. Do not raise the two-second cutoff or retry this preflight. The next
hypothesis must expose `search_memory` beside the other read-only tools on the
existing first `ToolRoundProvider` request and use same-model continuation.
Validation remains blocked. Schema-v6/profile-v6/cost-basis-v4 and
`memory-tool-route-development.json` remain immutable failed evidence; do not
rewrite them into first-round evidence.

Schema-v7 is the successor under the same capture-mode CLI string. It binds:

```text
reader version        = neo-chat.native-memory-reader-capture.v5
profile schema        = neo-chat.memory-regression-profile-config.v7
report schema         = neo-chat.memory-regression-relevance-calibration.v7
cost schema           = neo-chat.memory-regression-cost-basis.v5
policy                = memory_hybrid_main_model_first_tool_round_calibration_v1
admission mode        = development_main_model_first_tool_round_only
Tool adapter          = chat-first-tool-round-memory-decision-v1
artifact              = memory-first-tool-round-development.json
```

`internal/chat` is the single canonical Tool definition/hash/validation
authority. The Development `memoryroute.ChatToolAdapter` accepts a
`ToolRoundProvider` and emits one real first `ProviderRoundRequest` with the
current synthetic query/message, exact `search_memory` definition,
`tool_choice=auto`, and no continuation. It never calls `PlanTools`; it does not
force temperature, maximum-output, or thinking-control fields. Zero calls
abstains. One exact non-empty-ID `search_memory({})` call releases the unchanged
BGE Development final set. Invalid events, missing/null/non-empty arguments,
unknown/duplicate calls, Provider failure, cancellation, deadline, or
provenance drift fail closed.

The retained schema-v7 files are:

```text
memory-first-tool-round-development.json
run-manifest.json
```

They remain aggregate-only and mode-`0600`. The report binds the exact
Provider/model/Base-URL hash, canonical Tool tuple, adapter, BGE tuple,
unchanged evaluator gates, owner egress policy, and cost-basis v5 before
Provider construction. It omits the schema-v6 decoding/temperature/output/
thinking fields. Offline units, fake-Provider protocol tests, report/manifest
checks, lifecycle topology, and migration-065 PostgreSQL 17 replay pass.

The first live schema-v7 hypothesis bound
`SERVER_DEFAULT/gpt-5.6-sol`. It completed `28/300` first-round decisions, all
calling Memory, and failed closed on `272` (`266` `HARD_CUTOFF`, `6`
`MEMORY_TOOL_ROUTE_FAILED`). Candidate Recall@20 was `1.0`, Final Recall@5 was
`0.102564`, current-fact accuracy was `0.109091`, false injection was `2/300`,
and p95/p99 was `2002/2002 ms`. All authority/privacy leak counters were zero,
but unchanged quality, unrelated-negative slice, cutoff, and latency gates
failed. The two aggregate files remain mode `0600`; transient credentials and
all scoped runtime objects were destroyed.

The independent `FOHWSU/deepseek-v4-flash` schema-v7 hypothesis completed only
`33/300` decisions, all calling Memory, and failed closed on `267` (`4`
`HARD_CUTOFF`, `263` `MEMORY_TOOL_ROUTE_FAILED`). Final Recall@5/current-fact
accuracy was `0.128205/0.127273`, false injection was `2/300`, p95/p99 was
`1622/1860 ms`, and the evaluator recorded one hard-cutoff violation. Both
false injections were in the unrelated-negative slice; all authority/privacy
counters remained zero. The request/token/cost authority passed, both private
credentials were destroyed, the two aggregate files remain mode `0600`, and
the isolated runtime left no objects. No schema-v7 policy passed or froze;
Validation/Promotion remain blocked.

The v7 report did not retain a failure subtype, so the `263` DeepSeek
`MEMORY_TOOL_ROUTE_FAILED` cases cannot be retroactively attributed to quota,
rate limiting, HTTP rejection, transport, SSE, Tool Call, or provenance causes.
Those explanations remain `[unverified]` for the completed run.

The first failure-diagnosis contract used this non-selecting schema-v8 lane:

```text
capture mode          = development_memory_tool_route_diagnostic
profile schema        = neo-chat.memory-regression-profile-config.v8
reader                = neo-chat.native-memory-reader-capture.v6
report schema         = neo-chat.memory-regression-relevance-calibration.v8
admission mode        = development_main_model_first_tool_round_failure_diagnostic_only
taxonomy              = memory-tool-route-failure-taxonomy-v1
taxonomy SHA-256      = 66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0
artifact              = memory-first-tool-round-diagnostic-development.json
policySelected        = false
```

The report contains only aggregate fixed-enum counts and requires their sum to
equal `failedCaseCount`. It forbids raw Provider errors/bodies, queries, Tool
payloads, Memory content, scores, credentials, and case identity. It reuses the
unchanged cost-basis v5 request/token/price ceilings but binds the taxonomy in
the v8 configuration hash.

The first paid schema-v8 attempt, run
`memory-regression-20260730t043820z-dc26df80`, bound the configured
`FOHWSU/deepseek-v4-flash` route and the fixed SiliconFlow BGE Provider. It
completed Provider work but failed a post-capture integrity gate before report
or manifest publication. The pre-fix command exposed only the generic bounded
error `native Memory capture is invalid`, so the exact violated invariant is
not recoverable and the run is not diagnostic or quality evidence. It retained
zero artifacts. Both temporary mode-`0600` credentials were overwritten and
unlinked, and all scoped containers, networks, and volumes were removed.

Post-capture Memory Tool-route report and manifest gates now return only fixed,
content-free integrity reason codes. They still expose no query, case ID,
Memory content, Tool payload, Provider body/error text, or credential. This is
observability only: it does not relax an invariant, add a retry, change the
two-second cutoff, or authorize another paid run. Validation remains blocked.

The separately authorized second schema-v8 attempt,
`memory-regression-20260730t052917z-7b8c8bcf`, bound the same
`FOHWSU/deepseek-v4-flash` Provider/model, Base-URL hash
`12b8deaccc34b32757dbb1497e029da0c2e7b26ffa86b9c926c08cb4692f4508`,
and private cost-basis file SHA-256
`4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b`.
It consumed quota and failed with the bounded reason
`Memory Tool-route report admission_state`. This proves at least one
non-empty candidate case had `AdmissionReady=false`; BGE query-embedding
cutoff, invalid Provider response, and admission SQL failure remain
`[unverified]` subcauses. It published zero artifacts and is neither route-
diagnostic nor quality evidence. All transient credential/helper/export paths
and scoped containers, networks, and volumes were destroyed; the empty external
evidence directory was removed. Validation and Promotion were not run.

Schema v9 replaces v8 only for future execution; it does not rewrite either
empty v8 attempt:

```text
capture mode          = development_memory_tool_route_diagnostic
profile schema        = neo-chat.memory-regression-profile-config.v9
reader                = neo-chat.native-memory-reader-capture.v7
report schema         = neo-chat.memory-regression-relevance-calibration.v9
admission mode        = development_main_model_first_tool_round_route_failure_diagnostic_only
completeness policy   = route_complete_retrieval_fail_closed_v1
taxonomy              = memory-tool-route-failure-taxonomy-v1
taxonomy SHA-256      = 66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0
artifact              = memory-first-tool-round-route-diagnostic-development.json
policySelected        = false
```

Route failures still require exactly one bounded category and aggregate to
`failedCaseCount`. Admission/rerank incompleteness is retained only when Final,
Injected, and prompt-token surfaces are empty; it aggregates separately as
`retrievalIncompleteCaseCount` and `retrievalFailureCodeCounts`. The fixed
750 ms query-embedding cutoff, two-second hard cutoff, no-retry behavior,
Provider request/cost shape, route taxonomy, and v1 production authority do not
change. Schema-v7 bytes remain free of diagnostic fields, while schema v9 emits
explicit empty route/retrieval maps. The v9 lane cannot select a policy, unlock
Validation/Promotion, or run against paid Providers without new explicit quota
authorization.

The owner-authorized schema-v9 run
`memory-regression-20260730t094556z-0f4878dd` published the expected private
report and manifest, then returned non-zero because unchanged metric gates
failed. A preceding local operator preflight consumed no quota: it had compared
the private source file's raw SHA-256 with the decoded canonical cost hash.
The corrected gate binds those two different surfaces explicitly:

```text
private cost file SHA-256 = 4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b
manifest cost content SHA = b54b6fcfb62a33b31ef17cfd9876d392a20ef21bd25d19f67902350f194b1742
configuration SHA-256    = 13cc65b47ff7c358935ebd3bb1080412784e353ebc72503963b2822d9990c14f
```

Only `12/300` routes completed, all calling `search_memory`; `288` failed
closed. The reconciled route taxonomy contains `31` `CONTEXT_DEADLINE`, `83`
`TOOL_CALL_INVALID`, and `174` `ROUTER_FAILURE_UNCLASSIFIED` cases. Retrieval
completeness separately records `174` `RELEVANCE_ADMISSION_UNAVAILABLE`
cases. Equal aggregate counts do not prove per-case intersection because the
artifact intentionally retains no identity.

Candidate Recall@20 remained `1.0`, Final Recall@5/current-fact accuracy was
`0.010256/0.012121`, false injection was zero, p95/p99 latency was
`2001/2002 ms`, and `23` evaluation hard-cutoff violations were recorded.
Request/token/cost authority passed at `300/300` route requests and
`358533/2363529` input/output token upper bounds under `600000/2457600`
ceilings. Every authority/privacy leak counter remained zero. This is valid
diagnostic and failed-metric evidence only: no policy was selected, Validation
and Promotion were not run, and the product flag remains default-off.

The retained two-file evidence set is mode `0600` under a mode-`0700` external
directory. Temporary Vault copies, operator/helper files, runner temporary
state, and the exact Compose containers/network/volume were destroyed. Any
further paid diagnostic requires a new explicit authorization.

Offline tracing after the run found one deterministic producer of the
unclassified aggregate. The reader started Tool routing before query embedding
but did not await the route when non-empty candidates later failed closed at
admission. Capture could finish with a recorded route input but no result or
category, synthesize `ROUTER_FAILURE_UNCLASSIFIED`, and leave the delayed route
able to race the next sequential Recorder case. The repair does not change the
route request, 750 ms embedding cutoff, two-second hard cutoff, retry policy,
evaluation gates, or retained evidence:

- every started route exposes one replayable completion and is closed on all
  retrieval exits;
- the capture decorator selects a buffered delegated result against context
  termination, so a cancellation-ignoring router cannot hold the reader or
  publish Recorder state later; and
- route writes carry a per-case generation token, so an old result cannot
  attach to the next case even when an assistant identity is reused.

Focused offline/race regressions prove admission-unavailable closure,
cancellation-ignoring behavior, and old-generation rejection. Because the v9
artifact is identity-free, this finding records a concrete implementation
cause without retroactively relabeling all `174` cases or establishing a
case-level join. It grants no paid rerun, Validation, or Promotion authority.

That result also ends the candidate-blind Tool-route hypothesis. Candidate
recall is private retrieval, not prompt injection: a model that routes before
recall cannot discover an implicit allergy, preference, or project fact behind
an ordinary-looking query. The next schema-separated Development lane is:

```text
capture mode          = development_configured_candidate_judge
profile schema        = neo-chat.memory-regression-profile-config.v10
reader                = neo-chat.native-memory-reader-capture.v8
report schema         = neo-chat.memory-regression-relevance-calibration.v10
admission mode        = development_configured_candidate_judge_only
adapter               = chat-configured-candidate-judge-v1
cost schema           = neo-chat.memory-regression-cost-basis.v6
artifact              = configured-candidate-judge-development.json
```

It recalls and reauthorizes candidates first, sends only the secret-redacted
query plus contiguous request-local candidate ordinals/bodies to one exact
configured GPT or DeepSeek Provider/model, accepts the existing strict ordinal
JSON or an empty array, and intersects selected ordinals with fixed BGE order.
Rejected candidates never enter the answer prompt or Usage. GPT and DeepSeek
are separate hypotheses. Fake/offline protocol, report, cost, Compose, and
teardown behavior passed before the separately authorized live executions.

The schema-v10 GPT run
`memory-regression-20260731t012841z-bebeac67` bound
`SERVER_DEFAULT/gpt-5.6-sol` and published a valid private failed-gate bundle.
Candidate Recall@20 was `1.0`, but no candidate-bearing strict judge decision
completed: `146` attempted requests hit `HARD_CUTOFF`, while `49` cases failed
closed before judge egress as `RELEVANCE_ADMISSION_UNAVAILABLE`. Final
Recall@5/current-fact accuracy was `0/0`, false injection and every authority/
privacy leak counter were zero, and p95/p99 was `1856/1862 ms`. The report
SHA-256 is
`931228006b5f48b500cfdb56ac4a72ef8e8fa08f25d9d2c6c841ace8e34e2c7f`.

The independent DeepSeek run
`memory-regression-20260731t013610z-b91342e0` bound
`FOHWSU/deepseek-v4-flash` and also published a valid private failed-gate
bundle. Candidate Recall@20 was `1.0`; `157/195` candidate-bearing judge
decisions completed, including `60` valid abstentions. The `38` failures were
`36` `HARD_CUTOFF` plus `2` pre-judge
`RELEVANCE_ADMISSION_UNAVAILABLE` cases. Final Recall@5/current-fact accuracy
was `0.558974/0.581818`, false injection and every authority/privacy leak
counter were zero, and p95/p99 was `1854/1858 ms`. The report SHA-256 is
`c72874e9d0e11c34a88aa9a22b3c02924b8ec9fde9c0bcb0461d5c53fdc9d95a`.

The first GPT execution originally retained no bundle because schema v10
reused the historical cloud-judge reporter, which rejected every non-empty-
candidate case with `AdmissionReady=false`. Runtime behavior was correct:
retrieval had failed closed before judge egress and released no Memory. The
schema-v10 report path now accepts that state only when `RerankReady` and
`CloudJudgeReady` are false, the judge input-token bound is zero, and Provider-
sent IDs, Final, Injected, and prompt Memory tokens are empty/zero. It counts
one normalized failed case without incrementing `actualRequestCount`.
`BuildCloudJudgeDevelopmentReport` keeps the schema-v4/v5 strict behavior, so
historical report semantics do not drift.

Both exact configured profiles failed unchanged quality and latency gates and
selected no policy. Validation, production composition, and promotion remain
blocked. Each private run directory is mode `0700` with exactly two mode-
`0600` aggregate artifacts. The separate transient credentials were
overwritten and removed after each run, and all scoped Compose containers,
networks, volumes, helpers, and export files were destroyed.

The schema-v11 successor keeps candidate-first recall but fixes one global
cloud Memory Judge independently from the answer model:

```text
capture mode          = development_fixed_memory_judge
profile schema        = neo-chat.memory-regression-profile-config.v11
reader                = neo-chat.native-memory-reader-capture.v9
report schema         = neo-chat.memory-regression-relevance-calibration.v11
admission mode        = development_fixed_memory_judge_only
Provider ID           = SERVER_DEFAULT
Provider type         = openai_compatible
Base URL              = https://sub.mumubuku.top/v1
Base URL SHA-256      = 3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671
model alias           = gpt-5.6-luna
adapter               = chat-configured-candidate-judge-v1
criteria              = neo-chat.memory-benchmark-criteria.v2
cost schema           = neo-chat.memory-regression-cost-basis.v7
artifact              = fixed-memory-judge-development.json
```

Criteria v2 changes only the complete-flow latency budget: p95 `<=1500 ms`,
p99 `<=2500 ms`, and hard cutoff `<=3000 ms`. Quality, safety, token, and cost
gates remain the v1 values. The observed intermediary tuple does not prove an
upstream model implementation or public price. Timeout, transport error,
invalid JSON, protocol drift, and late output all produce an empty v2 Memory
set. Normal chat continues under v1 prompt/Usage authority and never falls
back to recalled, reranked, schema-v10, or other unjudged candidates.

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-cost-v7.json \
  --output-dir /secure/eval/native-memory-runs
```

Fake protocol proves only lifecycle and authority binding. A real 300-case
Development run requires separate fresh mode-`0600` SiliconFlow and Luna Key
files plus both exact quota approvals. Even a passing Development run stops
for owner review; it never starts Validation. Validation and production
activation each require a later independent authorization.

The retained schema-v11 Development bundle is immutable failed evidence. Run
`memory-regression-20260731t034030z-07481931` retained report SHA-256
`0dfe7733005bd211664ebaa47a9a5325c0638288f90c736986756eda34a37205`.
It attempted only `41` Luna requests and completed only `22` rerank-plus-judge
decisions; `154` cases recorded `RELEVANCE_ADMISSION_UNAVAILABLE` and `19`
complete stages recorded `HARD_CUTOFF`. Its latency criteria and bytes are not
rewritten by the successor.

Schema v12 is the accuracy-first Development successor:

```text
capture mode          = development_fixed_memory_judge_accuracy
profile schema        = neo-chat.memory-regression-profile-config.v12
reader                = neo-chat.native-memory-reader-capture.v10
report schema         = neo-chat.memory-regression-relevance-calibration.v12
admission mode        = development_fixed_memory_judge_accuracy_only
policy                = memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2
Provider ID           = SERVER_DEFAULT
Provider type         = openai_compatible
Base URL              = https://sub.mumubuku.top/v1
Base URL SHA-256      = 3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671
model alias           = gpt-5.6-luna
adapter               = chat-configured-candidate-judge-v1
criteria              = neo-chat.memory-benchmark-criteria.v3
cost schema           = neo-chat.memory-regression-cost-basis.v8
artifact              = fixed-memory-judge-accuracy-development.json
```

Its exact Provider sequence is BGE query embedding, local admission, BGE
rerank, Luna judge, then Record. One global gate holds Provider request
concurrency at `1`, including the one passage-projection call. The command,
per-case flow, BGE gateway, and Luna HTTP client have no application elapsed
deadline. HTTP redirects and environment proxies remain disabled, TLS 1.2 or
newer is required, and only caller cancellation may interrupt the run.

Criteria v3 retains the v1 quality, safety, token, cost, and slice thresholds
but makes latency diagnostic-only. The v12 report uses a separate evaluation
shape with aggregate p95/p99 values and no `latencyPassed`,
`hardCutoffPassed`, or hard-cutoff-violation verdict. Any
`HardCutoffApplied=true` or `HARD_CUTOFF` trace rejects the report as execution
drift.

Each request may retry once only for HTTP `408`, `429`, `5xx`, or a retryable
transport/read interruption. A valid `Retry-After` value controls the wait;
missing or invalid advice waits five seconds. Redirects, ordinary `4xx`,
invalid JSON/schema/protocol output, stream parse failures, and structured
remote-error payloads do not retry. After every case except the last, live mode
performs a real one-second cooldown. Fake protocol records 299 logical waits
and 299000 ms through a virtual/no-op clock and requires zero elapsed cooldown.

The report reconciles passage/query/rerank/judge attempts and retries,
per-stage aggregate request latency, logical/elapsed cooldown totals, total and
retry Judge input-token upper bounds, and `JudgeAttempts * 128` output-token
authority. Cost-basis v8 authorizes at most 600 Judge attempts and exactly
76800 output tokens. A passing report still emits `policySelected=false` and
must stop for manual review; Validation, production, and promotion remain
blocked.

The separately authorized schema-v12 live Development run
`memory-regression-20260731t080147z-8649c8ae` is retained failed evidence.
Report SHA-256 is
`126536772d71a5815f1cb6029deb568d0655c8780924ac0428951807975c8011`.
All `195` candidate-bearing cases completed rerank plus Luna judging with zero
failed cases; `203` Judge attempts included `8` bounded retries and all `299`
wall-clock cooldowns completed. Candidate Recall@20, Final Recall@5, and
current-fact accuracy were `1.0`, `0.974359`, and `0.969697`. The policy still
failed because `29` negative cases produced false injection `0.096667` against
the unchanged `0.02` maximum, and the `stable_fact` current-fact slice was
below criterion. Authority/privacy leak counts remained zero, prompt budgets
passed, and p95/p99 latency was diagnostic-only at `5366/12402 ms`.

The manifest binds canonical cost content SHA-256
`d75a6edf7fd5f050c3e30c4cae5960972a8e6065676f477321a5510ad7e5dd47`;
the private raw cost source SHA-256 is
`5d5c33e807185170fa52080349c8875f28c1313be2d64344f8dc3c31ec99e6c8`.
The report emitted `policySelected=false`. Both operator credential sources
were exact-secret scanned from retained files, overwritten, and removed, and
the isolated Compose project was destroyed. This consumed Development result
does not authorize a rerun, Validation, production, or promotion.

The separately authorized repaired-v3 run
`memory-regression-20260731t093606z-89719a18` is another immutable failed
schema-v12 Development bundle. It used configuration SHA-256
`72940f138ba53dda01e5eddad5e82bf05e2740fd671549e2310adea61a1bf49f`;
report SHA-256 is
`f35cfea03c98de4ecfff8ea9c774fbcef706f895da9db3a72d606e99efee2eb7`
and manifest SHA-256 is
`5be7db8903c5e26cd2dcadae12cde1a3c52f3421bb46862db481e8105e955176`.
All 300 cases completed with zero failed cases, 195 rerank attempts, 202 Luna
attempts including seven retries, and all 299 wall-clock cooldowns. Candidate
Recall@20/Final Recall@5/current-fact accuracy was
`1.0/0.984615/0.981818`. The corpus repair reduced false injection from the v2
run's `29/300` to `10/300`, but `0.033333` remained above the unchanged `0.02`
maximum; `stable_fact` current-fact accuracy also failed at `0.933333`.
Safety/authority counters remained zero, prompt budgets passed, and complete-
flow p95/p99 latency was diagnostic-only at `5382/10623 ms`. The manifest
binds the same canonical cost SHA-256
`d75a6edf7fd5f050c3e30c4cae5960972a8e6065676f477321a5510ad7e5dd47`.
Temporary credentials and every isolated runtime object were destroyed, and
the base PostgreSQL container remained stopped. This v3 result also selects no
policy and grants no rerun, Validation, production, or promotion authority.

The separately authorized v4 schema-v12 Development run
`memory-regression-20260801t075451z-050d5f7c` is immutable failed evidence. It
used configuration SHA-256
`c4505385b7103788c3006bf705865b2dda7c3dc5c803063d6a3bb5f09fa59d6c`,
completed all 300 cases, and reached `1.0` Candidate Recall@20, Final Recall@5,
and current-fact accuracy with zero safety/authority leaks. It recorded `201`
Judge attempts with `6` retries and all `299` cooldowns. One of 30
`unrelated_negative` cases still injected its weather-board Memory, so that
slice failed at `0.033333` against the unchanged `0.02` maximum even though
aggregate false injection was `1/300`. Report SHA-256 is
`04539bd899b22cea8cd3d17a4ee9e5b2b28adb6b10942e6be5b563eb230efc24`;
manifest SHA-256 is
`1904c41aff06839afdba642bf36101ccff3ef65526fe3577249b9c1f7be5d6af`.
Aggregate-only retention does not prove which case or wording caused the false
positive.

The separately authorized v5 schema-v12 Development run
`memory-regression-20260801t084301z-aabb31a2` is another immutable failed
bundle, not a v4 retry. Configuration SHA-256 is
`5f871f68fc0d4fed8f5822895ccc537254c843c6957362f7c8b6459ee7f6342f`;
report SHA-256 is
`dc4e1ca7036c5dcd5fde73d06c0404ae66539c3477493e3105590155df1923f5`
and manifest SHA-256 is
`43ba6e02e1b22322c56a088c5772ea769606a4acdc37d809f0fa239ca07b94e1`.
Candidate Recall@20 remained `1.0`, while Final Recall@5/current-fact accuracy
was `0.907692/0.909091`. Aggregate false injection remained `1/300`, and the
universally separated mug-location `unrelated_negative` still failed at
`1/30 = 0.033333`. This run also recorded `17`
`CANDIDATE_JUDGE_FAILED` cases, `217` Judge attempts with `22` retries, and all
`299` wall-clock cooldowns. The retained aggregate cannot identify the sole
negative response and does not establish whether the positive-quality decline
came from corpus semantics; the observed Judge failures make that attribution
unverified. Every safety/authority counter remained zero, prompt and absolute
cost authority passed, all transient credentials/runtime objects were
destroyed, and no policy was selected. It grants no automatic rerun,
Validation, production, or promotion authority.

These two disjoint hard-negative families each retaining exactly one false
injection breaks the corpus-only repair loop. Aggregate-only evidence supports
version-wide metrics but cannot identify or causally explain the selected
case. Do not create v6 from a guessed response, weaken the `0.02` criterion, or
attribute v5's positive-quality drop to corpus text while Judge failures are
non-zero. Any next corpus, Judge/prompt, bounded diagnostic, or local relation
gate must have a new explicit identity and separate owner authorization.

Schema v13 is the bounded failure diagnostic successor. It does not change or
rerun the v5 hypothesis; its implementation and live execution remain a
separate non-selecting diagnostic identity:

```text
capture mode          = development_fixed_memory_judge_failure_diagnostic
profile schema        = neo-chat.memory-regression-profile-config.v13
reader                = neo-chat.native-memory-reader-capture.v11
report schema         = neo-chat.memory-regression-relevance-calibration.v13
admission mode        = development_fixed_memory_judge_failure_diagnostic_only
policy                = memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2
criteria              = neo-chat.memory-benchmark-criteria.v3
cost schema           = neo-chat.memory-regression-cost-basis.v8
artifact              = fixed-memory-judge-failure-diagnostic-development.json
taxonomy version      = memory-candidate-judge-failure-taxonomy-v2
taxonomy SHA-256      = 229bb4fd6aaf0ec7fea2bf9c37c7f332f78876249b0dba5692ca8bf789646a6d
diagnosticComplete    = true
promotionEligible     = false
policySelected        = false
passed                = false
```

The execution remains authority-equivalent to schema v12: fixed BGE and Luna
identities, query/admission/rerank/judge/Record sequence, global Provider
concurrency `1`, no elapsed deadline, one bounded transient retry, one-second
inter-case cooldown, and unchanged cost-basis v8. Schema v13 cannot change the
prompt, decoder, threshold, corpus, or active reader and cannot authorize
Validation or production even if every quality metric would otherwise pass.

The taxonomy is the sorted union of 15 canonical typed Provider categories
from `internal/chat` and nine Judge-local categories. The strict decoder types
JSON syntax, exact-schema, and ordinal failures by stage; adapter/capture code
types invalid input, oversized output, unexpected events, provenance drift,
and Recorder conflict. Context cancellation/deadline uses the Provider
taxonomy. Any unknown cause collapses to
`CANDIDATE_JUDGE_FAILURE_UNCLASSIFIED`; matching or persisting error text is
forbidden.

The aggregate report retains two content-free maps:

- `providerAttempts.judgeAttemptFailureCategoryCounts`: every failed
  Provider/adapter attempt, including the first failure of a recovered retry;
- `diagnostics.judgeTerminalFailureCategoryCounts`: exactly one final category
  per `CANDIDATE_JUDGE_FAILED` case.

Provenance drift and Recorder conflict happen outside a failed Judge attempt,
so they contribute only to terminal counts. Publication fails closed unless:

```text
sum(judgeTerminalFailureCategoryCounts)
  = diagnostics.failureCodeCounts["CANDIDATE_JUDGE_FAILED"]

sum(judgeAttemptFailureCategoryCounts)
  = providerAttempts.judgeRetries
    + terminal failures whose category can originate in a Judge attempt

providerAttempts.judgeAttempts
  = logical Judge requests + providerAttempts.judgeRetries
```

Both maps may be empty after a failure-free fake replay, but unknown categories
and zero-valued entries are invalid. Reports retain no case ID, query, Memory
body/ID, Provider error/response, selected ordinal, raw score, or credential.
Schema-v12 JSON and configuration hashing omit all v13 fields.

Offline lifecycle command only:

```bash
bash scripts/run-memory-regression.sh \
  --regression-root /secure/memory-benchmark/v5-regression \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_failure_diagnostic \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-accuracy-cost-v8.json \
  --output-dir /secure/eval/native-memory-runs
```

This command uses deterministic fake Providers and grants no live quota. Any
live schema-v13 run requires fresh independent retrieval/Judge credentials and
explicit approvals not implied by this implementation.

The single authorized live diagnostic has already been consumed. Run
`memory-regression-20260804t005257z-8f43c5e7` completed all 300 Development
cases under global Provider/Compose concurrency `1` and configuration
`f1971a3fabc93149170b216d440998b73e1d5c40f277b1b41c574bcd72016579`.
It reconciled `105` empty-candidate, `194` Judge-completed, and one failed
case. There were `197` Judge attempts and two retries. Attempt failures were
one `PROVIDER_STREAM_READ_FAILED` plus two `PROVIDER_TRANSPORT_FAILED`; the
sole terminal category was `PROVIDER_TRANSPORT_FAILED`. Evaluation passed at
Candidate Recall@20 `1.0`, Final Recall@5 `0.9948717949`, current-fact accuracy
`0.9939393939`, false injection `0`, and zero safety violations. The report
correctly remained `passed=false`, `policySelected=false`, and
`promotionEligible=false`. Report/manifest SHA-256 values are respectively
`381df1eb72c29bf4a6a478731797250998cdc58482becaa44bf0b9abfef58527` and
`cff8b7408841939e530a53aacb98f1894c2c7cf797bf4124a52f6c64f86284a3`.
Cleanup retained only those two aggregate mode-`0600` files, removed both
temporary credentials and helpers, left zero scoped Docker objects, and kept
the base PostgreSQL container stopped.

Schema v14 now implements that transport-only repair offline under
`development_fixed_memory_judge_transport_stable`. It preserves global
concurrency one, the one-second cooldown, prompt v1, strict decoding, BGE,
corpus, criteria, typed failure maps, and fail-closed behavior. BGE remains at
one retry; only Judge permits two retries, with exact five- then ten-second
fallback waits and valid `Retry-After` precedence. Cost-basis v9 validates a
worst-case `900` Judge requests and `115200` output tokens. A terminal failed
case forces `passed=false`; a zero-terminal passing report still keeps
`policySelected=false` and `promotionEligible=false`.

Focused Go tests and the deterministic topology/lifecycle gate pass. At that
offline checkpoint no live Provider/Docker execution had been made and no
private v9 cost document or live authority existed.

The owner subsequently authorized exactly one live schema-v14 Development
run. `memory-regression-20260804t022413z-cc2afbf6` completed `105` empty-
candidate and `195` Judge-completed cases with zero failures and zero retries.
Candidate Recall@20, Final Recall@5, current-fact accuracy, MRR@5, and NDCG@5
were all `1.0`; false injection and every safety counter were zero. Actual
Judge authority was `195` requests, `257701` input-token upper bound, and
`24960` output-token upper bound under the v9 `900/1500000/115200` ceilings.
The report passed while remaining `policySelected=false` and
`promotionEligible=false`. Configuration SHA-256 was
`d9397bc5f0d33a8f3779263da3bbef78a41e0b174b32f4bf27aa328136613caf`;
report/manifest SHA-256 values were
`d05b991120b6878d3937f2dfdd13a899badd66e0a77f44f0f76fe8190c363ed8`
and `5c3923aa21fc65ec3f80c963e38e642a40d8d1471d9de7272bea529202704762`.
The authority is consumed. Do not mutate schema v12/v13, amplify retrieval
retries, switch SSE/HTTP2/keepalive, or rerun automatically. Validation and
production remain separately authorized stages.

The offline command shape is:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_transport_stable \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-transport-stable-cost-v9.json \
  --output-dir /secure/eval/native-memory-runs
```

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_accuracy \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-accuracy-cost-v8.json \
  --output-dir /secure/eval/native-memory-runs
```

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_configured_candidate_judge \
  --configured-candidate-judge-provider-id configured-gpt \
  --configured-candidate-judge-provider-type openai \
  --configured-candidate-judge-base-url https://api.openai.example/v1 \
  --configured-candidate-judge-model exact-configured-model \
  --cost-basis /secure/eval/configured-candidate-judge-cost-v6.json \
  --output-dir /secure/eval/native-memory-runs
```

Only after a Development candidate-aware policy passes every applicable gate may
its exact policy, Provider/model, adapter profile, and selection behavior be
frozen in code. Neither schema-v10 profile nor schema-v11 passed, and all v2,
repaired-v3, v4, and v5 schema-v12 runs failed. Schema v14 later passed its
separately authorized Development run and its exact selection semantics were
installed behind the product Memory Tool Loop. That does not alter the
historical single-Provider `frozen_validation` lane, whose command shape and
artifacts remain:

```bash
chmod 600 /secure/input/fresh-validation-siliconflow.key
bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode frozen_validation \
  --credential-file /secure/input/fresh-validation-siliconflow.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --cost-basis /secure/eval/cloud-judge-validation-cost-v2.json \
  --output-dir /secure/eval/native-memory-runs
```

The production-policy closure is a different schema-v15 lane. It runs only the
frozen 100-case Validation split through the exact production BGE-M3 retrieval/
rerank, fixed Luna judge, retry, reauthorization, redaction, intersection, and
Top-5 release semantics. It binds profile config v15, reader capture v13,
report/run-manifest v15, cost-basis v10, production policy
`memory_hybrid_fixed_cloud_candidate_judge_production_v1`, and frozen read-
intent policy `memory-explicit-read-intent-v1` with SHA-256
`538d9ccff34fb976cedfca0d9e153078cb3ce36f1baff0691f1d2124d182119c`.
It never retunes. The visible machine `holdout` is rejected by the split
selector and is not a CLI mode.

Run the zero-network lifecycle proof first:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode production_fixed_memory_judge_validation \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/production-memory-judge-validation-cost-v10.json \
  --output-dir /secure/eval/native-memory-runs
```

Fake protocol must exit non-zero after retaining exactly
`fixed-memory-judge-production-validation.json` and `run-manifest.json`. Its
report is always `fake_protocol_lifecycle_only`, `passed=false`, Yellow, and
`retain_beta`; it is not quality or Release evidence. The completed offline
PostgreSQL 17/Compose replay processed exactly 100 Validation cases, recorded
100 query-embedding attempts, 65 Judge attempts, 99 virtual cooldowns, and no
failed case, then removed every scoped runtime object. No Provider credential,
network request, or quota was used.

A live schema-v15 run requires two newly materialized, mutually independent
mode-`0600` files: one fixed SiliconFlow BGE credential and one fixed Luna
credential. Operational `fresh` means those new one-run files plus a new
explicit export authorization and the schema-v15 quota authorization; it does
not require or claim upstream Key reissuance. The preferred path reuses only
the already active, connection-attested Server Vault records and wipes the
operator copies on every exit:

```bash
bash scripts/run-memory-production-validation-from-vault.sh \
  --cost-basis /secure/eval/production-memory-judge-validation-cost-v10.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --production-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA
```

The wrapper requires the active mode-`0600` single-server environment and
cost-basis files and a private mode-`0700` output parent. It runs the existing
Compose `admin` service as the configured non-root UID/GID. The bounded
`memory-validation-credentials-export` subcommand has no arbitrary Provider,
record, context, model, or cardinality input: it resolves only active attested
`RAG:SILICONFLOW` and `SERVER_DEFAULT`, then verifies OpenAI Compatible,
normalized Base URL SHA-256
`3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671`,
and model `gpt-5.6-luna`. Existing paths, symlinks, insecure parents, equal
paths/bytes, missing or copied Vault contexts, disabled/unattested records,
and any tuple drift fail before Validation. Partial files are overwritten and
removed. The wrapper repeats pair preflight, invokes the unchanged schema-v15
runner, and overwrites/removes the exported pair on success, metric failure,
ordinary failure, `INT`, `TERM`, and `HUP`.

The report and manifest intentionally contain no credential value, Key hash,
issuance date, Vault envelope, or rotation proof. They attest the fixed
Provider tuple and aggregate execution only. A terminal Provider failure
records a fail-closed case and continues the remaining ordered cases, but
makes the final Validation fail. Retained evidence is aggregate-only: no query,
Memory plaintext, Provider response/error, raw score, or case-level identity.
Any privacy/authorization release is Red and disables the Tool Loop; false
injection above `0.02` is Orange and disables recall while preserving data;
stability or remaining quality failure is Yellow and retains Beta. A passing
result still has `releaseEligible=false`, selects no policy, changes no flag,
and stops for owner review.

For each live phase, the wrapper copies that phase's Key into a temporary
mode-`0600` file and mounts it read-only. Tool-route Development does this for
both independent credentials and rejects the same file, hard links, or equal
Key bytes. Values never enter argv, environment variables, Compose config,
Docker inspect, reports, or Git. Both in-process byte buffers are cleared, and
retained artifacts, runner logs, and Docker metadata are scanned for both
secrets. When the owner authorizes Server Vault reuse, the dedicated schema-v15
wrapper is the only supported automated export path. The runner itself never
inspects or decrypts the Vault. Live output alone uses profile
`native_v2_hybrid`.

Configured candidate-judge Development follows the same rule with one fresh
SiliconFlow retrieval credential and one different fresh configured-chat
credential. The wrapper rejects the same file, hard links, or equal bytes,
binds the exact Provider ID/type/Base-URL hash/model before Provider
construction, scans both values from every retained surface, and destroys both
temporary copies on every exit.

Fixed Memory Judge Development uses that same two-file isolation but rejects
every Provider/Base-URL/model deviation from the schema-v11 tuple. The runner
has no Vault decryption authority. Operator-created one-run copies are mounted
read-only, rejected when hard-linked or byte-equal, overwritten and removed on
success/failure/signal, and scanned out of artifacts, logs, and Docker
metadata.

Schema-v15 production Validation keeps the same physical isolation but changes
the authorization authority. It accepts only the dedicated frozen-Validation
approval, rejects the historical configured-judge Development approval, and
requires newly materialized independent BGE/Luna source files for that run.
The source values may be owner-authorized reuse of the exact active Vault
records; this does not attest upstream Key age. Missing approval, same-file/
hard-link/equal-byte credentials, tuple drift, or artifact leak fails before
retaining output. Wrapper success, failed metric, and signal paths all destroy
the temporary copies and the isolated Compose project.

The single owner-authorized live run
`memory-regression-20260806t013956z-31e67617` completed all 100 ordered cases
with zero failed case and retained a valid aggregate-only report/manifest pair.
Candidate Recall@20 was `1.0`, Final Recall@5 was `0.984615`, current-fact
accuracy was `0.981818`, and every cross-user, deleted-memory, Secret,
untrusted-source, and unauthorized-egress safety counter was zero. Nine
false-injection cases produced rate `0.09`, above the frozen `0.02` criterion,
so the result is immutable Orange failed evidence with required action
`disable_memory_recall_preserve_data`. Report and manifest SHA-256 values are
`6b2ec1a0cf26b2190302accac384f9fab4fce0898d1b1bad1eaacb5a2ce39c69`
and `3ee114b2991ad2d0de954ad4a5998947567c66672e010dc079f17c73c18ae650`.
Both one-run credential copies and every scoped runtime object were removed.
The operator path deliberately changed no Memory flag; do not rerun, promote,
run Holdout, or release from this consumed authorization.

The retained aggregate report cannot name the exact nine failed cases. It
proves only that all nine are contained in the 10-case `unrelated_negative`
Validation slice. The provider-free remediation audit therefore used only the
already-consumed Validation split and introduced no new evaluation or Provider
authority. The frozen bilingual guard
`memory-negative-policy-query-guard-v1` has SHA-256
`8fe79b55a0f136392081a81e471abae98d0db7b8e3bece74adcc590b9d2c8f39`.
It matched all `10/10` `unrelated_negative` cases, `16/45` expected-no-Memory
cases overall, and `0/55` relevant cases. The ordered `unrelated_negative` and
all-flagged case ID sets hash to
`1e8aa17ce6f8426ce9c91d3be7ffeef34be2bb8b14d0eaa9a8616b5426f0bc6f`
and `a3c322d299a24c3443b92e9e7136b53bed8fd17e1d0a9bd71815937e41ba76c2`.

The guard is wired only into Development policy
`memory_hybrid_fixed_cloud_candidate_judge_negative_guard_development_v1`.
It runs after authorized repository Prepare and before candidate admission,
BGE rerank, or Luna Judge. A match records a completed empty final set with
`NEGATIVE_POLICY_QUERY_ABSTAINED`; the earlier query-only BGE embedding may
still occur, but candidate plaintext Provider egress is zero. Production-v1,
its descriptor hash
`c65c2b0bee2561ebbc8d97a65c4cc0c64db243b8a09334a8f1836250d799095c`,
Server composition, and runtime flags remain unchanged. This offline audit is
not a live Development pass and authorizes no Validation, Holdout, promotion,
Release, or recall re-enable.

Schema v16 is the separately versioned online-calibration lane for that guard.
It selects only the frozen 300-case Development split and binds reader capture
v14, profile/report v16, cost-basis v11, the generic relevance-run v1 manifest,
the exact fixed BGE/Luna tuple, guard version/SHA-256, and relevance-policy
descriptor SHA-256. Cost-basis v11 preserves the v9 maximums of 900 Judge
requests, 1,500,000 input tokens, and 115,200 output tokens. A guarded trace may
retain authorized local candidates from Prepare, but admission, candidate
rerank, Judge attempts/input tokens, Provider-sent/final/injected IDs, and
prompt Memory tokens must all be empty. Only exact
`NEGATIVE_POLICY_QUERY_ABSTAINED` with completed result `NO_CANDIDATES` is a
normal guard result, and the fixed 300-case report requires at least one such
guard abstention. Every other pre-admission code remains a failed case.

Run the no-network PostgreSQL 17 lifecycle before any live request:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_negative_guard \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-negative-guard-cost-v11.json \
  --output-dir /secure/eval/native-memory-runs
```

The accepted Fake run completed `105` empty-candidate, `30` guard-abstained,
and `165` Judge-completed cases. It used no credential or network, retained
exactly one aggregate report plus its mode-`0600` manifest, and removed the
scoped containers, networks, and volume.

Live execution uses a distinct Development export approval and the ordinary
Development Judge quota approval, never the consumed schema-v15 Validation
approval:

```bash
bash scripts/run-memory-negative-guard-development-from-vault.sh \
  --cost-basis /secure/eval/fixed-memory-judge-negative-guard-cost-v11.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --development-judge-approval \
    I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA
```

The consumed live run `memory-regression-20260806t064355z-65407a6a` completed
all 300 cases as `105` empty-candidate, `30` guard-abstained, `162` Judge-
completed, and three failed. The guard eliminated false injection (`0/135`),
the `unrelated_negative` slice passed `30/30`, Candidate Recall@20 stayed
`1.0`, and all privacy/authority safety counters were zero. Five Judge
decisions abstained; 20 retries and 23 typed failed attempts ended with three
terminal `PROVIDER_TRANSPORT_FAILED` cases. Final Recall@5 was `0.958974`,
current-fact accuracy/MRR/NDCG were `0.951515`, and the
`preference_instruction` and `stable_fact` current-fact slice gates failed.
The immutable top level is therefore `passed=false`, `policySelected=false`,
and `promotionEligible=false`.

The v11 canonical cost SHA-256 is
`2af792a5e999ea1018ab628924247dfbdf45b0f69c5b72f1ed97dbda52394c88`;
actual Judge authority was `185/900` requests, `246555/1500000` input tokens,
and `23680/115200` output tokens. Configuration/report/manifest SHA-256 values
are `d50315f2b79b35530199550dbea1cabd75de264b285b886d164d561bfdaa058d`,
`895a8f524177645a159b6e1e15bfe8c4d828813ff4472dad4cdbe442d1b73929`,
and `495ec7b4a19021f600db0f2826dc2875cabfbd3f8bd51fe0ce5e94d10ce65a43`.
The wrapper destroyed both one-run credentials and the consumed cost source;
all scoped Docker objects were absent, all 43 sampled live Memory relation
counts were byte-identical before/after, both Memory flags remained false, and
the live services remained healthy. This result authorizes no automatic rerun,
Validation, Holdout, promotion, Release, product policy change, deployment, or
recall re-enable.

Schema v17 is a transport-only successor to that immutable failed schema-v16
evidence. It preserves the exact fixed BGE/Luna tuple, prompt/decoder/model
controls, guard/policy descriptor, serial order, two Judge retries, five/ten-
second fallback waits, criteria v3, 300-case Development split, and aggregate-
only privacy contract. Only the response transport changes: adapter
`chat-configured-candidate-judge-buffered-v1` calls the Provider-owned bounded
JSON completion with `stream:false` and `Accept: application/json`. Profile and
report are v17, reader capture is v15, cost-basis is v12, admission is
`development_fixed_memory_judge_negative_guard_buffered_only`, and the artifact
is `fixed-memory-judge-negative-guard-buffered-development.json`. Historical
streaming adapters and ordinary chat remain unchanged.

Run Fake before live:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_negative_guard_buffered \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-buffered-cost-v12.json \
  --output-dir /secure/eval/native-memory-runs
```

The accepted Fake PostgreSQL 17 run completed `105` empty-candidate, `30`
guard-abstained, and `165` Judge-completed cases. It retained exactly the mode-
`0600` aggregate report and manifest, used zero network/credentials, and left
zero scoped containers, networks, or volumes.

The one-run live path is:

```bash
bash scripts/run-memory-buffered-judge-development-from-vault.sh \
  --cost-basis /secure/eval/fixed-memory-judge-buffered-cost-v12.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --development-judge-approval \
    I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA
```

Credential export must run the already configured admin image with
`--no-build --pull never`. Building the admin service would mutate a local
`BACKEND_IMAGE` tag and could change a later service recreation; the evaluated
source belongs only in the isolated regression image.

The sole live run `memory-regression-20260806t082407z-1ce1eba8` completed all
300 cases as `105/30/165/0` empty/guard/Judge-completed/failed. Three Judge
decisions abstained. Nine typed `PROVIDER_TRANSPORT_FAILED` attempts each
recovered through one retry, leaving zero terminal categories and `174` total
Judge attempts. Candidate Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.984615/0.981818`; false injection was `0/135`; every slice and safety
counter passed. Actual cost authority was `174/900` requests,
`231925/1500000` input tokens, and `22272/115200` output tokens. The immutable
top level is `passed=true`, `policySelected=false`, and
`promotionEligible=false`.

Cost-basis decoded-content/configuration/report/manifest SHA-256 values are
`339d419caa56ba7414ec993b2d059f004279315de65e05479b603536cbeb17f4`,
`83d61297ac9e0dd07a457af947642a6fb88505e2b70b701bc9e0681dd29e7359`,
`d0a70c03eda7fbb1bee4107c057acc54870da56cb2041ebdb9fa4cac8955a6ce`,
and `182bbcc4cf553f9e7eb893abbd0122e9536dca970d3b232c5c7f832b703bdf2a`.
Both credential copies, the consumed cost source, comparison snapshots, and
every scoped runtime object were removed. The 43 sampled live Memory relation
counts remained byte-identical at aggregate hash
`d027b35dd8b667f21c84b2a38cd0b27fec94b684c0d4561c8677bb3b9885142b`,
both Memory flags remained false, and live services stayed healthy. This
consumes the only schema-v17 live authority and grants no rerun, Validation,
Holdout, promotion, Release, deployment, product-policy mutation, or recall
re-enable.

Schema v18 is the separate production-v2 Validation successor. It selects only
the frozen 100-case Validation split and binds profile config/report v18,
reader capture v16, Validation run manifest v18, cost-basis v13, policy
`memory_hybrid_fixed_cloud_candidate_judge_negative_guard_production_v2`,
adapter `chat-configured-candidate-judge-buffered-v1`, the unchanged guard,
criteria, case order, fixed BGE/Luna tuple, two-retry schedule, privacy rules,
and aggregate-only outcome contract. It does not reinterpret schema v15, v16,
or v17. Fake evidence is always non-passing; live failure retains the report
and manifest and denies any rollout.

The PostgreSQL 17 Fake run completed `35/10/55/0`
empty/guard/Judge-completed/failed cases, used zero network or credentials,
retained exactly two mode-`0600` aggregate files, and left zero scoped Docker
objects. The sole complete live run
`memory-regression-20260806t101512z-a057b161` repeated `35/10/55/0`; one Judge
decision validly abstained. Its `58` Judge attempts included three recovered
retries (`2` transport and `1` upstream) and no terminal failure. Candidate
Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.984615/0.981818`, false injection was `0/45`, all safety counters were
zero, and prompt/token/cost ceilings passed. However,
`mixed_language_entity` and `stable_fact` each had `0.9` current-fact accuracy,
below their required slice criterion. The immutable outcome is Yellow
`retain_beta`, `passed=false`, `policySelected=false`,
`promotionEligible=false`, and `releaseEligible=false`; no UUID canary was
selected and both Memory rollout gates remained off.

Actual Judge authority reconciled at `58/300` requests,
`77632/300000` input tokens, and `7424/38400` output tokens.
Configuration/cost/report/manifest SHA-256 values are
`14706c71ccf347ccdec63b879eb3e713bebf22274be17a326f17f0983001a272`,
`2cf7661fa91c592abc239cd946e852b31208b25ff5387cd6693fb3923d345f65`,
`dbc6219a5eb446e56460002e99ee763d13ff5f7b4ce417825da918f5e2d2c851`,
and `b316549f730831988d1fc37d1d07965d7ec8417bf384e4a7dc72cfca21f94f36`.
Both credential copies and the protected export env were destroyed; all live
services and container IDs remained stable. This evidence is consumed and
must not be rerun or treated as partial rollout authority.

Schema v19 is a diagnostic-only Development successor. It selects the fixed
85-case union of `mixed_language_entity` and `stable_fact`, repeats that order
three times in one 255-execution authority, and retains only opaque Candidate,
BGE, Luna, final-intersection, and Golden-current membership. It binds fresh
profile/report/reader/run/cost identities and can never select a policy or
grant release authority. The sole live run
`memory-regression-20260807t034006z-837ea5cd` completed all 255 executions and
classified the defect as systematic: Candidate and BGE lost zero expected
facts, while Luna omitted the expected fact ten times. Three stable-fact cases
failed in all repetitions and one mixed-language case failed once. Nine typed
transport failures recovered; no terminal failure remained. This consumed the
schema-v19 live authority and selected only the Luna prompt boundary for one
minimal Development repair.

Schema v20 changes exactly that boundary. Prompt
`memory-cloud-candidate-judge-prompt-v2`
(`90fac3f3c97a340e6ef1963dc1456c5f088ac083658a29e75aad747efba95d90`)
adds explicit saved fact/preference/decision/correction/fallback/project
context rules while retaining the same fixed model, input/output schema,
decoder, BGE order, negative guard, buffered transport, retry schedule, and
failure taxonomy. Profile/report/reader/run/cost identities advance to
v20/v20/v18/v20/v15. The PostgreSQL 17 Fake run passed all 300 cases with zero
network and zero scoped runtime residue.

The sole schema-v20 live run
`memory-regression-20260807t042823z-67950546` completed all 300 cases but
failed unchanged quality gates. Candidate Recall@20 was `1.0`, Final Recall@5
was `0.969231`, current-fact accuracy was `0.963636`, and false injection was
zero. `stable_fact` and `temporal_correction` each reached only `0.933333`
current-fact accuracy; Luna abstained six times. Eleven typed transport
failures recovered, leaving zero terminal cases. Actual Judge authority was
`176/900` requests, `316702/2000000` input tokens, and `22528/115200` output
tokens. Safety, privacy, token, and cost gates passed, but the immutable top
level is `passed=false`, `policySelected=false`, and
`promotionEligible=false`. Therefore schema v21 Validation was not constructed,
no canary UUID was selected, both Memory runtime flags stayed false, and all
Memory data was preserved.

The schema-v19 configuration/cost/report/manifest hashes are
`44c8859909bd51b2f0d5dcf163f28d21db94829d8d24618b0389b2dc94b2e85f`,
`b61949473b7a84a4548a32ab875544a6908d28f0b44defa3c455979a5e53f63c`,
`8f8a3a992509b4742b7e9b0296be209efc206584e67d8adf27f1f0203ce25c71`,
and `8725458d19c5e8c00acb6a6e33e1b4547715695891c8d46224c1393f5fb1f2a1`.
The schema-v20 live values are
`2f05171d7f7254a2e453fa1ab4a55a8ff00ff5ec1cc464db145bec101e0c4266`,
`d13cd173e8c8d386154e6977bc1a0b960fb3b3e3255fafb0a24a411e8c3a1850`,
`a7f270f2b822df3bdc218db6ca0baaff2a5f652a91416e50a4fa5e93b87a8463`,
and `6e91dedeefab1f2a54d638fec96421242d6287f0ea61dfa9b695ac4df00d4c7b`.

The schema-v20 abstention diagnostic is a separately named, non-promotional
Development lane; it does not consume the reserved schema-v21 Validation
identity. It selects the exact 57-case union of `stable_fact` and
`temporal_correction` (`30 + 30 - 3`), repeats the frozen order three times,
and preserves prompt v2, the fixed BGE/Luna tuple, buffered decoder, negative
guard, two-retry schedule, cooldown, and final intersection. Its report keeps
only opaque case identity, slice membership, stage counts/rank, and a closed
root-cause enum. Query text, Memory plaintext, raw scores, raw errors, and
Provider bodies are forbidden.

```bash
bash scripts/run-memory-v20-abstention-diagnostic-from-vault.sh \
  --cost-basis /secure/eval/v20-abstention-diagnostic-cost.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-v20-abstention-diagnostic-approval \
    I_UNDERSTAND_THIS_USES_REAL_MEMORY_SLICE_DIAGNOSTIC_QUOTA
```

The Fake lifecycle completed all 171 executions with zero network, credentials,
or scoped Docker residue. The sole live run
`memory-regression-20260807t071130z-90224759` also completed 171/171 and
classified the six Luna omissions as stochastic: four opaque cases were
affected, two once and two twice across the three repetitions, while no case
failed all three times. Candidate, BGE rerank, final rank/budget, and terminal
root causes were zero. Eleven typed transport failures recovered, so 171
logical Judge decisions reconciled to 182 attempts and zero terminal cases.
Authority reconciled at `182/513` requests, `320429/1000000` input-token upper
bound, and `23296/65664` output-token upper bound. The cost document uses an
owner-authorized conservative absolute ceiling, not a claim of observed spend.

Report/manifest SHA-256 values are
`7c17325844e0eb5d4874386437f471fd54f6f061318c538f4e1d30026823936a`
and `49b24b9f0cb914abcf6af7a15c659437c165ebc18c28c256ae4fca97f0f47e97`.
Both files are mode `0600`; credentials and all scoped runtime objects were
destroyed. Live migration/data counts, service health, disabled Memory flags,
and the empty canary remained unchanged. The result selects no policy and
grants no rerun, release, promotion, canary, or schema-v21 authority.

The abstention-confirmation successor then retained prompt v2 as the primary
decision and used separately hashed prompt v3 only after a valid empty result.
Its first authorized live wrapper invocation failed before Provider work due
to an omitted host Luna credential-copy predicate. After that topology defect
was fixed and covered by live-shaped shell tests, the owner granted one fresh
Development authority rather than reusing the consumed attempt.

The replacement run `memory-regression-20260807t100046z-3c7d216f`, capture
`85b2d1a1-8bca-4888-9e89-f1dbdec6acb4`, completed all 300 cases and passed.
Candidate Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.9897435897/0.9878787879`; false injection was `0/135`, every required
slice passed, and every privacy/safety counter was zero. Three logical
confirmations produced four attempts, including one retry. Ten primary retries
and that confirmation retry recovered with zero terminal cases. Actual Judge
authority reconciled at
`179/1800` requests, `321531/2000000` input-token upper bound, and
`22912/230400` output-token upper bound. Average/maximum prompt Memory was
`67.85/381` tokens.

Configuration/raw-cost/decoded-cost/report/manifest SHA-256 values are
`15a957e610e96d2048a617b73f513d003bbb5f0c856b8c9bd789307a10094026`,
`fa814cda981e10f7c7a596db51f82aabf0fcaa6571a02e8283fc63aa99c7853f`,
`e488d0a657dba7fe05d444e4bf38587a4ec16b2f46de74dee07ad5e4c1285119`,
`bcfd12e7d735f5b8bec09edd7fd74d22b8c7fb0f99e50f2d9367627811a3cf91`,
and `59e322e5b4817d009013606a3972514dae34e9de3e68aa7995f9350a6951597a`.
The private bundle is retained under
`/var/tmp/neo-chat-abstention-confirmation-development-20260807T100027Z`.
Credentials and all scoped runtime objects were destroyed; product services
remained healthy and both Memory flags stayed false. This Green Development
evidence remains non-promotional and grants no schema-v21 live Validation or
production rollout authority.

The next one-shot schema-v21 Validation invocation stopped at its argument
gate before Provider construction. The Vault wrapper and its isolated fake
runner forwarded an older approval literal without `FROZEN`; the generic
runner and Go live gate correctly required
`I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_ABSTENTION_CONFIRMATION_VALIDATION_QUOTA`.
Credential export completed, but there were zero BGE/Luna requests, no
isolated capture runtime, and no report or manifest. Both credential copies
and the export container were destroyed. The wrapper and live-shaped test now
use the frozen literal, and the test asserts literal parity across wrapper,
generic runner, and Go gate. This started authority was not rerun and grants no
Validation, promotion, or launch evidence.

Under a fresh owner authority, the repaired live schema-v21 Validation run
`memory-regression-20260808t131254z-f11b8601`, capture
`cf29492b-54d5-4367-a7c7-151b1c465dcd`, completed all 100 cases. Overall
Candidate Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.9846153846/0.9818181818`, false injection was `0/45`, and every safety
counter was zero. One confirmation request ran. Five typed transport retries
recovered with no terminal case. Judge authority reconciled at `61/600`
requests, `110083/600000` input-token upper bound, and `7808/76800`
output-token upper bound; average/maximum prompt Memory was `67.64/365` tokens.

One Judge decision still abstained. The aggregate slice membership arithmetic
places the same omitted current fact in `mixed_language_entity` and
`stable_fact`; each slice reached only `0.9` current-fact accuracy, below the
unchanged `0.95` criterion. The immutable outcome is Yellow `retain_beta`,
`passed=false`, `releaseEligible=false`, `policySelected=false`, and
`promotionEligible=false`.

Configuration/Validation-order/production-policy/raw-cost/decoded-cost/report/
manifest SHA-256 values are
`505cd678c703c2a0124e1af72d8ab4d83633e48810ed5f43f65aec3c7be01a99`,
`cea5ebff03cef920deb4b5a9b36bee45e2f17ecb1d1b4987bc4f902fc1c8d430`,
`63e1191d81a7579bc89f781187598a8887ac870eb59185ddf9e19a5c73dede47`,
`cd60276abf34cdd2629cc68d416828154692990b93fbb5e06690733e4c4c442d`,
`0990f44dd5ca7da2f31f07251aadcb7a00c49a42d8463b72ae229ad9a98a344b`,
`3aff1211c6d1a2bbe422fb9053deda18726a71d5374441f85cd8179d5cc60a13`,
and `0378b0a685267b2bb371cf35c5b965ee871a9001fc23337143b230cd176bd986`.
The private bundle is retained under
`/var/tmp/neo-chat-abstention-confirmation-validation-20260808T131253Z`.
Credentials and all scoped runtime objects were destroyed; product flags
remained off. The consumed result grants no rerun, promotion, image pinning,
canary admission, or production launch.

```bash
bash scripts/run-memory-production-buffered-validation-from-vault.sh \
  --cost-basis /secure/eval/production-buffered-validation-cost-v13.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --production-buffered-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA
```

The export helper pins an explicitly reviewed admin image, always uses
`--pull never`, never supplies positive `--build`, and supplies `--no-build`
only when the installed Compose `run` command supports that flag. Capability
inspection occurs before credential export or Provider work. The exact UUID
canary remains a separate success-only runtime action, not an evaluator side
effect.

Accuracy-first Development keeps the same exact two-file and fixed-Luna
authority. Its v12 execution policy changes no credential boundary: operator
copies remain mode `0600`, read-only in the runner, independent by file/inode/
bytes, and destroyed on every exit. No existing schema-v11 authorization or
credential copy authorizes a new schema-v12 live run. Each v2/v3/v4/v5 result
consumed only its exact authorization; none authorizes another corpus version
or a retry of any failed result.

The cost basis is strict JSON and all values are run-total integer microunits
in one named unit:

```json
{
  "schemaVersion": "neo-chat.memory-regression-cost-basis.v1",
  "baseline": {
    "unit": "cny_microunits",
    "memoryProviderCostMicrounits": 0,
    "chatProviderCostMicrounits": 1000000
  },
  "candidate": {
    "unit": "cny_microunits",
    "memoryProviderCostMicrounits": 100000,
    "chatProviderCostMicrounits": 1000000
  },
  "source": "versioned operator rate card reference",
  "effectiveAt": "2026-07-29T00:00:00Z"
}
```

The example numbers are structural placeholders, not an approved rate card.
For an actual run, the baseline Memory cost must be exactly zero, the candidate
Memory cost must be positive and calculated from the versioned SiliconFlow
rate basis, and both profiles must use the same non-zero chat cost denominator.
Unknown fields, duplicate keys, zero candidate cost, unit drift, denominator
drift, or an invalid timestamp fail before Provider work.

Historical schema-v4 `development_cloud_judge` requires
`neo-chat.memory-regression-cost-basis.v2`. It extends the same object with one
exact `cloudJudgeAuthority` object. Schema-v5 paid-model Development instead
requires `neo-chat.memory-regression-cost-basis.v3` plus:

```json
"providerCostPolicy": "owner_authorized_absolute_cap_v1"
```

Both versions use the same authority shape:

```text
modelId                          = exact --cloud-judge-model value
requestCount                     = 300
maximumInputTokens               = preauthorized conservative aggregate bound
maximumOutputTokens              = 300 * 128 = 38400
inputMicrounitsPerMillionTokens  = exact versioned rate-card price
outputMicrounitsPerMillionTokens = exact versioned rate-card price
maximumCostMicrounits            = ceil(input bound * input price / 1e6)
                                  + ceil(output bound * output price / 1e6)
```

The recorder conservatively counts one token per UTF-8 byte plus fixed chat
framing for every actual judge request. Aggregate actual input/output upper
bounds must fit the preauthorized values. The candidate's
`memoryProviderCostMicrounits` must cover `maximumCostMicrounits`; a model,
request-count, token, price, arithmetic, or coverage mismatch is rejected
before Provider construction or before report publication as applicable.
An exact zero input/output price and zero maximum judge cost are valid only when
the versioned official rate card makes that fixed judge model free. The total
candidate Memory cost must still be positive because BGE embedding/rerank work
is priced. Never invent a non-zero judge rate to satisfy the schema.

Schema-v4 preserves the historical relative Provider-cost gate. Schema-v5
reports the truthful historical ratio but deliberately omits
`evaluation.providerCostPassed`; it instead requires
`providerCostAuthorized=true`, the exact owner policy ID, cost unit, maximum
judge cost, and maximum total Memory Provider cost in both report and manifest
authority. Exceeding any absolute request/token/cost ceiling invalidates the
run. This changes only the explicitly owner-controlled economics criterion;
every relevance, safety, latency, cutoff, token, split, privacy, and promotion
gate remains identical.

Historical schema-v6 Tool-route Development requires
`neo-chat.memory-regression-cost-basis.v4`, the same explicit
`owner_authorized_absolute_cap_v1` policy, no `cloudJudgeAuthority`, and one
exact `memoryToolRouteAuthority`:

```text
providerId                       = exact configured Provider ID
providerType                     = openai | openai_compatible
baseUrlSha256                    = normalized exact Base URL SHA-256
modelId                          = exact route model
requestCount                     = 300
maximumInputTokens               = conservative aggregate bound
maximumOutputTokens              = 300 * 128 = 38400
inputMicrounitsPerMillionTokens  = exact versioned route price
outputMicrounitsPerMillionTokens = exact versioned route price
maximumCostMicrounits            = exact ceiling arithmetic
```

The actual offline 300-case bound was `358533`, so a `300000` input ceiling is
known to be insufficient for this fixture/profile. The operator must calculate
and authorize a truthful bound before a live Provider is constructed; the
verified offline replay used `600000`. The candidate's total Memory Provider
cost must cover the fixed BGE cost plus the absolute route ceiling. Request,
token, rate, arithmetic, Provider, Base URL, or model drift is rejected rather
than inferred after quota use.

Schema-v7 first-ToolRound Development requires
`neo-chat.memory-regression-cost-basis.v5` with the same explicit owner policy
and Provider/model/Base-URL binding. It retains `requestCount=300`, but its
maximum output is a conservative aggregate first-round event bound rather than
the fabricated schema-v6 `300 * 128` preflight constant. The bound must be at
least one token per request and evenly divisible by 300; actual aggregate input
and output bounds must not exceed it. Exact rates and maximum cost use the same
ceiling arithmetic, and candidate Memory cost must cover fixed BGE work plus
the authorized first-round route ceiling.

Schema-v10 configured candidate-judge Development requires
`neo-chat.memory-regression-cost-basis.v6`, the same explicit owner absolute-
cap policy, no `cloudJudgeAuthority` or `memoryToolRouteAuthority`, and one
exact `configuredCandidateJudgeAuthority`:

```text
providerId                       = exact configured Provider ID
providerType                     = openai | openai_compatible
baseUrlSha256                    = normalized exact Base URL SHA-256
modelId                          = exact configured judge model
requestCount                     = 300
maximumInputTokens               = conservative aggregate bound
maximumOutputTokens              = 300 * 128 = 38400
inputMicrounitsPerMillionTokens  = exact versioned price
outputMicrounitsPerMillionTokens = exact versioned price
maximumCostMicrounits            = exact ceiling arithmetic
```

The candidate Memory cost must cover fixed BGE work plus this maximum. Any
Provider/model/Base-URL, request/token/rate/arithmetic, mixed-authority, or
coverage drift fails before Provider construction or bundle publication.

Schema-v11 fixed Memory Judge Development requires
`neo-chat.memory-regression-cost-basis.v7`. Its request count and maximum
output remain `300` and `38400`; the exact input/output ceilings, unit rates,
and absolute cost ceiling are hash-bound. The authority must equal the fixed
Luna tuple above. Schema v7 never reinterprets schema-v6 evidence and never
asserts an upstream model identity or unverified public rate.

Schema-v12 accuracy-first Development requires
`neo-chat.memory-regression-cost-basis.v8`. The exact fixed-Luna authority is
unchanged, but `requestCount=600` and
`maximumOutputTokens=600 * 128 = 76800` pre-authorize the single possible
retry for every logical Judge request. Actual input authority equals aggregate
total Judge-attempt input bounds, including retry input; actual output
authority equals `JudgeAttempts * 128`. Schema-v6/v7 remain strict 300-request
documents and cannot be widened or reused.

Schema-v13 Judge failure diagnostics reuse that exact v8 ceiling without
widening it. Failed attempts must additionally reconcile through the fixed
taxonomy; the report remains permanently non-passing and non-selecting
regardless of cost headroom or evaluation metrics.

Schema-v14 transport-stable Development uses cost-basis v9 with the same exact
fixed-Luna tuple and expands only the Judge authority to 900 requests and
115,200 output tokens for two possible retries. Schema-v15 production-policy
Validation uses an independent cost-basis v10 with 300 requests and 38,400
output tokens for its 100-case split; it cannot consume a Development cost
document. Schema-v16 negative-guard Development uses cost-basis v11 and the
same `900/1500000/115200` ceilings as v9. Schema-v17 buffered Development uses
cost-basis v12 with those same ceilings. Schema-v18 production-v2 Validation
uses independent cost-basis v13 with the schema-v15 `300/300000/38400`
request/input/output ceilings. Unused authority is valid, but actual
attempt/input/output totals must reconcile exactly and the v9/v10/v11/v12/v13
schema identities are never interchangeable.

Schema-v19 slice diagnostics require cost-basis v14 with exactly 765 Judge
requests and 97,920 output tokens for 255 logical executions with two retries.
Schema-v20 full Development requires cost-basis v15 with exactly 900 requests
and 115,200 output tokens. The separately named v20 abstention diagnostic cost
schema authorizes at most 513 requests and 65,664 output tokens for 171 logical
executions with two retries. Its owner-authorized conservative absolute ceiling
is diagnostic-only and is not interchangeable with schema-v20, schema-v21, or
any historical lane. None of these documents is valid for Validation.

The abstention-confirmation successor has separate, non-interchangeable
authorities. Development uses
`neo-chat.memory-regression-cost-basis.v20-confirmation-development.v1` with
exactly `1800` Judge attempts and `230400` output tokens, covering one primary
decision, at most one confirmation, and two retries for each decision across
300 cases. Schema-v21 Validation uses
`neo-chat.memory-regression-cost-basis.v21` with exactly `600` Judge attempts
and `76800` output tokens for the frozen 100-case split. The pre-network input
ceilings are bound from the corresponding full-path Fake traces with all
allowed retries: `591134 * 3`, rounded upward to `2000000`, for Development;
and `196772 * 3`, rounded upward to `600000`, for Validation. Fake Validation
is always Yellow lifecycle-only evidence and can never pass or release.

Both confirmation Vault wrappers require independent export and quota
approvals. The runner must copy and mount both the BGE and Luna credentials for
these exact modes before constructing a Provider. Missing, empty, shared, or
drifted copies fail before network. A started one-shot live authority is never
automatically rerun after any failure, including a pre-network runner defect.

The failed schema-v21 result is consumed and is not a rerun target. Its v4
successor keeps the primary prompt v2 and confirmation prompt v3 unchanged but
permits a second prompt-v3 decision only after both earlier decisions return
valid empty selections. The v22 Development cost identity binds `2700` Judge
requests and `345600` output tokens. The successful full-path Fake measured a
deterministic `886206` logical input-token upper bound; applying the maximum
three attempts gives `2658618`, rounded upward to the frozen `2700000` input
ceiling and `3045600000000` maximum Judge-cost microunits. The older
`3000000` provisional Fake document is not live authority. V4 still requires a
fresh one-shot live Development pass and then a separately versioned Validation
pass before any production promotion.
The frozen v22 Development cost document has raw/canonical decoded SHA-256
`82771c9ebb4dd521cd4b5aa584a58f39cc2e786b3677ced6e523755c8a4c2de0`/
`1599e276ae4de940a889b51b5b11e861483b25558b9be34b66607f97717bce16`.

Its one-shot Vault path is separately named and accepts only the v4 approval:

```bash
bash scripts/run-memory-double-confirmation-development-from-vault.sh \
  --cost-basis /secure/eval/memory-double-confirmation-development-v22.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --double-confirmation-development-approval \
    I_UNDERSTAND_THIS_USES_REAL_MEMORY_DOUBLE_CONFIRMATION_DEVELOPMENT_QUOTA
```

The wrapper pins the already-attested credential-export image, forbids build
and pull, exports exactly one independent BGE/Luna pair, invokes only the v22
Development mode, and wipes both copies after success, metric failure,
ordinary failure, `INT`, `TERM`, or `HUP`.

The sole authorized v22 live Development run
`memory-regression-20260809t033639z-a1cde25c`, capture
`c04bb647-96a9-4f1e-b127-ab93ed027f05`, completed `300/300` cases and passed.
Candidate Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.9897435897/0.9878787879`; false injection was `0/135`, every required
slice passed, and every safety/privacy counter was zero. Eleven primary Judge
retries recovered. Eight confirmation attempts ran with no retry or terminal
failure. Actual Judge authority reconciled at `184/2700` requests,
`329182/2700000` input-token upper bound, and `23552/345600` output-token upper
bound. Average/maximum prompt Memory was `67.85/381` tokens.

Configuration/report/manifest SHA-256 values are
`836de9c5df3dc44f7be24ef54be90d10dfb1a31f0ec14129557de8eeea4f4ca1`,
`52b454c6faf11784f7852f2784c889b6f34c5b53e72146c221a37e2613c1ef5e`,
and `a690ad5f1a30238587336b8c4c99180789e87b432d54aa8402385b208e32a664`.
All one-run credentials and scoped runtime objects were destroyed. This Green
Development remains non-promotional; it permits construction of a fresh
Validation identity but does not authorize live Validation or production.

The successor Validation lane is independently frozen as schema v23:

```text
capture  production_fixed_memory_judge_negative_guard_double_confirmation_validation
profile  neo-chat.memory-regression-profile-config.v23-double-confirmation-validation.v1
reader   neo-chat.native-memory-reader-capture.v23-double-confirmation-validation.v1
report   neo-chat.memory-regression-relevance-validation.v23-double-confirmation.v1
run      neo-chat.memory-regression-relevance-validation-run.v23-double-confirmation.v1
cost     neo-chat.memory-regression-cost-basis.v23-double-confirmation-validation.v1
```

Its PostgreSQL 17 Fake run
`memory-regression-20260809t042942z-dd53bb1f`, capture
`79f147cd-437a-4afd-b0ff-b4193ee72ab5`, completed the exact 100-case frozen
Validation order. Routing reconciled as `35` empty-candidate, `10`
negative-guard, `55` Judge-completed, and zero failed cases. Every
Judge-completed case forced one primary plus two confirmations: `165` total
Judge attempts, `110` confirmation attempts, `294993` total input-token upper
bound, `196442` confirmation-only tokens, zero retry, and zero terminal case.
All quality metrics were `1.0` except false injection, which was `0/45`; every
safety/privacy counter was zero.

Fake is deliberately Yellow `FAKE_PROTOCOL_NON_EVIDENCE` with
`passed=false`, `policySelected=false`, `promotionEligible=false`, and
`releaseEligible=false`. The report/manifest SHA-256 values are
`2632e98b130ee0e1d898247b60cf0ce093d881eedf1062baa2a4df9538d88e0d` and
`29e590baa86617e010aa5ce3d699841145d6eea032e50f5ebe94c7c490d7417b`.
Both artifacts are mode `0600`; all containers, networks, and volumes were
destroyed.

The maximum three attempts per logical Judge decision produce
`294993 * 3 = 884979`, rounded upward to a frozen `900000` input-token bound.
The independent live cost authority binds `900` requests, `115200` output
tokens, and `1015200000000` maximum Judge-cost microunits. Its private raw and
canonical decoded SHA-256 values are
`62d3ec814d2dcec2f9e79e6d391212d86f557be253e0533250b64d02f995b860` and
`92563d188a999a108b027b65a543716c3fcf6cb6b3a93f30d997dad5e2cfde41`.
The mode-`0600` document passed `DecodeCostBasis` and
`ValidateDoubleConfirmationValidationCostAuthority`; this freezes ceilings
but grants no quota authority.

The one-shot schema-v23 Vault path is independently named and accepts only the
Validation approvals:

```bash
bash scripts/run-memory-double-confirmation-validation-from-vault.sh \
  --cost-basis /secure/eval/memory-double-confirmation-validation-v23.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --double-confirmation-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_DOUBLE_CONFIRMATION_VALIDATION_QUOTA
```

The already-consumed v22 Development authority, including any later wording
that grants another Development run, cannot be reused or translated into this
schema-v23 Validation authority. Until a fresh exact live Validation approval
is granted and the run passes, product composition remains on the historical
production policy and both Memory flags plus the canary remain unchanged.

The sole authorized schema-v23 live Validation ran once from
`2026-08-09T06:36:52Z` through `06:42:02Z`. Run
`memory-regression-20260809t063653z-20bfb10b`, capture
`79231ab8-bb00-4177-aeec-daf85daf2199`, completed `100/100` but failed the
unchanged required-slice quality gate. Overall Candidate Recall@20, Final
Recall@5, current-fact accuracy, and false injection were
`1.0/0.9692307692/0.9636363636/0`. The `mixed_language_entity`, `stable_fact`,
and `temporal_correction` slices each reached only `0.9` current-fact
accuracy. Aggregate arithmetic records two Judge abstentions; each exhausted
the primary plus both confirmations, producing four confirmation attempts.
The aggregate evidence intentionally cannot identify private cases, and slice
overlap must not be used to reconstruct them.

One typed `PROVIDER_TRANSPORT_FAILED` attempt recovered. The run had `60`
Judge attempts, one retry, zero terminal case, `107182/900000` input-token
upper bound, and `7680/115200` output-token upper bound. Confirmation-only
input was `6964`. Average/maximum prompt Memory was `66.53/365` tokens. False
injection was `0/45`, and every cross-user, deleted-Memory, Secret, untrusted-
source, unauthorized-egress, token, cost, cleanup, and privacy gate passed.

The immutable outcome is Yellow `QUALITY_OR_TOKEN_GATE_FAILURE`,
`passed=false`, `policySelected=false`, `promotionEligible=false`, and
`releaseEligible=false`. Configuration/report/manifest SHA-256 values are
`01f132d35c0546860ac3983a5842d7925feed74a472900b69d50ff8470cef8f3`,
`71e71db85fed3262388991c899aa2a4799a4c9e2ace15c87552bb8dd6dbce740`,
and `446f84c76a040ced6e9f39d657f1d7f3206337410df1fe019dd6fdfa71000682`.
The private evidence root is
`/var/tmp/neo-chat-double-confirmation-validation-v23-live-20260809T063629Z`.

Both credentials and every scoped container, network, and volume were
destroyed. Aggregate artifacts and logs passed the independent credential and
private-field scans. Pre/post state hashes are
`deabaac01944db51cbdaa2ae80ab11a88b8008bf63ab8da247a8417ba5efab65` and
`e31d3be691fba7100e459611e74e897f076bfb429d7a3f6bf1a43f00f68744f7`;
all six product container IDs and images remained unchanged and healthy. The
product composition still installs the historical negative-guard production
policy, both Memory flags remain false, and the canary is empty. Schema-v23 is
consumed and cannot be rerun or used for launch.


### Prospective single-user bounded-miss successor

The owner accepted a prospective, single-user-only fail-closed omission risk.
This does not reinterpret schema-v23. The new criteria identity
`neo-chat.memory-benchmark-criteria.v4-single-user-bounded-miss` keeps overall
current-fact accuracy at `0.95`, sets required-slice current-fact accuracy to
`0.90`, and requires both false-injection rate and false-injection case count
to equal zero. Candidate Recall@20, Final Recall@5, prompt-token, safety,
privacy, Provider-egress, cost, cleanup, and terminal-failure gates are
unchanged. The exact v4 double-confirmation reader, prompts, strict decoder,
negative guard, BGE intersection, retry policy, and request order are also
unchanged.

The successor uses fresh, non-interchangeable identities:

```text
Development capture  development_fixed_memory_judge_negative_guard_double_confirmation_single_user_bounded_miss
Development profile  neo-chat.memory-regression-profile-config.v24-single-user-bounded-miss-development.v1
Development reader   neo-chat.native-memory-reader-capture.v24-single-user-bounded-miss-development.v1
Development report   neo-chat.memory-regression-relevance-calibration.v24-single-user-bounded-miss-development.v1
Development run      neo-chat.memory-regression-relevance-run.v24-single-user-bounded-miss-development.v1
Development cost     neo-chat.memory-regression-cost-basis.v24-single-user-bounded-miss-development.v1

Validation capture   production_fixed_memory_judge_negative_guard_double_confirmation_single_user_bounded_miss_validation
Validation profile   neo-chat.memory-regression-profile-config.v25-single-user-bounded-miss-validation.v1
Validation reader    neo-chat.native-memory-reader-capture.v25-single-user-bounded-miss-validation.v1
Validation report    neo-chat.memory-regression-relevance-validation.v25-single-user-bounded-miss.v1
Validation run       neo-chat.memory-regression-relevance-validation-run.v25-single-user-bounded-miss.v1
Validation cost      neo-chat.memory-regression-cost-basis.v25-single-user-bounded-miss-validation.v1
```

The PostgreSQL 17 Fake Development run
`memory-regression-20260809t090107z-83ce1740`, capture
`83cab806-40b6-4c4d-9f02-363cfa8e7c4b`, completed all 300 cases. Candidate
Recall@20, Final Recall@5, and current-fact accuracy were `1.0`; false
injection was `0/135`. It reconciled `105` empty-candidate, `30`
negative-guard, `165` Judge-completed, and zero failed cases. All `495` Judge
attempts completed without retry or terminal failure; `330` were confirmation
attempts. Total and confirmation-only input-token upper bounds were `886206`
and `590144`; output-token authority reconciled at `63360/345600`. The report
passed but remains non-promotional.

The PostgreSQL 17 Fake Validation run
`memory-regression-20260809t090148z-629b20fa`, capture
`246e3896-fcf8-430c-9501-48f375c043c2`, completed the exact 100-case order as
`35/10/55/0` empty/guard/Judge-completed/failed. Every quality metric was
`1.0`, false injection was `0/45`, and every safety/privacy counter was zero.
It performed `165` Judge attempts, including `110` confirmations, with
`294993` total and `196442` confirmation-only input tokens. As required, Fake
returned non-zero and retained Yellow `FAKE_PROTOCOL_NON_EVIDENCE` with
`passed=false`, `releaseEligible=false`, and no launch authority.

The shared prospective criteria SHA-256 is
`2c7e7325d4f8bc5d991857b9a8a90d76d866da98ce26a6c171072351ce43e779`.
Schema-v24 report/manifest SHA-256 values are
`aba8b0d81e3a87c9227671a7cd3bc65467f376167f3111be8936512264ba98e9`
and `638d0d128781dc0d65a4f832366179260ce43f5e4c91c5c286b364bad888511b`;
schema-v25 values are
`e0d431b2f49e0fec7c1295a41c1c0c58d1952d934e6623d84ccbe972f1f997d5`
and `6dc9d45e43957c65f6599fa9fed053cf3c407feed11e20cb67fe74c298233fb4`.
Both isolated Compose projects and all credentials were destroyed; retained
artifacts are mode `0600` under
`/var/tmp/neo-chat-single-user-bounded-miss-v24-v25-fake-20260809T090053Z`.

Because criteria do not change Provider request shape, schema-v24 retains the
frozen v22 `2700/2700000/345600` request/input/output ceiling, and schema-v25
retains the schema-v23 `900/900000/115200` ceiling. The private schema-v24
cost document raw/canonical decoded SHA-256 values are
`7f3f9a1c35f844b5f62aa3a77e10ec2b92e8c82bd8f3232522fe8fc41fb8a1c5` and
`bd5e1e73a9feb7a878e905aa4ecb0ca734dcae4d78c9c9f65f2bacc67c6337e9`;
schema-v25 values are
`81b624658ea5b00dea6bfaa88ed8684cb9c938ec8c8fd4a1db3fd4904e64bdc7` and
`67edc4abbe1e0cba610474947382b1d05df80a75aab194805f34aa2114192eda`.
Both passed strict decode and their phase-specific cost validator during the
Fake lifecycle; neither grants live quota authority.

The one-shot Vault paths are independently named and approval-isolated:

```bash
bash scripts/run-memory-single-user-bounded-miss-development-from-vault.sh \
  --cost-basis /secure/eval/memory-single-user-bounded-miss-development-v24.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --single-user-bounded-miss-development-approval \
    I_UNDERSTAND_THIS_USES_REAL_MEMORY_SINGLE_USER_BOUNDED_MISS_DEVELOPMENT_QUOTA

bash scripts/run-memory-single-user-bounded-miss-validation-from-vault.sh \
  --cost-basis /secure/eval/memory-single-user-bounded-miss-validation-v25.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --single-user-bounded-miss-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_SINGLE_USER_BOUNDED_MISS_VALIDATION_QUOTA
```

The separately authorized schema-v24 live Development ran exactly once as
`memory-regression-20260809t093235z-c17a5d0a`, capture
`3eff1154-2fe9-40b1-af9c-cc6c4d1eb574`, and passed all 300 cases. Candidate
Recall@20 was `1.0`, Final Recall@5 was `0.9846153846153847`, overall
current-fact accuracy was `0.9818181818181818`, and false injection was
`0/135`. Every slice passed; the lowest slice current-fact accuracy was
`0.9666666666666667`, above the bounded-miss `0.90` floor. Safety counts,
terminal failed cases, and unauthorized Provider egress remained zero.

The route ledger reconciled `105` empty-candidate, `30` negative-guard,
`165` Judge-completed, and zero failed cases. The live Judge made `180`
attempts, including `9` recovered retries and `6` confirmation attempts. Its
request/input/output upper-bound ledger was `180/322607/23040`, within the
frozen `2700/2700000/345600` authority. The aggregate report and manifest
SHA-256 values are
`68d00d695c521e4f240b105f9ecc237b6c7d3c66ce8e9d40c7f3ae72528b34b0` and
`39a92d7174bd8f725cdb36152a33b7c6eded3fe4feb8beb6aea56caf4d363bd1`.
Both are mode `0600` under the private mode-`0700` root
`/var/tmp/neo-chat-single-user-bounded-miss-v24-live-20260809T093234Z`.

The schema-v24 authority is consumed and cannot be rerun. Its credential
copies and every scoped container, network, and volume were destroyed. The
production Memory flags remain false and the canary remains empty. Schema-v25
then ran once under its own exact live authorization as
`memory-regression-20260809t160732z-c5e0da74`, capture
`7ec205c3-9c65-4684-8063-e52e35d4c551`. All 100 Validation cases passed with
Candidate Recall@20, Final Recall@5, and current-fact accuracy all `1.0`, false
injection `0/45`, and every safety/privacy counter zero. Routing reconciled at
`35/10/55/0`; the Judge made `63` attempts, including `6` recovered retries
and `3` confirmation attempts. Request/input/output upper bounds were
`63/112543/8064`, within the frozen `900/900000/115200` authority.

The schema-v25 report/manifest SHA-256 values are
`ead77fd8f505813ec7bae1f0b8cf934c6798797afb9760c15b5a2017adbc2be3` and
`d97f3c4f1ee865eaf47e9ca188df4875c7266e07ab562d3687d9f9699c521bce`.
Both mode-`0600` artifacts remain under the private mode-`0700` root
`/var/tmp/neo-chat-single-user-bounded-miss-v25-live-20260809T160731Z`.
Credential copies and all scoped runtime objects were destroyed. The Validation
report intentionally remains non-promotional and requires owner review; the
owner's earlier pass-then-launch decision supplies the separate rollout intent.

Launch preflight found the live database at migration `069` while the reviewed
candidate backend requires additive Memory-health migration `070`. The source
launch contract forbids silently converting a flag-only rollout into a schema
change. PostgreSQL, MinIO, the live environment, container identities, row
counts, and the sole exact UUID were captured under
`/var/tmp/neo-chat-memory-v2-single-user-launch-20260809T162030Z`. An isolated
restore proved `069 -> 070 -> 069 -> 070` with all pre-existing persistent
table counts unchanged, but no live migration, flag change, service recreation,
or smoke was performed without expanded migration authority. Both schema-v24
and schema-v25 live authorities are consumed and cannot be rerun. A later
launch remains limited to the current sole exact authenticated UUID; adding a
user requires a new rollout decision.

The owner then separately authorized live migration `070`; it was applied
exactly once and all persistent counts remained unchanged. Exact-UUID canary
parsing and production v4 policy admission were corrected in the reviewed
image
`mm-chat/backend:memory-v2-v25-exact-uuid-policy-fix-candidate-20260810t011729z`
with image ID
`sha256:c79fd467421342c63185321fa966039419528ee507c9b0538f69901fe3568e08`.
Focused race tests, all backend tests, vet, and a 100-case Fake schema-v25
lifecycle passed before the image was deployed.

One fresh live exact-UUID admitted-smoke authority with
`provider.source=server-default` was consumed exactly once. The request
returned HTTP `200`, completed the Memory Tool and generation steps, released
one final Memory, wrote one immutable Usage row, and ended with
`message.completed`. A separately isolated non-admitted proof returned HTTP
`401` with zero Provider/Memory work and zero new hybrid observations. The live
smoke conversation was then soft-deleted with HTTP `204`.

The mandatory post-smoke health gate nevertheless returned
`degraded/memory_index_failed`. Content-free SQL proved one ready current
projection with no projection pending/failure, plus five future
`review_expire` capture jobs and two historical terminal extract jobs. The HTTP
`pendingCount` and `failedCount` intentionally aggregate capture and projection
lanes, so the result was not a projection join/count defect. Successful
retrieval does not override a degraded runtime gate.

The prepared fail-closed behavior rollback atomically restored
`MEMORY_HYBRID_SHADOW_ENABLED=false`,
`MEMORY_TOOL_LOOP_ENABLED=false`, and an empty exact-UUID canary, recreating
only `memory-worker` and `backend`. Both services are healthy; schema remains
`070` and `user_memories/users` remain `2/1`. The admitted-smoke authority is
consumed and must not be replayed. Memory v2 is not launched; a future rollout
requires a separately reviewed health remediation that preserves historical
job evidence and the unchanged fail-closed gate.

The reviewed offline remediation is additive migration `071`. It preserves the
applied `070` bytes, counts only `extract` jobs as capture health, and leaves all
future `review_expire` jobs intact outside that lane. Historical extract dead
letters remain degraded until the bootstrap owner appends one exact-job,
exact-error resolution with the fixed admin approval. The original job/error/
audit/Activity evidence is never updated or deleted. `source_no_longer_current`
requires `SOURCE_DRIFT` plus a present non-active same-owner Conversation;
`historical_failure_accepted` requires at least 24 hours of terminal age.

Disposable PostgreSQL 17 passed `070 -> 071 -> 070 -> 071`, valid resolution
and idempotent replay, user/error/status/age/source drift rejection, runtime
least privilege, append-only denial, and rollback refusal after evidence. This
does not authorize the live schema change or either live acknowledgement. Once
separately authorized, the launch continuation rechecks health, persistent
counts, non-admitted zero-work behavior, and rollback readiness without
replaying the consumed Provider smoke.

The offline release gate passed focused race checks, exact PostgreSQL 17 replay,
Frontend `964/964`, all Backend tests/vet, RAG `1906 passed / 7 skipped`, and
the full standalone verifier. The packaged, undeployed image is
`mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z`
with image ID
`sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b`.
Package inspection proved the migrator embeds `071` and the admin command is
present; the live runtime remains on the prior policy-fix image at schema
`070`.
The same exact image passed isolated PostgreSQL 17 `001 -> 071`, clean
`071 -> 070 -> 071`, packaged admin create/idempotent replay, original-error
preservation, zero post-resolution capture failures, and post-evidence down
refusal. The private disposable runtime was fully destroyed.

The owner subsequently granted one live migration-`071` authority. A fresh
custom-format PostgreSQL backup passed checksum verification before the old
backend/Worker were stopped. The frozen migrator changed exactly `071`, and
the frozen candidate image was then deployed only to those two services with
both Memory flags still false and the canary empty. Both services are healthy;
schema/data counts are `71/2/1`, resolutions remain zero, capture health is
`0/0/2`, projection health is `1/0/0`, and all five future `review_expire` jobs
remain unchanged. No Provider, Chat, acknowledgement, or Memory write was
authorized or performed. The migration authority is consumed.

The owner then separately authorized both exact historical acknowledgements.
The packaged admin boundary created `source_no_longer_current` for job
`d141be78-01e7-47e5-9db8-09d6e3938451` with expected error `SOURCE_DRIFT`, and
`historical_failure_accepted` for job
`3b384bee-87f5-41e2-9d28-72a8114c4459` with expected error
`EXTRACTION_INVALID`. Both returned `created=true`; their original job hashes
remained unchanged. Resolution count became `2`, unresolved extract dead
letters became zero, capture health became `0/0/0`, and projection health
remained `1/0/0`. The one-shot admin environment was overwritten after use.
Resolution evidence now permanently guards migration-`071` down; rollback is
behavior-only.

Provider-free launch preflight reconfirmed schema/resolution/Memory/user counts
`71/2/2/1`, the sole exact UUID
`00000000-0000-0000-0000-000000000001`, zero active capture/embedding work,
four retained hybrid observations, one retained Usage row, and the exact
candidate image ID. The consumed admitted smoke was not replayed. The launch
atomically enabled both reviewed flags, populated only that UUID, and recreated
only `memory-worker` and `backend` with `--no-build --no-deps`.

The first final-verifier attempt used the nonexistent identity path
`/v1/auth/me`; its HTTP `404` triggered the prepared fail-closed behavior
rollback. Schema, rows, image, and unrelated services were unchanged, and no
Provider or Chat request occurred. After source verification established
`/v1/me` as the correct route, a fresh Provider-free retry succeeded. The fixed
development owner and exact UUID were verified, `GET /v1/memory-health`
returned HTTP `200` with `ready`, Worker capabilities were `true/true`, capture
was `0/0/0`, and projection was `1/0/0`. A disposable same-image
`AUTH_MODE=required` instance returned HTTP `401` for an unauthenticated health
request with zero Provider/Memory work and unchanged durable counts.

Memory v2 is now live for the sole user with
`MEMORY_HYBRID_SHADOW_ENABLED=true`, `MEMORY_TOOL_LOOP_ENABLED=true`, and the
exact one-UUID canary. Both services remain healthy on image
`sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b`;
schema/resolution/Memory/user counts remain `71/2/2/1`. Successful mode-
`0700/0600` evidence is under
`/var/tmp/neo-chat-memory-v2-sole-user-launch-retry-20260810T033338Z`; the
fail-closed verifier attempt is retained separately under
`/var/tmp/neo-chat-memory-v2-sole-user-launch-20260810T032745Z`. Adding another
user requires a new rollout decision.

Each full fake-protocol run directory is mode `0700` and contains five
mode-`0600` files:

```text
native-v1-lexical.observations.json
native-v1-lexical.report.json
native-v2-hybrid[-fake-protocol].observations.json
native-v2-hybrid[-fake-protocol].report.json
run-manifest.json
```

Historical calibration, schema-v4/v5 cloud-judge Development, schema-v6-v9
Tool-route evidence, schema-v10 configured-candidate-judge Development, and
schema-v11 fixed-Memory-Judge Development, schema-v12 accuracy-first
Development, schema-v13 Judge-failure-diagnostic Development, schema-v14
transport-stable Development, schema-v15 production Validation, and schema-v16
negative-guard Development and schema-v17 buffered-judge Development
and schema-v18 production-v2 buffered Validation
and schema-v19 slice diagnostics and schema-v20 accuracy-repair Development
and the separately versioned v20 abstention diagnostic
and the confirmation Development/schema-v21 Validation successor
directories contain their named aggregate report plus `run-manifest.json`. In
every mode, evidence is exclusively linked first and the content-free
run manifest is the final completion marker. Existing targets are refused
before Provider work and are never overwritten. A metric/no-feasible failure
retains valid aggregate evidence and returns non-zero. An input, capture,
validation, publication, or signal failure removes partial output. Every exit
destroys the random Compose project's database, role, container, internal
network, egress network (live only), and volume, then removes temporary
database and Provider credentials.

Neither profile writes prompt Memory or Usage. For scoring only, the v2 final
IDs are copied into `injectedMemoryIds` as an offline counterfactual. Fake and
live reports remain `machine_reviewed_regression`, `regression_only`, and
`promotionEligible=false`; no run calls Golden freeze/Holdout or a promotion
pointer.

## 7. Keep promotion separate

A passing report is evidence, not authority. It does not toggle a feature flag,
change an active reader pointer, enable Learn/Use, start a worker, run a
migration, or activate Hindsight. Promotion remains a separately authorized
operator action after the owning implementation phase passes its shadow,
canary, deletion, resource, and rollback gates.
