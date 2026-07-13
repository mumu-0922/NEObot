# MinerU Lifecycle Evidence Promotion Plan

- 状态：implemented/reviewed；仅做 Summary Evidence 映射，不提升 Runtime Authority
- 日期：2026-07-13
- 输入：Git-external Evidence v2，SHA-256
  `5b4c3c8289c6c9ce8eec5f6bdc8af8fda60dea325376d55b7be62d72aaaa50e3`

## 1. Objective

把已完成的 MinerU Local Batch Lifecycle Capture 以可审计、可脱敏、可回滚的方式关联到
Public Draft Fixture。该映射只证明固定 synthetic PDF 的
`Allocate -> Upload -> Poll -> Download` Summary 在观测时刻成功，不把未保留的动态 URL、
原始 Request/Response Body、ZIP Entry 或 Provider Error 伪造成 Wire Fixture。

## 2. Promotion boundary

- Fixture 保持 `fixtureKind=public_documentation`、`lifecycle.state=draft`；不得进入
  `verified/frozen`。
- 新增一条 `sourceKind=redacted_capture_summary` Evidence metadata，只保存稳定 HTTPS 起始端点、
  Observed Time、Evidence Schema Version 和 Canonical Content Hash。
- `upload/poll_batch/download_result` 保持 `support.state=unknown`，但引用成功 Capture，并把
  “未 Capture”改为“仅有 Summary、Raw Wire 未保留”。
- 不新增 `redacted_capture` Response Case；当前 Schema 要求 Observed Operation 具备完整
  Method、Path、Request 和 Success Case，Summary 不满足该门槛。
- Capture Snapshot、Token、Signed URL、Batch/Trace ID、ZIP Entry Name/Content 均不进入 Git。

## 3. Residual blockers

Lifecycle 成功后仍必须保留：Raw Wire Body、动态 Upload/Download Target、稳定 Error/Recovery、
Cancel/Query-by-key/Idempotency、immutable Build、Region、Terms/Retention/License/SLA、ZIP Entry
JSON Schema、Canonical IR 转换和 Citation Locator。四个 Artifact Presence Boolean 不能替代
这些 Contract。

## 4. Execution checklist

- [x] 将脱敏 Evidence metadata/Hash 映射到 Local Batch Draft。
- [x] 用 Summary-only Reason Code 替换已过时的 `*_NOT_CAPTURED` 描述。
- [x] 增加回归测试，证明 Draft/Blocked、Unknown Wire 和 Synthetic Fake Replay 保持不变。
- [x] 同步 Promotion Readiness、Wire Contract、Tracking 文档。
- [x] 运行 Provider Contract、RAG 全量质量和安全关卡。
- [x] 完成独立 xhigh Review，并关闭全部 P0/P1/P2。

## 5. Rollback

回滚需删除新增 Evidence metadata/Schema 枚举与 provenance validator/test，恢复 Lifecycle
Blocker `SIGNED_UPLOAD_HOST_UNKNOWN`、`BATCH_POLL_NOT_CAPTURED`、
`RESULT_DOWNLOAD_NOT_CAPTURED`，并恢复 Operation Reason Code
`SIGNED_UPLOAD_WIRE_NOT_CAPTURED`、`BATCH_POLL_WIRE_NOT_CAPTURED`、
`RESULT_DOWNLOAD_WIRE_NOT_CAPTURED`。对应文档一并回退；Git-external Evidence 不变，Runtime
Registry、Governance、Dispatch、Migration `011/012` 和生产 Egress 始终保持关闭。
