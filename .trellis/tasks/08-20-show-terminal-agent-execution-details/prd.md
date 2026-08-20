# 展示 Terminal Agent 执行详情

## Goal

让 Agent 模式的 Process Trace 不再只显示重复的 `Terminal` 名称，而是像
DeepSeek Harness 一样提供可读、可回放的终端执行卡片，明确展示执行的
命令、工作目录与完成状态，同时不泄漏 raw Tool output、真实 Workspace
路径或常见凭据。

## What I already know

- 用户截图中的 live assistant message 为
  `101943cd-7c79-4160-9afa-13a75886757b`，`requestedToolMode=agent`、
  `toolMode=agent`，两次 Terminal 均成功。
- Backend persisted `argumentSummary={"timeoutSeconds":30}`，没有 command、
  cwd 或 result presentation；Frontend 又按现有契约不渲染
  `argumentSummary`，所以两个步骤只能显示同名 `Terminal`。
- DeepSeek Harness 使用 durable `tool/call → tool/result`、Tool-owned
  `presentCall/presentResult` 和 tagged Terminal Card，保证 live/replay 使用
  同一展示模型。
- neo-chat 已有 durable Agent events 与 `process.step.updated`，无需新增
  sideband、表或迁移。

## Requirements

- 为 `ProcessStep` 增加独立于 `detail` 的 typed terminal presentation。
- Terminal running event 带 bounded/redacted command 和 cwd；terminal event
  带 exit code、timed-out、truncated、background 状态。
- 真实 Workspace/Host Workspace/Skill cache 路径必须替换为稳定别名。
- command/cwd 必须经过现有 secret redaction 和 UTF-8-safe byte bounds。
- 不在 Process Trace、durable Agent event 或 SSE presentation 中加入 raw
  stdout/stderr。
- Frontend 必须严格 normalize presentation；未知/malformed presentation
  fail closed by dropping the presentation while retaining the valid step.
- Process Trace collapsed row 显示 command preview；展开后显示 terminal-like
  card，包括 cwd、command 与状态 pill。
- Live SSE 和 Conversation reload 必须产生同一 presentation。
- 非 Terminal Tool 与无 presentation 的历史消息保持现状。

## Acceptance Criteria

- [ ] running Terminal step 可见脱敏 command/cwd，completed step 可见 exit 0。
- [ ] nonzero/timed-out/truncated/background 状态有明确视觉语义。
- [ ] Workspace/Host/Skill internal paths and common credential forms are redacted.
- [ ] raw stdout/stderr never appears in process presentation/event payload.
- [ ] fragmented live updates and durable replay normalize to the same card.
- [ ] malformed or unknown presentation is ignored without invalidating the Tool step.
- [ ] focused Backend and Frontend tests pass; no unnecessary full suite.
- [ ] only Backend and Frontend are rebuilt/recreated for live rollout.

## Definition of Done

- Backend and Frontend focused tests, targeted format/lint/type checks pass.
- Specs document the typed presentation and output-retention boundary.
- Live Backend/Frontend health and image IDs are verified; unrelated container
  identities remain unchanged and rollback artifacts are retained.
- Changes are automatically committed, task archived and journal recorded;
  never auto-push.

## Out of Scope

- Persisting or rendering raw Terminal stdout/stderr.
- Replacing the entire Process Trace with DeepSeek Harness/Cordis.
- Generic render-intent unions for every File/MCP/Search Tool in this slice.
- Database migration or rewriting historical Agent events.
- Full repository test suite unless focused verification exposes a wider issue.

## Research References

- [`research/deepseek-harness-terminal-presentation.md`](research/deepseek-harness-terminal-presentation.md)
  — DeepSeek Harness event/presenter/card architecture and the bounded neo-chat mapping.

## Technical Notes

- Backend: `internal/chat/provider_tool_round.go`,
  `local_skill_tool_loop.go`, `process_trace.go`, `process_trace_runtime.go`,
  `chat_agent_events.go`.
- Frontend: `lib/chat/types.ts`, `lib/chat/processTrace.ts`,
  `components/content/ProcessTracePanel.tsx`.
- Existing Tool scheduling, allowlist, approvals, execution and model-facing
  results remain authoritative and unchanged.
