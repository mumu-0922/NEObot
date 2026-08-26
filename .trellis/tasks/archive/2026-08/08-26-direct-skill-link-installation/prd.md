# Direct Skill link installation

## Goal

当已认证用户在 Agent 对话中明确粘贴 Skill 链接并要求安装时，Neo Chat 直接把该
Skill 导入当前用户的私有 Skill 库；不要求候选先进入管理员审核商店，也不把私有直装
Skill 发布给其他用户。

## What I already know

- 用户明确否决“先进入已审核资源商店”的门槛，并要求自己决定安装。
- 当前 AIHero 页面公开给出的安装坐标是
  `npx skills@latest add mattpocock/skills --skill=grill-me`，源码位于 GitHub。
- 当前 Neo Chat 已有 GitHub exact-commit ZIP 获取、归档边界校验、canonical ZIP、SBOM、
  content-addressed object、私有 Library、卸载和 Local Skill Runtime materialization；缺口是
  所有安装都被 `admitted Store candidate` 数据库约束挡住。
- 当前显式链接路径只做 Store search；它不会获取 AIHero 页面或 GitHub 源。

## Decisions

- “直接安装”表示跳过 Store admission 和管理员 review，而不是执行网页里的 Shell/npm
  命令。服务端只解析来源坐标、固定 exact Git commit，并复用现有文件结构和大小校验。
- 显式用户安装与 Agent 自动补能力分开：本任务只处理用户明确说“安装”的链接。
- 直装包是 owner-private，不进入公共 Skill Store，也不能被其他用户安装或读取。
- 当前 Run 的 Resource Snapshot 保持冻结；安装完成后从下一次 Agent Turn 可用。
- 同一 exact source/fingerprint 重试必须幂等；同名但不同来源/内容不得静默覆盖。

## Requirements

- 支持当前 AIHero Skill 页面链接，并为后续 `skills.sh` / GitHub directory adapter 保留统一
  `DirectSkillSource` seam；本任务不得执行 `npx`, `git`, 页面脚本或任意安装命令。
- AIHero resolver 仅允许 HTTPS、固定 host、无 userinfo/query/fragment/异常端口，限制响应
  大小、重定向、Content-Type 和超时。
- 从页面文本中只接受唯一、严格受限的 `skills add owner/repo --skill=<slug>` 坐标，并要求
  slug 与 URL identifier 一致。
- GitHub source 必须先解析为 40 位 commit，再从 exact commit ZIP 中唯一定位 frontmatter
  `name` 相符的 Skill 目录。
- 复用 `ValidateArchive`、canonical package、SBOM、object storage 和运行时重校验；这些是
  格式/完整性校验，不是内容审核或 Store admission。
- 新增 owner-private direct candidate authority；数据库必须保证该候选只能安装给同一 owner。
- Library/list/uninstall/runtime selection 对公共 admitted 和私有 direct 安装保持一致。
- 显式 AIHero link flow 直接执行 `direct import + install`，不先 `resource_search`，不调用
  LLM Provider，返回清楚的成功/已安装/冲突/来源不可用结果与 Tool trace。
- mutation audit 只记录安全来源坐标、fingerprint、owner/conversation/action/result，不记录
  页面正文、Skill 指令或用户 secret。

## Acceptance Criteria

- [x] 原始 `https://www.aihero.dev/skills-grill-me帮我安装这个skill` 可直接安装到当前用户 Library。
- [x] 安装不创建公共 Store 条目，不需要 administrator review/admission。
- [x] Provider 为 502 时显式直装仍能完成。
- [x] exact retry 幂等；同名不同 fingerprint/source 明确冲突且不覆盖。
- [x] 另一用户无法读取、安装、运行或卸载该 private direct candidate。
- [x] query/fragment/userinfo/非 443、多个链接、坐标不一致、多个同名目录、超限/坏 ZIP 均拒绝。
- [x] 安装过程不执行 npm/git/Shell/Skill 内容；Local Runtime 仍只在后续 Agent Turn 读取包。
- [x] Backend migration replay/down guard、focused/full tests、race/vet 和 standalone gate 通过。
- [x] live Backend 部署后，原始 AIHero 消息得到 completed 安装结果且 Library 可见。

## Out of Scope

- Agent 在没有用户明确安装意图时静默联网安装 Skill。
- 任意网页、任意 Shell 安装命令、private GitHub repository 或需要登录的来源。
- 自动解析自然语言依赖或一次安装整个 Skill pack。
- MCP 直装规则变更。

## Research References

- [`research/direct-skill-source.md`](research/direct-skill-source.md)

## Definition of Done

- Tests and migration verification pass.
- Specs/contracts reflect private direct trust authority.
- Backend image is built, deployed, and live-smoked without persistent test residue.
