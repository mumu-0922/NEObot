# Neo Chat Server Memory v2 调研、选型与实施

## Goal

查清 Neo Chat 当前 Server mode 长期记忆的真实运行链路，比较 2026 年主流
Agent Memory 方案，并在“单服务器、自托管、Go + PostgreSQL + Python RAG”
约束下，按 `info.md` 第 17 章的 PR1–PR13 顺序实施已冻结的完整 end-state。
魔尊已于 2026-07-28 明确回复“开始”，构成实施授权；当前批次只实施 PR1 的
benchmark skeleton 与 contract tests，不修改 runtime 行为、migration 或 Live Memory。

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

## Definition of Done

- 调研文件可独立复核，结论与证据分离。
- 未将厂商自报分数写成独立第三方事实。
- PR1 benchmark contract、validator 与 focused tests 通过，并且 backend `go test ./...`
  与 `go vet ./...` 通过。
- PR1 未修改 runtime、migration、Live 设置、Memory 数据或部署拓扑。
- PR2–PR13 继续遵循 `info.md` 的逐批门槛、回滚和单一 authority 约束。

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

- PR1 不实现 migration、worker、embedding、rerank、UI、Graph DB 或任何 runtime 接线；
  这些能力只可在后续 PR2–PR13 按门槛逐批实施。
- PR1 不启用当前 Memory、自动记录，也不创建、修改、删除或导出用户 Memory。
- PR1 不调用 Live provider，不运行 Hindsight 真实 shadow，不生成虚假的人工审核数据。
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
授权，但必须从 PR1 离线 benchmark contract 开始；任何 Live Memory 修改、provider
调用或后续真实 shadow 仍受对应分期门槛与单独授权约束。
