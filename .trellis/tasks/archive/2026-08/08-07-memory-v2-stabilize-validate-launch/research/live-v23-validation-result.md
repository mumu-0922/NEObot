# Live schema-v23 double-confirmation Validation result

## Authority and immutable outcome

The owner granted one fresh exact live schema-v23 double-confirmation
Validation authority. It was consumed once from `2026-08-09T06:36:52Z`
through `06:42:02Z`; no rerun occurred.

```text
run:        memory-regression-20260809t063653z-20bfb10b
capture:    79231ab8-bb00-4177-aeec-daf85daf2199
cases:      100/100
outcome:    yellow / QUALITY_OR_TOKEN_GATE_FAILURE
passed:     false
report:     71e71db85fed3262388991c899aa2a4799a4c9e2ace15c87552bb8dd6dbce740
manifest:   446f84c76a040ced6e9f39d657f1d7f3206337410df1fe019dd6fdfa71000682
evidence:   /var/tmp/neo-chat-double-confirmation-validation-v23-live-20260809T063629Z
```

The report is non-selecting, non-promotional, and non-releasing. This consumes
schema-v23 and blocks rollout; it cannot be rerun or reinterpreted.

## Quality and telemetry

- Candidate Recall@20: `1.0`
- Final Recall@5: `0.9692307692307692`
- Current-fact accuracy: `0.9636363636363636`
- False injection: `0/45`
- Failed slices: `mixed_language_entity`, `stable_fact`, and
  `temporal_correction`, each at `0.9` current-fact accuracy
- Prompt Memory average/maximum: `66.53/365` tokens
- Safety/privacy counters: all zero

Two valid Judge abstentions remained after all three semantic decisions. They
produced four total confirmation attempts. The report intentionally contains
only aggregate evidence; slice overlap is not authority to recover private
case identities.

One typed `PROVIDER_TRANSPORT_FAILED` attempt recovered. The run reconciled
`60/900` Judge attempts, `107182/900000` input-token upper bound,
`6964` confirmation-only input tokens, and `7680/115200` output-token upper
bound. There were no terminal cases.

## Cleanup and production boundary

Both one-run credential copies and every scoped container, network, and volume
were destroyed. The aggregate artifacts and logs passed independent private-
field and credential-pattern scans. Every retained file is inside a mode-`0700`
root and is mode `0600`.

Pre/post state SHA-256 values are
`deabaac01944db51cbdaa2ae80ab11a88b8008bf63ab8da247a8417ba5efab65` and
`e31d3be691fba7100e459611e74e897f076bfb429d7a3f6bf1a43f00f68744f7`.
All six product container IDs and images remained unchanged and healthy. The
product composition still installs the historical negative-guard production
policy, both Memory flags remain false, and the canary remains empty. No image
pinning, recreation, rollout, smoke, or launch occurred.
