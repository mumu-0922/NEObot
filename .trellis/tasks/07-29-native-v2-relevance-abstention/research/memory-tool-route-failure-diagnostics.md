# Memory Tool-route failure diagnostics

## Question

Explain the `263/300` schema-v7 DeepSeek
`MEMORY_TOOL_ROUTE_FAILED` cases without weakening the two-second cutoff,
retrying Provider calls, retaining plaintext, or rewriting historical v7
evidence.

## Proven local data-flow

The live route used this exact chain:

```text
chat.OpenAICompatibleProvider.StreamToolRound
-> memoryroute.ChatToolAdapter.RouteHybridMemory
-> usermemory.routeHybridMemory
-> executeHybridCandidateStages
-> memorycapture.MemoryToolRouterDecorator
-> schema-v7 aggregate report
```

Before this diagnostic change, the adapter collapsed all of the following into
ordinary string errors and `usermemory` then collapsed them again into
`MEMORY_TOOL_ROUTE_FAILED`:

- synchronous request, transport, and HTTP-status failures;
- SSE parse, read, premature-termination, and remote-error frames;
- context deadline/cancellation;
- nil streams and unexpected Provider events;
- nil, rejected, malformed, unknown, or duplicate Tool Calls;
- model/contract provenance drift and capture-recorder conflicts.

The retained schema-v7 report contains only the final fallback code count. It
contains no request-local error subtype and no raw response. Therefore the
historical `263` cannot be retroactively split. Rate limiting, exhausted quota,
authentication rejection, upstream failure, and stream incompatibility all
remain `[unverified]` explanations for that completed run.

## Selected diagnostic contract

Add a schema-separated Development-only lane instead of mutating v7:

```text
capture mode  = development_memory_tool_route_diagnostic
profile       = neo-chat.memory-regression-profile-config.v8
reader        = neo-chat.native-memory-reader-capture.v6
report        = neo-chat.memory-regression-relevance-calibration.v8
admission     = development_main_model_first_tool_round_failure_diagnostic_only
taxonomy      = memory-tool-route-failure-taxonomy-v1
taxonomy SHA  = 66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0
artifact      = memory-first-tool-round-diagnostic-development.json
```

The OpenAI-compatible Provider emits typed, bounded categories for HTTP and
stream failures. The Memory route adds Tool/provenance/recorder categories.
The recorder retains only a request-local category and the report publishes
only `routeFailureCategoryCounts`. The sum must equal `failedCaseCount`.

No query, Tool arguments, Tool output, Provider response body, raw error text,
Memory content, score, case ID, or credential enters the retained diagnostic.
Schema-v7 continues to omit the new fields, so its historical artifact shape
remains immutable.

## Authority boundary

The v8 lane is diagnostic-only. Even if its unchanged quality evaluation
passes, the command summary cannot set `policySelected=true`. Validation and
Promotion remain blocked. Running the live lane still requires fresh explicit
quota and private-credential authorization; local implementation and fake
protocol tests do not grant that authorization.

## First live schema-v8 attempt

The owner-authorized run
`memory-regression-20260730t043820z-dc26df80` used:

```text
route Provider = FOHWSU
route type     = openai_compatible
route model    = deepseek-v4-flash
route URL hash = 12b8deaccc34b32757dbb1497e029da0c2e7b26ffa86b9c926c08cb4692f4508
cost hash      = 4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b
```

The isolated database migrated through `065`, the live runner performed
Provider traffic, and then the capture command returned the legacy generic
error `native Memory capture is invalid`. The wrapper observed an empty
artifact set and destroyed the isolated project. No report or manifest was
published, so no failure-category totals, quality metrics, or exact violated
invariant may be inferred from this attempt.

Post-run cleanup proved zero remaining temporary credential/helper paths and
zero scoped Docker containers, networks, or volumes. The empty evidence run
directory was removed. The run used quota but produced neither diagnostic nor
quality evidence.

The report builder previously mapped several distinct post-capture integrity
checks to the same error. It now returns a fixed content-free reason from a
closed set covering profile/config/policy/cost authority, observation/trace
integrity, candidate/admission/rerank state, route failure-category state, and
category totals. Manifest rejection similarly identifies only fixed authority,
configuration-hash, profile, or artifact classes. Tests require the error to
remain `ErrCaptureInvalid`, include no case ID, and preserve all report bytes
and admission gates. This does not recover the failed attempt retroactively and
does not authorize an automatic rerun.
