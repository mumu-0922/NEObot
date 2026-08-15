# Agent Center Frontend Contract

## Scenario: Operate package Skills with local-direct readiness

### 1. Scope / Trigger

Apply this contract when changing the top-level Agent Center, Agent Center API
types/client, package Skill product UI, Run/Schedule/Learning Review views,
Shadow opt-in, bounded product-canary submission, Artifact download or G20.9
legacy Skill retirement. The current single-server Skill backend is
`local_direct`; retained G20/G21 OCI controls are disabled optional/history.

### 2. Signatures

- Composition: `src/components/agent/AgentCenter.tsx`.
- URL state: `panel=agent-center`, `agentTab=skills|runs|schedules|learning`,
  and optional `agentId`.
- Typed boundary: `src/services/api/client/server/agentCenterApi.ts`.
- Legacy retirement: `src/store/storage/legacySkillRetirement.ts`, persistence
  version `7`, and `legacySkillRetired: true` history projection.
- Focused gates: `bash mm-chat/scripts/verify-agent-product-shadow.sh` and
  `bash mm-chat/scripts/verify-agent-legacy-cutover.sh`.
- Current Runtime status: `state=local_ready`,
  `reasonCode=LOCAL_DIRECT_EXECUTION`, `executable=true`,
  `productCanary=false`.

### 3. Contracts

- Agent Center is a top-level product surface. Do not merge Package Skills into
  Assistant Hub or MCP administration, and do not recreate a legacy text-Skill
  editor/inventory surface.
- Package Store/library uses `/v1/skills/*` and displays immutable package and
  runtime fingerprints plus the honest current Runtime state. An admitted
  instruction-only Skill without `neo.runtime.json` or an OCI image remains
  executable when the server reports `local_ready`.
- When `state=local_ready` and `reasonCode=LOCAL_DIRECT_EXECUTION`, render that
  local Skills can run in ordinary Chat. Also render an explicit localized
  warning that commands use the Backend user's configured local workspace
  authority and are **not** an isolated Sandbox. Do not imply root or `sudo`.
- `productCanary=false` is correct for `local_direct`: ordinary Chat Tool
  execution does not require the optional OCI product-canary path. Never
  replace current `local_ready` with historical `ISOLATION_UNAVAILABLE`.
- The product-canary action is visible only when the server Shadow snapshot has
  `effective=true`. Submit exactly `expectedPolicyRevision` and
  `expectedGeneration`; never send prompt, arguments, Package/model/Tool,
  Workspace, Egress, Secret, argv or resource selection from the browser.
- An accepted product-canary request is only `queued`. Announce its server ID,
  then reload Runs and status; never claim optimistic execution, completion or
  promotion. When activation/budget/policy changes, keep the exact server held
  reason visible and remove/disable the action.
- Runs expose server-owned summary/detail, step/attempt/event timeline,
  approvals, Child hierarchy, Artifact metadata/download and cancel/kill. Never
  infer mutation success locally; reload the server projection.
- Schedules expose immutable revision/fingerprint, approval, next trigger and
  missed/overlap policy. Create/lifecycle actions carry exact expected revision
  and request identity.
- Learning Review is administrator-only and shows exact check receipts,
  bounded ephemeral diff and exact Reject/Promote bindings. A stale conflict
  reloads rather than substituting cached authority.
- Every server DTO is `unknown` until strict Zod validation. Unknown/malformed
  data becomes `ApiClientError("INVALID_SERVER_RESPONSE")`.
- Desktop uses list/detail; mobile uses drill-in with a real back path. Tab and
  selected record survive reload/back/forward through URL state.
- Every action has a keyboard-operable control and accessible name. Restore
  focus after mobile/back transitions and announce loading, mutation, error and
  held status through live text. Do not encode state by color alone.
- Keep list and detail loading/error state independent. A selected mobile record
  must render its own back path plus loading/error/retry instead of hiding the
  list behind a generic selection prompt.
- Capture the invoking button only in its click handler. Do not attach the same
  mutable focus holder as a conditional React `ref`: deselection clears such a
  callback/object ref before the scheduled focus restoration executes.
- Artifact download uses the authenticated server URL only. Browser state never
  receives an object-store key, credential or direct bucket URL.
- G20.9 removes all legacy editor/sidebar/URL/composer/workspace/catalog/service/
  resolver authority. Never install or match a package from an old ID, title,
  name or body.
- Persistence version `7` strips the eight retired settings fields and
  Session/Workspace `activeSkills` from top-level and nested localStorage/
  IndexedDB records. Write the completion marker last; compensate successful
  writes when a later write fails; migrations and `partialize` remain
  idempotent and cannot resurrect authority.
- Historical `skillInvocations` may be recognized only by bounded schema/
  storage normalization and must immediately become
  `{ legacySkillRetired: true }`. Render exactly one localized, non-interactive
  retirement label with no legacy identity/detail.

### 4. Validation & Error Matrix

| Condition | UI behavior |
| --- | --- |
| Runtime is `local_ready` | render ready state plus the non-isolation/local-workspace warning |
| local Runtime disabled | render exact server-disabled reason; no executable claim |
| optional OCI Runtime/Shadow held | render its exact held reason only in the owning historical control; do not downgrade `local_ready` |
| Product canary ready | show bounded action and remaining request count from the server snapshot |
| Product request accepted | announce queued request, then reload Runs/status without optimistic Run state |
| malformed server payload | stable error state; no partial record render |
| stale mutation | announce conflict, reload exact server state |
| non-admin user | omit Learning Review and admin Shadow policy controls |
| empty collection | task-specific empty state, not a spinner |
| mobile record open/back | selected record in URL, focus returns to invoker |
| mobile detail fetch fails | detail keeps back path and renders error/retry; list state remains intact |
| any retired persistence field is present | strip it; preserve unrelated state; marker only after all writes succeed |
| a persistence write fails mid-cutover | compensate prior writes and leave marker absent so reload retries |
| historical invocation array is present | retain message content and one retirement fact; discard all invocation fields |

### 5. Good / Base / Bad Cases

- **Good**: a user sees that an installed instruction-only Skill is locally
  executable, sees the workspace-authority warning, then uses it in Chat.
- **Base**: no Runs/Schedules exist and local Runtime is disabled; all four
  product areas render honest empty/disabled states and no fallback starts.
- **Bad**: optimistic local authority, raw unvalidated JSON, object key in DOM,
  calling local execution isolated, forcing an OCI manifest for an ordinary
  Skill, admin tab for all users, Package/Legacy identity coercion, partial
  migration marker, or browser/API fallback execution.

### 6. Tests Required

- `serverAgentCenterApi.test.ts`: route, request shape, strict response failure.
- `chatPanelUrlState.test.ts`: panel/tab/record parse and serialization.
- `agentCenterComposition.test.ts`: tab separation, held state, accessibility,
  mobile/back/focus/action composition, `local_ready` copy, non-isolation
  warning and instruction-only Skill executability.
- Product-canary tests prove the two-field request, strict queued DTO,
  `effective=true` visibility and Runs/status refresh.
- `legacySkillRetirement.test.ts`: top-level/nested purge, Settings/Chat
  migrate/partialize non-resurrection, compensation, marker-last, idempotence
  and history-detail collapse.
- `verify-agent-legacy-cutover.sh`: deleted surface/assets, zero resolver/prompt
  references and zero legacy fallback authority. Historical OCI hold does not
  gate current `local_direct` execution.
- Run format, lint, typecheck, full Vitest and build before commit.

### 7. Wrong vs Correct

#### Wrong

```text
local_ready -> hide authority warning -> call the command an isolated Sandbox
```

#### Correct

```text
strict server DTO -> local_ready + explicit local-workspace warning
-> instruction-only Package Skill remains usable in ordinary Chat
-> no browser authority and no OCI/Podman prerequisite
```
