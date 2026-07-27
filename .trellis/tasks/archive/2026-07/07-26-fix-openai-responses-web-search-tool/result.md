# OpenAI Responses built-in Web Search repair result

## Outcome

- `1c41b47` replaced the rejected `web_search_preview` request with the current
  `web_search` tool contract and required execution for explicit built-in
  Search.
- Configured OpenAI-compatible aliases now restore the authoritative provider
  ID before capability lookup while unbound compatible providers retain the
  canonical alias.
- The request-only latest-user URL-source instruction, one bounded transient
  startup retry, final-output citation recovery, and role-correct Responses
  history encoding close the live first-turn and follow-up failures.

## Acceptance evidence

- The task PRD records all nine acceptance criteria as complete.
- Live gateway evidence proved legacy HTTP 400 versus current HTTP 200,
  successful administrator attestation, exact configured model resolution, ten
  persisted URL sources for `西安天气`, and a second successful two-turn replay.
- Temporary smoke conversations were deleted, and credentials/upstream bodies
  remained outside persisted output and errors.

## Reconciliation verification — 2026-07-27

- Focused OpenAI request/history/retry/redaction regressions remain present.
- `go vet ./...` and all focused affected Go packages passed.
- The post-change full standalone gate passed frontend `936 tests`, all backend
  tests/vet, and RAG `1,906 passed / 7 skipped`.
- The current standalone services reported healthy.

## Rollback

Revert `1c41b47` and clear the custom built-in Search selection if the upstream
gateway no longer accepts the current Responses contract. External Search is
independent.
