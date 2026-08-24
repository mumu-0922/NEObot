# 重构工作区会话导航并显示运行耗时

## Goal

把左侧导航收敛为“工作区包含对话”的单一层级，消除工作区与独立对话列表之间的重复和归属歧义；未绑定项目目录的会话归入“临时对话”。同时在 Chat 与 Agent 的每条 Assistant 回复底部显示可持久恢复的本次运行总耗时。

## What I Already Know

- 用户确认采用所选方案图：工作区作为可展开项目节点，其下直接展示属于该工作区的会话。
- 工作区固定绑定项目目录；会话保存消息历史、最后选择的模型、Chat/Agent 模式与权限状态。
- 独立的“对话列表”不再与工作区并列；没有工作区关联的会话进入“临时对话”虚拟分组。
- 会话继续以 PostgreSQL 为真源，不改为 Pi 风格的本地 JSONL 文件。
- 当前 `Sidebar.tsx` 已能按 `session.workspaceId` 拆分工作区会话和未绑定会话，但未绑定会话仍以独立“对话列表”的置顶/最近/归档分区呈现。
- 当前 `MessageItem.tsx` 已有 `message.timing.duration` 的 Footer 展示能力。
- 服务端 `ChatMessageDTO` 已返回 `createdAt` 与可选 `completedAt`，但 `mapChatMessageDtoToMessage()` 当前没有将两者恢复成前端 `timing`，导致持久会话刷新后无法稳定显示耗时。

## Assumptions (Temporary)

- 总耗时包括 RAG、联网搜索、Reasoning、LLM 生成及 Agent Tool 执行等待。
- 运行中不在 Footer 显示最终耗时；结束后显示稳定值，刷新后保持一致。

## Open Questions

- 无。

## Requirements (Evolving)

### Sidebar information architecture

- 左侧只保留一套会话导航层级，不再显示独立的“对话列表”区块。
- 每个工作区显示名称与绑定目录；展开后显示该工作区所属会话。
- 未设置 `workspaceId` 的会话统一显示在“临时对话”虚拟分组下；该分组不是数据库中的伪工作区。
- 同一会话在侧边栏只出现一次。
- 工作区与临时对话内部按最近更新时间排序，并继续支持搜索、重命名、置顶、复制、移动、删除等现有会话动作。
- 工作区节点只负责展开/收起和项目操作，不承载 Chat/Agent 模式切换。
- Chat/Agent 模式只保留在底部 Composer；切换会话时恢复该会话最后一次模式、模型和权限状态，不能影响其他会话。
- 当前会话头部应能识别工作区与会话，例如 `test / 查看当前工作区状态`。
- 在当前工作区中创建新会话时，新会话直接归属该工作区；无活动工作区时创建到“临时对话”。
- 临时对话切换到需要文件工具的 Agent 行为前，必须先绑定或选择一个工作区。

### Per-response total duration

- Chat 和 Agent 的 Assistant 回复 Footer 都显示总耗时，视觉位置参考用户截图中的 `用时 2分45秒`。
- “总计时间”是单次 Assistant 回复从用户发送开始，到成功、失败或取消结束的端到端 Wall-clock 时间；不是整段会话累计时间，也不是仅模型生成时间。
- 总耗时必须覆盖 RAG、联网搜索、Reasoning、LLM 生成及 Agent Tool 执行。
- Agent 等待用户手动批准 Tool 权限的停留时间也计入，因为该指标表达用户从发送到结束的实际等待时间。
- 耗时使用用户可读格式，例如 `<1秒`、`8秒`、`2分45秒`、`1小时03分`。
- 成功、失败和取消的回复只要存在合法开始/结束时间，都显示耗时。
- 刷新、重新登录或切换会话后耗时不丢失；服务端时间戳是持久真源，前端运行时计时仅用于当前进行中的状态。
- 不允许出现负耗时、`NaN` 或极端异常时间；无合法结束时间时隐藏最终耗时。
- Desktop Footer 直接展示；移动端沿用现有元信息 Tooltip/信息按钮，避免挤压操作区。

## Acceptance Criteria (Evolving)

- [x] 侧边栏不再出现独立“对话列表”标题或重复会话。
- [x] 工作区展开后只显示归属于该工作区的会话。
- [x] 未绑定工作区的会话只显示在“临时对话”下。
- [x] 创建、移动、删除、重命名和搜索会话后，层级立即且正确更新。
- [x] 不同会话继续分别记住模型、Chat/Agent 模式和权限，切换会话互不污染。
- [x] Chat 完成回复后 Footer 显示本次耗时。
- [x] Agent 完成包含 Tool/RAG/搜索步骤的回复后，Footer 显示包含全部步骤的端到端耗时。
- [x] 失败和取消回复在有完成时间时显示耗时，运行中的回复不显示伪造的最终耗时。
- [x] 页面刷新并重新读取服务端消息后，耗时与刷新前一致。
- [x] Desktop 与 Mobile 的元信息展示不遮挡消息操作按钮，且具备可访问标签。
- [x] 相关 Frontend focused tests、lint、typecheck 与 production build 通过；现有 Backend 时间戳契约充足，无需 Backend 变更。

## Definition of Done

- Frontend 组件、状态映射、i18n 和 focused Vitest 覆盖已更新。
- 若服务端现有时间戳语义不足，再做最小 Backend contract 变更并补 Go tests；否则不新增冗余字段。
- 格式化、lint、typecheck 与相关测试通过。
- 用户可见行为和存储语义写入相应项目文档或 Trellis spec（如产生新约定）。
- 变更以 focused conventional commit 提交，不推送。

## Out of Scope

- 不把 PostgreSQL 会话改成 JSONL、SQLite 或 Event Sourcing 全量重构。
- 不增加首 Token 延迟、Tokens/s、费用等额外性能指标。
- 不实现整段会话的统计报表或监控 Dashboard。
- 不改变模型 Provider、RAG、记忆系统本身的执行策略。
- 不删除或迁移用户现有会话数据；只调整关联和显示方式。

## Technical Notes

- 侧边栏主文件：`mm-chat/frontend/src/components/layout/Sidebar.tsx`。
- 会话侧边栏 Store 入口：`mm-chat/frontend/src/features/chat/hooks/useSidebarSessions.ts`。
- 消息 Footer：`mm-chat/frontend/src/components/chat/MessageItem.tsx`。
- DTO 到 UI Message 映射：`mm-chat/frontend/src/services/api/chatCrudService.ts`。
- 前端消息 DTO 已包含 `createdAt` / `completedAt`：`mm-chat/frontend/src/services/api/client/types.ts`。
- 后端 DTO 已包含 `CompletedAt`：`mm-chat/backend/internal/chat/handler.go`。
- 运行过程已有 `ProcessStep.startedAt/completedAt/durationMs`，但单条回复总耗时应使用 Assistant 消息生命周期，避免把并行或重试步骤简单相加造成重复计时。

## Decision (ADR-lite, evolving)

**Context**: 产品同时支持普通 Chat、Agent、RAG、Tool 调用和刷新恢复，单靠浏览器计时无法成为稳定真源。

**Decision**: 保留 PostgreSQL 会话/消息模型；工作区只是会话的可空归属。回复总耗时优先由 Assistant 消息 `createdAt` 到 `completedAt` 计算，运行时本地计时只负责即时体验。

**Consequences**: 无需新增本地 Session 文件；刷新后可恢复相同耗时。若发现 `createdAt` 并非 Run 开始边界，则需在实现前补充明确的服务端 Run 时间字段，而不能用 Tool 子步骤累计值代替。

### Confirmed duration semantics

**Context**: 用户需要类似参考截图中 `用时 2分45秒` 的信息，而非性能诊断面板。

**Decision**: 采用单次回复端到端总耗时，覆盖 Chat/Agent 自动执行链中的 RAG、联网、Reasoning、LLM、Tool 与人工批准等待；整段会话累计时间和纯模型生成时间均不采用。

**Consequences**: Footer 对用户表达“这一轮实际等了多久”；首 Token、Tokens/s、费用和分阶段耗时继续保持 Out of Scope。
