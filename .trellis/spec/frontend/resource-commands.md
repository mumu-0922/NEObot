# Composer Resource Picker Contract

## 1. Scope / Trigger

Apply when changing `MessageInput`, `ConversationResourcePickers`, Skill/MCP
conversation-selection APIs, Resource management deep links, or Resource
process cards. Resource lifecycle Slash commands are retired; inventory,
conversation selection, and Run activation are separate states.

## 2. Signatures

```ts
interface ConversationResourcePickersProps {
  conversationId?: string;
  skillEnabled: boolean;
  mcpEnabled: boolean;
  runActive: boolean;
  disabled?: boolean;
  onOpenSkillStore(): void;
  onOpenMcpTools(): void;
}

GET /v1/skills/conversations/{conversationId}/selection
PUT /v1/skills/conversations/{conversationId}/selection
{ revision: number; installationIds: string[] }

GET /v1/mcp/conversations/{conversationId}/selection
PUT /v1/mcp/conversations/{conversationId}/selection
{ mode: "custom"; revision: number; servers: McpSelectionServer[] }
```

The typed Skill response is
`{conversationId, revision, skills: AgentPackageInstallationDTO[]}`. MCP keeps
its existing `McpConversationSelection` DTO. Both server clients must encode the
conversation ID and validate responses before use.

## 3. Contracts

- Render exactly one compact Skill icon and one compact MCP icon beside the
  other composer controls when the corresponding server capability is enabled.
- Each popover lists only installed/authorized inventory, supports search and
  multi-select, shows selected count, and links to the full management page.
  It does not install, uninstall, configure credentials, or expose protocol
  diagnostics.
- Selection is server-authoritative and scoped to the exact conversation.
  Reload it when `conversationId` changes or the page refreshes. Never use
  localStorage or the previously opened conversation as fallback authority.
- Writes use the latest Backend revision. A stale write reloads the selection
  instead of overwriting another tab/device.
- An MCP picker write uses `mode="custom"`; explicit `servers=[]` means none.
  Installed servers that are not `ready` stay visible but disabled.
- During an active Run, changes are permitted but visibly labelled “next Run”.
  The current frozen Resource Snapshot is never edited in place.
- Resource lifecycle entries (`/skill`, `/mcp`, `/resources`, `/reload`,
  install/remove/enable/disable) must not return to the `/` palette or
  `ChatApp` send interception. Historical message replay remains a Backend
  compatibility concern.
- Process/configuration cards may deep-link to Skill Store or Tools through the
  bounded `RESOURCE_MANAGER_OPEN_EVENT`; they never place credentials, raw
  Tool arguments, paths, or package bodies in browser state.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| no persisted conversation ID | picker disabled; no selection request |
| Skill/MCP capability disabled | corresponding icon omitted |
| inventory or selection load fails | bounded popover error; no guessed selection |
| response violates strict DTO | `INVALID_SERVER_RESPONSE`; do not use it |
| stale revision/CAS conflict | show save failure, reload authoritative state |
| MCP server is not `ready` | visible disabled row; no write |
| Run is active | selection write may succeed; show next-Run notice |
| conversation changes during request | abort/ignore old result; do not contaminate new conversation |

## 5. Good / Base / Bad Cases

- **Good:** Conversation A selects Skill-A/MCP-A and Conversation B selects
  Skill-B/MCP-B; switching, refreshing, and signing in on another device reads
  the independent Backend rows.
- **Base:** a new conversation has revision zero and no selected Skills; MCP
  follows its explicit default/inherit policy until the first custom choice.
- **Bad:** selecting one resource updates every conversation, installing a
  package silently checks it in the composer, or Slash commands create a second
  mutation UI.

## 6. Tests Required

- API client GET/PUT paths, encoded IDs, request body, strict response parsing,
  and malformed-response rejection.
- MessageInput composition proves both lightweight picker wiring and the
  absence of `McpToolsControl` and lifecycle Slash command code.
- Selection tests cover conversation switch/reload, selected counts, ready-only
  MCP mutation, revision conflict reload, and next-Run notice.
- Backend tests cover owner isolation, zero-to-one revision, stale CAS,
  duplicate/foreign installation rejection, duplicate-conversation copy, and
  migration owner/fingerprint foreign keys.
- Run frontend format/lint/typecheck/Vitest/build plus focused Go and migration
  gates for cross-layer changes.

## 7. Wrong vs Correct

```text
Wrong: installed inventory -> global browser toggle -> every conversation
Correct: installed inventory -> conversation picker -> Backend CAS row -> next Run

Wrong: composer /mcp install ... -> lifecycle mutation
Correct: composer MCP icon -> select installed server; Tools owns installation

Wrong: Agent auto-match -> PUT conversation selection
Correct: Agent auto-match -> frozen current Run snapshot only
```
