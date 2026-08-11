# Single-user bounded-miss criteria boundary

## Evidence and owner decision

The consumed schema-v23 live Validation completed `100/100`, kept false
injection and every safety/privacy counter at zero, and passed overall current-
fact accuracy at `0.9636363636`. Two valid-empty requests remained after the
primary plus both confirmations. Because the required slice denominator is ten
current-fact cases, one omission produces `0.9`; the historical `0.95` slice
threshold is therefore discretely equivalent to requiring `10/10`.

The owner accepts occasional fail-closed omission for the sole current user.
They do not authorize false Memory, retroactive reinterpretation, or a rerun of
schema-v23.

## Prospective criteria

Create a fresh criteria identity with separate global and required-slice
current-fact thresholds:

```text
overall minimum current-fact accuracy:       0.95
required-slice minimum current-fact accuracy: 0.90
maximum false-injection rate:                 0
maximum false-injection cases:                0
```

Candidate Recall@20, Final Recall@5, prompt-token, safety, privacy, cost,
cleanup, completion, and terminal-failure gates remain unchanged. Latency
remains diagnostic-only. The v4 primary/two-confirmation policy and all
Provider/prompt/decoder/BGE/negative-guard semantics remain unchanged.

## Version and evidence boundary

- Historical criteria v3 JSON, hashes, schema-v22 Development, and schema-v23
  Validation remain immutable.
- The new criteria must be encoded in a fresh schema rather than modifying the
  existing `AccuracyFirstCriteria` JSON emitted by historical reports.
- A fresh Development and fresh Validation identity must bind the new criteria
  hash before any Provider request.
- Fake lifecycle and cost derivation precede each live authorization. No live
  authority is inferred from the owner's criteria preference.
- Any launch remains behind the exact UUID allowlist and applies only to the
  current sole user. A second user requires a new rollout decision.

## Rejected shortcuts

- Mark schema-v23 Green under the new threshold.
- Add a third or unbounded confirmation.
- Use BGE-only fallback after Luna abstains.
- Permit any non-zero false injection in exchange for recall.
- Reuse schema-v22/schema-v23 cost, approval, report, or run identities.
