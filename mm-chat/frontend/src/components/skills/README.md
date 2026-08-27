# Skill Store UI

`SkillStore.tsx` is the standalone user surface for discovering, installing,
and uninstalling Agent Skill packages. Its header separates the
server-authoritative installed library from the fixed OpenAI curated catalog
with `Installed | Skill Store` tabs. Installation refreshes both views and
returns the user to `Installed`. Discovery is served by the Backend from
`openai/skills/skills/.curated`; the browser never scrapes a marketplace.

The component uses `client.skillStore`, whose server adapter calls only
`/v1/skills/*`. It deliberately has no dependency on the retired Agent Center,
Runs, Schedules, Learning, Shadow, Canary, Runner, or OCI control APIs.

The selected package is URL-addressable through `panel=skill-store&skillId=...`,
where curated entries use their validated Skill name.
Old `panel=agent-center&agentTab=skills` links are migrated by
`lib/chat/panelUrlState.ts`; other control-plane URL state is discarded.

`ConversationResourcePickers.tsx` is the separate composer surface for choosing
already-installed Skills in one conversation. It reads and writes the
revision-bound Backend selection; it cannot install or uninstall packages.

Run focused coverage with:

```bash
corepack pnpm exec vitest run \
  src/__tests__/skillStoreComposition.test.ts \
  src/__tests__/serverSkillStoreApi.test.ts \
  src/__tests__/chatPanelUrlState.test.ts
```
