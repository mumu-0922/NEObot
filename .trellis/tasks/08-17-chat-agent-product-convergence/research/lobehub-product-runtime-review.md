# LobeHub Product/Runtime Source Review

## Reviewed source

- Clone: `/tmp/lobehub-src.2AjECM`
- Commit: `a625e350f65c57e589654d34ccc999676ffbbe86`
- Commit date: `2026-08-13T14:48:40+08:00`

## Confirmed LobeHub behavior

### Chat/Agent is a persisted runtime policy

- `src/features/ChatInput/ActionBar/AgentMode/index.tsx` exposes one input-level
  selector with `chat | agent` and describes Agent as tools, web, files and env.
- `src/features/ChatInput/hooks/useToggleAgentMode.ts` stores the choice on
  `chatConfig.enableAgentMode`; it deliberately leaves plugins untouched because
  Chat mode is enforced in the Runtime tools engine.
- `src/features/ChatInput/hooks/useEffectiveAgentMode.ts` resolves stored Agent plus
  model Tool capability to the effective mode and avoids transient UI downgrade while
  model abilities are loading.

### Chat mode uses a strict allowlist

- `packages/builtin-tools/src/index.ts` defines `chatModeAllowedToolIds` as Knowledge,
  Memory, Web Browsing and opt-in Image Generation.
- `src/helpers/toolEngineering/index.ts` and
  `apps/server/src/modules/Mecha/AgentToolsEngine/index.ts` omit user plugins and
  always-on Agent tools in Chat mode and disable explicit activation.
- Agent mode adds always-on Agent/Skill activation plus runtime-managed Browser,
  Local System, Cloud Sandbox and user connectors.

### Built-in Tool and MCP are separate layers

- Native packages include Browser, Local System, Knowledge Base, Memory, Task,
  Skills, Web Browsing and Cloud Sandbox under `packages/builtin-tool-*`.
- MCP execution is handled separately by `apps/server/src/services/toolExecution/`
  and `apps/server/src/services/mcp/`; MCP is an extension/connector path, not the
  definition of Agent itself.

### Agent Runtime is a real loop with persistence and approval

- `packages/agent-runtime/src/agents/GeneralChatAgent.ts` implements
  user input -> LLM -> Tool/approval -> Tool results -> LLM -> finish.
- `packages/agent-runtime/src/core/runtime.ts` owns step limits, cancellation,
  instruction execution and continuation.
- `apps/server/src/modules/AgentRuntime/` supplies message, operation, stream, Tool,
  context, blob and lifecycle adapters.

### Knowledge and Memory share the runtime context

- `apps/server/src/modules/AgentRuntime/adapters/serverCallLlmContextBuilder.ts`
  constructs one context containing Knowledge, userMemory, toolsConfig, skillsConfig
  and enableAgentMode.

### Deliverables are exported into chat

- `packages/builtin-tool-cloud-sandbox` defines `exportFile`, stores a download URL
  in Tool state and renders a download control in the Tool result card.
- `apps/server/src/modules/AgentRuntime/adapters/ServerBlobStore.ts` persists model
  blobs through the shared FileService rather than exposing arbitrary runtime paths.

### External messaging reuses the same Runtime

- `apps/server/src/services/bot/AgentBridgeService.ts` maps platform threads to topic
  and operation IDs, invokes `AiAgentService`, supports `/mode` and `/stop`, and maps
  progress/final attachments back to the platform.
- WeChat uses iLink Bot API with QR login and per-conversation `context_token`; the
  token must be returned on replies.

## Neo Chat mapping

### Keep and extend

- `backend/internal/chat/`: current ordinary Chat Agent loop and durable events.
- `backend/internal/localskills/` and `backend/internal/skillsupply/`: Skill catalog,
  local_direct and Store.
- `backend/internal/mcpclient/` plus `cmd/mcp-runner/`: connector execution.
- Knowledge/RAG, Memory Worker, PostgreSQL, Redis, MinIO and Assistant Store.

### Product gaps

- No explicit persisted Chat/Agent policy in ordinary stream requests.
- MCP is exposed in the composer and preflighted before every enabled Server send.
- Browser MCP currently fails Chromium startup because the process singleton Socket
  path exceeds the platform limit.
- Workspace File results do not yet become protected downloadable message artifacts.

### Remove after replacement

- The G20/G21 control stack is disconnected from ordinary Chat, default-off and has
  empty `agent_*` tables in the current runtime database.
- `chat_agent_*` is the active ordinary Chat truth and must remain.

## What not to copy from LobeHub now

- Cloud Sandbox, device gateway, remote devices, Group Agent, Subagent and
  heterogeneous external agent providers.
- Enterprise Workspace/member override complexity.
- Full Task/Schedule system before Chat Agent output and mode behavior are stable.

