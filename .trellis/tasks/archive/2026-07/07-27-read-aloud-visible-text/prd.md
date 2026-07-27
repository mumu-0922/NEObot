# Read Aloud Visible Message Text

## Goal

Make read-aloud speak the answer text the user can see instead of raw backend
Markdown/HTML and hidden formatting syntax, while preserving hosted source
ownership, cancellation, cache replacement, and local/browser TTS behavior.

## What I Already Know

- The user explicitly requires read-aloud to use frontend-visible text only.
- The current frontend sends raw `message.content` to every TTS path.
- Hosted TTS validates that raw text against the owned persisted message, then
  sends it unchanged to the provider.
- The reproduced answer contains Markdown headings/emphasis/links and a styled
  HTML `<div>`; the browser renders content, but the provider speaks tag names,
  CSS attributes, punctuation, and link destinations.
- Cached provider artifacts are valid audio; this is a speech-text projection
  defect.

## Requirements

- Hosted TTS must derive provider input from the authoritative stored message,
  never from arbitrary client-supplied replacement speech.
- Remove non-visible Markdown syntax, raw HTML tags/attributes/comments,
  `script`/`style` content, link destinations, and formatting-only punctuation.
- Preserve visible prose, headings, list/table cell text, link labels,
  citations, inline code, and visible fenced-code contents in reading order.
- Normalize whitespace into stable paragraphs so formatting does not create
  awkward repeated pauses.
- Keep exact client source-text validation and per-user message ownership.
- Key hosted cache lookup/commit by the projected speech text so old artifacts
  generated from raw markup miss and are replaced without a migration.
- For browser/local providers, use the rendered answer output's `innerText`
  rather than raw `message.content`.
- If the visible/projected text is empty, fail before provider I/O with a clear
  localized error path.
- Preserve the tab-scoped single playback owner and AbortSignal propagation.
- In the deployed Server mode, hide legacy Local-mode ElevenLabs/Mimo/model
  credentials and provider choices without deleting their rollback code.
- Normalize persisted Local-mode voice provider selections to the configured
  server default, or Browser speech when no server default is available.

## Acceptance Criteria

- [x] The reproduced weather answer speaks `西安天气`, weather prose, travel
      guidance, official-page labels, and visible source labels without
      speaking `##`, `**`, `<div>`, `style`, CSS values, closing tags, or hidden
      URL destinations.
- [x] Hosted requests still reject a stale/different raw source before provider
      I/O.
- [x] An existing cache row keyed from raw markup is not reused; the cleaned
      artifact replaces it through the existing cleanup queue.
- [x] Browser/local TTS receives normalized `innerText` from the rendered answer
      output.
- [x] Markdown/GFM, mixed raw HTML, links/images, code, tables, unsafe HTML,
      Unicode/emoji, whitespace, and markup-only input have focused tests.
- [x] Frontend format/lint/typecheck/tests/build, backend vet/tests, and the full
      standalone gate pass.
- [x] The rebuilt deployment is healthy and the reproduced answer no longer
      speaks hidden markup.
- [x] Server-mode Voice settings expose SiliconFlow/Browser/server defaults but
      do not expose legacy Local-mode API keys or provider choices.
- [x] Persisted Local-mode provider selections cannot leave the Server-mode UI
      on a hidden or unsupported provider.

## Verification

- `go test ./internal/voicejobs -count=1` passes the exact weather fixture,
  stale-source, readable cache-digest, markup-only, and visible-code cases.
- Frontend focused tests pass hosted raw-source preservation, Browser rendered
  text, and empty-readable-text completion.
- `bash mm-chat/scripts/verify-standalone.sh --full` passes against the final
  source: Frontend 954 tests, Backend all packages, and RAG 1906 passed / 7
  skipped.
- Rebuilt `app` profile is healthy at `http://127.0.0.1:18080`; `/` returns
  `200` and `/mm-api/ready` reports database, Redis, and storage ready.
- Server-mode Voice settings have focused composition coverage; the existing
  store test proves Local provider selections normalize to Server default or
  Browser speech. The rebuilt frontend container is healthy with this source.
- Manual proof completed: after hard refresh, the user replayed the persisted
  answer and confirmed that read-aloud works with the rebuilt deployment.

## Definition of Done

- One documented, tested hosted readable-text projection owns provider input.
- Frontend local/browser speech consumes rendered answer text.
- Source validation, cache concurrency, replacement cleanup, cancellation, and
  playback exclusivity remain intact.
- Specs document the visible-text and cache-digest contract.

## Technical Approach

- Add a focused readable-text projector in `backend/internal/voicejobs/` using
  Goldmark v1.8.4 with GFM AST traversal plus `x/net/html` for raw HTML text.
- In cached hosted synthesis, compare the client raw source to the persisted raw
  source first, then project readable text, pass only that projection to the
  executor, and use its digest for cache lookup/commit validation.
- Add a dedicated ref around `MessageOutputRenderer`; normalize its `innerText`
  at click time and pass it as optional local speech text while retaining raw
  source text for the hosted validation request.
- Extend service/API tests and frontend voice/integration tests with the exact
  reproduced markup fixture.

## Decision (ADR-lite)

**Context:** Sending client-visible text directly would match the browser but
would weaken the hosted ownership/cache boundary by allowing arbitrary paid TTS
input. Applying ad-hoc regexes would be fragile for nested Markdown and HTML.

**Decision:** Hosted TTS independently projects the owned persisted source with
an AST parser; browser/local TTS uses rendered `innerText`. The client continues
to submit raw source for stale-source validation.

**Consequences:** Hosted and browser projections share intent rather than an
identical implementation. Representative parity fixtures prevent drift. Visible
code is still read because it is visible; only hidden formatting/protocol syntax
is removed.

## Expansion Sweep

- Future evolution: pronunciation dictionaries and user-selectable inclusion of
  sources/code can be added after the visible-text boundary is stable.
- Related scenarios: copy/download retain raw Markdown; only read-aloud changes.
- Failure cases: markup-only content, stale source, cancellation, cache race,
  script/style payloads, and old-cache replacement are covered now.

## Out of Scope

- Pronunciation dictionaries, SSML, language detection, or reading-speed UI.
- A user preference to skip visible code, citations, or sources.
- Deleting legacy Local-mode provider implementation, changing audio encoding,
  storage schema, TTL/LRU, or playback controls.
- Changing copy, export, download, or normal message rendering behavior.

## Research References

- [`research/current-visible-text-boundary.md`](research/current-visible-text-boundary.md)
  — runtime trace, security/cache constraints, and the selected AST projection.

## Technical Notes

- Frontend entry: `mm-chat/frontend/src/components/chat/MessageItem.tsx`.
- Render authority:
  `mm-chat/frontend/src/components/content/MessageOutputRenderer.tsx` and
  `MarkdownRendererClient.tsx`.
- Hosted authority: `mm-chat/backend/internal/voicejobs/service.go` and
  `cache_postgres.go`.
- Existing hosted contract: `.trellis/spec/backend/hosted-tts-production.md`.
