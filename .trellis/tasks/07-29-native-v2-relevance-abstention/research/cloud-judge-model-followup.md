# Cloud candidate-judge model follow-up

## Purpose

Select the next Development-only cloud candidate-judge hypothesis after the
first schema-v4 run failed the unchanged relevance and latency gates. This
artifact does not authorize Validation, Holdout access, reader promotion, or a
silent evaluator change.

## Observed schema-v4 result

The fixed `Qwen/Qwen3-8B` Development run completed all 300 admitted cases on
2026-07-29. Candidate Recall@20 remained `1.0`, but the judge policy failed:

- Final Recall@5: `0.7589743589743589` (`>=0.90` required);
- current-fact accuracy: `0.7515151515151515` (`>=0.95` required);
- false injection: `14/300 = 0.04666666666666667` (`<=0.02` required);
- p95/p99 latency: `1853/1855 ms` (`<=900/1500 ms` required);
- `31/195` judge requests failed closed under `HARD_CUTOFF`;
- every authority/privacy leak counter remained zero.

The result is a truthful failed hypothesis. Increasing the two-second cutoff,
lowering relevance/safety/latency gates, or entering Validation is forbidden.

## Official rate and model evidence

Source: <https://siliconflow.cn/pricing>

- fetched on 2026-07-29;
- fetched HTML SHA-256:
  `77d3555925dc9a14b3628e5351d604d12366a1fb197a9b08a36abbc8eab69368`;
- `deepseek-ai/DeepSeek-V4-Flash`: CNY `1.00 / M` prompt tokens and
  CNY `2.00 / M` completion tokens;
- the official model description identifies a 284B-total/13B-active MoE,
  1M-token context, and explicit Non-think support.

The exact Provider behavior and latency remain unverified until a live run.
Marketing/model-size metadata is not quality evidence.

## Selected next hypothesis

Evaluate `deepseek-ai/DeepSeek-V4-Flash` first. It is the strongest available
candidate in the inspected SiliconFlow rate card and explicitly supports the
non-thinking mode required by the existing strict-ordinal judge adapter. The
prompt, strict JSON schema, `temperature=0`, `max_tokens=128`, no-thinking
setting, BGE models, concurrency, token limits, hard cutoff, corpus, and every
quality/safety/latency gate remain byte- or value-identical to schema v4.

Do not run an adaptive multi-model bakeoff. A later model requires another
named hypothesis, fresh price authority, fresh configuration hash, and fresh
Development evidence. Validation data must never select a model.

## Cost-policy conflict discovered before quota use

The previous free judge happened to satisfy the frozen relative Provider-cost
gate. A truthful paid-model cost basis cannot:

```text
maximum judge input tokens       = 300,000
maximum judge output tokens      = 38,400
judge prompt cost ceiling        = 300,000 CNY microunits
judge completion cost ceiling    =  76,800 CNY microunits
fixed BGE cost ceiling           = 110,916 CNY microunits
total Memory Provider ceiling    = 487,716 CNY microunits
chat comparison denominator      = 1,000,000 CNY microunits
relative ratio                   = 0.487716
historical ratio gate            = 0.15
```

The 300,000-token input ceiling is above the deterministic `258,647` UTF-8
byte/token upper bound observed for the unchanged 195 non-empty Development
judge requests. One UTF-8 byte per token plus fixed framing remains the
conservative recorder rule. The output ceiling remains 300 requests times 128
tokens even though only non-empty cases issue requests.

Spending quota under schema v4 would therefore produce `passed=false` even if
every quality, safety, and latency criterion passed. Inventing a free price,
inflating the chat denominator, or ignoring the report failure is forbidden.

## Versioned owner-budget decision

The owner has explicitly stated that Provider expense is not a product
selection constraint for this single-owner Server-mode deployment. Implement
a schema-separated owner-budget policy before the paid live run:

```text
owner_authorized_absolute_cap_v1
```

Under this policy:

- the historical `providerCostRatio` remains reported truthfully but is
  informational for this profile;
- the exact official per-token rates, request limit, input/output token
  ceilings, and maximum run cost remain hash-bound and enforced before and
  after Provider work;
- exceeding any authorized absolute ceiling invalidates the run;
- quality, safety, latency, hard-cutoff, token, split, privacy, and promotion
  gates remain unchanged;
- historical schema-v4/free-model evidence keeps its original relative-cost
  semantics and bytes;
- the new cost-policy identity is present in the cost basis, profile
  configuration, report, and run manifest.

This is a product economics policy change, not a relevance-gate relaxation.
It must use new schema identities and focused compatibility tests before any
fresh credential is created.

## Schema-v5 live result

The fixed `deepseek-ai/DeepSeek-V4-Flash` hypothesis completed its authorized
300-case Development run on 2026-07-29. The absolute budget, artifact, leak,
permission, and teardown contracts passed, but the reader policy failed:

- Candidate Recall@20: `1.0`;
- Final Recall@5: `0.14358974358974358`;
- current-fact accuracy: `0.14545454545454545`;
- false injection: `0/300`;
- p95/p99 latency: `1856/1865 ms`;
- empty/completed/failed judge cases: `105/31/164`;
- all 164 failures were fail-closed `HARD_CUTOFF` events;
- every authority/privacy leak counter remained zero;
- actual judge input/output upper bounds were `258647/24960`, both inside the
  authorized `300000/38400` ceilings.

This model is infeasible under the unchanged two-second production boundary.
Its low final recall is dominated by 164 cutoff failures and is not evidence
for prompt retuning. The Key and isolated runtime were destroyed. Validation
remains blocked.

## Next named model hypothesis

The next Development-only hypothesis is `Qwen/Qwen3.6-35B-A3B`. The official
rate-card description identifies a 35B-total/3B-active MoE with explicit
thinking/non-thinking support. Its lower active parameter count makes it the
strongest inspected candidate with a plausible latency improvement over the
13B-active DeepSeek hypothesis. Quality and latency remain unverified.

Use the unchanged schema-v5 owner budget, prompt, decoding profile, BGE tuple,
concurrency, hard cutoff, and every non-cost gate. Bind the new model ID,
fresh official rate evidence, exact conservative prices, and a new cost-basis
hash. This is a second named Development hypothesis, not a Validation choice
or an adaptive prompt change.

## Qwen3.6 live result

The fixed `Qwen/Qwen3.6-35B-A3B` Development run completed on 2026-07-29 but
also failed:

- Candidate Recall@20: `1.0`;
- Final Recall@5/current-fact accuracy: `0.7333333333333333`;
- false injection: `15/300 = 0.05`;
- p95/p99 latency: `1854/1856 ms`;
- empty/completed/failed judge cases: `105/155/40`;
- all 40 failures were fail-closed `HARD_CUTOFF` events;
- every authority/privacy leak counter remained zero.

The Key/runtime were destroyed and Validation remains blocked. Together with
the Qwen3-8B and DeepSeek results, this shows that another large-model swap is
not a sufficient hypothesis: the hosted per-turn judge frequently saturates
the reserved cutoff and still misses both recall and precision targets.

## Cancelled final cloud-latency falsification

The planned `Qwen/Qwen3.5-4B` run was cancelled before Provider construction or
quota use. The owner selected an architecture pivot that reuses the current
GPT or DeepSeek chat Provider through the existing Tool Loop instead of adding
another hidden Qwen judge. The empty credential and unused private cost basis
were destroyed.

Its tracking status is `cancelled_not_run_architecture_pivot`; this is not a
quality result for Qwen3.5. The three completed SiliconFlow runs are sufficient
to stop model-only hopping on the unchanged independent-judge structure, but
they do not claim that every hosted Provider is infeasible.

The next Development hypothesis is documented in
[`main-model-memory-tool-routing.md`](main-model-memory-tool-routing.md). It
keeps BGE-M3 retrieval/rerank, lets the user-selected main model decide whether
to call `search_memory` before seeing Memory bodies, and changes no benchmark
gate.
