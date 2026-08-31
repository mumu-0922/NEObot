# 账户与安全：修改密码与密码找回

## Goal

让已登录用户在前端“设置 → 账户与安全”中安全修改自己的登录密码，并让未登录用户从登录页完成邮件密码找回，消除当前只能登录、无法自助维护凭证的断链。

## What I already know

- Server Auth 已有邮箱/密码 Login、Invite Acceptance、Recovery Request、Recovery Completion、Logout 和全会话撤销能力。
- 前端 `AuthApi` 已封装 Recovery Request/Completion，但当前 Login UI 没有“忘记密码”流程。
- 后端尚无“已登录用户输入当前密码后修改密码”的路由、Service/Repository 方法。
- Settings 目前没有“账户与安全”页签。
- Recovery Completion 已具备共享密码校验、Argon2id 哈希、`credential_revision` 递增、Token 消耗和全 Session 撤销语义。
- 登录邮箱修改需要新邮箱验证和更宽的身份迁移设计，本任务暂不纳入。
- Deployment-level `ACCESS_PASSWORD` 是独立前端门禁，不得与 Server Auth 登录密码混用。

## Requirements

- 在 Settings 增加“账户与安全”页签，展示当前用户的基本身份信息与安全操作。
- 已登录修改密码必须要求：当前密码、新密码、确认新密码。
- 后端新增严格鉴权的当前用户修改密码接口；必须复用共享密码 validator 和 Argon2id 实现。
- 当前密码错误使用安全、稳定的错误响应；不得泄露 PHC、Credential 或 Session 内部信息。
- 修改密码的数据库操作必须原子递增 `credential_revision` 并撤销旧 Session，Cache 同步失效。
- Login 页面增加“忘记密码”，提供两步 Recovery UI：提交邮箱；提交 Token、新密码和确认密码。
- Recovery Request 必须继续使用无账号枚举的通用成功文案；Recovery Completion 成功后回到 Login。
- 前端不得 trim/normalize 密码；仅邮箱按既有规则 trim/canonicalize。
- 密码要求按后端统一规则呈现：至少 9 个 Unicode 字符，最多 256 UTF-8 bytes。
- 增加“退出所有设备”入口，复用现有 `DELETE /v1/me/sessions`，明确操作后需要重新登录。
- 所有 DTO 维持 strict JSON、运行时/类型边界和现有 API Client 分层。
- SMTP 未配置或邮件投递失败时不得伪造已发送事实；UI 保持防枚举文案，运维健康仍由现有后端/部署链报告。

## Acceptance Criteria

- [x] 已登录用户能从 Settings 打开“账户与安全”。
- [x] 当前密码错误时不修改 Credential，不撤销任何 Session。
- [x] 新密码不满足共享边界时，前后端均拒绝且没有部分写入。
- [x] 新密码确认不一致时，仅在前端阻止提交。
- [x] 修改成功后 Credential Revision 恰好递增一次，全部 Session 被撤销，旧 Bearer Token 不再可用，前端返回 Login。
- [x] Login 页面可以发起 Recovery Request，响应不暴露邮箱是否存在。
- [x] 有效 Recovery Token 可以设置新密码，Token 不可重放，全部旧 Session 失效。
- [x] 无效、过期、已使用 Recovery Token 使用统一失败反馈。
- [x] 密码前后空格能作为密码原文的一部分传给后端，不被前端裁剪。
- [x] “退出所有设备”成功后清理本地会话并返回 Login。
- [x] Backend focused tests、Frontend focused Vitest、format/lint/typecheck 通过。
- [x] 跨层 Auth contract 与运维 Recovery 文档同步。

## Definition of Done

- Backend Handler/Service/PostgreSQL Repository 与 focused tests 完成。
- Frontend Settings/Login/Recovery/API/types/i18n 与 focused Vitest 完成。
- 安全边界：共享校验、Argon2id、Revision Fence、Session Revocation、防枚举、Token 单次使用均有测试证据。
- 文档和 Trellis Auth Identity spec 在行为变化处同步。
- 验证通过并形成聚焦提交，不推送远端。

## Technical Approach

- 扩展现有 `internal/auth`，不创建第二套 Credential 服务。
- Repository 使用单事务锁定当前 Credential，验证后的 Service 提交新 PHC；SQL 递增 Revision 并撤销目标 Session。
- 前端复用现有 `createNeoChatApiClient().auth` 和 `ServerAuthGate`，新增 Account Security panel 与 Login 内 Recovery 状态机。
- 将 Session 失效后的前端收口集中到现有 `authSession` 边界，避免 Settings 自行复制 Token 清理逻辑。

## Decision (ADR-lite)

**Context**: Recovery 已覆盖“无法登录”，但已登录用户不应依赖邮件才能轮换密码。

**Decision**: 新增要求当前密码的 authenticated password-change route；修改成功后撤销全部 Session 并要求重新登录；Recovery 保持 public、rate-limited、anti-enumeration；邮箱修改另立任务。

**Consequences**: 增加一条跨 Handler/Service/Repository/UI 的高敏感写链，必须同步验证 Revision 与 Session Cache；用户修改密码后会中断当前会话，但换来最简单、最一致的旧凭证失效边界与完整的用户自助凭证闭环。

## Out of Scope

- 修改登录邮箱。
- 多因素认证、Passkey、TOTP、设备 Session 列表。
- 管理员代改其他用户密码。
- 修改 Deployment-level `ACCESS_PASSWORD`。
- 新增 SMTP Provider 或邮件模板管理后台。

## Technical Notes

- Backend contract: `.trellis/spec/backend/auth-identity.md`。
- Existing Backend: `mm-chat/backend/internal/auth/{handler,service,identity_repository_postgres}.go`。
- Existing Frontend: `mm-chat/frontend/src/components/app/{AccessPasswordPage,ServerAuthGate}.tsx`、`components/settings/SettingsPage.tsx`、`services/api/client/server/authApi.ts`。
- Existing operations reference: `mm-chat/docs/deployment/secret-rotation.md`。
- 当前 `AccessPasswordPage` 使用 `password.trim()`，与后端“不 trim 密码”的 contract 冲突，本任务一并修复并覆盖测试。
