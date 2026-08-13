# MCP Marketplace authentication/install routing research

## Sources inspected

- MCP Authorization specification, 2025-06-18:
  `https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization`
- MCP Transports specification, 2025-06-18:
  `https://modelcontextprotocol.io/specification/2025-06-18/basic/transports`
- Live LobeHub detail for `tavily-ai-tavily-mcp` on 2026-08-12.
- Tavily RFC 9728 protected-resource metadata and OAuth authorization-server
  metadata on 2026-08-12.
- Current Neo Chat Marketplace, OAuth, credential-vault, manifest, Runner, and
  private-server code/specs.

## Decisive findings

- MCP defines `stdio` and Streamable HTTP as the standard transports.
- Authorization is optional. HTTP MCP authorization should follow OAuth 2.1;
  stdio should obtain credentials from its process environment rather than use
  the HTTP authorization flow.
- MCP OAuth relies on authorization-server discovery, protected-resource
  metadata, PKCE, exact redirect registration, state validation, and resource
  audience binding. Dynamic Client Registration is part of the supported
  standards surface, but Neo Chat currently requires a preconfigured client ID
  and has no DCR implementation.
- Tavily's plain `https://mcp.tavily.com/mcp` returns `401` with protected-
  resource metadata. Its authorization server publishes PKCE and a dynamic
  registration endpoint. LobeHub nevertheless exposes that URL as a no-config
  HTTP option, so Marketplace metadata alone cannot safely classify it as
  anonymous.
- Tavily also advertises an API-key query-template HTTP option and config-
  requiring stdio options. Query-string secrets must not be persisted in
  endpoint URLs or emitted in logs.
- Neo Chat already has encrypted per-user `header` and `oauth` credential
  storage, OAuth discovery/PKCE/token refresh, and approved stdio artifacts.
  It lacks OAuth DCR, Marketplace auth hints, Marketplace credential ingress,
  and per-user Runner environment injection.
- The shared Runner intentionally accepts only an approved artifact ID. It has
  no database/vault access and currently receives neither browser credentials
  nor per-user environment. Passing raw secret values on each control request
  would weaken this boundary and make shared process reuse cross-user unsafe.

## Feasible approaches

### A. Server-authoritative install plan (recommended)

Backend normalizes each deployment into one explicit install mode:
`direct`, `header`, `oauth`, or `runner_env`, with bounded non-secret field
requirements. The browser collects only fields declared by that plan and sends
secrets once. Backend creates the private Server, encrypts credentials, then
validates/enables. OAuth returns a recoverable `needs_auth` Server and starts
authorization. Runner env uses a per-user/private-server process identity and
an internal encrypted/short-lived secret handoff, never a shared artifact-only
session.

Pros: correct authority boundary, one coherent UX, secure by construction.
Cons: widest implementation; Runner control protocol and process keying change.

### B. Remote-only first, Runner env later

Finish anonymous/header/OAuth now and leave config-requiring stdio disabled.

Pros: smaller and safer increment.
Cons: does not meet the requested four-way routing.

### C. Browser templates / arbitrary command configuration

Send raw LobeHub URLs, commands, arguments, and env values from browser to
Backend/Runner.

Rejected: allows Marketplace/browser metadata to become execution authority,
risks secret leakage in URLs/logs, and breaks the reviewed-artifact boundary.

## Superseding owner decision (2026-08-12)

The deployment owner explicitly selected a narrower multi-user authority model:

```text
AUTH_BOOTSTRAP_USER_ID: install/manage arbitrary npm-backed npx MCP
ordinary user: select/use ready administrator-installed MCP only
```

This supersedes the earlier image-bundled-only recommendation, but does not
make browser metadata executable authority. Backend must re-fetch the upstream
detail, accept only registry npm package names, bind the exact package version,
argv, environment-field names, deployment hash, and provenance into an
administrator-owned shared definition, and encrypt that definition on the
internal Runner control plane. The Runner remains one constrained container
that downloads and starts multiple children on demand rather than one resident
container per MCP.

## Recommended MVP interpretation

- `direct`: public Streamable HTTP verified without a credential challenge.
- `header`: API key/static bearer using a backend-approved header name; never a
  query-string secret.
- `oauth`: standards discovery + PKCE. Support pre-registered client identity
  first; Tavily needs DCR or client-ID metadata support before one-click OAuth
  can work in this local deployment.
- `runner_env`: only exact reviewed Runner artifacts whose manifest declares a
  bounded list of user-secret environment names. Store values in the existing
  vault and inject them into a process scoped by user/private Server; never
  install or run arbitrary Marketplace commands.

## Required failure behavior

- Unknown/conflicting auth metadata is `needs_configuration`, never
  `installable`.
- A credential challenge discovered during direct validation becomes
  `needs_auth`, not generic `unavailable`.
- Failed create/credential/OAuth/Runner validation keeps a recoverable installed
  draft with scoped diagnostics; selection is updated only after ready.
- Secrets never appear in DTOs, endpoint URLs, Runner logs, process identifiers,
  audit metadata, or Marketplace caches.
