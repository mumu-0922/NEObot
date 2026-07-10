# Phase 15 准确率优先 RAG 设计

- 状态：拟定的目标架构，实施前等待项目负责人确认锁定
- 日期：2026-07-10

## 1. 决策

Neo Chat 应采用自适应、分层、混合检索系统，而不是只使用单一分块器或单一路径的向量检索。

```text
Next.js UI
  -> Go API：身份、ACL、文件、聊天、引用、降级策略
     -> Python rag-api：查询路由、检索、重排序、证据包
     -> Redis：非权威的唤醒、租约、限流和查询缓存
     -> Python rag-worker：解析、规范化、分块、嵌入、建索引
        -> MinIO：原始文件和可重建的解析产物
        -> Postgres：文档元数据、ACL、版本、任务、outbox、引用
        -> 搜索投影：Qdrant 为领先候选，可从源数据重建
```

Qdrant 是当前领先的实施候选，因为目标流水线需要带过滤条件的稠密与稀疏检索、命名向量、Multi-vector MaxSim、融合以及多阶段查询。但不能靠主观判断直接选定。必须分别评测 Qdrant、OpenSearch/Elasticsearch 和 Vespa；它们在词法检索、过滤、排序与运维方面的差异，都可能改变本项目的实际结果。Postgres 始终是权威元数据存储；任何搜索引擎都不能成为源数据或派生数据的唯一副本。精确向量搜索是评估 ANN 保真度的基准，不是生产环境的回滚引擎。

不能因为某项技术流行或在公开榜单上领先就直接采用。每一种 Parser、表示方法、查询变换和 Ranker，都必须在 Neo Chat 版本化评测语料上优于当前 Profile。

Postgres outbox 是持久任务源。Redis 丢失可能导致唤醒延迟、租约过期或缓存条目消失，但不能造成索引、删除或重建任务丢失。Worker 会定期重新扫描 Postgres 中尚未分发或尚未确认的 outbox 记录，因此恢复过程不依赖 Redis 历史。

## 2. 不可妥协的边界

- Go 负责全部公开 Endpoint、认证身份、对象授权、会话状态、最终模型调用、SSE 和持久化引用。
- Python 只允许存在于私有网络。它负责解析和检索证据，不负责认证终端用户，也不生成最终聊天回答。Go 通过 mTLS Workload Identity 加上带 Audience、短有效期的签名请求调用 Python；Python 绝不能信任裸露的 `user_id`、`acl_groups` 或浏览器 Bearer Token。
- 原始文件、Provider 输出、Canonical Block、Chunk 和 Vector 必须是相互独立的产物。扁平化 Markdown 文件不得覆盖原始文件。
- Allowed Collection、Personal Owner/Team Scope、Collection/Document Revision、当前有效 Source Version 和未删除状态必须应用在每一条候选查询内部，而不是得到 Top-K 之后再过滤；浏览器提供的 `user_id` 或 `acl_groups` 永远不是授权来源。
- 检索出的内容一律视为不可信数据。它不能覆盖系统指令、授权工具、索要密钥或改变访问策略。
- 严格知识库问答必须 Fail Closed：证据缺失、过期或不足时，明确拒答，不能退回模型记忆作答。只有在普通聊天请求没有承诺知识库 Grounding 时，可选知识增强失败后才允许提示警告并继续。
- Python 返回不可变的 `evidence_id`/`source_span_id`、版本和内容 Hash。Go 必须在组装 Prompt 前立刻依据 Postgres 再次授权每个 Span，并且只有 Go 可以生成与答案一起持久化的 `citation_id`。
- Worker 只获得短有效期、对象级范围的 MinIO GET/PUT Capability。不得获得浏览器 URL、不受限的 Bucket Credential，或当前任务未授权的 Object Key。
- 外部 Parser、托管 Embedding、Reranker、VLM 和答案模型遵循同一套数据出站策略。Query、源文本、证据、图片裁剪和派生训练数据只有在具备租户同意、数据分级批准、地域/保留/删除控制和审计记录时，才允许离开服务器。

## 3. 解析：按格式和页面质量路由

单一 Parser 无法同时准确处理原生数字文本、扫描件、表格、公式、幻灯片、电子表格和代码。Worker 首先验证 MIME、大小、Checksum、页数和压缩包安全，然后在文件级和页面级执行路由。

下表只是首批候选路线，不是对任何 Vendor 的预设结论。每种格式的路线只有通过与源文件对齐的 Parser 评测后才能锁定。

| 输入                         | 候选主路线                                    | 候选回退路线                             |
| ---------------------------- | --------------------------------------------- | ---------------------------------------- |
| 纯文本、Markdown、JSON、YAML | 原生确定性 Parser                             | 编码检测器加隔离区                       |
| HTML                         | 保留标题、链接、列表、表格和代码的 DOM 提取   | 渲染动态区域后进行视觉解析               |
| DOCX/PPTX                    | Docling 或原生 OOXML 结构                     | 只将复杂 Shape/Page 交给 MinerU 渲染解析 |
| XLSX/CSV                     | 保留公式和值的原生 Sheet/Cell Parser          | 对图表或复杂渲染区域进行视觉解析         |
| 简单文本 PDF                 | PyMuPDF 预检加 Docling 结构化解析             | 页面级 PyMuPDF Block 恢复                |
| 多栏、公式、表格或扫描 PDF   | MinerU 高计算强度结构化输出                   | 经策略批准且固定版本的 LlamaParse        |
| 代码                         | 按 Module、Class、Symbol 使用 Tree-sitter/AST | 保留行号的确定性回退方案                 |

PDF 必须按页面路由。可靠的原生文本仍是字符内容的权威来源；OCR 或 VLM 用于补充版面和非文本结构。低质量页面单独重试，而不是重新处理整个文件。

每个 Parser 都必须运行在隔离的 Rootless Worker 中，使用只读 Root Filesystem、受限临时空间、不继承任何 Secret、默认禁止出站网络，并限制 CPU、内存、页数、压缩包嵌套深度、解压比和总执行时间。日志只记录 ID、版本、耗时和 Hash，不记录文档正文或 Prompt。机密数据默认禁止外部处理。任何获批的外部 Parser Endpoint 都必须遵守统一出站策略和明确的目标 Allowlist；批准 Parser 不代表同时批准托管 Embedding、Reranking 或多模态推理。

Parser 输出版本化的 Canonical Block 表示，而不只是 Markdown。必需字段包括：

```text
document_id, source_version, block_id, ordinal, block_type
parent_id, heading_path, text/markdown/html/latex/code
page/slide/sheet, bbox, reading_order, asset_refs
table cell grid and row/column spans
parser name/version/config hash and confidence
derived/non_indexable/needs_review flags
```

这样才能保留页面级引用、表格、公式、图形，并支持在不重新解析的情况下重新分块。解析质量门槛覆盖文本覆盖率、替换字符比例、阅读顺序、OCR 置信度、表格一致性以及源文件与输出的 Page 数一致性。失败页面必须进入隔离区，绝不能以空文本形式标记成功。

对视觉信息丰富的页面，要保留渲染后的 Page Image，以及与源页面对齐的 Figure/Table/Chart Crop。ColPali 或 ColQwen 等视觉 Late-interaction Profile 可作为评测候选；它们是 Canonical Text 和结构的补充，而不是替代品。多模态答案模型可以接收已授权的图片裁剪，但引用仍必须定位到不可变的页码和 Bounding Box。

## 4. 分层、结构感知的分块

默认采用 Small-to-Big 检索，并为不同内容类型设置不同 Chunk Policy。下面的数值只是第一版、Tokenizer-aware 的 Profile，不是永久固定的魔法参数；评测时要根据语言、格式和 Embedding Model 调整，但冻结的 Source-span Qrels 不得改变。

### 普通正文

- Parent：按 Section 对齐，目标 1,400-1,600 Token，硬上限 2,000。
- Child：目标 384 Token，硬上限 512。
- Overlap：只有过长语义 Block 必须拆分时才使用 48 Token。
- Sentence-window Metadata：保存前后 Block 链接；按证据需要扩展，而不是固定加载整个 Parent。

当 Provenance 和 Sibling Link 已存在时，普通段落边界不需要无条件 Overlap。过大的 Overlap 会制造重复候选并浪费 Reranker Slot。表格应重复 Header，而不是机械重叠正文；代码应携带 Signature/Import Context；一张 Slide 通常作为一个 Parent；电子表格的 Child 是有边界的 Row Group，并重复 Column Header。

每个 Child 保存两种严格区分的形式：

1. `source_text`：不可变、可引用的源内容。
2. `retrieval_text`：确定性 Breadcrumb，加可选生成 Context，再加源文本。

确定性前缀始终启用：

```text
document title > product/version > section path > block type
```

LLM 生成的 Contextual Retrieval 文本必须通过质量门槛，并作为派生 Metadata 保存。它适用于包含大量代词或依赖上下文的 Chunk，但绝不能作为源证据引用。Parser、Chunker、Tokenizer、Contextualizer 和 Prompt 的版本都属于 Index Profile。

稳定的 Chunk Identity 必须包含 Source Version 和 Content Hash。重新索引时创建新 Generation，只有验证通过后才切换 Active Generation。每条生成的 Context、Summary、Graph Edge、Visual Description 和 Embedding Input 都必须 Content-addressed，并记录 Source Hash 以及 Model、Tokenizer、Prompt、Code 和 Configuration Digest。这样才能复现检索结果，并防止模型或 Prompt 静默漂移。

表格还要生成具有 Cell-level Lineage 的 Arrow/Parquet 表示。对于数值、聚合或筛选问题，应评测 DuckDB 执行路线与文本检索路线。DuckDB 必须运行在无 Secret、默认断网、看不到宿主文件系统的 Sandbox 中，并且只接收当前 ACL Snapshot 已物化的不可变表。必须禁用 Extension 安装/加载、外部文件/网络函数、`ATTACH`、`COPY`、`PRAGMA`、DDL 和 DML。SQL AST Allowlist 只允许针对已注册 Relation/Column ID 的受限 `SELECT`，同时限制 Statement 数量、执行时间、内存、返回行数和输出字节数。测试必须主动尝试文件逃逸、SSRF、Extension 逃逸和跨 Snapshot 访问；仅把连接称为“只读”远远不够。

## 5. 多表示索引

候选搜索投影中的每个 Child Point 都必须包含强制的 ACL/Version Payload Index 和版本化表示：

```text
named vectors:
  dense_qwen_v1       contextual child dense embedding
  title_summary_v1    title/section/summary representation
  colbert_bge_v1      optional token-level multi-vector

sparse vectors:
  bm25_v1             analyzed lexical relevance path
  lexical_bge_v1      optional learned sparse weights

payload:
  scope, owner_user_id|team_id, collection_id
  collection_acl_revision, collection_visibility_epoch
  document_id, document_version_id, source_version
  document_visibility_epoch, document_status
  index_generation, projection_revision, source_span_hash
  parent_id, child_id, previous_id, next_id
  language, block_type, page_start, page_end, active
```

浏览器不得提交 Payload 中的 Identity/Fence 字段。`team_membership_revision`、`user_query_consent_revision` 与 `processor_governance_revision` 属于签名 Authorization Snapshot 和 Cache Key，不要求内容点因成员或用户 Consent 改变而全量 Reindex。字段归属以及 Mutation → Revision → Invalidation 的唯一规范由 Phase 15 [`knowledge-acl-api.md`](../contracts/knowledge-acl-api.md) §7.1 定义。

BM25 不是精确匹配路线。ID、Version、Symbol、Quoted Phrase、Raw Path 和大小写敏感标识符必须使用单独版本化的 Keyword/Phrase Index，并明确 Normalization Rule；原始形式和规范化形式都要保留。Analyzer、Tokenizer、Stop-word、Stemming、Locale 和 Phrase-position Rule 都是 Index Profile 的不可变组成部分。

第一轮准确率 Bake-off 推荐候选模型：

- Dense：`Qwen3-Embedding-4B` 和 `Qwen3-Embedding-8B`，比较完整维度与 MRL 2,048 维。
- Learned Sparse 与 Late Interaction：`BAAI/bge-m3`。
- Cross-encoder Rerank：`Qwen3-Reranker-4B`、`Qwen3-Reranker-8B` 和 `BAAI/bge-reranker-v2-m3`。
- 外部对照组：至少一个经策略批准且固定版本的托管 Embedding/Reranking Profile，避免在没有竞争性对照的情况下直接宣布本地模型最佳。

最终模型必须根据项目数据选择。Qwen3 的 Query Instruction 保持英文并纳入版本管理；Document Embedding 不附加 Query Instruction。准确率测试从 Float Vector 和高 ANN Search Setting 起步。Quantization 或降维只有在实测无回归后才能启用。

第一轮搜索引擎 Bake-off 必须固定准确的 Image 和 Client Version。Qdrant 的首个候选版本下限为 `>=1.17`，以确保支持可配置/带权 RRF；锁定 Profile 时必须验证并固定具体组合，生产环境绝不能跟随 `latest`。选型时必须把四个问题分开评估：Retrieval Profile 相关性、ANN 相对 Exhaustive Search 的保真度、高并发下 ACL/Filter 正确性，以及 Snapshot/Rebuild/Rollback 行为。不能让某个引擎通过使用不同的 Retrieval Profile，去补偿更差的 Embedding 或 Reranker。

### 5.1 单服务器模型服务

FastAPI 只负责编排，不能把 Model Lifecycle 隐藏在 Web Worker 内部。Embedding、Reranking、Visual/VLM Inference 以及后续 Training Job 都在独立版本化进程中运行，并统一经过一个 Admission Controller；该控制器负责有界 Queue、Batching、Health Probe、CPU/RAM/GPU Reservation 和 Maintenance Window。离线文档 Embedding 可以牺牲延迟换取 Batch Quality，而在线 Query Embedding 与 Reranking 必须设置明确 Deadline 和 Backpressure。

8B 准确率 Profile 要获得交互式延迟，通常需要合适的 GPU。CPU-only VPS 应使用经策略批准的托管推理对照方案，或者只有在较小本地 Profile 通过同一 Frozen Holdout 后才能采用；绝不能静默 Quantize、Truncate 或 Downgrade。Parser 和 Embedding Job 必须给公开 Go API 让出资源，不能拖垮聊天或存储服务。Visual、Training 或 Graph 路线在预留容量不足时保持 Disabled，不得偷偷抢占在线容量或降级模型。

## 6. 自适应检索流水线

### 6.1 查询理解

原始 Query 不可变，并且始终参与检索。Router 将 Query 分类为：

- 精确 Identifier、Version、Path 或 Quoted Phrase；
- 语义事实查询；
- 比较型或 Multi-hop 问题；
- 面向长语料的全局/主题问题；
- 表格/数值问题；
- Entity/Relationship 问题；
- 信息不足、需要澄清的问题。

确定性 Normalization 必须保留 ID、日期、数字、Quoted Phrase 和大小写敏感 Path。可以并行执行保守 Rewrite。Multi-query、HyDE 和 Decomposition 绝不能替代原始 Query，并且只对实测有效的 Query Class 启用。

如果已授权且已选择的 Corpus 能安全放入模型 Context Budget，可以启用 Full-context 路线，避免 Candidate Retrieval 阶段的截断损失，同时仍保留 Source-span Citation 和同一份 ACL Snapshot。它依然必须通过 Long-context Position Bias、Attention Dilution、Packing/Tokenization 和 Citation Coverage 测试。表格问题路由到结构化 Cell/Row Retrieval；关系密集的全局问题可以路由到 Graph 或 Hierarchical-summary Index。

### 6.2 候选生成

通用路线中，每个 Branch 必须使用完全相同的 ACL Filter：

```text
Dense child recall       top 100
BM25 lexical relevance   top 100
Exact phrase/key/path    top 50
Learned sparse           top 100 when validated
Title/section summary    top 50
Visual page/crop recall  top 50 for visually rich query classes
```

使用 RRF 融合 Rank，而不是直接混合未经校准的 Score。`k=60` 只是首个评测 Profile，不是通用常量。候选并集保留 100-200 条。精确 Identifier 必须走独立的 Keyword/Phrase 路线，避免 Semantic Rewrite 或语言 Analyzer 抹掉关键字。

### 6.3 精细交互与重排序

只有增量收益通过门槛后，才启用 ColBERT-style MaxSim，将候选并集缩减到 40-80 条。随后 Cross-encoder 将 40-100 条候选重排到 12-20 条。Cross-encoder 输入包含 Breadcrumb、Child Text，以及判断相关性所需的最小局部 Context；不得输入无限大的 Parent。

Reranker Score 不能被当作全局已校准概率。Threshold 应按 Query Class 学习，或改用 Rank 与 Evidence Coverage Rule。

### 6.4 证据扩展与打包

完成 Rerank 后：

- 合并相邻的获胜 Child；
- 只有 Parent 不超过 Evidence Budget 时才扩展到整个 Parent；
- 否则加入能闭合语义单元的最小 Sibling Window；
- 限制同一 Document 的重复证据；
- 比较型问题要保留 Source Diversity；
- Direct Evidence 优先于 Derived Summary；
- 初始打包约 8k-12k Evidence Token，并根据所选模型调整。

返回的 Evidence Package 包含不可变 Source Span、Page/Bbox、Document Version、各检索阶段 Score、`evidence_id`、`source_span_id` 和 Source Hash，不包含持久化 Citation ID。Go 必须根据当前 Postgres Visibility/ACL/Version 状态批量再次授权每个 Span、验证 Hash、取得已授权源材料，之后才能生成 Citation ID，并与最终 Assistant Message 一起持久化。

生成答案前，Evidence-sufficiency Gate 判断所选 Span 是否共同支持问题要求的事实。答案协议将每条 Claim 关联到 Evidence ID；生成后 Verifier 检查 Claim Entailment、Citation Coverage，以及无依据的数字和名称。严格知识库答案验证失败时，只能用同一批已授权证据重新生成一次或拒答，绝不能加入无引用的模型知识进行“修补”。

### 6.5 严格知识库回答的提交与 SSE 顺序

严格模式为了建立可验证的答案边界，需要放弃逐 Token 展示：

```text
generate into server-only buffer
-> verify claims, evidence, and citation coverage
-> recheck Postgres ACL/version/visibility fences
-> atomically persist assistant message + citations + source-span hashes
-> emit the persisted body through SSE
```

事务提交前，SSE 只能发送 Status/Progress Event，不能泄露 Draft Answer Text 或 Source Content。如果任一 Fence 已变化，Go 必须丢弃 Buffer，并重新执行一次完整检索或拒答。客户端在提交后断开连接，可以通过读取同一条持久化 Message 恢复；不会产生“有答案但没有引用”的状态。Optional-enrichment Chat 可以保留现有 Streaming UX，但不能把未经验证的输出标记为已 Grounded。

## 7. 条件式高级路线

以下能力是目标能力，不是全局开关：

- Contextual Retrieval：只对丢失 Entity/Reference Context 的 Chunk 启用；生成 Context 单独建索引，绝不直接引用。
- Late Chunking：只对能提供兼容 Token-level State 的 Embedding Model 测试；它不能替代 Parent/Window Hydration。
- RAPTOR：只对稳定的长篇叙事语料和全局/Multi-step 问题启用；Summary 只是导航辅助，不是主要证据。
- GraphRAG：只对关系密集语料以及全局/Entity Query 启用。Graph 必须在 ACL Domain 内构建，避免生成跨权限范围的 Summary。
- Multi-query/Decomposition：只用于宽泛或 Multi-hop Query，最多 2-4 个 Variant/Subquestion，并始终保留原始 Query。
- HyDE：只在实测存在 Vocabulary Gap 的场景用于 Dense Branch；绝不能污染 Exact Lexical Retrieval。
- Domain Adaptation/LTR：积累足够 Human Qrels 后，从生产候选中挖掘 Hard Negative，评测 Fine-tuned Embedding、Reranker 或 Learned Fusion Profile。训练数据绝不能看到 Frozen Holdout，Base Profile 必须保留用于回滚。

Domain-adaptation Row 默认 Tenant-scoped；只有所有贡献数据的 Tenant 都明确批准共享模型时才允许合并。Training Manifest 保存 Source Lineage、Consent、Retention、PII 处理和删除状态；源数据撤权或删除后，依赖它的 Dataset 失效，相关 Model Version 在继续使用前必须被隔离、重新训练或退役。Poisoning、Membership/Memorization Leakage、Cross-tenant Retrieval 和 Prompt-injection Transfer 都是硬性评测门槛，不能只依赖上线后监控。

盲目全局启用所有高级能力，可能因 Query Drift、Generated-context Pollution、重复证据和 Attribution Loss 而降低准确率。

每个 RAPTOR Node、Graph Node/Edge/Community Summary 和 Contextualized Chunk 都必须保存完整 Source Lineage。其有效权限取所有贡献来源允许访问者的最严格交集，并传播 `acl_revision` 和 `visibility_epoch`。如果不能安全形成非空交集，就按 ACL Domain 拆分派生产物，或者不构建。Source ACL 变化或删除在被确认前，必须先为依赖的派生产物写入 Tombstone。

## 8. 一致性、重建与失败语义

Postgres 保存 Source Metadata、Active Generation、Parse/Chunk Manifest、Index Job 和 Transactional Outbox。Worker 根据 Outbox 幂等执行 Search-index Upsert/Delete，并在重启或 Redis 丢失后定期重新扫描 Postgres。Document 状态如下：

```text
UPLOADED -> PARSING -> CHUNKING -> EMBEDDING -> BUILDING
         -> VERIFIED -> ACTIVE

ACTIVE -> TOMBSTONED -> PURGING -> DELETED
ACTIVE(v1) -> BUILDING(v2) -> VERIFIED(v2) -> ACTIVE(v2)
```

Query 只能看到 `ACTIVE + current_generation`。删除操作首先使数据在逻辑上不可见，然后再清除 Vector、Artifact、Cache 和 Source Object。

### 8.1 ACL、版本与删除栅栏

目标部署是小团队、多用户模型，不是单一公开 Corpus：每个 User 有自己的 Personal Knowledge，Team 另有 Shared Knowledge 和 Team Admin。Phase 15 使用显式 `teams`、`team_memberships`、`knowledge_collections`、`knowledge_documents` 和 `processing_consents`，不能继续把未经校验的 `workspaceId`/`knowledgeCollectionId` 塞进 File Metadata，也不能把 Chat Workspace 当作 IAM Team。

Personal Collection 必须且只能绑定一个 `owner_user_id`；Team Collection 必须且只能绑定一个 `team_id`。Team Membership 首批只设 `admin|member`：Admin 管理 Team Membership、Team Document 和 Team Collection Consent，但不得读取其他用户的 Personal Knowledge；Member 默认只能查询 Team Knowledge。需要成员维护 Team 文档时新增 `contributor`，不得静默扩大 Member 权限。系统必须始终保留至少一个 Active Admin，最后一个 Admin 的降级、删除或离队事务必须拒绝。

当前 Owner 对外部 Processor 的批准只属于指定的 Bootstrap Public Collection，不是未来上传的全局 Consent。Personal/Team 上传默认 `non-public + external processing denied`；Personal Content 由其 Owner 授权，Team Content 由 Team Admin 授权，Query Text 还需请求用户的 Query Consent。每次外部调用都先要求匹配 Purpose/Data Type/Endpoint/Model/Region 的 Approved Processor Governance Profile，再按操作应用 Consent：

| 外部操作                   | Governance 之后的 Consent 条件                        |
| -------------------------- | ----------------------------------------------------- |
| Parse / Passage Embedding  | 对应 Collection + Processor + Purpose + Data Type     |
| Query Embedding            | 当前 User + Processor + Query Data Type               |
| Rerank / Answer / Evidence | 当前 User Query Consent + 所有选中 Collection Consent |

后台 Parse/Index 不依赖 Query Consent；Query Embedding 不能拿 Collection Consent 代替用户授权；Rerank/Answer 不能静默丢弃缺少 Consent 的 Collection。

上传动作不构成 Consent。Consent 撤回必须推进对应的 Collection Processing、User Query Consent 或 Governance Head Revision，停止新 Job/调用、取消可取消队列并失效 Cache；它不能伪造 ACL/Visibility 变化。若适用条款要求 Derived Artifact 删除，则通过 Outbox 清理并保持受影响 Retrieval Lane Disabled，直到合规 Artifact 重建。内容权限收紧或删除才推进 ACL/Visibility Fence。Bootstrap Public Collection 必须以真实 Collection Row 和版本化 Consent Record 表示，不能落成系统默认布尔开关。

Knowledge Query 要求显式、非空的 Collection ID 列表；Go 依据当前 Principal、独立 Session、Team Membership Revision 和 Collection Row 计算 Allowed Collections，拒绝客户端提供 `user_id`、`team_id`、`acl_groups` 或 Impersonation Hint。Team 查询和 Personal 查询可以融合，但每条 Search Point 必须带 Canonical `scope`、`owner_user_id/team_id`、`collection_id`、Collection ACL/Visibility、Logical Document ID、Document Version ID/Visibility/Status 与 Active 状态。Collection Processing/User Query/Governance Revision 只进入授权 Snapshot/Cache Key，不要求为 Egress Consent 变化改写每个 Search Point。越权资源探测返回 404；已知 Team 资源上的角色不足操作返回 403。

完整 Future Endpoint、Permission Matrix、Consent、File Binding、Error 和 Required Test Contract 见 [`knowledge-acl-api.md`](../contracts/knowledge-acl-api.md)。

Collection 拥有单调递增的 `collection_acl_revision`、`collection_visibility_epoch` 和 `collection_processing_revision`；Logical Document 通过 `current_version_id` 指向 Content Identity 不可变的 Document Version Row，其 File/Source/Hash 不可原地修改，而 `document_visibility_epoch` 和 Status 只能通过带 Outbox 的受控事务推进。Team Membership、User Query Consent 与 Processor Governance Head/Profile 各自拥有独立 Revision。ACL 收紧或删除操作只有在一个 Postgres Transaction 已推进对应 Fence、改变权威 Visibility，并将依赖 Artifact/Cache/Index 的 Tombstone 写入 Outbox 后，才能向调用方确认成功。其他存储的物理清理可以异步执行，但此后创建的所有 Authorization Snapshot 必须立即拒绝旧 Revision。

Go 从 Postgres 授权 Corpus Selection，并签发短有效期请求。请求包含 Principal/Session ID、Allowed Collection ID、Authorization Fingerprint、Request ID、Audience、Contract Version、`iat`、`exp`、单次使用的 `jti`，以及 Team Membership、Collection、Document、User Query Consent、Processor Governance、Index Generation 和 Projection Revision Snapshot。签名还必须覆盖 Canonical Method、Path、Body Hash、Retrieval Profile 和 mTLS Workload Identity。Python 使用有界、Fail-closed 的 Nonce Store 拒绝 Replay，限制 Clock Skew，并通过重叠 `kid` 支持 Signing-key Rotation。

Python 将同一 Snapshot 应用于每条 Retrieval Branch，只返回 Source Reference，不返回授权决定。Go 随后必须依据当前 Postgres Row 批量再次授权每个返回的 Span：第一次在组装 Prompt 前，第二次在严格模式 Commit Boundary。过期 Span 触发带过滤条件的重试或拒答。Query/Cache Key 必须包含 Authorization Fingerprint、Team Membership Revision、全部 Collection/Document Fence、User Query Consent Revision、Processor Governance Profile/Revision、Index Generation、Content Projection Revision 和完整 Retrieval-profile Digest，确保新 Fence 或新 Projection 绝不会命中旧结果。

### 8.2 Generation、Projection Revision 与发布

必须使用两个相互独立的版本维度，避免把“不可变 Snapshot”与日常内容更新混为一谈：

- `index_generation_id`：定义 Parser/Chunker Schema、Embedding、Vector Shape、Analyzer/Tokenizer 和 Rank-profile Contract。迁移时创建新的 Physical Collection；发布后，其 Configuration 以及版本化 Routing-handle 到 Collection 的映射不得改变。
- `corpus_projection_revision`：表示某个 Generation 内单调递增的 Content/ACL/Version 状态。普通 Upload、Update 和 Purge Job 会幂等修改 Collection 并推进 `applied_outbox_id`；因此不能把 Collection Byte 描述为不可变。

Postgres Mutation Transaction 推进 Corpus Revision 并写入 Outbox ID。Worker 将它应用到仍需支持的每个 Serving/Rollback Generation，验证 Mutation 后再记录 Applied Watermark。严格请求必须指定签名后的 Generation 和最低 Corpus Revision；如果 Search Projection 尚未追平，就在 Deadline 内等待或拒答。即使物理 Purge 仍有延迟，Postgres Fence 仍会立即拒绝访问已删除的数据。

Schema/Model/Analyzer 迁移使用两阶段发布：

1. 从明确的 Postgres Source Watermark 构建新 Physical Collection，同时旧 Generation 继续服务；保存 Profile/Config Manifest Hash。
2. Replay Outbox 到 Publish Watermark，然后验证 Count、Hash、ACL Payload Index、Exhaustive-versus-ANN Fidelity、Retrieval Gate 和 Source Round Trip。
3. 创建版本化 Engine Routing Handle；如果使用 Qdrant，则为 `rag-generation-<id>` 形式的 Alias。验证它实际指向的 Physical Collection ID 和 Config Hash，并且永远不再重定向该 Handle。
4. 在一个 Postgres Transaction 中发布 Active Pointer，绑定 Generation ID、Physical Collection ID、Alias、Manifest Hash 和 Required Applied Watermark；同时写入 Cache-invalidation Outbox Row。
5. 在 Rollback Window 内，对旧 Generation 双写兼容的 Content Mutation。否则，回滚前必须先将旧 Generation Replay 到 Required Watermark，并保持 Retrieval Unready，之后才能切回 Postgres Pointer。

Retrieval 从签名后的 Postgres Snapshot 解析准确的 Generation Binding。可变 Convenience Alias 不能作为权威或一致性边界；Backup Snapshot ID 是另一个独立的 Point-in-time 概念。

### 8.3 备份、恢复与 Replay

一份协调一致的 Backup Manifest 必须绑定：

```text
backup_set_id
Postgres timeline + LSN/WAL range + outbox high-watermark
search physical collection/snapshot ID + applied_outbox_id
MinIO version/manifest root hash
search image digest + client/API version + collection config
analyzer/tokenizer/model/prompt/config asset hashes
```

针对仍在线的 Postgres 做 Search-only Restore 时，必须从 Snapshot 的 Applied Watermark Replay 到当前 Outbox 状态；在 ACL/Version/Delete Reconciliation 确认不存在可见 Orphan 前，服务保持 Unready。只要缺少任意一段 Outbox Interval，就必须丢弃该 Search Snapshot，并根据在线 Postgres 与 Canonical MinIO Artifact 全量重建。

Full Disaster Restore 只能声明恢复到由同一套协调 Postgres/WAL、MinIO 和 Search Backup 所代表的 Recovery Point。所有受支持 Backup/RPO 依赖的 Tombstone/Outbox Row 和 Archived WAL，在相应 Backup 过期前都不得执行 GC。如果无法证明 Postgres Timeline/LSN Continuity 或后续 Delete History，就不能声称已经 Replay 到更晚状态；必须从恢复后的权威状态重建 Search Projection，并保持 Unready。任何从未被备份的 Event，都无法从较旧的 Search Snapshot 中恢复。

Postgres Metadata，加上 MinIO 中的 Original、Parser-native、Canonical 和 Content-addressed Derived Artifact，必须足以复现所选 Recovery State 下的每个 Search Point。如果某个 Derived Artifact 缺少完整的 Source/Model/Prompt/Config Lineage，就必须重建而不是信任它。Redis 从 Postgres 重建，可以随时丢弃。同一台物理服务器上的备份不属于 Disaster Recovery。

### 8.4 失败行为

`strict_grounded` 与 `optional_enrichment` 是两个独立的请求契约：

- `strict_grounded`：Parser/Index 不可用、Authorization 过期、Retrieval Timeout、Evidence 不足或 Citation Verification 失败时，返回明确拒答/错误，绝不能使用模型记忆回答。
- `optional_enrichment`：只有在 UI 明确告知知识检索失败后，普通聊天才可以不使用 RAG 继续；响应不能宣称自己基于所选知识库完成了 Grounding。

Circuit Breaker、有界 Retry 和 Deadline 用于防止故障 Parser、Model Server 或 Search Projection 拖垮普通 Chat 与 Ingestion。它们绝不能把 Fail-closed 的授权或 Grounding 决策变成 Fail-open。

## 9. 评测是架构的一部分

没有针对自身 Corpus 的评测体系，就不能诚实地宣称“检索最准”。首批构建 300-1,000 个问题的版本化 Golden Set，并持续从真实流量扩展。数据集必须覆盖中英文、精确 ID、版本/日期、表格、多文档比较、Multi-hop、Hard Negative、No-answer Case、ACL 隔离、删除和 Prompt Injection。

将数据集拆为 Development Set 和 Profile Tuning 绝不能看到的 Frozen Holdout。人工 Qrels 指向不可变 Source Span，并确定性映射到 Child、Parent、Page 和 Document Relevance Label。对存在多种正确证明的问题，必须保存多组有效 Evidence Set。Synthetic Question 可用于扩充覆盖面，但不能主导 Holdout。

首批 Promotion Gate：

| 层级                | 绝对门槛                                                          |
| ------------------- | ----------------------------------------------------------------- |
| Provenance          | Source/Version/Page/Span/Hash 可追溯率 = 100%                     |
| Parser/OCR          | 按格式锁定 Text、Order、Table、Formula、Bbox 门槛                 |
| Candidate Retrieval | Recall@50 >= 0.95                                                 |
| Final Retrieval     | Recall@10 >= 0.90；nDCG@10 >= 0.85；MRR@10 >= 0.80                |
| Citation            | Correctness >= 0.95；Completeness >= 0.90                         |
| Grounding           | Factual Faithfulness >= 0.95                                      |
| Abstention          | No-answer False-answer Rate <= 2%                                 |
| Visual Lane         | Supporting Region Recall@10 >= 0.90；Citation Correctness >= 0.95 |
| Table Lane          | Deterministic Exact-answer >= 0.95；Cell Lineage = 100%           |
| Security            | Unauthorized Evidence、Deleted-version Leakage、ACL Leakage = 0   |
| Injection           | Policy Override、Secret Leakage、Unauthorized Tool Call = 0       |
| Adaptation          | Cross-tenant/Secret Leakage、Accepted Poisoned Evidence = 0       |

Parser 评分使用与源文件对齐的 Character Recall/OCR CER、Reading-order Agreement、Table Structure/Cell F1（适用时使用 TEDS）、Formula Transcription Accuracy、Figure/Bbox Alignment，以及 Page/Slide/Sheet Parity。Threshold 必须按格式和质量类别分别设定，不能让高平均分掩盖 Scanned PDF 或 Table Slice 的失败。

每次 Promotion 还必须相对 Frozen Production/Baseline Profile 通过门槛。在打开 Holdout 前，预注册 Macro/Micro Aggregation、Question-level Paired-bootstrap Seed、Primary Metric、Minimum Useful Gain 和 Per-slice Regression Budget。每个 Critical Slice 至少需要 50 个已标注问题，并且对预注册 Effect Size 的统计功效 `>=0.8`；否则该 Lane 不得在统计不足的 Slice 上启用。

Primary Metric 的 95% Paired-bootstrap Confidence Interval 必须超过预设 Margin，同时任何关键 Language、Format、Query-class、ACL 或 No-answer Slice 都不得超过允许的 Regression Budget。Reranker 的首个目标为 `nDCG@10 +0.03`；`relevant-drop <=2%` 的定义是：在 Answerable Question 中，从 Baseline Top-10 至少包含一个 Qrel，退化为 Candidate Top-10 完全没有 Qrel 的比例不得超过 2%。Latency、GPU/CPU Memory、Index Size 和 Cost 作为约束报告，但绝不能替代 Relevance。

Visual 测试覆盖 Page/Region Retrieval、Bbox-linked Citation、OCR/VLM Disagreement 和 Multimodal Prompt Injection。Table 测试覆盖 Exact Numeric Answer、Cell Lineage、Malformed Formula，以及 File/SSRF/Extension Escape Attempt。Domain-adaptation 测试覆盖 Poisoned Hard Negative、Membership/Memorization Probe、PII/Secret Canary、Deletion Propagation 和 Cross-tenant Isolation。缺少足够专项 Case 的候选必须保持 Disabled，不能继承普通文本的评分。

在 LLM Judge 对全部结果评分前，人工 Reviewer 必须先用分层样本校准 Judge；记录一致率，并人工审计分歧、全部安全失败以及随机抽取的通过样本。公开榜单和 Vendor Demo 只能作为先验。对样本执行 Exhaustive Vector Search，单独测量 ANN Recall，避免与 Embedding Relevance 混淆。比较搜索引擎时，必须使用完全相同的 Parser、Corpus、Vector、Lexical Rule、Filter、Fusion 和 Reranker；随后再分别测试 Filter Correctness、Failure Recovery 和 Snapshot Replay。每轮 Bake-off 只改变一个变量。

## 10. 实施顺序

这个顺序是为了保证归因与安全，不是为了缩减 MVP 功能：

1. 在选择最终 Parser、Model 或 Search Engine 前，冻结 Canonical Block/Chunk Schema、Security Invariant 和 Evaluation Corpus。
2. 在 MinIO 中保存 Original File 和 Parser-native Structured Output。
3. 实现 Sandbox Parser Candidate、Page-level Quality Gate 和 Canonical IR；按格式专项评测选择路线。
4. 实现具有确定性 Provenance 的 Parent/Child/Window Chunk。
5. 在提供检索服务前，实现 Postgres Authorization/Version Fence、Durable Outbox Rescan 和经过认证的 Go-to-Python Evidence Contract。
6. 使用等价的 Exact/Exhaustive 与 ANN Baseline，对 Qdrant、词法/搜索替代方案和 Qwen3/BGE Profile 做 Bake-off，并固定获胜 Profile。
7. 加入 Dense、BM25、Exact-key/Phrase 和通过评测的 Learned-sparse Recall；使用版本化 Analyzer Rule 和完全相同的 ACL Filter。
8. 加入受控 Visual/ColBERT Candidate、Cross-encoder Reranking、Dynamic Evidence Expansion，以及通过门槛后的 Structured Table Execution。
9. 加入 Query-class Routing，之后才测试 Contextual Retrieval、Late Chunking、Multi-query/Decomposition、RAPTOR 和 GraphRAG 路线。
10. 接入由 Go 管理的 Citation Minting、Strict-grounded Abstention、Claim/Evidence Verification、Cache Fencing、Observability、Backup、Restore 和 Tombstone Replay Drill。

## 11. 关键依据

- Anthropic Contextual Retrieval 报告显示，Contextual Embedding/BM25 能减少 Top-20 Retrieval Failure，加入 Reranking 后降幅更大；这是 Vendor Evidence，必须在本地复现。
- Qwen3 Embedding 支持多语言、Instruction-aware、Long-context Dense Retrieval 和 MRL Dimension；公开排名不能保证它在本项目 Corpus 上最优。
- BGE-M3 提供 Dense、Learned Sparse 和 ColBERT-style 表示，并推荐 Hybrid Retrieval 加 Reranking。
- Qdrant 文档说明同一引擎支持带过滤的 Dense/Sparse Named Vector、Multi-vector MaxSim、Prefetch、RRF/DBSF Fusion 和 Snapshot；项目 Bake-off 仍必须验证准确版本和真实行为。
- ColBERTv2 通过压缩 Token Representation 展示了细粒度 Late Interaction；其 Storage 与 Operating Cost 仍需要用增量收益证明。
- ColPali 展示了直接基于 Document Page Image 的检索，为依赖 Layout 的证据提供了可评测的 Visual Lane。
- RAPTOR 通过 Recursive Summary 改善长文档 Multi-step Retrieval，但生成 Summary 会增加 Incremental Update 与 Source Attribution 的复杂度。
- Microsoft GraphRAG 明确警告 Indexing 成本很高；它是针对特定 Query 的方法，不是 Hybrid Search 的通用替代品。

主要参考资料：

- https://www.anthropic.com/news/contextual-retrieval
- https://github.com/QwenLM/Qwen3-Embedding
- https://huggingface.co/BAAI/bge-m3
- https://qdrant.tech/documentation/concepts/hybrid-queries/
- https://qdrant.tech/documentation/concepts/vectors/
- https://qdrant.tech/documentation/concepts/snapshots/
- https://docs.vespa.ai/en/tutorials/hybrid-search.html
- https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/
- https://github.com/docling-project/docling
- https://github.com/opendatalab/MinerU
- https://arxiv.org/abs/2112.01488
- https://arxiv.org/abs/2407.01449
- https://arxiv.org/abs/2401.18059
- https://github.com/microsoft/graphrag
- https://arxiv.org/abs/2104.08663
- https://arxiv.org/abs/2305.14627
- https://arxiv.org/abs/2309.15217
- https://genai.owasp.org/llmrisk/llm01-prompt-injection/
- https://genai.owasp.org/llmrisk/llm082025-vector-and-embedding-weaknesses/
