# Candidate 8 frozen Holdout — 2026-07-25

## Outcome

The precommitted frozen Holdout was executed exactly once for Candidate 8.
The Candidate-only v2 promotion evaluator passed every aggregate and critical-
slice gate. No activation command was executed, the Active pointer remains on
Generation sequence 3, and Candidate 8 remains `verified / ready`.

## One-shot execution binding

```text
Candidate Generation:  4e9e18ef-c259-440b-9976-b4632e50b419 (sequence 8)
Artifact manifest:     ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f
Chunk profile:         36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73
Retrieval profile:     siliconflow_bge_m3_v1
Embedding model:       Pro/BAAI/bge-m3
Rerank model:          Pro/BAAI/bge-reranker-v2-m3
Holdout run ID:        ba1749c9-922e-4ab7-b6c0-37603e582617
Holdout ordinal:       1
Executed at:           2026-07-25T10:44:50Z
Capture ID:            7e887707-5657-457c-a518-d754de98e20e
Head / corpus revision: 4 / 298
Capture binary SHA-256: 627756db92d00bb4bda2c75f5197b0b8cb6ee4c86b9f19b1395ba6de3b0818fe
```

The runner validated the complete 300-case Development and 100-case Validation
reports, their raw hashes, input hashes, provider/model tuple, Generation,
manifest, Chunk Profile, answer model, scoring policy, and live revisions before
creating the exclusive seal. The seal was created before any Holdout provider
request. A repeated invocation was rejected before runtime initialization.

## Holdout-only result (100 cases)

```text
Recall@50 / Final Recall@10:  1.000 / 1.000
nDCG@10 / MRR@10:            0.9963092975 / 0.995
Citation correctness:         1.000
Citation completeness:        1.000
Faithfulness:                 1.000
Answer correctness:           1.000
Table exact-answer:           1.000
P95 retrieval latency:        658 ms
Average context:              1941.59 tokens
Locator/provenance/lineage:   100%
Leakage:                      0
```

## Formal 500-case gate

```text
Report schema:                 neo-chat-rag-candidate-gate-report.v2
Passed:                        true
Recall@50 / Final Recall@10:   1.000 / 1.000
nDCG@10 / MRR@10:             0.9992618595 / 0.999
Citation correctness:          0.9980039920
Citation completeness:         1.000
Faithfulness:                  1.000
Answer correctness:            1.000
Table exact-answer:            1.000
P95 retrieval latency:         644 ms (budget 1000 ms)
Average context:               1874.856 tokens (budget 4096)
Locator/provenance/lineage:    100%
Leakage:                       0
Critical slices passed:        10 / 10
Python activation validator:   accepted exact report and SHA-256
```

## Evidence

```text
One-shot seal:
  promotion-holdout-one-shot-candidate8-seal-2026-07-25.json
  SHA-256 d713943addb4d3a2dffc62b1a679c7566b85ad89787d15f97545c4de9b84ecea

Complete 500-case observations:
  promotion-observations-v1-candidate8-2026-07-25.json
  SHA-256 1ef4624733fd9443c3ecda3d551b28d0db815fd7d753321991cebaa44d07702c

Formal gate report:
  promotion-gate-v2-candidate8-2026-07-25.json
  SHA-256 fd93ae98700cdc923dfff3b82c6520eb13663ae03cc1927f09103387015b608b
```

## Activation boundary

Post-execution live verification:

```text
Active Generation:       46a1c7bb-44ed-4868-9d61-edd557f9d3f0 (sequence 3)
Candidate Generation:    4e9e18ef-c259-440b-9976-b4632e50b419 (sequence 8)
Candidate status:        verified / ready
Activation gate hash:    NULL
Activation audits:       0
Services:                Backend/Frontend/RAG Worker/Postgres healthy
```

The passing report is evidence for an operator decision only. It does not
authorize implicit activation. The exact report SHA-256 above must be reviewed
and separately approved before any Candidate 8 pointer transition.
