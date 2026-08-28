# Skill Store UI

`SkillStore.tsx` is the standalone user surface for discovering, installing,
and uninstalling Agent Skill packages. Its header separates the
server-authoritative installed library from a source-aware Skill Store
with `Installed | Skill Store` tabs. Installation refreshes both views and
returns the user to `Installed`. The Store keeps backend-mediated LobeHub search,
categories, pagination, detail, and exact-version install separate from the
fixed `openai/skills/skills/.curated` catalog. An exact LobeHub or GitHub Skill
link may also be installed explicitly. The browser never receives Marketplace
credentials or downloads unvalidated package bytes.

The component uses `client.skillStore`, whose server adapter calls only
`/v1/skills/*`. It deliberately has no dependency on the retired Agent Center,
Runs, Schedules, Learning, Shadow, Canary, Runner, or OCI control APIs.

`useSkillStore.ts` owns source-specific requests and install state;
`SkillStorePrimitives.tsx` owns the shell, Installed list, and accessible
loading/error primitives. `SkillMarketplacePrimitives.tsx` owns the category
rail and source-specific cards, while `SkillDetailDialog.tsx` owns the modal
detail/install layer. `SkillStore.tsx` composes the MCP-style top search,
responsive grid, and those focused components without duplicating authorities.

The selected package is URL-addressable through `panel=skill-store&skillId=...`,
where curated entries use their validated Skill name and LobeHub entries use
the source-qualified `lobehub:<identifier>` form.
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
