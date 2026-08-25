# Conversational Skill/MCP acquisition and Pi-style commands

## Goal

让用户既能在聊天框中用自然语言安装 Skill/MCP，也能用可发现的 slash
commands 确定性地搜索、安装、启用、停用和调用资源；当 Agent 因能力缺失而
无法继续时，它可以自动搜索合适资源、提出安装请求，并在完成安全审批或配置后
继续原任务。

本任务借鉴 Pi 的 ResourceLoader、PackageManager、动态 slash command 与
`/skill:<name>` 交互，但不照搬 Pi Extension 的任意代码执行权限。Neo Chat
现有 Skill supply chain、MCP Marketplace、credential、selection、Workspace
trust 与 approval 继续作为唯一写权限来源。

## User outcome

- 用户说“帮我安装能处理 Excel 的 Skill”，Agent 能搜索、说明为何匹配并发起安装。
- 用户说“装一下 DeepWiki MCP”，Agent 能定位商店条目；若需要 Secret/OAuth，
  显示安全配置卡，而不是向模型索要密钥。
- Agent 执行任务遇到真实能力缺口时，先搜索资源再决定能否继续，不直接宣告失败。
- 输入 `/` 可发现命令；已安装 Skill 可用 `/skill:<name> [args]` 明确调用。
- 安装成功后原任务可以在刷新后的 Runtime Resource Snapshot 上继续，用户不必重述。

## Research baseline

### Pi

- Pi TUI 内建命令包括 `/settings`、`/model`、`/scoped-models`、`/export`、
  `/import`、`/share`、`/copy`、`/name`、`/session`、`/changelog`、`/hotkeys`、
  `/fork`、`/clone`、`/tree`、`/trust`、`/login`、`/logout`、`/new`、
  `/compact`、`/resume`、`/reload`、`/quit`。
- Pi Web 只把 `/compact`、`/reload`、`/name`、`/session`、`/copy` 作为 Web
  内建命令，并动态加入 Extension commands、Prompt commands 和
  `/skill:<name>`。
- Pi 的 Skill/Package 搜索和安装是管理 API/UI 或 CLI 行为，不是 Agent 在
  对话中静默执行的内建能力。
- `/skill:<name>` 会在 prompt 前把该 Skill 的 `SKILL.md` 展开；`/reload`
  重新加载 Skills、Extensions、Prompts、Themes 与 context files。

### Neo Chat today

- Skill Store 只能由普通用户安装已 admitted 的 immutable package；Candidate
  ingest/review 属于管理员 supply-chain 权限。
- MCP Marketplace 已支持 search/detail/install、online validation、Secret/OAuth、
  conversation/workspace selection；安装目前需要管理员权限。
- Agent Run 已有冻结的 Runtime Resource Snapshot，但没有用户可调用的
  discovery/install Tools，也没有资源安装后的 refresh/continuation 协议。
- Skill 确定性调用只识别 legacy `/skill-name`，聊天框没有 slash autocomplete。
- Skill 与 MCP 分属不同领域服务，不能新建一个绕过它们校验的通用安装器。

## Product decisions already fixed

1. **One command plane, two entry paths**
   - slash command：确定性解析，不经过 LLM 猜测。
   - 自然语言/Agent 自救：调用同一组 server-side resource orchestration API/Tools。
2. **One mutation authority**
   - Skill 写操作委托现有 `skillsupply.Service`。
   - MCP 写操作委托现有 `mcpclient.Service`。
   - 禁止模型通过 Bash、npm、git、Docker 或任意 URL 直接安装。
3. **Search is not install**
   - Agent 可以自动搜索 bounded results。
   - 用户明确要求安装时，已 admitted 且 credential-free 的资源可直接安装，不重复确认。
   - Agent 在其他任务中自行发现能力缺口时可以自动搜索，但安装前必须显示一次确认卡。
   - Secret/OAuth、管理员权限、未 admitted 资源和其他高风险作用域变更始终进入配置或审批。
4. **Frozen run resources remain frozen**
   - 当前 step 不热插入新 Tool definition。
   - 安装完成后在明确 refresh boundary 创建新 Runtime Resource Snapshot，再续跑
     原任务；trace 记录前后 revision 与因果关系。
5. **No raw secrets in model context**
   - Secret/OAuth 只走 UI handoff 与 Backend vault；Agent 只看
     `credential_required/configured/ready` 等脱敏状态。

## Command surface

### Phase 1: read-only discovery and invocation

- `/skill`：列出已安装 Skill，并提供管理入口。
- `/skill search <query>`：搜索可安装 Skill。
- `/skill info <name-or-id>`：查看来源、版本、权限请求和 admission 状态。
- `/skill:<name> [args]`：确定性调用已安装 Skill；兼容 legacy `/skill-name` 回放。
- `/mcp`：列出当前会话已启用及可用 MCP Server。
- `/mcp search <query>`：搜索 MCP Marketplace。
- `/mcp info <identifier>`：查看部署、Tool、认证与兼容状态。
- `/mcp status`：展示当前会话 selection 与健康状态。
- `/resources`：展示本 Run 的脱敏 Resource Snapshot 与 revision。

### Phase 2: explicit mutations

- `/skill install <candidate-id>`、`/skill remove <installation-id>`。
- `/mcp install <identifier>`、`/mcp enable <server>`、
  `/mcp disable <server>`、`/mcp remove <server>`。
- `/reload`：在无运行中任务时刷新资源；运行中由 continuation protocol 使用新的
  snapshot，不允许破坏当前 step 的一致性。

### Later Pi-parity commands

- 候选：`/new`、`/model`、`/compact`、`/name`、`/session`、`/copy`、
  `/resume`、`/fork`。
- 不照搬：`/quit`、`/login`、`/logout`、`/share`、`/export`、`/import`、
  `/trust`，除非 Neo 对应的 Web 产品流程已定义且不与现有设置/权限冲突。

## Conversational acquisition flow

```text
User intent or capability-missing signal
  -> resource_search(kind, capability, query)
  -> ranked bounded candidates with provenance/auth/permission summary
  -> resource_request_install(exact candidate + exact revision)
  -> approval/configuration card
  -> existing Skill or MCP authority performs mutation
  -> install/validate/enable result is persisted and audited
  -> resource snapshot refresh boundary
  -> chained continuation resumes the original task
```

### Capability-gap trigger

- Agent 只有在以下情况才能发起自动 discovery：
  - 用户明确要求安装/寻找能力；
  - 当前已授权 Tools/Skills 无法满足任务；
  - Tool 返回结构化 `capability_missing`；
  - 已安装资源不可用，且存在功能等价替代项。
- 普通 Tool error、数据源超时、参数错误不得伪装成能力缺口。
- 每个用户任务最多 2 次 discovery round、每次最多 5 个候选、最多 1 个待安装
  proposal；禁止递归安装与“为了可能有用”而安装。

### Skill routes

- 已 admitted：可直接进入安装 proposal。
- 已发现但未 admitted：只创建 quarantine Candidate/管理员审核请求，Agent 不得
  自行 admission。
- 任意 Git/ZIP/URL：必须固定 immutable revision、完成 validation/SBOM/review，
  不能直接成为当前 Run 资源。

### MCP routes

- credential-free + validated deployment：可进入安装并为当前 conversation 启用。
- Secret required：显示字段化配置卡，Secret 直达 Backend vault。
- OAuth required：显示授权跳转，callback 后继续 validation。
- 不安全 endpoint、不匹配 deployment hash、在线 validation 失败：fail closed，
  保留草稿供“工具”页诊断，不向 Agent 暴露敏感错误细节。

## Runtime and persistence model

- `ResourceDiscoveryResult`：短生命周期、带 provider、candidate ID、exact revision、
  trust/admission/auth/permission 摘要和 expiry。
- `ResourceInstallProposal`：绑定 user、conversation/workspace、origin run、candidate
  revision、requested scope、approval state；支持 CAS 防止候选漂移。
- `ResourceMutationAudit`：记录谁、何时、通过何种入口、对哪个 immutable resource
  做了何种动作；不记录 Secret。
- `RunContinuation`：把原任务、origin run、mutation result、新 snapshot revision
  关联起来。UI 仍显示一个连续任务，但 trace 能区分 Run segment。
- 安装状态属于用户/Workspace/Conversation 的既有 authority；绝不只存在浏览器
  localStorage。

## UX requirements

- 输入单个 `/` 打开按 Built-in、Skill、MCP、Session 分组的 autocomplete；支持
  键盘选择、argument hint、空结果和 loading。
- slash 命令回显为 compact command block，不把展开后的 `SKILL.md` 当用户正文展示。
- discovery 结果显示“为什么匹配”、来源、版本、权限/认证需求、作用域和状态。
- mutation 采用 inline approval/config card；用户可拒绝、改作用域或进入完整管理页。
- 资源补装期间，原任务显示“等待安装/认证”；成功后显示“已补充能力，继续执行”，
  而不是要求用户重新输入。
- 失败必须区分：没找到、未 admission、权限不足、需配置、validation 失败、版本漂移、
  continuation 失败。

## Requirements

- [x] 建立 server-side Resource Orchestrator，只编排现有 Skill/MCP 服务，不复制写逻辑。
- [x] 提供统一的 read-only search/info/status contract 与 bounded provider adapters。
- [ ] 提供 proposal/approval/configuration/mutation/continuation 状态机。
- [x] 为 Agent 注册 `resource_search` 与 `resource_request_install`；模型不能直接调用
      mutation API。
- [x] 为 Chat composer 增加 slash autocomplete、argument hints 和 deterministic parser。
- [x] 支持 `/skill:<name>`，兼容历史 `/skill-name`，并处理 command name 冲突。
- [x] 安装后产生新的 Runtime Resource Snapshot revision 并在续跑前校验资源 ready。
- [ ] Skill/MCP 搜索、安装、启停、卸载均有 owner/admin/scope/CAS/audit 约束。
- [x] Secret、OAuth token、host/cache path、raw package content 不进入 prompt、trace、日志。
- [x] capability-gap discovery 有预算、去重和循环熔断。

## Acceptance criteria

- [x] 输入 `/` 能看到 `/skill`、`/mcp`、`/resources`、`/reload` 与所有已安装的
      `/skill:<name>`，命令分组、筛选和键盘操作正常。
- [x] `/skill search excel` 返回 bounded admitted candidates；安装必须绑定 exact
      fingerprint/revision，候选变化时拒绝旧 proposal。
- [x] “帮我安装 Excel Skill”与对应 slash command 走同一 Backend contract，结果一致。
- [ ] “安装 DeepWiki MCP”能完成搜索；需要 credential 时显示配置卡且模型上下文和
      日志中没有 Secret。
- [ ] Agent 执行 XLSX 任务发现缺少可靠生成/校验能力时，能搜索并提出合适 Skill，
      经 policy gate 安装后从新 snapshot 续跑，不要求用户重述。
- [x] 安装被拒绝、未 admission、没有匹配、认证失败、validation 失败时任务明确暂停
      或降级，绝不声称已安装/已完成。
- [x] 同一资源不会被并发重复安装；stale proposal、跨用户/跨会话 ID 均被拒绝。
- [x] 当前 Run 的旧 snapshot 不被中途修改；续跑 trace 能看到 old/new revision 与
      mutation audit ID。
- [x] `/skill:<name>` 确定性加载目标 Skill，legacy `/skill-name` 历史消息仍可回放。

## Delivery phases

1. **Command/discovery foundation**
   - command schema、Backend read APIs、slash palette、`/skill:<name>`、`/resources`。
2. **Confirmed mutation flow**
   - proposal state machine、inline approval/config、Skill/MCP existing-authority adapters、
     audit 与 CAS。
3. **Agent capability recovery**
   - `resource_search`/`resource_request_install`、structured gap taxonomy、bounded policy、
     snapshot refresh 与 chained continuation。
4. **Hardening and rollout**
   - concurrency/idempotency/security tests、metrics、feature flag、rollback、live smoke。

每阶段都可单独发布；Phase 3 不得早于 Phase 1/2 的确定性控制面与审批链。

## Out of scope

- 直接嵌入 Pi SDK 或替换现有 Go Agent loop。
- 支持任意 Pi Extension/npm package 在 Backend 进程内执行。
- Agent 自动 admission 未审核 Skill、直接运行商店提供的 Shell/Docker/npm 命令。
- 在首版复刻 Pi Themes、Prompt packages、TUI widgets。
- 把 Office/XLSX 生成器本身塞进本任务；它是本能力安装链的首个验收资源，可作为
  独立 first-party Skill 任务交付。

## Confirmed authorization policy

- **Explicit user install intent**：用户明确说“安装 X”或执行 install slash command
  时，已 admitted、credential-free 且不扩大高风险作用域的资源直接安装，不弹出
  冗余确认。
- **Agent-initiated capability recovery**：Agent 在执行其他任务时自行发现能力缺口，
  可以自动搜索和排序候选，但实际安装前必须显示一次确认卡。
- **Always gated**：需要 Secret、OAuth、管理员权限、未 admitted Candidate、来源或
  revision 发生变化，以及其他高风险作用域变更时，始终进入配置或人工审批。
- 所有入口仍受 owner、scope、CAS、audit、Workspace trust、Skill admission 与 MCP
  validation 约束；“用户明确要求”不绕过任何 Backend 安全校验。

## Definition of done

- [x] Backend/Frontend focused tests、跨层 contract、权限/CAS/Secret 泄漏与 continuation
  failure-path tests 全部通过。
- [x] 共享逻辑、持久化、安全和 Run lifecycle 变更执行完整 Go、Frontend 与 standalone
  quality gate。
- [x] `mm-chat/docs/` 同步 command contract、resource state machine、security model、
  rollout/rollback 与 operator diagnostics。
- [ ] 在 live workspace 完成两条 smoke：显式安装并调用 Skill；能力缺失后安装/配置 MCP
  并自动续跑原任务。

## Implementation status (2026-08-26)

Phase 1、显式安装链和 Agent capability-recovery 主链已经实现并通过 full standalone
gate。Skill/MCP 安装复用既有 authority，安装审计、durable Agent approval、bounded
discovery、fresh Runtime Resource Snapshot 与同一 Provider loop 的 continuation 已落地。

剩余两条闭环保持未勾选：

- Secret/OAuth MCP 当前会安全跳转既有 Marketplace 配置面，但“配置完成后自动恢复原
  Run”的持久化 configuration continuation 尚未实现。
- enable/disable/uninstall 仍复用既有 owner/CAS 管理 API；统一的
  `ResourceMutationAudit` 目前只覆盖 install，生命周期 mutation audit 尚待收口。

完成上述代码后，还必须在真实账号、真实 admitted Skill 与真实 MCP credential 环境中
执行两条 live smoke，才允许归档本任务。

## Research references

- [`research/pi-slash-and-conversational-install.md`](research/pi-slash-and-conversational-install.md)
  — Pi slash/package 行为、Neo 现状差距与推荐控制面。
- [`research/pi-resources-and-xlsx-forensics.md`](research/pi-resources-and-xlsx-forensics.md)
  — 早期 XLSX 根因与 Pi Resource/Skill/Extension/Package 机制。
