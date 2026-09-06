# Supplemental model discovery

## Scope

Administrator **Fetch Models** supplements incomplete OpenAI/OpenAI-compatible
GPT catalogs. A model can be callable even when `/v1/models` omits it. Discovery
is explicit, not a page-load/background/activation side effect.

## API and flow

`POST /v1/admin/providers/{id}/discover` uses the existing authenticated provider
ownership, vault resolution, connection test and compare-and-set commit path.
It returns `{provider, models, discoveredModels?}`. `models` includes the upstream
catalog and successful supplemental probes; `discoveredModels` contains the
verified additions (including valid cached evidence). Provider activation and
task default models are unchanged.

The frontend preserves selected models, enables verified additions, and saves
through the existing admin provider PUT before announcing success. A subsequent
refresh or reload must not discard a selected model merely because it is absent
from the upstream catalog. Cancellation is passed through discovery and saving.
The ordinary `/v1/providers/models`, `/test` and `/activate` actions never run
supplemental probes.

## Candidate and request boundaries

- Candidate source: the existing public metadata URL
  `https://basellm.github.io/llm-metadata/api/all.json`, cached for one hour.
  A bundled `gpt-6-astra` entry covers source outages. No developer Codex files
  or credentials are needed in production.
- Only well-formed, non-deprecated GPT text IDs are admitted; exclude image,
  audio, embeddings, search, Codex-only and pro variants. Compatible endpoints
  must already advertise a GPT text model. Gemini/Anthropic are listing-only.
- Sort by release date, newest first; exclude releases older than the newest
  listed candidate. Inspect at most three missing candidates per refresh.
- Catalog: 3-second timeout, 8 MiB response cap, no credentials. Probe stage:
  25 seconds total including the cancellable serialized queue; each call has an
  8-second timeout, 64 KiB response cap, and `max_completion_tokens: 32`.
- Probes call the configured `/v1/chat/completions` with one synthetic
  `Reply exactly OK.` message. No conversations, files or tools are included.
  Require a 2xx valid JSON completion, exact requested response model, one
  assistant text answer and `finish_reason: stop`. A proxy's silent fallback to
  another model is not discovery evidence. This proves text availability only.
- Never follow redirects or include upstream error bodies in client errors/logs.
  Provider credentials go only to the configured provider origin. HTTP remains
  supported for operator-configured local providers, as in ordinary listing.
- Process-local probe cache: success 24 hours, failure 15 minutes, at most 1024
  entries; partition by owner, row, provider ID/type/URL and encrypted credential
  fingerprint. Restart clears this cache. Cancellation does not create negative
  evidence. Cache expiry/eviction may incur another small probe charge.

## Failure matrix

| Condition | Result |
| --- | --- |
| Upstream listing fails | Existing connection-test error; no probes |
| Metadata unavailable | Bundled candidates; existing models remain usable |
| Candidate 404/429, timeout, redirect, malformed or empty completion | No promotion; original catalog still succeeds |
| Response names another model | No promotion |
| Key/URL/type changes during scan | Existing connection CAS rejects stale result |
| Request cancelled | Stop work; no late UI success/save |
| Selected model omitted on refresh | Preserve the selection |

## Verification

Backend offline tests cover verification, negative/positive caching, credential
rotation, bounded requests, metadata filtering, response limits, redirects,
cancellation and no probes on passive/test/activate paths. HTTP server tests
cover unauthenticated denial. Frontend tests cover merging and the dedicated
client endpoint; Playwright proves discover -> enable -> PUT -> refresh -> reload.

Good: a hidden GPT model is verified and remains selectable after refresh.
Base: ordinary upstream models are listed without extra probes for their IDs.
Bad: treating metadata entries as proof or issuing probes during page load.

## Rollback

Revert the discovery client/endpoint change together. No schema migration or
runtime secret changes are needed. Previously saved model IDs remain ordinary
provider selections and can be unchecked using the existing UI.
