# G12 Provider Protocols Plan

Status: complete. Gemini server chat and native Anthropic Claude are now
available as tested wire protocols. Provider presets remain explicitly out of
scope.

## Decision

- Keep the existing administrator-created provider workflow. An OpenAI-
  compatible provider remains a user-chosen name plus editable Base URL, API
  Key, and fetched model IDs; no vendor catalog or automatic preset is added.
- Keep provider credentials in the existing browser-encrypted ingress and
  Postgres provider-secret vault. No new model-provider secret is read from or
  written to `.env`.
- Treat `Gemini`, `OpenAI`, `OpenAI Compatible`, and `Anthropic` as wire
  protocols, not vendor marketing presets.
- Execute and commit one protocol slice at a time. A slice is not complete
  until focused tests, full backend/frontend gates, and the relevant runtime
  smoke pass.

## Slice sequence

### [x] G12.1 — Gemini server-runtime closure

- Route stored `Gemini` providers through a real Go chat Provider instead of
  returning `PROVIDER_CONFIG_UNSUPPORTED`.
- Use Google's OpenAI-compatible chat surface while retaining the existing
  native Gemini Models API connection test and administrator UI.
- Normalize Gemini service-root, `/v1beta`, and `/v1beta/openai` inputs without
  appending the OpenAI `/v1` suffix to the wrong path.
- Preserve streaming text, usage, current-branch history, image inputs, and
  tool planning through the existing Go Provider contract.
- Prove that OpenAI-compatible manual Base URLs still resolve exactly as before.

### [x] G12.2 — Native Anthropic Claude

- Add the `Anthropic` provider type to the Go runtime contract and existing
  frontend provider editor; do not add named Claude/vendor presets.
- Support the Anthropic Messages streaming API, system instructions,
  user/assistant history, supported base64 image inputs, usage, cancellation,
  bounded errors, and native tool planning.
- Test and activate provider configuration through the Anthropic Models API
  with `x-api-key` and a pinned `anthropic-version` header.
- Keep model-built-in Search unavailable; the existing server-owned external
  Web and Knowledge augmentation remains usable before the Anthropic request.
- Make transitional local-mode paths fail closed instead of accidentally
  treating Anthropic as Gemini.

## Deferred boundaries

- No provider preset registry or default-URL catalog.
- No Azure OpenAI, AWS Bedrock, OpenAI Codex OAuth, or GitHub Copilot OAuth.
- No Anthropic-native web-search tool. External server-owned Search remains the
  supported Web path.
- No provider secret environment fallback.

## Rollback

Each slice has its own commit. Revert the frontend/backend slice together.
Stored provider rows remain data-compatible; an older backend rejects the new
type rather than sending it through a different provider. Do not delete or
rewrite encrypted provider secrets during rollback.
