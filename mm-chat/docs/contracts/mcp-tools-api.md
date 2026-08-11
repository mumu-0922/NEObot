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
optional `unsupportedReason`. Remote annotations are not authority for private
server read classification.

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
| 500 | `MCP_INTERNAL` | Sanitized unexpected failure |

All error messages remain bounded and must not include a custom endpoint,
credential, arguments, result body, or upstream response.

After an SSE run has started, only an explicit Tool-protocol incompatibility
uses `MCP_MODEL_UNSUPPORTED`. Other first-round Provider rejections use the
stream failure code `MCP_PROVIDER_FAILED`; they must not poison the model Tool
capability cache.
