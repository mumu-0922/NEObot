# Agent Broker Design

```text
admitted package + frozen Grant + server Tool catalog
  -> canonical Registry fingerprint
  -> Prepare (no mutable effect)
  -> automatic fact or authenticated approval
  -> optional authenticated Cancel (same intent lock as Commit)
  -> atomic PostgreSQL Commit claim
  -> Egress / Secret / MCP / Project / Artifact executor
  -> exact receipt query or outcome_unknown
```

## Authority

PostgreSQL migration `086` is authoritative for effect intents, approvals,
cancellations, Grant revocations, receipts and secret-handle digests. The
narrow `agent_effect_control` role can
read these rows and call exact `SECURITY DEFINER` functions, but cannot mutate
tables directly. Runner and public API roles receive no effect authority.

## Non-negotiable fences

- Request replay binds lease owner/token digest, TTL and Kill Switch epoch.
- Prepare reserves per-capability and total Tool-call budgets under a per-Run
  advisory lock; Commit rechecks the consumed budget.
- Mutable execution is claimed once. A possible dispatch is queried by stable
  idempotency key and otherwise becomes `outcome_unknown`.
- Cancel and Commit lock the same intent. Cancel may advance only an approved
  or awaiting-approval intent plus its prepared Attempt; a Commit winner cannot
  later be labeled rolled back.
- Read-only execution uses a separate interface. Retry is eligible only for a
  Registry Tool explicitly classified read + idempotent + automatic, and only
  before any result is observed; production invocation remains unwired.
- Secret values and plaintext handles stay memory-only and are cleared after
  use; only handle digests and binding fingerprints are durable. PostgreSQL
  validates the exact committing intent, live lease and current Kill Switch
  before resolution/consume. Grant revocation, cancellation, expiry and
  terminal completion clear matching memory-only values and revoke durable
  handle digests.
- Agent Egress and MCP share `internal/safenet`, including dial-time DNS/IP and
  redirect revalidation. The Agent allowlist additionally rejects IP literals.
- Artifact publication is object-before-row with compensating object deletion.
  It reads bounded quarantine bytes once, rechecks the exact size and SHA-256,
  scans those same bytes, then publishes object-before-row with compensating
  object deletion. It never mutates Project files.

## Held wiring

`neo-runnerd` can validate and replay-fence strict Prepare/Commit requests, but
its default Broker relay is unavailable. Enabling a production relay, public
Agent API, Chat integration, real Project store or mutable canary requires a
later release group and exact-host acceptance.
