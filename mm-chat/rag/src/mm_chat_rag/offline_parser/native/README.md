# C1.3A Native Parsers

`native` 是 MM Chat 离线解析链的 Child-internal 解析层。它在 C1.2
Process-group、RLIMIT 与 Seccomp 握手之后，把 TXT、Markdown、HTML 转成确定性的
`parser-native-artifact.v1`；该产物只供后续 C1.4 Canonicalizer 使用。

## 核心能力

- **确定性解码**：BOM → strict UTF-8 → GB18030；禁止 Locale 和 replacement。
- **精确原始位置**：记录 Raw Byte、Decoded Unicode Scalar、Line/Column 半开区间。
- **结构保留**：保留 Heading、List、Code、Table、Raw HTML、Asset Reference 等节点。
- **安全解析**：固定 Markdown 配置；HTML 不执行脚本、不解引用 URL、不发起网络请求。
- **有界产物**：Node、Fragment、Line、Depth、Attribute、Text 与 Artifact Bytes 全部进入
  Parser Config Hash。
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

C1.3A **不会**产生 `canonical-ir.v2`。即使内部 Native Artifact 已验证，Sidecar 仍向
MMCP Controller 返回零 Body `FORMAT_UNSUPPORTED`，因此 `stageable == false`。

## 主要模块

| 模块                 | 职责                                                             |
| -------------------- | ---------------------------------------------------------------- |
| `decoding.py`        | Lightweight route decode 与 compact uint32 Locator index         |
| `model.py`           | Closed Native Artifact DTO、JCS 编解码与请求绑定                 |
| `txt.py`             | TXT 原样 Native Parser                                           |
| `markdown.py`        | 固定 CommonMark + Table Parser 与 Raw HTML Policy                |
| `html.py`            | `HTMLParser(convert_charrefs=False)` hardened DOM-to-node Parser |
| `dispatch.py`        | 固定格式表、Child 内 Locator 语义验证与资源上限                  |
| `internal_result.py` | 非 MMCP 的 Child/Supervisor Native Result Frame Header           |
| `profile.py`         | 组件源码 Hash、依赖版本/License/Wheel Hash 与固定选项            |

## 依赖

- `markdown-it-py==4.2.0`（MIT）及 `mdurl==0.1.2`（MIT）
- Python 3.13 stdlib `html.parser`

不允许运行时插件发现、Provider SDK、网络客户端、数据库或生产 Registry。

## 验证

```bash
uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_native_model.py \
  tests/unit/test_parser_native_text.py \
  tests/unit/test_parser_native_markdown.py \
  tests/unit/test_parser_native_html.py \
  tests/unit/test_parser_native_sandbox.py
```

设计与安全边界见 [DESIGN.md](DESIGN.md)。
