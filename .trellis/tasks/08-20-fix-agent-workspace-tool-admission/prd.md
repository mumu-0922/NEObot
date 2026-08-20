# 修复 Agent 模式未注入工作区工具

## Goal

修复用户明确选择 Agent 后，首次使用尚未缓存 Tool capability 的模型却被静默降级为
Chat、直接回答“没有工具”的问题；部署包含 `publish_file` 与旧控制面退役结果的当前
Backend/Frontend，使同一请求真实完成文件创建、读取验证和下载发布。

## Runtime Evidence

- 失败会话 `8b6bc391-ac3c-4b71-a76e-c2deac14b339` 持久化
  `metadata.toolMode=agent`，模型为 `openai_compatible/gpt-5.6-sol`。
- 失败 assistant message 同时记录 `requestedToolMode=agent`、`toolMode=chat`，只有
  generation trace，没有 Tool Call。
- 同一请求异步写入 capability cache：`gpt-5.6-sol` 为 `supported`、
  `structured_tool_call`；说明首次请求在 probe 完成前把 `unknown` 当成 unsupported。
- 当前运行 Backend/Frontend image 均构建于 2026-08-17 14:58 左右；Backend 有
  File Tools，但没有后续提交增加的 `publish_file`，且仍包含已退役的 legacy Agent
  binaries。数据库 migration head 为 `097`，当前源码 head 为 `098`。
- `AGENT_LOCAL_RUNTIME_ENABLED=true`，Backend 已正确挂载 `/workspace` 与 Skill root；
  不是 sudo、挂载或开关问题。
- 候选 Backend 首次迁移被 runner 拦截：生产 `096_chat_agent_event_log` checksum 为
  `f7c6227d...`，源码却因后续 commit 直接改写已应用 SQL 而变成 `ec82b421...`。
  生产函数已具有修复后的行为，但迁移账本仍正确保留原始 checksum；不得手工改写账本。

## Requirements

- 持久化 Agent 模式下，只有 capability **明确 unsupported** 才允许降级 Chat。
- capability cache miss/unknown 时复用 singleflight probe，并在首次 Agent 请求内有界
  等待；probe 返回 supported 后，同一请求必须直接进入完整 Agent Tool Registry。
- probe 因暂时性错误仍为 unknown 时，不得再把它解释成已确认 unsupported；对具有
  `ToolRoundProvider` adapter 的 Agent 请求保留原生 Tool 路径，让既有 runtime
  incompatibility/Provider error 链决定结果。
- Chat 模式继续保持非阻塞 capability 解析，不因为 Agent 修复增加普通聊天首包延迟。
- 保留 provider/model override、cache TTL、显式 unsupported 降级和 prewarm 行为。
- 增加聚焦测试，覆盖首次 supported probe、并发 singleflight、unknown fail-open-to-
  native-Agent、explicit unsupported downgrade 与 Chat 非阻塞。
- 恢复 migration `096` 的已应用原始字节并锁定其生产 checksum；以新的 forward-only
  migration `099` 幂等重放两处函数修复，保留 hardened `search_path` 与最小权限，禁止
  修改 `schema_migrations.checksum`。
- 构建新的不可变本机 Backend/Frontend image；Backend 必须包含 `publish_file` 且不含
  legacy Agent binaries。
- 升级 migration `098` 前创建并校验匹配的 PostgreSQL/MinIO pre-deploy backup；运行
  migration 后只重建 Backend/Frontend，不改写 `data/`、`secrets/` 或 live workspace。
- 用真实用户路径复测“创建 -> 读取验证 -> publish_file -> 下载卡片”。

## Acceptance Criteria

- [x] 首次 Agent 请求遇到 capability cache miss 时，supported probe 完成后同一请求
      收到 `file_read/file_write/file_edit/file_search/terminal/publish_file` 等 Agent Tools。
- [x] unknown/transient probe 不会产生 `requestedToolMode=agent`、`toolMode=chat` 的
      假性能力降级；明确 unsupported 仍降级 Chat。
- [x] Chat 请求保持非阻塞且物理不注入 Agent-only Tools。
- [x] 聚焦 Go 测试、`go vet ./...`、`go test ./...` 通过。
- [x] `verify-agent-local-runtime.sh`、migration `098/099` PostgreSQL 17 drills、Frontend
      format/lint/typecheck/test/build 与 standalone full gate 通过。
- [x] 生产数据库 migration head 为 `099_chat_agent_event_log_function_repair`，`096`
      checksum 保持 `f7c6227d...`，服务健康。
- [x] 真实 Agent 请求创建并验证 `agent-test.md`，聊天出现受权限保护的下载卡片。

## Definition of Done

- 根因修复、回归测试、Backend/Frontend/operations specs 同步并提交，不 push。
- 新镜像与数据库升级均有可验证的备份/回滚点。
- 工作区 clean；runtime `data/`、`secrets/`、`backup/`、live env 不进入 Git。

## Technical Approach

1. 将 capability probe singleflight state 从“仅 running 标记”改为可等待且可广播结果
   的 bounded state；保留现有非阻塞 resolver。
2. 新增 Agent-request resolver：cache miss 时等待同一 probe；supported/unsupported
   使用确证结果，unknown 对 adapter-capable Agent 保持原生 Tool admission。
3. Handler 在读取 persisted Conversation mode 后选择 Agent/Chat capability 策略。
4. 添加 handler/unit regression，更新 Backend/Frontend mode contract 对 unknown 的定义。
5. 恢复已应用 `096` 原始字节，新增 `099` 前向函数修复，并更新所有受 schema head
   影响的 PostgreSQL drill 与部署合同。
6. 完成备份、构建、migration、recreate、健康与真实聊天验收。

## Decision (ADR-lite)

**Context**：后台旧逻辑为降低普通 Chat 首包延迟，把 capability unknown 非阻塞降级，
但 Agent 模式因此把“尚未探测”错误等同于“不支持”，违反用户明确执行意图。

**Decision**：只对 persisted Agent 请求有界等待共享 probe；确证 unsupported 才降级，
transient unknown 继续走 adapter 支持的原生 Tool 链。Chat 仍非阻塞。

**Consequences**：某模型首次 Agent 请求可能多一次最长 20 秒的 capability probe，但只在
cache miss/expiry 时发生；换来首个 Agent 请求不丢任务、不伪装成无工具 Chat。

## Out of Scope

- 不增加 Subagent、递归派生、sudo、OCI/Podman 或 Skill 沙箱。
- 不改变 Tool capability cache schema/TTL。
- 不自动重放已经完成的失败消息；部署后发起新的验收请求。
- 不删除或重写用户工作区与已安装 Skill。
