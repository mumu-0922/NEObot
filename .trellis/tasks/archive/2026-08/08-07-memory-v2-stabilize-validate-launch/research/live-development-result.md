# Live abstention-confirmation Development result

## Authority and execution

The owner granted a fresh, exact one-shot live Development authority after the
earlier pre-network credential-copy failure. The replacement invocation ran
exactly once from `2026-08-07T10:00:45Z` through `2026-08-07T10:15:49Z` and
exited zero. It did not grant Validation or launch authority.

The immutable run is
`memory-regression-20260807t100046z-3c7d216f`, capture
`85b2d1a1-8bca-4888-9e89-f1dbdec6acb4`. Private evidence is retained under
`/var/tmp/neo-chat-abstention-confirmation-development-20260807T100027Z`.

## Quality and safety

- `300/300` cases completed and the report has `passed=true`.
- Candidate Recall@20, Final Recall@5, and current-fact accuracy are
  `1.0`, `0.9897435897`, and `0.9878787879`.
- False injection is `0/135`; all required slices passed.
- Cross-user, deleted-Memory, Secret, untrusted-source, and unauthorized-egress
  counters are all zero.
- Three logical confirmations produced four attempts, including one retry.
  Ten primary Judge retries and that confirmation retry recovered; no terminal
  case remained.
- Judge authority reconciled at `179/1800` requests,
  `321531/2000000` input-token upper bound, and `22912/230400` output-token
  upper bound.
- Prompt Memory averaged `67.85` tokens and peaked at `381`, below the
  unchanged `600/900` limits.

## Evidence identities

```text
configuration SHA-256: 15a957e610e96d2048a617b73f513d003bbb5f0c856b8c9bd789307a10094026
raw cost SHA-256:      fa814cda981e10f7c7a596db51f82aabf0fcaa6571a02e8283fc63aa99c7853f
decoded cost SHA-256:  e488d0a657dba7fe05d444e4bf38587a4ec16b2f46de74dee07ad5e4c1285119
report SHA-256:        bcfd12e7d735f5b8bec09edd7fd74d22b8c7fb0f99e50f2d9367627811a3cf91
manifest SHA-256:      59e322e5b4817d009013606a3972514dae34e9de3e68aa7995f9350a6951597a
```

The aggregate JSON contains no query, Memory plaintext, raw Provider body, or
credential field. Both exported credentials were destroyed. The exact scoped
containers, networks, and volumes are absent. Product services remained
healthy, both Memory flags remained false, and no canary allowlist was added.

## Gate state

Development is Green. Schema-v21 live Validation remains the next independent
paid gate. This result is non-promotional (`policySelected=false`,
`promotionEligible=false`) and cannot itself authorize production policy
promotion, flag changes, or rollout.
