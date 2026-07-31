# Native Memory regression capture

`memorycapture` executes either the protected 500-case machine regression
corpus or its exact 300-case Development / 100-case Validation views through
Neo Chat's production v1 lexical and v2 hybrid reader seams. It turns transient
ranking surfaces into strict, non-promotional full or aggregate artifacts
without changing prompt, Usage, feature flags, or production data.

## Responsibilities

- replay and byte-verify the fixed regression fixture/corpus/audit/manifest;
- map opaque fixture identities to deterministic ephemeral UUIDs;
- seed a fresh, marked PostgreSQL 17 database and build BGE-M3 projections;
- call the production `usermemory` v1 and hybrid readers;
- decorate repository and Provider seams to capture RRF, final, and Provider-
  sent Memory IDs before production diagnostics discard them;
- capture request-local admission/rerank scores only for historical
  Development grid simulation, then discard them before aggregate publication;
- run the owner-authorized strict cloud candidate judge concurrently with BGE
  rerank for schema-v4/v5 Development, while retaining only aggregate status,
  bounded cost authority, and shared evaluator metrics;
- preserve schema-v6 as immutable failed `PlanTools` evidence and run the exact
  current-model first-`ToolRoundProvider` `search_memory` decision concurrently
  with fixed BGE work for schema-v7 Development without sending candidate
  bodies to the route Provider;
- close every started route before the sequential capture case finishes, bind
  Recorder writes to that case generation, and prevent cancellation-ignoring
  delegated routers from holding the reader;
- stop candidate-blind routing at schema v9 and run the schema-v10
  configured-main-model candidate judge only after private current-authorized
  recall, reusing the strict ordinal/BGE intersection path without production
  injection authority;
- run the schema-v11 fixed Luna successor under its own profile/report/reader/
  criteria/cost identities, with a 3000-ms complete-flow cutoff and no
  unjudged-candidate fallback;
- run the schema-v12 accuracy-first successor through one globally serialized
  BGE/Luna request controller with no application/HTTP elapsed deadline,
  diagnostic-only latency, bounded transient retry, and an inter-case cooldown;
- enforce exact `development`/`validation` split lanes and reject the visible
  machine `holdout`;
- assemble strict regression observations, content-free run manifests, and
  exclusive private output bundles;
- separate the zero-network `fake_protocol` profile from live SiliconFlow
  reader-quality evidence.

The package has no Golden admission, Holdout, profile-promotion, prompt-
injection, or active-reader authority.

## Main entrypoints

| API | Purpose |
| --- | --- |
| `LoadProtectedRegression` | Regenerate and byte-verify all protected inputs. |
| `BuildFixtureIndex` | Create deterministic alias/UUID authority maps. |
| `SeedEphemeralDatabase` | Materialize synthetic users, scopes, messages, Memories, and lexical projections. |
| `PopulateProjectionVectors` | Populate fixed 1024-dimensional BGE projection vectors through the Provider interface. |
| `CaptureProfiles` | Execute v1 and v2 against the same reset synthetic state. |
| `CaptureDevelopmentCalibration` | Execute only the 300 Development cases under the all-score calibration policy. |
| `BuildDevelopmentCalibration` | Evaluate 20,301 scalar pairs plus 201 query-intent margins and return schema-v3 aggregate evidence. |
| `CaptureCloudJudgeDevelopment` | Execute the fixed strict-ordinal cloud judge and BGE rerank concurrently for the 300 Development cases. |
| `BuildCloudJudgeDevelopmentReport` | Apply version-matched Provider-egress/cost policy and return schema-v4/v5 aggregate evidence. |
| `CaptureMemoryToolRouteDevelopment` | Execute one exact GPT/DeepSeek first-ToolRound profile and fixed BGE path for the 300 Development cases. |
| `BuildMemoryToolRouteDevelopmentReport` | Apply cost-basis v5 and return schema-v7 aggregate first-ToolRound evidence. |
| `CaptureMemoryToolRouteDiagnostic` | Execute the schema-v9 request-equivalent route diagnostic with request-local bounded failure categories. |
| `BuildMemoryToolRouteDiagnosticReport` | Return aggregate-only schema-v9 route and retrieval-completeness counts; never select a policy. |
| `CaptureConfiguredCandidateJudgeDevelopment` | Execute candidate-first strict judging through one exact configured GPT/DeepSeek model on Development. |
| `BuildConfiguredCandidateJudgeDevelopmentReport` | Return schema-v10 aggregate evidence bound to Provider ID/type/Base-URL hash/model, adapter, and cost-basis v6. |
| `CaptureFixedMemoryJudgeDevelopment` | Execute the exact global `SERVER_DEFAULT/gpt-5.6-luna` candidate-aware policy on Development. |
| `BuildFixedMemoryJudgeDevelopmentReport` | Return schema-v11 aggregate evidence under criteria v2 and cost-basis v7. |
| `CaptureAccuracyFirstMemoryJudgeDevelopment` | Execute query embedding, admission, BGE rerank, fixed Luna judge, and Record serially on Development. |
| `BuildAccuracyFirstMemoryJudgeDevelopmentReport` | Return schema-v12 quality/safety/token evidence plus diagnostic latency and reconciled attempt/cost telemetry under criteria v3/cost-basis v8. |
| `CaptureFrozenValidation` | Execute only the 100 Validation cases under the code-frozen policy. |
| `BuildFrozenValidation` | Score the frozen Validation result without retuning. |
| `AssembleRegressionObservations` | Bind ordered captures to the strict regression schema. |
| `BuildRunManifest` | Create a content-free, explicitly non-promotional run record. |
| `PublishArtifactsExclusive` | Publish a private bundle without overwriting evidence. |

## Operator entrypoint

Use the isolated wrapper from the product root rather than invoking this
package directly:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --capture-mode full_regression \
  --cost-basis /secure/memory-regression-cost-basis.json \
  --output-dir /secure/memory-regression-runs
```

`fake_protocol` validates SQL, capture, evaluation, publication, and teardown
only. Live mode accepts `development_calibration`,
`development_cloud_judge`, `development_memory_tool_route`, or
`development_configured_candidate_judge`,
`development_fixed_memory_judge`,
`development_fixed_memory_judge_accuracy`, or `frozen_validation`. Each phase
requires a fresh separately authorized
mode-`0600` SiliconFlow key file. Tool-route and configured/fixed/accuracy-first
judge Development additionally require a different fresh mode-`0600` chat
Provider credential. Live output is labelled
`native_v2_hybrid` while fake output is labelled
`native_v2_hybrid_fake_protocol`. Frozen Validation is rejected before Key
reading while no Development-selected policy is committed.

The first live fixed-grid Development run completed `20,301` pairs with a
passing `0.033084` Provider cost ratio but no feasible pair. Validation remains
blocked. The schema-v2 Development-only rerun retained aggregate failure,
best-attempt, and cumulative admission/max-rerank/top-two-margin diagnostics;
it was not a threshold guess or a Validation run.

The schema-v2 rerun proved scalar admission, max rerank score, and candidate
margin infeasible. The completed schema-v3 query-only bilingual intent run also
found no feasible threshold: zero unrelated egress retained only `31/165`
relevant current-fact cases, while full recall produced `26` false injections
and unauthorized egress events.

The owner then explicitly authorized ordinary current-user Memory candidates
for this single-user Server-mode deployment to reach the configured cloud
Provider. Schema v4 therefore evaluates `development_cloud_judge` with the
fixed `Qwen/Qwen3-8B` model by default, strict ordinal-only JSON, deterministic
secret redaction, and the exact
`owner_authorized_normal_candidates_v1` egress policy. Cross-user,
out-of-scope, deleted, secret, superseded, Sensitive-disabled, and
untrusted-source candidates remain forbidden. False injection is unchanged.
The first fresh schema-v4 live Development run completed but did not select a
policy. Candidate Recall@20 stayed at `1.0`, authority leakage remained zero,
and the cost gate passed, but Final Recall@5 was `0.758974`, current-fact
accuracy was `0.751515`, false injection was `14/300`, and `31/195` judge
requests failed closed at the stage cutoff. Validation remains blocked,
promotion remains false, and the run's source Key was destroyed.

The next precommitted hypothesis was
`deepseek-ai/DeepSeek-V4-Flash`. Because a truthful paid-model ceiling cannot
pass the historical 15% relative-cost criterion, schema v5 uses the explicit
`owner_authorized_absolute_cap_v1` product policy requested by the owner.
Cost-basis v3 and profile/report/run-manifest identity bind exact prices,
request/token ceilings, and maximum absolute Memory Provider cost. The
historical ratio remains visible but has no `providerCostPassed` field under
schema v5. Every quality, safety, latency, cutoff, token, split, privacy, and
promotion gate is unchanged.

The DeepSeek schema-v5 live run then failed the unchanged production boundary:
`164/195` judge requests hit `HARD_CUTOFF`, Final Recall@5/current-fact
accuracy was `0.143590/0.145455`, and p95/p99 was `1856/1865 ms`. It produced
zero false injections and zero authority/privacy leaks, but selected no policy.
The next named Development model hypothesis was `Qwen/Qwen3.6-35B-A3B`; it
used a new cost basis and a fresh unexposed Key.

That Qwen3.6 run also failed: Final Recall@5/current-fact was
`0.733333/0.733333`, false injection was `15/300`, p95/p99 was `1854/1856 ms`,
and `40/195` judge requests hit `HARD_CUTOFF`. The planned
`Qwen/Qwen3.5-4B` follow-up was cancelled before Provider construction or quota
use as `cancelled_not_run_architecture_pivot`; it has no model result.

Schema v6 implemented the first architecture pivot as an independent
`PlanTools` preflight. Its profile-v6/cost-basis-v4 artifacts are immutable
historical evidence. GPT completed only `41/300` route decisions, the first
DeepSeek run is protocol-invalid, and corrected DeepSeek Flash completed
`77/300`; no profile passed. The separate preflight request shape is rejected.

The current `development_memory_tool_route` implementation emits schema v7. It
uses reader `neo-chat.native-memory-reader-capture.v5`, profile config v7,
policy `memory_hybrid_main_model_first_tool_round_calibration_v1`, adapter
`chat-first-tool-round-memory-decision-v1`, cost-basis v5, and artifact
`memory-first-tool-round-development.json`. `internal/chat` owns the canonical
Tool definition/hash/validation; the `memoryroute` adapter submits one real
first `ProviderRoundRequest` with the current synthetic query/message,
`tool_choice=auto`, and no continuation. It does not call `PlanTools` or force
the old temperature/output/thinking controls.

No call means `no_memory`; use Memory requires one call with a non-empty ID and
explicit `{}`. Missing/null/non-empty arguments, unknown/duplicate calls,
invalid Provider events, timeout, failure, and model/contract drift fail closed.
BGE work may overlap, but candidate bodies never reach the route model and final
rows are released only after a valid call. Schema-v7 offline protocol/report/
lifecycle tests pass. The first live `SERVER_DEFAULT/gpt-5.6-sol` profile
completed only `28/300` routes and failed unchanged quality, cutoff, and latency
gates. The independent `FOHWSU/deepseek-v4-flash` profile completed only
`33/300` routes and failed the same gate classes. Both retained zero
authority/privacy leaks; no policy was frozen and Validation remains blocked.

Schema v7 cannot retroactively split its collapsed route failures. Two
separately authorized schema-v8 attempts consumed quota but published no
artifact: the first returned the legacy generic post-capture integrity error;
the second returned bounded `admission_state`. The latter proves at least one
non-empty candidate case had incomplete admission, but its BGE/response/SQL
subcause remains `[unverified]`. Both isolated runtimes and all transient
credentials/helpers were destroyed.

The current `development_memory_tool_route_diagnostic` mode is the schema-v9
route-only successor. It uses reader v7, profile/report schema v9,
`route_complete_retrieval_fail_closed_v1`, and the unchanged
`memory-tool-route-failure-taxonomy-v1` SHA-256. It retains aggregate
`routeFailureCategoryCounts` whose sum equals `failedCaseCount`. Admission or
rerank incompleteness is accepted only with empty Final/Injected/token surfaces
and is reported separately through `retrievalIncompleteCaseCount` and
`retrievalFailureCodeCounts`. Schema-v7 bytes omit every diagnostic field; v9
emits explicit empty aggregate maps. Cutoffs, retries, selection authority, and
the default-off runtime remain unchanged. A third paid run requires fresh
explicit authorization.

The first schema-v9 live diagnostic completed `12/300` routes and classified
the failures as `31` context deadlines, `83` invalid Tool Calls, and `174`
unclassified router failures, alongside `174` independent admission-
unavailable retrieval aggregates. Offline tracing then found a concrete
unclassified producer: admission failure returned before the already-started
route was observed. Route completion is now replayable and mandatory on all
exits; delegated cancellation is context-selected through a buffered result,
and Recorder writes require the originating generation. The retained
identity-free artifact is unchanged and no paid rerun is authorized.

Candidate-blind routing is no longer the next hypothesis. The schema-v10
`development_configured_candidate_judge` lane recalls and reauthorizes
candidates first, then gives only the redacted query and ordinal candidate
bodies to the exact configured GPT or DeepSeek model through
`chat-configured-candidate-judge-v1`. Empty/invalid output fails closed;
accepted ordinals still intersect fixed BGE order. Profile config v10, reader
v8, report v10, cost-basis v6, and exact configured Provider authority keep
this evidence separate from schema-v4/v5 SiliconFlow judges and schema-v6-v9
Tool routes. Fake/offline protocol proof and two separately authorized live
Development profiles now exist; the lane still has no Validation, production
composition, or promotion authority.

The owner then separately authorized exact schema-v10 GPT and DeepSeek
Development runs. `SERVER_DEFAULT/gpt-5.6-sol` completed `0/195` candidate-
bearing judge decisions: `146` requests hit `HARD_CUTOFF` and `49` cases failed
before judge egress as `RELEVANCE_ADMISSION_UNAVAILABLE`. Final Recall@5/
current-fact was `0/0`, false injection and every authority/privacy leak
counter were zero, and p95/p99 was `1856/1862 ms`.
`FOHWSU/deepseek-v4-flash` completed `157/195` decisions with `60` valid
abstentions; its `38` failures were `36` hard cutoffs plus `2` pre-judge
retrieval failures. Final Recall@5/current-fact was `0.558974/0.581818`, false
injection and every authority/privacy leak counter were zero, and p95/p99 was
`1854/1858 ms`. Both profiles failed unchanged gates. Neither selected a
policy or unlocked Validation, production composition, or promotion.

Schema v10 treats a candidate-bearing pre-judge retrieval failure as valid
aggregate failed-gate evidence only when admission, rerank, and judge readiness
are false, the judge token bound is zero, and Provider-sent, Final, Injected,
and prompt-token surfaces are empty/zero. Such a case does not increment the
actual judge-request count. The historical schema-v4/v5 report entry point
retains its stricter rejection behavior. This distinction prevents a correct
runtime `no_memory` result from destroying the entire schema-v10 evidence
bundle without rewriting old report semantics.

Schema v11 fixes one global cloud Memory Judge independent of the answer
model: `SERVER_DEFAULT`, `openai_compatible`, normalized Base URL
`https://sub.mumubuku.top/v1`, and model alias `gpt-5.6-luna`. Profile v11,
reader v9, report v11, criteria v2, cost-basis v7, and policy
`memory_hybrid_fixed_cloud_candidate_judge_development_v1` are separate from
all schema-v10 evidence. Criteria v2 changes only complete-flow latency to
p95 `1500 ms`, p99 `2500 ms`, and hard cutoff `3000 ms`. Any timeout, Provider
error, invalid JSON, protocol drift, or late response yields empty v2 Memory;
normal chat continues through v1 without recalled/reranked fallback. A passing
Development bundle still stops before separately authorized Validation.

Schema v12 preserves that exact Luna/BGE/prompt/decoder/egress authority but
uses new identities: profile/report v12, reader v10, criteria v3, cost-basis
v8, policy `memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2`,
admission `development_fixed_memory_judge_accuracy_only`, and artifact
`fixed-memory-judge-accuracy-development.json`. It does not reinterpret the
failed schema-v11 bundle.

The v12 execution sequence is fixed as BGE query embedding, local admission,
BGE rerank, Luna judge, then Record. One controller enforces Provider
concurrency `1` across passage/query embedding, rerank, and judge calls. The
capture context and both HTTP clients have no elapsed deadline; only caller
cancellation can interrupt them. Criteria v3 omits latency/hard-cutoff verdicts
while retaining aggregate p95/p99 diagnostics. A hard-cutoff trace is rejected
as schema drift rather than scored.

Live mode sleeps one real second between the 300 cases. Fake protocol uses a
virtual/no-op clock but records the same 299 logical waits and 299000 ms, with
zero elapsed cooldown. A Provider request retries at most once and only for
408/429/5xx or a retryable transport/read interruption. Valid `Retry-After` is
honored; missing or invalid advice waits five seconds. Redirects, normal 4xx,
invalid JSON/schema/protocol output, stream parse failures, and structured
remote errors do not retry. Failure still releases no v2 Memory.

The schema-v4 cost basis fixes a 300-request ceiling, a 128-token output ceiling
per request, conservative UTF-8-plus-framing input token bounds, exact model
prices, and maximum judge cost before Provider construction. The candidate
Memory cost must cover that maximum, and actual aggregate token bounds may not
exceed the authorization.

Historical Tool-route cost-basis v4 fixed `38,400` preflight output tokens.
First-ToolRound cost-basis v5 instead binds the exact Provider ID/type, Base URL
SHA-256, model, 300 requests, conservative aggregate input/output event bounds,
exact prices, and absolute route cost without pretending product-round output
is always 128 tokens. The SiliconFlow and route credentials must not be the
same file, hard links, or equal bytes. Both are cleared after use and included
in leak scans.

Configured candidate-judge cost-basis v6 uses the same exact
Provider/model/Base-URL and 300-request/token/cost ceilings in a distinct
`configuredCandidateJudgeAuthority`. It rejects any cloud-judge or Tool-route
authority in the same document. Live mode also requires a judge credential
that is a different file and different bytes from the SiliconFlow retrieval
credential; both are cleared and leak-scanned.

Fixed Memory Judge cost-basis v7 preserves the 300-request/38,400-output-token
ceiling shape but requires the exact Luna tuple. The wrapper mounts independent
mode-`0600` retrieval and judge copies read-only, rejects hard links or equal
bytes, and overwrites/removes temporary copies on every exit. The runner has no
Vault decryption authority.

Accuracy-first cost-basis v8 is a distinct authority with a maximum of 600
Judge attempts and exactly 76800 maximum output tokens. Report telemetry binds
all BGE/Luna attempts and retries, per-stage aggregate request latency, cooldown
counts, total/retry Judge input-token upper bounds, and the exact
`JudgeAttempts * 128` output upper bound. Historical v6/v7 cost documents remain
300-request authorities and cannot be widened. A passing v12 Development
summary still sets `policySelected=false` and stops before Validation.

## Tests

```bash
cd mm-chat/backend
go test -race ./internal/memoryroute ./internal/usermemory \
  ./internal/memorycapture ./cmd/memory-regression-capture

# PostgreSQL 17 + pg_textsearch 1.3.1 + pgvector 0.8.5
MM_CHAT_TEST_DATABASE_URL=... \
  go test -run TestNativeMemoryRegressionLivePostgres ./internal/memorycapture

cd ..
bash scripts/test-memory-regression.sh
```

## Files

```text
recorder.go                        Per-case capture state and generation tokens
memory_tool_router_decorator.go    Bounded route delegation and Recorder publication
capture.go                         Production-reader observation assembly
memory_tool_route_development.go   Schema-v7/v9 aggregate report authority
configured_candidate_judge_development.go Schema-v10 aggregate report authority
fixed_memory_judge_development.go      Schema-v11 fixed-Luna report authority
accuracy_first_memory_judge_development.go Schema-v12 accuracy-first report/manifest authority
accuracy_first_providers.go            Global serial gate, retry, cooldown, and aggregate telemetry
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).
