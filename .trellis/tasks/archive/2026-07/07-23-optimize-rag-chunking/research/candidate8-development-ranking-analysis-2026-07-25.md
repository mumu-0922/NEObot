# Candidate 8 Development ranking analysis

## Bound evidence

- Input report:
  `promotion-preflight-v5-candidate8-development-2026-07-25.json`
- Candidate Generation:
  `4e9e18ef-c259-440b-9976-b4632e50b419`
- Analysis used only the frozen Development split. Validation and Holdout were
  not executed.

## Finding

All 300 positive Development cases retrieved their single expected evidence in
Top 50. The ranking loss was introduced after the BGE rerank boundary:

```text
Retrieved expected rank: {1: 204, 2: 86, 3: 2, 4: 5, 5: 3}
Final expected rank:     {missing: 2, 1: 202, 2: 46, 3: 24,
                          4: 17, 5: 4, 6: 2, 7: 2, 8: 1}
MRR@10:                  0.795980
nDCG@10:                 0.845287
```

The weakest slices were `cross_section`, `xlsx_table`, and `json_code`. In all
300 Development queries the normalized authorized filename basename was
explicitly present in the query. For the 298 cases whose expected evidence
survived Top 10, it was always the highest BGE-ranked Child from its target
document; unrelated documents were placed ahead of it. The remaining two
cases still had the expected Child in retrieval Top 5 but lost the complete
target document at final Top 10.

## Decision

Preserve the existing migration-048 source-name routing signal after BGE
rerank with one versioned deterministic fusion rule:

1. Match only a bounded normalized authorized filename basename explicitly
   present in the original or rewritten query.
2. Add a rank boost that cannot be exceeded by a `[0,1]` BGE relevance score.
3. Keep BGE relevance order among Children from the matched document.
4. Keep filename metadata outside source text and Citation authority.
5. Apply the identical rule in production chat and Candidate capture; do not
   change thresholds, Golden labels, or evaluator math.

This is a runtime ranking correction, not evaluator-only compensation. It is
versioned as production policy `g18-profiled-reranker-golden-v2`, capture
`neo-chat.rag-promotion-capture.v6`, and scoring policy
`synthetic-curator-bound-evidence-v4`.

## Live verification

Three previously decisive failures were replayed first against Candidate 8 and
the real SiliconFlow provider. The expected evidence moved to Rank 1 in each
case, and answer correctness, Citation support, and faithfulness all passed:

```text
rageval-code-zh-04-f09   missing -> Rank 1
rageval-xlsx-zh-03-f08  Rank 4  -> Rank 1
rageval-docx-en-08-f08  Rank 7  -> Rank 1
```

The complete v6 Development and Validation captures then passed without
executing Holdout or Activation:

```text
Development: 300 cases
  Recall@50 / Final Recall@10 / nDCG@10 / MRR@10 = 1.0
  Answer Correctness / Faithfulness / Table Exact = 1.0
  P95 latency = 600 ms; average context = 1857.31 tokens
  SHA-256 = 1ffc4a6335a7ff092eaca32ed0de2c443a4a1b95ddd9e5aea2d208d29959f7cc

Validation: 100 cases
  Every aggregate and critical-slice quality metric = 1.0
  P95 latency = 652 ms; average context = 1860.75 tokens
  SHA-256 = 3569f1a3a0f28bf3c1173b7e166b442d01694fa193081a1e164371e42f77bead
```

Both reports retain `promotionEligible=false` and
`holdout.state=not_executed`. The precommitted one-shot Holdout remains behind
separate explicit owner approval.
