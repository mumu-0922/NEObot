# 密码策略：ASCII 可见字符与至少 8 位

## Goal

将 Server Auth 新密码策略收敛为仅允许 ASCII 数字、英文字母和可见符号，长度至少 8 位、最多 256 位，同时避免新规则锁死历史账号。

## Requirements

- 新设或轮换的密码只能包含 ASCII 可见字符 `U+0021` 到 `U+007E`。
- 新密码至少 8 位、最多 256 位；不强制数字、字母、符号三类同时出现。
- 空格、控制字符、中文、Emoji 和其他非 ASCII 字符不得用于新密码。
- Bootstrap、Invite Acceptance 新建凭据、Recovery Completion、Authenticated Password Change 的新密码使用同一后端策略。
- Login、Invite Acceptance 已有凭据校验、Password Change 当前密码校验继续兼容历史 UTF-8 密码，避免存量账号被锁死；成功轮换后即受新策略约束。
- 前端 Account Security 与 Recovery 表单使用相同策略并提供中英日错误提示。
- 不 trim、不 normalize、不记录密码明文。

## Acceptance Criteria

- [x] 恰好 8 位 ASCII 数字、字母或可见符号的新密码可通过。
- [x] 7 位密码、空格、控制字符、中文、Emoji 和其他非 ASCII 字符被拒绝。
- [x] 256 位 ASCII 密码通过，257 位被拒绝。
- [x] 历史含空格或 Unicode 的正确密码仍能登录并可作为改密时的当前密码验证。
- [x] 新密码策略在 Backend、Frontend、E2E 与公开运维文档中一致。
- [x] Auth focused tests、Frontend focused tests、lint/typecheck、Backend full tests/vet 与安全关卡通过。

## Definition of Done

- 测试覆盖策略边界和历史凭据兼容路径。
- 文档与 Trellis Auth contract 更新。
- 本地部署使用新镜像并完成最小烟测。
- 创建 focused conventional commit，不 push。

## Technical Approach

拆分“新密码策略校验”和“历史密码验证输入校验”。`hashPassword` 与所有新密码入口使用严格 ASCII 策略；`verifyPassword`、Login 及当前密码验证仅执行有界的 legacy verification 校验，然后进行 Argon2id 比对。Frontend 只负责新密码表单的同构预校验，Backend 保持最终权威。

## Decision (ADR-lite)

**Context**: 旧策略允许 Unicode 与空格；直接替换共享校验器会使这些历史账号无法登录。

**Decision**: 新密码严格限制为 8–256 个 ASCII 可见字符，验证既有 hash 时保留旧输入兼容。

**Consequences**: 无需迁移或重哈希；用户下次轮换密码时自动进入新策略。登录入口与新密码入口不再共享完全相同的 validator，但共享明确命名、集中测试的边界。

## Out of Scope

- 不增加“必须同时包含大小写、数字和符号”的复杂度规则。
- 不强制历史用户立即改密。
- 不修改 Argon2id 参数、Session 或 Recovery 语义。

## Technical Notes

- Backend: `mm-chat/backend/internal/auth/password.go`, `service.go` and tests.
- Frontend: `mm-chat/frontend/src/lib/auth/passwordPolicy.ts`, locale copy, component tests and Auth E2E.
- Specs/docs: `.trellis/spec/backend/auth-identity.md`, `mm-chat/docs/deployment/secret-rotation.md`, tracking entry where policy is recorded.
