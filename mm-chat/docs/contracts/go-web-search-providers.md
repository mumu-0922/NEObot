# Go Web Search Provider Contract

## 1. Scope / Trigger

G11.9E.1 ports the legacy external provider adapters from the temporary Next
route into a closed Go package. G11.9E.2 adds the server-owned active resolver,
execution service, authenticated Go route, and OpenAI Responses model-built-in
Search stream. It still does not persist a Search Key, remove frontend code,
mint/persist `[W]` citations, or activate a provider in the normal API binary.

## 2. Signatures

```go
type Provider interface {
    ID() ProviderID
    Search(context.Context, Request) (Result, error)
}

func NewProvider(id ProviderID, config Config) (Provider, error)
```

```go
type Resolver interface {
    ResolveActive(context.Context) (ActiveExecution, error)
}

type ActiveExecution struct {
    Mode         ExecutionMode // external | model-built-in
    External     Provider
    ModelBuiltIn ModelBuiltInProviderID
}

func NewService(resolver Resolver) *Service
func (s *Service) ResolveActive(context.Context) (ActiveExecution, error)
func (s *Service) Search(context.Context, Request) (Result, error)
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
not admitted. The only admitted model-built-in ID is `openai`; it is executed
through `chat.ModelBuiltInSearchProvider`, not through `Service.Search`.

HTTP contract:

```http
POST /v1/search
Content-Type: application/json

{"query":"...","scope":"general|news|academic","maxResults":5}
```

Provider ID, base URL, API Key, and plaintext/encrypted secret fields are not
accepted in this request.

## 3. Contracts

- exactly one `Provider` instance executes one request; adapters never fan out
  or fall back to another provider;
- the trusted `Resolver` returns one validated union: external execution has
  exactly one non-nil `Provider`; model-built-in execution has exactly one
  admitted built-in ID and no external provider;
- the browser cannot select the active Provider; `/v1/search` accepts only the
  bounded query, scope, and result count;
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
- the service re-normalizes the selected adapter result before returning it;
- `POST /v1/search` runs behind the common session middleware and emits
  `Cache-Control: no-store`;
- selecting model-built-in execution on `/v1/search` returns
  `MODEL_BUILTIN_SEARCH_REQUIRES_CHAT`; it never falls back to an external
  adapter;
- only runtime providers explicitly typed `OpenAI` implement built-in Search;
  `OpenAI Compatible` remains on Chat Completions and returns
  `MODEL_BUILTIN_SEARCH_UNSUPPORTED` when selected for built-in Search;
- OpenAI built-in Search posts streaming Responses requests to `/responses`
  with `web_search_preview`, normalizes URL citations and web-search action
  sources, deduplicates at most ten records, and emits `search.results` SSE;
- built-in source events are transient in E.2. G11.9E.3 owns `[W]` markers,
  output blocks, persistence, reload, and frontend consumption.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Unknown provider, missing required Key, unsafe base URL | `ErrInvalidConfig` |
| Empty/oversized query, invalid scope/limit | `ErrInvalidRequest` |
| Resolver missing | `ErrNotConfigured` / HTTP `SEARCH_NOT_CONFIGURED` |
| Resolver fails with internal detail | redacted `ErrResolutionFailed` |
| Resolver returns zero, mixed, or unsupported execution | `ErrInvalidConfig` |
| Built-in execution sent to `/v1/search` | HTTP 409 `MODEL_BUILTIN_SEARCH_REQUIRES_CHAT` |
| Built-in OpenAI selected with a non-capable model provider | HTTP 501 `MODEL_BUILTIN_SEARCH_UNSUPPORTED` |
| Request build/transport failure | redacted `ProviderError` |
| Non-2xx provider status | `UPSTREAM_STATUS` plus numeric status |
| Non-JSON or non-identity response | `RESPONSE_*_INVALID` |
| Response exceeds 5 MiB | `RESPONSE_TOO_LARGE` |
| Malformed/trailing JSON | `RESPONSE_DECODE_FAILED` |
| Invalid/duplicate/oversized result row | row dropped, remaining order retained |

## 5. Good / Base / Bad Cases

- Good: the selected provider returns ordered web and image records; Go emits a
  bounded deduplicated `Result` with no provider-specific fields.
- Good built-in: the active execution is `model-built-in/openai`, the runtime
  model provider is explicitly `OpenAI`, Responses emits text plus grounded
  sources, and Go streams normalized `search.results` without using an external
  provider.
- Base: Firecrawl has no Key, sends the same legacy unauthenticated shape, and
  activation is deferred to G11.9F's real connection test.
- Base deployment: the normal API binary has no resolver yet, so the registered
  Go route returns `SEARCH_NOT_CONFIGURED` instead of reading an environment or
  browser Key.
- Bad: a custom endpoint resolves to loopback/private/link-local, redirects,
  returns compressed/non-JSON/oversized bytes, or includes an upstream secret;
  the call fails closed and the error remains redacted.
- Bad capability: active built-in Search meets an OpenAI-compatible-only model
  provider; Go returns the capability error and does not silently execute
  Tavily, Firecrawl, Exa, or Bocha.

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
- service/handler tests prove exactly one resolver call and one provider call,
  reject request-owned credentials, cover missing/invalid/built-in selections,
  and redact resolver failures;
- OpenAI Responses fixtures assert endpoint/auth/payload, image input,
  reasoning, text, citations, dedupe, usage, malformed frames, non-2xx
  redaction, ordinary Chat Completions preservation, and explicit capability
  typing;
- chat integration proves `useSearch` plus active `model-built-in/openai`
  invokes only the built-in stream and emits `search.results`; OpenAI Compatible
  rejects before creating an assistant message;
- common HTTP integration proves `/v1/search` is registered and protected in
  required-auth mode;
- no network or provider quota is consumed in E.1 or E.2.

## 7. Wrong vs Correct

Wrong: copy the Next route into Go while continuing to trust redirects,
environment proxies, arbitrary HTTP base URLs, unbounded JSON, or automatic
multi-provider fallback.

Correct: keep each provider behind one closed adapter, centralize the hardened
transport and normalizer, resolve the sole active execution on the server,
distinguish OpenAI from OpenAI Compatible by proven runtime capability, and
defer encrypted secrets, `[W]` persistence, UI cutover, deletion, and live spend
to their explicit later gates.

## 8. Rollback / Next Gate

Rollback removes the `/v1/search` registration and model-built-in handler option,
then reverts `OpenAIProvider` to the prior OpenAI-compatible Chat Completions
constructor. No persisted schema/state changed, the default binary has no
active Search resolver, and old frontend Search remains untouched.

G11.9E.3 must cut the frontend to the Go route/stream, assemble and persist
`[W]` citation artifacts, remove SearXNG and the legacy Next route, and run the
authorized real-provider smoke without adding cross-provider fallback.
