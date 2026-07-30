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
`frozen_validation`. Each phase requires a fresh separately authorized
mode-`0600` SiliconFlow key file; Tool-route Development additionally requires
a different fresh mode-`0600` GPT/DeepSeek credential. Live output is labelled
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
lifecycle tests pass; no live schema-v7 Development run exists and Validation
remains blocked.

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

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).
