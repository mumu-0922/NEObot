# Go Web Search Provider Design

## Goals

- Make Go the sole future owner of external search execution.
- Preserve the admitted legacy provider request/response shapes.
- Fail closed on invalid configuration, request, transport, status, size, or
  JSON while keeping errors credential/body-free.
- Produce one deterministic bounded result from exactly one selected provider.
- Keep the browser from selecting a provider or transmitting Search secrets.
- Resolve administrator state only from Postgres and the context-bound vault.
- Admit model-built-in Search only through a proven Go model-provider
  capability.

## Non-goals

- No browser-selected runtime provider, per-request Key, or provider fan-out.
- No retired self-hosted HTTP path.
- No automatic provider fallback or multi-provider fan-out.
- No Gemini model-built-in search until a real Go Gemini runtime provider
  exists.
- No conditional Knowledge/Web Router or RRF-style fusion until G11.9G.

## Flow

```text
administrator UI --BYOK--> Postgres/vault --bounded test--> atomic active row
                                            |
                                            v
authenticated POST /v1/search       Go chat stream with useSearch
             |                                  |
             +------------+---------------------+
                          v
                 server-owned Resolver
                          |
              exactly one ActiveExecution
                 /                    \
          external Provider        OpenAI built-in
                 |                 Responses Web Search
                 v                    |
       secure bounded HTTP             v
                 |              normalized source events
                 v                    |
       normalized Result               v
                 +-----------> cumulative search.results SSE
                                      |
                                      v
                         [W] output block + metadata
                                      |
                                      v
                           Postgres completion/reload
```

## Decisions

| Decision                                                 | Reason                                                                                                                         |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Closed `Provider` interface                              | Callers cannot select multiple providers inside one request.                                                                   |
| Server-owned `Resolver`                                  | API bodies never carry provider IDs, base URLs, or Keys.                                                                       |
| Validated `ActiveExecution` union                        | Exactly one external adapter or one admitted model-built-in capability is active.                                              |
| Standard library only                                    | Keeps the backend image and dependency surface unchanged.                                                                      |
| Inject only `HTTPDoer`                                   | Fixtures inspect exact HTTP without weakening production transport defaults.                                                   |
| HTTPS-only config                                        | Removes the old self-hosted/plain-HTTP SearXNG exception.                                                                      |
| Resolve and reject any non-public address before dialing | Blocks loopback/private/link-local DNS rebinding targets.                                                                      |
| Disable redirects and environment proxy use              | Prevents an admitted host from redirecting or proxying into a forbidden network.                                               |
| Provider-order normalization                             | Preserves upstream relevance order while deduplicating and applying the caller cap.                                            |
| Firecrawl adapter keeps legacy optional auth shape       | Administrator activation still requires a non-empty stored Key and a successful bounded test.                                  |
| Search config uses `kind="search"` reserved rows         | Model and Search records share storage without crossing resolution or vault contexts.                                          |
| Atomic single-active external provider                   | Serializable activation disables every other Search row for the same user.                                                     |
| OpenAI type differs from OpenAI Compatible               | Only explicit `OpenAI` runtime providers receive the Responses Web Search capability.                                          |
| Built-in sources use the common normalizer               | OpenAI annotations/actions receive the same URL, size, dedupe, and result fences.                                              |
| External route rejects built-in execution                | Built-in tools require model generation and cannot masquerade as a standalone search API.                                      |
| Resolve once, then `Service.Execute`                     | A request cannot switch active providers between capability selection and execution.                                           |
| Chat-owned `[W]` artifacts                               | Provider adapters remain transport-only while chat owns prompt markers, output-block shape, and message metadata.              |
| Built-in marker completion                               | Provider-emitted source annotations are known-used sources, so missing `[W]` markers are appended before terminal persistence. |

## Security Contract

- Query: non-empty, at most 2,048 bytes.
- Result limit: 1–10; default 5.
- API Key: trimmed, at most 4,096 bytes, never placed in errors.
- Response: identity JSON, at most 5 MiB, one JSON value, no body reflection.
- Source content: at most 64 KiB; title/URL/image description are separately
  bounded; invalid schemes, userinfo, localhost, and private literal IPs drop.
- Production DNS resolution rejects the entire host if any resolved address is
  not globally routable, then dials a checked address directly.
- `POST /v1/search` accepts only query/scope/limit, is protected by the common
  session middleware, and returns `Cache-Control: no-store`.
- Resolver failures are collapsed to stable errors; resolver details never
  cross the HTTP boundary.
- OpenAI Responses status/body/frame errors are redacted and never include Key,
  query, or upstream body.

## Known Limits

- Result URLs are displayed, not fetched; hostname DNS is therefore not
  resolved during normalization. Literal localhost/private IP results are
  rejected.
- Provider schema drift fails or drops malformed rows. A credentialless live
  Firecrawl call proved the hardened adapter reaches the real endpoint and
  returns a redacted 4xx `ProviderError`; owner-entered Tavily later passed the
  F3 positive Search/chat/restart path.
- The normal API binary resolves Postgres/vault state on every request. With no
  active external or eligible explicit OpenAI model provider, `/v1/search`
  fails closed with `SEARCH_NOT_CONFIGURED`.
- External Search runs whenever the existing Search toggle is enabled in this
  slice. Conditional routing and Knowledge-derived query planning remain
  G11.9G.
- Gemini remains unsupported because the Go runtime cannot currently execute a
  Gemini chat request. Capability checks return an explicit unsupported error
  instead of silently using another provider.

## Change History

### 2026-07-18 — G11.9E.1

Added the closed four-provider contract, secure client, normalization, and
fixture tests. No production route or credential authority changed.

### 2026-07-18 — G11.9E.2

Added the internal active resolver, one-provider Search service, authenticated
Go route, stable fail-closed errors, common built-in source normalization, and
OpenAI Responses Web Search stream capability. Explicit OpenAI providers may
emit transient `search.results`; OpenAI Compatible providers cannot. No Search
secret persistence, frontend cutover, real provider spend, or `[W]` persistence
entered this slice.

### 2026-07-18 — G11.9E.3

Added resolve-once external execution, bounded Web prompt context, cumulative
built-in source handling, `[W]` markers, Search output blocks, Web metadata,
Postgres completion/reload parity, typed frontend SSE consumption, and
server-owned Search availability. Removed the legacy Next route, browser
provider/Key/Base URL settings, client-side Search preflight, and the retired
self-hosted provider path. A real Firecrawl negative smoke passed the redacted
credential-rejection boundary; the configured OpenAI-compatible gateway
correctly failed its unsupported Responses Web Search probe without fallback.

### 2026-07-19 — G11.9F.3

Added fixed-provider administrator CRUD, BYOK-to-vault persistence, bounded
save/test and activation calls, one-active Serializable Postgres commit,
kind-aware secret rotation, dynamic availability, and the Search settings UI.
An isolated Postgres proof covered one-active reload and fresh-vault decryption;
the deployed API passed reversible no-Key CRUD/error cleanup. Owner-entered
Tavily then passed `/v1/search`, chat `[W]` persistence/reload, backend restart,
and forced external failure without fallback before exact state restoration.
