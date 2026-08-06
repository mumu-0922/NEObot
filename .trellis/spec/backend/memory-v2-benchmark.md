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
neo-chat.memory-regression-profile-config.v13
neo-chat.memory-regression-profile-config.v14
neo-chat.memory-regression-profile-config.v15
neo-chat.memory-regression-profile-config.v16
neo-chat.memory-regression-profile-config.v17
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
neo-chat.memory-regression-relevance-calibration.v17
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
neo-chat.memory-regression-cost-basis.v12
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

# Production-policy Validation is a distinct schema-v15 lane. Fake protocol
# proves only the 100-case lifecycle and must return non-zero after retaining
# its aggregate-only Yellow/non-evidence bundle.
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode production_fixed_memory_judge_validation \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/production-memory-judge-validation-cost-v10.json \
  --output-dir /secure/eval/native-memory-runs

# Preferred live schema-v15 operator path. It resolves only the exact active
# Vault-backed BGE/Luna pair, materializes two new one-run mode-0600 files,
# invokes the unchanged live runner, and wipes both files on every exit.
bash scripts/run-memory-production-validation-from-vault.sh \
  --cost-basis /secure/eval/production-memory-judge-validation-cost-v10.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --production-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA

# Schema-v16 negative-guard calibration is a distinct full Development lane.
# Fake must pass first; live Vault export uses Development approvals only.
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_negative_guard \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --cost-basis /secure/eval/fixed-memory-judge-negative-guard-cost-v11.json \
  --output-dir /secure/eval/native-memory-runs

bash scripts/run-memory-negative-guard-development-from-vault.sh \
  --cost-basis /secure/eval/fixed-memory-judge-negative-guard-cost-v11.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --development-judge-approval \
    I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA

# Schema-v17 changes only the fixed Luna Judge response framing from SSE to a
# bounded non-streaming JSON completion. Schema-v16 is immutable and is not a
# rerun target. The wrapper may export the active Vault-backed pair, but must
# never build or pull the mutable admin image while doing so.
bash scripts/run-memory-buffered-judge-development-from-vault.sh \
  --cost-basis /secure/eval/fixed-memory-judge-buffered-cost-v12.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_DEVELOPMENT_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --development-judge-approval \
    I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA

# Schema-v18 is a distinct 100-case production-v2 Validation lane. Fake must
# complete first; the sole live attempt uses independent approval and v13 cost.
bash scripts/run-memory-production-buffered-validation-from-vault.sh \
  --cost-basis /secure/eval/production-buffered-validation-cost-v13.json \
  --output-dir /secure/eval/native-memory-runs \
  --credential-export-approval \
    I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval \
    I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --production-buffered-validation-approval \
    I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA

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
memorycapture.CaptureNegativePolicyGuardMemoryJudgeDevelopment(ctx, adminDB, runtimeDB, runID, fullPool, index, seed, provider, judge, authority, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(pool, profile, authority, costBasis) (memorycapture.NegativePolicyGuardMemoryJudgeDevelopmentReport, []byte, error)
memorycapture.BuildNegativePolicyGuardMemoryJudgeRunManifest(runID, captureID, providerMode, startedAt, completedAt, protected, costBasisSHA256, report, artifacts) (memorycapture.RelevanceRunManifest, []byte, error)
memorycapture.CaptureBufferedMemoryJudgeDevelopment(ctx, adminDB, runtimeDB, runID, fullPool, index, seed, provider, judge, authority, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildBufferedMemoryJudgeDevelopmentReport(pool, profile, authority, costBasis) (memorycapture.BufferedMemoryJudgeDevelopmentReport, []byte, error)
memorycapture.BuildBufferedMemoryJudgeRunManifest(runID, captureID, providerMode, startedAt, completedAt, protected, costBasisSHA256, report, artifacts) (memorycapture.RelevanceRunManifest, []byte, error)
memorycapture.CaptureProductionMemoryJudgeValidation(ctx, adminDB, runtimeDB, runID, fullPool, index, seed, provider, judge, authority, profileID, configurationSHA256, cost) (memorycapture.CapturedProfile, error)
memorycapture.BuildProductionMemoryJudgeValidationReport(pool, profile, config, authority, costBasis) (memorycapture.ProductionMemoryJudgeValidationReport, []byte, error)
memorycapture.BuildProductionMemoryJudgeValidationRunManifest(runID, captureID, providerMode, startedAt, completedAt, protected, costBasisSHA256, report, artifacts) (memorycapture.ProductionMemoryJudgeValidationRunManifest, []byte, error)
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
- Schema-v15 operational `fresh` means a new explicit one-run export approval
  plus two newly created private mode-`0600` files. The Key values may be the
  already active Vault-backed Provider values; neither report nor manifest
  attests upstream issuance time, rotation, Key hashes, or Vault envelopes.
  `mm-chat-admin memory-validation-credentials-export` has no Provider/model
  selector and may resolve only active attested `RAG:SILICONFLOW` plus the
  exact fixed `SERVER_DEFAULT`/OpenAI-Compatible/Base-URL-hash/Luna tuple.
  Existing targets, symlinks, equal paths/bytes, copied contexts, disabled or
  drifted records, and partial publication fail closed. Its paired operator
  wrapper wipes both exported files on success, metric failure, ordinary
  failure, `INT`, `TERM`, and `HUP` before returning the runner status.
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
- The separately authorized schema-v13 diagnostic
  `memory-regression-20260804t005257z-8f43c5e7` completed all `300` cases and
  reconciled `105` empty-candidate, `194` Judge-completed, and one failed
  case. Its `197` Judge attempts included two retries; the attempt map was
  `PROVIDER_STREAM_READ_FAILED: 1` and `PROVIDER_TRANSPORT_FAILED: 2`, while
  the terminal map was `PROVIDER_TRANSPORT_FAILED: 1`. Independent evaluation
  passed at Candidate Recall@20 `1.0`, Final Recall@5 `0.9948717949`, current-
  fact accuracy `0.9939393939`, false injection `0`, and zero safety leaks.
  Diagnostic schema semantics still force `passed=false`,
  `policySelected=false`, and `promotionEligible=false`. Report SHA-256 is
  `381df1eb72c29bf4a6a478731797250998cdc58482becaa44bf0b9abfef58527`;
  manifest SHA-256 is
  `cff8b7408841939e530a53aacb98f1894c2c7cf797bf4124a52f6c64f86284a3`.
  Preserve the two aggregate artifacts and do not rerun automatically.
- Schema v14 implements that transport-only successor offline under capture
  mode `development_fixed_memory_judge_transport_stable`. It keeps the exact
  prompt, BGE tuple, corpus, criteria, fail-closed result, and global Provider
  concurrency one; BGE remains at one retry while Judge permits two retries
  with exact five/ten-second fallback waits and valid `Retry-After`
  precedence. Cost-basis v9 requires authority for at most `900` Judge
  attempts and `115200` output tokens. The fake lifecycle and focused Go gates
  pass. The single separately authorized live run
  `memory-regression-20260804t022413z-cc2afbf6` then completed all `300` cases
  with zero retries and zero failures; Candidate Recall@20, Final Recall@5,
  current-fact accuracy, MRR@5, and NDCG@5 were all `1.0`, false injection was
  zero, and every safety counter was zero. Report/manifest SHA-256 values are
  `d05b991120b6878d3937f2dfdd13a899badd66e0a77f44f0f76fe8190c363ed8`
  and `5c3923aa21fc65ec3f80c963e38e642a40d8d1471d9de7272bea529202704762`.
  The one-run authority is consumed. Do not amplify BGE retries, change SSE/
  HTTP2/keepalive/corpus/threshold, or rerun automatically from this result.
- Schema v15 is the separate production-policy Validation lane, not a rename
  or widening of historical `frozen_validation` or schema v12/v13/v14. It
  seeds only the ordered 100-case `validation` split, binds profile v15,
  reader capture v13, report/run-manifest v15, cost-basis v10, the exact
  production BGE/Luna policy hash, and frozen read-intent policy
  `memory-explicit-read-intent-v1`/
  `538d9ccff34fb976cedfca0d9e153078cb3ce36f1baff0691f1d2124d182119c`.
  It preserves one BGE retry, two Judge retries with exact `5s/10s` fallback,
  and global Provider concurrency one. A terminal case records fail-closed
  evidence and the batch continues; any terminal case makes the final report
  fail. Fake protocol is permanently `fake_protocol_lifecycle_only`, Yellow,
  `retain_beta`, and non-passing. Live execution needs newly materialized
  distinct one-run BGE/Luna files plus the independent exact export and
  Validation approvals. Documentation alone is never run authorization.
- Schema-v15 retained evidence is exactly one aggregate report plus its run
  manifest. It may contain hashes, metric/slice counts, bounded latency/token/
  cost totals, and typed category totals, but no query, Memory plaintext,
  Provider response/error, raw score, or case-level identity. Failure action
  precedence is frozen: privacy/authorization is Red/disable Tool Loop; false
  injection above `0.02` is Orange/disable recall while preserving data;
  Provider stability or remaining quality failure is Yellow/retain Beta; a
  passing live result stops at owner review with `releaseEligible=false` and
  changes no runtime flag.
- The single owner-authorized schema-v15 live run
  `memory-regression-20260806t013956z-31e67617` completed all 100 ordered
  Validation cases with zero terminal case failure and valid aggregate-only
  artifacts. Candidate Recall@20 was `1.0`, Final Recall@5 was `0.984615`,
  current-fact accuracy was `0.981818`, and every cross-user/deleted/Secret/
  untrusted-source/unauthorized-egress safety counter was zero. However, nine
  false-injection cases produced rate `0.09`, above the frozen `0.02` gate;
  the immutable result is Orange with required action
  `disable_memory_recall_preserve_data`, `passed=false`, and
  `releaseEligible=false`. The report/run-manifest SHA-256 values are
  `6b2ec1a0cf26b2190302accac384f9fab4fce0898d1b1bad1eaacb5a2ce39c69`
  and `3ee114b2991ad2d0de954ad4a5998947567c66672e010dc079f17c73c18ae650`.
  No runtime flag was changed automatically; do not rerun, promote, or start
  Holdout from this consumed authorization.
- The retained schema-v15 report cannot identify the exact nine failed cases:
  its nine failure entries are aggregate criterion/slice messages, not case
  IDs. All nine are contained in the 10-case `unrelated_negative` Validation
  slice, but naming any exact nine is unsupported. The separately versioned
  Development policy
  `memory_hybrid_fixed_cloud_candidate_judge_negative_guard_development_v1`
  adds only the hash-bound bilingual guard
  `memory-negative-policy-query-guard-v1`/
  `8fe79b55a0f136392081a81e471abae98d0db7b8e3bece74adcc590b9d2c8f39`.
  After authorized Prepare and before candidate admission/rerank/Judge, a
  match records an empty final set with
  `NEGATIVE_POLICY_QUERY_ABSTAINED`. Query-only BGE embedding may already have
  completed, but no candidate plaintext may cross a Provider boundary. A
  provider-free audit of only the already-consumed Validation split matched
  `10/10` `unrelated_negative`, `16/45` expected-no-Memory, and `0/55`
  relevant cases. Ordered case-set hashes are
  `1e8aa17ce6f8426ce9c91d3be7ffeef34be2bb8b14d0eaa9a8616b5426f0bc6f`
  and `a3c322d299a24c3443b92e9e7136b53bed8fd17e1d0a9bd71815937e41ba76c2`.
  This provider-free diagnostic by itself changes neither production-v1 nor
  the consumed Validation result and authorizes no live run, Holdout,
  promotion, Release, or recall re-enable.
- Schema v16 is the separately versioned full-Development capture lane for
  that guard: reader capture v14, profile/report v16, relevance-run manifest
  v1, cost-basis v11, and artifact
  `fixed-memory-judge-negative-guard-development.json`. It reuses schema-v14
  serialization, BGE retry, Judge two-retry, typed failure, cooldown, and
  criteria-v3 authorities while adding only exact guard/policy provenance.
  Its PostgreSQL 17 Fake lifecycle completed `105` empty-candidate, `30`
  guard-abstained, and `165` Judge-completed cases with zero network and zero
  scoped residue. The consumed live run
  `memory-regression-20260806t064355z-65407a6a` completed all 300 cases as
  `105/30/162/3` empty/guard/Judge-completed/failed. False injection fell to
  zero and every safety counter stayed zero, but five Judge abstentions plus
  three terminal `PROVIDER_TRANSPORT_FAILED` cases left Final Recall@5 at
  `0.958974` and current-fact accuracy/MRR/NDCG at `0.951515`; the
  `preference_instruction` and `stable_fact` current-fact slice gates failed.
  Report/manifest SHA-256 values are
  `895a8f524177645a159b6e1e15bfe8c4d828813ff4472dad4cdbe442d1b73929`
  and `495ec7b4a19021f600db0f2826dc2875cabfbd3f8bd51fe0ce5e94d10ce65a43`.
  The result is immutable failed, non-selecting, non-promotional Development
  evidence and grants no rerun or later-stage authority.
- Schema v17 is the transport-only successor to schema v16. Its identity is
  reader capture v15, profile/report v17, cost-basis v12, capture mode
  `development_fixed_memory_judge_negative_guard_buffered`, admission mode
  `development_fixed_memory_judge_negative_guard_buffered_only`, artifact
  `fixed-memory-judge-negative-guard-buffered-development.json`, adapter
  `chat-configured-candidate-judge-buffered-v1`, and execution sequence
  `bge_query_admission_bge_rerank_luna_judge_buffered_json_record_serial_judge_retry_v1`.
  It changes only Luna response framing: the request remains wire-equivalent
  except `stream:false` plus `Accept: application/json`, while the Provider
  owns a 2 MiB body cap, exactly one choice, present content, and exact
  `finish_reason == "stop"` validation. Prompt, decoder, model, no-thinking,
  temperature zero, 128-token ceiling, BGE, guard, criteria, cooldown, and the
  two-Judge-retry `5s/10s` policy remain unchanged. Read interruption without
  context termination is typed `PROVIDER_TRANSPORT_FAILED`; malformed,
  oversized, incomplete, or multi-choice JSON is
  `PROVIDER_RESPONSE_INVALID`; cancellation/deadline keeps its context
  category. Provider bodies and private error strings are never persisted.
- The consumed schema-v17 Fake run completed `105` empty-candidate, `30` guard,
  and `165` Judge cases with zero network/residue. The single authorized live
  run `memory-regression-20260806t082407z-1ce1eba8` completed all 300 cases:
  `174` Judge attempts, nine recovered
  `PROVIDER_TRANSPORT_FAILED` attempts, zero terminal failures, three valid
  Judge abstentions, Candidate Recall@20 `1.0`, Final Recall@5
  `0.9846153846`, current-fact accuracy `0.9818181818`, and false injection
  `0/135`. It passed every slice/safety/cost gate but remains
  `policySelected=false`, `promotionEligible=false`, and grants no rerun,
  Validation, recall activation, Release, deployment, or production-policy
  authority. Its configuration/report/manifest SHA-256 values are
  `83d61297ac9e0dd07a457af947642a6fb88505e2b70b701bc9e0681dd29e7359`,
  `d0a70c03eda7fbb1bee4107c057acc54870da56cb2041ebdb9fa4cac8955a6ce`,
  and `182bbcc4cf553f9e7eb893abbd0122e9536dca970d3b232c5c7f832b703bdf2a`.
  The v12 raw-file and decoded-content cost hashes are respectively
  `a8e339b0aff182773b886681ad125eb5dcc6205d705cf325309c698da9b44d6a`
  and `339d419caa56ba7414ec993b2d059f004279315de65e05479b603536cbeb17f4`.
  Pre/post hashes over 43 live Memory relations remained identical at
  the recorded schema-v17 boundary.
- Schema v18 is the separately versioned production-v2 buffered Validation:
  reader capture v16, profile/report v18, Validation run manifest v18,
  cost-basis v13, policy
  `memory_hybrid_fixed_cloud_candidate_judge_negative_guard_production_v2`,
  and adapter `chat-configured-candidate-judge-buffered-v1`. It seeds only the
  frozen 100-case Validation split. Fake completed `35/10/55/0`
  empty/guard/Judge/failed with zero network/residue. The sole complete live
  run `memory-regression-20260806t101512z-a057b161` also completed
  `35/10/55/0`, with `58` attempts, three recovered retries, zero terminal
  failures, Recall@20/Final Recall@5/current-fact accuracy
  `1.0/0.984615/0.981818`, false injection `0/45`, and all safety/cost gates
  passing. `mixed_language_entity` and `stable_fact` each had only `0.9`
  current-fact accuracy and failed their slice criteria. The Yellow
  `retain_beta` report is final; no UUID canary, rollout, rerun, Holdout,
  promotion, or Release is authorized.
  `d027b35dd8b667f21c84b2a38cd0b27fec94b684c0d4561c8677bb3b9885142b`.
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
  and cannot be reused or rewritten. Schema v14 requires v9 with at most `900`
  Judge attempts/`115200` output tokens. Schema-v15 Validation requires v10:
  exactly `300` maximum Judge attempts and `38400` output tokens for only the
  100-case split. Schema-v16 negative-guard Development requires v11 and the
  v9-equivalent `900`/`1500000`/`115200` request/input/output ceilings. V9
  cannot authorize schema v16, v10 cannot reinterpret a Development run, and
  v11 cannot authorize Validation. Schema-v17 requires a distinct v12 document
  with the same `900`/`1500000`/`115200` ceilings; v11 cannot authorize v17,
  and v12 cannot reinterpret schema-v16 or Validation evidence.
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
  Development, historical frozen Validation, schema-v15 production-policy
  Validation, schema-v16 negative-guard Development, and schema-v17 buffered
  Judge Development each link one aggregate report. Every mode links
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
| The Development negative-policy guard matches after Prepare | Record `NEGATIVE_POLICY_QUERY_ABSTAINED` with empty rerank/final/token surfaces; skip admission, candidate rerank, and Judge egress. Query-only BGE embedding before Prepare is allowed. |
| The Development guard/policy identity or descriptor provenance drifts, or the Development identity is installed as the product Tool policy | Reject before Provider work. Production-v1 descriptor bytes/hash remain immutable; only the separate production-v2 identity may carry the guard, and its failed Validation leaves runtime gates off. |
| Schema-v16 mode selects Validation/Holdout, accepts a non-v16 profile/reader/report/cost identity, or omits exact guard/descriptor provenance | Reject before Provider construction or report publication; never reinterpret v9/v10/v14/v15 evidence. |
| Schema-v16 has zero guard abstentions, a guard trace does not end as completed `NO_CANDIDATES`, or it retains admission/rerank/Judge attempts or input tokens, Provider-sent/final/injected IDs, or prompt Memory tokens | Reject the report as inconsistent even when the aggregate quality metrics would otherwise pass. |
| Schema-v16 Fake lifecycle has network/credentials, leaves scoped Compose state, or is presented as live quality evidence | Fail the lifecycle; Fake proves wiring and cleanup only. |
| Schema-v17 request differs from schema-v16 beyond `stream:false`/JSON response framing, or prompt/decoder/model/BGE/guard/criteria/retry changes | Reject the lane as confounded; preserve schema-v16 and create no evidence. |
| Schema-v17 JSON body is malformed, over 2 MiB, has other than one choice, lacks content, or has non-exact `finish_reason != "stop"` | Return bounded `PROVIDER_RESPONSE_INVALID`; retry only under the unchanged typed Judge retry policy and persist no body. |
| Schema-v17 successful-status body read is interrupted without context termination | Return `PROVIDER_TRANSPORT_FAILED`; if context ended, preserve the exact cancellation/deadline category. Never leak the private read error. |
| The Vault wrapper builds or pulls `admin`, or uses a mutable local tag as evidence authority | Reject before credential export. Use Compose `run --no-build --pull never`; do not move the active backend tag or restart production services. |
| The consumed schema-v17 live run is requested again, or its pass is used to enter Validation/enable recall/promote/deploy | Refuse. The one-shot Development evidence is complete and remains non-promotional. |
| Schema-v18 Fake is presented as quality or live authority | Reject; retain the two aggregate files and non-zero exit only as lifecycle evidence. |
| Schema-v18 aggregate metrics/safety pass but any required slice fails | Retain Yellow `retain_beta`, keep the global Tool flag false and allowlist empty, and never rerun or partially authorize. |
| Compose v5 lacks `run --no-build` | Capability-detect before credentials, require `--pull never`, omit positive `--build`, and pin the exact reviewed export image. |
| Development passes | Retain aggregate evidence and stop for owner review; never enter Validation automatically. |
| Frozen validation is requested before a Development-selected policy is committed | Reject before credential read or Provider work. |
| Schema-v15 mode selects Development/Holdout, seeds other fixtures, or changes the frozen case order/read-intent/policy/criteria hash | Reject before report publication; historical schemas and the visible machine Holdout remain untouched. |
| Schema-v15 live mode lacks its exact independent Validation approval, carries only the old Development approval, or BGE/Luna credentials are same-file/hard-linked/equal-byte | Reject before output or Provider construction without echoing credentials. |
| Exact-pair export approval is absent/wrong, an arbitrary Provider selector is supplied, or active RAG/Luna authority is missing/disabled/unattested/drifted/copied | Return only `MEMORY_VALIDATION_CREDENTIAL_EXPORT_NOT_AUTHORIZED` or `MEMORY_VALIDATION_CREDENTIAL_AUTHORITY_UNAVAILABLE`; create no output and make no Provider request. |
| Exact-pair export target exists/is symlinked/is not under a private direct parent, paths or bytes are equal, publication is partial, or cleanup fails | Never overwrite an existing target; wipe/remove invocation-created files and return only `MEMORY_VALIDATION_CREDENTIAL_OUTPUT_REJECTED` or `MEMORY_VALIDATION_CREDENTIAL_CLEANUP_FAILED`. |
| Schema-v15 Fake report claims pass/Release, omits Yellow `retain_beta`, or is treated as quality evidence | Reject the bundle; Fake remains `FAKE_PROTOCOL_NON_EVIDENCE` and the wrapper returns non-zero after valid retention. |
| Schema-v15 terminal Provider failure occurs | Record one typed aggregate terminal, release no Memory for that case, continue later ordered cases, and force the final Validation to fail without whole-run retry. |
| Schema-v15 report/manifest contains query, Memory plaintext, Provider response/error, raw score, or case-level identity | Reject and remove the bundle; aggregate slice `cases` counts are allowed but case arrays/IDs are not. |
| Schema-v15 live metrics show privacy/authorization, false injection above `0.02`, Provider/quality failure, or pass | Emit respectively Red/disable Tool Loop, Orange/disable recall preserving data, Yellow/retain Beta, or Pass/owner review; never auto-release or mutate flags. |
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
- **Production Validation good**: a separately authorized live schema-v15 run
  uses newly materialized distinct one-run BGE/Luna files, exact fixed
  hashes/tuple, all 100 ordered Validation cases, reconciled attempts/costs,
  zero terminal/privacy/injection failure, and stops at owner review without
  Release. Upstream Key reissuance is not claimed.
- **Production Validation base**: Fake PostgreSQL 17 completes the same 100-
  case lifecycle, publishes exactly two private aggregate artifacts, returns
  non-zero, and records Yellow/`retain_beta`/
  `FAKE_PROTOCOL_NON_EVIDENCE`; this proves engineering only.
- **Production Validation bad**: reuse v9 Development authority, use only the
  old configured-judge approval, share credential bytes, seed Holdout, stop at
  the first terminal case, persist a query/case ID/raw score, accept Fake as a
  pass, or let a live pass flip a product flag.
- **Production-v2 Validation good**: all v18 case, slice, safety, attempt,
  token, cost, cleanup, source, and runtime gates pass before one exact UUID is
  separately admitted on the reviewed image.
- **Production-v2 Validation base**: Fake completes all 100 cases with a
  positive guard count, returns non-zero, and leaves only two private aggregate
  artifacts.
- **Production-v2 Validation bad**: overall accuracy passes but a required
  slice fails, yet the consumed result is rerun or used to populate the canary.
- **Negative-guard Development base**: authorized Prepare returns candidates,
  the bilingual guard matches, Record completes with an empty final set and
  `NEGATIVE_POLICY_QUERY_ABSTAINED`, and admission/rerank/Judge call counts are
  zero. One earlier query-only embedding is not candidate plaintext egress.
- **Negative-guard Development bad**: mutate production-v1, change the Judge
  prompt or thresholds in the same cycle, infer the exact nine failed case
  IDs, inspect Holdout, or treat the provider-free audit as promotion evidence.
- **Schema-v16 good**: Fake completes first with zero network; live Development
  binds cost v11 and the exact Vault-backed BGE/Luna tuple, counts all 300 cases
  across empty/guard/Judge/failed outcomes, reconciles attempts/tokens/cost,
  retains two private aggregate files, and remains non-promotional.
- **Schema-v16 base**: the guard removes false injection but Provider terminals
  or quality slices still fail. Preserve the aggregate bundle, destroy the
  one-run credentials/cost source, keep both Memory flags false, and stop
  without rerun or Validation.
- **Schema-v16 bad**: call a failed full live result a guard pass, retry it from
  broad quota consent, reuse v9/v10, omit pre/post live-state comparison, or
  promote from zero false injection while stability/quality gates failed.
- **Schema-v17 good**: Fake lifecycle passes, then one authorized live run uses
  only the Provider-owned bounded JSON response path, reconciles 300 outcomes,
  174 attempts, nine recovered transport failures, zero terminals, tokens and
  v12 cost, retains two mode-`0600` aggregate files, and leaves Memory state and
  flags unchanged.
- **Schema-v17 base**: the Provider returns one complete JSON choice with exact
  `finish_reason=stop`; the existing strict Judge decoder and unchanged
  selection policy decide the result. A valid empty ordinal array remains a
  Judge abstention rather than a transport failure.
- **Schema-v17 bad**: duplicate the decoder in `memorycapture`, accept SSE
  fragments in the buffered path, loosen finish semantics/body bounds, rebuild
  the Vault export image, rerun the consumed lane, or treat its pass as
  Validation/Release authority.
- **Exact-pair export good**: resolve only active attested `RAG:SILICONFLOW`
  and the exact fixed Luna tuple, create two exclusive private mode-`0600`
  files, run schema-v15, and wipe both source copies on every exit.
- **Exact-pair export base**: an output already exists or Luna authority has
  drifted; no file is overwritten, any invocation-created first file is wiped,
  and only a fixed content-free error category is returned.
- **Exact-pair export bad**: add a Provider/context selector, export through a
  browser/API response, log a Key/hash/envelope, widen directory permissions,
  or claim that a new one-run file proves upstream Key issuance.

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
- Exact-pair operator tests: exact approval and fixed selection, disabled/
  missing/unattested/drifted/copied authority, existing/symlink/same/equal
  outputs, exclusive mode-`0600` publication, partial cleanup, and secret-free
  stdout/stderr. Wrapper tests must prove success, metric failure, ordinary
  failure, `INT`, `TERM`, and `HUP` all remove the exported pair before exit.
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
  Schema-v13/v14 fixtures additionally cover typed attempt/terminal category
  reconciliation, diagnostic non-selection, Judge-only two-retry recovery and
  exhaustion, exact `5s/10s` fallback/`Retry-After`, BGE one-retry preservation,
  zero-terminal pass semantics, and cost-basis-v9 `900`-attempt authority.
  Schema-v15 fixtures additionally cover exact 100-case selection and Holdout
  denial; profile/reader/report/manifest/hash identity; frozen read-intent
  hash; cost-basis-v10 `300`-attempt authority isolated from v9; independent
  approval and credential rejection; fake Yellow/non-evidence; live action
  precedence; terminal-failure continuation and final failure; aggregate-only
  deterministic replay; two-file `0700/0600` publication; PostgreSQL 17
  `go_api_runtime`; and cleanup on success, failed metrics, leak rejection, and
  signals without any real Provider request.
  Schema-v16 fixtures additionally cover exact Development-only selection;
  profile v16/reader v14/report v16/cost v11 identities; immutable guard,
  policy-descriptor, and historical JSON hashes; exact
  `NEGATIVE_POLICY_QUERY_ABSTAINED` aggregation with local candidates but zero
  admission/rerank/Judge/Provider-sent/final/token surfaces; other pre-
  admission code failure; Judge attempt/terminal/token/cost reconciliation;
  the Development-only Vault export approval; deterministic Fake two-file
  lifecycle; and success/failure/signal credential plus Compose cleanup.
  Schema-v17 fixtures additionally cover streaming/buffered request wire
  equivalence except transport fields; exact status, cancellation, body-read,
  malformed, oversized, missing-content, multi-choice, and non-`stop` finish
  classification; adapter prompt/decoder/provenance equality; profile v17,
  reader v15, report v17, cost v12, capture/admission/artifact/sequence
  separation; historical v14/v16 identity preservation; Fake PostgreSQL 17
  replay; aggregate privacy/equation validation; and Vault cleanup with an
  assertion that Compose receives `--no-build --pull never`.
  Schema-v18 fixtures additionally cover profile/reader/report/run/cost/policy/
  adapter isolation, exact `empty + guard + Judge-completed + failed = 100`,
  failed-slice no-rollout semantics, exact UUID fail-closed admission, both
  Compose-run capability branches, and cleanup of credentials/export state.
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

```text
Wrong: add a general Provider-secret export or claim a reused Key was newly
       issued; reuse schema-v14 approval, seed all 500 cases, or call Fake pass.
Correct: exact-pair Vault resolution -> new private one-run mode-0600 files ->
         schema-v15 + cost-basis v10 + exact approvals -> 100 Validation cases
         -> aggregate report/manifest -> wipe copies -> owner review only.
```

```text
Wrong: guard removed false injection, so ignore three terminal Judge failures,
       call the failed schema-v16 report a pass, and rerun Validation.
Correct: mandatory Fake -> exact Development-only Vault export -> schema-v16 +
         cost-basis v11 -> reconcile all 300 outcomes/attempts/tokens/cost ->
         retain failed aggregate evidence -> wipe credentials/cost source ->
         verify live flags and 43 Memory relation counts unchanged -> stop.
```

```text
Wrong: parse non-streaming Luna JSON inside memorycapture, accept any finish
       reason, rebuild admin during Vault export, or rerun schema-v16/v17 after
       the first complete live result.
Correct: internal/chat bounded completion -> exact one-choice/content/stop
         validation -> unchanged strict Memory Judge decoder -> schema-v17 +
         cost-basis v12 aggregate evidence -> no-build/no-pull credential
         export -> verify 43-relation hash and flags unchanged -> stop without
         Validation, recall activation, promotion, deployment, or rerun.
```

```text
Wrong: schema-v18 overall metrics and safety look good, so ignore two failed
       slices, choose the current account UUID, and recreate backend.
Correct: retain the one complete aggregate pair -> verify attempts/tokens/cost
         and cleanup -> keep MEMORY_TOOL_LOOP_ENABLED=false and the canary
         empty -> record no-rollout -> never rerun the consumed live attempt.
```
