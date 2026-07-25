# Candidate 8 supplemental no-answer 回归结果

日期：2026-07-25  
性质：独立 supplemental diagnostic，`promotionEvidence=false`  
结论：**no-answer 正确性与安全边界全通过；总 Gate 因冷启动阶段的 Retrieval P95 超过 1000ms 而失败。**

## 1. 为什么执行这份回归

冻结的 500-case Golden 中 `expectedNoAnswer=true` 的 case 数为 0，因此正式
Candidate-only Gate 中的 `NoAnswerFalseAnswerRate=0` 只是空 cohort 默认值，不能
证明模型面对不存在的文档或事实时会拒答。

本回归独立补充 50 个 synthetic no-answer cases，不修改 frozen Golden、不消费或
重跑 one-shot Holdout，也不构成 Promotion 或 Activation evidence。

## 2. 精确绑定

```text
Candidate Generation:       4e9e18ef-c259-440b-9976-b4632e50b419
Candidate Sequence:         8
Artifact Manifest:          ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f
Chunk Profile:              36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73
Retrieval Profile:          siliconflow_bge_m3_v1
Embedding:                  Pro/BAAI/bge-m3
Reranker:                   Pro/BAAI/bge-reranker-v2-m3
Answer Model:               gpt-5.6-sol
Generation Head Revision:   4
Corpus Projection Revision: 298
```

所有运行时 Retrieval provider 调用均为 SiliconFlow BGE；没有 Jina 调用或
fallback。

## 3. Suite 组成

| 维度               | Cases |
| ------------------ | ----: |
| Chinese            |    25 |
| English            |    25 |
| PDF                |    10 |
| DOCX               |    10 |
| PPTX               |    10 |
| XLSX               |    10 |
| Markdown JSON/code |    10 |

每个 query 同时包含一个唯一且未导入的 filename 与 subject token，例如：

```text
no-answer-pdf-zh-01.pdf
QZ-NOANSWER-PDF-ZH-01
```

生成器和 Go loader 都会拒绝 imported filename collision、重复 case/token、覆盖
不足、BGE profile 漂移、Candidate/manifest/hash/model/revision 漂移。

## 4. Gate 与实测结果

| 指标                                     |        Gate |          实测 |   结果   |
| ---------------------------------------- | ----------: | ------------: | :------: |
| False-answer rate                        |     `<= 2%` | `0 / 50 = 0%` |   通过   |
| Language/format slice false answers      |         `0` |      全部 `0` |   通过   |
| Cases with Citation evidence             |         `0` |           `0` |   通过   |
| Cases with `[K#]` markers                |         `0` |           `0` |   通过   |
| Absent filename matches                  |         `0` |           `0` |   通过   |
| Absent subject matches                   |         `0` |           `0` |   通过   |
| ACL/deletion/secret/unauthorized leakage |         `0` |           `0` |   通过   |
| Average context tokens                   |   `<= 4096` |     `2212.88` |   通过   |
| Retrieval P95                            | `<= 1000ms` |      `1606ms` | **失败** |

所有 50 个回答都归一为 `INSUFFICIENT_EVIDENCE`，报告只保存统一的 answer
SHA-256，不保存回答原文：

```text
8a6b10089bedd2a3eb619080fb096a529b097604cc2cdf5eab8f724025128670
```

因此，这次回归已经提供了该 exact Candidate/suite 下的 no-answer 行为证据，但
整份 supplemental Gate 不能标记为 passed。

## 5. Latency 失败定位

最慢四个 case 恰好是并发启动时最先执行的四个 PDF Chinese cases：

| Case                               |  Total | Embed | Fetch | Hydrate | Rerank |
| ---------------------------------- | -----: | ----: | ----: | ------: | -----: |
| `supplemental-no-answer-pdf-zh-04` | 1655ms | 561ms | 275ms |   471ms |  348ms |
| `supplemental-no-answer-pdf-zh-01` | 1624ms | 561ms | 277ms |   466ms |  319ms |
| `supplemental-no-answer-pdf-zh-03` | 1606ms | 561ms | 271ms |   469ms |  303ms |
| `supplemental-no-answer-pdf-zh-02` | 1494ms | 486ms | 226ms |   484ms |  295ms |

其余 46 cases 最大值为 `692ms`，总体 P50 为 `513ms`。四个并发首批 case 的
Embedding、DB Fetch、Hydration 和 Rerank 同时升高，因此随后在一个全新的 helper
process 内进行了严格顺序的 cold/warm 诊断：先并发执行 PDF、DOCX、PPTX、XLSX
各第一个 Chinese case，Cold 阶段完全结束后，再并发执行各格式第二个 Chinese
case。结果如下：

| 阶段 | Cases |    P50 |    P95 |    Max | 1000ms Gate |
| ---- | ----: | -----: | -----: | -----: | :---------: |
| Cold |     4 | 1470ms | 1498ms | 1498ms |    失败     |
| Warm |     4 |  859ms |  870ms |  870ms |    通过     |

Warm P95 比 Cold P95 下降 `628ms`，降幅 `41.9226%`。8/8 cases 仍全部正确拒答，
Citation evidence、`[K#]` markers、absent source/subject matches 及全部 leakage 均
为 0；因此诊断结论为 `cold_start_effect_observed`，且
`diagnosticIntegrityPassed=true`。

这确认了**新 helper process 冷启动效应存在**，不再是未验证假设；但
该 paired diagnostic 没有分别隔离连接池初始化、HTTP/TLS 建连和数据库 cache，
所以不能把 `628ms` 全部归因到其中某一个组件，也不能据此改写或放宽已执行的
canonical first-run Gate。

## 6. 生命周期与不可覆盖性验证

执行后状态保持：

```text
Active Generation:      sequence 3
Candidate Generation:   sequence 8, verified / ready
Head / corpus revision: 4 / 298
Activation gate hash:   NULL
Activation audits:      0
```

没有调用 `-execute-frozen-holdout`，没有删除或替换已有 Holdout seal/report，没有
执行 Activation。Supplemental report 即使失败也已先完整落盘；再次写入同一路径
会被拒绝。

## 7. 证据文件与 Hash

目录：

```text
.trellis/tasks/07-23-optimize-rag-chunking/research/
```

| 文件                                                                   | 用途                | SHA-256                                                            |
| ---------------------------------------------------------------------- | ------------------- | ------------------------------------------------------------------ |
| `generate-supplemental-no-answer-suite.py`                             | 可复现 suite 生成器 | `13703645113d9d63913c9d60f558cae7c634b68ebc962cd4c2e81c9cda5bd7cb` |
| `supplemental-no-answer-suite-candidate8-2026-07-25.json`              | 50-case bound suite | `ef9d273cf1a25db1165f8c411f985338a485745813639be1412a423cdbafc553` |
| `supplemental-no-answer-report-candidate8-2026-07-25.json`             | 完整实测报告        | `77488e1363e2b025ae1dc260a594fabd102218356d1f47f68853cbe8ad6ca7ad` |
| `supplemental-no-answer-latency-diagnostic-candidate8-2026-07-25.json` | Cold/Warm 诊断      | `54d5523245b1108337fdb5824a22dbe3224d1fc3c4ac4294c2875c3c42cfecf3` |

首次 50-case capture 使用的静态 binary：

```text
e1644f36e26127024aa5fbe1091a72e5106f00210af320decfd902afedc3421b
```

Cold/Warm 诊断使用的静态 binary：

```text
ec87daa7f5a71386fdc1e195cdeec2d38bd4c666ed37346356f434401cdad858
```

## 8. 下一步建议

保留当前失败报告作为 canonical first run，不覆盖、不事后调低阈值。版本化的
cold-start/warm-state diagnostic 已确认新 helper process 冷启动效应存在，但该
诊断仍不是 Promotion evidence 或 Activation authority。

产品裁决已确认：`1000ms Retrieval P95` 约束 steady-state；全新 helper process
的 cold-start 可以更长，单独记录且当前不设置 hard Promotion Gate。Warm P95
`870ms` 满足 steady-state 预算。

该裁决不追溯修改既有证据：首次 50-case 报告仍保持 `passed=false`，不覆盖、不
重跑、不自动预热，也不放宽其创建时的 `1000ms` Gate。未来若要约束 cold-start，
必须用新的版本化 SLA 和报告 schema 明确引入。
