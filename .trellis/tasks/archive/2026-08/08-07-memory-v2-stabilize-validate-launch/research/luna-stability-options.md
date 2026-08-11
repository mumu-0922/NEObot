# Luna stability repair options

## Current mechanics

- Luna receives one system prompt plus a strict JSON payload containing only
  the query and at most 20 ordinal candidates.
- Decoding is already `temperature=0`, max 128 output tokens, and no thinking.
- Prompt v2 already says not to abstain when an explicit saved-fact request has
  a directly matching current fact.
- Candidate retrieval, fixed BGE rerank, final ordinal intersection, strict
  JSON decoding, negative-policy guard, and transient transport retries passed
  the v20 diagnostic.
- Therefore changing BGE thresholds or bypassing Luna would broaden the defect
  surface without evidence.

## Feasible approaches

### A. Prompt v3 only

Add a separately versioned, more mechanical decision order for same-subject,
same-scope current facts while leaving one Luna call per admitted query.

- Advantage: lowest latency and cost; one semantic variable changes.
- Risk: v2 already contains the essential instruction, so prompt wording alone
  may not eliminate provider-side stochastic abstention.

### B. Bounded abstention confirmation (recommended)

Keep the first strict Luna decision. Only when it returns an empty ordinal set
after Candidate/BGE admission, invoke a separately versioned confirmation
decision with the same candidates and a narrow “directly answers the requested
fact” contract. Accept only strict valid output and continue to intersect with
BGE. Keep the negative-policy guard before both calls.

- Advantage: targets the observed empty-selection defect, leaves ordinary
  successful queries at one call, and preserves fail-closed decoding.
- Risk: adds latency and Provider cost on abstentions and must prove that the
  confirmation path does not increase false injection.

### C. Deterministic BGE fallback

If Luna abstains, inject the top BGE candidate directly.

- Advantage: removes Luna false negatives.
- Risk: bypasses the semantic safety boundary and can convert lexical/vector
  similarity into false memory injection. This contradicts the current design
  and is rejected.

## Recommendation

Use option B under a fresh Development identity. Change only the empty-result
confirmation boundary; keep prompt v2 for the primary call and keep Candidate,
BGE, negative guard, strict decoder, final intersection, and thresholds
unchanged. Fake fixtures must cover call bounds and fail-closed behavior. The
full 300-case Development remains the only semantic selection gate.
