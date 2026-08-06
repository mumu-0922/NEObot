# Memory candidate-judge adapter

`memoryjudge` binds the shared strict hybrid Memory candidate-judge contract to
one already authorized `chat.Provider` and exact model. It exists for the
historical schema-v4/v5 Development experiments and the schema-v10
candidate-first configured GPT/DeepSeek Development hypothesis. Server
composition does not install it as production reader authority.

## Responsibilities

- reuse `usermemory.BuildHybridCandidateJudgePrompt` as the single prompt
  authority;
- bind one explicit Provider ID and model ID;
- request `temperature=0`, no reasoning, disabled thinking, and at most 128
  output tokens;
- support the historical bounded SSE content-delta adapter and the separately
  versioned bounded JSON-completion adapter without changing prompt semantics;
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
stream adapter  = chat-configured-candidate-judge-v1
JSON adapter    = chat-configured-candidate-judge-buffered-v1
maximum bytes   = 1024
```

The accepted output is one exact JSON object containing only `schemaVersion`
and at most five unique in-range `selectedOrdinals`. An empty ordinal list is
the sole `no_memory` decision.

## Usage

```go
judge, err := memoryjudge.NewChatAdapter(provider, chat.ModelRef{
    ProviderID: "configured-gpt",
    ModelID:    "exact-configured-model",
})
if err != nil {
    return err
}

service := usermemory.NewService(
    repository,
    usermemory.WithHybridCandidateJudge(judge),
    usermemory.WithHybridShadowRelevancePolicy(
        usermemory.HybridShadowCloudJudgeCalibrationPolicy(
            "exact-configured-model",
        ),
    ),
)
```

This wiring belongs only to isolated regression capture. It is intentionally
absent from the Server composition root.

The schema-v17 transport diagnostic uses the same prompt/model controls through
the optional Provider-owned completion seam:

```go
bufferedProvider, ok := provider.(chat.BufferedChatProvider)
if !ok {
    return errors.New("configured candidate judge has no buffered completion support")
}
judge, err := memoryjudge.NewBufferedChatAdapter(bufferedProvider, modelRef)
```

`OpenAICompatibleProvider.CompleteChat` sends `stream:false`, requests
`application/json`, caps the HTTP body at 2 MiB, and accepts exactly one
completed choice whose content is present and whose `finish_reason` is
`stop`. The adapter then applies the same 1024-byte contract and strict ordinal
decoder as the streaming path.

## API overview

| API | Purpose |
| --- | --- |
| `NewChatAdapter(provider, modelRef)` | Bind an already authorized chat Provider/model. |
| `(*ChatAdapter).JudgeHybridCandidates(ctx, input)` | Execute and strictly validate one bounded judge response. |
| `NewBufferedChatAdapter(provider, modelRef)` | Bind the same contract to an already authorized bounded-completion Provider/model. |
| `(*BufferedChatAdapter).JudgeHybridCandidates(ctx, input)` | Execute one bounded JSON completion and reuse the strict shared decoder. |

## Dependencies

- `internal/chat`: normalized Provider stream, optional bounded completion, and
  model reference;
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
