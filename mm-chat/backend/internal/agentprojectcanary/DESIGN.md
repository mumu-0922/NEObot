# agentprojectcanary design

## Scope

This package proves exactly one mutable effect through the existing
Orchestrator -> Runner -> private relay -> Broker seam. The effect is a
synthetic PostgreSQL Project CAS, not a product Project adapter.

## State flow

```text
activation + approval gate
  -> reconcile terminal cleanup / require empty Runner
  -> enqueue stable Run and acquire one Attempt
  -> signed launch and heartbeats
  -> signed Prepare through caller-specific relay
  -> verify prepared intent against offline approval
  -> append fixed per_commit approval
  -> signed Commit
  -> resolve durable Project status when acknowledgement is lost
  -> terminalize and cancel/reap Sandbox
  -> restore exact baseline and retain content-free facts
```

## Invariants

1. Plan authority is exactly `project.patch/project.write/apply_patch`, mutable,
   non-idempotent and `per_commit`, with one exact resource and flat UTF-8 path.
2. Grant and Registry are derived server-side; Runner requests cannot widen
   subject, Tool, action, resource, arguments, base revision or TTL.
3. Approval binds a stable activation identity rather than the activation file
   hash, preventing an approval/activation hash cycle.
4. Approval is checked after Prepare against the returned immutable intent.
   The fixed approval ID and durable fingerprint collision fence provide
   single-use semantics without predicting random Intent IDs.
5. After a possible send, only status is queried. No restart or replay can
   dispatch a second CAS.
6. Cleanup requires the matching committed Broker receipt, restores only the
   reviewed baseline and retains immutable mutation/cleanup authority.

## Error matrix

| Condition | Result |
| --- | --- |
| activation, plan, approval or key drift | unavailable before DB/Runner access |
| approval rejected or expired | no Project write |
| stale lease/generation | `LEASE_STALE`, zero receipt |
| revoked Grant | `GRANT_DENIED`, zero receipt |
| Kill Switch epoch/mode drift | `KILL_SWITCH_ACTIVE`, zero receipt |
| committed receipt after lost acknowledgement | committed exactly once |
| clean unchanged base without receipt | failed without redispatch |
| conflicting/unavailable status | terminal `outcome_unknown` |
| cleanup crash | recovery pending; restart performs cleanup only |

## Non-goals

- user Project writes, deletes, traversal or multi-file patches;
- MCP writes, generic Egress, Secrets, Provider calls or package-selected input;
- API/Chat Agent execution, Child Agents, Cron, Learning or Skill install;
- exact-host activation on the current development machine.

## Known limitations

- The Project store is a dedicated synthetic PostgreSQL authority, not the
  product Project model or a user-workspace adapter.
- Only one flat UTF-8 file replacement is admitted; deletes, traversal,
  multi-file patches and package-selected arguments remain unavailable.
- Disposable PostgreSQL, Compose and source gates do not replace exact-host
  rootless isolation acceptance. This development host remains
  `ISOLATION_UNAVAILABLE`.

## Change history

### 2026-08-15 — G21.3 initial implementation

Added strict one-action bindings, detached `per_commit` approval, dedicated
private relay routing, durable Project CAS/status/cleanup reconciliation and
terminal no-retry recovery.
