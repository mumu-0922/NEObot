# Phase 15 RAG Projection Schema 与 Migration Contract

- 状态：Canonical `010/011` contract；`010` 与 durable dark-run Worker 已实现；
  `011` pending；`012` 由 Phase 15.2C Addendum 定义且 pending
- 日期：2026-07-12
- 当前 Schema Head：migration `010_phase15_rag_projection_consistency`
- 待实现迁移：`011_phase15_rag_search_projection` →
  `012_phase15_generation_dispatcher`
- 上位设计：
  [`phase-15-2-single-server-python-rag-consumer-indexing-plan.md`](../architecture/phase-15-2-single-server-python-rag-consumer-indexing-plan.md)
- `012` Addendum：
  [`phase-15-2c-generation-bound-indexing-plan.md`](../architecture/phase-15-2c-generation-bound-indexing-plan.md)

> Migration `010` 已实现本文定义的 extension-independent schema、受限 Functions、
> lease/ledger/replay/purge 一致性机制；Phase 15.2B Python Worker 也已实现为默认不
> claim 事件或 Job 的 durable dark-run Worker。Migration `011` 的 tokenizer、vector、
> BM25/search-extension、Search Projection 和检索 DDL 仍 pending。当前状态不表示真实
> Parser、Embedding、Projection Build/Search、Evidence API 或聊天 RAG 已 Ready。

## 1. 范围与不可越界项

`010` 已建立不依赖 PostgreSQL Extension 的一致性骨架：Profile、Corpus Index
Generation、Document Materialization、Projection Head/State、Parser Artifact、
Canonical Block、Parent/Child Chunk、Applied-event Ledger、Lease/CAS、Collection
Purge Fan-out、原子 Publish/Purge Function 及权限边界。

待实现的 `011` 只承载 Bake-off 晋升后的 Search 物理细节：选定的 BM25 Extension、Dense
存储类型和维度、Tokenizer/Analyzer、Extension-specific Search Projection、索引、
受限检索 Function 与恢复校验。`011` 必须引用 Bake-off 报告中已晋升且带精确版本/
Digest 的唯一 Profile；未晋升时不得编写“临时候选”DDL。

Pending migration `012` 的 Approved Profile Bundle 必须额外绑定：

```text
wire_contract_hash
terms_snapshot_hash
fixture_set_hash
freeze_report_hash
bakeoff_report_hash
```

前三个 Canonical Hash 与 `freeze_report_hash` 只能来自
`lifecycle.state=frozen` Provider Fixture/实际 Freeze Report；
`bakeoff_report_hash` 独立来自晋升后的 Search Evaluation。Draft/Verified/Synthetic Fixture、
Credential Presence 或当前 Governance Row 都不能反向生成这些 Hash，也不能用任一 Report
Hash 替代另一项。Provider Operation Intent 同时保存精确 Wire Contract Hash，外调前与
提交前重验当前 Bundle/Consent。

以下内容**禁止进入 `010`**：

- `CREATE EXTENSION`、`pg_search`、`vector(n)`、`halfvec(n)` 或其他
  Extension-specific 类型；
- Embedding Dimension、HNSW/IVFFlat 参数和索引；
- `pdb.jieba`、Lindera、`chinese_compatible` 或其他 Tokenizer 索引表达式；
- 把 PostgreSQL FTS Baseline 命名为 BM25；
- 任何因尚未结束的 Bake-off 而可能变化的 Search DDL。

Postgres 仍是授权与 Projection 总账本。Search Projection、Redis、MinIO 和 Python
均不能授予 ACL、Consent 或 Processor 权限。

## 2. 术语、ID 与已解决的 Generation 冲突

### 2.1 两层版本轴

此前文档把“全库重建代际”和“单文档重处理代际”都简称 Generation，容易造成
一个 Collection/Document 各自拥有 Active Index Generation 的错误实现。Canonical
定义如下：

1. **Corpus Index Generation**
   - ID：`index_generation_id UUID`；表：`knowledge_index_generations`。
   - 作用域是整个 RAG Corpus，不属于某个 Collection 或 Document。
   - 固定一个完整 Index Profile 和一次全库 Rebuild Boundary。
   - 全库任一时刻最多一个 Active Corpus Index Generation；Parser Major、Chunk
     Policy、Embedding Model/Dimension、Tokenizer/Analyzer、Search Shape 或 Rank
     Contract 变化都必须创建新的 Corpus Index Generation。
2. **Document Materialization**
   - ID：`materialization_id UUID`；表：`knowledge_document_materializations`。
   - `materialization_seq BIGINT` 只在
     `(index_generation_id, document_id)` 内单调递增且永不复用。
   - 表示某个 Document Version 在某个 Corpus Index Generation 内的一次 Parse/
     Chunk/Embed/Reprocess 结果；它不是新的 Corpus Profile，也不触发全库重建。
   - `knowledge_document_projection_heads` 为每个
     `(index_generation_id, document_id)` 指向当前已发布 Materialization。

因此，日常 Upload、Replace、Reprocess 或 Purge 只推进 Document Materialization
Pointer 和 `corpus_projection_revision`；Profile Shape 变化才创建新的 Corpus Index
Generation 并全库重建。旧文档中“一个 Profile 一个 Active Pointer”应解释为“一个
Corpus Head 指向一个 Active Generation”，不得实现成每 Profile/Collection 各自
Active。

### 2.2 ID 与 Hash 规则

- 所有 UUID 由调用方生成；数据库不依赖 UUID Extension。UUID 不表达时间、顺序或
  Revision。
- 所有 `*_revision`、`*_seq` 均为正 `BIGINT`；只能在持锁/CAS Function 内递增，
  不能由 Worker 先读后写。
- `document_version_id` 始终引用 `knowledge_document_versions.id`；
  `materialization_id` 始终引用派生结果；两者不可互换。
- `source_span_hash`、`content_hash`、`manifest_hash`、`result_hash` 和 Profile Hash
  均为小写 64 位 SHA-256 Hex。
- Manifest Hash 由应用对版本化 Canonical JSON Envelope 计算；不得对 PostgreSQL
  `jsonb::text` 直接 Hash。Envelope 必须包含 Contract Version 和所有引用的 Asset/
  Model/Config Hash。
- Object Key 不承载授权，且一经写入不可原地覆盖；新内容必须使用新 Key 和 Hash。

## 3. `010` 的前置兼容修正

### 3.1 Processing Consent 唯一性必须包含 Endpoint 与 Model

`004` 的四个 Partial/Revision Unique Index 原本只包含 `processor`：

- `idx_processing_consents_current_collection_processor`
- `idx_processing_consents_current_query_processor`
- `idx_processing_consents_collection_revision`
- `idx_processing_consents_query_revision`

这会把同一 Processor 的不同 Endpoint 错误压成一个 Consent Namespace。仅加
Endpoint 仍不足够：产品要求 Team Admin 审批 Endpoint/Model Allowlist，同一
OpenAI-compatible Endpoint 可同时批准多个 Model。`010` 已将它们替换为以下
Canonical Key：

```text
current collection: (collection_id, processor, endpoint_id, model_id)
current query:      (user_id, processor, endpoint_id, model_id)
collection history:(collection_id, processor, endpoint_id, model_id,
                    consent_revision)
query history:     (user_id, processor, endpoint_id, model_id,
                    consent_revision)
```

安全迁移顺序固定为：

1. 对 `processing_consents` 取得阻止并发 Consent Write 的表锁；API 在迁移窗口禁止
   新 Consent Mutation。
2. 按 §3.2 先添加 Nullable `model_id`，并仅通过已校验的 Governance Profile
   Mapping 回填每个既有 Consent；Missing/Ambiguous Mapping 使整笔迁移失败。
3. 显式检测上述四个目标 Key 的 Duplicate、空/非 Canonical Endpoint/Model，
   以及当前行多值冲突；任何异常用稳定错误码终止，不自动挑选 Winner。
4. 先创建带 `_endpoint_model` 后缀的新 Unique Index，再删除旧 Index，
   整个过程在一个 Migration Transaction 中完成，不留下无唯一约束窗口。
5. 更新所有 Consent Lookup/Upsert/Supersede SQL，禁止仅按
   `subject + processor` 或 `subject + processor + endpoint_id` 使用 `LIMIT 1`。
6. `010`-aware Runtime 已使用精确 Endpoint/Model 查询；任何旧 Runtime 在升级前都
   必须禁止为同一 Subject/Processor 创建第二个 Endpoint 或 Model Consent。

回滚到 `009` 前必须检测是否已出现同一 `subject + processor` 的多个 Current
Endpoint/Model 或重复 Revision；若存在则拒绝 Down，禁止删除或合并 Consent
History。

### 3.2 Governance Profile 必须有独立 `model_id`

`processor_governance_profiles.model_api_version` 不能同时代表 Model Identity 和 API
Version。`010` 已增加独立、不可空、非空白的 `model_id`，并增加
`profile_contract_hash`。其 Canonical Hash Envelope 至少包含：

```text
processor + endpoint_id + model_id + model_api_version
allowed_purposes + allowed_data_types + region
retention_policy + deletion_contract + training_use
governance_revision + source manifest_hash + contract_version
```

`processor_governance_profiles` 的 Revision/Composite Unique/FK Binding、
`processor_governance_heads` 的 Identity/Active Profile FK、`processing_consents` 的 Governance
Profile FK 和 `knowledge_processing_jobs` 的 Governance Snapshot 都必须携带或可由
受约束 Composite FK 唯一证明同一 `model_id`。Profile Revision Namespace 改为
`processor + endpoint_id + model_id + governance_revision`，不得让一个 Model 的 Revision
阻塞同 Endpoint 下另一个 Model。

Canonical 列迁移不是隐式 Join：`processor_governance_heads` 增加不可空
`model_id`，并将 PK/Head Namespace 改为
`(processor, endpoint_id, model_id)`。Disabled Head 仍是 Model-scoped，只是 Active Profile
为空；同 Endpoint 下可同时有多个 Active Model Head。`processing_consents` 增加
不可空 `model_id`。`knowledge_processing_jobs.model_id` 则必须保持 Nullable：
`parse/passage_embedding` 必须非空，`purge` 必须为空，与已有 Processor/
Governance/Consent Authority Shape 一致。Active Head FK、Consent Profile FK、Job
Profile/Consent FK 均把 `model_id` 纳入同一个 Composite Binding；Runtime 不得只凭
`governance_profile_id` 后补一个未受约束的 Model 字符串。

既有 Profile 不允许把 `model_api_version` 猜成 `model_id`。安全升级必须使用显式、
审计化的 Profile Mapping
`(profile_id, model_id, profile_contract_hash)`，并为每个既有 Head 提供精确
`(processor, endpoint_id, model_id)` Head Mapping：

1. Fresh Database 或零 Profile 数据库无需 Mapping。
2. Published Database 必须在同一 Migration Session 提供覆盖全部既有 Profile
   和 Head 的 Mapping；Active Head 的 Model 必须与其 Active Profile 一致，Disabled
   Head 也不得猜测 Model。Missing、Extra、Duplicate、空值、Hash 不匹配均终止迁移。
3. Migration Owner 在独占锁与单一 Transaction 内临时停用
   `processor_governance_profiles_immutable` Trigger，只回填新增列，立即恢复 Trigger
   并验证其 Enabled；不得改写旧 `manifest_hash` 或旧业务字段。
4. Consent 和 External-processing Job 的 `model_id` 仅从其已约束 Profile Mapping
   回填；Purge Job 保持 `NULL`。Authority Shape CHECK 先修改、后验证，不得为
   Purge 伪造 Processor/Model。
5. Profile/Head/Consent 的 `model_id` 和 `profile_contract_hash` 回填完成后才设为
   `NOT NULL` 并安装新 Composite Constraints；Job 使用 Stage-aware CHECK，不对整列
   设 `NOT NULL`。
6. 映射文件只含 ID/Model/Hash，不含 API Key；它随 Release Evidence 审计，但不得
   作为 Runtime 配置继续存在。

Internal Evidence/Answer Authorization 必须精确匹配：

```text
processor + endpoint_id + model_id + purpose='answer'
+ active governance head/profile/revisions/profile_contract_hash
+ active collection answer consent
+ active user query answer consent
```

任何一个字段不同、旧 Runtime 未提交 `model_id`、Profile Hash 不同或 Consent 已
过期/撤销，都 Fail Closed。Endpoint 相同不代表 Model 获批，持有 BYOK Credential
也不代表获得 Evidence Egress 权限。

## 4. `010` Canonical Schema

下列是逻辑 DDL Contract。实现可调整无语义差异的 Constraint/Index 名，但表名、
Identity、Reference、状态机和唯一性不得漂移。

### 4.1 Immutable Base Index Profile

`knowledge_index_profiles` 冻结 Extension-independent Materialization Contract：

```text
id UUID PK
contract_version SMALLINT NOT NULL
canonical_schema_version TEXT NOT NULL
parser_manifest JSONB NOT NULL + parser_manifest_hash TEXT NOT NULL
chunk_manifest JSONB NOT NULL + chunk_profile_hash TEXT NOT NULL
embedding_processor/embedding_endpoint_id/embedding_model_id TEXT NOT NULL
embedding_api_version/embedding_role TEXT NOT NULL
rerank_processor/rerank_endpoint_id/rerank_model_id TEXT NOT NULL
rerank_api_version TEXT NOT NULL
base_profile_hash TEXT NOT NULL UNIQUE
created_at TIMESTAMPTZ NOT NULL
```

JSON 必须是 Object；文本非空；Hash 格式固定。该表不保存 Dimension、Postgres
Vector Type、BM25 Extension 或 Tokenizer Index。`BEFORE UPDATE OR DELETE` Trigger
无条件拒绝 Mutation；修订配置只能 INSERT 新 Profile。

`011` 将添加独立、同样不可变的 `knowledge_search_profiles`，并由 Generation 的完整
Profile Binding 组合 Base/Search Hash。`010` 不 Seed Profile/Generation；当前
durable dark-run Worker 也不 Claim 或执行真实 Stage。创建首个可服务
`verified/active` Generation、启用真实 Parser/Embedding/Projection Runtime 前必须先
实现并应用 `011`，再通过对应 Promotion Gate。

### 4.2 Corpus Index Generation 与 Corpus Head

`knowledge_index_generations`：

```text
id UUID PK
index_profile_id UUID NOT NULL FK -> knowledge_index_profiles(id) RESTRICT
generation_seq BIGINT NOT NULL UNIQUE
status TEXT NOT NULL
build_snapshot JSONB NOT NULL
build_snapshot_hash TEXT NOT NULL
artifact_manifest_hash TEXT
failure_code TEXT
created_at/verified_at/activated_at/retired_at/failed_at TIMESTAMPTZ
```

状态为 `building | verified | active | retired | failed`。Partial Unique Index 保证全库
最多一个 `active`，并最多一个 `building/verified` Candidate。状态时间戳必须匹配；
`active` 必须已 Verified，`failed` 必须有稳定 Error Code。Profile、Build Snapshot 和
Generation Identity 不可更新；仅受限 Function 能推进状态。

`knowledge_corpus_projection_head` 是 Singleton Row，主键固定为 `singleton_id=1`：

```text
active_index_generation_id UUID NULL
corpus_projection_revision BIGINT NOT NULL >= 1
head_revision BIGINT NOT NULL >= 1
updated_at TIMESTAMPTZ NOT NULL
```

它是“当前全库 Active Generation”的唯一 Pointer。`active_index_generation_id=NULL`
表示尚无可服务 Projection；不得由 Runtime 解释为“选择最新 Generation”。所有切换
只能由 `knowledge_promote_index_generation(...)` 完成。

`knowledge_projection_state` 每个 Corpus Generation 一行：

```text
index_generation_id UUID PK FK RESTRICT
readiness TEXT: building | catching_up | ready | degraded | retired | failed
projection_revision BIGINT NOT NULL
required_outbox_floor BIGINT NOT NULL
contiguous_applied_outbox_id BIGINT NOT NULL
manifest_hash TEXT
document_count/parent_count/child_count BIGINT NOT NULL >= 0
verified_at/updated_at TIMESTAMPTZ
```

`contiguous_applied_outbox_id` 只是经过 Ledger 证明的连续前缀优化，绝不能用
`MAX(knowledge_outbox.id)` 计算 Readiness。`ready` 还必须通过 Applied Ledger
Anti-join、Manifest/Count/Reconciliation 和全部 Tombstone Fence。

### 4.3 Per-document Materialization 与 Projection Head

`knowledge_document_materializations`：

```text
id UUID PK
index_generation_id UUID NOT NULL FK RESTRICT
collection_id/document_id/document_version_id/file_id UUID NOT NULL
materialization_seq BIGINT NOT NULL
parse_artifact_set_id UUID NULL
source_content_hash/base_profile_hash TEXT NOT NULL
collection_acl_revision/collection_visibility_epoch BIGINT NOT NULL
collection_processing_revision/document_visibility_epoch BIGINT NOT NULL
status TEXT NOT NULL
manifest_hash/result_hash/failure_code TEXT NULL
created_at/verified_at/published_at/retired_at/purged_at TIMESTAMPTZ
UNIQUE(index_generation_id, document_id, materialization_seq)
UNIQUE(index_generation_id, document_id, id)
```

Composite FK 必须证明 Collection→Document、Document→Version→File 均与 `001`–`009`
权威行一致。状态为
`staging | verified | published | retired | failed | purging | purged`。
`published` 仅表示它是该 Generation 内 Document Head 当前指向的版本；是否对 Query
可见仍取决于 Corpus Head、权威 Document Current Version 和全部 Fence。

`knowledge_document_projection_heads`：

```text
index_generation_id UUID NOT NULL
document_id UUID NOT NULL
active_materialization_id UUID NULL
document_projection_revision BIGINT NOT NULL >= 1
last_corpus_projection_revision BIGINT NOT NULL >= 1
updated_at TIMESTAMPTZ NOT NULL
PK(index_generation_id, document_id)
composite FK(index_generation_id, document_id, active_materialization_id)
```

Pointer 只允许指向同一 Generation/Document 的 `published` Materialization。普通
Reprocess 在同一 Corpus Generation 内创建新 `materialization_seq` 并 CAS 切换该
Pointer；不得创建伪 Corpus Generation。旧 Materialization 先 Retire、后异步 Purge。

### 4.4 Parser Artifact Set 与 Artifact

`knowledge_parser_artifact_sets` 为一次可验证 Parse Result 的根：

```text
id UUID PK
document_id/document_version_id/file_id/index_profile_id UUID NOT NULL
parser_kind/parser_version TEXT NOT NULL
source_content_hash/config_hash/manifest_hash TEXT NOT NULL
status TEXT: staging | verified | quarantined | purging | purged
quality_report JSONB NOT NULL
created_at/verified_at/purged_at TIMESTAMPTZ
```

`knowledge_parser_artifacts` 保存不可变 Object Manifest Item：

```text
id UUID PK
artifact_set_id UUID NOT NULL FK RESTRICT
artifact_kind TEXT NOT NULL
object_key TEXT NOT NULL UNIQUE
content_type TEXT NOT NULL
byte_size BIGINT NOT NULL >= 0
sha256 TEXT NOT NULL
page_or_part_ref JSONB NULL
created_at TIMESTAMPTZ NOT NULL
UNIQUE(artifact_set_id, artifact_kind, object_key)
```

`artifact_kind` 至少允许 `parser_native | canonical_ir | quality_report | page_asset`。
Parser-native、Canonical IR、Quality Report 和 Page Asset 必须分项记录，不能只保存
一个可覆盖目录 Prefix。`rag_api_reader` 无权读取该表或 Object Key。

### 4.5 Canonical Blocks

`knowledge_blocks` 是可重建 Chunk 的权威 Canonical IR Row：

```text
id UUID PK
artifact_set_id/document_id/document_version_id UUID NOT NULL
parent_block_id UUID NULL
ordinal BIGINT NOT NULL
block_type TEXT NOT NULL
heading_path TEXT[] NOT NULL
text_content/markdown_content/html_content/latex_content/code_content TEXT NULL
table_data JSONB NULL
locator_kind TEXT NOT NULL
locator JSONB NOT NULL
reading_order BIGINT NOT NULL
provenance JSONB NOT NULL
confidence NUMERIC NULL
content_hash/source_span_hash TEXT NOT NULL
derived/non_indexable/needs_review BOOLEAN NOT NULL
UNIQUE(artifact_set_id, ordinal)
UNIQUE(artifact_set_id, source_span_hash)
```

Parent FK 必须限定在同一 `artifact_set_id`。至少一种 Content 表达存在；`table_data`、
`locator`、`provenance` 必须是 JSON Object。`locator_kind` 固定为：

```text
text_offset | line_range | page_bbox | slide_shape | sheet_cell |
ooxml_part_xpath
```

Locator 的 Discriminator/必填字段由 Contract Version 校验；BBox、Line、Offset、Page、
Slide、Cell 范围必须非负且有序。质量失败的 Artifact Set 只能 Quarantine，不能产生
Published Materialization。

Migration `010` 只冻结上述 Locator Shape，不足以证明 Provider 坐标语义。`012` 必须从
[`provider-wire-fixture.md`](../contracts/provider-wire-fixture.md) 固定
`page_index_basis/bbox_order/coordinate_unit/origin/axis_direction/bounds/rotation_semantics/
normalization_version` 与 Conversion Hash；未知或漂移时禁止 Publish/Citation。

### 4.6 Parent/Child Chunk 与 Provenance

`knowledge_parent_chunks`：

```text
id UUID PK
materialization_id/index_generation_id/document_id/document_version_id UUID NOT NULL
ordinal BIGINT NOT NULL
chunk_profile_hash/source_span_hash/content_hash TEXT NOT NULL
content TEXT NOT NULL
token_count INTEGER NOT NULL > 0
heading_path TEXT[] NOT NULL
locator_summary JSONB NOT NULL
UNIQUE(materialization_id, ordinal)
UNIQUE(index_generation_id, document_version_id, source_span_hash,
       chunk_profile_hash)
```

`knowledge_child_chunks`：

```text
id UUID PK
parent_chunk_id/materialization_id/index_generation_id UUID NOT NULL
document_id/document_version_id UUID NOT NULL
ordinal BIGINT NOT NULL
chunk_profile_hash/source_span_hash/content_hash TEXT NOT NULL
content TEXT NOT NULL
token_count INTEGER NOT NULL > 0
overlap_before_tokens/overlap_after_tokens INTEGER NOT NULL >= 0
UNIQUE(materialization_id, ordinal)
UNIQUE(index_generation_id, document_version_id, source_span_hash,
       chunk_profile_hash)
```

Composite FK 必须证明 Child 的 Parent、Materialization、Generation、Document 和
Version 完全一致。`knowledge_chunk_block_spans` 以
`(chunk_kind, chunk_id, block_id, span_ordinal)` 保存 Block 起止 Offset，保证 Citation
能从 Chunk 无歧义回溯 Source Block/Locator。

`010` 的 Chunk 不含 Dense Vector、BM25 类型或 Tokenizer Index。所有 Row 的可见性
由 Materialization/Document Projection Head 控制，禁止每行一个可被 Worker 任意
修改的 `active` Boolean。

### 4.7 Applied-event Ledger

`knowledge_outbox_applied_events` 固定字段：

```text
consumer_name TEXT NOT NULL
event_id UUID NOT NULL FK -> knowledge_outbox(event_id) RESTRICT
scope_kind TEXT NOT NULL: global | generation
index_generation_id UUID NULL FK -> knowledge_index_generations(id) RESTRICT
generation_scope_id UUID GENERATED ALWAYS AS
  (COALESCE(index_generation_id,
   '00000000-0000-0000-0000-000000000000'::UUID)) STORED
result_hash TEXT NOT NULL
outbox_id BIGINT NOT NULL
applied_at TIMESTAMPTZ NOT NULL
PK(consumer_name, event_id, generation_scope_id)
```

Global Scope UUID 固定为 `00000000-0000-0000-0000-000000000000`；
`scope_kind=global` 时 `index_generation_id IS NULL` 且 Scope ID 必须为该常量；
`scope_kind=generation` 时 Scope ID 必须等于真实、非零 `index_generation_id`。
不得依赖普通 Unique 对多个 `NULL` 的行为。

`010` 同时为 `knowledge_outbox(id, event_id)` 增加 Composite Unique Constraint，Ledger
使用 `FOREIGN KEY(outbox_id, event_id)` 引用它，使数据库证明两列来自同一
Outbox Row，禁止把合法 `event_id` 与另一行的 Allocation Cursor 拼接。该
Cursor 仍不代表 Commit Order。

Replay 时同一 PK + 同一 `result_hash` 是幂等成功；同一 PK + 不同 Hash 必须隔离并令
Projection Unready。Checkpoint 以下仍按 Ledger Anti-join 回扫，不能假设 Outbox ID
等于 Commit Order。

### 4.8 Outbox 与 Job Lease/CAS Fencing

`010` 为 `knowledge_outbox` 增加：

```text
lock_owner UUID
lock_token UUID
lock_expires_at TIMESTAMPTZ
```

`processing` 必须三者非空且 `lock_expires_at > locked_at`；其他状态三者为空。
Migration 应先检测基线中是否存在无法证明 Owner/Token 的 `processing` Row：存在即
终止，不制造 Token、不静默重置。Claim/Reclaim 每次使用新的不可复用 Token；Ack
必须匹配 `event_id + status=processing + lock_owner + lock_token`。

`010` 为 `knowledge_processing_jobs` 增加：

```text
lease_token UUID
index_generation_id UUID
materialization_id UUID
model_id TEXT NULL
legacy_projection_unbound BOOLEAN NOT NULL
```

Parse/Passage Embedding Job 必须绑定真实 Generation 与 Materialization；Purge Job
绑定待清理 Generation，Materialization 可按 Purge Scope 为空。与 `006`
Authority Shape 一致，Parse/Passage Embedding 必须携带非空且被 Composite FK
约束的 `model_id`，Purge 必须使 `processor/endpoint/governance/consent/model_id`
全部为空。不允许对 `model_id` 整列施加 `NOT NULL`。`processing` 必须有
`lease_owner + lease_token + lease_expires_at`，其他状态全部为空。Heartbeat、Success、
Fail、Cancel 都必须 CAS：

```text
id + status='processing' + lease_owner + lease_token
+ lease_expires_at > database_now
```

Provider 调用前与提交前另行重验权威 Snapshot。过期 Worker 即使持有旧 Owner ID、
结果 Hash 或 Provider Job ID，也不能覆盖新 Lease。

`009` 可能已有尚未消费、因当时不存在 Generation 而无法安全 Backfill 的 Job。
`010` 不得猜一个 Generation，也不得删除这些历史：Migration 在确认没有
`status='processing'` 后，把既有 Row 标记 `legacy_projection_unbound=true`；新写入
默认并强制为 `false` 且满足上述 Generation/Materialization Shape。Claim Function
永远排除 Legacy Row。`011` 和首个 Generation Ready 后，Reconciliation 在一个
Transaction 中从权威 Document/Outbox 重建 Generation-bound Successor Job，并将
对应 Legacy Nonterminal Job Cancel；失败则保留 Legacy Row 和 Unready 状态。完成
Reconciliation 后不得再产生 Legacy Row。

### 4.9 Durable Collection Purge Fan-out

`knowledge_collection_purges` 保存 Fan-out Root：Collection ID、Tombstone
`collection_visibility_epoch`、Source Event ID、枚举 Cursor、`enumeration_complete`、
状态、Attempt、Lease Owner/Token/Expiry、Remaining Count Snapshot、Error Code 和时间戳。
枚举 Cursor 是
`(document_id, document_version_id, index_generation_id, materialization_id)`；最后一维
用于在同 Version/Generation 存在 Reprocess Materialization 时稳定选择 Active Head
（无 Head 时选最新 Seq）并保持分页边界确定。

`knowledge_collection_purge_items` 至少包含：

```text
purge_id/collection_id/document_id/document_version_id UUID NOT NULL
collection_visibility_epoch BIGINT NOT NULL
index_generation_id UUID NOT NULL
materialization_id UUID NULL
status TEXT: pending | processing | succeeded | failed
attempt_count/max_attempts INTEGER NOT NULL
lease_owner/lease_token UUID NULL
lease_expires_at/completed_at TIMESTAMPTZ NULL
error_code TEXT NULL
UNIQUE(collection_id, collection_visibility_epoch,
       document_version_id, index_generation_id)
```

Fan-out 分页只按稳定 Composite Cursor 前进；Insert 使用上述 Unique Key 幂等。Root
只有在 `enumeration_complete=true` 且不存在非 `succeeded` Item 后才能 Complete。
Worker Crash、Redis Flush 或重复 Collection Tombstone 后必须从 Root/Item 表恢复。
Online Payload Purge 的 15 分钟 SLO 从 Tombstone Commit 计时；超时告警不恢复逻辑
可见性。该 SLO 不表示 WAL、Backup、Snapshot、Postgres Free Page 或 SSD Block 已完成
Retained-copy Expiry/Disk-forensic Erasure；完整状态与术语以 Phase 15.2C Offline Parser/
Canonical IR Addendum §15.1 为准。

## 5. SECURITY DEFINER 原子 Function

所有 Function 由专用 NOLOGIN Owner 持有，`SECURITY DEFINER`，固定安全
`search_path` 并对所有对象使用 Schema Qualification；`PUBLIC EXECUTE` 必须撤销。
Function 不接受调用方提交的 ACL、Owner、Team、Revision 或状态作为授权事实，只将
Expected Value 用于 CAS，并从权威表重读实际值。

### 5.1 Claim/Ack Functions

- `knowledge_claim_outbox(worker_id, lock_token, lease_seconds, limit)`：使用
  `FOR UPDATE SKIP LOCKED` Claim Pending/Expired Row；每次 Reclaim 替换 Token。
- `knowledge_apply_and_ack_outbox(...)`：同一 Transaction 内创建/确认 Job 或
  Tombstone Work、写 Applied Ledger、按 Token Ack Published；Result Hash 冲突失败。
- `knowledge_claim_processing_job(...)`、`knowledge_heartbeat_processing_job(...)`、
  `knowledge_finish_processing_job(...)`：全部执行 Lease Token CAS。

Dispatcher Function 不调用外部 API，不在 Outbox Lock Transaction 中执行 Parse、
Embedding 或 Object IO。

### 5.2 Publish Materialization

`knowledge_publish_materialization(...)` 是 Document Publish 的唯一入口。固定锁序：

```text
Collection -> Document -> Old/New Document Version -> Corpus Generation
-> Document Projection Head -> Processing Job
```

Function 必须验证：

- Job ID/Owner/Lease Token 未过期，且绑定同一 Generation/Materialization；
- Generation 是当前 Active 或唯一 Building Candidate，Profile/Hash 完全匹配；
- Materialization 已 Verified，Manifest、Artifact、Block、Chunk Count/Hash 完整；
- File/Version Content Hash 与 Job Snapshot 一致；
- Collection/Document 未删除，ACL/Visibility/Processing Revision 未变化；
- `current_version_id`、旧/新 Version Status、Document Visibility Epoch 符合 Expected
  CAS；
- Consent、Governance Head/Profile、`model_id`、Purpose 和各 Revision 在提交时仍
  Active。

在同一 Transaction 内，Function 才能：切换 Document Projection Head、推进
Materialization Status、递增 Corpus/Document Projection Revision、更新 Projection State、
结束 Job，并写 Outbox/Audit Effect。Initial/Replace/Replacement Retry 这类冻结了
`advance_current_version=true` 的权威 Version Activation 才同时 CAS Version/Document
Current Pointer；Active Version Reprocess 和 Building Generation Catch-up 必须验证且
保持当前 Pointer。单文档 Publish 不得切换 Corpus Head。任一
检查失败整笔回滚；Staging Chunk 不得部分可见。

Building Generation 的 Document Publish 只更新其隐藏 Head；只有全部 Live Document
完成、Replay 追平并验证后，`knowledge_promote_index_generation(...)` 才能原子切换
Singleton Corpus Head、把 New Generation 置 Active、Old Generation 置 Retired 并
递增 Head/Projection Revision。失败时旧 Active Generation 继续服务。

### 5.3 Purge Projection

`knowledge_purge_materialization(...)` 或 Document-scope Wrapper 是 DB Purge 的唯一
入口。它必须锁定 Collection→Document→Version→Generation→Head→Purge Item/Job，
验证 Tombstone/Visibility Epoch 只前进不回退，验证 Lease Token，并先清除/切换
Projection Head 后标记 Materialization/Artifact Purging/Purged、递增 Projection
Revision、完成 Item/Job。

外部 Object 删除不能伪装成同一 DB Transaction。Function 先持久化精确 Object
Manifest 与 Deletion Work；Go 使用受限 Capability 删除后，以 Manifest Hash CAS
确认。失败保持 Retryable Purging，绝不重新可见。Source File 仍遵循独立 File
Delete Contract，Document/Collection Purge 不自动删除 Source Object。

### 5.4 Go-only Reauthorization/Hydration

`knowledge_reauthorize_and_hydrate_evidence(...)` 是 Go 在 Python 返回 Source
Reference 后的唯一 DB Hydration 入口。它是 Extension-independent `010`
`SECURITY DEFINER` Function，只 Grant 给独立 NOLOGIN Group Role
`go_evidence_hydrator`，不 Grant 给 `rag_api_reader` 或 Worker。

Function 只接受 Go 从当前 Auth Context 取得的
`actor_user_id + session_id + conversation_id`、原请求 Authorization/Profile Fence，
以及最多 16 条精确
`collection/document/version/generation/materialization/parent/child/span-hash` Reference。它必须在
同一 Statement/Transaction Snapshot 内从权威表重读并验证：

- Session Active、Session User 与 Actor 一致，且 Conversation 归属/Team Membership 当前有效；
- Collection Scope/Owner/Team、ACL/Visibility/Processing Revision、Document/Version
  Active Current Pointer 和 Document Visibility Epoch；
- Corpus Head/Generation/Projection Revision、Document Projection Head 与精确 Published
  Materialization；
- Parent/Child/Block Span 归属关系、`source_span_hash/content_hash` 和返回数量/字节
  上限。

它只返回已重新授权的有界 Source Text、Locator、Hash 和构建 Citation 所需的
最小 Display Metadata；不返回 Object/Bucket Key、Credential、未选中 Block 或任意
Projection Row。如必须从 Source Object 补充内容，只能由受限 Object Gateway
内部解析精确 Ref 并返回 Bytes/Stream；Go Runtime 和该 Function 都不暴露 Raw
Object Key。无权或 Stale Ref 整条不返回，Go 仍必须将缺失视为撤权竞态并在
Answer Egress/Commit 前再验。

## 6. `011` Bake-off-selected Search Schema

`011` 只有在 Extension License、Crash/WAL Recovery、Logical Restore、中文
Tokenizer、ACL-in-query、资源、升级和回滚 Gate 全部通过后才能冻结 SQL。迁移名和
对象职责现在冻结，具体 Winner 值由签名 Bake-off Report 填入。

### 6.1 Immutable Search Profile

`knowledge_search_profiles` 必须包含并冻结：

```text
id UUID PK
index_profile_id UUID NOT NULL FK RESTRICT
engine_family/engine_version/extension_name/extension_version TEXT NOT NULL
embedding_processor/embedding_endpoint_id/embedding_model_id TEXT NOT NULL
embedding_api_version/embedding_role TEXT NOT NULL
embedding_dimensions INTEGER NOT NULL
dense_storage_type/distance_metric TEXT NOT NULL
lexical_engine/tokenizer/analyzer TEXT NOT NULL
tokenizer_config/analyzer_config/rank_config JSONB NOT NULL
extension_image_digest/bakeoff_report_hash/search_profile_hash TEXT NOT NULL
created_at TIMESTAMPTZ NOT NULL
```

它与 Base Profile 一样拒绝 UPDATE/DELETE。`knowledge_index_generations` 在 `011`
增加不可空 `search_profile_id` 和 `combined_profile_hash`；任何 Search 字段变化创建新
Corpus Generation。`011` 对已有 Generation 的安全前提是 `010` 尚未 Seed/Activate
任何 Generation；若发现数据则终止而非猜测 Backfill。

### 6.2 Search Projection 与 Query Surface

`knowledge_child_search_projections` 一对一引用 `knowledge_child_chunks.id`，保存
Winner-specific Dense/Lexical/Exact 字段。Dense 类型、Dimension、Lexical 类型、
Tokenizer Expression、BM25/HNSW/Exact Index 和参数必须逐字来自 Bake-off Report，
不得使用 `latest` 或浮动默认值。

所有 Lane 必须在 Lane 内部、Top-K 之前应用同一过滤：Active Corpus Generation、
Document Projection Head/Materialization、Collection/Document/Version Active State、
Current Version、ACL Revision、Collection/Document Visibility Epoch 和请求携带的
最小 Projection Revision。Exact Lane 不得绕过这些 Join。

`011` 创建以下两类参数化 `SECURITY DEFINER` Function，固定安全 Owner 并撤销
`PUBLIC EXECUTE`；`rag_api_reader` 在 Phase 15.2D 的 forward migration 前不得 Execute，
且始终不得 `SELECT` Base Table：

1. `knowledge_search_evidence_candidates(...)` 校验精确
   `actor/session/collection snapshot/profile/generation/projection` Fence，在每条 Lane 的
   Top-K 前过滤，并返回 Candidate Reference/Span/Score 与 **仅供 Python
   请求内部使用**的有界 Child Source Text。
2. `knowledge_expand_evidence_candidates(...)` 只接受第一个 Function 返回的精确
   Materialization/Chunk Ref 及同一 Authorization/Profile Fence，逐个 Parent/Window
   重验同样权威状态，并返回有界 Expansion Text 与最终 Span/Locator。

两者都必须验证 Active Session 归属 Actor，对 Candidate Count、单条 Text Bytes、
总 Text Bytes、Expansion Window 和执行时间使用 Immutable Profile/Server Hard Cap；禁止
调用方提高上限或请求任意 Chunk/Block ID。Source Text 只能用于当次 RRF/
Jina Rerank/Expansion，不落盘、不进日志/缓存，也不出现在 Python→Go HTTP
Response。Function 不返回 Object Key、Credential、Email/Display Name、Consent
Payload 或“已授权”结论。

这个内部 Text Surface 是 Rerank/Expansion 的必要最小权限，不改变 External
Evidence API 的 Source-reference-only Contract。Python 必须在把 Candidate Text 发给
Jina 前验证 `purpose=rerank` 的精确 Processor/Endpoint/Model Governance、User
Query Consent 和所有涉及 Collection Consent，并在结果纳入前再验一次。

## 7. Least-privilege Roles

角色边界固定如下；Login Role 与密码由部署层创建。`010` 在 Migration Owner 具备
`CREATEROLE` 时创建缺失的 NOLOGIN Capability Role，否则 Bootstrap 必须预先创建；
Migration 始终验证其受限属性后再 GRANT，不能回退到共用 Owner Credential。

| Role                   | 允许                                                                                     | 禁止                                                               |
| ---------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Migration Owner        | DDL、Extension、Owner/GRANT                                                              | 作为 Go/Python Runtime Credential                                  |
| `rag_projection_owner` | 持有受限 SECURITY DEFINER Function                                                       | LOGIN；业务直连                                                    |
| `rag_worker_executor`  | EXECUTE `010` Claim/CAS/Publish/Purge/Readiness Functions                                | Base Table DML；ACL/Consent/Document/Corpus Head 直写；读取 Secret |
| `rag_api_reader`       | EXECUTE `010` Worker Readiness；Phase 15.2D forward migration 才增加 Candidate/Expansion | Base Table SELECT；任何 DML；Object Store Credential               |
| `go_evidence_hydrator` | EXECUTE `010` exact-ref Reauthorization/Hydration Function                               | Base Table/Object-Key SELECT；任意 Projection 扫描；任何 DML       |
| Go API Runtime         | 权威业务 Mutation；显式 EXECUTE `010` Readiness/Hydration，无 Role Membership            | Extension DDL；以 Python 身份执行；读取 Raw Object Key             |

默认 `REVOKE ALL ... FROM PUBLIC`。Schema `CREATE`、Function Owner Membership、
`SET ROLE`、Trigger Disable、Sequence Write 和 Migration Table Write 均不得授予 Python
Role。Artifact/Chunk 在 Published 后由 Trigger/Function 防止 Worker 直接 UPDATE/DELETE。

## 8. 强制不变量

1. 全库最多一个 Active Corpus Index Generation，且 Singleton Head 与其一致。
2. 一个 Document 在一个 Corpus Generation 内只有一个 Projection Head；Head 只能指向
   同 Document/Generation 的 Published Materialization。
3. `materialization_seq` 单调且不复用；UUID 不参与顺序判断。
4. Query 只认 Corpus Head + Document Head + 当前权威 Version/Fence；仅有
   `status='published'` 不足以可见。
5. Staging/Failed/Retired/Purging/Purged Row 永不进入 Query。
6. Profile、Artifact、Block、Chunk 和已发布 Manifest 不原地改写；变更创建新 Identity。
7. 旧 Visibility Epoch、旧 Lease Token、旧 Consent/Governance/Profile Hash 不能完成
   Publish 或复活 Tombstone。
8. Applied Ledger Scope 永不为 NULL；相同 Event/Scope 的不同 Result Hash 必须隔离。
9. Outbox ID 不是 Commit Order；Readiness 必须回扫并对账。
10. Collection Purge Root 未完成枚举或仍有非成功 Item 时不能 Complete。
11. `processor + endpoint_id + model_id + purpose` 必须精确授权；Credential Presence
    不是 Consent。
12. Source Object 删除与 Projection Purge 分离；任何异步失败都不能恢复逻辑可见性。

## 9. Migration、发布与回滚

### 9.1 Up 顺序

1. 备份并记录 migration `001`–`009` 精确 Checksum；停止 Worker/Consent Mutation。
2. 执行 Consent Duplicate/Endpoint Preflight 和 Governance Model Mapping Preflight。
3. 在一个 Transaction 中完成 §3 兼容修正、Role/Privilege 验证、`010` Tables/
   Constraints/Triggers/Functions；不 Seed Profile/Generation。
4. 运行 Fresh `001→010`、Published `009→010`、Down/Up Replay 和权限负向测试。
5. Bake-off 晋升后，用固定 Extension/Image Digest 在新 Postgres Volume 做逻辑迁移，
   应用 `011`；不得让不同发行版共用旧 PGDATA。
6. `011` 成功且固定镜像新 Volume 的本地逻辑 Restore/Crash Drill 通过后，`012` 才能由
   受限 Operator Function 创建首个 Profile/Building Generation 并全量 Build；协调
   Postgres+Object Manifest 的 R2 Restore Drill 仍是 Phase 15.2E 生产 Promotion Gate。
7. 新 Generation Verified/Ready 前，现有非 RAG 功能继续；不得宣称 RAG Ready。

### 9.2 Down 与应用回滚

- 应用回滚优先切回已保留、仍满足当前 Tombstone/Consent Fence 的旧 Active Corpus
  Generation；绝不能切回遗漏删除事件的 Snapshot。
- `011.down` 先撤销 Search Function/GRANT 和 Winner-specific Index，再删除 Search
  Projection/Profile/Extension Object。仍有 Generation 引用时必须拒绝 Down。
- `010.down` 只可删除派生 Projection 数据；不得删除 Document、Version、Consent、
  Governance History、Outbox 或 Source File。
- 回滚 Lease 列前必须无 `processing` Outbox/Job/Purge Item；否则拒绝。
- 回滚 Consent Index 前执行 §3.1 冲突检测；存在多 Endpoint/Model
  Current/Revision 时拒绝。
- 回滚 `model_id/profile_contract_hash` 前必须确认没有 Post-010 Profile、Consent、Job
  或 Evidence Authorization 依赖；禁止丢字段后继续 Answer。
- Down 按 FK/Function/Trigger/Grant/Index/Table 的反向依赖顺序执行，并通过再次 Up
  验证。任何 Down Precondition 失败都保留新 Schema，不做部分降级。

## 10. Required Tests

`010` 和 durable dark-run Worker 的 Migration、权限、lease/ledger/replay、恢复与
默认不 Claim 行为已有实现测试；以下涉及 `011` Search DDL、`012` Dispatcher、真实
Parser/Embedding 或 Projection Ready 的条目仍是后续 Acceptance，不得解释为当前已通过。

### 10.1 Schema 与 Migration

- Fresh `001→010→011→012`、Published `009→010→011→012`、逐个 Down/Up、No-op Replay、
  Checksum/Name Drift Fail Closed。
- Consent 新四个 Unique Key、旧 Index 消失、跨 Endpoint 可共存、同 Endpoint
  的多 Model 可共存、同 Endpoint/Model Current/Revision 冲突失败；Down Conflict
  拒绝且不删 History。
- Existing Governance Mapping 全覆盖成功；Missing/Extra/Duplicate/Hash 错误失败并整笔
  Rollback；Profile/Head/Consent 的 `model_id` 非空，多 Model Head 可共存，且
  Immutable Trigger 恢复。External-processing Job 必须有约束的 `model_id`，Purge
  Job 必须为空；既有 Purge Row 迁移不失败也不伪造 Model。
- `010` SQL 静态断言不含 Extension、Vector Dimension、pg_search 或 Tokenizer Index。
- Composite FK、状态/时间戳 Shape、Hash/JSON Shape、Singleton 和 Partial Unique 均由
  PostgreSQL Constraint 实测，不只做文本断言。
- `012` Event Subscription 覆盖全部 Event Type；Dispatch Preparation/Nonce、多 Generation
  Allocation、Stage Execution/Attempt Replay、Profile Bundle、Rebuild Root/Child Event、
  Object Intent/Orphan Sweep 与 N-1 Fail-closed/Down 顺序均通过正负测试。

### 10.2 Generation 与 Materialization

- 两个 Active Corpus Generation 并发 Promotion 只能成功一个。
- 同 Profile Reprocess 只生成新 Materialization/Seq，不生成 Corpus Generation。
- Parser/Chunk/Search Profile 变化无法混写现有 Generation。
- Building Generation 可逐文档 Catch-up，但在全量对账前 Query 不可见；Promotion
  原子切换全库 Head。
- Publish Crash、Manifest Mismatch、Current Version Race、Delete/Consent/Governance
  Race 均整笔回滚，旧 Head 保持可服务。

### 10.3 Replay、Lease 与 Purge

- Duplicate、Out-of-order、Late-low-ID、High-ID-first、Crash-before-Ack、Effect-commit/
  Ack-fail、Redis Flush 均不漏事件。
- 相同 Ledger PK/Hash 幂等；不同 Hash 隔离；Global/Generation Scope Shape 强制。
- Outbox、Job、Purge Item Lease Expiry 可回收；旧 Token Heartbeat/Finish/Publish/Purge
  全部影响零行并返回稳定 Stale Lease Error。
- Collection Fan-out 分页 Crash/Retry 不跳 Item；只有枚举完成且全部 Item Success 才
  Complete；Tombstone 后立即查询不可见。
- Purge Object 删除失败可重试且不复活 Projection；Source File 未被隐式删除。

### 10.4 Parser、Chunk、Search 与权限

- 每种 Locator Union、Parent Block、Chunk→Block Span 可无歧义回溯；跨 Artifact Set/
  Document/Generation FK 失败。
- Published Artifact/Block/Chunk 不能由 Worker 直接改写；Hash/Count 对账可重建。
- `011` 每条 Dense/BM25/Exact Lane 在 Top-K 前应用相同 Fence，并与 Exact Baseline/
  Golden Set、低选择率 Personal/Team ACL Slice 对账。
- `rag_api_reader` 不能 SELECT Base Table、读取 Object Key/Credential 或 DML；
  `rag_worker_executor` 不能直接改 ACL、Consent、Document Pointer、Corpus Head 或
  Outbox 终态；`PUBLIC` 无 Execute。
- Candidate/Expansion Function 只返回有界、已应用完整 Fence 的内部 Text；
  无效 Session、越权 Collection、任意 Chunk ID、超限 Candidate/Bytes 均失败。
  Python→Go Response 仍必须无正文。
- `go_evidence_hydrator` 只能按最多 16 条精确 Ref 重新授权/Hydrate，不能
  SELECT Base Projection/Object Key、扫描任意 Chunk，也不能执行 Python Search
  Function；`rag_api_reader` 不能执行 Go Hydration Function。
- Answer 对 Processor/Endpoint/Model/Purpose 任一错配、旧 Profile Hash、过期 Consent
  和无 `model_id` 请求全部 Fail Closed。

## 11. Bake-off Baseline 与仍待 Promotion 的值

Phase 15.2A 已用可重复 harness 固定并验证以下 Operational Baseline：

- PostgreSQL `16.14`；`pg_search=0.24.2`；`vector=0.8.2`；
- Image
  `docker.io/paradedb/paradedb:0.24.2-pg16@sha256:556edd8c7500d5ab1bc9c9be1ae97a582cbceb261f8d551386715eb755ab3dcf`；
- `PDB_TUNE=false`、1 GiB/2 CPU、`shared_buffers=256MB`、`work_mem=4MB`、
  `maintenance_work_mem=64MB`；
- Jieba、Lindera Chinese、`chinese_compatible` 使用独立 BM25 Table/Index；
- `vector(1024)`、`halfvec(2048)`、Exact/HNSW、低选择率 ACL、Exact Lane、RRF、
  `template0` Logical Restore、Graceful Restart 和 SIGKILL Recovery 均通过 Synthetic
  Gate。

Harness 位于 `ops/bakeoff/postgres/`，入口为
`scripts/run-phase15-rag-postgres-bakeoff.sh`。该结果证明候选的机械可行性，不等于
真实 Corpus 相关性、Tail Latency 或 License Promotion。

以下仍是有意保留的 Promotion Decision，不得提前写进 `010` 或宣称已实现：

- AGPL 审批、Production Extension/Upgrade/Rollback 接受；
- Dense `vector(1024)`、`halfvec(2048)` 或 Exact-only 的 Relevance/SLO Winner；
- Jieba/Lindera/`chinese_compatible` 的 Golden-set Winner 及 Dictionary/Config Hash；
- HNSW/Exact Threshold、ACL Oversampling/Iterative Scan/Partial Index 策略；
- Winner-specific DDL、Index 参数和 Extension Image Registry Digest。

这些值必须由 Bake-off Report、Restore Drill 和 Promotion Gate 共同选定，再固化进
`011`、Search Profile Hash 与测试 Fixture。除此之外，本文定义的 Identity、Reference、
一致性、权限和迁移边界均为 Canonical。
