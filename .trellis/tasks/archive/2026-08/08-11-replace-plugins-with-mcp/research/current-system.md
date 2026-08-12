# Current System Findings

## Plugin runtime

- `mm-chat/frontend/src/services/api/serverPluginOrchestration.ts` implements
  the proprietary one-shot plugin planner/executor path. It is not MCP and it
  does not perform provider-native same-model continuation.
- `mm-chat/backend/internal/plugins/handler.go` owns `/v1/plugins`,
  `/v1/plugins/install`, and `/v1/plugins/execute`.
- `mm-chat/backend/internal/plugins/builtins.go` owns Weather, Unsplash, Agnes
  Image, and Agnes Video definitions.
- `mm-chat/backend/internal/plugins/repository_postgres.go` persists custom
  definitions in `plugin_registry`, introduced by migration `011`.
- Plugin routes are also present in HTTP authentication/metric path
  normalization, tests, typed frontend clients, exports, and browser-import
  fixtures; deletion must search all callers rather than remove only the
  handler package.

## Existing provider-native tool loop

- `mm-chat/backend/internal/chat/provider_tool_round.go` defines
  `ToolRoundProvider`, `ProviderRoundRequest`, `ProviderToolExchange`, and
  normalized provider tool calls/results.
- OpenAI-compatible, Gemini, and Anthropic implementations already support
  native continuation. `web_tool_loop.go` coordinates current Web, Knowledge,
  and Memory product tools.
- Chat SSE already emits provider tool and process-step events and owns
  detached run cancellation through the active-run registry.
- MCP should register another backend-owned executor with this loop instead of
  retaining the frontend plugin planning path.
- The current Tool Loop specification intentionally has no general product
  round/call cap and requires approval before registering side effects. The
  confirmed MCP design intentionally adds MCP-specific limits and treats
  explicit conversation server/tool enablement as authorization; the spec
  must record this scoped difference without changing existing tools.

## Frontend state and migration risk

- `SessionConfig.activePlugins`, `Workspace.activePlugins`, and global settings
  state create three overlapping selection authorities.
- Global plugin state/config is persisted in the `neo-chat-settings` IndexedDB
  store; sessions and Workspaces are persisted in `neo-chat-storage`.
- Empty session plugin arrays are normalized away, so the old state cannot
  express an explicit empty override.
- Creating a conversation from a Workspace copies its plugin list; opening a
  conversation with a non-empty list can write it back into global state.
- Server conversation configuration is stored as unrestricted
  `conversations.metadata JSONB`, and copy currently clones that metadata.
- The existing frontend Workspace is browser-owned. There is no reliable
  server conversation-to-Workspace authority suitable for MCP access grants.
- A safe migration must cover IndexedDB, imported/exported data, normalized
  session/Workspace types, server metadata, and copy allowlists.

## Authentication and secrets

- The application has no reusable OAuth callback, authorization-code, PKCE,
  refresh-token, or revoke lifecycle. Old plugin `oauth2` handling merely
  injects an already supplied bearer value.
- Browser application sessions use bearer tokens stored in `sessionStorage`;
  an OAuth provider redirect cannot be assumed to carry the Neo Chat token.
- `providersecrets.Vault` already implements strict bounded keyring loading,
  context-bound AES-256-GCM envelopes, fresh nonces, retained-key rotation, and
  safe PostgreSQL persistence. MCP credentials should reuse this primitive
  with a new context domain, not reuse provider-config JSON.
- Existing JSON handlers use bounded bodies, unknown-field rejection, trailing
  JSON rejection, camelCase DTOs, stable `{error:{code,message}}` envelopes,
  no-store responses for sensitive data, and request-context identity.
- There is no general application-level site-admin role authority. Shared MCP
  definitions are therefore deployment-operator manifests, while user and
  Team/Workspace actions require explicit user/membership authorization.

## Storage and deployment

- PostgreSQL migrations are embedded paired up/down files with checksums,
  advisory locking, and per-migration transactions.
- MinIO already stores server-authoritative artifacts and participates in
  backup/restore flows.
- The standalone Compose topology has hardened worker templates with non-root,
  read-only filesystem, dropped capabilities, no-new-privileges, tmpfs, and
  CPU/memory/PID limits.
- The backend runtime image does not contain Node, npm, Python, or uv. Common
  stdio servers cannot run safely in that image without a dedicated immutable
  runner artifact.
- Existing internal services use independent Docker-secret-backed tokens and
  private networks. A runner can reuse the pattern, but not the same secret.
- Logs use structured `slog` and request IDs. Metrics are custom Prometheus
  text with deliberately bounded labels; OpenTelemetry is not currently wired.

