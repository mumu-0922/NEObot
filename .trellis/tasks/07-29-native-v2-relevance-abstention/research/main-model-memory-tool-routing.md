# Main-model Memory Tool routing

## Decision question

Determine the next Development-only relevance hypothesis after three
SiliconFlow candidate-judge models failed unchanged recall, false-injection,
and latency gates. The owner wants to reuse the already selected GPT or
DeepSeek chat Provider rather than introduce a hidden Qwen dependency. BGE-M3
embedding and reranking remain fixed.

## Decisive local evidence

The original live native-v2 comparison already isolated the remaining
problem:

- Candidate Recall@20 was `1.0`;
- un-gated BGE Final Recall@5 and current-fact accuracy were both `1.0`;
- all `50/500` false-injection cases were the `unrelated_negative` slice;
- cross-user, deleted, secret, and untrusted-source leak counts were zero.

The retrieval and ordering stages therefore do not need another model swap.
The missing capability is a semantic decision about whether the current turn
should consult personal Memory at all.

Three candidate-aware SiliconFlow judge runs did not solve that decision:

- `Qwen/Qwen3-8B`: Final Recall@5 `0.758974`, false injection `14/300`,
  p95 `1853 ms`;
- `deepseek-ai/DeepSeek-V4-Flash`: Final Recall@5 `0.143590`, `164/195`
  judge calls hit the reserved hard cutoff;
- `Qwen/Qwen3.6-35B-A3B`: Final Recall@5 `0.733333`, false injection
  `15/300`, p95 `1854 ms`.

This evidence rejects further model-only hopping on that gateway. It does not
prove that every hosted Provider or every main-model Tool decision is
infeasible.

## Existing product seam

The chat runtime already has a provider-normalized Tool Loop:

- `chat.ToolRoundProvider` supports native automatic function calls and
  same-model continuation;
- OpenAI-compatible Providers cover the configured GPT and DeepSeek services;
- `search_web` and `search_knowledge` establish the read-only retrieval Tool
  pattern, bounded arguments, sanitized execution events, cancellation, and
  same-model compatibility fallback;
- Tool-capability state is already bound to Provider configuration and model.

This seam can host a read-only `search_memory` Tool without a second hidden
model or a second chat Provider configuration.

## Selected Development hypothesis

Use the current turn's selected GPT or DeepSeek model as the Memory intent
router through the exact `search_memory` Tool contract:

1. The first model round receives conversation context and the Tool definition,
   but no Memory candidates or Memory bodies.
2. No Tool Call means `no_memory`; prefetched retrieval evidence, if any, is
   discarded and never reaches the model.
3. One valid `search_memory` call authorizes the backend to run the unchanged
   BGE-M3 embedding, RRF, rerank, Top-5, and token-budget path.
4. SQL ownership/scope/revision/hash/epoch/generation authority is checked
   before hydration and again after Provider work.
5. The bounded Tool result is returned to the same Provider/model for normal
   continuation. No independent candidate-judge Provider is used.

The Development benchmark must reproduce the exact Tool definition and
decision semantics. A Tool Call on an unrelated-negative case counts as false
injection once the non-empty BGE final set would enter the continuation. A
missing Tool Call on a relevant case loses final recall/current-fact credit.
No metric or gate changes.

## Rejected alternatives

### Pass every candidate to the answer model in one prompt

Rejected. The model would see unrelated candidate bodies before it had made
the relevance decision. Under the unchanged benchmark contract those bodies
have already entered the answer prompt, so relabelling later `usedMemoryIds`
would silently weaken false-injection semantics.

### Continue with `Qwen/Qwen3.5-4B`

Rejected without a live run. It repeats the same gateway, independent-judge,
prompt, and deadline structure after three failed models. The empty credential
and unused private cost basis were destroyed. Tracking must record
`cancelled_not_run_architecture_pivot`, not a fabricated result.

### Promote the ungated BGE Top 5

Rejected. It has perfect Development recall but deterministically injects
Memory for unrelated-negative turns.

### Add a local LLM judge now

Deferred. It would add a new model/runtime/hardware dependency despite the
existing main-model Tool Loop. It remains a fallback only if the exact
main-model Tool decision fails unchanged Development gates.

## Evaluation and rollout boundaries

- The new lane is Development-only and receives a new schema/profile/policy
  identity.
- The exact Tool definition, Provider type/identity, model, decoding behavior,
  cost authority, and decision adapter are configuration-hashed.
- The isolated runner must use fresh credential files for any live run; it
  must not read or copy production vault secrets.
- GPT and DeepSeek are separate named hypotheses. A result from one cannot
  authorize the other.
- v1 remains the only production prompt/Usage authority. v2 remains
  default-off and shadow-only.
- Validation remains forbidden until one fixed Development profile passes all
  unchanged recall, current-fact, false-injection, latency, token, authority,
  privacy, and absolute-cost gates.

## Expected trade-off

Memory-relevant turns may require a second main-model round for Tool
continuation. That is explicit user-selected model work, not a hidden judge.
The first-round Tool decision and BGE retrieval may be speculatively overlapped
only if no candidate body is released before a valid Tool Call and the measured
latency contract remains truthful.
