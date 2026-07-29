# Memory Tool routing adapter

`memoryroute` owns the exact bridge between the normalized chat Tool-planning
surface and the Development-only hybrid Memory route interface. It lets one
explicitly bound GPT or DeepSeek model decide whether the current request needs
saved Memory without exposing any Memory candidate body to that model.

## Responsibilities

- expose the single shared `search_memory` Tool definition;
- verify the canonical Tool-definition SHA-256 before constructing an adapter;
- bind one exact configured Provider ID and model ID;
- submit only the already-secret-redacted current query to `chat.ToolPlanner`;
- normalize zero calls to `no_memory` and one exact empty-argument call to
  `use_memory`;
- reject missing IDs, unknown tools, nil/malformed/non-empty arguments,
  duplicate calls, Provider failure, cancellation, and contract drift.

The package does not retrieve Memory, execute a same-model answer continuation,
read credentials, persist Tool output, activate the hybrid reader, or authorize
promotion. Those responsibilities remain with `usermemory`, the chat Tool Loop,
and the isolated regression runner.

## Fixed contract

```text
tool name          = search_memory
contract version   = memory-search-tool-v1
contract SHA-256   = f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
decoding profile   = memory-search-tool-decoding-v1
temperature        = 0
maximum output     = 128 tokens
thinking requested = disabled
arguments          = explicit JSON object {}
```

The Tool has no query argument. The server already owns the current request
text, so the model receives no capability to rewrite the Memory query or select
Memory IDs.

## Usage

```go
planner := configuredProvider.(chat.ToolPlanner)
router, err := memoryroute.NewChatToolAdapter(planner, chat.ModelRef{
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
        usermemory.HybridShadowMemoryToolRouteCalibrationPolicy("deepseek-chat"),
    ),
)
```

This wiring is currently used only by
`cmd/memory-regression-capture -capture-mode development_memory_tool_route`.
Production Server composition installs neither this calibration policy nor this
router.

## API overview

| API | Purpose |
| --- | --- |
| `SearchMemoryToolDefinition()` | Return the canonical no-argument Tool definition. |
| `ToolContractSHA256()` | Hash the exact JSON representation used for profile authority. |
| `NewChatToolAdapter(planner, modelRef)` | Bind one normalized planner to one Provider/model after drift checks. |
| `(*ChatToolAdapter).RouteHybridMemory(ctx, input)` | Return a provenance-bound boolean route decision. |

## Dependencies

- `internal/chat`: normalized `ToolPlanner`, Tool definition, Tool Call, and
  Provider/model identity;
- `internal/usermemory`: route interface, fixed contract constants, and hybrid
  policy authority.

## Verification

```bash
cd mm-chat/backend
go test ./internal/chat ./internal/memoryroute ./internal/usermemory
go test -race ./internal/memoryroute ./internal/usermemory
```

See [DESIGN.md](DESIGN.md),
[`memory-v2-hybrid-shadow.md`](../../../../.trellis/spec/backend/memory-v2-hybrid-shadow.md),
and
[`chat-tool-loop.md`](../../../../.trellis/spec/backend/chat-tool-loop.md).
