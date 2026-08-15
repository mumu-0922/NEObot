# Agent Child Canary Design

## Goals

- Prove exactly one `depth=0 -> depth=1` production-path delegation.
- Make every subject, model, package/runtime, expiry and four-dimensional
  budget transition equal or narrower.
- Demonstrate physical `delegate_task` removal before Child Registry
  fingerprinting.
- Make Child-first cascade/reap and restart recovery replay-safe.

## Non-goals

Public delegation, sibling fan-out, depth two, package-selected arguments,
Provider work and every Broker effect remain out of scope. This package does
not promote the current development host beyond `ISOLATION_UNAVAILABLE`.

## Flow

```text
strict plan
  -> stable Parent enqueue/acquire/launch
  -> Root delegation authority registration
  -> stable Child enqueue + empty Registry admission
  -> Child acquire/launch/heartbeat
  -> durable Child cascade
  -> wait latest launch authority expiry
  -> Runner list/reconcile/list exact Child absence
  -> migration 093 atomic Sandbox/reap completion
  -> zero-usage canceled settlement
  -> signed Parent cancel + atomic Parent terminal chain
```

## Decisions

### Empty Child Grant

A valid depth-one Grant may contain zero capabilities. The Child still requests
`delegate_task`, but `BuildRegistry` removes it before capability matching and
fingerprinting. The resulting Registry is valid and empty; there is no hidden
callable alias.

### Durable reap transport

Migration `093` exposes pending work only through a `SECURITY DEFINER`
inventory. It returns exact Attempt/Sandbox fingerprints and the maximum
matching launch-authority expiry without returning a lease token. Successful
completion terminalizes the exact Runner projection and reap fact atomically.

### Restart monotonicity

Stable Parent and Child idempotency keys plus `ListChildren` make any existing
lineage a permanent second-Child fence. Lost lease tokens are never guessed;
the controller waits for expiry. A replacement Parent generation is allowed
only after the sole Child is terminal and only to finish Parent cleanup.

## Threat model

- **Late signed launch:** wait through its durable expiry before Runner reap and
  enforce the same fence again in PostgreSQL completion.
- **Depth or authority widening:** Go derivation, PostgreSQL subset checks and
  Runner launch validation independently reject it.
- **Wrong Sandbox reap:** match Run, Step, Attempt, generation, snapshot, spec,
  probe and optional Sandbox ID; ambiguity fails closed.
- **Controller compromise:** the LOGIN inherits only Orchestrator Runtime,
  Runner Control and Delegation Control. It has no owner role or direct DML.
- **Untrusted plan:** strict JSON, private regular-file loading, fixed argv,
  no-network Sandboxes and server-owned Tool catalog prevent input expansion.

## Known limits

The slice runs one synthetic lineage and relies on exact-host evidence created
outside this development machine. Product delegation remains a later stage.

## Change history

- 2026-08-15: Initial G21.4 depth-one canary and migration-093 reap bridge.
