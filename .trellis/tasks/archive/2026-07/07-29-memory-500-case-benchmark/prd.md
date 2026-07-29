# Build formal 500-case Memory benchmark authoring foundation

## Goal

为现有 10-case synthetic draft 建立可独立复核、可冻结、可一次性运行 Holdout 的
500-case 中文/中英 Memory Golden authoring toolchain。本任务交付 deterministic 650-case
candidate pool 与完整人工 review/freeze 工具；实际 500 条逐条人工审核、formal freeze 和
admission 由后续独立 operational task 收口。最终证据用于 Native Memory v2 进入
shadow/canary 前的正式离线门禁，且评测与 reader promotion 必须继续分离。

## What I already know

- 当前 `memoryeval` evaluator、strict schemas、freeze hash、exclusive report 和门槛已实现，
  focused/full tests 已通过。
- 当前 checked-in Golden 只有 10 个 draft cases，未人工复核，不能通过 formal admission。
- Formal corpus 固定为 500 cases，Development/Validation/Holdout=`300/100/100`。
- 13 个 critical slices 每个至少 50 cases，并至少覆盖 `30/10/10` 三个 split；case 可多标签。
- Corpus 必须 synthetic-only、no-real-user-data、no-sensitive-data、
  `promotionEligible=false`，并绑定独立 synthetic fixture manifest SHA-256。
- 每个 case 必须逐条真实 human review；不得把机器生成 reviewer/timestamp 当作人工证据。
- Holdout UUID 必须 freeze 前预提交，Holdout 只能按冻结顺序正式执行一次。
- Hindsight 两轨 8/10 的结果不能进入本 corpus 的答案或改变 Native reader authority。

## Assumptions (temporary)

- 使用 deterministic/template-driven authoring 生成大于 500 的 synthetic candidate pool，
  人工逐条接受/修改/拒绝后再选出精确 500 条。
- 新建通用 benchmark fixture artifact，不把 formal corpus 永久耦合到
  `neo-chat.memory-hindsight-fixtures.v1`。
- 本轮先完成 corpus authoring/review/freeze toolchain 与 deterministic 650-case candidate
  pool；实际 500 条人工审核、formal freeze、admission 及 Native baseline/candidate
  observations 由后续 operational task 分阶段执行。

## Open Questions

- None。等待最终需求确认后进入实现。

## Requirements

- 保留现有 evaluator v1 门槛，不通过扩宽阈值来让 corpus admission 通过。
- 提供可重放的 deterministic candidate generation，固定 seed/profile/version，并拒绝静默漂移。
- Fixture manifest 与 Golden 分离：fixture 保存 synthetic event/fact/scope/state，Golden 只保存
  query、opaque logical IDs、expected/exclusion authority 与 review records。
- Authoring、review、freeze、observation、evaluation 五个阶段使用不同状态/命令，禁止一步自动伪造
  human review 或 Holdout evidence。
- Development/Validation 可反复调试；Holdout 内容与预期在 freeze 后不可变，且正式运行 ordinal=1。
- 所有 artifacts strict/bounded、duplicate-key/unknown-field/trailing-value fail closed，并可由 hash
  独立重放。
- 不读取 Live chat/Memory，不调用 Live Provider，不改变 reader flags/pointers。
- 魔尊本人承担全部 case-by-case human review；工具只能展示、校验并记录明确的
  accept/edit/reject，不能在没有明确人工动作时预填/批量复制 reviewer/timestamp，也不能提供
  “approve all”。Reviewer UUID 由魔尊显式输入；`reviewedAt` 只在对应逐条动作发生时取 server clock。
- Review 使用独立 Go authoring command 提供 loopback-only browser UI；不进入生产 Next.js、
  backend HTTP API 或主 Compose。服务只监听 `127.0.0.1`，使用每次启动的随机 session token、
  strict Host/Origin、no-CORS 与 `Cache-Control: no-store`。
- Review ledger 写入用户明确选择的本地 authoring 目录，使用 exclusive/atomic file 更新与 mode
  `0600`；支持中断恢复，但每条 decision 必须绑定 case ID、fixture hash 与 case content hash。
- Review ledger 使用 append-only、hash-chained immutable events；checkpoint/status 只是可重建缓存。
  进程在任意 event 边界中断后必须从完整 event 恢复，partial/tampered/gapped/forked event
  一律 fail closed，禁止静默丢弃或改写历史。
- `edit` 必须先产生新 content/fixture hash 并把 case 恢复为 draft；旧 accept/reject attestation
  保留在 audit history 但不再有效，必须由魔尊重新明确 accept/reject。任何 action 都不能同时
  暗含 edit + human review 两个步骤。
- 最终 500 cases 固定语言比例为中文 350、mixed 100、英文 50；各 split 按相同比例分层：
  Development=`210/60/30`、Validation=`70/20/10`、Holdout=`70/20/10`。
- Candidate generation 完全离线且 deterministic：固定 generator schema/version/seed、模板 catalog、
  词表与组合顺序，不调用本地或远程模型；相同输入必须生成 byte-identical oversized candidate pool。
- 初始 pool 固定为 650 条，provisional split=`390/130/130`、语言=`455/130/65`，每个
  split 内保持 70/20/10 语言比例；每个 critical slice 至少 `65` 且至少跨 split `39/13/13`，
  为最终 `500`、`300/100/100`、`350/100/50`、slice `50`/`30/10/10` 留出 30% 筛选余量。
  先通过结构、重复、slice/language/split coverage diagnostics，再由真人逐条
  review；最终选择精确 500 条，不能为了凑数自动接受 rejected candidates。
- Formal working set 默认位于受保护且 gitignored 的
  `mm-chat/data/memory-benchmark/v1/`，包含 fixture、Golden draft、append-only review ledger、
  checkpoints 与 sealed Holdout；source tree 只保留 generator/templates/schema/tests、content-free
  hashes/status/report，不保留 query、expected IDs 或 Holdout 内容。
- Authoring command 必须 fail closed 拒绝把 formal artifacts 输出到 Git tracked path、`secrets/`、
  `backup/` 或任意 symlink/目录逃逸；tests 只使用临时目录，不读取或覆写现有 `mm-chat/data/`。
- 本任务的交付边界固定为 source-controlled authoring toolchain、schemas、tests、operator docs，
  以及 off-repo deterministic 650-case candidate pool；不得把未完成的人工审核伪装为 formal
  500-case Golden。实际 review/freeze/admission 必须另建 operational task，并以本任务产生的
  immutable generator/profile/hash 为输入。
- Freeze 前要求 650 candidates 全部已有当前 content hash 对应的明确 accept/reject，且恰好
  500 accepted 并满足精确 split/language/slice/semantic gates；freeze 不得自动补齐、重分配、
  改写或选择 cases。
- Freeze 原子写入 fixture/Golden/freeze manifest，并预提交 Holdout UUID、ordered case IDs、
  fixture/content hashes；成功后 review UI 不再返回 Holdout query、fixture 或 expectations。
- Holdout official-use state machine 固定为 `sealed -> consumed`。后续命令必须在暴露任何 Holdout
  输入前以 exclusive marker 绑定预提交 UUID、ordinal=1 与 frozen hashes；marker 一旦创建，
  即使执行崩溃也视为 Holdout 已消费/污染，不允许 retry 或 rollback，只能创建新 corpus version
  重新 review/freeze。该约束防止把失败运行变成调参机会。
- Read-once 是 authoring/evaluation toolchain 的 fail-closed operational contract，而非声称能阻止
  本机文件所有者绕过工具直接复制 bytes；原始工作集仍依赖 `0600/0700` 权限与受保护目录。

## Acceptance Criteria — current authoring foundation

- [x] 固定 profile 可在干净环境生成 byte-identical 的 650-case synthetic candidate pool，且
      diagnostics 证明存在筛选出精确 `300/100/100`、语言 `350/100/50`、13 slices
      `>=50` 且各 split `>=30/10/10` 的可行余量。
- [x] Fixture/Golden/content/review/freeze/Holdout bindings 可独立验证，任一字节漂移 fail closed。
- [x] Git/status/standalone copy 与 committed reports 中不出现 formal query、fixture content、expected
      Memory IDs、review ledger 或 Holdout 内容；只允许 schema/version/count/hash/status。
- [x] Secret/untrusted/cross-user/scope/deletion/temporal/negative/fallback cases 使用 synthetic sentinels，
      不包含真实用户数据或真实 credential。
- [x] Test-only temporary corpus 可贯通 generate → explicit per-case review → edit/re-review → freeze →
      `ValidateGoldenAdmission`；draft、机器伪造 review、错误 split、coverage、hash 或 Holdout binding
      均被拒绝，测试证据不得冒充真人 formal review。
- [x] Review workflow 不允许批量自动盖章，且记录真实 reviewer UUID 与 RFC3339 reviewedAt。
- [x] Review 可中断恢复，每次 decision 绑定 immutable case ID/content hash，修改后旧 decision
      自动失效并要求重新审核。
- [x] Ledger 的 concurrent write、kill/restart、partial event、hash-chain gap/fork、stale checkpoint
      测试通过；只允许一个 writer，损坏状态必须拒绝继续而非猜测恢复。
- [x] Freeze 后 review UI 不再暴露 Holdout 内容；official Holdout marker 在内容暴露前 exclusive
      创建，第二次运行或首次运行崩溃后的 retry 均被拒绝。
- [x] Focused race、全 backend tests、vet、artifact replay 和 full standalone gate 通过。

## Downstream Operational Exit Criteria (follow-up task)

- [ ] 魔尊逐条完成精确 500 个 synthetic cases 的真实 review，最终 split 为 `300/100/100`。
- [ ] 每个 critical slice 满足总数 `>=50` 和 `30/10/10` split 最低覆盖，并通过语义复核而非只贴标签。
- [ ] 最终 frozen corpus 通过 `ValidateGoldenAdmission`，且每条 review、修改、fixture/content/freeze
      hash 均可审计。
- [ ] Holdout 按预提交 UUID 与冻结顺序仅正式执行 ordinal=1；完成前不触发任何 reader promotion。

## Definition of Done

- Authoring generator/schema/review UI/freeze enforcement、focused tests 与 operator docs 已交付。
- 650-case candidate pool 仅生成到受保护 off-repo working set，其 content-free profile/hash/status
  可复核，且不会污染 Git/standalone artifacts。
- 文档说明 authoring、review、freeze、一次性 Holdout、rollback/restart 及后续 operational task 流程。
- 未宣称已完成 500 条 human review/formal admission，也未启动 shadow、canary、production reader
  或 Hindsight trial。

## Verification Evidence — 2026-07-29

- `go test -race ./internal/memoryauthor ./cmd/memory-benchmark-author ./internal/memoryeval ./cmd/memory-eval`：passed。
- `go test ./...` 与 `go vet ./...`（`mm-chat/backend/`）：passed。
- `go run ./cmd/memory-benchmark-author verify` 与 committed content-free status byte-identical；status SHA-256 为 `1e69314f8bfd1053af9627a3b4be6ec3ec2fd9b0a2ddf30889f194984dddb517`。
- `bash mm-chat/scripts/verify-standalone.sh --full`：Frontend 198 files / 961 tests、Backend passed、RAG 1906 passed / 7 skipped、standalone verification passed。
- `verify-module`、`verify-security`、`verify-quality`、`verify-change`：passed；Critical/High=0，quality warnings=0。
- Protected candidate pool remains Git-ignored and untracked with `0700` directories / `0600` files; immutable candidate-manifest SHA-256 is `bfca1b829ab4f886c558cad1be3c5f1e7c218492621405ac6d9fd8033113559c`.

## Technical Approach

### Source layout

- 新增 `mm-chat/backend/internal/memoryauthor/`：fixture/candidate/ledger/freeze schemas，strict codec，
  deterministic generator，coverage/duplicate diagnostics，event replay，freeze 与 Holdout state machine，
  protected-path/permission checks。该 package 不访问 DB、Provider、Live Memory 或 production flags。
- 新增 `mm-chat/backend/cmd/memory-benchmark-author/`：单一 operator command，提供
  `generate`、`review`、`status`、`freeze`、`verify` 与后续 operational task 使用的
  `holdout-begin` 状态转换；使用 Go standard library，不引入长期服务或生产路由。
- Embedded review UI 只服务于 `review` 子命令；只监听 `127.0.0.1` 随机端口。启动时生成
  cryptographically random bootstrap/session token，建立 `HttpOnly`/`SameSite=Strict` session；
  state-changing request 同时校验 method、Host、Origin、session 与 CSRF token，禁 CORS、禁缓存、
  禁 framing，并限制 body/字段大小。

### Deterministic artifacts

- Versioned profile 固定 schema、seed、template catalog、词表、组合顺序与 case/fixture ID derivation。
  canonical JSON 使用固定 field/order/line-ending；生成器只允许创建空的新 profile directory，
  相同 profile 重放必须 byte-identical，既有 artifact 不覆盖。
- Generic fixture schema 使用 `neo-chat.memory-benchmark-fixtures.v1`，保存 synthetic facts/events、
  trust/scope/temporal/deletion state 与 opaque aliases；Golden 继续使用 evaluator v1，只保存 query、
  logical IDs、authority/exclusion/review fields。
- Diagnostics 同时检查 exact/normalized duplicates、ID/reference integrity、slice semantic evidence、
  provisional split/language/slice coverage 与最终配额可行性；label-only coverage 不能过关。

### Review, resume, and freeze

- Reviewer UUID 必须由魔尊显式提供并确认；只有逐条 POST accept/reject 才能创建 attestation，
  `reviewedAt` 取该明确动作发生时的 server clock，不预填 machine decisions，不提供 approve-all。
- 每个 ledger event 使用 monotonically increasing sequence、previous-event hash、case ID、fixture hash、
  before/after content hash、action、reviewer 与 timestamp；event 通过 temporary file + fsync + exclusive
  rename/link 发布为 mode `0600`。Derived checkpoint 可原子替换，但恢复必须以 ledger replay 为 authority。
- Edit、decision supersession、resume 都追加 event，不修改旧 event。Freeze 只消费全量 replay 后的
  current state，生成新 immutable artifacts，并调用现有 `memoryeval.GoldenContentSHA256` 与
  `ValidateGoldenAdmission`，不复制 evaluator 门槛实现。

### Protected Holdout and filesystem boundary

- Formal root 默认 `mm-chat/data/memory-benchmark/v1/`，directories/files 分别为 `0700/0600`。
  CLI canonicalize 每个 path component，拒绝 symlink、Git tracked/source path、`secrets/`、`backup/`、
  非空目标与目录逃逸。Tests 直接调用 package API 使用临时目录，不提供可误用的 production
  `--allow-unsafe-root` 开关。
- Freeze manifest 预提交 Holdout UUID、order 与 hashes；freeze 后 UI/API 只返回 content-free
  Holdout counts/status。`holdout-begin` 先 exclusive 写入 consumed marker 再允许后续 producer
  获取 bounded input；marker/hash/UUID/ordinal 任一漂移均拒绝。

### Verification and rollback

- Focused tests覆盖 deterministic golden files、650 quotas、strict decode、tamper/path traversal、
  loopback HTTP security、ledger replay/concurrency/crash、edit invalidation、exact freeze admission 与
  read-once Holdout。随后运行 race、全 backend tests/vet 与 `verify-standalone.sh --full`。
- Source rollback 只删除/回退新增 source；runtime profile 不自动删除或覆写。候选生成失败留在
  staging 目录并拒绝发布；已发布 profile/ledger/freeze 只通过新 version 重建，不原地修补。

## Decision (ADR-lite)

**Context**：正式 500-case corpus 既要 deterministic 可重放，又必须由真人逐条建立 review authority；
review 会跨多天，Holdout 只能正式运行一次，且正式内容不能进入 Git 或 production surface。

**Decision**：采用“versioned deterministic 650-case pool + loopback human review + append-only
hash-chained ledger + exact freeze + pre-exposure consumed marker”的两阶段方案。本任务交付全部
authoring/fail-closed 能力，后续 operational task 才执行真实 500 条 review、freeze 与 observations。

**Consequences**：实现量高于简单 JSON generator，但能证明每条 case 的来源、修改和 review，支持
安全断点恢复并阻断 stale attestation/Holdout retry。工具不能对抗拥有本机文件权限的恶意所有者，
因此 read-once 的安全边界被明确限定为 toolchain + filesystem permissions，而不虚构 DRM 保证。

## Implementation Plan (small PRs)

1. **PR1 — schemas + deterministic generator**：建立 `memoryauthor` strict models/codecs、650 profile、
   generic fixture/Golden generation、coverage/duplicate diagnostics、protected-path publishing 与 tests。
2. **PR2 — append-only review engine**：实现 event ledger/replay/checkpoint、accept/edit/reject/status、
   stale-review invalidation、concurrency/crash/tamper tests。
3. **PR3 — loopback review command/UI**：实现 CLI 与 embedded browser UI、安全 headers/session/CSRF、
   keyboard flow、断点续审及 HTTP tests。
4. **PR4 — freeze + Holdout sealing**：实现 exact 500 gates、atomic immutable freeze、现有 evaluator
   admission 复用、sealed/consumed state machine 与 failure-path tests。
5. **PR5 — operations closure**：更新 contract/spec/README，生成 off-repo 650 pool 与 content-free status，
   跑 focused race、全 backend/vet、artifact replay、full standalone gate；记录后续 review/freeze task
   的固定输入与边界。

## Out of Scope

- 不在本任务中自动 promotion L1/L2/L3 reader。
- 不使用 Live chat/Memory 生成、去标识或验证 cases。
- 不把模型输出、assistant 自审或批量复制 timestamp 冒充 human review。
- 不重跑或恢复 Hindsight 实例。
- 不在 corpus 冻结前执行正式 Holdout。
- 不新增生产 review route、数据库表、frontend bundle、长期 review service 或网络监听地址配置。
- 不在本任务内等待魔尊跨多天完成 500 条人工审核，也不生成最终 formal freeze/admission 结论；
  这些由独立 review/freeze operational task 收口。

## Technical Notes

- Applicable spec: `.trellis/spec/backend/memory-v2-benchmark.md`。
- Operator contract: `mm-chat/docs/contracts/memory-benchmark-workflow.md`。
- Existing evaluator: `mm-chat/backend/internal/memoryeval/` 与
  `mm-chat/backend/cmd/memory-eval/`。
- Existing draft: `mm-chat/docs/contracts/memory-benchmark-golden-draft-template.json`。
- Local audit: [`research/formal-benchmark-authoring-audit.md`](research/formal-benchmark-authoring-audit.md)。

## Research References

- [`research/formal-benchmark-authoring-audit.md`](research/formal-benchmark-authoring-audit.md) —
  当前 artifact gap、authoring/review 方案与推荐路径。

## Confirmed Decisions

1. **真人逐条审核（2026-07-29，魔尊选择 A/1）**：魔尊本人逐条接受、修改或拒绝；允许
   deterministic generator 生成候选，但禁止 assistant/model 自审、无人工动作的 reviewer/timestamp、
   批量 approve 或把 draft 宣称为 formal evidence；review timestamp 只绑定逐条显式操作。
2. **Loopback browser review UI（2026-07-29，魔尊选择 1）**：使用独立本地 Go command
   服务嵌入式 review UI，支持键盘操作、逐条修改和断点续审；不接生产 frontend/API/Compose，
   不允许监听 LAN/all-interface。
3. **中文主导语言分层（2026-07-29，魔尊选择 1）**：最终 corpus 为中文 70%、中英混合
   20%、英文 10%，并在 Development/Validation/Holdout 中保持同一分层比例，避免 Holdout
   语言分布漂移。
4. **纯离线 deterministic generation（2026-07-29，魔尊选择 1）**：只用 versioned templates、
   固定 seed/词表/组合顺序生成候选，不调用任何 Provider 或模型 paraphrase；语言自然度与重复度
   由生成 diagnostics 和真人逐条审核兜底。
5. **Off-repo protected corpus（2026-07-29，魔尊选择 1）**：正式 fixture/Golden/review/Holdout
   存入 `mm-chat/data/memory-benchmark/v1/`，Git 只保留可复现生成逻辑、结构契约、content-free
   hash/status 与报告；禁止把 Holdout 明文提交到 repository。
6. **两阶段任务收口（2026-07-29，魔尊选择 1）**：当前任务交付完整 authoring toolchain 与
   deterministic 650-case pool 后即可归档；魔尊跨多天完成的 500 条逐条 review、formal freeze、
   admission 与后续 observations 另建 operational task，禁止把候选池交付误报成 formal corpus 完成。
7. **完整 fail-closed authoring foundation（2026-07-29，魔尊选择 1）**：本轮同时实现并测试
   crash-safe resume、edit 后 stale review invalidation 与 Holdout sealed/read-once enforcement；
   只把实际跨多天的人工操作后置，不把安全状态机债务推给 operational task。
8. **最终方案确认（2026-07-29）**：魔尊确认以上 Requirements、Acceptance Criteria、ADR-lite
   与五段实施计划，允许进入实现。
