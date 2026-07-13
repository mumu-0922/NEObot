# Provider Capture Harness Contract

- 状态：Submit-only v1 与 Lifecycle v2 Harness implemented；第三次 Lifecycle Capture 已越过 transport、停在 Download contract gate
- 日期：2026-07-13
- 实现：`rag/tools/provider_capture.py`（编排）、
  `provider_capture_common.py`、`provider_capture_http.py`、
  `provider_capture_evidence.py`，以及隔离的
  `provider_capture_mineru_lifecycle*.py`、`provider_capture_mineru_targets.py`、
  `provider_capture_mineru_shapes.py`、`provider_capture_mineru_archive.py`
- 证据版本：Submit-only `mm-chat.provider-capture-evidence.v1`；Lifecycle
  `mm-chat.provider-capture-evidence.v2`

> Harness 只采集可人工审阅的脱敏 Wire Evidence。它不修改五个 Public Draft
> Fixture，不冻结 External Gate，不生成/Apply Governance，不注册 Dispatch/Handler，也不
> 授权任何生产 Provider 调用。

## 1. Threat Model

受保护资产包括 Provider Key、用户/知识库正文、MinerU Signed URL 与 Task/Batch/Trace ID、
Jina `request_id`、原始响应及未来 Governance Authority。主要威胁是误触真实 Egress、代理
或重定向绕过、SSRF、响应体放大、畸形 JSON/Unicode、Submit 响应丢失后的重复提交、证据
泄密、Symlink/覆盖攻击，以及把一次成功调用误当作 Frozen Contract。

Fail-closed 边界：

- CLI 默认 dry-run，`--execute` 是唯一网络开关；dry-run 不检查 Key、不创建目录或文件；
- 不加载 dotenv，不接受 Key 参数；只读取当前进程环境中的 `JINA_API_KEY` 与
  `MINERU_API_KEY`，输出、异常和证据均不保留 Key 的值、Hash、长度或前后缀；
- 交互式 Key 输入只能使用完整 No-echo Prompt 或受控 Secret Injection；禁止用循环
  `read -s -n1` 实现“部分可见”遮罩，因为长 Clipboard Paste/终端回显切换和换行 Scrollback
  可能暴露 Token；禁止打印 Key 前后缀作为确认；
- `httpx.Client(trust_env=False, follow_redirects=False)`，并发和连接池上限为 `1`，无
  retry；connect/read/write/pool timeout 分别固定为 `5/15/15/5s`；请求固定
  `Accept-Encoding: identity`、每次调用前清空 Cookie，响应拒绝压缩编码；
- 目标只允许 HTTPS、默认/显式 `443`、无 Userinfo/Query/Fragment 的精确三元组：
  `api.jina.ai POST /v1/embeddings`、`api.jina.ai POST /v1/rerank`、
  `mineru.net POST /api/v4/file-urls/batch`；
- 不接受输入文件、URL 或正文，只使用代码内置 synthetic 文本和确定性生成的小 PDF；
- 响应必须是 `200 application/json`、UTF-8 strict、无重复 Key/NaN/Infinity/超 JCS
  安全整数；按 raw wire stream 读取，声明或实际超过 `1,048,576` bytes 立即中止；
- 原始 Response bytes、未知 Header value、Error detail 和任何动态 ID/URL 永不落盘。

### Explicit private proxy follow-up

WSL 现场验证表明 `api.jina.ai` 直连超时，而 Owner 确认端口 `7890` 是其自控代理。Harness
仍不得信任通用 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY`。唯一允许的兼容入口是进程环境
变量 `PROVIDER_CAPTURE_PROXY_URL`，并且必须同时满足：

- Scheme 固定为 `http`，Host 必须是字面量 IPv4 RFC1918/Loopback 或 IPv6
  Unique-local/Loopback，必须显式 Port；
- 禁止 Username、Password、非根 Path、Query、Fragment、Hostname 与 Link-local/
  Multicast/Unspecified 地址；
- 通过 `httpx.Client(proxy=<validated>, trust_env=False)` 显式注入；不回退到任何通用代理；
- Proxy URL、Host、Port 和 Key 都不进入 stdout/stderr、Evidence、Hash 或日志；非法值只返回
  `CAPTURE_PROXY_INVALID`；
- `Accept-Encoding: identity`、TLS 证书验证、Provider Host/Path Allowlist、无 Redirect/Retry、
  固定预算和全部响应 Gate 保持不变。

该兼容只解决 Operator Capture 的 WSL Egress，不授权生产 RAG Runtime 使用代理，也不关闭
External Provider Gate。回滚方式是删除 `PROVIDER_CAPTURE_PROXY_URL` 并恢复直连。

## 2. Fixed call budget

CLI 只能选 `all|jina|mineru`，不能改变调用数量、batch、并发、timeout 或 retry：

| Provider | 固定调用 | 最大次数 | 语义 |
| -------- | -------- | -------- | ---- |
| Jina | `POST /v1/embeddings` | 1 | `jina-embeddings-v4`、`retrieval.passage`、1024、float、所有 truncate/late/multivector/tokenized 开关为 false |
| Jina | `POST /v1/embeddings` | 1 | 与上项完全相同，只把 dimensions 改为 2048 |
| Jina | `POST /v1/rerank` | 1 | `jina-reranker-v3`、2 条 synthetic docs、`top_n=2`、`return_documents=false`、`return_embeddings=false` |
| MinerU | `POST /api/v4/file-urls/batch` | 1 | v4 local-upload Submit，只提交确定性 synthetic PDF 的固定文件名/选项 |

### MinerU staged capture

公开 v4 local-upload Flow 是：

```text
POST /api/v4/file-urls/batch
  -> PUT signed URL (raw bytes; no Content-Type/Auth/Cookie)
  -> GET /api/v4/extract-results/batch/{batch_id}
```

Remote URL Flow 是：

```text
POST /api/v4/extract/task
  -> GET /api/v4/extract/task/{task_id}
```

原 `tools.provider_capture` **只执行 local-upload Submit Stage**。它不请求 Signed Host、不上传 PDF、
不 Poll Result；Signed URL 与 Batch/Trace ID 只在响应校验期间短暂存在内存，并被转换为
`batchIdPresent`/`signedUploadUrlCount` 后丢弃。因此本切片不能宣称完整 MinerU Capture。
公开资料尚无可靠 query-by-key、idempotency 或 cancel Contract；任何 Submit 后响应丢失都
记为 `unknown_submission`，调用次数保持 `1`，绝不自动重提。

独立 `tools.provider_capture_mineru_lifecycle` 已执行一次未完成的真实 Capture。它在单一进程内固定
执行 `1 Allocate + 1 Signed PUT + <=60 Poll + 1 Result Download`，Poll 间隔固定 5 秒；
不接受调用方传入 Stage、URL、Host、Batch ID、文件、预算、timeout 或 retry。Upload 只允许
文档记录的 OSS Host/Path，Download 只允许 MinerU CDN ZIP Host/Path；两者均禁止 Redirect、
Auth/Cookie 继承，Upload 还禁止 `Content-Type`。任一 Target 漂移只形成脱敏失败状态，不会向
该 Target 发请求。

## 3. Evidence Snapshot v1

顶层与嵌套对象由实现生成，不接受调用方追加字段：

```text
schemaVersion = mm-chat.provider-capture-evidence.v1
captureMode = authorized_execute
captureOutcome = fixed_plan_complete | unknown_submission
observedAt
budgets
providers[]
  provider / state / operationCount / operations[]
  operations[]:
    operation / method / path / state?
    requestBodySha256
    httpStatus? / responseContentType? / responseHeaderNames?
    response?                    # allowlisted shape/count only
syntheticArtifacts[]             # kind / byteCount / sha256 only
```

Jina Embedding 只保存 model、usage counts、item count、vector dimension 与 index；Rerank
只保存 model、usage count、result count、index 与 finite relevance score。MinerU 只保存
Submit State、是否存在 Batch ID 和 Signed URL 数量。禁止保存 Vector、输入正文、Jina
`request_id`、MinerU ID/URL、Provider Error detail 或未知 Header value；Header 默认仅保存
allowlisted lowercase name，`Content-Type` 规范化为 `application/json`。

Snapshot 使用 UTF-8、sorted keys、无多余空白、单一尾随换行的 Canonical JSON。同一逻辑
响应与固定 `observedAt` 必须产生相同 Bytes 和 SHA-256。这个 Evidence Hash 只是审阅产物
完整性，不是 Provider Key 或生产 Authority。

### Evidence Snapshot v2

Lifecycle Snapshot 保持同一顶层 Envelope，但只允许 MinerU 和四个固定 Operation：
`allocate_upload/upload/poll_batch/download_result`。它记录固定/实际调用预算、Poll State
计数、已验证身份响应数、Result URL 是否存在，以及 ZIP Byte/Entry Count、SHA-256 和四类
必需 Artifact 的存在性。Signed Query、Host/URL、Batch/Trace ID、文件名、`err_msg`、ZIP
Entry Name/Content 与原始 Response 永不进入 Evidence。

v2 对 incomplete 状态返回非零并安全写 Evidence：`unknown_submission`、Target rejected、
Upload/Poll/Download unknown/failed、`poll_exhausted`、`parse_failed`。所有阶段零自动 retry、
零自动 resubmit。新生成的 transport unknown 可额外记录闭集
`transportFailureClass`：`connect_timeout/read_timeout/write_timeout/pool_timeout`、
`connect_error/read_error/write_error/close_error`、`local_protocol_error/remote_protocol_error`、
`proxy_error/unsupported_protocol/other_transport_error`。分类只来自 `isinstance()`，不保存异常
消息、动态类名、Request 或 URL；非 `httpx.TransportError` 固定报 `CAPTURE_FAILED`，不得伪装
成 Provider 网络故障。该字段是非权威诊断，不能驱动 retry、恢复、Fixture Promotion 或稳定
Provider Error 认定。既有 v2 unknown Snapshot 无该字段时继续有效，其他 State 或未知枚举
拒绝。历史 v1 Snapshot 继续由原分支按原 Closed Schema 校验。

新生成的 `download_failed` 可额外记录闭集 `downloadFailureClass`：
`result_target_invalid/redirect_forbidden/status_invalid/content_encoding_invalid/`
`content_type_invalid/content_length_invalid/archive_too_large/archive_invalid`。它只映射 Harness
内部稳定 `CaptureError` Code，不记录实际 HTTP Status、Header Value、Body、ZIP Entry、异常
消息或 URL。既有 v2 failed Snapshot 无该字段时继续有效；未知 Error Code 不得降级为 open
fallback，而是固定 `CAPTURE_FAILED`。该字段同样不具备 Promotion Authority。

## 4. Safe output

`--output-dir` 只接受当前目录下的单一安全目录名。实现拒绝 absolute/path separator、
`..`、Symlink 和已存在目录；在 Provider Egress 前先做只读目标检查。写入时从 root/CWD
Directory FD 逐级使用 `O_DIRECTORY|O_NOFOLLOW` 打开父目录，再以 parent-FD-relative
`mkdir/openat/linkat/unlinkat` 操作 direct child，不重新解析已检查的父路径。成功时新目录
强制 `0700`、证据文件强制 `0600`；临时 inode `fsync` 后用不覆盖的 hard-link 创建最终名，
再 `fsync` 子目录与父目录。竞态创建的外来目标不会被覆盖或删除。

## 5. Commands

所有命令从 `mm-chat/rag` 执行。先运行零网络 Plan：

```bash
uv run python -B -m tools.provider_capture
uv run python -B -m tools.provider_capture --provider jina
uv run python -B -m tools.provider_capture_mineru_lifecycle
```

只有获批的独立 Operator Shell 才能临时导出所选 Provider Key 并显式执行：

```bash
export JINA_API_KEY=
export PROVIDER_CAPTURE_PROXY_URL="$https_proxy"  # only for the Owner-controlled WSL proxy
uv run python -B -m tools.provider_capture \
  --execute --provider jina \
  --observed-at 2026-07-13T00:00:00Z \
  --output-dir provider-capture-jina-20260713
unset JINA_API_KEY PROVIDER_CAPTURE_PROXY_URL
```

Lifecycle 真实执行只能在独立授权后，由完整 no-echo/受控 Secret Injection 提供新 Token：

```bash
uv run python -B -m tools.provider_capture_mineru_lifecycle \
  --execute \
  --observed-at 2026-07-13T00:00:00Z \
  --output-dir provider-capture-mineru-lifecycle-20260713
```

`.env.capture.example` 只列空值与安全说明；禁止 `source .env`、dotenv、Shell history 中
内联 Key 或把真实 Evidence 直接提交 Git。Capture 目录由 `.gitignore` 防误提交，验收后应
移动到 Mode `0700/0600` 的 Git 外 Evidence Store。
`-B` 是 CLI Contract 的一部分，用于禁止 Python bytecode cache；dry-run 因而不产生
Evidence、目录、`__pycache__` 或 `.pyc`。MinerU `unknown_submission` 会先安全落盘证据，
再返回 exit code `3`，不得由自动化视为完成。Lifecycle 所有 incomplete Outcome 同样在
Evidence 安全落盘后返回 terminal/non-retryable exit code `3`；自动化不得重跑、续传或重新
Submit，后续新 Capture 必须获得独立人工授权。

## 6. Manual review and freeze flow

1. 两人独立检查调用授权、账户/Region、固定 OpenAPI/Terms 来源与 Harness Git revision；
2. 在隔离 Shell 执行一个 Provider 子集，立即撤销/清除进程环境 Key；
3. 检查 mode `0700/0600`、Canonical Hash、预算和无敏感字段；将 Snapshot 置于 Git 外的
   审阅存储；
4. 人工把 allowlisted Capture facts 映射到 Provider Fixture，附独立 Terms Snapshot 与
   Evidence metadata；不得自动修改 Public Draft；
5. 运行 `provider_contracts.py` 的 Schema/secret/hash/freeze gates，并由至少两名 Reviewer
   （含 `governance_security`）签署 Freeze Report；
6. 只有 Frozen Wire Contract 才能进入后续 Governance Manifest/Profile Review。Harness
   自身永不 Apply Governance，也不启用 Registry/Dispatch/Handler。

Jina OpenAPI 研究基线为 `2026.06.29.1712`；公开 Tier 数值存在冲突，immutable build、
region、account limits、SLA 未冻结。MinerU immutable build、region、terms、BBox、cancel、
query-by-key 仍未验证。因此五个 Public Fixture 必须继续 `draft/blocked`。

2026-07-13 的 Jina 固定计划已完成一次真实 Capture：两次 Passage Embedding（1024/2048）
和一次 Rerank 均通过 Shape Gate，Canonical Evidence SHA-256 为
`e0c1ccd82b1a3d09ac65ea37dd7e18c36e06d9cf3b57cf235f150b273436d1a8`。该 Snapshot 已
移至 Git 外 Store 并保持 `0700/0600`；它不证明 immutable build、region、账户限额或条款，
也不能单独把 Fixture 晋升为 `verified/frozen`。

同日 MinerU staged plan 完成一次真实 Submit Capture：只调用
`POST /api/v4/file-urls/batch` 一次，Signed Upload 与 Poll 均为 `0/0`。Canonical Evidence
SHA-256 为 `a47a34559fbd262ba29a59181fe7b3ecedc8f1652305b2f4a22afdb342d23b46`，已移至
Git 外 Store 并保持 `0700/0600`。交互式部分遮罩在长 Token 粘贴时发生终端回显泄露；Owner
随后撤销该 Token。Evidence 不含 Token、动态 ID 或 Signed URL，仍可用于后续人工审阅；
该事件不提升 Fixture Lifecycle。

同日首次 MinerU Lifecycle Capture 完成 Allocate、Signed PUT 与四次 Poll，Poll 最终为
`done` 且提供通过 Target Gate 的 Result URL；唯一一次 Download 遇到不可恢复的 transport
异常，写入 legacy v2 `unknown_download` 并返回 exit code `3`。Canonical Evidence SHA-256
为 `06edec92a8cbc3dbf96dd261ccfa88cea34b08de703eaefd8ffb088c1aabc4b1`，权限为
`0700/0600`，并已移至 Git 外 Evidence Store。原始异常分类未被采集，现已不可恢复，禁止
推测或改写该 Evidence；此次结果只证明 `1/1/4/1` 调用与前三阶段行为，不证明 Result ZIP
Contract，也不提升 Fixture Lifecycle。

第二次独立授权 Capture 未复用旧任务状态，重新按固定计划执行 `1 Allocate + 1 Upload +
2 Poll + 1 Download`。Poll 观测 `waiting-file/done`，Result URL 通过固定 CDN Target Gate；
Download Evidence 记录 `transportFailureClass=connect_error`，Outcome 仍为 `unknown_download`
并返回 terminal exit code `3`。Canonical Evidence SHA-256 为
`7041a1c09e2f741875ffccb11d97ea806fc63e90e059f390124a1f953f047b55`，按 `0700/0600`
移至 Git 外 Store。该字段不能区分 Private Proxy、Proxy upstream、TCP、DNS、TLS 或 CDN
临时故障，因此禁止第三次带 Token Capture；后续只允许在新的人工授权下做无 Token、无
Submit 的 CDN connect-path 诊断。

无 Token connect-path Probe 证明 Private Proxy 到 CDN 的 TLS handshake 收到 unexpected EOF，
而三个 MinerU 固定 Host 的 WSL 直连 TLS 均成功。Owner 随后明确授权使用一次性 Token 执行一
次全直连 Capture；实际调用 `1/1/2/1`，前三阶段成功，Download 变为 `download_failed`。
Evidence SHA-256 为 `ec5ad91cf1c062d713aa70a62381f2d36b86810ec59c6ba92f93419f3d62dc96`，
按 `0700/0600` 移至 Git 外 Store。旧 producer 未记录具体 Contract Gate，因此不能从该
Evidence 推测 HTTP Status、Content-Type/Encoding/Length、Archive Size 或 ZIP Shape；一次性
Token 不再使用并应撤销。

## 7. Rollback

- 网络中止：停止进程；无 retry、无后台任务。MinerU Submit 已发送但响应未知时保留
  `unknown_submission`，禁止再次执行；由人工 Provider Console/Support 处置；
- 证据回滚：删除新建 Capture Directory；Harness 不写数据库、Fixture 或 Governance；
- 代码回滚：删除 `rag/tools/provider_capture*.py`、`rag/tools/__init__.py`、测试和本
  Contract；生产 Image 只复制 `src/`，且无 `[project.scripts]` 注册，所以无需 Runtime
  Migration；
- 代理回滚：不设置或 `unset PROVIDER_CAPTURE_PROXY_URL`；Harness 继续忽略所有通用代理
  环境变量；
- 若发现证据泄漏，隔离文件、撤销 Key、按 Incident 流程处理，Public Draft 与 External
  Gate 保持不变。
