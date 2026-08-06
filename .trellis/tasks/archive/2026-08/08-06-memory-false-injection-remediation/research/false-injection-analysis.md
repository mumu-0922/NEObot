# Schema-v15 false-injection analysis

## Evidence boundary

- Live evidence directory:
  `/home/mumu/.local/state/neo-chat/memory-validation/runs/20260806T013956Z-31e67617`
- Report SHA-256:
  `6b2ec1a0cf26b2190302accac384f9fab4fce0898d1b1bad1eaacb5a2ce39c69`
- Manifest SHA-256:
  `3ee114b2991ad2d0de954ad4a5998947567c66672e010dc079f17c73c18ae650`
- Corpus SHA-256:
  `51401414b6f71f4052ddc7084185d62e39eeeb2fd47ab8826382b48893185be5`
- Only the already-consumed Validation split was inspected. Holdout content was
  not displayed, classified, or evaluated. No Provider request was made.

The retained report is intentionally aggregate-only. Its nine
`evaluation.failures` entries are criterion/slice messages, not case IDs, and
no case-level candidate, score, Judge output, or final-selection artifact was
retained. Therefore the exact identities of the nine failed cases cannot be
recovered from immutable evidence without a newly authorized run. Any claim to
name those exact nine would be fabricated.

## Decisive correlation

- Validation has 100 cases: 55 relevant and 45 expected-no-memory.
- All 9 false injections are inside the 10-case `unrelated_negative` slice:
  that slice reports `falseInjectionCases=9` and `falseInjectionRate=0.9`.
- Those 10 IDs, in frozen Validation order, hash to
  `1e8aa17ce6f8426ce9c91d3be7ffeef34be2bb8b14d0eaa9a8616b5426f0bc6f`.
- The same cases overlap `chinese_paraphrase`, `mixed_language_entity`,
  `untrusted_source`, `secret_rejection`, `scope_isolation`, and `deletion`.
  This explains the slice failures without implying separate leaks: every
  cross-user/deleted/Secret/untrusted-source safety counter remained zero.

The negative queries are meta-policy questions asking whether unrelated notes
should influence, override, control, or be used for an answer. They mention
Memory-like terms and project entities, so lexical/vector retrieval can produce
candidates. The fixed Judge prompt says to select information that is
"directly useful" and to avoid broad-topic matches, but does not define this
meta-policy shape as a mandatory abstention. Nine of ten therefore received a
non-empty final selection.

## Runtime chain

1. `chat.detectExplicitMemoryReadIntent` only forces a first-round call for
   narrow explicit reads; it does not veto model-routed Tool calls.
2. `CaptureProductionMemoryJudgeValidation` calls the candidate reader for all
   frozen Validation cases and does not execute the chat Tool-offering gate.
3. The production policy uses BGE rerank followed by Luna Judge with
   `MinimumProviderSimilarity=-1` and `MinimumFinalRelevanceScore=0`.
4. No deterministic negative meta-policy guard exists before candidate
   admission, rerank, or Judge selection.

Relevant code:

- `mm-chat/backend/internal/chat/memory_read_intent.go`
- `mm-chat/backend/internal/usermemory/hybrid_candidate_judge.go`
- `mm-chat/backend/internal/usermemory/hybrid_shadow.go`
- `mm-chat/backend/internal/memorycapture/profiles.go`
- `mm-chat/backend/internal/memorycapture/production_memory_judge_validation.go`

## Offline prototype

A narrow bilingual prototype required all of these semantic components:

1. an unrelated/irrelevant record, note, or Memory subject;
2. a policy modal such as should/can/是否/应该; and
3. an influence/override/control/use/recall predicate.

Against only the consumed Validation split it classified:

- 16 of 45 expected-no-memory cases;
- all 10 `unrelated_negative` cases, which necessarily cover the unknown nine;
- 0 of 55 relevant cases.

The 16 classified IDs in frozen order hash to
`a3c322d299a24c3443b92e9e7136b53bed8fd17e1d0a9bd71815937e41ba76c2`.
This is an offline design check, not replacement Validation evidence.

## Feasible approaches

### A. Versioned deterministic meta-policy guard (recommended)

Add a hash-bound bilingual guard to a new Development-only relevance policy.
After repository preparation but before relevance admission, candidate rerank,
or Luna Judge, a matched query records an empty final set with a bounded
`NEGATIVE_POLICY_QUERY_ABSTAINED` fallback. Query embedding may already have
occurred, but no Memory candidate plaintext reaches either hosted rerank or
Judge. Keep the current production-v1 policy and runtime flags unchanged.

Benefits: deterministic coverage, zero consumed-Validation relevant false
negatives in the offline check, auditable fallback, and one changed variable.
Cost: a new policy identity and later Development/Validation evidence are still
required before promotion.

### B. Judge prompt v2 only

Add mandatory abstention examples/rules to the Luna prompt. This is cheaper in
code but probabilistic, changes the prompt hash globally, and repeats the exact
authority that failed 9/10 meta-policy cases.

### C. Numeric rerank/final threshold

Raise similarity thresholds. The retained aggregate evidence contains no
case-level scores, so no safe threshold can be selected offline; it could harm
the currently strong Final Recall@5 and current-fact accuracy.

## Recommendation

Implement Approach A only. Do not combine it with a prompt or threshold change
in the same calibration cycle. Add unit tests plus a local consumed-Validation
audit, keep production recall disabled, then request separate authorization for
a new Development calibration. Promotion, a new Validation schema/run,
Holdout, release, and re-enable remain out of scope.
