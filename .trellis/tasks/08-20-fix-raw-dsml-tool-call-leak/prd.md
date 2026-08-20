# 修复原始 DSML 工具调用泄漏

## Goal

阻止 OpenAI-compatible Provider 把模型生成的原始 DSML 工具协议作为普通回答流式输出、持久化并在前端展示，同时保持现有原生 `tool_calls`、Chat/Agent Tool 权限和执行链不变。

## What I already know

- Live 消息 `8de257bc-02c9-4974-9581-230dc27b4d8d` 使用 `FOHWSU/deepseek-v4-flash`，持久化模式为 `requestedToolMode=chat`、`toolMode=chat`。
- 该消息先原生调用允许的 `search_memory`，调用因 `memory_service_unavailable` 失败；随后 Provider 的 `content` delta 返回 `<｜｜DSML｜｜tool_calls>`、`exec_command` 及命令参数。
- `output_blocks=[]`，durable `assistant.message` 事件与 `messages.content` 均含同一 DSML；因此污染发生在 Backend Provider content boundary，不是 Frontend Markdown 独有问题。
- Chat 模式按现有契约物理省略 Terminal/File/Skill/MCP/Goal Tool；原始 `exec_command` 没有执行，也不能因本修复获得执行能力。
- OpenAI-compatible adapter 已支持原生结构化 `tool_calls`，本任务不能用正则把 DSML 转成可执行调用。

## Assumptions

- DSML envelope 是 Provider 未结构化的协议残留，属于无效且不可信的回答内容。
- 一旦流中确认出现 DSML Tool 协议，应停止该 Provider stream 并返回稳定的 Provider failure；已安全发出的普通前缀按既有 partial-content failure 契约保留为 failed，协议正文不得跨 SSE 或 persistence boundary。
- 检测必须跨 delta 保留有界后缀，覆盖 ASCII/fullwidth vertical-bar 变体和分片 marker。

## Requirements

- 在 OpenAI-compatible Provider stream boundary 检测 DSML Tool protocol marker。
- marker 及其后内容不得生成 `ProviderEventDelta`，不得进入最终消息或 durable agent event。
- 检测后发出经过归类且不含 Provider raw body/command/参数的错误。
- 不改变原生 OpenAI `tool_calls` 的 accumulator、continuation 或现有 allowlist/argument validation。
- 不解析或执行 DSML 中的 Tool name/arguments；Chat 模式权限保持不变。
- 普通内容与跨 chunk UTF-8 输出不得损坏。

## Acceptance Criteria

- [ ] 分片的 `<｜｜DSML｜｜tool_calls>` 不会作为 content delta 输出，并产生 typed Provider failure。
- [ ] ASCII bar 变体同样被阻断。
- [ ] marker 前的普通文本可以安全输出，marker、Tool 名和参数均不输出。
- [ ] 普通流式回答和原生结构化 Tool Calls 的 focused tests 继续通过。
- [ ] Backend contract 记录 raw Provider Tool protocol fail-closed 边界。
- [ ] 仅执行 owning package 的 focused Go tests；非必要不运行全量 gate。

## Definition of Done

- 局部单元测试通过。
- Backend live image 可回滚，若部署则只 recreate Backend 并验证 health/image/无关容器不变。
- Trellis task 归档、journal 更新并自动 commit；不自动 push。

## Out of Scope

- 不新增 DSML parser 或 DSML Tool execution compatibility。
- 不修改 Frontend 以掩盖已污染的历史消息。
- 不修复独立的 Memory service unavailable 根因。
- 不清洗或重写历史用户消息。
- 不运行无关全量测试。

## Technical Notes

- Provider adapter：`mm-chat/backend/internal/chat/provider_openai_compatible.go`
- Focused tests：`mm-chat/backend/internal/chat/provider_openai_compatible_test.go`
- Runtime policy：`.trellis/spec/backend/chat-agent-mode.md`
- Tool loop contract：`.trellis/spec/backend/chat-tool-loop.md`
- Live flow：Provider content delta → `ProviderEventDelta` → Tool loop → SSE/persistence → durable `assistant.message` → Frontend Markdown。
- 复用 `processReasoningStream` 的 bounded suffix / UTF-8-safe holdback 思路，避免逐 chunk 独立匹配造成 marker 分片绕过。
