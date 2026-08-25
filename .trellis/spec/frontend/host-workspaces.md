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
GET    /v1/workspaces/{workspaceId}/files/content?path={relative}&download={bool}
GET    /v1/workspaces/{workspaceId}/files/preview?path={relative}

MessageOutputBlock.workspace_file = {
  id, type, workspaceId, path, fileName, mimeType, size, version
}
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
- Sidebar Conversation navigation is one hierarchy. Persisted Workspaces are
  expandable parents; `Conversation.workspaceId` is the sole membership
  authority. Conversations without it appear exactly once under the virtual
  `temporary-chats` group, which is presentation state and must never be
  persisted as a fake Workspace.
- A generic new-Conversation action inherits the active Conversation's
  Workspace. The explicit Temporary-group action always creates an unbound
  Conversation. A Workspace action must create, group, and only then navigate;
  if grouping fails, remove the empty Conversation when possible, restore the
  previous selection, and surface the failure.
- Preview limits may hide old Conversations, but selecting or restoring one
  must expand both its owning parent and its preview list so the active row is
  visible. Search does not create a second result tree or duplicate rows.
- Host unavailability preserves ordinary Workspace/chat reads and fails
  bind/browse/pick closed. It must not fall back to Docker `/workspace` or imply
  that Host execution is available.
- Show the permission selector only for Agent Conversations in a bound Host
  Workspace. Populate it from advertised Host modes, lock it during active
  generation, and persist through the dedicated Conversation permission route.
- Selecting Full access opens an accessible in-app `alertdialog` and sends
  `fullAccessAcknowledged=true` only after explicit confirmation. Never use
  generic Conversation config or `window.confirm` as permission authority.
- Normalize `workspace_file` blocks at the server DTO boundary. Require a
  canonical UUID, relative slash-separated path, bounded file name/MIME/size,
  and `sha256:<64 lowercase hex>` generation version. Invalid blocks are
  dropped without invalidating the remaining Message output.
- Render a Workspace File Card under the owning assistant Turn. Its primary
  action opens the current bound Workspace file; authenticated Download is a
  secondary action. Do not turn the card into a server-output attachment or
  trust a browser filesystem path.
- File preview is current-state authority. Compare its returned version with
  the generation version and show `file changed` on mismatch. Preview text,
  DOCX, image, audio, PDF, and XLSX in-app; XLSX requires sheet tabs and a
  bounded value grid. Unsupported/complex content may offer Download but must
  not be decoded as arbitrary text.

## 4. Validation and Error Matrix

| Condition | Required result |
| --- | --- |
| malformed Workspace/Host response | reject at API normalization; do not mutate store |
| stale Workspace revision | refresh authoritative server list; surface conflict without stale retry |
| Host disabled/unavailable | retain list/edit; disable interactive controls; bind/browse/pick fail closed |
| picker cancelled | no error and no path mutation |
| grouping clear after execution snapshot lock | surface locked failure; retain current grouping |
| server Conversation creation succeeds but grouping fails | roll back the empty Conversation when possible, restore the previous selection, and do not present it as a Workspace Conversation |
| Full access selected then cancelled | no permission request or local mutation |
| permission request conflicts with an active Turn | retain authoritative mode and surface the locked error |
| malformed/traversal/oversized `workspace_file` block | drop the block; render the rest of the Message |
| preview version differs from card version | preview current bytes and show `file changed` |
| preview endpoint fails or rejects format | show bounded failure state; do not invent content |

## 5. Good / Base / Bad Cases

- **Good**: server list loads, imports only missing browser records, user selects
  a Host directory, binding persists, and refreshed Conversations retain the
  exact Workspace grouping in one expandable navigation tree.
- **Good**: a completed Agent Turn reloads its XLSX `workspace_file` block,
  opens a sheet/value preview from the project directory, and keeps Download as
  a secondary action.
- **Base**: a Conversation without `workspaceId` remains durable and available
  under `temporary-chats`; it can later be moved into a real Workspace.
- **Bad**: browser state overwrites a newer server revision, the browser guesses
  `/mnt/<drive>`, a Conversation appears in both Workspace and root lists, or a
  failed grouping silently leaves the new row selected as if it succeeded.
- **Bad**: convert a project file into an automatic object-store attachment,
  show only Download, trust an absolute path from Message JSON, or silently
  show changed bytes as the historical snapshot.

## 6. Required Focused Tests

- DTO success and malformed-response rejection;
- legacy missing-only import and server-authoritative refresh;
- CAS conflict refresh;
- Conversation `workspaceId` round trip and grouping/clear failure handling;
- single-tree Sidebar composition, virtual Temporary grouping, active-row
  preview expansion, and contextual versus explicit-Temporary creation;
- bound/unbound and directory-control composition where UI changes.
- permission DTO round trip, active-generation selector lock, and Full access
  acknowledgement dialog composition.
- `workspace_file` valid/malformed normalization, live/reload preservation,
  encoded content/preview requests, open-first card composition, version-change
  notice, and bounded XLSX sheet/value rendering.

## 7. Wrong vs Correct

```text
Wrong: hydrate browser Workspace -> PUT every record -> overwrite another tab
Correct: GET server -> PUT only missing ids -> GET -> adopt server revisions

Wrong: browser path -> normalize to /mnt/d -> claim binding
Correct: browser selection/input -> Host resolve -> Backend CAS bind -> safe view

Wrong: put permissionMode in generic config or use window.confirm
Correct: accessible dialog -> dedicated acknowledged API -> refresh durable DTO

Wrong: render Workspaces and a second top-level Conversation list
Correct: Workspace children + virtual temporary-chats, partitioned by workspaceId

Wrong: project file -> automatic attachment copy -> Download-only card
Correct: workspace_file(path, version) -> authenticated current preview -> optional Download
```
