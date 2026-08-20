# 修复只读 Agent 完成验证误判

## Goal

让 Chat 的 Agent 模式能够自然完成仅使用前台 `terminal` 的任务，尤其是
`pwd`、`git status --short` 这类只读检查；同时保留结构化写操作和后台任务的
完成证据门禁，避免 Agent 在真正修改状态后未经检查就宣称完成。

## Requirements

- 成功的前台 `terminal` 调用不再产生待验证 mutation；Terminal-only Turn 可以
  直接生成最终答复。
- 不解析 Shell command 文本来猜测只读/写入；所有前台 Terminal 采用同一完成
  边界，降低 Shell 语义误判和绕过风险。
- `runInBackground=true` 的 Terminal 启动仍按精确 Job ID 产生待验证 mutation；
  只有同一 Job 后续成功且 `status=completed` 的 `job_output` 可作为完成证据，
  多个后台 Job 分别收口。
- `file_write`、`file_edit`、`job_kill`、MCP write 以及其他明确 `write` Tool
  继续触发完成证据门禁。
- 前台 Terminal 的成功结果仍可验证先前的结构化 mutation，例如写文件后的测试
  命令。
- 成功的本地 Tool Result 必须显式向同模型提供精确
  `evidenceToolCallId`，让模型无需猜测 Provider Tool Call ID 即可调用
  `verify_completion`。
- 更新模型 Completion instruction，明确 Terminal-only 不调用
  `verify_completion`，结构化 mutation 后引用后续检查结果中的精确 ID。
- 不改变 Terminal 的审批、路径、超时、输出限制、并发、进程组取消和非 Sandbox
  边界。

## Acceptance Criteria

- [x] `pwd` / `git status --short` 的 Agent 回归测试成功完成，不进入
      `AGENT_VERIFICATION_REQUIRED`。
- [x] 前台 Terminal-only 流程不要求、也不调用 `verify_completion`。
- [x] `file_write -> terminal check -> verify_completion` 仍可完成，且错误/陈旧
      Evidence ID 仍被拒绝。
- [x] 后台 Terminal 启动和运行中的 `job_output` 均不能完成验证；仅同一 Job
      成功的 `job_output(status=completed)` 可以完成验证，其他 Job 或前台
      Terminal 证据被拒绝。
- [x] `file_write`、`file_edit`、`job_kill` 与 MCP write 的 mutation 门禁不回退。
- [x] 本地 Tool 成功结果包含与 `ProviderToolResult.CallID` 一致的
      `evidenceToolCallId`。
- [x] `go test -race ./internal/chat`、`go vet ./...`、`go test ./...` 通过。
- [x] `verify-agent-local-runtime.sh` 与 `verify-standalone.sh --full` 通过。
- [ ] 只重建并重启 Backend 后，live 会话复测用户原话成功完成且 SSE 无错误。

## Definition of Done

- 代码、Focused tests、Backend 全量检查与 standalone gate 全绿。
- Backend executable contract 和运行记录同步。
- Live Backend 使用新镜像，旧 Backend 镜像保留为回滚入口；不迁移数据库、不删除
  Workspace 或其他 runtime state。
- 真实 Agent 会话证明 `requestedToolMode=agent`、`toolMode=agent`、Terminal 成功、
  Assistant completed。

## Technical Approach

在现有 `chatCompletionPolicy.observe` 中复用 Tool Registry 分类，但把“是否产生待
验证 mutation”从简单的 `write || execute` 收紧为独立判定：明确 `write` 保持
mutation；`execute` 默认保持 mutation，唯独成功的前台 `terminal` 不产生
mutation。Terminal 是否后台运行由结构化 Result 中的 `result.status=running`
识别，不解析命令字符串；待完成后台任务按 Result 中的 `jobId` 独立追踪。

本地成功 Result 统一增加 `evidenceToolCallId`。这只是同模型 Tool continuation
中的 Provider ID，不写入公开业务 DTO，也不改变现有 Process Event 的脱敏规则。

## Decision (ADR-lite)

**Context**: 当前 Registry 将全部 Terminal 标为 `execute`，Completion Policy 又将
每次成功 `execute` 当成状态修改。只读命令因此被迫验证；模型的 Result Content
没有精确 Tool Call ID，又导致 `verification_evidence_invalid` 循环。

**Decision**: 不分析 Shell command；以前台/后台执行边界区分 Completion Policy。
前台 Terminal 自带同步退出边界且不产生 outstanding mutation，后台 Terminal
必须等待成功的 completed Job Result。结构化写操作继续严格 read-back 验证。

**Consequences**: 恢复 Hermes/DeepSeek Harness 式自然 Terminal Agent 体验。前台
Terminal 仍可能运行写命令，但系统不再声称能可靠从任意 Shell 文本识别副作用；
明确的 File/MCP/Job 写接口继续保留强验证。

## Out of Scope

- 不新增 Shell AST、命令 allowlist 或只读命令分类器。
- 不改变本地执行权限、Workspace 挂载、sudo/隔离策略或审批模式。
- 不新增 Subagent、MCP Tool、数据库表或 migration。
- 不修改 Frontend、PostgreSQL、Redis、MinIO、MCP Runner。

## Technical Notes

- 失败会话：Conversation `6eb750e8-ab96-4c68-9c30-e7e9d0bfa541`，Run
  `1073af86-1995-44da-af34-976472133a41`。
- 两次 `verify_completion` 均为 `verification_evidence_invalid`；最终错误
  `AGENT_VERIFICATION_REQUIRED`。
- 主要改动面：
  `backend/internal/chat/chat_completion_policy.go`、
  `backend/internal/chat/local_skill_tool_loop.go` 及相邻测试。
- 研究记录：[`research/runtime-root-cause.md`](research/runtime-root-cause.md)。
