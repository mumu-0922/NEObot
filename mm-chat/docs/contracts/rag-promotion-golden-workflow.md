# RAG promotion Golden workflow

This workflow creates Candidate-only evidence consumed by
`neo-chat-rag-candidate-gate-report.v2` and
`neo-chat.rag-promotion-evaluator.v2`. It never activates an Index Generation.
The checked-in `rag-promotion-golden-draft-template.json` is deliberately
incomplete and must never be presented as a reviewed corpus or passing report.

Gate-report v1 and any Active-vs-Candidate comparison are historical
diagnostics only. They are not valid Activation evidence. Jina observations,
relative-improvement calculations, and per-slice no-regression comparisons do
not participate in Candidate admission.

## 0. Generate the synthetic draft queue

The first curation pass may be seeded from the dedicated 50-document synthetic
collection. Rebuild the exact source manifest, then bind it to the checked-in
import receipt:

```bash
research=../../.trellis/tasks/07-23-optimize-rag-chunking/research
python3 "$research/generate-promotion-draft-queue.py" \
  /tmp/neo-chat-rag-eval-corpus-20260724/manifest.json \
  "$research/live-evaluation-corpus-import-2026-07-24.json" \
  /secure/eval/promotion-golden-draft.json \
  /secure/eval/promotion-curation-queue.json
```

The generator fails closed unless the manifest/receipt describe exactly 50
unique active documents, ten per format lane, 25 per language, and ten stable
facts per document. It emits:

- a closed `neo-chat.rag-promotion-golden.v1` draft with 500 cases and exact
  `300/100/100` split counts;
- a separate synthetic review queue containing the bound source SHA-256, File
  UUID, Document UUID, fact anchor, and expected answer for each case.

Both outputs remain draft-only. The review queue explicitly sets
`promotionEligible=false`; the promotion draft has no reviewer identity,
review timestamp, freeze timestamp, or Holdout run ID. Machine-generated
questions and expected answers are reviewer aids, not human-reviewed truth.

## 1. Curate and review

Start from the draft template and keep every new case at
`review.state=draft`. The promotion corpus must contain at least 500 cases with
an exact `60/20/20` Development/Validation/Holdout split. Every critical slice
must contain at least 50 cases:

```text
pdf, text_markdown_docx, pptx, xlsx_table, json_code,
chinese, english, short_fact, cross_section, exact_numeric
```

At least 50 cases must also set `tableExactAnswerRequired=true`. A case becomes
admissible only after a human reviewer checks its question, expected answer,
source evidence, slice labels, and table/no-answer requirement, then records:

```json
{
  "state": "human_reviewed",
  "reviewerId": "<reviewer UUID>",
  "reviewedAt": "<RFC3339 timestamp>"
}
```

Preserve the machine-generated seed queue unchanged. Never copy a reviewer
identity or timestamp onto unchecked cases.

## 2. Freeze the corpus

After review, set `lifecycle.state=frozen`, set `lifecycle.frozenAt`, precommit
one UUID as `lifecycle.holdoutRunId`, and leave
`lifecycle.frozenContentSha256` absent while calculating the hash:

```bash
cd mm-chat/backend
go run ./cmd/rag-eval \
  -promotion-golden ../docs/contracts/my-promotion-golden.json \
  -print-promotion-freeze-hash
```

Copy the returned `frozenContentSha256` into the lifecycle object. The command
always returns `promotionEligible=false`; it computes identity only. Any later
edit to the corpus, review records, split, slices, criteria, or freeze time
invalidates the hash and requires a new review/freeze cycle.

`criteria.minimumAggregateQualityImprovement` remains in Golden v1 solely for
hash/schema compatibility. Candidate-only v2 accepts `0` and never reads this
field when deciding pass/fail.

## 3. Run Candidate-only Development and Validation

Capture all 400 non-Holdout cases against the exact verified Candidate and its
manifest. `rag-capture` is hard-bound to `siliconflow_bge_m3_v1`; it cannot
select Jina or execute Holdout:

```bash
cd mm-chat/backend
go run ./cmd/rag-capture \
  -promotion-golden /secure/eval/golden.json \
  -curation-queue /secure/eval/curation.json \
  -human-review-receipt /secure/eval/review.json \
  -source-import-receipt /secure/eval/import.json \
  -candidate-generation-id '4e9e18ef-c259-440b-9976-b4632e50b419' \
  -candidate-artifact-manifest-hash \
    'ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f' \
  -answer-model '<frozen answer model>' \
  -splits development,validation \
  -output /secure/eval/candidate-preflight.json \
  -pretty
```

The output schema is `neo-chat.rag-promotion-preflight.v2`, capture version
`neo-chat.rag-promotion-capture.v6`, and always
`promotionEligible=false`. Development and Validation each apply the same
Candidate-only absolute gates used by formal Holdout, including every critical
slice and the frozen latency/context budgets.

## 4. Execute exactly one frozen Holdout

Do not run Holdout until the complete Development/Validation preflight passes.
The controlled Holdout runner must produce one closed
`neo-chat.rag-promotion-observations.v1` Candidate file bound to:

- the frozen corpus hash and precommitted `holdoutRunId`;
- `holdoutRun.ordinal=1` and one execution timestamp;
- Candidate 8, its exact Search Profile, and its verified manifest hash;
- ordered retrieval candidates (at most 50), final evidence (at most 10),
  Citation evidence IDs, correctness, integrity, leakage, latency, and context
  cost for every frozen case.

The evaluator rejects missing or unknown cases, another Holdout ordinal, a
different run ID, hash drift, stale Candidate binding, or a Candidate
Generation/manifest mismatch. Holdout is one-shot and cannot be rerun for
tuning.

## 5. Generate the Candidate-only v2 gate report

```bash
cd mm-chat/backend
go run ./cmd/rag-eval \
  -promotion-golden /secure/eval/golden.json \
  -candidate-observations /secure/eval/candidate-holdout.json \
  -candidate-generation-id '4e9e18ef-c259-440b-9976-b4632e50b419' \
  -artifact-manifest-hash \
    'ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f' \
  -pretty > /secure/eval/candidate-gate-report.json
sha256sum /secure/eval/candidate-gate-report.json
```

There is no `-active-observations` or `-active-generation-id` input. The
report binds the Golden and Candidate raw SHA-256 values and applies these
absolute gates globally and to every critical slice:

- Recall@50 `>= 0.95`, Final Recall@10 `>= 0.90`;
- nDCG@10 `>= 0.85`, MRR@10 `>= 0.80`;
- Citation Correctness `>= 0.95`, Citation Completeness `>= 0.90`;
- Faithfulness `>= 0.95`, Answer Correctness `>= 0.95`;
- Table exact answer `>= 0.95`, no-answer false-answer rate `<= 0.02`;
- Citation locator, provenance, and cell lineage exactly `1.0`;
- zero ACL, deletion, secret, or unauthorized-evidence leakage;
- frozen P95 latency and average-context-token budgets.

## 6. Keep Activation separate

A passing report is evidence for an operator decision, not permission for an
automatic cutover. Record and present its exact file SHA-256. Candidate 8 must
remain `verified/ready` until the owner explicitly approves Activation and an
operator separately runs the guarded Activation command. Do not activate from
capture, evaluation, verification, deployment, restart, or migration code.
