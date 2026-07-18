# Go Web Search Provider Contract

## 1. Scope / Trigger

G11.9E.1 ports the legacy external provider adapters from the temporary Next
route into a closed Go package. This slice defines provider request/response and
outbound-network safety only. It does not expose a production route, persist a
Key, remove frontend code, or implement model-built-in search.

## 2. Signatures

```go
type Provider interface {
    ID() ProviderID
    Search(context.Context, Request) (Result, error)
}

func NewProvider(id ProviderID, config Config) (Provider, error)
```

```go
type Request struct {
    Query      string
    Scope      Scope // general | news | academic
    MaxResults int   // 1..10, zero defaults to 5
}

type Result struct {
    Sources []Source
    Images  []Image
}
```

Closed provider IDs are `tavily`, `firecrawl`, `exa`, and `bocha`. SearXNG is
not admitted.

## 3. Contracts

- exactly one `Provider` instance executes one request; adapters never fan out
  or fall back to another provider;
- Tavily uses Bearer auth and `/search`; Firecrawl uses optional Bearer auth and
  `/v2/search`; Exa uses `x-api-key` and `/search`; Bocha uses Bearer auth and
  `/v1/web-search`;
- provider-specific payloads retain the legacy search depth, topic/category,
  image, Markdown, freshness, and result-count behavior;
- production configuration is HTTPS-only with no userinfo, query, fragment,
  localhost, or private literal IP;
- production dialing resolves the hostname, rejects the whole resolution set
  if any address is non-public, disables environment proxies and redirects,
  and dials a checked address;
- requests accept only bounded query/scope/result values and responses accept
  only one identity-encoded JSON value up to 5 MiB;
- normalized sources/images preserve upstream order, reject unsafe URL shapes,
  strip fragments, deduplicate by URL, truncate fields, and enforce the caller
  result cap;
- errors contain provider ID, stable code, and optional status only; no Key,
  upstream body, raw query, or transport error crosses the boundary;
- `Config.Client` injection is test-only. Production callers leave it nil and
  receive the hardened HTTP client.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Unknown provider, missing required Key, unsafe base URL | `ErrInvalidConfig` |
| Empty/oversized query, invalid scope/limit | `ErrInvalidRequest` |
| Request build/transport failure | redacted `ProviderError` |
| Non-2xx provider status | `UPSTREAM_STATUS` plus numeric status |
| Non-JSON or non-identity response | `RESPONSE_*_INVALID` |
| Response exceeds 5 MiB | `RESPONSE_TOO_LARGE` |
| Malformed/trailing JSON | `RESPONSE_DECODE_FAILED` |
| Invalid/duplicate/oversized result row | row dropped, remaining order retained |

## 5. Good / Base / Bad Cases

- Good: the selected provider returns ordered web and image records; Go emits a
  bounded deduplicated `Result` with no provider-specific fields.
- Base: Firecrawl has no Key, sends the same legacy unauthenticated shape, and
  activation is deferred to G11.9F's real connection test.
- Bad: a custom endpoint resolves to loopback/private/link-local, redirects,
  returns compressed/non-JSON/oversized bytes, or includes an upstream secret;
  the call fails closed and the error remains redacted.

## 6. Tests Required

- fixture tests assert exact endpoint, auth header, and request shape for all
  four providers;
- response fixtures assert Markdown/snippet fallbacks, images, fragments,
  duplicate removal, private literal result removal, and result caps;
- configuration/request tables cover missing Keys, localhost/private/plain
  HTTP/userinfo/query endpoints, scope, query, and limit bounds;
- upstream status, transport, JSON, encoding, and size failures must be stable
  and body/credential-free;
- focused coverage, `go vet`, full backend tests, and backend source build pass;
- no network or provider quota is consumed in E.1.

## 7. Wrong vs Correct

Wrong: copy the Next route into Go while continuing to trust redirects,
environment proxies, arbitrary HTTP base URLs, unbounded JSON, or automatic
multi-provider fallback.

Correct: keep each provider behind one closed adapter, centralize the hardened
transport and normalizer, inject fixtures for tests, and defer routing, secrets,
model-built-in streams, `[W]` citations, and UI deletion to their explicit
later gates.

## 8. Rollback / Next Gate

Rollback is deletion of `backend/internal/websearch`; no route or persisted
state depends on it yet. G11.9E.2 must wire the Go execution/service boundary
and model-built-in search without weakening this transport contract.
