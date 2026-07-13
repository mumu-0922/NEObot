# MinerU Lifecycle Capture Harness Plan

- 状态：implemented/reviewed；两次真实 Capture 均在 Download 阶段未完成，Runtime Adapter 未启用
- 日期：2026-07-13
- 前置：`mineru_local_batch` Fixture 已建立，但 Upload/Poll/Download 仍为 `unknown`

## 1. Objective

新增一个与原 Submit-only Harness 隔离的 MinerU Lifecycle Capture CLI，在一次进程内完成：

```text
Allocate -> Signed PUT -> bounded Poll -> Result ZIP Download -> redacted Evidence
```

Signed URL、Batch/Trace ID、Provider Error、ZIP 正文只允许短暂存在内存，永不写入 Evidence、
日志或 Git。工具仍默认 dry-run；只有显式 `--execute` 才能联网。

## 2. Compatibility boundary

- 保留 `tools.provider_capture` 与 Evidence v1 的现有行为、预算和历史 Snapshot 校验。
- 新 CLI 使用 `tools.provider_capture_mineru_lifecycle` 与
  `mm-chat.provider-capture-evidence.v2`，不得自动改写 Public Fixture。
- 只接受进程环境 `MINERU_API_KEY`；继续忽略通用 Proxy 环境，只允许已校验的
  `PROVIDER_CAPTURE_PROXY_URL`。
- 使用代码生成的单页 synthetic PDF；不接受文件路径、用户正文、URL、Batch ID、Host、
  timeout、poll 次数或 retry 参数。

## 3. Fixed network plan

| Stage | 固定上限 | Target rule |
| --- | ---: | --- |
| Allocate | 1 | `POST https://mineru.net/api/v4/file-urls/batch` |
| Upload | 1 | Provider Response 派生；HTTPS、443、精确 Host `mineru.oss-cn-shanghai.aliyuncs.com`、`/api-upload/` 前缀；无 Auth/Cookie/Content-Type |
| Poll | 60 | 代码用已校验 Batch ID 构造 `GET /api/v4/extract-results/batch/{batch_id}`；每次间隔 5 秒，无网络 retry |
| Download | 1 | Provider Response 派生；HTTPS、443、精确 Host `cdn-mineru.openxlab.org.cn`、`/pdf/` 前缀、`.zip` 后缀；无 Auth/Cookie |

Upload/Download URL 最长 4096 bytes，禁止 Userinfo、Fragment、非默认 Port、控制字符和
Redirect。Signed Query 只用于该次请求，不记录名称、值、长度或 Hash。Host/Path 不匹配时，
在不泄露 URL 的情况下写入 `target_rejected` Evidence 并停止。

## 4. Response and archive gates

- Allocate/Poll 复用 1 MiB JSON 上限、strict UTF-8/JSON、`200 application/json`、
  identity encoding 和 Closed Shape。
- Poll 只接受一个与 synthetic 文件名匹配的 Result；允许
  `waiting-file/pending/running/converting/done/failed`。`done` 必须有 Result URL，
  `failed` 只记录状态，不记录 `err_msg`。
- Download 最大 32 MiB compressed；ZIP 最多 256 entries、128 MiB aggregate
  uncompressed、64 MiB per entry；拒绝 encrypted、symlink、absolute/traversal、duplicate
  entry 和异常压缩比。
- Evidence 只保留状态计数、调用计数、Body/Archive SHA-256、Byte/Entry Count，以及
  `full.md`、`*_content_list.json`、`*_middle.json|middle.json`、
  `*_model.json|model.json` 是否存在；不保存 Entry Name 或内容。

## 5. Failure and recovery states

任何阶段都不自动重试，不后台继续：

```text
unknown_submission / upload_target_rejected / unknown_upload / upload_failed
unknown_poll / poll_exhausted / parse_failed
download_target_rejected / unknown_download / download_failed
lifecycle_complete
```

Evidence 记录实际 `used*Calls`。由于 Batch ID 不落盘，非 complete 状态只能由 Operator
Console/Support 处置，禁止重新 Submit 冒充恢复。输出仍使用原有 parent-FD、no-symlink、
atomic no-overwrite、`0700/0600` Writer。

## 6. Implementation checklist

- [x] 锁定独立 CLI、Evidence v2、动态 Target、预算、ZIP 与失败恢复边界。
- [x] 实现 Lifecycle HTTP Flow、Target/ID/Poll/ZIP Gate 与可注入 Sleeper。
- [x] 扩展 Evidence Validator/Writer，保持历史 v1 Snapshot 向后兼容。
- [x] 增加 dry-run、固定调用、Header、Redirect、Target、响应、预算、超时、ZIP bomb、
      Evidence redaction/权限/确定性及 Runtime-disabled 回归。
- [x] 运行 Ruff、Mypy、定向/全量测试、Coverage、dependency/security scan 并记录结果。

## 7. Non-goals and rollback

原 Harness 实现切片不执行真实 Capture，也不读取 `.env`、修改 Fixture Lifecycle、生成
Governance、实现生产 Gateway/Adapter/Dispatcher 或应用 `011/012`。后续真实执行结果单独记录
在第 8 节，不改变这些 Runtime 边界。代码回滚只删除新增 Lifecycle 模块/测试并撤销 Evidence
v2 分支；Submit-only v1 Harness、历史 Evidence 和 Runtime 暗运行边界保持不变。

## 8. First real Capture and diagnostic follow-up

2026-07-13 首次授权 Lifecycle Capture 使用新 Token 和 synthetic PDF，实际预算为
`Allocate=1 / Upload=1 / Poll=4 / Download=1`。Allocate、Signed PUT 与 Poll 均成功，Poll
依次观测 `waiting-file/pending/running/done`，但 Download transport 未取得可验证结果，最终
写入 legacy v2 `unknown_download`。Evidence SHA-256 为
`06edec92a8cbc3dbf96dd261ccfa88cea34b08de703eaefd8ffb088c1aabc4b1`，目录/文件权限为
`0700/0600`，并已移至 Git 外 Evidence Store；它不包含 URL、Query、ID、Provider Error、
ZIP 内容或 Token，也不能提升 Fixture。

- [x] 保留原始 Evidence，不补写已经丢失的异常原因，不自动 retry/resubmit。
- [x] 为后续 unknown transport 增加闭集 `transportFailureClass`，只按
      `httpx.TransportError` 类型分类，不保存异常消息、类名、Request 或 URL。
- [x] 保持 legacy v2 unknown Evidence 无该字段时仍可验证；其他 State 或未知枚举拒绝。
- [x] 通过定向/全量质量与安全门，并完成独立 Review `P0/P1/P2 = 0/0/0`。
- [x] 由 Owner 单独授权一次新 Lifecycle Capture；未把它表述为旧任务恢复或自动 retry。
- [x] 在不使用 MinerU Token、不重新 Submit 的独立授权下诊断 CDN/proxy connect path。
- [x] 经 Owner 明确授权使用一次性 Token，以全直连方式执行第三次固定 Capture；不再复用 Token。
- [x] 为后续 `download_failed` 增加闭集 `downloadFailureClass`，保持历史 v2 Evidence 兼容，
      且不得记录 HTTP Status、Header Value、Body、ZIP Entry 或异常消息。
- [x] 使用新 Token 完成一次全直连诊断 Capture，确认 Download Gate 为 `archive_invalid`；
      Evidence 移至 Git 外 Store，Token 已由 Owner 撤销。
- [x] 将 `archive_invalid` 拆为闭集 `archiveFailureClass`，不记录 Entry Name/Content，并保持
      既有 v2 failed Evidence 向后兼容。
- [ ] 只有完整 ZIP Evidence 与外部 Authority/Terms/SLA Gate 同时通过，才允许 Fixture Freeze。

第二次独立 Capture 的实际调用数为 `1/1/2/1`，Poll 依次为 `waiting-file/done`。Result URL
再次通过固定 Target Gate，但 Download 记录闭集 `transportFailureClass=connect_error`，Outcome
仍为 `unknown_download`，exit code `3`。Evidence SHA-256 为
`7041a1c09e2f741875ffccb11d97ea806fc63e90e059f390124a1f953f047b55`，已按 `0700/0600`
移至 Git 外 Evidence Store。该分类只证明 HTTPX connect path 失败，不能区分本地 Proxy、
Proxy upstream、TCP、DNS、TLS 或 CDN 临时故障，也不能证明 Provider ZIP Contract。没有执行
第三次 Capture；下一步必须把连接诊断与带 Token 的业务 Capture 分离。

无 Token Probe 随后证明：WSL 到 Private Proxy TCP 正常、Proxy 到 `mineru.net` TLS 正常、
Proxy 到 MinerU CDN TLS 在握手中收到 unexpected EOF，而 CDN 直连 TLS 正常。Owner 随后明确
授权使用一枚一次性 Token 进行一次全直连 Capture；三个固定 MinerU Host 的预检 TLS 均成功。
该 Capture 的实际调用数为 `1/1/2/1`，Allocate/Upload/Poll 成功，Download 从 transport
`connect_error` 推进为 `download_failed`，说明连接已建立但某个 Response/Archive Contract
Gate 未通过。Evidence SHA-256 为
`ec5ad91cf1c062d713aa70a62381f2d36b86810ec59c6ba92f93419f3d62dc96`，已按 `0700/0600`
移至 Git 外 Store。当前 Evidence 不含具体失败 Gate，不能推测为 HTTP Status、Content-Type、
Encoding、Size 或 ZIP Shape；一次性 Token 不再使用并应由 Owner 撤销。

后续实现只扩展非权威、闭集的 `downloadFailureClass`：`result_target_invalid`、
`redirect_forbidden`、`status_invalid`、`content_encoding_invalid`、`content_type_invalid`、
`content_length_invalid`、`archive_too_large`、`archive_invalid`。旧 v2 failed Evidence 无该字段
时继续有效；未知 Error Code、未知枚举或错误 State 携带该字段必须 fail closed。该增强不授权
新的 Provider Capture，也不提升 Fixture。

应用 `downloadFailureClass` 后的新全直连 Capture 实际调用数仍为 `1/1/2/1`，前三阶段成功，
Download 精确记录 `archive_invalid`。Evidence SHA-256 为
`6d227220d52b944a0824a779d00bc595fd3b6f086cdc1753f8e1719c363a4dd6`，已按
`0700/0600` 移至 Git 外 Store；临时脚本已删除，Owner 已确认撤销 Token。该结果证明 HTTP
Status/Encoding/Content-Type/Length/32 MiB compressed Gate 均已越过，但不能区分无效 ZIP、
CRC、危险 Entry、解压上限/压缩比或缺少必需 Artifact。

后续闭集 `archiveFailureClass` 只允许：`empty_archive/invalid_zip/crc_mismatch`、
`too_many_entries/unsupported_compression/unsafe_entry_name/unsafe_entry_path/duplicate_entry/`
`encrypted_entry/symlink_entry`、
`expanded_entry_too_large/expanded_total_too_large/invalid_compression_metadata/`
`compression_ratio_exceeded`、`missing_full_markdown/missing_content_list/missing_middle_json/`
`missing_model_json`。实现只携带硬编码 Class；Entry Name、数量之外的 Metadata、正文及异常消息
继续禁止。未知 ZIP 异常不得映射为开放 fallback，必须 `CAPTURE_FAILED`。
