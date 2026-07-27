# 五强 Memory Engine 固定源码审计

调研日期：2026-07-27。本文只回答“当前公开 artifact 实际执行什么”，不把 README、
厂商 benchmark 或 cloud 能力当作 self-hosted OSS 事实。Neo Chat 的生产约束是：
Server mode、自托管、Go 掌握 auth/user scope/CRUD/delete/API authority、PostgreSQL 为
唯一 durable authority、Python RAG 负责模型能力。

## 1. 审计范围与复核方式

| 候选 | 固定快照 | 版本/许可证 | 审计结论类型 |
| --- | --- | --- | --- |
| Hindsight | `e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a` | `0.8.5` / MIT | 核心 engine 可源码审计 |
| Supermemory | `fa7588c43e766c7d8a7735a89fb1d9cf4af7d210` | root 未声明版本 / MIT | 仓库可审计，但 Local Memory 核心缺失 |
| Mem0 | `b357a5a1b03c299ec8229c268e63cfac0f7c6566` | `2.0.14` / Apache-2.0 | SDK、server、存储可源码审计 |
| Graphiti | `9140123a7282d44efc077a0af09179919f3defdf` | `0.29.2` / Apache-2.0 | core 与示例 FastAPI 可源码审计 |
| Cognee | `325acf356a81545b9892f19ab1ea7b61c51a776b` | `1.4.0` / Apache-2.0 | API、pipeline、adapter 可源码审计 |

本次逐项检查六个轴：authority、user/tenant isolation、delete/forget、durable storage、
capture/recall、deployment/recovery。`通过` 只表示当前源码满足 Neo Chat 的必要条件；
效果仍必须用冻结的中文数据集 benchmark。

## 2. 最终矩阵

| 候选 | Authority | 隔离 | 删除 | 存储/一致性 | Recall | 部署 | 结论 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Hindsight** | 条件通过 | 条件通过 | 条件通过 | 条件通过 | **最完整** | 条件通过 | **可审计现成引擎首选**，只做独立 DB shadow |
| **Mem0** | 不通过 | 不通过 | 不通过 | 不通过 | 有限 | 条件通过 | 成熟算法对照，不得接管多用户 authority |
| **Graphiti** | 不通过 | 不通过 | 不通过 | 不通过 | 关系/时间专项强 | 不通过 | 只在多跳关系需求出现后隔离 PoC |
| **Cognee** | 条件通过 | 不通过 | 不通过 | 不通过 | 中文 lexical 不合格 | 不通过 | 平台过重，当前不再列第二 shadow 候选 |
| **Supermemory Local** | 文档不通过 | 不可源码验证 | 不可源码验证 | 不可源码验证 | 不可源码验证 | 不通过 | **binary black-box**，不与开源核心同档 |

这张表推出三个不同结论，不能混称“冠军”：

1. **Neo Chat 最佳生产适配**：原生 PostgreSQL Memory v2。
2. **最强可审计现成引擎**：Hindsight，且必须置于 Go authority 后做 shadow。
3. **中文真实效果冠军**：尚未跑 Neo Chat frozen benchmark，当前不能宣称已确定。

## 3. Hindsight 0.8.5

### 3.1 Authority 与隔离

- 核心数据以 `bank_id` 分区，支持 API key、`TenantExtension` 和 operation validator；
  这比其余四项更接近可代理的多租户 engine。
- 但 Go 仍必须是唯一外部 authority：browser/client 不能提交 `bank_id`，由 Go 把
  authenticated user 映射成不可猜测的稳定 bank ID，Hindsight 只暴露在 Compose
  私网。
- 单条 `delete_memory_unit(s)` 只做 tenant authentication 后按 UUID 查询/删除，没有
  像 `delete_bank()` 一样调用带 bank scope 的 operation validator。当前 HTTP/MCP
  未暴露该内部方法，因此不是现成远程越权路径；它仍是 adapter/扩展调用时必须封死
  的内部边界。

证据：

- `hindsight-api-slim/hindsight_api/engine/memory_engine.py:6193-6273` — 单条删除只按
  UUID，未校验目标 bank。
- `hindsight-api-slim/hindsight_api/engine/memory_engine.py:6500-6667` — bank delete
  调用 operation validator，并在事务中清理主要 bank 数据。

### 3.2 删除语义

`delete_bank()` 对 `documents`、`memory_units`、`invalidated_memory_units`、`entities`
及其 cascade 关系处理较完整，但不是“所有可能含用户内容的数据均已擦除”：

- `audit_log` 与 `llm_requests` 没有指向 `banks` 的 cascade FK；它们可保存 request、
  response 或 LLM input/output，依赖独立 retention/治理。
- `graph_maintenance_queue` 以 `bank_id` 记录任务，但核心 bank delete 未显式清理。
- native `file_storage` 只用 storage key，没有 `bank_id`/FK。默认
  `file_delete_after_retain=true`，但解析或任务失败仍可能留下源文件 bytes。
- extension 可通过 `extra_bank_tables()` 补充删除表；默认核心并未自动把上述所有表
  纳入同一 privacy erase contract。

因此 Hindsight 可以做 derived shadow，但晋升前必须补齐并实测 `memory delete →
conversation delete → account delete → audit/file/queue sweep`。

证据：

- `hindsight-api-slim/hindsight_api/alembic/versions/c2d3e4f5g6h7_add_audit_log_table.py:33-54`
- `hindsight-api-slim/hindsight_api/alembic/versions/d3e4f5a6b7c8_add_llm_requests_table.py:36-78`
- `hindsight-api-slim/hindsight_api/alembic/versions/b5a4c3e2f1d8_add_graph_maintenance_queue.py:38-54`
- `hindsight-api-slim/hindsight_api/alembic/versions/a1b2c3d4e5f6_add_file_storage_table.py:33-53`
- `hindsight-api-slim/hindsight_api/config.py:1043-1055`

### 3.3 存储、Recall 与 durability

- Memory 主链使用 PostgreSQL：memory units、entities、links、BM25/vector、temporal、
  async operations 均可在 PG 内完成；不强制新增 Graph DB 或 Vector DB。
- `retain/recall/reflect`、semantic + BM25 + graph + temporal、RRF、cross-encoder
  rerank、token trimming 是五项中最接近 Neo Chat 目标 v2 的完整链路。
- async operation 有 durable PostgreSQL 状态和 worker/retry，比 Mem0/Graphiti/Cognee
  的进程内 background task 更适合作为可重放 shadow。
- 中文效果仍未验证；PostgreSQL lexical backend 与 BGE-M3/reranker 的组合必须在
  同一中文 benchmark 上实测，不能从英文 LongMemEval 推导。

### 3.4 部署阻断项

- startup migration 默认开启，默认 pool 为 `5..100`，单机共存时资源上限过高。
- pgvector 若已安装但不在 `public` schema，migration 会尝试执行
  `DROP EXTENSION vector CASCADE; CREATE EXTENSION vector;`。这可能破坏共享 database
  中 Neo Chat 对 vector extension 的依赖。
- 因而不能再采用“同一 database、不同 schema”作为隔离。正确 shadow 边界是：
  **同 PostgreSQL cluster 中独立 database + 独立 role**，禁止 Hindsight role 连接或
  migrate Neo Chat database；再单独限制 pool/CPU/RSS/WAL。

证据：

- `hindsight-api-slim/hindsight_api/config.py:1109-1129`
- `hindsight-api-slim/hindsight_api/migrations.py:69-130`
- `hindsight-api-slim/hindsight_api/alembic/env.py:119-136`
- `hindsight-api-slim/hindsight_api/engine/storage/base.py:45-53`
- `hindsight-api-slim/hindsight_api/engine/storage/postgresql.py:103-118`

**裁决**：Hindsight 是本轮唯一值得进入 isolated shadow benchmark 的现成引擎，
不是默认生产 authority。

## 4. Mem0 2.0.14

### 4.1 Auth 与 Memory scope 脱节

Server 能校验 JWT、API key 和 admin role，但得到的 `_auth` 没有绑定 Memory scope：

- create 可提交任意 `user_id/agent_id/run_id`；
- list/search 可读取任意调用方传入的 scope；
- get/update/history/delete by ID 不校验 owner；
- `/entities` 对任意已认证调用者枚举所有 Memory entity；
- 只有 delete-all/reset 需要 admin。

此外 register 只允许创建首个 admin，当前并不是一个把 app user 与 Memory owner
贯通的多用户账号系统。把它直接暴露为 Neo Chat Memory Server 会产生水平越权；
藏在 Go 私网后可减少攻击面，但不能让它拥有 authority。

证据：

- `server/auth.py:144-220`
- `server/main.py:366-381,410-523`
- `server/routers/entities.py:43-75`
- `server/routers/auth.py:94-122`
- `server/models.py:18-40`

### 4.2 Capture 已是 ADD-only

当前 V3 pipeline 与旧 docstring 不同：system prompt 明确“sole operation is ADD”；
existing memories 只用于 dedup/link；落库前仅做正文 MD5 exact dedup。偏好变化或事实
更正会新增关联 Memory，不会执行 `SUPERSEDE/UPDATE/DELETE`。同时 prompt 明确从
assistant 消息抽取推荐、计划、研究结果和解决方案，可能把 assistant 幻觉或临时
建议长期化。

证据：

- `mem0/configs/prompts.py:463-578,1016-1062`
- `mem0/memory/main.py:886-1054`
- `mem0/memory/utils.py:61-76`

### 4.3 删除与一致性不满足 privacy contract

- 向量正文由 PostgreSQL/其他 Vector Store 持久化；history 与最近 10 条 raw
  messages 在独立 SQLite。
- vector insert/delete 先提交，history 后写，二者不在同一事务。
- delete/delete-all 删除 vector 后，会向 SQLite history 写入仍含旧正文的 DELETE
  记录；session raw messages 也不会随 Memory 删除。
- 这些 raw messages 仍可通过 `last_k_messages` 进入之后的 extraction。
- 没有 tombstone/generation fence；认证 user 表与 vector payload 没有 FK/account
  deletion route。

证据：

- `mem0/memory/main.py:1015-1054,1839-1915,1988-2077`
- `mem0/memory/storage.py:102-324`
- `server/main.py:505-523`

### 4.4 Recall 与部署边界

- 先做 semantic candidate pool；BM25/entity score 只给这些 semantic candidates
  加权，keyword-only 命中不能救回没进入 semantic pool 的记录。
- reranker 默认关闭，必须每次显式 `rerank=true`；`reference_date` 标注为
  platform-only，OSS 直接拒绝；expiration 只是过滤，不是 temporal reasoning。
- Server 默认 OpenAI LLM/embedding；bundle 的 LLM 只有 OpenAI/Anthropic/Gemini，
  embedding 只有 OpenAI/Gemini。接 SiliconFlow/BGE-M3 需要扩 provider 并重建镜像。
- auth DB 默认 `mem0_app`，Memory vector 默认在 `postgres`，再加 SQLite history，
  形成三份备份/恢复面；dev Compose 还会启动时强制重装 `mem0ai`，可与固定 checkout
  漂移。

证据：

- `mem0/memory/main.py:1349-1492,1598-1783`
- `server/main.py:61-62,107-138`
- `server/requirements.txt`
- `server/init-db.sh:4-8`
- `server/docker-compose.yaml:14-29`
- `mem0/vector_stores/pgvector.py:142-210`

**裁决**：Mem0 可作为成熟 SDK/算法行为对照；当前 self-host server 的多用户隔离、
冲突处理和可证明删除均不满足 Neo Chat production gate。

## 5. Graphiti 0.29.2

### 5.1 Authority、隔离与删除

- 官方示例 FastAPI 没有 auth middleware；`group_id` 由客户端自由提交，所有 ingest、
  retrieve、delete route 共享同一 Graphiti driver。
- `/group/{group_id}` 只删除 `EntityEdge`、`EntityNode`、`EpisodicNode`，漏掉
  `CommunityNode` 与 `SagaNode`。
- 通用 `clear_data(group_ids=...)` 的 label allowlist 包含 Entity/Episodic/Community，
  仍漏 Saga。
- ingestion 使用进程内 `asyncio.Queue`；重启时丢未处理 job，停止时直接 drain；没有
  durable tombstone/generation fence。删除与旧 ingest 并发时，排队 job 可把已删
  group 重新写回来。
- Neptune 路径会把 Community 同步进 AOSS；community/group/node delete 没有对应
  document delete，可能在 AOSS 留下可搜索残影。

证据：

- `server/graph_service/main.py:20-24`
- `server/graph_service/zep_graphiti.py:46-65`
- `server/graph_service/routers/ingest.py:13-70,93-109`
- `graphiti_core/utils/maintenance/graph_data_operations.py:34-64`
- `graphiti_core/nodes.py:867-1024`
- `graphiti_core/graphiti.py:737-779`
- `graphiti_core/driver/neptune/operations/community_node_ops.py:43-67`
- `graphiti_core/driver/neptune_driver.py:310-364`

### 5.2 Recall 与部署

Graphiti 的 entity/fact/episode、validity window、graph traversal 与 provenance 是其
真实优势，适合关系密集、时间线和多跳 slice。但它要求 Neo4j/FalkorDB/Neptune 等
图基础设施，自身不提供 Neo Chat 所需的 user authority、durable ingestion、完整
erase contract 和 Go CRUD 兼容层。

**裁决**：不进入首轮通用 Memory shadow；只有 benchmark 证明 flat temporal model
无法解决多跳需求时，才以独立图数据库做专项 PoC。

## 6. Cognee 1.4.0

### 6.1 “单 PostgreSQL 原子 hybrid”当前未接通

仓库确实包含声称能在单事务写 graph + vector 的 `PostgresHybridAdapter`，但
`get_unified_engine()` 中创建 `pghybrid` adapter 的代码整段被注释。当前配置
graph=`postgres`、vector=`pgvector` 时仍会走 separate engines，并用
`asyncio.gather` 分别提交。任一侧失败都可能留下 graph/vector 半成功；不能拿死代码
中的 transaction 当生产保证。

证据：

- `cognee/infrastructure/databases/unified/get_unified_engine.py:9-24,64-85`
- `cognee/tasks/storage/add_data_points.py:188-239`
- `cognee/infrastructure/databases/hybrid/postgres/adapter.py:273-393`

### 6.2 Background 非 durable，隔离模型过重

- background pipeline 仅用 `asyncio.create_task` 和进程内 strong-ref set；重启不会
  replay。
- startup recovery 默认等 1 小时后把 stale cognify run rollback/reset，不是从 durable
  queue 继续执行。
- access-control 模式会为 dataset 创建独立 PostgreSQL database，并把 graph/vector
  connection 通过 ContextVar 切换。对 Neo Chat 的“单 PG authority”会放大 database、
  migration、backup、connection 和 role 管理面。

证据：

- `cognee/modules/pipelines/layers/pipeline_execution_mode.py:17-21,54-127`
- `cognee/modules/cognify/recovery.py:17-54,90-128`
- `cognee/context_global_variables.py:160-181,191-241`
- `cognee/infrastructure/databases/postgres/admin.py:60-94`

### 6.3 Forget 存在跨用户 cache wipe

任意认证用户可调用 `forget(everything=true)`。relational/graph/vector 删除虽然以当前
user 的 datasets 为目标，随后却执行全局 `cache_engine.prune()`：

- Redis backend 调 `FLUSHDB`；
- filesystem backend 调 `diskcache.clear()`；
- SQL backend 不带 user filter 删除全部 Cognee cache tables。

共享部署中，单个用户可清掉其他用户 session/cache，属于 production blocker。

证据：

- `cognee/api/v1/forget/routers/get_forget_router.py:56-109`
- `cognee/api/v1/forget/forget.py:159-189`
- `cognee/infrastructure/databases/cache/redis/RedisAdapter.py:654-668`
- `cognee/infrastructure/databases/cache/fscache/FsCacheAdapter.py:524-536`
- `cognee/infrastructure/databases/cache/sql/SqlCacheAdapter.py:1071-1090`

### 6.4 中文 lexical

BM25 tokenizer 只是 `re.findall(r"\w+", text.lower())` 后过滤 stop words，没有 jieba、
CJK bigram 或中文 analyzer。连续中文句子通常成为一个大 token，不能把英文 demo 的
lexical 表现推定到 Neo Chat 中文数据。

证据：

- `cognee/modules/retrieval/lexical_retriever.py:15-23`
- `cognee/modules/retrieval/bm25_retriever.py:54-59`
- `cognee/modules/retrieval/utils/stop_words.py`

**裁决**：Cognee 是完整 knowledge platform，不是当前 Neo Chat 的轻量第二候选；
非原子 projection、非 durable job、跨用户 cache prune 和 per-dataset DB 均阻断生产。

## 7. Supermemory Local

### 7.1 固定仓库没有可审计的 Local Memory 核心

root 仓库使用 MIT LICENSE，包含 web、docs、SDK、MCP 和 integrations，`package.json`
则标记 `"private": true`。对全部 tracked files 检查后，没有 `supermemory-server` 的
核心源码、Cargo/Go manifest 或可从此仓库构建该 binary 的 manifest；该名称只出现
在 README/self-host docs。

Local 安装流程是 `curl ... | bash` 或 `npx supermemory local` 下载匹配平台的预编译
binary。因而可以确认文档与 SDK 是公开的，但**当前仓库的 MIT 许可证不能单独证明
预编译 Memory 核心已在此仓库以可审计源码交付**。

证据：

- `package.json:1-21`
- `README.md:336-365`
- `apps/docs/self-hosting/overview.mdx:8-31`
- `apps/docs/self-hosting/quickstart.mdx:8-50`

### 7.2 只能确认的运行边界

文档明确 Local 是一个 machine/process、一个 org、一个自动生成 API key、embedded
graph、默认英文 `Xenova/bge-base-en-v1.5`；Enterprise 才有多成员/角色/scoped key、
dashboard 和 proprietary extraction models。无法从当前仓库复核 Local 的 owner
binding、delete propagation、事务、crash recovery、graph cleanup 或真正 recall
实现，也不能把 Enterprise benchmark 外推给 Local BYO-model。

证据：

- `apps/docs/self-hosting/local-vs-enterprise.mdx:8-43`
- `apps/docs/self-hosting/overview.mdx:24-35`

**裁决**：标记为 `binary black-box / 文档级候选`。除非上游提供对应核心源码或可
重现 build provenance，否则不进入“最强可审计 self-host engine”比较。

## 8. 对 Neo Chat 的定案

固定源码审计没有推翻原生路线，反而把边界钉死：

```text
Go auth / user scope / CRUD / delete / external API authority
  -> MemoryEngine contract
  -> PostgreSQL durable outbox + idempotent worker
  -> native L0/L1/L2/L3 projection
  -> exact + Chinese BM25 + BGE-M3 pgvector
  -> RRF + BGE reranker
  -> lower-priority untrusted memory block

optional benchmark only
  -> Hindsight private adapter
  -> dedicated PostgreSQL database + dedicated role
  -> shadow diagnostics, initially 0% prompt injection
```

最终候选顺序：

1. **原生 PostgreSQL Memory v2** — 生产首选。
2. **Hindsight** — 现成 shadow 首选；独立 database/role。
3. **Mem0** — 成熟行为基线，隔离、删除、冲突处理不合格。
4. **Graphiti** — 关系/时间专项 PoC。
5. **Cognee** — 平台过重且有共享 cache 删除阻断项。
6. **Supermemory Local** — 核心 binary black-box，不作为可审计首选。

未经 Neo Chat 中文 benchmark，不宣称任何候选已取得中文效果冠军；未经魔尊确认，
不进入 PoC、implementation 或 Live 数据变更。
