# Internal Go → Python Evidence API Contract

## 1. 状态、范围与权威边界

- Contract 版本：`v1`
- Phase：`15.2A`
- 状态：接口与安全边界冻结；实现尚未开始
- 唯一路由：`POST /internal/v1/evidence/query`

本 Contract 定义 Go API 调用私有 Python `rag-api` 的 Evidence Query 边界。它不定义
浏览器 API，也不把 Python 变成 Identity、ACL、Consent、Chat 或 Citation 的权威源。

```text
Browser -> Go (Session / ACL / Consent / Chat / Citation authority)
               -> private mTLS + compact JWS
                    -> Python rag-api (retrieval computation only)
               <- source references + scores only
          Go reauthorize -> hydrate -> answer-consent check -> BYOK -> citations
```

固定职责：

- Go 从当前 Postgres Session 解析 `actorUserId + sessionId`，解析用户显式选择的非空
  `collectionIds`，构造当前 ACL、Consent、Governance 和 Retrieval Profile Snapshot。
- Python 只执行 Dense、BM25、Exact、RRF、可选 Rerank 与 Parent/Window Expansion，
  并返回不含正文的 Source References、Scores、Profile 与 Degraded State。
- Python 不返回 Source Text、Snippet、Title、File Name、Object Key、预签名 URL、
  Prompt、Answer 或 Citation；不调用 Chat Provider，也不签发 Citation。
- Go 收到候选后必须逐条按当前数据库状态重新授权 Source Version，随后才可从受限
  Repository/Object Gateway Hydrate 正文。
- 在把任何 Hydrated Evidence 发往用户选择的 BYOK Answer Provider 前，Go 必须再检查
  精确 `processor + endpointId + modelId + apiVersion + purpose=answer` 的 Active
  Governance Profile、Collection Answer Consent 和当前用户 Query Consent。
- Go 仅在 Answer Provider 返回且第二次 Consent/ACL Recheck 成功后，才持久化答案并
  签发短时、Opaque、Session-bound Citation Capability。每次打开 Citation 再授权；
  Python 永远看不到 Capability Signing Key。

Postgres 仍是唯一授权源。JWS、Redis、Python Cache、检索索引和响应中的 Echoed
Snapshot 都不能授予权限。普通聊天不调用本接口；只有显式 Knowledge Selection 的
Grounded Flow 才可调用。

## 2. 网络与传输安全

### 2.1 私有网络

- 路由只监听 Compose Private Network，不发布 Host Port，不经过公开 Ingress。
- 浏览器、Next.js Public Runtime 和公网不得直连 `rag-api`。
- Network Policy/Compose Wiring 只允许 Go API 连接 `rag-api` 的 Query Port。
- 请求必须使用 TLS 1.3；明文 HTTP、TLS 降级和经 Public Reverse Proxy 转发均拒绝。

### 2.2 双向 TLS

- Go 和 Python 使用独立 Workload Certificate；证书由专用 Internal CA 签发。
- Python 同时验证 Trust Chain、有效期、Key Usage 和固定的 Go Workload DNS SAN；仅
  验证“由同一 CA 签发”不够。
- Go 同样固定 Python `rag-api` DNS SAN 并验证 Trust Chain；禁止
  `InsecureSkipVerify`。
- Private Key 只读挂载、不得进入 Image/Repository/日志；轮换允许新旧 CA/Leaf
  短暂双信任，但不得复用 JWS Signing Key。
- mTLS 失败在握手层终止，不返回业务 JSON。mTLS 只证明 Workload，不能替代 JWS 的
  Request、Body、Actor、Session、Authorization 和 Profile Binding。

## 3. HTTP Wire Contract

```http
POST /internal/v1/evidence/query HTTP/1.1
Host: rag-api.internal
Content-Type: application/json
Accept: application/json
Authorization: Bearer <Ed25519 compact JWS>
X-Request-Id: <UUIDv4>
X-Body-SHA256: <base64url-no-padding SHA-256 of exact body bytes>
```

规则：

- `Content-Type` 必须是 `application/json`，UTF-8，Body 上限 `64 KiB`。
- `X-Request-Id` 必须是 UUIDv4，并等于 Body `requestId` 和 JWS `rid`。
- `X-Body-SHA256` 基于收到的原始 Body Bytes 计算，不得先 Parse/Reserialize；它必须
  等于 JWS `bsh`。代理不得压缩或改写 Body。
- 只接受一个 `Authorization`、`X-Request-Id` 和 `X-Body-SHA256` Header；重复 Header
  Fail Closed。
- 不接受 Cookie、Browser Bearer、Raw Session Token、User BYOK Key、MinIO Credential
  或客户端提交的 ACL 字段。
- 成功和错误响应都使用 `application/json; charset=utf-8` 与
  `X-Content-Type-Options: nosniff`，并回显 `X-Request-Id`。

## 4. Workload Token：Ed25519 Compact JWS

### 4.1 Protected Header

JWS 使用 Compact Serialization；禁止 `alg=none`、HMAC、Unencoded Payload 和动态
`jku/jwk/x5u/x5c`。

```json
{
  "alg": "EdDSA",
  "kid": "go-evidence-2026-07-01",
  "typ": "evidence+jws"
}
```

- `alg` 固定为 `EdDSA`（Ed25519）。
- `kid` 只能命中 Python 本地只读 Allowlist 中的公钥；未知、Retired 或重复 `kid`
  返回统一认证失败。
- Key Rotation 使用不同 `kid`；最大 Token TTL 与 Clock Skew 过去后才能移除旧公钥。

### 4.2 Payload Claims

```json
{
  "ver": 1,
  "iss": "mm-chat-go-api",
  "sub": "evidence-query",
  "aud": "mm-chat-rag-api",
  "iat": 1783828800,
  "nbf": 1783828795,
  "exp": 1783828815,
  "jti": "85b6da2c-2eba-4a1a-88fe-a6bdfef87d0d",
  "rid": "3cd0bbd4-cee9-49f5-ae26-fab25e89eb37",
  "mth": "POST",
  "pth": "/internal/v1/evidence/query",
  "bsh": "base64url-sha256",
  "uid": "11111111-1111-4111-8111-111111111111",
  "sid": "22222222-2222-4222-8222-222222222222",
  "azf": "base64url-sha256",
  "rpf": "base64url-sha256"
}
```

校验要求：

- `iss/sub/aud/ver/mth/pth` 必须精确匹配上表；Audience 可以是 String，不接受数组。
- `iat/nbf/exp` 是整数 NumericDate；`max_token_ttl=15s`，
  `clock_skew=5s`。未来 `iat`、过期、尚未生效或超长 TTL 全部拒绝。
- `jti/rid/uid/sid` 必须为 UUID；`jti` 每次尝试重新生成，Retry 不得复用 Token。
- `uid/sid` 必须分别等于 Body `authorization.actorUserId/sessionId`。
- `azf` 必须等于 Body `authorization` 对象的 RFC 8785 JCS Bytes 的 SHA-256
  Base64url；`rpf` 必须等于 `retrieval.profileHash`。
- 在上述显式 Binding 之外，`bsh` 仍绑定完整原始 Body；任何 Query、Collection、
  Fence、Consent、Profile 或 Limit 变化都必须重新签名。
- Python 不从 Claims 推导新的 ACL。Claims 只证明 Go 对同一 Body 的授权快照签名。

### 4.3 Replay Prevention（单次 `jti`，Fail Closed）

Python 在完成 mTLS、JWS Signature、Time、Header、Body Hash、DTO 和 Binding 校验后，
在执行任何检索或 External Egress 前原子消费 `jti`：

```text
SET evidence:jti:<kid>:<jti> <requestId> NX EXAT <exp+5s>
```

- Redis `SET NX + TTL` 只是短时 Nonce Store，不是 Authorization Source。Key 的 TTL
  必须至少保留到 `exp + clock_skew`；Redis 使用 `noeviction` 或等价配置，不能因内存
  压力提前逐出尚未过期的 Nonce。
- `SET NX` 失败返回 `409 WORKLOAD_TOKEN_REPLAYED`；顺序或并发重复均只有第一个请求
  可以跨过 Nonce Gate。
- Redis 连接错误、Timeout、Read-only、写失败、Nonce Sentinel 缺失或 Epoch 无法确认
  时立即 Fail Closed，返回 `503 REPLAY_PROTECTION_UNAVAILABLE`。禁止继续检索，禁止
  退化为进程内 Cache-only，更禁止静默进入“无 Replay Protection”模式。
- Nonce Namespace 必须有独立 `epoch`。检测到 Redis `FLUSH*`、Restart、Failover 后
  State Loss、Sentinel 丢失或 Epoch 变化时，视为所有尚未过期 Nonce 已丢失：
  `rag-api` 立即退出 Readiness，并从检测时刻开始隔离至少
  `max_token_ttl + clock_skew`（当前为 `15s + 5s = 20s`）。隔离期间所有 Evidence
  Request 一律返回 `503 REPLAY_PROTECTION_UNAVAILABLE`；隔离结束并确认新 Epoch 可
  原子写入后才可恢复接收。不得因进程重启、流量压力或健康检查绕过这段窗口。
- 进程内有界 Consumed-JTI Cache 只能做纵深防御，不能缩短上述隔离窗口，也不能替代
  Redis Epoch/Nonce 连续性。
- 本单服务器 Profile 冻结 Redis 为唯一 Active Nonce Backend。不得按可用性切到
  Postgres、进程内 Cache 或其他未评审 Backend；以后若改用 Durable Nonce，必须单独
  ADR、迁移、故障注入和 Replay 测试后再切换。
- Token 一经消费，无论后续检索成功与否都不可重用；Go Retry 必须重新授权、重算
  Snapshot 并签发新 `jti/rid`。

## 5. Request DTO

以下 JSON 是唯一允许的 Shape；未知字段、重复 Key、非有限数字和错误 Union Shape
均返回 `400 INVALID_EVIDENCE_REQUEST`。

```ts
type UUID = string;
type Sha256B64Url = string;

interface EvidenceQueryRequestV1 {
  version: 1;
  requestId: UUID;
  query: {
    text: string; // trimmed UTF-8, 1..8192 bytes
    locale?: string; // BCP 47, max 35 bytes
  };
  authorization: {
    actorUserId: UUID;
    sessionId: UUID;
    queryConsentStateRevision: number; // global user-query invalidation fence, integer >= 1
    collections: CollectionAuthorizationSnapshot[]; // 1..32, unique ID
    processors: QueryProcessorAuthorization[];
  };
  retrieval: {
    profileId: UUID;
    profileHash: Sha256B64Url;
    indexGenerationId: UUID; // current corpus-wide generation
    projectionRevision: number; // exact corpus head revision, integer >= 1
    maxEvidence: number; // 1..16; Go server policy, not browser input
  };
}

interface CollectionAuthorizationSnapshot {
  collectionId: UUID;
  scope: "personal" | "team";
  ownerUserId?: UUID; // personal only; must equal actorUserId
  teamId?: UUID; // team only
  aclRevision: number;
  visibilityEpoch: number;
  collectionProcessingRevision: number;
}

interface QueryProcessorAuthorization {
  purpose: "query_embedding" | "rerank";
  processor: string;
  endpointId: string;
  modelId: string;
  apiVersion: string;
  governanceProfileId: UUID;
  governanceRevision: number;
  governanceHeadRevision: number;
  queryConsentRevision: number; // exact current processing-consent row revision
  collectionConsentRevisions?: Array<{
    collectionId: UUID;
    consentRevision: number;
  }>;
}
```

约束：

- Collection 顺序按 `collectionId` 升序；Processor Binding 按
  `purpose,processor,endpointId,modelId,apiVersion` 升序，确保 `azf` 可重复计算。
- Personal Snapshot 必须只有 `ownerUserId`，且等于 Actor；Team Snapshot 必须只有
  `teamId`。Python 不接受“Global Admin”“Role”“Allowed Groups”等扩权 Hint。
- `collections` 必须与用户显式选择完全一致。任何 Missing、Tombstoned、Unauthorized
  或 Stale Collection 使整个请求失败；不得静默丢弃某个 Collection 后返回部分结果。
- Processor Binding 只允许当前 Retrieval Profile 实际需要的
  `query_embedding/rerank`。本地 BM25/Exact 不伪造 External Processor Binding。
- `purpose=query_embedding` 不得携带 Collection Consent；它只处理用户 Query，并要求
  当前 User Query Consent。`purpose=rerank` 会处理候选 Source Text，必须携带与
  `collections` 完全一致且无重复的 `collectionConsentRevisions`，同时要求 User
  Query Consent。两种 Purpose 不得共用或相互替代 Consent。
- `authorization.queryConsentStateRevision` 绑定全局 User Query Consent State
  Fence；每个 Processor 的 `queryConsentRevision` 绑定该精确
  `processor + endpointId + modelId + purpose` 的 Current Query-scope Consent Row。两者
  必须同时为 Current，不得用一个 Revision 替代另一个。
- 外部调用前，Python 必须通过参数化、受限 DB Function 对签名 Snapshot 的 Active
  Governance Head、Profile、Query Consent、逐 Collection Consent 与 Revision/Fence
  做当前态验证；External Response 纳入结果前再验证一次。验证失败不得缓存或返回旧
  Evidence。
- Query 是 Sensitive Data：不得写日志、Metric Label、Trace Attribute、Redis Key 或
  Error Message。只允许内存中短时处理。

## 6. Python 检索与授权约束

1. Python 先校验 Transport、JWS、Replay、DTO 和 Profile，然后才访问数据库或外部
   Processor。
2. 每条 Dense/BM25/Exact Lane 的 SQL/Function 内部都应用全部签名 Collection Fence：
   Scope/Owner/Team、ACL Revision、Collection Visibility Epoch、Collection Processing
   Revision、Document/Version Active State、Document Visibility Epoch、精确
   `currentVersionId`、Active Generation 和 Projection Revision。
3. Python 的 `rag_api_reader` 仅可调用冻结的参数化
   `knowledge_search_evidence_candidates(...)` 和
   `knowledge_expand_evidence_candidates(...)`；不得任意读 Credential、Email、
   Display Name、Raw Object Key、Message、Consent Payload 或更新权威表。两个
   Function 可返回已应用完整 Fence 的有界 Candidate/Expansion Source Text，但只能
   存活于当次 Python 请求内存，不得进入 HTTP Response。
4. Python 不缓存“Actor 有权访问 Collection”的结论。允许缓存与 Actor 无关的 Query
   Embedding/Plan 时，Cache Key 必须含 Profile Hash；候选结果缓存还必须含完整 `azf`，
   且短 TTL、撤权事件可失效。
5. Expansion 生成的每个 Parent/Window Span 仍需经过同一 Fence；Child 命中不能自动
   授权 Parent 正文。
6. 只有 `purpose=rerank` 的精确 Governance、User Query Consent 和所有涉及
   Collection Consent 在 External Call 前均为 Current 时，Python 才可将有界
   Candidate Source Text 发往冻结的 Jina Reranker。External Response 纳入前必须
   再验一次；Query-embedding Consent 不授予这项 Source Egress。
7. 客户端断开或 Go Deadline 取消时，Python 必须取消排队、SQL 和 Provider Request，
   不继续后台计算。

## 7. Success Response DTO

```ts
interface EvidenceQueryResponseV1 {
  version: 1;
  requestId: UUID;
  authorizationFingerprint: Sha256B64Url; // exact azf echo
  outcome: "evidence" | "insufficient_evidence";
  evidence: EvidenceReference[]; // 0..16
  profile: {
    retrievalProfileId: UUID;
    profileHash: Sha256B64Url;
    indexGenerationId: UUID; // corpus-wide generation echo
    projectionRevision: number; // exact corpus head revision echo
  };
  degraded: {
    active: boolean;
    reasons: Array<"query_embedding_unavailable" | "reranker_unavailable">;
    omittedLanes: Array<"dense" | "rerank">;
  };
}

interface EvidenceReference {
  rank: number; // contiguous, starts at 1
  collectionId: UUID;
  documentId: UUID;
  documentVersionId: UUID;
  indexGenerationId: UUID;
  documentMaterializationId: UUID;
  parentChunkId: UUID;
  childChunkIds: UUID[]; // non-empty, unique
  sourceSpanHash: string; // lowercase hex SHA-256
  locator: SourceLocator;
  scores: {
    dense?: number;
    bm25?: number;
    exact?: number;
    rrf: number;
    rerank?: number;
    final: number;
  };
}

type SourceLocator =
  | { kind: "text_offset"; start: number; end: number }
  | { kind: "line_range"; startLine: number; endLine: number }
  | { kind: "page_bbox"; page: number; bbox: [number, number, number, number] }
  | { kind: "slide_shape"; slide: number; shapeId: string }
  | { kind: "sheet_cell"; sheet: string; range: string }
  | { kind: "ooxml_part_xpath"; part: string; xpath: string };
```

响应约束：

- `authorizationFingerprint`、`profileHash`、`indexGenerationId`、
  `projectionRevision` 或 `requestId` 不匹配时，Go 丢弃整个响应。
- `outcome=insufficient_evidence` 必须有空 `evidence`；它不是 Error，也不能触发模型用
  常识补答。Go 返回固定 Abstention 或仅展示本地 Source 列表（如有独立已授权流程）。
- Score 必须是有限 Number；不同 Profile 的分数不可横向比较。排序以 `rank` 为准，
  Tie-break 规则由 Retrieval Profile 固定。
- Response 绝不能包含正文、Snippet、Document Title、File Name、MIME、Object/Bucket
  Key、URL、用户/团队显示信息、Consent 内容、Provider Payload、Answer、Citation ID 或
  Citation URL。
- `EvidenceReference` 只是候选定位符，不是 Authorization Grant。Go 必须检查候选属于
  原请求集合、版本仍是 Active Current Version、Generation/Projection 仍有效，并在
  Hydration 前后重验 ACL/Visibility/Consent Fence。

## 8. Degradation Contract

允许的降级只有：

| 故障                   | Python 行为                                                                                                                  |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Query Embedding 不可用 | 仅当冻结 Profile 允许时使用 BM25 + Exact；`degraded.active=true`、Reason 为 `query_embedding_unavailable`、Omit `dense`。    |
| Reranker 不可用        | 仅当同一 Profile 已晋升 RRF-only Fallback 时继续；标记 `reranker_unavailable` 并 Omit `rerank`。否则 `503 RERANK_REQUIRED`。 |

以下情况禁止 Degrade：ACL/Session/Consent/Governance/Profile/Fence 不确定、某个选中
Collection 不可用、Postgres 不可用、Replay Protection 不可用、无 Active Generation、
Response Schema 不合法。它们全部 Fail Closed。禁止自动切换 Processor、Endpoint、
Model、API Version、Embedding Dimension 或 silently drop Collection。

Go 必须把 Degraded State 带到 Grounded Chat UI/SSE；不得把 BM25-only 或 RRF-only
结果宣称为完整语义检索。Python 返回 `200` 只表示检索 Contract 成功，不表示 Go 已
授权 Hydration 或 Answer Egress。

## 9. Error Contract

```json
{
  "version": 1,
  "requestId": "3cd0bbd4-cee9-49f5-ae26-fab25e89eb37",
  "error": {
    "code": "AUTHORIZATION_SNAPSHOT_STALE",
    "message": "evidence request could not be authorized",
    "retryable": true
  }
}
```

| HTTP  | Code                            | Retry    | 语义                                                                                              |
| ----- | ------------------------------- | -------- | ------------------------------------------------------------------------------------------------- |
| `400` | `INVALID_EVIDENCE_REQUEST`      | no       | JSON/DTO/UUID/Hash/Limit/Union 非法或未知字段。                                                   |
| `401` | `INVALID_WORKLOAD_AUTH`         | no       | JWS 缺失、签名/KID/Audience/Time/Method/Path/Body/Actor/Session Binding 失败；不细分泄露原因。    |
| `409` | `WORKLOAD_TOKEN_REPLAYED`       | no       | `jti` 已消费。                                                                                    |
| `409` | `AUTHORIZATION_SNAPSHOT_STALE`  | yes-once | ACL、Visibility、Consent、Governance 或 Current Version Fence 已变化；Go 重新读库后最多重试一次。 |
| `409` | `RETRIEVAL_PROFILE_MISMATCH`    | yes-once | 请求 Profile 与 Active/Loaded Profile 不一致。                                                    |
| `422` | `QUERY_NOT_SEARCHABLE`          | no       | Query 为空、仅控制字符或无法规范化。                                                              |
| `429` | `EVIDENCE_QUEUE_FULL`           | yes      | 三个执行槽与六个排队槽已满；带整数秒 `Retry-After`。                                              |
| `503` | `RAG_NOT_READY`                 | yes      | Active Projection 未追平或服务未 Ready。                                                          |
| `503` | `REPLAY_PROTECTION_UNAVAILABLE` | yes      | Redis Nonce Store/Sentinel 不可用。                                                               |
| `503` | `QUERY_DEPENDENCY_UNAVAILABLE`  | yes      | Postgres 或必需 Retrieval Dependency 不可用。                                                     |
| `503` | `RERANK_REQUIRED`               | yes      | Reranker 失败且无已晋升 RRF-only Profile。                                                        |
| `504` | `EVIDENCE_QUERY_TIMEOUT`        | yes      | Deadline 到期且执行已取消。                                                                       |
| `500` | `INTERNAL_EVIDENCE_ERROR`       | maybe    | 未分类内部错误；响应与日志均清洗敏感信息。                                                        |

错误不得包含 Query、Source Text、SQL、DSN、Host、Key ID Allowlist、Signature、Token、
Session/User/Team ID、Consent Payload、Object Key 或 Provider Response。Go 对 `409`
只能在重新解析当前 Session/ACL/Consent/Profile 后重签一次；不得原样重放请求。

## 10. Go 后处理与 Citation Contract

Python 成功返回后，Go 必须按以下顺序执行，任何一步失败都不得发送 Grounded Answer：

1. 校验 Response Schema、`requestId/azf/profileHash` Echo、数量、重复 Ref 和 Collection
   子集。
2. 对每条 Source Ref 回读当前 Postgres 权威状态；检查 Actor、Session、Membership、
   Collection ACL/Visibility/Processing Revision、Document/Version Active Current
   Pointer、Generation/Projection 和删除状态。
3. 只通过 Go-only `knowledge_reauthorize_and_hydrate_evidence(...)` 及受限
   Object Gateway 按精确 Ref + Span Hydrate；该 Function 只 Grant 给
   `go_evidence_hydrator`，不 Grant 给 `rag_api_reader`。Python Ref 不能直接转换成
   任意 SQL、Path 或 Object Key，Go Runtime 也不获得 Base Projection/Object-Key SELECT。
4. Hydration 后再次检查 Source Hash/Span Hash 与 Authorization Fence；失效候选整条
   删除。若剩余证据不足，固定 Abstain，不调用 Answer Provider。
5. 将 Document Content 包装为明确的 Untrusted Evidence Envelope，禁止执行其中的
   Prompt、URL、Tool Call 或 Secret Request。
6. 对用户实际 BYOK Answer Endpoint 执行精确 Governance/Consent Check：
   `processor + endpointId + modelId + apiVersion + answer`；逐 Collection Consent 与
   当前 User Query Consent 缺一不可。检查通过前不得发送 Source Text。
7. Provider Response 提交/持久化前，再检查 Session、ACL、Consent、Governance 和
   Source Fence。失败则丢弃 Provider Output 并返回受控错误。
8. Go 为真正用于答案的 Source Mint Opaque Citation ID。Citation 必须绑定
   `actorUserId + sessionId + conversationId + assistantMessageId + documentVersionId + documentMaterializationId + sourceSpanHash + expiry`，
   数据库只保存 Hash/Capability Record 或等价不可枚举状态。
9. Citation Resolve 每次都重新授权；Session Logout/Expiry/Recovery、Membership/ACL
   变化、Consent 撤回/过期、Governance Head 变化、Document Tombstone 或 Version
   Replacement 立即使旧 Citation 失效。

不得把 Python 的 `parentChunkId`、`sourceSpanHash` 或数据库 UUID 直接当作公开 Citation
Token。公开 Token 必须高熵、Opaque、短时且不可由 Source Identity 推导。

## 11. Data Minimization、日志与保留

- Go → Python 只发送 Query、必要 UUID/Fence、精确 Processor Binding 和 Retrieval
  Profile；不发送 Email、Display Name、Message History、Raw Bearer、BYOK Secret、
  Document Text、File Metadata 或 Team Member List。
- Python → Go 只发送 Source Refs/Scores/Profile/Degraded；不发送 Source Content 或
  Authorization Explanation。
- Python 日志只允许 Request ID、JTI Hash Prefix、Profile Hash Prefix、Collection
  Count、Evidence Count、Lane/Latency、Degraded/Error Code；禁止原始 UUID 列表、Query、
  JWS、Evidence Ref、Source、Consent 与 Provider Payload。
- Metric Label 禁止 User/Session/Collection/Document/Query/JTI 等高基数或敏感值。
- Trace 只记录受控状态与耗时。Request/Response Body、Authorization Header 和 TLS Key
  Material 永不采集。
- Query、Evidence Candidate 和 External Provider Response 不落 Python Disk/DB/Redis；
  请求结束后释放。Crash Dump/Core Dump 必须禁用或加密并受运维保留策略控制。

## 12. 当前 Runtime Gap（不是已实现能力）

截至 migration `009` 与当前 Go Runtime，以下均为 Phase 15.2D 前的阻断项：

1. **`sessionId` 未传播**：Session Resolver 能取到 `auth.Session.ID`，但 HTTP
   Middleware 只把 `auth.User` 写入 Context；Chat/Knowledge Handler 当前拿不到权威
   Session ID。必须新增 Server-owned Session Context，且继续拒绝 Body/Query 中的
   Caller-supplied `sessionId`。
2. **Chat 无显式 Knowledge DTO**：现有 Stream DTO 只有 `userMessageId`、
   `modelRef`、`config`、`systemInstruction`、`metadata` 和 `idempotencyKey`，并明确
   拒绝 `attachments`；现有 Message Attachment 的 `knowledge_source` 只是 File
   Link，不等于显式 Collection Selection。后续必须增加受严格校验的 Knowledge
   Selection DTO，禁止把 `collectionIds` 藏入 `metadata/config`。
3. **没有 User BYOK Runtime**：当前 Chat Provider 从服务器级
   `PROVIDER_*` Environment 读取一个 OpenAI-compatible Key/Endpoint/Model；虽然
   `provider_configs` 表存在，但当前 Streaming Runtime 未解析用户级 BYOK Credential。
   因而本 Contract 的 Answer Egress 流程尚不能宣称可用。
4. **Consent 唯一性未绑定 Endpoint/Model**：migration `004` 当前 Current/Revision Unique
   Index 只按 `subject + processor`，而 Governance Head 按
   `(processor, endpoint_id)`。为支持 Team Admin 同时批准同 Endpoint 的多个 Model，
   实现前必须把 Governance Head 迁移为
   `(processor, endpoint_id, model_id)`，Consent 迁移为
   `subject + processor + endpoint_id + model_id`，并把 Endpoint/Model 加入 Consent
   API/DTO；不能靠 Processor Alias 隐含 Endpoint/Model。
5. **Governance 缺独立 Model ID**：当前 `processor_governance_profiles` 只有
   `model_api_version`，Go Manifest 也只有 `modelApiVersion`。实现前必须拆为独立且均
   不可变的 `model_id` 与 `api_version`，并纳入 Manifest Hash、Head/Consent Binding、
   Workload Token 和调用前后 Recheck。
6. **Citation Runtime 不存在**：当前没有 Citation Capability Store、Mint/Resolve
   Endpoint、Session-bound Citation DTO 或 SSE Citation Event。Python 返回 Ref 不能
   被称为 Citation；Phase 15.2D 实现并通过撤权负向测试前，UI 不得展示“可点击引用”
   已完成。
7. **Evidence API 与 Python Projection 不存在**：当前没有 Python Service、Retrieval
   Projection、Workload JWS、mTLS Wiring 或 Replay Nonce Flow；本文冻结未来 Contract，
   不改变 Phase 15.2 设计中的 implementation pending 状态。

## 13. 必须通过的 Contract Tests

- mTLS：错误 CA/SAN、Expired Leaf、No Client Cert、公开端口访问全部失败。
- JWS：Wrong `alg/kid/aud/iss/sub`、Expired/Future/Long-lived、Method/Path/Body/Actor/
  Session/Profile mismatch、重复 Header 全部失败。
- Replay：顺序 Replay、并发 Replay、Redis Error、Redis Flush/Restart/Epoch Change、
  Sentinel Quarantine 和完整 `max_token_ttl + clock_skew` 隔离；只有首个合法 `jti`
  可执行检索，任何 Nonce Backend 不确定状态都 Fail Closed。
- DTO：Unknown/Duplicate Fields、Caller ACL/Role Hint、错误 Scope Union、重复
  Collection、超限 Query/Body/Evidence 全部失败。
- Authorization：Personal Cross-user、Removed Team Member、Stale ACL/Visibility/
  Processing/Consent/Governance/Current Version/Generation/Projection 全部零泄露；任一
  Collection 失败不得返回 Partial Result。
- Data Minimization：Python Response 与日志中无正文、Title、File Name、Object Key、
  User Metadata、Token 或 Citation。
- Degradation：Embedding Down 只走 BM25+Exact 并明示；Reranker Down 只允许已晋升
  RRF-only Profile；Processor/Model/Dimension 不自动切换。
- Go Post-processing：Response Echo Tamper、Ref Injection、Hydration Hash Mismatch、
  Reauthorization Race、Answer Consent Revocation、Provider Completion Race 全部 Fail
  Closed。
- Citation：Opaque/不可枚举/Session-bound；Logout、Recovery、Membership Removal、
  Consent/Governance Change、Tombstone、Version Replacement 后立即失效。
