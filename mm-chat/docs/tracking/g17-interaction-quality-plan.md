# G17 Interaction Quality Plan

Status: complete. G17 repairs auxiliary text generation, removes avoidable
marketplace interaction cost, and adds safe multi-document deletion without
changing the single-user product model.

## Execution rule

- Deliver one bounded slice at a time.
- Run focused tests and the affected quality gates after every slice.
- Commit every completed slice before starting the next one.
- Keep provider secrets, chat data, and Knowledge source content out of logs,
  fixtures, and commits.

## G17.1 Server-routed auxiliary text generation

- Add an authenticated Go endpoint for bounded, non-persistent text
  generation.
- Resolve Server Default, server-stored, and BYOK runtime providers through the
  same authority used by chat streaming.
- Route prompt polishing, message polishing, assistant/workspace prompt
  optimization, artifacts, and legacy client compression through that endpoint
  in server mode.
- Preserve the old Next route only for local mode.
- Reject missing/oversized input, bound provider output, and never expose raw
  provider errors.

## G17.2 Marketplace interaction performance

- Profile the assistant, skill, and plugin list/detail/edit surfaces before
  changing them.
- Remove large fixed backdrop blur and unnecessary paint-heavy effects.
- Stabilize filtered/paged collections and card callbacks where rerenders are
  measurable.
- Add safe off-screen rendering containment to repeated cards and use proper
  nested scroll boundaries.
- Apply the same low-risk pattern to equivalent marketplace/settings surfaces;
  do not redesign the UI or alter behavior.

## G17.3 Knowledge document bulk deletion

- Add per-document selection and select-all for the currently visible document
  list.
- Show selected count and require one explicit confirmation.
- Reuse the existing authoritative single-document DELETE endpoint with
  bounded concurrency.
- Remove successful items immediately, retain failed items selected, and show a
  concise success/failure result.
- Clear selection when the active collection changes or the list refreshes.

## Verification

- Focused Go and Vitest coverage for each slice.
- Frontend format, lint, typecheck, tests, and production build.
- Backend `gofmt`, `go vet`, and `go test ./...` for backend changes.
- Rebuild affected Compose services and perform deployed browser/API smoke.

## Rollback

Each slice is independently revertible. G17.1 adds no persistence; G17.2 is
rendering-only; G17.3 reuses the existing deletion contract and adds no schema.
