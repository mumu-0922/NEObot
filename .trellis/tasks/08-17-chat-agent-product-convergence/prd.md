# Chat/Agent 产品收口与旧控制面退役

## Goal

把 Neo Chat 收口成一个用户无需理解 MCP、Runner 或 Canary 的统一聊天产品：同一个
输入框可切换 Chat 与 Agent；Chat 保留知识库、记忆和联网搜索，Agent 额外自动使用
Skill、文件、Terminal、Browser 和外部 Connector；生成的文件直接回到聊天中下载。
在新链路稳定后，删除与普通聊天完全断开的旧 G20/G21 Agent 控制面。

## What I already know

- 当前普通 Chat 已具备 Unified Tool Registry、Turn/Step、Skill 自动加载、Durable
  Events、Goal、File、Terminal、Jobs、Compaction 和 Browser MCP。
- 当前 `streamMessageRequest` 没有明确 Chat/Agent 模式；模型支持 Tool 时，普通发送会
  准备 MCP、local Skill 和 Goal，因此产品层无法区分“只聊天”和“替我执行”。
- 当前输入框直接展示 `McpToolsControl`，并在发送前执行 MCP Preflight，用户被迫理解
  MCP；这不是目标产品体验。
- LobeHub 源码确认 Chat/Agent 是持久化 Runtime Policy，不是清空插件列表；模型不支持
  Tool Use 时有效模式自动降级 Chat。
- LobeHub Chat 模式严格只允许 Knowledge、Memory、Web Search 和显式 Image Tool；
  Agent 模式才开放 Browser、File、Environment、Skill、Task 和 Connector。
- Neo Chat 的旧 `agent_*` 控制面数据库表当前全部为 0 行；正在使用的
  `chat_agent_turns` 有 3 行、`chat_agent_events` 有 73 行，二者不能混删。
- 本机当前只运行 frontend、backend、mcp-runner、postgres、redis、minio；八个旧
  Agent Runtime/Canary Compose 服务均未运行。
- 用户要求：本机就是测试机、`local_direct`、不使用 sudo/OCI/每 Skill 隔离、禁止
  Subagent、不 push，普通开发决策直接按推荐执行并提交。

## Fixed product decisions

### Mode

- Conversation config 持久化 `toolMode: "chat" | "agent"`。
- 缺省或历史 Conversation 映射为 `agent`，保持当前 Tool 行为和 LobeHub 的兼容语义。
- 模型 Tool 能力未加载时保留用户选择；确认不支持 Tool 后，有效模式自动降级 Chat，
  不因已安装 Skill/MCP 返回 409。
- Chat 模式允许 Knowledge、Memory、Web Search 和现有非 Tool 图像生成路径。
- Chat 模式物理移除 Goal、Skill、File、Terminal、Jobs、Browser/MCP Tool definitions，
  不能只靠 Prompt 要求模型别调用。
- Agent 模式使用现有完整 Registry，Agent 自动选择 Tool；不增加 Subagent。

### Product surface

- 输入框增加一个清晰的 `Agent / Chat` 胶囊开关，显示当前有效模式和模型不支持原因。
- 从主输入框移除 MCP 专有控件和发送前 MCP Preflight。
- Sidebar 的 Tool 页面保留，但产品文案改为“工具/连接器”；MCP 仅作为高级实现名。
- Knowledge、Memory、Search、Reasoning 控件继续共用，不复制第二套聊天页面。

### Browser and deliverables

- 修复 Chromium `Socket path too long`，保持固定版本 Browser MCP 和当前 allowlist。
- Browser MCP 可作为底层 Provider 保留，但聊天 UI 只展示 Browser Tool 行为。
- 新增安全的 workspace 文件发布/下载闭环：Agent 明确发布产物后，文件进入现有对象
  存储与权限链，并在同一消息中显示可下载卡片。
- 不把任意本机路径直接暴露成下载 URL；发布动作必须显式、授权、限额且可回放。

### Legacy retirement

- Skill Store 从 Agent Center 拆出并继续使用 `/v1/skills/*`。
- 删除 Agent Center 的 Shadow、Canary Runs、Schedules、Learning 和控制面状态 UI。
- 删除默认关闭且不服务普通 Chat 的 G20/G21 Runner、Orchestrator、Broker、Delegation、
  Cron/Learning Worker、Canary commands/services/config；未来 Task/Schedule 基于 Chat Agent
  重建，不复用 Canary Frozen Spec 产品形态。
- 不从迁移历史中间直接删除 `084-095`；先增加向前 cleanup migration，在确认表仍为空并
  完成备份后删除旧表/权限，再决定是否于未来基线 squash。
- `metadata.processTrace` 与隐藏 `skills_list/skill_view` 只在兼容读取证明完成后另行退役。

## Delivery plan

### G1 — Chat/Agent 模式与 Backend Policy

- Conversation 模式持久化、前端切换和模型能力降级。
- Backend Tool Registry 按有效模式构建物理 allowlist。
- Chat 模式不准备 MCP/local Skill/Goal，不触发 Tool 不支持错误。
- 覆盖刷新、历史 Conversation、模型切换和两模式消息发送。

### G2 — MCP 产品退后

- 从输入框移除 `McpToolsControl` 和 client-side preflight。
- 保留 Tool/Connector 管理页、安装、认证、选择和 Backend MCP execution。
- 普通用户文案不要求理解 MCP；错误指向“连接器/工具设置”。

### G3 — Browser 修复

- 缩短 Browser Run 的临时/Socket 路径并证明并发 Run 身份仍隔离。
- 真实浏览器 smoke：打开网页、读取 DOM、交互、截图，刷新后回放 Tool 卡片。

### G4 — 可下载成果

- 设计并实现显式 `publish_file`/等价 Tool 与消息 Artifact projection。
- 生成文件、验证、发布、权限下载、刷新回放和删除/失效路径闭环。

### G5 — 旧控制面退役

- 拆出 Skill Store。
- 删除 Agent Center 控制面、八个 Compose 服务、对应 commands/modules/tests/docs/env。
- 前向 migration 清理空控制面表；保留 `chat_agent_*`、Skill、MCP 和聊天数据。
- 更新部署、备份、恢复、配置和 rollback 文档。

## Acceptance Criteria

### Mode behavior

- [ ] 输入框可切换 Chat/Agent，刷新后选择不丢失。
- [ ] 历史 Conversation 未设置模式时保持现有 Agent 行为。
- [ ] Tool-incapable 模型自动使用 Chat，不因 MCP/Skill 返回冲突。
- [ ] Chat 模式模型请求中没有 Skill/File/Terminal/Job/Goal/MCP/Browser definitions。
- [ ] Chat 模式仍可使用 Knowledge、Memory 和 Web Search。
- [ ] Agent 模式可连续调用 Skill、File/Terminal 和 Browser/MCP，并回填同一模型。

### Browser and deliverables

- [ ] Chromium 不再因 Socket path 过长失败。
- [ ] Browser 真实交互 smoke 通过，危险 Tool 仍不进入 allowlist。
- [ ] Agent 生成文件后，聊天中出现受权限保护的下载卡片。
- [ ] 刷新后下载卡片与 Tool 时间线一致，跨用户访问失败。

### Retirement

- [ ] Skill Store 在删除 Agent Center 后仍可安装、列出和卸载 Skill。
- [ ] Compose 不再包含旧 Agent Runtime/Canary 服务和无效配置。
- [ ] Subagent/Delegation/OCI Runner 产品代码不再参与构建。
- [ ] Cleanup migration 只清理已确认空的旧 `agent_*` 控制面，不触碰
      `chat_agent_*`、聊天、Skill、Knowledge、Memory、MCP 或文件数据。
- [ ] PostgreSQL migration replay、备份/恢复和 standalone full gate 通过。

## Rollback

- G1/G2 使用模式 policy 与 UI 独立提交；回滚后恢复当前全 Tool Chat 行为。
- Browser 修复只改变短路径分配，不改变 Tool allowlist，可独立回滚。
- Artifact 发布为新增能力，不复用或重写旧文件。
- 旧控制面删除前先保留 source tag/commit、数据库备份和 forward cleanup down/restore
  说明；不得直接重写运行数据目录或 live `.env.single-server`。

## Out of Scope

- Subagent、递归派生、Group Agent。
- OCI/Podman/Sandbox、sudo、新机器或每 Skill 隔离。
- Cloud Runner、Remote Device、企业 Workspace 权限模型。
- 本任务内接入微信；只保证 Runtime/Mode 可被未来渠道复用。
- 完整复制 LobeHub Task/Group/Heterogeneous Agent 架构。

## Research References

- [`research/lobehub-product-runtime-review.md`](research/lobehub-product-runtime-review.md)
  — LobeHub 源码事实与 Neo Chat 对照。

## Definition of Done

- 各 Gate 有聚焦测试并独立提交，不 push。
- Frontend format/lint/typecheck/test/build 通过。
- Backend `go vet ./...`、`go test ./...`、相关 PostgreSQL 17 drill 通过。
- `bash mm-chat/scripts/verify-standalone.sh --full` 通过。
- 配置、部署、数据迁移、安全边界和用户测试文档同步。

