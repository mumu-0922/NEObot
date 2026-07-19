# Go Web Search Providers

`websearch` is the server-owned Search execution boundary for G11.9E/F. It ports
the legacy Tavily, Firecrawl, Exa, and Bocha adapters into Go, resolves exactly
one active server-side execution, and exposes the authenticated `POST
/v1/search` contract without accepting browser-supplied credentials.

## Responsibilities

- validate one bounded query, scope, and result limit;
- construct the provider-specific request and authentication headers;
- use an HTTPS-only, redirect-disabled, DNS/IP-fenced production client;
- bound and decode provider JSON without logging bodies or credentials;
- normalize, deduplicate, truncate, and cap source/image results;
- resolve exactly one active external or model-built-in execution;
- execute chat against that already-resolved selection without a second
  resolver read;
- return stable redacted errors and never fall back to another provider;
- keep model-built-in execution on the Go chat stream rather than the external
  search route.

The retired self-hosted search path is absent. OpenAI Responses Web Search is the only
currently admitted model-built-in capability because Go has no Gemini runtime
provider yet. Chat assigns bounded `[W<n>]` markers, persists a Search output
block plus redacted Web metadata, and restores the same artifact after reload.
G11.9F.3 stores administrator Search records in Postgres, encrypts Keys with
the provider vault, and activates one external provider atomically after a
bounded real connection test.

## Usage

```go
provider, err := websearch.NewProvider(websearch.ProviderTavily, websearch.Config{
    APIKey: secret,
})
if err != nil {
    return err
}

service := websearch.NewService(myServerResolver{provider: provider})
result, err := service.Search(ctx, websearch.Request{
    Query: "latest Neo Chat release",
    Scope: websearch.ScopeNews,
    MaxResults: 5,
})
```

Production callers leave `Config.Client` nil. Tests may inject `HTTPDoer` to
exercise exact provider shapes without network or credential use.

`POST /v1/search` accepts only `query`, `scope`, and `maxResults`. The normal API
binary supplies the Postgres/vault-backed `Resolver`; the request cannot select
a provider, Key, or base URL. With no active external record, only a tested,
enabled explicit OpenAI model provider may supply model-built-in Search.

Administrator configuration is exposed separately at
`/v1/admin/search/providers`. Multiple external providers may be saved, but
activation tests the exact stored endpoint/ciphertext and disables every other
external Search record in the same Serializable transaction.

## Public API

- `NewProvider(ProviderID, Config) (Provider, error)`
- `NewService(Resolver) *Service`
- `Service.ResolveActive(context.Context) (ActiveExecution, error)`
- `Service.Execute(context.Context, ActiveExecution, Request) (Result, error)`
- `Service.Search(context.Context, Request) (Result, error)`
- `POST /v1/search`
- `Provider.Search(context.Context, Request) (Result, error)`
- providers: `tavily`, `firecrawl`, `exa`, `bocha`
- execution modes: `external`, `model-built-in`
- admitted model-built-in provider: `openai`
- scopes: `general`, `news`, `academic`
- stable errors: `ErrInvalidConfig`, `ErrInvalidRequest`, `ErrNotConfigured`,
  `ErrResolutionFailed`, `ErrModelBuiltInRequiresChat`, `ProviderError`

See [DESIGN.md](DESIGN.md) for trust boundaries and tradeoffs.
