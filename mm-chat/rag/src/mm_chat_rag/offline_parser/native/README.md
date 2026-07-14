# C1.3B Native Parsers

`native` 是 MM Chat 离线解析链的 Child-internal 解析层。它在 C1.2
Process-group、RLIMIT 与 Seccomp 握手之后，把 TXT、Markdown、HTML、CSV、DOCX、
PPTX、XLSX 转成确定性的 `parser-native-artifact.v2`；该产物只供后续 C1.4
Canonicalizer 使用。

## 核心能力

- **确定性文本解码**：BOM → strict UTF-8 → GB18030；禁止 Locale 和 replacement。
- **多 Source Unit Locator**：Raw File 固定 ordinal `0`；OOXML Part 按 canonical URI
  unsigned UTF-8 bytes 排序，记录 Raw Byte、Scalar、Line/Column 半开区间。
- **单一 OPC Admission**：Router 与 Parser 共用 `ValidatedOpcPackage`，Parser 不重开 ZIP、
  不猜 Part、不重复执行另一套 ZIP/XML preflight。
- **安全 OOXML**：仅 STORED/DEFLATED，禁 ZIP64、递归 Archive、Macro/OLE；严格解析
  Content Types 与 closed Relationships；External target 只保留 metadata，绝不 fetch。
- **Hardened XML**：source-aware Expat 仅收 UTF-8/BOM，拒绝 DTD、自定义 Entity、PI、
  XInclude 与 external resolution。
- **固定结构语义**：DOCX 文档/列表/表格/注释，PPTX Slide/Shape/Table/Notes，XLSX
  Sheet/Cell/Formula/Cached/Merge，以及无 Sniffer 的固定 CSV FSM。
- **有界产物**：Archive/XML/Source Unit/Node/Fragment/Cell/Sheet/Slide/Shape/CSV 与
  Artifact Bytes 上限全部进入 Parser Config Hash。
- **内部传输验证**：Child/Supervisor Frame 校验 Closed JCS Header、Length、SHA-256、
  Body Limit、EOF 与请求 Source Binding。

## 使用边界

生产调用只能通过 `ParserController -> Sidecar -> Sandbox Child`。下面的直接调用只用于
离线单元测试和 C1.4 开发：

```python
from mm_chat_rag.offline_parser.native.dispatch import parse_native_source

outcome = parse_native_source(b"# heading\n", declared_extension=".md")
assert outcome.artifact is not None
assert outcome.artifact.source_format.value == "markdown"
```

C1.3B **不会**产生 `canonical-ir.v2`。即使内部 Native Artifact 已验证，Sidecar 仍向
MMCP Controller 返回零 Body `FORMAT_UNSUPPORTED`，因此 `stageable == false`。

## 主要模块

| 模块                 | 职责                                                               |
| -------------------- | ------------------------------------------------------------------ |
| `decoding.py`        | strict decode 与 compact uint32 Locator index                      |
| `model.py`           | Artifact v2、多 Source Unit DTO、JCS 编解码与请求绑定              |
| `txt.py`             | TXT 原样 Native Parser                                             |
| `markdown.py`        | 固定 CommonMark + Table Parser 与 Raw HTML Policy                  |
| `html.py`            | `HTMLParser(convert_charrefs=False)` hardened Parser               |
| `csv.py`             | 固定 comma/quote/doublequote/record-terminator FSM                 |
| `xml_source.py`      | hardened Expat 与精确 XML Part Locator                             |
| `opc.py`             | ZIP/OPC Admission、Content Types、Relationships 与 Part Capability |
| `docx.py`            | DOCX Paragraph/List/Table/Footnote/Endnote                         |
| `pptx.py`            | PPTX Slide/Shape/Table/Notes 与 exact-rational Geometry            |
| `xlsx.py`            | XLSX Sheet/Cell/Formula/Cached/Merge/Hidden                        |
| `dispatch.py`        | 固定格式表、Child 内 Locator 重算与资源上限                        |
| `internal_result.py` | 非 MMCP 的 Child/Supervisor Native Result Frame Header             |
| `profile.py`         | 组件源码 inventory/hash、依赖 pin 与固定安全选项                   |

## 依赖

- `markdown-it-py==4.2.0`（MIT）及 `mdurl==0.1.2`（MIT）
- Python 3.13 stdlib `html.parser`、`zipfile`、`xml.parsers.expat` 与自有 CSV FSM

C1.3B 未增加第三方依赖。不允许运行时插件发现、Provider SDK、网络客户端、数据库或生产
Registry。

## 验证

```bash
uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_native_model.py \
  tests/unit/test_parser_native_text.py \
  tests/unit/test_parser_native_markdown.py \
  tests/unit/test_parser_native_html.py \
  tests/unit/test_parser_native_csv.py \
  tests/unit/test_parser_native_xml.py \
  tests/unit/test_parser_native_opc.py \
  tests/unit/test_parser_native_docx.py \
  tests/unit/test_parser_native_pptx.py \
  tests/unit/test_parser_native_xlsx.py \
  tests/unit/test_parser_native_dispatch.py \
  tests/unit/test_parser_native_sandbox.py
```

设计与安全边界见 [DESIGN.md](DESIGN.md)。
