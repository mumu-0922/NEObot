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

## Schema-v8 route-failure contract

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

## Second live schema-v8 attempt

The separately authorized run
`memory-regression-20260730t052917z-7b8c8bcf` reused the same exact
Provider/model, Base-URL hash, and cost-basis hash as the first attempt. It
consumed Provider quota and then failed with the new bounded reason:

```text
native Memory capture is invalid: Memory Tool-route report admission_state
```

This proves that at least one case had a non-empty candidate set with
`AdmissionReady=false`. It does not prove whether the cause was the fixed BGE
query-embedding cutoff, an invalid Provider response, or admission SQL failure;
those subcauses remain `[unverified]` because schema v8 retained no retrieval
failure aggregate. The attempt published zero artifacts, so it is neither
route-diagnostic nor quality evidence. Temporary credentials, helper/export
paths, containers, networks, and volumes were all destroyed; the empty external
evidence directory was removed. Validation and Promotion were not run.

## Selected schema-v9 successor

The v8 report treated a legitimate fail-closed BGE retrieval result as corrupt
route-diagnostic structure. That allowed unrelated retrieval tail latency to
erase the route failure taxonomy the run was intended to measure. The successor
keeps the same request shape, cost authority, route taxonomy, 750 ms embedding
cutoff, two-second hard cutoff, and no-retry behavior, but separates route
completeness from retrieval completeness:

```text
capture mode  = development_memory_tool_route_diagnostic
profile       = neo-chat.memory-regression-profile-config.v9
reader        = neo-chat.native-memory-reader-capture.v7
report        = neo-chat.memory-regression-relevance-calibration.v9
admission     = development_main_model_first_tool_round_route_failure_diagnostic_only
completeness  = route_complete_retrieval_fail_closed_v1
taxonomy      = memory-tool-route-failure-taxonomy-v1
taxonomy SHA  = 66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0
artifact      = memory-first-tool-round-route-diagnostic-development.json
```

Every case must still produce a complete route result or one bounded route
failure category. If admission or rerank is incomplete, Final/Injected/tokens
must be empty and the report increments `retrievalIncompleteCaseCount` plus one
normalized aggregate `retrievalFailureCodeCounts` entry. Schema-v7 bytes remain
free of all diagnostic fields; schema v9 explicitly emits empty route and
retrieval maps when their counts are zero. The v9 lane still cannot select a
policy, run Validation, enable production, or authorize Promotion. A third paid
run requires a new, explicit quota authorization.

## Debug retrospective

### Root cause category

- **Primary: E — implicit assumption.** Schema v8 assumed a route-failure
  diagnostic could require every independent BGE admission/rerank stage to be
  complete, even though those stages retain the production fail-closed cutoff.
- **Secondary: B — cross-layer contract.** The report builder treated
  request-local retrieval state as artifact-integrity authority for a route-
  taxonomy artifact. The objective and the admission gate were at different
  layers but were encoded as one completeness condition.

### Why the earlier fixes did not close the loop

1. The first run exposed only `ErrCaptureInvalid`, so cleanup destroyed the
   transient state before the violated invariant could be identified.
2. Bounded integrity reasons fixed observability, not semantics. The second run
   correctly named `admission_state` but still discarded all otherwise valid
   route observations.

### Prevention mechanisms

| Priority | Mechanism | Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Version route completeness separately from retrieval completeness. | Done |
| P0 | Runtime authority | Require empty Final/Injected/tokens whenever retrieval is incomplete. | Done |
| P0 | Evidence | Preserve v7 and both v8 attempts; never rewrite them as v9 evidence. | Done |
| P1 | Tests | Cover admission failure, rerank failure, v7 omission, explicit empty v9 maps, and malformed aggregate types. | Done |
| P1 | Operations | Require fresh authorization for every paid rerun and retain zero partial artifacts on structural failure. | Done |

### Systematic expansion

Multi-stage diagnostics must define completeness per measured stage. An
independent fail-closed stage may lower quality evidence, but it must not erase
valid evidence for another stage unless the retained artifact falsely claims
end-to-end completeness. Future diagnostic schemas must predeclare which stage
is authoritative, which incomplete stages are aggregated, and which output
surfaces must remain empty.
