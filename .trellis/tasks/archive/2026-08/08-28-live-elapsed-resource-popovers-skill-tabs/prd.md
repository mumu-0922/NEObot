# Live elapsed time, exclusive resource popovers, and Skill Store tabs

## 背景

当前聊天界面存在三处体验断裂：

1. Agent 正在运行时，消息底部不显示已运行时长，只在运行结束后显示最终用时。
2. 对话输入框中的 Skill 与 MCP 资源选择器各自维护开关状态，能够同时展开并互相遮挡。
3. 技能商店把“已安装”和“商店候选”混在同一列表，信息架构与已经稳定的 MCP“已安装 / MCP 商店”双页签不一致。

## 目标

- 正在生成的 Assistant/Agent 消息底部持续显示实时累计用时。
- 输入框的 Skill 与 MCP 资源弹层严格互斥，任一时刻最多展开一个。
- 技能管理页采用与 MCP 工具页一致的“已安装 / 技能商店”双页签结构。

## 功能需求

### 1. 实时累计用时

- Assistant/Agent 消息处于运行状态时，在消息内容最下方显示“用时 X秒 / X分X秒 / X小时X分”一类实时文本。
- 时间至少每秒刷新一次，从该次生成的稳定开始时间计算，不能因 React 重渲染、流式增量或切换页面而归零。
- 运行期间只补充实时元数据，不提前显示仅属于完成态的操作按钮。
- 完成后停止计时器，并继续以服务端/消息记录中的最终 `timing.duration` 为权威值。
- 组件卸载、切换消息或运行结束时必须清理 timer，避免泄漏和后台空转。

### 2. 资源选择器互斥

- Skill 与 MCP 选择器使用同一份“当前展开项”状态。
- 打开 Skill 时立即关闭 MCP；打开 MCP 时立即关闭 Skill。
- 再次点击当前项、点击外部、切换对话或打开资源管理页时正确收起。
- 已选 Skill/MCP 的对话级保存逻辑不变。
- 本期只约束输入框中的 Skill/MCP 资源选择器；Agent 模式、权限和模型选择器沿用现有行为。

### 3. 技能管理双页签

- 技能管理页顶部新增两个互斥页签：`已安装` 与 `技能商店`，视觉和交互对齐 MCP 工具页。
- `已安装`页只展示本机已安装 Skill，支持查看详情和卸载。
- `技能商店`页只展示可安装候选，支持搜索、查看详情和安装。
- 默认进入`已安装`页；带有商店搜索词或候选 Skill 深链时进入`技能商店`页。
- 安装成功后刷新两侧数据并自动切回`已安装`页，让用户立即看到安装结果；卸载后刷新已安装列表。
- 空状态、加载态、错误态和键盘/ARIA 语义必须保留。

## 商店数据源决策

本期不选择或接入新的外部 Skill Marketplace。`技能商店`页继续使用现有后端提供的 admitted catalog（已准入候选目录），仅重构前端信息架构。

后续接入 GitHub 索引、第三方 Marketplace 或 URL discovery 时，应在后端资源目录层替换/扩展 provider；本期 UI 不绑定任何外站协议，避免形成不可迁移耦合。

## 验收标准

- 使用 fake timer 或等价测试证明运行时用时会递增、完成后停止且显示最终 duration。
- 交互测试证明 Skill/MCP 不可能同时处于 open 状态。
- 技能页测试覆盖默认页签、搜索/深链进入商店、安装后切回已安装，以及中英文关键文案。
- Frontend focused tests、format、lint、typecheck 通过；共享 UI 或路由受影响时补 build 验证。
- 不修改后端 API、数据库和运行时数据。

## 非目标

- 本期不决定最终公共 Skill Marketplace 来源。
- 本期不新增未经用户触发的自动安装策略。
- 本期不改 MCP Server 的安装、启停与商店实现。
- 本期不调整 Agent loop、模型协议或任务持久化。

## 相关范围

- `mm-chat/frontend/src/components/chat/MessageItem.tsx`
- `mm-chat/frontend/src/components/chat/ConversationResourcePickers.tsx`
- `mm-chat/frontend/src/components/skills/SkillStore.tsx`
- `mm-chat/frontend/src/i18n/locales/*/SkillStore.json`
- `mm-chat/frontend/src/__tests__/`
- 对应 frontend 设计文档与 Trellis spec（若行为契约变化）
