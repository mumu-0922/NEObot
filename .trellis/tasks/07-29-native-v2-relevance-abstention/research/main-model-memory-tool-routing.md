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

## Live Development update

The first implementation adapted `chat.ToolPlanner`. That produced one
independent, non-streaming route request before the normal answer request; it
did not reuse the first `ToolRoundProvider` round described below.

The GPT profile bound `SERVER_DEFAULT/gpt-5.6-sol` and completed `41/300` route
decisions (`40` use, `1` abstain). It failed `259` cases: `250` reported
`HARD_CUTOFF` and `9` reported `MEMORY_TOOL_ROUTE_FAILED`. Final Recall@5 was
`0.087179`, current-fact accuracy was `0.090909`, false injection was `2/300`,
and p95/p99 was `2002/2003 ms`.

The first `FOHWSU/deepseek-v4-pro` execution completed no route decisions, but
it is not model-quality evidence. The adapter sent
`{"enable_thinking":false}` to official `api.deepseek.com`; DeepSeek's official
contract requires `{"thinking":{"type":"disabled"}}`. Retain the run only as
`protocol_mismatch_invalid_quality_evidence`.

Official protocol reference:
<https://api-docs.deepseek.com/guides/thinking_mode>.

After correcting the official DeepSeek wire shape, the
`FOHWSU/deepseek-v4-flash` profile completed `77/300` routes (`62` use, `15`
abstain) and failed `223` (`2` `HARD_CUTOFF`, `221`
`MEMORY_TOOL_ROUTE_FAILED`). Final Recall@5 was `0.256410`, current-fact
accuracy was `0.254545`, false injection was `3/300`, and p95/p99 was
`1377/1808 ms`. Every authority/privacy leak counter remained zero, but the
unchanged quality and latency gates failed.

Schema v6 does not retain a stable subtype under
`MEMORY_TOOL_ROUTE_FAILED`. Quota, rate limiting, overload, transport failure,
and other Provider errors therefore remain unverified explanations for the
Flash count.

## Schema-v7 first-round live result

The first schema-v7 Development hypothesis bound
`SERVER_DEFAULT/gpt-5.6-sol` to the real first `ToolRoundProvider` request. It
completed `28/300` decisions, all `28` choosing `search_memory`, and failed
closed on `272`: `266` `HARD_CUTOFF` plus `6`
`MEMORY_TOOL_ROUTE_FAILED`. Candidate Recall@20 remained `1.0`, but Final
Recall@5 was `0.102564`, current-fact accuracy was `0.109091`, false injection
was `2/300`, and p95/p99 was `2002/2002 ms`. The unrelated-negative slice had
`2/30` false injections, so both global quality/latency and slice safety gates
failed even though all cross-user, deleted, secret, untrusted-source, and
unauthorized Provider-egress counters remained zero.

This is valid failed quality evidence for the intended request shape, not a
schema-v6 preflight result. The conservative cost/token authority passed, the
two retained files remained mode `0600`, both transient Server Vault copies
were destroyed, and the isolated Compose project left zero containers,
networks, or volumes. No GPT policy was frozen; Validation and Promotion remain
blocked. A separately authorized DeepSeek schema-v7 hypothesis remains unrun.

## Architecture conclusion

No profile passed and no policy was frozen. The independent `PlanTools`
preflight amplifies Provider requests, quota exposure, latency, and transient
failure before answer generation. Increasing the two-second cutoff or retrying
the preflight would not test the originally selected architecture.

The next implementation should instead register `search_memory` beside
`search_web` and `search_knowledge` in the existing first
`ToolRoundProvider.StreamToolRound` request. A valid call then executes bounded
current-authorized Memory retrieval and returns the result in same-model
continuation. No call continues without v2 Memory. This is the actual reuse of
the product Tool Loop.

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
model or a second chat Provider configuration. It must also avoid a separate
pre-answer `PlanTools` request; merely sharing Provider code and credentials is
not the same as sharing the first model round.

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
- For the completed Development runs, the owner explicitly authorized
  transient mode-`0600` decrypted copies from the existing Server Vault. They
  were overwritten and removed after use, and the runner itself never opened
  the Vault. A future Validation run still requires fresh independent
  credentials.
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
It must not require a preliminary third request: the first answer round itself
makes the Tool decision. BGE retrieval may be speculatively overlapped only if
no candidate body is released before a valid Tool Call and the measured latency
contract remains truthful.
