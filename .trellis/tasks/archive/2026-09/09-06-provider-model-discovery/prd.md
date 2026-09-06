# Supplement provider model discovery

## Goal

The administrator's Fetch Models action discovers callable GPT text models omitted
by an OpenAI-compatible provider's models endpoint. Sub currently omits
gpt-6-astra while a same-key chat completion succeeds.

## Requirements

- Explicit administrator refresh combines the upstream list with bounded,
  successful chat probes of candidates from the existing public metadata catalog.
- Use a small bundled fallback for catalog outages/lag, including gpt-6-astra.
- Scope the initial implementation to OpenAI/OpenAI-compatible GPT text models;
  other protocols retain normal listing. Do not trigger paid probes on page load,
  activation, or ordinary connection tests.
- Bound candidate count, time, response size and completion tokens. Cache results
  per provider connection fingerprint. Never send secrets to the metadata source,
  follow redirects, send user conversations, or expose provider error bodies.
- Preserve already selected models across list refreshes, automatically select
  newly verified models, and save using the existing administrator config path.
- Show a concise explanation of discovery and its small probe cost.
- Provide focused offline success/failure/cache/cancellation/credential isolation
  tests and frontend selection/client tests. Update the operational contract.

## Acceptance criteria

- A fake upstream omits a candidate but accepts its chat request: refresh returns
  it; rejected, malformed, redirected, or timed-out candidates are not promoted.
- Repeat refresh reuses bounded cached results; changing credentials cannot reuse
  an old connection's evidence. Ordinary list/test/activate never probe.
- Refresh and reload retain selected models and the newly verified model.
- Build, type checks, relevant tests pass; make a focused local commit.

## Decision

Reuse the project's basellm metadata source plus a bundled GPT candidate, rather
than accessing a developer's private Codex cache or guessing arbitrary IDs.
An administrator-only discovery endpoint keeps paid probes out of passive reads.
Use existing config persistence; no schema migration or default-model changes.

## Out of scope

Scheduled background scans, universal hidden-model enumeration, probing image,
audio, embedding models, and changing other provider protocols.

## Completion evidence

- Backend `go vet ./...`, `go test ./...`, focused race tests passed.
- Frontend lint/typecheck/changed-file formatting passed; focused 76 tests and
  discover-enable-save-refresh-reload Playwright passed. Full suite: 1027 passed,
  one pre-existing `processTrace` rendering performance-ratio failure. The same
  isolated test failed on clean baseline commit 474f9174 (154.8ms versus 107.6ms
  threshold); no performance assertion was changed or disabled.
- Both Docker images built successfully and were deployed only to local frontend
  and backend containers. Other container identities remained unchanged.
- Paired backup `pre-discovery-20260906` verified. Restored database matched all
  109 migration entries and row samples; temporary MinIO restore verified 135
  objects plus 5 Knowledge/2 MCP/12 Skill samples. Drill resources were removed.
- Live discovery through the frontend proxy found gpt-6-astra and gpt-5.6; both
  were saved and appear in public configuration. First scan 5.72s, cache 0.19s.
  Temporary smoke session was revoked/deleted. Existing model defaults unchanged.
- Retained rollback image tags: `mm-chat/backend:pre-discovery-20260906` and
  `mm-chat/frontend:pre-discovery-20260906`. Candidate tags use
  `provider-discovery-20260906`. No runtime secrets or backups enter Git.
- The live environment's two image references now pin the candidate tags, so
  subsequent Compose restarts retain this release. Its pre-release environment
  snapshot is `/tmp/mm-discovery-rollout/runtime.env` (0600); secrets unchanged.
