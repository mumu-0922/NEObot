# Agent Center components

该目录实现独立、URL-addressable 的 Agent Center。它展示 Package Skills、Runs、
Schedules、administrator Learning Review，以及 G20.9 前的 legacy text-Skill 本地
inventory/backup/dry-run。它不复用 Assistant Hub、MCP 或 legacy Skill editor 身份。

## 能力

- desktop list/detail 与 mobile drill-in，tab/selection 由 URL 保存。
- Package Store/library 使用 `/v1/skills/*`，展示 immutable fingerprints。
- Run timeline、approval、Child、Artifact download 与 cancel/kill。
- Cron revision/lifecycle 与 administrator-only Draft review/bounded diff。
- honest `ISOLATION_UNAVAILABLE`/Shadow held 状态、keyboard controls、live
  announcements、mobile back/focus restoration。
- legacy inventory 仅本地计算；backup 显式下载，G20.8 dry-run 的 deletion set 为空。

## 使用

```tsx
<AgentCenter
  activeTab={agentTab}
  selectedId={agentId}
  onNavigate={navigateAgentCenter}
  onClose={closePanel}
/>
```

调用方必须通过 `panel=agent-center&agentTab=...&agentId=...` 的
`panelUrlState.ts` helpers 更新 history，不能建立第二套本地 selection authority。

## 文件

- `AgentCenter.tsx`: top-level shell、四个 product panels 与 shared accessible UI。
- `LegacySkillCutoverCard.tsx`: content-free inventory、explicit backup、no-delete dry-run。

DTO 由 `services/api/client/server/agentCenterApi.ts` 在渲染前 strict Zod 验证。

## 验证

```bash
cd mm-chat/frontend
corepack pnpm exec vitest run \
  src/__tests__/agentCenterComposition.test.ts \
  src/__tests__/serverAgentCenterApi.test.ts \
  src/__tests__/legacySkillCutover.test.ts \
  src/__tests__/chatPanelUrlState.test.ts
corepack pnpm typecheck
```

详见 [DESIGN.md](DESIGN.md) 与
[`docs/contracts/agent-runtime.md`](../../../../docs/contracts/agent-runtime.md)。
