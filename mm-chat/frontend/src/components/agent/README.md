# Agent Center components

该目录实现独立、URL-addressable 的 Agent Center。它展示 Package Skills、Runs、
Schedules 与 administrator Learning Review。G20.9 已删除 legacy text-Skill
inventory/editor 及其执行身份；Agent Center 不复用 Assistant Hub 或 MCP 身份。

## 能力

- desktop list/detail 与 mobile drill-in，tab/selection 由 URL 保存。
- Package Store/library 使用 `/v1/skills/*`，展示 immutable fingerprints。
- Run timeline、approval、Child、Artifact download 与 cancel/kill。
- Cron revision/lifecycle 与 administrator-only Draft review/bounded diff。
- honest `ISOLATION_UNAVAILABLE`/Shadow held 状态、keyboard controls、live
  announcements、mobile back/focus restoration。
- G20.9 后 Package Skills 是唯一 eligible Skill domain；当前 host 仍 held，UI 不提供
  browser/API fallback executor。

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

DTO 由 `services/api/client/server/agentCenterApi.ts` 在渲染前 strict Zod 验证。

## 验证

```bash
cd mm-chat/frontend
corepack pnpm exec vitest run \
  src/__tests__/agentCenterComposition.test.ts \
  src/__tests__/serverAgentCenterApi.test.ts \
  src/__tests__/legacySkillRetirement.test.ts \
  src/__tests__/chatPanelUrlState.test.ts
corepack pnpm typecheck
```

详见 [DESIGN.md](DESIGN.md) 与
[`docs/contracts/agent-runtime.md`](../../../../docs/contracts/agent-runtime.md)。
