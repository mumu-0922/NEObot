# MinerU Local Batch Draft Contract Plan

- 状态：implemented/reviewed；Contract 仍为 `draft/blocked`，Runtime 未启用
- 日期：2026-07-13
- 前置结论：生产候选使用 Local Upload Batch；现有 Remote URL Draft 保持不变

## 1. Objective

在不伪造 Wire Evidence、不保留 Signed URL、也不触碰生产 Adapter 的前提下，为 MinerU
Local Upload Batch 建立独立、封闭、可测试的 Provider Contract。该 Contract 只用于
Schema/Fixture/Fake Provider 验证，不能作为生产调用授权。

## 2. Closed boundary

新增独立 `providerKind=mineru_local_batch`，不得复用或改写
`providerKind=mineru_async` 的 Remote URL Flow。Operation Set 固定为：

```text
allocate_upload  POST /api/v4/file-urls/batch
upload           dynamic provider-signed URL
poll_batch       /api/v4/extract-results/batch/{batch_id}
download_result  dynamic provider result URL
cancel
query_by_key
```

本轮仅把 `allocate_upload` 标为 `observed`。其 Request/Response Case 来自官方文档的
`public_schema_synthetic` 安全样例；真实 Capture 只证明 Summary 与调用成功，不把它冒充
完整 `redacted_capture` Body。其余五项均为 `unknown`，不得携带 `method`、`pathTemplate`、
`request` 或 Response Case。

## 3. Implementation checklist

- [x] 固定独立 Provider Kind、六阶段 Operation Set 与 Remote/Local 隔离规则。
- [x] 扩展 JSON Schema 与语义校验器，保持旧四个 Draft 向后兼容。
- [x] 新增 `mineru-local-batch-public-draft.json`，生命周期保持 `draft/blocked`。
- [x] 对 Allocate Request/Response 使用 Closed Shape；拒绝额外字段、动态 Signed URL、
      Secret-like 字段与 Wire 漂移。
- [x] 验证未知 Operation 不得携带推测 Wire 字段，且 Frozen Gate 必须拒绝该 Draft。
- [x] 运行 Provider Contract 定向测试、非 Integration 全量测试、Coverage、Ruff、Mypy、
      dependency/security scan，并记录结果。

## 4. Non-goals and rollback

本轮不定义 Upload/Download 动态 Host allowlist，不 Capture 新请求，不实现 Adapter、Registry、
Dispatch、Governance Manifest、`011` 或 `012`。若 Contract 设计有误，只删除新增 Fixture、
回退 Schema/Validator/Test 变更；现有 Remote Draft 与 Runtime 暗运行边界不受影响。
