# MCP Tools Architecture

## Purpose

Neo Chat exposes **Tools** as a product capability and uses Model Context
Protocol (MCP) as the interoperability layer. Neo Chat is an MCP client only.
It does not expose a public MCP proxy/server and it does not retain the retired
OpenAPI Plugin runtime.

Assistants and Skills are unchanged:

- Assistants remain reusable model/system-prompt presets.
- Skills remain text instructions injected into model context.
- Tools are callable capabilities discovered and executed through MCP.

## Runtime topology

```text
Browser
  -> Next.js same-origin edge
  -> Go /v1/mcp/* control API
  -> PostgreSQL authority

Chat send
  -> Go provider-native ToolRoundProvider loop
  -> frozen MCP run snapshot
  -> remote Streamable HTTP MCP server
     or
  -> private mcp-control network
  -> optional hardened MCP runner
  -> one approved stdio child process started on demand

Large/media result
  -> private MinIO bucket under mcp-results/
```

The Go backend owns identity, Workspace grants, private definitions,
credentials, OAuth state, conversation selection, run snapshots, scheduling,
execution, normalized results, audit retention, and deletion. The browser owns
presentation state only.

## Definition sources and precedence

| Source | Authority | Transport |
| --- | --- | --- |
| Embedded release catalog | Audited release artifact | Streamable HTTP |
| Administrator manifest | Versioned deployment file | Streamable HTTP or stdio |
| Private user definition | PostgreSQL row after validation | Public HTTPS Streamable HTTP only |

Catalog and manifest definitions are declarative authorities. PostgreSQL must
not silently replace their endpoint, command, auth, grant, or tool policy.
Private definitions are created as disabled drafts and become selectable only
after live validation succeeds.

Migration `074_mcp_tools_foundation` adds Workspace membership/grant,
selection, credential, OAuth, run snapshot, call/result, and artifact cleanup
authority. The retired `plugin_registry` table from migration `011` is retained
read-only for one rollback release; no active route or runtime reads it.

## Authorization and selection

A conversation stores one of two explicit modes:

- `inherit`: resolve the current Workspace defaults.
- `custom`: use exactly the listed server references; an empty list means all
  Tools are disabled.

New Workspace conversations inherit defaults. Conversations without a
Workspace begin with no MCP selected. Copying a conversation copies selection
only, never credentials.

Enabling a server/tool is the user authorization boundary, so there is no
per-call confirmation dialog. Before accepting a send, the backend resolves
the effective selection, checks current user/Workspace grants, credentials,
availability, transport kill switches, tool compatibility, and model-native
tool capability. It then persists an immutable run snapshot. Every Tool call is
re-authorized against that snapshot; Tool output cannot expand it.

## Native Tool loop

MCP extends the existing backend `ToolRoundProvider` continuation loop:

```text
model -> native tool calls -> MCP results -> same model -> final response
```

It never asks the frontend to plan calls and never simulates Tool calls with
prompt JSON. Models without native Tool calling reject MCP-enabled sends.

Default hard limits:

- 8 Tool rounds and 32 calls per run;
- 30 seconds per call and 120 seconds wall-clock per run;
- 32 exposed Tool schemas per provider round;
- 4 concurrent calls per user;
- up to 4 concurrent trusted read-only calls;
- write/unknown calls serialized for the same user, including across runs.

Trusted idempotent reads may retry once only when no result was observed and
time remains. Write and unknown calls never retry automatically. A dropped
write connection becomes `outcome_unknown` and terminates the run.

Large servers use bounded relevance selection plus the internal
`neo_mcp_tool_search` Tool. Aliases are deterministic and derived from stable
server identity plus original Tool name. Tool schemas and grants are frozen for
the run; `tools/list_changed` affects only a later run.

## Transport and credential boundaries

Remote private definitions require public HTTPS. DNS resolution, redirects,
OAuth metadata, token endpoints, and reconnects reuse the backend SSRF policy.
Credentials are never forwarded across an origin-changing redirect. Static
header values and OAuth tokens are stored as MCP-specific encrypted vault
references and never returned to the frontend.

OAuth uses Authorization Code with PKCE. Only the hash of a high-entropy,
single-use state is stored, bound to user, server, return URL, and a ten-minute
expiry. The callback restores identity from that state rather than requiring a
browser bearer token. Refresh is singleflight per credential; `invalid_grant`
revokes authority and blocks dependent sends.

The optional stdio path is administrator-only. One resident Runner starts at
most four allowlisted child processes on demand, reaps them after 15 idle
minutes, and enforces a 24-hour maximum lifetime. It receives no user bearer,
PostgreSQL credential, MinIO credential, Docker socket, shell command, or
arbitrary host mount.

## Results, lifecycle, and observability

MCP Text, JSON, Image, Audio, and Resource Link content is normalized as
untrusted Tool data. Resource Links are never fetched automatically. Inline
text/JSON is capped at 256 KiB per call. Larger content and media is stored
under the validated `mcp-results/<conversation>/<call>/...` namespace, with a
25 MiB item and 50 MiB call cap.

The structured timeline persists `queued`, `running`, `succeeded`, `failed`,
`canceled`, and `outcome_unknown`. It exposes bounded redacted argument/result
summaries, not raw secrets or bodies.

Conversation deletion removes result objects before durable execution rows.
Account deletion queues validated object coordinates before the user FK
cascade; the cleanup-only worker removes the objects afterward. Audit rows and
objects expire after 90 days by default. Object deletion precedes database-row
deletion and is safe to retry after partial failure. Cleanup continues even
when `MCP_ENABLED=false`.

Metrics use only bounded transport, outcome, error-class, and read/write labels.
Logs may carry `request_id`, `chat_run_id`, and `tool_call_id`, but never raw
arguments, result bodies, tokens, custom URLs, or high-cardinality identities.

## Kill switches and rollback

- `MCP_ENABLED=false`: blocks discovery, selection changes, and execution while
  retention/account artifact cleanup remains active.
- `MCP_REMOTE_ENABLED=false`: blocks remote connections only.
- `MCP_STDIO_ENABLED=false`: blocks Runner use only.

A rollback disables MCP or restores prior application images while retaining
migration `074` and its data. Never run `074.down` after live MCP traffic. The
old Plugin runtime is not revived by any MCP switch.

## Verification anchors

- `backend/internal/mcpclient/`
- `backend/internal/mcprunner/`
- `backend/internal/chat/mcp_tool_loop.go`
- `backend/migrations/074_mcp_tools_foundation.up.sql`
- `scripts/verify-mcp-postgres17.sh`
- `scripts/test-preflight-single-server.sh`
- [`../contracts/mcp-tools-api.md`](../contracts/mcp-tools-api.md)
- [`../deployment/mcp-runner.md`](../deployment/mcp-runner.md)
