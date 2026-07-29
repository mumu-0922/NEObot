# Neo Chat Server Memory v2 详细技术方案

状态：完整 end-state 设计已冻结并获授权实施；PR1–PR12 已完成，当前仍保留 v1 reader，
下一批进入 PR13 Hindsight fixture-only adapter/profile。日期：2026-07-29。

> 本文覆盖最终生产形态，不把交付范围收缩为 Phase 0/1。后文 Phase 只是为了降低
> 数据、隐私和回滚风险而安排的落地顺序，不代表 L2/L3、治理 UI、删除闭环、shadow
> benchmark 等最终能力被排除。凡源码和研究无法决定的产品偏好，均以 PRD 中的
> `grill-me` 决策树逐项冻结；当前 Decision Register 已闭合。只有必须由 frozen
> benchmark 校准的阈值/模型 profile 可在不改变产品语义的前提下调整。

## 1. 最终判定

采用 **Neo Chat 原生 PostgreSQL Memory v2**，不把任何第三方 Memory Server 变成
生产主库：

```text
Hermes Agent     -> 借 provider lifecycle / 失败隔离思想
TencentDB Memory -> 借 L0/L1/L2/L3 / provenance / progressive disclosure
Hindsight        -> 借完整 recall pipeline，并作为隔离 shadow benchmark
Neo Chat         -> 自己掌握 auth、canonical data、delete、jobs、prompt 与发布权
```

目标架构：

```text
Go API / auth / settings / CRUD / prompt authority
  ├─ NativeMemoryEngine
  ├─ PostgreSQL canonical Memory + evidence + revision
  ├─ PostgreSQL durable outbox/job + generation fence
  ├─ exact + CJK BM25 + BGE-M3 pgvector
  ├─ RRF + BGE reranker
  └─ lower-priority untrusted prompt block

Go Memory worker (same code/image, separate private process)
  ├─ PostgreSQL lease/poll + Redis wake
  ├─ extraction / conflict proposal / embedding
  ├─ L2/L3 refresh / purge / rebuild
  └─ dedicated DB role, pool, provider concurrency and CPU/RSS limits

existing infrastructure
  ├─ PostgreSQL 17 + pgvector 0.8.5 + pg_textsearch 1.3.1
  ├─ Go SiliconFlow provider gateway
  ├─ Redis wake-up only; PostgreSQL remains queue authority
  └─ Python RAG remains document parsing/indexing worker，不与 atomic Memory 混库

optional benchmark only
  └─ Hindsight private service -> dedicated PostgreSQL database + dedicated role
```

### 为什么不是“直接接一个最强项目”

Neo Chat 已经拥有正确且正在运行的 user authority、Memory CRUD、Server mode、
PostgreSQL、BGE-M3、reranker、prompt trust boundary 和删除即时不可见语义。直接接
第三方会重复用户系统、API、数据库、备份和删除权威。真正缺的是检索、冲突、
durability 和 provenance，不是另一套 Chat runtime。

## 2. 三层蓝图

### 2.1 业务层：用户看到什么

Memory 只解决四件事：

1. **Use**：回答前只取当前问题真正相关的记忆。
2. **Learn**：回答完成后，从用户明确表达中学习稳定事实、偏好、长期项目和决定。
3. **Correct**：用户更正后只使用当前事实，旧事实保留版本关系但默认不可召回。
4. **Forget**：用户删除后立即不可召回，并可靠清理所有 derived projection。

Server mode 下浏览器只展示和调用 Go API；IndexedDB/Local Memory 不参与 prompt、
学习或删除传播。

### 2.2 系统层：谁负责什么

| 组件 | 权限与职责 | 明确禁止 |
| --- | --- | --- |
| Go API | auth user、settings、CRUD、完成态事件、Recall、prompt 注入、delete | 接受客户端 `user_id` 作为 authority |
| 独立 Go Memory worker | claim durable job、调用 Memory task model/BGE、提交受校验结果 | 对外暴露端口、绕过 job lease/user epoch 或直接信任模型输出 |
| PostgreSQL | canonical rows、事务、evidence、版本、outbox/jobs、projection、visibility gate | 让 Redis/sidecar 成为唯一数据源 |
| Redis | 唤醒 worker、减少 polling latency | 保存唯一 job、Memory 正文或删除权威 |
| Python RAG | 继续处理文档解析、chunk、Knowledge projection | 为 atomic user Memory 再复制一套 pipeline |
| Hindsight shadow | 离线/隔离 benchmark 与差异报告 | 对浏览器开放、写主库、默认注入 prompt |

### 2.3 技术层：核心数据流

```text
completed assistant turn
  -> same DB transaction: finalize message + append ID-only memory event
  -> Redis best-effort notify
  -> Go worker claims leased job
  -> bounded source hydration from PostgreSQL
  -> extract candidates with explicit Memory task model
  -> validate + secret filter + exact dedup
  -> related-memory hybrid lookup
  -> ADD / NOOP / SUPERSEDE / MERGE / REJECT
  -> transactionally apply canonical/evidence/revision
  -> enqueue embedding projection
  -> projection becomes ready only under matching profile/generation
```

## 3. 不可破坏的架构不变量

1. `user_id` 只来自 Go auth context；API body、Hindsight bank ID、metadata 都不能改写。
2. `user_memories` 与 source messages 是 canonical；embedding、BM25、L2/L3、Hindsight
   都是可重建 projection。
3. 回答完成与 capture event 必须同一 PostgreSQL transaction；不再使用每请求
   goroutine 作为 durability。
4. Worker 至少一次执行；所有 apply 操作必须幂等并验证 lease、source hash、user
   visibility epoch 和 processing profile。
5. Assistant 文本只能帮助消歧，不能单独授权一条“用户事实”；候选必须引用至少一条
   user-role source message。
6. 自动提取不能静默覆盖 manual Memory；冲突进入 review 或由用户显式编辑。
7. 删除先切断 visibility，再异步清 projection；任何旧 job 均不能复活已删内容。
8. Memory 永远以低优先级不可信数据注入，不获得 system/tool authority。
9. Embedding 维度相同不代表向量空间相同；profile ID、provider、model、dimensions、
   prompt/version 必须完整匹配。
10. 任一 Memory 故障不得阻断聊天；降级顺序必须确定且可观测。
11. Global/Project/Conversation 与 Sensitive filter 必须在 SQL candidate retrieval 前生效；
    不能先取出越权内容再靠 Go 丢弃。
12. Project/Conversation/portable/Hindsight IDs 都不是 user authority；所有 mutation 和
    hydration 必须重新绑定 auth user、revision 与 generation。
13. Review、import、model output、L2/L3、Hindsight 都是 untrusted/derived；只有明确 user
    action 或通过已冻结 auto-apply policy 才能改变 canonical L1。
14. 发布 flag、Server promotion 与用户 Use/Learn/Sensitive/L2/L3 preference 分离；发布
    或 migration 不能覆盖用户明确关闭的设置。

## 4. Go 内部契约

### 4.1 分开 authority 与 engine

不把所有能力塞进一个 vendor 风格接口。推荐两个边界：

```go
type AuthoritativeMemoryStore interface {
    GetSettings(ctx context.Context) (Settings, error)
    ResolvePolicy(ctx context.Context, conversationID string) (EffectivePolicy, error)
    List(ctx context.Context, filter MemoryFilter) (MemoryPage, error)
    CreateManual(ctx context.Context, in ManualCreate) (Memory, error)
    UpdateManual(ctx context.Context, id string, expectedRevision int64, in ManualUpdate) (Memory, error)
    MoveScope(ctx context.Context, id string, expectedRevision int64, target Scope) (Memory, error)
    PreviewForget(ctx context.Context, cmd ForgetCommand) (ForgetImpact, error)
    Forget(ctx context.Context, cmd ForgetCommand) error
    DecideReview(ctx context.Context, cmd ReviewDecision) (ReviewResult, error)
    ApplyUserAction(ctx context.Context, cmd MemoryUserAction) (ActionResult, error)
}

type MemoryEngine interface {
    Recall(ctx context.Context, req RecallRequest) (RecallResult, error)
    ProcessCapture(ctx context.Context, job LeasedCaptureJob) error
    Rebuild(ctx context.Context, cmd RebuildCommand) error
    Health(ctx context.Context) MemoryHealth
}
```

`AuthoritativeMemoryStore` 与生产 `MemoryEngine` 都只有 native PostgreSQL 实现。
第三方对照必须实现下面更窄的 `MemoryShadowObserver`，不能通过同一 interface/config
被误配成 prompt reader。

### 4.2 请求必须显式携带的 fence

内部结构至少包含：

- auth-derived `UserID`；
- `VisibilityEpoch`；
- source conversation/message IDs 与 content hash；
- extraction profile/prompt version；
- embedding retrieval profile ID；
- job ID、lease token、attempt；
- scope、temporal mode/as-of、validity、token budget。

这些值不是为了炫技，而是解决删除并发、模型变更、重复执行和跨用户数据混合。

### 4.3 Shadow contract 单独收窄

```go
type MemoryShadowObserver interface {
    MirrorFixture(ctx context.Context, event FrozenMemoryEvent) error
    CompareRecall(ctx context.Context, req FrozenRecallCase) (ShadowDiff, error)
    PurgeBank(ctx context.Context, bankRef OpaqueBankRef) error
}
```

生产路径不允许把 `MemoryShadowObserver` 强转成 `MemoryEngine`，避免配置错误让
Hindsight 直接注入 prompt。

## 5. PostgreSQL 数据模型

### 5.0 First-class Project 与 scope authority

当前源码没有 Project/Workspace 实体，只有 user-global Memory 与来源
`source_conversation_id`。完整方案新增产品级 `projects`，而不是把任意字符串当作
Project scope：

```text
projects
  id UUID PK
  user_id FK users ON DELETE CASCADE
  name / description
  lifecycle_status = active | archived
  revision / scope_generation
  created_at / updated_at / archived_at / deleted_at
  UNIQUE(id, user_id)

conversations
  project_id UUID NULL
  memory_scope_generation BIGINT
  memory_use_mode = inherit | on | off
  memory_learn_mode = inherit | on | off
  composite FK (project_id, user_id) -> projects(id, user_id)
```

一条 Conversation 至多属于一个 Project，避免同一请求同时拥有多个等价 Project
scope；移动 Conversation 必须递增 context/scope generation，让旧 Recall/job 失效。
Go API 从 auth context 验证 Project ownership，浏览器不得用 `project_id` 改写 user
authority。

会话级 Memory policy 的 effective value 只能由 Go 计算：

```text
effective_use = user_settings.enabled
                && resolve(conversation.memory_use_mode,
                           user_settings.search_enabled)

effective_learn = user_settings.enabled
                  && resolve(conversation.memory_learn_mode,
                             user_settings.auto_record_enabled)
                  && project.lifecycle_status != archived
```

`inherit` 跟随全局值；显式 `on/off` 只覆盖对应维度。首次由用户开启 Memory 时产品默认
把 Use/Learn 都设为 on，但 migration 必须保留已有 setting，尤其不能把已明确关闭的
`auto_record_enabled` 静默打开。Manual save 是显式用户动作，不受 effective Learn
限制；Sensitive Memory 另受 `sensitive_memory_enabled` authority filter。

`user_memory_settings` 在现有三字段上增加：

```text
sensitive_memory_enabled BOOLEAN
l2_mode = inherit | on | off
l3_mode = inherit | on | off
```

Migration 对现有用户将 `sensitive_memory_enabled=false`，避免升级本身扩大远程数据发送；
首次 Memory 启用向导在用户明确确认后，按本方案默认打开 Use/Learn/Sensitive。L2/L3
的 `inherit` 表示“Server 已 promotion 时跟随默认开启”，`off` 永久尊重用户关闭；Server
promotion flag 与用户 preference 分离，不能用发布动作覆盖用户设置。

`user_memories` 增加：

```text
scope_type = global | project | conversation
project_id UUID NULL
scope_conversation_id UUID NULL
scope_generation BIGINT
CHECK (
  global       -> project_id IS NULL AND scope_conversation_id IS NULL
  project      -> project_id IS NOT NULL AND scope_conversation_id IS NULL
  conversation -> project_id IS NULL AND scope_conversation_id IS NOT NULL
)
composite FK (project_id, user_id) -> projects(id, user_id)
composite FK (scope_conversation_id, user_id) -> conversations(id, user_id)
```

Scope FK 使用 `ON DELETE RESTRICT`，强制 Project/Conversation delete 先执行已定义的
tombstone/purge procedure，不能让普通 FK cascade 绕过 impact preview。为 composite FK
补 `UNIQUE(id,user_id)`；Go 与 SQL function 都再次验证归属。

现有 `source_conversation_id` 继续只表示 provenance，不能兼任可见 scope；一条 Global
Memory 完全可以来源于某个 Conversation，但之后被所有会话使用。

### 5.1 保留并扩展 `user_memories`，不新建第二主库

现有字段和 API 继续兼容，新增字段如下：

| 字段 | 作用 | 原因 |
| --- | --- | --- |
| `lifecycle_status` | `active/superseded/expired/rejected` | 区分业务失效与 user delete |
| `revision` | 当前 logical row 版本 | 防并发 PATCH、绑定 job input |
| `subject_key` / `fact_key` | 可选规范化事实键 | 缩小冲突候选，不靠全库 LLM |
| `confidence` | 自动提取置信度 | 低置信不自动进入 prompt |
| `observed_at` | 证据观察时间 | 区分写入时间与事实发生时间 |
| `valid_from/valid_to` | 事实有效区间 | 支持当前事实与历史事实 |
| `expires_at` | 过期时间 | 临时项目/警告自动失效 |
| `superseded_by_memory_id` | 当前替代条目 | 明确冲突链 |
| `visibility_epoch` | 用户可见性代次 | 阻止 delete 后旧 job 复活 |
| `extraction_profile_id` | 抽取模型/prompt 绑定 | 复现、重建、对比 |
| `content_hash` | canonical 正文 hash | 幂等与 projection 校验 |
| `sensitivity` | `normal/sensitive` | Sensitive 总开关必须在 SQL reader 前过滤；secret 不得落表 |
| `authority_kind` | `manual/direct_user/confirmed/import/auto` | 区分用户授权与模型推断 |
| `source` | 扩为 `manual/ai/import` | 导入内容不伪装成本地 message provenance |
| `scope_type/project_id/scope_conversation_id` | Global/Project/Conversation 权威边界 | 防跨项目污染，不能只靠 tag |
| `scope_generation` | scope/Project membership 代次 | 阻止移动后旧 job 写回旧 scope |

`enabled` 继续表示用户手动开关；`deleted_at` 继续表示立即不可见；
`lifecycle_status` 只描述 active/superseded/expired 等 Memory 语义，三者不能混用。

现有 `(user_id, normalized_content)` active unique index 必须按 scope 拆成三个 partial
unique indexes，否则 Global 与 Project/Conversation 无法合法保存相同 override：

```text
global:       (user_id, normalized_content) WHERE scope_type='global' ...
project:      (user_id, project_id, normalized_content) WHERE scope_type='project' ...
conversation: (user_id, scope_conversation_id, normalized_content)
              WHERE scope_type='conversation' ...
```

同 scope exact duplicate 才是 NOOP；跨 scope duplicate 可以共存，并在 Recall 阶段按
Conversation > Project > Global 决定 winner。

### 5.2 `user_memory_state`

每用户一行：

```text
user_id PK/FK
visibility_epoch BIGINT >= 1
active_projection_generation BIGINT
active_retrieval_profile_id TEXT
active_l2_generation BIGINT
active_l3_generation BIGINT
created_at / updated_at
```

全量 forget 通过 state row 加锁并递增 `visibility_epoch`；account erase 直接级联删除
state。普通 rebuild 只分配新的 `projection_generation`，验证完成后再切 active pointer，
不改变可见性 epoch。单条 Memory delete 与 conversation delete 使用 targeted
tombstone/source existence fence，避免局部删除让其余 Memory projection 全部失效。
state 是并发 fence，不保存正文。

### 5.3 `user_memory_evidence`

一条 Memory 可由多条 user message 佐证：

```text
memory_id + source_message_id PK
user_id
source_conversation_id
evidence_role = user | assistant_context
source_content_hash
observed_at
created_at
```

规则：每条自动 Memory 至少有一条 `user` evidence；`assistant_context` 不能单独存在。
L0 继续使用现有 `messages`，本表只保存引用与 hash，不复制原始对话正文。

### 5.4 `user_memory_revisions`

append-only 保存 manual edit、自动 merge/supersede 的前后状态：

```text
memory_id + revision PK
user_id
operation
old/new content hash
prior_content_snapshot
old/new lifecycle status
actor_type = user | worker | operator
job_id / created_at
```

完整审计 UI 需要展示 content history，因此每次内容变更只保存一次 prior snapshot，
current content 仍只在 canonical row；不能同时在 revision 和另一个 history 表重复复制。
revision 继承 user/sensitivity/scope 的权限和删除契约，单条 Memory 删除时其正文 snapshot
也必须在 24 小时内物理擦除；擦除后只保留 hash、operation、时间和结果码。

### 5.5 `user_memory_tombstones`

单条 Forget 的防复活记录：

```text
id
user_id
memory_id
content_hash
fact_key
source_conversation_id / source_message_id
reason
created_at
```

Recall 永远不读取 tombstone；extraction apply 会拒绝命中 targeted tombstone 的旧
source/hash/fact。用户以后手工创建同一内容时，可由明确 manual action 覆盖 tombstone，
自动 worker 无权清除。

### 5.6 `user_memory_search_projections`

可重建检索投影：

```text
memory_id PK/FK
user_id
scope_type / project_id / scope_conversation_id
sensitivity
visibility_epoch
projection_generation
retrieval_profile_id
exact_terms TEXT[]
bm25_text TEXT
embedding_model_id
embedding_dimensions = 1024
embedding_vector VECTOR(1024)
content_hash
lexical_status = ready | failed
embedding_status = pending | ready | failed
created_at / updated_at
```

索引：

- `(user_id, scope_type, project_id, scope_conversation_id, sensitivity,
  visibility_epoch, lexical_status, memory_id)` authority/scope index；
- `GIN(exact_terms)`；
- `USING bm25(bm25_text) WITH (text_config='simple')`；
- `HNSW(embedding_vector vector_cosine_ops)`。

中文 BM25 不直接把原句交给 English tokenizer。沿用当前 Go CJK unigram/bigram 与
Latin normalization，构造 whitespace-separated lexical shadow，再交给真实 BM25；
这样不新增 jieba service/PGroonga，也保留项目已经验证的中文 token 行为。

Exact/BM25 projection 由 deterministic Go normalization 随 canonical transaction
生成；embedding 单独异步补齐。这样 BGE/provider 失败时不会连 lexical Memory 一起
变得不可用。

Projection 中复制的 scope/sensitivity 只用于索引前置收窄，reader 仍必须 join/验证
canonical revision、deleted/lifecycle、epoch 与 content hash；projection 不能自行授予
可见性。

### 5.7 Durable outbox 与 jobs

不复用 `knowledge_outbox` 表，避免把个人 Memory 的生命周期、删除和重放耦合到
Knowledge generation。复用它的成熟模式，新建窄表：

```text
memory_outbox
  event_id UUID UNIQUE
  user_id FK CASCADE
  event_type
  aggregate_id
  visibility_epoch
  payload JSONB             # 只含 IDs/revisions/profile，不放完整正文
  status/attempt/available_at/lease/last_error/timestamps

memory_jobs
  job_id UUID PK
  user_id FK CASCADE
  event_id FK
  stage = extract | resolve | embed | l2_refresh | l3_refresh | purge | rebuild
  idempotency_key UNIQUE
  source_hash/profile/generation/visibility_epoch
  status/attempt/max_attempts/available_at/lease/completed_at/error_code
```

Redis channel `mm-chat:memory:outbox:v1` 只发 `event_id` 唤醒信号。Worker 无论是否收到
信号都周期性扫描 PostgreSQL；Redis 断线不会丢任务。

完整形态使用与 backend 相同 code/image 的独立 `memory-worker` command/container；不在
Go API 内运行第二个消费 loop。Worker 不 publish port，使用独立 `memory_worker_runtime`
role、连接池、provider concurrency、CPU/RSS limit 和 healthcheck。数据库 role 只可经
受 lease/generation/source fence 约束的 function hydrate/apply，不获得任意跨用户 CRUD。
它复用 Server-owned provider vault/decryption 实现，但使用独立运行时凭据。

Worker 停机时 API 继续同步 Recall/聊天，outbox/jobs 保持 pending；恢复后按 lease 扫描
接续。部署切换必须有 singleton/lease 验证，禁止 API 内 loop 与独立 worker 同时运行，
避免拓扑配置漂移。Redis 仍只唤醒，不能成为 job authority。

### 5.8 `user_memory_review_suggestions`

低置信、冲突或 scope 不确定候选不能塞进 active canonical table：

```text
id UUID PK
user_id FK CASCADE
candidate_type / candidate_content / candidate_hash
sensitivity / confidence_band
proposed_scope_type / project_id / conversation_id
decision_reason_code
extraction_profile_id
status = pending | accepted | rejected | expired
expires_at / decided_at / created_at

user_memory_review_targets
  suggestion_id + memory_id PK
  expected_revision

user_memory_review_evidence
  suggestion_id + source_message_id PK
  evidence_role / source_content_hash
```

Targets 使用真实 FK/join row 而非无约束 UUID array；Evidence 只保存受权 message 引用，
不复制 conversation 正文。Pending suggestion 永不
进入 exact/BM25/vector/L2/L3 或 prompt；accept 必须重新验证 source、current revisions、
scope generation、secret filter 和 user epoch，再以 user actor transaction 写
canonical/revision。Reject/expire 后 candidate plaintext 进入 24 小时 purge，仅留 hash
和 result code。Pending 固定保留 30 天；到期自动 `expired`，不能 auto-accept。UI 显示
数量/剩余时间并支持批量 accept/reject/clear；Conflict pending 期间原 current Memory
继续生效。Clear、reject、expire 均复用同一 24 小时 plaintext purge contract。

### 5.9 `message_memory_usages`

为了回答旁展示“本轮用了哪些 Memory”，assistant finalize transaction 写最多实际注入
数量的窄关联：

```text
assistant_message_id + ordinal PK
user_id
entity_type = l1_memory | l2_scene | l3_persona
entity_id / entity_revision
layer = l1 | l2 | l3
scope_type
purpose = answer_context
created_at
```

不保存 query、Memory content、embedding、raw score 或完整 prompt。Memory 被删除后 Usage
UI 只显示“已删除 Memory”，不从 revision/audit 恢复正文；account/conversation 删除按
既定 cascade/purge contract 清理关联。

### 5.10 `message_memory_activities`

自动 Capture 完成可能晚于回答 SSE，使用无正文窄关联把结果回填到原 assistant message：

```text
assistant_message_id + ordinal PK
user_id
subject_type = memory | review_suggestion | job
subject_id / subject_revision
action = created | merged | review_required | rejected | failed
status / reason_code
created_at
```

Activity 不复制 Memory/candidate/source content；UI 展开时再经 auth repository hydrate 当前
可见内容。Conversation/account 删除时 cascade；Memory 已删只显示 deleted。客户端用
cursor-based、user-scoped activity endpoint 在页面可见且存在 pending job 时短轮询，任务
terminal 后停止，不新增 WebSocket。

“撤销本次自动写入”必须带 activity ID + target revision：若 created row 无后续 revision，
执行正常 Forget；若 merge 后 current revision 未变，追加 user undo revision 恢复 prior
snapshot；若 target 已 stale，则不强制覆盖，生成 Review suggestion。NOOP 不写 Activity，
避免聊天界面刷屏。

### 5.11 L2/L3 derived tables

L2/L3 必须有自己的 generation/watermark，不能把摘要覆写进 L1：

```text
user_memory_scenes
  id UUID PK / user_id FK CASCADE
  scope_type / project_id / topic_key
  content / content_hash / sensitivity
  lifecycle_status = shadow | active | disabled | stale
  profile_id / generation / source_watermark
  created_at / updated_at / deleted_at

user_memory_scene_members
  scene_id + memory_id PK
  memory_revision / memory_content_hash

user_memory_persona_versions
  id UUID PK / user_id FK CASCADE
  content / content_hash / token_count
  lifecycle_status = shadow | active | disabled | stale
  profile_id / generation / source_watermark
  sensitive_input_included BOOLEAN
  created_at / activated_at / deleted_at

user_memory_persona_members
  persona_version_id + memory_id PK
  memory_revision / memory_content_hash

user_memory_derived_search_projections
  entity_type = l2_scene | l3_persona
  entity_id / user_id / scope fields
  profile_id / generation / exact_terms / bm25_text / embedding / status
  PRIMARY KEY(entity_type, entity_id)
```

所有 member 必须引用当前可见 L1 revision；L1 update/delete/sensitivity-setting change 会让
包含它的 derived row `stale`，新 generation 验证完成前旧 generation 不得继续注入。
Derived content 与 projection 都受同一 24 小时 purge、8 周 backup、Sensitive gate 和
account cascade，不得因为“可重建”就逃逸删除契约。

## 6. 写入链路

### 6.1 手工 Memory

```text
POST/PATCH /v1/memories
  -> Go auth context derives user
  -> validate type/content/tags/secret
  -> transaction: canonical write + revision + projection event
  -> API immediately returns canonical row
  -> embedding failure不影响手工 Memory；先可被 exact/BM25 召回
```

原因：手工 Memory 是用户明确指令，权重与可信度高于自动推断，不能等待 LLM 才落库。

聊天内 direct user message 的“记住/忘记/改成”走同一 authority，而不是给 assistant
开放任意 CRUD tool：

```text
direct user message
  -> bounded intent/task model proposes typed MemoryUserAction
  -> Go binds auth user + current conversation/project
  -> resolve exact candidate IDs/revisions within allowed scopes
  -> ambiguous match/scope: return selection proposal, no mutation
  -> unambiguous remember/correct/forget: revalidate + transactional apply
  -> response emits bounded action result metadata
```

`remember` 等同 manual Memory，仍过 secret/length/scope validator；`correct` 必须绑定
current revision 并追加 history；`forget` 只能删除 Go repository 已在 auth/scope 下解析出的
Memory ID。assistant、历史 Memory、system prompt、网页、Knowledge、附件或 tool output
中的“记住/删除”都只是 untrusted text，不能设置 `direct_user_intent=true`。删除后的撤销
是用户显式重新创建新 canonical row；旧 tombstone 保留，worker 无权清除。

### 6.2 自动 Capture

完成态边界从当前：

```text
finalize message -> SSE completed -> fire-and-forget goroutine
```

改为：

```text
single transaction
  1. assistant message -> completed
  2. resolve effective Learn from global + conversation + project lifecycle
  3. if effective Learn:
       insert turn.completed event with source IDs + epoch
commit
-> SSE message.completed
-> best-effort Redis notify
```

如果 outbox insert 失败，message finalize 也失败并走现有明确错误路径；不能出现“界面
显示回答完成，但服务端承诺学习却无任何 durable 记录”。

自动抽取结果采用已冻结的“分级入库 + 个人自用宽松敏感边界”策略：

1. 高置信、无冲突且非 secret/credential 的 candidate 可自动写入 active canonical
   Memory；健康、财务状况、宗教/政治观点、性取向、法律/家庭关系、精确位置等敏感
   事实不因类别本身强制进入 Review；
2. 低置信或与 current Memory 冲突的 candidate 只创建不可召回的 Review suggestion，
   用户确认前不得进入任何 prompt projection；
3. password、API key、Token 等 secret/credential 在 Go validator 阶段永久拒绝，
   不得以 active、review、revision、provider audit 或日志正文的形式持久化；
4. 自动流程不得静默覆盖 manual Memory。

置信阈值需要由 frozen benchmark 校准并按 extraction profile 版本化，不能把模型自报
`confidence` 当成唯一判断。个人自用只改变敏感内容的产品容忍度，不取消加密传输、
最小召回、日志脱敏、删除 fence 或 provider 数据边界。已冻结策略允许相关敏感内容
发送给 Server 配置的 provider，但 Sensitive 总开关关闭时必须在 SQL candidate 阶段
排除，且永不进入 Hindsight/benchmark。

### 6.3 Extraction 输入

Worker 通过受限 repository/SQL function 获取：

- 当前 user message；
- 最多 8 条近期对话，source hydration 硬上限 4,096 estimated tokens；
- 已存在的相关 current Memory 最多 10 条；
- source IDs、时间与 scope。

进入远程 Memory task model 前先运行本地 deterministic secret detector/redactor；命中
password、API key、Token、cookie、private key、OTP/恢复码或支付认证字段的片段不再
发送给 Memory provider，并按 `secret_redacted` 计数但不记录原文。普通敏感事实允许
以完成 extraction 所需的最小上下文发送给 Server 配置的 provider。

角色规则：

- user message 是事实证据；
- assistant 只解释代词、上下文和用户已经明确确认的计划；assistant message 本身不能
  成为 authority evidence；
- assistant 提出的计划/承诺/决定只有在后续 user message 明确同意、复述/修改后确认、
  手动保存或明确开始执行时才允许抽取；沉默、未反对或仅要求生成方案不算确认；
- quoted document、网页、Knowledge、tool output 不成为用户事实；
- 每个候选必须返回至少一个 `authority_user_message_id`；若内容来自已确认的 assistant
  方案，还要把对应 assistant message 作为 `context_message_id`，但它不提供 authority。

抽取使用已经存在的 Server-owned `TaskModels.Memory`，而不是继续绑定本轮回答模型。
任务首次执行时冻结 provider/model/prompt version；job 只保存引用，不保存 API key。
未配置 Memory task model 时 manual Memory 仍可用，auto-record 显示明确
`model_unavailable`，不能静默换模型。

### 6.4 Candidate schema

模型最多返回 5 条：

```text
type
content
importance
confidence
tags
subject_key / fact_key
observed_at / valid_from / valid_to / expires_at
authority_user_message_ids
context_message_ids
confirmation_kind
proposed_scope_type
proposed_project_id / proposed_conversation_id
scope_confidence
```

Go 再执行枚举、长度、UTF-8、credential/secret、source ownership、时间范围和 JSON
size 校验。模型输出永远不是 authority。Candidate 的 `sensitivity` 同时由模型提议和
本地 rule classifier 计算，取更严格结果；任一路判定 secret 都直接 REJECT，不允许模型
用低风险标签降级。

Scope routing 采用已冻结语义：

- 稳定个人事实、通用偏好、跨项目指令候选 `global`；
- 当前 Conversation 已属于 Project，且候选描述该项目技术栈、约束、决定或进度时
  `project`；模型只能提议当前 Project ID，不能返回任意 ID；
- 临时状态、仅当前讨论有效、无 Project 会话中的项目内容或归属不明确时
  `conversation`；
- `scope_confidence` 未达到校准阈值时进入 Review，不用宽 scope 猜测；
- 管理页手工创建默认 Global；会话内手工创建默认 Conversation，若已属于 Project，UI
  可把 Project 作为推荐项明确展示，但最终请求必须带用户选择。

Scope routing 的确定性上下文（当前 Conversation/Project IDs）由 Go 注入和校验，LLM
只判断语义类别。用户移动 scope 时追加 revision、递增 `scope_generation` 并重建
projection；旧 job 因 generation 不匹配拒绝 apply。

### 6.5 去重与冲突裁决

每个候选按以下顺序处理：

1. proposed scope 内 normalized exact/hash 命中：`NOOP`，只合并新 evidence；跨 scope
   exact 不视为 duplicate。
2. fact key 命中：进入 related set。
3. exact + BM25 + vector 取最多 5 条 current related Memory。
4. 无 related 且通过置信/secret 规则：`ADD`；否则生成 Review 或 `REJECT`。
5. 有 related：让独立、版本固定的 decision prompt 返回：
   - `NOOP`：同义重复；
   - `ADD`：不同事实；
   - `SUPERSEDE`：明确更正/状态变化；
   - `MERGE`：兼容补充，生成新 current row并 supersede 被合并项；
   - `REJECT`：不稳定、secret/credential、证据不足。
6. 真正构成事实冲突时只生成 Review suggestion；兼容补充只有在目标不是 manual、
   candidate 高置信且非 secret 时才允许自动 `MERGE`。涉及 manual Memory 的任何替换或
   合并都必须由用户确认。
7. SQL transaction 重新验证 related row revision、epoch 与 source hash，再 apply。
   期间有变更则整条 candidate 重试，不能用 stale decision 覆盖新事实。

任何 current Memory 冲突都不自动 `SUPERSEDE`；用户确认后才执行版本切换。Manual
Memory 额外禁止 worker 自动 `MERGE`。这保证“用户明确设置”高于“模型推断”，也让
自动更正不会在模型误判时悄悄改写用户画像。

跨 scope 的同一 `fact_key` 默认是 override，不是互相 supersede：Conversation/Project
事实可以覆盖 Recall 中的 Global 事实，但不能改写 Global canonical row；只有同一
scope 内的更正才进入该 scope 的 revision/conflict chain。把 Memory 移动到另一 scope
是显式用户操作，不由冲突裁决暗中完成。

### 6.6 Temporal lifecycle

`observed_at/valid_from/valid_to/expires_at` 只有在用户明确时间表达或确定性规则能解析时
才自动应用；模型猜测的时间进入 Review。相对时间以 source message timestamp + 用户
timezone 解析，并记录 `temporal_basis`/parser version，不能在重放时按“今天”重新解释。

- 稳定事实不会因为长期未使用而自动衰减或删除；`last_used_at` 只做治理指标；
- 到达 `expires_at` 的 job 只把 lifecycle 切为 `expired` 并令 L2/L3 stale，不等同 user
  delete；用户仍可在历史 UI 查看；
- 明确更正建立 valid interval/supersede chain，当前 Recall 只取当前 winner；
- 只有 query 明确要求“以前/当时/某日期”时，Go 才设置 `temporal_mode=as_of` 并允许
  检索对应时点的 superseded/expired history；普通 query 不注入历史事实；
- `deleted_at/tombstone` 在 current 与 as-of 模式都绝对优先，删除内容不能借历史查询
  复活。

## 7. Recall 链路

### 7.1 权威过滤必须先发生

所有 lane 都先绑定：

```text
user_id = authenticated user
deleted_at IS NULL
enabled = true
lifecycle_status = active
valid_from <= now < valid_to（若存在）
expires_at > now（若存在）
visibility_epoch = active user epoch
sensitivity = normal OR sensitive_memory_enabled = true
lexical projection ready；vector lane 另要求 profile/generation = active binding
```

不能先跨用户/历史状态取候选再在 Go 过滤。

上表是默认 `temporal_mode=current`。`as_of` 只能由 direct query intent + 合法 timestamp
启用，改用 validity interval 选择当时 winner，并允许相应 superseded/expired row；仍然
强制 user/scope/sensitivity/deleted/tombstone/profile/generation 全部 fence。

Scope 也必须在 SQL candidate query 前过滤：只允许 `global`、当前
`conversation_id`，以及该 Conversation 当前所属的唯一 `project_id`。三层都返回同一
`fact_key` 时先按 `Conversation > Project > Global` 选当前事实，再进行 lane fusion；
不能让全局高相似度条目压过更具体的项目决定。

### 7.2 三路 candidate retrieval

推荐初始参数，之后只能由 benchmark 校准：

1. **Exact/key lane**：exact terms、fact key、tag，Top 20。
2. **BM25 lane**：Latin + CJK lexical shadow，Top 30。
3. **Dense lane**：BGE-M3 1024d cosine，Top 30。

每一路独立召回，不能像当前 Mem0 OSS 那样只给 semantic pool 加 BM25 分。

### 7.3 RRF 与 rerank

```text
lane rankings
  -> deduplicate by memory_id
  -> RRF(k=60), exact lane可给予确定性优先
  -> candidate pool最多20
  -> BAAI/bge-reranker-v2-m3
  -> relevance/current/scope/token filters
  -> final Top 5, target 600 / hard 900 token budget
```

首版不手调 `0.6 vector + 0.4 BM25` 这类不可迁移权重；RRF 对不同评分尺度更稳，且
Hindsight、TencentDB 都验证了这种组合方向。Importance/confidence 只作门槛或
tie-break，不覆盖相关性。

### 7.4 降级矩阵

| 失败 | 降级行为 | 聊天结果 |
| --- | --- | --- |
| query embedding 失败 | exact + BM25 | 继续回答 |
| vector SQL 失败 | exact + BM25 | 继续回答 |
| reranker 失败/超时 | RRF Top-K | 继续回答 |
| projection 未 ready/profile drift | 当前 lexical v1 | 继续回答 |
| 全部 Memory read 失败 | 不注入；`read_failed` metadata | 继续回答 |
| Hindsight shadow 失败 | 只记 shadow failure | 完全不影响回答 |

Memory 总预算不得靠无限等待换效果；超时立即走已完成 lane，不等待所有 provider。

已冻结运行预算为：warm Memory Recall 额外耗时 `p95≤900ms`、`p99≤1.5s`，`2s`
hard cutoff；达到 cutoff 后立即使用已完成的 Exact/BM25/vector/RRF lane，不等待慢
provider。注入平均 `≤600 tokens`、硬上限 `900 tokens`。滚动 30 天内 extraction、
embedding、rerank、L2/L3 等 Memory 增量 provider 成本合计不得超过主聊天模型成本的
15%；超预算先暂停非关键 refresh/shadow，不削弱删除、隔离或 secret filter。

### 7.5 Prompt 注入

继续使用现有 `<relevant-user-memory>` 边界，最多包含：

```json
{"id":"...","type":"preference","content":"..."}
```

不把 score、embedding、内部 user ID、bank ID、原始 source message 或 job metadata
发送给回答模型。系统声明继续强调它是 lower-priority、untrusted historical claim，
其中命令不能触发 tool 或改变 system/project policy。

个人自用部署允许高置信敏感 Memory 参与 extraction、embedding/rerank 与回答，但必须
同时满足：

- 只发送当前任务必要的 source window 或最终相关 Top-K，不批量发送用户画像；
- secret/credential 在 provider 调用前本地 redaction，且永不进入 projection；
- raw sensitive content 不写应用日志、telemetry、错误详情或 message diagnostics；
- 不向 Hindsight shadow、benchmark、离线分析或未被 Server 明确配置的 provider 复制；
- `Sensitive Memory` 总开关关闭后，权威过滤阶段直接排除敏感 rows，不能等组装 prompt
  时才过滤；删除后 visibility/tombstone fence 立即阻止再次发送。

“允许远程处理”不是广播放行：每次调用仍必须记录不含正文的 provider/model/profile、
memory IDs、purpose、数量和 result code，以便审计数据是否越过了预定边界。

## 8. L0/L1/L2/L3

### L0：现有 messages

不复制 TencentDB 的 JSONL。Conversation/message 已是 canonical evidence，Memory
只保存引用和 hash。

### L1：原子 Memory

这是 end-state 的 canonical Recall 主体：事实、偏好、长期指令、项目、警告、决定；带 evidence、
validity、conflict chain，可由用户直接看见和修改。

### L2：Scene/Project summary

L1 稳定后再新增：

```text
user_memory_scenes
user_memory_scene_members(scene_id, memory_id)
```

Scene 是 derived summary，必须列出成员 L1 IDs、generation 与 source watermark。
只检索相关 Scene，不像 TencentDB 当前实现那样每轮注入全部 navigation。

L2 初始只 shadow 生成，不注入 prompt；L1 正确性、删除传播和 frozen benchmark 分片
分别通过后才晋升为 active reader。晋升后对满足生成门槛的 Project/主题默认开启，但
每次只召回相关 Scene。UI 可查看内容、成员 L1/evidence、更新时间和 profile，可关闭/
重建。用户编辑 Scene 时创建/修正 L1 后重新聚合，不能直接把 derived summary 变成
canonical authority。

### L3：Compact persona

版本化、可重建、默认 200–300 token；每项声明必须能下钻到 L1/L0。只保存稳定、高
置信且用户未禁用的偏好/背景，不包含 secret、临时状态或“死命令”。L3 不能替代
用户可编辑的 L1。

L3 与 L2 使用独立 shadow/promotion flag；通过 persona consistency、false-injection、
token saving 和 delete propagation 门槛后默认开启。UI 显示 persona、来源 L1/evidence、
更新时间与 profile，并可独立关闭/重建；修改必须转成 L1 correction。关闭
`sensitive_memory_enabled` 时，敏感 L1 不得进入新的 L2/L3，active derived projection
必须切换到不含敏感输入的新 generation 后才可继续注入。

### 为什么 L2/L3 属于完整方案、但晚于 L1 晋升

L2/L3 是 end-state 的正式组成，不从完整方案删除；但它们需要额外 LLM 聚合，最容易
把错误放大并占用每轮 token。先证明 L1 hybrid、冲突和删除正确，再用真实 query
校准聚合和注入策略；这比同一版本一次性打开四层全部复杂度更稳。

## 9. Delete、Forget 与隐私擦除

### 9.1 单条 Memory delete

同一 transaction：

1. lock `user_memory_state`；
2. 验证 memory 属于 auth user；
3. `deleted_at=now()`，立即退出所有权威 reader；
4. 写入 targeted tombstone，绑定 memory/content/fact/source；
5. append purge event；
6. commit 后 API 返回成功。

Purge job 删除 search projection、evidence、derived L2/L3 membership，并按保留策略
物理擦除 canonical/revision 正文。审计只留 ID、hash、时间、result code，不留旧正文。

已冻结删除 SLA：

- transaction 提交后立即零 Recall、零 provider 发送；
- canonical/revision/evidence/search projection/L2/L3 中的正文必须在 24 小时内物理
  清除；超时进入 paging alert，purge job 继续幂等重试；
- 删除生成 signed/encrypted deletion manifest entry，只含 event/memory/user opaque ID、
  content hash、scope generation、deleted_at 和结果码，不含正文；
- manifest 复制到 off-host backup 控制面并至少保留 12 个月；任何 restore 必须在 backend
  断网/未启动状态先重放最新 manifest，再重建 projections，不能直接开放旧 dump；
- Postgres backup 保持不可变，不做容易破坏 dump 的原地删改；daily 14 天，weekly 与
  含 Memory 正文的 pre-deploy backup 绝对上限 8 周，monthly drill 长期只保留 checksum、
  release、restore result 等无正文报告；
- 现有 backup script 尚不自动 prune，实施必须增加带 dry-run、路径边界、checksum pair
  处理和测试的 retention command，不能只把“8 周”写进文档。

因此“在线删除完成”与“所有备份介质物理过期”是两个时间点，但任一受支持 restore 都
不得让已删除 Memory 重新可见；最迟 8 周后受支持备份中不再包含其正文。

### 9.2 Project archive 与 permanent delete

Project 有两个不同动作，不能用一个模糊的 DELETE 兼任：

**Archive**：

- `lifecycle_status=archived`，从活跃 Project selector 隐藏；
- Project、Conversation、Project Memory 和 evidence 全部保留；
- 已有 Conversation 仍可打开并 Use Project Memory，但自动 Learn 暂停；
- 禁止把新 Conversation 加入 archived Project；恢复为 active 后继续；
- Archive 不写 tombstone，也不改变 canonical content。

**Permanent Delete**：

1. API 先返回 Project Memory/Conversation 数量与删除计划，UI 二次确认；
2. transaction 内锁 Project，令其 `deleted_at` 生效并递增 scope generation；
3. 所有 Project-scoped Memory 立即 tombstone，退出 Recall/provider 发送链；
4. 默认把 Conversation 的 `project_id` 置空并保留 Conversation；
5. 不自动把 Project Memory 提升成 Global；如需保留，必须在删除前显式 move；
6. “同时删除 Conversation”使用独立显式参数/确认，默认 `false`，选择后复用完整
   Conversation delete contract；
7. purge jobs 清 projection/evidence/revision 正文；旧 extraction/embed/L2/L3 job 因
   project tombstone + scope generation 不匹配而拒绝 apply。

### 9.3 Conversation delete

- API 先返回 Conversation-scoped、AI Project/Global、manual-linked Memory 的分组数量，
  UI 明确展示影响；
- Conversation-scoped Memory 无条件随 Conversation tombstone/purge；
- 对以该 conversation/message 为 evidence 的 AI Project/Global Memory，先移除对应
  evidence；若同一事实还有其他 surviving user evidence，进入重新 extraction/rebuild，
  否则 tombstone/purge；
- manual Memory 默认保留，因为用户显式保存本身就是 authority；其已删除 source
  evidence/reference 清除并显示 `source_deleted`，但 canonical content 不变；
- UI 提供“同时删除从此会话手工保存的 Memory”独立复选项，默认 `false`，选中时把
  对应 manual rows 纳入同一 delete transaction/tombstone batch；
- 删除期间旧 extraction/embed/L2/L3 job 因 source existence/hash/epoch 不匹配而拒绝
  apply。

宁可重新构建，也不在删除源证据后保留无法证明来源的摘要。

### 9.4 Account erase

- user-scoped canonical/evidence/revision/outbox/job/projection 通过 FK CASCADE 或受审计
  purge function 清理；
- Redis 不含正文，只删除 wake/cache key；
- Hindsight trial 若处于 active，必须成功清对应 bank/audit/LLM/file/queue 并销毁 trial
  database/role/volume；在此 contract 未验证前不得镜像真实用户数据；
- backup 的物理过期时间必须在运营文档单独说明，不能把在线删除等同于备份瞬时消失。

### 9.5 防“删除后复活”

Job apply 同时检查：`user_id + visibility_epoch + targeted tombstone + source exists +
source hash + job lease + profile generation`。任一变化就拒绝。Redis 消息或旧 provider
响应本身无写权限。

## 10. Hindsight Shadow Benchmark

### 10.1 部署边界

- 独立 PostgreSQL database；
- 独立 least-privilege role；
- private Compose network + API key；
- pool/CPU/RSS/WAL 上限；
- 禁止 role 连接或 migrate Neo Chat database；
- 第一轮只回放 synthetic/de-identified fixtures，不发送 Live 用户数据。

不能只用同 database 不同 schema，因为 Hindsight migration 会在 pgvector 非 public
时尝试 `DROP EXTENSION vector CASCADE`。

真实数据默认永远关闭。Fixture 隔离、中文效果、delete、resource/cost、failure injection
全部通过后，UI 才能提供一次显式 opt-in 的 30 天 trial：

- 只镜像非敏感 active L1；Sensitive、secret、L2/L3、raw conversation 永不发送；
- trial 创建专属 database、role、API key 和受限网络/volume，不能复用下次 trial；
- Hindsight recall 只写无正文 divergence metrics/report，不可进入生产回答；
- 到期自动停止；stop、Memory delete 或 opt-out 后 24 小时内 purge 对应 bank，并在 trial
  结束时终止连接、销毁 trial database/role/volume；
- adapter 必须验证 bank、audit、LLM cache、file、queue 和日志无正文残留；任一层无法
  证明清除，真实 trial feature flag 永久保持不可用；
- 继续或再次试验需要新的用户确认，不能把一次 opt-in 变成持续镜像授权。

### 10.2 两条公平测试线

1. **End-to-end**：native 与 Hindsight 使用相同底层 LLM/embedding/reranker，但各自
   运行自己的 extraction + recall，比较最终效果和成本。
2. **Retrieval-only**：向两边灌入语义等价的已冻结事实，隔离 extraction 差异，只比
   exact/BM25/vector/temporal/graph/rerank。

只跑第一条无法判断胜负来自 extraction 还是 retrieval。

### 10.3 晋升门槛

Hindsight 只有同时满足以下条件才进入真实用户 opt-in 讨论：

- 关键 temporal/multi-hop slice 相对 native 有预注册的显著收益；
- aggregate 与中文 slice 不回退；
- cross-user、secret persistence、delete-after-recall 均为 0；
- account delete 覆盖 audit/LLM/file/queue；
- p95、token、provider cost、CPU/RSS/WAL 在预算内；
- 可从 Neo Chat canonical events 全量 rebuild；
- Hindsight 故障时 native 无行为变化。

即使通过这些条件，也只获得上述 30 天 non-sensitive shadow 资格，不获得 prompt reader
或长期镜像资格。Trial 结果若显示显著收益，只能触发新的设计评审，不能自动晋升。

未来若另一次设计评审允许它承担 derived recall，也绝不能改变 Go/PostgreSQL canonical
authority、delete fence 或 native fallback；本方案本身不授予该权限。

## 11. API 与前端兼容

### End-state 仍遵守的兼容原则

- `/v1/memory-settings` 三个现有字段保留；
- `/v1/memory-settings` 增加 `sensitiveMemoryEnabled`，全局首次启用向导明确展示 Use、
  Learn、Sensitive 三项，不靠隐藏默认值改变既有设置；
- `/v1/memories` CRUD shape 保持兼容；
- 新字段先作为 optional response；
- 新增 `/v1/projects` CRUD 和 Conversation membership API；Project ownership 只能来自
  auth user，删除/移动操作必须携带 revision；
- `/v1/memories` 增加可选 scope fields；旧客户端从管理页创建 Memory 时默认 Global，
  response 明确返回解析后的 scope；
- 新增 Conversation Memory policy PATCH，分别接受 `useMode`/`learnMode` 的
  `inherit|on|off`，response 返回 mode 与 effective value；
- Chat request 不接受客户端自报的 executable Memory ID；Server 从已持久化 direct user
  message 解析 `MemoryUserAction`，歧义时返回只含受权候选的 selection token，确认请求
  必须携带短期签名 token + revision，防 stale/confused-deputy mutation；
- SSE/message metadata 可返回 bounded `memoryActionResults`（action、status、Memory IDs、
  scope、reviewRequired），不返回 source raw text、prompt 或 provider payload；
- Server mode 继续隐藏 Local Memory/Dream；
- v2 初期只在 message metadata 增加 bounded diagnostics：lane、fallback、count、IDs，
  不持久化 query 或 Memory 正文副本。

End-state 路由按资源拆分，所有 mutation 使用 idempotency key + expected revision，所有
list/cursor 都由 Server 绑定 auth user：

| 路由族 | 关键行为 |
| --- | --- |
| `GET/PATCH /v1/memory-settings` | 全局 Use/Learn/Sensitive/L2/L3 preference 与 effective 状态 |
| `/v1/projects` + Conversation membership | CRUD、archive/restore、delete preview/confirm、单 Project membership |
| `/v1/memories` | list/create/update/move/forget；scope、lifecycle、sensitivity、revision |
| `/v1/memories/{id}/evidence`、`/history`、`/usages` | provenance、revision chain、回答使用记录；删除正文不回显 |
| `/v1/memory-reviews` | pending list、accept/reject/edit-merge/keep-both/clear；重新验证 targets |
| `/v1/conversations/{id}/memory-policy` | Use/Learn `inherit/on/off` 与 effective value |
| `/v1/memory-activities` | cursor polling 与 revision-safe undo；terminal 后停止轮询 |
| `/v1/memory-derived` | L2/L3 list、source drill-down、disable/rebuild；不能直接编辑 derived text |
| `/v1/memory-export`、`/v1/memory-import:dry-run/confirm` | 加密流、plan hash、短期确认 token、无 plaintext temp |

Delete Project/Conversation/Memory 必须先提供 impact preview token；confirm token 绑定 auth
user、resource ID/revision、影响集合 hash、选项和短 TTL，资源漂移就失效。浏览器提供的
`user_id`、portable ID、Hindsight bank ID、Memory match ID 一律不能绕过 repository
reauthorization。

### Versioned encrypted Export/Import

提供独立、受 auth/CSRF/rate-limit 保护的 Export/Import，不把数据库 dump 当成用户可携带
格式。Export 使用流式 `.mm-memory` envelope：外层固定为 `age` v1 passphrase/scrypt
authenticated stream（Go library 必须固定版本与 source/license audit，不自造 crypto），内层
包含 manifest + JSONL。Passphrase 只在请求内存中使用，不写 env、job、log 或临时文件；
plaintext archive 不落 server disk。

Manifest 至少绑定：format/schema version、created_at、record counts、content SHA-256、
history included flag 和 exporter release。Portable IDs 只在包内引用，不能携带/覆盖源
实例 `user_id` authority。内容包括：

- Project metadata 与 portable project refs；
- L1 canonical content/type/scope/lifecycle/validity/sensitivity/tags；
- Memory settings 仅作可选导入建议；
- 用户勾选时包含 revision history；
- 不包含 raw conversation/message、provider request、embedding、BM25/exact projection、
  L2/L3、outbox/jobs、audit/log、API key/Token 或部署 secret。

Import 使用 streaming parser 和硬 size/count/content-length limits，顺序为：decrypt → envelope
hash/schema → local secret detector → field/type/scope validation → dry-run resolution。Secret/
credential 必须在任何 staging/persistent table 之前拒绝，报告只含 portable row ordinal、
reason code/hash，不回显 secret。Dry-run 固定输出 `NOOP/ADD/REVIEW/REJECT/SCOPE_REQUIRED`：

初始 hard caps：解密后 256 MiB、50,000 L1 rows、200,000 revision rows、1,000 Projects，
单条 content 继续沿用 2,000 Unicode code points；operator 只能下调，放宽必须重新做
memory/CPU/DoS benchmark。解析使用流式计数与 bounded buffers，不把整个 archive 放进
内存。

- exact duplicate → `NOOP`；
- new valid row → `ADD`；
- current fact 冲突 → `REVIEW`，不覆盖；
- secret/invalid → `REJECT`；
- 外部 Project 只能映射到当前 auth user 的现有 Project 或显式新建；
- Conversation portable ref 无本地映射时必须改成 Project/Global 或跳过，不能伪造会话；
- imported settings 默认不应用，尤其不得自动打开 Use/Learn/Sensitive。

确认请求必须绑定 dry-run plan hash、短期签名 token、当前 revisions/scope generations；任一
漂移就重新 dry-run。写入 source=`import`，provenance 明示“外部导入、无本地 message
evidence”，但用户确认本身作为 authority。Derived projections/L2/L3 全部在本地重建。

### 完整治理 UI

L1 稳定后按 promotion 开放：

- “来自哪条消息”；
- current / superseded / expired 状态；
- 更正、确认自动冲突、忘记；
- 每会话独立 `Use Memory` / `Learn Memory`；
- Project 列表、Conversation 所属 Project、三层 scope badge，以及 Memory 的
  promote/demote/move 操作；
- Conversation header 提供独立 Use/Learn 控件并显示“继承全局/本会话覆盖”；Archive
  Project 的 Learn 显示为 policy 强制暂停，而不是假装用户开关已被修改；
- background job 失败的可理解状态，不展示内部 stack/provider secret。
- L2 Scene/L3 Persona 内容、来源、profile/generation、shadow/active 状态，以及独立
  disable/rebuild；用户 correction 始终落到 L1 再触发重建。
- Memory 卡片展示 type、scope、Manual/Auto/Confirmed、lifecycle、更新时间、敏感标记
  与 Recall 状态；
- provenance 下钻 surviving evidence，来源删除后只显示 `source_deleted`；
- Conflict Review 并排显示 current/candidate/scope/evidence，动作固定为 keep current、
  accept new、edit merge、keep both、reject；每个动作都是显式 user actor transaction；
- revision/supersede timeline 从 canonical + revisions 读取；回答 usage 从窄关联表读取；
- 删除前 impact preview，删除后展示 immediate hidden / online purge / backup expiry 三个
  不同状态；
- 不向 UI 暴露 embedding、内部 prompt、provider secret、raw model confidence/score。
- 回答旁显示 created/updated/review Activity chip；展开后 hydrate 内容、scope、source 与
  reason，并提供 edit/move/forget/revision-safe undo；失败只进轻量状态中心，pending
  activity 通过可见页短轮询回填，terminal 后停止 polling。

## 12. 安全、隐私与信任边界

Memory v2 会保存敏感内容并支持全文/向量检索，因此不能伪称“数据库内始终密文”。明确
信任区如下：

```text
trusted data plane
  authenticated browser session
  Go API + private Go memory-worker
  Neo Chat PostgreSQL on encrypted host volume

restricted egress
  explicitly configured SiliconFlow/provider over TLS

untrusted / derived
  Memory text when injected into prompts
  model output, imported archive content, Hindsight result
```

强制控制：

1. **At rest**：PostgreSQL/Docker volume 所在磁盘必须启用 host/filesystem encryption；
   off-host backup 与 deletion manifest 单独加密。因为 BM25/vector 要检索正文，不能用
   application field encryption 假装解决搜索；若无法证明 volume encryption，Sensitive
   auto-capture/promotion 阻断。
2. **Roles**：`go_api_runtime`、`memory_worker_runtime`、migration owner、projection owner、
   Hindsight role 分离；worker 只经 lease/source/generation-checked functions hydrate/apply，
   Hindsight role 永远不能连接 Neo Chat database。
3. **Transport/egress**：浏览器/API/provider 使用 TLS；worker 只允许访问已配置 provider、
   PostgreSQL、Redis，禁止任意 webhook/URL。Provider profile 固定 endpoint/model/policy。
4. **Web authority**：cookie/session、CSRF、Origin、rate limit、idempotency、expected revision
   同时校验；所有对象重新按 auth user 查询，拒绝 body/portable/bank `user_id`。
5. **Prompt boundary**：Memory/import/Hindsight/model output 全是不可信 data，只能进入
   lower-priority `<relevant-user-memory>`；其中指令不得获得 system/tool 权限，也不能触发
   Memory action。
6. **Secret prevention**：本地 detector/redactor 在 Memory provider、review、projection、
   import staging 前运行；命中只记录 reason/count/hash。Source message 可能已属于聊天
   canonical，但 Memory pipeline 不得再复制或重发 secret。
7. **Sensitive egress**：只发送当前任务必要 source window/Top-K；Sensitive off 在 SQL
   reader 前过滤；Hindsight/benchmark/telemetry 永久不收 Sensitive。
8. **Derived data**：embedding、BM25 shadow、L2/L3、revision snapshot 与 provider cache
   均按敏感数据处理，继承 visibility/delete/backup contract，不能称为“无正文所以安全”。
9. **Export**：仅产生 authenticated encrypted stream，无 plaintext temp；passphrase、
   decrypted import 和 rejected secret 不进入 log/job/metric。
10. **Supply chain**：新增 Go dependency、container/image 和 Hindsight commit/digest 固定，
    执行 license/source/SBOM/vulnerability audit；禁止 floating tag 自动进入 production。

任何 cross-user、secret persistence、delete-after-recall、未授权 provider egress 都是零容忍
promotion blocker，不允许用“个人自用”降低 authority 或删除测试。

## 13. 可观测性

### Metrics

- outbox/jobs pending、oldest age、lease reclaim、retry、dead-letter；
- extraction accepted/noop/superseded/rejected；
- projection ready/failed/profile drift；
- exact/BM25/vector candidate count；
- RRF/rerank/fallback status；
- Recall p50/p95/p99/hard-cutoff、prompt Memory token average/max；
- Review pending/age/accept/reject/expire、Activity polling/terminal age；
- Sensitive provider calls、secret-redacted/rejected counts（无正文 label）；
- delete propagation/purge age、backup manifest replication/restore replay；
- provider token/cost 与 30 天 Memory/chat cost ratio；
- shadow divergence/resource usage、L2/L3 stale/active generation。

### Logs/metadata

只记 IDs、hash、model/profile ID、耗时、计数、错误码；不记 raw conversation、Memory
正文、provider request/response、embedding 或 secret。失败原因需可重放，但不能复制
Mem0/Hindsight 的正文 history/audit 残留问题。

## 14. Frozen Benchmark

发布前冻结 500 个中文/中英混合 case：

- 稳定事实、偏好、长期指令、项目与决定；
- 中文同义改写、简称、实体名、代词、省略；
- 更正、过期、相对时间、current-vs-history；
- 无关 query/negative，防止 false injection；
- quoted document、assistant hallucination、secret/credential；
- user A/B 隔离、Memory/conversation/account delete；
- worker crash、Redis down、provider timeout、profile drift、rebuild；
- 少量多跳关系 slice，用于判断是否真的需要 Graph。

数据分为 Development/Validation/Holdout；Holdout 首次执行前冻结 hash，结果不得覆盖。

初始技术门槛：

- Candidate Recall@20 ≥ 0.95；Final Recall@5 ≥ 0.90；
- current-fact accuracy ≥ 0.95；
- false-injection rate ≤ 2%；
- cross-user、secret persistence、删除后召回 = 0；
- critical Chinese slice 不回退；
- warm Memory recall p95 ≤ 900ms；
- warm Memory recall p99 ≤ 1.5s，2s hard cutoff；
- 平均注入 ≤ 600 tokens，单次硬上限 900 tokens；
- 滚动 30 天 Memory 增量 provider 成本 ≤ 主聊天模型成本 15%；
- reranker 必须证明 nDCG/MRR 收益，否则关闭以节省延迟与成本。

## 15. 实施分期与回滚

### Phase 0：只建 benchmark，不改 Live 行为

- 冻结 v1 lexical baseline；
- 建 fixture、评分器、privacy/failure cases；
- 输出可复跑报告。

**原因**：没有 baseline 就无法知道“复杂了”是否等于“变好了”。

### Phase 1：Contract + durable outbox

- 建 Go authority/engine 边界；
- assistant finalize 与 ID-only outbox 同事务；
- 独立 leased Go `memory-worker` + PG polling + Redis wake；
- 先运行现有 extraction prompt 语义，Recall 仍用 v1。

**回滚**：关闭 worker/auto-record；v1 CRUD/Recall 不变；pending event 保留可重放。

### Phase 2：L1 provenance + conflict

- 扩 canonical 字段、evidence、revision、epoch；
- 显式 Memory task model；
- exact dedup + related lookup + ADD/NOOP/SUPERSEDE/MERGE/REJECT；
- delete/rebuild fence。

**回滚**：关闭 auto apply，只允许 manual；新增列/表保留，不做 data-loss down。

### Phase 3：Native hybrid shadow

- 建 search projection；
- exact/BM25/BGE-M3/RRF/rerank；
- 0% prompt 注入，仅记录与 v1 的差异；
- 跑 frozen benchmark；
- 500-case gate 通过后，真实 L1 shadow 仍需至少 7 天且 100 个 eligible turns。

**回滚**：active reader pointer 保持 v1；projection 可清空重建。

### Phase 4：Native v2 小流量晋升

- 单用户产品不做虚假的 1% 用户灰度；先手工选择一个 Project/Conversation canary；
- canary 至少 7 天且 50 个有效 Recall，通过后全局 L1 并观察 14 天；
- 每步观察 quality、latency、false injection、delete、backlog 和 cost；
- lexical v1 至少保留一个版本作 fallback。

**回滚**：切回 v1 reader，不删除 canonical 或 v2 projection。

### Phase 5：Hindsight isolated shadow

- 单独 Compose profile/database/role；
- 先 synthetic fixture；
- end-to-end 与 retrieval-only 两轨比较；
- 不通过即删除 shadow DB，保留 adapter contract 和报告。

### Phase 6：L2/L3

L1 全局观察通过后，L2 与 L3 分别独立走 shadow → selected canary → active；任一层失败
只回滚该层，不能捆绑 promotion。过门槛后按已冻结产品决策默认开启且可见可控。

### Phase 7：Graph 专项

只有多跳关系 slice 持续不达标才比较原生 relational edges 与 Graphiti；Cognee 必须
先修复原子 adapter、durable job 和 cache isolation 才重评。

### 自动熔断与回滚矩阵

| 触发器 | 自动动作 | 不得执行 |
| --- | --- | --- |
| cross-user、secret persistence、delete-after-recall 任一 `>0` | 立即关闭 v2 Use/Learn 和 shadow，对外只走安全 fallback，page operator | 等观察窗口、继续 provider 发送 |
| 最近有效窗口 Recall `p95>900ms` 或 `p99>1.5s` 持续越界 | 切回上一 reader；保留 Exact/BM25 fallback | 删除 canonical/projection |
| reviewed false injection `>2%` 或 current-fact `<0.95` | 停止当前层 promotion，切回上一 reader | 自动改写错误 Memory 掩盖问题 |
| worker error/dead-letter/backlog 越过冻结运行阈值 | 暂停 Learn/非关键 refresh，保留 Use reader 和 durable jobs | 丢弃 pending jobs |
| 30 天成本预测 `>15%` | 先停 Hindsight、shadow、非关键 L2/L3 refresh；仍超限则关闭 rerank | 削弱 delete/secret/isolation |
| profile/generation/source drift | 对受影响 projection fail closed，回 lexical v1 并 rebuild | 用 stale embedding/summary |

Feature flags 与 active reader/projection generation 均由 Server authority 控制。回滚只
切 flag/pointer，保留 canonical、outbox、jobs、migration 和可重建 projection；不执行
data-loss down migration。v1 lexical Reader 至少保留一个完整发布版本，且 rollback drill
必须在每次 promotion 前通过。

运行时 queue circuit 初值也必须冻结：promotion 期间任何 dead-letter 都阻断晋升；active
期间 normal job failure rate 在至少 20 jobs/15min 窗口内 `>5%`，或 pending oldest
`>10min`/count `>100` 时暂停 Learn 和非关键 refresh；purge oldest `>1h` 立即告警，
`>24h` 触发 privacy incident。低流量不足 20 jobs 时使用连续 5 次同类失败触发，不因
样本少而无限等待。

## 16. Migration、backfill 与部署兼容

所有 schema change 使用当前 migration head 之后的 additive migrations，先扩展再切 reader，
不得在同一发布删除 v1 columns/tables：

1. 新建 Project、state/evidence/revision/tombstone/review/outbox/job/activity/usage/L2/L3/
   projection tables 与 least-privilege roles/functions；现有 API 行为不变。
2. 扩 `user_memory_settings`、`conversations`、`user_memories`；CHECK/FK/index 先在 shadow
   数据校验通过后 validate，避免长锁直接压 Live。
3. Backfill 现有 Memory：manual → Global/active/manual authority；AI row 只有 surviving
   user message evidence 才保持 active，否则 disabled + Review；不得凭 assistant/source
   null 猜事实。现有 Live 0 rows 不能代替通用 migration test。
4. 现有 setting 原值原样保留；新增 Sensitive 默认 false、L2/L3 mode 默认 inherit；
   只有用户完成启用向导才按已冻结默认打开 Use/Learn/Sensitive。
5. 创建 generation 1 lexical projection 并与 v1 结果对比；BGE projection 异步构建，
   完成前不能切 active generation。
6. Compose 增加同 image 的 private `memory-worker` service、独立 role/secret/resource/
   healthcheck；确保 API 内旧 goroutine/loop 已禁用后才启动 consumer。
7. Backup/restore 脚本加入 deletion manifest replay 与 14-day/8-week retention dry-run/prune；
   必须在 disposable restore 中证明旧 backup 不复活已删 Memory。
8. Down migration 只允许撤销尚未启用且无新数据的 additive object；一旦 production 写入
   v2 data，rollback 只切 flag/pointer，不 drop data。

Compatibility matrix 必须覆盖旧 frontend→新 backend、新 frontend→v1-compatible routes、
worker version N/N-1 event schema 和 rolling restart。Outbox payload、export envelope、API
response 都带 version，消费者拒绝未知 major 而不是猜测字段。

## 17. 推荐的实现批次

1. **PR1：benchmark skeleton + contract tests** — 已于 2026-07-28 实现并通过 backend
   focused race/full test/vet；新增内容保持离线，无 runtime 行为变化。
2. **PR2：Project/scope/settings additive schema + backfill** — 已于 2026-07-28 以 migration
   `053` 实现；flags 全关，旧 API 保持 Global-only，PG17 已验证 backfill、ownership/
   scope constraints、runtime role denial、guarded down 与 re-up。
3. **PR3：outbox/jobs + 独立 Go worker + lease/replay** — 已于 2026-07-28 以 migration
   `054` 实现；保持 v1 reader，PG17、Compose、Docker build 与 full standalone 已通过。
4. **PR4：evidence/revision/epoch/tombstone/delete manifest** — 已于 2026-07-28 以 migration
   `055` 实现数据库内 provenance/delete authority、旧响应防复活与 provider-free online
   purge；authenticated encrypted off-host manifest/restore replay/retention 仍按顺序归 PR10。
5. **PR5：capture candidate + Review + temporal/conflict/scope routing** — 已于 2026-07-28
   以 migration `056` 实现 strict candidate-wide proposal、secret/Sensitive egress guard、
   exact/manual/temporal routing、canonical auto-apply revocation 与 provider-free 30-day
   plaintext expiry；保持 v1 reader/API，PG17、Compose、Docker build 与 full standalone 已通过。
6. **PR6：direct-user Memory actions + activity/usage links** — 已于 2026-07-28 以 migration
   `057` 实现 current-user-only strict typed action planner、direct `remember|correct|forget`、
   immutable Usage、link-only Activity polling 与 revision-safe undo；保持 v1 reader/API，
   PG17、Compose、Docker build 与 full standalone 已通过。
7. **PR7：L1 exact/CJK BM25 projection** — 已于 2026-07-28 以 migration `058` 实现
   transactional canonical projection、独立 exact/CJK BM25 lanes 与 normalized ID-only shadow
   observations；`MEMORY_LEXICAL_SHADOW_ENABLED=false` 默认关闭，v1 Top 5/prompt/Usage 保持唯一
   authority，PG17、Compose、Docker build 与 full standalone 已通过。
8. **PR8：BGE-M3/vector + RRF + rerank + budget fallback** — 已于 2026-07-28 以 migration
   `059` 实现 BGE-M3 1024d projection、lease-fenced embedding jobs、Exact/BM25/Vector 独立
   lanes、`RRF(k=60)`、BGE rerank 与 600/900 token budget fallback；hybrid flag 默认关闭，
   v1 Top 5/prompt/Usage 保持唯一 authority，PG17、Compose、Docker build 与 full standalone 已通过。
9. **PR9：Project/Conversation policy + governance UI** — 已于 2026-07-28 以 migration
   `060` 实现 Project archive/restore、Conversation Use/Learn policy、scoped governance、
   Review decisions、current-only provenance/history/Usage/Activity hydration 与 delete progress；
   v1 Global Top 5 继续是唯一 prompt/Usage reader。
10. **PR10：encrypted Export/Import + retention/prune/restore replay** — 已于 2026-07-28
    以 migration `061` 实现 authenticated age portability、ADD-only dry-run/confirm fencing、
    encrypted off-host deletion replay、projection rebuild 与 verified backup-set retention；
    v1 Global Top 5 继续是唯一 prompt/Usage reader。
11. **PR11：L2 Scene shadow/promotion** — 已于 2026-07-28 以 migration `062` 实现
    same-scope derived Scene、leased refresh/embedding/purge、Exact/BM25/vector RRF/rerank、
    500-token active reader、Server governance 与 evidence-gated promotion/rollback；两个 runtime
    flag 默认关闭，数据库 profile 保持 shadow，v1 L1 继续是唯一默认 prompt/Usage authority。
12. **PR12：L3 Persona shadow/promotion** — 已于 2026-07-29 以 migration `063`
    实现 Global stable-L1 Persona、leased refresh/embedding/purge、独立 Exact/BM25/vector
    RRF/rerank、300-token reader、Server governance 与 evidence-gated promotion/rollback；
    两个 runtime flag 默认关闭，数据库 profile 保持 shadow，v1 L1 继续是唯一默认
    prompt/Usage authority。
13. **PR13：Hindsight fixture-only adapter/profile** — 已于 2026-07-29 实现 digest-pinned
    `net/http` adapter、hash-bound synthetic manifest、`end_to_end`/`retrieval_only` profiles、
    isolated Compose、content-free report 与强制 teardown。两轨 draft 均为 8/10，temporal 与
    negative case 被 `forbidden_memory_result` 阻断；结果不可 promotion，真实 trial 仍需另行
    显式授权。对照后的 Hindsight container/network/volume/database/role/key/bank runtime state
    已全部销毁。

每批必须有 migration replay/down 安全性、Go focused tests、PostgreSQL integration、
worker crash/reclaim、cross-user/delete tests；跨层完成后再跑 full standalone gate。

## 18. 选择原因总表

| 决策 | 选择 | 原因 | 未选方案的问题 |
| --- | --- | --- | --- |
| Authority | Go + current PostgreSQL | 已有正确 auth/CRUD/delete | vendor server 会形成双主 |
| Runtime | 增量改造现有 Go | 最小变更、可回滚 | Hermes/Letta/LangGraph 会重写 Chat runtime |
| Queue | PostgreSQL durable + Redis wake | 强一致、重启可恢复 | goroutine/Redis-only 会丢任务 |
| Worker | 同 code/image 的独立 Go process | 复用 provider/task model/credential boundary，同时隔离 API 延迟和故障 | API 内 loop 会竞争资源；Python 会增加跨语言 authority |
| L0 | existing messages | 无复制、天然 provenance | JSONL 会成为第二主库 |
| L1 | visible natural-language canonical | 用户可看、可改、可忘记 | 纯 triplet/embedding 不可治理 |
| Scope | Global + first-class Project + Conversation | 防跨项目污染并支持多会话项目 | global-only 会串项目；字符串 tag 无 authority |
| Capture | 高置信自动、低置信/冲突 Review | 自动化与可纠错兼顾 | 全 Review 摩擦大；全自动会污染 |
| Sensitive | 个人自用可自动，但独立 egress gate | 满足个性化且保留可关闭边界 | “自用”等同“无边界”会扩大远程暴露 |
| Lexical | CJK shadow + pg_textsearch BM25 | 复用 PG，保留中文 token 行为 | English-only analyzer 不可靠 |
| Dense | BGE-M3/pgvector | 项目已有 1024d profile | 新 Qdrant/OpenSearch 无必要 |
| Fusion | RRF | 跨 lane score 尺度稳定 | 固定线性权重需大量校准 |
| Rerank | BGE reranker | 项目已有 provider/profile | LLM rerank 更慢、更贵、更不稳定 |
| Conflict | deterministic first + bounded LLM | 降成本、可审计 | ADD-only 会长期积累矛盾 |
| Delete | visibility + epoch + purge | 即时不可见且防复活 | 只删 vector/只写 history 不合格 |
| L2/L3 | end-state 包含、分别晚于 L1 晋升 | 先证明 L1，再避免错误放大 | 一开始全量 persona/scene 过重 |
| External | Hindsight shadow only | 现成引擎最完整且 PG-only | 直接生产有 migration/delete/authority 风险 |
| Portability | encrypted versioned export/import + dry-run | 自托管数据可迁移且不绕过 conflict/secret | DB dump 不适合作为用户格式 |
| Graph | 有证据再上 | 当前场景以偏好/事实为主 | Graph DB 增加运维且 delete 更复杂 |

## 19. 不采用的候选

- **直接 Hermes**：它是完整 Agent runtime，只借 contract/lifecycle。
- **直接 TencentDB Gateway**：`user_id` 未隔离、L1 recall 丢失、无 delete，且新增
  Node + SQLite/TCVDB。
- **直接 Mem0 Server**：auth 与 Memory scope 脱节、ADD-only、SQLite 残留正文。
- **直接 Graphiti**：示例 Server 无 auth、delete/queue 不闭环、需要 Graph DB。
- **直接 Cognee**：Postgres hybrid adapter 未接通、job 非 durable、全局 cache wipe。
- **Supermemory Local**：公开仓库不含核心 server 源码，binary black-box。
- **保持 v1 不动**：边界正确但召回弱、capture 会丢任务、无冲突/时间/provenance。

## 20. Verification matrix

| 层 | 必须验证 |
| --- | --- |
| Migration | 空库与 v1 fixture 从 migration 001 replay；manual/AI/missing-source backfill；N/N-1 compatibility；无 data-loss rollback |
| Authority | cross-user/project/conversation IDOR、CSRF、stale revision、selection token replay、portable ID spoof 全拒绝 |
| Worker | crash-after-provider/before-apply、lease expiry/reclaim、duplicate event、Redis down、provider timeout/429、profile drift |
| Capture | user-only evidence、confirmed assistant plan、quoted document/tool injection、secret redaction、scope routing、Review expiry |
| Temporal/conflict | ADD/NOOP/MERGE/Review、scope override、as-of/current、expiry、manual precedence、stale target retry |
| Recall | Exact/BM25/vector lane 独立、RRF/rerank、Sensitive SQL filter、600/900 token、900ms/1.5s/2s fallback |
| Delete | Memory/Conversation/Project/account immediate zero recall；old job/provider response 不复活；L2/L3/Hindsight purge |
| Backup | manifest off-host replication、14-day/8-week prune dry-run、old dump restore-before-open replay、projection rebuild |
| UI | Use/Learn tri-state、Activity polling/undo、Review actions、provenance/history/usage、archive/delete previews、a11y |
| Export/Import | wrong passphrase、tamper/truncation/zip-bomb equivalent、schema major、size caps、secret pre-staging reject、plan drift |
| Hindsight | dedicated DB/role isolation、fixture parity、30-day timer、opt-out/delete/drop proof、native unaffected |
| Performance | 500-case frozen corpus、critical Chinese slices、p95/p99/tokens/cost、CPU/RSS/WAL、queue circuits |

实现后的组件关卡：frontend 在 `mm-chat/frontend/` 跑 format/lint/typecheck/Vitest/build；
backend 在 `mm-chat/backend/` 跑 `go vet ./...`、focused + full `go test ./...`；Compose、
migration、backup/restore 和跨层安全完成后跑
`bash mm-chat/scripts/verify-standalone.sh --full`。任何 flaky/跳过 privacy test 都不算通过。

## 21. 最终交付定义

只有同时满足以下条件才算 Memory v2 可发布：

1. Server mode 唯一 authority，客户端无法指定/越权 user scope；
2. completed turn 到 durable event 同事务，重启/Redis down 不丢；
3. Global/Project/Conversation scope、Use/Learn/Sensitive policy 与 Project lifecycle 闭环；
4. 高置信 auto、Review、direct-user actions、assistant confirmation、temporal/conflict chain 正确；
5. exact/BM25/vector 三路独立候选，RRF/rerank/budget/profile/generation fence 生效；
6. L2/L3 分别 shadow/promotion/rollback，可追溯到 L1，不形成第二 authority；
7. 删除立即零召回/零发送，旧 job 不复活，24h online purge、8-week backup expiry 可证明；
8. 完整治理 UI、Activity、history/usage 与 encrypted Export/Import 通过 authority tests；
9. assistant-only 信息不能成为用户事实，secret 不落库，Sensitive/Hindsight egress 合规；
10. frozen benchmark、中文 critical slices、p95/p99/token/cost 和 queue circuit 全达标；
11. v1 fallback、feature flags、restore replay、rollback drill 和 full standalone gate 通过；
12. Hindsight 无论成功或失败都不影响 canonical Memory 和正常聊天。

实施已由魔尊于 2026-07-28 明确启动。PR1–PR13 已按第 17 章顺序完成；当前仍未默认切换
v2 reader、未调用 Live provider 或回放 Live Memory。Project/Review/L2 Scene/L3 Persona
API/UI 已开放为治理面，Project/Conversation/L2 Scene/L3 Persona 在默认 flags 下尚不
进入 prompt/Usage。Hindsight fixture 对照未达到晋升门槛，运行实例已销毁；真实 trial 仍需
单独显式授权，formal 500-case benchmark、shadow/canary 与 reader promotion 仍是后续运营门禁，
不属于本轮自动启用范围。
