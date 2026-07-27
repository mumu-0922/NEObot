# Hosted Media Provider Live Smoke

## 1. Scope / Trigger

Use this contract when changing a Go media executor or any test/command that can
call a real hosted Voice or Image provider. Ordinary tests and the normal API
runtime must remain provider-free unless their separate production resolver is
explicitly installed. The product contract is
`mm-chat/docs/contracts/provider-live-smoke-authorization.md`.

## 2. Signatures

Authorization target:

```go
providersmoke.Target{
    Kind:       providersmoke.KindVoiceSynthesize,
    ProviderID: "siliconflow",
    ModelID:    "FunAudioLLM/CosyVoice2-0.5B",
}
```

Required gate environment:

```text
MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=true
MM_CHAT_PROVIDER_LIVE_SMOKE_APPROVAL=I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA
MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS=kind:providerId:modelId
MM_CHAT_PROVIDER_LIVE_SMOKE_RUN_ID=<sanitized-run-id>
```

Direct executor inputs are
`MM_CHAT_PROVIDER_LIVE_SMOKE_BASE_URL`,
`MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY`, and, for synthesis,
`MM_CHAT_PROVIDER_LIVE_SMOKE_VOICE`.

## 3. Contracts

- Authorize one exact `kind:providerId:modelId`; never accept wildcards,
  prefixes, missing fields, or an enabled flag by itself.
- Supply credentials only to the one-off test process. Do not read a RAG,
  Search, model-provider, persisted `.env`, or vault credential as Voice
  authority.
- A synthesis smoke must pass an explicit provider-compatible voice. Do not
  infer it from the model or silently use the executor's OpenAI `alloy`
  fallback. For the selected SiliconFlow tuple, use
  `FunAudioLLM/CosyVoice2-0.5B:claire`.
- Store synthesis output only in the operator smoke directory with mode `600`.
  Logs and durable evidence may contain the sanitized run ID, non-secret tuple,
  path, content type, byte count, status, and audible verdict—not the Key,
  input text, provider body, or audio bytes.
- A successful direct smoke proves the endpoint/model/voice/artifact seam. It
  does not install or authorize production provider/vault/runtime wiring.

## 4. Validation & Error Matrix

| Condition | Behavior |
|---|---|
| Gate disabled | Skip before executor construction; zero network calls |
| Approval, run ID, or exact target missing | `providersmoke.ErrNotAuthorized` with stable code |
| Enabled gate has no Voice target | Test failure, not a successful skip |
| Synthesis voice missing or blank | Fail before executor construction/network |
| Base URL or one-off Key missing | Executor configuration fails before network |
| Provider returns non-2xx | Sanitized `ErrVoiceProviderFailed`; discard bounded body |
| Audio is empty or exceeds the response limit | Fail without artifact fallback |
| Artifact is not independently identifiable as audio | Do not close the live gate |

## 5. Good / Base / Bad Cases

- Good: exact SiliconFlow target plus explicit `claire` voice stores one
  non-empty MP3 and the owner accepts playback quality.
- Base: ordinary `go test ./...` reports the live test disabled and makes no
  provider request.
- Bad: enabling the smoke but omitting the model target is reported as a skip.
- Bad: a SiliconFlow synthesis request inherits `alloy`.
- Bad: a successful HTTP status closes the gate without file-type/audio
  inspection or with a credential copied from RAG configuration.

## 6. Tests Required

- `providersmoke`: default denial, exact approval, sanitized run ID, exact
  target match, and slash-containing model IDs.
- Voice harness: explicit voice trimming and missing-voice failure without a
  network client.
- Executor: exact provider-qualified voice reaches `/audio/speech` JSON;
  authorization header and model/input fields remain correct.
- Focused and full Go tests plus `go vet`; ordinary live test remains skipped.
- Authorized live run: one stored artifact, mode/size/type inspection,
  sanitized evidence, and owner audible-quality verdict.
- Secret scan: no Key/Bearer/private-key pattern in the tracked diff.

## 7. Wrong vs Correct

### Wrong

```go
response, err := service.Synthesize(ctx, SynthesizeRequest{
    Provider: ProviderModel,
    ModelID:  target.ModelID,
    Text:     smokeText,
}) // silently inherits "alloy"
```

### Correct

```go
voice, err := loadLiveVoiceSmokeVoice(os.Getenv)
if err != nil {
    return err // before executor construction or network
}
response, err := service.Synthesize(ctx, SynthesizeRequest{
    Provider: ProviderModel,
    ModelID:  target.ModelID,
    Text:     smokeText,
    VoiceID:  voice,
})
```
