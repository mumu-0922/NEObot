# Live schema-v21 Validation result

## Outcome

The fresh repaired one-shot schema-v21 Validation completed all 100 cases but
failed two unchanged required-slice quality gates. The immutable outcome is
Yellow `retain_beta`, with `passed=false`, `releaseEligible=false`,
`policySelected=false`, and `promotionEligible=false`. It is consumed and is
not rerun.

Run `memory-regression-20260808t131254z-f11b8601`, capture
`cf29492b-54d5-4367-a7c7-151b1c465dcd`, ran from
`2026-08-08T13:13:19Z` through `2026-08-08T13:18:33Z`. Private evidence is
retained under
`/var/tmp/neo-chat-abstention-confirmation-validation-20260808T131253Z`.

## Quality and safety

- Candidate Recall@20, Final Recall@5, and current-fact accuracy were
  `1.0`, `0.9846153846`, and `0.9818181818`.
- False injection was `0/45`; every privacy and safety counter was zero.
- `mixed_language_entity` and `stable_fact` each reached only `0.9`
  current-fact accuracy, below the unchanged `0.95` slice requirement.
- Exactly one Judge decision abstained. The aggregate arithmetic shows that
  the omitted current fact belonged to both failed slice memberships; no
  private case identity is retained.
- One confirmation request was attempted. Five typed
  `PROVIDER_TRANSPORT_FAILED` retries recovered, leaving zero terminal cases.
- Judge authority reconciled at `61/600` requests, `110083/600000`
  input-token upper bound, and `7808/76800` output-token upper bound.
- Average/maximum prompt Memory was `67.64/365` tokens, below the unchanged
  `600/900` limits.

## Evidence identities

```text
configuration SHA-256:      505cd678c703c2a0124e1af72d8ab4d83633e48810ed5f43f65aec3c7be01a99
Validation order SHA-256:   cea5ebff03cef920deb4b5a9b36bee45e2f17ecb1d1b4987bc4f902fc1c8d430
production policy SHA-256:  63e1191d81a7579bc89f781187598a8887ac870eb59185ddf9e19a5c73dede47
raw cost SHA-256:           cd60276abf34cdd2629cc68d416828154692990b93fbb5e06690733e4c4c442d
decoded cost SHA-256:       0990f44dd5ca7da2f31f07251aadcb7a00c49a42d8463b72ae229ad9a98a344b
report SHA-256:             3aff1211c6d1a2bbe422fb9053deda18726a71d5374441f85cd8179d5cc60a13
manifest SHA-256:           0378b0a685267b2bb371cf35c5b965ee871a9001fc23337143b230cd176bd986
```

The report and manifest contain no query, Memory plaintext, raw Provider body,
or credential field. Both exported credentials and every scoped container,
network, and volume were destroyed. Product services remain healthy, both
Memory flags remain false, and the canary is unset. Production policy
promotion, image pinning, rollout, and smoke were not attempted; v1 remains
authoritative.
