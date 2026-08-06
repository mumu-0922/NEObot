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
route Provider         = FOHWSU
route type             = openai_compatible
route model            = deepseek-v4-flash
route URL hash         = 12b8deaccc34b32757dbb1497e029da0c2e7b26ffa86b9c926c08cb4692f4508
private cost file hash = 4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b
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

## First live schema-v9 result

The owner-authorized run
`memory-regression-20260730t094556z-0f4878dd` used the same exact
`FOHWSU/openai_compatible/deepseek-v4-flash` route, Base-URL hash, fixed BGE
Provider, and unchanged cost authority as both v8 attempts. A local preflight
first rejected the input before Provider construction because the operator
helper incorrectly compared the raw private cost-file SHA-256 with
`memorycapture.CostBasisSHA256`, which hashes the decoded canonical structure.
That local attempt consumed no quota and left no credential/helper directory.
The corrected preflight independently required both hashes:

```text
private cost file SHA-256 = 4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b
manifest cost content SHA = b54b6fcfb62a33b31ef17cfd9876d392a20ef21bd25d19f67902350f194b1742
```

The paid run published the expected private two-file artifact set. Its route
aggregate reconciles exactly:

```text
route completed / used / abstained / failed = 12 / 12 / 0 / 288
CONTEXT_DEADLINE                         = 31
TOOL_CALL_INVALID                        = 83
ROUTER_FAILURE_UNCLASSIFIED              = 174
retrieval incomplete                     = 174
RELEVANCE_ADMISSION_UNAVAILABLE          = 174
```

This current run proves that `83` failures reached the bounded invalid-Tool-
Call category and `31` reached the context-deadline category. The remaining
`174` route failures are still intentionally content-free and unclassified;
the equal retrieval-incomplete total does not by itself prove a per-case
intersection because the artifact retains no identity. It therefore narrows
the current failure shape without retroactively reclassifying schema-v7 or
inventing a more specific upstream cause.

Candidate Recall@20 remained `1.0`, but Final Recall@5/current-fact accuracy
was `0.010256/0.012121`, false injection was `0/300`, p95/p99 latency was
`2001/2002 ms`, and the evaluator recorded `23` hard-cutoff violations. All
cross-user, deleted-Memory, secret, untrusted-source, and unauthorized-
Provider-egress counters were zero. Request/token/cost authority passed at
`300/300` route requests with conservative input/output upper bounds
`358533/2363529` under ceilings `600000/2457600`.

The result is valid diagnostic and failed-metric evidence, but the diagnostic
lane has no policy-selection authority. `policySelected=false`, Validation and
Promotion were not run, and `MEMORY_TOOL_LOOP_ENABLED` remains default-off.
The two retained artifacts are mode `0600` under a mode-`0700` external
directory. Both temporary credentials, the operator helper/export, runner
temporary directory, and the exact Compose containers/network/volume were
destroyed. No further paid run is authorized.

## Offline lifecycle follow-up

The post-run source trace proved one concrete producer of the `174`
unclassified aggregate:

```text
executeHybridShadow
  -> start route before query embedding
  -> query embedding/admission becomes unavailable
  -> record fail-closed retrieval and return without awaiting route
  -> Recorder.Finish sees route input but no result/category
  -> capture synthesizes ROUTER_FAILURE_UNCLASSIFIED
```

The route goroutine was canceled only during the service return. A delegated
router could return after `Recorder.Finish`; because route writes were bound
only to the Recorder's then-current case, that late result could conflict with
or attach to the next sequential case. The immutable v9 artifact has no case
identity, so this finding does not retroactively relabel all `174` cases or
prove that its equal retrieval aggregate is the same per-case set.

The offline repair keeps every production threshold and request shape intact:

1. The route stage now publishes one immutable replayable completion and is
   awaited on retrieval early exits up to the existing two-second hard cutoff.
2. The capture decorator selects a buffered delegated result against
   `ctx.Done()`, so an implementation that ignores cancellation cannot block
   the reader or publish Recorder state later.
3. Recorder route input/result/failure writes carry a per-case generation
   token; an old token is rejected even if the assistant identity is reused.
4. Retrieval failure remains fail-closed with empty Final/Injected/token
   surfaces and keeps its own bounded fallback code.

Focused deterministic tests reproduce the admission-unavailable early return,
prove route completion before capture closure, exercise a cancellation-
ignoring router, and reject previous-generation writes under the race detector.
No Provider call or paid rerun was used or authorized for this follow-up.

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
