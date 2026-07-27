# Go Web Search Provider Contract

## 1. Scope / Trigger

G11.9E.1 ports the legacy external provider adapters from the temporary Next
route into a closed Go package. G11.9E.2 adds the server-owned active resolver,
execution service, authenticated Go route, and OpenAI Responses model-built-in
Search stream. It still does not persist a Search Key, remove frontend code,
or activate a provider in the normal API binary. G11.9E.3 cuts the frontend to
Go, persists bounded `[W]` artifacts, and deletes the legacy Next Search chain.

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
func (s *Service) Execute(context.Context, ActiveExecution, Request) (Result, error)
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
- runtime providers explicitly typed `OpenAI` implement built-in Search;
  `OpenAI Compatible` remains on Chat Completions unless an exact
  provider/model/protocol connection test attests `openai_responses`, after
  which that bound runtime uses the same built-in Search path;
- OpenAI built-in Search posts streaming Responses requests to `/responses`
  with the required `web_search` tool, normalizes URL citations and web-search
  action sources, deduplicates at most ten records, and emits `search.results`
  SSE;
- the provider-only request copy appends a bounded public-page/URL-citation
  requirement to the latest user item so vertical results without URLs do not
  silently erase Weather and similar Search citations. Persisted messages stay
  unchanged;
- Responses history encodes user text as `input_text` and assistant text as
  `output_text`. Encoding assistant history as `input_text` is invalid and can
  make otherwise healthy built-in Search fail only on browser follow-up turns;
- Responses startup retries the exact request once after 200 ms only for a
  transport failure, HTTP `408`, `429`, or `5xx`. Other `4xx`, cancellation,
  and in-stream failures are not retried, and no provider/model fallback occurs;
- completion parsing also reads final `response.output` items because a gateway
  may omit incremental citation events while retaining citations in the final
  response;
- chat resolves the active execution once. External Search uses `Execute` on
  that exact union; built-in selection uses the matching model capability;
- external results are injected as a total-bounded Web evidence section with
  `[W1]..[Wn]` markers. Built-in source events are cumulatively deduplicated;
- terminal messages persist one `type: "search"` output block and redacted
  `metadata.web` citation records. Completion/reload preserves source order,
  marker identity, images, and content bounds;
- `search.results` is consumed by the typed server-mode frontend and terminal
  `message.completed` replaces the draft with the persisted artifact;
- the frontend cannot choose a Search provider or send a Search Key/Base URL.
  Availability comes only from Go `/v1/config`;
- the legacy Next route, external adapter/service/policy, client preflight,
  browser Search secrets, and self-hosted provider types are absent.

## 4. Validation & Error Matrix

| Condition                                                  | Result                                            |
| ---------------------------------------------------------- | ------------------------------------------------- |
| Unknown provider, missing required Key, unsafe base URL    | `ErrInvalidConfig`                                |
| Empty/oversized query, invalid scope/limit                 | `ErrInvalidRequest`                               |
| Resolver missing                                           | `ErrNotConfigured` / HTTP `SEARCH_NOT_CONFIGURED` |
| Resolver fails with internal detail                        | redacted `ErrResolutionFailed`                    |
| Resolver returns zero, mixed, or unsupported execution     | `ErrInvalidConfig`                                |
| Built-in execution sent to `/v1/search`                    | HTTP 409 `MODEL_BUILTIN_SEARCH_REQUIRES_CHAT`     |
| Built-in OpenAI selected with a non-capable model provider | HTTP 501 `MODEL_BUILTIN_SEARCH_UNSUPPORTED`       |
| Request build/transport failure                            | redacted `ProviderError`                          |
| Non-2xx provider status                                    | `UPSTREAM_STATUS` plus numeric status             |
| Non-JSON or non-identity response                          | `RESPONSE_*_INVALID`                              |
| Response exceeds 5 MiB                                     | `RESPONSE_TOO_LARGE`                              |
| Malformed/trailing JSON                                    | `RESPONSE_DECODE_FAILED`                          |
| Invalid/duplicate/oversized result row                     | row dropped, remaining order retained             |

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
- E.3 tests external prompt injection, one resolver/provider call, cumulative
  built-in results, `[W]` output blocks, metadata, typed SSE consumption,
  frontend reload rendering, and live Postgres JSONB round-trip;
- no network or provider quota is consumed in E.1 or E.2. E.3 performs one
  owner-authorized real Firecrawl credential-rejection smoke and one configured
  gateway capability probe without exposing credentials or falling back.

## 7. Wrong vs Correct

Wrong: copy the Next route into Go while continuing to trust redirects,
environment proxies, arbitrary HTTP base URLs, unbounded JSON, or automatic
multi-provider fallback.

Correct: keep each provider behind one closed adapter, centralize the hardened
transport and normalizer, resolve the sole active execution on the server,
distinguish OpenAI from OpenAI Compatible by proven runtime capability, persist
bounded chat-owned `[W]` artifacts, and defer encrypted administrator secrets
plus positive activation tests to G11.9F.

## 8. Rollback / Next Gate

Rollback reverts the E.3 commit as one unit: restore the legacy route/UI only
with its original E.2 code, remove chat Web output-block construction, and keep
the normal binary resolver unset. No schema migration was added; already
persisted Search output blocks are ordinary JSONB and remain readable as
unknown output blocks by older code.

G11.9F next adds encrypted Postgres administrator configuration, a Docker
Secret master key, exactly-one-active validation, and positive bounded real
connection tests. G11.9G owns conditional Knowledge/Web/model fusion.
