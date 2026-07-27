# SiliconFlow TTS Production Integration

## Goal

Turn the already verified SiliconFlow CosyVoice2 smoke tuple into a
server-owned production TTS path: an administrator stores a fresh dedicated
credential through encrypted ingress, a bounded real test attests the exact
configuration, Go resolves and executes only that active Voice provider,
stores generated audio, and the existing read-aloud control plays the
authenticated artifact without exposing credentials or audio bytes in JSON.

## What I Already Know

- The owner wants Text-to-Speech only; speech-to-text/ASR is not part of this
  integration.
- The verified tuple is SiliconFlow
  `FunAudioLLM/CosyVoice2-0.5B` with
  `FunAudioLLM/CosyVoice2-0.5B:claire` at
  `https://api.siliconflow.cn/v1`.
- A real authorized Go smoke produced a valid MP3 and the owner accepted its
  Chinese playback quality.
- `voicejobs.OpenAICompatibleExecutor`, audit admission, artifact storage, Go
  routes, and frontend read-aloud UI already exist.
- Normal `cmd/api` intentionally installs no Voice executor, and the frontend
  server capability remains false.
- Current Voice reservation/vault validation recognizes only ElevenLabs and
  MiMo. Production SiliconFlow needs its own `VOICE:SILICONFLOW` authority and
  cannot reuse `RAG:SILICONFLOW`, a model provider row, or the exposed smoke
  key.
- The frontend has one `voice` capability for both TTS and STT. A TTS-only
  rollout must not accidentally advertise or enable transcription.

## Assumptions

- One administrator-managed SiliconFlow Voice provider is shared by
  authenticated users in the standalone deployment, matching the existing
  Server Default provider ownership model.
- The MVP fixes model and voice to the verified tuple rather than exposing
  arbitrary model, voice, cloning, or base-URL controls to ordinary users.
- A fresh production key will be entered through encrypted administrator BYOK
  ingress; the smoke key is revoked and never promoted.
- The existing read-aloud button remains the only generation trigger; TTS is
  never generated automatically when a chat response arrives.
- The current deployment is owner-operated and effectively single-user. It
  does not need a TTS-specific per-minute generation quota or daily paid-input
  budget.

## Requirements (Evolving)

- Add exact `VOICE:SILICONFLOW` provider reservation, Postgres/vault context,
  schema validation, rotation/backup compatibility, and exclusion from
  Model/Search/RAG readers.
- Add administrator create/update/status/test/enable behavior through the
  existing encrypted Provider Settings boundary; never accept plaintext keys
  in durable state or ordinary runtime environment variables.
- A provider can become active only after a bounded real test succeeds for the
  exact CosyVoice2 model and `claire` voice and records a current attestation
  bound to provider ID, endpoint, model, voice, credential fingerprint, and
  tested time.
- Resolve exactly one enabled, currently attested SiliconFlow Voice provider at
  runtime and install `OpenAICompatibleExecutor` in `cmd/api`; ambiguity,
  missing state, vault errors, or stale attestation fail closed before quota.
- Keep TTS and STT capability truth separate. TTS may reopen only after the
  production resolver is ready; ASR remains unavailable.
- Route server-mode read-aloud through Go `/v1/voice/synthesize`, then retrieve
  the actor-authorized stored artifact and play it through the existing
  disposable-audio lifecycle. Local/browser mode remains unchanged.
- In server mode, an explicit read-aloud click uses SiliconFlow by default.
  Browser `speechSynthesis` remains a user-selectable free option; provider
  failure must remain visible rather than silently changing engines. No click
  means no synthesis request and no provider charge.
- Preserve audit, rate limit, cancellation, timeout, response-size, artifact,
  credential-redaction, and user-ownership boundaries.
- Allow production TTS for every authenticated user. Reject unauthenticated
  synthesis and artifact access. Cache lookup and the 100 MiB storage ceiling
  are isolated per user; no user may observe, reuse, evict, or delete another
  user's audio.
- Add no TTS-specific generation-count or daily billing quota for this
  owner-operated deployment. Preserve the existing platform-wide abuse
  middleware and bounded per-request text/provider response limits; those are
  safety controls, not spend budgets. Cache hits make no provider call.
- Define bounded repeated-click behavior so retries do not silently create an
  unbounded paid-call and stored-artifact leak.
- Reuse at most one current artifact for the same message text, provider,
  model, and voice tuple. Concurrent first clicks must converge on one admitted
  generation; message/content/tuple changes replace and delete the old cached
  artifact rather than accumulating versions.
- Message, conversation, and user deletion must remove the associated TTS
  cache record and object. A retention/size policy must reclaim older audio
  even when the source message is never deleted.
- Treat a cached artifact as expired after three days without access. A
  periodic bounded cleanup worker deletes expired cache rows through the
  existing File/object deletion boundary; a later click generates a fresh
  artifact.
- Cap live cached TTS audio at 100 MiB per user. Before admitting or retaining
  an artifact above the ceiling, reclaim that user's least-recently-used cache
  entries. The current artifact may remain only when it fits within the hard
  ceiling; cleanup must be replay-safe and must not delete another user's
  files.
- Update contracts, deployment/admin instructions, public capability schema,
  progress evidence, and rollback instructions.

## Acceptance Criteria (Evolving)

- [x] Migration replay proves exact `VOICE:SILICONFLOW` reservation, vault
      context isolation, Model/Search/RAG exclusion, and down/up safety.
- [x] Administrator save stores only encrypted credential material; status is
      not active until the exact real test succeeds.
- [x] Credential/config changes invalidate the previous attestation and stop
      runtime resolution before provider execution.
- [x] `cmd/api` resolves the dedicated Voice credential and sends the verified
      model/voice tuple through the existing bounded executor.
- [x] TTS capability becomes true only when runtime synthesis is ready, while
      transcription remains false and fail-closed.
- [x] An authenticated server-mode read-aloud click produces and plays a stored
      audio artifact; response JSON contains metadata only.
- [x] Server mode defaults explicit read-aloud clicks to SiliconFlow, while a
      user can select browser speech without making a provider request.
- [x] Missing provider, disabled provider, ambiguity, vault failure, stale
      attestation, provider failure, audit failure, and storage failure expose
      stable sanitized errors with zero credential/text/audio leakage.
- [x] Every authenticated user can use TTS within independent limits;
      unauthenticated calls and cross-user artifact/cache access fail closed.
- [x] Repeated-click/cancellation behavior has tests and a bounded cost/storage
      outcome.
- [x] One unchanged message/tuple reuses one artifact; replacement and source
      deletion remove the old object, three-day idle expiry is materialized,
      and per-user LRU cleanup never exceeds 100 MiB.
- [x] Local browser speech behavior remains available and unchanged.
- [x] Focused tests, migration drill, frontend gates, all Go tests/vet, Compose
      rendering, full standalone verification, secret scan, and one final
      authorized end-to-end production-shape replay pass.

## Definition of Done

- Backend, frontend, persistence, security, deployment, and rollback contracts
  are implemented and tested end to end.
- No reusable Voice key is checked in, logged, placed in public config, or
  borrowed from another provider authority.
- Runtime state and generated audio cleanup/reuse behavior are documented.
- Changes are committed, the Task is archived, and the session journal is
  updated.

## Technical Approach

Implement one fresh, administrator-owned `VOICE:SILICONFLOW` provider through
the existing encrypted Provider Settings/vault boundary. Bind activation to a
real CosyVoice2/`claire` attestation, resolve exactly that authority into
`voicejobs.OpenAICompatibleExecutor`, reopen synthesis capability without
reopening transcription, and make the existing read-aloud button call Go and
fetch the stored actor-owned artifact. Add an idempotent per-message cache with
singleflight generation, one current tuple, three-day idle expiry, per-user
100 MiB LRU cleanup, and deletion through the existing File/object boundary.

## Decision (ADR-lite)

**Context:** The verified smoke proves protocol and voice quality but bypasses
the production provider/vault/runtime/frontend authority chain. Repeated paid
generation also needs bounded storage without imposing an unwanted spend cap
on the owner's single-user deployment.

**Decision:** Use SiliconFlow as the default server-mode engine only on an
explicit read-aloud click; retain browser speech as a manual free choice. Cache
one artifact per unchanged message/provider/model/voice tuple, expire it after
three idle days, and cap each user's live cache at 100 MiB. Do not add a
TTS-specific generation or daily billing quota.

**Consequences:** Replays normally avoid provider cost and stored audio cannot
grow without bound. The owner accepts that sustained new-message synthesis is
metered without a project-enforced daily spend ceiling; provider-console
billing remains the external cost authority.

## Implementation Plan

1. Add `VOICE:SILICONFLOW` migration, vault/admin validation, exact connection
   test, attestation, runtime resolver, and rollback coverage.
2. Install the resolved executor in `cmd/api`, split TTS/STT capability truth,
   and add the message-bound cache plus cleanup worker.
3. Cut server-mode read-aloud over to Go artifact fetch/playback while keeping
   browser speech selectable and local mode unchanged.
4. Run migration, component, security, Compose, full standalone, and one
   production-shape live replay; update contracts and operations evidence.

## Out of Scope

- Speech-to-text/ASR and microphone UI changes.
- VPS-local Piper or bundled voice model files.
- Voice cloning, custom voice upload, arbitrary model/base-URL editing, podcast
  or multi-speaker generation, and automatic read-aloud.
- Reusing the exposed smoke key, `RAG:SILICONFLOW`, Server Default model keys,
  or browser-local ElevenLabs/MiMo secrets.
- Kubernetes/multi-server work and unrelated former-root deletion.

## Research References

- [`research/current-production-path.md`](research/current-production-path.md)
  — repository-backed provider/vault/runtime/frontend path and production
  risks.

## Technical Notes

- Primary specs/contracts:
  `.trellis/spec/backend/provider-live-smoke.md`,
  `mm-chat/docs/contracts/voice-provider-reservation.md`,
  `mm-chat/docs/contracts/provider-secret-vault.md`,
  `mm-chat/docs/contracts/media-job-executor-seams.md`, and
  `mm-chat/docs/contracts/frontend-api-client.md`.
- Primary implementation seams:
  `backend/internal/runtimeconfig/voice_provider_reservation.go`,
  `backend/internal/voicejobs/`, `backend/cmd/api/main.go`,
  `frontend/src/services/api/voiceService.ts`, and
  `frontend/src/components/chat/MessageItem.tsx`.
- Complexity: high/cross-layer. This Task requires migration, backend,
  frontend, security, operational, and live-rollout validation.
