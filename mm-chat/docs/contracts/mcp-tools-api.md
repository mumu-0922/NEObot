# MCP Tools API Contract

## Common rules

The product API is rooted at `/v1/mcp`. It uses the existing authenticated
session/bearer context, camelCase JSON, strict unknown-field rejection, a
1 MiB request-body cap, and `Cache-Control: no-store` on every response.

Success responses use the shapes below. Errors use the stable envelope:

```json
{
  "error": {
    "code": "MCP_INVALID",
    "message": "Tools request is invalid"
  }
}
```

The public OAuth callback is the sole route that does not require an existing
browser bearer. Its single-use state restores the bound user and redirect.
No endpoint returns credential values, OAuth tokens, administrator command
arguments, environment values, or raw Tool result bodies.

## Server DTO

```ts
type McpServerRef = {
  source: "catalog" | "manifest" | "private";
  id: string;
};

type McpServer = {
  ref: McpServerRef;
  name: string;
  description?: string;
  icon?: string; // normalized HTTPS URL or short text/emoji; display-only
  transport: "streamable_http" | "stdio";
  endpointUrl?: string;
  authType: "none" | "header" | "oauth";
  status: "draft" | "ready" | "needs_auth" | "unavailable" | "disabled";
  hasCredential: boolean;
  toolCount: number;
  unsupportedToolCount: number;
  lastErrorCode?: string;
  validatedAt?: string;
  grants?: Array<{
    scopeType: "global" | "team" | "workspace";
    scopeId?: string;
    defaultEnabled: boolean;
  }>;
  tools: McpTool[];
};
```

Each Tool includes `serverRef`, original `name`, provider-safe `alias`, optional
`title`/`description`, `inputSchema`, `classification`, `supported`, and an
optional `unsupportedReason`. Ordinary private remote annotations are not
authority for read classification and normalize to `unknown`. A private stdio
Server may inherit `read|write|unknown` only from the current reviewed manifest
artifact's local `toolPolicy` after its exact Marketplace provenance is rebound;
missing policy remains `unknown`.

`icon` is optional display metadata, never trust or execution authority. The
backend returns it only after bounding it to a credential-free HTTPS URL or
short text/emoji. Reviewed private stdio Servers rebind the value from the
current exact manifest artifact; unsafe or missing values are omitted so the
frontend can use its local fallback.

## Routes

### List and create server definitions

```http
GET /v1/mcp/servers?conversationId=<uuid>
POST /v1/mcp/servers
```

`GET` returns `{"servers": McpServer[]}` filtered by current grants and, when
provided, the conversation scope.

Only private public-HTTPS Streamable HTTP drafts may be created:

```json
{
  "name": "Example",
  "endpointUrl": "https://mcp.example/v1",
  "authType": "none",
  "headerName": "Authorization",
  "clientId": "optional-static-client-id",
  "scopes": ["optional.scope"]
}
```

The response is `201 {"server": McpServer}`. The definition stays a draft
until validation succeeds. Unknown fields, plaintext credential values, HTTP or
private-network endpoints, and unsupported auth fail closed. Empty collection
fields remain arrays (`tools: []` for a new draft), never `null`. Creating a
second active private definition for the same user and endpoint returns
`409 MCP_CONFLICT` without changing the existing definition.

### Validate or delete a private definition

```http
POST   /v1/mcp/servers/private/<id>/validate
DELETE /v1/mcp/servers/private/<id>
```

Validation performs bounded MCP initialization/discovery and persists only the
normalized compatible Tool schemas and status. Delete is owner-scoped and
returns `204`.

### Static header credentials

```http
PUT    /v1/mcp/servers/<source>/<id>/credential
DELETE /v1/mcp/servers/<source>/<id>/credential?conversationId=<uuid>
```

PUT body:

```json
{
  "value": "secret-value",
  "conversationId": "optional-conversation-uuid"
}
```

The backend encrypts the value using the MCP vault context before persistence,
clears plaintext buffers where practical, and returns only `hasCredential`.

### OAuth

```http
POST /v1/mcp/oauth/start
GET  /v1/mcp/oauth/callback?state=<opaque>&code=<opaque>
POST /v1/mcp/oauth/revoke
```

Start body:

```json
{
  "serverRef": { "source": "private", "id": "server-id" },
  "conversationId": "optional-conversation-uuid",
  "returnUrl": "https://chat.example/settings/tools"
}
```

Start returns an HTTPS authorization URL. Callback consumes the hashed
single-use state and redirects with `303` to the previously validated return
URL. Revoke accepts `{"serverRef": McpServerRef}` and returns `204`.

### Conversation selection

```http
GET /v1/mcp/conversations/<conversationId>/selection
PUT /v1/mcp/conversations/<conversationId>/selection
```

PUT body:

```json
{
  "mode": "inherit",
  "revision": 3,
  "servers": [
    {
      "ref": { "source": "manifest", "id": "github" },
      "disabledTools": ["delete_repository"]
    }
  ]
}
```

The response is `{"selection": McpConversationSelection}`. `revision` is an
optimistic concurrency token. `custom` plus an empty `servers` array explicitly
disables all MCP Tools.

### Workspace defaults

```http
GET /v1/mcp/workspaces/<workspaceId>/selection
PUT /v1/mcp/workspaces/<workspaceId>/selection
```

PUT accepts `revision` plus the same `servers` array. The backend rechecks
Workspace membership and grants; the browser Workspace cache is not authority.

### Call timeline

```http
GET /v1/mcp/conversations/<conversationId>/calls?runId=<optional-run-id>
```

Returns `{"calls": McpCallRecord[]}`. Each record contains stable IDs, server
reference, Tool name/alias, classification, state, round/call numbers, bounded
redacted summaries, error code, timestamps, and duration. It never contains
raw argument values, Tool result bodies, credentials, or stored object bytes.

### LobeHub MCP Marketplace

```http
GET  /v1/mcp/marketplace/search?q=<text>&category=<slug>&page=1&pageSize=20
GET  /v1/mcp/marketplace/items/<identifier>?version=<exact-version>
POST /v1/mcp/marketplace/items/<identifier>/install
```

Search returns a bounded page of display metadata, bounded category/count
facets, and `source: "lobehub"`. `category` is an optional exact upstream
category key. Item icons are either a short text/emoji or a sanitized HTTPS
URL; clients must retain a local fallback.
Detail returns the exact version, bounded Tool preview, source links, and
sanitized deployment compatibility. Trust badges, ratings, stars, and install
counts are informational only. Neo Chat accepts the optional exact `version`
query but does not forward it to LobeHub's current detail endpoint; it fetches
the current detail and compares the returned version locally, failing with
`MCP_MARKETPLACE_CHANGED` on drift.

Install accepts no URL, command, config schema, or credential:

```json
{
  "version": "1.2.3",
  "conversationId": "optional-conversation-uuid",
  "selectionRevision": 3,
  "enableForConversation": true
}
```

The backend re-fetches the current authoritative detail, compares its returned
version to the exact install request, and selects either a public HTTPS
`http` deployment or a `stdio` deployment whose provider, identifier, exact
version, connection/install method, command, arguments, package name, and
deployment hash match one reviewed manifest artifact. It pins provenance in a
private Server record, then reuses quota/deduplication, validation, MCP
initialization, `tools/list`, and optional revision-checked Conversation
selection. The stdio record contains only an internal `runner://<artifact-id>`
reference; public responses omit that endpoint and all artifact metadata.

SSE and unmatched npm/Docker/Git/binary/manual command paths are display-only.
The backend and Runner never execute Marketplace-supplied commands or download
packages at runtime. Approved executables come only from the immutable Runner
image and are re-bound to the current manifest before validation, selection,
and execution. A validation failure may leave a visible recoverable private
Server and returns its bounded `validationErrorCode`.

## Chat stream integration

An MCP-enabled send uses the existing chat stream endpoint. Before accepting
the run, the backend resolves selection and rejects unavailable/auth-required
servers or a model without native Tool calling. The client may explicitly save
an empty custom selection and retry the send.

Process events include MCP timeline transitions. Completed results are fed back
to the same provider model only through the backend-native continuation loop.
Client-supplied Tool calls, aliases, grants, schemas, or results are never
trusted as execution authority. Arbitrary third-party MCP schemas are not
advertised with the Provider-specific OpenAI `strict` extension; server-side
validation against the frozen schema remains authoritative.

## Error mapping

| HTTP | Code | Meaning |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | Malformed, oversized, trailing, or unknown JSON |
| 400 | `MCP_INVALID` | Invalid selection, URL, credential, OAuth state, or Tool arguments |
| 404 | `MCP_NOT_FOUND` | Definition or Tool does not exist in the authorized scope |
| 409 | `MCP_AUTH_REQUIRED` | Selected server lacks current authorization |
| 409 | `MCP_LIMIT_REACHED` | Private-server or conversation-selection quota reached |
| 409 | `MCP_CONFLICT` | An active private definition already uses this endpoint |
| 503 | `MCP_DISABLED` | Global Tools kill switch is off |
| 503 | `MCP_TRANSPORT_DISABLED` | Selected remote or stdio transport is disabled |
| 503 | `MCP_SERVER_UNAVAILABLE` | Validation or selected server is unavailable |
| 404 | `MCP_MARKETPLACE_NOT_FOUND` | Marketplace item/version does not exist |
| 409 | `MCP_MARKETPLACE_INCOMPATIBLE` | No safe HTTPS HTTP or exact approved Runner artifact is installable |
| 409 | `MCP_MARKETPLACE_CHANGED` | Exact version/deployment changed before install |
| 503 | `MCP_MARKETPLACE_DISABLED` | Optional Marketplace adapter is off |
| 503 | `MCP_MARKETPLACE_UNAVAILABLE` | Adapter credentials or bounded upstream request are unavailable |
| 500 | `MCP_INTERNAL` | Sanitized unexpected failure |

All error messages remain bounded and must not include a custom endpoint,
credential, arguments, result body, or upstream response.

After an SSE run has started, only an explicit Tool-protocol incompatibility
uses `MCP_MODEL_UNSUPPORTED`. Other first-round Provider rejections use the
stream failure code `MCP_PROVIDER_FAILED`; they must not poison the model Tool
capability cache.
