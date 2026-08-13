# Agent Runtime Epic Phase 0

## Goal

为 Neo Chat 建立可达到 Hermes Agent 级完整能力、但不复制其运行时的
Neo Agent Runtime 设计基线。Phase 0 只交付架构、威胁模型、机器可检查契约、
验收套件入口和后续 Epic 分期，不启用生产 Agent、Sandbox、Cron 或 Skill
执行，也不提前删除当前纯文本 Skills。

## What I Already Know

- Assistant、Skill、Tool 是三个独立产品概念：Assistant 是模型/系统提示词预设，
  Skill 是可安装能力包，Tool 是受 Capability Grant 约束的可调用能力。
- 目标运行时是 Go Durable Orchestrator 加宿主机非 root `neo-runnerd`；每个 Run
  使用独立 Rootless OCI Sandbox。
- Skill 包兼容 `agentskills.io`：根目录必须有 `SKILL.md`，允许
  `scripts/`、`references/`、`assets/`，并增加独立 Neo Runtime Manifest。
- 安装源包括 LobeHub、Git、ZIP 和官方仓库。任何来源都先进入不可变候选，完成
  fingerprint、SBOM、人工准入后才能安装/执行。
- `allowed-tools` 仅是上游包声明，不是授权权威。执行权限只来自服务端冻结的
  Capability Grant；Egress 和 Secret 通过独立 policy/Secret Broker 收口。
- 外部副作用使用 Prepare/Commit 协议；Project Workspace、Scratch、Artifact
  三层存储具有不同权限和生命周期。
- Skill 自学习只能生成 Draft，必须验证并人工 Promote，运行中的 Skill 不得自改。
- Cron 固化创建时的授权、模型、预算和 runtime bundle；后续权限扩大不能自动
  扩大旧 Cron。
- 子 Agent 最大深度永久为 1。子 Agent 的 Tool Registry 构建阶段必须物理移除
  `delegate_task`，不能只靠 prompt 或运行时 depth 判断。
- Kill Switch 分层覆盖全局、调度、Runtime、Skill、Tool/Capability、Egress、
  Secret 和单 Run。
- 旧纯文本 Skills 在新 Runtime 正式切换时硬删除：不迁移、不包装；清除安装、
  选择、执行状态和旧引用。历史消息只保留只读“旧版技能已退役”事实标签。
- 当前浏览器仍持有 `installedSkills`、`customSkills`、`activeSkillIds`、
  `skillAutoSelect`，Conversation/Workspace 仍可引用 `activeSkills`，Chat 仍在浏览器
  拼接 `skillContext`。这些都是未来 cutover inventory，不是 Phase 0 删除目标。
- 当前 `/v1/code/executions` 保持 `CODE_EXECUTION_UNAVAILABLE` fail-closed；
  Phase 0 不得绕过该边界。
- 当前 WSL2 主机具备 cgroup v2、user namespace 配额和 subuid/subgid，但未安装
  Podman/crun/runc/rootlesskit/slirp4netns/pasta/fuse-overlayfs/bwrap。部署必须执行
  capability probe 并在缺项时 fail closed，不能用当前 Docker 可用性替代证明。

## Locked Requirements

### Architecture

- 用 C4 说明 Context/Container/Component 边界，用 ArchiMate 贯通业务授权、
  Application orchestration 和 Technology isolation。
- Go/PostgreSQL 是 durable Run、Step、Attempt、approval、event 和 lease authority；
  Redis 若被使用，只能作为非权威 wake/cancel hint。
- `neo-runnerd` 只接受经过双向认证、版本协商和 replay fence 的内部 RPC；它不得
  获得浏览器 bearer、PostgreSQL、对象存储、Provider vault 或 Docker socket。
- 每个 Run 冻结 Skill package、runtime bundle、model、budget、Capability Grant、
  Egress policy、Secret refs、workspace snapshot 和父子 lineage。

### Durable Execution

- 定义 Run / Step / Attempt 状态机、合法转换、terminal precedence、lease reclaim、
  idempotency、cancel/kill、heartbeat、recovery 和 `outcome_unknown`。
- Tool/外部副作用默认不可重试。可变操作必须先 Prepare，返回 bounded intent，
  在新鲜授权与审批后 Commit；Commit key 必须可幂等重放。无法证明结果时进入
  `outcome_unknown`，禁止自动再次 Commit。

### Skill Supply Chain

- `SKILL.md` 遵守 Agent Skills 标准；Neo Manifest 用 JSON Schema 严格定义 runtime、
  entrypoints、capability requests、egress requests、secret slots、resource profile、
  package limits 和 provenance binding。
- 安装流水线必须阻止路径穿越、symlink/hardlink escape、special files、解压炸弹、
  浮动依赖、未固定 Git ref、未知 executable、明文 secret 和 manifest/包 hash 漂移。
- Package 和 Runtime Bundle 都是 content-addressed immutable artifact，并有独立
  fingerprint/SBOM/人工 admission；执行只能引用 admitted fingerprint。

### Authorization and Isolation

- Capability Grant 是 allowlist，绑定 user/project/assistant/run、package fingerprint、
  tool/action/resource selector、approval class、budget 和 expiry；任何缺失或漂移均拒绝。
- Egress 分 `none`、`allowlist`、`brokered`，禁止裸 IP、metadata/link-local、DNS rebinding、
  origin-changing credential forwarding；Secret 只由 Broker 按 action scope 注入短期
  handle，不写入 env、prompt、workspace、artifact、event 或日志。
- Sandbox 必须 non-root、rootless user namespace、read-only immutable rootfs、
  `no_new_privs`、capabilities empty、seccomp、cgroup v2、PID/memory/CPU/time/output limits、
  无 host PID/IPC/network、无 host project bind、无 Docker socket。
- Project Workspace 通过受控 snapshot/patch 写入；Scratch per Run 自动销毁；Artifact
  经扫描、配额、owner binding 后由 Backend 存储。Sandbox 不能直连对象存储。

### Delegation, Cron and Learning

- 根 Agent 可创建一层 Child Run；Child Run 必须 `depth=1` 且 Tool Registry 中不存在
  `delegate_task`、Cron 管理、grant 管理、secret 管理和 runtime 管理能力。
- Child 的 grant、budget、model、skills、egress、secret scopes 都只能是 Parent
  frozen snapshot 的交集/子集；禁止 Child 自扩权或启动孙 Agent。
- Cron 创建时冻结 owner、prompt/input、model、budget、Skill/runtime fingerprints、
  grants、egress、secret refs 和 schedule/timezone；每次触发仍需重新验证当前 revoke、
  Kill Switch 和 expiry。
- Learning 只能输出 quarantined Draft package + evidence；自动验证不得自动 Promote，
  Promote 必须人工、生成新 fingerprint，并且不会改变已存在 Run/Cron snapshot。

### Cutover and Operations

- Phase 0 只记录旧 Skills 删除 inventory、切换条件、备份、rollback 和事实标签；不改
  当前前端 persistence version，不删除历史消息字段，不改变生产 Chat 行为。
- 定义分层 Kill Switch、审计/指标红线、备份/恢复、前向修复原则和旧 Runtime
  回滚窗口。Runtime 开关关闭时，cleanup/retention/reconciliation 仍要运行。
- Isolation Acceptance Suite 必须覆盖 capability probe、container contract、escape
  negatives、network/secret/filesystem isolation、resource exhaustion、kill/reap、
  rootless proof、depth-1 proof 和 Prepare/Commit crash matrix。

## Deliverables

- `mm-chat/docs/architecture/agent-runtime.md`
- `mm-chat/docs/contracts/agent-runtime.md`
- `mm-chat/docs/deployment/agent-runtime.md`
- `mm-chat/docs/tracking/g20-agent-runtime-plan.md`
- `mm-chat/docs/contracts/schemas/neo-skill-runtime-manifest.schema.json`
- `mm-chat/docs/contracts/schemas/neo-capability-grant.schema.json`
- `mm-chat/docs/contracts/schemas/neo-runner-rpc.schema.json`
- `mm-chat/docs/contracts/schemas/neo-run-event.schema.json`
- 每个 schema 至少一份 valid fixture 和一份 invalid fixture。
- `mm-chat/scripts/verify-agent-runtime-phase0.sh`：离线、只读生产状态、验证 schema
  结构/fixtures/交叉约束/文档锚点/Phase 0 fail-closed 边界。
- 文档索引、Trellis backend/operations specs 和总进度同步。

## Acceptance Criteria

- [ ] 四个 JSON Schema 均为有效 JSON Schema Draft 2020-12，`additionalProperties`
      默认拒绝未知字段，valid fixtures 通过、invalid fixtures 失败。
- [ ] Manifest、Grant、Runner RPC、Run Event 通过 stable ID/fingerprint/sequence/
      idempotency 字段相互关联，不依赖自然语言猜测。
- [ ] 状态机列出全部合法转换与 terminal precedence；reclaim 不会让旧 Attempt Commit。
- [ ] Child depth-1 同时在 schema、registry construction、RPC admission 和 verifier
      四层收口；invalid fixture 证明 `delegate_task` 无法进入 Child registry。
- [ ] Prepare/Commit crash matrix 覆盖 before prepare、after prepare、before commit ack、
      after side effect/before ack 和 cancel race，并明确 `outcome_unknown`。
- [ ] Rootless OCI 部署前置和 fail-closed capability probe 可执行；不得把 Docker daemon
      可用视为 Rootless OCI acceptance。
- [ ] 旧 Skills cutover 明确“切换时硬删除、历史只留退役事实标签”，且 Phase 0 verifier
      证明当前 runtime 行为没有被提前启用或删除。
- [ ] `bash mm-chat/scripts/verify-agent-runtime-phase0.sh` 通过。
- [ ] 文档引用、shell syntax、JSON 格式和敏感信息扫描通过。

## Definition of Done

- Phase 0 交付物和研究证据齐全。
- Trellis task 进入 `in_progress` 后完成变更；相关 backend/operations specs 已更新。
- 所有适用验证通过，并明确未运行/不适用的生产 Runtime 测试。
- 变更以单一、聚焦的 `docs:` commit 提交；不 push、不 amend。

## Out of Scope

- 实现 Go orchestrator、PostgreSQL migrations、`neo-runnerd` binary 或 OCI image。
- 安装/选择某个 rootless runtime，修改主机 sysctl/subuid/subgid 或启动生产 sandbox。
- 实现 Skill Store UI、Skill 安装 API、Cron UI、自学习 UI、Child Agent UI。
- 删除旧纯文本 Skills、升级前端 persistence、迁移/包装旧 Skill 内容。
- 启用 `/v1/code/executions` 或改动现有 MCP/Assistant Runtime 行为。
- 真实外部 Provider、LobeHub、Git 或 Registry 调用。

## Research References

- [`research/hermes-agent-runtime.md`](research/hermes-agent-runtime.md)
- [`research/agentskills-standard.md`](research/agentskills-standard.md)
- [`research/neo-runtime-fit.md`](research/neo-runtime-fit.md)

## Technical Notes

- Hermes evidence is pinned to commit
  `ae56c97c6063a87250e74eccfb5697dd303bf930`, version `0.20.0`, MIT.
- Agent Skills evidence was captured from `https://agentskills.io/specification.md`.
- Runtime evidence order follows live host/runtime facts over source comments.
- User completed the design-tree grill and confirmed this document as the shared baseline.
