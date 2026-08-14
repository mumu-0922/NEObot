# Agent control facade design

## 目标与非目标

**目标**：以一个 user-bound、server-authoritative facade 暴露 Agent Center 所需
read models 与既有 control transitions；即使 Runtime/Shadow disabled，也允许安全
读取、reconcile 和明确的 held 状态。

**非目标**：本模块不是 Runner、Scheduler、Broker executor、learning checker，也不
在 API process 执行 package code、创建 Child authority 或把 Shadow 输出写入 Chat。

## 架构与数据流

```text
session identity
    |
    v
HTTP Handler -- strict JSON / bounded routes / sanitized errors
    |
    v
Service -- ownership + admin + revision/generation/fingerprint validation
    |                    |                 |
    v                    v                 v
migration 090        owning services    ObjectStore
views/functions      Broker/Cron/Learn   server-only key
```

- Read：Storage projection → Repository → Service → JSON DTO → strict frontend Zod。
- Write：frontend exact authority → Handler → owning Service/function → PostgreSQL。
- Shadow：policy/opt-in snapshot → eligibility fence → injected adapter → content-free
  observation；生产无 adapter 时在执行前返回 `ISOLATION_UNAVAILABLE`。

## 关键决策

| 决策 | 理由 | 影响 |
| --- | --- | --- |
| 独立 `agentcontrol` facade | 不污染 Assistant/MCP/legacy Skill API | Agent Center 可独立演进 |
| Handler 不直接写表 | 保留 Broker/Cron/Learning transition 语义 | replay、CAS、审计保持一致 |
| migration `090` 使用 bounded views 与 `SECURITY DEFINER` functions | API role 不获得 worker/table authority | 需通过 PG17 ACL/replay/down-up gate |
| Shadow default-off 且 adapter injected | exact-host 未通过时不能伪造执行证据 | 生产只报告 held，测试可 synthetic replay |
| Artifact key 仅服务端持有 | 防止跨用户 bucket/object 访问 | 下载必须先绑定 user/run/artifact |
| Kill Switch 单独映射为 `AGENT_AUTHORITY_DENIED` | 不把 authority revocation 误报成 isolation failure | SQL、Service、HTTP 三层均有 regression test |

## 验证与错误矩阵

| 条件 | 结果 |
| --- | --- |
| cross-user Run/Artifact/Schedule | `AGENT_NOT_FOUND`，无 authority change |
| non-admin Draft/policy action | `AGENT_ADMIN_REQUIRED` |
| stale revision/generation/fingerprint | `AGENT_STALE_CONFLICT` |
| budget exhausted | `AGENT_BUDGET_EXCEEDED` |
| active Kill Switch | `AGENT_AUTHORITY_DENIED` |
| no exact-host adapter | `ISOLATION_UNAVAILABLE`，不调度 work |
| malformed JSON/identifier | bounded 400；不透传 PostgreSQL message |

## 安全模型

主要威胁是 cross-user projection、stale replay、object-key 泄露、worker authority
提升、content-bearing diagnostics 与 API-process execution。缓解措施包括全局 session
middleware、SQL user predicates、strict identifier/fingerprint/reason validation、
NOLOGIN owner、least-privilege grants、`json:"-"` object key、no-store responses、
content-free Shadow schema，以及 default-off/no-fallback execution。

## 已知限制

- exact-host isolation 尚未通过，生产 Run enqueue 与 executable Shadow 故意 held。
- Learning Promote 受 `LEARNING_DISABLED` 约束；G20.8 只建立人工 review surface。
- `handler.go` 与 `repository_postgres.go` 接近单文件质量阈值；后续新增第二组独立
  product domain 时应按 route/read-model 拆分，而不是继续堆叠。

## 变更历史

- **2026-08-14 / G20.8**：建立 Agent Center facade、migration `090`、held Shadow、
  Artifact download 与跨层 authority/error regression。
