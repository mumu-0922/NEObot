# Live v4 double-confirmation Development result

## Authority and execution

The owner granted exactly one fresh live v4 double-confirmation Development
authority. It was consumed once from `2026-08-09T03:36:39Z` through
`2026-08-09T03:52:20Z`; status was zero and no rerun occurred.

The run used the frozen v22 cost document with raw/canonical SHA-256
`82771c9ebb4dd521cd4b5aa584a58f39cc2e786b3677ced6e523755c8a4c2de0`/
`1599e276ae4de940a889b51b5b11e861483b25558b9be34b66607f97717bce16`.
Its hard ceilings were `2700` Judge requests, `2700000` input tokens,
`345600` output tokens, and `3045600000000` owner-budget microunits.

## Immutable result

- Run: `memory-regression-20260809t033639z-a1cde25c`
- Capture: `c04bb647-96a9-4f1e-b127-ab93ed027f05`
- Cases: `300/300`
- Outcome: Green, `passed=true`
- Candidate Recall@20: `1.0`
- Final Recall@5: `0.9897435897435898`
- Current-fact accuracy: `0.9878787878787879`
- False injection: `0/135`
- Required slice failures: zero
- Safety/privacy counters: all zero
- Average/maximum prompt Memory: `67.85/381` tokens
- P95/P99 diagnostic latency: `7828/10770` milliseconds

The run made `184` Judge attempts, including `11` recovered primary retries.
There were `8` confirmation attempts and zero confirmation retries. Judge
input-token upper bound reconciled at `329182`, including `13819`
confirmation-only tokens. Output-token upper bound was `23552`. There were no
terminal cases.

Configuration/report/manifest SHA-256 values are
`836de9c5df3dc44f7be24ef54be90d10dfb1a31f0ec14129557de8eeea4f4ca1`,
`52b454c6faf11784f7852f2784c889b6f34c5b53e72146c221a37e2613c1ef5e`,
and `a690ad5f1a30238587336b8c4c99180789e87b432d54aa8402385b208e32a664`.
The private evidence root is
`/var/tmp/neo-chat-double-confirmation-development-v22-live-20260809T033639Z`.

## Boundary after completion

Both one-run credential copies, the export container, and every regression
container/network/volume were destroyed. All six product services remained
healthy. `MEMORY_HYBRID_SHADOW_ENABLED=false`,
`MEMORY_TOOL_LOOP_ENABLED=false`, and the canary allowlist remained empty.

The report is Development evidence only: `policySelected=false` and
`promotionEligible=false`. It grants no Validation quota authority and cannot
enable production. Schema-v21 remains consumed. The next step is a fresh,
separately versioned double-confirmation production-policy Validation identity,
with network-free Fake lifecycle and a new frozen cost document before asking
for one live Validation authority.
