# Immutable revision、automation approval 与 trigger revalidation

## 当前 authority seams

- migration `083`：`skill_package_versions` 持有 immutable package/runtime
  fingerprints；`skill_package_candidates.status='admitted'` 与 exact user
  `skill_installations` 是当前 Skill authority。
- migration `084`：`users.deleted_at`、hierarchical Kill Switch、normal Run enqueue
  与 append-only Run events。
- migration `086`：append-only `agent_effect_grant_revocations`，Secret ref 通过
  Grant/Kill Switch/短生命周期 handle 受控；Runner 无 vault credential。
- migration `087`：Child authority独立于 Cron，depth 1 被物理移除 `cron_manage`；
  Cron 不能从 Child 继承或扩大 authority。

## 威胁

1. Template edit in place 会让已批准 schedule 静默换 model/package/grant/input。
2. 安装新 Skill、扩大 Grant 或更新 model 后，旧 template 若动态“跟随 latest”会
   自动获得新 authority。
3. interactive `once`/`per_commit` approval 不能被 Scheduler 当成 automation approval。
4. claim 后 pause/delete/revise/revoke 若不复查，会启动 stale occurrence。
5. 保存 Secret value、prompt/input plaintext 或 Provider credential 会把 Cron database
   变成 secret/content store。

## 推荐 authority model

### Logical template + immutable revisions

- logical template 只保存 owner、current revision、lifecycle、schedule cursor 与 claim
  state。
- 每次冻结字段变化都插入新 immutable revision；旧 revision 永不 update。
- revision 保存 exact installation/admission/package/runtime、input reference +
  fingerprint、model tuple、Grant ID/fingerprint + sanitized canonical authority、
  Registry fingerprint、budget、Egress、Secret refs、schedule/timezone/policies。
- 不保存 input/prompt body、Secret value、workspace bytes、Tool arguments/results。

### Automation approval

- 每个 revision 必须有独立 append-only approval，绑定 revision fingerprint。
- class 只允许 `automation_read_only` 或 `automation_brokered_effect`；interactive
  approval class 不合法。
- `automation_brokered_effect` 不绕过 migration `086` 的 per-effect Prepare/Commit
  规则。它只表示“允许 Scheduler 建立 Run”，不是提前批准具体 mutable Commit。
- 撤销 approval 写 append-only revocation fact；不更新/删除原 approval。

### 每次 trigger 的 final check

SQL 在 occurrence lock 内依次重查：

1. logical template 仍 active，revision 仍 current，fingerprint/cursor/claim 未 stale；
2. owner 未删除；
3. exact Skill installation 仍属于 owner，exact admission 仍 admitted，package/runtime
   fingerprints 完全相同；
4. Grant 未在 `agent_effect_grant_revocations` 中，revision expiry 未过；
5. automation approval 未 revoked；
6. global/scheduler/user/project/skill/secret scopes 无 active Kill Switch；
7. overlap/budget/retry policy 未被 caller 改写。

任何失败只产生 sanitized code（例如 `OWNER_REVOKED`、`SKILL_REVOKED`、
`GRANT_REVOKED`、`SECRET_REVOKED`、`APPROVAL_REVOKED`、
`KILL_SWITCH_ACTIVE`、`STALE_TEMPLATE`），不得 fallback 到 latest authority。

## 设计后果

- 后续 Grant/Skill/model 更新只能创建新 revision + 新 approval；旧 template 不扩大。
- exact Secret current existence 暂不由 Runner或 Cron DB 探测；G20.6 以冻结 Broker
  refs + current secret-scope Kill Switch/revocation seam fail closed。未来真实 vault adapter
  上线时必须在同一 trigger admission boundary 注入 current availability resolver。
- 本 slice 不接 production Scheduler，因此新增的 database/Go 控制面只能由 tests 和
  未来 private worker 调用。
