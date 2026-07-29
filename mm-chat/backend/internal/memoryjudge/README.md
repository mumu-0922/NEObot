# Memory candidate-judge adapter

`memoryjudge` binds the shared strict hybrid Memory candidate-judge contract to
one already authorized `chat.Provider` and exact model. It exists for the
historical schema-v4/v5 Development experiments; Server composition does not
install it as production reader authority.

## Responsibilities

- reuse `usermemory.BuildHybridCandidateJudgePrompt` as the single prompt
  authority;
- bind one explicit Provider ID and model ID;
- request `temperature=0`, no reasoning, disabled thinking, and at most 128
  output tokens;
- accumulate only bounded content deltas, ignoring reasoning and Usage as
  contract output;
- reject Provider errors, unknown events, late output, and responses over 1024
  bytes;
- run the shared strict ordinal decoder before returning model/prompt
  provenance.

The package does not choose a Provider, read credentials, authorize candidate
egress, retrieve or rank Memory, persist plaintext/output, activate a policy,
or promote a reader.

## Fixed contract

```text
input schema    = neo-chat.memory-cloud-candidate-judge-input.v1
output schema   = neo-chat.memory-cloud-candidate-judge-output.v1
prompt version  = memory-cloud-candidate-judge-prompt-v1
prompt SHA-256  = c004e834f2db572fc8393f088f47750d420379664f972357f987a09d8647f9c8
decoding        = temperature-0_max-output-128_no-thinking_v1
maximum bytes   = 1024
```

The accepted output is one exact JSON object containing only `schemaVersion`
and at most five unique in-range `selectedOrdinals`. An empty ordinal list is
the sole `no_memory` decision.

## Usage

```go
judge, err := memoryjudge.NewChatAdapter(provider, chat.ModelRef{
    ProviderID: "siliconflow",
    ModelID:    "fixed-development-model",
})
if err != nil {
    return err
}

service := usermemory.NewService(
    repository,
    usermemory.WithHybridCandidateJudge(judge),
    usermemory.WithHybridShadowRelevancePolicy(
        usermemory.HybridShadowCloudJudgeCalibrationPolicy(
            "fixed-development-model",
        ),
    ),
)
```

This wiring belongs only to isolated regression capture. It is intentionally
absent from the Server composition root.

## API overview

| API | Purpose |
| --- | --- |
| `NewChatAdapter(provider, modelRef)` | Bind an already authorized chat Provider/model. |
| `(*ChatAdapter).JudgeHybridCandidates(ctx, input)` | Execute and strictly validate one bounded judge response. |

## Dependencies

- `internal/chat`: normalized Provider stream and model reference;
- `internal/usermemory`: prompt, schema, decoding, and candidate-judge
  interface authority.

## Verification

```bash
cd mm-chat/backend
go test ./internal/memoryjudge ./internal/usermemory ./internal/memorycapture
go test -race ./internal/memoryjudge ./internal/usermemory
```

See [DESIGN.md](DESIGN.md) and
[`memory-v2-hybrid-shadow.md`](../../../../.trellis/spec/backend/memory-v2-hybrid-shadow.md).
