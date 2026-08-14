# Agent Center component design

## 目标与非目标

**目标**：把 durable Agent product authority 呈现为 reload-safe、mobile/keyboard/
screen-reader 可操作的控制面，并明确展示 held Runtime/Shadow。

**非目标**：不在浏览器执行 package code，不推断 server mutation success，不暴露
object-store key，不把 Package Skill 与 Assistant、MCP 合并，也不在浏览器/API
执行 package code。G20.9 后不保留 legacy text-Skill surface。

## 架构

```text
ChatApp URL state
    -> AgentCenter shell/status/admin tabs/live region
       -> Package Skills -> /v1/skills/*
       -> Runs/Schedules/Learning -> /v1/agent-center/*
```

所有 server payload 先作为 `unknown` 进入 client Zod schemas；mutation 后 reload
server projection。desktop 同时显示 list/detail，mobile 以 `agentId` drill-in 并提供
真实 back control。

## 关键决策

| 决策                        | 理由                                       | 影响                                     |
| --------------------------- | ------------------------------------------ | ---------------------------------------- |
| Agent Center 顶层独立 panel | 避免 Package/Assistant/MCP/legacy 身份混淆 | URL、Sidebar、i18n 独立                  |
| selection 存入 URL          | reload/back/forward 可复现                 | panel 组件不持有第二份 selection         |
| strict Zod `.strict()`      | 防止 malformed/扩张 DTO 被部分渲染         | failure 统一为 `INVALID_SERVER_RESPONSE` |
| mutation 后 reload          | 浏览器不替代 PostgreSQL authority          | stale conflict 会公告并重取状态          |
| 点击时保存 focus target     | React callback ref 在 deselect 时会清空    | mobile back 后可恢复到原按钮             |
| list/detail error 分离      | detail failure 时 mobile list 被隐藏       | detail 自带 back、error 与 retry         |
| legacy surface 已删除       | G20.9 硬切且不迁移/包装旧数据              | Package Skill 不匹配旧名称/正文          |

## 状态与错误矩阵

| 条件                          | UI                                          |
| ----------------------------- | ------------------------------------------- |
| Runtime/Shadow held           | 顶部显示 stable reason，不展示 fake success |
| malformed DTO                 | stable error + retry，不渲染 partial record |
| stale mutation                | live announcement + reload exact state      |
| non-admin                     | 不渲染 Learning tab/admin policy controls   |
| empty list                    | domain-specific empty state                 |
| mobile detail loading/failure | 保留 back path，显示 loading/error/retry    |
| mobile detail close           | URL 清除 selection，focus 返回 invoker      |

## 安全与隐私

- Artifact 仅请求 authenticated backend download URL；DOM 无 object key/credential。
- React text rendering/`JSON.stringify` 用于 untrusted values；禁用 raw HTML。
- 不读取、上传或记录已退役 legacy Skill body；历史只显示 retirement fact。
- UI 不提供 browser/in-process executor 或 Shadow output→Chat/admission 路径。

## 已知限制

- exact-host promotion 未完成，Run creation 与 executable Shadow 仍 held。
- `AgentCenter.tsx` 聚合四个紧密相关 panel；若下一阶段继续扩展，应按 panel 拆文件并
  保留 shared state/error/focus contract。

## 变更历史

- **2026-08-14 / G20.8**：建立 Agent Center、strict client boundary、mobile focus/
  error recovery 与 legacy cutover preparation。
- **2026-08-14 / G20.9**：删除 legacy cutover card/editor/executor，仅保留 held
  Package Skill control plane。
