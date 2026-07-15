# Phase 15.2C Generation-bound Parsing/Indexing 实施计划

- 状态：design locked / activation blocked
- 日期：2026-07-12
- 基线：migration `010` 与 Phase 15.2B Durable Consumer dark-run 已完成
- 目标：建立可恢复、可审计、按 Corpus Generation 隔离的解析、分块、Embedding、
  Search Projection Staging/Verify/Publish 链；在全部 Gate 通过前保持真实消费关闭

> 本计划是 Phase 15.2C 的唯一执行顺序。不得回改已发布 migration `010`，不得先开
> Dispatch 再补 Handler，也不得把评测候选值提前写成生产 `011`。当前
> `RAG_WORKER_DISPATCH_ENABLED=false`、空 Handler Registry 和 Legacy Job 排除规则继续
> 生效，直到本计划的 Activation Gate 明确放行。

## 1. 本阶段边界

### 1.1 包含

- Native Parser、MinerU Adapter、Canonical IR、Quality Gate；
- Parent/Child/Overlap Chunking 与精确 Source Provenance；
- Jina Passage Embedding、1024/2048 隔离评测和 Search Profile 选型；
- migration `012_rag_search_projection` Search Projection staging、后续 Generation Dispatcher；
- Processing Request、Provider Operation、Object Deletion Work；
- Object/Processor Gateway、Parser Sandbox 与最小权限 Postgres Roles；
- Generation-bound Dispatch、Staging、Verify、Atomic Document Publish；
- Legacy Reconciliation、Building Generation Rebuild、Rollback 和 Canary。

### 1.2 不包含

- 用户查询、`rag-api`、Dense/BM25/Exact/RRF/Rerank 在线检索；
- Go Chat 接 RAG、BYOK Answer Egress、Citation 或前端 UI 改动；
- 独立图片知识库、Image Embedding、以图搜图；
- 多服务器或 Kubernetes；
- 生产 Corpus Generation Promotion。用户可见 Query 属于 Phase 15.2D，最终生产
  Promotion、R2/Restore Drill 与完整告警属于 Phase 15.2E。

扫描 PDF 仍在范围内：由 MinerU/OCR 生成文本、表格和 Page/BBox Provenance，但不建立
图片向量。

## 2. 当前断层与裁定

| 当前断层                                        | Phase 15.2C 裁定                                                                            |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 新 Go Job 仍写 `legacy_projection_unbound=true` | `012` 增加 Processing Request 与 Dispatcher V2；Producer 只在检测到 `012` Capability 后切换 |
| `dispatch` 当前只 Ledger/Ack，不创建 Job        | 不复用旧 Action；新增原子 Dispatch Function，由 DB 重算完整目标 Generation 集合             |
| Worker 无 Artifact/Block/Chunk Staging API      | 只增加 Stage-specific `SECURITY DEFINER` Functions，不授予 Base Table DML                   |
| Chunk 唯一键阻止同 Generation Reprocess         | `012` 将唯一性收敛到 `materialization_id + source_span_hash + chunk_profile_hash`           |
| MinerU Submit 崩溃后无 Durable Identity         | 新增 Provider Operation Ledger，先落 Intent，再 Submit/Poll/Commit                          |
| Publish 缺完整 Profile/Consent/Manifest Gate    | 新增独立 Verify/Publish Finalizer；Publish 原子完成 Head CAS、Revision 与 Job 终态          |
| Handler 返回后 Runner 会再次 Finish             | Stage Finalizer 可返回 `terminal_committed=true`；Runner 不重复 Finish                      |
| Worker 没有 MinIO/Provider Credential           | 保持不发 Key；通过私有 Object Gateway 与 Processor Gateway 获取精确能力                     |
| 正文不可变触发器与删除 SLO 冲突                 | Immutable Lineage/Hash 与可删除 Payload 分离，并持久化精确 Object Deletion Work             |
| `011` Winner 尚未由真实 Relevance 决定          | 先完成 Evaluation-only Bake-off，再生成 Winner-specific `011`                               |

## 3. 不可破坏的系统不变量

1. migration `010` 已发布且不可修改；新对象只进入 forward migration。
2. Go/Postgres 是 ACL、Consent、Governance、Document Version 和 Generation Authority；
   Python 不从 Outbox Payload 推导授权事实。
3. 外部调用前和结果提交前各重验一次当前 Consent/Governance/Profile/Purpose。
4. Active 与唯一 Building/Verified Candidate 都必须收到完整 fan-out，不能只更新 Active。
5. Staging 数据永不可查询；单文档 Publish 不得切换 Corpus Generation Pointer。
6. Initial/Replace/Replacement Retry 可按冻结 CAS 推进 Current Version；Active Version
   Reprocess 与 Building Catch-up 保持权威 Version Pointer。
7. 旧 Active Generation 在 Build、Crash、Rollback 期间继续可服务。
8. Worker、Replay 无 MinIO Credential、Provider Key、任意公网 Egress 或 Base Table DML。
9. Source File 删除继续走独立 File Delete Contract；Document/Collection Purge 只清派生物。
10. 所有 Result、Manifest、Profile 与 Provider Request 使用 Canonical Bytes + SHA-256；
    密钥、正文、完整 Provider Response 不进入日志或 Hash Envelope。

## 4. 目标执行链

```text
Go admission transaction
  -> authoritative Document/Version mutation
  -> Processing Request(pending) + Outbox(pending)

Dispatcher V2 transaction
  -> lock and reread authority/fences
  -> DB computes Active + Building/Verified target Generations
  -> one Materialization(staging) + Parse Job per Generation
  -> Request(dispatched) + generation Ledger + Outbox(published)

Parse Job
  -> authorize -> source capability -> bounded fetch
  -> native parser sandbox OR durable MinerU operation
  -> quality gate -> Canonical IR -> parent/child chunks
  -> stage batches -> verify parse manifest
  -> atomic Parse completion + Passage Embedding Job enqueue

Passage Embedding Job
  -> authorize -> processor gateway -> Jina retrieval.passage
  -> validate response/dimension/hash -> stage search projection
  -> verify complete materialization
  -> operation-aware atomic Document publish

Generation rebuild
  -> reconcile every authoritative live version
  -> watermark/outbox replay caught up
  -> count/hash/ACL/tombstone manifest verified
  -> Generation verified (still hidden; promotion belongs to Phase 15.2E)
```

任何 Delete、Consent Revocation、Governance Head Change、Lease Expiry、Profile Mismatch
或 Manifest Mismatch 都必须使当前提交失败。失败的 Staging 可隔离/清理，但不得复活旧
Visibility Epoch。

## 5. Migration 顺序

### 5.1 `011_phase15_rag_search_projection`

`011` 只能在 C4 选出唯一 Search Winner 后生成，内容仅包含：

- 已审批且固定 Digest/Version 的 `pg_search`、`pgvector` Extension Contract；
- Winner Tokenizer/Dictionary/Config Hash；
- Winner Dense Shape：`vector(1024)` 或 `halfvec(2048)`；
- Cosine Opclass、Exact Threshold、HNSW 参数与 ACL Low-selectivity Strategy；
- `knowledge_search_profiles` 与 Generation `search_profile_id/combined_profile_hash`；
- Child Search Projection、BM25/Dense/Exact Index；
- 受限 Candidate Function 骨架、安全 Owner、`PUBLIC` Revoke 和 Restore Version
  Assertion；Phase 15.2D 前不向 `rag_api_reader` Grant Execute。

`011` 应在发现任何既有 Corpus Generation 时 Fail Closed。不得把 Synthetic L2
Bake-off 的参数晋升为生产配置；真实 Jina Vector 必须使用 Cosine 重跑。

### 5.2 `012_phase15_generation_dispatcher`

`012` 在 `011` 后增加：

```text
knowledge_processing_requests
knowledge_processing_stage_executions
knowledge_dispatch_preparations
knowledge_outbox_event_subscriptions
knowledge_approved_profile_bundles
knowledge_generation_rebuild_roots
knowledge_provider_operations
knowledge_object_operation_intents
knowledge_object_deletion_work
knowledge_deletion_authority_ledger/checkpoints
processing_request_id/source_event_id on materialization and job
materialization-bound chunk uniqueness
generation-bound purge fences
payload/lineage separation required for online payload purge
dispatcher/staging/finalizer/reconcile functions
gateway/operator capability roles
```

Processing Request 最小 Contract：

```text
id, source_event_id UNIQUE REFERENCES knowledge_outbox(event_id) RESTRICT
collection_id, document_id, document_version_id, file_id
operation: initial | replace | active_reprocess | replacement_retry
         | purge | generation_rebuild
requested_by_user_id
idempotency_scope, idempotency_key, request_hash
expected_current_version_id, advance_current_version
status: pending | dispatched | cancelled | failed
created_at, dispatched_at, cancelled_at
UNIQUE(idempotency_scope, idempotency_key)
```

同 Key 冲突时先读取既有 Request：Hash 相同返回原结果，Hash 不同返回
`IDEMPOTENCY_CONFLICT`，绝不能把 Hash 放入 Unique Key 后允许第二行。一个逻辑 Stage
使用独立 Head：

```text
knowledge_processing_stage_executions
  UNIQUE(processing_request_id, index_generation_id, stage)
  current_job_id, next_attempt_seq

knowledge_processing_jobs
  stage_execution_id, attempt_seq, replay_of_job_id
  UNIQUE(stage_execution_id, attempt_seq)
  UNIQUE(stage_execution_id) WHERE status IN ('pending', 'processing')
```

Replay 在同一 Stage Execution 下原子创建递增 Attempt 的 Successor，并切换
`current_job_id`；历史终态 Attempt 保留审计，任一时刻最多一个非终态 Attempt。这样 API
同步 Idempotency Fence、Active/Building 双 Generation Fan-out 与 `010` Successor Replay
可以同时成立。历史 Legacy Row 保留审计；只有 Successor 成功创建后才能 Cancel 对应
Legacy Nonterminal Row。

`active_reprocess` 只处理当前 Active Version，`advance_current_version=false`；现有
Reprocess API 若重新打开一个比 Current 更新的 Failed Replacement，Admission 必须写
`replacement_retry`、冻结 Expected Current Version 并令
`advance_current_version=true`。这两种语义不能在 Publish 时猜测。

## 6. Dispatcher V2 与原子数据库接口

禁止继续使用当前无业务 Effect 的通用 `action='dispatch'`。新增版本化接口：

```text
knowledge_dispatch_and_ack_outbox_v2(..., allocation_nonce, plan_hash, result_hash)
knowledge_prepare_dispatch_v2(...)
knowledge_claim_outbox_v2(...)
knowledge_apply_global_event_v2(...)
knowledge_reauthorize_processing_job(...)
knowledge_authorize_provider_call(...)
knowledge_stage_parse_batch(...)
knowledge_complete_parse_and_enqueue_embedding(...)
knowledge_stage_search_projection_batch(...)
knowledge_verify_materialization(...)
knowledge_finalize_canary_verification(...)
knowledge_publish_verified_materialization(...)
knowledge_purge_processing_job(...)
knowledge_reconcile_generation_batch(...)
knowledge_finalize_generation_readiness(...)
knowledge_create_building_generation(...)
knowledge_fail_index_generation(...)
knowledge_create_profile_bundle(...)
```

Worker 无 Projection Base SELECT。Claim 后先调用 `knowledge_prepare_dispatch_v2`：它验证
Outbox Lease，锁住 Authority/Candidate，由数据库计算并持久化有过期时间的目标 Generation
集合、Allocation Nonce 与 Plan Hash。Post-`012` Event 只能引用 Admission 已创建的唯一
Request ID；Pre-`012` Legacy Backlog 只在 Preparation Root 生成一个 Successor Request
ID。每个 Generation 只生成 Materialization/Stage Execution/Job UUID，不复制 Request。

Allocations 固定按 `(generation_seq ASC, generation_id ASC)` 排序。Plan Hash 是以下字段顺序
的 RFC 8785/JCS Hash：

```text
contractVersion, eventId, sourceEventId, processingRequestId, operation,
authoritySnapshotHash,
allocations[{generationSeq, generationId, materializationId,
             stageExecutionId, parseJobId,
             baseProfileHash, searchProfileHash, combinedProfileHash}]
```

Python `DispatchPlan` 必须表达同一有序多 Generation Allocations、Nonce 与 Plan Hash；
Adapter/Replay Result Hash 使用同一 Canonical 顺序。Apply 在锁内重算目标集合并与
Preparation 精确相等，随后一次性 Consume Nonce。调用方不能删减 Generation 或提交自选
ID；授权事实全部从权威表重读。

固定 Dispatch 行为：

1. Request-bound Event 锁 Outbox、Preparation、Processing Request、Corpus
   Head/Candidate、Collection、Document、Version；Global Event 走独立 Function，不伪造
   Processing Request；
2. 校验 Event/Request/Authority/Fence 与 Idempotency Hash；
3. 持锁分配 Materialization Sequence，禁止裸 `MAX()+1`；
4. 为每个目标 Generation 创建 Staging Materialization 与 Parse Job；
5. 写 Generation-scoped Applied Ledger；
6. Effect、Ledger、Request 状态与 Outbox Ack 同一 Transaction；
7. Replay 同 IDs/Hash 幂等，不同 Hash 隔离并令 Projection Unready。

Generation Operator Function 由 Corpus Head → Candidate Singleton → Base/Search Profile 顺序
加锁，校验 immutable Profile/Combined Hash、Migration/Extension Digest、唯一 Candidate 和
审计 Actor。Create 只能产生 `building`；Fail 只能作用于非 Active Candidate 并写稳定
Reason/Audit。Worker 无 Execute，Profile/Generation 不允许直接 Base Table DML。

首个 Profile 也不得手工 DML。C0 必须冻结 Parser/Chunk/Embedding 以及虽在 Phase 15.2D
才运行、但 `010` Base Profile 已要求的精确 Rerank Processor/Endpoint/Model/API Version；
禁止 Placeholder。`011/012` 以签名 Bake-off Report Hash 建立 Approved Profile Bundle
Registry，`knowledge_create_profile_bundle` 只接受 Registry 中逐字段完全一致的 Bundle，
在一个 Transaction 创建 immutable Base/Search Profile并写 Operator Audit。随后
`knowledge_create_building_generation` 才可引用该 Bundle。

Approved Registry Row 是 migration-owned Static Seed：其完整 Canonical Bytes、签名 Report
Hash 与 migration checksum 固定在对应 forward migration，不是运行期审批数据。Down 只可在没有
物化 Base/Search Profile、Generation 或 Work 引用时删除与内嵌值逐字一致的 Seed；发现
Drift、Extra Row 或引用必须整笔失败。

### 6.1 Event Ownership 与 Claim

`012` 增加 immutable `knowledge_outbox_event_subscriptions`。调用方不能传任意 Event Type；
`knowledge_claim_outbox_v2(consumer,...)` 只 Claim 数据库登记给该 Consumer 的类型。
Migration/Readiness 必须证明当前全部 Event Type 恰有一个 Owner：

| Event Type                                        | Owner               | C 阶段行为                                                                    |
| ------------------------------------------------- | ------------------- | ----------------------------------------------------------------------------- |
| `knowledge.collection.created`                    | `rag-index-v2`      | Global No-op Ledger/Ack；不创建 Request                                       |
| `knowledge.collection.updated`                    | `rag-index-v2`      | 刷新 Published/Verified Materialization 的 ACL/Revision Metadata，不 Re-embed |
| `knowledge.collection.tombstoned`                 | `rag-index-v2`      | Durable Purge Root，全量派生物枚举                                            |
| `knowledge.document.version.requested`            | `rag-index-v2`      | Request-bound Generation Dispatch                                             |
| `knowledge.document.reprocess.requested`          | `rag-index-v2`      | Request-bound Generation Dispatch                                             |
| `knowledge.document.tombstoned`                   | `rag-index-v2`      | Cancel + 全量 Purge                                                           |
| `knowledge.processing.cancelled`                  | `rag-index-v2`      | Cancel + 已写 Payload 清理                                                    |
| `knowledge.collection.consent.changed`            | `rag-index-v2`      | Global Egress Fence/Cancel/Delete-policy Effect                               |
| `knowledge.governance.head.changed`               | `rag-index-v2`      | Global Profile Fence/Cancel/Rebuild-policy Effect                             |
| `file.object.delete.requested`                    | `go-file-object-v1` | 既有 Source Object 删除链；RAG 不 Claim                                       |
| `knowledge.user.query-consent.changed`            | `rag-query-v1`      | Phase 15.2D Consumer；C 阶段保持 Pending                                      |
| `team.membership.changed`                         | `rag-query-v1`      | Phase 15.2D Cache/Auth Effect；C 阶段保持 Pending                             |
| `knowledge.generation.rebuild.requested`          | `rag-index-v2`      | 创建/恢复 Durable Rebuild Root                                                |
| `knowledge.document.generation-rebuild.requested` | `rag-index-v2`      | 为精确 Document/Generation 创建 Rebuild Request                               |

Operator 创建 Building Generation 时同一 Transaction 写
`knowledge.generation.rebuild.requested` 与 Durable Rebuild Root。Root 枚举器为每个
Document/Generation 写唯一 `knowledge.document.generation-rebuild.requested` Child Event；
只有 Child Event 创建一个 `generation_rebuild` Processing Request，因此其
`source_event_id UNIQUE` 仍成立。Global Event 走 `knowledge_apply_global_event_v2`，不执行
Request-bound 固定锁序。订阅 Owner 尚未部署时对应 Event 保持 Pending，不能被其他
Consumer 领取或耗尽 Attempt。

### 6.2 Outbox Effect Matrix

| Event Type                                        | Scope/目标                                            | 原子 DB Effect                                                                                       | Ack/Replay 条件                                                                   |
| ------------------------------------------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `knowledge.collection.created`                    | Global                                                | 写明确 No-op Ledger，不创建 Request                                                                  | Event Shape/Revision 合法后 Ack                                                   |
| `knowledge.collection.updated`                    | 该 Collection 的全部可见/隐藏 Materialization         | 更新 ACL/Revision Metadata Projection，不调用 Provider                                               | 全目标 Generation/Materialization 对账后 Ack                                      |
| `knowledge.document.version.requested`            | 每个 Active + Building/Verified Generation            | 创建/确认 Request、Materialization、Parse Stage Head/Job                                             | 所有目标集合与 Allocation 完全一致后写 Generation Ledger/Ack；Replay 同 Hash 幂等 |
| `knowledge.document.reprocess.requested`          | 同上                                                  | 按权威 Version 状态选择 `active_reprocess` 或 `replacement_retry`                                    | 冻结 Expected Current/Advance Flag；异 Hash 隔离                                  |
| `knowledge.document.generation-rebuild.requested` | 精确 Building Generation                              | 创建 `generation_rebuild` Request/Stage                                                              | Child Event/Document/Generation 唯一后 Ack                                        |
| `knowledge.generation.rebuild.requested`          | 精确 Building Generation                              | 创建/恢复 Rebuild Root，固定 Snapshot/Watermark                                                      | Root 持久化后 Ack；Child 枚举可重放                                               |
| `knowledge.document.tombstoned`                   | 该 Document/Version 的全部派生物                      | Cancel 非终态 Stage/Provider Work；枚举所有未 Purged Materialization/Artifact/Object Intent          | Durable Purge Work 已完整创建后 Global Ack；Online Payload Purge 异步重放         |
| `knowledge.collection.tombstoned`                 | Collection 全部 Version/Generation/Materialization    | 创建/恢复 Purge Root，分页枚举全部派生物                                                             | Root 已持久化后 Ack；枚举和删除可恢复，不能只枚举 Head                            |
| `knowledge.processing.cancelled`                  | 精确 Request/Job/Materialization                      | Cancel 非终态 Attempt，阻止 Provider/Publish；已有 Payload 转 Deletion Work                          | Cancel/Purge Intent 持久化后 Ack                                                  |
| `knowledge.collection.consent.changed`            | 受 Processor/Endpoint/Model/Purpose 影响的非终态 Work | 失效旧 Revision Job/Provider Operation；条款要求删除时创建完整 Purge/Rebuild Work，否则只阻断 Egress | 当前 Revision Effect 已落库后 Global Ack；旧 Event 不覆盖新 Revision              |
| `knowledge.governance.head.changed`               | 旧 Profile/Model 的非终态 Work                        | Cancel 旧 Profile Provider Call/Publish；需要时创建 Rebuild/Deletion Work                            | 新 Head Revision 与 Effect 原子确认后 Ack                                         |
| `file.object.delete.requested`                    | Source File Worker，不属于 RAG Projection Consumer    | RAG Dispatcher 不 Claim；独立 File Object Consumer 按既有 Contract 处理                              | 不写伪造 RAG Ledger/Ack                                                           |
| Team Membership/User Query Consent changed        | Query Cache/Authorization，Phase 15.2D                | C 阶段不 Reindex、不 Claim；后续 Query Consumer 处理                                                 | 不伪造 Index Effect                                                               |

索引 Fan-out 只面向可构建的 Active + Building/Verified Generation；删除 Fan-out 永远按
Document/Version 枚举所有未 Purged Materialization、Parser Artifact、Search Payload 和
Object Intent，不受 Generation `active/retired/failed/staging` 状态或 Projection Head
限制。`010` 的每 Generation 单 Materialization 枚举不能作为 `012` 删除实现。

## 7. Parser、Canonical IR 与 Chunking

### 7.1 Format Router

| 类型          | 主路线                                    | 强制边界                                   |
| ------------- | ----------------------------------------- | ------------------------------------------ |
| TXT/Markdown  | encoding detection + deterministic parser | 保留行号、Byte/Char Offset                 |
| HTML          | hardened DOM parser                       | 禁脚本、DTD、XXE、外部资源与网络           |
| DOCX/PPTX     | native OOXML parser                       | 禁宏；复杂布局可按 Policy 转 MinerU        |
| XLSX/CSV      | openpyxl/pyarrow-compatible reader        | 只读，不执行公式；保留 Sheet/Cell/公式和值 |
| 普通 PDF      | approved native text/layout parser        | 保留 Page/Block/BBox/字体/阅读序           |
| 扫描/复杂 PDF | MinerU Hosted                             | 异步恢复、OCR/表格/公式与 Page/BBox 对账   |

限制固定为原文件 `≤50 MiB`（`52,428,800` Bytes）、PDF `≤500` 页；压缩展开比、嵌套
深度、Cell、Block、Chunk、总文本字节和执行时长必须有硬上限。第一版独立图片 MIME 在
Admission 拒绝。

### 7.2 Parser Sandbox

Native Parser 运行在独立 Rootless Sandbox：无 DB、Secret、MinIO Credential、Provider
Key、Host Mount 和公网；只读 RootFS、有界 tmpfs/CPU/RAM/PID，输入/输出均使用长度和
Hash 约束的流。超限只能终止 Sandbox，不能杀死 Worker 控制循环。

Compose 使用无网络的常驻 Sidecar，通过仅 Worker/Sandbox 可见的 Unix Socket 通信，不挂
Host Path，也不暴露 Docker Socket。共享实体是 Docker Named Volume
`rag-parser-ipc`，Local Driver 固定 `type=tmpfs,device=tmpfs`，Options 为
`uid=10001,gid=10001,mode=0770,size=16m`，双方挂载路径均为
`/run/mm-chat-rag-ipc`。Worker 保持 `10001:10001`；Sandbox 使用独立 UID `10002`、共同
GID `10001`。Sandbox 启动时只在取得单实例 IPC Lock 后删除该目录中的 Stale Socket，
随后创建 Mode `0660` Socket；每个 Job 再启动一次受限 Parser Child。唯一 Parser Protocol
v1 由 Offline Parser/Canonical IR Addendum §6.2 逐 Byte 冻结：Request 只携带
Invocation、MIME/Extension Hint、Config/Size/Source Hash/Deadline，不携带调用方裁决的
Format；Response 使用 Closed `success | route_required | failure` Outcome、Schema、Result
Size/Hash 与 Stable Error，不暴露平台 Exit Code。Router 在 Sandbox 内由 Magic/Container
推导 Format。双方强制 Frame/总字节上限与 Deadline；Cancel/Sidecar Crash 是 Controller
本地 Outcome，短读、Hash/Schema 不符或 Timeout 均不得 Stage。禁止实现第二套 Runtime
Adapter Wire。

### 7.3 Canonical IR

Canonical Block 必须版本化并至少保存：

```text
document/version/materialization/block identity and ordinal
block_type, parent_id, heading_path, reading_order
text/markdown/html/latex/code representation
page/slide/sheet/cell/paragraph/line/BBox locator
table grid/span, parser/config/model hashes, confidence
source_span_hash, content_hash, derived, non_indexable, needs_review
```

质量门覆盖 Source Hash、页数、空页、文本覆盖率、替换字符、OCR Confidence、阅读顺序、
表格结构、Locator Round-trip 和 Manifest Count/Hash。失败页可有界重试；仍失败必须
Quarantine，禁止把空文本标记成功。

### 7.4 Parent/Child/Overlap

- Parent 按 Section 对齐：目标 `1400–1600` Token，硬上限 `2000`；
- Child：目标 `350–500` Token，硬上限 `650`；
- 相邻 Child Overlap：`60–100` Token；
- Heading、Table、Code、Sheet 边界不强行跨越；
- Table、Code、List、FAQ 使用独立、版本化 Policy；
- Chunk ID/Span/Content Hash 必须稳定且可回溯到精确 Canonical Block/Source Locator；
- Tokenizer Name、Revision、文件 Hash 和 Normalize Policy 必须写入 immutable Profile。

## 8. Provider Contract 与 Crash Recovery

### 8.1 C0 外部 Wire Contract Gate

唯一冻结源为
[`provider-wire-fixture.md`](../contracts/provider-wire-fixture.md)。实现真实 Adapter 前必须
由 Closed Schema、脱敏 Fixture、Evidence、Reviewer 与 RFC 8785 Hash 冻结：

- MinerU/Jina Base URL、Region、Endpoint ID、Auth Header 名称；
- 精确 Model ID、不可变 Model/API Build Version；
- MinerU Submit/Poll/Result/Cancel/Query-by-key 路径与 Result Schema；
- BBox 坐标系、Page Index 基准、分页/压缩包结构；
- Idempotency/Query-by-key 能力、Rate Limit、`Retry-After`；
- Jina Batch/Token/Bytes/Dimension/Normalization 与 Error Schema；
- 精确 Rerank Processor/Endpoint/Model/API Version 与 Governance Profile 输入；C 阶段不调用
  Rerank，但 `010` Base Profile 禁止 Placeholder；
- Retention、Deletion、Training-use、License、SLA；
- 至少一组成功和每类稳定错误的脱敏 Request/Response Fixture。

Key 只写 Secret File，绝不进入 Fixture、Git 或日志。若 Provider 不支持 Query-by-key，
必须在计划中明确 `UNKNOWN_SUBMISSION` 的人工/自动恢复规则，不能用盲目 Resubmit 掩盖。
当前公开 MinerU、Jina 1024/2048 和 Rerank Fixture 均为 blocked `draft`；它们与内存 Fake
只能用于 Contract Test，不能派生 Governance 或关闭本 Gate。

### 8.2 MinerU Durable Operation

```text
NEW -> SUBMITTING -> SUBMITTED -> RUNNING
-> RESULT_READY -> DOWNLOADED -> VALIDATED -> COMMITTED

RETRY_WAIT | UNKNOWN_SUBMISSION | FAILED | CANCELLED | DELETED
```

先持久化 Intent/Request Hash/Idempotency Key，再调用 Submit；收到 Provider Job ID 后立即
CAS 保存。Submit 响应丢失时优先 Query-by-key/Provider Job ID；下载结果必须验证 Size、
Hash、Schema、Page/BBox 和 Source Snapshot 后才 Commit。

### 8.3 Jina Passage Embedding Candidate Contract

候选请求固定语义：

```text
model=jina-embeddings-v4
task=retrieval.passage
embedding_type=float
truncate=false
late_chunking=false
return_multivector=false
dimensions=1024 OR 2048 (evaluation profiles are isolated)
```

Adapter 必须验证 Response Model、完整 `index=0..N-1`、有限数值、精确 Dimension、Usage
Shape 和 Result Hash。只有网络超时、`408/429/5xx` 可重试并遵守 `Retry-After`；普通
`4xx`、Schema/Dimension/Index/NaN 错误永久隔离。

Provider Batch 不能直接照搬数据库 100–250 Chunk Batch。C3 分别测试每请求
`8/16/32` Items、`4k/8k/16k` 聚合 Token、并发 1，按 p95/p99、429、费用和 Lease
裕量冻结唯一配置。

### 8.4 Hash Contract

使用 RFC 8785/JCS Canonical JSON 与 SHA-256 小写 64 Hex；禁止 Hash `jsonb::text`。

```text
content_hash      = sha256(exact UTF-8 child bytes)
request_hash      = sha256(JCS(contract/profile + ordered chunk ids/hashes/tokens))
provider_hash     = sha256(provider float32 canonical bytes)
stored_vector_hash= sha256(actual float32/float16 storage bytes)
batch_result_hash = sha256(JCS(request hash + usage + ordered stored vector hashes))
```

`halfvec(2048)` 必须 Hash 量化后的 Float16 Storage Bytes；不能用 Provider Float32 Hash
冒充已存向量 Hash。

## 9. Gateway、Credential 与 Role 边界

| 服务/Role                 | 唯一职责                                               | 明确禁止                                   |
| ------------------------- | ------------------------------------------------------ | ------------------------------------------ |
| `rag-worker`              | 编排 Claim、Heartbeat、Gateway、Stage/Finalize         | MinIO/Provider Key、任意公网、Base DML     |
| `rag-parser-sandbox`      | 处理已绑定 Hash 的单个输入并返回 Canonical Candidate   | DB、Secret、网络、Host Mount               |
| `rag-object-gateway`      | 解析一次性 Object Capability，精确 GET/PUT/Delete      | List Bucket、暴露 Object Key、任意 Prefix  |
| `rag-processor-gateway`   | 唯一 RAG Egress，Endpoint/Model/Purpose Allowlist      | MinIO Key、任意 URL、聊天 Provider Key     |
| `rag-generation-operator` | Create/Verify/Fail Building Generation                 | Promote Active Pointer、普通 Job、正文读取 |
| `deletion-sealer`         | Seal Payload-free Deletion Authority 与 Monotonic Head | Source/Object Payload、Parser、任意 DB DML |
| `rag-replay`              | 受限 DLQ Replay                                        | Object/Provider/Promotion Credential       |

Object Capability 至少绑定：

```text
aud, operation, job_id, lease_token_hash
file_id, document_version_id, source_hash
generation_id, materialization_id
max_bytes/body_hash, exp, jti
```

Source 仅精确 GET；Artifact 仅当前 Materialization 的 Content-addressed PUT；Purge 仅精确
Manifest Delete。错误 Operation、过期、重放、旧 Lease、撤权或 Hash 不符全部拒绝。

### 9.1 Internal Gateway Wire Contract

- Worker ↔ Gateway 使用独立 internal mTLS Client Identity；证书文件只读挂载、短周期轮换，
  Server 验证精确 SAN/Audience。Capability 不替代服务身份，服务身份也不替代 Job 授权。
- 单服务器使用 256-bit Opaque One-use Capability，不使用可离线伪造的自包含 Bearer
  Claim。Worker 生成随机 Secret，数据库只保存 Hash；受限 Mint Function 将 Hash 绑定当前
  Job/Lease/Object Operation Intent。Gateway 通过 Consume Function 原子验证并标记 `jti`
  Used。原始 Secret 只存在请求内存，日志/Metric/DB 均不保存。
- mTLS CA/Client Key Rotation 支持 Current+Next 双信任窗口；Opaque Capability 无签名 Key，
  过期、旧 Lease 或已经 Consume 的 Token 直接失效。
- 每次请求固定 Method/Content-Type，Header `≤16KiB`；Source/Artifact Body 受 Capability
  `max_bytes/body_hash` 约束，Streaming 全程计数，响应 Error Body `≤8KiB` 且只含稳定 Code。
- `rag-worker` 只连接 `rag-control` internal network；Object Gateway 双网卡只连
  `rag-control + storage-private`；Processor Gateway 双网卡只连
  `rag-control + rag-egress`，不能解析/连接任意 URL，也不能访问 Storage Network。
- Processor Gateway 同样要求 DB-fenced Provider Operation Intent、当前 Lease 与双重
  Reauthorization；Endpoint/Model/Purpose 从 Allowlist 解析，调用方不能传任意 URL/Header。

Artifact PUT 前必须创建 Durable Object Operation Intent：

```text
intent -> authorized -> stored -> bound -> deletion_pending -> deleted
                 \-> retry_wait | failed | expired_orphan
```

PUT 写入 Content-addressed 临时对象；Gateway CAS 保存 Size/Hash/ETag，Stage Function 才把
它绑定到 Materialization Manifest。`stored` 后在冻结 Grace 内未 `bound` 的对象由 Orphan
Sweep 创建精确 Deletion Work；Sweep 不能 List Bucket，只读取 Durable Intent。Crash 在
PUT、DB Stage 或 Publish 任一点都不会遗留不可追踪对象。

### 9.2 Role 变化

`012` 扩展 `010` 已创建的 Role，而不是重建：

- `rag_worker_executor`：Authorized Claim/Heartbeat/Stage/Finalize；
- `rag_replay_operator`：保持现有隔离，不继承新增能力；
- `rag_api_reader`：继续 NOLOGIN/无 Search Execute，Phase 15.2D 才开放。

`012` 只新建：

- `rag_object_gateway_runtime`：Job/Object 授权与内部 Object-key Resolution；
- `rag_processor_gateway_runtime`：Provider Reauthorization/Operation Ledger；
- `rag_generation_operator`：Create/Verify/Fail Building Generation；生产 Promote/Active
  Pointer Rollback Function 与 Grant 均留到 Phase 15.2E。
- `rag_deletion_sealer_executor NOLOGIN`：只 Execute Claim/Finish Seal Functions；
- `rag_deletion_sealer_runtime LOGIN NOINHERIT`：只允许精确 Client-certificate Mapping，
  仅被 Grant `rag_deletion_sealer_executor`，必须显式 `SET ROLE`；无其他 Membership、
  Base Table 或 Payload 权限。

`012.down` 只撤销新增 Grant/Function 并删除 `012` 新 Role；不得删除或重建 `010` Role。

Provider Secret 按 Purpose 分离为 MinerU Parse 与 Jina Passage Key；未来 Query/Rerank Key
另建，不把同一 Key 同时挂给 Backend、Worker、Replay 或 Query Service。

## 10. Staging、Publish、Purge 与 Rebuild

### 10.1 Stage Finalizer 语义

普通 Handler 返回：

```text
retry | permanent_failure | staged_result
```

Parse/Embedding 的成功终态只能由 Stage-specific Finalizer 在持有当前 Lease Token 时原子
提交。Finalizer 返回 `terminal_committed=true` 后 Runner 不再调用通用 Finish；异常和
Retry 仍由 Runner 统一处理。旧 Lease 的 Stage、Finalize、Capability 全失败。

### 10.2 Atomic Document Publish

固定锁序：

```text
Collection -> Document -> Old/New Version -> Corpus Generation
-> Projection Head -> Materialization -> Job
```

Publish 前必须对账：

- 当前 Lease、Request、Operation、Generation、Profile/Combined Hash；
- File/Version Source Hash 与 Parser/Chunk/Search Manifest；
- Artifact/Block/Parent/Child/Search Row Count 与 Aggregate Hash；
- Collection/Document Live State、ACL/Visibility/Processing Revision；
- Consent/Governance Head、Endpoint/Model/Purpose/Expiry；
- Expected Current Version/Projection Head 与 Visibility Epoch。

Verify 与 Publish 分离：`knowledge_verify_materialization` 只把完整 Staging Manifest 推进
到隐藏 `verified`，不改 Projection Head/Version/Job 终态，供 Canary 逐 Row 对账；
Canary 随后必须调用 `knowledge_finalize_canary_verification`，在同一 Transaction 保持 Head
不变、终结 Stage Attempt 并把 Materialization 标记为不可 Publish 的
`canary_verified`。Runner 收到 `terminal_committed=true` 后不再 Finish，Lease 不会回收
重跑。Crash-before-finalize 允许旧 Lease 回收并幂等重验；Crash-after-finalize 不再 Claim。
`knowledge_publish_verified_materialization` 才在同一 Transaction 更新
Materialization/Projection Head、Projection Revision、结束 Job并写 Audit/Outbox。
`initial`、`replace` 和 `replacement_retry` 可在冻结的 Expected Current Version CAS 成功时
推进 Version/Document Pointer；`active_reprocess` 保持当前 Pointer。任一检查失败整笔
回滚。

### 10.3 Durable Online Payload Purge 与 Retained-copy Expiry

Lineage ID/Hash/审计元数据可保持不可变；用户正文、Parser Artifact 与向量 Payload 必须
可精确删除。`knowledge_object_deletion_work` 保存 Manifest Hash、Deadline、Attempt、
Lease、Error、Terminal State。逻辑不可见立即生效，Online Payload Purge 10 分钟预警、
15 分钟告警；删除失败保持 `purging` 可重试，永不重新可见。该 SLO 不表示 WAL、Backup、
Snapshot 或当前 Data File 已完成介质擦除。

状态必须区分 `logically_tombstoned -> online_payload_purged -> retained_copy_pending ->
retained_copy_window_expired`。最后一态只证明受管 Object Version/WAL/Backup/Snapshot 已过
冻结保留窗；不宣称 Postgres Free Page 或 SSD Block 的 Disk-forensic Erasure。准确的介质
边界、最长 `8 weeks` Retention、独立 Deletion Authority 与禁用“physically erased”措辞
由 Offline Parser/Canonical IR Addendum §15.1 冻结。

Purge Claim 优先级高于 Parse/Embedding，且磁盘低水位 Admission 不得阻止 Purge/撤权。
Document/Collection Purge 必须枚举每个 Version 的所有未 Purged Materialization，包括旧
Reprocess 结果、Retired/Failed Generation 和无 Head 的 Staging；随后覆盖其全部 Artifact、
Search Payload、Object Operation Intent。删除完成判定不得使用每 Generation
`DISTINCT ON` 或“仅当前 Head”。

### 10.4 Generation Rebuild

Rebuild 扫描所有权威 Live/Pending Version，不只扫描 Legacy Job；为唯一 Building
Generation 创建 Request/Materialization/Job，记录 Watermark，并持续 Replay 其后的
Outbox。只有满足以下条件才能标记 Generation `verified`：

- 所有 Live Version 有唯一 Published Materialization；
- Tombstoned/Revoked/Old Version 无可见 Projection；
- Outbox Watermark/Applied Ledger 已追平；
- Count/Hash/ACL/Consent/Profile/Manifest 100% 对账；
- Crash/Restore/Rebuild 重跑得到相同 Logical Manifest。

本阶段不把它切成 Active；Corpus Pointer Promotion 留给 Phase 15.2E。

## 11. Search Winner 评测

2048/1024 必须使用同一 Source Snapshot、Parser Artifact、Canonical Block、Chunk
Manifest、Query/Relevance Set，唯一变量为 Dense Shape：

```text
A: Jina 2048 float response -> halfvec(2048)
B: Jina 1024 float response -> vector(1024)
```

两条候选在独立、可销毁 Evaluation Schema/Artifact 中运行，不写生产
`knowledge_index_generations`，避免当前唯一 Candidate 约束和生产状态污染。分别请求
1024/2048，不假设截取前 1024 维等价。

评测至少覆盖：

- 80 题 Development/Validation 与 20 题 Frozen Holdout；
- 中文、英文混排、型号、路径、错误码、Quoted Phrase、表格；
- 1024/2048 Exact Cosine 与 HNSW Recall；
- 三种中文 Tokenizer、BM25、Exact、RRF；
- Personal/Team 低选择率 ACL、零泄露；
- p50/p95/p99、429、Token/费用、RSS/WAL、Build/Restore 时间。

最终产出签名 Bake-off Report 与 Hash，冻结唯一 Tokenizer、Dimension、Dense Type、
Metric、Exact Threshold、HNSW/ACL Strategy 和 Image/Extension Digest。Holdout 不得用于
继续调参。C 阶段负责创建并冻结 100 题集、完成 Winner 选择和本地新 Volume 逻辑
Restore/Crash/Down-Up；Phase 15.2E 只消费已冻结 Report 做生产 Promotion Acceptance、
协调 Postgres+Object Manifest 的 R2 Restore Drill，不重新打开 Holdout 调参。

## 12. 实施顺序与可勾选任务

### C0 — Contract Addendum 与外部输入

- [x] 完成四路 xhigh 现状、执行链、Provider/Search 和安全运维审计。
- [x] 冻结 `010` 不可变、`011` Search-only、`012` Dispatcher 的迁移顺序。
- [x] 冻结 Processing Request、双 Generation Fan-out、Stage Finalizer、Gateway 和
      Deletion Work 设计。
- [x] 实现 Closed JSON Schema、Strict Loader、Secret/Placeholder/Hash Gate、完整长度
      Jina 1024/2048 Draft Fixture 和无网络 Fake Provider 基线。
- [x] 禁止危险 `default/model-v1/v1` 示例直接执行 Governance Apply；未冻结时 Fail Closed。
- [ ] 获取并校验 MinerU/Jina 脱敏 Wire Fixture、Model/API Build、License/SLA/Retention。
- [ ] 关闭 C0 外部 Contract Gate；未关闭前禁止真实 Provider 调用。

### C1 — Offline Parser/Chunk Harness

- [ ] 建立 Parser Corpus、恶意文件 Corpus、Canonical IR Schema 与 Golden Manifest。
- [ ] 实现 Format Router、Sandbox、Native Parser、Quality Gate 和确定性 Hash。
- [ ] 实现 Parent/Child/Overlap 与 Source Locator Round-trip 测试。

### C2 — Provider Fake/Adapter

- [ ] 建立 MinerU Fake Server：Submit/Poll/Result/Cancel、429/5xx、响应丢失和恢复。
- [ ] 实现 Storage-neutral Provider Operation State Machine 与
      `UNKNOWN_SUBMISSION` Fake 恢复路径；本步不依赖 `012` 数据库表。
- [ ] 建立 Jina Fake/Contract Test：Batch、Dimension、Index、NaN、Retry-After、Hash。

### C3 — Evaluation-only Shadow Embedding

- [ ] 用冻结 Parser/Chunk Output 分别生成 1024/2048 真实 Jina Passage Embedding。
- [ ] 对比 Float32/Float16 Storage Hash、Exact Cosine、HNSW 与 ACL Low-selectivity。
- [ ] 冻结 Provider Batch、Timeout、Retry、费用和资源上限。

### C4 — Relevance/SLO/License Promotion Gate

- [ ] 完成 Development/Validation，最后只运行一次 Frozen Holdout。
- [ ] 选出唯一 Tokenizer/Dimension/Search Profile，签名并记录 Bake-off Report Hash。
- [ ] 关闭 Extension/Provider License、Upgrade、Restore、Crash、Rollback Gate。

### C5 — Migration `011`

- [ ] 生成 Winner-specific `011.up/down.sql`，不得含占位值。
- [ ] 使用固定 Extension Image 新 Volume 进行逻辑迁移，禁止复用 Alpine PGDATA。
- [ ] 通过 Fresh、Published `010→011`、Generation-present rejection、Down/Up、Restore、
      Crash、Role/Permission 测试。

### C6 — Migration `012` 与 Go Producer Cutover

- [ ] 实现 Event Subscription/Ownership、Dispatch Preparation、多 Generation Plan 和
      Global/Request-bound Effect 分离。
- [ ] 实现 Approved Profile Bundle Registry/Create Function 与 Generation Rebuild
      Root/Child Event。
- [ ] 实现 Processing Request、Provider Operation、Deletion Work、Chunk/Purge Fence。
- [ ] 实现 Payload-free Append-only Deletion Authority、连续 Sequence/Hash Chain、独立
      Sealed Checkpoint 与 Online/Retained-copy 状态机；Restore 不得只信旧 Database。
- [ ] 实现独立 `deletion-sealer` Workload/最小 DB Role、Ed25519 Key Rotation、WORM
      Store、Monotonic Signed Head 与 Inclusion/Consistency Proof；旧签名 Head 不得回滚。
- [ ] 实现 Stage Execution/Replay Head、Object Intent、Durable Provider Operation Ledger。
- [ ] 实现 Dispatcher V2、Verify/Publish/Purge/Reconcile、Generation
      Create/Fail Functions 与 Roles。
- [ ] Go Producer 增加 `010` Legacy fallback 与 `012` Request+Outbox 双路径。
- [ ] 证明 `012` 路径零直接 Legacy Job，重复 Reprocess 仍被同步 Idempotency Fence 拦截。
- [ ] 证明 N-1 Binary 在 `012` 上的 Initial/Replace/Reprocess/Delete 全部 Fail Closed，且
      只有本地/Off-host Deletion Authority 均为 Genesis 时，`012.down→N-1` Maintenance
      Rollback 才可执行且不产生重复 Job/Outbox。
- [ ] 回归 `012` Admission 成功、Dispatch 未启用、尝试 Down、N-1 同 Key 重试：Down 必须
      因 Pending Request Fail Closed，并保留 Idempotency History。
- [ ] 通过 Fresh `010→011→012→012.down→N-1`：未物化 Profile/Generation/Work 时只删除
      完全匹配 migration checksum/report hash 的 Static Registry Seed；Drift/Extra 拒绝。
- [ ] 证明任一 `authoritySequence > 0` Runtime Entry 都永久阻断 N-1 Down；空 Authority
      可有 Freshness Checkpoint，但须由独立 Operator Preflight 离线校验 Off-host Genesis
      Root/最新签名 Head，SQL Down 不得访问网络或自行删除 Authority。

### C7 — Private Gateway/Sandbox Runtime

- [ ] 实现 Object Gateway 一次性 Capability 与精确 GET/PUT/Delete。
- [ ] 实现 Processor Gateway Endpoint/Model/Purpose Allowlist 与双重 Reauthorization。
- [ ] 实现 internal mTLS、Opaque Capability Mint/Consume、Object Intent/Orphan Sweep。
- [ ] 接入 Parser Sandbox、Purpose-separated Secret File、Egress/Network/Resource 限制。
- [ ] 固定 tmpfs-backed Named Volume、Unix Socket Framing、Size/Hash/Timeout/Cancel/Exit
      Contract，证明无 Host/Docker Socket 与额外 Network。

### C8 — Parse/Chunk Handler

- [ ] 实现 Source Fetch、Native/MinerU Parse、Artifact/Block/Chunk Stage。
- [ ] 实现 Parse Manifest Verify 与原子 Enqueue Passage Embedding。
- [ ] 通过格式、Zip Bomb/XXE/Macro/外链、超限、OCR/表格和 Crash Matrix。

### C9 — Embedding/Publish/Purge Handler

- [ ] 实现 Jina Passage Batch、Search Projection Stage 与 Dimension/Hash Verify。
- [ ] 实现 Atomic Publish、旧 Materialization Retire、Operation-aware Version Pointer。
- [ ] 实现 Document/Collection Purge、Object Deletion Work 与 15 分钟 SLO 告警。
- [ ] 验证 15 分钟只约束 Online Payload Purge；Object Version/WAL/Backup/Snapshot 按
      Retention Evidence 推进，禁止报告 Disk-forensic Erasure。

### C10 — Canary 与 Generation Rebuild

- [ ] 创建隔离 Canary Collection；只 Stage/Verify，不 Publish，逐 Row 对账。
- [ ] 通过 Canary Finalizer 原子结束 Stage Attempt 且保持 Head 不变，验证 Crash/Lease
      Reclaim 不会重复执行已终结 Canary。
- [ ] 通过受限 Operator Function 创建唯一 Building Generation，完成全量 Reconcile、
      Watermark Replay 和 Manifest。
- [ ] 通过 Initial/Replace/Reprocess/Delete/Consent/Governance/Crash Race。
- [ ] 验证 Rollback 后旧 Active/Legacy 路径保持一致且无 Staging 可见。

### C11 — Controlled Activation

- [ ] 先启用 Dispatcher，但保持 `RAG_WORKER_JOB_STAGES=""`，验证 Request/Job fan-out。
- [ ] 依次启用 `parse`、`passage_embedding`、`purge`，每级均有独立 Kill Switch。
- [ ] Successor 完成后 Cancel Legacy Nonterminal Job，并证明全量 Reconcile 无遗漏。
- [ ] 更新 Preflight，使 Activation 同时要求 migration、Registry、Profile、Generation、
      Credential/Role、Readiness Gate，而非只检查 Env Switch。

### C12 — Verification 与独立 Review

- [ ] 通过 Go Race/Vet、Python Ruff/Format/Mypy/Pytest Coverage、Migration、Docker、
      Compose、Security、Dependency Audit。
- [ ] 通过全部权限、撤权、删除、Provider、Crash、Restore、Resource 负向测试。
- [ ] 用删除前 Backup + 删除后独立 Sealed Ledger 执行 Restore Drill；Ledger 缺失、Gap、
      签名错或未重放时 Readiness 必须 Fail Closed。
- [ ] 独立 xhigh Review 最终 `P0/P1/P2 = 0/0/0`。
- [ ] 记录 Phase 15.2C 实施证据；保持用户 Query 与生产 Promotion 关闭。

## 13. 验证矩阵

| 范围           | 必过验证                                                                                        |
| -------------- | ----------------------------------------------------------------------------------------------- |
| Migration      | Fresh `001→012`、Published `010→011→012`、Down/Up、已有 Generation 时 `011` 拒绝、零部分降级    |
| N/N-1          | N 同时兼容 `010/011/012`；N-1 在 `012` 上拒绝写入；仅 `012.down` 后允许 N-1 恢复流量            |
| Dispatch       | Active+Building/Verified 完整 fan-out、无 Active 初始构建、Effect+Ledger+Ack 原子               |
| Ownership      | 当前全部 Event Type 恰有一个订阅 Owner；未部署 Owner 保持 Pending；RAG 不误领 File/Query Event  |
| Allocation     | Prepare 返回有序全 Generation Plan；Nonce 一次性；漏项/增项/过期/重放/异 Hash 全部失败          |
| Candidate Race | Building→Verified 与 Prepare/Apply 并发时集合重算一致；同 Hash Replay 幂等、异 Hash 隔离        |
| Idempotency    | 同 Key/同 Hash Replay、同 Key/异 Hash 冲突；Stage Head 唯一且 Successor Attempt 可递增          |
| Parser         | TXT/MD/HTML/OOXML/XLSX/CSV/PDF/扫描 PDF、空页、乱码、表格、Locator/Hash Round-trip              |
| Sandbox        | Zip Bomb、XXE、宏、外链、路径穿越、52,428,800 Bytes/500 页及 CPU/RAM/tmpfs/PID 超限             |
| Provider       | Intent 前后 Kill、Submit 响应丢失、Poll/Download/Commit Kill、429/Retry-After、Schema/Hash 错   |
| Object         | Worker 直连 MinIO/公网失败；Capability 错 Job/Lease/Hash/Generation/Operation/重放全部失败      |
| Publish        | Staging 不可见；Manifest/Count/Profile/Consent/Head 任一错即整笔回滚；无双 Head                 |
| Canary         | Verify+Finalize 终结 Attempt 但不改 Head；Crash 前可恢复，Crash 后不可重新 Claim                |
| Race           | Delete/Consent/Governance/Reprocess/Replace 与 Dispatch/Provider/Publish 并发不发布陈旧内容     |
| Purge          | 覆盖所有 Generation/Materialization/Intent；Backlog 下抢占；对象失败可恢复；Source File 不误删  |
| Deletion seal  | 最小 Role、连续 Hash/签名/Key Rotation、旧 Head/Gaps/Store 回滚拒绝、旧 Backup 强制重放         |
| Gateway        | mTLS/SAN、双网卡隔离、One-use Consume、Body 上限、PUT Crash/Orphan Sweep 与 Key Rotation        |
| Generation     | Building 隐藏、Rebuild 可重放、Manifest 确定、旧 Active 在 Crash/Rollback 后继续可用            |
| Secrets/Roles  | Worker/Replay 无 MinIO/Provider Key；Role 只 Execute 指定 Function；日志无正文/Token/Object Key |
| Resource       | 500 文件/100k Child、单 Worker、受限内存/CPU/磁盘下无 OOM；低磁盘停索引不停 Purge               |

## 14. Compose、可观测性与运维门

除 `rag-parser-sandbox` 外，新增服务均使用非 Root、`read_only`、`init: true`、
`cap_drop: [ALL]`、`no-new-privileges`、`ulimits.core=0`、有界 tmpfs/CPU/RAM/PID。
Parser Sandbox 明确使用 `init: false`，由受审计 Supervisor 自身作为 PID 1/Subreaper；
不得再注入 Tini 形成双 Init。Worker 的
`stop_grace_period` 必须大于内部 Shutdown Grace；全部服务配置 Log Rotation。

Phase 15.2C 至少新增：

```text
outbox/job oldest age, lease reclaim, DLQ
provider operation state/latency/429/retry
parse quality/page/block/chunk/artifact bytes
staging/published materialization count and lag
purge oldest age/deadline miss
generation reconcile/manifest mismatch
disk free, RSS, DB pool, gateway rejection by bounded reason
```

Metric Label 禁止用户、文档、正文、URL、Token、Object Key 等高基数或敏感值。C 阶段
要求指标可采集、关键删除/资源告警和新 Volume 本地逻辑 Restore；完整 Dashboard、协调
Backup Manifest、R2 上传与跨 Backup-set Restore Drill 在 Phase 15.2E 收口。

## 15. 回滚策略

按最小影响顺序回滚：

1. 关闭对应 Stage Kill Switch，停止新 Claim；
2. 关闭 Dispatcher，保留 Pending Request/Outbox 供恢复；
3. 隔离/清理未发布 Staging，保留审计 Hash 和 Provider Operation；
4. Corpus Head 不切换，旧 Active Generation/当前兼容应用继续服务；
5. `012` 明确不兼容 N-1 Go Binary：N-1 只认识 Legacy Job，不能读取 Processing
   Request。Migration 撤销 API Role 的 Legacy Job INSERT，并以 Constraint/Function 令 N-1
   Initial/Replace/Reprocess/Delete 在 `012` 上 Fail Closed，禁止产生双 Job/Outbox；
6. `012.down` 采用严格零流量策略：除无 Lease/不可逆 Work/Generation 引用外，还必须证明
   Processing Request、Stage Execution/Attempt、Dispatch Preparation、Provider/Object
   Operation、Deletion Work、已物化 Base/Search Profile、Rebuild Root/Child 与全部 `012`
   专属 Event 均为零；本地 Deletion Authority Ledger 必须保持 `authoritySequence=0`
   Genesis（允许无 Tombstone 的 Freshness Checkpoint），且
   停流量后的独立 Operator Preflight 已通过 Release-pinned Public Key 验证 Off-host Store
   也只有同一 Genesis Head。SQL Migration 不访问网络，只消费有短时 Expiry、Store Head
   Hash 与签名的 Preflight Assertion；
7. 一旦存在任一 Runtime Deletion Authority Entry 或 `authoritySequence > 0`，
   `012.down→N-1` 永久 Fail Closed；Off-host Entry 不可为回滚删除。只能回滚到仍理解
   `012` 的 N Image 并关闭 Kill Switch；
8. migration-owned Approved Registry Static Seed 不要求预先为空，但只可在 Canonical
   Bytes/report hash/checksum 完全匹配且无引用时由 Down 精确删除；Drift、Extra Row 或
   引用均失败；不把运行期数据静默删除或猜测回填为 Legacy Job；
9. 只有上述 Precondition 全部通过才执行 `012.down`；`011.down` 还必须证明无 Search
   Profile/Generation/Projection 引用；
10. 回滚到 N-1 必须先停流量/Worker、满足 Precondition 并执行 `012.down→N-1`；若不能
    Down，只能回滚到仍理解 `012` 的 N Image 并关闭 Kill Switch，不能启动 N-1；
11. 任何不确定状态都停止 Down，恢复新版本执行 Reconcile，而不是手工改表。

Down Preflight Assertion 是 Closed JCS/Ed25519 Envelope，精确绑定：

```text
schemaVersion, jti, issuedAt, expiresAt (<= issuedAt + 5m)
postgresSystemIdentifier, databaseOid, deploymentId
offhostStoreProvider, bucket, prefixHash
migration012Checksum, releaseId, migrateImageDigest
maintenanceFenceId, maintenanceFenceRevision
localAuthoritySequence=0, localGenesisHash
offhostAuthoritySequence=0, offhostGenesisHash
latestCheckpointSequence, latestCheckpointHash, latestCheckpointETag
```

Assertion 由停流量/停 Sealer 后的一次性 `012-down-preflight` 容器签发；其 Offline Operator
Public Key 随 Release 固定，Private Key 不进入 Runtime。它只使用独立
`MIGRATION_DATABASE_URL` 调用 `knowledge_register_012_down_assertion`，不得复用 API/RAG
Role。Maintenance Fence 阻断新 Admission/Seal；注册 Function 重读 Cluster/Database/
Migration/Local Genesis，并以 `jti UNIQUE` 保存未消费 Assertion。`012.down` 在同一
Migration Transaction 内锁 Fence、重验全部 Binding/Expiry/Signature，原子设置
`consumed_at` 后才允许 Drop；Assertion 不能跨 Cluster/Database/Deployment/Store Prefix/
Release 复用。Fence 变化、Off-host ETag 在 Preflight 前后变化、已消费 JTI、旧但未过期
Assertion或任一 Hash 不同都 Fail Closed。

Assertion Wire 复用 Deletion Authority Addendum 的 Raw Ed25519 Public Key/Kid/Base64url
编码和 Closed `{signedPayload,signatures[]}`，但 Domain Tag 固定为
`mm-chat.012-down-preflight.v1\n`；不得与 Checkpoint Signature 互换。

## 16. Definition of Done

Phase 15.2C 完成表示：真实文档可在隔离的 Generation-bound Pipeline 中完成解析、分块、
Passage Embedding、Search Projection Staging/Verify/Atomic Document Publish/Purge；全量
Building Generation 可以从 Postgres + Object Store 确定性重建并保持隐藏；所有权限、
撤权、删除、崩溃和回滚 Gate 通过。

它不表示用户已能检索或聊天已接知识库，也不表示新 Generation 已生产晋升。Phase
15.2D 才实现 Query/Chat/Citation，Phase 15.2E 才完成生产 Promotion、协调备份/R2 和
最终运维验收。
