# C1.3A Native Parsers Design

## 目标与非目标

目标是在 C1.2 隔离边界内解析 TXT、Markdown、HTML，保留结构及精确 Raw Byte / Unicode
Scalar / Line-Column Span，并生成确定性的内部 Native Artifact。

本模块不负责 NFC/LF Canonicalization、Normalization Map、Source Locator v2 Projection、
Canonical IR、Quality Report、Chunking、Provider、数据库、Registry 或生产 Dispatch。这些分别
属于 C1.4+ 或后续切片。

## 信任边界与数据流

```text
MMCP bound Source
  -> Sidecar/Supervisor（只编排，不解析 Source）
  -> READY + PID/PGID + Seccomp Filter Hash
  -> Child 读取 Source
  -> Router Admission
  -> Fixed Native Dispatch
  -> Parser + Child-local Locator Validation
  -> closed internal Native Artifact
  -> Supervisor 校验 JCS/Length/Hash/Limit/Source len+hash+format
  -> Sidecar 丢弃 Artifact，并返回 MMCP FORMAT_UNSUPPORTED + zero body
```

Parent 不解码或 DOM/Markdown-parse 原始 Source。Raw/Scalar/Line 语义验证使用 Child 内同一
`DecodedSource` 权威索引完成；Parent 只验证不执行内容语义的 Closed Artifact 与请求绑定。
在 C1.4 独立 Canonical/Quality Gate 完成前，任何 Native Artifact 均不可 Stage。

## 核心契约

### Native Artifact

`parser-native-artifact.v1` 是 Child-internal Closed DTO，包含：

- Source Format、Encoding、Bytes、SHA-256、Decoded Scalar Count；
- 连续 Node Ordinal、先于 Child 的 Parent Ordinal；
- Node/Fragment 的 Raw Byte、Decoded Scalar、Line/Column 半开区间；
- `identity | syntax_decode` Fragment Transform；
- 名称排序且唯一的 Closed Scalar Attributes。

它不是 Packaged Public Schema，也不是 `canonical-ir.v2`。

### Internal Result Frame

```text
4-byte big-endian canonical header length
N-byte closed canonical JCS header
8-byte big-endian body length
M-byte Native Artifact body
EOF; trailing bytes forbidden
```

Header discriminator 只允许 `native_success | failure`。Success 绑定 Format、Artifact
Version、Length 与 SHA-256；Failure 必须零 Body。Child 禁止伪造 Controller-only
`PARSER_CANCELLED` 与 `PARSER_SANDBOX_UNAVAILABLE`。

### Stable Errors

| 条件                                             | Error                                  |
| ------------------------------------------------ | -------------------------------------- |
| 非唯一编码、非法 Unicode                         | `ENCODING_AMBIGUOUS` / `INPUT_INVALID` |
| 未实现的 Native Format                           | `FORMAT_UNSUPPORTED`                   |
| Node/Fragment/Depth/Text/Artifact 超限           | `RESULT_TOO_LARGE`                     |
| Child-local Raw/Scalar/Line 不一致               | `QUALITY_LOCATOR_FAILED`               |
| Parent Closed Artifact 或 Request Binding 不一致 | `PARSER_SCHEMA_MISMATCH`               |
| Timeout/OOM/Cancel/Residual Process              | 复用 C1.2 Sandbox Stable Error         |

## 设计决策

| 决策                                  | 理由                                                               |
| ------------------------------------- | ------------------------------------------------------------------ |
| Native Artifact 不复用 MMCP success   | MMCP v1 success 冻结为 `canonical-ir.v2`，复用会伪造 C1.4          |
| Parser 只在 Seccomp Child lazy import | 保持 Source-before-parse 安全时序，Supervisor 不承载内容 Parser    |
| Markdown 固定 CommonMark + Table      | 禁止运行时插件发现和行为漂移                                       |
| HTML 关闭自动 CharRef 转换            | 显式保存 Entity 的 Raw Byte Span 与 decoded text                   |
| uint32 compact offset index           | 50 MiB Source 上限内 Offset 足够，避免 Python int tuple 的内存放大 |
| Parent 不复算 Locator                 | 避免可信编排进程解析不可信 Source；C1.4 再做独立 Quality Gate      |

## 威胁模型与措施

- **Active Content**：拒绝 Script、Event Handler、`javascript:`/`vbscript:`、`srcdoc`、
  DTD/Entity/XInclude、危险嵌入标签；公式或代码均不执行。
- **SSRF / Local File Read**：URL 只作为 non-dereferenced Native Attribute；无 fetch API。
- **Parser Bomb**：Source、Text、Line、Node、Fragment、Depth、Attribute、Artifact Bytes、
  CPU、Memory、PID 与 Wall Clock 全部有界。
- **Protocol Confusion**：Internal Result 与 MMCP 使用不同 Discriminator；JCS、Length、Hash、
  Body Limit、EOF、Format 与 Source Digest 均独立校验。
- **Locator 欺骗**：Child 内用 compact Decode Index 对每个 Node/Fragment 重算位置；Parent
  拒绝非 Closed 或不绑定 Request 的 Artifact。
- **Residual Process**：沿用 C1.2 Process-group kill/reap 与 restart fence。

## 已知限制

- C1.3A 只实现 TXT、Markdown、HTML；OOXML、CSV、PDF、MinerU 仍 Fail Closed。
- Native Attribute 尚未投影为最终 Provenance/Source Locator；C1.4 负责 Canonical Map。
- Native Artifact 本身不可持久化或供 Query 使用。
- Fresh-container 十次 Byte-identical Gate 属于 C1.4，当前只做重复解析与真实 Child 回归。

## 变更历史

- **2026-07-14**：建立 C1.3A Text Native Parsers、内部 Artifact Frame 与安全/资源 Gate。
