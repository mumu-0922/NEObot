# Fix resource context stream failure and orphan spinner

## Goal

修复上线后 Agent 请求在写入 Resource orchestration context event 时立即返回 500、
Assistant Message 永久停留在 `streaming` 的回归；同时让用户给出的可信 AIHero Skill
页面链接进入现有 Skill supply-chain 搜索/安装流程，而不是被当成任意 URL 执行。

## What I already know

- 2026-08-26 12:37:11 live `/stream` 在 52ms 内返回 500，模型与安装流程尚未启动。
- 对应 `chat_agent_turns.status=failed`，但 `messages.status=streaming`，前端因此持续轮询并显示转圈。
- 前两条 `context.injected` 成功；第三条使用 source `resource-orchestrator`，而事件规范只允许
  `system-prompt`、`skill-catalog`、`skill-instruction`、`runtime-context`。
- 同一错误已产生多个 failed Turn / streaming Message 孤儿记录。
- 当前链接解析只接受既有 allowlist；`https://www.aihero.dev/skills-grill-me` 尚未支持。

## Requirements

- Resource orchestration prompt 使用既有合法 source `runtime-context`，不扩张事件枚举。
- 所有 Assistant Message 创建后的 pre-stream context persistence 失败必须原子式收口为
  `failed`，写入稳定 error code，并让 Turn/Message 终态一致。
- 失败路径不能继续留 active Run，也不能触发浏览器无限轮询或自动制造重复 orphan Message。
- 增加覆盖真实 Handler 路径的回归测试，证明三个 context injection 均可记录。
- 增加失败注入测试，证明 event persistence 失败时 Message 为 `failed`、Turn 为 `failed`。
- AIHero Skill 链接仅接受 HTTPS、固定 host/端口和明确 `/skills-<slug>` 路径；拒绝 userinfo、
  query、fragment、非默认端口、路径穿越与未知页面。
- AIHero 页面只解析成 sanitized Skill identifier，再委托现有 admitted Skill search/install authority；
  禁止下载或执行页面脚本、Git/npm/Shell 指令。
- 修复 live 数据中由本缺陷产生的 failed Turn / streaming Message，并保留用户消息与审计记录。

## Acceptance Criteria

- [x] 普通 Agent 请求不再因 `resource-orchestrator` source 返回 500。
- [x] 三类 context event 均按规范持久化，Resource orchestration 显示为 `runtime-context`。
- [x] 任一 pre-stream event persistence failure 后，Assistant Message 与 Agent Turn 都为 `failed`。
- [x] 刷新页面后不再显示本次孤儿回复持续生成。
- [x] AIHero `skills-grill-me` 链接得到安全候选/安装流程；恶意或非 Skill AIHero URL 被拒绝。
- [x] Backend focused tests、Go full gate 和相关 live smoke 通过。

## Definition of Done

- 回归测试先覆盖根因与终态不变量。
- Backend format/vet/test 通过；涉及跨层时运行比例化 Frontend 验证。
- 更新相关 contract/spec，记录该类错误的防复发约束。
- 构建并部署新 Backend 镜像；迁移/数据修复前创建可校验备份。
- Git 提交、任务归档与 session journal 完成；不 push。

## Technical Approach

复用既有 `runtime-context` event source，避免扩大持久化协议。将三个 context injection
共用一个“记录失败即 finalize Assistant + finish Turn + 返回稳定错误”的 helper，保证任何
新增 pre-stream context 都继承相同终态语义。AIHero URL 只做严格语法解析和 identifier
映射，不新增通用网络安装器。

## Decision (ADR-lite)

**Context**：Handler 写入了 event schema 未声明的新 source，并且失败分支绕过 Message finalize。

**Decision**：使用现有 `runtime-context` source；集中 pre-stream failure finalization；站外链接
仅允许显式 adapter 映射到既有 supply-chain authority。

**Consequences**：无需数据库 schema migration；trace source 保持向后兼容；以后增加 context
source 或站点必须同步 contract 与真实 Handler/DB 测试。

## Out of Scope

- 不开放任意 Skill 网页、Git 仓库、ZIP、npm package 的直接安装或执行。
- 不重构整个 SSE/Agent loop。
- 不删除用户消息、Agent Turn 或审计事件。

## Technical Notes

- `mm-chat/backend/internal/chat/handler.go`
- `mm-chat/backend/internal/chat/chat_agent_events.go`
- `mm-chat/backend/internal/resourceorchestrator/resource_link.go`
- `.trellis/spec/backend/chat-tool-loop.md`
- `.trellis/spec/backend/resource-orchestration.md`

## Research References

- [`research/aihero-skill-link.md`](research/aihero-skill-link.md) — AIHero is a discovery alias;
  its page provides no immutable revision and therefore cannot be installation authority.
