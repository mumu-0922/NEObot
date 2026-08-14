# Agent Center Frontend Contract

## Scenario: Operate package Skills and held Agent controls

### 1. Scope / Trigger

Apply this contract when changing the top-level Agent Center, Agent Center API
types/client, package Skill product UI, Run/Schedule/Learning Review views,
Shadow opt-in, Artifact download or legacy Skill cutover inventory.

### 2. Signatures

- Composition: `src/components/agent/AgentCenter.tsx` and
  `src/components/agent/LegacySkillCutoverCard.tsx`.
- URL state: `panel=agent-center`, `agentTab=skills|runs|schedules|learning`,
  and optional `agentId`.
- Typed boundary: `src/services/api/client/server/agentCenterApi.ts`.
- Legacy preparation: `src/lib/skills/legacyCutover.ts`.
- Focused gate: `bash mm-chat/scripts/verify-agent-product-shadow.sh`.

### 3. Contracts

- Agent Center is a top-level product surface. Do not merge Package Skills into
  Assistant Hub, MCP administration or the legacy text-Skill editor.
- Package Store/library uses `/v1/skills/*` and displays immutable package and
  runtime fingerprints plus the honest held Runtime state.
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
- Legacy inventory is deterministic and content-free. Explicit local backup may
  include raw settings, but dry-run contains no delete call and preserves
  Assistant, MCP, Chat, Conversation, files, Knowledge and Memory. G20.8 does
  not auto-install a matching package or delete a text Skill.

### 4. Validation & Error Matrix

| Condition | UI behavior |
| --- | --- |
| Runtime/Shadow held | render reason text including `ISOLATION_UNAVAILABLE`; no fake success |
| malformed server payload | stable error state; no partial record render |
| stale mutation | announce conflict, reload exact server state |
| non-admin user | omit Learning Review and admin Shadow policy controls |
| empty collection | task-specific empty state, not a spinner |
| mobile record open/back | selected record in URL, focus returns to invoker |
| mobile detail fetch fails | detail keeps back path and renders error/retry; list state remains intact |
| legacy inventory invalid/orphan entry | show counts/fingerprint only; do not upload raw body |

### 5. Good / Base / Bad Cases

- **Good**: a user reloads a Run detail URL, reviews exact process facts,
  downloads an owned Artifact and cancels with the current fingerprint.
- **Base**: no Runs/Schedules exist and Runtime is held; all four product areas
  render honest empty/held states while Legacy Skills still operate separately.
- **Bad**: optimistic local authority, raw unvalidated JSON, object key in DOM,
  admin tab for all users, Package/Legacy Skill identity coercion or storage
  deletion during inventory.

### 6. Tests Required

- `serverAgentCenterApi.test.ts`: route, request shape, strict response failure.
- `chatPanelUrlState.test.ts`: panel/tab/record parse and serialization.
- `agentCenterComposition.test.ts`: tab separation, held state, accessibility,
  mobile/back/focus/action composition.
- `legacySkillCutover.test.ts`: deterministic inventory/backup/dry-run and
  unrelated state preservation.
- Run format, lint, typecheck, full Vitest and build before commit.

### 7. Wrong vs Correct

#### Wrong

```text
legacy Skill title matches package -> silently install -> execute from browser
```

#### Correct

```text
Legacy Skills stay labelled and authoritative in G20.8
Package Skills use server fingerprints in Agent Center
inventory + explicit local backup + dry-run only -> G20.9 deletion later
```
