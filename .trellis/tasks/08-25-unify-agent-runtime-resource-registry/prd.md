# Unify Agent Runtime Resource Registry

## Goal

在不替换 Go Agent loop、PostgreSQL 会话权威和 Host Runner 安全边界的前提下，
把 built-in Tools、Skills 与 MCP/Plugin resources 收口到一个可诊断、可启停、
可回放的 Runtime Resource Registry，完成上一任务定义的 Phase C。

## What I already know

- Phase A+B 已将模型可见本地工具统一为 `read/write/edit/grep/bash`，并完成自然
  completion 和 workspace-file open-first 链路。
- Backend 已有 server-owned Tool Registry，但 Skills、MCP selection、manifest、
  Store/UI 生命周期仍由多个入口拼装。
- Pi 把 extensions、skills、prompts、themes 作为 package resources 统一加载；
  Neo Chat 不能直接移植 Pi SDK，必须保留现有用户隔离、approval、permission、
  MCP allowlist 和 durable event log。
- 上一阶段明确 Phase C 不应重新实现会话存储或第二套 Tool authority。

## Assumptions (temporary)

- 本阶段优先统一 Backend runtime contract、诊断与原子启停，不把 Skill Store 和
  MCP 设置页整体视觉重做。
- 历史会话、已安装 Skill、既有 MCP Server/manifest 选择必须继续工作。
- Registry 是每个 Agent Step 从当前授权状态构建的只读快照，不成为新的数据库
  事实源。

## Requirements

- 一个 runtime snapshot 汇总 builtins、Skills 与 MCP/plugin tools，并记录来源、
  scope、启用状态、风险分类、冲突和不可用诊断。
- Resource 启停必须原子影响该资源贡献的 Tool/Skill/Prompt，而不能留下半启用状态。
- 项目级资源必须经过现有 Workspace trust/permission authority。
- 名称冲突、失效安装、Provider Tool 能力不足和 Runner/MCP 不可用必须 fail closed。
- 保留旧配置、历史 Tool 名和 durable trace 的兼容读取。
- Snapshot 分为两层：Run-level authority 固定 MCP selection、Skill installation
  materialization 和 workspace/permission；Step-level projection 根据当前可见 MCP
  Tool、首轮检索约束和已加载 Skill 生成不可变 Tool catalog。
- Resource descriptor 至少包含稳定 ID、kind、source、scope、status、revision、
  contributed Tool/Skill names 和脱敏 diagnostic code；绝不包含 credential、Host
  path、Skill cache path、Tool arguments 或结果。
- 保持现有 admission 语义：显式选中的 MCP 或 Skill runtime 准备失败仍然 fail
  closed，不在本任务中改成静默跳过；冲突只移除冲突资源，不覆盖先到者。
- Handler 只消费 snapshot 提供的 prompt、Tool registry 和 runtime handles，不再
  分别拼接 MCP/Skill/builtin catalog。

## Acceptance Criteria

- [x] 同一 Agent Step 只从一个 immutable runtime snapshot 构建 Provider Tool catalog。
- [x] 现有 authority 禁用一个 resource/package 后，其贡献不会进入下一 Run；
  已接受 Run 继续使用冻结快照，且无关资源不受影响。
- [x] 冲突和不可用状态有结构化、脱敏诊断，不静默覆盖。
- [x] builtins、Skill、MCP 三类资源均有跨层测试与回放兼容测试。
- [x] 不新增第二份会话、安装或 Tool authority。
- [x] Snapshot revision 对同一 authority/input 稳定，任何资源、可见 Tool 或
  first-step projection 变化都会产生新 revision。
- [x] 结构化诊断不包含 secret、URL credential、绝对 Host path、Skill RootPath、
  Tool arguments 或 Tool Results。
- [x] required Skill prelude 使用同一 snapshot 的受限投影，不另造平行 registry。

## Definition of Done

- Tests added/updated across Backend and Frontend where contracts change.
- Go vet/tests, Frontend format/lint/typecheck/test/build, and full standalone gate pass.
- Specs, product contracts, module docs, rollout, and rollback are synchronized.
- Live Backend/Frontend deployment remains healthy with unrelated services unchanged.

## Out of Scope (explicit)

- 将 Go Agent runtime 替换为 Pi SDK。
- 重写 PostgreSQL 会话、Skill 安装存储或 MCP transport。
- 在本阶段实现第三方 Marketplace 下载、远程包签名或主题系统。
- Approach B 的统一 Resources API/UI，以及跨 Skill/MCP 的统一开关。
- 改变现有 Skill 安装、MCP selection、显式资源失败的用户行为。

## Technical Notes

- Previous research: `.trellis/tasks/archive/2026-08/08-25-align-agent-harness-with-pi/research/pi-agent-harness-comparison.md`.
- Product root is `mm-chat/`; runtime state under `mm-chat/data/`, `secrets/`,
  `backup/`, and `.env.single-server` is protected.

## Research References

- [`research/runtime-registry-options.md`](research/runtime-registry-options.md)
  — Pi 与 Neo 当前资源生命周期对照，以及三个可执行分层方案。

## Research Notes

- **Approach A（推荐 MVP）**：先统一 immutable runtime snapshot、revision、
  source/scope/status/diagnostics 和 Tool/Skill assembly，不改现有管理 UI。
- **Approach B**：在 A 上增加脱敏 API 与统一 Resources 诊断页，但仍复用 Skill
  Store/MCP 的现有 mutation authority。
- **Approach C**：直接做完整 Pi package manager；需要新的 package/trust/prompt/
  extension 安全模型，本任务不应贸然承载。

## Decision (ADR-lite)

**Context**：Neo 已有一个 Tool Registry，但 Handler 仍分别准备 MCP、Skills、
builtins 与 prompt；直接做完整 Plugin Package Manager 会新增未经设计的执行与
信任边界。

**Decision**：选择 Approach A。建立 server-owned、immutable、revisioned 的
Runtime Resource Snapshot，统一资源描述、诊断、prompt 和 Tool catalog assembly；
现有 MCP/Skill persistence 继续作为唯一 authority。

**Consequences**：本次能先消灭运行时拼装漂移并建立扩展 seam，但用户仍从 Skill
Store 与 Tools/Connectors 两处管理资源。统一 Resources UI 与 package-level
atomic toggle 延后到独立任务。

## Expansion Sweep

- **Future evolution**：descriptor/diagnostic shape 为未来只读 Resources API 和
  package grouping 保留扩展点，但本次不公开路由。
- **Related scenarios**：required Skill prelude、MCP Tool Search、Memory/Knowledge/
  Goal first-step restrictions 必须全部由 snapshot projection 覆盖。
- **Failure/edge cases**：collision、失效安装、MCP 不可用、Provider 无 Tool 能力、
  snapshot revision drift、hidden legacy alias 和脱敏均进入测试矩阵。

## Technical Approach

1. 新建 `agentRuntimeResourceSnapshot`、descriptor、diagnostic 和稳定 revision
   builder；输入只引用既有 prepared runtimes。
2. 将 `chatToolRegistry` 改为 snapshot 的执行投影，保留现有 scheduler/executor，
   并让 required Skill prelude 走同一 projection 方法。
3. Handler 在完成现有 MCP/Skill authority preparation 后只创建一次 Run snapshot，
   每 Step 从它派生 immutable Tool projection 和 Skill prompt。
4. 增加 collision、source/scope、revision、redaction、first-step、MCP search
   expansion、Skill prelude、legacy alias 和自然 completion 回归测试。
5. 同步 Trellis spec、产品 contract 和模块文档；完成全仓验证与定向部署。

## Implementation Plan

- **PR1**：snapshot types/builder、revision 与安全诊断测试。
- **PR2**：Tool registry、required Skill prelude 和 Handler 接线。
- **PR3**：跨层回归、文档、全量验证、部署与回滚检查。
