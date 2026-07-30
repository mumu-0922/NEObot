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
