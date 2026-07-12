# Phase 15.2 单服务器 Python RAG 消费与索引设计

- 状态：Owner scope decisions locked；technical promotion gates open；implementation pending
- 日期：2026-07-12
- 前置条件：Phase 15.1D Go/Postgres Knowledge Control Plane 已完成至 migration `009`
- 适用规模：`≤10` 用户、`≤3` 并发、`≤500` 文件、`≤1GB` 原文

> 本文是当前单服务器实现的唯一执行方案。它收敛了早期 Qdrant-first 候选：第一版不部署 Qdrant、OpenSearch 或 Kubernetes，而在同一个 PostgreSQL 中使用 pgvector、真 BM25 和 Exact Lane。旧文档保留为 Accuracy Research，不得覆盖本文已锁定的部署边界。

## 1. 已锁定的产品决策

| 主题     | 决策                                                                                                |
| -------- | --------------------------------------------------------------------------------------------------- |
| 部署     | 一台 VPS、Docker Compose；不做多服务器/K8s                                                          |
| 前端     | 保持现有 Next.js/React UI 与 Knowledge Attachment 交互，最小改动                                    |
| 权威边界 | Go 负责 Identity、ACL、Consent、Chat、Citation 和 SSE；Python 只负责 RAG 计算                       |
| 数据     | Postgres 为总账本；Redis 仅加速；MinIO 保存原件和解析产物                                           |
| 外部计算 | Hosted MinerU 解析复杂/扫描 PDF；Hosted Jina 负责 Embedding/Rerank                                  |
| 密钥     | MinerU/Jina 使用管理员服务器级 Key；Chat Provider 继续支持用户 BYOK                                 |
| 检索     | pgvector Dense + 真 BM25 + Exact + RRF + Jina Multilingual Reranker                                 |
| 文档     | PDF、DOCX、PPTX、XLSX、CSV、Markdown、TXT、HTML；中文为主，兼容中英混合                             |
| 图片     | 第一版拒绝独立 PNG/JPEG/WebP 知识文档；支持扫描 PDF OCR                                             |
| 限制     | 单文件 `≤50MB`，PDF `≤500页`，全库 Active Child Chunk 起始上限 `100,000`                            |
| 分块     | Parent `1400–1600` Token；Child `350–500` Token；Overlap `60–100` Token                             |
| 聊天     | 普通聊天不检索；用户显式选择知识库后才执行 strict grounded RAG                                      |
| 引用     | 强制可点击 Source Citation；证据不足时拒绝猜测                                                      |
| 删除     | 立即逻辑不可见；索引/缓存/Artifact 目标 `15分钟内`清理；Source File 遵循 1.1 的绑定规则             |
| 性能     | 检索 `p95≤2s`；RAG 首 Token `p95≤5s`；查询并发 3；索引并发 1                                        |
| 备份     | 本地短期副本 + restic 客户端加密上传 Cloudflare R2；每月 Restore Drill                              |
| 评测     | 100 题 Relevance Golden Set：80 Development/Validation + 20 Frozen Holdout；安全/格式另有独立测试集 |

### 1.1 Owner 决策记录与未决项

下表记录 2026-07-12 交互式 Grill 中逐项确认的范围；它只锁定产品范围，不表示
旧 Profile 的所有 Promotion 条件已经关闭。

| 决策       | Owner 回答                                                                         |
| ---------- | ---------------------------------------------------------------------------------- |
| 检索主链   | 确认 Dense + 真 BM25 + Exact + RRF + Jina Reranker                                 |
| 文件与语言 | 确认全部列出的文档格式；中文为主、兼容中英；独立图片暂不支持，扫描 PDF 支持        |
| 容量/性能  | 确认 50MB、500 页、3 查询并发、1 索引并发及 §1 SLO                                 |
| 外部处理   | 确认 Collection 一次授权；Knowledge 使用管理员 MinerU/Jina Key；Chat 使用用户 BYOK |
| Chunk      | 确认 Parent/Child 与 60–100 Token Overlap                                          |
| 删除/引用  | 确认立即不可检索、异步清理和强制可点击引用                                         |
| 备份       | 确认使用 Cloudflare R2 保存 restic 加密备份                                        |
| 评测       | 确认先建立 100 题人工核验 Relevance Set                                            |

Source File 当前 Contract 是“Document 删除后仍保留，只有显式 File 删除才删除
Object”。Phase 15.2 保持该规则，除非后续 Owner 明确批准并实现“无其他 Live
Binding 时自动请求 File 删除”的新 Contract。15 分钟目标当前只覆盖 Projection、
Cache 和 Derived Artifact；显式 File 删除提交后再对 Source Object 使用同一目标。

仍未锁定：MinerU/Jina 账号额度、精确 Model/API Version、BM25 Extension 晋升、
Embedding Dimension、AGPL 审批和实际月度费用。这些保持为 Bake-off/Deployment
Gate，不得因本节状态写成已确认。

## 2. 当前基线与边界

当前数据库已有：

- `knowledge_collections`、`knowledge_documents`、`knowledge_document_versions`；
- `processor_governance_profiles/heads`、`processing_consents`、`user_query_consent_state`；
- `knowledge_processing_jobs`，支持 `parse`、`passage_embedding`、`purge`；
- `knowledge_outbox`，以唯一 `event_id` 作为 Replay Identity；
- Collection、Document、File、Consent、Governance 的 Revision/Visibility Fence。

当前尚无 Python 服务、Outbox Consumer、Canonical Block、Chunk、Embedding、BM25、pgvector、Index Generation、Projection Checkpoint、Evidence API 或 R2 备份。设计或测试中出现上述能力，不得描述为“已经实现”。

Postgres 是唯一授权源。Python、Redis、MinIO、BM25 Index、Vector Index 和浏览器状态均不能授予权限。`knowledge_outbox.id` 是分配顺序，不是事务提交顺序；Consumer 不得把 `MAX(id)` 当成不会漏事件的游标。

## 3. 目标拓扑

```text
Browser
  -> Reverse Proxy / Next.js
       -> Go API
            |- Postgres 16 + pgvector + BM25 extension
            |- Redis (wake-up/cache/nonce only)
            |- MinIO (original/parser/canonical artifacts)
            |- private rag-api (query concurrency ≤3)
            |    |- Jina query embedding + reranker
            |    `- Postgres hybrid retrieval
            `- private rag-worker (index concurrency =1)
                 |- Postgres outbox/jobs/projection
                 |- MinIO source/artifacts
                 |- MinerU parse
                 `- Jina passage embedding

Ops profile -> pg_dump + MinIO manifest -> restic encryption -> Cloudflare R2
```

`rag-api` 与 `rag-worker` 使用同一固定 Python Image、不同启动命令。两者只在 Compose Private Network 暴露；浏览器不得直连 Python、Postgres、Redis 或 MinIO。Python 不持有 MinIO Root Credential。

最小权限必须可由 Migration/Compose 自动验证：

- Migration Owner 独占 DDL、Extension 和 GRANT；运行时不得持有该凭证；
- `rag_api_reader` 只能执行参数化检索 View/Function，不得读取 Credential、原始
  Object Key、ACL 以外的 Identity 字段或更新任何表；
- `rag_worker_executor` 只能 Claim Job、调用受限 CAS/Publish/Purge Function，及
  CRUD Staging/Artifact/Chunk 表；不得直接更新 ACL、Consent、Document Pointer、
  Active Generation Pointer 或 Outbox 终态；
- Source 读取必须走 Go 私有 Object Gateway，使用 Job/Version/File/Hash-bound 短时
  Capability；禁止给 Worker 全 Source Prefix 的静态 `GetObject` 权限；
- Artifact 写入使用逐 Generation 临时 STS Session Policy；不支持 STS 时同样走
  Gateway。Purge 由 Go 对明确的 Retired Generation/Object Manifest 单独签发删除
  Capability，静态 MinIO Policy 不承担“当前 Generation”动态授权；
- `rag-api` 不持有任何 Source/Artifact Bucket Credential；
- GRANT 与 S3 Policy 必须有负向集成测试，证明 Python 凭证不能越权读写。

## 4. 服务职责与信任边界

### 4.1 Go API

- 认证 Session，计算 Personal/Team Allowed Collections；
- 拒绝客户端提交 `user_id`、`team_id`、ACL、Revision 或 Impersonation Hint；
- 生成短时、单次、Body-bound 的 Go → RAG Workload Token；
- 在 Python 返回候选后逐条重新授权 Source Version；
- 生成 Citation Capability，并把 Evidence 交给用户 BYOK Chat Provider；
- 普通聊天绕过 RAG；Knowledge Attachment 非空时进入 Grounded Flow。

### 4.2 Python `rag-api`

- 校验 Audience、Method、Path、Body Hash、`iat/exp/jti` 和签名 `kid`；
- 最多接受 3 个并发查询，超出有界队列后返回 `429/503 + Retry-After`；
- 执行 Dense、BM25、Exact、RRF、Rerank 和 Parent/Window Expansion；
- 只返回 Source Identity、Span、Score、Profile 和 Degraded State；
- 不签发 Citation，不缓存授权结论，不调用用户 Chat Provider。

### 4.3 Python `rag-worker`

- 轮询 Postgres；Redis Publish 只能提前唤醒，Redis 丢失不得丢 Job；
- Claim、Heartbeat、Retry、Parse、Chunk、Embed、Build、Verify、Publish、Purge；
- 开工前和提交前均重新校验 Job Snapshot、Consent、Governance 与删除状态；
- 通过受限 Stored Procedure/CAS 完成状态转换，禁止直接改写 ACL 权威行。

## 5. 数据模型增量

所有 DDL 继续由 Go Migration Runner 管理；Python 启动时禁止建表或改表。下一迁移至少需要以下逻辑对象，最终名称在 Contract 阶段冻结：

| 对象                              | 用途与关键约束                                                                        |
| --------------------------------- | ------------------------------------------------------------------------------------- |
| `knowledge_index_profiles`        | 不可变 Parser/Chunk/Embedding/BM25/Rerank 配置与 Hash                                 |
| `knowledge_index_generations`     | `building/verified/active/retired/failed`；一个 Profile 一个 Active Pointer           |
| `knowledge_projection_state`      | Generation、Projection Revision、Manifest Hash、Readiness                             |
| `knowledge_outbox_applied_events` | `(consumer_name,event_id,generation_scope_id)` 唯一，Scope 强制非空，记录 Result Hash |
| `knowledge_parser_artifacts`      | Parser-native、Canonical IR、Quality Report 的 MinIO Key 与 Hash                      |
| `knowledge_blocks`                | Canonical Block、结构、Locator、Asset Ref、Provenance、Confidence                     |
| `knowledge_parent_chunks`         | 可返回给模型的完整章节/窗口                                                           |
| `knowledge_child_chunks`          | 搜索文本、Exact Terms、BM25 字段、Embedding 与全部 ACL Fence                          |

`knowledge_processing_jobs` 需增加不可复用的 `lease_token` 和目标 `index_generation_id`。所有完成更新必须匹配 `id + status=processing + lease_owner + lease_token`；过期 Worker 即使晚返回也不能覆盖新 Worker。

Applied Ledger 的 `generation_scope_id` 不允许 NULL。全局事件使用固定、受约束的
Global Scope UUID；逐 Generation 事件使用真实 Generation ID，避免 PostgreSQL
普通 UNIQUE 对多个 NULL 不去重。

Block Locator 使用 Discriminated Union：

```text
text_offset | line_range | page_bbox | slide_shape | sheet_cell | ooxml_part_xpath
```

Chunk 的确定性唯一键至少包含：

```text
generation_id + document_version_id + source_span_hash + chunk_profile_hash
```

任何 Model、Dimension、Tokenizer、Chunk Policy 或 Parser Major Version 变化，都创建新 Generation，禁止原地混写。

## 6. Outbox Consumer、Lease 与 Replay

### 6.1 分工

- `knowledge_outbox`：事实事件、Replay 和 Projection Reconstruction；
- `knowledge_processing_jobs`：长任务状态、Lease、Attempt 和终态；
- Applied Event Ledger：表达每个 Consumer/Generation 是否已应用事件；
- Redis：只发 Wake-up，不保存权威队列或 Checkpoint。

### 6.2 消费算法

1. 每秒 Poll；每 30 秒执行一次不依赖 Redis 的强制 Rescan。
2. `knowledge_outbox` 增加 `lock_owner + lock_token + lock_expires_at`。Claim 使用
   `FOR UPDATE SKIP LOCKED` 写入三字段后立即提交短事务；进程崩溃后按 Expiry 回收。
3. 按 `event_id` 幂等；Checkpoint 以下也必须 Anti-join Applied Ledger 回扫。
4. 同一 Aggregate 使用单调 Revision；缺 Revision 时回读 Postgres 当前权威状态，不猜测事件顺序。
5. Dispatcher 只校验/创建 Job 或 Tombstone Work，不在 Outbox Lock 内调用外部 API。
6. 长任务 Lease 起始值 90 秒、Heartbeat 30 秒；配置值必须覆盖 Provider Deadline。
7. Dispatcher 在同一事务中幂等创建/确认 Job、写 Applied Ledger，并用
   `event_id + lock_owner + lock_token + status=processing` CAS Ack 为 Published；
   外部 API 只由 Job 执行，不进入该事务。
8. Effect 已存在但 Ack 未提交时，重放只确认相同 Result Hash；Hash 不同立即隔离。

必须正确处理：重复事件、低 ID 晚提交、高 ID 先提交、进程在外部调用后崩溃、DB Effect 已提交但 Ack 未提交、Redis Flush、Lease 过期和 Tombstone 先于旧 Upsert 到达。旧事件永远不能复活更高 Visibility Epoch 的文档。

### 6.3 已有事件的处理

| Event                                    | Consumer 行为                                               |
| ---------------------------------------- | ----------------------------------------------------------- |
| `knowledge.document.version.requested`   | Claim 已存在 Parse Job，构建目标 Generation                 |
| `knowledge.document.reprocess.requested` | 同一 Version 生成新 Processing Generation                   |
| `knowledge.document.tombstoned`          | 立即禁用并执行 Version Purge Job                            |
| `knowledge.collection.tombstoned`        | 创建/恢复 Durable Fan-out，按 Version/Generation 分页 Purge |
| `knowledge.processing.cancelled`         | 取消尚未发布的工作，提交前 Fence 必须失败                   |
| Collection/Query Consent changed         | 停止新 Egress、清授权缓存；按策略清派生产物                 |
| Governance Head changed                  | 旧 Profile Job 禁止继续调用外部 Processor                   |
| `team.membership.changed`                | 失效 Authorization Cache，不要求全量 Re-embed               |
| `file.object.delete.requested`           | 对象删除链；不得代替索引 Tombstone                          |

Collection Purge 使用持久 `knowledge_collection_purge_items`（最终名待 Migration
冻结），唯一键至少为 `(collection_id, collection_visibility_epoch,
document_version_id, generation_id)`，记录 Cursor、Attempt、Lease、终态与 Error
Code。Fan-out 可分页重启、重复执行和统计 Remaining Count；全部 Item 成功后才能把
Collection Purge 标为 Complete。Derived Artifact 的 15 分钟 SLO 从 Tombstone
Commit 计时并告警；未完成不影响立即逻辑不可见。

## 7. 索引流水线

```text
Admit -> Claim -> Reauthorize -> Fetch -> Parse -> Quality Gate
      -> Canonical IR -> Parent/Child Chunk -> Embed
      -> Stage Projection -> Verify -> Atomic Publish -> Cleanup Old Generation
```

Atomic Publish 只能调用受限 Stored Procedure。固定锁序为 Collection → Document →
Old/New Version → Generation → Job；随后校验 Lease Token、旧 `current_version_id`、
全部 ACL/Visibility/Processing/Consent/Governance Snapshot、Manifest/Artifact Hash
和 Verified 状态。在一个 Postgres Transaction 中切换 Version Status、Document
`current_version_id/status`、Active Generation Pointer、Projection Revision 和
Job 终态，并写 Outbox。任一 CAS 失败整笔回滚；新 Staging 永远不能部分可见。

### 7.1 输入与 Parser 路由

| 输入          | 主路线                                    | 回退/说明                        |
| ------------- | ----------------------------------------- | -------------------------------- |
| TXT/Markdown  | encoding detection + deterministic parser | 保留行号和 Offset                |
| HTML          | hardened DOM parser                       | 禁止外部资源和脚本执行           |
| DOCX/PPTX     | native OOXML parser                       | 复杂页面可渲染后送 MinerU        |
| XLSX/CSV      | openpyxl/pyarrow                          | 保留 Sheet、Cell、公式与值       |
| 普通 PDF      | PyMuPDF page preflight/native text        | 保留 Page、Block、BBox、字体     |
| 扫描/复杂 PDF | MinerU Hosted                             | 页面级 OCR、表格、公式、阅读顺序 |

独立图片 MIME 第一版在 Admission 阶段拒绝。扫描 PDF 仍保存 Page/BBox 和必要的内部 Page Asset，用于 OCR 与引用定位，但不建立 Image Embedding 或以图搜图。

原文件最大 50MB、PDF 最大 500 页。Parser 在 Rootless Sandbox 中运行，限制 CPU、内存、临时目录、总时长、压缩展开比和嵌套深度；禁止宏、DTD、XXE、网络访问和任意公式/代码执行。

### 7.2 Canonical IR 与质量门

权威解析结果不是 Markdown，而是版本化 Canonical Block。必须保存：

```text
document/version/block identity, ordinal, block_type, parent_id
heading_path, text/markdown/html/latex/code, locator, reading_order
table grid/span, parser/model/config hash, confidence, content hash
derived, non_indexable, needs_review
```

质量门覆盖页数一致性、非空页面覆盖率、替换字符比例、OCR Confidence、阅读顺序、表格结构与 Source Hash。失败页单独重试；仍失败则隔离，禁止把空文本标记成功。Original、Parser-native ZIP/JSON、Canonical IR、Chunk 和 Search Projection 分开保存。

### 7.3 Parent/Child/Overlap

- 正文 Parent：按 Section 对齐，目标 `1400–1600`，硬上限 `2000` Token；
- Child：目标 `350–500`，硬上限 `650` Token；
- 相邻 Child Overlap：`60–100` Token；
- 不跨 Heading、Table、Code Block 或 Sheet Boundary 强行重叠；
- Table、Code、List、FAQ 使用独立 Policy；
- 检索 Child，最终扩展 Parent 或相邻窗口；扩展 Span 仍需重新授权。

全库起始硬上限 `100,000` Active Child Chunks。超过前只保留原件和解析结果，不发布半截索引；管理员可在扩容或 Bake-off 后调整上限。

## 8. Postgres 检索投影

### 8.1 首轮 BM25 候选

首轮 Bake-off 候选固定为：

```text
PostgreSQL 16
ParadeDB pg_search 0.24.2
pgvector 0.8.2 (candidate image bundled version)
paradedb/paradedb:0.24.2-pg16
```

生产必须使用重新核验的 Registry Digest，禁止 `latest`。从 `postgres:16-alpine` 切换到 ParadeDB/Debian Image 时，不得直接复用旧 PGDATA；必须在新 Volume 安装扩展后，通过 `pg_dump/pg_restore` 逻辑迁移。

ParadeDB 是候选而非已晋升依赖。阻断项：AGPL-3.0 审批、WAL/Crash Recovery、逻辑恢复、中文 Tokenizer、ACL-in-query、峰值 RSS/WAL、升级/回滚。若失败，第二候选是 `pg_textsearch + PostgreSQL 17 + zhparser`；Postgres 原生 `ts_rank/ts_rank_cd` 只能作为 FTS Baseline，不能称为 BM25。

中文对比 `pdb.jieba`、`pdb.lindera(chinese)` 与 `pdb.chinese_compatible`，用 Golden Set 的中文、英文混排、型号、路径、错误码切片决定；不能凭默认 Tokenizer 上线。

### 8.2 Dense Profile

Jina Embeddings v4 `retrieval` 是 Hosted Baseline：Document 使用 `passage`，Query 使用 `query`。由于 pgvector HNSW 的 `vector` 维度上限与 2048 维存在冲突，首轮必须比较：

- `vector(1024)` Float32；
- `halfvec(2048)`；
- 小规模 Exact Vector Scan。

未通过 Golden Set 前不锁定维度。任何维度变化创建新 Generation；不得自动切换模型或把不同向量混入同一列。Chunk 少于实测阈值时优先 Exact Scan；达到阈值后才启用 HNSW，并以 Exact Search 对照 Recall。

HNSW Bake-off 必须覆盖低选择率 Personal/Team ACL，因为 pgvector ANN Filter 可能
在 Index Scan 后减少结果。Promotion 前冻结 `hnsw.iterative_scan`、`ef_search`、
Oversampling、Partial Index/Partition 或 Exact Fallback 策略；每个 ACL Slice 都
与 Exact Vector Search 比较 Recall 和 Top-K Completeness。

### 8.3 Hybrid Query

每条 Lane 都必须在 Top-K 生成内部应用相同 Fence：

```text
allowed collection ids
scope + owner_user_id/team_id
collection acl_revision + visibility_epoch
document/version active state + document visibility_epoch
current_version_id
active index_generation + projection_revision
```

起始 Candidate Profile：Dense 80、BM25 80、Exact 40；使用确定性 RRF 融合，取 30–50 条送 Jina Multilingual Reranker，最终保留 8–16 条 Evidence。所有 K、RRF Constant 和 Threshold 均由 Golden Set 决定，不是永久常量。

Exact Lane 处理 Quoted Phrase、文件名、路径、型号、版本号、错误码、代码符号和表格 Key。它不得被 Dense 或 BM25 分数覆盖。

## 9. Query、聊天与引用

```text
Knowledge Attachments
 -> Go current ACL/Consent snapshot
 -> signed private RAG request
 -> hybrid retrieval + rerank + parent expansion
 -> Go source reauthorization
 -> citation capability
 -> user BYOK Chat Provider with untrusted evidence envelope
 -> SSE answer + clickable citations
```

普通聊天不触发 RAG。Knowledge Mode 下的实质问题必须先检索；问候等非知识意图可以跳过。多轮追问先生成可审计的 Retrieval Query，但原始问题同时保留，防止改写漂移。证据不足时返回“知识库中未找到可靠依据”，不允许模型利用常识伪装成知识库答案。

引用定位：PDF Page/BBox、DOCX/HTML Heading/Paragraph、PPTX Slide/Shape、XLSX/CSV Sheet/Cell/Row、代码 Path/Line。Citation URL 由 Go 短时签发；每次打开重新授权。Document 删除、Membership 变化或 Consent 撤回后，旧 Citation 立即失效。

文档内容是 Untrusted Data：Evidence Envelope 明确禁止执行其中的 Prompt、URL、Tool Call 或密钥请求。RAG 不得把 Document Instruction 提升为 System/Developer Instruction。

用户 BYOK Provider 不是天然获准的 Evidence Processor。发送任何 Source Text
前，Go 必须验证该 Collection 对精确 `processor + endpoint + model + answer`
Purpose 的 Active Governance Profile、Collection Answer Consent 和 User Query
Consent；Personal Owner 自批，Team Collection 由 Team Admin 批准 Endpoint/Model
Allowlist。Prompt Assembly 前与 Provider Response 提交前各重验一次。未获批准时
只允许返回本地 Source 列表或拒绝回答，Team Member 不能借 BYOK 把 Team Evidence
发送到任意端点。

## 10. Consent、密钥与外部 API

- Collection 首次启用 Hosted Processing 时进行一次显式授权；
- Personal Collection 由 Owner 授权，Team Collection 由 Team Admin 授权；
- Bootstrap Public Collection 可由管理员统一授权；
- MinerU/Jina Key 仅存在服务器 Secret，不入 DB Payload、日志、前端或 Git；
- 限额因账号而异，运行时从 `429/Retry-After`、响应头和监控动态适配；
- 固定 Model ID、API Version、Role、Dimension 和 Response Schema；浮动 Alias 禁用；
- Consent/Governance 在外部调用前和结果提交前各检查一次。
- Parse、Embedding、Rerank、Answer 是独立 Purpose；批准 Jina/MinerU 不等于批准
  用户 BYOK Answer Endpoint。

Retry：仅 `408/429/5xx/网络超时` 使用 Full Jitter Backoff，遵守 `Retry-After`；普通 4xx、Schema/Hash/Dimension 错误立即隔离。MinerU 异步提交结果不确定时先查询 Provider Job ID，不能盲目重复创建。

## 11. 降级与故障行为

| 故障                     | 行为                                                                        |
| ------------------------ | --------------------------------------------------------------------------- |
| MinerU 不可用            | 新复杂文档保持排队/重试；存量查询不受影响                                   |
| Passage Embedding 不可用 | 新索引不发布；旧 Active Generation 继续服务                                 |
| Query Embedding 不可用   | 明示 `degraded=true`，仅使用 BM25 + Exact                                   |
| Reranker 不可用          | 使用已通过验证的 RRF-only Profile，否则拒绝 Grounded Answer                 |
| Redis 丢失               | Cache Miss，恢复 Postgres Poll；不丢 Job/ACL/Checkpoint                     |
| MinIO 不可用             | 上传/索引暂停；Strict Knowledge Query 因 Source 无法验证/打开而 Fail Closed |
| Worker 停止              | 新任务积压；存量检索继续                                                    |
| Postgres/rag-api 不可用  | Knowledge Mode Fail Closed；普通 BYOK Chat 可继续并告警                     |

所有降级都必须在 API 与前端可见，禁止静默宣称完整语义检索。服务恢复后自动重试；不同 Embedding Model/Dimension 不能作为自动 Fallback。

## 12. 单服务器资源与背压

Compose 起始 Hard Limit：Postgres 1024MiB、Worker 448MiB、RAG API 192MiB、MinIO
256MiB、Redis 64MiB、Go 160MiB、Next.js 256MiB、Proxy 64MiB，总计 2464MiB。
该值只是压测起点，给 3.82GiB 主机的 OS、Docker、Page Cache 和短时 Ops 留出约
1.3GiB；任何服务在该上限下不稳定就停止 Promotion，而不是借 Swap 掩盖 OOM。

Postgres 起始参数：`shared_buffers=256MB`、`work_mem=4MB`、
`maintenance_work_mem=64MB`、`max_connections=20`、`jit=off`。Go/RAG API/
Worker Pool 分别最多 6/6/2 个连接，并保留管理连接。

Worker 全局单并发，并持有 Session Advisory Lock 防误扩容。Chunk 每批 100–250 条短事务；查询压力高时暂停下一批写入。Query Semaphore=3、最多排队 6、排队超过 2 秒拒绝；客户端断开必须传播取消。磁盘剩余低于 20% 时停止新索引，不得填满系统盘。

Compose 同时限制 CPU、PID、只读 RootFS 和有界 tmpfs。Backup/Restic/Prune 与
BM25/HNSW Build/Reindex 使用同一全局 Maintenance Lock，禁止并行运行；Promotion
必须包含“索引进行时请求备份”和“备份进行时请求索引”的互斥及 OOM Gate。

## 13. 备份、恢复与重建

每日协调备份使用短时 Write Barrier：只暂停 Upload、Object Delete 和 Generation
Publish，等待 Worker 离开 Publish 临界区；ACL/Consent 撤回与账号禁用永远不得被
Backup 阻塞。分配共同 `backup_set_id`，在同一 Postgres MVCC Snapshot 中记录 LSN、
引用的不可变 MinIO Object Path/Size/Hash Manifest、Release/Image Digest、
Migration/Extension Version、Active Generation、Projection Revision 和未终结
Outbox 摘要。原始/Artifact Object Key 必须不可变。Redis 不备份。

本地 `pg_dump` 与引用 Manifest 固化并校验后立即解除 Barrier；后续 MinIO Copy、
restic Snapshot、R2 上传和 `restic check` 异步执行，不得继续阻塞安全撤权。只有
全部成功才更新 Last Successful Pointer。任一步失败都保留上一成功 Backup Set，
禁止把不同 `backup_set_id` 或 Manifest 外对象拼成一致集。

建议保留：`7 daily + 4 weekly + 3 monthly`；发布前额外备份；每月在临时目录或全新 VPS 完成 Restore Drill。Restic Password、R2 Recovery Credential 和 Workload Signing Recovery Key 不能只保存在被备份的 VPS 上。

恢复顺序：固定镜像 → 空 Postgres/MinIO → restic restore → Hash/Manifest 校验 → MinIO restore → `pg_restore` → 扩展/索引校验 → Redis 空启动 → Lease 回收 → Outbox Replay → Projection 对账 → RAG Ready。不同 Backup Set 的 DB 与 MinIO Artifact 禁止混用。

RPO 目标 `≤24h`、RTO 目标 `≤4h`，必须通过演练实测后才能宣称满足。

## 14. 可观测性与日志

最少指标：Outbox Lag/Age、Job Count/Attempt/Lease Reclaim、Provider Latency/429、Parse Quality、Chunk Count、Embedding Dimension、Generation State、Projection Lag、Lane Latency/Hit、Rerank Latency、Citation Reauthorization Failure、Degraded Query、Disk/RSS/DB Pool、Backup Age/Restore Result。

日志仅记录 Request/Event/Job/Document Version ID、Profile Hash、状态、耗时、错误码和 Hash；不记录正文、Query、Evidence、API Key、签名 Token 或 MinIO Credential。Readiness 必须区分“进程存活”和“Projection 已追平且 Active Generation 可用”。

## 15. Promotion Gates

### 数据与并发

- fresh `001 → new head`、published `009 → new head`、Down/Up Replay 全通过；
- duplicate/out-of-order/late-low-ID、crash-before-ack、Redis Flush 不漏事件；
- Lease Expiry 可回收，Stale Lease Completion 必须失败；
- Delete/Consent/Governance 与 Worker Completion Race 不得发布陈旧内容；
- 空 Projection 可由 Postgres + MinIO 完整重建，Hash/Span/ACL 100% 对账。

### 解析与检索

- 每种格式 Parse 成功率、非空覆盖、Page/Locator Provenance 达到冻结门槛；
- Dense、BM25、Exact、RRF、Rerank 每条 Lane 可单独复现；
- 中文 Tokenizer、1024/2048 Profile 和 HNSW/Exact 由 80 题集选择；
- 20 题 Frozen Holdout 只验证核心 Relevance Profile，不用于继续调参，也不能用于
  宣称 2% 级错误率或覆盖全部 Format/Security Slice；
- Personal/Team/Deleted/Old Version 泄露为 `0`；
- 无答案、Prompt Injection、Citation、Abstention 测试全部通过。

ACL、删除、Consent、Prompt Injection 和 Citation 使用独立确定性负向矩阵；每种
上线格式另有 Parser Corpus。若要声明百分比指标，必须预先定义样本量、Slice 最小
数量和置信区间，并扩充到足够规模；100 题 Relevance Set 不替代这些 Gate。

### 运行与恢复

- 500 文件/100k Child、索引并发 1、查询并发 3 下无 OOM；
- 检索 `p95≤2s`，首 Token `p95≤5s`；大型扫描 PDF `≤15min`；
- BM25 候选通过 License、逻辑恢复、Crash Recovery、升级和回滚；
- R2 中断不覆盖上一份成功备份；全新环境恢复满足实测 RPO/RTO；
- 独立 xhigh Review 最终为 `P0/P1/P2=0/0/0`。

## 16. 实施清单

### Phase 15.2A — Contract 与 Bake-off

- [x] 完成 Owner Grill，锁定单服务器产品、检索、文件、Consent、删除与备份边界。
- [x] 完成 Python RAG Consumer/Indexing 设计和独立调研。
- [ ] 冻结 Internal Evidence API、Workload Token、Error/Degraded/Citation DTO。
- [ ] Bake off ParadeDB/pg_search、pgvector Dimension、中文 Tokenizer 和恢复路径。
- [ ] 冻结 Canonical Block/Chunk/Generation/Projection Schema 与 Migration。

### Phase 15.2B — Durable Consumer

- [ ] 实现 Applied Event Ledger、Outbox Rescan、Lease Token/CAS 和 Redis Wake-up。
- [ ] 实现 Parse/Embedding/Purge Job Claim、Heartbeat、Retry、DLQ 和 Replay CLI。
- [ ] 通过重复、乱序、Late Commit、Crash、Redis Loss 和 Tombstone Race Gates。

### Phase 15.2C — Parsing 与 Indexing

- [ ] 实现格式路由、Sandbox、MinerU Adapter、Canonical IR 和 Quality Gates。
- [ ] 实现 Parent/Child/Overlap、Artifact Lineage、Jina Passage Embedding。
- [ ] 实现 Staging/Verify/Atomic Publish、Generation Rollback 和全量 Rebuild。

### Phase 15.2D — Query、Chat 与 Citation

- [ ] 实现 Go → RAG Workload Auth 和私有 `rag-api`。
- [ ] 实现 Dense/BM25/Exact/RRF/Rerank/Expansion 与统一 ACL-in-query Filter。
- [ ] 最小改动接入现有 Knowledge Attachment、BYOK Chat、Degraded UI 和 Citation。
- [ ] 通过三并发、租户隔离、Prompt Injection、Abstention 和 Citation Gates。

### Phase 15.2E — Operations 与 Promotion

- [ ] 固定 Compose Image Digest、资源上限、Egress Allowlist、Health/Readiness。
- [ ] 接入 restic + R2、Retention、Backup Manifest 和每月 Restore Drill。
- [ ] 冻结 100 题 Relevance Set 与独立安全/格式 Corpus，完成 Development/Validation
      与 Frozen Holdout。
- [ ] 完成安全、性能、备份恢复、回滚和独立 Review 后 Promotion。

## 17. 关键风险与回滚

- ParadeDB `pg_search` 的 AGPL、WAL 历史和索引恢复是阻断项；失败则不晋升。
- 当前约 4GB RAM 不允许本地运行大模型、Qdrant/OpenSearch 或多 Worker。
- `≤1GB` 原文不等于索引一定小；100k Child 是首发资源保险丝。
- 单服务器没有 HA；R2 备份解决恢复，不解决实时可用性。
- 新 Generation 未 Verified 前旧 Generation 始终服务；发布失败只删除新 Staging。
- 数据库镜像切换使用新 Volume + Logical Restore；回滚恢复旧 Image/Volume 和旧 Active Generation，禁止不同发行版争用同一 PGDATA。

## 18. 参考证据

- [`phase-15-accuracy-first-rag-design.md`](./phase-15-accuracy-first-rag-design.md)
- [`phase-15-recommended-implementation-profile.md`](./phase-15-recommended-implementation-profile.md)
- [`phase-15-1d-collection-document-consent-plan.md`](./phase-15-1d-collection-document-consent-plan.md)
- [`knowledge-acl-api.md`](../contracts/knowledge-acl-api.md)
- [ParadeDB self-hosted extension](https://docs.paradedb.com/deploy/self-hosted/extension)
- [ParadeDB guarantees](https://docs.paradedb.com/welcome/guarantees)
- [ParadeDB limitations](https://docs.paradedb.com/welcome/limitations)
- [ParadeDB tokenizers](https://docs.paradedb.com/documentation/tokenizers/available-tokenizers/jieba)
- [pg_textsearch v1.3.1](https://github.com/timescale/pg_textsearch/tree/v1.3.1)
- [pgvector](https://github.com/pgvector/pgvector)
- [Cloudflare R2 pricing](https://developers.cloudflare.com/r2/pricing/)
