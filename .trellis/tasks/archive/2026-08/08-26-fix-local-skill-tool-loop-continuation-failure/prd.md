# Fix Provider failure attribution and explicit Resource-link recovery

## Goal

修复 Agent 首轮 Provider 故障被错误归因成 Local Skill Tool Loop 的问题；同时让用户明确
粘贴受支持的 Skill/MCP 链接并要求安装时，优先进入 Backend 的确定性安全搜索/安装链，
不因模型 Provider 暂时不可用而直接失败。

## Live evidence

- 2026-08-26 14:07 的 live Turn `da8f2fa5-bb7f-437d-93ef-e4dd86a630ca`
  在 199ms 内终止，只有 context 与 generation events，没有任何 Tool Call。
- 持久化错误码为 `LOCAL_SKILL_PROVIDER_FAILED`，但相同 `PJRSVY` 配置的真实探针证明：
  `gpt-5.6-luna`、`gpt-5.6-terra` 在 0、2、16、17、18 个 Tool definitions 下均返回
  HTTP 502 / `PROVIDER_UPSTREAM_FAILED`。
- 因此本次不是 Tool schema、Tool 数量、resource_search 或 Skill 执行失败，而是上游
  Provider 整体不可用后被本地 runtime presence 错误改写了归因。
- AIHero 链接仍只是 admitted Skill Store 的 discovery alias；当前 Store 没有
  `grill-me` 候选，不能绕过 supply-chain 直接下载执行。

## Requirements

- 首轮 typed Provider failure 必须保留固定 `ProviderFailureCategory`，不得因 Local Skill、
  MCP 或 Resource runtime 存在而伪装成某个 Tool backend 失败。
- 对可重试的同步 Provider 启动故障做一次有界重试；遵守 context cancellation，禁止无限
  重试或重复执行任何 Tool mutation。
- 用户明确要求安装且文本中恰有一个受支持的 Skill/MCP discovery link 时，Backend 在
  Provider 调用前确定性执行 `resource_search`，复用现有 owner/admission/revision/
  credential/approval/mutation authority。
- 零候选是正常终态：Assistant Message/Turn 为 completed，明确说明未找到 admitted
  候选且未安装，不制造红色 Provider failure。
- 唯一 exact 候选才可继续安装；多候选、无 exact 候选、未 admission、需配置或安装失败
  必须 fail closed，不声称已安装。
- 成功安装继续使用既有 exact revision、audit 与 snapshot refresh 语义；不得写回
  conversation selection，不得执行网页、Git/npm/Shell 指令。
- 普通 Agent 请求若 Provider 仍不可用，显示稳定的 `PROVIDER_UPSTREAM_FAILED`，不得泄漏
  upstream body、API Key、Base URL 或 secret。

## Acceptance criteria

- [x] 502 首轮错误不再生成 `LOCAL_SKILL_PROVIDER_FAILED`。
- [x] retryable 同步 Provider failure 至多重试一次，取消能立即终止。
- [x] 明确 AIHero Skill 安装链接在 Provider 不被调用的情况下完成 bounded search。
- [x] `grill-me` 无 admitted candidate 时得到 completed 的“未安装”答复和 Resource search trace。
- [x] exact admitted credential-free candidate 可经同一条安全 authority 安装一次。
- [x] 恶意、含 query/fragment/userinfo/非 443 端口、多个或不受支持链接不进入确定性安装链。
- [x] Backend focused/full tests、vet 与 standalone gate 通过；live Backend 部署后回归 smoke 通过。

## Out of scope

- 从 AIHero 页面直接下载或执行任意 Skill。
- 自动 admission 未审核 Skill，或把 Codex 本机 Skill 静默复制进 Neo Chat。
- 在 Provider 502 时静默改用其他模型/供应商。
- 重构整个 Agent loop 或 Resource supply chain。
