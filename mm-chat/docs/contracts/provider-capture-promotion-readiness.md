# Provider Capture Promotion Readiness

- 状态：reviewed/blocked；all Provider Fixtures remain `draft/blocked`
- 日期：2026-07-13
- 输入：Git-external Jina/MinerU Evidence Snapshots、官方 OpenAPI/Docs/Terms

> 本报告是 Evidence→Fixture 的 Promotion Gate，不是 Fixture 本体。成功调用只证明对应
> Request/Response Summary 在该时刻被观察到；不得反推 immutable build、Region、错误覆盖、
> Terms、SLA、Idempotency 或生产授权。

## 1. Evidence authority

| Provider | Evidence SHA-256 | 已证明 | 未证明 |
| --- | --- | --- | --- |
| Jina | `e0c1ccd82b1a3d09ac65ea37dd7e18c36e06d9cf3b57cf235f150b273436d1a8` | `/v1/embeddings` 1024/2048 与 `/v1/rerank` 成功 Shape、Model、Index、Usage、Dimension | Error Case、Rate Limit、Normalization、Build、Region、Terms/SLA |
| MinerU | `a47a34559fbd262ba29a59181fe7b3ecedc8f1652305b2f4a22afdb342d23b46` | `/api/v4/file-urls/batch` 单次 Batch Submit、Batch ID/Signed URL 存在性 | Upload、Poll、Result、Remote URL Flow、Recovery、BBox、Build、Region、Terms/SLA |

Evidence 保持在 `~/.local/share/mm-chat/provider-evidence/`，不进入 Git。映射产物只能引用
Hash、Observed Time、Operation 与 allowlisted Summary；不能复制 Token、动态 ID、Signed
URL、Vector 或原始 Provider Body。

## 2. Candidate mapping decision

```text
Jina embedding_1024 -> jina-embedding-v4-1024 public draft / embed
Jina embedding_2048 -> jina-embedding-v4-2048 public draft / embed
Jina rerank         -> jina-rerank-v3 public draft / rerank
MinerU batch submit -> NO CURRENT FIXTURE OPERATION MATCH
```

Jina 三个 Success Behavior 可形成 `redacted_capture` Candidate，但当前 Harness 只保留 Closed
Summary，不保留完整 Vector/Raw Body；在 Freeze Schema 明确允许 Summary-derived Case 前，不得把
现有 `public_schema_synthetic` Case 改名冒充实测。

MinerU 当前 Draft 描述 Remote URL Flow：`/api/v4/extract/task` 与
`/api/v4/extract/task/{task_id}`；真实 Capture 是 Local Upload Batch Flow：
`/api/v4/file-urls/batch`。二者不是同一 Wire，禁止映射。生产 Flow 必须先选定并扩展 Closed
Operation Set，再单独验证 Upload/Poll/Result/Recovery。

## 3. Terms authority review

公开来源于 `2026-07-13T01:55:02Z` 保存到 Git 外
`~/.local/share/mm-chat/provider-evidence/public-sources-20260713T015502Z`：

| Source | SHA-256 | Authority decision |
| --- | --- | --- |
| Jina OpenAPI | `8befb880d62bb774a0eed2eb12b8f20a3d34ca3c2c3918435daf4385eb7b7a7e` | API Schema `2026.06.29.1712`，不是 immutable Model Build |
| Jina Legal | `afeb2a86049578e56b35b397dee080a600ac482d55ec0a609f18346f6393548c` | 页面称当前处理由 Elastic 条款治理，旧 Jina 条款可能已不反映实践 |
| Jina Embedding v4 model page | `be106f564e87624dd117daf9057b2a0be3a59935467e61841e56df8e015e065c` | Qwen Research License 是 Model 页面声明，不等于 Hosted API License |
| Jina Reranker v3 model page | `b70b72dfce1de7d4fc4365f73b88552485af7d2c616829b35ba34147947ef6d3` | CC-BY-NC-4.0 是 Model 页面声明，不等于 Hosted API License |
| Elastic Privacy | `ca75eba0da3191087b3c174ab63ca839f082d1f1428b0519fbe87c2ecf98da48` | 通用 Privacy，不证明本账户 Region/Retention/SLA |
| Elastic Customer Agreements | `25e987f7f18110eb20df18e17cd87c619065810d0e7e2a1f548c4e37830aeb1c` | 入口页，不替代适用 Order Form |
| Elastic Customer DPA PDF | `40128737f5207233dc9fbbb470e86fa9b6c80b4924c12ff0f4394075eee5e56a` | Jina Legal 引用，但账户适用性/具体 Region 仍需确认 |
| MinerU API Docs | `6b72fd975b37f5d64996bdd97d97f755b7de82602f7e6c1f37cc27b9f51e24fa` | 只证明技术 Flow；捕获页面未发现 Hosted API 法律条款入口 |

Jina 页面中的旧条款写有“仅为提供服务所需而保存 Input/Output”、终止后删除受控副本、保留
聚合匿名运行元数据以及“不用客户请求/Input/Prompt/上传内容训练模型”；但同页明确警告这些
旧条款可能不再反映当前处理实践，且 Order Form 优先。因此 Retention、Deletion、
Training-use、License、SLA 全部保持 `unknown`，不能标为 `terms_verified`。

MinerU Docs 证明 Local Upload 链接有效期 24 小时、上传无需 `Content-Type`、上传后自动提交、
Batch Poll 路径与技术限制；它没有证明结果保留、删除、训练用途、Hosted API License、Region
或 SLA。所有法律/Governance 字段保持 `unknown`。

## 4. Field decision matrix

| Field | Jina | MinerU |
| --- | --- | --- |
| Base URL / success path | `proven` | Batch Allocate path `proven`；旧 Remote path only public docs |
| Model ID | Capture Response `proven` | Request `vlm` observed；immutable build unknown |
| Dimension / Index / Usage | 1024/2048 and Rerank `proven` | not applicable to Batch Allocate |
| Error behavior / Retry-After | OpenAPI-declared only，Capture unknown | unknown；现有 429 Case 仍 synthetic |
| Idempotency / Query-by-key / Cancel | unknown | unknown |
| Region / immutable build | unknown | unknown |
| Retention / Deletion / Training | unknown | unknown |
| Hosted API License / SLA | unknown | unknown |

## 5. MinerU production Flow decision

生产候选锁定 **Local Upload Batch Flow**，不选 Remote URL Flow。原因：原文件位于私有
MinIO/Object Gateway；Remote Flow 需要向 Provider 暴露可访问 Source URL，而 Local Batch
只需受控 outbound Upload，不暴露内部 Object URL。

Closed Operation Set 必须在后续 Schema 变更中建模为：

```text
allocate_upload  POST /api/v4/file-urls/batch
upload           PUT  provider-signed URL      # no Authorization/Content-Type
poll_batch       GET  /api/v4/extract-results/batch/{batch_id}
download_result  GET  provider result URL
cancel/query_by_key                            # remain unknown until proven
```

当前 Capture 只证明 `allocate_upload`。Signed Upload/Result Host 未保留，Host Allowlist、Poll、
Result Archive、Crash Recovery 与 `UNKNOWN_SUBMISSION` 仍阻断 Adapter。现有 MinerU Draft 的
Remote `submit/poll/result` 不能静默改名复用。

## 6. Promotion checklist

- [x] Snapshot 并 Hash 当前官方 OpenAPI/Docs/Terms 来源。
- [x] 形成逐字段 `proven / contradicted / unknown / not_applicable` 映射。
- [x] 决定 MinerU 生产使用 Local Batch，不使用 Remote URL Flow。
- [ ] 明确 Summary-derived Capture Case 的 Closed Schema，或执行允许保留安全 Wire Body 的新 Capture。
- [ ] 补齐成功之外的稳定 Error Coverage。
- [ ] 两名 Reviewer（含 `governance_security`）复核 Terms、Hash 与未决项。
- [ ] 仅在全部 Freeze Gate 通过后生成 Governance Manifest。

## 7. Rollback

本阶段默认不修改四个 Public Draft Fixture。若 Mapping/Terms 结论错误，删除本报告和
Git-external Public-source Snapshot 即可；Runtime Registry、Governance、`011/012` 与生产
Egress 均保持关闭。
