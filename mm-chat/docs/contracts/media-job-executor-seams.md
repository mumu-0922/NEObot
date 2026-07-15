# Media Job Executor Seams Contract

## 1. Scope / Trigger

This contract covers the G6.5c opt-in seams for server-owned voice and image
jobs:

- `POST /v1/voice/transcribe`;
- `POST /v1/voice/synthesize`;
- `POST /v1/images/generations`.

The trigger is any future change that connects a real STT/TTS/image provider,
executor, worker, queue, or storage backend to these endpoints. The default
runtime remains fail-closed and must not consume provider quota.

## 2. Signatures

Backend service seams:

```go
type voicejobs.Executor interface {
    Transcribe(context.Context, voicejobs.TranscribeRequest) (voicejobs.TranscribeResponse, error)
    Synthesize(context.Context, voicejobs.SynthesizeRequest) (voicejobs.SynthesizeResult, error)
}

type imagejobs.Executor interface {
    Generate(context.Context, imagejobs.GenerateRequest) (imagejobs.GenerateResult, error)
}

type ArtifactStore interface {
    Store(context.Context, jobartifacts.StoreInput) (jobartifacts.Artifact, error)
}
```

Stored executor output enters the artifact boundary as:

```go
jobartifacts.StoreInput{
    JobID:       "...",
    Kind:        jobartifacts.KindAudio | jobartifacts.KindImage,
    Filename:    "...",
    ContentType: "audio/*" | "image/*",
    Size:        positiveBytes,
    Body:        reader,
}
```

## 3. Contracts

- Executors are opt-in only via `WithExecutor`.
- Real executor calls require an explicitly configured admitted-job audit
  recorder.
- Quota-consuming real provider smoke also requires the separate
  `provider-live-smoke-authorization.md` gate.
- Synthesis and image-generation executor outputs require an artifact store
  before executor invocation.
- `voice.synthesize` outputs are stored with artifact kind `audio`.
- `image.generate` outputs are stored with artifact kind `image`.
- The OpenAI-compatible image executor accepts `openai`, `openai_compatible`,
  and `openai-compatible` provider IDs, posts to `/images/generations`, and
  accepts either `b64_json` or generated image URLs from provider responses.
- Responses expose only compact artifact metadata:
  `fileId`, `purpose`, `contentType`, `size`.
- Responses and audit events must not expose prompt text, synthesis text, audio
  bytes, image bytes, provider credentials, object-store keys, or direct storage
  URLs.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| No executor configured | `501 VOICE_JOBS_UNAVAILABLE` or `501 IMAGE_JOBS_UNAVAILABLE` |
| Executor configured but artifact store absent for synthesis/image | `503 VOICE_ARTIFACT_STORE_UNAVAILABLE` or `503 IMAGE_ARTIFACT_STORE_UNAVAILABLE` |
| Admitted audit recorder absent or failing | `503 JOB_AUDIT_UNAVAILABLE`; executor is not called |
| Artifact kind/content-type/size/body invalid | artifact storage returns an error; no inline payload fallback |
| Request validation fails before admission | `400` with endpoint-specific validation code |
| Context cancelled before admission/execution | `408 REQUEST_CANCELLED` at handler boundary |

## 5. Good/Base/Bad Cases

- Good: fake executor emits `audio/webm` or `image/png` stream, artifact store
  persists it, response returns only file metadata.
- Base: no executor is configured; endpoints validate request shape, audit
  unavailable status if a recorder exists, then return fail-closed unavailable.
- Bad: executor is configured without admitted audit recorder; service returns
  `JOB_AUDIT_UNAVAILABLE` and never calls the executor.
- Bad: code returns base64 image/audio bytes directly to the frontend.
- Bad: audit events include prompt, synthesis text, audio bytes, image bytes, or
  credentials.

## 6. Tests Required

Any change enabling or extending media executors must include tests proving:

- default service still fails closed without executor;
- unavailable audit errors block executor invocation;
- absent admitted audit recorder blocks executor invocation;
- missing artifact store blocks synthesis/image executor invocation;
- successful fake executor output is stored through `jobartifacts`;
- response payload contains only artifact metadata;
- audit events contain only allowed metadata fields;
- no real provider/network call is required for unit tests.

## 7. Wrong vs Correct

### Wrong

```go
// Calls provider before audit/storage gates and returns inline bytes.
bytes := provider.Generate(prompt)
return GenerateResponse{Images: []GeneratedImage{{ContentType: "image/png"}}}, nil
```

### Correct

```go
if err := service.recordAdmitted(ctx, request); err != nil {
    return GenerateResponse{}, err
}
result, err := executor.Generate(ctx, request)
if err != nil {
    return GenerateResponse{}, err
}
artifact, err := artifactStore.Store(ctx, jobartifacts.StoreInput{
    Kind: jobartifacts.KindImage,
    Body: result.Images[0].Body,
})
```
