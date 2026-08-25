# PRD: Align Chat Agent Harness with Pi

## 1. 背景

Neo Chat 已具备 completion-driven Agent loop、Host workspace、读写执行工具、
Skills、MCP/Plugins、持久化 Tool timeline 和输出附件，但这些能力的领域边界
不统一。当前文件任务会把项目目录原件二次发布成下载附件，completion policy
还要求模型显式调用 `verify_completion`，导致过程泄漏、冗余调用和噪声回答。

## 2. 用户问题

用户要求：

- Agent 生成的文件留在项目目录，并可从对话直接打开；
- 不要把项目文件默认复制成必须下载的聊天附件；
- 文件正常时不要输出“已验证”式废话；
- `read/write/bash/edit`、loop、Skills 和 Plugins 应形成一套 Pi 式 Agent harness；
- 先研究 Pi，再实施，避免只改表面 UI。

## 3. 已确认事实

- 两份现场 XLSX 原件都是有效 OOXML ZIP；发布记录大小和 SHA-256 与原件一致。
- 现有前端对 server output attachment 只有 Download，没有 Open。
- Host Runner 已有安全的 binary `artifact_read` 能力。
- Neo Agent 模式已是 completion-driven，不再受整轮 5 分钟/固定轮数上限约束。
- `verify_completion` 和 forced verification narration 是当前主要偏离点。
- Pi 以自然 Tool-call loop、项目文件引用、ResourceLoader 和 package resources
  形成一条统一链路；Pi 本身没有 XLSX preview。

## 4. 目标

### 4.1 Agent loop

- 模型有 Tool Call 时继续，无 Tool Call 时自然完成。
- 保留取消、approval、per-Tool timeout、输出上限、Provider failure 和 no-progress
  guard。
- 将验证状态变成 harness 内部状态，不要求模型调用验证 Tool。
- `verify_completion` 不再出现在新 Turn 的模型 Tool 列表和用户过程卡片中。

### 4.2 工具协议

- 对模型提供 Pi 语义的 `read`、`write`、`edit`、`bash`。
- 内部继续复用现有安全实现；保留旧 Tool 名的历史回放兼容。
- Tool Result 是执行事实，成功/失败/版本/路径必须结构化。

### 4.3 文件交付

- 绑定 Host workspace 时，生成文件以 workspace-relative path 作为唯一权威。
- 成功写入或显式呈现的文件在 assistant Turn 下显示 Workspace File Card。
- 主动作是“打开”；“下载”是次动作。
- Bash 生成的二进制文件必须通过结构化路径结果登记，不能扫描自然语言猜路径。
- `publish_file` 仅在明确要求可下载附件、跨设备分享或兼容无绑定工作区时使用。
- 项目文件后续变化时，打开卡片读取当前文件；历史 Turn 同时保留生成时 version/
  hash，显示“文件已变化”而不是静默冒充旧快照。

### 4.4 文件查看

- text/image/audio/PDF/DOCX 使用对应 viewer。
- XLSX 在应用内至少支持 sheet 列表、单元格值和基础表格预览；复杂图表可提示
  使用系统 Excel 打开或下载。
- 所有读取都经过 authenticated conversation/workspace binding、permission、
  fingerprint、path traversal 和 symlink 边界校验。

### 4.5 Skills / Plugins

- 一个 Runtime Resource Registry 统一汇总 builtins、Skills、MCP/plugin tools。
- Skill catalog 只作为路由元数据，命中后加载完整指令。
- Plugin 的启停必须原子影响其注册的 tools/skills/prompts；项目级资源受 trust
  gate 控制。
- 本阶段不直接替换成 Pi SDK，不引入第二份会话或 Tool authority。

### 4.6 回答呈现

- 正常完成只报告结果和可操作文件卡/路径。
- 不强制输出“已验证”“验证方式”“下载 Excel”。
- 失败或部分完成时才报告验证失败、阻塞原因和安全下一步。
- 内部 policy/Goal Tool 不显示为用户工作过程卡。

## 5. 非目标

- 不把 Go Backend 重写成 Pi 的 TypeScript AgentSession。
- 不废弃现有 PostgreSQL 会话、Tool event、permission、approval 或 Host Runner。
- 不允许浏览器直接读取任意本机绝对路径。
- 不在本阶段实现完整 Office 编辑器或 Excel 图表编辑。
- 不把 MCP、Skill Store 和 Plugin UI 一次性全部重写。

## 6. 建议分期

### Phase A：收口错误链路

- 内部化 completion evidence，移除新 Turn 的 `verify_completion`。
- 删除 forced verification narration。
- 增加 workspace file reference、受控读取 API、文件卡和 Open-first 交互。
- 为 XLSX 增加基础 in-app preview。
- 保留显式 publish/download fallback。

### Phase B：统一模型工具协议

- 模型可见名称迁移到 `read/write/edit/bash`。
- 持久化事件和 UI 同时兼容旧名称。
- 统一 written/presented file extraction。

### Phase C：Resource Registry

- 统一 builtins、Skills、MCP/plugin tools 的注册、启停、诊断和 project trust。
- 整理 Skill Store / MCP / Plugins UI 的领域命名和生命周期。

## 7. 验收标准草案

1. 在绑定工作区请求生成 XLSX 后，原文件只存在于项目目录；不自动创建 MinIO
   output file，除非用户明确要求附件。
2. assistant Turn 下出现带真实 workspace-relative path 的文件卡，点击主区域打开
   应用内 XLSX preview，下载按钮为次级动作。
3. 写入、Bash 生成、失败写入、路径越界、symlink、文件被后续修改均有测试。
4. 新 Agent Turn 不提供/调用/展示 `verify_completion`。
5. 最终回答不出现模板化“已完成验证”，除非验证失败本身是用户需要知道的结果。
6. Agent 在持续取得进展时没有整轮时限；重复相同结果或连续错误仍会安全停止。
7. Chat 模式不暴露 Agent 工具；Agent 模式使用同一模型连续执行 Tool loop。
8. 旧历史中的 `file_read`、`terminal`、`verify_completion` 事件仍可加载，不破坏
   会话回放。

## 8. 已确认决策

- 本次一起完成 Phase A + Phase B。
- XLSX “打开”定义为应用内基础预览；系统下载/另存为是次级动作。
