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
neo-chat.memory-regression-relevance-validation.v1
neo-chat.memory-regression-relevance-run.v1
neo-chat.memory-regression-cost-basis.v2
neo-chat.memory-regression-cost-basis.v3
neo-chat.memory-regression-cost-basis.v4
neo-chat.memory-regression-cost-basis.v5
neo-chat.memory-regression-cost-basis.v6
neo-chat.memory-regression-cost-basis.v7
neo-chat.memory-regression-cost-basis.v8
neo-chat.memory-cloud-candidate-judge-input.v1
neo-chat.memory-cloud-candidate-judge-output.v1
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
go run ./cmd/memory-benchmark-author regression-v3-generate \
  [-root <new-protected-v3-regression-root>]
go run ./cmd/memory-benchmark-author regression-v3-status|regression-v3-verify \
  [-root <protected-v3-regression-root>]
go run ./cmd/memory-benchmark-author regression-v4-generate \
  [-root <new-protected-v4-regression-root>]
go run ./cmd/memory-benchmark-author regression-v4-status|regression-v4-verify \
  [-root <protected-v4-regression-root>]
go run ./cmd/memory-benchmark-author regression-v5-generate \
  [-root <new-protected-v5-regression-root>]
go run ./cmd/memory-benchmark-author regression-v5-status|regression-v5-verify \
  [-root <protected-v5-regression-root>]
```

```go
memoryauthor.GenerateRegression() (memoryauthor.RegressionPool, error)
memoryauthor.GenerateRegressionV3() (memoryauthor.RegressionPool, error)
memoryauthor.GenerateRegressionV4() (memoryauthor.RegressionPool, error)
memoryauthor.GenerateRegressionV5() (memoryauthor.RegressionPool, error)
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
  --capture-mode full_regression \
  --cost-basis /secure/eval/memory-regression-cost-basis.json \
  --output-dir /secure/eval/native-memory-runs

# Fixed global Memory Judge successor. The tuple is immutable and independent
# from the answer model. Fake protocol is lifecycle evidence only.
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-cost-v7.json \
  --output-dir /secure/eval/native-memory-runs

# Accuracy-first successor. Fake protocol uses a virtual cooldown clock and is
# lifecycle evidence only; it never supplies quality or promotion evidence.
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_accuracy \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-accuracy-cost-v8.json \
  --output-dir /secure/eval/native-memory-runs

bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode development_cloud_judge \
  --cloud-judge-model Qwen/Qwen3-8B \
  --credential-file /secure/input/fresh-siliconflow.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --cost-basis /secure/eval/cloud-judge-cost-v2.json \
  --output-dir /secure/eval/native-memory-runs

# Main-model Memory Tool-route Development requires two different fresh,
# mode-0600 credentials: fixed SiliconFlow BGE and the exact GPT/DeepSeek route
# Provider. GPT and DeepSeek are separate runs and separate hypotheses.
bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode development_memory_tool_route \
  --credential-file /secure/input/fresh-siliconflow-bge.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-tool-route-credential-file /secure/input/fresh-gpt-route.key \
  --memory-tool-route-provider-id configured-gpt \
  --memory-tool-route-provider-type openai \
  --memory-tool-route-base-url https://api.openai.com/v1 \
  --memory-tool-route-model exact-configured-model \
  --memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  --cost-basis /secure/eval/memory-first-tool-round-cost-v5.json \
  --output-dir /secure/eval/native-memory-runs

# Measurement-only successor with the identical Provider request/cost shape.
# It requires fresh explicit quota authorization and can never select a policy.
bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode development_memory_tool_route_diagnostic \
  --credential-file /secure/input/fresh-siliconflow-bge.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-tool-route-credential-file /secure/input/fresh-route.key \
  --memory-tool-route-provider-id exact-configured-provider \
  --memory-tool-route-provider-type openai_compatible \
  --memory-tool-route-base-url https://provider.example/v1 \
  --memory-tool-route-model exact-configured-model \
  --memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  --cost-basis /secure/eval/memory-first-tool-round-cost-v5.json \
  --output-dir /secure/eval/native-memory-runs

# Candidate-first successor. Fake protocol proves only the isolated lifecycle
# and exact configured-Provider bindings; it is not quality evidence.
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_configured_candidate_judge \
  --configured-candidate-judge-provider-id configured-gpt \
  --configured-candidate-judge-provider-type openai \
  --configured-candidate-judge-base-url https://api.openai.example/v1 \
  --configured-candidate-judge-model exact-configured-model \
  --cost-basis /secure/eval/configured-candidate-judge-cost-v6.json \
  --output-dir /secure/eval/native-memory-runs

# Only after the selected Development values are frozen in code; use a new,
# separately authorized mode-0600 Key file.
bash scripts/run-memory-regression.sh \
  --provider-mode live_siliconflow \
  --capture-mode frozen_validation \
  --credential-file /secure/input/fresh-validation-siliconflow.key \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --cost-basis /secure/eval/memory-regression-cost-basis.json \
  --output-dir /secure/eval/native-memory-runs
```

```go
memorycapture.LoadProtectedRegression(root string) (memorycapture.ProtectedRegression, error)
memorycapture.SeedEphemeralDatabase(ctx, adminDB, pool, index, runID) (memorycapture.SeedResult, error)
memorycapture.PopulateProjectionVectors(ctx, adminDB, runID, embedder) (int, error)
memorycapture.CaptureProfiles(ctx, adminDB, runtimeDB, runID, index, seed, provider, hashes, cost) (memorycapture.CapturedProfile, memorycapture.CapturedProfile, error)
memorycapture.CaptureMemoryToolRouteDevelopment(ctx, adminDB, runtimeDB, runID, pool, index, seed, provider, router, modelID, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildMemoryToolRouteDevelopmentReport(pool, profile, authority, costBasis) (memorycapture.MemoryToolRouteDevelopmentReport, []byte, error)
memorycapture.CaptureMemoryToolRouteDiagnostic(ctx, adminDB, runtimeDB, runID, pool, index, seed, provider, router, modelID, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildMemoryToolRouteDiagnosticReport(pool, profile, authority, costBasis) (memorycapture.MemoryToolRouteDevelopmentReport, []byte, error)
memorycapture.CaptureConfiguredCandidateJudgeDevelopment(ctx, adminDB, runtimeDB, runID, pool, index, seed, provider, judge, authority, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildConfiguredCandidateJudgeDevelopmentReport(pool, profile, authority, costBasis) (memorycapture.ConfiguredCandidateJudgeDevelopmentReport, []byte, error)
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
- The legacy v2 generator and every historical artifact/hash remain immutable.
  The separately named v3 generator changes only the `unrelated_negative`
  contract: a normal agenda-heading query shares entity/topic/scope terms with
  a weather-board Memory that cannot answer the query. Its semantic audit
  requires those task/observation markers and forbids `unrelated`, `无关`,
  `no bearing`, and `没有关系` in both query and candidate.
- V4 is the exact tuple generator
  `neo-chat.memory-benchmark-regression-generator.v3`, profile
  `memory-regression-zh-mixed-v4`, seed `2026080101`, auditor
  `deterministic-semantic-audit.v3`, and audit time
  `2026-08-01T00:00:00Z`. It does not rewrite v2/v3. Every one of its 275
  positive cases maps the uniquely queried Subject to the positionally aligned
  current value; each relevant Memory must contain that Subject/value pair,
  and every temporal superseded Memory must contain the aligned old pair.
  Aggregating multiple relevant Memories before this check is forbidden
  because one valid multi-hop record could mask another mutated record.
- Every v4 `unrelated_negative` keeps the exact synthetic entity/scope but
  omits both language forms of the queried Subject. Its candidate must contain
  `facilities inspection`/`设施巡检` plus weather-board/sunshine markers and must
  not contain agenda, meeting, discussion, or exact-task-event claims. Deleting
  it therefore leaves the requested agenda heading and every task premise
  unchanged; keyword-family separation alone is insufficient.
- V5 is the exact tuple generator
  `neo-chat.memory-benchmark-regression-generator.v4`, profile
  `memory-regression-zh-mixed-v5`, seed `2026080102`, auditor
  `deterministic-semantic-audit.v4`, and audit time
  `2026-08-01T08:30:00Z`. It preserves v2/v3/v4 and all v4 positive
  Subject/current/old semantics. Its `unrelated_negative` is a same-entity and
  same-scope physical observation: the commemorative mug is on the lounge's
  left third shelf. The audit requires those location markers and rejects all
  20 Subjects, every current/old value in both languages, and every v3/v4
  meeting, agenda, discussion, facilities, weather, and sunshine event marker
  from the hard-negative candidate.
- The default known roots are the gitignored v2/v3/v4/v5 paths under
  `mm-chat/data/memory-benchmark/`. Their final path component must explicitly
  contain `regression`; publication is create-only with `0700/0600`
  permissions. Verification dispatches only from an exact known generator
  tuple, rejects mixed v2/v3/v4/v5 artifacts, and byte-compares fixtures,
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
- Live full-regression mode is forbidden. Development calibration seeds and
  executes exactly the 300 `development` cases; frozen validation seeds and
  executes exactly the 100 `validation` cases. The visible machine `holdout`
  is rejected by the split selector and has no CLI entrypoint.
- Development uses the calibration-only `-1.00/0.00` policy to obtain complete
  request-local admission/rerank traces. Before Provider construction, the
  configuration hash commits to capture mode, split, policy ID/mode,
  thresholds, the fixed `[-1.00,1.00] x [0.00,1.00]` step-`0.01` grid, and the
  fixed selection objective. Calibration evaluates all `20,301` pairs through
  the shared scorer, including safety, latency, token, hard-cutoff, slice, and
  exact Provider-cost gates.
- After the fixed scalar grid proved infeasible, Development calibration also
  uses `memory_hybrid_relevance_intent_calibration_v1`. Before any Memory
  document egress, the fixed reranker compares only the redacted query with two
  non-user bilingual intent anchors. The anchor version/SHA-256 and the
  `[-1.00,1.00]` step-`0.01` positive-minus-negative margin grid are hashed
  before Provider construction. All `201` thresholds use the unchanged shared
  scorer.
- Calibration artifacts are aggregate-only: grid/frontier counts and metrics,
  the optional selected pair, fixed failure-pair counts, best safety/recall
  attempts, cumulative admission/max-rerank/top-two-margin threshold curves,
  and a run manifest. Curve rows contain only relevant/unrelated-negative
  eligible/missing and passing case counts; a single-candidate row is missing
  from the margin curve. They contain no case ID, query, Memory plaintext, raw
  vector/rerank score, credential, or observation file. The diagnostics
  version is configuration-hashed before Provider work. A failed/no-feasible
  calibration is retained as valid non-zero-exit evidence.
- Intent calibration additionally retains only cumulative relevant/unrelated
  margin counts, intent failure-threshold counts, safety/recall attempts, and
  an optional selected margin. The Provider call contains no Memory plaintext;
  missing, invalid, drifted, late, or low-margin intent evidence fails closed
  before local admission and candidate-document rerank.
- The completed schema-v3 Development evidence found `0/201` feasible intent
  thresholds. Zero unrelated egress retained `31/165` relevant current-fact
  cases, while full recall caused `26` false injections and unauthorized
  egress events. No intent policy is frozen; Validation remains unavailable.
- Do not tune more query-only anchors from benchmark outcomes. Any next policy
  must be separately versioned and candidate-aware. The owner subsequently
  authorized ordinary current-user Memory candidates for the configured cloud
  Provider in this single-user Server-mode deployment, enabling schema-v4
  cloud-judge Development without weakening forbidden-candidate controls.
- A no-feasible fixed scalar result activates research into a query-class,
  intent, or margin policy but does not authorize one. The aggregate curves
  must first show the separating signal on Development; evaluator gates remain
  unchanged and Validation remains unavailable until a passing policy is
  frozen in code.
- Schema-v4 Development uses
  `memory_hybrid_cloud_candidate_judge_calibration_v1`, reader version
  `neo-chat.native-memory-reader-capture.v3`, and the exact
  `owner_authorized_normal_candidates_v1` egress policy. Only `irrelevant`
  exclusion is newly authorized. Cross-user, out-of-scope, deleted, secret,
  superseded, Sensitive-disabled, and untrusted-source egress remains a hard
  failure, and false injection is scored exactly as before.
- The cloud judge receives the deterministic secret-redacted query plus
  current-authorized candidate bodies labelled by contiguous request-local
  ordinals. It receives no Memory IDs, revisions, scopes, raw scores, or
  database authority. Candidates are untrusted data. The exact output has only
  `schemaVersion` and at most five unique in-range `selectedOrdinals`; an empty
  array means `no_memory`.
- Judge model ID, prompt version/SHA-256, and
  `temperature-0_max-output-128_no-thinking_v1` are configuration-hashed before
  Provider construction. BGE rerank and the judge run concurrently under the
  existing hard cutoff. Either failure, timeout, malformed output, or
  provenance drift fails closed. Final order is the judge ordinal set
  intersected with BGE order, followed by the unchanged Top-5 and 600/900-token
  selector.
- Schema-v4/v5 retains only `cloud-judge-development.json` and
  `run-manifest.json`. The report contains shared aggregate evaluator metrics,
  bounded failure-code counts, request/input/output token upper bounds, exact
  policy/model/prompt identities, and no case ID, query, Memory plaintext, raw
  score, raw judge output, observation file, or credential.
- Cost-basis v2 fixes exactly 300 authorized judge requests and `300 * 128 =
  38,400` maximum output tokens. Maximum input tokens are conservatively
  accumulated from UTF-8 bytes plus framing per request. Exact input/output
  microunit prices and the resulting maximum judge cost are bound before
  Provider construction; candidate Memory cost must cover that maximum, and
  actual aggregate token upper bounds cannot exceed authorization.
  Officially free fixed judge models use exact zero input/output prices and a
  zero maximum judge cost; the total candidate Memory cost remains positive
  because the BGE stages are still priced. Fabricating a non-zero judge price
  is forbidden.
- The first schema-v4 live run fixed `Qwen/Qwen3-8B` and completed all 300
  Development cases. It failed Final Recall@5/current-fact/false-injection and
  p95/p99 latency gates, including 31 fail-closed hard-cutoff judge requests.
  It selected no policy and its Key was destroyed.
- The owner removed relative Provider expense as a selection constraint for
  this single-owner deployment. Schema-v5 therefore evaluates the separately
  precommitted `deepseek-ai/DeepSeek-V4-Flash` hypothesis under
  `owner_authorized_absolute_cap_v1`. The historical ratio remains reported,
  but the profile gates exact official prices, request/token ceilings, and the
  absolute run ceiling. It does not alter any quality, safety, latency, token,
  split, privacy, or promotion gate.
- The DeepSeek schema-v5 Development run failed with `164/195` hard-cutoff
  judge requests, Final Recall@5/current-fact `0.143590/0.145455`, and p95/p99
  `1856/1865 ms`. It had zero false injection and zero authority/privacy leaks,
  but selected no policy. The next named Development hypothesis was
  `Qwen/Qwen3.6-35B-A3B` with fresh model/rate/cost/credential authority.
- Qwen3.6 also failed with Final Recall@5/current-fact `0.733333/0.733333`,
  `15/300` false injections, p95/p99 `1854/1856 ms`, and `40/195` hard-cutoff
  judge requests. The planned `Qwen/Qwen3.5-4B` run was cancelled before
  Provider construction, credential creation, or quota use as
  `cancelled_not_run_architecture_pivot`; it has no fabricated model result.
- Historical schema-v6 replaced additional hidden-judge model hopping with
  `development_memory_tool_route`, reader
  `neo-chat.native-memory-reader-capture.v4`, policy
  `memory_hybrid_main_model_tool_route_calibration_v1`, and admission mode
  `development_main_model_memory_tool_route_only`. The current explicitly
  configured GPT or DeepSeek model receives only the deterministic
  secret-redacted current query and the exact no-argument `search_memory` Tool.
  It receives no Memory candidate body, Memory ID, scope, revision, score, or
  database authority.
- Its Tool authority was fixed as `memory-search-tool-v1`, SHA-256
  `f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6`,
  `memory-search-tool-decoding-v1`, `temperature=0`, maximum output `128`, and
  disabled thinking. Official OpenAI omits the non-standard
  `enable_thinking` field; OpenAI-compatible Providers encode
  `enable_thinking=false`.
- Zero Tool Calls is the only normal `no_memory` decision. Use Memory requires
  exactly one choice containing exactly one call with a non-empty ID, exact
  name `search_memory`, and an explicitly decoded empty JSON object `{}`.
  Missing/null/malformed/non-empty arguments, unknown names, duplicate calls,
  Provider failure, timeout, or model/contract drift fail closed.
- The schema-v6 Tool route and fixed BGE work may overlap under the existing two-second
  request boundary. BGE candidates remain request-local and never reach the
  route model. A valid Tool Call releases only the unchanged BGE rerank order,
  Top-5, and 600/900-token selection; abstention or failure yields empty final,
  injected, and prompt-token surfaces. This Development seam does not yet run a
  product same-model answer continuation.
- Profile config v6 hashes the exact route Provider ID/type, normalized Base URL
  SHA-256, model, Tool contract/decoding tuple, BGE tuple, limits, owner egress
  policy, absolute cost authority, and unchanged evaluator criteria before
  Provider construction. `openai` and `openai_compatible` are the only admitted
  route Provider types. GPT and DeepSeek are independent named Development
  hypotheses; evidence from one cannot authorize the other.
- Cost-basis v4 binds exactly 300 route requests, `38,400` maximum output
  tokens, a conservative aggregate input-token ceiling, exact rates, and an
  absolute route-cost ceiling to the same Provider/model/Base-URL hash. The two
  live credential files must be different regular non-symlink mode-`0600`
  files, not hard links and not equal byte content; both byte buffers are
  cleared and scanned from retained artifacts, logs, and Docker metadata.
- Schema-v6 retains only `memory-tool-route-development.json` and
  `run-manifest.json`. The report contains aggregate evaluator metrics, route
  completion/use/abstention/failure counts, token/cost ceilings, and exact
  profile authority. It contains no case ID, query, Memory plaintext, raw Tool
  response, raw score, credential, or observation file.
- The 300-case schema-v6 `fake_protocol` PostgreSQL 17 replay completed with 300 route
  requests, 300 completed decisions, zero protocol failures, a conservative
  input-token upper bound of `358533`, private artifacts, and total Compose
  teardown. Its deterministic routing intentionally fails quality gates and is
  protocol evidence only. Subsequent GPT and corrected DeepSeek live
  Development runs failed; the first DeepSeek run is protocol-invalid. The
  independent `PlanTools` preflight architecture is rejected and schema-v6 is
  immutable failed evidence.
- Schema-v7 is the separate successor under the existing capture-mode string
  `development_memory_tool_route`. It uses reader
  `neo-chat.native-memory-reader-capture.v5`, profile config v7, policy
  `memory_hybrid_main_model_first_tool_round_calibration_v1`, admission mode
  `development_main_model_first_tool_round_only`, adapter
  `chat-first-tool-round-memory-decision-v1`, report schema v7, cost-basis v5,
  and artifact `memory-first-tool-round-development.json`.
- The schema-v7 adapter receives a `chat.ToolRoundProvider` and emits one real
  first `ProviderRoundRequest` with the current synthetic query/message,
  canonical `search_memory` definition, `tool_choice=auto`, and no continuation.
  It does not call `PlanTools` and does not force the historical preflight's
  temperature, maximum-output, or thinking-control fields. Zero calls abstains;
  one exact non-empty-ID `search_memory({})` call authorizes the unchanged BGE
  Development final surface; every other call/event/failure shape fails closed.
- `internal/chat` is the single definition/hash/validation authority.
  `internal/memoryroute` delegates to that contract and is not production
  activation authority. The product Tool Loop separately performs retrieval,
  migration-065 final hydration, and same-model continuation behind the
  default-off `MEMORY_TOOL_LOOP_ENABLED` flag.
- Schema-v7 offline units, fake-Provider protocol tests, report/manifest checks,
  regression topology/lifecycle tests, and migration-065 PostgreSQL 17 replay
  pass. The first live `SERVER_DEFAULT/gpt-5.6-sol` Development run completed
  only `28/300` route decisions and failed unchanged recall, current-fact,
  unrelated-negative, hard-cutoff, and latency gates. It retained zero
  authority/privacy leaks but selected no policy. The independent
  `FOHWSU/deepseek-v4-flash` run then completed only `33/300` decisions and
  failed the same gate classes with zero authority/privacy leaks. No schema-v7
  policy passed; Validation/Promotion stay blocked.
- Schema-v7 intentionally remains immutable and cannot explain the DeepSeek
  profile's `263` collapsed `MEMORY_TOOL_ROUTE_FAILED` cases. Two schema-v8
  attempts consumed quota but published no artifacts; the first exposed only a
  generic integrity error and the second bounded it to `admission_state`.
  Preserve both as non-evidence. The executable
  `development_memory_tool_route_diagnostic` successor uses profile/report
  schema v9, reader v7, admission
  `development_main_model_first_tool_round_route_failure_diagnostic_only`,
  completeness `route_complete_retrieval_fail_closed_v1`, and aggregate
  artifact `memory-first-tool-round-route-diagnostic-development.json`. It
  binds `memory-tool-route-failure-taxonomy-v1` plus SHA-256
  `66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0`.
  Every failed route contributes exactly one fixed HTTP/transport/stream/
  context/Tool/provenance/recorder category; raw errors and Provider bodies
  are forbidden. Admission/rerank incompleteness is valid only with empty
  Final/Injected/token surfaces and contributes one separate normalized
  retrieval failure aggregate. This lane can never set
  `policySelected=true`.
- The first authorized live schema-v9 diagnostic published the expected
  private report/manifest and failed unchanged gates. Its reconciled current-
  run taxonomy was `31` `CONTEXT_DEADLINE`, `83` `TOOL_CALL_INVALID`, and
  `174` `ROUTER_FAILURE_UNCLASSIFIED`; retrieval completeness separately
  recorded `174` `RELEVANCE_ADMISSION_UNAVAILABLE` cases. Equal aggregate
  counts never prove per-case intersection because the artifact retains no
  identity. The result is valid diagnostic/failed-metric evidence only and
  leaves Validation/Promotion blocked.
- Candidate-blind routing stops at schema v9. The schema-v10 successor uses
  capture mode `development_configured_candidate_judge`, reader v8, profile
  config v10, report v10, admission
  `development_configured_candidate_judge_only`, adapter
  `chat-configured-candidate-judge-v1`, cost-basis v6, and artifact
  `configured-candidate-judge-development.json`.
- Schema v10 runs current-authorized candidate recall before admission. It
  reuses the shared strict query/candidate ordinal prompt and decoder, runs the
  exact configured GPT or DeepSeek judge with fixed BGE rerank, intersects
  selected ordinals with BGE order, and retains only aggregate evidence. It
  has no production composition, prompt, Usage, Validation, or promotion
  authority.
- The authorized GPT schema-v10 Development run completed `0/195` candidate-
  bearing judge decisions: `146` requests hit `HARD_CUTOFF` and `49` cases
  failed before judge egress as `RELEVANCE_ADMISSION_UNAVAILABLE`. Final
  Recall@5/current-fact was `0/0`, false injection and all authority/privacy
  leaks were zero, and p95/p99 was `1856/1862 ms`. The independent DeepSeek
  run completed `157/195` decisions with `60` valid abstentions; its `38`
  failures were `36` hard cutoffs and `2` pre-judge retrieval failures. Final
  Recall@5/current-fact was `0.558974/0.581818`, false injection and all
  authority/privacy leaks were zero, and p95/p99 was `1854/1858 ms`. Both
  exact profiles failed unchanged gates, selected no policy, and cannot enter
  Validation.
- A schema-v10 pre-judge retrieval failure is valid aggregate evidence only
  when the case had candidates but `AdmissionReady`, `RerankReady`, and
  `CloudJudgeReady` are false, the judge input-token bound is zero, and
  Provider-sent, Final, Injected, and prompt-token surfaces are empty/zero.
  Count it under `failedCaseCount` and its normalized failure code without
  incrementing `actualRequestCount`. Historical schema-v4/v5 report builders
  must continue rejecting this state rather than changing old evidence
  semantics.
- Schema v11 is a separate Development-only successor with capture mode
  `development_fixed_memory_judge`, reader v9, profile/report v11, admission
  `development_fixed_memory_judge_only`, cost-basis v7, and artifact
  `fixed-memory-judge-development.json`. It fixes the observable judge tuple
  to `SERVER_DEFAULT` / `openai_compatible` /
  `https://sub.mumubuku.top/v1` / `gpt-5.6-luna` regardless of the answer
  model. The profile hash also binds criteria v2: complete-flow p95
  `<=1500 ms`, p99 `<=2500 ms`, and hard cutoff `<=3000 ms`; every non-latency
  v1 quality, safety, token, and cost criterion remains unchanged.
- Operator code resolving the live Server Vault record must compare
  `ResolvedProvider.Type` with
  `runtimeconfig.ProviderTypeOpenAICompatible`. Its stored/runtime value is
  `OpenAI Compatible`; `openai_compatible` is the normalized capture-command
  authority token. Comparing the runtime enum's string value directly with
  the CLI token falsely rejects an otherwise valid exact Provider.
- A Luna timeout, transport failure, invalid JSON, protocol drift, or late
  result fails closed to an empty v2 final set. Normal chat continues under
  the v1 prompt/Usage authority; recalled, reranked, schema-v10, and other
  unjudged candidates are never fallback inputs. Development completion never
  starts Validation, and Validation completion never enables production.
- The retained schema-v11 result is immutable failed evidence: only `41` Luna
  requests and `22` complete rerank-plus-judge decisions were obtained, while
  `154` cases reported `RELEVANCE_ADMISSION_UNAVAILABLE` and `19` complete
  stages reported `HARD_CUTOFF`. It cannot be relabelled under later criteria
  or execution semantics.
- Schema v12 is the accuracy-first Development successor with capture mode
  `development_fixed_memory_judge_accuracy`, reader v10, profile/report v12,
  admission `development_fixed_memory_judge_accuracy_only`, criteria v3,
  cost-basis v8, policy
  `memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2`, and
  artifact `fixed-memory-judge-accuracy-development.json`. The fixed Luna,
  adapter, prompt/decoder, BGE, egress, and quality/safety/token/slice
  authorities do not change.
- Schema v12 executes `BGE query embedding -> local admission -> BGE rerank ->
  Luna judge -> Record` serially under one global Provider-request gate. It has
  no application stage/case deadline and uses HTTP clients with no elapsed
  timeout; only caller cancellation can stop a request. Criteria v3 retains
  aggregate p95/p99 timing as diagnostics but gives latency and hard-cutoff
  fields no pass/fail authority. Any `HardCutoffApplied` or `HARD_CUTOFF` trace
  is invalid schema-v12 evidence rather than a tolerated failure.
- Between all 300 cases, live mode performs `299` real one-second wall-clock
  cooldowns. Fake protocol records the same logical `299000 ms` through a
  virtual/no-op clock and must report zero elapsed cooldown time. Each BGE or
  Luna request may retry once only for `408`, `429`, `5xx`, or a retryable
  transport/read interruption. A valid `Retry-After` is authoritative;
  missing/invalid advice waits five seconds. Redirects, normal `4xx`, invalid
  JSON, schema/protocol drift, and structured remote-error payloads do not
  retry.
- Schema-v12 aggregate telemetry reconciles every passage/query/rerank/judge
  attempt and retry, per-stage p95/p99/max/total request latency, logical and
  elapsed cooldowns, and total/retry Judge input-token upper bounds. The
  cost-basis-v8 ceiling authorizes at most `600` Judge attempts and exactly
  `600 * 128 = 76800` output tokens; actual input/output authority is derived
  from attempt telemetry. Even a passing Development report emits
  `policySelected=false` and stops for owner review. It cannot enter
  Validation, production, or promotion automatically.
- The retained schema-v12 live Development result is immutable failed
  criteria-v3 evidence. It completed all `195` candidate-bearing decisions
  with zero failed cases, `203` Judge attempts including `8` retries, and all
  `299` cooldowns. Candidate Recall@20/Final Recall@5/current-fact accuracy was
  `1.0/0.974359/0.969697`, but `29` negative cases produced false injection
  `0.096667` against the unchanged `0.02` maximum, and the `stable_fact`
  current-fact slice also failed. Zero authority/privacy leaks and passing
  prompt budgets do not offset that failure. The report selected no policy and
  cannot enter Validation or be rerun under the consumed authorization.
- The repaired-v3 schema-v12 result is separate immutable failed evidence, not
  a relabel of the preceding v2 run. Configuration SHA-256
  `72940f138ba53dda01e5eddad5e82bf05e2740fd671549e2310adea61a1bf49f`
  completed all `300` cases with zero failed cases, `195` rerank attempts,
  `202` Judge attempts including `7` retries, and `299` real cooldowns.
  Candidate Recall@20/Final Recall@5/current-fact accuracy was
  `1.0/0.984615/0.981818`. False injection improved from `29/300` to `10/300`
  but still failed at `0.033333`; `stable_fact` also failed current-fact
  accuracy at `0.933333`. Report SHA-256
  `f35cfea03c98de4ecfff8ea9c774fbcef706f895da9db3a72d606e99efee2eb7`
  and manifest SHA-256
  `5be7db8903c5e26cd2dcadae12cde1a3c52f3421bb46862db481e8105e955176`
  bind this outcome. It selects no policy and grants no rerun, Validation,
  production, or promotion authority.
- The separately authorized v4 schema-v12 result is immutable failed evidence.
  Run `memory-regression-20260801t075451z-050d5f7c`, configuration SHA-256
  `c4505385b7103788c3006bf705865b2dda7c3dc5c803063d6a3bb5f09fa59d6c`,
  completed all `300` cases with Candidate Recall@20, Final Recall@5, and
  current-fact accuracy all `1.0`, `201` Judge attempts including `6` retries,
  and zero safety/authority leaks. One of 30 `unrelated_negative` cases still
  produced false injection `0.033333`, so the unchanged per-slice `0.02` gate
  failed even though aggregate false injection was `1/300`. Report SHA-256 is
  `04539bd899b22cea8cd3d17a4ee9e5b2b28adb6b10942e6be5b563eb230efc24`;
  manifest SHA-256 is
  `1904c41aff06839afdba642bf36101ccff3ef65526fe3577249b9c1f7be5d6af`.
- The separately authorized v5 schema-v12 result is also immutable failed
  evidence, not a retry or reinterpretation of v4. Run
  `memory-regression-20260801t084301z-aabb31a2`, configuration SHA-256
  `5f871f68fc0d4fed8f5822895ccc537254c843c6957362f7c8b6459ee7f6342f`,
  retained Candidate Recall@20 `1.0`, Final Recall@5 `0.907692`, current-fact
  accuracy `0.909091`, and aggregate false injection `1/300`. The
  `unrelated_negative` slice again failed at `1/30 = 0.033333`. The run also
  recorded `17` `CANDIDATE_JUDGE_FAILED` cases, `217` Judge attempts including
  `22` retries, and all `299` live cooldowns. Therefore the positive-quality
  decline cannot be attributed to the corpus from this aggregate-only bundle;
  the exact negative false-positive case and response are intentionally
  unavailable. Report SHA-256 is
  `dc4e1ca7036c5dcd5fde73d06c0404ae66539c3477493e3105590155df1923f5`;
  manifest SHA-256 is
  `43ba6e02e1b22322c56a088c5772ea769606a4acdc37d809f0fa239ca07b94e1`.
  Both v4/v5 runs select no policy and grant no automatic rerun, Validation,
  production, or promotion authority.
- Aggregate-only Development evidence authorizes metric comparison, not case-
  level or causal attribution. After disjoint v4 and v5 hard-negative families
  each retained one `unrelated_negative` false injection, do not author another
  corpus repair from a guessed case or response. A new corpus, prompt, Judge,
  or local relation gate requires separate versioning and evidence; non-zero
  Judge failures make cross-run positive-quality attribution `[unverified]`.
  Never relax the `0.02` gate to make either retained bundle pass.
- The native stdout summary schema remains the command-envelope v4, but its
  `corpusClass`, `admissionMode`, and `split` must come from the validated
  schema-v7 report rather than historical schema-v6 constants. A failed fake
  protocol report sets both `candidatePassed=false` and
  `policySelected=false`; protocol completion is not policy selection.
- Frozen validation is unavailable until the selected Development policy,
  model, prompt, decoding profile, and immutable policy ID are committed in
  code. It never recalibrates, emits only the aggregate validation report plus
  run manifest, and remains `promotionEligible=false` whether it passes or
  fails. Each live phase uses a fresh separately authorized Key.
- Cost authority is a strict versioned same-unit document. Baseline Memory
  cost is exactly zero, candidate Memory cost is positive, and both profiles
  share the same non-zero chat denominator. Missing, duplicate, unknown,
  zero-cost, unit-drift, or denominator-drift input fails before Provider work.
  Historical cloud-judge mode requires cost-basis v2; its owner absolute-cap
  follow-up requires v3. Historical schema-v6 Tool preflight requires v4;
  schema-v7 first-ToolRound Development requires v5; the configured candidate-
  judge successor requires v6 with one exact
  `configuredCandidateJudgeAuthority`. Every absolute-cap profile binds the
  exact policy ID and rejects mixed authority, request, model, token-ceiling,
  price, maximum-cost, or coverage drift before Provider construction.
  The fixed Memory Judge successor requires v7 with the same bounded request/
  token/cost shape and the exact Luna authority; Provider alias evidence does
  not claim an upstream implementation or public rate card. The accuracy-first
  successor requires v8 and expands only the Judge request/output ceilings to
  `600`/`76800` so the single allowed retry for every logical request is
  pre-authorized. Historical v6/v7 authorities remain exactly `300` requests
  and cannot be reused or rewritten.
- Cost authority has two distinct hash surfaces. An operator may bind the
  private source file's exact raw bytes with ordinary file SHA-256, while
  `DecodeCostBasis` / `CostBasisSHA256` hashes the decoded struct re-encoded by
  `encoding/json`; the run manifest uses this canonical content hash. Pretty
  whitespace can therefore change the raw file hash without changing the
  content hash. Never compare one surface with the expected value for the
  other; verify both explicitly when a live operator plan pins both.
  Schema-v9 diagnostics reuse the unchanged cost-basis v5 authority because
  they add no request, token, rate, or Provider capability; the v9 profile hash
  separately binds the failure taxonomy and completeness policy.
- A post-capture Memory Tool-route report or manifest rejection emits only a
  fixed content-free integrity reason class. It must preserve
  `ErrCaptureInvalid`, publish no partial artifact, expose no case ID/query/
  Memory/Tool/Provider body or error text, and never trigger an automatic paid
  rerun. A generic error without a bounded reason is insufficient operator
  evidence because the destroyed isolated state cannot be reconstructed.
- Native output uses a private new run directory. Full fake regression links
  four evidence files; historical calibration, schema-v4/v5 cloud Development,
  schema-v6 historical Tool-route Development, schema-v7 first-ToolRound
  Development, schema-v9 diagnostics, schema-v10 configured-candidate-judge
  Development, and frozen Validation each link one
  aggregate report. Every mode links
  `run-manifest.json` last as the completion marker. Failed metric/no-feasible
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
| Regression generator tuple is unknown, or fixture/corpus/manifest IDs, auditor, or audit time do not match that exact tuple | Reject decoding/admission; do not guess a nearest profile. |
| v2, v3, v4, and v5 fixture/corpus/audit/manifest artifacts are mixed | Reject admission and byte replay without changing any protected root. |
| v3 `unrelated_negative` lacks agenda-heading/weather-board markers or contains `unrelated`, `无关`, `no bearing`, or `没有关系` | Fail the semantic audit and refuse publication/admission. |
| A v4 query does not identify exactly one Subject, or any relevant/current/superseded Memory uses another Subject's value | Fail semantic audit/admission per Memory; do not let another correct multi-hop Memory mask the mutation. |
| A v4 `expectedNoMemory` candidate repeats the queried Subject, omits facilities/weather markers, or adds agenda/meeting/discussion/task-event claims | Fail semantic audit/admission; same entity/scope remains required but cannot establish task usefulness. |
| A v5 `expectedNoMemory` candidate lacks the mug/lounge/third-shelf observation, contains any known Subject/current/old value, or contains any v3/v4 meeting/weather/facilities event marker | Fail semantic audit/admission; do not weaken universal separation after one live v4 false positive. |
| Regression corpus/audit/fixture/manifest hash or byte replay drifts | Refuse verify/admission and preserve the existing protected root unchanged. |
| Regression observations contain a formal Holdout simulation, wrong audit/corpus/fixture binding, missing/reordered case, or bad stage subset | Reject before scoring. |
| Regression metric gate fails | Publish the new exclusive regression report, return non-zero, and keep `promotionEligible=false`. |
| Native capture DSN names a live/non-prefixed/different database or runtime lacks `role=go_api_runtime` | Reject before opening either database. |
| Fake protocol is labelled `native_v2_hybrid` or receives a credential file/egress network | Reject; fake output is protocol-only. |
| Live authorization/model target, mode-`0600` Key file, or cost authority is absent/invalid | Reject before network activity without echoing values. |
| Live mode requests `full_regression` or any mode requests machine `holdout` | Reject before Provider construction. |
| Calibration grid/objective/policy/split changes | Configuration hash changes before Provider work; the run cannot reuse indistinct authority. |
| Development trace has missing/stale admission, invalid/late rerank output, or Provider failure | Reject calibration rather than tune from incomplete scores. Fully redacted zero-egress traces may abstain. |
| No Development pair satisfies the unchanged quality/safety/cost gates | Retain aggregate failure evidence, return non-zero, keep frozen validation unavailable, and do not guess thresholds. |
| Calibration v2 diagnostics are absent, malformed, non-monotonic, wrong-length, or not configuration-hashed | Reject the retained bundle/manifest and rerun Development; never infer a dynamic policy from incomplete evidence. |
| Intent anchor version/hash, margin grid, selection objective, or cost authority drifts | Configuration hash changes; reject indistinct replay before Provider construction. |
| Intent classification is missing, malformed, late, or below the frozen margin | Record bounded intent failure/abstention, send zero Memory documents, and return `no_memory`. |
| Cloud-judge mode lacks exact target authorization, the schema-matched v2/v3 cost basis, or the exact cost-policy ID | Reject before credential read/Provider construction without echoing secrets. |
| Official judge rate is zero but the cost basis invents a non-zero judge cost, or vice versa | Reject exact rate-card arithmetic; free judge pricing never makes total candidate Memory cost zero. |
| Cloud judge model/prompt/SHA-256/decoding profile drifts | Reject the result and return `no_memory`; never accept an indistinct profile. |
| Cloud judge emits malformed/extra/duplicate/trailing JSON or invalid ordinals | Reject the entire result; never partially accept selected candidates. |
| Cloud judge or BGE rerank fails or crosses the shared cutoff | Return empty hybrid final while v1 chat remains unchanged. |
| Owner policy sees cross-user, out-of-scope, deleted, secret, superseded, Sensitive-disabled, or untrusted-source egress | Fail the zero-tolerance Provider-egress gate; only `irrelevant` is authorized. |
| Actual cloud-judge request/input/output upper bound exceeds cost authority | Reject the report and retained bundle; never infer quota after the run. |
| Schema-v5 owner absolute-cap input carries a relative-cost pass field, omits absolute authority, or reuses schema-v4 identity | Reject the report/manifest; historical cost evidence is immutable and the new policy must be explicit. |
| First-ToolRound mode lacks exact Provider ID/type/Base-URL hash/model approval or cost-basis v5 | Reject before route credential read or Provider construction. |
| SiliconFlow and Tool-route credentials are the same file, hard links, or equal bytes | Reject; do not construct either live route Provider. |
| Tool route returns no call | Record a completed abstention and empty final/injected/token surfaces. |
| Tool route returns a missing ID, unknown/duplicate call, or missing/null/malformed/non-empty arguments | Reject the route response and fail closed to `no_memory`. |
| Route Provider/model/contract/adapter authority drifts or the decision is late | Record bounded route failure, discard the decision, and keep hybrid final empty. |
| Schema-v7 emits preflight-only decoding/temperature/output/thinking fields | Reject the profile/report; first ToolRound must preserve ordinary chat-round decoding. |
| First-ToolRound stdout summary differs from the validated report admission/split or marks a failed report selected | Reject the summary implementation in tests; never relabel historical schema-v6 authority or fake-protocol completion. |
| Actual first-round request/input/output upper bound exceeds cost-basis v5 | Reject the report and bundle; never infer quota after the run. |
| Schema-v9 taxonomy/completeness version drifts, a failed route lacks one valid category, route totals differ from `failedCaseCount`, or retrieval totals do not reconcile | Reject the report/manifest; do not fall back to plaintext errors or reinterpret schema v7/v8. |
| Schema-v9 permits incomplete retrieval with non-empty Final/Injected/tokens | Reject; fail-closed retrieval may be measured but cannot release Memory. |
| Schema-v9 summary sets `policySelected=true`, unlocks Validation, or mutates the default-off runtime flag | Reject regardless of metric outcome; diagnostics have measurement authority only. |
| Configured candidate-judge mode lacks exact Provider ID/type/Base-URL hash/model approval or cost-basis v6 | Reject before the independent judge credential is read or either live Provider is constructed. |
| SiliconFlow and configured-judge credentials are the same file, hard links, or equal bytes | Reject; retrieval and configured chat Provider authorities must remain independent. |
| Configured judge output is empty | Record a valid abstention; recalled candidates remain private and final/injected/token surfaces are empty. |
| Configured judge output, adapter, Provider/model, cost, or BGE intersection authority drifts | Fail closed to `no_memory`; never inherit a schema-v4/v5 judge or schema-v6-v9 Tool-route result. |
| Schema-v10 retrieval fails before judge egress with candidates present | Aggregate one normalized failure only when rerank/judge readiness, judge token bound, Provider-sent IDs, Final/Injected, and prompt tokens prove strict `no_memory`; otherwise reject the report. Do not apply this exception to schema v4/v5. |
| Schema-v11 Provider/Base-URL/model, criteria-v2, 3000-ms cutoff, adapter/prompt/decoder, or cost-basis-v7 authority drifts | Reject before live Provider construction or report publication; never reinterpret schema v10 or relax a failed gate. |
| Luna times out, fails transport, emits invalid JSON, or returns after the shared cutoff | Return an empty v2 Memory set for that turn and continue normal chat through v1; never fall back to unjudged candidates. |
| Schema-v12 execution order, global concurrency, no-deadline/no-timeout mode, cooldown clock, retry policy, or criteria-v3 identity drifts | Reject the profile/report; preserve schema-v11 bytes and never reinterpret its failed evidence. |
| Schema-v12 receives a normal 4xx, redirect, invalid JSON/schema, or structured remote-error payload | Do not retry; fail closed for the case and continue only after the inter-case cooldown. |
| Schema-v12 receives 408/429/5xx or a retryable transport/read interruption | Honor valid `Retry-After`, otherwise wait five seconds, retry exactly once, and include both attempts in telemetry and cost authority. |
| Schema-v12 attempt counts, latency samples, cooldown totals, Judge input bounds, or `attempts * 128` output authority do not reconcile | Reject report/manifest publication; never repair aggregate evidence after the run. |
| Schema-v12 contains `HardCutoffApplied` or a `HARD_CUTOFF` trace | Reject it as execution-policy drift; criteria v3 is diagnostic-only, not permission to retain historical cutoff semantics. |
| Aggregate-only evidence shows a false injection but no case identity/response, or one run has Judge failures | Preserve the failed bundle; do not infer a causal case, mutate another corpus, relax `0.02`, or compare positive quality as if execution were stable. Require separately versioned diagnostic or policy evidence. |
| Development passes | Retain aggregate evidence and stop for owner review; never enter Validation automatically. |
| Frozen validation is requested before a Development-selected policy is committed | Reject before credential read or Provider work. |
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
- **Regression v3 good**: keep all v2 hashes fixed, generate the separately
  seeded v3 query/weather hard negative, pass exact marker/forbidden-term
  auditing, and publish only a content-free status.
- **Regression v3 base**: the private v3 bundle byte-replays and has no
  observations; it is offline authoring readiness only and cannot inherit any
  v2 run result.
- **Regression v3 bad**: edit v2 in place, combine a v3 fixture with a v2
  corpus/audit, restore self-referential negative wording, reuse v2
  observations, or invoke native capture/Validation without separate review
  and authorization.
- **Regression v4 good**: preserve v2/v3, use explicit compatible
  subject/current/old tuples, and make every no-Memory candidate deletion-
  invariant with no exact-task premise or event relationship.
- **Regression v4 base**: the separately versioned private bundle passes
  deterministic tuple/usefulness audit and byte replay. Its one fake-protocol
  run proves loading/publication/leak checks/teardown only; the fake Judge's
  expected metric failure is not live v4 quality or quota authority. Its
  separate authorized live run is immutable failed evidence because one of 30
  `unrelated_negative` cases exceeded the unchanged per-slice gate.
- **Regression v4 bad**: permute values across unrelated subjects,
  call a candidate irrelevant merely because it lacks the requested output,
  or retain `meeting about <exact queried subject>` while expecting prompt v1
  to treat the candidate as never directly useful.
- **Regression v5 good**: preserve v2/v3/v4, keep compatible positives, and use
  the same-entity/same-scope mug-location hard negative only after proving it
  contains no known Subject/current/old value or historical event family.
- **Regression v5 base**: the separately seeded private bundle passes exact
  tuple audit and byte replay at `0700/0600`; its content-free status grants no
  Provider, Validation, production, or promotion authority.
- **Regression v5 bad**: rewrite v4, reuse a v4 fixture/corpus/audit/manifest,
  admit a partial Subject/value substring, or claim the live result identifies
  a specific false-positive case when retained evidence is aggregate-only.
- **Tool-route good**: one exact configured model receives a redacted relevant
  query plus the fixed Tool, returns one exact `{}` call, and the unchanged BGE
  result becomes the offline final surface without exposing candidates to the
  route model.
- **Tool-route base**: the model returns no call for an unrelated request; BGE
  speculative work is discarded and final/injected/token surfaces stay empty.
- **Tool-route bad**: reuse one chat credential for both Provider boundaries,
  accept `null` arguments as `{}`, send candidates in the route prompt, or use a
  GPT result to authorize DeepSeek Validation.
- **Diagnostic good**: run a separately authorized schema-v9 Development lane,
  bind the route taxonomy/completeness hashes, publish route category counts
  whose sum equals all failed routes, and reconcile separate fail-closed
  retrieval aggregate counts.
- **Diagnostic base**: all routes complete, so the category map is empty and
  `policySelected=false` still holds even if retrieval is incomplete or
  unchanged quality metrics pass.
- **Diagnostic bad**: add subtype fields to schema v7, retain upstream body or
  error text, infer historical subtypes, treat either empty v8 attempt as
  evidence, or let a v9 result authorize
  Validation/Promotion.
- **Configured-judge good**: recall current-authorized candidates first, send
  only redacted query/ordinal bodies to one exact configured Provider, accept
  strict ordinals, intersect with BGE order, and publish the schema-v10
  aggregate two-file Development bundle.
- **Configured-judge base**: the strict judge returns an empty ordinal array
  for unrelated candidates; no rejected body reaches an answer prompt or
  Usage surface.
- **Configured-judge failure base**: candidate recall succeeds but admission
  becomes unavailable before judge egress; the schema-v10 report counts one
  retrieval failure, zero judge requests for that case, and empty Final/
  Injected/token surfaces while historical reports still reject the state.
- **Configured-judge bad**: route before recall, reuse a retrieval credential,
  retain candidate plaintext, accept free-form IDs, or use a GPT result to
  authorize a DeepSeek profile.

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
  protected-profile non-regression. Pin every legacy v2 raw/content hash; pin
  the new v3 raw/content hashes; assert all v3 unrelated-negative language
  variants contain required task/observation markers and none of the forbidden
  self-description terms; inject a legacy negative to prove audit failure;
  reject each v2/v3 artifact-mixing permutation; and cover content-free
  `regression-v3-generate|status|verify` output plus `0700/0600` permissions.
  Pin the v4 raw/content hashes, cover all splits/languages and all 275
  positives for explicit Subject/current/old compatibility, and prove all 50
  unrelated candidates omit the queried Subject. Mutation tests must change
  only one multi-hop relevant Memory, change only a superseded canonical value,
  and add an exact queried-task relationship to a negative; each must fail
  audit before publication/admission. Reject every v2/v3/v4 artifact-mixing
  permutation and cover content-free `regression-v4-generate|status|verify`,
  protected loading/split selection, and distinct capture configuration Hashes.
  Pin the v5 raw/content hashes, assert all 50 hard negatives retain exact
  entity/scope and mug/lounge/third-shelf markers while excluding every known
  Subject/current/old value and v3/v4 event family, and mutation-test each
  forbidden family. Reject every mixed v2/v3/v4/v5 permutation; cover
  content-free `regression-v5-generate|status|verify`, `0700/0600` publication,
  protected loading/split selection, and pairwise-distinct capture
  configuration Hashes.
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
- Relevance tests: exact Development/Validation split and holdout denial;
  `20,301`-pair grid/objective/config-hash binding; aggregate-only publication;
  schema-v3 diagnostics/anchor-version hash drift; exact `201/101/101`
  cumulative
  curve lengths and monotonic counts; eligible/missing margin handling;
  safety-first/recall-first attempts on no-feasible output; no-feasible
  manifest publication;
  fixed bilingual anchor hash; schema-v3 `201`-threshold intent grid;
  query-only/no-Memory-document classifier egress; invalid/late/low-margin
  fail-closed behavior; intent best-attempt/no-feasible publication;
  pre-rerank Provider-egress abstention; post-rerank no-memory/token
  abstention; cost-gate preservation; invalid/missing/late score rejection;
  schema-v4 exact owner-policy scoring; strict judge prompt/output/provenance;
  duplicate-key/ordinal/range/cardinality rejection; secret redaction;
  concurrent BGE/judge success/failure/cutoff; ordinal intersection; exact
  300-request and 38,400-output-token cost authority; cloud two-file manifest;
  immutable schema-v6 Tool evidence; schema-v7 Tool definition/hash,
  `chat-first-tool-round-memory-decision-v1`, and Provider/model/Base-URL
  binding; exact
  zero-call versus one-empty-object call decoding; nil/malformed/unknown/
  duplicate fail-closed cases; official OpenAI extension omission;
  historical Provider-specific thinking-control encoding; schema-v7 absence of
  preflight-only decoding fields; real `ToolRoundProvider` request shape;
  concurrent Tool/BGE completion;
  no candidate body in the Tool prompt; two distinct mode-`0600` credentials;
  cost-basis v4 historical evidence plus cost-basis v5 request/token/absolute-
  cost ceilings; schema-v7 stdout admission/split mirroring and failed-report
  `candidatePassed=false`/`policySelected=false`; first-ToolRound two-file
  manifest; schema-v9 profile/report/reader separation, exact taxonomy and
  completeness values, bounded Provider/Tool category propagation, route and
  retrieval aggregate invariants, fail-closed empty final enforcement, v7
  field omission, plaintext/raw-body leak rejection, bounded content-free
  post-capture integrity reasons with no partial publication, always-false
  `policySelected`, frozen-policy-unavailable denial; schema-v10 profile/report/
  reader/adapter separation, exact Provider and cost-basis-v6 drift denial,
  fake configured-judge construction, independent credential cleanup,
  flattened aggregate report fields, strict pre-judge retrieval-failure
  aggregation with zero request/final/token surfaces, historical schema-v4/v5
  rejection of the same state, and two-file manifest validation.
  Schema-v11 fixtures additionally cover the exact Luna tuple, criteria-v2
  values, 3000-ms capture semantics, cost-basis-v7 drift denial, independent
  credential cleanup, fixed artifact/admission identities, and the mandatory
  Development-to-Validation stop.
  Schema-v12 fixtures additionally cover serial rerank-before-judge execution,
  global concurrency one, no application/HTTP elapsed deadline, diagnostic-
  only latency output, hard-cutoff rejection, exact transient retry
  classification and `Retry-After` handling, fake virtual versus live
  wall-clock cooldown, attempt/latency/token reconciliation, cost-basis-v8
  `600`-attempt authority, historical profile-field omission, and an always-
  false `policySelected` summary.
  Cost-basis fixtures must also assert the raw private-file hash and
  the decoded canonical manifest hash as different named surfaces rather than
  assuming byte equality.
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

```text
immutable v2 bytes and failed historical evidence
-> separately seeded v3 hard-negative repair
-> real agenda task + same-entity/scope weather observation
-> deterministic v2 semantic audit plus anti-self-description checks
-> private create-only four-file bundle, regression_only
-> no Provider, Validation, or promotion authority from authoring alone
```

The native-capture wrapper keeps v2 as its compatibility default, but an
operator can select an exact verified v3, v4, or v5 bundle with
`--regression-root <protected-version-root>`. Loader admission dispatches only
from the known generator tuple, raw input hashes enter the capture
configuration, and the resulting configuration SHA-256 separates all four
versions. A live capture therefore requires new observations and fresh
authorization; historical observations can never be rebound to new hashes.

```text
immutable v2/v3 bytes and failed historical evidence
-> separately seeded v4 Subject/value and task-event repair
-> per-Memory current/old semantic audit + facilities/weather hard negative
-> private create-only four-file bundle, regression_only
-> explicit --regression-root selects v4 and creates a distinct config Hash
-> fake protocol may prove lifecycle only; no live/Validation/promotion reuse
```

```text
immutable v2/v3/v4 bytes and failed historical evidence
-> separately seeded v5 universal-negative repair
-> compatible positives + same-entity/scope physical mug-location observation
-> reject every known Subject/current/old value and v3/v4 event family
-> private create-only four-file bundle, regression_only
-> explicit --regression-root selects v5 and creates a distinct config Hash
-> one failed live Development bundle is immutable aggregate-only evidence
```

### Correct main-model first-ToolRound Development

```text
fresh SiliconFlow BGE credential + independent fresh GPT/DeepSeek credential
-> hash exact Provider/type/Base URL/model + canonical Tool + adapter + cost caps
-> one real first ToolRound request with current synthetic query/message
-> no call: no_memory
-> one exact search_memory({}) call: unchanged BGE Top 5/token selector
-> aggregate schema-v7 evidence, promotionEligible=false
-> Validation remains blocked until one exact live Development profile passes
```

This measures the user-selected model's retrieval intent without turning the
model into ownership, query-rewrite, candidate-selection, or promotion
authority.

```go
// Wrong: stale schema-v6 constant can contradict the retained v7 evidence.
summary.AdmissionMode = memorycapture.MemoryToolRouteDevelopmentAdmissionMode

// Correct: the validated report remains the command summary authority.
summary.AdmissionMode = report.AdmissionMode
summary.PolicySelected = report.Passed &&
    captureMode == memorycapture.CaptureModeMemoryToolRouteDevelopment
```

```text
Wrong: mutate schema v7/v8 or parse collapsed failure strings to invent causes.
Correct: preserve historical bytes/attempts; bind schema v9 route taxonomy and
retrieval completeness separately, with policySelected=false unconditionally.
```

```text
Wrong: compare sha256sum(private-pretty-cost.json) directly with
       memorycapture.CostBasisSHA256(decodedCost).
Correct: pin raw file bytes with the file SHA and independently pin the
         DecodeCostBasis content hash used by the manifest.
```
