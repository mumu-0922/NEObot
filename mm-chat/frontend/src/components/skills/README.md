# Skill Store UI

`SkillStore.tsx` is the standalone user surface for discovering, installing,
and uninstalling admitted Agent Skill packages.

The component uses `client.skillStore`, whose server adapter calls only
`/v1/skills/*`. It deliberately has no dependency on the retired Agent Center,
Runs, Schedules, Learning, Shadow, Canary, Runner, or OCI control APIs.

The selected package is URL-addressable through `panel=skill-store&skillId=...`.
Old `panel=agent-center&agentTab=skills` links are migrated by
`lib/chat/panelUrlState.ts`; other control-plane URL state is discarded.

Run focused coverage with:

```bash
corepack pnpm exec vitest run \
  src/__tests__/skillStoreComposition.test.ts \
  src/__tests__/serverSkillStoreApi.test.ts \
  src/__tests__/chatPanelUrlState.test.ts
```
