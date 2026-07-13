# MinerU Lifecycle Capture Harness Plan

- 状态：implemented/reviewed；真实 Capture 尚未执行，Runtime Adapter 未启用
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

本轮不执行真实 Capture、不读取 `.env`、不修改 Fixture Lifecycle、不生成 Governance、不实现
生产 Gateway/Adapter/Dispatcher，也不应用 `011/012`。回滚只删除新增 Lifecycle 模块/测试并
撤销 Evidence v2 分支；Submit-only v1 Harness、历史 Evidence 和 Runtime 暗运行边界保持不变。
