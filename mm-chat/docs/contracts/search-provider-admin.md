# Search Provider Administrator Contract

## 1. Scope / Trigger

G11.9F.3 makes external Search configuration server-owned. It applies to
Tavily, Firecrawl, Exa, and Bocha administrator settings, activation, runtime
resolution, backup/rotation, and the Search settings UI.

## 2. Signatures

```text
GET    /v1/admin/search/providers
PUT    /v1/admin/search/providers/{provider}
DELETE /v1/admin/search/providers/{provider}
POST   /v1/admin/search/providers/{provider}/test
POST   /v1/admin/search/providers/{provider}/activate
```

`provider` is exactly one of `tavily`, `firecrawl`, `exa`, or `bocha`.
Search execution remains `POST /v1/search`; chat requests never carry a Search
provider ID, endpoint, or credential.

## 3. Contracts

- each external provider has at most one active record per user, stored in
  `provider_configs` with a reserved record ID and `config.kind="search"`;
- multiple providers may be saved, but one atomic activation disables every
  other external Search record for the same user;
- API Keys cross the browser boundary only as BYOK ingress envelopes, are
  immediately re-encrypted under the Docker-Secret vault, and are exposed on
  reads only as `hasApiKey`;
- saving from the UI performs a real bounded test; activation repeats the test
  and commits the attestation plus active state atomically;
- the attestation binds record ID, Search provider, normalized base URL, and
  the exact encrypted secret reference. Changing endpoint or Key clears it and
  disables the provider;
- runtime resolves the single active, valid Postgres/vault external provider
  on every request. It never falls back to another external provider after an
  execution failure;
- if no external provider is active, an enabled and connection-tested explicit
  OpenAI model provider may supply built-in Web Search. This is a capability
  selection, not an external-provider failure fallback;
- provider settings have no `.env` authority. Infrastructure timeout, vault
  keyring, and database credentials remain deployment configuration.

## 4. Validation and Error Matrix

| Condition                                        | Result                                       |
| ------------------------------------------------ | -------------------------------------------- |
| Unknown provider, unsafe URL, malformed body     | `400 SEARCH_PROVIDER_CONFIG_UNSUPPORTED`     |
| Missing Key or vault unavailable                 | redacted `SEARCH_PROVIDER_SECRET_*` error    |
| Provider record absent                           | `404 SEARCH_PROVIDER_NOT_FOUND`              |
| Bounded real test fails                          | `502 SEARCH_PROVIDER_CONNECTION_TEST_FAILED` |
| Config changes during test                       | `409 SEARCH_PROVIDER_CONFIG_CHANGED`         |
| No active external or eligible built-in provider | `503 SEARCH_NOT_CONFIGURED`                  |
| Multiple/corrupt active rows                     | `503 SEARCH_RESOLUTION_FAILED`               |

Provider bodies, credentials, ciphertext, queries, and DNS details never cross
administrator or runtime error boundaries.

## 5. Good / Base / Bad Cases

- Good: save Tavily with a BYOK Key, test it, activate it, restart backend, and
  receive one normalized Search result through the same active record.
- Base: save several providers but leave them inactive; Search is unavailable
  unless the selected explicit OpenAI model supports built-in Search.
- Bad: activate Exa while Tavily is active and leave both enabled; the atomic
  commit must instead disable Tavily in the same transaction.
- Bad: active Tavily fails at runtime and silently falls back to Firecrawl or
  model-built-in Search; the request must fail with the active provider error.

## 6. Tests Required

- CRUD ownership, fixed provider IDs, empty-array JSON, Key redaction, and
  reserved-record filtering from model-provider APIs;
- test-versus-activate behavior, fingerprint invalidation, failed-test zero
  activation, and concurrent-change fencing;
- one-active Postgres transaction, reload persistence, vault decrypt after
  restart, rotation context, and isolated test database deletion;
- exact Tavily/Firecrawl/Exa/Bocha adapter authentication plus existing
  HTTPS/DNS/IP/redirect/timeout/response fences;
- frontend API mapping, BYOK context, saved-Key state, save-and-test,
  activation/deactivation, deletion, and concise accessible feedback;
- owner-authorized positive real provider test, `/v1/search`, chat `[W]`
  persistence/reload, backend restart, and no-fallback negative proof.

## 7. Wrong vs Correct

Wrong:

```ts
await fetch("/v1/search", {
  body: JSON.stringify({ provider, apiKey, query }),
});
```

Correct:

```text
administrator BYOK save -> Postgres/vault -> bounded activation
chat Search toggle -> Go active resolver -> exactly one execution -> [W]
```

The browser selects whether Search is enabled for the conversation; only Go
selects and authenticates the active Search execution.
