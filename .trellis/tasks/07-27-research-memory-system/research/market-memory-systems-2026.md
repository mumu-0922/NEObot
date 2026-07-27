# 2026 Agent Memory 市场调研

调研日期：2026-07-27。所有 benchmark 数字均为厂商/项目官方自报，不视为独立
复现结果；不同数据集、模型、Top-K、token budget 和 judge 不可直接横比。

## 审计口径

- **源码级审计**：固定 commit checkout，检查运行入口、存储、auth/tenant、删除、
  recall/capture 和部署配置，并记录可复核路径。
- **文档级初筛**：只用于建立候选名单和核对官方定位，不能单独支撑“最佳方案”。
- Hermes Agent 与 TencentDB Agent Memory 已完成源码级审计，证据另见
  [`reference-projects-hermes-tencentdb.md`](reference-projects-hermes-tencentdb.md)。
- Hindsight、Supermemory、Mem0、Graphiti、Cognee 已完成固定 commit 六轴审计，
  证据见 [`source-audit-top-memory-engines.md`](source-audit-top-memory-engines.md)。
  Supermemory 的审计结论是“公开仓库不含 Local Memory 核心源码”，因此仍只能作为
  binary black-box / 文档级候选，不能与可审计 engine 同档。

## 结论先行

不存在一个在所有维度都绝对“最强”的方案。2026 年多个厂商都声称榜单第一，
但指标并不相同：

- **最强可审计现成引擎：Hindsight**；Memory 主链 PostgreSQL-only 且 recall 完整，
  但删除残留和 migration 权限决定它只能先做独立 database shadow。
- **公开榜单声量强但核心不可审计：Supermemory Local**；公开仓库没有
  `supermemory-server` 的核心源码/build manifest。
- **通用 Memory SDK / 成熟生态：Mem0**；当前 self-host server 的 auth 与 Memory
  scope 脱节，删除也不是隐私擦除。
- **时间、关系、多跳专项路线：Graphiti**；示例 Server 无 auth、delete 不完整、
  ingestion queue 非 durable。
- **完整 knowledge platform：Cognee**；当前 Postgres hybrid 原子 adapter 未接通，
  background 非 durable，`forget(everything=true)` 还会跨用户清空共享 cache。
- **最适合 Neo Chat 的生产实现：原生 PostgreSQL Memory v2**；借 Hermes 的
  provider contract 与 TencentDB Agent Memory 的 L0→L3 分层，Hindsight 只作为
  可替换 shadow adapter 同场测试，不能直接把用户主权交给 vendor server。
- **中文真实效果冠军尚未确定**；固定源码审计解决工程适配问题，效果必须再跑同一
  frozen benchmark。

## 1. Hindsight

### 能力

- MIT；可 self-host REST/UI、Python embedded 或接外部 PostgreSQL。
- 官方架构只依赖 PostgreSQL：pgvector/HNSW、全文检索、JSONB、recursive CTE
  graph；生产要求 PostgreSQL 15+ 与 pgvector 0.5+，支持独立 schema。
- `retain` 抽取事实、时间、entity、relationship 并规范化；`recall` 并行执行
  semantic、BM25、graph、temporal，随后 RRF + cross-encoder rerank + token
  trimming；`reflect` 形成 mental model。
- 可配置 OpenAI-compatible LLM endpoint；reranker 有原生 SiliconFlow provider，
  默认模型即 `BAAI/bge-reranker-v2-m3`。向量也可通过 OpenAI-compatible
  embedding endpoint 接现有 BGE-M3。
- 有 bank、metadata filter、async operation/retry、memory/bank delete、定制
  tenant extension 和 API key auth。

### benchmark 与边界

项目称其 LongMemEval 为 2026 SOTA，并称 Hindsight 自身结果由 Virginia Tech
Sanghani Center 与 The Washington Post 合作者复现；其余对比项仍多为厂商自报。
这比纯 vendor demo 更有价值，但仍不能代替 Neo Chat 的中文、安全、延迟和成本
复测。

关键限制：

- 默认无 auth；生产必须启用 built-in shared API key 或自写 TenantExtension，
  最稳妥仍是只开放给 Go 内网代理。
- Hindsight 文档明确 `pg_textsearch` backend 为 English-only；CJK 推荐
  PGroonga 或 `pg_search` 中文 tokenizer，会增加 extension。能否仅靠 BGE-M3 +
  rerank 达到中文门槛必须实测。
- `delete_bank()` 能清主要 bank 表，但 `audit_log`、`llm_requests`、
  `graph_maintenance_queue` 和 native `file_storage` 不都在同一 cascade erase contract；
  晋升前必须补测 request/LLM trace、源文件和 queue 清理。
- startup migration 默认开启，pool 默认 `5..100`；若 pgvector 不在 `public` schema，
  migration 会尝试 `DROP EXTENSION vector CASCADE` 后重建。Shadow 必须使用**独立
  PostgreSQL database + 独立 role**，不能只用同 database 不同 schema。

### 对 Neo Chat

**这是现成引擎首选 shadow 对照。** 它比 Mem0/Cognee 更直接复刻目标 v2 pipeline，又不
要求 Neo4j/FalkorDB，且能复用 PostgreSQL 与 SiliconFlow。正确接法是：Go 保持
用户/设置/API authority，Hindsight 只作内网、可重建 derived engine；先 shadow
比较，绝不能一上来替换现有表和接口。

来源：

- <https://github.com/vectorize-io/hindsight>
- <https://hindsight.vectorize.io/developer/storage>
- <https://hindsight.vectorize.io/developer/configuration>
- <https://hindsight.vectorize.io/api-reference>
- <https://arxiv.org/abs/2512.12818>

## 2. Supermemory

### 能力

- MIT；云平台外还有 `supermemory local` 单文件 self-host server。
- 自动抽取、user profile、知识更新/冲突、expiration/forgetting、graph memory、
  hybrid search、文件/RAG 均在一套 API。
- 官方自报 LongMemEval Recall@15 为 95%，只加入约 720 context tokens，并称在
  LongMemEval、LoCoMo、ConvoMem 均为第一；提供 MemoryBench 复现框架。

### 关键边界

- Local 使用 embedded graph engine 和单目录存储，不复用 Neo Chat PostgreSQL；
  local 是单机单组织、单 API key，只有 server logs。
- Local 默认 `Xenova/bge-base-en-v1.5`，中文需换 multilingual embedding。
- 官方文档明确 Enterprise 使用专有、长程数据调优模型，而 Local 用用户自带
  LLM；云端榜单成绩不能推定为 self-host local 成绩。

### 对 Neo Chat

适合个人离线工具或快速 demo，但会新增另一套嵌入式数据库/备份面，且 self-host
质量与榜单平台存在模型差异。更关键的是，当前公开仓库只含 web/docs/SDK/
integration，没有 `supermemory-server` 核心源码或可重现 build manifest；安装器下载
预编译 binary。它是 binary black-box，不进入可审计引擎 PoC 顺序。

来源：

- <https://github.com/supermemoryai/supermemory>
- <https://supermemory.ai/docs/self-hosting/overview>
- <https://supermemory.ai/docs/self-hosting/local-vs-enterprise>
- <https://supermemory.ai/research>

## 3. Mem0

### 能力

- Apache-2.0；既可作为 Python/Node library，也有 self-hosted REST server。
- Self-host server 默认 Postgres + pgvector，另用 SQLite 保存 history 与最近 raw
  messages，并有独立 `mem0_app` auth database。
- 当前真实 V3 pipeline 为 ADD-only：从 user 与 assistant 双方抽取，existing memories
  只作 dedup/link，最终只有 MD5 exact dedup；不会 supersede 旧事实。
- 支持 expiration、metadata、reranker 和多种向量后端。

### 官方自报 benchmark

在 top_200、单次 retrieval 的平台配置下：

| Benchmark | Score | Mean tokens/query |
| --- | ---: | ---: |
| LoCoMo | 92.5 | 6,956 |
| LongMemEval | 94.4 | 6,787 |
| BEAM 1M | 64.1 | 6,719 |
| BEAM 10M | 48.6 | 6,914 |

官方同时明确：这些分数来自 managed platform，其中有 OSS SDK 不具备的专有
优化；OSS 只能期望方向相似，不能把平台分数当成自托管结果。BEAM 10M 的
temporal、event ordering、multi-session 仍明显较弱。

### 对 Neo Chat

只适合做成熟行为基线，不宜直接接管生产数据：JWT/API key 认证得到的 user 没有
绑定 Memory `user_id`，已认证调用者可读写任意传入 scope，按 ID 的 get/update/
history/delete 也不做 owner check。删除 vector 后，旧正文仍保留在 SQLite history，
raw messages 也未清；vector 与 SQLite 不在同一事务。Recall 以 semantic pool 为先，
BM25-only 结果不能救回 pool 外记录；reranker 默认关闭，`reference_date` 为
platform-only。以 Python library 嵌入 RAG worker 又会让写 authority 跨语言分裂。

来源：

- <https://docs.mem0.ai/open-source/overview>
- <https://docs.mem0.ai/open-source/setup>
- <https://docs.mem0.ai/open-source/configuration>
- <https://docs.mem0.ai/core-concepts/memory-operations>
- <https://docs.mem0.ai/core-concepts/memory-evaluation>
- <https://github.com/mem0ai/mem0>

## 4. Zep / Graphiti

### 能力

- Graphiti 为 Apache-2.0 OSS temporal context graph；Zep 为其 managed/企业化
  产品路线。
- 模型是 entity nodes + fact/relationship edges + raw episodes/provenance。
- fact 有 validity window；新事实使旧事实失效而非删除，可回答“现在是什么”与
  “过去某时是什么”。
- hybrid retrieval 组合 semantic、BM25、graph traversal；支持显式
  bi-temporal 与自动 fact invalidation。

### 部署代价

- Graphiti 是 Python 框架，需要 Neo4j 5.26、FalkorDB、Neptune（Kuzu 已标记
  deprecated）之一，并要求结构化输出质量较好的 LLM。
- OSS 需要自己建设用户/会话管理、治理、dashboard 和性能调优；官方 README
  明确其性能依赖自有部署。
- Zep 托管产品有 Go SDK、治理和官方所称 sub-200ms retrieval，但会引入外部
  authority/托管依赖，不符合当前“数据留在自己的 Server”优先级。

### benchmark 边界

Zep 2025 官方 LongMemEval 报告是相对 full-context baseline 的分类提升与 token/
latency 对比，不是与当前 Mem0 相同模型、相同预算的横向竞赛；不能用两家的
数字直接定胜负。其价值更应看 temporal/provenance 数据模型。

### 对 Neo Chat

若未来出现大量“人物—项目—决定—时间线”的多跳问题，Graphiti 是应优先测试的
Graph sidecar；当前个人偏好/事实记忆场景直接上 Graph DB 属于过度建设。固定源码
还显示示例 FastAPI 无 auth，`group_id` 可由客户端自由提交；group delete 漏
Community/Saga，进程内 ingest queue 无 tombstone/generation fence，旧任务可在删除后
复活数据。因此只能隔离专项 PoC，不能直接暴露或承担删除权威。

来源：

- <https://github.com/getzep/graphiti>
- <https://help.getzep.com/graphiti/getting-started/quick-start>
- <https://blog.getzep.com/state-of-the-art-agent-memory/>
- <https://arxiv.org/abs/2501.13956>

## 5. Cognee

### 能力

- Apache-2.0；Python API server，可自托管。
- 对外抽象为 `remember / recall / forget / improve`，组合 vector、graph、
  ontology 和 session memory。
- README 声称整个 memory layer 可放进单一 PostgreSQL：关系用 Postgres graph
  backend、向量用 pgvector、session 用 SQL cache、metadata 同库。
- 官方自报 BEAM 100K 为 0.79、10M 为 0.67，并主动称其只是 directional
  signal。

### 对 Neo Chat

固定源码推翻了“单 Postgres 第二候选”的阶段判断：`pghybrid` factory 代码被注释，
graph/vector 仍由 separate engines 并发提交；background 只有进程内 task，重启只
rollback stale run、不 requeue；access-control 模式为每个 dataset 建独立 database。
同时任意认证用户的 `forget(everything=true)` 会全局 prune cache（Redis 为
`FLUSHDB`），影响其他用户；BM25 tokenizer 仅 `\w+`，不适合连续中文。当前不进入
首轮 shadow。

来源：

- <https://github.com/topoteretes/cognee>
- <https://raw.githubusercontent.com/topoteretes/cognee/main/README.md>

## 6. Letta / MemGPT

Letta 的强项是完整 stateful agent runtime：Agent SDK、session、完整消息状态、
可自修改 memory/persona 和 continual learning。2026 README 已说明旧 Letta
server 是 legacy，活跃开发转向 Letta Agent/Code 与 App Server。

这不是一个单纯可嵌入的 Memory component。接入会与 Neo Chat 已有 Go chat、
provider、tool loop、context、streaming runtime 大面积重叠，因此不作为本项目
Memory 替换方案。

来源：

- <https://github.com/letta-ai/letta>
- <https://docs.letta.com/letta-agent-sdk/overview>

## 7. LangGraph

LangGraph 提供 thread-level checkpointer、cross-thread Store、Postgres/MongoDB
持久化和 semantic search。它是低层 stateful agent orchestration 基础设施，
并不自动提供高质量的 Memory 抽取、冲突解决、时序事实管理和治理产品。

Neo Chat 已有执行图、持久化和工具循环；只为 Memory 引入 LangGraph 等于重写
orchestration，收益不匹配。

来源：

- <https://docs.langchain.com/oss/javascript/langgraph/add-memory>
- <https://github.com/langchain-ai/langgraph>

## 8. Hermes Agent 与 TencentDB Agent Memory

### Hermes Agent

Hermes 是完整 Agent runtime，不是 Memory engine。它已经把 Honcho、Mem0、
Hindsight、Supermemory 等封装成单一 `MemoryProvider` contract，并统一处理
turn-start recall、completed-turn capture、session switch/end、tool routing、失败隔离、
超时和 shutdown drain。

它不适合替换 Neo Chat 的 Go runtime，但这套 contract 是本轮最值得吸收的工程
成果。Neo Chat 应在 Go 中建立更窄的 `Recall/Capture/Forget/Rebuild/Health`
port，并用 PostgreSQL outbox 替代 Hermes 的进程内 daemon worker。

### TencentDB Agent Memory

这是 Node/TypeScript Memory component，默认 SQLite + sqlite-vec，另支持 TCVDB。
长程 Memory 为 L0 Conversation → L1 Atom → L2 Scenario → L3 Persona；SQLite
FTS5 在写入和查询两侧使用 jieba，L1 vector + BM25 通过 RRF 融合；L2/L3 以
Markdown 保持白盒可审计。

四层模型、渐进式提取、provenance 和中文检索值得借，但当前 Hermes Gateway 不能
直接用于 Neo Chat：`/recall` 丢弃动态 L1 `prependContext`，`user_id` 虽进入请求却
未用于数据目录/schema/filter，多用户会共享 persona/检索库；同时没有 edit/forget/
delete API，写队列也不是 durable outbox。默认 embedding 为关闭，所谓 `hybrid`
零配置实际会退化为 FTS-only。

README 自报 WideSearch、连续 SWE-bench 和 PersonaMem 有显著 pass-rate/token 改善，
但当前仓库未附对应完整可重放 benchmark harness、数据和模型配置，不能直接当成
Neo Chat 中文效果证据。

详细源码证据见
[`reference-projects-hermes-tencentdb.md`](reference-projects-hermes-tencentdb.md)。

来源：

- <https://github.com/NousResearch/hermes-agent>
- <https://github.com/TencentCloud/TencentDB-Agent-Memory>

## 9. 适配矩阵

此表是固定源码审计后的工程裁决，不是公开 benchmark 排名。

| 方案 | 审计等级 | Authority/隔离 | 删除/恢复 | Neo Chat 结论 |
| --- | --- | --- | --- | --- |
| 原生 PG v2（Hermes contract + Tencent layering） | 本项目源码 | **Go 原生权威** | 可用事务/outbox/tombstone 设计闭环 | **生产首选** |
| Hindsight 0.8.5 | 核心可审计 | 条件通过，必须 Go 私网代理 | 主表完整；审计/LLM/file/queue 待补门槛 | **现成 shadow 首选，独立 DB/role** |
| TencentDB Agent Memory 0.3.6 | 核心可审计 | Gateway `user_id` 未隔离 | 无 edit/forget/delete API | 只借 L0→L3/provenance |
| Mem0 2.0.14 | 核心可审计 | auth 与 Memory scope 脱节 | SQLite 留正文/raw messages | 成熟行为基线，不接管 authority |
| Graphiti 0.29.2 | 核心可审计 | 示例 Server 无 auth | group delete/queue/AOSS 不闭环 | 关系/时间专项 PoC |
| Cognee 1.4.0 | 核心可审计 | per-dataset DB；共享 cache wipe | job 不 durable；graph/vector 非原子 | 平台过重，当前不测 |
| Supermemory Local | **核心 binary black-box** | 文档仅 single org/key | 无法从公开仓库复核 | 不进入可审计 PoC |
| Zep managed/BYOC | 文档/产品级 | 外部 authority | 依赖产品契约 | 当前隐私/authority 不合 |
| Letta | runtime 级 | 与现有 runtime 重叠 | 非独立 Memory component | 不推荐 |
| LangGraph | framework 级 | 与 orchestration 重叠 | 需自行实现 Memory 治理 | 不推荐 |

## 10. 其他已筛候选

- **Memori**：agent execution/structured state 与 BYODB 值得关注，但当前 quickstart
  和增强服务明显围绕 Memori Cloud/API key，Neo Chat 自托管优先级下不胜
  Hindsight。
- **MemoryOS**：研究型分层 Memory，有 LoCoMo 论文和 Docker，但产品化
  authority/运维/接口成熟度不如前三。
- **EverOS**：local-first Markdown/SQLite/LanceDB 很适合个人 Agent 文件记忆，
  与当前 Server/Postgres authority 相反。

它们可进入未来 watchlist，不占首轮 PoC 预算。

## 11. 产品原则

OpenAI 当前 Memory 文档强调：Memory 是 helpful recall layer，不应成为必须遵守
规则的唯一来源；chat 可分别控制“使用已有 Memory”和“贡献未来 Memory”。这与
Neo Chat 现有低优先级不可信注入边界一致。

因此 AGENTS/system/assistant/project 的确定性规则继续由配置管理；Memory 只存
用户事实、偏好、长期项目、决定和必要警告。后续值得增加 per-chat use/learn
控制，但不应把规则系统迁入 Memory。

来源：<https://learn.chatgpt.com/docs/customization/memories>
