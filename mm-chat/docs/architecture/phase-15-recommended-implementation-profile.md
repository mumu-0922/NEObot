# Phase 15 推荐落地方案

- 状态：Owner Review Draft，未锁定、未实施
- 日期：2026-07-10

Owner Lock 条件：第 11.2 节所有“阻断上线 = 是”的项目关闭，并完成 Development/Validation Bake-off；否则本文件只代表候选设计。

## 1. 推荐结论

吾建议 Neo Chat 的第一轮 Development/Validation 候选组合定为：

```text
文档解析候选：原生确定性 Parser + MinerU Precise
Embedding 高容量 Baseline：Jina Embeddings v4 API，retrieval，2048 维
搜索引擎首个候选：Qdrant Dense + BM25 + Exact Keyword/Phrase
融合：按 Query Class 加权 RRF
重排序：托管 Multilingual Cross-encoder，首批候选 Jina Reranker
引用：Source Version + Page/BBox/Cell/Span
最终回答：Go 管理 strict_grounded、Citation 与 SSE
```

这不是为了减少功能，而是在单服务器、暂按无本地 GPU 规划的约束下，优先评测把 GPU 密集型 Parser/Embedding/Reranker 放到受控外部服务，同时把身份、ACL、原文件、结构化产物、索引和引用控制权留在自己的服务器。Jina v4 Hosted、MinerU Hosted、Reranker 和 Qdrant 均是待验证候选，不是已经锁定的生产依赖。

## 2. 文档解析方案

### 2.1 推荐路由

| 文档类型                  | 推荐 Parser                     | 原因                                       |
| ------------------------- | ------------------------------- | ------------------------------------------ |
| TXT、Markdown、JSON、YAML | 原生确定性 Parser               | 无 OCR 误差，结构可稳定复现                |
| 代码                      | Tree-sitter/AST，失败时按行切分 | 保留 Module、Class、Symbol 和行号          |
| HTML                      | DOM Parser                      | 保留 Heading、Link、List、Table、Code      |
| CSV/XLSX                  | 原生 Sheet/Cell Parser          | 保留 Formula、Value、Sheet 和 Cell Lineage |
| DOCX/PPTX                 | 原生 OOXML Parser               | 保留标题层级、Slide、Notes 和对象结构      |
| 普通数字版 PDF            | PyMuPDF                         | 保留原生 Text Block、Page、BBox 和字体信息 |
| 扫描件、复杂公式/表格 PDF | MinerU Precise VLM              | 作为复杂视觉文档的核心 Parser              |
| OOXML 复杂渲染页          | 渲染为 Page 后交给 MinerU       | 只让 MinerU 处理视觉结构复杂的部分         |

### 2.2 原生 Parser 技术栈

原生路线固定为可复现、可版本化的确定性实现，不用一个通用 Loader 吞掉全部格式：

| 层/格式       | 推荐实现                             | 必须保留的结构或控制                         |
| ------------- | ------------------------------------ | -------------------------------------------- |
| MIME          | `libmagic` / `python-magic`          | Magic、扩展名、声明 MIME 的冲突与路由理由    |
| Encoding      | `charset-normalizer`                 | 原编码、置信度、替换字符数和标准化结果       |
| TXT           | 保留 Source Offset 的逐行解析        | Line、Byte/Character Offset、换行类型        |
| Markdown      | `markdown-it-py`                     | Heading、List、Table、Code Fence、Source Map |
| JSON/YAML     | `json` + `ruamel.yaml`               | Key Path、Array Index；禁止任意对象构造      |
| XML           | `defusedxml` 预检 + 加固的 `lxml`    | 禁实体/DTD/网络/XInclude/XSLT，保留 XPath    |
| HTML          | `selectolax` + `lxml`                | DOM Path、Heading、Link、List、Table、Code   |
| CSV           | `pyarrow.csv`                        | Row/Column、Schema、原值和解析错误           |
| XLSX          | `openpyxl` + `pyarrow`               | Sheet/Cell、Formula、Cached Value、Merged    |
| DOCX          | `python-docx` + `lxml`/OOXML         | Heading、Paragraph、Table、Relationship      |
| PPTX          | `python-pptx` + `lxml`/OOXML         | Slide、Notes、Shape、Reading Order           |
| Code          | 固定 Grammar 版本的 `tree-sitter`    | File、Module、Symbol、AST Path、Line Range   |
| 普通 PDF      | `PyMuPDF`                            | Page、Block、BBox、Font、Image Reference     |
| 复杂/扫描 PDF | MinerU Precise                       | Reading Order、Formula、Table、Region Asset  |
| Office 回退   | LibreOffice Headless 渲染后进 MinerU | 固定渲染版本、页图 Hash、原对象关联          |

XML 必须先经 `defusedxml` 预检，再使用固定配置 `lxml.XMLParser(resolve_entities=False, load_dtd=False, no_network=True, huge_tree=False)` 解析；禁止 XInclude、XSLT 和外部 Schema Resolver，并继承 Rootless Parser Sandbox 的 CPU、内存、文件大小和超时限制。

所有 Parser 都必须输出同一套版本化 Canonical IR。Core Field 至少包含 `document_id`、`source_version`、`block_id`、`ordinal`、`parent_id`、Block Type、Content、Structure Path、Asset Reference、Content Hash、Parser Version 和 Config Hash。来源位置使用 Discriminated Locator Union，而不是要求每种格式伪造相同坐标：

```text
text_offset | line_range | page_bbox | slide_shape | sheet_cell | ooxml_part_xpath
```

每种 Locator 固定坐标系、单位和端点规则；不适用的字段保持 Null，无法稳定定位时写入 `needs_review=true` 和 Provenance Quality，禁止猜测 BBox/Offset。Markdown 只作为展示投影，不能替代 Canonical IR 或 Parser-native Artifact。Office 宏、外链和公式求值默认禁用；压缩包大小、展开比和解析时限必须受限。

### 2.3 MinerU 的定位

MinerU 作为复杂 PDF、扫描件、公式、表格和页面视觉结构的主要 Parser，但不强制处理本来就能确定性解析的文本、代码和表格文件。

生产候选使用带 Token 的 `mineru-precise` VLM 路线；无 Token 的 `mineru-agent` 只作为开发测试或经评测通过的回退路线。每种格式至少要求 Parse Job 成功率 `>=99%`、Page/Source Provenance `=100%`、非空页面覆盖率 `>=99%`、Reading-order Agreement `>=0.95`；表格格式另要求 Cell/Structure F1 `>=0.90`。MinerU 在某格式未达到任一门槛时，该格式不得上线；只有数据出站策略允许且 LlamaParse 固定版本通过相同门槛后，才能将其作为该格式的受控 Fallback。

Owner 已确认具备 MinerU API Credential，并确认当前 Owner 提供的 Bootstrap Corpus 是公开、无隐私数据，批准将该指定 Collection 发送至 MinerU、LlamaParse、Embedding、Reranker 和 LLM RAG Evidence。此授权不是系统全局默认，不覆盖未来用户上传的 Personal/Team Knowledge；它只关闭该公开 Collection 的数据 Owner 授权，也不替代各 Processor 的 License、SLA、Retention、Deletion、Training-use、版本固定和审计核验。

MinerU 结果不能只保存 `full.md`。必须同时保存 Parser-native ZIP/JSON、Page Image、Image/Table Asset、Reading Order、Page/BBox、Formula/Table Structure 和 Parser Version/Config Hash。Markdown 只是展示和兼容产物，不是权威解析格式。

### 2.4 是否可以只用 MinerU

如果知识库几乎全部是 PDF，可以把实际运行路线收敛为“原生文本 + MinerU”。但不建议把 TXT、JSON、XLSX、代码等文件也绕一圈交给 MinerU，因为这会引入不必要的 OCR/VLM 不确定性、成本和延迟。

## 3. Embedding 方案

### 3.1 主推荐候选

第一轮 Accuracy Bake-off 使用 Jina 托管 Embeddings API 的 `jina-embeddings-v4` Retrieval Profile 作为高容量候选：

```text
文档块角色：passage
用户查询角色：query
向量维度：2048
向量格式：float
相似度：Cosine
批量写入：按 Token/Request Limit 分批
```

Jina v4 的官方资料显示它面向多语言、多模态检索，支持 Text Retrieval、Code Retrieval、Visual Document Retrieval、Late-interaction Multi-vector 和 Matryoshka Dimension，因此值得在本项目的中文、英文、图文混合文档上实测，但这些能力不等于它已满足生产吞吐和许可要求。

实施时必须从 Jina Dashboard/API 返回结果中固定准确的 Model ID、API Version、Task/Role 参数和输出维度；不能使用浮动 Alias。Promotion 前还必须完成 `[待核验—阻断上线]` 的 License/Commercial-use 审查，并验证 SLA、Rate Limit、并发、p95/p99、错误率、版本可固定性、Region、Retention、Deletion 和 Training-use。Jina v4 Hosted 未通过这些门槛前，只能用于评测，不能成为唯一生产 Profile。

无本地 GPU 时必须准备第二条已批准的 Hosted Embedding Profile；有 GPU 时可以准备自托管 Qwen/Jina Profile。严格请求在主 Embedding Profile 不可用时 Fail Closed，不能静默切到不同 Model/Dimension。

### 3.2 2048 维高容量 Baseline

2048 维是未验证的高容量 Baseline，不预设它必然比 1024 维更准确。2048 与 1024 维必须在 Development/Validation Set 上同时比较相关性、延迟、费用和存储，预注册唯一候选后才允许进入 Frozen Holdout。Float32 下，每个 2048 维向量约 8 KiB；100 万个 Child Vector 仅原始 Dense Vector 就约 8 GiB，连同 HNSW、Payload 和 Snapshot 需要预留更大空间。

如果预计超过 100 万个 Chunk，应提前评测 1024 维、Scalar Quantization 或更大磁盘，而不是索引建成后临时修改。任何 Model/Dimension 变化都创建新 `index_generation_id` 并全量 Re-embed，不能在同一 Collection 混用。

### 3.3 对照模型

Jina v4 不能凭 Vendor 名称直接胜出。至少保留以下对照：

- `Qwen3-Embedding-8B`：本地或托管，完整维度与 2048 维分别评测。
- `Qwen3-Embedding-4B`：资源/成本对照。
- Jina v4：Hosted Accuracy 候选。

先在 Development/Validation Set 上按 Recall@50、nDCG@10、MRR、中文 Slice、表格/精确 Identifier Slice 和 No-answer 行为选出并预注册唯一候选。Frozen Holdout 只用于一次最终 Promotion，不用于继续选择模型或调参；如果根据 Holdout 结果修改 Profile，必须重新冻结一份未见过的新 Holdout。

## 4. 检索与重排序

### 4.1 Qdrant Index Profile

Qdrant 是首个 Bake-off 候选，不是已选定依赖。实施时先固定待测的 `>=1.17` 具体 Image Digest 和 Client Version，再用同一 Parser、Vector、Lexical Rule、Filter、Fusion 和 Reranker 与 OpenSearch/Vespa Profile 比较；只有通过主设计的相关性、ANN、ACL/Filter 和恢复门槛后才能成为 Active Engine。

```text
Dense Vector：dense_jina_v4_2048
Title/Summary Vector：title_summary_v1
Lexical：bm25_v1
Exact：exact_key_phrase_v1
Visual Multi-vector：visual_jina_v4_v1，评测通过后启用
Payload：scope + owner_user_id|team_id + collection_id
         collection_acl_revision + collection_visibility_epoch
         document_id + document_version_id + source_version
         document_visibility_epoch + document_status
         index_generation + projection_revision + source_span_id/hash + active
```

BM25 与 Exact 不能合并：BM25 处理一般词法相关性；ID、Version、Path、Quoted Phrase、大小写敏感 Symbol 使用独立 Exact Lane。

首个 Lexical Manifest 候选明确为 `compute_engine=qdrant-server-bm25`、`model=qdrant/bm25`、`modifier=idf`、`k=1.2`、`b=0.75`、`language=none`、`tokenizer=word`、`lowercase=true`。`avg_len` 在 Generation Build 时根据 `lexical_text_v1` 的实际 Token Mean 计算为一个数值，并冻结进 Manifest；Ingest 与 Query 必须读取同一数值。Python 生成 `lexical_text_v1`，执行 Unicode NFKC；中文使用固定版本和 Dictionary Hash 的 Jieba 预分词，拉丁文本使用 Lowercase、无 Stemming、无 Stop-word Baseline。如果准确 Qdrant 版本不能接受并复现这些参数、Corpus Statistics、中文 Token 和 Phrase Position，则该 Profile 失败，改测 OpenSearch/Vespa，不能在应用层伪装成同一 BM25。

Exact Lane 由 Python 查询 Payload Index：Raw Keyword Equality 为 Tier 3，Normalized Keyword Equality 为 Tier 2，Position-aware Phrase 为 Tier 1；按 Tier、Field Priority、Document ID、Source Span ID 确定性排序并截取 Top-50。Qdrant Filter 只负责匹配，不被当作相关性排序器。Raw 与 Normalized Field、Phrase Analyzer、Case Rule 和 Tie-breaker 都进入 Profile Digest。

### 4.2 Candidate 与 RRF 起始值

```text
Dense Child       top 100
BM25              top 100
Exact             top 50
Title/Summary     top 50
Visual            top 50，仅视觉 Query Class
Candidate Union   100-200
```

初始 Weighted RRF Profile 使用 `k=60`。公式与 Qdrant v1.17 Profile 保持一致，其中 `rank_i` 是 Zero-based Rank：

```text
score(d) = sum(1 / (k + (rank_i(d) + 1) / weight_i - 1))
```

每个 Query Class 的 Enabled Lane、Prefetch Order 和 Weight Array 完整锁定，数组长度必须与 Lane 数量一致：

| Query Class             | Enabled/Prefetch Order                    | Weight Array            | Explicitly Disabled    |
| ----------------------- | ----------------------------------------- | ----------------------- | ---------------------- |
| Semantic                | `[dense,bm25,exact,title_summary]`        | `[1.0,0.8,0.5,0.6]`     | `visual`               |
| Identifier/Version/Path | `[dense,bm25,exact]`                      | `[0.5,1.2,2.0]`         | `title_summary,visual` |
| Comparison/Multi-hop    | `[dense,bm25,exact,title_summary]`        | `[1.0,1.0,0.5,0.8]`     | `visual`               |
| Visual Document         | `[dense,bm25,exact,title_summary,visual]` | `[1.0,0.7,0.3,0.5,1.2]` | none                   |

这些只是 Development/Validation Bake-off 起点，不能直接作为永久生产常量。Query Class Router、Normalizer、Lane 顺序、`k`、Weight Array 和 Fallback Rule 必须有版本与统一 Profile Digest。某 Lane 因 Query Class 明确禁用时才允许省略；运行故障导致 Lane 缺失时，Strict Request Fail Closed，Optional Request 明示降级，不能偷偷重算权重。

### 4.3 Reranker

Cross-encoder Reranker 是正式 Profile 的必需阶段，不是可有可无的优化。第一轮比较：

- Jina Dashboard 当前可用的固定版本 Multilingual Reranker。
- `Qwen3-Reranker-8B`。
- `BAAI/bge-reranker-v2-m3`。

将 40-100 个 Candidate 重排为 12-20 个 Evidence Child，再动态扩展 Parent/Sibling。只有 Reranker 在 Development/Validation 上达到 `nDCG@10 +0.03`，且从 Baseline Top-10 至少一个 Qrel 退化到 Candidate Top-10 无 Qrel 的 `relevant-drop <=2%`，才能作为预注册候选进入 Frozen Holdout。

## 5. Chunk 与 Context Profile

```text
Parent：Section-aligned，目标 1400-1600 Token，最大 2000
Child：目标 384 Token，最大 512
Overlap：仅长语义 Block 被迫拆分时使用 48 Token
Breadcrumb：Document > Product/Version > Section > Block Type
Context Expansion：按证据需要扩展 Parent 或最小 Sibling Window
```

表格不使用普通文本 Overlap，而是重复 Header 并保留 Cell Lineage；代码携带 Signature/Import；Slide 通常一个 Parent；Spreadsheet 按 Row Group 分 Child。

每个 Child 同时保存不可变 `source_text` 和用于检索的 `retrieval_text`。生成的 Context/Summary 只能帮助检索，不能作为 Source Citation。

## 6. 服务与数据流

```text
Next.js
  -> Go API：Auth、ACL、Files、Chat、Citation、Strict Answer、SSE
     -> Python rag-api：Query Routing、Embedding、Hybrid Retrieval、Rerank
     -> Python rag-worker：Parse、Canonicalize、Chunk、Embed、Index
        -> MinerU API：复杂文档解析
        -> Jina/第二候选 API：Embedding/Reranker
        -> Postgres：权威 Metadata、ACL、Version、Job、Outbox
        -> MinIO：Original + Parser-native + Canonical Artifact
        -> Qdrant/OpenSearch/Vespa 候选：可重建 Search Projection
        -> Redis：非权威 Wake-up/Lease/Rate-limit/Cache
```

旧项目的 `DEFAULT_RAG_BASE_URL` 不能直接填写 Jina URL。旧接口要求 `/upsert-data`、`/query-data` 和 `/delete-data`，而 Jina 只负责生成向量。新 Python Sidecar 必须调用获批的 Active Embedding Profile，并读写获胜 Search Projection；Jina 只是当前候选示例。

## 7. 单服务器部署建议

本地服务器运行：

```text
Next.js + Go + Python rag-api/rag-worker
Postgres + Redis + MinIO + 获胜 Search Engine
```

外部运行：

```text
MinerU Precise 候选 API
Jina/第二候选 Embeddings/Reranker API
LLM Provider
```

这样本机不需要为 8B Embedding/Reranker 或 VLM Parser 配置 GPU，但“当前没有可用 GPU”和服务器 CPU/RAM/磁盘容量仍是 `[待确认—阻断上线]` 的容量假设。必须给 Go、Postgres 和搜索引擎设置资源保底；Parse/Embed Job 使用有界 Queue 和并发限制，不能抢占在线 Chat。

如果未来增加本地 GPU，再用相同 Development/Validation 比较自托管 Jina/Qwen/MinerU；预注册唯一候选后才执行 Frozen Holdout Promotion。

### 7.1 两轴版本与发布

所有 Profile 迁移严格引用主设计 §8.2，同时保留：

- `index_generation_id`：Model、Dimension、Parser/Chunker、Analyzer、Vector Shape 和 Rank Profile；
- `corpus_projection_revision`：日常 Upload、Update、ACL、Version 和 Delete 的单调内容状态。

新 Profile 从声明的 Postgres Source Watermark 构建 Shadow Generation，Replay Outbox 到 Publish Watermark，验证 Count/Hash/ACL/ANN/Retrieval 后，再由一个 Postgres Transaction 发布绑定 Physical Collection、Manifest Hash 和 Applied Watermark 的 Active Pointer。Rollback Window 内旧 Generation 必须继续双写兼容 Mutation；否则回滚前先 Replay 追平并保持 Unready。不能直接修改当前 Collection，也不能只凭 Alias 切换宣称一致。

### 7.2 小团队 Personal/Team Knowledge ACL

Owner 已确认目标是小团队：每个用户拥有自己的 Personal Knowledge，另有 Team Knowledge，由 Team Admin 管理。当前 Go Backend 只有按 `user_id` 的资源隔离，尚无 Team、Membership、Knowledge Collection 或 Processing Consent Schema，因此下列模型是 Phase 15 必须实现的前置契约，不代表当前运行时已经支持。

```text
teams(id, created_by_user_id, membership_revision, deleted_at)
team_memberships(team_id, user_id, role=admin|member, status)
knowledge_collections(id, scope, owner_user_id?, team_id?, acl_revision, visibility_epoch, collection_processing_revision)
knowledge_documents(id, collection_id, current_version_id?, status, deleted_at)
knowledge_document_versions(id, document_id, file_id, source_version, visibility_epoch, status, content_hash)
user_query_consent_state(user_id, query_consent_revision)
processor_governance_profiles(id, processor, endpoint, model_api_version, governance_revision, manifest_hash)
processor_governance_heads(processor, endpoint, status, active_profile_id?, head_revision)
processing_consents(subject, processor, governance_profile/revision, purposes, data_types, policy_version, status)
```

Personal Collection 必须且只能绑定 `owner_user_id`；Team Collection 必须且只能绑定 `team_id`。`workspace` 继续表示聊天 UI 分组，不得静默等同 Team/IAM Scope。Team Membership Role 与全局 User Role 分离；系统至少保留一个 Active Team Admin，最后一个 Admin 的降级、删除或离队必须事务化拒绝。

| Principal                 | Personal Knowledge        | Team Knowledge                                 |
| ------------------------- | ------------------------- | ---------------------------------------------- |
| Personal Owner            | 上传、查询、删除、授权    | 仅按其 Team Membership                         |
| Team Admin                | 不得访问他人 Personal     | 上传、查询、删除、成员管理、Collection Consent |
| Team Member               | 仅自己的 Personal         | 默认只查询；不得上传、删除或改变 Consent       |
| Worker/External Processor | 仅访问签名 Job Capability | 无授权决策权                                   |

默认采用 Admin Invite、禁止公开注册、每用户独立 Session。Invite Acceptance 建立 Argon2id Password Credential，后续使用 Verified Email/Password 登录；Recovery Token 只发送到 Verified Mailbox，Team Admin 不得重置成员 Credential。若以后需要成员维护 Team Knowledge，新增 `contributor` 权限而不是扩大 `member`。请求必须显式提交非空 Collection ID 列表；Go 根据当前 User、Membership Revision 和 Collection Row 计算 Allowed Collections，拒绝客户端提供的 `user_id`、`team_id`、`acl_groups` 或 Impersonation Hint。Search Payload 和 Cache Key 使用 Contract 中规范化的 Collection/Document/User/Governance Fence。

Future API、权限矩阵、Consent、File Binding、Deletion 和测试要求由 [`knowledge-acl-api.md`](../contracts/knowledge-acl-api.md) 定义；它是 Phase 15 Contract，不代表当前 Backend 已实现。

## 8. 数据出站与安全

数据出站按 Processor × Data Type 独立授权，未明确批准的一律 Default Deny。Owner 只为当前指定的公开 Bootstrap Collection 批准下表全部 Processor 处理该 Collection 的原文件、资产、Chunk、Candidate 和 Evidence；此决定不能替未来用户授权其 Query Text。Personal/Team 上传默认 `non-public + external processing denied`，不得继承 Bootstrap Collection 的授权；UI 告知不能替代 Consent。

| Processor           | 可能发送的数据                               | 用途                           | 授权状态与上线控制                                                               |
| ------------------- | -------------------------------------------- | ------------------------------ | -------------------------------------------------------------------------------- |
| MinerU              | 原文件、Page Image、表格/图片页              | 文档解析                       | `[Scope Consent 已确认—Bootstrap Public Collection]`；Governance 通过前禁止调用  |
| LlamaParse          | 原文件、Page Image、表格/图片页              | MinerU 受控回退                | `[Scope Consent 已确认—Bootstrap Public Collection]`；仍保持 Disabled            |
| Jina/其他 Embedding | Query、`retrieval_text`、代码、可选图片 Crop | Query/Passage/Visual Embedding | `[Collection Consent 已确认—Bootstrap Public]`；每个 User 仍需 Query Consent     |
| Hosted Reranker     | Query、40-100 个 Candidate Child、Breadcrumb | 相关性重排                     | `[Collection Consent 已确认—Bootstrap Public]`；每个 User 仍需 Query Consent     |
| LLM Provider        | Query、最终 Evidence Span、可选授权 Crop     | 生成答案与 Claim Verification  | `[Collection Consent 已确认—Bootstrap Public]`；User Query Consent + Hosted Gate |

每个 Processor × Data Type 决策记录 Consent Version、Owner、Approved Region、Retention、Deletion API、Training-use、审计事件和撤回流程。所有调用先通过当前 Active Governance Head 指向的不可变 Approved Profile，并匹配 Purpose/Data Type/Endpoint/Model/Region；Parse/Passage Embedding 再检查 Collection Consent，Query Embedding 检查当前 User Query Consent，Rerank/Answer/Evidence 同时检查 User Query Consent 和所有选中 Collection Consent。Personal Content 由其 Owner 授权；Team Content 由 Team Admin 授权；上传动作本身不构成 Consent。撤回后推进 `collection_processing_revision`、`user_query_consent_revision` 或 Governance Head Revision，停止对应外部调用并清 Cache；它不伪造 ACL/Visibility 变化。只有内容权限收紧或删除才推进 ACL/Visibility Fence 和 Search Tombstone。

所有外部 Processor Key 只保存在服务器 Secret 中，不下发浏览器。日志不记录文档正文、Query、Evidence 或完整 Provider Response。Go 在 Prompt Assembly 和严格回答 Commit 前分别再次校验 ACL/Version/Visibility。外部服务任一条款不满足要求时，该 Profile 保持 Disabled。

## 9. 评测与上线门槛

首批建立至少 500 个 Human-reviewed Question，并在任何 Bake-off 前冻结 `60% Development / 20% Validation / 20% Frozen Holdout`：

```text
150：普通事实与精确查询
100：表格、数字、公式、图片
100：多文档比较与 Multi-hop
50：No-answer / Evidence Insufficient
50：ACL、删除、版本隔离
50：Prompt/Multimodal Injection
```

分类可以交叉，但每个 Critical Slice 至少 50 个 Case。所有模型、Dimension、Parser Route、RRF、Reranker 和 Engine 选择只使用 Development/Validation；预注册唯一候选后，Frozen Holdout 只运行一次最终 Promotion。如果依据 Holdout 结果继续修改 Profile，该 Holdout 立即失效并必须重新采集。

以下是主设计 §9 的增量摘要，不替代主设计中的全部 Absolute/Relative Gate。上线必须同时满足：

```text
Parser per-format quality thresholds = passed
Source/Version/Page/Span/Hash Provenance = 100%
Candidate Recall@50 >= 0.95
Final Recall@10 >= 0.90
nDCG@10 >= 0.85
MRR@10 >= 0.80
Citation Correctness >= 0.95
Citation Completeness >= 0.90
Faithfulness >= 0.95
No-answer False-answer <= 2%
Visual Region Recall@10 >= 0.90
Table Exact-answer >= 0.95 and Cell Lineage = 100%
ACL/Delete/Secret/Tool Leakage = 0
```

每次 Promotion 还必须满足预注册 Primary Metric 的 95% Paired-bootstrap Confidence Interval 超过 Minimum Useful Gain，且任何关键 Slice 不超过 Regression Budget。Jina、Qwen、不同维度和不同 RRF/Reranker Profile 必须使用同一 Development/Validation、Parser Output 和 Chunk Manifest 比较，一轮只改变一个变量。

Hosted Profile 另需通过 License/Commercial-use、SLA、Rate Limit、Expected Concurrency、p95/p99、错误率、版本固定、Region/Retention/Deletion/Training-use 和成本预算门槛；相关性通过不能替代生产可用性。

## 10. 推荐的实施顺序

1. 锁定 Canonical Block、Chunk、Evidence、Citation 和 ACL Schema。
2. 建立至少 500 个问题的 Development/Validation/Frozen Holdout 与 Source-span Qrels。
3. 接入原生 Parser 与 MinerU Precise，保存完整 Parser-native Artifact。
4. 实现 Parent/Child/Window Chunk 和 Stable Content Hash。
5. 在 Development/Validation 接入 Jina v4 与第二个获批 Profile，评测 2048/1024 维 Shadow Generation；暂不指定生产赢家。
6. Bake-off Qdrant/OpenSearch/Vespa 边界，锁定 Lexical Manifest、Exact Lane、RRF、ACL Filter 和 Profile Digest。
7. 加入 Jina/Qwen/BGE Reranker Bake-off。
8. 接入 Go Citation、Strict-grounded Buffer/Verify/Commit/SSE。
9. 完成 Delete Fence、两轴 Generation/Projection 发布、Outbox Replay、Backup/Restore 和 Injection Drill。
10. 预注册唯一候选，执行一次 Frozen Holdout，并同时通过生产 SLA/License/数据出站门槛后发布第一套 Active Profile。

## 11. 请 Owner 重点确认的修改点

### 11.1 当前推荐

| 决策          | 吾的推荐                                                      | 需要修改的情况                                    |
| ------------- | ------------------------------------------------------------- | ------------------------------------------------- |
| MinerU 范围   | 只处理 PDF/扫描/复杂视觉页                                    | 语料几乎全是 PDF 时可扩大                         |
| LlamaParse    | 默认关闭                                                      | MinerU 在某类格式未过门槛且 LlamaParse 通过时启用 |
| Knowledge ACL | 每用户 Personal + Team Shared；Admin 管 Team，Member 默认只查 | 需要成员维护 Team 文档时新增 Contributor          |
| 数据出站      | 仅 Bootstrap Public Collection 的全 Processor Consent 已批准  | Personal/Team 必须按 Collection/User 单独授权     |
| Embedding     | Jina v4 2048/1024 作为 Development/Validation 候选            | SLA/许可未过时换第二 Hosted 或自托管 Profile      |
| Reranker      | 必须启用并通过增量门槛                                        | 没有通过门槛的模型时暂不发布 RAG                  |
| Search Engine | Qdrant-first Bake-off                                         | OpenSearch/Vespa 同 Profile 明显更优时更换        |
| 本地 GPU      | 暂按没有 GPU 规划                                             | 有 GPU 后增加自托管对照，不直接替换               |
| Strict Answer | 默认知识库问答启用                                            | 普通聊天只做 Optional Enrichment                  |

### 11.2 阻断性假设与待确认项

第 11.2 节任何“阻断上线 = 是”的项目未关闭前，本 Draft 不得 Owner Lock，也不得 Promotion；标记为“阻断调用”的项目还禁止首次真实外部调用：

| 项目                                          | 状态                | 当前证据                                        | Owner           | 截止时间                  | 阻断上线 |
| --------------------------------------------- | ------------------- | ----------------------------------------------- | --------------- | ------------------------- | -------- |
| 主要文档格式与语言分布                        | `[待确认—阻断上线]` | 仅有源码能力清单，无真实 Corpus 统计            | 产品 Owner      | Phase 15 实施前           | 是       |
| 文档量、Chunk 数和一年增长                    | `[待确认—阻断上线]` | 无容量数据                                      | 产品 Owner      | Index Schema 锁定前       | 是       |
| 单机 CPU/RAM/磁盘和可用 GPU                   | `[待确认—阻断上线]` | 暂按无 GPU 规划                                 | 运维 Owner      | Compose 设计前            | 是       |
| 小团队 Personal/Team Knowledge 模型           | `[已确认]`          | 每用户私有库 + 团队库 + Team Admin              | 产品 Owner      | 已完成                    | 否       |
| Bootstrap Public Collection Scope Consent     | `[已确认]`          | 仅该指定公开 Collection 允许全部表列处理        | 数据 Owner      | 已完成                    | 否       |
| 全部 Active Processor Governance              | `[待核验—阻断调用]` | Region/Retention/Deletion/Training/Audit 未记录 | 技术/数据 Owner | 各 Processor 首次调用前   | 是       |
| LlamaParse 候选启用                           | `[Disabled]`        | 已获 Scope Consent，但不是当前必需 Profile      | 技术 Owner      | MinerU 某格式未过门槛时   | 否       |
| MinerU Precise API Credential                 | `[已确认]`          | Owner 确认凭证可用，Secret 未写入文档           | 产品 Owner      | 已完成                    | 否       |
| Jina API Credential                           | `[已确认]`          | Owner 确认凭证可用，Secret 未写入文档           | 产品 Owner      | 已完成                    | 否       |
| Jina v4 License/Commercial-use/SLA/Rate Limit | `[待核验—阻断上线]` | 仅确认模型能力，未核验生产条款                  | 技术 Owner      | Hosted Promotion 前       | 是       |
| 第二 Hosted 或自托管 Embedding Profile        | `[待选型—阻断上线]` | 尚未选定                                        | 技术 Owner      | Reliability Test 前       | 是       |
| Hosted Reranker 的准确 Model ID 与 SLA        | `[待核验—阻断上线]` | 仅有候选列表                                    | 技术 Owner      | Reranker Bake-off 前      | 是       |
| Search Engine 与 Lexical Compute Engine       | `[待评测—阻断上线]` | Qdrant-first，尚未胜出                          | 技术 Owner      | Index Profile 锁定前      | 是       |
| Golden Set 标注人力与 Reviewer                | `[待确认—阻断上线]` | 目标至少 500 Questions                          | 产品 Owner      | Corpus 构建前             | 是       |
| 月成本预算、Query SLO、RPO/RTO                | `[待确认—阻断上线]` | 尚无预算和目标                                  | 产品/运维 Owner | Production Profile 锁定前 | 是       |

Owner 只需一次回答 §11.3 中归属产品/运维的项目；回答后立即更新本表的状态、证据和决策，不把结论只留在聊天记录里。归属技术 Owner 的阻断项由实施调研与 Bake-off 关闭。

Provider License/SLA/Region/Retention、模型版本、第二 Embedding Profile、Reranker、Search Engine、Parser Route、Chunk/RRF/Top-K 和安全一致性实现由技术团队调研与 Bake-off 关闭，不再作为 Vendor 选择题反复询问 Owner。只有技术候选无法同时满足 Owner 给出的业务、预算、SLO 或容量边界时，才重新提交取舍。

### 11.3 一次性 Owner 问卷

Owner 要求剩余问题一次问完。回复“全部按默认”即接受每项推荐值，只需补充无法从代码或 Corpus 自动发现的信息：

1. **产品行为与技术授权**：知识库问答默认 `strict_grounded`，证据不足即拒答；普通聊天 RAG 失败时告警并继续；技术团队可在预算/SLO 内依据 Bake-off 自主选择 Processor、模型、维度和搜索引擎。推荐：全部接受，不再逐 Vendor 复问。
2. **公开范围与未来用户**：`[已回答]` 小团队使用；每个用户可以上传自己的 Personal Knowledge，另有 Team Knowledge 和 Team Admin。安全默认：Team Admin 不得读取他人 Personal；Member 默认只查询 Team Knowledge；Personal/Team 上传不继承 Bootstrap Public Collection Consent。
3. **代表 Corpus 与首发优先级**：真实文档位于哪个目录、Bucket 或导出包？哪些语言、格式和 Query Class 必须首发保障？推荐：技术团队扫描实际 Corpus；保障中文、英文以及占比 `>=5%` 或业务 Critical 的格式，优先 PDF、DOCX、PPTX、XLSX/CSV、Markdown/TXT、HTML 和代码。
4. **12 个月工作量上限**：`[部分回答：小团队]` 仍需预计文档数、原始容量、年增长、每日变更、每日 Query 和峰值并发。推荐规划上限：`100,000` 文档、`1,000,000` Active Chunks、年增长 `20%`、`500` 文档变更/日、`10,000` Query/日、`10` 个并发 Strict Query；未知时先以真实 Corpus 扫描结果校准。
5. **目标服务器与扩容**：VPS 所在 Region、vCPU、RAM、磁盘类型/容量、GPU，以及是否允许升级配置？推荐规划：单机 CPU-only，`16 vCPU / 64 GiB RAM / 2 TiB NVMe`，长期保留 `>=40%` 空闲；重型推理使用 Hosted Processor。
6. **第三方凭证可用性**：`[部分回答]` MinerU 与 Jina API Credential 已确认；Hosted Reranker 的准确 Endpoint/Model 和可选 LlamaParse Credential 尚未确认。不得把 Secret 写入聊天或文档；LlamaParse 没有也保持 Disabled。
7. **Golden Set 人力**：谁负责业务标注、独立复核和争议裁决？推荐：500 Questions，`60/20/20`；1 名业务标注人覆盖全部，1 名独立 Reviewer 复核 Holdout、安全 Case、争议和 20% 分层样本，产品 Owner 裁决，预留约 15 人日。
8. **成本边界**：Bake-off 一次性预算和生产每月 RAG 增量预算是多少？推荐：Bake-off 上限 `¥5,000`，生产 RAG 增量上限 `¥3,000/月`，70%/90% 告警；达到 100% 时停止新实验和批量索引，不破坏已发布查询服务。
9. **Query 与索引 SLO**：可接受的回答延迟、可用性和文档入库时效是什么？推荐：Strict Answer 端到端 `p95 <=15s`、`p99 <=30s`，月可用性 `99.5%`；普通数字文档 `p95 <=10min` 可检索，复杂/扫描 PDF `p95 <=30min`。
10. **恢复与运维**：RPO/RTO、备份位置、支持窗口和运维责任人是谁？推荐：`RPO <=24h`、`RTO <=4h`，每日协调备份、至少一份加密 Off-host 副本、每月 Restore Drill；1 名 Primary 和 1 名 Backup 运维责任人。

建议回复模板：

```text
1. 全部按默认（或列出修改）
2. 公开范围/未来用户：已回答（Personal + Team + Admin）
3. Corpus 位置与首发重点：
4. 工作量：默认 / 修改为...
5. VPS 配置与 Region：
6. Jina/Reranker/LlamaParse 凭证：有/待确认/待确认
7. 标注人、Reviewer、裁决人：
8. 预算：默认 / 修改为...
9. SLO：默认 / 修改为...
10. RPO/RTO、备份位置、运维人：默认 / 修改为...
```

## 12. 与主设计文档的关系

本文件是供 Owner 修改的具体推荐 Profile，不替代 [`phase-15-accuracy-first-rag-design.md`](./phase-15-accuracy-first-rag-design.md) 中的安全、一致性和评测原则。

Owner 确认后再把以下内容合并进主设计：

- Parser 推荐路由与 MinerU Precise 定位；
- Jina v4 Hosted 2048/1024 高容量候选、生产门槛和第二 Profile；
- Qdrant-first Engine Bake-off、Lexical/Exact Manifest、RRF 起始值与 Reranker 候选；
- 单服务器外部推理部署边界；
- 小团队 Personal/Team Knowledge ACL 与 Membership Revision；
- Processor-scoped Default-deny、Bootstrap Public Collection 授权边界、阻断性假设和 500 Question Evaluation Plan。
