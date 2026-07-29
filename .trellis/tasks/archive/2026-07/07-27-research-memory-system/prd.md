# Neo Chat Server Memory v2 调研、选型与实施

## Goal

查清 Neo Chat 当前 Server mode 长期记忆的真实运行链路，比较 2026 年主流
Agent Memory 方案，并在“单服务器、自托管、Go + PostgreSQL + Python RAG”
约束下，按 `info.md` 第 17 章的 PR1–PR13 顺序实施已冻结的完整 end-state。
魔尊已于 2026-07-28 明确回复“开始”，构成实施授权；PR1 benchmark contract、PR2
Project/scope/settings foundation、PR3 durable capture worker、PR4 provenance/delete
correctness、PR5 candidate/Review shadow、PR6 direct-user actions/Activity/Usage、PR7 L1
exact/CJK BM25 projection shadow、PR8 BGE-M3/vector hybrid shadow、PR9 governance 与
PR10 encrypted portability/retention、PR11 L2 Scene 与 PR12 L3 Persona 已完成。继续保留
v1 reader，不调用 Live provider 或回放 Live Memory；当前批次为 PR13 Hindsight
fixture-only adapter/profile，并在对照结束后销毁 Hindsight 运行实例。

## What I Already Know

- 产品以后只使用 Server mode；浏览器 Local Memory 不应成为 Server prompt
  的数据源。
- 当前 Live API 返回 `enabled=false`、`searchEnabled=true`、
  `autoRecordEnabled=false`，并且用户 Memory 为 0 条。
- Server Memory 已由 Go API 和 PostgreSQL migration `035` 承担权威；有设置、
  CRUD、回答前召回、回答完成后可选自动抽取。
- 当前召回是 Go 内存中的 lexical/CJK bigram 打分，不使用 embedding、BM25、
  reranker、关系图或时间推理。
- PostgreSQL 17 已安装 `pgvector 0.8.5` 和 `pg_textsearch 1.3.1`；RAG 已有
  BGE-M3 1024 维 embedding 与 BGE reranker 能力。因此无需为了 Memory 再引入
  Qdrant、OpenSearch 或另一套向量基础设施。

## Requirements

- 交付物必须是覆盖最终生产形态的 **完整 end-state 方案**，不得把需求收缩成“只做
  Phase 0 + Phase 1”或只给 MVP。分期只表示安全落地顺序，每一阶段的目标、依赖、
  晋升门槛、回滚和最终归宿都必须定义完整。
- 所有无法从 Live runtime、源码、配置、测试或外部固定版本研究中推导的产品偏好，
  必须通过 `grill-me` 一次一问冻结；每问给出推荐答案，魔尊回答后立即回写本 PRD
  和必要的 `info.md`。问题树未闭合前保持 `planning`，不得运行 `task.py start`。
- 以 Live runtime、当前源码和迁移为准，记录现状及已验证边界。
- 最终选型前必须检查魔尊随后提供的其他项目，比较其真实 Memory 数据模型、
  写入/召回链路、依赖、中文效果和可迁移部分；固定源码审计现已完成，Hindsight
  定位为“最强可审计现成 shadow”，而非生产默认 authority。
- 本轮指定参考项目：`NousResearch/hermes-agent` 与
  `TencentCloud/TencentDB-Agent-Memory`。必须以当前仓库源码、配置和部署入口为
  准，区分完整 Agent runtime、Memory component 与数据库产品集成。
- 比较 Hindsight、Supermemory、Mem0、Zep/Graphiti、Cognee、Letta、
  LangGraph，区分“原始能力强”与“适合 Neo Chat”。
- 厂商 benchmark 必须标明来源、约束和不可直接横比的风险。
- “最佳方案”只能在第一梯队候选完成固定 commit 的源码级审计后定稿；只查看
  README、官方文档或仓库页面的候选必须标成“文档级初筛”，不得与源码结论混写。
- 推荐必须符合 Server mode、自托管、数据留在服务器、最小新增运维面、Go API
  继续拥有鉴权和用户数据权威。
- 给出分期路线、验证指标、回滚边界和下一步决策选项。
- 给出可执行的详细技术方案，明确业务语义、系统组件、Go contract、PostgreSQL
  canonical/projection/job 数据模型、写入/召回/冲突/删除链、Hindsight shadow、
  失败降级、可观测性、分期与每项选择原因；方案本身不等于授权实施。

## Acceptance Criteria

- [x] 现有 Memory 的设置、CRUD、召回、prompt 注入、抽取、失败和删除链路已落盘。
- [x] Live 设置与数据状态已重新请求验证。
- [x] 主流候选的官方文档/仓库来源、能力、依赖和限制已落盘。
- [x] 合并本轮魔尊提供的参考项目后，给出适配 Neo Chat 的更新首选。
- [x] Hermes Agent 与 TencentDB Agent Memory 的实际链路、可复用点和不适配面已
  独立落盘并纳入比较矩阵。
- [x] Hindsight、Supermemory、Mem0、Graphiti、Cognee 已完成固定 commit 的源码级
  authority、隔离、删除、存储、召回和部署审计，并据此重排最终矩阵。
- [x] 给出不引入 Graph DB 的 MVP 以及何时才值得做 Graph PoC。
- [x] 详细技术方案已落盘，并说明组件边界、关键流程、数据结构、门槛和选择原因。
- [x] 完整 end-state 的产品决策树已逐项 grill 并由魔尊确认，方案不存在未标记的
  产品假设。
- [x] 完整方案确认后，再单独取得实施授权；确认设计不自动等于启动 migration、PoC
  或修改 Live Memory。
- [x] PR1 提供可承载 500-case 中文/中英 Memory benchmark 的 versioned fixture schema、
  strict validator、评分 contract 和自动化测试骨架，并复用现有 RAG Golden/promotion
  evaluator 的冻结哈希、split 与 exclusive-artifact 约束。
- [x] PR1 中的 example/draft fixture 必须显式 `promotionEligible=false`，不得伪装成
  已人工审核的 500-case frozen benchmark。
- [x] PR1 不改变 API/runtime 行为，不新增或修改 migration，不调用 Live provider，
  不读取、创建、修改或删除 Live Memory。
- [x] PR2 新增顺序 migration `053`，且不修改任何既有 migration 字节；新增 Project、
  Conversation Memory mode/scope generation、Memory settings 与 user memory scope 字段。
- [x] PR2 保留既有 `user_memory_settings` 三个开关的原值；新设置默认分别为
  `sensitive_memory_enabled=false`、`l2_mode=inherit`、`l3_mode=inherit`，尤其不得静默
  开启旧用户的 `auto_record_enabled`。
- [x] PR2 将所有既有 `user_memories` 回填为 `global` scope、空 Project/Conversation FK、
  `scope_generation=1`，并以 CHECK、composite ownership FK 与唯一索引阻止跨用户引用
  和非法 scope 组合。
- [x] PR2 保持旧 Memory API/CRUD/Recall 行为不变：旧 repository 创建的 Memory 仍为
  Global，同一用户的 active Global normalized content 仍正确去重。
- [x] PR2 不新增 Project 对外 API、不切换 v2 reader、不启动 worker、不调用 provider，
  且所有新 runtime flags 保持关闭。
- [x] PR2 migration up/down/re-up、schema/backfill/down guard 测试与既有 repository tests
  通过；存在 Project、非 Global Memory 或用户已修改新策略时 down 必须 fail closed。
- [x] PR3 新增顺序 migration `054`，且不修改任何已发布 migration 字节；新增 versioned、
  ID-only `memory_outbox` 与 leased `memory_jobs`，Redis 只能发送 `event_id` wake signal，
  不得成为 job authority 或保存正文。
- [x] assistant message finalize 与 eligible `turn.completed` outbox append 位于同一 PostgreSQL
  transaction；event payload 只含 source/assistant/conversation IDs、scope/profile/generation/
  hash references，不含 conversation、Memory 正文或 Provider secret。
- [x] 独立 `memory-worker` command/container 使用 PostgreSQL polling + optional Redis wake、
  bounded claim、lease expiry reclaim、bounded retry/dead-letter 和 idempotent completion；Redis
  不可用、worker crash 或滚动重启时 durable jobs 仍可恢复。
- [x] `memory_worker_runtime` 与 `go_api_runtime`、migration owner 分权；worker 只能经
  lease/user/source/generation-checked SQL functions claim、hydrate、complete/retry/apply，
  不获得 messages、user_memories、outbox 或 jobs 的任意跨用户直接 CRUD。
- [x] Worker 复用现有 Server provider/vault/config 构造与现有 extraction semantics，但 API
  内 request-local extraction goroutine 被完全移除；不得同时存在 API consumer loop 与独立
  worker。Offline tests 不调用 Live provider。
- [x] event schema 显式带 major version，worker 兼容当前 N 与 N-1 major 并 fail closed 拒绝
  unknown major；duplicate event/idempotency key、stale lease、cross-user hydration 与 source
  drift 均不得产生重复或越权 apply。
- [x] v1 Memory CRUD/Recall/prompt injection 与现有 HTTP contract 保持不变；PR3 不提前实现
  evidence/revision/epoch/tombstone、Review/conflict/temporal、BM25/vector、Project API/UI、
  L2/L3 或 Hindsight shadow。
- [x] Compose 增加同 backend image 的 private `memory-worker` service、独立 database URL/
  pool/concurrency/resource limit/healthcheck；不 publish 外网端口，API 在 worker/Redis 故障时
  继续聊天和 v1 Recall。
- [x] PR3 focused race/full test/vet、Compose/preflight、migration up/down/re-up 与 disposable
  PostgreSQL drill 通过，覆盖 finalize atomicity、duplicate、crash reclaim、stale lease、
  cross-user denial、worker role无直接表 CRUD及 Redis-down polling。
- [x] PR4 新增顺序 migration `055`，且不修改任何已发布 migration 字节；扩展
  `user_memories` 的 `revision`、`visibility_epoch`、`content_hash`、`authority_kind`、
  `extraction_profile_id`，新增 `user_memory_state`、`user_memory_evidence`、
  `user_memory_revisions`、`user_memory_tombstones` 与无正文 deletion manifest authority。
- [x] PR4 backfill 必须令每个用户 state 从 epoch 1 起步；既有 manual Memory 成为
  `manual` authority，只有仍存在、同用户且 role=`user` 的 source message 才能成为
  evidence；缺少 surviving user evidence 的既有 AI Memory 必须 fail closed 停用，不能
  凭 assistant 内容或 nullable source 猜 authority。
- [x] 手工 Create/Update 与 worker apply 必须维护 canonical `content_hash` 和单调
  `revision`；每次实际 canonical 变更只 append 一条 prior snapshot，revision history
  除受控 plaintext purge 外不可 UPDATE/DELETE。自动 Memory 至少一条 user evidence，
  evidence 只保存 message/conversation ID、hash 与 observed time，不复制消息正文。
- [x] 单条 Global Memory delete 必须在一个 transaction 内锁 user state/target row、立即
  `deleted_at` + disabled、写 targeted tombstone、无正文 deletion manifest、purge event/job；
  HTTP payload/状态码保持兼容，提交后 v1 Recall 与 Provider hydration 都不可再看到该条。
- [x] `memory_jobs` 支持 stage-specific extract/purge shape；purge job 不 hydrate Provider，
  只经 lease/user/epoch/tombstone-checked SQL capability 幂等擦除 canonical、revision 与
  evidence plaintext，并记录 bounded result。worker login 继续无表级直接 CRUD。
- [x] worker apply 必须同时重验 live lease、user、source existence/hash、provider profile/
  generation、live `visibility_epoch` 与 targeted tombstone；删除发生在 Provider 调用期间时，
  旧响应不得复活 Memory。自动 candidate 命中 manual authority 时只 NOOP，不得覆盖。
- [x] PR4 保持 v1 reader、Global-only API 与 response schema 不变，不提前实现 PR5 的
  Review/conflict/temporal/semantic scope routing，也不实现 PR7+ hybrid/vector reader、
  PR9 governance UI 或 PR10 encrypted off-host manifest export/replay/retention command。
- [x] PR4 schema/live PostgreSQL tests 覆盖 backfill、cross-user evidence、revision append-only、
  stale epoch/lease、delete immediate invisibility、old response no-resurrection、manual
  precedence、purge idempotency/plaintext wipe、manifest generation 与 guarded down/re-up；
  focused race、backend full test/vet、Compose/preflight 和 full standalone gate 通过。
- [x] PR5 新增顺序 migration `056`，且不修改任何已发布 migration 字节；为 canonical
  `user_memories` additive 增加 lifecycle、subject/fact key、confidence、observed/valid/
  expiry/supersede、sensitivity 与 temporal parser fields，并以 privacy-safe defaults 回填，
  v1 reader/CRUD response 不因字段存在改变。
- [x] PR5 新增 candidate batch authority、`user_memory_review_suggestions`、normalized target
  与 evidence join tables；一个 extract job 的最多 5 条 proposal 必须 candidate-wide atomic、
  hash-pinned、幂等，重放不得形成第二组 proposal 或部分 commit。Evidence 只存受权
  message ID/hash，Target 必须绑定同用户 current Memory revision，不能使用 UUID array
  冒充 ownership/revision authority。
- [x] Worker extraction contract 升级为 versioned strict JSON：候选显式携带 confidence、
  sensitivity、subject/fact key、authority/context message IDs、confirmation kind、absolute
  temporal fields、proposed scope/scope confidence；未知/重复字段、越界 JSON/正文/数组、
  非当前 Conversation/Project scope、无 surviving user authority 或伪造 target 均 fail closed。
- [x] Provider 前执行 deterministic secret redaction；secret/credential candidate 永不把正文、
  tag、key 写入 candidate/review/revision/log，Sensitive 取 model/local classifier 的更严格值，
  且用户 Sensitive switch 关闭时不得形成可保留的 sensitive proposal。Offline tests 不调用
  Live Provider。
- [x] PR5 proposal routing 先做 same-scope exact NOOP，再做 bounded current-target/fact-key
  conflict；低 confidence、低 scope confidence、ambiguous/relative temporal、manual target、
  MERGE/SUPERSEDE 或真实冲突只生成 pending Review。高置信无冲突只标记 shadow ADD，
  任何 outcome 都不得写/更新/supersede active canonical，跨 scope 同 fact 只视为 override。
- [x] Pending Review 与 shadow proposal 固定 30 天；到期由 provider-free `review_expire`
  worker lane 自动置 `expired` 并幂等擦除 candidate/normalized/tag/key plaintext，只留 ID/hash/
  reason/time/result。该 lane 必须在 Provider hydration 前 dispatch，使用 lease/user/batch fence，
  重试窗口覆盖 24 小时清除 SLA。
- [x] `memory_worker_runtime` 继续只有窄 SQL function execute、无 candidate/review/canonical
  table CRUD；PR5 migration 撤销旧自动 apply capability，使 N-1 worker 在 `056` 下不能继续
  写 canonical。Down 对任何 proposal/review/expiry history 或非默认 canonical metadata
  fail closed；clean `055 -> 056 -> 055 -> 056` 可重放。
- [x] PR5 focused race/full test/vet、PostgreSQL 17 replay/drill、Compose/preflight、backend image
  与 full standalone gate 通过，覆盖 strict candidate parse、secret zero-plaintext、scope/
  authority/target spoof、exact/manual/conflict/temporal routing、batch replay、crash-after-proposal
  resume、30-day provider-free expiry、direct-table denial 与 guarded down/re-up。
- [x] PR6 新增顺序 migration `057`，且不修改任何已发布 migration 字节；新增无 query/
  prompt/Memory 正文复制的 `message_memory_usages`、`message_memory_activities` 与
  direct-user action authority/normalized targets，并为 PR6 产生的 revision 保存可安全撤销
  所需的完整 prior typed snapshot。所有 ownership 使用同用户 composite FK 或窄 SQL 重验。
- [x] Chat 只对当前、completed、role=`user` 的 source message 运行 fail-closed lexical gate +
  versioned strict typed action planner；优先使用 Server-owned `task_model_settings.memory`，
  未配置时只回退到本轮已解析的 chat provider/model。未知/缺失/重复/trailing JSON、低置信、
  forged target、assistant/Memory/网页/附件/tool text 均不得触发 canonical mutation。
- [x] `remember|correct|forget` 只能形成 bounded typed proposal；Go 从当前 Conversation 绑定
  auth user、Project/Conversation scope 和 hydrated target revision，模型不能提供 user/scope ID
  authority。单一明确 target 才执行；0/多 target、scope 不存在、revision stale 或 exact
  conflict 只形成 hash/ID-only `review_required` action，不强制覆盖。
- [x] Direct `remember` 创建 `direct_user` authority canonical；`correct` 在 expected revision
  仍 current 时 append 一条完整 prior revision 再更新；`forget` 复用 tombstone/manifest/
  provider-free purge 语义。Secret/credential 只保留 hash/result，任何 direct action 都不能
  清旧 tombstone，explicit rebuild 必须创建新 canonical row。
- [x] Assistant finalize transaction 同步写入本轮实际注入的最多 5 条 L1 Usage link，绑定
  user、assistant、Memory ID/current revision/scope；不保存 query/content/embedding/raw score。
  Memory 删除后查询只返回 deleted marker，绝不从 revision 恢复旧正文。
- [x] Pending Review/rejected/dead-letter/direct action 形成 bounded Activity link；exact NOOP
  不写 Activity。提供 user-scoped cursor/poll API 与 activity undo capability，但 PR9 前不做
  frontend chip。Undo created 只在 target revision 未变时 Forget；undo corrected 只在 revision
  未变且完整 prior snapshot 存在时 append restore revision；stale 转 `review_required`，不得覆盖。
- [x] `go_api_runtime`/`memory_worker_runtime` 只能调用各自窄 capability，无 action/activity/
  usage/revision 任意写权限；down 对任何 PR6 action/activity/usage/full-snapshot history fail
  closed，clean `056 -> 057 -> 056 -> 057` 可重放。
- [x] PR6 strict planner、direct-only authority、scope/target/revision spoof、secret zero-plaintext、
  remember/correct/forget、usage finalize atomicity、Activity polling、NOOP silence、safe/stale undo、
  delete/purge 与 guarded down/re-up 均有 Go/static/PostgreSQL 自动化验证；focused race、backend
  full test/vet、Compose/preflight、backend image 与 full standalone gate 全部通过。
- [x] PR7 新增顺序 migration `058`，且不修改任何已发布 migration 字节；新增可重建、
  canonical-owned `user_memory_search_projections` 与 normalized shadow observation/result links。
  Projection 保存 scoped exact terms/CJK BM25 shadow、revision/hash/epoch/generation/profile fence，
  observation 只保存 query hash、ID/revision/rank/status/time，不保存 query、Memory 正文或 raw score。
- [x] PostgreSQL 17 与固定 `pg_textsearch 1.3.1` 是 PR7 lexical shadow 前提；复用已验证的
  Latin normalization、CJK bigram shadow 和 `simple` BM25 index，不新增 jieba/PGroonga/search
  service。Migration 对现有 eligible canonical rows backfill generation 1 projection，并验证
  content hash、scope、sensitivity、epoch、revision 与 ready count 完整一致。
- [x] Canonical insert/update/enable/lifecycle/scope/sensitivity/epoch/content/revision 变化必须在
  同 transaction 自动同步 projection；logical delete、disable、supersede/expire/reject、plaintext
  purge 必须立即物理移除 derived lexical plaintext。Projection trigger/function 为 internal-only，
  所有 runtime login 均无 projection table CRUD 或任意 sync capability。
- [x] Shadow query 只接收当前 authenticated user、current streaming assistant、所属 active
  Conversation/Project 与 bounded current query；SQL 在 candidate probe 前绑定 user、scope、
  Sensitive switch、visibility epoch、scope generation、enabled/lifecycle/validity/expiry/current
  canonical revision/hash。跨用户、归档/删除、stale projection 与 secret 均不得进入任何 lane。
- [x] Exact lane Top 20 与独立 CJK BM25 lane Top 30 形成 deterministic lexical Top 20 shadow；
  normalized result rows分别记录 v1/exact/bm25/lexical ordinal。相同 assistant/query/v1 baseline
  exact replay 幂等，任何 payload drift fail closed；shadow error 只返回 bounded code，不能改变
  v1 Top 5、prompt、Usage 或聊天成功状态。
- [x] `MEMORY_LEXICAL_SHADOW_ENABLED` 默认 `false`，只控制 shadow comparison/observation，不控制
  projection correctness；PR7 不修改 `user_memory_state.active_retrieval_profile_id`，不提供 reader
  promotion API，不让 lexical shadow 进入 prompt。Metadata 只可暴露 profile/status/count/overlap/
  duration 等无正文 diagnostics，不暴露 query、raw BM25 score、内部 user/scope authority。
- [x] `go_api_runtime` 只能执行 read/compare capability，无 projection/observation table CRUD；
  `memory_worker_runtime` 不获得 PR7 authority。Down 只允许 v1/NULL reader pointer，且存在 shadow
  observation history时 fail closed；clean `057 -> 058 -> 057 -> 058` 可重放，derived projection
  可在未运行 shadow时安全丢弃并由 re-up 重建。
- [x] PR7 static/Go/PostgreSQL tests 覆盖 CJK/Latin/punctuation terms、projection backfill与所有写链
  同事务维护、跨用户/scope/Sensitive/time/epoch/generation过滤、deleted/purged zero recall、lane
  independence/ranking、exact replay/conflict、shadow-disabled zero calls、shadow failure v1 unchanged、
  role denial 与 guarded down/re-up；focused race、backend full test/vet、Compose/preflight、backend
  image 与 full standalone gate 全部通过。
- [x] PR8 新增顺序 migration `059`，且不修改 migration `001`–`058` 已发布字节；在 PR7
  canonical projection 上 additive 增加固定 `siliconflow_bge_m3_v1`/1024d embedding binding、
  HNSW cosine index、lease-fenced derived embedding jobs，以及 query/content/raw-score-free 的
  normalized hybrid observation/result links。
- [x] Canonical create/content/revision/hash 变化必须在同 transaction 使 vector 变为 `pending` 并
  幂等排队；scope/epoch/generation-only rebind 可安全复用同 profile/content vector。Delete、disable、
  non-active、purge 继续立即物理删除 projection/job；旧 provider response 不能写回新 revision、
  hash、epoch、scope generation、projection generation 或 provider config。
- [x] Memory worker 只有在 `MEMORY_HYBRID_SHADOW_ENABLED=true` 时才 claim embedding job；默认
  `false` 必须零 Memory embedding/rerank Provider 调用。Worker 只经窄 claim/hydrate/complete/retry
  capability 读取单条 current Memory 与受权、enabled、attested `RAG:SILICONFLOW` secret；不得表
  CRUD、批量导出正文或把 plaintext/embedding 写入日志、Activity、Usage、outbox/job payload。
- [x] Hybrid prepare 只绑定当前 authenticated user/current streaming assistant/current completed user
  parent 与 active Conversation/Project；Exact Top 20、CJK BM25 Top 30、BGE-M3 cosine Top 30 均在
  SQL candidate probe 前应用 user/scope/Sensitive/time/epoch/generation/revision/hash/profile fence。
  Query embedding 缺失/失败时只跳过 vector lane，不得影响 exact/BM25 或聊天。
- [x] 三路 candidate 按 `RRF(k=60)`、memory ID 去重和 deterministic tie-break 形成 Top 20；不得线性
  混合不可比 raw scores，raw BM25/cosine/RRF/relevance score 均不得持久化。相同 assistant/query/
  ordered v1 baseline replay 幂等，任何 payload/profile drift fail closed，不重写首次 completed evidence。
- [x] 固定 `Pro/BAAI/bge-reranker-v2-m3` 只 rerank 当前已受权的 RRF Top 20 transient Memory 内容；
  invalid/timeout/failure 降级为 RRF order，query embedding/vector SQL failure 降级 lexical lanes，
  2s hard cutoff 使用已完成 lane。Shadow final 最多 5 条，按 conservative multilingual estimate
  保持 hard `≤900` tokens并记录 `600` target telemetry，不通过无限等待换预算。
- [x] PR8 flag 只控制 vector worker与 hybrid comparison，不修改 `active_retrieval_profile_id`，不让
  hybrid final 进入 prompt/Usage，也不提供 reader promotion API。v1 Top 5、prompt、Usage、chat
  success 必须保持唯一 authority；metadata 只暴露固定 profile、bounded status/fallback/counts/
  tokens/duration/overlap，不暴露 query、Memory content、raw score、内部 user/scope/provider authority。
- [x] `go_api_runtime` 只能执行 hybrid prepare/record capability；`memory_worker_runtime` 只能执行
  embedding lease capabilities，二者无 projection/job/observation table CRUD。Down 只允许 v1/NULL
  reader pointer 且 hybrid observation history为空；clean `058 -> 059 -> 058 -> 059` 可重放，derived
  vector/job 可安全丢弃并由 re-up 重建。
- [x] PR8 static/Go/PostgreSQL tests 覆盖 embedding backfill/invalidation/lease/retry/old-response fence、
  fake-provider vector shape、三 lane independence、RRF determinism、rerank/fallback/timeout、600/900
  budget、cross-user/scope/Sensitive/time/stale/delete过滤、exact replay/conflict、flag-off zero calls、
  v1 byte-equivalent prompt/Usage、role denial 与 guarded down/re-up；不调用 Live Provider、不触碰 Live
  Memory，并通过 focused race、backend full test/vet、Compose/preflight、backend image 与 full gate。
- [x] PR9 新增顺序 migration `060`，且不修改 migration `001`–`059` 已发布字节；只增加
  user-bound governance snapshot/mutation capabilities、Review decision audit 与 guarded rollback，
  `go_api_runtime` 不获得 Project/Review/evidence/revision/diagnostic table CRUD。
- [x] `/v1/memory-settings` 扩展 Sensitive/L2/L3 preference；新增 current-user Project create/list/
  edit/archive/restore 与 Conversation membership/Use/Learn `inherit|on|off` policy。移动 Conversation
  必须 revision/generation-fenced，Archive Project 强制 effective Learn=false 但不改用户显式 mode。
- [x] Governance Memory list/create/update/move/forget 支持 Global/Project/Conversation scope、expected
  revision、authority/lifecycle/sensitivity/validity badge；旧 `/v1/memories` Global shape保持兼容。
  Secret manual/edit input在写库前拒绝；Forget 复用 tombstone/manifest/purge 并返回 immediate hidden、
  online purge 与 8-week backup-expiry 分层状态，不从 revision 恢复 deleted plaintext。
- [x] Memory detail 只对 current authenticated owner hydrate surviving evidence、append-only revision
  timeline 与 Usage links；source 删除只返回 marker。Search diagnostics 只返回 profile/status/fallback/
  counts/tokens/duration，不暴露 query、content、embedding、raw score 或 Provider authority。
- [x] Pending Review API 支持 keep current、accept new、edit merge、keep both、reject；每次 decision
  必须重验 suggestion status/expiry/epoch/scope generation/target revisions/Sensitive authority，显式 user
  actor 才能创建或 supersede canonical，随后擦除 candidate plaintext并保留 ID/hash/result audit。
- [x] Server Memory governance UI 展示全局 Use/Learn/Sensitive、Project 与 Conversation policy、三层
  scope Memory、provenance/history/usage、Review actions、Activity 与 delete progress；Local Memory/Dream
  在 Server mode继续隐藏而非删除，所有 loading/empty/error/stale 状态和键盘/accessible name 完整。
- [x] Assistant 回答旁 Activity chip 按 assistant message ID 做可见页短轮询，展示 created/corrected/
  review/failed bounded状态并复用 revision-safe undo；terminal 后停止 polling，不把 candidate/source
  raw text复制进 message metadata或浏览器持久化。
- [x] PR9 只让 Conversation `Use Memory=off` 阻断现有 v1 prompt注入，不切换
  `active_retrieval_profile_id`、不把 hybrid shadow晋升为 reader、不调用 Live Provider；Project/
  Conversation scoped Memory在后续 promotion 前仅可治理且不进入 v1 prompt/Usage。
- [x] PR9 static/Go/PostgreSQL/frontend tests 覆盖 cross-user Project/policy/Memory/Review IDOR、revision/
  generation drift、archive Learn override、secret zero-plaintext、Review decision replay/conflict、deleted
  source/history/purge状态、Activity polling/undo、Server authority与 a11y；通过 frontend全套、focused
  race、backend full test/vet、migration `059 -> 060 -> 059 -> 060`、Compose/preflight/image/full gate。
- [x] PR10 新增顺序 migration `061`，且不修改 migration `001`–`060` 已发布字节；新增仅含
  ID/hash/status 的 import batch 与离线 deletion replay authority，扩展 `source=import`/imported
  revision contract，并保持 runtime role 无 portability/deletion/import 表级 CRUD。
- [x] Export 生成 versioned passphrase-encrypted `.mm-memory`：外层固定
  `filippo.io/age v1.3.1` scrypt authenticated stream，内层严格 canonical JSONL manifest/records；
  manifest 绑定 format/schema、UTC time、release、history flag、counts 与 records SHA-256。Passphrase
  只存在请求/命令内存，plaintext archive 不落盘，禁止自造加密或把 passphrase 写入 env/log/job/DB。
- [x] Export 只包含当前用户 Project metadata、settings suggestion、current L1 canonical Memory 的
  content/type/scope/lifecycle/validity/sensitivity/tags 与可选 revision history；portable refs 不携带源
  `user_id` authority。不包含 raw chat/conversation metadata、evidence excerpt、projection/vector、L2/L3、
  outbox/jobs、Usage/Activity/log、Provider request、credential 或部署 secret。
- [x] Import 严格执行 `decrypt/authenticate -> manifest hash/schema/caps -> local secret detector -> field/
  scope validation -> dry-run`，硬上限为解密后 256 MiB、50,000 Memory、200,000 revisions、1,000
  Projects、单 content 2,000 Unicode code points；解析使用 bounded line buffers，不把整个 plaintext
  archive 放入内存或 staging table。Secret/credential 只返回 ordinal/hash/reason 的 `REJECT`，正文
  在任何 staging/persistence 前拒绝。
- [x] Dry-run 只输出 `NOOP|ADD|REVIEW|REJECT|SCOPE_REQUIRED`，并对外部 Project/Conversation refs
  强制当前 auth user mapping/create/re-scope/skip；外部 user/project/conversation/Memory ID 不能成为
  authority。Settings 默认只展示建议且 confirm 不应用；冲突只报 `REVIEW`，不得覆盖 canonical。
- [x] Confirm 必须重新提交同一 encrypted package，并绑定 auth user、package/manifest/mappings/plan
  hash、短期签名 token、current Memory revisions、Project/Conversation scope generations 与 authority
  state hash；任何 drift/tamper/expiry 都要求重新 dry-run。只原子写入 `ADD`，source/authority 均为
  `import`，不伪造 local message evidence；optional imported revision chain 先验证连续 hash 后受控写入。
- [x] 提供 encrypted off-host deletion manifest export/replay；manifest 只含 event/user/memory/tombstone
  opaque ID、content hash、scope generation、visibility epoch、deleted/purged time/result，不含正文。
  Replay 幂等、Provider-free，必须在 backend 未开放时先恢复立即不可见/正文擦除，再全量重建 derived
  projection；旧 backup 中匹配 ID/hash 的已删 Memory 不得复活。
- [x] Production backup wrapper 生成统一 backup-set manifest/checksum pair，并标记
  `daily|weekly|pre-deploy`、exact relative artifact/checksum path、SHA-256、createdAt 与
  `containsMemoryPlaintext=true`。Retention command 默认 dry-run，daily 14 天、weekly/pre-deploy 8 周，
  execute 绑定 plan SHA-256；只处理固定 root 内无 symlink 且 artifact/checksum/manifest 全部校验通过的
  complete set，任何 drift fail closed，孤立或未纳入 set 的文件不删除。
- [x] PR10 API/UI/CLI/static/PostgreSQL tests 覆盖 age round-trip/wrong passphrase/tamper、unknown/duplicate/
  trailing JSON、caps、secret zero-persistence、cross-user mapping、plan/token/state drift、confirm replay、
  history chain、restore-before-open replay、projection rebuild、retention dry-run/path/symlink/checksum/plan
  drift 与 guarded `060 -> 061 -> 060 -> 061`；不调用 Live Provider、不读取/修改 Live Memory，v1
  Global Top 5/prompt/Usage 保持唯一 reader authority。
- [x] PR11 新增顺序 migration `062`，且不修改 migration `001`–`061` 已发布字节；新增 L2 Scene、
  member revision/hash、derived hybrid projection、leased refresh/embedding/purge job、content-free search
  observation、promotion event 与独立 L2 reader pointer。L2 始终是可重建 derived data，不得成为第二
  canonical authority，runtime roles 不获得 Scene/member/projection/job/promotion 表级 CRUD。
- [x] Scene 只聚合同一用户、同一 Global 或 Project scope 的 current active L1；Conversation L1 不得
  提升到 Project/Global Scene。每个 Scene 固定记录 topic key、成员 L1 ID/revision/content hash、scope
  generation、visibility epoch、L2 generation、profile 与全量 source watermark；Provider 输出中的 member
  ID 必须是本次受权输入集合的子集，所有身份、hash、sensitivity 与 watermark 由本地/SQL 重算。
- [x] `MEMORY_L2_SCENE_SHADOW_ENABLED=false` 默认关闭 Scene refresh 与 shadow retrieval Provider 调用；
  flag=false 时 queued refresh 不得 claim，但 provider-free stale/purge 仍可运行。Worker refresh 使用独立
  lease lane，pin user/scope/Project generation/epoch/L2 generation/profile/Provider record+updatedAt/watermark，
  strict versioned JSON 最多 8 个 Scene、每个 2–20 个 member；unknown/duplicate/oversized/secret/跨 scope/
  forged member 或 Provider deadline 后返回均 fail closed，旧响应不得覆盖新 generation。
- [x] Sensitive 总开关同时约束 L1 hydrate、Scene Provider egress、Scene 落库/projection 与最终注入；
  secret/credential sentence 在 Provider 前本地删除，完全 redacted scope 零调用，Provider 输出命中 secret
  拒绝整批且不落 plaintext。Scene sensitivity 取 member 与本地 derived-content classifier 的更严格值，
  不能信任 Provider label。
- [x] 任意 L1 canonical create/update/move/disable/supersede/delete、Project archive/generation、visibility
  epoch 或 Sensitive policy 变化，必须在数据库 authority 层使受影响 Scene 立即 stale、移除 reader
  projection、推进 L2 generation 并 enqueue current-scope rebuild；read/complete 再逐 member 检查 validity/
  expiry/revision/hash，时间到期即使尚未物化也不得召回。Stale Scene plaintext/member/projection 24 小时内
  provider-free purge，account cascade 与 8-week backup retention 继续覆盖 derived data。
- [x] Scene retrieval 复用固定 Exact/CJK BM25/BGE-M3/RRF(60)/BGE reranker，最多只召回当前 Conversation
  可见的相关 Global/Project Scene，绝不每轮注入全量 navigation；candidate/final 上限 20/2，L2 hard
  budget 500 estimated tokens、2s cutoff。每条 candidate 与 Provider 后 final 均重验 user/scope/Sensitive/
  member/epoch/generation/profile authority，durable observation 只保留 IDs/hash/ordinal/bounded telemetry，
  不保存 query/content/vector/raw score/Provider secret。
- [x] L2 profile 必须独立执行 `shadow -> active -> rollback`。Promotion 只能由 migration-owner 的显式
  capability 完成，且同时验证严格 passing 500-case benchmark report、零 cross-user/secret/delete/provider
  leak、7 天且至少 100 个 eligible shadow turns、零 dead-letter，以及当前 L1 hybrid reader pointer；
  evaluator本身仍不能 promotion。当前没有合格正式 evidence，因此 migration 只 seed shadow profile，
  `MEMORY_L2_SCENE_READER_ENABLED=false` 默认关闭，PR11 发布不得自动写 active pointer 或声称已晋升。
- [x] Server governance API/UI 展示 L2 server profile/status、用户 generation、每个 Scene 的 scope/topic/
  content/status/profile/generation/source watermark/member count/更新时间；detail 只 hydrate current member L1
  与 surviving evidence/source-deleted marker。用户可逐 Scene disable/enable、按 Scene 或全用户 rebuild；
  “修改 Scene”只能调用既有 governed L1 create/update 后由 stale/rebuild 链生成新 Scene，不提供 derived
  plaintext PATCH，disabled Scene 不得被后台 refresh 静默重新启用。
- [x] Active Scene reader 即使 env flag 开启，也必须同时满足数据库 promotion、用户 `l2_mode!=off`、
  Memory Use/Search、current L1 hybrid pointer、Scene active/current generation/member authority；任一失败或
  Scene retrieval/provider timeout 都 fail-open 到原 L1 reader。Shadow 永不进入 prompt/Usage；rollback 或
  flag-off 立即停止 L2 注入且不影响 L1、聊天、canonical Memory 或 L3 generation。
- [x] PR11 static/Go/PostgreSQL/frontend tests 覆盖 strict Scene JSON、scope/member/watermark spoof、secret/
  Sensitive zero-egress、lease reclaim/old response、L1 write/delete/expiry stale、24h purge、disabled preserve、
  exact/BM25/vector/RRF/rerank/budget、shadow zero injection、promotion denial/active/rollback、cross-user/role
  denial、governance detail/rebuild/a11y 与 guarded `061 -> 062 -> 061 -> 062`；不调用 Live Provider、不读取/
  修改 Live Memory，并通过 focused race、backend/frontend/RAG full、Compose/preflight/image/full gate。
- [x] PR12 新增顺序 migration `063`，且不修改 migration `001`–`062` 已发布字节；新增独立 L3 Persona
  profile/version/member、derived hybrid projection/embedding、leased refresh/purge、content-free search observation、
  promotion event 与 reader authority。Persona 始终是可重建 derived data，不得覆写 L1、成为第二事实主库，
  runtime roles 不获得 Persona/member/projection/job/promotion 表级 CRUD。
- [x] Persona 只聚合同一 auth user 的 current active Global L1，且仅接受稳定 `fact|preference|instruction|
  warning|decision`；Project/Conversation scope、`project|context`、disabled/superseded/expired/deleted 或当前
  Sensitive policy 不允许的 L1 均不得进入 Provider input。每个 member 固定 revision/content hash/visibility
  epoch/L3 generation，全量 source watermark 与 sensitivity/token count 均由 Go/SQL 重算，Provider 只能提议
  content 与本次 hydrated authority 子集中的 member IDs。
- [x] `MEMORY_L3_PERSONA_SHADOW_ENABLED=false` 默认关闭 Persona refresh、query embedding 与 shadow retrieval
  Provider 调用；flag=false 时 provider-free stale/purge 仍可 claim。Worker 使用独立 lease lane，pin user/epoch/
  L3 generation/profile/Provider record+updatedAt/watermark，strict versioned JSON 只允许一个 Persona、2–50 个
  unique members，目标 200–300 tokens、hard `≤300` estimated tokens；unknown/duplicate/oversized/secret/forged
  member 或 Provider deadline 后返回均 fail closed，旧响应不得覆盖新 generation。
- [x] Sensitive 总开关同时约束 Persona L1 hydrate、Provider egress、derived content/projection 与最终注入；
  secret/credential sentence 在 Provider 前本地删除，完全 redacted 或不足 2 个受权 member 时零调用。Persona
  sensitivity 取所有 member 与本地 derived-content classifier 的更严格值；Sensitive 从 on→off 时旧 Persona
  立即 stale/不可读，必须在新 generation 生成无敏感版本后才可再次 active。
- [x] 任意参与资格相关的 Global L1 create/update/move/disable/supersede/delete/expiry、visibility epoch 或
  Sensitive policy 变化，必须在数据库 authority 层推进 L3 generation、使旧 Persona 立即 stale、移除 reader
  projection并 enqueue refresh；read/complete 再逐 member 检查 revision/hash/validity。Stale Persona plaintext/
  member/projection 24 小时内 provider-free purge，account cascade 与既有 8-week backup retention 同样覆盖。
- [x] Persona retrieval 使用独立 Exact/CJK BM25/BGE-M3/vector/RRF(60) relevance gate，只允许当前 user 的 current
  generation/version；candidate/final 上限 5/1、L3 hard budget 300 estimated tokens、2s cutoff。Provider 后 final
  重新验证 user/Sensitive/member/epoch/generation/profile authority；durable observation 只保留 IDs/hash/ordinal/
  bounded telemetry，不保存 query/content/vector/raw score/Provider secret。query embedding失败降级 lexical，
  Persona 失败不改变 L1/L2 或聊天结果。
- [x] L3 profile 独立执行 `shadow -> active -> rollback`，不得依赖或改写 L2 pointer/generation。Promotion 只能
  由 migration-owner 显式 capability 完成，并同时验证 passing 500-case report、persona consistency `≥0.95`、
  false injection `≤0.02`、token saving ratio `≥0.20`、零 cross-user/secret/delete/provider leak、7 天且至少
  100 个 eligible shadow turns、零 dead-letter，以及当前 L1 hybrid reader pointer。当前无正式 evidence，
  migration 只 seed shadow profile，`MEMORY_L3_PERSONA_READER_ENABLED=false`，不得自动 promotion。
- [x] Server governance API/UI 展示 L3 profile/status/generation、current Persona content/status/revision/token count/
  sensitivity/source watermark/member count/更新时间；detail 只 hydrate current member L1 与 surviving evidence/
  source-deleted marker。用户可独立 disable/enable/rebuild；“修改 Persona”只能进入既有 governed L1 correction
  后重建，不提供 derived plaintext PATCH，refresh 不得静默重新启用 user-disabled Persona。
- [x] Active Persona reader 即使 env flag 开启，也必须同时满足数据库 promotion、用户 `l3_mode!=off`、Memory
  Use/Search、current L1 hybrid pointer、Persona active/current generation/member authority；任一失败、rollback 或
  flag-off 都立即零 L3 注入且 fail-open 到原 L1/L2。Shadow 永不进入 prompt/Usage；active 使用独立低优先级
  `<relevant-user-persona>` block，只发送 content、不发送 Persona/member IDs，L1 原子 Memory 始终优先。
- [x] PR12 static/Go/PostgreSQL/frontend tests 覆盖 strict Persona JSON、scope/type/member/watermark spoof、secret/
  Sensitive zero-egress、lease reclaim/old response、L1 write/delete/expiry stale、24h purge、disabled preserve、
  exact/BM25/vector/RRF/budget、shadow zero injection、promotion denial/active/rollback与 L2 independence、cross-user/
  role denial、governance detail/rebuild/a11y 与 guarded `062 -> 063 -> 062 -> 063`；不调用 Live Provider、不读取/
  修改 Live Memory，并通过 focused race、backend/frontend/RAG full、Compose/preflight/image/full gate。
- [x] PR13 固定 Hindsight `0.8.5`、upstream commit
  `e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a`、官方 multi-arch image digest 与
  PostgreSQL/pgvector image digest；adapter 只依赖已审计的 REST retain/recall/bank
  config/delete contract，不 vendor 上游源码或依赖 generated SDK。
- [x] PR13 提供严格版本化、bounded、duplicate-key/unknown-field/trailing-value 拒绝的
  synthetic fixture manifest 与 content hash binding；输入必须显式声明 synthetic-only、
  no real user data、no sensitive data、promotion ineligible，且任何 Live Memory、Live chat、
  Live Provider credential、`.env.single-server` 或 provider vault 都不能成为输入。
- [x] PR13 adapter 使用 server-generated stable opaque bank mapping；调用者只能提交受校验的
  fixture alias/mode，不能提交 Hindsight `bank_id`、Neo Chat user ID、database URL、API URL、
  credential 或任意 endpoint。Cross-bank fixture/result 混用必须 fail closed。
- [x] PR13 同时实现 `end_to_end` 与 `retrieval_only` 两条独立 fixture profile：前者用 Hindsight
  local mock extraction 验证 retain→recall 完整链，后者用 `chunks` 灌入等价冻结事实隔离检索
  差异；两者都只使用容器内预载 local embedding/reranker，不调用任何 Live/remote Provider。
- [x] Hindsight 只运行于独立 Compose project 的显式 optional profile；API、独立 PostgreSQL
  database/role、runner 只连接 `internal: true` private network，不 publish host port，不接入
  Neo Chat `private`/`rag-private` network，不挂载 Neo Chat database/data/secrets/backup，且使用
  ephemeral API key/DB password、read-only runner、cap drop、no-new-privileges、CPU/RSS/PID 上限。
- [x] PR13 不新增 frontend/HTTP route/migration/runtime flag，不进入 chat prompt、Usage、Activity、
  canonical/projection/job，也不改变 v1/L1/L2/L3 reader 或 Native Memory；Hindsight timeout、5xx、
  malformed response、unavailable 或 resource failure 只能使 benchmark fail，不能影响正常聊天。
- [x] Adapter 只输出 content-free observation/report：允许 case/fixture alias、opaque logical Memory
  ID、rank/count/status、bounded error code、latency/resource/hash/version；禁止输出 query、fixture
  plaintext、Hindsight fact text/raw score/trace、bank ID、API key、DB URL、Provider request 或 raw error。
- [x] 覆盖 end-to-end/retrieval-only、CJK/mixed、cross-bank、delete-after-recall、wrong API key、
  timeout/cancel、oversized/malformed response、default-off/zero-network、resource/config topology 与
  Native Memory unchanged tests；offline Go tests必须使用 `httptest`，不得访问网络或 Provider。
- [x] 对照命令无论成功、失败或中断都执行 scoped teardown；只允许对 PR13 独立 Compose project
  删除 exact containers/network/volume/secrets tempdir，并验证零 bank/audit/LLM request/file/
  async-operation/graph-queue/DB/role/volume/container/network 残留，禁止对主 Compose project 使用
  `down -v` 或删除 `mm-chat/data|secrets|backup|.env.single-server`。
- [x] 对照完成后删除 Hindsight 运行实例；仅保留脱敏 content-free report、synthetic fixtures、
  adapter/test harness 与 pinned provenance 以便复现。即使 Hindsight 胜出也只能触发新评审，
  不能保留实例或自动进入生产；未来任何真实 30 天 trial 必须由魔尊重新显式授权并创建全新
  隔离实例，禁止复用 PR13 key/database/role/volume/bank。
- [x] PR13 focused race/full backend tests、Compose static/render、container/PostgreSQL purge drill、
  failure/timeout/resource/teardown tests、backend image 与 `verify-standalone.sh --full` 全部通过；
  运行门禁期间不得调用 Live Provider、读取/修改 Live Memory 或启动主应用 Hindsight 集成。

## Definition of Done

- 调研文件可独立复核，结论与证据分离。
- 未将厂商自报分数写成独立第三方事实。
- PR1 benchmark contract、validator 与 focused tests 通过，并且 backend `go test ./...`
  与 `go vet ./...` 通过。
- PR1 未修改 runtime、migration、Live 设置、Memory 数据或部署拓扑。
- PR2 additive schema、backfill、ownership constraints、Global repository compatibility 与
  guarded rollback 均有自动化验证，backend `go test ./...` 与 `go vet ./...` 通过。
- PR3 durable event、job lease/replay、独立 worker 拓扑与最小权限 SQL capability 均有自动化
  验证，backend `go test ./...`、`go vet ./...`、Compose render 与 disposable PostgreSQL
  drill 通过。
- PR4 provenance/delete/purge correctness 与 least-privilege capability 均有自动化验证，
  backend `go test ./...`、`go vet ./...`、Compose render 与 disposable PostgreSQL drill
  通过；online canonical/revision plaintext purge 可幂等重放且旧 worker 响应不可复活。
- PR5 candidate/Review shadow、temporal/conflict/scope routing、candidate-wide idempotency 与
  30-day provider-free plaintext expiry 均有自动化验证；没有自动 candidate 改变 canonical。
- PR5 未启用 v2 reader、Project/Review API/UI 或 projection 能力；PR6–PR13 继续
  遵循 `info.md` 的逐批门槛、回滚和单一 authority 约束。
- PR6 direct actions、Usage/Activity links 与 safe undo 均有自动化验证；v1 reader 与
  Global CRUD compatibility 保持可用，PR9 前不要求 frontend governance UI。
- PR7 projection/shadow comparison、least-privilege capability、fail-open chat 与 guarded
  rollback 均有自动化验证；feature flag 默认关闭，v1 prompt 与 Usage authority 未改变。
- PR8 vector projection/embedding lease、三路 RRF/rerank/budget fallback、secret egress guard、
  least-privilege capability 与 guarded rollback 均有自动化验证；hybrid flag 默认关闭，v1
  Top 5、prompt、Usage 与聊天成功仍是唯一 authority。
- PR9 Project/Conversation policy、scoped governance、Review decision、detail/history/Usage、
  Activity chip/undo 与 delete progress 均有跨层自动化验证；Go/SQL 双重 Sensitive/secret
  防线、legacy wrapper capability、current-only plaintext hydration 与 guarded rollback 已通过
  PostgreSQL 17 实证，v1 Global Top 5 仍为唯一 prompt/Usage authority。
- PR10 encrypted portability、off-host deletion replay 与 backup retention 均通过 tamper/authority/
  rollback/restore 实证；plaintext package/passphrase/secret 不落盘或持久化，受支持 restore 不复活
  deleted Memory，v1 Global Top 5 仍为唯一 prompt/Usage authority。
- PR11 L2 Scene 与 PR12 L3 Persona 的 shadow/promotion/governance/rollback 均已通过各自门禁，
  两层 reader flag 默认关闭且不改变 v1 L1 默认 prompt/Usage authority。
- PR13 fixture-only adapter、双轨 profile、私网独立拓扑、content-free report 与 scoped teardown
  均有自动化证明；对照结束后 Hindsight runtime state 为零，Native Memory/chat 行为始终不变。

## Technical Approach

固定源码审计后的首选路线为 **保持 Neo Chat Go/Postgres 权威，先建立 Go MemoryEngine
contract、durable outbox 和原生 hybrid recall v2；Hindsight 作为同 contract 下的
shadow benchmark adapter，并使用独立 PostgreSQL database + role**。Hermes Agent
贡献可插拔 lifecycle/失败隔离设计，
TencentDB Agent Memory 贡献 L0→L3 layering/provenance/渐进调度设计，但都不替换
现有 runtime；TencentDB Gateway/SQLite 不直接接入。

无论 bake-off 结果如何，Go 继续拥有 auth/user scope/settings/delete/API；现成
引擎只能位于内部网络，作为可重建的 derived memory engine，不能直接成为对外
用户权威。若中文、资源、删除或可解释性门槛不通过，则落回原生 Memory v2，
复用当前 BGE-M3、BM25/pgvector、reranker、outbox/worker 能力。

## Decision (ADR-lite)

**Context**：Neo Chat 已有完整 chat runtime、用户鉴权、Postgres 检索栈和 Python
worker。整套接入外部 memory server 会重复 user/auth/database/API authority，并
扩大备份、迁移、故障和数据删除面。

**Decision（固定源码审计后）**：不立刻替换生产 Memory。生产方向优先原生
PostgreSQL v2，外部引擎只能通过可替换 adapter 做 shadow；Hindsight 是当前最强
可审计现成对照，但不再是生产实现的默认前置。Mem0 只作成熟行为基线，Graphiti
只作关系/时间专项，Cognee 暂因一致性/cache isolation 阻断，Supermemory Local
因核心 binary 不可审计而不入选。不得用 Hermes/Letta/LangGraph 重写 runtime，也
不让 TencentDB/Hindsight 或任何 vendor 直接成为对外主库。

**Consequences**：先建设对生产有复用价值的 contract/outbox/native shadow，再用
短 benchmark 回答现成 engine 是否有额外收益；代价是需要验证 sidecar authority、
独立 database 资源隔离、中文 lexical 与完整删除传播。无论选择哪条实现线，Graph
能力只有在 benchmark 证明显著收益后才晋升。

## Current Batch Out of Scope

- PR13 不实施真实用户 30 天 shadow trial，不复制、去标识或回放任何 Live Memory/chat，也不接入
  Live Provider；历史上“全门槛后可 trial”的产品偏好不构成本批授权。
- PR13 不把 Hindsight 做成生产 `MemoryEngine`、reader、worker、frontend/API 功能或长期运行服务，
  不新增 migration，不修改 active profile/pointer/flag，不进入 prompt/Usage/Activity。
- PR13 不为了第三方对照修改 Native Memory ranking、L2/L3、benchmark 门槛或 synthetic Golden
  authority；对照报告只是 evidence，不能 promotion。
- PR13 不保留 Hindsight container、database/role/key/network/volume/bank/audit/cache/file/queue/log
  runtime state；未来 trial 必须单独评审、重新授权并新建隔离实例。
- 不把 AGENTS/system/project 固定规则迁入概率性 Memory。
- 不把供应商 benchmark 当作发布门槛；发布必须使用 Neo Chat 中文数据集复测。

## Research References

- [`research/current-memory-runtime.md`](research/current-memory-runtime.md) — 当前
  Live/Backend/Frontend 真实链路与缺口。
- [`research/market-memory-systems-2026.md`](research/market-memory-systems-2026.md) —
  市场候选、官方证据、benchmark 限制与适配矩阵。
- [`research/recommended-memory-v2.md`](research/recommended-memory-v2.md) — 推荐架构、
  分期、评测门槛、风险和回滚边界。
- [`research/reference-projects-hermes-tencentdb.md`](research/reference-projects-hermes-tencentdb.md) —
  Hermes contract、TencentDB 四层 pipeline、源码缺口与可复用设计。
- [`research/source-audit-top-memory-engines.md`](research/source-audit-top-memory-engines.md) —
  Hindsight、Supermemory、Mem0、Graphiti、Cognee 的固定 commit 六轴源码审计与
  最终重排。
- [`info.md`](info.md) — 可执行的 Memory v2 技术设计、原因、分期、验证与回滚边界。

## Confirmed Product Decisions

1. **自动抽取采用分级入库（2026-07-27，魔尊选择 A）**：高置信、无冲突且符合
   数据边界的 candidate 自动写入；低置信或与 current Memory 冲突的 candidate
   进入 Review，不参与 Recall；secret/credential 永久拒绝落库；自动流程不得静默
   覆盖 manual Memory。敏感内容是否自动写入由决策 2 覆盖本项最初推荐。
2. **敏感内容采用宽松个性化边界（2026-07-27，魔尊选择 C，个人自用）**：健康、
   财务状况、宗教/政治观点、性取向、法律/家庭关系、精确位置等敏感事实，只要
   高置信且无冲突即可自动成为 active Memory；低置信或冲突仍进入 Review。
   password、API key、Token、cookie、private key、OTP/恢复码、银行卡安全码、认证
   凭据等 secret/credential 仍永久拒绝，不因单用户部署放宽。
3. **允许敏感 Memory 按相关性发送给远程 provider（2026-07-27，魔尊选择 A）**：
   Memory extraction、embedding/rerank 和回答可把完成任务所需的最小敏感片段发送给
   Server 配置的硅基流动等 provider；必须先做 secret redaction，只发送相关 Top-K，
   不写 raw logs/telemetry，不进入 Hindsight shadow 或 benchmark，并提供可关闭的
   Sensitive Memory 总开关。关闭或删除后立即停止发送。
4. **assistant 生成的计划/承诺/决定必须由用户确认（2026-07-27，魔尊选择 A）**：
   assistant message 只能作为内容上下文，不能单独授权长期 Memory；只有用户明确
   同意、复述/修改后确认、手动保存或明确开始执行时，才以该 user message 作为
   authority evidence 写入。沉默、未反对或仅要求“给个方案”均不构成确认。
5. **采用 Global + Project + Conversation 三层 scope（2026-07-27，魔尊选择 A）**：
   新增由 Go/PostgreSQL 掌权的 first-class Project 实体，不能用自由文本 tag 冒充；
   Global 用于跨会话稳定画像，Project 用于同项目多会话共享，Conversation 只对单聊
   生效。Recall 只合并当前 Conversation、所属 Project 与 Global，事实冲突优先级为
   `Conversation > Project > Global`，用户可以移动/提升/下放 Memory scope。
6. **自动抽取采用语义 scope 路由（2026-07-27，魔尊选择 A）**：稳定个人事实、通用
   偏好和跨项目指令进入 Global；Project 会话中的项目技术栈、约束、决定和进度进入
   Project；临时、仅本次有效或归属不明内容进入 Conversation；无 Project 会话中的
   项目类内容不得擅自进入 Global；scope 置信不足进入 Review。管理页手工创建默认
   Global，会话内创建默认当前最具体 scope，用户可随时移动。
7. **Project 采用 Archive + Permanent Delete 两阶段（2026-07-27，魔尊选择 A）**：
   Archive 保留 Project、Conversation 与 Memory，隐藏出活跃列表，已有会话仍可查看
   和 Use Memory，但暂停 Learn，恢复后继续；Permanent Delete 先展示影响数量，立即
   停止 Project Memory Recall 并异步物理擦除，Conversation 默认保留并移到未归类，
   不自动提升 Project Memory 到 Global；“同时删除 Conversation”是独立、默认不勾选
   的破坏性选择，所有旧 job 必须被 tombstone/generation 拦截。
8. **Memory 删除采用“立即不可见 + 在线 24 小时 + backup 最多 8 周”契约
   （2026-07-27，魔尊选择 A）**：删除 transaction 提交后立即零 Recall/零 provider
   发送；canonical/revision/evidence/projection 正文 24 小时内物理清除，超时告警并重试；
   加密备份不做原地篡改，daily 保留 14 天、weekly 和含 Memory 的 pre-deploy backup
   最多 8 周，monthly drill 长期只留无正文校验记录；独立加密 deletion manifest 只存
   ID/hash，恢复时必须先重放再开放 API，最迟 8 周后所有受支持备份不含被删正文。
9. **Conversation 删除采用 provenance 精确处理（2026-07-27，魔尊选择 A）**：
   Conversation-scoped Memory 随会话删除；AI 生成的 Project/Global Memory 移除该
   会话 evidence，有其他存活 user evidence 则重建保留，否则删除；manual Memory
   默认保留并移除已删来源，因为其 authority 来自用户显式保存。删除 UI 显示影响
   数量，并提供默认不勾选的“同时删除从此会话手工保存的 Memory”；旧 job 全部受
   source hash/tombstone fence 约束。
10. **每个 Conversation 使用独立 Use/Learn 三态策略（2026-07-27，魔尊选择 A）**：
    用户首次全局启用 Memory 时，Use 与 Learn 默认都开启；每个 Conversation 分别保存
    `inherit/on/off`，默认 inherit，可独立“不用但学”或“使用但不学”。Project Archive
    强制暂停 Learn 但允许 Use；manual save 不受 Learn 限制；Sensitive Memory 继续受
    独立总开关。迁移不得把已有用户明确关闭的 `auto_record_enabled` 静默改开。
11. **L2 Scene 与 L3 Persona 分别过门槛后默认开启且完全可控（2026-07-27，魔尊
    选择 A）**：两层先 shadow 生成且不注入，分别通过 L1 稳定性与 frozen benchmark
    后晋升；L2 按 Project/主题聚合相关 L1，L3 是 200–300 token 紧凑画像，每次只召回
    相关 derived projection。UI 必须展示内容、更新时间、来源 L1/evidence，并允许分别
    关闭和重建；用户修改 derived 内容时生成 L1 correction 后重建，不直接把摘要变成
    第二事实权威；Sensitive Memory 总开关同时约束生成和注入。
12. **采用完整可审计 Memory 治理 UI（2026-07-27，魔尊选择 A）**：列表展示内容、
    类型、scope、Manual/Auto/Confirmed、lifecycle、更新时间、敏感标记和 Recall 状态；
    provenance 可下钻 surviving evidence/source_deleted；Conflict Review 并排提供保留
    当前、接受新值、编辑合并、全部保留、拒绝；可查看 revision/supersede timeline、
    本轮回答使用的 Memory、L2/L3 到 L1/evidence 链和删除影响/进度。不得暴露
    embedding、内部 prompt、provider secret 或无治理价值的 raw score。
13. **Pending Review suggestion 保留 30 天（2026-07-27，魔尊选择 A）**：到期自动
    `expired` 且绝不写入 active Memory，candidate 正文在到期后 24 小时内物理清除，
    只留 hash/reason/time/result；UI 显示数量和剩余时间，支持批量接受、拒绝和立即
    清空；未处理 Conflict 期间继续使用原 current Memory。
14. **采用准确优先但有硬预算的运行档位（2026-07-27，魔尊选择 A）**：Candidate
    Recall@20 `≥0.95`、Final Recall@5 `≥0.90`、current-fact accuracy `≥0.95`、
    false-injection `≤2%`；Memory Recall 额外耗时 `p95≤900ms`、`p99≤1.5s`，`2s`
    硬超时后按已完成 lane 降级；注入平均 `≤600`、单次最多 `900 tokens`；滚动 30 天
    Memory 增量 provider 成本不得超过主聊天模型成本的 15%；extraction/L2/L3 全部
    后台执行，不增加当前回答等待。
15. **异步任务采用独立 Go Memory worker process/container（2026-07-27，魔尊选择
    A）**：与 backend 共用 Go code/image，但使用独立 command、least-privilege DB
    role、连接池、provider 并发和 CPU/RSS 限额；API 只做权威事务、outbox、同步 Recall
    与 Redis wake，worker 做 extraction/conflict proposal/embedding/L2/L3/purge/rebuild。
    Worker 不暴露外网端口，崩溃/升级只积压 PostgreSQL durable jobs，不阻断聊天；不得
    把职责并入 Python RAG，也不得同时运行 API 内 loop 造成双 worker 拓扑漂移。
16. **Hindsight 通过全门槛后允许一次限时真实 shadow（2026-07-27，魔尊选择 A）**：
    默认 fixture-only；隔离、中文效果、完整删除、资源、成本和故障注入全部通过后，
    用户可主动开启一次 30 天 trial，只镜像非敏感 L1，Sensitive/secret/L2/L3 永不
    发送。使用 trial 专属 database/role/network/resource，结果只做差异报告且不进回答；
    到期自动关闭，停止/删除/退出后 24 小时内清 bank/audit/LLM cache/file/queue，无法
    证明清除则禁止真实 trial；继续试验必须重新确认。
17. **发布采用分层晋升与自动熔断（2026-07-27，魔尊选择 A）**：先 additive migration
    且 flags 全关，再通过 500-case frozen benchmark；L1 shadow 至少 7 天且 100 个
    eligible turns，随后只对选定 Project/Conversation canary 至少 7 天且 50 个有效
    Recall，再全局 L1 观察 14 天；L2、L3 分别重新走 shadow/canary/active，不与 L1
    大爆炸上线。cross-user/secret/delete-after-recall 任一非零立即全局熔断；性能、
    false injection、错误/backlog、成本或 generation drift 越过冻结阈值自动切回 v1/
    上一 reader。回滚只切 feature flag/active pointer，不删除 canonical/outbox/schema，
    v1 Reader 至少保留一个完整发布版本。
18. **聊天内支持受控的“记住/忘记/改成”Memory actions（2026-07-27，魔尊选择 A）**：
    直接 user message 可创建 manual-equivalent Memory、显式指定 Global/Project/
    Conversation scope、提出 correction 或 forget；单一明确匹配可执行，多匹配/scope
    歧义必须先让用户选择。LLM 只能产生 typed proposal，Go 必须重验 auth user、Memory
    ID/revision、scope、secret 和 delete fence；assistant/Memory/网页/文档/tool output
    永远无权触发。删除后“撤销”通过显式重新创建实现，旧 tombstone 不由 worker 清除。
19. **自动写入使用非阻塞 Memory Activity chip（2026-07-27，魔尊选择 A）**：后台
    完成后在对应回答旁显示 created/updated/review 数量，展开可看内容、type、scope、
    source/reason，并可编辑、移动、Forget 或撤销本次自动变更；撤销绑定原 revision，
    无后续变更才删除新增/恢复 merge 前版本，stale 时转 Review。NOOP 不提示，失败进
    轻量状态中心，Review 明示尚未参与回答；SSE 结束后的结果用受权 polling 回填，
    不新增 WebSocket 服务。
20. **提供版本化、加密、可 dry-run 的 Memory Export/Import（2026-07-28，魔尊选择
    A）**：导出 passphrase 加密 `.mm-memory` 包，包含 L1、Project/scope/status/settings
    及可选 revision history，不含 chat raw text、projection、L2/L3、jobs/logs/credentials；
    manifest 绑定 schema/count/hash/time。Import 流式验密、schema/hash/size 后先生成
    dry-run：exact→NOOP、new→ADD、conflict→Review、secret→拒绝且不得落 staging；外部
    user/project/conversation ID 不可信，Project 映射/创建必须经当前 auth user，缺失
    Conversation scope 需改 scope 或跳过。用户确认计划后才写入，source 标记 `import`，
    不伪造 provenance，也不自动改变现有 Memory 开关。

## Product Decision Register

技术路线中能由源码与研究决定的部分已经定案；以下产品与运营偏好均已由魔尊逐项
确认并冻结：

1. `[已冻结：A，经问题 2 修订敏感项]` 高置信无冲突 candidate 自动写入；低置信或
   冲突进入 Review；secret/credential 永久拒绝；manual Memory 不被静默覆盖。
2. `[已冻结：C]` 个人自用，除 secret/credential 外，高置信无冲突的敏感事实允许
   自动写入。
3. `[已冻结：A]` 敏感 Memory 可按最小相关性发送给已配置远程 provider；secret
   先行剔除，禁止进入日志、telemetry、Hindsight shadow 与 benchmark，并有总开关。
4. `[已冻结：A]` assistant 计划/决定必须有用户明确确认；沉默和未反对不算确认。
5. `[已冻结：A]` 使用 Global + first-class Project + Conversation 三层 scope，冲突
   优先级为 Conversation > Project > Global，支持用户移动 scope。
5a. `[已冻结：A]` 自动按语义路由；Global 只收稳定跨场景内容，Project 收项目内容，
   临时/无归属内容进 Conversation，scope 不确定进入 Review。
5b. `[已冻结：A]` Project 支持 Archive；永久删除默认擦除 Project Memory、保留并
   解除 Conversation 归属，不自动提升 Global；级联删 Conversation 必须另行勾选。
6. `[已冻结：A]` 删除立即不可召回/发送，在线正文 24 小时内物理清除；含正文 backup
   最多 8 周，恢复必须先重放只含 ID/hash 的 deletion manifest。
7. `[已冻结：A]` Conversation scope 随会话删除；AI Memory 按 surviving evidence
   重建或删除；manual Memory 默认保留，可显式勾选一起删除。
8. `[已冻结：A]` Use/Learn 是每会话独立 `inherit/on/off`，首次全局启用后默认都开；
   manual save 不受 Learn 限制，Archive Project 强制暂停 Learn。
9. `[已冻结：A]` L2/L3 先 shadow，分别过门槛后默认开启；全量可见来源，可独立关闭/
   重建，修改通过 L1 correction，不建立第二事实权威。
10. `[已冻结：A]` 使用完整可审计 UI：provenance、Conflict Review、revision chain、
    answer usage、L2/L3 下钻和删除进度全部可见，内部 prompt/embedding/secret 不暴露。
10a. `[已冻结：A]` Pending Review 保留 30 天，过期不入库且正文 24 小时内 purge；
     Conflict 未决期间 current Memory 保持生效。
11. `[已冻结：A]` 准确优先但受硬预算约束：Recall p95 900ms/p99 1.5s/2s cutoff，
    平均 600/最大 900 tokens，Memory 增量 provider 成本 ≤ 主聊天成本 15%。
12. `[已冻结：A]` 使用同 code/image 的独立 Go Memory worker container，独立最小权限
    role/资源/并发；Worker 故障只积压 durable jobs，不影响聊天 API。
13. `[已冻结：A]` Hindsight 默认 fixture-only；全门槛通过后可主动开启一次 30 天
    非敏感真实 shadow，结果不进回答，到期自动关闭并在 24 小时内彻底 purge。
14. `[已冻结：A]` additive-off → 500-case gate → L1 shadow 7 天/100 turns → selected
    canary 7 天/50 recalls → global L1 观察 14 天；L2/L3 分别晋升，越界自动回滚且不
    删除 canonical/outbox/schema。
15. `[审计新增·已冻结：A]` 支持 direct-user typed Memory actions；Go 重验全部 authority，
    单一明确匹配可执行，歧义先选择，非 user 内容永远不能触发。
16. `[审计新增·已冻结：A]` 回答旁显示非阻塞 Activity chip，可查看/治理并按 revision
    安全撤销；NOOP 不提示，异步结果用受权 polling 回填。
17. `[审计新增·已冻结：A]` 提供版本化 passphrase-encrypted Export/Import；先 dry-run，
    exact NOOP、冲突 Review、secret pre-staging reject，外部 scope IDs 不可信。

本轮两个参考项目已完成源码/运行链路对比；若魔尊还有项目则继续并入矩阵。实施已获
授权，PR1–PR12 已按顺序完成；当前批次为 PR13 Hindsight fixture-only adapter/profile，
并在对照结束后删除其运行实例。任何 Live Memory 修改、Provider 调用或后续真实 shadow
仍受对应分期门槛与新的单独授权约束。
