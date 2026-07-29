# Neo Chat 当前 Server Memory 实况

调研日期：2026-07-27。证据优先级为 Live runtime → 当前源码/迁移 → 项目文档。

## 1. Live 状态

从正在运行的 Server mode 实例重新请求：

```text
GET http://127.0.0.1:18080/mm-api/v1/memory-settings
{"enabled":false,"searchEnabled":true,"autoRecordEnabled":false}

GET http://127.0.0.1:18080/mm-api/v1/memories
{"items":[]}
```

结论：截图所见不是 UI 假状态。总开关关闭、回答前搜索开、回答后自动记录关，
当前 0 条 Server Memory。由于总开关关闭，搜索开关当前不会产生召回。

## 2. 数据权威与 UI

- Migration `035_user_memories` 创建 `user_memory_settings` 与 `user_memories`。
- 所有查询从认证 context 取 `user_id`；客户端不能自行指定用户。
- 删除采用 soft delete，活跃内容以 `(user_id, normalized_content)` 唯一。
- Server mode 设置页通过 Go API 执行设置和 CRUD；旧 IndexedDB store 仅保留
  Local compatibility，不得注入 Server prompt。
- UI 有总开关、回答前搜索、回答后记录、手工增删改和筛选；Server mode 已隐藏
  Local 的 Dream consolidation。

主要证据：

- `mm-chat/backend/migrations/035_user_memories.up.sql`
- `mm-chat/backend/internal/usermemory/repository_postgres.go`
- `mm-chat/frontend/src/components/settings/MemorySettings.tsx`
- `mm-chat/frontend/src/__tests__/serverMemoryAuthority.test.ts`
- `mm-chat/docs/contracts/conversation-context.md`

## 3. 写入链路

### 手工写入

`POST/PATCH/DELETE /v1/memories` → Go `usermemory.Service` → user-scoped
PostgreSQL repository。

类型固定为：

```text
fact / preference / instruction / project / warning / decision / context
```

边界：每条正文 1–2000 rune，最多 12 个 tag，importance 为 1–5。

### 自动记录

仅当 `enabled && autoRecordEnabled` 时运行：

1. 正常回答先完成并持久化。
2. `queueDurableMemoryExtraction` 启动进程内 goroutine。
3. 用本次回答的同一个 Provider/model 再请求一次，timeout 45 秒。
4. 输入只有当前 raw user message，截断到 12,000 rune；不含完整对话上下文。
5. LLM 最多返回 5 条候选；Go 再拒绝 `context`、空/超长和 credential-like
   内容。
6. 按规范化正文写入；完全相同正文冲突时 repository 走精确 upsert 语义。
7. Provider/parse/write 失败静默，不影响已经完成的回答。

优点：回答可靠性与 Memory 抽取解耦，prompt-injection 和凭证过滤已有基础。

缺口：

- goroutine 不是 durable job；进程重启会丢任务，无 retry/dead-letter/审计。
- 只看当前 user message，代词、省略和更正容易失去上下文。
- 使用回答模型做第二次调用，成本和行为不稳定，也没有独立模型版本绑定。
- 只有正文精确去重，没有语义 dedup、merge、矛盾检测和 supersession。
- 没有 confidence、valid time、expiration、scope、extraction profile 等字段。

证据：`mm-chat/backend/internal/chat/durable_memory.go:17-239`、
`mm-chat/backend/internal/usermemory/service.go:125-163`。

## 4. 读取与 Prompt 注入链路

`SearchRelevant` 的真实逻辑：

1. `enabled && searchEnabled` 才继续。
2. PostgreSQL 按更新时间取该用户最多 500 条活跃 Memory。
3. Go 对 Latin token 和中文 unigram/bigram 做规范化与 stop-word 过滤。
4. exact/substring、tag、正文 term、importance、类型共同打分。
5. 分数 `< 2.5` 丢弃；最多返回 Top-5，并更新 `last_used_at`。
6. 检索发生在 Knowledge/Web 查询构造后。
7. 结果以 JSON 放入 `<relevant-user-memory>`，明确标为低优先级、不可信历史
   声明；当前 system/user request 优先，Memory 内指令不得触发工具执行。
8. 读取失败仅记录 `read_failed` metadata，回答继续。

优点：权限边界、Top-K、降级、提示词信任级别和删除即时不可见均已正确建立。

缺口：

- 召回是 O(最多 500 行) 的 Go 内存词法扫描。
- 无 embedding、真 BM25、RRF、cross-encoder rerank。
- 同义表达、中文改写、多跳关系和时间问题召回弱。
- `last_used_at` 只表示“被注入”，不表示“帮助了答案”；没有用户反馈。
- 无 token budget、score/provenance 诊断、过期/失效过滤和冲突裁决。

证据：`mm-chat/backend/internal/usermemory/types.go:9-16`、
`mm-chat/backend/internal/usermemory/service.go:165-222,303-377`、
`mm-chat/backend/internal/chat/durable_memory.go:34-109`。

## 5. 已有验证与可复用资产

项目在 2026-07-20 已用真实 `gpt-5.6-sol` 验证过一次：自动抽取 → 新会话
召回 → 无关问题零命中 → 删除后零命中，并清理所有 fixture。记录在：

`mm-chat/docs/tracking/g11-conversation-context-memory-process.md:114-188`

Memory v2 可直接复用：

- PostgreSQL 17.10；`pgvector 0.8.5`；`pg_textsearch 1.3.1` 真 BM25。
- `Pro/BAAI/bge-m3` 1024 维 embedding 与
  `Pro/BAAI/bge-reranker-v2-m3` reranker profile。
- Python RAG worker、Redis wakeup、durable outbox/job/retry 模式。
- Go user authority、typed API、soft delete、prompt trust boundary、metadata
  degradation 模式。

因此当前不是推倒重建问题，而是把 v1 的“正确边界 + 简单算法”升级成
“正确边界 + production retrieval/temporal pipeline”。
