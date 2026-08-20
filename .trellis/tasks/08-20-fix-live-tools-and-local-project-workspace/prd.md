# 修复实时工具事件并接入本机项目工作区

## Goal

修复 Agent 在网页实时生成时把合法 `tool.call.updated` 判为非法并中断显示的问题，
同时把用户明确授权的 `/home/mumu/projects/Oncall_Agent` 作为本机 Agent 工作区，
使 Linux 路径与常见 WSL UNC 路径都能安全映射到同一个 workspace-relative Tool 路径。

## Runtime Evidence

- 失败页面显示 `Server returned an invalid Tool call update.`，但对应 Backend Turn 已实际
  执行多次 `terminal`；因此不是 Bash/File Tools 缺失，而是 Live SSE DTO 校验失败。
- 捕获的 Backend `local_direct` Tool event 有稳定 `executionId`，出于持久化最小披露不含
  `callId`，并对 `file_write/file_edit/publish_file` 合法发送 `classification=write`。
- Frontend `normalizeMcpToolCallUpdate` 当前强制 `callId` 存在，并拒绝
  `local_direct + write`，与 Backend 的已部署合同冲突。
- 当前 Compose 已支持单一 `AGENT_LOCAL_WORKSPACE_SOURCE -> /workspace` bind，但 live env
  使用默认 `./data/agent-workspace`；Backend 看不到用户请求的 `Oncall_Agent`。
- 用户输入的是
  `\\wsl.localhost\Ubuntu\home\mumu\projects\Oncall_Agent\README.md`；现有 File Tools
  只接受 workspace-relative 路径，Terminal 也只允许 workspace 内 working directory。
- `Oncall_Agent` 由运行用户 `mumu:mumu` 持有。Neo Chat 的 live env、Secrets 与 Backup
  位于另一个未授权目录，不应因本次接入而暴露。

## Requirements

- Frontend 对 `local_direct` live Tool update 接受 `read|write|execute`，但继续拒绝未知
  classification、未知 mode/status、畸形字段与 MCP 的 `execute`。
- 当 Server 省略非公开的 `callId` 时，Frontend 使用必需且有界的 `executionId` 作为稳定
  UI call identity；显式合法 `callId` 仍优先使用。
- 补齐 Live SSE regression，覆盖无 `callId` 的 Terminal read/execute、File write，以及
  仍然 fail-closed 的畸形事件。
- 新增可选 `AGENT_LOCAL_WORKSPACE_HOST_ROOT`。它只描述 `/workspace` 在宿主机对应的
  clean absolute non-root path，不增加第二个访问根，也不绕过现有 `os.Root`/symlink
  边界。
- File read/write/edit/search/publish 与 Terminal `workingDir` 接受三种等价输入：
  workspace-relative、位于授权 Host root 下的 Linux absolute path、以及
  `\\wsl.localhost\<distro>\...` / `\\wsl$\<distro>\...` UNC path。
- 别名输入必须先映射为 workspace-relative path，再进入既有 `cleanWorkspacePath`、
  `os.Root`、CAS、symlink 与 bounded I/O 校验。Host root 外、其他 WSL 路径、Windows
  drive、traversal、NUL 与不规范路径继续拒绝。
- System instruction 明确当前授权 Host root，并要求 Terminal 命令使用 `$PWD` 或
  workspace-relative path；不得重写任意 shell command 文本。
- Example env、Compose、Backend config、local Runtime docs/spec 与验证脚本同步。
- 本机 live env 仅将 workspace source/host root 指向
  `/home/mumu/projects/Oncall_Agent`，不挂载 `/home/mumu/projects`、Neo Chat、Secrets、
  Backup、用户 Home 或容器 Socket。
- 部署前保留当前 Backend/Frontend image 与 mode-0600 live env；只重建 Backend/
  Frontend，不迁移数据库，不删除旧 `data/agent-workspace`。

## Acceptance Criteria

- [x] Frontend 单测证明捕获的无 `callId`、`local_direct + write` 事件可实时解析并分派，
      畸形/越权组合仍失败。
- [x] Backend 单测证明 Linux absolute 与两种 WSL UNC alias 可读 `README.md`，越界、
      traversal、其他 root、Windows drive 与 symlink 逃逸失败。
- [x] File write/edit/search/publish 与 Terminal workingDir 复用同一 alias 规范化合同。
- [x] Backend tests/vet、Frontend format/lint/typecheck/tests/build、Agent local Runtime gate、
      Compose example/live render 与 standalone full gate 通过。
- [ ] Live Backend/Frontend 健康，数据库与非目标容器不变，旧 workspace 数据保留。
- [ ] 网页等价真实 Agent 请求读取授权 WSL UNC `README.md`，Live Tool events 不再产生
      `invalid Tool call update`，assistant 以 `toolMode=agent` 完成并给出有内容依据的总结。

## Definition of Done

- 实现、测试、English docs/spec、回滚材料与真实验收均提交，不 push。
- Git 工作区 clean；live env、runtime workspace、Secrets 与 rollout evidence 不入 Git。

## Technical Approach

1. Frontend normalizer 将缺失 `callId` 收敛为 `executionId`，并对 `local_direct` 接受
   `read|write|execute`；新增 server stream regression。
2. Backend Config/Executor 增加可选 Host root，集中实现一个 alias-to-relative resolver，
   所有 Workspace File/Artifact/Search API 与 Terminal workingDir 在既有边界前调用它。
3. Runtime prompt 只披露当前授权 Host root，指导模型把用户绝对/UNC 路径映射到相对路径；
   Shell command 仍不做字符串替换。
4. 更新 Compose/env/preflight/docs，完成候选镜像、备份、定向 recreate 与真实 Agent smoke。

## Decision (ADR-lite)

**Context**：直接挂载整个 `/home/mumu/projects` 会把 Neo Chat live env、Secrets、Backup
和无关项目一并暴露；仅要求用户改说相对路径则不符合自然本机 Agent 体验。

**Decision**：继续使用单一 `/workspace` authority，只将用户明确点名的
`Oncall_Agent` bind 为该根；以显式 Host root 提供 Linux/WSL 路径别名映射。Frontend
对 Server 的最小披露事件做稳定兼容，不扩大持久化 Tool payload。

**Consequences**：本机一次只授权一个 Compose workspace；切换项目需要改 live env 并
定向重建 Backend。未来可以在不改变 Executor 边界的前提下增加管理员 workspace picker，
但本次不做动态任意目录挂载。

## Out of Scope

- 不挂载整个 Home、整个 projects 目录、Windows 磁盘、Docker/Podman socket 或 Secrets。
- 不增加 sudo、Sandbox、Runner、Subagent 或递归派生。
- 不增加动态网页目录选择器、多 workspace 并存或每 Conversation 独立 mount。
- 不重写任意 Terminal command 内的路径字符串。
- 不修改数据库 schema、聊天历史或旧 workspace 内容。

## Technical Notes

- Frontend: `frontend/src/lib/mcp/types.ts`, server Chat client and MCP timeline tests.
- Backend: `internal/localskills`, `internal/config`, `cmd/api`, local Chat prompt/tests.
- Operations: `compose.single-server.yml`, `.env.single-server.example`, local Runtime verifier,
  deployment and Agent Runtime contracts.
- Research: [`research/live-tool-event-and-workspace-root.md`](research/live-tool-event-and-workspace-root.md).
