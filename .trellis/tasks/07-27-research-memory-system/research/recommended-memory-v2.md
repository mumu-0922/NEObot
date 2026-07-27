# 推荐：原生 PostgreSQL v2 + 可插拔 Shadow Adapter

## 1. 判定

不推倒当前 Memory，也不让新 Memory server 直接接管生产。把现有 v1 保留为
Go API/Postgres authority 和 lexical fallback；**生产主线改为原生 PostgreSQL
Memory v2**。架构上借 Hermes Agent 的 provider contract，数据模型借 TencentDB
Agent Memory 的 L0→L3 分层；Hindsight 只作为同 contract 下的隔离 shadow adapter，
用来证明现成引擎是否在 temporal/graph slice 真有额外收益。

目标链路：

```text
message.completed
  -> durable outbox/job
  -> bounded conversation evidence
  -> extraction + deterministic validation
  -> related-memory hybrid lookup
  -> ADD / NOOP / SUPERSEDE decision
  -> embedding + temporal/provenance write

new user query
  -> exact + BM25 + pgvector candidates
  -> RRF fusion
  -> BGE rerank
  -> validity/scope/token-budget filter
  -> Top-K lower-priority untrusted prompt block
```

分层目标：

```text
L0 = existing conversations/messages（canonical evidence，不复制 JSONL）
L1 = atomic facts/preferences/instructions + provenance + validity
L2 = derived scene/project summaries + member L1 IDs
L3 = compact versioned persona projection，可由 L1/L2 重建
```

Hindsight shadow 接法：

```text
Go auth/user/settings/API (unchanged)
  -> durable outbox / internal recall client
  -> Hindsight REST on private Compose network
  -> dedicated PostgreSQL database + dedicated role
  -> BGE-M3 embedding + SiliconFlow BGE reranker
  -> shadow diagnostics only (no prompt injection initially)
```

Hindsight bank ID 必须由 Go 生成不可猜测的稳定映射，客户端不得提交 bank/user ID；
sidecar 需启用 API key，只绑定 Compose 内网。首轮必须用独立 database + role，
禁止 Hindsight role 连接或 migrate Neo Chat database。它不是 Phase 1 的前置依赖；
native v2 和 shadow adapter 共用同一 frozen event/recall contract。

## 2. 为什么这是最适合当前项目的路线

- 复用现栈：PostgreSQL、pgvector、pg_textsearch、BGE-M3、BGE reranker 和 Python
  worker 已在项目内，无需再引入 Node/SQLite/TCVDB 或另一套用户 API。
- 契约先行：借 Hermes 的 provider lifecycle，把 native/Hindsight/未来 engine 都放
  到同一个 `Recall/Capture/Forget/Rebuild/Health` port 后面，避免 vendor 特判散落。
- 分层而不双存：借 TencentDB 的 L0→L3 progressive disclosure，但 L0 直接使用
  现有 message，L2/L3 只作可重建 projection，不创建 JSONL/Markdown 第二主库。
- Hindsight 已有 temporal/graph/RRF/rerank，可验证“完整外部 Memory engine 是否真
  比原生 flat+temporal 强”，但不再阻塞原生 v2 的 durable 基建。
- 无新增数据库服务：PoC 可复用 PostgreSQL 17 cluster/pgvector，但必须独立
  database/role；不能只用同 database 不同 schema。
- 无新增 provider：复用现有 BGE-M3 embedding/reranker profile。
- 无双重用户权威：Go 继续负责 auth、user scope、CRUD、delete、API。
- 可撤：Hindsight 只作 derived engine；失败可清空重建并回到 lexical v1。
- 中文能力可控：使用项目自己的中文 tokenizer/embedding/rerank 和测试集，不依赖
  英文 vendor demo。
- shadow sidecar 无论达标与否，durable outbox/worker/retry、benchmark 和 adapter
  contract 都是生产主线资产，无研究浪费。

TencentDB Agent Memory 不作为 sidecar 候选：当前 Hermes Gateway 丢弃动态 L1
召回、`user_id` 不参与存储隔离、没有 edit/forget/delete API，并会引入 Node 22 +
SQLite/TCVDB + Gateway 运维面。只吸收其 layering、渐进触发、provenance 和中文
tokenization 设计。详见 `reference-projects-hermes-tencentdb.md`。

五强固定源码审计也已完成，详见 `source-audit-top-memory-engines.md`。审计确认：
Hindsight 是最强可审计现成对照；Mem0 隔离/删除/冲突处理不过关；Graphiti 只适合
关系/时间专项；Cognee 的原子 Postgres adapter 未接通且有跨用户 cache wipe；
Supermemory Local 核心是公开仓库外的预编译 binary。中文效果仍需 benchmark，不能
由源码审计直接宣布冠军。

## 3. Authority 先冻结

PoC 前必须决定并写进 contract：

- 对外只有 Go `/v1/memories` 与 `/v1/memory-settings`；浏览器不直连 Hindsight。
- 原始 conversation/message 与用户手工 Memory 是 canonical data。
- Hindsight fact/entity/mental model 是 derived projection，可重建，不可成为唯一
  备份。
- user delete、memory delete、conversation delete 都必须产生可重放 tombstone；
  shadow 期间先验证传播，不自动删除现有 canonical row。
- 不允许无 contract 双写。任何晋升方案必须能从 canonical event 重建 sidecar。

## 4. 原生 v2 数据模型退路

首期 migration 即预留，避免之后反复改表；具体命名在技术方案中冻结：

- `status`: active / superseded / expired / rejected（rejected 可只留审计）。
- `valid_from`, `valid_to`, `observed_at`, `expires_at`。
- `supersedes_memory_id` 或同一 fact key 的版本链。
- `confidence`、`subject_key/fact_key`、`scope`。
- `embedding`、`embedding_model_id`、`embedding_dimensions`、profile/generation。
- `source_conversation_id`、`source_message_id` 继续保留，并增加 extraction job/
  model/prompt version 与 evidence hash。
- 用户反馈与“被召回”分开存；`last_used_at` 不能当质量反馈。

不建议第一版把事实只存成图 triplet。用户可编辑的自然语言正文仍是可见主记录，
结构化字段用于检索、冲突和时间裁决。

## 5. 原生 v2 写入设计

### Durable extraction

- 在回答完成的持久化边界写 outbox，而不是只开 goroutine。
- worker 至少一次执行，job 通过 source message + extractor version 幂等。
- 输入使用“当前 user message + 有界的近期对话证据”；assistant 文本只作指代
  上下文，不得作为用户事实来源。
- extraction model 独立配置和版本固定，不再默认绑定本次回答模型。
- 结构化输出必须含 type、content、importance、confidence、fact key、时间字段和
  provenance；Go/Python 两侧均做长度、枚举、secret/credential 校验。

### 去重与矛盾

对每条候选先用 exact/hash，再对同 user/scope/fact key 做 hybrid related lookup：

- 同义重复 → `NOOP` 或合并 provenance。
- 新信息补充旧事实 → 新增版本并关联旧条目。
- 明确冲突/更正 → `SUPERSEDE`，旧事实保留历史但不进入“当前事实”召回。
- 有效期结束 → `expired`，默认检索隐藏；历史问题可显式搜索旧版本。
- 无法确定 → 宁可不自动写，交给用户审核，避免“记错比不记更糟”。

## 6. 原生 v2 读取设计

### Candidate retrieval

- Exact/key/phrase lane。
- `pg_textsearch` BM25 lane。
- `pgvector` BGE-M3 semantic lane。
- 在 SQL 查询中先执行 user/scope/status/time filter，不能先跨用户召回再过滤。
- 用 RRF 融合，候选 Top-20 左右送现有 BGE reranker；最后按 Top-K 和 token
  budget 选择，默认仍不超过 5 条。

### Degradation

- embedding 不可用 → BM25 + exact。
- reranker 不可用 → RRF Top-K。
- projection 未完成 → 当前 lexical v1 fallback。
- 任一 Memory 错误不得阻断正常回答；metadata 只给 bounded code/IDs，不泄露正文。

### Prompt trust

保留现有 `<relevant-user-memory>` 低优先级、不可信声明。Memory 中出现 command、
tool instruction、越权规则时不得执行；当前 user/system/project policy 永远优先。

## 7. 分期

### Phase 0 — 中文 benchmark 与 frozen baseline

- 建 200–500 个中文/中英混合 case：稳定事实、偏好、长期指令、项目、代词、
  更正、时间、冲突、无关请求、秘密、删除、跨用户隔离。
- 冻结当前 lexical v1 分数、延迟、token 和成本。
- 用完全相同 extraction/answer model 比 v1、Hindsight、原生目标；Mem0 只作
  可选成熟对照。一次只改一个变量。

### Phase 1 — 冻结 Engine contract + 原生 Recall v2（推荐下一步）

- 在 Go 定义 `Recall/Capture/Forget/Rebuild/Health` contract；user/scope 只能由 Go
  auth context 注入，客户端不得提交 authority ID。
- 新增 PostgreSQL outbox 与幂等 worker；先把现有自动抽取 goroutine 替换成 durable
  job，不改变 prompt 注入结果。
- L0 复用现有 messages；L1 增加 embedding/status/validity/provenance/generation，
  实现 exact + BM25 + pgvector + RRF + BGE rerank 的 shadow recall。
- L2/L3 暂只设计 schema/contract，不在 Recall@5 基线稳定前启动 LLM 聚合。

### Phase 2 — Hindsight 隔离 Shadow

- 新增测试 Compose profile，不接生产 prompt，不改当前用户设置。
- 使用独立 PostgreSQL database + role；限制连接池、CPU、RSS、WAL 和并发。
- 配置 SiliconFlow OpenAI-compatible LLM/embedding 和原生 SiliconFlow reranker。
- 回放脱敏 benchmark conversation；比较 extraction precision、Recall@5、冲突/
  时间、中文、延迟、token、成本、删除和重建。
- 验证 Hindsight 默认无 auth、共享 API key、bank 枚举/删除等接口在内网代理下
  不可越权。

### Phase 3A — Hindsight 在关键 slice 显著胜出时

- Go 写 durable outbox，内部 worker 调 `retain`；Go 回答前调 `recall`。
- 先 0% 注入只记差异，再 feature flag 小流量注入。
- 保留 canonical rows 与 v1 fallback；验证 tombstone、backup/restore、rebuild。

### Phase 3B — Hindsight 无显著收益时：保持纯原生

- 淘汰 sidecar，保留 adapter contract 和 benchmark fixture。
- 通过门槛后按用户 feature flag 把原生 v2 从 shadow 切到 prompt；lexical fallback
  永久保留至少一个版本。

### Phase 4 — L2/L3、Temporal、supersession、反馈与治理

- 参考 TencentDB 四层模型新增 L2 relevant scene 与 compact L3 persona；禁止每轮
  全量注入全部 scene/persona。
- fact key、validity、expiration、冲突链、provenance UI。
- “为什么记住/来自哪条消息/纠正/忘记”以及 per-chat “Use / Learn”控制。
- backup/restore、deletion propagation、重算 projection、模型 generation fence。

### Phase 5 — 其他关系图只做有证据的 PoC

只有当真实 query 中出现足够多的多跳关系/时间线，并且 flat temporal model 在
critical slice 持续不达标，才比较：

1. 原生 relational edges；
2. Graphiti sidecar（Neo4j/FalkorDB）；
3. 只有 Cognee 修复原子 adapter、durable job、cache isolation 后才重新评估。

Graph 候选必须在相同 corpus/model/budget 下对关键 slice 带来预注册的显著收益，
且通过备份、恢复、删除、权限、峰值资源门槛，才能晋升。

## 8. 建议发布门槛

以下是技术方案阶段应校准的初始门槛，不是已经通过的事实：

- Candidate Recall@20 ≥ 0.95；Final Recall@5 ≥ 0.90。
- 无关请求 Memory false-injection rate ≤ 2%。
- 更正/冲突 case 的 current-fact accuracy ≥ 0.95。
- secret/credential 持久化、跨用户泄露、删除后召回均为 0。
- 相对 v1 的关键中文 slice 不允许实质回退；rerank 需证明 nDCG/MRR 有收益。
- 记录 p50/p95 retrieval latency、每次抽取/召回 token、provider 成本、job retry/
  dead-letter，不用平均分掩盖尾延迟和失败。

厂商公开分数只能作为 prior；最终 promotion 以本项目 holdout 为准。

## 9. 风险与回滚

| 风险 | 控制 |
| --- | --- |
| Hindsight 默认无 auth / bank ID 越权 | 内网、API key、Go 生成 bank 映射、越权测试 |
| Hindsight 单条内部删除未做 bank validator | adapter 不暴露 UUID 删除；只走 Go scoped contract；补跨 bank 测试 |
| Hindsight audit/LLM/file/queue 未纳入完整 bank cascade | 删除门槛覆盖 traces、源文件、queue；未闭环不得晋升 |
| sidecar 抢连接/CPU/WAL | 独立 database/role、连接池上限、Compose resource limit |
| Hindsight migration 可能重建非 public pgvector | role 禁止连接 Neo Chat DB；只迁移独立 shadow DB |
| Hindsight `pg_textsearch` 对 CJK 不合 | BGE-M3/rerank 实测；不加新 extension 也过不了就淘汰 |
| self-host 与厂商 benchmark 模型不一致 | 相同 SiliconFlow 模型、本地 holdout、报告 token/成本 |
| 自动记错或把 assistant 幻觉当用户事实 | source attribution、confidence、审核、宁缺毋滥 |
| 旧/新事实同时注入 | status + validity + supersession，默认只召回 current |
| provider 失败导致写入丢失 | durable outbox、幂等、retry/dead-letter |
| 新模型换维度污染向量空间 | generation/profile fence，禁止仅凭维度复用 |
| Memory prompt injection | 保留低优先级 JSON 包装和 tool 禁令 |
| 复杂度膨胀 | 首期不加 Graph DB，不新增向量服务 |
| 发布效果不佳 | shadow → feature flag → lexical fallback；新 projection 可重建 |

数据库回滚不得删除用户原始 Memory。正确顺序是先关闭 v2 读写、恢复 v1 读取，
保留新增列/表；确认无需保留后才另行执行显式 data-loss migration。

## 10. 最终推荐顺序

1. **首选下一步：Go MemoryEngine contract + durable outbox + 原生 hybrid recall
   shadow。**
2. **并行对照：Hindsight 只在 frozen 中文 benchmark 上做隔离 shadow adapter。**
3. **数据模型：借 TencentDB 的 L0→L3，但落到现有 PostgreSQL/RAG，不接其
   Gateway/SQLite。**
4. **Hindsight 只有在关键 temporal/graph slice 显著胜出且过隔离/删除/恢复门槛时，
   才晋升为可重建 derived engine。**
5. **成熟行为对照：Mem0 OSS；Graph 需求后只先测 Graphiti，Cognee 待阻断项修复。**
6. **不选：Supermemory binary black-box；也不为 Memory 重写到 Hermes、Letta 或
   LangGraph。**
