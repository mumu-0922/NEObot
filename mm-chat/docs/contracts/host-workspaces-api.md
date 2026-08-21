# Host Workspaces API

## Status and scope

Migration `102_host_workspaces` and `/v1/workspaces*` provide the durable
Backend half of Harness-style filesystem Workspaces. They preserve legacy
browser Workspace settings and add one-time Host binding plus immutable
Conversation execution authority.

The live Backend is not yet connected to the Host Runner socket. Listing,
legacy import, settings updates, grouping, and deletion are available after the
migration; binding currently fails closed with `503`.

## Workspace representation

```json
{
  "id": "0198ca9a-81c6-7c8d-9444-b16da02de9b4",
  "name": "neo-chat",
  "systemPrompt": "Work in this repository",
  "files": [],
  "color": "blue",
  "enableSearch": true,
  "enableReasoning": true,
  "revision": 2,
  "bindingStatus": "bound",
  "runnerId": "wsl-0123456789abcdef01234567",
  "canonicalPath": "/mnt/d/projects/neo-chat",
  "displayPath": "D:\\projects\\neo-chat",
  "pathKind": "windows-mounted",
  "directoryFingerprint": "sha256:<64-lowercase-hex>",
  "boundAt": "2026-08-21T08:00:00Z",
  "createdAt": "2026-08-21T07:00:00Z",
  "updatedAt": "2026-08-21T08:00:00Z"
}
```

`bindingStatus` is `unbound` or `bound`. Optional Search/Reasoning booleans
preserve the difference between a legacy unset value and an explicit false.
`revision` is the compare-and-set authority for every mutation after import.

## Routes

### List and read

```text
GET /v1/workspaces
GET /v1/workspaces/{workspaceId}
```

Only active Workspaces owned by the authenticated user are returned.

### Idempotent legacy import

```text
PUT /v1/workspaces/{workspaceId}
Content-Type: application/json
```

```json
{
  "name": "Legacy Workspace",
  "systemPrompt": "",
  "files": [],
  "color": "",
  "enableSearch": true,
  "enableReasoning": false
}
```

The caller supplies the existing browser UUID. Replaying byte-equivalent
settings does not increment the revision. The operation never guesses or binds
a Host directory and never replaces a record owned by another user.

### Settings compare-and-set

```text
PATCH /v1/workspaces/{workspaceId}
```

The body contains the complete settings object above plus
`"expectedRevision": <positive integer>`. It is a full settings replacement,
not a merge patch.

### One-time Host binding

```text
POST /v1/workspaces/{workspaceId}/bind
```

```json
{"expectedRevision":1,"path":"D:\\projects\\neo-chat"}
```

The Backend must not canonicalize the path. It sends the path to the pinned
Host Runner, validates the returned descriptor, and atomically binds the full
Runner/path/fingerprint tuple. A Workspace cannot be rebound. Another spelling
of the same owner/Runner directory conflicts with the existing registration.

Until the Backend socket integration slice is enabled, this route returns:

```json
{"error":{"code":"HOST_WORKSPACE_UNAVAILABLE","message":"Host Workspace is unavailable"}}
```

### Conversation grouping

```text
PUT /v1/workspaces/{workspaceId}/conversations/{conversationId}
```

The request has no body. It sets current visible grouping only when both
records belong to the authenticated user. Once an Agent execution snapshot
exists, grouping cannot move to a different Workspace.

### Soft delete

```text
DELETE /v1/workspaces/{workspaceId}
Content-Type: application/json

{"expectedRevision":2}
```

Deletion changes only the registration. It never deletes, renames, or writes
the Host directory. An execution-bound Workspace cannot be deleted.

## Execution binding

`conversations.workspace_id` is the visible grouping selected by the user.
Before the first future Host Agent execution, the Backend must call
`LockConversationExecutionWorkspace`, which atomically persists:

```text
agent_workspace_id
agent_workspace_runner_id
agent_workspace_canonical_path
agent_workspace_fingerprint
agent_workspace_bound_at
```

The exact snapshot is returned on idempotent repeats. A different Workspace is
rejected; execution must never follow later grouping drift or fall back to the
Docker `local_direct` root.

## Validation and errors

Requests require authenticated ownership, exact route shape, no query string,
`application/json` where a body is defined, strict JSON, and bounded valid
UTF-8 fields. Responses use `Cache-Control: no-store`.

| Condition | Status/code |
| --- | --- |
| malformed input or UUID | `400 INVALID_WORKSPACE_REQUEST` |
| missing/cross-user record | `404 WORKSPACE_NOT_FOUND` |
| stale revision | `409 WORKSPACE_REVISION_CONFLICT` |
| Workspace already bound | `409 WORKSPACE_ALREADY_BOUND` |
| canonical directory duplicate | `409 WORKSPACE_DIRECTORY_REGISTERED` |
| Conversation execution binding conflicts | `409 CONVERSATION_WORKSPACE_LOCKED` |
| Workspace has no Host binding | `409 WORKSPACE_UNBOUND` |
| Workspace is execution-bound/in use | `409 WORKSPACE_IN_USE` |
| Host resolver absent/unavailable | `503 HOST_WORKSPACE_UNAVAILABLE` |
| Host returns a stable resolve error | `502 HOST_WORKSPACE_RESOLVE_FAILED` |
| Host response violates protocol | `502 HOST_WORKSPACE_PROTOCOL_INVALID` |

Errors never echo the submitted Host path, Runner transport details, SQL, or
raw OS errors.

## Migration and rollback

Migration `102` extends the existing `workspaces` table in place and adds the
Conversation execution snapshot. It does not create a second top-level Project
table. The runtime role receives only the required Workspace insert/update
columns and Conversation grouping/snapshot update columns.

An empty migration can down/re-up cleanly. Once legacy settings, a Host binding,
or an execution snapshot exists, down raises `HOST_WORKSPACE_ROLLBACK_BLOCKED`.
Use a compatible Backend image or restore a matched backup; do not delete
Workspace rows or project directories to force rollback.
