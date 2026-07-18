# Go Web Search Provider Design

## Goals

- Make Go the sole future owner of external search execution.
- Preserve the admitted legacy provider request/response shapes.
- Fail closed on invalid configuration, request, transport, status, size, or
  JSON while keeping errors credential/body-free.
- Produce one deterministic bounded result from exactly one selected provider.
- Keep the browser from selecting a provider or transmitting Search secrets.
- Admit model-built-in Search only through a proven Go model-provider
  capability.

## Non-goals

- No UI cutover, Key persistence, activation workflow, or connection test yet.
- No SearXNG/self-hosted HTTP path.
- No automatic provider fallback or multi-provider fan-out.
- No Gemini model-built-in search until a real Go Gemini runtime provider
  exists.
- No `[W]` citation minting or message persistence in this slice.

## Flow

```text
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
                 +-----------> search.results SSE
```

## Decisions

| Decision | Reason |
| --- | --- |
| Closed `Provider` interface | Callers cannot select multiple providers inside one request. |
| Server-owned `Resolver` | API bodies never carry provider IDs, base URLs, or Keys. |
| Validated `ActiveExecution` union | Exactly one external adapter or one admitted model-built-in capability is active. |
| Standard library only | Keeps the backend image and dependency surface unchanged. |
| Inject only `HTTPDoer` | Fixtures inspect exact HTTP without weakening production transport defaults. |
| HTTPS-only config | Removes the old self-hosted/plain-HTTP SearXNG exception. |
| Resolve and reject any non-public address before dialing | Blocks loopback/private/link-local DNS rebinding targets. |
| Disable redirects and environment proxy use | Prevents an admitted host from redirecting or proxying into a forbidden network. |
| Provider-order normalization | Preserves upstream relevance order while deduplicating and applying the caller cap. |
| Firecrawl Key optional in this slice | Matches legacy request behavior; G11.9F real connection tests decide activation. |
| OpenAI type differs from OpenAI Compatible | Only explicit `OpenAI` runtime providers receive the Responses Web Search capability. |
| Built-in sources use the common normalizer | OpenAI annotations/actions receive the same URL, size, dedupe, and result fences. |
| External route rejects built-in execution | Built-in tools require model generation and cannot masquerade as a standalone search API. |

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
- Provider schema drift currently fails or drops malformed rows. G11.9E live
  smoke will freeze any newly observed admitted shape.
- The normal API binary has no Search resolver until G11.9F; `/v1/search`
  therefore exists but fails closed with `SEARCH_NOT_CONFIGURED`.
- `search.results` SSE is intentionally not persisted yet; G11.9E.3 owns `[W]`
  citation minting, output blocks, reload parity, and frontend consumption.
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
