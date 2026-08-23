# Fix Web URL Reading and Partial Search Status

## Goal

Allow the Agent to read a user-supplied public URL directly and safely instead
of relying only on keyword search, beginning with Discourse topics such as
Linux.do. Correct turn-level search status so partial success is represented as
partial success rather than “no network results.”

## What I Already Know

- The current Agent invoked Tavily three times for one request: two searches
  succeeded with sources and one failed at the 30-second boundary.
- The UI displayed a global “network search unavailable / no network results”
  warning despite successful source-bearing calls.
- A normal request to the linked Linux.do HTML route receives a Cloudflare 403.
- Linux.do is Discourse and exposes the same topic through
  `/t/topic/{topicId}.json`; that endpoint returned the topic post stream, from
  which post number 14 was read.
- Direct URL retrieval is untrusted network and untrusted content. It requires
  SSRF, redirect, response-size, MIME, timeout, and indirect prompt-injection
  controls.

## Assumptions

- URL reading belongs in the existing Server-owned Agent Tool/runtime path.
- Search and URL reading should produce the existing source/citation contract
  instead of creating a second rendering format.
- Discourse JSON is a narrow adapter; a bounded generic public-page reader is
  still required as fallback.

## Requirements

- Detect explicit HTTP(S) URLs in the user request and make a direct URL-read
  capability available to the Agent.
- Normalize supported Discourse topic/post URLs to the public topic JSON API
  and select the requested post number when present.
- Fall back to a safe bounded HTML/text fetch for ordinary public URLs.
- Block loopback, private/link-local/reserved destinations and unsafe redirect
  hops; permit only HTTP(S), resolve DNS safely, enforce time and byte limits,
  and never forward browser/session/provider credentials.
- Treat retrieved page content as untrusted evidence, not executable
  instructions.
- Reuse existing source/citation structures and preserve the final canonical
  URL.
- Aggregate multiple search/read calls at turn level:
  - success when at least one call returned usable sources and none failed;
  - partial when usable sources exist and at least one call failed;
  - unavailable only when no call returned usable sources.
- A partial warning must state that some retrievals failed and report/use the
  successful sources; it must not claim the answer had no network results.
- Bound retries so one failed provider does not repeatedly consume 30 seconds.

## Acceptance Criteria

- [x] A Linux.do `/t/topic/{id}/{post}` URL returns the requested Discourse
      post content through its JSON endpoint even when the HTML route is
      Cloudflare-blocked.
- [x] A normal public HTML URL returns bounded readable text and canonical
      source metadata.
- [x] Private, loopback, link-local, non-HTTP(S), credential-bearing, unsafe
      redirect, oversized, and invalid responses fail closed.
- [x] Retrieved instructions cannot change Agent authority or trigger tools by
      themselves.
- [x] Mixed successful and failed retrieval calls produce a partial warning,
      not a no-results warning.
- [x] All-failed retrieval still produces the existing unavailable fallback.
- [x] Regression tests cover URL normalization, SSRF/redirect boundaries,
      response limits, Discourse extraction, aggregation, durable replay, and
      frontend rendering.

## Definition of Done

- Backend/runtime and frontend tests pass for the changed contracts.
- Relevant full component gates and production builds pass.
- Deployment changes only affected services and preserves runtime data.
- A deployed Linux.do post read and mixed-result status are verified.
- Specs and operational behavior are documented.

## Out of Scope

- A general browser automation or JavaScript-rendering crawler.
- Bypassing authenticated/private sites or Cloudflare challenges.
- Persisting arbitrary fetched pages into the Knowledge base automatically.
- Supporting non-HTTP protocols.

## Technical Notes

- User example: `https://linux.do/t/topic/2797040/14`.
- Successful diagnostic endpoint:
  `https://linux.do/t/topic/2797040.json` →
  `post_stream.posts[post_number == 14].cooked`.
- HTML from external sites must be sanitized/converted to text before entering
  model context.

## Research Decision

- Implement the bounded provider-Extract plus server-owned safe-reader hybrid
  described in `research/url-reading-options.md`.
- Do not enable ambient proxy use or browser automation in the MVP.
- Reuse `internal/safenet` and the current Web source/citation projection.
- Derive turn status from independent success/failure observations instead of
  letting the last failure overwrite earlier usable evidence.

## Verification

- `bash mm-chat/scripts/verify-standalone.sh --full` passed: Backend full test
  suite, Frontend format/lint/typecheck/969 tests/build, and RAG
  Ruff/mypy/1910 tests (7 external integration skips).
- The URL-reader security scan passed with zero findings; quality checks passed.
- Live images `mm-chat/backend:web-url-reader-74519dea-20260823T173826Z` and
  `mm-chat/frontend:web-url-reader-74519dea-20260823T173826Z` are healthy.
- A read-only one-off smoke using the deployed Backend environment and stored
  Tavily provider returned one source for
  `https://linux.do/t/topic/2797040/14`, preserved that canonical URL, and
  matched the requested post's `仅靠shell` content.
- Database, Redis, MinIO, RAG Worker, Memory Worker, and MCP Runner container
  IDs remained unchanged during the targeted Backend/Frontend rollout.
