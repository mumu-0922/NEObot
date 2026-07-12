# Phase 15.2B Durable Consumer 实施计划

- 状态：implemented and independently verified
- 日期：2026-07-12
- 基线：migration `009`、Phase 15.2A Contract/Bake-off 已完成
- 目标：完整建立 Extension-independent `010` Schema 与可验证的 Python Durable
  Consumer；在 `011` 和首个 Generation 晋升前保持真实 Stage Claim 关闭

> 本计划是 Phase 15.2B 的唯一执行顺序。Python 不建表、不解释 ACL 为授权事实；Go
> Migration Runner 管 DDL，Postgres `SECURITY DEFINER` Function 管原子状态机，Python
> 只编排 Poll、Rescan、Heartbeat、Retry、DLQ 与 Replay。未通过 `011`/Generation Gate
> 时，Worker 只能 Dark-run，不得消费真实 Parse/Embedding/Purge Job。

## 1. 当前裂缝与本阶段裁定

| 裂缝                                   | 裁定                                                                       |
| -------------------------------------- | -------------------------------------------------------------------------- |
| 旧 Governance 只有 `model_api_version` | `010` 通过显式、事务内 Mapping 增加 `model_id` 与 Contract Hash；禁止猜值  |
| Consent API/唯一键只按 Processor       | `010` 升级为 `subject + processor + endpoint + model`；Go Runtime 同步升级 |
| 旧 Producer 先写无 Generation Job      | 兼容期显式写 `legacy_projection_unbound=true`；Claim Function 永远排除     |
| `011` 尚未晋升                         | `RAG_WORKER_DISPATCH_ENABLED=false`、空 Job Handler Registry；默认不 Claim |
| Outbox 无 Token/Expiry/DLQ             | `010` 增加不可复用 Token、Bounded Attempts、结构化 Error 与 Replay Audit   |
| Redis 可能丢失                         | 每秒 Postgres Poll + 每 30 秒强制 Rescan；Redis 只发常量 Wake Hint         |
| API Worker 与 RAG Worker 故障域不同    | Python 独立容器/进程；不得作为 Go API goroutine                            |

本阶段不修改前端，不调用 MinerU/Jina，不读 MinIO，不生成 Chunk/Embedding，不发布
Search Projection。Phase 15.2C 才激活真实 Handler。

## 2. 最终职责与数据流

```text
Go authority transaction
  -> authoritative row mutation + legacy compatibility job + knowledge_outbox

Python rag-worker (default dark-run)
  -> Redis wake OR 1s timer OR 30s forced rescan
  -> SECURITY DEFINER claim (short transaction)
  -> strict event plan outside transaction; no provider/object IO
  -> SECURITY DEFINER apply + ledger + ack (one transaction)

Job loop (handler allowlist only)
  -> claim with new lease token
  -> handler + 30s heartbeat outside transaction
  -> retry/fail/finish by token CAS
```

Outbox `id` 仅是分配顺序。Worker 不保存本地 Checkpoint，不以 `MAX(id)` 判定追平；
强制 Rescan 直接重新扫描 Postgres 的 Pending/Expired 可领取状态，Applied Ledger 在
Apply/Ack Transaction 中提供幂等冲突隔离，因此 Late-low-ID Commit 仍可见。

## 3. Migration `010` Contract Addendum

### 3.1 Governance Mapping 注入

Go Runner 新增可选 Mapping：

```bash
mm-chat-migrate up \
  --phase15-governance-map=/run/secrets/phase15-governance-map.json
```

文件不含 Key，只含：

```json
{
  "profiles": [
    {
      "profileId": "uuid",
      "modelId": "jina-embeddings-v4",
      "profileContractHash": "64-lowercase-hex"
    }
  ],
  "heads": [
    {
      "processor": "jina",
      "endpointId": "hosted-default",
      "modelId": "jina-embeddings-v4"
    }
  ]
}
```

Runner 校验 JSON Shape、UUID、非空规范化字符串、Hash、Duplicate 后，把 Canonical JSON
通过 transaction-local custom GUC 注入 `010`；SQL 不读文件。零 Profile/Head 的 Fresh
Database 允许空 Mapping。Published Database 的 Missing、Extra、Ambiguous、Hash
Mismatch 必须整笔失败。Mapping 不落业务表、不进入日志。

### 3.2 Schema 范围

`010` 一次建立 Phase 15.2A 已冻结的完整 Extension-independent 对象：

- Governance/Head/Consent/Job 的 Endpoint+Model+Contract Hash 兼容迁移；
- Outbox Lease、Applied-event Ledger、Replay Audit；
- Immutable Index Profile、Corpus Generation/Head/Projection State；
- Document Materialization/Head；
- Parser Artifact、Canonical Block、Parent/Child Chunk 与 Block Span；
- Durable Collection Purge Root/Item；
- Claim/Ack/Retry/Fail/Heartbeat/Replay、Publish/Purge/Promotion Function；
- `rag_projection_owner`、`go_api_runtime`、`rag_worker_executor`、
  `rag_replay_operator`、`rag_api_reader`、`go_evidence_hydrator` NOLOGIN
  Roles 与最小 GRANT。

禁止 `pg_search`、`vector/halfvec`、Dimension、Tokenizer 或 HNSW 进入 `010`。

### 3.3 Lease 与 DLQ

Outbox：

```text
max_attempts default 8
lock_owner + lock_token + lock_expires_at
error_code + failed_at
processing => locked_at/owner/token/expiry 全非空
pending/published/failed => lease 字段全空
```

Job 保留已有 `max_attempts`，增加不可复用 `lease_token`、Generation/Materialization、
Model/Contract Hash 与 `legacy_projection_unbound`。外部处理 Job Model 非空；Purge 的
Processor/Consent/Model 全空。Legacy Row 永不 Claim。

DLQ 是 Postgres 中的 `status=failed`，不是 Redis List/Stream。稳定 Error Code 不得
包含正文、URL Credential、Provider Response 或 SQL。

### 3.4 Function Signatures

Phase 15.2B Worker 只调用：

```text
knowledge_claim_outbox(consumer, worker_id, lock_token, lease_seconds)
knowledge_apply_and_ack_outbox(consumer, event_id, worker_id, lock_token,
                               scope_kind, index_generation_id,
                               action, result_hash)
knowledge_retry_outbox(event_id, worker_id, lock_token, error_code,
                       retry_after_seconds)
knowledge_fail_outbox(event_id, worker_id, lock_token, error_code)
knowledge_claim_processing_job(worker_id, lease_token, lease_seconds,
                               allowed_stages)
knowledge_heartbeat_processing_job(job_id, worker_id, lease_token,
                                   lease_seconds)
knowledge_finish_processing_job(job_id, worker_id, lease_token,
                                outcome, error_code, retry_after_seconds)
knowledge_replay_outbox(event_id, expected_error_code, operator_id, reason)
knowledge_replay_processing_job(job_id, expected_error_code,
                                successor_job_id, operator_id, reason)
knowledge_rag_worker_readiness()
```

Claim 每次只取一 Row，因此一个 `lock_token` 不会跨 Row 复用。所有时间以
`clock_timestamp()` 为准。Function 固定安全 `search_path`、撤销 `PUBLIC EXECUTE`，
Worker 无 Base Table DML；Replay 只 Grant 给 `rag_replay_operator`。

Outbox Apply 的 Global Scope 固定零 UUID；Generation Reconstruction 后续使用真实
Generation Scope。相同 Ledger PK/Hash 幂等；同 PK 不同 Hash 原子隔离，不能 Ack。

### 3.5 Down 与兼容桥

`010.down` 先检查：无 Processing Lease、无非 Legacy Job、无新 Profile/Head/Consent、
无 Endpoint/Model 无法压回 Processor-only 的冲突。失败则保留全部 `010` 对象，禁止
部分降级。Down 不删除 Document、Version、Consent History、Outbox 或 Source File，
不使用宽泛 `CASCADE`。

Down 只撤销 `010` 增加的 API 权限，必须保留 `go_api_runtime` 及其运行 `009` API
所需的 capability。回滚后的 API LOGIN 仍只继承 `go_api_runtime`；禁止为方便回滚而把
API capability grant 给 bootstrap/migrator LOGIN 或 `rag_projection_owner`，也禁止把
API LOGIN 加入 migrator/owner。

现有 Go Producer 在本阶段继续创建 Compatibility Job，但必须显式
`legacy_projection_unbound=true`。这保持当前 API/测试语义，又确保 Python 永不误 Claim。
Phase 15.2C 切换为 Dispatcher 创建 Generation-bound Execution Job 后，再由新 Migration
移除兼容写路径。

## 4. Python `rag-worker`

### 4.1 Package 与依赖

```text
mm-chat/rag/
├── pyproject.toml / uv.lock / Dockerfile / README.md / DESIGN.md
├── src/mm_chat_rag/
│   ├── settings.py / logging.py / metrics.py / health.py
│   ├── postgres.py / redis_wakeup.py / retry.py
│   ├── consumer.py / jobs.py / handlers.py / worker.py / replay.py
└── tests/unit + tests/integration
```

使用 Python 3.13、Async psycopg 3、`psycopg_pool`、Redis Async Client、Starlette/
Uvicorn、Prometheus Client；不用 ORM/Alembic。Pool 最大 2；连接固定
`application_name`、Statement/Lock/Idle-in-transaction Timeout。

### 4.2 Dark-run Gate

默认：

```text
RAG_WORKER_DISPATCH_ENABLED=false
RAG_WORKER_JOB_STAGES=""
```

关闭时 Worker 只验证 DB Function/Single-worker Lock、提供 Health/Metrics，不 Claim
Outbox/Job。测试可显式启用 Synthetic Handler；Production 在 `011`、首个 Generation、
真实 Event Registry 和 Promotion Gate 通过前不得启用。

### 4.3 Poll、Rescan、Wake

- Outbox Poll：1 秒；强制 Rescan：30 秒；Claim Lease：30 秒；
- Job Lease：90 秒；Heartbeat：30 秒；全局 Job 并发 1；
- Redis Channel：`${REDIS_KEY_PREFIX}:rag:outbox:v1`，Payload 固定 `"1"`；
- Redis 断线/Flush 只使 Wake Degraded，Postgres Poll 继续；
- SIGTERM 停止新 Claim，当前 Handler 在 Grace 内结束，否则等待 Lease 回收；
- Session Advisory Lock 保证单 Worker，第二实例明确退出。

### 4.4 Health 与 Metrics

```text
GET /health  进程/Event Loop 存活；Compose 只检查此路由
GET /ready   DB/Functions/Worker Lock/Consumer/Projection 分项
GET /metrics bounded-label Prometheus metrics
```

`011` 前 `/ready` 可合法返回 `projection=not_ready`；不得因此触发容器重启。Redis 仅显示
`degraded`。日志和 Metric 禁止 Query、Payload、正文、Token、API Key、Object Key 与
高基数用户/文档标签。

### 4.5 Replay CLI

CLI 默认 Dry-run；真正执行必须同时给精确 ID、Expected Error Code、Operator UUID 和
非空 Reason。Outbox 原 Event ID 不变；Job Replay 创建 Successor，不复活旧 Lease，也
不清除旧失败审计。

## 5. Compose 与 Credential 边界

新增 `rag-worker` Profile，不发布 Host Port：

```text
read_only=true; init=true; cap_drop=ALL; no-new-privileges
mem_limit=448m; pids_limit=64; tmpfs /tmp 64m
private/internal network; no MinIO credential; no Provider key
```

DB credential 分成四条互不复用的登录链：

| Route                     | LOGIN / capability                                             |
| ------------------------- | -------------------------------------------------------------- |
| `MIGRATION_DATABASE_URL`  | `POSTGRES_USER` bootstrap/migrator；独立必填，不回退到 API URL |
| `DATABASE_URL`            | 非 superuser、非 `CREATEROLE` API LOGIN → `go_api_runtime`     |
| `RAG_WORKER_DATABASE_URL` | Worker LOGIN → `rag_worker_executor`                           |
| `RAG_REPLAY_DATABASE_URL` | Replay LOGIN → `rag_replay_operator`                           |

四个 LOGIN 的用户名和密码必须两两不同。`admin` 与 Go API 共用
`DATABASE_URL`/`go_api_runtime`，不得获得 migrator credential；migrate 只接收
`MIGRATION_DATABASE_URL`，变量缺失必须失败，禁止 fallback 到 `DATABASE_URL`。

Fresh install 先以 `POSTGRES_USER` 启动数据库并执行 migration 到 `010`，由 `010`
建立 NOLOGIN capability roles；随后才以安全交互输入创建 API、Worker、Replay LOGIN，
分别 grant 上表唯一 capability。上线前必须在 live database 验证 LOGIN 属性、成员关系
与四路 credential 可连接性，不得把密码或完整 URL 打印到命令行、日志或工单。

Production Overlay 移除 `rag-worker.build`，要求独立 `RAG_IMAGE@sha256`。本阶段不把
Redis 设为硬 `depends_on`，Postgres 是硬依赖。Preflight 必须校验 URL 用户不同、Image
Digest、Secret File Mode，但不得打印 Credential。

## 6. 实施顺序与可勾选任务

### B0 — Contract/Plan

- [x] 完成三路 xhigh 现状侦察。
- [x] 冻结 Dark-run、Mapping、DLQ/Replay、Role 和 Function Addendum。

### B1 — Migration/Compatibility

- [x] 扩展 Go Migration Runner/CLI 注入 Governance Mapping。
- [x] 实现完整 `010` Up/Down、Roles、Functions、`009` API capability 保留与
      Producer Legacy Bridge。
- [x] 通过 Fresh/Published/Mapping/Down-Up/Constraint/Permission Gates。

### B2 — Python Worker Core

- [x] 创建锁版本 Python Package、Docker Image 与严格 Settings/Logging。
- [x] 实现 Postgres Function Adapter、Poll/Rescan、Single-worker Lock。
- [x] 实现 Job Heartbeat/Retry/DLQ、Redis Wake 与 Health/Metrics。
- [x] 实现默认 Dry-run、受限 Replay CLI。

### B3 — Compose/Operations

- [x] 接入四路 DB LOGIN、无 fallback migration、`rag-worker` Profile、
      资源/网络/只读 RootFS 与独立 Credential。
- [x] 更新 Preflight、Env Example、Deployment/Persistence Docs。

### B4 — Verification/Promotion

- [x] 通过 Duplicate、Out-of-order、Late-low-ID、Crash-before-Ack、
      Effect-commit/Ack-loss、Stale Token、Redis Loss、DLQ/Replay、Tombstone Race。
- [x] 通过 Go Race/Vet、Python Ruff/Mypy/Pytest/Coverage、pip-audit、Docker Build。
- [x] 独立 xhigh Review 最终 `P0/P1/P2 = 0/0/0`。

## 7. Definition of Done

Phase 15.2B 完成只表示 Durable Mechanics 可重放、可恢复、可审计，且 Worker 以最小
权限 Dark-run。它不表示文档已经解析、Embedding 已生成、Search Projection Ready、
Evidence API 可用或聊天已接 RAG。真实 Stage Activation 仍属于 Phase 15.2C。

回滚：停止 `rag-worker` Profile即可停止消费；Application Rollback 保留 `010` Schema
优先。只有满足 Down Precondition 才允许 `010.down`，任何不确定状态都保留新 Schema。
