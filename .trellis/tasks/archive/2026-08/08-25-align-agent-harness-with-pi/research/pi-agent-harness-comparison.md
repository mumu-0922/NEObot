# Pi Agent Harness 对照研究

## 研究目标

解释 Neo Chat 当前为何出现以下问题，并给出可执行的 Pi 化边界：

- 生成在绑定项目目录中的文件，又被复制为聊天附件并强制下载；
- `verify_completion` 作为模型可见 Tool 出现在过程卡片中；
- 最终回答重复“已验证”“下载文件”等过程性废话；
- Agent 工具、loop、Skills、Plugins 各自存在，但没有形成一套一致的运行时协议。

对照源码：`/home/mumu/projects/pi-web` 及其锁定的
`@earendil-works/pi-* 0.84.2` 包。

## 1. Pi 的核心运行模型

Pi Web 不另造一套 Agent 协议，而是直接创建并托管一个
`AgentSession`：

```text
Browser
  -> Next API / SSE
  -> one in-process AgentSessionWrapper per running session
  -> AgentSession
  -> model response
  -> Tool calls
  -> Tool results
  -> same AgentSession continues
```

会话历史以 JSONL 保存在 `~/.pi/agent/sessions/<encoded-cwd>/`。浏览历史时
直接读 JSONL；发送消息时才启动或恢复 `AgentSession`。运行中的会话和持久化
会话是两个概念，SSE 只负责实时投影，不成为第二份历史权威。

## 2. Pi 的 loop 停止条件

`@earendil-works/pi-agent-core/dist/agent-loop.js` 的主循环行为是：

1. 模型生成 assistant message；
2. 若存在 Tool Call，则校验参数、执行 Tool、追加 Tool Result；
3. 使用同一上下文进入下一轮；
4. 若本轮没有 Tool Call、没有 steering/follow-up message，则自然结束；
5. abort、Tool 终止标志或明确的 `shouldStopAfterTurn` hook 可提前终止。

Pi 没有让模型调用 `verify_completion` 给自己“盖章”。Tool Result 本身就是
执行事实；是否继续由模型下一轮是否继续调用 Tool 决定。安全边界由参数校验、
Tool hook、abort、超时和 Tool Result 表达，不靠多一轮自证 Tool。

Neo Chat 已经具备 completion-driven loop：Agent 模式没有整轮 wall-clock、
Step、Tool-call、local-round 或 MCP-round 硬上限，仍保留 per-Tool timeout、
取消、approval、输出上限和 no-progress guard。当前偏离 Pi 的主要点不是
“能否一直运行”，而是额外的 completion evidence ceremony。

## 3. Pi 的基础工具

Pi 默认工具是：

- `read`：读取文件；
- `bash`：在当前 `cwd` 执行命令并流式返回有限输出；
- `edit`：精确替换并返回 diff；
- `write`：创建目录并直接写入目标文件；
- 可选 `grep`、`find`、`ls`。

系统提示只说明能力、当前工作目录和简洁原则。不存在“写完必须 publish”、
“验证完必须调用 verify_completion”或“最终必须讲述如何验证”的要求。

Neo Chat 当前等价工具名是 `file_read`、`terminal`、`file_edit`、
`file_write`。底层安全实现比 Pi 更严格：相对路径、`os.Root`、symlink 边界、
写入 CAS/version、三档 permission 和 approval 都已经存在。因此应迁移
**模型可见协议和产品呈现**，不应丢弃现有安全内核。

## 4. Pi 的文件交付

Pi 把项目目录中的文件当作唯一权威：

- 成功的 `write` / `edit` Tool Call 会由 `turn-written-files.ts` 提取；
- UI 在该 Turn 下显示可点击的项目文件 chip；
- 点击后在右侧 File Viewer 打开真实项目路径；
- assistant Markdown 中的相对文件链接也会按 `cwd` 解析并在 File Viewer 打开；
- 失败的 Tool、纯文本提到的路径不会冒充已生成文件。

Pi 的自动 written-file 提取不覆盖 Bash 生成的文件；这类文件可由 assistant
给出相对 Markdown 链接。Pi File Viewer 当前可预览 text/image/audio/PDF/DOCX，
没有 XLSX 预览，未知二进制会落入 text viewer，因此这一点不能原样照搬。

Neo Chat 已经有 Host `artifact_read`，可从绑定工作区安全读取最多 50 MiB 的
普通二进制文件，沿用 workspace fingerprint、permission 和 symlink 边界。
缺少的是“工作区文件引用”领域对象、读取路由和前端 File Viewer。

## 5. Neo Chat 当前错误链路

当前 prompt 强制：

```text
生成并验证文件
  -> 模型调用 publish_file
  -> Host artifact_read 把项目文件完整读回 Backend
  -> Backend 再上传到 File/MinIO
  -> assistant message 绑定 server output attachment
  -> 前端只显示 Download 按钮
```

这产生两个权威：项目目录原件和 MinIO 快照。`displayName` 还可以与项目文件名
不同，用户无法从卡片看出真实路径。绑定项目的 Agent 场景中，这个复制动作
没有必要；它只适合显式分享、跨设备下载或没有绑定 Host workspace 的兼容模式。

`chat_completion_policy.go` 又把成功写操作标记为 outstanding mutation，要求
模型执行后续检查，再显式调用 `verify_completion`。该 Tool 被持久化并由
`processTrace.ts` 当成普通 Tool 卡片展示。Goal wrap-up prompt 还强制要求
“summarize what was done and how it was verified”，直接造成最终回答的验证废话。

## 6. Excel 现场证据

绑定工作区 `/mnt/d/AI/myDOA` 中：

| 项目文件 | 大小 | SHA-256 | 结构 |
| --- | ---: | --- | --- |
| `gold_price_7days.xlsx` | 6247 | `02167840537d8cefa563b32b5124cfbb66d1e3fd816fd7789efedcbc0baa0ecd` | OOXML ZIP 通过 |
| `gold_price_last_7_days.xlsx` | 4463 | `31ccdf9def52c45f5082732b24757d331c25bc80c8956835f556b4c26757ae0e` | OOXML ZIP 通过 |

数据库中两个 `publish_file` 结果的大小和 SHA-256 与上述原件分别完全一致。
因此当前证据不支持“MinIO 复制损坏”；故障是文件卡只下载、没有项目路径、没有
XLSX 预览，以及 Agent 重复生成了两份语义相近的文件。

## 7. Pi 的 Skills 与 Plugins

### Skills

Pi 的 `DefaultResourceLoader` 从 global、project、package 和额外路径统一发现
`SKILL.md`，校验 name/description，去重并记录 collision diagnostics。system
prompt 只注入 name、description、location，并明确要求任务匹配时用 `read`
完整读取 Skill。`disable-model-invocation: true` 只让 Skill 退出自动目录，仍可
显式调用。

Neo Chat 的 progressive disclosure 方向相同，但单独发明了 `skill` Tool 和
缓存路径协议。可保留服务端安装/准入安全边界，后续将 catalog、加载状态和
工具运行时统一成一个 Resource Registry，避免 Skill Store 与 Agent runtime
出现两套事实。

### Plugins

Pi Plugin 是资源包，不只是一个 Tool：同一 package 可贡献 extensions、skills、
prompts、themes。`DefaultPackageManager` 负责 global/project scope、安装、更新、
启停；`DefaultResourceLoader` 将启用资源一次性合并进 AgentSession。项目插件
还受 project trust gate 控制。

Neo Chat 现有 MCP、Skill Store 和本地工具是三个独立入口。近期不应直接嵌入
Pi SDK，而应先建立统一的 runtime resource contract；否则 UI 上叫“插件”，
底层仍是三套生命周期，会继续形成四不像。

## 8. 推荐目标架构

```text
Conversation + bound Workspace
  -> AgentSession/Turn Driver (natural tool-call loop)
  -> Runtime Resource Registry
       - builtins: read / write / edit / bash
       - Skills: catalog + on-demand full load
       - Plugins/MCP: registered extension tools
  -> Tool Result = execution evidence
  -> Workspace File Reference = generated-file authority
  -> File Viewer: open/preview first, download second
```

关键约束：

1. 保留 Go Backend、Host Runner、CAS、approval、permission 和 durable event log；
2. 不直接换成 Pi SDK，也不另起第二个 Agent 控制面；
3. 在绑定 workspace 的 Agent Turn 中，以工作区文件为唯一权威；
4. `publish_file` 仅用于用户明确要求附件/下载、跨设备交付或无 Host workspace；
5. completion evidence 在 harness 内部推导，不再暴露模型 Tool；
6. 最终回答只说结果、必要路径和真实剩余问题，不强制复述验证过程；
7. XLSX 需要 Neo 自己补安全的 in-app viewer，不能照搬 Pi 的未知二进制回退。

