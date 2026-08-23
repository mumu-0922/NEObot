# URL Reading Options

## Question

How should the Chat Agent read an explicit public URL when ordinary Web search
cannot reliably surface or fetch that exact page?

## Evidence

- The HTML route `https://linux.do/t/topic/2797040/14` is protected by
  Cloudflare and rejects an ordinary HTTP client.
- Linux.do runs Discourse. Its public topic JSON route
  `https://linux.do/t/topic/2797040.json` returns the topic title and
  `post_stream.posts`; selecting `post_number == 14` returned the exact post
  body on 2026-08-24.
- Tavily documents a separate Extract API for caller-selected URLs. It can
  return cleaned Markdown/text and report failed URLs independently, but it
  consumes provider credits and makes direct reading depend on the selected
  external Search provider.
- The repository already owns DNS/IP/redirect/response-size controls in
  `internal/safenet` and already converts Web evidence into sources and
  citations in the Chat Tool Loop.

## Options

### A. Server-owned safe reader with narrow site adapters

Add `read_web_url`, validate the URL with `safenet`, use a narrow Discourse
adapter for public topic JSON, and use bounded HTML/plain-text extraction for
ordinary pages. Project the result into the existing Web source/citation path.

Advantages:

- Works independently of Search-provider quotas.
- Keeps URL, redirect, DNS, byte, MIME, timeout, and credential policy under
  server authority.
- Produces deterministic offline fixtures for security and parsing tests.
- Solves the demonstrated Linux.do route without browser automation.

Costs:

- Site-specific adapters require maintenance.
- Generic fetch cannot render JavaScript-heavy pages or bypass access controls.

### B. Provider Extract API

Call Tavily Extract (or an equivalent provider endpoint) with the explicit
URL.

Advantages:

- Better extraction for dynamic or complex pages.
- Less local HTML parsing.

Costs:

- Requires provider-specific implementation, credentials, quota, and latency.
- Does not remove the need for local URL admission controls.
- The current incident includes a provider timeout, so it is not a reliable
  sole path.

### C. Browser/MCP automation

Use a browser-capable MCP Tool to navigate and extract the page.

Advantages:

- Handles JavaScript and interactive pages.

Costs:

- Much larger authority and attack surface.
- Slower, optional at deployment time, and inappropriate as the default read
  path for one public article.

## Decision

Use a bounded A+B hybrid for the MVP. The active provider's exact-URL Extract
capability is preferred when present; the server-owned safe reader remains the
fallback. Keep C as an explicit separately-authorized Tool. Neither path may
bypass login, anti-bot, or private-network controls.

This changed after a real egress proof on 2026-08-24: the WSL host could read
Linux.do only through its configured ambient proxy, while both the host's
no-proxy request and the Backend container's direct route timed out. The
container DNS answer was public-shaped but unusable. Enabling an ambient proxy
inside `safenet` would weaken its DNS-binding guarantee, so the implementation
instead uses Tavily's documented Extract endpoint through the already-selected
and credentialed external provider. URL admission remains local and the
provider result still passes the normal source bounds. If Extract is unavailable
or fails, the direct safe reader is attempted without any environment proxy.

## Aggregation Decision

Track successful and failed Web retrieval calls separately for the turn.
Derive the final state from both facts:

- sources and no failure: `completed`;
- sources and any failure: `partial`;
- no sources and failure: `degraded`;
- no sources and no failure: `no_results`.

The durable diagnostic remains free of raw URLs, queries, source bodies, and
provider errors. The frontend renders `partial` with accurate copy and retains
the existing unavailable copy for `degraded`.
