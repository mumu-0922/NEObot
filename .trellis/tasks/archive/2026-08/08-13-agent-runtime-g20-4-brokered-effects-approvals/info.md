# G20.4 Brokered Effects — Technical Design

## Decision

Create `internal/agentbroker` as the Backend-owned policy and effect bounded
context. Migration `086` owns durable intent, approval, Commit claim, receipt
and handle-digest state. Executors never decide authorization. Runner only
validates exact signed/replay-fenced Prepare/Commit envelopes and calls an
injected Broker relay; no DB/vault/object credential enters `neo-runnerd`.

## Data flow

```text
frozen snapshot + Capability Grant + current Tool catalog
  -> deterministic Registry intersection/fingerprint
  -> Sandbox broker request through exact Attempt channel
  -> Runner local fence + signed method authority
  -> Backend Broker authorization
  -> PostgreSQL immutable Prepare intent
  -> automatic or authenticated human approval fact
  -> atomic Commit claim under lease/grant/Kill/budget fences
  -> Egress/Secret/MCP/Project/Artifact executor adapter
  -> exact receipt query or terminal outcome_unknown
  -> durable sanitized receipt + Run state consequence
```

## Privilege model

```text
agent_effect_owner    NOLOGIN; owns migration 086 tables/functions
agent_effect_control  NOLOGIN; SELECT + exact function EXECUTE only
neo-runnerd           non-root; no DB/vault/object/MCP credential
Sandbox               opaque broker channel only; no direct network/secret
```

## Recovery and rollback

- Prepared/awaiting intents expire provider-free while Runtime is disabled.
- A `committing` intent is never reset to prepared after restart. Reconcile by
  exact executor idempotency status; unknowable results become
  `outcome_unknown`.
- Roll back the Backend/Runner artifact with migration `086` retained inert.
  Down refuses while any durable effect authority exists.
- No public route/startup wiring means prior Backend images can ignore `086`
  safely while cleanup/reconciliation remains an operator responsibility.
