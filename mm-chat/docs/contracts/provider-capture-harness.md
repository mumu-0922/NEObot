# Provider Capture Harness Contract

- 状态：Phase 15.2C C0 Harness implemented；real capture not executed
- 日期：2026-07-13
- 实现：`rag/tools/provider_capture.py`（编排）、
  `provider_capture_common.py`、`provider_capture_http.py`、
  `provider_capture_evidence.py`
- 证据版本：`mm-chat.provider-capture-evidence.v1`

> Harness 只采集可人工审阅的脱敏 Wire Evidence。它不修改四个 Public Draft
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

当前 Harness **只执行 local-upload Submit Stage**。它不请求 Signed Host、不上传 PDF、
不 Poll Result；Signed URL 与 Batch/Trace ID 只在响应校验期间短暂存在内存，并被转换为
`batchIdPresent`/`signedUploadUrlCount` 后丢弃。因此本切片不能宣称完整 MinerU Capture。
公开资料尚无可靠 query-by-key、idempotency 或 cancel Contract；任何 Submit 后响应丢失都
记为 `unknown_submission`，调用次数保持 `1`，绝不自动重提。

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
```

只有获批的独立 Operator Shell 才能临时导出所选 Provider Key 并显式执行：

```bash
export JINA_API_KEY=
uv run python -B -m tools.provider_capture \
  --execute --provider jina \
  --observed-at 2026-07-13T00:00:00Z \
  --output-dir provider-capture-jina-20260713
unset JINA_API_KEY
```

`.env.capture.example` 只列空值与安全说明；禁止 `source .env`、dotenv、Shell history 中
内联 Key 或把真实 Evidence 直接提交 Git。当前任务没有执行上述 `--execute` 命令。
`-B` 是 CLI Contract 的一部分，用于禁止 Python bytecode cache；dry-run 因而不产生
Evidence、目录、`__pycache__` 或 `.pyc`。MinerU `unknown_submission` 会先安全落盘证据，
再返回 exit code `3`，不得由自动化视为完成。

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
query-by-key 仍未验证。因此四个 Public Fixture 必须继续 `draft/blocked`。

## 7. Rollback

- 网络中止：停止进程；无 retry、无后台任务。MinerU Submit 已发送但响应未知时保留
  `unknown_submission`，禁止再次执行；由人工 Provider Console/Support 处置；
- 证据回滚：删除新建 Capture Directory；Harness 不写数据库、Fixture 或 Governance；
- 代码回滚：删除 `rag/tools/provider_capture*.py`、`rag/tools/__init__.py`、测试和本
  Contract；生产 Image 只复制 `src/`，且无 `[project.scripts]` 注册，所以无需 Runtime
  Migration；
- 若发现证据泄漏，隔离文件、撤销 Key、按 Incident 流程处理，Public Draft 与 External
  Gate 保持不变。
