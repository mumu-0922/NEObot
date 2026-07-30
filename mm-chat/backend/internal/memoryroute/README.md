# Memory Tool routing adapter

`memoryroute` is a Development compatibility adapter between the canonical
chat Tool contract and `usermemory.HybridMemoryToolRouter`. The production
`search_memory` definition, hash calculation, call validation, execution, and
same-model continuation are owned by `internal/chat`; this package only lets
the isolated regression runner exercise the same first-`ToolRoundProvider`
decision boundary without copying that contract.

## Responsibilities

- delegate the canonical no-argument `search_memory` definition and SHA-256 to
  `internal/chat`;
- verify the canonical hash before constructing an adapter;
- bind one exact configured Provider ID and model ID;
- submit the current synthetic Development query as one first
  `ProviderRoundRequest` with `tool_choice=auto` and no continuation;
- normalize zero calls to `no_memory` and one exact empty-argument call to
  `use_memory` through the shared chat validator;
- reject missing IDs, unknown tools, nil/malformed/non-empty arguments,
  duplicate calls, invalid Provider events, failure, cancellation, and contract
  drift.

The package does not retrieve Memory, hydrate final content, execute an answer
continuation, read credentials, persist Tool output, enable the production
feature flag, or authorize promotion. Those responsibilities remain with
`usermemory`, `internal/chat`, and the isolated regression runner.

## Fixed contract

```text
tool name        = search_memory
contract version = memory-search-tool-v1
contract SHA-256 = f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
arguments        = explicit JSON object {}
tool choice      = auto
adapter version  = chat-first-tool-round-memory-decision-v1
```

The Tool has no query argument. The server owns the current request text, so
the model cannot rewrite the Memory query or select Memory IDs. Unlike the
historical schema-v6 `PlanTools` preflight, the first-round adapter does not
force a temperature, output-token limit, or Provider-specific thinking-control
field; it exercises the same normalized streaming round shape as product chat.

## Usage

```go
toolProvider := configuredProvider.(chat.ToolRoundProvider)
router, err := memoryroute.NewChatToolAdapter(toolProvider, chat.ModelRef{
    ProviderID: "configured-deepseek",
    ModelID:    "deepseek-chat",
})
if err != nil {
    return err
}

service := usermemory.NewService(
    repository,
    usermemory.WithHybridMemoryToolRouter(router),
    usermemory.WithHybridShadowRelevancePolicy(
        usermemory.HybridShadowMemoryFirstToolRoundCalibrationPolicy(
            "deepseek-chat",
        ),
    ),
)
```

This wiring is used only by
`cmd/memory-regression-capture -capture-mode development_memory_tool_route`.
Production Server composition does not install this adapter: the product path
is implemented directly in `internal/chat` and remains disabled by default
through `MEMORY_TOOL_LOOP_ENABLED=false`.

## Evidence status

Schema-v6/profile-v6/cost-basis-v4 are immutable failed evidence for the old
independent `PlanTools` preflight. The three retained live Development runs
remain unchanged:

- `SERVER_DEFAULT/gpt-5.6-sol`: `41` routes completed, Final Recall@5
  `0.087179`, current-fact accuracy `0.090909`, false injection `2/300`, and
  p95/p99 `2002/2003 ms`;
- `FOHWSU/deepseek-v4-pro`: protocol-invalid model-quality evidence because
  official DeepSeek received the generic thinking-control field;
- corrected `FOHWSU/deepseek-v4-flash`: `77` routes completed, Final Recall@5
  `0.256410`, current-fact accuracy `0.254545`, false injection `3/300`, and
  p95/p99 `1377/1808 ms`.

The successor is schema-v7/profile-v7/cost-basis-v5 with reader
`neo-chat.native-memory-reader-capture.v5`, policy
`memory_hybrid_main_model_first_tool_round_calibration_v1`, and artifact
`memory-first-tool-round-development.json`. Its offline protocol/lifecycle
tests pass, but no schema-v7 live Development run has been authorized or
executed. No policy is frozen; Validation and Promotion remain blocked.

## API overview

| API | Purpose |
| --- | --- |
| `SearchMemoryToolDefinition()` | Compatibility wrapper around the canonical chat Tool definition. |
| `ToolContractSHA256()` | Delegate the canonical JSON hash to `internal/chat`. |
| `NewChatToolAdapter(provider, modelRef)` | Bind one `ToolRoundProvider` to one Provider/model after drift checks. |
| `(*ChatToolAdapter).RouteHybridMemory(ctx, input)` | Return a provenance-bound boolean first-round decision. |

## Dependencies

- `internal/chat`: canonical Tool definition/hash/validator, normalized
  `ToolRoundProvider`, Tool events, and Provider/model identity;
- `internal/usermemory`: Development route interface, fixed provenance
  constants, and hybrid policy authority.

## Verification

```bash
cd mm-chat/backend
go test ./internal/chat ./internal/memoryroute ./internal/usermemory
go test -race ./internal/chat ./internal/memoryroute ./internal/usermemory
```

See [DESIGN.md](DESIGN.md),
[`memory-v2-hybrid-shadow.md`](../../../../.trellis/spec/backend/memory-v2-hybrid-shadow.md),
and
[`chat-tool-loop.md`](../../../../.trellis/spec/backend/chat-tool-loop.md).
