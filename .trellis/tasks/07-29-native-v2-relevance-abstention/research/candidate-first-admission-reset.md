# Candidate-first Memory admission reset

## Decision question

After the main-model `search_memory` Tool-route hypothesis failed, what is the
next Development-only relevance/abstention policy that preserves implicit
personalization without injecting unrelated Memory?

## Runtime distinction

Memory use has three separate stages:

1. **Candidate recall** privately searches current-authorized Memory.
2. **Admission** decides whether any recalled candidate is useful for the
   current turn.
3. **Injection** releases only the admitted, reranked, reauthorized final set
   to the answer model.

Candidate recall is not prompt injection. A route model that runs before
candidate recall is blind to implicit personalization. For example, a generic
meal request does not reveal that the database contains an allergy or diet
constraint. The route can detect explicit phrases such as “use my saved
preference,” but it cannot establish whether an ordinary-looking request has a
relevant stored fact.

## Existing evidence

- Native Exact/CJK BM25/BGE-M3/RRF candidate generation already reached
  Candidate Recall@20 `1.0` on Development.
- Ungated BGE final ranking reached Final Recall@5/current-fact `1.0`, but it
  injected Memory for unrelated-negative cases.
- Scalar local similarity, maximum rerank score, rerank margin, and query-only
  bilingual intent policies could not separate relevant cases from unrelated
  negatives under the unchanged gates.
- Three SiliconFlow candidate-aware judge models retained the correct
  candidate-first topology but failed quality and/or the two-second cutoff.
- The later configured GPT/DeepSeek Tool-route experiments did **not** test
  candidate-aware judging. They saw no Memory body and failed because a blind
  Tool decision was unreliable and its Tool/SSE protocol surface was fragile.
- Schema-v9 completed only `12/300` route decisions and achieved Final
  Recall@5 `0.010256`; it selected no policy.
- Schema-v10 then tested the previously unmeasured configured-model candidate-
  aware topology. GPT completed `0/195` candidate-bearing judge decisions;
  DeepSeek completed `157/195`, but reached only `0.558974` Final Recall@5 and
  `0.581818` current-fact accuracy. Both exceeded the latency criterion. Both
  kept false injection and every authority/privacy leak counter at zero, so
  the topology is safe under the fixture but neither exact profile is eligible
  for Validation.

## Comparable system behavior

- Hermes starts provider prefetch for each eligible turn and treats an empty,
  failed, or timed-out provider result as no Memory. Its manager does not ask
  the answer model to route before provider recall.
- TencentDB Agent Memory runs L1 retrieval on each turn, combining Chinese FTS
  and optional vector results before threshold/Top-K selection. It also exposes
  deeper search tools, but its automatic L1 path is recall-first.
- Neither system proves Neo Chat's stricter false-injection target. They support
  the topology decision—recall before admission—not the final admission model.

## Feasible approaches

### A. Configured main-model candidate judge (evaluated; failed gates)

Run current-authorized candidate recall first. Send the secret-redacted query
and request-local candidate ordinals/bodies to the exact configured GPT or
DeepSeek Provider through the existing strict candidate-judge contract. The
judge returns an exact ordinal set or an empty set. Intersect it with BGE order,
then reauthorize and hydrate only the final Top-5 set.

This is a new hypothesis even though the adapter already exists: prior
configured GPT/DeepSeek runs tested candidate-blind Tool routing, while prior
candidate-aware runs tested different SiliconFlow judge models.

Benefits:

- candidate-aware reasoning can detect implicit personalization;
- no Tool Call/SSE Tool-shape authority is required;
- existing strict prompt, ordinal decoder, BGE intersection, authority fences,
  aggregate recorder, and fake protocol can be reused;
- it uses the owner's already configured chat Provider family rather than
  adding another production model vendor.

Costs and risks:

- it adds a bounded pre-answer model round on turns with candidates;
- candidate text crosses the configured Provider boundary under the explicit
  owner policy;
- it may still fail the existing p95/p99 and two-second cutoff;
- model/provider/request-shape drift requires separate validation.

The Development profile kept the existing latency and quality gates. The exact
GPT profile failed reliability, quality, and latency. The exact DeepSeek
profile preserved safety but failed recall, current-fact accuracy, and latency.
This branch is closed for the tested Provider/model identities; it cannot be
rescued by increasing the cutoff or weakening a gate.

### B. Local candidate-aware model (recommended successor)

Use a version-pinned multilingual reranker or small instruction model on the
single server after candidate reauthorization. The current host has an RTX
5060 Ti with 16 GiB VRAM, so a bounded local model is technically feasible.

Benefits: no candidate cloud egress and potentially predictable latency after
warm-up. Costs: a new GPU runtime, model supply chain, image/model storage,
health/warm-up handling, and benchmark work. Ordinary semantic reranking alone
may repeat the already observed score-overlap failure; the local model must be
trained or prompted for **answer usefulness**, not generic similarity.

This is now the recommended successor because the configured models either
failed to complete strict decisions or failed relevance despite seeing the
candidates. It must be treated as a new version-pinned architecture with its
own supply-chain, warm-up, resource, usefulness-classification, and unchanged-
gate evidence—not as an in-place schema-v10 retune.

### C. Always inject thresholded Top-K

This mirrors the simpler TencentDB automatic path. It is rejected because the
existing Development evidence already shows false injection when all
authorized BGE results are admitted. Candidate recall may be unconditional;
prompt injection may not.

### D. Continue Tool-route diagnostics

Rejected. More taxonomy does not make the candidate-blind decision informed,
and the failed schema-v6/v7/v9 artifacts already establish that the request
shape cannot satisfy the unchanged gates.

## Evaluated schema-v10 topology

```text
current user query
  -> exact / CJK BM25 / BGE vector candidate recall
  -> current-user/scope/revision/hash/epoch/generation authorization
  -> secret redaction
  -> configured GPT or DeepSeek strict candidate-aware ordinal judge
     || fixed BGE-M3 rerank
  -> intersect judge ordinals with BGE order
  -> Top-5 and 600/900-token selection
  -> post-Provider reauthorization and final hydration
  -> inject only the non-empty final set; otherwise normal chat without v2
```

No candidate-blind route is part of admission. Speculative candidate recall is
allowed because it remains request-local and has no prompt or Usage authority.
The topology was executed for exact GPT and DeepSeek profiles; neither passed.

## Implementation boundary

- Preserve all historical schema-v4/v5/v6/v7/v8/v9 artifacts and identities.
- Add a schema-separated Development-only configured-main-model candidate
  judge profile; do not reinterpret the old SiliconFlow judge profile.
- Reuse `usermemory.BuildHybridCandidateJudgePrompt`, strict ordinal decoding,
  `memoryjudge.ChatAdapter`, hybrid candidate execution, and BGE intersection.
- Bind exact Provider ID/type/base-URL hash/model, adapter/prompt/decoding
  versions, BGE tuple, owner egress policy, cost authority, and unchanged gates
  before Provider construction.
- Keep `MEMORY_TOOL_LOOP_ENABLED=false`, v1 as prompt/Usage authority, and all
  promotion/Validation paths blocked until Development passes.
- The authorized live GPT and DeepSeek runs are complete and immutable failed-
  gate evidence. No additional configured-model paid run, Validation, or
  Promotion follows from them.
- A local candidate-aware model requires a new profile/model-manifest/resource
  contract and fresh Development evidence before it can become a selection
  candidate.

## Selection rule

Neither exact configured GPT nor DeepSeek profile passed, so no schema-v10
policy may be frozen and Validation remains unavailable. Models remain
separate hypotheses; one result never authorizes the other. No automatic
retry, adaptive bakeoff, cutoff increase, or gate relaxation is allowed. A
future local candidate-aware profile starts a new hypothesis with the same
quality/safety/latency authority.
