# Fix OpenAI Responses Web Search tool

## Goal

Restore model-built-in Web Search for the current OpenAI-compatible server
provider by emitting the current Responses API tool type accepted by its
upstream gateway.

## Runtime Evidence

- The saved `SERVER_DEFAULT` provider is `OpenAI Compatible`, model
  `gpt-5.6-sol`, protocol `openai_responses`, without a successful attestation.
- The exact production probe using `tools: [{"type":"web_search_preview"}]`
  returns HTTP 400 `upstream_error`.
- Holding model, input, streaming, and include fields constant while changing
  only the tool type to `web_search` returns HTTP 200, reaches
  `response.completed`, and emits Web-search source/citation events.
- Ordinary Responses requests and structured input already pass against the
  same endpoint and credential.
- A real chat turn selected the attested GPT model but degraded before provider
  I/O with `not_configured`. The server-default frontend model reference uses
  the canonical `openai_compatible` alias; the configured provider resolver
  preserved that alias instead of restoring the authoritative
  `SERVER_DEFAULT` provider ID, so model-built-in resolution queried a
  nonexistent provider record.
- After repairing provider identity, a live turn reached provider-managed Web
  execution but completed with `no_results`: the Responses request exposed the
  Web tool with the default `auto` choice, so the model could skip it even when
  the user explicitly selected model-built-in Search. The same weather request
  emits a completed Web-search call when `tool_choice` is `required`.

## Requirements

- Emit the current `web_search` Responses tool type for OpenAI model-built-in
  search.
- Keep the `/responses` path, streaming behavior, source includes, citation
  normalization, error redaction, and ordinary Chat Completions path unchanged.
- When an OpenAI-compatible runtime provider has an authoritative configured
  ID, resolve accepted canonical aliases back to that ID so downstream
  capability lookup uses the correct stored provider record.
- Treat the explicit model-built-in Search mode as a hard request to execute
  the sole Responses Web tool rather than merely making it available.
- Keep the stored conversation unchanged while requiring the provider request
  copy to search public pages and return accessible URL citations, avoiding
  provider-native Weather results that contain names but no URLs.
- Retry only transient Responses startup failures once; keep cancellation,
  other `4xx`, in-stream failures, and provider/model fallback unchanged.
- Recover citations from the final Responses output when incremental citation
  events are absent.
- Encode multi-turn Responses history with role-correct content parts:
  `input_text` for users and `output_text` for assistants.
- Update the focused request-contract regression test.
- Verify the backend unit suite and a real administrator attestation against
  the configured provider.

## Acceptance Criteria

- [x] Focused OpenAI provider tests assert `web_search` and pass.
- [x] Full backend `go test ./...` and `go vet ./...` pass.
- [x] The real built-in-search test completes with at least one source.
- [x] The provider attestation persists and the exact selected model becomes
      available for model-built-in Search.
- [x] No credential or upstream body is persisted or exposed.
- [x] A server-default canonical alias resolves back to `SERVER_DEFAULT` while
      an unbound provider still resolves to canonical `openai_compatible`.
- [x] A real chat turn reaches provider-managed Web execution and persists at
      least one source instead of `not_configured` degradation.
- [x] The exact browser query `西安天气` completes with ten persisted URL
      sources and no Search degradation.
- [x] The same query still completes with ten sources after a real prior
      assistant turn, matching the browser follow-up path.

## Out of Scope

- Changing external Tavily Search.
- Adding speculative fallback retries to legacy preview-only gateways.
- Changing provider credentials, models, Base URL, or ordinary chat behavior.

## Rollback

Revert the request tool type and clear the custom built-in-search selection;
external Tavily Search remains available independently.

## Debug Retrospective

- Category: cross-layer contract plus integration coverage gap. The frontend
  alias, configured provider identity, capability attestation, and provider
  request were individually valid-looking but were not proven as one flow.
- The first fix changed the rejected upstream tool type but could not repair
  the earlier provider-identity degradation. Restoring provider identity then
  exposed a second implicit assumption: `tool_choice=auto` does not guarantee
  execution when the user explicitly selected built-in Search.
- Prevention: the backend Tool Loop spec now fixes the exact Responses request,
  configured/unbound alias rules, regression assertions, and isolated real-chat
  replay required for this path.

## Verification Record

- The legacy `web_search_preview` request reproduced HTTP 400.
- Changing only the tool type to `web_search` produced HTTP 200,
  `response.completed`, and source/citation events.
- The deployed administrator attestation returned 10 normalized sources.
- PostgreSQL now contains a valid provider/model-bound attestation for
  `SERVER_DEFAULT` plus `gpt-5.6-sol`.
- Focused OpenAI provider tests, full `go test ./...`, and `go vet ./...`
  passed.
- The rebuilt Backend is healthy; direct health, readiness, and same-origin
  proxy probes all returned HTTP 200.
- The final isolated chat replay resolved the canonical frontend alias to
  `SERVER_DEFAULT`, emitted one `search.results` event, completed with ten
  normalized sources, and persisted one Search output block. Its temporary
  conversation was deleted and no prefixed smoke conversation remained.
- Browser evidence then exposed a second gateway edge: bare `西安天气` selected
  a provider-native Weather vertical whose only source record had `{type,name}`
  and no URL; an earlier attempt also received a transient non-success startup.
  After adding the provider-only URL-source directive plus bounded transient
  retry, the unchanged browser query completed with ten Citations, one Search
  output block, `webResolve=resolved`, and `webExecute=completed`.
- A second browser replay exposed the remaining first-turn blind spot: prior
  assistant history was incorrectly serialized as `input_text`, so the
  Responses gateway rejected only multi-turn requests. Role-correct
  `output_text` serialization now passes an isolated two-round replay with two
  completed assistants and ten final Citations.
