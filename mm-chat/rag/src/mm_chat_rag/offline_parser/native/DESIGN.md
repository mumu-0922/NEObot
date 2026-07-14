# C1.3B Native Parsers Design

## 目标与非目标

目标是在 C1.2 隔离边界内解析 TXT、Markdown、HTML、CSV、DOCX、PPTX、XLSX，保留
结构与精确 Raw Byte / Unicode Scalar / Line-Column Span，并生成确定性的内部 Native
Artifact。

本模块不负责 NFC/LF Canonicalization、Normalization Map、Source Locator v2 Projection、
Canonical IR、Quality Report、Chunking、Provider、数据库、Registry 或生产 Dispatch。这些
属于 C1.4+ 或后续切片。

## 信任边界与数据流

```text
MMCP bound Source
  -> Sidecar/Supervisor（只编排，不解析 Source）
  -> READY + PID/PGID + Seccomp Filter Hash
  -> Child 读取 Source
  -> Router Admission
       text -> one DecodedSource
       OOXML -> one ValidatedOpcPackage
  -> Fixed Native Dispatch（无 fallback）
  -> Parser + Child-local Locator 重算
  -> closed parser-native-artifact.v2
  -> Supervisor 校验 JCS/Length/Hash/Limit/Source len+hash+format
  -> Sidecar 丢弃 Artifact，并返回 MMCP FORMAT_UNSUPPORTED + zero body
```

Parent 不解码或解析原始 Source。C1.4 独立 Canonical/Quality Gate 完成前，任何 Native
Artifact 均不可 Stage。

## 核心契约

### Native Artifact v2

- Source Unit `0` 永远是完整 Raw File；Text Format 必须只有这一个 decoded unit。
- OOXML Raw Unit 是 binary；每个 ZIP Part 是正 ordinal `ooxml_part`，按 canonical `/URI`
  unsigned UTF-8 bytes 排序，URI 唯一且禁止 case-fold/percent alias。
- Binary Document Root 使用完整 Raw File `NativeBytePosition`；XML/Text 使用
  `NativeSourcePosition`，包含 Source Unit、Raw Byte、Scalar、Line/Column。
- Parent containment 只在同 Source Unit 生效；Root 可挂不同 Part 的结构。
- 跨 Unit Fragment 只允许 `syntax_decode`；`identity` 必须逐 Scalar 等于对应 decoded slice。
- Fragment Role 固定为 `text | cell_value | cached_value | formula | external_target`。

Artifact v2 是 Child-internal Closed DTO，不是 Packaged Public Schema，也不是
`canonical-ir.v2`。

### 单一 OPC / XML Capability

`admit_ooxml_package(source, limits) -> ValidatedOpcPackage` 是 Router 与 Parser 唯一共享
能力：

- EOCD 单盘、Local/Central/Data Descriptor、CRC、Size、Range 全量对账；拒绝 ZIP64、
  encryption、special file、nested archive 与非 STORED/DEFLATED method。
- Entry count/size/ratio/expanded/path、Source Units 与 package-wide XML budgets 有界。
- Part Name 做 NFC、case-fold、percent 与 traversal canonicalization。
- `[Content_Types].xml` 通过 hardened XML 做 semantic selection；comment/string marker 无权。
- `.rels` 形成 closed graph；Internal target 必须 canonical 且存在；External 仅允许冻结类型，
  只返回 metadata，永不解引用。
- Expat 仅收 strict UTF-8/BOM；拒绝 DTD、自定义 Entity、External Entity、PI、XInclude；
  Entity/CRLF 转换标为 `syntax_decode`。

Parser 只能消费该 Capability；不得重新打开 Raw ZIP 或建立第二套 admission。

### Format Profiles

- **CSV**：comma delimiter、double quote、doublequote、CRLF/LF/CR；无 Sniffer/Header 推断、
  不补 ragged row。
- **DOCX**：Document/Heading/Paragraph/List/Table/Row/Cell/Footnote/Endnote；Active embed
  与未冻结修订结构 fail closed。
- **PPTX**：按 Relationship 冻结 Slide 顺序，Shape preorder，DrawingML Table/Notes；EMU
  通过 exact rational 转 milli-point，最终才 half-even；不 clamp。
- **XLSX**：Workbook 顺序、Sheet/Row/Cell、Shared String、Formula/Cached、Merge/Hidden；
  公式只保留文本，绝不执行。

### Internal Result Frame

```text
4-byte big-endian canonical header length
N-byte closed canonical JCS header
8-byte big-endian body length
M-byte Native Artifact body
EOF; trailing bytes forbidden
```

Success 绑定 Format、Artifact Version、Length 与 SHA-256；Failure 必须零 Body。Child 禁止
伪造 Controller-only `PARSER_CANCELLED` 与 `PARSER_SANDBOX_UNAVAILABLE`。

## Stable Errors

| 条件                                             | Error                                  |
| ------------------------------------------------ | -------------------------------------- |
| 非唯一编码、非法 Unicode/ZIP/XML/OOXML 结构      | `ENCODING_AMBIGUOUS` / `INPUT_INVALID` |
| Archive/Package/XML admission 资源超限           | `ARCHIVE_LIMIT_EXCEEDED`               |
| Macro/OLE/Active Embed                           | `ACTIVE_CONTENT_UNSUPPORTED`           |
| 合法但未冻结的结构或非 Native Format             | `FORMAT_UNSUPPORTED`                   |
| Native Node/Fragment/Depth/Artifact 超限         | `RESULT_TOO_LARGE`                     |
| Child-local Byte/Scalar/Line/Geometry 不一致     | `QUALITY_LOCATOR_FAILED`               |
| Parent Closed Artifact 或 Request Binding 不一致 | `PARSER_SCHEMA_MISMATCH`               |
| Timeout/OOM/Cancel/Residual Process              | 复用 C1.2 Sandbox Stable Error         |

## 威胁模型与措施

- **Active Content / Formula Execution**：Macro、OLE、Control 与危险 Embed 拒绝；Formula
  永远作为 source-derived text。
- **SSRF / Local File Read**：无 fetch API；External target 不进入 Part resolution。
- **ZIP/XML Bomb**：压缩比、expanded bytes、Part/Node/Attribute/Text 与 package-wide totals
  均在解压/解析时计数。
- **Path / Relationship Confusion**：NFC/case/percent collision、absolute/traversal、missing
  Part、duplicate ID/source 全部 fail closed。
- **Protocol / Locator 欺骗**：Child 对所有 used Source Unit 重算位置；Parent 再做 Closed
  Artifact、Length、Hash、Format 与 Source binding。
- **Residual Process**：沿用 C1.2 Process-group kill/reap 与 restart fence。

## 已知限制

- PDF Native Parser、MinerU Offline Normalizer 与 C1.4 Canonical IR 尚未实现。
- Native Attribute 尚未投影为最终 Provenance/Source Locator。
- Native Artifact 不可持久化或供 Query 使用。
- Fresh-container 十次 Byte-identical Gate 属于 C1.4；当前覆盖重复解析与真实 Child 回归。

## 变更历史

- **2026-07-14 / C1.3A**：建立 TXT/Markdown/HTML Parser、Artifact v1 与内部 Frame。
- **2026-07-14 / C1.3B**：Artifact 升 v2；加入 CSV、共享 OPC/XML Capability、DOCX、
  PPTX、XLSX；仍保持 MMCP zero-body fail-closed。最终 Config Hash：
  `6251a7a71ec35d7d55e030b8ca1ef49da8995257734a76e8cd6864c25d88d8c3`。
