# Schema-v20 accuracy-repair result

## Decision

Schema-v19 isolated the systematic loss to Luna selection. The only semantic
repair changed the separately versioned Luna system prompt from v1 to v2. BGE,
model/provider bindings, decoder, input/output schema, negative guard, buffered
transport, retry/cooldown schedule, corpus, criteria, and final intersection
were unchanged.

## Fake lifecycle

- Run: `memory-regression-20260807t042423z-ee2b05e9`
- Capture: `cb1e52bf-18a6-48bc-a4ac-87791d8b9dc5`
- Result: 300/300 complete, `passed=true`, zero network, zero scoped Docker or
  credential residue.
- Prompt: `memory-cloud-candidate-judge-prompt-v2`, SHA-256
  `90fac3f3c97a340e6ef1963dc1456c5f088ac083658a29e75aad747efba95d90`.
- During the first Fake attempt, the new Recorder used prompt v2 while the
  Provider controller still calculated token authority with prompt v1. The
  mismatch failed closed before publication. The controller now receives the
  same versioned prompt builder, a regression test covers the boundary, and the
  successful Fake run is the clean successor.

## Sole live Development

- Run: `memory-regression-20260807t042823z-67950546`
- Capture: `50d54ec5-2003-42bf-b8f5-87a094aa8e48`
- Result: 300/300 complete, `passed=false`.
- Candidate Recall@20: `1.0`.
- Final Recall@5: `0.9692307692`.
- Current-fact accuracy: `0.9636363636`.
- False injection: `0/135`; every safety counter was zero.
- Failed slices:
  - `stable_fact`: current-fact accuracy `0.9333333333`.
  - `temporal_correction`: current-fact accuracy `0.9333333333`.
- Luna abstentions: `6`.
- Judge attempts/retries: `176/11`; all eleven
  `PROVIDER_TRANSPORT_FAILED` attempts recovered and terminal failures were
  zero.
- Cost reconciliation: `176/900` requests, `316702/2000000` input-token upper
  bound, `22528/115200` output-token upper bound.

## Runtime boundary

Credentials were exported from the protected pre-066 backup restored into an
isolated PostgreSQL 17 container with the pinned helper image. The restore
container, network, volume, and credential files were destroyed. The live empty
PostgreSQL database was not restored or changed. Backend and Memory Worker
container IDs remained unchanged and exited; the PostgreSQL container ID stayed
unchanged and healthy. `MEMORY_HYBRID_SHADOW_ENABLED=false`,
`MEMORY_TOOL_LOOP_ENABLED=false`, and no canary allowlist was installed.

Schema-v20 failed the unchanged full-Development gate. Per the confirmed chain,
schema-v21 Validation must not be constructed or run, and no canary rollout is
authorized.
