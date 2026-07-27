# Hosted TTS Production Contract

## 1. Scope / Trigger

Apply this contract when changing production Voice provider administration,
`POST /v1/voice/synthesize`, stored-audio reuse, cleanup, capability truth, or
frontend server-mode read-aloud. The current production tuple is fixed to
SiliconFlow CosyVoice2/`claire`; STT remains unavailable.

## 2. Signatures

Administrator and runtime APIs:

```text
GET    /v1/admin/voice/providers
PUT    /v1/admin/voice/providers/siliconflow
POST   /v1/admin/voice/providers/siliconflow/test
POST   /v1/admin/voice/providers/siliconflow/activate
DELETE /v1/admin/voice/providers/siliconflow

POST /v1/voice/synthesize
  { "provider": "default", "messageId": "<uuid>", "text": "<current text>" }
  -> { "fileId": "<uuid>", "purpose": "audio",
       "contentType": "audio/*", "size": <integer>, "cached": <boolean> }
```

Exact authority and tuple:

```text
record             VOICE:SILICONFLOW
vault context      provider:voice:<userId>:VOICE:SILICONFLOW
ingress context    provider:voice:siliconflow
base URL           https://api.siliconflow.cn/v1
model              FunAudioLLM/CosyVoice2-0.5B
voice              FunAudioLLM/CosyVoice2-0.5B:claire
```

Migration `051_siliconflow_tts_cache` owns
`provider_configs_voice_identity_check`, `tts_audio_cache`, and
`tts_audio_cleanup_queue`. Migration `052_tts_runtime_role_grants` grants
only `SELECT, INSERT, UPDATE, DELETE` on the two TTS runtime tables to
`go_api_runtime`; it grants neither ownership nor `TRUNCATE`.

## 3. Contracts

- Accept a reusable Key only through RSA-encrypted BYOK ingress; persist only a
  context-bound AES-GCM vault envelope. Model, Search, RAG, smoke, and
  environment credentials never authorize Voice.
- Test and activation make one bounded real `/audio/speech` call. Activation
  attests the exact record, endpoint, model, voice, and encrypted secret
  reference; any drift invalidates it.
- Runtime resolves exactly one enabled, currently attested
  `VOICE:SILICONFLOW`. The HTTP synthesis boundary accepts only
  `provider="default"`; legacy provider selectors are rejected rather than
  remapped.
- Provider `2xx` bytes must have an audio media type or be independently
  detected as audio. Never label JSON/text/unknown bytes as MP3.
- The source message must be current and owned by the actor. The cache key binds
  user, message, source update time, trimmed-text SHA-256, provider, model, and
  voice. Cache hits make zero provider calls.
- Reuse one artifact per user/message, expire after three idle days, enforce a
  100 MiB per-user LRU ceiling, and delete only through the actor-authorized
  File/object boundary. Cleanup claims are replayable after ten minutes.
- The API login must inherit `go_api_runtime`, and that capability role must
  hold DML on both TTS cache tables. Missing grants fail before Provider I/O;
  repair them through a new forward migration, never by editing applied `051`.
- Establish the deployed authentication mode before interpreting live access
  evidence. With `AUTH_REQUIRE_LOGIN=false` (including the unset standalone
  default), middleware injects the fixed Development Owner, so a request
  without a Bearer token is still owner-authorized. This proves single-user
  ownership but must not be reported as a missing-token `401` proof.
- Frontend server mode exposes `voiceSynthesis=true`,
  `voiceTranscription=false`, and legacy `voice=false`. Hosted default and
  explicit browser speech are the only server-mode TTS choices; there is no
  silent browser fallback.

## 4. Validation & Error Matrix

| Condition                           | Required result                                                           |
| ----------------------------------- | ------------------------------------------------------------------------- |
| Plaintext/malformed admin secret    | reject encrypted ingress                                                  |
| Missing Key                         | `VOICE_PROVIDER_SECRET_REQUIRED`                                          |
| Test failure or non-audio `2xx`     | sanitized provider/test failure; no body leakage                          |
| Config changes during test          | `VOICE_PROVIDER_CONFIG_CHANGED`                                           |
| Missing/ambiguous/stale authority   | `VOICE_JOBS_UNAVAILABLE`; zero provider call                              |
| Legacy synthesis provider selector  | `400 UNSUPPORTED_VOICE_PROVIDER`                                          |
| Missing/cross-user message          | `404 VOICE_SOURCE_MESSAGE_NOT_FOUND`                                      |
| Submitted text differs from source  | `409 VOICE_SOURCE_MESSAGE_CHANGED`                                        |
| Missing cache/artifact dependency   | `VOICE_CACHE_UNAVAILABLE` or `VOICE_ARTIFACT_STORE_UNAVAILABLE`           |
| Runtime role lacks TTS table DML    | PostgreSQL permission failure before Provider I/O; deploy migration `052` |
| Artifact metadata/download mismatch | frontend rejects playback                                                 |

## 5. Good / Base / Bad Cases

- Good: an explicit server-default click resolves the exact Voice vault row,
  stores one audio artifact, downloads it through `/v1/files/{id}/content`, and
  a second unchanged click returns `cached=true` without provider I/O.
- Base: no active Voice row; public TTS availability is false, browser speech
  remains manually selectable, and server synthesis fails closed.
- Bad: create TTS tables as the Migration Owner but omit `go_api_runtime` DML,
  or hot-grant production without an embedded forward/down migration pair.
- Bad: borrow `RAG:SILICONFLOW`, accept `provider="model"`, trust an HTTP 200
  containing JSON, or delete MinIO bytes without completing File/cache state.

## 6. Tests Required

- Unit: encrypted save/test/activate/invalidate, exact resolver tuple,
  non-audio response rejection, legacy selector rejection, sanitized errors,
  source-text validation, singleflight, rollback, and cleanup replay.
- PostgreSQL: `051` plus `052` down/up/down/up, representative existing provider rows,
  cache hit/replacement/cross-user miss, hard TTL, commit-time and worker LRU,
  claim/release/reclaim/complete, and `SET LOCAL ROLE go_api_runtime` proving
  DML/lock access while ownership and `TRUNCATE` remain denied.
- Frontend: capability split, hosted metadata normalization, authenticated file
  fetch, MIME/size equality, no server-mode `/api/voice/*` call, Browser-only
  local path, and persisted disabled-to-enabled default selection.
- Release: all Go tests/vet, frontend format/lint/typecheck/tests/build, Compose
  example and active render, diff secret scan, and standalone full gate.
- Live: only with explicit authorization and a fresh administrator-entered Key;
  prove activation, playback, cache hit, and fixture cleanup. Record whether
  the actor came from a required login session or the standalone fixed-owner
  middleware; test missing-token/cross-user rejection only with
  `AUTH_REQUIRE_LOGIN=true`.

## 7. Wrong vs Correct

### Wrong

```text
RAG:SILICONFLOW key -> provider="model" -> 200 JSON body -> assume audio/mpeg
```

### Correct

```text
encrypted admin ingress -> VOICE:SILICONFLOW vault + exact attestation
  -> provider="default" + owned current message
  -> go_api_runtime DML on cache/cleanup tables
  -> bounded audio-validated executor result
  -> File artifact + per-user cache
  -> authenticated download and exact metadata verification
```
