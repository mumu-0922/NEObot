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
neo-chat.memory-regression-relevance-calibration.v3
neo-chat.memory-regression-relevance-calibration.v4
neo-chat.memory-regression-relevance-calibration.v5
neo-chat.memory-regression-relevance-calibration.v6
neo-chat.memory-regression-relevance-calibration.v7
neo-chat.memory-regression-relevance-validation.v1
neo-chat.memory-regression-relevance-run.v1
neo-chat.memory-regression-cost-basis.v2
neo-chat.memory-regression-cost-basis.v3
neo-chat.memory-regression-cost-basis.v4
neo-chat.memory-regression-cost-basis.v5
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
checks, lifecycle topology, and migration-065 PostgreSQL 17 replay pass. No
schema-v7 live Development run has been executed, so no policy is frozen and
Validation/Promotion remain blocked.

Only after a schema-v7 first-round Tool Loop Development hypothesis passes may
its policy, Provider/model, Tool/adapter profile, and selection behavior be
frozen in code. The current Validation CLI remains unavailable because no
schema-v7 policy is frozen and it does not yet accept the second route
credential. The historical single-Provider Validation command shape remains:

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

Validation cannot run while the frozen policy is unavailable and never
retunes. It retains only `relevance-validation.json` and `run-manifest.json`.
The visible machine `holdout` is rejected by the split selector and is not a
CLI mode. Passing Validation still has no promotion authority.

For each live phase, the wrapper copies that phase's Key into a temporary
mode-`0600` file and mounts it read-only. Tool-route Development does this for
both independent credentials and rejects the same file, hard links, or equal
Key bytes. Values never enter argv, environment variables, Compose config,
Docker inspect, reports, or Git. Both in-process byte buffers are cleared, and
retained artifacts, runner logs, and Docker metadata are scanned for both
secrets. When the owner authorizes Server Vault reuse for Development, a
separate operator step must first create the two mode-`0600` input files and
must overwrite/remove them afterward; the runner itself never inspects or
decrypts the Vault. Live output alone uses profile `native_v2_hybrid`.

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

Each full fake-protocol run directory is mode `0700` and contains five
mode-`0600` files:

```text
native-v1-lexical.observations.json
native-v1-lexical.report.json
native-v2-hybrid[-fake-protocol].observations.json
native-v2-hybrid[-fake-protocol].report.json
run-manifest.json
```

Historical calibration, schema-v4/v5 cloud-judge Development, and Validation
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
