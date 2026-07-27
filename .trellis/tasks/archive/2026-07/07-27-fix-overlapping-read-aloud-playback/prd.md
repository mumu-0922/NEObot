# Fix Overlapping Read-Aloud Playback

## Goal

Make read-aloud a single-owner browser playback flow so messages and
conversations cannot create overlapping audio, stale asynchronous starts cannot
play after cancellation or unmount, and ordinary stop/switch actions do not
surface Chrome's interrupted `play()` rejection as a synthesis failure.

## What I Already Know

- The production synthesis and authenticated File download succeed; the
  reported failure occurs at browser playback after the audio artifact exists.
- Each `MessageItem` currently owns its own `currentAudioRef`, browser-speech
  poller, `isPlaying`, and `isTTSLoading` state.
- Two different message components can therefore play simultaneously.
- A second click while the first async synthesis/download/play operation is
  still loading can start another operation because `isPlaying` remains false.
- Component unmount cleanup can dispose an audio element while its `play()`
  promise is pending, producing Chrome's interrupted-play rejection; the catch
  path currently mislabels that lifecycle cancellation as synthesis failure.
- `SynthesizeVoiceInput` and File download inputs already support
  `AbortSignal`, but `synthesizeSpeech` does not pass one through.

## Assumptions

- Only one read-aloud operation should exist in the whole browser tab, across
  all messages and conversations.
- User-requested cancellation is normal control flow and must not show an error.
- The server cache remains authoritative; this task changes playback ownership,
  not TTS provider, storage, TTL, or LRU policy.

## Requirements

- Centralize pending request, audio element, browser speech synthesis, poller,
  and active message identity in one tab-scoped coordinator.
- Starting any message stops and invalidates every older pending/playing
  read-aloud operation before the new one can play.
- Clicking the active message while loading or playing stops it immediately.
- Switching conversations or otherwise unmounting the active message stops the
  current/pending read-aloud operation immediately.
- Pass cancellation into server synthesis and File download; dispose any audio
  that resolves after its operation became stale.
- Treat `AbortError` and interrupted `play()` caused by coordinator disposal as
  cancellation, not a user-visible synthesis failure.
- Keep per-message spinner/stop icon state synchronized with the one global
  owner.
- Apply the same exclusivity to hosted audio and browser `speechSynthesis`.

## Acceptance Criteria

- [x] Rapid double-click on one message creates no overlapping playback and the
      second click cancels the pending/playing operation.
- [x] Starting message B stops message A and only B remains active.
- [x] Switching conversations cannot leave hidden, uncontrollable playback.
- [x] Stale synthesis/download/play completion is discarded without an error
      banner.
- [x] Genuine synthesis/download/play failures still show the localized error.
- [x] Hosted and browser speech paths obey the same global ownership rule.
- [x] Focused Vitest coverage, format, lint, typecheck, frontend tests/build,
      and the standalone gate pass.

## Verification

- Focused Vitest: 3 files and 23 tests passed; the full suite passed with 951
  tests.
- Frontend: format, lint, strict typecheck, full Vitest, and production build
  passed.
- Standalone: `bash mm-chat/scripts/verify-standalone.sh --full` passed,
  including Go and RAG (`1906 passed, 7 skipped`).
- Artifact audit: all three cached TTS objects matched PostgreSQL byte size and
  SHA-256 metadata and were identified as MPEG Layer III audio, ruling out a
  corrupt provider artifact for the reproduced failure.
- Deployment: rebuilt the current Compose frontend image and recreated the
  dependency chain selected by Compose; all services returned healthy, cached
  TTS state remained intact, and `/` plus `/mm-api/ready` both returned HTTP 200
  through `127.0.0.1:18080`.

## Definition of Done

- One tab-scoped read-aloud coordinator owns all playback lifecycle state.
- Message controls render truthful loading/playing state from that authority.
- Regression tests cover replacement, cancellation, stale completion, unmount,
  and real failure behavior.
- The deployed frontend is rebuilt and the reproduced multi-conversation flow
  no longer overlaps or reports lifecycle cancellation as synthesis failure.

## Technical Approach

Add a small external-store style read-aloud coordinator under the existing
frontend Voice utility boundary. It owns one generation token and
`AbortController`, one disposable audio element or browser poller, and one
active message. `MessageItem` subscribes to the coordinator instead of owning
independent media refs. Extend `synthesizeSpeech` with an optional signal and
thread it through synthesis/download/fetch requests.

## Decision (ADR-lite)

**Context:** Independent per-message playback causes overlapping audio, hidden
playback after conversation navigation, stale async starts, and false
interrupted-play errors.

**Decision:** Use global single playback. Clicking the active message stops it;
clicking another message stops the old operation before starting the new one;
switching conversations stops playback immediately. This task uses stop/replay,
not pause/resume.

**Consequences:** Playback is always controllable from the visible conversation
and never overlaps. A user who returns to a stopped message starts it again from
the beginning, normally reusing the server cache without another provider call.

## Out of Scope

- Audio seek bar, volume control, playback speed, or persistent mini-player.
- Backend cache/provider/storage changes.
- Multiple simultaneous read-aloud tracks.

## Technical Notes

- Primary caller: `mm-chat/frontend/src/components/chat/MessageItem.tsx`.
- Synthesis boundary: `mm-chat/frontend/src/services/api/voiceService.ts`.
- Existing audio lifecycle:
  `mm-chat/frontend/src/lib/utils/disposableAudio.ts`.
- Existing browser-speech polling:
  `mm-chat/frontend/src/lib/utils/speechPolling.ts`.
- Research trace: [`research/current-playback-path.md`](research/current-playback-path.md).
