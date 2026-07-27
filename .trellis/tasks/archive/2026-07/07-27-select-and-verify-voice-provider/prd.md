# Select and Verify Hosted TTS Provider

## Goal

Close the remaining G6 hosted Voice/TTS gate by selecting a low-cost hosted
provider that fits the existing Go `/v1/voice/*` seam, then proving the exact
provider/model target through the default-deny live-smoke harness without
leaking credentials, synthesis text, transcripts, or audio bytes.

## What I Already Know

- The standalone product is complete except hosted Voice/TTS selection and an
  explicitly authorized smoke, destructive former-root deletion, and optional
  multi-server/Kubernetes work.
- The owner previously rejected a VPS-local Piper-style engine; browser
  `speechSynthesis` remains only a device-local fallback.
- Go already has an OpenAI-compatible `/audio/speech` and
  `/audio/transcriptions` executor, artifact storage boundary, sanitized audit
  gate, and exact-target live-smoke authorization.
- Normal `cmd/api` runtime deliberately installs no Voice executor. Production
  enablement requires the dedicated Voice provider/vault chain rather than a
  model, Search, RAG, or environment credential shortcut.
- Current reserved Voice identities cover ElevenLabs and Mimo. Adding another
  provider requires an explicit reservation/spec/migration update.
- No Voice/TTS credential key is configured in the checked local
  `.env.single-server` key set. No secret value was read.
- The owner's authenticated catalog shows both available TTS models at
  CNY 0.05 per 1,000 UTF-8 input bytes. Free ASR catalog entries are a
  different capability and are not part of this Task.

## Assumptions to Validate

- A protocol/quality smoke should precede production runtime wiring so a paid
  provider or unstable API is not embedded into the provider administration
  model prematurely.
- SiliconFlow is the lowest-change candidate because its official API exposes
  `/audio/speech` and `/audio/transcriptions`, matching the existing Go
  executor, and provides Chinese/English hosted TTS plus STT models. The
  owner's authenticated model catalog on 2026-07-27 confirms both
  `fnlp/MOSS-TTSD-v0.5` and `FunAudioLLM/CosyVoice2-0.5B`; this live catalog
  resolves the simplified API-schema conflict in favor of CosyVoice2 being
  available to the account.
- A live attempt will require the owner to supply a dedicated one-off provider
  key and the exact quota-approval environment values at execution time.

## Requirements

- Compare at least SiliconFlow, ElevenLabs, and Mimo against protocol fit,
  Chinese quality, STT/TTS coverage, cost transparency, privacy, operational
  complexity, and current repository boundaries.
- Preserve default-deny behavior and make zero provider calls during unit tests
  or ordinary verification.
- Never read or reuse the RAG/model credential as Voice runtime authority.
- Require the exact `provider-live-smoke-authorization.md` enablement,
  approval text, target, and sanitized run ID before any quota-consuming call.
- Store synthesis output only in the operator smoke output directory; logs may
  record the path and byte count but not input text, transcript text, audio,
  response bodies, or credentials.
- Keep `.env.single-server.example` secret-free and do not persist the one-off
  smoke key.
- Limit this Task to provider selection and one directly authorized
  SiliconFlow CosyVoice2 synthesis smoke. Keep production provider
  administration, vault resolution, `cmd/api` wiring, and frontend capability
  reopening for a separate Task after the protocol/quality result is known.

## Acceptance Criteria

- [x] Provider choice is backed by dated official-source research and a
      documented rejection/deferral rationale for alternatives.
- [x] The selected endpoint/model/voice tuple is exact and supported by the
      existing executor or a tested bounded adapter.
- [x] Unit/configuration tests prove default denial and zero accidental network
      calls.
- [x] An authorized synthesis smoke produces a non-empty audio artifact with
      sanitized evidence, or the result is explicitly blocked pending owner
      credentials/approval without weakening the gate.
- [x] Evidence and progress wording identify this as TTS-only coverage and do
      not imply that speech-to-text/ASR was implemented or verified.
- [x] G6 progress/process documentation reflects only behavior actually
      reproduced.
- [x] Local mode and browser speech fallback remain unchanged.

## Definition of Done

- Focused Go/frontend/config tests, lint/type checks, and secret scans pass for
  every changed layer.
- No normal runtime credential shortcut or quota-consuming default path exists.
- Provider behavior, live evidence or blocking prerequisite, rollback, and
  remaining production-wiring scope are recorded.
- Changes are committed, Task is archived, and the session journal is updated.

## Technical Approach

Extend only the existing operator live-smoke harness so it can pass an explicit
provider-qualified voice to the OpenAI-compatible executor. Select SiliconFlow
`FunAudioLLM/CosyVoice2-0.5B` with
`FunAudioLLM/CosyVoice2-0.5B:claire`, preserve the existing exact-target,
approval, run-ID, timeout, size, artifact, and sanitized-audit gates, and run no
network call during ordinary tests. If no dedicated one-off key is available,
finish the offline safety proof and record the live attempt as blocked rather
than weakening or bypassing authorization.

## Decision (ADR-lite)

**Context:** The product needs hosted TTS proof, while full production Voice
wiring crosses provider administration, encrypted credentials, attestation,
runtime resolution, frontend capability, and deployment contracts.

**Decision:** Use the smoke-first SiliconFlow approach and verify TTS only.
Defer production wiring until the selected endpoint/model/voice tuple has been
proven against the owner's account.

**Consequences:** This gives the lowest-risk protocol and audio-quality signal
without opening normal runtime access. A successful smoke does not by itself
make hosted TTS available to end users; that remains a separate Task.

## Out of Scope

- Speech-to-text/ASR models, endpoints, UI, and live verification.
- VPS-local TTS engines or bundled voice model files.
- Reusing the existing SiliconFlow RAG vault row or a model-provider row for
  Voice.
- Production Voice provider reservation, administrator ingress/test,
  credential-vault resolution, `cmd/api` installation, and frontend capability
  reopening.
- Voice cloning, custom-voice upload, long-form podcast generation, streaming
  playback UI, or visible settings redesign.
- Former-root deletion and Kubernetes work.

## Research References

- [`research/provider-selection.md`](research/provider-selection.md) — current
  repository fit and dated official API/pricing evidence for the candidates.

## Technical Notes

- Primary contracts:
  `docs/contracts/voice-provider-reservation.md`,
  `docs/contracts/media-job-executor-seams.md`, and
  `docs/contracts/provider-live-smoke-authorization.md`.
- Existing implementation:
  `backend/internal/voicejobs/openai_compatible_executor.go` and
  `backend/internal/voicejobs/openai_compatible_live_test.go`.
- Authoritative ledger: `docs/tracking/progress.md` G6.5c.2b.
- Recommended first slice: smoke `FunAudioLLM/CosyVoice2-0.5B` with preset
  voice `FunAudioLLM/CosyVoice2-0.5B:claire`; keep production Voice
  unavailable until a later dedicated provider/vault/runtime slice is
  approved. The live harness must pass this provider-qualified voice instead
  of its current OpenAI default `alloy`.
- Authorized live evidence: run
  `siliconflow-cosyvoice2-20260727T110435` completed in 0.59 seconds and stored
  one mode-`600`, 58,495-byte MP3 identified as ID3v2.3/MPEG Layer III,
  128 kbps, 24 kHz, mono audio. The owner accepted the Chinese playback
  quality. The one-off credential was not persisted or logged.
