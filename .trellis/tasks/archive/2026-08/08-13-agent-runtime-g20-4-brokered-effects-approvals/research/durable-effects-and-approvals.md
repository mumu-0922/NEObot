# Durable Effects, Approvals and Recovery

## Evidence

- Neo Phase 0 contract:
  `mm-chat/docs/contracts/agent-runtime.md`, especially sections 4, 5, 8, 10,
  13 and 15.
- Existing durable authority:
  `backend/internal/agentorchestrator` and migrations `084`/`085`.
- Stripe idempotent request contract, captured 2026-08-13:
  <https://docs.stripe.com/api/idempotent_requests>.
- HTTP retry semantics, RFC 9110 section 9.2.2, captured 2026-08-13:
  <https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2>.

External pages are design evidence only. Checked-in Neo contracts and live code
remain authoritative.

## Findings

Stable idempotency requires more than attaching a random key to an executor
call. The durable layer must bind one key to one canonical intent, reject a key
reused with different parameters, and return the original terminal receipt for
an exact replay. A validation failure before execution is not a completed
effect. Once dispatch might have begun, transport failure alone cannot prove
that the effect did not happen.

Neo already has the correct lease and Kill Switch authorities in PostgreSQL.
Effect authorization must join the prepared intent to the exact current
Run/Step/Attempt generation, lease token digest, frozen snapshot/grant, current
Kill Switch epoch, approval and budget. The executor receives no authority to
change those facts.

## Recommended state model

```text
prepared -> awaiting_approval -> approved -> committing
prepared|awaiting_approval|approved -> expired|rejected|canceled
committing -> committed|failed|outcome_unknown
```

- `automatic` creates an internal approval fact during Prepare.
- `once` requires an authenticated human decision bound to the exact immutable
  intent fingerprint.
- `per_commit` requires a distinct approval for each prepared intent.
- `denied` never creates a usable prepared intent.
- Commit claim and `committing` transition happen transactionally before the
  executor boundary.
- Exact completed replay returns the stored sanitized receipt.
- A different fingerprint for the same key is a replay/security failure.
- An acknowledgement loss is reconciled through an exact executor status
  query. When the executor cannot prove the result, the intent becomes terminal
  `outcome_unknown`; it is never automatically committed again.

## Crash matrix

| Point | Recovery |
| --- | --- |
| before prepared row | no effect; exact Prepare may be attempted again |
| after Prepare, before approval | pending intent expires provider-free |
| after approval, before Commit claim | cancel/Kill Switch can still fence it |
| after claim, before send | executor may resume only with the same key and proof that no send occurred |
| after send, before acknowledgement | exact status/receipt query; otherwise `outcome_unknown` |
| after receipt, before response | persisted receipt is returned by exact replay |

## Repository consequence

Migration `086` should own immutable intents, approval decisions and receipts
behind an `agent_effect_control` role with SELECT plus exact SECURITY DEFINER
functions and no table DML. `neo-runnerd` receives no database role. Down must
refuse while any effect authority exists. Runtime-off cleanup must still expire
prepared intents and retain ambiguous receipts.
