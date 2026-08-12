# Official MCP Go SDK Research

## Selected dependency

- Module: `github.com/modelcontextprotocol/go-sdk`
- Selected release: `v1.7.0`
- Upstream tag commit: `bc72835f62eb94d0fb484439f886b6885b075f36`
- Module checksum: `h1:yqjY2dsbKAC0LSuWZVBMrHgiG8ukXv6NRo0JiALay44=`
- The release requires Go 1.25, matching `mm-chat/backend/go.mod` (`go 1.25.0`).
- The upstream compatibility table identifies MCP specification `2026-07-28`
  as the latest supported protocol for v1.7.0 and retains compatibility with
  earlier supported MCP protocol versions.

## Relevant client seams

- `mcp.NewClient` and `Client.Connect` provide the common client/session model.
- `mcp.StreamableClientTransport` implements remote Streamable HTTP.
- `mcp.CommandTransport` implements command/stdio transport and demonstrates
  the SDK seam the isolated runner can wrap with stricter process ownership.
- `ClientSession.ListTools`, pagination helpers, and `CallTool` cover the MVP
  protocol operations.
- `ClientOptions.ToolListChangedHandler` handles
  `notifications/tools/list_changed`; Neo Chat must defer the resulting refresh
  until a subsequent run rather than mutate an in-flight snapshot.
- The SDK supports MCP content types and current protocol negotiation. Neo Chat
  still owns product normalization, size limits, storage, authorization, and
  provider-compatible schema/alias conversion.

## OAuth findings

- The official SDK contains `auth` and `oauthex` packages for authorization
  code flows, Protected Resource Metadata, authorization server metadata,
  PKCE validation, token exchange, and optional DCR primitives.
- The SDK README explicitly labels client-side OAuth experimental for the
  supported 2025-11-25 compatibility line. The application must not let SDK
  OAuth types become its persistence or API contract.
- Neo Chat should wrap discovery and exchange behind local interfaces, persist
  encrypted token envelopes itself, enforce its own state binding/expiry and
  SSRF-safe HTTP client, and keep DCR disabled by product policy.
- Pinning v1.7.0 avoids an unreviewed moving dependency. Any SDK upgrade must
  rerun protocol, OAuth, schema, transport, and dependency-security tests.

## Design consequence

Use the official SDK for protocol messages, negotiation, Streamable HTTP, and
stdio client behavior. Keep connection pooling, authorization, retries,
budgets, circuit breaking, result persistence, process management, and UI
contracts in Neo Chat-owned packages so upstream experimental surface changes
cannot rewrite product authority.

