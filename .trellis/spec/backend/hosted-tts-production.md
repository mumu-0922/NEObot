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
  { "provider": "default", "messageId": "<uuid>",
    "text": "<current raw message source>" }
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
  user, message, source update time, readable-text SHA-256, provider, model,
  and voice. Cache hits make zero provider calls.
- The request `text` remains the current raw Markdown/HTML message source and
  exists only for stale-source validation. After the exact source comparison,
  hosted synthesis derives provider input from the actor-owned persisted
  source; it never accepts client-supplied replacement speech text.
- Hosted readable-text projection removes Markdown formatting syntax, raw HTML
  tags/attributes/comments, `script`/`style` and hidden element content, image
  destinations, and link destinations. It preserves visible prose, headings,
  list/table cells, link labels, citations, inline code, and visible fenced
  code in reading order, then normalizes whitespace into stable paragraphs.
- `text_sha256` is the digest of that readable projection. Commit-time source
  validation reprojects the locked persisted source before comparing the
  digest. A pre-existing row keyed from raw markup therefore misses and is
  replaced through the normal replay-safe cleanup path without a migration.
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
- `VoiceSettings` derives Server mode from `createNeoChatApiClient().mode` and
  does not render legacy Local-mode model/ElevenLabs/Mimo provider options or
  ElevenLabs/Mimo browser-stored Key inputs. Keep the dormant Local-mode code
  as a rollback path. `applyServerConfig` owns normalization of persisted
  legacy provider selections to the hosted default or Browser speech; do not
  duplicate that migration in the component.
- Frontend browser/local synthesis reads normalized `innerText` from the
  forwarded `MessageOutputRenderer` root at click time. Hosted default still
  submits raw `message.content` for backend ownership/source validation; the
  rendered `innerText` must never replace that hosted request field.
- Read-aloud has one tab-scoped playback owner across all messages and
  conversations. The owner tracks one active message, generation,
  `AbortController`, and either one disposable audio element or one Browser
  Speech poller. Clicking the active message stops it; starting another message
  invalidates and disposes the older operation before the new one can play;
  unmounting the active message releases the owner.
- The frontend passes the owner's `AbortSignal` through hosted synthesis and
  authenticated File download. Any audio returned to a stale generation is
  disposed without calling `play()`. A `play()` rejection caused by stop,
  replacement, or unmount is normal cancellation and must not render a TTS
  error; a current synthesis, download, or playback failure remains visible.
- Blob-backed playback creates a hidden, `aria-hidden` `<audio>` node, appends
  it to `document.body`, and only then assigns the Object URL and calls
  `play()`. Disposal is idempotent and ordered as pause -> clear `src` ->
  `load()` -> remove the node -> revoke the Object URL. Do not rely on a
  detached `new Audio(objectUrl)` for hosted playback: Chromium may reject its
  pending `play()` as media removed from the document.

## 4. Validation & Error Matrix

| Condition                             | Required result                                                           |
| ------------------------------------- | ------------------------------------------------------------------------- |
| Plaintext/malformed admin secret      | reject encrypted ingress                                                  |
| Missing Key                           | `VOICE_PROVIDER_SECRET_REQUIRED`                                          |
| Test failure or non-audio `2xx`       | sanitized provider/test failure; no body leakage                          |
| Config changes during test            | `VOICE_PROVIDER_CONFIG_CHANGED`                                           |
| Missing/ambiguous/stale authority     | `VOICE_JOBS_UNAVAILABLE`; zero provider call                              |
| Legacy synthesis provider selector    | `400 UNSUPPORTED_VOICE_PROVIDER`                                          |
| Missing/cross-user message            | `404 VOICE_SOURCE_MESSAGE_NOT_FOUND`                                      |
| Submitted text differs from source    | `409 VOICE_SOURCE_MESSAGE_CHANGED`                                        |
| Owned source projects to empty text   | `422 VOICE_READABLE_TEXT_EMPTY`; zero Provider I/O                        |
| Missing cache/artifact dependency     | `VOICE_CACHE_UNAVAILABLE` or `VOICE_ARTIFACT_STORE_UNAVAILABLE`           |
| Runtime role lacks TTS table DML      | PostgreSQL permission failure before Provider I/O; deploy migration `052` |
| Artifact metadata/download mismatch   | frontend rejects playback                                                 |
| Active message clicked again          | abort/dispose current operation; return to idle without an error          |
| Another message starts                | abort/dispose old owner before new playback; never overlap                |
| Active message unmounts               | abort/dispose pending or playing work; hidden playback cannot survive     |
| Stale `play()` rejects after disposal | ignore as lifecycle cancellation; do not log/render synthesis failure     |
| Blob audio is created                 | append hidden node before assigning Object URL or calling `play()`        |
| Current synthesis or `play()` fails   | dispose resources and render the localized error for that message         |

## 5. Good / Base / Bad Cases

- Good: an explicit server-default click resolves the exact Voice vault row,
  validates the submitted raw source, speaks only the persisted readable-text
  projection, stores one audio artifact, downloads it through
  `/v1/files/{id}/content`, and a second unchanged click returns `cached=true`
  without provider I/O.
- Base: no active Voice row; public TTS availability is false, browser speech
  remains manually selectable, and server synthesis fails closed.
- Base: Browser Speech consumes the currently rendered output `innerText` and
  therefore speaks a link label but not its hidden destination.
- Base: a persisted Local-mode ElevenLabs/Mimo/model selection enters Server
  mode, is normalized by `applyServerConfig`, and cannot leave a hidden provider
  selected in the Voice settings UI.
- Bad: create TTS tables as the Migration Owner but omit `go_api_runtime` DML,
  or hot-grant production without an embedded forward/down migration pair.
- Bad: borrow `RAG:SILICONFLOW`, accept `provider="model"`, trust an HTTP 200
  containing JSON, or delete MinIO bytes without completing File/cache state.
- Bad: let each `MessageItem` own an independent audio ref or treat a disposed
  pending `play()` rejection as a provider failure. Both permit overlap or
  surface false errors during ordinary navigation.
- Bad: create hosted playback with a detached `new Audio(objectUrl)`, or revoke
  its Object URL without clearing the media source and removing the node.
- Bad: send rendered client text as hosted provider authority, send raw
  Markdown/HTML to the provider, or key cache rows by raw markup. The first
  permits arbitrary paid synthesis; the latter two speak hidden syntax or
  reuse stale markup audio.

## 6. Tests Required

- Unit: encrypted save/test/activate/invalidate, exact resolver tuple,
  non-audio response rejection, legacy selector rejection, sanitized errors,
  source-text validation, readable Markdown/GFM/HTML projection, markup-only
  rejection before Provider I/O, singleflight, rollback, and cleanup replay.
- PostgreSQL: `051` plus `052` down/up/down/up, representative existing provider rows,
  cache hit/replacement/cross-user miss, hard TTL, commit-time and worker LRU,
  claim/release/reclaim/complete, and `SET LOCAL ROLE go_api_runtime` proving
  DML/lock access while ownership and `TRUNCATE` remain denied.
- Frontend: capability split, hosted metadata normalization, authenticated file
  fetch, MIME/size equality, no server-mode `/api/voice/*` call, Browser-only
  local path, persisted disabled-to-enabled default selection, and signal
  forwarding through synthesis plus File download. Assert hosted requests keep
  raw source text while Browser/local speech receives normalized rendered
  `innerText`. Assert Server-mode composition hides legacy Local provider
  options and Key inputs while the existing Local rollback source remains.
- Frontend playback: rapid same-message cancellation, A-to-B replacement,
  stale synthesis/audio completion disposal, pending `play()` interruption,
  active-message unmount/release, genuine failure rendering, and Browser Speech
  cancellation/poller completion. Assert one active owner, zero overlap, DOM
  attachment before Object URL assignment, and idempotent ordered disposal.
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
raw Markdown/HTML -> Provider -> cache digest of hidden markup

or

rendered client text -> hosted Provider authority
```

### Correct

```text
encrypted admin ingress -> VOICE:SILICONFLOW vault + exact attestation
  -> provider="default" + raw client source validates owned current message
  -> persisted source -> readable-text projection -> Provider
  -> readable-text digest for lookup and commit-time locked revalidation
  -> go_api_runtime DML on cache/cleanup tables
  -> bounded audio-validated executor result
  -> File artifact + per-user cache
  -> authenticated download with propagated AbortSignal
  -> exact metadata verification
  -> hidden audio node attached before Object URL assignment
  -> tab-scoped generation owner disposes old playback before starting new
```
