# Go Web Search Provider Design

## Goals

- Make Go the sole future owner of external search execution.
- Preserve the admitted legacy provider request/response shapes.
- Fail closed on invalid configuration, request, transport, status, size, or
  JSON while keeping errors credential/body-free.
- Produce one deterministic bounded result from exactly one selected provider.

## Non-goals

- No route, UI, provider selection, Key persistence, or connection test yet.
- No SearXNG/self-hosted HTTP path.
- No automatic provider fallback or multi-provider fan-out.
- No model-built-in search; that must join the Go chat provider stream rather
  than pretending to be an external HTTP adapter.

## Flow

```text
future Go route/service
        |
        v
NewProvider(id, config) -> provider-specific request
        |                         |
        |                         v
        |               secure bounded HTTP client
        |                         |
        v                         v
stable error            provider JSON response
                                  |
                                  v
                       normalized Result{Sources,Images}
```

## Decisions

| Decision | Reason |
| --- | --- |
| Closed `Provider` interface | Callers cannot select multiple providers inside one request. |
| Standard library only | Keeps the backend image and dependency surface unchanged. |
| Inject only `HTTPDoer` | Fixtures inspect exact HTTP without weakening production transport defaults. |
| HTTPS-only config | Removes the old self-hosted/plain-HTTP SearXNG exception. |
| Resolve and reject any non-public address before dialing | Blocks loopback/private/link-local DNS rebinding targets. |
| Disable redirects and environment proxy use | Prevents an admitted host from redirecting or proxying into a forbidden network. |
| Provider-order normalization | Preserves upstream relevance order while deduplicating and applying the caller cap. |
| Firecrawl Key optional in this slice | Matches legacy request behavior; G11.9F real connection tests decide activation. |

## Security Contract

- Query: non-empty, at most 2,048 bytes.
- Result limit: 1–10; default 5.
- API Key: trimmed, at most 4,096 bytes, never placed in errors.
- Response: identity JSON, at most 5 MiB, one JSON value, no body reflection.
- Source content: at most 64 KiB; title/URL/image description are separately
  bounded; invalid schemes, userinfo, localhost, and private literal IPs drop.
- Production DNS resolution rejects the entire host if any resolved address is
  not globally routable, then dials a checked address directly.

## Known Limits

- Result URLs are displayed, not fetched; hostname DNS is therefore not
  resolved during normalization. Literal localhost/private IP results are
  rejected.
- Provider schema drift currently fails or drops malformed rows. G11.9E live
  smoke will freeze any newly observed admitted shape.
- Model-built-in grounding sources require separate OpenAI/Gemini stream
  adapters in the next slice.

## Change History

### 2026-07-18 — G11.9E.1

Added the closed four-provider contract, secure client, normalization, and
fixture tests. No production route or credential authority changed.
