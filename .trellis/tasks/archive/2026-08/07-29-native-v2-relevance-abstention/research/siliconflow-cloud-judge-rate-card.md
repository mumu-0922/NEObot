# SiliconFlow cloud-judge rate-card authority

## Source

- Official page: <https://siliconflow.cn/pricing>
- Refetched: `2026-07-29`
- Fetched HTML SHA-256:
  `77d3555925dc9a14b3628e5351d604d12366a1fb197a9b08a36abbc8eab69368`

The rendered official data declares these current prices:

| Model | Input/prompt | Output/completion |
| --- | ---: | ---: |
| `Qwen/Qwen3-8B` | `CNY 0 / M tokens` | `CNY 0 / M tokens` |
| `Qwen/Qwen3.5-4B` | `CNY 0 / M tokens` | `CNY 0 / M tokens` |
| `deepseek-ai/DeepSeek-V4-Flash` | `CNY 1 / M tokens` | `CNY 2 / M tokens` |
| `Qwen/Qwen3.6-35B-A3B` | `CNY 1.8 / M tokens` | `CNY 10.8 / M tokens` |
| `Pro/BAAI/bge-m3` | `CNY 0.07 / M tokens` | not used |
| `Pro/BAAI/bge-reranker-v2-m3` | `CNY 0.07 / M tokens` | `CNY 0 / M tokens` |

The rate page is dynamic. Every later live phase must fetch and bind a fresh
effective timestamp rather than assuming these values remain current.

## Development cost authority

Protected inputs:

```text
fixtures.json SHA-256 = f2f51a7a72f99dc66b2b0d3a30a34775f9dca2baee6232dbbbb66fb171ec8a3c
corpus.json   SHA-256 = 51401414b6f71f4052ddc7084185d62e39eeeb2fd47ab8826382b48893185be5
Development cases      = 300
Development Memories   = 375
query UTF-8 bytes       = 50,814
Memory UTF-8 bytes      = 48,578
global largest-20 bytes = 4,781
```

One UTF-8 byte per BGE token is conservative. Using the largest 20 Memory
bodies across the entire Development corpus for every request deliberately
overbounds the SQL-authorized per-user/per-scope candidate set:

```text
rerank upper bound       = 50,814 + 300 * 4,781 = 1,485,114
query-embedding bound    = 50,814
passage-embedding bound  = 48,578
total BGE token bound    = 1,584,506
BGE price                = 70,000 CNY microunits / M tokens
maximum BGE cost         = ceil(1,584,506 * 70,000 / 1,000,000)
                         = 110,916 CNY microunits
```

The cloud judge is officially free at the fetched rate, so its exact maximum
cost is zero. Cost-basis v2 still authorizes `300` requests, `80,000,000`
conservative input tokens, and `38,400` maximum output tokens. The total
candidate Memory cost remains positive at `110,916` microunits because the BGE
stages are priced. With the unchanged `1,000,000` microunit one-chat comparison
basis, the precommitted ratio is `0.110916`, below the frozen `0.15` gate.

## Implementation consequence

Rejecting every zero judge rate would force the operator to invent a price for
an officially free fixed model. Cost-basis v2 therefore permits exact zero
judge input/output prices and zero maximum judge cost while still requiring a
positive total candidate Memory cost and exact arithmetic.

## Paid follow-up authority

The later owner-selected `deepseek-ai/DeepSeek-V4-Flash` hypothesis uses a
300,000-token aggregate judge-input ceiling, the unchanged 38,400-token output
ceiling, and the official CNY 1/M input plus CNY 2/M output rates:

```text
maximum judge cost  = 300,000 + 76,800 = 376,800 CNY microunits
maximum BGE cost    = 110,916 CNY microunits
maximum Memory cost = 487,716 CNY microunits
```

The fixed input ceiling exceeds the deterministic 258,647-byte/token upper
bound measured for the unchanged 195 non-empty Development requests.
Cost-basis v3 binds these values with
`owner_authorized_absolute_cap_v1`; schema-v5 reports the historical ratio as
informational and enforces the absolute ceilings without changing any non-cost
gate.

The next Qwen3.6 hypothesis uses the actively rendered pricing-table values,
not stale embedded model metadata. Its conservative authority is:

```text
maximum judge input cost  = 540,000 CNY microunits
maximum judge output cost = 414,720 CNY microunits
maximum judge cost        = 954,720 CNY microunits
maximum total Memory cost = 1,065,636 CNY microunits
```

The final Qwen3.5-4B latency hypothesis is officially free in the inspected
catalog. Its maximum judge cost is therefore zero, while the unchanged BGE
ceiling keeps total Memory Provider cost positive at 110,916 microunits.
