# SiliconFlow Voice Provider Production Contract

## 1. Scope / Trigger

This contract owns the production Text-to-Speech path selected on 2026-07-27.
SiliconFlow CosyVoice2 is server-owned and is invoked only after an explicit
read-aloud click. Speech-to-text remains unavailable. ElevenLabs and MiMo Voice
identities remain reserved but are not production executors.

## 2. Exact Authority

The only production tuple is:

```text
provider record: VOICE:SILICONFLOW
config.kind:     voice
voiceProvider:   siliconflow
base URL:        https://api.siliconflow.cn/v1
model:           FunAudioLLM/CosyVoice2-0.5B
voice:           FunAudioLLM/CosyVoice2-0.5B:claire
browser ingress: provider:voice:siliconflow
vault at rest:   provider:voice:<userId>:VOICE:SILICONFLOW
```

The retained reservations are:

```text
VOICE:ELEVENLABS / voiceProvider=elevenlabs
VOICE:MIMO       / voiceProvider=mimo
```

They have no enabled runtime adapter in this release. Model, Search, and RAG
rows or credentials never authorize Voice execution.

## 3. Administrator Lifecycle

```text
GET    /v1/admin/voice/providers
PUT    /v1/admin/voice/providers/siliconflow
POST   /v1/admin/voice/providers/siliconflow/test
POST   /v1/admin/voice/providers/siliconflow/activate
DELETE /v1/admin/voice/providers/siliconflow
```

- `PUT` accepts only an RSA-encrypted BYOK ingress envelope, `enabled`, or
  `clearApiKey`; plaintext credentials and arbitrary tuple fields are rejected.
- Go decrypts ingress transiently and persists only an `A256GCM` vault envelope
  under the exact Voice record context.
- `test` performs one bounded real `/audio/speech` request and records an
  attestation without enabling a disabled record.
- `activate` repeats that test and atomically enables the unchanged record.
- The attestation binds record ID, provider, base URL, model, voice, and exact
  encrypted secret reference. Credential rotation or tuple drift invalidates
  it and disables runtime resolution.
- List/runtime responses expose only status, tuple metadata, test time, and
  `hasApiKey`; no plaintext, vault envelope, test text, provider body, or audio
  bytes are returned.

## 4. Runtime and Capability Contract

`cmd/api` resolves exactly one enabled, currently attested SiliconFlow Voice
record and creates an `OpenAICompatibleExecutor` for that request. Missing,
ambiguous, disabled, stale, corrupt, or undecryptable authority fails closed
before provider execution. There is no environment credential fallback.

Public runtime config advertises:

```text
voice.defaultProvider="siliconflow"
voice.defaultTtsAvailable=true
voice.defaultSttAvailable=false
```

Frontend API capabilities remain split: `voiceSynthesis=true`,
`voiceTranscription=false`, and legacy aggregate `voice=false`. Server-default
read-aloud posts `provider="default" + messageId + text` to
`POST /v1/voice/synthesize`; the Go HTTP boundary rejects legacy
`model`/`elevenlabs`/`mimo` synthesis selectors. The frontend downloads
the returned actor-authorized file through `/v1/files/{fileId}/content`, checks
audio type and exact size, then uses the disposable object-URL lifecycle.
Browser `speechSynthesis` is a separate manual free option; provider failure
is visible and never silently changes engines.

## 5. Cache and Cleanup Contract

- Cache authority is per authenticated user and source message. The source
  message must still belong to that user and its current trimmed text must
  match the submitted text.
- The exact key binds message, source update time, text SHA-256, provider,
  model, and voice. An unchanged click reuses one artifact and makes no
  provider request.
- In-process concurrent first clicks for one user/message share one generation.
  Cross-process convergence is protected by the transactional one-row-per-
  user/message constraint and replacement cleanup queue.
- A changed source or tuple replaces the current artifact and queues the old
  file for deletion. Cache/file ownership is revalidated inside the commit.
- A cache hit is invalid after three days without access, even if the periodic
  worker has not run yet. The worker runs every five minutes and processes a
  bounded replay-safe cleanup queue.
- Live cached audio is capped at 100 MiB per user. The current artifact is kept
  only if it fits; older entries are reclaimed least-recently-used. One user
  cannot hit, evict, claim, or delete another user's audio.
- Soft-deleted messages, conversations, users, missing/deleted files, expired
  entries, replacements, LRU victims, and failed cache commits all converge on
  the same file/object deletion boundary. Queue claims expire after ten
  minutes and can be replayed safely.
- TTS has no product-specific daily spend quota. Existing authentication,
  global rate limiting, 12 KiB text input, 10 MiB provider output, timeout,
  audit, and explicit-click controls remain mandatory safety bounds.

## 6. Failure Matrix

| Condition | Result |
| --- | --- |
| Plaintext or malformed admin secret | encrypted-ingress rejection |
| Missing Voice Key | `VOICE_PROVIDER_SECRET_REQUIRED` |
| Real connection test fails | `VOICE_PROVIDER_CONNECTION_TEST_FAILED` |
| Record changes while testing | `VOICE_PROVIDER_CONFIG_CHANGED` |
| Missing/ambiguous/stale runtime authority | `VOICE_JOBS_UNAVAILABLE`; zero provider call |
| Missing artifact/cache dependency | `VOICE_ARTIFACT_STORE_UNAVAILABLE` or `VOICE_CACHE_UNAVAILABLE` |
| Missing or cross-user source message | `VOICE_SOURCE_MESSAGE_NOT_FOUND` |
| Submitted text differs from current source | `VOICE_SOURCE_MESSAGE_CHANGED` |
| Text exceeds 12 KiB | `TEXT_TOO_LONG` |
| Provider failure | sanitized `VOICE_PROVIDER_ERROR`; no browser fallback |
| Artifact metadata/content mismatch | frontend rejects playback |

## 7. Verification and Rollback

Required proof includes encrypted save/test/activate/delete, attestation
invalidation and rotation, TTS-true/STT-false capability behavior, no legacy
Next route in server mode, cache hit/replacement/cross-user/TTL/LRU/claim replay,
051 down/up replay, all frontend and Go gates, Compose rendering, and a secret
scan. Ordinary tests are zero-network.

Migration `051_siliconflow_tts_cache` adds the exact Voice identity constraint,
`tts_audio_cache`, and `tts_audio_cleanup_queue`. Before rollback, stop the API
and cleanup worker, drain queued objects, verify no cache rows remain, and take
Postgres plus object-store backups. Only then roll back 051 and the application
image. Rolling back the schema first can orphan generated audio. Keep the
provider keyring while any Voice vault row or backup may need decryption.
