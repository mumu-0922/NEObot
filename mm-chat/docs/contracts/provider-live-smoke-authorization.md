# Provider Live Smoke Authorization Contract

## 1. Scope / Trigger

This contract covers any test, command, executor, or handler path that can call
a real third-party provider for G6 media jobs.

The gate exists because live STT/TTS/image smoke tests can consume provider
quota, use credentials, depend on network availability, and produce
non-deterministic outputs. Unit tests and normal development must remain
provider-free by default.

## 2. Signatures

Environment keys:

```text
MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=false
MM_CHAT_PROVIDER_LIVE_SMOKE_APPROVAL=
MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS=
MM_CHAT_PROVIDER_LIVE_SMOKE_RUN_ID=
MM_CHAT_PROVIDER_LIVE_SMOKE_OUTPUT_DIR=
MM_CHAT_PROVIDER_LIVE_SMOKE_BASE_URL=
MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY=
MM_CHAT_PROVIDER_LIVE_SMOKE_VOICE=
MM_CHAT_PROVIDER_LIVE_SMOKE_IMAGE_SIZE=
```

Go authorization seam:

```go
cfg := providersmoke.LoadFromEnv(getenv)
err := cfg.Authorize(providersmoke.Target{
    Kind:       providersmoke.KindImageGenerate,
    ProviderID: "openai",
    ModelID:    "gpt-image-1",
})
```

`MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS` is a comma-separated list of exact
`kind:providerId:modelId` targets. Supported kinds are:

- `voice.transcribe`;
- `voice.synthesize`;
- `image.generate`.

## 3. Contracts

- Live provider smoke is denied by default.
- Authorization requires all of:
  - `MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=true`;
  - exact approval text:
    `I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA`;
  - non-empty sanitized `MM_CHAT_PROVIDER_LIVE_SMOKE_RUN_ID`;
  - exact target match in `MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS`.
- Wildcards, prefix matches, missing model IDs, and unknown job kinds are not
  authorized.
- Authorization errors expose only stable error codes, not provider secrets,
  prompt text, model-private labels, or credentials.
- This gate authorizes only the smoke attempt; executor-specific audit,
  artifact storage, rate-limit, cancellation, and capability gates still apply.
- `MM_CHAT_PROVIDER_LIVE_SMOKE_OUTPUT_DIR` and
  `MM_CHAT_PROVIDER_LIVE_SMOKE_IMAGE_SIZE` are optional smoke-harness settings;
  they do not authorize live calls by themselves.
- A `voice.synthesize` harness attempt additionally requires a non-empty
  `MM_CHAT_PROVIDER_LIVE_SMOKE_VOICE`. The value is passed as the exact
  provider voice rather than inferred from the provider or model. Missing it
  fails before executor construction or any network call.
- `MM_CHAT_PROVIDER_LIVE_SMOKE_BASE_URL` and
  `MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY` are explicit one-off harness inputs for
  direct executor tests. They are never read by normal API runtime resolution,
  never persisted, and do not authorize a call without the four gate values.

## 4. Validation & Error Matrix

| Condition | Code |
| --- | --- |
| Enabled flag absent/false | `PROVIDER_LIVE_SMOKE_DISABLED` |
| Approval text absent or not exact | `PROVIDER_LIVE_SMOKE_APPROVAL_REQUIRED` |
| Run ID absent after sanitization | `PROVIDER_LIVE_SMOKE_RUN_ID_REQUIRED` |
| Requested target is malformed | `PROVIDER_LIVE_SMOKE_TARGET_REQUIRED` |
| Requested target not exactly listed | `PROVIDER_LIVE_SMOKE_TARGET_NOT_AUTHORIZED` |
| Synthesis target has no explicit voice | Harness fails before provider execution |

All codes wrap `providersmoke.ErrNotAuthorized`.

## 5. Good/Base/Bad Cases

- Good: a one-off operator smoke sets the four env values for
  `image.generate:openai:gpt-image-1` and records the run ID in the process log.
- Good: a live image smoke stores its generated artifact under a local operator
  output directory and records only the path, status, and non-secret target.
- Good: a live voice synthesize smoke stores its generated audio artifact under
  the same local operator output directory and passes an explicit provider
  voice; a live voice transcribe smoke logs only transcript length, not
  transcript content or audio bytes.
- Base: normal local/dev/test environment leaves the env values blank; live
  smoke is denied.
- Bad: a test calls a real provider when the enabled flag alone is true.
- Bad: a target uses `image.generate:openai:*`.
- Bad: a synthesis harness silently falls back to an OpenAI default voice when
  the selected provider requires a provider-qualified voice.
- Bad: an error message echoes a provider key, prompt, or private model label.

## 6. Tests Required

Any live-provider smoke integration must prove:

- default env denies authorization;
- exact approval text is required;
- sanitized run ID is required;
- targets must match exactly;
- unknown/malformed target entries are ignored;
- synthesis smoke rejects a missing explicit voice without a provider call;
- authorization errors do not leak target/provider secret values;
- no network/provider call is made in unit tests.

## 7. Wrong vs Correct

### Wrong

```go
if os.Getenv("MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED") == "true" {
    return provider.Generate(ctx, prompt)
}
```

### Correct

```go
cfg := providersmoke.LoadFromEnv(os.LookupEnv)
if err := cfg.Authorize(target); err != nil {
    return err
}
// Then still pass admitted audit, artifact storage, and executor gates.
return provider.Generate(ctx, request)
```

## 8. SiliconFlow TTS One-Off Run

The selected smoke-first tuple is:

```text
kind: voice.synthesize
provider: siliconflow
model: FunAudioLLM/CosyVoice2-0.5B
voice: FunAudioLLM/CosyVoice2-0.5B:claire
base URL: https://api.siliconflow.cn/v1
```

Run only with a dedicated one-off SiliconFlow key supplied directly to the
process environment. Do not source, copy, or reuse the RAG/model credential.

```bash
cd mm-chat/backend
read -r -s -p "Dedicated SiliconFlow Voice smoke key: " MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY
export MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY
echo

MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=true \
MM_CHAT_PROVIDER_LIVE_SMOKE_APPROVAL=I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA \
MM_CHAT_PROVIDER_LIVE_SMOKE_TARGETS='voice.synthesize:siliconflow:FunAudioLLM/CosyVoice2-0.5B' \
MM_CHAT_PROVIDER_LIVE_SMOKE_RUN_ID='siliconflow-cosyvoice2-YYYYMMDD' \
MM_CHAT_PROVIDER_LIVE_SMOKE_OUTPUT_DIR='/tmp/mm-chat-provider-smoke/siliconflow-cosyvoice2-YYYYMMDD' \
MM_CHAT_PROVIDER_LIVE_SMOKE_BASE_URL='https://api.siliconflow.cn/v1' \
MM_CHAT_PROVIDER_LIVE_SMOKE_VOICE='FunAudioLLM/CosyVoice2-0.5B:claire' \
GOCACHE=/tmp/neo-chat-go-build \
go test ./internal/voicejobs -run '^TestLiveOpenAICompatibleVoiceSmoke$' -count=1 -v

unset MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY
```

Keep only the sanitized run ID, exact non-secret tuple, output path, content
type, byte count, exit status, and a human audible-quality verdict. Never copy
the key, synthesis text, provider body, or audio bytes into documentation or
logs.
