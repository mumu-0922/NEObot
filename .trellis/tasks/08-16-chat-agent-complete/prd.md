# Chat Agent 完全体

## Goal

把 Neo Chat 的普通聊天升级成一个真正可执行的单 Agent：用户只需用自然语言说明
目标，Agent 自动匹配并加载已安装 Skill，选择可用 Tool，连续执行、观察结果、修正
失败并验证产物，最后才向用户汇报。继续使用本机单服务器和现有
`local_direct`，不要求新机器、sudo、Podman/OCI Runner 或每 Skill 隔离。

## User-visible target

用户看到的是一条简单闭环：

```text
说出目标
  -> Agent 判断要不要加载 Skill
  -> 自动调用 Skill / MCP / Web / Knowledge / Memory / Terminal / File Tool
  -> 根据 Tool Result 决定下一步
  -> 对产生的文件或变更做验证
  -> 成功后给出结果和证据；失败时说明真实阻塞点
```

典型最终体验：

- “把工作区的 CSV 合并成 Excel 并检查公式”会自动加载表格 Skill、读取文件、
  生成结果、重新读取或运行校验，然后给出产物位置。
- “拉取这个 GitHub 项目并分析源码”会选择 Web/Terminal/Git 能力，实际拉取并
  搜索源码，而不是只复述项目介绍。
- “修复这个项目的构建错误”会读取项目规则和文件、执行测试、修改、重跑测试，
  直到通过或遇到无法自行消除的真实阻塞。
- 页面刷新后仍能看到先前 Tool Call/Result；服务重启后可恢复会话状态，但不会
  偷偷继续长期任务，用户说“继续”后才恢复。

## Fixed product decisions

- 单 Agent 优先；默认 Tool catalog 不暴露 Subagent，且不存在递归派生。
- 继续使用现有 Go Backend、Provider adapters、PostgreSQL、SSE 和前端 Process UI。
- 继续使用 `local_direct`；不增加 Skill 沙箱、Runner、sudo 或新机器要求。
- Skill 来源仍由 Neo Chat Assistant/Skill Store 与服务端安装状态管理，不采用
  DeepSeek Harness 的本地目录覆盖优先级。
- 不引入 Cordis 或直接依赖 DeepSeek Harness 内部包；只吸收其已验证的行为模式。
- 默认限制仍需存在：Tool Call、Step、总运行时间、输出大小和并发都有上限。
- 现有 G20/G21 Orchestrator/Runner/Canary 保持独立且默认关闭，不作为 Chat Agent
  的执行前置条件。

## Architecture

### 1. Agent Turn/Step Driver

一次用户请求创建一个 Turn。Turn 中可以有多个 Step：

```text
Turn start
  -> Step start
  -> Provider streams assistant response
  -> no Tool Call: completion policy -> Turn end
  -> Tool Calls: Registry execute -> persist results -> next Step
```

取消、超时、预算耗尽、Provider 失败和 Tool 失败都必须成为稳定状态，不能靠散落
的特殊分支结束。Tool 失败默认作为结构化结果交还模型，由模型决定修正、换工具或
报告失败；只有取消和不可恢复的运行错误直接终止。

### 2. Unified Tool Registry

每个 Tool 注册同一份契约：

- name、description、JSON Schema；
- risk class：read / write / execute / external；
- timeout、output budget、是否允许并行；
- execute；
- model-facing result projector；
- replayable frontend presentation；
- optional approval rule。

第一批把现有 `search_web`、`search_knowledge`、`search_memory`、MCP Tools、
`skill`、`terminal` 全部迁入 Registry，不改变 Provider 的原生 continuation 结构。

### 3. Skill selection and loading

用一个 `skill({name})` 替代 `skills_list + skill_view(SKILL.md)` 主路径：

- 每 Step 注入当前用户完整但有界的 name/description catalog；
- 用户点名 Skill 或任务明显匹配描述时，模型必须在行动前加载；
- `/skill-name` 确定性加载，不依赖模型猜测；
- 同一 catalog revision 下已经加载的 Skill 不重复加载；
- catalog 变化发布完整 replacement，清空时发布 tombstone；
- Skill 的 references/assets/scripts 仍通过受控资源读取或激活后的
  `$NEO_CHAT_ACTIVE_SKILL_ROOT` 使用。

### 4. Durable Chat Agent Event Log

新增 Chat Agent 自己的 append-only Event Log，并关联现有 conversation、message、
run。至少记录：

```text
turn.started / turn.ended
step.started / step.ended
assistant.message
tool.called / tool.result
goal.changed / goal.round.started
context.replaced
```

Event 使用连续 sequence、幂等 event ID 和原子追加。模型历史、前端过程回放和
恢复从同一投影生成。现有 assistant `metadata.processTrace` 在迁移期保留为兼容投影，
不再作为唯一事实来源。

不得把完整 Tool Result 塞进现有 `agent_run_events`；该表属于运行控制面且只允许
有限的状态事件与脱敏 detail。

### 5. Goal and completion policy

普通短任务可在一个 Turn 内完成；长任务使用 same-session Goal：

- active / paused / blocked / complete；
- revision compare-and-set；
- 自动 Goal Round 和总轮数上限；
- 用户可暂停、继续、取消；
- 重启恢复后 active Goal 默认 disarmed，只有用户明确“继续”才 re-arm；
- 同一阻塞至少连续三个 Goal Round 才允许标记 blocked。

若本 Turn 执行过 write/execute 类 Tool，最后一次变更之后必须出现成功的验证证据
才能完成。没有验证时，completion policy 自动注入下一 Step，要求检查产物；不得
只凭模型叙述判定成功。

### 6. Tool expansion

核心闭环稳定后补齐：

- File read/write/edit/search，写前读取并携带版本，外部修改后拒绝覆盖；
- Background Jobs：job list/output/kill/wait 和完成通知，避免忙轮询；
- Context Compaction：先裁剪超大 Tool Result，再做不切断 Call/Result 的摘要替换；
- Browser Tool 或 Browser MCP：打开页面、读取 DOM、点击和填写；不把普通 HTTP
  fetch 冒充浏览器；
- 安全只读 Tool 并行，write/execute 保持有序屏障；
- 可选 Code Mode `run_code` 作为后期减少模型往返次数的优化。

## Delivery plan

### Delivery status

- [x] G1 — Skill 自动加载。
- [x] G2 — Unified Tool Registry + explicit Turn/Step driver。
- [x] G3 — Durable Events + 前端回放。
- [x] G4 — Goal + 完成验证。
- [x] G5 — File Tools + Jobs + Compaction。
- [x] G6 — Browser/MCP + 可选优化。

### G1 — Skill 自动加载（最快看到变化）

- 新增统一 `skill({name})`。
- catalog 强制匹配提示、显式 `/skill-name`、replacement/tombstone。
- 保留旧接口一小段兼容期，但不再向模型展示 `skills_list`。
- 用 fake Tool-round Provider 验证“先加载 Skill，再执行 terminal”。

**阶段效果**：用户点名或明显匹配 Skill 时，聊天不再经常跳过 Skill。

### G2 — Tool Registry + 正式 Turn/Step

- 建立 Registry 契约与执行管线。
- 迁移现有 Local Skill、MCP、Web、Knowledge、Memory。
- 将 `web_tool_loop.go` 巨型 name switch 缩为 Registry dispatch。
- 统一错误、取消、预算和 Tool Result 回填。

**阶段效果**：Agent 可以在一个回答中连续使用不同类型 Tool，不再被检索专用循环
限制。

### G3 — Durable Events + 前端回放

- PostgreSQL migration、Repository、projection 和 SSE 映射。
- Tool Call/Result、Turn、Step 独立持久化。
- 前端从 Event projection 恢复过程卡片；兼容旧 `processTrace`。
- 覆盖断流、刷新、服务重启和 torn/incomplete Run 修复。

**阶段效果**：执行过程不因刷新或服务重启丢失，可知道 Agent 做到哪一步。

### G4 — Goal + 完成验证

- same-session Goal、pause/resume/cancel、round cap、human re-arm。
- write/execute 后验证门槛。
- 空闲时按 Goal Round 自动续跑，完成或真实阻塞才停止。

**阶段效果**：复杂任务不需要用户反复发“继续”，且 Agent 不会未验证就宣布成功。

### G5 — File Tools + Jobs + Compaction

- 版本保护的 File Tool 套件。
- 进程内 Background Jobs 与完成通知；UI 明确标注服务重启后 Job 不可恢复。
- Tool Result pruning、summary compaction、context-overflow retry。

**阶段效果**：可以稳定完成编码、批处理和长时间任务，长对话不会轻易撑爆上下文。

### G6 — Browser/MCP + 可选优化

- 增加固定 `@playwright/mcp@0.0.79` 的真实 Browser MCP；每个 Chat Run 使用
  独立 Browser process identity，精确 allowlist 只开放 17 个 DOM/交互 Tool。
- 上游实际暴露的 `browser_run_code_unsafe`、`browser_evaluate`、上传和网络正文
  Tool 不进入 catalog，`tools/list` 与 `tools/call` 两端都 fail closed。
- 连续安全 read 最多 4 个跨 MCP/local backend 并行；write/execute/unknown、
  retrieval、Goal、`skill` 和 `mcp_tool_search` 保持原始顺序屏障，Result 仍按模型
  Call 顺序回填。
- Code Mode 结论：本阶段不增加通用 `run_code`。它只可能是后续可选的往返优化，
  不替代原生 Tool 路径；Playwright 的 unsafe code Tool 明确拒绝。

**最终效果**：形成接近 Hermes/DeepSeek Harness 使用体验的本机单 Agent，同时保留
Neo Chat 的 Store、服务端权限边界和现有 UI。

## Acceptance Criteria

### Skill and Tool behavior

- [ ] 用户明确点名已安装 Skill 时，第一个任务动作前必有一次成功 `skill` 调用。
- [ ] 明显匹配 catalog description 的测试请求会先加载对应 Skill。
- [ ] 同一 Turn 可执行至少三轮、两种不同 Tool，并把每次 Result 交回同一模型。
- [ ] 未知 Tool、坏参数、Tool 超时和普通 Tool 失败成为结构化 Result。
- [ ] 取消会停止 Provider 和在途 Tool，只产生一个 cancelled terminal event。
- [ ] Registry 中不存在默认 Subagent Tool。

### Persistence and recovery

- [ ] Turn、Step、assistant message、Tool Call 和 Tool Result 具有连续持久 sequence。
- [ ] 页面刷新后 Tool 卡片顺序、状态和最终回答一致。
- [ ] 服务重启后可重建会话；未结束 Tool/Step 被确定性标记为 interrupted，而非成功。
- [ ] 恢复的 active Goal 为 disarmed，用户明确继续后才启动下一 Goal Round。

### Completion quality

- [ ] write/execute 后无验证证据时，Agent 不能直接完成 Turn/Goal。
- [ ] 生成文件任务必须证明文件存在且通过对应格式/内容校验。
- [ ] 代码任务必须报告实际执行的检查及其结果，不能伪造通过。
- [ ] 达到 Call/Step/时间/输出预算时安全停止并给出真实原因。

### Compatibility and operations

- [ ] 现有 Provider 原生 Tool continuation fixtures 全部继续通过。
- [ ] 现有 Web/Knowledge/Memory/MCP/Local Skill UI 与取消行为无回归。
- [ ] 不要求 sudo、新机器、Podman/OCI Runner 或 Skill 隔离。
- [ ] `AGENT_LOCAL_RUNTIME_ENABLED=false` 仍可立即关闭本地执行而不删除安装和工作区。
- [ ] 新 migration 支持 up/down/up replay；旧 message 的 `processTrace` 仍可读取。

## Definition of Done

- Backend focused tests、migration replay/schema tests、`go vet ./...`、`go test ./...` 通过。
- Frontend format、lint、typecheck、Vitest 和 build 通过。
- `bash mm-chat/scripts/verify-standalone.sh --full` 通过。
- 更新 Chat Tool、SSE/Event、数据库、local_direct 部署和用户测试文档。
- 每个 Gate 独立提交、可通过 feature flag 回滚，运行数据目录不被删除或重写。

## Out of Scope

- 默认或递归 Subagent。
- 将开发机强制改成 OCI/Podman Runner host。
- 每 Skill 容器或 sudo 安装链。
- 一开始就复制 Cordis、Workflow/Ralph 或 Code Mode。
- 宣称 Background Job 在服务重启后仍能继续。
- 用 HTTP fetch 假装完成 GUI Browser 行为。

## Decision (ADR-lite)

**Context**：当前 Chat 已有 Provider Tool round，但 Skill 选择弱、工具执行分散、过程
只在最终 message metadata 中投影；独立 G20/G21 Runtime 又没有连接普通 Chat。

**Decision**：在现有 Chat 路径内渐进建立单 Agent Core，复用 Provider adapters、
Store、PostgreSQL、SSE、Process UI 和 `local_direct`。按 Skill → Registry/Loop →
Events → Goal → Tools/Compaction 的顺序交付。

**Consequences**：用户会较早看到 Skill 自动调用；中期需要一次数据库与前端回放迁移；
旧 control-plane Runtime 保持独立，避免让本机用户承担不需要的部署复杂度。

## Research References

- [`research/deepseek-harness-source-review.md`](research/deepseek-harness-source-review.md)
  — 完整源码审阅与 Neo Chat 差距映射。

## Technical Notes

- 当前主循环：`mm-chat/backend/internal/chat/web_tool_loop.go`。
- 当前 Skill Tool：`mm-chat/backend/internal/chat/local_skill_tool_loop.go`。
- 当前 UI trace：`mm-chat/backend/internal/chat/process_trace*.go` 与
  `mm-chat/frontend/src/lib/chat/processTrace.ts`。
- 当前 Chat contract：`mm-chat/docs/contracts/chat-tool-loop.md`。
- 当前 local_direct contract：`mm-chat/docs/deployment/local-skill-runtime.md`。
- G20/G21 control plane 不作为本任务 Agent Loop 的基础执行路径。
