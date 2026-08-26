# Conversation-scoped Skill/MCP selection and secure acquisition

> **Requirement correction — 2026-08-26:** The Slash-heavy Resource UX shipped
> for the first implementation pass is superseded. Lifecycle commands are not
> the primary UI. This task is reopened for a Pi-informed three-layer model:
> management pages own installed inventory, composer buttons own durable
> per-conversation selection, and the Agent owns bounded runtime auto-selection.

## Goal

让用户在 Skill Store 与 Tools 页面管理已安装资源，在聊天输入框通过 Skill/MCP
两个紧凑入口为**当前对话**独立选择可用资源；当 Agent 执行任务时，它可以在既有
权限和冻结快照边界内自动选择所需的已安装资源，遇到真实能力缺失时仍可安全搜索、
提出安装请求，并在完成审批或配置后继续原任务。

本任务借鉴 Pi 的 ResourceLoader、PackageManager、独立 Skills/Packages 管理面板与
session Tool policy，但不照搬 Pi Extension 的任意代码执行权限，也不照搬会淹没输入
体验的动态 Slash command 墙。Neo Chat
现有 Skill supply chain、MCP Marketplace、credential、selection、Workspace
trust 与 approval 继续作为唯一写权限来源。

## User outcome

- 用户在对话 A 选择的 Skill/MCP 不影响对话 B；刷新或切换后各自选择仍保持。
- 聊天框底部有独立 Skill 与 MCP 入口，只选择已安装资源，不承担卸载和复杂配置。
- Skill Store 与 Tools 页面分别查看、安装、配置和卸载 Skill/MCP。
- 用户粘贴受支持的 Skill/MCP 链接或说“帮我安装能处理 Excel 的 Skill”，Agent 能解析
  或搜索、说明为何匹配并发起同一条安全安装流程。
- 用户说“装一下 DeepWiki MCP”，Agent 能定位商店条目；若需要 Secret/OAuth，
  显示安全配置卡，而不是向模型索要密钥。
- Agent 执行任务遇到真实能力缺口时，先搜索资源再决定能否继续，不直接宣告失败。
- 安装成功后资源进入用户 inventory；是否长期加入某个对话由该对话的 composer
  selection 决定，原任务可在刷新后的 Runtime Resource Snapshot 上继续。

## Product model (corrected)

```text
Skill Store / Tools
  install · configure · update · uninstall
                |
                v
installed inventory (user/workspace authority)
                |
                v
composer Skill picker + MCP picker
  durable selection scoped to one conversation
                |
                v
frozen Run Resource Snapshot
  selected resources + policy-bounded Agent auto-selection
```

安装、会话选择、运行时调用是三个不同状态；任一层不得通过浏览器 localStorage 或
隐式全局开关篡改另一层。

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
- 旧实现已加入 `resource_search` / `resource_request_install`、approval/configuration、
  refresh/continuation 与冻结 Runtime Resource Snapshot；这些安全链可以复用。
- 旧实现也加入了 Resource lifecycle Slash autocomplete，但 live UX 证明它不应继续作为
  primary surface。
- MCP 已有 per-conversation selection；Skill 仍把用户全部 installed inventory 投影到
  Runtime，没有同构的 conversation selection authority。
- Skill 与 MCP 分属不同领域服务，不能新建一个绕过它们校验的通用安装器。

## Confirmed runtime preference

- Composer 中未手动选择、但已安装且授权的 Skill/MCP，Agent 可以为当前 Run 临时
  自动激活。该 **run-only activation** 进入冻结 Runtime Resource Snapshot 与 trace，
  但绝不静默写回当前对话；持久选择只由用户操作 composer picker 改变。

## Product decisions already fixed

1. **Three state layers**
   - Inventory：Skill Store / Tools 管理安装、配置、健康和卸载。
   - Conversation selection：composer Skill/MCP picker 管理当前对话的持久选择。
   - Run activation：Agent 在冻结 snapshot 内调用当前 Run 获准的资源。
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

## Interaction surface (corrected)

### Composer selectors

- Knowledge 按钮之后增加紧凑的 Skill 与 MCP 图标按钮。
- 点击按钮打开当前对话的已安装资源多选器；支持搜索、状态、空态与进入管理页。
- 选择立即写入 Backend authority，按 conversation ID 隔离并使用 revision/CAS。
- 刷新、切换对话和跨设备登录后读取服务端选择，不以 localStorage 为 authority。
- 当前 Run 使用冻结 snapshot；运行中修改标记为“下次运行生效”。

### Management pages

- Skill Store：installed/store/detail/install/uninstall/update（若后续支持）。
- Tools：installed/marketplace/configuration/health/install/uninstall。
- 管理页改变 inventory；不把资源静默加入所有对话。

### Slash boundary

- 移除 `/skill`、`/mcp`、`/resources`、`/reload` 及安装/启停/卸载等 Resource lifecycle
  命令，不再把 Slash 作为第二套管理 UI。
- 历史消息中的 legacy `/skill-name` 与现有 `/skill:<name>` 仍可兼容回放；是否继续在
  新输入的 Slash palette 展示显式 Skill invocation 不阻塞 MVP，可单独评审。
- 保留真正属于会话且有独立价值的 built-in command；本任务不扩充 Pi TUI 命令集。

## Conversational acquisition flow

```text
User intent or capability-missing signal
  -> resource_search(kind, capability, query)
  -> ranked bounded candidates with provenance/auth/permission summary
  -> resource_request_install(exact candidate + exact revision)
  -> approval/configuration card
  -> existing Skill or MCP authority performs mutation
  -> install/validate result is persisted and audited
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

- credential-free + validated deployment：可进入 inventory 安装；是否持久选入当前
  conversation 只由 composer picker 决定。
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
- `ConversationSkillSelection`：绑定 conversation、owner、mode、selected installation
  IDs/fingerprints、revision 与 timestamps；结构对齐 MCP selection 的 CAS 语义。
- `McpConversationSelection`：复用现有 `mcp_conversation_selections` authority；composer
  picker 只是新的轻量呈现面。
- 安装状态属于用户 inventory；选择状态属于 Conversation；Run activation 属于不可变
  snapshot。三者绝不只存在浏览器 localStorage，也不得互相冒充。

## UX requirements

- composer Skill/MCP 图标与 Knowledge、Reasoning、Search 同级，不占据正文输入空间。
- popover 只展示当前用户已安装且当前 Workspace/Conversation 有权使用的资源；安装、
  凭据和高级诊断通过“管理 Skill/打开 Tools”进入完整页面。
- 每个对话独立保存选择；按钮可看出是否已有选择，但避免长期铺满 resource chips。
- Resource lifecycle 命令不得占满 `/` palette。
- discovery 结果显示“为什么匹配”、来源、版本、权限/认证需求、作用域和状态。
- mutation 采用 inline approval/config card；用户可拒绝、改作用域或进入完整管理页。
- 资源补装期间，原任务显示“等待安装/认证”；成功后显示“已补充能力，继续执行”，
  而不是要求用户重新输入。
- 失败必须区分：没找到、未 admission、权限不足、需配置、validation 失败、版本漂移、
  continuation 失败。

## Requirements

- [x] 建立 server-side Resource Orchestrator，只编排现有 Skill/MCP 服务，不复制写逻辑。
- [x] 提供统一的 read-only search/info/status contract 与 bounded provider adapters。
- [x] 提供 proposal/approval/configuration/mutation/continuation 状态机。
- [x] 为 Agent 注册 `resource_search` 与 `resource_request_install`；模型不能直接调用
      mutation API。
- [x] 安装后产生新的 Runtime Resource Snapshot revision 并在续跑前校验资源 ready。
- [x] Skill/MCP 搜索、安装、启停、卸载均有 owner/admin/scope/CAS/audit 约束。
- [x] Secret、OAuth token、host/cache path、raw package content 不进入 prompt、trace、日志。
- [x] capability-gap discovery 有预算、去重和循环熔断。
- [x] 删除 Resource lifecycle Slash palette 与 ChatApp mutation handling，保留必要的历史
      replay compatibility。
- [x] 为 Skill 增加 owner-authorized、revision-bound 的 per-conversation selection 存储/API。
- [x] 让 `PrepareRuntimeSkills` 与 runtime snapshot 只投影当前对话授权的持久选择和本 Run
      获准的自动激活，不再默认暴露用户全部 installed Skills。
- [x] 为 composer 增加 Skill/MCP 两个独立、可搜索、可多选的轻量 picker。
- [x] MCP picker 复用现有 conversation selection authority；Skill picker 使用同构语义。
- [x] 安装/卸载只在 Skill Store/Tools 与安全的 conversational acquisition flow 中发生。
- [x] 识别受支持的 Skill/MCP 链接，展示 sanitized preview，并走既有
      admission/validation/credential/approval 流程。
- [x] Agent 自动选择已安装资源采用 run-only activation，不写回 conversation selection。

## Acceptance criteria

- [x] 输入 `/` 不再出现 Resource lifecycle 命令墙。
- [x] 对话 A 选择 Skill-A/MCP-A、对话 B 选择 Skill-B/MCP-B 后，刷新、切换和重新登录
      均保持各自选择，且互不污染。
- [x] 新对话采用明确的空选择或产品默认策略，不继承上一次打开对话的浏览器状态。
- [x] composer picker 只选择 installed inventory；安装/卸载分别在 Skill Store/Tools 完成。
- [x] 当前 Run 启动后选择冻结；运行中更改只影响下一 Run/continuation 并有可见提示。
- [x] 粘贴受支持的 Skill/MCP 链接能得到安全安装预览；恶意、漂移或未 admission 链接
      不得直接安装或执行代码。
- [x] 安装必须绑定 exact fingerprint/revision，候选变化时拒绝旧 proposal。
- [x] “安装 DeepWiki MCP”能完成搜索；需要 credential 时显示配置卡且模型上下文和
      日志中没有 Secret。
- [x] Agent 执行 XLSX 任务发现缺少可靠生成/校验能力时，能搜索并提出合适 Skill，
      经 policy gate 安装后从新 snapshot 续跑，不要求用户重述。
- [x] 安装被拒绝、未 admission、没有匹配、认证失败、validation 失败时任务明确暂停
      或降级，绝不声称已安装/已完成。
- [x] 同一资源不会被并发重复安装；stale proposal、跨用户/跨会话 ID 均被拒绝。
- [x] 当前 Run 的旧 snapshot 不被中途修改；续跑 trace 能看到 old/new revision 与
      mutation audit ID。
- [x] Runtime trace 能区分 user-selected、Agent auto-activated 与 newly-installed resource，
      且不泄漏 Secret、host path 或 raw package content。

## Delivery phases (reopened)

1. **Selection authority**
   - 新增 Skill conversation selection migration/repository/service/API；复核 MCP selection
     的 scope、inherit/custom 与 duplicate conversation 行为。
2. **Composer UX replacement**
   - Skill/MCP picker、服务端持久化、切换/刷新恢复、管理页 deep link；删除 lifecycle
     Slash UI/handler。
3. **Runtime projection**
   - selection-aware Skill preparation、Agent auto-activation policy、snapshot provenance 与
     continuation boundary。
4. **Conversational links and hardening**
   - supported URL parser/preview、supply-chain gates、并发/CAS/security tests、迁移回放、
     rollout/rollback 与 live smoke。

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
- [x] `mm-chat/docs/` 同步 selection contract、resource state machine、security model、
  rollout/rollback 与 operator diagnostics。
- [ ] 在 live workspace 完成三条 smoke：两对话独立选择；粘贴链接安全安装；能力缺失后
  自动选择/补装并续跑原任务。

## Implementation status (2026-08-26)

旧 Phase 1–4 的后端安全链已经实现：Skill/MCP 安装与生命周期 mutation 复用既有 authority，
统一记录 action-specific audit；Agent capability recovery 支持 durable approval、bounded
discovery、Secret/OAuth/Runner 配置 handoff、配置后的 provenance/readiness/owner/CAS
复验、fresh Runtime Resource Snapshot 与同一 Provider loop continuation。

旧实现自动化门禁已经完成（Frontend 992 tests、Backend 全量、RAG 1910 passed/7 skipped、
standalone full gate），但 live UX 验收暴露出状态分层错误，故任务已重开，旧门禁不能
代表新验收完成。进程重启仍按现有 Chat Agent approval
契约把 pending decision 标为 `restart_denied`，不会在无原 Run 执行上下文时伪恢复。

修正版已完成 Backend/Frontend/迁移/运行时/文档闭环。最终门禁为 Frontend 996 tests、
Backend 全量、RAG 1910 passed/7 skipped 与 standalone full gate；PostgreSQL 17 live schema
已迁移至 106，旧 231 个 Conversation 保持不变，Backend/Frontend 候选镜像已部署并通过
ready/proxy health。最后一项真人 UI smoke 保留给浏览器会话执行，不以生产用户数据进行
自动写入测试。

## Research references

- [`research/pi-slash-and-conversational-install.md`](research/pi-slash-and-conversational-install.md)
  — Pi slash/package 行为、Neo 现状差距与推荐控制面。
- [`research/pi-resources-and-xlsx-forensics.md`](research/pi-resources-and-xlsx-forensics.md)
  — 早期 XLSX 根因与 Pi Resource/Skill/Extension/Package 机制。
- [`research/pi-conversation-resource-picker.md`](research/pi-conversation-resource-picker.md)
  — Pi 管理面/会话控制分层、Neo 缺口与 composer picker 修正方案。
