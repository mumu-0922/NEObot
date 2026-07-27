# 参考项目拆解：Hermes Agent 与 TencentDB Agent Memory

调研日期：2026-07-27。结论基于以下源码快照，而不是仅按 README 宣传判断：

- `NousResearch/hermes-agent`：commit
  `d71033a4077a6dfdcdb42c9e9eeab4c41e4a7012`，`hermes-agent 0.19.0`，MIT。
- `TencentCloud/TencentDB-Agent-Memory`：commit
  `45e6e80ae2e63b65fad0d89f5e13171229c8f295`，npm `0.3.6`，MIT。

## 结论先行

这两个项目不是二选一：

- **Hermes Agent 是完整 Agent runtime，不是 Memory engine。** 不应替换 Neo Chat，
  但它的 `MemoryProvider + MemoryManager` 边界、生命周期、隔离、超时和降级设计很
  值得移植成 Go contract。
- **TencentDB Agent Memory 才是 Memory engine/component。** 它的
  `L0 Conversation → L1 Atom → L2 Scenario → L3 Persona`、可下钻 provenance、
  中文 jieba/FTS、后台分层调度很有参考价值；但当前默认 SQLite/TCVDB、Node
  sidecar、Hermes Gateway 多用户隔离和召回链存在明显不适配，不能原样接管 Neo
  Chat Memory。
- **最适合 Neo Chat 的组合不是“部署其中一个”，而是：借 Hermes 的插件契约，
  借 TencentDB 的分层模型，落到现有 Go + PostgreSQL + Python RAG 中。**
  Hindsight 保留为同 contract 下的 shadow benchmark adapter，而非先入生产。

## 1. Hermes Agent：应该借 contract，不应换 runtime

### 1.1 它实际提供什么

Hermes 当前有完整的模型、工具、会话、TUI/Gateway、cron、subagent、技能和持久化
runtime。Memory 只是其中一个可插拔子系统。内置 external provider 包括 Honcho、
Mem0、Hindsight、Supermemory、OpenViking、RetainDB、ByteRover 等，且一次只允许
一个 external provider。

核心抽象位于 `agent/memory_provider.py`：

```text
initialize(session_id, hermes_home, platform, identity...)
system_prompt_block()
on_turn_start(...)
prefetch(query, session_id)
queue_prefetch(query, session_id)
sync_turn(user, assistant, session_id, messages)
on_session_end(messages)
on_session_switch(...)
on_pre_compress(messages)
on_memory_write(action, target, content, metadata)
get_tool_schemas() / handle_tool_call()
backup_paths()
shutdown()
```

`MemoryManager` 负责：

- provider 注册与工具名冲突保护；
- 外部 recall 最长等待 8 秒，卡死调用未返回前后续 turn 直接跳过；
- turn 后写入经单 worker FIFO 串行化，避免 N+1 先于 N 落库；
- provider 失败不阻断主回答；
- session end/switch 在同一 FIFO 中保证顺序；
- shutdown 最多等待 5 秒并报告被放弃的 write/prefetch；
- skill 展开后的大段 scaffolding 不进入 Memory，只取真实用户指令；
- interrupted/partial turn 不写 Memory；
- cron、subagent 等可用 `skip_memory` 关闭污染。

真实调用顺序是：

```text
turn start
  -> on_turn_start
  -> prefetch(original user message)
  -> 召回结果以 sidecar 方式拼进本次 user API content
  -> Agent/tool loop
  -> final response 完成且未 interrupted
  -> sync_turn(clean user, final assistant, full messages) [background FIFO]
  -> queue_prefetch(clean user) [background FIFO]
```

### 1.2 值得 Neo Chat 直接吸收的设计

建议在 Go 定义一个更窄的 `MemoryEngine` port：

```go
type MemoryEngine interface {
    Recall(ctx context.Context, req RecallRequest) (RecallResult, error)
    Capture(ctx context.Context, event CompletedTurnEvent) error
    Forget(ctx context.Context, tombstone MemoryTombstone) error
    Rebuild(ctx context.Context, userID string, generation int64) error
    Health(ctx context.Context) Health
}
```

重点借鉴：

1. **单一 external engine**：防止双写权威和 prompt/tool surface 膨胀。
2. **统一 lifecycle**：turn completed、session boundary、delete、rebuild 都走同一
   contract，不在 handler 中散落 vendor 特判。
3. **完成态写入**：partial/interrupted answer 不当作稳定证据。
4. **session/user/profile scope 显式传递**：不能靠全局变量或客户端自由提交 ID。
5. **失败隔离与 deadline**：Memory 是 best-effort recall，不能拖死聊天。
6. **FIFO + durable queue**：Hermes 只有内存 worker；Neo Chat 应升级为 PostgreSQL
   outbox，而不是照搬 daemon thread。
7. **可备份/可重建/可删除**：provider state 必须声明运维边界。

### 1.3 不应该照搬的部分

- Hermes 把召回块标为 `authoritative reference data`；Neo Chat 当前的“低优先级、
  不可信历史声明”更安全，应保留，防 Memory prompt injection。
- Hermes 的 background executor 是进程内 daemon thread，5 秒后可放弃写入；这不是
  Server Memory 所需的 durability。
- external prefetch 仍可能阻塞当前 turn 最长 8 秒；Neo Chat 应设置更短 budget、
  分 lane deadline，并支持 stale/fallback。
- `sync_turn(user, assistant)` 容易把 assistant 幻觉写成用户事实。Neo Chat extraction
  必须区分 evidence role，assistant 只能帮助消歧，不能单独成为 user fact 来源。
- 内置 `MEMORY.md/USER.md` 与 external provider 并存属于 Hermes 产品语义；Neo Chat
  不应制造第二套隐藏 canonical Memory。

## 2. TencentDB Agent Memory：应该借 layering，不应直接接 sidecar

### 2.1 真实四层链路

它通过 `TdaiCore + HostAdapter + LLMRunnerFactory` 解耦 OpenClaw/Hermes，核心链路为：

```text
agent_end / POST /capture
  -> L0: 清洗并增量写 conversations/YYYY-MM-DD.jsonl
  -> L0 metadata/FTS 立即写 SQLite
  -> L0 embedding 后台补写（SQLite）
  -> checkpoint + pipeline notify
  -> 每 1→2→4→...→5 turns 或 idle 600s 触发 L1
  -> LLM 同时做 scene segmentation + atom extraction
  -> vector/FTS 候选查重
  -> LLM 决策 store/update/merge/skip
  -> records/YYYY-MM-DD.jsonl + SQLite dual-write
  -> L1 后 10s 且满足限流时触发 L2 scene Markdown
  -> L2 完成后检查阈值，通常每 50 个新 atoms 触发 L3 persona.md
```

默认参数中的关键值：

- L1：warm-up `1→2→4→5`，稳定后每 5 turn；idle 600 秒；单次最多 20 条。
- L2：L1 后延迟 10 秒；同 session 最短间隔 900 秒；最长轮询 3600 秒；最多
  15 个 scene。
- L3：每 50 个新 memory；persona 备份 3 份，scene 备份 10 份。
- Recall：Top-5、score threshold `0.3`、总 timeout 5 秒；默认 strategy 虽写
  `hybrid`，但默认 `embedding.provider=none`，实际会退化成 FTS-only。
- SQLite FTS5 写入和查询均用 jieba `cutForSearch`，中文能力不是简单靠
  `unicode61`；TCVDB 路径另有中文 BM25 sparse encoder。

L1 atom 类型只有三种：

- `persona`：稳定属性、偏好、习惯、技能；
- `episodic`：客观事件/决定/计划，可带 activity start/end；
- `instruction`：长期回答规则，含 priority `-1` 的“死命令”。

其 provenance 有 `source_message_ids`、`scene_name`、session、timestamps；L2/L3 是
Markdown，便于人工检查并下钻回 L1/L0。这比“扁平向量堆”更适合个人长期记忆。

### 2.2 召回链路

OpenClaw 原生路径：

- L1 动态 memory：FTS5 BM25 + vector 并行，client-side RRF；TCVDB 可 server-side
  dense+sparse+RRF。
- L2 scene navigation 和完整 L3 persona：每轮读取后放进稳定 system context。
- L1 Top-K：放进动态 user prepend context；有可选单条/总字符 budget。
- 另暴露 L1 memory search、L0 conversation search 两个工具。

这是“macro persona + micro fact + evidence drill-down”的好思路，但当前会**每轮整块
注入 persona 和全部 scene navigation**；它们没有相关性过滤和默认 token 上限。对
Neo Chat 应改成：稳定 persona 只注入小型 approved profile，L2 scene 先检索再注入，
L1 按 query 召回，三层各自有 token budget。

### 2.3 当前 Hermes Gateway 的阻断级缺口

以下不是理论风险，而是当前 commit 的源码行为：

1. **Hermes 自动召回丢掉 L1。** `performAutoRecall()` 生成动态
   `prependContext` 和稳定 `appendSystemContext`；但 `POST /recall` 只把
   `appendSystemContext` 填进 response `context`。因此 Hermes provider 获得的是
   persona/scene/tools guide，搜到的 L1 atoms 不会自动进入 prompt。响应里的
   `memory_count` 甚至可能非零，却没有相应正文。
2. **`user_id` 未形成数据隔离。** Python client 在 recall/capture/session-end 都发送
   `user_id`，Gateway request type 也声明了字段，但 route handler 没有使用它；
   SQLite schema 无 `user_id`，persona/scene 也是整个 `dataDir` 一份。多个 Neo Chat
   用户接同一 Gateway 会共享检索库和 persona，属于不可接受的跨用户泄露风险。
3. **无用户级 edit/forget/delete API。** Gateway 只有 health、recall、capture、两类
   search、session-end、seed；没有按 user/memory/conversation 删除和 tombstone
   传播。无法对接现有 Memory CRUD 和“删除后绝不召回”契约。
4. **写入不是 durable queue。** Hermes provider 自己再开 daemon thread 调
   `/capture`，Hermes Manager 外面又有 background FIFO；Gateway/进程异常时没有
   client-side spool/outbox。软上限到达且旧线程仍卡住时仍会继续创建新线程。
5. **重启不会立即恢复待处理 L1。** L0 和 checkpoint 在磁盘上，但 pipeline
   `recoverPendingSessions()` 把 `conversation_count` 转成 L2 pending 并只启动 L2
   timer，没有立即重新排 L1；要等后续 capture 才可能重新触发 L1 读取旧 cursor。
6. **更新/合并不是单事务双写。** JSONL 先追加、VectorStore target delete 与新记录
   upsert 分段执行；失败时可能出现 JSONL/SQLite 暂时分歧，依赖 cleaner/rebuild。
7. **Gateway auth 默认关闭。** 默认绑定 `127.0.0.1` 较安全，但 Docker 文档暴露
   `8420:8420`；一旦改为非 loopback 而未显式启用 API key，capture/search 都可被
   调用。它会 warning，但不是 fail-closed。

因此，**当前版本不能作为 Neo Chat production sidecar 直接接入**。即便只服务单一
管理员，L1 Gateway 召回丢失和 delete contract 缺失也必须先修。

### 2.4 值得 Neo Chat 吸收的部分

1. **L0 不重复存一份 JSONL**：Neo Chat 已有 canonical conversations/messages，
   直接把它们当 L0 evidence，并以 message ID 做 provenance。
2. **L1 Atom**：保留自然语言正文，增加 `persona/episodic/instruction`、priority、
   fact key、valid time、source message IDs、extraction generation。
3. **候选后 LLM 冲突裁决**：先 exact/hash + BM25/vector 缩小候选，再做
   `ADD/NOOP/SUPERSEDE/MERGE`；不要全库 LLM compare。
4. **L2 Scene**：不是给每条 atom 加一个字符串标签，而是生成可审计的项目/生活
   场景摘要，保留成员 atom IDs；只有相关 scene 才进入 prompt。
5. **L3 Persona**：作为 derived projection，可从 L1/L2 重建；应有版本、证据和
   token 上限，不能取代用户可编辑 Memory。
6. **渐进触发**：新用户快速形成第一批记忆，稳定后批量提取以降成本；idle/session
   end 可强制 flush。
7. **中文检索**：jieba/中文 BM25 的思路可留作 `pg_textsearch` 中文效果不达标时的
   tokenizer 对照；主线优先复用当前 BGE-M3 + PostgreSQL。
8. **human-readable projection + backup**：L2/L3 可提供 UI 可审计视图，但 canonical
   仍在 PostgreSQL，不把 Markdown 文件变成另一份主库。

### 2.5 不应移植的部分

- 不新增 Node 22 + SQLite/TCVDB + Gateway；这会重复 PostgreSQL、RAG worker、备份
  与身份边界。
- 不复制整个 persona/scene navigation 到每轮 system prompt。
- 不把用户长期 instruction 标成 priority `-1` 后当 system rule 执行；Memory 始终
  是不可信历史数据。
- 不采用“vector DB 删除、JSONL 留旧版本等待 cleaner”的双权威模型；PostgreSQL
  一次事务更新 canonical/status/outbox，向量和摘要是可重建 projection。
- Context Offload/Mermaid 解决的是长任务上下文压缩，不是本轮跨会话长期 Memory
  的必需项，应独立立项，避免混层。

## 3. 三条接入路线比较

`5` 为最适合 Neo Chat 当前约束。

| 路线 | Memory 能力 | 现栈复用 | 用户隔离/删除 | 运维 | 可回滚 | 结论 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 直接换 Hermes runtime | 4 | 1 | 3 | 1 | 1 | 不选，重写产品 runtime |
| 直接接 TencentDB Gateway | 4 | 2 | 1 | 2 | 3 | 不选，当前有阻断级 contract 缺口 |
| Hindsight sidecar 首先入生产 | 5 | 4 | 3 | 3 | 4 | 只保留 shadow benchmark，不先晋升 |
| **原生 PG v2 + 可插拔 adapter** | 4→5 | 5 | 5 | 5 | 5 | **首选** |

## 4. 更新后的推荐架构

```text
Go API / auth / Memory CRUD（唯一 authority）
  ├─ MemoryEngine port（借 Hermes contract）
  ├─ PostgreSQL outbox + idempotent worker
  ├─ L0 = existing conversation/message evidence
  ├─ L1 = atomic memory + provenance + validity/supersession
  ├─ L2 = derived scene/project summaries + member memory IDs
  └─ L3 = versioned compact persona projection

Recall
  exact/fact-key + Chinese BM25 + BGE-M3 pgvector
  -> RRF -> BGE reranker -> scope/status/time/token filter
  -> relevant L1 + optional relevant L2 + compact approved L3
  -> low-priority untrusted memory block

Adapter
  native-postgres (default)
  hindsight-shadow (benchmark only)
  future engine (only after same contract + deletion/isolation gate)
```

这条线不会浪费此前对 Hindsight 的研究：先实现 engine contract、outbox 和中文
benchmark，native 与 Hindsight 都可作为 adapter 同场 shadow。若 Hindsight 在
时间/关系 slice 明显胜出，再决定是否让它承接 derived recall；否则保持纯原生。

## 5. Benchmark 与发布门槛补充

TencentDB README 自报：WideSearch pass rate `33%→50%`、token `-61.38%`，连续
SWE-bench `58.4%→64.2%`、token `-33.09%`，PersonaMem `48%→76%`。当前仓库没有
同时提供这些结果对应的完整可重放数据、模型配置和 benchmark harness，所以只能
作为设计方向信号，不能与 Hindsight/Mem0 的不同数据集分数横比。

Neo Chat 本地评测必须额外覆盖：

- 中文同义、简称、项目名、日期、相对时间和更正；
- L1 atom 与 L2/L3 发生冲突时，只返回当前有效事实；
- persona/scene token budget 和无关 query 的 false injection；
- user A/B 隔离、conversation delete、memory forget、account delete；
- worker crash、provider timeout、embedding 失败、reranker 失败、重建；
- prompt injection：Memory 中的命令/代码/伪 system 标签不得改变执行权限。

## 6. 关键证据索引

Hermes：

- `agent/memory_provider.py:43-315` — provider lifecycle contract。
- `agent/memory_manager.py:364-780` — 单 external provider、timeout、FIFO background
  sync、bounded drain。
- `agent/turn_context.py:1125-1168` — turn-start recall 与 API sidecar 注入。
- `run_agent.py:3743-3800` — completed/non-interrupted turn 才 sync。
- `agent/agent_init.py:1598-1699` — built-in/external 初始化与 `skip_memory`。

TencentDB Agent Memory：

- `src/core/tdai-core.ts:143-283` — host-neutral recall/capture 主链。
- `src/config.ts:500-589` — L0/L1/L2/L3、recall、embedding 默认值。
- `src/core/hooks/auto-recall.ts:72-240` — L1 动态召回与 L2/L3 稳定注入。
- `src/core/record/l1-extractor.ts`、`l1-dedup.ts`、`l1-writer.ts` — atom 抽取、
  candidate + LLM 冲突裁决、dual-write。
- `src/core/store/sqlite.ts:232-265,557-835` — jieba FTS5、SQLite/vec schema。
- `src/utils/pipeline-manager.ts:300-433,1105-1132` — trigger、checkpoint recovery。
- `src/gateway/server.ts:371-443` — `/recall` 丢弃 `prependContext`、Gateway routes。
- `src/gateway/types.ts:32-100` + Gateway handler — `user_id` 声明但未应用。
- `hermes-plugin/memory/memory_tencentdb/__init__.py:828-1079` — Hermes recall/capture、
  daemon sync、shutdown/session-end。
