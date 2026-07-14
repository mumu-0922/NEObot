# Phase 15.2C Offline Parser 与 Canonical IR 实施计划

- 状态：implementation plan locked / runtime disabled
- 日期：2026-07-13
- 上位计划：
  [`phase-15-2c-generation-bound-indexing-plan.md`](./phase-15-2c-generation-bound-indexing-plan.md)
- 当前基线：migration `010` 与 Python durable dark-run 已完成；`011/012` 均未创建
- 本切片：C1 Offline Parser、Canonical IR v2、Source Locator v2、Quality Gate、
  Parent/Child Chunk Harness

> 本文只允许实现确定性的离线 Harness。不得连接 Postgres、Redis、MinIO、MinerU、
> Jina 或公网，不得注册 Production Handler，不得开启 Dispatch。扫描或复杂 PDF 在 C1
> 只返回 `MINERU_REQUIRED`；Provider Adapter、数据库 Staging 与 Runtime Activation
> 仍由后续切片负责。

## 1. 目标、范围与完成定义

### 1.1 目标

用同一份不可信输入，在固定 Parser Build、Config 与 Tokenizer 下，稳定生成：

```text
source bytes
  -> format preflight
  -> isolated native parser
  -> Canonical IR v2
  -> Quality Report v2
  -> Parent/Child Chunk Manifest v2
  -> RFC 8785/JCS logical manifest + SHA-256
```

产物必须可重复、可定位、可删除、可审计；任一 Chunk 都能由有序 Span Fragment 与固定
Joiner 精确重建，并能回溯到 Canonical Block 和原文件位置。

### 1.2 包含

- Magic/Header/Container-first Format Router；
- TXT、Markdown、HTML、DOCX、PPTX、XLSX、CSV 与 Native PDF；
- 只消费 Synthetic/Public Fixture 的离线 MinerU Artifact Normalizer；它不实现 Provider
  Wire、Submit/Poll/Download 或任何网络调用；
- 无网络 Parser Sandbox Protocol、独立 Child、资源限制和稳定错误；
- `canonical-ir.v2`、`source-locator.v2`、`canonical-manifest.v2`；
- Page、Block、Span、Table/Cell、Formula、Reading Flow 与 Provenance；
- Parent/Child/Overlap Chunking 和 Locator Round-trip；
- Golden、Adversarial、Recipe Corpus 与 10 次 Fresh-container Determinism；
- future `012` Payload/Lineage 分离和 Evidence/Citation v2 的兼容要求。

### 1.3 不包含

- 真实 MinerU/Jina 调用、Credential、Wire Freeze 或 Provider 恢复；
- Postgres DDL/DML、migration `011/012`、Outbox Claim 或 Job Handler；
- Passage Embedding、Search Projection、Publish、Purge 或用户 Query；
- 独立图片知识库、Image Embedding、OCR 图片检索；
- 修改旧 Next.js Markdown-only Doc Parse 链；
- 修改已发布 migration `010` 或静默改写 Internal Evidence API v1。

### 1.4 Definition of Done

C1 只有同时满足以下条件才可勾选完成：

1. 所有 Schema 为 Closed Shape，未知字段、重复 JSON Key、非有限数字一律拒绝；
2. Golden Corpus 在 10 个 Fresh Container 中输出 Byte-identical Manifest；
3. 每个 Chunk 通过 Content Rebuild、UTF-8 Boundary、Locator Round-trip 与 Hash Gate；
4. Adversarial Corpus 不联网、不执行宏/公式/脚本、不越界解压、不读取宿主路径；
5. 扫描/复杂 PDF 稳定返回 `MINERU_REQUIRED`，不自动调用 Provider；
6. Runtime Registry、DB、Compose Production Activation 与 migration Head 保持不变；
7. 独立 xhigh Review 达到 `P0/P1/P2 = 0/0/0`。

## 2. 当前基线与阻塞

### 2.1 可复用基线

- migration `010` 已有 Artifact Set、Artifact、Canonical Block、Parent/Child Chunk、
  Block Span、Materialization 与 Lease/CAS 骨架；
- Python Worker 已有 Poll/Rescan、Applied Ledger、Heartbeat、Lease-loss Cancel、Retry/DLQ；
- `DISPATCH_REGISTRY` 与 `JOB_HANDLER_REGISTRY` 当前为空，生产消费保持关闭；
- Phase 15.2C 已锁定原文件 `≤50 MiB`、PDF `≤500` 页及双 Generation Runtime 方向。

这些对象只作为语义参考。C1 不写数据库，也不让 Harness DTO 直接依赖数据库 UUID。

### 2.2 必须先冻结的兼容断层

1. **删除冲突**：`010` 的 Block/Chunk 正文与 immutable Trigger 共存，无法满足 Online
   Payload Purge。
   不回改 `010`；future `012` 必须把 immutable Lineage/Hash 与 deletable Payload 分离。
2. **Locator 不同构**：`010` Locator JSON 和 Internal Evidence API v1 都无法无损表达
   多 BBox、跨页 Span、Offset 单位与坐标基准。C1 采用 v2 Contract；以后通过
   Evidence/Citation v2 Addendum 暴露，禁止静默扩展 v1。
3. **Migration 顺序**：`011` 只允许 Search Winner DDL；Canonical IR、Payload/Lineage、
   Dispatcher 和 Citation 修正只能进入 future `012` Addendum 或更晚迁移。
4. **运行边界**：当前 Worker 容器资源和权限不是不可信文档解析边界。Parser 必须独立
   Sidecar/Child，不得把解析库加载进 Worker 主进程。

## 3. 离线数据流与信任边界

```text
Harness Controller (trusted orchestration, no content parsing)
  -> source stat/hash + fixed Invocation
  -> Unix socket, bounded framed stream
Parser Sidecar (UID 10002, no network/secret/storage/database)
  -> one isolated Child per Invocation
  -> preflight -> route -> parse -> normalize -> validate
  -> canonical candidate bytes + result hash
Harness Controller
  -> independent Schema/JCS/hash verification
  -> quality gate -> deterministic chunking
  -> golden comparison / local test artifact
```

信任裁定：Source、文件名、扩展名、MIME、Archive Entry、XML、Parser Output 与 Fixture
全部是不可信数据。Controller 只信长度、固定 Schema、重新计算的 Hash 和本地编译进镜像的
Config；Sidecar 不持有任何 Authority、Credential 或 Object Key。

## 4. Format Router 与依赖门

### 4.1 路由优先级

路由顺序固定为：

1. 文件 Header/Magic；
2. Container Central Directory、Content Types 与必需 Part；
3. 结构化 Parser Preflight；
4. 声明 MIME；
5. 扩展名仅作一致性检查。

Magic/Container 与 MIME/扩展名冲突返回 `FORMAT_MISMATCH`。Binary Parse 失败不得回退
TXT；OOXML/PDF 失败不得静默换 Parser；未知格式返回 `FORMAT_UNSUPPORTED`。

TXT/Markdown/CSV 是非自描述文本，必须至少有一个 Canonical MIME 或 Extension Hint：

- `.md/.markdown` 或冻结的 Markdown MIME → Markdown；
- `.csv` 或 `text/csv` → CSV；
- `.txt` 或 `text/plain` → TXT，即使正文恰好符合 Markdown/单列 CSV；
- 两个 Hint 指向不同类型 → `FORMAT_MISMATCH`；
- 两者均缺失或任一 Hint 是未登记的 Generic Text Type → `FORMAT_AMBIGUOUS`。

Parser Content Probe 只验证所选文本格式是否成立，不在 TXT/Markdown/CSV 之间猜 Winner。
HTML 只有在 Header/DOM Preflight 自描述或 Hint 明确时进入 HTML；普通 `<` 字符不能升级
格式。这样同一 Bytes 不因 Parser 尝试顺序改变类型。

| 输入          | C1 路线                          | Fail-closed 条件                                   |
| ------------- | -------------------------------- | -------------------------------------------------- |
| TXT           | deterministic decoder            | NUL/binary score、编码歧义、非法序列               |
| Markdown      | source-aware Markdown AST        | raw HTML 仍交 hardened HTML policy，不执行扩展脚本 |
| HTML          | hardened DOM-to-block            | DTD/Entity/XInclude、外部获取、脚本执行            |
| DOCX          | OOXML structure parser           | 宏、OLE、缺失主 Part、危险 Relationship            |
| PPTX          | OOXML slide/shape parser         | 宏、OLE、非法 Slide/Shape 引用                     |
| XLSX          | OOXML sheet/cell parser          | 宏、外部 Workbook、超限 Cell/Shared String         |
| CSV           | stdlib-compatible dialect parser | 编码/方言歧义、行列或字段超限                      |
| Native PDF    | native text/layout parser        | 加密、扫描、混排或复杂阅读序风险                   |
| 扫描/复杂 PDF | route outcome only               | 返回 `MINERU_REQUIRED`，C1 不联网                  |
| 独立图片      | unsupported                      | 返回 `FORMAT_UNSUPPORTED`                          |

### 4.2 文本编码

检测顺序固定为 BOM → strict UTF-8 → Profile 中冻结的候选编码集合。首版候选至少覆盖
`gb18030`，其余编码及置信差阈值必须由 Corpus 评测后写入 Config Hash。无法唯一判定时
返回 `ENCODING_AMBIGUOUS`，不得用系统 Locale 或 `errors=replace` 猜测。

### 4.3 依赖与 License Gate

候选依赖必须先通过版本、License、CVE、SBOM、Determinism 和 Fixture Gate 才能进入
锁文件。推荐评估边界：

| 能力      | 候选                                         | 裁定                                                |
| --------- | -------------------------------------------- | --------------------------------------------------- |
| Markdown  | `markdown-it-py` 或等价 source-aware parser  | 固定插件集，禁运行时插件发现                        |
| HTML/XML  | `lxml`/`defusedxml` 或等价 hardened parser   | DTD、Entity、XInclude、网络全部关闭                 |
| DOCX/PPTX | `python-docx`/`python-pptx` + 受限低层 OOXML | 高层 API 缺失的 Locator 必须从受限 XML 补齐         |
| XLSX      | `openpyxl` + 受限低层 XML                    | read-only；公式与 cached value 分开保留但不执行     |
| PDF       | `pypdf + pdfplumber` 优先评估                | 只处理 Native-safe 子集；复杂页转 `MINERU_REQUIRED` |
| PyMuPDF   | 暂不批准                                     | AGPL/商业授权风险未审完，不得直接锁入生产镜像       |

锁定报告必须记录 Package/Version/Wheel Hash/License/Transitive License/Image Digest；
License 不清、版本漂移或无法生成 SBOM 均阻断实现晋升。

## 5. Archive、XML、OOXML 与 PDF 安全

### 5.1 通用输入上限

- 原文件：`50 MiB`（`52,428,800` Bytes）；PDF：`500` 页；
- OOXML Archive Entry：最多 `10,000`；总展开：`512 MiB`；单 Entry：`64 MiB`；
- 最大压缩比：`100:1`；Archive 嵌套解析深度：`0`；
- Entry Path：UTF-8 `≤512` Bytes；拒绝绝对路径、Drive Prefix、`..`、NUL、Symlink；
- ZIP Entry Name 必须由 UTF-8 Flag/合法 UTF-8 唯一解码；拒绝 Backslash、Leading Slash、
  Empty/Dot Segment、NUL 和非 NFC。映射 OPC Part URI 时加一个 Root `/`，Percent Triplet
  统一 Uppercase，只解码 RFC 3986 Unreserved；Percent-encoded Separator/NUL/Dot Segment
  拒绝。Identity 按 Case-sensitive Canonical URI，同时额外拒绝 ASCII Case-fold Collision，
  消除跨平台/Library First-or-last 歧义；
- 拒绝重复 Entry/Canonical-equivalent Part URI、加密 Entry、重复
  `[Content_Types].xml` 或同一 Source Part 的重复 Relationship Part；
- Bit 3 未设置时，Central Directory 与 Local Header 的 Name、Flags、Compression Method、
  CRC、压缩/展开 Size 必须一致；Bit 3 设置时，Local CRC/Size 只允许规范的 `0` 或 ZIP64
  Sentinel，流式解析唯一 Data Descriptor，并将其 Effective CRC/Size 与 Central Directory
  对账。两种路径都必须用实际流式 Bytes/CRC 复核；Descriptor 缺失、重复、歧义或尾随
  数据一律拒绝；
- XML 总量、节点数、深度、Attribute 数、Text Bytes 必须有独立 Config 上限；
- Cell、Shared String、Shape、Block、Table、Asset、Chunk 和 Canonical Bytes 均有硬上限。

这些值属于 `parser-resource-profile.v1` 并进入 Config Hash。Corpus 压测只能收紧或通过
新 Profile 版本修改，不能靠未记录的环境变量漂移。

### 5.2 XML/HTML

- 禁止 DTD、General/Parameter Entity、XInclude、Schema Fetch 和任何外部网络；
- 禁止脚本、事件处理器、CSS/Font/Image Fetch；
- External Link 只可作为 non-dereferenced Provenance，绝不自动访问；
- XML/HTML Parse Error 不得用正则或 TXT fallback 掩盖。

### 5.3 OOXML

- `.docm/.xlsm/.pptm`、`vbaProject.bin`、Macro-enabled Content Type 返回
  `ACTIVE_CONTENT_UNSUPPORTED`；
- Embedded OLE/Package 不执行、不递归解析；作为 `non_indexable` Asset 记录或按
  Policy 拒绝；
- Relationship Target 必须 Canonicalize 并限制在 Container 内；External Target
  永不读取；
- OPC Part Name 只接受上述唯一 Canonical URI；Relationship `Id` 在各 Part 内唯一，解析
  选择不得依赖 ZIP first/last；
- DOCX 保留 Heading、Paragraph、List、Table、Footnote/Endnote 的结构与 OOXML Path；
- PPTX 保留 Slide `0-based` Ordinal、Shape Identity、Group/Placeholder、Notes Policy；
- XLSX 不执行公式。Formula 与 Cached Value 分字段保存；Merged Cell、Hidden Row/
  Column/Sheet、Number Format 和 Cell Range 进入结构/Provenance。隐藏内容默认
  `non_indexable=true`，但不得无痕丢弃。

### 5.4 PDF

Preflight 先验证 Header、XRef/Objects、Encryption、Page Count 与对象资源上限。Native
路线只接收每个非空页都有可提取文本、阅读序可确定且不存在高风险复杂布局的文件。
以下任一情况返回 `MINERU_REQUIRED`：

- 全扫描或混合扫描页；
- 多栏/浮动对象/表格/公式使 Native Reading Order 无法通过 Quality Gate；
- 字形映射异常、文本覆盖不足、BBox/Page 不一致；
- Native Parser 对同一 Build/Input 不能产生稳定结果。

Encrypted、损坏或超限 PDF 分别返回稳定的 Unsupported/Invalid/Limit Error，不得送
MinerU 规避本地 Admission。Page 使用 `0-based`；先应用页面 Rotation，再产生
Top-left `xyxy` BBox。

### 5.5 Geometry Canonicalization

几何计算禁止 Binary Float。PDF Number 先按原 Decimal Lexeme 转任意精度 Rational；Page
以有效 CropBox 为可见边界，`UserUnit`、Box Translation 与 `0/90/180/270` Rotation 组成
一个精确 `3x3` Affine Matrix。对 Rectangle 四角变换后取 `min/max`，转换为 milli-point
时只在最后一步执行 **round-half-to-even**：

```text
milliPoint = roundHalfEven(pointRational * 1000)
```

PPTX 先递归应用 Group `off/ext/chOff/chExt` 的有理 Affine Transform；EMU 到
milli-point 使用精确比例 `milliPoint = roundHalfEven(EMU * 10 / 127)`。Shape Rotation/
Flip 在 Parent Group Transform 前按 OOXML 规范顺序组合。任何 Library Float 必须回到原始
Token/EMU 重算，不能进入 Hash。

BBox 语义为 Top-left Origin 的半开区域，文本/Shape 要求
`0 <= x1 < x2 <= width`、`0 <= y1 < y2 <= height`；Line/Point 使用独立 Geometry Kind，
不得伪造零面积 BBox。禁止 Clamp；Rounding 后越界即 `QUALITY_LOCATOR_FAILED`。Golden
Fixture 必须覆盖负 Origin CropBox、四种 Rotation、Fractional PDF Number、Nested Group、
Flip 与边界 Rounding。

## 6. Sandbox、IPC 与资源策略

### 6.1 Sidecar 基线

- Sidecar UID/GID：`10002:10001`；Worker 为 `10001:10001`；
- `read_only: true`、`cap_drop: [ALL]`、`no-new-privileges`、Core Dump `0`；
- 无 Network、DB、Redis、MinIO、Provider Key、Host Mount、Docker Socket；
- IPC 使用仅双方可见的 tmpfs-backed Named Volume 与 Mode `0660` Unix Socket；
- 每个 Sidecar 同时只执行一个 Invocation，等待队列长度为 `1`；额外请求返回
  `PARSER_BUSY`，不得在 Sidecar 内并行争抢 RAM/PID；
- 每个 Invocation 启一个独立 Child，并设置 `RLIMIT_AS/CPU/NPROC/NOFILE/FSIZE/CORE`、
  Wall Deadline。Sidecar 是 Container PID Namespace 的 PID 1，并设置
  `PR_SET_CHILD_SUBREAPER`；Container Baseline Seccomp 拒绝 `unshare/setns/ptrace` 与
  Namespace Clone，但只对可信 Supervisor 保留 `setpgid`；
- Child Bootstrap 时序固定：Supervisor `fork` 后，在任何 Source Bytes/Parser Module 进入
  Child 前，Parent 与可信 Child Bootstrap 都执行/确认 `setpgid(childPid, childPid)`；Parent
  取得 `pidfd` 并用 `getpgid` 验证独立 Group，双方通过仅一次 Handshake Pipe 后，Child 才
  安装更严格的 Per-child Seccomp 并接收输入；
- Per-child Seccomp 拒绝 `setsid/setpgid/unshare/setns/ptrace`；`clone3` 完全以 `ENOSYS`
  拒绝，促使受支持 Runtime 回退；`clone` 只允许经参数 Mask 验证的 Thread/Fork Flags，
  任何 Namespace Flag 拒绝。Filter 跨 `exec` 保持；握手失败绝不发送 Source，立即用
  `pidfd_send_signal` 清理；
- Container/Per-child Seccomp Profile Source/Compiled Hash、安装阶段与 Kernel
  Compatibility Matrix 必须进入 Parser Config/Image Manifest；
- Cancel/Timeout 先杀 Child Process Group，再枚举/终止/回收该 Invocation 的全部 Descendant
  PID。恢复 Ready 前，`/proc` 必须只剩 Supervisor 与当前允许的固定线程；若无法证明零
  残留，Sidecar 必须退出并由 Compose 重启，不能接下一 Job；
- Child 先触发 `RLIMIT_AS`，其上限必须低于 Sidecar Cgroup Memory 并留出经实测冻结的
  Supervisor Reserve；
- 日志只允许 Invocation ID、Format、Size Bucket、Duration、Error Code、Hash Prefix；
  禁止文件名、正文、Locator、Archive Entry、Object Key 和完整 Hash。

起始资源候选为 `1 CPU / 768 MiB / 64 PID / 256 MiB tmpfs`，标记为 `[unverified]`；
必须以最坏 Golden/Adversarial Corpus 实测后冻结。当前 Worker 的 `1 CPU / 448 MiB`
不能作为 Parser 运行空间。若 Library 绕过 `RLIMIT_AS` 触发 Container-level OOM，Sidecar
可被重启并将该 Job 记为 `PARSER_SANDBOX_UNAVAILABLE`，但独立 Worker 必须存活。验收同时
覆盖 `setsid/double-fork/fork-bomb` 后零残留、“Child 超限后 Sidecar 仍 Ready”和“Sidecar
被强杀后 Worker/下次重启可恢复”，不作无法由共享 Cgroup 保证的绝对承诺。

### 6.2 Framed Protocol v1

每个 Frame 的 Byte Layout 固定为：

```text
4 bytes  magic ASCII "MMCP"
1 byte   protocol major = 1
1 byte   frame type = 1 request | 2 response
2 bytes  flags/reserved = 0
4 bytes  unsigned big-endian JCS header length (max 16 KiB)
8 bytes  unsigned big-endian body length
N bytes  UTF-8 JCS header, no BOM
M bytes  body
```

Request Header 使用 Closed JCS JSON，后接精确长度 Source Bytes：

```text
invocationId
declaredMime          # optional lowercase canonical media type, no parameters
declaredExtension     # optional lowercase ASCII suffix including leading dot, no path
parserConfigHash
expectedSourceBytes
expectedSourceSha256
requestBindingHash    # JCS hash of all header fields except itself + source hash
deadlineUnixMillis
maxResultBytes
```

Response Header 后接精确长度 Canonical Candidate Bytes：

```text
invocationId
outcome: success | route_required | failure
canonicalSchemaVersion
resultBytes
resultSha256
stableErrorCode
```

Router 的 Format 是解析结果，不由调用者声明；Sidecar 必须把 Magic/Container 与两个
Hint 独立对账。`success` 必须有非空 Body、Schema Version、Bytes/Hash 且无 Error；
`route_required` 必须零 Body、空 Schema/Hash 且 Error 只能为 `MINERU_REQUIRED`；
`failure` 必须零 Body、空 Schema/Hash 且有非 `MINERU_REQUIRED` Error。任何禁止字段组合
返回 `PROTOCOL_INVALID`。

Header/Body 各有独立上限。短读、超长、Invocation 错配、Deadline 过期、Schema/Hash
不符、Reserved 非零或尾随 Bytes 均 Fail Closed。Controller 必须独立重算 Source/Result/
Binding Hash；Sidecar 声明不能作为证明。

`PARSER_CANCELLED` 与 `PARSER_SANDBOX_UNAVAILABLE` 是 **Controller-synthesized local
outcome**，不是伪造的 Response Frame：Caller Cancel 时 Controller 关闭 Socket、触发
Sidecar 清理并本地记录 Cancel；EOF/Connection Reset/Sidecar Exit 且无 Caller Cancel 时
映射为 Sandbox Unavailable。两者都没有 Result Body/Hash，禁止 Stage。若同时发生，以
Controller 已持久观察到的 Caller Cancel Fence 优先；普通 Parser Failure 只能来自通过
Closed Wire 校验的 Response。

## 7. Canonical IR v2

### 7.1 顶层 Closed Shape

```text
CanonicalDocumentV2
  schemaVersion = "canonical-ir.v2"
  source
  normalizationProfile
  normalizationMapRef
  parser
  textBuffer
  pages[]
  readingFlows[]
  blocks[]
  tables[]
  formulas[]
  assets[]
  provenance[]
```

顶层与所有子对象均 `additionalProperties=false`。禁止无序 JSON Object 充当 Entity
Map；必须转换为有显式 Ordinal/ID 的数组。语义数组保留语义顺序，Set-like 数组使用 §9.1
逐类比较器；不得依赖语言 Map、Filesystem、Archive 或数据库插入顺序。

### 7.2 Text Buffer 与 Normalization

Canonical Text 固定：

- 解码后使用 Unicode NFC；禁止 NFKC、繁简转换、大小写折叠与空白折叠；
- `CRLF`/`CR` 统一为 `LF`；保留行内空格、Tab 和 Code Whitespace；
- 非法 Unicode Scalar、Unpaired Surrogate 与 NUL 拒绝；禁止 `errors=replace`；
- Top-level Block 按 Reading Flow 排列，以固定 `"\n\n"` 分隔；Block 内 Renderer
  按 Block Type 固定；
- Offset 是 UTF-8 Byte、`0-based`、半开区间 `[startByte,endByte)`；起止必须落在
  UTF-8 Code Point Boundary；
- `textBufferSha256` 对实际 UTF-8 Bytes 计算，不对语言 Runtime String 表示计算。

每个 Block 的 `textRange` 都必须是 `textBuffer` 的精确 Slice；重复保存的 Text 与 Slice
不一致即 `PARSER_SCHEMA_MISMATCH`。

Parser 同时生成独立 Closed Artifact `normalization-map.v1`；IR 只保存其 Schema、Bytes、
SHA-256 Ref，Logical Manifest 独立保存 Map Bytes/Hash/Source-unit/Segment Count，不能把它
藏在未计数的 `normalization` Object 中。Map Shape 固定为：

```text
NormalizationMapV1
  schemaVersion
  textBufferBytes/textBufferSha256
  sourceUnits[]
    sourceUnitOrdinal, opaqueSourceUnitId, kind
    sourceBytes/sourceSha256, displayMetadataPayloadRef
  segments[]
    segmentOrdinal, canonicalStartByte, canonicalEndByte
    transform                 # closed discriminated union
      kind = identity | newline_fold | nfc_compose | syntax_decode |
             renderer_insert
  aggregateHash
```

`aggregateHash` 不进入自身 Preimage，精确定义为：

```text
normalizationMapPayload = {
  schemaVersion, normalizationProfileHash,
  textBufferBytes, textBufferSha256,
  sourceUnits, segments
}
aggregateHash = SHA256(
  ASCII("mm-chat.normalization-map.v1\n") || JCS(normalizationMapPayload))
```

`sourceUnits/segments` 必须已通过各自比较器和 Closed Union 校验。任何 Runtime Metadata、
`aggregateHash` 字段本身或 IR `normalizationMapRef` 都禁止进入 Preimage。

Segment Union 的必填/禁填固定：

| `kind`            | 必填字段                                                           | 禁填字段                                     |
| ----------------- | ------------------------------------------------------------------ | -------------------------------------------- |
| `identity`        | `sourcePositions` non-empty                                        | Recipe、Subsegment、Structure/Renderer 字段  |
| `newline_fold`    | `sourcePositions` non-empty、`newlineRecipeId`                     | Subsegment、Structure/Renderer 字段          |
| `nfc_compose`     | `sourcePositions` non-empty、`normalizationForm="NFC"`、Cluster ID | Recipe、Subsegment、Structure/Renderer 字段  |
| `syntax_decode`   | `sourcePositions` non-empty、`recipeId`、`recipeProfileHash`       | Structure/Renderer 字段                      |
| `renderer_insert` | `structureRef`、`rendererRuleId`、`rendererProfileHash`            | `sourcePositions`、Recipe/Normalization 字段 |

`renderer_insert.structureRef` 是 Closed `{ownerSeedId,structureKind,structureOrdinal}`，只能
引用 ID DAG 的 B 阶段 Seed；禁止引用 Logical Block/Flow/Chunk ID、Provenance ID 或 Map Hash。

`SourcePositionV1` 是 Closed Union，每项都有连续 `positionOrdinal`：

```text
text_position:
  opaqueSourceUnitId, rawByteStart/rawByteEnd
  decodedScalarStart/decodedScalarEnd
  startLine/startColumn/endLine/endColumn
page_geometry_position:
  opaqueSourceUnitId, pageIndex, fragmentReadingOrdinal
  bboxMilliPoint[x1,y1,x2,y2]
```

`text_position` 按 `(sourceUnitOrdinal,rawByteStart,rawByteEnd,decodedScalarStart,
decodedScalarEnd)`；`page_geometry_position` 按
`(sourceUnitOrdinal,pageIndex,fragmentReadingOrdinal,y1,x1,y2,x2)`。Position 先按语义 Source
Order 排列并写连续 Ordinal；Validator 用对应比较器证明该顺序，完全相同或重叠且无
Transform 解释的 Position 拒绝。一个 Segment 不混合两种 Position Kind；Native PDF Text
必须使用 Geometry Variant，不能虚构 Raw/Scalar Offset。

`syntax_decode.subsegments` 可缺省；存在时每项固定为
`ordinal,relativeCanonicalStart/End,sourcePositions[]`，对 Parent Segment Canonical Range
Gapless/Non-overlap/Exact-cover，且禁止递归 Subsegment。Subsegment Positions 按 Ordinal
连接后，使用“同 Source Unit/Kind 且相邻 Text Range 可合并、Geometry 不合并”的 Canonical
Coalesce 规则，必须 Byte-identical 等于 Parent `sourcePositions`；它们不能增加、遗漏、
重排或扩大 Parent Source Mapping。缺省表示整个 Segment 原子。每个 Union Variant 的 JCS
Hash Envelope 只含该 Variant 允许字段；出现禁填字段、空 `sourcePositions`、重复 Position
或未知 Recipe/Rule Hash 即 `PARSER_SCHEMA_MISMATCH`。

Map 把每段 Canonical UTF-8 Byte Range 映射到一个或多个 Source Unit Position，并显式标记
`identity | newline_fold | nfc_compose | syntax_decode | renderer_insert`。`syntax_decode`
覆盖 HTML/XML Entity、Markdown Escape/Link Text、CSV Quote、OOXML Shared String/Text
Extraction 等 Parser Syntax Transform，必须绑定 Closed `recipeId + recipeProfileHash`；若
能提供精确子映射则保存 `subsegments[]`，否则整个 Transform Segment 原子化，绝不能误标
`identity`。Source Unit 是 Raw File 或 Container 内解压后的 Part；Opaque ID 按
`SHA256(domain + sourceHash + kind + sourceUnitOrdinal + sourceUnitBytesHash)` 生成。同 Bytes
的不同 Part 由 Ordinal 区分；Raw File 固定 Ordinal `0`，OOXML Part 按 §5.3 Canonical OPC
URI 的 **unsigned UTF-8 Byte Lexicographic Order** 排序，禁止按 Unicode Scalar、Locale、
Filesystem 或 JCS UTF-16 String Order。Display Name/URI 只放可删除 Payload Resolver。
Text-like Source Unit 的 `text_position` 固定包含：

`sourceUnitKindRank` 固定为 `raw_file=0, ooxml_part=1,
synthetic_mineru_artifact=2`；Canonical Unit Key 分别为空 Bytes、Canonical OPC URI UTF-8
Bytes、Synthetic Role Rank (`layout=0,middle=1`)。新增 Unit Kind 必须升级 Schema/Profile，
不能插入旧 Rank。

```text
rawByteStart/rawByteEnd                    # 原始或解压后 Unit Bytes
decodedScalarStart/decodedScalarEnd        # 解码后、Normalization 前 Unicode Scalar Index
startLine/startColumn/endLine/endColumn    # 0-based，Column 以 Unicode Scalar 计
```

`Char` 在本 Contract 中只表示 Unicode Scalar，不是 UTF-16 Code Unit、Grapheme 或 Byte。
NFC 的多对一、一对多以及 CRLF→LF 均保留映射 Segment；`renderer_insert` 必须引用产生它的
结构节点，不伪造 Raw Offset。TXT/Markdown/HTML 必须提供 Raw Byte + Scalar + Line/Column；
OOXML 对解压 Part 提供同样 Position；PDF 无可靠字符映射时使用上述
`page_geometry_position`，不得虚构 Raw Offset。

Segments 按 Canonical Range 排序、Ordinal 连续，并对整个 Text Buffer **gapless、无重叠、
exact-cover**；每个 Source Position 必须落在已登记 Source Unit Bound 内。Document-level
Block Text Anchors 同样对所有非 Separator Text Exact-cover，并与 Map Segment Boundary/
Source View 一致；Renderer Separator 由 `renderer_insert` Segment 覆盖。
若 Text Buffer 真正为空，`segments=[]` 是唯一合法表示，Quality Report 必须证明 Empty
Source/Empty Structural Content；Page/Asset 等位置只能使用 Structural Anchor。

只有 `identity` Segment 可在任意 UTF-8/Unicode Scalar Boundary 线性切分，并精确推导
Raw/Scalar/Line Position；`newline_fold` 只可在输出 LF 两侧切分；`nfc_compose` 只可在
冻结的 Normalization Cluster 边界切分；`syntax_decode` 只可沿验证过的 `subsegments[]`
切分，否则整体原子；`renderer_insert` 只可在 Renderer 声明的合法边界切分。实现若不能
提供普通长段落所需合法切点，Quality Gate 失败，不得把整个大 Segment 当作不可分块的
默认。

### 7.3 Page、Reading Flow 与 Block

Page 至少包含 `pageIndex`、整数尺寸、Rotation 与 Page-level Source Locator。所有 Page/
Slide Index 均为 `0-based`。PDF 坐标使用整数 milli-point：`1 point = 1000`，禁止 Float
进入 Canonical JSON/Hash。所有 Confidence 固定为整数 Basis Point `0..10000` 或
`null=unknown`，禁止 Decimal String 双表示。

Reading Flow 使用两阶段 ID：先由 Source/Parser Profile/Flow Ordinal/Geometry 生成
`flowSeedId`，Block 只引用该 Seed；Block IDs 完成后再由
`flowSeedId + orderedLogicalBlockIds` 生成 `logicalFlowId`。它是显式有序 Block ID 列表，
不以 JSON 出现顺序暗示布局。Block 至少包含：

```text
logicalBlockId, ordinal, blockType, parentBlockId
headingPath[], flowSeedId, textRange
locatorSet, structureRef, confidence, flags
sourceSpanHash, contentHash, provenanceRefs[]
```

`blockType` 首版闭合集合：`heading | paragraph | list | list_item | quote | code |
table | formula | caption | footnote | header | footer | page_break | asset_ref`。
Header/Footer 可留作 Provenance，但默认 `nonIndexable=true`；不得静默删除。

Provenance 使用 C4 阶段 Closed Shape：

```text
provenanceId, provenanceOrdinal, targetKind
targetOwnerSeedId, provenanceKind, sourceUnitRef
payloadRef, derivationProfileHash, provenanceHash
```

`provenanceHash` 排除自身与 `provenanceId`，按
`SHA256(ASCII("mm-chat.provenance.v1\n") || JCS(other fields))` 计算；`provenanceId` 再按
Domain `mm-chat.provenance-id.v1` 对 `provenanceHash + targetOwnerSeedId + ordinal` 计算。
Block 的 `provenanceRefs[]` 只引用已完成 C4 `provenanceId`。Provenance 不含最终 Node ID，
Source-derived String 只通过 C1 已完成的 deletable `payloadRef` 关联。

### 7.4 Table、Cell、Formula 与 Asset

- Table 保存 Row/Column Count、Cell Grid、Row/Column Span、Header Flag、Reading Order；
- Cell 保存位置、Span、Text Range、Source Locator 和独立 Hash；
- Formula 保存原始表示、Canonical LaTeX（若可靠）、Source Locator、Confidence；
- CSV/XLSX Table Renderer 必须固定分隔/转义规则；公式绝不求值；
- 内嵌图片只保存尺寸、媒体类型、Content Hash、Source Locator 与
  `nonIndexable=true`，不做 OCR/Image Embedding；
- OCR/Provider 派生字段在未来 Adapter 中必须标 `derived=true` 并绑定 Model/Build；C1
  Native Output 不得伪造 OCR Confidence。

## 8. Source Locator v2

### 8.1 Anchor 模型

`source-locator.v2` 把 Text 与 Structural Anchor 分开，并把“一个 Source Fragment 的
多种坐标视图”收在同一 Fragment 下：

```text
LocatorSetV2
  version = 2
  textAnchors[]             # may be empty; canonical ranges
    anchorOrdinal           # contiguous, starts at 0
    canonicalStartByte/canonicalEndByte     # non-empty, UTF-8 boundary
    sourceFragments[]        # non-empty, source order
      fragmentOrdinal        # contiguous, starts at 0
      views[]                # non-empty, ordered by View Kind Rank
        kind + closed kind-specific payload
  structuralAnchors[]       # may be empty; non-text Page/Asset/Shape/etc.
    anchorOrdinal
    nodeKind/ownerSeedId/structureOrdinal
    sourceFragments[]        # same Fragment/View shape
  aggregateHash
```

两个数组不能同时为空。Text Anchor 用于有 Canonical Text 的 Block/Span/Chunk；其 Range
严格递增且不重叠。“Overlap”只在不同 Chunk 复用 Anchor/Subanchor 时成立。Structural
Anchor 用于空页、Page Break、Asset、Shape 等无 Text Range 对象，不得进入 Chunk，也不得
伪造零长度 Text Range。

`sourceFragments[]` 表示按 Source Reading Order 排列的不同来源区域；同一 Fragment 的
`views[]` 是该区域的共指坐标，例如 Raw Text Position + OOXML Path + Sheet Range，View
之间没有 Source Order。不同坐标空间不定义几何 Overlap。

View Union 首版包括：

```text
source_text_position(
  opaqueSourceUnitId, rawByteStart, rawByteEnd,
  decodedScalarStart, decodedScalarEnd,
  startLine, startColumn, endLine, endColumn)
page_region(pageIndex, bboxMilliPoint[x1,y1,x2,y2])
slide_shape(slideIndex, opaqueShapeId, optional bboxMilliPoint)
sheet_range(opaqueSheetId, startCell, endCell)
ooxml_path(opaqueSourceUnitId, canonicalXPathPayloadRef)
derived_structure(structureKind, opaqueStructureId)
```

Source Unit/Sheet/Shape Identity 是 Parser 生成的 Opaque ID；Display Name、Part Name、XPath
String 与 External Target 是可删除 Payload，不进入 immutable Lineage 白名单。Line、Page、
Slide 均 `0-based`；Range 均半开。Cell 使用 uppercase A1 Canonical Range；BBox 遵循 §5.5。

### 8.2 排序、裁剪与跨页

Text Anchor 比较器固定为 `(canonicalStartByte, canonicalEndByte)`；Structural Anchor
比较器固定为 `(structureOrdinal, nodeKindRank, ownerSeedId)`。各自 `anchorOrdinal` 只
验证排序后的连续性，不打破 Duplicate；重复 Range/Structure 直接拒绝。Source Fragment
保持 Parser 已证明的 Source Order，并以连续 `fragmentOrdinal` 固定。View 按以下 Kind
Rank 排序；同一 Fragment 内同 Kind 最多一个：

```text
source_text_position=0, page_region=1, slide_shape=2,
sheet_range=3, ooxml_path=4, derived_structure=5
```

一个 Anchor 可含多个 Page Source Fragment；跨页、双栏或表格不得用一个大矩形覆盖中间
无关内容。

当 Chunk 只引用 Block 子区间时，必须在 Normalization Map 的合法 Segment Boundary 上裁剪
Text Anchor，重新生成精确 Source Fragment/View、连续 Ordinal 与 Locator Hash；不得
复用整个 Block 的 Locator Hash 伪装成子区间。无法无损裁剪时，该位置不是合法 Chunk
Boundary；Structural Anchor 永不参与该过程。

Text Anchor 的 `source_text_position/page_region` View 必须由覆盖该 Range 的
Normalization Map `text_position/page_geometry_position` 通过唯一、版本化 Projection
函数生成，并在 Canonical Bytes 上完全一致；Locator 不得维护第二份可漂移的 Source
Mapping。

`aggregateHash` 是 Locator Version + 有序 Anchor/Fragment/View JCS 的 SHA-256。Round-trip
测试必须验证：Canonical Subrange → Source Fixture → 预期 Block/Cell，以及 Chunk →
Subanchor → Source Fragment/View → Source 的完整链。

### 8.3 v1 兼容边界

- 不修改 Internal Evidence API v1 的 `SourceLocator` Union；
- 单 Text Anchor/Fragment/View 且能无损降级时，可由 future Adapter 生成 v1 View，但它不是
  Canonical；
- 多 Anchor/Fragment/View、Structural Anchor、跨页或 Offset-unit-sensitive Locator 只能
  进入 Evidence API v2；
- Citation v2 必须绑定 Locator Set Hash、Source Span Hash、Document Version、
  Materialization、Generation 与 Revision；不能把内部 UUID/Hash 直接当公开 Citation ID。

## 9. Canonical JSON、Hash 与 Logical ID

### 9.1 Canonical Bytes

- JSON 序列化遵循 RFC 8785/JCS；输入 Parser Output 必须先严格解析再重新 Canonicalize；
- Hash 算法固定 SHA-256，小写 64 Hex；
- Hash Envelope 必须含 Schema Version、Normalization Policy、Parser Build、Config Hash；
- Float/Decimal String 禁止进入 Hash Shape；所有 JSON Integer 必须位于 I-JSON 可互操作
  范围 `[-(2^53-1), 2^53-1]`，Confidence 只用 `0..10000` Integer 或 `null`；
- 时间、Lease、Object Key、DB UUID、插入顺序、临时 Provider ID 与运行路径不得进入
  Logical Manifest。

数组比较器固定如下；未列出的数组必须在 Schema 中声明“semantic order”或新增版本，不能
临时排序：

```text
pages:          pageIndex
sourceUnits:    sourceUnitKindRank, unsigned UTF-8 canonical-unit-key bytes
readingFlows:   flowOrdinal, flowSeedId
blocks:         readingFlowOrdinal, ordinal, logicalBlockId
tables:         owningBlockOrdinal, tableOrdinal, logicalTableId
cells:          rowIndex, columnIndex, rowSpan, columnSpan
formulas:       owningBlockOrdinal, formulaOrdinal, logicalFormulaId
assets:         owningBlockOrdinal, assetOrdinal, logicalAssetId
provenance:     targetKindRank, targetOwnerSeedId, provenanceOrdinal
```

Heading Path、Reading Flow Block IDs、Anchor/Source Fragment/View、Span Fragment 和 Joiner 都是
Semantic Order，不得另排。验收必须通过 RFC 8785 官方 Test Vectors，并由 Python、Go、
JavaScript 对同一 Fixture 产生 Byte-identical Canonical JSON/SHA-256；任一 Runtime 不一致
即 `PARSER_NONDETERMINISTIC`。

### 9.2 Logical ID

C1 使用 Deterministic Logical ID，不使用数据库 UUID：

```text
logicalBlockId = SHA256(sourceHash + parserProfileHash + blockOrdinal + blockEnvelope)
textSourceSpanHash = SHA256(sourceHash + ordered text-anchor/fragment/view + canonical text bytes)
structuralSourceSpanHash = SHA256(sourceHash + node kind/owner seed/ordinal
                                  + ordered structural-anchor/fragment/view)
logicalParentChunkId = SHA256(parent domain + JCS(
  chunkKind=parent, logicalFlowId, parentChunkSeedId, parentOrdinal,
  chunkProfileHash, ordered span fragments/joiners, contentHash))
logicalChildChunkId = SHA256(child domain + JCS(
  chunkKind=child, logicalFlowId, parentChunkSeedId, childOrdinal,
  chunkProfileHash, ordered span fragments/joiners, contentHash))
```

上式只表示字段集合，实际输入必须是带 Domain Tag 的 JCS Envelope，禁止字符串拼接。
`sourceSpanHash` 是 Closed Union：有 Text 的 Node 只使用 `textSourceSpanHash`，纯结构 Node
只使用 `structuralSourceSpanHash`；不能对空 Text/空 Anchor Hash。Chunk 只能使用 Text
Variant。`blockEnvelope` 明确排除自身 Logical ID 和任何引用它的反向边，避免循环 Hash。
Parent Domain 固定 `mm-chat.parent-chunk-id.v1\n`，Child Domain 固定
`mm-chat.child-chunk-id.v1\n`；`chunkKind`、E-stage `logicalFlowId`、Ordinal 与
`parentChunkSeedId` 均必填。即使
Parent/Child Content/Span 完全相同，也必须得到不同 Logical ID。

所有 Opaque ID 使用同一 JCS/Domain-separation 规则，Display Text 永不编码进 ID：

```text
opaqueSourceUnitId: source hash + unit kind/ordinal + unit bytes hash
opaqueSheetId:      source hash + workbook unit ID + sheet ordinal
opaqueShapeId:      source hash + slide index + canonical shape-tree ordinal path
ownerSeedId:        source hash + parser profile + node kind/structural path/ordinal
opaqueStructureId:  source hash + structure kind rank + owner seed ID + ordinal
sourceUnitPayloadRef: source unit ID + payload kind rank + ordinal + payload hash
nodePayloadRef:     source hash + payload kind rank + owner seed ID + ordinal + payload hash
```

Sheet Ordinal 来自 Workbook 关系顺序，Shape Path 来自经验证的 Shape Tree Preorder；两者必须
与对应 Source Fragment/View 一致。每种 Envelope 的字段顺序、Kind Rank 与 Golden Vector
在 C1.1 冻结；缺 Envelope 的新 Opaque Kind 必须升级 Schema Version。

ID DAG 固定为六阶段并在 C 内再分层，只允许向后引用：

```text
A  Source Unit IDs + Source-unit Payload Refs
B  flowSeedId + ownerSeedId（只依赖 A、Source Hash、Parser Profile、路径/Ordinal）
C1 opaqueSheet/Shape/Structure IDs + Node Payload Refs（只依赖 A+B）
C2 Locator Anchor/Fragment/View（只依赖 A+B+C1）
C3 text/structural sourceSpanHash（只依赖 A+B+C1+C2）
C4 provenance IDs/Hashes（只依赖 A+B+C1+C3）
D  logicalBlock/Table/Cell/Formula/Asset IDs（依赖 B+C2+C3+C4 与已完成 Parent D）
E  logicalFlowId（依赖 flowSeedId + ordered completed Block D IDs）
F  Parent/Child Chunk IDs（依赖 D+E 与 Chunk Profile）
```

Structural Anchor 使用 B 阶段 `ownerSeedId`，绝不引用最终 D 阶段 Logical ID。Locator 中的
`payloadRef/opaqueStructureId` 必须已在 C1 完成，C3 只能消费 C2 Locator，不能与 Locator
同阶段互引。Provenance 只在 C4 消费既有 Seed/Ref/Hash，Node 只能向 C4 引用，不能反向。
Block 只引用 B 阶段 `flowSeedId`，不引用 E 阶段 `logicalFlowId`；Reading Flow
在 E 保存最终 ID 与有序 Block IDs。Parent 引用只能指向已完成的 Ancestor；Descendant/
Reverse Edge 不进入 Ancestor Hash。Schema Validator 必须构建 DAG 并拒绝 Cycle/Forward
Reference。Runtime 在 future `012` Staging 时可分配 UUID，但必须保留 Logical ID/Hash 并
验证一一对应。

## 10. Manifest 与 Artifact

### 10.1 Logical Manifest v2

`canonical-manifest.v2` 至少包含：

```text
schema/config/parser/tokenizer profile hashes
source bytes/hash/format
canonical IR bytes/hash
text buffer bytes/hash
normalization map bytes/hash/source-unit/segment counts
page/block/table/cell/formula/asset counts
ordered aggregate hashes for every entity class
quality report bytes/hash/outcome
parent/child/span counts and aggregate hashes
```

Manifest 不含 Created-at、Container ID、Hostname 或临时文件路径。Golden 判定比较 Canonical
Bytes 与全量 Hash，不只比较 Count。

### 10.2 Artifact 分层

- **Parser Native Artifact**：Native Parser 的最小可审计结构，可能含敏感正文；
- **Canonical IR Artifact**：`canonical-ir.v2`；
- **Normalization Map Artifact**：`normalization-map.v1` 与 deletable Source Unit Resolver；
- **Quality Report**：只含有界指标、稳定错误和 Hash，不含正文；
- **Chunk Manifest**：Parent/Child/Span/Joiner/Locator Hash；
- **Logical Manifest**：上述产物的有序总账。

C1 只写由 §10.3 创建的 Test Output Directory，默认测试结束安全清理。未来 Object
Storage Key、Encryption、Retention 和 Online Payload Purge 由 `012` Object Intent/
Payload Contract 管理，不能进入 Logical Hash。

### 10.3 Test Output 安全根与清理

Harness 不接受任意 Cleanup Path，也不读取 `TMPDIR/TMP/TEMP`。Compose/测试镜像固定挂载
`/run/mm-chat-parser-harness` 作为 UID `10001`、Mode `0700`、noexec/nosuid/nodev tmpfs
Parent；只允许在该 Parent Dir FD 下通过 `mkdtemp` 原子创建每 Run 的 `0700` Root。文件
使用 `O_CREAT|O_EXCL|O_NOFOLLOW` 与 `0600`，目录使用 Dir FD 和
`openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS)` 或经过等价安全测试的 fallback。

Root 内必须有不可覆盖的 Ownership Marker，绑定随机 Run ID、Root `device/inode`、Host
Boot ID、Owner PID + Process Start Time 与 Harness Schema Version；同时持有带
`O_CLOEXEC` 的 Lock File Exclusive `flock`，每个 Heartbeat Interval 通过 Dir FD 原子更新
Heartbeat。`O_CLOEXEC` 只防 `exec` 继承；任何 Harness Fork Child 在运行 Parser/Test Code
前还必须使用 `close_range(..., CLOSE_RANGE_UNSHARE)` 或验证过的 `close_fds` 关闭 Lock/
Root FD，父进程随后检查 Child FD Contract。Cleanup 前重新验证 Marker、Lock、Dir FD、
Device/Inode、Owner 与 Mode，只删除本 Run 创建并登记的 Child。以下目标一律拒绝：`/`、
Home、Repo Root、Corpus Source/Recipe、父目录、现有非空目录、Symlink、Bind Mount、跨
Filesystem Path 或 Marker 不匹配目录。禁止 `rm -rf "$USER_PATH"`、Glob Cleanup 和字符串
前缀判断。

Parent tmpfs 的实际 Compose/Mount Options 固定为 `size=512m,nr_inodes=20000,mode=0700,
uid=10001,gid=10001`。每个 Parent 由 Global Admission `flock` 限制为最多 `1` 个 Active
Run；其他 Run 在 Parent 外排队，不创建目录。每 Run Aggregate Output `≤256 MiB`、文件数
`≤10000`、单 Artifact `≤64 MiB`，至少为 Supervisor/Metadata 保留剩余 256 MiB/10000
Inodes。Byte/File 计数在创建/写入前通过 Parent-level Quota Ledger + Lock 原子 Reserve，
短写/Crash 由 Reconcile 回收但不得超额；超限返回 `RESULT_TOO_LARGE` 并按 Owned-child API
清理。

Crash 遗留由独立 Scavenger 处理：仅扫描固定 Parent 的直接 Child；只有 Nonblocking
Exclusive `flock` 成功，且 Marker Boot ID 已变化，或 Heartbeat 过期并确认 Owner
`PID + Start Time` 不存在时，才可按同一 Dir-FD/no-follow 规则删除。仍被锁、活跃
Heartbeat、PID Identity 不确定或未验证对象只告警不删除。Rollback 的“删除 Derived
Output”同样只能调用该 Owned-child Cleanup API。

## 11. Quality Gate

Quality Gate 顺序固定，早期失败不得继续生成“成功”Manifest：

1. **Source**：Size/Hash/Format/Container 与 Invocation 一致；
2. **Safety**：Archive/XML/Macro/Path/Resource Gate；
3. **Structure**：Page/Slide/Sheet/Block/Table/Cell 引用完整且无环；
4. **Text**：UTF-8/NFC/LF、无非法替换、Text Range 精确；
5. **Spatial**：Page/BBox/Rotation/Reading Flow 与 Bounds 一致；
6. **Locator**：Closed Union、Canonical Ordering、Aggregate Hash、Round-trip；
7. **Chunk**：重建内容、Token Hard Limit、边界与 Span Coverage；
8. **Determinism**：重复运行 Byte-identical；
9. **Manifest**：Count、Ordered Aggregate、Artifact Hash 全部一致。

Quality Outcome 只有：

- `accepted`：可供离线 Chunk/评测使用；
- `route_required`：仅 `MINERU_REQUIRED`，不代表解析成功；
- `quarantined`：安全、结构、质量或确定性失败；
- `unsupported`：格式、加密、活动内容或硬上限不支持。

空文件、空页或无文本不自动失败，但必须由格式规则证明其确实为空；无法区分“真实为空”
与“解析失败”时必须 Quarantine。

## 12. Parent/Child/Overlap

### 12.1 Profile

- Parent：目标 `1400–1600` Tokens，硬上限 `2000`；
- Child：目标 `350–500` Tokens，硬上限 `650`；
- 相邻 Child Overlap：`60–100` Tokens；
- 不跨 Heading、Table、Code、Sheet、Slide 或 Document Boundary；
- 每个 Parent/Child 的全部 Fragment Block 必须具有同一 `flowSeedId`，并由 E-stage Reading
  Flow 证明映射到 Chunk Envelope 中唯一 `logicalFlowId`；禁止 Joiner/Overlap/Derived
  Context 跨 Reading Flow；
- Tokenizer Name、Revision、Vocabulary/File Hash、Special-token Policy 与 Normalization
  必须进入 `chunk-profile.v1` Hash。

Tokenizer 在 C1 使用固定离线构建；未锁定生产 Tokenizer 时，Chunk Harness 只能标记为
Evaluation Profile，不得把计数写进 `011`。

### 12.2 精确重建

每个 Parent/Child 保存有序 Closed `SpanFragmentV1` Discriminated Union：

```text
common:
  fragmentKind, blockLogicalId, blockStartByte, blockEndByte
  clippedLocatorSet, fragmentSourceSpanHash
fragmentKind=primary:
  # no extra fields
fragmentKind=window_overlap:
  previousChildOrdinal, overlapGroupId, overlapTokenCount
fragmentKind=derived_context:
  derived=true, derivedReason, sourceReuseGroupId
  originalFragmentSourceSpanHash
Joiner(kind, utf8Bytes)
```

`parentChunkSeedId` 在 Chunk ID 前由
`sourceHash + chunkProfileHash + sectionOwnerSeedId + parentOrdinal` 的 Domain-tagged JCS
生成；下述 Group ID 只引用该 Seed，不引用最终 Parent/Child Chunk ID。

Unknown/跨 Variant 字段禁止。`window_overlap` 只允许 Child，必须指向同 Parent 下紧邻的
`previousChildOrdinal`；`overlapGroupId` 由 Parent Seed + 两个 Child Ordinal + 原 Fragment
Span Hash 的 Domain-tagged JCS 生成。`derived_context` 可用于 Parent/Child，`derivedReason`
是 Closed Enum `table_header | heading_context | faq_context`；`sourceReuseGroupId` 由 Parent
Seed + Reason + Original Span Hash 生成，不依赖最终 Chunk ID，避免环。

`blockStartByte/endByte` 是相对该 Block `textRange.startByte` 的 UTF-8 Byte Offset，满足
`0 <= start < end <= blockTextBytes`。Fragment 必须非空、按 Canonical Reading Order 严格
递增；同一 Chunk 不得重叠。`clippedLocatorSet` 按 §8.2 从 Block Anchor 精确裁剪，
`fragmentSourceSpanHash` 对 Source Hash + Clipped Locator + Fragment Bytes 计算。

有 `n` 个 Fragment 时必须恰有 `n-1` 个 Joiner；禁止 Leading/Trailing Joiner。Joiner Kind
和 Bytes 来自 Closed Chunk Profile，不能由内容猜测：

```text
contentBytes = fragment[0] + joiner[0] + fragment[1] ...
chunkSourceSpanHash = SHA256(
  chunk profile hash + ordered fragment source-span hashes + ordered joiner envelopes)
```

重建 Hash 必须等于 Chunk Content Hash，Parent 和 Child 都必须保存独立
`chunkSourceSpanHash`，以满足 `010`/future Evidence Ref。Span 不得切断 UTF-8 Code Point、
Grapheme-sensitive Token Boundary、Markdown Link、Code Token、Table Cell 或 Formula。
Overlap 通过相邻 Child 重复同一精确 Subanchor 表达，不能复制失去 Locator 的纯文本。

Child 的每个 Source Fragment/View 必须是其 Parent 的有序子集；Parent 可含显式 Heading
Context Prefix，但 Child 不得引用 Parent 之外的 Source。
Chunk Validator 必须重读每个 Fragment Block 的 `flowSeedId`，证明全部相同且 E-stage
`logicalFlowId` 的 Ordered Block List 完整包含这些 Block；不一致返回
`QUALITY_CHUNK_BOUNDARY_FAILED`，不得任选一个 Flow ID。

重复分成两个互不混用的 Contract：

- `fragmentKind=window_overlap`：只允许相邻 Child，Token Count 必须在 Profile 范围内，双方对
  重叠 Bytes/Locator/Hash 完全一致；
- `fragmentKind=derived_context`：用于 Table Header、Heading/FAQ Context，可跨多个 Child
  重复；必须保存 `derivedReason`、`sourceReuseGroupId`、原 Source Fragment/View 与
  `derived=true`，计入每个 Chunk Token/Content/Source-span Hash，但**不计入** Sliding
  Overlap Token。Profile 对每类 Context 设置独立 Token/Repeat 上限。

任何 Empty Span、越界、Joiner 数错误、非相邻 Window Overlap、未标记的非相邻 Source
复用或 Parent/Child 不包含关系都返回 `QUALITY_CHUNK_BOUNDARY_FAILED`。

### 12.3 结构策略

- Heading 作为 Parent Context Prefix 时必须用显式 Fragment，并计入 Token/Hash；
- Table 优先按 Row Group 切；重复 Header Row 使用 `fragmentKind=derived_context`，不得伪装成
  Sliding Window Overlap；
- Code 按 Function/Class/Block Boundary，超长时按行切，不改变空白；
- List/FAQ 保持 Item/Question-Answer 原子性；
- XLSX 不跨 Sheet，PPTX 不跨 Slide；
- 超过硬上限且没有合法切点返回 `QUALITY_CHUNK_BOUNDARY_FAILED`，不得截断。

## 13. 错误与 Fallback Matrix

稳定错误属于 Versioned Enum；Error Message 仅供本地开发且不能进入调用方逻辑。

| Code                            | 含义                             | C1 行为                      |
| ------------------------------- | -------------------------------- | ---------------------------- |
| `FORMAT_MISMATCH`               | Magic/Container 与声明冲突       | quarantine；无 fallback      |
| `FORMAT_AMBIGUOUS`              | 非自描述文本缺少唯一 Hint        | unsupported；要求重提 Hint   |
| `FORMAT_UNSUPPORTED`            | 不支持的格式/能力                | unsupported                  |
| `INPUT_INVALID`                 | 损坏、截断或结构非法             | quarantine                   |
| `INPUT_TOO_LARGE`               | Source 超过硬上限                | unsupported                  |
| `PAGE_LIMIT_EXCEEDED`           | PDF 页数超限                     | unsupported                  |
| `ARCHIVE_LIMIT_EXCEEDED`        | Entry/展开/比例/深度超限         | quarantine                   |
| `ENCODING_AMBIGUOUS`            | 无法唯一确定编码                 | quarantine                   |
| `ACTIVE_CONTENT_UNSUPPORTED`    | 宏/活动内容/OLE Policy 拒绝      | unsupported                  |
| `PDF_ENCRYPTED_UNSUPPORTED`     | PDF 加密且不允许解密             | unsupported                  |
| `MINERU_REQUIRED`               | 扫描/复杂 PDF 需外部路线         | route_required；不联网       |
| `PROTOCOL_INVALID`              | Frame/Header/字段组合非法        | quarantine；关闭连接         |
| `RESULT_TOO_LARGE`              | Parser Output 超过上限           | quarantine                   |
| `PARSER_BUSY`                   | Sidecar 已有执行与一个等待者     | failure；由 Harness 排队     |
| `PARSER_CANCELLED`              | Controller 取消并终止 Child      | failure；不得 Stage          |
| `PARSER_TIMEOUT`                | Child 超时                       | quarantine；同配置不自动重试 |
| `PARSER_MEMORY_LIMIT`           | Child OOM/内存上限               | quarantine                   |
| `PARSER_SANDBOX_UNAVAILABLE`    | Sidecar Crash/Restart/Not Ready  | failure；不得 Stage          |
| `PARSER_SCHEMA_MISMATCH`        | Output 非 Closed Schema/引用错误 | quarantine                   |
| `PARSER_NONDETERMINISTIC`       | 重跑 Bytes/Hash 不同             | quarantine                   |
| `QUALITY_LOCATOR_FAILED`        | Locator/Bounds/Round-trip 失败   | quarantine                   |
| `QUALITY_CHUNK_BOUNDARY_FAILED` | 无合法分块边界                   | quarantine                   |

禁止以下 Fallback：Binary→TXT、OOXML→ZIP Text、PDF Parse Error→OCR Provider、
Schema Error→Best-effort、Timeout/OOM→放宽资源、Invalid Unicode→Replacement Character。
Stable Error Schema 还必须验证每个 Code 的唯一 Outcome、Retryability 与禁止字段组合；实现
不得创建未登记的字符串错误分支。

## 14. Corpus 与验证矩阵

### 14.1 目录

```text
mm-chat/rag/tests/fixtures/parser_corpus/
  manifest.v1.json
  golden/
    text/
    markdown/
    html/
    docx/
    pptx/
    xlsx/
    csv/
    pdf_native/
    mineru_artifact_synthetic/
  adversarial/
    archive/
    xml/
    ooxml/
    pdf/
    encoding/
    limits/
  recipes/
```

Binary Fixture 应由 `recipes/` 可重复生成；无法生成的最小 Fixture 必须记录 Source、
License、SHA-256 与允许再分发依据。`mineru_artifact_synthetic/` 由 C1 的**纯离线 Artifact
Normalizer**消费，只验证 `layout.json`/`middle.json` 到同一 Canonical IR/Locator 的映射；
只使用 Synthetic/Public Fixture，不使用 Capture Secret、真实用户文档或 Provider Wire。
其输入只允许 `synthetic-mineru-artifact.v1`：Page `0-based`、Top-left `xyxy`、整数
milli-point、Closed Synthetic Shape，且 Manifest 固定 `testOnly=true` 与独立 Config/
Golden Namespace。它不声称复刻 live MinerU 坐标或 Wire；其 Profile/Hash 禁止被 future
Provider Adapter、Fixture Freeze、Governance 或 Promotion 复用。真实 Provider Artifact
必须在 C0 Wire/Coordinate Contract 冻结后通过新的 Adapter/Profile 转换。Submit/Poll/
Download Fake 与 Operation State Machine 仍归 C2。

### 14.2 必测集合

| 领域            | 必测                                                                            |
| --------------- | ------------------------------------------------------------------------------- |
| Text            | UTF-8/GB18030/BOM/歧义、NFC/NFD、CRLF/CR、NUL、替换字符                         |
| Markdown/HTML   | Heading/List/Table/Code/Raw HTML、Script、DTD、XXE、External Resource           |
| OOXML           | Macro、OLE、External Rel、XXE、Zip Bomb、Traversal、Missing Part、Merged/Hidden |
| XLSX            | Formula/Cached Value、Merged Cell、Hidden Row/Sheet、Shared String、超限 Cell   |
| PDF             | Rotation、双栏、跨页、表格、扫描、混合、加密、损坏、500/501 页                  |
| MinerU Artifact | 离线 `layout.json`/`middle.json` Synthetic、缺结构、BBox/Page mismatch          |
| Locator         | UTF-8 Boundary、多 BBox、跨页、Round-trip、Hash mismatch、非 Canonical 顺序     |
| Chunk           | Parent/Child 上下限、Overlap、Heading/Table/Code/Sheet 边界、精确重建           |
| Sandbox         | 无网络/Secret/Host、Timeout、OOM、PID/tmpfs、短读、尾随 Bytes、Cancel           |
| Determinism     | 10 次 Fresh Container、版本化 Hash Seed Set、Locale/TZ、并行顺序扰动            |

### 14.3 Determinism 环境

测试固定镜像 Digest、Python/Package/Wheel Hash、Locale `C.UTF-8`、Timezone `UTC`、Font
Set 与 CPU Architecture。Hash Seed 使用版本化集合 `{1, 42, 2147483647}`：10 个 Fresh
Container 按 Manifest 指定 Seed 轮转，每个 Container 内固定，跨 Seed 输出必须完全一致。
并行任务提交/完成顺序另做确定性扰动。若第三方 PDF/OOXML Library 输出依赖环境，必须在
Adapter 中 Canonicalize 或拒绝；不得只在 CI 中放宽 Golden。

## 15. future `012`、Evidence 与 Citation 影响

### 15.1 Payload/Lineage 分离

future `012` 必须引入或等价实现：

- **默认分类**：任何由用户 Source 派生的 Byte/String/Name/Path/URI/Value 都是 deletable
  Payload；没有“看似元数据所以可留”的隐式例外；
- immutable Lineage 只允许 Closed Whitelist：系统生成 Opaque ID、Ordinal、枚举 Kind、
  Boolean Flag、Count/Size、Page/Slide/Row/Column Index、无内容整数 Geometry、Schema/
  Profile Version/Hash、Content/Source/Locator Hash、Revision/FK 与删除审计状态；
- deletable Payload 至少覆盖：原文与 Filename、Canonical/Block/Parent/Child Text、Heading
  Path/Label、Markdown/HTML/LaTeX/Code、Formula 原文、Table/Cell/Hidden 值、Sheet/Shape/
  Part Display Name、XPath String、External Link/Relationship、Provenance Value、Native
  Artifact、Page/Embedded Asset 以及任何 Parser Diagnostic Snippet；
- Locator 拆分为白名单 Lineage（Kind/Ordinal/Opaque ID/整数 Geometry/Hash）与上述可删除
  Source-detail Payload；Opaque ID 必须由系统生成，不能编码 Sheet/Part/User Text；
- Materialization/Document/Collection Purge 枚举所有旧 Payload，而非只删当前 Head；
- Payload 删除后 Lineage 可保留审计 Hash，但任何 Query/Hydration 都无法恢复正文；
- Search Projection 只引用当前未删除 Payload/Materialization；Deletion Work 可重试且
  永不恢复可见；
- `012` 必须对 `010` 每个含 Source-derived Column 建立逐字段 Classification Matrix，先
  迁入新 Payload/Lineage Shape、切换受限 Runtime，再在受审计 Migration Function 内替换
  Legacy Trigger/清除旧 Row；不得修改已发布 `010` 文件；
- Migration、Purge 与 Restore Gate 必须执行全列 Residual Scan，覆盖 Postgres Base/
  TOAST/Search、Object、Cache、Temp/Staging、DLQ/Diagnostic Artifact；任何白名单外非空
  Source-derived Value 都使 Publish/Purge/Restore 失败。

删除状态必须分层，禁止把 SQL Residual Scan 或 Retention Expiry 等同 Disk-forensic
Erasure：

```text
logically_tombstoned        # 权限/查询立即不可见
online_payload_purged       # <=15 分钟：live DB/TOAST/Search/Object/Cache/Temp 无 Payload
retained_copy_pending       # WAL/Object history/Backup/Snapshot 尚在冻结 Retention 内
retained_copy_window_expired # 所有受管 Retained Copy 已到期并有证据 Manifest
```

Single-server Production Gate 必须冻结并验证以下最大窗口：

- Derived Object Bucket 默认禁用 Versioning；若部署平台强制 Versioning，Purge 必须枚举并
  删除全部 Version ID，Delete Marker 不算成功，Noncurrent Version Lifecycle 最长 `24h`；
- Local WAL 与 WAL Archive 最长 `24h`；没有 Streaming Replica。以后增加 Replica 时，必须
  等全部 Replica Ack 删除 Watermark 且其 Snapshot/Slot 不再保留旧 WAL；
- Daily Backup Payload 最长 `14d`、Weekly Backup Payload 最长 `8 weeks`；Pre-deploy
  Payload Backup 最长 `14d`。Monthly Drill 只长期保存不含 Source-derived Value 的
  Hash/Report，不保存额外 Payload Copy；
- PITR Base Backup/Snapshot 不得超过上述最长 `8 weeks`。Prune Job 必须产生 Repository/
  Object Version/WAL Archive/Snapshot 清单与 Hash，Restore Gate 证明已删除 Materialization
  不会从仍受支持的 Restore Point 复活。

Restore 不得只信恢复出来的旧 Database。`012` 必须建立 Payload-free、Append-only 的
Deletion Authority：

```text
AuthorityEntryV1 = {entryPayload, entryHash}
entryPayload common:
  schemaVersion, authoritySequence, entryType, previousEntryHash, committedAt
entryType=genesis:                 # only authoritySequence=0
  deploymentId, storePrefixHash, authoritySchemaHash
  offlineRootKid, initialSealerKid
entryType=tombstone:
  collection/document/version/materialization opaque IDs
  visibilityEpoch, tombstoneRevision, sourceEventId
entryType=purge_transition:
  tombstoneAuthoritySequence, purgeState
  stateEvidenceHash, transitionedAt
entryType=key_transition:
  transitionEnvelope
```

Genesis 的 `previousEntryHash=null`；其他 Entry 必须 Sequence 连续且 Previous Hash 精确指向
前一 Entry。`entryHash` 排除自身，固定为：

```text
entryHash = lowercaseHex(SHA256(
  ASCII("mm-chat.deletion-authority-entry.v1\n") || JCS(entryPayload)))
```

`localGenesisHash/offhostGenesisHash` 均指 Sequence `0` Genesis 的上述 `entryHash`，不存在
第二种 Genesis 算法。

Tombstone Transaction 先写本地 Ledger/Outbox；独立 `deletion-sealer` 把连续 Hash Chain 与
签名 Checkpoint 持久化到**不属于 Postgres/Object Backup Set**的 Off-host Append-only
Store。
只有 Tombstone 及对应 `purge_transition` Sequence 均已 Sealed，Online Purge 才可进入可
对外报告的 Terminal；Seal 不可用时
逻辑不可见仍生效，但状态保持 Pending 并告警。Ledger/Checkpoint 不含 Source-derived
Payload，至少保留 `max restore window + 4 weeks = 12 weeks`；滚动 Compaction 只能生成带
完整 Tombstone Set/Watermark 的签名 Checkpoint，不能丢弃仍可能命中 Backup 的删除项。

Sealer 信任边界固定：

- 独立 Container/UID、无 Parser/Object Payload Read；mTLS Workload SAN 固定
  `mm-chat-deletion-sealer`；DB `rag_deletion_sealer_runtime LOGIN NOINHERIT` 只由 Postgres
  Client-certificate Map 绑定该 SAN，并仅可 `SET ROLE rag_deletion_sealer_executor`；后者
  `NOLOGIN` 且只可 Execute `claim_seal_batch/finish_seal_batch` SECURITY DEFINER
  Functions。两者均无 Base Table DML、任意 SELECT 或其他 Role Membership；
- 只有 Sealer 持 Off-host Prefix 的 Put/List/Get + Conditional-write Credential，显式 Deny
  Delete/Overwrite；Break-glass Delete Principal 不进入 Runtime；Store 必须支持 WORM/Object
  Lock 或经评审的不可覆盖 Sequence Object；
- Authority Entry 使用 `authoritySequence`；Checkpoint 独立使用单调
  `checkpointSequence`，并包含 `authoritySequence,authorityEntryHash,previousCheckpointHash,
previousTreeSize,treeSize,merkleRoot,inclusionProof[],consistencyProof[],keyringSequence,
keyringManifestHash,revocationSequence,revocationManifestHash,issuedAt,sealerConfigHash`。即使
  没有 Tombstone，Sealer 也最多每 `12h` 写一个
  新 `checkpointSequence` Freshness Head，可合法引用同一 Authority Root，并保证 Restore
  接受的 Latest Head Age `<=24h`；同一
  `checkpointSequence` 异 Hash 仍拒绝；
- Key Rotation 先写递增 `authoritySequence` 的 `key_transition` Entry，再写新
  `checkpointSequence` 的旧/新 Key 双签 Head；旧 Public Key 保留到所有可恢复 Backup
  过期，旧 Private Key 随后销毁。未知/撤销 Kid、缺 Transition 或只单签 Rotation Head
  均 Fail Closed；
- 每个 `entries/{zero-padded-authoritySequence}-{entryHash}.json` 和
  `heads/{zero-padded-checkpointSequence}-{headHash}.json` 只写一次；`latest` 只是
  Advisory。写新 Head 必须对上一 ETag 做 Conditional CAS，并附从上一 Merkle Root 到新
  Root 的 Inclusion/Consistency Proof；
- Restore/List 必须分别验证最高连续 Authority/Checkpoint Sequence、Hash Chain、
  Signature 与 Consistency Proof。存在比 `latest` 更高的 Head、旧 Head 回退、任一 Gap、
  同 Sequence 异 Hash、最新 Head Age `>24h` 或 Store 无法证明完整 Listing时全部 Fail
  Closed。

签名与 Merkle Wire 固定：

- Ed25519 Public Key 使用 Raw 32 Bytes，Signature 使用 Raw 64 Bytes，JSON 中均用无 Padding
  Base64url；`kid=lowercaseHex(SHA256(rawPublicKey))`；
- Checkpoint 是 Closed `{signedPayload,headHash,signatures[]}`；上述 Merkle Proof 与
  Keyring/Revocation 字段全部位于 `signedPayload`，不存在未签名的 Sidecar Proof；
  `checkpointSignatureInput = ASCII("mm-chat.deletion-checkpoint.v1\n") ||
JCS(signedPayload)`，`headHash=lowercaseHex(SHA256(checkpointSignatureInput))`。`headHash` 与
  `signatures` 都不进入 `signedPayload`；每个 Signature 签相同 Input，`signatures[]` 按
  `kid` UTF-8 Bytes 排序且 Kid 唯一；
- `key_transition.v1` 是 Closed `{signedPayload,signatures[]}`；Payload 含 Schema、Old/New
  Kid、Raw New Public Key、Authority Sequence、Previous Entry Hash、Reason Enum。Signature
  Input 固定为 `ASCII("mm-chat.deletion-key-transition.v1\n") || JCS(signedPayload)`；
  `signatures[]` 必须恰有 Release-pinned Offline Root、Old Sealer、New Sealer 三个不同 Kid
  的签名，按 Kid 排序，并作为 `transitionEnvelope` 嵌入 Entry；
- Keyring/Revocation Manifest 同样使用 Closed `{signedPayload,manifestHash,signatures[]}`，
  Domain 固定 `mm-chat.deletion-keyring.v1\n` /
  `mm-chat.deletion-revocations.v1\n`。两个 `signedPayload` 都绑定
  `schemaVersion,deploymentId,storePrefixHash,manifestSequence,previousManifestHash,issuedAt`；
  Keyring 另含按 Kid 排序的 `{kid,rawPublicKey,status,notBefore,notAfter}[]`，Revocation 另含
  按 Kid 排序的 `{kid,reason,effectiveAuthoritySequence}[]`；
- `manifestHash=lowercaseHex(SHA256(domain || JCS(signedPayload)))`，Manifest Hash 与
  Signatures 不进入 Signed Payload；Root Signature 也精确签名同一个
  `domain || JCS(signedPayload)` Bytes。每个 Manifest 只接受 Release-pinned Offline Root
  Signature；Root Public Key 随 Release 固定，Revocation 不信任被撤销 Sealer Key 自签；
- Manifest Sequence 从 `0` Genesis 开始、严格连续且 Previous Hash 必须匹配；Store 使用
  不可覆盖 Sequence Key并拒绝同 Sequence 异 Hash。Checkpoint `signedPayload` 显式绑定
  Restore/List 观察到的最高连续 Keyring/Revocation Sequence+Hash；Manifest Gap、旧
  Manifest 回放、Checkpoint 未引用当前最高 Head 或 Deployment/Store Binding 不同均 Fail
  Closed；

Keyring 与 Revocation Manifest 的 Base Case 都固定为
`manifestSequence=0,previousManifestHash=null`；后续 Sequence 必须前一值 `+1` 且 Previous
Hash 非空并精确匹配。不存在 Empty-string/Zero-hash 第二种 Genesis 表示。

首个 Checkpoint 固定：

```text
checkpointSequence=0
previousCheckpointHash=null
authoritySequence=0
previousTreeSize=0
treeSize=authoritySequence+1=1
merkleRoot=leaf(genesis entryPayload)
consistencyProof=[]
inclusionProof=[]             # single-leaf tree, leaf index 0
```

后续 Checkpoint 的 Sequence 必须前一值 `+1`、Previous Checkpoint Hash 非空且匹配；
`treeSize=authoritySequence+1`。同 Tree Size Freshness Head 使用空 Consistency Proof；Tree
增长时按 RFC 6962 验证非空或算法允许的最小 Proof。

- Merkle 使用 RFC 6962-style 算法：
  `leaf=SHA256(0x00 || JCS(entryPayload))`，
  `node=SHA256(0x01 || left32 || right32)`，Empty Root `SHA256(empty bytes)`；Inclusion/
  Consistency Proof 是 Closed、按算法顺序排列的 32-byte Base64url Hash 数组。Golden
  Vectors 必须由 Python/Go/Operator CLI 交叉验证。Freshness Head 在 Tree Size 不变时
  `consistencyProof=[]`，但仍携带并验证当前 Authority Entry 的 Inclusion Proof；Tree Size
  增长时必须同时验证 Previous→Current Consistency 与 Current Entry Inclusion。

每次 Restore 在 Backend/RAG Readiness 前必须取得最新签名 Checkpoint，从 Backup 内记录的
`authoritySequence` 重放到最新连续 Watermark，再执行 Purge/Residual Scan。Store 缺失、
Signature/Hash-chain/Sequence Gap、最新 Checkpoint 超过 `24h` 或重放未完成都 Fail Closed。
Restore Drill 必须覆盖“长期无 Tombstone 的 Freshness Head”“Key Rotation”和“旧 Backup
不含后续 Tombstone”。

`retained_copy_window_expired` 最迟是 `online_payload_purged + 8 weeks`，只有 Version/WAL/
Replica/Backup/Snapshot Evidence 全部闭合才能进入。15 分钟 SLO 仅指 Online Payload
Purge。

本 Contract **明确排除并绝不宣称 Disk-forensic/Physical Media Erasure**：Postgres MVCC
Dead Tuple、旧 TOAST Tuple、Heap/Index/Search Free Page 与 Filesystem/SSD 已释放块可能仍含
残留，普通 `DELETE/VACUUM/REINDEX` 均不构成覆写证明。Production 只要求全卷 At-rest
Encryption 与介质退役 Sanitization；状态/API/告警不得使用 `media_erasure_complete`、
`physically_erased` 等词。若法规要求按 Document 可证明擦除，必须另立 ADR，引入覆盖
Current Data File/Search Projection 的 per-materialization Encryption/DEK Destruction 或
受验证 Storage Sanitization，并通过 Restore/Forensic Gate 后才能改变本边界。

### 15.2 Staging Contract

Runtime Adapter 只能在 `012` 后把 v2 Artifact 映射到 DB：

- 先验证 Manifest/Profile/Generation/Materialization/Lease；
- Logical ID/Hash 到 UUID 的映射必须一一对应、幂等且受 Materialization 约束；
- Locator v2 不得塞进 `010` v1 Shape 后丢字段；
- Stage/Verify/Finalize 全部使用受限 Function，Worker 无 Base Table DML；
- C1 Harness 成功不表示 Runtime、Provider、Publish 或 Query Ready。

### 15.3 Evidence/Citation v2 Addendum

Phase 15.2D 前新增版本化 DTO：

- Evidence Reference v2 返回 `locatorSetVersion/hash` 与有界 Locator Set；
- Go 在 Hydration 前后重验 Collection/Document/Version/Generation/Revision；
- Citation v2 是 v1 的 additive version，原样保留并绑定 `actorUserId`、`sessionId`、
  `conversationId`、`assistantMessageId`、Expiry、Authorization Snapshot 及 v1 全部
  Invalidation Fence；另外绑定 Source Span Hash 和 Locator Set Hash；
- Citation Resolve 每次重新授权，并按 Source Format 生成可展示位置；
- Logout/Session Expiry/Recovery、Membership/ACL/Visibility/Consent/Current Version/
  Generation/Projection 变化继续立即使 Citation 失效；
- v1 保持原 Shape，不支持的 v2 Locator 禁止有损降级。

## 16. 可执行切片

### C1.1 Contract 与 Corpus

- [x] 新建 Closed JSON Schema：Canonical IR v2、Locator v2、Quality/Chunk/Manifest v2。
- [x] 冻结 Source Unit/Normalization Map、Anchor/Source Fragment/View、Geometry、数组比较器、JCS、
      Hash Envelope、Logical ID 与完整 Stable Error Enum。
- [x] 冻结 Normalization Map 独立 Artifact/Manifest Ref、Source Unit Resolver、Exact-cover
      与 Closed Transform/SourcePosition Union、Parent/Subsegment Coalesce、
      Identity/Syntax-decode/非线性 Segment Split Contract。
- [x] 冻结 Source Unit/Sheet/Shape/Structure/Payload Opaque ID Envelope、UTF-8 Byte
      Comparator、ownerSeed 分层 DAG、Text/Structural Span Hash、Kind Rank 与 Golden
      Vector。
- [x] 冻结 Normalization Aggregate、Provenance C4、Flow Seed/Final ID 与 Parent/Child
      Domain-separated Chunk ID Golden Vector，证明 Hash DAG 无环。
- [x] 建立 Golden/Adversarial/Recipe Corpus、License Manifest 与 Fixture Hash。
- [x] 增加 Schema Negative Test：Unknown/Duplicate Key、Float/Safe-integer、非 Canonical、
      错 Hash，并通过 RFC 8785 官方向量与 Python/Go/JS Cross-runtime Equality。

C1.1 实现证据位于 `mm-chat/rag/src/mm_chat_rag/contracts/`、
`tests/fixtures/parser_contracts/`、`tests/fixtures/parser_corpus/` 与
`tests/fixtures/jcs/`。18 份 Packaged Schema、24 类 Logical Hash Envelope、49 个 Source
Fixture、27 个 Deterministic Binary Recipe 和 89 个三运行时 JCS/Logical-ID Case 已通过
离线 Gate。Test-only Semantic Validator 额外证明 Normalization/Locator Ordering 与
Exact-cover、引用/DAG、Table Grid、Chunk Cardinality/Overlap、Manifest Count/Hash；它不等同于
Native Parser、Fresh-container Determinism 或生产 Runtime Promotion。
Integrated A–F Fixture 使用真实重算的 Logical ID/Hash，并把 IR、Normalization Map、Source
Unit Resolver、Quality、Chunk 与 Canonical Manifest 的 Bytes/Count/Aggregate 全量绑定；
Projection Fixture 证明 Map→Locator 唯一投影和 Child→Parent Fragment/View 有序子集。
Synthetic MinerU `layout`/`middle` 是两个不同的单 Role Artifact，Pair Gate 只做离线结构合流，
不冒充 live Wire 或 Parser Output。

### C1.2 Router 与 Sandbox Protocol

- [x] 实现 Magic/Container-first Router、非自描述文本 Hint/歧义规则与无回退 Error Matrix。
- [x] 实现逐 Byte 固定的 Framed UDS Protocol、MIME/Extension Hint、
      Length/Hash/Deadline/Cancel/Outcome Gate。
- [x] 构建 UID `10002` 无网络 Sidecar 与每 Job 独立 Child。
- [x] 实现 PID 1 Subreaper、`clone3=ENOSYS`/masked `clone` Seccomp、全后代 Reap 与
      Supervisor-prebuilt Process Group、Residual-process Restart Gate，并把 Seccomp
      Stage/Hash 纳入 Config。
- [x] 实现专用限额 Test Output Root、Ownership Marker、Fork FD Close、Dir-FD/no-follow
      Cleanup 与 flock/Heartbeat Scavenger。
- [x] 通过 Archive Duplicate/Header Drift/Encryption、XML/Traversal/XXE/Macro、Child/
      Sidecar OOM/Fork Bomb/Resource 与 Cleanup 负向测试。

C1.2 实现位于 `mm-chat/rag/src/mm_chat_rag/offline_parser/`。Router 对 49 份冻结 Corpus
Expectation 全量命中；MMCP Request Binding 固定为 Domain + Header-without-binding JCS +
Raw Source Digest。Sidecar 只在 `parser-c1` Compose Profile 中运行，UID/GID
`10002:10001`、PID 1、`network_mode:none`，生产 Registry/Dispatch 仍为空。已识别格式在
C1.3 前仍 Fail Closed 为 `FORMAT_UNSUPPORTED`，不生成占位 Canonical IR。190 项 C1.2
定向测试、866 项全量测试、91.16% Coverage、Ruff/Format/Mypy、pip-audit、离线 Wheel、
三运行时 JCS 与 Docker 实镜 Smoke 均通过。

### C1.3 Native Parsers

- [x] 实现 TXT/Markdown/HTML Parser 与精确 Byte/Line Locator。
- [ ] 实现 DOCX/PPTX/XLSX/CSV Parser 与 OOXML/Sheet/Shape Locator。
- [ ] 实现 Native PDF 安全子集与 `MINERU_REQUIRED` 分类器。
- [ ] 实现 Synthetic/Public MinerU Artifact Offline Normalizer；不实现 Wire 或网络。
- [ ] 保留 Table/Formula/Asset/Reading Flow/Provenance，不做图片索引。

C1.3A 实现位于
`mm-chat/rag/src/mm_chat_rag/offline_parser/native/`。TXT、固定 CommonMark + Table
Markdown 与 hardened HTML 只在 C1.2 Child 安全边界内执行，并生成 Closed
`parser-native-artifact.v1` 内部 Artifact；Supervisor 校验 JCS、Length、Hash、Limit、
Format 与 Source Binding，但不解码或重解析 Source。该 Artifact 不是
`canonical-ir.v2`，Sidecar 仍返回 zero-body `FORMAT_UNSUPPORTED`，不可 Stage；Registry、
Dispatch、Provider、Postgres/Redis/MinIO、迁移 `011/012` 与生产 Handler 均保持关闭。
全量 `1069 passed / 2 skipped`、Coverage `91.19%`，Ruff/Format/Mypy、pip-audit、
89 Case x 3 Runtime JCS、21 Artifact/18 Schema Offline Wheel、Security Scanner 与隔离
Docker Compose Smoke 均通过；独立 Review 为 `P0/P1/P2 = 0/0/0`。当前 Config Hash 为
`8a72668218932f6af95d3b6276646304451d7f9ea59ff658ca7887d925e83ea7`。

### C1.4 Canonicalize 与 Quality

- [ ] 实现 NFC/LF/Text Buffer、整数坐标、Closed IR Validator。
- [ ] 实现结构、文本、空间、Locator 与 Manifest Quality Gate。
- [ ] 实现 10 次 Fresh-container Determinism 与差异诊断 Artifact。

### C1.5 Chunk Harness

- [ ] 实现 Parent/Child/Overlap、结构边界与固定 Tokenizer Profile。
- [ ] 实现 Span Fragment/Joiner 精确重建、UTF-8 Boundary 与 Locator Round-trip。
- [ ] 生成 deterministic Chunk/Logical Manifest 并通过 Golden Gate。

### C1.6 收口

- [ ] 运行 Ruff/Format/Mypy/Pytest/Coverage、Dependency/License/Security Scan。
- [ ] 证明 Registry/Dispatch/Migration/Network/Provider 调用均未变化。
- [ ] 独立 xhigh Review 达到 `P0/P1/P2 = 0/0/0`。
- [ ] 对 Online Purge/Retained-copy/Deletion Sealer/Authority/Down Gate Schema 与措辞执行
      Static Contract Lint，包括 Genesis/Entry/Head/Checkpoint/Key/Merkle Wire、Freshness/
      Rotation、一次性 Down Assertion；Runtime State/Prune/Restore 验证明确留给上位计划
      C6/C9/C12。
- [ ] 更新 Progress/Process；提交前给出精确文件清单并等待 Owner 确认。

## 17. Rollback

C1 无数据库或生产 Runtime 状态，Rollback 只需：

1. 保持 Registry/Dispatch 关闭；
2. 删除新增 Harness Code、Test-only Compose Profile；Corpus Derived Output 只能通过
   §10.3 Owned-child Cleanup API 清理，禁止接收任意路径；
3. 保留 Source Fixture、Recipe、Schema/Plan 供问题复现；
4. 验证 Python dark-run Test、migration `010` Hash 与现有 Compose 服务不变。

若发现 Canonical Contract 缺陷，必须提升 Schema/Profile Version 并重生 Golden；禁止在
同一个 Version 下改语义或只更新预期 Hash。

## 18. 后续关键路径

```text
C1 Offline Parser/Canonical IR
  -> C2 Provider Fake/Operation State Machine（可与 C1 后半并行）
  -> C3/C4 real Jina bake-off + Search Winner freeze
  -> 011 Search-only migration
  -> 012 Dispatcher + Payload/Lineage + Locator v2 staging
  -> Go Producer Cutover + Runtime Gateways/Handlers
  -> Canary/Rebuild/Controlled Activation
```

下一实施刀固定为 **C1.3 Native Parsers**。保持 Registry/Dispatch、Provider、数据库、
迁移 `011/012` 与生产 Handler 关闭；Parser 必须复用 C1.2 Router/Sandbox，禁止绕过 MMCP、
Process-group/Seccomp、资源与 Cleanup Gate。
