# Go Web Search Providers

`websearch` is the server-owned external-search boundary for G11.9E. It ports
the legacy Tavily, Firecrawl, Exa, and Bocha request/response adapters into Go
without exposing an HTTP route or storing provider credentials yet.

## Responsibilities

- validate one bounded query, scope, and result limit;
- construct the provider-specific request and authentication headers;
- use an HTTPS-only, redirect-disabled, DNS/IP-fenced production client;
- bound and decode provider JSON without logging bodies or credentials;
- normalize, deduplicate, truncate, and cap source/image results;
- return stable redacted errors and never fall back to another provider.

SearXNG is intentionally absent. Model-built-in search, backend route wiring,
administrator persistence, and `[W]` citations remain later slices.

## Usage

```go
provider, err := websearch.NewProvider(websearch.ProviderTavily, websearch.Config{
    APIKey: secret,
})
if err != nil {
    return err
}

result, err := provider.Search(ctx, websearch.Request{
    Query: "latest Neo Chat release",
    Scope: websearch.ScopeNews,
    MaxResults: 5,
})
```

Production callers leave `Config.Client` nil. Tests may inject `HTTPDoer` to
exercise exact provider shapes without network or credential use.

## Public API

- `NewProvider(ProviderID, Config) (Provider, error)`
- `Provider.Search(context.Context, Request) (Result, error)`
- providers: `tavily`, `firecrawl`, `exa`, `bocha`
- scopes: `general`, `news`, `academic`
- stable errors: `ErrInvalidConfig`, `ErrInvalidRequest`, `ProviderError`

See [DESIGN.md](DESIGN.md) for trust boundaries and tradeoffs.
