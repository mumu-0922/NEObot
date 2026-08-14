# Cron schedule、timezone 与 DST 语义

## 调研对象

- `robfig/cron/v3@v3.0.1` 的 parser、`SpecSchedule.Next` 与 DST tests。
- Kubernetes CronJob schedule/timezone/concurrency/missed-run 文档。
- Temporal Schedule overlap/catchup/pause/backfill 文档。

## 已验证事实

### 表达式与 timezone

- `robfig/cron/v3` 默认标准格式是 5 fields：minute、hour、day-of-month、
  month、day-of-week；seconds 与 descriptor 需要显式启用。
- parser 支持把 `TZ=`/`CRON_TZ=` 写进表达式，但 Neo 已有独立 timezone
  冻结字段。若同时允许两处 timezone，会产生歧义与 fingerprint 漂移。
- Kubernetes 使用独立 `.spec.timeZone`，要求有效 IANA tz database name，并在
  binary 内提供 Go timezone database fallback。

### DST 实测

以 `America/New_York` 和 5-field `robfig/cron/v3` parser 实测：

- `30 2 * * *` 在 2026 spring-forward 的不存在本地时间 `02:30` 被跳过，
  下一次是次日 `02:30`。
- `30 1 * * *` 在 2026 fall-back 的重复本地时间触发两次，分别对应 EDT 与
  EST 的两个不同 UTC instants。
- `Schedule.Next(t)` 总是返回严格晚于 `t` 的 real instant；持久化必须用 UTC
  instant 去重，展示时再投影到冻结 timezone。

## 可行方案

### A. 严格 5-field + 独立 IANA timezone（推荐）

- pin `github.com/robfig/cron/v3 v3.0.1`。
- parser 只启用 `Minute|Hour|Dom|Month|Dow`；拒绝 seconds、descriptor、
  `TZ=`/`CRON_TZ=` prefix。
- 独立调用 `time.LoadLocation`；Backend 引入 `time/tzdata`，避免仅依赖 host
  zoneinfo。
- 明确定义：spring gap 不补造不存在时间；fall overlap 对两个 real instants
  各触发一次。

优点：格式窄、可复现、与现有 Go stack 对齐。缺点：依赖一个稳定但发布较旧的
小型 library；timezone database 仍会随重新构建的 Go toolchain 变化，因此 revision
必须保存 timezone name、scheduled UTC instant 与计算器版本。

### B. 自研 parser/calculator

避免依赖，但 day-of-month/day-of-week 组合、name/range/step 与 DST 很容易产生
边界错误；无收益地扩大安全关键代码，不选。

### C. PostgreSQL/extension 计算 schedule

把表达式运算放到 database 会引入 extension/SQL timezone 版本耦合，且当前镜像无
相应 parser；不选。PostgreSQL 只保存与仲裁时间 cursor。

## 收敛结论

采用 A。Canonical schedule 是 trim 后恰好 5 个 fields 的原字符串；timezone 是
独立 IANA name。Revision 同时冻结 `scheduleCalculator=robfig-cron/v3.0.1+go-tzdata`
和首个 `next_trigger_at`。Occurrence identity 使用
`template_id + revision + scheduled_for_utc`，不得用本地 wall-clock 字符串。

## 来源

- <https://github.com/robfig/cron/tree/v3.0.1>
- <https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/>
- 本地 probe：`/tmp/neo-cron-dst-probe`（仅调研临时产物，不进入 repository）。
