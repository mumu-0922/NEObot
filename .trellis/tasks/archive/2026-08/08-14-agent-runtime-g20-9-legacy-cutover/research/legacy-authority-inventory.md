# G20.9 legacy Skill authority inventory

## Classification rule

A reference is **authority** when it can install/edit/select/resolve/inject a
pure-text Skill. **History** may only detect that an old invocation existed.
**Migration guard** may name retired fields only to delete or reject them. UI
copy, tests and docs are not authority but must be synchronized so they cannot
advertise a removed path.

## Executable authority to remove

| Area | Current source | Classification | G20.9 action |
| --- | --- | --- | --- |
| Browser settings | `frontend/src/store/core/settingsStore.ts` | state, mutation and persistence authority for eight legacy fields | remove fields/actions/cache setters, strip them in migration, omit from `partialize` |
| Shared settings views | `store/hooks/useShallowStore.ts`, `features/chat/hooks/useChatShellState.ts`, `lib/settings/types.ts` | makes legacy state callable by product components | remove the entire Skill slice/export surface |
| Catalog/runtime library | `lib/skills/{index,types}.ts` | validates definitions, chooses Skills, builds selection Tool/prompt and executable context | delete after migration/history callers are isolated |
| Static catalog | `public/data/skills*`, `public/data/skills/**` | actively served text definitions | delete; package Store remains server-owned and unrelated |
| Browser service | `services/api/skillService.ts` | fetch/cache/resolve/auto-select execution path | delete; do not redirect names or IDs to package installs |
| Legacy editor | `components/skill/SkillMarket.tsx` | install/edit/uninstall/activate authority | delete the panel, sidebar entry, URL panel and locale namespace |
| Composer | `components/chat/MessageInput.tsx` | Conversation selection and mutation UI | delete selector, overrides and callbacks |
| Workspace editor | `components/layout/WorkspaceSettingsModal.tsx` | Workspace selection and mutation UI | delete active Skill controls and save field |
| Effective context | `lib/chat/effectiveChatContext.ts` | chooses Session/Workspace/global active IDs | delete Skill options/result; preserve prompt, files, search and reasoning |
| Generation flows | `components/app/ChatApp.tsx` | calls `resolveSkillsForMessage`, injects context, writes invocation detail | delete all local/server send, regenerate and edit branches; pass no replacement/fallback context |
| Conversation API mapping | `services/api/chatCrudService.ts` | reads server `config.activeSkills` into frontend authority | stop reading/writing the retired field |
| Local entities | `lib/chat/{types,entities}.ts`, `store/core/chatStore.ts` | Session/Workspace `activeSkills` authority and persistence | remove types; normalizers/migrations delete stale values |

## History-only path to retain in narrowed form

- Existing local messages may contain `skillInvocations` with `id`, `title`,
  `description`, `category` and `mode`.
- `lib/api/schemas.ts`, `store/storage/migrations.ts`, `lib/chat/types.ts` and
  `components/chat/MessageItem.tsx` currently preserve/render those details.
- G20.9 may recognize the old array only at a bounded migration/parse boundary,
  immediately collapse it to one boolean retirement fact, and render one
  localized label: `旧版技能已退役`.
- The normalized runtime `Message` type must not expose Skill ID, title,
  description, category, mode, definition lookup or reopen action.

## Backend persistence observation

The Go Chat API accepts generic `config`/metadata maps. Old server-mode frontend
requests wrote `activeSkills`; the repository stores conversation metadata as
JSONB. Duplicate already allowlists other keys and omits `activeSkills`, but
create/update/read can otherwise preserve it. G20.9 therefore needs:

1. request sanitization that rejects or deletes the retired key;
2. response projection that never returns it;
3. a backup-gated, explicit PostgreSQL cutover operation that removes only
   `conversations.metadata.activeSkills` and proves other metadata unchanged.

A normal reversible schema migration is the wrong carrier for destructive user
data: a `down` file cannot reconstruct deleted selections. Use an explicit
cutover script/runbook with preflight count, transaction and backup restore
rollback instead; keep schema head `090`.

## Non-authority references

- Legacy-focused unit/composition/dataset tests must be deleted or replaced by
  negative cutover tests.
- `Skill.json`, retired MessageInput/Workspace/Sidebar keys and G20.8 inventory
  copy must be deleted or rewritten.
- G20.8 scripts/specs that require `legacyCutover.ts` must advance to the G20.9
  retirement artifact and assert absence of execution.
- Package Skill `/v1/skills/*`, Agent Center `agentTab=skills`, immutable package
  fingerprints and held Run controls are the new product domain and must not be
  removed merely because they contain the word “Skill”.
