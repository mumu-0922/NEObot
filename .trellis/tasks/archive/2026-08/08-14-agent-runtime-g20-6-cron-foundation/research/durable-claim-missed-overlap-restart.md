# Durable claim、missed-run、overlap 与 restart authority

## 外部模式

### Kubernetes CronJob

- `startingDeadlineSeconds` 把“应触发时间”与“最晚允许开始时间”分开；超时的
  occurrence 被 skipped，未来 occurrence 继续。
- concurrency policy 是 `Allow`、`Forbid`、`Replace`。官方明确 Job 可能重复或
  缺失，因此 workload 仍需 idempotent。
- controller 对 missed occurrences 有上限，避免 outage 后无界展开。

### Temporal Schedule

- overlap 支持 `Skip`、`BufferOne`、`BufferAll`、`CancelOther`、
  `TerminateOther`、`AllowAll`。
- catchup window 约束 service outage 后允许补跑的时间范围。
- pause 不停止已经启动的 execution；backfill 是显式动作。

### PostgreSQL 17

- 官方文档明确 `FOR UPDATE SKIP LOCKED` 适合多个 consumer 访问 queue-like
  table，但返回的是不一致视图，不能当一般查询语义使用。
- 因此它只用于 claim 候选；最终 revision、authority、overlap 与 idempotency
  必须在同一个 locked transaction 内重复校验。

## Repo 约束映射

- migration `084` 已使 PostgreSQL 成为 Run/Step/Attempt/event/lease/Kill Switch
  authority；migration `086` 已要求 mutable effect 用稳定 idempotency key 和
  `outcome_unknown`，所以 Scheduler 不能把“没有看到 ack”解释为“没有创建 Run”。
- Redis 最多只能发 ID-only wake hint，不能持有 schedule cursor、claim 或 retry
  state。
- production worker、API、Chat/frontend、Runner promotion 在本 slice 必须继续
  disabled。

## 推荐持久化流

```text
due template cursor
  -> short PostgreSQL lease claim (SKIP LOCKED)
  -> Go computes bounded occurrences from frozen schedule/timezone
  -> one transaction inserts occurrence facts + advances exact cursor
  -> pending occurrence lease claim
  -> one transaction rechecks authority/overlap/current revision
  -> normal Orchestrator Run enqueue with stable Cron idempotency key
  -> trigger links exact Run, or becomes sanitized skipped/denied fact
```

关键点：

- 分离 schedule cursor claim 与 occurrence claim。Crash before cursor advance 会重算同一
  cursor；unique occurrence constraint 去重。Crash after materialization 由 occurrence
  lease reclaim。
- 每个 claim 有 generation/owner/expiry fence。旧 worker 不得 advance、enqueue、
  release 或 terminalize。
- `scheduled_for` 使用 UTC `TIMESTAMPTZ`；unique key 是
  `(template_id, revision, scheduled_for)`。
- Run idempotency key 由同一 occurrence identity 派生。SQL trigger enqueue 与
  Orchestrator enqueue 同 transaction，避免 trigger/Run 双写裂缝。

## Policy 收敛

- missed policy：`skip`、`fire_once`、`catch_up`；所有 policy 都有
  `catchupWindowSeconds` 与 `maxCatchupRuns <= 100`，无界 backlog 禁止。
- overlap policy：`skip`、`buffer_one`、`allow`。不做自动 Replace/Cancel，避免把
  Scheduler 变成隐式 kill authority；`buffer_all` 也因无界 backlog 排除。
- pause：停止新 occurrence；resume 从当前时间之后的 next instant 继续，并记录
  paused window 的 sanitized skipped count，不突发补跑。需要补跑时未来 API 必须做
  显式 backfill，新建独立 audit action。
- retry：只重试“尚未证明 Run 已 enqueue”的 occurrence claim；Run 一旦存在即不
  创建替代 Run。每次 retry 使用相同 occurrence/idempotency identity。Mutable effect
  重试继续服从 migration `086`，Scheduler 不越权重试 effect。

## Cleanup

- Runtime/Scheduler disabled 时仍允许 reclaim expired claims、terminalize exhausted
  retries、prune 已终态 trigger/audit history 和最终清理已删除 template。
- delete 是 tombstone + audit，不直接丢失未终态 trigger；retention function 才执行
  bounded physical cleanup。

## 来源

- <https://www.postgresql.org/docs/17/sql-select.html>
- <https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/>
- <https://docs.temporal.io/schedule>
