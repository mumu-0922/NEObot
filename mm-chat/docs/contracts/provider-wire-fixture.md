# Provider Wire Fixture Contract

- 状态：C0 intake implemented and reviewed；Remote/Local MinerU and Jina contracts remain `draft`
- 日期：2026-07-13
- Schema：
  `rag/tests/fixtures/provider_contracts/provider-contract-v1.schema.json`
- 适用：MinerU Remote URL/Local Upload Batch Parse、Jina Passage Embedding、Jina Rerank

> 本 Contract 只允许脱敏公开证据或脱敏实测结果进入 Git。Fixture 通过 Schema 不等于
> Provider 获准调用；只有 `lifecycle.state=frozen` 且 Freeze Gate 全绿，才能派生
> Governance Manifest。Credential Availability、`draft`/`verified` Fixture 和 Fake Provider
> 均不能授权 Egress。

## 1. Authority chain

```text
official docs/OpenAPI + authorized redacted capture + reviewed terms
  -> closed fixture schema + semantic/secret/integrity validation
  -> frozen Provider Wire Contract
  -> reviewed Governance Manifest/Profile
  -> Collection/User Consent
  -> pre-call and pre-commit reauthorization
```

逆向推导禁止：现有 API Key、Governance Row、Consent Row 或成功 HTTP 调用都不能证明
Wire Contract 已冻结。

## 2. Lifecycle

| State      | 含义                                              | 可生成 Governance |
| ---------- | ------------------------------------------------- | ----------------- |
| `draft`    | 公开资料已录入，仍有 unknown/blocked 字段         | 否                |
| `verified` | 脱敏实测已通过，但条款、Reviewer 或 Hash 尚未闭合 | 否                |
| `frozen`   | 所有 Gate、Evidence、Reviewer、Hash 完整          | 是                |
| `retired`  | 上游漂移或条款变化，不得新建调用                  | 否                |

未知事实必须使用 `{state:"unknown", reasonCode}`，禁止用 `TBD`、`default`、
`model-v1`、`v1`、`change-me`、`<value>` 等字符串伪装。若 Vendor 的真实不可变值恰好
匹配占位模式，必须带 Evidence 并设置显式 `literalOverride=true`。

## 3. Envelope

顶层 Closed Shape：

```text
schemaVersion / fixtureSetId / fixtureKind / providerKind / observedAt
lifecycle
identity
capabilities
governanceTerms
operations[]
redactionPolicy
evidence[]
unresolved[]
integrity?                 # frozen 时必填
```

每个 `identity/capabilities/governanceTerms` Fact 只能是：

```text
observed(value + evidenceRefs)
terms_verified(value + evidenceRefs + reviewedAt)
unknown(reasonCode)
not_applicable(reasonCode)
```

Provider Identity 至少包含 Processor、Endpoint ID、Base URL、Region、Model ID、Model API
Version、Immutable Build ID、Purpose 和 Allowed Data Types。

Observed Capability/Term Value 不是自由 JSON：Loader 固定 Closed Shape。Idempotency、
Rate Limit、Spatial、Embedding 使用各自具名字段；License、Retention、Deletion、
Training-use、SLA 分别固定 Identifier/Boolean/Duration/Scope/Availability 等类型。MIME
必须匹配正式 `type/subtype` Pattern，`{"status":"reviewed"}`、`{"x":"y"}`、`["/"]`
均不能冻结。Capability 还必须匹配 Provider：MinerU 的 Rate/Spatial 与 Jina 的
Rate/Embedding Shape 不可互换；Embedding Dimension 必须等于 Wire Request Dimension。

## 4. Wire operations

Operation 固定：

```text
operationId / phase / support
method / pathTemplate / request          # observed 时必填
responseCases[]
```

MinerU Remote URL 必须逐项声明
`submit/poll/result/cancel/query_by_key`；Local Upload Batch 必须逐项声明
`allocate_upload/upload/poll_batch/download_result/cancel/query_by_key`。两种 Flow 使用独立
Provider Kind，禁止交叉改名或复用 Wire。未公开/未 Capture 能力使用
`support.state=unknown`，不得携带猜测的 Method、Path、Request 或 Response。Jina Passage
使用 `embed`，Rerank 使用 `rerank`。所有静态 Path 禁止 Scheme、Query、Fragment 和 `..`。

Response Case 标记来源：

- `public_schema_synthetic`：根据官方字段构造，不冒充真实抓包；
- `redacted_capture`：授权实测后脱敏；
- `synthetic_test`：故障注入。

Evidence metadata 额外允许 `redacted_capture_summary`，用于引用 Git-external 的成功状态、
计数、Hash 与 Presence 摘要。该类型不得作为 Response Case Source，也不能单独支撑
`support.state=observed`；Unknown/Unsupported Support 若携带 `evidenceRefs`，引用必须存在且
仍不得携带 Method、Path、Request 或 Response Case。Summary metadata 强制携带
`sourceVersion=mm-chat.provider-capture-evidence.v2` 与 `contentHash`；Freeze 时必须对精确 Bytes
重新执行 v2 Producer Schema、Observed Time 与 Canonical JSON 校验，改标为完整
`redacted_capture` 必须失败。Local Batch 在专用 Upload/Poll/Download Fixture Schema 落地前，
除 Allocate 外的 Operation 一律禁止进入 Observed。

非 Success Case 必须映射稳定 Upper Snake Error Code，不保留原始 Provider Error、Header
或正文。

Classification 与 HTTP Status 强绑定：Success/Malformed Success 只能是 2xx；Retryable
只能是 408/425/429/5xx；Permanent 只能是其余 4xx。Frozen Provider Behavior Coverage
只计 `redacted_capture` Case；公开 Schema 构造与故障注入都不能代替实测。
`redacted_capture` Case 必须引用同类 Evidence，不能只改 Source Label 冒充实测。

## 5. Redaction and secret gate

- `Authorization`、API Key Header、Cookie、`Set-Cookie` 只记录名称，不记录值、Hash、
  长度或前后缀；
- URL 必须 HTTPS 且无 Userinfo、Query、Fragment；Signed Upload/Result URL 不入 Fixture；
- 不记录真实 Task/Batch/Trace ID、Object Key、Bucket、用户文件正文或 Query/Evidence；
- 记录 Body 必须在 `maximumRecordedBodyBytes` 内；
- `request.maximumBytes` 与 `maximumRecordedBodyBytes` 都只是本地 Fixture
  Replay/Recording Cap，不代表 Provider 公布的请求或账户限额；
- JSON 使用 UTF-8、禁止 NUL、Duplicate Key、NaN 和 Infinity；
- Placeholder、Private Key、Bearer/AWS Token-like Value 令校验失败。

## 6. Provider-specific freeze gates

### MinerU

- Remote URL 与 Local Upload Batch 必须独立冻结，不能用一方 Capture 证明另一方；
- Local Batch 的 Allocate、Signed Upload、Batch Poll、Result Download 必须逐段 Capture，
  动态 Signed Host/TTL/Redirect/Recovery Policy 必须另行进入 allowlist 和 Freeze Review；
- 精确 Endpoint/Model/API/Build；
- Submit/Poll/Result 状态与结果 Archive Schema；
- Poll/Result 按 `pending/running/done/failed` 使用独立 Closed Variant；Running 固定
  `err_msg` 与 `extract_progress.start_time`，Done 固定 `err_msg/full_zip_url`，Failed
  固定非空 `err_msg`；
- Cancel、Idempotency、Query-by-key 的 observed/unsupported 结论；
- Signed URL TTL、Compressed/Expanded Bytes、Entry Count；
- Page Index、BBox Order/Unit/Origin/Rotation/Bounds 与 Canonical Conversion Version；
- Rate Limit/`Retry-After`、Retention、Deletion、Training-use、License、SLA。

Submit 响应不确定且 Query-by-key 未证实前，Runtime 只能进入
`UNKNOWN_SUBMISSION`，禁止自动重提。

### Jina Passage Embedding

- `model=jina-embeddings-v4`、`task=retrieval.passage`；
- 1024 与 2048 是两份独立 Candidate Fixture；
- Float Response、Normalization、Batch Items/Tokens/Bytes、Timeout 和 Account Tier；
- Response Model、完整 Index Set、有限值、精确 Dimension、Usage Shape；
- Immutable Build、Region、Retention、Deletion、Training-use、License、SLA。

公开 API Schema Version 不能冒充 Immutable Model Build。当前 Draft 的 1024/2048
Response 均保存完整长度的 Synthetic Vector，不能用缩短数组通过 Dimension Test。

### Jina Rerank

`010` Base Profile 已要求精确 Rerank Identity，因此 Phase 15.2C 即使尚未调用 Rerank，也
必须选择并冻结精确 Model/API/Build/Terms，禁止使用 Placeholder 创建 Profile Bundle。
当前 `jina-reranker-v3` 仅是 Request Candidate；`identity.modelId` 保持 Unknown，公开
Synthetic Response 按官方 Required Shape 回显相同 Candidate Model，但不反向证明
Governance 已选择该 Model。Frozen 时 Identity/Request/Response 三者必须精确一致。

## 7. Spatial contract

Wire 与 Canonical 必须分离：

```text
wire: pageIndexBasis, bboxOrder, coordinateUnit, origin,
      axisDirection, bounds, rotationSemantics, pageDimensionsPath
canonical: pageIndexBasis=0, bboxOrder=xyxy, origin=top_left,
           normalizationVersion
```

每个转换 Fixture 验证 Page Width/Height、Rotation、非零面积、边界、Source Text Hash 和
Round-trip。现有 migration `010` 只约束 BBox Shape；完整 Basis/Conversion Hash 属于
forward migration `012`。

## 8. Integrity and freeze

三个 Hash 均使用 RFC 8785/JCS Canonical Bytes + SHA-256，并有独立固定边界：

- `wireContractHash`：Schema/Provider/ObservedAt、Identity、Capabilities、Operations、
  Redaction Policy；
- `termsSnapshotHash`：Schema/Provider/ObservedAt、Governance Terms，以及这些 Terms
  精确引用的 Evidence；
- `fixtureSetHash`：Schema/Fixture Identity/ObservedAt、Operations、Redaction Policy
  和完整 Fixture Evidence。

三者都不包含 Secret、Lifecycle/Freeze 时间或 Integrity 自身；`freezeReportHash` 单独绑定
实际审阅报告 Bytes。下游 Approved Profile Bundle 必须同时固定这三个 Provider Hash
与 `freezeReportHash`，禁止用笼统的 `contractHash` 替代；Search Profile 晋升后还要另加
`bakeoffReportHash`，两种 Report Hash 不得互相替代。

`frozen` 必须同时满足：

1. `blockedBy=[]`、`unresolved=[]`、无 Unknown Required Fact/Operation；
2. 所有 Evidence Ref 存在且未过期，每份 Evidence Snapshot 的实际 Bytes 重算后匹配
   `contentHash`；
3. Secret/Placeholder/Schema/Replay/Dimension/Spatial Tests 全绿；
4. 至少两名 Reviewer，且包含 `governance_security`；
5. 三个 Contract/Fixture Hash 重算一致，`freezeReportHash` 与实际审阅产物 Bytes 一致；
6. Fixture 不是 `synthetic_test`。

## 9. Checked-in C0 intake

```text
mineru-public-draft.json                    blocked
mineru-local-batch-public-draft.json        blocked
jina-embedding-v4-1024-public-draft.json   blocked
jina-embedding-v4-2048-public-draft.json   blocked
jina-rerank-v3-public-draft.json            blocked
```

公开依据：

- <https://mineru.net/apiManage/docs>
- <https://api.jina.ai/openapi.json>
- <https://api.jina.ai/docs>

公开信息只能证明 Draft 字段。Local Batch Draft 仅开放
`allocate_upload` 的 `public_schema_synthetic` Fake Replay；成功 Summary Capture 通过
`redacted_capture_summary` metadata 绑定 Git-external Evidence Hash，但没有保留完整 Body，
不能冒充 `redacted_capture`。Upload/Poll/Download 仍为 Unknown；MinerU 用户 Endpoint、
不可变 Build、Signed Wire、Result Entry Schema/Content、Citation Locator、Recovery、
Query-by-key/Cancel、BBox、条款，以及 Jina Immutable Build、Region、Account Tier/Batch、
Normalization 与条款仍需合规 Evidence 或 Reviewed Terms，因此 C0 External Contract Gate
仍关闭。

## 10. Local replay

`tests/support/provider_contracts.py` 使用固定 Fixture Root、Draft 2020-12 Schema、RFC 8785
Hash 和语义/Secret Gate；不接受任意文件路径。`tests/support/fake_provider.py` 通过
Starlette + `httpx.ASGITransport` 内存重放，不监听端口、不发 DNS/TCP，不保存 Header
Value 或 Body，只记录 Header Name、Body Size 和 SHA-256。

这些文件全部位于 `tests/`，不进入 Runtime Image，也不能被 Production Handler import。
