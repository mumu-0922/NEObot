# Host Workspace Frontend Contract

## 1. Scope / Trigger

Apply when changing the existing Workspace sidebar/settings surface,
`chatStore` Workspace actions, `/v1/workspaces*` client DTOs, Host directory
selection, or Conversation grouping.

## 2. Signatures

```text
GET    /v1/workspaces
GET    /v1/workspaces/host-status
POST   /v1/workspaces/directories/browse
POST   /v1/workspaces/directories/pick-native
PUT    /v1/workspaces/{workspaceId}/conversations/{conversationId}
DELETE /v1/workspaces/{workspaceId}/conversations/{conversationId}
PUT    /v1/chat/conversations/{conversationId}/permission
```

Frontend writes flow through `WorkspaceApi -> workspaceService -> chatStore`.
Server Workspace DTOs carry positive `revision`, `bindingStatus`, safe display
metadata, and optional canonical binding metadata. Conversation DTOs carry
optional `workspaceId` plus durable `permissionMode`.

## 3. Contracts

- Upgrade the existing visible `Workspace` in place. Do not add a competing
  top-level Project concept or discard prompt, files, color, Search, Reasoning,
  id, or Conversation membership.
- In server mode, first list server Workspaces, import only browser records
  whose ids are absent, list again, then adopt the server snapshot. Never replay
  a stale browser record over an existing server revision on page load.
- Validate every Workspace, Host status, directory listing, and picker response
  at the API boundary. `revision` is positive CAS authority; conflicts refresh
  the server snapshot instead of retrying a stale overwrite.
- Display `unbound` versus `bound` and only the sanitized `displayPath` in the
  sidebar/settings. Binding is one-time. Deleting registration never promises
  or requests project-directory deletion.
- Directory interaction is capability-driven. Native Windows selection and WSL
  browse are Host requests; manual path input remains explicit and is resolved
  only by the Host. Never canonicalize or guess a Host path in the browser.
- Creating a server Conversation inside a Workspace is ordered: create the
  Conversation, persist Workspace grouping, then navigate/select it. Moving to
  root uses the explicit clear route and must surface locked/conflict failures.
- Host unavailability preserves ordinary Workspace/chat reads and fails
  bind/browse/pick closed. It must not fall back to Docker `/workspace` or imply
  that Host execution is available.
- Show the permission selector only for Agent Conversations in a bound Host
  Workspace. Populate it from advertised Host modes, lock it during active
  generation, and persist through the dedicated Conversation permission route.
- Selecting Full access opens an accessible in-app `alertdialog` and sends
  `fullAccessAcknowledged=true` only after explicit confirmation. Never use
  generic Conversation config or `window.confirm` as permission authority.

## 4. Validation and Error Matrix

| Condition | Required result |
| --- | --- |
| malformed Workspace/Host response | reject at API normalization; do not mutate store |
| stale Workspace revision | refresh authoritative server list; surface conflict without stale retry |
| Host disabled/unavailable | retain list/edit; disable interactive controls; bind/browse/pick fail closed |
| picker cancelled | no error and no path mutation |
| grouping clear after execution snapshot lock | surface locked failure; retain current grouping |
| server Conversation creation succeeds but grouping fails | do not navigate/select it as a Workspace conversation |
| Full access selected then cancelled | no permission request or local mutation |
| permission request conflicts with an active Turn | retain authoritative mode and surface the locked error |

## 5. Good / Base / Bad Cases

- **Good**: server list loads, imports only missing browser records, user selects
  a Host directory, binding persists, and refreshed Conversations retain the
  exact Workspace grouping.
- **Base**: a legacy unbound Workspace remains fully editable while Host
  controls are unavailable and can be bound later.
- **Bad**: browser state overwrites a newer server revision, the browser guesses
  `/mnt/<drive>`, or a failed Host request silently uses `/workspace`.

## 6. Required Focused Tests

- DTO success and malformed-response rejection;
- legacy missing-only import and server-authoritative refresh;
- CAS conflict refresh;
- Conversation `workspaceId` round trip and grouping/clear failure handling;
- bound/unbound and directory-control composition where UI changes.
- permission DTO round trip, active-generation selector lock, and Full access
  acknowledgement dialog composition.

## 7. Wrong vs Correct

```text
Wrong: hydrate browser Workspace -> PUT every record -> overwrite another tab
Correct: GET server -> PUT only missing ids -> GET -> adopt server revisions

Wrong: browser path -> normalize to /mnt/d -> claim binding
Correct: browser selection/input -> Host resolve -> Backend CAS bind -> safe view

Wrong: put permissionMode in generic config or use window.confirm
Correct: accessible dialog -> dedicated acknowledged API -> refresh durable DTO
```
