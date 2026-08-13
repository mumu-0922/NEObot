# MCP Marketplace Pagination and Four-Way Installation

## Goal

Deliver bounded Marketplace browsing and turn compatible entries into a secure,
recoverable administrator-managed install flow. Installation must route
server-authoritatively across anonymous HTTP, API-key/header HTTP, OAuth HTTP,
and npm-backed stdio Runner environment configuration without allowing an
ordinary user or browser request to choose executable metadata.

## Requirements

- Load Marketplace results in backend-authoritative pages of 20 items.
- Search and category changes start a fresh generation at page 1, clear the old
  list, and cancel/fence all older page requests.
- When a bottom sentinel approaches the scroll viewport, request exactly the
  next page and append it to the current generation.
- Allow only one next-page request at a time and deduplicate appended entries by
  Marketplace identifier.
- Display loaded count versus the server-reported total count so category
  totals are not mistaken for already-rendered cards.
- Keep a visible `Load more`/`Retry` control at the end as an accessibility and
  network-failure fallback; automatic loading may resume after a successful
  retry.
- Preserve installed-server state, detail/install flow, category facets,
  latest-request race fencing, icon lazy loading, and Marketplace authority.
- Keep initial-search, next-page, detail, and install errors scoped to their
  owning UI. In particular, a failed item-detail request must not present the
  already-loaded result list as though the entire Marketplace were unavailable.
- Keep rendering incremental and apply browser-native off-screen rendering
  containment to cards; do not fetch all pages eagerly and do not add a large
  virtualization dependency for this change.
- Route every Marketplace deployment through an explicit backend-owned install
  mode: direct anonymous HTTP, API-key/header HTTP, OAuth HTTP, or reviewed
  stdio Runner environment configuration.
- Return only bounded non-secret configuration requirements to the browser.
  Secrets are transient UI state, submitted once, encrypted by Backend, and
  never returned, persisted in browser state, embedded into endpoint URLs, or
  logged.
- Treat Marketplace metadata as discovery until the authenticated deployment
  administrator installs an exact deployment. Remote direct installs must not
  remain falsely ready when the live endpoint challenges for authentication.
  For npm stdio, Backend re-fetches the authoritative item detail, binds an
  exact registry package/version and argv to the deployment hash, persists the
  definition, and sends it only over the authenticated Runner control plane.
  Shell, Docker, Git, URL package specs, and browser-supplied commands remain
  blocked.
- Keep failed or incomplete installs as recoverable drafts with `needs_auth` or
  bounded validation state. Enable a Conversation only after validation reaches
  `ready`.
- For OAuth, use protected-resource/authorization-server discovery, PKCE, state,
  exact redirect validation, audience binding, encrypted tokens, refresh, and
  revocation. When the authorization server advertises a compatible Dynamic
  Client Registration endpoint, register Neo Chat automatically and retain the
  resulting client identity server-side; do not require the user to obtain a
  client ID manually or silently downgrade OAuth to anonymous/header auth.
- For a remote Server that needs both an endpoint URL and API key, keep them as
  separate inputs and authorities. Marketplace-owned URLs are normalized and
  locked by Backend; private custom URLs are entered by the user and pass the
  existing HTTPS/SSRF checks. API keys are transient secret inputs, stored only
  in the Backend vault, and injected through an approved request header. Never
  interpolate an API key into a persisted URL or return it to the browser.
- Only `AUTH_BOOTSTRAP_USER_ID` may create/install/delete/configure/authorize or
  validate MCP definitions. Definitions installed by that administrator are
  globally visible; ordinary users may only select ready definitions and
  enable/disable their tools for Conversations.
- For stdio configuration, accept only authoritative schema-declared secret
  environment names for the exact installed npm artifact, isolate the child
  process by installed Server plus credential fingerprint, and prevent
  secret-changing sessions from reusing the old process. The shared Runner
  starts packages on demand and reaps them; installation does not create one
  permanent container per MCP.
- Match LobeHub's current upstream authentication split: Marketplace search
  and category requests use the M2M bearer token, while public item-detail
  requests send no Marketplace credential. Keep the same fixed-origin,
  timeout, response-size, cache, identifier, and normalization boundaries.

## Acceptance Criteria

- [x] Anonymous HTTP installs, validates, and enables without credential UI.
- [x] API-key/header HTTP prompts only for backend-declared fields, stores the
      value encrypted, validates, and never places it in the endpoint URL.
- [x] OAuth HTTP creates a recoverable `needs_auth` Server, completes a secure
      authorization flow with automatic client registration when advertised,
      validates, and then enables it.
- [x] A remote URL plus API key is collected as two separate fields; the URL is
      validated, the key is encrypted and header-injected, and persisted/logged
      URLs contain no secret.
- [x] Administrator-selected npm stdio with required environment prompts for
      declared fields, pins an exact registry package/version, injects secrets
      only into the installed-server-scoped Runner child, and validates.
- [x] A non-administrator receives stable `403 MCP_ADMIN_REQUIRED` responses
      from every MCP definition/credential/OAuth management route, cannot see
      management controls, and can still select and execute ready shared MCPs.
- [x] An administrator-installed ready MCP is visible to another user without
      copying its credential; execution resolves the administrator-owned vault
      entry server-side.
- [x] Dynamic Runner requests never contain a plaintext command definition or
      secret, and the Runner rejects malformed/non-npx definitions.
- [x] A live 401 challenge is presented as authentication/configuration needed,
      never as anonymous-installable or generic network failure.
- [x] Create/credential/OAuth/Runner failures preserve an installed recoverable
      draft and do not mutate Conversation selection before `ready`.
- [x] DTOs, logs, persisted endpoint URLs, cache entries, and Runner identifiers
      contain no submitted secret values.
- [x] Initial search requests page 1 with page size 20.
- [x] Scrolling near the bottom requests pages 2, 3, and onward in order.
- [x] New pages append without duplicate identifiers or replacing prior pages.
- [x] The UI shows `loaded / total`, and stops when the last page is reached.
- [x] Rapid search/category changes cannot append stale pages into the new list.
- [x] Repeated observer callbacks cannot create parallel or duplicate page requests.
- [x] A next-page failure keeps existing cards and exposes a retry action.
- [x] Detail failure keeps the loaded list, offers a scoped retry, and never
      raises the global Marketplace-unavailable banner.
- [x] A live detail request succeeds without sending the search M2M bearer
      token upstream.
- [x] Focused Marketplace Vitest, ESLint, Prettier, and typecheck pass.

## Definition of Done

- Focused regression coverage proves reset, append, last-page, stale-request,
  duplicate-request, and retry contracts.
- Frontend MCP specification documents the pagination behavior.
- A new frontend image is built and deployed for manual verification; unrelated
  services are not recreated.

## Technical Approach

Track the active query/category generation, current page, total pages, and
next-page error in `McpMarketplace.tsx`. Use one `IntersectionObserver` rooted
at the Marketplace scroll container with an approximately 300 px bottom root
margin. The initial/reset request replaces items and authoritative facets;
subsequent requests append unique items and preserve existing cards on failure.
A request ID plus AbortController/generation checks fence stale completions.
Render an explicit bottom action as a fallback and use CSS
`content-visibility: auto`/intrinsic sizing on cards to reduce off-screen paint
cost without changing scroll semantics.

## Decision (ADR-lite)

**Context**: Category totals can be tens of thousands, but the current UI always
requests only page 1 and provides no navigation.

**Decision**: Use bounded infinite scrolling with a manual fallback rather than
traditional next/previous pagination or eager loading. Retain all loaded entries
within the active browse generation and use native rendering containment rather
than truncating items or introducing a virtualization dependency.

**Consequences**: Browsing stays continuous and network/memory growth follows
actual user scrolling. Extremely long sessions can still accumulate DOM nodes,
but images remain lazy and off-screen cards are paint-contained; true list
virtualization remains a future option if measured usage justifies it.

## Out of Scope

- Fetching every Marketplace result eagerly.
- Changing LobeHub counts, sorting, filtering semantics, or install compatibility.
- Adding a third-party virtualization library.
- URL-addressable page numbers or previous-page navigation.
- General application-wide RBAC or changing the existing Session role model.
- Browser/Chromium MCP profiles and packages that require host mounts,
  privileged containers, Docker sockets, or native system packages absent from
  the Runner image.

## Research References

- [`research/auth-install-routing.md`](research/auth-install-routing.md) — MCP auth/transport standards, Tavily live behavior, current Neo Chat gaps, and recommended routing.

## Technical Notes

- Primary component: `mm-chat/frontend/src/components/mcp/McpMarketplace.tsx`.
- Existing API already accepts `page`/`pageSize` and returns `page`,
  `totalPages`, and `totalCount`.
- Existing frontend race fencing uses `searchRequestRef`; pagination must extend
  it without allowing an older request to clear a newer loading state.
- Applicable contract: `.trellis/spec/frontend/mcp-tools.md`.
