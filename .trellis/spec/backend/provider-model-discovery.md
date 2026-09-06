# Supplemental provider model discovery

## 1. Scope

Apply when changing administrator model refresh, candidate catalogs or paid
availability probes. Source: `runtimeconfig/provider_model_discovery.go` and
`mm-chat/docs/contracts/provider-model-discovery.md`.

## 2. Signatures

`POST /v1/admin/providers/{id}/discover` ->
`{provider, models, discoveredModels?}`; uses the existing authenticated
ownership/vault/connection CAS path. No migration.

## 3. Contracts

Only explicit refresh probes missing GPT text candidates. Remote metadata plus
bundled fallback provide names, never availability evidence. Three candidates,
32 output tokens each, eight seconds per probe, 25 seconds for the probe phase.
Cache positive/negative results for 24 hours/15 minutes with owner, row and
credential/configuration binding. Never send user data, credentials to metadata,
follow redirects, or probe on passive listing/test/activation. Preserve selected
models across refresh and await admin PUT before UI success.

## 4. Error matrix

- Listing failure: fail without probes.
- Catalog failure: fallback candidates.
- Invalid/mismatched/empty/error completion: no promotion.
- Config changed while probing: CAS rejects.
- Cancellation: stop request and no late UI success/save or negative cache.

## 5. Cases

Good: hidden model replies through the actual chat protocol and is saved.
Base: listed models pass straight through.
Bad: importing a private Codex cache or trusting catalog names as availability.

## 6. Tests

Offline success/negative/cache/rotation/size/redirect/cancellation/CAS tests,
unauthenticated HTTP denial, merge/client Vitest and discover-save-reload browser
coverage. Live test authorization does not allow real user content in probes.

## 7. Wrong vs correct

Wrong: refresh intersects selection with `/models`, losing hidden callable IDs.
Correct: keep explicit selections and add only server-verified candidates.
