# Candidate 8 RAG 评估结果与指标释义

> 日期：2026-07-25  
> 目的：解释 Candidate 8 的 Development、Validation、frozen Holdout 和最终
> 500-case Gate 结果，避免把“指标通过”误解成“已经激活”或“所有真实场景都已证明”。

## 1. 结论

Candidate 8 已完成一次且仅一次 frozen Holdout，并通过 Candidate-only v2
正式 Gate：

```text
Gate passed:                 true
Critical slices passed:      10 / 10
Recall@50:                   1.000
Final Recall@10:             1.000
nDCG@10:                     0.9992618595
MRR@10:                      0.999
Citation Correctness:        0.9980039920
Citation Completeness:       1.000
Faithfulness:                1.000
Answer Correctness:          1.000
Table Exact Answer:          1.000
P95 retrieval latency:       644 ms / 1000 ms budget
Average retrieval context:   1874.856 / 4096 tokens budget
Locator/provenance/lineage:  100%
Leakage:                     0
```

这说明 Candidate 8 在本次冻结评估集上满足既定的检索、排序、回答、引用、
完整性、安全和预算门槛。它不代表已经 Activation；当前 Active 仍为
Generation 3，Candidate 8 仍为 `verified / ready`。

## 2. 本次评估绑定

```text
Candidate Generation:   4e9e18ef-c259-440b-9976-b4632e50b419
Candidate Sequence:     8
Artifact Manifest:      ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f
Chunk Profile:          36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73
Retrieval Profile:      siliconflow_bge_m3_v1
Embedding:              Pro/BAAI/bge-m3
Reranker:               Pro/BAAI/bge-reranker-v2-m3
Answer Model:           gpt-5.6-sol
Holdout Run ID:         ba1749c9-922e-4ab7-b6c0-37603e582617
Holdout Ordinal:        1
Capture ID:             7e887707-5657-457c-a518-d754de98e20e
```

所有结论只适用于上述 Generation、Manifest、Chunk Profile、Retrieval Profile
和冻结语料绑定。更换模型、向量空间、分块 Profile 或重建 Generation 后，不能
直接沿用这份结果。

## 3. 数据集构成

| 阶段        | Cases | 比例 | 用途                                       |
| ----------- | ----: | ---: | ------------------------------------------ |
| Development |   300 |  60% | 调试、发现排序问题、验证修复               |
| Validation  |   100 |  20% | 检查修复是否能泛化到未用于直接调试的数据   |
| Holdout     |   100 |  20% | 预提交的一次性最终盲测，不允许反复试到通过 |
| 合计        |   500 | 100% | 生成最终 Candidate-only Gate Report        |

语料来自 50 份去重的合成评估文档，覆盖 PDF、DOCX、PPTX、XLSX、Markdown/
JSON/code、中英文、短事实、跨章节和精确数值场景；问题与证据经过人工逐条确认。

关键限制：这仍是一套受控的 synthetic + human-reviewed Golden，不等于所有真实
用户文档和开放式问题的完整分布。

## 4. 四阶段结果对比

| 指标                        | Gate             | Development 300 | Validation 100 | Holdout 100 | 最终 500 |
| --------------------------- | ---------------- | --------------: | -------------: | ----------: | -------: |
| Recall@50                   | `>= 0.95`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| Final Recall@10             | `>= 0.90`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| nDCG@10                     | `>= 0.85`        |        1.000000 |       1.000000 |    0.996309 | 0.999262 |
| MRR@10                      | `>= 0.80`        |        1.000000 |       1.000000 |    0.995000 | 0.999000 |
| Citation Correctness        | `>= 0.95`        |        0.996678 |       1.000000 |    1.000000 | 0.998004 |
| Citation Completeness       | `>= 0.90`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| Faithfulness                | `>= 0.95`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| Answer Correctness          | `>= 0.95`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| No-answer false-answer rate | `<= 0.02`        |        0.000000 |       0.000000 |    0.000000 | 0.000000 |
| Table Exact Answer          | `>= 0.95`        |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| Provenance/Cell Lineage     | `= 1.00`         |        1.000000 |       1.000000 |    1.000000 | 1.000000 |
| P95 retrieval latency       | `<= 1000 ms`     |          600 ms |         652 ms |      658 ms |   644 ms |
| Average retrieval context   | `<= 4096 tokens` |         1857.31 |        1860.75 |     1941.59 |  1874.86 |
| ACL/deletion/secret leakage | `= 0`            |               0 |              0 |           0 |        0 |
| Unauthorized evidence leaks | `= 0`            |               0 |              0 |           0 |        0 |

### 为什么最终 P95 可能低于 Validation/Holdout P95

P95 是对各自样本集合重新排序后取第 95 百分位，并不是三个阶段 P95 的加权平均。
合并 500 cases 后分位点位置发生变化，因此最终 `644 ms` 低于 Validation 的
`652 ms` 和 Holdout 的 `658 ms` 是可能且正常的。

## 5. 各指标代表什么

### Recall@50

在初步召回的前 50 条 evidence 中，预期相关证据被找回的比例。

- 本次结果 `1.0`：500 cases 的目标证据全部进入前 50。
- 它证明“没有在粗召回阶段丢失目标”，但不代表目标一定排在第一。

### Final Recall@10

经过 Rerank 后，目标证据仍保留在最终前 10 条中的比例。

- 本次结果 `1.0`：目标证据没有在重排和裁剪阶段丢失。
- 这是进入回答上下文前的重要召回门槛。

### nDCG@10

衡量相关证据在前 10 中的位置质量，越靠前得分越高；`1.0` 表示理想排序。

- Holdout 为 `0.996309`。
- 原因是 `rageval-code-zh-04-f02` 的目标证据位于第 2，而不是第 1。
- 目标仍在 Final Top 10，最终回答和引用仍正确，所以这是轻微排序损失，不是
  evidence 丢失。

### MRR@10

只关注第一个相关结果的排名，计算其 reciprocal rank，再对 cases 求平均。

- Holdout 为 `0.995`，对应 100 cases 中 99 个目标证据排第 1、1 个排第 2。
- 最终 500-case 为 `0.999`，说明整体首个相关结果几乎总在第一位。

### Citation Correctness

模型给出的 Citation 中，有多少指向 Golden 标记的相关证据。

- Development 为 `0.996678`，因为
  `rageval-code-en-06-f07` 除正确 Citation 外，还引用了 1 条 Golden 未标记为
  relevant 的额外 Chunk。
- 正确 Citation 没有缺失，因此 Citation Completeness 仍为 `1.0`。
- 最终 `0.998004` 高于 `0.95` 门槛，但说明仍存在减少非必要引用的优化空间。

### Citation Completeness

Golden 要求引用的证据中，有多少真正被模型引用。

- 本次为 `1.0`：所有需要引用的目标证据都被引用。
- Correctness 与 Completeness 必须结合看：前者防止乱引，后者防止漏引。

### Faithfulness

回答是否由实际进入上下文并被引用的 source evidence 支撑。

- 本次为 `1.0`：没有观察到脱离证据的回答。
- 该值绑定本次确定性评分规则和评估 Answer Model，不是对任意模型的永久保证。

### Answer Correctness

回答是否匹配人工确认的 expected answer，包括精确文本、日期或完整数值组合。

- 本次为 `1.0`：所有有答案 cases 都命中预期答案。

### Table Exact Answer

对要求精确表格答案的 cases，答案必须正确且 Citation 能回到原始 cell lineage。

- 冻结语料共有 50 个 table-exact cases。
- 本次为 `1.0`：答案、sheet/cell locator 和 lineage 全部满足要求。

### No-answer false-answer rate

理论含义：当授权证据不足时，系统错误给出确定答案的比例。

本次必须特别说明：冻结 Golden 中 `expectedNoAnswer=true` 的 case 数量为 **0**。
因此报告中的 `0.0` 是空 cohort 的默认值，不能解释为“no-answer 能力已经通过
100 个负例证明”。正式 evaluator 当前允许这种情况，所以 Gate 仍合法通过。

随后已运行一组不改变 frozen Holdout 的 50-case negative/no-answer regression：
50/50 正确拒答，Citation、marker、absent source/subject match 和 leakage 全为 0。
它补充了行为诊断证据，但明确标记为 `promotionEvidence=false`，不能追溯改变正式
Gate。若未来要把 no-answer 作为 Promotion 强证据，应在下一版 Golden 准入规则
中冻结明确的最小负例数量，而不是把 supplemental cases 追加到现有 Golden。

### Provenance、Locator 与 Cell Lineage

- `Citation Locator Rate = 1.0`：Citation 均能解析到有效原文定位。
- `Provenance Rate = 1.0`：证据均保留合法来源链。
- `Cell Lineage Rate = 1.0`：表格证据能回到原始 sheet/cell。

它们防止“答案看似正确，但 Citation 无法回源”或“引用了派生文本却伪装成原文”。

### Leakage

检查 ACL、删除状态、Secret 和未授权 evidence 是否进入检索结果。

- 本次全部为 `0`。
- 这证明本次 500-case capture 没观察到越权证据，不代表可以移除运行时权限和
  reauthorization 防线。

### P95 retrieval latency

从 Query Embedding、Candidate Fetch、Hydration 到 Rerank 完成的 retrieval
pipeline P95；**不包含 Answer Model 生成文本所耗时间**。

- 最终 `644 ms`，低于冻结预算 `1000 ms`。
- 不能把它直接当成用户界面完整回答的端到端延迟。

### Average retrieval context

评估回答阶段装入的平均 Knowledge context token 数，包含固定评估 envelope。

- 最终平均 `1874.856`，低于 `4096` 上限。
- 这是评估 capture 的 Knowledge context 成本，不等于整个聊天请求的总 Prompt
  Tokens；真实请求还包含系统 Prompt、历史消息和可能的 Web evidence。

## 6. Critical Slice 结果

同一个 case 可以同时属于格式、语言和问题类型等多个 slices，因此下面的 Cases
相加会超过 500，这是正常的重叠统计。

| Slice              | Cases |  nDCG@10 | MRR@10 | Citation Correctness | Passed |
| ------------------ | ----: | -------: | -----: | -------------------: | :----: |
| PDF                |   100 | 1.000000 | 1.0000 |             1.000000 |   是   |
| Text/Markdown/DOCX |   200 | 0.998155 | 0.9975 |             0.995025 |   是   |
| PPTX               |   100 | 1.000000 | 1.0000 |             1.000000 |   是   |
| XLSX/Table         |   100 | 1.000000 | 1.0000 |             1.000000 |   是   |
| JSON/Code          |   100 | 0.996309 | 0.9950 |             0.990099 |   是   |
| Chinese            |   250 | 0.998524 | 0.9980 |             1.000000 |   是   |
| English            |   250 | 1.000000 | 1.0000 |             0.996016 |   是   |
| Short Fact         |   250 | 0.998524 | 0.9980 |             0.996016 |   是   |
| Cross Section      |    50 | 1.000000 | 1.0000 |             1.000000 |   是   |
| Exact Numeric      |   250 | 1.000000 | 1.0000 |             1.000000 |   是   |

所有 slices 的 Recall@50、Final Recall@10、Faithfulness、Answer Correctness、
Table Exact、Locator/Provenance/Lineage 和 leakage 均通过对应 Gate。

## 7. 如何正确解读这份结果

可以得出的结论：

1. BGE Candidate 8 在冻结 500-case 语料中没有召回丢失。
2. Rerank 基本把正确 evidence 放在第一位，仅一个 Holdout case 位于第二。
3. 答案与引用完整，只有一个 Development case 产生额外 Citation。
4. 表格精确答案、Locator、Provenance、Cell Lineage 和授权隔离全部通过。
5. Retrieval P95 和平均 Knowledge context 均在冻结预算内。

不能直接得出的结论：

1. 不能说所有真实世界文档和开放式问题都已覆盖。
2. 不能把 `644 ms` 当成完整聊天端到端延迟。
3. 不能把 no-answer rate `0` 当成负例能力证明，因为本次无 no-answer cases。
4. 不能把 Gate `passed=true` 当成自动 Activation 授权。
5. 不能把该结果迁移到另一 Generation、Embedding/Rerank 模型或 Chunk Profile。

补充说明：后续独立 50-case regression 已证明该 exact Candidate 在对应 synthetic
负例上 50/50 正确拒答，但它仍不是 frozen Gate 或 Activation authority。

## 8. Activation 前应审核什么

1. 核对 Gate Report 文件 SHA-256 是否为：

   ```text
   fd93ae98700cdc923dfff3b82c6520eb13663ae03cc1927f09103387015b608b
   ```

2. 确认报告中的 Generation、Manifest、Profile 和本文件第 2 节完全一致。
3. 知悉两个非阻断偏差：一个目标 evidence 排第 2、一个 case 多引 1 条 Chunk。
4. 知悉 frozen no-answer cohort 为 0；独立 50-case regression 已完成且不属于
   Promotion evidence。
5. 只有在明确批准上述 exact report hash 后，才可执行 Candidate 8 Activation。

## 9. 原始证据文件

目录：

```text
.trellis/tasks/07-23-optimize-rag-chunking/research/
```

| 文件                                                            | 用途                       | SHA-256                                                            |
| --------------------------------------------------------------- | -------------------------- | ------------------------------------------------------------------ |
| `promotion-preflight-v6-candidate8-development-2026-07-25.json` | Development 300            | `1ffc4a6335a7ff092eaca32ed0de2c443a4a1b95ddd9e5aea2d208d29959f7cc` |
| `promotion-preflight-v6-candidate8-validation-2026-07-25.json`  | Validation 100             | `3569f1a3a0f28bf3c1173b7e166b442d01694fa193081a1e164371e42f77bead` |
| `promotion-holdout-one-shot-candidate8-seal-2026-07-25.json`    | One-shot execution seal    | `d713943addb4d3a2dffc62b1a679c7566b85ad89787d15f97545c4de9b84ecea` |
| `promotion-observations-v1-candidate8-2026-07-25.json`          | 完整 500-case observations | `1ef4624733fd9443c3ecda3d551b28d0db815fd7d753321991cebaa44d07702c` |
| `promotion-gate-v2-candidate8-2026-07-25.json`                  | 正式 Candidate-only Gate   | `fd93ae98700cdc923dfff3b82c6520eb13663ae03cc1927f09103387015b608b` |

查看最终总体指标：

```bash
jq '{passed, metrics, budgets, integrity}' \
  .trellis/tasks/07-23-optimize-rag-chunking/research/\
promotion-gate-v2-candidate8-2026-07-25.json
```

查看所有 slices：

```bash
jq '.slices' \
  .trellis/tasks/07-23-optimize-rag-chunking/research/\
promotion-gate-v2-candidate8-2026-07-25.json
```

核验正式报告 Hash：

```bash
sha256sum \
  .trellis/tasks/07-23-optimize-rag-chunking/research/\
promotion-gate-v2-candidate8-2026-07-25.json
```

## 10. 当前 Activation 状态

```text
Active Generation:      sequence 3
Candidate Generation:   sequence 8, verified / ready
Activation gate hash:   NULL
Activation audits:      0
```

因此，本文件是评估结果说明和 Activation 决策材料，不是 Activation 执行记录。

## 11. Supplemental no-answer 回归

随后执行的独立 50-case no-answer regression 已补上本文件第 7 节指出的空
cohort 盲区：50/50 均正确拒答，false-answer、Citation marker/evidence、absent
source/subject match 和 leakage 全为 0，平均 context 为 `2212.88` tokens。

该 supplemental report 的总结果仍为 `passed=false`，因为首批四个并发 case
使 Retrieval P95 达到 `1606ms`，超过 `1000ms` 预算。失败报告已保留且没有
改变 frozen Golden、one-shot Holdout 或 Activation 状态。

随后在一个新 helper process 内完成 paired cold/warm diagnostic：Cold P95 为
`1498ms`，Warm P95 为 `870ms`，下降 `628ms / 41.9226%`。两阶段共 8 cases 的
正确性、Citation 和 leakage integrity 全通过，报告结论为
`cold_start_effect_observed`。这确认新进程冷启动效应存在，但尚未把连接池、
HTTP/TLS 和数据库 cache 的贡献分别隔离，也不允许把 canonical first-run 报告
改写成 pass。

完整结果与 Hash：

```text
candidate8-supplemental-no-answer-results-2026-07-25.md
supplemental-no-answer-report-candidate8-2026-07-25.json
SHA-256 77488e1363e2b025ae1dc260a594fabd102218356d1f47f68853cbe8ad6ca7ad

supplemental-no-answer-latency-diagnostic-candidate8-2026-07-25.json
SHA-256 54d5523245b1108337fdb5824a22dbe3224d1fc3c4ac4294c2875c3c42cfecf3
```

产品裁决已确认：正式 `1000ms` Retrieval P95 约束 steady-state；cold-start 可以
更长，单独记录且当前不作为 hard Promotion Gate。Warm P95 `870ms` 满足该预算。
首次 50-case 失败报告仍保持 canonical，不自动预热、不重跑，也不追溯改写为
pass；未来 cold-start SLA 必须通过新的版本化策略另行引入。

## 12. Python 工程质量 Gate

2026-07-25 在补齐 Candidate lifecycle、Provider transport、SiliconFlow BGE、
Semantic fail-open、Frozen tokenizer、Source gateway 和日志脱敏等真实边界测试后，
重新执行了完整 Python 质量关卡：

| 检查项        | 最终结果                                     | 含义                                                                           |
| ------------- | -------------------------------------------- | ------------------------------------------------------------------------------ |
| Pytest 收集   | `1913` cases                                 | 本次测试进程发现的全部测试数量                                                 |
| Pytest 通过   | `1906 passed`                                | 所有实际执行的测试均通过                                                       |
| Pytest 跳过   | `7 skipped`                                  | 未提供显式 `RAG_TEST_DATABASE_URL` / `RAG_TEST_REDIS_URL` 的 integration tests |
| Python 覆盖率 | `90.14%`，要求 `>= 90.0%`                    | 被测试执行到的 Python statement/branch 综合覆盖比例                            |
| Ruff          | `ruff check`、`ruff format --check` 全部通过 | 静态规则和格式均符合项目约束                                                   |
| Mypy          | `77 source files`，`0 issues`                | 受检 Python source/support 类型检查通过                                        |
| Dependency    | `pip-audit: No known vulnerabilities found`  | 当前锁定依赖未命中已知漏洞库                                                   |
| 定向回归      | 本轮 9 个测试文件 `172 passed`               | 新增及修改的边界测试独立复跑通过                                               |
| 定向安全扫描  | 本轮 9 个测试文件 `0` findings               | 未发现硬编码凭据、注入、危险反序列化等 scanner 模式                            |

完整复验命令：

```bash
cd mm-chat/rag
uv run ruff check .
uv run ruff format --check .
uv run mypy src tests/support
uv run pytest --cov=mm_chat_rag --cov-report=term-missing
uv run pip-audit --skip-editable
```

整个 `tests/unit` 目录的启发式 scanner 仍会命中 3 个既有 synthetic fixture：
`test_mineru_gateway.py` 一处、`test_provider_capture.py` 两处。它们都是用于验证
错误信息不会泄露测试值的本地字符串，不是真实凭据，也不属于本轮新增文件；本轮
9 个变更文件经过隔离目录扫描，结果为 `0` findings。

覆盖率只能回答“测试执行覆盖了多少代码路径”，不能回答“RAG 回答有多准确”。
Candidate 的 Recall、nDCG、MRR、Citation、Faithfulness、no-answer 和 latency
仍以本文件前述 frozen Gate 与 supplemental artifacts 为准。`90.14%` 也不会产生
Activation authority，Candidate 8 仍需 operator 对 exact Gate Report Hash 做独立
明确批准。
