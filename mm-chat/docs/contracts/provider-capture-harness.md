# Provider Capture Harness Contract

- 状态：MinerU Submit-only v1 与 Lifecycle v2 可执行；Jina Capture 永久退休
- 日期：2026-07-25
- 实现：`rag/tools/provider_capture.py`、`provider_capture_common.py`、
  `provider_capture_http.py`、`provider_capture_evidence.py`，以及隔离的
  `provider_capture_mineru_lifecycle*.py`
- 证据版本：Submit-only `mm-chat.provider-capture-evidence.v1`；Lifecycle
  `mm-chat.provider-capture-evidence.v2`

> Harness 只采集可人工审阅的脱敏 MinerU Wire Evidence。它不冻结 External
> Gate，不生成或 Apply Governance，不注册 Runtime Handler，也不授权任何
> Retrieval Provider 调用。Jina 相关常量和校验器只用于读取既有历史 Evidence；
> 它们不提供 CLI selector、credential lookup、HTTP target 或网络执行能力。

## 1. Execution boundary

- CLI 默认 dry-run；只有 `--execute` 可以启用 MinerU 网络调用。
- CLI provider 选择严格为 `all|mineru`。`jina` 在参数解析阶段即失败。
- 只读取当前进程的 `MINERU_API_KEY`；不读取 `JINA_API_KEY`，不加载 dotenv，
  不接受 Key 参数。
- `httpx.Client` 固定 `trust_env=False`、`follow_redirects=False`，连接池和并发
  上限为 `1`，无 retry，使用固定 timeout 和有界响应流。
- HTTP allowlist 只包含：
  `mineru.net POST /api/v4/file-urls/batch`。`api.jina.ai`、`r.jina.ai` 及其
  子域、大小写和尾点变体均不在 allowlist。
- 不接受调用方 URL、Host、Method、Header、模型、文件或正文。输入仅为代码内
  生成的确定性 synthetic PDF。
- 原始 Response bytes、未知 Header value、错误详情、动态 ID/URL 和 Key 永不落盘。

`PROVIDER_CAPTURE_PROXY_URL` 只可显式设置为无凭据、带端口的私有或 Loopback
HTTP 地址。通用 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 永不继承。该代理只
服务 Operator Capture，不能授权生产 RAG Runtime 使用代理。

## 2. Fixed MinerU budgets

Submit-only v1 固定执行一次：

```text
POST /api/v4/file-urls/batch
```

它不 PUT Signed URL，也不 Poll。Submit 响应丢失记录为
`unknown_submission`，绝不自动重提。

Lifecycle v2 固定执行：

```text
1 Allocate + 1 Signed PUT + <=60 Poll + 1 Result Download
```

Upload/Download URL 必须通过冻结的 MinerU HTTPS Host/Path Gate；不得继承
Authorization、Cookie 或 redirect。ZIP 只做有界校验，不直接解压到工作区。

## 3. Historical Jina evidence decoder

`provider_capture_evidence.py` 仍可校验旧
`mm-chat.provider-capture-evidence.v1` 中的 Jina metadata shape，包括历史模型名、
调用数、维度、usage 和有限分数。保留这一 decoder 仅为了审计既有 SHA-256-bound
Evidence。

以下能力永久不存在：

- `--provider jina`；
- `JINA_API_KEY` credential loading；
- Jina Embedding/Rerank request builder；
- `api.jina.ai` 或 `r.jina.ai` target allowlist；
- 从历史 Evidence 派生 Governance、Consent、Evaluation、Rebuild、Rollback 或
  Activation 权限。

历史 Jina Capture 成功只说明当时返回形状通过校验，不是当前 Runtime Evidence，
也不能成为 Candidate promotion evidence。

## 4. Evidence and filesystem rules

Evidence 只保留 allowlisted state、count、boolean、finite metric 和 SHA-256；禁止
保存正文、Vector、Key、动态 URL/ID、原始响应或异常详情。输出目录/文件必须：

- 位于受控基目录的一个新 direct child；
- 使用 `0700/0600`；
- 拒绝 symlink、hardlink、覆盖和父目录漂移；
- Canonical JSON 后原子发布并 `fsync`。

任何 incomplete/unknown outcome 在安全落盘后返回非零，不得被自动化当作完成或
自动重跑。

## 5. Commands

所有命令从 `mm-chat/rag` 执行。零网络计划：

```bash
uv run python -B -m tools.provider_capture
uv run python -B -m tools.provider_capture --provider mineru
uv run python -B -m tools.provider_capture_mineru_lifecycle
```

经独立授权后，MinerU Submit-only 执行：

```bash
export MINERU_API_KEY='set-through-controlled-secret-injection'
uv run python -B -m tools.provider_capture \
  --execute --provider mineru \
  --observed-at 2026-07-25T00:00:00Z \
  --output-dir provider-capture-mineru-20260725
unset MINERU_API_KEY PROVIDER_CAPTURE_PROXY_URL
```

Lifecycle 执行：

```bash
uv run python -B -m tools.provider_capture_mineru_lifecycle \
  --execute \
  --observed-at 2026-07-25T00:00:00Z \
  --output-dir provider-capture-mineru-lifecycle-20260725
```

禁止 `source .env`、在 Shell history 内联真实 Key、提交真实 Evidence，或运行任何
历史 Jina Capture 命令。

## 6. Required tests

- `jina` selector 在 credential lookup、filesystem write 和网络之前失败；
- 所有 Jina host/method/path 组合均返回 `TARGET_NOT_ALLOWLISTED`；
- MinerU dry-run 不读取 Key、不写文件、不联网；
- fixed budget、response bounds、redirect/proxy/target gate 不可由参数放宽；
- 历史 Jina Evidence 可离线解码，但 decoder 不暴露网络调用对象；
- Evidence canonical hash、权限、no-overwrite、symlink/race 防护通过。

## 7. Wrong vs Correct

Wrong:

```bash
export JINA_API_KEY=...
uv run python -m tools.provider_capture --execute --provider jina
```

Correct:

```text
historical Jina JSON -> offline strict decoder -> audit only
MinerU authorized shell -> fixed capture plan -> redacted evidence
BGE runtime -> Go Provider Gateway -> SiliconFlow only
```
